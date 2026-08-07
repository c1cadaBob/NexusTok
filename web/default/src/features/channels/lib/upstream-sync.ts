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
import { CHANNEL_STATUS } from '../constants'
import type {
  ChannelAccount,
  UpstreamAccountKey,
  UpstreamAccountPreviewData,
  UpstreamAccountPreviewRequest,
  UpstreamAccountRefreshRequest,
  UpstreamAccountRatioConversion,
  UpstreamAccountPlatform,
  UpstreamAccountAuthMode,
  UpstreamAccountTwoFactorChallenge,
} from '../types'
import { formatGroups, parseGroups } from './channel-form'
import { dedupeModelNames } from './model-search'
import { parseModelsString } from './model-mapping-validation'

const PREVIEW_EXPIRED_ERROR_TEXT = '预览快照不存在或已过期'
const UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY = 'upstream_account_sync'
const CHANNEL_TYPE_NEW_API = 59
const CHANNEL_TYPE_SUB2API = 60
export const DEFAULT_UPSTREAM_PAID_AMOUNT = '1'
export const DEFAULT_UPSTREAM_PLATFORM_CREDIT = '10'

type RatioDisplaySource = {
  ratio_conversion?: number | null
  effective_ratio?: number | null
  group_ratio?: number | null
}

export type UpstreamAccountConfigDraft = {
  priority: number
  weight: number
  enabled: boolean
  models?: string
  group?: string
  access_groups?: string
}

export type UpstreamAccountCapabilitySummary = {
  enabledKeyCount: number
  totalKeyCount: number
  modelNames: string[]
  accessGroups: string[]
  modelCount: number
  accessGroupText: string
}

export type BuildUpstreamAccountPreviewRequestOptions = {
  channelId?: number | null
  platform: UpstreamAccountPlatform
  baseUrl: string
  username?: string
  password?: string
  authMode?: UpstreamAccountAuthMode
  captureId?: string
  sessionCookie?: string
  userId?: string
  accessToken?: string
  refreshToken?: string
  expiresAt?: number
  useSavedCredential: boolean
  ratioConversion?: UpstreamAccountRatioConversion
}

export type BuildUpstreamAccountRefreshPayloadOptions = {
  previewId: string
  keys: UpstreamAccountKey[]
  configs: Record<string, UpstreamAccountConfigDraft>
  applySuggested: boolean
  ratioConversion?: UpstreamAccountRatioConversion
  disableMissingKey?: boolean
}

export function parseSettingsRecord(
  settings: string | undefined
): Record<string, unknown> {
  if (!settings?.trim()) return {}
  try {
    const parsed = JSON.parse(settings)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
  } catch {
    return {}
  }
  return {}
}

export function upstreamPlatformFromChannelType(
  channelType: number
): UpstreamAccountPlatform | null {
  if (channelType === CHANNEL_TYPE_NEW_API) return 'new-api'
  if (channelType === CHANNEL_TYPE_SUB2API) return 'sub2api'
  return null
}

export function channelTypeFromUpstreamPlatform(platform: string | undefined | null) {
  const normalized = String(platform || '')
    .trim()
    .toLowerCase()
    .replaceAll('_', '-')
  if (normalized === 'new-api' || normalized === 'newapi') {
    return CHANNEL_TYPE_NEW_API
  }
  if (normalized === 'sub2api' || normalized === 'sub2-api') {
    return CHANNEL_TYPE_SUB2API
  }
  return undefined
}

export function defaultUpstreamChannelName(baseUrl: string, fallback = '') {
  const trimmedBaseUrl = baseUrl.trim()
  const fallbackName = fallback.trim()
  if (!trimmedBaseUrl) return fallbackName

  try {
    const withProtocol = /^[a-z][a-z\d+\-.]*:\/\//i.test(trimmedBaseUrl)
      ? trimmedBaseUrl
      : `https://${trimmedBaseUrl}`
    const host = new URL(withProtocol).hostname.replace(/^www\./i, '')
    if (!host) return fallbackName
    if (
      host === 'localhost' ||
      /^\d{1,3}(?:\.\d{1,3}){3}$/.test(host) ||
      host.includes(':')
    ) {
      return host
    }
    return host.split('.').filter(Boolean)[0] || fallbackName
  } catch {
    return fallbackName
  }
}

