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
import { useEffect, useMemo, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useForm, useWatch, type Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { updateSystemOption } from '../api'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSystemSettingPermissions } from '../hooks/use-system-setting-permissions'
import { removeTrailingSlash } from '../integrations/utils'
import { discoverOIDCEndpoints } from './custom-oauth/api'

// react-hook-form 会把 dotted name 当作嵌套路径解析。这里显式建模
// discord / oidc 对象，保存前再转回后端 option 需要的扁平 key。
const oauthSchema = z.object({
  GitHubOAuthEnabled: z.boolean(),
  GitHubClientId: z.string(),
  GitHubClientSecret: z.string(),
  discord: z.object({
    enabled: z.boolean(),
    client_id: z.string(),
    client_secret: z.string(),
  }),
  oidc: z.object({
    enabled: z.boolean(),
    client_id: z.string(),
    client_secret: z.string(),
    well_known: z.string(),
    authorization_endpoint: z.string(),
    token_endpoint: z.string(),
    user_info_endpoint: z.string(),
  }),
  TelegramOAuthEnabled: z.boolean(),
  TelegramBotToken: z.string(),
  TelegramBotName: z.string(),
  LinuxDOOAuthEnabled: z.boolean(),
  LinuxDOClientId: z.string(),
  LinuxDOClientSecret: z.string(),
  LinuxDOMinimumTrustLevel: z.string(),
  WeChatAuthEnabled: z.boolean(),
  WeChatServerAddress: z.string(),
  WeChatServerToken: z.string(),
  WeChatAccountQRCodeImageURL: z.string(),
})

type OAuthFormValues = z.infer<typeof oauthSchema>

type FlatOAuthDefaults = {
  GitHubOAuthEnabled: boolean
  GitHubClientId: string
  GitHubClientSecret: string
  'discord.enabled': boolean
  'discord.client_id': string
  'discord.client_secret': string
  'oidc.enabled': boolean
  'oidc.client_id': string
  'oidc.client_secret': string
  'oidc.well_known': string
  'oidc.authorization_endpoint': string
  'oidc.token_endpoint': string
  'oidc.user_info_endpoint': string
  TelegramOAuthEnabled: boolean
  TelegramBotToken: string
  TelegramBotName: string
  LinuxDOOAuthEnabled: boolean
  LinuxDOClientId: string
  LinuxDOClientSecret: string
  LinuxDOMinimumTrustLevel: string
  WeChatAuthEnabled: boolean
  WeChatServerAddress: string
  WeChatServerToken: string
  WeChatAccountQRCodeImageURL: string
}

type OAuthOptionKey = keyof FlatOAuthDefaults
type OAuthOptionValue = string | boolean

type PartialOAuthFormValues = Partial<
  Omit<OAuthFormValues, 'discord' | 'oidc'>
> & {
  discord?: Partial<OAuthFormValues['discord']>
  oidc?: Partial<OAuthFormValues['oidc']>
}

type OAuthSectionProps = {
  defaultValues: FlatOAuthDefaults
}

const oauthTabContentClassName =
  'grid min-w-0 gap-x-5 gap-y-6 lg:grid-cols-2 [&>[data-slot=form-item]]:min-w-0 lg:[&>[data-slot=form-item]:has([data-slot=switch])]:col-span-2'

const oauthSecretKeys = new Set<OAuthOptionKey>([
  'GitHubClientSecret',
  'discord.client_secret',
  'oidc.client_secret',
  'TelegramBotToken',
  'LinuxDOClientSecret',
  'WeChatServerToken',
])

// 后端在打开各 OAuth 开关时会立即校验对应配置是否已经存在。
// 因此保存时必须先提交 Client ID、Secret、端点、Bot 信息等凭据，
// 再提交 enabled 开关，避免同一轮保存因为顺序问题误报“配置缺失”。
const oauthUpdateOrder: OAuthOptionKey[] = [
  'GitHubClientId',
  'GitHubClientSecret',
  'discord.client_id',
  'discord.client_secret',
  'oidc.well_known',
  'oidc.client_id',
  'oidc.client_secret',
  'oidc.authorization_endpoint',
  'oidc.token_endpoint',
  'oidc.user_info_endpoint',
  'TelegramBotToken',
  'TelegramBotName',
  'LinuxDOClientId',
  'LinuxDOClientSecret',
  'LinuxDOMinimumTrustLevel',
  'WeChatServerAddress',
  'WeChatServerToken',
  'WeChatAccountQRCodeImageURL',
  'GitHubOAuthEnabled',
  'discord.enabled',
  'oidc.enabled',
  'TelegramOAuthEnabled',
  'LinuxDOOAuthEnabled',
  'WeChatAuthEnabled',
]

