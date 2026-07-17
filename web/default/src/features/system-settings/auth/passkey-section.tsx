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

type AttachmentSelectValue = 'none' | 'platform' | 'cross-platform'
type AttachmentStoredValue = '' | 'platform' | 'cross-platform'

// 这里使用嵌套的 passkey 对象，让 FormField 的 dotted path 与
// react-hook-form 的路径语义保持一致，避免旧实现里 flat key 与嵌套状态分离。
const passkeySchema = z.object({
  passkey: z.object({
    enabled: z.boolean(),
    rp_display_name: z.string(),
    rp_id: z.string(),
    origins: z.string(),
    allow_insecure_origin: z.boolean(),
    user_verification: z.enum(['required', 'preferred', 'discouraged']),
    attachment_preference: z.enum(['none', 'platform', 'cross-platform']),
  }),
})

type PasskeyFormValues = z.infer<typeof passkeySchema>

type FlatPasskeyDefaults = {
  'passkey.enabled': boolean
  'passkey.rp_display_name': string
  'passkey.rp_id': string
  'passkey.origins': string
  'passkey.allow_insecure_origin': boolean
  'passkey.user_verification': 'required' | 'preferred' | 'discouraged'
  'passkey.attachment_preference': AttachmentSelectValue
}

type NormalizedPasskeyValues = {
  'passkey.enabled': boolean
  'passkey.rp_display_name': string
  'passkey.rp_id': string
  'passkey.origins': string
  'passkey.allow_insecure_origin': boolean
  'passkey.user_verification': 'required' | 'preferred' | 'discouraged'
  'passkey.attachment_preference': AttachmentStoredValue
}

const passkeyUpdateOrder: Array<keyof NormalizedPasskeyValues> = [
  'passkey.enabled',
  'passkey.rp_display_name',
  'passkey.rp_id',
  'passkey.user_verification',
  'passkey.attachment_preference',
  'passkey.allow_insecure_origin',
  'passkey.origins',
]

function formatPasskeyOriginsForForm(value: string): string {
  return value
    .split(',')
    .map((origin) => origin.trim())
    .filter(Boolean)
    .join('\n')
}

function formatPasskeyOriginsForSave(value: string): string {
  return value
    .split('\n')
    .map((origin) => origin.trim())
    .filter(Boolean)
    .join(',')
}

function toAttachmentStoredValue(
  value: AttachmentSelectValue
): AttachmentStoredValue {
  return value === 'none' ? '' : value
}

function toAttachmentSelectValue(
  value: AttachmentStoredValue
): AttachmentSelectValue {
  return value === '' ? 'none' : value
}

function buildFormDefaults(defaults: FlatPasskeyDefaults): PasskeyFormValues {
  return {
    passkey: {
      enabled: defaults['passkey.enabled'],
      rp_display_name: defaults['passkey.rp_display_name'] ?? '',
      rp_id: defaults['passkey.rp_id'] ?? '',
      origins: formatPasskeyOriginsForForm(defaults['passkey.origins'] ?? ''),
      allow_insecure_origin: defaults['passkey.allow_insecure_origin'],
      user_verification: defaults['passkey.user_verification'],
      attachment_preference: defaults['passkey.attachment_preference'],
    },
  }
}

function normalizePasskeyDefaultValues(
  defaults: FlatPasskeyDefaults
): NormalizedPasskeyValues {
  return {
    'passkey.enabled': defaults['passkey.enabled'],
    'passkey.rp_display_name': defaults['passkey.rp_display_name'],
    'passkey.rp_id': defaults['passkey.rp_id'],
    'passkey.origins': formatPasskeyOriginsForSave(
      formatPasskeyOriginsForForm(defaults['passkey.origins'] ?? '')
    ),
    'passkey.allow_insecure_origin': defaults['passkey.allow_insecure_origin'],
    'passkey.user_verification': defaults['passkey.user_verification'],
    'passkey.attachment_preference': toAttachmentStoredValue(
      defaults['passkey.attachment_preference']
    ),
  }
}

function normalizeFormValues(
  values: PasskeyFormValues
): NormalizedPasskeyValues {
  return {
    'passkey.enabled': values.passkey.enabled,
    'passkey.rp_display_name': values.passkey.rp_display_name,
    'passkey.rp_id': values.passkey.rp_id,
    'passkey.origins': formatPasskeyOriginsForSave(values.passkey.origins),
    'passkey.allow_insecure_origin': values.passkey.allow_insecure_origin,
    'passkey.user_verification': values.passkey.user_verification,
    'passkey.attachment_preference': toAttachmentStoredValue(
      values.passkey.attachment_preference
    ),
  }
}

