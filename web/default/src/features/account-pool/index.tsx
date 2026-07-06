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
import {
  AlertTriangle,
  Download,
  Loader2,
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { StatusBadge } from '@/components/status-badge'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import { formatTimestamp } from '@/features/channels/lib'
import {
  accountPoolQueryKeys,
  batchCreatePoolAccounts,
  batchDeletePoolAccounts,
  batchUpdatePoolAccountStatus,
  checkPoolAccount,
  completeAccountPoolProviderOAuth,
  createAccountPoolGroup,
  createPoolAccount,
  deleteAccountPoolGroup,
  deletePoolAccount,
  exportPoolAccounts,
  getAccountPoolLoginSession,
  getAccountPoolGroups,
  getAccountPoolProviders,
  getAccountPoolStateLogs,
  getAccountPoolUsageLogs,
  getPoolAccountCheckTask,
  getPoolAccounts,
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
import type {
  AccountPoolCheckTask,
  AccountPoolGroup,
  AccountPoolGroupPayload,
  AccountPoolLoginSession,
  PoolAccount,
  PoolAccountPayload,
} from './types'

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

type AccountPoolView = 'accounts' | 'auth-files' | 'usage-logs' | 'state-logs'
type UsageLogStatusFilter = 'all' | 'success' | 'failed'

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

function strategyLabel(
  strategy: string,
  t: (key: string) => string
): string {
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

function groupLimitSummary(
  group: AccountPoolGroup,
  t: (key: string) => string
): string {
  const parts = [
    `${t('Max concurrency')}: ${formatLimitValue(group.max_concurrency, t)}`,
    `${t('RPM')}: ${formatLimitValue(group.rate_limit_rpm, t)}`,
    `${t('Daily requests')}: ${formatLimitValue(group.daily_request_limit, t)}`,
    `${t('Daily quota')}: ${formatLimitValue(group.daily_quota_limit, t)}`,
    `${t('Limit action')}: ${dailyLimitActionLabel(
      group.daily_limit_action || 'cooldown',
      t
    )}`,
  ]
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

function accountLimitSummary(
  account: PoolAccount,
  t: (key: string) => string
): string {
  const parts = [
    `${t('Max concurrency')}: ${formatLimitValue(account.max_concurrency, t)}`,
    `${t('RPM')}: ${formatLimitValue(account.rate_limit_rpm, t)}`,
    `${t('Daily requests')}: ${formatUsageNumber(
      account.daily_request_count
    )} / ${formatLimitValue(account.daily_request_limit, t)}`,
    `${t('Daily quota')}: ${formatUsageNumber(
      account.daily_used_quota
    )} / ${formatLimitValue(account.daily_quota_limit, t)}`,
    `${t('Limit action')}: ${dailyLimitActionLabel(
      account.daily_limit_action || 'inherit',
      t
    )}`,
  ]
  return parts.join(' · ')
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

export function AccountPool() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [activeView, setActiveView] = useState<AccountPoolView>('accounts')
  const [usageLogPage, setUsageLogPage] = useState(1)
  const [usageLogStatus, setUsageLogStatus] =
    useState<UsageLogStatusFilter>('all')
  const [usageLogSearch, setUsageLogSearch] = useState('')
  const [stateLogPage, setStateLogPage] = useState(1)
  const [stateLogSearch, setStateLogSearch] = useState('')
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
    if (!selectedGroupId && groups.length > 0 && activeView === 'accounts') {
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
  }, [activeView, groups, selectedGroupId])

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
      pool_group_id: selectedGroupId ?? undefined,
      success:
        usageLogStatus === 'all' ? undefined : usageLogStatus === 'success',
      search: usageLogSearch.trim() || undefined,
    }),
    [selectedGroupId, usageLogPage, usageLogSearch, usageLogStatus]
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
  const stateLogParams = useMemo(
    () => ({
      p: stateLogPage,
      page_size: 10,
      pool_group_id: selectedGroupId ?? undefined,
      search: stateLogSearch.trim() || undefined,
    }),
    [selectedGroupId, stateLogPage, stateLogSearch]
  )
  const stateLogsQuery = useQuery({
    queryKey: accountPoolQueryKeys.stateLogs(stateLogParams),
    queryFn: () => getAccountPoolStateLogs(stateLogParams),
    enabled: activeView === 'state-logs',
  })
  const stateLogPageInfo = stateLogsQuery.data?.data
  const stateLogs = stateLogPageInfo?.items ?? []
  const stateLogTotalPages = Math.max(
    1,
    Math.ceil(
      (stateLogPageInfo?.total ?? 0) / (stateLogPageInfo?.page_size ?? 10)
    )
  )

  useEffect(() => {
    setUsageLogPage(1)
  }, [selectedGroupId, usageLogSearch, usageLogStatus])

  useEffect(() => {
    setStateLogPage(1)
  }, [selectedGroupId, stateLogSearch])

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
      autoCheckIntervalMinutes: String(
        group.auto_check_interval_minutes || 60
      ),
      autoCheckLimit: String(group.auto_check_limit || 100),
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
            {t(
              'Manage official login accounts and expose account groups to channels.'
            )}
          </p>
        </div>
        <div className='flex flex-wrap gap-2'>
          <Button variant='outline' onClick={() => void refreshAll()}>
            <RefreshCw className='mr-2 h-4 w-4' />
            {t('Refresh')}
          </Button>
          <Button onClick={openCreateGroup}>
            <Plus className='mr-2 h-4 w-4' />
            {t('New Group')}
          </Button>
        </div>
      </div>

      <div className='grid min-h-0 flex-1 gap-4 lg:grid-cols-[340px_minmax(0,1fr)]'>
        <section className='border-border bg-background min-h-[260px] rounded-lg border'>
          <div className='border-border flex items-center justify-between border-b p-3'>
            <div className='text-sm font-medium'>{t('Account Groups')}</div>
            {groupsQuery.isLoading && (
              <Loader2 className='h-4 w-4 animate-spin' />
            )}
          </div>
          <div className='divide-border divide-y'>
            {groups.map((group) => {
              return (
                <button
                  key={group.id}
                  type='button'
                  className={`hover:bg-muted/60 flex w-full flex-col gap-2 px-3 py-3 text-left ${
                    selectedGroupId === group.id ? 'bg-muted' : ''
                  }`}
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
                      {t('Total')}: {group.stats?.total ?? 0}
                    </span>
                    <span>
                      {t('Available')}: {group.stats?.enabled ?? 0}
                    </span>
                    <span>
                      {t('Cooldown')}: {group.stats?.cooldown ?? 0}
                    </span>
                  </div>
                  {group.daily_limit_state?.limited ? (
                    <div className='text-warning flex items-center gap-1 text-xs'>
                      <AlertTriangle className='h-3.5 w-3.5 shrink-0' />
                      <span className='truncate'>
                        {groupDailyLimitSummary(group, t)}
                      </span>
                    </div>
                  ) : null}
                  <div className='text-muted-foreground truncate text-xs'>
                    {groupLimitSummary(group, t)}
                  </div>
                  <div className='text-muted-foreground truncate text-xs'>
                    {groupAutoCheckSummary(group, t)}
                  </div>
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

        <section className='border-border bg-background min-w-0 rounded-lg border'>
          <Tabs
            value={activeView}
            onValueChange={(value) => setActiveView(value as AccountPoolView)}
            className='flex min-h-0 flex-col'
          >
            <div className='border-border flex flex-col gap-3 border-b p-3 lg:flex-row lg:items-center lg:justify-between'>
              <div className='min-w-0'>
                <div className='truncate text-sm font-semibold'>
                  {activeView === 'accounts'
                    ? (selectedGroup?.name ?? t('Account Pool'))
                    : activeView === 'auth-files'
                      ? t('Auth Files')
                      : activeView === 'usage-logs'
                        ? t('Usage Logs')
                        : t('State Logs')}
                </div>
                <div className='text-muted-foreground text-xs'>
                  {activeView === 'accounts'
                    ? selectedGroup
                      ? `${strategyLabel(selectedGroup.strategy, t)} · ${selectedGroup.models || t('All Models')} · ${t('Group concurrency')}: ${
                          selectedGroup.max_concurrency > 0
                            ? selectedGroup.max_concurrency
                            : t('Unlimited')
                        } · ${groupAutoCheckSummary(selectedGroup, t)}`
                      : t('Select an account group')
                    : activeView === 'auth-files'
                      ? t(
                          'Manage imported JSON credentials and their linked pool accounts'
                        )
                      : activeView === 'usage-logs'
                        ? selectedGroup
                          ? t('Showing usage records for the selected group')
                          : t('Showing usage records for all groups')
                        : selectedGroup
                          ? t('Showing state changes for the selected group')
                          : t('Showing state changes for all groups')}
                </div>
              </div>
              <TabsList>
                <TabsTrigger value='accounts'>{t('Pool Accounts')}</TabsTrigger>
                <TabsTrigger value='auth-files'>{t('Auth Files')}</TabsTrigger>
                <TabsTrigger value='usage-logs'>{t('Usage Logs')}</TabsTrigger>
                <TabsTrigger value='state-logs'>{t('State Logs')}</TabsTrigger>
              </TabsList>
              <div className='flex flex-wrap gap-2'>
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
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => void stateLogsQuery.refetch()}
                  >
                    <RefreshCw data-icon='inline-start' />
                    {t('Refresh')}
                  </Button>
                )}
                {activeView === 'accounts' && selectedGroup && (
                  <>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => openEditGroup(selectedGroup)}
                    >
                      <Pencil className='mr-2 h-4 w-4' />
                      {t('Edit Group')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => void deleteGroup(selectedGroup)}
                    >
                      <Trash2 className='mr-2 h-4 w-4' />
                      {t('Delete')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={startCodexOAuth}
                    >
                      <ShieldCheck className='mr-2 h-4 w-4' />
                      {t('Codex OAuth')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={startCodexDevice}
                    >
                      <Smartphone className='mr-2 h-4 w-4' />
                      {t('Codex Device')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => setBatchOpen(true)}
                    >
                      <Upload className='mr-2 h-4 w-4' />
                      {t('Batch Import')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={actionLoading || accountTotal <= 0}
                      onClick={() => void exportAccounts()}
                    >
                      <Download className='mr-2 h-4 w-4' />
                      {t('Export Accounts')}
                    </Button>
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
                        <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                      ) : (
                        <Stethoscope className='mr-2 h-4 w-4' />
                      )}
                      {t('Check Group')}
                    </Button>
                    <Button size='sm' onClick={openCreateAccount}>
                      <Plus className='mr-2 h-4 w-4' />
                      {t('Add Account')}
                    </Button>
                  </>
                )}
              </div>
            </div>

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
              {selectedGroup ? (
                <div className='border-border text-muted-foreground flex flex-wrap gap-3 border-b p-3 text-xs'>
                  <span>{groupAutoCheckSummary(selectedGroup, t)}</span>
                  {selectedGroup.auto_check_enabled ? (
                    <>
                      <span>
                        {t('Last auto check')}:&nbsp;
                        {selectedGroup.auto_check_last_time
                          ? formatTimestamp(selectedGroup.auto_check_last_time)
                          : '-'}
                      </span>
                      <span>
                        {t('Next auto check')}:&nbsp;
                        {selectedGroup.auto_check_next_time
                          ? formatTimestamp(selectedGroup.auto_check_next_time)
                          : '-'}
                      </span>
                      {selectedGroup.auto_check_last_task_id ? (
                        <span>
                          {t('Last auto check task #{{id}}', {
                            id: selectedGroup.auto_check_last_task_id,
                          })}
                        </span>
                      ) : null}
                    </>
                  ) : null}
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
                    {t('Daily requests')}
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
                        {checkTaskStatusLabel(
                          selectedGroupCheckTask.status,
                          t
                        )}
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

              {accounts.length > 0 ? (
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
                      <Power className='mr-2 h-4 w-4' />
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
                      <PowerOff className='mr-2 h-4 w-4' />
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
                      <RefreshCw className='mr-2 h-4 w-4' />
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
                      <Download className='mr-2 h-4 w-4' />
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
                      <Trash2 className='mr-2 h-4 w-4' />
                      {t('Delete selected accounts')}
                    </Button>
                  </div>
                </div>
              ) : null}

              <div className='overflow-x-auto'>
                <Table>
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
                      <TableHead>{t('Name')}</TableHead>
                      <TableHead>{t('Credential')}</TableHead>
                      <TableHead>{t('Models')}</TableHead>
                      <TableHead>{t('Status')}</TableHead>
                      <TableHead>{t('Last Used')}</TableHead>
                      <TableHead>{t('Actions')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {accounts.map((account) => (
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
                        <TableCell>
                          <div className='font-medium'>{account.name}</div>
                          <div className='text-muted-foreground text-xs'>
                            #{account.id} ·{' '}
                            {account.credential_provider || account.platform} /{' '}
                            {account.auth_type}
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            {t('Success')}: {account.success_count ?? 0} ·{' '}
                            {t('Failed')}: {account.failed_count ?? 0}
                          </div>
                          <div className='text-muted-foreground text-xs'>
                            {accountLimitSummary(account, t)}
                          </div>
                        </TableCell>
                        <TableCell className='max-w-[260px] truncate text-xs'>
                          {formatCredentialSummary(account.credential_summary)}
                        </TableCell>
                        <TableCell className='max-w-[220px] truncate text-xs'>
                          {account.models || t('Inherited')}
                        </TableCell>
                        <TableCell>
                          <div className='flex flex-col gap-1'>
                            <StatusBadge
                              label={statusLabel(account, nowSeconds, t)}
                              variant={statusVariant(account, nowSeconds)}
                              copyable={false}
                            />
                            <span className='text-muted-foreground text-xs'>
                              {cooldownText(account, nowSeconds)}
                            </span>
                          </div>
                        </TableCell>
                        <TableCell className='text-xs'>
                          {account.last_used_time
                            ? formatTimestamp(account.last_used_time)
                            : '-'}
                          {account.next_refresh_time ? (
                            <div className='text-muted-foreground mt-1'>
                              {t('Next refresh')}:&nbsp;
                              {formatTimestamp(account.next_refresh_time)}
                            </div>
                          ) : null}
                          {account.last_checked_time ? (
                            <div className='text-muted-foreground mt-1'>
                              {t('Last check time')}:&nbsp;
                              {formatTimestamp(account.last_checked_time)}
                            </div>
                          ) : null}
                        </TableCell>
                        <TableCell>
                          <div className='flex flex-wrap gap-1.5'>
                            <Button
                              variant='ghost'
                              size='icon-sm'
                              onClick={() => openEditAccount(account)}
                            >
                              <Pencil className='h-4 w-4' />
                            </Button>
                            <Button
                              variant='ghost'
                              size='icon-sm'
                              onClick={() =>
                                void setAccountEnabled(
                                  account,
                                  account.status !== CHANNEL_STATUS.ENABLED ||
                                    !account.schedulable
                                )
                              }
                            >
                              {account.status === CHANNEL_STATUS.ENABLED &&
                              account.schedulable ? (
                                <PowerOff className='h-4 w-4' />
                              ) : (
                                <Power className='h-4 w-4' />
                              )}
                            </Button>
                            <Button
                              variant='ghost'
                              size='icon-sm'
                              onClick={() => void clearCooldown(account)}
                            >
                              <RefreshCw className='h-4 w-4' />
                            </Button>
                            <Button
                              variant='ghost'
                              size='icon-sm'
                              aria-label={t('Check Account')}
                              title={t('Check Account')}
                              disabled={checkingAccountId === account.id}
                              onClick={() => void checkAccount(account)}
                            >
                              {checkingAccountId === account.id ? (
                                <Loader2 className='h-4 w-4 animate-spin' />
                              ) : (
                                <Stethoscope className='h-4 w-4' />
                              )}
                            </Button>
                            <Button
                              variant='ghost'
                              size='icon-sm'
                              onClick={() => void resetRuntime(account)}
                            >
                              <RotateCcw className='h-4 w-4' />
                            </Button>
                            {account.platform === 'codex' &&
                              account.auth_type === 'official_oauth' && (
                                <Button
                                  variant='ghost'
                                  size='icon-sm'
                                  onClick={() =>
                                    void refreshCredential(account)
                                  }
                                >
                                  <ShieldCheck className='h-4 w-4' />
                                </Button>
                              )}
                            <Button
                              variant='ghost'
                              size='icon-sm'
                              onClick={() => void deleteAccount(account)}
                            >
                              <Trash2 className='h-4 w-4' />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                    {!accountsQuery.isLoading && accounts.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={7} className='h-24 text-center'>
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
              <AuthFilesPanel
                groups={groups}
                selectedGroupId={selectedGroupId}
                onSelectGroup={setSelectedGroupId}
              />
            </TabsContent>
            <TabsContent value='usage-logs' className='m-0 min-h-0'>
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
              <div className='border-border flex flex-col gap-3 border-b p-3 md:flex-row md:items-center md:justify-between'>
                <Input
                  className='md:max-w-xs'
                  placeholder={t(
                    'Search account, action, source, actor, or reason'
                  )}
                  value={stateLogSearch}
                  onChange={(event) => setStateLogSearch(event.target.value)}
                />
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
