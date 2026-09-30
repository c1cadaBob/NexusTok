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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  DownloadIcon,
  ExternalLinkIcon,
  PowerIcon,
  RefreshCcwIcon,
  RotateCcwIcon,
  ShieldAlertIcon,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { CopyButton } from '@/components/copy-button'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Markdown } from '@/components/ui/markdown'
import { Progress } from '@/components/ui/progress'
import { Spinner } from '@/components/ui/spinner'
import { formatTimestamp, formatTimestampToDate } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { statusQueryOptions } from '@/lib/status-query'
import { cn } from '@/lib/utils'

import {
  applySystemUpdate,
  getCurrentSystemTask,
  getLatestSystemUpdate,
  getSystemUpdateTask,
  restartSystemUpdate,
  rollbackSystemUpdate,
} from '../api'
import { SettingsSection } from '../components/settings-section'
import type {
  SystemTask,
  SystemUpdateInfo,
  SystemUpdateTask,
} from '../types'
import {
  getSystemUpdatePhaseLabel,
  getSystemUpdateProgress,
  getSystemUpdateTaskSummary,
  isActiveSystemUpdateStatus,
} from './system-update-utils'

type UpdateCheckerSectionProps = {
  currentVersion?: string | null
  startTime?: number | null
}

type ConfirmAction = 'apply' | 'rollback' | 'restart'

const TASK_POLL_INTERVAL_MS = 2_000
const RESTART_PROBE_INTERVAL_MS = 2_000
const RESTART_PROBE_TIMEOUT_MS = 60_000

