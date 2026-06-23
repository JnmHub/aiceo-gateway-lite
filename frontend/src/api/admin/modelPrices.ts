import { apiClient } from '../client'

export interface ModelPrice {
  model: string
  provider: string
  mode: string
  input_cost_per_1m_tokens: number
  output_cost_per_1m_tokens: number
  cache_creation_cost_per_1m_tokens: number
  cache_read_cost_per_1m_tokens: number
  output_cost_per_image: number
  currency: string
  supports_prompt_caching: boolean
  source: 'catalog' | 'override' | 'static_fallback' | string
  is_override: boolean
}

export interface ModelPriceListParams {
  page?: number
  page_size?: number
  search?: string
  provider?: string
  source?: string
}

export interface ModelPriceListResponse {
  items: ModelPrice[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface UpsertModelPriceRequest {
  model: string
  provider?: string
  mode?: string
  input_cost_per_1m_tokens?: number | null
  output_cost_per_1m_tokens?: number | null
  cache_creation_cost_per_1m_tokens?: number | null
  cache_read_cost_per_1m_tokens?: number | null
  output_cost_per_image?: number | null
  supports_prompt_caching?: boolean
}

export interface DeleteModelPriceResponse {
  deleted: boolean
  current: ModelPrice
}

export async function list(params: ModelPriceListParams = {}, options?: { signal?: AbortSignal }): Promise<ModelPriceListResponse> {
  const { data } = await apiClient.get<ModelPriceListResponse>('/admin/model-prices', {
    params,
    signal: options?.signal
  })
  return data
}

export async function upsert(payload: UpsertModelPriceRequest): Promise<ModelPrice> {
  const { data } = await apiClient.put<ModelPrice>('/admin/model-prices', payload)
  return data
}

export async function removeOverride(model: string): Promise<DeleteModelPriceResponse> {
  const { data } = await apiClient.delete<DeleteModelPriceResponse>('/admin/model-prices', {
    params: { model }
  })
  return data
}

export default { list, upsert, removeOverride }
