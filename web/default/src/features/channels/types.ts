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
import { z } from 'zod'

// ============================================================================
// Channel Schema & Types
// ============================================================================

export const channelInfoSchema = z.object({
  is_multi_key: z.boolean().default(false),
  multi_key_size: z.number().default(0),
  multi_key_status_list: z.record(z.string(), z.number()).optional(),
  multi_key_disabled_reason: z.record(z.string(), z.string()).optional(),
  multi_key_disabled_time: z.record(z.string(), z.number()).optional(),
  multi_key_polling_index: z.number().default(0),
  multi_key_mode: z.enum(['random', 'polling']).default('random'),
  credential_mode: z
    .enum(['single_key', 'multi_key', 'account_pool', 'global_account_pool'])
    .optional(),
  account_pool_enabled: z.boolean().optional(),
  account_pool_mode: z.enum(['polling', 'random']).optional(),
  account_pool_fallback: z.boolean().optional(),
  account_pool_group_id: z.number().optional(),
})

export type ChannelInfo = z.infer<typeof channelInfoSchema>

export const channelAccountStatsSchema = z.object({
  total: z.number().default(0),
  enabled: z.number().default(0),
  disabled: z.number().default(0),
  cooldown: z.number().default(0),
})

export type ChannelAccountStats = z.infer<typeof channelAccountStatsSchema>

export const channelSchema = z.object({
  id: z.number(),
  type: z.number(),
  key: z.string(),
  openai_organization: z.string().nullish(),
  test_model: z.string().nullish(),
  status: z.number(), // 1: enabled, 0: manual disabled, 2: auto disabled
  name: z.string(),
  weight: z.number().nullish(),
  created_time: z.number(),
  test_time: z.number(),
  response_time: z.number(), // in milliseconds
  base_url: z.string().nullish(),
  other: z.string().default(''),
  balance: z.number().default(0), // in USD
  balance_updated_time: z.number(),
  models: z.string().default(''),
  group: z.string().default('default'),
  used_quota: z.number().default(0),
  model_mapping: z.string().nullish(),
  status_code_mapping: z.string().nullish(),
  priority: z.number().nullish(),
  auto_ban: z.number().nullish(),
  other_info: z.string().default(''),
  tag: z.string().nullish(),
  setting: z.string().nullish(),
  param_override: z.string().nullish(),
  header_override: z.string().nullish(),
  remark: z.string().default(''),
  max_input_tokens: z.number().default(0),
  channel_info: channelInfoSchema.default({
    is_multi_key: false,
    multi_key_size: 0,
    multi_key_polling_index: 0,
    multi_key_mode: 'random',
  }),
  channel_account_stats: channelAccountStatsSchema.optional(),
  settings: z.string().default('{}'), // other_settings JSON
})

export type Channel = z.infer<typeof channelSchema>

// ============================================================================
// Channel Settings Types
// ============================================================================

export interface ChannelSettings {
  force_format?: boolean
  thinking_to_content?: boolean
  proxy?: string
  pass_through_body_enabled?: boolean
  system_prompt?: string
  system_prompt_override?: boolean
}

export interface ChannelOtherSettings {
  azure_responses_version?: string
  vertex_key_type?: 'json' | 'api_key'
  openrouter_enterprise?: boolean
  aws_key_type?: 'ak_sk' | 'api_key'
  allow_service_tier?: boolean
  disable_store?: boolean
  allow_safety_identifier?: boolean
  allow_include_obfuscation?: boolean
  allow_inference_geo?: boolean
  allow_speed?: boolean
  claude_beta_query?: boolean
  disable_task_polling_sleep?: boolean
  upstream_model_update_check_enabled?: boolean
  upstream_model_update_auto_sync_enabled?: boolean
  upstream_model_update_ignored_models?: string[]
  upstream_model_update_last_check_time?: number
  upstream_model_update_last_detected_models?: string[]
  advanced_custom?: AdvancedCustomConfig
}

export type AdvancedCustomConverter =
  | 'none'
  | 'anthropic_messages_to_openai_chat_completions'
  | 'openai_chat_completions_to_anthropic_messages'
  | 'openai_chat_completions_to_openai_responses'
  | 'openai_responses_to_openai_chat_completions'
  | 'gemini_generate_content_to_openai_chat_completions'
  | 'openai_chat_completions_to_gemini_generate_content'

export type AdvancedCustomAuthType = 'none' | 'header' | 'query'

export interface AdvancedCustomRouteAuth {
  type?: AdvancedCustomAuthType
  name?: string
  value?: string
}

export interface AdvancedCustomRoute {
  incoming_path?: string
  upstream_path?: string
  converter?: AdvancedCustomConverter
  auth?: AdvancedCustomRouteAuth
}

