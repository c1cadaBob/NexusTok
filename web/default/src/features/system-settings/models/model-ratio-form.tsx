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
import { memo, useCallback, useRef, useState } from 'react'
import { type UseFormReturn } from 'react-hook-form'
import { Link } from '@tanstack/react-router'
import { Code2, Eye } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { JsonCodeEditor } from '@/components/json-code-editor'
import {
  ModelRatioVisualEditor,
  type ModelRatioVisualEditorHandle,
} from './model-ratio-visual-editor'

type ModelFormValues = {
  ModelPrice: string
  ModelRatio: string
  CacheRatio: string
  CreateCacheRatio: string
  CompletionRatio: string
  ImageRatio: string
  AudioRatio: string
  AudioCompletionRatio: string
  ExposeRatioEnabled: boolean
  BillingMode: string
  BillingExpr: string
}

type ModelRatioFormProps = {
  form: UseFormReturn<ModelFormValues>
  onSave: (values: ModelFormValues) => Promise<void>
  onReset: () => void
  isSaving: boolean
  isResetting: boolean
  canSave: boolean
  canReset: boolean
  disabledReason: string
}

type ModelJsonFieldName =
  | 'ModelPrice'
  | 'ModelRatio'
  | 'CacheRatio'
  | 'CreateCacheRatio'
  | 'CompletionRatio'
  | 'ImageRatio'
  | 'AudioRatio'
  | 'AudioCompletionRatio'

const modelJsonFields: Array<{
  descriptionKey: string
  labelKey: string
  name: ModelJsonFieldName
  rows: number
}> = [
  {
    descriptionKey:
      'JSON map of model → USD cost per request. Takes precedence over ratio based billing.',
    labelKey: 'Model fixed pricing',
    name: 'ModelPrice',
    rows: 8,
  },
  {
    descriptionKey: 'JSON map of model → multiplier applied to quota billing.',
    labelKey: 'Model ratio',
    name: 'ModelRatio',
    rows: 8,
  },
  {
    descriptionKey: 'Optional ratio used when upstream cache hits occur.',
    labelKey: 'Prompt cache ratio',
    name: 'CacheRatio',
    rows: 8,
  },
  {
    descriptionKey:
      'Ratio applied when creating cache entries for supported models.',
    labelKey: 'Create cache ratio',
    name: 'CreateCacheRatio',
    rows: 8,
  },
  {
    descriptionKey:
      'Applies to custom completion endpoints. JSON map of model → ratio.',
    labelKey: 'Completion ratio',
    name: 'CompletionRatio',
    rows: 8,
  },
  {
    descriptionKey: 'Configure per-model ratio for image inputs or outputs.',
    labelKey: 'Image ratio',
    name: 'ImageRatio',
    rows: 6,
  },
  {
    descriptionKey:
      'Ratio applied to audio inputs where supported by the upstream model.',
    labelKey: 'Audio ratio',
    name: 'AudioRatio',
    rows: 6,
  },
  {
    descriptionKey: 'Ratio applied to audio completions for streaming models.',
    labelKey: 'Audio completion ratio',
    name: 'AudioCompletionRatio',
    rows: 6,
  },
]

/**
 * 渲染模型倍率 JSON 字段。
 *
 * 各字段仍然写回原 react-hook-form 字符串值；JSON 结构是否合法继续由
 * 表单 schema 和保存接口兜底，编辑器只提供行号、状态和格式化体验。
 */
