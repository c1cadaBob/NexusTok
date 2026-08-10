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
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useIsFetching, useQuery } from '@tanstack/react-query'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import {
  type ColumnDef,
  getCoreRowModel,
  getPaginationRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { formatTimestampToDate } from '@/lib/format'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { DataTableColumnHeader, DataTablePage } from '@/components/data-table'
import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import { CompactDateTimeRangePicker } from '@/features/usage-logs/components/compact-date-time-range-picker'
import { DetailsDialog } from '@/features/usage-logs/components/dialogs/details-dialog'
import {
  LogsFilterField,
  LogsFilterInput,
  LogsFilterToolbar,
} from '@/features/usage-logs/components/logs-filter-toolbar'
import { LOG_TYPE_ENUM } from '@/features/usage-logs/constants'
import type { UsageLog } from '@/features/usage-logs/data/schema'
import { parseLogOther, renderAuditContent } from '@/features/usage-logs/lib'
import {
  getDefaultTimeRange,
  getLogTypeConfig,
} from '@/features/usage-logs/lib/utils'
import { getAuditLogs } from '../api'

const route = getRouteApi('/_authenticated/audit-logs/')

const EMPTY_AUDIT_LOGS = {
  items: [] as UsageLog[],
  total: 0,
}

type AuditLogFilters = {
  startTime?: Date
  endTime?: Date
  username?: string
  requestId?: string
}

type AuditLogDraft = {
  sourceKey: string
  filters: AuditLogFilters
}

function buildSourceKey(values: {
  startTime?: unknown
  endTime?: unknown
  username?: unknown
  requestId?: unknown
}) {
  return [values.startTime, values.endTime, values.username, values.requestId]
    .map((value) => String(value ?? ''))
    .join('\u001f')
}

function toTimestamp(date?: Date): number | undefined {
  return date ? Math.floor(date.getTime() / 1000) : undefined
}

function useAuditLogsColumns(): ColumnDef<UsageLog>[] {
  const { t } = useTranslation()

  return [
    {
      accessorKey: 'created_at',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Time')} />
      ),
      cell: ({ row }) => (
        <span className='font-mono text-xs tabular-nums'>
          {formatTimestampToDate(row.getValue('created_at') as number)}
        </span>
      ),
      meta: { label: t('Time') },
    },
    {
      id: 'event_type',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Type')} />
      ),
      cell: ({ row }) => {
        const config = getLogTypeConfig(row.original.type)
        return (
          <StatusBadge
            label={t(config.label)}
            variant={config.color as StatusBadgeProps['variant']}
            size='sm'
            copyable={false}
          />
        )
      },
      meta: { label: t('Type'), mobileBadge: true },
    },
    {
      accessorKey: 'username',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('User')} />
      ),
      cell: ({ row }) => (
        <span className='max-w-[160px] truncate text-sm'>
          {row.original.username || '-'}
        </span>
      ),
      meta: { label: t('User') },
    },
    {
      id: 'operation',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Operation')} />
      ),
      cell: ({ row }) => {
        const log = row.original
        const text =
          renderAuditContent(parseLogOther(log.other), t) || log.content || '-'
        return (
          <span className='block max-w-[360px] truncate text-sm'>{text}</span>
        )
      },
      meta: { label: t('Operation'), mobileTitle: true },
    },
    {
      accessorKey: 'ip',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('IP Address')} />
      ),
      cell: ({ row }) => (
        <span className='font-mono text-xs tabular-nums'>
          {row.original.ip || '-'}
        </span>
      ),
      meta: { label: t('IP Address') },
    },
    {
      id: 'result',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Result')} />
      ),
      cell: ({ row }) => {
        const log = row.original
        const other = parseLogOther(log.other)
        const success =
          log.type === LOG_TYPE_ENUM.LOGIN ? true : other?.audit_info?.success

        if (success === true) {
          return (
            <StatusBadge
              label={t('Success')}
              variant='success'
              size='sm'
              copyable={false}
            />
          )
        }
        if (success === false) {
          return (
            <StatusBadge
              label={t('Failed')}
              variant='danger'
              size='sm'
              copyable={false}
            />
          )
        }
        return <span className='text-muted-foreground text-xs'>-</span>
      },
      meta: { label: t('Result') },
    },
    {
      accessorKey: 'content',
      header: t('Details'),
      cell: function DetailsCell({ row }) {
        const [open, setOpen] = useState(false)

        return (
          <>
            <button
              type='button'
              className='text-muted-foreground hover:text-foreground text-xs underline-offset-2 hover:underline'
              onClick={() => setOpen(true)}
            >
              {t('Details')}
            </button>
            <DetailsDialog
              log={row.original}
              isAdmin
              open={open}
              onOpenChange={setOpen}
            />
          </>
        )
      },
      meta: { label: t('Details') },
    },
  ]
}