const statusRelatedOAuthKeys = new Set<OAuthOptionKey>([
  'GitHubOAuthEnabled',
  'GitHubClientId',
  'discord.enabled',
  'discord.client_id',
  'oidc.enabled',
  'oidc.client_id',
  'oidc.authorization_endpoint',
  'TelegramOAuthEnabled',
  'TelegramBotName',
  'LinuxDOOAuthEnabled',
  'LinuxDOClientId',
  'LinuxDOMinimumTrustLevel',
  'WeChatAuthEnabled',
  'WeChatAccountQRCodeImageURL',
])

function normalizeString(value: string | undefined | null) {
  return value ?? ''
}

function normalizeTrimmedString(value: string | undefined | null) {
  return normalizeString(value).trim()
}

function normalizeOAuthDefaults(
  defaults: FlatOAuthDefaults
): FlatOAuthDefaults {
  return {
    GitHubOAuthEnabled: defaults.GitHubOAuthEnabled,
    GitHubClientId: normalizeTrimmedString(defaults.GitHubClientId),
    GitHubClientSecret: '',
    'discord.enabled': defaults['discord.enabled'],
    'discord.client_id': normalizeTrimmedString(defaults['discord.client_id']),
    'discord.client_secret': '',
    'oidc.enabled': defaults['oidc.enabled'],
    'oidc.client_id': normalizeTrimmedString(defaults['oidc.client_id']),
    'oidc.client_secret': '',
    'oidc.well_known': normalizeTrimmedString(defaults['oidc.well_known']),
    'oidc.authorization_endpoint': normalizeTrimmedString(
      defaults['oidc.authorization_endpoint']
    ),
    'oidc.token_endpoint': normalizeTrimmedString(
      defaults['oidc.token_endpoint']
    ),
    'oidc.user_info_endpoint': normalizeTrimmedString(
      defaults['oidc.user_info_endpoint']
    ),
    TelegramOAuthEnabled: defaults.TelegramOAuthEnabled,
    TelegramBotToken: '',
    TelegramBotName: normalizeTrimmedString(defaults.TelegramBotName),
    LinuxDOOAuthEnabled: defaults.LinuxDOOAuthEnabled,
    LinuxDOClientId: normalizeTrimmedString(defaults.LinuxDOClientId),
    LinuxDOClientSecret: '',
    LinuxDOMinimumTrustLevel: normalizeTrimmedString(
      defaults.LinuxDOMinimumTrustLevel
    ),
    WeChatAuthEnabled: defaults.WeChatAuthEnabled,
    WeChatServerAddress: removeTrailingSlash(defaults.WeChatServerAddress),
    WeChatServerToken: '',
    WeChatAccountQRCodeImageURL: normalizeTrimmedString(
      defaults.WeChatAccountQRCodeImageURL
    ),
  }
}

function buildFormValues(values: FlatOAuthDefaults): OAuthFormValues {
  return {
    GitHubOAuthEnabled: values.GitHubOAuthEnabled,
    GitHubClientId: values.GitHubClientId,
    GitHubClientSecret: values.GitHubClientSecret,
    discord: {
      enabled: values['discord.enabled'],
      client_id: values['discord.client_id'],
      client_secret: values['discord.client_secret'],
    },
    oidc: {
      enabled: values['oidc.enabled'],
      client_id: values['oidc.client_id'],
      client_secret: values['oidc.client_secret'],
      well_known: values['oidc.well_known'],
      authorization_endpoint: values['oidc.authorization_endpoint'],
      token_endpoint: values['oidc.token_endpoint'],
      user_info_endpoint: values['oidc.user_info_endpoint'],
    },
    TelegramOAuthEnabled: values.TelegramOAuthEnabled,
    TelegramBotToken: values.TelegramBotToken,
    TelegramBotName: values.TelegramBotName,
    LinuxDOOAuthEnabled: values.LinuxDOOAuthEnabled,
    LinuxDOClientId: values.LinuxDOClientId,
    LinuxDOClientSecret: values.LinuxDOClientSecret,
    LinuxDOMinimumTrustLevel: values.LinuxDOMinimumTrustLevel,
    WeChatAuthEnabled: values.WeChatAuthEnabled,
    WeChatServerAddress: values.WeChatServerAddress,
    WeChatServerToken: values.WeChatServerToken,
    WeChatAccountQRCodeImageURL: values.WeChatAccountQRCodeImageURL,
  }
}

