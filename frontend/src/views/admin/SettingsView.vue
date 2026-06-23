<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-12">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <form v-else class="space-y-6" novalidate @submit.prevent="saveSettings">
        <div class="settings-tabs-shell">
          <nav class="settings-tabs-scroll" role="tablist" aria-label="Gateway settings">
            <div class="settings-tabs">
              <button
                v-for="tab in settingsTabs"
                :id="`settings-tab-${tab.key}`"
                :key="tab.key"
                type="button"
                role="tab"
                :aria-selected="activeTab === tab.key"
                :tabindex="activeTab === tab.key ? 0 : -1"
                :class="['settings-tab', activeTab === tab.key && 'settings-tab-active']"
                @click="selectSettingsTab(tab.key)"
                @keydown="handleSettingsTabKeydown($event, tab.key)"
              >
                <span class="settings-tab-icon">
                  <Icon :name="tab.icon" size="sm" />
                </span>
                <span class="settings-tab-label">{{ tab.label }}</span>
              </button>
            </div>
          </nav>
        </div>

        <div v-show="activeTab === 'general'" class="space-y-6">
          <section class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ localText("基础信息", "General") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ localText("只保留网关后台需要展示和复用的站点配置。", "Gateway admin display settings.") }}
              </p>
            </div>
            <div class="grid gap-5 p-6 md:grid-cols-2">
              <label class="field">
                <span>{{ localText("站点名称", "Site name") }}</span>
                <input v-model.trim="form.site_name" class="input" placeholder="JnmGatewayApi" />
              </label>
              <label class="field">
                <span>{{ localText("站点副标题", "Site subtitle") }}</span>
                <input v-model.trim="form.site_subtitle" class="input" />
              </label>
              <label class="field">
                <span>{{ localText("API 基础地址", "API base URL") }}</span>
                <input v-model.trim="form.api_base_url" class="input" placeholder="https://api.example.com" />
              </label>
              <label class="field">
                <span>{{ localText("文档地址", "Documentation URL") }}</span>
                <input v-model.trim="form.doc_url" class="input" placeholder="https://docs.example.com" />
              </label>
              <label class="field md:col-span-2">
                <span>{{ localText("Logo 地址", "Logo URL") }}</span>
                <input v-model.trim="form.site_logo" class="input" />
              </label>
              <label class="field md:col-span-2">
                <span>{{ localText("联系方式", "Contact information") }}</span>
                <textarea v-model.trim="form.contact_info" class="textarea min-h-[88px]" />
              </label>
            </div>
          </section>

          <section class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ localText("后台体验", "Admin experience") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ localText("保留表格和后台模式相关设置，不再展示客户侧首页、自定义菜单和注册入口。", "Gateway admin table and backend mode settings.") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="setting-row">
                <div>
                  <h3>{{ localText("后台模式", "Backend mode") }}</h3>
                  <p>{{ localText("启用后以后台管理体验为主。", "Use admin-first behavior.") }}</p>
                </div>
                <Toggle v-model="form.backend_mode_enabled" />
              </div>
              <div class="setting-row">
                <div>
                  <h3>{{ localText("隐藏 CCS 导入按钮", "Hide CCS import button") }}</h3>
                  <p>{{ localText("减少轻量网关后台里不需要的导入入口。", "Remove unused import entry from gateway admin.") }}</p>
                </div>
                <Toggle v-model="form.hide_ccs_import_button" />
              </div>
              <div class="grid gap-5 md:grid-cols-2">
                <label class="field">
                  <span>{{ localText("默认分页大小", "Default page size") }}</span>
                  <input
                    v-model.number="form.table_default_page_size"
                    class="input"
                    type="number"
                    min="5"
                    max="1000"
                  />
                </label>
                <label class="field">
                  <span>{{ localText("分页选项", "Page size options") }}</span>
                  <input v-model.trim="tablePageSizeOptionsInput" class="input" placeholder="10, 20, 50, 100" />
                </label>
              </div>
            </div>
          </section>
        </div>

        <div v-show="activeTab === 'features'" class="space-y-6">
          <section class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ localText("网关功能开关", "Gateway feature switches") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ localText("这里只保留网关真实需要的运行开关。", "Only runtime switches used by gateway-lite are kept.") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="setting-row">
                <div>
                  <h3>{{ localText("渠道监控", "Channel monitor") }}</h3>
                  <p>{{ localText("允许后台执行渠道探活、可用性检查和监控任务。", "Enable channel availability checks.") }}</p>
                </div>
                <Toggle v-model="form.channel_monitor_enabled" />
              </div>
              <label v-if="form.channel_monitor_enabled" class="field max-w-xs">
                <span>{{ localText("默认监控间隔（秒）", "Default interval seconds") }}</span>
                <input
                  v-model.number="form.channel_monitor_default_interval_seconds"
                  class="input"
                  type="number"
                  min="10"
                  max="86400"
                />
              </label>
              <div class="setting-row">
                <div>
                  <h3>{{ localText("风控中心", "Risk control") }}</h3>
                  <p>{{ localText("启用请求拦截、限流和安全策略配置。", "Enable request blocking and risk policies.") }}</p>
                </div>
                <Toggle v-model="form.risk_control_enabled" />
              </div>
              <div class="setting-row">
                <div>
                  <h3>{{ localText("会话阻断", "Cyber session block") }}</h3>
                  <p>{{ localText("对命中风控的会话设置临时阻断时间。", "Temporarily block sessions matched by risk rules.") }}</p>
                </div>
                <Toggle v-model="form.cyber_session_block_enabled" />
              </div>
              <label v-if="form.cyber_session_block_enabled" class="field max-w-xs">
                <span>{{ localText("阻断时长（秒）", "Block TTL seconds") }}</span>
                <input
                  v-model.number="form.cyber_session_block_ttl_seconds"
                  class="input"
                  type="number"
                  min="60"
                  max="604800"
                />
              </label>
            </div>
          </section>
        </div>

        <div v-show="activeTab === 'controlPlane'" class="space-y-6">
          <section class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ localText("主站连接", "Control plane") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ localText("保存后会写入 config.yaml，并立即应用到当前运行中的轻量网关进程。", "Saving writes config.yaml and applies the settings to the running gateway-lite process immediately.") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div v-if="gatewayLiteConfigLoading" class="inline-loading">
                <span class="spinner-sm"></span>
                {{ localText("正在读取主站配置", "Loading control plane config") }}
              </div>
              <template v-else>
                <div class="grid gap-5 md:grid-cols-2">
                  <label class="field">
                    <span>{{ localText("主站 API 地址", "Control plane URL") }}</span>
                    <input v-model.trim="gatewayLiteConfigForm.control_plane_url" class="input" placeholder="http://127.0.0.1:8088" />
                  </label>
                  <label class="field">
                    <span>{{ localText("主站内部令牌", "Control plane token") }}</span>
                    <input
                      v-model.trim="gatewayLiteConfigForm.control_plane_token"
                      class="input"
                      type="password"
                      :placeholder="gatewayLiteConfigForm.control_plane_token_configured ? localText('已配置，留空表示不修改', 'Configured; leave blank to keep') : localText('未配置', 'Not configured')"
                    />
                  </label>
                  <label class="field">
                    <span>{{ localText("区域标识", "Region") }}</span>
                    <input v-model.trim="gatewayLiteConfigForm.region" class="input" placeholder="hk" />
                  </label>
                  <label class="field">
                    <span>{{ localText("网关编号", "Gateway code") }}</span>
                    <input v-model.trim="gatewayLiteConfigForm.gateway_code" class="input" placeholder="openai-hk-t1" />
                  </label>
                  <label class="field">
                    <span>{{ localText("Redis 前缀", "Redis prefix") }}</span>
                    <input v-model.trim="gatewayLiteConfigForm.redis_prefix" class="input" placeholder="jnm:gateway-lite" />
                  </label>
                  <label class="field">
                    <span>{{ localText("主站请求超时（毫秒）", "Timeout milliseconds") }}</span>
                    <input v-model.number="gatewayLiteConfigForm.control_plane_timeout_ms" class="input" type="number" min="100" max="30000" />
                  </label>
                  <label class="field">
                    <span>{{ localText("运行心跳间隔（秒）", "Runtime heartbeat seconds") }}</span>
                    <input v-model.number="gatewayLiteConfigForm.runtime_health_interval_seconds" class="input" type="number" min="5" max="3600" />
                  </label>
                  <label class="field">
                    <span>{{ localText("活跃窗口（秒）", "Active window seconds") }}</span>
                    <input v-model.number="gatewayLiteConfigForm.runtime_active_window_seconds" class="input" type="number" min="30" max="86400" />
                  </label>
                  <label class="field">
                    <span>{{ localText("主站配置同步间隔（秒）", "Config sync seconds") }}</span>
                    <input v-model.number="gatewayLiteConfigForm.config_sync_interval_seconds" class="input" type="number" min="5" max="3600" />
                  </label>
                  <label class="field">
                    <span>{{ localText("缓存失效同步间隔（秒）", "Cache invalidation seconds") }}</span>
                    <input v-model.number="gatewayLiteConfigForm.cache_invalidation_interval_seconds" class="input" type="number" min="1" max="3600" />
                  </label>
                  <label class="field">
                    <span>{{ localText("用量队列积压告警阈值", "Pending queue alert threshold") }}</span>
                    <input v-model.number="gatewayLiteConfigForm.usage_queue_pending_alert_threshold" class="input" type="number" min="1" />
                  </label>
                  <label class="field">
                    <span>{{ localText("死信队列告警阈值", "Dead queue alert threshold") }}</span>
                    <input v-model.number="gatewayLiteConfigForm.usage_queue_dead_alert_threshold" class="input" type="number" min="1" />
                  </label>
                </div>
                <div
                  class="rounded-lg border p-4 text-sm"
                  :class="gatewayLiteConfigForm.restart_required
                    ? 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300'
                    : 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-900/20 dark:text-emerald-300'"
                >
                  {{ localText("配置文件：", "Config file:") }}{{ gatewayLiteConfigForm.config_path || "config.yaml" }}。
                  <span v-if="gatewayLiteConfigForm.restart_required">
                    {{ localText("配置已保存，当前版本会尽量热更新；如遇旧队列或旧缓存残留，可手动同步一次。", "Config saved and hot-applied where possible; manually sync once if old queue or cache state remains.") }}
                  </span>
                  <span v-else>
                    {{ localText("所有主站连接配置已即时生效，并已触发一次配置同步。", "All control-plane settings applied immediately and triggered a config sync.") }}
                  </span>
                </div>
                <div class="flex justify-end">
                  <button type="button" class="btn btn-secondary btn-sm" :disabled="gatewayLiteConfigSaving" @click="saveGatewayLiteConfig">
                    <span v-if="gatewayLiteConfigSaving" class="spinner-sm"></span>
                    <Icon v-else name="checkCircle" size="xs" />
                    {{ gatewayLiteConfigSaving ? localText("保存中", "Saving") : localText("保存主站配置", "Save control plane config") }}
                  </button>
                </div>
              </template>
            </div>
          </section>
        </div>

        <div v-show="activeTab === 'security'" class="space-y-6">
          <section class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ localText("管理员 API Key", "Admin API Key") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ localText("用于脚本或运维工具调用后台管理接口。", "Used by admin scripts and operation tooling.") }}
              </p>
            </div>
            <div class="space-y-4 p-6">
              <div class="rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-900/20">
                <div class="flex items-start gap-3">
                  <Icon name="exclamationTriangle" size="md" class="mt-0.5 flex-shrink-0 text-amber-500" />
                  <p class="text-sm text-amber-700 dark:text-amber-300">
                    {{ localText("管理员密钥只会完整显示一次，请妥善保存。", "The full admin key is shown once. Store it securely.") }}
                  </p>
                </div>
              </div>
              <div v-if="adminApiKeyLoading" class="inline-loading">
                <span class="spinner-sm"></span>
                {{ localText("正在读取密钥状态", "Loading key status") }}
              </div>
              <div v-else-if="!adminApiKeyExists" class="flex items-center justify-between gap-4">
                <span class="text-sm text-gray-500 dark:text-gray-400">
                  {{ localText("当前未配置管理员 API Key。", "No admin API key configured.") }}
                </span>
                <button type="button" class="btn btn-primary btn-sm" :disabled="adminApiKeyOperating" @click="createAdminApiKey">
                  {{ adminApiKeyOperating ? localText("生成中", "Creating") : localText("生成密钥", "Create key") }}
                </button>
              </div>
              <div v-else class="space-y-4">
                <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <code class="rounded bg-gray-100 px-2 py-1 font-mono text-sm text-gray-900 dark:bg-dark-700 dark:text-gray-100">
                    {{ adminApiKeyMasked }}
                  </code>
                  <div class="flex flex-wrap gap-2">
                    <button type="button" class="btn btn-secondary btn-sm" :disabled="adminApiKeyOperating" @click="regenerateAdminApiKey">
                      {{ localText("重新生成", "Regenerate") }}
                    </button>
                    <button
                      type="button"
                      class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
                      :disabled="adminApiKeyOperating"
                      @click="deleteAdminApiKey"
                    >
                      {{ localText("删除", "Delete") }}
                    </button>
                  </div>
                </div>
                <div v-if="newAdminApiKey" class="space-y-3 rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-800 dark:bg-green-900/20">
                  <p class="text-sm font-medium text-green-700 dark:text-green-300">
                    {{ localText("新密钥如下，离开页面后不会再次显示。", "This key will not be shown again.") }}
                  </p>
                  <div class="flex flex-col gap-2 sm:flex-row">
                    <code class="flex-1 select-all break-all rounded border border-green-300 bg-white px-3 py-2 font-mono text-sm dark:border-green-700 dark:bg-dark-800">
                      {{ newAdminApiKey }}
                    </code>
                    <button type="button" class="btn btn-primary btn-sm flex-shrink-0" @click="copyNewKey">
                      <Icon name="copy" size="xs" />
                      {{ localText("复制", "Copy") }}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ localText("访问安全", "Access security") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ localText("保留管理员登录和 API Key 鉴权相关安全项。", "Security settings used by admin login and API key access.") }}
              </p>
            </div>
            <div class="space-y-5 p-6">
              <div class="setting-row">
                <div>
                  <h3>{{ localText("Cloudflare Turnstile 人机验证", "Cloudflare Turnstile") }}</h3>
                  <p>{{ localText("用于登录页的人机验证。", "Human verification on the login page.") }}</p>
                </div>
                <Toggle v-model="form.turnstile_enabled" />
              </div>
              <div v-if="form.turnstile_enabled" class="grid gap-5 md:grid-cols-2">
                <label class="field">
                  <span>{{ localText("Site Key（站点密钥）", "Site key") }}</span>
                  <input v-model.trim="form.turnstile_site_key" class="input" />
                </label>
                <label class="field">
                  <span>{{ localText("Secret Key（私密密钥）", "Secret key") }}</span>
                  <input
                    v-model.trim="form.turnstile_secret_key"
                    class="input"
                    type="password"
                    :placeholder="form.turnstile_secret_key_configured ? localText('已配置，留空表示不修改', 'Configured. Leave blank to keep it.') : ''"
                  />
                </label>
              </div>
              <div class="setting-row">
                <div>
                  <h3>{{ localText("信任转发 IP", "Trust forwarded IP") }}</h3>
                  <p>{{ localText("网关前有反向代理时，允许 API Key ACL 使用转发来源 IP。", "Allow API key ACL to use forwarded source IP behind a proxy.") }}</p>
                </div>
                <Toggle v-model="form.api_key_acl_trust_forwarded_ip" />
              </div>
            </div>
          </section>
        </div>

        <div v-show="activeTab === 'gateway'" class="space-y-6">
          <section class="card">
            <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ localText("网关转发策略", "Gateway forwarding") }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ localText("保留模型兜底、身份补丁、缓存控制和客户端兼容策略。", "Fallback, identity patch, cache and client compatibility policies.") }}
              </p>
            </div>
            <div class="space-y-6 p-6">
              <div class="setting-row">
                <div>
                  <h3>{{ localText("启用模型兜底", "Enable model fallback") }}</h3>
                  <p>{{ localText("当请求模型需要重写时使用对应平台兜底模型。", "Use platform fallback models when rewrite is needed.") }}</p>
                </div>
                <Toggle v-model="form.enable_model_fallback" />
              </div>
              <div v-if="form.enable_model_fallback" class="grid gap-5 md:grid-cols-2">
                <label class="field">
                  <span>Anthropic fallback（Anthropic 兜底模型）</span>
                  <input v-model.trim="form.fallback_model_anthropic" class="input" />
                </label>
                <label class="field">
                  <span>OpenAI fallback（OpenAI 兜底模型）</span>
                  <input v-model.trim="form.fallback_model_openai" class="input" />
                </label>
                <label class="field">
                  <span>Gemini fallback（Gemini 兜底模型）</span>
                  <input v-model.trim="form.fallback_model_gemini" class="input" />
                </label>
                <label class="field">
                  <span>Antigravity fallback（Antigravity 兜底模型）</span>
                  <input v-model.trim="form.fallback_model_antigravity" class="input" />
                </label>
              </div>

              <div class="setting-row">
                <div>
                  <h3>{{ localText("身份补丁", "Identity patch") }}</h3>
                  <p>{{ localText("用于 Claude 到 Gemini 等场景的系统提示补丁。", "System prompt patch for Claude-to-Gemini style routing.") }}</p>
                </div>
                <Toggle v-model="form.enable_identity_patch" />
              </div>
              <label v-if="form.enable_identity_patch" class="field">
                <span>{{ localText("身份补丁提示词", "Identity patch prompt") }}</span>
                <textarea v-model="form.identity_patch_prompt" class="textarea min-h-[140px] font-mono text-xs" />
              </label>

              <div class="grid gap-5 md:grid-cols-2">
                <label class="field">
                  <span>{{ localText("最低 Claude Code 版本", "Minimum Claude Code version") }}</span>
                  <input v-model.trim="form.min_claude_code_version" class="input" placeholder="1.0.0" />
                </label>
                <label class="field">
                  <span>{{ localText("最高 Claude Code 版本", "Maximum Claude Code version") }}</span>
                  <input v-model.trim="form.max_claude_code_version" class="input" placeholder="2.0.0" />
                </label>
                <label class="field">
                  <span>{{ localText("Antigravity User-Agent 版本", "Antigravity User-Agent version") }}</span>
                  <input v-model.trim="form.antigravity_user_agent_version" class="input" />
                </label>
                <label class="field">
                  <span>{{ localText("OpenAI Codex User-Agent", "OpenAI Codex User-Agent") }}</span>
                  <input v-model.trim="form.openai_codex_user_agent" class="input" />
                </label>
              </div>

              <div class="grid gap-4 md:grid-cols-2">
                <SwitchCard v-model="form.allow_ungrouped_key_scheduling" :title="localText('允许未分组 Key 调度', 'Allow ungrouped key scheduling')" />
                <SwitchCard v-model="form.openai_advanced_scheduler_enabled" :title="localText('OpenAI 高级调度', 'OpenAI advanced scheduler')" />
                <SwitchCard v-model="form.enable_fingerprint_unification" :title="localText('统一指纹', 'Fingerprint unification')" />
                <SwitchCard v-model="form.enable_metadata_passthrough" :title="localText('透传 metadata', 'Metadata passthrough')" />
                <SwitchCard v-model="form.enable_cch_signing" :title="localText('CCH 签名', 'CCH signing')" />
                <SwitchCard v-model="form.enable_anthropic_cache_ttl_1h_injection" :title="localText('注入 1h 缓存 TTL', 'Inject 1h cache TTL')" />
                <SwitchCard v-model="form.rewrite_message_cache_control" :title="localText('重写消息缓存控制', 'Rewrite message cache control')" />
                <SwitchCard v-model="form.openai_allow_claude_code_codex_plugin" :title="localText('允许 Claude Code Codex 插件', 'Allow Claude Code Codex plugin')" />
              </div>

              <div class="setting-row">
                <div>
                  <h3>{{ localText("Claude OAuth 系统提示词注入", "Claude OAuth system prompt injection") }}</h3>
                  <p>{{ localText("保留网关转发层的系统提示词注入能力。", "Keep gateway-level system prompt injection.") }}</p>
                </div>
                <Toggle v-model="form.enable_claude_oauth_system_prompt_injection" />
              </div>
              <div v-if="form.enable_claude_oauth_system_prompt_injection" class="space-y-5">
                <label class="field">
                  <span>{{ localText("系统提示词", "System prompt") }}</span>
                  <textarea v-model="form.claude_oauth_system_prompt" class="textarea min-h-[140px] font-mono text-xs" />
                </label>
                <label class="field">
                  <span>{{ localText("系统提示词 blocks JSON", "System prompt blocks JSON") }}</span>
                  <textarea v-model="form.claude_oauth_system_prompt_blocks" class="textarea min-h-[160px] font-mono text-xs" />
                </label>
              </div>
            </div>
          </section>
        </div>

        <div v-show="activeTab === 'policy'" class="space-y-6">
          <PolicyCard
            :title="localText('529 过载冷却', '529 overload cooldown')"
            :description="localText('上游过载后临时冷却，避免持续打满故障渠道。', 'Temporarily cool down overloaded upstream channels.')"
            :loading="overloadCooldownLoading"
            :saving="overloadCooldownSaving"
            @save="saveOverloadCooldownSettings"
          >
            <div class="setting-row">
              <div>
                <h3>{{ localText("启用", "Enabled") }}</h3>
              </div>
              <Toggle v-model="overloadCooldownForm.enabled" />
            </div>
            <label v-if="overloadCooldownForm.enabled" class="field max-w-xs">
              <span>{{ localText("冷却分钟", "Cooldown minutes") }}</span>
              <input v-model.number="overloadCooldownForm.cooldown_minutes" class="input" type="number" min="1" max="120" />
            </label>
          </PolicyCard>

          <PolicyCard
            :title="localText('429 限流冷却', '429 rate-limit cooldown')"
            :description="localText('请求被限流后短暂冷却，减少无效重试。', 'Cool down briefly after rate-limit responses.')"
            :loading="rateLimit429CooldownLoading"
            :saving="rateLimit429CooldownSaving"
            @save="saveRateLimit429CooldownSettings"
          >
            <div class="setting-row">
              <div>
                <h3>{{ localText("启用", "Enabled") }}</h3>
              </div>
              <Toggle v-model="rateLimit429CooldownForm.enabled" />
            </div>
            <label v-if="rateLimit429CooldownForm.enabled" class="field max-w-xs">
              <span>{{ localText("冷却秒数", "Cooldown seconds") }}</span>
              <input v-model.number="rateLimit429CooldownForm.cooldown_seconds" class="input" type="number" min="1" max="3600" />
            </label>
          </PolicyCard>

          <PolicyCard
            :title="localText('账号兜底排队', 'Account fallback queue')"
            :description="localText('所有可用账号并发满时，允许请求短暂排队等待账号槽位；关闭后满并发会立即返回并发限制。', 'When all eligible accounts are full, queue briefly for an account slot; disabled means fail fast on concurrency limits.')"
            :loading="fallbackQueueLoading"
            :saving="fallbackQueueSaving"
            @save="saveFallbackQueueSettings"
          >
            <div class="setting-row">
              <div>
                <h3>{{ localText("启用", "Enabled") }}</h3>
                <p>{{ localText("仅影响账号级兜底排队，不改变套餐并发和单账号并发上限。", "Only affects account-level fallback queueing, not plan concurrency or per-account concurrency.") }}</p>
              </div>
              <Toggle v-model="fallbackQueueForm.enabled" />
            </div>
            <label v-if="fallbackQueueForm.enabled" class="field max-w-xs">
              <span>{{ localText("单账号最大排队请求数", "Max queued requests per account") }}</span>
              <input v-model.number="fallbackQueueForm.max_waiting" class="input" type="number" min="1" max="100000" />
            </label>
            <label v-if="fallbackQueueForm.enabled" class="field max-w-xs">
              <span>{{ localText("最长等待秒数", "Max wait seconds") }}</span>
              <input v-model.number="fallbackQueueForm.wait_timeout_seconds" class="input" type="number" min="1" max="3600" />
            </label>
          </PolicyCard>

          <PolicyCard
            :title="localText('流超时处理', 'Stream timeout')"
            :description="localText('连续流式超时后执行临时下线、报错或忽略。', 'Handle repeated stream timeouts.')"
            :loading="streamTimeoutLoading"
            :saving="streamTimeoutSaving"
            @save="saveStreamTimeoutSettings"
          >
            <div class="setting-row">
              <div>
                <h3>{{ localText("启用", "Enabled") }}</h3>
              </div>
              <Toggle v-model="streamTimeoutForm.enabled" />
            </div>
            <div v-if="streamTimeoutForm.enabled" class="grid gap-5 md:grid-cols-2">
              <label class="field">
                <span>{{ localText("动作", "Action") }}</span>
                <select v-model="streamTimeoutForm.action" class="input">
                  <option value="temp_unsched">{{ localText("临时下线", "Temporary unschedule") }}</option>
                  <option value="error">{{ localText("直接报错", "Return error") }}</option>
                  <option value="none">{{ localText("仅记录", "Record only") }}</option>
                </select>
              </label>
              <label class="field">
                <span>{{ localText("临时下线分钟", "Unschedule minutes") }}</span>
                <input v-model.number="streamTimeoutForm.temp_unsched_minutes" class="input" type="number" min="1" max="1440" />
              </label>
              <label class="field">
                <span>{{ localText("阈值次数", "Threshold count") }}</span>
                <input v-model.number="streamTimeoutForm.threshold_count" class="input" type="number" min="1" max="100" />
              </label>
              <label class="field">
                <span>{{ localText("统计窗口分钟", "Window minutes") }}</span>
                <input v-model.number="streamTimeoutForm.threshold_window_minutes" class="input" type="number" min="1" max="1440" />
              </label>
            </div>
          </PolicyCard>

          <PolicyCard
            :title="localText('修正器', 'Rectifier')"
            :description="localText('修正上游响应中的 thinking、budget 和 API Key 签名问题。', 'Rectify thinking, budget and API key signature issues.')"
            :loading="rectifierLoading"
            :saving="rectifierSaving"
            @save="saveRectifierSettings"
          >
            <div class="grid gap-4 md:grid-cols-2">
              <SwitchCard v-model="rectifierForm.enabled" :title="localText('启用修正器', 'Enable rectifier')" />
              <SwitchCard v-model="rectifierForm.thinking_signature_enabled" :title="localText('Thinking 签名修正', 'Thinking signature rectifier')" />
              <SwitchCard v-model="rectifierForm.thinking_budget_enabled" :title="localText('Thinking budget 修正', 'Thinking budget rectifier')" />
              <SwitchCard v-model="rectifierForm.apikey_signature_enabled" :title="localText('API Key 签名修正', 'API key signature rectifier')" />
            </div>
            <label v-if="rectifierForm.apikey_signature_enabled" class="field">
              <span>{{ localText("API Key 签名模式，每行一个", "API key signature patterns, one per line") }}</span>
              <textarea v-model="rectifierPatternsInput" class="textarea min-h-[120px] font-mono text-xs" />
            </label>
          </PolicyCard>

          <PolicyCard
            :title="localText('Beta 策略', 'Beta policy')"
            :description="localText('控制 Anthropic beta token 的透传、过滤或阻断。', 'Pass, filter or block Anthropic beta tokens.')"
            :loading="betaPolicyLoading"
            :saving="betaPolicySaving"
            @save="saveBetaPolicySettings"
          >
            <div class="space-y-4">
              <button type="button" class="btn btn-secondary btn-sm" @click="addBetaPolicyRule">
                <Icon name="plus" size="xs" />
                {{ localText("添加规则", "Add rule") }}
              </button>
              <div v-if="betaPolicyForm.rules.length === 0" class="empty-hint">
                {{ localText("当前没有 beta 策略规则。", "No beta policy rules configured.") }}
              </div>
              <div
                v-for="(rule, index) in betaPolicyForm.rules"
                :key="index"
                class="rounded-lg border border-gray-200 p-4 dark:border-dark-700"
              >
                <div class="grid gap-4 md:grid-cols-4">
                  <label class="field md:col-span-2">
                    <span>Beta token</span>
                    <input v-model.trim="rule.beta_token" class="input" placeholder="context-1m-2025-08-07" />
                  </label>
                  <label class="field">
                    <span>{{ localText("动作", "Action") }}</span>
                    <select v-model="rule.action" class="input">
                      <option value="pass">{{ localText("透传", "Pass") }}</option>
                      <option value="filter">{{ localText("过滤", "Filter") }}</option>
                      <option value="block">{{ localText("阻断", "Block") }}</option>
                    </select>
                  </label>
                  <label class="field">
                    <span>{{ localText("范围", "Scope") }}</span>
                    <select v-model="rule.scope" class="input">
                      <option value="all">{{ localText("全部", "All") }}</option>
                      <option value="oauth">OAuth</option>
                      <option value="apikey">API Key</option>
                      <option value="bedrock">Bedrock</option>
                    </select>
                  </label>
                  <label class="field md:col-span-2">
                    <span>{{ localText("模型白名单，逗号分隔", "Model whitelist, comma-separated") }}</span>
                    <input :value="(rule.model_whitelist || []).join(', ')" class="input" @input="updateBetaWhitelist(rule, $event)" />
                  </label>
                  <label class="field">
                    <span>{{ localText("兜底动作", "Fallback action") }}</span>
                    <select v-model="rule.fallback_action" class="input">
                      <option value="pass">{{ localText("透传", "Pass") }}</option>
                      <option value="filter">{{ localText("过滤", "Filter") }}</option>
                      <option value="block">{{ localText("阻断", "Block") }}</option>
                    </select>
                  </label>
                  <label class="field">
                    <span>{{ localText("错误提示", "Error message") }}</span>
                    <input v-model.trim="rule.error_message" class="input" />
                  </label>
                </div>
                <div class="mt-4 flex justify-end">
                  <button type="button" class="btn btn-secondary btn-sm text-red-600 dark:text-red-400" @click="removeBetaPolicyRule(index)">
                    <Icon name="trash" size="xs" />
                    {{ localText("删除规则", "Delete rule") }}
                  </button>
                </div>
              </div>
            </div>
          </PolicyCard>
        </div>

        <div class="flex justify-end">
          <button type="submit" :disabled="saving || loadFailed" class="btn btn-primary">
            <span v-if="saving" class="spinner-sm"></span>
            <Icon v-else name="checkCircle" size="xs" />
            {{ saving ? localText("保存中", "Saving") : localText("保存设置", "Save settings") }}
          </button>
        </div>
      </form>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import AppLayout from "@/components/layout/GatewayLiteLayout.vue";
