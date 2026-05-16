import { useEffect } from 'react'
import { type Resolver, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
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
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  createRequestRule,
  updateRequestRule,
  requestRulesQueryKeys,
} from '../api'
import {
  requestRuleFormSchema,
  RELAY_FORMAT_OPTIONS,
  RELAY_FORMAT_ALL_VALUE,
  MODEL_MATCH_MODE_OPTIONS,
  type RequestRule,
  type RequestRuleFormValues,
} from '../types'

type RuleFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  rule?: RequestRule | null
}

export function RuleFormDialog(props: RuleFormDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEditing = !!props.rule

  // 创建 mutation
  const createMutation = useMutation({
    mutationFn: (data: RequestRuleFormValues) => createRequestRule(data),
    onSuccess: (res) => {
      if (res.success) {
        toast.success(t('Rule created successfully'))
        queryClient.invalidateQueries({ queryKey: requestRulesQueryKeys.all })
        props.onOpenChange(false)
      }
    },
  })

  // 更新 mutation
  const updateMutation = useMutation({
    mutationFn: (data: Partial<RequestRule> & { id: number }) =>
      updateRequestRule(data),
    onSuccess: (res) => {
      if (res.success) {
        toast.success(t('Rule updated successfully'))
        queryClient.invalidateQueries({ queryKey: requestRulesQueryKeys.all })
        props.onOpenChange(false)
      }
    },
  })

  const form = useForm<RequestRuleFormValues>({
    resolver: zodResolver(
      requestRuleFormSchema
    ) as unknown as Resolver<RequestRuleFormValues>,
    defaultValues: {
      name: '',
      description: '',
      status: 1,
      priority: 0,
      relay_format: RELAY_FORMAT_ALL_VALUE,
      model_pattern: '',
      model_match_mode: 0,
      param_override: '',
      header_override: '',
      log_request: false,
      log_response: false,
      log_max_size: 4096,
    },
  })

  // 打开对话框时重置表单
  useEffect(() => {
    if (props.open && props.rule) {
      // 编辑模式：加载已有数据
      form.reset({
        name: props.rule.name,
        description: props.rule.description || '',
        status: props.rule.status,
        priority: props.rule.priority,
        relay_format: props.rule.relay_format || RELAY_FORMAT_ALL_VALUE,
        model_pattern: props.rule.model_pattern,
        model_match_mode: props.rule.model_match_mode,
        param_override: props.rule.param_override || '',
        header_override: props.rule.header_override || '',
        log_request: props.rule.log_request,
        log_response: props.rule.log_response,
        log_max_size: props.rule.log_max_size,
      })
    } else if (props.open && !props.rule) {
      // 创建模式：重置为默认值
      form.reset({
        name: '',
        description: '',
        status: 1,
        priority: 0,
        relay_format: RELAY_FORMAT_ALL_VALUE,
        model_pattern: '',
        model_match_mode: 0,
        param_override: '',
        header_override: '',
        log_request: false,
        log_response: false,
        log_max_size: 4096,
      })
    }
  }, [props.open, props.rule, form])

  const onSubmit = async (values: RequestRuleFormValues) => {
    // 将 'all' 转换回空字符串，空字符串的 JSON 字段转为 null
    const payload = {
      ...values,
      relay_format:
        values.relay_format === RELAY_FORMAT_ALL_VALUE
          ? ''
          : values.relay_format,
      param_override: values.param_override || null,
      header_override: values.header_override || null,
    }

    if (isEditing && props.rule) {
      await updateMutation.mutateAsync({
        id: props.rule.id,
        ...payload,
      })
    } else {
      await createMutation.mutateAsync(payload)
    }
  }

  const isPending = createMutation.isPending || updateMutation.isPending

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[85vh] overflow-y-auto sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>
            {isEditing ? t('Edit Rule') : t('Create Rule')}
          </DialogTitle>
          <DialogDescription>
            {isEditing
              ? t('Update the configuration for this request rule.')
              : t('Create a new request rule to manage API request behavior.')}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-4'>
            {/* 基础信息 */}
            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Name')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('Rule name')} {...field} />
                  </FormControl>
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
                    <Input
                      placeholder={t('Optional description')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
              {/* 优先级 */}
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
                        {...field}
                        onChange={(e) => field.onChange(Number(e.target.value))}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Higher value = higher priority')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* 状态 */}
              <FormField
                control={form.control}
                name='status'
                render={({ field }) => (
                  <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                    <div className='space-y-0.5'>
                      <FormLabel>{t('Enabled')}</FormLabel>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value === 1}
                        onCheckedChange={(checked) =>
                          field.onChange(checked ? 1 : 0)
                        }
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </div>

            {/* 匹配条件 */}
            <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
              {/* Relay Format */}
              <FormField
                control={form.control}
                name='relay_format'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Relay Format')}</FormLabel>
                    <Select
                      items={RELAY_FORMAT_OPTIONS.map((o) => ({
                        value: o.value,
                        label: t(o.labelKey),
                      }))}
                      value={field.value}
                      onValueChange={field.onChange}
                    >
                      <FormControl>
                        <SelectTrigger className='w-full'>
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {RELAY_FORMAT_OPTIONS.map((option) => (
                            <SelectItem
                              key={option.value}
                              value={option.value}
                            >
                              {t(option.labelKey)}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* 匹配模式 */}
              <FormField
                control={form.control}
                name='model_match_mode'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Match Mode')}</FormLabel>
                    <Select
                      items={MODEL_MATCH_MODE_OPTIONS.map((o) => ({
                        value: String(o.value),
                        label: t(o.labelKey),
                      }))}
                      value={String(field.value)}
                      onValueChange={(val) => field.onChange(Number(val))}
                    >
                      <FormControl>
                        <SelectTrigger className='w-full'>
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {MODEL_MATCH_MODE_OPTIONS.map((option) => (
                            <SelectItem
                              key={option.value}
                              value={String(option.value)}
                            >
                              {t(option.labelKey)}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            {/* 模型模式 */}
            <FormField
              control={form.control}
              name='model_pattern'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Model Pattern')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('e.g. gpt-4*, claude-*, leave empty for all')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Pattern to match model names. Empty matches all models.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Param Override */}
            <FormField
              control={form.control}
              name='param_override'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Param Override')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={t('JSON parameter override, e.g. {"temperature": 0.7}')}
                      className='min-h-[80px] font-mono text-xs'
                      value={field.value ?? ''}
                      onChange={field.onChange}
                      onBlur={field.onBlur}
                      name={field.name}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('JSON format. Override request parameters sent to the upstream provider.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Header Override */}
            <FormField
              control={form.control}
              name='header_override'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Header Override')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={t('JSON header override, e.g. {"X-Custom": "value"}')}
                      className='min-h-[80px] font-mono text-xs'
                      value={field.value ?? ''}
                      onChange={field.onChange}
                      onBlur={field.onBlur}
                      name={field.name}
                      ref={field.ref}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('JSON format. Override request headers sent to the upstream provider.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* 日志设置 */}
            <div className='space-y-3'>
              <h4 className='text-sm font-medium'>{t('Logging')}</h4>

              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
                <FormField
                  control={form.control}
                  name='log_request'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                      <div className='space-y-0.5'>
                        <FormLabel>{t('Log Request')}</FormLabel>
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
                  name='log_response'
                  render={({ field }) => (
                    <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                      <div className='space-y-0.5'>
                        <FormLabel>{t('Log Response')}</FormLabel>
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

              <FormField
                control={form.control}
                name='log_max_size'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Max Log Size (bytes)')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        placeholder='4096'
                        {...field}
                        onChange={(e) => field.onChange(Number(e.target.value))}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('Maximum size of logged request/response body in bytes.')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <DialogFooter>
              <Button
                type='button'
                variant='outline'
                onClick={() => props.onOpenChange(false)}
                disabled={isPending}
              >
                {t('Cancel')}
              </Button>
              <Button type='submit' disabled={isPending}>
                {isPending
                  ? t('Saving...')
                  : isEditing
                    ? t('Update Rule')
                    : t('Create Rule')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
