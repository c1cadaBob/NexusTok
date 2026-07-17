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
import { useMemo, useState } from 'react'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { ChatSettingsVisualEditor } from './chat-settings-visual-editor'
import { formatJsonForEditor, normalizeJsonString } from './utils'

const createChatSchema = (t: (key: string) => string) =>
  z.object({
    Chats: z.string().superRefine((value, ctx) => {
      try {
        const parsed = JSON.parse(value || '[]')
        if (!Array.isArray(parsed)) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('Expected a JSON array.'),
          })
          return
        }
        for (const item of parsed) {
          if (
            item === null ||
            typeof item !== 'object' ||
            Array.isArray(item)
          ) {
            ctx.addIssue({
              code: z.ZodIssueCode.custom,
              message: t(
                'Each item must be an object with a single key-value pair.'
              ),
            })
            return
          }
          const entries = Object.entries(item)
          if (entries.length !== 1) {
            ctx.addIssue({
              code: z.ZodIssueCode.custom,
              message: t('Each item must have exactly one key-value pair.'),
            })
            return
          }
        }
      } catch {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: t('Invalid JSON string.'),
        })
      }
    }),
  })

type ChatSettingsFormValues = z.infer<ReturnType<typeof createChatSchema>>

type ChatSettingsSectionProps = {
  defaultValue: string
}

export function ChatSettingsSection({
  defaultValue,
}: ChatSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [editMode, setEditMode] = useState<'visual' | 'json'>('visual')

  const chatSchema = createChatSchema(t)
  const formDefaults = useMemo(
    () => ({ Chats: formatJsonForEditor(defaultValue, '[]') }),
    [defaultValue]
  )
  const defaultNormalized = useMemo(
    () => normalizeJsonString(defaultValue, '[]'),
    [defaultValue]
  )

  const {
    form,
    handleSubmit,
    handleReset,
    isDirty,
    isSubmitting,
  } = useSettingsForm<ChatSettingsFormValues>({
    resolver: zodResolver(chatSchema) as Resolver<
      ChatSettingsFormValues,
      unknown,
      ChatSettingsFormValues
    >,
    mode: 'onChange', // 启用实时校验，便于 JSON 模式下即时提示格式错误。
    defaultValues: formDefaults,
    compareValues: (value, baseline) =>
      normalizeJsonString(String(value ?? ''), '[]') ===
      normalizeJsonString(String(baseline ?? ''), '[]'),
    onSubmit: async (values) => {
      const normalized = normalizeJsonString(values.Chats, '[]')
      if (normalized === defaultNormalized) {
        return
      }

      await updateOption.mutateAsync({
        key: 'Chats',
        value: normalized,
      })
    },
  })

  return (
    <SettingsSection
      title={t('Third-party Presets')}
      description={t(
        'Configure predefined third-party links surfaced to end users.'
      )}
    >
      <FormNavigationGuard when={isDirty} />

      <Form {...form}>
        <SettingsForm onSubmit={handleSubmit}>
          <SettingsPageFormActions
            onSave={handleSubmit}
            onReset={handleReset}
            isSaving={updateOption.isPending || isSubmitting}
            isSaveDisabled={!isDirty || !updateOption.canUpdate}
            isResetDisabled={!isDirty}
            saveDisabledReason={
              updateOption.canUpdate ? undefined : updateOption.disabledReason
            }
            saveLabel='Save third-party settings'
          />
          <FormDirtyIndicator isDirty={isDirty} />

          <Tabs
            value={editMode}
            onValueChange={(value) => setEditMode(value as 'visual' | 'json')}
          >
            <TabsList className='grid w-full grid-cols-2'>
              <TabsTrigger value='visual'>{t('Visual')}</TabsTrigger>
              <TabsTrigger value='json'>{t('JSON')}</TabsTrigger>
            </TabsList>

            <TabsContent value='visual' className='mt-6'>
              <FormField
                control={form.control}
                name='Chats'
                render={({ field }) => (
                  <FormItem>
                    <FormControl>
                      <ChatSettingsVisualEditor
                        value={field.value}
                        onChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </TabsContent>

            <TabsContent value='json' className='mt-6'>
              <FormField
                control={form.control}
                name='Chats'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Third-party configuration JSON')}</FormLabel>
                    <FormControl>
                      <Textarea
                        rows={12}
                        placeholder={t(
                          '[{"ChatGPT":"https://chat.openai.com"},{"Lobe Chat":"https://chat-preview.lobehub.com/?settings={...}"}]'
                        )}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Array of third-party client presets. Each item is an object with one key-value pair: client name and its URL.'
                      )}
                    </FormDescription>
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
