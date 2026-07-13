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
import { afterEach, describe, test } from 'node:test'
import {
  BUILD_CHANNEL_TAG,
  BUILD_REVISION_STORAGE_KEY,
  computeBuildRevision,
  installBuildMetadata,
  resetBuildMetadataForTests,
} from './build-metadata'

class MemoryStorage {
  private values = new Map<string, string>()

  getItem(key: string) {
    return this.values.get(key) ?? null
  }

  setItem(key: string, value: string) {
    this.values.set(key, value)
  }
}

class FakeElement {
  readonly attributes = new Map<string, string>()
  readonly children: FakeElement[] = []
  readonly styleValues = new Map<string, string>()
  readonly style = {
    setProperty: (name: string, value: string) => {
      this.styleValues.set(name, value)
    },
  }

  appendChild(child: FakeElement) {
    this.children.push(child)
    return child
  }

  getAttribute(name: string) {
    return this.attributes.get(name) ?? null
  }

  setAttribute(name: string, value: string) {
    this.attributes.set(name, value)
  }
}

class FakeDocument {
  readonly documentElement = new FakeElement()
  readonly head = new FakeElement()
  private buildMeta: FakeElement | null = null

  createElement() {
    const element = new FakeElement()
    this.buildMeta = element
    return element
  }

  querySelector(selector: string) {
    return selector === 'meta[name="build-id"]' ? this.buildMeta : null
  }
}

afterEach(() => {
  resetBuildMetadataForTests()
  Reflect.deleteProperty(globalThis, 'window')
  Reflect.deleteProperty(globalThis, 'document')
})

describe('前端构建元数据', () => {
  test('按 NexusTok 渠道生成构建版本号', () => {
    assert.equal(
      computeBuildRevision('2026.07.13'),
      `rv.2026.07.13.${BUILD_CHANNEL_TAG}`
    )
  })

  test('安装构建元数据到 window、DOM、meta、CSS 变量和 localStorage', () => {
    const storage = new MemoryStorage()
    const fakeWindow = { localStorage: storage }
    const fakeDocument = new FakeDocument()

    Object.defineProperty(globalThis, 'window', {
      value: fakeWindow,
      configurable: true,
    })
    Object.defineProperty(globalThis, 'document', {
      value: fakeDocument,
      configurable: true,
    })

    const originalDebug = console.debug
    try {
      console.debug = () => undefined
      installBuildMetadata()
    } finally {
      console.debug = originalDebug
    }

    const descriptor = (
      fakeWindow as typeof fakeWindow & {
        __NEXUSTOK_BUILD__?: { rev: string; ch: string; at: number }
      }
    ).__NEXUSTOK_BUILD__

    assert.ok(descriptor)
    assert.equal(descriptor.ch, BUILD_CHANNEL_TAG)
    assert.equal(descriptor.rev, `rv.0000.${BUILD_CHANNEL_TAG}`)
    assert.equal(
      fakeDocument.documentElement.getAttribute('data-build-rev'),
      descriptor.rev
    )
    assert.equal(
      fakeDocument.documentElement.getAttribute('data-app-channel'),
      BUILD_CHANNEL_TAG
    )
    assert.equal(
      fakeDocument.documentElement.styleValues.get('--app-build-rev'),
      `'${descriptor.rev}'`
    )
    assert.equal(fakeDocument.head.children.length, 1)
    assert.equal(
      fakeDocument.head.children[0]?.getAttribute('content'),
      descriptor.rev
    )
    assert.equal(storage.getItem(BUILD_REVISION_STORAGE_KEY), descriptor.rev)
  })
})
