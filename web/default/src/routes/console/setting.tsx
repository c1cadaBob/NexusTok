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
import z from 'zod'
import { createFileRoute, redirect } from '@tanstack/react-router'

const searchSchema = z.record(z.string(), z.unknown()).catch({})

function omitLegacyTab(
  search: Record<string, unknown>
): Record<string, unknown> | undefined {
  const nextSearch = { ...search }
  delete nextSearch.tab
  return Object.keys(nextSearch).length > 0 ? nextSearch : undefined
}

export const Route = createFileRoute('/console/setting')({
  validateSearch: searchSchema,
  beforeLoad: ({ search }) => {
    const tab = typeof search.tab === 'string' ? search.tab : ''
    const nextSearch = omitLegacyTab(search)

    // classic 设置页曾用单一 tab 参数承载多个设置分区。
    // 这里只映射已经确认语义的一组旧 tab，未知值保守回到新的系统设置入口。
    switch (tab) {
      case 'operation':
        throw redirect({
          to: '/system-settings/operations/$section',
          params: { section: 'behavior' },
          search: nextSearch,
          replace: true,
        })
      case 'dashboard':
        throw redirect({
          to: '/system-settings/content/$section',
          params: { section: 'dashboard' },
          search: nextSearch,
          replace: true,
        })
      case 'chats':
        throw redirect({
          to: '/system-settings/content/$section',
          params: { section: 'chat' },
          search: nextSearch,
          replace: true,
        })
      case 'drawing':
        throw redirect({
          to: '/system-settings/content/$section',
          params: { section: 'drawing' },
          search: nextSearch,
          replace: true,
        })
      case 'payment':
        throw redirect({
          to: '/system-settings/billing/$section',
          params: { section: 'payment' },
          search: nextSearch,
          replace: true,
        })
      case 'ratio':
        throw redirect({
          to: '/pricing-settings',
          search: nextSearch,
          replace: true,
        })
      case 'ratelimit':
        throw redirect({
          to: '/system-settings/security/$section',
          params: { section: 'rate-limit' },
          search: nextSearch,
          replace: true,
        })
      case 'models':
        throw redirect({
          to: '/system-settings/models/$section',
          params: { section: 'global' },
          search: nextSearch,
          replace: true,
        })
      case 'model-deployment':
        throw redirect({
          to: '/system-settings/models/$section',
          params: { section: 'model-deployment' },
          search: nextSearch,
          replace: true,
        })
      case 'performance':
        throw redirect({
          to: '/system-settings/operations/$section',
          params: { section: 'performance' },
          search: nextSearch,
          replace: true,
        })
      default:
        throw redirect({
          to: '/system-settings',
          search: nextSearch,
          replace: true,
        })
    }
  },
})
