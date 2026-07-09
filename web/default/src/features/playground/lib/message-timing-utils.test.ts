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
  completeAssistantTiming,
  completeReasoningTiming,
  formatDuration,
  formatMessageTime,
  startReasoningTiming,
} from './message-timing-utils'

function makeMessage(overrides: Partial<Message> = {}): Message {
  return {
    key: 'message-1',
    from: MESSAGE_ROLES.ASSISTANT,
    versions: [{ id: 'version-1', content: 'answer' }],
    ...overrides,
  }
}

function t(
  key: string,
  options?: Record<string, string | number>
): string {
  if (key === '{{value}}ms') {
    return `${options?.value}ms`
  }

  if (key === '{{value}}s') {
    return `${options?.value}s`
  }

  return key
}

describe('message timing utils', () => {
  test('calculates assistant duration from startedAt', () => {
    const message = makeMessage({ startedAt: 1000 })
    const completed = completeAssistantTiming(message, 2500)

    assert.equal(completed.completedAt, 2500)
    assert.equal(completed.durationMs, 1500)
  })

  test('keeps non-assistant messages unchanged', () => {
    const message = makeMessage({ from: MESSAGE_ROLES.USER })

    assert.equal(completeAssistantTiming(message, 2500), message)
  })

  test('falls back to createdAt and completedAt without negative duration', () => {
    const createdOnly = completeAssistantTiming(
      makeMessage({ createdAt: 2000 }),
      2500
    )
    const missingTiming = completeAssistantTiming(makeMessage(), 2500)
    const backwardsClock = completeAssistantTiming(
      makeMessage({ startedAt: 3000 }),
      2500
    )

    assert.equal(createdOnly.startedAt, 2000)
    assert.equal(createdOnly.durationMs, 500)
    assert.equal(missingTiming.startedAt, 2500)
    assert.equal(missingTiming.durationMs, 0)
    assert.equal(backwardsClock.durationMs, 0)
  })

  test('starts reasoning once and preserves existing timing', () => {
    const first = startReasoningTiming(makeMessage(), 1000)
    const second = startReasoningTiming(
      makeMessage({
        reasoning: {
          content: 'thinking',
          duration: 0,
          startedAt: first.startedAt,
        },
      }),
      3000
    )

    assert.equal(first.startedAt, 1000)
    assert.equal(second.startedAt, 1000)
    assert.equal(second.content, 'thinking')
  })

  test('completes reasoning only once', () => {
    const message = makeMessage({
      reasoning: {
        content: 'thinking',
        duration: 0,
        startedAt: 1000,
      },
    })
    const completed = completeReasoningTiming(message, 2600)
    const completedAgain = completeReasoningTiming(completed, 4000)

    assert.equal(completed.reasoning?.completedAt, 2600)
    assert.equal(completed.reasoning?.durationMs, 1600)
    assert.equal(completed.reasoning?.duration, 2)
    assert.equal(completedAgain.reasoning?.completedAt, 2600)
  })

  test('formats duration with millisecond and second units', () => {
    assert.equal(formatDuration(12.3, t), '12ms')
    assert.equal(formatDuration(0, t), '1ms')
    assert.equal(formatDuration(1500, t), '1.50s')
    assert.equal(formatDuration(undefined, t), undefined)
    assert.equal(formatDuration(Number.NaN, t), undefined)
  })

  test('formats message time only for finite timestamps', () => {
    assert.equal(formatMessageTime(undefined), undefined)
    assert.equal(formatMessageTime(Number.NaN), undefined)
    assert.equal(formatMessageTime(Number.MAX_VALUE), undefined)
    assert.equal(typeof formatMessageTime(1720000000000), 'string')
  })
})