import Icon from "@/components/icons/Icon.vue";
import Toggle from "@/components/common/Toggle.vue";
import type {
  BetaPolicyRule,
  GatewayLiteControlPlaneConfig,
  SystemSettings,
  UpdateGatewayLiteControlPlaneConfig,
  UpdateSettingsRequest,
} from "@/api/admin/settings";
import { extractApiErrorMessage } from "@/utils/apiError";
import { useAppStore } from "@/stores/app";
import { useAdminSettingsStore } from "@/stores/adminSettings";

type SettingsTab = "general" | "features" | "controlPlane" | "security" | "gateway" | "policy";
type IconName = InstanceType<typeof Icon>["$props"]["name"];

interface GatewaySettingsForm {
  site_name: string;
  site_logo: string;
  site_subtitle: string;
  api_base_url: string;
  contact_info: string;
  doc_url: string;
  hide_ccs_import_button: boolean;
  table_default_page_size: number;
  table_page_size_options: number[];
  backend_mode_enabled: boolean;
  turnstile_enabled: boolean;
  turnstile_site_key: string;
  turnstile_secret_key: string;
  turnstile_secret_key_configured: boolean;
  api_key_acl_trust_forwarded_ip: boolean;
  enable_model_fallback: boolean;
  fallback_model_anthropic: string;
  fallback_model_openai: string;
  fallback_model_gemini: string;
  fallback_model_antigravity: string;
  enable_identity_patch: boolean;
  identity_patch_prompt: string;
  min_claude_code_version: string;
  max_claude_code_version: string;
  allow_ungrouped_key_scheduling: boolean;
  enable_fingerprint_unification: boolean;
  enable_metadata_passthrough: boolean;
  enable_cch_signing: boolean;
  enable_claude_oauth_system_prompt_injection: boolean;
  claude_oauth_system_prompt: string;
  claude_oauth_system_prompt_blocks: string;
  enable_anthropic_cache_ttl_1h_injection: boolean;
  rewrite_message_cache_control: boolean;
  antigravity_user_agent_version: string;
  openai_codex_user_agent: string;
  openai_allow_claude_code_codex_plugin: boolean;
  openai_advanced_scheduler_enabled: boolean;
  risk_control_enabled: boolean;
  cyber_session_block_enabled: boolean;
  cyber_session_block_ttl_seconds: number;
  channel_monitor_enabled: boolean;
  channel_monitor_default_interval_seconds: number;
}

