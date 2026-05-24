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
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useCallback, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Main } from '@/components/layout/components/main'
import { useAuthStore } from '@/stores/auth-store'
import { useThemeCustomization } from '@/context/theme-customization-provider'
import { useTheme } from '@/context/theme-provider'
import { ROLE } from '@/lib/roles'

const ACCOUNT_POOL_MANAGER_URL = '/account-pool/manager/?embeddedFrame=true'
const ACCOUNT_POOL_PREFERENCES_EVENT = 'nexustok:account-pool-preferences'
const ACCOUNT_POOL_READY_EVENT = 'nexustok:account-pool-ready'
const PREFERENCE_SYNC_RETRY_DELAYS = [0, 100, 300, 700, 1500, 3000, 5000]

export const Route = createFileRoute('/_authenticated/account-pool/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  component: AccountPoolFrame,
})

function AccountPoolFrame() {
  const { i18n, t } = useTranslation()
  const iframeRef = useRef<HTMLIFrameElement | null>(null)
  const { resolvedTheme } = useTheme()
  const { customization } = useThemeCustomization()

  const syncPreferencesToFrame = useCallback(() => {
    const target = iframeRef.current?.contentWindow
    if (!target) return

    target.postMessage(
      {
        type: ACCOUNT_POOL_PREFERENCES_EVENT,
        language: i18n.language,
        resolvedTheme,
        themePreset: customization.preset,
      },
      window.location.origin
    )
  }, [customization.preset, i18n.language, resolvedTheme])

  const schedulePreferenceSync = useCallback(() => {
    const timers = PREFERENCE_SYNC_RETRY_DELAYS.map((delay) =>
      window.setTimeout(syncPreferencesToFrame, delay)
    )

    return () => timers.forEach((timer) => window.clearTimeout(timer))
  }, [syncPreferencesToFrame])

  useEffect(() => {
    return schedulePreferenceSync()
  }, [schedulePreferenceSync])

  useEffect(() => {
    const handleLanguageChanged = () => schedulePreferenceSync()
    i18n.on('languageChanged', handleLanguageChanged)
    return () => i18n.off('languageChanged', handleLanguageChanged)
  }, [i18n, schedulePreferenceSync])

  useEffect(() => {
    const handleFrameMessage = (event: MessageEvent) => {
      if (event.origin !== window.location.origin) return
      if (event.source !== iframeRef.current?.contentWindow) return
      if (event.data?.type !== ACCOUNT_POOL_READY_EVENT) return
      schedulePreferenceSync()
    }

    window.addEventListener('message', handleFrameMessage)
    return () => window.removeEventListener('message', handleFrameMessage)
  }, [schedulePreferenceSync])

  return (
    <Main className='h-full bg-background p-0'>
      <iframe
        ref={iframeRef}
        title={t('Account Pool Management')}
        src={ACCOUNT_POOL_MANAGER_URL}
        className='block h-full min-h-0 w-full flex-1 border-0 bg-background'
        allow='clipboard-read; clipboard-write'
        onLoad={schedulePreferenceSync}
      />
    </Main>
  )
}
