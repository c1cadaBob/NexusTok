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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { UpdateCheckerSection } from '../update-checker-section'

const mocks = vi.hoisted(() => ({
  applySystemUpdate: vi.fn(),
  getCurrentSystemTask: vi.fn(),
  getLatestSystemUpdate: vi.fn(),
  getSystemUpdateTask: vi.fn(),
  restartSystemUpdate: vi.fn(),
  rollbackSystemUpdate: vi.fn(),
  statusQueryFn: vi.fn(),
}))

vi.mock('../../api', () => ({
  applySystemUpdate: mocks.applySystemUpdate,
  getCurrentSystemTask: mocks.getCurrentSystemTask,
  getLatestSystemUpdate: mocks.getLatestSystemUpdate,
  getSystemUpdateTask: mocks.getSystemUpdateTask,
  restartSystemUpdate: mocks.restartSystemUpdate,
  rollbackSystemUpdate: mocks.rollbackSystemUpdate,
}))

vi.mock('@/lib/status-query', () => ({
  statusQueryOptions: {
    queryKey: ['status'],
    queryFn: mocks.statusQueryFn,
  },
}))

const releaseInfo = {
  tag_name: 'v1.1.0',
  name: 'NexusTok v1.1.0',
  body: '## Release notes\n\nBug fixes and safer rollback.',
  html_url: 'https://github.com/c1cadaBob/NexusTok/releases/tag/v1.1.0',
  published_at: '2026-09-30T00:00:00Z',
}

const binaryInfo = {
  current_version: 'v1.0.0',
  latest_version: 'v1.1.0',
  has_update: true,
  cached: false,
  release_info: releaseInfo,
  matched_asset: {
    name: 'nexustok-v1.1.0',
    download_url:
      'https://github.com/c1cadaBob/NexusTok/releases/download/v1.1.0/nexustok-v1.1.0',
    size: 1024,
  },
  checksum_asset: {
    name: 'checksums-linux.txt',
    download_url:
      'https://github.com/c1cadaBob/NexusTok/releases/download/v1.1.0/checksums-linux.txt',
    size: 128,
  },
  runtime: {
    goos: 'linux',
    goarch: 'amd64',
    is_running_in_container: false,
  },
  build_type: 'release',
  deployment_mode: 'binary',
  comparison_status: 'older',
  update_method: 'binary_replace',
  docker_control_available: false,
  can_apply: true,
  rollback_available: true,
  release_status: 'published',
}

const taskState = {
  phase: 'checking',
  progress: 10,
  target_version: 'v1.1.0',
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  })
}

