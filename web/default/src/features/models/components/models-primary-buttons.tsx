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
import {
  Plus,
  MoreHorizontal,
  RefreshCw,
  List,
  Building2,
  AlertCircle,
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
import { useModelPermissions } from '../hooks/use-model-permissions'
import { useModels } from './models-provider'

export function ModelsPrimaryButtons() {
  const { t } = useTranslation()
  const { setOpen, setCurrentRow } = useModels()
  const permissions = useModelPermissions()
  const noPermissionMessage = t("You don't have necessary permission")
  const canSyncUpstream = permissions.canOperate && permissions.canWrite
  const canManagePrefillGroups =
    permissions.canWrite || permissions.canSensitiveWrite

  const guardPermission = (allowed: boolean) => {
    if (allowed) return true
    toast.error(noPermissionMessage)
    return false
  }

  const handleCreateModel = () => {
    if (!guardPermission(permissions.canWrite)) return
    setCurrentRow(null)
    setOpen('create-model')
  }

  const handleMissingModels = () => {
    setOpen('missing-models')
  }

  const handleSync = () => {
    if (!guardPermission(canSyncUpstream)) return
    setOpen('sync-wizard')
  }

  const handlePrefillGroups = () => {
    if (!guardPermission(canManagePrefillGroups)) return
    setOpen('prefill-groups')
  }

  const handleManageVendors = () => {
    if (!guardPermission(permissions.canWrite)) return
    setOpen('create-vendor') // Will be a separate vendors management dialog
  }

  return (
    <div className='flex items-center gap-2'>
      {/* 创建模型 */}
      <Button
        onClick={handleCreateModel}
        size='sm'
        disabled={!permissions.canWrite}
        title={permissions.canWrite ? undefined : noPermissionMessage}
      >
        <Plus data-icon='inline-start' />
        {t('Add Model')}
      </Button>

      {/* 更多操作 */}
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button variant='outline' size='sm' />}>
          <MoreHorizontal />
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' className='w-56'>
          <DropdownMenuItem onClick={handleMissingModels}>
            {t('Missing Models')}
            <DropdownMenuShortcut>
              <AlertCircle />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          <DropdownMenuItem
            onClick={handleSync}
            disabled={!canSyncUpstream}
            title={canSyncUpstream ? undefined : noPermissionMessage}
          >
            {t('Sync Source Models')}
            <DropdownMenuShortcut>
              <RefreshCw />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          <DropdownMenuSeparator />

          <DropdownMenuItem
            onClick={handlePrefillGroups}
            disabled={!canManagePrefillGroups}
            title={canManagePrefillGroups ? undefined : noPermissionMessage}
          >
            {t('Prefill Groups')}
            <DropdownMenuShortcut>
              <List />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          <DropdownMenuItem
            onClick={handleManageVendors}
            disabled={!permissions.canWrite}
            title={permissions.canWrite ? undefined : noPermissionMessage}
          >
            {t('Manage Vendors')}
            <DropdownMenuShortcut>
              <Building2 />
            </DropdownMenuShortcut>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
