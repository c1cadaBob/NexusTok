/*
Copyright (C) 2023-2026 c1cada

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or (at your
option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@c1cada.dev
*/
import * as z from 'zod'
import { useMemo } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { Switch } from '@/components/ui/switch'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'
import { ChannelAffinitySection } from '../general/channel-affinity'
import type { ChannelAffinitySettings } from '../general/channel-affinity/types'

const ttftSchema = z.object({
  routing_ttft_setting: z.object({
    enabled: z.boolean(),
    apply_to_affinity: z.boolean(),
    threshold_ms: z.number().int().min(1),
    cooldown_seconds: z.number().int().min(1),
    min_samples: z.number().int().min(1),
  }),
})

type TTFTFormValues = z.infer<typeof ttftSchema>

export type KeySwitchingSettings = ChannelAffinitySettings & {
  'routing_ttft_setting.enabled': boolean
  'routing_ttft_setting.apply_to_affinity': boolean
  'routing_ttft_setting.threshold_ms': number
  'routing_ttft_setting.cooldown_seconds': number
  'routing_ttft_setting.min_samples': number
}

type Props = KeySwitchingSettings

function buildDefaults(settings: Props): TTFTFormValues {
  return {
    routing_ttft_setting: {
      enabled: settings['routing_ttft_setting.enabled'] ?? true,
      apply_to_affinity: settings['routing_ttft_setting.apply_to_affinity'] ?? true,
      threshold_ms: settings['routing_ttft_setting.threshold_ms'] ?? 800,
      cooldown_seconds: settings['routing_ttft_setting.cooldown_seconds'] ?? 90,
      min_samples: settings['routing_ttft_setting.min_samples'] ?? 2,
    },
  }
}

export function KeySwitchingSection(settings: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const formDefaults = useMemo(() => buildDefaults(settings), [settings])
  const affinityDefaults = useMemo<ChannelAffinitySettings>(
    () => ({
      'channel_affinity_setting.enabled':
        settings['channel_affinity_setting.enabled'],
      'channel_affinity_setting.switch_on_success':
        settings['channel_affinity_setting.switch_on_success'],
      'channel_affinity_setting.keep_on_channel_disabled':
        settings['channel_affinity_setting.keep_on_channel_disabled'],
      'channel_affinity_setting.max_entries':
        settings['channel_affinity_setting.max_entries'],
      'channel_affinity_setting.default_ttl_seconds':
        settings['channel_affinity_setting.default_ttl_seconds'],
      'channel_affinity_setting.max_request_interval_seconds':
        settings['channel_affinity_setting.max_request_interval_seconds'],
      'channel_affinity_setting.rules':
        settings['channel_affinity_setting.rules'],
    }),
    [settings]
  )
  const {
    form,
    handleSubmit,
    handleReset,
    isDirty,
    isSubmitting,
  } = useSettingsForm<TTFTFormValues>({
    resolver: zodResolver(ttftSchema),
    defaultValues: formDefaults,
    onSubmit: async (_values, changedFields) => {
      for (const [key, value] of Object.entries(changedFields)) {
        await updateOption.mutateAsync({
          key,
          value: value as boolean | number,
        })
      }
    },
  })

  const ttftEnabled = form.watch('routing_ttft_setting.enabled')

  return (
    <div className='flex flex-col gap-6'>
      <Alert>
        <AlertDescription>
          {t(
            'Configure channel affinity and candidate first-token latency protection.'
          )}
        </AlertDescription>
      </Alert>

      <div className='flex flex-col gap-4'>
        <div className='flex flex-col gap-1'>
          <h3 className='text-base font-semibold'>{t('Channel Affinity')}</h3>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Reuse the last successful key when the request matches an affinity rule.'
            )}
          </p>
        </div>
        <ChannelAffinitySection defaultValues={affinityDefaults} />
      </div>

      <div className='flex flex-col gap-4 border-t pt-6'>
        <div className='flex flex-col gap-1'>
          <h3 className='text-base font-semibold'>
            {t('Candidate first-token latency cooling')}
          </h3>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Temporarily avoid candidates with repeated slow first-token latency while keeping a fallback available.'
            )}
          </p>
        </div>

        <FormNavigationGuard when={isDirty} />
        <Form {...form}>
          <SettingsForm onSubmit={handleSubmit}>
            <SettingsPageFormActions
              onSave={handleSubmit}
              onReset={handleReset}
              isSaving={updateOption.isPending || isSubmitting}
              isSaveDisabled={!isDirty || !updateOption.canUpdate}
              isResetDisabled={!isDirty}
              saveLabel='Save TTFT settings'
              saveDisabledReason={
                updateOption.canUpdate
                  ? undefined
                  : updateOption.disabledReason
              }
            />
            <FormDirtyIndicator isDirty={isDirty} />

            <FormField
              control={form.control}
              name='routing_ttft_setting.enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable TTFT protection')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Keep slow candidates out of normal routing during cooling.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='routing_ttft_setting.apply_to_affinity'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>
                      {t('Apply TTFT cooling to key switching')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Also filter cooled candidates during channel affinity hits.'
                      )}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={!ttftEnabled}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='routing_ttft_setting.threshold_ms'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('TTFT threshold (ms)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!ttftEnabled}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'A candidate enters cooling after the configured minimum number of samples reaches the latency threshold.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='routing_ttft_setting.cooldown_seconds'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Cooling duration (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!ttftEnabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='routing_ttft_setting.min_samples'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Minimum samples')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                      disabled={!ttftEnabled}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </SettingsForm>
        </Form>
      </div>
    </div>
  )
}
