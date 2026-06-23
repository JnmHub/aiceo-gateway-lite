package gatewaylite

import (
	"sync"
	"sync/atomic"
	"time"
)

// RuntimeStats tracks local gateway activity for lightweight health reports.
type RuntimeStats struct {
	mu         sync.Mutex
	activeUser map[int64]time.Time
}

var defaultRuntimeStats atomic.Value

func NewRuntimeStats() *RuntimeStats {
	return &RuntimeStats{activeUser: make(map[int64]time.Time)}
}

func (s *RuntimeStats) RecordUser(userID int64, at time.Time) {
	if s == nil || userID <= 0 {
		return
	}
	if at.IsZero() {
		at = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeUser == nil {
		s.activeUser = make(map[int64]time.Time)
	}
	s.activeUser[userID] = at
}

func (s *RuntimeStats) OnlineUsers(now time.Time, window time.Duration) int {
	if s == nil {
		return 0
	}
	if now.IsZero() {
		now = time.Now()
	}
	if window <= 0 {
		window = 5 * time.Minute
	}
	cutoff := now.Add(-window)
	count := 0
	s.mu.Lock()
	defer s.mu.Unlock()
	for userID, lastSeen := range s.activeUser {
		if lastSeen.Before(cutoff) {
			delete(s.activeUser, userID)
			continue
		}
		count++
	}
	return count
}

func SetDefaultRuntimeStats(stats *RuntimeStats) {
	defaultRuntimeStats.Store(stats)
}

func DefaultRuntimeStats() *RuntimeStats {
	value := defaultRuntimeStats.Load()
	if value == nil {
		return nil
	}
	stats, _ := value.(*RuntimeStats)
	return stats
}
