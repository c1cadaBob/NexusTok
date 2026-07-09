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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { GroupOption, ModelOption } from '../types'
import {
  getGroupFallback,
  getModelFallback,
  getOptionLoadErrorMessage,
  shouldClearModelForGroup,
} from './playground-option-utils'

const models: ModelOption[] = [
  { label: 'gpt-4o', value: 'gpt-4o' },
  { label: 'claude-sonnet-4', value: 'claude-sonnet-4' },
]

const groups: GroupOption[] = [
  { label: 'vip', value: 'vip', ratio: 2 },
  { label: 'default', value: 'default', ratio: 1 },
]

describe('playground option utils', () => {
  test('keeps current model when it is available', () => {
    assert.equal(getModelFallback(models, 'gpt-4o'), null)
  })

  test('falls back to the first available model', () => {
    assert.equal(getModelFallback(models, 'missing-model'), 'gpt-4o')
  })

  test('does not fallback when model list is empty', () => {
    assert.equal(getModelFallback([], 'gpt-4o'), null)
  })

  test('clears stale model only when no model in group matches', () => {
    assert.equal(shouldClearModelForGroup([], 'gpt-4o'), true)
    assert.equal(shouldClearModelForGroup(models, 'gpt-4o'), false)
    assert.equal(shouldClearModelForGroup([], ''), false)
  })

  test('falls back to default group before first group', () => {
    assert.equal(getGroupFallback(groups, 'missing-group'), 'default')
    assert.equal(getGroupFallback(groups, 'vip'), null)
  })

  test('falls back to the first group when default is unavailable', () => {
    assert.equal(
      getGroupFallback([{ label: 'vip', value: 'vip', ratio: 2 }], 'missing'),
      'vip'
    )
  })

  test('uses error message when available', () => {
    assert.equal(
      getOptionLoadErrorMessage(new Error('network failed'), 'fallback'),
      'network failed'
    )
    assert.equal(getOptionLoadErrorMessage('bad', 'fallback'), 'fallback')
  })
})
