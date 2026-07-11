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
import type {
  AdminPermissionMatrix,
  PermissionCatalog,
} from '@/lib/admin-permissions'

export type PermissionMatrixDiff = {
  changed: number
  enabled: number
  disabled: number
}

// 角色策略保存必须提交完整矩阵；这里以后端 catalog 为唯一来源补齐缺失动作，
// 避免前端只提交局部字段时把其它资源的授权状态意外清空或遗漏。
export function normalizeRolePolicyGrants(
  value: AdminPermissionMatrix | null | undefined,
  catalog: PermissionCatalog
): AdminPermissionMatrix {
  const normalized: AdminPermissionMatrix = {}

  for (const resource of catalog.resources) {
    const actions: Record<string, boolean> = {}
    for (const action of resource.actions) {
      actions[action.action] =
        value?.[resource.resource]?.[action.action] === true
    }
    normalized[resource.resource] = actions
  }

  return normalized
}

export function countTotalActions(catalog: PermissionCatalog): number {
  return catalog.resources.reduce(
    (total, resource) => total + resource.actions.length,
    0
  )
}

export function countEnabledActions(
  grants: AdminPermissionMatrix,
  catalog: PermissionCatalog
): number {
  return catalog.resources.reduce((total, resource) => {
    return (
      total +
      resource.actions.filter(
        (action) => grants[resource.resource]?.[action.action] === true
      ).length
    )
  }, 0)
}

export function countResourceEnabledActions(
  grants: AdminPermissionMatrix,
  catalog: PermissionCatalog,
  resourceKey: string
): number {
  const resource = catalog.resources.find(
    (item) => item.resource === resourceKey
  )
  if (!resource) return 0
  return resource.actions.filter(
    (action) => grants[resource.resource]?.[action.action] === true
  ).length
}

export function replaceResourceGrants(
  grants: AdminPermissionMatrix,
  catalog: PermissionCatalog,
  resourceKey: string,
  enabled: boolean
): AdminPermissionMatrix {
  const normalized = normalizeRolePolicyGrants(grants, catalog)
  const resource = catalog.resources.find(
    (item) => item.resource === resourceKey
  )
  if (!resource) return normalized

  normalized[resource.resource] = Object.fromEntries(
    resource.actions.map((action) => [action.action, enabled])
  )

  return normalized
}

export function diffPermissionMatrix(
  before: AdminPermissionMatrix,
  after: AdminPermissionMatrix,
  catalog: PermissionCatalog
): PermissionMatrixDiff {
  const diff: PermissionMatrixDiff = {
    changed: 0,
    enabled: 0,
    disabled: 0,
  }

  for (const resource of catalog.resources) {
    for (const action of resource.actions) {
      const beforeValue = before[resource.resource]?.[action.action] === true
      const afterValue = after[resource.resource]?.[action.action] === true
      if (beforeValue === afterValue) continue

      diff.changed += 1
      if (afterValue) {
        diff.enabled += 1
      } else {
        diff.disabled += 1
      }
    }
  }

  return diff
}

export function permissionMatrixSignature(
  grants: AdminPermissionMatrix,
  catalog: PermissionCatalog
): string {
  return catalog.resources
    .map((resource) => {
      const values = resource.actions
        .map((action) =>
          grants[resource.resource]?.[action.action] === true ? '1' : '0'
        )
        .join('')
      return `${resource.resource}:${values}`
    })
    .join('|')
}