function parsePositiveNumber(value: string): number | undefined {
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed <= 0) return undefined
  return parsed
}

export function buildUpstreamRatioConversionPayload(
  paidCny: string,
  platformUsdCredit: string
): UpstreamAccountRatioConversion | undefined {
  const normalizedPaidCny = parsePositiveNumber(paidCny)
  const normalizedPlatformUsdCredit = parsePositiveNumber(platformUsdCredit)
  if (!normalizedPaidCny || !normalizedPlatformUsdCredit) {
    return undefined
  }
  return {
    paid_cny: normalizedPaidCny,
    platform_usd_credit: normalizedPlatformUsdCredit,
  }
}

// 构建上游预览请求。使用已保存登录时只传 channel_id 和站点信息，
// 避免把空账号/密码发给后端触发“密码不能为空”。
export function buildUpstreamAccountPreviewRequest({
  channelId,
  platform,
  baseUrl,
  username = '',
  password = '',
  authMode = 'password',
  captureId = '',
  sessionCookie = '',
  userId = '',
  accessToken = '',
  refreshToken = '',
  expiresAt,
  useSavedCredential,
  ratioConversion,
}: BuildUpstreamAccountPreviewRequestOptions): UpstreamAccountPreviewRequest {
  const payload: UpstreamAccountPreviewRequest = {
    platform,
    base_url: baseUrl.trim(),
  }

  if (channelId && useSavedCredential) {
    payload.channel_id = channelId
  }

  if (!useSavedCredential) {
    payload.auth_mode = authMode
    if (authMode === 'oauth_browser') {
      payload.capture_id = captureId.trim()
    } else if (authMode === 'session_cookie') {
      payload.session_cookie = sessionCookie
      payload.user_id = userId.trim() || undefined
    } else if (authMode === 'access_token') {
      payload.access_token = accessToken.trim()
      payload.refresh_token = refreshToken.trim() || undefined
      payload.expires_at = expiresAt
      if (platform === 'new-api') {
        payload.user_id = userId.trim() || undefined
      }
    } else if (platform === 'new-api') {
      payload.username = username.trim()
      payload.password = password
    } else if (platform === 'sub2api') {
      payload.email = username.trim()
      payload.password = password
    }
  }

  if (ratioConversion) {
    payload.ratio_conversion = ratioConversion
  }

  return payload
}

export function resolveUpstreamChannelGroup(groups: string[] | undefined) {
  return formatGroups(groups || []) || 'default'
}

export function formatUpstreamRatioCompact(value: number | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  if (Number.isInteger(value)) return value.toString()
  return value.toFixed(8).replace(/\.?0+$/, '')
}

export function getUpstreamRatioDisplayValue(
  value: RatioDisplaySource | null | undefined
): number | undefined {
  if (!value) return undefined
  const candidates = [
    value.ratio_conversion,
    value.effective_ratio,
    value.group_ratio,
  ]
  return candidates.find(
    (candidate) => candidate != null && Number.isFinite(candidate)
  ) as number | undefined
}

export function getUpstreamKeyRatioDisplayValue(
  value:
    | Pick<RatioDisplaySource, 'effective_ratio' | 'group_ratio'>
    | null
    | undefined
): number | undefined {
  if (!value) return undefined
  const candidates = [value.effective_ratio, value.group_ratio]
  return candidates.find(
    (candidate) => candidate != null && Number.isFinite(candidate)
  ) as number | undefined
}

export function formatUpstreamModelRatioDetails(
  ratios?: Record<string, number>
): string {
  if (!ratios) return ''
  return Object.entries(ratios)
    .filter(([modelName, ratio]) => modelName.trim() && Number.isFinite(ratio))
    .sort(([left], [right]) => left.localeCompare(right))
    .map(
      ([modelName, ratio]) =>
        `${modelName}: ${formatUpstreamRatioCompact(ratio)}x`
    )
    .join('\n')
}

export function getUpstreamKeyGroupLabel(
  value: Pick<
    ChannelAccount,
    'key_group_name' | 'key_group_id' | 'group'
  > | Pick<UpstreamAccountKey, 'group_name' | 'group_id'>
): string {
  if ('group' in value) {
    return value.key_group_name || value.key_group_id || value.group || ''
  }
  return value.group_name || value.group_id || ''
}

