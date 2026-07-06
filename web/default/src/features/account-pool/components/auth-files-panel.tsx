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
import { useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { FileJson, Loader2, Pencil, Trash2, Upload } from 'lucide-react'
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
import { Switch } from '@/components/ui/switch'
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
  deleteAccountPoolAuthFile,
  getAccountPoolAuthFiles,
  importAccountPoolAuthFiles,
  updateAccountPoolAuthFile,
} from '../api'
import type {
  AccountPoolAuthFile,
  AccountPoolAuthFilePayload,
  AccountPoolAuthFileUpdatePayload,
  AccountPoolGroup,
} from '../types'

type AuthFilesPanelProps = {
  groups: AccountPoolGroup[]
}

type AuthFileFormState = {
  id?: number
  name: string
  content: string
  poolGroupId: string
  groupName: string
  provider: string
  platform: string
  authType: string
  accountGroups: string
  models: string
  proxy: string
  baseUrl: string
  priority: string
  weight: string
  maxConcurrency: string
  status: string
  skipDuplicates: boolean
}

const authTypeOptions = [
  'api_key',
  'official_oauth',
  'cookie',
  'service_account',
  'custom_json',
]

const emptyForm: AuthFileFormState = {
  name: '',
  content: '',
  poolGroupId: 'auto',
  groupName: '',
  provider: '',
  platform: '',
  authType: '',
  accountGroups: '',
  models: '',
  proxy: '',
  baseUrl: '',
  priority: '',
  weight: '',
  maxConcurrency: '',
  status: '',
  skipDuplicates: true,
}