export function UpdateCheckerSection(props: UpdateCheckerSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(
    null
  )
  const [releaseDialogOpen, setReleaseDialogOpen] = useState(false)
  const [trackedTaskId, setTrackedTaskId] = useState<string | null>(null)
  const [restartProbing, setRestartProbing] = useState(false)
  const notifiedTaskIds = useRef(new Set<string>())

  const updateQuery = useQuery({
    queryKey: ['system-update', 'latest'],
    queryFn: async () => {
      const response = await getLatestSystemUpdate(false)
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to check for updates'))
      }
      return response.data
    },
    retry: false,
    staleTime: 5 * 60 * 1000,
  })

  const currentUpdateTaskQuery = useQuery({
    queryKey: ['system-update', 'current-task', 'system_update'],
    queryFn: async () => {
      const response = await getCurrentSystemTask<SystemUpdateTask>(
        'system_update'
      )
      if (!response.success) {
        throw new Error(response.message || t('Failed to load system tasks'))
      }
      return response.data ?? null
    },
    retry: false,
    refetchInterval: (query) =>
      isActiveTask(query.state.data) ? TASK_POLL_INTERVAL_MS : false,
  })

  const currentRollbackTaskQuery = useQuery({
    queryKey: ['system-update', 'current-task', 'system_rollback'],
    queryFn: async () => {
      const response = await getCurrentSystemTask<SystemUpdateTask>(
        'system_rollback'
      )
      if (!response.success) {
        throw new Error(response.message || t('Failed to load system tasks'))
      }
      return response.data ?? null
    },
    retry: false,
    refetchInterval: (query) =>
      isActiveTask(query.state.data) ? TASK_POLL_INTERVAL_MS : false,
  })

  const trackedTaskQuery = useQuery({
    queryKey: ['system-update', 'task', trackedTaskId],
    enabled: Boolean(trackedTaskId),
    queryFn: async () => {
      const response = await getSystemUpdateTask(trackedTaskId ?? '')
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load system tasks'))
      }
      return response.data
    },
    retry: false,
    refetchInterval: (query) =>
      isActiveTask(query.state.data) ? TASK_POLL_INTERVAL_MS : false,
  })

  const info = updateQuery.data
  const currentTask = useMemo(() => {
    if (trackedTaskQuery.data) return trackedTaskQuery.data
    if (isActiveTask(currentUpdateTaskQuery.data)) {
      return currentUpdateTaskQuery.data
    }
    if (isActiveTask(currentRollbackTaskQuery.data)) {
      return currentRollbackTaskQuery.data
    }
    return currentUpdateTaskQuery.data ?? currentRollbackTaskQuery.data ?? null
  }, [
    currentRollbackTaskQuery.data,
    currentUpdateTaskQuery.data,
    trackedTaskQuery.data,
  ])
  const taskActive = isActiveTask(currentTask)
  const taskProgress = getSystemUpdateProgress(currentTask)
  const taskResult = currentTask?.result

  const checkMutation = useMutation({
    mutationFn: () => getLatestSystemUpdate(true),
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to check for updates'))
      }
      queryClient.setQueryData(['system-update', 'latest'], response.data)
      if (response.data.release_status === 'none') {
        toast.success(t('No published release was found'))
      } else if (response.data.has_update) {
        toast.success(
          t('Update available: {{version}}', {
            version: response.data.latest_version,
          })
        )
      } else {
        toast.success(
          t('You are running the latest version ({{version}}).', {
            version: response.data.current_version,
          })
        )
      }
    },
    onError: (error) =>
      handleServerError(error, t('Failed to check for updates')),
  })

  const applyMutation = useMutation({
    mutationFn: applySystemUpdate,
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to apply update'))
      }
      setTrackedTaskId(response.data.task_id)
      setConfirmAction(null)
      toast.success(t('System update task started'))
      void invalidateSystemUpdateQueries()
    },
    onError: (error) => handleServerError(error, t('Failed to apply update')),
  })

  const rollbackMutation = useMutation({
    mutationFn: rollbackSystemUpdate,
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to roll back update'))
      }
      setTrackedTaskId(response.data.task_id)
      setConfirmAction(null)
      toast.success(t('System rollback task started'))
      void invalidateSystemUpdateQueries()
    },
    onError: (error) =>
      handleServerError(error, t('Failed to roll back update')),
  })

  const restartMutation = useMutation({
    mutationFn: restartSystemUpdate,
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to restart service'))
      }
      setConfirmAction(null)
      if (response.data.restart_scheduled) {
        toast.success(t('Restart scheduled. Waiting for service to return.'))
        void probeRestart()
      } else if (response.data.manual_required) {
        toast.success(t('Manual restart required'))
      }
    },
    onError: (error) =>
      handleServerError(error, t('Failed to restart service')),
  })

  async function invalidateSystemUpdateQueries() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['system-update'] }),
      queryClient.invalidateQueries({ queryKey: ['system-info', 'system-tasks'] }),
    ])
  }

  async function probeRestart() {
    setRestartProbing(true)
    const startedAt = Date.now()
    while (Date.now() - startedAt < RESTART_PROBE_TIMEOUT_MS) {
      await new Promise((resolve) =>
        window.setTimeout(resolve, RESTART_PROBE_INTERVAL_MS)
      )
      try {
        await queryClient.fetchQuery({
          ...statusQueryOptions,
          staleTime: 0,
          gcTime: 0,
        })
        await queryClient.invalidateQueries({
          queryKey: statusQueryOptions.queryKey,
        })
        window.location.reload()
        return
      } catch {
        // 服务重启期间探活失败是预期状态，继续下一轮。
      }
    }
    setRestartProbing(false)
    handleServerError(
      new Error(t('Service did not return in time. Please refresh manually.'))
    )
  }

  useEffect(() => {
    if (!currentTask || taskActive) return
    if (notifiedTaskIds.current.has(currentTask.task_id)) return
    notifiedTaskIds.current.add(currentTask.task_id)
    if (currentTask.status === 'succeeded') {
      toast.success(
        currentTask.type === 'system_rollback'
          ? t('System rollback completed')
          : t('System update completed')
      )
    } else if (currentTask.status === 'failed' && currentTask.error) {
      handleServerError(new Error(currentTask.error))
    }
    void Promise.all([
      queryClient.invalidateQueries({ queryKey: ['system-update'] }),
      queryClient.invalidateQueries({ queryKey: ['system-info', 'system-tasks'] }),
    ])
  }, [currentTask, queryClient, t, taskActive])

  const runtimeLabel = info
    ? `${info.runtime.goos}/${info.runtime.goarch}`
    : t('Unknown')
  const currentVersion =
    info?.current_version || props.currentVersion || t('Unknown')
  const latestVersion =
    info?.release_status === 'none'
      ? t('No release published')
      : info?.latest_version || t('Unknown')
  const uptime = props.startTime ? formatTimestamp(props.startTime) : t('Unknown')
  const statusLabel = getUpdateStatusLabel(info, t)
  const statusVariant = getStatusBadgeVariant(info)
  const releaseNotesAvailable = Boolean(info?.release_info?.body)
  const confirmLoading =
    applyMutation.isPending ||
    rollbackMutation.isPending ||
    restartMutation.isPending
  const confirmContent = getConfirmContent(confirmAction)

  return (
    <>
      <SettingsSection title={t('System maintenance')}>
        <div className='flex flex-col gap-4'>
          {updateQuery.isError ? (
            <Alert variant='destructive'>
              <ShieldAlertIcon aria-hidden='true' />
              <AlertTitle>{t('Failed to check for updates')}</AlertTitle>
              <AlertDescription>
                {updateQuery.error instanceof Error
                  ? updateQuery.error.message
                  : t('Please try again later.')}
              </AlertDescription>
            </Alert>
          ) : null}

          <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-4'>
            <StatusTile label={t('Current version')} value={currentVersion} />
            <StatusTile label={t('Latest version')} value={latestVersion} />
            <StatusTile label={t('Runtime')} value={runtimeLabel} />
            <StatusTile label={t('Uptime since')} value={uptime} />
          </div>

          <div className='rounded-lg border p-4'>
            <div className='flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between'>
              <div className='min-w-0 flex-1'>
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge variant={statusVariant}>{statusLabel}</Badge>
                  {info?.cached ? (
                    <Badge variant='outline'>{t('Cached')}</Badge>
                  ) : null}
                  {taskResult?.restart_required ? (
                    <Badge variant='destructive'>{t('Restart required')}</Badge>
                  ) : null}
                </div>
                <div className='mt-3 grid gap-3 md:grid-cols-2'>
                  <DetailItem
                    label={t('Build type')}
                    value={t(getBuildTypeLabel(info?.build_type))}
                  />
                  <DetailItem
                    label={t('Deployment mode')}
                    value={t(getDeploymentModeLabel(info?.deployment_mode))}
                  />
                  <DetailItem
                    label={t('Version comparison')}
                    value={t(getComparisonStatusLabel(info?.comparison_status))}
                  />
                  <DetailItem
                    label={t('Release asset')}
                    value={info?.matched_asset?.name || t('Not available')}
                  />
                  <DetailItem
                    label={t('Asset size')}
                    value={formatBytes(info?.matched_asset?.size)}
                  />
                  <DetailItem
                    label={t('Checksum file')}
                    value={info?.checksum_asset?.name || t('Not available')}
                  />
                  {info?.release_info?.published_at ? (
                    <DetailItem
                      label={t('Published')}
                      value={formatTimestampToDate(
                        new Date(info.release_info.published_at).getTime(),
                        'milliseconds'
                      )}
                    />
                  ) : null}
                  {info?.target_image ? (
                    <DetailItem
                      label={t('Target image')}
                      value={info.target_image}
                      mono
                    />
                  ) : null}
                  {info?.docker ? (
                    <DetailItem
                      label={t('Docker control')}
                      value={getDockerStatusLabel(info, t)}
                    />
                  ) : null}
                </div>
              </div>

              <div className='grid gap-2 sm:flex sm:flex-wrap lg:justify-end'>
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => checkMutation.mutate()}
                  disabled={checkMutation.isPending}
                >
                  {checkMutation.isPending ? (
                    <Spinner data-icon='inline-start' aria-hidden='true' />
                  ) : (
                    <RefreshCcwIcon data-icon='inline-start' aria-hidden='true' />
                  )}
                  {checkMutation.isPending
                    ? t('Checking updates...')
                    : t('Check for updates')}
                </Button>
                <Button
                  type='button'
                  onClick={() => setConfirmAction('apply')}
                  disabled={
                    !info?.can_apply || taskActive || applyMutation.isPending
                  }
                >
                  <DownloadIcon data-icon='inline-start' aria-hidden='true' />
                  {t('Apply update')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => setConfirmAction('rollback')}
                  disabled={
                    !info?.rollback_available ||
                    taskActive ||
                    rollbackMutation.isPending
                  }
                >
                  <RotateCcwIcon data-icon='inline-start' aria-hidden='true' />
                  {t('Rollback')}
                </Button>
                <Button
                  type='button'
                  variant='outline'
                  onClick={() => setConfirmAction('restart')}
                  disabled={restartMutation.isPending || restartProbing}
                >
                  {restartMutation.isPending || restartProbing ? (
                    <Spinner data-icon='inline-start' aria-hidden='true' />
                  ) : (
                    <PowerIcon data-icon='inline-start' aria-hidden='true' />
                  )}
                  {restartProbing ? t('Waiting for restart') : t('Restart service')}
                </Button>
              </div>
            </div>

            {info?.apply_disabled_reason ? (
              <Alert variant='destructive' className='mt-4'>
                <ShieldAlertIcon aria-hidden='true' />
                <AlertTitle>{t('Automatic update unavailable')}</AlertTitle>
                <AlertDescription>
                  {translateUpdateMessage(t, info.apply_disabled_reason)}
                </AlertDescription>
              </Alert>
            ) : null}

            {info?.manual_update_hint ? (
              <Alert className='mt-4'>
                <AlertTitle>{t('Manual update')}</AlertTitle>
                <AlertDescription>
                  {translateUpdateMessage(t, info.manual_update_hint)}
                </AlertDescription>
              </Alert>
            ) : null}

            {info?.docker?.manual_update_command ? (
              <ManualCommandPanel
                command={info.docker.manual_update_command}
                title={t('Docker manual update command')}
                t={t}
              />
            ) : null}

            {!info?.docker?.socket_available &&
            info?.docker?.one_time_enable_command ? (
              <ManualCommandPanel
                command={info.docker.one_time_enable_command}
                title={t('Docker socket enable command')}
                t={t}
              />
            ) : null}

            {info?.warning ? (
              <Alert className='mt-4'>
                <AlertTitle>{t('Update check warning')}</AlertTitle>
                <AlertDescription>
                  {translateUpdateMessage(t, info.warning)}
                </AlertDescription>
              </Alert>
            ) : null}
          </div>

          {currentTask ? (
            <TaskProgressPanel
              task={currentTask}
              progress={taskProgress}
              t={t}
            />
          ) : null}

          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              variant='outline'
              disabled={!releaseNotesAvailable}
              onClick={() => setReleaseDialogOpen(true)}
            >
              <ExternalLinkIcon data-icon='inline-start' aria-hidden='true' />
              {t('View release notes')}
            </Button>
            {info?.release_info?.html_url ? (
              <Button
                type='button'
                variant='ghost'
                onClick={() =>
                  window.open(
                    info.release_info?.html_url,
                    '_blank',
                    'noopener,noreferrer'
                  )
                }
              >
                <ExternalLinkIcon data-icon='inline-start' aria-hidden='true' />
                {t('Open release')}
              </Button>
            ) : null}
          </div>
        </div>
      </SettingsSection>

      <Dialog
        open={releaseDialogOpen}
        onOpenChange={setReleaseDialogOpen}
        title={
          info?.latest_version
            ? t('Release notes for {{version}}', {
                version: info.latest_version,
              })
            : t('Release details')
        }
        description={
          info?.release_info?.published_at
            ? `${t('Published')} ${formatTimestampToDate(
                new Date(info.release_info.published_at).getTime(),
                'milliseconds'
              )}`
            : undefined
        }
        contentClassName='max-h-[80vh]'
        bodyClassName='space-y-4'
        footer={
          <Button
            type='button'
            variant='secondary'
            onClick={() => setReleaseDialogOpen(false)}
          >
            {t('Close')}
          </Button>
        }
      >
        {info?.release_info?.body ? (
          <Markdown>{info.release_info.body}</Markdown>
        ) : (
          <p className='text-muted-foreground text-sm'>
            {t('No release notes provided.')}
          </p>
        )}
      </Dialog>

      <ConfirmDialog
        open={Boolean(confirmAction)}
        onOpenChange={(open) => {
          if (!open) setConfirmAction(null)
        }}
        title={t(confirmContent.title)}
        desc={<p>{t(confirmContent.description)}</p>}
        confirmText={
          <span className='inline-flex items-center gap-2'>
            {confirmLoading ? <Spinner aria-hidden='true' /> : null}
            {t(confirmContent.confirmText)}
          </span>
        }
        destructive={
          confirmAction === 'rollback' || confirmAction === 'restart'
        }
        isLoading={confirmLoading}
        handleConfirm={() => {
          if (confirmAction === 'apply') applyMutation.mutate()
          if (confirmAction === 'rollback') rollbackMutation.mutate()
          if (confirmAction === 'restart') restartMutation.mutate()
        }}
      />
    </>
  )
}

