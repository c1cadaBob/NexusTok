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
import { parseThinkTags } from './message-reasoning-utils'

describe('message reasoning utils', () => {
  test('keeps content without think tags unchanged', () => {
    assert.deepEqual(parseThinkTags('plain answer'), {
      visibleContent: 'plain answer',
      reasoning: '',
      hasUnclosedTag: false,
    })
  })

  test('extracts a complete think block from assistant content', () => {
    assert.deepEqual(parseThinkTags('before <think>hidden</think> after'), {
      visibleContent: 'before  after',
      reasoning: 'hidden',
      hasUnclosedTag: false,
    })
  })

  test('extracts multiple think blocks in order', () => {
    assert.deepEqual(
      parseThinkTags('<think>a</think> visible <think>b</think>'),
      {
        visibleContent: 'visible',
        reasoning: 'a\n\nb',
        hasUnclosedTag: false,
      }
    )
  })

  test('marks unclosed think block as streaming reasoning', () => {
    assert.deepEqual(parseThinkTags('visible <think>still thinking'), {
      visibleContent: 'visible',
      reasoning: 'still thinking',
      hasUnclosedTag: true,
    })
  })
})
