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
import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  AlertCircle,
  ArrowUpRight,
  Gauge,
  Loader2,
  Power,
  PowerOff,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { StatusBadge } from '@/components/status-badge'
import { TruncatedText } from '@/components/truncated-text'
import {
  getChannelAccounts,
  updateChannelAccount,
  updateChannelAccountStatus,
} from '../api'
import { CHANNEL_STATUS } from '../constants'
import { useChannelPermissions } from '../hooks/use-channel-permissions'
import {
  channelsQueryKeys,
  formatTimestamp,
  handleTestChannel,
  isUpstreamAccountSyncAccountPoolChannel,
} from '../lib'
import {
  CHANNEL_ACCOUNT_PAGE_SIZE_LIMIT,
  formatUpstreamModelRatioDetails,
  formatUpstreamRatioCompact,
  getChannelAccountAssetDisplaySource,
  getUpstreamKeyRatioDisplayValue,
  getUpstreamKeyGroupLabel,
  getUpstreamRatioDisplayValue,
  loadAllChannelAccounts,
} from '../lib/upstream-sync'
import { NumericSpinnerInput } from './numeric-spinner-input'
import type { Channel, ChannelAccount } from '../types'

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
 * 账号池内联面板承担主表展开后的快速核对职责。
 * 同步平台账号池的测试和启停已经是密钥级操作，所以直接下沉到每把密钥行末尾；
 * 普通手动账号池仍保留完整预览字段，深度维护继续进入账号池管理弹窗。
 */
