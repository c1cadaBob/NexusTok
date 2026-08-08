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
  dedupeModelCatalogNames,
  filterModelCatalogNames,
  getModelCatalogNames,
} from './model-catalog'

describe('模型仓库候选归一化', () => {
  test('模型名按 trim 和大小写不敏感方式去重', () => {
    assert.deepEqual(
      dedupeModelCatalogNames([
        ' gpt-5.5 ',
        'GPT-5.5',
        '',
        'claude-sonnet',
      ]),
      ['gpt-5.5', 'claude-sonnet']
    )
  })

  test('普通模型保留模型名和展开命中', () => {
    assert.deepEqual(
      getModelCatalogNames([
        {
          model_name: 'gpt-5.5',
          name_rule: 0,
          matched_models: ['gpt-5.5'],
          matched_count: 1,
        },
        {
          model_name: 'custom-rule',
          name_rule: 0,
          matched_models: ['custom-model-a'],
          matched_count: 1,
        },
      ]),
      ['gpt-5.5', 'custom-rule', 'custom-model-a']
    )
  })

  test('规则模型优先使用后端展开出的真实模型', () => {
    assert.deepEqual(
      getModelCatalogNames([
        {
          model_name: 'gpt-5.*',
          name_rule: 1,
          matched_models: ['gpt-5.5', 'gpt-5.6-terra'],
          matched_count: 2,
        },
      ]),
      ['gpt-5.5', 'gpt-5.6-terra']
    )
  })
})

describe('模型仓库安全文字匹配', () => {
  const catalog = ['gpt-5.5', 'gpt-5x5', 'GPT-4.1', 'claude-sonnet']

  test('空输入返回完整候选列表', () => {
    assert.deepEqual(filterModelCatalogNames(catalog, ''), catalog)
  })

  test('输入按大小写不敏感包含匹配', () => {
    assert.deepEqual(filterModelCatalogNames(catalog, 'GPT'), [
      'gpt-5.5',
      'gpt-5x5',
      'GPT-4.1',
    ])
  })

  test('特殊字符按普通文本匹配而不是原生正则', () => {
    assert.deepEqual(filterModelCatalogNames(catalog, 'gpt-5.5'), ['gpt-5.5'])
  })
})