interface GatewayLiteConfigForm extends GatewayLiteControlPlaneConfig {
  control_plane_token: string;
}

const { locale } = useI18n();
const appStore = useAppStore();
const adminSettingsStore = useAdminSettingsStore();

const activeTab = ref<SettingsTab>("general");
const loading = ref(true);
const loadFailed = ref(false);
const saving = ref(false);
const tablePageSizeOptionsInput = ref("10, 20, 50, 100");
const gatewayLiteConfigLoading = ref(true);
const gatewayLiteConfigSaving = ref(false);

const form = reactive<GatewaySettingsForm>({
  site_name: "JnmGatewayApi",
  site_logo: "",
  site_subtitle: "",
  api_base_url: "",
  contact_info: "",
  doc_url: "",
  hide_ccs_import_button: false,
  table_default_page_size: 20,
  table_page_size_options: [10, 20, 50, 100],
  backend_mode_enabled: true,
  turnstile_enabled: false,
  turnstile_site_key: "",
  turnstile_secret_key: "",
  turnstile_secret_key_configured: false,
  api_key_acl_trust_forwarded_ip: false,
  enable_model_fallback: false,
  fallback_model_anthropic: "",
  fallback_model_openai: "",
  fallback_model_gemini: "",
  fallback_model_antigravity: "",
  enable_identity_patch: false,
  identity_patch_prompt: "",
  min_claude_code_version: "",
  max_claude_code_version: "",
  allow_ungrouped_key_scheduling: false,
  enable_fingerprint_unification: false,
  enable_metadata_passthrough: false,
  enable_cch_signing: false,
  enable_claude_oauth_system_prompt_injection: false,
  claude_oauth_system_prompt: "",
  claude_oauth_system_prompt_blocks: "",
  enable_anthropic_cache_ttl_1h_injection: false,
  rewrite_message_cache_control: false,
  antigravity_user_agent_version: "",
  openai_codex_user_agent: "",
  openai_allow_claude_code_codex_plugin: false,
  openai_advanced_scheduler_enabled: false,
  risk_control_enabled: true,
  cyber_session_block_enabled: false,
  cyber_session_block_ttl_seconds: 3600,
  channel_monitor_enabled: true,
  channel_monitor_default_interval_seconds: 60,
});

