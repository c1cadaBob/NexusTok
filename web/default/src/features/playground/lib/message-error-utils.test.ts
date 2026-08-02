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
import { MESSAGE_ROLES, MESSAGE_STATUS } from '../constants'
import type { Message } from '../types'
import {
  FALLBACK_ERROR_CONTENT,
  getMessageErrorState,
  isAdminRole,
  isErrorMessage,
} from './message-error-utils'

function makeMessage(overrides: Partial<Message> = {}): Message {
  return {
    key: 'message-1',
    from: MESSAGE_ROLES.ASSISTANT,
    status: MESSAGE_STATUS.ERROR,
    versions: [{ id: 'version-1', content: 'Request error occurred: failed' }],
    ...overrides,
  }
}

describe('message error utils', () => {
  test('detects admin roles using the current frontend role threshold', () => {
    assert.equal(isAdminRole(10), true)
    assert.equal(isAdminRole(99), true)
    assert.equal(isAdminRole(1), false)
    assert.equal(isAdminRole(null), false)
    assert.equal(isAdminRole(undefined), false)
  })

  test('returns null for non-error messages', () => {
    const message = makeMessage({ status: MESSAGE_STATUS.COMPLETE })

    assert.equal(isErrorMessage(message), false)
    assert.equal(getMessageErrorState(message, true), null)
  })

  test('builds generic error state from message content', () => {
    const state = getMessageErrorState(makeMessage(), true)

    assert.deepEqual(state, {
      content: 'Request error occurred: failed',
      kind: 'generic',
      showSettingsLink: false,
    })
  })

  test('uses fallback content for empty error messages', () => {
    const state = getMessageErrorState(
      makeMessage({ versions: [{ id: 'version-1', content: '' }] }),
      true
    )

    assert.equal(state?.content, FALLBACK_ERROR_CONTENT)
  })

  test('shows model page pricing link only for admins', () => {
    const message = makeMessage({ errorCode: 'model_price_error' })
    const adminState = getMessageErrorState(message, true)
    const userState = getMessageErrorState(message, false)

    assert.equal(adminState?.kind, 'model-price')
    assert.equal(adminState?.showSettingsLink, true)
    assert.equal(userState?.kind, 'model-price')
    assert.equal(userState?.showSettingsLink, false)
  })

  test('treats unknown error codes as generic errors', () => {
    const state = getMessageErrorState(
      makeMessage({ errorCode: 'upstream_timeout' }),
      true
    )

    assert.equal(state?.kind, 'generic')
    assert.equal(state?.showSettingsLink, false)
  })
})
