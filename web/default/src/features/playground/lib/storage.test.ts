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
import { afterEach, beforeEach, describe, test } from 'node:test'
import { MESSAGE_ROLES, MESSAGE_STATUS, STORAGE_KEYS } from '../constants'
import type { Message } from '../types'
import {
  clearPlaygroundData,
  loadConfig,
  loadMessages,
  loadParameterEnabled,
  saveConfig,
  saveMessages,
  saveParameterEnabled,
} from './storage'
import {
  MAX_LOADED_MESSAGE_CHARS,
  MAX_LOADED_MESSAGES_CHARS,
  MAX_STORED_MESSAGES,
  MAX_STORED_MESSAGES_BYTES,
  STORAGE_VERSION,
} from './storage-schema'

class MemoryStorage implements Storage {
  private readonly store = new Map<string, string>()

  get length(): number {
    return this.store.size
  }

  clear(): void {
    this.store.clear()
  }

  getItem(key: string): string | null {
    return this.store.get(key) ?? null
  }

  key(index: number): string | null {
    return Array.from(this.store.keys())[index] ?? null
  }

  removeItem(key: string): void {
    this.store.delete(key)
  }

  setItem(key: string, value: string): void {
    this.store.set(key, value)
  }
}

// eslint-disable-next-line no-console
const originalConsoleError = console.error

function installLocalStorage(): MemoryStorage {
  const storage = new MemoryStorage()

  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: storage,
  })

  return storage
}

function parseStoredEnvelope<T>(storage: Storage, key: string): {
  version: number
  data: T
} {
  const saved = storage.getItem(key)
  assert.ok(saved)

  return JSON.parse(saved) as { version: number; data: T }
}

function makeMessage(
  key: string,
  from: Message['from'],
  content: string,
  overrides: Partial<Message> = {}
): Message {
  return {
    key,
    from,
    versions: [{ id: `${key}-v1`, content }],
    ...overrides,
  }
}

