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

export type ChannelPermissions = {
  canRead: boolean
  canOperate: boolean
  canWrite: boolean
  canSensitiveWrite: boolean
  canViewSecret: boolean
  canReadAccountPool: boolean
  canReadChannelAccount: boolean
  canOperateChannelAccount: boolean
  canWriteChannelAccount: boolean
  canSensitiveWriteChannelAccount: boolean
}

// 渠道管理页同时消费 channel、channel_account 与 account_pool 权限。
// channel 控制渠道配置本身；channel_account 只覆盖渠道内账号列表、启停、普通调度字段和凭证维护；
// account_pool 仍用于全局账号池组选择等跨渠道资源。集中封装可以避免各组件散落权限常量。
export function useChannelPermissions(): ChannelPermissions {
  const user = useAuthStore((state) => state.auth.user)

  return useMemo(
    () => ({
      canRead: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL,
        ADMIN_PERMISSION_ACTIONS.READ
      ),
      canOperate: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL,
        ADMIN_PERMISSION_ACTIONS.OPERATE
      ),
      canWrite: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL,
        ADMIN_PERMISSION_ACTIONS.WRITE
      ),
      canSensitiveWrite: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL,
        ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
      ),
      canViewSecret: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL,
        ADMIN_PERMISSION_ACTIONS.SECRET_VIEW
      ),
      canReadAccountPool: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL,
        ADMIN_PERMISSION_ACTIONS.READ
      ),
      canReadChannelAccount: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL_ACCOUNT,
        ADMIN_PERMISSION_ACTIONS.READ
      ),
      canOperateChannelAccount: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL_ACCOUNT,
        ADMIN_PERMISSION_ACTIONS.OPERATE
      ),
      canWriteChannelAccount: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL_ACCOUNT,
        ADMIN_PERMISSION_ACTIONS.WRITE
      ),
      canSensitiveWriteChannelAccount: hasAdminPermission(
        user,
        ADMIN_PERMISSION_RESOURCES.CHANNEL_ACCOUNT,
        ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
      ),
    }),
    [user]
  )
}
