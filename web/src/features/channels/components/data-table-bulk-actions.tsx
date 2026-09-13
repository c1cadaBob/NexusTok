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
import type { Table } from '@tanstack/react-table'
import { Power, PowerOff, Tag, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { batchUpdateUpstreamKeyStatus } from '../api'
import { CHANNEL_STATUS } from '../constants'
import {
  handleBatchDelete,
  handleBatchDisable,
  handleBatchEnable,
  handleBatchSetTag,
  channelsQueryKeys,
  isTagAggregateRow,
} from '../lib'
import type { Channel } from '../types'

interface DataTableBulkActionsProps<TData> {
  table: Table<TData>
}

export function DataTableBulkActions<TData>({
  table,
}: DataTableBulkActionsProps<TData>) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [showTagDialog, setShowTagDialog] = useState(false)
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)
  const [tagValue, setTagValue] = useState('')
  const currentUser = useAuthStore((s) => s.auth.user)
  const canEditSensitive = hasPermission(
    currentUser,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )

  const selectedRows = table.getFilteredSelectedRowModel().rows
  const selectedChannelIds = new Set<number>()
  const selectedUpstreamKeyIdsByChannelId = new Map<number, number[]>()

  for (const row of selectedRows) {
    const channel = row.original as Channel
    if (isTagAggregateRow(channel)) {
      continue
    }

    if (!channel.is_upstream_key && typeof channel.id === 'number') {
      selectedChannelIds.add(channel.id)
      continue
    }

    const parentChannelId =
      channel.parent_channel_id ?? channel.upstream_key?.channel_id
    const upstreamKeyId = channel.upstream_key?.id ?? channel.id
    if (
      typeof parentChannelId !== 'number' ||
      typeof upstreamKeyId !== 'number' ||
      selectedChannelIds.has(parentChannelId)
    ) {
      continue
    }

    const keyIds = selectedUpstreamKeyIdsByChannelId.get(parentChannelId) || []
    if (!keyIds.includes(upstreamKeyId)) {
      keyIds.push(upstreamKeyId)
      selectedUpstreamKeyIdsByChannelId.set(parentChannelId, keyIds)
    }
  }

  const selectedIds = [...selectedChannelIds]

  const handleClearSelection = () => {
    table.resetRowSelection()
  }

  const updateSelectedUpstreamKeyStatus = async (
    status: number
  ): Promise<number> => {
    let updatedCount = 0

    for (const [channelId, keyIds] of selectedUpstreamKeyIdsByChannelId) {
      try {
        const response = await batchUpdateUpstreamKeyStatus(
          channelId,
          keyIds,
          status
        )
        if (!response.success) {
          handleServerError(
            createServerError(
              response,
              status === CHANNEL_STATUS.ENABLED
                ? t('Failed to enable upstream keys')
                : t('Failed to disable upstream keys')
            )
          )
          continue
        }

        updatedCount += response.data?.updated || 0
        await queryClient.invalidateQueries({
          queryKey: ['upstream-keys', channelId],
        })
      } catch (error) {
        handleServerError(
          error,
          status === CHANNEL_STATUS.ENABLED
            ? t('Failed to enable upstream keys')
            : t('Failed to disable upstream keys')
        )
      }
    }

    if (updatedCount > 0) {
      toast.success(
        status === CHANNEL_STATUS.ENABLED
          ? t('{{count}} upstream key(s) enabled', { count: updatedCount })
          : t('{{count}} upstream key(s) disabled', { count: updatedCount })
      )
      await queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.lists(),
      })
    }

    return updatedCount
  }

  const handleEnableAll = async () => {
    const channelsUpdated =
      selectedIds.length > 0
        ? await handleBatchEnable(selectedIds, queryClient)
        : false
    const keysUpdated = await updateSelectedUpstreamKeyStatus(
      CHANNEL_STATUS.ENABLED
    )
    if (channelsUpdated || keysUpdated > 0) {
      handleClearSelection()
    }
  }

  const handleDisableAll = async () => {
    const channelsUpdated =
      selectedIds.length > 0
        ? await handleBatchDisable(selectedIds, queryClient)
        : false
    const keysUpdated = await updateSelectedUpstreamKeyStatus(
      CHANNEL_STATUS.MANUAL_DISABLED
    )
    if (channelsUpdated || keysUpdated > 0) {
      handleClearSelection()
    }
  }

  const handleDeleteAll = () => {
    if (!canEditSensitive || selectedIds.length === 0) return
    handleBatchDelete(selectedIds, queryClient, () => {
      setShowDeleteConfirm(false)
      handleClearSelection()
    })
  }

  const handleSetTag = () => {
    if (selectedIds.length === 0) return
    handleBatchSetTag(selectedIds, tagValue || null, queryClient, () => {
      setShowTagDialog(false)
      setTagValue('')
      handleClearSelection()
    })
  }

  const tagActionTooltip =
    selectedIds.length > 0
      ? t('Set tag for selected channels')
      : t('Select a parent channel')
  let deleteActionTooltip = t('No permission to perform this action')
  if (canEditSensitive) {
    deleteActionTooltip =
      selectedIds.length > 0
        ? t('Delete selected channels')
        : t('Select a parent channel')
  }

  return (
    <>
      <BulkActionsToolbar table={table} entityName='channel'>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                onClick={handleEnableAll}
                className='size-8'
                aria-label={t('Enable selected channels')}
                title={t('Enable selected channels')}
              />
            }
          >
            <Power />
            <span className='sr-only'>{t('Enable selected channels')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Enable selected channels')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                onClick={handleDisableAll}
                className='size-8'
                aria-label={t('Disable selected channels')}
                title={t('Disable selected channels')}
              />
            }
          >
            <PowerOff />
            <span className='sr-only'>{t('Disable selected channels')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Disable selected channels')}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                onClick={() => setShowTagDialog(true)}
                disabled={selectedIds.length === 0}
                className='size-8'
                aria-label={t('Set tag for selected channels')}
                title={tagActionTooltip}
              />
            }
          >
            <Tag />
            <span className='sr-only'>
              {t('Set tag for selected channels')}
            </span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{tagActionTooltip}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='destructive'
                size='icon'
                onClick={() => {
                  if (!canEditSensitive || selectedIds.length === 0) return
                  setShowDeleteConfirm(true)
                }}
                disabled={!canEditSensitive || selectedIds.length === 0}
                aria-disabled={!canEditSensitive || selectedIds.length === 0}
                className={cn(
                  'size-8',
                  (!canEditSensitive || selectedIds.length === 0) &&
                    'cursor-not-allowed opacity-50'
                )}
                aria-label={t('Delete selected channels')}
                title={deleteActionTooltip}
              />
            }
          >
            <Trash2 />
            <span className='sr-only'>{t('Delete selected channels')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{deleteActionTooltip}</p>
          </TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>

      {/* Set Tag Dialog */}
      <Dialog
        open={showTagDialog}
        onOpenChange={setShowTagDialog}
        title={t('Set Tag')}
        description={
          <>
            {t('Set a tag for')}
            {selectedIds.length}{' '}
            {t('selected channel(s). Leave empty to remove tag.')}
          </>
        }
        contentHeight='auto'
        bodyClassName='space-y-4'
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => {
                setShowTagDialog(false)
                setTagValue('')
              }}
            >
              {t('Cancel')}
            </Button>
            <Button onClick={handleSetTag}>{t('Set Tag')}</Button>
          </>
        }
      >
        <div className='grid gap-4 py-4'>
          <div className='grid gap-2'>
            <Label htmlFor='tag'>{t('Tag')}</Label>
            <Input
              id='tag'
              placeholder={t('Enter tag name (optional)')}
              value={tagValue}
              onChange={(e) => setTagValue(e.target.value)}
            />
          </div>
        </div>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={showDeleteConfirm}
        onOpenChange={setShowDeleteConfirm}
        title={t('Delete Channels?')}
        description={
          <>
            {t('Are you sure you want to delete')}
            {selectedIds.length}{' '}
            {t('channel(s)? This action cannot be undone.')}
          </>
        }
        contentHeight='auto'
        footer={
          <>
            <Button
              variant='outline'
              onClick={() => setShowDeleteConfirm(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              variant='destructive'
              onClick={handleDeleteAll}
              disabled={!canEditSensitive || selectedIds.length === 0}
            >
              {t('Delete')}
            </Button>
          </>
        }
      >
        {' '}
      </Dialog>
    </>
  )
}
