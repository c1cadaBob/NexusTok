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
import type { AxiosRequestConfig } from 'axios'
import { api } from '@/lib/api'
import { getGroups as getUserGroups } from '@/features/users/api'
import type {
  AddChannelRequest,
  BatchDeleteParams,
  BatchSetTagParams,
  ChannelAccountBatchPayload,
  ChannelAccountBatchResponse,
  ChannelAccountListResponse,
  ChannelAccountMutationResponse,
  ChannelAccountPayload,
  Channel,
  ChannelBalanceResponse,
  ChannelTestResponse,
  CopyChannelParams,
  CopyChannelResponse,
  FetchModelsResponse,
  GetChannelResponse,
  GetChannelsParams,
  GetChannelsResponse,
  MultiKeyManageParams,
  MultiKeyStatusResponse,
  SearchChannelsParams,
  SearchChannelsResponse,
  TagOperationParams,
  UpstreamAccountCreateRequest,
  UpstreamAccountCreateResponse,
  UpstreamAccountCaptureStartRequest,
  UpstreamAccountCaptureStartResponse,
  UpstreamAccountCaptureStatusResponse,
  UpstreamAccountPreview2FARequest,
  UpstreamAccountPreview2FAResponse,
  UpstreamAccountPreviewRequest,
  UpstreamAccountPreviewResponse,
  UpstreamAccountRefreshRequest,
  UpstreamAccountRefreshResponse,
} from './types'

// 扩展 API 请求配置，主要用于跳过全局业务错误处理或禁用重复请求合并。
interface ExtendedApiConfig extends AxiosRequestConfig {
  skipBusinessError?: boolean
  disableDuplicate?: boolean
}

export type CodexOAuthStartResponse = {
  success: boolean
  message?: string
  data?: {
    authorize_url?: string
  }
}

export type CodexOAuthCompleteResponse = {
  success: boolean
  message?: string
  data?: {
    key?: string
    account_id?: string
    email?: string
    expires_at?: string
    last_refresh?: string
  }
}

export type CodexUsageResponse = {
  success: boolean
  message?: string
  upstream_status?: number
  data?: Record<string, unknown>
}

export type CodexResetCreditsResponse = CodexUsageResponse

export type CodexUsageResetResponse = CodexUsageResponse

export type CodexCredentialRefreshResponse = {
  success: boolean
  message?: string
  data?: {
    expires_at?: string
    last_refresh?: string
    account_id?: string
    email?: string
    channel_id?: number
    channel_type?: number
    channel_name?: string
  }
}

// ============================================================================
// Base Channel CRUD Operations
// ============================================================================

/**
 * 获取分页渠道列表。
 */
export async function getChannels(
  params: GetChannelsParams = {}
): Promise<GetChannelsResponse> {
  const res = await api.get('/api/channel/', { params })
  return res.data
}

/**
 * Search channels with filters
 */
export async function searchChannels(
  params: SearchChannelsParams
): Promise<SearchChannelsResponse> {
  const res = await api.get('/api/channel/search', { params })
  return res.data
}

/**
 * Get single channel by ID
 */
export async function getChannel(id: number): Promise<GetChannelResponse> {
  const res = await api.get(`/api/channel/${id}`)
  return res.data
}

/**
 * Create new channel(s)
 * Supports single, batch, and multi-key modes
 */
export async function createChannel(
  data: AddChannelRequest
): Promise<{ success: boolean; message?: string }> {
  const res = await api.post('/api/channel', data)
  return res.data
}

/**
 * 使用上游平台账号密码或已保存凭据读取密钥、分组、倍率和余额预览。
 */
export async function previewUpstreamAccount(
  data: UpstreamAccountPreviewRequest
): Promise<UpstreamAccountPreviewResponse> {
  const res = await api.post('/api/channel/upstream-account/preview', data)
  return res.data
}

/**
 * 提交上游平台 2FA 验证码，验证成功后换取普通预览快照。
 */
export async function completeUpstreamAccountPreview2FA(
  data: UpstreamAccountPreview2FARequest
): Promise<UpstreamAccountPreview2FAResponse> {
  const res = await api.post('/api/channel/upstream-account/preview/2fa', data)
  return res.data
}

