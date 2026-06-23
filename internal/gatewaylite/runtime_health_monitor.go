package gatewaylite

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type RuntimeHealthClient interface {
	ProbeControlPlaneHealth(ctx context.Context) (time.Duration, error)
	ReportGatewayHealth(ctx context.Context, req GatewayHealthReportRequest) (*GatewayHealthReportResponse, error)
}

type RuntimeHealthMonitor struct {
	client                RuntimeHealthClient
	stats                 *RuntimeStats
	usageReporter         *RedisUsageReporter
	intervalSource        RuntimeHealthIntervalSource
	mu                    sync.RWMutex
	region                string
	gatewayCode           string
	interval              time.Duration
	activeWindow          time.Duration
	usagePendingThreshold int64
	usageDeadThreshold    int64
	availableModels       []string
	availableModelsSource RuntimeAvailableModelsProvider
	modelPriceProvider    RuntimeModelPriceProvider
	metricsProviders      []RuntimeMetricsProvider
}

type RuntimeModelPriceProvider interface {
	GatewayModelPrices(models []string, gatewayCode string) []GatewayModelPrice
}

type RuntimeMetricsProvider interface {
	RuntimeMetrics(ctx context.Context) map[string]any
}

type RuntimeAvailableModelsProvider interface {
	AvailableModels(ctx context.Context) []string
}

type RuntimeHealthIntervalSource interface {
	RuntimeHealthInterval() time.Duration
}

type RuntimeHealthSettings struct {
	intervalSeconds atomic.Int64
}

func NewRuntimeHealthSettings(interval time.Duration) *RuntimeHealthSettings {
	settings := &RuntimeHealthSettings{}
	settings.SetInterval(interval)
	return settings
}

func (s *RuntimeHealthSettings) SetInterval(interval time.Duration) {
	if s == nil {
		return
	}
	if interval <= 0 {
		interval = 15 * time.Second
	}
	s.intervalSeconds.Store(int64(interval.Seconds()))
}

