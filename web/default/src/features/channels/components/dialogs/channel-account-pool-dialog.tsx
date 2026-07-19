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
import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Loader2,
  Pencil,
  Plus,
  Power,
  PowerOff,
  RefreshCw,
  ShieldOff,
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
import { ConfirmDialog } from '@/components/confirm-dialog'
import { StatusBadge } from '@/components/status-badge'
import {
  batchCreateChannelAccounts,
  createChannelAccount,
  deleteChannelAccount,
  getChannelAccounts,
  importMultiKeyToChannelAccounts,
  updateChannelAccount,
  updateChannelAccountStatus,
} from '../../api'
import { CHANNEL_STATUS } from '../../constants'
import { useChannelPermissions } from '../../hooks/use-channel-permissions'
import { channelsQueryKeys, formatTimestamp } from '../../lib'
import type { ChannelAccount, ChannelAccountPayload } from '../../types'
import { useChannels } from '../channels-provider'

type ChannelAccountPoolDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type AccountFormState = {
  id?: number
  name: string
  key: string
  models: string
  group: string
  priority: string
  weight: string
  maxConcurrency: string
}

const emptyForm: AccountFormState = {
  name: '',
  key: '',
  models: '',
  group: '',
  priority: '0',
  weight: '1',
  maxConcurrency: '0',
}

function numberOrZero(value: string): number {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return 0
  return parsed
}

function isCoolingDown(account: ChannelAccount, nowSeconds: number): boolean {
  return (
    account.rate_limited_until > nowSeconds ||
    account.overload_until > nowSeconds ||
    account.temp_disabled_until > nowSeconds
  )
}

function cooldownText(account: ChannelAccount, nowSeconds: number): string {
  const until = Math.max(
    account.rate_limited_until,
    account.overload_until,
    account.temp_disabled_until
  )
  if (until <= nowSeconds) return '-'
  return formatTimestamp(until)
}

function statusLabel(
  account: ChannelAccount,
  nowSeconds: number
): {
  label: string
  variant: 'success' | 'warning' | 'danger' | 'neutral'
} {
  if (account.status !== CHANNEL_STATUS.ENABLED) {
    return { label: 'Disabled', variant: 'danger' }
  }
  if (isCoolingDown(account, nowSeconds)) {
    return { label: 'Cooling Down', variant: 'warning' }
  }
  return { label: 'Enabled', variant: 'success' }
}

