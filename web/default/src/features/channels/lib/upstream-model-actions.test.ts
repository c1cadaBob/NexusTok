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
import { resolveUpstreamModelFetchResult } from './upstream-model-actions'

describe('上游模型动作解析', () => {
  test('没有上游模型时返回 empty，弹窗显示空结果而不是已应用', () => {
    assert.deepEqual(resolveUpstreamModelFetchResult([]), {
      status: 'empty',
      models: [],
      count: 0,
    })
  })

  test('上游返回模型只做去重整理，不比较当前已选模型', () => {
    assert.deepEqual(
      resolveUpstreamModelFetchResult(['gpt-5.5', 'gpt-5.5', ' qwen3 ']),
      {
        status: 'fetched',
        models: ['gpt-5.5', 'qwen3'],
        count: 2,
      }
    )
  })
})
