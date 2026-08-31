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
import type {
  Channel,
  ChannelAccount,
  ChannelAccountListResponse,
  UpstreamAccountKey,
} from '../types'
import {
  buildUpstreamAccountConfigDraft,
  buildUpstreamAccountConfigsFromSnapshotKeys,
  buildUpstreamAccountPreviewRequest,
  buildUpstreamAccountRefreshPayload,
  buildUpstreamRatioConversionPayload,
  collectUpstreamAccountCapabilityValidationErrors,
  getChannelAccountAssetDisplaySource,
  getChannelBalanceDisplaySource,
  getUpstreamRatioConversionInputValues,
  getUpstreamPreviewBalanceDisplay,
  loadAllChannelAccounts,
  resolveStoredUpstreamRatioConversionConfig,
  shouldEnableUpstreamAccountKeyByDefault,
  summarizeUpstreamAccountCapabilities,
  upstreamAssetConversionFactor,
} from './upstream-sync'

function makeSnapshotKey(
  overrides: Partial<UpstreamAccountKey> = {}
): UpstreamAccountKey {
  return {
    sync_id: 'sync-1',
    external_id: 'external-1',
    name: 'upstream key',
    masked_key: 'sk-live...7890',
    group_name: 'upstream-group',
    group_id: 'upstream-group',
    models: ['gpt-upstream'],
    suggested_priority: 1,
    suggested_weight: 100,
    ...overrides,
  }
}

function makeChannelAccount(
  overrides: Partial<ChannelAccount> = {}
): ChannelAccount {
  return {
    id: 1,
    channel_id: 10,
    name: 'synced key',
    key: 'sk-****',
    status: 1,
    models: 'gpt-4o',
    group: 'default',
    access_groups: 'default',
    priority: 0,
    weight: 100,
    last_used_time: 0,
    used_quota: 0,
    other: '',
    settings: '{}',
    last_error: '',
    rate_limited_until: 0,
    overload_until: 0,
    temp_disabled_until: 0,
    disabled_reason: '',
    max_concurrency: 0,
    created_time: 0,
    ...overrides,
  }
}

function makeChannelAccountPage({
  items,
  total,
  page,
  pageSize,
}: {
  items: ChannelAccount[]
  total: number
  page: number
  pageSize: number
}): ChannelAccountListResponse {
  return {
    success: true,
    data: {
      accounts: {
        items,
        total,
        page,
        page_size: pageSize,
      },
      stats: {
        total,
        enabled: total,
        disabled: 0,
        cooldown: 0,
      },
    },
  }
}

