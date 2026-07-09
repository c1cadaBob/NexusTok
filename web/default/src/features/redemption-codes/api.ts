/*
Copyright (C) 2023-2026 c1cada

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@c1cada.dev
*/
import { api } from '@/lib/api'
import type {
  Redemption,
  ApiResponse,
  GetRedemptionsParams,
  GetRedemptionsResponse,
  SearchRedemptionsParams,
  RedemptionFormData,
  RedemptionKeyResponse,
} from './types'

// ============================================================================
// 兑换码管理
// ============================================================================

// 获取分页兑换码列表。
export async function getRedemptions(
  params: GetRedemptionsParams = {}
): Promise<GetRedemptionsResponse> {
  const { p = 1, page_size = 10 } = params
  const res = await api.get(`/api/redemption/?p=${p}&page_size=${page_size}`)
  return res.data
}

// 按关键词和状态搜索兑换码；status 为空时保持旧搜索语义。
export async function searchRedemptions(
  params: SearchRedemptionsParams
): Promise<GetRedemptionsResponse> {
  const { keyword = '', status = '', p = 1, page_size = 10 } = params
  const queryParams = new URLSearchParams()
  queryParams.set('keyword', keyword)
  if (status) queryParams.set('status', status)
  queryParams.set('p', String(p))
  queryParams.set('page_size', String(page_size))
  const res = await api.get(`/api/redemption/search?${queryParams.toString()}`)
  return res.data
}

// 按 ID 获取单个兑换码。
export async function getRedemption(
  id: number
): Promise<ApiResponse<Redemption>> {
  const res = await api.get(`/api/redemption/${id}`)
  return res.data
}

// 获取单个兑换码完整值；该接口需要 redemption.secret_view 和安全验证。
export async function getRedemptionKey(
  id: number
): Promise<ApiResponse<RedemptionKeyResponse>> {
  const res = await api.post(`/api/redemption/${id}/key`)
  return res.data
}

// 创建一个或多个兑换码。
export async function createRedemption(
  data: RedemptionFormData
): Promise<ApiResponse<string[]>> {
  const res = await api.post('/api/redemption/', data)
  return res.data
}

// 更新兑换码信息。
export async function updateRedemption(
  data: RedemptionFormData & { id: number }
): Promise<ApiResponse<Redemption>> {
  const res = await api.put('/api/redemption/', data)
  return res.data
}

// 更新兑换码状态（启用/禁用）。
export async function updateRedemptionStatus(
  id: number,
  status: number
): Promise<ApiResponse<Redemption>> {
  const res = await api.put('/api/redemption/?status_only=true', { id, status })
  return res.data
}

// 删除单个兑换码。
export async function deleteRedemption(id: number): Promise<ApiResponse> {
  const res = await api.delete(`/api/redemption/${id}/`)
  return res.data
}

// 删除已使用、已禁用和已过期的失效兑换码。
export async function deleteInvalidRedemptions(): Promise<ApiResponse<number>> {
  const res = await api.delete('/api/redemption/invalid')
  return res.data
}
