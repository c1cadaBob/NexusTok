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
  unavailable?: number
}

export type AccountPoolDailyLimitState = {
  limited: boolean
  limit_type?: 'daily_request' | 'daily_quota' | string
  reason?: string
  window_start: number
  next_reset_time?: number
}

export type AccountPoolPreflightCheckMode = 'off' | 'warmup' | 'require_recent'

export type AccountPoolNoAvailableAction = 'fail' | 'wait'

export type AccountPoolTaskLimitAction = 'fail' | 'wait'

export type AccountPoolGroup = {
  id: number
  name: string
  platform: string
  auth_type: string
  source?: 'native' | 'cliproxyapi' | string
  external_group_key?: string
  status: number
  strategy: string
  models: string
  group: string
  model_mapping?: string | null
  settings: string
  max_concurrency: number
  rate_limit_rpm: number
  daily_request_limit: number
  daily_quota_limit: number
  daily_limit_action: string
  daily_request_count: number
  used_quota: number
  daily_used_quota: number
  daily_reset_time: number
  daily_limit_state?: AccountPoolDailyLimitState
  auto_check_enabled: boolean
  auto_check_interval_minutes: number
  auto_check_limit: number
  auto_check_last_time: number
  auto_check_next_time: number
  auto_check_last_task_id: number
  preflight_check_mode: AccountPoolPreflightCheckMode | string
  preflight_check_freshness_minutes: number
  preflight_check_limit: number
  no_available_action: AccountPoolNoAvailableAction | string
  no_available_wait_seconds: number
  task_max_concurrency: number
  task_rate_limit_rpm: number
  task_limit_action: AccountPoolTaskLimitAction | string
  task_limit_wait_seconds: number
  created_time: number
  updated_time: number
  stats?: AccountPoolStats
}

