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
import { useEffect, useState, useCallback } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
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
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
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
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { JsonEditor } from '@/components/json-editor'
import { TagInput } from '@/components/tag-input'
import {
  combineBillingExpr,
  splitBillingExprAndRequestRules,
} from '@/features/pricing/lib/billing-expr'
import { formatPricingNumber } from '@/features/system-settings/models/pricing-format'
import { TieredPricingEditor } from '@/features/system-settings/models/tiered-pricing-editor'
import {
  createModel,
  updateModel,
  getModel,
  getVendors,
  getModelPricing,
  updateModelPricing,
} from '../../api'
import { getNameRuleOptions, ENDPOINT_TEMPLATES } from '../../constants'
import { useModelPermissions } from '../../hooks/use-model-permissions'
import { modelsQueryKeys, vendorsQueryKeys, parseModelTags } from '../../lib'
import type { Model, UpdateModelPricingRequest } from '../../types'

// 扩展模型表单 schema，仅用于抽屉内部承载定价编辑状态。
const extendedModelFormSchema = z.object({
  id: z.number().optional(),
  model_name: z.string().min(1, 'Model name is required'),
  description: z.string(),
  icon: z.string(),
  tags: z.array(z.string()),
  vendor_id: z.number().optional(),
  endpoints: z.string(),
  name_rule: z.number(),
  status: z.boolean(),
  sync_official: z.boolean(),
  price: z.string().optional(),
  ratio: z.string().optional(),
  cacheRatio: z.string().optional(),
  createCacheRatio: z.string().optional(),
  completionRatio: z.string().optional(),
  imageRatio: z.string().optional(),
  audioRatio: z.string().optional(),
  audioCompletionRatio: z.string().optional(),
})

type ExtendedModelFormValues = z.infer<typeof extendedModelFormSchema>

type PricingMode = 'per-token' | 'per-request' | 'tiered_expr'
type PriceLaneKey =
  | 'completion'
  | 'cache'
  | 'createCache'
  | 'image'
  | 'audioInput'
  | 'audioOutput'
type PricingRatioField =
  | 'completionRatio'
  | 'cacheRatio'
  | 'createCacheRatio'
  | 'imageRatio'
  | 'audioRatio'
  | 'audioCompletionRatio'

const numericDraftRegex = /^(\d+(\.\d*)?|\.\d*)?$/

const EMPTY_LANE_ENABLED: Record<PriceLaneKey, boolean> = {
  completion: false,
  cache: false,
  createCache: false,
  image: false,
  audioInput: false,
  audioOutput: false,
}

const ratioFieldByLane: Record<PriceLaneKey, PricingRatioField> = {
  completion: 'completionRatio',
  cache: 'cacheRatio',
  createCache: 'createCacheRatio',
  image: 'imageRatio',
  audioInput: 'audioRatio',
  audioOutput: 'audioCompletionRatio',
}

const priceLaneConfigs: Array<{
  key: PriceLaneKey
  titleKey: string
  descriptionKey: string
  placeholder: string
}> = [
  {
    key: 'completion',
    titleKey: 'Completion price',
    descriptionKey: 'Output token price for generated tokens.',
    placeholder: '15',
  },
  {
    key: 'cache',
    titleKey: 'Cache read price',
    descriptionKey: 'Token price for cache reads.',
    placeholder: '0.3',
  },
  {
    key: 'createCache',
    titleKey: 'Cache write price',
    descriptionKey: 'Token price for creating cache entries.',
    placeholder: '3.75',
  },
  {
    key: 'image',
    titleKey: 'Image input price',
    descriptionKey: 'Token price for image input.',
    placeholder: '2.5',
  },
  {
    key: 'audioInput',
    titleKey: 'Audio input price',
    descriptionKey: 'Token price for audio input.',
    placeholder: '3.81',
  },
  {
    key: 'audioOutput',
    titleKey: 'Audio output price',
    descriptionKey: 'Token price for audio output.',
    placeholder: '15.11',
  },
]

function hasPricingValue(value: unknown): boolean {
  return value !== '' && value !== null && value !== undefined
}

