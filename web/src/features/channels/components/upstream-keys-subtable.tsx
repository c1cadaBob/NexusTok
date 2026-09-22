/*
Copyright (C) 2023-2026 c1cadaBob

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

For commercial licensing, please contact support@c1cadabob.dev
*/
import { useQueryClient } from '@tanstack/react-query'
import {
  DollarSign,
  Download,
  Gauge,
  Loader2,
  MoreHorizontal,
  Pencil,
  Power,
  PowerOff,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  BadgeListCell,
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { MultiSelect } from '@/components/multi-select'
import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import { TruncatedText } from '@/components/truncated-text'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import { batchUpdateUpstreamKeyStatus, patchUpstreamKey } from '../api'
import {
  CHANNEL_ACTIONS_COLUMN_CLASS_NAME,
  CHANNEL_STATUS_CONFIG,
} from '../constants'
import {
  channelsQueryKeys,
  createChannelFieldUpdateScheduler,
  formatConversionRatio,
  formatRelativeTime,
  sortUpstreamKeysForDisplay,
} from '../lib'
import type { Channel, UpstreamKey } from '../types'
import { useChannels } from './channels-provider'
import { NumericSpinnerInput } from './numeric-spinner-input'

const SENSITIVE_MASK = '••••'

type UpstreamKeysSubTableProps = {
  channel: Channel
}

type UpstreamKeyEditDialogProps = {
  channel: Channel
  upstreamKey: UpstreamKey | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

type UpstreamKeyChannelDialog =
  | 'test-channel'
  | 'balance-query'
  | 'fetch-models'

function getUpstreamKeyStatusConfig(status: number): {
  label: string
  variant: StatusBadgeProps['variant']
} {
  if (status === 4) {
    return { label: 'Missing', variant: 'warning' }
  }

  const config =
    CHANNEL_STATUS_CONFIG[status as keyof typeof CHANNEL_STATUS_CONFIG] ||
    CHANNEL_STATUS_CONFIG[0]
  return { label: config.label, variant: config.variant }
}

function getUpstreamKeyHealthStatusConfig(upstreamKey: UpstreamKey): {
  label: string
  variant: StatusBadgeProps['variant']
} {
  switch (upstreamKey.health_status) {
    case 'disabled':
      return { label: 'Disabled', variant: 'neutral' }
    case 'enabled':
      return { label: 'Enabled', variant: 'info' }
    case 'normal':
      return { label: 'Normal', variant: 'success' }
    case 'degraded':
      return { label: 'Degraded', variant: 'warning' }
    case 'invalid':
      return { label: 'Invalid', variant: 'danger' }
    default:
      return getUpstreamKeyStatusConfig(upstreamKey.status)
  }
}

function getUpstreamKeyHealthReasonLabel(reason?: string): string | null {
  switch (reason) {
    case 'credential_unavailable':
      return 'Credential unavailable'
    case 'models_unavailable':
      return 'Model capability unavailable'
    case 'snapshot_only':
      return 'Historical snapshot'
    case 'missing':
      return 'Missing'
    case 'expired':
      return 'Expired'
    case 'quota_exhausted':
      return 'Quota exhausted'
    case 'upstream_disabled':
      return 'Upstream disabled'
    case 'manual_disabled':
      return 'Manually disabled'
    case 'site_sync_unavailable':
      return 'Platform snapshot unavailable'
    case 'not_enough_samples':
      return 'Not enough recent samples'
    case 'stale_samples':
      return 'No recent key usage'
    case 'no_health_record':
      return 'No health samples yet'
    case 'recent_failures':
      return 'Recent failures exceeded threshold'
    case 'recent_degraded':
      return 'Recent success rate degraded'
    case 'high_first_latency':
      return 'First response latency exceeded threshold'
    default:
      return null
  }
}

function getKeyDisplayName(upstreamKey: UpstreamKey): string {
  return upstreamKey.name || upstreamKey.external_id || `#${upstreamKey.id}`
}

function hasConversionRatioOverride(upstreamKey: UpstreamKey): boolean {
  return (
    upstreamKey.conversion_ratio_override !== null &&
    upstreamKey.conversion_ratio_override !== undefined
  )
}

function isFreeUpstreamKey(upstreamKey: UpstreamKey): boolean {
  return upstreamKey.conversion_ratio === 0
}

function getEffectiveWeight(upstreamKey: UpstreamKey): number {
  if (upstreamKey.conversion_ratio === 0) {
    return 2000
  }
  return upstreamKey.weight
}

function LastSyncCell({ timestamp }: { timestamp: number }) {
  const { t, i18n } = useTranslation()
  if (!timestamp) {
    return <span className='text-muted-foreground text-xs'>-</span>
  }

  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const relative = formatRelativeTime(timestamp, locale)
  const fullDate = formatTimestampToDate(timestamp)

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger
          render={
            <StatusBadge
              label={relative}
              variant='neutral'
              size='sm'
              copyable={false}
              className='cursor-help'
            />
          }
        />
        <TooltipContent side='top'>
          <span className='font-mono text-sm'>{fullDate || t('Never')}</span>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

function UpstreamKeyStatusBadge({ upstreamKey }: { upstreamKey: UpstreamKey }) {
  const { t } = useTranslation()
  const config = getUpstreamKeyHealthStatusConfig(upstreamKey)
  const reasonLabel = getUpstreamKeyHealthReasonLabel(
    upstreamKey.health_reason || upstreamKey.availability_reason
  )

  return (
    <div className='flex flex-wrap items-center justify-center gap-1'>
      <TooltipProvider delay={100}>
        <Tooltip>
          <TooltipTrigger render={<span />}>
            <StatusBadge
              label={t(config.label)}
              variant={config.variant}
              size='sm'
              copyable={false}
            />
          </TooltipTrigger>
          {reasonLabel && (
            <TooltipContent side='top'>{t(reasonLabel)}</TooltipContent>
          )}
        </Tooltip>
      </TooltipProvider>
      {upstreamKey.snapshot_only && (
        <StatusBadge
          label={t('Historical snapshot')}
          variant='warning'
          size='sm'
          copyable={false}
        />
      )}
    </div>
  )
}

function UpstreamKeyWeightCell({ upstreamKey }: { upstreamKey: UpstreamKey }) {
  const { t } = useTranslation()
  const hasOverride =
    upstreamKey.weight_override !== null &&
    upstreamKey.weight_override !== undefined
  const weight = getEffectiveWeight(upstreamKey)

  return (
    <div className='flex items-center justify-center gap-1'>
      <span className='font-mono text-sm tabular-nums'>{weight}</span>
      {hasOverride && (
        <StatusBadge
          label={t('Override')}
          variant='blue'
          size='sm'
          copyable={false}
        />
      )}
    </div>
  )
}

function UpstreamKeyRatioCell({ upstreamKey }: { upstreamKey: UpstreamKey }) {
  const { t } = useTranslation()

  return (
    <div className='flex items-center justify-center gap-1'>
      <span className='font-mono text-sm tabular-nums'>
        {formatConversionRatio(upstreamKey.conversion_ratio)}
      </span>
      {hasConversionRatioOverride(upstreamKey) && (
        <StatusBadge
          label={t('Override')}
          variant='blue'
          size='sm'
          copyable={false}
        />
      )}
      {isFreeUpstreamKey(upstreamKey) && (
        <StatusBadge
          label={t('Free')}
          variant='success'
          size='sm'
          copyable={false}
        />
      )}
    </div>
  )
}

function UpstreamKeyPriorityCell({
  channelId,
  upstreamKey,
}: {
  channelId: number
  upstreamKey: UpstreamKey
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const serverValue = upstreamKey.key_priority ?? 0
  const [localValue, setLocalValue] = useState(serverValue)

  useEffect(() => {
    setLocalValue(serverValue)
  }, [serverValue, upstreamKey.id])

  const scheduler = useMemo(
    () =>
      createChannelFieldUpdateScheduler((nextValue) => {
        void (async () => {
          try {
            const response = await patchUpstreamKey(channelId, upstreamKey.id, {
              key_priority: nextValue,
            })
            if (!response.success) {
              throw createServerError(response, t('Operation failed'))
            }
            await Promise.all([
              queryClient.invalidateQueries({
                queryKey: ['upstream-keys', channelId],
              }),
              queryClient.invalidateQueries({
                queryKey: channelsQueryKeys.lists(),
              }),
            ])
          } catch (error: unknown) {
            setLocalValue(serverValue)
            handleServerError(error, t('Operation failed'))
          }
        })()
      }),
    [channelId, queryClient, serverValue, t, upstreamKey.id]
  )

  useEffect(() => () => scheduler.flush(), [scheduler])

  return (
    <NumericSpinnerInput
      value={localValue}
      min={0}
      max={99}
      onChange={(nextValue) => {
        setLocalValue(nextValue)
        scheduler.schedule(nextValue)
      }}
      onCommit={scheduler.flush}
    />
  )
}

function UpstreamKeyModelsCell({ upstreamKey }: { upstreamKey: UpstreamKey }) {
  const { t } = useTranslation()

  return (
    <div className='min-w-0 space-y-1'>
      <BadgeListCell
        items={upstreamKey.models.map((model) => (
          <StatusBadge
            key={model}
            label={model}
            autoColor={model}
            size='sm'
            className='font-mono'
          />
        ))}
      />
      {!upstreamKey.models_synced && (
        <StatusBadge
          label={t('Unavailable')}
          variant='warning'
          size='sm'
          copyable={false}
        />
      )}
    </div>
  )
}

function UpstreamKeyActions({
  channel,
  upstreamKey,
  onEdit,
}: {
  channel: Channel
  upstreamKey: UpstreamKey
  onEdit: (upstreamKey: UpstreamKey) => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { setCurrentRow, setCurrentUpstreamKey, setOpen } = useChannels()
  const [isToggling, setIsToggling] = useState(false)
  const isEnabled = upstreamKey.status === 1
  let statusIcon = <Power />
  if (isToggling) {
    statusIcon = <Loader2 className='animate-spin' />
  } else if (isEnabled) {
    statusIcon = <PowerOff />
  }

  const openChannelDialog = useCallback(
    (dialog: UpstreamKeyChannelDialog) => {
      setCurrentRow(channel)
      setCurrentUpstreamKey(upstreamKey)
      setOpen(dialog)
    },
    [channel, setCurrentRow, setCurrentUpstreamKey, setOpen, upstreamKey]
  )

  const handleToggleStatus = useCallback(async () => {
    setIsToggling(true)
    try {
      const response = await batchUpdateUpstreamKeyStatus(
        channel.id,
        [upstreamKey.id],
        isEnabled ? 2 : 1
      )
      if (!response.success) {
        throw createServerError(response, t('Operation failed'))
      }
      toast.success(t('Operation successful'))
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['upstream-keys', channel.id],
        }),
        queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() }),
      ])
    } catch (error: unknown) {
      handleServerError(error, t('Operation failed'))
    } finally {
      setIsToggling(false)
    }
  }, [channel.id, isEnabled, queryClient, t, upstreamKey.id])

  const handleOpenTestDialog = useCallback(
    (event?: React.MouseEvent<HTMLElement>) => {
      event?.stopPropagation()
      openChannelDialog('test-channel')
    },
    [openChannelDialog]
  )

  const handleOpenBalanceDialog = useCallback(
    (event?: React.MouseEvent<HTMLElement>) => {
      event?.stopPropagation()
      openChannelDialog('balance-query')
    },
    [openChannelDialog]
  )

  const handleOpenFetchModelsDialog = useCallback(
    (event?: React.MouseEvent<HTMLElement>) => {
      event?.stopPropagation()
      openChannelDialog('fetch-models')
    },
    [openChannelDialog]
  )

  const handleActionAreaEvent = useCallback((event: React.SyntheticEvent) => {
    event.stopPropagation()
  }, [])

  const handleEdit = useCallback(() => {
    onEdit(upstreamKey)
  }, [onEdit, upstreamKey])

  const handleToggleStatusClick = useCallback(
    (event: React.MouseEvent<HTMLButtonElement>) => {
      event.stopPropagation()
      void handleToggleStatus()
    },
    [handleToggleStatus]
  )

  return (
    <div
      className='flex items-center justify-center gap-1'
      onClick={handleActionAreaEvent}
      onPointerDown={handleActionAreaEvent}
    >
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleOpenTestDialog}
              aria-label={t('Test Connection')}
            />
          }
        >
          <Gauge />
        </TooltipTrigger>
        <TooltipContent>{t('Test Connection')}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleToggleStatusClick}
              disabled={isToggling}
              aria-label={isEnabled ? t('Disable') : t('Enable')}
              className={cn(
                isEnabled
                  ? 'text-destructive hover:text-destructive'
                  : 'text-success hover:text-success'
              )}
            />
          }
        >
          {statusIcon}
        </TooltipTrigger>
        <TooltipContent>
          {isEnabled ? t('Disable') : t('Enable')}
        </TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleEdit}
              aria-label={t('Edit')}
            />
          }
        >
          <Pencil />
        </TooltipTrigger>
        <TooltipContent>{t('Edit')}</TooltipContent>
      </Tooltip>

      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              aria-label={t('Open menu')}
            />
          }
        >
          <MoreHorizontal />
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' className='w-48'>
          <DropdownMenuGroup>
            <DropdownMenuItem onClick={handleOpenTestDialog}>
              {t('Test Connection')}
              <DropdownMenuShortcut>
                <Gauge size={16} />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleOpenBalanceDialog}>
              {t('Query Balance')}
              <DropdownMenuShortcut>
                <DollarSign size={16} />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handleOpenFetchModelsDialog}>
              {t('Fetch Models')}
              <DropdownMenuShortcut>
                <Download size={16} />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