/**
 * 根据预览快照创建一个渠道和多条渠道账号。
 */
export async function createUpstreamAccountChannel(
  data: UpstreamAccountCreateRequest
): Promise<UpstreamAccountCreateResponse> {
  const res = await api.post('/api/channel/upstream-account/create', data)
  return res.data
}

/**
 * 创建油猴脚本登录态采集会话。
 */
export async function startUpstreamAccountCaptureSession(
  data: UpstreamAccountCaptureStartRequest
): Promise<UpstreamAccountCaptureStartResponse> {
  const res = await api.post(
    '/api/channel/upstream-account/capture-session/start',
    data
  )
  return res.data
}

/**
 * 查询油猴脚本登录态采集会话状态。
 */
export async function getUpstreamAccountCaptureSession(
  captureId: string
): Promise<UpstreamAccountCaptureStatusResponse> {
  const res = await api.get(
    `/api/channel/upstream-account/capture-session/${captureId}`
  )
  return res.data
}

/**
 * 读取当前采集会话对应的完整油猴脚本源码。
 */
export async function getUpstreamAccountCaptureUserscript(
  captureIdOrURL: string
): Promise<string> {
  const scriptURL = captureIdOrURL.includes('/capture-session/')
    ? captureIdOrURL
    : `/api/channel/upstream-account/capture-session/${encodeURIComponent(captureIdOrURL)}/userscript.user.js`
  const res = await api.get(
    scriptURL,
    {
      responseType: 'text',
      disableDuplicate: true,
    } as ExtendedApiConfig
  )
  const source = String(res.data || '')
  if (!source.trimStart().startsWith('// ==UserScript==')) {
    let message = 'Failed to load userscript source'
    try {
      const body = JSON.parse(source) as {
        message?: string
        error?: { message?: string }
      }
      message = body.message || body.error?.message || message
    } catch {
      // 非 JSON 响应通常是代理错误页或过期链接的 HTML，前端统一给出稳定提示。
    }
    throw new Error(message)
  }
  return source
}

/**
 * 使用重新输入或已保存的上游账号凭据刷新已有账号同步渠道。
 */
export async function refreshUpstreamAccountChannel(
  id: number,
  data: UpstreamAccountRefreshRequest
): Promise<UpstreamAccountRefreshResponse> {
  const res = await api.post(
    `/api/channel/${id}/upstream-account/refresh`,
    data
  )
  return res.data
}

/**
 * Update existing channel
 */
export async function updateChannel(
  id: number,
  data: Partial<Channel>
): Promise<{ success: boolean; message?: string; data?: Channel }> {
  const res = await api.put('/api/channel/', { id, ...data })
  return res.data
}

/**
 * 通过专用操作接口更新渠道启用状态，避免通用编辑接口误改运行状态。
 */
export async function updateChannelStatus(
  id: number,
  status: number
): Promise<{ success: boolean; message?: string; data?: boolean }> {
  const res = await api.post(`/api/channel/${id}/status`, { status })
  return res.data
}

/**
 * 批量更新渠道启用状态，后端返回实际发生变化的渠道数量。
 */
export async function batchUpdateChannelStatus(
  ids: number[],
  status: number
): Promise<{ success: boolean; message?: string; data?: number }> {
  const res = await api.post('/api/channel/status/batch', { ids, status })
  return res.data
}

/**
 * Delete single channel
 */
export async function deleteChannel(
  id: number
): Promise<{ success: boolean; message?: string }> {
  const res = await api.delete(`/api/channel/${id}`)
  return res.data
}

/**
 * Batch delete channels
 */
export async function batchDeleteChannels(
  data: BatchDeleteParams
): Promise<{ success: boolean; message?: string; data?: number }> {
  const res = await api.post('/api/channel/batch', data)
  return res.data
}

/**
 * Batch set tag for channels
 */
export async function batchSetChannelTag(
  data: BatchSetTagParams
): Promise<{ success: boolean; message?: string; data?: number }> {
  const res = await api.post('/api/channel/batch/tag', data)
  return res.data
}

