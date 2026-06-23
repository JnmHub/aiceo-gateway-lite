package server

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gatewaylite"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type gatewayLiteSchedulerConfigApplier struct {
	cache          service.SchedulerCache
	gatewayCode    string
	region         string
	runtimeRef     *gatewaylite.RuntimeConfigRef
	healthSettings *gatewaylite.RuntimeHealthSettings
}

func newGatewayLiteSchedulerConfigApplier(cache service.SchedulerCache) *gatewayLiteSchedulerConfigApplier {
	return &gatewayLiteSchedulerConfigApplier{cache: cache}
}

func (a *gatewayLiteSchedulerConfigApplier) WithRuntimeHealthSettings(gatewayCode, region string, settings *gatewaylite.RuntimeHealthSettings) *gatewayLiteSchedulerConfigApplier {
	if a == nil {
		return nil
	}
	a.gatewayCode = strings.TrimSpace(gatewayCode)
	a.region = strings.TrimSpace(region)
	a.healthSettings = settings
	return a
}

func (a *gatewayLiteSchedulerConfigApplier) WithRuntimeConfig(ref *gatewaylite.RuntimeConfigRef, settings *gatewaylite.RuntimeHealthSettings) *gatewayLiteSchedulerConfigApplier {
	if a == nil {
		return nil
	}
	a.runtimeRef = ref
	a.healthSettings = settings
	return a
}

func (a *gatewayLiteSchedulerConfigApplier) ApplyGatewayConfigSnapshot(ctx context.Context, snapshot gatewaylite.GatewayConfigSnapshot) error {
	if a == nil || a.cache == nil {
		return nil
	}
	a.applyRuntimeHealthSettings(snapshot)
	groups := make(map[int64]*service.Group, len(snapshot.Groups))
	for _, item := range snapshot.Groups {
		group := gatewayLiteGroupSnapshotToServiceGroup(item)
		groups[group.ID] = group
		if groupCache, ok := a.cache.(service.SchedulerGroupCache); ok {
			if err := groupCache.SetGroup(ctx, group); err != nil {
				return err
			}
		}
	}

	accounts := make([]service.Account, 0, len(snapshot.Accounts))
	for _, item := range snapshot.Accounts {
		account := gatewayLiteAccountSnapshotToServiceAccount(item, groups)
		if err := a.cache.SetAccount(ctx, &account); err != nil {
			return err
		}
		accounts = append(accounts, account)
	}

	for _, bucket := range gatewayLiteBuildSchedulerBuckets(groups, accounts) {
		filtered := gatewayLiteAccountsForBucket(accounts, bucket)
		if err := a.cache.SetSnapshot(ctx, bucket, filtered); err != nil {
			return err
		}
	}
	return nil
}

func (a *gatewayLiteSchedulerConfigApplier) applyRuntimeHealthSettings(snapshot gatewaylite.GatewayConfigSnapshot) {
	if a == nil || a.healthSettings == nil {
		return
	}
	gatewayCode := a.gatewayCode
	region := a.region
	if a.runtimeRef != nil {
		cfg := a.runtimeRef.Current()
		gatewayCode = cfg.GatewayCode
		region = cfg.Region
	}
	var selected *gatewaylite.GatewayNodeSnapshot
	for i := range snapshot.GatewayNodes {
		node := &snapshot.GatewayNodes[i]
		if gatewayCode != "" && strings.EqualFold(node.Code, gatewayCode) {
			selected = node
			break
		}
		if selected == nil && region != "" && strings.EqualFold(node.Region, region) {
			selected = node
		}
	}
	if selected == nil {
		return
	}
	seconds := gatewayLiteMetadataInt(selected.Metadata, "health_probe_interval_seconds")
	if seconds <= 0 {
		return
	}
	a.healthSettings.SetInterval(time.Duration(seconds) * time.Second)
}

