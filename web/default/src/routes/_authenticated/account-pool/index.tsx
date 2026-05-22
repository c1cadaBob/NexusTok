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
import { useEffect } from 'react'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'

const ACCOUNT_POOL_MANAGER_URL = '/account-pool/manager/'

export const Route = createFileRoute('/_authenticated/account-pool/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: AccountPoolRedirect,
})

function AccountPoolRedirect() {
  const { t } = useTranslation()

  useEffect(() => {
    window.location.assign(ACCOUNT_POOL_MANAGER_URL)
  }, [])

  return (
    <main className='flex min-h-[50vh] flex-col items-center justify-center gap-4 text-center'>
      <p className='text-muted-foreground'>
        {t('Opening account pool management...')}
      </p>
      <Button onClick={() => window.location.assign(ACCOUNT_POOL_MANAGER_URL)}>
        {t('Open Account Pool Management')}
      </Button>
    </main>
  )
}
