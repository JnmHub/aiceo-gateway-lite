package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type gatewayLiteMissSchedulerCache struct{}

func (gatewayLiteMissSchedulerCache) GetSnapshot(context.Context, SchedulerBucket) ([]*Account, bool, error) {
	return nil, false, nil
}
func (gatewayLiteMissSchedulerCache) SetSnapshot(context.Context, SchedulerBucket, []Account) error {
	return nil
}
func (gatewayLiteMissSchedulerCache) GetAccount(context.Context, int64) (*Account, error) {
	return nil, nil
}
func (gatewayLiteMissSchedulerCache) SetAccount(context.Context, *Account) error {
	return nil
}
func (gatewayLiteMissSchedulerCache) DeleteAccount(context.Context, int64) error {
	return nil
}
func (gatewayLiteMissSchedulerCache) UpdateLastUsed(context.Context, map[int64]time.Time) error {
	return nil
}
func (gatewayLiteMissSchedulerCache) TryLockBucket(context.Context, SchedulerBucket, time.Duration) (bool, error) {
	return false, nil
}
func (gatewayLiteMissSchedulerCache) UnlockBucket(context.Context, SchedulerBucket) error {
	return nil
}
func (gatewayLiteMissSchedulerCache) ListBuckets(context.Context) ([]SchedulerBucket, error) {
	return nil, nil
}
func (gatewayLiteMissSchedulerCache) GetOutboxWatermark(context.Context) (int64, error) {
	return 0, nil
}
func (gatewayLiteMissSchedulerCache) SetOutboxWatermark(context.Context, int64) error {
	return nil
}

type gatewayLiteRebuildSchedulerCache struct {
	mu        sync.Mutex
	snapshots map[SchedulerBucket][]Account
	written   chan SchedulerBucket
}

type gatewayLiteRebuildAccountRepo struct {
	stubOpenAIAccountRepo
}

func (r gatewayLiteRebuildAccountRepo) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	platformSet := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		platformSet[platform] = struct{}{}
	}
	out := make([]Account, 0)
	for _, account := range r.accounts {
		if _, ok := platformSet[account.Platform]; ok && account.IsSchedulable() {
			out = append(out, account)
		}
	}
	return out, nil
}

func (r gatewayLiteRebuildAccountRepo) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	return r.ListSchedulableByPlatforms(ctx, platforms)
}

func newGatewayLiteRebuildSchedulerCache() *gatewayLiteRebuildSchedulerCache {
	return &gatewayLiteRebuildSchedulerCache{
		snapshots: make(map[SchedulerBucket][]Account),
		written:   make(chan SchedulerBucket, 32),
	}
}

func (c *gatewayLiteRebuildSchedulerCache) GetSnapshot(_ context.Context, bucket SchedulerBucket) ([]*Account, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	accounts, ok := c.snapshots[bucket]
	if !ok {
		return nil, false, nil
	}
	out := make([]*Account, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		out = append(out, &account)
	}
	return out, true, nil
}
func (c *gatewayLiteRebuildSchedulerCache) SetSnapshot(_ context.Context, bucket SchedulerBucket, accounts []Account) error {
	c.mu.Lock()
	c.snapshots[bucket] = append([]Account(nil), accounts...)
	c.mu.Unlock()
	select {
	case c.written <- bucket:
	default:
	}
	return nil
}
func (c *gatewayLiteRebuildSchedulerCache) GetAccount(context.Context, int64) (*Account, error) {
	return nil, nil
}
func (c *gatewayLiteRebuildSchedulerCache) SetAccount(context.Context, *Account) error {
	return nil
}
func (c *gatewayLiteRebuildSchedulerCache) DeleteAccount(context.Context, int64) error {
	return nil
}
func (c *gatewayLiteRebuildSchedulerCache) UpdateLastUsed(context.Context, map[int64]time.Time) error {
	return nil
}
func (c *gatewayLiteRebuildSchedulerCache) TryLockBucket(context.Context, SchedulerBucket, time.Duration) (bool, error) {
	return true, nil
}
func (c *gatewayLiteRebuildSchedulerCache) UnlockBucket(context.Context, SchedulerBucket) error {
	return nil
}
func (c *gatewayLiteRebuildSchedulerCache) ListBuckets(context.Context) ([]SchedulerBucket, error) {
	return nil, nil
}
func (c *gatewayLiteRebuildSchedulerCache) GetOutboxWatermark(context.Context) (int64, error) {
	return 0, nil
}
func (c *gatewayLiteRebuildSchedulerCache) SetOutboxWatermark(context.Context, int64) error {
	return nil
}

func (c *gatewayLiteRebuildSchedulerCache) snapshot(bucket SchedulerBucket) []Account {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Account(nil), c.snapshots[bucket]...)
}

func TestSchedulerSnapshotGatewayLiteDoesNotFallbackToDB(t *testing.T) {
	svc := NewSchedulerSnapshotService(
		gatewayLiteMissSchedulerCache{},
		nil,
		nil,
		nil,
		&config.Config{RunMode: config.RunModeGatewayLite},
	)

	accounts, _, err := svc.ListSchedulableAccounts(context.Background(), nil, PlatformOpenAI, false)
	require.ErrorIs(t, err, ErrSchedulerCacheNotReady)
	require.Nil(t, accounts)

	account, err := svc.GetAccount(context.Background(), 1001)
	require.ErrorIs(t, err, ErrSchedulerCacheNotReady)
	require.Nil(t, account)

	group, err := svc.GetGroupByID(context.Background(), 7)
	require.NoError(t, err)
	require.Nil(t, group)
}

func TestSchedulerSnapshotGatewayLiteStartRebuildsLocalAccountBuckets(t *testing.T) {
	cache := newGatewayLiteRebuildSchedulerCache()
	svc := NewSchedulerSnapshotService(
		cache,
		nil,
		gatewayLiteRebuildAccountRepo{stubOpenAIAccountRepo{accounts: []Account{{
			ID:          1001,
			Name:        "gateway-lite-openai",
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Status:      StatusActive,
			Schedulable: true,
			Concurrency: 200,
		}}}},
		nil,
		&config.Config{RunMode: config.RunModeGatewayLite},
	)

	svc.Start()
	defer svc.Stop()

	target := SchedulerBucket{GroupID: 0, Platform: PlatformOpenAI, Mode: SchedulerModeSingle}
	deadline := time.After(2 * time.Second)
	for {
		if got := cache.snapshot(target); len(got) == 1 && got[0].ID == 1001 {
			return
		}
		select {
		case <-cache.written:
		case <-deadline:
			t.Fatalf("gateway-lite scheduler startup rebuild did not populate bucket %s", target.String())
		}
	}
}

func TestSchedulerSnapshotGatewayLiteNormalizesRequestedGroupToPlatformBucket(t *testing.T) {
	cache := newGatewayLiteRebuildSchedulerCache()
	account := Account{
		ID:          1001,
		Name:        "gateway-lite-openai",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 200,
	}
	require.NoError(t, cache.SetSnapshot(context.Background(), SchedulerBucket{
		GroupID:  0,
		Platform: PlatformOpenAI,
		Mode:     SchedulerModeSingle,
	}, []Account{account}))

	svc := NewSchedulerSnapshotService(
		cache,
		nil,
		nil,
		nil,
		&config.Config{RunMode: config.RunModeGatewayLite},
	)

	mainGroupID := int64(1)
	accounts, _, err := svc.ListSchedulableAccounts(context.Background(), &mainGroupID, PlatformOpenAI, false)
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, int64(1001), accounts[0].ID)
}
