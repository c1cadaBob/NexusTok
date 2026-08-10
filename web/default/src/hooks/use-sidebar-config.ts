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
import { useMemo } from 'react'
import { useAuthStore } from '@/stores/auth-store'
import { useStatus } from '@/hooks/use-status'
import type { NavGroup, NavItem } from '@/components/layout/types'

type SidebarSectionConfig = {
  enabled: boolean
  [key: string]: boolean
}

type SidebarModulesAdminConfig = Record<string, SidebarSectionConfig>

// 用户层配置与管理员配置结构一致；null 表示旧用户或空配置不再额外收窄可见项。
type SidebarModulesUserConfig = SidebarModulesAdminConfig | null

/**
 * 默认侧边栏模块配置。
 */
const DEFAULT_SIDEBAR_MODULES: SidebarModulesAdminConfig = {
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

const mergeWithDefaultSidebarModules = (
  config: SidebarModulesAdminConfig
): SidebarModulesAdminConfig => {
  const merged: SidebarModulesAdminConfig = { ...config }

  Object.entries(DEFAULT_SIDEBAR_MODULES).forEach(
    ([sectionKey, defaultSection]) => {
      const existingSection = merged[sectionKey]
      if (!existingSection) {
        merged[sectionKey] = { ...defaultSection }
        return
      }

      merged[sectionKey] = { ...defaultSection, ...existingSection }
      Object.keys(defaultSection).forEach((moduleKey) => {
        if (merged[sectionKey][moduleKey] === undefined) {
          merged[sectionKey][moduleKey] = defaultSection[moduleKey]
        }
      })
    }
  )

  return merged
}

/**
 * URL 到侧栏模块配置键的映射。
 */
const URL_TO_CONFIG_MAP: Record<string, { section: string; module: string }> = {
  '/playground': { section: 'chat', module: 'playground' },
  '/dashboard': { section: 'console', module: 'detail' },
  '/dashboard/overview': { section: 'console', module: 'detail' },
  '/dashboard/models': { section: 'console', module: 'detail' },
  '/dashboard/users': { section: 'console', module: 'detail' },
  '/keys': { section: 'console', module: 'token' },
  '/usage-logs': { section: 'console', module: 'log' },
  '/usage-logs/common': { section: 'console', module: 'log' },
  '/usage-logs/drawing': { section: 'console', module: 'midjourney' },
  '/usage-logs/task': { section: 'console', module: 'task' },
  '/audit-logs': { section: 'console', module: 'log' },
  '/wallet': { section: 'personal', module: 'topup' },
  '/profile': { section: 'personal', module: 'personal' },
  '/channels': { section: 'admin', module: 'channel' },
  '/account-pool': { section: 'admin', module: 'account_pool' },
  '/pricing-settings': { section: 'admin', module: 'pricing' },
  '/models': { section: 'admin', module: 'models' },
  '/models/metadata': { section: 'admin', module: 'models' },
  '/models/deployments': { section: 'admin', module: 'models' },
  '/users': { section: 'admin', module: 'user' },
  '/redemption-codes': { section: 'admin', module: 'redemption' },
  '/subscriptions': { section: 'admin', module: 'subscription' },
  '/system-settings': { section: 'admin', module: 'setting' },
  '/system-settings/site': { section: 'admin', module: 'setting' },
  '/system-info': { section: 'admin', module: 'system_info' },
}

/**
 * 解析后端 SidebarModulesAdmin 配置。
 */
function parseSidebarConfig(
  value: string | null | undefined
): SidebarModulesAdminConfig {
  // 空字符串、null 或 undefined 都表示未配置，直接使用默认侧栏模块。
  if (!value || value.trim() === '') {
    return DEFAULT_SIDEBAR_MODULES
  }

  try {
    const parsed = JSON.parse(value) as SidebarModulesAdminConfig
    return mergeWithDefaultSidebarModules(parsed)
  } catch {
    // eslint-disable-next-line no-console
    console.error('Failed to parse sidebar modules configuration')
    return DEFAULT_SIDEBAR_MODULES
  }
}

/**
 * 解析用户级 sidebar_modules。
 *
 * 返回 null 表示用户配置为空、无效或不可用，调用方会把它理解为“不再收窄”。
 * 这样旧用户在没有 sidebar_modules 字段时，仍能看到管理员允许的完整入口。
 */
function parseUserSidebarConfig(
  value: string | null | undefined
): SidebarModulesUserConfig {
  if (!value || value.trim() === '') {
    return null
  }
  try {
    const parsed = JSON.parse(value) as SidebarModulesAdminConfig
    if (!parsed || typeof parsed !== 'object') return null
    return parsed
  } catch {
    return null
  }
}

/**
 * 判断模块是否可见。
 *
 * 可见性由两层配置取交集：
 * 1. 管理员配置来自 status.SidebarModulesAdmin，是全局权威配置；
 * 2. 用户配置来自 auth.user.sidebar_modules，只能在管理员允许的范围内继续隐藏。
 *
 * 当用户配置为 null 时表示旧用户或空配置，不额外隐藏任何管理员允许的入口。
 */
function isModuleEnabled(
  url: string,
  adminConfig: SidebarModulesAdminConfig,
  userConfig: SidebarModulesUserConfig
): boolean {
  const mapping = URL_TO_CONFIG_MAP[url]
  if (!mapping) {
    // 没有显式映射的新入口默认可见，避免新增功能因为旧配置缺字段而被意外隐藏。
    return true
  }

  const { section, module } = mapping
  const adminSection = adminConfig[section]
  const adminAllowed = Boolean(
    adminSection && adminSection.enabled && adminSection[module] === true
  )
  if (!adminAllowed) return false

  if (!userConfig) return true

  const userSection = userConfig[section]
  if (!userSection) return true
  if (userSection.enabled === false) return false
  return userSection[module] !== false
}

/**
 * 判断导航项是否应该展示。
 */
function isNavItemVisible(
  item: NavItem,
  adminConfig: SidebarModulesAdminConfig,
  userConfig: SidebarModulesUserConfig
): boolean {
  // 动态第三方预设入口同样遵循管理员配置和用户配置的双层收敛。
  if ('type' in item && item.type === 'chat-presets') {
    const adminChat = adminConfig.chat
    const adminAllowed = Boolean(adminChat?.enabled && adminChat.chat === true)
    if (!adminAllowed) return false
    if (!userConfig) return true
    const userChat = userConfig.chat
    if (!userChat) return true
    if (userChat.enabled === false) return false
    return userChat.chat !== false
  }

  // 单链接入口按自身 URL 或 configUrls 参与配置过滤。
  if ('url' in item && item.url) {
    const configUrls = item.configUrls ?? [item.url]
    return configUrls.some((url) =>
      isModuleEnabled(url as string, adminConfig, userConfig)
    )
  }

  // 折叠菜单只要仍有一个可见子入口，就保留父级入口。
  if ('items' in item && item.items) {
    return item.items.some((subItem) =>
      isModuleEnabled(subItem.url as string, adminConfig, userConfig)
    )
  }

  return true
}

/**
 * 过滤导航项。
 */
function filterNavItems(
  items: NavItem[],
  adminConfig: SidebarModulesAdminConfig,
  userConfig: SidebarModulesUserConfig
): NavItem[] {
  return items
    .map((item) => {
      // 折叠菜单的子入口需要同步过滤，避免父级展开后露出被配置隐藏的页面。
      if ('items' in item && item.items) {
        const filteredSubItems = item.items.filter((subItem) =>
          isModuleEnabled(subItem.url as string, adminConfig, userConfig)
        )

        return {
          ...item,
          items: filteredSubItems,
        }
      }
      return item
    })
    .filter((item) => isNavItemVisible(item, adminConfig, userConfig))
}

/**
 * 按管理员配置和用户配置过滤侧边栏分组。
 *
 * 两层配置取交集：
 * 1. 管理员配置来自 status.SidebarModulesAdmin，空值或非法值回退默认配置；
 * 2. 用户配置来自 auth.user.sidebar_modules，只能进一步隐藏管理员允许的模块。
 *
 * 当后端标记当前用户不能配置 sidebar_settings 时，忽略用户层配置，避免历史
 * sidebar_modules 值把 root 等没有恢复入口的账号锁在部分菜单之外。
 */
export function useSidebarConfig(navGroups: NavGroup[]): NavGroup[] {
  const { status } = useStatus()
  const { auth } = useAuthStore()

  const adminConfig = useMemo(
    () =>
      parseSidebarConfig(
        status?.SidebarModulesAdmin as string | null | undefined
      ),
    [status?.SidebarModulesAdmin]
  )

  const userConfig = useMemo(() => {
    // root 等账号没有用户级侧栏配置入口时，旧的本地配置不应继续影响可见菜单。
    if (auth?.user?.permissions?.sidebar_settings === false) {
      return null
    }
    return parseUserSidebarConfig(auth?.user?.sidebar_modules)
  }, [auth?.user?.permissions?.sidebar_settings, auth?.user?.sidebar_modules])

  const filteredNavGroups = useMemo(
    () =>
      navGroups
        .map((group) => ({
          ...group,
          items: filterNavItems(group.items, adminConfig, userConfig),
        }))
        .filter((group) => group.items.length > 0),
    [navGroups, adminConfig, userConfig]
  )

  return filteredNavGroups
}
