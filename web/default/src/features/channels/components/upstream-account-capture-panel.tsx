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
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { AlertCircle, Loader2, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  getUpstreamAccountCaptureSession,
  startUpstreamAccountCaptureSession,
} from '../api'
import type {
  UpstreamAccountCaptureStartData,
  UpstreamAccountCaptureStatusData,
  UpstreamAccountPlatform,
} from '../types'

type UpstreamAccountCapturePanelProps = {
  platform: UpstreamAccountPlatform
  baseUrl: string
  channelId?: number | null
  disabled?: boolean
  captureId: string
  returnUrl?: string
  onCaptureIdChange: (captureId: string) => void
  onCompleted?: (captureId: string) => void
}

export type UpstreamAccountCapturePanelHandle = {
  start: () => void
  refresh: () => void
}

function formatUnixTime(value?: number) {
  if (!value) return ''
  return new Date(value * 1000).toLocaleString()
}

function buildCompleteEndpoint(captureId: string) {
  if (!captureId) return ''
  return `/api/channel/upstream-account/capture-session/${captureId}/complete`
}

function formatBrowserSessionRestoreStatus(
  value: string | undefined,
  t: (key: string) => string
) {
  switch (value) {
    case 'authenticated':
      return t('Restored')
    case 'unauthenticated':
      return t('Unauthenticated')
    case 'failed':
      return t('Failed')
    case 'not_attempted':
    case '':
    case undefined:
      return t('Not attempted')
    default:
      return value
  }
}

function formatYesNo(value: boolean | undefined, t: (key: string) => string) {
  return value ? t('Yes') : t('No')
}

function compactKeys(
  value: string[] | undefined,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  const count = value?.length ?? 0
  if (count === 0) return t('None')
  return t('{{count}} key(s)', { count })
}

export const UpstreamAccountCapturePanel = forwardRef<
  UpstreamAccountCapturePanelHandle,
  UpstreamAccountCapturePanelProps
