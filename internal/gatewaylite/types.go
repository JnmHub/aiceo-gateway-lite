package gatewaylite

import "time"

// KeySnapshot is the hot authentication shape a regional gateway needs from
// the control plane. It intentionally excludes user profile and billing UI data.
type KeySnapshot struct {
	KeyID                 string                `json:"key_id"`
	UserID                int64                 `json:"user_id"`
	SecretHash            string                `json:"secret_hash"`
	Status                string                `json:"status"`
	Tier                  int                   `json:"tier"`
	Platform              string                `json:"platform"`
	GroupID               int64                 `json:"group_id"`
	GroupName             string                `json:"group_name"`
	AllowedModels         []string              `json:"allowed_models,omitempty"`
	RateLimitRPM          int                   `json:"rate_limit_rpm"`
	RateLimitTPM          int                   `json:"rate_limit_tpm"`
	Concurrency           int                   `json:"concurrency"`
	Version               int64                 `json:"version"`
	ExpiresAt             int64                 `json:"expires_at,omitempty"`
	CacheTTLSecond        int                   `json:"cache_ttl_seconds"`
	Group                 *GatewayGroupSnapshot `json:"group,omitempty"`
	DefaultGateway        *GatewayRouteSummary  `json:"default_gateway,omitempty"`
	AvailableGateways     []GatewayRouteSummary `json:"available_gateways,omitempty"`
	GatewayAccessEnforced bool                  `json:"gateway_access_enforced,omitempty"`
}

func (s KeySnapshot) Active() bool {
	return s.Status == "" || s.Status == "active"
}

// LeaseSnapshot is a regional spend lease. The regional Redis copy is allowed
// to make local reserve/commit decisions within this allocated budget.
type LeaseSnapshot struct {
	LeaseID        string `json:"lease_id"`
	UserID         int64  `json:"user_id"`
	Region         string `json:"region"`
	AllocatedCents int64  `json:"allocated_cents"`
	ReservedCents  int64  `json:"reserved_cents"`
	SpentCents     int64  `json:"spent_cents"`
	Version        int64  `json:"version"`
	ExpiresAt      int64  `json:"expires_at"`
}

type GatewayRouteSummary struct {
	ID           int64  `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	Region       string `json:"region"`
	Country      string `json:"country,omitempty"`
	City         string `json:"city,omitempty"`
	BaseURL      string `json:"base_url"`
	PublicURL    string `json:"public_url"`
	Format       string `json:"format"`
	RequiredTier int    `json:"required_tier"`
	LatencyMS    int    `json:"latency_ms"`
	HealthStatus string `json:"health_status"`
	OnlineUsers  int    `json:"online_users"`
	IsDefault    bool   `json:"is_default"`
}

func (l LeaseSnapshot) AvailableCents() int64 {
	return l.AllocatedCents - l.ReservedCents - l.SpentCents
}

func (l LeaseSnapshot) Expired(now time.Time) bool {
	return l.ExpiresAt > 0 && now.Unix() >= l.ExpiresAt
}

type ResolveKeyRequest struct {
	KeyID         string `json:"key_id"`
	Region        string `json:"region"`
	BillingExempt bool   `json:"billing_exempt,omitempty"`
}

type ResolveKeyResponse struct {
	OK    bool        `json:"ok"`
	Key   KeySnapshot `json:"key"`
	Error string      `json:"error,omitempty"`
}

type AcquireLeaseRequest struct {
	UserID             int64  `json:"user_id"`
	Region             string `json:"region"`
	EstimatedCostCents int64  `json:"estimated_cost_cents"`
	Reason             string `json:"reason,omitempty"`
}

type AcquireLeaseResponse struct {
	OK    bool          `json:"ok"`
	Lease LeaseSnapshot `json:"lease"`
	Error string        `json:"error,omitempty"`
}

type RebalanceLeaseRequest struct {
	UserID             int64  `json:"user_id"`
	Region             string `json:"region"`
	EstimatedCostCents int64  `json:"estimated_cost_cents"`
	Reason             string `json:"reason,omitempty"`
}

type RebalanceLeaseResponse struct {
	OK               bool          `json:"ok"`
	Lease            LeaseSnapshot `json:"lease,omitempty"`
	TransferredCents int64         `json:"transferred_cents,omitempty"`
	ReleasedRegions  []string      `json:"released_regions,omitempty"`
	Error            string        `json:"error,omitempty"`
}

type UsageEvent struct {
	RequestID        string `json:"request_id"`
	UserID           int64  `json:"user_id"`
	KeyID            string `json:"key_id"`
	LeaseID          string `json:"lease_id,omitempty"`
	Region           string `json:"region"`
	GatewayID        string `json:"gateway_id"`
	UpstreamID       string `json:"upstream_id,omitempty"`
	Protocol         string `json:"protocol"`
	Model            string `json:"model"`
	Method           string `json:"method"`
	Path             string `json:"path"`
	Status           int    `json:"status"`
	TokensIn         int64  `json:"tokens_in"`
	TokensOut        int64  `json:"tokens_out"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	EstimatedCents   int64  `json:"estimated_cents"`
	ActualCents      int64  `json:"actual_cents"`
	LatencyMillis    int64  `json:"latency_millis"`
	StartedAtMillis  int64  `json:"started_at_millis"`
	EndedAtMillis    int64  `json:"ended_at_millis"`
}

