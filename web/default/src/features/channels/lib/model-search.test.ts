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
import { getMissingModelSearchMatches } from './model-search'

describe('渠道模型搜索缺失项计算', () => {
  test('只返回搜索命中但渠道尚未包含的模型', () => {
    assert.deepEqual(
      getMissingModelSearchMatches(
        ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol'],
        ['gpt-5.4', 'gpt-5.5', 'gpt-5.6-sol']
      ),
      ['gpt-5.6-terra', 'gpt-5.6-luna']
    )
  })

  test('按大小写不敏感方式识别已存在模型', () => {
    assert.deepEqual(
      getMissingModelSearchMatches(
        ['GPT-5.6-Terra', 'gpt-5.6-luna'],
        ['gpt-5.6-terra']
      ),
      ['gpt-5.6-luna']
    )
  })

  test('搜索结果本身会去重并忽略空白项', () => {
    assert.deepEqual(
      getMissingModelSearchMatches(
        [' gpt-5.6-terra ', 'GPT-5.6-TERRA', '', 'gpt-5.6-luna'],
        []
      ),
      ['gpt-5.6-terra', 'gpt-5.6-luna']
    )
  })
})
