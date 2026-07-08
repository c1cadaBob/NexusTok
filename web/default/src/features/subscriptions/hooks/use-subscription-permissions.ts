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
import { useAuthStore } from '@/stores/auth-store'
import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasAdminPermission,
} from '@/lib/admin-permissions'

export type SubscriptionPermissions = {
  canRead: boolean
  canOperate: boolean
  canWrite: boolean
  canSensitiveWrite: boolean
}

// 订阅管理既有独立页面，也会从用户行操作进入用户订阅弹窗。
// 统一 hook 可以避免跨入口时把订阅创建、失效或删除错误归到 user 权限域。
export function useSubscriptionPermissions(): SubscriptionPermissions {
  const user = useAuthStore((state) => state.auth.user)

  return useMemo(
    () => ({
      canRead: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.SUBSCRIPTION,
        ADMIN_PERMISSION_ACTIONS.READ
      ),
      canOperate: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.SUBSCRIPTION,
        ADMIN_PERMISSION_ACTIONS.OPERATE
      ),
      canWrite: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.SUBSCRIPTION,
        ADMIN_PERMISSION_ACTIONS.WRITE
      ),
      canSensitiveWrite: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.SUBSCRIPTION,
        ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
      ),
    }),
    [user]
  )
}
