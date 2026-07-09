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
import { MESSAGE_ROLES } from '../constants'
import type { Message } from '../types'
import {
  getMessageAlignment,
  getMessageAlignmentClass,
} from './message-layout-utils'

function makeMessage(from: Message['from']): Message {
  return {
    key: `${from}-message`,
    from,
    versions: [{ id: `${from}-version`, content: 'content' }],
  }
}

describe('message layout utils', () => {
  test('right-aligns user messages in alternating layout', () => {
    assert.equal(
      getMessageAlignment(makeMessage(MESSAGE_ROLES.USER), 'alternating'),
      'right'
    )
  })

  test('left-aligns assistant and system messages in alternating layout', () => {
    assert.equal(
      getMessageAlignment(makeMessage(MESSAGE_ROLES.ASSISTANT), 'alternating'),
      'left'
    )
    assert.equal(
      getMessageAlignment(makeMessage(MESSAGE_ROLES.SYSTEM), 'alternating'),
      'left'
    )
  })

  test('left-aligns every role in left layout', () => {
    assert.equal(
      getMessageAlignment(makeMessage(MESSAGE_ROLES.USER), 'left'),
      'left'
    )
    assert.equal(
      getMessageAlignment(makeMessage(MESSAGE_ROLES.ASSISTANT), 'left'),
      'left'
    )
    assert.equal(
      getMessageAlignment(makeMessage(MESSAGE_ROLES.SYSTEM), 'left'),
      'left'
    )
  })

  test('returns stable alignment classes', () => {
    assert.equal(getMessageAlignmentClass('right'), 'items-end text-right')
    assert.equal(getMessageAlignmentClass('left'), 'items-start text-left')
  })
})
