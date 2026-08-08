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
  dedupeModelNames,
  getModelSearchModelNameResult,
  getModelSearchModelNames,
  getModelSearchVendorForChannelType,
  mergeModelNames,
  parseModelDraftList,
  summarizeModelSearchCandidates,
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
  test('保留后端搜索命中的精确模型名，不再按关键词二次过滤', () => {
    assert.deepEqual(
      getModelSearchModelNames(
        [
          {
            model_name: 'gpt-5.6-terra',
            description: 'GPT-5.6 Terra is an AI model from OpenAI.',
          },
          {
            model_name: 'claude-sonnet',
            description: 'OpenAI-compatible Claude route.',
          },
          { model_name: ' gpt-5.6-luna ', tags: 'OpenAI,Reasoning' },
        ],
        'openai'
      ),
      ['gpt-5.6-terra', 'claude-sonnet', 'gpt-5.6-luna']
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
      ['gpt-5.6-terra', 'gpt-5.6-luna', 'claude-sonnet', 'gpt-5.6-sol']
    )
  })

  test('名称规则没有展开结果时只在规则名自身命中时保留占位名', () => {
    assert.deepEqual(
      getModelSearchModelNames(
        [
          {
            model_name: 'gpt-5.6-*',
            name_rule: 1,
            matched_models: [],
          },
          {
            model_name: 'claude-*',
            name_rule: 1,
            matched_models: [],
          },
        ],
        'gpt-5.6'
      ),
      ['gpt-5.6-*']
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

  test('规则模型会以展开出的真实模型数作为候选基准', () => {
    assert.deepEqual(
      getModelSearchModelNameResult(
        [
          {
            model_name: 'gpt-5.6',
            name_rule: 1,
            matched_count: 3,
            matched_models: ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol'],
          },
        ],
        'gpt-5.6'
      ),
      {
        names: ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol'],
        unresolvedMatchedCount: 0,
      }
    )
  })

  test('规则模型声明的未展开匹配数只作为候选缺口提示', () => {
    assert.deepEqual(
      getModelSearchModelNameResult(
        [
          {
            model_name: 'gpt-5.6',
            name_rule: 1,
            matched_count: 3,
            matched_models: ['gpt-5.6-terra'],
          },
        ],
        'gpt-5.6'
      ),
      {
        names: ['gpt-5.6-terra'],
        unresolvedMatchedCount: 2,
      }
    )
  })
})

describe('渠道模型搜索候选汇总', () => {
  test('模型仓库返回 gpt-5.5 时保留为真实候选', () => {
    assert.deepEqual(
      getModelSearchModelNames(
        [
          {
            model_name: 'gpt-5.5',
            description: 'GPT-5.5',
            name_rule: 0,
          },
        ],
        'gpt-5.5'
      ),
      ['gpt-5.5']
    )
  })

  test('区分全部命中、可新增命中和已存在命中', () => {
    assert.deepEqual(
      summarizeModelSearchCandidates(
        ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol'],
        ['gpt-5.4', 'GPT-5.6-SOL']
      ),
      {
        matched: ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol'],
        addable: ['gpt-5.6-terra', 'gpt-5.6-luna'],
        existingCount: 1,
      }
    )
  })

  test('其它账号已使用的模型不影响当前密钥追加候选', () => {
    assert.deepEqual(summarizeModelSearchCandidates(['gpt-5.5'], []), {
      matched: ['gpt-5.5'],
      addable: ['gpt-5.5'],
      existingCount: 0,
    })
  })

  test('候选自身先按大小写不敏感去重', () => {
    assert.deepEqual(
      summarizeModelSearchCandidates(
        ['gpt-5.6-terra', 'GPT-5.6-TERRA', 'gpt-5.6-luna'],
        []
      ),
      {
        matched: ['gpt-5.6-terra', 'gpt-5.6-luna'],
        addable: ['gpt-5.6-terra', 'gpt-5.6-luna'],
        existingCount: 0,
      }
    )
  })
})

describe('渠道模型搜索默认供应商', () => {
  test('OpenAI 与 Codex 渠道默认搜索 OpenAI 模型库', () => {
    assert.equal(getModelSearchVendorForChannelType(1), 'OpenAI')
    assert.equal(getModelSearchVendorForChannelType(57), 'OpenAI')
  })

  test('明确供应商渠道会收窄到对应模型供应商', () => {
    assert.equal(getModelSearchVendorForChannelType(14), 'Anthropic')
    assert.equal(getModelSearchVendorForChannelType(24), 'Google')
    assert.equal(getModelSearchVendorForChannelType(43), 'DeepSeek')
  })

  test('使用当前模型库的 canonical vendor 名称，避免搜索被错误收窄', () => {
    assert.equal(getModelSearchVendorForChannelType(16), '智谱')
    assert.equal(getModelSearchVendorForChannelType(17), '阿里巴巴')
    assert.equal(getModelSearchVendorForChannelType(23), '腾讯')
    assert.equal(getModelSearchVendorForChannelType(25), 'Moonshot')
    assert.equal(getModelSearchVendorForChannelType(26), '智谱')
  })

  test('自定义与高级自定义渠道保持全局模型库搜索', () => {
    assert.equal(getModelSearchVendorForChannelType(8), '')
    assert.equal(getModelSearchVendorForChannelType(58), '')
  })
})