function TaskProgressPanel(props: {
  task: SystemUpdateTask
  progress: number | null
  t: ReturnType<typeof useTranslation>['t']
}) {
  const state = props.task.state
  const result = props.task.result
  return (
    <div className='rounded-lg border p-4' aria-live='polite'>
      <div className='flex flex-col gap-3'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='min-w-0'>
            <div className='text-sm font-medium'>
              {props.t(
                props.task.type === 'system_rollback'
                  ? 'System rollback'
                  : 'System update'
              )}
            </div>
            <div className='text-muted-foreground truncate font-mono text-xs'>
              {props.task.task_id}
            </div>
          </div>
          <Badge
            variant={getTaskStatusVariant(props.task.status)}
          >
            {props.t(props.task.status)}
          </Badge>
        </div>
        <div className='flex flex-col gap-2'>
          <div className='flex items-center justify-between gap-3 text-sm'>
            <span className='text-muted-foreground'>
              {props.t(getSystemUpdatePhaseLabel(state?.phase))}
            </span>
            <span className='text-muted-foreground tabular-nums'>
              {props.progress === null ? '-' : `${props.progress}%`}
            </span>
          </div>
          <Progress value={props.progress ?? 0} />
          {state?.downloaded_bytes ? (
            <div className='text-muted-foreground text-xs'>
              {formatBytes(state.downloaded_bytes)}
              {state.total_bytes ? ` / ${formatBytes(state.total_bytes)}` : null}
            </div>
          ) : null}
        </div>
        <div
          className={cn(
            'text-sm',
            props.task.status === 'failed'
              ? 'text-destructive'
              : 'text-muted-foreground'
          )}
        >
          {props.t(getSystemUpdateTaskSummary(props.task), {
            version: result?.target_version || state?.target_version,
            image: result?.target_image || state?.target_image,
          })}
        </div>
      </div>
    </div>
  )
}

