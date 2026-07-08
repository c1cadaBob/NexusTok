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
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { useRedemptionPermissions } from '../hooks/use-redemption-permissions'
import { useRedemptions } from './redemptions-provider'

export function RedemptionsPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen } = useRedemptions()
  const permissions = useRedemptionPermissions()
  const noPermissionMessage = t("You don't have necessary permission")

  const handleCreate = () => {
    if (!permissions.canWrite) {
      toast.error(noPermissionMessage)
      return
    }
    setOpen('create')
  }

  return (
    <div className='flex gap-2'>
      <Button
        size='sm'
        onClick={handleCreate}
        disabled={!permissions.canWrite}
        title={permissions.canWrite ? undefined : noPermissionMessage}
      >
        <Plus data-icon='inline-start' />
        {t('Create Code')}
      </Button>
    </div>
  )
}
