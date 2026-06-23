package server

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayLiteRegistersAdminOpsSurfaceOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	cfg := &config.Config{
		RunMode: config.RunModeGatewayLite,
	}

	registerRoutes(
		r,
		newGatewayLiteRouteTestHandlers(),
		servermiddleware.JWTAuthMiddleware(noopRouteTestMiddleware),
		servermiddleware.AdminAuthMiddleware(noopRouteTestMiddleware),
		servermiddleware.APIKeyAuthMiddleware(noopRouteTestMiddleware),
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
	)

	routes := routeSet(r)
	requireRoute(t, routes, "POST", "/v1/messages")
	requireRoute(t, routes, "POST", "/api/v1/auth/login")
	requireRoute(t, routes, "GET", "/api/v1/auth/me")
	requireRoute(t, routes, "GET", "/api/v1/settings/public")
	requireRoute(t, routes, "GET", "/api/v1/admin/accounts")
	requireRoute(t, routes, "GET", "/api/v1/admin/ops/dashboard/overview")
	requireRoute(t, routes, "GET", "/api/v1/admin/usage")
	requireRoute(t, routes, "GET", "/api/v1/admin/channel-monitors")

	// gateway-lite（轻量网关）只保留运维后台，不暴露客户侧注册、购买、兑换和用户中心。
	requireNoRoute(t, routes, "POST", "/api/v1/auth/register")
	requireNoRoute(t, routes, "GET", "/api/v1/user/profile")
	requireNoRoute(t, routes, "GET", "/api/v1/payment/config")
	requireNoRoute(t, routes, "GET", "/api/v1/admin/redeem-codes")
	requireNoRoute(t, routes, "GET", "/api/v1/admin/promo-codes")
	requireNoRoute(t, routes, "GET", "/api/v1/admin/subscriptions")
}

func noopRouteTestMiddleware(c *gin.Context) {
	c.Next()
}

func routeSet(r *gin.Engine) map[string]struct{} {
	routes := make(map[string]struct{})
	for _, route := range r.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	return routes
}

func requireRoute(t *testing.T, routes map[string]struct{}, method, path string) {
	t.Helper()
	_, ok := routes[method+" "+path]
	require.Truef(t, ok, "expected route %s %s to be registered", method, path)
}

func requireNoRoute(t *testing.T, routes map[string]struct{}, method, path string) {
	t.Helper()
	_, ok := routes[method+" "+path]
	require.Falsef(t, ok, "expected route %s %s to be absent", method, path)
}

func newGatewayLiteRouteTestHandlers() *handler.Handlers {
	return &handler.Handlers{
		Auth:          &handler.AuthHandler{},
		Setting:       &handler.SettingHandler{},
		Gateway:       &handler.GatewayHandler{},
		OpenAIGateway: &handler.OpenAIGatewayHandler{},
		Admin: &handler.AdminHandlers{
			Compliance:             &adminhandler.ComplianceHandler{},
			Dashboard:              &adminhandler.DashboardHandler{},
			Group:                  &adminhandler.GroupHandler{},
			Account:                &adminhandler.AccountHandler{},
			OAuth:                  &adminhandler.OAuthHandler{},
			OpenAIOAuth:            &adminhandler.OpenAIOAuthHandler{},
			GeminiOAuth:            &adminhandler.GeminiOAuthHandler{},
			AntigravityOAuth:       &adminhandler.AntigravityOAuthHandler{},
			Proxy:                  &adminhandler.ProxyHandler{},
			Setting:                &adminhandler.SettingHandler{},
			Ops:                    &adminhandler.OpsHandler{},
			Usage:                  &adminhandler.UsageHandler{},
			ErrorPassthrough:       &adminhandler.ErrorPassthroughHandler{},
			TLSFingerprintProfile:  &adminhandler.TLSFingerprintProfileHandler{},
			ScheduledTest:          &adminhandler.ScheduledTestHandler{},
			Channel:                &adminhandler.ChannelHandler{},
			ChannelMonitor:         &adminhandler.ChannelMonitorHandler{},
			ChannelMonitorTemplate: &adminhandler.ChannelMonitorRequestTemplateHandler{},
			ContentModeration:      &adminhandler.ContentModerationHandler{},
		},
	}
}
