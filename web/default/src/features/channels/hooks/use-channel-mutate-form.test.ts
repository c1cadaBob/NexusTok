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
import type { Channel } from '../types'
import {
  buildAllowedChannelUpdatePayload,
  pickNonSensitiveChannelUpdatePayload,
} from './use-channel-mutate-form'

function makeUpdatePayload(): Partial<Channel> & Record<string, unknown> {
  return {
    id: 7,
    name: 'OpenAI Main',
    type: 1,
    key: 'sk-new',
    base_url: 'https://api.openai.com',
    openai_organization: 'org-test',
    models: 'gpt-5.6-sol',
    group: 'default',
    model_mapping: '{"client":"upstream"}',
    priority: 10,
    weight: 20,
    test_model: 'gpt-5.6-sol',
    auto_ban: 1,
    status: 1,
    status_code_mapping: '{"429":500}',
    tag: 'prod',
    remark: 'visible note',
    setting: '{"proxy":"http://proxy"}',
    param_override: '{"temperature":0}',
    header_override: '{"X-Test":"1"}',
    settings: '{"allow_service_tier":true}',
    other: '2024-02-01',
    other_info: 'runtime metadata',
    multi_key_mode: 'random',
    channel_info: {
      is_multi_key: true,
      multi_key_size: 2,
      multi_key_polling_index: 0,
      multi_key_mode: 'random',
    },
    unexpected_sensitive_field: 'drop-me',
  }
}

describe('渠道更新权限 payload 裁剪', () => {
  test('普通写权限只保留非敏感更新字段', () => {
    const allowed = pickNonSensitiveChannelUpdatePayload(makeUpdatePayload())

    assert.deepEqual(Object.keys(allowed).sort(), [
      'auto_ban',
      'group',
      'id',
      'model_mapping',
      'models',
      'multi_key_mode',
      'name',
      'other_info',
      'priority',
      'remark',
      'status_code_mapping',
      'tag',
      'test_model',
      'weight',
    ])
    assert.equal('key' in allowed, false)
    assert.equal('base_url' in allowed, false)
    assert.equal('setting' in allowed, false)
    assert.equal('settings' in allowed, false)
    assert.equal('param_override' in allowed, false)
    assert.equal('header_override' in allowed, false)
    assert.equal('status' in allowed, false)
    assert.equal('channel_info' in allowed, false)
    assert.equal('unexpected_sensitive_field' in allowed, false)
  })

  test('敏感写权限保留完整更新 payload 并允许多 Key 覆盖模式', () => {
    const payload = makeUpdatePayload()
    const allowed = buildAllowedChannelUpdatePayload({
      payload,
      canEditSensitiveFields: true,
      isMultiKeyChannel: true,
      keyMode: 'replace',
    })

    assert.equal(allowed.key, 'sk-new')
    assert.equal(allowed.base_url, 'https://api.openai.com')
    assert.equal(allowed.settings, '{"allow_service_tier":true}')
    assert.equal(allowed.param_override, '{"temperature":0}')
    assert.equal(allowed.header_override, '{"X-Test":"1"}')
    assert.equal(allowed.key_mode, 'replace')
  })

  test('普通写权限不会携带多 Key key_mode 或未知字段', () => {
    const allowed = buildAllowedChannelUpdatePayload({
      payload: makeUpdatePayload(),
      canEditSensitiveFields: false,
      isMultiKeyChannel: true,
      keyMode: 'replace',
    })

    assert.equal('key_mode' in allowed, false)
    assert.equal('unexpected_sensitive_field' in allowed, false)
    assert.equal(allowed.models, 'gpt-5.6-sol')
  })

  test('非多 Key 渠道即使有敏感写权限也不会附加 key_mode', () => {
    const allowed = buildAllowedChannelUpdatePayload({
      payload: makeUpdatePayload(),
      canEditSensitiveFields: true,
      isMultiKeyChannel: false,
      keyMode: 'replace',
    })

    assert.equal('key_mode' in allowed, false)
  })
})
