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
import type { StatusBadgeProps } from '@/components/status-badge'
import {
  BILLING_PRICING_VARS,
  normalizeTierLabel,
  parseTiersFromExpr,
  type ParsedTier,
} from '@/features/pricing/lib/billing-expr'
import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'

export { normalizeTierLabel }

export type StreamSeverity = 'ok' | 'warning' | 'error'

/**
 * 解析流状态严重级别，兼容新旧消费日志。
 *
 * 新日志优先使用后端写入的 severity；旧日志没有该字段时，仅将明确的
 * client_gone 推断为 warning，其余仍沿用 status 的 ok/error 语义。
 */
export function getStreamSeverity(
  streamStatus: LogOtherData['stream_status'] | null | undefined
): StreamSeverity {
  if (!streamStatus) return 'ok'
  if (
    streamStatus.severity === 'ok' ||
    streamStatus.severity === 'warning' ||
    streamStatus.severity === 'error'
  ) {
    return streamStatus.severity
  }
  if (streamStatus.end_reason === 'client_gone') return 'warning'
  return streamStatus.status === 'ok' ? 'ok' : 'error'
}

const PARAM_OVERRIDE_ACTION_MAP: Record<string, string> = {
  set: 'Set',
  delete: 'Delete',
  copy: 'Copy',
  move: 'Move',
  append: 'Append',
  prepend: 'Prepend',
  trim_prefix: 'Trim Prefix',
  trim_suffix: 'Trim Suffix',
  ensure_prefix: 'Ensure Prefix',
  ensure_suffix: 'Ensure Suffix',
  trim_space: 'Trim Space',
  to_lower: 'To Lower',
  to_upper: 'To Upper',
  replace: 'Replace',
  regex_replace: 'Regex Replace',
  set_header: 'Set Header',
  delete_header: 'Delete Header',
  copy_header: 'Copy Header',
  move_header: 'Move Header',
  pass_headers: 'Pass Headers',
  sync_fields: 'Sync Fields',
  return_error: 'Return Error',
}

/**
 * Get localized label for a param override action
 */
export function getParamOverrideActionLabel(
  action: string,
  t: (key: string) => string
): string {
  const key = PARAM_OVERRIDE_ACTION_MAP[action.toLowerCase()]
  return key ? t(key) : action
}

/**
 * Parse a param override audit line into action and content
 */
export function parseAuditLine(
  line: string
): { action: string; content: string } | null {
  if (typeof line !== 'string') return null
  const firstSpace = line.indexOf(' ')
  if (firstSpace <= 0) return { action: line, content: line }
  return {
    action: line.slice(0, firstSpace),
    content: line.slice(firstSpace + 1),
  }
}

/**
 * Check if the log is a violation fee log
 */
export function isViolationFeeLog(other: LogOtherData | null): boolean {
  if (!other) return false
  return (
    other.violation_fee === true ||
    Boolean(other.violation_fee_code) ||
    Boolean(other.violation_fee_marker)
  )
}

/**
 * Parse the 'other' field from JSON string to object
 */
export function parseLogOther(other: string): LogOtherData | null {
  if (!other) return null
  try {
    return JSON.parse(other) as LogOtherData
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to parse log other field:', error)
    return null
  }
}

/**
 * Get time color based on duration (in seconds)
 */
export function getTimeColor(
  seconds: number
): 'success' | 'warning' | 'danger' {
  if (seconds < 10) return 'success'
  if (seconds < 30) return 'warning'
  return 'danger'
}

/**
 * Get first-response-token color based on latency (in seconds)
 */
export function getFirstResponseTimeColor(
  seconds: number
): 'success' | 'warning' | 'danger' {
  if (seconds < 5) return 'success'
  if (seconds < 10) return 'warning'
  return 'danger'
}

/**
 * Get throughput color based on generated tokens per second
 */
export function getThroughputColor(
  tokensPerSecond: number
): 'success' | 'warning' | 'danger' {
  if (tokensPerSecond >= 30) return 'success'
  if (tokensPerSecond >= 15) return 'warning'
  return 'danger'
}

/**
 * Get response color using throughput only when enough output tokens exist.
 */
export function getResponseTimeColor(
  seconds: number,
  completionTokens: number
): 'success' | 'warning' | 'danger' {
  if (completionTokens < 100 || seconds <= 0) return getTimeColor(seconds)
  return getThroughputColor(completionTokens / seconds)
}

/**
 * Format model name with mapping indicator
 */
