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
import type { PermissionCatalog } from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import {
  AUTHZ_ROLE_ADMIN_VALUE,
  transformFormDataToPayload,
  transformUserToFormDefaults,
  type UserFormValues,
} from './user-form'

const catalog: PermissionCatalog = {
  resources: [
    {
      resource: 'channel',
      label_key: 'Channels',
      actions: [
        {
          action: 'read',
          label_key: 'Read',
          description_key: 'Read channel metadata.',
        },
        {
          action: 'write',
          label_key: 'Write',
          description_key: 'Update channel metadata.',
        },
      ],
    },
  ],
  roles: [
    {
      key: AUTHZ_ROLE_ADMIN_VALUE,
      name: 'Admin',
      built_in: true,
      superuser: false,
      grants: {
        channel: {
          read: true,
          write: true,
        },
      },
    },
  ],
}

function adminForm(overrides: Partial<UserFormValues> = {}): UserFormValues {
  return {
    username: 'scoped-admin',
    display_name: 'Scoped Admin',
    password: '',
    role: ROLE.ADMIN,
    group: 'default',
    remark: '',
    authz_role: AUTHZ_ROLE_ADMIN_VALUE,
    admin_permissions: {
      channel: {
        read: true,
        write: false,
      },
    },
    ...overrides,
  }
}

describe('用户表单授权角色 payload', () => {
  test('内置 Admin 基线保存为空角色并提交完整权限矩阵', () => {
    const payload = transformFormDataToPayload(
      adminForm({ authz_role: AUTHZ_ROLE_ADMIN_VALUE }),
      42,
      catalog
    )

    assert.equal(payload.authz_role, '')
    assert.deepEqual(payload.admin_permissions, {
      channel: {
        read: true,
        write: false,
      },
    })
  })

  test('自定义角色基线按规范化 key 写入 payload', () => {
    const payload = transformFormDataToPayload(
      adminForm({ authz_role: ' Auditor ' }),
      42,
      catalog
    )

    assert.equal(payload.authz_role, 'auditor')
  })

  test('权限 catalog 不可用时不提交授权字段，避免覆盖后端现有设置', () => {
    const payload = transformFormDataToPayload(
      adminForm({ authz_role: 'auditor' }),
      42
    )

    assert.equal(payload.authz_role, undefined)
    assert.equal(payload.admin_permissions, undefined)
  })

  test('用户详情为空角色时回填内置 Admin 基线', () => {
    const defaults = transformUserToFormDefaults({
      id: 42,
      username: 'scoped-admin',
      display_name: 'Scoped Admin',
      role: ROLE.ADMIN,
      status: 1,
      quota: 0,
      used_quota: 0,
      request_count: 0,
      group: 'default',
      authz_role: '',
      created_at: 0,
    })

    assert.equal(defaults.authz_role, AUTHZ_ROLE_ADMIN_VALUE)
  })
})
