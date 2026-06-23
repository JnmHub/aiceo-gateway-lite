package gatewaylite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type runtimeHealthSink struct {
	mu          sync.Mutex
	reports     []GatewayHealthReportRequest
	latency     time.Duration
	zeroLatency bool
	err         error
	reportErr   error
}

type runtimeAvailableModelsProviderFunc func(context.Context) []string

func (f runtimeAvailableModelsProviderFunc) AvailableModels(ctx context.Context) []string {
	return f(ctx)
}

type runtimeMetricsProviderFunc func(context.Context) map[string]any

func (f runtimeMetricsProviderFunc) RuntimeMetrics(ctx context.Context) map[string]any {
	return f(ctx)
}

func (s *runtimeHealthSink) ProbeControlPlaneHealth(context.Context) (time.Duration, error) {
	if s.zeroLatency {
		return 0, s.err
	}
	if s.latency == 0 {
		s.latency = 42 * time.Millisecond
	}
	return s.latency, s.err
}

func (s *runtimeHealthSink) ReportGatewayHealth(_ context.Context, req GatewayHealthReportRequest) (*GatewayHealthReportResponse, error) {
	if s.reportErr != nil {
		return nil, s.reportErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports = append(s.reports, req)
	return &GatewayHealthReportResponse{OK: true}, nil
}

func (s *runtimeHealthSink) lastReport() (GatewayHealthReportRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reports) == 0 {
		return GatewayHealthReportRequest{}, false
	}
	return s.reports[len(s.reports)-1], true
}

func TestRuntimeHealthMonitorReportsLatencyAndOnlineUsers(t *testing.T) {
	now := time.Now()
	stats := NewRuntimeStats()
	stats.RecordUser(42, now)
	stats.RecordUser(43, now.Add(-10*time.Minute))
	sink := &runtimeHealthSink{latency: 57 * time.Millisecond}

	monitor := NewRuntimeHealthMonitor(sink, stats, "sg", "sg-1", time.Second, 5*time.Minute)
	monitor.reportOnce(context.Background())

	report, ok := sink.lastReport()
	require.True(t, ok)
	require.Equal(t, "sg-1", report.GatewayCode)
	require.Equal(t, "sg", report.Region)
	require.Equal(t, "healthy", report.HealthStatus)
	require.Equal(t, 57, report.LatencyMS)
	require.Equal(t, 1, report.OnlineUsers)
	require.Equal(t, "gateway-lite-runtime-health-monitor", report.Metadata["source"])
}

func TestRuntimeHealthMonitorRoundsSubMillisecondLatencyUp(t *testing.T) {
	stats := NewRuntimeStats()
	sink := &runtimeHealthSink{latency: 500 * time.Microsecond}

	monitor := NewRuntimeHealthMonitor(sink, stats, "hk", "openai-hk-t1", time.Second, 5*time.Minute)
	monitor.reportOnce(context.Background())

	report, ok := sink.lastReport()
	require.True(t, ok)
	require.Equal(t, 1, report.LatencyMS)
}

func TestRuntimeHealthMonitorReportsAtLeastOneMillisecondOnSuccessfulZeroLatencyProbe(t *testing.T) {
	stats := NewRuntimeStats()
	sink := &runtimeHealthSink{zeroLatency: true}

	monitor := NewRuntimeHealthMonitor(sink, stats, "hk", "openai-hk-t1", time.Second, 5*time.Minute)
	monitor.reportOnce(context.Background())

	report, ok := sink.lastReport()
	require.True(t, ok)
	require.Equal(t, "healthy", report.HealthStatus)
	require.Equal(t, 1, report.LatencyMS)
}

func TestRuntimeHealthMonitorUsesDynamicIntervalSource(t *testing.T) {
	settings := NewRuntimeHealthSettings(60 * time.Second)
	monitor := NewRuntimeHealthMonitor(&runtimeHealthSink{}, NewRuntimeStats(), "sg", "sg-1", 15*time.Second, 5*time.Minute).
		WithIntervalSource(settings)

	require.Equal(t, 60*time.Second, monitor.currentInterval())

	settings.SetInterval(180 * time.Second)
	require.Equal(t, 180*time.Second, monitor.currentInterval())
}

func TestRuntimeHealthMonitorReportsConfiguredAvailableModels(t *testing.T) {
	stats := NewRuntimeStats()
	sink := &runtimeHealthSink{latency: 10 * time.Millisecond}

	monitor := NewRuntimeHealthMonitor(sink, stats, "sg", "sg-1", time.Second, 5*time.Minute).
		WithAvailableModels([]string{" gpt-4o ", "claude-sonnet-4", "gpt-4o", ""})
	monitor.reportOnce(context.Background())

	report, ok := sink.lastReport()
	require.True(t, ok)
	require.Equal(t, []string{"claude-sonnet-4", "gpt-4o"}, report.AvailableModels)
}

func TestRuntimeHealthMonitorMergesDynamicAvailableModels(t *testing.T) {
	stats := NewRuntimeStats()
	sink := &runtimeHealthSink{latency: 10 * time.Millisecond}

	monitor := NewRuntimeHealthMonitor(sink, stats, "sg", "sg-1", time.Second, 5*time.Minute).
		WithAvailableModels([]string{"gpt-5.5", "mimo-v2.5"}).
		WithAvailableModelsProvider(runtimeAvailableModelsProviderFunc(func(context.Context) []string {
			return []string{"mimo-v2-flash", "mimo-v2.5", " "}
		}))
	monitor.reportOnce(context.Background())

	report, ok := sink.lastReport()
	require.True(t, ok)
	require.Equal(t, []string{"gpt-5.5", "mimo-v2-flash", "mimo-v2.5"}, report.AvailableModels)
}

func TestRuntimeHealthMonitorReportFailureDoesNotStopNextReport(t *testing.T) {
	stats := NewRuntimeStats()
	sink := &runtimeHealthSink{
		latency:   10 * time.Millisecond,
		reportErr: errors.New("temporary report failure"),
	}

	monitor := NewRuntimeHealthMonitor(sink, stats, "sg", "sg-1", time.Second, 5*time.Minute).
		WithAvailableModels([]string{"gpt-4o"})
	monitor.reportOnce(context.Background())

	_, ok := sink.lastReport()
	require.False(t, ok)

	sink.reportErr = nil
	monitor.reportOnce(context.Background())

	report, ok := sink.lastReport()
	require.True(t, ok)
	require.Equal(t, []string{"gpt-4o"}, report.AvailableModels)
}

func TestRuntimeHealthMonitorMergesRuntimeMetrics(t *testing.T) {
	stats := NewRuntimeStats()
	sink := &runtimeHealthSink{latency: 10 * time.Millisecond}

	monitor := NewRuntimeHealthMonitor(sink, stats, "sg", "sg-1", time.Second, 5*time.Minute).
		WithRuntimeMetricsProvider(runtimeMetricsProviderFunc(func(context.Context) map[string]any {
			return map[string]any{
				" usage_record_waiting_tasks ": uint64(3),
				"billing_cache_queue_length":   7,
				"":                             "ignored",
			}
		}))
	monitor.reportOnce(context.Background())

	report, ok := sink.lastReport()
	require.True(t, ok)
	require.EqualValues(t, 3, report.Metadata["usage_record_waiting_tasks"])
	require.EqualValues(t, 7, report.Metadata["billing_cache_queue_length"])
	require.NotContains(t, report.Metadata, "")
}
