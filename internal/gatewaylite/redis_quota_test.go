package gatewaylite

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRedisQuotaReserveCommitRefund(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	quota := NewRedisQuota(client, "test")
	lease := LeaseSnapshot{
		LeaseID:        "lease1",
		UserID:         42,
		Region:         "sg",
		AllocatedCents: 100,
		ExpiresAt:      time.Now().Add(time.Hour).Unix(),
	}

	require.NoError(t, quota.EnsureLease(ctx, lease))
	loaded, ok, err := quota.LoadLease(ctx, 42, "sg")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "lease1", loaded.LeaseID)

	ok, err = quota.Reserve(ctx, ReserveRequest{
		RequestID:          "req1",
		KeyID:              "key1",
		UserID:             42,
		Region:             "sg",
		LeaseID:            "lease1",
		EstimatedCostCents: 10,
	})
	require.NoError(t, err)
	require.True(t, ok)

	fields, err := client.HGetAll(ctx, "test:lease:42:sg").Result()
	require.NoError(t, err)
	require.EqualValues(t, 10, ParseInt64Field(fields, "reserved_cents"))

	require.NoError(t, quota.Commit(ctx, UsageCommit{RequestID: "req1", ActualCents: 7}))
	fields, err = client.HGetAll(ctx, "test:lease:42:sg").Result()
	require.NoError(t, err)
	require.EqualValues(t, 0, ParseInt64Field(fields, "reserved_cents"))
	require.EqualValues(t, 7, ParseInt64Field(fields, "spent_cents"))

	require.NoError(t, quota.AdjustCommitted(ctx, UsageCommit{RequestID: "req1", ActualCents: 9}))
	fields, err = client.HGetAll(ctx, "test:lease:42:sg").Result()
	require.NoError(t, err)
	require.EqualValues(t, 9, ParseInt64Field(fields, "spent_cents"))

	ok, err = quota.Reserve(ctx, ReserveRequest{
		RequestID:          "req2",
		KeyID:              "key1",
		UserID:             42,
		Region:             "sg",
		LeaseID:            "lease1",
		EstimatedCostCents: 10,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, quota.Refund(ctx, "req2"))
	fields, err = client.HGetAll(ctx, "test:lease:42:sg").Result()
	require.NoError(t, err)
	require.EqualValues(t, 0, ParseInt64Field(fields, "reserved_cents"))
	require.EqualValues(t, 9, ParseInt64Field(fields, "spent_cents"))
}

func TestRedisQuotaAdjustBeforeCommit(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	quota := NewRedisQuota(client, "test")

	require.NoError(t, quota.EnsureLease(ctx, LeaseSnapshot{
		LeaseID:        "lease1",
		UserID:         42,
		Region:         "sg",
		AllocatedCents: 100,
		ExpiresAt:      time.Now().Add(time.Hour).Unix(),
	}))
	ok, err := quota.Reserve(ctx, ReserveRequest{
		RequestID:          "req1",
		KeyID:              "key1",
		UserID:             42,
		Region:             "sg",
		LeaseID:            "lease1",
		EstimatedCostCents: 1,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, quota.AdjustCommitted(ctx, UsageCommit{RequestID: "req1", ActualCents: 7}))
	require.NoError(t, quota.Commit(ctx, UsageCommit{RequestID: "req1", ActualCents: 1}))

	fields, err := client.HGetAll(ctx, "test:lease:42:sg").Result()
	require.NoError(t, err)
	require.EqualValues(t, 0, ParseInt64Field(fields, "reserved_cents"))
	require.EqualValues(t, 7, ParseInt64Field(fields, "spent_cents"))
}

func TestRedisQuotaEnsureLeasePreservesLocalCounters(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	quota := NewRedisQuota(client, "test")

	require.NoError(t, quota.EnsureLease(ctx, LeaseSnapshot{
		LeaseID:        "lease1",
		UserID:         42,
		Region:         "sg",
		AllocatedCents: 100,
		ExpiresAt:      time.Now().Add(time.Hour).Unix(),
	}))
	ok, err := quota.Reserve(ctx, ReserveRequest{
		RequestID:          "req1",
		KeyID:              "key1",
		UserID:             42,
		Region:             "sg",
		LeaseID:            "lease1",
		EstimatedCostCents: 10,
	})
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, quota.EnsureLease(ctx, LeaseSnapshot{
		LeaseID:        "lease1",
		UserID:         42,
		Region:         "sg",
		AllocatedCents: 200,
		ReservedCents:  0,
		SpentCents:     0,
		Version:        2,
		ExpiresAt:      time.Now().Add(2 * time.Hour).Unix(),
	}))

	fields, err := client.HGetAll(ctx, "test:lease:42:sg").Result()
	require.NoError(t, err)
	require.EqualValues(t, 200, ParseInt64Field(fields, "allocated_cents"))
	require.EqualValues(t, 10, ParseInt64Field(fields, "reserved_cents"))
	require.EqualValues(t, 0, ParseInt64Field(fields, "spent_cents"))
	require.EqualValues(t, 2, ParseInt64Field(fields, "version"))
}
