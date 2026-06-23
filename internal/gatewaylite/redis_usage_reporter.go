package gatewaylite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultUsageReportTimeout = 10 * time.Second
	defaultUsageQueueWorkers  = 1
	defaultUsageBatchSize     = 50
	defaultUsageStreamMaxLen  = 10000
	maxUsageReportAttempts    = 10
	maxUsageRetryDelay        = 30 * time.Second
)

type UsageReportClient interface {
	ReportUsage(ctx context.Context, event UsageEvent) error
}

type UsageBatchReportClient interface {
	ReportUsageBatch(ctx context.Context, events []UsageEvent) error
}

type RedisUsageReporter struct {
	client      *redis.Client
	prefixValue atomic.Value
	prefixMu    sync.RWMutex
	prefixes    map[string]struct{}
	sink        UsageReportClient
}

type UsageQueueStats struct {
	StreamLength int64
	PendingCount int64
	DeadCount    int64
}

type queuedUsageEvent struct {
	Event      UsageEvent `json:"event"`
	Attempts   int        `json:"attempts"`
	EnqueuedAt int64      `json:"enqueued_at"`
}

func NewRedisUsageReporter(client *redis.Client, prefix string, sink UsageReportClient) *RedisUsageReporter {
	reporter := &RedisUsageReporter{client: client, sink: sink}
	reporter.SetPrefix(prefix)
	return reporter
}

func (r *RedisUsageReporter) Enabled() bool {
	return r != nil && r.client != nil
}

func (r *RedisUsageReporter) WithPrefix(prefix string) *RedisUsageReporter {
	if r == nil {
		return nil
	}
	return NewRedisUsageReporter(r.client, prefix, r.sink)
}

func (r *RedisUsageReporter) ReportUsage(ctx context.Context, event UsageEvent) error {
	if r == nil || r.sink == nil {
		return errors.New("usage reporter sink is nil")
	}
	if !r.Enabled() {
		return r.sink.ReportUsage(ctx, event)
	}
	if event.RequestID != "" {
		quota := NewRedisQuota(r.client, r.prefix())
		if err := quota.AdjustCommitted(ctx, UsageCommit{RequestID: event.RequestID, ActualCents: event.ActualCents}); err != nil {
			return err
		}
	}
	body, err := json.Marshal(queuedUsageEvent{
		Event:      event,
		Attempts:   0,
		EnqueuedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		return err
	}
	return r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.streamKey(),
		Values: map[string]any{"event": body},
	}).Err()
}

func (r *RedisUsageReporter) Start(ctx context.Context) {
	r.StartWorkers(ctx, defaultUsageQueueWorkers)
}

func (r *RedisUsageReporter) StartWorkers(ctx context.Context, workers int) {
	if !r.Enabled() || r.sink == nil {
		return
	}
	if workers <= 0 {
		workers = defaultUsageQueueWorkers
	}
	if err := r.ensureGroup(ctx); err != nil {
		log.Printf("gateway-lite: usage reporter group init failed: %v", err)
		return
	}
	for i := 0; i < workers; i++ {
		go r.runWorker(ctx, i)
	}
}

func (r *RedisUsageReporter) UsageQueueStats(ctx context.Context) (UsageQueueStats, error) {
	if !r.Enabled() {
		return UsageQueueStats{}, nil
	}
	var stats UsageQueueStats
	streamLength, err := r.client.XLen(ctx, r.streamKey()).Result()
	if err != nil {
		return stats, err
	}
	stats.StreamLength = streamLength
	deadCount, err := r.client.XLen(ctx, r.deadKey()).Result()
	if err != nil {
		return stats, err
	}
	stats.DeadCount = deadCount
	pending, err := r.client.XPending(ctx, r.streamKey(), r.groupName()).Result()
	if err != nil {
		if strings.Contains(err.Error(), "NOGROUP") {
			return stats, nil
		}
		return stats, err
	}
	stats.PendingCount = pending.Count
	return stats, nil
}

func (r *RedisUsageReporter) runWorker(ctx context.Context, workerID int) {
	consumer := fmt.Sprintf("worker-%d", workerID)
	for {
		prefixes := r.activePrefixes()
		if len(prefixes) == 0 {
			prefixes = []string{NormalizeRedisPrefix("")}
		}
		block := 2 * time.Second
		if len(prefixes) > 1 {
			block = 500 * time.Millisecond
		}
		handled := false
		for _, prefix := range prefixes {
			if r.readWorkerPrefix(ctx, workerID, consumer, prefix, block) {
				handled = true
			}
			if ctx.Err() != nil {
				return
			}
		}
		if !handled && len(prefixes) == 0 {
			time.Sleep(time.Second)
		}
	}
}