function renderSection() {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <UpdateCheckerSection
        currentVersion='v1.0.0'
        startTime={Date.parse('2026-09-29T00:00:00Z')}
      />
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

beforeEach(() => {
  mocks.applySystemUpdate.mockReset()
  mocks.getCurrentSystemTask.mockReset()
  mocks.getLatestSystemUpdate.mockReset()
  mocks.getSystemUpdateTask.mockReset()
  mocks.restartSystemUpdate.mockReset()
  mocks.rollbackSystemUpdate.mockReset()
  mocks.statusQueryFn.mockReset()

  mocks.getLatestSystemUpdate.mockResolvedValue({
    success: true,
    message: '',
    data: binaryInfo,
  })
  mocks.getCurrentSystemTask.mockResolvedValue({
    success: true,
    message: '',
    data: null,
  })
  mocks.statusQueryFn.mockResolvedValue({})
})

describe('UpdateCheckerSection', () => {
  it('checks updates through the backend and displays release notes without browser GitHub requests', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    const user = userEvent.setup()

    renderSection()

    expect(await screen.findByText('v1.1.0')).toBeVisible()
    expect(fetchSpy).not.toHaveBeenCalled()
    expect(mocks.getLatestSystemUpdate).toHaveBeenCalledWith(false)

    await user.click(screen.getByRole('button', { name: 'View release notes' }))
    expect(screen.getByRole('heading', { name: 'Release notes' })).toBeVisible()
    expect(screen.getByText('Bug fixes and safer rollback.')).toBeVisible()
    await user.click(
      within(screen.getByRole('dialog')).getAllByRole('button', {
        name: 'Close',
      })[0]
    )

    await user.click(screen.getByRole('button', { name: 'Check for updates' }))
    await waitFor(() =>
      expect(mocks.getLatestSystemUpdate).toHaveBeenLastCalledWith(true)
    )
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it('requires confirmation before applying an update and polls the system task', async () => {
    const user = userEvent.setup()
    const pendingTask = {
      id: 1,
      task_id: 'task-update-1',
      type: 'system_update',
      status: 'pending' as const,
      state: taskState,
      result: {},
      error: '',
      created_at: 1,
      updated_at: 1,
    }
    let taskReadCount = 0
    mocks.applySystemUpdate.mockResolvedValue({
      success: true,
      message: '',
      data: pendingTask,
    })
    mocks.getSystemUpdateTask.mockImplementation(async () => {
      taskReadCount += 1
      if (taskReadCount === 1) {
        return {
          success: true,
          message: '',
          data: {
            ...pendingTask,
            status: 'running',
            state: { ...taskState, phase: 'downloading', progress: 42 },
          },
        }
      }
      return {
        success: true,
        message: '',
        data: {
          ...pendingTask,
          status: 'succeeded',
          state: { ...taskState, phase: 'ready', progress: 100 },
          result: {
            target_version: 'v1.1.0',
            restart_required: true,
          },
        },
      }
    })

    renderSection()
    await screen.findByText('v1.1.0')
    await user.click(screen.getByRole('button', { name: 'Apply update' }))

    expect(mocks.applySystemUpdate).not.toHaveBeenCalled()
    const dialog = await screen.findByRole('alertdialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Apply update' })
    )
    await waitFor(() => expect(mocks.applySystemUpdate).toHaveBeenCalledOnce())
    expect(await screen.findByText('42%')).toBeVisible()

    expect(
      await screen.findByText('100%', {}, { timeout: 4_000 })
    ).toBeVisible()
    expect(
      screen.getByText('Updated to v1.1.0. Restart required.')
    ).toBeVisible()
  })

  it('shows a failure state and keeps the backend error visible without exposing secrets', async () => {
    const failedTask = {
      id: 2,
      task_id: 'task-update-failed',
      type: 'system_update',
      status: 'failed' as const,
      state: { ...taskState, phase: 'verifying', progress: 75 },
      result: {},
      error: 'checksum mismatch; SESSION_SECRET=***',
      created_at: 1,
      updated_at: 1,
    }
    mocks.getCurrentSystemTask.mockResolvedValue({
      success: true,
      message: '',
      data: failedTask,
    })

    renderSection()

    expect(
      await screen.findByText('checksum mismatch; SESSION_SECRET=***')
    ).toBeVisible()
    expect(screen.queryByText('hidden-value')).not.toBeInTheDocument()
  })

  it('degrades to manual Docker instructions when the socket is unavailable', async () => {
    const dockerInfo = {
      ...binaryInfo,
      current_version: 'v1.0.0',
      latest_version: '',
      has_update: true,
      build_type: 'container',
      deployment_mode: 'container_unknown',
      update_method: 'docker_engine',
      docker_control_available: false,
      can_apply: false,
      rollback_available: false,
      release_status: 'none',
      manual_update_hint:
        'Docker deployments should update by pulling c1cadabob/nexustok:latest and recreating the container with the same mounted data directories.',
      docker: {
        socket_available: false,
        socket_path: '/var/run/docker.sock',
        manual_update_command:
          'docker pull c1cadabob/nexustok:latest\ndocker run --name nexustok',
        one_time_enable_command:
          'docker run -e SESSION_SECRET_FILE=/data/session_secret c1cadabob/nexustok:latest',
        healthcheck_available: false,
        healthcheck_degraded: false,
      },
    }
    mocks.getLatestSystemUpdate.mockResolvedValue({
      success: true,
      message: '',
      data: dockerInfo,
    })

    renderSection()

    expect(await screen.findByText('Docker socket not mounted')).toBeVisible()
    expect(screen.getByText(/docker pull c1cadabob/)).toBeVisible()
    expect(screen.getByText(/SESSION_SECRET_FILE/)).toBeVisible()
    expect(screen.queryByText('hidden-value')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Apply update' })).toBeDisabled()
  })

  it('probes the shared status query after a scheduled restart', async () => {
    const user = userEvent.setup()
    mocks.restartSystemUpdate.mockResolvedValue({
      success: true,
      message: '',
      data: {
        restart_supported: true,
        restart_scheduled: true,
        manual_required: false,
        message: 'Restart scheduled.',
      },
    })

    renderSection()
    await screen.findByText('v1.1.0')
    await user.click(screen.getByRole('button', { name: 'Restart service' }))
    const dialog = await screen.findByRole('alertdialog')
    await user.click(
      within(dialog).getByRole('button', { name: 'Restart service' })
    )
    await waitFor(() =>
      expect(mocks.restartSystemUpdate).toHaveBeenCalledOnce()
    )

    await waitFor(() => expect(mocks.statusQueryFn).toHaveBeenCalled(), {
      timeout: 4_000,
    })
  })
})