function UpstreamKeyMobileField({
  label,
  children,
  className,
}: {
  label: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn('min-w-0 space-y-0.5', className)}>
      <div className='text-muted-foreground text-[11px]'>{label}</div>
      <div className='min-w-0 text-sm'>{children}</div>
    </div>
  )
}

export function UpstreamKeysMobileList(props: UpstreamKeysSubTableProps) {
  const { t } = useTranslation()
  const { sensitiveVisible } = useChannels()
  const [editingKey, setEditingKey] = useState<UpstreamKey | null>(null)
  const keys = sortUpstreamKeysForDisplay(props.channel.upstream_keys || [])

  return (
    <div className='border-border mt-3 border-t pt-3'>
      <div className='text-muted-foreground mb-2 text-xs font-medium'>
        {t('Upstream keys')} ({keys.length})
      </div>
      {keys.length === 0 ? (
        <div className='text-muted-foreground py-3 text-center text-sm'>
          {t('No upstream keys')}
        </div>
      ) : (
        <div className='relative max-h-[70vh] w-full overflow-auto overscroll-contain'>
          <div className='divide-y rounded-md border'>
            {keys.map((upstreamKey) => (
              <div key={upstreamKey.id} className='space-y-3 p-3'>
                <div className='flex min-w-0 items-start justify-between gap-2'>
                  <div className='flex min-w-0 items-start gap-2'>
                    <TruncatedText
                      text={
                        sensitiveVisible
                          ? getKeyDisplayName(upstreamKey)
                          : SENSITIVE_MASK
                      }
                      maxWidth='max-w-[calc(100%-5rem)]'
                      className='font-medium'
                    />
                  </div>
                  <UpstreamKeyStatusBadge upstreamKey={upstreamKey} />
                </div>

                <div className='grid min-w-0 grid-cols-2 gap-x-3 gap-y-3'>
                  <UpstreamKeyMobileField label={t('Key ID')}>
                    <StatusBadge
                      label={`#${upstreamKey.key_id || upstreamKey.id}`}
                      copyText={String(upstreamKey.key_id || upstreamKey.id)}
                      size='sm'
                      showDot={false}
                      className='font-mono'
                    />
                  </UpstreamKeyMobileField>
                  <UpstreamKeyMobileField label={t('Key')}>
                    <span
                      className='block max-w-full truncate font-mono text-xs'
                      title={
                        sensitiveVisible
                          ? upstreamKey.key_preview || undefined
                          : undefined
                      }
                    >
                      {sensitiveVisible
                        ? upstreamKey.key_preview || '-'
                        : SENSITIVE_MASK}
                    </span>
                  </UpstreamKeyMobileField>
                  <UpstreamKeyMobileField label={t('Models')}>
                    <UpstreamKeyModelsCell upstreamKey={upstreamKey} />
                  </UpstreamKeyMobileField>
                  <UpstreamKeyMobileField label={t('Converted ratio')}>
                    <UpstreamKeyRatioCell upstreamKey={upstreamKey} />
                  </UpstreamKeyMobileField>
                  <UpstreamKeyMobileField label={t('Key priority')}>
                    <UpstreamKeyPriorityCell
                      channelId={props.channel.id}
                      upstreamKey={upstreamKey}
                    />
                  </UpstreamKeyMobileField>
                  <UpstreamKeyMobileField label={t('Key weight')}>
                    <UpstreamKeyWeightCell upstreamKey={upstreamKey} />
                  </UpstreamKeyMobileField>
                  <UpstreamKeyMobileField label={t('Last sync')}>
                    <LastSyncCell timestamp={upstreamKey.last_sync_at} />
                  </UpstreamKeyMobileField>
                </div>

                <div className='flex justify-end'>
                  <UpstreamKeyActions
                    channel={props.channel}
                    upstreamKey={upstreamKey}
                    onEdit={setEditingKey}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
      <UpstreamKeyEditDialog
        channel={props.channel}
        upstreamKey={editingKey}
        open={editingKey !== null}
        onOpenChange={(open) => {
          if (!open) {
            setEditingKey(null)
          }
        }}
      />
    </div>
  )
}

function UpstreamKeyEditDialog(props: UpstreamKeyEditDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyPriority, setKeyPriority] = useState(0)
  const [ratioOverrideEnabled, setRatioOverrideEnabled] = useState(false)
  const [conversionRatio, setConversionRatio] = useState('')
  const [weightOverride, setWeightOverride] = useState('')
  const [allowedModels, setAllowedModels] = useState<string[]>([])
  const [allowedModelsLimited, setAllowedModelsLimited] = useState(false)
  const [isSaving, setIsSaving] = useState(false)

  useEffect(() => {
    if (!props.upstreamKey || !props.open) {
      return
    }

    setKeyPriority(props.upstreamKey.key_priority)
    setRatioOverrideEnabled(hasConversionRatioOverride(props.upstreamKey))
    setConversionRatio(
      String(
        props.upstreamKey.conversion_ratio_override ??
          props.upstreamKey.conversion_ratio
      )
    )
    setWeightOverride(
      props.upstreamKey.weight_override === null ||
        props.upstreamKey.weight_override === undefined
        ? ''
        : String(props.upstreamKey.weight_override)
    )
    setAllowedModels(
      props.upstreamKey.allowed_models ?? props.upstreamKey.models
    )
    setAllowedModelsLimited(props.upstreamKey.allowed_models != null)
  }, [props.open, props.upstreamKey])

  const handleSave = async () => {
    const upstreamKey = props.upstreamKey
    if (!upstreamKey) {
      return
    }

    const payload: Parameters<typeof patchUpstreamKey>[2] = {
      key_priority: keyPriority,
    }

    if (allowedModelsLimited) {
      payload.allowed_models = allowedModels
    } else {
      payload.clear_allowed_models = true
    }

    let parsedRatio: number | undefined
    if (ratioOverrideEnabled) {
      if (conversionRatio.trim() === '') {
        toast.error(t('Conversion ratio must be a finite non-negative number'))
        return
      }
      parsedRatio = Number(conversionRatio)
      if (!Number.isFinite(parsedRatio) || parsedRatio < 0) {
        toast.error(t('Conversion ratio must be a finite non-negative number'))
        return
      }
      payload.conversion_ratio = parsedRatio
    } else {
      payload.clear_conversion_ratio = true
    }

    const trimmedOverride = weightOverride.trim()
    let parsedOverride: number | undefined
    if (trimmedOverride !== '') {
      parsedOverride = Number(trimmedOverride)
      if (
        !Number.isInteger(parsedOverride) ||
        parsedOverride < 0 ||
        parsedOverride > 2000
      ) {
        toast.error(t('Weight override must be an integer between 0 and 2000'))
        return
      }
    }
    const freeAfterSave =
      ratioOverrideEnabled && parsedRatio !== undefined
        ? parsedRatio === 0
        : !hasConversionRatioOverride(upstreamKey) &&
          isFreeUpstreamKey(upstreamKey)
    if (freeAfterSave && trimmedOverride !== '') {
      toast.error(t('Free keys always use weight 2000.'))
      return
    }
    const originalWeightOverride = upstreamKey.weight_override ?? null
    if (!freeAfterSave) {
      if (trimmedOverride === '' && originalWeightOverride !== null) {
        payload.clear_weight = true
      } else if (
        parsedOverride !== undefined &&
        parsedOverride !== originalWeightOverride
      ) {
        payload.weight_override = parsedOverride
      }
    }

    setIsSaving(true)
    try {
      const response = await patchUpstreamKey(
        props.channel.id,
        upstreamKey.id,
        payload
      )
      if (!response.success) {
        throw createServerError(response, t('Operation failed'))
      }
      toast.success(t('Operation successful'))
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['upstream-keys', props.channel.id],
        }),
        queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() }),
      ])
      props.onOpenChange(false)
    } catch (error: unknown) {
      handleServerError(error, t('Operation failed'))
    } finally {
      setIsSaving(false)
    }
  }

  const title = props.upstreamKey
    ? `${t('Edit upstream key')}: ${getKeyDisplayName(props.upstreamKey)}`
    : t('Edit upstream key')
  const sourceRatio = props.upstreamKey?.source_conversion_ratio
  const inputRatio = Number(conversionRatio)
  const isFreeWeight =
    ratioOverrideEnabled &&
    conversionRatio.trim() !== '' &&
    Number.isFinite(inputRatio)
      ? inputRatio === 0
      : props.upstreamKey?.conversion_ratio === 0

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={title}
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <div className='flex justify-end gap-2'>
          <Button
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={isSaving}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={handleSave} disabled={isSaving}>
            {isSaving && (
              <Loader2 data-icon='inline-start' className='animate-spin' />
            )}
            {t('Save')}
          </Button>
        </div>
      }
    >
      <div className='grid gap-4 py-4'>
        <div className='bg-muted/30 grid gap-3 rounded-md border p-3 text-sm sm:grid-cols-2'>
          <div className='min-w-0 space-y-1'>
            <div className='text-muted-foreground text-xs'>
              {t('Effective ratio')}
            </div>
            <div className='font-mono tabular-nums'>
              {formatConversionRatio(props.upstreamKey?.conversion_ratio)}
            </div>
          </div>
          <div className='min-w-0 space-y-1'>
            <div className='text-muted-foreground text-xs'>
              {t('Source ratio')}
            </div>
            <div className='font-mono tabular-nums'>
              {formatConversionRatio(sourceRatio)}
            </div>
          </div>
        </div>
        <div className='grid gap-2'>
          <Label>{t('Key priority')}</Label>
          <NumericSpinnerInput
            value={keyPriority}
            min={0}
            max={99}
            onChange={setKeyPriority}
          />
        </div>
        <div className='flex items-center justify-between gap-3 rounded-md border p-3'>
          <Label htmlFor='upstream-key-ratio-override' className='text-sm'>
            {t('Override conversion ratio')}
          </Label>
          <Switch
            id='upstream-key-ratio-override'
            checked={ratioOverrideEnabled}
            onCheckedChange={(checked) => {
              setRatioOverrideEnabled(checked)
              if (props.upstreamKey) {
                setConversionRatio(
                  String(
                    checked
                      ? (props.upstreamKey.conversion_ratio_override ??
                          props.upstreamKey.conversion_ratio)
                      : props.upstreamKey.conversion_ratio
                  )
                )
              }
            }}
          />
        </div>
        <div className='grid gap-2'>
          <Label htmlFor='upstream-key-conversion-ratio'>
            {t('Conversion ratio')}
          </Label>
          <Input
            id='upstream-key-conversion-ratio'
            inputMode='decimal'
            value={conversionRatio}
            disabled={!ratioOverrideEnabled}
            onChange={(event) => setConversionRatio(event.target.value)}
          />
          {!ratioOverrideEnabled && (
            <p className='text-muted-foreground text-xs'>
              {t('Automatic ratio will be restored after saving.')}
            </p>
          )}
        </div>
        <div className='grid gap-2'>
          <Label htmlFor='upstream-key-weight-override'>
            {t('Weight override')}
          </Label>
          <Input
            id='upstream-key-weight-override'
            inputMode='numeric'
            placeholder={t('Use automatic weight')}
            value={weightOverride}
            disabled={isFreeWeight}
            onChange={(event) => setWeightOverride(event.target.value)}
          />
          <p className='text-muted-foreground text-xs'>
            {isFreeWeight
              ? t('Free keys always use weight 2000.')
              : t('Leave empty to use automatic weight.')}
          </p>
        </div>
        <div className='grid gap-3 rounded-md border p-3'>
          <div className='flex items-center justify-between gap-3'>
            <Label htmlFor='upstream-key-allowed-models' className='text-sm'>
              {t('Limit which models can be used with this key')}
            </Label>
            <Switch
              id='upstream-key-allowed-models'
              checked={allowedModelsLimited}
              onCheckedChange={setAllowedModelsLimited}
            />
          </div>
          {allowedModelsLimited && (
            <>
              <MultiSelect
                options={(props.upstreamKey?.models || []).map((model) => ({
                  label: model,
                  value: model,
                }))}
                selected={allowedModels}
                onChange={setAllowedModels}
                placeholder={t('Select models')}
                emptyText={t('No models available')}
              />
              <div className='text-muted-foreground text-xs'>
                {t('Synced')}: {props.upstreamKey?.models.length ?? 0}
              </div>
            </>
          )}
        </div>
      </div>
    </Dialog>
  )
}

