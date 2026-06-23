<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex flex-1 flex-col gap-3 sm:flex-row">
            <div class="relative max-w-md flex-1">
              <Icon name="search" class="pointer-events-none absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 text-gray-400" />
              <input
                v-model="searchQuery"
                type="text"
                class="input pl-10"
                :placeholder="t('admin.modelPrices.searchPlaceholder', '搜索模型或供应商...')"
                @input="handleSearchInput"
              />
            </div>
            <select v-model="providerFilter" class="input max-w-[180px]" @change="reloadFirstPage">
              <option value="">{{ t('admin.modelPrices.allProviders', '全部供应商') }}</option>
              <option value="xiaomi">Xiaomi</option>
              <option value="openai">OpenAI</option>
              <option value="anthropic">Anthropic</option>
              <option value="google">Google</option>
              <option value="custom">Custom</option>
            </select>
            <select v-model="sourceFilter" class="input max-w-[180px]" @change="reloadFirstPage">
              <option value="">{{ t('admin.modelPrices.allSources', '全部来源') }}</option>
              <option value="catalog">{{ t('admin.modelPrices.source.catalog', '默认目录') }}</option>
              <option value="override">{{ t('admin.modelPrices.source.override', '覆盖价') }}</option>
              <option value="static_fallback">{{ t('admin.modelPrices.source.staticFallback', '内置兜底') }}</option>
            </select>
          </div>
          <div class="flex items-center gap-2">
            <button class="btn btn-secondary" type="button" :disabled="loading" @click="reload">
              <Icon name="refresh" size="sm" />
            </button>
            <button class="btn btn-primary" type="button" @click="openCreateDialog">
              <Icon name="plus" size="sm" />
              {{ t('admin.modelPrices.create', '新增价格') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="prices" :loading="loading">
          <template #cell-model="{ row }">
            <div class="min-w-[220px]">
              <div class="font-medium text-gray-900 dark:text-white">{{ row.model }}</div>
              <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ row.mode || 'chat' }}</div>
            </div>
          </template>

          <template #cell-provider="{ row }">
            <span class="inline-flex rounded-md bg-gray-100 px-2 py-1 text-xs font-medium text-gray-700 dark:bg-dark-700 dark:text-dark-200">
              {{ row.provider || 'custom' }}
            </span>
          </template>

          <template #cell-prices="{ row }">
            <div class="grid min-w-[360px] grid-cols-2 gap-x-4 gap-y-1 text-xs">
              <span class="text-gray-500 dark:text-dark-400">Input</span>
              <span class="text-right font-medium text-gray-900 dark:text-white">{{ formatMoney(row.input_cost_per_1m_tokens) }}</span>
              <span class="text-gray-500 dark:text-dark-400">Output</span>
              <span class="text-right font-medium text-gray-900 dark:text-white">{{ formatMoney(row.output_cost_per_1m_tokens) }}</span>
              <span class="text-gray-500 dark:text-dark-400">Cache Read</span>
              <span class="text-right font-medium text-gray-900 dark:text-white">{{ formatMoney(row.cache_read_cost_per_1m_tokens) }}</span>
              <span class="text-gray-500 dark:text-dark-400">Cache Write</span>
              <span class="text-right font-medium text-gray-900 dark:text-white">{{ formatMoney(row.cache_creation_cost_per_1m_tokens) }}</span>
            </div>
          </template>

          <template #cell-image="{ row }">
            <span class="text-sm text-gray-900 dark:text-white">
              {{ row.output_cost_per_image > 0 ? formatMoney(row.output_cost_per_image, false) : '-' }}
            </span>
          </template>

          <template #cell-source="{ row }">
            <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="sourceClass(row.source)">
              {{ sourceLabel(row.source) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
              <button class="btn btn-secondary btn-sm" type="button" @click="openEditDialog(row)">
                {{ t('common.edit', '编辑') }}
              </button>
              <button
                v-if="row.is_override"
                class="btn btn-secondary btn-sm text-red-600 hover:text-red-700 dark:text-red-400"
                type="button"
                @click="confirmReset(row)"
              >
                {{ t('admin.modelPrices.resetOverride', '取消覆盖') }}
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.modelPrices.emptyTitle', '暂无模型价格')"
              :description="t('admin.modelPrices.emptyDescription', '新增覆盖价或检查默认价格目录是否加载成功')"
              :action-text="t('admin.modelPrices.create', '新增价格')"
              @action="openCreateDialog"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="onPageChange"
          @update:pageSize="onPageSizeChange"
        />
      </template>
    </TablePageLayout>

    <BaseDialog
      :show="showDialog"
      :title="editing ? t('admin.modelPrices.editTitle', '编辑模型价格') : t('admin.modelPrices.createTitle', '新增模型价格')"
      width="wide"
      @close="closeDialog"
    >
      <form class="space-y-5" @submit.prevent="savePrice">
        <div class="grid gap-4 md:grid-cols-2">
          <label class="space-y-1">
            <span class="input-label">{{ t('admin.modelPrices.model', '模型') }}</span>
            <input v-model.trim="form.model" class="input" :disabled="Boolean(editing)" placeholder="mimo-v2.5-pro" required />
          </label>
          <label class="space-y-1">
            <span class="input-label">{{ t('admin.modelPrices.provider', '供应商') }}</span>
            <input v-model.trim="form.provider" class="input" placeholder="xiaomi" />
          </label>
          <label class="space-y-1">
            <span class="input-label">{{ t('admin.modelPrices.mode', '模式') }}</span>
            <input v-model.trim="form.mode" class="input" placeholder="chat" />
          </label>
          <label class="flex items-center gap-2 pt-7">
            <input v-model="form.supports_prompt_caching" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" />
            <span class="text-sm text-gray-700 dark:text-dark-200">{{ t('admin.modelPrices.promptCaching', '支持缓存计费') }}</span>
          </label>
        </div>

        <div class="grid gap-4 md:grid-cols-2">
          <PriceInput v-model="form.input_cost_per_1m_tokens" label="Input / 1M tokens" />
          <PriceInput v-model="form.output_cost_per_1m_tokens" label="Output / 1M tokens" />
          <PriceInput v-model="form.cache_read_cost_per_1m_tokens" label="Cache Read / 1M tokens" />
          <PriceInput v-model="form.cache_creation_cost_per_1m_tokens" label="Cache Write / 1M tokens" />
          <PriceInput v-model="form.output_cost_per_image" :label="t('admin.modelPrices.imagePrice', '图片单价')" />
        </div>
      </form>

      <template #footer>
        <button class="btn btn-secondary" type="button" @click="closeDialog">{{ t('common.cancel', '取消') }}</button>
        <button class="btn btn-primary" type="button" :disabled="saving" @click="savePrice">
          {{ saving ? t('common.saving', '保存中...') : t('common.save', '保存') }}
        </button>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showResetDialog"
      :title="t('admin.modelPrices.resetOverride', '取消覆盖')"
      :message="resetMessage"
      :confirm-text="t('admin.modelPrices.resetOverride', '取消覆盖')"
      :cancel-text="t('common.cancel', '取消')"
      :danger="true"
      @confirm="resetOverride"
      @cancel="showResetDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref, type PropType } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { ModelPrice, UpsertModelPriceRequest } from '@/api/admin'
import { extractApiErrorMessage } from '@/utils/apiError'
import { useAppStore } from '@/stores/app'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/GatewayLiteLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

type PriceForm = {
  model: string
  provider: string
  mode: string
  input_cost_per_1m_tokens: number | null
  output_cost_per_1m_tokens: number | null
  cache_creation_cost_per_1m_tokens: number | null
  cache_read_cost_per_1m_tokens: number | null
  output_cost_per_image: number | null
  supports_prompt_caching: boolean
}

const { t } = useI18n()
const appStore = useAppStore()

const PriceInput = defineComponent({
  props: {
    modelValue: { type: Number as PropType<number | null>, default: null },
    label: { type: String, required: true }
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => h('label', { class: 'space-y-1' }, [
      h('span', { class: 'input-label' }, props.label),
      h('input', {
        class: 'input',
        type: 'number',
        min: '0',
        step: '0.000001',
        value: props.modelValue ?? '',
        placeholder: '0',
        onInput: (event: Event) => {
          const value = (event.target as HTMLInputElement).value
          emit('update:modelValue', value === '' ? null : Number(value))
        }
      })
    ])
  }
})

const prices = ref<ModelPrice[]>([])
const loading = ref(false)
const saving = ref(false)
const searchQuery = ref('')
const providerFilter = ref('')
const sourceFilter = ref('')
const pagination = reactive({ page: 1, page_size: getPersistedPageSize(), total: 0 })
const showDialog = ref(false)
const editing = ref<ModelPrice | null>(null)
const showResetDialog = ref(false)
const resetting = ref<ModelPrice | null>(null)
let searchTimer: ReturnType<typeof setTimeout> | null = null
let abortController: AbortController | null = null

const form = reactive<PriceForm>(emptyForm())

const columns = computed<Column[]>(() => [
  { key: 'model', label: t('admin.modelPrices.columns.model', '模型'), sortable: false },
  { key: 'provider', label: t('admin.modelPrices.columns.provider', '供应商'), sortable: false },
  { key: 'prices', label: t('admin.modelPrices.columns.prices', 'Token 价格（USD / 1M）'), sortable: false },
  { key: 'image', label: t('admin.modelPrices.columns.image', '图片价格'), sortable: false },
  { key: 'source', label: t('admin.modelPrices.columns.source', '来源'), sortable: false },
  { key: 'actions', label: t('admin.modelPrices.columns.actions', '操作'), sortable: false },
])

const resetMessage = computed(() => {
  const model = resetting.value?.model || ''
  return t('admin.modelPrices.resetConfirm', { model }, `确定取消 ${model} 的覆盖价吗？取消后会回退到默认价格。`)
})

function emptyForm(): PriceForm {
  return {
    model: '',
    provider: '',
    mode: 'chat',
    input_cost_per_1m_tokens: null,
    output_cost_per_1m_tokens: null,
    cache_creation_cost_per_1m_tokens: null,
    cache_read_cost_per_1m_tokens: null,
    output_cost_per_image: null,
    supports_prompt_caching: false
  }
}

function fillForm(price?: ModelPrice | null) {
  const next = price
    ? {
        model: price.model,
        provider: price.provider || 'custom',
        mode: price.mode || 'chat',
        input_cost_per_1m_tokens: price.input_cost_per_1m_tokens || null,
        output_cost_per_1m_tokens: price.output_cost_per_1m_tokens || null,
        cache_creation_cost_per_1m_tokens: price.cache_creation_cost_per_1m_tokens || null,
        cache_read_cost_per_1m_tokens: price.cache_read_cost_per_1m_tokens || null,
        output_cost_per_image: price.output_cost_per_image || null,
        supports_prompt_caching: price.supports_prompt_caching
      }
    : emptyForm()
  Object.assign(form, next)
}

async function loadPrices() {
  abortController?.abort()
  abortController = new AbortController()
  loading.value = true
  try {
    const res = await adminAPI.modelPrices.list({
      page: pagination.page,
      page_size: pagination.page_size,
      search: searchQuery.value || undefined,
      provider: providerFilter.value || undefined,
      source: sourceFilter.value || undefined
    }, { signal: abortController.signal })
    prices.value = res.items || []
    pagination.total = res.total || 0
  } catch (error: any) {
    if (error?.code !== 'ERR_CANCELED') {
      appStore.showError(extractApiErrorMessage(error, t('admin.modelPrices.loadError', '加载模型价格失败')))
    }
  } finally {
    loading.value = false
  }
}

function reload() {
  void loadPrices()
}

function reloadFirstPage() {
  pagination.page = 1
  reload()
}

function handleSearchInput() {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(reloadFirstPage, 300)
}

function onPageChange(page: number) {
  pagination.page = page
  reload()
}

function onPageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  reload()
}

function openCreateDialog() {
  editing.value = null
  fillForm(null)
  showDialog.value = true
}

function openEditDialog(price: ModelPrice) {
  editing.value = price
  fillForm(price)
  showDialog.value = true
}

function closeDialog() {
  showDialog.value = false
  editing.value = null
}

async function savePrice() {
  const payload: UpsertModelPriceRequest = { ...form }
  if (!payload.model) {
    appStore.showError(t('admin.modelPrices.modelRequired', '请填写模型名称'))
    return
  }
  if (
    payload.input_cost_per_1m_tokens === null &&
    payload.output_cost_per_1m_tokens === null &&
    payload.cache_creation_cost_per_1m_tokens === null &&
    payload.cache_read_cost_per_1m_tokens === null &&
    payload.output_cost_per_image === null
  ) {
    appStore.showError(t('admin.modelPrices.priceRequired', '请至少填写一个价格字段'))
    return
  }
  saving.value = true
  try {
    await adminAPI.modelPrices.upsert(payload)
    appStore.showSuccess(t('admin.modelPrices.saveSuccess', '模型价格已保存'))
    closeDialog()
    reload()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.modelPrices.saveError', '保存模型价格失败')))
  } finally {
    saving.value = false
  }
}

function confirmReset(price: ModelPrice) {
  resetting.value = price
  showResetDialog.value = true
}

async function resetOverride() {
  if (!resetting.value) return
  try {
    await adminAPI.modelPrices.removeOverride(resetting.value.model)
    appStore.showSuccess(t('admin.modelPrices.resetSuccess', '覆盖价已取消'))
    showResetDialog.value = false
    resetting.value = null
    reload()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.modelPrices.resetError', '取消覆盖失败')))
  }
}

function formatMoney(value: number | null | undefined, perMillion = true) {
  if (!value || value <= 0) return '-'
  const fixed = value >= 1 ? value.toFixed(4) : value.toFixed(6)
  return `$${trimZeros(fixed)}${perMillion ? '' : ''}`
}

function trimZeros(value: string) {
  return value.replace(/\.0+$/, '').replace(/(\.\d*?)0+$/, '$1')
}

function sourceLabel(source: string) {
  if (source === 'override') return t('admin.modelPrices.source.override', '覆盖价')
  if (source === 'static_fallback') return t('admin.modelPrices.source.staticFallback', '内置兜底')
  return t('admin.modelPrices.source.catalog', '默认目录')
}

function sourceClass(source: string) {
  if (source === 'override') return 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
  if (source === 'static_fallback') return 'bg-cyan-100 text-cyan-700 dark:bg-cyan-500/15 dark:text-cyan-300'
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
}

onMounted(loadPrices)
</script>
