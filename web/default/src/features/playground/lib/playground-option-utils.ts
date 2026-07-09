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

/**
 * 当前模型不在可用列表中时，选择第一个模型作为安全 fallback。
 */
export function getModelFallback(
  models: ModelOption[],
  currentModel: string
): string | null {
  const hasCurrentModel = models.some((model) => model.value === currentModel)

  if (hasCurrentModel || models.length === 0) {
    return null
  }

  return models[0].value
}

/**
 * 分组下无模型时清空模型选择，避免沿用其它分组的旧模型发起请求。
 */
export function shouldClearModelForGroup(
  models: ModelOption[],
  currentModel: string
): boolean {
  if (currentModel === '') {
    return false
  }

  return !models.some((model) => model.value === currentModel)
}

/**
 * 当前分组不可用时优先回到 default，再回到服务端返回的第一项。
 */
export function getGroupFallback(
  groups: GroupOption[],
  currentGroup: string
): string | null {
  const hasCurrentGroup = groups.some((group) => group.value === currentGroup)

  if (hasCurrentGroup || groups.length === 0) {
    return null
  }

  return (
    groups.find((group) => group.value === 'default')?.value ?? groups[0].value
  )
}

/**
 * 从请求错误中提取可展示消息，非 Error 对象则使用调用方提供的翻译文案。
 */
export function getOptionLoadErrorMessage(
  error: unknown,
  fallbackMessage: string
): string {
  return error instanceof Error ? error.message : fallbackMessage
}
