package gatewaylite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type usageSinkFunc func(context.Context, UsageEvent) error

func (f usageSinkFunc) ReportUsage(ctx context.Context, event UsageEvent) error {
	return f(ctx, event)
}

type usageBatchSink struct {
	mu      sync.Mutex
	batches [][]UsageEvent
}

type healthReportSink struct {
	mu      sync.Mutex
	reports []GatewayHealthReportRequest
}

func (s *healthReportSink) ReportGatewayHealth(_ context.Context, req GatewayHealthReportRequest) (*GatewayHealthReportResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports = append(s.reports, req)
	return &GatewayHealthReportResponse{OK: true}, nil
}

func (s *healthReportSink) lastReport() (GatewayHealthReportRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reports) == 0 {
		return GatewayHealthReportRequest{}, false
	}
	return s.reports[len(s.reports)-1], true
}

func (s *usageBatchSink) ReportUsage(context.Context, UsageEvent) error {
	return nil
}

func (s *usageBatchSink) ReportUsageBatch(_ context.Context, events []UsageEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := append([]UsageEvent(nil), events...)
	s.batches = append(s.batches, copied)
	return nil
}

func (s *usageBatchSink) batchCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.batches)
}

func (s *usageBatchSink) firstBatch() []UsageEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.batches) == 0 {
		return nil
	}
	return append([]UsageEvent(nil), s.batches[0]...)
}

func TestRedisUsageReporterQueuesAndFlushes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	reported := make(chan UsageEvent, 1)
	reporter := NewRedisUsageReporter(client, "test", usageSinkFunc(func(_ context.Context, event UsageEvent) error {
		reported <- event
		return nil
	}))
	reporter.Start(ctx)

	event := UsageEvent{
		RequestID:   "req1",
		UserID:      42,
		KeyID:       "key1",
		Region:      "sg",
		ActualCents: 1,
	}
	require.NoError(t, reporter.ReportUsage(ctx, event))

	select {
	case got := <-reported:
		require.Equal(t, event.RequestID, got.RequestID)
		require.Equal(t, event.UserID, got.UserID)
	case <-time.After(2 * time.Second):
		t.Fatal("expected queued usage report to flush")
	}

	require.Eventually(t, func() bool {
		streamLen, err := client.XLen(ctx, "test:usage:stream").Result()
		return err == nil && streamLen == 0
	}, 2*time.Second, 20*time.Millisecond)
	pending, err := client.XPending(ctx, "test:usage:stream", "test-usage-reporters").Result()
	require.NoError(t, err)
	require.EqualValues(t, 0, pending.Count)
}

func TestRedisUsageReporterUsesBatchSink(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	sink := &usageBatchSink{}
	reporter := NewRedisUsageReporter(client, "batch", sink)
	for i := 0; i < 3; i++ {
		require.NoError(t, reporter.ReportUsage(ctx, UsageEvent{
			RequestID:   "req-batch",
			UserID:      int64(40 + i),
			KeyID:       "key1",
			Region:      "sg",
			ActualCents: 1,
		}))
	}
	reporter.Start(ctx)

	require.Eventually(t, func() bool {
		return sink.batchCount() > 0
	}, 2*time.Second, 20*time.Millisecond)
	first := sink.firstBatch()
	require.Len(t, first, 3)
	require.EqualValues(t, 40, first[0].UserID)
	require.EqualValues(t, 42, first[2].UserID)
}

func TestRedisUsageReporterUsageQueueStats(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	reporter := NewRedisUsageReporter(client, "stats", usageSinkFunc(func(context.Context, UsageEvent) error { return nil }))
	require.NoError(t, reporter.ReportUsage(ctx, UsageEvent{RequestID: "req1", UserID: 42}))
	require.NoError(t, client.XAdd(ctx, &redis.XAddArgs{
		Stream: "stats:usage:dead",
		Values: map[string]any{"event": "bad", "reason": "test"},
	}).Err())

	stats, err := reporter.UsageQueueStats(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, stats.StreamLength)
	require.EqualValues(t, 1, stats.DeadCount)
}

func TestUsageQueueHealthMonitorReportsCriticalWhenDeadStreamHasEvents(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	reporter := NewRedisUsageReporter(client, "monitor", usageSinkFunc(func(context.Context, UsageEvent) error { return nil }))
	require.NoError(t, client.XAdd(ctx, &redis.XAddArgs{
		Stream: "monitor:usage:dead",
		Values: map[string]any{"event": "bad", "reason": "test"},
	}).Err())
	sink := &healthReportSink{}
	monitor := NewUsageQueueHealthMonitor(sink, reporter, "sg", "sg-1", time.Second, 1000, 1)
	monitor.reportOnce(ctx)

	report, ok := sink.lastReport()
	require.True(t, ok)
	require.Equal(t, "sg-1", report.GatewayCode)
	require.Equal(t, "sg", report.Region)
	require.Equal(t, "healthy", report.HealthStatus)
	require.Equal(t, "critical", report.UsageQueueStatus)
	require.EqualValues(t, 1, report.UsageQueueDeadCount)
}

func TestUsageQueueHealthMonitorDoesNotWarnOnStreamLengthOnly(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	reporter := NewRedisUsageReporter(client, "monitor-len", usageSinkFunc(func(context.Context, UsageEvent) error { return nil }))
	for i := 0; i < 3; i++ {
		require.NoError(t, reporter.ReportUsage(ctx, UsageEvent{RequestID: "req-len", UserID: int64(i + 1)}))
	}
	sink := &healthReportSink{}
	monitor := NewUsageQueueHealthMonitor(sink, reporter, "sg", "sg-1", time.Second, 3, 1)
	monitor.reportOnce(ctx)

	report, ok := sink.lastReport()
	require.True(t, ok)
	require.Equal(t, "healthy", report.UsageQueueStatus)
	require.EqualValues(t, 3, report.UsageQueueStreamLength)
	require.EqualValues(t, 0, report.UsageQueuePendingCount)
}

func TestRedisUsageReporterAdjustsLocalCommittedQuota(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	quota := NewRedisQuota(client, "test")
	require.NoError(t, quota.EnsureLease(ctx, LeaseSnapshot{
		LeaseID:        "lease1",
		UserID:         42,
		Region:         "sg",
		AllocatedCents: 100,
		ExpiresAt:      time.Now().Add(time.Hour).Unix(),
	}))
	ok, err := quota.Reserve(ctx, ReserveRequest{
		RequestID:          "req1",
		KeyID:              "key1",
		UserID:             42,
		Region:             "sg",
		LeaseID:            "lease1",
		EstimatedCostCents: 1,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, quota.Commit(ctx, UsageCommit{RequestID: "req1", ActualCents: 1}))

	reporter := NewRedisUsageReporter(client, "test", usageSinkFunc(func(_ context.Context, event UsageEvent) error {
		return nil
	}))
	require.NoError(t, reporter.ReportUsage(ctx, UsageEvent{RequestID: "req1", ActualCents: 7}))

	fields, err := client.HGetAll(ctx, "test:lease:42:sg").Result()
	require.NoError(t, err)
	require.EqualValues(t, 7, ParseInt64Field(fields, "spent_cents"))
}

func TestRedisUsageReporterFallsBackWithoutRedis(t *testing.T) {
	called := false
	reporter := NewRedisUsageReporter(nil, "test", usageSinkFunc(func(_ context.Context, event UsageEvent) error {
		called = event.RequestID == "req1"
		return nil
	}))

	require.NoError(t, reporter.ReportUsage(context.Background(), UsageEvent{RequestID: "req1"}))
	require.True(t, called)
}
