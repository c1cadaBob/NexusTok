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
import { z } from 'zod'
import {
  type AdminPermissionMatrix,
  type PermissionCatalog,
  normalizeAdminPermissions,
} from '@/lib/admin-permissions'
import { quotaUnitsToDollars } from '@/lib/format'
import { ROLE } from '@/lib/roles'
import { DEFAULT_GROUP } from '../constants'
import { type UserFormData, type User } from '../types'

export const AUTHZ_ROLE_ADMIN_VALUE = 'admin'
export const USERNAME_MAX_LENGTH = 20
export const USER_PASSWORD_MIN_LENGTH = 8
export const USER_PASSWORD_MAX_LENGTH = 20

const optionalPasswordSchema = z
  .string()
  .refine(
    (value) =>
      value.trim() === '' || value.length >= USER_PASSWORD_MIN_LENGTH,
    {
      message: 'Password must be at least 8 characters long',
    }
  )
  .refine(
    (value) =>
      value.trim() === '' || value.length <= USER_PASSWORD_MAX_LENGTH,
    {
      message: 'Password must be at most 20 characters long',
    }
  )

// ============================================================================
// Form Schema
// ============================================================================

export const userFormSchema = z.object({
  username: z
    .string()
    .trim()
    .min(1, 'Username is required')
    .max(USERNAME_MAX_LENGTH, 'Username must be at most 20 characters long'),
  display_name: z.string().optional(),
  password: optionalPasswordSchema,
  role: z.number().optional(),
  quota_dollars: z.number().min(0).optional(),
  group: z.string().optional(),
  remark: z.string().optional(),
  authz_role: z.string().optional(),
  admin_permissions: z
    .record(z.string(), z.record(z.string(), z.boolean()))
    .optional(),
})

export type UserFormValues = z.infer<typeof userFormSchema>

// ============================================================================
// Form Defaults
// ============================================================================

export const USER_FORM_DEFAULT_VALUES: UserFormValues = {
  username: '',
  display_name: '',
  password: '',
  role: 1, // Default to common user
  quota_dollars: 0,
  group: DEFAULT_GROUP,
  remark: '',
  authz_role: AUTHZ_ROLE_ADMIN_VALUE,
  admin_permissions: {},
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * 将表单值转换为用户管理接口 payload。
 */
export function transformFormDataToPayload(
  data: UserFormValues,
  userId?: number,
  catalog?: PermissionCatalog
): UserFormData & { id?: number } {
  const payload: UserFormData & { id?: number } = {
    username: data.username,
    display_name: data.display_name || data.username,
    password: data.password || undefined,
  }

  const role = userId === undefined ? data.role || ROLE.USER : (data.role ?? 0)

  // 权限矩阵必须基于后端 catalog 补齐后提交。catalog 不可用时省略该字段，
  // 后端会保留现有 override，避免前端保存部分矩阵导致权限被意外重置。
  if (role >= ROLE.ADMIN && catalog) {
    const authzRole = (data.authz_role || AUTHZ_ROLE_ADMIN_VALUE)
      .trim()
      .toLowerCase()
    payload.authz_role = authzRole === AUTHZ_ROLE_ADMIN_VALUE ? '' : authzRole
    payload.admin_permissions = normalizeAdminPermissions(
      data.admin_permissions as AdminPermissionMatrix | undefined,
      catalog
    )
  }

  // 新建用户时只提交创建所需字段，角色只能在创建阶段由后台指定。
  if (userId === undefined) {
    payload.role = role
  } else {
    // 编辑用户时额度通过 /api/user/manage 原子调整，不在资料保存接口里写入。
    payload.group = data.group
    payload.remark = data.remark || undefined
    payload.id = userId
  }

  return payload
}

/**
 * 将用户详情转换为抽屉表单默认值。
 */
export function transformUserToFormDefaults(user: User): UserFormValues {
  return {
    username: user.username,
    display_name: user.display_name,
    password: '',
    role: user.role,
    quota_dollars: quotaUnitsToDollars(user.quota),
    group: user.group || DEFAULT_GROUP,
    remark: user.remark || '',
    authz_role: user.authz_role?.trim() || AUTHZ_ROLE_ADMIN_VALUE,
    admin_permissions: user.admin_permissions ?? {},
  }
}
