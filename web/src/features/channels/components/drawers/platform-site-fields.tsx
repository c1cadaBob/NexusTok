import { useEffect, useRef } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ExternalLink, Loader2, RefreshCw, ShieldCheck } from 'lucide-react'
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
  getPlatformSiteCaptureStatus,
  startPlatformSiteCapture,
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
      const opened = window.open(
        response.data.handoff_url,
        '_blank',
        'noopener,noreferrer'
      )
      if (!opened) {
        toast.error(t('Browser blocked the upstream capture tab.'))
      }
      toast.success(t('Capture session created'))
    },
    onError: (error: unknown) => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to create capture session')
      )
    },
  })

  const captureStatus = captureStatusQuery.data?.data
  const onCaptureCompleted = props.onCaptureCompleted

  useEffect(() => {
    if (!captureStatus || captureStatus.status !== 'completed') return
    if (completedCaptureRef.current === captureStatus.capture_id) return
    completedCaptureRef.current = captureStatus.capture_id
    toast.success(t('Upstream login state captured'))
    onCaptureCompleted?.(captureStatus.capture_id)
  }, [captureStatus, onCaptureCompleted, t])

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
            <FormItem>
              <FormLabel>{t('Authentication method')}</FormLabel>
              <Select
                value={field.value}
                onValueChange={(value) => {
                  field.onChange(value)
                  if (value !== 'password') {
                    form.setValue('platform_site_username', '')
                    form.setValue('platform_site_password', '')
                  }
                  if (value !== 'access_token') {
                    form.setValue('platform_site_access_token', '')
                  }
                  if (value !== 'admin_key') {
                    form.setValue('platform_site_admin_key', '')
                  }
                  if (value !== 'cookie') {
                    form.setValue('platform_site_cookie', '')
                  }
                  form.setValue('platform_site_capture_id', '')
                  completedCaptureRef.current = ''
                }}
                items={[
                  { value: 'password', label: t('Username and password') },
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
                  <SelectItem value='access_token'>
                    {t('Access token')}
                  </SelectItem>
                  <SelectItem value='admin_key'>{t('Admin Key')}</SelectItem>
                  <SelectItem value='cookie'>{t('Cookie')}</SelectItem>
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
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

      {authType === 'password' && (
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
      )}

      {authType !== 'password' && (
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
              'Open the upstream site and let the Capture Helper collect the selected authentication method. Tokens, cookies, and Admin Keys are never entered here.'
            )}
          </p>
          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              size='sm'
              onClick={() => captureMutation.mutate()}
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
                {t('Authentication method')}: {captureStatus.summary.auth_type}
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
