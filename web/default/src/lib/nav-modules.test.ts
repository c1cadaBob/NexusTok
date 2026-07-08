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
  getModuleAccessFromStatus,
  parseHeaderNavBoolean,
  parseHeaderNavModules,
  parseHeaderNavModulesFromStatus,
} from './nav-modules'

describe('HeaderNavModules 解析', () => {
  test('空配置和无效 JSON 使用默认公开模块', () => {
    assert.equal(parseHeaderNavModules('').pricing.enabled, true)
    assert.equal(parseHeaderNavModules('not-json').rankings.enabled, true)
  })

  test('兼容顶层布尔、字符串和数字形式', () => {
    const modules = parseHeaderNavModules(
      JSON.stringify({
        home: '0',
        console: 0,
        docs: 'false',
        about: false,
      })
    )

    assert.equal(modules.home, false)
    assert.equal(modules.console, false)
    assert.equal(modules.docs, false)
    assert.equal(modules.about, false)
  })

  test('兼容 pricing/rankings 的历史短格式', () => {
    const modules = parseHeaderNavModules({
      pricing: '0',
      rankings: 1,
    })

    assert.deepEqual(modules.pricing, {
      enabled: false,
      requireAuth: false,
    })
    assert.deepEqual(modules.rankings, {
      enabled: true,
      requireAuth: false,
    })
  })

  test('兼容对象字段中的字符串和数字布尔值', () => {
    const modules = parseHeaderNavModules(
      JSON.stringify({
        pricing: { enabled: '1', requireAuth: 'true' },
        rankings: { enabled: 0, requireAuth: 1 },
      })
    )

    assert.deepEqual(modules.pricing, {
      enabled: true,
      requireAuth: true,
    })
    assert.deepEqual(modules.rankings, {
      enabled: false,
      requireAuth: true,
    })
  })

  test('未知顶层模块使用布尔解析并保留在结果中', () => {
    const modules = parseHeaderNavModules(
      JSON.stringify({
        custom: 'false',
      })
    )

    assert.equal(modules.custom, false)
  })

  test('状态对象入口与模块访问入口共用同一语义', () => {
    const status = {
      HeaderNavModules: JSON.stringify({
        pricing: { enabled: 'false', requireAuth: '1' },
      }),
    }

    assert.equal(parseHeaderNavModulesFromStatus(status).pricing.enabled, false)
    assert.deepEqual(getModuleAccessFromStatus(status, 'pricing'), {
      enabled: false,
      requireAuth: true,
    })
  })

  test('非法布尔文本回退到调用方默认值', () => {
    assert.equal(parseHeaderNavBoolean('enabled', false), false)
    assert.equal(parseHeaderNavBoolean(2, true), true)
  })
})
