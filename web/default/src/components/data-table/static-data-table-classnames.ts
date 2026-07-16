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

/**
 * 静态 DataTable 常用样式。
 *
 * 这些 className 用于本地数组表格，例如弹窗里的状态清单、导入预览、
 * 只读摘要列表等。它们不包含业务语义，只约束表格密度、对齐和截断边界。
 */
export const staticDataTableClassNames = {
  actionCell: 'w-auto max-w-none text-right',
  actionHeaderCell: 'w-auto max-w-none text-right',
  codeCell: 'font-mono text-sm',
  compactCell: 'py-2.5',
  compactHeaderCell:
    'text-muted-foreground py-2 text-[10px] font-medium tracking-wider uppercase',
  compactHeaderCellRight:
    'text-muted-foreground py-2 text-right text-[10px] font-medium tracking-wider uppercase',
  compactHeaderRow: 'hover:bg-transparent',
  compactMutedCell: 'text-muted-foreground py-2.5',
  compactMutedCodeCell: 'text-muted-foreground py-2.5 font-mono',
  compactMutedNumericCell: 'text-muted-foreground py-2.5 text-right font-mono',
  compactNumericCell: 'py-2.5 text-right font-mono',
  compactTable: 'text-sm',
  compactTopCell: 'py-2.5 align-top',
  compactTopNumericCell: 'py-2.5 text-right align-top font-mono',
  container: 'overflow-hidden rounded-md border',
  embeddedContainer: 'rounded-none border-0',
  mediumCell: 'font-medium',
  mutedCell: 'text-muted-foreground text-sm',
  mutedCodeCell: 'text-muted-foreground font-mono text-sm',
  mutedHeaderRow:
    '[background-color:var(--table-header)] hover:[background-color:var(--table-header-hover)]',
  sectionContainer: 'border-border/60 rounded-lg',
  topCell: 'py-2 align-top',
  topMutedCell: 'text-muted-foreground py-2 align-top',
  topNumericCell: 'py-2 text-right font-mono',
} as const
