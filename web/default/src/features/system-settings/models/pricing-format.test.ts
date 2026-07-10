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

import { formatPricingNumber } from './pricing-format'

describe('模型定价价格格式化', () => {
  test('空值和非法数字返回空字符串', () => {
    assert.equal(formatPricingNumber(''), '')
    assert.equal(formatPricingNumber(null), '')
    assert.equal(formatPricingNumber(undefined), '')
    assert.equal(formatPricingNumber(false), '')
    assert.equal(formatPricingNumber('not-a-number'), '')
    assert.equal(formatPricingNumber(Number.NaN), '')
  })

  test('吸附常见十进制浮点漂移', () => {
    assert.equal(formatPricingNumber(0.1 + 0.2), '0.3')
    assert.equal(formatPricingNumber(0.1 * 0.2), '0.02')
    assert.equal(formatPricingNumber(3.7500000000000004), '3.75')
    assert.equal(formatPricingNumber(15.110000000000001), '15.11')
  })

  test('保留有效高精度价格并裁剪展示尾零', () => {
    assert.equal(formatPricingNumber(1.005), '1.005')
    assert.equal(formatPricingNumber('0.3333333333333333'), '0.333333333333')
    assert.equal(formatPricingNumber('10.000000000000'), '10')
  })
})
