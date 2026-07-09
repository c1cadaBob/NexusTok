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
import { getMessageContentState } from './message-content-utils'

function makeMessage(overrides: Partial<Message> = {}): Message {
  return {
    key: 'message-1',
    from: MESSAGE_ROLES.ASSISTANT,
    status: MESSAGE_STATUS.COMPLETE,
    versions: [{ id: 'version-1', content: '' }],
    ...overrides,
  }
}

describe('message content utils', () => {
  test('keeps user content unchanged', () => {
    const message = makeMessage({ from: MESSAGE_ROLES.USER })
    const state = getMessageContentState(message, 'hello <think>kept</think>')

    assert.equal(state.displayContent, 'hello <think>kept</think>')
    assert.equal(state.showMessageContent, true)
    assert.equal(state.isMessageFinal, true)
  })

  test('removes assistant think tags from visible content', () => {
    const message = makeMessage()
    const state = getMessageContentState(
      message,
      '<think>hidden reasoning</think>visible answer'
    )

    assert.equal(state.displayContent, 'visible answer')
    assert.equal(state.hasReasoning, false)
  })

  test('exposes assistant reasoning content when present', () => {
    const message = makeMessage({
      reasoning: { content: 'reasoning buffer', duration: 0 },
    })
    const state = getMessageContentState(message, 'visible')

    assert.equal(state.hasReasoning, true)
    assert.equal(state.reasoningContent, 'reasoning buffer')
  })

  test('shows loader for empty loading assistant message', () => {
    const message = makeMessage({ status: MESSAGE_STATUS.LOADING })
    const state = getMessageContentState(message, '')

    assert.equal(state.showLoader, true)
    assert.equal(state.showMessageContent, false)
    assert.equal(state.isMessageFinal, false)
  })

  test('does not show loader while reasoning is streaming', () => {
    const message = makeMessage({
      isReasoningStreaming: true,
      status: MESSAGE_STATUS.STREAMING,
    })
    const state = getMessageContentState(message, '')

    assert.equal(state.showLoader, false)
    assert.equal(state.showMessageContent, false)
  })

  test('tracks sources and error state', () => {
    const message = makeMessage({
      sources: [{ href: 'https://example.com', title: 'Example' }],
      status: MESSAGE_STATUS.ERROR,
    })
    const state = getMessageContentState(message, 'failed')

    assert.equal(state.hasSources, true)
    assert.equal(state.sources[0].title, 'Example')
    assert.equal(state.isError, true)
    assert.equal(state.isMessageFinal, true)
  })
})
