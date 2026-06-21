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
import { ChevronDown, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
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
import { Textarea } from '@/components/ui/textarea'
import { JsonEditor } from '@/components/json-editor'
import { TagInput } from '@/components/tag-input'
import {
  combineBillingExpr,
  splitBillingExprAndRequestRules,
} from '@/features/pricing/lib/billing-expr'
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
type PricingSubMode = 'ratio' | 'price'

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
  const isEditing = Boolean(currentRow?.id)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [pricingMode, setPricingMode] = useState<PricingMode>('per-token')
  const [pricingSubMode, setPricingSubMode] = useState<PricingSubMode>('ratio')
  const [advancedOpen, setAdvancedOpen] = useState(false)
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

  const validateNumber = (value: string) => {
    if (value === '') return true
    const parsed = Number(value)
    return Number.isFinite(parsed) && parsed >= 0
  }

  const valueToNumber = (value?: string): number | undefined => {
    if (value === undefined || value.trim() === '') return undefined
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : undefined
  }

  const isValidNumberValue = (value?: string): boolean => {
    if (value === undefined || value.trim() === '') return false
    const parsed = Number(value)
    return Number.isFinite(parsed) && parsed >= 0
  }

  const numberToString = (value?: number): string =>
    value === undefined || value === null ? '' : String(value)

  const handlePromptPriceChange = (value: string) => {
    setPromptPrice(value)
    if (value && !isNaN(parseFloat(value))) {
      const ratio = parseFloat(value) / 2
      form.setValue('ratio', ratio.toString())
    } else {
      form.setValue('ratio', '')
    }
  }

  const handleCompletionPriceChange = (value: string) => {
    setCompletionPrice(value)
    if (
      value &&
      !isNaN(parseFloat(value)) &&
      promptPrice &&
      !isNaN(parseFloat(promptPrice)) &&
      parseFloat(promptPrice) > 0
    ) {
      const completionRatio = parseFloat(value) / parseFloat(promptPrice)
      form.setValue('completionRatio', completionRatio.toString())
    } else {
      form.setValue('completionRatio', '')
    }
  }

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
          billing_mode: 'tiered_expr',
          billing_expr: combinedExpr,
          model_price: valueToNumber(values.price),
          model_ratio: valueToNumber(values.ratio),
          input_price_per_million: valueToNumber(promptPrice),
          output_price_per_million: valueToNumber(completionPrice),
          completion_ratio: valueToNumber(values.completionRatio),
          cache_ratio: valueToNumber(values.cacheRatio),
          create_cache_ratio: valueToNumber(values.createCacheRatio),
          image_ratio: valueToNumber(values.imageRatio),
          audio_ratio: valueToNumber(values.audioRatio),
          audio_completion_ratio: valueToNumber(values.audioCompletionRatio),
        }
      }

      return {
        billing_mode: 'ratio',
        model_ratio: valueToNumber(values.ratio),
        input_price_per_million: valueToNumber(promptPrice),
        output_price_per_million: valueToNumber(completionPrice),
        completion_ratio: valueToNumber(values.completionRatio),
        cache_ratio: valueToNumber(values.cacheRatio),
        create_cache_ratio: valueToNumber(values.createCacheRatio),
        image_ratio: valueToNumber(values.imageRatio),
        audio_ratio: valueToNumber(values.audioRatio),
        audio_completion_ratio: valueToNumber(values.audioCompletionRatio),
      }
    },
    [billingExpr, completionPrice, pricingMode, promptPrice, requestRuleExpr]
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
      const split = splitBillingExprAndRequestRules(
        effective.billing_expr || ''
      )
      setBillingExpr(split.billingExpr || '')
      setRequestRuleExpr(split.requestRuleExpr || '')

      if (pricing.billing_mode === 'fixed') {
        setPricingMode('per-request')
        setPricingSubMode('price')
        setPromptPrice('')
        setCompletionPrice('')
        form.reset({
          ...baseModelData,
          price: numberToString(effective.model_price),
        })
        setAdvancedOpen(false)
      } else if (pricing.billing_mode === 'tiered_expr') {
        setPricingMode('tiered_expr')
        setPricingSubMode('price')
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
        setAdvancedOpen(false)
      } else {
        setPricingMode('per-token')
        setPricingSubMode('price')
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
        setAdvancedOpen(
          !!(
            effective.cache_ratio !== undefined ||
            effective.create_cache_ratio !== undefined ||
            effective.image_ratio !== undefined ||
            effective.audio_ratio !== undefined ||
            effective.audio_completion_ratio !== undefined
          )
        )
      }
    } else if (open && !isEditing) {
      // 从缺失模型入口新建时，保留已传入的模型名称以减少重复输入。
      setPricingMode('per-token')
      setPricingSubMode('price')
      setPromptPrice('')
      setCompletionPrice('')
      setBillingExpr('')
      setRequestRuleExpr('')
      setAdvancedOpen(false)
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
            toast.error(pricingResponse.message || t('Failed to update pricing'))
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
            <div className='space-y-4'>
              <h3 className='text-sm font-semibold'>
                {t('Pricing Configuration')}
              </h3>

              <div className='space-y-4'>
                <Label>{t('Pricing mode')}</Label>
                <RadioGroup
                  value={pricingMode}
                  onValueChange={handlePricingModeChange}
                >
                  <div className='flex items-center space-x-2'>
                    <RadioGroupItem value='per-token' id='per-token' />
                    <Label htmlFor='per-token' className='font-normal'>
                      {t('Per-token (ratio based)')}
                    </Label>
                  </div>
                  <div className='flex items-center space-x-2'>
                    <RadioGroupItem value='per-request' id='per-request' />
                    <Label htmlFor='per-request' className='font-normal'>
                      {t('Per-request (fixed price)')}
                    </Label>
                  </div>
                  <div className='flex items-center space-x-2'>
                    <RadioGroupItem value='tiered_expr' id='tiered-expr' />
                    <Label htmlFor='tiered-expr' className='font-normal'>
                      {t('Expression pricing')}
                    </Label>
                  </div>
                </RadioGroup>
              </div>

              {pricingMode === 'per-request' ? (
                <FormField
                  control={form.control}
                  name='price'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Fixed price (USD)')}</FormLabel>
                      <FormControl>
                        <Input
                          type='text'
                          placeholder='0.01'
                          {...field}
                          onChange={(e) => {
                            const value = e.target.value
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
              ) : pricingMode === 'tiered_expr' ? (
                <div className='space-y-4'>
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
                </div>
              ) : (
                <>
                  <div className='space-y-4'>
                    <Label>{t('Input mode')}</Label>
                    <RadioGroup
                      value={pricingSubMode}
                      onValueChange={(value) =>
                        setPricingSubMode(value as PricingSubMode)
                      }
                    >
                      <div className='flex items-center space-x-2'>
                        <RadioGroupItem value='ratio' id='ratio' />
                        <Label htmlFor='ratio' className='font-normal'>
                          {t('Ratio mode')}
                        </Label>
                      </div>
                      <div className='flex items-center space-x-2'>
                        <RadioGroupItem value='price' id='price' />
                        <Label htmlFor='price' className='font-normal'>
                          {t('Price mode (USD per 1M tokens)')}
                        </Label>
                      </div>
                    </RadioGroup>
                  </div>

                  {pricingSubMode === 'ratio' ? (
                    <>
                      <FormField
                        control={form.control}
                        name='ratio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Model ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                    if (value) {
                                      setPromptPrice(
                                        (parseFloat(value) * 2).toString()
                                      )
                                    } else {
                                      setPromptPrice('')
                                    }
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {field.value && !isNaN(parseFloat(field.value))
                                ? t(
                                    'Calculated price: ${{price}} per 1M tokens',
                                    {
                                      price: (
                                        parseFloat(field.value) * 2
                                      ).toFixed(4),
                                    }
                                  )
                                : t('Multiplier for prompt tokens.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='completionRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Completion ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                    const ratio = form.getValues('ratio')
                                    if (value && ratio) {
                                      const compPrice =
                                        parseFloat(ratio) *
                                        2 *
                                        parseFloat(value)
                                      setCompletionPrice(compPrice.toString())
                                    } else {
                                      setCompletionPrice('')
                                    }
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {field.value &&
                              !isNaN(parseFloat(field.value)) &&
                              promptPrice &&
                              !isNaN(parseFloat(promptPrice))
                                ? t(
                                    'Calculated price: ${{price}} per 1M tokens',
                                    {
                                      price: (
                                        parseFloat(promptPrice) *
                                        parseFloat(field.value)
                                      ).toFixed(4),
                                    }
                                  )
                                : t('Multiplier for completion tokens.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </>
                  ) : (
                    <>
                      <div className='space-y-4'>
                        <div className='space-y-2'>
                          <Label>{t('Prompt price ($/1M tokens)')}</Label>
                          <Input
                            type='text'
                            placeholder='2.0'
                            value={promptPrice}
                            onChange={(e) =>
                              handlePromptPriceChange(e.target.value)
                            }
                          />
                          <p className='text-muted-foreground text-sm'>
                            {promptPrice && !isNaN(parseFloat(promptPrice))
                              ? t('Calculated ratio: {{ratio}}', {
                                  ratio: (
                                    parseFloat(promptPrice) / 2
                                  ).toFixed(4),
                                })
                              : t('Enter Input price to calculate ratio')}
                          </p>
                        </div>

                        <div className='space-y-2'>
                          <Label>{t('Completion price ($/1M tokens)')}</Label>
                          <Input
                            type='text'
                            placeholder='4.0'
                            value={completionPrice}
                            onChange={(e) =>
                              handleCompletionPriceChange(e.target.value)
                            }
                          />
                          <p className='text-muted-foreground text-sm'>
                            {completionPrice &&
                            !isNaN(parseFloat(completionPrice)) &&
                            promptPrice &&
                            !isNaN(parseFloat(promptPrice)) &&
                            parseFloat(promptPrice) > 0
                              ? t('Calculated ratio: {{ratio}}', {
                                  ratio: (
                                    parseFloat(completionPrice) /
                                    parseFloat(promptPrice)
                                  ).toFixed(4),
                                })
                              : t('Enter Completion price to calculate ratio')}
                          </p>
                        </div>
                      </div>
                    </>
                  )}

                  <Collapsible
                    open={advancedOpen}
                    onOpenChange={setAdvancedOpen}
                  >
                    <CollapsibleTrigger
                      render={
                        <Button
                          type='button'
                          variant='outline'
                          className='flex w-full items-center justify-between'
                        />
                      }
                    >
                      {t('Advanced options')}
                      <ChevronDown
                        className={`h-4 w-4 transition-transform duration-200 ${
                          advancedOpen ? 'rotate-180' : ''
                        }`}
                      />
                    </CollapsibleTrigger>
                    <CollapsibleContent className='space-y-6 pt-6'>
                      <FormField
                        control={form.control}
                        name='cacheRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Cache ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='0.1'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {t('Discount ratio for cache hits.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='createCacheRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Create cache ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.25'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {t('Multiplier for creating cache entries.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='imageRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Image ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {t('Multiplier for image processing.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='audioRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Audio ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {t('Multiplier for audio inputs.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='audioCompletionRatio'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Audio completion ratio')}</FormLabel>
                            <FormControl>
                              <Input
                                type='text'
                                placeholder='1.0'
                                {...field}
                                onChange={(e) => {
                                  const value = e.target.value
                                  if (validateNumber(value)) {
                                    field.onChange(value)
                                  }
                                }}
                              />
                            </FormControl>
                            <FormDescription>
                              {t('Multiplier for audio outputs.')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </CollapsibleContent>
                  </Collapsible>
                </>
              )}
            </div>

            <Separator />

            {/* Status & Sync */}
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
          <Button form='model-form' type='submit' disabled={isSubmitting}>
            {isSubmitting && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {isEditing ? t('Update Model') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
