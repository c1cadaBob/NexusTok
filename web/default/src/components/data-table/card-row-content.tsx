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
import {
  flexRender,
  type Cell,
  type Row,
  type Table,
} from '@tanstack/react-table'

type CardColumnMeta = {
  label?: string
  mobileTitle?: boolean
  mobileBadge?: boolean
  mobileHidden?: boolean
  mobileOrder?: number
}

function getCellMeta<TData>(
  cell: Cell<TData, unknown>
): CardColumnMeta | undefined {
  return cell.column.columnDef.meta as CardColumnMeta | undefined
}

function getCellLabel<TData>(cell: Cell<TData, unknown>): string | null {
  const meta = getCellMeta(cell)
  if (meta?.label) return meta.label
  const header = cell.column.columnDef.header
  return typeof header === 'string' ? header : null
}

function renderCellContent<TData>(cell: Cell<TData, unknown>) {
  const cellRenderer = cell.column.columnDef.cell
  if (cellRenderer) {
    return flexRender(cellRenderer, cell.getContext())
  }
  return cell.getValue() as ReactNode
}

function orderCardCells<TData>(
  cells: Cell<TData, unknown>[]
): Cell<TData, unknown>[] {
  return [...cells].sort((a, b) => {
    const aOrder = getCellMeta(a)?.mobileOrder
    const bOrder = getCellMeta(b)?.mobileOrder

    if (aOrder == null && bOrder == null) return 0
    if (aOrder == null) return 1
    if (bOrder == null) return -1
    return aOrder - bOrder
  })
}

export function tableHasCompactCardMeta<TData>(table: Table<TData>): boolean {
  return table.getVisibleLeafColumns().some((column) => {
    const meta = column.columnDef.meta as CardColumnMeta | undefined
    return Boolean(meta?.mobileTitle || meta?.mobileBadge)
  })
}

function CompactCardContent<TData>({ row }: { row: Row<TData> }) {
  const allCells = row
    .getVisibleCells()
    .filter((cell) => cell.column.id !== 'select')

  const titleCell = allCells.find((cell) => getCellMeta(cell)?.mobileTitle)
  const badgeCell = allCells.find((cell) => getCellMeta(cell)?.mobileBadge)
  const actionsCell = allCells.find((cell) => cell.column.id === 'actions')

  const fieldCells = orderCardCells(
    allCells.filter(
      (cell) =>
        cell !== titleCell &&
        cell !== badgeCell &&
        cell !== actionsCell &&
        !getCellMeta(cell)?.mobileHidden
    )
  )

  return (
    <>
      <div className='flex items-center justify-between gap-2'>
        {titleCell && (
          <div className='min-w-0 flex-1 overflow-hidden text-sm font-medium [&_[data-slot=status-badge]]:max-w-full [&_[data-slot=status-badge]]:whitespace-normal'>
            {renderCellContent(titleCell)}
          </div>
        )}
        {badgeCell && (
          <div className='shrink-0'>{renderCellContent(badgeCell)}</div>
        )}
      </div>

      {fieldCells.length > 0 && (
        <div className='mt-1.5 grid grid-cols-2 gap-x-3 gap-y-1.5'>
          {fieldCells.map((cell) => {
            const label = getCellLabel(cell)
            return (
              <div key={cell.id} className='min-w-0 flex-1 overflow-hidden'>
                {label && (
                  <div className='text-muted-foreground mb-0.5 text-[10px] leading-none select-none'>
                    {label}
                  </div>
                )}
                <div className='min-w-0 overflow-hidden text-xs [&_:is([data-slot=badge-cell],[data-slot=provider-badge],[data-slot=status-badge])]:ml-0'>
                  {renderCellContent(cell) ?? '-'}
                </div>
              </div>
            )
          })}
        </div>
      )}

      {actionsCell && (
        <div className='mt-1 -mb-0.5 flex justify-end'>
          {renderCellContent(actionsCell)}
        </div>
      )}
    </>
  )
}

function FallbackCardContent<TData>({ row }: { row: Row<TData> }) {
  const allCells = row
    .getVisibleCells()
    .filter((cell) => cell.column.id !== 'select')

  const actionsCell = allCells.find((cell) => cell.column.id === 'actions')
  const contentCells = orderCardCells(
    allCells.filter(
      (cell) => cell.column.id !== 'actions' && !getCellMeta(cell)?.mobileHidden
    )
  )

  return (
    <>
      {contentCells.map((cell) => {
        const label = getCellLabel(cell)
        const content = renderCellContent(cell)

        if (!label) {
          return (
            <div key={cell.id} className='flex justify-end overflow-hidden'>
              {content}
            </div>
          )
        }

        return (
          <div
            key={cell.id}
            className='flex items-start justify-between gap-2 overflow-hidden'
          >
            <span className='text-muted-foreground shrink-0 text-[10px] font-medium select-none'>
              {label}
            </span>
            <div className='flex min-w-0 flex-1 items-center justify-end overflow-hidden text-xs'>
              {content ?? '-'}
            </div>
          </div>
        )
      })}

      {actionsCell && (
        <div className='-mb-0.5 flex justify-end pt-0.5'>
          {renderCellContent(actionsCell)}
        </div>
      )}
    </>
  )
}

export function CardRowContent<TData>(props: {
  row: Row<TData>
  compact: boolean
}) {
  return props.compact ? (
    <CompactCardContent row={props.row} />
  ) : (
    <FallbackCardContent row={props.row} />
  )
}
