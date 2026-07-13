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
import { Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  SettingsPageTitleStatusPortal,
  useSettingsPageTitleStatusContainer,
} from './settings-page-context'

type FormDirtyIndicatorProps = {
  isDirty: boolean
  message?: string
}

/**
 * Visual indicator that the form has unsaved changes
 *
 * @example
 * ```tsx
 * <FormDirtyIndicator isDirty={form.formState.isDirty} />
 * ```
 */
export function FormDirtyIndicator({
  isDirty,
  message,
}: FormDirtyIndicatorProps) {
  const { t } = useTranslation()
  const titleStatusContainer = useSettingsPageTitleStatusContainer()
  if (!isDirty) return null

  if (titleStatusContainer) {
    return (
      <SettingsPageTitleStatusPortal>
        <Badge variant='outline' className='text-muted-foreground'>
          <Info data-icon='inline-start' />
          <span>{message ? t(message) : t('Unsaved changes')}</span>
        </Badge>
      </SettingsPageTitleStatusPortal>
    )
  }

  return (
    <Alert variant='default'>
      <Info />
      <AlertDescription>
        {message ?? t('You have unsaved changes')}
      </AlertDescription>
    </Alert>
  )
}
