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

const ssrfSchema = z.object({
  fetch_setting: z.object({
    enable_ssrf_protection: z.boolean(),
    allow_private_ip: z.boolean(),
    domain_filter_mode: z.boolean(),
    ip_filter_mode: z.boolean(),
    domain_list: z.string(),
    ip_list: z.string(),
    allowed_ports: z.string(),
    apply_ip_filter_for_domain: z.boolean(),
  }),
})

type SSRFFormValues = z.output<typeof ssrfSchema>
type SSRFFormInput = z.input<typeof ssrfSchema>

type NormalizedSSRFValues = {
  'fetch_setting.enable_ssrf_protection': boolean
  'fetch_setting.allow_private_ip': boolean
  'fetch_setting.domain_filter_mode': boolean
  'fetch_setting.ip_filter_mode': boolean
  'fetch_setting.domain_list': string[]
  'fetch_setting.ip_list': string[]
  'fetch_setting.allowed_ports': number[]
  'fetch_setting.apply_ip_filter_for_domain': boolean
}

type SSRFSectionProps = {
  defaultValues: {
    'fetch_setting.enable_ssrf_protection': boolean
    'fetch_setting.allow_private_ip': boolean
    'fetch_setting.domain_filter_mode': boolean
    'fetch_setting.ip_filter_mode': boolean
    'fetch_setting.domain_list': string[]
    'fetch_setting.ip_list': string[]
    'fetch_setting.allowed_ports': number[]
    'fetch_setting.apply_ip_filter_for_domain': boolean
  }
}

const ssrfUpdateOrder: Array<keyof NormalizedSSRFValues> = [
  'fetch_setting.enable_ssrf_protection',
  'fetch_setting.allow_private_ip',
  'fetch_setting.domain_filter_mode',
  'fetch_setting.domain_list',
  'fetch_setting.ip_filter_mode',
  'fetch_setting.ip_list',
  'fetch_setting.allowed_ports',
  'fetch_setting.apply_ip_filter_for_domain',
]

const joinLines = (values: string[]) => values.join('\n')

const joinPorts = (values: number[]) => values.join(',')

const splitLines = (value: string) =>
  value
    .split('\n')
    .map((entry) => entry.trim())
    .filter(Boolean)

const parsePorts = (value: string) =>
  value
    .split(',')
    .map((item) => Number.parseInt(item.trim(), 10))
    .filter((port) => Number.isFinite(port))

const buildFormDefaults = (
  defaults: SSRFSectionProps['defaultValues']
): SSRFFormInput => ({
  fetch_setting: {
    enable_ssrf_protection: defaults['fetch_setting.enable_ssrf_protection'],
    allow_private_ip: defaults['fetch_setting.allow_private_ip'],
    domain_filter_mode: defaults['fetch_setting.domain_filter_mode'],
    ip_filter_mode: defaults['fetch_setting.ip_filter_mode'],
    domain_list: joinLines(defaults['fetch_setting.domain_list']),
    ip_list: joinLines(defaults['fetch_setting.ip_list']),
    allowed_ports: joinPorts(defaults['fetch_setting.allowed_ports']),
    apply_ip_filter_for_domain:
      defaults['fetch_setting.apply_ip_filter_for_domain'],
  },
})

const normalizeDefaults = (
  defaults: SSRFSectionProps['defaultValues']
): NormalizedSSRFValues => ({
  'fetch_setting.enable_ssrf_protection':
    defaults['fetch_setting.enable_ssrf_protection'],
  'fetch_setting.allow_private_ip': defaults['fetch_setting.allow_private_ip'],
  'fetch_setting.domain_filter_mode':
    defaults['fetch_setting.domain_filter_mode'],
  'fetch_setting.ip_filter_mode': defaults['fetch_setting.ip_filter_mode'],
  'fetch_setting.domain_list': defaults['fetch_setting.domain_list'],
  'fetch_setting.ip_list': defaults['fetch_setting.ip_list'],
  'fetch_setting.allowed_ports': defaults['fetch_setting.allowed_ports'],
  'fetch_setting.apply_ip_filter_for_domain':
    defaults['fetch_setting.apply_ip_filter_for_domain'],
})

const normalizeFormValues = (values: SSRFFormValues): NormalizedSSRFValues => ({
  'fetch_setting.enable_ssrf_protection':
    values.fetch_setting.enable_ssrf_protection,
  'fetch_setting.allow_private_ip': values.fetch_setting.allow_private_ip,
  'fetch_setting.domain_filter_mode': values.fetch_setting.domain_filter_mode,
  'fetch_setting.ip_filter_mode': values.fetch_setting.ip_filter_mode,
  'fetch_setting.domain_list': splitLines(values.fetch_setting.domain_list),
  'fetch_setting.ip_list': splitLines(values.fetch_setting.ip_list),
  'fetch_setting.allowed_ports': parsePorts(values.fetch_setting.allowed_ports),
  'fetch_setting.apply_ip_filter_for_domain':
    values.fetch_setting.apply_ip_filter_for_domain,
})

const isSameSSRFValue = (a: unknown, b: unknown) => {
  if (Array.isArray(a) && Array.isArray(b)) {
    return JSON.stringify(a) === JSON.stringify(b)
  }
  return a === b
}

