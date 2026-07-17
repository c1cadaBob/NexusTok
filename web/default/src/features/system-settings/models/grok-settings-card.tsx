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
import * as z from 'zod'
import { useEffect, useMemo, useRef } from 'react'
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
import { safeNumberFieldProps } from '../utils/numeric-field'
import {
  buildGrokFormDefaults,
  getChangedGrokSettingKeys,
  normalizeGrokFormValues,
  type FlatGrokSettings,
  type GrokFormInput,
  type GrokFormValues,
} from './grok-settings-utils'

const XAI_VIOLATION_FEE_DOC_URL =
  'https://docs.x.ai/docs/models#usage-guidelines-violation-fee'

// react-hook-form 会把 dotted name 当作路径解析。这里必须使用嵌套 schema，
// 保存前再转回后端 option 使用的扁平 key，避免表单状态和提交值分裂。
const grokSchema = z.object({
  grok: z.object({
    violation_deduction_enabled: z.boolean(),
    violation_deduction_amount: z.coerce.number().min(0),
  }),
})

interface Props {
  defaultValues: FlatGrokSettings
}

const grokUpdateOrder: Array<keyof FlatGrokSettings> = [
  'grok.violation_deduction_enabled',
  'grok.violation_deduction_amount',
]

export function GrokSettingsCard(props: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const formDefaults = useMemo(
    () => buildGrokFormDefaults(props.defaultValues),
    [props.defaultValues]
  )
  const baselineRef = useRef<FlatGrokSettings>(props.defaultValues)
  const baselineSerializedRef = useRef<string>(
    JSON.stringify(props.defaultValues)
  )

  const {
    form,
    handleSubmit,
    handleReset,
    isDirty,
    isSubmitting,
  } = useSettingsForm<GrokFormInput>({
    resolver: zodResolver(grokSchema) as Resolver<
      GrokFormInput,
      unknown,
      GrokFormInput
    >,
    defaultValues: formDefaults,
    onSubmit: async (values) => {
      const normalized = normalizeGrokFormValues(values as GrokFormValues)
      const updates = getChangedGrokSettingKeys(
        normalized,
        baselineRef.current
      )

      for (const key of grokUpdateOrder) {
        if (!updates.includes(key)) {
          continue
        }

        await updateOption.mutateAsync({
          key,
          value: normalized[key],
        })
      }

      baselineRef.current = normalized
      baselineSerializedRef.current = JSON.stringify(normalized)
      // 保存成功后把后端扁平 key 重新映射为嵌套表单基线，避免 dotted key
      // 与 react-hook-form 路径语义再次分裂。
      form.reset(buildGrokFormDefaults(normalized))
    },
  })

  useEffect(() => {
    const serialized = JSON.stringify(props.defaultValues)
    if (serialized === baselineSerializedRef.current) return

    baselineRef.current = props.defaultValues
    baselineSerializedRef.current = serialized
  }, [props.defaultValues])

  const enabled = form.watch('grok.violation_deduction_enabled')

  return (
    <SettingsSection
      title={t('Grok Settings')}
      description={t('Configure xAI Grok model specific settings')}
    >
      <FormNavigationGuard when={isDirty} />

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsPageFormActions
            onSave={handleSubmit}
            onReset={handleReset}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty || !updateOption.canUpdate}
            isResetDisabled={!isDirty}
            saveDisabledReason={
              updateOption.canUpdate ? undefined : updateOption.disabledReason
            }
          />
          <FormDirtyIndicator isDirty={isDirty} />

          <FormField
            control={form.control}
            name='grok.violation_deduction_enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable violation deduction')}</FormLabel>
                  <FormDescription>
                    {t(
                      'When enabled, violation requests will incur additional charges.'
                    )}{' '}
                    <a
                      href={XAI_VIOLATION_FEE_DOC_URL}
                      target='_blank'
                      rel='noreferrer'
                      className='underline'
                    >
                      {t('Official documentation')}
                    </a>
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
            name='grok.violation_deduction_amount'
            render={({ field }) => (
              <FormItem className='max-w-xs'>
                <FormLabel>{t('Violation deduction amount')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    step={0.01}
                    min={0}
                    {...safeNumberFieldProps(field)}
                    disabled={!enabled}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Base amount. Actual deduction = base amount × system group rate.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
