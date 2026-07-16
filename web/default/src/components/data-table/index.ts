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
export { DataTablePagination } from './pagination'
export { DataTableColumnHeader } from './column-header'
export { BadgeCell } from './badge-cell'
export { BadgeListCell } from './badge-list-cell'
export { TruncatedCell } from './truncated-cell'
export {
  StaticDataTable,
  type StaticDataTableColumn,
} from './static-data-table'
export { staticDataTableClassNames } from './static-data-table-classnames'
export { DataTableFacetedFilter } from './faceted-filter'
export { DataTableViewOptions } from './view-options'
export { DataTableToolbar } from './toolbar'
export { DataTableBulkActions } from './bulk-actions'
export {
  DataTableCardGrid,
  type DataTableCardGridProps,
  type DataTableCardHelpers,
} from './card-grid'
export { CardRowContent, tableHasCompactCardMeta } from './card-row-content'
export {
  DataTableViewModeToggle,
  type DataTableViewModeToggleProps,
} from './view-mode-toggle'
export {
  DATA_TABLE_VIEW_MODES,
  useDataTableViewMode,
  isDataTableViewMode,
  readDataTableViewMode,
  writeDataTableViewMode,
  type DataTableViewMode,
} from './use-data-table-view-mode'
export { useDataTable } from './use-data-table'
export { useDebouncedColumnFilter } from './use-debounced-column-filter'
export { TableSkeleton } from './table-skeleton'
export { TableEmpty } from './table-empty'
export { MobileCardList } from './mobile-card-list'
export { DataTablePage, type DataTablePageProps } from './data-table-page'

export const DISABLED_ROW_DESKTOP =
  'bg-muted/85 hover:bg-muted [&>td:first-child]:border-l-muted-foreground/35 [&>td:first-child]:border-l-4 [&>td:first-child]:pl-1'

export const DISABLED_ROW_MOBILE =
  'border-l-4 border-l-muted-foreground/35 bg-muted/85'
