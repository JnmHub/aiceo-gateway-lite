package server

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/gatewaylite"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/routes"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/web"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const frameSrcRefreshTimeout = 5 * time.Second

// SetupRouter 配置路由器中间件和路由
func SetupRouter(
	r *gin.Engine,
	handlers *handler.Handlers,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	apiKeyAuth middleware2.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	opsService *service.OpsService,
	settingService *service.SettingService,
	pricingService *service.PricingService,
	cfg *config.Config,
	redisClient *redis.Client,
) *gin.Engine {
	if cfg.RunMode == config.RunModeGatewayLite {
		var gatewayHandler *handler.GatewayHandler
		if handlers != nil {
			gatewayHandler = handlers.Gateway
		}
		apiKeyAuth = gatewayLiteAPIKeyAuthOrFallback(apiKeyAuth, redisClient, cfg, pricingService, gatewayHandler, handlers)
		setupCommonMiddlewareAndFrontend(r, settingService, cfg)
		registerRoutes(r, handlers, jwtAuth, adminAuth, apiKeyAuth, apiKeyService, subscriptionService, opsService, settingService, cfg, redisClient)
		return r
	}

	setupCommonMiddlewareAndFrontend(r, settingService, cfg)

	// 注册路由
	registerRoutes(r, handlers, jwtAuth, adminAuth, apiKeyAuth, apiKeyService, subscriptionService, opsService, settingService, cfg, redisClient)

	return r
}

func setupCommonMiddlewareAndFrontend(r *gin.Engine, settingService *service.SettingService, cfg *config.Config) {
	// 缓存 iframe 页面的 origin 列表，用于动态注入 CSP frame-src
	var cachedFrameOrigins atomic.Pointer[[]string]
	emptyOrigins := []string{}
	cachedFrameOrigins.Store(&emptyOrigins)

	refreshFrameOrigins := func() {
		if settingService == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), frameSrcRefreshTimeout)
		defer cancel()
		origins, err := settingService.GetFrameSrcOrigins(ctx)
		if err != nil {
			// 获取失败时保留已有缓存，避免 frame-src 被意外清空
			return
		}
		cachedFrameOrigins.Store(&origins)
	}
	refreshFrameOrigins() // 启动时初始化

	// 应用中间件
	r.Use(middleware2.RequestLogger())
	r.Use(middleware2.Logger())
	r.Use(middleware2.CORS(cfg.CORS))
	r.Use(middleware2.SecurityHeaders(cfg.Security.CSP, func() []string {
		if p := cachedFrameOrigins.Load(); p != nil {
			return *p
		}
		return nil
	}))

	// Serve embedded frontend with settings injection if available
	if web.HasEmbeddedFrontend() {
		frontendServer, err := web.NewFrontendServer(settingService)
		if err != nil {
			log.Printf("Warning: Failed to create frontend server with settings injection: %v, using legacy mode", err)
			r.Use(web.ServeEmbeddedFrontend())
			if settingService != nil {
				settingService.SetOnUpdateCallback(refreshFrameOrigins)
			}
		} else {
			// Register combined callback: invalidate HTML cache + refresh frame origins
			if settingService != nil {
				settingService.SetOnUpdateCallback(func() {
					frontendServer.InvalidateCache()
					refreshFrameOrigins()
				})
			}
			r.Use(frontendServer.Middleware())
		}
	} else if settingService != nil {
		settingService.SetOnUpdateCallback(refreshFrameOrigins)
	}
}

