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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { BadgeListCell, StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
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
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency } from '@/lib/currency'
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

function formatOptionalQuota(value: number | null | undefined): string {
  if (value === null || value === undefined) {
    return '-'
  }

  return formatQuotaWithCurrency(value, {
    digitsLarge: 2,
    digitsSmall: 4,
    abbreviate: true,
  })
}

function getKeyDisplayName(upstreamKey: UpstreamKey): string {
  return upstreamKey.name || upstreamKey.external_id || `#${upstreamKey.id}`
}

function getEffectiveWeight(upstreamKey: UpstreamKey): number {
  if (upstreamKey.conversion_ratio === 0) {
    return 2000
  }
  return upstreamKey.weight_override ?? upstreamKey.weight
}

function LastUsedCell({ timestamp }: { timestamp: number }) {
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

  return (
    <StatusBadge
      label={t(config.label)}
      variant={config.variant}
      size='sm'
      copyable={false}
    />
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
        <div className='divide-y rounded-md border'>
          {keys.map((upstreamKey) => (
            <div key={upstreamKey.id} className='space-y-3 p-3'>
              <div className='flex min-w-0 items-start justify-between gap-2'>
                <TruncatedText
                  text={
                    sensitiveVisible
                      ? getKeyDisplayName(upstreamKey)
                      : SENSITIVE_MASK
                  }
                  maxWidth='max-w-[calc(100%-5rem)]'
                  className='font-medium'
                />
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
                  <span className='block max-w-full truncate font-mono text-xs'>
                    {upstreamKey.models.length > 0
                      ? upstreamKey.models.join(', ')
                      : '-'}
                  </span>
                </UpstreamKeyMobileField>
                <UpstreamKeyMobileField label={t('Converted ratio')}>
                  <span className='font-mono tabular-nums'>
                    {formatConversionRatio(upstreamKey.conversion_ratio)}
                  </span>
                </UpstreamKeyMobileField>
                <UpstreamKeyMobileField label={t('Key priority')}>
                  <span className='font-mono tabular-nums'>
                    {upstreamKey.key_priority}
                  </span>
                </UpstreamKeyMobileField>
                <UpstreamKeyMobileField label={t('Key weight')}>
                  <UpstreamKeyWeightCell upstreamKey={upstreamKey} />
                </UpstreamKeyMobileField>
                <UpstreamKeyMobileField label={t('Upstream used')}>
                  {formatOptionalQuota(upstreamKey.used_quota)}
                </UpstreamKeyMobileField>
                <UpstreamKeyMobileField label={t('Last used')}>
                  <LastUsedCell timestamp={upstreamKey.last_used_at} />
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
  const [conversionRatio, setConversionRatio] = useState('')
  const [weightOverride, setWeightOverride] = useState('')
  const [isSaving, setIsSaving] = useState(false)

  useEffect(() => {
    if (!props.upstreamKey || !props.open) {
      return
    }

    setKeyPriority(props.upstreamKey.key_priority)
    setConversionRatio(String(props.upstreamKey.conversion_ratio))
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

    const parsedRatio = Number(conversionRatio)
    if (!Number.isFinite(parsedRatio) || parsedRatio < 0) {
      toast.error(t('Conversion ratio must be a finite non-negative number'))
      return
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

    setIsSaving(true)
    try {
      const response = await patchUpstreamKey(
        props.channel.id,
        upstreamKey.id,
        {
          key_priority: keyPriority,
          conversion_ratio: parsedRatio,
          ...(trimmedOverride === ''
            ? { clear_weight: true }
            : { weight_override: parsedOverride }),
        }
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
        <div className='grid gap-2'>
          <Label>{t('Key priority')}</Label>
          <NumericSpinnerInput
            value={keyPriority}
            min={-999}
            onChange={setKeyPriority}
          />
        </div>
        <div className='grid gap-2'>
          <Label htmlFor='upstream-key-conversion-ratio'>
            {t('Converted ratio')}
          </Label>
          <Input
            id='upstream-key-conversion-ratio'
            inputMode='decimal'
            value={conversionRatio}
            onChange={(event) => setConversionRatio(event.target.value)}
          />
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
            onChange={(event) => setWeightOverride(event.target.value)}
          />
          <p className='text-muted-foreground text-xs'>
            {t('Leave empty to use automatic weight.')}
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

  const columns = useMemo(
    () => [
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
        ),
      },
      {
        id: 'converted-ratio',
        header: t('Converted ratio'),
        className: 'w-32 text-center',
        cellClassName: 'text-center font-mono tabular-nums',
        cell: (upstreamKey: UpstreamKey) =>
          formatConversionRatio(upstreamKey.conversion_ratio),
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
        id: 'upstream-used',
        header: t('Upstream used'),
        className: 'w-32 text-center',
        cellClassName: 'text-center',
        cell: (upstreamKey: UpstreamKey) =>
          formatOptionalQuota(upstreamKey.used_quota),
      },
      {
        id: 'last-used',
        header: t('Last used'),
        className: 'w-36 text-center',
        cellClassName: 'text-center',
        cell: (upstreamKey: UpstreamKey) => (
          <LastUsedCell timestamp={upstreamKey.last_used_at} />
        ),
      },
      {
        id: 'actions',
        header: t('Actions'),
        className: 'w-48 text-center',
        cellClassName: 'text-center',
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
    <div className='border-border bg-muted/20 border-y px-3 py-3'>
      <StaticDataTable
        className='bg-background rounded-md'
        tableClassName='min-w-[1180px]'
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
