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
import {
  CHANNEL_STATUS,
  ERROR_MESSAGES,
  MODEL_FETCHABLE_TYPES,
} from '../constants'
import type { Channel } from '../types'
import {
  CHANNEL_TYPE_ADVANCED_CUSTOM,
  advancedCustomConfigUsesRelativeUpstreamPath,
  parseAdvancedCustomConfig,
  stringifyAdvancedCustomConfig,
  validateAdvancedCustomConfig,
} from './advanced-custom'

// ============================================================================
// 表单校验 Schema
// ============================================================================

const UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY = 'upstream_account_sync'

function parseOptionalJson(value: string | undefined): unknown {
  if (!value?.trim()) return undefined
  return JSON.parse(value)
}

function isJsonObjectValue(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isOptionalJsonObject(value: string | undefined): boolean {
  try {
    const parsed = parseOptionalJson(value)
    return parsed === undefined || isJsonObjectValue(parsed)
  } catch {
    return false
  }
}

function isOptionalModelMapping(value: string | undefined): boolean {
  try {
    const parsed = parseOptionalJson(value)
    if (parsed === undefined) return true
    if (!isJsonObjectValue(parsed)) return false
    return Object.values(parsed).every((item) => typeof item === 'string')
  } catch {
    return false
  }
}

function isOptionalStatusCodeMapping(value: string | undefined): boolean {
  try {
    const parsed = parseOptionalJson(value)
    if (parsed === undefined) return true
    if (!isJsonObjectValue(parsed)) return false
    return Object.entries(parsed).every(([from, to]) => {
      const fromCode = Number(from)
      const toCode = Number(to)
      return (
        Number.isInteger(fromCode) &&
        Number.isInteger(toCode) &&
        fromCode >= 100 &&
        fromCode <= 599 &&
        toCode >= 100 &&
        toCode <= 599
      )
    })
  } catch {
    return false
  }
}

function isCodexCredential(value: string | undefined): boolean {
  try {
    const parsed = parseOptionalJson(value)
    if (parsed === undefined) return true
    return (
      isJsonObjectValue(parsed) &&
      typeof parsed.access_token === 'string' &&
      parsed.access_token.trim().length > 0 &&
      typeof parsed.account_id === 'string' &&
      parsed.account_id.trim().length > 0
    )
  } catch {
    return false
  }
}

function isVertexJsonKey(value: string | undefined): boolean {
  try {
    const parsed = parseOptionalJson(value)
    if (parsed === undefined) return true
    if (Array.isArray(parsed)) {
      return parsed.every((item) => isJsonObjectValue(item))
    }
    return isJsonObjectValue(parsed)
  } catch {
    return false
  }
}

function addRequiredIssue(
  ctx: z.RefinementCtx,
  path: string,
  message: string
): void {
  ctx.addIssue({
    code: z.ZodIssueCode.custom,
    path: [path],
    message,
  })
}

function usesGlobalAccountPool(data: { credential_mode?: string }): boolean {
  return data.credential_mode === 'global_account_pool'
}

function hasUpstreamAccountSyncMetadata(settings: string | undefined): boolean {
  if (!settings?.trim()) return false
  try {
    const parsed = JSON.parse(settings) as Record<string, unknown>
    const metadata = parsed[UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY]
    if (metadata === undefined || metadata === null) {
      return false
    }
    if (typeof metadata === 'object') {
      return true
    }
    if (typeof metadata === 'boolean') {
      return metadata
    }
    return typeof metadata === 'string' && metadata.trim().length > 0
  } catch {
    return false
  }
}

export const channelFormSchema = z
  .object({
    name: z.string().min(1, ERROR_MESSAGES.REQUIRED_NAME),
    type: z.number().min(0, ERROR_MESSAGES.REQUIRED_TYPE),
    base_url: z.string().optional(),
    key: z.string(),
    openai_organization: z.string().optional(),
    models: z.string(),
    group: z.array(z.string()).optional(),
    model_mapping: z
      .string()
      .optional()
      .refine(
        isOptionalModelMapping,
        ERROR_MESSAGES.INVALID_MODEL_MAPPING_OBJECT
      ),
    priority: z.number().optional(),
    weight: z.number().optional(),
    test_model: z.string().optional(),
    auto_ban: z.number().optional(),
    status: z.number(),
    status_code_mapping: z
      .string()
      .optional()
      .refine(
        isOptionalStatusCodeMapping,
        ERROR_MESSAGES.INVALID_STATUS_CODE_MAPPING
      ),
    tag: z.string().optional(),
    remark: z
      .string()
      .max(255, 'Remark must be less than 255 characters')
      .optional(),
    setting: z
      .string()
      .optional()
      .refine(isOptionalJsonObject, ERROR_MESSAGES.INVALID_JSON),
    param_override: z
      .string()
      .optional()
      .refine(isOptionalJsonObject, ERROR_MESSAGES.INVALID_JSON),
    header_override: z
      .string()
      .optional()
      .refine(isOptionalJsonObject, ERROR_MESSAGES.INVALID_JSON),
    settings: z
      .string()
      .optional()
      .refine(isOptionalJsonObject, ERROR_MESSAGES.INVALID_JSON),
    advanced_custom: z.string().optional(),
    other: z.string().optional(),
    // 多 Key 表单选项只用于前端决定创建模式，不作为渠道字段直接保存。
    multi_key_mode: z.enum(['single', 'batch', 'multi_to_single']).optional(),
    multi_key_type: z.enum(['random', 'polling']).optional(),
    credential_mode: z
      .enum(['single_key', 'multi_key', 'account_pool', 'global_account_pool'])
      .optional(),
    account_pool_mode: z.enum(['polling', 'random']).optional(),
    account_pool_fallback: z.boolean().optional(),
    account_pool_group_id: z.number().optional(),
    batch_add_set_key_prefix_2_name: z.boolean().optional(),
    key_mode: z.enum(['append', 'replace']).optional(), // 编辑多 Key 渠道时用于控制覆盖或追加。
    // 渠道扩展设置会汇总进 setting JSON，避免污染后端 Channel 顶层字段。
    force_format: z.boolean().optional(),
    thinking_to_content: z.boolean().optional(),
    proxy: z.string().optional(),
    pass_through_body_enabled: z.boolean().optional(),
    system_prompt: z.string().optional(),
    system_prompt_override: z.boolean().optional(),
    // 类型专属配置统一保存在 settings JSON 中，便于新增 provider 时保持后端结构稳定。
    is_enterprise_account: z.boolean().optional(), // OpenRouter 专属配置。
    vertex_key_type: z.enum(['json', 'api_key']).optional(), // Vertex AI 专属配置。
    aws_key_type: z.enum(['ak_sk', 'api_key']).optional(), // AWS 专属配置。
    azure_responses_version: z.string().optional(), // Azure 专属配置。
    // 字段透传控制统一保存在 settings JSON 中，避免用户无感开启高风险上游参数。
    allow_service_tier: z.boolean().optional(), // OpenAI/Anthropic 透传控制。
    disable_store: z.boolean().optional(), // 仅 OpenAI 使用。
    allow_safety_identifier: z.boolean().optional(), // 仅 OpenAI 使用。
    allow_include_obfuscation: z.boolean().optional(), // OpenAI stream_options.include_obfuscation。
    allow_inference_geo: z.boolean().optional(), // OpenAI/Anthropic 推理地域控制。
    allow_speed: z.boolean().optional(), // Anthropic speed 模式控制。
    claude_beta_query: z.boolean().optional(), // Anthropic beta query 透传控制。
    disable_task_polling_sleep: z.boolean().optional(), // 异步视频任务轮询是否跳过逐任务等待。
    // 上游模型更新设置同样保存在 settings JSON 中，后端定时任务会读取这些字段。
    upstream_model_update_check_enabled: z.boolean().optional(),
    upstream_model_update_auto_sync_enabled: z.boolean().optional(),
    upstream_model_update_ignored_models: z.string().optional(),
    // 上游账号同步创建会先从目标平台读取密钥，再由后端推断模型和类型。
    // 该标记只用于前端表单校验分支，不会写入普通渠道 payload。
    upstream_account_sync: z.boolean().optional(),
  })
  .superRefine((data, ctx) => {
    const isUpstreamAccountSync = data.upstream_account_sync === true
    if (!data.upstream_account_sync && !data.models.trim()) {
      addRequiredIssue(ctx, 'models', ERROR_MESSAGES.REQUIRED_MODELS)
    }

    if (
      [3, 8, 36, 45].includes(data.type) &&
      !isUpstreamAccountSync &&
      !usesGlobalAccountPool(data) &&
      !data.base_url?.trim()
    ) {
      addRequiredIssue(
        ctx,
        'base_url',
        ERROR_MESSAGES.REQUIRED_BASE_URL_FOR_TYPE
      )
    }

    if (data.type === CHANNEL_TYPE_ADVANCED_CUSTOM) {
      const advancedCustomConfig = parseAdvancedCustomConfig(
        data.advanced_custom
      )
      const advancedCustomError =
        validateAdvancedCustomConfig(advancedCustomConfig)
      if (advancedCustomError) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['advanced_custom'],
          message: advancedCustomError.message,
        })
      }
      if (
        advancedCustomConfigUsesRelativeUpstreamPath(advancedCustomConfig) &&
        !data.base_url?.trim()
      ) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['base_url'],
          message:
            'Base URL is required when an advanced route uses an upstream path',
        })
      }
    }

    if (
      [3, 18, 21, 39, 41, 49].includes(data.type) &&
      !isUpstreamAccountSync &&
      !usesGlobalAccountPool(data) &&
      !data.other?.trim()
    ) {
      addRequiredIssue(ctx, 'other', ERROR_MESSAGES.REQUIRED_EXTRA_CONFIG)
    }

    if (data.type === 57) {
      if (data.multi_key_mode && data.multi_key_mode !== 'single') {
        addRequiredIssue(
          ctx,
          'multi_key_mode',
          ERROR_MESSAGES.CODEX_BATCH_UNSUPPORTED
        )
      }
      if (data.key?.trim() && !isCodexCredential(data.key)) {
        addRequiredIssue(ctx, 'key', ERROR_MESSAGES.INVALID_CODEX_CREDENTIAL)
      }
    }

    if (
      data.type === 41 &&
      data.vertex_key_type === 'json' &&
      data.key?.trim() &&
      !isVertexJsonKey(data.key)
    ) {
      addRequiredIssue(ctx, 'key', ERROR_MESSAGES.INVALID_VERTEX_JSON_KEY)
    }

    if (
      data.type === 41 &&
      data.vertex_key_type === 'api_key' &&
      data.multi_key_mode &&
      data.multi_key_mode !== 'single'
    ) {
      addRequiredIssue(
        ctx,
        'multi_key_mode',
        ERROR_MESSAGES.VERTEX_API_KEY_BATCH_UNSUPPORTED
      )
    }

    if (
      data.credential_mode === 'global_account_pool' &&
      (!data.account_pool_group_id || data.account_pool_group_id <= 0)
    ) {
      // 账号池组模式的上游凭证全部来自被选中的账号池组；没有组 ID 时后端无法确定调度范围。
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['account_pool_group_id'],
        message: 'Account pool group is required',
      })
    }

    if (
      !isUpstreamAccountSync &&
      !usesGlobalAccountPool(data) &&
      (!data.group || data.group.length === 0)
    ) {
      addRequiredIssue(ctx, 'group', ERROR_MESSAGES.REQUIRED_GROUP)
    }
  })

