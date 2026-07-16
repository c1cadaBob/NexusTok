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
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { createChannel, updateChannel } from '../api'
import { ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import {
  transformFormDataToCreatePayload,
  transformFormDataToUpdatePayload,
  type ChannelFormValues,
} from '../lib'
import type { Channel } from '../types'
import type { ChannelPermissions } from './use-channel-permissions'

export type ChannelUpdatePayload = Partial<Channel> & {
  key_mode?: NonNullable<ChannelFormValues['key_mode']>
}

type UseChannelMutateFormParams = {
  currentRow?: Channel | null
  isEditing: boolean
  isMultiKeyChannel: boolean
  permissions: ChannelPermissions
  onSuccess: () => void
}

type BuildAllowedUpdatePayloadParams = {
  payload: Partial<Channel>
  canEditSensitiveFields: boolean
  isMultiKeyChannel: boolean
  keyMode?: ChannelFormValues['key_mode']
}

export const NON_SENSITIVE_CHANNEL_UPDATE_FIELDS = [
  'id',
  'name',
  'models',
  'group',
  'model_mapping',
  'priority',
  'weight',
  'test_model',
  'auto_ban',
  'status_code_mapping',
  'tag',
  'remark',
  'other_info',
  'multi_key_mode',
] as const

export const NON_SENSITIVE_CHANNEL_FORM_FIELDS = [
  'name',
  'models',
  'group',
  'model_mapping',
  'priority',
  'weight',
  'test_model',
  'auto_ban',
  'status_code_mapping',
  'tag',
  'remark',
  'multi_key_mode',
] satisfies readonly (keyof ChannelFormValues)[]

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function hasDirtyValue(value: unknown): boolean {
  if (value === true) return true
  if (Array.isArray(value)) return value.some(hasDirtyValue)
  if (isRecord(value)) return Object.values(value).some(hasDirtyValue)
  return Boolean(value)
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

// 普通写权限只能编辑渠道调度与模型暴露相关字段，凭证、上游地址、请求改写、
// settings 等敏感字段必须 fail-closed 剔除，避免前端状态漂移导致越权保存。
export function pickNonSensitiveChannelUpdatePayload(
  payload: Partial<Channel>
): Partial<Channel> {
  return NON_SENSITIVE_CHANNEL_UPDATE_FIELDS.reduce<Partial<Channel>>(
    (nextPayload, field) => {
      if (field in payload) {
        ;(nextPayload as Record<string, unknown>)[field] = (
          payload as Record<string, unknown>
        )[field]
      }
      return nextPayload
    },
    {}
  )
}

// 普通写权限只能提交调度、模型暴露和备注类字段；任何不在白名单内的 dirty 字段
// 都按敏感变更处理。这里用于前端提交前提示，Hook 的 payload 裁剪仍是最终防线。
export function getDirtySensitiveChannelFormFields(
  dirtyFields: Partial<Record<string, unknown>>
): string[] {
  const nonSensitiveFields = new Set<string>(NON_SENSITIVE_CHANNEL_FORM_FIELDS)

  return Object.entries(dirtyFields).reduce<string[]>(
    (sensitiveFields, [field, dirtyValue]) => {
      if (!hasDirtyValue(dirtyValue)) return sensitiveFields
      if (!nonSensitiveFields.has(field)) {
        sensitiveFields.push(field)
      }
      return sensitiveFields
    },
    []
  )
}

export function hasDirtySensitiveChannelFormFields(
  dirtyFields: Partial<Record<string, unknown>>
): boolean {
  return getDirtySensitiveChannelFormFields(dirtyFields).length > 0
}

export function buildAllowedChannelUpdatePayload({
  payload,
  canEditSensitiveFields,
  isMultiKeyChannel,
  keyMode,
}: BuildAllowedUpdatePayloadParams): ChannelUpdatePayload {
  const payloadWithKeyMode: ChannelUpdatePayload =
    canEditSensitiveFields && isMultiKeyChannel && keyMode
      ? {
          ...payload,
          key_mode: keyMode,
        }
      : payload

  return canEditSensitiveFields
    ? payloadWithKeyMode
    : pickNonSensitiveChannelUpdatePayload(payloadWithKeyMode)
}

export function useChannelMutateForm(props: UseChannelMutateFormParams) {
  const { t } = useTranslation()
  const canEditSensitiveFields = props.permissions.canSensitiveWrite
  const canEditBasicFields =
    props.permissions.canWrite || props.permissions.canSensitiveWrite

  return useMutation({
    mutationFn: async (data: ChannelFormValues): Promise<string> => {
      if (!props.isEditing && !canEditSensitiveFields) {
        throw new Error(t("You don't have necessary permission"))
      }
      if (props.isEditing && !canEditBasicFields) {
        throw new Error(t("You don't have necessary permission"))
      }

      if (props.isEditing && props.currentRow) {
        // 更新渠道时按权限裁剪 payload。敏感写权限可保存完整配置；
        // 普通写权限只允许保存模型、分组、调度权重等非敏感字段。
        const payload = transformFormDataToUpdatePayload(
          data,
          props.currentRow.id
        )
        const allowedPayload = buildAllowedChannelUpdatePayload({
          payload,
          canEditSensitiveFields,
          isMultiKeyChannel: props.isMultiKeyChannel,
          keyMode: data.key_mode,
        })

        const response = await updateChannel(
          props.currentRow.id,
          allowedPayload
        )
        if (!response.success) {
          throw new Error(response.message || t(ERROR_MESSAGES.UPDATE_FAILED))
        }
        return SUCCESS_MESSAGES.UPDATED
      }

      // 创建渠道只允许敏感写权限触发；多 Key、批量创建和账号池组模式在转换函数中统一归一。
      const payload = transformFormDataToCreatePayload(data)
      const response = await createChannel(payload)
      if (!response.success) {
        throw new Error(response.message || t(ERROR_MESSAGES.CREATE_FAILED))
      }
      return SUCCESS_MESSAGES.CREATED
    },
    onSuccess: (messageKey) => {
      toast.success(t(messageKey))
      props.onSuccess()
    },
    onError: (error: unknown) => {
      const fallbackMessage = props.isEditing
        ? ERROR_MESSAGES.UPDATE_FAILED
        : ERROR_MESSAGES.CREATE_FAILED
      toast.error(getErrorMessage(error) || t(fallbackMessage))
    },
  })
}
