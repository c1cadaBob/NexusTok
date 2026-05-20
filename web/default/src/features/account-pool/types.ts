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
  status: number
  schedulable: boolean
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
  created_time: number
  updated_time: number
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
