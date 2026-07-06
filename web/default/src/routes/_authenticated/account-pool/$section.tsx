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
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import { AccountPool } from '@/features/account-pool'
import {
  ACCOUNT_POOL_DEFAULT_SECTION,
  isAccountPoolSectionId,
} from '@/features/account-pool/section-registry'

export const Route = createFileRoute('/_authenticated/account-pool/$section')({
  beforeLoad: ({ params }) => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
    if (!isAccountPoolSectionId(params.section)) {
      throw redirect({
        to: '/account-pool/$section',
        params: { section: ACCOUNT_POOL_DEFAULT_SECTION },
        replace: true,
      })
    }
  },
  component: AccountPool,
})
