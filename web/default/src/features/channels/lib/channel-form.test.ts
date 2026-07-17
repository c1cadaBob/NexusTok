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
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToUpdatePayload,
  type ChannelFormValues,
} from './channel-form'

function makeValidChannelForm(
  overrides: Partial<ChannelFormValues> = {}
): ChannelFormValues {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'Test Channel',
    key: 'sk-test',
    models: 'gpt-4o',
    group: ['default'],
    ...overrides,
  }
}

function getIssuePaths(result: ReturnType<typeof channelFormSchema.safeParse>) {
  if (result.success) return []
  return result.error.issues.map((issue) => issue.path.join('.'))
}

describe('渠道表单 JSON 字段校验', () => {
  test('允许空 JSON 字段和 JSON object', () => {
    const result = channelFormSchema.safeParse(
      makeValidChannelForm({
        setting: '{"proxy":""}',
        param_override: '{"temperature":0}',
        header_override: '{"X-Test":"1"}',
        settings: '{"vertex_key_type":"json"}',
      })
    )

    assert.equal(result.success, true)
  })

  test('拒绝 JSON 数组作为 object 配置', () => {
    const result = channelFormSchema.safeParse(
      makeValidChannelForm({ settings: '[]' })
    )

    assert.equal(result.success, false)
    assert.deepEqual(getIssuePaths(result), ['settings'])
  })

  test('拒绝非法 JSON 配置', () => {
    const result = channelFormSchema.safeParse(
      makeValidChannelForm({ param_override: '{"temperature":' })
    )

    assert.equal(result.success, false)
    assert.deepEqual(getIssuePaths(result), ['param_override'])
  })
})

describe('渠道表单映射字段校验', () => {
  test('模型映射必须是 string 到 string 的 JSON object', () => {
    const valid = channelFormSchema.safeParse(
      makeValidChannelForm({
        model_mapping: '{"client-model":"upstream-model"}',
      })
    )
    const invalid = channelFormSchema.safeParse(
      makeValidChannelForm({ model_mapping: '{"client-model":123}' })
    )

    assert.equal(valid.success, true)
    assert.equal(invalid.success, false)
    assert.deepEqual(getIssuePaths(invalid), ['model_mapping'])
  })

  test('状态码映射必须使用合法 HTTP 状态码', () => {
    const valid = channelFormSchema.safeParse(
      makeValidChannelForm({ status_code_mapping: '{"429":500}' })
    )
    const invalid = channelFormSchema.safeParse(
      makeValidChannelForm({ status_code_mapping: '{"99":700}' })
    )

    assert.equal(valid.success, true)
    assert.equal(invalid.success, false)
    assert.deepEqual(getIssuePaths(invalid), ['status_code_mapping'])
  })
})

describe('渠道类型专属表单校验', () => {
  test('普通渠道仍要求填写模型', () => {
    const result = channelFormSchema.safeParse(
      makeValidChannelForm({ models: '' })
    )

    assert.equal(result.success, false)
    assert.deepEqual(getIssuePaths(result), ['models'])
  })

  test('上游账号同步创建允许先不填写模型', () => {
    const result = channelFormSchema.safeParse(
      makeValidChannelForm({
        models: '',
        upstream_account_sync: true,
      })
    )

    assert.equal(result.success, true)
  })

  test('指定渠道类型要求填写 Base URL', () => {
    const result = channelFormSchema.safeParse(
      makeValidChannelForm({
        type: 3,
        base_url: '',
        other: '2024-02-01',
      })
    )

    assert.equal(result.success, false)
    assert.deepEqual(getIssuePaths(result), ['base_url'])
  })

  test('账号池组模式豁免本地 Base URL 必填校验', () => {
    const result = channelFormSchema.safeParse(
      makeValidChannelForm({
        type: 3,
        base_url: '',
        other: '',
        credential_mode: 'global_account_pool',
        account_pool_group_id: 1,
      })
    )

    assert.equal(result.success, true)
  })

  test('指定渠道类型要求填写附加配置', () => {
    const result = channelFormSchema.safeParse(
      makeValidChannelForm({
        type: 18,
        other: '',
      })
    )

    assert.equal(result.success, false)
    assert.deepEqual(getIssuePaths(result), ['other'])
  })

  test('Codex 渠道不允许批量创建', () => {
    const result = channelFormSchema.safeParse(
      makeValidChannelForm({
        type: 57,
        multi_key_mode: 'batch',
        key: '',
      })
    )

    assert.equal(result.success, false)
    assert.deepEqual(getIssuePaths(result), ['multi_key_mode'])
  })

  test('Codex 手动凭证必须包含 access_token 和 account_id', () => {
    const invalid = channelFormSchema.safeParse(
      makeValidChannelForm({
        type: 57,
        key: '{"access_token":"token"}',
      })
    )
    const valid = channelFormSchema.safeParse(
      makeValidChannelForm({
        type: 57,
        key: '{"access_token":"token","account_id":"acct"}',
      })
    )

    assert.equal(invalid.success, false)
    assert.deepEqual(getIssuePaths(invalid), ['key'])
    assert.equal(valid.success, true)
  })

  test('Vertex AI 服务账号模式要求 JSON key', () => {
    const invalid = channelFormSchema.safeParse(
      makeValidChannelForm({
        type: 41,
        other: 'us-central1',
        vertex_key_type: 'json',
        key: 'plain-text-key',
      })
    )
    const valid = channelFormSchema.safeParse(
      makeValidChannelForm({
        type: 41,
        other: 'us-central1',
        vertex_key_type: 'json',
        key: '{"type":"service_account"}',
      })
    )

    assert.equal(invalid.success, false)
    assert.deepEqual(getIssuePaths(invalid), ['key'])
    assert.equal(valid.success, true)
  })

  test('Vertex AI API Key 模式不允许批量创建', () => {
    const result = channelFormSchema.safeParse(
      makeValidChannelForm({
        type: 41,
        other: 'us-central1',
        vertex_key_type: 'api_key',
        multi_key_mode: 'batch',
      })
    )

    assert.equal(result.success, false)
    assert.deepEqual(getIssuePaths(result), ['multi_key_mode'])
  })

  test('NexusTok 账号池组模式仍要求选择账号池组', () => {
    const result = channelFormSchema.safeParse(
      makeValidChannelForm({
        credential_mode: 'global_account_pool',
        account_pool_group_id: 0,
      })
    )

    assert.equal(result.success, false)
    assert.deepEqual(getIssuePaths(result), ['account_pool_group_id'])
  })
})

