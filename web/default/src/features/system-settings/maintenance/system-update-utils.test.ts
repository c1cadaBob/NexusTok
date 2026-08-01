import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { SystemTask } from '@/features/system-info/types'
import {
  getSystemUpdatePhaseLabel,
  getSystemUpdateProgress,
  getSystemUpdateTaskSummary,
  isSystemUpdateTask,
} from './system-update-utils'

describe('system update utilities', () => {
  test('maps known task phases to display keys', () => {
    assert.equal(getSystemUpdatePhaseLabel('downloading'), 'Downloading binary')
    assert.equal(getSystemUpdatePhaseLabel('verifying'), 'Verifying checksum')
    assert.equal(getSystemUpdatePhaseLabel(), 'Waiting to start')
    assert.equal(getSystemUpdatePhaseLabel('custom_phase'), 'custom_phase')
  })

  test('clamps task progress to the progress bar range', () => {
    assert.equal(
      getSystemUpdateProgress(makeTask({ state: { progress: -10 } })),
      0
    )
    assert.equal(
      getSystemUpdateProgress(makeTask({ state: { progress: 42 } })),
      42
    )
    assert.equal(
      getSystemUpdateProgress(makeTask({ state: { progress: 120 } })),
      100
    )
    assert.equal(getSystemUpdateProgress(makeTask({ state: {} })), null)
  })

  test('detects update task types', () => {
    assert.equal(isSystemUpdateTask(makeTask({ type: 'system_update' })), true)
    assert.equal(
      isSystemUpdateTask(makeTask({ type: 'system_rollback' })),
      true
    )
    assert.equal(isSystemUpdateTask(makeTask({ type: 'log_cleanup' })), false)
  })

  test('builds terminal summaries for update and rollback tasks', () => {
    assert.equal(
      getSystemUpdateTaskSummary(
        makeTask({
          type: 'system_update',
          status: 'succeeded',
          result: { target_version: 'v1.2.3' },
        })
      ),
      'Updated to {{version}}. Restart required.'
    )
    assert.equal(
      getSystemUpdateTaskSummary(
        makeTask({ type: 'system_rollback', status: 'succeeded' })
      ),
      'Rollback applied. Restart required.'
    )
    assert.equal(
      getSystemUpdateTaskSummary(
        makeTask({ status: 'running', state: { phase: 'backing_up' } })
      ),
      'Backing up current binary'
    )
  })
})

function makeTask(overrides: Partial<SystemTask> = {}): SystemTask {
  return {
    id: 1,
    task_id: 'systask_test',
    type: 'system_update',
    status: 'running',
    payload: {},
    state: {},
    result: {},
    error: '',
    locked_by: '',
    created_at: 1,
    updated_at: 1,
    ...overrides,
  }
}