export function AuditLogsTable() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const searchParams = route.useSearch()
  const isFetchingAuditLogs = useIsFetching({ queryKey: ['audit-logs'] })

  const searchState = useMemo<AuditLogDraft>(() => {
    const { start, end } = getDefaultTimeRange()
    return {
      sourceKey: buildSourceKey({
        startTime: searchParams.startTime,
        endTime: searchParams.endTime,
        username: searchParams.username,
        requestId: searchParams.requestId,
      }),
      filters: {
        startTime: searchParams.startTime
          ? new Date(searchParams.startTime)
          : start,
        endTime: searchParams.endTime ? new Date(searchParams.endTime) : end,
        username: searchParams.username || undefined,
        requestId: searchParams.requestId || undefined,
      },
    }
  }, [
    searchParams.endTime,
    searchParams.requestId,
    searchParams.startTime,
    searchParams.username,
  ])
  const [draft, setDraft] = useState<AuditLogDraft>(() => searchState)
  const activeDraft =
    draft.sourceKey === searchState.sourceKey ? draft : searchState
  const filters = activeDraft.filters

  const { pagination, onPaginationChange, ensurePageInRange } =
    useTableUrlState({
      search: searchParams,
      navigate: route.useNavigate(),
      pagination: { defaultPage: 1, defaultPageSize: 50 },
      globalFilter: { enabled: false },
    })

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [
      'audit-logs',
      pagination.pageIndex + 1,
      pagination.pageSize,
      searchParams,
      t,
    ],
    queryFn: async () => {
      const result = await getAuditLogs({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
        start_timestamp: toTimestamp(
          searchParams.startTime
            ? new Date(searchParams.startTime)
            : getDefaultTimeRange().start
        ),
        end_timestamp: toTimestamp(
          searchParams.endTime
            ? new Date(searchParams.endTime)
            : getDefaultTimeRange().end
        ),
        username: searchParams.username || undefined,
        request_id: searchParams.requestId || undefined,
      })

      if (!result.success) {
        toast.error(result.message || t('Failed to load logs'))
        return EMPTY_AUDIT_LOGS
      }
      return result.data || EMPTY_AUDIT_LOGS
    },
    placeholderData: (previousData) => previousData,
  })

  const columns = useAuditLogsColumns()
  const table = useReactTable({
    data: data?.items || [],
    columns,
    state: { pagination },
    onPaginationChange,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    manualPagination: true,
    pageCount: Math.ceil((data?.total || 0) / pagination.pageSize),
  })

  const pageCount = table.getPageCount()
  useEffect(() => {
    ensurePageInRange(pageCount)
  }, [ensurePageInRange, pageCount])

  const updateFilter = useCallback(
    (field: keyof AuditLogFilters, value: Date | string | undefined) => {
      setDraft((current) => {
        const base =
          current.sourceKey === searchState.sourceKey ? current : searchState
        return {
          sourceKey: searchState.sourceKey,
          filters: { ...base.filters, [field]: value },
        }
      })
    },
    [searchState]
  )

  const handleApply = useCallback(() => {
    navigate({
      to: '/audit-logs',
      search: {
        page: 1,
        startTime: filters.startTime?.getTime(),
        endTime: filters.endTime?.getTime(),
        username: filters.username?.trim() || undefined,
        requestId: filters.requestId?.trim() || undefined,
      },
    })
  }, [filters, navigate])

  const handleReset = useCallback(() => {
    const { start, end } = getDefaultTimeRange()
    const nextFilters: AuditLogFilters = { startTime: start, endTime: end }
    const nextSearch = {
      startTime: start.getTime(),
      endTime: end.getTime(),
    }
    setDraft({
      sourceKey: buildSourceKey(nextSearch),
      filters: nextFilters,
    })
    navigate({
      to: '/audit-logs',
      search: { page: 1, ...nextSearch },
    })
  }, [navigate])

  const dateRangeFilter = (
    <LogsFilterField wide>
      <CompactDateTimeRangePicker
        start={filters.startTime}
        end={filters.endTime}
        onChange={({ start, end }) => {
          updateFilter('startTime', start)
          updateFilter('endTime', end)
        }}
      />
    </LogsFilterField>
  )
  const usernameFilter = (
    <LogsFilterField>
      <LogsFilterInput
        placeholder={t('Username')}
        value={filters.username || ''}
        onChange={(event) => updateFilter('username', event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') handleApply()
        }}
      />
    </LogsFilterField>
  )
  const requestIdFilter = (
    <LogsFilterField>
      <LogsFilterInput
        placeholder={t('Request ID')}
        value={filters.requestId || ''}
        onChange={(event) => updateFilter('requestId', event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') handleApply()
        }}
      />
    </LogsFilterField>
  )

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No Logs Found')}
      emptyDescription={t(
        'Administrative actions and successful sign-ins will appear here.'
      )}
      skeletonKeyPrefix='audit-log-skeleton'
      tableClassName='max-h-[calc(100dvh-13rem)] overflow-auto sm:max-h-[calc(100dvh-14rem)]'
      tableHeaderClassName='bg-muted/30 sticky top-0 z-10'
      toolbar={
        <LogsFilterToolbar
          table={table}
          primaryFilters={
            <>
              {dateRangeFilter}
              {usernameFilter}
              {requestIdFilter}
            </>
          }
          mobilePinnedFilters={dateRangeFilter}
          mobileFilters={
            <>
              {usernameFilter}
              {requestIdFilter}
            </>
          }
          mobileFilterCount={
            [filters.username, filters.requestId].filter(Boolean).length
          }
          hasActiveFilters={Boolean(filters.username || filters.requestId)}
          searchLoading={isFetchingAuditLogs > 0}
          onReset={handleReset}
          onSearch={handleApply}
        />
      }
    />
  )
}
