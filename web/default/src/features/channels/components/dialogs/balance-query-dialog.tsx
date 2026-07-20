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
import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Loader2, RefreshCw, DollarSign } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { formatTimestampToDate } from '@/lib/format'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { getCodexUsage, updateChannelBalance } from '../../api'
import { useChannelPermissions } from '../../hooks/use-channel-permissions'
import { channelsQueryKeys, patchChannelBalanceCache } from '../../lib'
import { useChannels } from '../channels-provider'
import {
  CodexUsageDialog,
  type CodexUsageDialogData,
} from './codex-usage-dialog'

type BalanceQueryDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function BalanceQueryDialog({
  open,
  onOpenChange,
}: BalanceQueryDialogProps) {
  const { t } = useTranslation()
  const { currentRow, setCurrentRow } = useChannels()
  const queryClient = useQueryClient()
  const [isQuerying, setIsQuerying] = useState(false)
  const [balance, setBalance] = useState<number | null>(null)
  const [balanceUpdatedTime, setBalanceUpdatedTime] = useState<number | null>(
    null
  )
  const [codexUsageResponse, setCodexUsageResponse] =
    useState<CodexUsageDialogData | null>(null)
  const permissions = useChannelPermissions()
  const noPermissionMessage = t("You don't have necessary permission")

  const isCodex = currentRow?.type === 57
  const isUpstreamAccountSync = (() => {
    if (!currentRow?.settings?.trim()) return false
    try {
      const parsed = JSON.parse(currentRow.settings) as Record<string, unknown>
      const metadata = parsed.upstream_account_sync
      return Boolean(
        metadata &&
        (typeof metadata === 'object' ||
          metadata === true ||
          (typeof metadata === 'string' && metadata.trim().length > 0))
      )
    } catch {
      return false
    }
  })()

  const handleQueryCodexUsage = async () => {
    const row = currentRow
    if (!row) return
    if (!permissions.canOperate) {
      toast.error(noPermissionMessage)
      return
    }
    setIsQuerying(true)
    try {
      const res = await getCodexUsage(row.id)
      if (!res.success) {
        throw new Error(res.message || t('Failed to fetch usage'))
      }
      setCodexUsageResponse(res)
    } catch (error: unknown) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to fetch usage')
      )
    } finally {
      setIsQuerying(false)
    }
  }

  useEffect(() => {
    if (!isCodex) return
    if (!open) return
    handleQueryCodexUsage()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, isCodex])

  if (!currentRow) return null

  const handleQueryBalance = async () => {
    if (!permissions.canOperate) {
      toast.error(noPermissionMessage)
      return
    }
    setIsQuerying(true)
    try {
      const response = await updateChannelBalance(currentRow.id)
      if (response.success && response.balance !== undefined) {
        const newBalance = response.balance
        const updatedTime =
          response.balance_updated_time ?? Math.floor(Date.now() / 1000)

        setBalance(newBalance)
        setBalanceUpdatedTime(updatedTime)
        toast.success(t('Balance updated successfully'))

        // 同步渠道余额刷新会同时返回账号已使用量，弹窗和列表都要立即更新。
        setCurrentRow({
          ...currentRow,
          balance: newBalance,
          used_quota: response.used_quota ?? currentRow.used_quota,
          balance_updated_time: updatedTime,
        })
        patchChannelBalanceCache(queryClient, currentRow.id, response)

        // 后台重新拉取列表，确保缓存补丁之外的其他字段也保持最新。
        await queryClient.invalidateQueries({
          queryKey: channelsQueryKeys.lists(),
        })
      } else {
        toast.error(response.message || t('Failed to query balance'))
      }
    } catch (error: unknown) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to query balance')
      )
    } finally {
      setIsQuerying(false)
    }
  }

  const handleClose = () => {
    setBalance(null)
    setBalanceUpdatedTime(null)
    setCodexUsageResponse(null)
    onOpenChange(false)
  }

  const formatBalance = (bal: number) =>
    formatCurrencyFromUSD(bal, {
      digitsLarge: 2,
      digitsSmall: 4,
      abbreviate: false,
    })

  const formatDate = (timestamp: number) => {
    if (!timestamp) return 'Never'
    return formatTimestampToDate(timestamp)
  }

  if (isCodex) {
    return (
      <CodexUsageDialog
        open={open}
        onOpenChange={(v) => {
          if (!v) handleClose()
        }}
        channelName={currentRow.name}
        channelId={currentRow.id}
        response={codexUsageResponse}
        onRefresh={handleQueryCodexUsage}
        isRefreshing={isQuerying}
      />
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Query Balance')}</DialogTitle>
          <DialogDescription>
            {isUpstreamAccountSync
              ? t(
                  'Refreshes the real upstream account balance using the saved encrypted upstream account credential.'
                )
              : t('Update balance for:')}{' '}
            <strong>{currentRow.name}</strong>
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4 py-4'>
          {/* Current Balance Display */}
          <div className='bg-muted/50 rounded-lg border p-4'>
            <div className='text-muted-foreground mb-2 flex items-center gap-2 text-sm'>
              <DollarSign className='h-4 w-4' />
              <span>{t('Current Balance')}</span>
            </div>
            <div className='text-2xl font-bold'>
              {balance !== null
                ? formatBalance(balance)
                : formatBalance(currentRow.balance)}
            </div>
            <div className='text-muted-foreground mt-2 text-xs'>
              {t('Last updated:')}{' '}
              {formatDate(
                balanceUpdatedTime ?? currentRow.balance_updated_time
              )}
            </div>
          </div>

          {/* Balance Update Button */}
          <Button
            className='w-full'
            onClick={handleQueryBalance}
            disabled={isQuerying || !permissions.canOperate}
            title={permissions.canOperate ? undefined : noPermissionMessage}
          >
            {isQuerying && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {!isQuerying && <RefreshCw className='mr-2 h-4 w-4' />}
            {isQuerying ? t('Querying...') : t('Update Balance')}
          </Button>
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={handleClose} disabled={isQuerying}>
            {t('Close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
