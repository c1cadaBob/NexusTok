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

import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, RefreshCw, ServerCog } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ErrorState } from '@/components/error-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import dayjs from '@/lib/dayjs'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { listSystemInstances } from '../api'
import type { SystemInstance, SystemInstanceStatus } from '../types'

const INSTANCE_POLL_INTERVAL_MS = 30_000

const STATUS_BADGE_VARIANT: Record<
  SystemInstanceStatus,
  'default' | 'secondary'
> = {
  online: 'default',
  stale: 'secondary',
}

function statusDotClass(status: SystemInstanceStatus) {
  return cn(
    'size-1.5 rounded-full',
    status === 'online' ? 'bg-primary' : 'bg-muted-foreground'
  )
}

function roleLabel(instance: SystemInstance) {
  return instance.info?.role?.is_master ? 'master' : 'worker'
}

function roleDescriptionKey(instance: SystemInstance) {
  if (instance.info?.role?.is_master) {
    return 'Master instances run scheduled background tasks.'
  }
  return 'Worker instances do not run master-only background tasks.'
}

function getNodeName(instance: SystemInstance) {
  return instance.info?.node?.name || instance.node_name
}

function runtimeLabel(instance: SystemInstance) {
  const runtime = instance.info?.runtime
  const value = [runtime?.goos, runtime?.goarch].filter(Boolean).join('/')
  return value || '-'
}

function formatPercent(value?: number) {
  if (typeof value !== 'number' || Number.isNaN(value)) return '-'
  return `${new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 1,
  }).format(value)}%`
}

function formatBytes(bytes?: number): string {
  if (typeof bytes !== 'number' || Number.isNaN(bytes)) return '-'
  if (bytes === 0) return '0 B'
  if (bytes < 0) return `-${formatBytes(-bytes)}`

  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1
  )
  const value = bytes / 1024 ** index
  return `${new Intl.NumberFormat(undefined, {
    maximumFractionDigits: index === 0 ? 0 : 1,
  }).format(value)} ${units[index]}`
}

function formatRelativeTimestamp(timestamp?: number) {
  if (!timestamp || timestamp <= 0) return '-'
  return dayjs(timestamp * 1000).fromNow()
}

function ringColorClass(percent: number | null) {
  if (percent === null) return 'text-muted-foreground/40'
  if (percent >= 90) return 'text-destructive'
  if (percent >= 70) return 'text-muted-foreground'
  return 'text-primary'
}

type RingProgressProps = {
  percent: number | null
}

function RingProgress({ percent }: RingProgressProps) {
  const size = 22
  const stroke = 2.5
  const radius = (size - stroke) / 2
  const circumference = 2 * Math.PI * radius
  const offset =
    percent === null ? circumference : circumference - (percent / 100) * circumference

  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      className='shrink-0 -rotate-90'
      aria-hidden='true'
    >
      <circle
        cx={size / 2}
        cy={size / 2}
        r={radius}
        fill='none'
        strokeWidth={stroke}
        stroke='currentColor'
        className='text-muted'
      />
      <circle
        cx={size / 2}
        cy={size / 2}
        r={radius}
        fill='none'
        strokeWidth={stroke}
        strokeLinecap='round'
        stroke='currentColor'
        strokeDasharray={circumference}
        strokeDashoffset={offset}
        className={cn(
          'transition-[stroke-dashoffset] duration-500',
          ringColorClass(percent)
        )}
      />
    </svg>
  )
}

type ResourceCellProps = {
  value?: number
  tooltip?: ReactNode
}

