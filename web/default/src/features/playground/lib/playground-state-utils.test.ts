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
import { applyMessageStateUpdate } from './playground-state-utils'

function makeMessage(key: string): Message {
  return {
    key,
    from: MESSAGE_ROLES.USER,
    versions: [{ id: `${key}-v1`, content: key }],
  }
}

describe('playground state utils', () => {
  test('uses direct message arrays as the next state', () => {
    const next = [makeMessage('next')]

    assert.equal(applyMessageStateUpdate([makeMessage('old')], next), next)
  })

  test('applies functional message state updater', () => {
    const previous = [makeMessage('old')]
    const next = applyMessageStateUpdate(previous, (messages) => [
      ...messages,
      makeMessage('new'),
    ])

    assert.deepEqual(
      next.map((message) => message.key),
      ['old', 'new']
    )
  })
})
