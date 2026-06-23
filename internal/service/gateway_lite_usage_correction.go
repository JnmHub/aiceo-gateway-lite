package service

import (
	"context"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/internal/gatewaylite"
)

func reportGatewayLiteUsageCorrection(ctx context.Context, usageLog *UsageLog) {
	if usageLog == nil {
		return
	}
	if _, ok := gatewaylite.RequestMetaFromContext(ctx); !ok {
		return
	}
	event := gatewaylite.UsageEvent{
		Protocol:         "openai_compatible",
		Model:            usageLog.Model,
		Path:             optionalStringValue(usageLog.InboundEndpoint),
		TokensIn:         int64(usageLog.InputTokens),
		TokensOut:        int64(usageLog.OutputTokens),
		CacheReadTokens:  int64(usageLog.CacheReadTokens),
		CacheWriteTokens: int64(usageLog.CacheCreationTokens),
		ActualCents:      gatewaylite.ActualCostToCentsForContext(ctx, usageLog.ActualCost),
	}
	if err := gatewaylite.ReportUsageCorrection(ctx, event); err != nil {
		slog.Warn("gateway-lite usage correction report failed", "request_id", usageLog.RequestID, "error", err)
	}
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
