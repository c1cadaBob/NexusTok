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
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  UPSTREAM_ACCOUNT_SYNC_UNITS,
  buildUpstreamAccountSyncFormDefaults,
  buildUpstreamAccountSyncPersistedDefaults,
  formatUpstreamAccountSyncDescription,
  normalizeUpstreamAccountSyncInterval,
  normalizeUpstreamAccountSyncUnit,
} from '../integrations/upstream-account-sync-settings'
import { safeNumberFieldProps } from '../utils/numeric-field'

function createSystemTasksSchema(t: (key: string) => string) {
  return z.object({
    upstream_account_sync: z.object({
      enabled: z.boolean(),
      interval: z.coerce
        .number()
        .int()
        .min(1, t('Sync interval must be at least 1')),
      unit: z.enum(UPSTREAM_ACCOUNT_SYNC_UNITS),
      sync_key_models_enabled: z.boolean(),
      key_model_sync_overwrite_manual_enabled: z.boolean(),
    }),
    upstream_account_key_check: z.object({
      enabled: z.boolean(),
      interval_minutes: z.coerce
        .number()
        .int()
        .min(1, t('Check interval must be at least 1 minute')),
      ratio_threshold: z.coerce
        .number()
        .min(0, t('Ratio threshold cannot be negative')),
      failure_threshold: z.coerce
        .number()
        .int()
        .min(1, t('Failure threshold must be at least 1')),
      auto_recover_enabled: z.boolean(),
    }),
    system_task_setting: z.object({
      async_task_poll_enabled: z.boolean(),
      midjourney_poll_enabled: z.boolean(),
      subscription_maintenance_enabled: z.boolean(),
      models_dev_sync_enabled: z.boolean(),
      models_dev_sync_time: z
        .string()
        .regex(/^([01]\d|2[0-3]):[0-5]\d$/, t('Use HH:mm')),
    }),
  })
}

type SystemTasksFormValues = z.output<
  ReturnType<typeof createSystemTasksSchema>
>
type SystemTasksFormInput = z.input<ReturnType<typeof createSystemTasksSchema>>

type SystemTasksSectionProps = {
  defaultValues: {
    'upstream_account_sync.enabled': boolean
    'upstream_account_sync.interval': number
    'upstream_account_sync.unit': string
    'upstream_account_sync.sync_key_models_enabled': boolean
    'upstream_account_sync.key_model_sync_overwrite_manual_enabled': boolean
    'upstream_account_key_check.enabled': boolean
    'upstream_account_key_check.interval_minutes': number
    'upstream_account_key_check.ratio_threshold': number
    'upstream_account_key_check.failure_threshold': number
    'upstream_account_key_check.auto_recover_enabled': boolean
    'system_task_setting.async_task_poll_enabled': boolean
    'system_task_setting.midjourney_poll_enabled': boolean
    'system_task_setting.subscription_maintenance_enabled': boolean
    'system_task_setting.models_dev_sync_enabled': boolean
    'system_task_setting.models_dev_sync_time': string
  }
}

type NormalizedSystemTaskValues = {
  'upstream_account_sync.enabled': boolean
  'upstream_account_sync.interval': number
  'upstream_account_sync.unit': string
  'upstream_account_sync.sync_key_models_enabled': boolean
  'upstream_account_sync.key_model_sync_overwrite_manual_enabled': boolean
  'upstream_account_key_check.enabled': boolean
  'upstream_account_key_check.interval_minutes': number
  'upstream_account_key_check.ratio_threshold': number
  'upstream_account_key_check.failure_threshold': number
  'upstream_account_key_check.auto_recover_enabled': boolean
  'system_task_setting.async_task_poll_enabled': boolean
  'system_task_setting.midjourney_poll_enabled': boolean
  'system_task_setting.subscription_maintenance_enabled': boolean
  'system_task_setting.models_dev_sync_enabled': boolean
  'system_task_setting.models_dev_sync_time': string
}

function buildFormDefaults(
  defaults: SystemTasksSectionProps['defaultValues']
): SystemTasksFormInput {
  return {
    upstream_account_sync: buildUpstreamAccountSyncFormDefaults({
      enabled: defaults['upstream_account_sync.enabled'],
      interval: defaults['upstream_account_sync.interval'],
      unit: defaults['upstream_account_sync.unit'],
      syncKeyModelsEnabled:
        defaults['upstream_account_sync.sync_key_models_enabled'],
      keyModelSyncOverwriteManualEnabled:
        defaults[
          'upstream_account_sync.key_model_sync_overwrite_manual_enabled'
        ],
    }),
    upstream_account_key_check: {
      enabled: defaults['upstream_account_key_check.enabled'],
      interval_minutes:
        defaults['upstream_account_key_check.interval_minutes'] || 30,
      ratio_threshold:
        defaults['upstream_account_key_check.ratio_threshold'] || 0,
      failure_threshold:
        defaults['upstream_account_key_check.failure_threshold'] || 3,
      auto_recover_enabled:
        defaults['upstream_account_key_check.auto_recover_enabled'],
    },
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
  }
}

