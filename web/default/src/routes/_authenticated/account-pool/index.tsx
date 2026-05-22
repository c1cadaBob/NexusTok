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
import { useTranslation } from 'react-i18next'
import { Main } from '@/components/layout/components/main'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'

const ACCOUNT_POOL_MANAGER_URL = '/account-pool/manager/?embeddedFrame=true'

export const Route = createFileRoute('/_authenticated/account-pool/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: AccountPoolFrame,
})

function AccountPoolFrame() {
  const { t } = useTranslation()

  return (
    <Main className='bg-background p-0'>
      <iframe
        title={t('Account Pool Management')}
        src={ACCOUNT_POOL_MANAGER_URL}
        className='min-h-0 w-full flex-1 border-0 bg-background'
        allow='clipboard-read; clipboard-write'
      />
    </Main>
  )
}