function completeFormValues(
  values: PartialOAuthFormValues | undefined,
  fallback: OAuthFormValues
): OAuthFormValues {
  return {
    GitHubOAuthEnabled:
      values?.GitHubOAuthEnabled ?? fallback.GitHubOAuthEnabled,
    GitHubClientId: values?.GitHubClientId ?? fallback.GitHubClientId,
    GitHubClientSecret:
      values?.GitHubClientSecret ?? fallback.GitHubClientSecret,
    discord: {
      enabled: values?.discord?.enabled ?? fallback.discord.enabled,
      client_id: values?.discord?.client_id ?? fallback.discord.client_id,
      client_secret:
        values?.discord?.client_secret ?? fallback.discord.client_secret,
    },
    oidc: {
      enabled: values?.oidc?.enabled ?? fallback.oidc.enabled,
      client_id: values?.oidc?.client_id ?? fallback.oidc.client_id,
      client_secret:
        values?.oidc?.client_secret ?? fallback.oidc.client_secret,
      well_known: values?.oidc?.well_known ?? fallback.oidc.well_known,
      authorization_endpoint:
        values?.oidc?.authorization_endpoint ??
        fallback.oidc.authorization_endpoint,
      token_endpoint:
        values?.oidc?.token_endpoint ?? fallback.oidc.token_endpoint,
      user_info_endpoint:
        values?.oidc?.user_info_endpoint ?? fallback.oidc.user_info_endpoint,
    },
    TelegramOAuthEnabled:
      values?.TelegramOAuthEnabled ?? fallback.TelegramOAuthEnabled,
    TelegramBotToken: values?.TelegramBotToken ?? fallback.TelegramBotToken,
    TelegramBotName: values?.TelegramBotName ?? fallback.TelegramBotName,
    LinuxDOOAuthEnabled:
      values?.LinuxDOOAuthEnabled ?? fallback.LinuxDOOAuthEnabled,
    LinuxDOClientId: values?.LinuxDOClientId ?? fallback.LinuxDOClientId,
    LinuxDOClientSecret:
      values?.LinuxDOClientSecret ?? fallback.LinuxDOClientSecret,
    LinuxDOMinimumTrustLevel:
      values?.LinuxDOMinimumTrustLevel ?? fallback.LinuxDOMinimumTrustLevel,
    WeChatAuthEnabled: values?.WeChatAuthEnabled ?? fallback.WeChatAuthEnabled,
    WeChatServerAddress:
      values?.WeChatServerAddress ?? fallback.WeChatServerAddress,
    WeChatServerToken: values?.WeChatServerToken ?? fallback.WeChatServerToken,
    WeChatAccountQRCodeImageURL:
      values?.WeChatAccountQRCodeImageURL ??
      fallback.WeChatAccountQRCodeImageURL,
  }
}

function normalizeFormValues(values: OAuthFormValues): FlatOAuthDefaults {
  return {
    GitHubOAuthEnabled: values.GitHubOAuthEnabled,
    GitHubClientId: normalizeTrimmedString(values.GitHubClientId),
    GitHubClientSecret: normalizeString(values.GitHubClientSecret),
    'discord.enabled': values.discord.enabled,
    'discord.client_id': normalizeTrimmedString(values.discord.client_id),
    'discord.client_secret': normalizeString(values.discord.client_secret),
    'oidc.enabled': values.oidc.enabled,
    'oidc.client_id': normalizeTrimmedString(values.oidc.client_id),
    'oidc.client_secret': normalizeString(values.oidc.client_secret),
    'oidc.well_known': normalizeTrimmedString(values.oidc.well_known),
    'oidc.authorization_endpoint': normalizeTrimmedString(
      values.oidc.authorization_endpoint
    ),
    'oidc.token_endpoint': normalizeTrimmedString(values.oidc.token_endpoint),
    'oidc.user_info_endpoint': normalizeTrimmedString(
      values.oidc.user_info_endpoint
    ),
    TelegramOAuthEnabled: values.TelegramOAuthEnabled,
    TelegramBotToken: normalizeString(values.TelegramBotToken),
    TelegramBotName: normalizeTrimmedString(values.TelegramBotName),
    LinuxDOOAuthEnabled: values.LinuxDOOAuthEnabled,
    LinuxDOClientId: normalizeTrimmedString(values.LinuxDOClientId),
    LinuxDOClientSecret: normalizeString(values.LinuxDOClientSecret),
    LinuxDOMinimumTrustLevel: normalizeTrimmedString(
      values.LinuxDOMinimumTrustLevel
    ),
    WeChatAuthEnabled: values.WeChatAuthEnabled,
    WeChatServerAddress: removeTrailingSlash(values.WeChatServerAddress),
    WeChatServerToken: normalizeString(values.WeChatServerToken),
    WeChatAccountQRCodeImageURL: normalizeTrimmedString(
      values.WeChatAccountQRCodeImageURL
    ),
  }
}

