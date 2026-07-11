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
import { type SubmitErrorHandler, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useDebounce } from '@/hooks'
import {
  ArrowRight,
  AlertCircle,
  CheckCircle2,
  Circle,
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
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { useHiddenClickUnlock } from '@/hooks/use-hidden-click-unlock'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
  getChannelKey,
  getGroups,
  getPrefillGroups,
  refreshCodexCredential,
} from '../../api'
import {
  ADD_MODE_OPTIONS,
  CHANNEL_STATUS_LABELS,
  CHANNEL_TYPE_OPTIONS,
  CHANNEL_TYPE_WARNINGS,
  ERROR_MESSAGES,
  FIELD_DESCRIPTIONS,
  FIELD_PLACEHOLDERS,
  MODEL_FETCHABLE_TYPES,
} from '../../constants'
import { useChannelMutateForm } from '../../hooks/use-channel-mutate-form'
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
  getChannelTypeIcon,
  getKeyPromptForType,
  parseModelsString,
  formatModelsArray,
  extractRedirectModels,
  extractMappingSourceModels,
  hasModelConfigChanged,
  findMissingModelsInMapping,
  buildModelSearchAppendPlan,
  buildModelSearchAppendSummary,
  dedupeModelNames,
  getModelSearchVendorForChannelType,
  getModelSearchModelNames,
  getMissingModelSearchMatches,
  isModelSearchAppendContextCurrent,
  mergeModelNames,
  validateModelMappingJson,
  hasAdvancedSettingsErrors,
} from '../../lib'
import {
  collectInvalidStatusCodeEntries,
  collectNewDisallowedStatusCodeRedirects,
} from '../../lib/status-code-risk-guard'
import type { Channel } from '../../types'
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

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function getErrorMessage(error: unknown): string | undefined {
  if (error instanceof Error && typeof error.message === 'string') {
    return error.message
  }

  if (!isRecord(error)) return undefined

  const response = error.response
  if (isRecord(response)) {
    const data = response.data
    if (isRecord(data)) {
      const message = data.message
      if (typeof message === 'string') return message
    }
  }

  const message = error.message
  if (typeof message === 'string') return message
  return undefined
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
const MODEL_SEARCH_APPEND_PAGE_SIZE = 100

async function fetchAllModelSearchModelNames(
  keyword: string,
  vendor = ''
): Promise<string[]> {
  const trimmedKeyword = keyword.trim()
  const trimmedVendor = vendor.trim()
  if (!trimmedKeyword) return []

  const names: string[] = []
  let page = 1

  for (;;) {
    const response = await searchModels({
      keyword: trimmedKeyword,
      vendor: trimmedVendor || undefined,
      p: page,
      page_size: MODEL_SEARCH_APPEND_PAGE_SIZE,
    })

    if (!response.success) {
      throw new Error(response.message || '')
    }

    const data = response.data
    if (!data) return names

    const items = data.items ?? []
    names.push(...getModelSearchModelNames(items, trimmedKeyword))

    const pageSize = data.page_size || MODEL_SEARCH_APPEND_PAGE_SIZE
    const loadedCount = page * pageSize
    if (loadedCount >= data.total || items.length === 0) {
      return names
    }

    page += 1
  }
}

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

function ChannelTypeLogo(props: {
  type: number
  size?: number
  className?: string
}) {
  const isKnownType = CHANNEL_TYPE_OPTIONS.some(
    (option) => option.value === props.type
  )

  if (!isKnownType) {
    return (
      <Server
        className={cn('text-muted-foreground shrink-0', props.className)}
        style={{
          width: props.size ?? 16,
          height: props.size ?? 16,
        }}
        aria-hidden='true'
      />
    )
  }

  return (
    <span className={cn('inline-flex shrink-0', props.className)}>
      {getLobeIcon(`${getChannelTypeIcon(props.type)}.Color`, props.size ?? 16)}
    </span>
  )
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
          className='bg-primary block size-2 rounded-full'
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
          <span className='block truncate text-sm font-medium'>
            {item.title}
          </span>
          {item.description && (
            <span className='text-muted-foreground block truncate text-xs'>
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
                        className='bg-primary size-1.5 shrink-0 rounded-full'
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
                              className='bg-primary size-1.5 shrink-0 rounded-full'
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
  const form = useForm<ChannelFormValues>({
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
  const initialModelsRef = useRef<string[]>([])
  const initialModelMappingRef = useRef<string>('')
  const initialStatusCodeMappingRef = useRef<string>('')
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
  const channelFormRef = useRef<HTMLFormElement>(null)
  const advancedNavScrollPendingRef = useRef(false)
  const modelSearchAppendRequestSeqRef = useRef(0)
  const isAddingModelSearchMatchesRef = useRef(false)
  const [advancedSettingsOpen, setAdvancedSettingsOpen] = useState(false)
  const [paramOverrideEditorOpen, setParamOverrideEditorOpen] = useState(false)
  const [advancedCustomEditorOpen, setAdvancedCustomEditorOpen] =
    useState(false)
  const [activeEditorSectionId, setActiveEditorSectionId] = useState<string>(
    CHANNEL_EDITOR_SECTION_IDS.identity
  )
  const [expandedEditorNavItemId, setExpandedEditorNavItemId] = useState<
    string | undefined
  >()
  const [modelSearchKeyword, setModelSearchKeyword] = useState('')
  const [modelSelectOpen, setModelSelectOpen] = useState(false)
  const [isAddingModelSearchMatches, setIsAddingModelSearchMatches] =
    useState(false)
  const trimmedModelSearchKeyword = modelSearchKeyword.trim()
  const modelSearchVendor = useMemo(
    () => getModelSearchVendorForChannelType(currentType),
    [currentType]
  )
  const debouncedModelSearchKeyword = useDebounce(
    trimmedModelSearchKeyword,
    300
  )
  const isModelSearchDebouncing =
    trimmedModelSearchKeyword.length > 0 &&
    trimmedModelSearchKeyword !== debouncedModelSearchKeyword
  const isModelSearchResultCurrent =
    trimmedModelSearchKeyword.length > 0 &&
    trimmedModelSearchKeyword === debouncedModelSearchKeyword
  const clearModelSearch = useCallback(() => {
    setModelSearchKeyword('')
  }, [])
  const modelSearchKeywordRef = useRef(trimmedModelSearchKeyword)
  const modelSearchVendorRef = useRef(modelSearchVendor)

  const modelSearchAppendContextRef = useRef({
    open,
    channelId: currentRow?.id ?? null,
    keyword: trimmedModelSearchKeyword,
    vendor: modelSearchVendor,
  })

  const isEditing = Boolean(currentRow)
  const channelId = currentRow?.id ?? null
  const canSubmitForm = isEditing
    ? permissions.canWrite || permissions.canSensitiveWrite
    : permissions.canSensitiveWrite
  const canEditSensitiveFields = permissions.canSensitiveWrite
  const canEditBasicFields =
    permissions.canWrite || permissions.canSensitiveWrite

  // 编辑渠道时拉取完整渠道详情，用于回填表单和保留历史配置。
  const { data: channelData, isLoading: isChannelLoading } = useQuery({
    queryKey: channelsQueryKeys.detail(currentRow?.id || 0),
    queryFn: () => getChannel(currentRow!.id),
    enabled: isEditing && Boolean(currentRow?.id),
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

  // 用模型元信息搜索补齐系统模型候选源，避免已同步到模型库但尚未加入任何渠道的模型不可选。
  const { data: modelSearchData, isFetching: isSearchingModelMeta } = useQuery({
    queryKey: [
      'channel_model_meta_search',
      debouncedModelSearchKeyword,
      modelSearchVendor,
    ],
    queryFn: () =>
      searchModels({
        keyword: debouncedModelSearchKeyword,
        vendor: modelSearchVendor || undefined,
        p: 1,
        page_size: 50,
      }),
    enabled: open && debouncedModelSearchKeyword.length > 0,
    staleTime: 30_000,
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

  const { copyToClipboard } = useCopyToClipboard()

  useEffect(() => {
    modelSearchKeywordRef.current = trimmedModelSearchKeyword
  }, [trimmedModelSearchKeyword])

  useEffect(() => {
    modelSearchVendorRef.current = modelSearchVendor
  }, [modelSearchVendor])

  useEffect(() => {
    modelSearchAppendContextRef.current = {
      open,
      channelId,
      keyword: trimmedModelSearchKeyword,
      vendor: modelSearchVendor,
    }
  }, [channelId, modelSearchVendor, open, trimmedModelSearchKeyword])

  useEffect(() => {
    modelSearchAppendRequestSeqRef.current += 1
    isAddingModelSearchMatchesRef.current = false
    setIsAddingModelSearchMatches(false)
  }, [channelId, modelSearchVendor, open])

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
      clearModelSearch()
      setModelSelectOpen(false)
    } else if (channelId) {
      setChannelKey(null)
    }
  }, [open, channelId, clearModelSearch])

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
  const supportsMultiKeyAddMode =
    currentType !== 57 && !(currentType === 41 && vertexKeyType === 'api_key')

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

  const modelSearchModelNames = useMemo(() => {
    if (!isModelSearchResultCurrent) return []
    return getModelSearchModelNames(
      modelSearchData?.data?.items ?? [],
      debouncedModelSearchKeyword
    )
  }, [debouncedModelSearchKeyword, isModelSearchResultCurrent, modelSearchData])

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
    !isGlobalAccountPoolMode && [3, 8, 36, 45].includes(currentType)
  const providerRequiresOther = [3, 18, 21, 39, 41, 49].includes(currentType)
  const identityComplete = Boolean(currentName?.trim() && currentType > 0)
  const credentialsComplete = isGlobalAccountPoolMode
    ? Boolean(accountPoolGroupId)
    : Boolean(
        (isEditing || currentKey?.trim()) &&
        (!providerRequiresBaseUrl || currentBaseUrl?.trim()) &&
        (!providerRequiresOther || currentOther?.trim())
      )
  const modelsComplete = Boolean(
    currentModelsArray.length > 0 && currentGroups?.length
  )
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
    {
      id: CHANNEL_EDITOR_SECTION_IDS.models,
      title: t('Models & Groups'),
      description: getSectionStatusLabel(modelsStatus, t),
      statusLabel: getSectionStatusLabel(modelsStatus, t),
      status: modelsStatus,
      icon: <Boxes className='h-4 w-4' aria-hidden='true' />,
    },
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
      icon: <ChannelTypeLogo type={option.value} size={16} />,
    }))
    if (!options.some((option) => Number(option.value) === currentType)) {
      options.push({
        value: String(currentType),
        label: `#${currentType}`,
        icon: <ChannelTypeLogo type={currentType} size={16} />,
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

  // 将系统模型和当前渠道模型合并成模型选择器选项，避免编辑历史模型时选项丢失。
  const modelOptions = useMemo(() => {
    const allModels = new Set([
      ...modelSearchModelNames,
      ...allModelsList,
      ...currentModelsArray,
    ])
    return Array.from(allModels).map((model) => ({
      value: model,
      label: model,
    }))
  }, [allModelsList, currentModelsArray, modelSearchModelNames])

  const modelSearchAppendPlan = useMemo(
    () =>
      buildModelSearchAppendPlan(modelSearchModelNames, currentModelsArray, 6),
    [currentModelsArray, modelSearchModelNames]
  )
  const modelSearchAppendSummary = useMemo(
    () =>
      buildModelSearchAppendSummary(modelSearchModelNames, currentModelsArray),
    [currentModelsArray, modelSearchModelNames]
  )
  const modelSearchMissingPreview = modelSearchAppendPlan.previewModels
  const modelSearchMissingOmittedCount = modelSearchAppendPlan.omittedCount
  const loadedModelSearchResultCount = isModelSearchResultCurrent
    ? (modelSearchData?.data?.items?.length ?? 0)
    : 0
  const modelSearchBackendResultTotal = isModelSearchResultCurrent
    ? (modelSearchData?.data?.total ?? loadedModelSearchResultCount)
    : loadedModelSearchResultCount
  const unscannedModelSearchResultCount = Math.max(
    0,
    modelSearchBackendResultTotal - loadedModelSearchResultCount
  )
  const canRunModelSearchAppend =
    modelSearchAppendSummary.addableCount > 0 ||
    unscannedModelSearchResultCount > 0
  const modelSearchAddButtonLabel =
    unscannedModelSearchResultCount > 0
      ? t('Scan all search results')
      : modelSearchAppendSummary.addableCount > 0
        ? t('Add {{count}} new model(s)', {
            count: modelSearchAppendSummary.addableCount,
          })
        : t('Add')
  const shouldShowModelSearchAppend =
    trimmedModelSearchKeyword.length > 0 &&
    !isSearchingModelMeta &&
    !isModelSearchDebouncing &&
    (modelSearchAppendSummary.matchedCount > 0 ||
      unscannedModelSearchResultCount > 0)

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
      const defaults = transformChannelToFormDefaults(channelData.data)
      form.reset(defaults)
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
      setAdvancedSettingsOpen(false)
      initialModelsRef.current = []
      initialModelMappingRef.current = ''
      initialStatusCodeMappingRef.current = ''
    }
  }, [isEditing, channelData, form])

  // 渠道类型变化时补充类型默认值；编辑模式不自动覆盖已有渠道配置。
  useEffect(() => {
    if (isEditing) return

    // 火山引擎默认使用北京区域；账号池组模式下不填写渠道自身 base_url。
    if (currentType === 45 && !isGlobalAccountPoolMode) {
      const currentBaseUrlValue = form.getValues('base_url')
      if (!currentBaseUrlValue || currentBaseUrlValue === '') {
        form.setValue('base_url', 'https://ark.cn-beijing.volces.com')
      }
    }

    // 讯飞星火渠道需要默认版本号，账号池模式也保留该协议字段。
    if (currentType === 18) {
      const currentOther = form.getValues('other')
      if (!currentOther || currentOther === '') {
        form.setValue('other', 'v2.1')
      }
    }
  }, [currentType, isEditing, form, isGlobalAccountPoolMode])

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

  const handleAddModelSearchMatches = useCallback(async () => {
    if (!canEditBasicFields) {
      toast.error(noPermissionMessage)
      return
    }
    if (isAddingModelSearchMatchesRef.current) {
      return
    }
    const keyword = modelSearchKeywordRef.current.trim()
    const vendor = modelSearchVendorRef.current.trim()
    if (!keyword) {
      toast.info(t('No new search results to add'))
      return
    }
    if (debouncedModelSearchKeyword.trim() !== keyword) {
      toast.info(t('Searching model metadata...'))
      return
    }

    const requestSeq = modelSearchAppendRequestSeqRef.current + 1
    modelSearchAppendRequestSeqRef.current = requestSeq
    isAddingModelSearchMatchesRef.current = true
    setIsAddingModelSearchMatches(true)
    const requestChannelId = channelId
    try {
      const allSearchModelNames = await fetchAllModelSearchModelNames(
        keyword,
        vendor
      )
      const latestContext = modelSearchAppendContextRef.current
      if (
        modelSearchAppendRequestSeqRef.current !== requestSeq ||
        !isModelSearchAppendContextCurrent(latestContext, {
          channelId: requestChannelId,
          keyword,
          vendor,
        })
      ) {
        return
      }

      const currentModels = parseModelsString(form.getValues('models') || '')
      const modelsToAdd = getMissingModelSearchMatches(
        allSearchModelNames,
        currentModels
      )

      if (modelsToAdd.length === 0) {
        toast.info(t('No new search results to add'))
        return
      }

      // 点击搜索补齐按钮时，模型 Combobox 的弹层可能仍处于打开状态。
      // 这里始终读取 form 里的最新草稿后再合并，避免弹层关闭或旧闭包把新增模型覆盖回去。
      const count = updateModels(modelsToAdd, true)
      setModelSelectOpen(false)
      clearModelSearch()
      window.setTimeout(() => {
        toast.success(t('Added {{count}} model(s) from search', { count }))
      }, 0)
    } catch (error) {
      toast.error(getErrorMessage(error) || t('Refresh failed'))
    } finally {
      if (modelSearchAppendRequestSeqRef.current === requestSeq) {
        isAddingModelSearchMatchesRef.current = false
        setIsAddingModelSearchMatches(false)
      }
    }
  }, [
    canEditBasicFields,
    channelId,
    clearModelSearch,
    debouncedModelSearchKeyword,
    form,
    noPermissionMessage,
    t,
    updateModels,
  ])

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
      currentRow,
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
      noPermissionMessage,
      permissions.canSensitiveWrite,
      form,
      confirmMissingModelMappings,
      confirmStatusCodeRisk,
      submitChannelMutation,
      t,
    ]
  )

  // 关闭抽屉时同步重置表单状态，避免下一次新建渠道沿用上一次编辑残留字段。
  const handleOpenChange = useCallback(
    (v: boolean) => {
      onOpenChange(v)
      if (!v) {
        modelSearchAppendRequestSeqRef.current += 1
        isAddingModelSearchMatchesRef.current = false
        setIsAddingModelSearchMatches(false)
        form.reset(CHANNEL_FORM_DEFAULT_VALUES)
        advancedNavScrollPendingRef.current = false
        setActiveEditorSectionId(CHANNEL_EDITOR_SECTION_IDS.identity)
        setExpandedEditorNavItemId(undefined)
        setAdvancedSettingsOpen(false)
        setAdvancedCustomEditorOpen(false)
      }
    },
    [onOpenChange, form]
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
        <SheetContent className={sideDrawerContentClassName('sm:max-w-5xl')}>
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle className='flex items-center gap-3'>
              <span className='bg-muted flex size-9 shrink-0 items-center justify-center rounded-md'>
                <ChannelTypeLogo type={currentType} size={22} />
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
                  'grid gap-5 lg:grid-cols-[13rem_minmax(0,1fr)] lg:items-start',
                  isChannelDetailLoading && 'hidden'
                )}
              >
                <ChannelEditorNav
                  providerLogo={
                    <ChannelTypeLogo type={currentType} size={18} />
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
                              <FormLabel>{t('Type *')}</FormLabel>
                              <FormControl>
                                <div className='relative'>
                                  <span className='pointer-events-none absolute top-1/2 left-3 z-10 flex -translate-y-1/2'>
                                    <ChannelTypeLogo
                                      type={Number(field.value)}
                                      size={18}
                                    />
                                  </span>
                                  <Combobox
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

                      {currentType === 1 && !isGlobalAccountPoolMode && (
                        <FormField
                          control={form.control}
                          name='openai_organization'
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t('OpenAI Organization')}</FormLabel>
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
                                    placeholder={t('e.g., 2025-04-01-preview')}
                                    disabled={!canEditSensitiveFields}
                                    {...field}
                                  />
                                </FormControl>
                                <FormDescription>
                                  {t('Default API version for this channel')}
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
                                {t('Enter the complete URL, supports')} {'{'}
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
                              <FormLabel>{t('Model Version *')}</FormLabel>
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
                                <FormLabel>{t('Enterprise Account')}</FormLabel>
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
                                    label: t('AccessKey / SecretAccessKey'),
                                  },
                                  { value: 'api_key', label: t('API Key') },
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
                                <SelectContent alignItemWithTrigger={false}>
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
                              <FormLabel>{t('Knowledge Base ID *')}</FormLabel>
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
                                {t('API Base URL (Important: Not Chat API) *')}
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
                              {t('https://cloud.siliconflow.cn/i/hij0YNTZ')}
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
                                    { value: 'api_key', label: t('API Key') },
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
                                  <SelectContent alignItemWithTrigger={false}>
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
                                      toast.info(t('Please upload key file(s)'))
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
                                      toast.info(t('Please upload key file(s)'))
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
                                    {
                                      value: 'doubao-coding-plan',
                                      label: t('Doubao Coding Plan'),
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
                                    field.value ||
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
                                  <SelectContent alignItemWithTrigger={false}>
                                    <SelectGroup>
                                      <SelectItem value='https://ark.cn-beijing.volces.com'>
                                        {t('https://ark.cn-beijing.volces.com')}
                                      </SelectItem>
                                      <SelectItem value='https://ark.ap-southeast.bytepluses.com'>
                                        {t(
                                          'https://ark.ap-southeast.bytepluses.com'
                                        )}
                                      </SelectItem>
                                      <SelectItem value='doubao-coding-plan'>
                                        {t('Doubao Coding Plan')}
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
                                <FormLabel>{t('API Base URL *')}</FormLabel>
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
                                    placeholder={t(FIELD_PLACEHOLDERS.BASE_URL)}
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
                                    {hiddenAdvancedCustomRouteTypeCount > 0 && (
                                      <Badge
                                        variant='outline'
                                        title={advancedCustomRouteTypeTitle}
                                      >
                                        +{hiddenAdvancedCustomRouteTypeCount}
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
                              <FormLabel>{t('Credential Mode')}</FormLabel>
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
                                    form.setValue('multi_key_mode', 'single')
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
                                    form.setValue('multi_key_mode', 'single')
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
                                <SelectContent alignItemWithTrigger={false}>
                                  <SelectGroup>
                                    {credentialModeOptions.map((option) => (
                                      <SelectItem
                                        key={option.value}
                                        value={option.value}
                                      >
                                        {option.label}
                                      </SelectItem>
                                    ))}
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
                                    { value: 'polling', label: t('Polling') },
                                    { value: 'random', label: t('Random') },
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
                                  <SelectContent alignItemWithTrigger={false}>
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
                                <FormLabel>{t('Account Pool Group')}</FormLabel>
                                <Select
                                  items={accountPoolGroupOptions}
                                  onValueChange={(value) => {
                                    if (!canEditSensitiveFields) {
                                      toast.error(noPermissionMessage)
                                      return
                                    }
                                    field.onChange(Number(value))
                                  }}
                                  value={field.value ? String(field.value) : ''}
                                >
                                  <FormControl>
                                    <SelectTrigger
                                      disabled={!canEditSensitiveFields}
                                    >
                                      <SelectValue
                                        placeholder={t('Select account group')}
                                      />
                                    </SelectTrigger>
                                  </FormControl>
                                  <SelectContent alignItemWithTrigger={false}>
                                    <SelectGroup>
                                      {accountPoolGroupOptions.map((option) => (
                                        <SelectItem
                                          key={option.value}
                                          value={option.value}
                                        >
                                          {option.label}
                                        </SelectItem>
                                      ))}
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
                        {!isEditing && credentialMode === 'single_key' && (
                          <FormField
                            control={form.control}
                            name='multi_key_mode'
                            render={({ field }) => (
                              <FormItem>
                                <FormLabel>{t('Add Mode')}</FormLabel>
                                <Select
                                  items={addModeOptions.map((option) => ({
                                    value: option.value,
                                    label: t(option.label),
                                  }))}
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
                                  <SelectContent alignItemWithTrigger={false}>
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
                                  return t('Leave empty to keep existing key')
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
                                          disabled={!canEditSensitiveFields}
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
                                      onClick={handleRefreshCodexCredential}
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
                            form.setValue('key', key, { shouldDirty: true })
                          }}
                        />

                        {isEditing && isMultiKeyChannel && (
                          <FormField
                            control={form.control}
                            name='key_mode'
                            render={({ field }) => (
                              <FormItem>
                                <FormLabel>{t('Key Update Mode')}</FormLabel>
                                <Select
                                  items={[
                                    {
                                      value: 'append',
                                      label: t('Append to existing keys'),
                                    },
                                    {
                                      value: 'replace',
                                      label: t('Replace all existing keys'),
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
                                  <SelectContent alignItemWithTrigger={false}>
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

                        {!isEditing && multiKeyMode === 'multi_to_single' && (
                          <FormField
                            control={form.control}
                            name='multi_key_type'
                            render={({ field }) => (
                              <FormItem>
                                <FormLabel>{t('Multi-Key Strategy')}</FormLabel>
                                <Select
                                  items={[
                                    { value: 'random', label: t('Random') },
                                    { value: 'polling', label: t('Polling') },
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
                                  <SelectContent alignItemWithTrigger={false}>
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
                    </ChannelApiAccessSection>
                  </div>

                  {/* ── Models & Groups ── */}
                  <div
                    id={CHANNEL_EDITOR_SECTION_IDS.models}
                    className='scroll-mt-4'
                  >
                    <ChannelModelsSection>
                      <div className='space-y-5'>
                        <div className='border-border/60 bg-muted/10 rounded-lg border p-4'>
                          <FormField
                            control={form.control}
                            name='models'
                            render={() => (
                              <FormItem className='space-y-3'>
                                <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                                  <div className='space-y-1'>
                                    <FormLabel>{t('Models *')}</FormLabel>
                                    <FormDescription>
                                      {t(FIELD_DESCRIPTIONS.MODELS)}
                                    </FormDescription>
                                  </div>
                                  <div className='flex flex-wrap gap-2'>
                                    <Badge variant='outline' className='w-fit'>
                                      {t('Selected {{count}}', {
                                        count: currentModelsArray.length,
                                      })}
                                    </Badge>
                                    <Badge
                                      variant='secondary'
                                      className='w-fit'
                                    >
                                      {modelSearchVendor
                                        ? `${t('Vendor')}: ${modelSearchVendor}`
                                        : t('All Vendors')}
                                    </Badge>
                                  </div>
                                </div>
                                <FormControl>
                                  <MultiSelect
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
                                    disabled={!canEditBasicFields}
                                    emptyText={t('No matching models')}
                                    open={modelSelectOpen}
                                    onOpenChange={setModelSelectOpen}
                                    searchValue={modelSearchKeyword}
                                    onSearchChange={setModelSearchKeyword}
                                    onSearchSubmit={handleAddModelSearchMatches}
                                    submitSearchOnEnterWithMatches
                                    isLoading={
                                      isSearchingModelMeta ||
                                      isModelSearchDebouncing
                                    }
                                    loadingText={t(
                                      'Searching model metadata...'
                                    )}
                                    preserveSelectedOnEmptyRemovalKey
                                    contentFooter={
                                      trimmedModelSearchKeyword.length > 0 ? (
                                        <div className='flex flex-col gap-2'>
                                          {isSearchingModelMeta ||
                                          isModelSearchDebouncing ? (
                                            <span className='text-muted-foreground text-sm'>
                                              {t('Searching model metadata...')}
                                            </span>
                                          ) : shouldShowModelSearchAppend ? (
                                            <>
                                              <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                                                <span className='text-muted-foreground text-sm'>
                                                  {t(
                                                    '{{matched}} matched · {{addable}} new · {{existing}} already selected',
                                                    {
                                                      matched:
                                                        modelSearchAppendSummary.matchedCount,
                                                      addable:
                                                        modelSearchAppendSummary.addableCount,
                                                      existing:
                                                        modelSearchAppendSummary.existingCount,
                                                    }
                                                  )}
                                                </span>
                                                <Button
                                                  type='button'
                                                  variant='outline'
                                                  size='sm'
                                                  onClick={
                                                    handleAddModelSearchMatches
                                                  }
                                                  disabled={
                                                    !canEditBasicFields ||
                                                    isAddingModelSearchMatches ||
                                                    isSearchingModelMeta ||
                                                    isModelSearchDebouncing ||
                                                    !canRunModelSearchAppend
                                                  }
                                                  title={
                                                    canEditBasicFields
                                                      ? undefined
                                                      : noPermissionMessage
                                                  }
                                                >
                                                  {isAddingModelSearchMatches ? (
                                                    <Loader2
                                                      data-icon='inline-start'
                                                      className='animate-spin'
                                                    />
                                                  ) : (
                                                    <Plus data-icon='inline-start' />
                                                  )}
                                                  {modelSearchAddButtonLabel}
                                                </Button>
                                              </div>
                                              {unscannedModelSearchResultCount >
                                                0 && (
                                                <span className='text-muted-foreground text-xs'>
                                                  {t(
                                                    '{{count}} more result(s) will be checked when adding',
                                                    {
                                                      count:
                                                        unscannedModelSearchResultCount,
                                                    }
                                                  )}
                                                </span>
                                              )}
                                              {modelSearchAppendPlan.totalCount >
                                              0 ? (
                                                <span className='text-xs'>
                                                  <span className='break-all'>
                                                    {modelSearchMissingPreview.join(
                                                      ', '
                                                    )}
                                                  </span>
                                                  {modelSearchMissingOmittedCount >
                                                    0 && (
                                                    <span className='ml-1'>
                                                      {t(
                                                        '({{total}} total, {{omit}} omitted)',
                                                        {
                                                          total:
                                                            modelSearchAppendPlan.totalCount,
                                                          omit: modelSearchMissingOmittedCount,
                                                        }
                                                      )}
                                                    </span>
                                                  )}
                                                </span>
                                              ) : (
                                                <span className='text-muted-foreground text-xs'>
                                                  {t(
                                                    'No new search results to add'
                                                  )}
                                                </span>
                                              )}
                                            </>
                                          ) : (
                                            <span className='text-muted-foreground text-sm'>
                                              {t('No matching models')}
                                            </span>
                                          )}
                                        </div>
                                      ) : undefined
                                    }
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

                          <Separator className='my-4' />

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
                                  !canEditBasicFields || !basicModels.length
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
                                  !canEditBasicFields || !allModelsList.length
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
                                      !canEditBasicFields
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
                                    onClick={() => handleAddPrefillGroup(group)}
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
                                      <FormLabel className='mb-0'>
                                        {t('Model Mapping')}
                                      </FormLabel>
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
                                            {mappingPreviewPairs.map((pair) => (
                                              <div
                                                key={`${pair.source}-${pair.target}`}
                                                className='flex items-center gap-1'
                                              >
                                                <span>{pair.source}</span>
                                                <ArrowRight className='size-3.5 opacity-70' />
                                                <span>{pair.target}</span>
                                              </div>
                                            ))}
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
                                    value={field.value || ''}
                                    onChange={field.onChange}
                                    disabled={
                                      isSubmitting || !canEditBasicFields
                                    }
                                    sourceModelOptions={currentModelsArray}
                                    targetModelOptions={modelOptions.map(
                                      (option) => option.value
                                    )}
                                  />
                                </FormControl>
                                {modelMappingGuardrail.invalidJson && (
                                  <Alert variant='destructive' className='mt-3'>
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
                              <FormItem className='space-y-3'>
                                <div className='space-y-1'>
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
                                      selected={field.value}
                                      onChange={(values) => {
                                        if (!canEditBasicFields) {
                                          toast.error(noPermissionMessage)
                                          return
                                        }
                                        field.onChange(values)
                                      }}
                                      placeholder={t(FIELD_PLACEHOLDERS.GROUP)}
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
              render={<Button variant='outline' disabled={isSubmitting} />}
            >
              {t('Cancel')}
            </SheetClose>
            <Button
              form='channel-form'
              type='submit'
              disabled={
                isSubmitting || !canSubmitForm || isChannelDetailLoading
              }
              title={canSubmitForm ? undefined : noPermissionMessage}
            >
              {isSubmitting && (
                <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              )}
              {isEditing ? t('Update Channel') : t('Save changes')}
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
