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
import { buildApiParams } from './utils'

describe('buildApiParams', () => {
  test('保留 type=0 作为 common logs 全部类型哨兵', () => {
    const params = buildApiParams({
      page: 1,
      pageSize: 100,
      searchParams: { type: ['0'] },
      isAdmin: false,
    })

    assert.equal(params.type, 0)
  })

  test('兼容 URL 中的单值字符串 type', () => {
    const params = buildApiParams({
      page: 2,
      pageSize: 20,
      searchParams: { type: '2' },
      isAdmin: false,
    })

    assert.equal(params.p, 2)
    assert.equal(params.page_size, 20)
    assert.equal(params.type, 2)
  })

  test('兼容路由解析后的数值 type', () => {
    const params = buildApiParams({
      page: 1,
      pageSize: 20,
      searchParams: { type: 0 },
      isAdmin: false,
    })

    assert.equal(params.type, 0)
  })

  test('表格列筛选中的 type 会覆盖 URL type', () => {
    const params = buildApiParams({
      page: 1,
      pageSize: 50,
      searchParams: { type: ['0'] },
      columnFilters: [{ id: 'type', value: ['5'] }],
      isAdmin: true,
    })

    assert.equal(params.type, 5)
  })
})
