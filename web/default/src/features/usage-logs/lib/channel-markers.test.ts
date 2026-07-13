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
import { getUsageLogChannelMarkers } from './channel-markers'

describe('getUsageLogChannelMarkers', () => {
  test('多次渠道命中会生成重试链路', () => {
    const markers = getUsageLogChannelMarkers({
      use_channel: [12, '15', 18],
    })

    assert.equal(markers.hasRetryChain, true)
    assert.deepEqual(markers.retryChannels, ['12', '15', '18'])
    assert.equal(markers.retryChain, '12 → 15 → 18')
  })

  test('单个渠道命中不显示重试链路', () => {
    const markers = getUsageLogChannelMarkers({
      use_channel: [12],
    })

    assert.equal(markers.hasRetryChain, false)
    assert.deepEqual(markers.retryChannels, ['12'])
    assert.equal(markers.retryChain, undefined)
  })

  test('multi-key 序号只在后端明确标记时展示', () => {
    const markers = getUsageLogChannelMarkers({
      is_multi_key: true,
      multi_key_index: 0,
    })

    assert.equal(markers.multiKeyIndex, 0)
  })

  test('无效 multi-key 序号会被忽略', () => {
    assert.equal(
      getUsageLogChannelMarkers({
        is_multi_key: false,
        multi_key_index: 2,
      }).multiKeyIndex,
      undefined
    )
    assert.equal(
      getUsageLogChannelMarkers({
        is_multi_key: true,
        multi_key_index: Number.NaN,
      }).multiKeyIndex,
      undefined
    )
  })
})
