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
  ExternalLink,
  Loader2,
  RefreshCw,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { MultiSelect } from '@/components/multi-select'
import { ModelCatalogMultiSelect } from '@/features/models/components/model-catalog-multi-select'
import {
  completeUpstreamAccountPreview2FA,
  getChannelAccounts,
  getGroups,
  previewUpstreamAccount,
  refreshUpstreamAccountChannel,
} from '../api'
import {
  channelsQueryKeys,
  dedupeModelNames,
  formatGroups,
  formatModelsArray,
  parseGroups,
} from '../lib'
import {
  buildUpstreamAccountConfigsFromChannelAccounts,
  buildUpstreamAccountConfigsFromSnapshotKeys,
  buildUpstreamAccountConfigDraft,
  buildUpstreamAccountPreviewRequest,
  buildUpstreamAccountRefreshPayload,
  buildUpstreamRatioConversionPayload,
  collectUpstreamAccountCapabilityValidationErrors,
  DEFAULT_UPSTREAM_PAID_AMOUNT,
  DEFAULT_UPSTREAM_PLATFORM_CREDIT,
  formatUpstreamModelRatioDetails,
  formatUpstreamPreviewRemaining,
  formatUpstreamRatioCompact,
  getUpstreamSyncCredentialAuthModeFromSettings,
  getUpstreamKeyGroupLabel,
  getUpstreamKeyRatioDisplayValue,
  getUpstreamPreviewChallenge,
  getUpstreamRatioDisplayValue,
  getUpstreamSyncBaseUrlFromSettings,
  getUpstreamSyncPlatformFromSettings,
  hasUpstreamPreviewSnapshot,
  hasUpstreamSyncSavedCredential,
  isUpstreamPreviewExpiredError,
  normalizeUpstreamChannelBaseUrl,
  summarizeUpstreamAccountCapabilities,
  upstreamAccountKeyConfigId,
  upstreamAccountModelsArrayValue,
  upstreamPlatformFromChannelType,
  upstreamPreviewRemainingSeconds,
  type UpstreamAccountConfigDraft,
} from '../lib/upstream-sync'
import type {
  UpstreamAccountPlatform,
  UpstreamAccountAuthMode,
  UpstreamAccountPreviewData,
  UpstreamAccountSnapshot,
  UpstreamAccountTwoFactorChallenge,
} from '../types'
import {
  UpstreamAccountCapturePanel,
  type UpstreamAccountCapturePanelHandle,
} from './upstream-account-capture-panel'

type UpstreamAccountRefreshPanelProps = {
  open: boolean
  channelId: number | null
  channelType?: number | null
  channelBaseUrl?: string | null
  channelModels?: string
  channelSettings?: string
  canReadChannelAccount: boolean
  canSensitiveWrite: boolean
  noPermissionMessage: string
  onSuccess: () => Promise<void> | void
  onBusyChange?: (busy: boolean) => void
}

function mergeGroupOptionsWithSelected(
  options: Array<{ value: string; label: string }>,
  selected: string[]
) {
  const seen = new Set<string>()
  const merged: Array<{ value: string; label: string }> = []
  for (const option of options) {
    if (!option.value || seen.has(option.value)) continue
    seen.add(option.value)
    merged.push(option)
  }
  for (const group of ['default', ...selected]) {
    const value = group.trim()
    if (!value || seen.has(value)) continue
    seen.add(value)
    merged.push({ value, label: value })
  }
  return merged
}

