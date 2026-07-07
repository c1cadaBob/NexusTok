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
import { useQueryClient } from '@tanstack/react-query'
import { type Row } from '@tanstack/react-table'
import {
  MoreHorizontal,
  Boxes,
  Pencil,
  TestTube,
  Gauge,
  DollarSign,
  Download,
  Copy,
  Power,
  PowerOff,
  Key,
  Trash2,
  RefreshCw,
  Loader2,
  UsersRound,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { MODEL_FETCHABLE_TYPES } from '../constants'
import {
  channelsQueryKeys,
  handleDeleteChannel,
  handleTestChannel,
  handleToggleChannelStatus,
  isChannelEnabled,
  isMultiKeyChannel,
} from '../lib'
import { parseUpstreamUpdateMeta } from '../lib/upstream-update-utils'
import type { Channel } from '../types'
import { useChannelPermissions } from '../hooks/use-channel-permissions'
import { useChannels } from './channels-provider'

interface DataTableRowActionsProps {
  row: Row<Channel>
}

export function DataTableRowActions({ row }: DataTableRowActionsProps) {
  const { t } = useTranslation()
  const channel = row.original
  const { setOpen, setCurrentRow, upstream } = useChannels()
  const queryClient = useQueryClient()
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)
  const [isTesting, setIsTesting] = useState(false)
  const [isTogglingStatus, setIsTogglingStatus] = useState(false)
  const permissions = useChannelPermissions()
  const noPermissionMessage = t("You don't have necessary permission")

  const isEnabled = isChannelEnabled(channel)
  const isMultiKey = isMultiKeyChannel(channel)
  const canFetchAndSaveModels = permissions.canOperate && permissions.canWrite
  const canManageChannelAccounts =
    permissions.canReadAccountPool && permissions.canSensitiveWrite

  const guardPermission = (allowed: boolean) => {
    if (allowed) return true
    toast.error(noPermissionMessage)
    return false
  }

  const handleEdit = () => {
    if (!guardPermission(permissions.canWrite)) return
    setCurrentRow(channel)
    setOpen('update-channel')
  }

  const handleTest = () => {
    if (!guardPermission(permissions.canOperate)) return
    setCurrentRow(channel)
    setOpen('test-channel')
  }

  const handleDirectTest = async (e: React.MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation()
    if (!guardPermission(permissions.canOperate)) return
    setIsTesting(true)
    try {
      await handleTestChannel(channel.id, undefined, () => {
        queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
      })
    } finally {
      setIsTesting(false)
    }
  }

  const handleQueryBalance = () => {
    if (!guardPermission(permissions.canOperate)) return
    setCurrentRow(channel)
    setOpen('balance-query')
  }

  const handleFetchModels = () => {
    if (!guardPermission(canFetchAndSaveModels)) return
    setCurrentRow(channel)
    setOpen('fetch-models')
  }

  const handleManageOllamaModels = () => {
    if (!guardPermission(permissions.canOperate)) return
    setCurrentRow(channel)
    setOpen('ollama-models')
  }

  const handleCopy = () => {
    if (!guardPermission(permissions.canSensitiveWrite)) return
    setCurrentRow(channel)
    setOpen('copy-channel')
  }

  const handleManageKeys = () => {
    if (!guardPermission(permissions.canSensitiveWrite)) return
    setCurrentRow(channel)
    setOpen('multi-key-manage')
  }

  const handleManageAccountPool = () => {
    if (!guardPermission(canManageChannelAccounts)) return
    setCurrentRow(channel)
    setOpen('account-pool-manage')
  }

  const handleToggleStatus = async (
    e?: React.MouseEvent<HTMLButtonElement>
  ) => {
    e?.stopPropagation()
    if (!guardPermission(permissions.canOperate)) return
    setIsTogglingStatus(true)
    try {
      await handleToggleChannelStatus(channel.id, channel.status, queryClient)
    } finally {
      setIsTogglingStatus(false)
    }
  }

  return (
    <div className='flex items-center justify-end gap-1'>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleDirectTest}
              disabled={!permissions.canOperate || isTesting}
              aria-label={t('Test Connection')}
            />
          }
        >
          {isTesting ? (
            <Loader2 className='size-4 animate-spin' />
          ) : (
            <Gauge className='size-4' />
          )}
        </TooltipTrigger>
        <TooltipContent>
          {permissions.canOperate ? t('Test Connection') : noPermissionMessage}
        </TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={handleToggleStatus}
              disabled={!permissions.canOperate || isTogglingStatus}
              aria-label={isEnabled ? t('Disable') : t('Enable')}
              className={
                isEnabled
                  ? 'text-destructive hover:text-destructive'
                  : 'text-emerald-600 hover:text-emerald-600 dark:text-emerald-400 dark:hover:text-emerald-400'
              }
            />
          }
        >
          {isTogglingStatus ? (
            <Loader2 className='size-4 animate-spin' />
          ) : isEnabled ? (
            <PowerOff className='size-4' />
          ) : (
            <Power className='size-4' />
          )}
        </TooltipTrigger>
        <TooltipContent>
          {permissions.canOperate
            ? isEnabled
              ? t('Disable')
              : t('Enable')
            : noPermissionMessage}
        </TooltipContent>
      </Tooltip>

      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant='ghost'
              className='data-popup-open:bg-muted flex h-8 w-8 p-0'
            />
          }
        >
          <MoreHorizontal className='h-4 w-4' />
          <span className='sr-only'>{t('Open menu')}</span>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' className='w-48'>
          {/* Edit */}
          <DropdownMenuItem
            onClick={handleEdit}
            disabled={!permissions.canWrite}
            title={permissions.canWrite ? undefined : noPermissionMessage}
          >
            {t('Edit')}
            <DropdownMenuShortcut>
              <Pencil size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          {/* Test Connection */}
          <DropdownMenuItem
            onClick={handleTest}
            disabled={!permissions.canOperate}
            title={permissions.canOperate ? undefined : noPermissionMessage}
          >
            {t('Test Connection')}
            <DropdownMenuShortcut>
              <TestTube size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          {/* Query Balance */}
          <DropdownMenuItem
            onClick={handleQueryBalance}
            disabled={!permissions.canOperate}
            title={permissions.canOperate ? undefined : noPermissionMessage}
          >
            {t('Query Balance')}
            <DropdownMenuShortcut>
              <DollarSign size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          {/* Fetch Models */}
          <DropdownMenuItem
            onClick={handleFetchModels}
            disabled={!canFetchAndSaveModels}
            title={canFetchAndSaveModels ? undefined : noPermissionMessage}
          >
            {t('Fetch Models')}
            <DropdownMenuShortcut>
              <Download size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          {/* Detect Upstream Updates (only for fetchable channel types) */}
          {MODEL_FETCHABLE_TYPES.has(channel.type) && (
            <DropdownMenuItem
              onClick={() => {
                if (!guardPermission(permissions.canOperate)) return
                const meta = parseUpstreamUpdateMeta(channel.settings)
                if (
                  meta.pendingAddModels.length > 0 ||
                  meta.pendingRemoveModels.length > 0
                ) {
                  if (!guardPermission(permissions.canSensitiveWrite)) return
                  upstream.openModal(
                    channel,
                    meta.pendingAddModels,
                    meta.pendingRemoveModels,
                    meta.pendingAddModels.length > 0 ? 'add' : 'remove'
                  )
                } else {
                  upstream.detectChannelUpdates(channel)
                }
              }}
              disabled={!permissions.canOperate}
              title={permissions.canOperate ? undefined : noPermissionMessage}
            >
              {t('Upstream Updates')}
              <DropdownMenuShortcut>
                <RefreshCw size={16} />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
          )}

          {/* Ollama Models (only for Ollama channels) */}
          {channel.type === 4 && (
            <DropdownMenuItem
              onClick={handleManageOllamaModels}
              disabled={!permissions.canOperate}
              title={permissions.canOperate ? undefined : noPermissionMessage}
            >
              {t('Manage Ollama Models')}
              <DropdownMenuShortcut>
                <Boxes size={16} />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
          )}

          <DropdownMenuSeparator />

          {/* Copy Channel */}
          <DropdownMenuItem
            onClick={handleCopy}
            disabled={!permissions.canSensitiveWrite}
            title={
              permissions.canSensitiveWrite ? undefined : noPermissionMessage
            }
          >
            {t('Copy Channel')}
            <DropdownMenuShortcut>
              <Copy size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          {/* Manage Keys (only for multi-key channels) */}
          {isMultiKey && (
            <DropdownMenuItem
              onClick={handleManageKeys}
              disabled={!permissions.canSensitiveWrite}
              title={
                permissions.canSensitiveWrite ? undefined : noPermissionMessage
              }
            >
              {t('Manage Keys')}
              <DropdownMenuShortcut>
                <Key size={16} />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
          )}

          <DropdownMenuItem
            onClick={handleManageAccountPool}
            disabled={!canManageChannelAccounts}
            title={canManageChannelAccounts ? undefined : noPermissionMessage}
          >
            {t('Account Pool')}
            <DropdownMenuShortcut>
              <UsersRound size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          <DropdownMenuSeparator />

          {/* Delete */}
          <DropdownMenuItem
            onSelect={(e) => {
              e.preventDefault()
              if (!guardPermission(permissions.canSensitiveWrite)) return
              setDeleteConfirmOpen(true)
            }}
            disabled={!permissions.canSensitiveWrite}
            className='text-destructive focus:text-destructive'
          >
            {t('Delete')}
            <DropdownMenuShortcut>
              <Trash2 size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <ConfirmDialog
        open={deleteConfirmOpen}
        onOpenChange={setDeleteConfirmOpen}
        title={t('Delete Channel')}
        desc={t(
          'Are you sure you want to delete channel "{{name}}"? This action cannot be undone.',
          { name: channel.name }
        )}
        confirmText={t('Delete')}
        destructive
        handleConfirm={() => {
          if (!guardPermission(permissions.canSensitiveWrite)) return
          handleDeleteChannel(channel.id, queryClient)
          setDeleteConfirmOpen(false)
        }}
      />
    </div>
  )
}
