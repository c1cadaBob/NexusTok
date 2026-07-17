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
import { useMemo } from 'react'
import type { Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
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

const createEmailSchema = (t: (key: string) => string) =>
  z.object({
    SMTPServer: z.string(),
    SMTPPort: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return /^\d+$/.test(trimmed)
    }, t('Port must be a positive integer')),
    SMTPAccount: z.string(),
    SMTPFrom: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(trimmed)
    }, t('Enter a valid email or leave blank')),
    SMTPToken: z.string(),
    SMTPSSLEnabled: z.boolean(),
    SMTPStartTLSEnabled: z.boolean(),
    SMTPInsecureSkipVerify: z.boolean(),
    SMTPForceAuthLogin: z.boolean(),
  })

type EmailFormValues = z.infer<ReturnType<typeof createEmailSchema>>

type EmailSettingsSectionProps = {
  defaultValues: EmailFormValues
}

const emailUpdateOrder: Array<keyof EmailFormValues> = [
  'SMTPServer',
  'SMTPPort',
  'SMTPAccount',
  'SMTPFrom',
  'SMTPToken',
  'SMTPSSLEnabled',
  'SMTPStartTLSEnabled',
  'SMTPInsecureSkipVerify',
  'SMTPForceAuthLogin',
]

const emailOptionKeyMap: Record<keyof EmailFormValues, string> = {
  SMTPServer: 'SMTPServer',
  SMTPPort: 'SMTPPort',
  SMTPAccount: 'SMTPAccount',
  SMTPFrom: 'SMTPFrom',
  SMTPToken: 'SMTPToken',
  SMTPSSLEnabled: 'SMTPSSLEnabled',
  SMTPStartTLSEnabled: 'SMTPStartTLSEnabled',
  SMTPInsecureSkipVerify: 'SMTPInsecureSkipVerify',
  SMTPForceAuthLogin: 'SMTPForceAuthLogin',
}

function normalizeEmailValues(values: EmailFormValues): EmailFormValues {
  return {
    SMTPServer: values.SMTPServer.trim(),
    SMTPPort: values.SMTPPort.trim(),
    SMTPAccount: values.SMTPAccount.trim(),
    SMTPFrom: values.SMTPFrom.trim(),
    SMTPToken: values.SMTPToken.trim(),
    SMTPSSLEnabled: values.SMTPSSLEnabled,
    SMTPStartTLSEnabled: values.SMTPStartTLSEnabled,
    SMTPInsecureSkipVerify: values.SMTPInsecureSkipVerify,
    SMTPForceAuthLogin: values.SMTPForceAuthLogin,
  }
}

export function EmailSettingsSection({
  defaultValues,
}: EmailSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const emailSchema = createEmailSchema(t)
  const formDefaults = useMemo<EmailFormValues>(
    () => ({
      ...defaultValues,
      // SMTPToken 是敏感凭据：后端已有值不应成为前端表单基线。
      // 留空表示保留现有凭据，只有管理员输入新 token 时才提交更新。
      SMTPToken: '',
    }),
    [defaultValues]
  )

  const {
    form,
    handleSubmit,
    handleReset,
    isDirty,
    isSubmitting,
  } = useSettingsForm<EmailFormValues>({
    resolver: zodResolver(emailSchema) as Resolver<
      EmailFormValues,
      unknown,
      EmailFormValues
    >,
    defaultValues: formDefaults,
    onSubmit: async (values, changedFields) => {
      const sanitized = normalizeEmailValues(values)
      const initial = normalizeEmailValues(defaultValues)
      const updates: Array<{ key: string; value: string | boolean }> = []

      for (const key of emailUpdateOrder) {
        if (!(key in changedFields)) {
          continue
        }

        if (key === 'SMTPToken') {
          if (!sanitized.SMTPToken || sanitized.SMTPToken === initial.SMTPToken) {
            continue
          }
        } else if (sanitized[key] === initial[key]) {
          continue
        }

        updates.push({
          key: emailOptionKeyMap[key],
          value: sanitized[key],
        })
      }

      Object.assign(values, {
        ...sanitized,
        SMTPToken: '',
      })

      if (updates.length === 0) {
        toast.info(t('No changes to save'))
        return
      }

      for (const update of updates) {
        await updateOption.mutateAsync(update)
      }
    },
  })

  return (
    <SettingsSection
      title={t('SMTP Email')}
      description={t('Configure outgoing email server for notifications')}
    >
      <FormNavigationGuard when={isDirty} />

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit} autoComplete='off'>
          <SettingsPageFormActions
            onSave={handleSubmit}
            onReset={handleReset}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty || !updateOption.canUpdate}
            isResetDisabled={!isDirty}
            saveDisabledReason={
              updateOption.canUpdate ? undefined : updateOption.disabledReason
            }
            saveLabel='Save SMTP settings'
          />
          <FormDirtyIndicator isDirty={isDirty} />

          <FormField
            control={form.control}
            name='SMTPServer'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('SMTP Host')}</FormLabel>
                <FormControl>
                  <Input
                    autoComplete='off'
                    placeholder={t('smtp.example.com')}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Hostname or IP of your SMTP provider')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='SMTPPort'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Port')}</FormLabel>
                  <FormControl>
                    <Input
                      autoComplete='off'
                      type='number'
                      placeholder='587'
                      {...field}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Common ports include 25, 465, and 587')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='SMTPSSLEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Enable SSL/TLS')}</FormLabel>
                    <FormDescription>
                      {t('Use secure connection when sending emails')}
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
              name='SMTPStartTLSEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Use STARTTLS')}</FormLabel>
                    <FormDescription>
                      {t('Require STARTTLS upgrade for non-SSL SMTP connections')}
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
              name='SMTPInsecureSkipVerify'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>
                      {t('Skip SMTP TLS certificate verification')}
                    </FormLabel>
                    <FormDescription>
                      {t('Allow self-signed or mismatched SMTP certificates')}
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
              name='SMTPForceAuthLogin'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Force AUTH LOGIN')}</FormLabel>
                    <FormDescription>
                      {t('Force SMTP authentication using AUTH LOGIN method')}
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
          </div>

          <FormField
            control={form.control}
            name='SMTPAccount'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Username')}</FormLabel>
                <FormControl>
                  <Input
                    autoComplete='off'
                    placeholder={t('noreply@example.com')}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Account used when authenticating with the SMTP server')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='SMTPFrom'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('From Address')}</FormLabel>
                <FormControl>
                  <Input
                    autoComplete='off'
                    placeholder={t('NexusTok &lt;noreply@example.com&gt;')}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Display name and email used in outgoing messages')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='SMTPToken'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Password / Access Token')}</FormLabel>
                <FormControl>
                  <Input
                    autoComplete='off'
                    type='password'
                    placeholder={t('Enter new token to update')}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Leave blank to keep the existing credential')}
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
