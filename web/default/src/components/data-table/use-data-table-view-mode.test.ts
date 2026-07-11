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
  DATA_TABLE_VIEW_MODES,
  isDataTableViewMode,
  readDataTableViewMode,
  writeDataTableViewMode,
} from './use-data-table-view-mode'

class MemoryStorage {
  private values = new Map<string, string>()

  getItem(key: string) {
    return this.values.get(key) ?? null
  }

  setItem(key: string, value: string) {
    this.values.set(key, value)
  }
}

class FailingStorage {
  getItem(): string {
    throw new Error('storage disabled')
  }

  setItem(): void {
    throw new Error('storage disabled')
  }
}

describe('DataTable 视图模式存储', () => {
  test('只接受 table/card 两种视图模式', () => {
    assert.equal(isDataTableViewMode(DATA_TABLE_VIEW_MODES.TABLE), true)
    assert.equal(isDataTableViewMode(DATA_TABLE_VIEW_MODES.CARD), true)
    assert.equal(isDataTableViewMode('grid'), false)
    assert.equal(isDataTableViewMode(null), false)
  })

  test('读取已持久化的合法视图模式', () => {
    const storage = new MemoryStorage()
    storage.setItem('models-view', DATA_TABLE_VIEW_MODES.CARD)

    assert.equal(
      readDataTableViewMode(
        'models-view',
        DATA_TABLE_VIEW_MODES.TABLE,
        storage
      ),
      DATA_TABLE_VIEW_MODES.CARD
    )
  })

  test('缺失或非法存储值会回退到默认视图', () => {
    const storage = new MemoryStorage()
    storage.setItem('models-view', 'grid')

    assert.equal(
      readDataTableViewMode(
        'models-view',
        DATA_TABLE_VIEW_MODES.TABLE,
        storage
      ),
      DATA_TABLE_VIEW_MODES.TABLE
    )
    assert.equal(
      readDataTableViewMode(undefined, DATA_TABLE_VIEW_MODES.CARD, storage),
      DATA_TABLE_VIEW_MODES.CARD
    )
  })

  test('存储不可用时读写都不会抛出异常', () => {
    const storage = new FailingStorage()

    assert.equal(
      readDataTableViewMode(
        'models-view',
        DATA_TABLE_VIEW_MODES.TABLE,
        storage
      ),
      DATA_TABLE_VIEW_MODES.TABLE
    )
    assert.doesNotThrow(() =>
      writeDataTableViewMode('models-view', DATA_TABLE_VIEW_MODES.CARD, storage)
    )
  })
})