const gatewayLiteConfigForm = reactive<GatewayLiteConfigForm>({
  region: "default",
  gateway_code: "",
  redis_prefix: "",
  control_plane_url: "",
  control_plane_token: "",
  control_plane_token_configured: false,
  control_plane_timeout_ms: 1000,
  runtime_health_interval_seconds: 15,
  runtime_active_window_seconds: 300,
  config_sync_interval_seconds: 30,
  cache_invalidation_interval_seconds: 5,
  usage_queue_pending_alert_threshold: 1000,
  usage_queue_dead_alert_threshold: 1,
  config_path: "config.yaml",
  restart_required: false,
});

const settingsTabs = computed<Array<{ key: SettingsTab; label: string; icon: IconName }>>(() => [
  { key: "general", label: localText("基础", "General"), icon: "cog" },
  { key: "features", label: localText("功能", "Features"), icon: "bolt" },
  { key: "controlPlane", label: localText("主站", "Control"), icon: "server" },
  { key: "security", label: localText("安全", "Security"), icon: "shield" },
  { key: "gateway", label: localText("网关", "Gateway"), icon: "database" },
  { key: "policy", label: localText("策略", "Policy"), icon: "clock" },
]);

const settingsTabKeyboardActions = {
  ArrowLeft: -1,
  ArrowUp: -1,
  ArrowRight: 1,
  ArrowDown: 1,
  Home: "first",
  End: "last",
} as const;

