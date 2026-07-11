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
import type { ReactNode } from 'react'
import { Database } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { StatusBadge } from '@/components/status-badge'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import { formatTimestamp } from '@/features/channels/lib'
import type {
  AccountPoolCheckTask,
  AccountPoolStateLog,
  AccountPoolUsageLog,
} from '../types'

type AccountPoolLogMobileListProps<TItem> = {
  items: TItem[]
  isLoading?: boolean
  emptyTitle: string
}

type AccountPoolUsageLogsMobileListProps =
  AccountPoolLogMobileListProps<AccountPoolUsageLog> & {
    onFilterRequest: (requestId: string) => void
    onFilterAccount: (accountId: number, label: string) => void
  }

type AccountPoolStateLogsMobileListProps =
  AccountPoolLogMobileListProps<AccountPoolStateLog> & {
    onFilterRequest: (requestId: string) => void
    onFilterAccount: (accountId: number, label: string) => void
  }

type AccountPoolCheckTasksMobileListProps =
  AccountPoolLogMobileListProps<AccountPoolCheckTask> & {
    onViewTask: (task: AccountPoolCheckTask) => void
  }

function formatUsageNumber(value: number): string {
  return new Intl.NumberFormat().format(value || 0)
}

function formatUsageDuration(seconds: number): string {
  if (!seconds || seconds <= 0) return '-'
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  return rest > 0 ? `${minutes}m ${rest}s` : `${minutes}m`
}

function accountStateText(
  status: number,
  schedulable: boolean,
  unavailable: boolean,
  t: (key: string) => string
): string {
  if (status !== CHANNEL_STATUS.ENABLED || !schedulable) return t('Disabled')
  if (unavailable) return t('Unavailable')
  return t('Enabled')
}

