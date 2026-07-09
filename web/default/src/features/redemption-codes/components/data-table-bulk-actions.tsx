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
import { useCallback, useState } from 'react'
import { type Table } from '@tanstack/react-table'
import { Trash2 } from 'lucide-react'
import { Copy01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { Spinner } from '@/components/ui/spinner'
import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { deleteInvalidRedemptions, getRedemptionKey } from '../api'
import { useRedemptionPermissions } from '../hooks/use-redemption-permissions'
import { type Redemption } from '../types'
import { useRedemptions } from './redemptions-provider'

type DataTableBulkActionsProps<TData> = {
  table: Table<TData>
}

export function DataTableBulkActions<TData>({
  table,
}: DataTableBulkActionsProps<TData>) {
  const { t } = useTranslation()
  const { triggerRefresh } = useRedemptions()
  const permissions = useRedemptionPermissions()
  const noPermissionMessage = t("You don't have necessary permission")
  const [showDeleteInvalidConfirm, setShowDeleteInvalidConfirm] =
    useState(false)
  const [isDeleting, setIsDeleting] = useState(false)
  const [isCopying, setIsCopying] = useState(false)
  const selectedRows = table.getFilteredSelectedRowModel().rows
  const { copyToClipboard } = useCopyToClipboard({
    notify: true,
    successMessage: t('Codes copied!'),
  })

  const fetchSelectedCodes = useCallback(async () => {
    if (!permissions.canViewSecret) {
      throw new Error(noPermissionMessage)
    }
    const selectedCodes = await Promise.all(
      selectedRows.map(async (row) => {
        const redemption = row.original as Redemption
        const response = await getRedemptionKey(redemption.id)
        if (!response.success || !response.data?.key) {
          throw new Error(
            response.message || 'Failed to fetch redemption code'
          )
        }
        return `${redemption.name}\t${response.data.key}`
      })
    )
    return selectedCodes.join('\n')
  }, [noPermissionMessage, permissions.canViewSecret, selectedRows])

  const {
    open: verificationOpen,
    setOpen: setVerificationOpen,
    methods: verificationMethods,
    state: verificationState,
    withVerification,
    executeVerification,
    cancel: cancelVerification,
    setCode: setVerificationCode,
    switchMethod: switchVerificationMethod,
  } = useSecureVerification({
    successMessage: t('Redemption codes unlocked'),
    onSuccess: async (result) => {
      if (typeof result === 'string' && result.length > 0) {
        await copyToClipboard(result)
      }
    },
  })

  const handleCopySelectedCodes = async () => {
    if (!permissions.canViewSecret) {
      toast.error(noPermissionMessage)
      return
    }
    setIsCopying(true)
    try {
      const result = await withVerification(fetchSelectedCodes, {
        preferredMethod: 'passkey',
        title: t('Verify to copy redemption codes'),
        description: t(
          'Use Passkey or 2FA to confirm your identity before copying selected redemption codes.'
        ),
      })
      if (typeof result === 'string' && result.length > 0) {
        await copyToClipboard(result)
      }
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to fetch redemption code')
      toast.error(message)
    } finally {
      setIsCopying(false)
    }
  }

  const selectedCount = selectedRows.length
  const copyDisabled =
    selectedCount === 0 ||
    !permissions.canViewSecret ||
    isCopying ||
    verificationState.loading

  const copyTooltip = permissions.canViewSecret
    ? t('Copy selected codes')
    : noPermissionMessage

  const handleDeleteInvalid = async () => {
    if (!permissions.canSensitiveWrite) {
      toast.error(noPermissionMessage)
      return
    }
    setIsDeleting(true)
    try {
      const result = await deleteInvalidRedemptions()

      if (result.success) {
        const count = result.data || 0
        toast.success(
          t('Successfully deleted {{count}} invalid redemption codes', {
            count,
          })
        )
        table.resetRowSelection()
        triggerRefresh()
        setShowDeleteInvalidConfirm(false)
      }
    } finally {
      setIsDeleting(false)
    }
  }

  return (
    <>
      <BulkActionsToolbar table={table} entityName={t('redemption code')}>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                disabled={copyDisabled}
                title={copyTooltip}
                aria-label={t('Copy selected codes')}
                onClick={handleCopySelectedCodes}
              />
            }
          >
            {isCopying || verificationState.loading ? (
              <Spinner />
            ) : (
              <HugeiconsIcon icon={Copy01Icon} />
            )}
            <span className='sr-only'>{t('Copy selected codes')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{copyTooltip}</p>
          </TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='destructive'
                size='icon'
                onClick={() => {
                  if (!permissions.canSensitiveWrite) {
                    toast.error(noPermissionMessage)
                    return
                  }
                  setShowDeleteInvalidConfirm(true)
                }}
                className='size-8'
                disabled={!permissions.canSensitiveWrite}
                aria-label={t('Delete invalid redemption codes')}
                title={
                  permissions.canSensitiveWrite
                    ? t('Delete invalid redemption codes')
                    : noPermissionMessage
                }
              />
            }
          >
            <Trash2 />
            <span className='sr-only'>{t('Delete invalid codes')}</span>
          </TooltipTrigger>
          <TooltipContent>
            <p>{t('Delete invalid codes (used/disabled/expired)')}</p>
          </TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>

      <ConfirmDialog
        destructive
        open={showDeleteInvalidConfirm}
        onOpenChange={setShowDeleteInvalidConfirm}
        handleConfirm={handleDeleteInvalid}
        disabled={!permissions.canSensitiveWrite}
        isLoading={isDeleting}
        className='max-w-md'
        title={t('Delete Invalid Redemption Codes?')}
        desc={
          <>
            {t('This will delete all')} <strong>{t('used')}</strong>,{' '}
            <strong>{t('disabled')}</strong>
            {t(', and')} <strong>{t('expired')}</strong>{' '}
            {t('redemption codes.')}
            <br />
            {t('This action cannot be undone.')}
          </>
        }
        confirmText={t('Delete Invalid')}
      />

      <SecureVerificationDialog
        open={verificationOpen}
        onOpenChange={(open) => {
          setVerificationOpen(open)
          if (!open) {
            cancelVerification()
          }
        }}
        methods={verificationMethods}
        state={verificationState}
        onVerify={async (method, code) => {
          await executeVerification(method, code)
        }}
        onCancel={cancelVerification}
        onCodeChange={setVerificationCode}
        onMethodChange={switchVerificationMethod}
      />
    </>
  )
}
