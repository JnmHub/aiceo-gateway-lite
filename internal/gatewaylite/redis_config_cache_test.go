package gatewaylite

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type configClientFunc func(context.Context, GatewayConfigSnapshotRequest) (*GatewayConfigSnapshotResponse, error)

func (f configClientFunc) FetchGatewayConfigSnapshot(ctx context.Context, req GatewayConfigSnapshotRequest) (*GatewayConfigSnapshotResponse, error) {
	return f(ctx, req)
}

func TestRedisConfigCacheGetSet(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	cache := NewRedisConfigCache(client, "test")

	snapshot := GatewayConfigSnapshot{
		Version:           3,
		GeneratedAtMillis: time.Now().UnixMilli(),
		Accounts: []GatewayAccountSnapshot{{
			ID:          10,
			Name:        "openai-1",
			Platform:    "openai",
			Type:        "api_key",
			Status:      "active",
			Schedulable: true,
			Concurrency: 5,
			GroupIDs:    []int64{7},
		}},
		Groups: []GatewayGroupSnapshot{{
			ID:             7,
			Name:           "default",
			Platform:       "openai",
			Status:         "active",
			RateMultiplier: 1,
		}},
	}
	require.NoError(t, cache.Set(ctx, snapshot))

	version, err := cache.Version(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 3, version)

	got, ok, err := cache.Get(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.EqualValues(t, 10, got.Accounts[0].ID)
	require.EqualValues(t, 7, got.Groups[0].ID)
}

func TestRedisConfigCacheInvalidationCursor(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	cache := NewRedisConfigCache(client, "cursor")

	got, err := cache.InvalidationCursor(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, got)

	require.NoError(t, cache.SetInvalidationCursor(ctx, 123))
	got, err = cache.InvalidationCursor(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 123, got)
}

func TestConfigSyncerWritesSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	cache := NewRedisConfigCache(client, "sync")

	calls := 0
	syncer := NewConfigSyncer(configClientFunc(func(_ context.Context, req GatewayConfigSnapshotRequest) (*GatewayConfigSnapshotResponse, error) {
		calls++
		require.Equal(t, "sg", req.Region)
		return &GatewayConfigSnapshotResponse{
			OK: true,
			Snapshot: GatewayConfigSnapshot{
				Version:           1,
				GeneratedAtMillis: time.Now().UnixMilli(),
				Accounts:          []GatewayAccountSnapshot{{ID: 1, Platform: "openai", Status: "active"}},
			},
		}, nil
	}), cache, "sg", time.Hour)

	syncer.syncOnce(ctx)
	require.Equal(t, 1, calls)
	got, ok, err := cache.Get(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.EqualValues(t, 1, got.Version)
	require.Len(t, got.Accounts, 1)
}
