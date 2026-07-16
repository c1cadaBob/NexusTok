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
import { type LinkProps } from '@tanstack/react-router'
import { type TFunction } from 'i18next'

/**
 * 工作区定义
 * 用于顶部工作区切换器展示不同工作区
 */
export type Workspace = {
  id: string
  name: string
  logo: React.ElementType
  plan: string
}

/**
 * 基础导航项定义
 */
type BaseNavItem = {
  title: string
  badge?: string
  icon?: React.ElementType
  activeUrls?: (LinkProps['to'] | (string & {}))[]
  configUrls?: (LinkProps['to'] | (string & {}))[]
}

/**
 * 单链接导航项
 */
export type NavLink = BaseNavItem & {
  url: LinkProps['to'] | (string & {})
  items?: never
  type?: never
}

/**
 * 可折叠导航项，包含子链接
 */
export type NavCollapsible = BaseNavItem & {
  items: (BaseNavItem & { url: LinkProps['to'] | (string & {}) })[]
  url?: never
  type?: never
}

/**
 * 动态第三方预设导航项，列表内容来自后端 Chats 配置
 */
export type NavChatPresets = BaseNavItem & {
  type: 'chat-presets'
  url?: never
  items?: never
}

/**
 * 侧边栏导航项联合类型
 */
export type NavItem = NavCollapsible | NavLink | NavChatPresets

/**
 * 侧边栏导航分组
 */
export type NavGroup = {
  id?: string
  title: string
  items: NavItem[]
}

/**
 * 根侧边栏数据
 */
export type SidebarData = {
  workspaces: Workspace[]
  navGroups: NavGroup[]
}

/**
 * 顶部导航链接
 */
export type TopNavLink = {
  title: string
  href: string
  isActive?: boolean
  disabled?: boolean
  external?: boolean
}

/**
 * 嵌套侧边栏视图的返回入口定义
 */
export type SidebarViewParent = {
  /** 返回按钮跳转地址 */
  to: LinkProps['to'] | (string & {})
  /** 返回按钮文案，使用英文源文案交给 i18n 翻译 */
  label: string
}

/**
 * 侧边栏 Drill-in 视图定义
 *
 * 当用户进入某个工作区时，侧边栏不再继续叠加根导航，
 * 而是切换成该工作区自己的上下文导航视图。
 */
export type SidebarView = {
  /** 稳定视图标识，同时作为过渡动画 key */
  id: string
  /** 命中当前视图的路径规则 */
  pathPattern: RegExp
  /** 返回根导航的入口 */
  parent: SidebarViewParent
  /** 根据当前语言生成该视图的导航分组 */
  getNavGroups: (t: TFunction) => NavGroup[]
}

/**
 * 当前路径解析后的侧边栏视图结果
 */
export type ResolvedSidebarView = {
  /** 视图切换动画使用的稳定 key */
  key: string
  /** `null` 表示根导航，非空表示嵌套工作区视图 */
  view: SidebarView | null
  /** 当前应渲染的导航分组 */
  navGroups: NavGroup[]
}
