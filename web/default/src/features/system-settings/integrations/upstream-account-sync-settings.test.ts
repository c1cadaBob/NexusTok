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
  UPSTREAM_ACCOUNT_SYNC_UNITS,
  buildUpstreamAccountSyncFormDefaults,
  buildUpstreamAccountSyncPersistedDefaults,
  formatUpstreamAccountSyncDescription,
  normalizeUpstreamAccountSyncInterval,
  normalizeUpstreamAccountSyncUnit,
} from './upstream-account-sync-settings'

function t(key: string, options?: Record<string, unknown>) {
  return key
    .replace('{{interval}}', String(options?.interval ?? ''))
    .replace('{{unit}}', String(options?.unit ?? ''))
}

describe('上游账号自动同步设置', () => {
  test('支持所有周期单位并兼容大小写和空格', () => {
    for (const unit of UPSTREAM_ACCOUNT_SYNC_UNITS) {
      assert.equal(normalizeUpstreamAccountSyncUnit(unit), unit)
      assert.equal(
        normalizeUpstreamAccountSyncUnit(` ${unit.toUpperCase()} `),
        unit
      )
    }
  })

  test('非法单位和非法间隔回退到安全默认值', () => {
    assert.equal(normalizeUpstreamAccountSyncUnit('calendar-month'), 'hour')
    assert.equal(normalizeUpstreamAccountSyncInterval(0), 1)
    assert.equal(normalizeUpstreamAccountSyncInterval(-2), 1)
    assert.equal(normalizeUpstreamAccountSyncInterval(1.5), 1)
    assert.equal(normalizeUpstreamAccountSyncInterval(Number.NaN), 1)
    assert.equal(normalizeUpstreamAccountSyncInterval(6), 6)
  })

  test('表单默认值会回退，但持久化基线保留原始值以便保存时修正脏配置', () => {
    const raw = {
      enabled: true,
      interval: 0,
      unit: 'invalid',
    }

    assert.deepEqual(buildUpstreamAccountSyncFormDefaults(raw), {
      enabled: true,
      interval: 1,
      unit: 'hour',
    })
    assert.deepEqual(buildUpstreamAccountSyncPersistedDefaults(raw), raw)
  })

  test('根据开关状态生成周期说明', () => {
    assert.equal(
      formatUpstreamAccountSyncDescription(true, 6, 'hour', t),
      'Syncing every 6 Hours'
    )
    assert.equal(
      formatUpstreamAccountSyncDescription(false, 6, 'hour', t),
      'This setting is disabled; upstream account pools will not be synchronized automatically.'
    )
  })
})
