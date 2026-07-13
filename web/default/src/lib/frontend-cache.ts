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

export const FRONTEND_CACHE_VERSION = 'default-v1'
export const FRONTEND_CACHE_VERSION_KEY = 'nexustok:default:cache-version'

const PRESERVED_LOCAL_STORAGE_KEYS = new Set([
  FRONTEND_CACHE_VERSION_KEY,
  'nexustok:default:app-rev',
  'user',
  'uid',
  'aff',
  'oauth:binding:result',
  'status',
  'i18nextLng',
  'system-config-storage',
  'notification-storage',
  'table_compact_modes',
  'models-table-view-mode',
  'channels-table-view-mode',
  'enable-tag-mode',
  'channels-id-sort',
  'setup_status_checked',
  'data_export_default_time',
  'dashboard_models_chart_preferences',
  'playground_config',
  'playground_messages',
  'playground_parameter_enabled',
  'model-ratio-column-visibility',
  'home_page_content',
])

const PRESERVED_LOCAL_STORAGE_PREFIXES = ['system-settings-']

export type FrontendCacheStorage = Pick<
  Storage,
  'getItem' | 'setItem' | 'removeItem' | 'key' | 'length'
>

function shouldPreserveLocalStorageKey(key: string): boolean {
  return (
    PRESERVED_LOCAL_STORAGE_KEYS.has(key) ||
    PRESERVED_LOCAL_STORAGE_PREFIXES.some((prefix) => key.startsWith(prefix))
  )
}

/**
 * 根据前端缓存版本清理旧 UI 缓存。
 *
 * 该逻辑刻意保留登录态、语言、系统状态、通知已读、表格视图、渠道模式和
 * Playground 草稿等用户工作状态；版本切换时主要清理历史品牌缓存、废弃展示
 * 缓存和未知临时 UI 状态，避免热更新后旧 localStorage 继续污染当前包。
 */
export function syncFrontendCacheVersion(
  storage: FrontendCacheStorage,
  version = FRONTEND_CACHE_VERSION
): string[] {
  const currentVersion = storage.getItem(FRONTEND_CACHE_VERSION_KEY)
  if (currentVersion === version) return []

  const keysToRemove: string[] = []
  for (let index = 0; index < storage.length; index += 1) {
    const key = storage.key(index)
    if (key && !shouldPreserveLocalStorageKey(key)) {
      keysToRemove.push(key)
    }
  }

  keysToRemove.forEach((key) => storage.removeItem(key))
  storage.setItem(FRONTEND_CACHE_VERSION_KEY, version)
  return keysToRemove
}

export function initializeFrontendCache(): void {
  if (typeof window === 'undefined') return

  try {
    syncFrontendCacheVersion(window.localStorage)
  } catch {
    // localStorage 在隐私模式、受限 iframe 或浏览器策略下可能不可用；应用启动不应被缓存清理阻断。
  }
}
