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
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { useLayout } from '@/context/layout-provider'
import { useSidebarView } from '@/hooks/use-sidebar-view'
import { Sidebar, SidebarContent, SidebarRail } from '@/components/ui/sidebar'
import { MOTION_TRANSITION, MOTION_VARIANTS } from '@/lib/motion'
import { NavGroup } from './nav-group'
import { SidebarViewHeader } from './sidebar-view-header'

/**
 * 应用侧边栏
 *
 * 侧边栏根据 URL 决定渲染根导航还是嵌套工作区视图：
 * - 根导航继续沿用 NexusTok 现有的权限矩阵和 sidebar_modules 过滤；
 * - 已注册的工作区（当前为 `/system-settings/*`）切换为 Drill-in 视图，
 *   并在头部提供返回主导航上下文的入口。
 */
export function AppSidebar() {
  const { collapsible, variant } = useLayout()
  const { key, view, navGroups } = useSidebarView()
  const shouldReduceMotion = useReducedMotion()

  return (
    <Sidebar collapsible={collapsible} variant={variant}>
      {view && <SidebarViewHeader view={view} />}

      <SidebarContent className='py-2'>
        <AnimatePresence mode='wait' initial={false}>
          <motion.div
            key={key}
            initial={
              shouldReduceMotion ? false : MOTION_VARIANTS.sidebarSlide.initial
            }
            animate={MOTION_VARIANTS.sidebarSlide.animate}
            exit={
              shouldReduceMotion ? undefined : MOTION_VARIANTS.sidebarSlide.exit
            }
            transition={MOTION_TRANSITION.fast}
            className='flex flex-col'
          >
            {navGroups.map((props) => (
              <NavGroup key={props.id || props.title} {...props} />
            ))}
          </motion.div>
        </AnimatePresence>
      </SidebarContent>
      <SidebarRail />
    </Sidebar>
  )
}
