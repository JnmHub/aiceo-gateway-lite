package server

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gatewaylite"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayLiteSchedulerConfigApplier(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	cache := repository.NewSchedulerCache(client)
	applier := newGatewayLiteSchedulerConfigApplier(cache)

	require.NoError(t, applier.ApplyGatewayConfigSnapshot(ctx, gatewaylite.GatewayConfigSnapshot{
		Version: 1,
		Groups: []gatewaylite.GatewayGroupSnapshot{{
			ID:                    7,
			Name:                  "openai",
			Platform:              service.PlatformOpenAI,
			Status:                service.StatusActive,
			RateMultiplier:        1,
			RequirePrivacySet:     true,
			AllowImageGeneration:  true,
			AllowMessagesDispatch: true,
			ModelsListConfig: gatewaylite.GroupModelsListConfig{
				Enabled: true,
				Models:  []string{"gpt-5-mini"},
			},
			Config: map[string]any{
				"rpm_limit":            88,
				"default_mapped_model": "gpt-5-mini",
			},
		}},
		Accounts: []gatewaylite.GatewayAccountSnapshot{{
			ID:          1001,
			Name:        "openai-primary",
			Platform:    service.PlatformOpenAI,
			Type:        "api_key",
			Status:      service.StatusActive,
			Schedulable: true,
			Concurrency: 5,
			Priority:    100,
			Credentials: map[string]any{"api_key": "secret"},
			GroupIDs:    []int64{7},
		}},
	}))

	got, hit, err := cache.GetSnapshot(ctx, service.SchedulerBucket{
		GroupID:  7,
		Platform: service.PlatformOpenAI,
		Mode:     service.SchedulerModeSingle,
	})
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, got, 1)
	require.EqualValues(t, 1001, got[0].ID)
	require.Equal(t, service.PlatformOpenAI, got[0].Platform)
	require.Equal(t, []int64{7}, got[0].GroupIDs)
	require.Len(t, got[0].Groups, 1)
	require.True(t, got[0].Groups[0].RequirePrivacySet)
	require.True(t, got[0].Groups[0].AllowMessagesDispatch)
	require.Equal(t, 88, got[0].Groups[0].RPMLimit)
	require.Equal(t, "gpt-5-mini", got[0].Groups[0].DefaultMappedModel)
	require.True(t, got[0].Groups[0].ModelsListConfig.Enabled)

	account, err := cache.GetAccount(ctx, 1001)
	require.NoError(t, err)
	require.NotNil(t, account)
	require.Equal(t, service.AccountTypeAPIKey, account.Type)
	require.Equal(t, "secret", account.Credentials["api_key"])

	groupCache, ok := cache.(service.SchedulerGroupCache)
	require.True(t, ok)
	group, err := groupCache.GetGroup(ctx, 7)
	require.NoError(t, err)
	require.NotNil(t, group)
	require.True(t, group.RequirePrivacySet)
	require.Equal(t, 88, group.RPMLimit)
}

func TestGatewayLiteSchedulerConfigApplierUpdatesRuntimeHealthInterval(t *testing.T) {
	settings := gatewaylite.NewRuntimeHealthSettings(15 * time.Second)
	applier := newGatewayLiteSchedulerConfigApplier(nil).
		WithRuntimeHealthSettings("openai-sg-t1", "sg", settings)

	applier.applyRuntimeHealthSettings(gatewaylite.GatewayConfigSnapshot{
		GatewayNodes: []gatewaylite.GatewayNodeSnapshot{
			{Code: "openai-hk-t1", Region: "hk", Metadata: map[string]any{"health_probe_interval_seconds": 180}},
			{Code: "openai-sg-t1", Region: "sg", Metadata: map[string]any{"health_probe_interval_seconds": 60}},
		},
	})

	require.Equal(t, 60*time.Second, settings.RuntimeHealthInterval())
}