interface PasskeySectionProps {
  defaultValues: FlatPasskeyDefaults
}

export function PasskeySection({ defaultValues }: PasskeySectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )
  const normalizedDefaults = useMemo(
    () => normalizePasskeyDefaultValues(defaultValues),
    [defaultValues]
  )
  const savedValuesRef = useRef<NormalizedPasskeyValues>(normalizedDefaults)
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
    useSettingsForm<PasskeyFormValues>({
      resolver: zodResolver(passkeySchema) as Resolver<
        PasskeyFormValues,
        unknown,
        PasskeyFormValues
      >,
      defaultValues: formDefaults,
      onSubmit: async (data, changedFields) => {
        const normalizedValues = normalizeFormValues(data)
        const savedValues = savedValuesRef.current
        const updates: Array<{
          key: keyof NormalizedPasskeyValues
          value: string | boolean
        }> = []

        for (const key of passkeyUpdateOrder) {
          if (!(key in changedFields)) {
            continue
          }

          if (normalizedValues[key] !== savedValues[key]) {
            updates.push({
              key,
              value: normalizedValues[key],
            })
          }
        }

        for (const update of updates) {
          await updateOption.mutateAsync(update)
        }

        // Origins 在表单里按换行编辑、在后端按逗号存储。保存成功后继续把
        // 规范化后的多行展示格式推进表单基线，避免空白和空行差异残留为脏态。
        data.passkey.origins = formatPasskeyOriginsForForm(
          normalizedValues['passkey.origins']
        )
        data.passkey.attachment_preference = toAttachmentSelectValue(
          normalizedValues['passkey.attachment_preference']
        )
        savedValuesRef.current = normalizedValues
        savedSerializedRef.current = JSON.stringify(normalizedValues)
      },
    })

  return (
    <SettingsSection
      title={t('Passkey Authentication')}
      description={t('Configure Passkey (WebAuthn) login settings')}
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
            saveLabel='Save Changes'
          />
          <FormDirtyIndicator isDirty={isDirty} />

          <FormField
            control={form.control}
            name='passkey.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable Passkey')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Allow users to register and sign in with Passkey (WebAuthn)'
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
            name='passkey.rp_display_name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Relying Party Display Name')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('e.g. NexusTok Console')}
                    value={field.value ?? ''}
                    onChange={(event) => field.onChange(event.target.value)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Human-readable name shown to users during Passkey prompts.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='passkey.rp_id'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Relying Party ID')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('e.g. example.com')}
                    value={field.value ?? ''}
                    onChange={(event) => field.onChange(event.target.value)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'The effective domain for Passkey registration. Must match the current domain or be its parent domain.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='passkey.user_verification'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('User Verification')}</FormLabel>
                <FormControl>
                  <Select
                    items={[
                      { value: 'required', label: t('Required') },
                      { value: 'preferred', label: t('Recommended') },
                      { value: 'discouraged', label: t('Discouraged') },
                    ]}
                    value={field.value}
                    onValueChange={field.onChange}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder={t('Select requirement')} />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem value='required'>
                          {t('Required')}
                        </SelectItem>
                        <SelectItem value='preferred'>
                          {t('Recommended')}
                        </SelectItem>
                        <SelectItem value='discouraged'>
                          {t('Discouraged')}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </FormControl>
                <FormDescription>
                  {t(
                    'Controls whether user verification (biometrics/PIN) is required during Passkey flows.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='passkey.attachment_preference'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Device Type Preference')}</FormLabel>
                <FormControl>
                  <Select
                    items={[
                      { value: 'none', label: t('Unlimited') },
                      { value: 'platform', label: t('Built-in Device') },
                      { value: 'cross-platform', label: t('External Device') },
                    ]}
                    value={field.value}
                    onValueChange={field.onChange}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder={t('No preference')} />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        <SelectItem value='none'>{t('Unlimited')}</SelectItem>
                        <SelectItem value='platform'>
                          {t('Built-in Device')}
                        </SelectItem>
                        <SelectItem value='cross-platform'>
                          {t('External Device')}
                        </SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </FormControl>
                <FormDescription>
                  {t(
                    'Built-in: phone fingerprint/face, or Windows Hello; External: USB security key'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='passkey.allow_insecure_origin'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Allow Insecure Origins')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Permit Passkey registration on non-HTTPS origins (only recommended for development)'
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
            name='passkey.origins'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Allowed Origins')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={4}
                    placeholder={t('https://example.com')}
                    value={field.value ?? ''}
                    onChange={(event) => field.onChange(event.target.value)}
                    name={field.name}
                    onBlur={field.onBlur}
                    ref={field.ref}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'List of origins (one per line) allowed for Passkey registration and authentication.'
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
