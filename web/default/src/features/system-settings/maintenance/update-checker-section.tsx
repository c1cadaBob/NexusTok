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
import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import {
  DownloadIcon,
  ExternalLinkIcon,
  PowerIcon,
  RefreshCcwIcon,
  RotateCcwIcon,
  ShieldAlertIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { getStatus } from '@/lib/api'
import { formatTimestamp, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Markdown } from '@/components/ui/markdown'
import { Progress } from '@/components/ui/progress'
import { Spinner } from '@/components/ui/spinner'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { getCurrentSystemTask, getSystemTask } from '@/features/system-info/api'
import type { SystemTask } from '@/features/system-info/types'
import {
  applySystemUpdate,
  getLatestSystemUpdate,
  restartSystemUpdate,
  rollbackSystemUpdate,
} from '../api'
import { SettingsSection } from '../components/settings-section'
import type { SystemUpdateInfo } from '../types'
import {
  getSystemUpdatePhaseLabel,
  getSystemUpdateProgress,
  getSystemUpdateTaskSummary,
  isActiveSystemUpdateStatus,
  type SystemUpdateTaskResult,
  type SystemUpdateTaskState,
} from './system-update-utils'

type UpdateCheckerSectionProps = {
  currentVersion?: string | null
  startTime?: number | null
}

type ConfirmAction = 'apply' | 'rollback' | 'restart'

const TASK_POLL_INTERVAL_MS = 3_000
const RESTART_PROBE_INTERVAL_MS = 2_000
const RESTART_PROBE_TIMEOUT_MS = 60_000

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
  return `${value >= 10 || index === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[index]}`
}

function isActiveTask(task?: SystemTask | null) {
  return Boolean(task && isActiveSystemUpdateStatus(task.status))
}

function getTaskState(task?: SystemTask | null) {
  return task?.state as SystemUpdateTaskState | undefined
}

function getTaskResult(task?: SystemTask | null) {
  return task?.result as SystemUpdateTaskResult | undefined
}