function isSameOAuthValues(
  left: FlatOAuthDefaults,
  right: FlatOAuthDefaults
) {
  return oauthUpdateOrder.every((key) => left[key] === right[key])
}

function getChangedOAuthKeys(
  current: FlatOAuthDefaults,
  saved: FlatOAuthDefaults
): OAuthOptionKey[] {
  return oauthUpdateOrder.filter((key) => {
    // Secret/Token 字段不会通过 /api/option/ 回填到前端。
    // 留空应表示“保留现有密钥”，不能被当作清空请求发送给后端。
    if (oauthSecretKeys.has(key) && current[key] === '') {
      return false
    }
    return current[key] !== saved[key]
  })
}

function shouldDiscoverOIDC(
  current: FlatOAuthDefaults,
  saved: FlatOAuthDefaults
) {
  return (
    current['oidc.well_known'] !== '' &&
    current['oidc.well_known'] !== saved['oidc.well_known']
  )
}

function toSavedVisibleValues(values: FlatOAuthDefaults): FlatOAuthDefaults {
  return {
    ...values,
    GitHubClientSecret: '',
    'discord.client_secret': '',
    'oidc.client_secret': '',
    TelegramBotToken: '',
    LinuxDOClientSecret: '',
    WeChatServerToken: '',
  }
}

function toOptionValue(value: string | boolean): OAuthOptionValue {
  return typeof value === 'boolean' ? value : value
}

function isHandledApiError(error: unknown) {
  return (
    typeof error === 'object' &&
    error !== null &&
    ('response' in error || 'request' in error)
  )
}

