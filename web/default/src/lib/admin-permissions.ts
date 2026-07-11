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
import type { AuthUser } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'

export type AdminPermissionMatrix = Record<string, Record<string, boolean>>
export type AdminCapabilities = AdminPermissionMatrix

export const ADMIN_PERMISSION_RESOURCES = {
  CHANNEL: 'channel',
  ACCOUNT_POOL: 'account_pool',
  ACCOUNT_POOL_AUTH_FILE: 'account_pool_auth_file',
  USER: 'user',
  MODEL: 'model',
  SUBSCRIPTION: 'subscription',
  REDEMPTION: 'redemption',
  USAGE_LOG: 'usage_log',
  USAGE_DATA: 'usage_data',
  SYSTEM_SETTING: 'system_setting',
} as const

export const ADMIN_PERMISSION_ACTIONS = {
  READ: 'read',
  OPERATE: 'operate',
  WRITE: 'write',
  SENSITIVE_WRITE: 'sensitive_write',
  SECRET_VIEW: 'secret_view',
} as const

export const ADMIN_ROLE_KEY = 'admin'

export interface PermissionActionDefinition {
  action: string
  label_key: string
  description_key: string
}

export interface PermissionResourceDefinition {
  resource: string
  label_key: string
  actions: PermissionActionDefinition[]
}

export interface PermissionRoleDefinition {
  key: string
  name: string
  built_in: boolean
  superuser: boolean
  grants: AdminPermissionMatrix
}

export interface PermissionCatalog {
  resources: PermissionResourceDefinition[]
  roles: PermissionRoleDefinition[]
}

export const EMPTY_PERMISSION_CATALOG: PermissionCatalog = {
  resources: [],
  roles: [],
}

export type AdminPermissionResource =
  (typeof ADMIN_PERMISSION_RESOURCES)[keyof typeof ADMIN_PERMISSION_RESOURCES]

export type AdminPermissionAction =
  (typeof ADMIN_PERMISSION_ACTIONS)[keyof typeof ADMIN_PERMISSION_ACTIONS]

const ADMIN_DEFAULT_GRANTS: Record<
  AdminPermissionResource,
  Partial<Record<AdminPermissionAction, boolean>>
> = {
  [ADMIN_PERMISSION_RESOURCES.CHANNEL]: {
    [ADMIN_PERMISSION_ACTIONS.READ]: true,
    [ADMIN_PERMISSION_ACTIONS.OPERATE]: true,
    [ADMIN_PERMISSION_ACTIONS.WRITE]: true,
  },
  [ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL]: {
    [ADMIN_PERMISSION_ACTIONS.READ]: true,
    [ADMIN_PERMISSION_ACTIONS.OPERATE]: true,
    [ADMIN_PERMISSION_ACTIONS.WRITE]: true,
  },
  [ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL_AUTH_FILE]: {
    [ADMIN_PERMISSION_ACTIONS.READ]: true,
  },
  [ADMIN_PERMISSION_RESOURCES.USER]: {
    [ADMIN_PERMISSION_ACTIONS.READ]: true,
    [ADMIN_PERMISSION_ACTIONS.OPERATE]: true,
    [ADMIN_PERMISSION_ACTIONS.WRITE]: true,
  },
  [ADMIN_PERMISSION_RESOURCES.MODEL]: {
    [ADMIN_PERMISSION_ACTIONS.READ]: true,
    [ADMIN_PERMISSION_ACTIONS.OPERATE]: true,
    [ADMIN_PERMISSION_ACTIONS.WRITE]: true,
  },
  [ADMIN_PERMISSION_RESOURCES.SUBSCRIPTION]: {
    [ADMIN_PERMISSION_ACTIONS.READ]: true,
    [ADMIN_PERMISSION_ACTIONS.OPERATE]: true,
    [ADMIN_PERMISSION_ACTIONS.WRITE]: true,
  },
  [ADMIN_PERMISSION_RESOURCES.REDEMPTION]: {
    [ADMIN_PERMISSION_ACTIONS.READ]: true,
    [ADMIN_PERMISSION_ACTIONS.OPERATE]: true,
    [ADMIN_PERMISSION_ACTIONS.WRITE]: true,
  },
  [ADMIN_PERMISSION_RESOURCES.USAGE_LOG]: {
    [ADMIN_PERMISSION_ACTIONS.READ]: true,
  },
  [ADMIN_PERMISSION_RESOURCES.USAGE_DATA]: {
    [ADMIN_PERMISSION_ACTIONS.READ]: true,
  },
  [ADMIN_PERMISSION_RESOURCES.SYSTEM_SETTING]: {
    [ADMIN_PERMISSION_ACTIONS.READ]: true,
  },
}

function baselineAllows(
  role: number | undefined,
  resource: AdminPermissionResource,
  action: AdminPermissionAction
): boolean {
  if ((role ?? 0) >= ROLE.SUPER_ADMIN) return true
  if ((role ?? 0) < ROLE.ADMIN) return false
  return ADMIN_DEFAULT_GRANTS[resource]?.[action] === true
}

function staticAdminBaselineAllows(resource: string, action: string): boolean {
  return (
    ADMIN_DEFAULT_GRANTS[resource as AdminPermissionResource]?.[
      action as AdminPermissionAction
    ] === true
  )
}

// roleGrants 返回后端 catalog 中指定角色的基线矩阵。
export function roleGrants(
  catalog: PermissionCatalog,
  roleKey: string
): AdminPermissionMatrix {
  return catalog.roles.find((role) => role.key === roleKey)?.grants ?? {}
}

// normalizeAdminPermissions 根据后端 catalog 补齐完整权限矩阵。
//
// 用户编辑页必须提交完整矩阵，否则只保存局部字段会让未渲染动作继承旧状态。缺失值优先
// 回落到 Admin 角色基线；如果 catalog 临时缺少 roles，则使用前端静态基线兜底，保证旧
// 权限显隐逻辑和编辑器默认值保持一致。
export function normalizeAdminPermissions(
  value: AdminPermissionMatrix | null | undefined,
  catalog: PermissionCatalog
): AdminPermissionMatrix {
  const baseline = roleGrants(catalog, ADMIN_ROLE_KEY)
  const normalized: AdminPermissionMatrix = {}

  for (const resource of catalog.resources) {
    const actions: Record<string, boolean> = {}
    for (const action of resource.actions) {
      actions[action.action] =
        value?.[resource.resource]?.[action.action] ??
        baseline[resource.resource]?.[action.action] ??
        staticAdminBaselineAllows(resource.resource, action.action)
    }
    normalized[resource.resource] = actions
  }

  return normalized
}

export function hasAdminPermission(
  user: AuthUser | null | undefined,
  resource: AdminPermissionResource,
  action: AdminPermissionAction
): boolean {
  if (!user) return false
  if (user.role >= ROLE.SUPER_ADMIN) return true
  const explicit = user.permissions?.admin_permissions?.[resource]?.[action]
  if (typeof explicit === 'boolean') return explicit
  return baselineAllows(user.role, resource, action)
}

export function canReadAdminResource(
  user: AuthUser | null | undefined,
  resource: AdminPermissionResource
): boolean {
  return hasAdminPermission(user, resource, ADMIN_PERMISSION_ACTIONS.READ)
}