function getBuildTypeLabel(buildType?: string) {
  switch (buildType) {
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

function getDeploymentModeLabel(mode?: string) {
  switch (mode) {
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

function getComparisonStatusLabel(status?: string) {
  switch (status) {
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

function getStatusBadgeVariant(info?: SystemUpdateInfo) {
  if (!info) return 'secondary' as const
  if (info.build_type === 'container' && info.can_apply) return 'default' as const
  if (info.release_status === 'none') return 'secondary' as const
  if (info.has_update && info.can_apply) return 'default' as const
  if (info.has_update && !info.can_apply) return 'destructive' as const
  return 'secondary' as const
}

function getUpdateStatusLabel(info?: SystemUpdateInfo) {
  if (!info) return 'Not checked'
  if (info.build_type === 'container' && info.can_apply)
    return 'Docker image refresh available'
  if (info.release_status === 'none') return 'No release published'
  if (info.has_update && info.can_apply) return 'Update available'
  if (info.has_update) return 'Manual update required'
  return 'Up to date'
}

function getTaskTypeLabel(type?: string) {
  if (type === 'system_rollback') return 'System rollback'
  return 'System update'
}

function translateManualUpdateHint(t: TFunction, hint?: string) {
  switch (hint) {
    case 'Docker deployments should update by pulling c1cadabob/nexustok:latest and recreating the container with the same mounted data directories.':
      return t(
        'Docker deployments should update by pulling c1cadabob/nexustok:latest and recreating the container with the same mounted data directories.'
      )
    case 'Source or development builds should be updated by pulling the latest code, rebuilding, and restarting the service manually.':
      return t(
        'Source or development builds should be updated by pulling the latest code, rebuilding, and restarting the service manually.'
      )
    case 'Publish a GitHub Release with matching binary assets and checksums before applying dashboard updates.':
      return t(
        'Publish a GitHub Release with matching binary assets and checksums before applying dashboard updates.'
      )
    default:
      return hint || ''
  }
}

export function UpdateCheckerSection({
  currentVersion,
  startTime,
}: UpdateCheckerSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null)
  const [releaseDialogOpen, setReleaseDialogOpen] = useState(false)
  const [trackedTaskId, setTrackedTaskId] = useState<string | null>(null)
  const [restartProbing, setRestartProbing] = useState(false)
  const notifiedTaskIds = useRef(new Set<string>())

  const updateQuery = useQuery({
    queryKey: ['system-update', 'latest'],
    queryFn: async () => {
      const res = await getLatestSystemUpdate(false)
      if (!res.success || !res.data) {
        throw new Error(res.message || 'Failed to check for updates')
      }
      return res.data
    },
    staleTime: 5 * 60 * 1000,
    retry: false,
  })

  const currentUpdateTaskQuery = useQuery({
    queryKey: ['system-update', 'current-task', 'system_update'],
    queryFn: async () => {
      const res = await getCurrentSystemTask('system_update')
      if (!res.success) {
        throw new Error(res.message || 'We could not load system tasks.')
      }
      return res.data ?? null
    },
    retry: false,
    refetchInterval: (query) =>
      isActiveTask(query.state.data) ? TASK_POLL_INTERVAL_MS : false,
  })

  const currentRollbackTaskQuery = useQuery({
    queryKey: ['system-update', 'current-task', 'system_rollback'],
    queryFn: async () => {
      const res = await getCurrentSystemTask('system_rollback')
      if (!res.success) {
        throw new Error(res.message || 'We could not load system tasks.')
      }
      return res.data ?? null
    },
    retry: false,
    refetchInterval: (query) =>
      isActiveTask(query.state.data) ? TASK_POLL_INTERVAL_MS : false,
  })

  const trackedTaskQuery = useQuery({
    queryKey: ['system-update', 'task', trackedTaskId],
    enabled: Boolean(trackedTaskId),
    queryFn: async () => {
      const res = await getSystemTask(trackedTaskId!)
      if (!res.success || !res.data) {
        throw new Error(res.message || 'We could not load system tasks.')
      }
      return res.data
    },
    retry: false,
    refetchInterval: (query) =>
      isActiveTask(query.state.data) ? TASK_POLL_INTERVAL_MS : false,
  })

  const info = updateQuery.data
  const currentTask = useMemo(() => {
    const tracked = trackedTaskQuery.data
    if (tracked) return tracked
    if (isActiveTask(currentUpdateTaskQuery.data))
      return currentUpdateTaskQuery.data
    if (isActiveTask(currentRollbackTaskQuery.data)) {
      return currentRollbackTaskQuery.data
    }
    return currentUpdateTaskQuery.data ?? currentRollbackTaskQuery.data ?? null
  }, [
    currentRollbackTaskQuery.data,
    currentUpdateTaskQuery.data,
    trackedTaskQuery.data,
  ])
  const taskState = getTaskState(currentTask)
  const taskResult = getTaskResult(currentTask)
  const taskProgress = getSystemUpdateProgress(currentTask)
  const taskActive = isActiveTask(currentTask)
  const restartRequired = Boolean(taskResult?.restart_required)
  const version = info?.current_version || currentVersion || t('Unknown')
  const latestVersion =
    info?.release_status === 'none'
      ? t('No release published')
      : info?.latest_version || t('Unknown')
  const uptime = startTime ? formatTimestamp(startTime) : t('Unknown')
  const runtimeLabel = info
    ? `${info.runtime.goos}/${info.runtime.goarch}`
    : t('Unknown')
  const hasReleaseNotes = Boolean(info?.release_info?.body)
  const manualUpdateHint = translateManualUpdateHint(
    t,
    info?.manual_update_hint
  )
  const showDockerManualCommands =
    info?.build_type === 'container' && Boolean(info?.manual_update_hint)
  const dockerSocketEnabled = Boolean(info?.docker?.socket_available)
  const dockerEnableCommand =
    info?.docker?.one_time_enable_command ||
    `docker rm -f nexustok 2>/dev/null || true

docker run --name nexustok -d --restart always \\
  -p 3030:3030 \\
  -e TZ=Asia/Shanghai \\
  -e PORT=3030 \\
  -e SESSION_SECRET_FILE=/data/session_secret \\
  -v /opt/nexustok/data:/data \\
  -v /opt/nexustok/logs:/app/logs \\
  -v /var/run/docker.sock:/var/run/docker.sock \\
  c1cadabob/nexustok:latest`
  const dockerManualCommand =
    info?.docker?.manual_update_command ||
    `docker pull c1cadabob/nexustok:latest
docker stop nexustok
docker rm nexustok
# ${t('Recreate docker run with the same mounted data directories')}`

  const checkMutation = useMutation({
    mutationFn: () => getLatestSystemUpdate(true),
    onSuccess: (res) => {
      if (!res.success || !res.data) {
        throw new Error(res.message || t('Failed to check for updates'))
      }
      queryClient.setQueryData(['system-update', 'latest'], res.data)
      if (res.data.release_status === 'none') {
        toast.info(t('No published GitHub release was found.'))
      } else if (res.data.has_update) {
        toast.success(
          t('New version available: {{version}}', {
            version: res.data.latest_version,
          })
        )
      } else {
        toast.success(
          t('You are running the latest version ({{version}}).', {
            version: res.data.current_version,
          })
        )
      }
    },
    onError: (error) => {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to check for updates')
      toast.error(message)
    },
  })

  const applyMutation = useMutation({
    mutationFn: applySystemUpdate,
    onSuccess: (res) => {
      if (!res.success || !res.data) {
        throw new Error(res.message || t('Failed to apply update'))
      }
      setTrackedTaskId(res.data.task_id)
      setConfirmAction(null)
      toast.success(t('System update task created.'))
      void queryClient.invalidateQueries({
        queryKey: ['system-info', 'system-tasks'],
      })
      void queryClient.invalidateQueries({
        queryKey: ['system-update', 'current-task'],
      })
    },
    onError: (error) => {
      const message =
        error instanceof Error ? error.message : t('Failed to apply update')
      toast.error(message)
    },
  })

  const rollbackMutation = useMutation({
    mutationFn: rollbackSystemUpdate,
    onSuccess: (res) => {
      if (!res.success || !res.data) {
        throw new Error(res.message || t('Failed to roll back update'))
      }
      setTrackedTaskId(res.data.task_id)
      setConfirmAction(null)
      toast.success(t('System rollback task created.'))
      void queryClient.invalidateQueries({
        queryKey: ['system-info', 'system-tasks'],
      })
      void queryClient.invalidateQueries({
        queryKey: ['system-update', 'current-task'],
      })
    },
    onError: (error) => {
      const message =
        error instanceof Error ? error.message : t('Failed to roll back update')
      toast.error(message)
    },
  })

  const restartMutation = useMutation({
    mutationFn: restartSystemUpdate,
    onSuccess: (res) => {
      if (!res.success || !res.data) {
        throw new Error(res.message || t('Failed to restart service'))
      }
      setConfirmAction(null)
      if (res.data.restart_scheduled) {
        toast.success(t('Restart scheduled. Waiting for service to return.'))
        void probeRestart()
      } else {
        toast.info(t(res.data.message))
      }
    },
    onError: (error) => {
      const message =
        error instanceof Error ? error.message : t('Failed to restart service')
      toast.error(message)
    },
  })

  useEffect(() => {
    if (!currentTask || isActiveTask(currentTask)) return
    if (notifiedTaskIds.current.has(currentTask.task_id)) return
    notifiedTaskIds.current.add(currentTask.task_id)

    if (currentTask.status === 'succeeded') {
      toast.success(
        t(getSystemUpdateTaskSummary(currentTask), {
          version:
            getTaskResult(currentTask)?.target_version ||
            getTaskState(currentTask)?.target_version,
        })
      )
      void queryClient.invalidateQueries({
        queryKey: ['system-update', 'latest'],
      })
      void queryClient.invalidateQueries({
        queryKey: ['system-update', 'current-task'],
      })
      void queryClient.invalidateQueries({
        queryKey: ['system-info', 'system-tasks'],
      })
      return
    }

    if (currentTask.status === 'failed') {
      toast.error(currentTask.error || t('System update task failed.'))
    }
  }, [currentTask, queryClient, t])

  const probeRestart = async () => {
    setRestartProbing(true)
    const startedAt = Date.now()
    while (Date.now() - startedAt < RESTART_PROBE_TIMEOUT_MS) {
      await new Promise((resolve) =>
        setTimeout(resolve, RESTART_PROBE_INTERVAL_MS)
      )
      try {
        await getStatus()
        window.location.reload()
        return
      } catch {
        /* 服务重启期间请求失败是预期状态，继续等待下一轮探活。 */
      }
    }
    setRestartProbing(false)
    toast.error(t('Service did not return in time. Please refresh manually.'))
  }

  const handleConfirm = () => {
    if (confirmAction === 'apply') {
      applyMutation.mutate()
      return
    }
    if (confirmAction === 'rollback') {
      rollbackMutation.mutate()
      return
    }
    if (confirmAction === 'restart') {
      restartMutation.mutate()
    }
  }

  const confirmLoading =
    applyMutation.isPending ||
    rollbackMutation.isPending ||
    restartMutation.isPending

  const confirmContent = getConfirmContent(confirmAction)

  return (
    <>
      <SettingsSection
        title={t('System maintenance')}
        description={t('Check, apply, roll back, and restart release updates.')}
      >
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
            <StatusTile label={t('Current version')} value={version} />
            <StatusTile label={t('Latest version')} value={latestVersion} />
            <StatusTile label={t('Runtime')} value={runtimeLabel} />
            <StatusTile label={t('Uptime since')} value={uptime} />
          </div>

          <div className='rounded-lg border p-4'>
            <div className='flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between'>
              <div className='min-w-0 flex-1'>
                <div className='flex flex-wrap items-center gap-2'>
                  <Badge variant={getStatusBadgeVariant(info)}>
                    {t(getUpdateStatusLabel(info))}
                  </Badge>
                  {info?.cached ? (
                    <Badge variant='outline'>{t('Cached')}</Badge>
                  ) : null}
                  {restartRequired ? (
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
                    value={t(
                      getComparisonStatusLabel(info?.comparison_status)
                    )}
                  />
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
                      value={
                        dockerSocketEnabled
                          ? t('Docker socket available')
                          : t('Docker socket not mounted')
                      }
                    />
                  ) : null}
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
                </div>
              </div>

              <div className='grid gap-2 sm:flex sm:flex-wrap lg:justify-end'>
                <Button
                  type='button'
                  variant='outline'
                  className='w-full sm:w-auto'
                  onClick={() => checkMutation.mutate()}
                  disabled={checkMutation.isPending}
                >
                  {checkMutation.isPending ? (
                    <Spinner data-icon='inline-start' aria-hidden='true' />
                  ) : (
                    <RefreshCcwIcon
                      data-icon='inline-start'
                      aria-hidden='true'
                    />
                  )}
                  {checkMutation.isPending
                    ? t('Checking updates...')
                    : t('Check for updates')}
                </Button>
                <Button
                  type='button'
                  className='w-full sm:w-auto'
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
                  className='w-full sm:w-auto'
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
                  className='w-full sm:w-auto'
                  onClick={() => setConfirmAction('restart')}
                  disabled={restartMutation.isPending || restartProbing}
                >
                  {restartMutation.isPending || restartProbing ? (
                    <Spinner data-icon='inline-start' aria-hidden='true' />
                  ) : (
                    <PowerIcon data-icon='inline-start' aria-hidden='true' />
                  )}
                  {restartProbing
                    ? t('Waiting for restart')
                    : t('Restart service')}
                </Button>
              </div>
            </div>

            {info?.apply_disabled_reason ? (
              <Alert variant='destructive' className='mt-4'>
                <ShieldAlertIcon aria-hidden='true' />
                <AlertTitle>{t('Automatic update unavailable')}</AlertTitle>
                <AlertDescription>
                  {t(info.apply_disabled_reason)}
                </AlertDescription>
              </Alert>
            ) : null}

            {info?.manual_update_hint ? (
              <Alert className='mt-4'>
                <AlertTitle>{t('Manual update')}</AlertTitle>
                <AlertDescription>
                  <div className='flex flex-col gap-3'>
                    <p>{manualUpdateHint}</p>
                    {showDockerManualCommands ? (
                      <div className='grid gap-3 lg:grid-cols-2'>
                        <div className='min-w-0'>
                          <div className='mb-1 text-xs font-medium'>
                            {dockerSocketEnabled
                              ? t('Manual Docker update')
                              : t('Enable dashboard Docker updates')}
                          </div>
                          <pre className='bg-muted text-muted-foreground overflow-x-auto rounded-md p-3 text-xs'>
                            <code>
                              {dockerSocketEnabled
                                ? dockerManualCommand
                                : dockerEnableCommand}
                            </code>
                          </pre>
                        </div>
                        <div className='min-w-0'>
                          <div className='mb-1 text-xs font-medium'>
                            {t('Docker Compose')}
                          </div>
                          <pre className='bg-muted text-muted-foreground overflow-x-auto rounded-md p-3 text-xs'>
                            <code>{`docker compose pull nexustok
docker compose up -d nexustok`}</code>
                          </pre>
                        </div>
                      </div>
                    ) : null}
                  </div>
                </AlertDescription>
              </Alert>
            ) : null}

            {info?.build_type === 'container' ? (
              <Alert className='mt-4'>
                <ShieldAlertIcon aria-hidden='true' />
                <AlertTitle>{t('Docker update boundary')}</AlertTitle>
                <AlertDescription>
                  {dockerSocketEnabled
                    ? t(
                        'Docker socket is mounted. Dashboard updates can pull the target image and recreate this container with the same ports, volumes, environment, and restart policy.'
                      )
                    : t(
                        'Mount /var/run/docker.sock to enable dashboard Docker updates. Without it, NexusTok can check updates but cannot control the host Docker Engine.'
                      )}
                </AlertDescription>
              </Alert>
            ) : null}

            {info?.warning ? (
              <Alert className='mt-4'>
                <AlertTitle>{t('Update check warning')}</AlertTitle>
                <AlertDescription>{t(info.warning)}</AlertDescription>
              </Alert>
            ) : null}
          </div>

          {currentTask ? (
            <div className='rounded-lg border p-4' aria-live='polite'>
              <div className='flex flex-col gap-3'>
                <div className='flex flex-wrap items-center justify-between gap-3'>
                  <div className='min-w-0'>
                    <div className='text-sm font-medium'>
                      {t(getTaskTypeLabel(currentTask.type))}
                    </div>
                    <div className='text-muted-foreground truncate font-mono text-xs'>
                      {currentTask.task_id}
                    </div>
                  </div>
                  <Badge
                    variant={
                      currentTask.status === 'failed'
                        ? 'destructive'
                        : currentTask.status === 'running'
                          ? 'default'
                          : 'secondary'
                    }
                  >
                    {t(currentTask.status)}
                  </Badge>
                </div>

                <div className='flex flex-col gap-2'>
                  <div className='flex items-center justify-between gap-3 text-sm'>
                    <span className='text-muted-foreground'>
                      {t(getSystemUpdatePhaseLabel(taskState?.phase))}
                    </span>
                    <span className='text-muted-foreground tabular-nums'>
                      {taskProgress === null ? '-' : `${taskProgress}%`}
                    </span>
                  </div>
                  <Progress value={taskProgress ?? 0} />
                  {taskState?.downloaded_bytes ? (
                    <div className='text-muted-foreground text-xs'>
                      {formatBytes(taskState.downloaded_bytes)}
                      {taskState.total_bytes
                        ? ` / ${formatBytes(taskState.total_bytes)}`
                        : null}
                    </div>
                  ) : null}
                </div>

                <div
                  className={cn(
                    'text-sm',
                    currentTask.status === 'failed'
                      ? 'text-destructive'
                      : 'text-muted-foreground'
                  )}
                >
                  {t(getSystemUpdateTaskSummary(currentTask), {
                    version:
                      taskResult?.target_version || taskState?.target_version,
                    image: taskResult?.target_image || taskState?.target_image,
                  })}
                </div>

                {taskResult?.backup_path ||
                taskResult?.sha256 ||
                taskResult?.backup_container_name ||
                taskResult?.new_container_id ? (
                  <div className='grid gap-3 md:grid-cols-2'>
                    <DetailItem
                      label={t('Backup path')}
                      value={
                        taskResult.backup_path ||
                        taskResult.backup_container_name ||
                        t('Not available')
                      }
                    />
                    <DetailItem
                      label={
                        taskResult.new_container_id
                          ? t('New container')
                          : t('SHA256')
                      }
                      value={
                        taskResult.new_container_id ||
                        taskResult.sha256 ||
                        t('Not available')
                      }
                      mono
                    />
                  </div>
                ) : null}
              </div>
            </div>
          ) : null}

          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              variant='outline'
              disabled={!hasReleaseNotes}
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

      <Dialog open={releaseDialogOpen} onOpenChange={setReleaseDialogOpen}>
        <DialogContent className='max-h-[80vh] overflow-y-auto'>
          <DialogHeader>
            <DialogTitle>
              {info?.latest_version
                ? t('Release notes for {{version}}', {
                    version: info.latest_version,
                  })
                : t('Release details')}
            </DialogTitle>
            {info?.release_info?.published_at ? (
              <DialogDescription>
                {t('Published')}{' '}
                {formatTimestampToDate(
                  new Date(info.release_info.published_at).getTime(),
                  'milliseconds'
                )}
              </DialogDescription>
            ) : null}
          </DialogHeader>
          {info?.release_info?.body ? (
            <Markdown>{info.release_info.body}</Markdown>
          ) : (
            <p className='text-muted-foreground text-sm'>
              {t('No release notes provided.')}
            </p>
          )}
          <DialogFooter>
            <Button
              type='button'
              variant='secondary'
              onClick={() => setReleaseDialogOpen(false)}
            >
              {t('Close')}
            </Button>
          </DialogFooter>
        </DialogContent>
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
        handleConfirm={handleConfirm}
      />
    </>
  )
}

function StatusTile({
  label,
  value,
}: {
  label: string
  value: React.ReactNode
}) {
  return (
    <div className='rounded-lg border p-4'>
      <div className='text-muted-foreground text-sm'>{label}</div>
      <div className='mt-1 truncate text-lg font-semibold'>{value}</div>
    </div>
  )
}

function DetailItem({
  label,
  value,
  mono,
}: {
  label: string
  value: React.ReactNode
  mono?: boolean
}) {
  return (
    <div className='min-w-0'>
      <div className='text-muted-foreground text-xs'>{label}</div>
      <div
        className={cn(
          'mt-1 truncate text-sm',
          mono && 'font-mono text-xs break-all whitespace-normal'
        )}
        title={typeof value === 'string' ? value : undefined}
      >
        {value}
      </div>
    </div>
  )
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
        'Restore the .backup executable created by the previous successful update. The service must be restarted after rollback succeeds.',
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
