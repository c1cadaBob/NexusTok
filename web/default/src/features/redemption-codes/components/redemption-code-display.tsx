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
import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { CopyButton } from '@/components/copy-button'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { getRedemptionKey } from '../api'
import { useRedemptionPermissions } from '../hooks/use-redemption-permissions'
import type { Redemption } from '../types'

type PendingAction = 'view' | 'copy'

type RedemptionCodeDisplayProps = {
  redemption: Redemption
}

function extractKey(result: unknown): string | null {
  return typeof result === 'string' && result.length > 0 ? result : null
}

export function RedemptionCodeDisplay({
  redemption,
}: RedemptionCodeDisplayProps) {
  const { t } = useTranslation()
  const permissions = useRedemptionPermissions()
  const noPermissionMessage = t("You don't have necessary permission")
  const [fullCode, setFullCode] = useState<string | null>(
    redemption.key_redacted === false ? redemption.key : null
  )
  const [popoverOpen, setPopoverOpen] = useState(false)
  const pendingActionRef = useRef<PendingAction | null>(null)
  const { copyToClipboard } = useCopyToClipboard({
    notify: true,
    successMessage: t('Code copied!'),
  })

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
    successMessage: t('Redemption code unlocked'),
    onSuccess: async (result) => {
      const key = extractKey(result)
      if (!key) return

      setFullCode(key)
      if (pendingActionRef.current === 'copy') {
        await copyToClipboard(key)
      } else if (pendingActionRef.current === 'view') {
        setPopoverOpen(true)
      }
      pendingActionRef.current = null
    },
    onError: () => {
      pendingActionRef.current = null
    },
  })

  const fetchFullCode = useCallback(async () => {
    if (!permissions.canViewSecret) {
      throw new Error(noPermissionMessage)
    }
    const response = await getRedemptionKey(redemption.id)
    if (!response.success || !response.data?.key) {
      throw new Error(response.message || 'Failed to fetch redemption code')
    }
    setFullCode(response.data.key)
    return response.data.key
  }, [noPermissionMessage, permissions.canViewSecret, redemption.id])

  const ensureFullCode = useCallback(
    async (action: PendingAction) => {
      if (fullCode) {
        if (action === 'view') {
          setPopoverOpen(true)
        }
        return fullCode
      }

      if (!permissions.canViewSecret) {
        toast.error(noPermissionMessage)
        return null
      }

      pendingActionRef.current = action
      try {
        const result = await withVerification(fetchFullCode, {
          preferredMethod: 'passkey',
          title: t('Verify to view redemption code'),
          description: t(
            'Use Passkey or 2FA to confirm your identity before revealing this redemption code.'
          ),
        })
        const key = extractKey(result)
        if (!key) return null

        pendingActionRef.current = null
        if (action === 'view') {
          setPopoverOpen(true)
        }
        return key
      } catch (error) {
        pendingActionRef.current = null
        const message =
          error instanceof Error
            ? error.message
            : t('Failed to fetch redemption code')
        toast.error(message)
        return null
      }
    },
    [
      copyToClipboard,
      fetchFullCode,
      fullCode,
      noPermissionMessage,
      permissions.canViewSecret,
      t,
      withVerification,
    ]
  )

  return (
    <>
      <div className='flex items-center'>
        <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
          <PopoverTrigger
            render={
              <Button
                variant='ghost'
                size='sm'
                className='h-7 font-mono'
                disabled={!permissions.canViewSecret}
                title={
                  permissions.canViewSecret
                    ? t('Reveal redemption code')
                    : noPermissionMessage
                }
                onClick={(event) => {
                  event.preventDefault()
                  void ensureFullCode('view')
                }}
              />
            }
          >
            {redemption.key}
          </PopoverTrigger>
          <PopoverContent
            className='w-auto max-w-[min(90vw,28rem)]'
            align='start'
          >
            <div className='flex flex-col gap-2'>
              <p className='text-muted-foreground text-xs'>{t('Full Code')}</p>
              <pre
                className='bg-muted/50 max-h-[50vh] overflow-auto rounded-md border px-3 py-2 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap'
                style={{ wordBreak: 'break-all' }}
              >
                {fullCode ?? redemption.key}
              </pre>
            </div>
          </PopoverContent>
        </Popover>
        <CopyButton
          resolveValue={() => ensureFullCode('copy')}
          disabled={!permissions.canViewSecret}
          className='size-7'
          iconClassName='size-3.5'
          tooltip={
            permissions.canViewSecret ? t('Copy code') : noPermissionMessage
          }
          successTooltip={t('Code copied!')}
          aria-label={t('Copy redemption code')}
        />
      </div>

      <SecureVerificationDialog
        open={verificationOpen}
        onOpenChange={(open) => {
          setVerificationOpen(open)
          if (!open) {
            pendingActionRef.current = null
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