export type ChannelFormValues = z.infer<typeof channelFormSchema>

// ============================================================================
// 表单默认值
// ============================================================================

export const CHANNEL_FORM_DEFAULT_VALUES: ChannelFormValues = {
  name: '',
  type: 1,
  base_url: '',
  key: '',
  openai_organization: '',
  models: '',
  group: [],
  model_mapping: '',
  priority: 0,
  weight: 0,
  test_model: '',
  auto_ban: 1,
  status: CHANNEL_STATUS.ENABLED,
  status_code_mapping: '',
  tag: '',
  remark: '',
  setting: '',
  param_override: '',
  header_override: '',
  settings: '{}',
  advanced_custom: '',
  other: '',
  multi_key_mode: 'single',
  multi_key_type: 'random',
  credential_mode: 'single_key',
  account_pool_mode: 'polling',
  account_pool_fallback: false,
  account_pool_group_id: 0,
  batch_add_set_key_prefix_2_name: false,
  key_mode: 'append',
  // 渠道扩展设置默认值。
  force_format: false,
  thinking_to_content: false,
  proxy: '',
  pass_through_body_enabled: false,
  system_prompt: '',
  system_prompt_override: false,
  // 类型专属设置默认值。
  is_enterprise_account: false,
  vertex_key_type: 'json',
  aws_key_type: 'ak_sk',
  azure_responses_version: '',
  // 字段透传控制默认值。
  allow_service_tier: false,
  disable_store: false,
  allow_safety_identifier: false,
  allow_include_obfuscation: false,
  allow_inference_geo: false,
  allow_speed: false,
  claude_beta_query: false,
  disable_task_polling_sleep: false,
  upstream_model_update_check_enabled: false,
  upstream_model_update_auto_sync_enabled: false,
  upstream_model_update_ignored_models: '',
  upstream_account_sync: false,
}

