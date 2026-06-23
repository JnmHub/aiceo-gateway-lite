package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gatewaylite"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

const gatewayLiteDefaultLeaseEstimateCents int64 = 1
const gatewayLiteFinalizeTimeout = 800 * time.Millisecond

var gatewayLiteResolveKeyGroup singleflight.Group

type GatewayLiteControlPlane interface {
	ResolveKey(ctx context.Context, req gatewaylite.ResolveKeyRequest) (*gatewaylite.ResolveKeyResponse, error)
	AcquireLease(ctx context.Context, req gatewaylite.AcquireLeaseRequest) (*gatewaylite.AcquireLeaseResponse, error)
	RebalanceLease(ctx context.Context, req gatewaylite.RebalanceLeaseRequest) (*gatewaylite.RebalanceLeaseResponse, error)
}

// NewGatewayLiteAPIKeyAuthMiddleware validates API keys against the control
// plane and uses regional Redis as the hot-path key/lease cache.
func NewGatewayLiteAPIKeyAuthMiddleware(control GatewayLiteControlPlane, region, gatewayCode string, quota *gatewaylite.RedisQuota, keyCache *gatewaylite.RedisKeyCache, usageReporter gatewaylite.UsageReportClient) APIKeyAuthMiddleware {
	redisPrefix := ""
	if quota != nil {
		redisPrefix = quota.Prefix()
	} else if keyCache != nil {
		redisPrefix = keyCache.Prefix()
	} else if reporter, ok := usageReporter.(*gatewaylite.RedisUsageReporter); ok && reporter != nil {
		redisPrefix = reporter.Prefix()
	}
	return NewGatewayLiteAPIKeyAuthMiddlewareWithRuntimeConfig(control, gatewaylite.NewRuntimeConfigRef(gatewaylite.RuntimeConfig{
		Region:      region,
		GatewayCode: gatewayCode,
		RedisPrefix: redisPrefix,
	}), quota, keyCache, usageReporter)
}