func gatewayLiteGroupSnapshotToServiceGroup(item gatewaylite.GatewayGroupSnapshot) *service.Group {
	group := &service.Group{
		ID:                              item.ID,
		Name:                            item.Name,
		Platform:                        item.Platform,
		Status:                          item.Status,
		IsExclusive:                     item.IsExclusive,
		SubscriptionType:                item.SubscriptionType,
		RateMultiplier:                  item.RateMultiplier,
		DailyLimitUSD:                   item.DailyLimitUSD,
		WeeklyLimitUSD:                  item.WeeklyLimitUSD,
		MonthlyLimitUSD:                 item.MonthlyLimitUSD,
		AllowImageGeneration:            item.AllowImageGeneration,
		ImageRateIndependent:            item.ImageRateIndependent,
		ImageRateMultiplier:             item.ImageRateMultiplier,
		ImagePrice1K:                    item.ImagePrice1K,
		ImagePrice2K:                    item.ImagePrice2K,
		ImagePrice4K:                    item.ImagePrice4K,
		ClaudeCodeOnly:                  item.ClaudeCodeOnly,
		FallbackGroupID:                 item.FallbackGroupID,
		FallbackGroupIDOnInvalidRequest: item.FallbackGroupIDOnInvalidRequest,
		ModelRouting:                    item.ModelRouting,
		ModelRoutingEnabled:             item.ModelRoutingEnabled,
		MCPXMLInject:                    item.MCPXMLInject,
		SupportedModelScopes:            item.SupportedModelScopes,
		AllowMessagesDispatch:           item.AllowMessagesDispatch,
		RequireOAuthOnly:                item.RequireOAuthOnly,
		RequirePrivacySet:               item.RequirePrivacySet,
		DefaultMappedModel:              item.DefaultMappedModel,
		MessagesDispatchModelConfig:     gatewayLiteMessagesDispatchConfig(item.MessagesDispatchModelConfig),
		ModelsListConfig:                gatewayLiteModelsListConfig(item.ModelsListConfig),
		RPMLimit:                        item.RPMLimit,
		Hydrated:                        true,
	}
	gatewayLiteApplyGroupConfig(group, item.Config)
	if group.Status == "" {
		group.Status = service.StatusActive
	}
	if group.RateMultiplier == 0 {
		group.RateMultiplier = 1
	}
	if group.ImageRateMultiplier == 0 {
		group.ImageRateMultiplier = 1
	}
	return group
}

func gatewayLiteMetadataInt(metadata map[string]any, key string) int {
	if metadata == nil {
		return 0
	}
	switch value := metadata[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		parsed, _ := value.Int64()
		return int(parsed)
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return 0
		}
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return 0
}

func gatewayLiteAccountSnapshotToServiceAccount(item gatewaylite.GatewayAccountSnapshot, groups map[int64]*service.Group) service.Account {
	accountGroups := make([]service.AccountGroup, 0, len(item.GroupIDs))
	serviceGroups := make([]*service.Group, 0, len(item.GroupIDs))
	for _, groupID := range item.GroupIDs {
		accountGroups = append(accountGroups, service.AccountGroup{AccountID: item.ID, GroupID: groupID})
		if group := groups[groupID]; group != nil {
			serviceGroups = append(serviceGroups, group)
		}
	}
	return service.Account{
		ID:             item.ID,
		Name:           item.Name,
		Platform:       item.Platform,
		Type:           gatewayLiteNormalizeAccountType(item.Type),
		Status:         item.Status,
		Schedulable:    item.Schedulable,
		Concurrency:    item.Concurrency,
		Priority:       item.Priority,
		RateMultiplier: item.RateMultiplier,
		LoadFactor:     item.LoadFactor,
		Credentials:    item.Credentials,
		Extra:          item.Extra,
		GroupIDs:       item.GroupIDs,
		AccountGroups:  accountGroups,
		Groups:         serviceGroups,
	}
}

func gatewayLiteNormalizeAccountType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "api_key", "api-key":
		return service.AccountTypeAPIKey
	default:
		return value
	}
}

func gatewayLiteMessagesDispatchConfig(in gatewaylite.OpenAIMessagesDispatchModelConfig) service.OpenAIMessagesDispatchModelConfig {
	return service.OpenAIMessagesDispatchModelConfig{
		OpusMappedModel:    in.OpusMappedModel,
		SonnetMappedModel:  in.SonnetMappedModel,
		HaikuMappedModel:   in.HaikuMappedModel,
		ExactModelMappings: in.ExactModelMappings,
	}
}

func gatewayLiteModelsListConfig(in gatewaylite.GroupModelsListConfig) service.GroupModelsListConfig {
	return service.GroupModelsListConfig{
		Enabled: in.Enabled,
		Models:  in.Models,
	}
}

