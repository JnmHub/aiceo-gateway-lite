# gateway-lite API（轻量网关接口）文档

## 对外用户接口

gateway-lite（轻量网关）保留 sub2api 原有 AI 网关接口，例如：

- `/v1/messages`
- `/v1/chat/completions`
- `/v1/responses`
- `/v1beta/*`

这些接口用于接收用户的 AI 请求，并转发到上游模型账号。

### 请求来源观测字段

gateway-lite（轻量网关）的 access log（访问日志）会记录脱敏后的客户端来源摘要，便于排查慢请求、流式超时和客户端兼容性问题。字段会写入标准日志，并同步进入 `ops_system_logs（运维系统日志）`：

- `user_agent（客户端标识）`：来自 `User-Agent`，最长保留 512 字符。
- `content_length（请求体长度）`：来自 HTTP `Content-Length`，未知时为 `-1`。
- `host（请求域名）`：来自 Host，最长保留 255 字符。
- `referer（来源页面）`：来自 `Referer`，最长保留 512 字符。
- `x_request_id_header（请求编号头）`：客户端传入的 `X-Request-ID`。
- `x_client_request_id_header（客户端请求编号头）`：客户端传入的 `X-Client-Request-ID`。
- `effective_client_request_id（实际链路编号）`：网关生成或保留后的链路请求编号。

安全说明：访问日志不会记录 `Authorization（鉴权头）`、Cookie（浏览器凭据）或完整 request body（请求体）。

## 后台登录接口

gateway-lite（轻量网关）保留最小后台 auth（认证）接口，方便管理员登录区域网关后台：

- `POST /api/v1/auth/login`
- `POST /api/v1/auth/login/2fa`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `POST /api/v1/auth/revoke-all-sessions`
- `GET /api/v1/settings/public`

gateway-lite（轻量网关）不注册客户侧注册、找回密码、OAuth（第三方登录）注册、邀请码和优惠码校验接口。

首次启动管理员：

- gateway-lite（轻量网关）主服务启动后会检查 users（用户表）。
- 默认后台账号：`105626@qq.com`。
- 默认后台密码：`00hhg5210`。
- 当 users（用户表）为空且不存在 admin（管理员）时，会自动创建默认 admin（管理员）账号。
- 默认账号密码来自 `config.yaml（配置文件）` 的 `admin（管理员）` 配置段，只在第一次空库启动时生效。
- gateway-lite（轻量网关）模式下，如果已经存在任意 admin（管理员），启动时不会覆盖密码，也不会同步默认账号。
- standard（标准）模式如果已经存在任意用户或管理员，则跳过默认账号创建，避免覆盖真实账号。
- `AUTO_SETUP`（自动初始化）模式也使用同一套默认管理员账号；可以用 `ADMIN_EMAIL` 和 `ADMIN_PASSWORD` 环境变量覆盖。
- gateway-lite（轻量网关）后台页面默认随二进制嵌入。执行 `make build` 会先构建 `frontend`，再生成带后台页面的轻量网关二进制；普通 `go build ./cmd/server` 会嵌入当前 `internal/web/dist` 中已有的后台页面，不需要额外 `-tags embed`。

`config.yaml（配置文件）` 示例：

```yaml
admin:
  email: 105626@qq.com
  password: 00hhg5210
gateway_lite:
  region: local
  gateway_code: local-gateway
  redis_prefix: jnm:gateway-lite
  admin_sync_key: dev-gateway-admin-key
  control_plane_url: http://127.0.0.1:8088
  control_plane_token: dev-internal-token
  control_plane_timeout_ms: 1000
  runtime_health_interval_seconds: 15
  runtime_active_window_seconds: 300
  config_sync_interval_seconds: 30
  cache_invalidation_interval_seconds: 5
  usage_queue_pending_alert_threshold: 1000
  usage_queue_dead_alert_threshold: 1
```

`GET /api/v1/settings/public` 会返回 UI capabilities（界面能力）字段，前端可以用它隐藏不适合当前运行模式的菜单：

```json
{
  "run_mode": "gateway-lite",
  "ui_surface": "gateway-lite-admin",
  "gateway_lite_admin_enabled": true,
  "customer_self_service_enabled": false
}
```

字段说明：

