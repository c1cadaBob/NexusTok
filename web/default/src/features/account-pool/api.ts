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
  AccountPoolAuthFile,
  AccountPoolAuthFileBatchImportResult,
  AccountPoolAuthFileMutationResult,
  AccountPoolAuthFilePayload,
  AccountPoolAuthFileUpdatePayload,
  AccountPoolBatchCheckResult,
  AccountPoolBatchStatusResult,
  AccountPoolCheckResult,
  AccountPoolGroup,
  AccountPoolGroupOption,
  AccountPoolGroupPayload,
  AccountPoolLoginSession,
  AccountPoolLoginStartResult,
  AccountPoolProvider,
  AccountPoolStateLog,
  AccountPoolUsageLog,
  ApiResponse,
  PageResponse,
  PoolAccount,
  PoolAccountBatchPayload,
  PoolAccountBatchStatusPayload,
  PoolAccountPayload,
  AccountPoolStats,
} from './types'

export const accountPoolQueryKeys = {
  groups: (params?: unknown) => ['account-pool', 'groups', params] as const,
  groupOptions: () => ['account-pool', 'groups', 'options'] as const,
  authFiles: (params?: unknown) =>
    ['account-pool', 'auth-files', params] as const,
  providers: () => ['account-pool', 'providers'] as const,
  loginSession: (sessionId: string) =>
    ['account-pool', 'login-sessions', sessionId] as const,
  accounts: (groupId: number, params?: unknown) =>
    ['account-pool', 'groups', groupId, 'accounts', params] as const,
  usageLogs: (params?: unknown) =>
    ['account-pool', 'usage-logs', params] as const,
  stateLogs: (params?: unknown) =>
    ['account-pool', 'state-logs', params] as const,
}

export async function getAccountPoolProviders(): Promise<
  ApiResponse<AccountPoolProvider[]>
> {
  const res = await api.get('/api/account-pool/providers')
  return res.data
}

export async function getAccountPoolGroups(params: {
  p?: number
  page_size?: number
  status?: number
  search?: string
}): Promise<ApiResponse<PageResponse<AccountPoolGroup>>> {
  const res = await api.get('/api/account-pool/groups', { params })
  return res.data
}

export async function getAccountPoolGroupOptions(): Promise<
  ApiResponse<AccountPoolGroupOption[]>
> {
  const res = await api.get('/api/account-pool/groups/options')
  return res.data
}

export async function getAccountPoolAuthFiles(params: {
  p?: number
  page_size?: number
  status?: number
  pool_group_id?: number
  provider?: string
  search?: string
}): Promise<ApiResponse<PageResponse<AccountPoolAuthFile>>> {
  const res = await api.get('/api/account-pool/auth-files', { params })
  return res.data
}

export async function createAccountPoolAuthFile(
  data: AccountPoolAuthFilePayload
): Promise<ApiResponse<AccountPoolAuthFileMutationResult>> {
  const res = await api.post('/api/account-pool/auth-files', data)
  return res.data
}

export async function importAccountPoolAuthFiles(
  data: AccountPoolAuthFilePayload
): Promise<ApiResponse<AccountPoolAuthFileBatchImportResult>> {
  const res = await api.post('/api/account-pool/auth-files/import', data)
  return res.data
}

export async function updateAccountPoolAuthFile(
  authFileId: number,
  data: AccountPoolAuthFileUpdatePayload
): Promise<ApiResponse<AccountPoolAuthFileMutationResult>> {
  const res = await api.put(`/api/account-pool/auth-files/${authFileId}`, data)
  return res.data
}

export async function deleteAccountPoolAuthFile(
  authFileId: number,
  deleteAccount = true
): Promise<ApiResponse<null>> {
  const res = await api.delete(`/api/account-pool/auth-files/${authFileId}`, {
    params: { delete_account: deleteAccount },
  })
  return res.data
}

export async function createAccountPoolGroup(
  data: AccountPoolGroupPayload
): Promise<ApiResponse<AccountPoolGroup>> {
  const res = await api.post('/api/account-pool/groups', data)
  return res.data
}

export async function updateAccountPoolGroup(
  groupId: number,
  data: AccountPoolGroupPayload
): Promise<ApiResponse<AccountPoolGroup>> {
  const res = await api.put(`/api/account-pool/groups/${groupId}`, data)
  return res.data
}

export async function deleteAccountPoolGroup(
  groupId: number
): Promise<ApiResponse<null>> {
  const res = await api.delete(`/api/account-pool/groups/${groupId}`)
  return res.data
}

export async function getPoolAccounts(
  groupId: number,
  params: {
    p?: number
    page_size?: number
    status?: number
    search?: string
  }
): Promise<
  ApiResponse<{
    accounts: PageResponse<PoolAccount>
    stats?: AccountPoolStats
  }>
> {
  const res = await api.get(`/api/account-pool/groups/${groupId}/accounts`, {
    params,
  })
  return res.data
}

export async function getAccountPoolUsageLogs(params: {
  p?: number
  page_size?: number
  pool_group_id?: number
  pool_account_id?: number
  channel_id?: number
  user_id?: number
  success?: boolean
  start_timestamp?: number
  end_timestamp?: number
  model_name?: string
  request_id?: string
  upstream_request_id?: string
  search?: string
}): Promise<ApiResponse<PageResponse<AccountPoolUsageLog>>> {
  const res = await api.get('/api/account-pool/usage-logs', { params })
  return res.data
}

