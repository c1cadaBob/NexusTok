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
  getDuplicateSources,
  modelMappingRowsToJson,
  parseModelMappingJson,
} from './model-mapping-editor'

describe('模型映射编辑器纯函数', () => {
  test('解析合法 JSON object 为可视化映射行', () => {
    assert.deepEqual(
      parseModelMappingJson(
        JSON.stringify({
          'gpt-5.6': 'gpt-5.6-sol',
          'gpt-5.6-fast': 'gpt-5.6-luna',
        })
      ),
      {
        ok: true,
        entries: [
          { from: 'gpt-5.6', to: 'gpt-5.6-sol' },
          { from: 'gpt-5.6-fast', to: 'gpt-5.6-luna' },
        ],
      }
    )
  })

  test('拒绝数组、非法 JSON 和非字符串目标模型', () => {
    assert.deepEqual(parseModelMappingJson('[]'), {
      ok: false,
      error: 'Model mapping must be a valid JSON object',
    })
    assert.deepEqual(parseModelMappingJson('{'), {
      ok: false,
      error: 'Model mapping must be valid JSON format',
    })
    assert.deepEqual(parseModelMappingJson('{"gpt-5.6": 1}'), {
      ok: false,
      error: 'Model mapping values must be strings',
    })
  })

  test('可视化行转 JSON 时忽略空入口模型并保留空目标模型', () => {
    assert.equal(
      modelMappingRowsToJson([
        { from: ' gpt-5.6 ', to: ' gpt-5.6-sol ' },
        { from: '', to: 'ignored-target' },
        { from: 'gpt-5.6-empty-target', to: '   ' },
      ]),
      JSON.stringify(
        {
          'gpt-5.6': 'gpt-5.6-sol',
          'gpt-5.6-empty-target': '',
        },
        null,
        2
      )
    )
  })

  test('按 trim 后的入口模型检测重复项', () => {
    assert.deepEqual(
      getDuplicateSources([
        { from: 'gpt-5.6' },
        { from: ' gpt-5.6 ' },
        { from: 'gpt-5.6-luna' },
      ]),
      ['gpt-5.6']
    )
  })
})
