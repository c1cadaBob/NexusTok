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
  buildPreviewRows,
  createInitialLaneState,
  hasValue,
  numericDraftRegex,
  toNumberOrNull,
  type ModelPricingFormValues,
} from './model-pricing-core'

const t = (key: string) => key

const baseValues: ModelPricingFormValues = {
  name: 'gpt-test',
  price: '',
  ratio: '',
  cacheRatio: '',
  createCacheRatio: '',
  completionRatio: '',
  imageRatio: '',
  audioRatio: '',
  audioCompletionRatio: '',
}

describe('模型定价编辑核心', () => {
  test('从保存的倍率推导输入价格、lane 价格和启用状态', () => {
    const state = createInitialLaneState({
      name: 'gpt-test',
      ratio: '1.5',
      completionRatio: '2',
      cacheRatio: '0.2',
      createCacheRatio: '',
      imageRatio: '0',
      audioRatio: '1.25',
      audioCompletionRatio: '4',
    })

    assert.equal(state.promptPrice, '3')
    assert.equal(state.prices.completion, '6')
    assert.equal(state.prices.cache, '0.6')
    assert.equal(state.prices.createCache, '')
    assert.equal(state.prices.image, '0')
    assert.equal(state.prices.audioInput, '3.75')
    assert.equal(state.prices.audioOutput, '15')
    assert.equal(state.enabled.completion, true)
    assert.equal(state.enabled.cache, true)
    assert.equal(state.enabled.createCache, false)
    assert.equal(state.enabled.image, true)
    assert.equal(state.enabled.audioInput, true)
    assert.equal(state.enabled.audioOutput, true)
  })

  test('空模型返回空 lane 状态副本', () => {
    const first = createInitialLaneState(null)
    const second = createInitialLaneState(null)

    first.prices.cache = '1'
    first.enabled.cache = true

    assert.equal(second.prices.cache, '')
    assert.equal(second.enabled.cache, false)
  })

  test('per-token 预览只展示已启用且有价格的 lane', () => {
    const rows = buildPreviewRows(
      baseValues,
      'per-token',
      '',
      '',
      '3',
      {
        completion: '6',
        cache: '0.6',
        createCache: '',
        image: '0',
        audioInput: '',
        audioOutput: '',
      },
      {
        completion: true,
        cache: true,
        createCache: false,
        image: true,
        audioInput: false,
        audioOutput: false,
      },
      t
    )

    assert.deepEqual(
      rows.map((row) => [row.key, row.value]),
      [
        ['inputPrice', '$3'],
        ['completion', '$6'],
        ['cache', '$0.6'],
        ['createCache', 'Empty'],
        ['image', '$0'],
        ['audio', 'Empty'],
        ['audioCompletion', 'Empty'],
      ]
    )
  })

  test('per-request 和 tiered_expr 预览保持保存格式', () => {
    const requestRows = buildPreviewRows(
      { ...baseValues, price: '0.01' },
      'per-request',
      '',
      '',
      '',
      createInitialLaneState(null).prices,
      createInitialLaneState(null).enabled,
      t
    )

    assert.deepEqual(requestRows, [
      { key: 'price', label: 'ModelPrice', value: '0.01' },
    ])

    const exprRows = buildPreviewRows(
      baseValues,
      'tiered_expr',
      'tier("base", p * 2 + c * 8)',
      '(has(header("x-plan"), "pro") ? 2 : 1)',
      '',
      createInitialLaneState(null).prices,
      createInitialLaneState(null).enabled,
      t
    )

    assert.equal(exprRows[0].value, 'tiered_expr')
    assert.equal(
      exprRows[1].value,
      '(tier("base", p * 2 + c * 8)) * (has(header("x-plan"), "pro") ? 2 : 1)'
    )
    assert.equal(exprRows[1].multiline, true)
  })

  test('空值、零值和数字草稿解析保持表单语义', () => {
    assert.equal(hasValue(''), false)
    assert.equal(hasValue(false), false)
    assert.equal(hasValue(0), true)
    assert.equal(toNumberOrNull(''), null)
    assert.equal(toNumberOrNull('0'), 0)
    assert.equal(toNumberOrNull('1.25'), 1.25)
    assert.equal(toNumberOrNull('abc'), null)
    assert.equal(numericDraftRegex.test('.'), true)
    assert.equal(numericDraftRegex.test('1.'), true)
    assert.equal(numericDraftRegex.test('1.2.3'), false)
  })
})