func (s *RuntimeHealthSettings) RuntimeHealthInterval() time.Duration {
	if s == nil {
		return 0
	}
	seconds := s.intervalSeconds.Load()
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func NewRuntimeHealthMonitor(client RuntimeHealthClient, stats *RuntimeStats, region, gatewayCode string, interval, activeWindow time.Duration) *RuntimeHealthMonitor {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if activeWindow <= 0 {
		activeWindow = 5 * time.Minute
	}
	return &RuntimeHealthMonitor{
		client:                client,
		stats:                 stats,
		region:                region,
		gatewayCode:           gatewayCode,
		interval:              interval,
		activeWindow:          activeWindow,
		usagePendingThreshold: 1000,
		usageDeadThreshold:    1,
	}
}

func (m *RuntimeHealthMonitor) WithIntervalSource(source RuntimeHealthIntervalSource) *RuntimeHealthMonitor {
	if m == nil {
		return nil
	}
	m.intervalSource = source
	return m
}

func (m *RuntimeHealthMonitor) WithUsageQueue(reporter *RedisUsageReporter, pendingThreshold, deadThreshold int64) *RuntimeHealthMonitor {
	if m == nil {
		return nil
	}
	if pendingThreshold <= 0 {
		pendingThreshold = 1000
	}
	if deadThreshold <= 0 {
		deadThreshold = 1
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usageReporter = reporter
	m.usagePendingThreshold = pendingThreshold
	m.usageDeadThreshold = deadThreshold
	return m
}

func (m *RuntimeHealthMonitor) WithAvailableModels(models []string) *RuntimeHealthMonitor {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.availableModels = normalizeAvailableModels(models)
	return m
}

func (m *RuntimeHealthMonitor) WithAvailableModelsProvider(provider RuntimeAvailableModelsProvider) *RuntimeHealthMonitor {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.availableModelsSource = provider
	return m
}

func (m *RuntimeHealthMonitor) WithModelPriceProvider(provider RuntimeModelPriceProvider) *RuntimeHealthMonitor {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modelPriceProvider = provider
	return m
}

func (m *RuntimeHealthMonitor) WithRuntimeMetricsProvider(provider RuntimeMetricsProvider) *RuntimeHealthMonitor {
	if m == nil || provider == nil {
		return m
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metricsProviders = append(m.metricsProviders, provider)
	return m
}

func (m *RuntimeHealthMonitor) SetRuntimeConfig(region, gatewayCode string, activeWindow time.Duration, pendingThreshold, deadThreshold int64) {
	if m == nil {
		return
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = "default"
	}
	gatewayCode = strings.TrimSpace(gatewayCode)
	if gatewayCode == "" {
		gatewayCode = region
	}
	if activeWindow <= 0 {
		activeWindow = 5 * time.Minute
	}
	if pendingThreshold <= 0 {
		pendingThreshold = 1000
	}
	if deadThreshold <= 0 {
		deadThreshold = 1
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.region = region
	m.gatewayCode = gatewayCode
	m.activeWindow = activeWindow
	m.usagePendingThreshold = pendingThreshold
	m.usageDeadThreshold = deadThreshold
}

func (m *RuntimeHealthMonitor) Start(ctx context.Context) {
	if m == nil || m.client == nil {
		return
	}
	go m.run(ctx)
}

func (m *RuntimeHealthMonitor) ReportOnce(ctx context.Context) {
	if m == nil || m.client == nil {
		return
	}
	m.reportOnce(ctx)
}

func (m *RuntimeHealthMonitor) run(ctx context.Context) {
	m.reportOnce(ctx)
	timer := time.NewTimer(m.currentInterval())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			m.reportOnce(ctx)
			timer.Reset(m.currentInterval())
		}
	}
}

func (m *RuntimeHealthMonitor) currentInterval() time.Duration {
	if m != nil && m.intervalSource != nil {
		if interval := m.intervalSource.RuntimeHealthInterval(); interval > 0 {
			return interval
		}
	}
	if m == nil || m.interval <= 0 {
		return 15 * time.Second
	}
	return m.interval
}

func (m *RuntimeHealthMonitor) reportOnce(ctx context.Context) {
	m.mu.RLock()
	region := m.region
	gatewayCode := m.gatewayCode
	activeWindow := m.activeWindow
	availableModels := append([]string{}, m.availableModels...)
	availableModelsSource := m.availableModelsSource
	modelPriceProvider := m.modelPriceProvider
	metricsProviders := append([]RuntimeMetricsProvider{}, m.metricsProviders...)
	m.mu.RUnlock()

	probeCtx, probeCancel := context.WithTimeout(ctx, defaultUsageReportTimeout)
	latency, err := m.client.ProbeControlPlaneHealth(probeCtx)
	probeCancel()

	status := "healthy"
	message := ""
	latencyMS := latencyMilliseconds(latency)
	if latencyMS == 0 {
		latencyMS = 1
	}
	if err != nil {
		status = "warning"
		message = "control plane health probe failed"
		latencyMS = 0
	}
	if availableModelsSource != nil {
		modelsCtx, modelsCancel := context.WithTimeout(ctx, defaultUsageReportTimeout)
		dynamicModels := availableModelsSource.AvailableModels(modelsCtx)
		modelsCancel()
		// 静态配置保留为兜底，动态模型来自本地可调度账号池，避免新增账号后主站仍只看到旧环境变量。
		availableModels = normalizeAvailableModels(append(availableModels, dynamicModels...))
	}
	req := GatewayHealthReportRequest{
		GatewayCode:     gatewayCode,
		Region:          region,
		HealthStatus:    status,
		LatencyMS:       latencyMS,
		OnlineUsers:     m.stats.OnlineUsers(time.Now(), activeWindow),
		AvailableModels: availableModels,
		Message:         message,
		Metadata: map[string]any{
			"source":                "gateway-lite-runtime-health-monitor",
			"probe_target":          "control-plane-health",
			"active_window_seconds": int64(activeWindow.Seconds()),
		},
	}
	if modelPriceProvider != nil && len(availableModels) > 0 {
		req.ModelPrices = modelPriceProvider.GatewayModelPrices(availableModels, gatewayCode)
	}
	m.fillUsageQueueHealth(ctx, &req)
	m.fillRuntimeMetrics(ctx, &req, metricsProviders)
	reportCtx, reportCancel := context.WithTimeout(ctx, defaultUsageReportTimeout)
	defer reportCancel()
	resp, reportErr := m.client.ReportGatewayHealth(reportCtx, req)
	if reportErr != nil {
		log.Printf("gateway-lite: runtime health report failed: %v", reportErr)
		return
	}
	if resp != nil && !resp.OK {
		log.Printf("gateway-lite: runtime health report rejected: %s", resp.Error)
	}
}

func normalizeAvailableModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	sort.Strings(out)
	return out
}

func latencyMilliseconds(latency time.Duration) int {
	if latency <= 0 {
		return 0
	}
	ms := int(latency.Milliseconds())
	if ms <= 0 {
		return 1
	}
	return ms
}

func (m *RuntimeHealthMonitor) fillUsageQueueHealth(ctx context.Context, req *GatewayHealthReportRequest) {
	if m == nil || req == nil {
		return
	}
	m.mu.RLock()
	usageReporter := m.usageReporter
	pendingThreshold := m.usagePendingThreshold
	deadThreshold := m.usageDeadThreshold
	m.mu.RUnlock()
	if usageReporter == nil || !usageReporter.Enabled() {
		return
	}
	queueCtx, cancel := context.WithTimeout(ctx, defaultUsageReportTimeout)
	stats, err := usageReporter.UsageQueueStats(queueCtx)
	cancel()

	status := "healthy"
	message := ""
	if err != nil {
		status = "warning"
		message = "usage queue stats unavailable"
	} else if stats.DeadCount >= deadThreshold {
		status = "critical"
		message = "usage queue dead stream has events"
	} else if stats.PendingCount >= pendingThreshold {
		status = "warning"
		message = "usage queue backlog exceeds threshold"
	}
	req.UsageQueueStatus = status
	req.UsageQueueStreamLength = stats.StreamLength
	req.UsageQueuePendingCount = stats.PendingCount
	req.UsageQueueDeadCount = stats.DeadCount
	if req.Message == "" && message != "" {
		req.Message = message
	}
	req.Metadata["usage_queue_pending_max"] = pendingThreshold
	req.Metadata["usage_queue_dead_max"] = deadThreshold
}

func (m *RuntimeHealthMonitor) fillRuntimeMetrics(ctx context.Context, req *GatewayHealthReportRequest, providers []RuntimeMetricsProvider) {
	if req == nil || len(providers) == 0 {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		metrics := provider.RuntimeMetrics(ctx)
		for key, value := range metrics {
			key = strings.TrimSpace(key)
			if key == "" || value == nil {
				continue
			}
			req.Metadata[key] = value
		}
	}
}
