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
import { flexRender, type Row, type Table } from '@tanstack/react-table'
import { Database } from 'lucide-react'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'
import { GroupBadge } from '@/components/group-badge'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { CHANNEL_STATUS } from '../constants'
import { isTagAggregateRow, parseGroupsList } from '../lib'
import type { Channel } from '../types'

interface ChannelsMobileListProps {
  emptyDescription?: string
  emptyTitle?: string
  getRowClassName?: (row: Row<Channel>) => string | undefined
  isLoading?: boolean
  table: Table<Channel>
}

interface ChannelCardProps {
  className?: string
  isSelected: boolean
  row: Row<Channel>
}

function ChannelMobileSkeleton() {
  return (
    <div className='overflow-hidden rounded-lg border'>
      {[1, 2, 3].map((item) => (
        <div
          className='bg-card flex flex-col gap-3 border-b p-3 last:border-b-0'
          key={item}
        >
          <div className='flex items-center justify-between gap-3'>
            <Skeleton className='h-6 w-44 rounded-md' />
            <Skeleton className='size-8 rounded-md' />
          </div>
          <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-3'>
            <div className='flex min-w-0 flex-col gap-2'>
              <Skeleton className='h-4 w-20 rounded' />
              <Skeleton className='h-8 w-full rounded-md' />
            </div>
            <div className='grid grid-cols-2 gap-2'>
              <Skeleton className='h-8 w-16 rounded-md' />
              <Skeleton className='h-8 w-16 rounded-md' />
            </div>
          </div>
          <div className='flex gap-1.5'>
            <Skeleton className='h-5 w-16 rounded-md' />
            <Skeleton className='h-5 w-20 rounded-md' />
          </div>
        </div>
      ))}
    </div>
  )
}

function renderCell(row: Row<Channel>, id: string) {
  const cell = row.getAllCells().find((item) => item.column.id === id)
  if (!cell || !cell.column.columnDef.cell) return null
  return flexRender(cell.column.columnDef.cell, cell.getContext())
}

function ChannelCardComponent({
  className,
  isSelected,
  row,
}: ChannelCardProps) {
  const { t } = useTranslation()
  const channel = row.original
  const isTagRow = isTagAggregateRow(channel)
  const groups = parseGroupsList(channel.group ?? '')

  const selectCell = renderCell(row, 'select')
  const typeCell = renderCell(row, 'type')
  const nameCell = renderCell(row, 'name')
  const statusCell = renderCell(row, 'status')
  const actionsCell = renderCell(row, 'actions')
  const priorityCell = renderCell(row, 'priority')
  const weightCell = renderCell(row, 'weight')
  const balanceCell = renderCell(row, 'balance')
  const responseCell = renderCell(row, 'response_time')
  const testCell = renderCell(row, 'test_time')

  const labelClass = 'text-muted-foreground text-[11px] font-medium select-none'
  const showStatusBadge =
    isTagRow ||
    (channel.status !== CHANNEL_STATUS.ENABLED &&
      channel.status !== CHANNEL_STATUS.MANUAL_DISABLED)

  return (
    <div
      className={cn('bg-card flex flex-col gap-3 px-3 py-3', className)}
      data-state={isSelected ? 'selected' : undefined}
    >
      <div className='flex items-center justify-between gap-2'>
        <div className='flex min-w-0 flex-1 items-center gap-2'>
          {!isTagRow && selectCell && (
            <span className='shrink-0'>{selectCell}</span>
          )}
          <div className='min-w-0 overflow-hidden'>{typeCell}</div>
        </div>
        <div className='flex shrink-0 items-center gap-1.5'>
          {showStatusBadge && statusCell}
          {actionsCell}
        </div>
      </div>

      <div className='grid grid-cols-[minmax(0,1fr)_auto] items-start gap-3'>
        <div className='flex min-w-0 flex-col gap-3 overflow-hidden'>
          <div className='min-w-0'>
            {!isTagRow && (
              <div className={labelClass}>#{channel.id}</div>
            )}
            <div className='min-w-0 text-sm'>{nameCell}</div>
          </div>

          <div className='min-w-0'>
            <div className={cn('mb-1', labelClass)}>
              {t('Used / Remaining')}
            </div>
            <div className='min-w-0 overflow-hidden text-sm'>
              {balanceCell ?? <span className='text-muted-foreground'>-</span>}
            </div>
          </div>
        </div>

        <div className='grid shrink-0 grid-cols-[auto_auto] items-center gap-x-3 gap-y-1'>
          <span className={labelClass}>{t('Priority')}</span>
          <span className={labelClass}>{t('Weight')}</span>
          <div className='flex justify-start'>{priorityCell}</div>
          <div className='flex justify-start'>{weightCell}</div>
          <span className={cn('mt-2', labelClass)}>{t('Response')}</span>
          <span className={cn('mt-2', labelClass)}>{t('Last Tested')}</span>
          <div className='overflow-hidden text-sm'>
            {responseCell ?? <span className='text-muted-foreground'>-</span>}
          </div>
          <div className='overflow-hidden text-sm'>
            {testCell ?? <span className='text-muted-foreground'>-</span>}
          </div>
        </div>
      </div>

      <div className='min-w-0'>
        {groups.length > 0 ? (
          <div className='flex flex-wrap gap-1'>
            {groups.map((group) => (
              <GroupBadge group={group} key={group} size='sm' />
            ))}
          </div>
        ) : (
          <span className='text-muted-foreground text-sm'>-</span>
        )}
      </div>
    </div>
  )
}

const ChannelCard = memo(ChannelCardComponent)

export function ChannelsMobileList({
  emptyDescription,
  emptyTitle,
  getRowClassName,
  isLoading = false,
  table,
}: ChannelsMobileListProps) {
  const { t } = useTranslation()

  if (isLoading) {
    return <ChannelMobileSkeleton />
  }

  const rows = table.getRowModel().rows
  if (rows.length === 0) {
    return (
      <div className='rounded-lg border p-6'>
        <Empty className='border-none p-0'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <Database className='size-6' />
            </EmptyMedia>
            <EmptyTitle>{emptyTitle ?? t('No Channels Found')}</EmptyTitle>
            <EmptyDescription>
              {emptyDescription ??
                t(
                  'No channels available. Create your first channel to get started.'
                )}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      </div>
    )
  }

  return (
    <div className='divide-border overflow-hidden rounded-lg border'>
      {rows.map((row) => (
        <ChannelCard
          className={cn('border-b last:border-b-0', getRowClassName?.(row))}
          isSelected={row.getIsSelected()}
          key={row.id}
          row={row}
        />
      ))}
    </div>
  )
}
