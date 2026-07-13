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
  FRONTEND_CACHE_VERSION,
  FRONTEND_CACHE_VERSION_KEY,
  syncFrontendCacheVersion,
  type FrontendCacheStorage,
} from './frontend-cache'

class MemoryStorage implements FrontendCacheStorage {
  private values = new Map<string, string>()

  get length() {
    return this.values.size
  }

  getItem(key: string) {
    return this.values.get(key) ?? null
  }

  key(index: number) {
    return Array.from(this.values.keys())[index] ?? null
  }

  removeItem(key: string) {
    this.values.delete(key)
  }

  setItem(key: string, value: string) {
    this.values.set(key, value)
  }
}

describe('前端缓存版本清理', () => {
  test('版本一致时不清理任何缓存', () => {
    const storage = new MemoryStorage()
    storage.setItem(FRONTEND_CACHE_VERSION_KEY, FRONTEND_CACHE_VERSION)
    storage.setItem('legacy-ui-cache', 'old')

    assert.deepEqual(syncFrontendCacheVersion(storage), [])
    assert.equal(storage.getItem('legacy-ui-cache'), 'old')
  })

  test('版本变化时清理未知 UI 缓存并保留关键状态', () => {
    const storage = new MemoryStorage()
    storage.setItem('user', '{"id":1}')
    storage.setItem('uid', '1')
    storage.setItem('aff', 'invite')
    storage.setItem('status', '{"system_name":"NexusTok"}')
    storage.setItem('i18nextLng', 'zh')
    storage.setItem('channels-table-view-mode', 'card')
    storage.setItem('system-settings-security-accordion', '["authz"]')
    storage.setItem('legacy_logo', '/old-logo.png')
    storage.setItem('footer_html', '<b>old</b>')

    const removed = syncFrontendCacheVersion(storage)

    assert.deepEqual(removed.sort(), ['footer_html', 'legacy_logo'])
    assert.equal(storage.getItem('user'), '{"id":1}')
    assert.equal(storage.getItem('uid'), '1')
    assert.equal(storage.getItem('aff'), 'invite')
    assert.equal(storage.getItem('status'), '{"system_name":"NexusTok"}')
    assert.equal(storage.getItem('i18nextLng'), 'zh')
    assert.equal(storage.getItem('channels-table-view-mode'), 'card')
    assert.equal(
      storage.getItem('system-settings-security-accordion'),
      '["authz"]'
    )
    assert.equal(storage.getItem('legacy_logo'), null)
    assert.equal(storage.getItem('footer_html'), null)
    assert.equal(storage.getItem(FRONTEND_CACHE_VERSION_KEY), 'default-v1')
  })
})
