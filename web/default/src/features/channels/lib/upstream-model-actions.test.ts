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
import { resolveUpstreamModelApplyResult } from './upstream-model-actions'

describe('上游模型动作解析', () => {
  test('没有上游模型时返回 empty，按钮由组件保持可点击并提示原因', () => {
    assert.deepEqual(resolveUpstreamModelApplyResult([], ['gpt-5.5']), {
      status: 'empty',
      models: [],
      count: 0,
    })
  })

  test('上游模型与当前选择一致时返回 same，避免用户感觉点击无反馈', () => {
    assert.deepEqual(
      resolveUpstreamModelApplyResult(['GPT-5.5', 'claude-3.7'], [
        'claude-3.7',
        'gpt-5.5',
      ]),
      {
        status: 'same',
        models: ['GPT-5.5', 'claude-3.7'],
        count: 2,
      }
    )
  })

  test('上游模型与当前选择不一致时返回可回填模型列表', () => {
    assert.deepEqual(
      resolveUpstreamModelApplyResult(['gpt-5.5', 'gpt-5.5', ' qwen3 '], [
        'gpt-4o',
      ]),
      {
        status: 'applied',
        models: ['gpt-5.5', 'qwen3'],
        count: 2,
      }
    )
  })
})