func gatewayLiteApplyGroupConfig(group *service.Group, config map[string]any) {
	if group == nil || len(config) == 0 {
		return
	}
	gatewayLiteConfigValue(config, "is_exclusive", &group.IsExclusive)
	gatewayLiteConfigValue(config, "subscription_type", &group.SubscriptionType)
	gatewayLiteConfigValue(config, "rate_multiplier", &group.RateMultiplier)
	gatewayLiteConfigValue(config, "daily_limit_usd", &group.DailyLimitUSD)
	gatewayLiteConfigValue(config, "weekly_limit_usd", &group.WeeklyLimitUSD)
	gatewayLiteConfigValue(config, "monthly_limit_usd", &group.MonthlyLimitUSD)
	gatewayLiteConfigValue(config, "allow_image_generation", &group.AllowImageGeneration)
	gatewayLiteConfigValue(config, "image_rate_independent", &group.ImageRateIndependent)
	gatewayLiteConfigValue(config, "image_rate_multiplier", &group.ImageRateMultiplier)
	gatewayLiteConfigValue(config, "image_price_1k", &group.ImagePrice1K)
	gatewayLiteConfigValue(config, "image_price_2k", &group.ImagePrice2K)
	gatewayLiteConfigValue(config, "image_price_4k", &group.ImagePrice4K)
	gatewayLiteConfigValue(config, "claude_code_only", &group.ClaudeCodeOnly)
	gatewayLiteConfigValue(config, "fallback_group_id", &group.FallbackGroupID)
	gatewayLiteConfigValue(config, "fallback_group_id_on_invalid_request", &group.FallbackGroupIDOnInvalidRequest)
	gatewayLiteConfigValue(config, "model_routing", &group.ModelRouting)
	gatewayLiteConfigValue(config, "model_routing_enabled", &group.ModelRoutingEnabled)
	gatewayLiteConfigValue(config, "mcp_xml_inject", &group.MCPXMLInject)
	gatewayLiteConfigValue(config, "supported_model_scopes", &group.SupportedModelScopes)
	gatewayLiteConfigValue(config, "allow_messages_dispatch", &group.AllowMessagesDispatch)
	gatewayLiteConfigValue(config, "require_oauth_only", &group.RequireOAuthOnly)
	gatewayLiteConfigValue(config, "require_privacy_set", &group.RequirePrivacySet)
	gatewayLiteConfigValue(config, "default_mapped_model", &group.DefaultMappedModel)
	gatewayLiteConfigValue(config, "messages_dispatch_model_config", &group.MessagesDispatchModelConfig)
	gatewayLiteConfigValue(config, "models_list_config", &group.ModelsListConfig)
	gatewayLiteConfigValue(config, "rpm_limit", &group.RPMLimit)
}

func gatewayLiteConfigValue[T any](config map[string]any, key string, out *T) {
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

func gatewayLiteBuildSchedulerBuckets(groups map[int64]*service.Group, accounts []service.Account) []service.SchedulerBucket {
	seen := map[service.SchedulerBucket]struct{}{}
	add := func(bucket service.SchedulerBucket) {
		if bucket.Platform == "" {
			return
		}
		seen[bucket] = struct{}{}
	}
	for _, account := range accounts {
		add(service.SchedulerBucket{GroupID: 0, Platform: account.Platform, Mode: service.SchedulerModeSingle})
		add(service.SchedulerBucket{GroupID: 0, Platform: account.Platform, Mode: service.SchedulerModeForced})
		if account.Platform == service.PlatformAnthropic || account.Platform == service.PlatformGemini {
			add(service.SchedulerBucket{GroupID: 0, Platform: account.Platform, Mode: service.SchedulerModeMixed})
		}
		for _, groupID := range account.GroupIDs {
			platform := account.Platform
			if group := groups[groupID]; group != nil && group.Platform != "" {
				platform = group.Platform
			}
			add(service.SchedulerBucket{GroupID: groupID, Platform: platform, Mode: service.SchedulerModeSingle})
			add(service.SchedulerBucket{GroupID: groupID, Platform: platform, Mode: service.SchedulerModeForced})
			if platform == service.PlatformAnthropic || platform == service.PlatformGemini {
				add(service.SchedulerBucket{GroupID: groupID, Platform: platform, Mode: service.SchedulerModeMixed})
			}
		}
	}
	out := make([]service.SchedulerBucket, 0, len(seen))
	for bucket := range seen {
		out = append(out, bucket)
	}
	return out
}

func gatewayLiteAccountsForBucket(accounts []service.Account, bucket service.SchedulerBucket) []service.Account {
	out := make([]service.Account, 0)
	for _, account := range accounts {
		if account.Platform != bucket.Platform {
			continue
		}
		if bucket.GroupID > 0 && !gatewayLiteAccountHasGroup(account, bucket.GroupID) {
			continue
		}
		out = append(out, account)
	}
	return out
}

func gatewayLiteAccountHasGroup(account service.Account, groupID int64) bool {
	for _, id := range account.GroupIDs {
		if id == groupID {
			return true
		}
	}
	return false
}