export function ChannelAccountInlinePanel({
  channel,
  sensitiveVisible,
  className,
  onManage,
}: ChannelAccountInlinePanelProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const permissions = useChannelPermissions()
  const noPermissionMessage = t("You don't have necessary permission")
  const [testingAccountId, setTestingAccountId] = useState<number | null>(null)
  const [togglingAccountId, setTogglingAccountId] = useState<number | null>(
    null
  )
  const [updatingPriorityAccountId, setUpdatingPriorityAccountId] = useState<
    number | null
  >(null)
  const isSyncedAccountPool = isUpstreamAccountSyncAccountPoolChannel(channel)
  const query = useQuery({
    queryKey: [
      ...channelsQueryKeys.detail(channel.id),
      'accounts',
      'inline',
      'all',
      CHANNEL_ACCOUNT_PAGE_SIZE_LIMIT,
    ],
    queryFn: () =>
      loadAllChannelAccounts((page, pageSize) =>
        getChannelAccounts(channel.id, {
          p: page,
          page_size: pageSize,
        })
      ),
  })

  const accounts = query.data?.accounts ?? []
  const total = query.data?.total ?? 0
  const stats = query.data?.stats ?? channel.channel_account_stats
  const nowSeconds =
    query.dataUpdatedAt > 0 ? Math.floor(query.dataUpdatedAt / 1000) : 0
  const maskedText = sensitiveVisible ? undefined : SENSITIVE_MASK

  const refreshRelatedQueries = async () => {
    await Promise.all([
      query.refetch(),
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() }),
      queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.detail(channel.id),
      }),
    ])
  }

  const handleTestAccount = async (account: ChannelAccount) => {
    if (!permissions.canOperate) {
      toast.error(noPermissionMessage)
      return
    }
    setTestingAccountId(account.id)
    try {
      await handleTestChannel(
        channel.id,
        { accountId: account.id },
        undefined,
        queryClient
      )
      await refreshRelatedQueries()
    } finally {
      setTestingAccountId(null)
    }
  }

  const handleToggleAccountStatus = async (account: ChannelAccount) => {
    if (!permissions.canOperateChannelAccount) {
      toast.error(noPermissionMessage)
      return
    }
    const shouldEnable = account.status !== CHANNEL_STATUS.ENABLED
    setTogglingAccountId(account.id)
    try {
      const response = await updateChannelAccountStatus(
        channel.id,
        account.id,
        shouldEnable
          ? {
              status: CHANNEL_STATUS.ENABLED,
              reason: '',
              clear_cooldown: true,
            }
          : {
              status: CHANNEL_STATUS.MANUAL_DISABLED,
              reason: '',
              clear_cooldown: false,
            }
      )
      if (!response.success) {
        throw new Error(response.message || t('Operation failed'))
      }
      toast.success(t('Operation successful'))
      await refreshRelatedQueries()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setTogglingAccountId(null)
    }
  }

  const handleUpdateAccountPriority = async (
    account: ChannelAccount,
    value: number
  ) => {
    if (!permissions.canWriteChannelAccount) {
      toast.error(noPermissionMessage)
      return
    }
    if (value === (account.priority ?? 0)) return

    setUpdatingPriorityAccountId(account.id)
    try {
      const response = await updateChannelAccount(channel.id, account.id, {
        priority: value,
      })
      if (!response.success) {
        throw new Error(response.message || t('Operation failed'))
      }
      toast.success(
        t('{{field}} updated to {{value}}', {
          field: t('Key Priority'),
          value,
        })
      )
      await refreshRelatedQueries()
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Operation failed')
      )
    } finally {
      setUpdatingPriorityAccountId(null)
    }
  }

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
        <div className='max-h-[min(55vh,640px)] overflow-auto overscroll-contain'>
          <Table
            className={cn(
              isSyncedAccountPool ? 'min-w-[980px]' : 'min-w-[1120px]'
            )}
          >
            <TableHeader className='bg-background sticky top-0'>
              <TableRow>
                <TableHead>{t('Name')}</TableHead>
                <TableHead>{t('Key')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Models')}</TableHead>
                {!isSyncedAccountPool && (
                  <TableHead>{t('Key Group')}</TableHead>
                )}
                {!isSyncedAccountPool && (
                  <TableHead>{t('Key Ratio')}</TableHead>
                )}
                <TableHead>{t('Ratio Conversion')}</TableHead>
                <TableHead className='text-center'>
                  {t('Key Priority')}
                </TableHead>
                <TableHead className='text-center'>{t('Key Weight')}</TableHead>
                <TableHead className='text-center'>
                  {t('Upstream Used')}
                </TableHead>
                {!isSyncedAccountPool && (
                  <TableHead>{t('Upstream Remaining')}</TableHead>
                )}
                <TableHead className='text-center'>{t('Last Used')}</TableHead>
                {isSyncedAccountPool && (
                  <TableHead className='text-right'>{t('Actions')}</TableHead>
                )}
              </TableRow>
            </TableHeader>
            <TableBody>
              {accounts.map((account) => {
                const status = getAccountStatus(account, nowSeconds)
                const cooldownUntil = getCooldownUntil(account)
                const assetDisplay =
                  getChannelAccountAssetDisplaySource(account)
                const usedQuota = formatQuotaWithCurrency(
                  assetDisplay.usedQuota,
                  {
                    digitsLarge: 2,
                    digitsSmall: 4,
                    abbreviate: true,
                  }
                )
                const remainingQuota =
                  assetDisplay.remainingQuota == null
                    ? '-'
                    : formatQuotaWithCurrency(assetDisplay.remainingQuota, {
                        digitsLarge: 2,
                        digitsSmall: 4,
                        abbreviate: true,
                      })
                const keyRatioValue =
                  getUpstreamKeyRatioDisplayValue(account)
                const ratioValue = getUpstreamRatioDisplayValue(account)
                const ratioDetails = formatUpstreamModelRatioDetails(
                  account.model_ratios
                )
                const keyRatioTitle = ratioDetails
                  ? `${t('Model Ratios')}:\n${ratioDetails}`
                  : undefined
                const isTestingAccount = testingAccountId === account.id
                const isTogglingAccount = togglingAccountId === account.id
                const accountEnabled = account.status === CHANNEL_STATUS.ENABLED

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
                    {!isSyncedAccountPool && (
                      <TableCell>
                        <span className='text-xs'>
                          {maskedText ??
                            (getUpstreamKeyGroupLabel(account) || '-')}
                        </span>
                      </TableCell>
                    )}
                    {!isSyncedAccountPool && (
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
                    )}
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
                    <TableCell className='text-center'>
                      {isSyncedAccountPool ? (
                        <div
                          className='flex min-w-[104px] justify-center'
                          title={
                            permissions.canWriteChannelAccount
                              ? undefined
                              : noPermissionMessage
                          }
                        >
                          <NumericSpinnerInput
                            value={account.priority ?? 0}
                            min={-999}
                            disabled={
                              !permissions.canWriteChannelAccount ||
                              updatingPriorityAccountId === account.id ||
                              testingAccountId !== null ||
                              togglingAccountId !== null
                            }
                            onChange={(value) => {
                              void handleUpdateAccountPriority(account, value)
                            }}
                          />
                        </div>
                      ) : (
                        <span className='font-mono text-xs tabular-nums'>
                          {account.priority}
                        </span>
                      )}
                    </TableCell>
                    <TableCell className='text-center'>
                      <span
                        className='font-mono text-xs tabular-nums'
                        title={
                          isSyncedAccountPool
                            ? t('Sync-managed weight')
                            : undefined
                        }
                      >
                        {account.weight}
                      </span>
                    </TableCell>
                    <TableCell className='text-center'>
                      <span className='font-mono text-xs tabular-nums'>
                        {sensitiveVisible ? usedQuota : SENSITIVE_MASK}
                      </span>
                    </TableCell>
                    {!isSyncedAccountPool && (
                      <TableCell>
                        <span className='font-mono text-xs tabular-nums'>
                          {sensitiveVisible
                            ? remainingQuota
                            : remainingQuota === '-'
                              ? '-'
                              : SENSITIVE_MASK}
                        </span>
                      </TableCell>
                    )}
                    <TableCell className='text-center'>
                      <span className='text-muted-foreground text-xs'>
                        {account.last_used_time > 0
                          ? formatTimestamp(account.last_used_time)
                          : '-'}
                      </span>
                    </TableCell>
                    {isSyncedAccountPool && (
                      <TableCell>
                        <div className='flex justify-end gap-1'>
                          <Tooltip>
                            <TooltipTrigger
                              render={
                                <Button
                                  type='button'
                                  variant='ghost'
                                  size='icon-sm'
                                  onClick={(event) => {
                                    event.stopPropagation()
                                    void handleTestAccount(account)
                                  }}
                                  disabled={
                                    !permissions.canOperate ||
                                    updatingPriorityAccountId !== null ||
                                    testingAccountId !== null ||
                                    togglingAccountId !== null
                                  }
                                  aria-label={t('Test Connection')}
                                />
                              }
                            >
                              {isTestingAccount ? (
                                <Loader2 className='animate-spin' />
                              ) : (
                                <Gauge />
                              )}
                            </TooltipTrigger>
                            <TooltipContent>
                              {permissions.canOperate
                                ? t('Test Connection')
                                : noPermissionMessage}
                            </TooltipContent>
                          </Tooltip>
                          <Tooltip>
                            <TooltipTrigger
                              render={
                                <Button
                                  type='button'
                                  variant='ghost'
                                  size='icon-sm'
                                  onClick={(event) => {
                                    event.stopPropagation()
                                    void handleToggleAccountStatus(account)
                                  }}
                                  disabled={
                                    !permissions.canOperateChannelAccount ||
                                    updatingPriorityAccountId !== null ||
                                    testingAccountId !== null ||
                                    togglingAccountId !== null
                                  }
                                  aria-label={
                                    accountEnabled ? t('Disable') : t('Enable')
                                  }
                                  className={cn(
                                    accountEnabled &&
                                      'text-destructive hover:text-destructive'
                                  )}
                                />
                              }
                            >
                              {isTogglingAccount ? (
                                <Loader2 className='animate-spin' />
                              ) : accountEnabled ? (
                                <PowerOff />
                              ) : (
                                <Power />
                              )}
                            </TooltipTrigger>
                            <TooltipContent>
                              {permissions.canOperateChannelAccount
                                ? accountEnabled
                                  ? t('Disable')
                                  : t('Enable')
                                : noPermissionMessage}
                            </TooltipContent>
                          </Tooltip>
                        </div>
                      </TableCell>
                    )}
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
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
