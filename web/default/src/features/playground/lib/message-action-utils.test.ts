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
  MESSAGE_ACTION_LABELS,
  MESSAGE_ROLES,
  MESSAGE_STATUS,
} from '../constants'
import type { Message } from '../types'
import {
  canRegenerateMessage,
  canToggleMessageSource,
  getMessageActionsVisibilityClass,
  getMessageActionState,
  getMessageSourceToggleLabel,
} from './message-action-utils'

function makeMessage(overrides: Partial<Message> = {}): Message {
  return {
    key: 'message-1',
    from: MESSAGE_ROLES.ASSISTANT,
    versions: [{ id: 'version-1', content: 'answer' }],
    status: MESSAGE_STATUS.COMPLETE,
    ...overrides,
  }
}

describe('message action utils', () => {
  test('computes assistant action state from current content and status', () => {
    const state = getMessageActionState(
      makeMessage({ status: MESSAGE_STATUS.STREAMING })
    )

    assert.equal(state.content, 'answer')
    assert.equal(state.hasContent, true)
    assert.equal(state.isAssistant, true)
    assert.equal(state.isUser, false)
    assert.equal(state.isLoading, true)
  })

  test('treats whitespace-only content as empty', () => {
    const state = getMessageActionState(
      makeMessage({ versions: [{ id: 'version-1', content: '   ' }] })
    )

    assert.equal(state.content, '   ')
    assert.equal(state.hasContent, false)
  })

  test('computes user action state', () => {
    const state = getMessageActionState(
      makeMessage({ from: MESSAGE_ROLES.USER, status: undefined })
    )

    assert.equal(state.isAssistant, false)
    assert.equal(state.isUser, true)
    assert.equal(state.isLoading, false)
  })

  test('returns stable visibility classes', () => {
    assert.equal(getMessageActionsVisibilityClass(true), 'opacity-100')
    assert.equal(
      getMessageActionsVisibilityClass(false),
      'opacity-0 group-hover:opacity-100 max-md:opacity-100'
    )
  })

  test('allows source toggle only for completed assistant content with handler', () => {
    assert.equal(
      canToggleMessageSource({
        hasContent: true,
        hasToggleHandler: true,
        isAssistant: true,
        isLoading: false,
      }),
      true
    )

    assert.equal(
      canToggleMessageSource({
        hasContent: true,
        hasToggleHandler: true,
        isAssistant: true,
        isLoading: true,
      }),
      false
    )

    assert.equal(
      canToggleMessageSource({
        hasContent: true,
        hasToggleHandler: true,
        isAssistant: false,
        isLoading: false,
      }),
      false
    )

    assert.equal(
      canToggleMessageSource({
        hasContent: false,
        hasToggleHandler: true,
        isAssistant: true,
        isLoading: false,
      }),
      false
    )

    assert.equal(
      canToggleMessageSource({
        hasContent: true,
        hasToggleHandler: false,
        isAssistant: true,
        isLoading: false,
      }),
      false
    )
  })

  test('allows regenerate for completed user or assistant content with handler', () => {
    assert.equal(
      canRegenerateMessage({
        hasContent: true,
        hasRegenerateHandler: true,
        isAssistant: true,
        isLoading: false,
        isUser: false,
      }),
      true
    )

    assert.equal(
      canRegenerateMessage({
        hasContent: true,
        hasRegenerateHandler: true,
        isAssistant: false,
        isLoading: false,
        isUser: true,
      }),
      true
    )

    assert.equal(
      canRegenerateMessage({
        hasContent: true,
        hasRegenerateHandler: true,
        isAssistant: true,
        isLoading: true,
        isUser: false,
      }),
      false
    )

    assert.equal(
      canRegenerateMessage({
        hasContent: false,
        hasRegenerateHandler: true,
        isAssistant: true,
        isLoading: false,
        isUser: false,
      }),
      false
    )

    assert.equal(
      canRegenerateMessage({
        hasContent: true,
        hasRegenerateHandler: false,
        isAssistant: false,
        isLoading: false,
        isUser: true,
      }),
      false
    )

    assert.equal(
      canRegenerateMessage({
        hasContent: true,
        hasRegenerateHandler: true,
        isAssistant: false,
        isLoading: false,
        isUser: false,
      }),
      false
    )
  })

  test('returns source toggle labels for preview and raw response modes', () => {
    assert.equal(
      getMessageSourceToggleLabel(false),
      MESSAGE_ACTION_LABELS.SHOW_SOURCE
    )
    assert.equal(
      getMessageSourceToggleLabel(true),
      MESSAGE_ACTION_LABELS.SHOW_PREVIEW
    )
  })
})
