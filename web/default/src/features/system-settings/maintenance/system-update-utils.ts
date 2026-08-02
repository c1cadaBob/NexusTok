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
import type { SystemTask, SystemTaskStatus } from '@/features/system-info/types'

export const SYSTEM_UPDATE_TASK_TYPES = ['system_update', 'system_rollback']

export type SystemUpdateTaskState = {
  phase?: string
  progress?: number
  downloaded_bytes?: number
  total_bytes?: number
  target_version?: string
  asset_name?: string
  mode?: string
  target_image?: string
}

export type SystemUpdateTaskResult = {
  current_version?: string
  target_version?: string
  asset_name?: string
  current_executable?: string
  backup_path?: string
  sha256?: string
  html_url?: string
  published_at?: string
  restart_required?: boolean
  rollback_available?: boolean
  mode?: string
  target_image?: string
  old_container_id?: string
  new_container_id?: string
  backup_container_name?: string
}

export type SystemUpdateTask = SystemTask<
  Record<string, unknown>,
  SystemUpdateTaskState,
  SystemUpdateTaskResult
>

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
    if (result?.new_container_id) {
      return 'Docker container rollback applied.'
    }
    return 'Rollback applied. Restart required.'
  }
  return getSystemUpdatePhaseLabel(state?.phase)
}
