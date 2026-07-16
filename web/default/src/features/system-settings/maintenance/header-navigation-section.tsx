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
import { useMemo } from 'react'
import * as z from 'zod'
import type { Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { parseHeaderNavBoolean } from '@/lib/nav-modules'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsControlChildren,
  SettingsControlGroup,
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  HEADER_NAV_DEFAULT,
  type HeaderNavModulesConfig,
  serializeHeaderNavModules,
} from './config'

const headerNavSchema = z.object({
  home: z.boolean(),
  console: z.boolean(),
  pricingEnabled: z.boolean(),
  pricingRequireAuth: z.boolean(),
  rankingsEnabled: z.boolean(),
  rankingsRequireAuth: z.boolean(),
  docs: z.boolean(),
  about: z.boolean(),
})

type HeaderNavFormValues = z.infer<typeof headerNavSchema>

type HeaderNavigationSectionProps = {
  config: HeaderNavModulesConfig
  initialSerialized: string
}

const toFormValues = (config: HeaderNavModulesConfig): HeaderNavFormValues => ({
  home:
    config.home === undefined
      ? HEADER_NAV_DEFAULT.home
      : parseHeaderNavBoolean(config.home, HEADER_NAV_DEFAULT.home),
  console:
    config.console === undefined
      ? HEADER_NAV_DEFAULT.console
      : parseHeaderNavBoolean(config.console, HEADER_NAV_DEFAULT.console),
  pricingEnabled:
    config.pricing?.enabled === undefined
      ? HEADER_NAV_DEFAULT.pricing.enabled
      : parseHeaderNavBoolean(
          config.pricing.enabled,
          HEADER_NAV_DEFAULT.pricing.enabled
        ),
  pricingRequireAuth:
    config.pricing?.requireAuth === undefined
      ? HEADER_NAV_DEFAULT.pricing.requireAuth
      : parseHeaderNavBoolean(
          config.pricing.requireAuth,
          HEADER_NAV_DEFAULT.pricing.requireAuth
        ),
  rankingsEnabled:
    config.rankings?.enabled === undefined
      ? HEADER_NAV_DEFAULT.rankings.enabled
      : parseHeaderNavBoolean(
          config.rankings.enabled,
          HEADER_NAV_DEFAULT.rankings.enabled
        ),
  rankingsRequireAuth:
    config.rankings?.requireAuth === undefined
      ? HEADER_NAV_DEFAULT.rankings.requireAuth
      : parseHeaderNavBoolean(
          config.rankings.requireAuth,
          HEADER_NAV_DEFAULT.rankings.requireAuth
        ),
  docs:
    config.docs === undefined
      ? HEADER_NAV_DEFAULT.docs
      : parseHeaderNavBoolean(config.docs, HEADER_NAV_DEFAULT.docs),
  about:
    config.about === undefined
      ? HEADER_NAV_DEFAULT.about
      : parseHeaderNavBoolean(config.about, HEADER_NAV_DEFAULT.about),
})

