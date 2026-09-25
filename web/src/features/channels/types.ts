/*
Copyright (C) 2023-2026 c1cadaBob

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

For commercial licensing, please contact support@c1cadabob.dev
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
})

export type ChannelInfo = z.infer<typeof channelInfoSchema>

export const channelSchema = z.object({
  id: z.number(),
  type: z.number(),
  upstream_kind: z
    .enum(['key_channel', 'platform_site'])
    .default('key_channel'),
  key_priority: z.number().nullish(),
  conversion_ratio: z.number().nullish(),
  key_weight_override: z.number().nullish(),
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
  model_ratio: z.number().nullish(),
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
  settings: z.string().default('{}'), // other_settings JSON
})

export type Channel = z.infer<typeof channelSchema> & {
  children?: Channel[]
  is_upstream_key?: boolean
  upstream_key?: UpstreamKey
  upstream_keys?: UpstreamKey[]
  upstream_site_status?: UpstreamSiteStatus
  parent_channel_id?: number
  upstream_group?: string
}

// ============================================================================
// Channel Settings Types
// ============================================================================

export interface ChannelSettings {
  task_plugin_key?: string
  force_format?: boolean
  thinking_to_content?: boolean
  proxy?: string
  pass_through_body_enabled?: boolean
  system_prompt?: string
  system_prompt_override?: boolean
  http_protocol?: 'auto' | 'http1' | string
  http2_connection_shards?: number
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

export interface AdvancedCustomConfig {
  advanced_routes?: AdvancedCustomRoute[]
}

export interface AdvancedCustomRoute {
  incoming_path?: string
  upstream_path?: string
  converter?: AdvancedCustomConverter
  models?: string[]
  auth?: AdvancedCustomRouteAuth
}

export interface AdvancedCustomRouteAuth {
  type?: AdvancedCustomAuthType
  name?: string
  value?: string
}

export type AdvancedCustomConverter =
  | 'none'
  | 'anthropic_messages_to_openai_chat_completions'
  | 'openai_chat_completions_to_anthropic_messages'
  | 'openai_chat_completions_to_openai_responses'
  | 'openai_responses_to_openai_chat_completions'
  | 'openai_responses_to_gemini_generate_content'
  | 'gemini_generate_content_to_openai_chat_completions'
  | 'openai_chat_completions_to_gemini_generate_content'

export type AdvancedCustomAuthType = 'none' | 'header' | 'query'

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

export interface ChannelOpsResponse {
  success: boolean
  message?: string
  data?: {
    retry_times: number
  }
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
  sync_status?: string
  last_sync_at?: number
  last_sync_error?: string
  key_count?: number
  routable_key_count?: number
  currency?: string
  raw_response?: string
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

// ============================================================================
// Multi-Key Management Types
// ============================================================================

export interface KeyStatus {
  index: number
  key_id?: number
  status: number // 1: enabled, 2: manual disabled, 3: auto disabled
  disabled_time?: number
  reason?: string
  key_preview?: string
  key_priority?: number
  weight?: number
  auto_weight?: number
  health_status?: 'disabled' | 'enabled' | 'normal' | 'degraded' | 'invalid'
  health_reason?: string
  health_sample_count?: number
  health_success_count?: number
  health_first_latency_ms?: number
  last_used_at?: number
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
  keyId?: number
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
  | 'model_ratio'

export type ChannelSortOrder = 'asc' | 'desc'

export interface GetChannelsParams {
  p?: number
  page_size?: number
  status?: string // 'enabled', 'disabled', or empty for all
  type?: number
  group?: string
  model?: string
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
  key_id?: number
  upstream_key_id?: number
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
    | 'update_key'
  key_id?: number
  key_index?: number
  key_priority?: number
  weight_override?: number
  clear_weight?: boolean
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
  platform_site?: PlatformSiteInput
}

export type UpstreamKind = 'key_channel' | 'platform_site'
export type UpstreamAuthType =
  | 'password'
  | 'access_token'
  | 'admin_key'
  | 'cookie'
export type PlatformSiteCaptureAuthType = 'auto' | UpstreamAuthType

export interface PlatformSiteInput {
  platform: 'newapi' | 'sub2api'
  base_url: string
  relay_base_url?: string
  auth_type: UpstreamAuthType | 'auto'
  capture_id?: string
  username?: string
  password?: string
  user_id?: string
  access_token?: string
  refresh_token?: string
  admin_key?: string
  cookie?: string
  recharge_amount: number
  credited_amount: number
  conversion_ratio?: number
}

export interface UpstreamSiteStatus {
  channel_id: number
  platform: string
  base_url: string
  auth_type: UpstreamAuthType
  recharge_amount: number
  credited_amount: number
  conversion_ratio: number
  balance: number
  used_quota: number
  balance_updated_time: number
  sync_status: string
  last_sync_at: number
  last_sync_error?: string
  consecutive_failures: number
  key_count?: number
  routable_key_count?: number
  snapshot_usable?: boolean
  using_last_snapshot?: boolean
  credential_available?: boolean
  needs_credential_save?: boolean
  routable?: boolean
  availability_reason?: string
}

export interface UpstreamKey {
  id: number
  key_id: number
  channel_id: number
  external_id: string
  name: string
  key_preview?: string
  models: string[]
  allowed_models?: string[] | null
  models_synced: boolean
  key_priority: number
  source_conversion_ratio: number
  conversion_ratio: number
  conversion_ratio_override?: number | null
  weight: number
  auto_weight: number
  weight_override?: number | null
  status: number
  disabled_reason?: string
  last_sync_at: number
  last_used_at?: number
  routable?: boolean
  availability_reason?: string
  snapshot_only?: boolean
  credential_unavailable?: boolean
  health_status?: 'disabled' | 'enabled' | 'normal' | 'degraded' | 'invalid'
  health_reason?: string
  health_sample_count?: number
  health_success_count?: number
  health_first_latency_ms?: number
}

export interface UpstreamSiteStatusResponse {
  success: boolean
  message?: string
  data?: UpstreamSiteStatus
}

export interface PlatformSiteCaptureStartRequest {
  platform: 'newapi' | 'sub2api'
  base_url: string
  auth_type: PlatformSiteCaptureAuthType
  channel_id?: number
}

export interface PlatformSiteCaptureSummary {
  platform: string
  auth_type: Exclude<PlatformSiteCaptureAuthType, 'auto'>
  base_url: string
  management_base_url?: string
  relay_base_url?: string
  api_base_url?: string
  origin: string
  access_token_masked?: string
  refresh_token_present?: boolean
  admin_key_present?: boolean
  cookie_present?: boolean
  user_id?: string
  username?: string
  email?: string
  token_expires_at?: number
  captured_at?: number
}

export interface PlatformSiteCaptureStartResponse {
  success: boolean
  message?: string
  data?: {
    capture_id: string
    expires_at: number
    platform: string
    base_url: string
    auth_type: PlatformSiteCaptureAuthType
    origin: string
    userscript_url: string
    helper_install_url: string
    handoff_url: string
    login_url: string
  }
}

export interface PlatformSiteCaptureStatusResponse {
  success: boolean
  message?: string
  data?: {
    capture_id: string
    status: 'pending' | 'completed' | 'failed'
    message?: string
    expires_at: number
    platform: string
    base_url: string
    auth_type: PlatformSiteCaptureAuthType
    origin: string
    userscript_url?: string
    helper_install_url?: string
    handoff_url?: string
    login_url?: string
    summary?: PlatformSiteCaptureSummary
  }
}

export interface UpstreamKeysResponse {
  success: boolean
  message?: string
  data?: {
    items: UpstreamKey[]
    total: number
  }
}
