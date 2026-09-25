/*
Copyright (C) 2023-2026 c1cadaBob

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

For commercial licensing, please contact support@c1cadabob.dev
*/
import { describe, expect, test } from 'vitest'

import {
  CHANNEL_TYPE_NEW_API,
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_SUB2_API,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import type { Channel } from '../../types'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  channelFormSchema,
  transformChannelToFormDefaults,
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
} from '../channel-form'
import { getChannelTypeConfig } from '../channel-type-config'
import { getChannelTypeIcon, getKeyPromptForType } from '../channel-utils'

function newAPIForm(baseUrl: string) {
  return {
    ...CHANNEL_FORM_DEFAULT_VALUES,
    name: 'New API upstream',
    type: CHANNEL_TYPE_NEW_API,
    base_url: baseUrl,
    key: 'test-key',
    models: 'gpt-5',
  }
}

describe('New API channel', () => {
  test('registers selection, ordering, model discovery, and icon metadata', () => {
    const option = CHANNEL_TYPE_OPTIONS.find(
      (item) => item.value === CHANNEL_TYPE_NEW_API
    )

    expect(option).toEqual({
      value: CHANNEL_TYPE_NEW_API,
      label: 'New API',
    })
    expect(
      CHANNEL_TYPE_OPTIONS.findIndex(
        (item) => item.value === CHANNEL_TYPE_NEW_API
      ) + 1
    ).toBe(CHANNEL_TYPE_OPTIONS.findIndex((item) => item.value === 58))
    expect(MODEL_FETCHABLE_TYPES.has(CHANNEL_TYPE_NEW_API)).toBe(true)
    expect(getChannelTypeIcon(CHANNEL_TYPE_NEW_API)).toBe('NewAPI')
    expect(getKeyPromptForType(CHANNEL_TYPE_NEW_API)).toBe(
      'Enter API key for this channel'
    )
    expect(getChannelTypeConfig(CHANNEL_TYPE_NEW_API).icon).toBe('NewAPI')
  })

  test('requires a non-blank Base URL', () => {
    const blankResult = channelFormSchema.safeParse(newAPIForm('  '))

    expect(blankResult.success).toBe(false)
    if (!blankResult.success) {
      expect(
        blankResult.error.issues.some(
          (issue) =>
            issue.path[0] === 'base_url' &&
            issue.message === 'Base URL is required for this channel type'
        )
      ).toBe(true)
    }

    expect(
      channelFormSchema.safeParse(newAPIForm('https://nexustok.example'))
        .success
    ).toBe(true)
  })

  test('keeps Sub2API Base URL validation unchanged', () => {
    const result = channelFormSchema.safeParse({
      ...newAPIForm(''),
      type: 59,
    })

    expect(result.success).toBe(true)
  })

  test('validates platform site fields and keeps the selected platform payload', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Synthetic platform site',
      type: CHANNEL_TYPE_NEW_API,
      upstream_kind: 'platform_site',
      platform_site_platform: 'newapi',
      platform_site_auth_type: 'password',
      platform_site_username: 'synthetic-user',
      platform_site_password: 'synthetic-password',
      base_url: 'https://upstream.example',
      models: 'gpt-4o',
      platform_site_recharge_amount: 1,
      platform_site_credited_amount: 10,
    })

    expect(result.success).toBe(true)
    if (!result.success) return

    const payload = transformFormDataToCreatePayload(result.data)
    expect(payload.channel.upstream_kind).toBe('platform_site')
    expect(payload.platform_site).toMatchObject({
      platform: 'newapi',
      base_url: 'https://upstream.example',
      recharge_amount: 1,
      credited_amount: 10,
    })
    expect(payload.platform_site?.conversion_ratio).toBeUndefined()
  })

  test('sends an explicit platform ratio only after a manual override', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Manual ratio site',
      type: CHANNEL_TYPE_NEW_API,
      upstream_kind: 'platform_site',
      base_url: 'https://upstream.example',
      models: 'gpt-4o',
      platform_site_recharge_amount: 1,
      platform_site_credited_amount: 10,
      platform_site_conversion_ratio: 1,
      platform_site_conversion_ratio_override: true,
    })

    expect(result.success).toBe(true)
    if (!result.success) return

    const payload = transformFormDataToCreatePayload(result.data)
    expect(payload.platform_site?.conversion_ratio).toBe(1)
  })

  test('keeps the complete password credential in an update payload', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Updated Sub2API site',
      type: CHANNEL_TYPE_SUB2_API,
      upstream_kind: 'platform_site',
      platform_site_platform: 'sub2api',
      platform_site_auth_type: 'password',
      platform_site_username: 'operator@example.com',
      platform_site_password: 'synthetic-password',
      base_url: 'https://upstream.example',
      platform_site_recharge_amount: 1,
      platform_site_credited_amount: 1,
    })

    expect(result.success).toBe(true)
    if (!result.success) return

    const payload = transformFormDataToUpdatePayload(result.data, 42)
    expect(payload.platform_site).toMatchObject({
      platform: 'sub2api',
      auth_type: 'password',
      username: 'operator@example.com',
      password: 'synthetic-password',
    })
  })

  test('restricts platform sites to NewAPI and Sub2API channel types', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Invalid platform site',
      type: 1,
      upstream_kind: 'platform_site',
      base_url: 'https://upstream.example',
      models: 'gpt-4o',
    })

    expect(result.success).toBe(false)
    if (!result.success) {
      expect(
        result.error.issues.some(
          (issue) =>
            issue.path[0] === 'type' &&
            issue.message === '平台站点仅支持 NewAPI 或 Sub2API'
        )
      ).toBe(true)
    }
  })

  test('supports Sub2API as a platform site type', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Synthetic Sub2API site',
      type: CHANNEL_TYPE_SUB2_API,
      upstream_kind: 'platform_site',
      platform_site_platform: 'sub2api',
      platform_site_auth_type: 'admin_key',
      platform_site_admin_key: 'synthetic-admin-key',
      base_url: 'https://upstream.example',
      models: 'gpt-4o',
      platform_site_recharge_amount: 1,
      platform_site_credited_amount: 1,
    })

    expect(result.success).toBe(true)
  })

  test('sends only capture_id for automatic platform authentication', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Captured platform site',
      type: CHANNEL_TYPE_SUB2_API,
      upstream_kind: 'platform_site',
      platform_site_platform: 'sub2api',
      platform_site_auth_type: 'auto',
      platform_site_capture_id: 'capture-123',
      platform_site_access_token: 'should-not-be-sent',
      base_url: 'http://127.0.0.1:8080',
      models: 'gpt-4o',
      platform_site_recharge_amount: 1,
      platform_site_credited_amount: 1,
    })

    expect(payload.platform_site).toMatchObject({
      auth_type: 'auto',
      capture_id: 'capture-123',
    })
    expect(payload.platform_site?.access_token).toBeUndefined()
    expect(payload.platform_site?.admin_key).toBeUndefined()
    expect(payload.platform_site?.cookie).toBeUndefined()
  })

  test('keeps password credential while sending optional capture_id', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Password platform site with cached login',
      type: CHANNEL_TYPE_SUB2_API,
      upstream_kind: 'platform_site',
      platform_site_platform: 'sub2api',
      platform_site_auth_type: 'password',
      platform_site_username: 'synthetic-user',
      platform_site_password: 'synthetic-password',
      platform_site_capture_id: 'capture-password-123',
      base_url: 'http://127.0.0.1:8080',
      models: 'gpt-4o',
      platform_site_recharge_amount: 1,
      platform_site_credited_amount: 1,
    })

    expect(payload.platform_site).toMatchObject({
      auth_type: 'password',
      username: 'synthetic-user',
      password: 'synthetic-password',
      capture_id: 'capture-password-123',
    })
    expect(payload.platform_site?.access_token).toBeUndefined()
    expect(payload.platform_site?.admin_key).toBeUndefined()
    expect(payload.platform_site?.cookie).toBeUndefined()
  })

  test('rejects mismatched platform site channel type and platform', () => {
    const result = channelFormSchema.safeParse({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Mismatched platform site',
      type: CHANNEL_TYPE_NEW_API,
      upstream_kind: 'platform_site',
      platform_site_platform: 'sub2api',
      platform_site_auth_type: 'password',
      platform_site_username: 'operator@example.com',
      platform_site_password: 'synthetic-password',
      base_url: 'https://upstream.example',
      platform_site_recharge_amount: 1,
      platform_site_credited_amount: 1,
    })

    expect(result.success).toBe(false)
    if (!result.success) {
      expect(
        result.error.issues.some(
          (issue) =>
            issue.path[0] === 'type' &&
            issue.message === '平台站点仅支持 NewAPI 或 Sub2API'
        )
      ).toBe(true)
    }
  })

  test('normalizes platform site payload type from selected platform', () => {
    const payload = transformFormDataToCreatePayload({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Sub2API site normalized at submit',
      type: CHANNEL_TYPE_NEW_API,
      upstream_kind: 'platform_site',
      platform_site_platform: 'sub2api',
      platform_site_auth_type: 'password',
      platform_site_username: 'operator@example.com',
      platform_site_password: 'synthetic-password',
      base_url: 'https://upstream.example',
      platform_site_recharge_amount: 1,
      platform_site_credited_amount: 1,
    })

    expect(payload.channel.type).toBe(CHANNEL_TYPE_SUB2_API)
    expect(payload.platform_site?.platform).toBe('sub2api')
  })

  test('preserves an explicit zero conversion ratio for free official keys', () => {
    const defaults = transformChannelToFormDefaults({
      ...newAPIForm('https://upstream.example'),
      group: 'default',
      channel_info: {
        multi_key_mode: 'random',
      },
      conversion_ratio: 0,
    } as unknown as Channel)

    expect(defaults.conversion_ratio).toBe(0)
  })
})