type UsageBatchReportRequest struct {
	Events []UsageEvent `json:"events"`
}

type UsageBatchReportResponse struct {
	OK            bool   `json:"ok"`
	AcceptedCount int    `json:"accepted_count"`
	AppliedCount  int    `json:"applied_count"`
	Error         string `json:"error,omitempty"`
}

type GatewayHealthReportRequest struct {
	GatewayNodeID          int64               `json:"gateway_node_id,omitempty"`
	GatewayCode            string              `json:"gateway_code,omitempty"`
	Region                 string              `json:"region,omitempty"`
	HealthStatus           string              `json:"health_status"`
	LatencyMS              int                 `json:"latency_ms,omitempty"`
	OnlineUsers            int                 `json:"online_users,omitempty"`
	Message                string              `json:"message,omitempty"`
	UsageQueueStatus       string              `json:"usage_queue_status,omitempty"`
	UsageQueueStreamLength int64               `json:"usage_queue_stream_length,omitempty"`
	UsageQueuePendingCount int64               `json:"usage_queue_pending_count,omitempty"`
	UsageQueueDeadCount    int64               `json:"usage_queue_dead_count,omitempty"`
	AvailableModels        []string            `json:"available_models,omitempty"`
	ModelPrices            []GatewayModelPrice `json:"model_prices,omitempty"`
	Metadata               map[string]any      `json:"metadata,omitempty"`
}

type GatewayModelPrice struct {
	GatewayNodeID                int64   `json:"gateway_node_id,omitempty"`
	GatewayCode                  string  `json:"gateway_code,omitempty"`
	Model                        string  `json:"model"`
	Provider                     string  `json:"provider,omitempty"`
	Mode                         string  `json:"mode,omitempty"`
	InputCostPer1MTokens         float64 `json:"input_cost_per_1m_tokens"`
	OutputCostPer1MTokens        float64 `json:"output_cost_per_1m_tokens"`
	CacheReadCostPer1MTokens     float64 `json:"cache_read_cost_per_1m_tokens"`
	CacheCreationCostPer1MTokens float64 `json:"cache_creation_cost_per_1m_tokens"`
	Currency                     string  `json:"currency"`
	Source                       string  `json:"source,omitempty"`
	UpdatedAtMillis              int64   `json:"updated_at_millis,omitempty"`
}

type GatewayHealthReportResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type GatewayConfigSnapshotRequest struct {
	Region       string `json:"region"`
	SinceVersion int64  `json:"since_version,omitempty"`
}

type GatewayConfigSnapshotResponse struct {
	OK       bool                  `json:"ok"`
	Snapshot GatewayConfigSnapshot `json:"snapshot"`
	Error    string                `json:"error,omitempty"`
}

type GatewayConfigSnapshot struct {
	Version           int64                    `json:"version"`
	GeneratedAtMillis int64                    `json:"generated_at_millis"`
	Accounts          []GatewayAccountSnapshot `json:"accounts"`
	Groups            []GatewayGroupSnapshot   `json:"groups"`
	GatewayNodes      []GatewayNodeSnapshot    `json:"gateway_nodes,omitempty"`
}

