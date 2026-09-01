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
import { Fragment, useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import {
  flexRender,
  type OnChangeFn,
  type SortingState,
  type Row,
} from '@tanstack/react-table'
import { useMediaQuery } from '@/hooks'
import { Eye, EyeOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { TableCell, TableRow } from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
  useDebouncedColumnFilter,
  useDataTable,
} from '@/components/data-table'
import { getChannels, searchChannels, getGroups } from '../api'
import {
  DEFAULT_PAGE_SIZE,
  CHANNEL_STATUS,
  CHANNEL_STATUS_OPTIONS,
} from '../constants'
import {
  channelsQueryKeys,
  aggregateChannelsByTag,
  isTagAggregateRow,
  getChannelTypeLabel,
} from '../lib'
import type { Channel, ChannelSortBy } from '../types'
import { ChannelAccountInlinePanel } from './channel-account-inline-panel'
import { ChannelCard, ChannelsMobileList } from './channel-card'
import { ChannelTypeIcon } from './channel-type-icon'
import { useChannelsColumns } from './channels-columns'
import { useChannels } from './channels-provider'
import { DataTableBulkActions } from './data-table-bulk-actions'
import { MinimumRatioModelFilterButton } from './minimum-ratio-model-selector'

const route = getRouteApi('/_authenticated/channels/')

const CHANNEL_SORTABLE_COLUMNS = new Set<ChannelSortBy>([
  'id',
  'name',
  'priority',
  'balance',
  'response_time',
  'test_time',
  'minimum_ratio',
])

const EMPTY_FILTER_VALUES: string[] = []
const CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY = 'channels-column-visibility'
const CHANNELS_VIEW_MODE_STORAGE_KEY = 'channels-table-view-mode'
const CHANNELS_SORTING_STORAGE_KEY = 'channels-table-sorting'
const CHANNELS_MINIMUM_RATIO_MODEL_STORAGE_KEY =
  'channels-minimum-ratio-model'
const CHANNELS_INITIAL_COLUMN_VISIBILITY = {
  models: false,
  tag: false,
}
const SENSITIVE_MASK = '••••'

function isDisabledChannelRow(channel: Channel) {
  return (
    !isTagAggregateRow(channel) && channel.status !== CHANNEL_STATUS.ENABLED
  )
}

function isAccountPoolChannel(channel: Channel) {
  return (
    !isTagAggregateRow(channel) &&
    (channel.channel_info?.credential_mode === 'account_pool' ||
      channel.channel_info?.account_pool_enabled === true ||
      (channel.channel_account_stats?.total ?? 0) > 0)
  )
}

function normalizeChannelSorting(value: SortingState): SortingState {
  const active = value[0]
  if (!active || !CHANNEL_SORTABLE_COLUMNS.has(active.id as ChannelSortBy)) {
    return []
  }
  return [
    {
      id: active.id,
      desc: Boolean(active.desc),
    },
  ]
}

function loadSavedChannelSorting(): SortingState {
  if (typeof window === 'undefined') return []
  try {
    const raw = window.localStorage.getItem(CHANNELS_SORTING_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw) as unknown
    if (!Array.isArray(parsed) || parsed.length === 0) return []
    const active = parsed[0] as { id?: unknown; desc?: unknown }
    if (!active || typeof active.id !== 'string') return []
    if (!CHANNEL_SORTABLE_COLUMNS.has(active.id as ChannelSortBy)) return []
    if (typeof active.desc !== 'boolean') return []
    return [{ id: active.id, desc: active.desc }]
  } catch {
    return []
  }
}

function loadSavedMinimumRatioModel(): string {
  if (typeof window === 'undefined') return ''
  try {
    return (
      window.localStorage
        .getItem(CHANNELS_MINIMUM_RATIO_MODEL_STORAGE_KEY)
        ?.trim() ?? ''
    )
  } catch {
    return ''
  }
}

function persistMinimumRatioModel(model: string) {
  if (typeof window === 'undefined') return
  try {
    const nextModel = model.trim()
    if (nextModel) {
      window.localStorage.setItem(
        CHANNELS_MINIMUM_RATIO_MODEL_STORAGE_KEY,
        nextModel
      )
    } else {
      window.localStorage.removeItem(CHANNELS_MINIMUM_RATIO_MODEL_STORAGE_KEY)
    }
  } catch {
    // 忽略本地存储失败，URL 中的选择仍会继续生效。
  }
}

function hasMinimumRatioModelSearchParam(): boolean {
  if (typeof window === 'undefined') return false
  return new URLSearchParams(window.location.search).has('minimumRatioModel')
}

export function ChannelsTable() {
  const { t } = useTranslation()
  const routeSearch = route.useSearch()
  const navigate = route.useNavigate()
  const routeMinimumRatioModel =
    typeof routeSearch.minimumRatioModel === 'string'
      ? routeSearch.minimumRatioModel.trim()
      : ''
  const minimumRatioModelSearchParamPresent =
    hasMinimumRatioModelSearchParam()
  const [storedMinimumRatioModel, setStoredMinimumRatioModel] = useState(
    () => loadSavedMinimumRatioModel()
  )
  const minimumRatioModel = minimumRatioModelSearchParamPresent
    ? routeMinimumRatioModel
    : storedMinimumRatioModel
  const {
    enableTagMode,
    idSort,
    batchMode,
    sensitiveVisible,
    setSensitiveVisible,
    isAccountPoolExpanded,
    setCurrentRow,
    setOpen,
  } = useChannels()
  const isMobile = useMediaQuery('(max-width: 640px)')

  // 表格排序状态由后端排序参数消费。
  const [sorting, setSorting] = useState<SortingState>(() =>
    loadSavedChannelSorting()
  )

  // URL 状态负责让筛选、搜索和分页可分享、可刷新恢复。
  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: routeSearch,
    navigate,
    pagination: {
      defaultPage: 1,
      defaultPageSize: isMobile ? 10 : DEFAULT_PAGE_SIZE,
    },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [
      { columnId: 'status', searchKey: 'status', type: 'array' },
      { columnId: 'type', searchKey: 'type', type: 'array' },
      { columnId: 'group', searchKey: 'group', type: 'array' },
      { columnId: 'model', searchKey: 'model', type: 'string' },
    ],
  })

  // 从列过滤状态中提取后端接口需要的查询参数。
  const statusFilter =
    (columnFilters.find((f) => f.id === 'status')?.value as
      | string[]
      | undefined) ?? EMPTY_FILTER_VALUES
  const typeFilter =
    (columnFilters.find((f) => f.id === 'type')?.value as
      | string[]
      | undefined) ?? EMPTY_FILTER_VALUES
  const groupFilter =
    (columnFilters.find((f) => f.id === 'group')?.value as
      | string[]
      | undefined) ?? EMPTY_FILTER_VALUES
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

  // 全局关键字或模型过滤存在时走搜索接口，否则走普通列表接口。
  const shouldSearch = Boolean(globalFilter?.trim() || modelFilter.trim())

  useEffect(() => {
    if (minimumRatioModelSearchParamPresent) {
      persistMinimumRatioModel(routeMinimumRatioModel)
      setStoredMinimumRatioModel(routeMinimumRatioModel)
      return
    }

    if (!storedMinimumRatioModel) return

    // URL 仍是可分享状态；没有 URL 参数时，用本地记忆补回，刷新和重新进入页面都能恢复排序上下文。
    navigate({
      replace: true,
      search: (prev) => ({
        ...(prev as Record<string, unknown>),
        minimumRatioModel: storedMinimumRatioModel,
      }),
    })
  }, [
    minimumRatioModelSearchParamPresent,
    navigate,
    routeMinimumRatioModel,
    storedMinimumRatioModel,
  ])

  const handleMinimumRatioModelChange = useCallback(
    (model: string) => {
      const nextModel = model.trim()
      persistMinimumRatioModel(nextModel)
      setStoredMinimumRatioModel(nextModel)
      navigate({
        search: (prev) => ({
          ...(prev as Record<string, unknown>),
          page: undefined,
          minimumRatioModel: nextModel || undefined,
        }),
      })
    },
    [navigate]
  )

  const sortParams = useMemo(() => {
    const activeSort = normalizeChannelSorting(sorting)[0]
    if (!activeSort) {
      return {}
    }

    return {
      sort_by: activeSort.id as ChannelSortBy,
      sort_order: activeSort.desc ? 'desc' : 'asc',
    } as const
  }, [sorting])

  const handleSortingChange: OnChangeFn<SortingState> = (updater) => {
    setSorting((previous) => {
      const next = normalizeChannelSorting(
        typeof updater === 'function' ? updater(previous) : updater
      )
      if (pagination.pageIndex > 0) {
        onPaginationChange({ ...pagination, pageIndex: 0 })
      }
      try {
        if (next.length === 0) {
          window.localStorage.removeItem(CHANNELS_SORTING_STORAGE_KEY)
        } else {
          window.localStorage.setItem(
            CHANNELS_SORTING_STORAGE_KEY,
            JSON.stringify(next)
          )
        }
      } catch {
        // 忽略本地存储失败，排序仍然可用，只是刷新后不会记忆。
      }
      return next
    })
  }

  // 获取分组列表，用于构造分组过滤器。
  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: getGroups,
  })

  const groupOptions = useMemo(
    () =>
      (groupsData?.data || []).map((g) => ({
        label: g,
        value: g,
      })),
    [groupsData]
  )

  // 获取渠道数据。
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
      minimum_ratio_model: minimumRatioModel || undefined,
      ...sortParams,
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
    }),
    queryFn: async () => {
      if (shouldSearch) {
        return searchChannels({
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
          minimum_ratio_model: minimumRatioModel || undefined,
          ...sortParams,
          p: pagination.pageIndex + 1,
          page_size: pagination.pageSize,
        })
      } else {
        return getChannels({
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
          minimum_ratio_model: minimumRatioModel || undefined,
          ...sortParams,
          p: pagination.pageIndex + 1,
          page_size: pagination.pageSize,
        })
      }
    },
    placeholderData: (previousData) => previousData,
  })

  // Tag 模式开启时把后端返回的普通渠道聚合成可展开行。
  const channels = useMemo(() => {
    const rawChannels = data?.data?.items || []

    if (enableTagMode && rawChannels.length > 0) {
      return aggregateChannelsByTag(rawChannels)
    }

    return rawChannels
  }, [data, enableTagMode])

  const totalCount = data?.data?.total || 0
  const typeCounts = data?.data?.type_counts
  const minimumRatioModels = data?.data?.minimum_ratio_models ?? []

  // 列定义会跟随批量模式决定是否注入选择列。
  const columns = useChannelsColumns({
    enableSelection: batchMode,
    minimumRatioModel,
    minimumRatioModels,
    onMinimumRatioModelChange: handleMinimumRatioModelChange,
  })

  // 公共 DataTable hook 统一管理列显隐、行选择、展开行和页码范围修正。
  const { table } = useDataTable({
    data: channels,
    columns,
    totalCount,
    sorting,
    columnFilters,
    initialColumnVisibility: CHANNELS_INITIAL_COLUMN_VISIBILITY,
    columnVisibilityStorageKey: CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY,
    pagination,
    globalFilter,
    enableRowSelection: batchMode
      ? (row: Row<Channel>) => !isTagAggregateRow(row.original)
      : false,
    onSortingChange: handleSortingChange,
    onColumnFiltersChange,
    onPaginationChange,
    onGlobalFilterChange,
    getSubRows: (row: Channel & { children?: Channel[] }) => row.children,
    manualPagination: true,
    manualSorting: true,
    manualFiltering: true,
    withExpandedRowModel: true,
    ensurePageInRange,
  })

  useEffect(() => {
    if (!batchMode) {
      table.resetRowSelection()
    }
  }, [batchMode, table])

  // 类型过滤只展示当前数据集中实际存在的渠道类型。
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
          iconNode: <ChannelTypeIcon type={item.type} size={16} />,
        }
      }),
    ]
  }, [t, typeCounts, typeFilter])

  const groupFilterOptions = useMemo(
    () => [
      { label: t('All Groups'), value: 'all' },
      ...groupOptions.map((option) => ({
        ...option,
        label: sensitiveVisible ? option.label : SENSITIVE_MASK,
      })),
    ],
    [groupOptions, sensitiveVisible, t]
  )

  const getChannelRowClassName = (row: Row<Channel>, isMobileRow: boolean) =>
    isDisabledChannelRow(row.original)
      ? isMobileRow
        ? DISABLED_ROW_MOBILE
        : DISABLED_ROW_DESKTOP
      : undefined

  const renderDesktopRow = (row: Row<Channel>) => {
    const channel = row.original
    const showInlineAccounts =
      isAccountPoolChannel(channel) && isAccountPoolExpanded(channel.id)
    const colSpan = row.getVisibleCells().length

    return (
      <Fragment key={row.id}>
        <TableRow
          data-state={row.getIsSelected() && 'selected'}
          className={getChannelRowClassName(row, false)}
        >
          {row.getVisibleCells().map((cell) => (
            <TableCell key={cell.id}>
              {flexRender(cell.column.columnDef.cell, cell.getContext())}
            </TableCell>
          ))}
        </TableRow>
        {showInlineAccounts && (
          <TableRow key={`${row.id}-accounts`} className='hover:bg-transparent'>
            <TableCell colSpan={colSpan} className='bg-muted/10 p-3'>
              <ChannelAccountInlinePanel
                channel={channel}
                sensitiveVisible={sensitiveVisible}
                onManage={() => {
                  setCurrentRow(channel)
                  setOpen('account-pool-manage')
                }}
              />
            </TableCell>
          </TableRow>
        )}
      </Fragment>
    )
  }

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
      renderCard={(row, { isSelected }) => {
        const channel = row.original
        const showInlineAccounts =
          isAccountPoolChannel(channel) && isAccountPoolExpanded(channel.id)

        return (
          <div className='flex min-w-0 flex-col gap-2'>
            <ChannelCard
              className='px-0 py-0'
              enableSelection={batchMode}
              isSelected={isSelected}
              row={row}
              sensitiveVisible={sensitiveVisible}
            />
            {showInlineAccounts && (
              <ChannelAccountInlinePanel
                channel={channel}
                sensitiveVisible={sensitiveVisible}
                onManage={() => {
                  setCurrentRow(channel)
                  setOpen('account-pool-manage')
                }}
              />
            )}
          </div>
        )
      }}
      cardGridClassName='grid grid-cols-1 gap-3 sm:gap-4 lg:grid-cols-3'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Filter by name, ID, or key...'),
        onReset: () => {
          resetModelFilterInput()
          handleMinimumRatioModelChange('')
        },
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
        additionalSearch: (
          <div className='flex w-full min-w-0 flex-col gap-2 sm:w-auto sm:flex-row'>
            <MinimumRatioModelFilterButton
              modelOptions={minimumRatioModels}
              selectedModel={minimumRatioModel}
              onModelChange={handleMinimumRatioModelChange}
            />
            <Input
              placeholder={t('Filter by model...')}
              value={modelFilterInput}
              onChange={onModelFilterInputChange}
              onCompositionStart={onModelFilterCompositionStart}
              onCompositionEnd={onModelFilterCompositionEnd}
              className='w-full sm:w-[150px] lg:w-[180px]'
            />
          </div>
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
      }}
      mobile={
        <ChannelsMobileList
          enableSelection={batchMode}
          sensitiveVisible={sensitiveVisible}
          table={table}
          isLoading={isLoading}
          emptyTitle={t('No Channels Found')}
          emptyDescription={t(
            'No channels available. Create your first channel to get started.'
          )}
          getRowClassName={(row) => getChannelRowClassName(row, true)}
        />
      }
      getRowClassName={(row, { isMobile }) =>
        getChannelRowClassName(row, isMobile)
      }
      renderRow={renderDesktopRow}
      bulkActions={batchMode ? <DataTableBulkActions table={table} /> : null}
    />
  )
}