export function HeaderNavigationSection({
  config,
  initialSerialized,
}: HeaderNavigationSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const formDefaults = useMemo(() => toFormValues(config), [config])

  const simpleModules: Array<{
    key: keyof HeaderNavFormValues
    title: string
    description: string
  }> = [
    {
      key: 'home',
      title: t('Home'),
      description: t('Landing page with system overview.'),
    },
    {
      key: 'console',
      title: t('Console'),
      description: t('User dashboard and quota controls.'),
    },
    {
      key: 'docs',
      title: t('Docs'),
      description: t('Documentation or external knowledge base.'),
    },
    {
      key: 'about',
      title: t('About'),
      description: t('Static page describing the platform.'),
    },
  ]

  const accessModules: Array<{
    enabledKey: keyof HeaderNavFormValues
    requireAuthKey: keyof HeaderNavFormValues
    requireAuthDependsOn: 'pricingEnabled' | 'rankingsEnabled'
    title: string
    description: string
    requireAuthTitle: string
    requireAuthDescription: string
  }> = [
    {
      enabledKey: 'pricingEnabled',
      requireAuthKey: 'pricingRequireAuth',
      requireAuthDependsOn: 'pricingEnabled',
      title: t('Model Square'),
      description: t('Public model catalog and pricing page.'),
      requireAuthTitle: t('Require login to view models'),
      requireAuthDescription: t(
        'Visitors must authenticate before accessing the pricing directory.'
      ),
    },
    {
      enabledKey: 'rankingsEnabled',
      requireAuthKey: 'rankingsRequireAuth',
      requireAuthDependsOn: 'rankingsEnabled',
      title: t('Rankings'),
      description: t('Public rankings page based on live usage data.'),
      requireAuthTitle: t('Require login to view rankings'),
      requireAuthDescription: t(
        'Visitors must authenticate before accessing the rankings page.'
      ),
    },
  ]

  const { form, handleSubmit, isDirty, isSubmitting } =
    useSettingsForm<HeaderNavFormValues>({
      resolver: zodResolver(headerNavSchema) as Resolver<
        HeaderNavFormValues,
        unknown,
        HeaderNavFormValues
      >,
      defaultValues: formDefaults,
      onSubmit: async (values) => {
        const payload: HeaderNavModulesConfig = {
          ...config,
          home: values.home,
          console: values.console,
          docs: values.docs,
          about: values.about,
          pricing: {
            ...(config.pricing ?? HEADER_NAV_DEFAULT.pricing),
            enabled: values.pricingEnabled,
            requireAuth: values.pricingRequireAuth,
          },
          rankings: {
            ...(config.rankings ?? HEADER_NAV_DEFAULT.rankings),
            enabled: values.rankingsEnabled,
            requireAuth: values.rankingsRequireAuth,
          },
        }

        const serialized = serializeHeaderNavModules(payload)
        if (serialized === initialSerialized) {
          return
        }

        await updateOption.mutateAsync({
          key: 'HeaderNavModules',
          value: serialized,
        })
      },
    })

  const resetToDefault = () => {
    // 这里只重置当前草稿到平台默认值，但不能把默认值误记为“已保存基线”，
    // 否则后续点击保存时 dirtyFields 会被清空，无法真正提交默认配置。
    form.reset(toFormValues(HEADER_NAV_DEFAULT), {
      keepDefaultValues: true,
    })
  }

  return (
    <SettingsSection
      title={t('Header navigation')}
      description={t('Enable or disable top navigation modules globally.')}
    >
      <FormNavigationGuard when={isDirty} />

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsPageFormActions
            onSave={handleSubmit}
            onReset={resetToDefault}
            isSaving={isSubmitting || updateOption.isPending}
            isSaveDisabled={!updateOption.canUpdate}
            saveDisabledReason={updateOption.disabledReason}
            resetLabel='Reset to default'
            saveLabel='Save navigation'
          />
          <FormDirtyIndicator isDirty={isDirty} />

          <div
            data-settings-form-span='full'
            className='grid gap-4 md:grid-cols-2'
          >
            {simpleModules.map((module) => (
              <FormField
                key={module.key}
                control={form.control}
                name={module.key}
                render={({ field }) => (
                  <SettingsSwitchItem className='rounded-xl border px-4 py-3.5'>
                    <SettingsSwitchContent className='pe-4'>
                      <FormLabel>{module.title}</FormLabel>
                      <FormDescription>{module.description}</FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </SettingsSwitchItem>
                )}
              />
            ))}
          </div>

          <div
            data-settings-form-span='full'
            className='grid gap-4 lg:grid-cols-2'
          >
            {accessModules.map((module) => (
              <SettingsControlGroup
                key={module.enabledKey}
                className='gap-4 rounded-xl px-4 py-3.5'
              >
                <FormField
                  control={form.control}
                  name={module.enabledKey}
                  render={({ field }) => (
                    <SettingsSwitchItem className='py-0'>
                      <SettingsSwitchContent className='pe-4'>
                        <FormLabel>{module.title}</FormLabel>
                        <FormDescription>{module.description}</FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                      <FormMessage />
                    </SettingsSwitchItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name={module.requireAuthKey}
                  render={({ field }) => (
                    <SettingsControlChildren className='pl-4'>
                      <SettingsSwitchItem className='py-0'>
                        <SettingsSwitchContent className='pe-4'>
                          <FormLabel>{module.requireAuthTitle}</FormLabel>
                          <FormDescription>
                            {module.requireAuthDescription}
                          </FormDescription>
                        </SettingsSwitchContent>
                        <FormControl>
                          <Switch
                            checked={field.value}
                            onCheckedChange={field.onChange}
                            disabled={!form.watch(module.requireAuthDependsOn)}
                          />
                        </FormControl>
                        <FormMessage />
                      </SettingsSwitchItem>
                    </SettingsControlChildren>
                  )}
                />
              </SettingsControlGroup>
            ))}
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
