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
import { getMessageEditorState } from './message-editor-utils'

function makeMessage(from: Message['from']): Message {
  return {
    key: `${from}-message`,
    from,
    versions: [{ id: 'version-1', content: 'original' }],
  }
}

describe('message editor utils', () => {
  test('allows saving changed non-empty user content and submit action', () => {
    const state = getMessageEditorState(
      makeMessage(MESSAGE_ROLES.USER),
      'updated prompt',
      'original prompt'
    )

    assert.equal(state.hasChanged, true)
    assert.equal(state.canSave, true)
    assert.equal(state.showSaveAndSubmit, true)
  })

  test('keeps assistant edits local without submit action', () => {
    const state = getMessageEditorState(
      makeMessage(MESSAGE_ROLES.ASSISTANT),
      'updated answer',
      'original answer'
    )

    assert.equal(state.hasChanged, true)
    assert.equal(state.canSave, true)
    assert.equal(state.showSaveAndSubmit, false)
  })

  test('disables save when content has not changed', () => {
    const state = getMessageEditorState(
      makeMessage(MESSAGE_ROLES.USER),
      'same content',
      'same content'
    )

    assert.equal(state.hasChanged, false)
    assert.equal(state.canSave, false)
    assert.equal(state.showSaveAndSubmit, true)
  })

  test('tracks changed blank content but refuses saving it', () => {
    const state = getMessageEditorState(
      makeMessage(MESSAGE_ROLES.USER),
      '   ',
      'original prompt'
    )

    assert.equal(state.hasChanged, true)
    assert.equal(state.canSave, false)
    assert.equal(state.showSaveAndSubmit, true)
  })
})
