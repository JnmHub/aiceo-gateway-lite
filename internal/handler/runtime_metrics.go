package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// RuntimeMetrics 汇总网关请求热路径里的本地队列/工作池指标。
// 这里只读内存状态，避免健康上报反过来给 Redis/DB 增加额外压力。
func (h *Handlers) RuntimeMetrics(_ context.Context) map[string]any {
	metrics := map[string]any{}
	if h == nil {
		return metrics
	}
	var billingCacheService *service.BillingCacheService
	var usageRecordWorkerPool *service.UsageRecordWorkerPool
	if h.Gateway != nil {
		billingCacheService = h.Gateway.billingCacheService
		usageRecordWorkerPool = h.Gateway.usageRecordWorkerPool
	}
	if h.OpenAIGateway != nil {
		if billingCacheService == nil {
			billingCacheService = h.OpenAIGateway.billingCacheService
		}
		if usageRecordWorkerPool == nil {
			usageRecordWorkerPool = h.OpenAIGateway.usageRecordWorkerPool
		}
	}
	appendBillingCacheRuntimeMetrics(metrics, billingCacheService)
	appendUsageRecordWorkerRuntimeMetrics(metrics, usageRecordWorkerPool)
	return metrics
}

func appendBillingCacheRuntimeMetrics(metrics map[string]any, svc *service.BillingCacheService) {
	if metrics == nil || svc == nil {
		return
	}
	stats := svc.RuntimeStats()
	metrics["billing_cache_queue_length"] = stats.QueueLength
	metrics["billing_cache_queue_capacity"] = stats.QueueCapacity
	metrics["billing_cache_worker_count"] = stats.WorkerCount
	metrics["billing_cache_dropped_full_total"] = stats.DroppedFullTotal
	metrics["billing_cache_dropped_closed_total"] = stats.DroppedClosedTotal
	metrics["billing_cache_dropped_full_recent"] = stats.DroppedFullRecent
	metrics["billing_cache_dropped_closed_recent"] = stats.DroppedClosedRecent
	metrics["billing_cache_stopped"] = stats.Stopped
}

func appendUsageRecordWorkerRuntimeMetrics(metrics map[string]any, pool *service.UsageRecordWorkerPool) {
	if metrics == nil || pool == nil {
		return
	}
	stats := pool.Stats()
	metrics["usage_record_max_concurrency"] = stats.MaxConcurrency
	metrics["usage_record_running_workers"] = stats.RunningWorkers
	metrics["usage_record_waiting_tasks"] = stats.WaitingTasks
	metrics["usage_record_queue_capacity"] = stats.QueueCapacity
	metrics["usage_record_queue_utilization_percent"] = stats.QueueUtilizationPct
	metrics["usage_record_overflow_policy"] = stats.OverflowPolicy
	metrics["usage_record_overflow_sample_percent"] = stats.OverflowSamplePercent
	metrics["usage_record_task_timeout_seconds"] = stats.TaskTimeoutSeconds
	metrics["usage_record_auto_scale_enabled"] = stats.AutoScaleEnabled
	metrics["usage_record_auto_scale_min_workers"] = stats.AutoScaleMinWorkers
	metrics["usage_record_auto_scale_max_workers"] = stats.AutoScaleMaxWorkers
	metrics["usage_record_auto_scale_up_queue_percent"] = stats.AutoScaleUpPercent
	metrics["usage_record_auto_scale_down_queue_percent"] = stats.AutoScaleDownPercent
	metrics["usage_record_auto_scale_up_step"] = stats.AutoScaleUpStep
	metrics["usage_record_auto_scale_down_step"] = stats.AutoScaleDownStep
	metrics["usage_record_auto_scale_interval_ms"] = stats.AutoScaleIntervalMS
	metrics["usage_record_auto_scale_cooldown_ms"] = stats.AutoScaleCooldownMS
	metrics["usage_record_submitted_tasks_total"] = stats.SubmittedTasks
	metrics["usage_record_completed_tasks_total"] = stats.CompletedTasks
	metrics["usage_record_successful_tasks_total"] = stats.SuccessfulTasks
	metrics["usage_record_failed_tasks_total"] = stats.FailedTasks
	metrics["usage_record_dropped_tasks_total"] = stats.DroppedTasks
	metrics["usage_record_dropped_queue_full_total"] = stats.DroppedQueueFull
	metrics["usage_record_dropped_pool_stopped_total"] = stats.DroppedPoolStopped
	metrics["usage_record_sync_fallback_tasks_total"] = stats.SyncFallbackTasks
}
