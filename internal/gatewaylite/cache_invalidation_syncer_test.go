package gatewaylite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type cacheInvalidationClientFunc func(context.Context, CacheInvalidationRequest) (*CacheInvalidationResponse, error)

func (f cacheInvalidationClientFunc) FetchCacheInvalidations(ctx context.Context, req CacheInvalidationRequest) (*CacheInvalidationResponse, error) {
	return f(ctx, req)
}

type configRefresherFunc func(context.Context)

func (f configRefresherFunc) SyncFull(ctx context.Context) error {
	f(ctx)
	return nil
}

type failingConfigRefresher struct{}

func (f failingConfigRefresher) SyncFull(context.Context) error {
	return errors.New("refresh failed")
}

func TestCacheInvalidationSyncerRefreshesConfigAndDeletesKeys(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	configCache := NewRedisConfigCache(client, "inv")
	keyCache := NewRedisKeyCache(client, "inv")
	require.NoError(t, keyCache.Set(ctx, KeySnapshot{KeyID: "key-sg", CacheTTLSecond: 60}, "sg"))
	require.NoError(t, keyCache.Set(ctx, KeySnapshot{KeyID: "key-us", CacheTTLSecond: 60}, "us"))

	refreshes := 0
	requests := make([]CacheInvalidationRequest, 0)
	syncer := NewCacheInvalidationSyncer(cacheInvalidationClientFunc(func(_ context.Context, req CacheInvalidationRequest) (*CacheInvalidationResponse, error) {
		requests = append(requests, req)
		if len(req.AckIDs) > 0 {
			return &CacheInvalidationResponse{OK: true, LatestID: req.SinceID}, nil
		}
		require.Equal(t, "sg-1", req.GatewayCode)
		require.Equal(t, "sg", req.Region)
		require.EqualValues(t, 0, req.SinceID)
		return &CacheInvalidationResponse{
			OK:       true,
			LatestID: 2,
			Events: []CacheInvalidationEvent{
				{ID: 1, Scope: "config:snapshot", GatewayCode: "sg-1", Region: "sg"},
				{ID: 2, Scope: "key:snapshot", GatewayCode: "sg-1", Region: "sg"},
			},
		}, nil
	}), configCache, keyCache, configRefresherFunc(func(context.Context) {
		refreshes++
	}), "sg", "sg-1", time.Hour)

	syncer.SyncOnce(ctx)

	require.Equal(t, 1, refreshes)
	cursor, err := configCache.InvalidationCursor(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, cursor)
	_, ok, err := keyCache.Get(ctx, "key-sg", "sg")
	require.NoError(t, err)
	require.False(t, ok)
	got, ok, err := keyCache.Get(ctx, "key-us", "us")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "key-us", got.KeyID)
	require.Len(t, requests, 2)
	require.Equal(t, []int64{1, 2}, requests[1].AckIDs)
}

func TestCacheInvalidationSyncerSkipsOtherGatewayEvents(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	configCache := NewRedisConfigCache(client, "skip")

	refreshes := 0
	syncer := NewCacheInvalidationSyncer(cacheInvalidationClientFunc(func(_ context.Context, req CacheInvalidationRequest) (*CacheInvalidationResponse, error) {
		if len(req.AckIDs) > 0 {
			return &CacheInvalidationResponse{OK: true, LatestID: req.SinceID}, nil
		}
		return &CacheInvalidationResponse{
			OK:       true,
			LatestID: 9,
			Events: []CacheInvalidationEvent{
				{ID: 9, Scope: "config:snapshot", GatewayCode: "jp-1", Region: "jp"},
			},
		}, nil
	}), configCache, nil, configRefresherFunc(func(context.Context) {
		refreshes++
	}), "sg", "sg-1", time.Hour)

	syncer.SyncOnce(ctx)

	require.Equal(t, 0, refreshes)
	cursor, err := configCache.InvalidationCursor(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 9, cursor)
}

func TestCacheInvalidationSyncerDoesNotAdvanceCursorOnRefreshFailure(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	configCache := NewRedisConfigCache(client, "fail")

	syncer := NewCacheInvalidationSyncer(cacheInvalidationClientFunc(func(_ context.Context, req CacheInvalidationRequest) (*CacheInvalidationResponse, error) {
		if len(req.AckIDs) > 0 {
			t.Fatalf("unexpected ack after failed refresh")
		}
		return &CacheInvalidationResponse{
			OK:       true,
			LatestID: 7,
			Events: []CacheInvalidationEvent{
				{ID: 7, Scope: "config:snapshot", GatewayCode: "sg-1", Region: "sg"},
			},
		}, nil
	}), configCache, nil, failingConfigRefresher{}, "sg", "sg-1", time.Hour)

	syncer.SyncOnce(ctx)

	cursor, err := configCache.InvalidationCursor(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, cursor)
}