function accountStateVariant(
  status: number,
  schedulable: boolean,
  unavailable: boolean
): 'success' | 'warning' | 'danger' | 'neutral' {
  if (status !== CHANNEL_STATUS.ENABLED || !schedulable) return 'danger'
  if (unavailable) return 'warning'
  return 'success'
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

function checkTaskProgressValue(task: AccountPoolCheckTask): number {
  if (!task.total) return 0
  const progressed = Math.min(task.total, task.checked + task.skipped)
  return Math.round((progressed / task.total) * 100)
}

function AccountPoolLogMobileSkeleton({
  kind,
}: {
  kind: 'usage' | 'state' | 'check'
}) {
  return (
    <div
      data-account-pool-log-mobile-list={kind}
      className='bg-card overflow-hidden rounded-lg border md:hidden'
    >
      {[1, 2, 3].map((item) => (
        <div
          key={item}
          className='flex flex-col gap-2.5 border-b p-3 last:border-b-0'
        >
          <div className='flex items-center justify-between gap-3'>
            <Skeleton className='h-5 w-40 rounded-md' />
            <Skeleton className='h-5 w-20 rounded-md' />
          </div>
          <div className='grid grid-cols-2 gap-2'>
            {[1, 2, 3, 4, 5, 6].map((field) => (
              <div key={field} className='flex min-w-0 flex-col gap-1'>
                <Skeleton className='h-3 w-12 rounded' />
                <Skeleton className='h-4 w-full rounded' />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function AccountPoolLogMobileEmpty({
  kind,
  title,
  description,
}: {
  kind: 'usage' | 'state' | 'check'
  title: string
  description: string
}) {
  return (
    <div
      data-account-pool-log-mobile-list={kind}
      className='rounded-lg border p-6 md:hidden'
    >
      <Empty className='border-none p-0'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <Database className='size-6' />
          </EmptyMedia>
          <EmptyTitle>{title}</EmptyTitle>
          <EmptyDescription>{description}</EmptyDescription>
        </EmptyHeader>
      </Empty>
    </div>
  )
}

function MobileCard({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'border-b border-l-2 border-l-transparent p-3 last:border-b-0',
        className
      )}
    >
      {children}
    </div>
  )
}

function MobileField({
  label,
  children,
  className,
}: {
  label: string
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'bg-muted/25 flex min-w-0 flex-col gap-1 rounded-md px-2 py-1.5',
        className
      )}
    >
      <div className='text-muted-foreground text-[11px] leading-none font-medium select-none'>
        {label}
      </div>
      <div className='min-w-0 text-xs leading-tight break-words'>
        {children || '-'}
      </div>
    </div>
  )
}

function RequestFilterButton({
  requestId,
  onFilter,
}: {
  requestId?: string
  onFilter: (requestId: string) => void
}) {
  const { t } = useTranslation()
  if (!requestId) return <span className='text-muted-foreground/70'>-</span>

  return (
    <Button
      variant='link'
      size='xs'
      className='h-auto max-w-full justify-start truncate p-0 text-xs'
      title={t('Click to filter by request')}
      onClick={() => onFilter(requestId)}
    >
      {requestId}
    </Button>
  )
}

function AccountFilterButton({
  accountId,
  label,
  onFilter,
}: {
  accountId: number
  label: string
  onFilter: (accountId: number, label: string) => void
}) {
  const { t } = useTranslation()
  if (accountId <= 0) return <span>{label}</span>

  return (
    <Button
      variant='link'
      size='xs'
      className='h-auto max-w-full justify-start truncate p-0 text-xs font-medium'
      title={t('Click to filter by account')}
      onClick={() => onFilter(accountId, label)}
    >
      {label}
    </Button>
  )
}

function AccountStateSnapshot({
  label,
  status,
  schedulable,
  unavailable,
  message,
  disabledReason,
  nextRetryTime,
}: {
  label: string
  status: number
  schedulable: boolean
  unavailable: boolean
  message: string
  disabledReason: string
  nextRetryTime: number
}) {
  const { t } = useTranslation()
  const stateText = accountStateText(status, schedulable, unavailable, t)
  const detail = message || disabledReason || '-'

  return (
    <MobileField label={label}>
      <div className='flex flex-col gap-1'>
        <StatusBadge
          label={stateText}
          variant={accountStateVariant(status, schedulable, unavailable)}
          copyable={false}
        />
        <span className='text-muted-foreground break-words'>{detail}</span>
        {nextRetryTime > 0 ? (
          <span className='text-muted-foreground'>
            {t('Next retry')}: {formatTimestamp(nextRetryTime)}
          </span>
        ) : null}
      </div>
    </MobileField>
  )
}

export function AccountPoolUsageLogsMobileList({
  items,
  isLoading = false,
  emptyTitle,
  onFilterRequest,
  onFilterAccount,
}: AccountPoolUsageLogsMobileListProps) {
  const { t } = useTranslation()

  if (isLoading) return <AccountPoolLogMobileSkeleton kind='usage' />
  if (items.length === 0) {
    return (
      <AccountPoolLogMobileEmpty
        kind='usage'
        title={emptyTitle}
        description={t(
          'No usage logs available. Logs will appear here once API calls are made.'
        )}
      />
    )
  }

  return (
    <div
      data-account-pool-log-mobile-list='usage'
      className='bg-card overflow-hidden rounded-lg border md:hidden'
    >
      {items.map((log) => {
        const accountLabel =
          log.pool_account_name || `#${log.pool_account_id || '-'}`
        const totalTokens = log.prompt_tokens + log.completion_tokens

        return (
          <MobileCard
            key={log.id}
            className={
              log.success ? 'border-l-success/60' : 'border-l-destructive'
            }
          >
            <div className='flex min-w-0 flex-col gap-2.5'>
              <div className='flex min-w-0 items-start justify-between gap-3'>
                <div className='min-w-0'>
                  <div className='truncate text-sm font-medium'>
                    {log.model_name || '-'}
                  </div>
                  <div className='text-muted-foreground mt-0.5 truncate text-xs'>
                    {accountLabel} ·{' '}
                    {log.pool_group_name || `#${log.pool_group_id}`}
                  </div>
                </div>
                <StatusBadge
                  label={log.success ? t('Success') : t('Failed')}
                  variant={log.success ? 'success' : 'danger'}
                  copyable={false}
                />
              </div>

              <div className='grid grid-cols-2 gap-1.5'>
                <MobileField label={t('Time')}>
                  <div className='font-mono tabular-nums'>
                    {formatTimestamp(log.created_at)}
                  </div>
                </MobileField>
                <MobileField label={t('Request')}>
                  <RequestFilterButton
                    requestId={log.request_id}
                    onFilter={onFilterRequest}
                  />
                </MobileField>
                <MobileField label={t('Account')}>
                  <AccountFilterButton
                    accountId={log.pool_account_id}
                    label={accountLabel}
                    onFilter={onFilterAccount}
                  />
                  <div className='text-muted-foreground mt-1 truncate'>
                    {log.pool_account_auth_type || '-'}
                  </div>
                </MobileField>
                <MobileField label={t('Channel')}>
                  <div className='truncate'>
                    {log.channel_name || `#${log.channel_id}`}
                  </div>
                  <div className='text-muted-foreground mt-1 truncate'>
                    {log.username || '-'} / {log.token_name || '-'}
                  </div>
                </MobileField>
                <MobileField label={t('Usage')}>
                  {t('Quota')}: {formatUsageNumber(log.quota)}
                  <div className='text-muted-foreground mt-1'>
                    {t('Tokens')}: {formatUsageNumber(totalTokens)} ·{' '}
                    {formatUsageDuration(log.use_time)}
                  </div>
                </MobileField>
                <MobileField label={t('Result')}>
                  <div>{log.is_stream ? t('Stream') : t('Non-stream')}</div>
                  {!log.success ? (
                    <div className='text-muted-foreground mt-1 break-words'>
                      {log.status_code ? `${log.status_code} · ` : ''}
                      {log.error_message || log.error_code || '-'}
                    </div>
                  ) : null}
                  {log.retry_index > 0 ? (
                    <div className='text-muted-foreground mt-1'>
                      {t('Retry')}: {log.retry_index}
                    </div>
                  ) : null}
                </MobileField>
              </div>
            </div>
          </MobileCard>
        )
      })}
    </div>
  )
}

export function AccountPoolStateLogsMobileList({
  items,
  isLoading = false,
  emptyTitle,
  onFilterRequest,
  onFilterAccount,
}: AccountPoolStateLogsMobileListProps) {
  const { t } = useTranslation()

  if (isLoading) return <AccountPoolLogMobileSkeleton kind='state' />
  if (items.length === 0) {
    return (
      <AccountPoolLogMobileEmpty
        kind='state'
        title={emptyTitle}
        description={t('No audit summary yet')}
      />
    )
  }

  return (
    <div
      data-account-pool-log-mobile-list='state'
      className='bg-card overflow-hidden rounded-lg border md:hidden'
    >
      {items.map((log) => {
        const accountLabel =
          log.pool_account_name || `#${log.pool_account_id || '-'}`

        return (
          <MobileCard key={log.id}>
            <div className='flex min-w-0 flex-col gap-2.5'>
              <div className='flex min-w-0 items-start justify-between gap-3'>
                <div className='min-w-0'>
                  <div className='truncate text-sm font-medium'>
                    {stateLogActionLabel(log.action, t)}
                  </div>
                  <div className='text-muted-foreground mt-0.5 truncate text-xs'>
                    {stateLogSourceLabel(log.source, t)}
                    {log.actor ? ` · ${t('Actor')}: ${log.actor}` : ''}
                  </div>
                </div>
                <StatusBadge
                  label={accountStateText(
                    log.after_status,
                    log.after_schedulable,
                    log.after_unavailable,
                    t
                  )}
                  variant={accountStateVariant(
                    log.after_status,
                    log.after_schedulable,
                    log.after_unavailable
                  )}
                  copyable={false}
                />
              </div>

              <div className='grid grid-cols-2 gap-1.5'>
                <MobileField label={t('Time')}>
                  <div className='font-mono tabular-nums'>
                    {formatTimestamp(log.created_at)}
                  </div>
                </MobileField>
                <MobileField label={t('Request')}>
                  <RequestFilterButton
                    requestId={log.request_id}
                    onFilter={onFilterRequest}
                  />
                </MobileField>
                <MobileField label={t('Account')}>
                  <AccountFilterButton
                    accountId={log.pool_account_id}
                    label={accountLabel}
                    onFilter={onFilterAccount}
                  />
                  <div className='text-muted-foreground mt-1 truncate'>
                    {log.pool_group_name || `#${log.pool_group_id}`} ·{' '}
                    {log.pool_account_auth_type || '-'}
                  </div>
                </MobileField>
                <MobileField label={t('Reason')}>
                  {log.reason || '-'}
                </MobileField>
                <AccountStateSnapshot
                  label={t('Before state')}
                  status={log.before_status}
                  schedulable={log.before_schedulable}
                  unavailable={log.before_unavailable}
                  message={log.before_status_message}
                  disabledReason={log.before_disabled_reason}
                  nextRetryTime={log.before_next_retry_time}
                />
                <AccountStateSnapshot
                  label={t('After state')}
                  status={log.after_status}
                  schedulable={log.after_schedulable}
                  unavailable={log.after_unavailable}
                  message={log.after_status_message}
                  disabledReason={log.after_disabled_reason}
                  nextRetryTime={log.after_next_retry_time}
                />
              </div>
            </div>
          </MobileCard>
        )
      })}
    </div>
  )
}

export function AccountPoolCheckTasksMobileList({
  items,
  isLoading = false,
  emptyTitle,
  onViewTask,
}: AccountPoolCheckTasksMobileListProps) {
  const { t } = useTranslation()

  if (isLoading) return <AccountPoolLogMobileSkeleton kind='check' />
  if (items.length === 0) {
    return (
      <AccountPoolLogMobileEmpty
        kind='check'
        title={emptyTitle}
        description={t('No check tasks found')}
      />
    )
  }

  return (
    <div
      data-account-pool-log-mobile-list='check'
      className='bg-card overflow-hidden rounded-lg border md:hidden'
    >
      {items.map((task) => {
        const progress = checkTaskProgressValue(task)

        return (
          <MobileCard key={task.id}>
            <div className='flex min-w-0 flex-col gap-2.5'>
              <div className='flex min-w-0 items-start justify-between gap-3'>
                <div className='min-w-0'>
                  <div className='truncate text-sm font-medium'>#{task.id}</div>
                  <div className='text-muted-foreground mt-0.5 truncate text-xs'>
                    {task.pool_group_name || `#${task.pool_group_id}`}
                  </div>
                </div>
                <Badge variant={checkTaskBadgeVariant(task.status)}>
                  {checkTaskStatusLabel(task.status, t)}
                </Badge>
              </div>

              <div className='grid grid-cols-2 gap-1.5'>
                <MobileField label={t('Progress')} className='col-span-2'>
                  <div className='flex flex-col gap-1'>
                    <div className='flex items-center justify-between gap-2'>
                      <span>
                        {t('{{checked}}/{{total}} checked', {
                          checked: task.checked + task.skipped,
                          total: task.total,
                        })}
                      </span>
                      <span className='text-muted-foreground'>{progress}%</span>
                    </div>
                    <Progress value={progress} />
                  </div>
                </MobileField>
                <MobileField label={t('Result')}>
                  <div>
                    {t('{{success}} passed', { success: task.success })}
                  </div>
                  <div className='text-muted-foreground mt-1'>
                    {t('{{failed}} failed', { failed: task.failed })}
                    {' · '}
                    {t('{{skipped}} skipped', { skipped: task.skipped })}
                  </div>
                </MobileField>
                <MobileField label={t('Actor')}>
                  {task.actor || '-'}
                </MobileField>
                <MobileField label={t('Created')}>
                  {task.created_time ? formatTimestamp(task.created_time) : '-'}
                </MobileField>
                <MobileField label={t('Finished')}>
                  {task.finished_time
                    ? formatTimestamp(task.finished_time)
                    : '-'}
                </MobileField>
                <MobileField label={t('Request')}>
                  {task.request_id || '-'}
                </MobileField>
                <MobileField label={t('Message')}>
                  {task.message || '-'}
                </MobileField>
              </div>

              <Button
                variant='outline'
                size='sm'
                className='w-full'
                onClick={() => onViewTask(task)}
              >
                {t('View')}
              </Button>
            </div>
          </MobileCard>
        )
      })}
    </div>
  )
}