- `run_mode`：run mode（运行模式），gateway-lite（轻量网关）节点返回 `gateway-lite`。
- `ui_surface`：UI surface（界面形态），gateway-lite（轻量网关）节点返回 `gateway-lite-admin`。
- `gateway_lite_admin_enabled`：gateway-lite admin（轻量网关后台）是否启用。
- `customer_self_service_enabled`：customer self-service（客户自助功能）是否启用；gateway-lite（轻量网关）返回 `false`，前端应隐藏充值、注册、邀请、用户中心等客户侧入口。

## 后台管理接口

gateway-lite（轻量网关）保留 admin（后台管理）里的运维功能，方便管理区域网关：

- `/api/v1/admin/dashboard/*`：dashboard（仪表盘）和统计概览。
- `/api/v1/admin/accounts/*`：accounts（账号池）管理、测试、刷新和批量维护。
- `/api/v1/admin/groups/*`：groups（分组）管理和容量统计。
- `/api/v1/admin/proxies/*`：proxies（代理）管理和质量检测。
- `/api/v1/admin/ops/*`：ops（运维监控）、告警、日志、实时流量和请求错误。
- `/api/v1/admin/usage/*`：usage（用量记录）查询和清理任务。
- `/api/v1/admin/settings/*`：settings（系统设置）中和网关运维相关的配置。
- `GET /api/v1/admin/gateway-lite/config`：gateway-lite config（轻量网关配置）读取当前主站控制面连接配置；不会返回完整 `control_plane_token（主站控制面令牌）`。
- `PUT /api/v1/admin/gateway-lite/config`：gateway-lite config（轻量网关配置）写入 `config.yaml（配置文件）`，并立即刷新运行时主站客户端、同步器、健康上报和 Redis prefix（缓存前缀）等配置；端口、数据库连接这类启动级配置仍属于重启范围。
- `GET /api/v1/admin/settings/web-search-emulation`：web search emulation（网页搜索模拟）配置；如果尚未配置，返回默认关闭配置 `{"enabled":false,"providers":[]}`，避免后台首次打开账号池或渠道页时出现 404。
- `/api/v1/admin/model-prices/*`：model prices（模型价格）统一管理，读取默认价格目录并维护本地覆盖价。
- `/api/v1/admin/channels/*`：channels（渠道）管理。
- `/api/v1/admin/channel-monitors/*`：channel monitors（渠道监控）管理。
- `/api/v1/admin/channel-monitor-templates/*`：channel monitor templates（渠道监控模板）管理。
- `/api/v1/admin/error-passthrough-rules/*`：error passthrough（错误透传）规则。
- `/api/v1/admin/tls-fingerprint-profiles/*`：TLS fingerprint（TLS 指纹）模板。
- `/api/v1/admin/scheduled-test-plans/*`：scheduled tests（定时测试）计划。
- `/api/v1/admin/risk-control/*`：risk control（风控）配置和日志。
- `/api/v1/admin/openai/*`、`/api/v1/admin/gemini/*`、`/api/v1/admin/antigravity/*`：上游账号 OAuth（授权）维护能力。

gateway-lite（轻量网关）不注册客户侧和主站运营类后台接口，例如：

- `/api/v1/user/*`
- `/api/v1/payment/*`
- `/api/v1/admin/payment/*`
- `/api/v1/admin/redeem-codes/*`
- `/api/v1/admin/promo-codes/*`
- `/api/v1/admin/subscriptions/*`
- `/api/v1/admin/affiliates/*`

### 主站代理账号池鉴权

gateway-lite（轻量网关）后台接口默认仍使用管理员 JWT（登录令牌）或 admin API key（管理员接口 Key）鉴权。为了让 control-plane（主站控制面）能统一管理区域网关账号池，以下账号池接口在 gateway-lite（轻量网关）模式下额外接受 `X-Gateway-Lite-Admin-Key: <gateway_lite.admin_sync_key>`：

- `GET /api/v1/admin/accounts`
- `POST /api/v1/admin/accounts`
- `PUT /api/v1/admin/accounts/:id`
- `DELETE /api/v1/admin/accounts/:id`
- `POST /api/v1/admin/accounts/:id/test`
- `POST /api/v1/admin/accounts/:id/models/sync-upstream`

这个 Key 只用于主站代理账号池管理，不开放完整后台权限。请求必须命中以上账号池路径；其他后台接口仍需要正常管理员登录或 admin API key。

