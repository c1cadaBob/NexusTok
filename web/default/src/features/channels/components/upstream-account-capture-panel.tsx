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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  AlertCircle,
  CheckCircle2,
  Copy,
  ExternalLink,
  Loader2,
  RefreshCw,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
  onCaptureIdChange: (captureId: string) => void
}

function formatUnixTime(value?: number) {
  if (!value) return ''
  return new Date(value * 1000).toLocaleString()
}

function hasInstallLinks(
  value: UpstreamAccountCaptureStartData | UpstreamAccountCaptureStatusData | null
): value is UpstreamAccountCaptureStartData {
  return Boolean(value && 'userscript_url' in value)
}

export function UpstreamAccountCapturePanel({
  platform,
  baseUrl,
  channelId,
  disabled,
  captureId,
  onCaptureIdChange,
}: UpstreamAccountCapturePanelProps) {
  const { t } = useTranslation()
  const [localSession, setLocalSession] =
    useState<UpstreamAccountCaptureStartData | null>(null)
  const completedToastRef = useRef('')

  const statusQuery = useQuery({
    queryKey: ['upstream-account-capture-session', captureId],
    queryFn: () => getUpstreamAccountCaptureSession(captureId),
    enabled: Boolean(captureId),
    refetchInterval: (query) => {
      const data = query.state.data?.data
      return data?.status === 'pending' ? 3000 : false
    },
  })

  const status = statusQuery.data?.data
  const session = useMemo<
    UpstreamAccountCaptureStartData | UpstreamAccountCaptureStatusData | null
  >(() => status || localSession, [localSession, status])

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
      toast.success(t('Capture session created'))
    },
    onError: (error: unknown) => {
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
    toast.success(t('Login state captured. Preview the upstream account to verify it.'))
  }, [status, t])

  const handleStart = useCallback(() => {
    const trimmedBaseUrl = baseUrl.trim()
    if (!trimmedBaseUrl) {
      toast.error(t('Upstream platform URL is required'))
      return
    }
    startMutation.mutate({
      platform,
      base_url: trimmedBaseUrl,
      channel_id: channelId || undefined,
    })
  }, [baseUrl, channelId, platform, startMutation, t])

  const handleCopy = useCallback(async (value: string, label: string) => {
    try {
      await navigator.clipboard.writeText(value)
      toast.success(t('{{label}} copied', { label }))
    } catch {
      toast.error(t('Failed to copy {{label}}', { label }))
    }
  }, [t])

  const openURL = useCallback((value?: string) => {
    if (!value) return
    window.open(value, '_blank', 'noopener,noreferrer')
  }, [])

  const installURL = hasInstallLinks(session) ? session.userscript_url : undefined
  const loginURL = hasInstallLinks(session) ? session.login_url : baseUrl
  const isCompleted = status?.status === 'completed'
  const isFailed = status?.status === 'failed'
  const summary = status?.summary

  return (
    <div className='flex flex-col gap-3 rounded-lg border p-3 sm:col-span-2 lg:col-span-6'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between'>
        <div className='flex min-w-0 flex-col gap-1'>
          <div className='text-sm font-medium'>
            {t('Userscript login capture')}
          </div>
          <div className='text-muted-foreground text-xs'>
            {platform === 'new-api'
              ? t(
                  'The userscript runs inside the upstream new-api site, calls /api/user/self and /api/user/token with your logged-in browser session, then sends only the captured upstream token to NexusTok.'
                )
              : t(
                  'The userscript runs inside the upstream sub2api site and reads auth_token, refresh_token, and token_expires_at from localStorage or the OAuth callback hash.'
                )}
          </div>
        </div>
        <Button
          type='button'
          variant='outline'
          disabled={disabled || startMutation.isPending}
          onClick={handleStart}
        >
          {startMutation.isPending ? (
            <Loader2 data-icon='inline-start' className='animate-spin' />
          ) : (
            <RefreshCw data-icon='inline-start' />
          )}
          {session ? t('Create New Session') : t('Create Capture Session')}
        </Button>
      </div>

      {platform === 'new-api' ? (
        <Alert>
          <AlertCircle aria-hidden='true' />
          <AlertDescription>
            {t(
              'Generating a new-api access token may rotate the upstream user token. Use manual Cookie import if you do not want the upstream token to change.'
            )}
          </AlertDescription>
        </Alert>
      ) : (
        <Alert>
          <AlertCircle aria-hidden='true' />
          <AlertDescription>
            {t(
              'If sub2api access token expires, NexusTok will use the captured refresh token to refresh it during later syncs.'
            )}
          </AlertDescription>
        </Alert>
      )}

      {session ? (
        <div className='flex flex-col gap-3'>
          <div className='grid gap-2 text-xs sm:grid-cols-2 lg:grid-cols-4'>
            <div className='min-w-0 rounded-md border p-2'>
              <div className='text-muted-foreground'>{t('Status')}</div>
              <div className='truncate font-medium'>
                {isCompleted
                  ? t('Completed')
                  : isFailed
                    ? t('Failed')
                    : t('Pending')}
              </div>
            </div>
            <div className='min-w-0 rounded-md border p-2'>
              <div className='text-muted-foreground'>{t('Target Origin')}</div>
              <div className='truncate font-medium' title={session.origin}>
                {session.origin}
              </div>
            </div>
            <div className='min-w-0 rounded-md border p-2'>
              <div className='text-muted-foreground'>{t('Expires At')}</div>
              <div className='truncate font-medium'>
                {formatUnixTime(session.expires_at)}
              </div>
            </div>
            <div className='min-w-0 rounded-md border p-2'>
              <div className='text-muted-foreground'>{t('Captured Token')}</div>
              <div className='truncate font-medium'>
                {summary?.access_token_masked || '-'}
              </div>
            </div>
          </div>

          <div className='flex flex-col gap-2 sm:flex-row sm:flex-wrap'>
            <Button
              type='button'
              variant='outline'
              disabled={!installURL}
              onClick={() => openURL(installURL)}
            >
              <ExternalLink data-icon='inline-start' />
              {t('Install Userscript')}
            </Button>
            <Button
              type='button'
              variant='outline'
              disabled={!loginURL}
              onClick={() => openURL(loginURL)}
            >
              <ExternalLink data-icon='inline-start' />
              {t('Open Upstream Site')}
            </Button>
            <Button
              type='button'
              variant='outline'
              disabled={!installURL}
              onClick={() =>
                installURL &&
                void handleCopy(installURL, t('Userscript install link'))
              }
            >
              <Copy data-icon='inline-start' />
              {t('Copy Script Link')}
            </Button>
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

          {summary ? (
            <Alert>
              <CheckCircle2 aria-hidden='true' />
              <AlertDescription>
                {t(
                  'Captured {{platform}} login state for {{account}}. The token is stored only in the temporary capture session until you preview and save.',
                  {
                    platform: summary.platform,
                    account:
                      summary.username ||
                      summary.email ||
                      summary.user_id ||
                      t('the upstream account'),
                  }
                )}
              </AlertDescription>
            </Alert>
          ) : null}

          {status?.message ? (
            <Alert variant={isFailed ? 'destructive' : 'default'}>
              <AlertCircle aria-hidden='true' />
              <AlertDescription>{status.message}</AlertDescription>
            </Alert>
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