export function UpstreamKeysSubTable(props: UpstreamKeysSubTableProps) {
  const { t } = useTranslation()
  const { sensitiveVisible } = useChannels()
  const [editingKey, setEditingKey] = useState<UpstreamKey | null>(null)
  const keys = sortUpstreamKeysForDisplay(props.channel.upstream_keys || [])

  const columns = useMemo<StaticDataTableColumn<UpstreamKey>[]>(
    () => [
      {
        id: 'key-id',
        header: t('Key ID'),
        className: 'w-20 min-w-20 text-left',
        cellClassName: 'text-left',
        cell: (upstreamKey: UpstreamKey) => (
          <StatusBadge
            label={`#${upstreamKey.key_id || upstreamKey.id}`}
            copyText={String(upstreamKey.key_id || upstreamKey.id)}
            size='sm'
            showDot={false}
            className='font-mono'
          />
        ),
      },
      {
        id: 'name',
        header: t('Name'),
        className: 'w-32 min-w-32 max-w-32 text-left',
        cellClassName: 'text-left',
        cell: (upstreamKey: UpstreamKey) => (
          <TruncatedText
            text={
              sensitiveVisible ? getKeyDisplayName(upstreamKey) : SENSITIVE_MASK
            }
            maxWidth='max-w-32'
            className='font-medium'
          />
        ),
      },
      {
        id: 'key',
        header: t('Key'),
        className: 'w-40 min-w-40 max-w-40 text-left',
        cellClassName: 'text-left',
        cell: (upstreamKey: UpstreamKey) => (
          <span
            className='block max-w-40 truncate font-mono text-sm'
            title={
              sensitiveVisible ? upstreamKey.key_preview || '-' : undefined
            }
          >
            {sensitiveVisible ? upstreamKey.key_preview || '-' : SENSITIVE_MASK}
          </span>
        ),
      },
      {
        id: 'status',
        header: t('Status'),
        className: 'w-28 text-center',
        cellClassName: 'text-center',
        cell: (upstreamKey: UpstreamKey) => (
          <UpstreamKeyStatusBadge upstreamKey={upstreamKey} />
        ),
      },
      {
        id: 'models',
        header: t('Models'),
        width: '334.95px',
        minWidth: '334.95px',
        maxWidth: '334.95px',
        className: 'w-[334.95px] min-w-[334.95px] max-w-[334.95px] text-left',
        cellClassName:
          'w-[334.95px] min-w-[334.95px] max-w-[334.95px] text-left',
        cell: (upstreamKey: UpstreamKey) => (
          <UpstreamKeyModelsCell upstreamKey={upstreamKey} />
        ),
      },
      {
        id: 'converted-ratio',
        header: t('Converted ratio'),
        className: 'w-32 text-center',
        cellClassName: 'text-center',
        cell: (upstreamKey: UpstreamKey) => (
          <UpstreamKeyRatioCell upstreamKey={upstreamKey} />
        ),
      },
      {
        id: 'key-priority',
        header: t('Key priority'),
        className: 'w-32 text-center',
        cellClassName: 'text-center',
        cell: (upstreamKey: UpstreamKey) => (
          <UpstreamKeyPriorityCell
            channelId={props.channel.id}
            upstreamKey={upstreamKey}
          />
        ),
      },
      {
        id: 'key-weight',
        header: t('Key weight'),
        className: 'w-32 text-center',
        cellClassName: 'text-center',
        cell: (upstreamKey: UpstreamKey) => (
          <UpstreamKeyWeightCell upstreamKey={upstreamKey} />
        ),
      },
      {
        id: 'last-sync',
        header: t('Last sync'),
        className: 'w-36 text-center',
        cellClassName: 'text-center',
        cell: (upstreamKey: UpstreamKey) => (
          <LastSyncCell timestamp={upstreamKey.last_sync_at} />
        ),
      },
      {
        id: 'actions',
        header: t('Actions'),
        pinned: 'right',
        className: `${CHANNEL_ACTIONS_COLUMN_CLASS_NAME} text-center`,
        cellClassName: `${CHANNEL_ACTIONS_COLUMN_CLASS_NAME} text-center`,
        cell: (upstreamKey: UpstreamKey) => (
          <UpstreamKeyActions
            channel={props.channel}
            upstreamKey={upstreamKey}
            onEdit={setEditingKey}
          />
        ),
      },
    ],
    [props.channel, sensitiveVisible, t]
  )

  return (
    <div className='border-border bg-muted/20 relative z-20 w-full max-w-none min-w-0 overflow-visible border-y px-3 py-3'>
      <StaticDataTable
        className='relative min-w-0 !overflow-visible !rounded-none !border-0 !bg-transparent'
        tableClassName='bg-background w-max min-w-full table-fixed'
        tableProps={{ withContainer: false }}
        data={keys}
        columns={columns}
        getRowKey={(upstreamKey) => upstreamKey.id}
        emptyContent={
          <span className='text-muted-foreground text-sm'>
            {t('No upstream keys')}
          </span>
        }
      />
      <UpstreamKeyEditDialog
        channel={props.channel}
        upstreamKey={editingKey}
        open={editingKey !== null}
        onOpenChange={(open) => {
          if (!open) {
            setEditingKey(null)
          }
        }}
      />
    </div>
  )
}