创建账号时可传 `status` 和 `schedulable` 控制初始调度状态：`status` 支持 `active`、`inactive`、`error`；`schedulable` 支持布尔值。不传时保持历史默认值：`status=active` 且 `schedulable=true`。主站如果要先下发一个不参与调度的账号，应显式传 `status=inactive` 或 `schedulable=false`。

`GET /api/v1/admin/accounts` 支持 `lite=true&include_credentials=true`。该组合仅供 control-plane（主站控制面）使用 `X-Gateway-Lite-Admin-Key` 同步账号池镜像时调用，会返回完整 `credentials（凭证）`，让主站生成 config snapshot（配置快照）时能下发真实参与调度的账号。普通后台页面不要传 `include_credentials=true`，默认仍返回脱敏凭证和 `credentials_status（凭证存在状态）`。

### 模型价格统一管理

模型价格由两层组成：

- default catalog（默认价格目录）：轻量网关启动时从远程/本地 LiteLLM 价格目录加载，新服务器随二进制自带 MiMo（小米）文本模型价格。
- override（覆盖价）：管理员在轻量网关后台“模型价格”页面保存的本地价格，持久化到 `data/model_pricing_overrides.json`，优先生效；删除覆盖价后回退默认目录或内置兜底价。

#### `GET /api/v1/admin/model-prices`

用途：分页查询当前生效的 model prices（模型价格）。

查询参数：

- `page`、`page_size`：分页。
- `search`：按模型名或 provider（供应商）搜索。
- `provider`：按 provider 过滤，例如 `xiaomi`、`openai`、`anthropic`、`google`、`custom`。
- `source`：按来源过滤，支持 `catalog`（默认目录）、`override`（覆盖价）、`static_fallback`（内置兜底）。

返回字段中的价格单位为 USD / 1M tokens（美元/百万 tokens），图片价格为 USD / image（美元/张）。

#### `PUT /api/v1/admin/model-prices`

用途：新增或更新某个模型的 override（覆盖价）。保存后网关热路径、账号统计价格和健康上报价格都会优先使用覆盖价。

请求体示例：

```json
{
  "model": "mimo-v2.5-pro",
  "provider": "xiaomi",
  "mode": "chat",
  "input_cost_per_1m_tokens": 0.435,
  "output_cost_per_1m_tokens": 0.87,
  "cache_read_cost_per_1m_tokens": 0.0036,
  "cache_creation_cost_per_1m_tokens": 0,
  "supports_prompt_caching": true
}
```

#### `DELETE /api/v1/admin/model-prices?model=mimo-v2.5-pro`

用途：删除某个模型的 override（覆盖价），不会删除默认目录价格；删除后返回当前回退后的生效价格。

### `GET /api/v1/admin/gateway-lite/config`

用途：读取 gateway-lite config（轻量网关配置），用于后台设置页展示主站控制面连接信息。

返回示例：

```json
{
  "region": "local",
  "gateway_code": "local-gateway",
  "redis_prefix": "jnm:gateway-lite",
  "admin_sync_key_configured": true,
  "control_plane_url": "http://127.0.0.1:8088",
  "control_plane_token_configured": true,
  "control_plane_timeout_ms": 1000,
  "runtime_health_interval_seconds": 15,
  "runtime_active_window_seconds": 300,
  "config_sync_interval_seconds": 30,
  "cache_invalidation_interval_seconds": 5,
  "usage_queue_pending_alert_threshold": 1000,
  "usage_queue_dead_alert_threshold": 1,
  "config_path": "./config.yaml",
  "restart_required": false
}
```

字段说明：

- `control_plane_url（主站控制面地址）`：轻量网关访问主站后端的地址。
- `admin_sync_key_configured（管理员同步 Key 是否已配置）`：只返回是否已配置，不返回完整 Key。
- `control_plane_token_configured（主站控制面令牌是否已配置）`：只返回是否已配置，不返回完整令牌。
- `restart_required（是否需要重启）`：读取时为 `false`。

### `PUT /api/v1/admin/gateway-lite/config`

用途：保存 gateway-lite config（轻量网关配置）到 `config.yaml（配置文件）`，并立即刷新运行时配置。保存后会尝试触发一次 full sync（全量同步）和 health report（健康上报），让主站通信配置尽快生效。

