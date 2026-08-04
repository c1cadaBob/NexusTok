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
  Code2,
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  getUpstreamAccountCaptureSession,
  getUpstreamAccountCaptureUserscript,
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

function buildCompleteEndpoint(captureId: string) {
  if (!captureId) return ''
  return `/api/channel/upstream-account/capture-session/${captureId}/complete`
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
  const [scriptDialogOpen, setScriptDialogOpen] = useState(false)
  const [userscriptSource, setUserscriptSource] = useState('')
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
  >(() => {
    if (!status && !localSession) return null
    return {
      capture_id: status?.capture_id || localSession?.capture_id || captureId,
      status: status?.status || 'pending',
      expires_at: status?.expires_at || localSession?.expires_at || 0,
      platform: status?.platform || localSession?.platform || platform,
      base_url: status?.base_url || localSession?.base_url || baseUrl,
      origin: status?.origin || localSession?.origin || '',
      userscript_url:
        status?.userscript_url || localSession?.userscript_url || '',
      login_url: status?.login_url || localSession?.login_url || baseUrl,
      summary: status?.summary,
      message: status?.message,
    }
  }, [baseUrl, captureId, localSession, platform, status])

  const startMutation = useMutation({
    mutationFn: startUpstreamAccountCaptureSession,
    onSuccess: (res) => {
      if (!res.success || !res.data) {
        toast.error(res.message || t('Failed to create capture session'))
        return
      }
      setLocalSession(res.data)
      setUserscriptSource('')
      setScriptDialogOpen(false)
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
    toast.success(
      t('Login state captured. Click Sync Keys to validate, preview, and save it.')
    )
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

  const installURL = session?.userscript_url || ''
  const loginURL = session?.login_url || baseUrl
  const callbackEndpoint = buildCompleteEndpoint(captureId)
  const isCompleted = status?.status === 'completed'
  const isFailed = status?.status === 'failed'
  const summary = status?.summary

  const scriptMutation = useMutation({
    mutationFn: getUpstreamAccountCaptureUserscript,
  })

  const loadUserscriptSource = useCallback(async () => {
    if (!installURL) {
      throw new Error(t('Signed userscript install link is not ready yet'))
    }
    if (userscriptSource) return userscriptSource
    const script = await scriptMutation.mutateAsync(installURL)
    setUserscriptSource(script)
    return script
  }, [installURL, scriptMutation, t, userscriptSource])

  const handleViewScript = useCallback(async () => {
    try {
      await loadUserscriptSource()
      setScriptDialogOpen(true)
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Failed to load userscript source')
      toast.error(message)
    }
  }, [loadUserscriptSource, t])

  const handleCopyFullScript = useCallback(async () => {
    try {
      const script = await loadUserscriptSource()
      await navigator.clipboard.writeText(script)
      toast.success(t('Userscript source copied'))
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Failed to copy userscript source')
      toast.error(message)
    }
  }, [loadUserscriptSource, t])

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
                  'The userscript runs inside the upstream new-api site, reads the user ID from localStorage when possible, calls /api/user/self and /api/user/token with your logged-in browser session, then sends only the captured upstream token to NexusTok.'
                )
              : t(
                  'The userscript runs inside the upstream sub2api site, reads auth_token, refresh_token, and token_expires_at from localStorage or the OAuth callback hash, then checks /api/v1/auth/me before sending the login state.'
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

      <Alert>
        <AlertCircle aria-hidden='true' />
        <AlertDescription>
          {t(
            'Capture flow: create a session, install the userscript, open and log in to the upstream site, click Send login to NexusTok on that site, then return here to preview and save.'
          )}
        </AlertDescription>
      </Alert>

      <Alert>
        <AlertCircle aria-hidden='true' />
        <AlertDescription>
          {t(
            'The userscript install link is signed and expires with this capture session. If it expires, create a new capture session.'
          )}
        </AlertDescription>
      </Alert>

      <Alert>
        <AlertCircle aria-hidden='true' />
        <AlertDescription>
          {t(
            'In multi-node deployments, enable Redis or sticky routing so the userscript callback and status polling can read the same capture session.'
          )}
        </AlertDescription>
      </Alert>

      {session ? (
        <div className='flex flex-col gap-3'>
          <div className='grid gap-2 text-xs sm:grid-cols-2 lg:grid-cols-5'>
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
            <div className='min-w-0 rounded-md border p-2'>
              <div className='text-muted-foreground'>{t('Callback Endpoint')}</div>
              <div className='truncate font-medium' title={callbackEndpoint}>
                {callbackEndpoint || '-'}
              </div>
            </div>
          </div>

          {callbackEndpoint ? (
            <div className='text-muted-foreground text-xs'>
              {t(
                'This script posts captured login state to {{callback}}. It only writes to the temporary capture session; use Sync Keys to preview and save it.',
                { callback: callbackEndpoint }
              )}
            </div>
          ) : null}

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
              disabled={!installURL || scriptMutation.isPending}
              onClick={() => void handleViewScript()}
            >
              {scriptMutation.isPending ? (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              ) : (
                <Code2 data-icon='inline-start' />
              )}
              {t('View Script Source')}
            </Button>
            <Button
              type='button'
              variant='outline'
              disabled={!installURL || scriptMutation.isPending}
              onClick={() => void handleCopyFullScript()}
            >
              {scriptMutation.isPending ? (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              ) : (
                <Copy data-icon='inline-start' />
              )}
              {t('Copy Full Script')}
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
                  'Captured {{platform}} login state for {{account}}. The token is stored only in the temporary capture session until you click Sync Keys to validate, preview, and save.',
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

          {status?.diagnostics ? (
            <Alert>
              <AlertCircle aria-hidden='true' />
              <AlertDescription>
                <div className='space-y-1 text-xs'>
                  <div className='font-medium'>{t('Capture diagnostics')}</div>
                  <div>
                    {t('Page origin')}: {status.diagnostics.page_origin || '-'}
                  </div>
                  <div>
                    {t('localStorage keys')}:{' '}
                    {status.diagnostics.local_storage_keys?.join(', ') || '-'}
                  </div>
                  <div>
                    {t('sessionStorage keys')}:{' '}
                    {status.diagnostics.session_storage_keys?.join(', ') || '-'}
                  </div>
                  <div className='grid gap-x-4 gap-y-1 sm:grid-cols-2'>
                    <span>
                      auth_token: {status.diagnostics.auth_token_present ? t('Yes') : t('No')}
                    </span>
                    <span>
                      access_token: {status.diagnostics.access_token_present ? t('Yes') : t('No')}
                    </span>
                    <span>
                      refresh_token: {status.diagnostics.refresh_token_present ? t('Yes') : t('No')}
                    </span>
                    <span>
                      {t('OAuth hash token')}: {status.diagnostics.oauth_hash_token_present ? t('Yes') : t('No')}
                    </span>
                  </div>
                  {status.diagnostics.auth_me_path ? (
                    <div>
                      {t('Auth validation endpoint')}: {status.diagnostics.auth_me_path}
                    </div>
                  ) : null}
                </div>
              </AlertDescription>
            </Alert>
          ) : null}
        </div>
      ) : null}

      <Dialog open={scriptDialogOpen} onOpenChange={setScriptDialogOpen}>
        <DialogContent className='flex max-h-[85vh] flex-col gap-0 p-0 sm:max-w-4xl'>
          <DialogHeader className='border-b px-6 py-4'>
            <DialogTitle>{t('Userscript source')}</DialogTitle>
            <DialogDescription>
              {t(
                'Read-only userscript generated for this capture session. Install it in Tampermonkey or copy it manually.'
              )}
            </DialogDescription>
          </DialogHeader>
          <ScrollArea className='min-h-0 flex-1'>
            <pre className='text-foreground bg-muted/40 m-0 overflow-x-auto p-4 text-xs leading-relaxed'>
              <code>{userscriptSource}</code>
            </pre>
          </ScrollArea>
          <DialogFooter className='border-t px-6 py-4'>
            <Button
              type='button'
              variant='outline'
              disabled={!userscriptSource}
              onClick={() => void handleCopyFullScript()}
            >
              <Copy data-icon='inline-start' />
              {t('Copy Full Script')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
