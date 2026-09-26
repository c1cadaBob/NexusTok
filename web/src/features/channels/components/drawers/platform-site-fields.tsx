import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  ChevronDown,
  ExternalLink,
  KeyRound,
  Loader2,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import {
  cancelPlatformSiteAuthFlow,
  getPlatformSiteCaptureStatus,
  startPlatformSiteAuthFlow,
  startPlatformSiteCapture,
  verifyPlatformSiteAuthFlow,
} from '../../api'
import { CHANNEL_TYPE_NEW_API, CHANNEL_TYPE_SUB2_API } from '../../constants'
import type { ChannelFormValues } from '../../lib'
import type { UpstreamSiteStatus } from '../../types'

type PlatformSiteFieldsProps = {
  disabled: boolean
  isEditing: boolean
  channelId?: number
  syncStatus?: UpstreamSiteStatus
  onCaptureCompleted?: (captureID: string) => void
}

const RATIO_COMPARE_EPSILON = 0.0000001
const RATIO_INPUT_PRECISION = 1000000

function openPlatformCaptureURL(
  url: string,
  targetWindow?: Window | null
): boolean {
  const targetURL = url.trim()
  if (!targetURL) return false
  try {
    if (targetWindow && !targetWindow.closed) {
      targetWindow.location.href = targetURL
      targetWindow.focus()
      return true
    }
  } catch {
    // 复用预打开窗口失败时继续尝试创建新标签页。
  }
  try {
    const opened = window.open(targetURL, '_blank')
    if (!opened) return false
    try {
      opened.opener = null
    } catch {
      // 部分浏览器不允许修改 opener；打开结果仍然可用。
    }
    opened.focus()
    return true
  } catch {
    return false
  }
}

function formatPlatformAuthType(
  value: string | undefined,
  t: (key: string) => string
): string {
  switch (value) {
    case 'password':
      return t('Username and password')
    case 'auto':
      return t('Automatic configuration')
    case 'access_token':
      return t('Access token')
    case 'admin_key':
      return t('Admin Key')
    case 'cookie':
      return t('Cookie')
    default:
      return value || '-'
  }
}

function normalizePlatformNumber(
  value: number | null | undefined,
  fallback = 0
): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
    return fallback
  }
  return value
}

function normalizePlatformRatio(value: number): number {
  return Math.round(value * RATIO_INPUT_PRECISION) / RATIO_INPUT_PRECISION
}

function formatPlatformRatioInput(value: number): string {
  return String(normalizePlatformRatio(normalizePlatformNumber(value, 1)))
}

