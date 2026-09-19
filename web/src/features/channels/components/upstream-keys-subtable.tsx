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
import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import { TruncatedText } from '@/components/truncated-text'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { CHANNEL_STATUS_CONFIG } from '../constants'
import {
  channelsQueryKeys,
  formatConversionRatio,
  formatRelativeTime,
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

function useUpstreamKeySelection(channel: Channel, keys: UpstreamKey[]) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedIds, setSelectedIds] = useState<Set<number>>(() => new Set())
  const [isBatchUpdating, setIsBatchUpdating] = useState(false)

  useEffect(() => {
    const visibleIds = new Set(keys.map((upstreamKey) => upstreamKey.id))
    setSelectedIds((previous) => {
      const next = new Set<number>()
      for (const id of previous) {
        if (visibleIds.has(id)) {
          next.add(id)
        }
      }
      if (next.size === previous.size) {
        return previous
      }
      return next
    })
  }, [keys])

  const selectedIdList = useMemo(() => [...selectedIds], [selectedIds])
  const allSelected = keys.length > 0 && selectedIds.size === keys.length
  const partiallySelected =
    selectedIds.size > 0 && selectedIds.size < keys.length

  const toggleSelected = useCallback((id: number, checked: boolean) => {
    setSelectedIds((previous) => {
      const next = new Set(previous)
      if (checked) {
        next.add(id)
      } else {
        next.delete(id)
      }
      return next
    })
  }, [])

  const toggleAll = useCallback(
    (checked: boolean) => {
      if (!checked) {
        setSelectedIds(new Set())
        return
      }
      setSelectedIds(new Set(keys.map((upstreamKey) => upstreamKey.id)))
    },
    [keys]
  )

  const clearSelection = useCallback(() => {
    setSelectedIds(new Set())
  }, [])

  const updateSelectedStatus = useCallback(
    async (status: number) => {
      if (selectedIdList.length === 0) {
        return
      }

      setIsBatchUpdating(true)
      try {
        const response = await batchUpdateUpstreamKeyStatus(
          channel.id,
          selectedIdList,
          status
        )
        if (!response.success) {
          throw createServerError(response, t('Operation failed'))
        }
        toast.success(t('Operation successful'))
        setSelectedIds(new Set())
        await Promise.all([
          queryClient.invalidateQueries({
            queryKey: ['upstream-keys', channel.id],
          }),
          queryClient.invalidateQueries({
            queryKey: channelsQueryKeys.lists(),
          }),
        ])
      } catch (error: unknown) {
        handleServerError(error, t('Operation failed'))
      } finally {
        setIsBatchUpdating(false)
      }
    },
    [channel.id, queryClient, selectedIdList, t]
  )

  return useMemo(
    () => ({
      allSelected,
      clearSelection,
      isBatchUpdating,
      partiallySelected,
      selectedIds,
      selectedIdList,
      toggleAll,
      toggleSelected,
      updateSelectedStatus,
    }),
    [
      allSelected,
      clearSelection,
      isBatchUpdating,
      partiallySelected,
      selectedIds,
      selectedIdList,
      toggleAll,
      toggleSelected,
      updateSelectedStatus,
    ]
  )
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
  const config = getUpstreamKeyStatusConfig(upstreamKey.status)
  let availabilityLabel = t(config.label)
  let availabilityVariant: StatusBadgeProps['variant'] = config.variant
  switch (upstreamKey.availability_reason) {
    case 'credential_unavailable':
      availabilityLabel = t('Credential unavailable')
      availabilityVariant = 'danger'
      break
    case 'models_unavailable':
      availabilityLabel = t('Model capability unavailable')
      break
    case 'snapshot_only':
      availabilityLabel = t('Historical snapshot')
      break
    case 'missing':
      availabilityLabel = t('Missing')
      break
    case 'expired':
      availabilityLabel = t('Expired')
      break
    case 'quota_exhausted':
      availabilityLabel = t('Quota exhausted')
      break
    case 'upstream_disabled':
      availabilityLabel = t('Upstream disabled')
      break
    case 'manual_disabled':
      availabilityLabel = t('Manually disabled')
      break
  }

  return (
    <div className='flex flex-wrap items-center justify-center gap-1'>
      <StatusBadge
        label={
          upstreamKey.routable === true
            ? t('Routable')
            : availabilityLabel
        }
        variant={upstreamKey.routable === true ? 'success' : availabilityVariant}
        size='sm'
        copyable={false}
      />
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
      <span
        className='text-muted-foreground font-mono text-[11px] tabular-nums'
        title={t('Automatic weight')}
      >
        ({upstreamKey.auto_weight ?? upstreamKey.weight})
      </span>
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
      <StatusBadge
        label={upstreamKey.models_synced ? t('Synced') : t('Unavailable')}
        variant={upstreamKey.models_synced ? 'success' : 'warning'}
        size='sm'
        copyable={false}
      />
    </div>
  )
}

