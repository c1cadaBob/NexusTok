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
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import {
  AlertTriangle,
  Copy,
  Download,
  FilterX,
  FileJson,
  KeyRound,
  Loader2,
  MoreHorizontal,
  Pencil,
  Plus,
  Power,
  PowerOff,
  RefreshCw,
  RotateCcw,
  ShieldCheck,
  Smartphone,
  Stethoscope,
  Trash2,
  Upload,
  X,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
} from '@/lib/admin-permissions'
import { useAdminPermission } from '@/hooks/use-admin-permission'
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { EmptyState } from '@/components/empty-state'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import { formatTimestamp } from '@/features/channels/lib'
import {
  accountPoolQueryKeys,
  attachPoolAccountsToGroup,
  batchCreatePoolAccounts,
  batchDeletePoolAccounts,
  batchUpdatePoolAccountStatus,
  checkPoolAccount,
  cleanupPoolAccountCheckTasks,
  completeAccountPoolProviderOAuth,
  createAccountPoolGroup,
  createPoolAccount,
  deleteAccountPoolGroup,
  deletePoolAccount,
  exportPoolAccounts,
  exportAccountPoolStateLogs,
  getAccountPoolLoginSession,
  getAccountPoolAuthFiles,
  getAccountPoolGroups,
  getAccountPoolHealth,
  getAccountPoolProviders,
  getAccountPoolStateLogAuditSummary,
  getAccountPoolStateLogs,
  getAccountPoolUsageLogs,
  getPoolAccountCheckTask,
  getPoolAccounts,
  listPoolAccountCheckTasks,
  refreshPoolAccountCredential,
  resetPoolAccountRuntime,
  startAccountPoolProviderDevice,
  startAccountPoolProviderOAuth,
  startPoolAccountCheckTask,
  updateAccountPoolGroup,
  updatePoolAccount,
  updatePoolAccountStatus,
} from './api'
import { AccountPoolHistoryFilterDrawer } from './components/account-pool-history-filter-drawer'
import {
  AccountPoolCheckTasksMobileList,
  AccountPoolStateLogsMobileList,
  AccountPoolUsageLogsMobileList,
} from './components/account-pool-log-mobile-cards'
import { AuthFilesPanel } from './components/auth-files-panel'
import {
  ACCOUNT_POOL_DEFAULT_SECTION,
  type AccountPoolSectionId,
  isAccountPoolSectionId,
} from './section-registry'
import type {
  AccountPoolAbnormalAccount,
  AccountPoolAuthFile,
  AccountPoolCheckTask,
  AccountPoolCheckTaskStatus,
  AccountPoolGroup,
  AccountPoolGroupHealth,
  AccountPoolGroupPayload,
  AccountPoolLoginSession,
  AccountPoolNoAvailableAction,
  AccountPoolPreflightCheckMode,
  AccountPoolStateLogBulkAuditSummary,
  AccountPoolTaskLimitAction,
  PoolAccount,
  PoolAccountPayload,
} from './types'

const route = getRouteApi('/_authenticated/account-pool/$section')

type GroupFormState = {
  id?: number
  name: string
  platform: string
  authType: string
  strategy: string
  models: string
  group: string
  modelMapping: string
  settings: string
  maxConcurrency: string
  rateLimitRpm: string
  dailyRequestLimit: string
  dailyQuotaLimit: string
  dailyLimitAction: string
  autoCheckEnabled: boolean
  autoCheckIntervalMinutes: string
  autoCheckLimit: string
  preflightCheckMode: AccountPoolPreflightCheckMode
  preflightCheckFreshnessMinutes: string
  preflightCheckLimit: string
  noAvailableAction: AccountPoolNoAvailableAction
  noAvailableWaitSeconds: string
  taskMaxConcurrency: string
  taskRateLimitRpm: string
  taskLimitAction: AccountPoolTaskLimitAction
  taskLimitWaitSeconds: string
}

type AccountFormState = {
  id?: number
  name: string
  credentials: string
  platform: string
  authType: string
  models: string
  group: string
  priority: string
  weight: string
  maxConcurrency: string
  rateLimitRpm: string
  dailyRequestLimit: string
  dailyQuotaLimit: string
  dailyLimitAction: string
  proxy: string
}

type AccountAddMode = 'credentials' | 'group' | 'manual'

type AccountPoolView =
  | 'health'
  | 'accounts'
  | 'auth-files'
  | 'usage-logs'
  | 'state-logs'
  | 'check-tasks'
type AccountPoolLogView = Extract<
  AccountPoolView,
  'usage-logs' | 'state-logs' | 'check-tasks'
>
type UsageLogStatusFilter = 'all' | 'success' | 'failed'
type CheckTaskStatusFilter = 'all' | AccountPoolCheckTaskStatus
type StateLogActionFilter =
  | 'all'
  | 'manual_status'
  | 'manual_clear_cooldown'
  | 'manual_delete'
  | 'runtime_reset'
  | 'check_succeeded'
  | 'check_failed'
  | 'relay_error'
  | 'daily_limit_cooling'
  | 'daily_limit_recovered'
  | 'daily_limit_disabled'
  | 'refresh_succeeded'
  | 'refresh_failed'
type StateLogSourceFilter =
  | 'all'
  | 'admin'
  | 'relay'
  | 'daily_limit'
  | 'auto_refresh'

const emptyGroupForm: GroupFormState = {
  name: '',
  platform: 'codex',
  authType: 'official_oauth',
  strategy: 'round_robin',
  models: '',
  group: '',
  modelMapping: '',
  settings: '',
  maxConcurrency: '0',
  rateLimitRpm: '0',
  dailyRequestLimit: '0',
  dailyQuotaLimit: '0',
  dailyLimitAction: 'cooldown',
  autoCheckEnabled: false,
  autoCheckIntervalMinutes: '60',
  autoCheckLimit: '100',
  preflightCheckMode: 'off',
  preflightCheckFreshnessMinutes: '1440',
  preflightCheckLimit: '20',
  noAvailableAction: 'fail',
  noAvailableWaitSeconds: '5',
  taskMaxConcurrency: '0',
  taskRateLimitRpm: '0',
  taskLimitAction: 'fail',
  taskLimitWaitSeconds: '5',
}

const emptyAccountForm: AccountFormState = {
  name: '',
  credentials: '',
  platform: '',
  authType: '',
  models: '',
  group: '',
  priority: '0',
  weight: '1',
  maxConcurrency: '0',
  rateLimitRpm: '0',
  dailyRequestLimit: '0',
  dailyQuotaLimit: '0',
  dailyLimitAction: 'inherit',
  proxy: '',
}

const authTypeOptions = [
  'api_key',
  'official_oauth',
  'cookie',
  'service_account',
  'custom_json',
]

const strategyOptions = [
  'round_robin',
  'random',
  'weighted',
  'fill_first',
  'least_used',
  'success_rate',
]
const strategyLabelKeys: Record<string, string> = {
  round_robin: 'Round robin',
  random: 'Random',
  weighted: 'Weighted',
  fill_first: 'Fill first',
  least_used: 'Least used',
  success_rate: 'Success rate first',
}
const dailyLimitActionOptions = ['cooldown', 'disable']
const accountDailyLimitActionOptions = ['inherit', ...dailyLimitActionOptions]
const preflightCheckModeOptions: AccountPoolPreflightCheckMode[] = [
  'off',
  'warmup',
  'require_recent',
]
const noAvailableActionOptions: AccountPoolNoAvailableAction[] = [
  'fail',
  'wait',
]
const taskLimitActionOptions: AccountPoolTaskLimitAction[] = ['fail', 'wait']
const checkTaskStatusFilterOptions: CheckTaskStatusFilter[] = [
  'all',
  'queued',
  'running',
  'completed',
  'failed',
]
const stateLogActionFilterOptions: StateLogActionFilter[] = [
  'all',
  'manual_status',
  'manual_clear_cooldown',
  'manual_delete',
  'runtime_reset',
  'check_succeeded',
  'check_failed',
  'relay_error',
  'daily_limit_cooling',
  'daily_limit_recovered',
  'daily_limit_disabled',
  'refresh_succeeded',
  'refresh_failed',
]
const stateLogSourceFilterOptions: StateLogSourceFilter[] = [
  'all',
  'admin',
  'relay',
  'daily_limit',
  'auto_refresh',
]

const accountPoolLogViews: AccountPoolLogView[] = [
  'usage-logs',
  'state-logs',
  'check-tasks',
]

function accountPoolViewFromSection(
  section: AccountPoolSectionId,
  logView: AccountPoolLogView
): AccountPoolView {
  if (section === 'overview') return 'health'
  if (section === 'credentials') return 'auth-files'
  if (section === 'history') return logView
  return 'accounts'
}

function accountPoolLogViewLabel(
  view: AccountPoolLogView,
  t: (key: string) => string
): string {
  if (view === 'state-logs') return t('State Logs')
  if (view === 'check-tasks') return t('Check History')
  return t('Usage Logs')
}

function numberOrZero(value: string): number {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return 0
  return parsed
}

