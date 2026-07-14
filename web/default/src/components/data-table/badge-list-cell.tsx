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
import { cn } from '@/lib/utils'
import { StatusBadgeList } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

interface BadgeListCellProps {
  className?: string
  items: ReactNode[]
  max?: number
  tooltipClassName?: string
}

/**
 * DataTable 多 badge 单元格。
 *
 * 默认只展示前两个 badge，剩余项通过 `+N` 提示；当确实存在被折叠项时
 * 才挂载 tooltip，避免普通短列表多一层无意义浮层。
 */
export function BadgeListCell({
  className,
  items,
  max = 2,
  tooltipClassName,
}: BadgeListCellProps) {
  const content = (
    <StatusBadgeList
      className={className}
      items={items}
      max={max}
      renderItem={(item) => item}
    />
  )

  if (items.length <= max) {
    return content
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger render={<div className='max-w-full min-w-0' />}>
          {content}
        </TooltipTrigger>
        <TooltipContent
          side='top'
          className={cn(
            'border-border bg-popover max-h-48 max-w-[320px] overflow-y-auto p-2',
            tooltipClassName
          )}
        >
          <div className='flex flex-wrap gap-1'>{items}</div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