function applyNormalizedFormValues(
  values: SSRFFormValues,
  normalized: NormalizedSSRFValues
) {
  values.fetch_setting.domain_list = joinLines(
    normalized['fetch_setting.domain_list']
  )
  values.fetch_setting.ip_list = joinLines(normalized['fetch_setting.ip_list'])
  values.fetch_setting.allowed_ports = joinPorts(
    normalized['fetch_setting.allowed_ports']
  )
}

export function SSRFSection({ defaultValues }: SSRFSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )
  const normalizedDefaults = useMemo(
    () => normalizeDefaults(defaultValues),
    [defaultValues]
  )
  const savedValuesRef = useRef<NormalizedSSRFValues>(normalizedDefaults)
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
    useSettingsForm<SSRFFormValues>({
      resolver: zodResolver(ssrfSchema) as Resolver<
        SSRFFormValues,
        unknown,
        SSRFFormValues
      >,
      defaultValues: formDefaults,
      onSubmit: async (data, changedFields) => {
        const normalizedValues = normalizeFormValues(data)
        const savedValues = savedValuesRef.current
        const updates: Array<{
          key: keyof NormalizedSSRFValues
          value: string | boolean
        }> = []

        for (const key of ssrfUpdateOrder) {
          if (!(key in changedFields)) {
            continue
          }

          if (!isSameSSRFValue(normalizedValues[key], savedValues[key])) {
            const value = normalizedValues[key]
            updates.push({
              key,
              value: Array.isArray(value) ? JSON.stringify(value) : value,
            })
          }
        }

        for (const update of updates) {
          await updateOption.mutateAsync(update)
        }

        // 这些字段在表单里按换行或逗号编辑，提交后继续把当前值规范化回
        // 统一展示格式，避免仅空白字符或无效端口差异残留为未保存状态。
        applyNormalizedFormValues(data, normalizedValues)
        savedValuesRef.current = normalizedValues
        savedSerializedRef.current = JSON.stringify(normalizedValues)
      },
    })

  const domainFilterMode = form.watch('fetch_setting.domain_filter_mode')
  const ipFilterMode = form.watch('fetch_setting.ip_filter_mode')

  return (
    <SettingsSection
      title={t('SSRF Protection')}
      description={t(
        'Prevent server-side request forgery attacks by controlling outbound requests.'
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
            saveLabel='Save SSRF settings'
          />
          <FormDirtyIndicator isDirty={isDirty} />

          <FormField
            control={form.control}
            name='fetch_setting.enable_ssrf_protection'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable SSRF Protection')}</FormLabel>
                  <FormDescription>
                    {t('Prevent server-side request forgery attacks')}
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
            name='fetch_setting.allow_private_ip'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Allow Private IPs')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Allow requests to private IP ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)'
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
            name='fetch_setting.domain_filter_mode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Domain Filter Mode')}</FormLabel>
                <Select
                  items={[
                    {
                      value: 'false',
                      label: t('Blacklist (Block listed domains)'),
                    },
                    {
                      value: 'true',
                      label: t('Whitelist (Only allow listed domains)'),
                    },
                  ]}
                  onValueChange={(value) => field.onChange(value === 'true')}
                  value={field.value ? 'true' : 'false'}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      <SelectItem value='false'>
                        {t('Blacklist (Block listed domains)')}
                      </SelectItem>
                      <SelectItem value='true'>
                        {t('Whitelist (Only allow listed domains)')}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FormDescription>
                  {t('Choose how to filter domains')}
                </FormDescription>
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='fetch_setting.domain_list'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('Domain')}{' '}
                  {domainFilterMode ? t('Whitelist') : t('Blacklist')}
                </FormLabel>
                <FormControl>
                  <Textarea
                    placeholder={t('example.com&#10;blocked-site.com')}
                    rows={4}
                    {...field}
                  />
                </FormControl>
                <FormDescription>{t('One domain per line')}</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='fetch_setting.ip_filter_mode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('IP Filter Mode')}</FormLabel>
                <Select
                  items={[
                    {
                      value: 'false',
                      label: t('Blacklist (Block listed IPs)'),
                    },
                    {
                      value: 'true',
                      label: t('Whitelist (Only allow listed IPs)'),
                    },
                  ]}
                  onValueChange={(value) => field.onChange(value === 'true')}
                  value={field.value ? 'true' : 'false'}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      <SelectItem value='false'>
                        {t('Blacklist (Block listed IPs)')}
                      </SelectItem>
                      <SelectItem value='true'>
                        {t('Whitelist (Only allow listed IPs)')}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <FormDescription>
                  {t('Choose how to filter IP addresses')}
                </FormDescription>
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='fetch_setting.ip_list'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('IP')} {ipFilterMode ? t('Whitelist') : t('Blacklist')}
                </FormLabel>
                <FormControl>
                  <Textarea
                    placeholder={t('192.168.1.1&#10;10.0.0.0/8')}
                    rows={4}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('One IP or CIDR range per line')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='fetch_setting.allowed_ports'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Allowed Ports')}</FormLabel>
                <FormControl>
                  <Input placeholder={t('80,443,8080')} {...field} />
                </FormControl>
                <FormDescription>
                  {t(
                    'Comma-separated list of allowed ports (empty = all ports)'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='fetch_setting.apply_ip_filter_for_domain'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Apply IP Filter to Resolved Domains')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Check resolved IPs against IP filters even when accessing by domain'
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
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
