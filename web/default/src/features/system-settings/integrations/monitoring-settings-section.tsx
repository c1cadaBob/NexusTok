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
import { useMemo, useRef } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { parseHttpStatusCodeRules } from '@/lib/http-status-code-rules'
import { Button } from '@/components/ui/button'
import {
  Form,
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
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'
import {
  UPSTREAM_ACCOUNT_SYNC_UNITS,
  buildUpstreamAccountSyncFormDefaults,
  buildUpstreamAccountSyncPersistedDefaults,
  formatUpstreamAccountSyncDescription,
  normalizeUpstreamAccountSyncInterval,
  normalizeUpstreamAccountSyncUnit,
} from './upstream-account-sync-settings'

const numericString = z.string().refine((value) => {
  const trimmed = value.trim()
  if (!trimmed) return true
  return !Number.isNaN(Number(trimmed)) && Number(trimmed) >= 0
}, 'Enter a non-negative number or leave empty')

const monitoringSchema = z
  .object({
    ChannelDisableThreshold: numericString,
    QuotaRemindThreshold: numericString,
    AutomaticDisableChannelEnabled: z.boolean(),
    AutomaticEnableChannelEnabled: z.boolean(),
    AutomaticDisableKeywords: z.string(),
    AutomaticDisableStatusCodes: z.string(),
    AutomaticRetryStatusCodes: z.string(),
    monitor_setting: z.object({
      auto_test_channel_enabled: z.boolean(),
      auto_test_channel_minutes: z.coerce
        .number()
        .int()
        .min(1, 'Interval must be at least 1 minute'),
    }),
    upstream_account_sync: z.object({
      enabled: z.boolean(),
      interval: z.coerce
        .number()
        .int()
        .min(1, 'Sync interval must be at least 1'),
      unit: z.enum(UPSTREAM_ACCOUNT_SYNC_UNITS),
    }),
    system_task_setting: z.object({
      async_task_poll_enabled: z.boolean(),
      midjourney_poll_enabled: z.boolean(),
      subscription_maintenance_enabled: z.boolean(),
      models_dev_sync_enabled: z.boolean(),
      models_dev_sync_time: z
        .string()
        .regex(/^([01]\d|2[0-3]):[0-5]\d$/, 'Use HH:mm'),
    }),
  })
  .superRefine((values, ctx) => {
    const disableParsed = parseHttpStatusCodeRules(
      values.AutomaticDisableStatusCodes
    )
    if (!disableParsed.ok) {
      ctx.addIssue({
        code: 'custom',
        path: ['AutomaticDisableStatusCodes'],
        message: `Invalid status code rules: ${disableParsed.invalidTokens.join(
          ', '
        )}`,
      })
    }

    const retryParsed = parseHttpStatusCodeRules(
      values.AutomaticRetryStatusCodes
    )
    if (!retryParsed.ok) {
      ctx.addIssue({
        code: 'custom',
        path: ['AutomaticRetryStatusCodes'],
        message: `Invalid status code rules: ${retryParsed.invalidTokens.join(
          ', '
        )}`,
      })
    }
  })

type MonitoringFormValues = z.output<typeof monitoringSchema>
type MonitoringFormInput = z.input<typeof monitoringSchema>

type MonitoringSettingsSectionProps = {
  defaultValues: {
    ChannelDisableThreshold: string
    QuotaRemindThreshold: string
    AutomaticDisableChannelEnabled: boolean
    AutomaticEnableChannelEnabled: boolean
    AutomaticDisableKeywords: string
    AutomaticDisableStatusCodes: string
    AutomaticRetryStatusCodes: string
    'monitor_setting.auto_test_channel_enabled': boolean
    'monitor_setting.auto_test_channel_minutes': number
    'upstream_account_sync.enabled': boolean
    'upstream_account_sync.interval': number
    'upstream_account_sync.unit': string
    'system_task_setting.async_task_poll_enabled': boolean
    'system_task_setting.midjourney_poll_enabled': boolean
    'system_task_setting.subscription_maintenance_enabled': boolean
    'system_task_setting.models_dev_sync_enabled': boolean
    'system_task_setting.models_dev_sync_time': string
  }
}

function normalizeLineEndings(value: string) {
  return value.replace(/\r\n/g, '\n')
}

type NormalizedMonitoringValues = {
  ChannelDisableThreshold: string
  QuotaRemindThreshold: string
  AutomaticDisableChannelEnabled: boolean
  AutomaticEnableChannelEnabled: boolean
  AutomaticDisableKeywords: string
  AutomaticDisableStatusCodes: string
  AutomaticRetryStatusCodes: string
  'monitor_setting.auto_test_channel_enabled': boolean
  'monitor_setting.auto_test_channel_minutes': number
  'upstream_account_sync.enabled': boolean
  'upstream_account_sync.interval': number
  'upstream_account_sync.unit': string
  'system_task_setting.async_task_poll_enabled': boolean
  'system_task_setting.midjourney_poll_enabled': boolean
  'system_task_setting.subscription_maintenance_enabled': boolean
  'system_task_setting.models_dev_sync_enabled': boolean
  'system_task_setting.models_dev_sync_time': string
}

const buildFormDefaults = (
  defaults: MonitoringSettingsSectionProps['defaultValues']
): MonitoringFormInput => ({
  ChannelDisableThreshold: defaults.ChannelDisableThreshold ?? '',
  QuotaRemindThreshold: defaults.QuotaRemindThreshold ?? '',
  AutomaticDisableChannelEnabled: defaults.AutomaticDisableChannelEnabled,
  AutomaticEnableChannelEnabled: defaults.AutomaticEnableChannelEnabled,
  AutomaticDisableKeywords: normalizeLineEndings(
    defaults.AutomaticDisableKeywords ?? ''
  ),
  AutomaticDisableStatusCodes: defaults.AutomaticDisableStatusCodes ?? '',
  AutomaticRetryStatusCodes: defaults.AutomaticRetryStatusCodes ?? '',
  monitor_setting: {
    auto_test_channel_enabled:
      defaults['monitor_setting.auto_test_channel_enabled'],
    auto_test_channel_minutes:
      defaults['monitor_setting.auto_test_channel_minutes'],
  },
  upstream_account_sync: buildUpstreamAccountSyncFormDefaults({
    enabled: defaults['upstream_account_sync.enabled'],
    interval: defaults['upstream_account_sync.interval'],
    unit: defaults['upstream_account_sync.unit'],
  }),
  system_task_setting: {
    async_task_poll_enabled:
      defaults['system_task_setting.async_task_poll_enabled'],
    midjourney_poll_enabled:
      defaults['system_task_setting.midjourney_poll_enabled'],
    subscription_maintenance_enabled:
      defaults['system_task_setting.subscription_maintenance_enabled'],
    models_dev_sync_enabled:
      defaults['system_task_setting.models_dev_sync_enabled'],
    models_dev_sync_time:
      defaults['system_task_setting.models_dev_sync_time'] || '02:00',
  },
})

const normalizeDefaults = (
  defaults: MonitoringSettingsSectionProps['defaultValues']
): NormalizedMonitoringValues => ({
  ChannelDisableThreshold: (defaults.ChannelDisableThreshold ?? '').trim(),
  QuotaRemindThreshold: (defaults.QuotaRemindThreshold ?? '').trim(),
  AutomaticDisableChannelEnabled: defaults.AutomaticDisableChannelEnabled,
  AutomaticEnableChannelEnabled: defaults.AutomaticEnableChannelEnabled,
  AutomaticDisableKeywords: normalizeLineEndings(
    defaults.AutomaticDisableKeywords ?? ''
  ),
  AutomaticDisableStatusCodes: parseHttpStatusCodeRules(
    defaults.AutomaticDisableStatusCodes ?? ''
  ).normalized,
  AutomaticRetryStatusCodes: parseHttpStatusCodeRules(
    defaults.AutomaticRetryStatusCodes ?? ''
  ).normalized,
  'monitor_setting.auto_test_channel_enabled':
    defaults['monitor_setting.auto_test_channel_enabled'],
  'monitor_setting.auto_test_channel_minutes':
    defaults['monitor_setting.auto_test_channel_minutes'],
  'upstream_account_sync.enabled': defaults['upstream_account_sync.enabled'],
  'upstream_account_sync.interval': defaults['upstream_account_sync.interval'],
  'upstream_account_sync.unit': buildUpstreamAccountSyncPersistedDefaults({
    enabled: defaults['upstream_account_sync.enabled'],
    interval: defaults['upstream_account_sync.interval'],
    unit: defaults['upstream_account_sync.unit'],
  }).unit,
  'system_task_setting.async_task_poll_enabled':
    defaults['system_task_setting.async_task_poll_enabled'],
  'system_task_setting.midjourney_poll_enabled':
    defaults['system_task_setting.midjourney_poll_enabled'],
  'system_task_setting.subscription_maintenance_enabled':
    defaults['system_task_setting.subscription_maintenance_enabled'],
  'system_task_setting.models_dev_sync_enabled':
    defaults['system_task_setting.models_dev_sync_enabled'],
  'system_task_setting.models_dev_sync_time':
    (defaults['system_task_setting.models_dev_sync_time'] || '02:00').trim(),
})

const normalizeFormValues = (
  values: MonitoringFormValues
): NormalizedMonitoringValues => ({
  ChannelDisableThreshold: values.ChannelDisableThreshold.trim(),
  QuotaRemindThreshold: values.QuotaRemindThreshold.trim(),
  AutomaticDisableChannelEnabled: values.AutomaticDisableChannelEnabled,
  AutomaticEnableChannelEnabled: values.AutomaticEnableChannelEnabled,
  AutomaticDisableKeywords: normalizeLineEndings(
    values.AutomaticDisableKeywords
  ),
  AutomaticDisableStatusCodes: parseHttpStatusCodeRules(
    values.AutomaticDisableStatusCodes
  ).normalized,
  AutomaticRetryStatusCodes: parseHttpStatusCodeRules(
    values.AutomaticRetryStatusCodes
  ).normalized,
  'monitor_setting.auto_test_channel_enabled':
    values.monitor_setting.auto_test_channel_enabled,
  'monitor_setting.auto_test_channel_minutes':
    values.monitor_setting.auto_test_channel_minutes,
  'upstream_account_sync.enabled': values.upstream_account_sync.enabled,
  'upstream_account_sync.interval': values.upstream_account_sync.interval,
  'upstream_account_sync.unit': values.upstream_account_sync.unit,
  'system_task_setting.async_task_poll_enabled':
    values.system_task_setting.async_task_poll_enabled,
  'system_task_setting.midjourney_poll_enabled':
    values.system_task_setting.midjourney_poll_enabled,
  'system_task_setting.subscription_maintenance_enabled':
    values.system_task_setting.subscription_maintenance_enabled,
  'system_task_setting.models_dev_sync_enabled':
    values.system_task_setting.models_dev_sync_enabled,
  'system_task_setting.models_dev_sync_time':
    values.system_task_setting.models_dev_sync_time.trim(),
})

export function MonitoringSettingsSection({
  defaultValues,
}: MonitoringSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const baselineRef = useRef<NormalizedMonitoringValues>(
    normalizeDefaults(defaultValues)
  )

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<MonitoringFormInput, unknown, MonitoringFormValues>({
    resolver: zodResolver(monitoringSchema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  const autoDisableStatusCodes = form.watch('AutomaticDisableStatusCodes')
  const autoRetryStatusCodes = form.watch('AutomaticRetryStatusCodes')
  const autoDisableParsed = useMemo(
    () => parseHttpStatusCodeRules(autoDisableStatusCodes),
    [autoDisableStatusCodes]
  )
  const autoRetryParsed = useMemo(
    () => parseHttpStatusCodeRules(autoRetryStatusCodes),
    [autoRetryStatusCodes]
  )
  const upstreamAccountSyncEnabled = form.watch('upstream_account_sync.enabled')
  const upstreamAccountSyncInterval = form.watch(
    'upstream_account_sync.interval'
  )
  const upstreamAccountSyncUnit = form.watch('upstream_account_sync.unit')
  const upstreamAccountSyncDescriptionInterval =
    normalizeUpstreamAccountSyncInterval(Number(upstreamAccountSyncInterval))
  const upstreamAccountSyncDescriptionUnit = normalizeUpstreamAccountSyncUnit(
    String(upstreamAccountSyncUnit)
  )
  const modelsDevSyncEnabled = form.watch(
    'system_task_setting.models_dev_sync_enabled'
  )

  const onSubmit = async (values: MonitoringFormValues) => {
    const normalized = normalizeFormValues(values)
    const updates = (
      Object.keys(normalized) as Array<keyof NormalizedMonitoringValues>
    ).filter((key) => normalized[key] !== baselineRef.current[key])

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      const value = normalized[key]
      await updateOption.mutateAsync({
        key,
        value,
      })
    }

    baselineRef.current = normalized
  }

  return (
    <SettingsSection
      title={t('Monitoring & Alerts')}
      description={t(
        'Automatically test channels and notify users when limits are hit'
      )}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-6'>
          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='monitor_setting.auto_test_channel_enabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Scheduled channel tests')}
                    </FormLabel>
                    <FormDescription>
                      {t('Automatically probe all channels in the background')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='monitor_setting.auto_test_channel_minutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Test interval (minutes)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('How frequently the system tests all channels')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='flex flex-col gap-4 rounded-lg border p-4'>
            <div>
              <h3 className='text-base font-medium'>
                {t('Background maintenance tasks')}
              </h3>
              <p className='text-muted-foreground mt-1 text-sm'>
                {t(
                  'Choose which background jobs may run. Existing defaults keep the current behavior.'
                )}
              </p>
            </div>
            <div className='grid gap-3 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='system_task_setting.async_task_poll_enabled'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                    <div className='min-w-0 pr-3'>
                      <FormLabel>{t('Async task polling')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Updates video and Suno task status, timeouts, and billing.'
                        )}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='system_task_setting.midjourney_poll_enabled'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                    <div className='min-w-0 pr-3'>
                      <FormLabel>{t('Drawing task polling')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Updates Midjourney task results and refunds failed tasks.'
                        )}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='system_task_setting.subscription_maintenance_enabled'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                    <div className='min-w-0 pr-3'>
                      <FormLabel>{t('Subscription maintenance')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Expires subscriptions, resets quotas, and cleans pre-consume records.'
                        )}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
              <div className='grid gap-3 sm:grid-cols-[minmax(0,1fr)_8rem] sm:items-start'>
                <FormField
                  control={form.control}
                  name='system_task_setting.models_dev_sync_enabled'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                      <div className='min-w-0 pr-3'>
                        <FormLabel>{t('Models.dev model sync')}</FormLabel>
                        <FormDescription>
                          {t(
                            'Syncs the public model directory once a day without overwriting manual prices.'
                          )}
                        </FormDescription>
                      </div>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='system_task_setting.models_dev_sync_time'
                  render={({ field }) => (
                    <FormItem data-disabled={!modelsDevSyncEnabled}>
                      <FormLabel>{t('Daily sync time')}</FormLabel>
                      <FormControl>
                        <Input
                          type='time'
                          disabled={!modelsDevSyncEnabled}
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>{t('Local server time')}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='upstream_account_sync.enabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Automatic upstream account sync')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Automatically refresh saved upstream account pools in the background'
                      )}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='upstream_account_sync.interval'
                render={({ field }) => (
                  <FormItem data-disabled={!upstreamAccountSyncEnabled}>
                    <FormLabel>{t('Sync interval')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        step={1}
                        disabled={!upstreamAccountSyncEnabled}
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>
                      {upstreamAccountSyncEnabled
                        ? t(
                            'Choose how often eligible upstream account pools are refreshed'
                          )
                        : t(
                            'This setting is disabled; upstream account pools will not be synchronized automatically.'
                          )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='upstream_account_sync.unit'
                render={({ field }) => (
                  <FormItem data-disabled={!upstreamAccountSyncEnabled}>
                    <FormLabel>{t('Sync unit')}</FormLabel>
                    <Select
                      value={field.value}
                      onValueChange={field.onChange}
                      disabled={!upstreamAccountSyncEnabled}
                    >
                      <FormControl>
                        <SelectTrigger
                          className='w-full'
                          aria-label={t('Sync unit')}
                        >
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='month'>{t('Months')}</SelectItem>
                          <SelectItem value='week'>{t('Weeks')}</SelectItem>
                          <SelectItem value='day'>{t('Days')}</SelectItem>
                          <SelectItem value='hour'>{t('Hours')}</SelectItem>
                          <SelectItem value='minute'>{t('Minutes')}</SelectItem>
                          <SelectItem value='second'>{t('Seconds')}</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormDescription>
                      {formatUpstreamAccountSyncDescription(
                        upstreamAccountSyncEnabled,
                        upstreamAccountSyncDescriptionInterval,
                        upstreamAccountSyncDescriptionUnit,
                        t
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='ChannelDisableThreshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Disable threshold (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Automatically disable channels exceeding this response time'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='QuotaRemindThreshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Quota reminder (tokens)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Send email alerts when a user falls below this quota')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='AutomaticDisableChannelEnabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Disable on failure')}
                    </FormLabel>
                    <FormDescription>
                      {t('Automatically disable channels when tests fail')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AutomaticEnableChannelEnabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Re-enable on success')}
                    </FormLabel>
                    <FormDescription>
                      {t('Bring channels back online after successful checks')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='AutomaticDisableKeywords'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Failure keywords')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={6}
                    placeholder={t('one keyword per line')}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'If an upstream error contains any of these keywords (case insensitive), the channel will be disabled automatically.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='AutomaticDisableStatusCodes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Auto-disable status codes')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('e.g. 401, 403, 429, 500-599')}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Accepts comma-separated status codes and inclusive ranges.'
                    )}{' '}
                    {autoDisableParsed.ok &&
                      autoDisableParsed.normalized &&
                      autoDisableParsed.normalized !== field.value.trim() && (
                        <span className='text-muted-foreground'>
                          {t('Normalized:')} {autoDisableParsed.normalized}
                        </span>
                      )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AutomaticRetryStatusCodes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Auto-retry status codes')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('e.g. 401, 403, 429, 500-599')}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Accepts comma-separated status codes and inclusive ranges.'
                    )}{' '}
                    {autoRetryParsed.ok &&
                      autoRetryParsed.normalized &&
                      autoRetryParsed.normalized !== field.value.trim() && (
                        <span className='text-muted-foreground'>
                          {t('Normalized:')} {autoRetryParsed.normalized}
                        </span>
                      )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <Button
            type='submit'
            disabled={updateOption.isPending || !updateOption.canUpdate}
            title={
              updateOption.canUpdate ? undefined : updateOption.disabledReason
            }
          >
            {updateOption.isPending
              ? t('Saving...')
              : t('Save monitoring rules')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
