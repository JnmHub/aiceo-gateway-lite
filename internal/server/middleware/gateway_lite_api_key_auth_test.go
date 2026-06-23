package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gatewaylite"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type gatewayLiteControlStub struct {
	key            gatewaylite.KeySnapshot
	lease          gatewaylite.LeaseSnapshot
	resolveResp    *gatewaylite.ResolveKeyResponse
	acquireResp    *gatewaylite.AcquireLeaseResponse
	rebalanceResp  *gatewaylite.RebalanceLeaseResponse
	resolveCount   *atomic.Int32
	acquireCount   *atomic.Int32
	rebalanceCount *atomic.Int32
	resolveFn      func(gatewaylite.ResolveKeyRequest)
	reportFn       func(gatewaylite.UsageEvent)
}

func (s gatewayLiteControlStub) ResolveKey(_ context.Context, req gatewaylite.ResolveKeyRequest) (*gatewaylite.ResolveKeyResponse, error) {
	if s.resolveCount != nil {
		s.resolveCount.Add(1)
	}
	if s.resolveFn != nil {
		s.resolveFn(req)
	}
	if s.resolveResp != nil {
		return s.resolveResp, nil
	}
	return &gatewaylite.ResolveKeyResponse{OK: true, Key: s.key}, nil
}

func (s gatewayLiteControlStub) AcquireLease(context.Context, gatewaylite.AcquireLeaseRequest) (*gatewaylite.AcquireLeaseResponse, error) {
	if s.acquireCount != nil {
		s.acquireCount.Add(1)
	}
	if s.acquireResp != nil {
		return s.acquireResp, nil
	}
	return &gatewaylite.AcquireLeaseResponse{OK: true, Lease: s.lease}, nil
}

func (s gatewayLiteControlStub) RebalanceLease(context.Context, gatewaylite.RebalanceLeaseRequest) (*gatewaylite.RebalanceLeaseResponse, error) {
	if s.rebalanceCount != nil {
		s.rebalanceCount.Add(1)
	}
	if s.rebalanceResp != nil {
		return s.rebalanceResp, nil
	}
	return &gatewaylite.RebalanceLeaseResponse{OK: false, Error: "not_configured"}, nil
}

func (s gatewayLiteControlStub) ReportUsage(_ context.Context, event gatewaylite.UsageEvent) error {
	if s.reportFn != nil {
		s.reportFn(event)
	}
	return nil
}

func TestGatewayLiteAPIKeyAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stats := gatewaylite.NewRuntimeStats()
	gatewaylite.SetDefaultRuntimeStats(stats)
	secret := "secret-value"
	hash := sha256.Sum256([]byte(secret))
	key := "aiceo_sk_key123_" + secret
	stub := gatewayLiteControlStub{
		key: gatewaylite.KeySnapshot{
			KeyID:         "key123",
			UserID:        42,
			SecretHash:    hex.EncodeToString(hash[:]),
			Status:        service.StatusActive,
			Platform:      service.PlatformOpenAI,
			GroupID:       7,
			GroupName:     "openai",
			Concurrency:   3,
			RateLimitRPM:  120,
			RateLimitTPM:  100000,
			AllowedModels: []string{"*"},
		},
		lease: gatewaylite.LeaseSnapshot{
			LeaseID:        "lease_sg_1",
			UserID:         42,
			Region:         "sg",
			AllocatedCents: 100,
			ExpiresAt:      time.Now().Add(time.Minute).Unix(),
		},
	}

	router := gin.New()
	router.GET("/probe", gin.HandlerFunc(NewGatewayLiteAPIKeyAuthMiddleware(stub, "sg", "sg-1", nil, nil, stub)), func(c *gin.Context) {
		apiKey, ok := GetAPIKeyFromContext(c)
		require.True(t, ok)
		require.Equal(t, int64(42), apiKey.User.ID)
		require.Equal(t, service.PlatformOpenAI, apiKey.Group.Platform)
		require.Equal(t, []string{"*"}, apiKey.AllowedModels)
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, 1, stats.OnlineUsers(time.Now(), time.Minute))
}

