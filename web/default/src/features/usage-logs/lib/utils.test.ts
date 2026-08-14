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
import { buildApiParams, buildBaseParams } from './utils'

describe('buildApiParams', () => {
  test('不再发送 type 参数，使用日志由后端固定为消费日志', () => {
    const params = buildApiParams({
      page: 1,
      pageSize: 100,
      searchParams: { type: ['0'] },
      isAdmin: false,
    })

    assert.equal(params.p, 1)
    assert.equal(params.page_size, 100)
    assert.equal('type' in params, false)
  })
})

describe('buildBaseParams', () => {
  test('任务日志优先使用 channel_id 查询参数', () => {
    const params = buildBaseParams({
      page: 1,
      pageSize: 20,
      searchParams: {
        channel_id: '102',
        channel: '101',
        startTime: Date.UTC(2026, 7, 14, 0, 0, 0),
        endTime: Date.UTC(2026, 7, 14, 1, 0, 0),
      },
    })

    assert.equal(params.channel_id, '102')
    assert.equal(params.start_timestamp, 1786665600)
    assert.equal(params.end_timestamp, 1786669200)
  })

  test('兼容旧链接中的 channel 参数', () => {
    const params = buildBaseParams({
      page: 1,
      pageSize: 20,
      searchParams: { channel: '101' },
    })

    assert.equal(params.channel_id, '101')
  })
})
