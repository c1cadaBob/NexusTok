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
import {
  type ReactNode,
  useEffect,
  useState,
  useMemo,
  useCallback,
  useRef,
} from 'react'
import { z } from 'zod'
import { type SubmitErrorHandler, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowRight,
  AlertCircle,
  CheckCircle2,
  Circle,
  ChevronDown,
  HelpCircle,
  Loader2,
  Sparkles,
  Trash2,
  Copy,
  FileText,
  Eraser,
  Plus,
  Eye,
  Link2,
  RefreshCw,
  Code,
  Boxes,
  KeyRound,
  Route,
  Server,
  Settings,
  SlidersHorizontal,
  Wand2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { useDebounce } from '@/hooks/use-debounce'
import { useHiddenClickUnlock } from '@/hooks/use-hidden-click-unlock'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Combobox } from '@/components/ui/combobox'
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
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
  sideDrawerSectionClassName,
  sideDrawerSwitchItemClassName,
} from '@/components/drawer-layout'
import { JsonEditor } from '@/components/json-editor'
import { MultiSelect } from '@/components/multi-select'
import { getAccountPoolGroupOptions } from '@/features/account-pool/api'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { searchModels } from '@/features/models/api'
import {
  fetchModels,
  getAllModels,
  getChannel,
  getChannelAccounts,
  getChannelKey,
  getGroups,
  getPrefillGroups,
  completeUpstreamAccountPreview2FA,
  createUpstreamAccountChannel,
  previewUpstreamAccount,
  refreshUpstreamAccountChannel,
  refreshCodexCredential,
  updateChannel,
  updateChannelAccount,
  updateChannelAccountStatus,
} from '../../api'
import {
  ADD_MODE_OPTIONS,
  CHANNEL_STATUS,
  CHANNEL_STATUS_LABELS,
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_WARNINGS,
  ERROR_MESSAGES,
  FIELD_DESCRIPTIONS,
  FIELD_PLACEHOLDERS,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import {
  buildAllowedChannelUpdatePayload,
  hasDirtySensitiveChannelFormFields,
  useChannelMutateForm,
} from '../../hooks/use-channel-mutate-form'
import { useChannelPermissions } from '../../hooks/use-channel-permissions'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  CHANNEL_TYPE_ADVANCED_CUSTOM,
  channelFormSchema,
  channelsQueryKeys,
  transformChannelToFormDefaults,
  type ChannelFormValues,
  deduplicateKeys,
  getAdvancedCustomStats,
  getKeyPromptForType,
  parseModelsString,
  formatModelsArray,
  formatGroups,
  extractRedirectModels,
  extractMappingSourceModels,
  hasModelConfigChanged,
  findMissingModelsInMapping,
  dedupeModelNames,
  getModelSearchModelNameResult,
  getModelSearchVendorForChannelType,
  isUpstreamAccountSyncChannel,
  mergeModelNames,
  summarizeModelSearchCandidates,
  validateModelMappingJson,
  hasAdvancedSettingsErrors,
  transformFormDataToUpdatePayload,
} from '../../lib'
import {
  collectInvalidStatusCodeEntries,
  collectNewDisallowedStatusCodeRedirects,
} from '../../lib/status-code-risk-guard'
import {
  formatUpstreamModelRatioDetails,
  formatUpstreamRatioCompact,
  getUpstreamKeyRatioDisplayValue,
  getUpstreamKeyGroupLabel,
  getUpstreamRatioDisplayValue,
} from '../../lib/upstream-sync'
import type {
  Channel,
  ChannelAccount,
  UpstreamAccountKey,
  UpstreamAccountPreviewData,
  UpstreamAccountRatioConversion,
  UpstreamAccountTwoFactorChallenge,
  UpstreamAccountPlatform,
  UpstreamAccountSnapshot,
} from '../../types'
import { ChannelTypeIcon } from '../channel-type-icon'
import { useChannels } from '../channels-provider'
import { AdvancedCustomEditorDialog } from '../dialogs/advanced-custom-editor-dialog'
import { CodexOAuthDialog } from '../dialogs/codex-oauth-dialog'
import { FetchModelsDialog } from '../dialogs/fetch-models-dialog'
import {
  MissingModelsConfirmationDialog,
  type MissingModelsAction,
} from '../dialogs/missing-models-confirmation-dialog'
import { ParamOverrideEditorDialog } from '../dialogs/param-override-editor-dialog'
import { StatusCodeRiskDialog } from '../dialogs/status-code-risk-dialog'
import { ModelMappingEditor } from '../model-mapping-editor'
import {
  ChannelAdvancedSection,
  ChannelApiAccessSection,
  ChannelAuthSection,
  ChannelBasicSection,
  ChannelEditorLoadingState,
  ChannelModelsSection,
} from './sections'

type ChannelMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: Channel | null
}

type ModelMappingGuardrail = {
  invalidJson: boolean
  entries: Array<{ source: string; target: string }>
  missingSourceModels: string[]
  exposedTargetModels: string[]
}

type ChannelEditorSectionStatus = 'complete' | 'configured' | 'error' | 'idle'

type ChannelEditorNavChildItem = {
  id: string
  title: string
  configured?: boolean
}

type ChannelEditorNavItem = {
  id: string
  title: string
  description?: string
  statusLabel: string
  status: ChannelEditorSectionStatus
  icon: ReactNode
  configured?: boolean
  children?: ChannelEditorNavChildItem[]
}

type UpstreamAccountConfigDraft = {
  priority: number
  weight: number
  enabled: boolean
  models?: string
  group?: string
}

export type UpstreamEditableAccount = UpstreamAccountKey & {
  account_id?: number
  account_status?: number
}

type UpstreamTwoFactorMode = 'create' | 'refresh'

const PREVIEW_EXPIRED_ERROR_TEXT = '预览快照不存在或已过期'
const UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY = 'upstream_account_sync'
const CHANNEL_TYPE_NEW_API = 59
const CHANNEL_TYPE_SUB2API = 60

function upstreamPlatformFromChannelType(
  channelType: number
): UpstreamAccountPlatform | null {
  if (channelType === CHANNEL_TYPE_NEW_API) return 'new-api'
  if (channelType === CHANNEL_TYPE_SUB2API) return 'sub2api'
  return null
}

function channelTypeFromUpstreamPlatform(platform: string | undefined | null) {
  const normalized = String(platform || '')
    .trim()
    .toLowerCase()
    .replaceAll('_', '-')
  if (normalized === 'new-api' || normalized === 'newapi') {
    return CHANNEL_TYPE_NEW_API
  }
  if (normalized === 'sub2api' || normalized === 'sub2-api') {
    return CHANNEL_TYPE_SUB2API
  }
  return undefined
}

export function defaultUpstreamChannelName(baseUrl: string, fallback = '') {
  const trimmedBaseUrl = baseUrl.trim()
  const fallbackName = fallback.trim()
  if (!trimmedBaseUrl) return fallbackName

  try {
    const withProtocol = /^[a-z][a-z\d+\-.]*:\/\//i.test(trimmedBaseUrl)
      ? trimmedBaseUrl
      : `https://${trimmedBaseUrl}`
    const host = new URL(withProtocol).hostname.replace(/^www\./i, '')
    if (!host) return fallbackName
    if (
      host === 'localhost' ||
      /^\d{1,3}(?:\.\d{1,3}){3}$/.test(host) ||
      host.includes(':')
    ) {
      return host
    }
    return host.split('.').filter(Boolean)[0] || fallbackName
  } catch {
    return fallbackName
  }
}

function parsePositiveNumber(value: string): number | undefined {
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed <= 0) return undefined
  return parsed
}

function buildUpstreamRatioConversionPayload(
  paidCny: string,
  platformUsdCredit: string
): UpstreamAccountRatioConversion | undefined {
  const normalizedPaidCny = parsePositiveNumber(paidCny)
  const normalizedPlatformUsdCredit = parsePositiveNumber(platformUsdCredit)
  if (!normalizedPaidCny || !normalizedPlatformUsdCredit) {
    return undefined
  }
  return {
    paid_cny: normalizedPaidCny,
    platform_usd_credit: normalizedPlatformUsdCredit,
  }
}

export function resolveUpstreamChannelGroup(groups: string[] | undefined) {
  return formatGroups(groups || []) || 'default'
}

function setUpstreamRatioConversionState(
  config: UpstreamAccountRatioConversion | null | undefined,
  setPaidCny: (value: string) => void,
  setPlatformUsdCredit: (value: string) => void
) {
  setPaidCny(
    config?.paid_cny && Number.isFinite(config.paid_cny)
      ? String(config.paid_cny)
      : ''
  )
  setPlatformUsdCredit(
    config?.platform_usd_credit && Number.isFinite(config.platform_usd_credit)
      ? String(config.platform_usd_credit)
      : ''
  )
}

export function upstreamKeyConfigId(key: UpstreamAccountKey, index: number) {
  return key.sync_id || key.external_id || key.masked_key || `${index}`
}

export function getUpstreamAccountConfig(
  configs: Record<string, UpstreamAccountConfigDraft>,
  key: UpstreamAccountKey,
  index: number
) {
  const candidates = [
    upstreamKeyConfigId(key, index),
    key.sync_id,
    key.external_id,
    key.masked_key,
  ]
  for (const candidate of candidates) {
    if (candidate && configs[candidate]) {
      return configs[candidate]
    }
  }
  return undefined
}

export function buildUpstreamAccountConfigsFromSnapshotKeys(
  keys: UpstreamAccountKey[],
  previousConfigs: Record<string, UpstreamAccountConfigDraft> = {}
): Record<string, UpstreamAccountConfigDraft> {
  const configs: Record<string, UpstreamAccountConfigDraft> = {}
  keys.forEach((key, index) => {
    const configId = upstreamKeyConfigId(key, index)
    const previousConfig = getUpstreamAccountConfig(previousConfigs, key, index)
    configs[configId] = {
      priority: previousConfig?.priority ?? key.suggested_priority,
      weight: previousConfig?.weight ?? key.suggested_weight,
      enabled: previousConfig?.enabled ?? true,
      models: previousConfig?.models ?? key.models?.join(',') ?? '',
      group: previousConfig?.group ?? key.group_name ?? key.group_id ?? '',
    }
  })
  return configs
}

function upstreamPreviewRemainingSeconds(expiresAt: number, nowMs: number) {
  if (!expiresAt) return 0
  return Math.max(0, Math.ceil(expiresAt - nowMs / 1000))
}

function hasUpstreamPreviewSnapshot(
  data: unknown
): data is UpstreamAccountPreviewData {
  return Boolean(
    data &&
    typeof data === 'object' &&
    'preview_id' in data &&
    'snapshot' in data
  )
}

function getUpstreamPreviewChallenge(
  data: unknown
): UpstreamAccountTwoFactorChallenge | null {
  if (!data || typeof data !== 'object' || !('challenge' in data)) return null
  const challenge = (data as { challenge?: unknown }).challenge
  if (!challenge || typeof challenge !== 'object') return null
  if (!('challenge_id' in challenge) || !('expires_at' in challenge))
    return null
  return challenge as UpstreamAccountTwoFactorChallenge
}

function formatUpstreamPreviewRemaining(seconds: number) {
  const safeSeconds = Math.max(0, seconds)
  const minutes = Math.floor(safeSeconds / 60)
  const remainingSeconds = safeSeconds % 60
  return `${minutes}:${remainingSeconds.toString().padStart(2, '0')}`
}

function normalizeUpstreamChannelBaseUrl(value?: string | null) {
  return String(value || '')
    .trim()
    .replace(/\/+$/, '')
}

function isUpstreamPreviewExpiredError(message?: string) {
  return Boolean(message?.includes(PREVIEW_EXPIRED_ERROR_TEXT))
}

function getUpstreamSyncBaseUrlFromSettings(
  settings: string | undefined
): string {
  const metadata =
    parseSettingsRecord(settings)[UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY]
  if (!metadata || typeof metadata !== 'object') return ''
  return String((metadata as Record<string, unknown>).base_url || '').trim()
}

function getUpstreamSyncPlatformFromSettings(
  settings: string | undefined
): UpstreamAccountPlatform | '' {
  const metadata =
    parseSettingsRecord(settings)[UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY]
  if (!metadata || typeof metadata !== 'object') return ''
  const platform = String((metadata as Record<string, unknown>).platform || '')
    .trim()
    .toLowerCase()
    .replaceAll('_', '-')
  if (platform === 'new-api' || platform === 'newapi') return 'new-api'
  if (platform === 'sub2api' || platform === 'sub2-api') return 'sub2api'
  return ''
}

function hasUpstreamSyncSavedCredential(settings: string | undefined): boolean {
  const metadata =
    parseSettingsRecord(settings)[UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY]
  if (!metadata || typeof metadata !== 'object') return false
  const record = metadata as Record<string, unknown>
  return Boolean(record.credential_saved || record.credentials)
}

function upstreamModelsToString(keys: UpstreamAccountKey[]) {
  const seen = new Set<string>()
  const models: string[] = []
  keys.forEach((key) => {
    key.models?.forEach((modelName) => {
      const trimmed = modelName.trim()
      if (!trimmed || seen.has(trimmed)) return
      seen.add(trimmed)
      models.push(trimmed)
    })
  })
  return models.join(',')
}

function upstreamAccountConfigTextValue(
  configValue: string | undefined,
  fallbackValue: string
) {
  return configValue ?? fallbackValue
}

function upstreamAccountModelsValue(
  key: UpstreamAccountKey,
  config: UpstreamAccountConfigDraft | undefined,
  fallbackModels = ''
) {
  // 逐密钥输入框允许管理员显式清空模型。只有配置对象里完全没有 models 字段时，
  // 才回退到上游快照和渠道级默认值，避免保存时把用户输入的空值吞回旧值。
  if (config && Object.prototype.hasOwnProperty.call(config, 'models')) {
    return config.models?.trim() || ''
  }
  return key.models?.join(',') || fallbackModels.trim() || ''
}

export function upstreamAccountModelsArrayValue(
  key: UpstreamAccountKey,
  config: UpstreamAccountConfigDraft | undefined,
  fallbackModels = ''
) {
  return parseModelsString(
    upstreamAccountModelsValue(key, config, fallbackModels)
  )
}

export function buildUpstreamAccountModelOptions(
  key: UpstreamAccountKey,
  config: UpstreamAccountConfigDraft | undefined,
  candidateModels: readonly string[],
  fallbackModels = ''
) {
  return dedupeModelNames([
    ...upstreamAccountModelsArrayValue(key, config, fallbackModels),
    ...(key.models ?? []),
    ...candidateModels,
  ]).map((model) => ({
    value: model,
    label: model,
  }))
}

function upstreamAccountGroupValue(
  key: UpstreamAccountKey,
  config: UpstreamAccountConfigDraft | undefined,
  fallbackGroup = ''
) {
  // 分组与模型相同：空字符串是显式配置，不能再回退到上游原始分组。
  if (config && Object.prototype.hasOwnProperty.call(config, 'group')) {
    return config.group?.trim() || ''
  }
  return key.group_name || key.group_id || fallbackGroup.trim() || ''
}

function upstreamAccountPriorityValue(
  config: UpstreamAccountConfigDraft | undefined,
  applySuggested: boolean
) {
  return applySuggested ? undefined : config?.priority
}

function upstreamAccountWeightValue(
  config: UpstreamAccountConfigDraft | undefined,
  applySuggested: boolean
) {
  return applySuggested ? undefined : config?.weight
}

export function upstreamAccountValuesToString(
  keys: UpstreamAccountKey[],
  configs: Record<string, UpstreamAccountConfigDraft>,
  getValue: (
    key: UpstreamAccountKey,
    config: UpstreamAccountConfigDraft | undefined
  ) => string
) {
  const seen = new Set<string>()
  const values: string[] = []
  keys.forEach((key, index) => {
    const config = getUpstreamAccountConfig(configs, key, index)
    if (config?.enabled === false) return
    getValue(key, config)
      .split(',')
      .map((value) => value.trim())
      .filter(Boolean)
      .forEach((value) => {
        if (seen.has(value)) return
        seen.add(value)
        values.push(value)
      })
  })
  return values.join(',')
}

function buildUpstreamAccountPayloads(
  keys: UpstreamAccountKey[],
  configs: Record<string, UpstreamAccountConfigDraft>,
  applySuggested: boolean,
  fallbackModels = '',
  fallbackGroup = ''
) {
  return keys.map((key, index) => {
    const config = getUpstreamAccountConfig(configs, key, index)
    return {
      sync_id: key.sync_id,
      external_id: key.external_id,
      name: key.name || key.masked_key,
      enabled: config?.enabled ?? true,
      models: upstreamAccountModelsValue(key, config, fallbackModels),
      group: upstreamAccountGroupValue(key, config, fallbackGroup),
      priority: upstreamAccountPriorityValue(config, applySuggested),
      weight: upstreamAccountWeightValue(config, applySuggested),
    }
  })
}

export function upstreamAccountFromChannelAccount(
  account: ChannelAccount
): UpstreamEditableAccount {
  const syncMetadata = parseSettingsRecord(account.settings)[
    UPSTREAM_ACCOUNT_SYNC_SETTINGS_KEY
  ]
  const upstreamExternalId =
    syncMetadata && typeof syncMetadata === 'object'
      ? String(
          (syncMetadata as Record<string, unknown>).external_id || ''
        ).trim()
      : ''
  const upstreamConfigId =
    upstreamExternalId || account.key || String(account.id)

  return {
    account_id: account.id,
    sync_id: upstreamConfigId,
    external_id: upstreamConfigId,
    name: account.name || `#${account.id}`,
    masked_key: account.key,
    status: account.status,
    account_status: account.status,
    group_name: account.key_group_name || account.group,
    group_id: account.key_group_id || account.group,
    models: parseModelsString(account.models || ''),
    model_ratios: account.model_ratios,
    group_ratio: account.group_ratio ?? undefined,
    effective_ratio: account.effective_ratio,
    ratio_conversion: account.ratio_conversion,
    suggested_priority: account.priority || 0,
    suggested_weight: account.weight || 0,
  }
}

export function buildUpstreamAccountConfigsFromChannelAccounts(
  accounts: ChannelAccount[]
): Record<string, UpstreamAccountConfigDraft> {
  const configs: Record<string, UpstreamAccountConfigDraft> = {}
  accounts.forEach((account) => {
    const key = upstreamAccountFromChannelAccount(account)
    configs[upstreamKeyConfigId(key, account.id)] = {
      priority: account.priority || 0,
      weight: account.weight || 0,
      enabled: account.status === CHANNEL_STATUS.ENABLED,
      models: account.models || '',
      group: account.group || '',
    }
  })
  return configs
}

// 表单辅助函数
const createEmptyModelMappingGuardrail = (): ModelMappingGuardrail => ({
  invalidJson: false,
  entries: [],
  missingSourceModels: [],
  exposedTargetModels: [],
})

const formatModelNames = (models: string[]): string =>
  models.map((model) => `"${model}"`).join(', ')

const MODEL_MAPPING_PREVIEW_FALLBACK: Array<{
  source: string
  target: string
}> = [{ source: 'client-model', target: 'upstream-model' }]

const ADVANCED_SETTINGS_EXPANDED_KEY = 'channel-advanced-settings-expanded'
const CHANNEL_EDITOR_SECTION_IDS = {
  identity: 'channel-section-identity',
  credentials: 'channel-section-credentials',
  models: 'channel-section-models',
  advanced: 'channel-section-advanced',
} as const
const CHANNEL_EDITOR_MAIN_SECTION_IDS = [
  CHANNEL_EDITOR_SECTION_IDS.identity,
  CHANNEL_EDITOR_SECTION_IDS.credentials,
  CHANNEL_EDITOR_SECTION_IDS.models,
  CHANNEL_EDITOR_SECTION_IDS.advanced,
]
const ADVANCED_SETTINGS_SECTION_IDS = {
  routingStrategy: 'channel-section-advanced-routing-strategy',
  internalNotes: 'channel-section-advanced-internal-notes',
  overrideRules: 'channel-section-advanced-override-rules',
  extraSettings: 'channel-section-advanced-extra-settings',
  fieldPassthrough: 'channel-section-advanced-field-passthrough',
  upstreamModelDetection: 'channel-section-advanced-upstream-model-detection',
} as const
const ADVANCED_SETTINGS_CHILD_SECTION_IDS: string[] = Object.values(
  ADVANCED_SETTINGS_SECTION_IDS
)
const ADVANCED_CUSTOM_ROUTE_TYPE_PREVIEW_LIMIT = 3
const UPSTREAM_DETECTED_MODEL_PREVIEW_LIMIT = 8
const MODEL_SEARCH_RESULT_PREVIEW_LIMIT = 6

function readAdvancedSettingsPreference(): boolean {
  if (typeof window === 'undefined') return false
  return window.localStorage.getItem(ADVANCED_SETTINGS_EXPANDED_KEY) === 'true'
}

function hasAdvancedSettingsValues(values: ChannelFormValues): boolean {
  return Boolean(
    hasConfiguredOverrideValue(values.param_override) ||
    hasConfiguredOverrideValue(values.header_override) ||
    values.advanced_custom?.trim() ||
    hasConfiguredOverrideValue(values.status_code_mapping) ||
    values.tag?.trim() ||
    values.remark?.trim() ||
    values.priority ||
    values.weight ||
    values.proxy?.trim() ||
    values.system_prompt?.trim() ||
    values.force_format ||
    values.thinking_to_content ||
    values.pass_through_body_enabled ||
    values.disable_task_polling_sleep ||
    values.system_prompt_override ||
    values.claude_beta_query ||
    values.upstream_model_update_check_enabled ||
    values.upstream_model_update_auto_sync_enabled ||
    values.upstream_model_update_ignored_models?.trim()
  )
}

function hasConfiguredOverrideValue(value: unknown): boolean {
  if (typeof value !== 'string') return false

  const trimmed = value.trim()
  if (!trimmed || trimmed === 'null') return false

  try {
    const parsed = JSON.parse(trimmed)
    if (parsed === null) return false
    if (Array.isArray(parsed)) return parsed.length > 0
    if (typeof parsed === 'object') return Object.keys(parsed).length > 0
  } catch {
    return true
  }

  return true
}

function parseSettingsRecord(
  settings: string | undefined
): Record<string, unknown> {
  if (!settings?.trim()) return {}
  try {
    const parsed = JSON.parse(settings)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
  } catch {
    return {}
  }
  return {}
}

function formatUnixTime(timestamp: unknown): string {
  const seconds = Number(timestamp)
  if (!Number.isFinite(seconds) || seconds <= 0) return '-'
  return new Date(seconds * 1000).toLocaleString()
}

function CardHeading({
  title,
  description,
  icon,
}: {
  title: string
  description?: string
  icon?: ReactNode
}) {
  return (
    <div className='flex items-start gap-3'>
      {icon && (
        <span className='bg-primary/10 text-primary flex size-8 shrink-0 items-center justify-center rounded-md'>
          {icon}
        </span>
      )}
      <div className='min-w-0 flex-1'>
        <h3 className='text-sm leading-none font-semibold tracking-tight'>
          {title}
        </h3>
        {description && (
          <p className='text-muted-foreground mt-1 text-xs leading-5'>
            {description}
          </p>
        )}
      </div>
    </div>
  )
}

function SubHeading({ title, icon }: { title: string; icon?: ReactNode }) {
  return (
    <div className='flex items-center gap-2'>
      {icon && <span className='text-muted-foreground'>{icon}</span>}
      <h4 className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
        {title}
      </h4>
    </div>
  )
}

function configuredAdvancedSectionClassName(
  className: string,
  configured: boolean
) {
  return cn(
    className,
    'border-border/60 rounded-lg border p-3 transition-colors',
    configured && 'border-primary/35 ring-primary/20 ring-1'
  )
}

function getSectionStatusIcon(status: ChannelEditorSectionStatus): ReactNode {
  if (status === 'error') {
    return <AlertCircle className='h-3.5 w-3.5' aria-hidden='true' />
  }
  if (status === 'complete' || status === 'configured') {
    return <CheckCircle2 className='h-3.5 w-3.5' aria-hidden='true' />
  }
  return <Circle className='h-3.5 w-3.5' aria-hidden='true' />
}

function getCompletionStatus(
  hasErrors: boolean,
  isComplete: boolean
): ChannelEditorSectionStatus {
  if (hasErrors) return 'error'
  if (isComplete) return 'complete'
  return 'idle'
}

function getSectionStatusLabel(
  status: ChannelEditorSectionStatus,
  t: (key: string) => string
): string {
  if (status === 'error') return t('Error')
  if (status === 'complete' || status === 'configured') return t('Ready')
  return t('Incomplete')
}