describe('渠道表单 settings 转换', () => {
  test('从 settings JSON 回填跳过异步任务轮询等待开关', () => {
    const channel: Channel = {
      id: 1,
      type: 1,
      key: '',
      openai_organization: '',
      test_model: '',
      status: 1,
      name: 'Test Channel',
      weight: 0,
      created_time: 0,
      test_time: 0,
      response_time: 0,
      base_url: 'https://api.openai.com',
      other: '',
      balance: 0,
      balance_updated_time: 0,
      models: 'gpt-4o',
      group: 'default',
      used_quota: 0,
      model_mapping: '',
      status_code_mapping: '',
      priority: 0,
      auto_ban: 1,
      other_info: '',
      tag: '',
      setting: '{}',
      param_override: '',
      header_override: '',
      remark: '',
      max_input_tokens: 0,
      channel_info: {
        is_multi_key: false,
        multi_key_size: 0,
        multi_key_polling_index: 0,
        multi_key_mode: 'random',
      },
      settings: '{"disable_task_polling_sleep":true}',
    }

    const values = transformChannelToFormDefaults(channel)

    assert.equal(values.disable_task_polling_sleep, true)
  })

  test('保存跳过异步任务轮询等待开关到 settings JSON', () => {
    const payload = transformFormDataToUpdatePayload(
      makeValidChannelForm({
        disable_task_polling_sleep: true,
      }),
      1
    )

    const settings = JSON.parse(String(payload.settings || '{}')) as Record<
      string,
      unknown
    >

    assert.equal(settings.disable_task_polling_sleep, true)
  })

  test('Codex 渠道保存 OpenAI 兼容字段透传开关', () => {
    const payload = transformFormDataToUpdatePayload(
      makeValidChannelForm({
        type: 57,
        allow_service_tier: true,
        disable_store: true,
        allow_safety_identifier: true,
        allow_include_obfuscation: true,
        allow_inference_geo: true,
      }),
      1
    )

    const settings = JSON.parse(String(payload.settings || '{}')) as Record<
      string,
      unknown
    >

    assert.equal(settings.allow_service_tier, true)
    assert.equal(settings.disable_store, true)
    assert.equal(settings.allow_safety_identifier, true)
    assert.equal(settings.allow_include_obfuscation, true)
    assert.equal(settings.allow_inference_geo, true)
  })

  test('非 OpenAI 兼容渠道会清理历史字段透传开关', () => {
    const payload = transformFormDataToUpdatePayload(
      makeValidChannelForm({
        type: 33,
        settings: JSON.stringify({
          allow_service_tier: true,
          disable_store: true,
          allow_safety_identifier: true,
          allow_include_obfuscation: true,
          allow_inference_geo: true,
          allow_speed: true,
          claude_beta_query: true,
        }),
      }),
      1
    )

    const settings = JSON.parse(String(payload.settings || '{}')) as Record<
      string,
      unknown
    >

    assert.equal('allow_service_tier' in settings, false)
    assert.equal('disable_store' in settings, false)
    assert.equal('allow_safety_identifier' in settings, false)
    assert.equal('allow_include_obfuscation' in settings, false)
    assert.equal('allow_inference_geo' in settings, false)
    assert.equal('allow_speed' in settings, false)
    assert.equal('claude_beta_query' in settings, false)
  })
})

describe('渠道表单更新 payload', () => {
  test('更新时规范化 Base URL 并显式发送可清空字段', () => {
    const payload = transformFormDataToUpdatePayload(
      makeValidChannelForm({
        base_url: ' https://api.example.com/// ',
        openai_organization: '',
        test_model: '',
        tag: '',
        remark: '',
        model_mapping: '',
        status_code_mapping: '',
        param_override: '',
        header_override: '',
      }),
      7
    )

    assert.equal(payload.base_url, 'https://api.example.com')
    assert.equal(payload.openai_organization, '')
    assert.equal(payload.test_model, '')
    assert.equal(payload.tag, '')
    assert.equal(payload.remark, '')
    assert.equal(payload.model_mapping, '')
    assert.equal(payload.status_code_mapping, '')
    assert.equal(payload.param_override, '')
    assert.equal(payload.header_override, '')
  })

  test('账号池组模式更新时显式清空本地 Base URL 并保留哨兵 key', () => {
    const payload = transformFormDataToUpdatePayload(
      makeValidChannelForm({
        credential_mode: 'global_account_pool',
        account_pool_group_id: 12,
        base_url: 'https://api.example.com',
      }),
      7
    )

    assert.equal(payload.base_url, '')
    assert.equal(payload.key, 'global_account_pool')
    assert.equal(payload.channel_info?.account_pool_group_id, 12)
  })
})
