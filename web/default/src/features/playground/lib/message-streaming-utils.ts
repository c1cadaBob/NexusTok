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
import { MESSAGE_ROLES, MESSAGE_STATUS } from '../constants'
import type { ChatCompletionResponse, Message } from '../types'
import {
  finalizeMessage,
  getCurrentVersion,
  processStreamingContent,
  updateCurrentVersionContent,
} from './message-utils'
import {
  completeReasoningTiming,
  startReasoningTiming,
} from './message-timing-utils'

export type MessageStreamChunkType = 'reasoning' | 'content'

type ChatCompletionChoice = ChatCompletionResponse['choices'][number]

/**
 * 计算真正需要追加的 chunk。
 *
 * 标准 SSE 会返回纯 delta；少数兼容上游可能误返回累计内容。当前内容是
 * chunk 前缀时只追加新增部分，避免 UI 中出现重复正文或重复 reasoning。
 */
function getAppendableChunk(currentContent: string, chunk: string): string {
  if (!currentContent || !chunk.startsWith(currentContent)) {
    return chunk
  }

  return chunk.slice(currentContent.length)
}

/**
 * 将单个流式 chunk 应用到 assistant 消息。
 */
export function applyStreamingChunk(
  message: Message,
  type: MessageStreamChunkType,
  chunk: string
): Message {
  if (message.status === MESSAGE_STATUS.ERROR) {
    return message
  }

  if (type === 'reasoning') {
    const reasoning = startReasoningTiming(message)
    const appendableChunk = getAppendableChunk(reasoning.content, chunk)

    return {
      ...message,
      reasoning: {
        ...reasoning,
        content: reasoning.content + appendableChunk,
      },
      isReasoningStreaming: true,
      status: MESSAGE_STATUS.STREAMING,
    }
  }

  const currentVersion = getCurrentVersion(message)
  const appendableChunk = getAppendableChunk(currentVersion.content, chunk)
  const contentMessage = processStreamingContent(message, appendableChunk)

  return {
    ...(contentMessage.isReasoningStreaming
      ? contentMessage
      : completeReasoningTiming(contentMessage)),
    status: MESSAGE_STATUS.STREAMING,
  }
}

/**
 * 将 assistant 消息稳定为完成态，并统一清理 `<think>` 标签和 timing。
 */
export function completeAssistantMessage(message: Message): Message {
  return {
    ...finalizeMessage(message),
    status: MESSAGE_STATUS.COMPLETE,
  }
}

export function isAssistantMessageFinal(message: Message): boolean {
  return (
    message.status === MESSAGE_STATUS.COMPLETE ||
    message.status === MESSAGE_STATUS.ERROR
  )
}

export function isAssistantMessagePending(message: Message): boolean {
  return (
    message.status === MESSAGE_STATUS.LOADING ||
    message.status === MESSAGE_STATUS.STREAMING
  )
}

export function isPendingAssistantMessage(message?: Message): boolean {
  return Boolean(
    message?.from === MESSAGE_ROLES.ASSISTANT &&
      isAssistantMessagePending(message)
  )
}

/**
 * 将非流式 Chat Completions choice 应用到当前 assistant 占位消息。
 */
export function applyChatCompletionChoice(
  message: Message,
  choice: ChatCompletionChoice
): Message {
  return {
    ...finalizeMessage(
      updateCurrentVersionContent(message, choice.message?.content || ''),
      choice.message?.reasoning_content
    ),
    status: MESSAGE_STATUS.COMPLETE,
  }
}

export function applyChatCompletionResponse(
  message: Message,
  response: ChatCompletionResponse
): Message | null {
  const choice = response.choices?.[0]

  if (!choice) {
    return null
  }

  return applyChatCompletionChoice(message, choice)
}
