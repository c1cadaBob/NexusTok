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
import {
  parseSidebarModulesAdmin,
  serializeSidebarModulesAdmin,
} from './config'

const EXPECTED_ADMIN_MODULE_ORDER = [
  'channel',
  'account_pool',
  'models',
  'pricing',
  'user',
  'subscription',
  'redemption',
  'setting',
  'system_info',
]

const getAdminModuleKeys = (raw: string | null | undefined): string[] => {
  const config = parseSidebarModulesAdmin(raw)
  return Object.keys(config.admin).filter((key) => key !== 'enabled')
}

describe('SidebarModulesAdmin 配置', () => {
  test('旧顺序配置会归一化为管理员工作流顺序', () => {
    const legacyConfig = JSON.stringify({
      admin: {
        enabled: true,
        channel: true,
        account_pool: true,
        pricing: true,
        models: true,
        redemption: true,
        user: true,
        setting: true,
        subscription: true,
        system_info: true,
      },
    })

    assert.deepEqual(
      getAdminModuleKeys(legacyConfig),
      EXPECTED_ADMIN_MODULE_ORDER
    )
  })

  test('缺失模块按默认值补齐并保持默认顺序', () => {
    const partialConfig = JSON.stringify({
      admin: {
        enabled: true,
        channel: false,
      },
    })
    const parsed = parseSidebarModulesAdmin(partialConfig)

    assert.deepEqual(Object.keys(parsed.admin), [
      'enabled',
      ...EXPECTED_ADMIN_MODULE_ORDER,
    ])
    assert.equal(parsed.admin.channel, false)
    assert.equal(parsed.admin.models, true)
    assert.equal(parsed.admin.system_info, true)
  })

  test('未知自定义模块会保留在已知模块之后', () => {
    const configWithCustomModule = JSON.stringify({
      admin: {
        enabled: true,
        custom_audit: true,
        redemption: false,
        models: true,
      },
    })
    const parsed = parseSidebarModulesAdmin(configWithCustomModule)

    assert.deepEqual(Object.keys(parsed.admin), [
      'enabled',
      ...EXPECTED_ADMIN_MODULE_ORDER,
      'custom_audit',
    ])
    assert.equal(parsed.admin.custom_audit, true)
    assert.equal(parsed.admin.redemption, false)
  })

  test('序列化时同样归一化模块顺序', () => {
    const serialized = serializeSidebarModulesAdmin({
      admin: {
        enabled: true,
        system_info: true,
        setting: true,
        custom_audit: false,
        channel: true,
      },
    })

    assert.deepEqual(getAdminModuleKeys(serialized), [
      ...EXPECTED_ADMIN_MODULE_ORDER,
      'custom_audit',
    ])
  })
})