export function formatModelName(log: UsageLog): {
  name: string
  isMapped: boolean
  actualModel?: string
} {
  const other = parseLogOther(log.other)
  const isMapped = !!(
    other?.is_model_mapped &&
    other?.upstream_model_name &&
    other.upstream_model_name !== ''
  )

  return {
    name: log.model_name,
    isMapped,
    actualModel: isMapped ? other.upstream_model_name : undefined,
  }
}

/**
 * Decode a base64-encoded billing expression. Safely returns an empty string
 * when the input is missing or malformed (e.g. legacy logs without expr_b64).
 */
export function decodeBillingExprB64(exprB64: string | undefined): string {
  if (!exprB64) return ''
  try {
    const binaryString =
      typeof window !== 'undefined'
        ? window.atob(exprB64)
        : Buffer.from(exprB64, 'base64').toString('binary')
    const bytes = new Uint8Array(binaryString.length)

    for (let i = 0; i < binaryString.length; i++) {
      bytes[i] = binaryString.charCodeAt(i)
    }

    if (typeof TextDecoder !== 'undefined') {
      return new TextDecoder().decode(bytes)
    }

    return decodeURIComponent(
      Array.prototype.map
        .call(bytes, (byte: number) => '%' + byte.toString(16).padStart(2, '0'))
        .join('')
    )
  } catch {
    return ''
  }
}

/**
 * Resolve which parsed tier corresponds to the matched_tier label in a log
 * entry. Missing or unknown labels do not fall back to another tier because
 * that would display guessed unit prices.
 */
export function resolveMatchedTier(
  tiers: ParsedTier[],
  matchedLabel: string | undefined
): ParsedTier | null {
  if (tiers.length === 0) return null
  if (!matchedLabel) return null
  const found = tiers.find((tier) => {
    const l1 = normalizeTierLabel(tier.label)
    const l2 = normalizeTierLabel(matchedLabel)
    return l1 === l2 && l1 !== ''
  })
  return found || null
}

/**
 * Tiered pricing summary derived from an `other` log payload using the
 * billing-expression library. Returns null when the entry is not a tiered
 * billing log or the expression failed to parse.
 */
export interface TieredBillingSummary {
  tiers: ParsedTier[]
  tier: ParsedTier
  priceEntries: Array<{ field: string; shortLabel: string; price: number }>
}

/**
 * Whether the request payload reports any cache-related token usage. Used to
 * suppress cache pricing rows from the tiered breakdown when the request did
 * not exercise the cache path (mirrors the classic frontend behaviour).
 */
export function hasAnyCacheTokens(
  other: LogOtherData | null | undefined
): boolean {
  if (!other) return false
  return (
    (other.cache_tokens || 0) > 0 ||
    (other.cache_creation_tokens || 0) > 0 ||
    (other.cache_creation_tokens_5m || 0) > 0 ||
    (other.cache_creation_tokens_1h || 0) > 0
  )
}

export function getTieredBillingSummary(
  other: LogOtherData | null
): TieredBillingSummary | null {
  if (!other || other.billing_mode !== 'tiered_expr') return null
  const exprStr = decodeBillingExprB64(other.expr_b64)
  if (!exprStr) return null
  const tiers = parseTiersFromExpr(exprStr)
  const tier = resolveMatchedTier(tiers, other.matched_tier)
  if (!tier) return null

  const cacheTokensPresent = hasAnyCacheTokens(other)

  const priceEntries: TieredBillingSummary['priceEntries'] = []
  for (const v of BILLING_PRICING_VARS) {
    if (!v.field) continue
    if (v.group === 'cache' && !cacheTokensPresent) continue
    const raw = tier[v.field as keyof ParsedTier]
    const price = Number(raw)
    if (Number.isFinite(price) && price > 0) {
      priceEntries.push({
        field: v.field,
        shortLabel: v.shortLabel,
        price,
      })
    }
  }
  return { tiers, tier, priceEntries }
}

/**
 * Calculate duration and return formatted result with color variant
 * @param submitTime - Submit timestamp
 * @param finishTime - Finish timestamp
 * @param unit - Unit of the timestamps ('seconds' or 'milliseconds')
 */
export function formatDuration(
  submitTime?: number,
  finishTime?: number,
  unit: 'seconds' | 'milliseconds' = 'milliseconds'
): { durationSec: number; variant: StatusBadgeProps['variant'] } | null {
  if (!submitTime || !finishTime) return null

  const durationSec =
    unit === 'milliseconds'
      ? (finishTime - submitTime) / 1000
      : finishTime - submitTime

  return { durationSec, variant: durationSec > 60 ? 'red' : 'green' }
}

type AuditTranslateFn = (
  key: string,
  opts?: Record<string, unknown>
) => string

