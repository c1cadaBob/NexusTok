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
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  canReadAdminResource,
  hasAdminPermission,
} from './admin-permissions'

describe('admin permission helpers', () => {
  test('uses backend permission matrix before role fallback', () => {
    const user = {
      id: 10,
      username: 'limited-admin',
      role: 10,
      permissions: {
        admin_permissions: {
          channel: {
            read: false,
            write: true,
          },
        },
      },
    }

    assert.equal(
      canReadAdminResource(user, ADMIN_PERMISSION_RESOURCES.CHANNEL),
      false
    )
    assert.equal(
      hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL,
        ADMIN_PERMISSION_ACTIONS.WRITE
      ),
      true
    )
  })

  test('keeps legacy admin users readable when matrix is absent', () => {
    const user = {
      id: 11,
      username: 'legacy-admin',
      role: 10,
    }

    assert.equal(
      canReadAdminResource(user, ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL),
      true
    )
    assert.equal(
      canReadAdminResource(user, ADMIN_PERMISSION_RESOURCES.CHANNEL_ACCOUNT),
      true
    )
    assert.equal(
      hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL_ACCOUNT,
        ADMIN_PERMISSION_ACTIONS.OPERATE
      ),
      true
    )
    assert.equal(
      hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL_ACCOUNT,
        ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
      ),
      false
    )
    assert.equal(
      canReadAdminResource(
        user,
        ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL_AUTH_FILE
      ),
      true
    )
    assert.equal(
      hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL_AUTH_FILE,
        ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
      ),
      false
    )
    assert.equal(
      canReadAdminResource(user, ADMIN_PERMISSION_RESOURCES.REDEMPTION),
      true
    )
    assert.equal(
      canReadAdminResource(user, ADMIN_PERMISSION_RESOURCES.USAGE_LOG),
      true
    )
    assert.equal(
      canReadAdminResource(user, ADMIN_PERMISSION_RESOURCES.USAGE_DATA),
      true
    )
    assert.equal(
      hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL,
        ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
      ),
      false
    )
  })

  test('denies common users without explicit grants', () => {
    const user = {
      id: 12,
      username: 'common-user',
      role: 1,
    }

    assert.equal(
      canReadAdminResource(user, ADMIN_PERMISSION_RESOURCES.MODEL),
      false
    )
  })

  test('allows root users even if the local matrix is stale', () => {
    const user = {
      id: 1,
      username: 'root',
      role: 100,
      permissions: {
        admin_permissions: {
          system_setting: {
            secret_view: false,
          },
        },
      },
    }

    assert.equal(
      hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.SYSTEM_SETTING,
        ADMIN_PERMISSION_ACTIONS.SECRET_VIEW
      ),
      true
    )
  })
})