// ============================================================================
// Channel Operations
// ============================================================================

/**
 * Test channel connectivity
 */
export async function testChannel(
  id: number,
  params?: { model?: string; endpoint_type?: string; stream?: boolean }
): Promise<ChannelTestResponse> {
  const res = await api.get(`/api/channel/test/${id}`, { params })
  return res.data
}

/**
 * Update channel balance
 */
export async function updateChannelBalance(
  id: number
): Promise<ChannelBalanceResponse> {
  const res = await api.get(`/api/channel/update_balance/${id}`)
  return res.data
}

/**
 * Fetch available models from upstream provider
 */
export async function fetchUpstreamModels(
  id: number
): Promise<FetchModelsResponse> {
  const res = await api.get(`/api/channel/fetch_models/${id}`)
  return res.data
}

/**
 * Copy/clone a channel
 */
export async function copyChannel(
  id: number,
  params: CopyChannelParams = {}
): Promise<CopyChannelResponse> {
  const res = await api.post(`/api/channel/copy/${id}`, null, { params })
  return res.data
}

/**
 * Fix channel abilities
 */
export async function fixChannelAbilities(): Promise<{
  success: boolean
  message?: string
  data?: { success: number; fails: number }
}> {
  const res = await api.post('/api/channel/fix')
  return res.data
}

/**
 * Delete all disabled channels
 */
export async function deleteDisabledChannels(): Promise<{
  success: boolean
  message?: string
  data?: number
}> {
  const res = await api.delete('/api/channel/disabled')
  return res.data
}

/**
 * Get channel key (requires 2FA verification)
 */
export async function getChannelKey(
  id: number,
  code?: string
): Promise<{ success: boolean; message?: string; data?: { key: string } }> {
  const payload = code ? { code } : undefined
  const res = await api.post(`/api/channel/${id}/key`, payload)
  return res.data
}

// ============================================================================
// Channel Account Pool
// ============================================================================

export async function getChannelAccounts(
  channelId: number,
  params: {
    p?: number
    page_size?: number
    status?: number
    search?: string
  } = {}
): Promise<ChannelAccountListResponse> {
  const res = await api.get(`/api/channel/${channelId}/accounts`, { params })
  return res.data
}

export async function createChannelAccount(
  channelId: number,
  data: ChannelAccountPayload
): Promise<ChannelAccountMutationResponse> {
  const res = await api.post(`/api/channel/${channelId}/accounts`, data)
  return res.data
}

export async function updateChannelAccount(
  channelId: number,
  accountId: number,
  data: ChannelAccountPayload
): Promise<ChannelAccountMutationResponse> {
  const res = await api.put(
    `/api/channel/${channelId}/accounts/${accountId}`,
    data
  )
  return res.data
}

export async function deleteChannelAccount(
  channelId: number,
  accountId: number
): Promise<{ success: boolean; message?: string }> {
  const res = await api.delete(
    `/api/channel/${channelId}/accounts/${accountId}`
  )
  return res.data
}

export async function updateChannelAccountStatus(
  channelId: number,
  accountId: number,
  data: { status?: number; reason?: string; clear_cooldown?: boolean }
): Promise<{ success: boolean; message?: string }> {
  const res = await api.post(
    `/api/channel/${channelId}/accounts/${accountId}/status`,
    data
  )
  return res.data
}

export async function batchCreateChannelAccounts(
  channelId: number,
  data: ChannelAccountBatchPayload
): Promise<ChannelAccountBatchResponse> {
  const res = await api.post(`/api/channel/${channelId}/accounts/batch`, data)
  return res.data
}

export async function importMultiKeyToChannelAccounts(
  channelId: number,
  data: Partial<ChannelAccountBatchPayload> = {}
): Promise<ChannelAccountBatchResponse> {
  const res = await api.post(
    `/api/channel/${channelId}/accounts/import-multikey`,
    data
  )
  return res.data
}

// ============================================================================
// Codex 渠道操作
// ============================================================================

