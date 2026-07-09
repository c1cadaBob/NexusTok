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
import { MoreHorizontal, Pencil, Power, PowerOff, RotateCcw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useSubscriptionPermissions } from '../hooks/use-subscription-permissions'
import type { PlanRecord } from '../types'
import { useSubscriptions } from './subscriptions-provider'

interface DataTableRowActionsProps {
  row: Row<PlanRecord>
}

export function DataTableRowActions({ row }: DataTableRowActionsProps) {
  const { t } = useTranslation()
  const { setOpen, setCurrentRow, complianceConfirmed } = useSubscriptions()
  const permissions = useSubscriptionPermissions()
  const noPermissionMessage = t("You don't have necessary permission")

  const guardWrite = () => {
    if (permissions.canWrite) return true
    toast.error(noPermissionMessage)
    return false
  }
  const guardOperate = () => {
    if (permissions.canOperate) return true
    toast.error(noPermissionMessage)
    return false
  }

  const canWritePlan = complianceConfirmed && permissions.canWrite
  const canOperatePlan = complianceConfirmed && permissions.canOperate

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant='ghost' className='h-8 w-8 p-0' />}
      >
        <MoreHorizontal />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end'>
        <DropdownMenuGroup>
          <DropdownMenuItem
            disabled={!canWritePlan}
            title={permissions.canWrite ? undefined : noPermissionMessage}
            onClick={() => {
              if (!guardWrite() || !complianceConfirmed) return
              setCurrentRow(row.original)
              setOpen('update')
            }}
          >
            <Pencil />
            {t('Edit')}
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={!canOperatePlan}
            title={permissions.canOperate ? undefined : noPermissionMessage}
            onClick={() => {
              if (!guardOperate() || !complianceConfirmed) return
              setCurrentRow(row.original)
              setOpen('reset-subscriptions')
            }}
          >
            <RotateCcw />
            {t('Reset quota')}
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={!canWritePlan}
            title={permissions.canWrite ? undefined : noPermissionMessage}
            onClick={() => {
              if (!guardWrite() || !complianceConfirmed) return
              setCurrentRow(row.original)
              setOpen('toggle-status')
            }}
          >
            {row.original.plan.enabled ? (
              <>
                <PowerOff />
                {t('Disable')}
              </>
            ) : (
              <>
                <Power />
                {t('Enable')}
              </>
            )}
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
