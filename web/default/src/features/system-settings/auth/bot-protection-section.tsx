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
import { useMemo, useState } from 'react'
import * as z from 'zod'
import { useWatch, type Resolver } from 'react-hook-form'
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

const botProtectionSchema = z.object({
  TurnstileCheckEnabled: z.boolean(),
  TurnstileSiteKey: z.string().optional(),
  TurnstileSecretKey: z.string().optional(),
})

type BotProtectionFormValues = z.infer<typeof botProtectionSchema>

type BotProtectionSectionProps = {
  defaultValues: BotProtectionFormValues
}

type NormalizedBotProtectionValues = {
  TurnstileCheckEnabled: boolean
  TurnstileSiteKey: string
  TurnstileSecretKey: string
}

type BotProtectionSavedState = {
  sourceSignature: string
  values: NormalizedBotProtectionValues
}

const botProtectionUpdateOrder: Array<keyof BotProtectionFormValues> = [
  'TurnstileSiteKey',
  'TurnstileSecretKey',
  'TurnstileCheckEnabled',
]

function normalizeBotProtectionValues(
  values: BotProtectionFormValues
): NormalizedBotProtectionValues {
  return {
    TurnstileCheckEnabled: values.TurnstileCheckEnabled,
    TurnstileSiteKey: values.TurnstileSiteKey ?? '',
    TurnstileSecretKey: values.TurnstileSecretKey ?? '',
  }
}

function isSameBotProtectionValues(
  left: NormalizedBotProtectionValues,
  right: NormalizedBotProtectionValues
): boolean {
  return (
    left.TurnstileCheckEnabled === right.TurnstileCheckEnabled &&
    left.TurnstileSiteKey === right.TurnstileSiteKey &&
    left.TurnstileSecretKey === right.TurnstileSecretKey
  )
}

export function BotProtectionSection({
  defaultValues,
}: BotProtectionSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const normalizedDefaults = useMemo(
    () => normalizeBotProtectionValues(defaultValues),
    [defaultValues]
  )
  const defaultValuesSignature = useMemo(
    () => JSON.stringify(normalizedDefaults),
    [normalizedDefaults]
  )
  const [savedState, setSavedState] = useState<BotProtectionSavedState>(
    () => ({
      sourceSignature: defaultValuesSignature,
      values: normalizedDefaults,
    })
  )
  const { form, handleSubmit, isSubmitting } =
    useSettingsForm<BotProtectionFormValues>({
      resolver: zodResolver(botProtectionSchema) as Resolver<
        BotProtectionFormValues,
        unknown,
        BotProtectionFormValues
      >,
      defaultValues,
      onSubmit: async (data, changedFields) => {
        const normalizedValues = normalizeBotProtectionValues(data)

        // 启用 Turnstile 时后端会立即校验站点密钥是否已存在。
        // 因此必须先保存密钥，再保存开关，避免把开关提前打开时触发校验失败。
        for (const key of botProtectionUpdateOrder) {
          if (!(key in changedFields)) {
            continue
          }
          await updateOption.mutateAsync({
            key,
            value: normalizedValues[key],
          })
        }

        const nextSavedValues: NormalizedBotProtectionValues = {
          TurnstileCheckEnabled: normalizedValues.TurnstileCheckEnabled,
          TurnstileSiteKey: '',
          TurnstileSecretKey: '',
        }

        // Turnstile 的站点密钥和密钥都不会通过 /api/option/ 回填到前端。
        // 保存成功后继续把这两个字段恢复为空字符串，并把空值作为基线，
        // 保持“留空表示保留现有值”的既有语义，避免共享表单把密钥误回显或误判为要清空。
        setSavedState({
          sourceSignature: defaultValuesSignature,
          values: nextSavedValues,
        })
        data.TurnstileSiteKey = ''
        data.TurnstileSecretKey = ''
      },
    })
  const [
    watchedTurnstileCheckEnabled,
    watchedTurnstileSiteKey,
    watchedTurnstileSecretKey,
  ] = useWatch({
    control: form.control,
    name: ['TurnstileCheckEnabled', 'TurnstileSiteKey', 'TurnstileSecretKey'],
  })
  const currentValues = useMemo<NormalizedBotProtectionValues>(
    () => ({
      TurnstileCheckEnabled:
        watchedTurnstileCheckEnabled ?? normalizedDefaults.TurnstileCheckEnabled,
      TurnstileSiteKey: watchedTurnstileSiteKey ?? '',
      TurnstileSecretKey: watchedTurnstileSecretKey ?? '',
    }),
    [
      normalizedDefaults.TurnstileCheckEnabled,
      watchedTurnstileCheckEnabled,
      watchedTurnstileSiteKey,
      watchedTurnstileSecretKey,
    ]
  )
  const savedValues = useMemo(
    () =>
      savedState.sourceSignature === defaultValuesSignature
        ? savedState.values
        : normalizedDefaults,
    [defaultValuesSignature, normalizedDefaults, savedState]
  )
  const hasPendingChanges = useMemo(
    () => !isSameBotProtectionValues(currentValues, savedValues),
    [currentValues, savedValues]
  )

  return (
    <SettingsSection
      title={t('Bot Protection')}
      description={t(
        'Protect login and registration with Cloudflare Turnstile'
      )}
    >
      <FormNavigationGuard when={hasPendingChanges} />

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit} autoComplete='off'>
          <SettingsPageFormActions
            onSave={handleSubmit}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!hasPendingChanges || !updateOption.canUpdate}
            saveDisabledReason={
              updateOption.canUpdate ? undefined : updateOption.disabledReason
            }
            saveLabel='Save Changes'
          />
          <FormDirtyIndicator isDirty={hasPendingChanges} />

          <FormField
            control={form.control}
            name='TurnstileCheckEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable Turnstile')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Protect login and registration with Cloudflare Turnstile'
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
            name='TurnstileSiteKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Site Key')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Your Turnstile site key')}
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='TurnstileSecretKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Secret Key')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    placeholder={t('Your Turnstile secret key')}
                    autoComplete='new-password'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