// ============================================================================
// 表单与 API 数据转换
// ============================================================================

/**
 * 将后端 Channel 转换为表单默认值。
 *
 * 后端不会回传完整 key，编辑模式下 key 字段必须保持空字符串；只有用户显式输入新 key 时，
 * 更新 payload 才会携带 key，避免不小心用空值覆盖已有密钥。
 */
export function transformChannelToFormDefaults(
  channel: Channel
): ChannelFormValues {
  // setting 是渠道通用扩展设置，解析失败时回落到安全默认值，避免坏数据阻塞编辑页面。
  let extraSettings = {
    force_format: false,
    thinking_to_content: false,
    proxy: '',
    pass_through_body_enabled: false,
    system_prompt: '',
    system_prompt_override: false,
  }

  if (channel.setting) {
    try {
      const parsed = JSON.parse(channel.setting)
      extraSettings = {
        force_format: parsed.force_format || false,
        thinking_to_content: parsed.thinking_to_content || false,
        proxy: parsed.proxy || '',
        pass_through_body_enabled: parsed.pass_through_body_enabled || false,
        system_prompt: parsed.system_prompt || '',
        system_prompt_override: parsed.system_prompt_override || false,
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to parse channel setting:', error)
    }
  }

  // settings 保存类型专属设置和字段透传开关，解析失败时只影响高级配置展示，不影响基础字段编辑。
  let vertexKeyType: 'json' | 'api_key' = 'json'
  let azureResponsesVersion = ''
  let isEnterpriseAccount = false
  let awsKeyType: 'ak_sk' | 'api_key' = 'ak_sk'
  let allowServiceTier = false
  let disableStore = false
  let allowSafetyIdentifier = false
  let allowIncludeObfuscation = false
  let allowInferenceGeo = false
  let allowSpeed = false
  let claudeBetaQuery = false
  let disableTaskPollingSleep = false
  let upstreamModelUpdateCheckEnabled = false
  let upstreamModelUpdateAutoSyncEnabled = false
  let upstreamModelUpdateIgnoredModels = ''
  let advancedCustom = ''

  if (channel.settings) {
    try {
      const parsed = JSON.parse(channel.settings)
      vertexKeyType = parsed.vertex_key_type || 'json'
      azureResponsesVersion = parsed.azure_responses_version || ''
      isEnterpriseAccount = parsed.openrouter_enterprise === true
      awsKeyType = parsed.aws_key_type || 'ak_sk'
      allowServiceTier = parsed.allow_service_tier === true
      disableStore = parsed.disable_store === true
      allowSafetyIdentifier = parsed.allow_safety_identifier === true
      allowIncludeObfuscation = parsed.allow_include_obfuscation === true
      allowInferenceGeo = parsed.allow_inference_geo === true
      allowSpeed = parsed.allow_speed === true
      claudeBetaQuery = parsed.claude_beta_query === true
      disableTaskPollingSleep = parsed.disable_task_polling_sleep === true
      upstreamModelUpdateCheckEnabled =
        parsed.upstream_model_update_check_enabled === true
      upstreamModelUpdateAutoSyncEnabled =
        parsed.upstream_model_update_auto_sync_enabled === true
      upstreamModelUpdateIgnoredModels = Array.isArray(
        parsed.upstream_model_update_ignored_models
      )
        ? parsed.upstream_model_update_ignored_models.join(',')
        : ''
      if (parsed.advanced_custom) {
        advancedCustom = stringifyAdvancedCustomConfig(parsed.advanced_custom)
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to parse channel settings:', error)
    }
  }

  let credentialMode: ChannelFormValues['credential_mode'] = 'single_key'
  if (channel.channel_info.credential_mode) {
    credentialMode = channel.channel_info.credential_mode
  } else if (channel.channel_info.account_pool_enabled) {
    credentialMode = 'account_pool'
  } else if (channel.channel_info.is_multi_key) {
    credentialMode = 'multi_key'
  }

  return {
    name: channel.name || '',
    type: channel.type,
    base_url: channel.base_url || '',
    key: '', // 安全要求：后端不会在普通详情接口回传完整 key，表单也不应自动填充。
    openai_organization: channel.openai_organization || '',
    models: channel.models || '',
    group: parseGroups(channel.group || 'default'),
    model_mapping: channel.model_mapping || '',
    priority: channel.priority || 0,
    weight: channel.weight || 0,
    test_model: channel.test_model || '',
    auto_ban: channel.auto_ban ?? 1,
    status: channel.status,
    status_code_mapping: channel.status_code_mapping || '',
    tag: channel.tag || '',
    remark: channel.remark || '',
    setting: channel.setting || '',
    param_override: channel.param_override || '',
    header_override: channel.header_override || '',
    settings: channel.settings || '{}',
    advanced_custom: advancedCustom,
    other: channel.other || '',
    multi_key_mode: 'single',
    multi_key_type: channel.channel_info.multi_key_mode || 'random',
    credential_mode: credentialMode,
    account_pool_mode: channel.channel_info.account_pool_mode || 'polling',
    account_pool_fallback: channel.channel_info.account_pool_fallback === true,
    account_pool_group_id: channel.channel_info.account_pool_group_id || 0,
    batch_add_set_key_prefix_2_name: false,
    key_mode: 'append', // 编辑多 Key 渠道时默认追加，降低误覆盖风险。
    // 渠道扩展设置。
    ...extraSettings,
    // 类型专属设置。
    is_enterprise_account: isEnterpriseAccount,
    vertex_key_type: vertexKeyType,
    azure_responses_version: azureResponsesVersion,
    aws_key_type: awsKeyType,
    allow_service_tier: allowServiceTier,
    disable_store: disableStore,
    allow_include_obfuscation: allowIncludeObfuscation,
    allow_inference_geo: allowInferenceGeo,
    allow_speed: allowSpeed,
    claude_beta_query: claudeBetaQuery,
    disable_task_polling_sleep: disableTaskPollingSleep,
    allow_safety_identifier: allowSafetyIdentifier,
    upstream_model_update_check_enabled: upstreamModelUpdateCheckEnabled,
    upstream_model_update_auto_sync_enabled: upstreamModelUpdateAutoSyncEnabled,
    upstream_model_update_ignored_models: upstreamModelUpdateIgnoredModels,
    upstream_account_sync: hasUpstreamAccountSyncMetadata(channel.settings),
  }
}

/**
 * 根据表单中的渠道扩展设置构造 setting JSON。
 */
function buildSettingJSON(formData: ChannelFormValues): string {
  const settingObj = {
    force_format: formData.force_format || false,
    thinking_to_content: formData.thinking_to_content || false,
    proxy: formData.proxy || '',
    pass_through_body_enabled: formData.pass_through_body_enabled || false,
    system_prompt: formData.system_prompt || '',
    system_prompt_override: formData.system_prompt_override || false,
  }
  return JSON.stringify(settingObj)
}

/**
 * 根据表单中的类型专属配置构造 settings JSON。
 *
 * 这里会先保留已有 settings 中未知字段，再按当前渠道类型写入或清理已知字段。
 * 这样做是为了兼容历史数据和后端后续新增字段，避免前端编辑一次就把未知配置抹掉。
 */
function buildSettingsJSON(formData: ChannelFormValues): string {
  let settingsObj: Record<string, unknown> = {}

  // 优先读取已有 settings，避免编辑渠道时丢失当前前端暂不识别的后端配置。
  if (formData.settings && formData.settings !== '{}') {
    try {
      settingsObj = JSON.parse(formData.settings)
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to parse existing settings:', error)
    }
  }

  // Vertex AI 渠道需要区分服务账号 JSON 和 API Key 两种凭证格式。
  if (formData.type === 41) {
    settingsObj.vertex_key_type = formData.vertex_key_type || 'json'
  } else if ('vertex_key_type' in settingsObj) {
    delete settingsObj.vertex_key_type
  }

  // Azure Responses API 可使用与默认 API version 不同的版本号。
  if (formData.type === 3 && formData.azure_responses_version) {
    settingsObj.azure_responses_version = formData.azure_responses_version
  } else if ('azure_responses_version' in settingsObj) {
    delete settingsObj.azure_responses_version
  }

  // OpenRouter 企业账户返回格式不同，需要在 relay 中做特殊处理。
  if (formData.type === 20) {
    settingsObj.openrouter_enterprise = formData.is_enterprise_account === true
  } else if ('openrouter_enterprise' in settingsObj) {
    delete settingsObj.openrouter_enterprise
  }

  // AWS 支持 AK/SK 和 API Key 两种接入方式。
  if (formData.type === 33) {
    settingsObj.aws_key_type = formData.aws_key_type || 'ak_sk'
  } else if ('aws_key_type' in settingsObj) {
    delete settingsObj.aws_key_type
  }

  // 字段透传开关必须按渠道类型保存，避免把仅某个 provider 支持的字段带到其他渠道。
  // Codex 走 OpenAI 兼容请求链路，需要与 OpenAI 一样持久化相关透传开关；
  // 否则编辑页能看到开关但保存后会丢失，形成 UI 与后端语义不一致。
  if (formData.type === 1 || formData.type === 14 || formData.type === 57) {
    settingsObj.allow_service_tier = formData.allow_service_tier === true
  } else if ('allow_service_tier' in settingsObj) {
    delete settingsObj.allow_service_tier
  }

  if (formData.type === 1 || formData.type === 57) {
    settingsObj.disable_store = formData.disable_store === true
    settingsObj.allow_safety_identifier =
      formData.allow_safety_identifier === true
    settingsObj.allow_include_obfuscation =
      formData.allow_include_obfuscation === true
    settingsObj.allow_inference_geo = formData.allow_inference_geo === true
  } else {
    if ('disable_store' in settingsObj) delete settingsObj.disable_store
    if ('allow_safety_identifier' in settingsObj)
      delete settingsObj.allow_safety_identifier
    if ('allow_include_obfuscation' in settingsObj)
      delete settingsObj.allow_include_obfuscation
    if (formData.type !== 14 && 'allow_inference_geo' in settingsObj)
      delete settingsObj.allow_inference_geo
  }

  // Anthropic 专属 beta query、推理地域和 speed 模式控制。
  if (formData.type === 14) {
    settingsObj.allow_inference_geo = formData.allow_inference_geo === true
    settingsObj.allow_speed = formData.allow_speed === true
    settingsObj.claude_beta_query = formData.claude_beta_query === true
  } else {
    if ('allow_speed' in settingsObj) delete settingsObj.allow_speed
    if ('claude_beta_query' in settingsObj) delete settingsObj.claude_beta_query
  }

  // 该开关由后端异步视频任务轮询读取；默认 false 仍保留 1 秒保护性等待。
  settingsObj.disable_task_polling_sleep =
    formData.disable_task_polling_sleep === true

  // 只有可拉取模型的渠道才保留上游模型更新配置，避免无效渠道被定时任务扫描。
  if (MODEL_FETCHABLE_TYPES.has(formData.type)) {
    settingsObj.upstream_model_update_check_enabled =
      formData.upstream_model_update_check_enabled === true
    settingsObj.upstream_model_update_auto_sync_enabled =
      settingsObj.upstream_model_update_check_enabled === true &&
      formData.upstream_model_update_auto_sync_enabled === true
    settingsObj.upstream_model_update_ignored_models = Array.from(
      new Set(
        String(formData.upstream_model_update_ignored_models || '')
          .split(',')
          .map((model) => model.trim())
          .filter(Boolean)
      )
    )
    if (
      !Array.isArray(settingsObj.upstream_model_update_last_detected_models) ||
      settingsObj.upstream_model_update_check_enabled !== true
    ) {
      settingsObj.upstream_model_update_last_detected_models = []
    }
    if (typeof settingsObj.upstream_model_update_last_check_time !== 'number') {
      settingsObj.upstream_model_update_last_check_time = 0
    }
  }

  // Advanced Custom 的 route/auth/converter 配置只允许保存在 type 58 渠道上。
  // 管理员把渠道切换回其它类型时主动清理，避免后端未来误消费历史高风险配置。
  if (formData.type === CHANNEL_TYPE_ADVANCED_CUSTOM) {
    const advancedCustomConfig = parseAdvancedCustomConfig(
      formData.advanced_custom
    )
    if (advancedCustomConfig) {
      settingsObj.advanced_custom = advancedCustomConfig
    }
  } else if ('advanced_custom' in settingsObj) {
    delete settingsObj.advanced_custom
  }

  return JSON.stringify(settingsObj)
}

function normalizeBaseUrl(value: string | undefined): string {
  return String(value || '')
    .trim()
    .replace(/\/+$/, '')
}

/**
 * 将创建表单转换为后端创建 payload。
 *
 * `global_account_pool` 是当前面向用户的“账号池”模式：渠道只保存模型、分组、优先级、
 * 计费和日志配置；真实上游 token 由账号池组内账号提供。因此创建时必须清空 base_url，
 * 并写入哨兵 key `global_account_pool`，用于兼容数据库 key 非空和旧路径判断。
 */
export function transformFormDataToCreatePayload(formData: ChannelFormValues): {
  mode: 'single' | 'batch' | 'multi_to_single'
  multi_key_mode?: 'random' | 'polling'
  batch_add_set_key_prefix_2_name?: boolean
  channel: Partial<Channel>
} {
  const credentialMode = formData.credential_mode || 'single_key'
  const mode =
    credentialMode === 'multi_key'
      ? 'multi_to_single'
      : credentialMode === 'account_pool' ||
          credentialMode === 'global_account_pool'
        ? 'single'
        : formData.multi_key_mode || 'single'

  const channel: Partial<Channel> = {
    name: formData.name,
    type: formData.type,
    base_url:
      credentialMode === 'global_account_pool'
        ? null
        : normalizeBaseUrl(formData.base_url) || null,
    key:
      credentialMode === 'global_account_pool'
        ? 'global_account_pool'
        : formData.key,
    openai_organization: formData.openai_organization || null,
    models: formData.models,
    group: formatGroups(formData.group || []),
    model_mapping: formData.model_mapping || null,
    priority: formData.priority || null,
    weight: formData.weight || null,
    test_model: formData.test_model || null,
    auto_ban: formData.auto_ban ?? 1,
    status: formData.status,
    status_code_mapping: formData.status_code_mapping || null,
    tag: formData.tag || null,
    remark: formData.remark || '',
    setting: buildSettingJSON(formData),
    param_override: formData.param_override || null,
    header_override: formData.header_override || null,
    settings: buildSettingsJSON(formData),
    other: formData.other || '',
    channel_info: {
      credential_mode: credentialMode,
      account_pool_enabled: credentialMode === 'account_pool',
      account_pool_mode: formData.account_pool_mode || 'polling',
      account_pool_fallback:
        credentialMode === 'global_account_pool'
          ? false
          : formData.account_pool_fallback === true,
      account_pool_group_id:
        credentialMode === 'global_account_pool'
          ? formData.account_pool_group_id || 0
          : 0,
      is_multi_key: credentialMode === 'multi_key',
      multi_key_size: 0,
      multi_key_polling_index: 0,
      multi_key_mode: formData.multi_key_type || 'random',
    },
  }

  // 将可选字段的空字符串归一为 null，减少后端保存无意义空值。
  Object.keys(channel).forEach((key) => {
    if (channel[key as keyof typeof channel] === '') {
      ;(channel as Record<string, unknown>)[key] = null
    }
  })

  return {
    mode,
    multi_key_mode:
      mode === 'multi_to_single' ? formData.multi_key_type : undefined,
    batch_add_set_key_prefix_2_name:
      mode === 'batch' ? formData.batch_add_set_key_prefix_2_name : undefined,
    channel,
  }
}

/**
 * 将编辑表单转换为后端更新 payload。
 *
 * 账号池组模式不依赖渠道 key/base_url，因此更新时强制写入哨兵 key 并清空 base_url。
 * 其他模式下，只有用户输入了新 key 才提交 key 字段，防止编辑保存时误清空已有密钥。
 *
 * 渠道启停状态已经迁移到专用操作接口，普通编辑 payload 不能携带 status。
 * 后端会直接拒绝带 status 的 `PUT /api/channel/` 请求，避免编辑保存顺手绕过
 * ChannelOperate 权限边界。
 */
export function transformFormDataToUpdatePayload(
  formData: ChannelFormValues,
  channelId: number
): Partial<Channel> {
  const credentialMode = formData.credential_mode || 'single_key'
  const payload: Partial<Channel> = {
    id: channelId,
    name: formData.name,
    type: formData.type,
    base_url:
      credentialMode === 'global_account_pool'
        ? null
        : normalizeBaseUrl(formData.base_url) || null,
    openai_organization: formData.openai_organization || null,
    models: formData.models,
    group: formatGroups(formData.group || []),
    model_mapping: formData.model_mapping || null,
    priority: formData.priority || null,
    weight: formData.weight || null,
    test_model: formData.test_model || null,
    auto_ban: formData.auto_ban ?? 1,
    status_code_mapping: formData.status_code_mapping || null,
    tag: formData.tag || null,
    remark: formData.remark || '',
    setting: buildSettingJSON(formData),
    param_override: formData.param_override || null,
    header_override: formData.header_override || null,
    settings: buildSettingsJSON(formData),
    other: formData.other || '',
  }

  // 只有账号池组模式或用户显式输入新 key 时才携带 key，避免空 key 覆盖旧凭证。
  if (credentialMode === 'global_account_pool') {
    payload.key = 'global_account_pool'
  } else if (formData.key && formData.key.trim()) {
    payload.key = formData.key
  }

  // 将可选字段的空字符串归一为 null，减少后端保存无意义空值。
  Object.keys(payload).forEach((key) => {
    if (payload[key as keyof typeof payload] === '') {
      ;(payload as Record<string, unknown>)[key] = null
    }
  })

  // 这些 nullable 文本字段需要允许用户清空；显式发送空字符串可让 GORM 更新旧值。
  // 这与 new-api 最新编辑页保持一致，也避免管理员在 UI 清空后后端仍保留旧值。
  payload.base_url =
    credentialMode === 'global_account_pool'
      ? ''
      : normalizeBaseUrl(formData.base_url) || ''
  payload.openai_organization = formData.openai_organization || ''
  payload.test_model = formData.test_model || ''
  payload.tag = formData.tag || ''
  payload.remark = formData.remark || ''
  payload.model_mapping = formData.model_mapping || ''
  payload.status_code_mapping = formData.status_code_mapping || ''
  payload.param_override = formData.param_override || ''
  payload.header_override = formData.header_override || ''

  return payload
}

// ============================================================================
// 校验与解析辅助函数
// ============================================================================

/**
 * 校验字符串是否为合法 JSON。
 */
export function validateJSON(value: string): boolean {
  if (!value || value.trim() === '') return true
  try {
    JSON.parse(value)
    return true
  } catch {
    return false
  }
}

/**
 * 校验模型映射格式。
 */
export function validateModelMapping(value: string): boolean {
  if (!value || value.trim() === '') return true
  return validateJSON(value)
}

/**
 * 将逗号分隔的模型字符串解析为数组。
 */
export function parseModels(models: string): string[] {
  if (!models) return []
  return models
    .split(',')
    .map((m) => m.trim())
    .filter((m) => m.length > 0)
}

/**
 * Parse groups string to array
 */
export function parseGroups(groups: string): string[] {
  if (!groups) return []
  return groups
    .split(',')
    .map((g) => g.trim())
    .filter((g) => g.length > 0)
}

/**
 * Format models array to string
 */
export function formatModels(models: string[]): string {
  return models.join(',')
}

/**
 * Format groups array to string
 */
export function formatGroups(groups: string[]): string {
  return groups.join(',')
}