describe('playground storage', () => {
  let storage: MemoryStorage

  beforeEach(() => {
    storage = installLocalStorage()
    // eslint-disable-next-line no-console
    console.error = () => {}
  })

  afterEach(() => {
    storage.clear()
    // eslint-disable-next-line no-console
    console.error = originalConsoleError
  })

  test('writes config as a versioned envelope and reads it back', () => {
    saveConfig({ model: 'gpt-4o-mini', stream: false, temperature: 0 })

    const stored = parseStoredEnvelope<ReturnType<typeof loadConfig>>(
      storage,
      STORAGE_KEYS.CONFIG
    )

    assert.equal(stored.version, STORAGE_VERSION)
    assert.deepEqual(stored.data, {
      model: 'gpt-4o-mini',
      temperature: 0,
      stream: false,
    })
    assert.deepEqual(loadConfig(), stored.data)
  })

  test('loads legacy config and falls back on invalid config', () => {
    storage.setItem(
      STORAGE_KEYS.CONFIG,
      JSON.stringify({ model: 'legacy-model', stream: true })
    )

    assert.deepEqual(loadConfig(), {
      model: 'legacy-model',
      stream: true,
    })

    storage.setItem(
      STORAGE_KEYS.CONFIG,
      JSON.stringify({ temperature: 'invalid' })
    )

    assert.deepEqual(loadConfig(), {})
  })

  test('writes parameter enabled state as a versioned envelope', () => {
    saveParameterEnabled({ seed: true, max_tokens: false })

    const stored = parseStoredEnvelope<ReturnType<typeof loadParameterEnabled>>(
      storage,
      STORAGE_KEYS.PARAMETER_ENABLED
    )

    assert.equal(stored.version, STORAGE_VERSION)
    assert.deepEqual(stored.data, {
      max_tokens: false,
      seed: true,
    })
    assert.deepEqual(loadParameterEnabled(), stored.data)
  })

  test('saves only the latest messages', () => {
    const messages = Array.from({ length: MAX_STORED_MESSAGES + 5 }, (_, index) =>
      makeMessage(`m-${index}`, MESSAGE_ROLES.USER, `message ${index}`)
    )

    saveMessages(messages)

    const stored = parseStoredEnvelope<Message[]>(
      storage,
      STORAGE_KEYS.MESSAGES
    )

    assert.equal(stored.version, STORAGE_VERSION)
    assert.equal(stored.data.length, MAX_STORED_MESSAGES)
    assert.equal(stored.data[0].key, 'm-5')
  })

  test('loads legacy messages and stabilizes pending assistants', () => {
    const messages: Message[] = [
      makeMessage('u-1', MESSAGE_ROLES.USER, 'prompt'),
      makeMessage('a-1', MESSAGE_ROLES.ASSISTANT, '<think>hidden</think>answer', {
        createdAt: 500,
        startedAt: 1000,
        status: MESSAGE_STATUS.STREAMING,
      }),
      makeMessage('a-2', MESSAGE_ROLES.ASSISTANT, '', {
        createdAt: 2000,
        startedAt: 2000,
        status: MESSAGE_STATUS.LOADING,
      }),
    ]
    storage.setItem(STORAGE_KEYS.MESSAGES, JSON.stringify(messages))

    const loaded = loadMessages()

    assert.ok(loaded)
    assert.equal(loaded[1].status, MESSAGE_STATUS.COMPLETE)
    assert.equal(loaded[1].versions[0].content, 'answer')
    assert.equal(loaded[1].reasoning?.content, 'hidden')
    assert.equal(loaded[1].completedAt, 1000)
    assert.equal(loaded[1].durationMs, 0)
    assert.equal(loaded[2].status, MESSAGE_STATUS.ERROR)

    const rewritten = parseStoredEnvelope<Message[]>(
      storage,
      STORAGE_KEYS.MESSAGES
    )
    assert.equal(rewritten.version, STORAGE_VERSION)
  })

  test('removes oversized raw message storage before parsing', () => {
    storage.setItem(
      STORAGE_KEYS.MESSAGES,
      'x'.repeat(MAX_STORED_MESSAGES_BYTES + 1)
    )

    assert.equal(loadMessages(), null)
    assert.equal(storage.getItem(STORAGE_KEYS.MESSAGES), null)
  })

  test('truncates overly long message content when loading', () => {
    storage.setItem(
      STORAGE_KEYS.MESSAGES,
      JSON.stringify([
        makeMessage(
          'long',
          MESSAGE_ROLES.ASSISTANT,
          'a'.repeat(MAX_LOADED_MESSAGE_CHARS + 50)
        ),
      ])
    )

    const loaded = loadMessages()

    assert.ok(loaded)
    assert.equal(loaded[0].versions[0].content.length, MAX_LOADED_MESSAGE_CHARS)
    assert.ok(loaded[0].versions[0].content.endsWith('\n\n[...]'))
  })

  test('keeps the newest messages when loaded content exceeds total limit', () => {
    const chunk = 'a'.repeat(MAX_LOADED_MESSAGES_CHARS / 4)
    const messages = Array.from({ length: 5 }, (_, index) =>
      makeMessage(`m-${index}`, MESSAGE_ROLES.USER, chunk)
    )
    storage.setItem(STORAGE_KEYS.MESSAGES, JSON.stringify(messages))

    const loaded = loadMessages()

    assert.ok(loaded)
    assert.deepEqual(
      loaded.map((message) => message.key),
      ['m-1', 'm-2', 'm-3', 'm-4']
    )
  })

  test('collapses repeated markdown section snapshots while loading', () => {
    const repeatedContent = [
      'intro '.repeat(350),
      '## 1. Snapshot\nold content\n',
      'middle '.repeat(200),
      '## 1. Snapshot\nmiddle content\n',
      'tail '.repeat(200),
      '## 1. Snapshot\nlatest content\n',
    ].join('\n')

    storage.setItem(
      STORAGE_KEYS.MESSAGES,
      JSON.stringify([
        makeMessage('snapshots', MESSAGE_ROLES.ASSISTANT, repeatedContent),
      ])
    )

    const loaded = loadMessages()

    assert.ok(loaded)
    assert.ok(loaded[0].versions[0].content.startsWith('## 1. Snapshot'))
    assert.ok(loaded[0].versions[0].content.includes('latest content'))
    assert.ok(!loaded[0].versions[0].content.includes('old content'))
  })

  test('clears all playground storage keys', () => {
    saveConfig({ model: 'gpt-4o' })
    saveParameterEnabled({ seed: true })
    saveMessages([makeMessage('m-1', MESSAGE_ROLES.USER, 'hello')])

    clearPlaygroundData()

    assert.equal(storage.getItem(STORAGE_KEYS.CONFIG), null)
    assert.equal(storage.getItem(STORAGE_KEYS.PARAMETER_ENABLED), null)
    assert.equal(storage.getItem(STORAGE_KEYS.MESSAGES), null)
  })
})
