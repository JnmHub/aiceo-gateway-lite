package gatewaylite

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type contextKey string

const requestMetaContextKey contextKey = "gateway_lite_request_meta"

type RequestMeta struct {
	RequestID string
	KeyID     string
	UserID    int64
	LeaseID   string
	Region    string
	GatewayID string
}

var defaultUsageReporter atomic.Value
var defaultControlPlaneClientRef atomic.Value
var defaultConfigSyncer atomic.Value
var defaultRuntimeHealthMonitor atomic.Value
var defaultRuntimeConfigRef atomic.Value
var defaultRuntimeHealthSettings atomic.Value
var defaultRedisQuota atomic.Value
var defaultRedisKeyCache atomic.Value
var defaultRedisConfigCache atomic.Value
var defaultCacheInvalidationSyncer atomic.Value

const actualCostMicroCentsScale int64 = 1_000_000

var actualCostLocalRemainders = struct {
	sync.Mutex
	values map[string]int64
}{values: map[string]int64{}}

func ContextWithRequestMeta(ctx context.Context, meta RequestMeta) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if meta.GatewayID == "" {
		meta.GatewayID = meta.Region
	}
	return context.WithValue(ctx, requestMetaContextKey, meta)
}

func RequestMetaFromContext(ctx context.Context) (RequestMeta, bool) {
	if ctx == nil {
		return RequestMeta{}, false
	}
	meta, ok := ctx.Value(requestMetaContextKey).(RequestMeta)
	if !ok || strings.TrimSpace(meta.RequestID) == "" || meta.UserID <= 0 {
		return RequestMeta{}, false
	}
	return meta, true
}

func SetDefaultUsageReporter(reporter UsageReportClient) {
	defaultUsageReporter.Store(reporter)
}

func DefaultUsageReporter() UsageReportClient {
	value := defaultUsageReporter.Load()
	if value == nil {
		return nil
	}
	reporter, _ := value.(UsageReportClient)
	return reporter
}

type ControlPlaneClientRef struct {
	client atomic.Pointer[ControlPlaneClient]
}

func NewControlPlaneClientRef(client *ControlPlaneClient) *ControlPlaneClientRef {
	ref := &ControlPlaneClientRef{}
	ref.Set(client)
	return ref
}

func (r *ControlPlaneClientRef) Set(client *ControlPlaneClient) {
	if r != nil && client != nil {
		r.client.Store(client)
	}
}

func (r *ControlPlaneClientRef) Configured() bool {
	if r == nil {
		return false
	}
	return r.client.Load() != nil
}

func (r *ControlPlaneClientRef) current() (*ControlPlaneClient, error) {
	if r == nil {
		return nil, errors.New("control plane client is not configured")
	}
	client := r.client.Load()
	if client == nil {
		return nil, errors.New("control plane client is not configured")
	}
	return client, nil
}

func (r *ControlPlaneClientRef) ResolveKey(ctx context.Context, req ResolveKeyRequest) (*ResolveKeyResponse, error) {
	client, err := r.current()
	if err != nil {
		return nil, err
	}
	return client.ResolveKey(ctx, req)
}

func (r *ControlPlaneClientRef) AcquireLease(ctx context.Context, req AcquireLeaseRequest) (*AcquireLeaseResponse, error) {
	client, err := r.current()
	if err != nil {
		return nil, err
	}
	return client.AcquireLease(ctx, req)
}

func (r *ControlPlaneClientRef) RefillLease(ctx context.Context, req AcquireLeaseRequest) (*AcquireLeaseResponse, error) {
	client, err := r.current()
	if err != nil {
		return nil, err
	}
	return client.RefillLease(ctx, req)
}

func (r *ControlPlaneClientRef) RebalanceLease(ctx context.Context, req RebalanceLeaseRequest) (*RebalanceLeaseResponse, error) {
	client, err := r.current()
	if err != nil {
		return nil, err
	}
	return client.RebalanceLease(ctx, req)
}