function ChannelEditorNav(props: {
  providerLogo: ReactNode
  providerLabel: string
  statusLabel: string
  progressLabel: string
  navigationLabel: string
  items: ChannelEditorNavItem[]
  activeItemId?: string
  expandedItemId?: string
  onNavigate: (targetId: string) => void
}) {
  const renderStatusMarker = (item: ChannelEditorNavItem) => {
    const isError = item.status === 'error'
    const isDone = item.status === 'complete' || item.status === 'configured'
    const isConfigured = Boolean(item.configured)

    if (isConfigured && !isError && !isDone) {
      return (
        <span
          className='bg-success block size-2 rounded-full'
          aria-hidden='true'
        />
      )
    }

    return getSectionStatusIcon(item.status)
  }

  const renderNavButton = (
    item: ChannelEditorNavItem,
    layout: 'horizontal' | 'vertical'
  ) => {
    const isError = item.status === 'error'
    const isDone = item.status === 'complete' || item.status === 'configured'
    const isConfigured = Boolean(item.configured)
    const isActive = props.activeItemId === item.id

    return (
      <button
        key={item.id}
        type='button'
        className={cn(
          'hover:bg-muted/60 flex items-start gap-2 rounded-md px-2 py-2 text-left transition-colors',
          layout === 'horizontal' && 'min-w-[9.5rem] shrink-0',
          layout === 'vertical' && 'w-full',
          isActive && 'bg-muted/80',
          isConfigured && !isError && 'text-primary',
          isError && 'text-destructive hover:bg-destructive/10'
        )}
        onClick={() => props.onNavigate(item.id)}
        aria-current={isActive ? 'true' : undefined}
      >
        <span
          className={cn(
            'bg-muted text-muted-foreground mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md',
            isConfigured && !isError && 'bg-primary/10 text-primary',
            isError && 'bg-destructive/10 text-destructive',
            isDone && !isError && 'text-primary'
          )}
        >
          {item.icon}
        </span>
        <span className='min-w-0 flex-1'>
          <span
            className={cn(
              'block text-sm font-medium',
              layout === 'horizontal' ? 'truncate' : 'leading-4 break-words'
            )}
          >
            {item.title}
          </span>
          {item.description && (
            <span
              className={cn(
                'text-muted-foreground block text-xs',
                layout === 'horizontal' ? 'truncate' : 'leading-4 break-words'
              )}
            >
              {item.description}
            </span>
          )}
        </span>
        <span
          className={cn(
            'text-muted-foreground mt-1 shrink-0',
            isError && 'text-destructive',
            isDone && !isError && 'text-primary',
            isConfigured && !isError && !isDone && 'pt-1.5'
          )}
          aria-label={item.statusLabel}
        >
          {renderStatusMarker(item)}
        </span>
      </button>
    )
  }

  return (
    <>
      <div className='bg-background/95 supports-[backdrop-filter]:bg-background/80 sticky top-0 z-20 -mx-1 py-1 backdrop-blur lg:hidden'>
        <div className='border-border/60 bg-background rounded-lg border p-2 shadow-sm'>
          <div className='flex flex-col gap-2 xl:flex-row xl:items-center'>
            <div className='bg-muted/30 flex min-w-0 items-center gap-2 rounded-md border px-2 py-2 xl:w-56'>
              <span className='bg-background flex size-8 shrink-0 items-center justify-center rounded-md border'>
                {props.providerLogo}
              </span>
              <div className='min-w-0'>
                <p className='truncate text-sm font-medium'>
                  {props.providerLabel}
                </p>
                <p className='text-muted-foreground truncate text-xs'>
                  {props.statusLabel} · {props.progressLabel}
                </p>
              </div>
            </div>

            <nav
              className='flex min-w-0 flex-1 gap-1 overflow-x-auto pb-0.5'
              aria-label={props.navigationLabel}
            >
              {props.items.map((item) => renderNavButton(item, 'horizontal'))}
            </nav>
          </div>

          {props.items.map((item) => {
            const isExpanded = props.expandedItemId === item.id
            if (!item.children || !isExpanded) return null

            return (
              <div
                key={`${item.id}-children`}
                className='border-border/60 mt-2 flex gap-1 overflow-x-auto border-t pt-2'
              >
                {item.children.map((child) => (
                  <button
                    key={child.id}
                    type='button'
                    className={cn(
                      'text-muted-foreground hover:bg-muted/50 hover:text-foreground flex min-w-fit items-center gap-2 rounded-md px-2 py-1 text-left text-xs transition-colors',
                      child.configured && 'text-primary'
                    )}
                    onClick={() => props.onNavigate(child.id)}
                  >
                    <span className='truncate'>{child.title}</span>
                    {child.configured && (
                      <span
                        className='bg-success size-1.5 shrink-0 rounded-full'
                        aria-hidden='true'
                      />
                    )}
                  </button>
                ))}
              </div>
            )
          })}
        </div>
      </div>

      <aside className='hidden self-start lg:sticky lg:top-4 lg:z-20 lg:block'>
        <div className='flex max-h-[calc(100dvh-12rem)] flex-col gap-3 overflow-y-auto overscroll-contain pr-1'>
          <div className='border-border/60 bg-muted/20 rounded-lg border p-3'>
            <div className='flex min-w-0 items-center gap-2'>
              <span className='bg-background flex size-8 shrink-0 items-center justify-center rounded-md border'>
                {props.providerLogo}
              </span>
              <div className='min-w-0'>
                <p className='truncate text-sm font-medium'>
                  {props.providerLabel}
                </p>
                <p className='text-muted-foreground truncate text-xs'>
                  {props.statusLabel} · {props.progressLabel}
                </p>
              </div>
            </div>
          </div>

          <nav
            className='border-border/60 bg-background rounded-lg border p-1'
            aria-label={props.navigationLabel}
          >
            {props.items.map((item) => {
              const isExpanded = props.expandedItemId === item.id
              return (
                <div key={item.id}>
                  {renderNavButton(item, 'vertical')}
                  {item.children && isExpanded && (
                    <div className='border-border/60 ml-5 flex flex-col gap-0.5 border-l py-1 pl-3'>
                      {item.children.map((child) => (
                        <button
                          key={child.id}
                          type='button'
                          className={cn(
                            'text-muted-foreground hover:bg-muted/50 hover:text-foreground flex w-full items-center gap-2 rounded-md px-2 py-1 text-left text-xs transition-colors',
                            child.configured && 'text-primary'
                          )}
                          onClick={() => props.onNavigate(child.id)}
                        >
                          <span className='min-w-0 flex-1 truncate'>
                            {child.title}
                          </span>
                          {child.configured && (
                            <span
                              className='bg-success size-1.5 shrink-0 rounded-full'
                              aria-hidden='true'
                            />
                          )}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </nav>
        </div>
      </aside>
    </>
  )
}

export function ChannelMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: ChannelMutateDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { setOpen } = useChannels()
  const permissions = useChannelPermissions()
  const noPermissionMessage = t("You don't have necessary permission")
  // 表单实例初始化。
  const form = useForm<
    z.input<typeof channelFormSchema>,
    unknown,
    ChannelFormValues
  >({
    resolver: zodResolver(channelFormSchema),
    defaultValues: CHANNEL_FORM_DEFAULT_VALUES,
  })
  const currentType = form.watch('type')
  const [fetchModelsDialogOpen, setFetchModelsDialogOpen] = useState(false)
  const [channelKey, setChannelKey] = useState<string | null>(null)
  const [isChannelKeyLoading, setIsChannelKeyLoading] = useState(false)
  const [codexOAuthDialogOpen, setCodexOAuthDialogOpen] = useState(false)
  const [isCodexCredentialRefreshing, setIsCodexCredentialRefreshing] =
    useState(false)
  const [upstreamPlatform, setUpstreamPlatform] =
    useState<UpstreamAccountPlatform>('new-api')
  const [upstreamBaseUrl, setUpstreamBaseUrl] = useState('')
  const [upstreamUsername, setUpstreamUsername] = useState('')
  const [upstreamPassword, setUpstreamPassword] = useState('')
  const [upstreamUseSavedCredential, setUpstreamUseSavedCredential] =
    useState(false)
  const [upstreamPaidCny, setUpstreamPaidCny] = useState('')
  const [upstreamPlatformUsdCredit, setUpstreamPlatformUsdCredit] = useState('')
  const [upstreamPreviewId, setUpstreamPreviewId] = useState('')
  const [upstreamPreviewExpiresAt, setUpstreamPreviewExpiresAt] = useState(0)
  const [upstreamSnapshot, setUpstreamSnapshot] =
    useState<UpstreamAccountSnapshot | null>(null)
  const [upstreamTwoFactorChallenge, setUpstreamTwoFactorChallenge] =
    useState<UpstreamAccountTwoFactorChallenge | null>(null)
  const [upstreamTwoFactorCode, setUpstreamTwoFactorCode] = useState('')
  const [upstreamRefreshPreviewExpiresAt, setUpstreamRefreshPreviewExpiresAt] =
    useState(0)
  const [upstreamRefreshPreviewId, setUpstreamRefreshPreviewId] = useState('')
  const [upstreamRefreshSnapshot, setUpstreamRefreshSnapshot] =
    useState<UpstreamAccountSnapshot | null>(null)
  const [
    upstreamRefreshTwoFactorChallenge,
    setUpstreamRefreshTwoFactorChallenge,
  ] = useState<UpstreamAccountTwoFactorChallenge | null>(null)
  const [upstreamRefreshTwoFactorCode, setUpstreamRefreshTwoFactorCode] =
    useState('')
  const [upstreamPreviewNowMs, setUpstreamPreviewNowMs] = useState(() =>
    Date.now()
  )
  const [upstreamApplySuggested, setUpstreamApplySuggested] = useState(true)
  const [upstreamAccountConfigs, setUpstreamAccountConfigs] = useState<
    Record<string, UpstreamAccountConfigDraft>
  >({})
  const upstreamRatioConversion = useMemo(
    () =>
      buildUpstreamRatioConversionPayload(
        upstreamPaidCny,
        upstreamPlatformUsdCredit
      ),
    [upstreamPaidCny, upstreamPlatformUsdCredit]
  )
  const [isSavingSyncedAccountConfigs, setIsSavingSyncedAccountConfigs] =
    useState(false)
  const initialModelsRef = useRef<string[]>([])
  const initialModelMappingRef = useRef<string>('')
  const initialStatusCodeMappingRef = useRef<string>('')
  const upstreamCredentialFingerprintRef = useRef('')
  const [statusCodeRiskOpen, setStatusCodeRiskOpen] = useState(false)
  const [statusCodeRiskDetailItems, setStatusCodeRiskDetailItems] = useState<
    string[]
  >([])
  const statusCodeRiskResolveRef = useRef<
    ((confirmed: boolean) => void) | null
  >(null)
  const [missingModelsDialogOpen, setMissingModelsDialogOpen] = useState(false)
  const [missingModelsList, setMissingModelsList] = useState<string[]>([])
  const missingModelsResolveRef = useRef<
    ((action: MissingModelsAction) => void) | null
  >(null)
  const modelSearchPointerHandledRef = useRef(false)
  const channelFormRef = useRef<HTMLFormElement>(null)
  const advancedNavScrollPendingRef = useRef(false)
  const [advancedSettingsOpen, setAdvancedSettingsOpen] = useState(false)
  const [syncRefreshOpen, setSyncRefreshOpen] = useState(false)
  const [paramOverrideEditorOpen, setParamOverrideEditorOpen] = useState(false)
  const [advancedCustomEditorOpen, setAdvancedCustomEditorOpen] =
    useState(false)
  const [modelSearchValue, setModelSearchValue] = useState('')
  const [modelSearchOpen, setModelSearchOpen] = useState(false)
  const [activeEditorSectionId, setActiveEditorSectionId] = useState<string>(
    CHANNEL_EDITOR_SECTION_IDS.identity
  )
  const [expandedEditorNavItemId, setExpandedEditorNavItemId] = useState<
    string | undefined
  >()
  const renderModeRef = useRef<'create' | 'edit'>('create')
  const renderCurrentRowRef = useRef<Channel | null>(null)

  // Sheet 关闭动画期间父级会先把 open 状态清空，如果此时直接根据 currentRow
  // 渲染，会把正在关闭的编辑抽屉短暂切成创建抽屉。这里在打开期间锁定渲染模式，
  // 只有下一次真正打开时才切换创建/编辑内容，避免保存成功后误显示创建表单。
  if (open) {
    renderModeRef.current = currentRow ? 'edit' : 'create'
    renderCurrentRowRef.current = currentRow ?? null
  }
  const renderCurrentRow =
    renderModeRef.current === 'edit' ? renderCurrentRowRef.current : null

  const isEditing = Boolean(renderCurrentRow)
  const channelId = renderCurrentRow?.id ?? null
  const canSubmitForm = isEditing
    ? permissions.canWrite || permissions.canSensitiveWrite
    : permissions.canSensitiveWrite
  const canEditSensitiveFields = permissions.canSensitiveWrite
  const canEditBasicFields =
    permissions.canWrite || permissions.canSensitiveWrite
  const debouncedModelSearchValue = useDebounce(modelSearchValue, 300)
  const trimmedModelSearchValue = modelSearchValue.trim()
  const trimmedDebouncedModelSearchValue = debouncedModelSearchValue.trim()

  // 编辑渠道时拉取完整渠道详情，用于回填表单和保留历史配置。
  const { data: channelData, isLoading: isChannelLoading } = useQuery({
    queryKey: channelsQueryKeys.detail(renderCurrentRow?.id || 0),
    queryFn: () => getChannel(renderCurrentRow!.id),
    enabled: isEditing && Boolean(renderCurrentRow?.id),
  })

  // 拉取 NexusTok 用户分组，渠道仍然需要用它做路由、权限和计费归属。
  const { data: groupsData, isLoading: isLoadingGroups } = useQuery({
    queryKey: ['groups'],
    queryFn: getGroups,
  })

  // 拉取当前系统可见模型，供渠道模型选择器和快捷填充使用。
  const { data: allModelsData } = useQuery({
    queryKey: ['channel_models'],
    queryFn: getAllModels,
  })

  const modelSearchVendor = useMemo(
    () => getModelSearchVendorForChannelType(currentType),
    [currentType]
  )

  const shouldSearchModels = trimmedDebouncedModelSearchValue.length >= 2

  // 在渠道编辑页搜索模型元信息库，把已同步但尚未绑定到渠道的模型纳入可选候选。
  // 例如 OpenAI 的 gpt-5.6 系列有 Terra/Luna/Sol 三个真实模型，不能只依赖
  // `/api/channel/models` 中已经绑定过的能力列表。
  const {
    data: modelSearchData,
    isFetching: isModelSearchFetching,
    isError: isModelSearchError,
  } = useQuery({
    queryKey: [
      'channel-model-search',
      modelSearchVendor,
      trimmedDebouncedModelSearchValue,
    ],
    queryFn: () =>
      searchModels({
        keyword: trimmedDebouncedModelSearchValue,
        vendor: modelSearchVendor || undefined,
        p: 1,
        page_size: 50,
      }),
    enabled: open && shouldSearchModels,
    placeholderData: (previousData) => previousData,
  })

  // 拉取模型预设分组，便于管理员快速批量填入常用模型集合。
  const { data: prefillGroupsData } = useQuery({
    queryKey: ['prefill_groups', 'model'],
    queryFn: () => getPrefillGroups('model'),
  })

  const { data: accountPoolGroupsData } = useQuery({
    queryKey: ['account-pool', 'groups', 'options'],
    queryFn: getAccountPoolGroupOptions,
  })

  const upstreamPreviewMutation = useMutation({
    mutationFn: previewUpstreamAccount,
  })

  const upstreamPreview2FAMutation = useMutation({
    mutationFn: completeUpstreamAccountPreview2FA,
  })

  const upstreamPreviewRemaining = upstreamPreviewRemainingSeconds(
    upstreamPreviewExpiresAt,
    upstreamPreviewNowMs
  )
  const upstreamTwoFactorRemaining = upstreamPreviewRemainingSeconds(
    upstreamTwoFactorChallenge?.expires_at ?? 0,
    upstreamPreviewNowMs
  )
  const upstreamRefreshPreviewRemaining = upstreamPreviewRemainingSeconds(
    upstreamRefreshPreviewExpiresAt,
    upstreamPreviewNowMs
  )
  const upstreamRefreshTwoFactorRemaining = upstreamPreviewRemainingSeconds(
    upstreamRefreshTwoFactorChallenge?.expires_at ?? 0,
    upstreamPreviewNowMs
  )
  const isUpstreamPreviewExpired = Boolean(
    upstreamSnapshot &&
    upstreamPreviewExpiresAt &&
    upstreamPreviewRemaining <= 0
  )
  const isUpstreamRefreshPreviewExpired = Boolean(
    upstreamRefreshSnapshot &&
    upstreamRefreshPreviewExpiresAt &&
    upstreamRefreshPreviewRemaining <= 0
  )
  const isUpstreamTwoFactorExpired = Boolean(
    upstreamTwoFactorChallenge && upstreamTwoFactorRemaining <= 0
  )
  const isUpstreamRefreshTwoFactorExpired = Boolean(
    upstreamRefreshTwoFactorChallenge && upstreamRefreshTwoFactorRemaining <= 0
  )

  const clearUpstreamCreatePreview = useCallback(() => {
    setUpstreamPreviewId('')
    setUpstreamPreviewExpiresAt(0)
    setUpstreamSnapshot(null)
    setUpstreamTwoFactorChallenge(null)
    setUpstreamTwoFactorCode('')
  }, [])

  const clearUpstreamRefreshPreview = useCallback(() => {
    setUpstreamRefreshPreviewId('')
    setUpstreamRefreshPreviewExpiresAt(0)
    setUpstreamRefreshSnapshot(null)
    setUpstreamRefreshTwoFactorChallenge(null)
    setUpstreamRefreshTwoFactorCode('')
  }, [])

  const clearAllUpstreamPreviews = useCallback(() => {
    clearUpstreamCreatePreview()
    clearUpstreamRefreshPreview()
    setUpstreamAccountConfigs({})
  }, [clearUpstreamCreatePreview, clearUpstreamRefreshPreview])

  const showUpstreamPreviewExpiredToast = useCallback(() => {
    toast.error(
      t(
        'The upstream account preview expired or was already used. Sync the upstream account again.'
      )
    )
  }, [t])

  useEffect(() => {
    if (
      !upstreamPreviewExpiresAt &&
      !upstreamRefreshPreviewExpiresAt &&
      !upstreamTwoFactorChallenge?.expires_at &&
      !upstreamRefreshTwoFactorChallenge?.expires_at
    ) {
      return
    }

    setUpstreamPreviewNowMs(Date.now())
    const timer = window.setInterval(() => {
      setUpstreamPreviewNowMs(Date.now())
    }, 1000)

    return () => window.clearInterval(timer)
  }, [
    upstreamPreviewExpiresAt,
    upstreamRefreshPreviewExpiresAt,
    upstreamRefreshTwoFactorChallenge?.expires_at,
    upstreamTwoFactorChallenge?.expires_at,
  ])

  useEffect(() => {
    if (isEditing) return
    const platform = upstreamPlatformFromChannelType(currentType)
    if (platform) {
      setUpstreamPlatform(platform)
      form.setValue('upstream_account_sync', true, {
        shouldDirty: false,
        shouldValidate: true,
      })
      if (!(form.getValues('group') || []).length) {
        form.setValue('group', ['default'], {
          shouldDirty: true,
          shouldValidate: true,
        })
      }
      return
    }
    form.setValue('upstream_account_sync', false, {
      shouldDirty: false,
      shouldValidate: true,
    })
    clearAllUpstreamPreviews()
  }, [clearAllUpstreamPreviews, currentType, form, isEditing])

  const { copyToClipboard } = useCopyToClipboard()

  const {
    open: verificationOpen,
    methods: verificationMethods,
    state: verificationState,
    executeVerification,
    withVerification,
    cancel: cancelVerification,
    setCode: setVerificationCode,
    switchMethod: switchVerificationMethod,
  } = useSecureVerification()

  useEffect(() => {
    if (!open) {
      setChannelKey(null)
      setIsChannelKeyLoading(false)
    } else if (channelId) {
      setChannelKey(null)
    }
  }, [open, channelId])

  // 判断当前编辑对象是否为多 Key 渠道，决定是否展示追加/覆盖等历史密钥管理入口。
  const isMultiKeyChannel =
    isEditing && channelData?.data?.channel_info?.is_multi_key === true
  const isChannelDetailLoading = isEditing && isChannelLoading
  const sensitiveFieldsReadOnly = isEditing && !canEditSensitiveFields

  // 监听表单字段变化，用于驱动凭证模式、渠道类型和高级配置的条件渲染。
  const multiKeyMode = form.watch('multi_key_mode')
  const multiKeyType = form.watch('multi_key_type')
  const credentialMode = form.watch('credential_mode')
  const accountPoolGroupId = form.watch('account_pool_group_id')
  const keyMode = form.watch('key_mode')
  const currentGroups = form.watch('group')
  const currentStatus = form.watch('status')
  const currentBaseUrl = form.watch('base_url')
  const currentKey = form.watch('key')
  const currentOther = form.watch('other')
  const currentModels = form.watch('models')
  const currentName = form.watch('name')
  const currentModelMapping = form.watch('model_mapping')
  const vertexKeyType = form.watch('vertex_key_type')
  const awsKeyType = form.watch('aws_key_type')
  const upstreamModelUpdateCheckEnabled = form.watch(
    'upstream_model_update_check_enabled'
  )
  const currentSettings = form.watch('settings')
  const currentAdvancedCustom = form.watch('advanced_custom')
  const currentFormValues = form.watch()

  useEffect(() => {
    const fingerprint = [
      upstreamBaseUrl || '',
      upstreamPlatform,
      upstreamUsername,
      upstreamPassword,
      upstreamUseSavedCredential ? 'saved' : 'manual',
    ].join('\n')
    if (!upstreamCredentialFingerprintRef.current) {
      upstreamCredentialFingerprintRef.current = fingerprint
      return
    }
    if (upstreamCredentialFingerprintRef.current === fingerprint) return
    upstreamCredentialFingerprintRef.current = fingerprint
    if (
      !upstreamSnapshot &&
      !upstreamRefreshSnapshot &&
      !upstreamTwoFactorChallenge &&
      !upstreamRefreshTwoFactorChallenge
    ) {
      return
    }
    clearAllUpstreamPreviews()
  }, [
    clearAllUpstreamPreviews,
    upstreamBaseUrl,
    upstreamPassword,
    upstreamPlatform,
    upstreamUseSavedCredential,
    upstreamRefreshTwoFactorChallenge,
    upstreamRefreshSnapshot,
    upstreamSnapshot,
    upstreamTwoFactorChallenge,
    upstreamUsername,
  ])

  const {
    unlocked: doubaoApiEditUnlocked,
    handleClick: handleApiConfigSecretClick,
    reset: resetDoubaoApiUnlock,
  } = useHiddenClickUnlock({
    requiredClicks: 10,
    disabled: currentType !== 45,
    onUnlock: () => {
      toast.info(t('Doubao custom API address editing unlocked'))
    },
  })

  useEffect(() => {
    if (!open) {
      resetDoubaoApiUnlock()
    }
  }, [open, resetDoubaoApiUnlock])

  // 根据表单状态计算渲染分支。
  const isBatchMode =
    multiKeyMode === 'batch' || multiKeyMode === 'multi_to_single'
  const isGlobalAccountPoolMode = credentialMode === 'global_account_pool'
  const isLegacyChannelAccountPoolMode = credentialMode === 'account_pool'
  const hasUpstreamAccountSyncMetadata = isUpstreamAccountSyncChannel(
    channelData?.data ?? renderCurrentRow
  )
  const isUpstreamAccountSyncedChannel =
    isEditing && hasUpstreamAccountSyncMetadata
  const forcedUpstreamPlatform = upstreamPlatformFromChannelType(currentType)
  const currentTypeRequiresUpstreamSync = Boolean(forcedUpstreamPlatform)
  const isCreateUpstreamSyncMode = !isEditing && currentTypeRequiresUpstreamSync
  const isEditingUnsupportedUnsyncedUpstreamType =
    isEditing &&
    currentTypeRequiresUpstreamSync &&
    !isUpstreamAccountSyncedChannel
  const usesUpstreamAccountCredentialSource =
    currentTypeRequiresUpstreamSync || isUpstreamAccountSyncedChannel
  // 同步模式下渠道模型由逐 key 配置主导，但渠道分组仍然决定 NexusTok 用户可见性，
  // 因此保留该区块，仅隐藏共享模型编辑能力。
  const showSharedModelsSection = true
  const showManualCredentialSection = !usesUpstreamAccountCredentialSource
  const supportsMultiKeyAddMode =
    currentType !== 57 && !(currentType === 41 && vertexKeyType === 'api_key')
  const syncedChannelAccountsQuery = useQuery({
    queryKey: [
      ...channelsQueryKeys.detail(channelId || 0),
      'upstream-sync-accounts',
    ],
    queryFn: () =>
      getChannelAccounts(channelId!, {
        p: 1,
        page_size: 100,
      }),
    enabled:
      open &&
      Boolean(channelId) &&
      isUpstreamAccountSyncedChannel &&
      permissions.canReadChannelAccount,
  })
  // 创建渠道时查询不会启用，这里必须给空列表一个稳定引用。
  // 否则 `undefined ?? []` 会在每次渲染时生成新数组，触发依赖它的 useEffect
  // 反复执行 `form.reset`，最终让创建抽屉进入 React 最大更新深度错误。
  const syncedChannelAccounts = useMemo(
    () => syncedChannelAccountsQuery.data?.data?.accounts.items ?? [],
    [syncedChannelAccountsQuery.data?.data?.accounts.items]
  )
  const syncedChannelAccountsTotal =
    syncedChannelAccountsQuery.data?.data?.accounts.total ?? 0
  const syncedChannelAccountsLoadedCount = syncedChannelAccounts.length
  const syncedEditableAccounts = useMemo(
    () => syncedChannelAccounts.map(upstreamAccountFromChannelAccount),
    [syncedChannelAccounts]
  )

  const credentialModeOptions = useMemo(() => {
    const options = [
      {
        value: 'single_key',
        label: t('Single Key'),
      },
      ...(supportsMultiKeyAddMode || isEditing || credentialMode === 'multi_key'
        ? [
            {
              value: 'multi_key',
              label: t('Multi-Key Rotation'),
            },
          ]
        : []),
      {
        value: 'global_account_pool',
        label: t('Account Pool'),
      },
    ]

    if (isLegacyChannelAccountPoolMode) {
      options.push({
        value: 'account_pool',
        label: t('Legacy Channel Account Pool'),
      })
    }

    return options
  }, [
    credentialMode,
    isEditing,
    isLegacyChannelAccountPoolMode,
    supportsMultiKeyAddMode,
    t,
  ])

  const addModeOptions = useMemo(
    () =>
      supportsMultiKeyAddMode
        ? ADD_MODE_OPTIONS
        : ADD_MODE_OPTIONS.filter((option) => option.value === 'single'),
    [supportsMultiKeyAddMode]
  )

  useEffect(() => {
    if (isEditing || supportsMultiKeyAddMode) return
    if (credentialMode === 'multi_key') {
      form.setValue('credential_mode', 'single_key', {
        shouldDirty: true,
        shouldValidate: true,
      })
    }
    if (multiKeyMode && multiKeyMode !== 'single') {
      form.setValue('multi_key_mode', 'single', {
        shouldDirty: true,
        shouldValidate: true,
      })
    }
  }, [credentialMode, form, isEditing, multiKeyMode, supportsMultiKeyAddMode])

  // 汇总系统模型列表。
  const allModelsList = useMemo(
    () => allModelsData?.data?.map((model) => model.id).filter(Boolean) || [],
    [allModelsData]
  )

  // 模型预设分组列表。
  const prefillGroups = useMemo(
    () => prefillGroupsData?.data || [],
    [prefillGroupsData]
  )

  const accountPoolGroupOptions = useMemo(
    () =>
      accountPoolGroupsData?.data?.map((group) => {
        const dailyLimitLabel =
          group.daily_limit_state?.limit_type === 'daily_request'
            ? t('Daily request limit reached')
            : group.daily_limit_state?.limit_type === 'daily_quota'
              ? t('Daily quota limit reached')
              : t('Daily limit reached')
        const dailyLimitSuffix = group.daily_limit_state?.limited
          ? ` · ${dailyLimitLabel}`
          : ''
        const preflightSuffix =
          group.preflight_check_mode === 'warmup'
            ? ` · ${t('Warm up stale accounts')}`
            : group.preflight_check_mode === 'require_recent'
              ? ` · ${t('Require recent check')}`
              : ''
        return {
          value: String(group.id),
          label: `${group.name} · ${group.platform}/${group.auth_type}${dailyLimitSuffix}${preflightSuffix}`,
        }
      }) ?? [],
    [accountPoolGroupsData, t]
  )

  // 将用户分组转换成多选组件选项，同时保留当前渠道已有但接口暂未返回的历史分组。
  const groupOptions = useMemo(() => {
    if (!groupsData?.data) return []
    const allGroups = new Set([...groupsData.data, ...(currentGroups || [])])
    return Array.from(allGroups).map((group) => ({
      value: group,
      label: group,
    }))
  }, [groupsData, currentGroups])

  // 将当前模型字符串解析成数组，供模型映射和多选组件复用。
  const currentModelsArray = useMemo(
    () => parseModelsString(currentModels),
    [currentModels]
  )

  // 按渠道类型推导基础模型集合。
  const basicModels = useMemo(() => {
    if (!allModelsList.length) return []
    // OpenAI 类型只优先填充常见文本模型，避免把无关 provider 的模型一起塞入渠道。
    if (currentType === 1) {
      return allModelsList.filter(
        (model) => model.startsWith('gpt-') || model.startsWith('text-')
      )
    }
    return allModelsList
  }, [allModelsList, currentType])

  const advancedCustomStats = useMemo(
    () => getAdvancedCustomStats(currentAdvancedCustom),
    [currentAdvancedCustom]
  )
  const advancedCustomRouteTypeLabels =
    advancedCustomStats.routeTypeLabels.slice(
      0,
      ADVANCED_CUSTOM_ROUTE_TYPE_PREVIEW_LIMIT
    )
  const hiddenAdvancedCustomRouteTypeCount =
    advancedCustomStats.routeTypeLabels.length -
    advancedCustomRouteTypeLabels.length
  const advancedCustomRouteTypeTitle =
    hiddenAdvancedCustomRouteTypeCount > 0
      ? advancedCustomStats.routeTypeLabels.join(', ')
      : undefined

  const renderUpstreamPreviewExpiryNotice = useCallback(
    (remainingSeconds: number, expired: boolean) => (
      <Alert>
        <AlertCircle aria-hidden='true' />
        <AlertDescription>
          {expired
            ? t(
                'This upstream account preview expired. Sync the upstream account again before saving.'
              )
            : t(
                'This upstream account preview expires in {{time}}. Sync again if it expires before you save.',
                {
                  time: formatUpstreamPreviewRemaining(remainingSeconds),
                }
              )}
        </AlertDescription>
      </Alert>
    ),
    [t]
  )

  const renderUpstreamSnapshotReview = useCallback(
    (
      snapshot: Pick<UpstreamAccountSnapshot, 'balance' | 'keys'>,
      options: {
        showBalance?: boolean
        showSuggestedToggle?: boolean
        emptyText?: string
      } = {}
    ) => {
      const showSuggestedToggle = options.showSuggestedToggle !== false
      return (
        <div className='flex flex-col gap-3'>
          {options.showBalance !== false && (
            <div className='grid gap-3 sm:grid-cols-3'>
              <div className='rounded-md border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Synced Keys')}
                </div>
                <div className='text-lg font-semibold'>
                  {snapshot.keys.length}
                </div>
              </div>
              <div className='rounded-md border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Remaining Balance')}
                </div>
                <div className='text-lg font-semibold'>
                  {snapshot.balance?.balance_usd ?? '-'}
                </div>
              </div>
              <div className='rounded-md border p-3'>
                <div className='text-muted-foreground text-xs'>
                  {t('Used Balance')}
                </div>
                <div className='text-lg font-semibold'>
                  {snapshot.balance?.used_usd ?? '-'}
                </div>
              </div>
            </div>
          )}

          {options.showBalance === false && (
            <div className='rounded-md border p-3'>
              <div className='text-muted-foreground text-xs'>
                {t('Synced Keys')}
              </div>
              <div className='text-lg font-semibold'>
                {snapshot.keys.length}
              </div>
            </div>
          )}

          {showSuggestedToggle && (
            <div className='flex items-center justify-between gap-3'>
              <div className='flex flex-col gap-1'>
                <span className='text-sm font-medium'>
                  {t('Apply suggested priority and weight')}
                </span>
                <span className='text-muted-foreground text-xs'>
                  {t(
                    'Lower ratio conversion gets higher priority and weight by default.'
                  )}
                </span>
              </div>
              <Switch
                checked={upstreamApplySuggested}
                disabled={snapshot.keys.length === 0}
                onCheckedChange={setUpstreamApplySuggested}
              />
            </div>
          )}

          {snapshot.keys.length === 0 ? (
            <Alert>
              <AlertCircle aria-hidden='true' />
              <AlertDescription>
                {options.emptyText ||
                  t('No upstream keys were found for this account.')}
              </AlertDescription>
            </Alert>
          ) : (
            <>
              {!upstreamModelsToString(snapshot.keys) && (
                <Alert>
                  <AlertCircle aria-hidden='true' />
                  <AlertDescription>
                    {t(
                      'No models were returned by the upstream account. Add models manually after creation so this channel can receive routed requests.'
                    )}
                  </AlertDescription>
                </Alert>
              )}
              <div className='overflow-x-auto rounded-md border'>
                <div className='grid min-w-[74rem] grid-cols-[minmax(8rem,0.95fr)_minmax(16rem,1.35fr)_minmax(8rem,0.75fr)_5.5rem_6.75rem_4.5rem_4.5rem_4rem] gap-2 border-b px-2 py-2 text-[11px] font-medium'>
                  <span className='min-w-0 truncate' title={t('Key')}>
                    {t('Key')}
                  </span>
                  <span className='min-w-0 truncate' title={t('Models')}>
                    {t('Models')}
                  </span>
                  <span className='min-w-0 truncate' title={t('Key Group')}>
                    {t('Key Group')}
                  </span>
                  <span className='min-w-0 truncate' title={t('Key Ratio')}>
                    {t('Key Ratio')}
                  </span>
                  <span
                    className='min-w-0 truncate'
                    title={t('Ratio Conversion')}
                  >
                    {t('Ratio Conversion')}
                  </span>
                  <span className='min-w-0 truncate' title={t('Priority')}>
                    {t('Priority')}
                  </span>
                  <span className='min-w-0 truncate' title={t('Weight')}>
                    {t('Weight')}
                  </span>
                  <span className='min-w-0 truncate' title={t('Enabled')}>
                    {t('Enabled')}
                  </span>
                </div>
                {snapshot.keys.map((key, index) => {
                  const configId = upstreamKeyConfigId(key, index)
                  const config = getUpstreamAccountConfig(
                    upstreamAccountConfigs,
                    key,
                    index
                  )
                  const currentModelsArrayValue =
                    upstreamAccountModelsArrayValue(key, config)
                  const upstreamKeyModelOptions =
                    buildUpstreamAccountModelOptions(key, config, [
                      ...allModelsList,
                      ...currentModelsArray,
                    ])
                  const updateConfig = (
                    updater: (
                      previous: UpstreamAccountConfigDraft | undefined
                    ) => UpstreamAccountConfigDraft
                  ) =>
                    setUpstreamAccountConfigs((prev) => ({
                      ...prev,
                      [configId]: updater(prev[configId]),
                    }))
                  const buildConfigWithDefaults = (
                    previous: UpstreamAccountConfigDraft | undefined,
                    overrides: Partial<UpstreamAccountConfigDraft>
                  ): UpstreamAccountConfigDraft => ({
                    enabled: previous?.enabled ?? true,
                    priority: previous?.priority ?? key.suggested_priority ?? 0,
                    weight: previous?.weight ?? key.suggested_weight ?? 0,
                    models: previous?.models ?? key.models?.join(',') ?? '',
                    group:
                      previous?.group ?? key.group_name ?? key.group_id ?? '',
                    ...overrides,
                  })
                  const setConfigValue = (
                    overrides: Partial<UpstreamAccountConfigDraft>
                  ) =>
                    updateConfig((previous) =>
                      buildConfigWithDefaults(previous, overrides)
                    )
                  const handleKeyModelsChange = (values: string[]) =>
                    setConfigValue({
                      models: formatModelsArray(dedupeModelNames(values)),
                    })
                  const currentGroupValue = upstreamAccountConfigTextValue(
                    config?.group,
                    key.group_name || key.group_id || ''
                  )
                  const currentPriorityValue =
                    config?.priority ?? key.suggested_priority ?? 0
                  const currentWeightValue =
                    config?.weight ?? key.suggested_weight ?? 0
                  const currentKeyGroupLabel = getUpstreamKeyGroupLabel(key)
                  const keyRatioValue = getUpstreamKeyRatioDisplayValue(key)
                  const displayedRatioValue = getUpstreamRatioDisplayValue(key)
                  const modelRatioDetails = formatUpstreamModelRatioDetails(
                    key.model_ratios
                  )
                  const keyRatioTitle = modelRatioDetails
                    ? `${t('Model Ratios')}:\n${modelRatioDetails}`
                    : undefined
                  const ratioTitle = [
                    key.ratio_conversion != null
                      ? `${t('Ratio Conversion')}: ${formatUpstreamRatioCompact(key.ratio_conversion)}x`
                      : '',
                    key.effective_ratio != null
                      ? `${t('Upstream Ratio')}: ${formatUpstreamRatioCompact(key.effective_ratio)}x`
                      : '',
                    modelRatioDetails,
                  ]
                    .filter(Boolean)
                    .join('\n')
                  return (
                    <div
                      key={configId}
                      className='grid min-w-[74rem] grid-cols-[minmax(8rem,0.95fr)_minmax(16rem,1.35fr)_minmax(8rem,0.75fr)_5.5rem_6.75rem_4.5rem_4.5rem_4rem] items-center gap-2 border-b px-2 py-2 last:border-b-0'
                    >
                      <div className='min-w-0'>
                        <div className='truncate text-sm font-medium'>
                          {key.name || key.masked_key}
                        </div>
                        <div className='text-muted-foreground truncate text-xs'>
                          {key.masked_key}
                        </div>
                      </div>
                      <MultiSelect
                        options={upstreamKeyModelOptions}
                        selected={currentModelsArrayValue}
                        onChange={handleKeyModelsChange}
                        placeholder={t('Select models or add custom ones')}
                        allowCreate
                        allowCreateWithMatches={false}
                        createLabel='Add custom model "{{value}}"'
                        maxVisibleChips={2}
                        copyChipOnClick
                        emptyText={t('No matching models')}
                        className='min-h-8'
                      />
                      <div className='flex min-w-0 flex-col gap-1'>
                        <Input
                          value={currentGroupValue}
                          placeholder={t(
                            'Key group inherited from upstream if empty'
                          )}
                          onChange={(event) =>
                            setConfigValue({ group: event.target.value })
                          }
                          className='h-8 px-2 text-xs'
                        />
                        <span
                          className='text-muted-foreground truncate text-[11px]'
                          title={currentKeyGroupLabel || undefined}
                        >
                          {currentKeyGroupLabel || t('Inherited')}
                        </span>
                      </div>
                      <span className='font-mono text-xs' title={keyRatioTitle}>
                        {keyRatioValue != null
                          ? `${formatUpstreamRatioCompact(keyRatioValue)}x`
                          : '-'}
                      </span>
                      <div
                        className='flex min-w-0 flex-col gap-1'
                        title={ratioTitle || undefined}
                      >
                        <span className='font-mono text-xs'>
                          {displayedRatioValue != null
                            ? `${formatUpstreamRatioCompact(displayedRatioValue)}x`
                            : '-'}
                        </span>
                        {key.ratio_conversion != null &&
                          keyRatioValue != null &&
                          Math.abs(key.ratio_conversion - keyRatioValue) >
                            Number.EPSILON && (
                            <span className='text-muted-foreground truncate text-[11px]'>
                              {t('Converted')}
                            </span>
                          )}
                      </div>
                      <Input
                        type='number'
                        value={currentPriorityValue}
                        disabled={showSuggestedToggle && upstreamApplySuggested}
                        onChange={(event) =>
                          setConfigValue({
                            priority: Number(event.target.value),
                          })
                        }
                        className='h-8 px-2 text-xs'
                      />
                      <Input
                        type='number'
                        value={currentWeightValue}
                        disabled={showSuggestedToggle && upstreamApplySuggested}
                        onChange={(event) =>
                          setConfigValue({
                            weight: Number(event.target.value),
                          })
                        }
                        className='h-8 px-2 text-xs'
                      />
                      <Switch
                        checked={config?.enabled ?? true}
                        onCheckedChange={(checked) =>
                          setConfigValue({ enabled: checked })
                        }
                      />
                    </div>
                  )
                })}
              </div>
            </>
          )}
        </div>
      )
    },
    [
      allModelsList,
      currentModelsArray,
      t,
      upstreamAccountConfigs,
      upstreamApplySuggested,
    ]
  )

  const currentTypeLabel = useMemo(
    () =>
      CHANNEL_TYPE_OPTIONS.find((option) => option.value === currentType)
        ?.label || `#${currentType}`,
    [currentType]
  )

  const credentialModeDescription = useMemo(() => {
    switch (credentialMode) {
      case 'global_account_pool':
        return t(
          'Select an account pool group; upstream tokens are provided by accounts in that group.'
        )
      case 'account_pool':
        return t(
          'Legacy channel account pool mode is kept for existing channels.'
        )
      case 'multi_key':
        return t('Rotate keys stored on this channel using the multi-key list.')
      default:
        return t('Use the channel key directly.')
    }
  }, [credentialMode, t])

  const formErrors = form.formState.errors
  const identityHasErrors = Boolean(
    formErrors.name ||
    formErrors.type ||
    formErrors.status ||
    formErrors.openai_organization
  )
  const credentialsHaveErrors = Boolean(
    formErrors.key ||
    formErrors.base_url ||
    formErrors.other ||
    formErrors.multi_key_mode ||
    formErrors.multi_key_type ||
    formErrors.key_mode ||
    formErrors.vertex_key_type ||
    formErrors.aws_key_type ||
    formErrors.azure_responses_version ||
    formErrors.account_pool_group_id
  )
  const modelsHaveErrors = Boolean(
    formErrors.models || formErrors.group || formErrors.model_mapping
  )
  const advancedHaveErrors = hasAdvancedSettingsErrors(formErrors)
  const providerRequiresBaseUrl =
    !isGlobalAccountPoolMode &&
    !usesUpstreamAccountCredentialSource &&
    [3, 8, 36, 45].includes(currentType)
  const providerRequiresOther =
    !usesUpstreamAccountCredentialSource &&
    [3, 18, 21, 39, 41, 49].includes(currentType)
  const identityComplete = Boolean(currentName?.trim() && currentType > 0)
  const credentialsComplete = (() => {
    if (isCreateUpstreamSyncMode) {
      return Boolean(upstreamPreviewId && upstreamSnapshot?.keys.length)
    }
    if (isUpstreamAccountSyncedChannel) {
      return true
    }
    if (isEditingUnsupportedUnsyncedUpstreamType) {
      return false
    }
    if (isGlobalAccountPoolMode) {
      return Boolean(accountPoolGroupId)
    }
    return Boolean(
      (isEditing || currentKey?.trim()) &&
      (!providerRequiresBaseUrl || currentBaseUrl?.trim()) &&
      (!providerRequiresOther || currentOther?.trim())
    )
  })()
  const modelsComplete = usesUpstreamAccountCredentialSource
    ? Boolean(
        isUpstreamAccountSyncedChannel ||
        upstreamSnapshot?.keys.length ||
        upstreamRefreshSnapshot?.keys.length
      )
    : Boolean(currentModelsArray.length > 0 && currentGroups?.length)
  const requiredCompletedCount = [
    identityComplete,
    credentialsComplete,
    modelsComplete,
  ].filter(Boolean).length
  const currentStatusLabel =
    CHANNEL_STATUS_LABELS[
      currentStatus as keyof typeof CHANNEL_STATUS_LABELS
    ] || 'Unknown'
  const progressLabel = `${requiredCompletedCount}/3`
  const identityStatus = getCompletionStatus(
    identityHasErrors,
    identityComplete
  )
  const credentialsStatus = getCompletionStatus(
    credentialsHaveErrors,
    credentialsComplete
  )
  const modelsStatus = getCompletionStatus(modelsHaveErrors, modelsComplete)
  const advancedConfigured = hasAdvancedSettingsValues(currentFormValues)
  const advancedStatus: ChannelEditorSectionStatus = advancedHaveErrors
    ? 'error'
    : advancedConfigured
      ? 'configured'
      : 'idle'
  const advancedSummary = advancedHaveErrors
    ? t('Error')
    : advancedConfigured
      ? t('Ready')
      : undefined
  const routingStrategyConfigured = Boolean(
    currentFormValues.priority ||
    currentFormValues.weight ||
    currentFormValues.test_model?.trim() ||
    (currentFormValues.auto_ban ?? 1) !== 1
  )
  const internalNotesConfigured = Boolean(
    currentFormValues.tag?.trim() || currentFormValues.remark?.trim()
  )
  const overrideRulesConfigured = Boolean(
    currentFormValues.status_code_mapping?.trim() ||
    currentFormValues.param_override?.trim() ||
    currentFormValues.header_override?.trim()
  )
  const extraSettingsConfigured = Boolean(
    currentFormValues.force_format ||
    currentFormValues.thinking_to_content ||
    currentFormValues.pass_through_body_enabled ||
    currentFormValues.disable_task_polling_sleep ||
    currentFormValues.proxy?.trim() ||
    currentFormValues.system_prompt?.trim() ||
    currentFormValues.system_prompt_override ||
    (currentType === CHANNEL_TYPE_ADVANCED_CUSTOM &&
      currentFormValues.advanced_custom?.trim())
  )
  let fieldPassthroughConfigured = false
  if (currentType === 1 || currentType === 57) {
    fieldPassthroughConfigured = Boolean(
      currentFormValues.allow_service_tier ||
      currentFormValues.disable_store ||
      currentFormValues.allow_safety_identifier ||
      currentFormValues.allow_include_obfuscation ||
      currentFormValues.allow_inference_geo
    )
  } else if (currentType === 14) {
    fieldPassthroughConfigured = Boolean(
      currentFormValues.allow_service_tier ||
      currentFormValues.allow_inference_geo ||
      currentFormValues.allow_speed ||
      currentFormValues.claude_beta_query
    )
  }
  const upstreamModelDetectionConfigured = Boolean(
    currentFormValues.upstream_model_update_check_enabled ||
    currentFormValues.upstream_model_update_auto_sync_enabled ||
    currentFormValues.upstream_model_update_ignored_models?.trim()
  )
  const advancedNavChildren: ChannelEditorNavChildItem[] = [
    {
      id: ADVANCED_SETTINGS_SECTION_IDS.routingStrategy,
      title: t('Routing Strategy'),
      configured: routingStrategyConfigured,
    },
    {
      id: ADVANCED_SETTINGS_SECTION_IDS.internalNotes,
      title: t('Internal Notes'),
      configured: internalNotesConfigured,
    },
    {
      id: ADVANCED_SETTINGS_SECTION_IDS.overrideRules,
      title: t('Override Rules'),
      configured: overrideRulesConfigured,
    },
    {
      id: ADVANCED_SETTINGS_SECTION_IDS.extraSettings,
      title: t('Channel Extra Settings'),
      configured: extraSettingsConfigured,
    },
  ]
  if (currentType === 1 || currentType === 14 || currentType === 57) {
    advancedNavChildren.push({
      id: ADVANCED_SETTINGS_SECTION_IDS.fieldPassthrough,
      title: t('Field passthrough controls'),
      configured: fieldPassthroughConfigured,
    })
  }
  if (MODEL_FETCHABLE_TYPES.has(currentType)) {
    advancedNavChildren.push({
      id: ADVANCED_SETTINGS_SECTION_IDS.upstreamModelDetection,
      title: t('Upstream Model Detection Settings'),
      configured: upstreamModelDetectionConfigured,
    })
  }
  const editorNavItems: ChannelEditorNavItem[] = [
    {
      id: CHANNEL_EDITOR_SECTION_IDS.identity,
      title: t('Basic Information'),
      description: getSectionStatusLabel(identityStatus, t),
      statusLabel: getSectionStatusLabel(identityStatus, t),
      status: identityStatus,
      icon: <Server className='h-4 w-4' aria-hidden='true' />,
    },
    {
      id: CHANNEL_EDITOR_SECTION_IDS.credentials,
      title: t('Credentials'),
      description: getSectionStatusLabel(credentialsStatus, t),
      statusLabel: getSectionStatusLabel(credentialsStatus, t),
      status: credentialsStatus,
      icon: <KeyRound className='h-4 w-4' aria-hidden='true' />,
    },
    ...(showSharedModelsSection
      ? [
          {
            id: CHANNEL_EDITOR_SECTION_IDS.models,
            title: t('Models & Groups'),
            description: getSectionStatusLabel(modelsStatus, t),
            statusLabel: getSectionStatusLabel(modelsStatus, t),
            status: modelsStatus,
            icon: <Boxes className='h-4 w-4' aria-hidden='true' />,
          } satisfies ChannelEditorNavItem,
        ]
      : []),
    {
      id: CHANNEL_EDITOR_SECTION_IDS.advanced,
      title: t('Advanced Settings'),
      description: advancedSummary,
      statusLabel: advancedSummary ?? t('Advanced Settings'),
      status: advancedStatus,
      icon: <Settings className='h-4 w-4' aria-hidden='true' />,
      configured: advancedConfigured,
      children: advancedNavChildren,
    },
  ]

  const channelTypeOptions = useMemo(() => {
    const options = CHANNEL_TYPE_OPTIONS.map((option) => ({
      value: String(option.value),
      label: t(option.label),
      icon: <ChannelTypeIcon type={option.value} size={16} />,
    }))
    if (!options.some((option) => Number(option.value) === currentType)) {
      options.push({
        value: String(currentType),
        label: `#${currentType}`,
        icon: <ChannelTypeIcon type={currentType} size={16} />,
      })
    }
    return options
  }, [currentType, t])

  // 从 model_mapping 中提取重定向目标模型，用于提示用户避免暴露上游真实模型名。
  const redirectModelList = useMemo(
    () => extractRedirectModels(currentModelMapping || ''),
    [currentModelMapping]
  )

  // 从 model_mapping 中提取源模型名，用于检查这些源模型是否已经包含在渠道模型列表里。
  const redirectModelKeyList = useMemo(
    () => extractMappingSourceModels(currentModelMapping || ''),
    [currentModelMapping]
  )

  const modelSearchNameResult = useMemo(
    () =>
      getModelSearchModelNameResult(
        modelSearchData?.data?.items ?? [],
        trimmedDebouncedModelSearchValue
      ),
    [modelSearchData, trimmedDebouncedModelSearchValue]
  )

  const modelSearchSummary = useMemo(
    () =>
      summarizeModelSearchCandidates(
        modelSearchNameResult.names,
        currentModelsArray
      ),
    [currentModelsArray, modelSearchNameResult.names]
  )

  // 将系统模型和当前渠道模型合并成基础候选，避免编辑历史模型时选项丢失。
  // 该列表保留编辑草稿里已经存在的模型，避免历史能力在切换渠道后丢失。
  const baseModelOptions = useMemo(() => {
    return dedupeModelNames([...allModelsList, ...currentModelsArray]).map(
      (model) => ({
        value: model,
        label: model,
      })
    )
  }, [allModelsList, currentModelsArray])

  // 本地能力列表和远程模型元信息搜索结果合并成选择器候选。
  // 远程结果只来自真实 `model_name`/规则展开结果，不把搜索关键词本身写入渠道。
  const modelOptions = useMemo(
    () =>
      dedupeModelNames([
        ...modelSearchSummary.addable,
        ...modelSearchSummary.matched,
        ...baseModelOptions.map((option) => option.value),
      ]).map((model) => ({
        value: model,
        label: model,
      })),
    [baseModelOptions, modelSearchSummary.addable, modelSearchSummary.matched]
  )

  const modelSearchPreviewNames = modelSearchSummary.addable.slice(
    0,
    MODEL_SEARCH_RESULT_PREVIEW_LIMIT
  )
  const modelSearchPreviewOmittedCount =
    modelSearchSummary.addable.length - modelSearchPreviewNames.length
  const modelSearchIsWaitingForDebounce =
    trimmedModelSearchValue.length >= 2 &&
    trimmedModelSearchValue !== trimmedDebouncedModelSearchValue
  const modelSearchIsLoading =
    modelSearchIsWaitingForDebounce || isModelSearchFetching
  const showModelSearchPanel =
    trimmedModelSearchValue.length >= 2 &&
    (modelSearchIsLoading ||
      isModelSearchError ||
      modelSearchSummary.matched.length > 0)

  const modelMappingGuardrail = useMemo<ModelMappingGuardrail>(() => {
    if (!currentModelMapping?.trim()) {
      return createEmptyModelMappingGuardrail()
    }

    try {
      const parsed = JSON.parse(currentModelMapping)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        return { ...createEmptyModelMappingGuardrail(), invalidJson: true }
      }

      const entries = Object.entries(parsed).reduce<
        Array<{ source: string; target: string }>
      >((acc, [rawSource, rawTarget]) => {
        const source = String(rawSource).trim()
        const target = String(rawTarget ?? '').trim()

        if (!source || !target) {
          return acc
        }

        acc.push({ source, target })
        return acc
      }, [])

      const missingSourceModels = Array.from(
        new Set(
          entries
            .filter(
              (entry) =>
                Boolean(entry.source) &&
                !currentModelsArray.includes(entry.source)
            )
            .map((entry) => entry.source)
        )
      )

      const exposedTargetModels = Array.from(
        new Set(
          entries
            .filter(
              (entry) =>
                Boolean(entry.target) &&
                currentModelsArray.includes(entry.target)
            )
            .map((entry) => entry.target)
        )
      )

      return {
        invalidJson: false,
        entries,
        missingSourceModels,
        exposedTargetModels,
      }
    } catch {
      return { ...createEmptyModelMappingGuardrail(), invalidJson: true }
    }
  }, [currentModelMapping, currentModelsArray])

  const mappingPreviewPairs =
    modelMappingGuardrail.entries.length > 0
      ? modelMappingGuardrail.entries.slice(0, 3)
      : MODEL_MAPPING_PREVIEW_FALLBACK
  const remainingMappingCount =
    modelMappingGuardrail.entries.length > 3
      ? modelMappingGuardrail.entries.length - 3
      : 0

  const upstreamUpdateMeta = useMemo(() => {
    const settings = parseSettingsRecord(currentSettings)
    const detectedModels = Array.isArray(
      settings.upstream_model_update_last_detected_models
    )
      ? settings.upstream_model_update_last_detected_models
          .map((model) => String(model || '').trim())
          .filter(Boolean)
      : []

    return {
      lastCheckTime: settings.upstream_model_update_last_check_time,
      detectedModels: Array.from(new Set(detectedModels)),
    }
  }, [currentSettings])

  const savedUpstreamCredentialAvailable = useMemo(
    () =>
      hasUpstreamSyncSavedCredential(
        channelData?.data?.settings ?? renderCurrentRow?.settings
      ),
    [channelData?.data?.settings, renderCurrentRow?.settings]
  )

  const upstreamDetectedModelsPreview = upstreamUpdateMeta.detectedModels.slice(
    0,
    UPSTREAM_DETECTED_MODEL_PREVIEW_LIMIT
  )
  const upstreamDetectedModelsOmittedCount =
    upstreamUpdateMeta.detectedModels.length -
    upstreamDetectedModelsPreview.length

  // 编辑模式加载渠道数据并写入表单，同时记录初始模型配置用于后续风险提示。
  useEffect(() => {
    if (isEditing && channelData?.data) {
      const isSyncedChannel =
        isUpstreamAccountSyncChannel(channelData.data) ||
        (channelData.data.channel_info?.credential_mode === 'account_pool' &&
          channelData.data.channel_info?.account_pool_enabled === true)
      const defaults = {
        ...transformChannelToFormDefaults(channelData.data),
        upstream_account_sync: isSyncedChannel,
      }
      form.reset(defaults)
      clearAllUpstreamPreviews()
      setUpstreamPlatform(
        getUpstreamSyncPlatformFromSettings(channelData.data.settings) ||
          upstreamPlatformFromChannelType(channelData.data.type) ||
          'new-api'
      )
      setUpstreamBaseUrl(
        getUpstreamSyncBaseUrlFromSettings(channelData.data.settings) ||
          channelData.data.base_url ||
          ''
      )
      setUpstreamUseSavedCredential(savedUpstreamCredentialAvailable)
      setUpstreamUsername('')
      setUpstreamPassword('')
      setUpstreamRatioConversionState(
        syncedChannelAccounts[0]?.ratio_conversion_config,
        setUpstreamPaidCny,
        setUpstreamPlatformUsdCredit
      )
      setSyncRefreshOpen(false)
      setAdvancedSettingsOpen(
        readAdvancedSettingsPreference() || hasAdvancedSettingsValues(defaults)
      )
      // 记录初始值，提交前用来判断是否需要弹出模型映射风险确认。
      initialModelsRef.current = parseModelsString(
        channelData.data.models || ''
      )
      initialModelMappingRef.current = channelData.data.model_mapping || ''
      initialStatusCodeMappingRef.current =
        channelData.data.status_code_mapping || ''
    } else if (!isEditing) {
      form.reset(CHANNEL_FORM_DEFAULT_VALUES)
      clearAllUpstreamPreviews()
      setUpstreamPlatform('new-api')
      setUpstreamBaseUrl('')
      setUpstreamUseSavedCredential(false)
      setUpstreamUsername('')
      setUpstreamPassword('')
      setUpstreamPaidCny('')
      setUpstreamPlatformUsdCredit('')
      setSyncRefreshOpen(false)
      setAdvancedSettingsOpen(false)
      initialModelsRef.current = []
      initialModelMappingRef.current = ''
      initialStatusCodeMappingRef.current = ''
    }
  }, [
    clearAllUpstreamPreviews,
    isEditing,
    channelData,
    form,
    syncedChannelAccounts,
    savedUpstreamCredentialAvailable,
  ])

  useEffect(() => {
    if (
      !isUpstreamAccountSyncedChannel ||
      upstreamRefreshSnapshot ||
      syncedChannelAccounts.length === 0 ||
      Object.keys(upstreamAccountConfigs).length > 0
    ) {
      return
    }
    setUpstreamAccountConfigs(
      buildUpstreamAccountConfigsFromChannelAccounts(syncedChannelAccounts)
    )
  }, [
    isUpstreamAccountSyncedChannel,
    syncedChannelAccounts,
    upstreamRefreshSnapshot,
    upstreamAccountConfigs,
  ])

  useEffect(() => {
    if (
      !isUpstreamAccountSyncedChannel ||
      upstreamRefreshSnapshot ||
      upstreamSnapshot ||
      upstreamPaidCny.trim() ||
      upstreamPlatformUsdCredit.trim()
    ) {
      return
    }
    const ratioConfig = syncedChannelAccounts.find(
      (account) => account.ratio_conversion_config
    )?.ratio_conversion_config
    if (!ratioConfig) return
    setUpstreamRatioConversionState(
      ratioConfig,
      setUpstreamPaidCny,
      setUpstreamPlatformUsdCredit
    )
  }, [
    isUpstreamAccountSyncedChannel,
    syncedChannelAccounts,
    upstreamPaidCny,
    upstreamPlatformUsdCredit,
    upstreamRefreshSnapshot,
    upstreamSnapshot,
  ])

  // 渠道类型变化时补充类型默认值；编辑模式不自动覆盖已有渠道配置。
  useEffect(() => {
    if (isEditing) return

    // 火山引擎默认使用北京区域；账号池组模式下不填写渠道自身 base_url。
    if (currentType === 45 && !isGlobalAccountPoolMode) {
      const currentBaseUrlValue = form.getValues('base_url')
      if (!currentBaseUrlValue || currentBaseUrlValue === '') {
        form.setValue('base_url', 'https://ark.cn-beijing.volces.com', {
          shouldDirty: false,
          shouldValidate: true,
        })
      }
    }

    // 讯飞星火渠道需要默认版本号，账号池模式也保留该协议字段。
    if (currentType === 18) {
      const currentOther = form.getValues('other')
      if (!currentOther || currentOther === '') {
        form.setValue('other', 'v2.1', {
          shouldDirty: false,
          shouldValidate: true,
        })
      }
    }
  }, [currentType, isEditing, form, isGlobalAccountPoolMode])

  useEffect(() => {
    if (currentType !== 45 || currentBaseUrl !== 'doubao-coding-plan') return

    // Doubao Coding Plan 是火山引擎早期内置别名，new-api 已从可选 endpoint 中移除。
    // 这里仅迁移历史值到官方北京 endpoint，避免管理员编辑其它字段时继续保存弃用别名。
    form.setValue('base_url', 'https://ark.cn-beijing.volces.com', {
      shouldDirty: false,
      shouldValidate: true,
    })
  }, [currentBaseUrl, currentType, form])

  // base_url 末尾带 /v1 时很容易与后端自动拼接逻辑冲突，因此延迟提示管理员确认。
  useEffect(() => {
    if (
      isGlobalAccountPoolMode ||
      !currentBaseUrl ||
      !currentBaseUrl.endsWith('/v1')
    ) {
      return
    }

    // 延迟触发可以避开输入过程中的瞬时状态，减少提示打断。
    const timer = setTimeout(() => {
      toast.warning(
        t(
          'Warning: Base URL should not end with /v1. NexusTok will handle it automatically. This may cause request failures.'
        ),
        { duration: 5000 }
      )
    }, 500)

    return () => clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentBaseUrl, isGlobalAccountPoolMode])

  // 多 Key 输入去重。
  const handleDeduplicateKeys = () => {
    if (!canEditSensitiveFields) {
      toast.error(noPermissionMessage)
      return
    }

    const currentKey = form.getValues('key')
    if (!currentKey || currentKey.trim() === '') {
      toast.info(t('Please enter keys first'))
      return
    }

    const result = deduplicateKeys(currentKey)

    if (result.removedCount === 0) {
      toast.info(t('No duplicate keys found'))
    } else {
      form.setValue('key', result.deduplicatedText)
      toast.success(
        t(
          'Removed {{removed}} duplicate key(s). Before: {{before}}, After: {{after}}',
          {
            removed: result.removedCount,
            before: result.beforeCount,
            after: result.afterCount,
          }
        )
      )
    }
  }

  const fetchChannelKey = useCallback(async () => {
    if (!channelId) {
      throw new Error('Channel is not selected')
    }
    if (!permissions.canViewSecret) {
      throw new Error(noPermissionMessage)
    }

    setIsChannelKeyLoading(true)
    try {
      const res = await getChannelKey(channelId)
      if (!res.success) {
        throw new Error(res.message || 'Failed to fetch channel key')
      }

      const keyValue = res.data?.key ?? ''
      setChannelKey(keyValue)
      toast.success(t('Channel key unlocked'))
      return res
    } finally {
      setIsChannelKeyLoading(false)
    }
  }, [channelId, noPermissionMessage, permissions.canViewSecret, t])

  const handleRevealKey = useCallback(async () => {
    if (!channelId) return
    if (!permissions.canViewSecret) {
      toast.error(noPermissionMessage)
      return
    }

    try {
      await withVerification(fetchChannelKey, {
        preferredMethod: 'passkey',
        title: 'Verify to view channel key',
        description:
          'Use Passkey or 2FA to confirm your identity before revealing this channel key.',
      })
    } catch (error) {
      if (error instanceof Error) {
        toast.error(error.message)
      }
    }
  }, [
    channelId,
    fetchChannelKey,
    noPermissionMessage,
    permissions.canViewSecret,
    withVerification,
  ])

  const handleRefreshCodexCredential = useCallback(async () => {
    if (!channelId) return
    if (!canEditSensitiveFields) {
      toast.error(noPermissionMessage)
      return
    }
    setIsCodexCredentialRefreshing(true)
    try {
      const res = await refreshCodexCredential(channelId)
      if (!res.success) {
        throw new Error(res.message || 'Failed to refresh credential')
      }
      toast.success(t('Credential refreshed'))
      queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.detail(channelId),
      })
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Refresh failed'))
    } finally {
      setIsCodexCredentialRefreshing(false)
    }
  }, [canEditSensitiveFields, channelId, noPermissionMessage, queryClient, t])

  // 统一更新模型字段，所有快捷填充和预设导入都走这里保持格式一致。
  const updateModels = useCallback(
    (newModels: string[], merge: boolean = false) => {
      const normalizedNewModels = dedupeModelNames(newModels)
      const existingModels = merge
        ? dedupeModelNames(parseModelsString(form.getValues('models') || ''))
        : []
      const existingModelSet = new Set(
        existingModels.map((model) => model.trim().toLowerCase())
      )
      const finalModelsArray = merge
        ? mergeModelNames(existingModels, normalizedNewModels)
        : normalizedNewModels
      const finalModels = formatModelsArray(finalModelsArray)
      const nextModels = parseModelsString(finalModels)
      form.setValue('models', finalModels, {
        shouldDirty: true,
        shouldValidate: true,
      })
      if (!merge) return nextModels.length
      return nextModels.filter(
        (model) => !existingModelSet.has(model.trim().toLowerCase())
      ).length
    },
    [form]
  )

  // 从上游拉取模型列表。账号池组模式不使用渠道 key，因此不允许走该路径。
  // 新建和编辑渠道都统一进入选择弹窗，避免搜索或拉取动作直接改写模型列表。
  const handleFetchModels = useCallback(async () => {
    if (!permissions.canOperate || !canEditBasicFields) {
      toast.error(noPermissionMessage)
      return
    }

    if (isGlobalAccountPoolMode) {
      toast.info(t('Account pool mode does not fetch models from channel key.'))
      return
    }

    const type = form.getValues('type')

    if (!MODEL_FETCHABLE_TYPES.has(type)) {
      toast.error(t('This channel type does not support fetching models'))
      return
    }

    if (!isEditing) {
      const key = form.getValues('key')
      if (!key?.trim()) {
        toast.error(t('Please enter API key first'))
        return
      }
    }

    setFetchModelsDialogOpen(true)
  }, [
    canEditBasicFields,
    form,
    isEditing,
    isGlobalAccountPoolMode,
    noPermissionMessage,
    permissions.canOperate,
    t,
  ])

  // 新建渠道还没有 channel id，弹窗需要使用当前表单里的连接信息实时拉取上游模型。
  const createModeFetcher = useCallback(async (): Promise<string[]> => {
    if (!canEditSensitiveFields) {
      throw new Error(noPermissionMessage)
    }
    const response = await fetchModels({
      type: form.getValues('type'),
      key: form.getValues('key'),
      base_url: form.getValues('base_url') || '',
    })
    if (response.success && response.data) {
      return response.data
    }
    throw new Error(response.message || t('No models fetched from upstream'))
  }, [canEditSensitiveFields, form, noPermissionMessage, t])

  // 模型快捷操作。
  const handleFillRelatedModels = useCallback(() => {
    if (!canEditBasicFields) {
      toast.error(noPermissionMessage)
      return
    }
    if (!basicModels.length) {
      toast.info(t('No related models available for this channel type'))
      return
    }
    updateModels(basicModels)
    toast.success(
      t('Filled {{count}} related model(s)', { count: basicModels.length })
    )
  }, [basicModels, canEditBasicFields, noPermissionMessage, updateModels, t])

  const handleFillAllModels = useCallback(() => {
    if (!canEditBasicFields) {
      toast.error(noPermissionMessage)
      return
    }
    if (!allModelsList.length) {
      toast.info(t('No models available'))
      return
    }
    updateModels(allModelsList)
    toast.success(
      t('Filled {{count}} model(s)', { count: allModelsList.length })
    )
  }, [allModelsList, canEditBasicFields, noPermissionMessage, updateModels, t])

  const handleClearModels = useCallback(() => {
    if (!canEditBasicFields) {
      toast.error(noPermissionMessage)
      return
    }
    form.setValue('models', '', {
      shouldDirty: true,
      shouldValidate: true,
    })
    toast.success(t('Cleared all models'))
  }, [canEditBasicFields, form, noPermissionMessage, t])

  const handleCopyModels = useCallback(async () => {
    const models = form.getValues('models')
    if (!models?.trim()) {
      toast.info(t('No models to copy'))
      return
    }
    await copyToClipboard(models)
  }, [form, copyToClipboard, t])

  const applyUpstreamPreviewData = useCallback(
    (data: UpstreamAccountPreviewData, mode: UpstreamTwoFactorMode) => {
      if (mode === 'create') {
        setUpstreamPreviewId(data.preview_id)
        setUpstreamPreviewExpiresAt(data.expires_at)
        setUpstreamSnapshot(data.snapshot)
        setUpstreamTwoFactorChallenge(null)
        setUpstreamTwoFactorCode('')
        clearUpstreamRefreshPreview()
      } else {
        setUpstreamRefreshPreviewId(data.preview_id)
        setUpstreamRefreshPreviewExpiresAt(data.expires_at)
        setUpstreamRefreshSnapshot(data.snapshot)
        setUpstreamRefreshTwoFactorChallenge(null)
        setUpstreamRefreshTwoFactorCode('')
        clearUpstreamCreatePreview()
      }

      setUpstreamAccountConfigs((prev) =>
        buildUpstreamAccountConfigsFromSnapshotKeys(data.snapshot.keys, prev)
      )
      setUpstreamRatioConversionState(
        data.snapshot.ratio_conversion,
        setUpstreamPaidCny,
        setUpstreamPlatformUsdCredit
      )

      if (mode === 'create') {
        const models = upstreamModelsToString(data.snapshot.keys)
        const syncedChannelType = channelTypeFromUpstreamPlatform(
          data.snapshot.platform
        )
        if (syncedChannelType) {
          form.setValue('type', syncedChannelType, {
            shouldDirty: true,
            shouldValidate: true,
          })
          setUpstreamPlatform(
            upstreamPlatformFromChannelType(syncedChannelType) || 'new-api'
          )
        }
        if (models) {
          form.setValue('models', models, {
            shouldDirty: true,
            shouldValidate: true,
          })
        }
        if (!(form.getValues('group') || []).length) {
          form.setValue('group', ['default'], {
            shouldDirty: true,
            shouldValidate: true,
          })
        }
        if (!form.getValues('name')?.trim()) {
          form.setValue(
            'name',
            defaultUpstreamChannelName(
              data.snapshot.base_url || upstreamBaseUrl,
              upstreamUsername
            ),
            { shouldDirty: true, shouldValidate: true }
          )
        }
      }

      toast.success(
        t('Synced {{count}} upstream key(s)', {
          count: data.snapshot.keys.length,
        })
      )
    },
    [
      clearUpstreamCreatePreview,
      clearUpstreamRefreshPreview,
      form,
      t,
      upstreamBaseUrl,
      upstreamUsername,
    ]
  )

  const applyUpstreamPreviewChallenge = useCallback(
    (
      challenge: UpstreamAccountTwoFactorChallenge,
      mode: UpstreamTwoFactorMode
    ) => {
      if (mode === 'create') {
        setUpstreamTwoFactorChallenge(challenge)
        setUpstreamTwoFactorCode('')
        setUpstreamPreviewId('')
        setUpstreamPreviewExpiresAt(0)
        setUpstreamSnapshot(null)
        clearUpstreamRefreshPreview()
      } else {
        setUpstreamRefreshTwoFactorChallenge(challenge)
        setUpstreamRefreshTwoFactorCode('')
        setUpstreamRefreshPreviewId('')
        setUpstreamRefreshPreviewExpiresAt(0)
        setUpstreamRefreshSnapshot(null)
        clearUpstreamCreatePreview()
      }
      setUpstreamAccountConfigs({})
      toast.info(t('Enter the 2FA code from the upstream account.'))
    },
    [clearUpstreamCreatePreview, clearUpstreamRefreshPreview, t]
  )

  const handlePreviewUpstreamAccount = useCallback(async () => {
    if (!permissions.canSensitiveWrite) {
      toast.error(noPermissionMessage)
      return
    }
    const baseUrl = upstreamBaseUrl.trim()
    if (!baseUrl) {
      toast.error(t('Upstream platform URL is required'))
      return
    }
    if (!upstreamUsername.trim() || !upstreamPassword.trim()) {
      toast.error(t('Account and password are required'))
      return
    }
    const res = await upstreamPreviewMutation.mutateAsync({
      platform: forcedUpstreamPlatform ?? upstreamPlatform,
      base_url: baseUrl,
      username:
        (forcedUpstreamPlatform ?? upstreamPlatform) === 'new-api'
          ? upstreamUsername
          : undefined,
      email:
        (forcedUpstreamPlatform ?? upstreamPlatform) === 'sub2api'
          ? upstreamUsername
          : undefined,
      password: upstreamPassword,
      ratio_conversion: upstreamRatioConversion,
    })
    if (!res.success || !res.data) {
      toast.error(res.message || t('Failed to sync upstream account'))
      return
    }
    const challenge = getUpstreamPreviewChallenge(res.data)
    if (challenge) {
      applyUpstreamPreviewChallenge(challenge, 'create')
      return
    }
    if (hasUpstreamPreviewSnapshot(res.data)) {
      applyUpstreamPreviewData(res.data, 'create')
      return
    }
    toast.error(t('Failed to sync upstream account'))
  }, [
    applyUpstreamPreviewChallenge,
    applyUpstreamPreviewData,
    noPermissionMessage,
    permissions.canSensitiveWrite,
    t,
    forcedUpstreamPlatform,
    upstreamBaseUrl,
    upstreamPassword,
    upstreamPlatform,
    upstreamPreviewMutation,
    upstreamRatioConversion,
    upstreamUsername,
  ])

  // 添加模型预设分组中的模型。
  const handleAddPrefillGroup = useCallback(
    (group: { id: number; name: string; items: string | string[] }) => {
      if (!canEditBasicFields) {
        toast.error(noPermissionMessage)
        return
      }
      try {
        const items = Array.isArray(group.items)
          ? group.items
          : JSON.parse(group.items)

        if (!Array.isArray(items)) {
          throw new Error('Invalid items format')
        }

        const count = updateModels(items, true)
        toast.success(
          t('Added {{count}} models from "{{name}}"', {
            count,
            name: group.name,
          })
        )
      } catch {
        toast.error(t('Failed to parse group items'))
      }
    },
    [canEditBasicFields, noPermissionMessage, updateModels, t]
  )

  // MultiSelect 组件会回传数组，保存前仍要转成逗号分隔字符串。
  const handleModelsChange = useCallback(
    (selected: string[]) => {
      if (!canEditBasicFields) {
        toast.error(noPermissionMessage)
        return
      }
      form.setValue('models', formatModelsArray(dedupeModelNames(selected)), {
        shouldDirty: true,
        shouldValidate: true,
      })
    },
    [canEditBasicFields, form, noPermissionMessage]
  )

  // 将模型元信息搜索命中的真实模型一次性追加到当前渠道能力草稿。
  // 只追加未选择项，避免把系列前缀或重复模型写入 `models` 字段。
  const handleAddModelSearchResults = useCallback(() => {
    if (!canEditBasicFields) {
      toast.error(noPermissionMessage)
      return
    }

    if (modelSearchSummary.addable.length === 0) {
      toast.info(t('No new search results to add'))
      return
    }

    const addedCount = updateModels(modelSearchSummary.addable, true)
    if (addedCount === 0) {
      toast.info(t('No new search results to add'))
      return
    }

    toast.success(
      t('Added {{count}} model(s) from search', { count: addedCount })
    )
    setModelSearchOpen(false)
  }, [
    canEditBasicFields,
    modelSearchSummary.addable,
    noPermissionMessage,
    t,
    updateModels,
  ])

  const handleAddModelSearchResultsPress = useCallback(
    (event: { preventDefault: () => void; stopPropagation: () => void }) => {
      event.preventDefault()
      event.stopPropagation()

      if (modelSearchPointerHandledRef.current) return

      modelSearchPointerHandledRef.current = true
      handleAddModelSearchResults()
      window.setTimeout(() => {
        modelSearchPointerHandledRef.current = false
      }, 0)
    },
    [handleAddModelSearchResults]
  )

  // 提交成功后刷新渠道列表并关闭抽屉。
  const handleSuccess = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
    if (channelId) {
      queryClient.invalidateQueries({
        queryKey: channelsQueryKeys.detail(channelId),
      })
    }
    onOpenChange(false)
    setOpen(null)
  }, [channelId, queryClient, onOpenChange, setOpen])

  const upstreamCreateMutation = useMutation({
    mutationFn: createUpstreamAccountChannel,
    onSuccess: (res) => {
      if (!res.success) {
        if (isUpstreamPreviewExpiredError(res.message)) {
          clearUpstreamCreatePreview()
          setUpstreamAccountConfigs({})
          showUpstreamPreviewExpiredToast()
          return
        }
        toast.error(res.message || t('Failed to create channel'))
        return
      }
      toast.success(t('Channel created'))
      handleSuccess()
    },
    onError: (error: unknown) => {
      const message =
        error instanceof Error ? error.message : t('Failed to create channel')
      toast.error(message)
    },
  })

  const upstreamRefreshMutation = useMutation({
    mutationFn: ({
      id,
      payload,
    }: {
      id: number
      payload: Parameters<typeof refreshUpstreamAccountChannel>[1]
    }) => refreshUpstreamAccountChannel(id, payload),
    onSuccess: (res) => {
      if (!res.success) {
        if (isUpstreamPreviewExpiredError(res.message)) {
          clearUpstreamRefreshPreview()
          setUpstreamAccountConfigs({})
          showUpstreamPreviewExpiredToast()
          return
        }
        toast.error(res.message || t('Failed to refresh upstream account'))
        return
      }
      toast.success(
        t(
          'Upstream account refreshed: {{created}} created, {{updated}} updated, {{disabled}} disabled',
          {
            created: res.data?.created ?? 0,
            updated: res.data?.updated ?? 0,
            disabled: res.data?.disabled ?? 0,
          }
        )
      )
      setUpstreamPassword('')
      clearUpstreamRefreshPreview()
      setUpstreamAccountConfigs({})
      queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() })
      if (channelId) {
        queryClient.invalidateQueries({
          queryKey: channelsQueryKeys.detail(channelId),
        })
      }
    },
    onError: (error: unknown) => {
      const message =
        error instanceof Error
          ? error.message
          : t('Failed to refresh upstream account')
      toast.error(message)
    },
  })

  const saveSyncedAccountLocalConfigs = useCallback(
    async (_data: ChannelFormValues) => {
      if (!channelId || !renderCurrentRow) return
      if (!canEditBasicFields) {
        toast.error(noPermissionMessage)
        return
      }
      if (!permissions.canWriteChannelAccount) {
        toast.error(noPermissionMessage)
        return
      }
      if (syncedChannelAccountsQuery.isLoading) {
        toast.error(t('Channel account list is still loading'))
        return
      }
      if (syncedChannelAccountsTotal > syncedChannelAccountsLoadedCount) {
        toast.error(
          t(
            'Only {{loaded}} of {{total}} synced keys are loaded. Open the channel account list to edit all keys.',
            {
              loaded: syncedChannelAccountsLoadedCount,
              total: syncedChannelAccountsTotal,
            }
          )
        )
        return
      }

      setIsSavingSyncedAccountConfigs(true)
      try {
        const aggregatedChannelModels = upstreamAccountValuesToString(
          syncedEditableAccounts,
          upstreamAccountConfigs,
          upstreamAccountModelsValue
        )
        const normalizedData: ChannelFormValues = {
          ..._data,
          type:
            channelTypeFromUpstreamPlatform(
              getUpstreamSyncPlatformFromSettings(renderCurrentRow.settings)
            ) ?? _data.type,
          models: aggregatedChannelModels,
          group: _data.group?.length ? _data.group : ['default'],
        }
        const channelPayload = transformFormDataToUpdatePayload(
          normalizedData,
          channelId
        )
        const allowedChannelPayload = buildAllowedChannelUpdatePayload({
          payload: channelPayload,
          canEditSensitiveFields,
          isMultiKeyChannel,
          keyMode: normalizedData.key_mode,
        })
        const channelResponse = await updateChannel(
          channelId,
          allowedChannelPayload
        )
        if (!channelResponse.success) {
          throw new Error(channelResponse.message || t('Operation failed'))
        }

        let updated = 0
        let statusUpdated = 0
        for (let index = 0; index < syncedEditableAccounts.length; index += 1) {
          const editableAccount = syncedEditableAccounts[index]
          if (!editableAccount.account_id) continue
          const config = getUpstreamAccountConfig(
            upstreamAccountConfigs,
            editableAccount,
            index
          )
          if (!config) continue

          const account = syncedChannelAccounts.find(
            (item) => item.id === editableAccount.account_id
          )
          const accountResponse = await updateChannelAccount(
            channelId,
            editableAccount.account_id,
            {
              models: upstreamAccountModelsValue(editableAccount, config),
              group: upstreamAccountGroupValue(editableAccount, config),
              priority: config.priority,
              weight: config.weight,
            }
          )
          if (!accountResponse.success) {
            throw new Error(accountResponse.message || t('Operation failed'))
          }
          updated += 1

          const nextEnabled = config.enabled === true
          const currentEnabled =
            (account?.status ?? editableAccount.account_status) ===
            CHANNEL_STATUS.ENABLED
          if (nextEnabled !== currentEnabled) {
            if (!permissions.canOperateChannelAccount) {
              toast.warning(
                t(
                  'Saved key configuration, but status changes require operate permission.'
                )
              )
              continue
            }
            const statusResponse = await updateChannelAccountStatus(
              channelId,
              editableAccount.account_id,
              {
                status: nextEnabled
                  ? CHANNEL_STATUS.ENABLED
                  : CHANNEL_STATUS.MANUAL_DISABLED,
                reason: nextEnabled ? '' : 'upstream account sync disabled',
                clear_cooldown: nextEnabled,
              }
            )
            if (!statusResponse.success) {
              throw new Error(statusResponse.message || t('Operation failed'))
            }
            statusUpdated += 1
          }
        }

        toast.success(
          t(
            'Synced key configuration saved: {{updated}} key update(s) applied.',
            {
              updated: updated + statusUpdated,
            }
          )
        )
        handleSuccess()
      } catch (error) {
        toast.error(
          error instanceof Error
            ? error.message
            : t(ERROR_MESSAGES.UPDATE_FAILED)
        )
      } finally {
        setIsSavingSyncedAccountConfigs(false)
      }
    },
    [
      canEditBasicFields,
      canEditSensitiveFields,
      channelId,
      handleSuccess,
      isMultiKeyChannel,
      noPermissionMessage,
      permissions.canOperateChannelAccount,
      permissions.canWriteChannelAccount,
      renderCurrentRow,
      syncedChannelAccounts,
      syncedChannelAccountsLoadedCount,
      syncedChannelAccountsQuery.isLoading,
      syncedChannelAccountsTotal,
      syncedEditableAccounts,
      t,
      upstreamAccountConfigs,
    ]
  )

  const handleRefreshUpstreamAccount = useCallback(async () => {
    if (!channelId) return
    if (!permissions.canSensitiveWrite) {
      toast.error(noPermissionMessage)
      return
    }
    if (!upstreamRefreshPreviewId || !upstreamRefreshSnapshot) {
      toast.error(t('Preview upstream account before applying refresh'))
      return
    }
    if (isUpstreamRefreshPreviewExpired) {
      clearUpstreamRefreshPreview()
      setUpstreamAccountConfigs({})
      showUpstreamPreviewExpiredToast()
      return
    }
    await upstreamRefreshMutation.mutateAsync({
      id: channelId,
      payload: {
        preview_id: upstreamRefreshPreviewId,
        apply_suggested: upstreamApplySuggested,
        disable_missing_key: true,
        ratio_conversion: upstreamRatioConversion,
        accounts: buildUpstreamAccountPayloads(
          upstreamRefreshSnapshot.keys,
          upstreamAccountConfigs,
          upstreamApplySuggested
        ),
      },
    })
  }, [
    channelId,
    noPermissionMessage,
    permissions.canSensitiveWrite,
    t,
    upstreamApplySuggested,
    upstreamAccountConfigs,
    upstreamRefreshPreviewId,
    upstreamRefreshSnapshot,
    upstreamRefreshMutation,
    upstreamRatioConversion,
    clearUpstreamRefreshPreview,
    isUpstreamRefreshPreviewExpired,
    showUpstreamPreviewExpiredToast,
  ])

  const handlePreviewUpstreamRefresh = useCallback(async () => {
    if (!permissions.canSensitiveWrite) {
      toast.error(noPermissionMessage)
      return
    }
    const usingSavedCredential =
      isUpstreamAccountSyncedChannel &&
      upstreamUseSavedCredential &&
      savedUpstreamCredentialAvailable
    if (upstreamUseSavedCredential && !savedUpstreamCredentialAvailable) {
      toast.error(
        t(
          'No saved upstream login is available yet. Complete a sync once to enable it.'
        )
      )
      return
    }
    if (!usingSavedCredential) {
      const baseUrl = upstreamBaseUrl.trim()
      if (!baseUrl) {
        toast.error(t('Upstream platform URL is required'))
        return
      }
      if (!upstreamUsername.trim() || !upstreamPassword.trim()) {
        toast.error(t('Account and password are required'))
        return
      }
    }
    const refreshPlatform = forcedUpstreamPlatform ?? upstreamPlatform
    const res = await upstreamPreviewMutation.mutateAsync(
      usingSavedCredential
        ? {
            channel_id: channelId ?? undefined,
            platform: refreshPlatform,
            base_url: upstreamBaseUrl.trim(),
            ratio_conversion: upstreamRatioConversion,
          }
        : {
            platform: refreshPlatform,
            base_url: upstreamBaseUrl.trim(),
            username:
              refreshPlatform === 'new-api' ? upstreamUsername : undefined,
            email:
              refreshPlatform === 'sub2api' ? upstreamUsername : undefined,
            password: upstreamPassword,
            ratio_conversion: upstreamRatioConversion,
          }
    )
    if (!res.success || !res.data) {
      toast.error(res.message || t('Failed to sync upstream account'))
      return
    }
    const challenge = getUpstreamPreviewChallenge(res.data)
    if (challenge) {
      applyUpstreamPreviewChallenge(challenge, 'refresh')
      return
    }
    if (hasUpstreamPreviewSnapshot(res.data)) {
      applyUpstreamPreviewData(res.data, 'refresh')
      return
    }
    toast.error(t('Failed to sync upstream account'))
  }, [
    applyUpstreamPreviewChallenge,
    applyUpstreamPreviewData,
    forcedUpstreamPlatform,
    noPermissionMessage,
    permissions.canSensitiveWrite,
    t,
    upstreamBaseUrl,
    upstreamPassword,
    upstreamPlatform,
    upstreamUseSavedCredential,
    upstreamPreviewMutation,
    upstreamRatioConversion,
    upstreamUsername,
    savedUpstreamCredentialAvailable,
    isUpstreamAccountSyncedChannel,
    channelId,
  ])

  const handleCompleteUpstreamTwoFactor = useCallback(
    async (mode: UpstreamTwoFactorMode) => {
      const challenge =
        mode === 'create'
          ? upstreamTwoFactorChallenge
          : upstreamRefreshTwoFactorChallenge
      const code =
        mode === 'create'
          ? upstreamTwoFactorCode.trim()
          : upstreamRefreshTwoFactorCode.trim()
      const expired =
        mode === 'create'
          ? isUpstreamTwoFactorExpired
          : isUpstreamRefreshTwoFactorExpired

      if (!challenge) return
      if (expired) {
        toast.error(
          t(
            'The upstream 2FA challenge expired. Sync the upstream account again.'
          )
        )
        if (mode === 'create') {
          clearUpstreamCreatePreview()
        } else {
          clearUpstreamRefreshPreview()
        }
        setUpstreamAccountConfigs({})
        return
      }
      if (!code) {
        toast.error(t('Enter the upstream 2FA code'))
        return
      }

      const res = await upstreamPreview2FAMutation.mutateAsync({
        challenge_id: challenge.challenge_id,
        code,
        ratio_conversion: upstreamRatioConversion,
      })
      if (!res.success || !res.data) {
        toast.error(res.message || t('Failed to verify upstream 2FA code'))
        if (mode === 'create') {
          clearUpstreamCreatePreview()
        } else {
          clearUpstreamRefreshPreview()
        }
        setUpstreamAccountConfigs({})
        return
      }
      applyUpstreamPreviewData(res.data, mode)
    },
    [
      applyUpstreamPreviewData,
      clearUpstreamCreatePreview,
      clearUpstreamRefreshPreview,
      isUpstreamRefreshTwoFactorExpired,
      isUpstreamTwoFactorExpired,
      t,
      upstreamRatioConversion,
      upstreamPreview2FAMutation,
      upstreamRefreshTwoFactorChallenge,
      upstreamRefreshTwoFactorCode,
      upstreamTwoFactorChallenge,
      upstreamTwoFactorCode,
    ]
  )

  const renderUpstreamTwoFactorChallenge = useCallback(
    (
      mode: UpstreamTwoFactorMode,
      challenge: UpstreamAccountTwoFactorChallenge,
      code: string,
      setCode: (value: string) => void,
      remainingSeconds: number,
      expired: boolean
    ) => (
      <Alert>
        <AlertCircle aria-hidden='true' />
        <AlertTitle>{t('Upstream 2FA required')}</AlertTitle>
        <AlertDescription>
          <div className='flex flex-col gap-3'>
            <p>
              {expired
                ? t(
                    'The upstream 2FA challenge expired. Sync the upstream account again.'
                  )
                : t(
                    'Enter the TOTP code for {{account}}. This challenge expires in {{time}}.',
                    {
                      account: challenge.username || t('the upstream account'),
                      time: formatUpstreamPreviewRemaining(remainingSeconds),
                    }
                  )}
            </p>
            <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-end'>
              <div className='flex flex-col gap-2'>
                <Label
                  htmlFor={
                    mode === 'create'
                      ? 'upstream-sync-2fa-code'
                      : 'upstream-refresh-2fa-code'
                  }
                >
                  {t('Verification Code')}
                </Label>
                <Input
                  id={
                    mode === 'create'
                      ? 'upstream-sync-2fa-code'
                      : 'upstream-refresh-2fa-code'
                  }
                  value={code}
                  onChange={(event) => setCode(event.target.value)}
                  inputMode='numeric'
                  autoComplete='one-time-code'
                  placeholder={t('Enter the upstream 2FA code')}
                  disabled={expired || upstreamPreview2FAMutation.isPending}
                />
              </div>
              <Button
                type='button'
                disabled={
                  expired ||
                  upstreamPreview2FAMutation.isPending ||
                  !code.trim()
                }
                onClick={() => handleCompleteUpstreamTwoFactor(mode)}
              >
                {upstreamPreview2FAMutation.isPending ? (
                  <Loader2 data-icon='inline-start' className='animate-spin' />
                ) : (
                  <CheckCircle2 data-icon='inline-start' />
                )}
                {t('Continue Sync')}
              </Button>
            </div>
          </div>
        </AlertDescription>
      </Alert>
    ),
    [handleCompleteUpstreamTwoFactor, t, upstreamPreview2FAMutation.isPending]
  )

  // 模型映射源模型缺失时弹出确认，避免管理员保存后看不到映射入口模型。
  const confirmMissingModelMappings = useCallback(
    (missingModels: string[]): Promise<MissingModelsAction> => {
      return new Promise((resolve) => {
        setMissingModelsList(missingModels)
        setMissingModelsDialogOpen(true)
        missingModelsResolveRef.current = resolve
      })
    },
    []
  )

  // 处理模型缺失确认弹窗的用户选择。
  const handleMissingModelsAction = useCallback(
    (action: MissingModelsAction) => {
      setMissingModelsDialogOpen(false)
      if (missingModelsResolveRef.current) {
        missingModelsResolveRef.current(action)
        missingModelsResolveRef.current = null
      }
    },
    []
  )

  const confirmStatusCodeRisk = useCallback(
    (detailItems: string[]): Promise<boolean> =>
      new Promise((resolve) => {
        statusCodeRiskResolveRef.current = resolve
        setStatusCodeRiskDetailItems(detailItems)
        setStatusCodeRiskOpen(true)
      }),
    []
  )

  const handleStatusCodeRiskAction = useCallback((confirmed: boolean) => {
    setStatusCodeRiskOpen(false)
    setStatusCodeRiskDetailItems([])
    if (statusCodeRiskResolveRef.current) {
      statusCodeRiskResolveRef.current(confirmed)
      statusCodeRiskResolveRef.current = null
    }
  }, [])

  useEffect(() => {
    return () => {
      if (statusCodeRiskResolveRef.current) {
        statusCodeRiskResolveRef.current(false)
        statusCodeRiskResolveRef.current = null
      }
    }
  }, [])

  const { mutateAsync: submitChannelMutation, isPending: isSubmitting } =
    useChannelMutateForm({
      currentRow: renderCurrentRow,
      isEditing,
      isMultiKeyChannel,
      permissions,
      onSuccess: handleSuccess,
    })

  // 提交前先做前端侧快速校验，避免把明显不完整的数据发给后端。
  //
  // 注意：`global_account_pool` 是用户当前看到的“账号池”模式，它只需要选择账号池组，
  // 上游 token 由组内账号提供，不再要求渠道自身填写 API Key 或 Base URL。
  // 旧的 `account_pool` 仍表示“渠道内账号池”，只在编辑历史渠道时保留入口。
  const onSubmit = useCallback(
    async (data: ChannelFormValues) => {
      if (!isEditing && !permissions.canSensitiveWrite) {
        toast.error(noPermissionMessage)
        return
      }
      if (isEditing && !canEditBasicFields) {
        toast.error(noPermissionMessage)
        return
      }
      if (
        isEditing &&
        !canEditSensitiveFields &&
        hasDirtySensitiveChannelFormFields(
          form.formState.dirtyFields as Partial<Record<string, unknown>>
        )
      ) {
        toast.error(
          t('You do not have permission to edit sensitive channel settings.')
        )
        return
      }

      if (!isEditing && isCreateUpstreamSyncMode) {
        if (!upstreamPreviewId || !upstreamSnapshot) {
          toast.error(t('Sync upstream account before creating the channel'))
          return
        }
        if (isUpstreamPreviewExpired) {
          clearUpstreamCreatePreview()
          setUpstreamAccountConfigs({})
          showUpstreamPreviewExpiredToast()
          return
        }
        if (upstreamSnapshot.keys.length === 0) {
          toast.error(t('No upstream keys were found for this account.'))
          return
        }
        const upstreamChannelModels =
          upstreamAccountValuesToString(
            upstreamSnapshot.keys,
            upstreamAccountConfigs,
            upstreamAccountModelsValue
          ) || data.models
        const upstreamChannelGroup = resolveUpstreamChannelGroup(data.group)
        await upstreamCreateMutation.mutateAsync({
          preview_id: upstreamPreviewId,
          apply_suggested: upstreamApplySuggested,
          ratio_conversion: upstreamRatioConversion,
          channel: {
            name: data.name,
            type: data.type,
            base_url:
              normalizeUpstreamChannelBaseUrl(upstreamSnapshot.base_url) ||
              normalizeUpstreamChannelBaseUrl(upstreamBaseUrl) ||
              null,
            models: upstreamChannelModels,
            group: upstreamChannelGroup,
            status: data.status,
            priority: data.priority ?? null,
            weight: data.weight ?? null,
          },
          accounts: buildUpstreamAccountPayloads(
            upstreamSnapshot.keys,
            upstreamAccountConfigs,
            upstreamApplySuggested,
            upstreamChannelModels,
            ''
          ),
        })
        return
      }

      if (isEditingUnsupportedUnsyncedUpstreamType) {
        toast.error(
          t(
            'new-api and sub2api channel types must use upstream account sync. Create a synced channel or refresh an existing synced channel.'
          )
        )
        return
      }

      if (isEditing && isUpstreamAccountSyncedChannel) {
        await saveSyncedAccountLocalConfigs(data)
        return
      }

      const isAccountPoolGroupMode =
        data.credential_mode === 'global_account_pool'

      if (!isEditing && !isAccountPoolGroupMode && !data.key?.trim()) {
        form.setError('key', {
          type: 'manual',
          message: ERROR_MESSAGES.REQUIRED_KEY,
        })
        return
      }

      // 状态码复写会直接影响本地重试和禁用判断，提交前必须先拦截非法状态码。
      if (data.status_code_mapping?.trim()) {
        const invalidEntries = collectInvalidStatusCodeEntries(
          data.status_code_mapping
        )
        if (invalidEntries.length > 0) {
          toast.error(
            t('Invalid status code mapping entries: {{entries}}', {
              entries: invalidEntries.join(', '),
            })
          )
          return
        }

        const riskyRedirects = collectNewDisallowedStatusCodeRedirects(
          initialStatusCodeMappingRef.current,
          data.status_code_mapping
        )
        if (riskyRedirects.length > 0) {
          const confirmed = await confirmStatusCodeRisk(riskyRedirects)
          if (!confirmed) return
        }
      }

      // 模型映射既影响用户可见模型，也影响实际请求模型；格式错误时不能保存。
      const hasModelMapping =
        typeof data.model_mapping === 'string' &&
        data.model_mapping.trim() !== ''

      if (hasModelMapping) {
        const validation = validateModelMappingJson(data.model_mapping!)
        if (!validation.valid) {
          toast.error(t(validation.error || 'Invalid model mapping'))
          return
        }
      }

      // 模型字段最终以逗号分隔字符串提交，先归一化便于后续映射检查。
      const normalizedModels = parseModelsString(data.models || '')

      // 当模型映射的源模型没有出现在渠道模型列表中时，请用户确认是否自动补齐。
      if (hasModelMapping) {
        const missingModels = findMissingModelsInMapping(
          data.model_mapping!,
          normalizedModels
        )

        const shouldPromptMissing =
          missingModels.length > 0 &&
          hasModelConfigChanged(
            normalizedModels,
            data.model_mapping || '',
            initialModelsRef.current,
            initialModelMappingRef.current
          )

        if (shouldPromptMissing) {
          const confirmAction = await confirmMissingModelMappings(missingModels)
          if (confirmAction === 'cancel') {
            return
          }
          if (confirmAction === 'add') {
            const updatedModels = Array.from(
              new Set([...normalizedModels, ...missingModels])
            )
            data.models = formatModelsArray(updatedModels)
            form.setValue('models', data.models)
          }
        }
      }

      try {
        await submitChannelMutation(data)
      } catch {
        // mutation 的 onError 已经负责展示后端或权限错误，这里只阻止异常继续冒泡到表单层。
      }
    },
    [
      isEditing,
      canEditBasicFields,
      canEditSensitiveFields,
      channelId,
      noPermissionMessage,
      permissions.canSensitiveWrite,
      form,
      confirmMissingModelMappings,
      confirmStatusCodeRisk,
      saveSyncedAccountLocalConfigs,
      submitChannelMutation,
      t,
      upstreamAccountConfigs,
      upstreamApplySuggested,
      upstreamCreateMutation,
      upstreamRatioConversion,
      clearUpstreamCreatePreview,
      isCreateUpstreamSyncMode,
      isEditingUnsupportedUnsyncedUpstreamType,
      isUpstreamAccountSyncedChannel,
      isUpstreamPreviewExpired,
      showUpstreamPreviewExpiredToast,
      upstreamBaseUrl,
      upstreamPreviewId,
      upstreamSnapshot,
    ]
  )

  // 关闭抽屉时同步重置表单状态，避免下一次新建渠道沿用上一次编辑残留字段。
  const handleOpenChange = useCallback(
    (v: boolean) => {
      onOpenChange(v)
      if (!v) {
        form.reset(CHANNEL_FORM_DEFAULT_VALUES)
        advancedNavScrollPendingRef.current = false
        setActiveEditorSectionId(CHANNEL_EDITOR_SECTION_IDS.identity)
        setExpandedEditorNavItemId(undefined)
        setAdvancedSettingsOpen(false)
        setAdvancedCustomEditorOpen(false)
        setSyncRefreshOpen(false)
        form.setValue('upstream_account_sync', false)
        setUpstreamBaseUrl('')
        setUpstreamUsername('')
        setUpstreamPassword('')
        setUpstreamPaidCny('')
        setUpstreamPlatformUsdCredit('')
        clearAllUpstreamPreviews()
        upstreamCredentialFingerprintRef.current = ''
      }
    },
    [clearAllUpstreamPreviews, onOpenChange, form]
  )

  const handleAdvancedSettingsOpenChange = useCallback((nextOpen: boolean) => {
    if (!nextOpen) {
      advancedNavScrollPendingRef.current = false
      setExpandedEditorNavItemId(undefined)
    }
    setAdvancedSettingsOpen(nextOpen)
    if (typeof window !== 'undefined') {
      window.localStorage.setItem(
        ADVANCED_SETTINGS_EXPANDED_KEY,
        String(nextOpen)
      )
    }
  }, [])

  const handleEditorNavNavigate = useCallback(
    (targetId: string) => {
      const isAdvancedTarget =
        targetId === CHANNEL_EDITOR_SECTION_IDS.advanced ||
        ADVANCED_SETTINGS_CHILD_SECTION_IDS.includes(targetId)

      if (isAdvancedTarget) {
        advancedNavScrollPendingRef.current = true
        handleAdvancedSettingsOpenChange(true)
        setActiveEditorSectionId(CHANNEL_EDITOR_SECTION_IDS.advanced)
        setExpandedEditorNavItemId(CHANNEL_EDITOR_SECTION_IDS.advanced)
      } else {
        advancedNavScrollPendingRef.current = false
        setActiveEditorSectionId(targetId)
        setExpandedEditorNavItemId(undefined)
      }

      const scrollTargetIntoView = () => {
        document
          .querySelector<HTMLElement>(`#${targetId}`)
          ?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      }

      window.requestAnimationFrame(scrollTargetIntoView)
    },
    [handleAdvancedSettingsOpenChange]
  )

  const updateActiveEditorSection = useCallback(() => {
    const formElement = channelFormRef.current
    if (!formElement) return

    const activationY = formElement.getBoundingClientRect().top + 80
    let nextActiveSectionId: string = CHANNEL_EDITOR_SECTION_IDS.identity

    for (const sectionId of CHANNEL_EDITOR_MAIN_SECTION_IDS) {
      const sectionElement = document.querySelector<HTMLElement>(
        `#${sectionId}`
      )
      if (!sectionElement) continue
      if (sectionElement.getBoundingClientRect().top <= activationY) {
        nextActiveSectionId = sectionId
      } else {
        break
      }
    }

    setActiveEditorSectionId((current) =>
      current === nextActiveSectionId ? current : nextActiveSectionId
    )

    if (nextActiveSectionId === CHANNEL_EDITOR_SECTION_IDS.advanced) {
      advancedNavScrollPendingRef.current = false
      setExpandedEditorNavItemId(CHANNEL_EDITOR_SECTION_IDS.advanced)
      if (!advancedSettingsOpen) {
        handleAdvancedSettingsOpenChange(true)
      }
    } else if (!advancedNavScrollPendingRef.current) {
      setExpandedEditorNavItemId(undefined)
    }
  }, [advancedSettingsOpen, handleAdvancedSettingsOpenChange])

  useEffect(() => {
    if (!open || isChannelDetailLoading) return
    const formElement = channelFormRef.current
    if (!formElement) return

    updateActiveEditorSection()
    formElement.addEventListener('scroll', updateActiveEditorSection, {
      passive: true,
    })
    window.addEventListener('resize', updateActiveEditorSection)

    return () => {
      formElement.removeEventListener('scroll', updateActiveEditorSection)
      window.removeEventListener('resize', updateActiveEditorSection)
    }
  }, [isChannelDetailLoading, open, updateActiveEditorSection])

  const onInvalid: SubmitErrorHandler<ChannelFormValues> = useCallback(
    (errors) => {
      if (hasAdvancedSettingsErrors(errors)) {
        handleAdvancedSettingsOpenChange(true)
      }
      toast.error(t('Please fix the highlighted fields before saving'))
    },
    [handleAdvancedSettingsOpenChange, t]
  )

  return (
    <>
      <Sheet open={open} onOpenChange={handleOpenChange}>
        <SheetContent
          className={sideDrawerContentClassName(
            'sm:max-w-[96vw] xl:max-w-[1700px]'
          )}
        >
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle className='flex items-center gap-3'>
              <span className='bg-muted flex size-9 shrink-0 items-center justify-center rounded-md'>
                <ChannelTypeIcon type={currentType} size={22} />
              </span>
              <span>
                {isEditing ? t('Edit Channel') : t('Create Channel')}
                <span className='text-muted-foreground ml-2 text-sm font-normal'>
                  {t(currentTypeLabel)}
                </span>
              </span>
            </SheetTitle>
            <SheetDescription>
              {isEditing
                ? t(
                    "Update channel configuration and click save when you're done."
                  )
                : t(
                    'Add a new channel by providing the necessary information.'
                  )}
            </SheetDescription>
          </SheetHeader>

          {sensitiveFieldsReadOnly && (
            <Alert className='mx-4 mt-4 sm:mx-6'>
              <AlertCircle aria-hidden='true' />
              <AlertTitle>
                {t('Sensitive channel settings are read-only')}
              </AlertTitle>
              <AlertDescription>
                {t(
                  'You can still edit non-sensitive fields such as models, groups, priority, and weight.'
                )}
              </AlertDescription>
            </Alert>
          )}

          <Form {...form}>
            <form
              id='channel-form'
              ref={channelFormRef}
              onSubmit={form.handleSubmit(onSubmit, onInvalid)}
              className={sideDrawerFormClassName('gap-5')}
            >
              {isChannelDetailLoading && <ChannelEditorLoadingState />}
              <div
                className={cn(
                  'grid gap-5 lg:grid-cols-[13rem_minmax(0,1fr)] lg:items-start xl:grid-cols-[15rem_minmax(0,1fr)]',
                  isChannelDetailLoading && 'hidden'
                )}
              >
                <ChannelEditorNav
                  providerLogo={
                    <ChannelTypeIcon type={currentType} size={18} />
                  }
                  providerLabel={t(currentTypeLabel)}
                  statusLabel={t(currentStatusLabel)}
                  progressLabel={progressLabel}
                  navigationLabel={t('Channels')}
                  items={editorNavItems}
                  activeItemId={activeEditorSectionId}
                  expandedItemId={expandedEditorNavItemId}
                  onNavigate={handleEditorNavNavigate}
                />
                <div className='flex min-w-0 flex-col gap-5'>
                  {/* ── Basic Information ── */}
                  <div
                    id={CHANNEL_EDITOR_SECTION_IDS.identity}
                    className='scroll-mt-4'
                  >
                    <ChannelBasicSection>
                      <div className='grid gap-4 sm:grid-cols-2'>
                        <FormField
                          control={form.control}
                          name='type'
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel htmlFor='channel-type'>
                                {t('Type *')}
                              </FormLabel>
                              <FormControl>
                                <div className='relative'>
                                  <span className='pointer-events-none absolute top-1/2 left-3 z-10 flex -translate-y-1/2'>
                                    <ChannelTypeIcon
                                      type={Number(field.value)}
                                      size={18}
                                    />
                                  </span>
                                  <Combobox
                                    id='channel-type'
                                    options={channelTypeOptions}
                                    value={String(field.value)}
                                    onValueChange={(value) => {
                                      if (!canEditSensitiveFields) {
                                        toast.error(noPermissionMessage)
                                        return
                                      }
                                      const nextType = Number(value)
                                      if (
                                        Number.isInteger(nextType) &&
                                        nextType > 0
                                      ) {
                                        field.onChange(nextType)
                                      }
                                    }}
                                    placeholder={t('Select channel type')}
                                    searchPlaceholder={t(
                                      'Search channel type...'
                                    )}
                                    openOnFocus={false}
                                    emptyText={t('No channel type found.')}
                                    allowCustomValue
                                    className={cn(
                                      'pl-10',
                                      !canEditSensitiveFields &&
                                        'pointer-events-none opacity-50'
                                    )}
                                  />
                                </div>
                              </FormControl>
                              <FormMessage />
                            </FormItem>
                          )}
                        />

                        <FormField
                          control={form.control}
                          name='name'
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t('Name *')}</FormLabel>
                              <FormControl>
                                <Input
                                  autoComplete='off'
                                  placeholder={t(FIELD_PLACEHOLDERS.NAME)}
                                  {...field}
                                />
                              </FormControl>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                      </div>

                      {!isEditing && (
                        <FormField
                          control={form.control}
                          name='status'
                          render={({ field }) => (
                            <FormItem
                              className={sideDrawerSwitchItemClassName()}
                            >
                              <div className='flex flex-col gap-0.5'>
                                <FormLabel>{t('Enabled')}</FormLabel>
                                <FormDescription className='text-xs'>
                                  {t('Enable or disable this channel')}
                                </FormDescription>
                              </div>
                              <FormControl>
                                <Switch
                                  checked={field.value === 1}
                                  disabled={!permissions.canOperate}
                                  onCheckedChange={(checked) =>
                                    field.onChange(checked ? 1 : 2)
                                  }
                                />
                              </FormControl>
                            </FormItem>
                          )}
                        />
                      )}

                      {currentType === 1 &&
                        !isGlobalAccountPoolMode &&
                        showManualCredentialSection && (
                          <FormField
                            control={form.control}
                            name='openai_organization'
                            render={({ field }) => (
                              <FormItem>
                                <FormLabel>
                                  {t('OpenAI Organization')}
                                </FormLabel>
                                <FormControl>
                                  <Input
                                    placeholder={t('org-...')}
                                    disabled={!canEditSensitiveFields}
                                    {...field}
                                  />
                                </FormControl>
                                <FormDescription>
                                  {t(FIELD_DESCRIPTIONS.OPENAI_ORG)}
                                </FormDescription>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        )}
                    </ChannelBasicSection>
                  </div>

                  {/* ── Credentials ── */}
                  <div
                    id={CHANNEL_EDITOR_SECTION_IDS.credentials}
                    className='scroll-mt-4'
                  >
                    <ChannelApiAccessSection>
                      {CHANNEL_TYPE_WARNINGS[currentType] && (
                        <Alert>
                          <AlertDescription>
                            {t(CHANNEL_TYPE_WARNINGS[currentType])}
                          </AlertDescription>
                        </Alert>
                      )}

                      {!isEditing && isCreateUpstreamSyncMode && (
                        <div className='border-border/60 bg-muted/10 rounded-lg border p-4'>
                          <div className='flex flex-col gap-4'>
                            <div className='flex flex-col gap-1'>
                              <div className='flex items-center gap-2'>
                                <KeyRound aria-hidden='true' />
                                <span className='text-sm font-semibold'>
                                  {t('Upstream Account Sync')}
                                </span>
                              </div>
                              <p className='text-muted-foreground text-xs'>
                                {t(
                                  'This channel type uses upstream account sync to fetch keys, groups, rates, and balance.'
                                )}
                              </p>
                            </div>

                            <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-6'>
                              <div className='flex flex-col gap-2'>
                                <Label htmlFor='upstream-sync-base-url'>
                                  {t('Upstream Platform URL')}
                                </Label>
                                <Input
                                  id='upstream-sync-base-url'
                                  value={upstreamBaseUrl}
                                  onChange={(event) =>
                                    setUpstreamBaseUrl(event.target.value)
                                  }
                                  placeholder={t('new-api or sub2api site URL')}
                                  disabled={!canEditSensitiveFields}
                                />
                              </div>
                              <div className='flex flex-col gap-2'>
                                <Label htmlFor='upstream-sync-account'>
                                  {t('Account')}
                                </Label>
                                <Input
                                  id='upstream-sync-account'
                                  value={upstreamUsername}
                                  onChange={(event) =>
                                    setUpstreamUsername(event.target.value)
                                  }
                                  autoComplete='username'
                                  placeholder={
                                    upstreamPlatform === 'new-api'
                                      ? t('Username')
                                      : t('Email')
                                  }
                                />
                              </div>
                              <div className='flex flex-col gap-2'>
                                <Label htmlFor='upstream-sync-password'>
                                  {t('Password')}
                                </Label>
                                <Input
                                  id='upstream-sync-password'
                                  value={upstreamPassword}
                                  onChange={(event) =>
                                    setUpstreamPassword(event.target.value)
                                  }
                                  type='password'
                                  autoComplete='current-password'
                                  placeholder={t('Password')}
                                />
                              </div>
                              <div className='flex flex-col gap-2'>
                                <Label htmlFor='upstream-sync-paid-cny'>
                                  {t('Paid CNY')}
                                </Label>
                                <Input
                                  id='upstream-sync-paid-cny'
                                  value={upstreamPaidCny}
                                  onChange={(event) =>
                                    setUpstreamPaidCny(event.target.value)
                                  }
                                  inputMode='decimal'
                                  placeholder='1'
                                />
                              </div>
                              <div className='flex flex-col gap-2'>
                                <Label htmlFor='upstream-sync-platform-usd-credit'>
                                  {t('Platform USD Credit')}
                                </Label>
                                <Input
                                  id='upstream-sync-platform-usd-credit'
                                  value={upstreamPlatformUsdCredit}
                                  onChange={(event) =>
                                    setUpstreamPlatformUsdCredit(
                                      event.target.value
                                    )
                                  }
                                  inputMode='decimal'
                                  placeholder='20'
                                />
                              </div>
                              <div className='flex items-end'>
                                <Button
                                  type='button'
                                  variant='outline'
                                  className='w-full'
                                  disabled={
                                    upstreamPreviewMutation.isPending ||
                                    upstreamPreview2FAMutation.isPending ||
                                    upstreamRefreshMutation.isPending
                                  }
                                  onClick={handlePreviewUpstreamAccount}
                                >
                                  {upstreamPreviewMutation.isPending ? (
                                    <Loader2
                                      data-icon='inline-start'
                                      className='animate-spin'
                                    />
                                  ) : (
                                    <RefreshCw data-icon='inline-start' />
                                  )}
                                  {t('Sync Keys')}
                                </Button>
                              </div>
                            </div>
                            <p className='text-muted-foreground text-xs'>
                              {t(
                                'Used to calculate the actual cost ratio for synced keys. Leave both empty to use the upstream ratio directly.'
                              )}
                            </p>

                            {upstreamTwoFactorChallenge &&
                              renderUpstreamTwoFactorChallenge(
                                'create',
                                upstreamTwoFactorChallenge,
                                upstreamTwoFactorCode,
                                setUpstreamTwoFactorCode,
                                upstreamTwoFactorRemaining,
                                isUpstreamTwoFactorExpired
                              )}

                            {upstreamSnapshot?.warnings?.length ? (
                              <Alert>
                                <AlertCircle aria-hidden='true' />
                                <AlertDescription>
                                  {upstreamSnapshot.warnings.join('；')}
                                </AlertDescription>
                              </Alert>
                            ) : null}

                            {upstreamSnapshot &&
                              renderUpstreamPreviewExpiryNotice(
                                upstreamPreviewRemaining,
                                isUpstreamPreviewExpired
                              )}

                            {upstreamSnapshot &&
                              renderUpstreamSnapshotReview(upstreamSnapshot)}
                          </div>
                        </div>
                      )}

                      {isUpstreamAccountSyncedChannel && (
                        <div className='border-border/60 bg-muted/10 rounded-lg border p-4'>
                          <div className='flex flex-col gap-4'>
                            <div className='flex flex-col gap-1'>
                              <div className='flex items-center gap-2'>
                                <KeyRound aria-hidden='true' />
                                <span className='text-sm font-semibold'>
                                  {t('Synced Key Configuration')}
                                </span>
                              </div>
                              <p className='text-muted-foreground text-xs'>
                                {t(
                                  'Configure models, groups, priority, weight, and enabled state for each synced key. Save changes updates the current keys without refreshing the upstream account.'
                                )}
                              </p>
                            </div>
                            {!permissions.canReadChannelAccount ? (
                              <Alert>
                                <AlertCircle aria-hidden='true' />
                                <AlertDescription>
                                  {noPermissionMessage}
                                </AlertDescription>
                              </Alert>
                            ) : syncedChannelAccountsQuery.isLoading ? (
                              <div className='text-muted-foreground flex items-center gap-2 text-sm'>
                                <Loader2
                                  data-icon='inline-start'
                                  className='animate-spin'
                                />
                                {t('Loading synced keys...')}
                              </div>
                            ) : (
                              <>
                                {syncedChannelAccountsTotal >
                                  syncedChannelAccountsLoadedCount && (
                                  <Alert>
                                    <AlertCircle aria-hidden='true' />
                                    <AlertDescription>
                                      {t(
                                        'Only {{loaded}} of {{total}} synced keys are loaded. Open the channel account list to edit all keys.',
                                        {
                                          loaded:
                                            syncedChannelAccountsLoadedCount,
                                          total: syncedChannelAccountsTotal,
                                        }
                                      )}
                                    </AlertDescription>
                                  </Alert>
                                )}
                                {renderUpstreamSnapshotReview(
                                  { keys: syncedEditableAccounts },
                                  {
                                    showBalance: false,
                                    showSuggestedToggle: false,
                                    emptyText: t(
                                      'No synced keys were found for this channel.'
                                    ),
                                  }
                                )}
                              </>
                            )}
                          </div>
                        </div>
                      )}

                      {isUpstreamAccountSyncedChannel && (
                        <Collapsible
                          open={syncRefreshOpen}
                          onOpenChange={setSyncRefreshOpen}
                          className='border-border/60 bg-muted/10 rounded-lg border'
                        >
                          <CollapsibleTrigger className='hover:bg-muted/40 flex w-full items-center justify-between gap-3 rounded-lg px-4 py-3 text-left transition-colors'>
                            <div className='flex min-w-0 items-start gap-2'>
                              <RefreshCw
                                className='mt-0.5 size-4 shrink-0'
                                aria-hidden='true'
                              />
                              <div className='min-w-0'>
                                <span className='block text-sm font-semibold'>
                                  {t('Refresh Upstream Account')}
                                </span>
                                <span className='text-muted-foreground block text-xs'>
                                  {t(
                                    'Optional upstream re-sync. It does not replace the current synced key configuration until you apply the refresh.'
                                  )}
                                </span>
                              </div>
                            </div>
                            <ChevronDown
                              className={cn(
                                'text-muted-foreground size-4 shrink-0 transition-transform',
                                syncRefreshOpen && 'rotate-180'
                              )}
                              aria-hidden='true'
                            />
                          </CollapsibleTrigger>
                          <CollapsibleContent className='border-border/60 border-t px-4 py-4'>
                            <div className='flex flex-col gap-4'>
                              <div
                                className={sideDrawerSwitchItemClassName(
                                  'rounded-lg px-3 py-3'
                                )}
                              >
                                <div className='min-w-0 flex-1'>
                                  <div className='text-sm font-medium'>
                                    {t('Use saved upstream login')}
                                  </div>
                                  <div className='text-muted-foreground text-xs'>
                                    {savedUpstreamCredentialAvailable
                                      ? t(
                                          'Reuse the encrypted upstream account credential saved after the last successful sync.'
                                        )
                                      : t(
                                          'No saved upstream login is available yet. Complete a sync once to enable it.'
                                        )}
                                  </div>
                                </div>
                                <Switch
                                  checked={upstreamUseSavedCredential}
                                  disabled={
                                    !savedUpstreamCredentialAvailable ||
                                    upstreamPreviewMutation.isPending ||
                                    upstreamRefreshMutation.isPending
                                  }
                                  onCheckedChange={setUpstreamUseSavedCredential}
                                />
                              </div>
                              <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-6'>
                                <div className='flex flex-col gap-2'>
                                  <Label htmlFor='upstream-refresh-base-url'>
                                    {t('Upstream Platform URL')}
                                  </Label>
                                  <Input
                                    id='upstream-refresh-base-url'
                                    value={upstreamBaseUrl}
                                    onChange={(event) =>
                                      setUpstreamBaseUrl(event.target.value)
                                    }
                                    placeholder={t(
                                      'new-api or sub2api site URL'
                                    )}
                                    disabled={
                                      !canEditSensitiveFields ||
                                      upstreamUseSavedCredential
                                    }
                                  />
                                </div>
                                <div className='flex flex-col gap-2'>
                                  <Label htmlFor='upstream-refresh-account'>
                                    {t('Account')}
                                  </Label>
                                  <Input
                                    id='upstream-refresh-account'
                                    value={upstreamUsername}
                                    onChange={(event) =>
                                      setUpstreamUsername(event.target.value)
                                    }
                                    autoComplete='username'
                                    placeholder={
                                      upstreamPlatform === 'new-api'
                                        ? t('Username')
                                        : t('Email')
                                    }
                                    disabled={upstreamUseSavedCredential}
                                  />
                                </div>
                                <div className='flex flex-col gap-2'>
                                  <Label htmlFor='upstream-refresh-password'>
                                    {t('Password')}
                                  </Label>
                                  <Input
                                    id='upstream-refresh-password'
                                    value={upstreamPassword}
                                    onChange={(event) =>
                                      setUpstreamPassword(event.target.value)
                                    }
                                    type='password'
                                    autoComplete='current-password'
                                    placeholder={
                                      upstreamUseSavedCredential
                                        ? t('Saved upstream login will be reused')
                                        : t('Password')
                                    }
                                    disabled={upstreamUseSavedCredential}
                                  />
                                </div>
                                <div className='flex flex-col gap-2'>
                                  <Label htmlFor='upstream-refresh-paid-cny'>
                                    {t('Paid CNY')}
                                  </Label>
                                  <Input
                                    id='upstream-refresh-paid-cny'
                                    value={upstreamPaidCny}
                                    onChange={(event) =>
                                      setUpstreamPaidCny(event.target.value)
                                    }
                                    inputMode='decimal'
                                    placeholder='1'
                                  />
                                </div>
                                <div className='flex flex-col gap-2'>
                                  <Label htmlFor='upstream-refresh-platform-usd-credit'>
                                    {t('Platform USD Credit')}
                                  </Label>
                                  <Input
                                    id='upstream-refresh-platform-usd-credit'
                                    value={upstreamPlatformUsdCredit}
                                    onChange={(event) =>
                                      setUpstreamPlatformUsdCredit(
                                        event.target.value
                                      )
                                    }
                                    inputMode='decimal'
                                    placeholder='20'
                                  />
                                </div>
                                <div className='flex flex-col justify-end gap-2'>
                                  <Button
                                    type='button'
                                    variant='outline'
                                    className='flex-1'
                                    disabled={upstreamPreviewMutation.isPending}
                                    onClick={handlePreviewUpstreamRefresh}
                                  >
                                    {upstreamPreviewMutation.isPending ? (
                                      <Loader2
                                        data-icon='inline-start'
                                        className='animate-spin'
                                      />
                                    ) : (
                                      <RefreshCw data-icon='inline-start' />
                                    )}
                                    {t('Preview Refresh')}
                                  </Button>
                                  <Button
                                    type='button'
                                    className='flex-1'
                                    disabled={
                                      !upstreamRefreshSnapshot ||
                                      upstreamRefreshSnapshot.keys.length ===
                                        0 ||
                                      isUpstreamRefreshPreviewExpired ||
                                      upstreamPreviewMutation.isPending ||
                                      upstreamPreview2FAMutation.isPending ||
                                      upstreamRefreshMutation.isPending
                                    }
                                    onClick={handleRefreshUpstreamAccount}
                                  >
                                    {upstreamRefreshMutation.isPending ? (
                                      <Loader2
                                        data-icon='inline-start'
                                        className='animate-spin'
                                      />
                                    ) : (
                                      <CheckCircle2 data-icon='inline-start' />
                                    )}
                                    {t('Apply Refresh')}
                                  </Button>
                                </div>
                              </div>
                              <p className='text-muted-foreground text-xs'>
                                {t(
                                  'Used to calculate the actual cost ratio for synced keys. Leave both empty to use the upstream ratio directly.'
                                )}
                              </p>

                              <Alert>
                                <AlertCircle aria-hidden='true' />
                                <AlertDescription>
                                  {upstreamUseSavedCredential
                                    ? t(
                                        'This refresh will reuse the saved upstream login. If the upstream site asks for 2FA again, only the code is needed.'
                                      )
                                    : t(
                                        'Use this only when you need to log in to the upstream account again. The main save button below only saves per-key models, groups, priority, weight, and enabled state.'
                                      )}
                                </AlertDescription>
                              </Alert>

                              {upstreamRefreshTwoFactorChallenge &&
                                renderUpstreamTwoFactorChallenge(
                                  'refresh',
                                  upstreamRefreshTwoFactorChallenge,
                                  upstreamRefreshTwoFactorCode,
                                  setUpstreamRefreshTwoFactorCode,
                                  upstreamRefreshTwoFactorRemaining,
                                  isUpstreamRefreshTwoFactorExpired
                                )}

                              {upstreamRefreshSnapshot?.warnings?.length ? (
                                <Alert>
                                  <AlertCircle aria-hidden='true' />
                                  <AlertDescription>
                                    {upstreamRefreshSnapshot.warnings.join(
                                      '；'
                                    )}
                                  </AlertDescription>
                                </Alert>
                              ) : null}

                              {upstreamRefreshSnapshot &&
                                renderUpstreamPreviewExpiryNotice(
                                  upstreamRefreshPreviewRemaining,
                                  isUpstreamRefreshPreviewExpired
                                )}

                              {upstreamRefreshSnapshot &&
                                renderUpstreamSnapshotReview(
                                  upstreamRefreshSnapshot
                                )}
                            </div>
                          </CollapsibleContent>
                        </Collapsible>
                      )}

                      {isEditingUnsupportedUnsyncedUpstreamType && (
                        <Alert>
                          <AlertCircle aria-hidden='true' />
                          <AlertTitle>{t('Upstream Account Sync')}</AlertTitle>
                          <AlertDescription>
                            {t(
                              'new-api and sub2api channel types must use upstream account sync. Create a synced channel or refresh an existing synced channel.'
                            )}
                          </AlertDescription>
                        </Alert>
                      )}

                      {showManualCredentialSection && (
                        <div className='border-border/60 bg-muted/10 rounded-lg border p-4'>
                          <div className='flex flex-col gap-4'>
                            {/* Azure 类型的 endpoint 和 API version 配置。 */}
                            {currentType === 3 && !isGlobalAccountPoolMode && (
                              <>
                                <FormField
                                  control={form.control}
                                  name='base_url'
                                  render={({ field }) => (
                                    <FormItem>
                                      <FormLabel>
                                        {t('AZURE_OPENAI_ENDPOINT *')}
                                      </FormLabel>
                                      <FormControl>
                                        <Input
                                          placeholder={t(
                                            'e.g., https://docs-test-001.openai.azure.com'
                                          )}
                                          disabled={!canEditSensitiveFields}
                                          {...field}
                                        />
                                      </FormControl>
                                      <FormDescription>
                                        {t('Your Azure OpenAI endpoint URL')}
                                      </FormDescription>
                                      <FormMessage />
                                    </FormItem>
                                  )}
                                />
                                <FormField
                                  control={form.control}
                                  name='other'
                                  render={({ field }) => (
                                    <FormItem>
                                      <FormLabel>
                                        {t('Default API Version *')}
                                      </FormLabel>
                                      <FormControl>
                                        <Input
                                          placeholder={t(
                                            'e.g., 2025-04-01-preview'
                                          )}
                                          disabled={!canEditSensitiveFields}
                                          {...field}
                                        />
                                      </FormControl>
                                      <FormDescription>
                                        {t(
                                          'Default API version for this channel'
                                        )}
                                      </FormDescription>
                                      <FormMessage />
                                    </FormItem>
                                  )}
                                />
                                <FormField
                                  control={form.control}
                                  name='azure_responses_version'
                                  render={({ field }) => (
                                    <FormItem>
                                      <FormLabel>
                                        {t('Responses API Version')}
                                      </FormLabel>
                                      <FormControl>
                                        <Input
                                          placeholder={t('e.g., preview')}
                                          disabled={!canEditSensitiveFields}
                                          {...field}
                                        />
                                      </FormControl>
                                      <FormDescription>
                                        {t(
                                          'Default Responses API version, if empty, will use the API version above'
                                        )}
                                      </FormDescription>
                                      <FormMessage />
                                    </FormItem>
                                  )}
                                />
                              </>
                            )}

                            {/* 自定义完整 URL 渠道。 */}
                            {currentType === 8 && !isGlobalAccountPoolMode && (
                              <FormField
                                control={form.control}
                                name='base_url'
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>
                                      {t('Full Base URL (supports')} {'{'}
                                      {t('model')}
                                      {'}'} {t('variable) *')}
                                    </FormLabel>
                                    <FormControl>
                                      <Input
                                        placeholder={t(
                                          'e.g., https://api.openai.com/v1/chat/completions'
                                        )}
                                        disabled={!canEditSensitiveFields}
                                        {...field}
                                      />
                                    </FormControl>
                                    <FormDescription>
                                      {t('Enter the complete URL, supports')}{' '}
                                      {'{'}
                                      {t('model')}
                                      {'}'} {t('variable')}
                                    </FormDescription>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                            )}

                            {/* 讯飞星火模型版本配置。 */}
                            {currentType === 18 && (
                              <FormField
                                control={form.control}
                                name='other'
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>
                                      {t('Model Version *')}
                                    </FormLabel>
                                    <FormControl>
                                      <Input
                                        placeholder={t('e.g., v2.1')}
                                        disabled={!canEditSensitiveFields}
                                        {...field}
                                      />
                                    </FormControl>
                                    <FormDescription>
                                      {t(
                                        'Spark model version, e.g., v2.1 (version number in API URL)'
                                      )}
                                    </FormDescription>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                            )}

                            {/* OpenRouter 企业账户配置。 */}
                            {currentType === 20 && (
                              <FormField
                                control={form.control}
                                name='is_enterprise_account'
                                render={({ field }) => (
                                  <FormItem className='flex items-center justify-between'>
                                    <div className='space-y-0.5'>
                                      <FormLabel>
                                        {t('Enterprise Account')}
                                      </FormLabel>
                                      <FormDescription>
                                        {t(
                                          'Enable if this is an OpenRouter enterprise account with special response format'
                                        )}
                                      </FormDescription>
                                    </div>
                                    <FormControl>
                                      <Switch
                                        checked={field.value}
                                        disabled={!canEditSensitiveFields}
                                        onCheckedChange={field.onChange}
                                      />
                                    </FormControl>
                                  </FormItem>
                                )}
                              />
                            )}

                            {/* AWS 凭证格式配置；账号池组模式下由组内账号提供，不在渠道表单展示。 */}
                            {currentType === 33 && !isGlobalAccountPoolMode && (
                              <FormField
                                control={form.control}
                                name='aws_key_type'
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>{t('AWS Key Format')}</FormLabel>
                                    <Select
                                      items={[
                                        {
                                          value: 'ak_sk',
                                          label: t(
                                            'AccessKey / SecretAccessKey'
                                          ),
                                        },
                                        {
                                          value: 'api_key',
                                          label: t('API Key'),
                                        },
                                      ]}
                                      onValueChange={(value) => {
                                        if (!canEditSensitiveFields) {
                                          toast.error(noPermissionMessage)
                                          return
                                        }
                                        field.onChange(value)
                                      }}
                                      value={field.value}
                                    >
                                      <FormControl>
                                        <SelectTrigger
                                          disabled={!canEditSensitiveFields}
                                        >
                                          <SelectValue
                                            placeholder={t('Select key format')}
                                          />
                                        </SelectTrigger>
                                      </FormControl>
                                      <SelectContent
                                        alignItemWithTrigger={false}
                                      >
                                        <SelectGroup>
                                          <SelectItem value='ak_sk'>
                                            {t('AccessKey / SecretAccessKey')}
                                          </SelectItem>
                                          <SelectItem value='api_key'>
                                            {t('API Key')}
                                          </SelectItem>
                                        </SelectGroup>
                                      </SelectContent>
                                    </Select>
                                    <FormDescription>
                                      {field.value === 'api_key'
                                        ? t('API Key mode: use APIKey|Region')
                                        : t(
                                            'AK/SK mode: use AccessKey|SecretAccessKey|Region'
                                          )}
                                    </FormDescription>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                            )}

                            {/* AI Proxy Library 知识库 ID。 */}
                            {currentType === 21 && (
                              <FormField
                                control={form.control}
                                name='other'
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>
                                      {t('Knowledge Base ID *')}
                                    </FormLabel>
                                    <FormControl>
                                      <Input
                                        placeholder={t('e.g., 123456')}
                                        disabled={!canEditSensitiveFields}
                                        {...field}
                                      />
                                    </FormControl>
                                    <FormDescription>
                                      {t('Enter the knowledge base ID')}
                                    </FormDescription>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                            )}

                            {/* FastGPT 私有部署地址。 */}
                            {currentType === 22 && !isGlobalAccountPoolMode && (
                              <FormField
                                control={form.control}
                                name='base_url'
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>
                                      {t('Private Deployment URL')}
                                    </FormLabel>
                                    <FormControl>
                                      <Input
                                        placeholder={t(
                                          'e.g., https://fastgpt.run/api/openapi'
                                        )}
                                        disabled={!canEditSensitiveFields}
                                        {...field}
                                      />
                                    </FormControl>
                                    <FormDescription>
                                      {t(
                                        'For private deployments, format: https://fastgpt.run/api/openapi'
                                      )}
                                    </FormDescription>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                            )}

                            {/* SunoAPI 专用基础地址。 */}
                            {currentType === 36 && !isGlobalAccountPoolMode && (
                              <FormField
                                control={form.control}
                                name='base_url'
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>
                                      {t(
                                        'API Base URL (Important: Not Chat API) *'
                                      )}
                                    </FormLabel>
                                    <FormControl>
                                      <Input
                                        placeholder={t(
                                          'e.g., https://api.example.com (path before /suno)'
                                        )}
                                        disabled={!canEditSensitiveFields}
                                        {...field}
                                      />
                                    </FormControl>
                                    <FormDescription>
                                      {t(
                                        'Enter the path before /suno, usually just the domain'
                                      )}
                                    </FormDescription>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                            )}

                            {/* Cloudflare Workers AI Account ID。 */}
                            {currentType === 39 && (
                              <FormField
                                control={form.control}
                                name='other'
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>{t('Account ID *')}</FormLabel>
                                    <FormControl>
                                      <Input
                                        placeholder={t(
                                          'e.g., d6b5da8hk1awo8nap34ube6gh'
                                        )}
                                        disabled={!canEditSensitiveFields}
                                        {...field}
                                      />
                                    </FormControl>
                                    <FormDescription>
                                      {t('Your Cloudflare Account ID')}
                                    </FormDescription>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                            )}

                            {/* SiliconFlow 推荐链接提示。 */}
                            {currentType === 40 && (
                              <Alert>
                                <AlertDescription>
                                  {t('Referral link:')}{' '}
                                  <a
                                    href='https://cloud.siliconflow.cn/i/hij0YNTZ'
                                    target='_blank'
                                    rel='noopener noreferrer'
                                    className='text-primary underline'
                                  >
                                    {t(
                                      'https://cloud.siliconflow.cn/i/hij0YNTZ'
                                    )}
                                  </a>
                                </AlertDescription>
                              </Alert>
                            )}

                            {/* Vertex AI 凭证和部署地区配置；账号池组模式下由组内账号提供。 */}
                            {currentType === 41 && !isGlobalAccountPoolMode && (
                              <>
                                <FormField
                                  control={form.control}
                                  name='vertex_key_type'
                                  render={({ field }) => (
                                    <FormItem>
                                      <FormLabel>
                                        {t('Vertex AI Key Format')}
                                      </FormLabel>
                                      <Select
                                        items={[
                                          { value: 'json', label: t('JSON') },
                                          {
                                            value: 'api_key',
                                            label: t('API Key'),
                                          },
                                        ]}
                                        onValueChange={(value) => {
                                          if (!canEditSensitiveFields) {
                                            toast.error(noPermissionMessage)
                                            return
                                          }
                                          field.onChange(value)
                                        }}
                                        value={field.value}
                                      >
                                        <FormControl>
                                          <SelectTrigger
                                            disabled={!canEditSensitiveFields}
                                          >
                                            <SelectValue />
                                          </SelectTrigger>
                                        </FormControl>
                                        <SelectContent
                                          alignItemWithTrigger={false}
                                        >
                                          <SelectGroup>
                                            <SelectItem value='json'>
                                              {t('JSON')}
                                            </SelectItem>
                                            <SelectItem value='api_key'>
                                              {t('API Key')}
                                            </SelectItem>
                                          </SelectGroup>
                                        </SelectContent>
                                      </Select>
                                      <FormDescription>
                                        {field.value === 'json'
                                          ? t(
                                              'JSON format supports service account JSON files'
                                            )
                                          : t(
                                              'API Key mode (does not support batch creation)'
                                            )}
                                      </FormDescription>
                                      <FormMessage />
                                    </FormItem>
                                  )}
                                />
                                {form.watch('vertex_key_type') === 'json' && (
                                  <FormItem>
                                    <FormLabel>
                                      {t('Service account JSON file(s)')}
                                    </FormLabel>
                                    <FormControl>
                                      <Input
                                        type='file'
                                        accept='.json,application/json'
                                        multiple={isBatchMode}
                                        disabled={!canEditSensitiveFields}
                                        onChange={async (e) => {
                                          if (!canEditSensitiveFields) {
                                            toast.error(noPermissionMessage)
                                            return
                                          }
                                          const fileList = e.target.files
                                          const files = fileList
                                            ? Array.from(fileList)
                                            : []
                                          // 清空 input value，允许管理员重新选择同一个文件并触发 change。
                                          e.target.value = ''

                                          if (files.length === 0) {
                                            toast.info(
                                              t('Please upload key file(s)')
                                            )
                                            return
                                          }

                                          const keys: unknown[] = []
                                          for (const file of files) {
                                            try {
                                              const txt = await file.text()
                                              keys.push(JSON.parse(txt))
                                            } catch {
                                              toast.error(
                                                t(
                                                  'Failed to parse JSON file: {{name}}',
                                                  {
                                                    name: file.name,
                                                  }
                                                )
                                              )
                                              return
                                            }
                                          }

                                          if (keys.length === 0) {
                                            toast.info(
                                              t('Please upload key file(s)')
                                            )
                                            return
                                          }

                                          const keyValue = isBatchMode
                                            ? JSON.stringify(keys)
                                            : JSON.stringify(keys[0])

                                          form.setValue('key', keyValue, {
                                            shouldDirty: true,
                                            shouldValidate: true,
                                          })

                                          toast.success(
                                            t(
                                              'Parsed {{count}} service account file(s)',
                                              {
                                                count: keys.length,
                                              }
                                            )
                                          )
                                        }}
                                      />
                                    </FormControl>
                                    <FormDescription>
                                      {isBatchMode
                                        ? t(
                                            'Upload multiple JSON files in batch modes'
                                          )
                                        : t(
                                            'Upload a single service account JSON file'
                                          )}
                                    </FormDescription>
                                    <FormMessage />
                                  </FormItem>
                                )}
                                <FormField
                                  control={form.control}
                                  name='other'
                                  render={({ field }) => (
                                    <FormItem>
                                      <FormLabel>
                                        {t('Deployment Region *')}
                                      </FormLabel>
                                      <FormControl>
                                        <Textarea
                                          placeholder={t(
                                            'e.g., us-central1 or JSON format for model-specific regions'
                                          )}
                                          rows={3}
                                          disabled={!canEditSensitiveFields}
                                          {...field}
                                        />
                                      </FormControl>
                                      <FormDescription>
                                        {t(
                                          'Enter deployment region or JSON mapping:'
                                        )}{' '}
                                        {'{'}
                                        {t(
                                          '"default": "us-central1", "claude-3-5-sonnet-20240620": "europe-west1"'
                                        )}
                                        {'}'}
                                      </FormDescription>
                                      <FormMessage />
                                    </FormItem>
                                  )}
                                />
                              </>
                            )}

                            {/* 火山引擎内置区域地址选择。 */}
                            {currentType === 45 &&
                              !doubaoApiEditUnlocked &&
                              !isGlobalAccountPoolMode && (
                                <FormField
                                  control={form.control}
                                  name='base_url'
                                  render={({ field }) => (
                                    <FormItem>
                                      <FormLabel
                                        className='cursor-pointer select-none'
                                        onClick={handleApiConfigSecretClick}
                                      >
                                        {t('API Base URL *')}
                                      </FormLabel>
                                      <Select
                                        items={[
                                          {
                                            value:
                                              'https://ark.cn-beijing.volces.com',
                                            label: t(
                                              'https://ark.cn-beijing.volces.com'
                                            ),
                                          },
                                          {
                                            value:
                                              'https://ark.ap-southeast.bytepluses.com',
                                            label: t(
                                              'https://ark.ap-southeast.bytepluses.com'
                                            ),
                                          },
                                        ]}
                                        onValueChange={(value) => {
                                          if (!canEditSensitiveFields) {
                                            toast.error(noPermissionMessage)
                                            return
                                          }
                                          field.onChange(value)
                                        }}
                                        value={
                                          field.value === 'doubao-coding-plan'
                                            ? 'https://ark.cn-beijing.volces.com'
                                            : field.value ||
                                              'https://ark.cn-beijing.volces.com'
                                        }
                                      >
                                        <FormControl>
                                          <SelectTrigger
                                            disabled={!canEditSensitiveFields}
                                          >
                                            <SelectValue />
                                          </SelectTrigger>
                                        </FormControl>
                                        <SelectContent
                                          alignItemWithTrigger={false}
                                        >
                                          <SelectGroup>
                                            <SelectItem value='https://ark.cn-beijing.volces.com'>
                                              {t(
                                                'https://ark.cn-beijing.volces.com'
                                              )}
                                            </SelectItem>
                                            <SelectItem value='https://ark.ap-southeast.bytepluses.com'>
                                              {t(
                                                'https://ark.ap-southeast.bytepluses.com'
                                              )}
                                            </SelectItem>
                                          </SelectGroup>
                                        </SelectContent>
                                      </Select>
                                      <FormDescription>
                                        {t('Select the API endpoint region')}
                                      </FormDescription>
                                      <FormMessage />
                                    </FormItem>
                                  )}
                                />
                              )}

                            {/* 火山引擎自定义 API URL，仅在隐藏开关解锁后展示。 */}
                            {currentType === 45 &&
                              doubaoApiEditUnlocked &&
                              !isGlobalAccountPoolMode && (
                                <FormField
                                  control={form.control}
                                  name='base_url'
                                  render={({ field }) => (
                                    <FormItem>
                                      <FormLabel>
                                        {t('API Base URL *')}
                                      </FormLabel>
                                      <FormControl>
                                        <Input
                                          placeholder={t(
                                            'e.g., https://ark.cn-beijing.volces.com'
                                          )}
                                          disabled={!canEditSensitiveFields}
                                          {...field}
                                        />
                                      </FormControl>
                                      <FormDescription>
                                        {t('Enter custom API endpoint URL')}
                                      </FormDescription>
                                      <FormMessage />
                                    </FormItem>
                                  )}
                                />
                              )}

                            {/* Coze 智能体 ID。 */}
                            {currentType === 49 && (
                              <FormField
                                control={form.control}
                                name='other'
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>{t('Agent ID *')}</FormLabel>
                                    <FormControl>
                                      <Input
                                        placeholder={t('e.g., 7342866812345')}
                                        disabled={!canEditSensitiveFields}
                                        {...field}
                                      />
                                    </FormControl>
                                    <FormDescription>
                                      {t('Enter the Coze agent ID')}
                                    </FormDescription>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                            )}

                            {/* 其他渠道类型的通用 base_url 配置。 */}
                            {![3, 8, 22, 36, 45].includes(currentType) &&
                              !isGlobalAccountPoolMode && (
                                <FormField
                                  control={form.control}
                                  name='base_url'
                                  render={({ field }) => (
                                    <FormItem>
                                      <FormLabel>{t('Base URL')}</FormLabel>
                                      <FormControl>
                                        <Input
                                          placeholder={t(
                                            FIELD_PLACEHOLDERS.BASE_URL
                                          )}
                                          disabled={!canEditSensitiveFields}
                                          {...field}
                                        />
                                      </FormControl>
                                      <FormDescription>
                                        {t(
                                          'Custom API base URL. For official channels, NexusTok has built-in addresses. Only fill this for third-party proxy sites or special endpoints. Do not add /v1 or trailing slash.'
                                        )}
                                      </FormDescription>
                                      <FormMessage />
                                    </FormItem>
                                  )}
                                />
                              )}

                            {currentType === CHANNEL_TYPE_ADVANCED_CUSTOM && (
                              <FormField
                                control={form.control}
                                name='advanced_custom'
                                render={({ field }) => (
                                  <FormItem className='border-border/60 rounded-lg border p-4'>
                                    <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
                                      <div className='min-w-0 space-y-2'>
                                        <FormLabel>
                                          {t('Advanced Custom Routes')}
                                        </FormLabel>
                                        <FormDescription>
                                          {t(
                                            'Configure incoming paths, upstream paths, converters, and authentication for this Advanced Custom channel.'
                                          )}
                                        </FormDescription>
                                        <div className='flex flex-wrap gap-2'>
                                          <Badge variant='secondary'>
                                            {t('Routes')}:{' '}
                                            {advancedCustomStats.routeCount}
                                          </Badge>
                                          {advancedCustomRouteTypeLabels.map(
                                            (label) => (
                                              <Badge
                                                key={label}
                                                variant='outline'
                                                className='max-w-[12rem]'
                                                title={label}
                                              >
                                                <span className='truncate'>
                                                  {t(label)}
                                                </span>
                                              </Badge>
                                            )
                                          )}
                                          {hiddenAdvancedCustomRouteTypeCount >
                                            0 && (
                                            <Badge
                                              variant='outline'
                                              title={
                                                advancedCustomRouteTypeTitle
                                              }
                                            >
                                              +
                                              {
                                                hiddenAdvancedCustomRouteTypeCount
                                              }
                                            </Badge>
                                          )}
                                          {!advancedCustomStats.valid && (
                                            <Badge variant='destructive'>
                                              {t('Incomplete')}
                                            </Badge>
                                          )}
                                        </div>
                                      </div>
                                      <Button
                                        type='button'
                                        variant='outline'
                                        size='sm'
                                        onClick={() => {
                                          if (!canEditSensitiveFields) {
                                            toast.error(noPermissionMessage)
                                            return
                                          }
                                          setAdvancedCustomEditorOpen(true)
                                        }}
                                        disabled={!canEditSensitiveFields}
                                        title={
                                          canEditSensitiveFields
                                            ? undefined
                                            : noPermissionMessage
                                        }
                                      >
                                        <Route data-icon='inline-start' />
                                        {t('Configure routes')}
                                      </Button>
                                    </div>
                                    <FormControl>
                                      <input type='hidden' {...field} />
                                    </FormControl>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                            )}

                            <ChannelAuthSection>
                              <FormField
                                control={form.control}
                                name='credential_mode'
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>
                                      {t('Credential Mode')}
                                    </FormLabel>
                                    <Select
                                      items={credentialModeOptions}
                                      onValueChange={(value) => {
                                        if (!canEditSensitiveFields) {
                                          toast.error(noPermissionMessage)
                                          return
                                        }
                                        field.onChange(value)
                                        if (value === 'multi_key') {
                                          form.setValue(
                                            'multi_key_mode',
                                            'multi_to_single'
                                          )
                                        } else if (
                                          value === 'account_pool' ||
                                          value === 'global_account_pool'
                                        ) {
                                          form.setValue(
                                            'multi_key_mode',
                                            'single'
                                          )
                                          if (value === 'global_account_pool') {
                                            form.setValue(
                                              'account_pool_fallback',
                                              false
                                            )
                                            form.setValue('base_url', '')
                                            form.setValue('key', '')
                                          }
                                        } else if (
                                          form.getValues('multi_key_mode') ===
                                          'multi_to_single'
                                        ) {
                                          form.setValue(
                                            'multi_key_mode',
                                            'single'
                                          )
                                        }
                                      }}
                                      value={field.value}
                                    >
                                      <FormControl>
                                        <SelectTrigger
                                          disabled={!canEditSensitiveFields}
                                        >
                                          <SelectValue />
                                        </SelectTrigger>
                                      </FormControl>
                                      <SelectContent
                                        alignItemWithTrigger={false}
                                      >
                                        <SelectGroup>
                                          {credentialModeOptions.map(
                                            (option) => (
                                              <SelectItem
                                                key={option.value}
                                                value={option.value}
                                              >
                                                {option.label}
                                              </SelectItem>
                                            )
                                          )}
                                        </SelectGroup>
                                      </SelectContent>
                                    </Select>
                                    <FormDescription>
                                      {credentialModeDescription}
                                    </FormDescription>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                              {credentialMode === 'account_pool' && (
                                <FormField
                                  control={form.control}
                                  name='account_pool_mode'
                                  render={({ field }) => (
                                    <FormItem>
                                      <FormLabel>
                                        {t('Account Pool Strategy')}
                                      </FormLabel>
                                      <Select
                                        items={[
                                          {
                                            value: 'polling',
                                            label: t('Polling'),
                                          },
                                          {
                                            value: 'random',
                                            label: t('Random'),
                                          },
                                        ]}
                                        onValueChange={(value) => {
                                          if (!canEditSensitiveFields) {
                                            toast.error(noPermissionMessage)
                                            return
                                          }
                                          field.onChange(value)
                                        }}
                                        value={field.value}
                                      >
                                        <FormControl>
                                          <SelectTrigger
                                            disabled={!canEditSensitiveFields}
                                          >
                                            <SelectValue />
                                          </SelectTrigger>
                                        </FormControl>
                                        <SelectContent
                                          alignItemWithTrigger={false}
                                        >
                                          <SelectGroup>
                                            <SelectItem value='polling'>
                                              {t('Polling')}
                                            </SelectItem>
                                            <SelectItem value='random'>
                                              {t('Random')}
                                            </SelectItem>
                                          </SelectGroup>
                                        </SelectContent>
                                      </Select>
                                      <FormDescription>
                                        {t(
                                          'Highest priority wins; accounts with the same priority rotate by weight.'
                                        )}
                                      </FormDescription>
                                      <FormMessage />
                                    </FormItem>
                                  )}
                                />
                              )}
                              {credentialMode === 'global_account_pool' && (
                                <FormField
                                  control={form.control}
                                  name='account_pool_group_id'
                                  render={({ field }) => (
                                    <FormItem>
                                      <FormLabel>
                                        {t('Account Pool Group')}
                                      </FormLabel>
                                      <Select
                                        items={accountPoolGroupOptions}
                                        onValueChange={(value) => {
                                          if (!canEditSensitiveFields) {
                                            toast.error(noPermissionMessage)
                                            return
                                          }
                                          field.onChange(Number(value))
                                        }}
                                        value={
                                          field.value ? String(field.value) : ''
                                        }
                                      >
                                        <FormControl>
                                          <SelectTrigger
                                            disabled={!canEditSensitiveFields}
                                          >
                                            <SelectValue
                                              placeholder={t(
                                                'Select account group'
                                              )}
                                            />
                                          </SelectTrigger>
                                        </FormControl>
                                        <SelectContent
                                          alignItemWithTrigger={false}
                                        >
                                          <SelectGroup>
                                            {accountPoolGroupOptions.map(
                                              (option) => (
                                                <SelectItem
                                                  key={option.value}
                                                  value={option.value}
                                                >
                                                  {option.label}
                                                </SelectItem>
                                              )
                                            )}
                                          </SelectGroup>
                                        </SelectContent>
                                      </Select>
                                      <FormDescription>
                                        {accountPoolGroupId
                                          ? t(
                                              'Channels reference this group at relay time.'
                                            )
                                          : t(
                                              'Create account groups in Admin Account Pool.'
                                            )}
                                      </FormDescription>
                                      <FormMessage />
                                    </FormItem>
                                  )}
                                />
                              )}
                              {credentialMode === 'account_pool' && (
                                <FormField
                                  control={form.control}
                                  name='account_pool_fallback'
                                  render={({ field }) => (
                                    <FormItem
                                      className={sideDrawerSwitchItemClassName()}
                                    >
                                      <div className='flex flex-col gap-0.5'>
                                        <FormLabel>
                                          {t('Fallback to Channel Key')}
                                        </FormLabel>
                                        <FormDescription className='text-xs'>
                                          {t(
                                            'Use the channel key or multi-key list only when no account is available.'
                                          )}
                                        </FormDescription>
                                      </div>
                                      <FormControl>
                                        <Switch
                                          checked={field.value === true}
                                          disabled={!canEditSensitiveFields}
                                          onCheckedChange={field.onChange}
                                        />
                                      </FormControl>
                                    </FormItem>
                                  )}
                                />
                              )}
                              {!isEditing &&
                                credentialMode === 'single_key' && (
                                  <FormField
                                    control={form.control}
                                    name='multi_key_mode'
                                    render={({ field }) => (
                                      <FormItem>
                                        <FormLabel>{t('Add Mode')}</FormLabel>
                                        <Select
                                          items={addModeOptions.map(
                                            (option) => ({
                                              value: option.value,
                                              label: t(option.label),
                                            })
                                          )}
                                          onValueChange={(value) => {
                                            if (!canEditSensitiveFields) {
                                              toast.error(noPermissionMessage)
                                              return
                                            }
                                            field.onChange(value)
                                          }}
                                          value={field.value}
                                        >
                                          <FormControl>
                                            <SelectTrigger
                                              disabled={!canEditSensitiveFields}
                                            >
                                              <SelectValue />
                                            </SelectTrigger>
                                          </FormControl>
                                          <SelectContent
                                            alignItemWithTrigger={false}
                                          >
                                            <SelectGroup>
                                              {addModeOptions.map((option) => (
                                                <SelectItem
                                                  key={option.value}
                                                  value={option.value}
                                                >
                                                  {t(option.label)}
                                                </SelectItem>
                                              ))}
                                            </SelectGroup>
                                          </SelectContent>
                                        </Select>
                                        <FormDescription>
                                          {t(FIELD_DESCRIPTIONS.BATCH_ADD)}
                                        </FormDescription>
                                        <FormMessage />
                                      </FormItem>
                                    )}
                                  />
                                )}

                              {!isGlobalAccountPoolMode && (
                                <FormField
                                  control={form.control}
                                  name='key'
                                  render={({ field }) => {
                                    const keyPlaceholder = (() => {
                                      if (isEditing) {
                                        return t(
                                          'Leave empty to keep existing key'
                                        )
                                      }
                                      if (currentType === 33) {
                                        if (awsKeyType === 'api_key') {
                                          return isBatchMode
                                            ? t(
                                                'Enter API Key, one per line, format: APIKey|Region'
                                              )
                                            : t(
                                                'Enter API Key, format: APIKey|Region'
                                              )
                                        }
                                        return isBatchMode
                                          ? t(
                                              'Enter key, one per line, format: AccessKey|SecretAccessKey|Region'
                                            )
                                          : t(
                                              'Enter key, format: AccessKey|SecretAccessKey|Region'
                                            )
                                      }
                                      if (isBatchMode) {
                                        return t(
                                          'Enter one key per line for batch creation'
                                        )
                                      }
                                      return t(getKeyPromptForType(currentType))
                                    })()
                                    return (
                                      <FormItem>
                                        <FormLabel>{t('API Key *')}</FormLabel>
                                        <FormControl>
                                          <Textarea
                                            placeholder={keyPlaceholder}
                                            rows={isBatchMode ? 8 : 4}
                                            disabled={!canEditSensitiveFields}
                                            {...field}
                                          />
                                        </FormControl>
                                        <FormDescription>
                                          <span className='flex flex-col gap-2'>
                                            <span>
                                              {isEditing ? (
                                                <>
                                                  {t(
                                                    'Enter new key to update, or leave empty to keep current key'
                                                  )}
                                                  {isMultiKeyChannel && (
                                                    <span className='text-warning mt-1 block'>
                                                      {t(
                                                        'Multi-key channel: Keys will be'
                                                      )}{' '}
                                                      {keyMode === 'replace'
                                                        ? t('replaced')
                                                        : t('appended')}
                                                    </span>
                                                  )}
                                                </>
                                              ) : isBatchMode ? (
                                                t(
                                                  'Enter one API key per line for batch creation'
                                                )
                                              ) : (
                                                t(FIELD_DESCRIPTIONS.KEY)
                                              )}
                                            </span>
                                            {isBatchMode && (
                                              <Button
                                                type='button'
                                                variant='outline'
                                                size='sm'
                                                onClick={handleDeduplicateKeys}
                                                disabled={
                                                  !canEditSensitiveFields
                                                }
                                                title={
                                                  canEditSensitiveFields
                                                    ? undefined
                                                    : noPermissionMessage
                                                }
                                                className='w-fit'
                                              >
                                                <Trash2 className='mr-2 h-4 w-4' />
                                                {t('Remove Duplicates')}
                                              </Button>
                                            )}
                                          </span>
                                        </FormDescription>
                                        {isEditing && (
                                          <div className='mt-4 space-y-3 rounded-lg border border-dashed p-4'>
                                            <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                                              <div>
                                                <p className='text-sm font-medium'>
                                                  {t('Current key')}
                                                </p>
                                                <p className='text-muted-foreground text-xs'>
                                                  {t(
                                                    'Verification required to reveal the saved key.'
                                                  )}
                                                </p>
                                              </div>
                                              <div className='flex items-center gap-2'>
                                                <Button
                                                  type='button'
                                                  variant='outline'
                                                  size='sm'
                                                  onClick={handleRevealKey}
                                                  disabled={
                                                    !permissions.canViewSecret ||
                                                    isChannelKeyLoading ||
                                                    verificationState.loading
                                                  }
                                                  title={
                                                    permissions.canViewSecret
                                                      ? undefined
                                                      : noPermissionMessage
                                                  }
                                                >
                                                  {isChannelKeyLoading ||
                                                  verificationState.loading ? (
                                                    <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                                                  ) : (
                                                    <Eye className='mr-2 h-4 w-4' />
                                                  )}
                                                  {t('Reveal key')}
                                                </Button>
                                                <Button
                                                  type='button'
                                                  variant='ghost'
                                                  size='sm'
                                                  onClick={async () => {
                                                    if (channelKey) {
                                                      await copyToClipboard(
                                                        channelKey
                                                      )
                                                    }
                                                  }}
                                                  disabled={!channelKey}
                                                >
                                                  <Copy className='mr-2 h-4 w-4' />
                                                  {t('Copy')}
                                                </Button>
                                              </div>
                                            </div>
                                            <Input
                                              readOnly
                                              value={channelKey ?? ''}
                                              placeholder={t(
                                                'Hidden — verify to reveal'
                                              )}
                                              className='font-mono'
                                            />
                                          </div>
                                        )}
                                        <FormMessage />
                                      </FormItem>
                                    )
                                  }}
                                />
                              )}

                              {currentType === 57 &&
                                credentialMode !== 'global_account_pool' && (
                                  <div className='bg-muted/20 space-y-3 rounded-lg border p-4'>
                                    <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                                      <div className='space-y-0.5'>
                                        <div className='text-sm font-semibold'>
                                          {t('Codex Authorization')}
                                        </div>
                                        <div className='text-muted-foreground text-xs'>
                                          {t(
                                            'Codex channels use an OAuth JSON credential as the key.'
                                          )}
                                        </div>
                                      </div>
                                      <div className='flex flex-wrap items-center gap-2'>
                                        <Button
                                          type='button'
                                          variant='outline'
                                          size='sm'
                                          onClick={() =>
                                            setCodexOAuthDialogOpen(true)
                                          }
                                          disabled={!canEditSensitiveFields}
                                          title={
                                            canEditSensitiveFields
                                              ? undefined
                                              : noPermissionMessage
                                          }
                                        >
                                          <Link2 className='mr-2 h-4 w-4' />
                                          {t('Authorize')}
                                        </Button>
                                        {isEditing && channelId && (
                                          <Button
                                            type='button'
                                            variant='outline'
                                            size='sm'
                                            onClick={
                                              handleRefreshCodexCredential
                                            }
                                            disabled={
                                              !canEditSensitiveFields ||
                                              isCodexCredentialRefreshing
                                            }
                                            title={
                                              canEditSensitiveFields
                                                ? undefined
                                                : noPermissionMessage
                                            }
                                          >
                                            {isCodexCredentialRefreshing ? (
                                              <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                                            ) : (
                                              <RefreshCw className='mr-2 h-4 w-4' />
                                            )}
                                            {isCodexCredentialRefreshing
                                              ? t('Refreshing...')
                                              : t('Refresh credential')}
                                          </Button>
                                        )}
                                      </div>
                                    </div>
                                    <Alert>
                                      <AlertDescription>
                                        {t(
                                          'If authorization succeeds, the generated JSON will be inserted into the key field. You still need to save the channel to persist it.'
                                        )}
                                      </AlertDescription>
                                    </Alert>
                                  </div>
                                )}

                              <CodexOAuthDialog
                                open={codexOAuthDialogOpen}
                                onOpenChange={setCodexOAuthDialogOpen}
                                onKeyGenerated={(key) => {
                                  if (!canEditSensitiveFields) {
                                    toast.error(noPermissionMessage)
                                    return
                                  }
                                  form.setValue('key', key, {
                                    shouldDirty: true,
                                  })
                                }}
                              />

                              {isEditing && isMultiKeyChannel && (
                                <FormField
                                  control={form.control}
                                  name='key_mode'
                                  render={({ field }) => (
                                    <FormItem>
                                      <FormLabel>
                                        {t('Key Update Mode')}
                                      </FormLabel>
                                      <Select
                                        items={[
                                          {
                                            value: 'append',
                                            label: t('Append to existing keys'),
                                          },
                                          {
                                            value: 'replace',
                                            label: t(
                                              'Replace all existing keys'
                                            ),
                                          },
                                        ]}
                                        onValueChange={(value) => {
                                          if (!canEditSensitiveFields) {
                                            toast.error(noPermissionMessage)
                                            return
                                          }
                                          field.onChange(value)
                                        }}
                                        value={field.value}
                                      >
                                        <FormControl>
                                          <SelectTrigger
                                            disabled={!canEditSensitiveFields}
                                          >
                                            <SelectValue />
                                          </SelectTrigger>
                                        </FormControl>
                                        <SelectContent
                                          alignItemWithTrigger={false}
                                        >
                                          <SelectGroup>
                                            <SelectItem value='append'>
                                              {t('Append to existing keys')}
                                            </SelectItem>
                                            <SelectItem value='replace'>
                                              {t('Replace all existing keys')}
                                            </SelectItem>
                                          </SelectGroup>
                                        </SelectContent>
                                      </Select>
                                      <FormDescription>
                                        {field.value === 'replace'
                                          ? t(
                                              'Replace mode: Will completely replace all existing keys'
                                            )
                                          : t(
                                              'Append mode: New keys will be added to the end of the existing key list'
                                            )}
                                      </FormDescription>
                                      <FormMessage />
                                    </FormItem>
                                  )}
                                />
                              )}

                              {!isEditing &&
                                multiKeyMode === 'multi_to_single' && (
                                  <FormField
                                    control={form.control}
                                    name='multi_key_type'
                                    render={({ field }) => (
                                      <FormItem>
                                        <FormLabel>
                                          {t('Multi-Key Strategy')}
                                        </FormLabel>
                                        <Select
                                          items={[
                                            {
                                              value: 'random',
                                              label: t('Random'),
                                            },
                                            {
                                              value: 'polling',
                                              label: t('Polling'),
                                            },
                                          ]}
                                          onValueChange={(value) => {
                                            if (!canEditSensitiveFields) {
                                              toast.error(noPermissionMessage)
                                              return
                                            }
                                            field.onChange(value)
                                          }}
                                          value={field.value}
                                        >
                                          <FormControl>
                                            <SelectTrigger
                                              disabled={!canEditSensitiveFields}
                                            >
                                              <SelectValue />
                                            </SelectTrigger>
                                          </FormControl>
                                          <SelectContent
                                            alignItemWithTrigger={false}
                                          >
                                            <SelectGroup>
                                              <SelectItem value='random'>
                                                {t('Random')}
                                              </SelectItem>
                                              <SelectItem value='polling'>
                                                {t('Polling')}
                                              </SelectItem>
                                            </SelectGroup>
                                          </SelectContent>
                                        </Select>
                                        <FormDescription>
                                          {multiKeyType === 'polling' ? (
                                            <span className='text-warning'>
                                              {t(
                                                'Polling mode requires Redis and memory cache, otherwise performance will be significantly degraded'
                                              )}
                                            </span>
                                          ) : (
                                            t(
                                              'Randomly select a key from the pool for each request'
                                            )
                                          )}
                                        </FormDescription>
                                        <FormMessage />
                                      </FormItem>
                                    )}
                                  />
                                )}
                            </ChannelAuthSection>
                          </div>
                        </div>
                      )}
                    </ChannelApiAccessSection>
                  </div>

                  {showSharedModelsSection && (
                    <div
                      id={CHANNEL_EDITOR_SECTION_IDS.models}
                      className='scroll-mt-4'
                    >
                      <ChannelModelsSection>
                        <div className='flex flex-col gap-5'>
                          <div className='border-border/60 bg-muted/10 flex flex-col gap-4 rounded-lg border p-4'>
                            <FormField
                              control={form.control}
                              name='models'
                              render={() => (
                                <FormItem className='flex flex-col gap-3'>
                                  <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                                    <div className='flex flex-col gap-1'>
                                      <FormLabel htmlFor='channel-models'>
                                        {usesUpstreamAccountCredentialSource
                                          ? t('Aggregated Models')
                                          : t('Models *')}
                                      </FormLabel>
                                      <FormDescription>
                                        {usesUpstreamAccountCredentialSource
                                          ? t(
                                              'Derived from enabled synced keys. Edit per-key models in synced key configuration.'
                                            )
                                          : t(FIELD_DESCRIPTIONS.MODELS)}
                                      </FormDescription>
                                    </div>
                                    <div className='flex flex-wrap gap-2'>
                                      <Badge
                                        variant='outline'
                                        className='w-fit'
                                      >
                                        {t('Selected {{count}}', {
                                          count: currentModelsArray.length,
                                        })}
                                      </Badge>
                                    </div>
                                  </div>
                                  <FormControl>
                                    <MultiSelect
                                      id='channel-models'
                                      options={modelOptions}
                                      selected={currentModelsArray}
                                      onChange={handleModelsChange}
                                      placeholder={t(
                                        'Select models or add custom ones'
                                      )}
                                      allowCreate
                                      allowCreateWithMatches={false}
                                      createLabel='Add custom model "{{value}}"'
                                      maxVisibleChips={8}
                                      copyChipOnClick
                                      disabled={
                                        !canEditBasicFields ||
                                        usesUpstreamAccountCredentialSource
                                      }
                                      isLoading={modelSearchIsLoading}
                                      emptyText={t('No matching models')}
                                      loadingText={t('Searching...')}
                                      searchValue={modelSearchValue}
                                      onSearchChange={setModelSearchValue}
                                      open={modelSearchOpen}
                                      onOpenChange={setModelSearchOpen}
                                      onSearchSubmit={
                                        handleAddModelSearchResults
                                      }
                                      contentHeader={
                                        showModelSearchPanel ? (
                                          <div className='bg-background flex flex-col gap-3 rounded-md'>
                                            <div className='flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between'>
                                              <div className='flex min-w-0 flex-col gap-1'>
                                                <p className='text-sm font-medium'>
                                                  {t('Search results')}
                                                </p>
                                                <p className='text-muted-foreground text-xs'>
                                                  {modelSearchIsLoading
                                                    ? t('Searching...')
                                                    : isModelSearchError
                                                      ? t('No matching models')
                                                      : t(
                                                          '{{matched}} matched · {{addable}} new · {{existing}} already selected',
                                                          {
                                                            matched:
                                                              modelSearchSummary
                                                                .matched.length,
                                                            addable:
                                                              modelSearchSummary
                                                                .addable.length,
                                                            existing:
                                                              modelSearchSummary.existingCount,
                                                          }
                                                        )}
                                                </p>
                                              </div>
                                              <Button
                                                type='button'
                                                variant='outline'
                                                size='sm'
                                                onPointerDown={
                                                  handleAddModelSearchResultsPress
                                                }
                                                onMouseDown={
                                                  handleAddModelSearchResultsPress
                                                }
                                                onClick={(event) => {
                                                  if (
                                                    modelSearchPointerHandledRef.current
                                                  ) {
                                                    modelSearchPointerHandledRef.current = false
                                                    return
                                                  }
                                                  event.preventDefault()
                                                  handleAddModelSearchResults()
                                                }}
                                                disabled={
                                                  !canEditBasicFields ||
                                                  modelSearchIsLoading ||
                                                  modelSearchSummary.addable
                                                    .length === 0
                                                }
                                                title={
                                                  canEditBasicFields
                                                    ? undefined
                                                    : noPermissionMessage
                                                }
                                              >
                                                <Plus data-icon='inline-start' />
                                                {t(
                                                  'Add {{count}} search result(s)',
                                                  {
                                                    count:
                                                      modelSearchSummary.addable
                                                        .length,
                                                  }
                                                )}
                                              </Button>
                                            </div>
                                            {modelSearchPreviewNames.length >
                                              0 && (
                                              <div className='flex flex-wrap gap-1.5'>
                                                {modelSearchPreviewNames.map(
                                                  (model) => (
                                                    <Badge
                                                      key={model}
                                                      variant='secondary'
                                                      className='max-w-full truncate font-mono'
                                                    >
                                                      {model}
                                                    </Badge>
                                                  )
                                                )}
                                                {modelSearchPreviewOmittedCount >
                                                  0 && (
                                                  <Badge variant='outline'>
                                                    {t('+{{count}} more', {
                                                      count:
                                                        modelSearchPreviewOmittedCount,
                                                    })}
                                                  </Badge>
                                                )}
                                              </div>
                                            )}
                                            {modelSearchNameResult.unresolvedMatchedCount >
                                              0 && (
                                              <p className='text-muted-foreground text-xs'>
                                                {t(
                                                  '{{count}} more result(s) will be checked when adding',
                                                  {
                                                    count:
                                                      modelSearchNameResult.unresolvedMatchedCount,
                                                  }
                                                )}
                                              </p>
                                            )}
                                          </div>
                                        ) : undefined
                                      }
                                      preserveSelectedOnEmptyRemovalKey
                                      hideSelectedOptionsWhenSearching
                                      submitSearchOnEnterWithMatches
                                      submitSearchOnEnterWhenHighlighted
                                      clearSearchOnSelect={false}
                                    />
                                  </FormControl>
                                  {modelMappingGuardrail.exposedTargetModels
                                    .length > 0 && (
                                    <Alert className='mt-3 border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-50'>
                                      <AlertDescription className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
                                        <span>
                                          {t('The mapped upstream model(s)')}{' '}
                                          {formatModelNames(
                                            modelMappingGuardrail.exposedTargetModels
                                          )}{' '}
                                          {t(
                                            'are also listed here. Remove them from Models to keep the `/v1/models` response user-friendly and hide vendor-specific names.'
                                          )}
                                        </span>
                                        <Button
                                          type='button'
                                          variant='outline'
                                          size='sm'
                                          onClick={() => {
                                            const hiddenTargets = new Set(
                                              modelMappingGuardrail.exposedTargetModels
                                            )
                                            updateModels(
                                              currentModelsArray.filter(
                                                (model) =>
                                                  !hiddenTargets.has(model)
                                              )
                                            )
                                          }}
                                          disabled={!canEditBasicFields}
                                          title={
                                            canEditBasicFields
                                              ? undefined
                                              : noPermissionMessage
                                          }
                                        >
                                          {t('Remove mapped targets')}
                                        </Button>
                                      </AlertDescription>
                                    </Alert>
                                  )}
                                  <FormMessage />
                                </FormItem>
                              )}
                            />

                            <Separator />

                            <div className='flex flex-col gap-3'>
                              <div>
                                <p className='text-sm font-medium'>
                                  {t('Quick actions')}
                                </p>
                                <p className='text-muted-foreground text-xs'>
                                  {t(
                                    'Use presets or upstream discovery to populate the model list faster.'
                                  )}
                                </p>
                              </div>
                              <div className='flex flex-wrap gap-2'>
                                <Button
                                  type='button'
                                  variant='outline'
                                  size='sm'
                                  onClick={handleFillRelatedModels}
                                  disabled={
                                    !canEditBasicFields ||
                                    usesUpstreamAccountCredentialSource ||
                                    !basicModels.length
                                  }
                                  title={
                                    canEditBasicFields
                                      ? undefined
                                      : noPermissionMessage
                                  }
                                >
                                  <FileText data-icon='inline-start' />
                                  {t('Fill Related Models')}
                                </Button>
                                <Button
                                  type='button'
                                  variant='outline'
                                  size='sm'
                                  onClick={handleFillAllModels}
                                  disabled={
                                    !canEditBasicFields ||
                                    usesUpstreamAccountCredentialSource ||
                                    !allModelsList.length
                                  }
                                  title={
                                    canEditBasicFields
                                      ? undefined
                                      : noPermissionMessage
                                  }
                                >
                                  <Plus data-icon='inline-start' />
                                  {t('Fill All Models')}
                                </Button>
                                {MODEL_FETCHABLE_TYPES.has(currentType) &&
                                  !isGlobalAccountPoolMode && (
                                    <Button
                                      type='button'
                                      variant='outline'
                                      size='sm'
                                      onClick={handleFetchModels}
                                      disabled={
                                        !permissions.canOperate ||
                                        !canEditBasicFields ||
                                        usesUpstreamAccountCredentialSource
                                      }
                                      title={
                                        permissions.canOperate &&
                                        canEditBasicFields
                                          ? undefined
                                          : noPermissionMessage
                                      }
                                    >
                                      <Sparkles data-icon='inline-start' />
                                      {t('Fetch from Upstream')}
                                    </Button>
                                  )}
                                <Button
                                  type='button'
                                  variant='outline'
                                  size='sm'
                                  onClick={handleCopyModels}
                                  disabled={currentModelsArray.length === 0}
                                >
                                  <Copy data-icon='inline-start' />
                                  {t('Copy All')}
                                </Button>
                                <Button
                                  type='button'
                                  variant='ghost'
                                  size='sm'
                                  onClick={handleClearModels}
                                  disabled={
                                    !canEditBasicFields ||
                                    usesUpstreamAccountCredentialSource ||
                                    currentModelsArray.length === 0
                                  }
                                  title={
                                    canEditBasicFields
                                      ? undefined
                                      : noPermissionMessage
                                  }
                                >
                                  <Eraser data-icon='inline-start' />
                                  {t('Clear All')}
                                </Button>
                              </div>
                              {prefillGroups.length > 0 && (
                                <div className='flex flex-wrap items-center gap-2'>
                                  <span className='text-muted-foreground text-xs'>
                                    {t('Preset groups')}:
                                  </span>
                                  {prefillGroups.map((group) => (
                                    <Button
                                      key={group.id}
                                      type='button'
                                      variant='secondary'
                                      size='sm'
                                      onClick={() =>
                                        handleAddPrefillGroup(group)
                                      }
                                      disabled={!canEditBasicFields}
                                      title={
                                        canEditBasicFields
                                          ? undefined
                                          : noPermissionMessage
                                      }
                                    >
                                      {group.name}
                                    </Button>
                                  ))}
                                </div>
                              )}
                            </div>
                          </div>

                          <div className='border-border/60 rounded-lg border p-4'>
                            <FormField
                              control={form.control}
                              name='model_mapping'
                              render={({ field }) => (
                                <FormItem className='flex flex-col gap-3'>
                                  <div className='flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between'>
                                    <div className='flex flex-col gap-1'>
                                      <div className='flex items-center gap-2'>
                                        <div
                                          id='channel-model-mapping-label'
                                          className='text-sm leading-none font-medium'
                                        >
                                          {t('Model Mapping')}
                                        </div>
                                        <Tooltip>
                                          <TooltipTrigger
                                            render={
                                              <Button
                                                type='button'
                                                variant='ghost'
                                                size='icon-sm'
                                                className='text-muted-foreground hover:text-foreground size-auto p-0'
                                                aria-label={t(
                                                  'How model mapping works'
                                                )}
                                              />
                                            }
                                          >
                                            <HelpCircle aria-hidden='true' />
                                          </TooltipTrigger>
                                          <TooltipContent
                                            side='top'
                                            align='start'
                                            className='flex max-w-xs flex-col gap-2 text-left'
                                          >
                                            <p className='text-xs font-semibold tracking-wide uppercase'>
                                              {t('Request flow')}
                                            </p>
                                            <div className='flex flex-col gap-1 font-mono text-xs'>
                                              {mappingPreviewPairs.map(
                                                (pair) => (
                                                  <div
                                                    key={`${pair.source}-${pair.target}`}
                                                    className='flex items-center gap-1'
                                                  >
                                                    <span>{pair.source}</span>
                                                    <ArrowRight className='size-3.5 opacity-70' />
                                                    <span>{pair.target}</span>
                                                  </div>
                                                )
                                              )}
                                              {remainingMappingCount > 0 && (
                                                <div className='text-[11px] opacity-70'>
                                                  +{remainingMappingCount}{' '}
                                                  {t('more mapping')}
                                                  {remainingMappingCount > 1
                                                    ? 's'
                                                    : ''}
                                                </div>
                                              )}
                                            </div>
                                            <p className='text-[11px] leading-relaxed opacity-80'>
                                              {t(
                                                'Users call the model on the left. The platform forwards the request to the upstream model on the right.'
                                              )}
                                            </p>
                                          </TooltipContent>
                                        </Tooltip>
                                      </div>
                                      <FormDescription>
                                        {t(FIELD_DESCRIPTIONS.MODEL_MAPPING)}
                                      </FormDescription>
                                    </div>
                                  </div>
                                  <FormControl>
                                    <ModelMappingEditor
                                      aria-labelledby='channel-model-mapping-label'
                                      value={field.value || ''}
                                      onChange={field.onChange}
                                      disabled={
                                        isSubmitting || !canEditBasicFields
                                      }
                                      sourceModelOptions={currentModelsArray}
                                      targetModelOptions={baseModelOptions.map(
                                        (option) => option.value
                                      )}
                                    />
                                  </FormControl>
                                  {modelMappingGuardrail.invalidJson && (
                                    <Alert
                                      variant='destructive'
                                      className='mt-3'
                                    >
                                      <AlertDescription>
                                        {t(
                                          'Model Mapping must be a JSON object like'
                                        )}{' '}
                                        <code className='font-mono'>
                                          {'{"gpt-4":"Azure-GPT4"}'}
                                        </code>
                                        {t(
                                          '. Please fix the JSON before saving.'
                                        )}
                                      </AlertDescription>
                                    </Alert>
                                  )}
                                  {modelMappingGuardrail.missingSourceModels
                                    .length > 0 && (
                                    <Alert className='mt-3 border-amber-200 bg-amber-50 text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-50'>
                                      <AlertDescription className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
                                        <span>
                                          {t('Add')}{' '}
                                          {formatModelNames(
                                            modelMappingGuardrail.missingSourceModels
                                          )}{' '}
                                          {t(
                                            'to the Models list so users can use them before the mapping sends traffic upstream.'
                                          )}
                                        </span>
                                        <Button
                                          type='button'
                                          variant='outline'
                                          size='sm'
                                          onClick={() => {
                                            updateModels(
                                              modelMappingGuardrail.missingSourceModels,
                                              true
                                            )
                                          }}
                                          disabled={!canEditBasicFields}
                                          title={
                                            canEditBasicFields
                                              ? undefined
                                              : noPermissionMessage
                                          }
                                        >
                                          {t('Add missing models')}
                                        </Button>
                                      </AlertDescription>
                                    </Alert>
                                  )}
                                  <FormMessage />
                                </FormItem>
                              )}
                            />
                          </div>

                          <div className='border-border/60 rounded-lg border p-4'>
                            <FormField
                              control={form.control}
                              name='group'
                              render={({ field }) => (
                                <FormItem className='flex flex-col gap-3'>
                                  <div className='flex flex-col gap-1'>
                                    <FormLabel>{t('Groups *')}</FormLabel>
                                    <FormDescription>
                                      {t(FIELD_DESCRIPTIONS.GROUP)}
                                    </FormDescription>
                                  </div>
                                  <FormControl>
                                    {isLoadingGroups ? (
                                      <Skeleton className='h-10 w-full' />
                                    ) : (
                                      <MultiSelect
                                        options={groupOptions}
                                        selected={field.value || []}
                                        onChange={(values) => {
                                          if (!canEditBasicFields) {
                                            toast.error(noPermissionMessage)
                                            return
                                          }
                                          field.onChange(values)
                                        }}
                                        placeholder={t(
                                          FIELD_PLACEHOLDERS.GROUP
                                        )}
                                        disabled={!canEditBasicFields}
                                      />
                                    )}
                                  </FormControl>
                                  <FormMessage />
                                </FormItem>
                              )}
                            />
                          </div>
                        </div>
                      </ChannelModelsSection>
                    </div>
                  )}

                  <div
                    id={CHANNEL_EDITOR_SECTION_IDS.advanced}
                    className='scroll-mt-4'
                  >
                    <ChannelAdvancedSection
                      open={advancedSettingsOpen}
                      onOpenChange={handleAdvancedSettingsOpenChange}
                      summary={advancedSummary}
                    >
                      {/* ── Routing & Overrides ── */}
                      <div className={sideDrawerSectionClassName()}>
                        <CardHeading
                          title={t('Routing & Overrides')}
                          icon={<Route className='h-4 w-4' />}
                        />
                        <div
                          id={ADVANCED_SETTINGS_SECTION_IDS.routingStrategy}
                          className={configuredAdvancedSectionClassName(
                            'flex scroll-mt-4 flex-col gap-4',
                            routingStrategyConfigured
                          )}
                        >
                          <SubHeading
                            title={t('Routing Strategy')}
                            icon={<Route className='h-3.5 w-3.5' />}
                          />
                          <div className='grid gap-4 sm:grid-cols-2'>
                            <FormField
                              control={form.control}
                              name='priority'
                              render={({ field }) => (
                                <FormItem>
                                  <FormLabel>{t('Priority')}</FormLabel>
                                  <FormControl>
                                    <Input
                                      type='number'
                                      placeholder='0'
                                      disabled={!canEditBasicFields}
                                      {...field}
                                      onChange={(e) =>
                                        field.onChange(Number(e.target.value))
                                      }
                                    />
                                  </FormControl>
                                  <FormDescription>
                                    {t(FIELD_DESCRIPTIONS.PRIORITY)}
                                  </FormDescription>
                                  <FormMessage />
                                </FormItem>
                              )}
                            />

                            <FormField
                              control={form.control}
                              name='weight'
                              render={({ field }) => (
                                <FormItem>
                                  <FormLabel>{t('Weight')}</FormLabel>
                                  <FormControl>
                                    <Input
                                      type='number'
                                      placeholder='0'
                                      disabled={!canEditBasicFields}
                                      {...field}
                                      onChange={(e) =>
                                        field.onChange(Number(e.target.value))
                                      }
                                    />
                                  </FormControl>
                                  <FormDescription>
                                    {t(FIELD_DESCRIPTIONS.WEIGHT)}
                                  </FormDescription>
                                  <FormMessage />
                                </FormItem>
                              )}
                            />
                          </div>

                          <FormField
                            control={form.control}
                            name='test_model'
                            render={({ field }) => (
                              <FormItem>
                                <FormLabel>{t('Test Model')}</FormLabel>
                                <FormControl>
                                  <Input
                                    placeholder={t(
                                      FIELD_PLACEHOLDERS.TEST_MODEL
                                    )}
                                    disabled={!canEditBasicFields}
                                    {...field}
                                  />
                                </FormControl>
                                <FormDescription>
                                  {t(FIELD_DESCRIPTIONS.TEST_MODEL)}
                                </FormDescription>
                                <FormMessage />
                              </FormItem>
                            )}
                          />

                          <FormField
                            control={form.control}
                            name='auto_ban'
                            render={({ field }) => (
                              <FormItem className='flex items-center justify-between'>
                                <div className='space-y-0.5'>
                                  <FormLabel>{t('Auto Ban')}</FormLabel>
                                  <FormDescription>
                                    {t(FIELD_DESCRIPTIONS.AUTO_BAN)}
                                  </FormDescription>
                                </div>
                                <FormControl>
                                  <Switch
                                    checked={field.value === 1}
                                    disabled={!canEditBasicFields}
                                    onCheckedChange={(checked) =>
                                      field.onChange(checked ? 1 : 0)
                                    }
                                  />
                                </FormControl>
                              </FormItem>
                            )}
                          />
                        </div>

                        <div
                          id={ADVANCED_SETTINGS_SECTION_IDS.internalNotes}
                          className={configuredAdvancedSectionClassName(
                            'flex scroll-mt-4 flex-col gap-4 border-t pt-4',
                            internalNotesConfigured
                          )}
                        >
                          <SubHeading
                            title={t('Internal Notes')}
                            icon={<FileText className='h-3.5 w-3.5' />}
                          />
                          <div className='grid gap-4 sm:grid-cols-2'>
                            <FormField
                              control={form.control}
                              name='tag'
                              render={({ field }) => (
                                <FormItem>
                                  <FormLabel>{t('Tag')}</FormLabel>
                                  <FormControl>
                                    <Input
                                      placeholder={t(FIELD_PLACEHOLDERS.TAG)}
                                      disabled={!canEditBasicFields}
                                      {...field}
                                    />
                                  </FormControl>
                                  <FormDescription>
                                    {t(FIELD_DESCRIPTIONS.TAG)}
                                  </FormDescription>
                                  <FormMessage />
                                </FormItem>
                              )}
                            />

                            <FormField
                              control={form.control}
                              name='remark'
                              render={({ field }) => (
                                <FormItem>
                                  <FormLabel>{t('Remark')}</FormLabel>
                                  <FormControl>
                                    <Textarea
                                      placeholder={t(FIELD_PLACEHOLDERS.REMARK)}
                                      rows={2}
                                      disabled={!canEditBasicFields}
                                      {...field}
                                    />
                                  </FormControl>
                                  <FormDescription>
                                    {t(FIELD_DESCRIPTIONS.REMARK)}
                                  </FormDescription>
                                  <FormMessage />
                                </FormItem>
                              )}
                            />
                          </div>
                        </div>

                        <div
                          id={ADVANCED_SETTINGS_SECTION_IDS.overrideRules}
                          className={configuredAdvancedSectionClassName(
                            'flex scroll-mt-4 flex-col gap-4 border-t pt-4',
                            overrideRulesConfigured
                          )}
                        >
                          <SubHeading
                            title={t('Override Rules')}
                            icon={<Code className='h-3.5 w-3.5' />}
                          />

                          <FormField
                            control={form.control}
                            name='status_code_mapping'
                            render={({ field }) => (
                              <FormItem className='space-y-3'>
                                <div className='space-y-1'>
                                  <FormLabel>
                                    {t('Status Code Mapping')}
                                  </FormLabel>
                                  <FormDescription>
                                    {t(
                                      'Map upstream status codes to different codes'
                                    )}
                                  </FormDescription>
                                </div>
                                <FormControl>
                                  <JsonEditor
                                    value={field.value || ''}
                                    onChange={field.onChange}
                                    disabled={
                                      isSubmitting || !canEditBasicFields
                                    }
                                    keyPlaceholder='400'
                                    valuePlaceholder='500'
                                    keyLabel='Original Code'
                                    valueLabel='Mapped Code'
                                    emptyMessage={t(
                                      'No status code mappings configured.'
                                    )}
                                    template={{ '400': '500', '429': '503' }}
                                    valueType='string'
                                  />
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />

                          <FormField
                            control={form.control}
                            name='param_override'
                            render={({ field }) => (
                              <FormItem className='space-y-3 border-t pt-4'>
                                <div className='flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between'>
                                  <div className='space-y-1'>
                                    <FormLabel>
                                      {t('Parameter Override')}
                                    </FormLabel>
                                    <FormDescription>
                                      {t(
                                        'Override request parameters. Cannot override stream parameter.'
                                      )}
                                    </FormDescription>
                                  </div>
                                  <div className='flex flex-wrap gap-2'>
                                    <Button
                                      type='button'
                                      variant='outline'
                                      size='sm'
                                      onClick={() => {
                                        if (!canEditSensitiveFields) {
                                          toast.error(noPermissionMessage)
                                          return
                                        }
                                        setParamOverrideEditorOpen(true)
                                      }}
                                      disabled={!canEditSensitiveFields}
                                      title={
                                        canEditSensitiveFields
                                          ? undefined
                                          : noPermissionMessage
                                      }
                                    >
                                      <Wand2 className='mr-2 h-4 w-4' />
                                      {t('Visual edit')}
                                    </Button>
                                    <Button
                                      type='button'
                                      variant='outline'
                                      size='sm'
                                      onClick={() => {
                                        if (!canEditSensitiveFields) {
                                          toast.error(noPermissionMessage)
                                          return
                                        }
                                        field.onChange(
                                          JSON.stringify(
                                            {
                                              operations: [
                                                {
                                                  path: 'temperature',
                                                  mode: 'set',
                                                  value: 0.7,
                                                  conditions: [
                                                    {
                                                      path: 'model',
                                                      mode: 'prefix',
                                                      value: 'gpt',
                                                    },
                                                  ],
                                                  logic: 'AND',
                                                },
                                              ],
                                            },
                                            null,
                                            2
                                          )
                                        )
                                      }}
                                      disabled={!canEditSensitiveFields}
                                      title={
                                        canEditSensitiveFields
                                          ? undefined
                                          : noPermissionMessage
                                      }
                                    >
                                      <Code className='mr-2 h-4 w-4' />
                                      {t('New Format Template')}
                                    </Button>
                                    <Button
                                      type='button'
                                      variant='ghost'
                                      size='sm'
                                      onClick={() => {
                                        if (!canEditSensitiveFields) {
                                          toast.error(noPermissionMessage)
                                          return
                                        }
                                        field.onChange('')
                                      }}
                                      disabled={!canEditSensitiveFields}
                                      title={
                                        canEditSensitiveFields
                                          ? undefined
                                          : noPermissionMessage
                                      }
                                    >
                                      {t('Clear')}
                                    </Button>
                                  </div>
                                </div>
                                <FormControl>
                                  <JsonEditor
                                    value={field.value || ''}
                                    onChange={field.onChange}
                                    disabled={
                                      isSubmitting || !canEditSensitiveFields
                                    }
                                    keyPlaceholder='temperature'
                                    valuePlaceholder='0.7'
                                    keyLabel='Parameter'
                                    valueLabel='Value'
                                    emptyMessage={t(
                                      'No parameter overrides configured.'
                                    )}
                                    template={{
                                      temperature: 0.7,
                                      max_tokens: 2000,
                                      top_p: 1,
                                    }}
                                    valueType='any'
                                  />
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />

                          <FormField
                            control={form.control}
                            name='header_override'
                            render={({ field }) => (
                              <FormItem className='space-y-3 border-t pt-4'>
                                <div className='flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between'>
                                  <div className='space-y-1'>
                                    <FormLabel>
                                      {t('Request Header Override')}
                                    </FormLabel>
                                    <FormDescription>
                                      {t('Override request headers')}
                                    </FormDescription>
                                  </div>
                                  <div className='flex flex-wrap gap-2'>
                                    <Button
                                      type='button'
                                      variant='outline'
                                      size='sm'
                                      onClick={() => {
                                        if (!canEditSensitiveFields) {
                                          toast.error(noPermissionMessage)
                                          return
                                        }
                                        field.onChange(
                                          JSON.stringify(
                                            {
                                              '*': true,
                                              're:^X-Trace-.*$': true,
                                              'X-Foo': '{client_header:X-Foo}',
                                              Authorization: 'Bearer {api_key}',
                                            },
                                            null,
                                            2
                                          )
                                        )
                                      }}
                                      disabled={!canEditSensitiveFields}
                                      title={
                                        canEditSensitiveFields
                                          ? undefined
                                          : noPermissionMessage
                                      }
                                    >
                                      {t('Fill Template')}
                                    </Button>
                                    <Button
                                      type='button'
                                      variant='outline'
                                      size='sm'
                                      onClick={() => {
                                        if (!canEditSensitiveFields) {
                                          toast.error(noPermissionMessage)
                                          return
                                        }
                                        field.onChange(
                                          JSON.stringify({ '*': true }, null, 2)
                                        )
                                      }}
                                      disabled={!canEditSensitiveFields}
                                      title={
                                        canEditSensitiveFields
                                          ? undefined
                                          : noPermissionMessage
                                      }
                                    >
                                      {t('Passthrough Template')}
                                    </Button>
                                    <Button
                                      type='button'
                                      variant='outline'
                                      size='sm'
                                      onClick={() => {
                                        if (!canEditSensitiveFields) {
                                          toast.error(noPermissionMessage)
                                          return
                                        }
                                        try {
                                          const parsed = JSON.parse(
                                            field.value || '{}'
                                          )
                                          field.onChange(
                                            JSON.stringify(parsed, null, 2)
                                          )
                                        } catch (_e) {
                                          /* ignore invalid JSON */
                                        }
                                      }}
                                      disabled={!canEditSensitiveFields}
                                      title={
                                        canEditSensitiveFields
                                          ? undefined
                                          : noPermissionMessage
                                      }
                                    >
                                      {t('Format')}
                                    </Button>
                                    <Button
                                      type='button'
                                      variant='ghost'
                                      size='sm'
                                      onClick={() => {
                                        if (!canEditSensitiveFields) {
                                          toast.error(noPermissionMessage)
                                          return
                                        }
                                        field.onChange('')
                                      }}
                                      disabled={!canEditSensitiveFields}
                                      title={
                                        canEditSensitiveFields
                                          ? undefined
                                          : noPermissionMessage
                                      }
                                    >
                                      {t('Clear')}
                                    </Button>
                                  </div>
                                </div>
                                <FormControl>
                                  <Textarea
                                    className='font-mono text-sm'
                                    rows={6}
                                    value={field.value || ''}
                                    onChange={field.onChange}
                                    disabled={
                                      isSubmitting || !canEditSensitiveFields
                                    }
                                    placeholder={t(
                                      'Enter JSON to override request headers'
                                    )}
                                  />
                                </FormControl>
                                <FormDescription className='text-xs'>
                                  {t('Supported variables')}:{' '}
                                  <code className='bg-muted rounded px-1 py-0.5'>
                                    {'{api_key}'}
                                  </code>{' '}
                                  — {t('Channel key')},{' '}
                                  <code className='bg-muted rounded px-1 py-0.5'>
                                    {'{client_header:NAME}'}
                                  </code>{' '}
                                  — {t('Client header value')}
                                </FormDescription>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        </div>
                      </div>

                      {/* ── Extra Settings ── */}
                      <div
                        id={ADVANCED_SETTINGS_SECTION_IDS.extraSettings}
                        className={sideDrawerSectionClassName(
                          configuredAdvancedSectionClassName(
                            'scroll-mt-4',
                            extraSettingsConfigured
                          )
                        )}
                      >
                        <CardHeading
                          title={t('Channel Extra Settings')}
                          icon={<Settings className='h-4 w-4' />}
                        />
                        {(currentType === 1 ||
                          currentType === 14 ||
                          currentType === 57) && (
                          <div
                            id={ADVANCED_SETTINGS_SECTION_IDS.fieldPassthrough}
                            className={configuredAdvancedSectionClassName(
                              'flex scroll-mt-4 flex-col gap-3',
                              fieldPassthroughConfigured
                            )}
                          >
                            <SubHeading
                              title={t('Field passthrough controls')}
                              icon={
                                <SlidersHorizontal className='h-3.5 w-3.5' />
                              }
                            />

                            <div className='divide-border space-y-0 divide-y border-y'>
                              <FormField
                                control={form.control}
                                name='allow_service_tier'
                                render={({ field }) => (
                                  <FormItem className='flex items-center justify-between gap-3 px-4 py-3'>
                                    <div className='space-y-0.5'>
                                      <FormLabel className='text-sm'>
                                        {t('Allow service_tier passthrough')}
                                      </FormLabel>
                                      <FormDescription>
                                        {t(
                                          'Pass through the service_tier field'
                                        )}
                                      </FormDescription>
                                    </div>
                                    <FormControl>
                                      <Switch
                                        checked={field.value}
                                        disabled={!canEditSensitiveFields}
                                        onCheckedChange={field.onChange}
                                      />
                                    </FormControl>
                                  </FormItem>
                                )}
                              />

                              {(currentType === 1 || currentType === 57) && (
                                <>
                                  <FormField
                                    control={form.control}
                                    name='disable_store'
                                    render={({ field }) => (
                                      <FormItem className='flex items-center justify-between gap-3 px-4 py-3'>
                                        <div className='space-y-0.5'>
                                          <FormLabel className='text-sm'>
                                            {t('Disable store passthrough')}
                                          </FormLabel>
                                          <FormDescription>
                                            {t(
                                              'When enabled, the store field will be blocked'
                                            )}
                                          </FormDescription>
                                        </div>
                                        <FormControl>
                                          <Switch
                                            checked={field.value}
                                            disabled={!canEditSensitiveFields}
                                            onCheckedChange={field.onChange}
                                          />
                                        </FormControl>
                                      </FormItem>
                                    )}
                                  />

                                  <FormField
                                    control={form.control}
                                    name='allow_safety_identifier'
                                    render={({ field }) => (
                                      <FormItem className='flex items-center justify-between gap-3 px-4 py-3'>
                                        <div className='space-y-0.5'>
                                          <FormLabel className='text-sm'>
                                            {t(
                                              'Allow safety_identifier passthrough'
                                            )}
                                          </FormLabel>
                                          <FormDescription>
                                            {t(
                                              'Pass through the safety_identifier field'
                                            )}
                                          </FormDescription>
                                        </div>
                                        <FormControl>
                                          <Switch
                                            checked={field.value}
                                            disabled={!canEditSensitiveFields}
                                            onCheckedChange={field.onChange}
                                          />
                                        </FormControl>
                                      </FormItem>
                                    )}
                                  />

                                  <FormField
                                    control={form.control}
                                    name='allow_include_obfuscation'
                                    render={({ field }) => (
                                      <FormItem className='flex items-center justify-between gap-3 px-4 py-3'>
                                        <div className='space-y-0.5'>
                                          <FormLabel className='text-sm'>
                                            {t(
                                              'Allow include usage obfuscation passthrough'
                                            )}
                                          </FormLabel>
                                          <FormDescription>
                                            {t(
                                              'Pass through the include field for usage obfuscation'
                                            )}
                                          </FormDescription>
                                        </div>
                                        <FormControl>
                                          <Switch
                                            checked={field.value}
                                            disabled={!canEditSensitiveFields}
                                            onCheckedChange={field.onChange}
                                          />
                                        </FormControl>
                                      </FormItem>
                                    )}
                                  />

                                  <FormField
                                    control={form.control}
                                    name='allow_inference_geo'
                                    render={({ field }) => (
                                      <FormItem className='flex items-center justify-between gap-3 px-4 py-3'>
                                        <div className='space-y-0.5'>
                                          <FormLabel className='text-sm'>
                                            {t(
                                              'Allow inference geography passthrough'
                                            )}
                                          </FormLabel>
                                          <FormDescription>
                                            {t(
                                              'Pass through the inference_geo field for geographic routing'
                                            )}
                                          </FormDescription>
                                        </div>
                                        <FormControl>
                                          <Switch
                                            checked={field.value}
                                            disabled={!canEditSensitiveFields}
                                            onCheckedChange={field.onChange}
                                          />
                                        </FormControl>
                                      </FormItem>
                                    )}
                                  />
                                </>
                              )}

                              {currentType === 14 && (
                                <>
                                  <FormField
                                    control={form.control}
                                    name='allow_inference_geo'
                                    render={({ field }) => (
                                      <FormItem className='flex items-center justify-between gap-3 px-4 py-3'>
                                        <div className='space-y-0.5'>
                                          <FormLabel className='text-sm'>
                                            {t(
                                              'Allow inference_geo passthrough'
                                            )}
                                          </FormLabel>
                                          <FormDescription>
                                            {t(
                                              'Pass through the inference_geo field for Claude data residency region control'
                                            )}
                                          </FormDescription>
                                        </div>
                                        <FormControl>
                                          <Switch
                                            checked={field.value}
                                            disabled={!canEditSensitiveFields}
                                            onCheckedChange={field.onChange}
                                          />
                                        </FormControl>
                                      </FormItem>
                                    )}
                                  />

                                  <FormField
                                    control={form.control}
                                    name='allow_speed'
                                    render={({ field }) => (
                                      <FormItem className='flex items-center justify-between gap-3 px-4 py-3'>
                                        <div className='space-y-0.5'>
                                          <FormLabel className='text-sm'>
                                            {t('Allow speed passthrough')}
                                          </FormLabel>
                                          <FormDescription>
                                            {t(
                                              'Pass through the speed field for Claude inference speed mode control'
                                            )}
                                          </FormDescription>
                                        </div>
                                        <FormControl>
                                          <Switch
                                            checked={field.value}
                                            disabled={!canEditSensitiveFields}
                                            onCheckedChange={field.onChange}
                                          />
                                        </FormControl>
                                      </FormItem>
                                    )}
                                  />

                                  <FormField
                                    control={form.control}
                                    name='claude_beta_query'
                                    render={({ field }) => (
                                      <FormItem className='flex items-center justify-between gap-3 px-4 py-3'>
                                        <div className='space-y-0.5'>
                                          <FormLabel className='text-sm'>
                                            {t(
                                              'Allow Claude beta query passthrough'
                                            )}
                                          </FormLabel>
                                          <FormDescription>
                                            {t(
                                              'Pass through the anthropic-beta header for beta features'
                                            )}
                                          </FormDescription>
                                        </div>
                                        <FormControl>
                                          <Switch
                                            checked={field.value}
                                            disabled={!canEditSensitiveFields}
                                            onCheckedChange={field.onChange}
                                          />
                                        </FormControl>
                                      </FormItem>
                                    )}
                                  />
                                </>
                              )}
                            </div>
                          </div>
                        )}

                        <div className='divide-border space-y-0 divide-y border-y'>
                          {currentType === 1 && (
                            <FormField
                              control={form.control}
                              name='force_format'
                              render={({ field }) => (
                                <FormItem className='flex items-center justify-between px-4 py-3'>
                                  <div className='space-y-0.5'>
                                    <FormLabel>{t('Force Format')}</FormLabel>
                                    <FormDescription>
                                      {t(
                                        'Force format response to OpenAI standard (OpenAI channel only)'
                                      )}
                                    </FormDescription>
                                  </div>
                                  <FormControl>
                                    <Switch
                                      checked={field.value}
                                      disabled={!canEditSensitiveFields}
                                      onCheckedChange={field.onChange}
                                    />
                                  </FormControl>
                                </FormItem>
                              )}
                            />
                          )}

                          <FormField
                            control={form.control}
                            name='thinking_to_content'
                            render={({ field }) => (
                              <FormItem className='flex items-center justify-between px-4 py-3'>
                                <div className='space-y-0.5'>
                                  <FormLabel>
                                    {t('Thinking to Content')}
                                  </FormLabel>
                                  <FormDescription>
                                    {t(
                                      'Convert reasoning_content to <think> tag in content'
                                    )}
                                  </FormDescription>
                                </div>
                                <FormControl>
                                  <Switch
                                    checked={field.value}
                                    disabled={!canEditSensitiveFields}
                                    onCheckedChange={field.onChange}
                                  />
                                </FormControl>
                              </FormItem>
                            )}
                          />

                          <FormField
                            control={form.control}
                            name='pass_through_body_enabled'
                            render={({ field }) => (
                              <FormItem className='flex items-center justify-between px-4 py-3'>
                                <div className='space-y-0.5'>
                                  <FormLabel>
                                    {t('Pass Through Body')}
                                  </FormLabel>
                                  <FormDescription>
                                    {t(
                                      'Pass request body directly to upstream'
                                    )}
                                  </FormDescription>
                                </div>
                                <FormControl>
                                  <Switch
                                    checked={field.value}
                                    disabled={!canEditSensitiveFields}
                                    onCheckedChange={field.onChange}
                                  />
                                </FormControl>
                              </FormItem>
                            )}
                          />

                          <FormField
                            control={form.control}
                            name='disable_task_polling_sleep'
                            render={({ field }) => (
                              <FormItem className='flex items-center justify-between px-4 py-3'>
                                <div className='flex flex-col gap-0.5'>
                                  <FormLabel>
                                    {t('Skip async task polling delay')}
                                  </FormLabel>
                                  <FormDescription>
                                    {t(
                                      'Do not wait one second between polling async tasks for this channel'
                                    )}
                                  </FormDescription>
                                </div>
                                <FormControl>
                                  <Switch
                                    checked={field.value}
                                    disabled={!canEditSensitiveFields}
                                    onCheckedChange={field.onChange}
                                  />
                                </FormControl>
                              </FormItem>
                            )}
                          />
                        </div>

                        <FormField
                          control={form.control}
                          name='proxy'
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t('Proxy Address')}</FormLabel>
                              <FormControl>
                                <Input
                                  placeholder={t(
                                    'socks5://user:pass@host:port'
                                  )}
                                  disabled={!canEditSensitiveFields}
                                  {...field}
                                />
                              </FormControl>
                              <FormDescription>
                                {t(
                                  'Network proxy for this channel (supports socks5 protocol)'
                                )}
                              </FormDescription>
                              <FormMessage />
                            </FormItem>
                          )}
                        />

                        <FormField
                          control={form.control}
                          name='system_prompt'
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t('System Prompt')}</FormLabel>
                              <FormControl>
                                <Textarea
                                  placeholder={t(
                                    'Enter system prompt (user prompt takes priority)'
                                  )}
                                  rows={3}
                                  disabled={!canEditSensitiveFields}
                                  {...field}
                                />
                              </FormControl>
                              <FormDescription>
                                {t('Default system prompt for this channel')}
                              </FormDescription>
                              <FormMessage />
                            </FormItem>
                          )}
                        />

                        <FormField
                          control={form.control}
                          name='system_prompt_override'
                          render={({ field }) => (
                            <FormItem className='flex items-center justify-between'>
                              <div className='space-y-0.5'>
                                <FormLabel>
                                  {t('System Prompt Concatenation')}
                                </FormLabel>
                                <FormDescription>
                                  {t(
                                    'Concatenate channel system prompt with user&apos;s prompt'
                                  )}
                                </FormDescription>
                              </div>
                              <FormControl>
                                <Switch
                                  checked={field.value}
                                  disabled={!canEditSensitiveFields}
                                  onCheckedChange={field.onChange}
                                />
                              </FormControl>
                            </FormItem>
                          )}
                        />

                        {MODEL_FETCHABLE_TYPES.has(currentType) && (
                          <div
                            id={
                              ADVANCED_SETTINGS_SECTION_IDS.upstreamModelDetection
                            }
                            className={configuredAdvancedSectionClassName(
                              'flex scroll-mt-4 flex-col gap-3',
                              upstreamModelDetectionConfigured
                            )}
                          >
                            <SubHeading
                              title={t('Upstream Model Detection Settings')}
                              icon={<RefreshCw className='h-3.5 w-3.5' />}
                            />
                            <div className='divide-border space-y-0 divide-y border-y'>
                              <FormField
                                control={form.control}
                                name='upstream_model_update_check_enabled'
                                render={({ field }) => (
                                  <FormItem className='flex items-center justify-between px-4 py-3'>
                                    <div className='space-y-0.5'>
                                      <FormLabel>
                                        {t('Upstream Model Update Check')}
                                      </FormLabel>
                                      <FormDescription>
                                        {t(
                                          'Periodically check for upstream model changes'
                                        )}
                                      </FormDescription>
                                    </div>
                                    <FormControl>
                                      <Switch
                                        checked={field.value}
                                        disabled={!canEditSensitiveFields}
                                        onCheckedChange={field.onChange}
                                      />
                                    </FormControl>
                                  </FormItem>
                                )}
                              />
                              <FormField
                                control={form.control}
                                name='upstream_model_update_auto_sync_enabled'
                                render={({ field }) => (
                                  <FormItem className='flex items-center justify-between px-4 py-3'>
                                    <div className='space-y-0.5'>
                                      <FormLabel>
                                        {t('Auto Sync Upstream Models')}
                                      </FormLabel>
                                      <FormDescription>
                                        {t(
                                          'Automatically sync model list when upstream changes are detected'
                                        )}
                                      </FormDescription>
                                    </div>
                                    <FormControl>
                                      <Switch
                                        checked={field.value}
                                        disabled={
                                          !canEditSensitiveFields ||
                                          !upstreamModelUpdateCheckEnabled
                                        }
                                        onCheckedChange={field.onChange}
                                      />
                                    </FormControl>
                                  </FormItem>
                                )}
                              />
                            </div>
                            <FormField
                              control={form.control}
                              name='upstream_model_update_ignored_models'
                              render={({ field }) => (
                                <FormItem>
                                  <FormLabel>
                                    {t('Ignored upstream models')}
                                  </FormLabel>
                                  <FormControl>
                                    <Input
                                      placeholder={t(
                                        'e.g., gpt-4.1-nano,regex:^claude-.*$,regex:^sora-.*$'
                                      )}
                                      disabled={!canEditSensitiveFields}
                                      {...field}
                                    />
                                  </FormControl>
                                  <FormDescription>
                                    {t(
                                      'Comma-separated exact model names. Prefix with regex: to ignore by regular expression.'
                                    )}
                                  </FormDescription>
                                  <FormMessage />
                                </FormItem>
                              )}
                            />
                            <div className='text-muted-foreground space-y-2 border-t pt-3 text-xs'>
                              <div>
                                <span className='text-foreground font-medium'>
                                  {t('Last check time')}:
                                </span>{' '}
                                {formatUnixTime(
                                  upstreamUpdateMeta.lastCheckTime
                                )}
                              </div>
                              <div>
                                <span className='text-foreground font-medium'>
                                  {t('Last detected addable models')}:
                                </span>{' '}
                                {upstreamUpdateMeta.detectedModels.length ===
                                0 ? (
                                  t('None')
                                ) : (
                                  <>
                                    <Tooltip>
                                      <TooltipTrigger
                                        render={
                                          <button
                                            type='button'
                                            className='text-left break-all underline decoration-dotted underline-offset-2'
                                          />
                                        }
                                      >
                                        {upstreamDetectedModelsPreview.join(
                                          ', '
                                        )}
                                      </TooltipTrigger>
                                      <TooltipContent
                                        side='top'
                                        align='start'
                                        className='max-w-[40rem] whitespace-normal'
                                      >
                                        <span className='break-all'>
                                          {upstreamUpdateMeta.detectedModels.join(
                                            ', '
                                          )}
                                        </span>
                                      </TooltipContent>
                                    </Tooltip>
                                    {upstreamDetectedModelsOmittedCount > 0 && (
                                      <span className='ml-1'>
                                        {t(
                                          '({{total}} total, {{omit}} omitted)',
                                          {
                                            total:
                                              upstreamUpdateMeta.detectedModels
                                                .length,
                                            omit: upstreamDetectedModelsOmittedCount,
                                          }
                                        )}
                                      </span>
                                    )}
                                  </>
                                )}
                              </div>
                            </div>
                          </div>
                        )}
                      </div>
                    </ChannelAdvancedSection>
                  </div>
                </div>
              </div>
            </form>
          </Form>

          <SheetFooter className={sideDrawerFooterClassName()}>
            <SheetClose
              render={
                <Button
                  variant='outline'
                  disabled={isSubmitting || isSavingSyncedAccountConfigs}
                />
              }
            >
              {t('Cancel')}
            </SheetClose>
            <Button
              form='channel-form'
              type='submit'
              disabled={
                isSubmitting ||
                isSavingSyncedAccountConfigs ||
                !canSubmitForm ||
                isChannelDetailLoading
              }
              title={canSubmitForm ? undefined : noPermissionMessage}
            >
              {(isSubmitting || isSavingSyncedAccountConfigs) && (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              )}
              {isEditing && isUpstreamAccountSyncedChannel
                ? t('Save synced key configuration')
                : isEditing
                  ? t('Update Channel')
                  : t('Save changes')}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      {paramOverrideEditorOpen && (
        <ParamOverrideEditorDialog
          open={paramOverrideEditorOpen}
          value={form.watch('param_override') || ''}
          onOpenChange={setParamOverrideEditorOpen}
          onSave={(nextValue) => {
            if (!canEditSensitiveFields) {
              toast.error(noPermissionMessage)
              return
            }
            form.setValue('param_override', nextValue, {
              shouldDirty: true,
              shouldValidate: true,
            })
          }}
        />
      )}

      {advancedCustomEditorOpen && (
        <AdvancedCustomEditorDialog
          open={advancedCustomEditorOpen}
          value={form.watch('advanced_custom') || ''}
          onOpenChange={setAdvancedCustomEditorOpen}
          onSave={(nextValue) => {
            if (!canEditSensitiveFields) {
              toast.error(noPermissionMessage)
              return
            }
            form.setValue('advanced_custom', nextValue, {
              shouldDirty: true,
              shouldValidate: true,
            })
          }}
        />
      )}

      {/* 上游模型选择弹窗：编辑模式按 channel id 拉取，新建模式按当前表单凭证拉取。 */}
      <FetchModelsDialog
        open={fetchModelsDialogOpen}
        onOpenChange={setFetchModelsDialogOpen}
        onModelsSelected={(models) => {
          form.setValue('models', formatModelsArray(models))
        }}
        redirectModels={redirectModelList}
        redirectSourceModels={redirectModelKeyList}
        customFetcher={!isEditing ? createModeFetcher : undefined}
        existingModelsOverride={parseModelsString(
          form.getValues('models') || ''
        )}
        channelName={!isEditing ? currentName?.trim() : undefined}
      />

      <SecureVerificationDialog
        open={verificationOpen}
        onOpenChange={(open) => {
          if (!open) {
            cancelVerification()
          }
        }}
        methods={verificationMethods}
        state={verificationState}
        onVerify={async (method, code) => {
          await executeVerification(method, code)
        }}
        onCancel={cancelVerification}
        onCodeChange={setVerificationCode}
        onMethodChange={switchVerificationMethod}
      />

      {/* 模型映射源模型缺失确认弹窗。 */}
      <MissingModelsConfirmationDialog
        open={missingModelsDialogOpen}
        missingModels={missingModelsList}
        onConfirm={handleMissingModelsAction}
        onOpenChange={setMissingModelsDialogOpen}
      />

      <StatusCodeRiskDialog
        open={statusCodeRiskOpen}
        onOpenChange={(v) => {
          if (!v) handleStatusCodeRiskAction(false)
        }}
        detailItems={statusCodeRiskDetailItems}
        onConfirm={() => handleStatusCodeRiskAction(true)}
      />
    </>
  )
}