func NewGatewayLiteAPIKeyAuthMiddlewareWithRuntimeConfig(control GatewayLiteControlPlane, runtimeRef *gatewaylite.RuntimeConfigRef, quota *gatewaylite.RedisQuota, keyCache *gatewaylite.RedisKeyCache, usageReporter gatewaylite.UsageReportClient) APIKeyAuthMiddleware {
	return APIKeyAuthMiddleware(func(c *gin.Context) {
		startedAt := time.Now()
		runtimeCfg := gatewaylite.NormalizeRuntimeConfig(gatewaylite.RuntimeConfig{})
		if runtimeRef != nil {
			runtimeCfg = runtimeRef.Current()
		}
		region := runtimeCfg.Region
		gatewayCode := runtimeCfg.GatewayCode
		requestQuota := quota
		if quota != nil {
			requestQuota = quota.WithPrefix(runtimeCfg.RedisPrefix)
		}
		requestKeyCache := keyCache
		if keyCache != nil {
			requestKeyCache = keyCache.WithPrefix(runtimeCfg.RedisPrefix)
		}
		requestUsageReporter := usageReporter
		if reporter, ok := usageReporter.(*gatewaylite.RedisUsageReporter); ok && reporter != nil {
			requestUsageReporter = reporter.WithPrefix(runtimeCfg.RedisPrefix)
		}
		rawKey := extractGatewayLiteAPIKey(c)
		if rawKey == "" {
			AbortWithError(c, 401, "API_KEY_REQUIRED", "API key is required in Authorization header (Bearer scheme), x-api-key header, or x-goog-api-key header")
			return
		}

		keyID, secret, ok := splitGatewayLiteAPIKey(rawKey)
		if !ok {
			AbortWithError(c, 401, "INVALID_API_KEY", "Invalid API key")
			return
		}

		billableRequest := gatewayLiteRequestBillable(c)
		key, err := gatewayLiteResolveKey(c.Request.Context(), control, requestKeyCache, keyID, region, !billableRequest)
		if err != nil {
			var rejected gatewayLiteResolveRejectedError
			if errors.As(err, &rejected) {
				abortGatewayLiteResolveRejected(c, rejected.Code)
				return
			}
			AbortWithError(c, 503, "CONTROL_PLANE_UNAVAILABLE", "Failed to validate API key")
			return
		}
		if key == nil || !key.Active() {
			AbortWithError(c, 401, "INVALID_API_KEY", "Invalid API key")
			return
		}
		if !gatewayLiteSecretMatches(secret, key.SecretHash) {
			AbortWithError(c, 401, "INVALID_API_KEY", "Invalid API key")
			return
		}
		if !gatewayLiteGatewayAllowed(*key, region, gatewayCode) {
			AbortWithError(c, 403, "GATEWAY_NOT_ALLOWED", "Gateway is not available for this API key")
			return
		}
		requestModel, err := gatewayLiteRequestModel(c)
		if err != nil {
			AbortWithError(c, 400, "INVALID_REQUEST_BODY", "Invalid request body")
			return
		}
		if !gatewayLiteModelAllowed(requestModel, key.AllowedModels) {
			AbortWithError(c, 403, "MODEL_NOT_ALLOWED", "This model is not allowed by your current plan")
			return
		}

		requestID := gatewayLiteRequestID(c)
		apiKey := gatewayLiteSnapshotToAPIKey(rawKey, *key)
		SetOpsFallbackAPIKey(c, apiKey)
		c.Set(string(ContextKeyAPIKey), apiKey)
		c.Set(string(ContextKeyUser), AuthSubject{
			UserID:      apiKey.User.ID,
			Concurrency: apiKey.User.Concurrency,
		})
		c.Set(string(ContextKeyUserRole), apiKey.User.Role)
		setGroupContext(c, apiKey.Group)
		if !billableRequest {
			c.Request = c.Request.WithContext(gatewaylite.ContextWithRequestMeta(c.Request.Context(), gatewaylite.RequestMeta{
				RequestID: requestID,
				KeyID:     key.KeyID,
				UserID:    key.UserID,
				Region:    region,
				GatewayID: gatewayLiteCurrentGatewayID(region, gatewayCode),
			}))
			c.Next()
			return
		}

		lease, err := gatewayLiteReserveQuota(c.Request.Context(), control, requestQuota, *key, region, requestID)
		if err != nil {
			AbortWithError(c, 503, "QUOTA_UNAVAILABLE", "Failed to reserve quota")
			return
		}
		if lease == nil {
			AbortWithError(c, 429, "INSUFFICIENT_QUOTA", "Insufficient quota")
			return
		}
		if stats := gatewaylite.DefaultRuntimeStats(); stats != nil {
			stats.RecordUser(key.UserID, startedAt)
		}

		c.Set(string(ContextKeyGatewayLiteLease), *lease)
		c.Request = c.Request.WithContext(gatewaylite.ContextWithRequestMeta(c.Request.Context(), gatewaylite.RequestMeta{
			RequestID: requestID,
			KeyID:     key.KeyID,
			UserID:    key.UserID,
			LeaseID:   lease.LeaseID,
			Region:    region,
			GatewayID: gatewayLiteCurrentGatewayID(region, gatewayCode),
		}))
		c.Next()

		gatewayLiteFinalizeUsage(requestUsageReporter, requestQuota, gatewayLiteFinalizeInput{
			RequestID:      requestID,
			KeyID:          key.KeyID,
			UserID:         key.UserID,
			LeaseID:        lease.LeaseID,
			Region:         region,
			GatewayID:      gatewayLiteCurrentGatewayID(region, gatewayCode),
			Method:         c.Request.Method,
			Path:           c.Request.URL.Path,
			Model:          requestModel,
			Status:         c.Writer.Status(),
			EstimatedCents: gatewayLiteDefaultLeaseEstimateCents,
			StartedAt:      startedAt,
			EndedAt:        time.Now(),
			Aborted:        c.IsAborted(),
		})
	})
}

