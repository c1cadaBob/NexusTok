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
import {
  HEADER_NAV_DEFAULT,
  parseHeaderNavModules as parseHeaderNavModulesConfig,
  type HeaderNavModules as HeaderNavModulesConfig,
} from '@/lib/nav-modules'

export { HEADER_NAV_DEFAULT }
export type {
  HeaderNavModules as HeaderNavModulesConfig,
  ModuleAccess as HeaderNavAccessConfig,
} from '@/lib/nav-modules'

export type SidebarSectionConfig = {
  enabled: boolean
  [key: string]: boolean
}

export type SidebarModulesAdminConfig = Record<string, SidebarSectionConfig>

export const SIDEBAR_MODULES_DEFAULT: SidebarModulesAdminConfig = {
  chat: {
    enabled: true,
    playground: true,
    chat: true,
  },
  console: {
    enabled: true,
    detail: true,
    token: true,
    log: true,
    midjourney: true,
    task: true,
  },
  personal: {
    enabled: true,
    topup: true,
    personal: true,
  },
  admin: {
    enabled: true,
    channel: true,
    account_pool: true,
    models: true,
    pricing: true,
    user: true,
    subscription: true,
    redemption: true,
    setting: true,
    system_info: true,
  },
}

const toBoolean = (value: unknown, fallback: boolean): boolean => {
  if (typeof value === 'boolean') return value
  if (typeof value === 'number') return value === 1
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase()
    if (normalized === 'true' || normalized === '1') return true
    if (normalized === 'false' || normalized === '0') return false
  }
  return fallback
}

const cloneSidebarDefault = (): SidebarModulesAdminConfig =>
  Object.entries(SIDEBAR_MODULES_DEFAULT).reduce<SidebarModulesAdminConfig>(
    (acc, [section, config]) => {
      acc[section] = { ...config }
      return acc
    },
    {}
  )

const orderSidebarSection = (
  section: SidebarSectionConfig,
  defaultSection: SidebarSectionConfig
): SidebarSectionConfig => {
  const ordered: SidebarSectionConfig = {
    enabled: section.enabled,
  }

  // 已知模块始终按平台默认信息架构展示；未知自定义模块继续保留在末尾，
  // 避免旧配置或插件式扩展因为排序归一化而丢失。
  Object.keys(defaultSection).forEach((moduleKey) => {
    if (moduleKey === 'enabled') return
    if (moduleKey in section) {
      ordered[moduleKey] = section[moduleKey]
    }
  })

  Object.entries(section).forEach(([moduleKey, moduleValue]) => {
    if (moduleKey === 'enabled' || moduleKey in ordered) return
    ordered[moduleKey] = moduleValue
  })

  return ordered
}

const orderSidebarModulesAdmin = (
  config: SidebarModulesAdminConfig
): SidebarModulesAdminConfig => {
  const ordered: SidebarModulesAdminConfig = {}

  Object.entries(SIDEBAR_MODULES_DEFAULT).forEach(
    ([sectionKey, defaultSection]) => {
      const section = config[sectionKey]
      if (!section) return
      ordered[sectionKey] = orderSidebarSection(section, defaultSection)
    }
  )

  Object.entries(config).forEach(([sectionKey, section]) => {
    if (sectionKey in ordered) return
    ordered[sectionKey] = { ...section }
  })

  return ordered
}

export function parseHeaderNavModules(
  value: string | null | undefined
): HeaderNavModulesConfig {
  return parseHeaderNavModulesConfig(value)
}

export function serializeHeaderNavModules(
  config: HeaderNavModulesConfig
): string {
  return JSON.stringify(config)
}

export function parseSidebarModulesAdmin(
  value: string | null | undefined
): SidebarModulesAdminConfig {
  const defaults = cloneSidebarDefault()
  // 空字符串、null 或 undefined 都表示未配置，直接使用默认侧栏模块。
  if (!value || value.trim() === '') return defaults

  try {
    const parsed = JSON.parse(value) as Record<string, unknown>
    const result: SidebarModulesAdminConfig = {}

    Object.entries(parsed).forEach(([sectionKey, raw]) => {
      if (!raw || typeof raw !== 'object') return

      const defaultSection = defaults[sectionKey] ?? { enabled: true }
      const sectionConfig: SidebarSectionConfig = {
        enabled: toBoolean(
          (raw as Record<string, unknown>).enabled,
          defaultSection.enabled ?? true
        ),
      }

      Object.entries(raw as Record<string, unknown>).forEach(
        ([moduleKey, moduleValue]) => {
          if (moduleKey === 'enabled') return
          sectionConfig[moduleKey] = toBoolean(
            moduleValue,
            defaultSection[moduleKey] ?? true
          )
        }
      )

      result[sectionKey] = sectionConfig
    })

    // 合并默认值，确保新增的侧栏分区在旧配置中也能出现。
    Object.entries(defaults).forEach(([sectionKey, config]) => {
      if (!result[sectionKey]) {
        result[sectionKey] = { ...config }
        return
      }

      Object.entries(config).forEach(([moduleKey, moduleValue]) => {
        if (!(moduleKey in result[sectionKey])) {
          result[sectionKey][moduleKey] = moduleValue
        }
      })
    })

    return orderSidebarModulesAdmin(result)
  } catch {
    return defaults
  }
}

export function serializeSidebarModulesAdmin(
  config: SidebarModulesAdminConfig
): string {
  return JSON.stringify(orderSidebarModulesAdmin(config))
}
