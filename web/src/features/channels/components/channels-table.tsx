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
import { useQueries, useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import type {
  ColumnFiltersState,
  ExpandedState,
  OnChangeFn,
  SortingState,
  Row,
  Updater,
} from '@tanstack/react-table'
import { Eye, EyeOff } from 'lucide-react'
import { Fragment, useState, useMemo, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
  DataTableRow,
  type DataTableRenderRowHelpers,
  useDebouncedColumnFilter,
  useDataTable,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { TableCell, TableRow } from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  getChannels,
  getGroups,
  getUpstreamKeys,
  getUpstreamSiteStatus,
  searchChannels,
} from '../api'
import {
  DEFAULT_PAGE_SIZE,
  CHANNEL_STATUS,
  CHANNEL_STATUS_OPTIONS,
} from '../constants'
import {
  channelsQueryKeys,
  aggregateChannelsByTag,
  filterUpstreamKeys,
  getChannelTableRowId,
  isTagAggregateRow,
  getChannelTypeLabel,
} from '../lib'
import type { Channel, ChannelSortBy, UpstreamKey } from '../types'
import { ChannelCard } from './channel-card'
import { ChannelTypeLogo } from './channel-type-badge'
import { useChannelsColumns } from './channels-columns'
import { useChannels } from './channels-provider'
import { DataTableBulkActions } from './data-table-bulk-actions'
import { UpstreamKeysSubTable } from './upstream-keys-subtable'

const route = getRouteApi('/_authenticated/channels/')
const CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY = 'channels:column-visibility'
const CHANNELS_COLUMN_SIZING_STORAGE_KEY = 'channels:column-sizing'
const CHANNELS_VIEW_MODE_STORAGE_KEY = 'channels:view-mode'
const CHANNELS_STATUS_FILTER_STORAGE_KEY = 'channel-status-filter'
const CHANNELS_UPSTREAM_EXPANDED_STORAGE_KEY = 'channels:upstream-expanded:v1'

const CHANNEL_SORTABLE_COLUMNS = new Set<ChannelSortBy>([
  'id',
  'name',
  'priority',
  'balance',
  'response_time',
  'test_time',
])

function isDisabledChannelRow(channel: Channel) {
  return (
    !isTagAggregateRow(channel) && channel.status !== CHANNEL_STATUS.ENABLED
  )
}

function getChannelColumnClassName(columnId: string) {
  if (columnId === 'select') {
    return 'text-center'
  }
  if (['name', 'models', 'group', 'tag'].includes(columnId)) {
    return 'text-left'
  }
  return 'text-center'
}

function resolveTableUpdater<TValue>(
  updater: Updater<TValue>,
  previous: TValue
): TValue {
  return typeof updater === 'function'
    ? (updater as (old: TValue) => TValue)(previous)
    : updater
}

function readUpstreamExpandedState(): ExpandedState {
  if (typeof window === 'undefined') {
    return {}
  }

  try {
    const raw = window.localStorage.getItem(
      CHANNELS_UPSTREAM_EXPANDED_STORAGE_KEY
    )
    if (!raw) {
      return {}
    }
    const ids = JSON.parse(raw) as unknown
    if (!Array.isArray(ids)) {
      return {}
    }
    return ids.reduce<Record<string, boolean>>((state, id) => {
      if (typeof id === 'number' && Number.isInteger(id) && id > 0) {
        state[`channel:${id}`] = true
      }
      return state
    }, {})
  } catch {
    return {}
  }
}

function persistUpstreamExpandedState(expanded: ExpandedState): void {
  if (typeof window === 'undefined' || expanded === true) {
    return
  }

  try {
    const ids = Object.entries(expanded)
      .filter(([, value]) => value)
      .map(([rowId]) => {
        const prefix = 'channel:'
        if (!rowId.startsWith(prefix)) {
          return null
        }
        const id = Number(rowId.slice(prefix.length))
        return Number.isInteger(id) && id > 0 ? id : null
      })
      .filter((id): id is number => id !== null)
    window.localStorage.setItem(
      CHANNELS_UPSTREAM_EXPANDED_STORAGE_KEY,
      JSON.stringify(ids)
    )
  } catch {
    // Storage can be unavailable; expansion still works for the current page.
  }
}

export function ChannelsTable() {
  const { t } = useTranslation()
  const {
    enableTagMode,
    idSort,
    batchMode,
    sensitiveVisible,
    setSensitiveVisible,
  } = useChannels()
  const isMobile = useMediaQuery('(max-width: 640px)')

  // Table state
  const [sorting, setSorting] = useState<SortingState>([])
  const [expanded, setExpanded] = useState<ExpandedState>(() =>
    readUpstreamExpandedState()
  )

  // URL state management
  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: {
      defaultPage: 1,
      defaultPageSize: isMobile ? 10 : DEFAULT_PAGE_SIZE,
    },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      {
        columnId: 'status',
        searchKey: 'status',
        type: 'array',
        deserialize: (value) => {
          if (value !== undefined) return value
          const stored = localStorage.getItem(
            CHANNELS_STATUS_FILTER_STORAGE_KEY
          )
          return stored === 'enabled' || stored === 'disabled' ? [stored] : []
        },
      },
      { columnId: 'type', searchKey: 'type', type: 'array' },
      { columnId: 'group', searchKey: 'group', type: 'array' },
      { columnId: 'model', searchKey: 'model', type: 'string' },
    ],
  })

  const handleColumnFiltersChange: OnChangeFn<ColumnFiltersState> = (
    updater
  ) => {
    onColumnFiltersChange((previous) => {
      const next = typeof updater === 'function' ? updater(previous) : updater
      const status = next.find((f) => f.id === 'status')?.value as
        | string[]
        | undefined
      localStorage.setItem(
        CHANNELS_STATUS_FILTER_STORAGE_KEY,
        status?.[0] ?? 'all'
      )
      return next
    })
  }

  // Extract filters from column filters
  const statusFilter = useMemo(
    () =>
      (columnFilters.find((f) => f.id === 'status')?.value as string[]) || [],
    [columnFilters]
  )
  const typeFilter = useMemo(
    () => (columnFilters.find((f) => f.id === 'type')?.value as string[]) || [],
    [columnFilters]
  )
  const groupFilter =
    (columnFilters.find((f) => f.id === 'group')?.value as string[]) || []
  const {
    value: modelFilter,
    inputValue: modelFilterInput,
    onChange: onModelFilterInputChange,
    onCompositionStart: onModelFilterCompositionStart,
    onCompositionEnd: onModelFilterCompositionEnd,
    resetInput: resetModelFilterInput,
  } = useDebouncedColumnFilter({
    columnFilters,
    columnId: 'model',
    onColumnFiltersChange,
  })

  // Determine whether to use search or regular list API
  const shouldSearch = Boolean(globalFilter?.trim() || modelFilter.trim())

  const sortParams = useMemo(() => {
    const activeSort = sorting[0]
    if (
      !activeSort ||
      !CHANNEL_SORTABLE_COLUMNS.has(activeSort.id as ChannelSortBy)
    ) {
      return {}
    }

    return {
      sort_by: activeSort.id as ChannelSortBy,
      sort_order: activeSort.desc ? 'desc' : 'asc',
    } as const
  }, [sorting])

  const handleSortingChange: OnChangeFn<SortingState> = (updater) => {
    setSorting((previous) => {
      const next = typeof updater === 'function' ? updater(previous) : updater
      if (pagination.pageIndex > 0) {
        onPaginationChange({ ...pagination, pageIndex: 0 })
      }
      return next
    })
  }

  const handleExpandedChange: OnChangeFn<ExpandedState> = useCallback(
    (updater) => {
      setExpanded((previous) => {
        const next = resolveTableUpdater(updater, previous)
        persistUpstreamExpandedState(next)
        return next
      })
    },
    []
  )

  // Fetch groups for filter
  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: async () => requireServerSuccess(await getGroups()),
  })

  const groupOptions = useMemo(
    () =>
      (groupsData?.data || []).map((g) => ({
        label: g,
        value: g,
      })),
    [groupsData]
  )

  // Fetch channels data
  // eslint-disable-next-line @tanstack/query/exhaustive-deps
  const { data, isLoading, isFetching } = useQuery({
    queryKey: channelsQueryKeys.list({
      keyword: globalFilter,
      model: modelFilter,
      group:
        groupFilter.length > 0 && !groupFilter.includes('all')
          ? groupFilter[0]
          : undefined,
      status:
        statusFilter.length > 0 && !statusFilter.includes('all')
          ? statusFilter[0]
          : undefined,
      type:
        typeFilter.length > 0 && !typeFilter.includes('all')
          ? Number(typeFilter[0])
          : undefined,
      tag_mode: enableTagMode,
      id_sort: idSort,
      ...sortParams,
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
    }),
    queryFn: async () => {
      if (shouldSearch) {
        return requireServerSuccess(
          await searchChannels({
            keyword: globalFilter,
            model: modelFilter,
            group:
              groupFilter.length > 0 && !groupFilter.includes('all')
                ? groupFilter[0]
                : undefined,
            status:
              statusFilter.length > 0 && !statusFilter.includes('all')
                ? statusFilter[0]
                : undefined,
            type:
              typeFilter.length > 0 && !typeFilter.includes('all')
                ? Number(typeFilter[0])
                : undefined,
            tag_mode: enableTagMode,
            id_sort: idSort,
            ...sortParams,
            p: pagination.pageIndex + 1,
            page_size: pagination.pageSize,
          })
        )
      } else {
        return requireServerSuccess(
          await getChannels({
            group:
              groupFilter.length > 0 && !groupFilter.includes('all')
                ? groupFilter[0]
                : undefined,
            status:
              statusFilter.length > 0 && !statusFilter.includes('all')
                ? statusFilter[0]
                : undefined,
            type:
              typeFilter.length > 0 && !typeFilter.includes('all')
                ? Number(typeFilter[0])
                : undefined,
            tag_mode: enableTagMode,
            id_sort: idSort,
            ...sortParams,
            p: pagination.pageIndex + 1,
            page_size: pagination.pageSize,
          })
        )
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const platformChannels = useMemo(
    () =>
      (data?.data?.items || []).filter(
        (channel) => channel.upstream_kind === 'platform_site'
      ),
    [data]
  )
  const upstreamKeyQueries = useQueries({
    queries: platformChannels.map((channel) => ({
      queryKey: ['upstream-keys', channel.id],
      queryFn: async () =>
        requireServerSuccess(await getUpstreamKeys(channel.id)),
      staleTime: 30_000,
    })),
  })
  const upstreamSiteStatusQueries = useQueries({
    queries: platformChannels.map((channel) => ({
      queryKey: ['upstream-site-status', channel.id],
      queryFn: async () =>
        requireServerSuccess(await getUpstreamSiteStatus(channel.id)),
      staleTime: 30_000,
    })),
  })

  const upstreamKeysByChannelId = useMemo(() => {
    const result = new Map<number, UpstreamKey[]>()
    platformChannels.forEach((channel, index) => {
      const items = upstreamKeyQueries[index]?.data?.data?.items || []
      result.set(channel.id, items)
    })
    return result
  }, [platformChannels, upstreamKeyQueries])
  const upstreamSiteStatusByChannelId = useMemo(() => {
    const result = new Map<number, Channel['upstream_site_status']>()
    platformChannels.forEach((channel, index) => {
      result.set(channel.id, upstreamSiteStatusQueries[index]?.data?.data)
    })
    return result
  }, [platformChannels, upstreamSiteStatusQueries])

  // Apply tag aggregation if tag mode is enabled
  const channels = useMemo(() => {
    const rawChannels = data?.data?.items || []
    const withUpstreamData = rawChannels.map((channel) => {
      if (channel.upstream_kind !== 'platform_site') {
        return channel
      }
      const upstreamKeys = filterUpstreamKeys(
        upstreamKeysByChannelId.get(channel.id) || [],
        {
          keyword: globalFilter,
          model: modelFilter,
          status: statusFilter,
        }
      )
      const upstreamSiteStatus = upstreamSiteStatusByChannelId.get(channel.id)
      return {
        ...channel,
        upstream_site_status: upstreamSiteStatus,
        upstream_keys: upstreamKeys,
        balance: upstreamSiteStatus?.balance ?? channel.balance,
        used_quota: upstreamSiteStatus?.used_quota ?? channel.used_quota,
      }
    })

    if (enableTagMode && withUpstreamData.length > 0) {
      return aggregateChannelsByTag(withUpstreamData)
    }

    return withUpstreamData
  }, [
    data,
    globalFilter,
    enableTagMode,
    modelFilter,
    statusFilter,
    upstreamKeysByChannelId,
    upstreamSiteStatusByChannelId,
  ])

  const totalCount = data?.data?.total || 0
  const typeCounts = data?.data?.type_counts

  // Columns configuration
  const columns = useChannelsColumns({ enableSelection: batchMode })

  // React Table instance
  const { table } = useDataTable({
    data: channels,
    columns,
    totalCount,
    sorting,
    initialColumnVisibility: {},
    columnVisibilityStorageKey: CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: isMobile
      ? false
      : CHANNELS_COLUMN_SIZING_STORAGE_KEY,
    columnFilters,
    expanded,
    pagination,
    globalFilter,
    enableRowSelection: batchMode
      ? (row: Row<Channel>) => !isTagAggregateRow(row.original)
      : false,
    onSortingChange: handleSortingChange,
    onColumnFiltersChange: handleColumnFiltersChange,
    onExpandedChange: handleExpandedChange,
    onPaginationChange,
    onGlobalFilterChange,
    getRowId: getChannelTableRowId,
    getRowCanExpand: (row) =>
      isTagAggregateRow(row.original) ||
      row.original.upstream_kind === 'platform_site',
    getSubRows: (row: Channel & { children?: Channel[] }) =>
      isTagAggregateRow(row) ? row.children : undefined,
    manualPagination: true,
    manualSorting: true,
    manualFiltering: true,
    withExpandedRowModel: true,
    enableColumnResizing: !isMobile,
    ensurePageInRange,
  })

  useEffect(() => {
    if (!batchMode) {
      table.resetRowSelection()
    }
  }, [batchMode, table])

  const getRowClassName = useCallback(
    (row: Row<Channel>, { isMobile }: { isMobile: boolean }) => {
      if (!isDisabledChannelRow(row.original)) {
        return undefined
      }
      if (isMobile) {
        return DISABLED_ROW_MOBILE
      }
      return DISABLED_ROW_DESKTOP
    },
    []
  )

  const renderTableRow = useCallback(
    (row: Row<Channel>, helpers: DataTableRenderRowHelpers) => {
      const channel = row.original
      const shouldRenderUpstreamKeys =
        !isTagAggregateRow(channel) &&
        channel.upstream_kind === 'platform_site' &&
        row.getIsExpanded()

      return (
        <Fragment key={row.id}>
          <DataTableRow
            row={row}
            className={getRowClassName(row, { isMobile: false })}
            getColumnClassName={(columnId) =>
              helpers.getCellClassName(columnId)
            }
            cellRenderColumns={columns}
          />
          {shouldRenderUpstreamKeys && (
            <TableRow className='w-full bg-muted/20 hover:bg-muted/20'>
              <TableCell
                colSpan={row.getVisibleCells().length}
                className='w-0 max-w-0 min-w-0 p-0 whitespace-normal'
              >
                <UpstreamKeysSubTable channel={channel} />
              </TableCell>
            </TableRow>
          )}
        </Fragment>
      )
    },
    [columns, getRowClassName]
  )

  // Prepare filter options from existing channel types only.
  const typeFilterOptions = useMemo(() => {
    const counts = typeCounts || {}
    const typeIds = Object.entries(counts)
      .map(([type, count]) => ({
        type: Number(type),
        count: Number(count) || 0,
      }))
      .filter((item) => item.type > 0 && item.count > 0)
      .sort((a, b) => {
        const labelA = t(getChannelTypeLabel(a.type))
        const labelB = t(getChannelTypeLabel(b.type))
        return labelA.localeCompare(labelB)
      })

    const selectedType = typeFilter.find((value) => value !== 'all')
    if (selectedType) {
      const selectedTypeId = Number(selectedType)
      const alreadyIncluded = typeIds.some(
        (item) => item.type === selectedTypeId
      )
      if (selectedTypeId > 0 && !alreadyIncluded) {
        typeIds.push({
          type: selectedTypeId,
          count: Number(counts[selectedType]) || 0,
        })
      }
    }

    const totalTypes = Object.values(counts).reduce(
      (sum, count) => sum + (Number(count) || 0),
      0
    )

    return [
      {
        label: 'All Types',
        value: 'all',
        count: totalTypes,
      },
      ...typeIds.map((item) => {
        return {
          label: getChannelTypeLabel(item.type),
          value: String(item.type),
          count: item.count,
          iconNode: <ChannelTypeLogo type={item.type} size={16} />,
        }
      }),
    ]
  }, [t, typeCounts, typeFilter])

  const groupFilterOptions = [
    { label: t('All Groups'), value: 'all' },
    ...groupOptions.map((option) => ({
      ...option,
      label: sensitiveVisible ? option.label : '••••',
    })),
  ]

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Channels Found')}
      emptyDescription={t(
        'No channels available. Create your first channel to get started.'
      )}
      skeletonKeyPrefix='channel-skeleton'
      enableCardView
      viewModeStorageKey={CHANNELS_VIEW_MODE_STORAGE_KEY}
      renderCard={(row, { isSelected }) => (
        <ChannelCard row={row} isSelected={isSelected} />
      )}
      cardGridClassName='grid grid-cols-1 gap-3 sm:gap-4 lg:grid-cols-3'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Filter by name, ID, or key...'),
        searchDebounceMs: 500,
        onReset: () => {
          resetModelFilterInput()
        },
        additionalSearch: (
          <Input
            placeholder={t('Filter by model...')}
            value={modelFilterInput}
            onChange={onModelFilterInputChange}
            onCompositionStart={onModelFilterCompositionStart}
            onCompositionEnd={onModelFilterCompositionEnd}
            className='w-full sm:w-[150px] lg:w-[180px]'
          />
        ),
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            options: [...CHANNEL_STATUS_OPTIONS],
            singleSelect: true,
          },
          {
            columnId: 'type',
            title: t('Type'),
            options: typeFilterOptions,
            singleSelect: true,
          },
          {
            columnId: 'group',
            title: t('Group'),
            options: groupFilterOptions,
            singleSelect: true,
          },
        ],
        preActions: (
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant='ghost'
                  size='icon'
                  onClick={() => setSensitiveVisible(!sensitiveVisible)}
                  aria-label={sensitiveVisible ? t('Hide') : t('Show')}
                  className='text-muted-foreground hover:text-foreground size-8'
                />
              }
            >
              {sensitiveVisible ? <Eye /> : <EyeOff />}
            </TooltipTrigger>
            <TooltipContent>
              {sensitiveVisible ? t('Hide') : t('Show')}
            </TooltipContent>
          </Tooltip>
        ),
      }}
      getRowClassName={getRowClassName}
      renderRow={renderTableRow}
      getColumnClassName={getChannelColumnClassName}
      bulkActions={batchMode ? <DataTableBulkActions table={table} /> : null}
    />
  )
}