func (r *ControlPlaneClientRef) ReportUsage(ctx context.Context, event UsageEvent) error {
	client, err := r.current()
	if err != nil {
		return err
	}
	return client.ReportUsage(ctx, event)
}

func (r *ControlPlaneClientRef) ReportUsageBatch(ctx context.Context, events []UsageEvent) error {
	client, err := r.current()
	if err != nil {
		return err
	}
	return client.ReportUsageBatch(ctx, events)
}

func (r *ControlPlaneClientRef) ReportGatewayHealth(ctx context.Context, req GatewayHealthReportRequest) (*GatewayHealthReportResponse, error) {
	client, err := r.current()
	if err != nil {
		return nil, err
	}
	return client.ReportGatewayHealth(ctx, req)
}

func (r *ControlPlaneClientRef) ProbeControlPlaneHealth(ctx context.Context) (time.Duration, error) {
	client, err := r.current()
	if err != nil {
		return 0, err
	}
	return client.ProbeControlPlaneHealth(ctx)
}

func (r *ControlPlaneClientRef) FetchGatewayConfigSnapshot(ctx context.Context, req GatewayConfigSnapshotRequest) (*GatewayConfigSnapshotResponse, error) {
	client, err := r.current()
	if err != nil {
		return nil, err
	}
	return client.FetchGatewayConfigSnapshot(ctx, req)
}

func (r *ControlPlaneClientRef) FetchCacheInvalidations(ctx context.Context, req CacheInvalidationRequest) (*CacheInvalidationResponse, error) {
	client, err := r.current()
	if err != nil {
		return nil, err
	}
	return client.FetchCacheInvalidations(ctx, req)
}

func SetDefaultControlPlaneClientRef(ref *ControlPlaneClientRef) {
	defaultControlPlaneClientRef.Store(ref)
}

func DefaultControlPlaneClientRef() *ControlPlaneClientRef {
	value := defaultControlPlaneClientRef.Load()
	if value == nil {
		return nil
	}
	ref, _ := value.(*ControlPlaneClientRef)
	return ref
}

func SetDefaultConfigSyncer(syncer *ConfigSyncer) {
	defaultConfigSyncer.Store(syncer)
}

func DefaultConfigSyncer() *ConfigSyncer {
	value := defaultConfigSyncer.Load()
	if value == nil {
		return nil
	}
	syncer, _ := value.(*ConfigSyncer)
	return syncer
}

func SetDefaultRuntimeHealthMonitor(monitor *RuntimeHealthMonitor) {
	defaultRuntimeHealthMonitor.Store(monitor)
}

func DefaultRuntimeHealthMonitor() *RuntimeHealthMonitor {
	value := defaultRuntimeHealthMonitor.Load()
	if value == nil {
		return nil
	}
	monitor, _ := value.(*RuntimeHealthMonitor)
	return monitor
}

func SetDefaultRuntimeConfigRef(ref *RuntimeConfigRef) {
	defaultRuntimeConfigRef.Store(ref)
}

func DefaultRuntimeConfigRef() *RuntimeConfigRef {
	value := defaultRuntimeConfigRef.Load()
	if value == nil {
		return nil
	}
	ref, _ := value.(*RuntimeConfigRef)
	return ref
}

func SetDefaultRuntimeHealthSettings(settings *RuntimeHealthSettings) {
	defaultRuntimeHealthSettings.Store(settings)
}

func DefaultRuntimeHealthSettings() *RuntimeHealthSettings {
	value := defaultRuntimeHealthSettings.Load()
	if value == nil {
		return nil
	}
	settings, _ := value.(*RuntimeHealthSettings)
	return settings
}

func SetDefaultRedisQuota(quota *RedisQuota) {
	defaultRedisQuota.Store(quota)
}

func DefaultRedisQuota() *RedisQuota {
	value := defaultRedisQuota.Load()
	if value == nil {
		return nil
	}
	quota, _ := value.(*RedisQuota)
	return quota
}

