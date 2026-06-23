package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/gatewaylite"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestSubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "request-456")

	var gotClientRequestID string
	var gotRequestID string
	h := &GatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
	})

	require.Equal(t, "client-request-123", gotClientRequestID)
	require.Equal(t, "request-456", gotRequestID)
}

func TestOpenAISubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "openai-client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "openai-request-456")

	var gotClientRequestID string
	var gotRequestID string
	h := &OpenAIGatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
	})

	require.Equal(t, "openai-client-request-123", gotClientRequestID)
	require.Equal(t, "openai-request-456", gotRequestID)
}

func TestSubmitUsageRecordTaskCopiesGatewayLiteMeta(t *testing.T) {
	parent := gatewaylite.ContextWithRequestMeta(context.Background(), gatewaylite.RequestMeta{
		RequestID: "client-request-123",
		KeyID:     "key123",
		UserID:    42,
		LeaseID:   "lease1",
		Region:    "sg",
	})

	var got gatewaylite.RequestMeta
	var ok bool
	h := &GatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		got, ok = gatewaylite.RequestMetaFromContext(ctx)
	})

	require.True(t, ok)
	require.Equal(t, "client-request-123", got.RequestID)
	require.Equal(t, "key123", got.KeyID)
	require.Equal(t, int64(42), got.UserID)
	require.Equal(t, "lease1", got.LeaseID)
	require.Equal(t, "sg", got.Region)
}
