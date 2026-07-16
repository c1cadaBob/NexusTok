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
import { type TFunction } from 'i18next'
import {
  getSystemSettingsNavGroups,
  WORKSPACE_SYSTEM_SETTINGS_ID,
} from '../config/system-settings.config'
import type { NavGroup } from '../types'

export const WORKSPACE_IDS = {
  SYSTEM_SETTINGS: WORKSPACE_SYSTEM_SETTINGS_ID,
  DEFAULT: 'default',
} as const

export type WorkspaceId = (typeof WORKSPACE_IDS)[keyof typeof WORKSPACE_IDS]

/**
 * Workspace configuration type
 * Each workspace contains name, path matching rules, and corresponding navigation group configuration
 */
export type WorkspaceConfig = {
  /** Workspace identifier (for logic) */
  id: WorkspaceId
  /** Workspace name */
  name: string
  /** Path matching rule, supports string (contains match) or regular expression */
  pathPattern: string | RegExp
  /** Sidebar navigation group configuration for this workspace */
  getNavGroups?: (t: TFunction) => NavGroup[]
}

/**
 * 工作区注册表
 *
 * 该注册表继续承担“当前路径属于哪个工作区”的识别职责，供顶部
 * 工作区语义和兼容调用方使用。侧边栏 Drill-in 导航解析已迁移到
 * `sidebar-view-registry.ts`，避免把工作区识别与导航视图切换耦合。
 */
const workspaceRegistry: WorkspaceConfig[] = [
  // System Settings workspace
  {
    id: WORKSPACE_IDS.SYSTEM_SETTINGS,
    name: 'System Settings',
    pathPattern: /^\/system-settings/,
    getNavGroups: getSystemSettingsNavGroups,
  },
  // Default workspace (must be last)
  {
    id: WORKSPACE_IDS.DEFAULT,
    name: 'Default',
    pathPattern: /.*/,
    // getNavGroups is undefined, will be handled by consumers (e.g. useSidebarData)
  },
]

/**
 * 根据路径获取命中的工作区配置
 */
export function getWorkspaceByPath(pathname: string): WorkspaceConfig {
  const workspace = workspaceRegistry.find((ws) => {
    if (typeof ws.pathPattern === 'string') {
      return pathname.includes(ws.pathPattern)
    }
    return ws.pathPattern.test(pathname)
  })

  // 未命中时回退到末尾的默认工作区
  return workspace || workspaceRegistry[workspaceRegistry.length - 1]
}

/**
 * 兼容旧调用方：根据工作区返回对应导航分组
 *
 * 注意：侧边栏本身已改为使用 `sidebar-view-registry.ts` 驱动 Drill-in
 * 视图；这里保留该函数，避免其它仅依赖“工作区分组”语义的调用方
 * 立即断裂。
 */
export function getNavGroupsForPath(
  pathname: string,
  t: TFunction
): NavGroup[] | undefined {
  const workspace = getWorkspaceByPath(pathname)
  return workspace.getNavGroups?.(t)
}

/**
 * 判断当前路径是否属于指定工作区
 */
export function isInWorkspace(
  pathname: string,
  workspaceId: WorkspaceId
): boolean {
  return getWorkspaceByPath(pathname).id === workspaceId
}

/**
 * 返回全部已注册工作区配置
 */
export function getAllWorkspaces(): WorkspaceConfig[] {
  return workspaceRegistry
}
