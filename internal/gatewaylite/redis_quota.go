package gatewaylite

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	reservationTTL = 24 * time.Hour
)

type RedisQuota struct {
	client      *redis.Client
	prefixValue atomic.Value
}

type ReserveRequest struct {
	RequestID          string
	KeyID              string
	UserID             int64
	Region             string
	LeaseID            string
	EstimatedCostCents int64
}

type UsageCommit struct {
	RequestID   string
	ActualCents int64
}

func NewRedisQuota(client *redis.Client, prefix string) *RedisQuota {
	quota := &RedisQuota{client: client}
	quota.SetPrefix(prefix)
	return quota
}

func (q *RedisQuota) Enabled() bool {
	return q != nil && q.client != nil
}

func (q *RedisQuota) WithPrefix(prefix string) *RedisQuota {
	if q == nil {
		return nil
	}
	return NewRedisQuota(q.client, prefix)
}

func (q *RedisQuota) EnsureLease(ctx context.Context, lease LeaseSnapshot) error {
	if !q.Enabled() {
		return nil
	}
	if lease.UserID <= 0 || lease.Region == "" || lease.LeaseID == "" {
		return errors.New("invalid lease snapshot")
	}
	key := q.leaseKey(lease.UserID, lease.Region)
	pipe := q.client.TxPipeline()
	pipe.HSet(ctx, key, map[string]any{
		"lease_id":        lease.LeaseID,
		"user_id":         lease.UserID,
		"region":          lease.Region,
		"allocated_cents": lease.AllocatedCents,
		"version":         lease.Version,
		"expires_at":      lease.ExpiresAt,
	})
	// 冷启动高并发下，多个请求可能同时从主站拿到同一个 lease。
	// 本地 Redis 的 reserved/spent 是网关热路径权威计数，后续 EnsureLease 只能初始化，
	// 不能覆盖已经发生的本地预留或扣费。
	pipe.HSetNX(ctx, key, "reserved_cents", lease.ReservedCents)
	pipe.HSetNX(ctx, key, "spent_cents", lease.SpentCents)
	if lease.ExpiresAt > 0 {
		pipe.ExpireAt(ctx, key, time.Unix(lease.ExpiresAt, 0).Add(time.Hour))
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (q *RedisQuota) LoadLease(ctx context.Context, userID int64, region string) (LeaseSnapshot, bool, error) {
	if !q.Enabled() {
		return LeaseSnapshot{}, false, nil
	}
	if userID <= 0 || region == "" {
		return LeaseSnapshot{}, false, errors.New("invalid lease lookup")
	}
	fields, err := q.client.HGetAll(ctx, q.leaseKey(userID, region)).Result()
	if err != nil {
		return LeaseSnapshot{}, false, err
	}
	if len(fields) == 0 {
		return LeaseSnapshot{}, false, nil
	}
	lease := LeaseSnapshot{
		LeaseID:        fields["lease_id"],
		UserID:         ParseInt64Field(fields, "user_id"),
		Region:         fields["region"],
		AllocatedCents: ParseInt64Field(fields, "allocated_cents"),
		ReservedCents:  ParseInt64Field(fields, "reserved_cents"),
		SpentCents:     ParseInt64Field(fields, "spent_cents"),
		Version:        ParseInt64Field(fields, "version"),
		ExpiresAt:      ParseInt64Field(fields, "expires_at"),
	}
	if lease.UserID == 0 {
		lease.UserID = userID
	}
	if lease.Region == "" {
		lease.Region = region
	}
	if lease.LeaseID == "" {
		return LeaseSnapshot{}, false, nil
	}
	if lease.Expired(time.Now()) {
		return LeaseSnapshot{}, false, nil
	}
	return lease, true, nil
}

func (q *RedisQuota) Reserve(ctx context.Context, req ReserveRequest) (bool, error) {
	if !q.Enabled() {
		return true, nil
	}
	if req.RequestID == "" || req.UserID <= 0 || req.Region == "" || req.EstimatedCostCents <= 0 {
		return false, errors.New("invalid reserve request")
	}
	leaseKey := q.leaseKey(req.UserID, req.Region)
	result, err := reserveScript.Run(ctx, q.client, []string{
		leaseKey,
		q.requestKey(req.RequestID),
	}, time.Now().Unix(), req.EstimatedCostCents, req.RequestID, req.KeyID, req.UserID, req.Region, req.LeaseID, leaseKey, int(reservationTTL.Seconds())).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (q *RedisQuota) Commit(ctx context.Context, req UsageCommit) error {
	if !q.Enabled() {
		return nil
	}
	if req.RequestID == "" {
		return errors.New("request_id is required")
	}
	_, err := commitScript.Run(ctx, q.client, []string{q.requestKey(req.RequestID)}, req.ActualCents).Int()
	return err
}

func (q *RedisQuota) AdjustCommitted(ctx context.Context, req UsageCommit) error {
	if !q.Enabled() {
		return nil
	}
	if req.RequestID == "" {
		return errors.New("request_id is required")
	}
	if req.ActualCents < 0 {
		return errors.New("actual_cents cannot be negative")
	}
	_, err := adjustCommittedScript.Run(ctx, q.client, []string{q.requestKey(req.RequestID)}, req.ActualCents).Int()
	return err
}

func (q *RedisQuota) AccumulateActualCostMicroCents(ctx context.Context, key string, deltaMicroCents, scale int64) (int64, error) {
	if !q.Enabled() {
		return 0, nil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		key = "global"
	}
	if deltaMicroCents <= 0 {
		return 0, nil
	}
	if scale <= 0 {
		scale = actualCostMicroCentsScale
	}
	return actualCostAccumulatorScript.Run(ctx, q.client, []string{q.actualCostRemainderKey(key)}, deltaMicroCents, scale, int((30 * 24 * time.Hour).Seconds())).Int64()
}

func (q *RedisQuota) Refund(ctx context.Context, requestID string) error {
	if !q.Enabled() {
		return nil
	}
	if requestID == "" {
		return errors.New("request_id is required")
	}
	_, err := refundScript.Run(ctx, q.client, []string{q.requestKey(requestID)}).Int()
	return err
}

func (q *RedisQuota) requestKey(requestID string) string {
	return fmt.Sprintf("%s:req:%s", q.prefix(), requestID)
}

func (q *RedisQuota) leaseKey(userID int64, region string) string {
	return fmt.Sprintf("%s:lease:%d:%s", q.prefix(), userID, region)
}

func (q *RedisQuota) actualCostRemainderKey(key string) string {
	return fmt.Sprintf("%s:usage:actual_cost_remainder:%s", q.prefix(), key)
}

func (q *RedisQuota) SetPrefix(prefix string) {
	if q != nil {
		q.prefixValue.Store(NormalizeRedisPrefix(prefix))
	}
}

func (q *RedisQuota) prefix() string {
	if q == nil {
		return NormalizeRedisPrefix("")
	}
	if value := q.prefixValue.Load(); value != nil {
		if prefix, ok := value.(string); ok && prefix != "" {
			return prefix
		}
	}
	return NormalizeRedisPrefix("")
}

func (q *RedisQuota) Prefix() string {
	return q.prefix()
}

func ParseInt64Field(fields map[string]string, name string) int64 {
	value, _ := strconv.ParseInt(fields[name], 10, 64)
	return value
}

var reserveScript = redis.NewScript(`
local lease_key = KEYS[1]
local req_key = KEYS[2]
local now = tonumber(ARGV[1])
local estimated = tonumber(ARGV[2])
local request_id = ARGV[3]
local key_id = ARGV[4]
local user_id = ARGV[5]
local region = ARGV[6]
local lease_id = ARGV[7]
local saved_lease_key = ARGV[8]
local ttl = tonumber(ARGV[9])

if redis.call("EXISTS", req_key) == 1 then
  return 1
end

if redis.call("EXISTS", lease_key) == 0 then
  return 0
end

local expires_at = tonumber(redis.call("HGET", lease_key, "expires_at") or "0")
if expires_at > 0 and expires_at <= now then
  return 0
end

local allocated = tonumber(redis.call("HGET", lease_key, "allocated_cents") or "0")
local reserved = tonumber(redis.call("HGET", lease_key, "reserved_cents") or "0")
local spent = tonumber(redis.call("HGET", lease_key, "spent_cents") or "0")
if allocated - reserved - spent < estimated then
  return 0
end
if not lease_id or lease_id == "" then
  lease_id = redis.call("HGET", lease_key, "lease_id") or ""
end

redis.call("HINCRBY", lease_key, "reserved_cents", estimated)
redis.call("HSET", req_key,
  "status", "reserved",
  "request_id", request_id,
  "key_id", key_id,
  "user_id", user_id,
  "region", region,
  "lease_id", lease_id,
  "lease_key", saved_lease_key,
  "estimated_cents", estimated,
  "actual_cents", 0
)
redis.call("EXPIRE", req_key, ttl)
return 1
`)

var commitScript = redis.NewScript(`
local req_key = KEYS[1]
local actual = tonumber(ARGV[1])
if redis.call("EXISTS", req_key) == 0 then
  return 0
end
local status = redis.call("HGET", req_key, "status")
if status == "committed" then
  return 1
end
if status ~= "reserved" then
  return 0
end
local user_id = redis.call("HGET", req_key, "user_id")
local region = redis.call("HGET", req_key, "region")
local estimated = tonumber(redis.call("HGET", req_key, "estimated_cents") or "0")
local corrected = redis.call("HGET", req_key, "corrected_actual_cents")
if corrected and corrected ~= "" then
  actual = tonumber(corrected)
end
local lease_key = redis.call("HGET", req_key, "lease_key")
if not lease_key or lease_key == "" then
  lease_key = string.gsub(req_key, ":req:.*$", ":lease:" .. user_id .. ":" .. region)
end
if redis.call("EXISTS", lease_key) == 1 then
  if estimated > 0 then
    redis.call("HINCRBY", lease_key, "reserved_cents", -estimated)
  end
  if actual > 0 then
    redis.call("HINCRBY", lease_key, "spent_cents", actual)
  end
end
redis.call("HSET", req_key, "status", "committed", "actual_cents", actual)
return 1
`)

var adjustCommittedScript = redis.NewScript(`
local req_key = KEYS[1]
local actual = tonumber(ARGV[1])
if redis.call("EXISTS", req_key) == 0 then
  return 0
end
local status = redis.call("HGET", req_key, "status")
if status == "reserved" then
  redis.call("HSET", req_key, "corrected_actual_cents", actual)
  return 1
end
if status ~= "committed" then
  return 0
end
local current = tonumber(redis.call("HGET", req_key, "actual_cents") or "0")
local delta = actual - current
if delta == 0 then
  return 1
end
local user_id = redis.call("HGET", req_key, "user_id")
local region = redis.call("HGET", req_key, "region")
local lease_key = redis.call("HGET", req_key, "lease_key")
if not lease_key or lease_key == "" then
  lease_key = string.gsub(req_key, ":req:.*$", ":lease:" .. user_id .. ":" .. region)
end
if redis.call("EXISTS", lease_key) == 1 then
  redis.call("HINCRBY", lease_key, "spent_cents", delta)
end
redis.call("HSET", req_key, "actual_cents", actual)
return 1
`)

var actualCostAccumulatorScript = redis.NewScript(`
local key = KEYS[1]
local delta = tonumber(ARGV[1]) or 0
local scale = tonumber(ARGV[2]) or 1000000
local ttl = tonumber(ARGV[3]) or 2592000
if delta <= 0 then
  return 0
end
local current = tonumber(redis.call("GET", key) or "0")
local total = current + delta
local whole = math.floor(total / scale)
local remainder = total - (whole * scale)
redis.call("SET", key, remainder, "EX", ttl)
return whole
`)

var refundScript = redis.NewScript(`
local req_key = KEYS[1]
if redis.call("EXISTS", req_key) == 0 then
  return 0
end
local status = redis.call("HGET", req_key, "status")
if status == "refunded" or status == "committed" then
  return 1
end
if status ~= "reserved" then
  return 0
end
local user_id = redis.call("HGET", req_key, "user_id")
local region = redis.call("HGET", req_key, "region")
local estimated = tonumber(redis.call("HGET", req_key, "estimated_cents") or "0")
local lease_key = redis.call("HGET", req_key, "lease_key")
if not lease_key or lease_key == "" then
  lease_key = string.gsub(req_key, ":req:.*$", ":lease:" .. user_id .. ":" .. region)
end
if redis.call("EXISTS", lease_key) == 1 and estimated > 0 then
  redis.call("HINCRBY", lease_key, "reserved_cents", -estimated)
end
redis.call("HSET", req_key, "status", "refunded")
return 1
`)