function ManualCommandPanel(props: {
  command: string
  title: string
  t: ReturnType<typeof useTranslation>['t']
}) {
  return (
    <div className='mt-4 rounded-lg border bg-muted/30 p-4'>
      <div className='flex items-center justify-between gap-3'>
        <div className='text-sm font-medium'>{props.title}</div>
        <CopyButton
          value={props.command}
          tooltip={props.t('Copy command')}
          successTooltip={props.t('Command copied')}
          aria-label={props.t('Copy command')}
        />
      </div>
      <pre className='mt-3 max-h-72 overflow-auto whitespace-pre-wrap break-words rounded-md bg-background p-3 font-mono text-xs'>
        {props.command}
      </pre>
    </div>
  )
}

function StatusTile(props: { label: string; value: React.ReactNode }) {
  return (
    <div className='rounded-lg border p-4'>
      <div className='text-muted-foreground text-sm'>{props.label}</div>
      <div className='mt-1 truncate text-lg font-semibold'>{props.value}</div>
    </div>
  )
}

function DetailItem(props: {
  label: string
  value: React.ReactNode
  mono?: boolean
}) {
  return (
    <div className='min-w-0'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div
        className={cn(
          'mt-1 truncate text-sm',
          props.mono && 'font-mono text-xs break-all whitespace-normal'
        )}
        title={typeof props.value === 'string' ? props.value : undefined}
      >
        {props.value}
      </div>
    </div>
  )
}