const adminApiKeyLoading = ref(true);
const adminApiKeyExists = ref(false);
const adminApiKeyMasked = ref("");
const adminApiKeyOperating = ref(false);
const newAdminApiKey = ref("");

const overloadCooldownLoading = ref(true);
const overloadCooldownSaving = ref(false);
const overloadCooldownForm = reactive({
  enabled: true,
  cooldown_minutes: 10,
});

const rateLimit429CooldownLoading = ref(true);
const rateLimit429CooldownSaving = ref(false);
const rateLimit429CooldownForm = reactive({
  enabled: true,
  cooldown_seconds: 5,
});

const fallbackQueueLoading = ref(true);
const fallbackQueueSaving = ref(false);
const fallbackQueueForm = reactive({
  enabled: true,
  max_waiting: 100,
  wait_timeout_seconds: 120,
});

const streamTimeoutLoading = ref(true);
const streamTimeoutSaving = ref(false);
const streamTimeoutForm = reactive({
  enabled: true,
  action: "temp_unsched" as "temp_unsched" | "error" | "none",
  temp_unsched_minutes: 5,
  threshold_count: 3,
  threshold_window_minutes: 10,
});

const rectifierLoading = ref(true);
const rectifierSaving = ref(false);
const rectifierPatternsInput = ref("");
const rectifierForm = reactive({
  enabled: true,
  thinking_signature_enabled: true,
  thinking_budget_enabled: true,
  apikey_signature_enabled: false,
  apikey_signature_patterns: [] as string[],
});

const betaPolicyLoading = ref(true);
const betaPolicySaving = ref(false);
const betaPolicyForm = reactive({
  rules: [] as BetaPolicyRule[],
});

function localText(zh: string, en: string): string {
  return locale.value.startsWith("zh") ? zh : en;
}

function selectSettingsTab(tab: SettingsTab): void {
  activeTab.value = tab;
}

function handleSettingsTabKeydown(event: KeyboardEvent, tab: SettingsTab): void {
  const action = settingsTabKeyboardActions[event.key as keyof typeof settingsTabKeyboardActions];
  if (action === undefined) return;

  event.preventDefault();
  const visibleTabs = settingsTabs.value;
  const currentIndex = visibleTabs.findIndex((item) => item.key === tab);
  let nextIndex = currentIndex < 0 ? 0 : currentIndex;

  if (action === "first") {
    nextIndex = 0;
  } else if (action === "last") {
    nextIndex = visibleTabs.length - 1;
  } else {
    nextIndex = (nextIndex + action + visibleTabs.length) % visibleTabs.length;
  }

  const nextTab = visibleTabs[nextIndex]?.key;
  if (!nextTab) return;
  activeTab.value = nextTab;
  window.requestAnimationFrame(() => {
    document.getElementById(`settings-tab-${nextTab}`)?.focus();
  });
}

function applySettings(settings: SystemSettings): void {
  const next: Partial<GatewaySettingsForm> = {
    site_name: settings.site_name || "JnmGatewayApi",
    site_logo: settings.site_logo || "",
    site_subtitle: settings.site_subtitle || "",
    api_base_url: settings.api_base_url || "",
    contact_info: settings.contact_info || "",
    doc_url: settings.doc_url || "",
    hide_ccs_import_button: Boolean(settings.hide_ccs_import_button),
    table_default_page_size: normalizeNumber(settings.table_default_page_size, 20, 5, 1000),
    table_page_size_options: normalizePageSizeOptions(settings.table_page_size_options),
    backend_mode_enabled: settings.backend_mode_enabled !== false,
    turnstile_enabled: Boolean(settings.turnstile_enabled),
    turnstile_site_key: settings.turnstile_site_key || "",
    turnstile_secret_key: "",
    turnstile_secret_key_configured: Boolean(settings.turnstile_secret_key_configured),
    api_key_acl_trust_forwarded_ip: Boolean(settings.api_key_acl_trust_forwarded_ip),
    enable_model_fallback: Boolean(settings.enable_model_fallback),
    fallback_model_anthropic: settings.fallback_model_anthropic || "",
    fallback_model_openai: settings.fallback_model_openai || "",
    fallback_model_gemini: settings.fallback_model_gemini || "",
    fallback_model_antigravity: settings.fallback_model_antigravity || "",
    enable_identity_patch: Boolean(settings.enable_identity_patch),
    identity_patch_prompt: settings.identity_patch_prompt || "",
    min_claude_code_version: settings.min_claude_code_version || "",
    max_claude_code_version: settings.max_claude_code_version || "",
    allow_ungrouped_key_scheduling: Boolean(settings.allow_ungrouped_key_scheduling),
    enable_fingerprint_unification: Boolean(settings.enable_fingerprint_unification),
    enable_metadata_passthrough: Boolean(settings.enable_metadata_passthrough),
    enable_cch_signing: Boolean(settings.enable_cch_signing),
    enable_claude_oauth_system_prompt_injection: Boolean(settings.enable_claude_oauth_system_prompt_injection),
    claude_oauth_system_prompt: settings.claude_oauth_system_prompt || "",
    claude_oauth_system_prompt_blocks: settings.claude_oauth_system_prompt_blocks || "",
    enable_anthropic_cache_ttl_1h_injection: Boolean(settings.enable_anthropic_cache_ttl_1h_injection),
    rewrite_message_cache_control: Boolean(settings.rewrite_message_cache_control),
    antigravity_user_agent_version: settings.antigravity_user_agent_version || "",
    openai_codex_user_agent: settings.openai_codex_user_agent || "",
    openai_allow_claude_code_codex_plugin: Boolean(settings.openai_allow_claude_code_codex_plugin),
    openai_advanced_scheduler_enabled: Boolean(settings.openai_advanced_scheduler_enabled),
    risk_control_enabled: settings.risk_control_enabled !== false,
    cyber_session_block_enabled: Boolean(settings.cyber_session_block_enabled),
    cyber_session_block_ttl_seconds: normalizeNumber(settings.cyber_session_block_ttl_seconds, 3600, 60, 604800),
    channel_monitor_enabled: settings.channel_monitor_enabled !== false,
    channel_monitor_default_interval_seconds: normalizeNumber(settings.channel_monitor_default_interval_seconds, 60, 10, 86400),
  };

  Object.assign(form, next);
  tablePageSizeOptionsInput.value = form.table_page_size_options.join(", ");
}

