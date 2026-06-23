package gatewaylite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultKeySnapshotTTL = 30 * time.Second

type RedisKeyCache struct {
	client      *redis.Client
	prefixValue atomic.Value
}

func NewRedisKeyCache(client *redis.Client, prefix string) *RedisKeyCache {
	cache := &RedisKeyCache{client: client}
	cache.SetPrefix(prefix)
	return cache
}

func (c *RedisKeyCache) Enabled() bool {
	return c != nil && c.client != nil
}

func (c *RedisKeyCache) WithPrefix(prefix string) *RedisKeyCache {
	if c == nil {
		return nil
	}
	return NewRedisKeyCache(c.client, prefix)
}

func (c *RedisKeyCache) Get(ctx context.Context, keyID, region string) (*KeySnapshot, bool, error) {
	if !c.Enabled() {
		return nil, false, nil
	}
	if keyID == "" || region == "" {
		return nil, false, errors.New("invalid key cache lookup")
	}
	raw, err := c.client.Get(ctx, c.cacheKey(keyID, region)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var snapshot KeySnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		_ = c.client.Del(ctx, c.cacheKey(keyID, region)).Err()
		return nil, false, nil
	}
	return &snapshot, true, nil
}

func (c *RedisKeyCache) Set(ctx context.Context, key KeySnapshot, region string) error {
	if !c.Enabled() {
		return nil
	}
	if key.KeyID == "" || region == "" {
		return errors.New("invalid key cache snapshot")
	}
	ttl := time.Duration(key.CacheTTLSecond) * time.Second
	if ttl <= 0 {
		ttl = defaultKeySnapshotTTL
	}
	body, err := json.Marshal(key)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.cacheKey(key.KeyID, region), body, ttl).Err()
}

func (c *RedisKeyCache) DeleteRegion(ctx context.Context, region string) error {
	if !c.Enabled() {
		return nil
	}
	if region == "" {
		return errors.New("invalid key cache region")
	}
	pattern := fmt.Sprintf("%s:key:%s:*", c.prefix(), region)
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	keys := make([]string, 0)
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

func (c *RedisKeyCache) cacheKey(keyID, region string) string {
	return fmt.Sprintf("%s:key:%s:%s", c.prefix(), region, keyID)
}

func (c *RedisKeyCache) SetPrefix(prefix string) {
	if c != nil {
		c.prefixValue.Store(NormalizeRedisPrefix(prefix))
	}
}

func (c *RedisKeyCache) prefix() string {
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

func (c *RedisKeyCache) Prefix() string {
	return c.prefix()
}