function formatBytes(bytes?: number) {
  if (typeof bytes !== 'number' || Number.isNaN(bytes)) return '-'
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(
    Math.floor(Math.log(Math.abs(bytes)) / Math.log(1024)),
    units.length - 1
  )
  const value = bytes / 1024 ** index
  return `${value >= 10 || index === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[index]}`
}

function isActiveTask(task?: SystemTask | null) {
  return Boolean(task && isActiveSystemUpdateStatus(task.status))
}

function getBuildTypeLabel(value?: string) {
  switch (value) {
    case 'release':
      return 'Release binary'
    case 'container':
      return 'Container'
    case 'source':
      return 'Source or development build'
    default:
      return 'Unknown'
  }
}

function getDeploymentModeLabel(value?: string) {
  switch (value) {
    case 'binary':
      return 'Binary deployment'
    case 'docker_run':
      return 'Docker run'
    case 'docker_compose':
      return 'Docker Compose'
    case 'container_unknown':
      return 'Container'
    case 'source':
      return 'Source or development build'
    default:
      return 'Unknown'
  }
}

function getComparisonStatusLabel(value?: string) {
  switch (value) {
    case 'older':
      return 'Current version is older'
    case 'latest':
      return 'Current version matches latest release'
    case 'newer':
      return 'Current version is newer than latest release'
    case 'unknown':
      return 'Version comparison unavailable'
    default:
      return 'Unknown'
  }
}

