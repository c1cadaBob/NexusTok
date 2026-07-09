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
  appendUserMessagePair,
  applyMessageEdit,
  createRegeneratedMessages,
  getEditingMessageContent,
  getPreviousUserMessage,
  removeMessageByKey,
} from './conversation-message-utils'

function makeMessage(
  key: string,
  from: Message['from'],
  content: string,
  status?: Message['status']
): Message {
  return {
    key,
    from,
    status,
    versions: [{ id: `${key}-v1`, content }],
  }
}

describe('conversation message utils', () => {
  test('appends user message and loading assistant placeholder', () => {
    const messages = [makeMessage('a1', MESSAGE_ROLES.ASSISTANT, 'hello')]
    const next = appendUserMessagePair(messages, 'new prompt')

    assert.equal(next.length, 3)
    assert.equal(next[1].from, MESSAGE_ROLES.USER)
    assert.equal(next[1].versions[0].content, 'new prompt')
    assert.equal(next[2].from, MESSAGE_ROLES.ASSISTANT)
    assert.equal(next[2].status, MESSAGE_STATUS.LOADING)
    assert.equal(next[1].createdAt, next[2].startedAt)
    assert.equal(next[2].createdAt, next[2].startedAt)
  })

  test('regenerates assistant messages by replacing the assistant branch', () => {
    const messages = [
      makeMessage('u1', MESSAGE_ROLES.USER, 'prompt'),
      makeMessage('a1', MESSAGE_ROLES.ASSISTANT, 'old answer'),
      makeMessage('u2', MESSAGE_ROLES.USER, 'later prompt'),
    ]

    const next = createRegeneratedMessages(messages, 'a1')

    assert.ok(next)
    assert.equal(next.length, 2)
    assert.equal(next[0].key, 'u1')
    assert.equal(next[1].from, MESSAGE_ROLES.ASSISTANT)
    assert.equal(next[1].status, MESSAGE_STATUS.LOADING)
  })

  test('regenerates user messages by keeping the selected prompt', () => {
    const messages = [
      makeMessage('u1', MESSAGE_ROLES.USER, 'prompt'),
      makeMessage('a1', MESSAGE_ROLES.ASSISTANT, 'old answer'),
    ]

    const next = createRegeneratedMessages(messages, 'u1')

    assert.ok(next)
    assert.equal(next.length, 2)
    assert.equal(next[0].key, 'u1')
    assert.equal(next[1].from, MESSAGE_ROLES.ASSISTANT)
    assert.equal(next[1].status, MESSAGE_STATUS.LOADING)
  })

  test('finds previous user prompt for an assistant error', () => {
    const messages = [
      makeMessage('sys', MESSAGE_ROLES.SYSTEM, 'system'),
      makeMessage('u1', MESSAGE_ROLES.USER, 'prompt'),
      makeMessage('a1', MESSAGE_ROLES.ASSISTANT, 'error', 'error'),
    ]

    assert.equal(getPreviousUserMessage(messages, 2)?.key, 'u1')
    assert.equal(getPreviousUserMessage(messages, 1), null)
  })

  test('applies edit without submit', () => {
    const messages = [
      makeMessage('u1', MESSAGE_ROLES.USER, 'old prompt'),
      makeMessage('a1', MESSAGE_ROLES.ASSISTANT, 'old answer'),
    ]

    const result = applyMessageEdit(messages, 'u1', 'new prompt', false)

    assert.ok(result)
    assert.equal(result.shouldSend, false)
    assert.equal(result.messages.length, 2)
    assert.equal(result.messages[0].versions[0].content, 'new prompt')
    assert.equal(result.messages[1].key, 'a1')
  })

  test('submits edited user prompt and drops stale following messages', () => {
    const messages = [
      makeMessage('u1', MESSAGE_ROLES.USER, 'old prompt'),
      makeMessage('a1', MESSAGE_ROLES.ASSISTANT, 'old answer'),
      makeMessage('u2', MESSAGE_ROLES.USER, 'later prompt'),
    ]

    const result = applyMessageEdit(messages, 'u1', 'new prompt', true)

    assert.ok(result)
    assert.equal(result.shouldSend, true)
    assert.equal(result.messages.length, 2)
    assert.equal(result.messages[0].key, 'u1')
    assert.equal(result.messages[0].versions[0].content, 'new prompt')
    assert.equal(result.messages[1].from, MESSAGE_ROLES.ASSISTANT)
    assert.equal(result.messages[1].status, MESSAGE_STATUS.LOADING)
    assert.equal(result.messages[0].createdAt, result.messages[1].startedAt)
  })

  test('does not submit edited assistant messages', () => {
    const messages = [
      makeMessage('u1', MESSAGE_ROLES.USER, 'prompt'),
      makeMessage('a1', MESSAGE_ROLES.ASSISTANT, 'old answer'),
    ]

    const result = applyMessageEdit(messages, 'a1', 'new answer', true)

    assert.ok(result)
    assert.equal(result.shouldSend, false)
    assert.equal(result.messages.length, 2)
    assert.equal(result.messages[1].versions[0].content, 'new answer')
  })

  test('reads editing content and removes messages by key', () => {
    const messages = [
      makeMessage('u1', MESSAGE_ROLES.USER, 'prompt'),
      makeMessage('a1', MESSAGE_ROLES.ASSISTANT, 'answer'),
    ]

    assert.equal(getEditingMessageContent(messages, 'a1'), 'answer')
    assert.equal(getEditingMessageContent(messages, null), '')
    assert.deepEqual(
      removeMessageByKey(messages, 'u1').map((message) => message.key),
      ['a1']
    )
  })
})
