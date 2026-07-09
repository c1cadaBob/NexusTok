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
import type { GroupOption, ModelOption } from '../types'

interface InputControlStateOptions {
  disabled?: boolean
  groups: GroupOption[]
  hasStopHandler: boolean
  isGenerating?: boolean
  isModelLoading?: boolean
  models: ModelOption[]
  text: string
}

interface InputControlState {
  canSubmit: boolean
  isSelectorDisabled: boolean
  shouldShowStop: boolean
}

interface SubmittableInputMessage {
  text?: string | null
}

/**
 * 解析可提交的输入文本。
 *
 * 这里保持和旧输入区一致：只用 trim 判断是否为空，但提交原始文本，
 * 避免用户有意输入的首尾空白在调试模型时被前端静默改写。
 */
export function getSubmittableInputText(
  message: SubmittableInputMessage,
  disabled?: boolean
): string | null {
  if (disabled || !message.text?.trim()) {
    return null
  }

  return message.text
}

/**
 * 集中计算输入区按钮和选择器状态。
 *
 * 模型/分组选择器缺少可用数据时禁用，提交按钮则要求至少已有模型列表；
 * 这样加载期间不会发出明显无效的 Playground 请求。
 */
export function getInputControlState({
  disabled,
  groups,
  hasStopHandler,
  isGenerating,
  isModelLoading,
  models,
  text,
}: InputControlStateOptions): InputControlState {
  const hasModels = models.length > 0

  return {
    canSubmit: !disabled && hasModels && text.trim().length > 0,
    isSelectorDisabled:
      disabled || isModelLoading || models.length === 0 || groups.length === 0,
    shouldShowStop: Boolean(isGenerating && hasStopHandler),
  }
}