export type PoolAccount = {
  id: number
  pool_group_id: number
  auth_file_id?: number
  name: string
  platform: string
  auth_type: string
  credential_summary: string
  credential_provider: string
  credential_label: string
  status: number
  status_message: string
  schedulable: boolean
  unavailable: boolean
  models: string
  group: string
  priority: number
  weight: number
  max_concurrency: number
  rate_limit_rpm: number
  daily_request_limit: number
  daily_quota_limit: number
  daily_limit_action: string
  daily_request_count: number
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
  daily_used_quota: number
  daily_reset_time: number
  rate_limited_until: number
  overload_until: number
  temp_disabled_until: number
  disabled_reason: string
  last_error: string
  quota_snapshot: string
  model_states: string
  last_checked_time: number
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

export type AccountPoolHealthTotals = {
  group_count: number
  limited_group_count: number
  total_accounts: number
  available_accounts: number
  disabled_accounts: number
  cooldown_accounts: number
  unavailable_accounts: number
  today_requests: number
  today_successes: number
  today_failures: number
  success_rate: number
  availability_rate: number
}

export type AccountPoolGroupHealth = {
  id: number
  name: string
  platform: string
  auth_type: string
  status: number
  strategy: string
  stats: AccountPoolStats
  daily_limit_state: AccountPoolDailyLimitState
  auto_check_enabled: boolean
  auto_check_interval_minutes: number
  auto_check_next_time: number
  auto_check_last_task_id: number
  preflight_check_mode: AccountPoolPreflightCheckMode | string
  preflight_check_freshness_minutes: number
  today_requests: number
  today_successes: number
  today_failures: number
  success_rate: number
  availability_rate: number
}

export type AccountPoolAbnormalAccount = {
  id: number
  pool_group_id: number
  pool_group_name: string
  name: string
  platform: string
  auth_type: string
  credential_provider: string
  status: number
  schedulable: boolean
  unavailable: boolean
  cooling_until: number
  reason: string
  status_message: string
  disabled_reason: string
  last_error: string
  last_checked_time: number
  last_used_time: number
  next_retry_time: number
  success_count: number
  failed_count: number
  failure_rate: number
}

export type AccountPoolHealthSummary = {
  generated_at: number
  window_start: number
  window_end: number
  totals: AccountPoolHealthTotals
  groups: AccountPoolGroupHealth[]
  recent_abnormal_accounts: AccountPoolAbnormalAccount[]
  recent_state_logs: AccountPoolStateLog[]
}

export type AccountPoolAuthFile = {
  id: number
  name: string
  source_platform: string
  format: string
  provider: string
  platform: string
  auth_type: string
  pool_group_id: number
  pool_account_id: number
  pool_group_ids?: number[]
  pool_group_names?: string[]
  pool_account_ids?: number[]
  status: number
  file_digest: string
  credential_summary: string
  subscription_type?: string
  account_group?: string
  account_groups: string[]
  models: string
  proxy: string
  base_url?: string | null
  priority: number
  weight: number
  max_concurrency: number
  last_imported_time: number
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
  max_concurrency?: number
  rate_limit_rpm?: number
  daily_request_limit?: number
  daily_quota_limit?: number
  daily_limit_action?: string
  auto_check_enabled?: boolean
  auto_check_interval_minutes?: number
  auto_check_limit?: number
  preflight_check_mode?: AccountPoolPreflightCheckMode | string
  preflight_check_freshness_minutes?: number
  preflight_check_limit?: number
  no_available_action?: AccountPoolNoAvailableAction | string
  no_available_wait_seconds?: number
  task_max_concurrency?: number
  task_rate_limit_rpm?: number
  task_limit_action?: AccountPoolTaskLimitAction | string
  task_limit_wait_seconds?: number
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
  rate_limit_rpm?: number
  daily_request_limit?: number
  daily_quota_limit?: number
  daily_limit_action?: string
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
  rate_limit_rpm?: number
  daily_request_limit?: number
  daily_quota_limit?: number
  daily_limit_action?: string
}

export type PoolAccountAttachPayload = {
  auth_file_ids?: number[]
  source_group_id?: number
  skip_existing?: boolean
}

export type AccountPoolAttachAccountItem = {
  auth_file_id?: number
  auth_file_name?: string
  source_id?: number
  account_id?: number
  account_name?: string
  group_id: number
  success: boolean
  skipped: boolean
  message?: string
}

export type AccountPoolAttachAccountsResult = {
  total: number
  created: number
  skipped: number
  failed: number
  items: AccountPoolAttachAccountItem[]
}

export type PoolAccountBatchStatusPayload = {
  account_ids: number[]
  status?: number
  reason?: string
  clear_cooldown?: boolean
  schedulable?: boolean
}

export type AccountPoolBatchStatusItem = {
  account_id: number
  account_name?: string
  success: boolean
  skipped: boolean
  message?: string
}

export type AccountPoolBatchStatusResult = {
  total: number
  updated: number
  skipped: number
  failed: number
  items: AccountPoolBatchStatusItem[]
}

export type PoolAccountBatchDeletePayload = {
  account_ids: number[]
  reason?: string
}

export type AccountPoolBatchDeleteItem = {
  account_id: number
  account_name?: string
  success: boolean
  skipped: boolean
  message?: string
}

export type AccountPoolBatchDeleteResult = {
  total: number
  deleted: number
  skipped: number
  failed: number
  items: AccountPoolBatchDeleteItem[]
}

export type PoolAccountExportPayload = {
  account_ids?: number[]
}

export type AccountPoolExportAccount = {
  id: number
  pool_group_id: number
  name: string
  platform: string
  auth_type: string
  credential_summary: string
  credential_provider: string
  credential_label: string
  status: number
  status_message: string
  schedulable: boolean
  unavailable: boolean
  models: string
  group: string
  priority: number
  weight: number
  max_concurrency: number
  rate_limit_rpm: number
  daily_request_limit: number
  daily_quota_limit: number
  daily_limit_action: string
  daily_request_count: number
  daily_used_quota: number
  daily_reset_time: number
  proxy_configured: boolean
  base_url_configured: boolean
  openai_organization?: string | null
  has_other_settings: boolean
  has_model_mapping: boolean
  has_param_override: boolean
  has_header_override: boolean
  has_status_code_mapping: boolean
  last_used_time: number
  used_quota: number
  rate_limited_until: number
  overload_until: number
  temp_disabled_until: number
  disabled_reason: string
  last_error: string
  last_checked_time: number
  last_refreshed_time: number
  next_refresh_time: number
  next_retry_time: number
  success_count: number
  failed_count: number
  created_time: number
  updated_time: number
}

export type AccountPoolExportResult = {
  exported_at: number
  format: string
  pool_group: AccountPoolGroup
  total: number
  exported: number
  skipped: number
  skipped_account_ids?: number[]
  credentials_exported: boolean
  sensitive_fields_redacted: string[]
  accounts: AccountPoolExportAccount[]
}

export type AccountPoolAuthFilePayload = {
  name?: string
  content: string
  pool_group_id?: number
  group_name?: string
  provider?: string
  platform?: string
  auth_type?: string
  account_group?: string
  account_groups?: string[]
  models?: string
  proxy?: string
  base_url?: string | null
  priority?: number
  weight?: number
  max_concurrency?: number
  status?: number
  skip_duplicates?: boolean
}

export type AccountPoolAuthFileUpdatePayload = {
  name?: string
  content?: string
  pool_group_id?: number
  group_name?: string
  provider?: string
  platform?: string
  auth_type?: string
  account_group?: string
  account_groups?: string[]
  models?: string
  proxy?: string
  base_url?: string | null
  priority?: number
  weight?: number
  max_concurrency?: number
  status?: number
}

export type AccountPoolAuthFileMutationResult = {
  auth_file: AccountPoolAuthFile
  account: PoolAccount
  group: AccountPoolGroup
}

export type AccountPoolAuthFileImportError = {
  index: number
  name?: string
  message: string
}

export type AccountPoolAuthFileBatchImportResult = {
  created: number
  skipped: number
  failed: number
  items: AccountPoolAuthFileMutationResult[]
  errors: AccountPoolAuthFileImportError[]
}

export type AccountPoolGroupOption = {
  id: number
  name: string
  platform: string
  auth_type: string
  source?: 'native' | 'cliproxyapi' | string
  external_group_key?: string
  strategy: string
  daily_limit_state?: AccountPoolDailyLimitState
  auto_check_enabled?: boolean
  auto_check_interval_minutes?: number
  auto_check_limit?: number
  auto_check_last_time?: number
  auto_check_next_time?: number
  auto_check_last_task_id?: number
  preflight_check_mode?: AccountPoolPreflightCheckMode | string
  preflight_check_freshness_minutes?: number
  preflight_check_limit?: number
  no_available_action?: AccountPoolNoAvailableAction | string
  no_available_wait_seconds?: number
  task_max_concurrency?: number
  task_rate_limit_rpm?: number
  task_limit_action?: AccountPoolTaskLimitAction | string
  task_limit_wait_seconds?: number
  stats?: AccountPoolStats
}

export type AccountPoolProvider = {
  name: string
  display_name: string
  supports_oauth: boolean
  supports_device: boolean
  supports_refresh: boolean
}

export type AccountPoolCheckResult = {
  account_id: number
  account_name: string
  pool_group_id: number
  provider: string
  checked: boolean
  success: boolean
  message: string
  checked_at: number
  refreshed: boolean
  next_retry_time?: number
}

export type AccountPoolBatchCheckResult = {
  total: number
  checked: number
  success: number
  failed: number
  skipped: number
  items: AccountPoolCheckResult[]
}

export type AccountPoolCheckTaskStatus =
  | 'queued'
  | 'running'
  | 'completed'
  | 'failed'

export type AccountPoolCheckTask = {
  id: number
  pool_group_id: number
  pool_group_name: string
  status: AccountPoolCheckTaskStatus
  actor?: string
  request_id?: string
  account_ids: number[]
  total: number
  checked: number
  success: number
  failed: number
  skipped: number
  message: string
  items: AccountPoolCheckResult[]
  started_time: number
  finished_time: number
  created_time: number
  updated_time: number
}

export type AccountPoolCheckTaskListParams = {
  p?: number
  page_size?: number
  pool_group_id?: number
  status?: AccountPoolCheckTaskStatus
  actor?: string
  start_timestamp?: number
  end_timestamp?: number
  search?: string
}

export type AccountPoolCheckTaskCleanupPayload = {
  pool_group_id?: number
  before_timestamp?: number
  statuses?: AccountPoolCheckTaskStatus[]
  limit?: number
}

export type AccountPoolCheckTaskCleanupResult = {
  deleted: number
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

export type AccountPoolUsageLog = {
  id: number
  created_at: number
  pool_group_id: number
  pool_group_name: string
  pool_account_id: number
  pool_account_name: string
  pool_account_auth_type: string
  channel_id: number
  channel_name: string
  model_name: string
  user_id: number
  username: string
  token_id: number
  token_name: string
  group: string
  quota: number
  prompt_tokens: number
  completion_tokens: number
  use_time: number
  is_stream: boolean
  success: boolean
  status_code: number
  error_code: string
  error_message: string
  request_id?: string
  upstream_request_id?: string
  retry_index: number
}

export type AccountPoolStateLog = {
  id: number
  created_at: number
  pool_group_id: number
  pool_group_name: string
  pool_account_id: number
  pool_account_name: string
  pool_account_auth_type: string
  action: string
  source: string
  actor: string
  reason: string
  before_status: number
  after_status: number
  before_schedulable: boolean
  after_schedulable: boolean
  before_unavailable: boolean
  after_unavailable: boolean
  before_next_retry_time: number
  after_next_retry_time: number
  before_status_message: string
  after_status_message: string
  before_disabled_reason: string
  after_disabled_reason: string
  request_id?: string
}

export type AccountPoolStateLogAuditBucket = {
  key: string
  total: number
  latest_at: number
}

export type AccountPoolStateLogAuditAccountRef = {
  id: number
  name: string
}

export type AccountPoolStateLogBulkAuditSummary = {
  action: string
  source: string
  actor: string
  reason: string
  request_id?: string
  pool_group_id: number
  pool_group_name: string
  account_count: number
  first_at: number
  last_at: number
  sample_accounts: AccountPoolStateLogAuditAccountRef[]
}

export type AccountPoolStateLogAuditSummary = {
  generated_at: number
  total: number
  manual_total: number
  automatic_total: number
  affected_accounts: number
  action_stats: AccountPoolStateLogAuditBucket[]
  source_stats: AccountPoolStateLogAuditBucket[]
  actor_stats: AccountPoolStateLogAuditBucket[]
  recent_bulk_operations: AccountPoolStateLogBulkAuditSummary[]
}

export type AccountPoolStateLogExportItem = AccountPoolStateLog

export type AccountPoolStateLogAuditExportResult = {
  exported_at: number
  format: string
  total: number
  exported: number
  limit: number
  filters: Record<string, unknown>
  sensitive_fields_redacted: string[]
  logs: AccountPoolStateLogExportItem[]
}
