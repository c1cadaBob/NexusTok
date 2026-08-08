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
import type { UpstreamAccountKey } from '../types'
import {
  buildUpstreamAccountConfigDraft,
  buildUpstreamAccountPreviewRequest,
  buildUpstreamAccountRefreshPayload,
  collectUpstreamAccountCapabilityValidationErrors,
  summarizeUpstreamAccountCapabilities,
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

describe('上游账号刷新共享 payload', () => {
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

  test('应用刷新默认禁用缺失 key 并携带逐 key 配置', () => {
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
          weight: 40,
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
    assert.equal(draft.weight, 40)
    assert.equal(draft.models, 'gpt-upstream,gpt-shared')
    assert.equal(draft.group, 'vip')
    assert.equal(draft.access_groups, 'default,vip')
  })
})
