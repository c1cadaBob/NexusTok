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
import { ERROR_MESSAGES, MESSAGE_ROLES, MESSAGE_STATUS } from '../constants'
import type { Message } from '../types'
import {
  updateAssistantMessageWithError,
  updateLastAssistantMessage,
} from './message-update-utils'

function makeMessage(overrides: Partial<Message> = {}): Message {
  return {
    key: 'message-1',
    from: MESSAGE_ROLES.ASSISTANT,
    versions: [{ id: 'version-1', content: 'answer' }],
    ...overrides,
  }
}

describe('message update utils', () => {
  test('updates the last assistant message only', () => {
    const user = makeMessage({
      key: 'user-1',
      from: MESSAGE_ROLES.USER,
      versions: [{ id: 'user-v1', content: 'prompt' }],
    })
    const assistant = makeMessage()
    const messages = [user, assistant]
    const next = updateLastAssistantMessage(messages, (message) => ({
      ...message,
      status: MESSAGE_STATUS.COMPLETE,
    }))

    assert.notEqual(next, messages)
    assert.equal(next[0], user)
    assert.equal(next[1].status, MESSAGE_STATUS.COMPLETE)
  })

  test('preserves array when there is no trailing assistant', () => {
    const messages = [
      makeMessage({
        from: MESSAGE_ROLES.USER,
        versions: [{ id: 'user-v1', content: 'prompt' }],
      }),
    ]

    assert.equal(
      updateLastAssistantMessage(messages, (message) => message),
      messages
    )
  })

  test('marks assistant error and completes timing', () => {
    const messages = [
      makeMessage({
        startedAt: 1000,
        reasoning: {
          content: 'thinking',
          duration: 0,
          startedAt: 1200,
        },
        status: MESSAGE_STATUS.STREAMING,
      }),
    ]

    const [failed] = updateAssistantMessageWithError(
      messages,
      'network failed',
      'network_error'
    )

    assert.equal(
      failed.versions[0].content,
      `${ERROR_MESSAGES.API_REQUEST_ERROR}: network failed`
    )
    assert.equal(failed.status, MESSAGE_STATUS.ERROR)
    assert.equal(failed.errorCode, 'network_error')
    assert.equal(failed.isReasoningStreaming, false)
    assert.equal(typeof failed.completedAt, 'number')
    assert.equal(typeof failed.durationMs, 'number')
    assert.equal(typeof failed.reasoning?.durationMs, 'number')
  })

  test('allows custom error title', () => {
    const [failed] = updateAssistantMessageWithError(
      [makeMessage()],
      'bad request',
      undefined,
      'Custom title'
    )

    assert.equal(failed.versions[0].content, 'Custom title: bad request')
  })
})
