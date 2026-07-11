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
import { useMemo } from 'react'
import type { Row, Table } from '@tanstack/react-table'
import { DatabaseIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { CardRowContent, tableHasCompactCardMeta } from './card-row-content'

export type DataTableCardHelpers = {
  compact: boolean
  isSelected: boolean
}

export interface DataTableCardGridProps<TData> {
  table: Table<TData>
  isLoading?: boolean
  emptyTitle?: string
  emptyDescription?: string
  emptyIcon?: ReactNode
  getRowKey?: (row: Row<TData>) => string | number
  getRowClassName?: (row: Row<TData>) => string | undefined
  renderCard?: (row: Row<TData>, helpers: DataTableCardHelpers) => ReactNode
  gridClassName?: string
  skeletonKeyPrefix?: string
}

const DEFAULT_GRID_CLASSNAME =
  'grid grid-cols-1 gap-3 sm:gap-4 md:grid-cols-2 xl:grid-cols-3'

function CardGridSkeleton(props: {
  gridClassName?: string
  keyPrefix?: string
}) {
  const prefix = props.keyPrefix ?? 'card-skeleton'
  return (
    <div className={props.gridClassName ?? DEFAULT_GRID_CLASSNAME}>
      {[1, 2, 3, 4, 5, 6].map((i) => (
        <div key={`${prefix}-${i}`} className='space-y-3 rounded-lg border p-3'>
          <div className='flex items-center justify-between gap-2'>
            <Skeleton className='h-4 w-32' />
            <Skeleton className='h-5 w-16 rounded-md' />
          </div>
          <div className='grid grid-cols-2 gap-x-3 gap-y-1.5'>
            {[1, 2, 3, 4].map((j) => (
              <div key={j}>
                <Skeleton className='mb-1 h-2 w-8' />
                <Skeleton className='h-4 w-full' />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

export function DataTableCardGrid<TData>(props: DataTableCardGridProps<TData>) {
  const { t } = useTranslation()
  const resolvedEmptyTitle = props.emptyTitle ?? t('No Data')
  const resolvedEmptyDescription =
    props.emptyDescription ?? t('No data available')
  const visibleColumns = props.table.getVisibleLeafColumns()
  const compact = useMemo(
    () => tableHasCompactCardMeta(props.table),
    // 这里只关心可见列集合变化，避免每行渲染时重复扫描整张表。
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [visibleColumns]
  )

  if (props.isLoading) {
    return (
      <CardGridSkeleton
        gridClassName={props.gridClassName}
        keyPrefix={props.skeletonKeyPrefix}
      />
    )
  }

  const rows = props.table.getRowModel().rows
  if (!rows || rows.length === 0) {
    return (
      <div className='rounded-lg border p-6'>
        <Empty className='border-none p-0'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              {props.emptyIcon ?? (
                <HugeiconsIcon icon={DatabaseIcon} strokeWidth={2} />
              )}
            </EmptyMedia>
            <EmptyTitle>{resolvedEmptyTitle}</EmptyTitle>
            <EmptyDescription>{resolvedEmptyDescription}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      </div>
    )
  }

  return (
    <div className={props.gridClassName ?? DEFAULT_GRID_CLASSNAME}>
      {rows.map((row) => {
        const key = props.getRowKey ? props.getRowKey(row) : row.id
        const isSelected = row.getIsSelected()
        return (
          <div
            key={key}
            data-slot='data-table-card'
            data-state={isSelected ? 'selected' : undefined}
            className={cn(
              'bg-card data-[state=selected]:border-primary/40 data-[state=selected]:bg-primary/5 rounded-lg border px-3 py-2.5 transition-colors',
              props.getRowClassName?.(row)
            )}
          >
            {props.renderCard ? (
              props.renderCard(row, { compact, isSelected })
            ) : (
              <CardRowContent row={row} compact={compact} />
            )}
          </div>
        )
      })}
    </div>
  )
}
