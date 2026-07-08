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
import { useLocation } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ADMIN_PERMISSION_RESOURCES,
  type AdminPermissionResource,
  canReadAdminResource,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'
import { useLayout } from '@/context/layout-provider'
import { useSidebarConfig } from '@/hooks/use-sidebar-config'
import { useSidebarData } from '@/hooks/use-sidebar-data'
import { Sidebar, SidebarContent, SidebarRail } from '@/components/ui/sidebar'
import { getNavGroupsForPath } from '../lib/workspace-registry'
import { NavGroup } from './nav-group'

/**
 * Application sidebar component
 * Fetches corresponding navigation menu from workspace registry based on current path
 * Dynamically filters navigation items based on backend SidebarModulesAdmin configuration
 *
 * Automatically matches workspace configuration for current path through workspace registry system
 * Adding new workspaces only requires registration in workspace-registry.ts
 */
export function AppSidebar() {
  const { t } = useTranslation()
  const { collapsible, variant } = useLayout()
  const { pathname } = useLocation()
  const user = useAuthStore((state) => state.auth.user)
  const sidebarData = useSidebarData()

  // Get navigation group configuration corresponding to current path from workspace registry
  const allNavGroups = getNavGroupsForPath(pathname, t) || sidebarData.navGroups

  // Filter sidebar navigation items based on backend configuration
  const configFilteredNavGroups = useSidebarConfig(allNavGroups)

  // Filter navigation groups based on user role
  // Non-Admin users cannot see Admin navigation group
  const currentNavGroups = useMemo(() => {
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
    return configFilteredNavGroups.filter((group) => {
      if (group.id === 'admin') {
        return canReadAnyAdminResource
      }
      return true
    })
  }, [configFilteredNavGroups, user])

  return (
    <Sidebar collapsible={collapsible} variant={variant}>
      <SidebarContent className='py-2'>
        {currentNavGroups.map((props) => {
          const key = props.id || props.title
          return <NavGroup key={key} {...props} />
        })}
      </SidebarContent>
      <SidebarRail />
    </Sidebar>
  )
}
