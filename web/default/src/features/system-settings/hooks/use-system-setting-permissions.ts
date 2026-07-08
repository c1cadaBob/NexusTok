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

export type SystemSettingPermissions = {
  canRead: boolean
  canOperate: boolean
  canWrite: boolean
  canSensitiveWrite: boolean
  canViewSecret: boolean
}

// 系统设置页横跨全局配置、运行维护、OAuth 登录入口和性能工具。
// 集中封装权限矩阵可以让页面按钮与后端 system_setting 路由权限表保持一致，
// 也为后续逐步接入普通写和密钥查看动作预留单一落点。
export function useSystemSettingPermissions(): SystemSettingPermissions {
  const user = useAuthStore((state) => state.auth.user)

  return useMemo(
    () => ({
      canRead: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.SYSTEM_SETTING,
        ADMIN_PERMISSION_ACTIONS.READ
      ),
      canOperate: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.SYSTEM_SETTING,
        ADMIN_PERMISSION_ACTIONS.OPERATE
      ),
      canWrite: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.SYSTEM_SETTING,
        ADMIN_PERMISSION_ACTIONS.WRITE
      ),
      canSensitiveWrite: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.SYSTEM_SETTING,
        ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
      ),
      canViewSecret: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.SYSTEM_SETTING,
        ADMIN_PERMISSION_ACTIONS.SECRET_VIEW
      ),
    }),
    [user]
  )
}