// 审计 action 是后端写入的语言无关标识。这里吸收 new-api 的结构化摘要模板，
// 并补入 NexusTok 自身的权限治理、订阅和管理操作，列表与详情共用同一渲染规则。
const AUDIT_TEMPLATES: Record<string, string> = {
  login: 'Logged in successfully via {{method}}',

  'user.create': 'Created user {{username}}',
  'user.update': 'Updated user {{username}} (ID: {{id}})',
  'user.delete': 'Deleted user {{username}} (ID: {{id}})',
  'user.manage': 'Managed user {{username}} (ID: {{id}})',
  'user.quota_add': 'Increased user quota by {{quota}}',
  'user.quota_subtract': 'Decreased user quota by {{quota}}',
  'user.quota_override': 'Overrode user quota from {{from}} to {{to}}',
  'user.binding_clear': 'Cleared {{bindingType}} binding for user {{username}}',
  'user.2fa_disable': 'Force-disabled two-factor authentication for the user',
  'user.passkey_register': 'Registered a passkey',
  'user.passkey_delete': 'Deleted a passkey',
  'user.topup_complete': 'Completed top-up order for the user',
  'user.reset_passkey': 'Reset the user passkey',
  'user.oauth_unbind': 'Removed an OAuth binding for the user',

  'option.update': 'Updated system setting {{key}}',
  'option.payment_compliance': 'Confirmed payment compliance',
  'option.reset_ratio': 'Reset model ratios',
  'option.reset_model_ratio': 'Reset model ratios',
  'option.clear_affinity_cache': 'Cleared channel affinity cache',
  'option.migrate_console_setting': 'Migrated console settings',

  'custom_oauth.discovery': 'Discovered a custom OAuth provider',
  'custom_oauth.create': 'Created a custom OAuth provider',
  'custom_oauth.update': 'Updated a custom OAuth provider',
  'custom_oauth.delete': 'Deleted a custom OAuth provider',

  'performance.reset_stats': 'Reset performance statistics',
  'performance.clear_disk_cache': 'Cleared disk cache',
  'performance.gc': 'Triggered garbage collection',
  'performance.clear_logs': 'Cleared log files',

  'channel.create': 'Created channel {{name}}',
  'channel.update': 'Updated channel {{name}} (ID: {{id}})',
  'channel.delete': 'Deleted channel {{name}} (ID: {{id}})',
  'channel.delete_batch': 'Batch deleted {{count}} channels',
  'channel.delete_disabled': 'Deleted all disabled channels ({{count}})',
  'channel.key_view': 'Viewed channel key {{name}} (ID: {{id}})',
  'channel.tag_disable': 'Disabled channels with tag {{tag}}',
  'channel.tag_enable': 'Enabled channels with tag {{tag}}',
  'channel.tag_edit': 'Edited channels with tag {{tag}}',
  'channel.tag_batch_set': 'Batch set tag for {{count}} channels',
  'channel.copy':
    'Copied channel {{sourceId}} to {{name}} (ID: {{id}})',
  'channel.upstream_account_sync_refresh':
    'Refreshed upstream account sync for channel {{name}} (ID: {{id}})',
  'channel.multi_key_manage':
    'Multi-key management {{action}} on channel (ID: {{id}})',
  'channel.upstream_apply':
    'Applied upstream model changes to channel (ID: {{id}})',
  'channel.upstream_apply_all':
    'Applied upstream model changes to {{count}} channels',
  'channel.upstream_detect': 'Detected upstream model changes',
  'channel.upstream_detect_all': 'Detected upstream model changes for all channels',
  'channel.fix': 'Fixed channel metadata',
  'channel.fetch_models': 'Fetched channel models',
  'channel.codex_oauth_start': 'Started Codex OAuth for channels',
  'channel.codex_oauth_complete': 'Completed Codex OAuth for channels',
  'channel.codex_oauth_start_for_channel':
    'Started Codex OAuth for channel (ID: {{id}})',
  'channel.codex_oauth_complete_for_channel':
    'Completed Codex OAuth for channel (ID: {{id}})',
  'channel.codex_refresh': 'Refreshed Codex credentials for channel (ID: {{id}})',
  'channel.codex_usage_reset': 'Reset Codex usage for channel (ID: {{id}})',
  'channel.ollama_pull': 'Pulled an Ollama model',
  'channel.ollama_pull_stream': 'Pulled an Ollama model with streaming progress',
  'channel.ollama_delete': 'Deleted an Ollama model',

  'channel_account.create': 'Created a channel account',
  'channel_account.batch_create': 'Batch created channel accounts',
  'channel_account.import_multikey': 'Imported multi-key channel accounts',
  'channel_account.update': 'Updated a channel account',
  'channel_account.delete': 'Deleted a channel account',
  'channel_account.status': 'Changed channel account status',

  'redemption.create':
    'Created {{count}} redemption codes named {{name}} ({{quota}} each)',
  'redemption.update': 'Updated a redemption code',
  'redemption.delete': 'Deleted a redemption code',
  'redemption.delete_invalid': 'Deleted invalid redemption codes',

  'prefill_group.create': 'Created a prefill group',
  'prefill_group.update': 'Updated a prefill group',
  'prefill_group.delete': 'Deleted a prefill group',

  'vendor.create': 'Created a vendor',
  'vendor.update': 'Updated a vendor',
  'vendor.delete': 'Deleted a vendor',

  'model.create': 'Created a model',
  'model.update': 'Updated a model',
  'model.delete': 'Deleted a model',
  'model.pricing_update': 'Updated model pricing',
  'model.sync_upstream': 'Synced upstream models',

  'deployment.settings_test_connection':
    'Tested deployment settings connection',
  'deployment.test_connection': 'Tested deployment connection',
  'deployment.price_estimation': 'Estimated deployment price',
  'deployment.create': 'Created a deployment',
  'deployment.update': 'Updated a deployment',
  'deployment.rename': 'Renamed a deployment',
  'deployment.extend': 'Extended a deployment',
  'deployment.delete': 'Deleted a deployment',

  'subscription.plan_create': 'Created a subscription plan',
  'subscription.plan_update': 'Updated a subscription plan',
  'subscription.plan_status_update': 'Updated subscription plan status',
  'subscription.bind': 'Bound a subscription',
  'subscription.user_create': 'Created a user subscription',
  'subscription.user_invalidate': 'Invalidated a user subscription',
  'subscription.user_delete': 'Deleted a user subscription',
  'subscription.plan_reset': 'Reset active subscriptions for plan {{planId}}',
  'subscription.user_plan_reset':
    'Reset active plan {{planId}} subscriptions for user {{targetUserId}}',

  'authz.role_create': 'Created authz role {{roleKey}}',
  'authz.role_update': 'Updated authz role {{roleKey}}',
  'authz.role_delete': 'Deleted authz role {{roleKey}}',
  'authz.role_policies_update': 'Processed authz role policies {{roleKey}}',
  'authz.policies_import': 'Processed authz policy import {{mode}}',

  'account_pool.auth_file_create': 'Created an account pool auth file',
  'account_pool.auth_file_import': 'Imported account pool auth files',
  'account_pool.auth_file_update': 'Updated an account pool auth file',
  'account_pool.auth_file_delete': 'Deleted an account pool auth file',
  'account_pool.check_task_cleanup': 'Cleaned up account pool check tasks',
  'account_pool.group_create': 'Created an account pool group',
  'account_pool.group_update': 'Updated an account pool group',
  'account_pool.group_delete': 'Deleted an account pool group',
  'account_pool.oauth_start': 'Started account pool OAuth',
  'account_pool.oauth_complete': 'Completed account pool OAuth',
  'account_pool.device_start': 'Started account pool device authorization',
  'account_pool.account_create': 'Created an account pool account',
  'account_pool.account_batch_create': 'Batch created account pool accounts',
  'account_pool.account_attach': 'Attached account pool accounts',
  'account_pool.account_batch_status':
    'Changed account pool account statuses',
  'account_pool.account_batch_export': 'Exported account pool accounts',
  'account_pool.account_batch_delete': 'Batch deleted account pool accounts',
  'account_pool.account_batch_check': 'Checked account pool accounts',
  'account_pool.account_check_task': 'Created account pool check tasks',
  'account_pool.login_session_cancel':
    'Canceled an account pool login session',
  'account_pool.account_update': 'Updated an account pool account',
  'account_pool.account_delete': 'Deleted an account pool account',
  'account_pool.account_status': 'Changed account pool account status',
  'account_pool.account_check': 'Checked an account pool account',
  'account_pool.account_refresh': 'Refreshed an account pool account',
  'account_pool.account_runtime_reset':
    'Reset an account pool account runtime',
  'account_pool.codex_oauth_start': 'Started account pool Codex OAuth',
  'account_pool.codex_oauth_complete': 'Completed account pool Codex OAuth',

  'ratio_sync.fetch': 'Fetched upstream ratio sync data',
  'system_task.log_cleanup': 'Cleaned up historical logs',
  'system_task.upstream_account_sync':
    'Ran upstream account sync system task {{taskId}}',
  'log.clear': 'Cleared historical logs',
  generic: '{{method}} {{route}}',
}