func SetDefaultRedisKeyCache(cache *RedisKeyCache) {
	defaultRedisKeyCache.Store(cache)
}

func DefaultRedisKeyCache() *RedisKeyCache {
	value := defaultRedisKeyCache.Load()
	if value == nil {
		return nil
	}
	cache, _ := value.(*RedisKeyCache)
	return cache
}

func SetDefaultRedisConfigCache(cache *RedisConfigCache) {
	defaultRedisConfigCache.Store(cache)
}

func DefaultRedisConfigCache() *RedisConfigCache {
	value := defaultRedisConfigCache.Load()
	if value == nil {
		return nil
	}
	cache, _ := value.(*RedisConfigCache)
	return cache
}

func SetDefaultCacheInvalidationSyncer(syncer *CacheInvalidationSyncer) {
	defaultCacheInvalidationSyncer.Store(syncer)
}

func DefaultCacheInvalidationSyncer() *CacheInvalidationSyncer {
	value := defaultCacheInvalidationSyncer.Load()
	if value == nil {
		return nil
	}
	syncer, _ := value.(*CacheInvalidationSyncer)
	return syncer
}

func ActualCostToCents(cost float64) int64 {
	if cost <= 0 {
		return 0
	}
	return int64(math.Ceil(cost * 100))
}

func ActualCostToCentsForContext(ctx context.Context, cost float64) int64 {
	if cost <= 0 {
		return 0
	}
	deltaMicroCents := int64(math.Round(cost * 100 * float64(actualCostMicroCentsScale)))
	if deltaMicroCents <= 0 {
		return 0
	}
	key := actualCostAccumulatorKey(ctx)
	if quota := DefaultRedisQuota(); quota != nil && quota.Enabled() {
		if cents, err := quota.AccumulateActualCostMicroCents(ctx, key, deltaMicroCents, actualCostMicroCentsScale); err == nil {
			return cents
		}
	}
	return accumulateActualCostMicroCentsLocal(key, deltaMicroCents, actualCostMicroCentsScale)
}

func actualCostAccumulatorKey(ctx context.Context) string {
	meta, ok := RequestMetaFromContext(ctx)
	if !ok {
		return "global"
	}
	parts := []string{
		strings.TrimSpace(meta.Region),
		strings.TrimSpace(meta.KeyID),
	}
	if meta.UserID > 0 {
		parts = append(parts, strconvFormatInt(meta.UserID))
	}
	return strings.Join(parts, ":")
}

func accumulateActualCostMicroCentsLocal(key string, deltaMicroCents, scale int64) int64 {
	if key == "" {
		key = "global"
	}
	if scale <= 0 || deltaMicroCents <= 0 {
		return 0
	}
	actualCostLocalRemainders.Lock()
	defer actualCostLocalRemainders.Unlock()
	total := actualCostLocalRemainders.values[key] + deltaMicroCents
	cents := total / scale
	actualCostLocalRemainders.values[key] = total % scale
	return cents
}

func strconvFormatInt(value int64) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	negative := value < 0
	if negative {
		value = -value
	}
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func ReportUsageCorrection(ctx context.Context, event UsageEvent) error {
	reporter := DefaultUsageReporter()
	if reporter == nil {
		return nil
	}
	meta, ok := RequestMetaFromContext(ctx)
	if !ok {
		return nil
	}
	if event.RequestID == "" {
		event.RequestID = meta.RequestID
	}
	if event.UserID == 0 {
		event.UserID = meta.UserID
	}
	if event.KeyID == "" {
		event.KeyID = meta.KeyID
	}
	if event.LeaseID == "" {
		event.LeaseID = meta.LeaseID
	}
	if event.Region == "" {
		event.Region = meta.Region
	}
	if event.GatewayID == "" {
		event.GatewayID = meta.GatewayID
	}
	return reporter.ReportUsage(ctx, event)
}
