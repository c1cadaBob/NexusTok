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
  getFirstFormErrorTarget,
  hasFormErrors,
} from './form-validation-focus'

class FakeNode {
  constructor(private readonly positionMask: number) {}

  compareDocumentPosition() {
    return this.positionMask
  }
}

describe('Form 提交错误聚焦辅助函数', () => {
  test('空错误对象不触发自动聚焦', () => {
    assert.equal(hasFormErrors(null), false)
    assert.equal(hasFormErrors({}), false)
  })

  test('存在字段错误时触发自动聚焦', () => {
    assert.equal(hasFormErrors({ name: { message: 'Required' } }), true)
  })

  test('无错误消息时优先使用 invalid 控件', () => {
    const invalidControl = new FakeNode(0)

    assert.equal(
      getFirstFormErrorTarget(invalidControl, null, 2),
      invalidControl
    )
  })

  test('错误消息在 invalid 控件前方时优先滚动到错误消息', () => {
    const invalidControl = new FakeNode(2)
    const errorMessage = new FakeNode(0)

    assert.equal(
      getFirstFormErrorTarget(invalidControl, errorMessage, 2),
      errorMessage
    )
  })

  test('错误消息在 invalid 控件后方时保留 invalid 控件为目标', () => {
    const invalidControl = new FakeNode(0)
    const errorMessage = new FakeNode(0)

    assert.equal(
      getFirstFormErrorTarget(invalidControl, errorMessage, 2),
      invalidControl
    )
  })
})
