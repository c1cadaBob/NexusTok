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
  Download,
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
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { cn } from '@/lib/utils'
import { StatusBadge } from '@/components/status-badge'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import { formatTimestamp } from '@/features/channels/lib'
import {
  accountPoolQueryKeys,
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
import { AuthFilesPanel } from './components/auth-files-panel'
import {
  ACCOUNT_POOL_DEFAULT_SECTION,
  ACCOUNT_POOL_SECTION_IDS,
  type AccountPoolSectionId,
  isAccountPoolSectionId,
} from './section-registry'
import type {
  AccountPoolAbnormalAccount,
  AccountPoolCheckTask,
  AccountPoolCheckTaskStatus,
  AccountPoolGroup,
  AccountPoolGroupHealth,
  AccountPoolGroupPayload,
  AccountPoolLoginSession,
  AccountPoolPreflightCheckMode,
  AccountPoolStateLogBulkAuditSummary,
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

const accountPoolSectionMeta: Record<
  AccountPoolSectionId,
  { titleKey: string; descriptionKey: string }
> = {
  overview: {
    titleKey: 'Overview',
    descriptionKey: 'Review account pool health and recent exceptions.',
  },
  credentials: {
    titleKey: 'Account Credentials',
    descriptionKey:
      'Manage imported account credentials as reusable pool resources.',
  },
  groups: {
    titleKey: 'Groups',
    descriptionKey:
      'Configure pool groups, scheduling policies, and linked accounts.',
  },
  history: {
    titleKey: 'Logs & History',
    descriptionKey: 'Inspect usage records, state changes, and check tasks.',
  },
}

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

  useEffect(() => {
    if (!selectedGroupId && groups.length > 0 && activeSection === 'groups') {
      setSelectedGroupId(groups[0].id)
      return
    }
    if (
      selectedGroupId &&
      groups.length > 0 &&
      !groups.some((group) => group.id === selectedGroupId)
    ) {
      setSelectedGroupId(groups[0].id)
    }
  }, [activeSection, groups, selectedGroupId])

  const handleSectionChange = useCallback(
    (section: string) => {
      void navigate({
        to: '/account-pool/$section',
        params: { section: section as AccountPoolSectionId },
      })
    },
    [navigate]
  )

  const sectionMeta =
    accountPoolSectionMeta[activeSection] ??
    accountPoolSectionMeta[ACCOUNT_POOL_DEFAULT_SECTION]
  const logViewTabs = (
    <div className='border-border border-b p-3'>
      <Tabs
        value={logView}
        onValueChange={(value) => setLogView(value as AccountPoolLogView)}
      >
        <TabsList className='h-auto max-w-full flex-wrap justify-start'>
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
    enabled: Boolean(selectedGroupId),
  })

  const accountItems = accountsQuery.data?.data?.accounts.items
  const accounts = useMemo(() => accountItems ?? [], [accountItems])
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
  const stateLogFilterParams = useMemo(
    () => ({
      action: stateLogAction === 'all' ? undefined : stateLogAction,
      source: stateLogSource === 'all' ? undefined : stateLogSource,
      search: stateLogSearch.trim() || undefined,
    }),
    [stateLogAction, stateLogSearch, stateLogSource]
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

  useEffect(() => {
    setUsageLogPage(1)
  }, [usageLogSearch, usageLogStatus])

  useEffect(() => {
    setStateLogPage(1)
  }, [stateLogAction, stateLogSearch, stateLogSource])

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
    setGroupForm(emptyGroupForm)
    setGroupFormOpen(true)
  }

  const openEditGroup = (group: AccountPoolGroup) => {
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
    })
    setGroupFormOpen(true)
  }

  const submitGroup = async () => {
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
    setAccountForm({
      ...emptyAccountForm,
      platform: selectedGroup?.platform ?? '',
      authType: selectedGroup?.auth_type ?? '',
    })
    setAccountFormOpen(true)
  }

  const openEditAccount = (account: PoolAccount) => {
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
    setAccountFormOpen(true)
  }

  const submitAccount = async () => {
    if (!selectedGroupId) return
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
    <div className='flex h-full min-h-0 flex-col gap-4 p-4'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div>
          <h1 className='text-xl font-semibold'>{t('Account Pool')}</h1>
          <p className='text-muted-foreground text-sm'>
            {t('Manage native account pools and credential scheduling.')}
          </p>
        </div>
        <div className='flex flex-wrap gap-2'>
          <Button variant='outline' onClick={() => void refreshAll()}>
            <RefreshCw data-icon='inline-start' />
            {t('Refresh')}
          </Button>
          {activeSection === 'groups' ? (
            <Button onClick={openCreateGroup}>
              <Plus data-icon='inline-start' />
              {t('New Group')}
            </Button>
          ) : null}
        </div>
      </div>

      <div
        className={cn(
          'grid min-h-0 flex-1 gap-4',
          activeSection === 'groups'
            ? 'lg:grid-cols-[320px_minmax(0,1fr)]'
            : 'lg:grid-cols-1'
        )}
      >
        {activeSection === 'groups' ? (
          <section className='border-border bg-background min-h-[260px] rounded-lg border'>
            <div className='border-border flex items-center justify-between border-b p-3'>
              <div className='text-sm font-medium'>{t('Groups')}</div>
              {groupsQuery.isLoading && (
                <Loader2 className='size-4 animate-spin' />
              )}
            </div>
            <div className='divide-border divide-y'>
              {groups.map((group) => {
                return (
                  <button
                    key={group.id}
                    type='button'
                    className={cn(
                      'hover:bg-muted/60 flex w-full flex-col gap-2 px-3 py-3 text-left',
                      selectedGroupId === group.id && 'bg-muted'
                    )}
                    onClick={() => {
                      setSelectedGroupId(group.id)
                      setPage(1)
                    }}
                  >
                    <div className='flex items-start justify-between gap-2'>
                      <div className='min-w-0'>
                        <div className='truncate text-sm font-medium'>
                          {group.name}
                        </div>
                        <div className='text-muted-foreground truncate text-xs'>
                          {group.platform} / {group.auth_type}
                        </div>
                      </div>
                      <StatusBadge
                        label={groupStatusLabel(group, t)}
                        variant={groupStatusVariant(group)}
                        copyable={false}
                      />
                    </div>
                    <div className='text-muted-foreground flex gap-3 text-xs'>
                      <span>
                        {t('Available')}: {group.stats?.enabled ?? 0} /{' '}
                        {group.stats?.total ?? 0}
                      </span>
                      {(group.stats?.disabled ?? 0) > 0 ? (
                        <span>
                          {t('Disabled')}: {group.stats?.disabled ?? 0}
                        </span>
                      ) : null}
                    </div>
                    {group.daily_limit_state?.limited ? (
                      <div className='text-warning flex items-center gap-1 text-xs'>
                        <AlertTriangle className='size-3.5 shrink-0' />
                        <span className='truncate'>
                          {groupDailyLimitSummary(group, t)}
                        </span>
                      </div>
                    ) : null}
                  </button>
                )
              })}
              {!groupsQuery.isLoading && groups.length === 0 && (
                <div className='text-muted-foreground p-6 text-center text-sm'>
                  {t('No account groups found')}
                </div>
              )}
            </div>
          </section>
        ) : null}

        <section className='border-border bg-background min-w-0 rounded-lg border'>
          <Tabs
            value={activeView}
            className='flex min-h-0 flex-col'
          >
            <div className='border-border flex flex-col gap-3 border-b p-3 lg:flex-row lg:items-center lg:justify-between'>
              <div className='flex min-w-0 flex-col gap-2'>
                <div className='truncate text-sm font-semibold'>
                  {activeSection === 'groups' && selectedGroup
                    ? selectedGroup.name
                    : t(sectionMeta.titleKey)}
                </div>
                <div className='text-muted-foreground text-xs'>
                  {activeSection === 'groups' && selectedGroup
                    ? `${selectedGroup.platform} / ${selectedGroup.auth_type}`
                    : t(sectionMeta.descriptionKey)}
                </div>
                {activeSection === 'groups' && selectedGroup ? (
                  <div className='flex flex-wrap items-center gap-2 text-xs'>
                    <StatusBadge
                      label={groupStatusLabel(selectedGroup, t)}
                      variant={groupStatusVariant(selectedGroup)}
                      copyable={false}
                    />
                    <span className='text-muted-foreground'>
                      {t('Available')}: {stats?.enabled ?? 0}
                    </span>
                    <span className='text-muted-foreground'>
                      {t('Total')}: {stats?.total ?? 0}
                    </span>
                    <span className='text-muted-foreground'>
                      {strategyLabel(selectedGroup.strategy, t)}
                    </span>
                    <span className='text-muted-foreground truncate'>
                      {selectedGroup.models || t('All Models')}
                    </span>
                    <span
                      className='text-muted-foreground truncate'
                      title={`${groupAutoCheckSummary(
                        selectedGroup,
                        t
                      )} · ${groupPreflightCheckSummary(selectedGroup, t)}`}
                    >
                      {groupAutoCheckSummary(selectedGroup, t)} ·{' '}
                      {groupPreflightCheckSummary(selectedGroup, t)}
                    </span>
                  </div>
                ) : null}
              </div>
              <div className='flex flex-col gap-2 lg:items-end'>
                <Tabs
                  value={activeSection}
                  onValueChange={handleSectionChange}
                >
                  <TabsList className='h-auto max-w-full flex-wrap justify-start'>
                    {ACCOUNT_POOL_SECTION_IDS.map((section) => (
                      <TabsTrigger key={section} value={section}>
                        {t(accountPoolSectionMeta[section].titleKey)}
                      </TabsTrigger>
                    ))}
                  </TabsList>
                </Tabs>
                <div className='flex flex-wrap justify-start gap-2 lg:justify-end'>
                  {activeView === 'health' && (
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => void healthQuery.refetch()}
                    >
                      <RefreshCw data-icon='inline-start' />
                      {t('Refresh health')}
                    </Button>
                  )}
                  {activeView === 'usage-logs' && (
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => void usageLogsQuery.refetch()}
                    >
                      <RefreshCw data-icon='inline-start' />
                      {t('Refresh')}
                    </Button>
                  )}
                  {activeView === 'state-logs' && (
                    <>
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
                        disabled={stateLogExporting}
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
                    </>
                  )}
                  {activeView === 'check-tasks' && (
                    <>
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
                        disabled={checkTaskCleaning}
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
                    </>
                  )}
                  {activeView === 'accounts' && selectedGroup && (
                    <>
                      <Button
                        variant='outline'
                        size='sm'
                        disabled={
                          batchChecking ||
                          selectedGroupCheckTaskActive ||
                          accountTotal <= 0
                        }
                        onClick={() => void checkSelectedGroupAccounts()}
                      >
                        {batchChecking || selectedGroupCheckTaskActive ? (
                          <Loader2
                            data-icon='inline-start'
                            className='animate-spin'
                          />
                        ) : (
                          <Stethoscope data-icon='inline-start' />
                        )}
                        {t('Check Group')}
                      </Button>
                      <Button size='sm' onClick={openCreateAccount}>
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
                        <DropdownMenuContent align='end' className='w-48'>
                          <DropdownMenuGroup>
                            <DropdownMenuItem
                              onClick={() => openEditGroup(selectedGroup)}
                            >
                              <Pencil />
                              {t('Edit Group')}
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              onClick={() => void deleteGroup(selectedGroup)}
                            >
                              <Trash2 />
                              {t('Delete')}
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={startCodexOAuth}>
                              <ShieldCheck />
                              {t('Codex OAuth')}
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={startCodexDevice}>
                              <Smartphone />
                              {t('Codex Device')}
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => setBatchOpen(true)}>
                              <Upload />
                              {t('Batch Import')}
                            </DropdownMenuItem>
                            <DropdownMenuItem
                              disabled={actionLoading || accountTotal <= 0}
                              onClick={() => void exportAccounts()}
                            >
                              <Download />
                              {t('Export Accounts')}
                            </DropdownMenuItem>
                          </DropdownMenuGroup>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </>
                  )}
                </div>
              </div>
            </div>

            <TabsContent value='health' className='m-0 min-h-0'>
              <div className='border-border text-muted-foreground grid grid-cols-1 gap-1 border-b p-3 text-xs lg:grid-cols-2'>
                <span className='min-w-0'>
                  {t('Generated at')}:&nbsp;
                  {health?.generated_at
                    ? formatTimestamp(health.generated_at)
                    : '-'}
                </span>
                <span className='min-w-0 lg:text-right'>
                  {t('Window')}:&nbsp;
                  {health?.window_start
                    ? formatTimestamp(health.window_start)
                    : '-'}
                  {' - '}
                  {health?.window_end
                    ? formatTimestamp(health.window_end)
                    : '-'}
                </span>
              </div>
              <div className='border-border grid grid-cols-2 gap-3 border-b p-3 text-sm md:grid-cols-5 xl:grid-cols-10'>
                {[
                  {
                    label: t('Total accounts'),
                    value: formatUsageNumber(healthTotals?.total_accounts ?? 0),
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
                    value: formatUsageNumber(healthTotals?.today_requests ?? 0),
                  },
                  {
                    label: t('Today failures'),
                    value: formatUsageNumber(healthTotals?.today_failures ?? 0),
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
                    <div className='truncate font-medium'>{metric.value}</div>
                  </div>
                ))}
              </div>

              <div className='border-border border-b'>
                <div className='flex items-center justify-between gap-2 p-3'>
                  <div className='text-sm font-medium'>{t('Group health')}</div>
                  {healthQuery.isFetching ? (
                    <Loader2 className='text-muted-foreground h-4 w-4 animate-spin' />
                  ) : null}
                </div>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Group')}</TableHead>
                      <TableHead>{t('Available rate')}</TableHead>
                      <TableHead>{t('Today requests')}</TableHead>
                      <TableHead>{t('Today failures')}</TableHead>
                      <TableHead>{t('Success rate')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Automation')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(health?.groups ?? []).map((group) => (
                      <TableRow key={group.id}>
                        <TableCell className='min-w-[200px]'>
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
                            {formatUsageNumber(group.stats?.enabled ?? 0)} /{' '}
                            {formatUsageNumber(group.stats?.total ?? 0)}
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
                                      group.daily_limit_state.next_reset_time
                                    )}`
                                  : group.daily_limit_state.reason || '-'}
                              </span>
                            ) : null}
                          </div>
                        </TableCell>
                        <TableCell className='min-w-[260px] text-xs'>
                          {healthGroupAutomationSummary(group, t)}
                          {group.auto_check_next_time ? (
                            <div className='text-muted-foreground mt-1'>
                              {t('Next auto check')}:&nbsp;
                              {formatTimestamp(group.auto_check_next_time)}
                            </div>
                          ) : null}
                        </TableCell>
                      </TableRow>
                    ))}
                    {!healthQuery.isLoading &&
                      (health?.groups ?? []).length === 0 && (
                        <TableRow>
                          <TableCell colSpan={7} className='h-24 text-center'>
                            {t('No account groups found')}
                          </TableCell>
                        </TableRow>
                      )}
                    {healthQuery.isLoading ? (
                      <TableRow>
                        <TableCell colSpan={7} className='h-24 text-center'>
                          {t('Loading')}
                        </TableCell>
                      </TableRow>
                    ) : null}
                  </TableBody>
                </Table>
              </div>

              <div className='border-border border-b'>
                <div className='p-3 text-sm font-medium'>
                  {t('Recent abnormal accounts')}
                </div>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Account')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Reason')}</TableHead>
                      <TableHead>{t('Cooling until')}</TableHead>
                      <TableHead>{t('Failure rate')}</TableHead>
                      <TableHead>{t('Last Used')}</TableHead>
                      <TableHead>{t('Last check time')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(health?.recent_abnormal_accounts ?? []).map((account) => (
                      <TableRow key={account.id}>
                        <TableCell className='min-w-[220px]'>
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
                        <TableCell className='min-w-[150px] text-xs'>
                          {account.last_checked_time
                            ? formatTimestamp(account.last_checked_time)
                            : '-'}
                        </TableCell>
                      </TableRow>
                    ))}
                    {!healthQuery.isLoading &&
                      (health?.recent_abnormal_accounts ?? []).length === 0 && (
                        <TableRow>
                          <TableCell colSpan={7} className='h-24 text-center'>
                            {t('No abnormal accounts found')}
                          </TableCell>
                        </TableRow>
                      )}
                  </TableBody>
                </Table>
              </div>

              <div>
                <div className='p-3 text-sm font-medium'>
                  {t('Recent state changes')}
                </div>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t('Time')}</TableHead>
                      <TableHead>{t('Account')}</TableHead>
                      <TableHead>{t('Action')}</TableHead>
                      <TableHead>{t('After state')}</TableHead>
                      <TableHead>{t('Reason')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(health?.recent_state_logs ?? []).map((log) => (
                      <TableRow key={log.id}>
                        <TableCell className='min-w-[150px] text-xs'>
                          {formatTimestamp(log.created_at)}
                          <div className='text-muted-foreground mt-1'>
                            {log.request_id || '-'}
                          </div>
                        </TableCell>
                        <TableCell className='min-w-[200px]'>
                          <div className='text-sm font-medium'>
                            {log.pool_account_name || `#${log.pool_account_id}`}
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            {log.pool_group_name || `#${log.pool_group_id}`} ·{' '}
                            {log.pool_account_auth_type || '-'}
                          </div>
                        </TableCell>
                        <TableCell className='min-w-[180px] text-xs'>
                          {stateLogActionLabel(log.action, t)}
                          <div className='text-muted-foreground mt-1'>
                            {t('Source')}: {log.source || '-'}
                            {log.actor ? ` · ${t('Actor')}: ${log.actor}` : ''}
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
                        <TableCell className='max-w-[320px] min-w-[220px] text-xs break-words'>
                          {log.reason || '-'}
                        </TableCell>
                      </TableRow>
                    ))}
                    {!healthQuery.isLoading &&
                      (health?.recent_state_logs ?? []).length === 0 && (
                        <TableRow>
                          <TableCell colSpan={5} className='h-24 text-center'>
                            {t('No recent state changes found')}
                          </TableCell>
                        </TableRow>
                      )}
                  </TableBody>
                </Table>
              </div>
            </TabsContent>
            <TabsContent value='accounts' className='m-0 min-h-0'>
              {selectedGroupDailyLimitTitle ? (
                <div className='border-warning/30 bg-warning/10 text-warning flex gap-2 border-b px-3 py-2 text-sm'>
                  <AlertTriangle className='mt-0.5 h-4 w-4 shrink-0' />
                  <div className='min-w-0'>
                    <div className='font-medium'>
                      {selectedGroupDailyLimitTitle}
                    </div>
                    <div className='text-xs'>
                      {t(
                        'Relay will stop selecting accounts from this group until the next daily reset.'
                      )}
                      {selectedGroup?.daily_limit_state?.next_reset_time ? (
                        <>
                          {' '}
                          {t('Next daily reset')}:&nbsp;
                          {formatTimestamp(
                            selectedGroup.daily_limit_state.next_reset_time
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
                  <div className='font-medium'>{stats?.total ?? 0}</div>
                </div>
                <div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Available')}
                  </div>
                  <div className='font-medium'>{stats?.enabled ?? 0}</div>
                </div>
                <div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Disabled')}
                  </div>
                  <div className='font-medium'>{stats?.disabled ?? 0}</div>
                </div>
                <div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Cooldown')}
                  </div>
                  <div className='font-medium'>{stats?.cooldown ?? 0}</div>
                </div>
                <div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Today requests')}
                  </div>
                  <div className='font-medium'>
                    {formatUsageNumber(selectedGroup?.daily_request_count ?? 0)}
                    {' / '}
                    {formatLimitValue(
                      selectedGroup?.daily_request_limit ?? 0,
                      t
                    )}
                  </div>
                </div>
                <div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Daily quota')}
                  </div>
                  <div className='font-medium'>
                    {formatUsageNumber(selectedGroup?.daily_used_quota ?? 0)}
                    {' / '}
                    {formatLimitValue(selectedGroup?.daily_quota_limit ?? 0, t)}
                  </div>
                </div>
              </div>

              {selectedGroupCheckTask ? (
                <div className='border-border flex flex-col gap-2 border-b p-3 text-sm'>
                  <div className='flex flex-wrap items-center justify-between gap-2'>
                    <div className='flex flex-wrap items-center gap-2'>
                      <span className='font-medium'>{t('Check task')}</span>
                      <Badge
                        variant={checkTaskBadgeVariant(
                          selectedGroupCheckTask.status
                        )}
                      >
                        {checkTaskStatusLabel(selectedGroupCheckTask.status, t)}
                      </Badge>
                      {checkTaskPolling ? (
                        <Loader2 className='text-muted-foreground h-4 w-4 animate-spin' />
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
                        success: selectedGroupCheckTask.success,
                      })}
                    </span>
                    <span>
                      {t('{{failed}} failed', {
                        failed: selectedGroupCheckTask.failed,
                      })}
                    </span>
                    <span>
                      {t('{{skipped}} skipped', {
                        skipped: selectedGroupCheckTask.skipped,
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

              {accounts.length > 0 && selectedAccountIds.length > 0 ? (
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
                        actionLoading || selectedAccountIds.length === 0
                      }
                      onClick={() =>
                        void batchUpdateSelectedAccountStatus('enable')
                      }
                    >
                      <Power data-icon='inline-start' />
                      {t('Enable selected accounts')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={
                        actionLoading || selectedAccountIds.length === 0
                      }
                      onClick={() =>
                        void batchUpdateSelectedAccountStatus('disable')
                      }
                    >
                      <PowerOff data-icon='inline-start' />
                      {t('Disable selected accounts')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={
                        actionLoading || selectedAccountIds.length === 0
                      }
                      onClick={() =>
                        void batchUpdateSelectedAccountStatus('clear_cooldown')
                      }
                    >
                      <RefreshCw data-icon='inline-start' />
                      {t('Clear cooldown')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={
                        actionLoading || selectedAccountIds.length === 0
                      }
                      onClick={() => void exportAccounts(selectedAccountIds)}
                    >
                      <Download data-icon='inline-start' />
                      {t('Export selected accounts')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={
                        actionLoading || selectedAccountIds.length === 0
                      }
                      onClick={() => void batchDeleteSelectedAccounts()}
                    >
                      <Trash2 data-icon='inline-start' />
                      {t('Delete selected accounts')}
                    </Button>
                  </div>
                </div>
              ) : null}

              <div className='min-w-0'>
                <Table className='min-w-[760px] table-fixed'>
                  <TableHeader>
                    <TableRow>
                      <TableHead className='w-10'>
                        <Checkbox
                          checked={allAccountsOnPageSelected}
                          indeterminate={someAccountsOnPageSelected}
                          onCheckedChange={(checked) =>
                            toggleAllAccountsOnPage(Boolean(checked))
                          }
                          aria-label={t('Select all')}
                        />
                      </TableHead>
                      <TableHead className='w-[34%]'>{t('Account')}</TableHead>
                      <TableHead className='w-[13%]'>{t('Status')}</TableHead>
                      <TableHead className='w-[18%]'>{t('Usage')}</TableHead>
                      <TableHead className='w-[23%]'>
                        {t('Last Used')}
                      </TableHead>
                      <TableHead className='w-[104px] text-right'>
                        {t('Actions')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {accounts.map((account) => {
                      const fullCredentialSummary = formatCredentialSummary(
                        account.credential_summary
                      )
                      const accountIdentity = formatAccountIdentity(
                        account.credential_summary,
                        account.name
                      )
                      const accountFileLabel = poolAccountFileLabel(account)
                      const statusReason = visibleAccountStatusReason(
                        account,
                        nowSeconds,
                        t
                      )
                      const accountEnabled =
                        account.status === CHANNEL_STATUS.ENABLED &&
                        account.schedulable

                      return (
                        <TableRow key={account.id}>
                          <TableCell>
                            <Checkbox
                              checked={selectedAccountIds.includes(account.id)}
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
                              {t('File')}: {accountFileLabel}
                            </div>
                            {account.models ? (
                              <div className='text-muted-foreground truncate text-xs'>
                                {t('Models')}: {account.models}
                              </div>
                            ) : null}
                          </TableCell>
                          <TableCell className='min-w-0'>
                            <div className='flex flex-col gap-1'>
                              <StatusBadge
                                label={statusLabel(account, nowSeconds, t)}
                                variant={statusVariant(account, nowSeconds)}
                                copyable={false}
                              />
                              {statusReason ? (
                                <span
                                  className='text-muted-foreground max-w-full truncate text-xs'
                                  title={statusReason}
                                >
                                  {limitInlineText(statusReason, 80)}
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
                              {t('Success')}: {account.success_count ?? 0} ·{' '}
                              {t('Failed')}: {account.failed_count ?? 0}
                            </div>
                          </TableCell>
                          <TableCell
                            className='min-w-0 text-xs'
                            title={[
                              `${t('Last Used')}: ${
                                account.last_used_time
                                  ? formatTimestamp(account.last_used_time)
                                  : '-'
                              }`,
                              `${t('Last check time')}: ${
                                account.last_checked_time
                                  ? formatTimestamp(account.last_checked_time)
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
                                ? formatTimestamp(account.last_used_time)
                                : '-'}
                            </div>
                            <div className='text-muted-foreground mt-1 truncate'>
                              {t('Last check time')}:&nbsp;
                              {account.last_checked_time
                                ? formatTimestamp(account.last_checked_time)
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
                                {formatTimestamp(account.next_refresh_time)}
                              </div>
                            ) : null}
                          </TableCell>
                          <TableCell className='w-[104px]'>
                            <div className='flex flex-nowrap justify-end gap-1.5'>
                              <Button
                                variant='ghost'
                                size='icon-sm'
                                aria-label={t('Check Account')}
                                title={t('Check Account')}
                                disabled={checkingAccountId === account.id}
                                onClick={() => void checkAccount(account)}
                              >
                                {checkingAccountId === account.id ? (
                                  <Loader2 className='animate-spin' />
                                ) : (
                                  <Stethoscope />
                                )}
                              </Button>
                              <Button
                                variant='ghost'
                                size='icon-sm'
                                aria-label={
                                  accountEnabled ? t('Disable') : t('Enable')
                                }
                                title={
                                  accountEnabled ? t('Disable') : t('Enable')
                                }
                                onClick={() =>
                                  void setAccountEnabled(
                                    account,
                                    !accountEnabled
                                  )
                                }
                              >
                                {accountEnabled ? <PowerOff /> : <Power />}
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
                                      onClick={() => openEditAccount(account)}
                                    >
                                      <Pencil />
                                      {t('Edit')}
                                    </DropdownMenuItem>
                                    <DropdownMenuItem
                                      onClick={() => void clearCooldown(account)}
                                    >
                                      <RefreshCw />
                                      {t('Clear cooldown')}
                                    </DropdownMenuItem>
                                    <DropdownMenuItem
                                      onClick={() => void resetRuntime(account)}
                                    >
                                      <RotateCcw />
                                      {t('Reset runtime')}
                                    </DropdownMenuItem>
                                    {account.platform === 'codex' &&
                                      account.auth_type ===
                                        'official_oauth' && (
                                        <DropdownMenuItem
                                          onClick={() =>
                                            void refreshCredential(account)
                                          }
                                        >
                                          <ShieldCheck />
                                          {t('Refresh credential')}
                                        </DropdownMenuItem>
                                      )}
                                    <DropdownMenuItem
                                      variant='destructive'
                                      onClick={() => void deleteAccount(account)}
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
                    {!accountsQuery.isLoading && accounts.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={6} className='h-24 text-center'>
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
                      setPage((current) => Math.max(1, current - 1))
                    }
                  >
                    {t('Previous')}
                  </Button>
                  <Button
                    variant='outline'
                    size='sm'
                    disabled={page >= totalPages}
                    onClick={() =>
                      setPage((current) => Math.min(totalPages, current + 1))
                    }
                  >
                    {t('Next')}
                  </Button>
                </div>
              </div>
            </TabsContent>
            <TabsContent value='auth-files' className='m-0 min-h-0'>
              <AuthFilesPanel groups={groups} />
            </TabsContent>
            <TabsContent value='usage-logs' className='m-0 min-h-0'>
              {logViewTabs}
              <div className='border-border flex flex-col gap-3 border-b p-3 md:flex-row md:items-center md:justify-between'>
                <div className='flex flex-wrap gap-2'>
                  <Button
                    variant={usageLogStatus === 'all' ? 'secondary' : 'outline'}
                    size='sm'
                    onClick={() => setUsageLogStatus('all')}
                  >
                    {t('All')}
                  </Button>
                  <Button
                    variant={
                      usageLogStatus === 'success' ? 'secondary' : 'outline'
                    }
                    size='sm'
                    onClick={() => setUsageLogStatus('success')}
                  >
                    {t('Success')}
                  </Button>
                  <Button
                    variant={
                      usageLogStatus === 'failed' ? 'secondary' : 'outline'
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
                  onChange={(event) => setUsageLogSearch(event.target.value)}
                />
              </div>
              <div className='overflow-x-auto'>
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
                          <div className='text-muted-foreground mt-1'>
                            {log.request_id || '-'}
                          </div>
                        </TableCell>
                        <TableCell className='min-w-[190px]'>
                          <div className='text-sm font-medium'>
                            {log.pool_account_name || `#${log.pool_account_id}`}
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            {log.pool_group_name || `#${log.pool_group_id}`} ·{' '}
                            {log.pool_account_auth_type || '-'}
                          </div>
                        </TableCell>
                        <TableCell className='min-w-[160px]'>
                          <div className='text-sm'>
                            {log.channel_name || `#${log.channel_id}`}
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            {log.username || '-'} / {log.token_name || '-'}
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
                              label={log.success ? t('Success') : t('Failed')}
                              variant={log.success ? 'success' : 'danger'}
                              copyable={false}
                            />
                            {!log.success && (
                              <div className='text-muted-foreground max-w-[260px] text-xs break-words'>
                                {log.status_code ? `${log.status_code} · ` : ''}
                                {log.error_message || log.error_code || '-'}
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
                    {!usageLogsQuery.isLoading && usageLogs.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={6} className='h-24 text-center'>
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
                      setUsageLogPage((current) => Math.max(1, current - 1))
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
            </TabsContent>
            <TabsContent value='state-logs' className='m-0 min-h-0'>
              {logViewTabs}
              <div className='border-border flex flex-col gap-3 border-b p-3 lg:flex-row lg:items-center lg:justify-between'>
                <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
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
                    <SelectTrigger className='w-full sm:w-[220px]'>
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
                    <SelectTrigger className='w-full sm:w-[200px]'>
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
                </div>
                <Input
                  className='lg:max-w-xs'
                  placeholder={t(
                    'Search account, action, source, actor, or reason'
                  )}
                  value={stateLogSearch}
                  onChange={(event) => setStateLogSearch(event.target.value)}
                />
              </div>
              <div className='border-border grid grid-cols-2 gap-3 border-b p-3 text-sm md:grid-cols-4'>
                {[
                  {
                    label: t('Audit logs'),
                    value: formatUsageNumber(stateLogAuditSummary?.total ?? 0),
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
                    <div className='truncate font-medium'>{metric.value}</div>
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
                    (stateLogAuditSummary?.action_stats ?? []).length === 0 && (
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
                    (stateLogAuditSummary?.source_stats ?? []).length === 0 && (
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
              <div className='overflow-x-auto'>
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
                          <div className='text-muted-foreground mt-1'>
                            {log.request_id || '-'}
                          </div>
                        </TableCell>
                        <TableCell className='min-w-[190px]'>
                          <div className='text-sm font-medium'>
                            {log.pool_account_name || `#${log.pool_account_id}`}
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            {log.pool_group_name || `#${log.pool_group_id}`} ·{' '}
                            {log.pool_account_auth_type || '-'}
                          </div>
                        </TableCell>
                        <TableCell className='min-w-[170px]'>
                          <div className='text-sm'>
                            {stateLogActionLabel(log.action, t)}
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            {t('Source')}: {log.source || '-'}
                            {log.actor ? ` · ${t('Actor')}: ${log.actor}` : ''}
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
                              {formatTimestamp(log.before_next_retry_time)}
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
                    {!stateLogsQuery.isLoading && stateLogs.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={6} className='h-24 text-center'>
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
                      setStateLogPage((current) => Math.max(1, current - 1))
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
            </TabsContent>
            <TabsContent value='check-tasks' className='m-0 min-h-0'>
              {logViewTabs}
              <div className='border-border flex flex-col gap-3 border-b p-3 md:flex-row md:items-center md:justify-between'>
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
                  onChange={(event) => setCheckTaskSearch(event.target.value)}
                />
              </div>
              <div className='overflow-x-auto'>
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
                            {task.pool_group_name || `#${task.pool_group_id}`}
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            #{task.pool_group_id}
                          </div>
                        </TableCell>
                        <TableCell className='min-w-[120px]'>
                          <Badge variant={checkTaskBadgeVariant(task.status)}>
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
                    {!checkTasksQuery.isLoading && checkTasks.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={10} className='h-24 text-center'>
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
                      setCheckTaskPage((current) => Math.max(1, current - 1))
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
            </TabsContent>
          </Tabs>
        </section>
      </div>

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
            <Textarea
              id='account-pool-group-settings'
              name='settings'
              className='sm:col-span-2'
              autoComplete='off'
              placeholder={t('Settings JSON')}
              value={groupForm.settings}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  settings: event.target.value,
                }))
              }
            />
          </div>
          <DialogFooter>
            <Button onClick={submitGroup} disabled={actionLoading}>
              {actionLoading && (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              )}
              {t('Save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={accountFormOpen} onOpenChange={setAccountFormOpen}>
        <DialogContent className='sm:max-w-xl'>
          <DialogHeader>
            <DialogTitle>
              {t(accountForm.id ? 'Edit Account' : 'Add Account')}
            </DialogTitle>
            <DialogDescription>
              {t(
                'Credentials are encrypted at rest and never returned in full.'
              )}
            </DialogDescription>
          </DialogHeader>
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
              items={authTypeOptions.map((value) => ({ value, label: value }))}
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
          <DialogFooter>
            <Button onClick={submitAccount} disabled={actionLoading}>
              {actionLoading && (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              )}
              {t('Save')}
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
            <Button onClick={submitBatch} disabled={actionLoading}>
              {actionLoading && (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
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
            <Button onClick={completeCodexOAuth} disabled={actionLoading}>
              {actionLoading && (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
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
    </div>
  )
}
