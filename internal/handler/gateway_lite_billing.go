package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIGatewayHandler) shouldSkipGatewayLiteBillingEligibility(c *gin.Context) bool {
	if h == nil {
		return false
	}
	return shouldSkipGatewayLiteBillingEligibility(h.cfg, c)
}

func (h *GatewayHandler) shouldSkipGatewayLiteBillingEligibility(c *gin.Context) bool {
	if h == nil {
		return false
	}
	return shouldSkipGatewayLiteBillingEligibility(h.cfg, c)
}

func shouldSkipGatewayLiteBillingEligibility(cfg *config.Config, c *gin.Context) bool {
	return false
}