export function OAuthSection({ defaultValues }: OAuthSectionProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const permissions = useSystemSettingPermissions()
  const noPermissionMessage = t("You don't have necessary permission")
  const [activeTab, setActiveTab] = useState('github')
  const [isSaving, setIsSaving] = useState(false)

  const normalizedDefaults = useMemo(
    () => normalizeOAuthDefaults(defaultValues),
    [defaultValues]
  )
  const formDefaults = useMemo(
    () => buildFormValues(normalizedDefaults),
    [normalizedDefaults]
  )
  const [savedValues, setSavedValues] = useState<FlatOAuthDefaults>(
    () => normalizedDefaults
  )
  const savedSerializedRef = useRef(JSON.stringify(normalizedDefaults))

  const form = useForm<OAuthFormValues>({
    resolver: zodResolver(oauthSchema) as Resolver<
      OAuthFormValues,
      unknown,
      OAuthFormValues
    >,
    defaultValues: formDefaults,
  })

  useEffect(() => {
    const serialized = JSON.stringify(normalizedDefaults)
    if (serialized === savedSerializedRef.current) return

    savedSerializedRef.current = serialized
    setSavedValues(normalizedDefaults)
    form.reset(buildFormValues(normalizedDefaults))
  }, [form, normalizedDefaults])

  const watchedValues = useWatch({ control: form.control })
  const currentValues = useMemo(
    () =>
      normalizeFormValues(
        completeFormValues(watchedValues as PartialOAuthFormValues, formDefaults)
      ),
    [formDefaults, watchedValues]
  )
  const hasPendingChanges = useMemo(
    () => !isSameOAuthValues(currentValues, savedValues),
    [currentValues, savedValues]
  )

  const invalidateOAuthQueries = async (keys: OAuthOptionKey[]) => {
    await queryClient.invalidateQueries({ queryKey: ['system-options'] })
    if (keys.some((key) => statusRelatedOAuthKeys.has(key))) {
      await queryClient.invalidateQueries({ queryKey: ['status'] })
    }
  }

  const handleReset = () => {
    form.reset(buildFormValues(savedValues))
    toast.success(t('Form reset to saved values'))
  }

  const onSubmit = async (values: OAuthFormValues) => {
    if (!permissions.canSensitiveWrite) {
      toast.error(noPermissionMessage)
      return
    }

    setIsSaving(true)
    const savedBeforeSubmit = savedValues
    let savedKeys: OAuthOptionKey[] = []

    try {
      let normalizedValues = normalizeFormValues(values)

      if (shouldDiscoverOIDC(normalizedValues, savedBeforeSubmit)) {
        const wellKnown = normalizedValues['oidc.well_known']
        if (
          !wellKnown.startsWith('http://') &&
          !wellKnown.startsWith('https://')
        ) {
          toast.error(t('Well-Known URL must start with http:// or https://'))
          return
        }

        // 通过后端复用自定义 OAuth 的 Discovery 接口，避免浏览器直连
        // 第三方 well-known 时触发 CORS、内网访问或控制台网络错误。
        const response = await discoverOIDCEndpoints(wellKnown)
        if (!response.success || !response.data?.discovery) {
          return
        }

        const discovery = response.data.discovery
        normalizedValues = {
          ...normalizedValues,
          'oidc.authorization_endpoint':
            discovery.authorization_endpoint ?? '',
          'oidc.token_endpoint': discovery.token_endpoint ?? '',
          'oidc.user_info_endpoint': discovery.userinfo_endpoint ?? '',
        }
        values.oidc.authorization_endpoint =
          normalizedValues['oidc.authorization_endpoint']
        values.oidc.token_endpoint = normalizedValues['oidc.token_endpoint']
        values.oidc.user_info_endpoint =
          normalizedValues['oidc.user_info_endpoint']
        form.setValue(
          'oidc.authorization_endpoint',
          normalizedValues['oidc.authorization_endpoint'],
          { shouldDirty: true }
        )
        form.setValue('oidc.token_endpoint', normalizedValues['oidc.token_endpoint'], {
          shouldDirty: true,
        })
        form.setValue(
          'oidc.user_info_endpoint',
          normalizedValues['oidc.user_info_endpoint'],
          { shouldDirty: true }
        )
      }

      const changedKeys = getChangedOAuthKeys(
        normalizedValues,
        savedBeforeSubmit
      )
      if (changedKeys.length === 0) {
        toast.info(t('No changes to save'))
        return
      }

      for (const key of changedKeys) {
        const response = await updateSystemOption({
          key,
          value: toOptionValue(normalizedValues[key]),
        })
        if (!response.success) {
          await invalidateOAuthQueries(savedKeys)
          return
        }
        savedKeys = [...savedKeys, key]
      }

      const nextSavedValues = toSavedVisibleValues(normalizedValues)
      setSavedValues(nextSavedValues)
      savedSerializedRef.current = JSON.stringify(nextSavedValues)
      form.reset(buildFormValues(nextSavedValues))
      toast.success(t('Setting updated successfully'))
      await invalidateOAuthQueries(changedKeys)
    } catch (error) {
      if (!isHandledApiError(error)) {
        toast.error(
          error instanceof Error
            ? error.message
            : t('Failed to update setting')
        )
      }
      if (savedKeys.length > 0) {
        await invalidateOAuthQueries(savedKeys)
      }
    } finally {
      setIsSaving(false)
    }
  }

  const handleSubmit = form.handleSubmit(onSubmit)

  return (
    <SettingsSection
      title={t('OAuth Integrations')}
      description={t('Configure third-party authentication providers')}
    >
      <FormNavigationGuard when={hasPendingChanges} />

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit} autoComplete='off'>
          <SettingsPageFormActions
            onSave={handleSubmit}
            onReset={handleReset}
            isSaving={isSaving}
            isSaveDisabled={!hasPendingChanges || !permissions.canSensitiveWrite}
            saveDisabledReason={
              permissions.canSensitiveWrite ? undefined : noPermissionMessage
            }
            isResetDisabled={!hasPendingChanges}
            saveLabel='Save Changes'
          />
          <FormDirtyIndicator isDirty={hasPendingChanges} />

          <Tabs value={activeTab} onValueChange={setActiveTab}>
            <TabsList className='grid w-full grid-cols-2 sm:grid-cols-3 lg:grid-cols-6'>
              <TabsTrigger value='github'>{t('GitHub')}</TabsTrigger>
              <TabsTrigger value='discord'>{t('Discord')}</TabsTrigger>
              <TabsTrigger value='oidc'>{t('OIDC')}</TabsTrigger>
              <TabsTrigger value='telegram'>{t('Telegram')}</TabsTrigger>
              <TabsTrigger value='linuxdo'>{t('LinuxDO')}</TabsTrigger>
              <TabsTrigger value='wechat'>{t('WeChat')}</TabsTrigger>
            </TabsList>

            <TabsContent value='github' className={oauthTabContentClassName}>
              <FormField
                control={form.control}
                name='GitHubOAuthEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Enable GitHub OAuth')}</FormLabel>
                      <FormDescription>
                        {t('Allow users to sign in with GitHub')}
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
                name='GitHubClientId'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Client ID')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Your GitHub OAuth Client ID')}
                        autoComplete='off'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='GitHubClientSecret'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Client Secret')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('Your GitHub OAuth Client Secret')}
                        autoComplete='new-password'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TabsContent>

            <TabsContent value='discord' className={oauthTabContentClassName}>
              <FormField
                control={form.control}
                name='discord.enabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Enable Discord OAuth')}</FormLabel>
                      <FormDescription>
                        {t('Allow users to sign in with Discord')}
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
                name='discord.client_id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Client ID')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Your Discord OAuth Client ID')}
                        autoComplete='off'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='discord.client_secret'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Client Secret')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('Your Discord OAuth Client Secret')}
                        autoComplete='new-password'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TabsContent>

            <TabsContent value='oidc' className={oauthTabContentClassName}>
              <FormField
                control={form.control}
                name='oidc.enabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Enable OIDC')}</FormLabel>
                      <FormDescription>
                        {t('Allow users to sign in with OpenID Connect')}
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
                name='oidc.client_id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Client ID')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('OIDC Client ID')}
                        autoComplete='off'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='oidc.client_secret'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Client Secret')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('OIDC Client Secret')}
                        autoComplete='new-password'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='oidc.well_known'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Well-Known URL')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t(
                          'https://provider.com/.well-known/openid-configuration'
                        )}
                        autoComplete='off'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Auto-discovers endpoints from the provider')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='oidc.authorization_endpoint'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Authorization Endpoint (Optional)')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Override auto-discovered endpoint')}
                        autoComplete='off'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='oidc.token_endpoint'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Token Endpoint (Optional)')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Override auto-discovered endpoint')}
                        autoComplete='off'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='oidc.user_info_endpoint'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('User Info Endpoint (Optional)')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Override auto-discovered endpoint')}
                        autoComplete='off'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TabsContent>

            <TabsContent value='telegram' className={oauthTabContentClassName}>
              <FormField
                control={form.control}
                name='TelegramOAuthEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Enable Telegram OAuth')}</FormLabel>
                      <FormDescription>
                        {t('Allow users to sign in with Telegram')}
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
                name='TelegramBotToken'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Bot Token')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('Your Telegram Bot Token')}
                        autoComplete='new-password'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='TelegramBotName'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Bot Name')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('Your Bot Name')}
                        autoComplete='off'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TabsContent>

            <TabsContent value='linuxdo' className={oauthTabContentClassName}>
              <FormField
                control={form.control}
                name='LinuxDOOAuthEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Enable LinuxDO OAuth')}</FormLabel>
                      <FormDescription>
                        {t('Allow users to sign in with LinuxDO')}
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
                name='LinuxDOClientId'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Client ID')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('LinuxDO Client ID')}
                        autoComplete='off'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='LinuxDOClientSecret'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Client Secret')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('LinuxDO Client Secret')}
                        autoComplete='new-password'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='LinuxDOMinimumTrustLevel'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Minimum Trust Level')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder='0'
                        autoComplete='off'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Minimum LinuxDO trust level required')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TabsContent>

            <TabsContent value='wechat' className={oauthTabContentClassName}>
              <FormField
                control={form.control}
                name='WeChatAuthEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Enable WeChat Auth')}</FormLabel>
                      <FormDescription>
                        {t('Allow users to sign in with WeChat')}
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
                name='WeChatServerAddress'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Server Address')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('https://wechat-server.example.com')}
                        autoComplete='off'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='WeChatServerToken'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Server Token')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        placeholder={t('Server Token')}
                        autoComplete='new-password'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='WeChatAccountQRCodeImageURL'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('QR Code Image URL')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('https://example.com/qr-code.png')}
                        autoComplete='off'
                        value={field.value}
                        onChange={(event) => field.onChange(event.target.value)}
                        name={field.name}
                        onBlur={field.onBlur}
                        ref={field.ref}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TabsContent>
          </Tabs>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
