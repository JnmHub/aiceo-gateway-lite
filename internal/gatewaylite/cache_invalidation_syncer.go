package gatewaylite

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

const defaultCacheInvalidationInterval = 5 * time.Second

type CacheInvalidationClient interface {
	FetchCacheInvalidations(ctx context.Context, req CacheInvalidationRequest) (*CacheInvalidationResponse, error)
}

type ConfigRefresher interface {
	SyncFull(ctx context.Context) error
}

type KeyRegionInvalidator interface {
	DeleteRegion(ctx context.Context, region string) error
}

type CacheInvalidationSyncer struct {
	client          CacheInvalidationClient
	configCache     *RedisConfigCache
	keyInvalidator  KeyRegionInvalidator
	configRefresher ConfigRefresher
	mu              sync.RWMutex
	region          string
	gatewayCode     string
	interval        time.Duration
}

func NewCacheInvalidationSyncer(client CacheInvalidationClient, configCache *RedisConfigCache, keyInvalidator KeyRegionInvalidator, configRefresher ConfigRefresher, region string, gatewayCode string, interval time.Duration) *CacheInvalidationSyncer {
	if region == "" {
		region = "default"
	}
	if gatewayCode == "" {
		gatewayCode = region
	}
	if interval <= 0 {
		interval = defaultCacheInvalidationInterval
	}
	return &CacheInvalidationSyncer{
		client:          client,
		configCache:     configCache,
		keyInvalidator:  keyInvalidator,
		configRefresher: configRefresher,
		region:          region,
		gatewayCode:     gatewayCode,
		interval:        interval,
	}
}

func (s *CacheInvalidationSyncer) Start(ctx context.Context) {
	if s == nil || s.client == nil || s.configCache == nil || !s.configCache.Enabled() {
		return
	}
	go s.run(ctx)
}

func (s *CacheInvalidationSyncer) SyncOnce(ctx context.Context) {
	if s == nil {
		return
	}
	s.syncOnce(ctx)
}

func (s *CacheInvalidationSyncer) run(ctx context.Context) {
	s.syncOnce(ctx)
	timer := time.NewTimer(s.currentInterval())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.syncOnce(ctx)
			timer.Reset(s.currentInterval())
		}
	}
}

func (s *CacheInvalidationSyncer) syncOnce(ctx context.Context) {
	region, gatewayCode, _ := s.currentRuntimeConfig()
	sinceID, err := s.configCache.InvalidationCursor(ctx)
	if err != nil {
		log.Printf("gateway-lite: cache invalidation cursor read failed: %v", err)
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	resp, err := s.client.FetchCacheInvalidations(reqCtx, CacheInvalidationRequest{
		GatewayCode: gatewayCode,
		Region:      region,
		SinceID:     sinceID,
		Limit:       50,
	})
	if err != nil {
		log.Printf("gateway-lite: cache invalidation fetch failed: %v", err)
		return
	}
	if resp == nil || !resp.OK {
		errMsg := ""
		if resp != nil {
			errMsg = resp.Error
		}
		log.Printf("gateway-lite: cache invalidation rejected: %s", errMsg)
		return
	}

	ackIDs := make([]int64, 0, len(resp.Events))
	latestID := resp.LatestID
	for _, event := range resp.Events {
		if event.ID > latestID {
			latestID = event.ID
		}
		if !s.shouldProcess(event, region, gatewayCode) {
			ackIDs = append(ackIDs, event.ID)
			continue
		}
		if err := s.handleEvent(ctx, event, region); err != nil {
			log.Printf("gateway-lite: cache invalidation event handle failed id=%d scope=%s: %v", event.ID, event.Scope, err)
			return
		}
		ackIDs = append(ackIDs, event.ID)
	}

	if latestID > sinceID {
		if err := s.configCache.SetInvalidationCursor(ctx, latestID); err != nil {
			log.Printf("gateway-lite: cache invalidation cursor write failed: %v", err)
			return
		}
	}
	if len(ackIDs) > 0 {
		s.ack(ctx, latestID, ackIDs, region, gatewayCode)
	}
}

func (s *CacheInvalidationSyncer) SetRuntimeConfig(region, gatewayCode string, interval time.Duration) {
	if s == nil {
		return
	}
	region = strings.TrimSpace(region)
	if region == "" {
		region = "default"
	}
	gatewayCode = strings.TrimSpace(gatewayCode)
	if gatewayCode == "" {
		gatewayCode = region
	}
	if interval <= 0 {
		interval = defaultCacheInvalidationInterval
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.region = region
	s.gatewayCode = gatewayCode
	s.interval = interval
}

func (s *CacheInvalidationSyncer) currentInterval() time.Duration {
	_, _, interval := s.currentRuntimeConfig()
	return interval
}

func (s *CacheInvalidationSyncer) currentRuntimeConfig() (string, string, time.Duration) {
	if s == nil {
		return "default", "default", defaultCacheInvalidationInterval
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	region := s.region
	gatewayCode := s.gatewayCode
	interval := s.interval
	if region == "" {
		region = "default"
	}
	if gatewayCode == "" {
		gatewayCode = region
	}
	if interval <= 0 {
		interval = defaultCacheInvalidationInterval
	}
	return region, gatewayCode, interval
}

func (s *CacheInvalidationSyncer) shouldProcess(event CacheInvalidationEvent, region, gatewayCode string) bool {
	if event.GatewayCode != "" && event.GatewayCode != gatewayCode {
		return false
	}
	if event.Region != "" && event.Region != region {
		return false
	}
	return true
}

func (s *CacheInvalidationSyncer) handleEvent(ctx context.Context, event CacheInvalidationEvent, region string) error {
	switch event.Scope {
	case "config:snapshot":
		if s.configRefresher != nil {
			return s.configRefresher.SyncFull(ctx)
		}
	case "key:snapshot":
		if s.keyInvalidator != nil {
			return s.keyInvalidator.DeleteRegion(ctx, region)
		}
	default:
		log.Printf("gateway-lite: ignored cache invalidation event id=%d scope=%s", event.ID, event.Scope)
	}
	return nil
}

func (s *CacheInvalidationSyncer) ack(ctx context.Context, latestID int64, ids []int64, region, gatewayCode string) {
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := s.client.FetchCacheInvalidations(reqCtx, CacheInvalidationRequest{
		GatewayCode: gatewayCode,
		Region:      region,
		SinceID:     latestID,
		Limit:       1,
		AckIDs:      ids,
	})
	if err != nil {
		log.Printf("gateway-lite: cache invalidation ack failed: %v", err)
	}
}
