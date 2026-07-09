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
  MESSAGE_ACTION_LABELS,
  MESSAGE_ROLES,
  MESSAGE_STATUS,
} from '../constants'
import type { Message } from '../types'
import { getMessageContent, hasMessageContent } from './message-utils'

export type MessageActionState = {
  content: string
  hasContent: boolean
  isAssistant: boolean
  isLoading: boolean
  isUser: boolean
}

/**
 * 计算消息动作栏所需的稳定状态，避免组件内重复手写角色和状态判断。
 */
export function getMessageActionState(message: Message): MessageActionState {
  return {
    content: getMessageContent(message),
    hasContent: hasMessageContent(message),
    isAssistant: message.from === MESSAGE_ROLES.ASSISTANT,
    isUser: message.from === MESSAGE_ROLES.USER,
    isLoading:
      message.status === MESSAGE_STATUS.LOADING ||
      message.status === MESSAGE_STATUS.STREAMING,
  }
}

/**
 * 根据调用方配置返回动作栏可见性 class。
 */
export function getMessageActionsVisibilityClass(
  alwaysVisible: boolean
): string {
  return alwaysVisible
    ? 'opacity-100'
    : 'opacity-0 group-hover:opacity-100 max-md:opacity-100'
}

/**
 * 判断原始响应切换按钮是否应该展示。
 *
 * 该按钮只面向已经有完整正文的 assistant 消息；loading/streaming 阶段的
 * 原始文本仍在拼接中，此时展示 source 视图容易让用户误判为最终响应。
 */
export function canToggleMessageSource({
  hasContent,
  hasToggleHandler,
  isAssistant,
  isLoading,
}: {
  hasContent: boolean
  hasToggleHandler: boolean
  isAssistant: boolean
  isLoading: boolean
}): boolean {
  return isAssistant && hasContent && !isLoading && hasToggleHandler
}

/**
 * 根据当前 source 视图状态返回切换按钮文案 key。
 */
export function getMessageSourceToggleLabel(isSourceVisible: boolean): string {
  return isSourceVisible
    ? MESSAGE_ACTION_LABELS.SHOW_PREVIEW
    : MESSAGE_ACTION_LABELS.SHOW_SOURCE
}
