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
import type { ChatCompletionResponse, Message } from '../types'
import {
  applyChatCompletionResponse,
  applyStreamingChunk,
  completeAssistantMessage,
  isAssistantMessageFinal,
  isAssistantMessagePending,
  isPendingAssistantMessage,
} from './message-streaming-utils'

function makeAssistant(overrides: Partial<Message> = {}): Message {
  return {
    key: 'assistant-1',
    from: MESSAGE_ROLES.ASSISTANT,
    versions: [{ id: 'version-1', content: '' }],
    startedAt: 1000,
    status: MESSAGE_STATUS.LOADING,
    ...overrides,
  }
}

function makeChatResponse(
  content: string,
  reasoningContent?: string
): ChatCompletionResponse {
  return {
    id: 'chatcmpl-1',
    object: 'chat.completion',
    created: 1,
    model: 'gpt-test',
    choices: [
      {
        index: 0,
        message: {
          role: MESSAGE_ROLES.ASSISTANT,
          content,
          reasoning_content: reasoningContent,
        },
        finish_reason: 'stop',
      },
    ],
  }
}

describe('message streaming utils', () => {
  test('appends reasoning chunks and marks message streaming', () => {
    const message = makeAssistant({
      reasoning: { content: 'plan', duration: 0, startedAt: 1100 },
    })
    const next = applyStreamingChunk(message, 'reasoning', ' next')

    assert.equal(next.reasoning?.content, 'plan next')
    assert.equal(next.isReasoningStreaming, true)
    assert.equal(next.status, MESSAGE_STATUS.STREAMING)
  })

  test('deduplicates cumulative reasoning chunks', () => {
    const message = makeAssistant({
      reasoning: { content: 'plan', duration: 0, startedAt: 1100 },
    })
    const next = applyStreamingChunk(message, 'reasoning', 'plan next')

    assert.equal(next.reasoning?.content, 'plan next')
  })

  test('deduplicates cumulative content chunks', () => {
    const message = makeAssistant({
      status: MESSAGE_STATUS.STREAMING,
      versions: [{ id: 'version-1', content: 'hello' }],
    })
    const next = applyStreamingChunk(message, 'content', 'hello world')

    assert.equal(next.versions[0].content, 'hello world')
  })

  test('keeps error messages unchanged during streaming updates', () => {
    const message = makeAssistant({
      status: MESSAGE_STATUS.ERROR,
      versions: [{ id: 'version-1', content: 'failed' }],
    })

    assert.equal(applyStreamingChunk(message, 'content', 'ignored'), message)
  })

  test('extracts reasoning from think tags and completes reasoning timing', () => {
    const next = applyStreamingChunk(
      makeAssistant({ status: MESSAGE_STATUS.STREAMING }),
      'content',
      '<think>hidden</think>visible'
    )

    assert.equal(next.versions[0].content, '<think>hidden</think>visible')
    assert.equal(next.reasoning?.content, 'hidden')
    assert.equal(next.isReasoningStreaming, false)
    assert.equal(typeof next.reasoning?.durationMs, 'number')
  })

  test('completes assistant messages and removes visible think tags', () => {
    const next = completeAssistantMessage(
      makeAssistant({
        status: MESSAGE_STATUS.STREAMING,
        versions: [{ id: 'version-1', content: '<think>hidden</think>answer' }],
      })
    )

    assert.equal(next.status, MESSAGE_STATUS.COMPLETE)
    assert.equal(next.versions[0].content, 'answer')
    assert.equal(next.reasoning?.content, 'hidden')
    assert.equal(typeof next.durationMs, 'number')
  })

  test('applies non-streaming chat completion response', () => {
    const next = applyChatCompletionResponse(
      makeAssistant(),
      makeChatResponse('answer', 'why')
    )

    assert.ok(next)
    assert.equal(next.status, MESSAGE_STATUS.COMPLETE)
    assert.equal(next.versions[0].content, 'answer')
    assert.equal(next.reasoning?.content, 'why')
  })

  test('returns null for non-streaming responses without choices', () => {
    const response = {
      ...makeChatResponse('answer'),
      choices: [],
    }

    assert.equal(applyChatCompletionResponse(makeAssistant(), response), null)
  })

  test('classifies assistant message lifecycle states', () => {
    assert.equal(
      isAssistantMessageFinal(
        makeAssistant({ status: MESSAGE_STATUS.COMPLETE })
      ),
      true
    )
    assert.equal(
      isAssistantMessagePending(
        makeAssistant({ status: MESSAGE_STATUS.LOADING })
      ),
      true
    )
    assert.equal(
      isPendingAssistantMessage(
        makeAssistant({ status: MESSAGE_STATUS.STREAMING })
      ),
      true
    )
    assert.equal(
      isPendingAssistantMessage({
        key: 'user-1',
        from: MESSAGE_ROLES.USER,
        versions: [{ id: 'user-v1', content: 'prompt' }],
      }),
      false
    )
  })
})
