/*
Copyright (C) 2023-2026 c1cada

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License
as published by the Free Software Foundation, either version 3 of the
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
  canManuallyMutateChannelAccounts,
  isUpstreamAccountSyncChannel,
} from './channel-utils'

describe('账号池手动入口可见性', () => {
  test('上游同步渠道不允许手动新增、导入或删除账号', () => {
    const channel = {
      settings:
        '{"upstream_account_sync":{"platform":"new-api","base_url":"https://upstream.example","synced_at":1}}',
    }

    assert.equal(isUpstreamAccountSyncChannel(channel), true)
    assert.equal(canManuallyMutateChannelAccounts(channel), false)
  })

  test('普通账号池渠道仍允许手动维护账号', () => {
    const channel = {
      settings: '{"allow_service_tier":false}',
    }

    assert.equal(isUpstreamAccountSyncChannel(channel), false)
    assert.equal(canManuallyMutateChannelAccounts(channel), true)
  })
})
