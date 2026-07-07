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
import { ListChecks, RefreshCw } from 'lucide-react'
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
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { listSystemTasks } from '../api'
import type { SystemTask, SystemTaskStatus } from '../types'

const TASK_LIMIT = 20
const ACTIVE_TASK_POLL_INTERVAL_MS = 8_000

const STATUS_BADGE_VARIANT: Record<
  SystemTaskStatus,
  'default' | 'secondary' | 'destructive'
> = {
  pending: 'secondary',
  running: 'default',
  succeeded: 'secondary',
  failed: 'destructive',
}

const TYPE_LABEL: Record<string, string> = {
  log_cleanup: 'Log cleanup',
  channel_test: 'Batch channel test',
  model_update: 'Batch upstream model update',
  midjourney_poll: 'Drawing task polling',
  async_task_poll: 'Async task polling',
  account_pool_check: 'Account pool check',
  subscription_maintenance: 'Subscription maintenance',
}

function isActiveStatus(status: SystemTaskStatus) {
  return status === 'pending' || status === 'running'
}

function getProgress(task: SystemTask) {
  const progress = task.state?.progress
  if (typeof progress !== 'number' || Number.isNaN(progress)) return null
  return Math.max(0, Math.min(100, progress))
}

function getTaskTypeLabel(taskType: string) {
  return TYPE_LABEL[taskType] ?? taskType
}

function getDeletedCount(task: SystemTask) {
  const result = task.result as { deleted_count?: unknown } | undefined
  return typeof result?.deleted_count === 'number' ? result.deleted_count : null
}

function taskStatusDotClass(status: SystemTaskStatus) {
  return cn(
    'size-1.5 rounded-full',
    status === 'failed'
      ? 'bg-destructive'
      : status === 'running'
        ? 'bg-primary'
        : 'bg-muted-foreground'
  )
}

type SystemTasksTableProps = {
  tasks: SystemTask[]
}

