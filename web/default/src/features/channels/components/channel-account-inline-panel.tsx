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
import { useQuery } from '@tanstack/react-query'
import { AlertCircle, ArrowUpRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { StatusBadge } from '@/components/status-badge'
import { TruncatedText } from '@/components/truncated-text'
import { getChannelAccounts } from '../api'
import { CHANNEL_STATUS } from '../constants'
import { channelsQueryKeys, formatTimestamp } from '../lib'
import {
  formatUpstreamModelRatioDetails,
  formatUpstreamRatioCompact,
  getUpstreamKeyRatioDisplayValue,
  getUpstreamKeyGroupLabel,
  getUpstreamRatioDisplayValue,
} from '../lib/upstream-sync'
import type { Channel, ChannelAccount } from '../types'

const INLINE_ACCOUNT_PAGE_SIZE = 50
const SENSITIVE_MASK = '••••'

type ChannelAccountInlinePanelProps = {
  channel: Channel
  sensitiveVisible: boolean
  className?: string
  onManage: () => void
}

function isCoolingDown(account: ChannelAccount, nowSeconds: number): boolean {
  return (
    account.rate_limited_until > nowSeconds ||
    account.overload_until > nowSeconds ||
    account.temp_disabled_until > nowSeconds
  )
}

function getCooldownUntil(account: ChannelAccount): number {
  return Math.max(
    account.rate_limited_until,
    account.overload_until,
    account.temp_disabled_until
  )
}

function getAccountStatus(
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

function formatAccountModels(models: string): string {
  const items = models
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
  if (items.length === 0) return '-'
  if (items.length <= 2) return items.join(', ')
  return `${items.slice(0, 2).join(', ')} +${items.length - 2}`
}

/**
 * 账号池内联面板只承担快速核对职责，逐 key 的写操作统一进入现有账号池管理弹窗。
 * 这样可以复用已经过权限校验、缓存刷新和能力重建验证的保存路径。
 */
export function ChannelAccountInlinePanel({
  channel,
  sensitiveVisible,
  className,
  onManage,
}: ChannelAccountInlinePanelProps) {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: [
      ...channelsQueryKeys.detail(channel.id),
      'accounts',
      'inline',
      INLINE_ACCOUNT_PAGE_SIZE,
    ],
    queryFn: () =>
      getChannelAccounts(channel.id, {
        p: 1,
        page_size: INLINE_ACCOUNT_PAGE_SIZE,
      }),
  })

  const accounts = query.data?.data?.accounts.items ?? []
  const total = query.data?.data?.accounts.total ?? 0
  const stats = query.data?.data?.stats ?? channel.channel_account_stats
  const nowSeconds =
    query.dataUpdatedAt > 0 ? Math.floor(query.dataUpdatedAt / 1000) : 0
  const maskedText = sensitiveVisible ? undefined : SENSITIVE_MASK

  return (
    <div
      className={cn(
        'bg-muted/20 flex flex-col gap-3 rounded-md border p-3',
        className
      )}
    >
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex flex-wrap items-center gap-1.5'>
          <span className='text-sm font-medium'>
            {t('Synced Key Configuration')}
          </span>
          <StatusBadge
            label={`${t('Total')}: ${stats?.total ?? total}`}
            variant='neutral'
            size='sm'
            copyable={false}
          />
          <StatusBadge
            label={`${t('Enabled')}: ${stats?.enabled ?? 0}`}
            variant='success'
            size='sm'
            copyable={false}
          />
          {(stats?.cooldown ?? 0) > 0 && (
            <StatusBadge
              label={`${t('Cooling Down')}: ${stats?.cooldown ?? 0}`}
              variant='warning'
              size='sm'
              copyable={false}
            />
          )}
        </div>
        <Button size='sm' variant='outline' onClick={onManage}>
          <ArrowUpRight data-icon='inline-start' />
          {t('Account Pool Management')}
        </Button>
      </div>

      {query.isLoading ? (
        <div className='grid gap-2 sm:grid-cols-2 lg:grid-cols-4'>
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className='h-16 w-full' />
          ))}
        </div>
      ) : query.isError ? (
        <div className='text-destructive flex items-center gap-2 text-sm'>
          <AlertCircle className='size-4' />
          {t('Failed to load channel accounts')}
        </div>
      ) : accounts.length === 0 ? (
        <div className='text-muted-foreground text-sm'>
          {t('No channel accounts found')}
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Name')}</TableHead>
              <TableHead>{t('Key')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Models')}</TableHead>
              <TableHead>{t('Key Group')}</TableHead>
              <TableHead>{t('Key Ratio')}</TableHead>
              <TableHead>{t('Ratio Conversion')}</TableHead>
              <TableHead>{t('Priority')}</TableHead>
              <TableHead>{t('Weight')}</TableHead>
              <TableHead>{t('Used')}</TableHead>
              <TableHead>{t('Last Used')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {accounts.map((account) => {
              const status = getAccountStatus(account, nowSeconds)
              const cooldownUntil = getCooldownUntil(account)
              const usedQuota = formatQuotaWithCurrency(account.used_quota, {
                digitsLarge: 2,
                digitsSmall: 4,
                abbreviate: true,
              })
              const keyRatioValue = getUpstreamKeyRatioDisplayValue(account)
              const ratioValue = getUpstreamRatioDisplayValue(account)
              const ratioDetails = formatUpstreamModelRatioDetails(
                account.model_ratios
              )
              const keyRatioTitle = ratioDetails
                ? `${t('Model Ratios')}:\n${ratioDetails}`
                : undefined

              return (
                <TableRow key={account.id}>
                  <TableCell>
                    <div className='flex min-w-[120px] flex-col gap-0.5'>
                      <TruncatedText
                        text={
                          sensitiveVisible
                            ? account.name || `#${account.id}`
                            : SENSITIVE_MASK
                        }
                        maxWidth='max-w-[180px]'
                        className='font-medium'
                      />
                      <span className='text-muted-foreground font-mono text-[11px]'>
                        #{account.id}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <span className='font-mono text-xs'>
                      {sensitiveVisible ? account.key || '-' : SENSITIVE_MASK}
                    </span>
                  </TableCell>
                  <TableCell>
                    <div className='flex flex-col gap-1'>
                      <StatusBadge
                        label={t(status.label)}
                        variant={status.variant}
                        size='sm'
                        copyable={false}
                      />
                      {cooldownUntil > nowSeconds && (
                        <span className='text-muted-foreground text-[11px]'>
                          {formatTimestamp(cooldownUntil)}
                        </span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>
                    <TruncatedText
                      text={
                        sensitiveVisible
                          ? formatAccountModels(account.models)
                          : SENSITIVE_MASK
                      }
                      maxWidth='max-w-[220px]'
                      className='text-xs'
                    />
                  </TableCell>
                  <TableCell>
                    <span className='text-xs'>
                      {maskedText ?? (getUpstreamKeyGroupLabel(account) || '-')}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span
                      className='font-mono text-xs'
                      title={keyRatioTitle}
                    >
                      {maskedText ??
                        (keyRatioValue != null
                          ? `${formatUpstreamRatioCompact(keyRatioValue)}x`
                          : '-')}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span
                      className='font-mono text-xs'
                      title={ratioDetails || undefined}
                    >
                      {maskedText ??
                        (ratioValue != null
                          ? `${formatUpstreamRatioCompact(ratioValue)}x`
                          : '-')}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className='font-mono text-xs tabular-nums'>
                      {account.priority}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className='font-mono text-xs tabular-nums'>
                      {account.weight}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className='font-mono text-xs tabular-nums'>
                      {sensitiveVisible ? usedQuota : SENSITIVE_MASK}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className='text-muted-foreground text-xs'>
                      {account.last_used_time > 0
                        ? formatTimestamp(account.last_used_time)
                        : '-'}
                    </span>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}

      {total > accounts.length && (
        <div className='text-muted-foreground text-xs'>
          {t('Showing {{shown}} of {{total}} channel accounts', {
            shown: accounts.length,
            total,
          })}
        </div>
      )}
    </div>
  )
}
