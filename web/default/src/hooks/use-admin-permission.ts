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
import { useAuthStore } from '@/stores/auth-store'
import {
  type AdminPermissionAction,
  type AdminPermissionResource,
  ADMIN_PERMISSION_ACTIONS,
  hasAdminPermission,
} from '@/lib/admin-permissions'

export function useAdminPermission(
  resource: AdminPermissionResource,
  action: AdminPermissionAction
): boolean {
  const user = useAuthStore((state) => state.auth.user)
  return hasAdminPermission(user, resource, action)
}

export function useCanReadAdminResource(
  resource: AdminPermissionResource
): boolean {
  return useAdminPermission(resource, ADMIN_PERMISSION_ACTIONS.READ)
}
