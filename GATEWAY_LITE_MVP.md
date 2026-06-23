# Gateway Lite MVP

This fork keeps `sub2api` as a regional AI gateway and moves user, payment,
balance, and API key ownership to a separate control plane.

## Run Mode

Set:

```bash
RUN_MODE=gateway-lite
GATEWAY_LITE_REGION=sg
GATEWAY_LITE_CONTROL_PLANE_URL=https://control.example.com
GATEWAY_LITE_CONTROL_PLANE_TOKEN=replace-me
GATEWAY_LITE_CONTROL_PLANE_TIMEOUT_MS=300
GATEWAY_LITE_REDIS_PREFIX=gl
GATEWAY_LITE_CONFIG_SYNC_INTERVAL_SECONDS=30
GATEWAY_LITE_RUNTIME_HEALTH_INTERVAL_SECONDS=15
GATEWAY_LITE_RUNTIME_ACTIVE_WINDOW_SECONDS=300
```

In `gateway-lite` mode the router registers only:

- common health/status routes
- gateway routes such as `/v1/messages`, `/v1/chat/completions`, `/v1/responses`

It skips:

- frontend middleware
- auth/user/admin/payment routes under `/api/v1`
- custom page routes

## Control Plane Contract

The regional gateway expects these internal endpoints:

- `POST /internal/key/resolve`
- `POST /internal/quota/acquire-lease`
- `POST /internal/quota/refill-lease`
- `POST /internal/usage/report`
- `POST /internal/config/snapshot`
- `POST /internal/gateway/health-report`

The runtime health monitor（运行时健康监视器）reports（上报）`latency_ms（延迟毫秒）`
from a real control-plane（主站控制面）`/health（健康检查）` probe（探测）,
`health_status（健康状态）`, `online_users（在线用户数）`, and `usage_queue（用量队列）`
metrics（指标）in one heartbeat（心跳）request（请求）. `online_users（在线用户数）`
is estimated（估算）from users（用户）who successfully used（成功使用）this gateway（网关）
during `GATEWAY_LITE_RUNTIME_ACTIVE_WINDOW_SECONDS（运行时活跃窗口秒数）`.

API keys should use this shape:

```text
aiceo_sk_<key_id>_<secret>
```

The gateway sends only `key_id` to resolve metadata, then verifies
`sha256(secret)` against the returned `secret_hash`.

## Current MVP State

Implemented:

- `run_mode: gateway-lite`
- gateway-only route registration
- control-plane HTTP client
- control-plane DTOs for key snapshots, quota leases, and usage events
- remote API key auth middleware that:
  - parses `Authorization: Bearer ...`, `x-api-key`, or `x-goog-api-key`
  - resolves the key from regional Redis first, then falls back to the control plane
  - verifies the local secret hash
  - reserves quota from an existing regional Redis lease when possible
  - lazily acquires an on-demand regional lease from the control plane on cache miss or low local quota
  - writes new leases into regional Redis
  - commits local quota with a small placeholder cost and queues usage reports after successful requests
  - refunds local quota for aborted or 5xx requests
  - sends a later correction event with real `ActualCost`/token data after sub2api `RecordUsage` completes
  - fills the existing `service.APIKey/User/Group` context expected by handlers
- regional Redis Lua reserve/commit/refund helpers
- regional Redis committed-usage adjustment so real-cost corrections update local `spent_cents`
  - if a correction arrives before placeholder commit, Redis stores `corrected_actual_cents` and commit uses it
- regional Redis key snapshot cache
- regional Redis usage queue:
  - requests enqueue `UsageEvent` into `gl:usage:pending`
  - workers atomically move events into `gl:usage:processing`
  - successful reports are removed from processing
  - failed reports are retried with capped backoff
- regional Redis config snapshot cache:
  - syncs `/internal/config/snapshot` from the control plane
  - stores the latest snapshot at `gl:config:snapshot`
  - stores the version at `gl:config:version`
  - applies account/group snapshots into sub2api `sched:*` scheduler cache buckets
  - writes lightweight group metadata into scheduler Redis for gateway-lite routing checks
- gateway-lite scheduler runs cache-only:
  - startup/outbox/full DB rebuild workers are disabled
  - scheduler cache misses return `ErrSchedulerCacheNotReady`
  - OpenAI selected-account rechecks use scheduler cache instead of local account DB
  - group lookups use request context or scheduler Redis group snapshots instead of local DB
- control-plane group snapshots include gateway routing fields:
  - Claude Code fallback groups
  - privacy-set requirement
  - model routing
  - image generation controls
  - OpenAI Messages dispatch/model-list config
  - group RPM limits
- gateway-lite API key auth context hydrates `service.Group` from the control-plane key snapshot
- control-plane server implementation
- control-plane idempotent usage correction by `request_id`, applying only balance/lease/ledger deltas
  - late placeholder events do not overwrite a richer real-cost correction
- `/v1beta` Gemini-compatible gateway routes use the same remote gateway-lite auth
- one-shot control-plane importer for existing sub2api admin DB config:
  - imports non-deleted groups, accounts, and account-group bindings
  - preserves credentials/extra JSON, model routing, privacy requirements, fallback groups, image controls, Messages dispatch config, models-list config, and RPM limits
  - supports dry-run by default and writes only with `IMPORT_APPLY=true`

Not implemented yet:

- physical removal of unused user/admin/payment source files
- periodic sync mode for migration windows where the old sub2api admin DB still changes

## Recommended Next Step

The gateway now reports real usage corrections after `RecordUsage` calculates
cost:

```text
placeholder reserve/commit -> usage queue -> control-plane
RecordUsage actual cost -> local Redis spent delta -> usage queue -> control-plane delta
```

The next major backend task is running the importer against a real sub2api admin
database, starting a gateway-lite node against the imported snapshot, and
verifying an end-to-end request through Redis scheduler cache and control-plane
usage reporting.

Hot-path request flow:

```text
client key -> Redis key cache -> Redis lease reserve -> upstream
                      |                 |
                      v                 v
              control-plane        acquire/refill lease
```
