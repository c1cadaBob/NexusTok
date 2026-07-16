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
import { Link } from '@tanstack/react-router'
import { ChevronLeft } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from '@/components/ui/sidebar'
import type { SidebarView } from '../types'

type SidebarViewHeaderProps = {
  view: SidebarView
}

/**
 * 嵌套侧边栏视图头部
 *
 * 该头部只负责提供“返回主导航上下文”的入口，工作区语义则由
 * 下方的分组标题和导航项本身表达，避免重复堆叠标题信息。
 */
export function SidebarViewHeader(props: SidebarViewHeaderProps) {
  const { t } = useTranslation()
  const { setOpenMobile } = useSidebar()

  return (
    <SidebarHeader className='border-sidebar-border border-b px-2 py-2'>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton
            tooltip={t(props.view.parent.label)}
            className='text-muted-foreground hover:text-foreground gap-1.5 font-medium'
            render={
              <Link
                to={props.view.parent.to}
                onClick={() => setOpenMobile(false)}
              />
            }
          >
            <ChevronLeft />
            <span className='truncate'>{t(props.view.parent.label)}</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarHeader>
  )
}