>(function UpstreamAccountCapturePanel(
  {
    platform,
    baseUrl,
    channelId,
    disabled,
    captureId,
    returnUrl,
    onCaptureIdChange,
    onCompleted,
  },
  ref
) {
  const { t } = useTranslation()
  const [localSession, setLocalSession] =
    useState<UpstreamAccountCaptureStartData | null>(null)
  const completedToastRef = useRef('')
  const pendingCaptureWindowsRef = useRef<{
    installWindow?: Window | null
    loginWindow?: Window | null
  }>({})

  const statusQuery = useQuery({
    queryKey: ['upstream-account-capture-session', captureId],
    queryFn: () => getUpstreamAccountCaptureSession(captureId),
    enabled: Boolean(captureId),
    refetchInterval: (query) => {
      const data = query.state.data?.data
      if (!data) return 3000
      return data.status === 'completed' ? false : 3000
    },
  })

  const status = statusQuery.data?.data
  const session = useMemo<
    UpstreamAccountCaptureStartData | UpstreamAccountCaptureStatusData | null
  >(() => {
    if (!status && !localSession) return null
    return {
      capture_id: status?.capture_id || localSession?.capture_id || captureId,
      status: status?.status || 'pending',
      expires_at: status?.expires_at || localSession?.expires_at || 0,
      platform: status?.platform || localSession?.platform || platform,
      base_url: status?.base_url || localSession?.base_url || baseUrl,
      management_base_url:
        status?.management_base_url ||
        localSession?.management_base_url ||
        status?.base_url ||
        localSession?.base_url ||
        baseUrl,
      relay_base_url:
        status?.relay_base_url || localSession?.relay_base_url || '',
      api_base_url: status?.api_base_url || localSession?.api_base_url || '',
      origin: status?.origin || localSession?.origin || '',
      userscript_url:
        status?.userscript_url || localSession?.userscript_url || '',
      login_url: status?.login_url || localSession?.login_url || baseUrl,
      return_url: status?.return_url || localSession?.return_url || '',
      summary: status?.summary,
      diagnostics: status?.diagnostics,
      message: status?.message,
    }
  }, [baseUrl, captureId, localSession, platform, status])

  const openCaptureTargets = useCallback(
    (
      sessionData: UpstreamAccountCaptureStartData,
      installWindow?: Window | null,
      loginWindow?: Window | null
    ) => {
      const installURL = sessionData.userscript_url
      const loginURL = sessionData.login_url || sessionData.base_url || baseUrl
      if (installURL) {
        if (installWindow && !installWindow.closed) {
          installWindow.location.href = installURL
        } else {
          window.open(installURL, '_blank', 'noopener,noreferrer')
        }
      }
      if (loginURL) {
        window.setTimeout(() => {
          if (loginWindow && !loginWindow.closed) {
            loginWindow.location.href = loginURL
          } else {
            window.open(loginURL, '_blank', 'noopener,noreferrer')
          }
        }, 1200)
      }
    },
    [baseUrl]
  )

  const startMutation = useMutation({
    mutationFn: startUpstreamAccountCaptureSession,
    onSuccess: (res) => {
      const windows = pendingCaptureWindowsRef.current
      pendingCaptureWindowsRef.current = {}
      if (!res.success || !res.data) {
        toast.error(res.message || t('Failed to create capture session'))
        windows.installWindow?.close()
        windows.loginWindow?.close()
        return
      }
      setLocalSession(res.data)
      onCaptureIdChange(res.data.capture_id)
      completedToastRef.current = ''
      openCaptureTargets(res.data, windows.installWindow, windows.loginWindow)
      toast.success(t('Capture session created'))
    },
    onError: (error: unknown) => {
      const windows = pendingCaptureWindowsRef.current
      pendingCaptureWindowsRef.current = {}
      windows.installWindow?.close()
      windows.loginWindow?.close()
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to create capture session')
      toast.error(message)
    },
  })

  useEffect(() => {
    if (!status || status.status !== 'completed') return
    if (completedToastRef.current === status.capture_id) return
    completedToastRef.current = status.capture_id
    toast.success(
      t('Login state captured. Previewing upstream account automatically.')
    )
    onCompleted?.(status.capture_id)
  }, [onCompleted, status, t])

  const handleStart = useCallback(() => {
    if (disabled || startMutation.isPending) return
    const trimmedBaseUrl = baseUrl.trim()
    if (!trimmedBaseUrl) {
      toast.error(t('Upstream platform URL is required'))
      return
    }
    let installWindow: Window | null = null
    let loginWindow: Window | null = null
    try {
      installWindow = window.open('about:blank', '_blank')
      loginWindow = window.open('about:blank', '_blank')
      if (installWindow) installWindow.opener = null
      if (loginWindow) loginWindow.opener = null
    } catch {
      installWindow = null
      loginWindow = null
    }
    pendingCaptureWindowsRef.current = { installWindow, loginWindow }
    startMutation.mutate({
      platform,
      base_url: trimmedBaseUrl,
      channel_id: channelId || undefined,
      return_url: returnUrl || window.location.href,
    })
  }, [baseUrl, channelId, disabled, platform, returnUrl, startMutation, t])

  useImperativeHandle(
    ref,
    () => ({
      start: handleStart,
      refresh: () => {
        if (captureId) void statusQuery.refetch()
      },
    }),
    [captureId, handleStart, statusQuery]
  )

  const callbackEndpoint = buildCompleteEndpoint(captureId)
  const isCompleted = status?.status === 'completed'
  const isFailed = status?.status === 'failed'
  const summary = status?.summary
  const managementBaseURL =
    summary?.management_base_url ||
    status?.management_base_url ||
    session?.management_base_url ||
    session?.base_url ||
    ''
  const relayBaseURL =
    summary?.relay_base_url ||
    status?.relay_base_url ||
    session?.relay_base_url ||
    summary?.api_base_url ||
    status?.api_base_url ||
    session?.api_base_url ||
    status?.diagnostics?.api_base_url_seen ||
    ''
  if (!session) return null

  return (
    <div className='flex flex-col gap-3 rounded-lg border p-3 sm:col-span-2 lg:col-span-6'>
      <div className='grid gap-2 text-xs sm:grid-cols-2 xl:grid-cols-4'>
        {[
          {
            label: t('Status'),
            value: isCompleted
              ? t('Completed')
              : isFailed
                ? t('Failed')
                : t('Pending'),
          },
          {
            label: t('Target Origin'),
            value: session.origin || '-',
            title: session.origin,
          },
          {
            label: t('Management Panel URL'),
            value: managementBaseURL || '-',
            title: managementBaseURL,
          },
          {
            label: t('Model API URL'),
            value: relayBaseURL || '-',
            title: relayBaseURL,
          },
          {
            label: t('Expires At'),
            value: formatUnixTime(session.expires_at) || '-',
          },
          {
            label: t('Captured Token'),
            value: summary?.access_token_masked || '-',
          },
          {
            label: t('Refresh Token'),
            value: summary
              ? summary.refresh_token_present
                ? t('Present')
                : t('Not provided')
              : '-',
          },
          {
            label: t('Callback Endpoint'),
            value: callbackEndpoint || '-',
            title: callbackEndpoint,
          },
        ].map((item) => (
          <div key={item.label} className='min-w-0 rounded-md border p-2'>
            <div className='text-muted-foreground'>{item.label}</div>
            <div className='truncate font-medium' title={item.title || item.value}>
              {item.value}
            </div>
          </div>
        ))}
      </div>

      <div className='flex flex-col gap-2 sm:flex-row sm:flex-wrap'>
        <Button
          type='button'
          variant='outline'
          disabled={!captureId || statusQuery.isFetching}
          onClick={() => void statusQuery.refetch()}
        >
          {statusQuery.isFetching ? (
            <Loader2 data-icon='inline-start' className='animate-spin' />
          ) : (
            <RefreshCw data-icon='inline-start' />
          )}
          {t('Refresh Capture Status')}
        </Button>
      </div>

      {status?.message ? (
        <Alert variant={isFailed ? 'destructive' : 'default'}>
          <AlertCircle aria-hidden='true' />
          <AlertDescription>{status.message}</AlertDescription>
        </Alert>
      ) : null}

      {status?.diagnostics ? (
        <div className='flex flex-col gap-2 rounded-md border p-3 text-xs'>
          <div className='font-medium'>{t('Capture diagnostics')}</div>
          <div className='grid gap-2 sm:grid-cols-2 xl:grid-cols-4'>
            {[
              {
                label: t('Page origin'),
                value: status.diagnostics.page_origin || '-',
              },
              {
                label: t('Detected model API URL'),
                value: status.diagnostics.api_base_url_seen || '-',
              },
              {
                label: t('Session restore endpoint'),
                value: status.diagnostics.browser_session_restore_path || '-',
              },
              {
                label: t('Auth validation endpoint'),
                value: status.diagnostics.auth_me_path || '-',
              },
              {
                label: 'auth_token',
                value: formatYesNo(status.diagnostics.auth_token_present, t),
                badge: true,
              },
              {
                label: 'access_token',
                value: formatYesNo(status.diagnostics.access_token_present, t),
                badge: true,
              },
              {
                label: 'refresh_token',
                value: formatYesNo(status.diagnostics.refresh_token_present, t),
                badge: true,
              },
              {
                label: t('Auth Client ID'),
                value: formatYesNo(status.diagnostics.auth_client_id_present, t),
                badge: true,
              },
              {
                label: t('Browser session restore'),
                value: formatBrowserSessionRestoreStatus(
                  status.diagnostics.browser_session_restore_status,
                  t
                ),
              },
              {
                label: t('OAuth hash token'),
                value: formatYesNo(
                  status.diagnostics.oauth_hash_token_present,
                  t
                ),
                badge: true,
              },
              {
                label: t('localStorage keys'),
                value: compactKeys(status.diagnostics.local_storage_keys, t),
                title: status.diagnostics.local_storage_keys?.join(', ') || '',
              },
              {
                label: t('sessionStorage keys'),
                value: compactKeys(status.diagnostics.session_storage_keys, t),
                title:
                  status.diagnostics.session_storage_keys?.join(', ') || '',
              },
            ].map((item) => (
              <div key={item.label} className='min-w-0 rounded-md border p-2'>
                <div className='text-muted-foreground truncate'>
                  {item.label}
                </div>
                <div className='truncate font-medium' title={item.title || item.value}>
                  {item.badge ? (
                    <Badge variant='outline'>{item.value}</Badge>
                  ) : (
                    item.value
                  )}
                </div>
              </div>
            ))}
          </div>
          {status.diagnostics.browser_session_restore_message ? (
            <div className='text-muted-foreground truncate' title={status.diagnostics.browser_session_restore_message}>
              {t('Session restore message')}: {status.diagnostics.browser_session_restore_message}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  )
})
