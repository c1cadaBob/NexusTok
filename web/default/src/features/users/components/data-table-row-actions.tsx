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
import { type Row } from '@tanstack/react-table'
import {
  MoreHorizontal,
  Pencil,
  Trash2,
  Power,
  PowerOff,
  ArrowUp,
  ArrowDown,
  KeyRound,
  ShieldAlert,
  Link2,
  CreditCard,
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
import { ConfirmDialog } from '@/components/confirm-dialog'
import { UserSubscriptionsDialog } from '@/features/subscriptions/components/dialogs/user-subscriptions-dialog'
import { useSubscriptionPermissions } from '@/features/subscriptions/hooks/use-subscription-permissions'
import { manageUser, resetUserPasskey, resetUserTwoFA } from '../api'
import {
  USER_STATUS,
  USER_ROLE,
  ERROR_MESSAGES,
  isUserDeleted,
} from '../constants'
import { useUserPermissions } from '../hooks/use-user-permissions'
import { getUserActionMessage } from '../lib'
import { type User, type ManageUserAction } from '../types'
import { UserBindingDialog } from './dialogs/user-binding-dialog'
import { useUsers } from './users-provider'

interface DataTableRowActionsProps {
  row: Row<User>
}

export function DataTableRowActions({ row }: DataTableRowActionsProps) {
  const { t } = useTranslation()
  const user = row.original
  const { setOpen, setCurrentRow, triggerRefresh } = useUsers()
  const [resetPasskeyOpen, setResetPasskeyOpen] = useState(false)
  const [resetTwoFAOpen, setResetTwoFAOpen] = useState(false)
  const [bindingDialogOpen, setBindingDialogOpen] = useState(false)
  const [subscriptionsDialogOpen, setSubscriptionsDialogOpen] = useState(false)
  const permissions = useUserPermissions()
  const subscriptionPermissions = useSubscriptionPermissions()
  const noPermissionMessage = t("You don't have necessary permission")

  const guardPermission = (allowed: boolean) => {
    if (allowed) return true
    toast.error(noPermissionMessage)
    return false
  }

  const handleEdit = () => {
    if (!guardPermission(permissions.canWrite)) return
    setCurrentRow(user)
    setOpen('update')
  }

  const handleDelete = () => {
    if (!guardPermission(permissions.canSensitiveWrite)) return
    setCurrentRow(user)
    setOpen('delete')
  }

  const handleManage = async (action: Exclude<ManageUserAction, 'delete'>) => {
    const allowed =
      action === 'promote' || action === 'demote'
        ? permissions.canOperate && permissions.canSensitiveWrite
        : permissions.canOperate
    if (!guardPermission(allowed)) return

    try {
      const result = await manageUser(user.id, action)
      if (result.success) {
        toast.success(t(getUserActionMessage(action)))
        triggerRefresh()
      } else {
        toast.error(
          result.message || t('Failed to {{action}} user', { action })
        )
      }
    } catch (_error) {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    }
  }

  const handleResetPasskey = async () => {
    if (!guardPermission(permissions.canOperate)) return
    try {
      const result = await resetUserPasskey(user.id)
      if (result.success) {
        toast.success(t('Passkey reset successfully'))
        triggerRefresh()
      } else {
        toast.error(result.message || t('Failed to reset Passkey'))
      }
    } catch (_error) {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setResetPasskeyOpen(false)
    }
  }

  const handleResetTwoFA = async () => {
    if (!guardPermission(permissions.canOperate)) return
    try {
      const result = await resetUserTwoFA(user.id)
      if (result.success) {
        toast.success(t('Two-factor authentication reset'))
        triggerRefresh()
      } else {
        toast.error(result.message || t('Failed to reset 2FA'))
      }
    } catch (_error) {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setResetTwoFAOpen(false)
    }
  }

  const isDisabled = user.status === USER_STATUS.DISABLED
  const isAdmin = user.role >= USER_ROLE.ADMIN
  const isRoot = user.role === USER_ROLE.ROOT
  const canPromoteOrDemote =
    permissions.canOperate && permissions.canSensitiveWrite

  if (isUserDeleted(user)) {
    return null
  }

  return (
    <>
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
        <DropdownMenuContent align='end' className='w-[180px]'>
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

          {isDisabled ? (
            <DropdownMenuItem
              onClick={() => handleManage('enable')}
              disabled={!permissions.canOperate}
              title={permissions.canOperate ? undefined : noPermissionMessage}
            >
              {t('Enable')}
              <DropdownMenuShortcut>
                <Power />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem
              onClick={() => handleManage('disable')}
              disabled={isRoot || !permissions.canOperate}
              title={permissions.canOperate ? undefined : noPermissionMessage}
            >
              {t('Disable')}
              <DropdownMenuShortcut>
                <PowerOff />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
          )}

          {isAdmin && !isRoot && (
            <DropdownMenuItem
              onClick={() => handleManage('demote')}
              disabled={!canPromoteOrDemote}
              title={canPromoteOrDemote ? undefined : noPermissionMessage}
            >
              {t('Demote')}
              <DropdownMenuShortcut>
                <ArrowDown />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
          )}

          {!isAdmin && (
            <DropdownMenuItem
              onClick={() => handleManage('promote')}
              disabled={!canPromoteOrDemote}
              title={canPromoteOrDemote ? undefined : noPermissionMessage}
            >
              {t('Promote')}
              <DropdownMenuShortcut>
                <ArrowUp />
              </DropdownMenuShortcut>
            </DropdownMenuItem>
          )}

          <DropdownMenuItem
            onSelect={(event) => {
              event.preventDefault()
              if (!guardPermission(permissions.canRead)) return
              setBindingDialogOpen(true)
            }}
            disabled={!permissions.canRead}
            title={permissions.canRead ? undefined : noPermissionMessage}
          >
            {t('Manage Bindings')}
            <DropdownMenuShortcut>
              <Link2 />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          <DropdownMenuItem
            onSelect={(event) => {
              event.preventDefault()
              if (!guardPermission(subscriptionPermissions.canRead)) return
              setSubscriptionsDialogOpen(true)
            }}
            disabled={!subscriptionPermissions.canRead}
            title={
              subscriptionPermissions.canRead ? undefined : noPermissionMessage
            }
          >
            {t('Manage Subscriptions')}
            <DropdownMenuShortcut>
              <CreditCard />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          <DropdownMenuSeparator />

          <DropdownMenuItem
            onSelect={(event) => {
              event.preventDefault()
              if (!guardPermission(permissions.canOperate)) return
              setResetPasskeyOpen(true)
            }}
            disabled={isRoot || !permissions.canOperate}
            title={permissions.canOperate ? undefined : noPermissionMessage}
          >
            {t('Reset Passkey')}
            <DropdownMenuShortcut>
              <KeyRound />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          <DropdownMenuItem
            onSelect={(event) => {
              event.preventDefault()
              if (!guardPermission(permissions.canOperate)) return
              setResetTwoFAOpen(true)
            }}
            disabled={isRoot || !permissions.canOperate}
            title={permissions.canOperate ? undefined : noPermissionMessage}
          >
            {t('Reset 2FA')}
            <DropdownMenuShortcut>
              <ShieldAlert />
            </DropdownMenuShortcut>
          </DropdownMenuItem>

          <DropdownMenuSeparator />

          <DropdownMenuItem
            onClick={handleDelete}
            className='text-destructive focus:text-destructive'
            disabled={isRoot || !permissions.canSensitiveWrite}
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

      <ConfirmDialog
        open={resetPasskeyOpen}
        onOpenChange={setResetPasskeyOpen}
        title={t('Reset Passkey')}
        desc={`Reset Passkey for ${user.username}? The user will need to register a new Passkey before using passwordless login.`}
        confirmText='Reset Passkey'
        handleConfirm={handleResetPasskey}
        disabled={!permissions.canOperate}
      />

      <ConfirmDialog
        open={resetTwoFAOpen}
        onOpenChange={setResetTwoFAOpen}
        title={t('Reset Two-Factor Authentication')}
        desc={`Reset 2FA for ${user.username}? The user must set up 2FA again to continue using it.`}
        confirmText='Reset 2FA'
        handleConfirm={handleResetTwoFA}
        disabled={!permissions.canOperate}
      />

      <UserBindingDialog
        open={bindingDialogOpen}
        onOpenChange={setBindingDialogOpen}
        userId={user.id}
        onUnbindSuccess={triggerRefresh}
        canOperate={permissions.canOperate}
        disabledReason={noPermissionMessage}
      />

      <UserSubscriptionsDialog
        open={subscriptionsDialogOpen}
        onOpenChange={setSubscriptionsDialogOpen}
        user={{ id: user.id, username: user.username }}
        onSuccess={triggerRefresh}
      />
    </>
  )
}
