package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/gatewaylite"
	"github.com/stretchr/testify/require"
)

type gatewayLiteUsageLogRepoStub struct {
	UsageLogRepository
	bestEffortCalls int
	createCalls     int
	resolveCalls    int
	resolveErr      error
	remoteUserID    int64
	remoteKeyID     string
	resolvedUserID  int64
	resolvedKeyID   int64
	lastLog         *UsageLog
}

func (s *gatewayLiteUsageLogRepoStub) CreateBestEffort(_ context.Context, log *UsageLog) error {
	s.bestEffortCalls++
	s.lastLog = log
	return nil
}

func (s *gatewayLiteUsageLogRepoStub) Create(_ context.Context, log *UsageLog) (bool, error) {
	s.createCalls++
	s.lastLog = log
	return true, nil
}

func (s *gatewayLiteUsageLogRepoStub) ResolveGatewayLiteUsageIdentity(_ context.Context, remoteUserID int64, remoteKeyID string) (int64, int64, error) {
	s.resolveCalls++
	s.remoteUserID = remoteUserID
	s.remoteKeyID = remoteKeyID
	if s.resolveErr != nil {
		return 0, 0, s.resolveErr
	}
	return s.resolvedUserID, s.resolvedKeyID, nil
}

func TestWriteUsageLogBestEffortPersistsRemoteGatewayLiteKeyWithLocalShadowIdentity(t *testing.T) {
	repo := &gatewayLiteUsageLogRepoStub{resolvedUserID: 420, resolvedKeyID: 700}
	cfg := &config.Config{RunMode: config.RunModeGatewayLite}
	groupID := int64(99)
	subscriptionID := int64(77)
	ctx := gatewaylite.ContextWithRequestMeta(context.Background(), gatewaylite.RequestMeta{
		RequestID: "req-1",
		KeyID:     "key123",
		UserID:    42,
		Region:    "sg",
	})

	writeUsageLogBestEffort(ctx, repo, &UsageLog{
		UserID:         42,
		APIKeyID:       0,
		GroupID:        &groupID,
		SubscriptionID: &subscriptionID,
	}, "service.gateway", cfg)

	require.Equal(t, 1, repo.resolveCalls)
	require.Equal(t, int64(42), repo.remoteUserID)
	require.Equal(t, "key123", repo.remoteKeyID)
	require.Equal(t, 1, repo.bestEffortCalls)
	require.Equal(t, 0, repo.createCalls)
	require.NotNil(t, repo.lastLog)
	require.Equal(t, int64(420), repo.lastLog.UserID)
	require.Equal(t, int64(700), repo.lastLog.APIKeyID)
	require.Nil(t, repo.lastLog.GroupID)
	require.Nil(t, repo.lastLog.SubscriptionID)
}

func TestWriteUsageLogBestEffortSkipsRemoteGatewayLiteKeyWhenShadowIdentityFails(t *testing.T) {
	repo := &gatewayLiteUsageLogRepoStub{resolveErr: errors.New("db unavailable")}
	cfg := &config.Config{RunMode: config.RunModeGatewayLite}

	writeUsageLogBestEffort(context.Background(), repo, &UsageLog{
		UserID:   42,
		APIKeyID: 0,
	}, "service.gateway", cfg)

	require.Equal(t, 1, repo.resolveCalls)
	require.Equal(t, 0, repo.bestEffortCalls)
	require.Equal(t, 0, repo.createCalls)
}

func TestWriteUsageLogBestEffortKeepsLocalKeyInGatewayLite(t *testing.T) {
	repo := &gatewayLiteUsageLogRepoStub{}
	cfg := &config.Config{RunMode: config.RunModeGatewayLite}

	writeUsageLogBestEffort(context.Background(), repo, &UsageLog{
		UserID:   42,
		APIKeyID: 7,
	}, "service.gateway", cfg)

	require.Equal(t, 1, repo.bestEffortCalls)
	require.Equal(t, 0, repo.createCalls)
}
