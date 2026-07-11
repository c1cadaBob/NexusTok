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
import {
  countEnabledActions,
  countResourceEnabledActions,
  diffPermissionMatrix,
  normalizeRolePolicyGrants,
  permissionMatrixSignature,
  replaceResourceGrants,
} from './role-policy-utils'

const catalog: PermissionCatalog = {
  resources: [
    {
      resource: 'channel',
      label_key: 'Channels',
      actions: [
        {
          action: 'read',
          label_key: 'Read',
          description_key: 'Read channels',
        },
        {
          action: 'write',
          label_key: 'Write',
          description_key: 'Write channels',
        },
      ],
    },
    {
      resource: 'system_setting',
      label_key: 'System Settings',
      actions: [
        {
          action: 'read',
          label_key: 'Read',
          description_key: 'Read settings',
        },
        {
          action: 'sensitive_write',
          label_key: 'Sensitive write',
          description_key: 'Write sensitive settings',
        },
      ],
    },
  ],
  roles: [],
}

describe('role policy utils', () => {
  test('normalizes a partial matrix into the full backend catalog shape', () => {
    const normalized = normalizeRolePolicyGrants(
      {
        channel: {
          read: true,
        },
      },
      catalog
    )

    assert.deepEqual(normalized, {
      channel: {
        read: true,
        write: false,
      },
      system_setting: {
        read: false,
        sensitive_write: false,
      },
    })
  })

  test('replaces one resource row without losing other resources', () => {
    const normalized = normalizeRolePolicyGrants(
      {
        channel: {
          read: true,
          write: false,
        },
        system_setting: {
          read: true,
          sensitive_write: false,
        },
      },
      catalog
    )

    const next = replaceResourceGrants(
      normalized,
      catalog,
      'system_setting',
      true
    )

    assert.deepEqual(next.channel, {
      read: true,
      write: false,
    })
    assert.deepEqual(next.system_setting, {
      read: true,
      sensitive_write: true,
    })
    assert.equal(
      countResourceEnabledActions(next, catalog, 'system_setting'),
      2
    )
  })

  test('counts changed, newly enabled, and disabled permissions', () => {
    const before = normalizeRolePolicyGrants(
      {
        channel: {
          read: true,
          write: true,
        },
      },
      catalog
    )
    const after = normalizeRolePolicyGrants(
      {
        channel: {
          read: true,
          write: false,
        },
        system_setting: {
          read: true,
        },
      },
      catalog
    )

    assert.deepEqual(diffPermissionMatrix(before, after, catalog), {
      changed: 2,
      enabled: 1,
      disabled: 1,
    })
    assert.equal(countEnabledActions(after, catalog), 2)
    assert.equal(
      permissionMatrixSignature(after, catalog),
      'channel:10|system_setting:10'
    )
  })
})