function normalizeDefaults(
  defaults: SystemTasksSectionProps['defaultValues']
): NormalizedSystemTaskValues {
  const upstream = buildUpstreamAccountSyncPersistedDefaults({
    enabled: defaults['upstream_account_sync.enabled'],
    interval: defaults['upstream_account_sync.interval'],
    unit: defaults['upstream_account_sync.unit'],
    syncKeyModelsEnabled:
      defaults['upstream_account_sync.sync_key_models_enabled'],
    keyModelSyncOverwriteManualEnabled:
      defaults['upstream_account_sync.key_model_sync_overwrite_manual_enabled'],
  })

  return {
    'upstream_account_sync.enabled': upstream.enabled,
    'upstream_account_sync.interval': upstream.interval,
    'upstream_account_sync.unit': upstream.unit,
    'upstream_account_sync.sync_key_models_enabled':
      upstream.syncKeyModelsEnabled,
    'upstream_account_sync.key_model_sync_overwrite_manual_enabled':
      upstream.keyModelSyncOverwriteManualEnabled,
    'upstream_account_key_check.enabled':
      defaults['upstream_account_key_check.enabled'],
    'upstream_account_key_check.interval_minutes':
      defaults['upstream_account_key_check.interval_minutes'] || 30,
    'upstream_account_key_check.ratio_threshold':
      defaults['upstream_account_key_check.ratio_threshold'] || 0,
    'upstream_account_key_check.failure_threshold':
      defaults['upstream_account_key_check.failure_threshold'] || 3,
    'upstream_account_key_check.auto_recover_enabled':
      defaults['upstream_account_key_check.auto_recover_enabled'],
    'system_task_setting.async_task_poll_enabled':
      defaults['system_task_setting.async_task_poll_enabled'],
    'system_task_setting.midjourney_poll_enabled':
      defaults['system_task_setting.midjourney_poll_enabled'],
    'system_task_setting.subscription_maintenance_enabled':
      defaults['system_task_setting.subscription_maintenance_enabled'],
    'system_task_setting.models_dev_sync_enabled':
      defaults['system_task_setting.models_dev_sync_enabled'],
    'system_task_setting.models_dev_sync_time': (
      defaults['system_task_setting.models_dev_sync_time'] || '02:00'
    ).trim(),
  }
}

function normalizeFormValues(
  values: SystemTasksFormValues
): NormalizedSystemTaskValues {
  return {
    'upstream_account_sync.enabled': values.upstream_account_sync.enabled,
    'upstream_account_sync.interval': values.upstream_account_sync.interval,
    'upstream_account_sync.unit': values.upstream_account_sync.unit,
    'upstream_account_sync.sync_key_models_enabled':
      values.upstream_account_sync.sync_key_models_enabled,
    'upstream_account_sync.key_model_sync_overwrite_manual_enabled':
      values.upstream_account_sync.key_model_sync_overwrite_manual_enabled,
    'upstream_account_key_check.enabled':
      values.upstream_account_key_check.enabled,
    'upstream_account_key_check.interval_minutes':
      values.upstream_account_key_check.interval_minutes,
    'upstream_account_key_check.ratio_threshold':
      values.upstream_account_key_check.ratio_threshold,
    'upstream_account_key_check.failure_threshold':
      values.upstream_account_key_check.failure_threshold,
    'upstream_account_key_check.auto_recover_enabled':
      values.upstream_account_key_check.auto_recover_enabled,
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
  }
}