async function loadSettings(): Promise<void> {
  loading.value = true;
  loadFailed.value = false;
  try {
    const settings = await adminAPI.settings.getSettings();
    applySettings(settings);
  } catch (error: unknown) {
    loadFailed.value = true;
    appStore.showError(extractApiErrorMessage(error, localText("读取设置失败", "Failed to load settings")));
  } finally {
    loading.value = false;
  }
}

function applyGatewayLiteConfig(settings: GatewayLiteControlPlaneConfig): void {
  Object.assign(gatewayLiteConfigForm, {
    ...settings,
    control_plane_token: "",
  });
}

async function loadGatewayLiteConfig(): Promise<void> {
  gatewayLiteConfigLoading.value = true;
  try {
    const settings = await adminAPI.settings.getGatewayLiteControlPlaneConfig();
    applyGatewayLiteConfig(settings);
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, localText("读取主站配置失败", "Failed to load control plane config")));
  } finally {
    gatewayLiteConfigLoading.value = false;
  }
}

function buildGatewayLiteConfigPayload(): UpdateGatewayLiteControlPlaneConfig {
  const payload: UpdateGatewayLiteControlPlaneConfig = {
    region: gatewayLiteConfigForm.region.trim() || "default",
    gateway_code: gatewayLiteConfigForm.gateway_code.trim(),
    redis_prefix: gatewayLiteConfigForm.redis_prefix.trim(),
    control_plane_url: gatewayLiteConfigForm.control_plane_url.trim(),
    control_plane_timeout_ms: normalizeNumber(gatewayLiteConfigForm.control_plane_timeout_ms, 1000, 100, 30000),
    runtime_health_interval_seconds: normalizeNumber(gatewayLiteConfigForm.runtime_health_interval_seconds, 15, 5, 3600),
    runtime_active_window_seconds: normalizeNumber(gatewayLiteConfigForm.runtime_active_window_seconds, 300, 30, 86400),
    config_sync_interval_seconds: normalizeNumber(gatewayLiteConfigForm.config_sync_interval_seconds, 30, 5, 3600),
    cache_invalidation_interval_seconds: normalizeNumber(gatewayLiteConfigForm.cache_invalidation_interval_seconds, 5, 1, 3600),
    usage_queue_pending_alert_threshold: normalizeNumber(gatewayLiteConfigForm.usage_queue_pending_alert_threshold, 1000, 1, 10000000),
    usage_queue_dead_alert_threshold: normalizeNumber(gatewayLiteConfigForm.usage_queue_dead_alert_threshold, 1, 1, 10000000),
  };
  if (gatewayLiteConfigForm.control_plane_token.trim()) {
    payload.control_plane_token = gatewayLiteConfigForm.control_plane_token.trim();
  }
  return payload;
}

async function saveGatewayLiteConfig(): Promise<void> {
  gatewayLiteConfigSaving.value = true;
  try {
    const updated = await adminAPI.settings.updateGatewayLiteControlPlaneConfig(buildGatewayLiteConfigPayload());
    applyGatewayLiteConfig(updated);
    appStore.showSuccess(
      updated.restart_required
        ? localText("配置已保存并已热更新，建议手动同步确认最新状态", "Config saved and hot-applied; manually sync to confirm latest state")
        : localText("配置已保存并即时生效，已触发一次配置同步", "Config saved and applied immediately; sync triggered"),
    );
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, localText("保存主站配置失败", "Failed to save control plane config")));
  } finally {
    gatewayLiteConfigSaving.value = false;
  }
}

function buildSettingsPayload(): UpdateSettingsRequest {
  const pageSizeOptions = normalizePageSizeOptions(parsePageSizeOptionsInput(tablePageSizeOptionsInput.value));
  const defaultPageSize = pageSizeOptions.includes(form.table_default_page_size)
    ? form.table_default_page_size
    : pageSizeOptions[0] || 20;

  form.table_page_size_options = pageSizeOptions;
  form.table_default_page_size = defaultPageSize;
  tablePageSizeOptionsInput.value = pageSizeOptions.join(", ");

  const payload: UpdateSettingsRequest = {
    site_name: form.site_name.trim(),
    site_logo: form.site_logo.trim(),
    site_subtitle: form.site_subtitle.trim(),
    api_base_url: form.api_base_url.trim(),
    contact_info: form.contact_info.trim(),
    doc_url: form.doc_url.trim(),
    hide_ccs_import_button: form.hide_ccs_import_button,
    table_default_page_size: form.table_default_page_size,
    table_page_size_options: form.table_page_size_options,
    backend_mode_enabled: form.backend_mode_enabled,
    turnstile_enabled: form.turnstile_enabled,
    turnstile_site_key: form.turnstile_site_key.trim(),
    api_key_acl_trust_forwarded_ip: form.api_key_acl_trust_forwarded_ip,
    enable_model_fallback: form.enable_model_fallback,
    fallback_model_anthropic: form.fallback_model_anthropic.trim(),
    fallback_model_openai: form.fallback_model_openai.trim(),
    fallback_model_gemini: form.fallback_model_gemini.trim(),
    fallback_model_antigravity: form.fallback_model_antigravity.trim(),
    enable_identity_patch: form.enable_identity_patch,
    identity_patch_prompt: form.identity_patch_prompt,
    min_claude_code_version: form.min_claude_code_version.trim(),
    max_claude_code_version: form.max_claude_code_version.trim(),
    allow_ungrouped_key_scheduling: form.allow_ungrouped_key_scheduling,
    enable_fingerprint_unification: form.enable_fingerprint_unification,
    enable_metadata_passthrough: form.enable_metadata_passthrough,
    enable_cch_signing: form.enable_cch_signing,
    enable_claude_oauth_system_prompt_injection: form.enable_claude_oauth_system_prompt_injection,
    claude_oauth_system_prompt: form.claude_oauth_system_prompt.trim() ? form.claude_oauth_system_prompt : undefined,
    claude_oauth_system_prompt_blocks: form.claude_oauth_system_prompt_blocks.trim()
      ? form.claude_oauth_system_prompt_blocks
      : undefined,
    enable_anthropic_cache_ttl_1h_injection: form.enable_anthropic_cache_ttl_1h_injection,
    rewrite_message_cache_control: form.rewrite_message_cache_control,
    antigravity_user_agent_version: form.antigravity_user_agent_version.trim(),
    openai_codex_user_agent: form.openai_codex_user_agent.trim(),
    openai_allow_claude_code_codex_plugin: form.openai_allow_claude_code_codex_plugin,
    openai_advanced_scheduler_enabled: form.openai_advanced_scheduler_enabled,
    risk_control_enabled: form.risk_control_enabled,
    cyber_session_block_enabled: form.cyber_session_block_enabled,
    cyber_session_block_ttl_seconds: normalizeNumber(form.cyber_session_block_ttl_seconds, 3600, 60, 604800),
    channel_monitor_enabled: form.channel_monitor_enabled,
    channel_monitor_default_interval_seconds: normalizeNumber(form.channel_monitor_default_interval_seconds, 60, 10, 86400),
  };

  if (form.turnstile_secret_key.trim()) {
    payload.turnstile_secret_key = form.turnstile_secret_key.trim();
  }

  return payload;
}

async function saveSettings(): Promise<void> {
  saving.value = true;
  try {
    const updated = await adminAPI.settings.updateSettings(buildSettingsPayload());
    applySettings(updated);
    adminSettingsStore.setOpsMonitoringEnabledLocal(updated.ops_monitoring_enabled ?? true);
    adminSettingsStore.setOpsRealtimeMonitoringEnabledLocal(updated.ops_realtime_monitoring_enabled ?? true);
    adminSettingsStore.setOpsQueryModeDefaultLocal(updated.ops_query_mode_default || "auto");
    appStore.showSuccess(localText("设置已保存", "Settings saved"));
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, localText("保存设置失败", "Failed to save settings")));
  } finally {
    saving.value = false;
  }
}

