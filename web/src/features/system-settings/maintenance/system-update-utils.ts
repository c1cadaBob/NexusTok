import type {
  SystemTask,
  SystemTaskStatus,
  SystemUpdateTaskResult,
  SystemUpdateTaskState,
} from '../types'

export const SYSTEM_UPDATE_TASK_TYPES = ['system_update', 'system_rollback']

export const SYSTEM_UPDATE_PHASE_LABELS: Record<string, string> = {
  checking: 'Checking release',
  downloading: 'Downloading binary',
  verifying: 'Verifying checksum',
  backing_up: 'Backing up current binary',
  replacing: 'Replacing executable',
  ready: 'Ready to restart',
  rolling_back: 'Rolling back binary',
  pulling_image: 'Pulling Docker image',
  starting_helper: 'Starting update helper',
  recreating_container: 'Recreating Docker container',
  probing: 'Checking updated service',
}

export function isActiveSystemUpdateStatus(status: SystemTaskStatus) {
  return status === 'pending' || status === 'running'
}

export function isSystemUpdateTask(task?: SystemTask | null) {
  return Boolean(task && SYSTEM_UPDATE_TASK_TYPES.includes(task.type))
}

export function getSystemUpdateProgress(task?: SystemTask | null) {
  const progress = (task?.state as SystemUpdateTaskState | undefined)?.progress
  if (typeof progress !== 'number' || Number.isNaN(progress)) return null
  return Math.max(0, Math.min(100, progress))
}

export function getSystemUpdatePhaseLabel(phase?: string) {
  if (!phase) return 'Waiting to start'
  return SYSTEM_UPDATE_PHASE_LABELS[phase] ?? phase
}

export function getSystemUpdateTaskSummary(task: SystemTask) {
  const result = task.result as SystemUpdateTaskResult | undefined
  const state = task.state as SystemUpdateTaskState | undefined

  if (task.error) return task.error
  if (task.type === 'system_update' && task.status === 'succeeded') {
    if (result?.target_image) {
      return 'Docker image updated to {{image}}.'
    }
    const targetVersion = result?.target_version || state?.target_version
    return targetVersion
      ? 'Updated to {{version}}. Restart required.'
      : 'Update applied. Restart required.'
  }
  if (task.type === 'system_rollback' && task.status === 'succeeded') {
    if (result?.restart_required === false || result?.health_status) {
      return 'Docker container rollback applied.'
    }
    return 'Rollback applied. Restart required.'
  }
  return getSystemUpdatePhaseLabel(state?.phase)
}
