package server

import (
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/gatewaylite"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type gatewayLiteModelPriceProvider struct {
	pricingService *service.PricingService
}

func (p gatewayLiteModelPriceProvider) GatewayModelPrices(models []string, gatewayCode string) []gatewaylite.GatewayModelPrice {
	if p.pricingService == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]gatewaylite.GatewayModelPrice, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		key := strings.ToLower(model)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		pricing := p.pricingService.GetModelPricing(model)
		if pricing == nil {
			continue
		}
		cacheCreation := pricing.CacheCreationInputTokenCost
		if cacheCreation == 0 {
			cacheCreation = pricing.CacheCreationInputTokenCostAbove1hr
		}
		out = append(out, gatewaylite.GatewayModelPrice{
			GatewayCode:                  strings.TrimSpace(gatewayCode),
			Model:                        model,
			Provider:                     strings.TrimSpace(pricing.LiteLLMProvider),
			Mode:                         strings.TrimSpace(pricing.Mode),
			InputCostPer1MTokens:         perTokenToPerMillion(pricing.InputCostPerToken),
			OutputCostPer1MTokens:        perTokenToPerMillion(pricing.OutputCostPerToken),
			CacheReadCostPer1MTokens:     perTokenToPerMillion(pricing.CacheReadInputTokenCost),
			CacheCreationCostPer1MTokens: perTokenToPerMillion(cacheCreation),
			Currency:                     "USD",
			Source:                       "gateway-lite-litellm",
			UpdatedAtMillis:              time.Now().UnixMilli(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Model) < strings.ToLower(out[j].Model)
	})
	return out
}

func perTokenToPerMillion(value float64) float64 {
	if value <= 0 {
		return 0
	}
	return value * 1000000
}