function normalizeNumber(value: unknown, fallback: number, min: number, max: number): number {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return fallback;
  return Math.min(max, Math.max(min, Math.floor(numeric)));
}

function normalizePageSizeOptions(options: unknown): number[] {
  const source = Array.isArray(options) ? options : [10, 20, 50, 100];
  const normalized = source
    .map((value) => normalizeNumber(value, 0, 5, 1000))
    .filter((value) => value > 0);
  return [...new Set(normalized)].sort((a, b) => a - b);
}

function parsePageSizeOptionsInput(value: string): number[] {
  return value
    .split(/[,\s]+/)
    .map((item) => Number(item.trim()))
    .filter((item) => Number.isFinite(item));
}

async function loadAdminApiKeyStatus(): Promise<void> {
  adminApiKeyLoading.value = true;
  try {
    const status = await adminAPI.settings.getAdminApiKey();
    adminApiKeyExists.value = status.exists;
    adminApiKeyMasked.value = status.masked_key || "";
  } catch {
    // 管理员密钥状态读取失败不影响设置页其它功能。
  } finally {
    adminApiKeyLoading.value = false;
  }
}

async function createAdminApiKey(): Promise<void> {
  adminApiKeyOperating.value = true;
  try {
    const result = await adminAPI.settings.regenerateAdminApiKey();
    newAdminApiKey.value = result.key;
    adminApiKeyExists.value = true;
    adminApiKeyMasked.value = `${result.key.substring(0, 10)}...${result.key.slice(-4)}`;
    appStore.showSuccess(localText("管理员密钥已生成", "Admin API key generated"));
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, localText("生成管理员密钥失败", "Failed to create admin API key")));
  } finally {
    adminApiKeyOperating.value = false;
  }
}

async function regenerateAdminApiKey(): Promise<void> {
  if (!window.confirm(localText("确认重新生成管理员 API Key？旧密钥会立即失效。", "Regenerate admin API key? The old key will stop working."))) {
    return;
  }
  await createAdminApiKey();
}

async function deleteAdminApiKey(): Promise<void> {
  if (!window.confirm(localText("确认删除管理员 API Key？", "Delete admin API key?"))) {
    return;
  }
  adminApiKeyOperating.value = true;
  try {
    await adminAPI.settings.deleteAdminApiKey();
    adminApiKeyExists.value = false;
    adminApiKeyMasked.value = "";
    newAdminApiKey.value = "";
    appStore.showSuccess(localText("管理员密钥已删除", "Admin API key deleted"));
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, localText("删除管理员密钥失败", "Failed to delete admin API key")));
  } finally {
    adminApiKeyOperating.value = false;
  }
}

async function copyNewKey(): Promise<void> {
  try {
    await navigator.clipboard.writeText(newAdminApiKey.value);
    appStore.showSuccess(localText("已复制", "Copied"));
  } catch {
    appStore.showError(localText("复制失败", "Copy failed"));
  }
}

async function loadOverloadCooldownSettings(): Promise<void> {
  overloadCooldownLoading.value = true;
  try {
    Object.assign(overloadCooldownForm, await adminAPI.settings.getOverloadCooldownSettings());
  } finally {
    overloadCooldownLoading.value = false;
  }
}

async function saveOverloadCooldownSettings(): Promise<void> {
  overloadCooldownSaving.value = true;
  try {
    Object.assign(
      overloadCooldownForm,
      await adminAPI.settings.updateOverloadCooldownSettings({
        enabled: overloadCooldownForm.enabled,
        cooldown_minutes: normalizeNumber(overloadCooldownForm.cooldown_minutes, 10, 1, 120),
      }),
    );
    appStore.showSuccess(localText("529 过载冷却已保存", "Overload cooldown saved"));
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, localText("保存 529 过载冷却失败", "Failed to save overload cooldown")));
  } finally {
    overloadCooldownSaving.value = false;
  }
}

async function loadRateLimit429CooldownSettings(): Promise<void> {
  rateLimit429CooldownLoading.value = true;
  try {
    Object.assign(rateLimit429CooldownForm, await adminAPI.settings.getRateLimit429CooldownSettings());
  } finally {
    rateLimit429CooldownLoading.value = false;
  }
}

async function saveRateLimit429CooldownSettings(): Promise<void> {
  rateLimit429CooldownSaving.value = true;
  try {
    Object.assign(
      rateLimit429CooldownForm,
      await adminAPI.settings.updateRateLimit429CooldownSettings({
        enabled: rateLimit429CooldownForm.enabled,
        cooldown_seconds: normalizeNumber(rateLimit429CooldownForm.cooldown_seconds, 5, 1, 3600),
      }),
    );
    appStore.showSuccess(localText("429 限流冷却已保存", "429 cooldown saved"));
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, localText("保存 429 限流冷却失败", "Failed to save 429 cooldown")));
  } finally {
    rateLimit429CooldownSaving.value = false;
  }
}

async function loadFallbackQueueSettings(): Promise<void> {
  fallbackQueueLoading.value = true;
  try {
    Object.assign(fallbackQueueForm, await adminAPI.settings.getFallbackQueueSettings());
  } finally {
    fallbackQueueLoading.value = false;
  }
}

async function saveFallbackQueueSettings(): Promise<void> {
  fallbackQueueSaving.value = true;
  try {
    Object.assign(
      fallbackQueueForm,
      await adminAPI.settings.updateFallbackQueueSettings({
        enabled: fallbackQueueForm.enabled,
        max_waiting: normalizeNumber(fallbackQueueForm.max_waiting, 100, 1, 100000),
        wait_timeout_seconds: normalizeNumber(fallbackQueueForm.wait_timeout_seconds, 120, 1, 3600),
      }),
    );
    appStore.showSuccess(localText("账号兜底排队已保存", "Fallback queue saved"));
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, localText("保存账号兜底排队失败", "Failed to save fallback queue")));
  } finally {
    fallbackQueueSaving.value = false;
  }
}

async function loadStreamTimeoutSettings(): Promise<void> {
  streamTimeoutLoading.value = true;
  try {
    Object.assign(streamTimeoutForm, await adminAPI.settings.getStreamTimeoutSettings());
  } finally {
    streamTimeoutLoading.value = false;
  }
}

async function saveStreamTimeoutSettings(): Promise<void> {
  streamTimeoutSaving.value = true;
  try {
    Object.assign(
      streamTimeoutForm,
      await adminAPI.settings.updateStreamTimeoutSettings({
        enabled: streamTimeoutForm.enabled,
        action: streamTimeoutForm.action,
        temp_unsched_minutes: normalizeNumber(streamTimeoutForm.temp_unsched_minutes, 5, 1, 1440),
        threshold_count: normalizeNumber(streamTimeoutForm.threshold_count, 3, 1, 100),
        threshold_window_minutes: normalizeNumber(streamTimeoutForm.threshold_window_minutes, 10, 1, 1440),
      }),
    );
    appStore.showSuccess(localText("流超时策略已保存", "Stream timeout saved"));
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, localText("保存流超时策略失败", "Failed to save stream timeout")));
  } finally {
    streamTimeoutSaving.value = false;
  }
}

async function loadRectifierSettings(): Promise<void> {
  rectifierLoading.value = true;
  try {
    Object.assign(rectifierForm, await adminAPI.settings.getRectifierSettings());
    if (!Array.isArray(rectifierForm.apikey_signature_patterns)) {
      rectifierForm.apikey_signature_patterns = [];
    }
    rectifierPatternsInput.value = rectifierForm.apikey_signature_patterns.join("\n");
  } finally {
    rectifierLoading.value = false;
  }
}

async function saveRectifierSettings(): Promise<void> {
  rectifierSaving.value = true;
  try {
    const patterns = rectifierPatternsInput.value
      .split(/\r?\n/)
      .map((item) => item.trim())
      .filter(Boolean);
    Object.assign(
      rectifierForm,
      await adminAPI.settings.updateRectifierSettings({
        enabled: rectifierForm.enabled,
        thinking_signature_enabled: rectifierForm.thinking_signature_enabled,
        thinking_budget_enabled: rectifierForm.thinking_budget_enabled,
        apikey_signature_enabled: rectifierForm.apikey_signature_enabled,
        apikey_signature_patterns: patterns,
      }),
    );
    rectifierPatternsInput.value = rectifierForm.apikey_signature_patterns.join("\n");
    appStore.showSuccess(localText("修正器设置已保存", "Rectifier settings saved"));
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, localText("保存修正器设置失败", "Failed to save rectifier settings")));
  } finally {
    rectifierSaving.value = false;
  }
}