function splitCSV(value: string): string[] {
  return value
    .split(/[\n,;]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function numberOrUndefined(value: string): number | undefined {
  const trimmed = value.trim()
  if (!trimmed) return undefined
  const parsed = Number(trimmed)
  return Number.isFinite(parsed) ? parsed : undefined
}

function authFileStatusVariant(
  authFile: AccountPoolAuthFile
): 'success' | 'danger' | 'neutral' {
  if (authFile.status === CHANNEL_STATUS.ENABLED) return 'success'
  if (authFile.status === CHANNEL_STATUS.MANUAL_DISABLED) return 'danger'
  return 'neutral'
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

function maskSecret(value: string): string {
  if (value.length <= 12) return shortenMiddle(value, 4, 4)
  return `${value.slice(0, 6)}...${value.slice(-4)}`
}

function limitInlineText(value: string, maxLength = 64): string {
  if (value.length <= maxLength) return value
  return `${value.slice(0, maxLength)}...`
}

function formatCompactCredentialSummary(summary: string): string {
  const record = credentialSummaryRecord(summary)
  const email = summaryStringValue(record, [
    'email',
    'account',
    'username',
    'client_email',
  ])
  if (email) return limitInlineText(email)

  const accountId = summaryStringValue(record, [
    'account_id',
    'id',
    'user_id',
    'subject',
  ])
  if (accountId) return `account_id: ${shortenMiddle(accountId)}`

  const secret = summaryStringValue(record, [
    'api_key',
    'access_token',
    'refresh_token',
    'session_token',
  ])
  if (secret) return maskSecret(secret)

  return limitInlineText(formatCredentialSummary(summary))
}

function assignedGroups(authFile: AccountPoolAuthFile): string[] {
  const groups = authFile.account_groups?.filter(Boolean) ?? []
  if (groups.length > 0) return groups
  if (authFile.account_group) return [authFile.account_group]
  if (authFile.pool_group_id > 0) return [`#${authFile.pool_group_id}`]
  return []
}

function assignedGroupsText(authFile: AccountPoolAuthFile): string {
  const groups = assignedGroups(authFile)
  if (groups.length === 0) return '-'
  const visibleGroups = groups.slice(0, 2).join(', ')
  return groups.length > 2
    ? `${visibleGroups} +${groups.length - 2}`
    : visibleGroups
}

function assignedGroupsTitle(authFile: AccountPoolAuthFile): string {
  const groups = assignedGroups(authFile)
  return groups.length > 0 ? groups.join(', ') : '-'
}

function sourceText(authFile: AccountPoolAuthFile): string {
  return [authFile.platform, authFile.auth_type].filter(Boolean).join(' / ')
}

function subscriptionTypeText(authFile: AccountPoolAuthFile): string {
  const value = authFile.subscription_type?.trim()
  return value ? limitInlineText(value, 28) : '-'
}

function ruleText(
  authFile: AccountPoolAuthFile,
  t: (key: string) => string
): string {
  const models = authFile.models || t('Inherited')
  const concurrency =
    authFile.max_concurrency > 0
      ? `${t('Max concurrency')}: ${authFile.max_concurrency}`
      : ''
  return [limitInlineText(models, 36), concurrency].filter(Boolean).join(' · ')
}

function ruleTitle(
  authFile: AccountPoolAuthFile,
  t: (key: string) => string
): string {
  return [
    `${t('Models')}: ${authFile.models || t('Inherited')}`,
    `${t('Max concurrency')}: ${authFile.max_concurrency || 0}`,
    `${t('Priority')}: ${authFile.priority}`,
    `${t('Weight')}: ${authFile.weight}`,
    `${t('Proxy')}: ${authFile.proxy || '-'}`,
  ].join('\n')
}

function accountTitle(
  authFile: AccountPoolAuthFile,
  fullSummary: string,
  t: (key: string) => string
): string {
  return [
    `${t('Account')}: ${
      authFile.pool_account_id > 0 ? `#${authFile.pool_account_id}` : '-'
    }`,
    `${t('File')}: ${authFile.name} (#${authFile.id})`,
    fullSummary,
  ]
    .filter(Boolean)
    .join('\n')
}

function authFileFormFromRow(authFile: AccountPoolAuthFile): AuthFileFormState {
  return {
    id: authFile.id,
    name: authFile.name,
    content: '',
    poolGroupId: authFile.pool_group_id
      ? String(authFile.pool_group_id)
      : 'auto',
    groupName: '',
    provider: authFile.provider,
    platform: authFile.platform,
    authType: authFile.auth_type,
    accountGroups: authFile.account_groups?.join('\n') ?? '',
    models: authFile.models ?? '',
    proxy: authFile.proxy ?? '',
    baseUrl: authFile.base_url ?? '',
    priority: String(authFile.priority ?? 0),
    weight: String(authFile.weight || 1),
    maxConcurrency: String(authFile.max_concurrency || 0),
    status: String(authFile.status || CHANNEL_STATUS.ENABLED),
    skipDuplicates: true,
  }
}

export function AuthFilesPanel({ groups }: AuthFilesPanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const [page, setPage] = useState(1)
  const [groupFilterId, setGroupFilterId] = useState<number | null>(null)
  const [search, setSearch] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [form, setForm] = useState<AuthFileFormState>(emptyForm)
  const [actionLoading, setActionLoading] = useState(false)

  const queryParams = useMemo(
    () => ({
      p: page,
      page_size: 10,
      pool_group_id: groupFilterId || undefined,
      search: search.trim() || undefined,
    }),
    [groupFilterId, page, search]
  )

  const authFilesQuery = useQuery({
    queryKey: accountPoolQueryKeys.authFiles(queryParams),
    queryFn: () => getAccountPoolAuthFiles(queryParams),
  })

  const authFiles = authFilesQuery.data?.data?.items ?? []
  const pageInfo = authFilesQuery.data?.data
  const totalPages = Math.max(
    1,
    Math.ceil((pageInfo?.total ?? 0) / (pageInfo?.page_size ?? 10))
  )

  const invalidateAccountPool = async () => {
    await queryClient.invalidateQueries({ queryKey: ['account-pool'] })
    await queryClient.invalidateQueries({ queryKey: ['channels'] })
  }

  const openImport = () => {
    setForm({
      ...emptyForm,
      poolGroupId: groupFilterId ? String(groupFilterId) : 'auto',
    })
    setFormOpen(true)
  }

  const openEdit = (authFile: AccountPoolAuthFile) => {
    setForm(authFileFormFromRow(authFile))
    setFormOpen(true)
  }

  const buildCreatePayload = (): AccountPoolAuthFilePayload => ({
    name: form.name.trim() || undefined,
    content: form.content.trim(),
    pool_group_id:
      form.poolGroupId !== 'auto' ? Number(form.poolGroupId) : undefined,
    group_name: form.groupName.trim() || undefined,
    provider: form.provider.trim() || undefined,
    platform: form.platform.trim() || undefined,
    auth_type: form.authType || undefined,
    account_groups: splitCSV(form.accountGroups),
    models: form.models.trim() || undefined,
    proxy: form.proxy.trim() || undefined,
    base_url: form.baseUrl.trim() || undefined,
    priority: numberOrUndefined(form.priority),
    weight: numberOrUndefined(form.weight),
    max_concurrency: numberOrUndefined(form.maxConcurrency),
    status: numberOrUndefined(form.status),
    skip_duplicates: form.skipDuplicates,
  })

  const buildUpdatePayload = (): AccountPoolAuthFileUpdatePayload => ({
    name: form.name.trim() || undefined,
    content: form.content.trim() || undefined,
    pool_group_id:
      form.poolGroupId !== 'auto' ? Number(form.poolGroupId) : undefined,
    group_name: form.groupName.trim() || undefined,
    provider: form.provider.trim() || undefined,
    platform: form.platform.trim() || undefined,
    auth_type: form.authType || undefined,
    account_groups: splitCSV(form.accountGroups),
    models: form.models.trim(),
    proxy: form.proxy.trim(),
    base_url: form.baseUrl.trim(),
    priority: numberOrUndefined(form.priority),
    weight: numberOrUndefined(form.weight),
    max_concurrency: numberOrUndefined(form.maxConcurrency),
    status: numberOrUndefined(form.status),
  })

  const submitAuthFile = async () => {
    if (!form.id && !form.content.trim()) {
      toast.error(t('Auth file JSON is required'))
      return
    }
    setActionLoading(true)
    try {
      if (form.id) {
        const response = await updateAccountPoolAuthFile(
          form.id,
          buildUpdatePayload()
        )
        if (!response.success) throw new Error(response.message)
        toast.success(t('Operation successful'))
      } else {
        const response = await importAccountPoolAuthFiles(buildCreatePayload())
        if (!response.success) throw new Error(response.message)
        toast.success(
          t(
            'Imported {{created}} auth file(s), skipped {{skipped}}, failed {{failed}}',
            {
              created: response.data?.created ?? 0,
              skipped: response.data?.skipped ?? 0,
              failed: response.data?.failed ?? 0,
            }
          )
        )
        if ((response.data?.errors?.length ?? 0) > 0) {
          toast.error(
            response.data?.errors?.[0]?.message ?? t('Operation failed')
          )
        }
      }
      setFormOpen(false)
      setForm(emptyForm)
      await invalidateAccountPool()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const deleteAuthFile = async (authFile: AccountPoolAuthFile) => {
    if (
      !window.confirm(t('Delete this credential and its linked pool account?'))
    ) {
      return
    }
    setActionLoading(true)
    try {
      const response = await deleteAccountPoolAuthFile(authFile.id, true)
      if (!response.success) throw new Error(response.message)
      toast.success(t('Operation successful'))
      await invalidateAccountPool()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setActionLoading(false)
    }
  }

  const readSelectedFile = async (file: File) => {
    const text =
      typeof file.text === 'function'
        ? await file.text()
        : await new Promise<string>((resolve, reject) => {
            const reader = new FileReader()
            reader.onload = () => resolve(String(reader.result ?? ''))
            reader.onerror = () =>
              reject(reader.error || new Error('Failed to read file'))
            reader.readAsText(file)
          })
    setForm((current) => ({
      ...current,
      name: current.name || file.name.replace(/\.json$/i, ''),
      content: text,
    }))
  }

  return (
    <div className='flex min-h-0 flex-col'>
      <div className='border-border flex flex-col gap-3 border-b p-3 lg:flex-row lg:items-center lg:justify-end'>
        <div className='flex flex-wrap gap-2 lg:justify-end'>
          <Input
            className='w-[220px]'
            placeholder={t('Search credentials')}
            value={search}
            onChange={(event) => {
              setPage(1)
              setSearch(event.target.value)
            }}
          />
          <Select
            items={[
              { value: 'all', label: t('All Groups') },
              ...groups.map((group) => ({
                value: String(group.id),
                label: group.name,
              })),
            ]}
            value={groupFilterId ? String(groupFilterId) : 'all'}
            onValueChange={(value) => {
              setPage(1)
              if (!value || value === 'all') {
                setGroupFilterId(null)
                return
              }
              setGroupFilterId(Number(value))
            }}
          >
            <SelectTrigger className='w-[180px]'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value='all'>{t('All Groups')}</SelectItem>
                {groups.map((group) => (
                  <SelectItem key={group.id} value={String(group.id)}>
                    {group.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Button
            variant='outline'
            onClick={() => void invalidateAccountPool()}
          >
            {authFilesQuery.isFetching ? (
              <Loader2 data-icon='inline-start' className='animate-spin' />
            ) : (
              <FileJson data-icon='inline-start' />
            )}
            {t('Refresh')}
          </Button>
          <Button onClick={openImport}>
            <Upload data-icon='inline-start' />
            {t('Import Credential')}
          </Button>
        </div>
      </div>

      <div className='overflow-x-auto'>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Account')}</TableHead>
              <TableHead>{t('Source')}</TableHead>
              <TableHead>{t('Type')}</TableHead>
              <TableHead>{t('Groups')}</TableHead>
              <TableHead>{t('Rules')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {authFiles.map((authFile) => {
              const fullSummary = formatCredentialSummary(
                authFile.credential_summary
              )
              const accountSummary =
                formatCompactCredentialSummary(authFile.credential_summary)
              const groupText = assignedGroupsText(authFile)
              const sourceSummary = sourceText(authFile)
              const subscriptionType = subscriptionTypeText(authFile)
              const ruleSummary = ruleText(authFile, t)

              return (
                <TableRow key={authFile.id}>
                  <TableCell className='min-w-[280px] max-w-[440px]'>
                    <div
                      className='truncate font-medium'
                      title={accountTitle(authFile, fullSummary, t)}
                    >
                      {accountSummary === '-' ? authFile.name : accountSummary}
                    </div>
                    <div
                      className='text-muted-foreground truncate text-xs'
                      title={`${t('File')}: ${authFile.name} (#${authFile.id})`}
                    >
                      {t('File')}: {authFile.name} · #{authFile.id}
                    </div>
                  </TableCell>
                  <TableCell className='min-w-[150px] max-w-[190px]'>
                    <div className='truncate text-sm'>
                      {authFile.provider || '-'}
                    </div>
                    <div
                      className='text-muted-foreground truncate text-xs'
                      title={sourceSummary}
                    >
                      {sourceSummary || '-'}
                    </div>
                  </TableCell>
                  <TableCell
                    className='min-w-[90px] max-w-[130px]'
                    title={`${t('Type')}: ${subscriptionType}`}
                  >
                    {subscriptionType === '-' ? (
                      <span className='text-muted-foreground text-xs'>-</span>
                    ) : (
                      <span className='border-border bg-muted/50 inline-flex max-w-full items-center rounded-md border px-2 py-0.5 text-xs font-medium'>
                        <span className='truncate'>{subscriptionType}</span>
                      </span>
                    )}
                  </TableCell>
                  <TableCell
                    className='min-w-[150px] max-w-[220px] truncate text-xs font-medium'
                    title={[
                      assignedGroupsTitle(authFile),
                      authFile.pool_account_id > 0
                        ? `${t('Account')}: #${authFile.pool_account_id}`
                        : '',
                    ]
                      .filter(Boolean)
                      .join('\n')}
                  >
                    {groupText}
                  </TableCell>
                  <TableCell
                    className='min-w-[190px] max-w-[260px] truncate text-xs'
                    title={ruleTitle(authFile, t)}
                  >
                    {ruleSummary || '-'}
                  </TableCell>
                  <TableCell className='min-w-[140px]'>
                    <div className='flex flex-col gap-1'>
                      <StatusBadge
                        label={
                          authFile.status === CHANNEL_STATUS.ENABLED
                            ? t('Enabled')
                            : t('Disabled')
                        }
                        variant={authFileStatusVariant(authFile)}
                        copyable={false}
                      />
                      <span className='text-muted-foreground text-xs'>
                        {authFile.last_imported_time
                          ? formatTimestamp(authFile.last_imported_time)
                          : '-'}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className='min-w-[96px]'>
                    <div className='flex flex-nowrap gap-1.5'>
                      <Button
                        variant='ghost'
                        size='icon-sm'
                        aria-label={t('Edit')}
                        title={t('Edit')}
                        onClick={() => openEdit(authFile)}
                      >
                        <Pencil />
                      </Button>
                      <Button
                        variant='ghost'
                        size='icon-sm'
                        aria-label={t('Delete')}
                        title={t('Delete')}
                        onClick={() => void deleteAuthFile(authFile)}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              )
            })}
            {!authFilesQuery.isLoading && authFiles.length === 0 && (
              <TableRow>
                <TableCell colSpan={7} className='h-24 text-center'>
                  {t('No credentials found')}
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

      <Dialog open={formOpen} onOpenChange={setFormOpen}>
        <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-3xl'>
          <DialogHeader>
            <DialogTitle>
              {t(form.id ? 'Edit Credential' : 'Import Credential')}
            </DialogTitle>
            <DialogDescription>
              {t(
                'Import native, sub2, newapi, or Sub2api JSON credentials into the native account pool.'
              )}
            </DialogDescription>
          </DialogHeader>

          <div className='grid gap-3 sm:grid-cols-2'>
            <Input
              placeholder={t('Name')}
              value={form.name}
              onChange={(event) =>
                setForm((current) => ({ ...current, name: event.target.value }))
              }
            />
            <Select
              items={[
                { value: 'auto', label: t('Auto group') },
                ...groups.map((group) => ({
                  value: String(group.id),
                  label: group.name,
                })),
              ]}
              value={form.poolGroupId}
              onValueChange={(value) =>
                setForm((current) => ({
                  ...current,
                  poolGroupId: value ?? 'auto',
                }))
              }
            >
              <SelectTrigger className='w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  <SelectItem value='auto'>{t('Auto group')}</SelectItem>
                  {groups.map((group) => (
                    <SelectItem key={group.id} value={String(group.id)}>
                      {group.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
            <Input
              placeholder={t('Auto group name')}
              value={form.groupName}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  groupName: event.target.value,
                }))
              }
            />
            <Input
              placeholder={t('Provider override')}
              value={form.provider}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  provider: event.target.value,
                }))
              }
            />
            <Input
              placeholder={t('Platform override')}
              value={form.platform}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  platform: event.target.value,
                }))
              }
            />
            <Select
              items={authTypeOptions.map((value) => ({ value, label: value }))}
              value={form.authType || 'custom_json'}
              onValueChange={(value) =>
                setForm((current) => ({
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
              placeholder={t('Models')}
              value={form.models}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  models: event.target.value,
                }))
              }
            />
            <Input
              placeholder={t('Proxy')}
              value={form.proxy}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  proxy: event.target.value,
                }))
              }
            />
            <Input
              placeholder={t('Base URL')}
              value={form.baseUrl}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  baseUrl: event.target.value,
                }))
              }
            />
            <Input
              placeholder={t('Account groups')}
              value={form.accountGroups}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  accountGroups: event.target.value,
                }))
              }
            />
            <Input
              placeholder={t('Priority')}
              value={form.priority}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  priority: event.target.value,
                }))
              }
            />
            <Input
              placeholder={t('Weight')}
              value={form.weight}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  weight: event.target.value,
                }))
              }
            />
            <Input
              placeholder={t('Max concurrency')}
              value={form.maxConcurrency}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  maxConcurrency: event.target.value,
                }))
              }
            />
            <Select
              items={[
                { value: 'auto', label: t('Auto') },
                { value: String(CHANNEL_STATUS.ENABLED), label: t('Enabled') },
                {
                  value: String(CHANNEL_STATUS.MANUAL_DISABLED),
                  label: t('Disabled'),
                },
              ]}
              value={form.status || 'auto'}
              onValueChange={(value) =>
                setForm((current) => ({
                  ...current,
                  status: value === 'auto' ? '' : (value ?? ''),
                }))
              }
            >
              <SelectTrigger className='w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  <SelectItem value='auto'>{t('Auto')}</SelectItem>
                  <SelectItem value={String(CHANNEL_STATUS.ENABLED)}>
                    {t('Enabled')}
                  </SelectItem>
                  <SelectItem value={String(CHANNEL_STATUS.MANUAL_DISABLED)}>
                    {t('Disabled')}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            {!form.id && (
              <div className='flex items-center justify-between gap-4 rounded-lg border p-3 sm:col-span-2'>
                <div>
                  <div className='text-sm font-medium'>
                    {t('Skip duplicate auth files')}
                  </div>
                  <div className='text-muted-foreground text-xs'>
                    {t('Duplicates are detected by the original JSON digest.')}
                  </div>
                </div>
                <Switch
                  checked={form.skipDuplicates}
                  onCheckedChange={(checked) =>
                    setForm((current) => ({
                      ...current,
                      skipDuplicates: !!checked,
                    }))
                  }
                />
              </div>
            )}
            <div className='flex items-center justify-between gap-3 sm:col-span-2'>
              <Button
                type='button'
                variant='outline'
                onClick={() => fileInputRef.current?.click()}
              >
                <FileJson data-icon='inline-start' />
                {t('Choose JSON file')}
              </Button>
              <input
                ref={fileInputRef}
                type='file'
                accept='application/json,.json'
                className='hidden'
                onChange={(event) => {
                  const file = event.target.files?.[0]
                  if (file) void readSelectedFile(file)
                  event.target.value = ''
                }}
              />
            </div>
            <Textarea
              className='font-mono text-xs sm:col-span-2'
              rows={10}
              placeholder={
                form.id
                  ? t('Leave empty to keep existing auth file JSON')
                  : t('Auth file JSON')
              }
              value={form.content}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  content: event.target.value,
                }))
              }
            />
          </div>

          <DialogFooter>
            <Button onClick={submitAuthFile} disabled={actionLoading}>
              {actionLoading && (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              )}
              {t(form.id ? 'Save' : 'Import')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
