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
export type AccountPoolStats = {
  total: number
  enabled: number
  disabled: number
  cooldown: number
}

export type AccountPoolGroup = {
  id: number
  name: string
  platform: string
  auth_type: string
  status: number
  strategy: string
  models: string
  group: string
  model_mapping?: string | null
  settings: string
  created_time: number
  updated_time: number
  stats?: AccountPoolStats
}

export type PoolAccount = {
  id: number
  pool_group_id: number
  name: string
  platform: string
  auth_type: string
  credential_summary: string
  credential_provider: string
  credential_label: string
  credential_metadata: string
  credential_attributes: string
  status: number
  status_message: string
  schedulable: boolean
  unavailable: boolean
  models: string
  group: string
  priority: number
  weight: number
  max_concurrency: number
  proxy: string
  base_url?: string | null
  openai_organization?: string | null
  other: string
  setting?: string | null
  settings: string
  model_mapping?: string | null
  param_override?: string | null
  header_override?: string | null
  status_code_mapping?: string | null
  last_used_time: number
  used_quota: number
  rate_limited_until: number
  overload_until: number
  temp_disabled_until: number
  disabled_reason: string
  last_error: string
  quota_snapshot: string
  model_states: string
  last_refreshed_time: number
  next_refresh_time: number
  next_retry_time: number
  success_count: number
  failed_count: number
  recent_requests: string
  runtime?: AccountRuntimeView
  created_time: number
  updated_time: number
}

export type AccountRuntimeView = {
  status: string
  status_message?: string
  unavailable: boolean
  last_refreshed_time?: number
  next_refresh_time?: number
  next_retry_time?: number
  success_count: number
  failed_count: number
}

export type PageResponse<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}

export type ApiResponse<T> = {
  success: boolean
  message?: string
  data?: T
}

export type AccountPoolGroupPayload = {
  name: string
  platform: string
  auth_type: string
  status?: number
  strategy: string
  models?: string
  group?: string
  model_mapping?: string | null
  settings?: string
}

export type PoolAccountPayload = {
  name: string
  platform?: string
  auth_type?: string
  credentials?: string
  status?: number
  schedulable?: boolean
  models?: string
  group?: string
  priority?: number
  weight?: number
  max_concurrency?: number
  proxy?: string
  base_url?: string | null
  openai_organization?: string | null
  other?: string
  setting?: string | null
  settings?: string
  model_mapping?: string | null
  param_override?: string | null
  header_override?: string | null
  status_code_mapping?: string | null
}

export type PoolAccountBatchPayload = {
  credentials: string
  name_prefix?: string
  platform?: string
  auth_type?: string
  models?: string
  group?: string
  priority?: number
  weight?: number
  status?: number
  max_concurrency?: number
}

export type AccountPoolGroupOption = {
  id: number
  name: string
  platform: string
  auth_type: string
  strategy: string
  stats?: AccountPoolStats
}

export type AccountPoolProvider = {
  name: string
  display_name: string
  supports_oauth: boolean
  supports_device: boolean
  supports_refresh: boolean
}

export type AccountPoolLoginStartResult = {
  session_id: string
  provider: string
  mode: string
  authorize_url?: string
  verification_url?: string
  user_code?: string
  expires_at?: number
  poll_interval?: number
}

export type AccountPoolLoginSession = {
  session_id: string
  account_id?: number
  provider: string
  mode: string
  status: 'pending' | 'completed' | 'failed' | 'cancelled'
  status_message?: string
  verification_url?: string
  user_code?: string
  expires_at?: number
  poll_interval?: number
}
