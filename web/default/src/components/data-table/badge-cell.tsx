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
import type { HTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

type BadgeCellProps = HTMLAttributes<HTMLDivElement>

/**
 * DataTable badge 单元格容器。
 *
 * 表格里的 `StatusBadge`、`GroupBadge`、`ProviderBadge` 等 badge 常会出现在
 * 固定列宽或可横向滚动的区域。该容器统一提供收缩边界，确保长 badge
 * 在单元格内部截断，而不是撑宽整张表格。
 */
export function BadgeCell({ className, ...props }: BadgeCellProps) {
  return (
    <div
      data-slot='badge-cell'
      className={cn(
        '-ml-1.5 flex max-w-full min-w-0 items-center gap-1 overflow-hidden [&_[data-slot=status-badge]]:max-w-full [&_[data-slot=status-badge]]:min-w-0 [&_[data-slot=status-badge]]:shrink [&_[data-slot=status-badge]]:overflow-hidden',
        className
      )}
      {...props}
    />
  )
}
