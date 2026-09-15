import { useEffect } from 'react'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import type { ChannelFormValues } from '../../lib'
import type { UpstreamSiteStatus } from '../../types'

type PlatformSiteFieldsProps = {
  disabled: boolean
  isEditing: boolean
  syncStatus?: UpstreamSiteStatus
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
  const authType = useWatch({
    control: form.control,
    name: 'platform_site_auth_type',
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
                onValueChange={field.onChange}
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
                onValueChange={field.onChange}
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
                'HTTPS is required by default. Private and local addresses are blocked.'
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
                <FormDescription>
                  {t('Leave blank when editing to keep the saved credential.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
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

      <div className='grid gap-4 sm:grid-cols-3'>
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
        <div className='space-y-2'>
          <FormField
            control={form.control}
            name='platform_site_conversion_ratio'
            render={({ field }) => {
              let ratioDescription = t(
                'Enter recharge and credited amounts to calculate the ratio.'
              )
              if (previewRatio !== undefined) {
                if (ratioIsOverridden) {
                  ratioDescription = t('Manual ratio override: {{ratio}}.', {
                    ratio: conversionRatio.toFixed(3),
                  })
                } else {
                  ratioDescription = t(
                    'Automatic ratio: {{ratio}}. Edit this field to override.',
                    { ratio: previewRatio.toFixed(3) }
                  )
                }
              }

              return (
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
                            normalizePlatformNumber(
                              event.target.valueAsNumber
                            )
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
                  <FormDescription>{ratioDescription}</FormDescription>
                  <FormMessage />
                </FormItem>
              )
            }}
          />
          <FormField
            control={form.control}
            name='platform_site_conversion_ratio_override'
            render={({ field }) => (
              <FormItem className='flex items-center justify-between gap-3 rounded-md border p-2'>
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
      </div>
    </fieldset>
  )
}
