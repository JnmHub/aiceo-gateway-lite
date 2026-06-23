package gatewaylite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultGatewayConfigTTL = 10 * time.Minute

type GatewayConfigClient interface {
	FetchGatewayConfigSnapshot(ctx context.Context, req GatewayConfigSnapshotRequest) (*GatewayConfigSnapshotResponse, error)
}

type ConfigSnapshotApplier interface {
	ApplyGatewayConfigSnapshot(ctx context.Context, snapshot GatewayConfigSnapshot) error
}

type RedisConfigCache struct {
	client      *redis.Client
	prefixValue atomic.Value
	ttl         time.Duration
}

func NewRedisConfigCache(client *redis.Client, prefix string) *RedisConfigCache {
	cache := &RedisConfigCache{client: client, ttl: defaultGatewayConfigTTL}
	cache.SetPrefix(prefix)
	return cache
}

func (c *RedisConfigCache) Enabled() bool {
	return c != nil && c.client != nil
}

func (c *RedisConfigCache) Get(ctx context.Context) (*GatewayConfigSnapshot, bool, error) {
	if !c.Enabled() {
		return nil, false, nil
	}
	raw, err := c.client.Get(ctx, c.snapshotKey()).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var snapshot GatewayConfigSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		_ = c.client.Del(ctx, c.snapshotKey()).Err()
		return nil, false, nil
	}
	return &snapshot, true, nil
}

func (c *RedisConfigCache) Set(ctx context.Context, snapshot GatewayConfigSnapshot) error {
	if !c.Enabled() {
		return nil
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	pipe := c.client.TxPipeline()
	pipe.Set(ctx, c.snapshotKey(), body, c.ttl)
	pipe.Set(ctx, c.versionKey(), snapshot.Version, c.ttl)
	_, err = pipe.Exec(ctx)
	return err
}

func (c *RedisConfigCache) Version(ctx context.Context) (int64, error) {
	if !c.Enabled() {
		return 0, nil
	}
	version, err := c.client.Get(ctx, c.versionKey()).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return version, err
}

func (c *RedisConfigCache) InvalidationCursor(ctx context.Context) (int64, error) {
	if !c.Enabled() {
		return 0, nil
	}
	id, err := c.client.Get(ctx, c.invalidationCursorKey()).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return id, err
}

func (c *RedisConfigCache) SetInvalidationCursor(ctx context.Context, id int64) error {
	if !c.Enabled() {
		return nil
	}
	return c.client.Set(ctx, c.invalidationCursorKey(), id, 0).Err()
}

func (c *RedisConfigCache) SetPrefix(prefix string) {
	if c != nil {
		c.prefixValue.Store(NormalizeRedisPrefix(prefix))
	}
}

func (c *RedisConfigCache) prefix() string {
	if c == nil {
		return NormalizeRedisPrefix("")
	}
	if value := c.prefixValue.Load(); value != nil {
		if prefix, ok := value.(string); ok && prefix != "" {
			return prefix
		}
	}
	return NormalizeRedisPrefix("")
}

func (c *RedisConfigCache) snapshotKey() string {
	return fmt.Sprintf("%s:config:snapshot", c.prefix())
}

func (c *RedisConfigCache) versionKey() string {
	return fmt.Sprintf("%s:config:version", c.prefix())
}

func (c *RedisConfigCache) invalidationCursorKey() string {
	return fmt.Sprintf("%s:config:invalidation:last_id", c.prefix())
}

type ConfigSyncer struct {
	client        GatewayConfigClient
	cache         *RedisConfigCache
	applier       ConfigSnapshotApplier
	region        string
	regionValue   atomic.Value
	interval      time.Duration
	intervalNanos atomic.Int64
}

func NewConfigSyncer(client GatewayConfigClient, cache *RedisConfigCache, region string, interval time.Duration) *ConfigSyncer {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if region == "" {
		region = "default"
	}
	syncer := &ConfigSyncer{client: client, cache: cache, region: region, interval: interval}
	syncer.SetRegion(region)
	syncer.intervalNanos.Store(int64(interval))
	return syncer
}

func (s *ConfigSyncer) SetApplier(applier ConfigSnapshotApplier) {
	if s != nil {
		s.applier = applier
	}
}

func (s *ConfigSyncer) Start(ctx context.Context) {
	if s == nil || s.client == nil || s.cache == nil || !s.cache.Enabled() {
		return
	}
	go s.run(ctx)
}

func (s *ConfigSyncer) run(ctx context.Context) {
	s.syncOnce(ctx)
	timer := time.NewTimer(s.currentInterval())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.syncOnce(ctx)
			timer.Reset(s.currentInterval())
		}
	}
}

func (s *ConfigSyncer) SetInterval(interval time.Duration) {
	if s == nil || interval <= 0 {
		return
	}
	s.interval = interval
	s.intervalNanos.Store(int64(interval))
}

func (s *ConfigSyncer) SetRegion(region string) {
	if s == nil {
		return
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = "default"
	}
	s.regionValue.Store(region)
}

func (s *ConfigSyncer) currentRegion() string {
	if s == nil {
		return "default"
	}
	if value := s.regionValue.Load(); value != nil {
		if region, ok := value.(string); ok && region != "" {
			return region
		}
	}
	if s.region != "" {
		return s.region
	}
	return "default"
}

func (s *ConfigSyncer) currentInterval() time.Duration {
	if s == nil {
		return 30 * time.Second
	}
	if value := s.intervalNanos.Load(); value > 0 {
		return time.Duration(value)
	}
	if s.interval > 0 {
		return s.interval
	}
	return 30 * time.Second
}

func (s *ConfigSyncer) SyncOnce(ctx context.Context) {
	if s == nil {
		return
	}
	s.syncOnce(ctx)
}

func (s *ConfigSyncer) SyncFull(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if err := s.syncWithVersion(ctx, 0, true); err != nil {
		log.Printf("gateway-lite: config snapshot full sync failed: %v", err)
		return err
	}
	return nil
}

func (s *ConfigSyncer) syncOnce(ctx context.Context) {
	version, err := s.cache.Version(ctx)
	if err != nil {
		log.Printf("gateway-lite: config cache version read failed: %v", err)
	}
	if err := s.syncWithVersion(ctx, version, false); err != nil {
		log.Printf("gateway-lite: config snapshot sync failed: %v", err)
	}
}

func (s *ConfigSyncer) syncWithVersion(ctx context.Context, version int64, force bool) error {
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	resp, err := s.client.FetchGatewayConfigSnapshot(reqCtx, GatewayConfigSnapshotRequest{
		Region:       s.currentRegion(),
		SinceVersion: version,
	})
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}
	if resp == nil || !resp.OK {
		errMsg := ""
		if resp != nil {
			errMsg = resp.Error
		}
		return fmt.Errorf("rejected: %s", errMsg)
	}
	if !force && resp.Snapshot.Version <= version && len(resp.Snapshot.Accounts) == 0 && len(resp.Snapshot.Groups) == 0 {
		return nil
	}
	if err := s.cache.Set(ctx, resp.Snapshot); err != nil {
		return fmt.Errorf("cache write failed: %w", err)
	}
	if s.applier != nil {
		if err := s.applier.ApplyGatewayConfigSnapshot(ctx, resp.Snapshot); err != nil {
			return fmt.Errorf("apply failed: %w", err)
		}
	}
	log.Printf("gateway-lite: config snapshot synced version=%d accounts=%d groups=%d", resp.Snapshot.Version, len(resp.Snapshot.Accounts), len(resp.Snapshot.Groups))
	return nil
}