function SystemTasksTable(props: SystemTasksTableProps) {
  const { t } = useTranslation()

  return (
    <div className='rounded-md border'>
      <Table className='min-w-[980px]'>
        <TableHeader>
          <TableRow className='bg-muted/40 hover:bg-muted/40'>
            <TableHead className='h-9 min-w-[240px] px-4 text-xs'>
              {t('Task')}
            </TableHead>
            <TableHead className='h-9 w-[120px] text-xs'>
              {t('Status')}
            </TableHead>
            <TableHead className='h-9 w-[190px] text-xs'>
              {t('Progress')}
            </TableHead>
            <TableHead className='h-9 w-[220px] text-xs'>
              {t('Executor')}
            </TableHead>
            <TableHead className='h-9 w-[170px] text-xs'>
              {t('Updated')}
            </TableHead>
            <TableHead className='h-9 w-[220px] pr-4 text-xs'>
              {t('Result')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.tasks.map((task) => {
            const progress = getProgress(task)
            const deletedCount = getDeletedCount(task)

            return (
              <TableRow key={task.task_id} className='hover:bg-muted/30'>
                <TableCell className='px-4 py-2.5 align-middle'>
                  <div className='min-w-0'>
                    <div className='truncate text-sm font-medium'>
                      {t(getTaskTypeLabel(task.type))}
                    </div>
                    <div className='text-muted-foreground truncate font-mono text-[11px]'>
                      {task.task_id}
                    </div>
                  </div>
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <Badge
                    variant={STATUS_BADGE_VARIANT[task.status]}
                    className='gap-1.5'
                  >
                    <span
                      className={taskStatusDotClass(task.status)}
                      aria-hidden='true'
                    />
                    {t(task.status)}
                  </Badge>
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <div className='flex items-center gap-2'>
                    <Progress value={progress ?? 0} className='w-24' />
                    <span className='text-muted-foreground w-10 text-right text-xs tabular-nums'>
                      {progress === null ? '-' : `${progress}%`}
                    </span>
                  </div>
                </TableCell>
                <TableCell className='py-2.5 align-middle'>
                  <div className='text-muted-foreground max-w-[210px] truncate font-mono text-xs'>
                    {task.locked_by || '-'}
                  </div>
                </TableCell>
                <TableCell
                  className='text-muted-foreground py-2.5 align-middle text-xs'
                  title={formatTimestampToDate(task.updated_at)}
                >
                  {formatTimestampToDate(task.updated_at)}
                </TableCell>
                <TableCell
                  className={cn(
                    'max-w-[220px] truncate py-2.5 pr-4 align-middle text-xs',
                    task.error ? 'text-destructive' : 'text-muted-foreground'
                  )}
                  title={task.error || undefined}
                >
                  {task.error ||
                    (deletedCount === null
                      ? '-'
                      : t('{{count}} log entries removed.', {
                          count: deletedCount,
                        }))}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}

export function SystemTasksPanel() {
  const { t } = useTranslation()
  const tasksQuery = useQuery({
    queryKey: ['system-info', 'system-tasks'],
    queryFn: async () => {
      const res = await listSystemTasks(TASK_LIMIT)
      if (!res.success || !Array.isArray(res.data)) {
        throw new Error(res.message || t('We could not load system tasks.'))
      }
      return res.data
    },
    staleTime: 30 * 1000,
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.some((task) => isActiveStatus(task.status))
        ? ACTIVE_TASK_POLL_INTERVAL_MS
        : false,
  })

  const tasks = tasksQuery.data ?? []
  const activeTasks = tasks.filter((task) => isActiveStatus(task.status))
  const historyTasks = tasks.filter((task) => !isActiveStatus(task.status))
  const loading = tasksQuery.isLoading
  const refreshing = tasksQuery.isFetching && !tasksQuery.isLoading
  const hasActiveTasks = activeTasks.length > 0

  return (
    <Card size='sm'>
      <CardHeader className='border-b'>
        <div className='flex items-start gap-2'>
          <span className='bg-muted text-muted-foreground inline-flex size-7 items-center justify-center rounded-md'>
            <ListChecks className='size-4' aria-hidden='true' />
          </span>
          <div className='min-w-0'>
            <CardTitle>{t('System Tasks')}</CardTitle>
            <CardDescription>
              {t(
                'Recent maintenance tasks running across instances and their execution status.'
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
              {hasActiveTasks
                ? t('Auto-refreshing every {{seconds}}s', {
                    seconds: ACTIVE_TASK_POLL_INTERVAL_MS / 1000,
                  })
                : t('Live refresh pauses when no task is running')}
            </span>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => void tasksQuery.refetch()}
              disabled={tasksQuery.isFetching}
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

      <CardContent aria-busy={tasksQuery.isFetching} className='pt-0'>
        {loading ? (
          <div className='flex flex-col gap-2 py-4'>
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className='h-9 w-full rounded-md' />
            ))}
          </div>
        ) : tasksQuery.isError ? (
          <ErrorState
            title={t('We could not load system tasks.')}
            description={
              tasksQuery.error instanceof Error
                ? tasksQuery.error.message
                : undefined
            }
            onRetry={() => {
              void tasksQuery.refetch()
            }}
            className='min-h-[220px]'
          />
        ) : tasks.length === 0 ? (
          <Empty className='min-h-[220px] border-0'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <ListChecks aria-hidden='true' />
              </EmptyMedia>
              <EmptyTitle>{t('No system tasks yet.')}</EmptyTitle>
              <EmptyDescription>
                {t(
                  'Tasks will appear after maintenance actions are scheduled.'
                )}
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <div className='flex flex-col gap-4 py-4'>
            <div>
              <div className='mb-2 flex items-center justify-between gap-3'>
                <div className='min-w-0'>
                  <h3 className='text-sm font-medium'>{t('Active Tasks')}</h3>
                  <p className='text-muted-foreground mt-0.5 text-xs'>
                    {t('Tasks currently pending or running.')}
                  </p>
                </div>
                <Badge variant='outline'>{activeTasks.length}</Badge>
              </div>
              {activeTasks.length > 0 ? (
                <SystemTasksTable tasks={activeTasks} />
              ) : (
                <div className='text-muted-foreground rounded-md border border-dashed px-4 py-6 text-center text-sm'>
                  {t('No active system tasks.')}
                </div>
              )}
            </div>

            <div>
              <div className='mb-2 flex items-center justify-between gap-3'>
                <div className='min-w-0'>
                  <h3 className='text-sm font-medium'>{t('Task History')}</h3>
                  <p className='text-muted-foreground mt-0.5 text-xs'>
                    {t('Recently completed or failed system task runs.')}
                  </p>
                </div>
                <Badge variant='outline'>{historyTasks.length}</Badge>
              </div>
              {historyTasks.length > 0 ? (
                <SystemTasksTable tasks={historyTasks} />
              ) : (
                <div className='text-muted-foreground rounded-md border border-dashed px-4 py-6 text-center text-sm'>
                  {t('No historical system tasks.')}
                </div>
              )}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
