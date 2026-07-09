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
import { ERROR_MESSAGES, MESSAGE_ROLES, MESSAGE_STATUS } from '../constants'
import type { Message } from '../types'
import {
  completeAssistantTiming,
  completeReasoningTiming,
} from './message-timing-utils'
import { updateCurrentVersionContent } from './message-utils'

/**
 * 将最后一条 assistant 消息更新为错误态。
 *
 * title 默认保持当前通用 API 请求错误文案；调用方可以传入更具体的错误标题，
 * 但本函数始终负责补齐错误状态、reasoning 停止标记和耗时信息。
 */
export function updateAssistantMessageWithError(
  messages: Message[],
  errorMessage: string,
  errorCode?: string,
  title: string = ERROR_MESSAGES.API_REQUEST_ERROR
): Message[] {
  return updateLastAssistantMessage(messages, (message) => {
    const updatedMessage = updateCurrentVersionContent(
      message,
      `${title}: ${errorMessage}`
    )
    const failedMessage: Message = {
      ...updatedMessage,
      status: MESSAGE_STATUS.ERROR,
      isReasoningStreaming: false,
      errorCode: errorCode || null,
    }

    return completeAssistantTiming(completeReasoningTiming(failedMessage))
  })
}

/**
 * 更新最后一条 assistant 消息；没有 assistant 时返回原数组。
 */
export function updateLastAssistantMessage(
  messages: Message[],
  updater: (message: Message) => Message
): Message[] {
  if (messages.length === 0) return messages
  const last = messages[messages.length - 1]
  if (!last || last.from !== MESSAGE_ROLES.ASSISTANT) return messages

  const updated = [...messages]
  updated[updated.length - 1] = updater(last)
  return updated
}