function ModelJsonEditorField(props: {
  description: string
  form: UseFormReturn<ModelFormValues>
  label: string
  name: ModelJsonFieldName
  rows: number
}) {
  return (
    <FormField
      control={props.form.control}
      name={props.name}
      render={({ field }) => (
        <FormItem className='min-w-0'>
          <FormLabel>{props.label}</FormLabel>
          <FormControl>
            <JsonCodeEditor
              ariaLabel={props.label}
              onChange={field.onChange}
              rows={props.rows}
              value={field.value}
            />
          </FormControl>
          <FormDescription>{props.description}</FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export const ModelRatioForm = memo(function ModelRatioForm({
  form,
  onSave,
  onReset,
  isSaving,
  isResetting,
  canSave,
  canReset,
  disabledReason,
}: ModelRatioFormProps) {
  const { t } = useTranslation()
  const [editMode, setEditMode] = useState<'visual' | 'json'>('visual')
  const visualEditorRef = useRef<ModelRatioVisualEditorHandle>(null)

  const handleFieldChange = useCallback(
    (field: keyof ModelFormValues, value: string) => {
      form.setValue(field, value, {
        shouldValidate: true,
        shouldDirty: true,
      })
    },
    [form]
  )

  const toggleEditMode = useCallback(() => {
    setEditMode((prev) => (prev === 'visual' ? 'json' : 'visual'))
  }, [])

  const handleSave = useCallback(async () => {
    if (editMode === 'visual') {
      // 可视化模式右侧定价面板可能还停留在未点击 Update/Add 的草稿态。
      // 外层保存前先提交该草稿到表单 JSON，避免管理员保存后丢失当前编辑。
      const committed = await visualEditorRef.current?.commitOpenEditor()
      if (committed === false) return
    }

    await form.handleSubmit(onSave)()
  }, [editMode, form, onSave])

  return (
    <div className='space-y-6'>
      <Alert>
        <AlertTitle>{t('Advanced bulk pricing')}</AlertTitle>
        <AlertDescription>
          {t(
            'Use this page for batch pricing changes. For a single model, edit pricing from the model management page.'
          )}{' '}
          <Link to='/models' className='underline underline-offset-4'>
            {t('Open model management')}
          </Link>
        </AlertDescription>
      </Alert>

      <div className='flex justify-end'>
        <Button variant='outline' size='sm' onClick={toggleEditMode}>
          {editMode === 'visual' ? (
            <>
              <Code2 data-icon='inline-start' />
              {t('Switch to JSON')}
            </>
          ) : (
            <>
              <Eye data-icon='inline-start' />
              {t('Switch to Visual')}
            </>
          )}
        </Button>
      </div>

      <Form {...form}>
        {editMode === 'visual' ? (
          <div className='space-y-6'>
            <ModelRatioVisualEditor
              ref={visualEditorRef}
              modelPrice={form.watch('ModelPrice')}
              modelRatio={form.watch('ModelRatio')}
              cacheRatio={form.watch('CacheRatio')}
              createCacheRatio={form.watch('CreateCacheRatio')}
              completionRatio={form.watch('CompletionRatio')}
              imageRatio={form.watch('ImageRatio')}
              audioRatio={form.watch('AudioRatio')}
              audioCompletionRatio={form.watch('AudioCompletionRatio')}
              billingMode={form.watch('BillingMode')}
              billingExpr={form.watch('BillingExpr')}
              onChange={(field, value) => {
                const fieldMap: Record<string, keyof ModelFormValues> = {
                  'billing_setting.billing_mode': 'BillingMode',
                  'billing_setting.billing_expr': 'BillingExpr',
                }
                const formField =
                  fieldMap[field] || (field as keyof ModelFormValues)
                handleFieldChange(formField, value)
              }}
            />

            <FormField
              control={form.control}
              name='ExposeRatioEnabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Expose ratio API')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Allow clients to query configured ratios via `/api/ratio`.'
                      )}
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

            <div className='flex flex-wrap gap-4'>
              <Button
                type='button'
                onClick={handleSave}
                disabled={isSaving || !canSave}
                title={canSave ? undefined : disabledReason}
              >
                {isSaving ? t('Saving...') : t('Save model prices')}
              </Button>
              <Button
                type='button'
                variant='destructive'
                onClick={onReset}
                disabled={isResetting || !canReset}
                title={canReset ? undefined : disabledReason}
              >
                {t('Reset prices')}
              </Button>
            </div>
          </div>
        ) : (
          <form onSubmit={form.handleSubmit(onSave)} className='space-y-6'>
            {modelJsonFields.map((item) => (
              <ModelJsonEditorField
                description={t(item.descriptionKey)}
                form={form}
                key={item.name}
                label={t(item.labelKey)}
                name={item.name}
                rows={item.rows}
              />
            ))}

            <FormField
              control={form.control}
              name='ExposeRatioEnabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-4'>
                  <div className='space-y-0.5'>
                    <FormLabel className='text-base'>
                      {t('Expose ratio API')}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'Allow clients to query configured ratios via `/api/ratio`.'
                      )}
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

            <div className='flex flex-wrap gap-4'>
              <Button
                type='submit'
                disabled={isSaving || !canSave}
                title={canSave ? undefined : disabledReason}
              >
                {isSaving ? t('Saving...') : t('Save model prices')}
              </Button>
              <Button
                type='button'
                variant='destructive'
                onClick={onReset}
                disabled={isResetting || !canReset}
                title={canReset ? undefined : disabledReason}
              >
                {t('Reset prices')}
              </Button>
            </div>
          </form>
        )}
      </Form>
    </div>
  )
})