const AUDIT_PARAM_ALIASES: Record<string, string> = {
  auth_file_id: 'authFileId',
  binding_type: 'bindingType',
  group_id: 'groupId',
  plan_id: 'planId',
  role_key: 'roleKey',
  source_id: 'sourceId',
  task_id: 'taskId',
  target_user_id: 'targetUserId',
}

const AUDIT_FALLBACK_PARAM_ORDER = [
  'username',
  'name',
  'id',
  'targetUserId',
  'target_user_id',
  'roleKey',
  'role_key',
  'key',
  'tag',
  'count',
  'quota',
  'method',
  'route',
  'status',
  'mode',
]

function normalizeAuditParams(
  params: Record<string, unknown> | undefined
): Record<string, unknown> {
  const normalized: Record<string, unknown> = {}
  if (params) {
    for (const [key, value] of Object.entries(params)) {
      normalized[key] = value
      const alias = AUDIT_PARAM_ALIASES[key]
      if (alias && normalized[alias] == null) {
        normalized[alias] = value
      }
    }
  }
  return normalized
}

function getTemplateParams(template: string): string[] {
  const params = new Set<string>()
  template.replace(/{{\s*([A-Za-z0-9_]+)\s*}}/g, (_match, key: string) => {
    params.add(key)
    return _match
  })
  return [...params]
}