func gatewayLiteRequestBillable(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return true
	}
	method := strings.ToUpper(strings.TrimSpace(c.Request.Method))
	path := strings.TrimRight(c.Request.URL.Path, "/")
	switch method {
	case http.MethodGet, http.MethodHead:
		switch path {
		case "/v1/models", "/v1beta/models", "/antigravity/models", "/antigravity/v1/models", "/antigravity/v1beta/models":
			return false
		}
	}
	return true
}

func gatewayLiteGatewayAllowed(snapshot gatewaylite.KeySnapshot, region, gatewayCode string) bool {
	if !snapshot.GatewayAccessEnforced {
		return true
	}
	current := gatewayLiteCurrentGatewayID(region, gatewayCode)
	for _, gateway := range snapshot.AvailableGateways {
		if current != "" && strings.EqualFold(strings.TrimSpace(gateway.Code), current) {
			return true
		}
		if strings.TrimSpace(gateway.Code) == "" && strings.EqualFold(strings.TrimSpace(gateway.Region), strings.TrimSpace(region)) {
			return true
		}
	}
	return false
}

func gatewayLiteRequestModel(c *gin.Context) (string, error) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return "", nil
	}
	method := strings.ToUpper(strings.TrimSpace(c.Request.Method))
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch {
		return "", nil
	}
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "json") {
		return "", nil
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return "", err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if len(bytes.TrimSpace(body)) == 0 {
		return "", nil
	}
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", nil
	}
	return strings.TrimSpace(payload.Model), nil
}

func gatewayLiteModelAllowed(model string, allowedModels []string) bool {
	model = strings.TrimSpace(model)
	if model == "" || len(allowedModels) == 0 {
		return true
	}
	for _, pattern := range allowedModels {
		if gatewayLiteModelPatternMatch(model, pattern) {
			return true
		}
	}
	return false
}

func gatewayLiteModelPatternMatch(model, pattern string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	if pattern == "*" || model == pattern {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return false
	}
	parts := strings.Split(pattern, "*")
	cursor := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		pos := strings.Index(model[cursor:], part)
		if pos < 0 {
			return false
		}
		if i == 0 && !strings.HasPrefix(pattern, "*") && pos != 0 {
			return false
		}
		cursor += pos + len(part)
	}
	if !strings.HasSuffix(pattern, "*") && len(parts) > 0 {
		last := parts[len(parts)-1]
		return last == "" || strings.HasSuffix(model, last)
	}
	return true
}

func gatewayLiteCurrentGatewayID(region, gatewayCode string) string {
	if strings.TrimSpace(gatewayCode) != "" {
		return strings.TrimSpace(gatewayCode)
	}
	return strings.TrimSpace(region)
}

func extractGatewayLiteAPIKey(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}
	if key := strings.TrimSpace(c.GetHeader("x-api-key")); key != "" {
		return key
	}
	return strings.TrimSpace(c.GetHeader("x-goog-api-key"))
}

func splitGatewayLiteAPIKey(raw string) (keyID string, secret string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	parts := strings.Split(raw, "_")
	if len(parts) < 4 || (parts[0] != "aiceo" && parts[0] != "fm") || parts[1] != "sk" {
		return "", "", false
	}
	keyID = strings.Join(parts[2:len(parts)-1], "_")
	secret = parts[len(parts)-1]
	return keyID, secret, keyID != "" && secret != ""
}

func gatewayLiteSecretMatches(secret, expectedHash string) bool {
	sum := sha256.Sum256([]byte(secret))
	actual := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(actual), []byte(strings.ToLower(expectedHash))) == 1
}