function toNumberOrNull(value: unknown): number | null {
  if (!hasPricingValue(value) && value !== 0) return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

function deriveLanePrice(
  ratio: unknown,
  denominator: unknown,
  fallback = ''
): string {
  const ratioNumber = toNumberOrNull(ratio)
  const denominatorNumber = toNumberOrNull(denominator)
  if (ratioNumber === null || denominatorNumber === null) return fallback
  return formatPricingNumber(ratioNumber * denominatorNumber)
}

type ModelMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: Model | null
}

export function ModelMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: ModelMutateDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const permissions = useModelPermissions()
  const noPermissionMessage = t("You don't have necessary permission")
  const isEditing = Boolean(currentRow?.id)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [pricingMode, setPricingMode] = useState<PricingMode>('per-token')
  const [laneEnabled, setLaneEnabled] =
    useState<Record<PriceLaneKey, boolean>>(EMPTY_LANE_ENABLED)
  const [promptPrice, setPromptPrice] = useState('')
  const [completionPrice, setCompletionPrice] = useState('')
  const [billingExpr, setBillingExpr] = useState('')
  const [requestRuleExpr, setRequestRuleExpr] = useState('')

  // 加载供应商列表，用于模型基础信息中的下拉选择。
  const { data: vendorsData } = useQuery({
    queryKey: vendorsQueryKeys.list(),
    queryFn: () => getVendors({ page_size: 1000 }),
    enabled: open,
  })

  const vendors = vendorsData?.data?.items || []

  // 编辑模式下加载模型详情，避免列表字段不完整导致保存覆盖。
  const { data: modelData } = useQuery({
    queryKey: modelsQueryKeys.detail(currentRow?.id || 0),
    queryFn: () => getModel(currentRow!.id),
    enabled: open && isEditing,
  })

  const { data: pricingData } = useQuery({
    queryKey: modelsQueryKeys.pricing(currentRow?.id || 0),
    queryFn: () => getModelPricing(currentRow!.id),
    enabled: open && isEditing,
  })

  const form = useForm<ExtendedModelFormValues>({
    resolver: zodResolver(extendedModelFormSchema),
    defaultValues: {
      model_name: '',
      description: '',
      icon: '',
      tags: [],
      vendor_id: undefined,
      endpoints: '',
      name_rule: 0,
      status: true,
      sync_official: true,
      price: '',
      ratio: '',
      cacheRatio: '',
      createCacheRatio: '',
      completionRatio: '',
      imageRatio: '',
      audioRatio: '',
      audioCompletionRatio: '',
    },
  })

  const validateNumber = (value: string) => numericDraftRegex.test(value)

  const valueToNumber = (value?: string): number | undefined => {
    if (value === undefined || value.trim() === '') return undefined
    const parsed = Number(value)
    return Number.isFinite(parsed) && parsed >= 0 ? parsed : undefined
  }

  const isValidNumberValue = (value?: string): boolean => {
    if (value === undefined || value.trim() === '') return false
    const parsed = Number(value)
    return Number.isFinite(parsed) && parsed >= 0
  }

  const numberToString = (value?: number): string =>
    value === undefined || value === null ? '' : String(value)

  const setFormPricingValue = useCallback(
    (name: keyof ExtendedModelFormValues, value: string) => {
      form.setValue(name, value as never, {
        shouldDirty: true,
        shouldValidate: true,
      })
    },
    [form]
  )

  const getLanePrice = useCallback(
    (lane: PriceLaneKey) => {
      if (lane === 'completion') return completionPrice

      switch (lane) {
        case 'cache':
          return deriveLanePrice(form.getValues('cacheRatio'), promptPrice)
        case 'createCache':
          return deriveLanePrice(
            form.getValues('createCacheRatio'),
            promptPrice
          )
        case 'image':
          return deriveLanePrice(form.getValues('imageRatio'), promptPrice)
        case 'audioInput':
          return deriveLanePrice(form.getValues('audioRatio'), promptPrice)
        case 'audioOutput': {
          const audioInputPrice = deriveLanePrice(
            form.getValues('audioRatio'),
            promptPrice
          )
          return deriveLanePrice(
            form.getValues('audioCompletionRatio'),
            audioInputPrice
          )
        }
      }
    },
    [completionPrice, form, promptPrice]
  )

  const deriveLaneRatio = useCallback(
    (
      lane: PriceLaneKey,
      price: string,
      nextPromptPrice = promptPrice,
      nextAudioInputPrice?: string
    ) => {
      const priceNumber = toNumberOrNull(price)
      if (priceNumber === null) return ''

      if (lane === 'audioOutput') {
        const audioInputPrice = toNumberOrNull(
          nextAudioInputPrice ?? getLanePrice('audioInput')
        )
        if (audioInputPrice === null || audioInputPrice === 0) return ''
        return formatPricingNumber(priceNumber / audioInputPrice)
      }

      const inputPrice = toNumberOrNull(nextPromptPrice)
      if (inputPrice === null || inputPrice === 0) return ''
      return formatPricingNumber(priceNumber / inputPrice)
    },
    [getLanePrice, promptPrice]
  )

  const syncLaneRatios = useCallback(
    (inputPrice: string) => {
      const parsedInputPrice = toNumberOrNull(inputPrice)
      setFormPricingValue(
        'ratio',
        parsedInputPrice !== null
          ? formatPricingNumber(parsedInputPrice / 2)
          : ''
      )

      priceLaneConfigs.forEach(({ key }) => {
        if (!laneEnabled[key]) {
          setFormPricingValue(ratioFieldByLane[key], '')
          return
        }
        const price = key === 'completion' ? completionPrice : getLanePrice(key)
        setFormPricingValue(
          ratioFieldByLane[key],
          deriveLaneRatio(key, price, inputPrice, getLanePrice('audioInput'))
        )
      })
    },
    [
      completionPrice,
      deriveLaneRatio,
      getLanePrice,
      laneEnabled,
      setFormPricingValue,
    ]
  )

  const handlePromptPriceChange = (value: string) => {
    if (!validateNumber(value)) return
    setPromptPrice(value)
    syncLaneRatios(value)
  }

  const handleCompletionPriceChange = (value: string) => {
    if (!validateNumber(value)) return
    setCompletionPrice(value)
    if (laneEnabled.completion) {
      setFormPricingValue(
        'completionRatio',
        deriveLaneRatio('completion', value)
      )
    }
  }

  const handleLanePriceChange = (lane: PriceLaneKey, value: string) => {
    if (!validateNumber(value)) return
    if (lane === 'completion') {
      handleCompletionPriceChange(value)
      return
    }

    if (laneEnabled[lane]) {
      setFormPricingValue(ratioFieldByLane[lane], deriveLaneRatio(lane, value))
    }

    if (lane === 'audioInput' && laneEnabled.audioOutput) {
      setFormPricingValue(
        'audioCompletionRatio',
        deriveLaneRatio(
          'audioOutput',
          getLanePrice('audioOutput'),
          promptPrice,
          value
        )
      )
    }
  }

  const handleLaneToggle = (lane: PriceLaneKey, checked: boolean) => {
    const nextEnabled = { ...laneEnabled, [lane]: checked }
    if (!checked && lane === 'audioInput') {
      nextEnabled.audioOutput = false
      setFormPricingValue('audioCompletionRatio', '')
    }

    setLaneEnabled(nextEnabled)
    if (!checked) {
      setFormPricingValue(ratioFieldByLane[lane], '')
      return
    }

    const price = lane === 'completion' ? completionPrice : getLanePrice(lane)
    setFormPricingValue(ratioFieldByLane[lane], deriveLaneRatio(lane, price))
  }

  const optionalLaneNumber = (
    values: ExtendedModelFormValues,
    lane: PriceLaneKey
  ) =>
    laneEnabled[lane]
      ? valueToNumber(values[ratioFieldByLane[lane]])
      : undefined

  const buildRatioPricingPayload = useCallback(
    (values: ExtendedModelFormValues): UpdateModelPricingRequest => ({
      billing_mode: 'ratio',
      model_ratio: valueToNumber(values.ratio),
      input_price_per_million: valueToNumber(promptPrice),
      output_price_per_million: laneEnabled.completion
        ? valueToNumber(completionPrice)
        : undefined,
      completion_ratio: optionalLaneNumber(values, 'completion'),
      cache_ratio: optionalLaneNumber(values, 'cache'),
      create_cache_ratio: optionalLaneNumber(values, 'createCache'),
      image_ratio: optionalLaneNumber(values, 'image'),
      audio_ratio: optionalLaneNumber(values, 'audioInput'),
      audio_completion_ratio: optionalLaneNumber(values, 'audioOutput'),
    }),
    [completionPrice, laneEnabled, promptPrice, valueToNumber]
  )

  const validateTokenPricing = useCallback(() => {
    const inputPrice = toNumberOrNull(promptPrice)
    const hasEnabledDependentLane = priceLaneConfigs.some(
      ({ key }) => laneEnabled[key]
    )
    if (inputPrice === null && hasEnabledDependentLane) {
      form.setError('ratio', {
        message: t('Input price is required before saving dependent prices.'),
      })
      return false
    }

    if (
      laneEnabled.audioOutput &&
      (!laneEnabled.audioInput ||
        toNumberOrNull(getLanePrice('audioInput')) === null)
    ) {
      form.setError('audioRatio', {
        message: t('Audio output price requires an audio input price.'),
      })
      return false
    }

    return true
  }, [form, getLanePrice, laneEnabled, promptPrice, t])

  const buildPricingPayload = useCallback(
    (values: ExtendedModelFormValues): UpdateModelPricingRequest => {
      if (pricingMode === 'per-request') {
        return {
          billing_mode: 'fixed',
          model_price: valueToNumber(values.price) ?? 0,
        }
      }

      if (pricingMode === 'tiered_expr') {
        const combinedExpr =
          combineBillingExpr(billingExpr, requestRuleExpr) || billingExpr
        return {
          ...buildRatioPricingPayload(values),
          billing_mode: 'tiered_expr',
          billing_expr: combinedExpr,
          model_price: valueToNumber(values.price),
        }
      }

      return buildRatioPricingPayload(values)
    },
    [billingExpr, buildRatioPricingPayload, pricingMode, requestRuleExpr]
  )

  const handlePricingModeChange = (value: string) => {
    const nextMode = value as PricingMode
    setPricingMode(nextMode)
    if (nextMode === 'tiered_expr' && !billingExpr) {
      setBillingExpr('tier("base", p * 0 + c * 0)')
      setRequestRuleExpr('')
    }
  }

  // 编辑模式下同时等待模型详情和聚合定价，确保表单一次性重置到一致状态。
  useEffect(() => {
    if (open && isEditing && modelData?.data && pricingData?.data) {
      const model = modelData.data

      // 基础模型字段先统一归一化，后续按计费模式补充定价字段。
      const baseModelData = {
        id: model.id,
        model_name: model.model_name,
        description: model.description || '',
        icon: model.icon || '',
        tags: parseModelTags(model.tags),
        vendor_id: model.vendor_id,
        endpoints: model.endpoints || '',
        name_rule: model.name_rule || 0,
        status: model.status === 1,
        sync_official: model.sync_official === 1,
        price: '',
        ratio: '',
        cacheRatio: '',
        createCacheRatio: '',
        completionRatio: '',
        imageRatio: '',
        audioRatio: '',
        audioCompletionRatio: '',
      }

      const pricing = pricingData.data
      const effective = pricing.effective || {}
      const override = pricing.override || {}
      const nextLaneEnabled: Record<PriceLaneKey, boolean> = {
        completion:
          override.completion_ratio !== undefined ||
          override.output_price_per_million !== undefined,
        cache: override.cache_ratio !== undefined,
        createCache: override.create_cache_ratio !== undefined,
        image: override.image_ratio !== undefined,
        audioInput: override.audio_ratio !== undefined,
        audioOutput: override.audio_completion_ratio !== undefined,
      }
      const split = splitBillingExprAndRequestRules(
        effective.billing_expr || ''
      )
      setBillingExpr(split.billingExpr || '')
      setRequestRuleExpr(split.requestRuleExpr || '')
      setLaneEnabled(nextLaneEnabled)

      if (pricing.billing_mode === 'fixed') {
        setPricingMode('per-request')
        setPromptPrice('')
        setCompletionPrice('')
        form.reset({
          ...baseModelData,
          price: numberToString(effective.model_price),
        })
      } else if (pricing.billing_mode === 'tiered_expr') {
        setPricingMode('tiered_expr')
        setPromptPrice(numberToString(effective.input_price_per_million))
        setCompletionPrice(numberToString(effective.output_price_per_million))
        form.reset({
          ...baseModelData,
          price: numberToString(effective.model_price),
          ratio: numberToString(effective.model_ratio),
          cacheRatio: numberToString(effective.cache_ratio),
          createCacheRatio: numberToString(effective.create_cache_ratio),
          completionRatio: numberToString(effective.completion_ratio),
          imageRatio: numberToString(effective.image_ratio),
          audioRatio: numberToString(effective.audio_ratio),
          audioCompletionRatio: numberToString(
            effective.audio_completion_ratio
          ),
        })
      } else {
        setPricingMode('per-token')
        setPromptPrice(numberToString(effective.input_price_per_million))
        setCompletionPrice(numberToString(effective.output_price_per_million))
        form.reset({
          ...baseModelData,
          ratio: numberToString(effective.model_ratio),
          cacheRatio: numberToString(effective.cache_ratio),
          createCacheRatio: numberToString(effective.create_cache_ratio),
          completionRatio: numberToString(effective.completion_ratio),
          imageRatio: numberToString(effective.image_ratio),
          audioRatio: numberToString(effective.audio_ratio),
          audioCompletionRatio: numberToString(
            effective.audio_completion_ratio
          ),
        })
      }
    } else if (open && !isEditing) {
      // 从缺失模型入口新建时，保留已传入的模型名称以减少重复输入。
      setPricingMode('per-token')
      setPromptPrice('')
      setCompletionPrice('')
      setBillingExpr('')
      setRequestRuleExpr('')
      setLaneEnabled({ ...EMPTY_LANE_ENABLED })
      form.reset({
        model_name: currentRow?.model_name || '',
        description: '',
        icon: '',
        tags: [],
        vendor_id: undefined,
        endpoints: '',
        name_rule: 0,
        status: true,
        sync_official: true,
        price: '',
        ratio: '',
        cacheRatio: '',
        createCacheRatio: '',
        completionRatio: '',
        imageRatio: '',
        audioRatio: '',
        audioCompletionRatio: '',
      })
    }
  }, [open, isEditing, modelData, pricingData, currentRow, form])

  const onSubmit = useCallback(
    async (values: ExtendedModelFormValues): Promise<void> => {
      if (!permissions.canWrite) {
        toast.error(noPermissionMessage)
        return
      }
      setIsSubmitting(true)
      try {
        if (
          pricingMode === 'per-request' &&
          !isValidNumberValue(values.price)
        ) {
          form.setError('price', {
            message: t('Fixed price is required.'),
          })
          return
        }
        if (pricingMode === 'tiered_expr' && !billingExpr.trim()) {
          toast.error(t('Billing expression is required.'))
          return
        }
        if (
          (pricingMode === 'per-token' || pricingMode === 'tiered_expr') &&
          !validateTokenPricing()
        ) {
          return
        }

        const submitData = {
          ...values,
          id: isEditing ? currentRow!.id : undefined,
          tags: Array.isArray(values.tags) ? values.tags.join(',') : '',
          status: values.status ? 1 : 0,
          sync_official: values.sync_official ? 1 : 0,
        }

        // 定价字段通过模型级 pricing API 保存，不混入模型元数据更新。
        const {
          price,
          ratio,
          cacheRatio,
          createCacheRatio,
          completionRatio,
          imageRatio,
          audioRatio,
          audioCompletionRatio,
          ...modelData
        } = submitData

        const response = isEditing
          ? await updateModel({ ...modelData, id: currentRow!.id })
          : await createModel(modelData)

        if (response.success) {
          const savedModel = response.data
          const modelId = isEditing ? currentRow!.id : savedModel?.id
          if (!modelId) {
            toast.error(t('Model saved but pricing could not be updated.'))
            return
          }

          const pricingResponse = await updateModelPricing(
            modelId,
            buildPricingPayload(values)
          )
          if (!pricingResponse.success) {
            toast.error(
              pricingResponse.message || t('Failed to update pricing')
            )
            return
          }

          toast.success(
            isEditing
              ? t('Model updated successfully')
              : t('Model created successfully')
          )
          queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
          queryClient.invalidateQueries({
            queryKey: modelsQueryKeys.detail(modelId),
          })
          queryClient.invalidateQueries({
            queryKey: modelsQueryKeys.pricing(modelId),
          })
          queryClient.invalidateQueries({ queryKey: ['system-options'] })
          queryClient.invalidateQueries({ queryKey: ['pricing'] })
          onOpenChange(false)
        } else {
          toast.error(response.message || t('Operation failed'))
        }
      } catch (error: unknown) {
        toast.error((error as Error)?.message || t('Operation failed'))
      } finally {
        setIsSubmitting(false)
      }
    },
    [
      isEditing,
      currentRow,
      form,
      pricingMode,
      billingExpr,
      queryClient,
      onOpenChange,
      t,
      buildPricingPayload,
      validateTokenPricing,
      permissions.canWrite,
      noPermissionMessage,
    ]
  )

  const handleFillEndpointTemplate = (templateKey: string) => {
    const template = ENDPOINT_TEMPLATES[templateKey]
    if (template) {
      const templateJson = JSON.stringify({ [templateKey]: template }, null, 2)
      form.setValue('endpoints', templateJson)
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='flex h-dvh w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl'>
        <SheetHeader className='border-b px-4 py-3 text-start sm:px-6 sm:py-4'>
          <SheetTitle>
            {isEditing ? t('Edit Model') : t('Create Model')}
          </SheetTitle>
          <SheetDescription>
            {isEditing
              ? t("Update model configuration and click save when you're done.")
              : t(
                  'Add a new model to the system by providing the necessary information.'
                )}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='model-form'
            onSubmit={form.handleSubmit(
              onSubmit as Parameters<typeof form.handleSubmit>[0]
            )}
            className='flex-1 space-y-4 overflow-y-auto px-3 py-3 pb-4 sm:space-y-6 sm:px-4'
          >
            {/* 基础信息 */}
            <div className='space-y-4'>
              <h3 className='text-sm font-semibold'>
                {t('Basic Information')}
              </h3>

              <FormField
                control={form.control}
                name='model_name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Model Name *')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('gpt-4, claude-3-opus, etc.')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('The unique identifier for this model')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='description'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Description')}</FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder={t('Describe this model...')}
                        rows={3}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='icon'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Icon')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('OpenAI, Anthropic, etc.')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription className='text-xs'>
                      {t('@lobehub/icons key')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='vendor_id'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Vendor')}</FormLabel>
                    <Select
                      items={[
                        ...vendors.map((vendor) => ({
                          value: String(vendor.id),
                          label: vendor.name,
                        })),
                      ]}
                      onValueChange={(value) =>
                        field.onChange(value ? parseInt(value) : undefined)
                      }
                      value={field.value ? String(field.value) : undefined}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder={t('Select vendor')} />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {vendors.map((vendor) => (
                            <SelectItem
                              key={vendor.id}
                              value={String(vendor.id)}
                            >
                              {vendor.name}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='tags'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Tags')}</FormLabel>
                    <FormControl>
                      <TagInput
                        value={field.value || []}
                        onChange={field.onChange}
                        placeholder={t('Add tags...')}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Press Enter or comma to add tags')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <Separator />

            {/* 匹配配置 */}
            <div className='space-y-4'>
              <h3 className='text-sm font-semibold'>{t('Matching Rules')}</h3>

              <FormField
                control={form.control}
                name='name_rule'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Name Rule')}</FormLabel>
                    <FormControl>
                      <RadioGroup
                        onValueChange={(value) =>
                          field.onChange(parseInt(value))
                        }
                        value={String(field.value)}
                        className='grid grid-cols-2 gap-4'
                      >
                        {getNameRuleOptions(t).map((option) => (
                          <div
                            key={option.value}
                            className='flex items-center space-x-2'
                          >
                            <RadioGroupItem
                              value={String(option.value)}
                              id={`rule-${option.value}`}
                            />
                            <Label
                              htmlFor={`rule-${option.value}`}
                              className='cursor-pointer font-normal'
                            >
                              {option.label}
                            </Label>
                          </div>
                        ))}
                      </RadioGroup>
                    </FormControl>
                    <FormDescription>
                      {t('How this model name should match requests')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <Separator />

            {/* 端点配置 */}
            <div className='space-y-4'>
              <div className='flex items-center justify-between'>
                <h3 className='text-sm font-semibold'>{t('Endpoints')}</h3>
                <Select<string>
                  items={[
                    ...Object.keys(ENDPOINT_TEMPLATES).map((key) => ({
                      value: key,
                      label: key,
                    })),
                  ]}
                  onValueChange={(v) =>
                    v !== null && handleFillEndpointTemplate(v)
                  }
                >
                  <SelectTrigger size='sm' className='w-[200px]'>
                    <SelectValue placeholder={t('Load template...')} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {Object.keys(ENDPOINT_TEMPLATES).map((key) => (
                        <SelectItem key={key} value={key}>
                          {key}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>

              <FormField
                control={form.control}
                name='endpoints'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Endpoint Configuration')}</FormLabel>
                    <FormControl>
                      <JsonEditor
                        value={field.value || ''}
                        onChange={field.onChange}
                        keyPlaceholder='endpoint_type'
                        valuePlaceholder='{"path": "/v1/...", "method": "POST"}'
                        keyLabel='Endpoint Type'
                        valueLabel='Configuration'
                        valueType='any'
                        emptyMessage={t(
                          'No endpoints configured. Switch to JSON mode or add rows to define endpoints.'
                        )}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Define API endpoints for this model (JSON format)')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <Separator />

            {/* 定价配置 */}
            <div className='flex flex-col gap-4'>
              <h3 className='text-sm font-semibold'>
                {t('Pricing Configuration')}
              </h3>

              <Tabs value={pricingMode} onValueChange={handlePricingModeChange}>
                <TabsList className='grid w-full grid-cols-3'>
                  <TabsTrigger value='per-token'>{t('Per-token')}</TabsTrigger>
                  <TabsTrigger value='per-request'>
                    {t('Per-request')}
                  </TabsTrigger>
                  <TabsTrigger value='tiered_expr'>
                    {t('Expression')}
                  </TabsTrigger>
                </TabsList>

                <TabsContent value='per-token' className='flex flex-col gap-4'>
                  <FieldGroup className='gap-4'>
                    <Field>
                      <FieldLabel>{t('Input price')}</FieldLabel>
                      <PriceInput
                        value={promptPrice}
                        placeholder='3'
                        onChange={handlePromptPriceChange}
                      />
                      <FieldDescription>
                        {t('USD price per 1M input tokens.')}
                      </FieldDescription>
                    </Field>

                    <div className='grid gap-3 sm:grid-cols-2'>
                      {priceLaneConfigs.map((lane) => {
                        const audioOutputDisabled =
                          lane.key === 'audioOutput' &&
                          (!laneEnabled.audioInput ||
                            toNumberOrNull(getLanePrice('audioInput')) === null)

                        return (
                          <PriceLaneCard
                            key={lane.key}
                            title={t(lane.titleKey)}
                            description={t(lane.descriptionKey)}
                            placeholder={lane.placeholder}
                            value={
                              lane.key === 'completion'
                                ? completionPrice
                                : getLanePrice(lane.key)
                            }
                            enabled={laneEnabled[lane.key]}
                            disabled={audioOutputDisabled}
                            onEnabledChange={(checked) =>
                              handleLaneToggle(lane.key, checked)
                            }
                            onChange={(value) =>
                              handleLanePriceChange(lane.key, value)
                            }
                          />
                        )
                      })}
                    </div>
                  </FieldGroup>
                </TabsContent>

                <TabsContent
                  value='per-request'
                  className='flex flex-col gap-4'
                >
                  <FormField
                    control={form.control}
                    name='price'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Fixed price')}</FormLabel>
                        <FormControl>
                          <PriceInput
                            value={field.value || ''}
                            placeholder='0.01'
                            suffix={t('per request')}
                            onChange={(value) => {
                              if (validateNumber(value)) {
                                field.onChange(value)
                              }
                            }}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Cost in USD per request, regardless of tokens used.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </TabsContent>

                <TabsContent
                  value='tiered_expr'
                  className='flex flex-col gap-4'
                >
                  <TieredPricingEditor
                    modelName={form.watch('model_name')}
                    billingExpr={billingExpr}
                    requestRuleExpr={requestRuleExpr}
                    onBillingExprChange={setBillingExpr}
                    onRequestRuleExprChange={setRequestRuleExpr}
                  />
                  <FormDescription>
                    {t(
                      'Expression prices use real USD per 1M tokens and are evaluated by the billing expression engine.'
                    )}
                  </FormDescription>
                </TabsContent>
              </Tabs>
            </div>

            <Separator />

            {/* 状态与同步 */}
            <div className='space-y-4'>
              <h3 className='text-sm font-semibold'>{t('Status & Sync')}</h3>

              <FormField
                control={form.control}
                name='status'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                    <div className='space-y-0.5'>
                      <FormLabel className='text-base'>
                        {t('Enabled')}
                      </FormLabel>
                      <FormDescription>
                        {t('Enable or disable this model')}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='sync_official'
                render={({ field }) => (
                  <FormItem className='flex items-center justify-between rounded-lg border p-4'>
                    <div className='space-y-0.5'>
                      <FormLabel className='text-base'>
                        {t('Official Sync')}
                      </FormLabel>
                      <FormDescription>
                        {t('Sync this model with official upstream')}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </div>
          </form>
        </Form>

        <SheetFooter className='grid grid-cols-2 gap-2 border-t px-4 py-3 sm:flex sm:px-6 sm:py-4'>
          <SheetClose
            render={<Button variant='outline' disabled={isSubmitting} />}
          >
            {t('Cancel')}
          </SheetClose>
          <Button
            form='model-form'
            type='submit'
            disabled={isSubmitting || !permissions.canWrite}
            title={permissions.canWrite ? undefined : noPermissionMessage}
          >
            {isSubmitting && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {isEditing ? t('Update Model') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

function PriceInput(props: {
  value: string
  placeholder?: string
  disabled?: boolean
  suffix?: string
  onChange: (value: string) => void
}) {
  return (
    <InputGroup>
      <InputGroupAddon>$</InputGroupAddon>
      <InputGroupInput
        inputMode='decimal'
        value={props.value}
        placeholder={props.placeholder}
        disabled={props.disabled}
        onChange={(event) => props.onChange(event.target.value)}
      />
      <InputGroupAddon align='inline-end'>
        {props.suffix || '$/1M'}
      </InputGroupAddon>
    </InputGroup>
  )
}

function PriceLaneCard(props: {
  title: string
  description: string
  placeholder: string
  value: string
  enabled: boolean
  disabled?: boolean
  onEnabledChange: (checked: boolean) => void
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  const effectiveDisabled = props.disabled || !props.enabled

  return (
    <Field
      className={cn(
        'rounded-lg border p-3',
        effectiveDisabled && 'bg-muted/35'
      )}
      data-disabled={effectiveDisabled || undefined}
    >
      <div className='flex items-start justify-between gap-3'>
        <FieldContent>
          <FieldTitle>{props.title}</FieldTitle>
          <FieldDescription>{props.description}</FieldDescription>
        </FieldContent>
        <Switch
          checked={props.enabled}
          disabled={props.disabled}
          onCheckedChange={props.onEnabledChange}
          aria-label={props.title}
        />
      </div>
      <PriceInput
        value={props.value}
        placeholder={props.placeholder}
        disabled={effectiveDisabled}
        onChange={props.onChange}
      />
      <FieldDescription>
        {props.enabled
          ? t('USD price per 1M tokens.')
          : t('Disabled lanes are omitted on save.')}
      </FieldDescription>
    </Field>
  )
}
