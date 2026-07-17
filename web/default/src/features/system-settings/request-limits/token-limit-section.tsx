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
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'

const tokenLimitSchema = z.object({
  token_setting: z.object({
    max_user_tokens: z.number().int().min(1),
  }),
})

type TokenLimitFormValues = z.infer<typeof tokenLimitSchema>
type NormalizedTokenLimitValues = {
  'token_setting.max_user_tokens': number
}

type TokenLimitSectionProps = {
  defaultValues: NormalizedTokenLimitValues
}

const buildFormDefaults = (
  defaults: TokenLimitSectionProps['defaultValues']
): TokenLimitFormValues => ({
  token_setting: {
    max_user_tokens: defaults['token_setting.max_user_tokens'],
  },
})

const normalizeFormValues = (
  values: TokenLimitFormValues
): NormalizedTokenLimitValues => ({
  'token_setting.max_user_tokens': values.token_setting.max_user_tokens,
})

export function TokenLimitSection({ defaultValues }: TokenLimitSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<TokenLimitFormValues>({
      resolver: zodResolver(tokenLimitSchema) as Resolver<
        TokenLimitFormValues,
        unknown,
        TokenLimitFormValues
      >,
      mode: 'onChange',
      defaultValues: buildFormDefaults(defaultValues),
      onSubmit: async (values) => {
        const key = 'token_setting.max_user_tokens'
        const normalized = normalizeFormValues(values)
        const value = normalized[key]

        await updateOption.mutateAsync({ key, value })
      },
    })

  return (
    <SettingsSection
      title={t('Token Limits')}
      description={t(
        'Set how many API tokens each user can create before new token creation is blocked.'
      )}
    >
      <FormNavigationGuard when={isDirty} />

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsPageFormActions
            onSave={handleSubmit}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty || !updateOption.canUpdate}
            saveDisabledReason={
              updateOption.canUpdate ? undefined : updateOption.disabledReason
            }
            saveLabel='Save token limits'
          />
          <FormDirtyIndicator isDirty={isDirty} />

          <FormField
            control={form.control}
            name='token_setting.max_user_tokens'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Maximum tokens per user')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min={1}
                    step={1}
                    {...safeNumberFieldProps(field)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Maximum number of API tokens each user can create. Default is 1000. Very large values may reduce search protection for users with many tokens.'
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
