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
import { buildSearchParams } from './filter'

describe('buildSearchParams', () => {
  test('任务日志使用 channel_id 参数', () => {
    const params = buildSearchParams(
      {
        channel: '102',
        taskId: 'systask_mcp_upstream_sync_001',
      },
      'task'
    )

    assert.equal(params.channel_id, '102')
    assert.equal(params.channel, undefined)
    assert.equal(params.filter, 'systask_mcp_upstream_sync_001')
  })

  test('普通日志继续使用 channel 参数', () => {
    const params = buildSearchParams(
      {
        channel: '101',
        model: 'gpt-4o',
      },
      'common'
    )

    assert.equal(params.channel, '101')
    assert.equal(params.channel_id, undefined)
    assert.equal(params.model, 'gpt-4o')
  })
})
