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
import { SYSTEM_SETTINGS_VIEW } from '../config/system-settings.config'
import type { NavGroup, SidebarView } from '../types'

/**
 * 已注册的嵌套侧边栏视图
 *
 * 视图按数组顺序匹配，首个命中的 pathPattern 即为当前视图。
 * 当前仅先把 System Settings 收敛为 Drill-in 视图，避免一次性
 * 改动所有工作区侧栏结构。
 */
const SIDEBAR_VIEWS: readonly SidebarView[] = [SYSTEM_SETTINGS_VIEW]

/**
 * 根据当前路径解析命中的嵌套侧边栏视图
 */
export function resolveSidebarView(pathname: string): SidebarView | null {
  return SIDEBAR_VIEWS.find((view) => view.pathPattern.test(pathname)) ?? null
}

/**
 * 为只关心导航分组的调用方提供兼容辅助函数
 *
 * 例如命令面板只需要当前路径对应的分组列表，而不需要返回按钮等
 * 视图元信息时，可直接复用该函数。
 */
export function getNavGroupsForPath(
  pathname: string,
  t: TFunction
): NavGroup[] | null {
  const view = resolveSidebarView(pathname)
  return view ? view.getNavGroups(t) : null
}