function hasUsableAuditValue(value: unknown): boolean {
  if (value == null) return false
  if (typeof value !== 'string') return true
  return value.trim() !== ''
}

function hasTemplateParams(
  template: string,
  params: Record<string, unknown>
): boolean {
  return getTemplateParams(template).every((key) =>
    hasUsableAuditValue(params[key])
  )
}

function formatAuditParamValue(value: unknown): string {
  if (value == null) return '-'
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean')
    return String(value)
  if (Array.isArray(value)) {
    return value
      .map((item) => formatAuditParamValue(item))
      .filter((item) => item !== '-')
      .join(', ')
  }
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

function humanizeAuditAction(action: string): string {
  return action
    .replace(/[._-]+/g, ' ')
    .split(' ')
    .filter(Boolean)
    .map((word) => {
      const lower = word.toLowerCase()
      if (lower === 'oauth') return 'OAuth'
      if (lower === 'api') return 'API'
      if (lower === 'id') return 'ID'
      if (lower === 'codex') return 'Codex'
      if (lower === 'authz') return 'Authz'
      return lower.charAt(0).toUpperCase() + lower.slice(1)
    })
    .join(' ')
}

function summarizeAuditParams(params: Record<string, unknown>): string | null {
  const entries: Array<[string, unknown]> = []
  const used = new Set<string>()

  for (const key of AUDIT_FALLBACK_PARAM_ORDER) {
    if (!hasUsableAuditValue(params[key])) continue
    entries.push([key, params[key]])
    used.add(key)
  }

  for (const [key, value] of Object.entries(params)) {
    if (used.has(key) || !hasUsableAuditValue(value)) continue
    entries.push([key, value])
  }

  if (entries.length === 0) return null
  return entries
    .slice(0, 4)
    .map(([key, value]) => `${key}: ${formatAuditParamValue(value)}`)
    .join(', ')
}

/**
 * 从结构化的审计/登录日志 `other.op` 渲染本地化摘要。
 *
 * 返回 `null` 表示该日志没有可识别的结构化 op，调用方应继续使用旧版 content 兜底。
 * 如果模板缺少必要参数，则返回稳定 action 摘要和可展示参数，避免历史日志出现空白插值。
 */
export function renderAuditContent(
  other: LogOtherData | null | undefined,
  t: AuditTranslateFn
): string | null {
  const op = other?.op
  const action = typeof op?.action === 'string' ? op.action.trim() : ''
  if (!action) return null

  const params = normalizeAuditParams(op?.params)
  const auditInfo = other?.audit_info
  const fallbackParams = normalizeAuditParams({
    ...params,
    ...(auditInfo?.method ? { method: auditInfo.method } : {}),
    ...(auditInfo?.route ? { route: auditInfo.route } : {}),
    ...(auditInfo?.status != null ? { status: auditInfo.status } : {}),
  })
  const template = AUDIT_TEMPLATES[action]

  if (template && hasTemplateParams(template, params)) {
    return t(template, params)
  }

  const fallback = t('Audit operation {{action}}', {
    action: humanizeAuditAction(action),
  })
  const summary = summarizeAuditParams(fallbackParams)
  return summary ? `${fallback} · ${summary}` : fallback
}
