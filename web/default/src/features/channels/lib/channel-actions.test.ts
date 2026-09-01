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
import {
  buildChannelTestParams,
  channelsQueryKeys,
  patchChannelBalanceCache,
  selectChannelAccountQuickTestModel,
} from './channel-actions'

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

  test('会完整保留超过 int32 上限的大额已使用量', () => {
    const queryClient = new QueryClient()
    const listKey = channelsQueryKeys.list({ p: 1, page_size: 10 })
    const detailKey = channelsQueryKeys.detail(1)
    const largeUsedQuota = 33508580000

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
        ],
        total: 1,
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
      balance: 99999933182.84,
      used_quota: largeUsedQuota,
      balance_updated_time: 1000,
    })

    const listData = queryClient.getQueryData<GetChannelsResponse>(listKey)
    assert.equal(listData?.data?.items[0].used_quota, largeUsedQuota)

    const detailData = queryClient.getQueryData<GetChannelResponse>(detailKey)
    assert.equal(detailData?.data?.used_quota, largeUsedQuota)
  })

  test('同步渠道余额刷新同时更新上游字段，不把上游已用量写入本地已用量', () => {
    const queryClient = new QueryClient()
    const listKey = channelsQueryKeys.list({ p: 1, page_size: 10 })
    const detailKey = channelsQueryKeys.detail(1)

    const channel = makeChannel({
      id: 1,
      balance: 2,
      used_quota: 10,
      balance_updated_time: 100,
    })
    queryClient.setQueryData<GetChannelsResponse>(listKey, {
      success: true,
      data: {
        items: [channel],
        total: 1,
        page: 1,
        page_size: 10,
      },
    })
    queryClient.setQueryData<GetChannelResponse>(detailKey, {
      success: true,
      data: channel,
    })

    patchChannelBalanceCache(queryClient, 1, {
      success: true,
      balance: 26.9510572,
      used_quota: 10,
      upstream_balance_usd: 26.9510572,
      upstream_used_usd: 96.510803,
      upstream_used_quota: 96_510_803,
      upstream_conversion_factor: 0.1,
      upstream_partial: false,
      balance_updated_time: 999,
    })

    const listData = queryClient.getQueryData<GetChannelsResponse>(listKey)
    const detailData = queryClient.getQueryData<GetChannelResponse>(detailKey)
    assert.equal(listData?.data?.items[0].used_quota, 10)
    assert.equal(listData?.data?.items[0].upstream_used_quota, 96_510_803)
    assert.equal(listData?.data?.items[0].upstream_conversion_factor, 0.1)
    assert.equal(detailData?.data?.used_quota, 10)
    assert.equal(detailData?.data?.upstream_used_usd, 96.510803)
  })
})

describe('渠道测试请求参数', () => {
  test('只指定同步账号密钥时也会携带 account_id', () => {
    assert.deepEqual(buildChannelTestParams({ accountId: 42 }), {
      account_id: 42,
    })
  })

  test('没有测试选项时不产生查询参数', () => {
    assert.equal(buildChannelTestParams(), undefined)
  })
})

describe('同步密钥快速测试模型选择', () => {
  test('渠道测试模型属于密钥模型时优先使用渠道配置', () => {
    assert.equal(
      selectChannelAccountQuickTestModel(
        { test_model: 'gpt-5.4' },
        { models: 'gpt-5.4,gpt-5.4-mini' }
      ),
      'gpt-5.4'
    )
  })

  test('渠道测试模型不属于密钥模型时回退到密钥首个具体模型', () => {
    assert.equal(
      selectChannelAccountQuickTestModel(
        { test_model: 'deepseek' },
        { models: 'claude-haiku-4-5-20251001,claude-opus-4-7' }
      ),
      'claude-haiku-4-5-20251001'
    )
  })

  test('通配模型支持渠道测试模型命中', () => {
    assert.equal(
      selectChannelAccountQuickTestModel(
        { test_model: 'gpt-5.4-mini' },
        { models: 'gpt-5.4-*' }
      ),
      'gpt-5.4-mini'
    )
  })

  test('没有具体模型时不生成快速测试模型', () => {
    assert.equal(
      selectChannelAccountQuickTestModel(
        { test_model: 'deepseek' },
        { models: 'gpt-*' }
      ),
      undefined
    )
  })
})
