package gatewaylite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRuntimeStatsCountsActiveUsersWithinWindow(t *testing.T) {
	stats := NewRuntimeStats()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)

	stats.RecordUser(1, now.Add(-time.Minute))
	stats.RecordUser(2, now.Add(-4*time.Minute))
	stats.RecordUser(3, now.Add(-6*time.Minute))

	require.Equal(t, 2, stats.OnlineUsers(now, 5*time.Minute))
	require.Equal(t, 2, stats.OnlineUsers(now, 5*time.Minute), "过期用户应被清理，重复统计应稳定")
}