function safeDownloadName(value: string): string {
  const normalized = value
    .trim()
    .replace(/[^a-zA-Z0-9._-]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return normalized || 'account-pool'
}

function datetimeLocalToTimestamp(value: string): number | undefined {
  const trimmed = value.trim()
  if (!trimmed) return undefined
  const time = new Date(trimmed).getTime()
  if (!Number.isFinite(time)) return undefined
  return Math.floor(time / 1000)
}

function downloadJsonFile(filename: string, data: unknown) {
  const blob = new Blob([`${JSON.stringify(data, null, 2)}\n`], {
    type: 'application/json',
  })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function strategyLabel(strategy: string, t: (key: string) => string): string {
  const labelKey = strategyLabelKeys[strategy]
  if (!labelKey) return strategy || '-'
  return t(labelKey)
}

function statusVariant(
  account: PoolAccount,
  nowSeconds: number
): 'success' | 'warning' | 'danger' | 'neutral' {
  if (account.status !== CHANNEL_STATUS.ENABLED || !account.schedulable) {
    return 'danger'
  }
  if (
    account.rate_limited_until > nowSeconds ||
    account.overload_until > nowSeconds ||
    account.temp_disabled_until > nowSeconds ||
    account.next_retry_time > nowSeconds
  ) {
    return 'warning'
  }
  if (account.unavailable) {
    return 'danger'
  }
  return 'success'
}

function statusLabel(
  account: PoolAccount,
  nowSeconds: number,
  t: (key: string) => string
): string {
  if (account.status !== CHANNEL_STATUS.ENABLED || !account.schedulable) {
    return t('Disabled')
  }
  if (statusVariant(account, nowSeconds) === 'warning') {
    return t('Cooling Down')
  }
  if (account.unavailable) {
    return t('Unavailable')
  }
  return t('Enabled')
}

function cooldownText(account: PoolAccount, nowSeconds: number): string {
  const until = Math.max(
    account.rate_limited_until,
    account.overload_until,
    account.temp_disabled_until,
    account.next_retry_time
  )
  if (until <= nowSeconds) return '-'
  return formatTimestamp(until)
}

function formatCredentialSummary(summary: string): string {
  if (!summary) return '-'
  try {
    const parsed = JSON.parse(summary) as Record<string, unknown>
    return Object.entries(parsed)
      .map(([key, value]) => `${key}: ${String(value)}`)
      .join(' | ')
  } catch {
    return summary
  }
}

function limitInlineText(value: string, maxLength = 96): string {
  if (value.length <= maxLength) return value
  return `${value.slice(0, maxLength)}...`
}

function formatCompactCredentialSummary(summary: string): string {
  const fullSummary = formatCredentialSummary(summary)
  if (fullSummary === '-') return fullSummary

  const parts = fullSummary.split(' | ').filter(Boolean)
  const preferredKeys = ['email', 'account_id', 'access_token', 'api_key']
  const preferredParts = preferredKeys
    .map((key) => parts.find((part) => part.startsWith(`${key}:`)))
    .filter((part): part is string => Boolean(part))

  const compactParts = preferredParts.length > 0 ? preferredParts : parts
  return limitInlineText(compactParts.slice(0, 2).join(' · '))
}

function credentialSummaryRecord(
  summary: string
): Record<string, unknown> | null {
  if (!summary) return null
  try {
    const parsed = JSON.parse(summary) as Record<string, unknown>
    return parsed && typeof parsed === 'object' ? parsed : null
  } catch {
    return null
  }
}

function summaryStringValue(
  record: Record<string, unknown> | null,
  keys: string[]
): string {
  if (!record) return ''
  for (const key of keys) {
    const value = record[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
    if (typeof value === 'number' && Number.isFinite(value)) {
      return String(value)
    }
  }
  return ''
}

function shortenMiddle(value: string, head = 12, tail = 6): string {
  if (value.length <= head + tail + 3) return value
  return `${value.slice(0, head)}...${value.slice(-tail)}`
}

function formatAccountIdentity(summary: string, fallback: string): string {
  const record = credentialSummaryRecord(summary)
  const email = summaryStringValue(record, [
    'email',
    'account',
    'username',
    'client_email',
  ])
  if (email) return limitInlineText(email, 64)

  const accountId = summaryStringValue(record, [
    'account_id',
    'id',
    'user_id',
    'subject',
  ])
  if (accountId) return `account_id: ${shortenMiddle(accountId)}`

  const compact = formatCompactCredentialSummary(summary)
  return compact === '-' ? fallback : compact
}

function poolAccountFileLabel(account: PoolAccount): string {
  return account.credential_label || account.name || `#${account.id}`
}

function accountRowTitle(
  account: PoolAccount,
  fullSummary: string,
  t: (key: string) => string
): string {
  return [
    `${t('Account')}: #${account.id}`,
    `${t('File')}: ${poolAccountFileLabel(account)}`,
    `${account.credential_provider || account.platform || '-'} / ${
      account.auth_type || '-'
    }`,
    fullSummary,
  ]
    .filter(Boolean)
    .join('\n')
}

function authFilePoolGroupNames(authFile: AccountPoolAuthFile): string[] {
  const names = authFile.pool_group_names?.filter(Boolean) ?? []
  if (names.length > 0) return names
  const ids = authFile.pool_group_ids?.filter((id) => id > 0) ?? []
  if (ids.length > 0) return ids.map((id) => `#${id}`)
  if (authFile.pool_group_id > 0) return [`#${authFile.pool_group_id}`]
  return []
}

function authFileAssignedToGroup(
  authFile: AccountPoolAuthFile,
  groupId: number | null
): boolean {
  if (!groupId) return false
  if (authFile.pool_group_ids?.includes(groupId)) return true
  return authFile.pool_group_id === groupId
}

function authFileSourceLabel(authFile: AccountPoolAuthFile): string {
  return [authFile.provider || authFile.platform, authFile.auth_type]
    .filter(Boolean)
    .join(' / ')
}

function authFileGroupLabel(authFile: AccountPoolAuthFile): string {
  const groups = authFilePoolGroupNames(authFile)
  if (groups.length === 0) return '-'
  const visible = groups.slice(0, 2).join(', ')
  return groups.length > 2 ? `${visible} +${groups.length - 2}` : visible
}

function accountStatusReason(
  account: PoolAccount,
  nowSeconds: number,
  t: (key: string) => string
): string {
  const coolingUntil = cooldownText(account, nowSeconds)
  if (coolingUntil !== '-') {
    return `${t('Cooling until')}: ${coolingUntil}`
  }
  return (
    account.status_message ||
    account.disabled_reason ||
    account.last_error ||
    (account.unavailable ? t('Unavailable') : '')
  )
}

function visibleAccountStatusReason(
  account: PoolAccount,
  nowSeconds: number,
  t: (key: string) => string
): string {
  const reason = accountStatusReason(account, nowSeconds, t)
  if (!reason) return ''

  const isHealthy =
    account.status === CHANNEL_STATUS.ENABLED &&
    account.schedulable &&
    !account.unavailable &&
    cooldownText(account, nowSeconds) === '-'

  if (isHealthy && reason.toLowerCase() === 'credential is available') {
    return ''
  }
  return reason
}

function formatUsageDuration(seconds: number): string {
  if (!seconds || seconds <= 0) return '-'
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  return rest > 0 ? `${minutes}m ${rest}s` : `${minutes}m`
}

function formatUsageNumber(value: number): string {
  return new Intl.NumberFormat().format(value || 0)
}

function formatPercent(value: number | undefined): string {
  const normalized = Number.isFinite(value) ? (value ?? 0) : 0
  return new Intl.NumberFormat(undefined, {
    style: 'percent',
    maximumFractionDigits: 1,
  }).format(normalized)
}

function formatLimitValue(value: number, t: (key: string) => string): string {
  return value > 0 ? formatUsageNumber(value) : t('Unlimited')
}

function dailyLimitActionLabel(
  action: string | undefined,
  t: (key: string) => string
): string {
  if (action === 'disable') {
    return t('Auto disable')
  }
  if (action === 'inherit' || !action) {
    return t('Inherit group')
  }
  return t('Cooldown until reset')
}

function preflightCheckModeLabel(
  mode: string | undefined,
  t: (key: string) => string
): string {
  if (mode === 'warmup') {
    return t('Warm up stale accounts')
  }
  if (mode === 'require_recent') {
    return t('Require recent check')
  }
  return t('Preflight off')
}

function noAvailableActionLabel(
  action: string | undefined,
  t: (key: string) => string
): string {
  if (action === 'wait') {
    return t('Wait for idle account')
  }
  return t('Fail immediately')
}

function taskLimitActionLabel(
  action: string | undefined,
  t: (key: string) => string
): string {
  if (action === 'wait') {
    return t('Wait for task slot')
  }
  return t('Fail immediately')
}

function groupPreflightCheckSummary(
  group: AccountPoolGroup,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  const mode = group.preflight_check_mode || 'off'
  if (mode === 'off') {
    return t('Preflight off')
  }
  return [
    preflightCheckModeLabel(mode, t),
    t('Fresh within {{minutes}} min', {
      minutes: group.preflight_check_freshness_minutes || 1440,
    }),
    t('Preflight limit {{limit}}', {
      limit: group.preflight_check_limit || 20,
    }),
  ].join(' · ')
}

function groupNoAvailableSummary(
  group: AccountPoolGroup,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  if (group.no_available_action === 'wait') {
    return t('Wait up to {{seconds}}s for idle account', {
      seconds: group.no_available_wait_seconds || 5,
    })
  }
  return t('Fail immediately when no idle account')
}

function groupTaskLimitSummary(
  group: AccountPoolGroup,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  const concurrency = group.task_max_concurrency || 0
  const rpm = group.task_rate_limit_rpm || 0
  if (concurrency <= 0 && rpm <= 0) {
    return t('Task submit limit off')
  }
  const parts: string[] = []
  if (concurrency > 0) {
    parts.push(
      group.task_limit_action === 'wait'
        ? t('Task concurrency {{limit}}, wait {{seconds}}s', {
            limit: concurrency,
            seconds: group.task_limit_wait_seconds || 5,
          })
        : t('Task concurrency {{limit}}, fail immediately', {
            limit: concurrency,
          })
    )
  }
  if (rpm > 0) {
    parts.push(t('Task RPM {{limit}}', { limit: rpm }))
  }
  return parts.join(' · ')
}

function groupAutoCheckSummary(
  group: AccountPoolGroup,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  if (!group.auto_check_enabled) {
    return t('Auto check off')
  }
  return [
    t('Auto check every {{minutes}} min', {
      minutes: group.auto_check_interval_minutes || 60,
    }),
    t('Auto check limit {{limit}}', {
      limit: group.auto_check_limit || 100,
    }),
  ].join(' · ')
}

function groupDailyLimitTitle(
  group: AccountPoolGroup | undefined,
  t: (key: string) => string
): string {
  if (!group?.daily_limit_state?.limited) return ''
  if (group.daily_limit_state.limit_type === 'daily_request') {
    return t('Daily request limit reached')
  }
  if (group.daily_limit_state.limit_type === 'daily_quota') {
    return t('Daily quota limit reached')
  }
  return t('Daily limit reached')
}

function groupDailyLimitSummary(
  group: AccountPoolGroup | undefined,
  t: (key: string) => string
): string {
  const title = groupDailyLimitTitle(group, t)
  if (!title) return ''
  const nextReset = group?.daily_limit_state?.next_reset_time
  return nextReset
    ? `${title} · ${t('Next daily reset')}: ${formatTimestamp(nextReset)}`
    : title
}

function groupStatusVariant(
  group: AccountPoolGroup
): 'success' | 'warning' | 'danger' | 'neutral' {
  if (group.status !== CHANNEL_STATUS.ENABLED) {
    return 'danger'
  }
  if (group.daily_limit_state?.limited) {
    return 'warning'
  }
  return 'success'
}

function groupStatusLabel(
  group: AccountPoolGroup,
  t: (key: string) => string
): string {
  if (group.status !== CHANNEL_STATUS.ENABLED) {
    return t('Disabled')
  }
  if (group.daily_limit_state?.limited) {
    return t('Daily limit reached')
  }
  return t('Enabled')
}

function healthGroupVariant(
  group: AccountPoolGroupHealth
): 'success' | 'warning' | 'danger' | 'neutral' {
  if (group.status !== CHANNEL_STATUS.ENABLED) {
    return 'danger'
  }
  if (group.daily_limit_state?.limited) {
    return 'warning'
  }
  if ((group.stats?.total ?? 0) > 0 && group.availability_rate <= 0) {
    return 'danger'
  }
  if (group.availability_rate < 1) {
    return 'warning'
  }
  return 'success'
}

function healthGroupLabel(
  group: AccountPoolGroupHealth,
  t: (key: string) => string
): string {
  if (group.status !== CHANNEL_STATUS.ENABLED) {
    return t('Disabled')
  }
  if (group.daily_limit_state?.limited) {
    return t('Daily limit reached')
  }
  if ((group.stats?.total ?? 0) > 0 && group.availability_rate <= 0) {
    return t('Unavailable')
  }
  if (group.availability_rate < 1) {
    return t('Attention')
  }
  return t('Enabled')
}

function healthGroupAutomationSummary(
  group: AccountPoolGroupHealth,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  const autoCheck = group.auto_check_enabled
    ? t('Auto check every {{minutes}} min', {
        minutes: group.auto_check_interval_minutes || 60,
      })
    : t('Auto check off')
  return [
    autoCheck,
    preflightCheckModeLabel(group.preflight_check_mode, t),
  ].join(' · ')
}

function abnormalAccountVariant(
  account: AccountPoolAbnormalAccount,
  nowSeconds: number
): 'success' | 'warning' | 'danger' | 'neutral' {
  if (account.cooling_until > nowSeconds) {
    return 'warning'
  }
  if (
    account.status !== CHANNEL_STATUS.ENABLED ||
    !account.schedulable ||
    account.unavailable
  ) {
    return 'danger'
  }
  return 'neutral'
}

function abnormalAccountStatusLabel(
  account: AccountPoolAbnormalAccount,
  nowSeconds: number,
  t: (key: string) => string
): string {
  if (account.cooling_until > nowSeconds) {
    return t('Cooling Down')
  }
  if (account.status !== CHANNEL_STATUS.ENABLED || !account.schedulable) {
    return t('Disabled')
  }
  if (account.unavailable) {
    return t('Unavailable')
  }
  return t('Attention')
}

function poolAccountStatusText(
  status: number,
  schedulable: boolean,
  unavailable: boolean,
  t: (key: string) => string
): string {
  if (status !== CHANNEL_STATUS.ENABLED || !schedulable) {
    return t('Disabled')
  }
  if (unavailable) {
    return t('Unavailable')
  }
  return t('Enabled')
}

function isAccountPoolCheckTaskActive(
  status: AccountPoolCheckTask['status'] | undefined
): boolean {
  return status === 'queued' || status === 'running'
}

function checkTaskStatusLabel(
  status: AccountPoolCheckTask['status'] | undefined,
  t: (key: string) => string
): string {
  if (status === 'queued') return t('Queued')
  if (status === 'running') return t('Running')
  if (status === 'completed') return t('Completed')
  if (status === 'failed') return t('Failed')
  return t('Unknown')
}

function checkTaskStatusFilterLabel(
  status: CheckTaskStatusFilter,
  t: (key: string) => string
): string {
  if (status === 'all') return t('All statuses')
  return checkTaskStatusLabel(status, t)
}

function checkTaskBadgeVariant(
  status: AccountPoolCheckTask['status'] | undefined
): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (status === 'failed') return 'destructive'
  if (status === 'completed') return 'default'
  if (status === 'running') return 'secondary'
  return 'outline'
}

function checkTaskProgressValue(task: AccountPoolCheckTask | null): number {
  if (!task?.total) return 0
  const progressed = Math.min(task.total, task.checked + task.skipped)
  return Math.round((progressed / task.total) * 100)
}

function stateLogActionLabel(
  action: string,
  t: (key: string) => string
): string {
  const labels: Record<string, string> = {
    manual_status: t('Manual status change'),
    manual_clear_cooldown: t('Manual cooldown clear'),
    manual_delete: t('Manual delete'),
    runtime_reset: t('Runtime reset'),
    check_succeeded: t('Check succeeded'),
    check_failed: t('Check failed'),
    relay_error: t('Relay error'),
    daily_limit_cooling: t('Daily limit cooling'),
    daily_limit_recovered: t('Daily limit recovered'),
    daily_limit_disabled: t('Daily limit auto disabled'),
    refresh_succeeded: t('Credential refresh succeeded'),
    refresh_failed: t('Credential refresh failed'),
  }
  return labels[action] || action || t('Unknown')
}

function stateLogActionFilterLabel(
  action: StateLogActionFilter,
  t: (key: string) => string
): string {
  if (action === 'all') return t('All actions')
  return stateLogActionLabel(action, t)
}

function stateLogSourceLabel(
  source: string,
  t: (key: string) => string
): string {
  const labels: Record<string, string> = {
    admin: t('Admin operations'),
    relay: t('Relay runtime'),
    daily_limit: t('Daily limit automation'),
    auto_refresh: t('Auto refresh'),
  }
  return labels[source] || source || t('Unknown')
}

function stateLogSourceFilterLabel(
  source: StateLogSourceFilter,
  t: (key: string) => string
): string {
  if (source === 'all') return t('All sources')
  return stateLogSourceLabel(source, t)
}

function bulkAuditSampleText(
  operation: AccountPoolStateLogBulkAuditSummary,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  const names = (operation.sample_accounts ?? [])
    .map((account) => account.name || `#${account.id}`)
    .filter(Boolean)
  if (names.length === 0) return '-'
  const extra = Math.max(0, operation.account_count - names.length)
  return extra > 0
    ? `${names.join(', ')} ${t('+{{count}} more', { count: extra })}`
    : names.join(', ')
}

export function AccountPool() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const params = route.useParams()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const activeSection =
    params.section && isAccountPoolSectionId(params.section)
      ? params.section
      : ACCOUNT_POOL_DEFAULT_SECTION
  const [logView, setLogView] = useState<AccountPoolLogView>('usage-logs')
  const activeView = accountPoolViewFromSection(activeSection, logView)
  const [usageLogPage, setUsageLogPage] = useState(1)
  const [usageLogStatus, setUsageLogStatus] =
    useState<UsageLogStatusFilter>('all')
  const [usageLogSearch, setUsageLogSearch] = useState('')
  const [stateLogPage, setStateLogPage] = useState(1)
  const [stateLogSearch, setStateLogSearch] = useState('')
  const [stateLogAction, setStateLogAction] =
    useState<StateLogActionFilter>('all')
  const [stateLogSource, setStateLogSource] =
    useState<StateLogSourceFilter>('all')
  const [stateLogRequestId, setStateLogRequestId] = useState('')
  const [stateLogStartTime, setStateLogStartTime] = useState('')
  const [stateLogEndTime, setStateLogEndTime] = useState('')
  const [stateLogAccountId, setStateLogAccountId] = useState<number | null>(
    null
  )
  const [stateLogAccountLabel, setStateLogAccountLabel] = useState('')
  const [stateLogExporting, setStateLogExporting] = useState(false)
  const [checkTaskPage, setCheckTaskPage] = useState(1)
  const [checkTaskStatus, setCheckTaskStatus] =
    useState<CheckTaskStatusFilter>('all')
  const [checkTaskSearch, setCheckTaskSearch] = useState('')
  const [checkTaskCleaning, setCheckTaskCleaning] = useState(false)
  const [selectedGroupId, setSelectedGroupId] = useState<number | null>(null)
  const [groupFormOpen, setGroupFormOpen] = useState(false)
  const [groupForm, setGroupForm] = useState<GroupFormState>(emptyGroupForm)
  const [accountFormOpen, setAccountFormOpen] = useState(false)
  const [accountForm, setAccountForm] =
    useState<AccountFormState>(emptyAccountForm)
  const [accountAddMode, setAccountAddMode] =
    useState<AccountAddMode>('credentials')
  const [credentialSearch, setCredentialSearch] = useState('')
  const [selectedAuthFileIds, setSelectedAuthFileIds] = useState<number[]>([])
  const [sourceGroupId, setSourceGroupId] = useState<string>('')
  const [batchOpen, setBatchOpen] = useState(false)
  const [batchCredentials, setBatchCredentials] = useState('')
  const [codexInputOpen, setCodexInputOpen] = useState(false)
  const [codexInput, setCodexInput] = useState('')
  const [codexName, setCodexName] = useState('')
  const [codexSessionId, setCodexSessionId] = useState('')
  const [deviceSessionOpen, setDeviceSessionOpen] = useState(false)
  const [deviceSession, setDeviceSession] =
    useState<AccountPoolLoginSession | null>(null)
  const [actionLoading, setActionLoading] = useState(false)
  const [checkingAccountId, setCheckingAccountId] = useState<number | null>(
    null
  )
  const [batchChecking, setBatchChecking] = useState(false)
  const [checkTask, setCheckTask] = useState<AccountPoolCheckTask | null>(null)
  const [checkTaskPolling, setCheckTaskPolling] = useState(false)
  const [selectedAccountIds, setSelectedAccountIds] = useState<number[]>([])

  const groupsQuery = useQuery({
    queryKey: accountPoolQueryKeys.groups({ page_size: 100 }),
    queryFn: () => getAccountPoolGroups({ p: 1, page_size: 100 }),
  })

  useQuery({
    queryKey: accountPoolQueryKeys.providers(),
    queryFn: getAccountPoolProviders,
  })

  const groups = groupsQuery.data?.data?.items ?? []
  const selectedGroup = groups.find((group) => group.id === selectedGroupId)
  const sourceGroupOptions = groups.filter(
    (group) => group.id !== selectedGroupId
  )
  const selectedSourceGroup = sourceGroupOptions.find(
    (group) => String(group.id) === sourceGroupId
  )
  const canOperateAccountPool = useAdminPermission(
    ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL,
    ADMIN_PERMISSION_ACTIONS.OPERATE
  )
  const canWriteAccountPool = useAdminPermission(
    ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL,
    ADMIN_PERMISSION_ACTIONS.WRITE
  )
  const canSensitiveWriteAccountPool = useAdminPermission(
    ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )
  const canReadAccountPoolAuthFile = useAdminPermission(
    ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL_AUTH_FILE,
    ADMIN_PERMISSION_ACTIONS.READ
  )
  const canSensitiveWriteAccountPoolAuthFile = useAdminPermission(
    ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL_AUTH_FILE,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )
  const selectedGroupIsEditable = Boolean(selectedGroup && canWriteAccountPool)
  const canEditGroupSensitiveFields = Boolean(
    !groupForm.id || canSensitiveWriteAccountPool
  )
  const ensureAccountPoolPermission = useCallback(
    (allowed: boolean) => {
      if (allowed) return true
      toast.error(t("You don't have necessary permission"))
      return false
    },
    [t]
  )

  useEffect(() => {
    if (
      selectedGroupId &&
      groups.length > 0 &&
      !groups.some((group) => group.id === selectedGroupId)
    ) {
      setSelectedGroupId(null)
    }
  }, [groups, selectedGroupId])

  const handleSectionChange = useCallback(
    (section: string) => {
      if (section !== 'groups') {
        setSelectedGroupId(null)
      }
      void navigate({
        to: '/account-pool/$section',
        params: { section: section as AccountPoolSectionId },
      })
    },
    [navigate]
  )

  const logViewTabs = (
    <div className='border-border border-b p-3'>
      <Tabs
        value={logView}
        onValueChange={(value) => setLogView(value as AccountPoolLogView)}
      >
        <TabsList className='max-w-full justify-start overflow-x-auto'>
          {accountPoolLogViews.map((view) => (
            <TabsTrigger key={view} value={view}>
              {accountPoolLogViewLabel(view, t)}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
    </div>
  )

  const accountsQuery = useQuery({
    queryKey: accountPoolQueryKeys.accounts(selectedGroupId ?? 0, { page }),
    queryFn: () =>
      getPoolAccounts(selectedGroupId ?? 0, {
        p: page,
        page_size: 10,
      }),
    enabled: activeSection === 'groups' && Boolean(selectedGroupId),
  })

  const attachCredentialsQuery = useQuery({
    queryKey: accountPoolQueryKeys.authFiles({
      p: 1,
      page_size: 200,
      attach_group_id: selectedGroupId ?? 0,
    }),
    queryFn: () =>
      getAccountPoolAuthFiles({
        p: 1,
        page_size: 200,
      }),
    enabled:
      canReadAccountPoolAuthFile &&
      accountFormOpen &&
      !accountForm.id &&
      Boolean(selectedGroupId),
  })

  const accountItems = accountsQuery.data?.data?.accounts.items
  const accounts = useMemo(() => accountItems ?? [], [accountItems])
  const attachCredentials = attachCredentialsQuery.data?.data?.items ?? []
  const filteredAttachCredentials = useMemo(() => {
    const needle = credentialSearch.trim().toLowerCase()
    if (!needle) return attachCredentials
    return attachCredentials.filter((authFile) =>
      [
        authFile.name,
        authFile.provider,
        authFile.platform,
        authFile.auth_type,
        authFile.subscription_type,
        authFile.credential_summary,
        authFilePoolGroupNames(authFile).join(' '),
      ]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(needle))
    )
  }, [attachCredentials, credentialSearch])
  const selectableAttachCredentialIds = filteredAttachCredentials
    .filter(
      (authFile) => !authFileAssignedToGroup(authFile, selectedGroupId ?? null)
    )
    .map((authFile) => authFile.id)
  const allAttachCredentialsSelected =
    selectableAttachCredentialIds.length > 0 &&
    selectableAttachCredentialIds.every((id) =>
      selectedAuthFileIds.includes(id)
    )
  const accountPage = accountsQuery.data?.data?.accounts
  const stats = accountsQuery.data?.data?.stats ?? selectedGroup?.stats
  const accountIdsOnPage = useMemo(
    () => accounts.map((account) => account.id),
    [accounts]
  )
  const accountIdsOnPageKey = accountIdsOnPage.join(',')
  const selectedAccountsOnPage = selectedAccountIds.filter((accountId) =>
    accountIdsOnPage.includes(accountId)
  )
  const allAccountsOnPageSelected =
    accountIdsOnPage.length > 0 &&
    accountIdsOnPage.every((accountId) =>
      selectedAccountIds.includes(accountId)
    )
  const someAccountsOnPageSelected =
    selectedAccountsOnPage.length > 0 && !allAccountsOnPageSelected
  const selectedGroupDailyLimitTitle = groupDailyLimitTitle(selectedGroup, t)
  const accountTotal = accountPage?.total ?? stats?.total ?? accounts.length
  const checkTaskBelongsToSelectedGroup =
    Boolean(checkTask) && checkTask?.pool_group_id === selectedGroupId
  const selectedGroupCheckTask = checkTaskBelongsToSelectedGroup
    ? checkTask
    : null
  const selectedGroupCheckTaskActive = isAccountPoolCheckTaskActive(
    selectedGroupCheckTask?.status
  )
  const checkTaskProgress = checkTaskProgressValue(selectedGroupCheckTask)
  const totalPages = Math.max(
    1,
    Math.ceil((accountPage?.total ?? 0) / (accountPage?.page_size ?? 10))
  )
  const nowSeconds = useMemo(() => Math.floor(Date.now() / 1000), [accounts])
  const usageLogParams = useMemo(
    () => ({
      p: usageLogPage,
      page_size: 10,
      success:
        usageLogStatus === 'all' ? undefined : usageLogStatus === 'success',
      search: usageLogSearch.trim() || undefined,
    }),
    [usageLogPage, usageLogSearch, usageLogStatus]
  )
  const usageLogsQuery = useQuery({
    queryKey: accountPoolQueryKeys.usageLogs(usageLogParams),
    queryFn: () => getAccountPoolUsageLogs(usageLogParams),
    enabled: activeView === 'usage-logs',
  })
  const usageLogPageInfo = usageLogsQuery.data?.data
  const usageLogs = usageLogPageInfo?.items ?? []
  const usageLogTotalPages = Math.max(
    1,
    Math.ceil(
      (usageLogPageInfo?.total ?? 0) / (usageLogPageInfo?.page_size ?? 10)
    )
  )
  const usageLogFilterCount =
    (usageLogStatus !== 'all' ? 1 : 0) + (usageLogSearch.trim() !== '' ? 1 : 0)
  const stateLogFilterParams = useMemo(
    () => ({
      pool_account_id: stateLogAccountId ?? undefined,
      action: stateLogAction === 'all' ? undefined : stateLogAction,
      source: stateLogSource === 'all' ? undefined : stateLogSource,
      request_id: stateLogRequestId.trim() || undefined,
      start_timestamp: datetimeLocalToTimestamp(stateLogStartTime),
      end_timestamp: datetimeLocalToTimestamp(stateLogEndTime),
      search: stateLogSearch.trim() || undefined,
    }),
    [
      stateLogAccountId,
      stateLogAction,
      stateLogEndTime,
      stateLogRequestId,
      stateLogSearch,
      stateLogSource,
      stateLogStartTime,
    ]
  )
  const stateLogParams = useMemo(
    () => ({
      ...stateLogFilterParams,
      p: stateLogPage,
      page_size: 10,
    }),
    [stateLogFilterParams, stateLogPage]
  )
  const stateLogsQuery = useQuery({
    queryKey: accountPoolQueryKeys.stateLogs(stateLogParams),
    queryFn: () => getAccountPoolStateLogs(stateLogParams),
    enabled: activeView === 'state-logs',
  })
  const stateLogAuditQuery = useQuery({
    queryKey: accountPoolQueryKeys.stateLogAuditSummary(stateLogFilterParams),
    queryFn: () => getAccountPoolStateLogAuditSummary(stateLogFilterParams),
    enabled: activeView === 'state-logs',
  })
  const stateLogPageInfo = stateLogsQuery.data?.data
  const stateLogs = stateLogPageInfo?.items ?? []
  const stateLogAuditSummary = stateLogAuditQuery.data?.data
  const stateLogTotalPages = Math.max(
    1,
    Math.ceil(
      (stateLogPageInfo?.total ?? 0) / (stateLogPageInfo?.page_size ?? 10)
    )
  )
  const stateLogFilterCount = [
    stateLogAction !== 'all',
    stateLogSource !== 'all',
    stateLogSearch.trim() !== '',
    stateLogRequestId.trim() !== '',
    stateLogStartTime.trim() !== '',
    stateLogEndTime.trim() !== '',
    stateLogAccountId !== null,
  ].filter(Boolean).length
  const hasStateLogFilters = stateLogFilterCount > 0
  const checkTaskParams = useMemo(
    () => ({
      p: checkTaskPage,
      page_size: 10,
      status: checkTaskStatus === 'all' ? undefined : checkTaskStatus,
      search: checkTaskSearch.trim() || undefined,
    }),
    [checkTaskPage, checkTaskSearch, checkTaskStatus]
  )
  const checkTasksQuery = useQuery({
    queryKey: accountPoolQueryKeys.checkTasks(checkTaskParams),
    queryFn: () => listPoolAccountCheckTasks(checkTaskParams),
    enabled: activeView === 'check-tasks',
  })
  const checkTaskPageInfo = checkTasksQuery.data?.data
  const checkTasks = checkTaskPageInfo?.items ?? []
  const checkTaskTotalPages = Math.max(
    1,
    Math.ceil(
      (checkTaskPageInfo?.total ?? 0) / (checkTaskPageInfo?.page_size ?? 10)
    )
  )
  const checkTaskFilterCount =
    (checkTaskStatus !== 'all' ? 1 : 0) +
    (checkTaskSearch.trim() !== '' ? 1 : 0)
  const healthParams = useMemo(
    () => ({
      abnormal_limit: 10,
      audit_limit: 10,
    }),
    []
  )
  const healthQuery = useQuery({
    queryKey: accountPoolQueryKeys.health(healthParams),
    queryFn: () => getAccountPoolHealth(healthParams),
    enabled: activeView === 'health',
  })
  const health = healthQuery.data?.data
  const healthTotals = health?.totals

  const clearUsageLogFilters = useCallback(() => {
    setUsageLogStatus('all')
    setUsageLogSearch('')
    setUsageLogPage(1)
  }, [])

  const clearStateLogFilters = useCallback(() => {
    setStateLogAction('all')
    setStateLogSource('all')
    setStateLogSearch('')
    setStateLogRequestId('')
    setStateLogStartTime('')
    setStateLogEndTime('')
    setStateLogAccountId(null)
    setStateLogAccountLabel('')
    setStateLogPage(1)
  }, [])

  const clearCheckTaskFilters = useCallback(() => {
    setCheckTaskStatus('all')
    setCheckTaskSearch('')
    setCheckTaskPage(1)
  }, [])

  const filterStateLogsByRequest = useCallback((requestId: string) => {
    const trimmed = requestId.trim()
    if (!trimmed) return
    setStateLogRequestId(trimmed)
    setStateLogPage(1)
  }, [])

  const filterStateLogsByAccount = useCallback(
    (accountId: number, label: string) => {
      if (accountId <= 0) return
      setStateLogAccountId(accountId)
      setStateLogAccountLabel(label || `#${accountId}`)
      setStateLogPage(1)
    },
    []
  )

  useEffect(() => {
    setUsageLogPage(1)
  }, [usageLogSearch, usageLogStatus])

  useEffect(() => {
    setStateLogPage(1)
  }, [
    stateLogAccountId,
    stateLogAction,
    stateLogEndTime,
    stateLogRequestId,
    stateLogSearch,
    stateLogSource,
    stateLogStartTime,
  ])

  useEffect(() => {
    setCheckTaskPage(1)
  }, [checkTaskSearch, checkTaskStatus])

  useEffect(() => {
    const currentPageIds = new Set(
      accountIdsOnPageKey
        ? accountIdsOnPageKey.split(',').map((accountId) => Number(accountId))
        : []
    )
    setSelectedAccountIds((current) =>
      current.filter((accountId) => currentPageIds.has(accountId))
    )
  }, [accountIdsOnPageKey])

  useEffect(() => {
    if (!deviceSessionOpen || !deviceSession?.session_id) return
    if (deviceSession.status !== 'pending') return
    const timer = window.setInterval(
      () => {
        void getAccountPoolLoginSession(deviceSession.session_id).then(
          async (response) => {
            if (!response.success || !response.data) return
            setDeviceSession(response.data)
            if (response.data.status === 'completed') {
              toast.success(t('Account created successfully'))
              await refreshAll()
            }
            if (response.data.status === 'failed') {
              toast.error(response.data.status_message || t('Operation failed'))
            }
          }
        )
      },
      Math.max(3, deviceSession.poll_interval ?? 5) * 1000
    )
    return () => window.clearInterval(timer)
  }, [deviceSession, deviceSessionOpen, t])

  const refreshAll = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: ['account-pool'] })
    await queryClient.invalidateQueries({ queryKey: ['channels'] })
  }, [queryClient])

  useEffect(() => {
    if (!checkTask?.id || !isAccountPoolCheckTaskActive(checkTask.status)) {
      return
    }

    let cancelled = false
    let timer: number | undefined

    const pollTask = async () => {
      setCheckTaskPolling(true)
      try {
        const response = await getPoolAccountCheckTask(checkTask.id)
        if (cancelled) return
        if (!response.success || !response.data) {
          throw new Error(response.message || t('Operation failed'))
        }
        const nextTask = response.data
        setCheckTask(nextTask)
        if (!isAccountPoolCheckTaskActive(nextTask.status)) {
          if (nextTask.status === 'completed') {
            const message = t(
              'Checked {{checked}} account(s): {{success}} passed, {{failed}} failed',
              {
                checked: nextTask.checked,
                success: nextTask.success,
                failed: nextTask.failed,
              }
            )
            if (nextTask.failed > 0) {
              toast.warning(message)
            } else {
              toast.success(message)
            }
          } else {
            toast.error(nextTask.message || t('Account check task failed'))
          }
          await refreshAll()
          return
        }
        timer = window.setTimeout(() => void pollTask(), 1800)
      } catch (error) {
        if (!cancelled) {
          toast.error(
            error instanceof Error ? error.message : t('Operation failed')
          )
        }
      } finally {
        if (!cancelled) {
          setCheckTaskPolling(false)
        }
      }
    }

    timer = window.setTimeout(() => void pollTask(), 1200)

    return () => {
      cancelled = true
      if (timer) {
        window.clearTimeout(timer)
      }
    }
  }, [checkTask?.id, checkTask?.status, refreshAll, t])

  const openCreateGroup = () => {
    if (!ensureAccountPoolPermission(canWriteAccountPool)) return
    setGroupForm(emptyGroupForm)
    setGroupFormOpen(true)
  }

  const openEditGroup = (group: AccountPoolGroup) => {
    if (!ensureAccountPoolPermission(canWriteAccountPool)) return
    setGroupForm({
      id: group.id,
      name: group.name,
      platform: group.platform,
      authType: group.auth_type,
      strategy: group.strategy,
      models: group.models,
      group: group.group,
      modelMapping: group.model_mapping ?? '',
      settings: group.settings ?? '',
      maxConcurrency: String(group.max_concurrency || 0),
      rateLimitRpm: String(group.rate_limit_rpm || 0),
      dailyRequestLimit: String(group.daily_request_limit || 0),
      dailyQuotaLimit: String(group.daily_quota_limit || 0),
      dailyLimitAction: group.daily_limit_action || 'cooldown',
      autoCheckEnabled: Boolean(group.auto_check_enabled),
      autoCheckIntervalMinutes: String(group.auto_check_interval_minutes || 60),
      autoCheckLimit: String(group.auto_check_limit || 100),
      preflightCheckMode:
        group.preflight_check_mode === 'warmup' ||
        group.preflight_check_mode === 'require_recent'
          ? group.preflight_check_mode
          : 'off',
      preflightCheckFreshnessMinutes: String(
        group.preflight_check_freshness_minutes || 1440
      ),
      preflightCheckLimit: String(group.preflight_check_limit || 20),
      noAvailableAction: group.no_available_action === 'wait' ? 'wait' : 'fail',
      noAvailableWaitSeconds: String(group.no_available_wait_seconds || 5),
      taskMaxConcurrency: String(group.task_max_concurrency || 0),
      taskRateLimitRpm: String(group.task_rate_limit_rpm || 0),
      taskLimitAction: group.task_limit_action === 'wait' ? 'wait' : 'fail',
      taskLimitWaitSeconds: String(group.task_limit_wait_seconds || 5),
    })
    setGroupFormOpen(true)
  }

  const submitGroup = async () => {
    if (!ensureAccountPoolPermission(canWriteAccountPool)) return
    if (!groupForm.name.trim()) {
      toast.error(t('Name is required'))
      return
    }
    setActionLoading(true)
    try {
      const payload: AccountPoolGroupPayload = {
        name: groupForm.name.trim(),
        platform: groupForm.platform.trim(),
        auth_type: groupForm.authType,
        strategy: groupForm.strategy,
        models: groupForm.models.trim(),
        group: groupForm.group.trim(),
        model_mapping: groupForm.modelMapping.trim(),
        settings: groupForm.settings.trim(),
        max_concurrency: numberOrZero(groupForm.maxConcurrency),
        rate_limit_rpm: numberOrZero(groupForm.rateLimitRpm),
        daily_request_limit: numberOrZero(groupForm.dailyRequestLimit),
        daily_quota_limit: numberOrZero(groupForm.dailyQuotaLimit),
        daily_limit_action: groupForm.dailyLimitAction,
        auto_check_enabled: groupForm.autoCheckEnabled,
        auto_check_interval_minutes: numberOrZero(
          groupForm.autoCheckIntervalMinutes
        ),
        auto_check_limit: numberOrZero(groupForm.autoCheckLimit),
        preflight_check_mode: groupForm.preflightCheckMode,
        preflight_check_freshness_minutes: numberOrZero(
          groupForm.preflightCheckFreshnessMinutes
        ),
        preflight_check_limit: numberOrZero(groupForm.preflightCheckLimit),
        no_available_action: groupForm.noAvailableAction,
        no_available_wait_seconds: numberOrZero(
          groupForm.noAvailableWaitSeconds
        ),
        task_max_concurrency: numberOrZero(groupForm.taskMaxConcurrency),
        task_rate_limit_rpm: numberOrZero(groupForm.taskRateLimitRpm),
        task_limit_action: groupForm.taskLimitAction,
        task_limit_wait_seconds: numberOrZero(groupForm.taskLimitWaitSeconds),
      }
      const response = groupForm.id
        ? await updateAccountPoolGroup(groupForm.id, payload)
        : await createAccountPoolGroup(payload)
      if (!response.success) throw new Error(response.message)
      toast.success(t('Operation successful'))
      setGroupFormOpen(false)
      await refreshAll()
      if (!groupForm.id && response.data?.id) {
        setSelectedGroupId(response.data.id)
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const deleteGroup = async (group: AccountPoolGroup) => {
    if (!ensureAccountPoolPermission(canSensitiveWriteAccountPool)) return
    if (
      !window.confirm(
        t('Are you sure you want to delete this account pool group?')
      )
    ) {
      return
    }
    setActionLoading(true)
    try {
      const response = await deleteAccountPoolGroup(group.id)
      if (!response.success) throw new Error(response.message)
      toast.success(t('Operation successful'))
      if (selectedGroupId === group.id) {
        setSelectedGroupId(null)
      }
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const openCreateAccount = () => {
    if (!ensureAccountPoolPermission(canSensitiveWriteAccountPool)) return
    setAccountForm({
      ...emptyAccountForm,
      platform: selectedGroup?.platform ?? '',
      authType: selectedGroup?.auth_type ?? '',
    })
    setAccountAddMode(canReadAccountPoolAuthFile ? 'credentials' : 'manual')
    setCredentialSearch('')
    setSelectedAuthFileIds([])
    setSourceGroupId('')
    setAccountFormOpen(true)
  }

  const openEditAccount = (account: PoolAccount) => {
    if (!ensureAccountPoolPermission(canSensitiveWriteAccountPool)) return
    setAccountForm({
      id: account.id,
      name: account.name,
      credentials: '',
      platform: account.platform,
      authType: account.auth_type,
      models: account.models,
      group: account.group,
      priority: String(account.priority),
      weight: String(account.weight || 1),
      maxConcurrency: String(account.max_concurrency || 0),
      rateLimitRpm: String(account.rate_limit_rpm || 0),
      dailyRequestLimit: String(account.daily_request_limit || 0),
      dailyQuotaLimit: String(account.daily_quota_limit || 0),
      dailyLimitAction: account.daily_limit_action || 'inherit',
      proxy: account.proxy || '',
    })
    setAccountAddMode('manual')
    setAccountFormOpen(true)
  }

  const toggleAuthFileSelection = (authFileId: number, checked: boolean) => {
    setSelectedAuthFileIds((current) => {
      if (checked) {
        return current.includes(authFileId) ? current : [...current, authFileId]
      }
      return current.filter((id) => id !== authFileId)
    })
  }

  const toggleAllAttachCredentials = (checked: boolean) => {
    if (!checked) {
      setSelectedAuthFileIds((current) =>
        current.filter((id) => !selectableAttachCredentialIds.includes(id))
      )
      return
    }
    setSelectedAuthFileIds((current) => [
      ...current,
      ...selectableAttachCredentialIds.filter((id) => !current.includes(id)),
    ])
  }

  const submitAttachAccounts = async () => {
    if (!ensureAccountPoolPermission(canSensitiveWriteAccountPool)) return
    if (!selectedGroupId) return
    if (
      accountAddMode === 'credentials' &&
      !ensureAccountPoolPermission(canReadAccountPoolAuthFile)
    ) {
      return
    }
    if (accountAddMode === 'credentials' && selectedAuthFileIds.length === 0) {
      toast.error(t('Select at least one credential'))
      return
    }
    if (accountAddMode === 'group' && !sourceGroupId) {
      toast.error(t('Select a source account group'))
      return
    }
    setActionLoading(true)
    try {
      const response = await attachPoolAccountsToGroup(selectedGroupId, {
        auth_file_ids:
          accountAddMode === 'credentials' ? selectedAuthFileIds : undefined,
        source_group_id:
          accountAddMode === 'group' ? Number(sourceGroupId) : undefined,
        skip_existing: true,
      })
      if (!response.success) throw new Error(response.message)
      toast.success(
        t('Added {{created}} account(s), skipped {{skipped}}', {
          created: response.data?.created ?? 0,
          skipped: response.data?.skipped ?? 0,
        })
      )
      if ((response.data?.failed ?? 0) > 0) {
        const firstError = response.data?.items?.find(
          (item) => !item.success && !item.skipped
        )
        toast.error(firstError?.message ?? t('Some accounts failed to add'))
      }
      setAccountFormOpen(false)
      setSelectedAuthFileIds([])
      setSourceGroupId('')
      setCredentialSearch('')
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const submitAccount = async () => {
    if (!ensureAccountPoolPermission(canSensitiveWriteAccountPool)) return
    if (!selectedGroupId) return
    if (!accountForm.id && accountAddMode !== 'manual') {
      await submitAttachAccounts()
      return
    }
    if (!accountForm.name.trim()) {
      toast.error(t('Name is required'))
      return
    }
    if (!accountForm.id && !accountForm.credentials.trim()) {
      toast.error(t('Credentials are required'))
      return
    }
    setActionLoading(true)
    try {
      const payload: PoolAccountPayload = {
        name: accountForm.name.trim(),
        platform: accountForm.platform.trim(),
        auth_type: accountForm.authType,
        credentials: accountForm.credentials.trim() || undefined,
        models: accountForm.models.trim(),
        group: accountForm.group.trim(),
        priority: numberOrZero(accountForm.priority),
        weight: numberOrZero(accountForm.weight),
        max_concurrency: numberOrZero(accountForm.maxConcurrency),
        rate_limit_rpm: numberOrZero(accountForm.rateLimitRpm),
        daily_request_limit: numberOrZero(accountForm.dailyRequestLimit),
        daily_quota_limit: numberOrZero(accountForm.dailyQuotaLimit),
        daily_limit_action: accountForm.dailyLimitAction,
        proxy: accountForm.proxy.trim(),
        status: CHANNEL_STATUS.ENABLED,
        schedulable: true,
      }
      const response = accountForm.id
        ? await updatePoolAccount(accountForm.id, payload)
        : await createPoolAccount(selectedGroupId, payload)
      if (!response.success) throw new Error(response.message)
      toast.success(t('Operation successful'))
      setAccountFormOpen(false)
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const submitBatch = async () => {
    if (!ensureAccountPoolPermission(canSensitiveWriteAccountPool)) return
    if (!selectedGroupId || !batchCredentials.trim()) return
    setActionLoading(true)
    try {
      const response = await batchCreatePoolAccounts(selectedGroupId, {
        credentials: batchCredentials,
        name_prefix: selectedGroup?.name ?? '账号',
        platform: selectedGroup?.platform,
        auth_type: selectedGroup?.auth_type,
        weight: 1,
        status: CHANNEL_STATUS.ENABLED,
      })
      if (!response.success) throw new Error(response.message)
      toast.success(
        t('Imported {{created}} account(s), skipped {{skipped}}', {
          created: response.data?.created ?? 0,
          skipped: response.data?.skipped ?? 0,
        })
      )
      setBatchOpen(false)
      setBatchCredentials('')
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const exportAccounts = async (accountIds?: number[]) => {
    if (!ensureAccountPoolPermission(canOperateAccountPool)) return
    if (!selectedGroupId || !selectedGroup) return
    if (accountIds && accountIds.length === 0) {
      toast.info(t('No accounts selected'))
      return
    }
    setActionLoading(true)
    try {
      const response = await exportPoolAccounts(selectedGroupId, {
        account_ids: accountIds,
      })
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      const exportedAt =
        response.data.exported_at || Math.floor(Date.now() / 1000)
      const filename = `${safeDownloadName(
        selectedGroup.name
      )}-accounts-${exportedAt}.json`
      downloadJsonFile(filename, response.data)
      toast.success(
        t('Exported {{count}} account(s)', {
          count: response.data.exported,
        })
      )
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const exportStateLogs = async () => {
    if (!ensureAccountPoolPermission(canOperateAccountPool)) return
    setStateLogExporting(true)
    try {
      const response = await exportAccountPoolStateLogs({
        ...stateLogFilterParams,
        limit: 1000,
      })
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      const exportedAt =
        response.data.exported_at || Math.floor(Date.now() / 1000)
      const filename = `${safeDownloadName(
        selectedGroup?.name ?? 'all-groups'
      )}-state-audit-${exportedAt}.json`
      downloadJsonFile(filename, response.data)
      toast.success(
        t('Exported {{count}} audit log(s)', {
          count: response.data.exported,
        })
      )
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setStateLogExporting(false)
    }
  }

  const toggleAccountSelection = (accountId: number, checked: boolean) => {
    setSelectedAccountIds((current) => {
      if (checked) {
        if (current.includes(accountId)) return current
        return [...current, accountId]
      }
      return current.filter((id) => id !== accountId)
    })
  }

  const toggleAllAccountsOnPage = (checked: boolean) => {
    setSelectedAccountIds((current) => {
      if (!checked) {
        return current.filter(
          (accountId) => !accountIdsOnPage.includes(accountId)
        )
      }
      const next = [...current]
      for (const accountId of accountIdsOnPage) {
        if (!next.includes(accountId)) {
          next.push(accountId)
        }
      }
      return next
    })
  }

  const batchUpdateSelectedAccountStatus = async (
    action: 'enable' | 'disable' | 'clear_cooldown'
  ) => {
    if (!ensureAccountPoolPermission(canOperateAccountPool)) return
    if (!selectedGroupId) return
    if (selectedAccountIds.length === 0) {
      toast.info(t('No accounts selected'))
      return
    }
    setActionLoading(true)
    try {
      const accountIds = [...selectedAccountIds]
      const response = await batchUpdatePoolAccountStatus(selectedGroupId, {
        account_ids: accountIds,
        status:
          action === 'clear_cooldown'
            ? undefined
            : action === 'enable'
              ? CHANNEL_STATUS.ENABLED
              : CHANNEL_STATUS.MANUAL_DISABLED,
        schedulable:
          action === 'clear_cooldown' ? undefined : action === 'enable',
        clear_cooldown: action === 'clear_cooldown',
      })
      if (!response.success) throw new Error(response.message)
      const message = t(
        'Updated {{updated}} account(s), skipped {{skipped}}, failed {{failed}}',
        {
          updated: response.data?.updated ?? 0,
          skipped: response.data?.skipped ?? 0,
          failed: response.data?.failed ?? 0,
        }
      )
      if ((response.data?.failed ?? 0) > 0) {
        toast.warning(message)
      } else {
        toast.success(message)
      }
      setSelectedAccountIds([])
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const batchDeleteSelectedAccounts = async () => {
    if (!ensureAccountPoolPermission(canSensitiveWriteAccountPool)) return
    if (!selectedGroupId) return
    if (selectedAccountIds.length === 0) {
      toast.info(t('No accounts selected'))
      return
    }
    if (
      !window.confirm(t('Are you sure you want to delete selected accounts?'))
    ) {
      return
    }
    setActionLoading(true)
    try {
      const response = await batchDeletePoolAccounts(selectedGroupId, {
        account_ids: [...selectedAccountIds],
      })
      if (!response.success) throw new Error(response.message)
      const message = t(
        'Deleted {{deleted}} account(s), skipped {{skipped}}, failed {{failed}}',
        {
          deleted: response.data?.deleted ?? 0,
          skipped: response.data?.skipped ?? 0,
          failed: response.data?.failed ?? 0,
        }
      )
      if ((response.data?.failed ?? 0) > 0) {
        toast.warning(message)
      } else {
        toast.success(message)
      }
      setSelectedAccountIds([])
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const setAccountEnabled = async (account: PoolAccount, enabled: boolean) => {
    if (!ensureAccountPoolPermission(canOperateAccountPool)) return
    setActionLoading(true)
    try {
      const response = await updatePoolAccountStatus(account.id, {
        status: enabled
          ? CHANNEL_STATUS.ENABLED
          : CHANNEL_STATUS.MANUAL_DISABLED,
        schedulable: enabled,
      })
      if (!response.success) throw new Error(response.message)
      toast.success(t('Operation successful'))
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const clearCooldown = async (account: PoolAccount) => {
    if (!ensureAccountPoolPermission(canOperateAccountPool)) return
    setActionLoading(true)
    try {
      const response = await updatePoolAccountStatus(account.id, {
        clear_cooldown: true,
      })
      if (!response.success) throw new Error(response.message)
      toast.success(t('Operation successful'))
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const deleteAccount = async (account: PoolAccount) => {
    if (!ensureAccountPoolPermission(canSensitiveWriteAccountPool)) return
    if (!window.confirm(t('Are you sure you want to delete this account?'))) {
      return
    }
    setActionLoading(true)
    try {
      const response = await deletePoolAccount(account.id)
      if (!response.success) throw new Error(response.message)
      toast.success(t('Operation successful'))
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const refreshCredential = async (account: PoolAccount) => {
    if (!ensureAccountPoolPermission(canSensitiveWriteAccountPool)) return
    setActionLoading(true)
    try {
      const response = await refreshPoolAccountCredential(account.id)
      if (!response.success) throw new Error(response.message)
      toast.success(t('Credential refreshed'))
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const checkAccount = async (account: PoolAccount) => {
    if (!ensureAccountPoolPermission(canOperateAccountPool)) return
    setCheckingAccountId(account.id)
    try {
      const response = await checkPoolAccount(account.id)
      if (!response.success) throw new Error(response.message)
      if (response.data?.success) {
        toast.success(t('Account check passed'))
      } else {
        toast.error(
          t('Account check failed: {{message}}', {
            message: response.data?.message || t('Unknown error'),
          })
        )
      }
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setCheckingAccountId(null)
    }
  }

  const checkSelectedGroupAccounts = async () => {
    if (!ensureAccountPoolPermission(canOperateAccountPool)) return
    if (!selectedGroupId) return
    if (accountTotal <= 0) {
      toast.info(t('No accounts found'))
      return
    }
    if (selectedGroupCheckTaskActive) {
      toast.info(t('Account check task is already running'))
      return
    }
    setBatchChecking(true)
    try {
      const response = await startPoolAccountCheckTask(selectedGroupId, {
        limit: 100,
      })
      if (!response.success) throw new Error(response.message)
      if (response.data) {
        setCheckTask(response.data)
      }
      toast.success(t('Account check task started'))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setBatchChecking(false)
    }
  }

  const cleanupCheckTasks = async () => {
    if (!ensureAccountPoolPermission(canOperateAccountPool)) return
    const confirmed = window.confirm(
      t('Clean completed and failed check tasks older than 7 days?')
    )
    if (!confirmed) return
    setCheckTaskCleaning(true)
    try {
      const beforeTimestamp = Math.floor(Date.now() / 1000) - 7 * 24 * 60 * 60
      const response = await cleanupPoolAccountCheckTasks({
        pool_group_id: selectedGroupId ?? undefined,
        before_timestamp: beforeTimestamp,
        statuses: ['completed', 'failed'],
        limit: 500,
      })
      if (!response.success) throw new Error(response.message)
      toast.success(
        t('Cleaned {{count}} check task(s)', {
          count: response.data?.deleted ?? 0,
        })
      )
      await checkTasksQuery.refetch()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setCheckTaskCleaning(false)
    }
  }

  const viewCheckTask = (task: AccountPoolCheckTask) => {
    setCheckTask(task)
    if (task.pool_group_id) {
      setSelectedGroupId(task.pool_group_id)
    }
    handleSectionChange('groups')
  }

  const resetRuntime = async (account: PoolAccount) => {
    if (!ensureAccountPoolPermission(canOperateAccountPool)) return
    setActionLoading(true)
    try {
      const response = await resetPoolAccountRuntime(account.id)
      if (!response.success) throw new Error(response.message)
      toast.success(t('Operation successful'))
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const startCodexOAuth = async () => {
    if (!ensureAccountPoolPermission(canSensitiveWriteAccountPool)) return
    if (!selectedGroupId) return
    setActionLoading(true)
    try {
      const response = await startAccountPoolProviderOAuth(
        selectedGroupId,
        'codex',
        {}
      )
      if (!response.success) throw new Error(response.message)
      if (response.data?.authorize_url) {
        window.open(
          response.data.authorize_url,
          '_blank',
          'noopener,noreferrer'
        )
      }
      setCodexSessionId(response.data?.session_id ?? '')
      setCodexInputOpen(true)
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const completeCodexOAuth = async () => {
    if (!ensureAccountPoolPermission(canSensitiveWriteAccountPool)) return
    if (!selectedGroupId || !codexInput.trim()) return
    setActionLoading(true)
    try {
      const response = await completeAccountPoolProviderOAuth(
        selectedGroupId,
        'codex',
        {
          session_id: codexSessionId,
          input: codexInput.trim(),
          name: codexName.trim(),
        }
      )
      if (!response.success) throw new Error(response.message)
      toast.success(t('Account created successfully'))
      setCodexInputOpen(false)
      setCodexInput('')
      setCodexName('')
      setCodexSessionId('')
      await refreshAll()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const startCodexDevice = async () => {
    if (!ensureAccountPoolPermission(canSensitiveWriteAccountPool)) return
    if (!selectedGroupId) return
    setActionLoading(true)
    try {
      const response = await startAccountPoolProviderDevice(
        selectedGroupId,
        'codex',
        {}
      )
      if (!response.success || !response.data) {
        throw new Error(response.message)
      }
      setDeviceSession({
        session_id: response.data.session_id,
        provider: response.data.provider,
        mode: response.data.mode,
        status: 'pending',
        verification_url: response.data.verification_url,
        user_code: response.data.user_code,
        expires_at: response.data.expires_at,
        poll_interval: response.data.poll_interval,
      })
      setDeviceSessionOpen(true)
      if (response.data.verification_url) {
        window.open(
          response.data.verification_url,
          '_blank',
          'noopener,noreferrer'
        )
      }
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Account Pool')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button variant='outline' onClick={() => void refreshAll()}>
            <RefreshCw data-icon='inline-start' />
            {t('Refresh')}
          </Button>
          {activeSection === 'groups' ? (
            <Button disabled={!canWriteAccountPool} onClick={openCreateGroup}>
              <Plus data-icon='inline-start' />
              {t('New Group')}
            </Button>
          ) : null}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex flex-col gap-4'>
            <Tabs value={activeView} className='flex min-h-0 flex-col gap-4'>
              <TabsContent value='health' className='m-0 flex flex-col gap-4'>
                <Card size='sm'>
                  <CardHeader className='border-b'>
                    <CardTitle>{t('Overview')}</CardTitle>
                    <CardDescription>
                      <span className='block'>
                        {t('Generated at')}:&nbsp;
                        {health?.generated_at
                          ? formatTimestamp(health.generated_at)
                          : '-'}
                      </span>
                      <span className='block'>
                        {t('Window')}:&nbsp;
                        {health?.window_start
                          ? formatTimestamp(health.window_start)
                          : '-'}
                        {' - '}
                        {health?.window_end
                          ? formatTimestamp(health.window_end)
                          : '-'}
                      </span>
                    </CardDescription>
                    <CardAction>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => void healthQuery.refetch()}
                      >
                        {healthQuery.isFetching ? (
                          <Loader2
                            data-icon='inline-start'
                            className='animate-spin'
                          />
                        ) : (
                          <RefreshCw data-icon='inline-start' />
                        )}
                        {t('Refresh health')}
                      </Button>
                    </CardAction>
                  </CardHeader>
                  <CardContent>
                    <div className='grid grid-cols-2 gap-3 text-sm md:grid-cols-5 xl:grid-cols-10'>
                      {[
                        {
                          label: t('Total accounts'),
                          value: formatUsageNumber(
                            healthTotals?.total_accounts ?? 0
                          ),
                        },
                        {
                          label: t('Available accounts'),
                          value: formatUsageNumber(
                            healthTotals?.available_accounts ?? 0
                          ),
                        },
                        {
                          label: t('Disabled accounts'),
                          value: formatUsageNumber(
                            healthTotals?.disabled_accounts ?? 0
                          ),
                        },
                        {
                          label: t('Cooldown accounts'),
                          value: formatUsageNumber(
                            healthTotals?.cooldown_accounts ?? 0
                          ),
                        },
                        {
                          label: t('Unavailable accounts'),
                          value: formatUsageNumber(
                            healthTotals?.unavailable_accounts ?? 0
                          ),
                        },
                        {
                          label: t('Today requests'),
                          value: formatUsageNumber(
                            healthTotals?.today_requests ?? 0
                          ),
                        },
                        {
                          label: t('Today failures'),
                          value: formatUsageNumber(
                            healthTotals?.today_failures ?? 0
                          ),
                        },
                        {
                          label: t('Success rate'),
                          value: formatPercent(healthTotals?.success_rate),
                        },
                        {
                          label: t('Availability rate'),
                          value: formatPercent(healthTotals?.availability_rate),
                        },
                        {
                          label: t('Limited groups'),
                          value: formatUsageNumber(
                            healthTotals?.limited_group_count ?? 0
                          ),
                        },
                      ].map((metric) => (
                        <div key={metric.label} className='min-w-0'>
                          <div className='text-muted-foreground truncate text-xs'>
                            {metric.label}
                          </div>
                          <div className='truncate font-medium'>
                            {metric.value}
                          </div>
                        </div>
                      ))}
                    </div>
                  </CardContent>
                </Card>

                <Card size='sm'>
                  <CardHeader className='border-b'>
                    <CardTitle>{t('Group health')}</CardTitle>
                    <CardDescription>
                      {t('Account pool health across all native groups')}
                    </CardDescription>
                    <CardAction>
                      {healthQuery.isFetching ? (
                        <Loader2
                          className='text-muted-foreground animate-spin'
                          aria-hidden='true'
                        />
                      ) : null}
                    </CardAction>
                  </CardHeader>
                  <CardContent>
                    <div className='rounded-md border'>
                      <Table className='min-w-[1120px]'>
                        <TableHeader>
                          <TableRow className='bg-muted/40 hover:bg-muted/40'>
                            <TableHead className='px-4'>{t('Group')}</TableHead>
                            <TableHead>{t('Available rate')}</TableHead>
                            <TableHead>{t('Today requests')}</TableHead>
                            <TableHead>{t('Today failures')}</TableHead>
                            <TableHead>{t('Success rate')}</TableHead>
                            <TableHead>{t('Status')}</TableHead>
                            <TableHead className='pr-4'>
                              {t('Automation')}
                            </TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {(health?.groups ?? []).map((group) => (
                            <TableRow key={group.id}>
                              <TableCell className='min-w-[200px] px-4'>
                                <div className='text-sm font-medium'>
                                  {group.name || `#${group.id}`}
                                </div>
                                <div className='text-muted-foreground text-xs'>
                                  {group.platform} / {group.auth_type} ·{' '}
                                  {strategyLabel(group.strategy, t)}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[150px] text-xs'>
                                <div className='font-medium'>
                                  {formatPercent(group.availability_rate)}
                                </div>
                                <div className='text-muted-foreground mt-1'>
                                  {formatUsageNumber(group.stats?.enabled ?? 0)}{' '}
                                  / {formatUsageNumber(group.stats?.total ?? 0)}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[130px] text-xs'>
                                {formatUsageNumber(group.today_requests)}
                              </TableCell>
                              <TableCell className='min-w-[130px] text-xs'>
                                {formatUsageNumber(group.today_failures)}
                              </TableCell>
                              <TableCell className='min-w-[120px] text-xs'>
                                {formatPercent(group.success_rate)}
                              </TableCell>
                              <TableCell className='min-w-[150px]'>
                                <div className='flex flex-col gap-1'>
                                  <StatusBadge
                                    label={healthGroupLabel(group, t)}
                                    variant={healthGroupVariant(group)}
                                    copyable={false}
                                  />
                                  {group.daily_limit_state?.limited ? (
                                    <span className='text-muted-foreground text-xs'>
                                      {group.daily_limit_state.next_reset_time
                                        ? `${t('Next daily reset')}: ${formatTimestamp(
                                            group.daily_limit_state
                                              .next_reset_time
                                          )}`
                                        : group.daily_limit_state.reason || '-'}
                                    </span>
                                  ) : null}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[260px] pr-4 text-xs'>
                                {healthGroupAutomationSummary(group, t)}
                                {group.auto_check_next_time ? (
                                  <div className='text-muted-foreground mt-1'>
                                    {t('Next auto check')}:&nbsp;
                                    {formatTimestamp(
                                      group.auto_check_next_time
                                    )}
                                  </div>
                                ) : null}
                              </TableCell>
                            </TableRow>
                          ))}
                          {!healthQuery.isLoading &&
                            (health?.groups ?? []).length === 0 && (
                              <TableRow>
                                <TableCell
                                  colSpan={7}
                                  className='h-24 text-center'
                                >
                                  {t('No account groups found')}
                                </TableCell>
                              </TableRow>
                            )}
                          {healthQuery.isLoading ? (
                            <TableRow>
                              <TableCell
                                colSpan={7}
                                className='h-24 text-center'
                              >
                                {t('Loading')}
                              </TableCell>
                            </TableRow>
                          ) : null}
                        </TableBody>
                      </Table>
                    </div>
                  </CardContent>
                </Card>

                <Card size='sm'>
                  <CardHeader className='border-b'>
                    <CardTitle>{t('Recent abnormal accounts')}</CardTitle>
                    <CardDescription>
                      {t('Review account pool health and recent exceptions.')}
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className='rounded-md border'>
                      <Table className='min-w-[1180px]'>
                        <TableHeader>
                          <TableRow className='bg-muted/40 hover:bg-muted/40'>
                            <TableHead className='px-4'>
                              {t('Account')}
                            </TableHead>
                            <TableHead>{t('Status')}</TableHead>
                            <TableHead>{t('Reason')}</TableHead>
                            <TableHead>{t('Cooling until')}</TableHead>
                            <TableHead>{t('Failure rate')}</TableHead>
                            <TableHead>{t('Last Used')}</TableHead>
                            <TableHead className='pr-4'>
                              {t('Last check time')}
                            </TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {(health?.recent_abnormal_accounts ?? []).map(
                            (account) => (
                              <TableRow key={account.id}>
                                <TableCell className='min-w-[220px] px-4'>
                                  <div className='text-sm font-medium'>
                                    {account.name || `#${account.id}`}
                                  </div>
                                  <div className='text-muted-foreground text-xs'>
                                    {account.pool_group_name ||
                                      `#${account.pool_group_id}`}
                                    {' · '}
                                    {account.credential_provider ||
                                      account.platform ||
                                      '-'}
                                    {' / '}
                                    {account.auth_type || '-'}
                                  </div>
                                </TableCell>
                                <TableCell className='min-w-[140px]'>
                                  <StatusBadge
                                    label={abnormalAccountStatusLabel(
                                      account,
                                      nowSeconds,
                                      t
                                    )}
                                    variant={abnormalAccountVariant(
                                      account,
                                      nowSeconds
                                    )}
                                    copyable={false}
                                  />
                                </TableCell>
                                <TableCell className='max-w-[320px] min-w-[240px] text-xs break-words'>
                                  {account.reason ||
                                    account.last_error ||
                                    account.status_message ||
                                    account.disabled_reason ||
                                    '-'}
                                </TableCell>
                                <TableCell className='min-w-[150px] text-xs'>
                                  {account.cooling_until > nowSeconds
                                    ? formatTimestamp(account.cooling_until)
                                    : '-'}
                                </TableCell>
                                <TableCell className='min-w-[130px] text-xs'>
                                  {formatPercent(account.failure_rate)}
                                  <div className='text-muted-foreground mt-1'>
                                    {t('Success')}: {account.success_count} ·{' '}
                                    {t('Failed')}: {account.failed_count}
                                  </div>
                                </TableCell>
                                <TableCell className='min-w-[150px] text-xs'>
                                  {account.last_used_time
                                    ? formatTimestamp(account.last_used_time)
                                    : '-'}
                                </TableCell>
                                <TableCell className='min-w-[150px] pr-4 text-xs'>
                                  {account.last_checked_time
                                    ? formatTimestamp(account.last_checked_time)
                                    : '-'}
                                </TableCell>
                              </TableRow>
                            )
                          )}
                          {!healthQuery.isLoading &&
                            (health?.recent_abnormal_accounts ?? []).length ===
                              0 && (
                              <TableRow>
                                <TableCell
                                  colSpan={7}
                                  className='h-24 text-center'
                                >
                                  {t('No abnormal accounts found')}
                                </TableCell>
                              </TableRow>
                            )}
                        </TableBody>
                      </Table>
                    </div>
                  </CardContent>
                </Card>

                <Card size='sm'>
                  <CardHeader className='border-b'>
                    <CardTitle>{t('Recent state changes')}</CardTitle>
                    <CardDescription>
                      {t(
                        'Inspect usage records, state changes, and check tasks.'
                      )}
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className='rounded-md border'>
                      <Table className='min-w-[960px]'>
                        <TableHeader>
                          <TableRow className='bg-muted/40 hover:bg-muted/40'>
                            <TableHead className='px-4'>{t('Time')}</TableHead>
                            <TableHead>{t('Account')}</TableHead>
                            <TableHead>{t('Action')}</TableHead>
                            <TableHead>{t('After state')}</TableHead>
                            <TableHead className='pr-4'>
                              {t('Reason')}
                            </TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {(health?.recent_state_logs ?? []).map((log) => (
                            <TableRow key={log.id}>
                              <TableCell className='min-w-[150px] px-4 text-xs'>
                                {formatTimestamp(log.created_at)}
                                <div className='text-muted-foreground mt-1'>
                                  {log.request_id || '-'}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[200px]'>
                                <div className='text-sm font-medium'>
                                  {log.pool_account_name ||
                                    `#${log.pool_account_id}`}
                                </div>
                                <div className='text-muted-foreground text-xs'>
                                  {log.pool_group_name ||
                                    `#${log.pool_group_id}`}{' '}
                                  · {log.pool_account_auth_type || '-'}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[180px] text-xs'>
                                {stateLogActionLabel(log.action, t)}
                                <div className='text-muted-foreground mt-1'>
                                  {t('Source')}: {log.source || '-'}
                                  {log.actor
                                    ? ` · ${t('Actor')}: ${log.actor}`
                                    : ''}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[180px] text-xs'>
                                {poolAccountStatusText(
                                  log.after_status,
                                  log.after_schedulable,
                                  log.after_unavailable,
                                  t
                                )}
                                <div className='text-muted-foreground mt-1 max-w-[240px] break-words'>
                                  {log.after_status_message ||
                                    log.after_disabled_reason ||
                                    '-'}
                                </div>
                              </TableCell>
                              <TableCell className='max-w-[320px] min-w-[220px] pr-4 text-xs break-words'>
                                {log.reason || '-'}
                              </TableCell>
                            </TableRow>
                          ))}
                          {!healthQuery.isLoading &&
                            (health?.recent_state_logs ?? []).length === 0 && (
                              <TableRow>
                                <TableCell
                                  colSpan={5}
                                  className='h-24 text-center'
                                >
                                  {t('No recent state changes found')}
                                </TableCell>
                              </TableRow>
                            )}
                        </TableBody>
                      </Table>
                    </div>
                  </CardContent>
                </Card>
              </TabsContent>
              <TabsContent value='accounts' className='m-0'>
                <Card size='sm'>
                  <CardHeader className='border-b'>
                    <CardTitle>{t('Account Groups')}</CardTitle>
                    <CardDescription>
                      {t(
                        'Configure pool groups, scheduling policies, and linked accounts.'
                      )}
                    </CardDescription>
                    <CardAction>
                      {groupsQuery.isLoading ? (
                        <Loader2
                          className='text-muted-foreground animate-spin'
                          aria-hidden='true'
                        />
                      ) : null}
                    </CardAction>
                  </CardHeader>
                  <CardContent>
                    {groupsQuery.isLoading ? (
                      <div className='text-muted-foreground rounded-md border p-6 text-center text-sm'>
                        {t('Loading')}
                      </div>
                    ) : null}
                    {!groupsQuery.isLoading && groups.length === 0 ? (
                      <EmptyState
                        icon={FileJson}
                        title={t('No account groups found')}
                        bordered
                      />
                    ) : null}
                    {!groupsQuery.isLoading && groups.length > 0 ? (
                      <Accordion
                        value={selectedGroupId ? [String(selectedGroupId)] : []}
                        onValueChange={(value) => {
                          const nextGroupId = value[0] ? Number(value[0]) : null
                          setSelectedGroupId(nextGroupId)
                          setPage(1)
                          setSelectedAccountIds([])
                        }}
                        className='gap-3'
                      >
                        {groups.map((group) => (
                          <AccordionItem
                            key={group.id}
                            value={String(group.id)}
                            className='rounded-md border px-3'
                          >
                            <AccordionTrigger className='gap-3 py-3 hover:no-underline'>
                              <span className='flex min-w-0 flex-1 flex-col gap-2'>
                                <span className='flex min-w-0 flex-wrap items-center gap-2'>
                                  <span className='truncate text-sm font-medium'>
                                    {group.name}
                                  </span>
                                  <StatusBadge
                                    label={groupStatusLabel(group, t)}
                                    variant={groupStatusVariant(group)}
                                    copyable={false}
                                  />
                                </span>
                                <span className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs font-normal'>
                                  <span>
                                    {group.platform} / {group.auth_type}
                                  </span>
                                  <span>
                                    {t('Available')}:{' '}
                                    {group.stats?.enabled ?? 0} /{' '}
                                    {group.stats?.total ?? 0}
                                  </span>
                                  {(group.stats?.disabled ?? 0) > 0 ? (
                                    <span>
                                      {t('Disabled')}:{' '}
                                      {group.stats?.disabled ?? 0}
                                    </span>
                                  ) : null}
                                  <span>
                                    {strategyLabel(group.strategy, t)}
                                  </span>
                                  <span className='max-w-full truncate'>
                                    {group.models || t('All Models')}
                                  </span>
                                </span>
                                <span
                                  className='text-muted-foreground truncate text-xs font-normal'
                                  title={`${groupAutoCheckSummary(
                                    group,
                                    t
                                  )} · ${groupPreflightCheckSummary(
                                    group,
                                    t
                                  )} · ${groupNoAvailableSummary(
                                    group,
                                    t
                                  )} · ${groupTaskLimitSummary(group, t)}`}
                                >
                                  {groupAutoCheckSummary(group, t)} ·{' '}
                                  {groupPreflightCheckSummary(group, t)} ·{' '}
                                  {groupNoAvailableSummary(group, t)} ·{' '}
                                  {groupTaskLimitSummary(group, t)}
                                </span>
                                {group.daily_limit_state?.limited ? (
                                  <span className='text-warning flex items-center gap-1 text-xs font-normal'>
                                    <AlertTriangle
                                      className='size-3.5 shrink-0'
                                      aria-hidden='true'
                                    />
                                    <span className='truncate'>
                                      {groupDailyLimitSummary(group, t)}
                                    </span>
                                  </span>
                                ) : null}
                              </span>
                            </AccordionTrigger>
                            <AccordionContent className='pb-3'>
                              {selectedGroupId === group.id ? (
                                <div className='flex min-w-0 flex-col overflow-hidden rounded-md border'>
                                  {selectedGroupDailyLimitTitle ? (
                                    <div className='border-warning/30 bg-warning/10 text-warning flex gap-2 border-b px-3 py-2 text-sm'>
                                      <AlertTriangle className='mt-0.5 size-4 shrink-0' />
                                      <div className='min-w-0'>
                                        <div className='font-medium'>
                                          {selectedGroupDailyLimitTitle}
                                        </div>
                                        <div className='text-xs'>
                                          {t(
                                            'Relay will stop selecting accounts from this group until the next daily reset.'
                                          )}
                                          {selectedGroup?.daily_limit_state
                                            ?.next_reset_time ? (
                                            <>
                                              {' '}
                                              {t('Next daily reset')}:&nbsp;
                                              {formatTimestamp(
                                                selectedGroup.daily_limit_state
                                                  .next_reset_time
                                              )}
                                            </>
                                          ) : null}
                                        </div>
                                      </div>
                                    </div>
                                  ) : null}
                                  <div className='border-border grid grid-cols-2 gap-3 border-b p-3 text-sm md:grid-cols-6'>
                                    <div>
                                      <div className='text-muted-foreground text-xs'>
                                        {t('Total')}
                                      </div>
                                      <div className='font-medium'>
                                        {stats?.total ?? 0}
                                      </div>
                                    </div>
                                    <div>
                                      <div className='text-muted-foreground text-xs'>
                                        {t('Available')}
                                      </div>
                                      <div className='font-medium'>
                                        {stats?.enabled ?? 0}
                                      </div>
                                    </div>
                                    <div>
                                      <div className='text-muted-foreground text-xs'>
                                        {t('Disabled')}
                                      </div>
                                      <div className='font-medium'>
                                        {stats?.disabled ?? 0}
                                      </div>
                                    </div>
                                    <div>
                                      <div className='text-muted-foreground text-xs'>
                                        {t('Cooldown')}
                                      </div>
                                      <div className='font-medium'>
                                        {stats?.cooldown ?? 0}
                                      </div>
                                    </div>
                                    <div>
                                      <div className='text-muted-foreground text-xs'>
                                        {t('Today requests')}
                                      </div>
                                      <div className='font-medium'>
                                        {formatUsageNumber(
                                          selectedGroup?.daily_request_count ??
                                            0
                                        )}
                                        {' / '}
                                        {formatLimitValue(
                                          selectedGroup?.daily_request_limit ??
                                            0,
                                          t
                                        )}
                                      </div>
                                    </div>
                                    <div>
                                      <div className='text-muted-foreground text-xs'>
                                        {t('Daily quota')}
                                      </div>
                                      <div className='font-medium'>
                                        {formatUsageNumber(
                                          selectedGroup?.daily_used_quota ?? 0
                                        )}
                                        {' / '}
                                        {formatLimitValue(
                                          selectedGroup?.daily_quota_limit ?? 0,
                                          t
                                        )}
                                      </div>
                                    </div>
                                  </div>

                                  <div className='border-border flex flex-col gap-2 border-b p-3 text-sm lg:flex-row lg:items-center lg:justify-between'>
                                    <span className='text-muted-foreground'>
                                      {t(
                                        'Manage accounts and credentials assigned to this group.'
                                      )}
                                    </span>
                                    <div className='flex flex-wrap gap-2'>
                                      <Button
                                        variant='outline'
                                        size='sm'
                                        disabled={
                                          batchChecking ||
                                          selectedGroupCheckTaskActive ||
                                          accountTotal <= 0 ||
                                          !canOperateAccountPool
                                        }
                                        onClick={() =>
                                          void checkSelectedGroupAccounts()
                                        }
                                      >
                                        {batchChecking ||
                                        selectedGroupCheckTaskActive ? (
                                          <Loader2
                                            data-icon='inline-start'
                                            className='animate-spin'
                                          />
                                        ) : (
                                          <Stethoscope data-icon='inline-start' />
                                        )}
                                        {t('Check Group')}
                                      </Button>
                                      <Button
                                        size='sm'
                                        disabled={!canSensitiveWriteAccountPool}
                                        onClick={openCreateAccount}
                                      >
                                        <Plus data-icon='inline-start' />
                                        {t('Add Account')}
                                      </Button>
                                      <DropdownMenu>
                                        <DropdownMenuTrigger
                                          render={
                                            <Button variant='outline' size='sm'>
                                              <MoreHorizontal data-icon='inline-start' />
                                              {t('More')}
                                            </Button>
                                          }
                                        />
                                        <DropdownMenuContent
                                          align='end'
                                          className='w-48'
                                        >
                                          <DropdownMenuGroup>
                                            <DropdownMenuItem
                                              disabled={
                                                !selectedGroupIsEditable
                                              }
                                              onClick={() =>
                                                openEditGroup(group)
                                              }
                                            >
                                              <Pencil />
                                              {t('Edit Group')}
                                            </DropdownMenuItem>
                                            <DropdownMenuItem
                                              disabled={
                                                !canSensitiveWriteAccountPool
                                              }
                                              onClick={() =>
                                                void deleteGroup(group)
                                              }
                                            >
                                              <Trash2 />
                                              {t('Delete')}
                                            </DropdownMenuItem>
                                            <DropdownMenuItem
                                              disabled={
                                                !canSensitiveWriteAccountPool
                                              }
                                              onClick={startCodexOAuth}
                                            >
                                              <ShieldCheck />
                                              {t('Codex OAuth')}
                                            </DropdownMenuItem>
                                            <DropdownMenuItem
                                              disabled={
                                                !canSensitiveWriteAccountPool
                                              }
                                              onClick={startCodexDevice}
                                            >
                                              <Smartphone />
                                              {t('Codex Device')}
                                            </DropdownMenuItem>
                                            <DropdownMenuItem
                                              disabled={
                                                !canSensitiveWriteAccountPool
                                              }
                                              onClick={() => setBatchOpen(true)}
                                            >
                                              <Upload />
                                              {t('Batch Import')}
                                            </DropdownMenuItem>
                                            <DropdownMenuItem
                                              disabled={
                                                actionLoading ||
                                                accountTotal <= 0 ||
                                                !canOperateAccountPool
                                              }
                                              onClick={() =>
                                                void exportAccounts()
                                              }
                                            >
                                              <Download />
                                              {t('Export Accounts')}
                                            </DropdownMenuItem>
                                          </DropdownMenuGroup>
                                        </DropdownMenuContent>
                                      </DropdownMenu>
                                    </div>
                                  </div>

                                  {selectedGroupCheckTask ? (
                                    <div className='border-border flex flex-col gap-2 border-b p-3 text-sm'>
                                      <div className='flex flex-wrap items-center justify-between gap-2'>
                                        <div className='flex flex-wrap items-center gap-2'>
                                          <span className='font-medium'>
                                            {t('Check task')}
                                          </span>
                                          <Badge
                                            variant={checkTaskBadgeVariant(
                                              selectedGroupCheckTask.status
                                            )}
                                          >
                                            {checkTaskStatusLabel(
                                              selectedGroupCheckTask.status,
                                              t
                                            )}
                                          </Badge>
                                          {checkTaskPolling ? (
                                            <Loader2 className='text-muted-foreground size-4 animate-spin' />
                                          ) : null}
                                        </div>
                                        <span className='text-muted-foreground text-xs'>
                                          {t('{{checked}}/{{total}} checked', {
                                            checked:
                                              selectedGroupCheckTask.checked +
                                              selectedGroupCheckTask.skipped,
                                            total: selectedGroupCheckTask.total,
                                          })}
                                        </span>
                                      </div>
                                      <Progress value={checkTaskProgress} />
                                      <div className='text-muted-foreground flex flex-wrap gap-3 text-xs'>
                                        <span>
                                          {t('{{success}} passed', {
                                            success:
                                              selectedGroupCheckTask.success,
                                          })}
                                        </span>
                                        <span>
                                          {t('{{failed}} failed', {
                                            failed:
                                              selectedGroupCheckTask.failed,
                                          })}
                                        </span>
                                        <span>
                                          {t('{{skipped}} skipped', {
                                            skipped:
                                              selectedGroupCheckTask.skipped,
                                          })}
                                        </span>
                                        {selectedGroupCheckTask.message ? (
                                          <span className='max-w-full truncate'>
                                            {selectedGroupCheckTask.message}
                                          </span>
                                        ) : null}
                                      </div>
                                    </div>
                                  ) : null}

                                  {accounts.length > 0 &&
                                  selectedAccountIds.length > 0 ? (
                                    <div className='border-border flex flex-col gap-2 border-b p-3 text-sm md:flex-row md:items-center md:justify-between'>
                                      <span className='text-muted-foreground'>
                                        {t('{{count}} account(s) selected', {
                                          count: selectedAccountIds.length,
                                        })}
                                      </span>
                                      <div className='flex flex-wrap gap-2'>
                                        <Button
                                          variant='outline'
                                          size='sm'
                                          disabled={
                                            actionLoading ||
                                            selectedAccountIds.length === 0 ||
                                            !canOperateAccountPool
                                          }
                                          onClick={() =>
                                            void batchUpdateSelectedAccountStatus(
                                              'enable'
                                            )
                                          }
                                        >
                                          <Power data-icon='inline-start' />
                                          {t('Enable selected accounts')}
                                        </Button>
                                        <Button
                                          variant='outline'
                                          size='sm'
                                          disabled={
                                            actionLoading ||
                                            selectedAccountIds.length === 0 ||
                                            !canOperateAccountPool
                                          }
                                          onClick={() =>
                                            void batchUpdateSelectedAccountStatus(
                                              'disable'
                                            )
                                          }
                                        >
                                          <PowerOff data-icon='inline-start' />
                                          {t('Disable selected accounts')}
                                        </Button>
                                        <Button
                                          variant='outline'
                                          size='sm'
                                          disabled={
                                            actionLoading ||
                                            selectedAccountIds.length === 0 ||
                                            !canOperateAccountPool
                                          }
                                          onClick={() =>
                                            void batchUpdateSelectedAccountStatus(
                                              'clear_cooldown'
                                            )
                                          }
                                        >
                                          <RefreshCw data-icon='inline-start' />
                                          {t('Clear cooldown')}
                                        </Button>
                                        <Button
                                          variant='outline'
                                          size='sm'
                                          disabled={
                                            actionLoading ||
                                            selectedAccountIds.length === 0 ||
                                            !canOperateAccountPool
                                          }
                                          onClick={() =>
                                            void exportAccounts(
                                              selectedAccountIds
                                            )
                                          }
                                        >
                                          <Download data-icon='inline-start' />
                                          {t('Export selected accounts')}
                                        </Button>
                                        <Button
                                          variant='outline'
                                          size='sm'
                                          disabled={
                                            actionLoading ||
                                            selectedAccountIds.length === 0 ||
                                            !canSensitiveWriteAccountPool
                                          }
                                          onClick={() =>
                                            void batchDeleteSelectedAccounts()
                                          }
                                        >
                                          <Trash2 data-icon='inline-start' />
                                          {t('Delete selected accounts')}
                                        </Button>
                                      </div>
                                    </div>
                                  ) : null}

                                  <div className='min-w-0'>
                                    <Table className='w-[1060px] min-w-[1060px] table-fixed'>
                                      <TableHeader>
                                        <TableRow>
                                          <TableHead className='w-11'>
                                            <Checkbox
                                              checked={
                                                allAccountsOnPageSelected
                                              }
                                              indeterminate={
                                                someAccountsOnPageSelected
                                              }
                                              onCheckedChange={(checked) =>
                                                toggleAllAccountsOnPage(
                                                  Boolean(checked)
                                                )
                                              }
                                              aria-label={t('Select all')}
                                            />
                                          </TableHead>
                                          <TableHead className='w-[368px]'>
                                            {t('Account')}
                                          </TableHead>
                                          <TableHead className='w-[136px]'>
                                            {t('Status')}
                                          </TableHead>
                                          <TableHead className='w-[188px]'>
                                            {t('Usage')}
                                          </TableHead>
                                          <TableHead className='w-[212px]'>
                                            {t('Last Used')}
                                          </TableHead>
                                          <TableHead className='w-[112px] text-right'>
                                            {t('Actions')}
                                          </TableHead>
                                        </TableRow>
                                      </TableHeader>
                                      <TableBody>
                                        {accounts.map((account) => {
                                          const fullCredentialSummary =
                                            formatCredentialSummary(
                                              account.credential_summary
                                            )
                                          const accountIdentity =
                                            formatAccountIdentity(
                                              account.credential_summary,
                                              account.name
                                            )
                                          const accountFileLabel =
                                            poolAccountFileLabel(account)
                                          const statusReason =
                                            visibleAccountStatusReason(
                                              account,
                                              nowSeconds,
                                              t
                                            )
                                          const accountEnabled =
                                            account.status ===
                                              CHANNEL_STATUS.ENABLED &&
                                            account.schedulable

                                          return (
                                            <TableRow key={account.id}>
                                              <TableCell>
                                                <Checkbox
                                                  checked={selectedAccountIds.includes(
                                                    account.id
                                                  )}
                                                  onCheckedChange={(checked) =>
                                                    toggleAccountSelection(
                                                      account.id,
                                                      Boolean(checked)
                                                    )
                                                  }
                                                  aria-label={t('Select row')}
                                                />
                                              </TableCell>
                                              <TableCell className='min-w-0'>
                                                <div
                                                  className='truncate font-medium'
                                                  title={accountRowTitle(
                                                    account,
                                                    fullCredentialSummary,
                                                    t
                                                  )}
                                                >
                                                  {accountIdentity}
                                                </div>
                                                <div
                                                  className='text-muted-foreground truncate text-xs'
                                                  title={`${t('File')}: ${accountFileLabel}`}
                                                >
                                                  {t('File')}:{' '}
                                                  {accountFileLabel}
                                                </div>
                                                {account.models ? (
                                                  <div className='text-muted-foreground truncate text-xs'>
                                                    {t('Models')}:{' '}
                                                    {account.models}
                                                  </div>
                                                ) : null}
                                              </TableCell>
                                              <TableCell className='min-w-0'>
                                                <div className='flex flex-col gap-1'>
                                                  <StatusBadge
                                                    label={statusLabel(
                                                      account,
                                                      nowSeconds,
                                                      t
                                                    )}
                                                    variant={statusVariant(
                                                      account,
                                                      nowSeconds
                                                    )}
                                                    copyable={false}
                                                  />
                                                  {statusReason ? (
                                                    <span
                                                      className='text-muted-foreground max-w-full truncate text-xs'
                                                      title={statusReason}
                                                    >
                                                      {limitInlineText(
                                                        statusReason,
                                                        80
                                                      )}
                                                    </span>
                                                  ) : null}
                                                </div>
                                              </TableCell>
                                              <TableCell
                                                className='min-w-0 text-xs'
                                                title={[
                                                  `${t('Daily requests')}: ${formatUsageNumber(
                                                    account.daily_request_count
                                                  )} / ${formatLimitValue(
                                                    account.daily_request_limit,
                                                    t
                                                  )}`,
                                                  `${t('Max concurrency')}: ${formatLimitValue(
                                                    account.max_concurrency,
                                                    t
                                                  )}`,
                                                  `${t('RPM')}: ${formatLimitValue(
                                                    account.rate_limit_rpm,
                                                    t
                                                  )}`,
                                                  `${t('Daily quota')}: ${formatUsageNumber(
                                                    account.daily_used_quota
                                                  )} / ${formatLimitValue(
                                                    account.daily_quota_limit,
                                                    t
                                                  )}`,
                                                ].join('\n')}
                                              >
                                                <div className='truncate'>
                                                  {t('Request')}:&nbsp;
                                                  {formatUsageNumber(
                                                    account.daily_request_count
                                                  )}{' '}
                                                  /{' '}
                                                  {formatLimitValue(
                                                    account.daily_request_limit,
                                                    t
                                                  )}
                                                </div>
                                                <div className='text-muted-foreground mt-1 truncate'>
                                                  {t('Success')}:{' '}
                                                  {account.success_count ?? 0} ·{' '}
                                                  {t('Failed')}:{' '}
                                                  {account.failed_count ?? 0}
                                                </div>
                                              </TableCell>
                                              <TableCell
                                                className='min-w-0 text-xs'
                                                title={[
                                                  `${t('Last Used')}: ${
                                                    account.last_used_time
                                                      ? formatTimestamp(
                                                          account.last_used_time
                                                        )
                                                      : '-'
                                                  }`,
                                                  `${t('Last check time')}: ${
                                                    account.last_checked_time
                                                      ? formatTimestamp(
                                                          account.last_checked_time
                                                        )
                                                      : '-'
                                                  }`,
                                                  account.next_refresh_time
                                                    ? `${t('Next refresh')}: ${formatTimestamp(
                                                        account.next_refresh_time
                                                      )}`
                                                    : '',
                                                ]
                                                  .filter(Boolean)
                                                  .join('\n')}
                                              >
                                                <div className='truncate'>
                                                  {account.last_used_time
                                                    ? formatTimestamp(
                                                        account.last_used_time
                                                      )
                                                    : '-'}
                                                </div>
                                                <div className='text-muted-foreground mt-1 truncate'>
                                                  {t('Last check time')}:&nbsp;
                                                  {account.last_checked_time
                                                    ? formatTimestamp(
                                                        account.last_checked_time
                                                      )
                                                    : '-'}
                                                </div>
                                                {account.next_refresh_time ? (
                                                  <div
                                                    className='text-muted-foreground mt-1 truncate'
                                                    title={`${t('Next refresh')}: ${formatTimestamp(
                                                      account.next_refresh_time
                                                    )}`}
                                                  >
                                                    {t('Next refresh')}:&nbsp;
                                                    {formatTimestamp(
                                                      account.next_refresh_time
                                                    )}
                                                  </div>
                                                ) : null}
                                              </TableCell>
                                              <TableCell className='w-[112px]'>
                                                <div className='flex flex-nowrap justify-end gap-1.5'>
                                                  <Button
                                                    variant='ghost'
                                                    size='icon-sm'
                                                    aria-label={t(
                                                      'Check Account'
                                                    )}
                                                    title={t('Check Account')}
                                                    disabled={
                                                      checkingAccountId ===
                                                        account.id ||
                                                      !canOperateAccountPool
                                                    }
                                                    onClick={() =>
                                                      void checkAccount(account)
                                                    }
                                                  >
                                                    {checkingAccountId ===
                                                    account.id ? (
                                                      <Loader2 className='animate-spin' />
                                                    ) : (
                                                      <Stethoscope />
                                                    )}
                                                  </Button>
                                                  <Button
                                                    variant='ghost'
                                                    size='icon-sm'
                                                    aria-label={
                                                      accountEnabled
                                                        ? t('Disable')
                                                        : t('Enable')
                                                    }
                                                    title={
                                                      accountEnabled
                                                        ? t('Disable')
                                                        : t('Enable')
                                                    }
                                                    disabled={
                                                      !canOperateAccountPool
                                                    }
                                                    onClick={() =>
                                                      void setAccountEnabled(
                                                        account,
                                                        !accountEnabled
                                                      )
                                                    }
                                                  >
                                                    {accountEnabled ? (
                                                      <PowerOff />
                                                    ) : (
                                                      <Power />
                                                    )}
                                                  </Button>
                                                  <DropdownMenu>
                                                    <DropdownMenuTrigger
                                                      render={
                                                        <Button
                                                          variant='ghost'
                                                          size='icon-sm'
                                                          aria-label={t('More')}
                                                          title={t('More')}
                                                        >
                                                          <MoreHorizontal />
                                                        </Button>
                                                      }
                                                    />
                                                    <DropdownMenuContent
                                                      align='end'
                                                      className='w-44'
                                                    >
                                                      <DropdownMenuGroup>
                                                        <DropdownMenuItem
                                                          disabled={
                                                            !canSensitiveWriteAccountPool
                                                          }
                                                          onClick={() =>
                                                            openEditAccount(
                                                              account
                                                            )
                                                          }
                                                        >
                                                          <Pencil />
                                                          {t('Edit')}
                                                        </DropdownMenuItem>
                                                        <DropdownMenuItem
                                                          disabled={
                                                            !canOperateAccountPool
                                                          }
                                                          onClick={() =>
                                                            void clearCooldown(
                                                              account
                                                            )
                                                          }
                                                        >
                                                          <RefreshCw />
                                                          {t('Clear cooldown')}
                                                        </DropdownMenuItem>
                                                        <DropdownMenuItem
                                                          disabled={
                                                            !canOperateAccountPool
                                                          }
                                                          onClick={() =>
                                                            void resetRuntime(
                                                              account
                                                            )
                                                          }
                                                        >
                                                          <RotateCcw />
                                                          {t('Reset runtime')}
                                                        </DropdownMenuItem>
                                                        {account.platform ===
                                                          'codex' &&
                                                          account.auth_type ===
                                                            'official_oauth' && (
                                                            <DropdownMenuItem
                                                              disabled={
                                                                !canSensitiveWriteAccountPool
                                                              }
                                                              onClick={() =>
                                                                void refreshCredential(
                                                                  account
                                                                )
                                                              }
                                                            >
                                                              <ShieldCheck />
                                                              {t(
                                                                'Refresh credential'
                                                              )}
                                                            </DropdownMenuItem>
                                                          )}
                                                        <DropdownMenuItem
                                                          variant='destructive'
                                                          disabled={
                                                            !canSensitiveWriteAccountPool
                                                          }
                                                          onClick={() =>
                                                            void deleteAccount(
                                                              account
                                                            )
                                                          }
                                                        >
                                                          <Trash2 />
                                                          {t('Delete')}
                                                        </DropdownMenuItem>
                                                      </DropdownMenuGroup>
                                                    </DropdownMenuContent>
                                                  </DropdownMenu>
                                                </div>
                                              </TableCell>
                                            </TableRow>
                                          )
                                        })}
                                        {!accountsQuery.isLoading &&
                                          accounts.length === 0 && (
                                            <TableRow>
                                              <TableCell
                                                colSpan={6}
                                                className='h-24 text-center'
                                              >
                                                {t('No accounts found')}
                                              </TableCell>
                                            </TableRow>
                                          )}
                                      </TableBody>
                                    </Table>
                                  </div>
                                  <div className='border-border flex items-center justify-between border-t p-3 text-sm'>
                                    <span className='text-muted-foreground'>
                                      {t('Page {{page}} of {{total}}', {
                                        page,
                                        total: totalPages,
                                      })}
                                    </span>
                                    <div className='flex gap-2'>
                                      <Button
                                        variant='outline'
                                        size='sm'
                                        disabled={page <= 1}
                                        onClick={() =>
                                          setPage((current) =>
                                            Math.max(1, current - 1)
                                          )
                                        }
                                      >
                                        {t('Previous')}
                                      </Button>
                                      <Button
                                        variant='outline'
                                        size='sm'
                                        disabled={page >= totalPages}
                                        onClick={() =>
                                          setPage((current) =>
                                            Math.min(totalPages, current + 1)
                                          )
                                        }
                                      >
                                        {t('Next')}
                                      </Button>
                                    </div>
                                  </div>
                                </div>
                              ) : null}
                            </AccordionContent>
                          </AccordionItem>
                        ))}
                      </Accordion>
                    ) : null}
                  </CardContent>
                </Card>
              </TabsContent>
              <TabsContent value='auth-files' className='m-0'>
                <Card size='sm'>
                  <CardHeader className='border-b'>
                    <CardTitle>{t('Account Credentials')}</CardTitle>
                    <CardDescription>
                      {t(
                        'Manage imported account credentials as reusable pool resources.'
                      )}
                    </CardDescription>
                  </CardHeader>
                  <CardContent className='p-0'>
                    <AuthFilesPanel
                      groups={groups}
                      canRead={canReadAccountPoolAuthFile}
                      canSensitiveWrite={canSensitiveWriteAccountPoolAuthFile}
                    />
                  </CardContent>
                </Card>
              </TabsContent>
              <TabsContent value='usage-logs' className='m-0'>
                <Card size='sm'>
                  <CardHeader className='border-b'>
                    <CardTitle>{t('Account Records')}</CardTitle>
                    <CardDescription>
                      {t(
                        'Inspect usage records, state changes, and check tasks.'
                      )}
                    </CardDescription>
                    <CardAction>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => void usageLogsQuery.refetch()}
                      >
                        <RefreshCw data-icon='inline-start' />
                        {t('Refresh')}
                      </Button>
                    </CardAction>
                  </CardHeader>
                  <CardContent className='p-0'>
                    {logViewTabs}
                    <div className='border-border flex flex-col gap-2 border-b p-3 md:hidden'>
                      <div className='flex min-w-0 items-center gap-2'>
                        <Input
                          className='min-w-0 flex-1'
                          placeholder={t(
                            'Search account, channel, model, user, or error'
                          )}
                          value={usageLogSearch}
                          onChange={(event) =>
                            setUsageLogSearch(event.target.value)
                          }
                        />
                        <AccountPoolHistoryFilterDrawer
                          kind='usage'
                          activeCount={usageLogFilterCount}
                          resetDisabled={usageLogFilterCount === 0}
                          onReset={clearUsageLogFilters}
                        >
                          <div className='grid grid-cols-3 gap-2'>
                            <Button
                              variant={
                                usageLogStatus === 'all'
                                  ? 'secondary'
                                  : 'outline'
                              }
                              size='sm'
                              onClick={() => setUsageLogStatus('all')}
                            >
                              {t('All')}
                            </Button>
                            <Button
                              variant={
                                usageLogStatus === 'success'
                                  ? 'secondary'
                                  : 'outline'
                              }
                              size='sm'
                              onClick={() => setUsageLogStatus('success')}
                            >
                              {t('Success')}
                            </Button>
                            <Button
                              variant={
                                usageLogStatus === 'failed'
                                  ? 'secondary'
                                  : 'outline'
                              }
                              size='sm'
                              onClick={() => setUsageLogStatus('failed')}
                            >
                              {t('Failed')}
                            </Button>
                          </div>
                        </AccountPoolHistoryFilterDrawer>
                      </div>
                    </div>
                    <div className='border-border hidden flex-col gap-3 border-b p-3 md:flex md:flex-row md:items-center md:justify-between'>
                      <div className='flex flex-wrap gap-2'>
                        <Button
                          variant={
                            usageLogStatus === 'all' ? 'secondary' : 'outline'
                          }
                          size='sm'
                          onClick={() => setUsageLogStatus('all')}
                        >
                          {t('All')}
                        </Button>
                        <Button
                          variant={
                            usageLogStatus === 'success'
                              ? 'secondary'
                              : 'outline'
                          }
                          size='sm'
                          onClick={() => setUsageLogStatus('success')}
                        >
                          {t('Success')}
                        </Button>
                        <Button
                          variant={
                            usageLogStatus === 'failed'
                              ? 'secondary'
                              : 'outline'
                          }
                          size='sm'
                          onClick={() => setUsageLogStatus('failed')}
                        >
                          {t('Failed')}
                        </Button>
                      </div>
                      <Input
                        className='md:max-w-xs'
                        placeholder={t(
                          'Search account, channel, model, user, or error'
                        )}
                        value={usageLogSearch}
                        onChange={(event) =>
                          setUsageLogSearch(event.target.value)
                        }
                      />
                    </div>
                    <AccountPoolUsageLogsMobileList
                      items={usageLogs}
                      isLoading={usageLogsQuery.isLoading}
                      emptyTitle={t('No usage logs found')}
                      onFilterRequest={filterStateLogsByRequest}
                      onFilterAccount={filterStateLogsByAccount}
                    />
                    <div className='hidden overflow-x-auto md:block'>
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>{t('Time')}</TableHead>
                            <TableHead>{t('Account')}</TableHead>
                            <TableHead>{t('Channel')}</TableHead>
                            <TableHead>{t('Model')}</TableHead>
                            <TableHead>{t('Usage')}</TableHead>
                            <TableHead>{t('Result')}</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {usageLogs.map((log) => (
                            <TableRow key={log.id}>
                              <TableCell className='min-w-[150px] text-xs'>
                                {formatTimestamp(log.created_at)}
                                {log.request_id ? (
                                  <Button
                                    variant='link'
                                    size='xs'
                                    className='text-muted-foreground mt-1 h-auto max-w-[150px] justify-start truncate p-0 text-xs'
                                    title={t('Click to filter by request')}
                                    onClick={() =>
                                      filterStateLogsByRequest(
                                        log.request_id ?? ''
                                      )
                                    }
                                  >
                                    {log.request_id}
                                  </Button>
                                ) : (
                                  <div className='text-muted-foreground mt-1'>
                                    -
                                  </div>
                                )}
                              </TableCell>
                              <TableCell className='min-w-[190px]'>
                                {log.pool_account_id > 0 ? (
                                  <Button
                                    variant='link'
                                    size='sm'
                                    className='h-auto max-w-[190px] justify-start truncate p-0 text-sm font-medium'
                                    title={t('Click to filter by account')}
                                    onClick={() =>
                                      filterStateLogsByAccount(
                                        log.pool_account_id,
                                        log.pool_account_name ||
                                          `#${log.pool_account_id}`
                                      )
                                    }
                                  >
                                    {log.pool_account_name ||
                                      `#${log.pool_account_id}`}
                                  </Button>
                                ) : (
                                  <div className='text-sm font-medium'>
                                    {log.pool_account_name ||
                                      `#${log.pool_account_id}`}
                                  </div>
                                )}
                                <div className='text-muted-foreground text-xs'>
                                  {log.pool_group_name ||
                                    `#${log.pool_group_id}`}{' '}
                                  · {log.pool_account_auth_type || '-'}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[160px]'>
                                <div className='text-sm'>
                                  {log.channel_name || `#${log.channel_id}`}
                                </div>
                                <div className='text-muted-foreground text-xs'>
                                  {log.username || '-'} /{' '}
                                  {log.token_name || '-'}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[160px] text-xs'>
                                {log.model_name || '-'}
                                {log.group ? (
                                  <div className='text-muted-foreground mt-1'>
                                    {t('Group')}: {log.group}
                                  </div>
                                ) : null}
                              </TableCell>
                              <TableCell className='min-w-[160px] text-xs'>
                                {t('Quota')}: {formatUsageNumber(log.quota)}
                                <div className='text-muted-foreground mt-1'>
                                  {t('Tokens')}:&nbsp;
                                  {formatUsageNumber(
                                    log.prompt_tokens + log.completion_tokens
                                  )}
                                  &nbsp;· {formatUsageDuration(log.use_time)}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[200px]'>
                                <div className='flex flex-col gap-1'>
                                  <StatusBadge
                                    label={
                                      log.success ? t('Success') : t('Failed')
                                    }
                                    variant={log.success ? 'success' : 'danger'}
                                    copyable={false}
                                  />
                                  {!log.success && (
                                    <div className='text-muted-foreground max-w-[260px] text-xs break-words'>
                                      {log.status_code
                                        ? `${log.status_code} · `
                                        : ''}
                                      {log.error_message ||
                                        log.error_code ||
                                        '-'}
                                    </div>
                                  )}
                                  {log.retry_index > 0 && (
                                    <div className='text-muted-foreground text-xs'>
                                      {t('Retry')}: {log.retry_index}
                                    </div>
                                  )}
                                </div>
                              </TableCell>
                            </TableRow>
                          ))}
                          {!usageLogsQuery.isLoading &&
                            usageLogs.length === 0 && (
                              <TableRow>
                                <TableCell
                                  colSpan={6}
                                  className='h-24 text-center'
                                >
                                  {t('No usage logs found')}
                                </TableCell>
                              </TableRow>
                            )}
                        </TableBody>
                      </Table>
                    </div>
                    <div className='border-border flex items-center justify-between border-t p-3 text-sm'>
                      <span className='text-muted-foreground'>
                        {t('Page {{page}} of {{total}}', {
                          page: usageLogPage,
                          total: usageLogTotalPages,
                        })}
                      </span>
                      <div className='flex gap-2'>
                        <Button
                          variant='outline'
                          size='sm'
                          disabled={usageLogPage <= 1}
                          onClick={() =>
                            setUsageLogPage((current) =>
                              Math.max(1, current - 1)
                            )
                          }
                        >
                          {t('Previous')}
                        </Button>
                        <Button
                          variant='outline'
                          size='sm'
                          disabled={usageLogPage >= usageLogTotalPages}
                          onClick={() =>
                            setUsageLogPage((current) =>
                              Math.min(usageLogTotalPages, current + 1)
                            )
                          }
                        >
                          {t('Next')}
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              </TabsContent>
              <TabsContent value='state-logs' className='m-0'>
                <Card size='sm'>
                  <CardHeader className='border-b'>
                    <CardTitle>{t('Account Records')}</CardTitle>
                    <CardDescription>
                      {t(
                        'Inspect usage records, state changes, and check tasks.'
                      )}
                    </CardDescription>
                    <CardAction className='flex flex-wrap justify-start gap-2 lg:justify-end'>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => {
                          void stateLogsQuery.refetch()
                          void stateLogAuditQuery.refetch()
                        }}
                      >
                        <RefreshCw data-icon='inline-start' />
                        {t('Refresh')}
                      </Button>
                      <Button
                        variant='outline'
                        size='sm'
                        disabled={stateLogExporting || !canOperateAccountPool}
                        onClick={() => void exportStateLogs()}
                      >
                        {stateLogExporting ? (
                          <Loader2
                            data-icon='inline-start'
                            className='animate-spin'
                          />
                        ) : (
                          <Download data-icon='inline-start' />
                        )}
                        {t('Export audit')}
                      </Button>
                    </CardAction>
                  </CardHeader>
                  <CardContent className='p-0'>
                    {logViewTabs}
                    <div className='border-border flex flex-col gap-2 border-b p-3 md:hidden'>
                      <div className='flex min-w-0 items-center gap-2'>
                        <Input
                          className='min-w-0 flex-1'
                          placeholder={t(
                            'Search account, action, source, actor, or reason'
                          )}
                          value={stateLogSearch}
                          onChange={(event) =>
                            setStateLogSearch(event.target.value)
                          }
                        />
                        <AccountPoolHistoryFilterDrawer
                          kind='state'
                          activeCount={stateLogFilterCount}
                          resetDisabled={!hasStateLogFilters}
                          onReset={clearStateLogFilters}
                        >
                          <NativeSelect
                            className='w-full'
                            value={stateLogAction}
                            onChange={(event) =>
                              setStateLogAction(
                                event.target.value as StateLogActionFilter
                              )
                            }
                          >
                            {stateLogActionFilterOptions.map((value) => (
                              <NativeSelectOption key={value} value={value}>
                                {stateLogActionFilterLabel(value, t)}
                              </NativeSelectOption>
                            ))}
                          </NativeSelect>
                          <NativeSelect
                            className='w-full'
                            value={stateLogSource}
                            onChange={(event) =>
                              setStateLogSource(
                                event.target.value as StateLogSourceFilter
                              )
                            }
                          >
                            {stateLogSourceFilterOptions.map((value) => (
                              <NativeSelectOption key={value} value={value}>
                                {stateLogSourceFilterLabel(value, t)}
                              </NativeSelectOption>
                            ))}
                          </NativeSelect>
                          <Input
                            placeholder={t('Request ID')}
                            value={stateLogRequestId}
                            onChange={(event) =>
                              setStateLogRequestId(event.target.value)
                            }
                          />
                          <label className='flex min-w-0 flex-col gap-1'>
                            <span className='text-muted-foreground text-xs'>
                              {t('Start time')}
                            </span>
                            <Input
                              type='datetime-local'
                              value={stateLogStartTime}
                              onChange={(event) =>
                                setStateLogStartTime(event.target.value)
                              }
                            />
                          </label>
                          <label className='flex min-w-0 flex-col gap-1'>
                            <span className='text-muted-foreground text-xs'>
                              {t('End time')}
                            </span>
                            <Input
                              type='datetime-local'
                              value={stateLogEndTime}
                              onChange={(event) =>
                                setStateLogEndTime(event.target.value)
                              }
                            />
                          </label>
                          {stateLogAccountId ? (
                            <div className='flex min-w-0 items-center gap-2'>
                              <Badge
                                variant='secondary'
                                className='max-w-full gap-1 overflow-hidden'
                              >
                                <span className='shrink-0'>
                                  {t('Account filter')}:
                                </span>
                                <span className='truncate'>
                                  {stateLogAccountLabel ||
                                    `#${stateLogAccountId}`}
                                </span>
                              </Badge>
                              <Button
                                variant='ghost'
                                size='icon-xs'
                                title={t('Clear filters')}
                                onClick={() => {
                                  setStateLogAccountId(null)
                                  setStateLogAccountLabel('')
                                  setStateLogPage(1)
                                }}
                              >
                                <X aria-hidden='true' />
                              </Button>
                            </div>
                          ) : null}
                        </AccountPoolHistoryFilterDrawer>
                      </div>
                    </div>
                    <div className='border-border hidden flex-col gap-3 border-b p-3 md:flex'>
                      <div className='grid gap-2 lg:grid-cols-[220px_200px_minmax(240px,1fr)]'>
                        <Select
                          items={stateLogActionFilterOptions.map((value) => ({
                            value,
                            label: stateLogActionFilterLabel(value, t),
                          }))}
                          value={stateLogAction}
                          onValueChange={(value) =>
                            setStateLogAction(
                              (value as StateLogActionFilter | null) ?? 'all'
                            )
                          }
                        >
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent alignItemWithTrigger={false}>
                            <SelectGroup>
                              {stateLogActionFilterOptions.map((value) => (
                                <SelectItem key={value} value={value}>
                                  {stateLogActionFilterLabel(value, t)}
                                </SelectItem>
                              ))}
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                        <Select
                          items={stateLogSourceFilterOptions.map((value) => ({
                            value,
                            label: stateLogSourceFilterLabel(value, t),
                          }))}
                          value={stateLogSource}
                          onValueChange={(value) =>
                            setStateLogSource(
                              (value as StateLogSourceFilter | null) ?? 'all'
                            )
                          }
                        >
                          <SelectTrigger className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent alignItemWithTrigger={false}>
                            <SelectGroup>
                              {stateLogSourceFilterOptions.map((value) => (
                                <SelectItem key={value} value={value}>
                                  {stateLogSourceFilterLabel(value, t)}
                                </SelectItem>
                              ))}
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                        <Input
                          placeholder={t(
                            'Search account, action, source, actor, or reason'
                          )}
                          value={stateLogSearch}
                          onChange={(event) =>
                            setStateLogSearch(event.target.value)
                          }
                        />
                      </div>
                      <div className='grid gap-2 lg:grid-cols-[minmax(180px,1fr)_180px_180px_auto]'>
                        <Input
                          placeholder={t('Request ID')}
                          value={stateLogRequestId}
                          onChange={(event) =>
                            setStateLogRequestId(event.target.value)
                          }
                        />
                        <label className='flex min-w-0 flex-col gap-1'>
                          <span className='text-muted-foreground text-xs'>
                            {t('Start time')}
                          </span>
                          <Input
                            type='datetime-local'
                            value={stateLogStartTime}
                            onChange={(event) =>
                              setStateLogStartTime(event.target.value)
                            }
                          />
                        </label>
                        <label className='flex min-w-0 flex-col gap-1'>
                          <span className='text-muted-foreground text-xs'>
                            {t('End time')}
                          </span>
                          <Input
                            type='datetime-local'
                            value={stateLogEndTime}
                            onChange={(event) =>
                              setStateLogEndTime(event.target.value)
                            }
                          />
                        </label>
                        <Button
                          variant='outline'
                          size='sm'
                          className='self-end'
                          disabled={!hasStateLogFilters}
                          onClick={clearStateLogFilters}
                        >
                          <FilterX data-icon='inline-start' />
                          {t('Clear filters')}
                        </Button>
                      </div>
                      {stateLogAccountId ? (
                        <div className='flex min-w-0 items-center gap-2'>
                          <Badge
                            variant='secondary'
                            className='max-w-full gap-1 overflow-hidden'
                          >
                            <span className='shrink-0'>
                              {t('Account filter')}:
                            </span>
                            <span className='truncate'>
                              {stateLogAccountLabel || `#${stateLogAccountId}`}
                            </span>
                          </Badge>
                          <Button
                            variant='ghost'
                            size='icon-xs'
                            title={t('Clear filters')}
                            onClick={() => {
                              setStateLogAccountId(null)
                              setStateLogAccountLabel('')
                              setStateLogPage(1)
                            }}
                          >
                            <X aria-hidden='true' />
                          </Button>
                        </div>
                      ) : null}
                    </div>
                    <div className='border-border grid grid-cols-2 gap-3 border-b p-3 text-sm md:grid-cols-4'>
                      {[
                        {
                          label: t('Audit logs'),
                          value: formatUsageNumber(
                            stateLogAuditSummary?.total ?? 0
                          ),
                        },
                        {
                          label: t('Manual changes'),
                          value: formatUsageNumber(
                            stateLogAuditSummary?.manual_total ?? 0
                          ),
                        },
                        {
                          label: t('Automatic changes'),
                          value: formatUsageNumber(
                            stateLogAuditSummary?.automatic_total ?? 0
                          ),
                        },
                        {
                          label: t('Affected accounts'),
                          value: formatUsageNumber(
                            stateLogAuditSummary?.affected_accounts ?? 0
                          ),
                        },
                      ].map((metric) => (
                        <div key={metric.label} className='min-w-0'>
                          <div className='text-muted-foreground truncate text-xs'>
                            {metric.label}
                          </div>
                          <div className='truncate font-medium'>
                            {metric.value}
                          </div>
                        </div>
                      ))}
                    </div>
                    <div className='border-border grid grid-cols-1 border-b lg:grid-cols-3'>
                      <div className='border-border flex flex-col gap-2 border-b p-3 lg:border-r lg:border-b-0'>
                        <div className='text-sm font-medium'>
                          {t('Action summary')}
                        </div>
                        {(stateLogAuditSummary?.action_stats ?? [])
                          .slice(0, 5)
                          .map((bucket) => (
                            <div
                              key={bucket.key || 'unknown-action'}
                              className='flex items-center justify-between gap-2 text-xs'
                            >
                              <div className='min-w-0'>
                                <div className='truncate font-medium'>
                                  {stateLogActionLabel(bucket.key, t)}
                                </div>
                                <div className='text-muted-foreground'>
                                  {bucket.latest_at
                                    ? formatTimestamp(bucket.latest_at)
                                    : '-'}
                                </div>
                              </div>
                              <Badge variant='secondary'>
                                {formatUsageNumber(bucket.total)}
                              </Badge>
                            </div>
                          ))}
                        {!stateLogAuditQuery.isLoading &&
                          (stateLogAuditSummary?.action_stats ?? []).length ===
                            0 && (
                            <div className='text-muted-foreground text-xs'>
                              {t('No audit summary yet')}
                            </div>
                          )}
                      </div>
                      <div className='border-border flex flex-col gap-2 border-b p-3 lg:border-r lg:border-b-0'>
                        <div className='text-sm font-medium'>
                          {t('Source summary')}
                        </div>
                        {(stateLogAuditSummary?.source_stats ?? [])
                          .slice(0, 5)
                          .map((bucket) => (
                            <div
                              key={bucket.key || 'unknown-source'}
                              className='flex items-center justify-between gap-2 text-xs'
                            >
                              <div className='min-w-0'>
                                <div className='truncate font-medium'>
                                  {stateLogSourceLabel(bucket.key, t)}
                                </div>
                                <div className='text-muted-foreground'>
                                  {bucket.latest_at
                                    ? formatTimestamp(bucket.latest_at)
                                    : '-'}
                                </div>
                              </div>
                              <Badge variant='secondary'>
                                {formatUsageNumber(bucket.total)}
                              </Badge>
                            </div>
                          ))}
                        {!stateLogAuditQuery.isLoading &&
                          (stateLogAuditSummary?.source_stats ?? []).length ===
                            0 && (
                            <div className='text-muted-foreground text-xs'>
                              {t('No audit summary yet')}
                            </div>
                          )}
                      </div>
                      <div className='flex flex-col gap-2 p-3'>
                        <div className='text-sm font-medium'>
                          {t('Recent bulk operations')}
                        </div>
                        {(stateLogAuditSummary?.recent_bulk_operations ?? [])
                          .slice(0, 3)
                          .map((operation, index) => (
                            <div
                              key={`${operation.request_id || operation.last_at}-${index}`}
                              className='border-border flex flex-col gap-1 border-t pt-2 text-xs first:border-t-0 first:pt-0'
                            >
                              <div className='flex items-center justify-between gap-2'>
                                <span className='min-w-0 truncate font-medium'>
                                  {stateLogActionLabel(operation.action, t)}
                                </span>
                                <Badge variant='secondary'>
                                  {t('{{count}} accounts affected', {
                                    count: operation.account_count,
                                  })}
                                </Badge>
                              </div>
                              <div className='text-muted-foreground truncate'>
                                {formatTimestamp(operation.last_at)} ·{' '}
                                {stateLogSourceLabel(operation.source, t)}
                                {operation.actor
                                  ? ` · ${t('Actor')}: ${operation.actor}`
                                  : ''}
                              </div>
                              <div className='text-muted-foreground break-words'>
                                {bulkAuditSampleText(operation, t)}
                              </div>
                              {operation.request_id ? (
                                <div className='text-muted-foreground truncate'>
                                  {t('Request')}: {operation.request_id}
                                </div>
                              ) : null}
                            </div>
                          ))}
                        {!stateLogAuditQuery.isLoading &&
                          (stateLogAuditSummary?.recent_bulk_operations ?? [])
                            .length === 0 && (
                            <div className='text-muted-foreground text-xs'>
                              {t('No bulk operations found')}
                            </div>
                          )}
                      </div>
                    </div>
                    <AccountPoolStateLogsMobileList
                      items={stateLogs}
                      isLoading={stateLogsQuery.isLoading}
                      emptyTitle={t('No state logs found')}
                      onFilterRequest={filterStateLogsByRequest}
                      onFilterAccount={filterStateLogsByAccount}
                    />
                    <div className='hidden overflow-x-auto md:block'>
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>{t('Time')}</TableHead>
                            <TableHead>{t('Account')}</TableHead>
                            <TableHead>{t('Action')}</TableHead>
                            <TableHead>{t('Before state')}</TableHead>
                            <TableHead>{t('After state')}</TableHead>
                            <TableHead>{t('Reason')}</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {stateLogs.map((log) => (
                            <TableRow key={log.id}>
                              <TableCell className='min-w-[150px] text-xs'>
                                {formatTimestamp(log.created_at)}
                                {log.request_id ? (
                                  <Button
                                    variant='link'
                                    size='xs'
                                    className='text-muted-foreground mt-1 h-auto max-w-[150px] justify-start truncate p-0 text-xs'
                                    title={t('Click to filter by request')}
                                    onClick={() =>
                                      filterStateLogsByRequest(
                                        log.request_id ?? ''
                                      )
                                    }
                                  >
                                    {log.request_id}
                                  </Button>
                                ) : (
                                  <div className='text-muted-foreground mt-1'>
                                    -
                                  </div>
                                )}
                              </TableCell>
                              <TableCell className='min-w-[190px]'>
                                {log.pool_account_id > 0 ? (
                                  <Button
                                    variant='link'
                                    size='sm'
                                    className='h-auto max-w-[190px] justify-start truncate p-0 text-sm font-medium'
                                    title={t('Click to filter by account')}
                                    onClick={() =>
                                      filterStateLogsByAccount(
                                        log.pool_account_id,
                                        log.pool_account_name ||
                                          `#${log.pool_account_id}`
                                      )
                                    }
                                  >
                                    {log.pool_account_name ||
                                      `#${log.pool_account_id}`}
                                  </Button>
                                ) : (
                                  <div className='text-sm font-medium'>
                                    {log.pool_account_name ||
                                      `#${log.pool_account_id}`}
                                  </div>
                                )}
                                <div className='text-muted-foreground text-xs'>
                                  {log.pool_group_name ||
                                    `#${log.pool_group_id}`}{' '}
                                  · {log.pool_account_auth_type || '-'}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[170px]'>
                                <div className='text-sm'>
                                  {stateLogActionLabel(log.action, t)}
                                </div>
                                <div className='text-muted-foreground text-xs'>
                                  {t('Source')}: {log.source || '-'}
                                  {log.actor
                                    ? ` · ${t('Actor')}: ${log.actor}`
                                    : ''}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[180px] text-xs'>
                                {poolAccountStatusText(
                                  log.before_status,
                                  log.before_schedulable,
                                  log.before_unavailable,
                                  t
                                )}
                                <div className='text-muted-foreground mt-1 max-w-[240px] break-words'>
                                  {log.before_status_message ||
                                    log.before_disabled_reason ||
                                    '-'}
                                </div>
                                {log.before_next_retry_time > 0 ? (
                                  <div className='text-muted-foreground mt-1'>
                                    {t('Next retry')}:&nbsp;
                                    {formatTimestamp(
                                      log.before_next_retry_time
                                    )}
                                  </div>
                                ) : null}
                              </TableCell>
                              <TableCell className='min-w-[180px] text-xs'>
                                {poolAccountStatusText(
                                  log.after_status,
                                  log.after_schedulable,
                                  log.after_unavailable,
                                  t
                                )}
                                <div className='text-muted-foreground mt-1 max-w-[240px] break-words'>
                                  {log.after_status_message ||
                                    log.after_disabled_reason ||
                                    '-'}
                                </div>
                                {log.after_next_retry_time > 0 ? (
                                  <div className='text-muted-foreground mt-1'>
                                    {t('Next retry')}:&nbsp;
                                    {formatTimestamp(log.after_next_retry_time)}
                                  </div>
                                ) : null}
                              </TableCell>
                              <TableCell className='max-w-[280px] text-xs break-words'>
                                {log.reason || '-'}
                              </TableCell>
                            </TableRow>
                          ))}
                          {!stateLogsQuery.isLoading &&
                            stateLogs.length === 0 && (
                              <TableRow>
                                <TableCell
                                  colSpan={6}
                                  className='h-24 text-center'
                                >
                                  {t('No state logs found')}
                                </TableCell>
                              </TableRow>
                            )}
                        </TableBody>
                      </Table>
                    </div>
                    <div className='border-border flex items-center justify-between border-t p-3 text-sm'>
                      <span className='text-muted-foreground'>
                        {t('Page {{page}} of {{total}}', {
                          page: stateLogPage,
                          total: stateLogTotalPages,
                        })}
                      </span>
                      <div className='flex gap-2'>
                        <Button
                          variant='outline'
                          size='sm'
                          disabled={stateLogPage <= 1}
                          onClick={() =>
                            setStateLogPage((current) =>
                              Math.max(1, current - 1)
                            )
                          }
                        >
                          {t('Previous')}
                        </Button>
                        <Button
                          variant='outline'
                          size='sm'
                          disabled={stateLogPage >= stateLogTotalPages}
                          onClick={() =>
                            setStateLogPage((current) =>
                              Math.min(stateLogTotalPages, current + 1)
                            )
                          }
                        >
                          {t('Next')}
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              </TabsContent>
              <TabsContent value='check-tasks' className='m-0'>
                <Card size='sm'>
                  <CardHeader className='border-b'>
                    <CardTitle>{t('Account Records')}</CardTitle>
                    <CardDescription>
                      {t(
                        'Inspect usage records, state changes, and check tasks.'
                      )}
                    </CardDescription>
                    <CardAction className='flex flex-wrap justify-start gap-2 lg:justify-end'>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => void checkTasksQuery.refetch()}
                      >
                        <RefreshCw data-icon='inline-start' />
                        {t('Refresh')}
                      </Button>
                      <Button
                        variant='outline'
                        size='sm'
                        disabled={checkTaskCleaning || !canOperateAccountPool}
                        onClick={() => void cleanupCheckTasks()}
                      >
                        {checkTaskCleaning ? (
                          <Loader2
                            data-icon='inline-start'
                            className='animate-spin'
                          />
                        ) : (
                          <Trash2 data-icon='inline-start' />
                        )}
                        {t('Cleanup')}
                      </Button>
                    </CardAction>
                  </CardHeader>
                  <CardContent className='p-0'>
                    {logViewTabs}
                    <div className='border-border flex flex-col gap-2 border-b p-3 md:hidden'>
                      <div className='flex min-w-0 items-center gap-2'>
                        <Input
                          className='min-w-0 flex-1'
                          placeholder={t(
                            'Search task, group, actor, request, or message'
                          )}
                          value={checkTaskSearch}
                          onChange={(event) =>
                            setCheckTaskSearch(event.target.value)
                          }
                        />
                        <AccountPoolHistoryFilterDrawer
                          kind='check'
                          activeCount={checkTaskFilterCount}
                          resetDisabled={checkTaskFilterCount === 0}
                          onReset={clearCheckTaskFilters}
                        >
                          <div className='grid grid-cols-2 gap-2'>
                            {checkTaskStatusFilterOptions.map((value) => (
                              <Button
                                key={value}
                                type='button'
                                variant={
                                  checkTaskStatus === value
                                    ? 'secondary'
                                    : 'outline'
                                }
                                size='sm'
                                onClick={() => setCheckTaskStatus(value)}
                              >
                                {checkTaskStatusFilterLabel(value, t)}
                              </Button>
                            ))}
                          </div>
                        </AccountPoolHistoryFilterDrawer>
                      </div>
                    </div>
                    <div className='border-border hidden flex-col gap-3 border-b p-3 md:flex md:flex-row md:items-center md:justify-between'>
                      <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
                        <Select
                          items={checkTaskStatusFilterOptions.map((value) => ({
                            value,
                            label: checkTaskStatusFilterLabel(value, t),
                          }))}
                          value={checkTaskStatus}
                          onValueChange={(value) =>
                            setCheckTaskStatus(
                              (value as CheckTaskStatusFilter | null) ?? 'all'
                            )
                          }
                        >
                          <SelectTrigger className='w-full sm:w-[180px]'>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent alignItemWithTrigger={false}>
                            <SelectGroup>
                              {checkTaskStatusFilterOptions.map((value) => (
                                <SelectItem key={value} value={value}>
                                  {checkTaskStatusFilterLabel(value, t)}
                                </SelectItem>
                              ))}
                            </SelectGroup>
                          </SelectContent>
                        </Select>
                      </div>
                      <Input
                        className='md:max-w-xs'
                        placeholder={t(
                          'Search task, group, actor, request, or message'
                        )}
                        value={checkTaskSearch}
                        onChange={(event) =>
                          setCheckTaskSearch(event.target.value)
                        }
                      />
                    </div>
                    <AccountPoolCheckTasksMobileList
                      items={checkTasks}
                      isLoading={checkTasksQuery.isLoading}
                      emptyTitle={t('No check tasks found')}
                      onViewTask={viewCheckTask}
                    />
                    <div className='hidden overflow-x-auto md:block'>
                      <Table>
                        <TableHeader>
                          <TableRow>
                            <TableHead>{t('Task')}</TableHead>
                            <TableHead>{t('Group')}</TableHead>
                            <TableHead>{t('Status')}</TableHead>
                            <TableHead>{t('Progress')}</TableHead>
                            <TableHead>{t('Result')}</TableHead>
                            <TableHead>{t('Actor')}</TableHead>
                            <TableHead>{t('Created')}</TableHead>
                            <TableHead>{t('Finished')}</TableHead>
                            <TableHead>{t('Message')}</TableHead>
                            <TableHead>{t('Action')}</TableHead>
                          </TableRow>
                        </TableHeader>
                        <TableBody>
                          {checkTasks.map((task) => (
                            <TableRow key={task.id}>
                              <TableCell className='min-w-[90px] text-sm font-medium'>
                                #{task.id}
                                {task.request_id ? (
                                  <div className='text-muted-foreground mt-1 max-w-[160px] truncate text-xs'>
                                    {task.request_id}
                                  </div>
                                ) : null}
                              </TableCell>
                              <TableCell className='min-w-[170px]'>
                                <div className='text-sm'>
                                  {task.pool_group_name ||
                                    `#${task.pool_group_id}`}
                                </div>
                                <div className='text-muted-foreground text-xs'>
                                  #{task.pool_group_id}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[120px]'>
                                <Badge
                                  variant={checkTaskBadgeVariant(task.status)}
                                >
                                  {checkTaskStatusLabel(task.status, t)}
                                </Badge>
                              </TableCell>
                              <TableCell className='min-w-[150px] text-xs'>
                                {t('{{checked}}/{{total}} checked', {
                                  checked: task.checked + task.skipped,
                                  total: task.total,
                                })}
                                <div className='text-muted-foreground mt-1'>
                                  {task.total > 0
                                    ? `${checkTaskProgressValue(task)}%`
                                    : '-'}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[170px] text-xs'>
                                <div>
                                  {t('{{success}} passed', {
                                    success: task.success,
                                  })}
                                </div>
                                <div className='text-muted-foreground mt-1'>
                                  {t('{{failed}} failed', {
                                    failed: task.failed,
                                  })}
                                  {' · '}
                                  {t('{{skipped}} skipped', {
                                    skipped: task.skipped,
                                  })}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[140px] text-xs'>
                                {task.actor || '-'}
                              </TableCell>
                              <TableCell className='min-w-[150px] text-xs'>
                                {task.created_time
                                  ? formatTimestamp(task.created_time)
                                  : '-'}
                              </TableCell>
                              <TableCell className='min-w-[150px] text-xs'>
                                {task.finished_time
                                  ? formatTimestamp(task.finished_time)
                                  : '-'}
                              </TableCell>
                              <TableCell className='max-w-[300px] min-w-[220px] text-xs break-words'>
                                {task.message || '-'}
                              </TableCell>
                              <TableCell className='min-w-[100px]'>
                                <Button
                                  variant='outline'
                                  size='sm'
                                  onClick={() => viewCheckTask(task)}
                                >
                                  {t('View')}
                                </Button>
                              </TableCell>
                            </TableRow>
                          ))}
                          {!checkTasksQuery.isLoading &&
                            checkTasks.length === 0 && (
                              <TableRow>
                                <TableCell
                                  colSpan={10}
                                  className='h-24 text-center'
                                >
                                  {t('No check tasks found')}
                                </TableCell>
                              </TableRow>
                            )}
                        </TableBody>
                      </Table>
                    </div>
                    <div className='border-border flex items-center justify-between border-t p-3 text-sm'>
                      <span className='text-muted-foreground'>
                        {t('Page {{page}} of {{total}}', {
                          page: checkTaskPage,
                          total: checkTaskTotalPages,
                        })}
                      </span>
                      <div className='flex gap-2'>
                        <Button
                          variant='outline'
                          size='sm'
                          disabled={checkTaskPage <= 1}
                          onClick={() =>
                            setCheckTaskPage((current) =>
                              Math.max(1, current - 1)
                            )
                          }
                        >
                          {t('Previous')}
                        </Button>
                        <Button
                          variant='outline'
                          size='sm'
                          disabled={checkTaskPage >= checkTaskTotalPages}
                          onClick={() =>
                            setCheckTaskPage((current) =>
                              Math.min(checkTaskTotalPages, current + 1)
                            )
                          }
                        >
                          {t('Next')}
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              </TabsContent>
            </Tabs>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <Dialog open={groupFormOpen} onOpenChange={setGroupFormOpen}>
        <DialogContent className='sm:max-w-xl'>
          <DialogHeader>
            <DialogTitle>
              {t(groupForm.id ? 'Edit Group' : 'New Group')}
            </DialogTitle>
            <DialogDescription>
              {t('Configure the account group used by channels.')}
            </DialogDescription>
          </DialogHeader>
          <div className='grid gap-3 sm:grid-cols-2'>
            <Input
              placeholder={t('Name')}
              value={groupForm.name}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  name: event.target.value,
                }))
              }
            />
            <Input
              placeholder={t('Platform')}
              value={groupForm.platform}
              disabled={!canEditGroupSensitiveFields}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  platform: event.target.value,
                }))
              }
            />
            <Select
              items={authTypeOptions.map((value) => ({ value, label: value }))}
              value={groupForm.authType}
              disabled={!canEditGroupSensitiveFields}
              onValueChange={(value) =>
                setGroupForm((current) => ({
                  ...current,
                  authType: value ?? '',
                }))
              }
            >
              <SelectTrigger className='w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {authTypeOptions.map((value) => (
                    <SelectItem key={value} value={value}>
                      {value}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Select
              items={strategyOptions.map((value) => ({
                value,
                label: strategyLabel(value, t),
              }))}
              value={groupForm.strategy}
              onValueChange={(value) =>
                setGroupForm((current) => ({
                  ...current,
                  strategy: value ?? '',
                }))
              }
            >
              <SelectTrigger className='w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {strategyOptions.map((value) => (
                    <SelectItem key={value} value={value}>
                      {strategyLabel(value, t)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Input
              placeholder={t('Models')}
              value={groupForm.models}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  models: event.target.value,
                }))
              }
            />
            <Input
              placeholder={t('Group')}
              value={groupForm.group}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  group: event.target.value,
                }))
              }
            />
            <Textarea
              id='account-pool-group-model-mapping'
              name='model_mapping'
              className='sm:col-span-2'
              autoComplete='off'
              placeholder={t('Model Mapping')}
              value={groupForm.modelMapping}
              disabled={!canEditGroupSensitiveFields}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  modelMapping: event.target.value,
                }))
              }
            />
            <Input
              type='number'
              min='0'
              inputMode='numeric'
              placeholder={t('Group max concurrency')}
              value={groupForm.maxConcurrency}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  maxConcurrency: event.target.value,
                }))
              }
            />
            <Input
              type='number'
              min='0'
              inputMode='numeric'
              placeholder={t('Rate limit RPM')}
              value={groupForm.rateLimitRpm}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  rateLimitRpm: event.target.value,
                }))
              }
            />
            <Input
              type='number'
              min='0'
              inputMode='numeric'
              placeholder={t('Daily request limit')}
              value={groupForm.dailyRequestLimit}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  dailyRequestLimit: event.target.value,
                }))
              }
            />
            <Input
              type='number'
              min='0'
              inputMode='numeric'
              placeholder={t('Daily quota limit')}
              value={groupForm.dailyQuotaLimit}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  dailyQuotaLimit: event.target.value,
                }))
              }
            />
            <Select
              items={dailyLimitActionOptions.map((value) => ({
                value,
                label: dailyLimitActionLabel(value, t),
              }))}
              value={groupForm.dailyLimitAction}
              onValueChange={(value) =>
                setGroupForm((current) => ({
                  ...current,
                  dailyLimitAction: value ?? 'cooldown',
                }))
              }
            >
              <SelectTrigger className='w-full'>
                <SelectValue placeholder={t('Daily limit action')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {dailyLimitActionOptions.map((value) => (
                    <SelectItem key={value} value={value}>
                      {dailyLimitActionLabel(value, t)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <div className='flex items-center justify-between gap-4 rounded-lg border p-3 sm:col-span-2'>
              <div>
                <div className='text-sm font-medium'>{t('Auto check')}</div>
                <div className='text-muted-foreground text-xs'>
                  {groupForm.autoCheckEnabled
                    ? t('Auto check enabled')
                    : t('Auto check off')}
                </div>
              </div>
              <Switch
                id='account-pool-group-auto-check-enabled'
                name='auto_check_enabled'
                aria-label={t('Auto check')}
                checked={groupForm.autoCheckEnabled}
                onCheckedChange={(checked) =>
                  setGroupForm((current) => ({
                    ...current,
                    autoCheckEnabled: !!checked,
                  }))
                }
              />
            </div>
            <Input
              id='account-pool-group-auto-check-interval-minutes'
              name='auto_check_interval_minutes'
              type='number'
              min='1'
              inputMode='numeric'
              autoComplete='off'
              placeholder={t('Auto check interval minutes')}
              value={groupForm.autoCheckIntervalMinutes}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  autoCheckIntervalMinutes: event.target.value,
                }))
              }
            />
            <Input
              id='account-pool-group-auto-check-limit'
              name='auto_check_limit'
              type='number'
              min='1'
              max='100'
              inputMode='numeric'
              autoComplete='off'
              placeholder={t('Auto check account limit')}
              value={groupForm.autoCheckLimit}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  autoCheckLimit: event.target.value,
                }))
              }
            />
            <Select
              items={preflightCheckModeOptions.map((value) => ({
                value,
                label: preflightCheckModeLabel(value, t),
              }))}
              value={groupForm.preflightCheckMode}
              onValueChange={(value) =>
                setGroupForm((current) => ({
                  ...current,
                  preflightCheckMode:
                    (value as AccountPoolPreflightCheckMode | null) ?? 'off',
                }))
              }
            >
              <SelectTrigger className='w-full'>
                <SelectValue placeholder={t('Preflight check')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {preflightCheckModeOptions.map((value) => (
                    <SelectItem key={value} value={value}>
                      {preflightCheckModeLabel(value, t)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Input
              id='account-pool-group-preflight-check-freshness-minutes'
              name='preflight_check_freshness_minutes'
              type='number'
              min='1'
              inputMode='numeric'
              autoComplete='off'
              placeholder={t('Preflight freshness minutes')}
              value={groupForm.preflightCheckFreshnessMinutes}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  preflightCheckFreshnessMinutes: event.target.value,
                }))
              }
            />
            <Input
              id='account-pool-group-preflight-check-limit'
              name='preflight_check_limit'
              type='number'
              min='1'
              max='100'
              inputMode='numeric'
              autoComplete='off'
              placeholder={t('Preflight check account limit')}
              value={groupForm.preflightCheckLimit}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  preflightCheckLimit: event.target.value,
                }))
              }
            />
            <Select
              items={noAvailableActionOptions.map((value) => ({
                value,
                label: noAvailableActionLabel(value, t),
              }))}
              value={groupForm.noAvailableAction}
              onValueChange={(value) =>
                setGroupForm((current) => ({
                  ...current,
                  noAvailableAction:
                    (value as AccountPoolNoAvailableAction | null) ?? 'fail',
                }))
              }
            >
              <SelectTrigger className='w-full'>
                <SelectValue placeholder={t('No idle account action')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {noAvailableActionOptions.map((value) => (
                    <SelectItem key={value} value={value}>
                      {noAvailableActionLabel(value, t)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Input
              id='account-pool-group-no-available-wait-seconds'
              name='no_available_wait_seconds'
              type='number'
              min='1'
              max='30'
              inputMode='numeric'
              autoComplete='off'
              placeholder={t('Idle account wait seconds')}
              value={groupForm.noAvailableWaitSeconds}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  noAvailableWaitSeconds: event.target.value,
                }))
              }
            />
            <Input
              id='account-pool-group-task-max-concurrency'
              name='task_max_concurrency'
              type='number'
              min='0'
              inputMode='numeric'
              autoComplete='off'
              placeholder={t('Task submit concurrency')}
              value={groupForm.taskMaxConcurrency}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  taskMaxConcurrency: event.target.value,
                }))
              }
            />
            <Input
              id='account-pool-group-task-rate-limit-rpm'
              name='task_rate_limit_rpm'
              type='number'
              min='0'
              inputMode='numeric'
              autoComplete='off'
              placeholder={t('Task submit RPM')}
              value={groupForm.taskRateLimitRpm}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  taskRateLimitRpm: event.target.value,
                }))
              }
            />
            <Select
              items={taskLimitActionOptions.map((value) => ({
                value,
                label: taskLimitActionLabel(value, t),
              }))}
              value={groupForm.taskLimitAction}
              onValueChange={(value) =>
                setGroupForm((current) => ({
                  ...current,
                  taskLimitAction:
                    (value as AccountPoolTaskLimitAction | null) ?? 'fail',
                }))
              }
            >
              <SelectTrigger className='w-full'>
                <SelectValue placeholder={t('Task concurrency action')} />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {taskLimitActionOptions.map((value) => (
                    <SelectItem key={value} value={value}>
                      {taskLimitActionLabel(value, t)}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Input
              id='account-pool-group-task-limit-wait-seconds'
              name='task_limit_wait_seconds'
              type='number'
              min='1'
              max='30'
              inputMode='numeric'
              autoComplete='off'
              placeholder={t('Task wait seconds')}
              value={groupForm.taskLimitWaitSeconds}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  taskLimitWaitSeconds: event.target.value,
                }))
              }
            />
            <Textarea
              id='account-pool-group-settings'
              name='settings'
              className='sm:col-span-2'
              autoComplete='off'
              placeholder={t('Settings JSON')}
              value={groupForm.settings}
              disabled={!canEditGroupSensitiveFields}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  settings: event.target.value,
                }))
              }
            />
          </div>
          <DialogFooter>
            <Button
              onClick={submitGroup}
              disabled={actionLoading || !canWriteAccountPool}
            >
              {actionLoading && (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              )}
              {t('Save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={accountFormOpen} onOpenChange={setAccountFormOpen}>
        <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-3xl'>
          <DialogHeader>
            <DialogTitle>
              {t(accountForm.id ? 'Edit Account' : 'Add Account')}
            </DialogTitle>
            <DialogDescription>
              {accountForm.id
                ? t(
                    'Credentials are encrypted at rest and never returned in full.'
                  )
                : t(
                    'Select credentials, reuse another account group, or add one manually.'
                  )}
            </DialogDescription>
          </DialogHeader>
          <Tabs
            value={accountForm.id ? 'manual' : accountAddMode}
            onValueChange={(value) =>
              setAccountAddMode(value as AccountAddMode)
            }
          >
            {!accountForm.id && (
              <TabsList className='max-w-full justify-start overflow-x-auto'>
                <TabsTrigger
                  value='credentials'
                  disabled={!canReadAccountPoolAuthFile}
                >
                  <FileJson data-icon='inline-start' />
                  {t('Select Credentials')}
                </TabsTrigger>
                <TabsTrigger value='group'>
                  <Copy data-icon='inline-start' />
                  {t('Reuse Group')}
                </TabsTrigger>
                <TabsTrigger value='manual'>
                  <KeyRound data-icon='inline-start' />
                  {t('Manual')}
                </TabsTrigger>
              </TabsList>
            )}

            <TabsContent
              value='credentials'
              className='mt-3 flex flex-col gap-3'
            >
              {!canReadAccountPoolAuthFile ? (
                <EmptyState
                  icon={FileJson}
                  title={t("You don't have necessary permission")}
                  bordered
                />
              ) : (
                <>
                  <div className='flex flex-col gap-2 sm:flex-row'>
                    <Input
                      className='sm:max-w-xs'
                      placeholder={t('Search credentials')}
                      value={credentialSearch}
                      onChange={(event) =>
                        setCredentialSearch(event.target.value)
                      }
                    />
                    <Button
                      variant='outline'
                      type='button'
                      onClick={() =>
                        toggleAllAttachCredentials(
                          !allAttachCredentialsSelected
                        )
                      }
                      disabled={selectableAttachCredentialIds.length === 0}
                    >
                      {allAttachCredentialsSelected
                        ? t('Clear selection')
                        : t('Select visible')}
                    </Button>
                    <div className='text-muted-foreground flex items-center text-sm'>
                      {t('{{count}} selected', {
                        count: selectedAuthFileIds.length,
                      })}
                    </div>
                  </div>
                  <div className='border-border max-h-[360px] overflow-y-auto rounded-md border'>
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead className='w-10'>
                            <Checkbox
                              checked={allAttachCredentialsSelected}
                              onCheckedChange={(checked) =>
                                toggleAllAttachCredentials(Boolean(checked))
                              }
                              disabled={
                                selectableAttachCredentialIds.length === 0
                              }
                              aria-label={t('Select all')}
                            />
                          </TableHead>
                          <TableHead>{t('Account')}</TableHead>
                          <TableHead>{t('Source')}</TableHead>
                          <TableHead>{t('Type')}</TableHead>
                          <TableHead>{t('Account Groups')}</TableHead>
                          <TableHead>{t('Status')}</TableHead>
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {filteredAttachCredentials.map((authFile) => {
                          const alreadyAssigned = authFileAssignedToGroup(
                            authFile,
                            selectedGroupId ?? null
                          )
                          const fullSummary = formatCredentialSummary(
                            authFile.credential_summary
                          )
                          return (
                            <TableRow key={authFile.id}>
                              <TableCell>
                                <Checkbox
                                  checked={selectedAuthFileIds.includes(
                                    authFile.id
                                  )}
                                  disabled={alreadyAssigned}
                                  onCheckedChange={(checked) =>
                                    toggleAuthFileSelection(
                                      authFile.id,
                                      Boolean(checked)
                                    )
                                  }
                                  aria-label={t('Select row')}
                                />
                              </TableCell>
                              <TableCell className='max-w-[320px] min-w-[220px]'>
                                <div
                                  className='truncate font-medium'
                                  title={fullSummary}
                                >
                                  {formatAccountIdentity(
                                    authFile.credential_summary,
                                    authFile.name
                                  )}
                                </div>
                                <div className='text-muted-foreground truncate text-xs'>
                                  {authFile.name} · #{authFile.id}
                                </div>
                              </TableCell>
                              <TableCell className='min-w-[140px] text-xs'>
                                {authFileSourceLabel(authFile) || '-'}
                              </TableCell>
                              <TableCell className='min-w-[90px] text-xs'>
                                {authFile.subscription_type || '-'}
                              </TableCell>
                              <TableCell
                                className='max-w-[220px] min-w-[150px] truncate text-xs'
                                title={authFilePoolGroupNames(authFile).join(
                                  ', '
                                )}
                              >
                                {authFileGroupLabel(authFile)}
                              </TableCell>
                              <TableCell className='min-w-[120px]'>
                                <StatusBadge
                                  label={
                                    alreadyAssigned
                                      ? t('Already in group')
                                      : t('Available')
                                  }
                                  variant={
                                    alreadyAssigned ? 'neutral' : 'success'
                                  }
                                  copyable={false}
                                />
                              </TableCell>
                            </TableRow>
                          )
                        })}
                        {!attachCredentialsQuery.isLoading &&
                          filteredAttachCredentials.length === 0 && (
                            <TableRow>
                              <TableCell
                                colSpan={6}
                                className='h-24 text-center'
                              >
                                {t('No credentials found')}
                              </TableCell>
                            </TableRow>
                          )}
                        {attachCredentialsQuery.isLoading && (
                          <TableRow>
                            <TableCell colSpan={6} className='h-24 text-center'>
                              {t('Loading')}
                            </TableCell>
                          </TableRow>
                        )}
                      </TableBody>
                    </Table>
                  </div>
                </>
              )}
            </TabsContent>

            <TabsContent value='group' className='mt-3 flex flex-col gap-3'>
              <Select
                items={sourceGroupOptions.map((group) => ({
                  value: String(group.id),
                  label: group.name,
                }))}
                value={sourceGroupId}
                onValueChange={(value) => setSourceGroupId(value ?? '')}
              >
                <SelectTrigger className='w-full'>
                  <SelectValue placeholder={t('Source account group')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {sourceGroupOptions.map((group) => (
                      <SelectItem key={group.id} value={String(group.id)}>
                        {group.name}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <div className='border-border grid gap-3 rounded-md border p-3 text-sm sm:grid-cols-3'>
                <div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Source')}
                  </div>
                  <div className='truncate font-medium'>
                    {selectedSourceGroup?.name || '-'}
                  </div>
                </div>
                <div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Total accounts')}
                  </div>
                  <div className='font-medium'>
                    {formatUsageNumber(selectedSourceGroup?.stats?.total ?? 0)}
                  </div>
                </div>
                <div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Available accounts')}
                  </div>
                  <div className='font-medium'>
                    {formatUsageNumber(
                      selectedSourceGroup?.stats?.enabled ?? 0
                    )}
                  </div>
                </div>
              </div>
              {sourceGroupOptions.length === 0 && (
                <div className='text-muted-foreground text-sm'>
                  {t('No other account groups available')}
                </div>
              )}
            </TabsContent>

            <TabsContent value='manual' className='mt-3'>
              <div className='grid gap-3 sm:grid-cols-2'>
                <Input
                  placeholder={t('Account name')}
                  value={accountForm.name}
                  onChange={(event) =>
                    setAccountForm((current) => ({
                      ...current,
                      name: event.target.value,
                    }))
                  }
                />
                <Input
                  placeholder={t('Platform')}
                  value={accountForm.platform}
                  onChange={(event) =>
                    setAccountForm((current) => ({
                      ...current,
                      platform: event.target.value,
                    }))
                  }
                />
                <Select
                  items={authTypeOptions.map((value) => ({
                    value,
                    label: value,
                  }))}
                  value={accountForm.authType}
                  onValueChange={(value) =>
                    setAccountForm((current) => ({
                      ...current,
                      authType: value ?? '',
                    }))
                  }
                >
                  <SelectTrigger className='w-full'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {authTypeOptions.map((value) => (
                        <SelectItem key={value} value={value}>
                          {value}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <Input
                  placeholder={t('Proxy')}
                  value={accountForm.proxy}
                  onChange={(event) =>
                    setAccountForm((current) => ({
                      ...current,
                      proxy: event.target.value,
                    }))
                  }
                />
                <Input
                  placeholder={t('Models')}
                  value={accountForm.models}
                  onChange={(event) =>
                    setAccountForm((current) => ({
                      ...current,
                      models: event.target.value,
                    }))
                  }
                />
                <Input
                  placeholder={t('Group')}
                  value={accountForm.group}
                  onChange={(event) =>
                    setAccountForm((current) => ({
                      ...current,
                      group: event.target.value,
                    }))
                  }
                />
                <Input
                  placeholder={t('Priority')}
                  value={accountForm.priority}
                  onChange={(event) =>
                    setAccountForm((current) => ({
                      ...current,
                      priority: event.target.value,
                    }))
                  }
                />
                <Input
                  placeholder={t('Weight')}
                  value={accountForm.weight}
                  onChange={(event) =>
                    setAccountForm((current) => ({
                      ...current,
                      weight: event.target.value,
                    }))
                  }
                />
                <Input
                  type='number'
                  min='0'
                  inputMode='numeric'
                  placeholder={t('Max concurrency')}
                  value={accountForm.maxConcurrency}
                  onChange={(event) =>
                    setAccountForm((current) => ({
                      ...current,
                      maxConcurrency: event.target.value,
                    }))
                  }
                />
                <Input
                  type='number'
                  min='0'
                  inputMode='numeric'
                  placeholder={t('Rate limit RPM')}
                  value={accountForm.rateLimitRpm}
                  onChange={(event) =>
                    setAccountForm((current) => ({
                      ...current,
                      rateLimitRpm: event.target.value,
                    }))
                  }
                />
                <Input
                  type='number'
                  min='0'
                  inputMode='numeric'
                  placeholder={t('Daily request limit')}
                  value={accountForm.dailyRequestLimit}
                  onChange={(event) =>
                    setAccountForm((current) => ({
                      ...current,
                      dailyRequestLimit: event.target.value,
                    }))
                  }
                />
                <Input
                  type='number'
                  min='0'
                  inputMode='numeric'
                  placeholder={t('Daily quota limit')}
                  value={accountForm.dailyQuotaLimit}
                  onChange={(event) =>
                    setAccountForm((current) => ({
                      ...current,
                      dailyQuotaLimit: event.target.value,
                    }))
                  }
                />
                <Select
                  items={accountDailyLimitActionOptions.map((value) => ({
                    value,
                    label: dailyLimitActionLabel(value, t),
                  }))}
                  value={accountForm.dailyLimitAction}
                  onValueChange={(value) =>
                    setAccountForm((current) => ({
                      ...current,
                      dailyLimitAction: value ?? 'inherit',
                    }))
                  }
                >
                  <SelectTrigger className='w-full'>
                    <SelectValue placeholder={t('Daily limit action')} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {accountDailyLimitActionOptions.map((value) => (
                        <SelectItem key={value} value={value}>
                          {dailyLimitActionLabel(value, t)}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <Textarea
                  className='sm:col-span-2'
                  rows={5}
                  placeholder={
                    accountForm.id
                      ? t('Leave empty to keep existing credential')
                      : t('Credentials')
                  }
                  value={accountForm.credentials}
                  onChange={(event) =>
                    setAccountForm((current) => ({
                      ...current,
                      credentials: event.target.value,
                    }))
                  }
                />
              </div>
            </TabsContent>
          </Tabs>
          <DialogFooter>
            <Button
              onClick={submitAccount}
              disabled={actionLoading || !canSensitiveWriteAccountPool}
            >
              {actionLoading && (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              )}
              {t(
                accountForm.id || accountAddMode === 'manual' ? 'Save' : 'Add'
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={batchOpen} onOpenChange={setBatchOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Batch Import')}</DialogTitle>
            <DialogDescription>
              {t('One credential per line.')}
            </DialogDescription>
          </DialogHeader>
          <Textarea
            rows={8}
            value={batchCredentials}
            onChange={(event) => setBatchCredentials(event.target.value)}
            placeholder={t('Credentials')}
          />
          <DialogFooter>
            <Button
              onClick={submitBatch}
              disabled={actionLoading || !canSensitiveWriteAccountPool}
            >
              {actionLoading && (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              )}
              {t('Import')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={codexInputOpen} onOpenChange={setCodexInputOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Codex OAuth')}</DialogTitle>
            <DialogDescription>
              {t(
                'Paste the callback URL or code after official authorization.'
              )}
            </DialogDescription>
          </DialogHeader>
          <Input
            placeholder={t('Account name')}
            value={codexName}
            onChange={(event) => setCodexName(event.target.value)}
          />
          <Textarea
            rows={5}
            placeholder={t('Authorization callback')}
            value={codexInput}
            onChange={(event) => setCodexInput(event.target.value)}
          />
          <DialogFooter>
            <Button
              onClick={completeCodexOAuth}
              disabled={actionLoading || !canSensitiveWriteAccountPool}
            >
              {actionLoading && (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              )}
              {t('Complete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={deviceSessionOpen} onOpenChange={setDeviceSessionOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('Codex Device Login')}</DialogTitle>
            <DialogDescription>
              {t('Open the verification page and enter the user code.')}
            </DialogDescription>
          </DialogHeader>
          <div className='grid gap-3 text-sm'>
            <div>
              <div className='text-muted-foreground text-xs'>
                {t('Verification URL')}
              </div>
              <div className='font-medium break-all'>
                {deviceSession?.verification_url ?? '-'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground text-xs'>
                {t('User Code')}
              </div>
              <div className='text-lg font-semibold'>
                {deviceSession?.user_code ?? '-'}
              </div>
            </div>
            <div>
              <div className='text-muted-foreground text-xs'>{t('Status')}</div>
              <div className='font-medium'>
                {deviceSession?.status ?? t('Pending')}
              </div>
              {deviceSession?.status_message ? (
                <div className='text-destructive mt-1 text-xs'>
                  {deviceSession.status_message}
                </div>
              ) : null}
            </div>
          </div>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() =>
                deviceSession?.verification_url &&
                window.open(
                  deviceSession.verification_url,
                  '_blank',
                  'noopener,noreferrer'
                )
              }
            >
              {t('Open')}
            </Button>
            <Button onClick={() => setDeviceSessionOpen(false)}>
              {t('Close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
