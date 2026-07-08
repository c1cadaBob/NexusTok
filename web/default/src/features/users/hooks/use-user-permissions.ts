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

export type UserPermissions = {
  canRead: boolean
  canOperate: boolean
  canWrite: boolean
  canSensitiveWrite: boolean
}

// 用户管理页覆盖账号资料、生命周期、安全凭据、绑定和额度调整。
// 集中封装后，各按钮只表达业务动作，具体角色基线和用户 override 仍由权限矩阵决定。
export function useUserPermissions(): UserPermissions {
  const user = useAuthStore((state) => state.auth.user)

  return useMemo(
    () => ({
      canRead: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.USER,
        ADMIN_PERMISSION_ACTIONS.READ
      ),
      canOperate: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.USER,
        ADMIN_PERMISSION_ACTIONS.OPERATE
      ),
      canWrite: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.USER,
        ADMIN_PERMISSION_ACTIONS.WRITE
      ),
      canSensitiveWrite: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.USER,
        ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
      ),
    }),
    [user]
  )
}