请求体示例：

```json
{
  "region": "local",
  "gateway_code": "local-gateway",
  "redis_prefix": "jnm:gateway-lite",
  "admin_sync_key": "dev-gateway-admin-key",
  "control_plane_url": "http://127.0.0.1:8088",
  "control_plane_token": "dev-internal-token",
  "control_plane_timeout_ms": 1000,
  "runtime_health_interval_seconds": 15,
  "runtime_active_window_seconds": 300,
  "config_sync_interval_seconds": 30,
  "cache_invalidation_interval_seconds": 5,
  "usage_queue_pending_alert_threshold": 1000,
  "usage_queue_dead_alert_threshold": 1
}
```

说明：

- `admin_sync_key（轻量网关管理员同步 Key）` 留空或不传时，会保留原配置文件里的旧 Key。主站同步网关、主站代理账号池管理都依赖这个 Key。
- `control_plane_token（主站控制面令牌）` 留空或不传时，会保留原配置文件里的旧令牌。
- `control_plane_url（主站控制面地址）` 如果不为空，必须是 `http（超文本传输协议）` 或 `https（安全超文本传输协议）` 绝对地址。
- 返回体和 `GET /api/v1/admin/gateway-lite/config` 一致。当前主站连接类配置保存后 `restart_required（是否需要重启）` 为 `false`。

## 鉴权方式

用户可以通过以下方式传 API Key（接口密钥）：

- `Authorization: Bearer <api_key>`
- `x-api-key: <api_key>`
- `x-goog-api-key: <api_key>`

API Key（接口密钥）格式：

```text
aiceo_sk_<key_id>_<secret>
```

gateway-lite（轻量网关）只把 `key_id` 发给 control-plane（主站控制面），本地用 `sha256(secret)` 校验密钥。

## 内部依赖接口

gateway-lite（轻量网关）会调用 control-plane（主站控制面）的内部接口：

- `POST /internal/key/resolve`
- `POST /internal/quota/acquire-lease`
- `POST /internal/quota/refill-lease`
- `POST /internal/quota/rebalance-lease`
- `POST /internal/usage/report`
- `POST /internal/usage/report-batch`
- `POST /internal/config/snapshot`
- `POST /internal/cache/invalidate`

模型校验规则：

- `POST /internal/key/resolve（内部密钥解析）` 返回的 `allowed_models（允许模型）` 会进入 gateway-lite（轻量网关）的热路径缓存。
- 用户请求体里存在顶层 `model（模型名）` 时，gateway-lite（轻量网关）会在预留额度前校验该模型是否命中允许列表；不命中时返回 `403 MODEL_NOT_ALLOWED（模型不允许）`，不会占用本地 lease（额度租约）。
- `["*"]` 或空列表表示不限制模型。模型缺失的接口不会被这个规则误拦截。

额度调用规则：

- gateway-lite（轻量网关）优先使用本地 Redis（缓存服务）里的 quota lease（额度租约）完成热路径 reserve（预占）。
- 本地 lease（额度租约）不存在、过期或余额不足时，先调用 `POST /internal/quota/acquire-lease（申请额度租约）`。
- 如果 acquire lease（申请额度租约）返回不可用额度，才调用 `POST /internal/quota/rebalance-lease（额度租约重分配）`，让 control-plane（主站控制面）从其他闲置 region（区域）释放并转移额度。
- rebalance（重分配）成功后，gateway-lite（轻量网关）把返回的 lease snapshot（额度租约快照）写入本地 Redis（缓存服务），再重新 reserve（预占）；失败则返回 `INSUFFICIENT_QUOTA（额度不足）`。
- rebalance（重分配）不在每个请求都调用，只有当前网关本地额度不足且普通申请失败时进入慢路径，避免影响正常客户访问。

用量上报规则：