export interface AdvancedCustomConfig {
  advanced_routes?: AdvancedCustomRoute[]
}

// ============================================================================
// API Response Types
// ============================================================================

export interface GetChannelsResponse {
  success: boolean
  message?: string
  data?: {
    items: Channel[]
    total: number
    page: number
    page_size: number
    type_counts?: Record<string, number>
  }
}

export interface SearchChannelsResponse {
  success: boolean
  message?: string
  data?: {
    items: Channel[]
    total: number
    type_counts?: Record<string, number>
  }
}

export interface GetChannelResponse {
  success: boolean
  message?: string
  data?: Channel
}

export interface ChannelTestResponse {
  success: boolean
  message?: string
  error_code?: string
  time?: number
  data?: {
    response_time?: number
    error?: string
  }
}

export interface ChannelBalanceResponse {
  success: boolean
  message?: string
  balance?: number
  used_quota?: number
  balance_updated_time?: number
  currency?: string
}

export interface FetchModelsResponse {
  success: boolean
  message?: string
  data?: string[]
}

export interface CopyChannelResponse {
  success: boolean
  message?: string
  data?: {
    id: number
  }
}

export type UpstreamAccountPlatform = 'new-api' | 'sub2api'
export type UpstreamAccountAuthMode =
  | 'password'
  | 'session_cookie'
  | 'access_token'
  | 'oauth_browser'

export interface UpstreamAccountBalanceSnapshot {
  balance_usd?: number
  used_usd?: number
  raw_balance?: number
  raw_used?: number
  quota_per_unit?: number
  source?: string
  partial?: boolean
  missing_used_value?: boolean
  missing_balance_value?: boolean
}

export interface UpstreamAccountGroup {
  id?: string
  name: string
  platform?: string
  ratio?: number
  peak_ratio?: number
  description?: string
  model_ratios?: Record<string, number>
}

export interface UpstreamAccountKey {
  sync_id?: string
  external_id?: string
  name?: string
  masked_key?: string
  status?: number
  group_id?: string
  group_name?: string
  models?: string[]
  model_ratios?: Record<string, number>
  group_ratio?: number
  effective_ratio?: number
  ratio_conversion?: number
  quota_limit_usd?: number
  quota_used_usd?: number
  quota_remaining_usd?: number
  unlimited?: boolean
  suggested_priority: number
  suggested_weight: number
}

export interface UpstreamAccountRatioConversion {
  paid_cny?: number
  platform_usd_credit?: number
  enabled?: boolean
}

export interface UpstreamAccountSnapshot {
  platform: UpstreamAccountPlatform
  base_url: string
  management_base_url?: string
  relay_base_url?: string
  balance?: UpstreamAccountBalanceSnapshot
  groups: UpstreamAccountGroup[]
  keys: UpstreamAccountKey[]
  ratio_conversion?: UpstreamAccountRatioConversion
  warnings?: string[]
}

export interface UpstreamAccountPreviewRequest {
  platform: UpstreamAccountPlatform
  base_url: string
  username?: string
  email?: string
  password?: string
  auth_mode?: UpstreamAccountAuthMode
  capture_id?: string
  session_cookie?: string
  user_id?: string
  access_token?: string
  refresh_token?: string
  expires_at?: number
  channel_id?: number
  ratio_conversion?: UpstreamAccountRatioConversion
}

export interface UpstreamAccountPreviewData {
  preview_id: string
  expires_at: number
  snapshot: UpstreamAccountSnapshot
}

export interface UpstreamAccountTwoFactorChallenge {
  challenge_id: string
  platform: UpstreamAccountPlatform
  type: 'totp'
  expires_at: number
  username?: string
}

export interface UpstreamAccountPreviewChallengeData {
  expires_at: number
  challenge: UpstreamAccountTwoFactorChallenge
}

export type UpstreamAccountPreviewResponseData =
  | UpstreamAccountPreviewData
  | UpstreamAccountPreviewChallengeData

export interface UpstreamAccountPreviewResponse {
  success: boolean
  message?: string
  data?: UpstreamAccountPreviewResponseData
}

export interface UpstreamAccountPreview2FARequest {
  challenge_id: string
  code: string
  ratio_conversion?: UpstreamAccountRatioConversion
}

export interface UpstreamAccountPreview2FAResponse {
  success: boolean
  message?: string
  data?: UpstreamAccountPreviewData
}

export interface UpstreamAccountCreateAccountConfig {
  sync_id?: string
  external_id?: string
  name?: string
  enabled?: boolean
  models?: string
  group?: string
  priority?: number
  weight?: number
}