export function ChannelAccountPoolDialog(props: ChannelAccountPoolDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [statusFilter, setStatusFilter] = useState('all')
  const [search, setSearch] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [formState, setFormState] = useState<AccountFormState>(emptyForm)
  const [batchOpen, setBatchOpen] = useState(false)
  const [batchKeys, setBatchKeys] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<ChannelAccount | null>(null)
  const [actionLoading, setActionLoading] = useState(false)
  const permissions = useChannelPermissions()
  const noPermissionMessage = t("You don't have necessary permission")
  const canReadChannelAccounts = permissions.canReadChannelAccount
  const canOperateChannelAccounts = permissions.canOperateChannelAccount
  const canWriteChannelAccounts = permissions.canWriteChannelAccount
  const canSensitiveWriteChannelAccounts =
    permissions.canSensitiveWriteChannelAccount
  const canEditChannelAccounts =
    canWriteChannelAccounts || canSensitiveWriteChannelAccounts

  const channelId = currentRow?.id ?? 0
  const accountsQueryKey = [
    ...channelsQueryKeys.detail(channelId),
    'accounts',
    page,
    statusFilter,
    search,
  ] as const

  const accountsQuery = useQuery({
    queryKey: accountsQueryKey,
    queryFn: () =>
      getChannelAccounts(channelId, {
        p: page,
        page_size: 10,
        status: statusFilter === 'all' ? undefined : Number(statusFilter),
        search: search || undefined,
      }),
    enabled: props.open && channelId > 0 && canReadChannelAccounts,
  })

  const accounts = accountsQuery.data?.data?.accounts.items ?? []
  const total = accountsQuery.data?.data?.accounts.total ?? 0
  const pageSize = accountsQuery.data?.data?.accounts.page_size ?? 10
  const stats = accountsQuery.data?.data?.stats
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const nowSeconds = useMemo(() => Math.floor(Date.now() / 1000), [accounts])

  const resetForm = () => {
    setFormState(emptyForm)
    setFormOpen(false)
  }

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: accountsQueryKey })
    await queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
  }

  const openCreateForm = () => {
    if (!canSensitiveWriteChannelAccounts) {
      toast.error(noPermissionMessage)
      return
    }
    setFormState({
      ...emptyForm,
      models: currentRow?.models ?? '',
      group: currentRow?.group ?? '',
    })
    setFormOpen(true)
  }

  const openEditForm = (account: ChannelAccount) => {
    if (!canEditChannelAccounts) {
      toast.error(noPermissionMessage)
      return
    }
    setFormState({
      id: account.id,
      name: account.name,
      key: '',
      models: account.models,
      group: account.group,
      priority: String(account.priority),
      weight: String(account.weight || 1),
      maxConcurrency: String(account.max_concurrency || 0),
    })
    setFormOpen(true)
  }

  const buildPayload = (): ChannelAccountPayload => {
    const payload: ChannelAccountPayload = {
      name: formState.name.trim(),
      models: formState.models.trim(),
      group: formState.group.trim(),
      priority: numberOrZero(formState.priority),
      weight: numberOrZero(formState.weight),
      max_concurrency: numberOrZero(formState.maxConcurrency),
    }

    if (!formState.id) {
      payload.status = CHANNEL_STATUS.ENABLED
    }
    if (canSensitiveWriteChannelAccounts && formState.key.trim()) {
      payload.key = formState.key.trim()
    }

    return payload
  }

  const submitForm = async () => {
    if (!currentRow) return
    if (!canEditChannelAccounts) {
      toast.error(noPermissionMessage)
      return
    }
    if (
      !formState.id &&
      (!canSensitiveWriteChannelAccounts || !formState.key.trim())
    ) {
      toast.error(t('API key is required'))
      return
    }
    setActionLoading(true)
    try {
      const payload = buildPayload()
      const response = formState.id
        ? await updateChannelAccount(currentRow.id, formState.id, payload)
        : await createChannelAccount(currentRow.id, payload)
      if (!response.success) {
        throw new Error(response.message || t('Operation failed'))
      }
      toast.success(
        formState.id
          ? t('Account updated successfully')
          : t('Account created successfully')
      )
      resetForm()
      await refresh()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const performStatusAction = async (
    account: ChannelAccount,
    action: 'enable' | 'disable' | 'clear'
  ) => {
    if (!currentRow) return
    if (!canOperateChannelAccounts) {
      toast.error(noPermissionMessage)
      return
    }
    setActionLoading(true)
    try {
      let payload: {
        status?: number
        reason?: string
        clear_cooldown?: boolean
      }
      if (action === 'clear') {
        payload = { clear_cooldown: true, reason: '' }
      } else if (action === 'enable') {
        payload = {
          status: CHANNEL_STATUS.ENABLED,
          reason: '',
          clear_cooldown: true,
        }
      } else {
        payload = {
          status: CHANNEL_STATUS.MANUAL_DISABLED,
          reason: '',
          clear_cooldown: false,
        }
      }
      const response = await updateChannelAccountStatus(
        currentRow.id,
        account.id,
        payload
      )
      if (!response.success) {
        throw new Error(response.message || t('Operation failed'))
      }
      toast.success(t('Operation successful'))
      await refresh()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const performDelete = async () => {
    if (!currentRow || !deleteTarget) return
    if (!canSensitiveWriteChannelAccounts) {
      toast.error(noPermissionMessage)
      setDeleteTarget(null)
      return
    }
    setActionLoading(true)
    try {
      const response = await deleteChannelAccount(
        currentRow.id,
        deleteTarget.id
      )
      if (!response.success) {
        throw new Error(response.message || t('Operation failed'))
      }
      toast.success(t('Account deleted successfully'))
      setDeleteTarget(null)
      await refresh()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const submitBatch = async (importFromMultiKey = false) => {
    if (!currentRow) return
    if (!canSensitiveWriteChannelAccounts) {
      toast.error(noPermissionMessage)
      return
    }
    setActionLoading(true)
    try {
      const response = importFromMultiKey
        ? await importMultiKeyToChannelAccounts(currentRow.id, {
            name_prefix: currentRow.name,
            models: currentRow.models,
            group: currentRow.group,
          })
        : await batchCreateChannelAccounts(currentRow.id, {
            keys: batchKeys,
            name_prefix: currentRow.name,
            models: currentRow.models,
            group: currentRow.group,
            priority: currentRow.priority ?? 0,
            weight: currentRow.weight ?? 1,
            status: CHANNEL_STATUS.ENABLED,
          })
      if (!response.success) {
        throw new Error(response.message || t('Operation failed'))
      }
      toast.success(
        t('Imported {{created}} account(s), skipped {{skipped}}', {
          created: response.data?.created ?? 0,
          skipped: response.data?.skipped ?? 0,
        })
      )
      setBatchKeys('')
      setBatchOpen(false)
      await refresh()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  if (!currentRow) return null

  return (
    <>
      <Dialog open={props.open} onOpenChange={props.onOpenChange}>
        <DialogContent className='flex max-h-[92vh] !w-[calc(100vw-1rem)] !max-w-none flex-col sm:!w-[min(98vw,1560px)]'>
          <DialogHeader>
            <DialogTitle className='flex items-center gap-2'>
              {t('Account Pool')}
              <StatusBadge
                label={currentRow.name}
                variant='neutral'
                copyable={false}
              />
            </DialogTitle>
            <DialogDescription>
              {t('Manage account rotation for the selected channel.')}
            </DialogDescription>
          </DialogHeader>

          <div className='flex min-h-0 flex-1 flex-col gap-4 overflow-hidden'>
            <div className='grid gap-3 sm:grid-cols-4'>
              <StatusBadge
                label={`${t('Total')}: ${stats?.total ?? 0}`}
                variant='neutral'
                copyable={false}
              />
              <StatusBadge
                label={`${t('Enabled')}: ${stats?.enabled ?? 0}`}
                variant='success'
                copyable={false}
              />
              <StatusBadge
                label={`${t('Cooling Down')}: ${stats?.cooldown ?? 0}`}
                variant='warning'
                copyable={false}
              />
              <StatusBadge
                label={`${t('Disabled')}: ${stats?.disabled ?? 0}`}
                variant='danger'
                copyable={false}
              />
            </div>

            <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
              <div className='flex flex-1 flex-col gap-2 sm:flex-row sm:items-center'>
                <Input
                  value={search}
                  onChange={(event) => {
                    setSearch(event.target.value)
                    setPage(1)
                  }}
                  placeholder={t('Search accounts')}
                  className='sm:max-w-xs'
                />
                <Select
                  items={[
                    { value: 'all', label: t('All') },
                    { value: '1', label: t('Enabled') },
                    { value: '2', label: t('Disabled') },
                    { value: '3', label: t('Auto Disabled') },
                  ]}
                  value={statusFilter}
                  onValueChange={(value) => {
                    setStatusFilter(value ?? 'all')
                    setPage(1)
                  }}
                >
                  <SelectTrigger className='sm:w-40'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      <SelectItem value='all'>{t('All')}</SelectItem>
                      <SelectItem value='1'>{t('Enabled')}</SelectItem>
                      <SelectItem value='2'>{t('Disabled')}</SelectItem>
                      <SelectItem value='3'>{t('Auto Disabled')}</SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
              <div className='flex flex-wrap items-center gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => accountsQuery.refetch()}
                  disabled={!canReadChannelAccounts || accountsQuery.isFetching}
                  title={
                    canReadChannelAccounts ? undefined : noPermissionMessage
                  }
                >
                  <RefreshCw className='mr-2 h-4 w-4' />
                  {t('Refresh')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => {
                    if (!canSensitiveWriteChannelAccounts) {
                      toast.error(noPermissionMessage)
                      return
                    }
                    setBatchOpen((value) => !value)
                  }}
                  disabled={!canSensitiveWriteChannelAccounts}
                  title={
                    canSensitiveWriteChannelAccounts
                      ? undefined
                      : noPermissionMessage
                  }
                >
                  <Upload className='mr-2 h-4 w-4' />
                  {t('Batch Import')}
                </Button>
                <Button
                  type='button'
                  size='sm'
                  onClick={openCreateForm}
                  disabled={!canSensitiveWriteChannelAccounts}
                  title={
                    canSensitiveWriteChannelAccounts
                      ? undefined
                      : noPermissionMessage
                  }
                >
                  <Plus className='mr-2 h-4 w-4' />
                  {t('Add Account')}
                </Button>
              </div>
            </div>

            {batchOpen && (
              <div className='space-y-3 rounded-lg border p-3'>
                <Textarea
                  value={batchKeys}
                  onChange={(event) => setBatchKeys(event.target.value)}
                  placeholder={t('Enter one key per line for batch creation')}
                  rows={5}
                />
                <div className='flex flex-wrap gap-2'>
                  <Button
                    type='button'
                    size='sm'
                    onClick={() => submitBatch(false)}
                    disabled={
                      actionLoading || !canSensitiveWriteChannelAccounts
                    }
                    title={
                      canSensitiveWriteChannelAccounts
                        ? undefined
                        : noPermissionMessage
                    }
                  >
                    {actionLoading && (
                      <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                    )}
                    {t('Import Keys')}
                  </Button>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => submitBatch(true)}
                    disabled={
                      actionLoading || !canSensitiveWriteChannelAccounts
                    }
                    title={
                      canSensitiveWriteChannelAccounts
                        ? undefined
                        : noPermissionMessage
                    }
                  >
                    {t('Import from Multi-Key')}
                  </Button>
                </div>
              </div>
            )}

            {formOpen && (
              <div className='grid gap-3 rounded-lg border p-3 sm:grid-cols-2'>
                <Input
                  value={formState.name}
                  onChange={(event) =>
                    setFormState({ ...formState, name: event.target.value })
                  }
                  placeholder={t('Account name')}
                />
                <Input
                  value={formState.key}
                  onChange={(event) =>
                    setFormState({ ...formState, key: event.target.value })
                  }
                  disabled={
                    Boolean(formState.id) && !canSensitiveWriteChannelAccounts
                  }
                  placeholder={
                    formState.id && canSensitiveWriteChannelAccounts
                      ? t('Leave empty to keep existing key')
                      : formState.id
                        ? t('Sensitive channel settings are read-only')
                        : t('Enter secret key')
                  }
                />
                <Input
                  value={formState.models}
                  onChange={(event) =>
                    setFormState({ ...formState, models: event.target.value })
                  }
                  placeholder={t('Models inherited from channel if empty')}
                />
                <Input
                  value={formState.group}
                  onChange={(event) =>
                    setFormState({ ...formState, group: event.target.value })
                  }
                  placeholder={t('Group inherited from channel if empty')}
                />
                <Input
                  value={formState.priority}
                  onChange={(event) =>
                    setFormState({ ...formState, priority: event.target.value })
                  }
                  placeholder={t('Priority')}
                  inputMode='numeric'
                />
                <Input
                  value={formState.weight}
                  onChange={(event) =>
                    setFormState({ ...formState, weight: event.target.value })
                  }
                  placeholder={t('Weight')}
                  inputMode='numeric'
                />
                <Input
                  value={formState.maxConcurrency}
                  onChange={(event) =>
                    setFormState({
                      ...formState,
                      maxConcurrency: event.target.value,
                    })
                  }
                  placeholder={t('Max concurrency')}
                  inputMode='numeric'
                />
                <div className='flex items-center gap-2'>
                  <Button
                    type='button'
                    onClick={submitForm}
                    disabled={actionLoading || !canEditChannelAccounts}
                    title={
                      canEditChannelAccounts ? undefined : noPermissionMessage
                    }
                  >
                    {actionLoading && (
                      <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                    )}
                    {formState.id ? t('Save') : t('Create')}
                  </Button>
                  <Button type='button' variant='ghost' onClick={resetForm}>
                    {t('Cancel')}
                  </Button>
                </div>
              </div>
            )}

            <div className='min-h-0 flex-1 overflow-auto rounded-md border'>
              <Table className='min-w-[1180px]'>
                <TableHeader>
                  <TableRow>
                    <TableHead className='min-w-[160px]'>{t('Name')}</TableHead>
                    <TableHead className='min-w-[170px]'>{t('Key')}</TableHead>
                    <TableHead className='min-w-[120px]'>{t('Status')}</TableHead>
                    <TableHead className='min-w-[260px]'>{t('Models')}</TableHead>
                    <TableHead className='min-w-[130px]'>{t('Group')}</TableHead>
                    <TableHead className='min-w-[86px]'>{t('Priority')}</TableHead>
                    <TableHead className='min-w-[86px]'>{t('Weight')}</TableHead>
                    <TableHead className='min-w-[140px]'>{t('Cooldown')}</TableHead>
                    <TableHead className='min-w-[130px] text-right'>
                      {t('Actions')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {!canReadChannelAccounts ? (
                    <TableRow>
                      <TableCell
                        colSpan={9}
                        className='text-muted-foreground h-24 text-center'
                      >
                        {noPermissionMessage}
                      </TableCell>
                    </TableRow>
                  ) : accountsQuery.isLoading ? (
                    <TableRow>
                      <TableCell colSpan={9} className='h-24 text-center'>
                        <Loader2 className='mx-auto h-5 w-5 animate-spin' />
                      </TableCell>
                    </TableRow>
                  ) : accounts.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={9}
                        className='text-muted-foreground h-24 text-center'
                      >
                        {t('No accounts found')}
                      </TableCell>
                    </TableRow>
                  ) : (
                    accounts.map((account) => {
                      const status = statusLabel(account, nowSeconds)
                      return (
                        <TableRow key={account.id}>
                          <TableCell className='max-w-[220px] min-w-[160px] truncate font-medium'>
                            {account.name || `#${account.id}`}
                          </TableCell>
                          <TableCell className='min-w-[170px] font-mono text-xs'>
                            {account.key || '-'}
                          </TableCell>
                          <TableCell className='min-w-[120px]'>
                            <StatusBadge
                              label={t(status.label)}
                              variant={status.variant}
                              copyable={false}
                            />
                          </TableCell>
                          <TableCell
                            className='max-w-[420px] min-w-[260px] truncate'
                            title={account.models || t('Inherited')}
                          >
                            {account.models || t('Inherited')}
                          </TableCell>
                          <TableCell
                            className='max-w-[220px] min-w-[130px] truncate'
                            title={account.group || t('Inherited')}
                          >
                            {account.group || t('Inherited')}
                          </TableCell>
                          <TableCell className='min-w-[86px]'>
                            {account.priority}
                          </TableCell>
                          <TableCell className='min-w-[86px]'>
                            {account.weight || 1}
                          </TableCell>
                          <TableCell className='min-w-[140px] whitespace-nowrap'>
                            {cooldownText(account, nowSeconds)}
                          </TableCell>
                          <TableCell className='min-w-[130px]'>
                            <div className='flex justify-end gap-1'>
                              <Button
                                type='button'
                                variant='ghost'
                                size='icon-sm'
                                onClick={() => openEditForm(account)}
                                disabled={!canEditChannelAccounts}
                                title={
                                  canEditChannelAccounts
                                    ? undefined
                                    : noPermissionMessage
                                }
                                aria-label={t('Edit')}
                              >
                                <Pencil className='h-4 w-4' />
                              </Button>
                              <Button
                                type='button'
                                variant='ghost'
                                size='icon-sm'
                                onClick={() =>
                                  performStatusAction(
                                    account,
                                    account.status === CHANNEL_STATUS.ENABLED
                                      ? 'disable'
                                      : 'enable'
                                  )
                                }
                                disabled={!canOperateChannelAccounts}
                                title={
                                  canOperateChannelAccounts
                                    ? undefined
                                    : noPermissionMessage
                                }
                                aria-label={
                                  account.status === CHANNEL_STATUS.ENABLED
                                    ? t('Disable')
                                    : t('Enable')
                                }
                              >
                                {account.status === CHANNEL_STATUS.ENABLED ? (
                                  <PowerOff className='h-4 w-4' />
                                ) : (
                                  <Power className='h-4 w-4' />
                                )}
                              </Button>
                              <Button
                                type='button'
                                variant='ghost'
                                size='icon-sm'
                                onClick={() =>
                                  performStatusAction(account, 'clear')
                                }
                                disabled={!canOperateChannelAccounts}
                                title={
                                  canOperateChannelAccounts
                                    ? undefined
                                    : noPermissionMessage
                                }
                                aria-label={t('Clear cooldown')}
                              >
                                <ShieldOff className='h-4 w-4' />
                              </Button>
                              <Button
                                type='button'
                                variant='ghost'
                                size='icon-sm'
                                onClick={() => {
                                  if (!canSensitiveWriteChannelAccounts) {
                                    toast.error(noPermissionMessage)
                                    return
                                  }
                                  setDeleteTarget(account)
                                }}
                                disabled={!canSensitiveWriteChannelAccounts}
                                title={
                                  canSensitiveWriteChannelAccounts
                                    ? undefined
                                    : noPermissionMessage
                                }
                                aria-label={t('Delete')}
                                className='text-destructive hover:text-destructive'
                              >
                                <Trash2 className='h-4 w-4' />
                              </Button>
                            </div>
                          </TableCell>
                        </TableRow>
                      )
                    })
                  )}
                </TableBody>
              </Table>
            </div>

            <div className='flex items-center justify-between'>
              <div className='text-muted-foreground text-sm'>
                {t('Page {{page}} of {{total}}', {
                  page,
                  total: totalPages,
                })}
              </div>
              <div className='flex gap-2'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={page <= 1}
                  onClick={() => setPage((value) => Math.max(1, value - 1))}
                >
                  {t('Previous')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={page >= totalPages}
                  onClick={() =>
                    setPage((value) => Math.min(totalPages, value + 1))
                  }
                >
                  {t('Next')}
                </Button>
              </div>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t('Delete Account')}
        desc={t('Are you sure you want to delete this account?')}
        confirmText={t('Delete')}
        destructive
        handleConfirm={performDelete}
      />
    </>
  )
}
