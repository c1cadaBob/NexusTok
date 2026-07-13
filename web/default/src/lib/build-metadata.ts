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

export const BUILD_CHANNEL_TAG = 'nexustok-default'
export const BUILD_REVISION_STORAGE_KEY = 'nexustok:default:app-rev'

const BUILD_REVISION_PREFIX = 'rv'

export interface BuildDescriptor {
  readonly rev: string
  readonly ch: string
  readonly at: number
}

declare global {
  interface Window {
    __NEXUSTOK_BUILD__?: BuildDescriptor
  }
}

function readEnvRevision(): string | undefined {
  try {
    const env = (
      import.meta as unknown as { env?: Record<string, string | undefined> }
    ).env
    const raw =
      env?.VITE_REACT_APP_VERSION ??
      env?.VITE_APP_VERSION ??
      env?.APP_VERSION ??
      env?.VERSION
    if (typeof raw === 'string' && raw.trim().length > 0) return raw.trim()
  } catch {
    // 测试环境或极端内嵌环境可能没有 import.meta.env。
  }
  return undefined
}

export function computeBuildRevision(envRevision = readEnvRevision()): string {
  const head = envRevision && envRevision.length > 0 ? envRevision : '0000'
  return `${BUILD_REVISION_PREFIX}.${head}.${BUILD_CHANNEL_TAG}`
}

let installed = false

export function resetBuildMetadataForTests(): void {
  installed = false
}

export function getBuildRevision(): string {
  return computeBuildRevision()
}

/**
 * 将当前前端构建指纹写入浏览器可观测位置。
 *
 * 3003 热更新验证、排障脚本和用户支持场景可以从 `window.__NEXUSTOK_BUILD__`、
 * `<html data-build-rev>`、`<meta name="build-id">`、CSS 变量或 localStorage
 * 中确认当前浏览器加载的是哪一个前端包。
 */
export function installBuildMetadata(): void {
  if (installed) return
  if (typeof window === 'undefined' || typeof document === 'undefined') return
  installed = true

  const rev = getBuildRevision()
  const descriptor: BuildDescriptor = Object.freeze({
    rev,
    ch: BUILD_CHANNEL_TAG,
    at: Date.now(),
  })

  try {
    Object.defineProperty(window, '__NEXUSTOK_BUILD__', {
      value: descriptor,
      writable: false,
      configurable: false,
      enumerable: false,
    })
  } catch {
    // 热更新或测试环境可能已经锁定该属性；其它元数据写入仍继续尝试。
  }

  try {
    const html = document.documentElement
    html.setAttribute('data-build-rev', rev)
    html.setAttribute('data-app-channel', BUILD_CHANNEL_TAG)
  } catch {
    // 非标准嵌入环境可能没有完整 documentElement。
  }

  try {
    document.documentElement.style.setProperty('--app-build-rev', `'${rev}'`)
  } catch {
    // 受限 CSSOM 环境下忽略，避免影响应用启动。
  }

  try {
    let meta = document.querySelector<HTMLMetaElement>('meta[name="build-id"]')
    if (!meta) {
      meta = document.createElement('meta')
      meta.setAttribute('name', 'build-id')
      document.head.appendChild(meta)
    }
    meta.setAttribute('content', rev)
  } catch {
    // head 不可写时继续保持其它观测层。
  }

  try {
    window.localStorage.setItem(BUILD_REVISION_STORAGE_KEY, rev)
  } catch {
    // localStorage 不可用时不影响页面渲染。
  }

  try {
    // eslint-disable-next-line no-console
    console.debug('[nexustok-build] %s', rev)
  } catch {
    // console 被替换或禁用时忽略。
  }
}
