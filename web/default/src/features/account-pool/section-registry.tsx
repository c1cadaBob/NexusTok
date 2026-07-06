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
import { createSectionRegistry } from '@/features/system-settings/utils/section-registry'

/**
 * 账号池管理页的主分区。
 *
 * 分区只按管理员的实际任务拆分，避免把单页 Tab 继续堆成过深的导航：
 * - overview：看整体健康和异常；
 * - credentials：把导入的账号凭证作为独立资源集中管理；
 * - groups：维护分组、调度策略以及组内账号；
 * - history：查询使用、状态和检测记录。
 */
const ACCOUNT_POOL_SECTIONS = [
  {
    id: 'overview',
    titleKey: 'Overview',
    descriptionKey: 'Review account pool health and recent exceptions.',
    build: () => null,
  },
  {
    id: 'credentials',
    titleKey: 'Account Credentials',
    descriptionKey:
      'Manage imported account credentials as reusable pool resources.',
    build: () => null,
  },
  {
    id: 'groups',
    titleKey: 'Groups',
    descriptionKey:
      'Configure pool groups, scheduling policies, and linked accounts.',
    build: () => null,
  },
  {
    id: 'history',
    titleKey: 'Logs & History',
    descriptionKey: 'Inspect usage records, state changes, and check tasks.',
    build: () => null,
  },
] as const

export type AccountPoolSectionId =
  (typeof ACCOUNT_POOL_SECTIONS)[number]['id']

const accountPoolRegistry = createSectionRegistry<
  AccountPoolSectionId,
  Record<string, never>,
  []
>({
  sections: ACCOUNT_POOL_SECTIONS,
  defaultSection: 'credentials',
  basePath: '/account-pool',
  urlStyle: 'path',
})

export const ACCOUNT_POOL_SECTION_IDS = accountPoolRegistry.sectionIds
export const ACCOUNT_POOL_DEFAULT_SECTION =
  accountPoolRegistry.defaultSection
export const getAccountPoolSectionNavItems =
  accountPoolRegistry.getSectionNavItems

export function isAccountPoolSectionId(
  section: string
): section is AccountPoolSectionId {
  return (ACCOUNT_POOL_SECTION_IDS as readonly string[]).includes(section)
}