- 用户请求完成后，gateway-lite（轻量网关）先把 usage event（用量事件）写入本地 Redis Streams（Redis 消息流）`gl:usage:stream`，不在用户请求热路径同步等待主站入账。
- usage worker（用量后台任务）使用 consumer group（消费者组）读取 `gl:usage:stream`，默认每批最多 50 条。
- control-plane client（主站客户端）优先调用 `POST /internal/usage/report-batch（批量用量上报）`；如果下游不支持 batch（批量），代码仍可回退到单条 `POST /internal/usage/report（单条用量上报）`。
- 批量上报成功后执行 `XACK（消息确认）`，随后执行 `XDEL（删除已处理消息）`；同时通过 `XTRIM MAXLEN ~ 10000` 防止旧版本遗留消息或异常路径让 stream（消息流）无限增长。失败时重新写回 `gl:usage:stream` 并按次数退避重试。
- 重试耗尽或事件无法解析时写入 `gl:usage:dead` dead stream（死信消息流），后续运维可单独排查。
- runtime health monitor（运行时健康监控）会定期探测 control-plane（主站控制面）`/health（健康检查）`，同时读取 `gl:usage:stream（用量消息流）`、consumer group pending count（消费者组待确认数量）和 `gl:usage:dead（用量死信消息流）`，再合并上报到 `POST /internal/gateway/health-report（网关健康上报）`。
- runtime health monitor（运行时健康监控）也会在 `metadata（元数据）` 中上报轻量网关本地运行指标：`usage_record_*（用量记录工作池）`、`billing_cache_*（计费缓存写入队列）` 和 `redis_pool_*（Redis 连接池）`，用于定位高并发下的队列堆积、丢弃、同步回退和 Redis 连接瓶颈。其中 `usage_record_queue_capacity（用量记录队列容量）`、`usage_record_queue_utilization_percent（用量记录队列占用率）`、`usage_record_overflow_policy（队列满策略）`、`usage_record_auto_scale_*（自动扩缩容配置）` 会反映当前进程实际运行配置。
- 如果 `gateway_lite.available_models（轻量网关可用模型）` 或 `GATEWAY_LITE_AVAILABLE_MODELS（轻量网关可用模型环境变量）` 配置了模型列表，runtime health monitor（运行时健康监控）会把它们作为 `available_models（可用模型列表）` 一起上报给主站。环境变量使用英文逗号分隔，例如 `gpt-4o,claude-sonnet-4`。空列表不会上报，不影响用户请求。
- `stream length（消息流长度）` 只作为观测指标上报，不直接触发告警；`pending count（待确认数量）` 超过阈值触发 warning（警告），`dead count（死信数量）` 超过阈值触发 critical（严重）。
- 如果 control-plane（主站控制面）的 gateway node（网关节点）`metadata（元数据）` 里配置了 `health_probe_interval_seconds（健康探测间隔秒数）`，gateway-lite（轻量网关）会在 config snapshot（配置快照）同步后动态使用该间隔；否则使用环境变量默认值。

相关环境变量：

- `GATEWAY_LITE_RUNTIME_HEALTH_INTERVAL_SECONDS`：runtime health report interval（运行时健康上报间隔），默认 `15` 秒；这是启动默认值，可被主站 gateway node metadata（网关节点元数据）里的 `health_probe_interval_seconds（健康探测间隔秒数）` 覆盖。
- `GATEWAY_LITE_RUNTIME_ACTIVE_WINDOW_SECONDS`：online users active window（在线用户活跃窗口），默认 `300` 秒。
- `GATEWAY_LITE_AVAILABLE_MODELS`：available models（可用模型列表），英文逗号分隔；用于上报当前 gateway-lite（轻量网关）节点声明支持的模型。
- `GATEWAY_LITE_USAGE_QUEUE_PENDING_ALERT_THRESHOLD`：pending count alert threshold（待确认数量告警阈值），默认 `1000`。
- `GATEWAY_LITE_USAGE_QUEUE_DEAD_ALERT_THRESHOLD`：dead stream alert threshold（死信数量告警阈值），默认 `1`。

### 本地用量记录队列配置

`gateway.usage_record（本地用量记录队列）` 控制 gateway-lite（轻量网关）请求完成后的本地统计/用量记录任务。它不替代 control-plane（主站控制面）的最终入账，只负责本地请求日志、统计和缓存类收尾任务的后台执行。

默认配置：

