import { api } from '@/lib/api'
import type {
  GetRequestRulesResponse,
  GetRequestLogsResponse,
  RequestRule,
  RequestLog,
  RequestRuleFormValues,
} from './types'

// ============================================================================
// 请求规则 CRUD
// ============================================================================

/**
 * 获取请求规则分页列表
 */
export async function getRequestRules(
  params: { p?: number; page_size?: number } = {}
): Promise<GetRequestRulesResponse> {
  const res = await api.get('/api/request_rule/', { params })
  return res.data
}

/**
 * 搜索请求规则
 */
export async function searchRequestRules(
  params: { keyword?: string; p?: number; page_size?: number } = {}
): Promise<GetRequestRulesResponse> {
  const res = await api.get('/api/request_rule/search', { params })
  return res.data
}

/**
 * 获取单个请求规则
 */
export async function getRequestRule(
  id: number
): Promise<{ success: boolean; message?: string; data?: RequestRule }> {
  const res = await api.get(`/api/request_rule/${id}`)
  return res.data
}

/**
 * 创建请求规则
 */
export async function createRequestRule(
  data: RequestRuleFormValues
): Promise<{ success: boolean; message?: string }> {
  const res = await api.post('/api/request_rule/', data)
  return res.data
}

/**
 * 更新请求规则
 */
export async function updateRequestRule(
  data: Partial<RequestRule> & { id: number }
): Promise<{ success: boolean; message?: string }> {
  const res = await api.put('/api/request_rule/', data)
  return res.data
}

/**
 * 删除请求规则
 */
export async function deleteRequestRule(
  id: number
): Promise<{ success: boolean; message?: string }> {
  const res = await api.delete(`/api/request_rule/${id}`)
  return res.data
}

/**
 * 更新请求规则启用/禁用状态
 */
export async function updateRequestRuleStatus(
  id: number,
  status: number
): Promise<{ success: boolean; message?: string }> {
  const res = await api.put(`/api/request_rule/${id}/status`, { status })
  return res.data
}

// ============================================================================
// 请求记录
// ============================================================================

/**
 * 获取请求记录分页列表
 */
export async function getRequestLogs(
  params: {
    p?: number
    page_size?: number
    request_rule_id?: number
  } = {}
): Promise<GetRequestLogsResponse> {
  const res = await api.get('/api/request_log/', { params })
  return res.data
}

/**
 * 获取请求记录详情
 */
export async function getRequestLogDetail(
  id: number
): Promise<{ success: boolean; message?: string; data?: RequestLog }> {
  const res = await api.get(`/api/request_log/${id}`)
  return res.data
}

/**
 * 清理请求记录
 */
export async function deleteRequestLogs(
  params: { request_rule_id?: number } = {}
): Promise<{ success: boolean; message?: string }> {
  const res = await api.delete('/api/request_log/', { params })
  return res.data
}

// ============================================================================
// Query Keys
// ============================================================================

export const requestRulesQueryKeys = {
  all: ['request-rules'] as const,
  list: (params: Record<string, unknown> = {}) =>
    ['request-rules', 'list', params] as const,
  detail: (id: number) => ['request-rules', 'detail', id] as const,
}

export const requestLogsQueryKeys = {
  all: ['request-logs'] as const,
  list: (params: Record<string, unknown> = {}) =>
    ['request-logs', 'list', params] as const,
  detail: (id: number) => ['request-logs', 'detail', id] as const,
}
