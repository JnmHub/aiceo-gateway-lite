package gatewaylite

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRedisKeyCacheGetSet(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	cache := NewRedisKeyCache(client, "test")

	key := KeySnapshot{
		KeyID:          "key123",
		UserID:         42,
		SecretHash:     "hash",
		Status:         "active",
		Platform:       "openai",
		GroupID:        7,
		GroupName:      "openai",
		CacheTTLSecond: 60,
	}
	require.NoError(t, cache.Set(ctx, key, "sg"))

	got, ok, err := cache.Get(ctx, "key123", "sg")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, key.KeyID, got.KeyID)
	require.Equal(t, key.UserID, got.UserID)
	require.Equal(t, key.SecretHash, got.SecretHash)
}

func TestRedisKeyCacheDeleteRegion(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	cache := NewRedisKeyCache(client, "test")

	require.NoError(t, cache.Set(ctx, KeySnapshot{KeyID: "sg-key", CacheTTLSecond: 60}, "sg"))
	require.NoError(t, cache.Set(ctx, KeySnapshot{KeyID: "us-key", CacheTTLSecond: 60}, "us"))

	require.NoError(t, cache.DeleteRegion(ctx, "sg"))
	_, ok, err := cache.Get(ctx, "sg-key", "sg")
	require.NoError(t, err)
	require.False(t, ok)

	got, ok, err := cache.Get(ctx, "us-key", "us")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "us-key", got.KeyID)
}