func gatewayLiteAPIKeyAuthOrFallback(fallback middleware2.APIKeyAuthMiddleware, redisClient *redis.Client, cfg *config.Config, pricingService *service.PricingService, gatewayHandler *handler.GatewayHandler, handlers *handler.Handlers) middleware2.APIKeyAuthMiddleware {
	liteCfg := config.GatewayLiteConfig{}
	if cfg != nil {
		liteCfg = cfg.GatewayLite
	}
	baseURL := gatewayLiteStringFromEnv("GATEWAY_LITE_CONTROL_PLANE_URL", liteCfg.ControlPlaneURL)
	region := gatewayLiteStringFromEnv("GATEWAY_LITE_REGION", liteCfg.Region)
	if region == "" {
		region = "default"
	}
	timeout := time.Duration(gatewayLiteIntFromEnv("GATEWAY_LITE_CONTROL_PLANE_TIMEOUT_MS", liteCfg.ControlPlaneTimeoutMS, 300)) * time.Millisecond
	var client *gatewaylite.ControlPlaneClient
	if baseURL == "" {
		log.Printf("gateway-lite: control plane URL is empty; initializing runtime in pending mode")
	} else {
		var err error
		client, err = gatewaylite.NewControlPlaneClient(baseURL, gatewayLiteStringFromEnv("GATEWAY_LITE_CONTROL_PLANE_TOKEN", liteCfg.ControlPlaneToken), timeout)
		if err != nil {
			log.Printf("gateway-lite: invalid control plane client config: %v; initializing runtime in pending mode", err)
		}
	}
	clientRef := gatewaylite.NewControlPlaneClientRef(client)
	gatewaylite.SetDefaultControlPlaneClientRef(clientRef)
	redisPrefix := gatewayLiteStringFromEnv("GATEWAY_LITE_REDIS_PREFIX", liteCfg.RedisPrefix)
	quota := gatewaylite.NewRedisQuota(redisClient, redisPrefix)
	keyCache := gatewaylite.NewRedisKeyCache(redisClient, redisPrefix)
	usageReporter := gatewaylite.NewRedisUsageReporter(redisClient, redisPrefix, clientRef)
	gatewaylite.SetDefaultUsageReporter(usageReporter)
	runtimeStats := gatewaylite.NewRuntimeStats()
	gatewaylite.SetDefaultRuntimeStats(runtimeStats)
	usageReporter.Start(context.Background())
	usageQueuePendingThreshold := gatewayLiteInt64FromEnv("GATEWAY_LITE_USAGE_QUEUE_PENDING_ALERT_THRESHOLD", liteCfg.UsageQueuePendingAlertThreshold, 1000)
	usageQueueDeadThreshold := gatewayLiteInt64FromEnv("GATEWAY_LITE_USAGE_QUEUE_DEAD_ALERT_THRESHOLD", liteCfg.UsageQueueDeadAlertThreshold, 1)
	gatewayCode := gatewayLiteStringFromEnv("GATEWAY_LITE_GATEWAY_CODE", liteCfg.GatewayCode)
	if gatewayCode == "" {
		gatewayCode = region
	}
	runtimeHealthInterval := gatewayLiteDurationFromEnv("GATEWAY_LITE_RUNTIME_HEALTH_INTERVAL_SECONDS", liteCfg.RuntimeHealthIntervalSeconds, 15*time.Second)
	configSyncInterval := time.Duration(gatewayLiteIntFromEnv("GATEWAY_LITE_CONFIG_SYNC_INTERVAL_SECONDS", liteCfg.ConfigSyncIntervalSeconds, 30)) * time.Second
	invalidationInterval := time.Duration(gatewayLiteIntFromEnv("GATEWAY_LITE_CACHE_INVALIDATION_INTERVAL_SECONDS", liteCfg.CacheInvalidationIntervalSeconds, 5)) * time.Second
	runtimeActiveWindow := gatewayLiteDurationFromEnv("GATEWAY_LITE_RUNTIME_ACTIVE_WINDOW_SECONDS", liteCfg.RuntimeActiveWindowSeconds, 5*time.Minute)
	runtimeRef := gatewaylite.NewRuntimeConfigRef(gatewaylite.RuntimeConfig{
		Region:                           region,
		GatewayCode:                      gatewayCode,
		RedisPrefix:                      redisPrefix,
		RuntimeHealthIntervalSeconds:     int(runtimeHealthInterval / time.Second),
		RuntimeActiveWindowSeconds:       int(runtimeActiveWindow / time.Second),
		ConfigSyncIntervalSeconds:        int(configSyncInterval / time.Second),
		CacheInvalidationIntervalSeconds: int(invalidationInterval / time.Second),
		UsageQueuePendingAlertThreshold:  usageQueuePendingThreshold,
		UsageQueueDeadAlertThreshold:     usageQueueDeadThreshold,
	})
	gatewaylite.SetDefaultRuntimeConfigRef(runtimeRef)
	runtimeHealthSettings := gatewaylite.NewRuntimeHealthSettings(runtimeHealthInterval)
	gatewaylite.SetDefaultRuntimeHealthSettings(runtimeHealthSettings)
	configCache := gatewaylite.NewRedisConfigCache(redisClient, redisPrefix)
	configSyncer := gatewaylite.NewConfigSyncer(clientRef, configCache, region, configSyncInterval)
	configSyncer.SetApplier(newGatewayLiteSchedulerConfigApplier(repository.NewSchedulerCache(redisClient)).
		WithRuntimeConfig(runtimeRef, runtimeHealthSettings))
	gatewaylite.SetDefaultConfigSyncer(configSyncer)
	gatewaylite.SetDefaultRedisQuota(quota)
	gatewaylite.SetDefaultRedisKeyCache(keyCache)
	gatewaylite.SetDefaultRedisConfigCache(configCache)
	configSyncer.Start(context.Background())
	availableModels := gatewayLiteStringSliceFromEnv("GATEWAY_LITE_AVAILABLE_MODELS", liteCfg.AvailableModels)
	runtimeHealthMonitor := gatewaylite.NewRuntimeHealthMonitor(clientRef, runtimeStats, region, gatewayCode, runtimeHealthInterval, runtimeActiveWindow).
		WithIntervalSource(runtimeHealthSettings).
		WithUsageQueue(usageReporter, usageQueuePendingThreshold, usageQueueDeadThreshold).
		WithAvailableModels(availableModels).
		WithAvailableModelsProvider(gatewayLiteAvailableModelsProvider{
			staticModels: availableModels,
			handler:      gatewayHandler,
		}).
		WithModelPriceProvider(gatewayLiteModelPriceProvider{pricingService: pricingService}).
		WithRuntimeMetricsProvider(handlers).
		WithRuntimeMetricsProvider(gatewayLiteRedisRuntimeMetricsProvider{client: redisClient})
	gatewaylite.SetDefaultRuntimeHealthMonitor(runtimeHealthMonitor)
	runtimeHealthMonitor.Start(context.Background())
	cacheInvalidationSyncer := gatewaylite.NewCacheInvalidationSyncer(clientRef, configCache, keyCache, configSyncer, region, gatewayCode, invalidationInterval)
	gatewaylite.SetDefaultCacheInvalidationSyncer(cacheInvalidationSyncer)
	cacheInvalidationSyncer.Start(context.Background())
	if quota.Enabled() {
		log.Printf("gateway-lite: remote key/quota auth enabled for region=%s with local Redis key and quota cache", region)
	} else {
		log.Printf("gateway-lite: remote key/quota auth enabled for region=%s without local Redis key/quota cache", region)
	}
	gatewayAuth := middleware2.NewGatewayLiteAPIKeyAuthMiddlewareWithRuntimeConfig(clientRef, runtimeRef, quota, keyCache, usageReporter)
	if !clientRef.Configured() {
		log.Printf("gateway-lite: control plane client is pending; stock API key auth will be used until config sync")
	}
	return middleware2.APIKeyAuthMiddleware(func(c *gin.Context) {
		if clientRef.Configured() {
			gatewayAuth(c)
			return
		}
		fallback(c)
	})
}