func gatewayLiteResolveKey(ctx context.Context, control GatewayLiteControlPlane, cache *gatewaylite.RedisKeyCache, keyID, region string, billingExempt bool) (*gatewaylite.KeySnapshot, error) {
	if cached, ok, err := cache.Get(ctx, keyID, region); err != nil {
		return nil, err
	} else if ok {
		return cached, nil
	}

	resolveGroupKey := strings.Join([]string{keyID, region, boolString(billingExempt)}, "\x00")
	value, err, _ := gatewayLiteResolveKeyGroup.Do(resolveGroupKey, func() (any, error) {
		if cached, ok, err := cache.Get(ctx, keyID, region); err != nil {
			return nil, err
		} else if ok {
			return cached, nil
		}
		resp, err := control.ResolveKey(ctx, gatewaylite.ResolveKeyRequest{
			KeyID:         keyID,
			Region:        region,
			BillingExempt: billingExempt,
		})
		if err != nil {
			return nil, err
		}
		if resp == nil || !resp.OK {
			code := ""
			if resp != nil {
				code = resp.Error
			}
			return nil, gatewayLiteResolveRejectedError{Code: code}
		}
		if resp.Key.Active() {
			if err := cache.Set(ctx, resp.Key, region); err != nil {
				return nil, err
			}
		}
		return &resp.Key, nil
	})
	if err != nil {
		return nil, err
	}
	key, ok := value.(*gatewaylite.KeySnapshot)
	if !ok || key == nil {
		return nil, errors.New("control plane returned invalid key snapshot")
	}
	return key, nil
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

type gatewayLiteResolveRejectedError struct {
	Code string
}

func (e gatewayLiteResolveRejectedError) Error() string {
	if strings.TrimSpace(e.Code) == "" {
		return "control plane rejected key"
	}
	return "control plane rejected key: " + e.Code
}

func abortGatewayLiteResolveRejected(c *gin.Context, code string) {
	switch strings.TrimSpace(code) {
	case "insufficient_balance":
		AbortWithError(c, 429, "INSUFFICIENT_QUOTA", "Insufficient quota")
	case "subscription_required":
		AbortWithError(c, 403, "SUBSCRIPTION_REQUIRED", "An active subscription is required")
	case "phone_verification_required":
		AbortWithError(c, 403, "PHONE_VERIFICATION_REQUIRED", "Phone verification is required")
	case "linux_do_binding_required":
		AbortWithError(c, 403, "LINUX_DO_BINDING_REQUIRED", "Linux.do account binding is required")
	case "github_binding_required":
		AbortWithError(c, 403, "GITHUB_BINDING_REQUIRED", "GitHub account binding is required")
	default:
		AbortWithError(c, 401, "INVALID_API_KEY", "Invalid API key")
	}
}

func gatewayLiteReserveQuota(ctx context.Context, control GatewayLiteControlPlane, quota *gatewaylite.RedisQuota, key gatewaylite.KeySnapshot, region, requestID string) (*gatewaylite.LeaseSnapshot, error) {
	localLease, ok, err := quota.LoadLease(ctx, key.UserID, region)
	if err != nil {
		return nil, err
	}
	if ok && localLease.AvailableCents() >= gatewayLiteDefaultLeaseEstimateCents {
		reserved, err := quota.Reserve(ctx, gatewaylite.ReserveRequest{
			RequestID:          requestID,
			KeyID:              key.KeyID,
			UserID:             key.UserID,
			Region:             region,
			LeaseID:            localLease.LeaseID,
			EstimatedCostCents: gatewayLiteDefaultLeaseEstimateCents,
		})
		if err != nil {
			return nil, err
		}
		if reserved {
			return &localLease, nil
		}
	}

	lease, err := gatewayLiteAcquireOrRebalanceLease(ctx, control, key.UserID, region, gatewayLiteDefaultLeaseEstimateCents)
	if err != nil {
		return nil, err
	}
	if lease == nil || lease.AvailableCents() < gatewayLiteDefaultLeaseEstimateCents || lease.Expired(time.Now()) {
		return nil, nil
	}
	if err := quota.EnsureLease(ctx, *lease); err != nil {
		return nil, err
	}
	reserved, err := quota.Reserve(ctx, gatewaylite.ReserveRequest{
		RequestID:          requestID,
		KeyID:              key.KeyID,
		UserID:             key.UserID,
		Region:             region,
		LeaseID:            lease.LeaseID,
		EstimatedCostCents: gatewayLiteDefaultLeaseEstimateCents,
	})
	if err != nil {
		return nil, err
	}
	if !reserved {
		return nil, nil
	}
	return lease, nil
}

func gatewayLiteAcquireOrRebalanceLease(ctx context.Context, control GatewayLiteControlPlane, userID int64, region string, estimatedCents int64) (*gatewaylite.LeaseSnapshot, error) {
	lease, err := control.AcquireLease(ctx, gatewaylite.AcquireLeaseRequest{
		UserID:             userID,
		Region:             region,
		EstimatedCostCents: estimatedCents,
		Reason:             "gateway_request",
	})
	if err != nil {
		return nil, err
	}
	if lease != nil && lease.OK {
		return &lease.Lease, nil
	}

	rebalance, err := control.RebalanceLease(ctx, gatewaylite.RebalanceLeaseRequest{
		UserID:             userID,
		Region:             region,
		EstimatedCostCents: estimatedCents,
		Reason:             "gateway_local_lease_exhausted",
	})
	if err != nil {
		return nil, err
	}
	if rebalance == nil || !rebalance.OK {
		return nil, nil
	}
	return &rebalance.Lease, nil
}

func gatewayLiteSnapshotToAPIKey(rawKey string, snapshot gatewaylite.KeySnapshot) *service.APIKey {
	groupID := snapshot.GroupID
	now := time.Now()
	group := gatewayLiteSnapshotGroupToServiceGroup(snapshot)
	user := &service.User{
		ID:          snapshot.UserID,
		Role:        service.RoleUser,
		Status:      service.StatusActive,
		Concurrency: snapshot.Concurrency,
		RPMLimit:    snapshot.RateLimitRPM,
		TPMLimit:    snapshot.RateLimitTPM,
	}
	if user.Concurrency <= 0 {
		user.Concurrency = 1
	}
	return &service.APIKey{
		ID:            0,
		UserID:        snapshot.UserID,
		Key:           rawKey,
		Name:          snapshot.KeyID,
		GroupID:       &groupID,
		Status:        service.StatusAPIKeyActive,
		CreatedAt:     now,
		UpdatedAt:     now,
		User:          user,
		Group:         group,
		AllowedModels: append([]string(nil), snapshot.AllowedModels...),
	}
}

func gatewayLiteSnapshotGroupToServiceGroup(snapshot gatewaylite.KeySnapshot) *service.Group {
	if snapshot.Group != nil && snapshot.Group.ID > 0 {
		group := &service.Group{
			ID:                              snapshot.Group.ID,
			Name:                            snapshot.Group.Name,
			Platform:                        snapshot.Group.Platform,
			Status:                          snapshot.Group.Status,
			IsExclusive:                     snapshot.Group.IsExclusive,
			SubscriptionType:                snapshot.Group.SubscriptionType,
			RateMultiplier:                  snapshot.Group.RateMultiplier,
			DailyLimitUSD:                   snapshot.Group.DailyLimitUSD,
			WeeklyLimitUSD:                  snapshot.Group.WeeklyLimitUSD,
			MonthlyLimitUSD:                 snapshot.Group.MonthlyLimitUSD,
			AllowImageGeneration:            snapshot.Group.AllowImageGeneration,
			ImageRateIndependent:            snapshot.Group.ImageRateIndependent,
			ImageRateMultiplier:             snapshot.Group.ImageRateMultiplier,
			ImagePrice1K:                    snapshot.Group.ImagePrice1K,
			ImagePrice2K:                    snapshot.Group.ImagePrice2K,
			ImagePrice4K:                    snapshot.Group.ImagePrice4K,
			ClaudeCodeOnly:                  snapshot.Group.ClaudeCodeOnly,
			FallbackGroupID:                 snapshot.Group.FallbackGroupID,
			FallbackGroupIDOnInvalidRequest: snapshot.Group.FallbackGroupIDOnInvalidRequest,
			ModelRouting:                    snapshot.Group.ModelRouting,
			ModelRoutingEnabled:             snapshot.Group.ModelRoutingEnabled,
			MCPXMLInject:                    snapshot.Group.MCPXMLInject,
			SupportedModelScopes:            snapshot.Group.SupportedModelScopes,
			AllowMessagesDispatch:           snapshot.Group.AllowMessagesDispatch,
			RequireOAuthOnly:                snapshot.Group.RequireOAuthOnly,
			RequirePrivacySet:               snapshot.Group.RequirePrivacySet,
			DefaultMappedModel:              snapshot.Group.DefaultMappedModel,
			MessagesDispatchModelConfig: service.OpenAIMessagesDispatchModelConfig{
				OpusMappedModel:    snapshot.Group.MessagesDispatchModelConfig.OpusMappedModel,
				SonnetMappedModel:  snapshot.Group.MessagesDispatchModelConfig.SonnetMappedModel,
				HaikuMappedModel:   snapshot.Group.MessagesDispatchModelConfig.HaikuMappedModel,
				ExactModelMappings: snapshot.Group.MessagesDispatchModelConfig.ExactModelMappings,
			},
			ModelsListConfig: service.GroupModelsListConfig{
				Enabled: snapshot.Group.ModelsListConfig.Enabled,
				Models:  snapshot.Group.ModelsListConfig.Models,
			},
			RPMLimit: snapshot.Group.RPMLimit,
			Hydrated: true,
		}
		gatewayLiteApplySnapshotGroupConfig(group, snapshot.Group.Config)
		gatewayLiteNormalizeSnapshotGroup(group, snapshot)
		return group
	}
	group := &service.Group{
		ID:             snapshot.GroupID,
		Name:           snapshot.GroupName,
		Platform:       snapshot.Platform,
		Status:         service.StatusActive,
		RateMultiplier: 1,
		Hydrated:       true,
	}
	gatewayLiteNormalizeSnapshotGroup(group, snapshot)
	return group
}

func gatewayLiteNormalizeSnapshotGroup(group *service.Group, snapshot gatewaylite.KeySnapshot) {
	if group.ID <= 0 {
		group.ID = snapshot.GroupID
	}
	if group.Name == "" {
		group.Name = snapshot.GroupName
	}
	if group.Platform == "" {
		group.Platform = snapshot.Platform
	}
	if group.Status == "" {
		group.Status = service.StatusActive
	}
	if group.RateMultiplier == 0 {
		group.RateMultiplier = 1
	}
	if group.ImageRateMultiplier == 0 {
		group.ImageRateMultiplier = 1
	}
}

func gatewayLiteApplySnapshotGroupConfig(group *service.Group, config map[string]any) {
	if group == nil || len(config) == 0 {
		return
	}
	gatewayLiteSnapshotConfigValue(config, "is_exclusive", &group.IsExclusive)
	gatewayLiteSnapshotConfigValue(config, "subscription_type", &group.SubscriptionType)
	gatewayLiteSnapshotConfigValue(config, "rate_multiplier", &group.RateMultiplier)
	gatewayLiteSnapshotConfigValue(config, "daily_limit_usd", &group.DailyLimitUSD)
	gatewayLiteSnapshotConfigValue(config, "weekly_limit_usd", &group.WeeklyLimitUSD)
	gatewayLiteSnapshotConfigValue(config, "monthly_limit_usd", &group.MonthlyLimitUSD)
	gatewayLiteSnapshotConfigValue(config, "allow_image_generation", &group.AllowImageGeneration)
	gatewayLiteSnapshotConfigValue(config, "image_rate_independent", &group.ImageRateIndependent)
	gatewayLiteSnapshotConfigValue(config, "image_rate_multiplier", &group.ImageRateMultiplier)
	gatewayLiteSnapshotConfigValue(config, "image_price_1k", &group.ImagePrice1K)
	gatewayLiteSnapshotConfigValue(config, "image_price_2k", &group.ImagePrice2K)
	gatewayLiteSnapshotConfigValue(config, "image_price_4k", &group.ImagePrice4K)
	gatewayLiteSnapshotConfigValue(config, "claude_code_only", &group.ClaudeCodeOnly)
	gatewayLiteSnapshotConfigValue(config, "fallback_group_id", &group.FallbackGroupID)
	gatewayLiteSnapshotConfigValue(config, "fallback_group_id_on_invalid_request", &group.FallbackGroupIDOnInvalidRequest)
	gatewayLiteSnapshotConfigValue(config, "model_routing", &group.ModelRouting)
	gatewayLiteSnapshotConfigValue(config, "model_routing_enabled", &group.ModelRoutingEnabled)
	gatewayLiteSnapshotConfigValue(config, "mcp_xml_inject", &group.MCPXMLInject)
	gatewayLiteSnapshotConfigValue(config, "supported_model_scopes", &group.SupportedModelScopes)
	gatewayLiteSnapshotConfigValue(config, "allow_messages_dispatch", &group.AllowMessagesDispatch)
	gatewayLiteSnapshotConfigValue(config, "require_oauth_only", &group.RequireOAuthOnly)
	gatewayLiteSnapshotConfigValue(config, "require_privacy_set", &group.RequirePrivacySet)
	gatewayLiteSnapshotConfigValue(config, "default_mapped_model", &group.DefaultMappedModel)
	gatewayLiteSnapshotConfigValue(config, "messages_dispatch_model_config", &group.MessagesDispatchModelConfig)
	gatewayLiteSnapshotConfigValue(config, "models_list_config", &group.ModelsListConfig)
	gatewayLiteSnapshotConfigValue(config, "rpm_limit", &group.RPMLimit)
}

func gatewayLiteSnapshotConfigValue[T any](config map[string]any, key string, out *T) {
	raw, ok := config[key]
	if !ok || raw == nil || out == nil {
		return
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return
	}
	_ = json.Unmarshal(payload, out)
}

type gatewayLiteFinalizeInput struct {
	RequestID      string
	KeyID          string
	UserID         int64
	LeaseID        string
	Region         string
	GatewayID      string
	Method         string
	Path           string
	Model          string
	Status         int
	EstimatedCents int64
	StartedAt      time.Time
	EndedAt        time.Time
	Aborted        bool
}

func gatewayLiteRequestID(c *gin.Context) string {
	if c != nil && c.Request != nil {
		if id, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id)
		}
	}
	return uuid.New().String()
}

