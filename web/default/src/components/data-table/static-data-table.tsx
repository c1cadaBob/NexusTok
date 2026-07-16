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
import { isValidElement, type ComponentProps, type Key, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { staticDataTableClassNames } from './static-data-table-classnames'
import { TruncatedCell } from './truncated-cell'

type StaticDataTableBaseProps = {
  className?: string
  containerProps?: Omit<ComponentProps<'div'>, 'className' | 'children'>
  tableClassName?: string
  tableProps?: Omit<ComponentProps<typeof Table>, 'className' | 'children'>
}

type StaticDataTableDataProps<TData = unknown> =
  StaticDataTableBaseProps & {
    columns: StaticDataTableColumn<TData>[]
    data: TData[]
    empty?: boolean
    emptyClassName?: string
    emptyContent?: ReactNode
    getRowClassName?: (row: TData, index: number) => string | undefined
    getRowKey?: (row: TData, index: number) => Key
    headerRowClassName?: string
    renderRow?: (row: TData, index: number) => ReactNode
  }

type StaticDataTableChildrenProps = StaticDataTableBaseProps & {
  children: ReactNode
  columns?: never
  data?: never
}

type StaticDataTableProps<TData = unknown> =
  | StaticDataTableDataProps<TData>
  | StaticDataTableChildrenProps

export type StaticDataTableColumn<TData = unknown> = {
  cell?: (row: TData, index: number) => ReactNode
  cellClassName?: string | ((row: TData, index: number) => string | undefined)
  className?: string
  header: ReactNode
  id: string
}

/**
 * 静态数组表格。
 *
 * 该组件用于弹窗预览、状态清单等“本地数组 + 固定列”的轻量场景。
 * 它不承载排序、筛选、选择或分页状态；需要这些能力时仍应使用
 * `DataTablePage` 和 TanStack Table。
 */
export function StaticDataTable<TData = unknown>(
  props: StaticDataTableProps<TData>
) {
  const { className, containerProps, tableClassName, tableProps } = props

  return (
    <div
      className={cn(staticDataTableClassNames.container, className)}
      {...containerProps}
    >
      <Table className={tableClassName} {...tableProps}>
        {props.columns !== undefined ? (
          <StaticDataTableWithColumns {...props} />
        ) : (
          props.children
        )}
      </Table>
    </div>
  )
}

function StaticDataTableWithColumns<TData>({
  columns,
  data,
  empty,
  emptyClassName,
  emptyContent,
  getRowClassName,
  getRowKey,
  headerRowClassName,
  renderRow,
}: StaticDataTableDataProps<TData>) {
  const isEmpty = empty ?? data.length === 0

  return (
    <>
      <TableHeader>
        <TableRow className={headerRowClassName}>
          {columns.map((column) => (
            <TableHead key={column.id} className={column.className}>
              {column.header}
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {isEmpty ? (
          <StaticDataTableEmptyRow
            colSpan={columns.length}
            className={emptyClassName}
          >
            {emptyContent}
          </StaticDataTableEmptyRow>
        ) : (
          data.map((row, index) => (
            <StaticDataTableRow
              key={getRowKey?.(row, index) ?? index}
              row={row}
              index={index}
              columns={columns}
              getRowClassName={getRowClassName}
              renderRow={renderRow}
            />
          ))
        )}
      </TableBody>
    </>
  )
}

type StaticDataTableRowProps<TData> = Pick<
  StaticDataTableDataProps<TData>,
  'columns' | 'getRowClassName' | 'renderRow'
> & {
  index: number
  row: TData
}

function StaticDataTableRow<TData>({
  columns,
  getRowClassName,
  index,
  renderRow,
  row,
}: StaticDataTableRowProps<TData>) {
  if (renderRow) {
    return <>{renderRow(row, index)}</>
  }

  return (
    <TableRow className={getRowClassName?.(row, index)}>
      {columns.map((column) => (
        <TableCell
          key={column.id}
          className={cn(
            'max-w-full min-w-0 overflow-hidden',
            getStaticCellClassName(column, row, index)
          )}
        >
          {renderStaticCellContent(column, row, index)}
        </TableCell>
      ))}
    </TableRow>
  )
}

function renderStaticCellContent<TData>(
  column: StaticDataTableColumn<TData>,
  row: TData,
  index: number
) {
  const content = column.cell?.(row, index)
  const textContent = getPrimitiveTextContent(content)

  if (!textContent) return content

  return <TruncatedCell tooltipContent={textContent}>{content}</TruncatedCell>
}

function getPrimitiveTextContent(content: ReactNode): string | null {
  if (typeof content === 'string' || typeof content === 'number') {
    return String(content)
  }

  if (
    isValidElement<{ children?: ReactNode }>(content) &&
    (typeof content.props.children === 'string' ||
      typeof content.props.children === 'number')
  ) {
    return String(content.props.children)
  }

  return null
}

function getStaticCellClassName<TData>(
  column: StaticDataTableColumn<TData>,
  row: TData,
  index: number
) {
  return typeof column.cellClassName === 'function'
    ? column.cellClassName(row, index)
    : column.cellClassName
}

type StaticDataTableEmptyRowProps = {
  children: ReactNode
  className?: string
  colSpan: number
}

function StaticDataTableEmptyRow({
  children,
  className,
  colSpan,
}: StaticDataTableEmptyRowProps) {
  return (
    <TableRow>
      <TableCell
        colSpan={colSpan}
        className={cn('h-24 text-center', className)}
      >
        {children}
      </TableCell>
    </TableRow>
  )
}
