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
import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { type Row } from '@tanstack/react-table'
import { MoreHorizontal, Pencil, Power, PowerOff, Trash2 } from 'lucide-react'
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
import { ConfirmDialog } from '@/components/confirm-dialog'
import { useModelPermissions } from '../hooks/use-model-permissions'
import {
  handleDeleteModel,
  handleToggleModelStatus,
  isModelEnabled,
} from '../lib'
import type { Model } from '../types'
import { useModels } from './models-provider'

interface DataTableRowActionsProps {
  row: Row<Model>
}

export function DataTableRowActions({ row }: DataTableRowActionsProps) {
  const { t } = useTranslation()
  const model = row.original
  const { setOpen, setCurrentRow } = useModels()
  const queryClient = useQueryClient()
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false)
  const permissions = useModelPermissions()
  const noPermissionMessage = t("You don't have necessary permission")

  const isEnabled = isModelEnabled(model)
  const guardPermission = (allowed: boolean) => {
    if (allowed) return true
    toast.error(noPermissionMessage)
    return false
  }

  const handleEdit = () => {
    if (!guardPermission(permissions.canWrite)) return
    setCurrentRow(model)
    setOpen('update-model')
  }

  const handleToggleStatus = () => {
    if (!guardPermission(permissions.canWrite)) return
    handleToggleModelStatus(model.id, model.status, queryClient)
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            className='data-popup-open:bg-muted flex h-8 w-8 p-0'
          />
        }
      >
        <MoreHorizontal />
        <span className='sr-only'>{t('Open menu')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-48'>
        {/* 编辑 */}
        <DropdownMenuItem
          onClick={handleEdit}
          disabled={!permissions.canWrite}
          title={permissions.canWrite ? undefined : noPermissionMessage}
        >
          {t('Edit')}
          <DropdownMenuShortcut>
            <Pencil />
          </DropdownMenuShortcut>
        </DropdownMenuItem>

        <DropdownMenuSeparator />

        {/* 启用/禁用 */}
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

        <DropdownMenuSeparator />

        {/* 删除 */}
        <DropdownMenuItem
          onSelect={(e) => {
            e.preventDefault()
            if (!guardPermission(permissions.canSensitiveWrite)) return
            setDeleteConfirmOpen(true)
          }}
          disabled={!permissions.canSensitiveWrite}
          title={
            permissions.canSensitiveWrite ? undefined : noPermissionMessage
          }
          className='text-destructive focus:text-destructive'
        >
          {t('Delete')}
          <DropdownMenuShortcut>
            <Trash2 />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>

      <ConfirmDialog
        open={deleteConfirmOpen}
        onOpenChange={setDeleteConfirmOpen}
        title={t('Delete Model')}
        desc={`Are you sure you want to delete "${model.model_name}"? This action cannot be undone.`}
        confirmText='Delete'
        destructive
        handleConfirm={() => {
          if (!guardPermission(permissions.canSensitiveWrite)) return
          handleDeleteModel(model.id, queryClient)
          setDeleteConfirmOpen(false)
        }}
      />
    </DropdownMenu>
  )
}
