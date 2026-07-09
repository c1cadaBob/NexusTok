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
import { useEffect } from 'react'
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
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

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
  const form = useForm<TokenLimitFormValues>({
    resolver: zodResolver(tokenLimitSchema),
    mode: 'onChange',
    defaultValues: buildFormDefaults(defaultValues),
  })

  useEffect(() => {
    form.reset(buildFormDefaults(defaultValues))
  }, [defaultValues, form])

  const onSubmit = async (values: TokenLimitFormValues) => {
    const key = 'token_setting.max_user_tokens'
    const normalized = normalizeFormValues(values)
    const value = normalized[key]

    if (value === defaultValues[key]) {
      toast.info(t('No changes to save'))
      return
    }

    await updateOption.mutateAsync({ key, value })
  }

  return (
    <SettingsSection
      title={t('Token Limits')}
      description={t(
        'Set how many API tokens each user can create before new token creation is blocked.'
      )}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-6'>
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
                    {...field}
                    onChange={(event) =>
                      field.onChange(Number.parseInt(event.target.value) || 1)
                    }
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

          <Button
            type='submit'
            disabled={updateOption.isPending || !updateOption.canUpdate}
            title={
              updateOption.canUpdate ? undefined : updateOption.disabledReason
            }
          >
            {updateOption.isPending ? t('Saving...') : t('Save token limits')}
          </Button>
        </form>
      </Form>
    </SettingsSection>
  )
}
