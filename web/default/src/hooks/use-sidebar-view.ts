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
import { useLocation } from '@tanstack/react-router'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ADMIN_PERMISSION_RESOURCES,
  type AdminPermissionResource,
  canReadAdminResource,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'
import { resolveSidebarView } from '@/components/layout/lib/sidebar-view-registry'
import type { NavGroup, ResolvedSidebarView } from '@/components/layout/types'
import { useSidebarConfig } from './use-sidebar-config'
import { useSidebarData } from './use-sidebar-data'

const ROOT_VIEW_KEY = '__root'

/**
 * 解析当前路径对应的侧边栏视图
 *
 * - 普通页面返回根导航，并继续复用 NexusTok 现有的权限与
 *   `sidebar_modules` 双层过滤逻辑。
 * - `/system-settings/*` 等已注册工作区返回嵌套 Drill-in 视图，
 *   以 URL 驱动侧边栏上下文切换。
 */
export function useSidebarView(): ResolvedSidebarView {
  const { t } = useTranslation()
  const pathname = useLocation({ select: (location) => location.pathname })
  const user = useAuthStore((state) => state.auth.user)
  const sidebarData = useSidebarData()
  const configFilteredRoot = useSidebarConfig(sidebarData.navGroups)

  const rootNavGroups = useMemo<NavGroup[]>(() => {
    const adminResources: AdminPermissionResource[] = [
      ADMIN_PERMISSION_RESOURCES.CHANNEL,
      ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL,
      ADMIN_PERMISSION_RESOURCES.USER,
      ADMIN_PERMISSION_RESOURCES.MODEL,
      ADMIN_PERMISSION_RESOURCES.SUBSCRIPTION,
      ADMIN_PERMISSION_RESOURCES.REDEMPTION,
      ADMIN_PERMISSION_RESOURCES.SYSTEM_SETTING,
    ]
    const canReadAnyAdminResource = adminResources.some((resource) =>
      canReadAdminResource(user, resource)
    )

    return configFilteredRoot.filter((group) => {
      if (group.id === 'admin') {
        return canReadAnyAdminResource
      }
      return true
    })
  }, [configFilteredRoot, user])

  const view = resolveSidebarView(pathname)

  if (view) {
    return {
      key: view.id,
      view,
      navGroups: view.getNavGroups(t),
    }
  }

  return {
    key: ROOT_VIEW_KEY,
    view: null,
    navGroups: rootNavGroups,
  }
}