export async function startCodexOAuth(): Promise<CodexOAuthStartResponse> {
  const config: ExtendedApiConfig = { skipBusinessError: true }
  const res = await api.post('/api/channel/codex/oauth/start', {}, config)
  return res.data
}

export async function completeCodexOAuth(
  input: string
): Promise<CodexOAuthCompleteResponse> {
  const config: ExtendedApiConfig = { skipBusinessError: true }
  const res = await api.post(
    '/api/channel/codex/oauth/complete',
    { input },
    config
  )
  return res.data
}

export async function refreshCodexCredential(
  channelId: number
): Promise<CodexCredentialRefreshResponse> {
  const config: ExtendedApiConfig = { skipBusinessError: true }
  const res = await api.post(
    `/api/channel/${channelId}/codex/refresh`,
    {},
    config
  )
  return res.data
}

export async function getCodexUsage(
  channelId: number
): Promise<CodexUsageResponse> {
  const config: ExtendedApiConfig = {
    skipBusinessError: true,
    disableDuplicate: true,
  }
  const res = await api.get(`/api/channel/${channelId}/codex/usage`, config)
  return res.data
}

export async function getCodexResetCredits(
  channelId: number
): Promise<CodexResetCreditsResponse> {
  const config: ExtendedApiConfig = {
    skipBusinessError: true,
    disableDuplicate: true,
  }
  const res = await api.get(
    `/api/channel/${channelId}/codex/usage/reset-credits`,
    config
  )
  return res.data
}

export async function resetCodexUsage(
  channelId: number
): Promise<CodexUsageResetResponse> {
  const config: ExtendedApiConfig = {
    skipBusinessError: true,
    disableDuplicate: true,
  }
  const res = await api.post(
    `/api/channel/${channelId}/codex/usage/reset`,
    {},
    config
  )
  return res.data
}

// ============================================================================
// Multi-Key Management
// ============================================================================

/**
 * Manage multi-key channel operations
 */
export async function manageMultiKeys(
  params: MultiKeyManageParams
): Promise<MultiKeyStatusResponse | { success: boolean; message?: string }> {
  const res = await api.post('/api/channel/multi_key/manage', params)
  return res.data
}

/**
 * Get key status for multi-key channel
 */
export async function getMultiKeyStatus(
  channelId: number,
  page = 1,
  pageSize = 50,
  status?: number
): Promise<MultiKeyStatusResponse> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'get_key_status',
    page,
    page_size: pageSize,
    status,
  }) as Promise<MultiKeyStatusResponse>
}

/**
 * Enable a specific key in multi-key channel
 */
export async function enableMultiKey(
  channelId: number,
  keyIndex: number
): Promise<{ success: boolean; message?: string }> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'enable_key',
    key_index: keyIndex,
  }) as Promise<{ success: boolean; message?: string }>
}

/**
 * Disable a specific key in multi-key channel
 */
export async function disableMultiKey(
  channelId: number,
  keyIndex: number
): Promise<{ success: boolean; message?: string }> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'disable_key',
    key_index: keyIndex,
  }) as Promise<{ success: boolean; message?: string }>
}

/**
 * Delete a specific key in multi-key channel
 */
export async function deleteMultiKey(
  channelId: number,
  keyIndex: number
): Promise<{ success: boolean; message?: string }> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'delete_key',
    key_index: keyIndex,
  }) as Promise<{ success: boolean; message?: string }>
}

/**
 * Enable all keys in multi-key channel
 */
export async function enableAllMultiKeys(
  channelId: number
): Promise<{ success: boolean; message?: string }> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'enable_all_keys',
  }) as Promise<{ success: boolean; message?: string }>
}

/**
 * Disable all keys in multi-key channel
 */
export async function disableAllMultiKeys(
  channelId: number
): Promise<{ success: boolean; message?: string }> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'disable_all_keys',
  }) as Promise<{ success: boolean; message?: string }>
}

/**
 * Delete all disabled keys in multi-key channel
 */
export async function deleteDisabledMultiKeys(
  channelId: number
): Promise<{ success: boolean; message?: string; data?: number }> {
  return manageMultiKeys({
    channel_id: channelId,
    action: 'delete_disabled_keys',
  }) as Promise<{ success: boolean; message?: string; data?: number }>
}