func gatewayLiteFinalizeUsage(reporter gatewaylite.UsageReportClient, quota *gatewaylite.RedisQuota, in gatewayLiteFinalizeInput) {
	if in.Status == 0 {
		in.Status = http.StatusOK
	}
	actualCents := in.EstimatedCents
	shouldRefund := in.Aborted || in.Status >= http.StatusBadRequest

	ctx, cancel := context.WithTimeout(context.Background(), gatewayLiteFinalizeTimeout)
	defer cancel()

	if shouldRefund {
		if err := quota.Refund(ctx, in.RequestID); err != nil {
			log.Printf("gateway-lite: failed to refund quota request_id=%s: %v", in.RequestID, err)
		}
		actualCents = 0
	} else {
		if err := quota.Commit(ctx, gatewaylite.UsageCommit{RequestID: in.RequestID, ActualCents: actualCents}); err != nil {
			log.Printf("gateway-lite: failed to commit quota request_id=%s: %v", in.RequestID, err)
		}
	}

	event := gatewaylite.UsageEvent{
		RequestID:       in.RequestID,
		UserID:          in.UserID,
		KeyID:           in.KeyID,
		LeaseID:         in.LeaseID,
		Region:          in.Region,
		GatewayID:       gatewayLiteCurrentGatewayID(in.Region, in.GatewayID),
		Protocol:        "openai_compatible",
		Method:          in.Method,
		Path:            in.Path,
		Model:           in.Model,
		Status:          in.Status,
		EstimatedCents:  in.EstimatedCents,
		ActualCents:     actualCents,
		LatencyMillis:   in.EndedAt.Sub(in.StartedAt).Milliseconds(),
		StartedAtMillis: in.StartedAt.UnixMilli(),
		EndedAtMillis:   in.EndedAt.UnixMilli(),
	}
	go func() {
		if reporter == nil {
			return
		}
		reportCtx, reportCancel := context.WithTimeout(context.Background(), gatewayLiteFinalizeTimeout)
		defer reportCancel()
		if err := reporter.ReportUsage(reportCtx, event); err != nil {
			log.Printf("gateway-lite: failed to report usage request_id=%s: %v", event.RequestID, err)
		}
	}()
}
