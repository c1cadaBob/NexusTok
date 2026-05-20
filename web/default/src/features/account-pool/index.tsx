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
import { useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Loader2,
  Pencil,
  Plus,
  Power,
  PowerOff,
  RefreshCw,
  ShieldCheck,
  Trash2,
  Upload,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
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
import { Textarea } from '@/components/ui/textarea'
import { StatusBadge } from '@/components/status-badge'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import { formatTimestamp } from '@/features/channels/lib'
import {
  accountPoolQueryKeys,
  batchCreatePoolAccounts,
  completeAccountPoolCodexOAuth,
  createAccountPoolGroup,
  createPoolAccount,
  deleteAccountPoolGroup,
  deletePoolAccount,
  getAccountPoolGroups,
  getPoolAccounts,
  refreshPoolAccountCredential,
  startAccountPoolCodexOAuth,
  updateAccountPoolGroup,
  updatePoolAccount,
  updatePoolAccountStatus,
} from './api'
import type {
  AccountPoolGroup,
  AccountPoolGroupPayload,
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
  proxy: string
}

const emptyGroupForm: GroupFormState = {
  name: '',
  platform: 'codex',
  authType: 'official_oauth',
  strategy: 'round_robin',
  models: '',
  group: '',
  modelMapping: '',
  settings: '',
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
  proxy: '',
}

const authTypeOptions = [
  'api_key',
  'official_oauth',
  'cookie',
  'service_account',
  'custom_json',
]

const strategyOptions = ['round_robin', 'weighted', 'fill_first', 'least_used']

function numberOrZero(value: string): number {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return 0
  return parsed
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
    account.temp_disabled_until > nowSeconds
  ) {
    return 'warning'
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
  return t('Enabled')
}

function cooldownText(account: PoolAccount, nowSeconds: number): string {
  const until = Math.max(
    account.rate_limited_until,
    account.overload_until,
    account.temp_disabled_until
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

export function AccountPool() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
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
  const [actionLoading, setActionLoading] = useState(false)

  const groupsQuery = useQuery({
    queryKey: accountPoolQueryKeys.groups({ page_size: 100 }),
    queryFn: () => getAccountPoolGroups({ p: 1, page_size: 100 }),
  })

  const groups = groupsQuery.data?.data?.items ?? []
  const selectedGroup = groups.find((group) => group.id === selectedGroupId)

  useEffect(() => {
    if (!selectedGroupId && groups.length > 0) {
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
  }, [groups, selectedGroupId])

  const accountsQuery = useQuery({
    queryKey: accountPoolQueryKeys.accounts(selectedGroupId ?? 0, { page }),
    queryFn: () =>
      getPoolAccounts(selectedGroupId ?? 0, {
        p: page,
        page_size: 10,
      }),
    enabled: Boolean(selectedGroupId),
  })

  const accounts = accountsQuery.data?.data?.accounts.items ?? []
  const accountPage = accountsQuery.data?.data?.accounts
  const stats = accountsQuery.data?.data?.stats ?? selectedGroup?.stats
  const totalPages = Math.max(
    1,
    Math.ceil((accountPage?.total ?? 0) / (accountPage?.page_size ?? 10))
  )
  const nowSeconds = useMemo(() => Math.floor(Date.now() / 1000), [accounts])

  const refreshAll = async () => {
    await queryClient.invalidateQueries({ queryKey: ['account-pool'] })
    await queryClient.invalidateQueries({ queryKey: ['channels'] })
  }

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

  const startCodexOAuth = async () => {
    if (!selectedGroupId) return
    setActionLoading(true)
    try {
      const response = await startAccountPoolCodexOAuth({
        pool_group_id: selectedGroupId,
      })
      if (!response.success) throw new Error(response.message)
      if (response.data?.authorize_url) {
        window.open(
          response.data.authorize_url,
          '_blank',
          'noopener,noreferrer'
        )
      }
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
      const response = await completeAccountPoolCodexOAuth({
        pool_group_id: selectedGroupId,
        input: codexInput.trim(),
        name: codexName.trim(),
      })
      if (!response.success) throw new Error(response.message)
      toast.success(t('Account created successfully'))
      setCodexInputOpen(false)
      setCodexInput('')
      setCodexName('')
      await refreshAll()
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
            {groups.map((group) => (
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
                    label={
                      group.status === CHANNEL_STATUS.ENABLED
                        ? t('Enabled')
                        : t('Disabled')
                    }
                    variant={
                      group.status === CHANNEL_STATUS.ENABLED
                        ? 'success'
                        : 'danger'
                    }
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
              </button>
            ))}
            {!groupsQuery.isLoading && groups.length === 0 && (
              <div className='text-muted-foreground p-6 text-center text-sm'>
                {t('No account groups found')}
              </div>
            )}
          </div>
        </section>

        <section className='border-border bg-background min-w-0 rounded-lg border'>
          <div className='border-border flex flex-col gap-3 border-b p-3 lg:flex-row lg:items-center lg:justify-between'>
            <div className='min-w-0'>
              <div className='truncate text-sm font-semibold'>
                {selectedGroup?.name ?? t('Account Pool')}
              </div>
              <div className='text-muted-foreground text-xs'>
                {selectedGroup
                  ? `${selectedGroup.strategy} · ${selectedGroup.models || t('All Models')}`
                  : t('Select an account group')}
              </div>
            </div>
            <div className='flex flex-wrap gap-2'>
              {selectedGroup && (
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
                  <Button variant='outline' size='sm' onClick={startCodexOAuth}>
                    <ShieldCheck className='mr-2 h-4 w-4' />
                    {t('Codex OAuth')}
                  </Button>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => setBatchOpen(true)}
                  >
                    <Upload className='mr-2 h-4 w-4' />
                    {t('Batch Import')}
                  </Button>
                  <Button size='sm' onClick={openCreateAccount}>
                    <Plus className='mr-2 h-4 w-4' />
                    {t('Add Account')}
                  </Button>
                </>
              )}
            </div>
          </div>

          <div className='border-border grid grid-cols-2 gap-3 border-b p-3 text-sm md:grid-cols-4'>
            <div>
              <div className='text-muted-foreground text-xs'>{t('Total')}</div>
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
          </div>

          <div className='overflow-x-auto'>
            <Table>
              <TableHeader>
                <TableRow>
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
                      <div className='font-medium'>{account.name}</div>
                      <div className='text-muted-foreground text-xs'>
                        #{account.id} · {account.platform} / {account.auth_type}
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
                        {account.platform === 'codex' &&
                          account.auth_type === 'official_oauth' && (
                            <Button
                              variant='ghost'
                              size='icon-sm'
                              onClick={() => void refreshCredential(account)}
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
                onClick={() => setPage((current) => Math.max(1, current - 1))}
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
              items={strategyOptions.map((value) => ({ value, label: value }))}
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
                      {value}
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
              className='sm:col-span-2'
              placeholder={t('Model Mapping')}
              value={groupForm.modelMapping}
              onChange={(event) =>
                setGroupForm((current) => ({
                  ...current,
                  modelMapping: event.target.value,
                }))
              }
            />
            <Textarea
              className='sm:col-span-2'
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
              className='sm:col-span-2'
              placeholder={t('Max concurrency')}
              value={accountForm.maxConcurrency}
              onChange={(event) =>
                setAccountForm((current) => ({
                  ...current,
                  maxConcurrency: event.target.value,
                }))
              }
            />
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
    </div>
  )
}
