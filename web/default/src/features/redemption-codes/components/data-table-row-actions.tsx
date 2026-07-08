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
import { type Row } from '@tanstack/react-table'
import {
  Trash2,
  Edit,
  Power,
  PowerOff,
  MoreHorizontal as DotsHorizontalIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { updateRedemptionStatus } from '../api'
import { REDEMPTION_STATUS, SUCCESS_MESSAGES } from '../constants'
import { useRedemptionPermissions } from '../hooks/use-redemption-permissions'
import { isRedemptionExpired } from '../lib'
import { redemptionSchema } from '../types'
import { useRedemptions } from './redemptions-provider'

interface DataTableRowActionsProps<TData> {
  row: Row<TData>
}

export function DataTableRowActions<TData>({
  row,
}: DataTableRowActionsProps<TData>) {
  const { t } = useTranslation()
  const redemption = redemptionSchema.parse(row.original)
  const { setOpen, setCurrentRow, triggerRefresh } = useRedemptions()
  const permissions = useRedemptionPermissions()
  const noPermissionMessage = t("You don't have necessary permission")
  const isEnabled = redemption.status === REDEMPTION_STATUS.ENABLED
  const isUsed = redemption.status === REDEMPTION_STATUS.USED
  const isExpired = isRedemptionExpired(
    redemption.expired_time,
    redemption.status
  )

  const handleToggleStatus = async () => {
    if (!permissions.canWrite) {
      toast.error(noPermissionMessage)
      return
    }
    const newStatus = isEnabled
      ? REDEMPTION_STATUS.DISABLED
      : REDEMPTION_STATUS.ENABLED

    const result = await updateRedemptionStatus(redemption.id, newStatus)
    if (result.success) {
      const message = isEnabled
        ? t(SUCCESS_MESSAGES.REDEMPTION_DISABLED)
        : t(SUCCESS_MESSAGES.REDEMPTION_ENABLED)
      toast.success(message)
      triggerRefresh()
    }
  }

  const canEdit = isEnabled && !isExpired && permissions.canWrite
  const canToggleByState = !isUsed && !isExpired

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            className='data-popup-open:bg-muted flex h-8 w-8 p-0'
          />
        }
      >
        <DotsHorizontalIcon />
        <span className='sr-only'>{t('Open menu')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-[160px]'>
        <DropdownMenuItem
          onClick={() => {
            if (!permissions.canWrite) {
              toast.error(noPermissionMessage)
              return
            }
            setCurrentRow(redemption)
            setOpen('update')
          }}
          disabled={!canEdit}
          title={permissions.canWrite ? undefined : noPermissionMessage}
        >
          {t('Edit')}
          <DropdownMenuShortcut>
            <Edit />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
        {canToggleByState && (
          <DropdownMenuItem
            onClick={handleToggleStatus}
            disabled={!permissions.canWrite}
            title={permissions.canWrite ? undefined : noPermissionMessage}
          >
            {isEnabled ? (
              <>
                {t('Disable')}
                <DropdownMenuShortcut>
                  <PowerOff />
                </DropdownMenuShortcut>
              </>
            ) : (
              <>
                {t('Enable')}
                <DropdownMenuShortcut>
                  <Power />
                </DropdownMenuShortcut>
              </>
            )}
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => {
            if (!permissions.canSensitiveWrite) {
              toast.error(noPermissionMessage)
              return
            }
            setCurrentRow(redemption)
            setOpen('delete')
          }}
          className='text-destructive focus:text-destructive'
          disabled={!permissions.canSensitiveWrite}
          title={
            permissions.canSensitiveWrite ? undefined : noPermissionMessage
          }
        >
          {t('Delete')}
          <DropdownMenuShortcut>
            <Trash2 />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