func TestGatewayLiteAPIKeyAuthMiddlewareAcceptsLegacyFMKeyPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "secret-value"
	hash := sha256.Sum256([]byte(secret))
	key := "fm_sk_key123_" + secret
	stub := gatewayLiteControlStub{
		key: gatewaylite.KeySnapshot{
			KeyID:         "key123",
			UserID:        42,
			SecretHash:    hex.EncodeToString(hash[:]),
			Status:        service.StatusActive,
			Platform:      service.PlatformOpenAI,
			GroupID:       7,
			GroupName:     "openai",
			AllowedModels: []string{"*"},
		},
		lease: gatewaylite.LeaseSnapshot{
			LeaseID:        "lease_sg_1",
			UserID:         42,
			Region:         "sg",
			AllocatedCents: 100,
			ExpiresAt:      time.Now().Add(time.Minute).Unix(),
		},
	}

	router := gin.New()
	router.GET("/probe", gin.HandlerFunc(NewGatewayLiteAPIKeyAuthMiddleware(stub, "sg", "sg-1", nil, nil, stub)), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestGatewayLiteAPIKeyAuthMiddlewareDoesNotBillModelsList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "secret-value"
	hash := sha256.Sum256([]byte(secret))
	key := "aiceo_sk_key123_" + secret
	var acquireCount atomic.Int32
	resolveRequests := make(chan gatewaylite.ResolveKeyRequest, 1)
	reported := make(chan gatewaylite.UsageEvent, 1)
	stub := gatewayLiteControlStub{
		key: gatewaylite.KeySnapshot{
			KeyID:         "key123",
			UserID:        42,
			SecretHash:    hex.EncodeToString(hash[:]),
			Status:        service.StatusActive,
			Platform:      service.PlatformOpenAI,
			GroupID:       7,
			GroupName:     "openai",
			Concurrency:   3,
			AllowedModels: []string{"*"},
		},
		acquireCount: &acquireCount,
		resolveFn: func(req gatewaylite.ResolveKeyRequest) {
			resolveRequests <- req
		},
		reportFn: func(event gatewaylite.UsageEvent) {
			reported <- event
		},
	}

	router := gin.New()
	router.GET("/v1/models", gin.HandlerFunc(NewGatewayLiteAPIKeyAuthMiddleware(stub, "sg", "sg-1", nil, nil, stub)), func(c *gin.Context) {
		apiKey, ok := GetAPIKeyFromContext(c)
		require.True(t, ok)
		require.Equal(t, int64(42), apiKey.User.ID)
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": []any{}})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.EqualValues(t, 0, acquireCount.Load())
	select {
	case req := <-resolveRequests:
		require.True(t, req.BillingExempt)
	default:
		t.Fatal("expected resolve request")
	}
	select {
	case event := <-reported:
		t.Fatalf("models list should not report usage: %+v", event)
	default:
	}
}

func TestGatewayLiteAPIKeyAuthMiddlewareRejectsUnavailableGateway(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "secret-value"
	hash := sha256.Sum256([]byte(secret))
	key := "aiceo_sk_key123_" + secret
	stub := gatewayLiteControlStub{
		key: gatewaylite.KeySnapshot{
			KeyID:                 "key123",
			UserID:                42,
			SecretHash:            hex.EncodeToString(hash[:]),
			Status:                service.StatusActive,
			Platform:              service.PlatformOpenAI,
			GroupID:               7,
			GroupName:             "openai",
			GatewayAccessEnforced: true,
			AvailableGateways: []gatewaylite.GatewayRouteSummary{
				{Code: "sg-1", Region: "sg"},
			},
		},
		lease: gatewaylite.LeaseSnapshot{
			LeaseID:        "lease_hk_1",
			UserID:         42,
			Region:         "hk",
			AllocatedCents: 100,
			ExpiresAt:      time.Now().Add(time.Minute).Unix(),
		},
	}

	router := gin.New()
	router.GET("/probe", gin.HandlerFunc(NewGatewayLiteAPIKeyAuthMiddleware(stub, "hk", "hk-1", nil, nil, stub)), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestGatewayLiteAPIKeyAuthMiddlewareRejectsPhoneVerificationRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := "aiceo_sk_key123_secret-value"
	stub := gatewayLiteControlStub{
		resolveResp: &gatewaylite.ResolveKeyResponse{OK: false, Error: "phone_verification_required"},
	}

	router := gin.New()
	router.GET("/probe", gin.HandlerFunc(NewGatewayLiteAPIKeyAuthMiddleware(stub, "hk", "hk-1", nil, nil, stub)), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "PHONE_VERIFICATION_REQUIRED")
}

func TestGatewayLiteAPIKeyAuthMiddlewareRejectsDisallowedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "secret-value"
	hash := sha256.Sum256([]byte(secret))
	key := "aiceo_sk_key123_" + secret
	var acquireCount atomic.Int32
	stub := gatewayLiteControlStub{
		key: gatewaylite.KeySnapshot{
			KeyID:         "key123",
			UserID:        42,
			SecretHash:    hex.EncodeToString(hash[:]),
			Status:        service.StatusActive,
			Platform:      service.PlatformOpenAI,
			GroupID:       7,
			GroupName:     "openai",
			AllowedModels: []string{"gpt-4o-mini"},
		},
		lease: gatewaylite.LeaseSnapshot{
			LeaseID:        "lease_sg_1",
			UserID:         42,
			Region:         "sg",
			AllocatedCents: 100,
			ExpiresAt:      time.Now().Add(time.Minute).Unix(),
		},
		acquireCount: &acquireCount,
	}

	router := gin.New()
	router.POST("/v1/chat/completions", gin.HandlerFunc(NewGatewayLiteAPIKeyAuthMiddleware(stub, "sg", "sg-1", nil, nil, stub)), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.1","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "MODEL_NOT_ALLOWED")
	require.EqualValues(t, 0, acquireCount.Load())
}

func TestGatewayLiteAPIKeyAuthMiddlewareAllowsModelAndRestoresBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "secret-value"
	hash := sha256.Sum256([]byte(secret))
	key := "aiceo_sk_key123_" + secret
	stub := gatewayLiteControlStub{
		key: gatewaylite.KeySnapshot{
			KeyID:         "key123",
			UserID:        42,
			SecretHash:    hex.EncodeToString(hash[:]),
			Status:        service.StatusActive,
			Platform:      service.PlatformOpenAI,
			GroupID:       7,
			GroupName:     "openai",
			AllowedModels: []string{"gpt-5*"},
		},
		lease: gatewaylite.LeaseSnapshot{
			LeaseID:        "lease_sg_1",
			UserID:         42,
			Region:         "sg",
			AllocatedCents: 100,
			ExpiresAt:      time.Now().Add(time.Minute).Unix(),
		},
	}

	router := gin.New()
	router.POST("/v1/chat/completions", gin.HandlerFunc(NewGatewayLiteAPIKeyAuthMiddleware(stub, "sg", "sg-1", nil, nil, stub)), func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"gpt-5.1","messages":[]}`, string(body))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.1","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestGatewayLiteAPIKeyAuthMiddlewareCommitsRedisQuotaAndReportsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	quota := gatewaylite.NewRedisQuota(client, "mwtest")

	secret := "secret-value"
	hash := sha256.Sum256([]byte(secret))
	key := "aiceo_sk_key123_" + secret
	reported := make(chan gatewaylite.UsageEvent, 1)
	stub := gatewayLiteControlStub{
		key: gatewaylite.KeySnapshot{
			KeyID:        "key123",
			UserID:       42,
			SecretHash:   hex.EncodeToString(hash[:]),
			Status:       service.StatusActive,
			Platform:     service.PlatformOpenAI,
			GroupID:      7,
			GroupName:    "openai",
			Concurrency:  3,
			RateLimitRPM: 120,
		},
		lease: gatewaylite.LeaseSnapshot{
			LeaseID:        "lease_sg_1",
			UserID:         42,
			Region:         "sg",
			AllocatedCents: 100,
			ExpiresAt:      time.Now().Add(time.Minute).Unix(),
		},
		reportFn: func(event gatewaylite.UsageEvent) {
			reported <- event
		},
	}

	router := gin.New()
	router.Use(ClientRequestID())
	router.POST("/v1/chat/completions", gin.HandlerFunc(NewGatewayLiteAPIKeyAuthMiddleware(stub, "sg", "sg-1", quota, nil, stub)), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	fields, err := client.HGetAll(ctx, "mwtest:lease:42:sg").Result()
	require.NoError(t, err)
	require.EqualValues(t, 0, gatewaylite.ParseInt64Field(fields, "reserved_cents"))
	require.EqualValues(t, 1, gatewaylite.ParseInt64Field(fields, "spent_cents"))

	select {
	case event := <-reported:
		require.Equal(t, "key123", event.KeyID)
		require.Equal(t, int64(42), event.UserID)
		require.Equal(t, "lease_sg_1", event.LeaseID)
		require.Equal(t, int64(1), event.ActualCents)
		require.Equal(t, http.StatusOK, event.Status)
	case <-time.After(time.Second):
		t.Fatal("expected usage report")
	}
}

func TestGatewayLiteAPIKeyAuthMiddlewareUsesRedisFastPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	quota := gatewaylite.NewRedisQuota(client, "fast")
	keyCache := gatewaylite.NewRedisKeyCache(client, "fast")

	secret := "secret-value"
	hash := sha256.Sum256([]byte(secret))
	key := "aiceo_sk_key123_" + secret
	var resolveCount atomic.Int32
	var acquireCount atomic.Int32
	stub := gatewayLiteControlStub{
		key: gatewaylite.KeySnapshot{
			KeyID:          "key123",
			UserID:         42,
			SecretHash:     hex.EncodeToString(hash[:]),
			Status:         service.StatusActive,
			Platform:       service.PlatformOpenAI,
			GroupID:        7,
			GroupName:      "openai",
			Concurrency:    3,
			CacheTTLSecond: 60,
		},
		lease: gatewaylite.LeaseSnapshot{
			LeaseID:        "lease_sg_1",
			UserID:         42,
			Region:         "sg",
			AllocatedCents: 100,
			ExpiresAt:      time.Now().Add(time.Minute).Unix(),
		},
		resolveCount: &resolveCount,
		acquireCount: &acquireCount,
	}

	router := gin.New()
	router.Use(ClientRequestID())
	router.POST("/v1/messages", gin.HandlerFunc(NewGatewayLiteAPIKeyAuthMiddleware(stub, "sg", "sg-1", quota, keyCache, stub)), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
	}

	require.EqualValues(t, 1, resolveCount.Load())
	require.EqualValues(t, 1, acquireCount.Load())
}

func TestGatewayLiteAPIKeyAuthMiddlewareRebalancesWhenAcquireCannotRefill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	quota := gatewaylite.NewRedisQuota(client, "rebalance")

	require.NoError(t, quota.EnsureLease(ctx, gatewaylite.LeaseSnapshot{
		LeaseID:        "lease_sg_old",
		UserID:         42,
		Region:         "sg",
		AllocatedCents: 1,
		SpentCents:     1,
		ExpiresAt:      time.Now().Add(time.Minute).Unix(),
	}))

	secret := "secret-value"
	hash := sha256.Sum256([]byte(secret))
	key := "aiceo_sk_key123_" + secret
	var acquireCount atomic.Int32
	var rebalanceCount atomic.Int32
	stub := gatewayLiteControlStub{
		key: gatewaylite.KeySnapshot{
			KeyID:        "key123",
			UserID:       42,
			SecretHash:   hex.EncodeToString(hash[:]),
			Status:       service.StatusActive,
			Platform:     service.PlatformOpenAI,
			GroupID:      7,
			GroupName:    "openai",
			Concurrency:  3,
			RateLimitRPM: 120,
		},
		acquireResp: &gatewaylite.AcquireLeaseResponse{OK: false, Error: "insufficient_unallocated_balance"},
		rebalanceResp: &gatewaylite.RebalanceLeaseResponse{
			OK: true,
			Lease: gatewaylite.LeaseSnapshot{
				LeaseID:        "lease_sg_rebalanced",
				UserID:         42,
				Region:         "sg",
				AllocatedCents: 50,
				ExpiresAt:      time.Now().Add(time.Minute).Unix(),
			},
			TransferredCents: 50,
			ReleasedRegions:  []string{"us"},
		},
		acquireCount:   &acquireCount,
		rebalanceCount: &rebalanceCount,
	}

	router := gin.New()
	router.Use(ClientRequestID())
	router.POST("/v1/messages", gin.HandlerFunc(NewGatewayLiteAPIKeyAuthMiddleware(stub, "sg", "sg-1", quota, nil, stub)), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.EqualValues(t, 1, acquireCount.Load())
	require.EqualValues(t, 1, rebalanceCount.Load())

	fields, err := client.HGetAll(ctx, "rebalance:lease:42:sg").Result()
	require.NoError(t, err)
	require.Equal(t, "lease_sg_rebalanced", fields["lease_id"])
	require.EqualValues(t, 50, gatewaylite.ParseInt64Field(fields, "allocated_cents"))
	require.EqualValues(t, 0, gatewaylite.ParseInt64Field(fields, "reserved_cents"))
	require.EqualValues(t, 2, gatewaylite.ParseInt64Field(fields, "spent_cents"))
}
