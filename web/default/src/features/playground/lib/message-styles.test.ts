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
  getMessageContentStyles,
  getMessageEditorStyles,
} from './message-styles'

function getMessageContentClassSet() {
  return new Set(getMessageContentStyles().split(/\s+/))
}

function getMessageEditorClassSet() {
  return new Set(getMessageEditorStyles().split(/\s+/))
}

describe('message styles', () => {
  test('limits assistant content to a readable document column', () => {
    const classes = getMessageContentClassSet()

    assert.equal(classes.has('group-[.is-assistant]:w-full'), true)
    assert.equal(classes.has('group-[.is-assistant]:max-w-[78ch]'), true)
    assert.equal(classes.has('group-[.is-assistant]:max-w-none'), false)
  })

  test('uses the current UI font axis for assistant responses', () => {
    const classes = getMessageContentClassSet()

    assert.equal(classes.has('group-[.is-assistant]:font-sans'), true)
    assert.equal(classes.has('group-[.is-assistant]:font-serif'), false)
  })

  test('keeps user messages as compact semantic muted bubbles', () => {
    const classes = getMessageContentClassSet()

    assert.equal(classes.has('group-[.is-user]:rounded-2xl'), true)
    assert.equal(classes.has('group-[.is-user]:rounded-br-md'), true)
    assert.equal(classes.has('group-[.is-user]:border'), true)
    assert.equal(classes.has('group-[.is-user]:border-border/70'), true)
    assert.equal(classes.has('group-[.is-user]:bg-muted/70'), true)
    assert.equal(classes.has('group-[.is-user]:bg-secondary'), false)
    assert.equal(classes.has('dark:group-[.is-user]:bg-muted'), false)
  })

  test('sets stable readable type scale and wrapping', () => {
    const classes = getMessageContentClassSet()

    assert.equal(classes.has('text-[0.95rem]'), true)
    assert.equal(classes.has('leading-6'), true)
    assert.equal(classes.has('sm:text-[0.975rem]'), true)
    assert.equal(classes.has('sm:leading-7'), true)
    assert.equal(classes.has('break-words'), true)
    assert.equal(classes.has('whitespace-pre-wrap'), true)
  })

  test('keeps editor width aligned with message reading columns', () => {
    const classes = getMessageEditorClassSet()

    assert.equal(classes.has('w-full'), true)
    assert.equal(classes.has('group-[.is-assistant]:max-w-[78ch]'), true)
    assert.equal(classes.has('group-[.is-user]:max-w-[85%]'), true)
    assert.equal(classes.has('sm:group-[.is-user]:max-w-[62ch]'), true)
    assert.equal(classes.has('md:group-[.is-user]:max-w-[68ch]'), true)
    assert.equal(classes.has('lg:group-[.is-user]:max-w-[72ch]'), true)
  })
})