type GatewayNodeSnapshot struct {
	ID           int64          `json:"id"`
	Code         string         `json:"code"`
	Name         string         `json:"name"`
	Region       string         `json:"region"`
	Format       string         `json:"format"`
	RequiredTier int            `json:"required_tier"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type CacheInvalidationRequest struct {
	GatewayCode string  `json:"gateway_code,omitempty"`
	Region      string  `json:"region,omitempty"`
	SinceID     int64   `json:"since_id,omitempty"`
	Limit       int     `json:"limit,omitempty"`
	AckIDs      []int64 `json:"ack_ids,omitempty"`
}

type CacheInvalidationResponse struct {
	OK       bool                     `json:"ok"`
	Events   []CacheInvalidationEvent `json:"events,omitempty"`
	LatestID int64                    `json:"latest_id,omitempty"`
	Error    string                   `json:"error,omitempty"`
}

type CacheInvalidationEvent struct {
	ID                 int64          `json:"id"`
	Scope              string         `json:"scope"`
	Reason             string         `json:"reason"`
	GatewayCode        string         `json:"gateway_code,omitempty"`
	Region             string         `json:"region,omitempty"`
	ConfigVersion      int64          `json:"config_version,omitempty"`
	Payload            map[string]any `json:"payload,omitempty"`
	CreatedAtMillis    int64          `json:"created_at_millis,omitempty"`
	AcknowledgedMillis int64          `json:"acknowledged_at_millis,omitempty"`
}

type GatewayAccountSnapshot struct {
	ID              int64          `json:"id"`
	Name            string         `json:"name"`
	Platform        string         `json:"platform"`
	Type            string         `json:"type"`
	Status          string         `json:"status"`
	Schedulable     bool           `json:"schedulable"`
	Concurrency     int            `json:"concurrency"`
	Priority        int            `json:"priority"`
	RateMultiplier  *float64       `json:"rate_multiplier,omitempty"`
	LoadFactor      *int           `json:"load_factor,omitempty"`
	Credentials     map[string]any `json:"credentials,omitempty"`
	Extra           map[string]any `json:"extra,omitempty"`
	GroupIDs        []int64        `json:"group_ids,omitempty"`
	Version         int64          `json:"version"`
	UpdatedAtMillis int64          `json:"updated_at_millis"`
}

type GatewayGroupSnapshot struct {
	ID                              int64                             `json:"id"`
	Name                            string                            `json:"name"`
	Platform                        string                            `json:"platform"`
	Status                          string                            `json:"status"`
	IsExclusive                     bool                              `json:"is_exclusive"`
	SubscriptionType                string                            `json:"subscription_type,omitempty"`
	RateMultiplier                  float64                           `json:"rate_multiplier"`
	DailyLimitUSD                   *float64                          `json:"daily_limit_usd,omitempty"`
	WeeklyLimitUSD                  *float64                          `json:"weekly_limit_usd,omitempty"`
	MonthlyLimitUSD                 *float64                          `json:"monthly_limit_usd,omitempty"`
	AllowImageGeneration            bool                              `json:"allow_image_generation"`
	ImageRateIndependent            bool                              `json:"image_rate_independent"`
	ImageRateMultiplier             float64                           `json:"image_rate_multiplier"`
	ImagePrice1K                    *float64                          `json:"image_price_1k,omitempty"`
	ImagePrice2K                    *float64                          `json:"image_price_2k,omitempty"`
	ImagePrice4K                    *float64                          `json:"image_price_4k,omitempty"`
	ClaudeCodeOnly                  bool                              `json:"claude_code_only"`
	FallbackGroupID                 *int64                            `json:"fallback_group_id,omitempty"`
	FallbackGroupIDOnInvalidRequest *int64                            `json:"fallback_group_id_on_invalid_request,omitempty"`
	ModelRouting                    map[string][]int64                `json:"model_routing,omitempty"`
	ModelRoutingEnabled             bool                              `json:"model_routing_enabled"`
	MCPXMLInject                    bool                              `json:"mcp_xml_inject"`
	SupportedModelScopes            []string                          `json:"supported_model_scopes,omitempty"`
	AllowMessagesDispatch           bool                              `json:"allow_messages_dispatch"`
	RequireOAuthOnly                bool                              `json:"require_oauth_only"`
	RequirePrivacySet               bool                              `json:"require_privacy_set"`
	DefaultMappedModel              string                            `json:"default_mapped_model,omitempty"`
	MessagesDispatchModelConfig     OpenAIMessagesDispatchModelConfig `json:"messages_dispatch_model_config,omitempty"`
	ModelsListConfig                GroupModelsListConfig             `json:"models_list_config,omitempty"`
	RPMLimit                        int                               `json:"rpm_limit"`
	Config                          map[string]any                    `json:"config,omitempty"`
	Version                         int64                             `json:"version"`
	UpdatedAtMillis                 int64                             `json:"updated_at_millis"`
}

type OpenAIMessagesDispatchModelConfig struct {
	OpusMappedModel    string            `json:"opus_mapped_model,omitempty"`
	SonnetMappedModel  string            `json:"sonnet_mapped_model,omitempty"`
	HaikuMappedModel   string            `json:"haiku_mapped_model,omitempty"`
	ExactModelMappings map[string]string `json:"exact_model_mappings,omitempty"`
}

type GroupModelsListConfig struct {
	Enabled bool     `json:"enabled"`
	Models  []string `json:"models,omitempty"`
}
