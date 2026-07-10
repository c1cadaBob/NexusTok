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
  getSafeNumberDisplayValue,
  safeNumberFieldProps,
} from './numeric-field'

describe('系统设置数字字段安全绑定', () => {
  test('只有有限数字会作为 input 展示值', () => {
    assert.equal(getSafeNumberDisplayValue(12), 12)
    assert.equal(getSafeNumberDisplayValue(0), 0)
    assert.equal(getSafeNumberDisplayValue(Number.NaN), '')
    assert.equal(getSafeNumberDisplayValue(Number.POSITIVE_INFINITY), '')
    assert.equal(getSafeNumberDisplayValue('12'), '')
    assert.equal(getSafeNumberDisplayValue(null), '')
  })

  test('有限数字会写入表单状态', () => {
    const changes: number[] = []
    const props = safeNumberFieldProps({
      value: 3,
      onChange: (value: number) => changes.push(value),
      onBlur: () => undefined,
      name: 'field',
      ref: () => undefined,
    } as never)

    props.onChange({ target: { valueAsNumber: 8.5 } } as never)

    assert.deepEqual(changes, [8.5])
  })

  test('NaN 和 Infinity 不会污染表单状态', () => {
    const changes: number[] = []
    const props = safeNumberFieldProps({
      value: 3,
      onChange: (value: number) => changes.push(value),
      onBlur: () => undefined,
      name: 'field',
      ref: () => undefined,
    } as never)

    props.onChange({ target: { valueAsNumber: Number.NaN } } as never)
    props.onChange({
      target: { valueAsNumber: Number.POSITIVE_INFINITY },
    } as never)

    assert.deepEqual(changes, [])
  })
})
