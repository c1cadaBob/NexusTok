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
import type { ChannelAccount, UpstreamAccountKey } from '../../types'
import {
  buildUpstreamAccountConfigsFromChannelAccounts,
  buildUpstreamAccountConfigsFromSnapshotKeys,
  getUpstreamAccountConfig,
  upstreamAccountFromChannelAccount,
  upstreamAccountValuesToString,
} from './channel-mutate-drawer'

function makeChannelAccount(
  overrides: Partial<ChannelAccount> = {}
): ChannelAccount {
  return {
    id: 12,
    channel_id: 14,
    name: 'synced key',
    key: 'sk-abc...7890',
    status: 1,
    models: 'gpt-local',
    group: 'local-group',
    priority: 3,
    weight: 40,
    last_used_time: 0,
    used_quota: 0,
    other: '',
    settings:
      '{"upstream_account_sync":{"platform":"sub2api","base_url":"https://sub2api.example","external_id":"9001","key_digest":"digest","synced_at":1}}',
    rate_limited_until: 0,
    overload_until: 0,
    temp_disabled_until: 0,
    disabled_reason: '',
    last_error: '',
    max_concurrency: 0,
    created_time: 0,
    ...overrides,
  }
}

function makeSnapshotKey(
  overrides: Partial<UpstreamAccountKey> = {}
): UpstreamAccountKey {
  return {
    sync_id: '9001',
    external_id: '9001',
    name: 'upstream key',
    masked_key: 'sk-abc...7890',
    group_name: 'upstream-group',
    group_id: 'upstream-group',
    models: ['gpt-upstream'],
    suggested_priority: 1,
    suggested_weight: 100,
    ...overrides,
  }
}

describe('上游同步渠道本地配置索引', () => {
  test('已同步账号使用上游 external_id 作为本地配置标识', () => {
    const editable = upstreamAccountFromChannelAccount(makeChannelAccount())

    assert.equal(editable.sync_id, '9001')
    assert.equal(editable.external_id, '9001')
    assert.equal(editable.masked_key, 'sk-abc...7890')
  })

  test('刷新快照复用已保存账号配置而不是覆盖回上游建议值', () => {
    const existingConfigs = buildUpstreamAccountConfigsFromChannelAccounts([
      makeChannelAccount(),
    ])
    const refreshedKey = makeSnapshotKey({
      group_name: 'upstream-group',
      models: ['gpt-upstream'],
      suggested_priority: 9,
      suggested_weight: 10,
    })

    const nextConfigs = buildUpstreamAccountConfigsFromSnapshotKeys(
      [refreshedKey],
      existingConfigs
    )
    const config = getUpstreamAccountConfig(nextConfigs, refreshedKey, 0)

    assert.deepEqual(config, {
      priority: 3,
      weight: 40,
      enabled: true,
      models: 'gpt-local',
      group: 'local-group',
    })
  })

  test('渠道级能力汇总忽略已禁用的同步密钥', () => {
    const enabledKey = makeSnapshotKey({
      sync_id: 'enabled',
      external_id: 'enabled',
      models: ['gpt-enabled'],
      group_name: 'enabled-group',
    })
    const disabledKey = makeSnapshotKey({
      sync_id: 'disabled',
      external_id: 'disabled',
      models: ['gpt-disabled'],
      group_name: 'disabled-group',
    })
    const configs = {
      enabled: {
        enabled: true,
        priority: 1,
        weight: 100,
        models: 'gpt-enabled',
        group: 'enabled-group',
      },
      disabled: {
        enabled: false,
        priority: 1,
        weight: 100,
        models: 'gpt-disabled',
        group: 'disabled-group',
      },
    }

    const models = upstreamAccountValuesToString(
      [enabledKey, disabledKey],
      configs,
      (key, config) => config?.models ?? key.models?.join(',') ?? ''
    )
    const groups = upstreamAccountValuesToString(
      [enabledKey, disabledKey],
      configs,
      (key, config) => config?.group ?? key.group_name ?? key.group_id ?? ''
    )

    assert.equal(models, 'gpt-enabled')
    assert.equal(groups, 'enabled-group')
  })
})