export interface UpstreamAccountCreateRequest {
  preview_id: string
  apply_suggested: boolean
  ratio_conversion?: UpstreamAccountRatioConversion
  channel: {
    name: string
    type: number
    base_url?: string | null
    models?: string
    group?: string
    status?: number
    priority?: number | null
    weight?: number | null
  }
  accounts?: UpstreamAccountCreateAccountConfig[]
}

export interface UpstreamAccountCreateResponse {
  success: boolean
  message?: string
  data?: {
    channel_id: number
    created: number
    skipped: number
  }
}

export interface UpstreamAccountRefreshRequest {
  preview_id?: string
  platform?: UpstreamAccountPlatform
  base_url?: string
  username?: string
  email?: string
  password?: string
  auth_mode?: UpstreamAccountAuthMode
  capture_id?: string
  session_cookie?: string
  user_id?: string
  access_token?: string
  refresh_token?: string
  expires_at?: number
  apply_suggested: boolean
  disable_missing_key?: boolean
  ratio_conversion?: UpstreamAccountRatioConversion
  accounts?: UpstreamAccountCreateAccountConfig[]
}

export interface UpstreamAccountRefreshResponse {
  success: boolean
  message?: string
  data?: {
    channel_id: number
    created: number
    updated: number
    disabled: number
  }
}

export interface UpstreamAccountCaptureStartRequest {
  platform: UpstreamAccountPlatform
  base_url: string
  channel_id?: number
  return_url?: string
}

export interface UpstreamAccountCaptureStartData {
  capture_id: string
  expires_at: number
  platform: UpstreamAccountPlatform
  base_url: string
  management_base_url?: string
  relay_base_url?: string
  api_base_url?: string
  origin: string
  userscript_url: string
  login_url: string
  return_url?: string
}

export interface UpstreamAccountCaptureCredentialSummary {
  platform: UpstreamAccountPlatform
  auth_mode: UpstreamAccountAuthMode
  base_url: string
  management_base_url?: string
  relay_base_url?: string
  api_base_url?: string
  origin: string
  user_id?: string
  username?: string
  email?: string
  access_token_masked?: string
  refresh_token_present?: boolean
  expires_at?: number
  captured_at?: number
  capture_source?: string
}

export interface UpstreamAccountCaptureDiagnostics {
  page_origin?: string
  api_base_url_seen?: string
  local_storage_keys?: string[]
  session_storage_keys?: string[]
  auth_token_present?: boolean
  access_token_present?: boolean
  refresh_token_present?: boolean
  oauth_hash_token_present?: boolean
  auth_client_id_present?: boolean
  auth_me_path?: string
  browser_session_restore_path?: string
  browser_session_restore_status?: string
  browser_session_restore_message?: string
}

export interface UpstreamAccountCaptureStatusData {
  capture_id: string
  status: 'pending' | 'completed' | 'failed'
  message?: string
  expires_at: number
  platform: UpstreamAccountPlatform
  base_url: string
  management_base_url?: string
  relay_base_url?: string
  api_base_url?: string
  origin: string
  userscript_url?: string
  login_url?: string
  return_url?: string
  summary?: UpstreamAccountCaptureCredentialSummary
  diagnostics?: UpstreamAccountCaptureDiagnostics
}

export interface UpstreamAccountCaptureStartResponse {
  success: boolean
  message?: string
  data?: UpstreamAccountCaptureStartData
}

export interface UpstreamAccountCaptureStatusResponse {
  success: boolean
  message?: string
  data?: UpstreamAccountCaptureStatusData
}

// ============================================================================
// Multi-Key Management Types
// ============================================================================

export interface KeyStatus {
  index: number
  status: number // 1: enabled, 2: manual disabled, 3: auto disabled
  disabled_time?: number
  reason?: string
  key_preview?: string
}

export type MultiKeyConfirmAction = {
  type:
    | 'enable'
    | 'disable'
    | 'delete'
    | 'enable-all'
    | 'disable-all'
    | 'delete-disabled'
  keyIndex?: number
}

export interface MultiKeyStatusResponse {
  success: boolean
  message?: string
  data?: {
    keys: KeyStatus[]
    total: number
    page: number
    page_size: number
    total_pages: number
    enabled_count: number
    manual_disabled_count: number
    auto_disabled_count: number
  }
}

export type ChannelCredentialMode =
  | 'single_key'
  | 'multi_key'
  | 'account_pool'
  | 'global_account_pool'
export type ChannelAccountPoolMode = 'polling' | 'random'