func (r *RedisUsageReporter) readWorkerPrefix(ctx context.Context, workerID int, consumer string, prefix string, block time.Duration) bool {
	for {
		streams, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    r.groupNameForPrefix(prefix),
			Consumer: consumer,
			Streams:  []string{r.streamKeyForPrefix(prefix), ">"},
			Count:    defaultUsageBatchSize,
			Block:    block,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				return false
			}
			if ctx.Err() != nil {
				return false
			}
			if strings.Contains(err.Error(), "NOGROUP") {
				if groupErr := r.ensureGroupForPrefix(ctx, prefix); groupErr != nil {
					log.Printf("gateway-lite: usage reporter worker=%d group init failed: %v", workerID, groupErr)
					time.Sleep(time.Second)
				}
				return false
			}
			log.Printf("gateway-lite: usage reporter worker=%d queue read failed: %v", workerID, err)
			time.Sleep(time.Second)
			return false
		}
		r.handleStreams(ctx, streams, workerID, prefix)
		return true
	}
}

func (r *RedisUsageReporter) handleStreams(ctx context.Context, streams []redis.XStream, workerID int, prefix string) {
	for _, stream := range streams {
		if len(stream.Messages) == 0 {
			continue
		}
		r.handleMessages(ctx, stream.Messages, workerID, prefix)
	}
}

func (r *RedisUsageReporter) handleMessages(ctx context.Context, messages []redis.XMessage, workerID int, prefix string) {
	items := make([]queuedUsageEvent, 0, len(messages))
	itemIDs := make([]string, 0, len(messages))
	ackIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		raw, ok := message.Values["event"].(string)
		if !ok || strings.TrimSpace(raw) == "" {
			log.Printf("gateway-lite: usage reporter worker=%d invalid stream message id=%s", workerID, message.ID)
			r.deadLetter(ctx, queuedUsageEvent{EnqueuedAt: time.Now().UnixMilli()}, "invalid_stream_message", prefix)
			ackIDs = append(ackIDs, message.ID)
			continue
		}
		var item queuedUsageEvent
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			log.Printf("gateway-lite: usage reporter worker=%d invalid event dropped id=%s: %v", workerID, message.ID, err)
			r.deadLetterRaw(ctx, raw, "invalid_json", prefix)
			ackIDs = append(ackIDs, message.ID)
			continue
		}
		items = append(items, item)
		itemIDs = append(itemIDs, message.ID)
	}
	if len(items) == 0 {
		r.ack(ctx, ackIDs, prefix)
		return
	}

	reportCtx, cancel := context.WithTimeout(ctx, defaultUsageReportTimeout)
	err := r.reportBatch(reportCtx, items)
	cancel()
	if err == nil {
		ackIDs = append(ackIDs, itemIDs...)
		r.ack(ctx, ackIDs, prefix)
		return
	}

	log.Printf("gateway-lite: usage reporter worker=%d batch report failed events=%d: %v", workerID, len(items), err)
	for i, item := range items {
		item.Attempts++
		if item.Attempts >= maxUsageReportAttempts {
			if err := r.deadLetter(ctx, item, "retry_exhausted", prefix); err == nil {
				ackIDs = append(ackIDs, itemIDs[i])
			}
			continue
		}
		if err := r.retryLater(ctx, item, prefix); err == nil {
			ackIDs = append(ackIDs, itemIDs[i])
		}
	}
	r.ack(ctx, ackIDs, prefix)
}