export function getUpstreamSyncBaseUrlFromSettings(
  settings: string | undefined
): string {
  const metadata =
    parseSettingsRecord(settings)[UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY]
  if (!metadata || typeof metadata !== 'object') return ''
  const record = metadata as Record<string, unknown>
  return String(record.management_base_url || record.base_url || '').trim()
}

export function getUpstreamSyncPlatformFromSettings(
  settings: string | undefined
): UpstreamAccountPlatform | '' {
  const metadata =
    parseSettingsRecord(settings)[UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY]
  if (!metadata || typeof metadata !== 'object') return ''
  const platform = String((metadata as Record<string, unknown>).platform || '')
    .trim()
    .toLowerCase()
    .replaceAll('_', '-')
  if (platform === 'new-api' || platform === 'newapi') return 'new-api'
  if (platform === 'sub2api' || platform === 'sub2-api') return 'sub2api'
  return ''
}

export function hasUpstreamSyncSavedCredential(
  settings: string | undefined
): boolean {
  const metadata =
    parseSettingsRecord(settings)[UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY]
  if (!metadata || typeof metadata !== 'object') return false
  const record = metadata as Record<string, unknown>
  return Boolean(record.credential_saved || record.credentials)
}

export function getUpstreamSyncCredentialAuthModeFromSettings(
  settings: string | undefined
): UpstreamAccountAuthMode | '' {
  const metadata =
    parseSettingsRecord(settings)[UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY]
  if (!metadata || typeof metadata !== 'object') return ''
  const record = metadata as Record<string, unknown>
  const mode = String(
    record.credential_auth_mode || record.auth_mode || ''
  )
    .trim()
    .toLowerCase()
    .replaceAll('_', '-')
  if (mode === 'password') return 'password'
  if (mode === 'session-cookie') return 'session_cookie'
  if (mode === 'access-token') return 'access_token'
  if (mode === 'oauth-browser') return 'oauth_browser'
  return ''
}

export function normalizeUpstreamChannelBaseUrl(value?: string | null) {
  return String(value || '')
    .trim()
    .replace(/\/+$/, '')
}

export function upstreamPreviewRemainingSeconds(
  expiresAt: number,
  nowMs: number
) {
  if (!expiresAt) return 0
  return Math.max(0, Math.ceil(expiresAt - nowMs / 1000))
}

export function hasUpstreamPreviewSnapshot(
  data: unknown
): data is UpstreamAccountPreviewData {
  return Boolean(
    data &&
    typeof data === 'object' &&
    'preview_id' in data &&
    'snapshot' in data
  )
}

export function getUpstreamPreviewChallenge(
  data: unknown
): UpstreamAccountTwoFactorChallenge | null {
  if (!data || typeof data !== 'object' || !('challenge' in data)) return null
  const challenge = (data as { challenge?: unknown }).challenge
  if (!challenge || typeof challenge !== 'object') return null
  if (!('challenge_id' in challenge) || !('expires_at' in challenge))
    return null
  return challenge as UpstreamAccountTwoFactorChallenge
}

export function formatUpstreamPreviewRemaining(seconds: number) {
  const safeSeconds = Math.max(0, seconds)
  const minutes = Math.floor(safeSeconds / 60)
  const remainingSeconds = safeSeconds % 60
  return `${minutes}:${remainingSeconds.toString().padStart(2, '0')}`
}

export function isUpstreamPreviewExpiredError(message?: string) {
  return Boolean(message?.includes(PREVIEW_EXPIRED_ERROR_TEXT))
}

function upstreamAccountModelsValue(
  key: UpstreamAccountKey,
  config: UpstreamAccountConfigDraft | undefined,
  fallbackModels = ''
) {
  if (config && Object.prototype.hasOwnProperty.call(config, 'models')) {
    return config.models?.trim() || ''
  }
  return key.models?.join(',') || fallbackModels.trim() || ''
}

export function upstreamAccountModelsArrayValue(
  key: UpstreamAccountKey,
  config: UpstreamAccountConfigDraft | undefined,
  fallbackModels = ''
) {
  return parseModelsString(
    upstreamAccountModelsValue(key, config, fallbackModels)
  )
}

