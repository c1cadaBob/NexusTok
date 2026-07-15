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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

type TruncatedCellProps = {
  cellClassName?: string
  children: ReactNode
  className?: string
  contentClassName?: string
  side?: 'top' | 'bottom' | 'left' | 'right'
  tooltipClassName?: string
  tooltipContent?: ReactNode
}

/**
 * DataTable 单行文本截断单元格。
 *
 * 该组件吸收 new-api 的表格 core cell 思路：列内容保持单行稳定宽度，
 * 同时通过 tooltip 暴露完整文本。它适用于名称、ID、短描述等表格字段；
 * 已有交互组件、徽标组或需要移动端 Popover 的文本仍应继续使用各自专用组件。
 */
export function TruncatedCell({
  cellClassName,
  children,
  className,
  contentClassName,
  side = 'top',
  tooltipClassName,
  tooltipContent,
}: TruncatedCellProps) {
  const content = tooltipContent ?? getTextContent(children)

  if (!content) {
    return (
      <div
        className={cn(
          'block max-w-full min-w-0 truncate',
          cellClassName,
          className
        )}
      >
        {children}
      </div>
    )
  }

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <div
            className={cn(
              'block max-w-full min-w-0 truncate',
              cellClassName,
              className
            )}
          />
        }
      >
        <div className={cn('truncate', contentClassName)}>{children}</div>
      </TooltipTrigger>
      <TooltipContent
        side={side}
        className={cn('max-w-xs break-all', tooltipClassName)}
      >
        {content}
      </TooltipContent>
    </Tooltip>
  )
}

function getTextContent(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(getTextContent).join('')
  return ''
}