function UpstreamKeyBatchToolbar(props: {
  allSelected: boolean
  disabled: boolean
  onClear: () => void
  onDisable: () => void
  onEnable: () => void
  onToggleAll: (checked: boolean) => void
  partiallySelected: boolean
  selectedCount: number
}) {
  const { t } = useTranslation()
  const checked = props.allSelected

  return (
    <div className='mb-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
      <label className='flex min-w-0 items-center gap-2 text-sm'>
        <Checkbox
          checked={checked}
          indeterminate={props.partiallySelected}
          onCheckedChange={(value) => props.onToggleAll(!!value)}
          aria-label={t('Select all upstream keys')}
        />
        <span className='text-muted-foreground min-w-0'>
          {props.selectedCount} {t('selected')}
        </span>
      </label>
      <div className='flex flex-wrap items-center gap-2'>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={props.disabled}
          onClick={props.onEnable}
        >
          <Power />
          {t('Enable selected keys')}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={props.disabled}
          onClick={props.onDisable}
        >
          <PowerOff />
          {t('Disable selected keys')}
        </Button>
        {props.selectedCount > 0 && (
          <Button
            type='button'
            variant='ghost'
            size='sm'
            disabled={props.disabled}
            onClick={props.onClear}
          >
            {t('Clear selection')}
          </Button>
        )}
      </div>
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

  const openChannelDialog = (
    dialog: 'test-channel' | 'balance-query' | 'fetch-models'
  ) => {
    setCurrentRow(channel)
    setCurrentUpstreamKey(upstreamKey)
    setOpen(dialog)
  }

  const handleToggleStatus = async () => {
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
  }

  return (
    <div className='flex items-center justify-center gap-1'>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => openChannelDialog('test-channel')}
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
              onClick={handleToggleStatus}
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
              onClick={() => onEdit(upstreamKey)}
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
            <DropdownMenuItem onClick={() => openChannelDialog('test-channel')}>
              {t('Test Connection')}
              <DropdownMenuShortcut>
                <Gauge size={16} />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
            <DropdownMenuItem
              onClick={() => openChannelDialog('balance-query')}
            >
              {t('Query Balance')}
              <DropdownMenuShortcut>
                <DollarSign size={16} />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => openChannelDialog('fetch-models')}>
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
  const keys = props.channel.upstream_keys || []
  const selection = useUpstreamKeySelection(props.channel, keys)
  const batchDisabled =
    selection.isBatchUpdating || selection.selectedIdList.length === 0

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
        <>
          <UpstreamKeyBatchToolbar
            allSelected={selection.allSelected}
            disabled={batchDisabled}
            onClear={selection.clearSelection}
            onDisable={() => selection.updateSelectedStatus(2)}
            onEnable={() => selection.updateSelectedStatus(1)}
            onToggleAll={selection.toggleAll}
            partiallySelected={selection.partiallySelected}
            selectedCount={selection.selectedIdList.length}
          />
          <div className='divide-y rounded-md border'>
            {keys.map((upstreamKey) => (
              <div key={upstreamKey.id} className='space-y-3 p-3'>
                <div className='flex min-w-0 items-start justify-between gap-2'>
                  <div className='flex min-w-0 items-start gap-2'>
                    <Checkbox
                      checked={selection.selectedIds.has(upstreamKey.id)}
                      onCheckedChange={(value) =>
                        selection.toggleSelected(upstreamKey.id, !!value)
                      }
                      aria-label={t('Select upstream key')}
                      className='mt-0.5'
                    />
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
                    <span className='font-mono tabular-nums'>
                      {upstreamKey.key_priority}
                    </span>
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
        </>
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
  }, [props.open, props.upstreamKey])

  const handleSave = async () => {
    const upstreamKey = props.upstreamKey
    if (!upstreamKey) {
      return
    }

    const payload: Parameters<typeof patchUpstreamKey>[2] = {
      key_priority: keyPriority,
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
            min={-999}
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
      </div>
    </Dialog>
  )
}

export function UpstreamKeysSubTable(props: UpstreamKeysSubTableProps) {
  const { t } = useTranslation()
  const { sensitiveVisible } = useChannels()
  const [editingKey, setEditingKey] = useState<UpstreamKey | null>(null)
  const keys = props.channel.upstream_keys || []
  const selection = useUpstreamKeySelection(props.channel, keys)
  const batchDisabled =
    selection.isBatchUpdating || selection.selectedIdList.length === 0

  const columns = useMemo<StaticDataTableColumn<UpstreamKey>[]>(
    () => [
      {
        id: 'select',
        header: (
          <Checkbox
            checked={selection.allSelected}
            indeterminate={selection.partiallySelected}
            onCheckedChange={(value) => selection.toggleAll(!!value)}
            aria-label={t('Select all upstream keys')}
          />
        ),
        className: 'w-10 text-center',
        cellClassName: 'text-center',
        cell: (upstreamKey: UpstreamKey) => (
          <Checkbox
            checked={selection.selectedIds.has(upstreamKey.id)}
            onCheckedChange={(value) =>
              selection.toggleSelected(upstreamKey.id, !!value)
            }
            aria-label={t('Select upstream key')}
          />
        ),
      },
      {
        id: 'name',
        header: t('Name'),
        className: 'min-w-40 text-left',
        cellClassName: 'text-left',
        cell: (upstreamKey: UpstreamKey) => (
          <TruncatedText
            text={
              sensitiveVisible ? getKeyDisplayName(upstreamKey) : SENSITIVE_MASK
            }
            maxWidth='max-w-[180px]'
            className='font-medium'
          />
        ),
      },
      {
        id: 'key',
        header: t('Key'),
        className: 'min-w-44 text-left',
        cellClassName: 'text-left',
        cell: (upstreamKey: UpstreamKey) => (
          <span
            className='font-mono text-sm'
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
        className: 'min-w-56 text-left',
        cellClassName: 'text-left',
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
        cellClassName: 'text-center font-mono tabular-nums',
        cell: (upstreamKey: UpstreamKey) => upstreamKey.key_priority,
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
        className: 'w-48 min-w-48 text-center',
        cellClassName: 'w-48 min-w-48 text-center',
        cell: (upstreamKey: UpstreamKey) => (
          <UpstreamKeyActions
            channel={props.channel}
            upstreamKey={upstreamKey}
            onEdit={setEditingKey}
          />
        ),
      },
    ],
    [props.channel, selection, sensitiveVisible, t]
  )

  return (
    <div className='border-border bg-muted/20 border-y px-3 py-3'>
      {keys.length > 0 && (
        <UpstreamKeyBatchToolbar
          allSelected={selection.allSelected}
          disabled={batchDisabled}
          onClear={selection.clearSelection}
          onDisable={() => selection.updateSelectedStatus(2)}
          onEnable={() => selection.updateSelectedStatus(1)}
          onToggleAll={selection.toggleAll}
          partiallySelected={selection.partiallySelected}
          selectedCount={selection.selectedIdList.length}
        />
      )}
      <StaticDataTable
        className='bg-background rounded-md'
        tableClassName='min-w-[1320px]'
        containerProps={{ style: { overflow: 'auto' } }}
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