export function buildUpstreamAccountModelOptions(
  key: UpstreamAccountKey,
  config: UpstreamAccountConfigDraft | undefined,
  candidateModels: readonly string[],
  fallbackModels = ''
) {
  return dedupeModelNames([
    ...upstreamAccountModelsArrayValue(key, config, fallbackModels),
    ...(key.models ?? []),
    ...candidateModels,
  ]).map((model) => ({
    value: model,
    label: model,
  }))
}

function upstreamAccountGroupValue(
  key: UpstreamAccountKey,
  config: UpstreamAccountConfigDraft | undefined,
  fallbackGroup = ''
) {
  if (config && Object.prototype.hasOwnProperty.call(config, 'group')) {
    return config.group?.trim() || ''
  }
  return key.group_name || key.group_id || fallbackGroup.trim() || ''
}

function upstreamAccountPriorityValue(
  config: UpstreamAccountConfigDraft | undefined,
  applySuggested: boolean
) {
  return applySuggested ? undefined : config?.priority
}

function upstreamAccountWeightValue(
  config: UpstreamAccountConfigDraft | undefined,
  applySuggested: boolean
) {
  return applySuggested ? undefined : config?.weight
}

export function upstreamAccountKeyConfigId(key: UpstreamAccountKey, index: number) {
  return key.sync_id || key.external_id || key.masked_key || `${index}`
}

export function getUpstreamAccountConfig(
  configs: Record<string, UpstreamAccountConfigDraft>,
  key: UpstreamAccountKey,
  index: number
) {
  const candidates = [
    upstreamAccountKeyConfigId(key, index),
    key.sync_id,
    key.external_id,
    key.masked_key,
  ]
  for (const candidate of candidates) {
    if (candidate && configs[candidate]) {
      return configs[candidate]
    }
  }
  return undefined
}

export function buildUpstreamAccountConfigsFromSnapshotKeys(
  keys: UpstreamAccountKey[],
  previousConfigs: Record<string, UpstreamAccountConfigDraft> = {}
): Record<string, UpstreamAccountConfigDraft> {
  const configs: Record<string, UpstreamAccountConfigDraft> = {}
  keys.forEach((key, index) => {
    const configId = upstreamAccountKeyConfigId(key, index)
    const previousConfig = getUpstreamAccountConfig(previousConfigs, key, index)
    configs[configId] = {
      priority: previousConfig?.priority ?? key.suggested_priority,
      weight: previousConfig?.weight ?? key.suggested_weight,
      enabled: previousConfig?.enabled ?? true,
      models: previousConfig?.models ?? key.models?.join(',') ?? '',
      group: previousConfig?.group ?? key.group_name ?? key.group_id ?? '',
      access_groups: previousConfig?.access_groups ?? key.access_groups ?? 'default',
    }
  })
  return configs
}

export function buildUpstreamAccountConfigsFromChannelAccounts(
  accounts: ChannelAccount[]
): Record<string, UpstreamAccountConfigDraft> {
  const configs: Record<string, UpstreamAccountConfigDraft> = {}
  accounts.forEach((account) => {
    const key = upstreamAccountFromChannelAccount(account)
    configs[upstreamAccountKeyConfigId(key, account.id)] = {
      priority: account.priority || 0,
      weight: account.weight || 0,
      enabled: account.status === CHANNEL_STATUS.ENABLED,
      models: account.models || '',
      group: account.group || '',
      access_groups: account.access_groups ?? 'default',
    }
  })
  return configs
}

export function upstreamModelsToString(keys: UpstreamAccountKey[]) {
  const seen = new Set<string>()
  const models: string[] = []
  keys.forEach((key) => {
    key.models?.forEach((modelName) => {
      const trimmed = modelName.trim()
      if (!trimmed || seen.has(trimmed)) return
      seen.add(trimmed)
      models.push(trimmed)
    })
  })
  return models.join(',')
}

export function upstreamAccountValuesToString(
  keys: UpstreamAccountKey[],
  configs: Record<string, UpstreamAccountConfigDraft>,
  getValue: (
    key: UpstreamAccountKey,
    config: UpstreamAccountConfigDraft | undefined
  ) => string
) {
  const seen = new Set<string>()
  const values: string[] = []
  keys.forEach((key, index) => {
    const config = getUpstreamAccountConfig(configs, key, index)
    if (config?.enabled === false) return
    getValue(key, config)
      .split(',')
      .map((value) => value.trim())
      .filter(Boolean)
      .forEach((value) => {
        if (seen.has(value)) return
        seen.add(value)
        values.push(value)
      })
  })
  return values.join(',')
}

