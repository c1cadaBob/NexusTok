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
import {
  isMessageSourceVisible,
  toggleMessageSourceKey,
} from './message-source-utils'

describe('message source utils', () => {
  test('adds a message key without mutating the current set', () => {
    const current = new Set(['message-1'])
    const next = toggleMessageSourceKey(current, 'message-2')

    assert.notEqual(next, current)
    assert.deepEqual(Array.from(next).sort(), ['message-1', 'message-2'])
    assert.deepEqual(Array.from(current), ['message-1'])
  })

  test('removes an existing message key', () => {
    const current = new Set(['message-1', 'message-2'])
    const next = toggleMessageSourceKey(current, 'message-1')

    assert.deepEqual(Array.from(next), ['message-2'])
    assert.equal(isMessageSourceVisible(next, 'message-1'), false)
    assert.equal(isMessageSourceVisible(next, 'message-2'), true)
  })
})