export async function getAccountPoolStateLogs(params: {
  p?: number
  page_size?: number
  pool_group_id?: number
  pool_account_id?: number
  action?: string
  source?: string
  actor?: string
  start_timestamp?: number
  end_timestamp?: number
  search?: string
}): Promise<ApiResponse<PageResponse<AccountPoolStateLog>>> {
  const res = await api.get('/api/account-pool/state-logs', { params })
  return res.data
}

export async function createPoolAccount(
  groupId: number,
  data: PoolAccountPayload
): Promise<ApiResponse<PoolAccount>> {
  const res = await api.post(
    `/api/account-pool/groups/${groupId}/accounts`,
    data
  )
  return res.data
}

export async function batchCreatePoolAccounts(
  groupId: number,
  data: PoolAccountBatchPayload
): Promise<ApiResponse<{ created: number; skipped: number }>> {
  const res = await api.post(
    `/api/account-pool/groups/${groupId}/accounts/batch`,
    data
  )
  return res.data
}

export async function updatePoolAccount(
  accountId: number,
  data: PoolAccountPayload
): Promise<ApiResponse<PoolAccount>> {
  const res = await api.put(`/api/account-pool/accounts/${accountId}`, data)
  return res.data
}

export async function deletePoolAccount(
  accountId: number
): Promise<ApiResponse<null>> {
  const res = await api.delete(`/api/account-pool/accounts/${accountId}`)
  return res.data
}

export async function updatePoolAccountStatus(
  accountId: number,
  data: {
    status?: number
    reason?: string
    clear_cooldown?: boolean
    schedulable?: boolean
  }
): Promise<ApiResponse<null>> {
  const res = await api.post(
    `/api/account-pool/accounts/${accountId}/status`,
    data
  )
  return res.data
}

export async function batchUpdatePoolAccountStatus(
  groupId: number,
  data: PoolAccountBatchStatusPayload
): Promise<ApiResponse<AccountPoolBatchStatusResult>> {
  const res = await api.post(
    `/api/account-pool/groups/${groupId}/accounts/status`,
    data
  )
  return res.data
}

export async function refreshPoolAccountCredential(
  accountId: number
): Promise<ApiResponse<PoolAccount>> {
  const res = await api.post(`/api/account-pool/accounts/${accountId}/refresh`)
  return res.data
}

export async function checkPoolAccount(
  accountId: number
): Promise<ApiResponse<AccountPoolCheckResult>> {
  const res = await api.post(`/api/account-pool/accounts/${accountId}/check`)
  return res.data
}

export async function checkPoolAccountsInGroup(
  groupId: number,
  data?: { account_ids?: number[]; limit?: number }
): Promise<ApiResponse<AccountPoolBatchCheckResult>> {
  const res = await api.post(
    `/api/account-pool/groups/${groupId}/accounts/check`,
    data
  )
  return res.data
}

export async function resetPoolAccountRuntime(
  accountId: number
): Promise<ApiResponse<null>> {
  const res = await api.post(
    `/api/account-pool/accounts/${accountId}/runtime/reset`
  )
  return res.data
}

export async function startAccountPoolProviderOAuth(
  groupId: number,
  provider: string,
  data: { name?: string; proxy?: string }
): Promise<ApiResponse<AccountPoolLoginStartResult>> {
  const res = await api.post(
    `/api/account-pool/groups/${groupId}/oauth/${provider}/start`,
    data
  )
  return res.data
}

export async function completeAccountPoolProviderOAuth(
  groupId: number,
  provider: string,
  data: { session_id?: string; input: string; name?: string; proxy?: string }
): Promise<ApiResponse<PoolAccount>> {
  const res = await api.post(
    `/api/account-pool/groups/${groupId}/oauth/${provider}/complete`,
    data
  )
  return res.data
}

export async function startAccountPoolProviderDevice(
  groupId: number,
  provider: string,
  data: { name?: string; proxy?: string }
): Promise<ApiResponse<AccountPoolLoginStartResult>> {
  const res = await api.post(
    `/api/account-pool/groups/${groupId}/device/${provider}/start`,
    data
  )
  return res.data
}

export async function getAccountPoolLoginSession(
  sessionId: string
): Promise<ApiResponse<AccountPoolLoginSession>> {
  const res = await api.get(`/api/account-pool/login-sessions/${sessionId}`)
  return res.data
}

export async function startAccountPoolCodexOAuth(data: {
  pool_group_id: number
  proxy?: string
}): Promise<ApiResponse<{ authorize_url?: string; session_id?: string }>> {
  const res = await api.post('/api/account-pool/oauth/codex/start', data)
  return res.data
}

export async function completeAccountPoolCodexOAuth(data: {
  pool_group_id: number
  input: string
  session_id?: string
  name?: string
  proxy?: string
}): Promise<ApiResponse<PoolAccount>> {
  const res = await api.post('/api/account-pool/oauth/codex/complete', data)
  return res.data
}