- `worker_count`: `128`，初始 worker（工作协程）数量。
- `queue_size`: `16384`，等待队列容量。
- `task_timeout_seconds`: `5`，单个本地记录任务超时时间。
- `overflow_policy`: `sample`，队列满时策略。
- `overflow_sample_percent`: `10`，`sample` 策略下同步回退采样比例。
- `auto_scale_enabled`: `true`，是否按队列占用率自动扩缩容。
- `auto_scale_min_workers`: `128`，自动扩缩容最小 worker 数。
- `auto_scale_max_workers`: `512`，自动扩缩容最大 worker 数。
- `auto_scale_up_queue_percent`: `70`，队列占用率达到该值时扩容。
- `auto_scale_down_queue_percent`: `15`，队列占用率低于该值时缩容。
- `auto_scale_up_step`: `32`，单次扩容 worker 数。
- `auto_scale_down_step`: `16`，单次缩容 worker 数。
- `auto_scale_check_interval_seconds`: `3`，扩缩容检查间隔。
- `auto_scale_cooldown_seconds`: `10`，扩缩容冷却时间。

`overflow_policy（队列满策略）` 说明：

- `drop（丢弃）`：队列满时丢弃本地记录任务，最不影响请求尾部耗时，但本地统计可能缺失。
- `sample（抽样同步）`：默认策略。队列满时按 `overflow_sample_percent` 比例同步执行，其他丢弃，在性能和统计完整性之间折中。
- `sync（同步回退）`：队列满时同步执行任务，记录最完整，但高并发尾部请求可能变慢。

本轮优化没有新增 image（生图）专用队列；生图或强制记录任务后续可以单独拆队列，避免普通文本请求高峰挤压关键记录。

`POST /internal/cache/invalidate（缓存失效事件）` 用于让 gateway-lite（轻量网关）短轮询主站配置变更事件。gateway-lite（轻量网关）启动后会在后台消费事件，不进入用户请求热路径。

请求体示例：

```json
{
  "gateway_code": "sg-1",
  "region": "sg",
  "since_id": 0,
  "limit": 50,
  "ack_ids": [1, 2]
}
```

响应体示例：

```json
{
  "ok": true,
  "events": [
    {
      "id": 1,
      "scope": "config:snapshot",
      "reason": "gateway_config_updated",
      "gateway_code": "sg-1",
      "region": "sg",
      "config_version": 12
    }
  ],
  "latest_id": 1
}
```

事件处理规则：

- `config:snapshot（配置快照）`：gateway-lite（轻量网关）强制调用 `POST /internal/config/snapshot（配置快照）` 拉取全量配置，并写入 scheduler cache（调度缓存）。
- `key:snapshot（密钥快照）`：gateway-lite（轻量网关）清理本 region（区域）的 Key cache（密钥缓存），下一次用户请求会重新解析 key（密钥）。
- 其他 scope（范围）暂时忽略，但会推进 cursor（游标），避免重复消费。

相关环境变量：

- `GATEWAY_LITE_GATEWAY_CODE`：gateway code（网关编号），默认使用 `GATEWAY_LITE_REGION（网关区域）`。
- `GATEWAY_LITE_CACHE_INVALIDATION_INTERVAL_SECONDS`：cache invalidation polling interval（缓存失效轮询间隔），默认 `5` 秒。
- `GATEWAY_LITE_CONFIG_SYNC_INTERVAL_SECONDS`：config snapshot sync interval（配置快照同步间隔），默认 `30` 秒。

网关权限拦截：control-plane（主站控制面）返回的 key snapshot（密钥快照）如果带有 `gateway_access_enforced=true（强制网关访问检查）`，gateway-lite（轻量网关）会检查当前 `GATEWAY_LITE_GATEWAY_CODE（网关编号）` 是否在 `available_gateways（可用网关）` 里；不在列表中时返回 `GATEWAY_NOT_ALLOWED（网关不可用或无权限）`，不会预占额度。

## Redis（缓存服务）用途

- Key cache（密钥缓存）：减少每次请求访问 control-plane（主站控制面）。
- quota lease（额度租约）：在区域网关本地快速判断余额。
- usage queue（用量队列）：异步回传用量，避免影响用户请求。
- scheduler cache（调度缓存）：保存账号和分组快照，用于选择上游账号。
- config snapshot（配置快照）：`gl:config:snapshot`。
- config version（配置版本）：`gl:config:version`。
- cache invalidation cursor（缓存失效游标）：`gl:config:invalidation:last_id`。
- key snapshot（密钥快照）：`gl:key:<region>:<key_id>`。
- usage stream（用量消息流）：`gl:usage:stream`。
- usage dead stream（用量死信消息流）：`gl:usage:dead`。
