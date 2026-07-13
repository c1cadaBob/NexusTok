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
import { getPricingModelIconKey } from './model-icon'

describe('getPricingModelIconKey', () => {
  test('优先返回模型级图标', () => {
    assert.equal(
      getPricingModelIconKey({
        icon: 'OpenAI.Color',
        vendor_icon: 'OpenAI',
      }),
      'OpenAI.Color'
    )
  })

  test('模型级图标缺失时回退到供应商图标', () => {
    assert.equal(
      getPricingModelIconKey({
        vendor_icon: 'Anthropic.Color',
      }),
      'Anthropic.Color'
    )
  })

  test('模型级图标为空白时不阻断供应商 fallback', () => {
    assert.equal(
      getPricingModelIconKey({
        icon: '   ',
        vendor_icon: 'Google.Color',
      }),
      'Google.Color'
    )
  })

  test('模型级和供应商图标都缺失时返回 undefined', () => {
    assert.equal(getPricingModelIconKey({}), undefined)
  })
})