func gatewayLiteStringFromEnv(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

type gatewayLiteAvailableModelsProvider struct {
	staticModels []string
	handler      *handler.GatewayHandler
}

func (p gatewayLiteAvailableModelsProvider) AvailableModels(ctx context.Context) []string {
	if p.handler == nil {
		return cloneGatewayLiteStringSlice(p.staticModels)
	}
	return mergeGatewayLiteStringSlices(p.staticModels, p.handler.GlobalAvailableModels(ctx))
}

func mergeGatewayLiteStringSlices(values ...[]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, list := range values {
		for _, item := range list {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

func cloneGatewayLiteStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

type gatewayLiteRedisRuntimeMetricsProvider struct {
	client *redis.Client
}

func (p gatewayLiteRedisRuntimeMetricsProvider) RuntimeMetrics(context.Context) map[string]any {
	metrics := map[string]any{}
	if p.client == nil {
		return metrics
	}
	stats := p.client.PoolStats()
	if stats == nil {
		return metrics
	}
	metrics["redis_pool_hits_total"] = stats.Hits
	metrics["redis_pool_misses_total"] = stats.Misses
	metrics["redis_pool_timeouts_total"] = stats.Timeouts
	metrics["redis_pool_total_conns"] = stats.TotalConns
	metrics["redis_pool_idle_conns"] = stats.IdleConns
	metrics["redis_pool_stale_conns_total"] = stats.StaleConns
	return metrics
}

func gatewayLiteIntFromEnv(name string, configured int, fallback int) int {
	raw := os.Getenv(name)
	if raw != "" {
		value, err := strconv.Atoi(raw)
		if err == nil && value > 0 {
			return value
		}
	}
	if configured > 0 {
		return configured
	}
	return fallback
}

func gatewayLiteDurationFromEnv(name string, configuredSeconds int, fallback time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	if configuredSeconds > 0 {
		return time.Duration(configuredSeconds) * time.Second
	}
	return fallback
}

func gatewayLiteInt64FromEnv(name string, configured int64, fallback int64) int64 {
	raw := os.Getenv(name)
	if raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && value > 0 {
			return value
		}
	}
	if configured > 0 {
		return configured
	}
	return fallback
}

func gatewayLiteStringSliceFromEnv(name string, configured []string) []string {
	raw := os.Getenv(name)
	if raw == "" {
		return configured
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// registerRoutes 注册所有 HTTP 路由
func registerRoutes(
	r *gin.Engine,
	h *handler.Handlers,
	jwtAuth middleware2.JWTAuthMiddleware,
	adminAuth middleware2.AdminAuthMiddleware,
	apiKeyAuth middleware2.APIKeyAuthMiddleware,
	apiKeyService *service.APIKeyService,
	subscriptionService *service.SubscriptionService,
	opsService *service.OpsService,
	settingService *service.SettingService,
	cfg *config.Config,
	redisClient *redis.Client,
) {
	// 通用路由（健康检查、状态等）
	routes.RegisterCommonRoutes(r)

	if cfg.RunMode == config.RunModeGatewayLite {
		v1 := r.Group("/api/v1")
		routes.RegisterGatewayLiteAuthRoutes(v1, h, jwtAuth, redisClient, settingService)
		routes.RegisterGatewayLiteAdminRoutes(v1, h, adminAuth, settingService, cfg)
		routes.RegisterGatewayRoutes(r, h, apiKeyAuth, apiKeyService, subscriptionService, opsService, settingService, cfg)
		return
	}

	// API v1
	v1 := r.Group("/api/v1")

	// 注册各模块路由
	routes.RegisterAuthRoutes(v1, h, jwtAuth, redisClient, settingService)
	routes.RegisterUserRoutes(v1, h, jwtAuth, settingService)
	routes.RegisterAdminRoutes(v1, h, adminAuth, settingService)
	routes.RegisterGatewayRoutes(r, h, apiKeyAuth, apiKeyService, subscriptionService, opsService, settingService, cfg)
	routes.RegisterPaymentRoutes(v1, h.Payment, h.PaymentWebhook, h.Admin.Payment, jwtAuth, adminAuth, settingService)

	handler.RegisterPageRoutes(v1, cfg.Pricing.DataDir, gin.HandlerFunc(jwtAuth), gin.HandlerFunc(adminAuth), settingService)
}
