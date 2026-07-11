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
  buildModelSearchAppendPlan,
  buildModelSearchAppendSummary,
  dedupeModelNames,
  getMissingModelSearchMatches,
  getModelSearchModelNames,
  isModelSearchAppendContextCurrent,
  mergeModelNames,
  parseModelDraftList,
} from './model-search'

describe('渠道模型草稿解析', () => {
  test('支持逗号、中文逗号和换行并按大小写不敏感去重', () => {
    assert.deepEqual(
      parseModelDraftList(
        ' gpt-5.6-terra, GPT-5.6-TERRA，gpt-5.6-luna\n\n gpt-5.6-sol '
      ),
      ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol']
    )
  })
})

describe('渠道模型列表归一化', () => {
  test('按 trim + 大小写不敏感方式去重并保留首次展示形式', () => {
    assert.deepEqual(
      dedupeModelNames([
        ' gpt-5.6-terra ',
        'GPT-5.6-TERRA',
        '',
        'gpt-5.6-luna',
      ]),
      ['gpt-5.6-terra', 'gpt-5.6-luna']
    )
  })

  test('合并模型列表时保留已有模型并只追加新增项', () => {
    assert.deepEqual(
      mergeModelNames(
        ['gpt-5.6-sol', 'gpt-5.6-terra'],
        ['GPT-5.6-TERRA', 'gpt-5.6-luna']
      ),
      ['gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna']
    )
  })
})

describe('渠道模型搜索候选提取', () => {
  test('只保留模型名自身包含关键词的搜索结果', () => {
    assert.deepEqual(
      getModelSearchModelNames(
        [
          { model_name: 'gpt-5.6-terra' },
          { model_name: 'claude-sonnet' },
          { model_name: ' gpt-5.6-luna ' },
        ],
        'gpt-5.6'
      ),
      ['gpt-5.6-terra', 'gpt-5.6-luna']
    )
  })

  test('搜索候选按大小写不敏感方式去重', () => {
    assert.deepEqual(
      getModelSearchModelNames(
        [
          { model_name: 'GPT-5.6-Terra' },
          { model_name: 'gpt-5.6-terra' },
          { model_name: '' },
        ],
        'gpt-5.6'
      ),
      ['GPT-5.6-Terra']
    )
  })

  test('搜索候选会纳入名称规则展开出的具体匹配模型', () => {
    assert.deepEqual(
      getModelSearchModelNames(
        [
          {
            model_name: 'gpt-5.6',
            name_rule: 1,
            matched_models: ['gpt-5.6-terra', 'gpt-5.6-luna', 'claude-sonnet'],
          },
          {
            model_name: 'reasoning family',
            name_rule: 2,
            matched_models: ['gpt-5.6-sol', 'GPT-5.6-TERRA'],
          },
        ],
        'gpt-5.6'
      ),
      ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol']
    )
  })

  test('精确模型仍会保留 model_name 自身', () => {
    assert.deepEqual(
      getModelSearchModelNames(
        [
          {
            model_name: 'gpt-5.6-sol',
            name_rule: 0,
            matched_models: ['gpt-5.6-sol'],
          },
        ],
        'gpt-5.6'
      ),
      ['gpt-5.6-sol']
    )
  })
})

describe('渠道模型搜索缺失项计算', () => {
  test('搜索 gpt-5.6 时会保留全部三个待追加模型', () => {
    assert.deepEqual(
      getMissingModelSearchMatches(
        ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol'],
        []
      ),
      ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol']
    )
  })

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

describe('渠道模型搜索批量追加计划', () => {
  test('预览和总数共用同一套缺失模型计算结果', () => {
    assert.deepEqual(
      buildModelSearchAppendPlan(
        ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol'],
        ['gpt-5.6-sol'],
        1
      ),
      {
        missingModels: ['gpt-5.6-terra', 'gpt-5.6-luna'],
        previewModels: ['gpt-5.6-terra'],
        omittedCount: 1,
        totalCount: 2,
      }
    )
  })

  test('预览限制不会影响最终待追加模型列表', () => {
    const plan = buildModelSearchAppendPlan(
      ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol'],
      [],
      2
    )

    assert.deepEqual(plan.missingModels, [
      'gpt-5.6-terra',
      'gpt-5.6-luna',
      'gpt-5.6-sol',
    ])
    assert.deepEqual(plan.previewModels, ['gpt-5.6-terra', 'gpt-5.6-luna'])
    assert.equal(plan.omittedCount, 1)
    assert.equal(plan.totalCount, 3)
  })

  test('搜索摘要会区分命中、可新增和已存在模型', () => {
    assert.deepEqual(
      buildModelSearchAppendSummary(
        ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol'],
        ['gpt-5.4', 'gpt-5.5', 'gpt-5.6-sol']
      ),
      {
        matchedCount: 3,
        addableCount: 2,
        existingCount: 1,
      }
    )
  })
})

describe('渠道模型搜索追加上下文校验', () => {
  test('同一渠道且关键词仅大小写或空格变化时视为同一请求', () => {
    assert.equal(
      isModelSearchAppendContextCurrent(
        { open: true, channelId: 7, keyword: ' GPT-5.6 ' },
        { channelId: 7, keyword: 'gpt-5.6' }
      ),
      true
    )
  })

  test('抽屉关闭、渠道切换或关键词变化时拒绝旧搜索结果', () => {
    assert.equal(
      isModelSearchAppendContextCurrent(
        { open: false, channelId: 7, keyword: 'gpt-5.6' },
        { channelId: 7, keyword: 'gpt-5.6' }
      ),
      false
    )
    assert.equal(
      isModelSearchAppendContextCurrent(
        { open: true, channelId: 8, keyword: 'gpt-5.6' },
        { channelId: 7, keyword: 'gpt-5.6' }
      ),
      false
    )
    assert.equal(
      isModelSearchAppendContextCurrent(
        { open: true, channelId: 7, keyword: 'gpt-5.7' },
        { channelId: 7, keyword: 'gpt-5.6' }
      ),
      false
    )
  })
})