export function SystemTasksSection({ defaultValues }: SystemTasksSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = useMemo(() => createSystemTasksSchema(t), [t])
  const baselineRef = useRef<NormalizedSystemTaskValues>(
    normalizeDefaults(defaultValues)
  )
  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )
  const form = useForm<SystemTasksFormInput, unknown, SystemTasksFormValues>({
    resolver: zodResolver(schema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  const modelsDevSyncEnabled = form.watch(
    'system_task_setting.models_dev_sync_enabled'
  )
  const upstreamAccountSyncEnabled = form.watch('upstream_account_sync.enabled')
  const upstreamAccountSyncInterval = form.watch(
    'upstream_account_sync.interval'
  )
  const upstreamAccountSyncUnit = form.watch('upstream_account_sync.unit')
  const upstreamAccountKeyCheckEnabled = form.watch(
    'upstream_account_key_check.enabled'
  )
  const upstreamAccountSyncDescriptionInterval =
    normalizeUpstreamAccountSyncInterval(Number(upstreamAccountSyncInterval))
  const upstreamAccountSyncDescriptionUnit = normalizeUpstreamAccountSyncUnit(
    String(upstreamAccountSyncUnit)
  )

  const onSubmit = async (values: SystemTasksFormValues) => {
    const normalized = normalizeFormValues(values)
    const updates = (
      Object.keys(normalized) as Array<keyof NormalizedSystemTaskValues>
    ).filter((key) => normalized[key] !== baselineRef.current[key])

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      await updateOption.mutateAsync({
        key,
        value: normalized[key],
      })
    }

    baselineRef.current = normalized
  }

  return (
    <SettingsSection
      title={t('System Tasks')}
      description={t(
        'Configure background jobs and synchronization schedules.'
      )}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit)}
          className='flex flex-col gap-6'
        >
          <div className='flex flex-col gap-4 rounded-lg border p-4'>
            <div className='flex flex-col gap-1'>
              <h3 className='text-base font-medium'>
                {t('Background maintenance tasks')}
              </h3>
              <p className='text-muted-foreground text-sm'>
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
                    <div className='flex min-w-0 flex-col gap-1 pr-3'>
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
                    <div className='flex min-w-0 flex-col gap-1 pr-3'>
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
                    <div className='flex min-w-0 flex-col gap-1 pr-3'>
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
                      <div className='flex min-w-0 flex-col gap-1 pr-3'>
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
                      <FormDescription>
                        {t('Local server time')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>
          </div>

          <div className='flex flex-col gap-4 rounded-lg border p-4'>
            <FormField
              control={form.control}
              name='upstream_account_sync.enabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between'>
                  <div className='flex min-w-0 flex-col gap-1 pr-3'>
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

            <div className='grid gap-3 md:grid-cols-2'>
              <FormField
                control={form.control}
                name='upstream_account_sync.sync_key_models_enabled'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                    <div className='flex min-w-0 flex-col gap-1 pr-3'>
                      <FormLabel>{t('Sync upstream key models')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Update each synced key model list after upstream account sync.'
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
                name='upstream_account_sync.key_model_sync_overwrite_manual_enabled'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                    <div className='flex min-w-0 flex-col gap-1 pr-3'>
                      <FormLabel>
                        {t('Overwrite manually edited key models')}
                      </FormLabel>
                      <FormDescription>
                        {t(
                          'Allow upstream sync to replace local key model allowlists.'
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
            </div>
          </div>

          <div className='flex flex-col gap-4 rounded-lg border p-4'>
            <FormField
              control={form.control}
              name='upstream_account_key_check.enabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between'>
                  <div className='flex min-w-0 flex-col gap-1 pr-3'>
                    <FormLabel className='text-base'>
                      {t('Synced key connection checks')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Automatically test synced upstream keys in the background.'
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

            <div className='grid gap-4 md:grid-cols-3'>
              <FormField
                control={form.control}
                name='upstream_account_key_check.interval_minutes'
                render={({ field }) => (
                  <FormItem data-disabled={!upstreamAccountKeyCheckEnabled}>
                    <FormLabel>{t('Check interval minutes')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        step={1}
                        disabled={!upstreamAccountKeyCheckEnabled}
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('How often synced keys are tested.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='upstream_account_key_check.ratio_threshold'
                render={({ field }) => (
                  <FormItem data-disabled={!upstreamAccountKeyCheckEnabled}>
                    <FormLabel>{t('Ratio threshold')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        step='0.01'
                        disabled={!upstreamAccountKeyCheckEnabled}
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Only test keys whose converted ratio is lower than this value. Leave 0 to test all eligible synced keys.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='upstream_account_key_check.failure_threshold'
                render={({ field }) => (
                  <FormItem data-disabled={!upstreamAccountKeyCheckEnabled}>
                    <FormLabel>{t('Failure threshold')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        step={1}
                        disabled={!upstreamAccountKeyCheckEnabled}
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Disable a synced key after this many failures.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name='upstream_account_key_check.auto_recover_enabled'
              render={({ field }) => (
                <FormItem
                  data-disabled={!upstreamAccountKeyCheckEnabled}
                  className='flex flex-row items-center justify-between rounded-lg border p-3'
                >
                  <div className='flex min-w-0 flex-col gap-1 pr-3'>
                    <FormLabel>{t('Auto recover synced keys')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Restore keys disabled by automatic checks after a successful test.'
                      )}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={!upstreamAccountKeyCheckEnabled}
                    />
                  </FormControl>
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
            {updateOption.isPending ? t('Saving...') : t('Save system tasks')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
