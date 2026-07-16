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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { type TFunction } from 'i18next'
import { getNavGroupsForPath, resolveSidebarView } from './sidebar-view-registry'

const identityT = ((key: string) => key) as unknown as TFunction

describe('sidebar view registry', () => {
  test('system settings 路由命中 drill-in 视图', () => {
    const view = resolveSidebarView('/system-settings/models/global')

    assert.ok(view)
    assert.equal(view?.id, 'system-settings')
    assert.equal(view?.parent.label, 'Back to Dashboard')
  })

  test('非 system settings 路由回退到根导航', () => {
    assert.equal(resolveSidebarView('/dashboard/overview'), null)
    assert.equal(getNavGroupsForPath('/dashboard/overview', identityT), null)
  })

  test('命中的视图可生成系统设置导航分组', () => {
    const groups = getNavGroupsForPath(
      '/system-settings/security/general',
      identityT
    )

    assert.ok(groups)
    assert.equal(groups?.[0]?.title, 'System Administration')
    assert.ok(groups?.[0]?.items.some((item) => item.title === 'Security & Limits'))
  })
})