export interface ChannelAccount {
  id: number
  channel_id: number
  name: string
  key: string
  status: number
  models: string
  group: string
  priority: number
  weight: number
  last_used_time: number
  used_quota: number
  base_url?: string | null
  openai_organization?: string | null
  other: string
  setting?: string | null
  settings: string
  model_mapping?: string | null
  param_override?: string | null
  header_override?: string | null
  status_code_mapping?: string | null
  rate_limited_until: number
  overload_until: number
  temp_disabled_until: number
  disabled_reason: string
  last_error: string
  max_concurrency: number
  created_time: number
  key_group_id?: string
  key_group_name?: string
  group_ratio?: number | null
  model_ratios?: Record<string, number>
  effective_ratio?: number
  ratio_conversion?: number
  ratio_conversion_config?: UpstreamAccountRatioConversion | null
}

export interface ChannelAccountListResponse {
  success: boolean
  message?: string
  data?: {
    accounts: {
      items: ChannelAccount[]
      total: number
      page: number
      page_size: number
    }
    stats: ChannelAccountStats
  }
}

export interface ChannelAccountMutationResponse {
  success: boolean
  message?: string
  data?: ChannelAccount
}

export interface ChannelAccountBatchResponse {
  success: boolean
  message?: string
  data?: {
    created: number
    skipped: number
  }
}

export interface ChannelAccountPayload {
  name?: string
  key?: string
  status?: number
  models?: string
  group?: string
  priority?: number
  weight?: number
  base_url?: string | null
  openai_organization?: string | null
  other?: string
  setting?: string | null
  settings?: string
  model_mapping?: string | null
  param_override?: string | null
  header_override?: string | null
  status_code_mapping?: string | null
  max_concurrency?: number
}

export interface ChannelAccountBatchPayload {
  keys: string
  name_prefix?: string
  models?: string
  group?: string
  priority?: number
  weight?: number
  status?: number
  max_concurrency?: number
}

// ============================================================================
// API Request Parameters
// ============================================================================

export type ChannelSortBy =
  | 'id'
  | 'name'
  | 'priority'
  | 'balance'
  | 'response_time'
  | 'test_time'

export type ChannelSortOrder = 'asc' | 'desc'

export interface GetChannelsParams {
  p?: number
  page_size?: number
  status?: string // 'enabled', 'disabled', or empty for all
  type?: number
  group?: string
  id_sort?: boolean
  tag_mode?: boolean
  sort_by?: ChannelSortBy
  sort_order?: ChannelSortOrder
}

export interface SearchChannelsParams {
  keyword?: string
  group?: string
  model?: string
  status?: string
  type?: number
  id_sort?: boolean
  tag_mode?: boolean
  sort_by?: ChannelSortBy
  sort_order?: ChannelSortOrder
  p?: number
  page_size?: number
}

export interface ChannelTestParams {
  test_model?: string
}

export interface CopyChannelParams {
  suffix?: string
  reset_balance?: boolean
}

export interface MultiKeyManageParams {
  channel_id: number
  action:
    | 'get_key_status'
    | 'disable_key'
    | 'enable_key'
    | 'enable_all_keys'
    | 'disable_all_keys'
    | 'delete_key'
    | 'delete_disabled_keys'
  key_index?: number
  page?: number
  page_size?: number
  status?: number // 1=enabled, 2=manual_disabled, 3=auto_disabled
}

export interface BatchDeleteParams {
  ids: number[]
}

export interface BatchSetTagParams {
  ids: number[]
  tag: string | null
}

export interface TagOperationParams {
  tag: string
  new_tag?: string
  priority?: number
  weight?: number
  model_mapping?: string
  models?: string
  groups?: string
}

// ============================================================================
// Form Data Types
// ============================================================================

export interface ChannelFormData {
  name: string
  type: number
  base_url: string
  key: string
  openai_organization?: string
  models: string
  group: string
  model_mapping?: string
  priority?: number
  weight?: number
  test_model?: string
  auto_ban?: number
  status: number
  status_code_mapping?: string
  tag?: string
  remark?: string
  setting?: string
  param_override?: string
  header_override?: string
  settings?: string
  other?: string
  channel_info?: Partial<ChannelInfo>
  // Multi-key specific
  multi_key_mode?: 'single' | 'batch' | 'multi_to_single'
  multi_key_type?: 'random' | 'polling'
  batch_add_set_key_prefix_2_name?: boolean
}

// ============================================================================
// Add Channel Request (special structure)
// ============================================================================

export interface AddChannelRequest {
  mode: 'single' | 'batch' | 'multi_to_single'
  multi_key_mode?: 'random' | 'polling'
  batch_add_set_key_prefix_2_name?: boolean
  channel: Partial<Channel>
}