function ResourceCell(props: ResourceCellProps) {
  const percent =
    typeof props.value === 'number' && !Number.isNaN(props.value)
      ? Math.max(0, Math.min(100, props.value))
      : null
  const content = (
    <div className='flex items-center gap-2'>
      <RingProgress percent={percent} />
      <span className='font-mono text-[11px] tabular-nums'>
        {formatPercent(props.value)}
      </span>
    </div>
  )

  if (!props.tooltip) return content

  return (
    <TooltipProvider delay={100}>
      <Tooltip>
        <TooltipTrigger className='block rounded-sm text-left focus-visible:ring-2 focus-visible:outline-none'>
          {content}
        </TooltipTrigger>
        <TooltipContent className='max-w-80'>{props.tooltip}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

type SystemInstancesTableProps = {
  instances: SystemInstance[]
}

function SystemInstancesTable(props: SystemInstancesTableProps) {
  const { t } = useTranslation()

  return (
    <div className='rounded-md border'>
      <Table className='min-w-[1140px]'>
        <TableHeader>
          <TableRow className='bg-muted/40 hover:bg-muted/40'>
            <TableHead className='h-9 min-w-[240px] px-4 text-xs'>
              {t('Instances')}
            </TableHead>
            <TableHead className='h-9 w-[110px] text-xs'>
              {t('Status')}
            </TableHead>
            <TableHead className='h-9 w-[100px] text-xs'>{t('Role')}</TableHead>
            <TableHead className='h-9 w-[96px] text-xs'>{t('CPU')}</TableHead>
            <TableHead className='h-9 w-[96px] text-xs'>
              {t('Memory')}
            </TableHead>
            <TableHead className='h-9 w-[96px] text-xs'>
              {t('Storage')}
            </TableHead>
            <TableHead className='h-9 w-[100px] text-xs'>
              {t('Version')}
            </TableHead>
            <TableHead className='h-9 w-[140px] text-xs'>
              {t('Runtime')}
            </TableHead>
            <TableHead className='h-9 w-[170px] text-xs'>
              {t('Started')}
            </TableHead>
            <TableHead className='h-9 w-[170px] pr-4 text-xs'>
              {t('Last Seen')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.instances.map((instance) => {
            const shouldConfigure =
              instance.info?.node?.should_configure_manually === true
            const resources = instance.info?.resources
            const storage = resources?.storage

            return (
              <TableRow key={instance.node_name} className='hover:bg-muted/30'>
                <TableCell className='px-4 py-2.5 align-middle'>
                  <div className='flex min-w-0 items-center gap-2'>
                    <span
                      className={statusDotClass(instance.status)}
                      aria-hidden='true'
                    />
                    <div className='min-w-0'>
                      <div className='flex min-w-0 items-center gap-1.5'>
                        <span className='truncate text-sm font-medium'>
                          {getNodeName(instance)}
                        </span>
                        {shouldConfigure && (
                          <Popover>
                            <PopoverTrigger
                              className='inline-flex shrink-0 rounded-full focus-visible:ring-2 focus-visible:outline-none'
                              aria-label={t('Configure NODE_NAME')}
                            >
                              <Badge variant='outline'>
                                <AlertTriangle aria-hidden='true' />
                              </Badge>
                            </PopoverTrigger>
                            <PopoverContent align='start' className='w-80'>
                              <PopoverHeader>
                                <PopoverTitle>
                                  {t('Configure NODE_NAME')}
                                </PopoverTitle>
                                <PopoverDescription>
                                  {t(
                                    'This instance is using an automatic hostname. Set NODE_NAME to a stable unique value for multi-instance management.'
                                  )}
                                </PopoverDescription>
                              </PopoverHeader>
                              <div className='flex flex-col gap-2 text-xs'>
                                <div>
                                  <div className='mb-1 font-medium'>
                                    {t('Example')}
                                  </div>
                                  <code className='bg-muted block rounded-md px-2 py-1.5 font-mono text-[11px] break-all'>
                                    NODE_NAME=nexustok-master-1
                                  </code>
                                </div>
                                <p className='text-muted-foreground'>
                                  {t(
                                    'Use a different stable value for each instance, then restart the service.'
                                  )}
                                </p>
                              </div>
                            </PopoverContent>
                          </Popover>
                        )}
                      </div>
                      <div className='text-muted-foreground truncate font-mono text-[11px]'>
                        {instance.info?.host?.hostname || '-'}
                      </div>
                    </div>
                  </div>
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <Badge
                    variant={STATUS_BADGE_VARIANT[instance.status]}
                    className='gap-1.5'
                  >
                    <span
                      className={statusDotClass(instance.status)}
                      aria-hidden='true'
                    />
                    {t(instance.status)}
                  </Badge>
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <TooltipProvider delay={100}>
                    <Tooltip>
                      <TooltipTrigger className='inline-flex shrink-0 rounded-full focus-visible:ring-2 focus-visible:outline-none'>
                        <Badge variant='outline'>{t(roleLabel(instance))}</Badge>
                      </TooltipTrigger>
                      <TooltipContent>
                        {t(roleDescriptionKey(instance))}
                      </TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <ResourceCell value={resources?.cpu?.usage_percent} />
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <ResourceCell value={resources?.memory?.usage_percent} />
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <ResourceCell
                    value={storage?.used_percent}
                    tooltip={
                      storage ? (
                        <div className='grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs'>
                          <span className='text-muted-foreground'>
                            {t('Used')}
                          </span>
                          <span className='font-mono'>
                            {formatBytes(storage.used_bytes)}
                          </span>
                          <span className='text-muted-foreground'>
                            {t('Free')}
                          </span>
                          <span className='font-mono'>
                            {formatBytes(storage.free_bytes)}
                          </span>
                          <span className='text-muted-foreground'>
                            {t('Total')}
                          </span>
                          <span className='font-mono'>
                            {formatBytes(storage.total_bytes)}
                          </span>
                        </div>
                      ) : undefined
                    }
                  />
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <div className='truncate font-mono text-xs'>
                    {instance.info?.runtime?.version || '-'}
                  </div>
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <div className='truncate font-mono text-xs'>
                    {runtimeLabel(instance)}
                  </div>
                </TableCell>
                <TableCell className='text-muted-foreground py-2.5 align-middle text-xs'>
                  {formatTimestampToDate(instance.started_at)}
                </TableCell>
                <TableCell
                  className='text-muted-foreground py-2.5 pr-4 align-middle text-xs'
                  title={formatTimestampToDate(instance.last_seen_at)}
                >
                  {formatRelativeTimestamp(instance.last_seen_at)}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}

export function SystemInstancesPanel() {
  const { t } = useTranslation()
  const instancesQuery = useQuery({
    queryKey: ['system-info', 'instances'],
    queryFn: async () => {
      const res = await listSystemInstances()
      if (!res.success || !Array.isArray(res.data)) {
        throw new Error(res.message || t('We could not load instances.'))
      }
      return res.data
    },
    staleTime: INSTANCE_POLL_INTERVAL_MS,
    retry: false,
    refetchInterval: INSTANCE_POLL_INTERVAL_MS,
  })

  const instances = instancesQuery.data ?? []
  const loading = instancesQuery.isLoading
  const refreshing = instancesQuery.isFetching && !instancesQuery.isLoading

  return (
    <Card size='sm'>
      <CardHeader className='border-b'>
        <div className='flex items-start gap-2'>
          <span className='bg-muted text-muted-foreground inline-flex size-7 items-center justify-center rounded-md'>
            <ServerCog className='size-4' aria-hidden='true' />
          </span>
          <div className='min-w-0'>
            <CardTitle>{t('Instances')}</CardTitle>
            <CardDescription>
              {t(
                'Nodes reporting from this deployment and their latest heartbeat.'
              )}
            </CardDescription>
          </div>
        </div>
        <CardAction>
          <div className='flex shrink-0 items-center gap-3'>
            <span
              className='text-muted-foreground hidden text-xs sm:inline'
              aria-live='polite'
            >
              {t('Auto-refreshing every {{seconds}}s', {
                seconds: INSTANCE_POLL_INTERVAL_MS / 1000,
              })}
            </span>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => void instancesQuery.refetch()}
              disabled={instancesQuery.isFetching}
              aria-label={t('Refresh')}
            >
              <RefreshCw
                data-icon='inline-start'
                className={cn(refreshing && 'animate-spin')}
                aria-hidden='true'
              />
              {refreshing ? t('Refreshing...') : t('Refresh')}
            </Button>
          </div>
        </CardAction>
      </CardHeader>

      <CardContent aria-busy={instancesQuery.isFetching} className='pt-0'>
        {loading ? (
          <div className='flex flex-col gap-2 py-4'>
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className='h-9 w-full rounded-md' />
            ))}
          </div>
        ) : instancesQuery.isError ? (
          <ErrorState
            title={t('We could not load instances.')}
            description={
              instancesQuery.error instanceof Error
                ? instancesQuery.error.message
                : undefined
            }
            onRetry={() => {
              void instancesQuery.refetch()
            }}
            className='min-h-[220px]'
          />
        ) : instances.length === 0 ? (
          <Empty className='min-h-[220px] border-0'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <ServerCog aria-hidden='true' />
              </EmptyMedia>
              <EmptyTitle>{t('No instances have reported yet.')}</EmptyTitle>
              <EmptyDescription>
                {t(
                  'The current node will appear after its first successful heartbeat.'
                )}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <div className='py-4'>
            <SystemInstancesTable instances={instances} />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