func (r *RedisUsageReporter) reportBatch(ctx context.Context, items []queuedUsageEvent) error {
	events := make([]UsageEvent, 0, len(items))
	for _, item := range items {
		events = append(events, item.Event)
	}
	if batchSink, ok := r.sink.(UsageBatchReportClient); ok {
		return batchSink.ReportUsageBatch(ctx, events)
	}
	for _, event := range events {
		if err := r.sink.ReportUsage(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (r *RedisUsageReporter) retryLater(ctx context.Context, item queuedUsageEvent, prefix string) error {
	delay := usageRetryDelay(item.Attempts)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	body, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.streamKeyForPrefix(prefix),
		Values: map[string]any{"event": body},
	}).Err()
}

func (r *RedisUsageReporter) ensureGroup(ctx context.Context) error {
	return r.ensureGroupForPrefix(ctx, r.prefix())
}

func (r *RedisUsageReporter) ensureGroupForPrefix(ctx context.Context, prefix string) error {
	err := r.client.XGroupCreateMkStream(ctx, r.streamKeyForPrefix(prefix), r.groupNameForPrefix(prefix), "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

func (r *RedisUsageReporter) ack(ctx context.Context, ids []string, prefix string) {
	if len(ids) == 0 {
		return
	}
	streamKey := r.streamKeyForPrefix(prefix)
	if err := r.client.XAck(ctx, streamKey, r.groupNameForPrefix(prefix), ids...).Err(); err != nil {
		log.Printf("gateway-lite: usage reporter ack failed ids=%d: %v", len(ids), err)
		return
	}
	// XACK 只会移出 consumer group（消费组）的 pending 列表，不会删除 stream（消息流）里的历史记录。
	// 成功上报后立即 XDEL，防止长时间运行时 usage stream 持续占用 Redis 内存。
	if err := r.client.XDel(ctx, streamKey, ids...).Err(); err != nil {
		log.Printf("gateway-lite: usage reporter cleanup failed ids=%d: %v", len(ids), err)
		return
	}
	// 防御性兜底：异常路径或旧版本遗留消息可能仍在 stream 中，保留少量历史便于排障。
	if err := r.client.XTrimMaxLenApprox(ctx, streamKey, defaultUsageStreamMaxLen, 0).Err(); err != nil {
		log.Printf("gateway-lite: usage reporter trim failed max_len=%d: %v", defaultUsageStreamMaxLen, err)
	}
}

func (r *RedisUsageReporter) deadLetter(ctx context.Context, item queuedUsageEvent, reason string, prefix string) error {
	body, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return r.deadLetterRaw(ctx, string(body), reason, prefix)
}

func (r *RedisUsageReporter) deadLetterRaw(ctx context.Context, raw string, reason string, prefix string) error {
	return r.client.XAdd(ctx, &redis.XAddArgs{
		Stream: r.deadKeyForPrefix(prefix),
		Values: map[string]any{
			"event":   raw,
			"reason":  reason,
			"dead_at": time.Now().UnixMilli(),
		},
	}).Err()
}

func usageRetryDelay(attempts int) time.Duration {
	if attempts <= 0 {
		return time.Second
	}
	delay := time.Duration(attempts) * time.Second
	if delay > maxUsageRetryDelay {
		return maxUsageRetryDelay
	}
	return delay
}

func (r *RedisUsageReporter) streamKey() string {
	return r.streamKeyForPrefix(r.prefix())
}

func (r *RedisUsageReporter) deadKey() string {
	return r.deadKeyForPrefix(r.prefix())
}

func (r *RedisUsageReporter) groupName() string {
	return r.groupNameForPrefix(r.prefix())
}

func (r *RedisUsageReporter) streamKeyForPrefix(prefix string) string {
	return fmt.Sprintf("%s:usage:stream", NormalizeRedisPrefix(prefix))
}

func (r *RedisUsageReporter) deadKeyForPrefix(prefix string) string {
	return fmt.Sprintf("%s:usage:dead", NormalizeRedisPrefix(prefix))
}

func (r *RedisUsageReporter) groupNameForPrefix(prefix string) string {
	return fmt.Sprintf("%s-usage-reporters", NormalizeRedisPrefix(prefix))
}

func (r *RedisUsageReporter) SetPrefix(prefix string) {
	if r != nil {
		prefix = NormalizeRedisPrefix(prefix)
		r.prefixValue.Store(prefix)
		r.prefixMu.Lock()
		if r.prefixes == nil {
			r.prefixes = map[string]struct{}{}
		}
		r.prefixes[prefix] = struct{}{}
		r.prefixMu.Unlock()
	}
}

func (r *RedisUsageReporter) prefix() string {
	if r == nil {
		return NormalizeRedisPrefix("")
	}
	if value := r.prefixValue.Load(); value != nil {
		if prefix, ok := value.(string); ok && prefix != "" {
			return prefix
		}
	}
	return NormalizeRedisPrefix("")
}

func (r *RedisUsageReporter) Prefix() string {
	return r.prefix()
}

func (r *RedisUsageReporter) activePrefixes() []string {
	if r == nil {
		return nil
	}
	current := r.prefix()
	r.prefixMu.RLock()
	out := make([]string, 0, len(r.prefixes)+1)
	seen := map[string]struct{}{}
	for prefix := range r.prefixes {
		prefix = NormalizeRedisPrefix(prefix)
		out = append(out, prefix)
		seen[prefix] = struct{}{}
	}
	r.prefixMu.RUnlock()
	if _, ok := seen[current]; !ok {
		out = append(out, current)
	}
	return out
}
