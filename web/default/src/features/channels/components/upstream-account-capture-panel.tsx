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
import { AlertCircle, ExternalLink, Loader2, RefreshCw } from 'lucide-react'
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

function isUnixExpired(value?: number) {
  return Boolean(
    value && value > 0 && Math.floor(Date.now() / 1000) >= value
  )
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

const CAPTURE_HELPER_READY_EVENT = 'nexustok-upstream-capture-helper-ready'
const CAPTURE_HELPER_PROBE_TIMEOUT_MS = 4500

type CaptureHelperDetection = 'idle' | 'probing' | 'ready' | 'missing' | 'outdated'

type CaptureHelperMessage = {
  type?: string
  captureID?: string
  capture_id?: string
  helperVersion?: string
  helper_version?: string
  target_origin?: string
  platform?: string
}

function openCaptureURL(
  url: string,
  targetWindow?: Window | null,
  preserveOpener = false
) {
  const trimmedURL = url.trim()
  if (!trimmedURL) return false
  try {
    if (targetWindow && !targetWindow.closed) {
      targetWindow.location.href = trimmedURL
      targetWindow.focus()
      return true
    }
  } catch {
    // 复用预打开窗口失败时，继续尝试创建新标签页，避免一次异常中断兜底链路。
  }
  try {
    const nextWindow = window.open(trimmedURL, '_blank')
    if (!nextWindow) return false
    if (!preserveOpener) {
      try {
        nextWindow.opener = null
      } catch {
        // 部分浏览器在跨域导航后不允许再触碰 opener，打开结果本身仍然有效。
      }
    }
    nextWindow.focus()
    return true
  } catch {
    return false
  }
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
  const { t, i18n } = useTranslation()
  const [localSession, setLocalSession] =
    useState<UpstreamAccountCaptureStartData | null>(null)
  const [helperVersion, setHelperVersion] = useState('')
  const [helperDetection, setHelperDetection] =
    useState<CaptureHelperDetection>('idle')
  const completedToastRef = useRef('')
  const pendingCaptureWindowRef = useRef<Window | null>(null)
  const activeCaptureWindowRef = useRef<Window | null>(null)
  const helperProbeTimerRef = useRef<number | null>(null)

  const helperInstalled = helperDetection === 'ready'

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
      helper_install_url:
        status?.helper_install_url || localSession?.helper_install_url || '',
      handoff_url: status?.handoff_url || localSession?.handoff_url || '',
      helper_version:
        status?.helper_version || localSession?.helper_version || '',
      helper_required_version:
        status?.helper_required_version ||
        localSession?.helper_required_version ||
        '',
      helper_status_message:
        status?.helper_status_message ||
        localSession?.helper_status_message ||
        '',
      login_url: status?.login_url || localSession?.login_url || baseUrl,
      return_url: status?.return_url || localSession?.return_url || '',
      locale: status?.locale || localSession?.locale || '',
      summary: status?.summary,
      diagnostics: status?.diagnostics,
      message: status?.message,
    }
  }, [baseUrl, captureId, localSession, platform, status])

  const navigateToHandoff = useCallback(
    (
      sessionData:
        | UpstreamAccountCaptureStartData
        | UpstreamAccountCaptureStatusData
        | null,
      targetWindow?: Window | null
    ) => {
      const handoffURL = sessionData?.handoff_url || sessionData?.login_url || sessionData?.base_url || baseUrl
      if (!handoffURL.trim()) {
        toast.error(t('Upstream platform URL is required'))
        return false
      }
      const opened = openCaptureURL(handoffURL, targetWindow, true)
      if (!opened) {
        toast.error(
          t(
            'Browser blocked the upstream capture tab. Allow pop-ups for NexusTok and try again.'
          )
        )
      }
      return opened
    },
    [baseUrl, t]
  )

  const clearHelperProbeTimer = useCallback(() => {
    if (helperProbeTimerRef.current) {
      window.clearTimeout(helperProbeTimerRef.current)
      helperProbeTimerRef.current = null
    }
  }, [])

  const openInstallerInCaptureWindow = useCallback(
    (
      sessionData:
        | UpstreamAccountCaptureStartData
        | UpstreamAccountCaptureStatusData
        | null,
      targetWindow?: Window | null
    ) => {
      const installURL =
        sessionData?.helper_install_url || sessionData?.userscript_url || ''
      if (!installURL.trim()) return false
      return openCaptureURL(
        installURL,
        targetWindow || activeCaptureWindowRef.current
      )
    },
    []
  )

  const beginHelperProbe = useCallback(
    (
      sessionData:
        | UpstreamAccountCaptureStartData
        | UpstreamAccountCaptureStatusData
        | null,
      targetWindow?: Window | null
    ) => {
      clearHelperProbeTimer()
      setHelperDetection('probing')
      setHelperVersion('')
      if (targetWindow && !targetWindow.closed) {
        activeCaptureWindowRef.current = targetWindow
      }
      const opened = navigateToHandoff(sessionData, targetWindow)
      helperProbeTimerRef.current = window.setTimeout(() => {
        setHelperDetection('missing')
        toast.info(
          t('Capture helper was not detected. Install it before continuing.')
        )
        openInstallerInCaptureWindow(sessionData, targetWindow)
      }, CAPTURE_HELPER_PROBE_TIMEOUT_MS)
      return opened
    },
    [clearHelperProbeTimer, navigateToHandoff, openInstallerInCaptureWindow, t]
  )

  const startMutation = useMutation({
    mutationFn: startUpstreamAccountCaptureSession,
    onSuccess: (res) => {
      if (!res.success || !res.data) {
        toast.error(res.message || t('Failed to create capture session'))
        return
      }
      setLocalSession(res.data)
      onCaptureIdChange(res.data.capture_id)
      completedToastRef.current = ''
      const targetWindow = pendingCaptureWindowRef.current
      pendingCaptureWindowRef.current = null
      beginHelperProbe(res.data, targetWindow)
      toast.success(t('Capture session created'))
    },
    onError: (error: unknown) => {
      if (
        pendingCaptureWindowRef.current &&
        !pendingCaptureWindowRef.current.closed
      ) {
        pendingCaptureWindowRef.current.close()
      }
      pendingCaptureWindowRef.current = null
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to create capture session')
      toast.error(message)
    },
  })

  useEffect(() => {
    const handleCaptureMessage = (event: MessageEvent) => {
      const activeWindow = activeCaptureWindowRef.current
      if (activeWindow && event.source && event.source !== activeWindow) {
        return
      }
      const data = event.data as CaptureHelperMessage | undefined
      if (!data || typeof data !== 'object') return
      const nextCaptureId = data.captureID || data.capture_id || ''
      if (!nextCaptureId) return
      const currentCaptureId = captureId || localSession?.capture_id || ''
      if (currentCaptureId && currentCaptureId !== nextCaptureId) return

      if (data.type === CAPTURE_HELPER_READY_EVENT) {
        clearHelperProbeTimer()
        const detectedVersion = String(
          data.helper_version || data.helperVersion || ''
        ).trim()
        const requiredVersion = String(
          session?.helper_required_version || session?.helper_version || ''
        ).trim()
        setHelperVersion(detectedVersion)
        if (detectedVersion && (!requiredVersion || detectedVersion === requiredVersion)) {
          setHelperDetection('ready')
          toast.success(t('Helper installed'))
          return
        }
        setHelperDetection('outdated')
        toast.info(t('Detected an outdated capture helper. Please update it.'))
        openInstallerInCaptureWindow(session, activeWindow)
        return
      }

      if (data.type !== 'nexustok-upstream-capture-completed') return
      clearHelperProbeTimer()
      setHelperDetection('ready')
      onCaptureIdChange(nextCaptureId)
      void statusQuery.refetch()
    }
    window.addEventListener('message', handleCaptureMessage)
    return () => {
      window.removeEventListener('message', handleCaptureMessage)
      clearHelperProbeTimer()
    }
  }, [
    captureId,
    clearHelperProbeTimer,
    localSession?.capture_id,
    onCaptureIdChange,
    openInstallerInCaptureWindow,
    session,
    statusQuery,
    t,
  ])

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
    try {
      pendingCaptureWindowRef.current = window.open('about:blank', '_blank')
      activeCaptureWindowRef.current = pendingCaptureWindowRef.current
    } catch {
      pendingCaptureWindowRef.current = null
      activeCaptureWindowRef.current = null
    }
    if (!pendingCaptureWindowRef.current) {
      toast.info(t('Browser blocked the upstream capture tab. Use the buttons below to continue.'))
    }
    startMutation.mutate({
      platform,
      base_url: trimmedBaseUrl,
      channel_id: channelId || undefined,
      return_url: returnUrl || window.location.href,
      locale: i18n.resolvedLanguage || i18n.language,
    })
  }, [
    baseUrl,
    channelId,
    disabled,
    i18n.language,
    i18n.resolvedLanguage,
    platform,
    returnUrl,
    startMutation,
    t,
  ])

  const handleOpenInstaller = useCallback(() => {
    const installURL = session?.helper_install_url || session?.userscript_url || ''
    if (!installURL) {
      toast.error(t('Signed userscript install link is not ready yet'))
      return
    }
    const opened = openCaptureURL(installURL, activeCaptureWindowRef.current)
    if (!opened) {
      toast.error(
        t(
          'Browser blocked the script installer tab. Allow pop-ups for NexusTok and try again.'
        )
      )
    }
  }, [session?.helper_install_url, session?.userscript_url, t])

  const handleOpenUpstreamSite = useCallback(() => {
    if (isUnixExpired(session?.expires_at)) {
      toast.info(t('Capture session expired. Creating a fresh session.'))
      handleStart()
      return
    }
    beginHelperProbe(session, activeCaptureWindowRef.current)
  }, [beginHelperProbe, handleStart, session, t])

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
  const showFallbackNotice = !isCompleted && Boolean(session?.capture_id)

  if (!session) return null

  return (
    <div className='flex flex-col gap-3 rounded-lg border p-3 sm:col-span-2 lg:col-span-6'>
      <div className='flex flex-col gap-3 rounded-md border p-3'>
        <div className='flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between'>
          <div className='min-w-0'>
            <div className='flex flex-wrap items-center gap-2'>
              <span className='text-sm font-medium'>
                {t('Capture Status')}
              </span>
              <Badge
                variant={
                  isCompleted ? 'default' : isFailed ? 'destructive' : 'outline'
                }
              >
                {isCompleted ? t('Completed') : isFailed ? t('Failed') : t('Pending')}
              </Badge>
              <Badge variant={helperInstalled ? 'default' : 'outline'}>
                {helperInstalled ? t('Helper installed') : t('Helper not installed')}
              </Badge>
              {helperVersion ? (
                <Badge variant='outline'>{helperVersion}</Badge>
              ) : null}
            </div>
            {!summary?.refresh_token_present && summary ? (
              <div className='text-muted-foreground mt-1 text-xs'>
                {t('This login can sync now; collect again after it expires.')}
              </div>
            ) : null}
          </div>
          <Button
            type='button'
            variant='outline'
            size='sm'
            className='sm:self-end'
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

        <div className='grid gap-2 text-xs md:grid-cols-5'>
          {[
            {
              label: t('Management URL'),
              value: managementBaseURL || '-',
              title: managementBaseURL,
            },
            {
              label: t('Model API URL'),
              value: relayBaseURL || '-',
              title: relayBaseURL,
            },
            {
              label: t('Token'),
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
              label: t('Expires At'),
              value: formatUnixTime(session.expires_at) || '-',
            },
          ].map((item) => (
            <div key={item.label} className='min-w-0'>
              <div className='text-muted-foreground truncate'>{item.label}</div>
              <div
                className='truncate font-medium'
                title={item.title || item.value}
              >
                {item.value}
              </div>
            </div>
          ))}
        </div>
      </div>

      {showFallbackNotice ? (
        <Alert variant={isFailed ? 'destructive' : 'default'}>
          <AlertCircle aria-hidden='true' />
          <AlertDescription className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
            <span>
              {isFailed
                ? status?.message || t('Capture failed. Refresh status or create a new session.')
                : helperDetection === 'outdated'
                  ? t('Detected an outdated capture helper. Please update it.')
                  : helperDetection === 'probing'
                    ? t('Checking capture helper version...')
                    : helperInstalled
                  ? t('Open the upstream platform in a new tab to continue capture.')
                  : t('Install the capture helper, then open the upstream platform to capture.')}
            </span>
            <span className='flex flex-wrap gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={!session.helper_install_url && !session.userscript_url}
                onClick={handleOpenInstaller}
              >
                <ExternalLink data-icon='inline-start' />
                {t('Install Capture Helper')}
              </Button>
              {!isCompleted ? (
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  disabled={!session.handoff_url && !session.login_url && !session.base_url}
                  onClick={handleOpenUpstreamSite}
                >
                  <ExternalLink data-icon='inline-start' />
                  {t('Continue Capture')}
                </Button>
              ) : null}
            </span>
          </AlertDescription>
        </Alert>
      ) : null}

      {status?.message && !showFallbackNotice ? (
        <Alert variant={isFailed ? 'destructive' : 'default'}>
          <AlertCircle aria-hidden='true' />
          <AlertDescription>{status.message}</AlertDescription>
        </Alert>
      ) : null}

      {status?.diagnostics ? (
        <details className='rounded-md border p-3 text-xs'>
          <summary className='cursor-pointer font-medium'>
            {t('Capture diagnostics')}
          </summary>
          <div className='mt-3 grid gap-2 sm:grid-cols-2 xl:grid-cols-3'>
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
              <div
                key={item.label}
                className='grid min-w-0 grid-cols-[8.5rem_minmax(0,1fr)] items-center gap-2'
              >
                <div className='text-muted-foreground truncate'>{item.label}</div>
                <div
                  className='truncate font-medium'
                  title={item.title || item.value}
                >
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
            <div
              className='text-muted-foreground mt-2 truncate'
              title={status.diagnostics.browser_session_restore_message}
            >
              {t('Session restore message')}: {status.diagnostics.browser_session_restore_message}
            </div>
          ) : null}
        </details>
      ) : null}
    </div>
  )
})
