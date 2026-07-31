/*
Copyright (C) 2023-2026 c1cada

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License
as published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@c1cada.dev
*/
import { QueryClient } from '@tanstack/react-query'
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { Channel, GetChannelResponse, GetChannelsResponse } from '../types'
import { channelsQueryKeys, patchChannelBalanceCache } from './channel-actions'

function makeChannel(overrides: Partial<Channel>): Channel {
  return {
    id: 1,
    type: 1,
    key: '',
    name: 'test',
    status: 1,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    balance: 0,
    balance_updated_time: 0,
    models: '',
    group: 'default',
    used_quota: 0,
    other: '',
    other_info: '',
    remark: '',
    settings: '{}',
    max_input_tokens: 0,
    channel_info: {
      is_multi_key: false,
      multi_key_size: 0,
      multi_key_polling_index: 0,
      multi_key_mode: 'random',
    },
    ...overrides,
  }
}

describe('渠道余额缓存补丁', () => {
  test('会同时更新列表和详情缓存中的余额、已使用量和更新时间', () => {
    const queryClient = new QueryClient()
    const listKey = channelsQueryKeys.list({ p: 1, page_size: 10 })
    const detailKey = channelsQueryKeys.detail(1)

    queryClient.setQueryData<GetChannelsResponse>(listKey, {
      success: true,
      data: {
        items: [
          makeChannel({
            id: 1,
            balance: 1,
            used_quota: 10,
            balance_updated_time: 100,
          }),
          makeChannel({
            id: 2,
            balance: 2,
            used_quota: 20,
            balance_updated_time: 200,
          }),
        ],
        total: 2,
        page: 1,
        page_size: 10,
      },
    })
    queryClient.setQueryData<GetChannelResponse>(detailKey, {
      success: true,
      data: makeChannel({
        id: 1,
        balance: 1,
        used_quota: 10,
        balance_updated_time: 100,
      }),
    })

    patchChannelBalanceCache(queryClient, 1, {
      success: true,
      balance: 12.5,
      used_quota: 4750,
      balance_updated_time: 999,
    })

    const listData = queryClient.getQueryData<GetChannelsResponse>(listKey)
    assert.ok(listData?.data?.items)
    assert.equal(listData.data.items[0].balance, 12.5)
    assert.equal(listData.data.items[0].used_quota, 4750)
    assert.equal(listData.data.items[0].balance_updated_time, 999)
    assert.equal(listData.data.items[1].balance, 2)

    const detailData = queryClient.getQueryData<GetChannelResponse>(detailKey)
    assert.ok(detailData?.data)
    assert.equal(detailData.data.balance, 12.5)
    assert.equal(detailData.data.used_quota, 4750)
    assert.equal(detailData.data.balance_updated_time, 999)
  })

  test('会更新 tag 聚合行的子渠道并重新计算父行已使用量', () => {
    const queryClient = new QueryClient()
    const listKey = channelsQueryKeys.list({ p: 1, page_size: 10 })

    queryClient.setQueryData<GetChannelsResponse>(listKey, {
      success: true,
      data: {
        items: [
          {
            ...makeChannel({
              id: 900,
              name: 'tag-row',
              tag: 'prod',
              used_quota: 30,
            }),
            children: [
              makeChannel({
                id: 1,
                tag: 'prod',
                balance: 1,
                used_quota: 10,
                balance_updated_time: 100,
              }),
              makeChannel({
                id: 2,
                tag: 'prod',
                balance: 2,
                used_quota: 20,
                balance_updated_time: 200,
              }),
            ],
          } as Channel & { children: Channel[] },
        ],
        total: 2,
        page: 1,
        page_size: 10,
      },
    })

    patchChannelBalanceCache(queryClient, 1, {
      success: true,
      balance: 9.5,
      used_quota: 70,
      balance_updated_time: 999,
    })

    const listData = queryClient.getQueryData<GetChannelsResponse>(listKey)
    const tagRow = listData?.data?.items[0] as
      | (Channel & { children?: Channel[] })
      | undefined

    assert.ok(tagRow?.children)
    assert.equal(tagRow.children[0].balance, 9.5)
    assert.equal(tagRow.children[0].used_quota, 70)
    assert.equal(tagRow.children[0].balance_updated_time, 999)
    assert.equal(tagRow.children[1].used_quota, 20)
    assert.equal(tagRow.used_quota, 90)
  })
})
