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
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { parseHeaderNavModulesFromStatus } from '@/lib/nav-modules'
import { useStatus } from '@/hooks/use-status'

export type TopNavLink = {
  title: string
  href: string
  disabled?: boolean
  external?: boolean
}

/**
 * 基于后端 `/api/status` 返回的 HeaderNavModules 生成顶部导航。
 * 后端配置通常是字符串化 JSON，也可能来自历史版本的对象、数字或字符串布尔值。
 */
export function useTopNavLinks(): TopNavLink[] {
  const { t } = useTranslation()
  const { status } = useStatus()
  const { auth } = useAuthStore()

  // 解析 HeaderNavModules，确保顶栏和页面守卫共享同一套兼容语义。
  const modules = useMemo(() => {
    return parseHeaderNavModulesFromStatus(
      status as Record<string, unknown> | null
    )
  }, [status])

  // 文档链接可能是外部地址，也可能回退到内置文档页。
  const docsLink: string | undefined = status?.docs_link as string | undefined

  const isAuthed = !!auth?.user

  const links: TopNavLink[] = []

  // 首页入口。
  if (modules?.home !== false) {
    links.push({ title: t('Home'), href: '/' })
  }

  // 控制台入口统一指向新版 `/dashboard`。
  if (modules?.console !== false) {
    links.push({ title: t('Console'), href: '/dashboard' })
  }

  // 模型广场支持 requireAuth：未登录时保留入口但置为禁用态。
  const pricing = modules?.pricing
  if (pricing && typeof pricing === 'object' && pricing.enabled) {
    const disabled = pricing.requireAuth && !isAuthed
    links.push({ title: t('Model Square'), href: '/pricing', disabled })
  }

  // 排行榜同样支持 requireAuth，保持与模型广场一致的交互规则。
  const rankings = modules?.rankings
  if (rankings && typeof rankings === 'object' && rankings.enabled) {
    const disabled = rankings.requireAuth && !isAuthed
    links.push({ title: t('Rankings'), href: '/rankings', disabled })
  }

  // 文档入口支持外链配置。
  if (modules?.docs !== false) {
    if (docsLink) {
      links.push({ title: t('Docs'), href: docsLink, external: true })
    } else {
      links.push({ title: t('Docs'), href: '/docs' })
    }
  }

  // 关于页入口。
  if (modules?.about !== false) {
    links.push({ title: t('About'), href: '/about' })
  }

  return links
}