async function loadBetaPolicySettings(): Promise<void> {
  betaPolicyLoading.value = true;
  try {
    const settings = await adminAPI.settings.getBetaPolicySettings();
    betaPolicyForm.rules = settings.rules || [];
  } finally {
    betaPolicyLoading.value = false;
  }
}

function addBetaPolicyRule(): void {
  betaPolicyForm.rules.push({
    beta_token: "",
    action: "pass",
    scope: "all",
    error_message: "",
    model_whitelist: [],
    fallback_action: "pass",
    fallback_error_message: "",
  });
}

function removeBetaPolicyRule(index: number): void {
  betaPolicyForm.rules.splice(index, 1);
}

function updateBetaWhitelist(rule: BetaPolicyRule, event: Event): void {
  const value = (event.target as HTMLInputElement).value;
  rule.model_whitelist = value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

async function saveBetaPolicySettings(): Promise<void> {
  betaPolicySaving.value = true;
  try {
    const cleanedRules = betaPolicyForm.rules
      .filter((rule) => rule.beta_token.trim())
      .map((rule) => {
        const whitelist = rule.model_whitelist?.filter((item) => item.trim()) || [];
        return {
          beta_token: rule.beta_token.trim(),
          action: rule.action,
          scope: rule.scope,
          error_message: rule.error_message?.trim() || undefined,
          model_whitelist: whitelist.length > 0 ? whitelist : undefined,
          fallback_action: whitelist.length > 0 ? rule.fallback_action || "pass" : undefined,
          fallback_error_message: rule.fallback_error_message?.trim() || undefined,
        };
      });
    const updated = await adminAPI.settings.updateBetaPolicySettings({ rules: cleanedRules });
    betaPolicyForm.rules = updated.rules || [];
    appStore.showSuccess(localText("Beta 策略已保存", "Beta policy saved"));
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, localText("保存 Beta 策略失败", "Failed to save beta policy")));
  } finally {
    betaPolicySaving.value = false;
  }
}

watch(
  () => rectifierPatternsInput.value,
  (value) => {
    rectifierForm.apikey_signature_patterns = value
      .split(/\r?\n/)
      .map((item) => item.trim())
      .filter(Boolean);
  },
);

onMounted(() => {
  void Promise.allSettled([
    loadSettings(),
    loadGatewayLiteConfig(),
    loadAdminApiKeyStatus(),
    loadOverloadCooldownSettings(),
    loadRateLimit429CooldownSettings(),
    loadFallbackQueueSettings(),
    loadStreamTimeoutSettings(),
    loadRectifierSettings(),
    loadBetaPolicySettings(),
  ]);
});

const SwitchCard = defineComponent({
  name: "SwitchCard",
  props: {
    modelValue: { type: Boolean, required: true },
    title: { type: String, required: true },
  },
  emits: ["update:modelValue"],
  setup(props, { emit }) {
    return () =>
      h("div", { class: "switch-card" }, [
        h("span", { class: "switch-card-title" }, props.title),
        h(Toggle, {
          modelValue: props.modelValue,
          "onUpdate:modelValue": (value: boolean) => emit("update:modelValue", value),
        }),
      ]);
  },
});

const PolicyCard = defineComponent({
  name: "PolicyCard",
  props: {
    title: { type: String, required: true },
    description: { type: String, required: true },
    loading: { type: Boolean, required: true },
    saving: { type: Boolean, required: true },
  },
  emits: ["save"],
  setup(props, { slots, emit }) {
    return () =>
      h("section", { class: "card" }, [
        h("div", { class: "border-b border-gray-100 px-6 py-4 dark:border-dark-700" }, [
          h("h2", { class: "text-lg font-semibold text-gray-900 dark:text-white" }, props.title),
          h("p", { class: "mt-1 text-sm text-gray-500 dark:text-gray-400" }, props.description),
        ]),
        h("div", { class: "space-y-5 p-6" }, [
          props.loading
            ? h("div", { class: "inline-loading" }, [
                h("span", { class: "spinner-sm" }),
                localText("加载中", "Loading"),
              ])
            : slots.default?.(),
          h("div", { class: "flex justify-end" }, [
            h(
              "button",
              {
                type: "button",
                class: "btn btn-secondary btn-sm",
                disabled: props.loading || props.saving,
                onClick: () => emit("save"),
              },
              props.saving ? localText("保存中", "Saving") : localText("保存", "Save"),
            ),
          ]),
        ]),
      ]);
  },
});
</script>

<style scoped>
.field {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.field > span {
  font-size: 0.875rem;
  font-weight: 500;
  color: rgb(55 65 81);
}

.dark .field > span {
  color: rgb(209 213 219);
}

.textarea {
  width: 100%;
  border-radius: 0.375rem;
  border: 1px solid rgb(209 213 219);
  background: white;
  padding: 0.625rem 0.75rem;
  font-size: 0.875rem;
  color: rgb(17 24 39);
}

.textarea:focus {
  border-color: rgb(59 130 246);
  outline: none;
  box-shadow: 0 0 0 1px rgb(59 130 246);
}

.dark .textarea {
  border-color: rgb(75 85 99);
  background: rgb(31 41 55);
  color: rgb(243 244 246);
}

.setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.setting-row h3 {
  font-size: 0.875rem;
  font-weight: 500;
  color: rgb(17 24 39);
}

.setting-row p {
  margin-top: 0.25rem;
  font-size: 0.875rem;
  color: rgb(107 114 128);
}

.dark .setting-row h3 {
  color: white;
}

.dark .setting-row p {
  color: rgb(156 163 175);
}

.switch-card {
  display: flex;
  min-height: 3.5rem;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  border-radius: 0.5rem;
  border: 1px solid rgb(229 231 235);
  padding: 0.75rem 1rem;
}

.dark .switch-card {
  border-color: rgb(55 65 81);
}

.switch-card-title {
  font-size: 0.875rem;
  font-weight: 500;
  color: rgb(31 41 55);
}

.dark .switch-card-title {
  color: rgb(229 231 235);
}

.inline-loading {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  color: rgb(107 114 128);
}

.spinner-sm {
  display: inline-block;
  height: 1rem;
  width: 1rem;
  flex-shrink: 0;
  animation: spin 1s linear infinite;
  border-radius: 9999px;
  border-bottom: 2px solid currentColor;
}

.empty-hint {
  border-radius: 0.5rem;
  border: 1px dashed rgb(209 213 219);
  padding: 1rem;
  font-size: 0.875rem;
  color: rgb(107 114 128);
}

.dark .empty-hint {
  border-color: rgb(75 85 99);
  color: rgb(156 163 175);
}

.settings-tabs-shell {
  position: sticky;
  top: 0;
  z-index: 10;
  margin: -0.25rem -0.25rem 0;
  border-radius: 0.75rem;
  border: 1px solid rgba(229, 231, 235, 0.9);
  background: rgba(255, 255, 255, 0.92);
  padding: 0.25rem;
  backdrop-filter: blur(16px);
}

.settings-tabs-scroll {
  overflow-x: auto;
}

.settings-tabs-scroll::-webkit-scrollbar {
  display: none;
}

.settings-tabs {
  display: inline-flex;
  min-width: 100%;
  gap: 0.25rem;
}

.settings-tab {
  display: inline-flex;
  min-height: 2.75rem;
  min-width: 7.5rem;
  flex: 1 0 auto;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  border-radius: 0.5rem;
  padding: 0.625rem 0.875rem;
  font-size: 0.875rem;
  font-weight: 500;
  color: rgb(75 85 99);
  transition: background-color 0.15s ease, color 0.15s ease, box-shadow 0.15s ease;
}

.settings-tab:hover {
  background: rgb(249 250 251);
  color: rgb(17 24 39);
}

.settings-tab-active {
  background: white;
  color: rgb(37 99 235);
  box-shadow: 0 1px 2px rgba(15, 23, 42, 0.08);
}

.settings-tab-icon {
  display: inline-flex;
  flex-shrink: 0;
}

.settings-tab-label {
  white-space: nowrap;
}

.dark .settings-tabs-shell {
  border-color: rgba(55, 65, 81, 0.9);
  background: rgba(17, 24, 39, 0.9);
}

.dark .settings-tab {
  color: rgb(209 213 219);
}

.dark .settings-tab:hover {
  background: rgb(31 41 55);
  color: white;
}

.dark .settings-tab-active {
  background: rgb(31 41 55);
  color: rgb(96 165 250);
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 640px) {
  .setting-row {
    align-items: flex-start;
  }

  .settings-tab {
    min-width: 6.5rem;
  }
}
</style>
