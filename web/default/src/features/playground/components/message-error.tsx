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
import { AlertCircle, AlertTriangle, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  FALLBACK_ERROR_CONTENT,
  getMessageErrorState,
  isAdminRole,
  MODEL_PRICING_SETTINGS_PATH,
} from '../lib'
import type { Message } from '../types'

interface MessageErrorProps {
  message: Message
  className?: string
}

/**
 * 使用 Alert 展示 Playground 消息级错误。
 */
export function MessageError({ message, className = '' }: MessageErrorProps) {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.auth.user)
  const errorState = getMessageErrorState(message, isAdminRole(user?.role))

  if (!errorState) {
    return null
  }

  const errorContent =
    errorState.content === FALLBACK_ERROR_CONTENT
      ? t(FALLBACK_ERROR_CONTENT)
      : errorState.content

  if (errorState.kind === 'model-price') {
    return (
      <Alert variant='default' className={className}>
        <AlertTriangle className='text-warning' />
        <AlertTitle>{t('Model Price Not Configured')}</AlertTitle>
        <AlertDescription className='flex flex-col gap-2'>
          <p>{errorContent}</p>
          {errorState.showSettingsLink && (
            <Button
              variant='outline'
              size='sm'
              onClick={() =>
                window.open(MODEL_PRICING_SETTINGS_PATH, '_blank')
              }
            >
              <Settings data-icon='inline-start' />
              {t('Go to Settings')}
            </Button>
          )}
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <Alert variant='destructive' className={className}>
      <AlertCircle />
      <AlertTitle>{t('Error')}</AlertTitle>
      <AlertDescription>{errorContent}</AlertDescription>
    </Alert>
  )
}