export function UpstreamAccountRefreshPanel({
  open,
  channelId,
  channelType,
  channelBaseUrl,
  channelSettings,
  canReadChannelAccount,
  canSensitiveWrite,
  noPermissionMessage,
  onSuccess,
  onBusyChange,
}: UpstreamAccountRefreshPanelProps) {
  const { t } = useTranslation()
  const savedUpstreamCredentialAvailable = useMemo(
    () => hasUpstreamSyncSavedCredential(channelSettings),
    [channelSettings]
  )
  const savedUpstreamCredentialAuthMode = useMemo(
    () => getUpstreamSyncCredentialAuthModeFromSettings(channelSettings),
    [channelSettings]
  )
  const forcedUpstreamPlatform = useMemo(
    () => upstreamPlatformFromChannelType(channelType ?? 0),
    [channelType]
  )
  const captureReturnUrl = useMemo(() => {
    if (typeof window === 'undefined') return ''
    const url = new URL(window.location.href)
    url.searchParams.set('upstream_capture_mode', 'refresh')
    if (forcedUpstreamPlatform) {
      url.searchParams.set('upstream_capture_platform', forcedUpstreamPlatform)
    }
    if (channelId) {
      url.searchParams.set('upstream_capture_channel_id', String(channelId))
    }
    const managementBaseUrl =
      getUpstreamSyncBaseUrlFromSettings(channelSettings) ||
      normalizeUpstreamChannelBaseUrl(channelBaseUrl) ||
      ''
    if (managementBaseUrl) {
      url.searchParams.set('upstream_capture_base_url', managementBaseUrl)
    }
    url.searchParams.delete('upstream_capture_id')
    return url.toString()
  }, [channelBaseUrl, channelId, channelSettings, forcedUpstreamPlatform])
  const [upstreamPlatform, setUpstreamPlatform] =
    useState<UpstreamAccountPlatform>('new-api')
  const [upstreamBaseUrl, setUpstreamBaseUrl] = useState('')
  const [upstreamUsername, setUpstreamUsername] = useState('')
  const [upstreamPassword, setUpstreamPassword] = useState('')
  const [upstreamAuthMode, setUpstreamAuthMode] =
    useState<UpstreamAccountAuthMode>('oauth_browser')
  const [upstreamSessionCookie, setUpstreamSessionCookie] = useState('')
  const [upstreamUserId, setUpstreamUserId] = useState('')
  const [upstreamAccessToken, setUpstreamAccessToken] = useState('')
  const [upstreamRefreshToken, setUpstreamRefreshToken] = useState('')
  const [upstreamTokenExpiresAt, setUpstreamTokenExpiresAt] = useState('')
  const [upstreamCaptureId, setUpstreamCaptureId] = useState('')
  const [upstreamUseSavedCredential, setUpstreamUseSavedCredential] =
    useState(false)
  const [upstreamPaidCny, setUpstreamPaidCny] = useState(
    DEFAULT_UPSTREAM_PAID_AMOUNT
  )
  const [upstreamPlatformUsdCredit, setUpstreamPlatformUsdCredit] = useState(
    DEFAULT_UPSTREAM_PLATFORM_CREDIT
  )
  const [upstreamRefreshPreviewId, setUpstreamRefreshPreviewId] = useState('')
  const [upstreamRefreshPreviewExpiresAt, setUpstreamRefreshPreviewExpiresAt] =
    useState(0)
  const [upstreamRefreshSnapshot, setUpstreamRefreshSnapshot] =
    useState<UpstreamAccountSnapshot | null>(null)
  const [
    upstreamRefreshTwoFactorChallenge,
    setUpstreamRefreshTwoFactorChallenge,
  ] = useState<UpstreamAccountTwoFactorChallenge | null>(null)
  const [upstreamRefreshTwoFactorCode, setUpstreamRefreshTwoFactorCode] =
    useState('')
  const [upstreamPreviewNowMs, setUpstreamPreviewNowMs] = useState(() =>
    Date.now()
  )
  const [upstreamApplySuggested, setUpstreamApplySuggested] = useState(true)
  const [upstreamAccountConfigs, setUpstreamAccountConfigs] = useState<
    Record<string, UpstreamAccountConfigDraft>
  >({})
  const autoPreviewTriggeredRef = useRef(false)
  const capturePanelRef = useRef<UpstreamAccountCapturePanelHandle>(null)
  const ratioConfigLoadedRef = useRef(false)

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: getGroups,
    enabled: open,
  })

  const refreshAccountsQuery = useQuery({
    queryKey: [
      ...channelsQueryKeys.detail(channelId || 0),
      'upstream-refresh-accounts',
    ],
    queryFn: () =>
      getChannelAccounts(channelId!, {
        p: 1,
        page_size: 10000,
      }),
    enabled: open && Boolean(channelId) && canReadChannelAccount,
  })

  const refreshAccounts = useMemo(
    () => refreshAccountsQuery.data?.data?.accounts.items ?? [],
    [refreshAccountsQuery.data?.data?.accounts.items]
  )
  const refreshAccountsTotal =
    refreshAccountsQuery.data?.data?.accounts.total ?? 0
  const refreshAccountsLoadedCount = refreshAccounts.length
  const groupOptions = useMemo(
    () =>
      (groupsData?.data ?? []).map((group) => ({
        value: group,
        label: group,
      })),
    [groupsData?.data]
  )
  const showUpstreamCapabilityValidationError = useCallback(
    (
      errors: ReturnType<
        typeof collectUpstreamAccountCapabilityValidationErrors
      >
    ) => {
      const firstError = errors[0]
      if (!firstError) return false
      toast.error(
        t(
          firstError.field === 'models'
            ? 'Enabled synced key "{{name}}" must select at least one model.'
            : 'Enabled synced key "{{name}}" must select at least one NexusTok access group.',
          {
            name: firstError.keyName,
          }
        )
      )
      return true
    },
    [t]
  )
  const upstreamRefreshPreviewRemaining = upstreamPreviewRemainingSeconds(
    upstreamRefreshPreviewExpiresAt,
    upstreamPreviewNowMs
  )
  const isUpstreamRefreshPreviewExpired = Boolean(
    upstreamRefreshSnapshot &&
    upstreamRefreshPreviewExpiresAt &&
    upstreamRefreshPreviewRemaining <= 0
  )
  const upstreamRefreshTwoFactorRemaining = upstreamPreviewRemainingSeconds(
    upstreamRefreshTwoFactorChallenge?.expires_at ?? 0,
    upstreamPreviewNowMs
  )
  const isUpstreamRefreshTwoFactorExpired = Boolean(
    upstreamRefreshTwoFactorChallenge && upstreamRefreshTwoFactorRemaining <= 0
  )

  const resetRefreshState = useCallback(() => {
    const nextPlatform =
      getUpstreamSyncPlatformFromSettings(channelSettings) ||
      forcedUpstreamPlatform ||
      'new-api'
    setUpstreamPlatform(nextPlatform)
    setUpstreamBaseUrl(
      getUpstreamSyncBaseUrlFromSettings(channelSettings) ||
        normalizeUpstreamChannelBaseUrl(channelBaseUrl) ||
        ''
    )
    setUpstreamUsername('')
    setUpstreamPassword('')
    setUpstreamAuthMode('oauth_browser')
    setUpstreamSessionCookie('')
    setUpstreamUserId('')
    setUpstreamAccessToken('')
    setUpstreamRefreshToken('')
    setUpstreamTokenExpiresAt('')
    setUpstreamCaptureId('')
    setUpstreamUseSavedCredential(savedUpstreamCredentialAvailable)
    setUpstreamPaidCny(DEFAULT_UPSTREAM_PAID_AMOUNT)
    setUpstreamPlatformUsdCredit(DEFAULT_UPSTREAM_PLATFORM_CREDIT)
    setUpstreamRefreshPreviewId('')
    setUpstreamRefreshPreviewExpiresAt(0)
    setUpstreamRefreshSnapshot(null)
    setUpstreamRefreshTwoFactorChallenge(null)
    setUpstreamRefreshTwoFactorCode('')
    // 刷新已有渠道时默认保留本地密钥优先级/权重，管理员可通过开关主动应用
    // 上游建议值；这样普通刷新不会覆盖账号池中的手工调度配置。
    setUpstreamApplySuggested(false)
    setUpstreamAccountConfigs({})
    autoPreviewTriggeredRef.current = false
    ratioConfigLoadedRef.current = false
  }, [
    channelBaseUrl,
    channelSettings,
    forcedUpstreamPlatform,
    savedUpstreamCredentialAvailable,
  ])

  const clearRefreshPreview = useCallback(() => {
    setUpstreamRefreshPreviewId('')
    setUpstreamRefreshPreviewExpiresAt(0)
    setUpstreamRefreshSnapshot(null)
    setUpstreamRefreshTwoFactorChallenge(null)
    setUpstreamRefreshTwoFactorCode('')
  }, [])

  const applyUpstreamPreviewData = useCallback(
    (data: UpstreamAccountPreviewData) => {
      setUpstreamRefreshPreviewId(data.preview_id)
      setUpstreamRefreshPreviewExpiresAt(data.expires_at)
      setUpstreamRefreshSnapshot(data.snapshot)
      setUpstreamRefreshTwoFactorChallenge(null)
      setUpstreamRefreshTwoFactorCode('')
      setUpstreamAccountConfigs((prev) =>
        buildUpstreamAccountConfigsFromSnapshotKeys(data.snapshot.keys, prev)
      )
      const ratio = data.snapshot.ratio_conversion
      setUpstreamPaidCny(
        ratio?.paid_cny && Number.isFinite(ratio.paid_cny)
          ? String(ratio.paid_cny)
          : DEFAULT_UPSTREAM_PAID_AMOUNT
      )
      setUpstreamPlatformUsdCredit(
        ratio?.platform_usd_credit && Number.isFinite(ratio.platform_usd_credit)
          ? String(ratio.platform_usd_credit)
          : DEFAULT_UPSTREAM_PLATFORM_CREDIT
      )
      toast.success(
        t('Synced {{count}} upstream key(s)', {
          count: data.snapshot.keys.length,
        })
      )
    },
    [t]
  )

  const previewMutation = useMutation({
    mutationFn: previewUpstreamAccount,
  })

  const preview2FAMutation = useMutation({
    mutationFn: completeUpstreamAccountPreview2FA,
  })

  const refreshMutation = useMutation({
    mutationFn: ({
      id,
      payload,
    }: {
      id: number
      payload: Parameters<typeof refreshUpstreamAccountChannel>[1]
    }) => refreshUpstreamAccountChannel(id, payload),
    onSuccess: async (res) => {
      if (!res.success) {
        if (isUpstreamPreviewExpiredError(res.message)) {
          clearRefreshPreview()
          setUpstreamAccountConfigs({})
          toast.error(
            t(
              'The upstream account preview expired or was already used. Sync the upstream account again.'
            )
          )
          return
        }
        toast.error(res.message || t('Failed to refresh upstream account'))
        return
      }
      toast.success(
        t(
          'Upstream account refreshed: {{created}} created, {{updated}} updated, {{disabled}} disabled',
          {
            created: res.data?.created ?? 0,
            updated: res.data?.updated ?? 0,
            disabled: res.data?.disabled ?? 0,
          }
        )
      )
      setUpstreamPassword('')
      clearRefreshPreview()
      setUpstreamAccountConfigs({})
      await Promise.resolve(onSuccess())
    },
    onError: (error: unknown) => {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to refresh upstream account')
      toast.error(message)
    },
  })

  const handlePreviewUpstreamRefresh = useCallback(async () => {
    if (!canSensitiveWrite) {
      toast.error(noPermissionMessage)
      return
    }
    if (upstreamUseSavedCredential && !savedUpstreamCredentialAvailable) {
      toast.error(
        t(
          'No saved upstream login is available yet. Complete a sync once to enable it.'
        )
      )
      return
    }
    const refreshPlatform = forcedUpstreamPlatform ?? upstreamPlatform
    if (!refreshPlatform) {
      toast.error(t('Upstream platform is required'))
      return
    }
    if (!upstreamUseSavedCredential) {
      const baseUrl = upstreamBaseUrl.trim()
      if (!baseUrl) {
        toast.error(t('Upstream platform URL is required'))
        return
      }
      if (
        upstreamAuthMode === 'password' &&
        (!upstreamUsername.trim() || !upstreamPassword.trim())
      ) {
        toast.error(t('Account and password are required'))
        return
      }
      if (
        upstreamAuthMode === 'session_cookie' &&
        !upstreamSessionCookie.trim()
      ) {
        toast.error(t('Session/Cookie is required'))
        return
      }
      if (upstreamAuthMode === 'access_token' && !upstreamAccessToken.trim()) {
        toast.error(t('Access Token is required'))
        return
      }
      if (
        refreshPlatform === 'new-api' &&
        upstreamAuthMode === 'access_token' &&
        !upstreamUserId.trim()
      ) {
        toast.error(
          t('New-Api-User / User ID is required for new-api access token')
        )
        return
      }
      if (upstreamAuthMode === 'oauth_browser') {
        if (!upstreamCaptureId.trim()) {
          toast.error(
            t(
              'Complete userscript capture before previewing the upstream account'
            )
          )
          return
        }
      }
    }

    let res: Awaited<ReturnType<typeof previewUpstreamAccount>>
    try {
      res = await previewMutation.mutateAsync(
        buildUpstreamAccountPreviewRequest({
          channelId,
          platform: refreshPlatform,
          baseUrl: upstreamBaseUrl,
          username: upstreamUsername,
          password: upstreamPassword,
          authMode: upstreamAuthMode,
          captureId: upstreamCaptureId,
          sessionCookie: upstreamSessionCookie,
          userId: upstreamUserId,
          accessToken: upstreamAccessToken,
          refreshToken: upstreamRefreshToken,
          expiresAt: Number(upstreamTokenExpiresAt) || undefined,
          useSavedCredential: upstreamUseSavedCredential,
          ratioConversion: buildUpstreamRatioConversionPayload(
            upstreamPaidCny,
            upstreamPlatformUsdCredit
          ),
        })
      )
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to sync upstream account')
      toast.error(message)
      return
    }
    if (!res.success || !res.data) {
      toast.error(res.message || t('Failed to sync upstream account'))
      return
    }
    const challenge = getUpstreamPreviewChallenge(res.data)
    if (challenge) {
      setUpstreamRefreshTwoFactorChallenge(challenge)
      setUpstreamRefreshTwoFactorCode('')
      setUpstreamRefreshPreviewId('')
      setUpstreamRefreshPreviewExpiresAt(0)
      setUpstreamRefreshSnapshot(null)
      setUpstreamAccountConfigs({})
      toast.info(t('Enter the 2FA code from the upstream account.'))
      return
    }
    if (hasUpstreamPreviewSnapshot(res.data)) {
      applyUpstreamPreviewData(res.data)
      return
    }
    toast.error(t('Failed to sync upstream account'))
  }, [
    applyUpstreamPreviewData,
    canSensitiveWrite,
    channelId,
    forcedUpstreamPlatform,
    noPermissionMessage,
    previewMutation,
    savedUpstreamCredentialAvailable,
    t,
    upstreamBaseUrl,
    upstreamAccessToken,
    upstreamAuthMode,
    upstreamCaptureId,
    upstreamPassword,
    upstreamPaidCny,
    upstreamPlatform,
    upstreamPlatformUsdCredit,
    upstreamRefreshToken,
    upstreamSessionCookie,
    upstreamTokenExpiresAt,
    upstreamUseSavedCredential,
    upstreamUsername,
    upstreamUserId,
  ])

  const handleCompleteUpstreamTwoFactor = useCallback(async () => {
    const challenge = upstreamRefreshTwoFactorChallenge
    const code = upstreamRefreshTwoFactorCode.trim()
    if (!challenge) return
    if (isUpstreamRefreshTwoFactorExpired) {
      toast.error(
        t(
          'The upstream 2FA challenge expired. Sync the upstream account again.'
        )
      )
      clearRefreshPreview()
      setUpstreamAccountConfigs({})
      return
    }
    if (!code) {
      toast.error(t('Enter the upstream 2FA code'))
      return
    }

    let res: Awaited<ReturnType<typeof completeUpstreamAccountPreview2FA>>
    try {
      res = await preview2FAMutation.mutateAsync({
        challenge_id: challenge.challenge_id,
        code,
        ratio_conversion: buildUpstreamRatioConversionPayload(
          upstreamPaidCny,
          upstreamPlatformUsdCredit
        ),
      })
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to verify upstream 2FA code')
      toast.error(message)
      return
    }
    if (!res.success || !res.data) {
      toast.error(res.message || t('Failed to verify upstream 2FA code'))
      clearRefreshPreview()
      setUpstreamAccountConfigs({})
      return
    }
    applyUpstreamPreviewData(res.data)
  }, [
    applyUpstreamPreviewData,
    clearRefreshPreview,
    isUpstreamRefreshTwoFactorExpired,
    preview2FAMutation,
    t,
    upstreamPaidCny,
    upstreamPlatformUsdCredit,
    upstreamRefreshTwoFactorChallenge,
    upstreamRefreshTwoFactorCode,
  ])

  const busy =
    previewMutation.isPending ||
    preview2FAMutation.isPending ||
    refreshMutation.isPending

  useEffect(() => {
    if (!open || !channelId) {
      resetRefreshState()
      onBusyChange?.(false)
      return
    }
    resetRefreshState()
  }, [channelId, onBusyChange, open, resetRefreshState])

  useEffect(() => {
    if (
      upstreamAuthMode !== 'password' &&
      upstreamAuthMode !== 'oauth_browser'
    ) {
      setUpstreamAuthMode('oauth_browser')
    }
  }, [upstreamAuthMode])

  useEffect(() => {
    if (!open || !canReadChannelAccount) {
      return
    }
    if (upstreamRefreshSnapshot) {
      return
    }
    if (refreshAccounts.length === 0) {
      return
    }
    if (Object.keys(upstreamAccountConfigs).length > 0) {
      return
    }
    setUpstreamAccountConfigs(
      buildUpstreamAccountConfigsFromChannelAccounts(refreshAccounts)
    )
  }, [
    canReadChannelAccount,
    open,
    refreshAccounts,
    upstreamAccountConfigs,
    upstreamRefreshSnapshot,
  ])

  useEffect(() => {
    if (
      !open ||
      upstreamRefreshSnapshot ||
      ratioConfigLoadedRef.current ||
      refreshAccounts.length === 0
    ) {
      return
    }
    const ratioConfig = refreshAccounts.find(
      (account) => account.ratio_conversion_config
    )?.ratio_conversion_config
    if (!ratioConfig) return
    setUpstreamPaidCny(
      ratioConfig.paid_cny && Number.isFinite(ratioConfig.paid_cny)
        ? String(ratioConfig.paid_cny)
        : DEFAULT_UPSTREAM_PAID_AMOUNT
    )
    setUpstreamPlatformUsdCredit(
      ratioConfig.platform_usd_credit &&
        Number.isFinite(ratioConfig.platform_usd_credit)
        ? String(ratioConfig.platform_usd_credit)
        : DEFAULT_UPSTREAM_PLATFORM_CREDIT
    )
    ratioConfigLoadedRef.current = true
  }, [open, refreshAccounts, upstreamRefreshSnapshot])

  useEffect(() => {
    if (
      !open ||
      !savedUpstreamCredentialAvailable ||
      !upstreamUseSavedCredential ||
      busy ||
      autoPreviewTriggeredRef.current ||
      upstreamRefreshSnapshot
    ) {
      return
    }
    autoPreviewTriggeredRef.current = true
    void handlePreviewUpstreamRefresh()
  }, [
    busy,
    handlePreviewUpstreamRefresh,
    open,
    savedUpstreamCredentialAvailable,
    upstreamRefreshSnapshot,
    upstreamUseSavedCredential,
  ])

  useEffect(() => {
    onBusyChange?.(busy)
  }, [busy, onBusyChange])

  useEffect(() => {
    if (
      !upstreamRefreshPreviewExpiresAt &&
      !upstreamRefreshTwoFactorChallenge?.expires_at
    ) {
      return
    }
    setUpstreamPreviewNowMs(Date.now())
    const timer = window.setInterval(() => {
      setUpstreamPreviewNowMs(Date.now())
    }, 1000)
    return () => window.clearInterval(timer)
  }, [
    upstreamRefreshPreviewExpiresAt,
    upstreamRefreshTwoFactorChallenge?.expires_at,
  ])

  const showLoadedWarning =
    canReadChannelAccount && refreshAccountsTotal > refreshAccountsLoadedCount
  const previewManagementBaseURL =
    upstreamRefreshSnapshot?.management_base_url ||
    upstreamRefreshSnapshot?.base_url ||
    ''
  const previewRelayBaseURL =
    upstreamRefreshSnapshot?.relay_base_url ||
    upstreamRefreshSnapshot?.base_url ||
    ''

  const renderUpstreamPreviewExpiryNotice = useCallback(
    (remainingSeconds: number, expired: boolean) => (
      <Alert>
        <AlertCircle aria-hidden='true' />
        <AlertDescription>
          {expired
            ? t(
                'This upstream account preview expired. Sync the upstream account again before saving.'
              )
            : t(
                'This upstream account preview expires in {{time}}. Sync again if it expires before you save.',
                {
                  time: formatUpstreamPreviewRemaining(remainingSeconds),
                }
              )}
        </AlertDescription>
      </Alert>
    ),
    [t]
  )

  const renderTwoFactorChallenge = useCallback(() => {
    if (!upstreamRefreshTwoFactorChallenge) return null
    return (
      <div className='grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto]'>
        <div className='flex flex-col gap-2'>
          <Label htmlFor='upstream-refresh-2fa-code'>{t('2FA code')}</Label>
          <Input
            id='upstream-refresh-2fa-code'
            value={upstreamRefreshTwoFactorCode}
            onChange={(event) =>
              setUpstreamRefreshTwoFactorCode(event.target.value)
            }
            autoComplete='one-time-code'
            placeholder={t('Enter the upstream 2FA code')}
            disabled={refreshMutation.isPending || preview2FAMutation.isPending}
          />
        </div>
        <div className='flex items-end'>
          <Button
            type='button'
            variant='outline'
            className='w-full sm:w-auto'
            disabled={refreshMutation.isPending || preview2FAMutation.isPending}
            onClick={() => void handleCompleteUpstreamTwoFactor()}
          >
            {preview2FAMutation.isPending ? (
              <Loader2 data-icon='inline-start' className='animate-spin' />
            ) : (
              <CheckCircle2 data-icon='inline-start' />
            )}
            {t('Verify')}
          </Button>
        </div>
        <div className='sm:col-span-2'>
          <div className='text-muted-foreground text-xs'>
            {t('Enter the 2FA code from the upstream account.')}
          </div>
        </div>
      </div>
    )
  }, [
    handleCompleteUpstreamTwoFactor,
    preview2FAMutation.isPending,
    refreshMutation.isPending,
    t,
    upstreamRefreshTwoFactorChallenge,
    upstreamRefreshTwoFactorCode,
  ])

  const renderSnapshotReview = useCallback(
    (snapshot: Pick<UpstreamAccountSnapshot, 'balance' | 'keys'>) => {
      const capabilitySummary = summarizeUpstreamAccountCapabilities(
        snapshot.keys,
        upstreamAccountConfigs
      )
      return (
        <div className='flex flex-col gap-3'>
          <div className='grid gap-3 sm:grid-cols-3'>
            <div className='rounded-md border p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Synced Keys')}
              </div>
              <div className='text-lg font-semibold'>
                {capabilitySummary.enabledKeyCount}/
                {capabilitySummary.totalKeyCount}
              </div>
            </div>
            <div className='rounded-md border p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Routable Models')}
              </div>
              <div className='text-lg font-semibold'>
                {capabilitySummary.modelCount}
              </div>
            </div>
            <div className='rounded-md border p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('NexusTok Access Groups')}
              </div>
              <div className='truncate text-lg font-semibold'>
                {capabilitySummary.accessGroupText}
              </div>
            </div>
          </div>

          <div className='flex items-center justify-between gap-3'>
            <div className='flex flex-col gap-1'>
              <span className='text-sm font-medium'>
                {t(
                  'Use upstream suggestions to overwrite key priority and weight'
                )}
              </span>
              <span className='text-muted-foreground text-xs'>
                {t(
                  'Lower ratio conversion gets higher key priority and weight by default.'
                )}
              </span>
            </div>
            <Switch
              checked={upstreamApplySuggested}
              disabled={snapshot.keys.length === 0}
              onCheckedChange={setUpstreamApplySuggested}
            />
          </div>

          {snapshot.keys.length === 0 ? (
            <Alert>
              <AlertCircle aria-hidden='true' />
              <AlertDescription>
                {t('No upstream keys were found for this account.')}
              </AlertDescription>
            </Alert>
          ) : (
            <>
              {capabilitySummary.modelCount === 0 && (
                <Alert>
                  <AlertCircle aria-hidden='true' />
                  <AlertDescription>
                    {t('This key will not route any model.')}
                  </AlertDescription>
                </Alert>
              )}
              <div className='overflow-x-auto rounded-md border'>
                <div className='grid min-w-[84rem] grid-cols-[minmax(8rem,0.95fr)_minmax(16rem,1.35fr)_minmax(8rem,0.75fr)_minmax(9rem,0.8fr)_5.5rem_6.75rem_4.5rem_4.5rem_4rem] gap-2 border-b px-2 py-2 text-[11px] font-medium'>
                  <span className='min-w-0 truncate' title={t('Key')}>
                    {t('Key')}
                  </span>
                  <span className='min-w-0 truncate' title={t('Models')}>
                    {t('Models')}
                  </span>
                  <span className='min-w-0 truncate' title={t('Key Group')}>
                    {t('Key Group')}
                  </span>
                  <span
                    className='min-w-0 truncate'
                    title={t('NexusTok Access Groups')}
                  >
                    {t('NexusTok Access Groups')}
                  </span>
                  <span className='min-w-0 truncate' title={t('Key Ratio')}>
                    {t('Key Ratio')}
                  </span>
                  <span
                    className='min-w-0 truncate'
                    title={t('Ratio Conversion')}
                  >
                    {t('Ratio Conversion')}
                  </span>
                  <span className='min-w-0 truncate' title={t('Key Priority')}>
                    {t('Key Priority')}
                  </span>
                  <span className='min-w-0 truncate' title={t('Key Weight')}>
                    {t('Key Weight')}
                  </span>
                  <span className='min-w-0 truncate' title={t('Enabled')}>
                    {t('Enabled')}
                  </span>
                </div>
                {snapshot.keys.map((key, index) => {
                  const configId = upstreamAccountKeyConfigId(key, index)
                  const config = upstreamAccountConfigs[configId]
                  const currentModelsArrayValue =
                    upstreamAccountModelsArrayValue(key, config)
                  const updateConfig = (
                    updater: (
                      previous: UpstreamAccountConfigDraft | undefined
                    ) => UpstreamAccountConfigDraft
                  ) =>
                    setUpstreamAccountConfigs((prev) => ({
                      ...prev,
                      [configId]: updater(prev[configId]),
                    }))
                  const setConfigValue = (
                    overrides: Partial<UpstreamAccountConfigDraft>
                  ) =>
                    updateConfig((previous) =>
                      buildUpstreamAccountConfigDraft(key, previous, overrides)
                    )
                  const handleKeyModelsChange = (values: string[]) =>
                    setConfigValue({
                      models: formatModelsArray(dedupeModelNames(values)),
                    })
                  const upstreamModelNames = dedupeModelNames(key.models ?? [])
                  const handleUseUpstreamKeyModels = () => {
                    if (upstreamModelNames.length === 0) {
                      toast.info(t('No upstream models returned for this key'))
                      return
                    }
                    handleKeyModelsChange(upstreamModelNames)
                    toast.success(
                      t('Applied {{count}} upstream model(s)', {
                        count: upstreamModelNames.length,
                      })
                    )
                  }
                  const preventModelActionBlur = (event: {
                    preventDefault: () => void
                  }) => {
                    event.preventDefault()
                  }
                  const upstreamGroupValue =
                    key.group_name || key.group_id || ''
                  const currentGroupValue = config?.group ?? upstreamGroupValue
                  const currentAccessGroupsValue =
                    config?.access_groups ?? key.access_groups ?? 'default'
                  const currentAccessGroupsArrayValue = parseGroups(
                    currentAccessGroupsValue
                  )
                  const currentEnabledValue = config?.enabled ?? true
                  const accessGroupOptions = mergeGroupOptionsWithSelected(
                    groupOptions,
                    currentAccessGroupsArrayValue
                  )
                  const currentPriorityValue =
                    config?.priority ?? key.suggested_priority ?? 0
                  const currentWeightValue =
                    config?.weight ?? key.suggested_weight ?? 0
                  const currentKeyGroupLabel = getUpstreamKeyGroupLabel(key)
                  const keyRatioValue = getUpstreamKeyRatioDisplayValue(key)
                  const displayedRatioValue = getUpstreamRatioDisplayValue(key)
                  const modelRatioDetails = formatUpstreamModelRatioDetails(
                    key.model_ratios
                  )
                  const keyRatioTitle = modelRatioDetails
                    ? `${t('Model Ratios')}:\n${modelRatioDetails}`
                    : undefined
                  const ratioTitle = [
                    key.ratio_conversion != null
                      ? `${t('Ratio Conversion')}: ${formatUpstreamRatioCompact(key.ratio_conversion)}x`
                      : '',
                    key.effective_ratio != null
                      ? `${t('Upstream Ratio')}: ${formatUpstreamRatioCompact(key.effective_ratio)}x`
                      : '',
                    modelRatioDetails,
                  ]
                    .filter(Boolean)
                    .join('\n')
                  return (
                    <div
                      key={configId}
                      className='grid min-w-[84rem] grid-cols-[minmax(8rem,0.95fr)_minmax(16rem,1.35fr)_minmax(8rem,0.75fr)_minmax(9rem,0.8fr)_5.5rem_6.75rem_4.5rem_4.5rem_4rem] items-center gap-2 border-b px-2 py-2 last:border-b-0'
                    >
                      <div className='min-w-0'>
                        <div className='truncate text-sm font-medium'>
                          {key.name || key.masked_key}
                        </div>
                        <div className='text-muted-foreground truncate text-xs'>
                          {key.masked_key}
                        </div>
                      </div>
                      <div className='flex min-w-0 flex-col gap-1'>
                        <ModelCatalogMultiSelect
                          selected={currentModelsArrayValue}
                          onChange={handleKeyModelsChange}
                          extraModels={key.models ?? []}
                          placeholder={t('Select models or add custom ones')}
                          createLabel='Add custom model "{{value}}"'
                          maxVisibleChips={2}
                          copyChipOnClick
                          contentFooter={
                            <div className='bg-background flex flex-wrap gap-2 border-t pt-2'>
                              <Button
                                type='button'
                                variant='ghost'
                                size='sm'
                                onMouseDown={preventModelActionBlur}
                                onClick={handleUseUpstreamKeyModels}
                              >
                                {t('Use Upstream Models')} (
                                {upstreamModelNames.length})
                              </Button>
                            </div>
                          }
                          clearSearchOnSelect={false}
                          className='min-h-8'
                          compactInput
                        />
                        {currentModelsArrayValue.length === 0 ? (
                          <span
                            className={cn(
                              'truncate text-[11px]',
                              currentEnabledValue
                                ? 'text-destructive'
                                : 'text-muted-foreground'
                            )}
                          >
                            {currentEnabledValue
                              ? t('Models are required')
                              : t('This key will not route any model.')}
                          </span>
                        ) : null}
                      </div>
                      <div className='flex min-w-0 flex-col gap-1'>
                        <Input
                          value={currentGroupValue}
                          placeholder={t(
                            'Key group inherited from upstream if empty'
                          )}
                          onChange={(event) =>
                            setConfigValue({ group: event.target.value })
                          }
                          className='h-8 px-2 text-xs'
                        />
                        <span
                          className='text-muted-foreground truncate text-[11px]'
                          title={currentKeyGroupLabel || undefined}
                        >
                          {currentKeyGroupLabel || t('Inherited')}
                        </span>
                      </div>
                      <div className='flex min-w-0 flex-col gap-1'>
                        <MultiSelect
                          options={accessGroupOptions}
                          selected={currentAccessGroupsArrayValue}
                          onChange={(values) =>
                            setConfigValue({
                              access_groups: formatGroups(values),
                            })
                          }
                          placeholder={t(
                            'Please Select user groups that can access this channel.'
                          )}
                          maxVisibleChips={2}
                          className='min-h-8'
                          compactInput
                        />
                        {currentAccessGroupsArrayValue.length === 0 ? (
                          <span
                            className={cn(
                              'truncate text-[11px]',
                              currentEnabledValue
                                ? 'text-destructive'
                                : 'text-muted-foreground'
                            )}
                          >
                            {currentEnabledValue
                              ? t('Group is required')
                              : t(
                                  'This key will not be available to any user group.'
                                )}
                          </span>
                        ) : null}
                      </div>
                      <span className='font-mono text-xs' title={keyRatioTitle}>
                        {keyRatioValue != null
                          ? `${formatUpstreamRatioCompact(keyRatioValue)}x`
                          : '-'}
                      </span>
                      <div
                        className='flex min-w-0 flex-col gap-1'
                        title={ratioTitle || undefined}
                      >
                        <span className='font-mono text-xs'>
                          {displayedRatioValue != null
                            ? `${formatUpstreamRatioCompact(displayedRatioValue)}x`
                            : '-'}
                        </span>
                        {key.ratio_conversion != null &&
                          keyRatioValue != null &&
                          Math.abs(key.ratio_conversion - keyRatioValue) >
                            Number.EPSILON && (
                            <span className='text-muted-foreground truncate text-[11px]'>
                              {t('Converted')}
                            </span>
                          )}
                      </div>
                      <Input
                        type='number'
                        value={currentPriorityValue}
                        disabled={upstreamApplySuggested}
                        onChange={(event) =>
                          setConfigValue({
                            priority: Number(event.target.value),
                          })
                        }
                        className='h-8 px-2 text-xs'
                      />
                      <Input
                        type='number'
                        value={currentWeightValue}
                        disabled={upstreamApplySuggested}
                        onChange={(event) =>
                          setConfigValue({
                            weight: Number(event.target.value),
                          })
                        }
                        className='h-8 px-2 text-xs'
                      />
                      <Switch
                        checked={config?.enabled ?? true}
                        onCheckedChange={(checked) =>
                          setConfigValue({ enabled: checked })
                        }
                      />
                    </div>
                  )
                })}
              </div>
            </>
          )}
        </div>
      )
    },
    [
      t,
      upstreamAccountConfigs,
      upstreamApplySuggested,
    ]
  )

  if (!open || !channelId) {
    return null
  }

  const refreshPreviewButtonsDisabled =
    !canSensitiveWrite ||
    previewMutation.isPending ||
    preview2FAMutation.isPending ||
    refreshMutation.isPending

  const usingSavedCredential =
    savedUpstreamCredentialAvailable && upstreamUseSavedCredential
  const savedCredentialDisplayAuthMode =
    savedUpstreamCredentialAuthMode === 'password'
      ? 'password'
      : 'oauth_browser'
  const effectiveAuthMode = usingSavedCredential
    ? savedCredentialDisplayAuthMode
    : upstreamAuthMode
  const savedCredentialDescription = t('Saved upstream login will be reused')
  const loginURL = upstreamBaseUrl.trim()
  const canOpenLoginURL =
    /^https?:\/\//i.test(loginURL) && !previewMutation.isPending

  return (
    <div className='flex flex-col gap-4'>
      <div className='flex items-center gap-3 rounded-lg border px-3 py-3'>
        <div className='min-w-0 flex-1'>
          <div className='text-sm font-medium'>
            {t('Use saved upstream login')}
          </div>
          <div className='text-muted-foreground text-xs'>
            {savedUpstreamCredentialAvailable
              ? savedCredentialDescription
              : t(
                  'No saved upstream login is available yet. Complete a sync once to enable it.'
                )}
          </div>
        </div>
        <Switch
          checked={upstreamUseSavedCredential}
          disabled={
            !savedUpstreamCredentialAvailable ||
            previewMutation.isPending ||
            refreshMutation.isPending
          }
          onCheckedChange={setUpstreamUseSavedCredential}
        />
      </div>

      <div className='grid gap-3 lg:grid-cols-[1.5fr_7fr_1.5fr]'>
        <div className='flex min-w-0 flex-col gap-2'>
          <Label htmlFor='upstream-refresh-auth-mode'>
            {t('Authentication method')}
          </Label>
          <Select
            value={effectiveAuthMode}
            disabled={!canSensitiveWrite || usingSavedCredential}
            onValueChange={(value) =>
              setUpstreamAuthMode(value as UpstreamAccountAuthMode)
            }
          >
            <SelectTrigger
              id='upstream-refresh-auth-mode'
              className='w-full min-w-0'
            >
              <SelectValue>
                {effectiveAuthMode === 'password'
                  ? t('Account password')
                  : t('Automatic configuration')}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value='password'>
                  {t('Account password')}
                </SelectItem>
                <SelectItem value='oauth_browser'>
                  {t('Automatic configuration')}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
        <div className='flex min-w-0 flex-col gap-2'>
          <Label htmlFor='upstream-refresh-base-url'>
            {t('Upstream Platform URL')}
          </Label>
          <div className='flex gap-2'>
            <Input
              id='upstream-refresh-base-url'
              value={upstreamBaseUrl}
              onChange={(event) => setUpstreamBaseUrl(event.target.value)}
              placeholder={t('new-api or sub2api site URL')}
              disabled={!canSensitiveWrite || usingSavedCredential}
            />
            <Button
              type='button'
              variant='outline'
              size='icon'
              disabled={!canOpenLoginURL}
              onClick={() =>
                window.open(loginURL, '_blank', 'noopener,noreferrer')
              }
              title={t('Open upstream login page')}
            >
              <ExternalLink data-icon='icon' aria-hidden='true' />
              <span className='sr-only'>{t('Open upstream login page')}</span>
            </Button>
          </div>
        </div>
        <div className='flex items-end'>
          <Button
            type='button'
            variant='outline'
            className='w-full'
            disabled={
              !canSensitiveWrite ||
              usingSavedCredential ||
              effectiveAuthMode !== 'oauth_browser' ||
              !upstreamBaseUrl.trim()
            }
            onClick={() => capturePanelRef.current?.start()}
          >
            <RefreshCw data-icon='inline-start' />
            {t('Create New Session')}
          </Button>
        </div>
      </div>

      {!usingSavedCredential && effectiveAuthMode === 'password' ? (
        <div className='grid gap-3 sm:grid-cols-2'>
          <div className='flex flex-col gap-2'>
            <Label htmlFor='upstream-refresh-account'>{t('Account')}</Label>
            <Input
              id='upstream-refresh-account'
              value={upstreamUsername}
              onChange={(event) => setUpstreamUsername(event.target.value)}
              autoComplete='username'
              placeholder={
                upstreamPlatform === 'new-api' ? t('Username') : t('Email')
              }
              disabled={!canSensitiveWrite || usingSavedCredential}
            />
          </div>
          <div className='flex flex-col gap-2'>
            <Label htmlFor='upstream-refresh-password'>{t('Password')}</Label>
            <Input
              id='upstream-refresh-password'
              value={upstreamPassword}
              onChange={(event) => setUpstreamPassword(event.target.value)}
              type='password'
              autoComplete='current-password'
              placeholder={
                usingSavedCredential
                  ? savedCredentialDescription
                  : t('Password')
              }
              disabled={!canSensitiveWrite || usingSavedCredential}
            />
          </div>
        </div>
      ) : null}

      {!usingSavedCredential && effectiveAuthMode === 'oauth_browser' ? (
        <UpstreamAccountCapturePanel
          ref={capturePanelRef}
          platform={forcedUpstreamPlatform ?? upstreamPlatform}
          baseUrl={upstreamBaseUrl}
          channelId={channelId}
          disabled={!canSensitiveWrite || usingSavedCredential}
          captureId={upstreamCaptureId}
          returnUrl={captureReturnUrl}
          onCaptureIdChange={setUpstreamCaptureId}
          onCompleted={() => void handlePreviewUpstreamRefresh()}
        />
      ) : null}

      <div className='grid gap-3 sm:grid-cols-2'>
        <div className='flex flex-col gap-2'>
          <Label htmlFor='upstream-refresh-paid-cny'>{t('Paid Amount')}</Label>
          <Input
            id='upstream-refresh-paid-cny'
            value={upstreamPaidCny}
            onChange={(event) => setUpstreamPaidCny(event.target.value)}
            inputMode='decimal'
            placeholder={DEFAULT_UPSTREAM_PAID_AMOUNT}
            disabled={!canSensitiveWrite}
          />
        </div>
        <div className='flex flex-col gap-2'>
          <Label htmlFor='upstream-refresh-platform-usd-credit'>
            {t('Upstream Platform Credit')}
          </Label>
          <Input
            id='upstream-refresh-platform-usd-credit'
            value={upstreamPlatformUsdCredit}
            onChange={(event) =>
              setUpstreamPlatformUsdCredit(event.target.value)
            }
            inputMode='decimal'
            placeholder={DEFAULT_UPSTREAM_PLATFORM_CREDIT}
            disabled={!canSensitiveWrite}
          />
        </div>
      </div>
      <div className='flex min-w-0 flex-col items-stretch gap-2 sm:flex-row sm:items-end sm:justify-end'>
        <Button
          type='button'
          variant='outline'
          className='min-w-0 whitespace-nowrap sm:min-w-36'
          disabled={refreshPreviewButtonsDisabled}
          onClick={() => void handlePreviewUpstreamRefresh()}
        >
          {previewMutation.isPending ? (
            <Loader2 data-icon='inline-start' className='animate-spin' />
          ) : (
            <RefreshCw data-icon='inline-start' />
          )}
          {t('Preview Refresh')}
        </Button>
        <Button
          type='button'
          className='min-w-0 whitespace-nowrap sm:min-w-36'
          disabled={
            !upstreamRefreshSnapshot ||
            upstreamRefreshSnapshot.keys.length === 0 ||
            isUpstreamRefreshPreviewExpired ||
            refreshPreviewButtonsDisabled
          }
          onClick={() => {
            if (!channelId || !upstreamRefreshSnapshot) return
            if (isUpstreamRefreshPreviewExpired) {
              clearRefreshPreview()
              setUpstreamAccountConfigs({})
              toast.error(
                t(
                  'The upstream account preview expired or was already used. Sync the upstream account again.'
                )
              )
              return
            }
            const capabilityErrors =
              collectUpstreamAccountCapabilityValidationErrors(
                upstreamRefreshSnapshot.keys,
                upstreamAccountConfigs
              )
            if (showUpstreamCapabilityValidationError(capabilityErrors)) {
              return
            }
            void refreshMutation.mutateAsync({
              id: channelId,
              payload: buildUpstreamAccountRefreshPayload({
                previewId: upstreamRefreshPreviewId,
                keys: upstreamRefreshSnapshot.keys,
                configs: upstreamAccountConfigs,
                applySuggested: upstreamApplySuggested,
                ratioConversion: buildUpstreamRatioConversionPayload(
                  upstreamPaidCny,
                  upstreamPlatformUsdCredit
                ),
              }),
            })
          }}
        >
          {refreshMutation.isPending ? (
            <Loader2 data-icon='inline-start' className='animate-spin' />
          ) : (
            <CheckCircle2 data-icon='inline-start' />
          )}
          {t('Apply Refresh')}
        </Button>
      </div>

      {upstreamRefreshTwoFactorChallenge && renderTwoFactorChallenge()}

      {upstreamRefreshSnapshot?.warnings?.length ? (
        <Alert>
          <AlertCircle aria-hidden='true' />
          <AlertDescription>
            {upstreamRefreshSnapshot.warnings.join('；')}
          </AlertDescription>
        </Alert>
      ) : null}

      {upstreamRefreshSnapshot &&
      (previewManagementBaseURL || previewRelayBaseURL) ? (
        <Alert>
          <AlertCircle aria-hidden='true' />
          <AlertDescription>
            <div className='flex flex-col gap-1'>
              {previewManagementBaseURL ? (
                <div>
                  {t(
                    'Management requests will use {{url}} for user, group, key, and balance sync.',
                    { url: previewManagementBaseURL }
                  )}
                </div>
              ) : null}
              {previewRelayBaseURL ? (
                <div>
                  {t('Created channels will use {{url}} for model requests.', {
                    url: previewRelayBaseURL,
                  })}
                </div>
              ) : null}
            </div>
          </AlertDescription>
        </Alert>
      ) : null}

      {upstreamRefreshSnapshot &&
        renderUpstreamPreviewExpiryNotice(
          upstreamRefreshPreviewRemaining,
          isUpstreamRefreshPreviewExpired
        )}

      {canReadChannelAccount && refreshAccountsQuery.isError ? (
        <Alert>
          <AlertCircle aria-hidden='true' />
          <AlertDescription>
            {t('Failed to load channel accounts')}
          </AlertDescription>
        </Alert>
      ) : null}

      {upstreamRefreshSnapshot ? (
        renderSnapshotReview(upstreamRefreshSnapshot)
      ) : canReadChannelAccount && refreshAccountsQuery.isLoading ? (
        <div className='text-muted-foreground flex items-center gap-2 text-sm'>
          <Loader2 data-icon='inline-start' className='animate-spin' />
          {t('Loading synced keys...')}
        </div>
      ) : showLoadedWarning ? (
        <Alert>
          <AlertCircle aria-hidden='true' />
          <AlertDescription>
            {t(
              'Only {{loaded}} of {{total}} synced keys are loaded. Open the channel account list to edit all keys.',
              {
                loaded: refreshAccountsLoadedCount,
                total: refreshAccountsTotal,
              }
            )}
          </AlertDescription>
        </Alert>
      ) : null}

    </div>
  )
}
