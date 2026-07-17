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
import { useEffect, useMemo, useRef } from 'react'
import * as z from 'zod'
import type { Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
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
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { removeTrailingSlash } from './utils'

const createWorkerSchema = (t: (key: string) => string) =>
  z.object({
    WorkerUrl: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return /^https?:\/\//.test(trimmed)
    }, t('Provide a valid URL starting with http:// or https://')),
    WorkerValidKey: z.string(),
    WorkerAllowHttpImageRequestEnabled: z.boolean(),
  })

type WorkerFormValues = z.infer<ReturnType<typeof createWorkerSchema>>

type WorkerSettingsSectionProps = {
  defaultValues: WorkerFormValues
}

type NormalizedWorkerValues = {
  WorkerUrl: string
  WorkerValidKey: string
  WorkerAllowHttpImageRequestEnabled: boolean
}

function normalizeWorkerValues(
  values: WorkerFormValues
): NormalizedWorkerValues {
  return {
    WorkerUrl: removeTrailingSlash(values.WorkerUrl),
    WorkerValidKey: values.WorkerValidKey.trim(),
    WorkerAllowHttpImageRequestEnabled:
      values.WorkerAllowHttpImageRequestEnabled,
  }
}

export function WorkerSettingsSection({
  defaultValues,
}: WorkerSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const workerSchema = createWorkerSchema(t)
  const normalizedDefaults = useMemo(
    () => normalizeWorkerValues(defaultValues),
    [defaultValues]
  )
  const savedValuesRef = useRef<NormalizedWorkerValues>(normalizedDefaults)
  const savedSerializedRef = useRef<string>(
    JSON.stringify(normalizedDefaults)
  )

  useEffect(() => {
    const serialized = JSON.stringify(normalizedDefaults)
    if (serialized === savedSerializedRef.current) return
    savedValuesRef.current = normalizedDefaults
    savedSerializedRef.current = serialized
  }, [normalizedDefaults])

  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<WorkerFormValues>({
      resolver: zodResolver(workerSchema) as Resolver<
        WorkerFormValues,
        unknown,
        WorkerFormValues
      >,
      defaultValues,
      onSubmit: async (values) => {
        const sanitizedUrl = removeTrailingSlash(values.WorkerUrl)
        const sanitizedKey = values.WorkerValidKey.trim()
        const savedValues = savedValuesRef.current

        const updates: Array<{ key: string; value: string | boolean }> = []

        if (sanitizedUrl !== savedValues.WorkerUrl) {
          updates.push({ key: 'WorkerUrl', value: sanitizedUrl })
        }

        if (
          sanitizedKey !== savedValues.WorkerValidKey ||
          sanitizedUrl === ''
        ) {
          updates.push({ key: 'WorkerValidKey', value: sanitizedKey })
        }

        if (
          values.WorkerAllowHttpImageRequestEnabled !==
          savedValues.WorkerAllowHttpImageRequestEnabled
        ) {
          updates.push({
            key: 'WorkerAllowHttpImageRequestEnabled',
            value: values.WorkerAllowHttpImageRequestEnabled,
          })
        }

        for (const update of updates) {
          await updateOption.mutateAsync(update)
        }

        const nextSavedValues: NormalizedWorkerValues = {
          WorkerUrl: sanitizedUrl,
          // Worker 密钥不会回显到页面；保存成功后继续以空字符串作为“保留现有 secret”
          // 的基线，避免用户后续把输入框清空时被误判为要主动清空后端密钥。
          WorkerValidKey: '',
          WorkerAllowHttpImageRequestEnabled:
            values.WorkerAllowHttpImageRequestEnabled,
        }

        savedValuesRef.current = nextSavedValues
        savedSerializedRef.current = JSON.stringify(nextSavedValues)
        values.WorkerUrl = sanitizedUrl
        values.WorkerValidKey = ''
      },
    })

  return (
    <SettingsSection
      title={t('Worker Proxy')}
      description={t(
        'Configure upstream worker or proxy service for outbound requests'
      )}
    >
      <FormNavigationGuard when={isDirty} />

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit} autoComplete='off'>
          <SettingsPageFormActions
            onSave={handleSubmit}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty || !updateOption.canUpdate}
            saveDisabledReason={
              updateOption.canUpdate ? undefined : updateOption.disabledReason
            }
            saveLabel='Save Worker settings'
          />
          <FormDirtyIndicator isDirty={isDirty} />

          <FormField
            control={form.control}
            name='WorkerUrl'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Worker URL')}</FormLabel>
                <FormControl>
                  <Input
                    type='url'
                    inputMode='url'
                    placeholder={t('https://worker.example.workers.dev')}
                    autoComplete='off'
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Requests will be forwarded to this worker. Trailing slashes are removed automatically.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='WorkerValidKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Worker Access Key')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    placeholder={t('Enter new key to update')}
                    autoComplete='new-password'
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Used to authenticate with the worker. Leave blank to keep the existing secret.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='WorkerAllowHttpImageRequestEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel className='text-base'>
                    {t('Allow HTTP image requests')}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'Enable when proxying workers that fetch images over HTTP.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={updateOption.isPending || isSubmitting}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