// summarizeUpstreamAccountCapabilities 只按启用的同步密钥计算前端摘要。
// 上游同步渠道的真实能力由每个密钥的 models 与 access_groups 决定；
// 渠道级 models/group 只是后端聚合缓存和旧接口兼容字段，不能再作为兜底配置展示。
export function summarizeUpstreamAccountCapabilities(
  keys: UpstreamAccountKey[],
  configs: Record<string, UpstreamAccountConfigDraft> = {}
): UpstreamAccountCapabilitySummary {
  const modelNames: string[] = []
  const accessGroups: string[] = []
  let enabledKeyCount = 0

  keys.forEach((key, index) => {
    const config = getUpstreamAccountConfig(configs, key, index)
    if (config?.enabled === false) return

    enabledKeyCount += 1
    modelNames.push(...upstreamAccountModelsArrayValue(key, config))
    accessGroups.push(
      ...parseGroups(config?.access_groups ?? key.access_groups ?? 'default')
    )
  })

  const dedupedModels = dedupeModelNames(modelNames)
  const dedupedGroups = dedupeModelNames(accessGroups)

  return {
    enabledKeyCount,
    totalKeyCount: keys.length,
    modelNames: dedupedModels,
    accessGroups: dedupedGroups,
    modelCount: dedupedModels.length,
    accessGroupText: formatGroups(dedupedGroups) || '-',
  }
}

export function buildUpstreamAccountPayloads(
  keys: UpstreamAccountKey[],
  configs: Record<string, UpstreamAccountConfigDraft>,
  applySuggested: boolean
) {
  return keys.map((key, index) => {
    const config = getUpstreamAccountConfig(configs, key, index)
    return {
      sync_id: key.sync_id,
      external_id: key.external_id,
      name: key.name || key.masked_key,
      enabled: config?.enabled ?? true,
      models: upstreamAccountModelsValue(key, config),
      group: upstreamAccountGroupValue(key, config),
      access_groups: config?.access_groups ?? key.access_groups ?? 'default',
      priority: upstreamAccountPriorityValue(config, applySuggested),
      weight: upstreamAccountWeightValue(config, applySuggested),
    }
  })
}

// 应用刷新时的 payload 需要和预览快照一致，避免前端分散组装导致字段漂移。
export function buildUpstreamAccountRefreshPayload({
  previewId,
  keys,
  configs,
  applySuggested,
  ratioConversion,
  disableMissingKey = true,
}: BuildUpstreamAccountRefreshPayloadOptions): UpstreamAccountRefreshRequest {
  const payload: UpstreamAccountRefreshRequest = {
    preview_id: previewId,
    apply_suggested: applySuggested,
    disable_missing_key: disableMissingKey,
    accounts: buildUpstreamAccountPayloads(keys, configs, applySuggested),
  }

  if (ratioConversion) {
    payload.ratio_conversion = ratioConversion
  }

  return payload
}

export function upstreamAccountFromChannelAccount(
  account: ChannelAccount
): UpstreamAccountKey {
  const syncMetadata = parseSettingsRecord(account.settings)[
    UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY
  ]
  const upstreamExternalId =
    syncMetadata && typeof syncMetadata === 'object'
      ? String(
          (syncMetadata as Record<string, unknown>).external_id || ''
        ).trim()
      : ''
  const upstreamConfigId =
    upstreamExternalId || account.key || String(account.id)

  return {
    sync_id: upstreamConfigId,
    external_id: upstreamConfigId,
    name: account.name || `#${account.id}`,
    masked_key: account.key,
    status: account.status,
    group_name: account.key_group_name || account.group,
    group_id: account.key_group_id || account.group,
    access_groups: account.access_groups,
    models: parseModelsString(account.models || ''),
    model_ratios: account.model_ratios,
    group_ratio: account.group_ratio ?? undefined,
    effective_ratio: account.effective_ratio,
    ratio_conversion: account.ratio_conversion,
    suggested_priority: account.priority || 0,
    suggested_weight: account.weight || 0,
  }
}