// ============================================================================
// Tag Operations
// ============================================================================

/**
 * Enable all channels with a specific tag
 */
export async function enableTagChannels(
  tag: string
): Promise<{ success: boolean; message?: string }> {
  const res = await api.post('/api/channel/tag/enabled', { tag })
  return res.data
}

/**
 * Disable all channels with a specific tag
 */
export async function disableTagChannels(
  tag: string
): Promise<{ success: boolean; message?: string }> {
  const res = await api.post('/api/channel/tag/disabled', { tag })
  return res.data
}

/**
 * Edit all channels with a specific tag
 */
export async function editTagChannels(
  params: TagOperationParams
): Promise<{ success: boolean; message?: string }> {
  const res = await api.put('/api/channel/tag', params)
  return res.data
}

/**
 * Get models for a specific tag
 */
export async function getTagModels(
  tag: string
): Promise<{ success: boolean; message?: string; data?: string }> {
  const res = await api.get('/api/channel/tag/models', { params: { tag } })
  return res.data
}

// ============================================================================
// Utility Functions
// ============================================================================

/**
 * Fetch models from a custom endpoint (for testing before creating channel)
 */
export async function fetchModels(data: {
  base_url: string
  type: number
  key: string
}): Promise<FetchModelsResponse> {
  const res = await api.post('/api/channel/fetch_models', data)
  return res.data
}

/**
 * Delete an Ollama model from a channel
 */
export async function deleteOllamaModel(params: {
  channel_id: number
  model_name: string
}): Promise<{ success: boolean; message?: string }> {
  const res = await api.delete('/api/channel/ollama/delete', { data: params })
  return res.data
}

/**
 * Test all enabled channels
 */
export async function testAllChannels(): Promise<{
  success: boolean
  message?: string
  data?: {
    task_id?: string
    status?: string
    type?: string
  }
}> {
  try {
    const res = await api.get('/api/channel/test', {
      skipErrorHandler: true,
    } as ExtendedApiConfig)
    return res.data
  } catch (error) {
    const response = (
      error as {
        response?: {
          data?: {
            success?: boolean
            message?: string
            data?: {
              task_id?: string
              status?: string
              type?: string
            }
          }
        }
      }
    ).response
    if (response?.data && typeof response.data.success === 'boolean') {
      return {
        success: response.data.success,
        message: response.data.message,
        data: response.data.data,
      }
    }
    throw error
  }
}

/**
 * Update balance for all enabled channels
 */
export async function updateAllChannelsBalance(): Promise<{
  success: boolean
  message?: string
}> {
  const res = await api.get('/api/channel/update_balance')
  return res.data
}

/**
 * Get all available models
 */
export async function getAllModels(): Promise<{
  success: boolean
  message?: string
  data?: Array<{ id: string; [key: string]: unknown }>
}> {
  const res = await api.get('/api/channel/models')
  return res.data
}

/**
 * Get all enabled models
 */
export async function getEnabledModels(): Promise<{
  success: boolean
  message?: string
  data?: string[]
}> {
  const res = await api.get('/api/channel/models_enabled')
  return res.data
}

// ============================================================================
// Ollama Utilities
// ============================================================================

/**
 * Check Ollama version for a given channel
 */
export async function getOllamaVersion(
  channelId: number
): Promise<{ success: boolean; message?: string; data?: { version: string } }> {
  const res = await api.get(`/api/channel/ollama/version/${channelId}`)
  return res.data
}

// ============================================================================
// Group Management
// ============================================================================

/**
 * Get all available groups (re-exported from users API for convenience)
 */
export const getGroups = getUserGroups

// ============================================================================
// Prefill Groups (Model Groups)
// ============================================================================

/**
 * Get prefill groups for quick model selection
 */
export async function getPrefillGroups(
  type: 'model' | 'group' = 'model'
): Promise<{
  success: boolean
  message?: string
  data?: Array<{ id: number; name: string; items: string | string[] }>
}> {
  const res = await api.get('/api/prefill_group', { params: { type } })
  return res.data
}