function getUpdateStatusLabel(
  info: SystemUpdateInfo | undefined,
  t: ReturnType<typeof useTranslation>['t']
) {
  if (!info) return t('Not checked')
  if (info.build_type === 'container' && info.can_apply) {
    return t('Docker image refresh available')
  }
  if (info.release_status === 'none') return t('No release published')
  if (info.has_update && info.can_apply) return t('Update available')
  if (info.has_update) return t('Manual update required')
  return t('Up to date')
}

function getStatusBadgeVariant(info: SystemUpdateInfo | undefined) {
  if (!info) return 'secondary' as const
  if (info.release_status === 'none') return 'secondary' as const
  if (info.has_update && !info.can_apply) return 'destructive' as const
  if (info.has_update && info.can_apply) return 'default' as const
  return 'secondary' as const
}

function getDockerStatusLabel(
  info: SystemUpdateInfo,
  t: ReturnType<typeof useTranslation>['t']
) {
  if (!info.docker?.socket_available) return t('Docker socket not mounted')
  if (info.docker.healthcheck_degraded) {
    return t('Docker running without Healthcheck')
  }
  return t('Docker socket available')
}

function getTaskStatusVariant(status: SystemTask['status']) {
  if (status === 'failed') return 'destructive' as const
  if (status === 'running') return 'default' as const
  return 'secondary' as const
}

function translateUpdateMessage(
  t: ReturnType<typeof useTranslation>['t'],
  message: string
) {
  return t(message)
}

function getConfirmContent(action: ConfirmAction | null) {
  if (action === 'apply') {
    return {
      title: 'Apply system update',
      description:
        'Download, verify, and replace the current executable. The service must be restarted after the task succeeds.',
      confirmText: 'Apply update',
    }
  }
  if (action === 'rollback') {
    return {
      title: 'Rollback system update',
      description:
        'Exchange the current executable with the stable backup. The backup remains available for another rollback.',
      confirmText: 'Rollback',
    }
  }
  return {
    title: 'Restart service',
    description:
      'The process will exit after the response is sent. Make sure systemd or your supervisor is configured to start it again.',
    confirmText: 'Restart service',
  }
}
