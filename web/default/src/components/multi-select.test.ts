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
import { filterMultiSelectItems } from './multi-select'

describe('MultiSelect 搜索过滤', () => {
  test('空搜索保持原候选顺序', () => {
    const items = ['gpt-4o', 'gpt-5.6-terra', 'claude-sonnet']

    assert.deepEqual(filterMultiSelectItems(items, ''), items)
  })

  test('按模型名过滤远程同步候选', () => {
    const items = [
      'gpt-5.6-terra',
      'gpt-5.6-luna',
      'gpt-5.6-sol',
      'gpt-5.4',
      'gpt-5.5',
    ]

    assert.deepEqual(filterMultiSelectItems(items, 'gpt-5.6'), [
      'gpt-5.6-terra',
      'gpt-5.6-luna',
      'gpt-5.6-sol',
    ])
  })

  test('兼容 label 命中', () => {
    const items = ['model-a', 'model-b']
    const labels = new Map([
      ['model-a', 'GPT-5.6 Terra'],
      ['model-b', 'Claude Sonnet'],
    ])

    assert.deepEqual(filterMultiSelectItems(items, 'terra', labels), [
      'model-a',
    ])
  })
})