describe('上游账号刷新共享 payload', () => {
  test('全量渠道账号加载器按后端分页上限拉满并保持顺序', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) =>
      makeChannelAccount({ id: index + 1 })
    )
    const secondPage = [makeChannelAccount({ id: 101 })]
    const calls: Array<{ page: number; pageSize: number }> = []

    const result = await loadAllChannelAccounts((page, pageSize) => {
      calls.push({ page, pageSize })
      return Promise.resolve(
        makeChannelAccountPage({
          items: page === 1 ? firstPage : secondPage,
          total: 101,
          page,
          pageSize,
        })
      )
    })

    assert.deepEqual(calls, [
      { page: 1, pageSize: 100 },
      { page: 2, pageSize: 100 },
    ])
    assert.equal(result.total, 101)
    assert.deepEqual(
      result.accounts.map((account) => account.id),
      Array.from({ length: 101 }, (_, index) => index + 1)
    )
  })

  test('全量渠道账号加载器单页完成时不再追加请求', async () => {
    let callCount = 0

    const result = await loadAllChannelAccounts((page, pageSize) => {
      callCount += 1
      return Promise.resolve(
        makeChannelAccountPage({
          items: [makeChannelAccount({ id: 7 })],
          total: 1,
          page,
          pageSize,
        })
      )
    })

    assert.equal(callCount, 1)
    assert.deepEqual(
      result.accounts.map((account) => account.id),
      [7]
    )
  })

  test('全量渠道账号加载器遇到失败响应时整体失败', async () => {
    await assert.rejects(
      () =>
        loadAllChannelAccounts(async () => ({
          success: false,
          message: 'boom',
        })),
      /boom/
    )
  })

  test('刷新比例优先从渠道 settings 回填，再回退账号元数据', () => {
    const channelSettings =
      '{"upstream_account_sync":{"ratio_conversion_config":{"paid_cny":3,"platform_usd_credit":12,"enabled":true}}}'
    const accountRatio = {
      paid_cny: 9,
      platform_usd_credit: 18,
      enabled: true,
    }

    assert.deepEqual(
      resolveStoredUpstreamRatioConversionConfig({
        channelSettings,
        accounts: [
          makeChannelAccount({ ratio_conversion_config: accountRatio }),
        ],
      }),
      {
        paid_cny: 3,
        platform_usd_credit: 12,
        enabled: true,
      }
    )

    assert.deepEqual(
      resolveStoredUpstreamRatioConversionConfig({
        channelSettings:
          '{"upstream_account_sync":{"ratio_conversion_config":{"paid_cny":0,"platform_usd_credit":12,"enabled":true}}}',
        accounts: [
          makeChannelAccount({ ratio_conversion_config: accountRatio }),
        ],
      }),
      accountRatio
    )
  })

  test('刷新比例缺失时输入值才回退默认值', () => {
    assert.equal(
      resolveStoredUpstreamRatioConversionConfig({
        channelSettings: '{}',
        accounts: [],
      }),
      undefined
    )
    assert.deepEqual(getUpstreamRatioConversionInputValues(undefined), {
      paidCny: '1',
      platformUsdCredit: '10',
    })
  })

  test('刷新 payload 使用已回填的历史比例而不是输入框默认值', () => {
    const ratioConfig = resolveStoredUpstreamRatioConversionConfig({
      channelSettings:
        '{"upstream_account_sync":{"ratio_conversion_config":{"paid_cny":6,"platform_usd_credit":30,"enabled":true}}}',
      accounts: [],
    })
    const inputValues = getUpstreamRatioConversionInputValues(ratioConfig)
    const payload = buildUpstreamAccountRefreshPayload({
      previewId: 'preview-1',
      keys: [makeSnapshotKey()],
      configs: {},
      applySuggested: false,
      ratioConversion: buildUpstreamRatioConversionPayload(
        inputValues.paidCny,
        inputValues.platformUsdCredit
      ),
    })

    assert.deepEqual(payload.ratio_conversion, {
      paid_cny: 6,
      platform_usd_credit: 30,
    })
  })

  test('上游资产预览使用实付金额与到账额度换算', () => {
    const display = getUpstreamPreviewBalanceDisplay(
      {
        balance_usd: 269.510572,
        used_usd: 965.10803,
      },
      {
        paid_cny: 1,
        platform_usd_credit: 10,
      }
    )

    assert.equal(display.conversionFactor, 0.1)
    assert.ok(Math.abs((display.balanceUSD ?? 0) - 26.9510572) < 1e-12)
    assert.ok(Math.abs((display.usedUSD ?? 0) - 96.510803) < 1e-12)
  })

  test('无效上游资产换算配置回退为 1', () => {
    assert.equal(
      upstreamAssetConversionFactor({
        paid_cny: 0,
        platform_usd_credit: 10,
      }),
      1
    )
    assert.equal(
      upstreamAssetConversionFactor({
        paid_cny: 1,
        platform_usd_credit: Number.NaN,
      }),
      1
    )
  })

  test('同步渠道优先展示换算后的上游资产，普通渠道继续展示本地资产', () => {
    const syncedChannel = {
      id: 1,
      type: 59,
      key: '',
      name: 'synced',
      status: 1,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      balance: 999,
      balance_updated_time: 0,
      models: '',
      group: 'default',
      used_quota: 123,
      other: '',
      other_info: '',
      remark: '',
      settings:
        '{"upstream_account_sync":{"platform":"sub2api","base_url":"https://upstream.example","synced_at":1}}',
      max_input_tokens: 0,
      channel_info: {
        is_multi_key: false,
        multi_key_size: 0,
        multi_key_polling_index: 0,
        multi_key_mode: 'random',
      },
      upstream_balance_usd: 26.95,
      upstream_used_quota: 96510803,
      upstream_partial: false,
    } satisfies Channel
    const plainChannel = {
      ...syncedChannel,
      type: 1,
      balance: 55,
      used_quota: 777,
      upstream_balance_usd: undefined,
      upstream_used_quota: undefined,
      settings: '{}',
    } satisfies Channel

    assert.deepEqual(getChannelBalanceDisplaySource(syncedChannel), {
      usedQuota: 96510803,
      balanceUSD: 26.95,
      partial: false,
      upstream: true,
    })
    assert.deepEqual(getChannelBalanceDisplaySource(plainChannel), {
      usedQuota: 777,
      balanceUSD: 55,
      partial: false,
      upstream: false,
    })

    assert.deepEqual(
      getChannelBalanceDisplaySource({
        ...syncedChannel,
        upstream_balance_usd: undefined,
        upstream_used_quota: undefined,
        upstream_partial: false,
      }),
      {
        usedQuota: undefined,
        balanceUSD: 999,
        partial: false,
        upstream: true,
      }
    )
  })

  test('同步密钥优先展示换算后的已用和剩余，旧数据回退本地已用', () => {
    const syncedAccount = {
      id: 1,
      channel_id: 1,
      name: 'synced-key',
      key: 'sk-****',
      status: 1,
      models: '',
      group: 'default',
      access_groups: 'default',
      priority: 0,
      weight: 1,
      last_used_time: 0,
      used_quota: 10,
      other: '',
      settings: '{}',
      last_error: '',
      rate_limited_until: 0,
      overload_until: 0,
      temp_disabled_until: 0,
      disabled_reason: '',
      max_concurrency: 0,
      created_time: 0,
      upstream_used_quota: 96_510_803,
      upstream_remaining_quota: 26_951_057,
      upstream_partial: false,
    } satisfies ChannelAccount
    const legacyAccount = {
      ...syncedAccount,
      upstream_used_quota: undefined,
      upstream_remaining_quota: undefined,
      used_quota: 1234,
    } satisfies ChannelAccount

    assert.deepEqual(getChannelAccountAssetDisplaySource(syncedAccount), {
      usedQuota: 96_510_803,
      remainingQuota: 26_951_057,
      upstream: true,
      partial: false,
    })
    assert.deepEqual(getChannelAccountAssetDisplaySource(legacyAccount), {
      usedQuota: 1234,
      remainingQuota: undefined,
      upstream: false,
      partial: false,
    })
  })

  test('使用已保存登录预览时不发送账号密码', () => {
    const payload = buildUpstreamAccountPreviewRequest({
      channelId: 42,
      platform: 'new-api',
      baseUrl: ' https://upstream.example/ ',
      username: 'admin',
      password: 'secret',
      useSavedCredential: true,
      ratioConversion: {
        paid_cny: 1,
        platform_usd_credit: 20,
      },
    })

    assert.deepEqual(payload, {
      platform: 'new-api',
      base_url: 'https://upstream.example/',
      channel_id: 42,
      ratio_conversion: {
        paid_cny: 1,
        platform_usd_credit: 20,
      },
    })
  })

  test('手动登录预览按上游平台写入账号字段', () => {
    const payload = buildUpstreamAccountPreviewRequest({
      platform: 'sub2api',
      baseUrl: 'https://sub2api.example',
      username: 'owner@example.com',
      password: 'secret',
      useSavedCredential: false,
    })

    assert.equal(payload.email, 'owner@example.com')
    assert.equal(payload.username, undefined)
    assert.equal(payload.password, 'secret')
    assert.equal(payload.channel_id, undefined)
  })

  test('应用刷新携带管理员 priority 且不提交同步托管 weight', () => {
    const payload = buildUpstreamAccountRefreshPayload({
      previewId: 'preview-1',
      keys: [makeSnapshotKey()],
      configs: {
        'sync-1': {
          enabled: false,
          priority: 9,
          weight: 40,
          models: 'gpt-local',
          group: 'vip',
        },
      },
      applySuggested: false,
      ratioConversion: {
        paid_cny: 2,
        platform_usd_credit: 10,
      },
    })

    assert.deepEqual(payload, {
      preview_id: 'preview-1',
      apply_suggested: false,
      disable_missing_key: true,
      ratio_conversion: {
        paid_cny: 2,
        platform_usd_credit: 10,
      },
      accounts: [
        {
          sync_id: 'sync-1',
          external_id: 'external-1',
          name: 'upstream key',
          enabled: false,
          models: 'gpt-local',
          group: 'vip',
          access_groups: 'default',
          priority: 9,
        },
      ],
    })
  })

  test('同步密钥摘要只统计启用密钥的模型白名单和访问用户组', () => {
    const summary = summarizeUpstreamAccountCapabilities(
      [
        makeSnapshotKey({
          sync_id: 'sync-enabled',
          external_id: 'sync-enabled',
          models: ['gpt-upstream', 'gpt-shared'],
          access_groups: 'default',
        }),
        makeSnapshotKey({
          sync_id: 'sync-disabled',
          external_id: 'sync-disabled',
          models: ['gpt-disabled'],
          access_groups: 'internal',
        }),
        makeSnapshotKey({
          sync_id: 'sync-empty',
          external_id: 'sync-empty',
          models: ['gpt-empty-fallback'],
          access_groups: 'vip',
        }),
      ],
      {
        'sync-enabled': {
          enabled: true,
          priority: 1,
          weight: 100,
          models: 'gpt-local,gpt-shared',
          group: 'upstream-group',
          access_groups: 'default,vip',
        },
        'sync-disabled': {
          enabled: false,
          priority: 1,
          weight: 100,
          models: 'gpt-disabled',
          group: 'upstream-group',
          access_groups: 'internal',
        },
        'sync-empty': {
          enabled: true,
          priority: 1,
          weight: 100,
          models: '',
          group: 'upstream-group',
          access_groups: '',
        },
      }
    )

    assert.equal(summary.enabledKeyCount, 2)
    assert.equal(summary.totalKeyCount, 3)
    assert.deepEqual(summary.modelNames, ['gpt-local', 'gpt-shared'])
    assert.deepEqual(summary.accessGroups, ['default', 'vip'])
    assert.equal(summary.accessGroupText, 'default,vip')
  })

  test('启用同步密钥必须同时配置模型和 NexusTok 可访问用户组', () => {
    const errors = collectUpstreamAccountCapabilityValidationErrors(
      [makeSnapshotKey({ models: [], access_groups: '' })],
      {
        'sync-1': {
          enabled: true,
          priority: 1,
          weight: 100,
          models: '',
          group: 'upstream-group',
          access_groups: '',
        },
      }
    )

    assert.deepEqual(
      errors.map((error) => error.field),
      ['models', 'access_groups']
    )
    assert.equal(errors[0].keyName, 'upstream key')
  })

  test('禁用同步密钥允许暂存空模型和空访问组', () => {
    const errors = collectUpstreamAccountCapabilityValidationErrors(
      [makeSnapshotKey({ models: [], access_groups: '' })],
      {
        'sync-1': {
          enabled: false,
          priority: 1,
          weight: 100,
          models: '',
          group: 'upstream-group',
          access_groups: '',
        },
      }
    )

    assert.deepEqual(errors, [])
  })

  test('模型同步失败的空模型密钥默认禁用并允许导入', () => {
    const key = makeSnapshotKey({
      models: [],
      key_models_sync_source: 'fetch_models',
      key_models_sync_error: 'stage=fetch_models: INSUFFICIENT_BALANCE',
    })

    assert.equal(shouldEnableUpstreamAccountKeyByDefault(key), false)

    const configs = buildUpstreamAccountConfigsFromSnapshotKeys([key])
    assert.equal(configs['sync-1']?.enabled, false)

    const errors = collectUpstreamAccountCapabilityValidationErrors(
      [key],
      configs
    )
    assert.deepEqual(errors, [])

    const payload = buildUpstreamAccountRefreshPayload({
      previewId: 'preview-model-sync-failure',
      keys: [key],
      configs,
      applySuggested: true,
    })
    assert.equal(payload.accounts?.[0]?.enabled, false)
  })

  test('自定义模型会原样进入同步密钥 payload', () => {
    const payload = buildUpstreamAccountRefreshPayload({
      previewId: 'preview-custom',
      keys: [makeSnapshotKey({ models: ['gpt-upstream'] })],
      configs: {
        'sync-1': {
          enabled: true,
          priority: 1,
          weight: 100,
          models: 'vendor-missing-model,gpt-upstream',
          group: 'upstream-group',
          access_groups: 'default',
        },
      },
      applySuggested: false,
    })

    assert.equal(
      payload.accounts?.[0]?.models,
      'vendor-missing-model,gpt-upstream'
    )
  })

  test('回填上游返回模型时保留密钥原有优先级和访问配置', () => {
    const draft = buildUpstreamAccountConfigDraft(
      makeSnapshotKey({
        suggested_priority: 7,
        suggested_weight: 88,
        models: ['gpt-upstream'],
        access_groups: 'default,vip',
      }),
      {
        enabled: true,
        priority: 3,
        weight: 40,
        models: 'gpt-local',
        group: 'vip',
        access_groups: 'default,vip',
      },
      {
        models: 'gpt-upstream,gpt-shared',
      }
    )

    assert.equal(draft.enabled, true)
    assert.equal(draft.priority, 3)
    assert.equal(draft.weight, 88)
    assert.equal(draft.models, 'gpt-upstream,gpt-shared')
    assert.equal(draft.group, 'vip')
    assert.equal(draft.access_groups, 'default,vip')

    const defaultDraft = buildUpstreamAccountConfigDraft(
      makeSnapshotKey({
        suggested_priority: 7,
        suggested_weight: 66,
      }),
      undefined
    )
    assert.equal(defaultDraft.priority, 0)
    assert.equal(defaultDraft.weight, 66)
  })
})
