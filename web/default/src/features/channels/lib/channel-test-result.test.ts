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
import { getChannelTestFailureDisplay } from './channel-test-result'

const fallbackSummary = 'Test failed'
const modelPriceSummary =
  'Model price is not configured. Please complete pricing in Models → Sync Source Models.'

describe('渠道测试失败结果展示', () => {
  test('空错误使用兜底摘要', () => {
    assert.deepEqual(
      getChannelTestFailureDisplay({
        fallbackSummary,
        isModelPriceError: false,
        modelPriceSummary,
      }),
      { summary: fallbackSummary }
    )
  })

  test('短单行错误只展示摘要，不重复保留详情', () => {
    assert.deepEqual(
      getChannelTestFailureDisplay({
        errorText: 'upstream timeout',
        fallbackSummary,
        isModelPriceError: false,
        modelPriceSummary,
      }),
      { summary: 'upstream timeout' }
    )
  })

  test('多行错误取首条非空行作为摘要，并保留完整详情', () => {
    const errorText = '\n\nupstream rejected request\ntrace id: abc123'

    assert.deepEqual(
      getChannelTestFailureDisplay({
        errorText,
        fallbackSummary,
        isModelPriceError: false,
        modelPriceSummary,
      }),
      {
        summary: 'upstream rejected request',
        details: errorText,
      }
    )
  })

  test('长错误摘要会截断并保留完整详情', () => {
    const errorText = `${'a'.repeat(120)}\nraw body`
    const display = getChannelTestFailureDisplay({
      errorText,
      fallbackSummary,
      isModelPriceError: false,
      modelPriceSummary,
    })

    assert.equal(display.summary.length, 99)
    assert.equal(display.summary.endsWith('...'), true)
    assert.equal(display.details, errorText)
  })

  test('模型价格错误使用固定摘要，原始错误不同时保留详情', () => {
    assert.deepEqual(
      getChannelTestFailureDisplay({
        errorText: 'model gpt-test has no price configuration',
        fallbackSummary,
        isModelPriceError: true,
        modelPriceSummary,
      }),
      {
        summary: modelPriceSummary,
        details: 'model gpt-test has no price configuration',
      }
    )
  })

  test('模型价格错误原文等于摘要时不重复展示详情', () => {
    assert.deepEqual(
      getChannelTestFailureDisplay({
        errorText: modelPriceSummary,
        fallbackSummary,
        isModelPriceError: true,
        modelPriceSummary,
      }),
      { summary: modelPriceSummary }
    )
  })
})