export function PlatformSiteFields(props: PlatformSiteFieldsProps) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  const channelType = useWatch({
    control: form.control,
    name: 'type',
  })
  const platform = useWatch({
    control: form.control,
    name: 'platform_site_platform',
  })
  const authType = useWatch({
    control: form.control,
    name: 'platform_site_auth_type',
  })
  const baseURL = useWatch({
    control: form.control,
    name: 'base_url',
  })
  const captureID = useWatch({
    control: form.control,
    name: 'platform_site_capture_id',
  })
  const authFlowID = useWatch({
    control: form.control,
    name: 'platform_site_auth_flow_id',
  })
  const username = useWatch({
    control: form.control,
    name: 'platform_site_username',
  })
  const password = useWatch({
    control: form.control,
    name: 'platform_site_password',
  })
  const watchedRechargeAmount = useWatch({
    control: form.control,
    name: 'platform_site_recharge_amount',
  })
  const watchedCreditedAmount = useWatch({
    control: form.control,
    name: 'platform_site_credited_amount',
  })
  const watchedConversionRatio = useWatch({
    control: form.control,
    name: 'platform_site_conversion_ratio',
  })
  const watchedRatioIsOverridden = useWatch({
    control: form.control,
    name: 'platform_site_conversion_ratio_override',
  })
  const rechargeAmount = normalizePlatformNumber(watchedRechargeAmount)
  const creditedAmount = normalizePlatformNumber(watchedCreditedAmount)
  const conversionRatio = normalizePlatformNumber(watchedConversionRatio, 1)
  const previewRatio =
    creditedAmount > 0
      ? normalizePlatformRatio(rechargeAmount / creditedAmount)
      : undefined
  const ratioIsOverridden = watchedRatioIsOverridden === true
  const completedCaptureRef = useRef('')
  const pendingCaptureWindowRef = useRef<Window | null>(null)
  const activeCaptureWindowRef = useRef<Window | null>(null)
  const [handoffURL, setHandoffURL] = useState('')
  const [showHandoffFallback, setShowHandoffFallback] = useState(false)
  const [twoFactorCode, setTwoFactorCode] = useState('')
  const [authFlowStatus, setAuthFlowStatus] = useState<
    'two_factor_required' | 'authenticated' | undefined
  >()
  const [advancedAuthOpen, setAdvancedAuthOpen] = useState(
    authType === 'access_token' ||
      authType === 'admin_key' ||
      authType === 'cookie'
  )
  const authFlowRef = useRef('')

  const captureStatusQuery = useQuery({
    queryKey: ['platform-site-capture-status', captureID],
    queryFn: () => getPlatformSiteCaptureStatus(captureID || ''),
    enabled: Boolean(captureID),
    refetchInterval: (query) => {
      const status = query.state.data?.data?.status
      return status === 'completed' || status === 'failed' ? false : 2000
    },
  })

  const captureMutation = useMutation({
    mutationFn: () => {
      if (
        authType !== 'auto' &&
        authType !== 'access_token' &&
        authType !== 'admin_key' &&
        authType !== 'cookie'
      ) {
        throw new Error(t('Select a script-based authentication method first.'))
      }
      return startPlatformSiteCapture({
        platform,
        base_url: baseURL || '',
        auth_type: authType,
        channel_id: props.channelId,
      })
    },
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        toast.error(response.message || t('Failed to create capture session'))
        return
      }
      form.setValue('platform_site_capture_id', response.data.capture_id, {
        shouldDirty: true,
        shouldValidate: true,
      })
      completedCaptureRef.current = ''
      setHandoffURL(response.data.handoff_url)
      const targetWindow = pendingCaptureWindowRef.current
      pendingCaptureWindowRef.current = null
      const hasTargetWindow = Boolean(targetWindow && !targetWindow.closed)
      if (hasTargetWindow) {
        activeCaptureWindowRef.current = targetWindow
      }
      const opened =
        hasTargetWindow &&
        openPlatformCaptureURL(response.data.handoff_url, targetWindow)
      if (!opened) {
        setShowHandoffFallback(true)
        toast.info(
          t(
            'Browser blocked the upstream capture tab. Use the button below to continue.'
          )
        )
      } else {
        setShowHandoffFallback(false)
      }
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
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to create capture session')
      )
    },
  })

  const passwordAuthMutation = useMutation({
    mutationFn: () =>
      startPlatformSiteAuthFlow({
        platform,
        base_url: baseURL || '',
        auth_type: 'password',
        username: form.getValues('platform_site_username') || '',
        password: form.getValues('platform_site_password') || '',
        channel_id: props.channelId,
      }),
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        toast.error(response.message || t('Platform login failed'))
        return
      }
      const flow = response.data
      form.setValue('platform_site_auth_flow_id', flow.flow_id, {
        shouldDirty: true,
        shouldValidate: true,
      })
      authFlowRef.current = flow.flow_id
      setAuthFlowStatus(flow.status === 'two_factor_required' ? flow.status : 'authenticated')
      form.setValue('platform_site_username', '')
      form.setValue('platform_site_password', '')
      if (flow.status === 'two_factor_required') {
        setTwoFactorCode('')
        toast.info(t('Enter the upstream two-factor code to continue'))
        return
      }
      toast.success(t('Platform login verified'))
    },
    onError: (error: unknown) => {
      toast.error(
        error instanceof Error ? error.message : t('Platform login failed')
      )
    },
  })

  const verifyAuthFlowMutation = useMutation({
    mutationFn: () =>
      verifyPlatformSiteAuthFlow(authFlowID || '', {
        code: twoFactorCode.trim(),
      }),
    onSuccess: (response) => {
      if (!response.success || !response.data) {
        toast.error(response.message || t('Two-factor verification failed'))
        return
      }
      if (response.data.status !== 'authenticated') {
        toast.error(response.message || t('Two-factor verification failed'))
        return
      }
      form.setValue('platform_site_auth_flow_id', response.data.flow_id, {
        shouldDirty: true,
        shouldValidate: true,
      })
      authFlowRef.current = response.data.flow_id
      setAuthFlowStatus('authenticated')
      setTwoFactorCode('')
      toast.success(t('Platform login verified'))
    },
    onError: (error: unknown) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Two-factor verification failed')
      )
    },
  })

  const captureStatus = captureStatusQuery.data?.data
  const onCaptureCompleted = props.onCaptureCompleted

  useEffect(() => {
    if (!captureStatus || captureStatus.status !== 'completed') return
    if (completedCaptureRef.current === captureStatus.capture_id) return
    completedCaptureRef.current = captureStatus.capture_id
    setShowHandoffFallback(false)
    toast.success(t('Upstream login state captured'))
    onCaptureCompleted?.(captureStatus.capture_id)
  }, [captureStatus, onCaptureCompleted, t])

  function handleStartCapture() {
    if (props.disabled || captureMutation.isPending || !(baseURL || '').trim()) {
      return
    }
    try {
      pendingCaptureWindowRef.current = window.open('about:blank', '_blank')
      activeCaptureWindowRef.current = pendingCaptureWindowRef.current
      if (pendingCaptureWindowRef.current) {
        try {
          pendingCaptureWindowRef.current.opener = null
        } catch {
          // 部分浏览器不允许修改 opener；仍保留窗口引用用于后续导航。
        }
      }
    } catch {
      pendingCaptureWindowRef.current = null
      activeCaptureWindowRef.current = null
    }
    if (!pendingCaptureWindowRef.current) {
      setShowHandoffFallback(true)
      toast.info(
        t(
          'Browser blocked the upstream capture tab. Use the button below to continue.'
        )
      )
    }
    captureMutation.mutate()
  }

  function handleOpenHandoff() {
    const opened = openPlatformCaptureURL(
      handoffURL || captureStatus?.handoff_url || '',
      activeCaptureWindowRef.current
    )
    if (!opened) {
      toast.error(t('Capture page is not ready yet'))
      return
    }
    setShowHandoffFallback(false)
  }

  useEffect(() => {
    if (ratioIsOverridden || previewRatio === undefined) {
      return
    }
    if (Math.abs(conversionRatio - previewRatio) <= RATIO_COMPARE_EPSILON) {
      return
    }
    form.setValue('platform_site_conversion_ratio', previewRatio, {
      shouldDirty: false,
      shouldValidate: false,
    })
  }, [conversionRatio, form, previewRatio, ratioIsOverridden])

  useEffect(() => {
    if (channelType === CHANNEL_TYPE_NEW_API && platform !== 'newapi') {
      form.setValue('platform_site_platform', 'newapi', {
        shouldDirty: true,
        shouldValidate: true,
      })
      return
    }
    if (channelType === CHANNEL_TYPE_SUB2_API && platform !== 'sub2api') {
      form.setValue('platform_site_platform', 'sub2api', {
        shouldDirty: true,
        shouldValidate: true,
      })
    }
  }, [channelType, form, platform])

  useEffect(() => {
    if (
      authType === 'access_token' ||
      authType === 'admin_key' ||
      authType === 'cookie'
    ) {
      setAdvancedAuthOpen(true)
    }
  }, [authType])

  useEffect(() => {
    authFlowRef.current = authFlowID || ''
  }, [authFlowID])

  useEffect(() => {
    return () => {
      const flowID = authFlowRef.current
      if (flowID) {
        void cancelPlatformSiteAuthFlow(flowID)
      }
    }
  }, [])

  function clearAuthenticationFields() {
    const previousFlowID = authFlowRef.current
    if (previousFlowID) {
      void cancelPlatformSiteAuthFlow(previousFlowID)
    }
    authFlowRef.current = ''
    setAuthFlowStatus(undefined)
    form.setValue('platform_site_auth_flow_id', '')
    form.setValue('platform_site_username', '')
    form.setValue('platform_site_password', '')
    form.setValue('platform_site_access_token', '')
    form.setValue('platform_site_admin_key', '')
    form.setValue('platform_site_cookie', '')
    form.setValue('platform_site_capture_id', '')
    setTwoFactorCode('')
    setHandoffURL('')
    setShowHandoffFallback(false)
    completedCaptureRef.current = ''
  }

  return (
    <fieldset
      disabled={props.disabled}
      className='space-y-4 disabled:opacity-60'
    >
      {props.syncStatus && (
        <div className='border-border/60 bg-background grid gap-1 rounded-md border p-3 text-sm'>
          <div className='flex flex-wrap items-center gap-2'>
            <span className='text-muted-foreground'>{t('Sync status')}</span>
            <span className='font-medium'>{props.syncStatus.sync_status}</span>
            {props.syncStatus.consecutive_failures > 0 && (
              <span className='text-destructive'>
                {t('Consecutive failures')}:{' '}
                {props.syncStatus.consecutive_failures}
              </span>
            )}
          </div>
          {props.syncStatus.last_sync_error && (
            <p className='text-destructive text-xs'>
              {props.syncStatus.last_sync_error}
            </p>
          )}
        </div>
      )}
      <div className='grid gap-4 sm:grid-cols-2'>
        <FormField
          control={form.control}
          name='platform_site_platform'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Platform site')}</FormLabel>
              <Select
                value={field.value}
                onValueChange={(value) => {
                  field.onChange(value)
                  form.setValue(
                    'type',
                    value === 'sub2api'
                      ? CHANNEL_TYPE_SUB2_API
                      : CHANNEL_TYPE_NEW_API,
                    {
                      shouldDirty: true,
                      shouldValidate: true,
                    }
                  )
                }}
                items={[
                  { value: 'newapi', label: 'NewAPI' },
                  { value: 'sub2api', label: 'Sub2API' },
                ]}
              >
                <FormControl>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value='newapi'>NewAPI</SelectItem>
                  <SelectItem value='sub2api'>Sub2API</SelectItem>
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name='platform_site_auth_type'
          render={({ field }) => (
            <div className='space-y-2'>
              <FormItem>
                <FormLabel>{t('Authentication method')}</FormLabel>
                <Select
                  value={field.value}
                  onValueChange={(value) => {
                    field.onChange(value)
                    clearAuthenticationFields()
                  }}
                  items={[
                    { value: 'password', label: t('Username and password') },
                    { value: 'auto', label: t('Automatic configuration') },
                    { value: 'access_token', label: t('Access token') },
                    { value: 'admin_key', label: t('Admin Key') },
                    { value: 'cookie', label: t('Cookie') },
                  ]}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value='password'>
                      {t('Username and password')}
                    </SelectItem>
                    <SelectItem value='auto'>
                      {t('Automatic configuration')}
                    </SelectItem>
                    {advancedAuthOpen && (
                      <>
                        <SelectItem value='access_token'>
                          {t('Access token')}
                        </SelectItem>
                        <SelectItem value='admin_key'>
                          {t('Admin Key')}
                        </SelectItem>
                        <SelectItem value='cookie'>{t('Cookie')}</SelectItem>
                      </>
                    )}
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
              <Button
                type='button'
                variant='ghost'
                size='sm'
                className='h-auto px-0 text-xs'
                onClick={() => setAdvancedAuthOpen((open) => !open)}
                aria-expanded={advancedAuthOpen}
              >
                <ChevronDown
                  className={`size-4 transition-transform ${
                    advancedAuthOpen ? 'rotate-180' : ''
                  }`}
                  aria-hidden='true'
                />
                {t('Advanced authentication')}
              </Button>
            </div>
          )}
        />
      </div>

      <FormField
        control={form.control}
        name='base_url'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('Site URL *')}</FormLabel>
            <FormControl>
              <Input type='url' placeholder='https://example.com' {...field} />
            </FormControl>
            <FormDescription>
              {t(
                'HTTP and HTTPS are supported, including private and local addresses.'
              )}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      {authType === 'password' && !authFlowID && (
        <div className='space-y-3'>
          <div className='grid gap-4 sm:grid-cols-2'>
            <FormField
              control={form.control}
              name='platform_site_username'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Username')}</FormLabel>
                  <FormControl>
                    <Input autoComplete='username' {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='platform_site_password'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Password')}</FormLabel>
                  <FormControl>
                    <Input
                      type='password'
                      autoComplete='new-password'
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
          <Button
            type='button'
            size='sm'
            onClick={() => passwordAuthMutation.mutate()}
            disabled={
              props.disabled ||
              passwordAuthMutation.isPending ||
              !(baseURL || '').trim() ||
              !username?.trim() ||
              !password
            }
          >
            {passwordAuthMutation.isPending ? (
              <Loader2 className='size-4 animate-spin' aria-hidden='true' />
            ) : (
              <KeyRound className='size-4' aria-hidden='true' />
            )}
            {t('Sign in to upstream site')}
          </Button>
        </div>
      )}

      {authType === 'password' && authFlowID && (
        <div className='border-border/60 bg-background grid gap-3 rounded-md border p-3'>
          <div className='flex items-center gap-2'>
            <ShieldCheck className='size-4' aria-hidden='true' />
            <span className='text-sm font-medium'>
              {authFlowStatus === 'two_factor_required'
                ? t('Two-factor verification required')
                : t('Upstream authentication verified')}
            </span>
          </div>
          {authFlowStatus === 'two_factor_required' && (
            <div className='grid gap-2 sm:max-w-xs'>
              <FormLabel htmlFor='platform-site-two-factor-code'>
                {t('Two-factor code')}
              </FormLabel>
              <Input
                id='platform-site-two-factor-code'
                inputMode='numeric'
                autoComplete='one-time-code'
                maxLength={6}
                value={twoFactorCode}
                onChange={(event) =>
                  setTwoFactorCode(
                    event.target.value.replaceAll(/\D/g, '').slice(0, 6)
                  )
                }
                placeholder={t('Enter the 6-digit code')}
              />
              <Button
                type='button'
                size='sm'
                onClick={() => verifyAuthFlowMutation.mutate()}
                disabled={
                  props.disabled ||
                  verifyAuthFlowMutation.isPending ||
                  twoFactorCode.length !== 6
                }
              >
                {verifyAuthFlowMutation.isPending && (
                  <Loader2
                    className='size-4 animate-spin'
                    aria-hidden='true'
                  />
                )}
                {t('Verify two-factor code')}
              </Button>
            </div>
          )}
        </div>
      )}

      {authType === 'auto' && (
        <div className='border-border/60 bg-background grid gap-3 rounded-md border p-3'>
          <div className='flex flex-wrap items-center gap-2'>
            <ShieldCheck className='size-4' aria-hidden='true' />
            <span className='text-sm font-medium'>
              {t('Browser login state capture')}
            </span>
            {captureStatus?.status === 'completed' && (
              <span className='text-muted-foreground text-xs'>
                {t('Captured')}
              </span>
            )}
          </div>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Open the upstream site and let the Capture Helper choose the best available authentication method. Tokens, cookies, and Admin Keys are never entered here.'
            )}
          </p>
          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              size='sm'
              onClick={handleStartCapture}
              disabled={
                props.disabled ||
                captureMutation.isPending ||
                !(baseURL || '').trim()
              }
            >
              {captureMutation.isPending ? (
                <Loader2 className='size-4 animate-spin' aria-hidden='true' />
              ) : (
                <ShieldCheck className='size-4' aria-hidden='true' />
              )}
              {t('Capture upstream login state')}
            </Button>
            {showHandoffFallback && (handoffURL || captureStatus?.handoff_url) && (
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={handleOpenHandoff}
              >
                <ExternalLink className='size-4' aria-hidden='true' />
                {t('Open upstream capture page')}
              </Button>
            )}
            {captureStatus?.helper_install_url && (
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() =>
                  window.open(
                    captureStatus.helper_install_url,
                    '_blank',
                    'noopener,noreferrer'
                  )
                }
              >
                <ExternalLink className='size-4' aria-hidden='true' />
                {t('Install Capture Helper')}
              </Button>
            )}
            {captureID && captureStatus?.status !== 'completed' && (
              <Button
                type='button'
                variant='ghost'
                size='sm'
                onClick={() => void captureStatusQuery.refetch()}
                disabled={captureStatusQuery.isFetching}
              >
                <RefreshCw
                  className='size-4'
                  aria-hidden='true'
                />
                {t('Refresh capture status')}
              </Button>
            )}
          </div>
          {captureStatus?.status === 'failed' && (
            <p className='text-destructive text-xs'>
              {captureStatus.message || t('Upstream login state capture failed')}
            </p>
          )}
          {captureStatus?.summary && (
            <div className='text-muted-foreground grid gap-1 text-xs'>
              <span>
                {t('Authentication method')}:{' '}
                {formatPlatformAuthType(captureStatus.summary.auth_type, t)}
              </span>
              {captureStatus.summary.access_token_masked && (
                <span>
                  {t('Access token')}: {captureStatus.summary.access_token_masked}
                </span>
              )}
              {captureStatus.summary.refresh_token_present && (
                <span>{t('Refresh token captured')}</span>
              )}
              {captureStatus.summary.admin_key_present && (
                <span>{t('Admin Key captured')}</span>
              )}
              {captureStatus.summary.cookie_present && (
                <span>{t('Cookie captured')}</span>
              )}
              {captureStatus.summary.token_expires_at && (
                <span>
                  {t('Token expires')}: {new Date(
                    captureStatus.summary.token_expires_at * 1000
                  ).toLocaleString()}
                </span>
              )}
            </div>
          )}
        </div>
      )}

      {authType === 'access_token' && (
        <FormField
          control={form.control}
          name='platform_site_access_token'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Access token')}</FormLabel>
              <FormControl>
                <Input type='password' autoComplete='off' {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}

      {authType === 'admin_key' && (
        <FormField
          control={form.control}
          name='platform_site_admin_key'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Admin Key')}</FormLabel>
              <FormControl>
                <Input type='password' autoComplete='off' {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}

      {authType === 'cookie' && (
        <FormField
          control={form.control}
          name='platform_site_cookie'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Cookie')}</FormLabel>
              <FormControl>
                <Input type='password' autoComplete='off' {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}

      <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
        <FormField
          control={form.control}
          name='platform_site_recharge_amount'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Recharge amount')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min='0'
                  step='0.001'
                  {...field}
                  onChange={(event) =>
                    field.onChange(
                      normalizePlatformNumber(event.target.valueAsNumber)
                    )
                  }
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='platform_site_credited_amount'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Credited amount')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min='0'
                  step='0.001'
                  {...field}
                  onChange={(event) =>
                    field.onChange(
                      normalizePlatformNumber(event.target.valueAsNumber)
                    )
                  }
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='platform_site_conversion_ratio'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Conversion ratio')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min='0'
                  step='0.001'
                  {...field}
                  value={formatPlatformRatioInput(field.value)}
                  onChange={(event) => {
                    field.onChange(
                      normalizePlatformRatio(
                        normalizePlatformNumber(event.target.valueAsNumber)
                      )
                    )
                    form.setValue(
                      'platform_site_conversion_ratio_override',
                      true,
                      { shouldDirty: true }
                    )
                  }}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='platform_site_conversion_ratio_override'
          render={({ field }) => (
            <FormItem className='flex min-h-9 items-center justify-between gap-3 self-end rounded-md border p-2'>
              <FormLabel className='cursor-pointer text-xs'>
                {t('Override conversion ratio')}
              </FormLabel>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={(checked) => {
                    field.onChange(checked)
                    if (!checked) {
                      form.setValue(
                        'platform_site_conversion_ratio',
                        previewRatio ?? 1,
                        { shouldDirty: true, shouldValidate: false }
                      )
                    }
                  }}
                />
              </FormControl>
            </FormItem>
          )}
        />
      </div>
    </fieldset>
  )
}
