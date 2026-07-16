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

export const INTERFACE_LANGUAGE_OPTIONS = [
  { code: 'zh', label: '简体中文' },
  { code: 'en', label: 'English' },
  { code: 'fr', label: 'Français' },
  { code: 'ru', label: 'Русский' },
  { code: 'ja', label: '日本語' },
  { code: 'vi', label: 'Tiếng Việt' },
  { code: 'zh-TW', label: '繁體中文' },
] as const

export type InterfaceLanguageCode =
  (typeof INTERFACE_LANGUAGE_OPTIONS)[number]['code']

// 统一收敛浏览器检测值、历史缓存值和用户设置中的语言码，
// 避免 zh-TW / zhTW / zh_HK / zhCN 等多种写法把界面错误回退到英文。
export function normalizeInterfaceLanguage(
  value?: string | null
): InterfaceLanguageCode {
  if (!value) return 'en'

  const normalized = value.trim().replaceAll('_', '-')
  const lower = normalized.toLowerCase()

  if (
    lower === 'zh-tw' ||
    lower === 'zh-hk' ||
    lower === 'zh-mo' ||
    lower === 'zhtw' ||
    lower.startsWith('zh-hant')
  ) {
    return 'zh-TW'
  }

  if (
    lower === 'zh' ||
    lower === 'zh-cn' ||
    lower === 'zhcn' ||
    lower.startsWith('zh-hans')
  ) {
    return 'zh'
  }

  const exact = INTERFACE_LANGUAGE_OPTIONS.find(
    (language) => language.code === normalized
  )
  if (exact) {
    return exact.code
  }

  const prefix = INTERFACE_LANGUAGE_OPTIONS.find(
    (language) =>
      language.code !== 'zh' &&
      language.code !== 'zh-TW' &&
      lower.startsWith(language.code.toLowerCase())
  )
  if (prefix) {
    return prefix.code
  }

  return 'en'
}

// 前端初始化时优先复用 i18next 已缓存的语言，其次读取浏览器语言。
// 这样首次访问即可命中 zh-TW，且不会破坏当前已保存的简体/英文偏好。
export function getInitialInterfaceLanguage(): InterfaceLanguageCode {
  if (typeof window !== 'undefined') {
    const storedLanguage = window.localStorage.getItem('i18nextLng')
    if (storedLanguage) {
      return normalizeInterfaceLanguage(storedLanguage)
    }
  }

  if (typeof navigator !== 'undefined') {
    const candidates = navigator.languages?.length
      ? navigator.languages
      : [navigator.language]
    for (const candidate of candidates) {
      if (candidate) {
        return normalizeInterfaceLanguage(candidate)
      }
    }
  }

  return 'en'
}
