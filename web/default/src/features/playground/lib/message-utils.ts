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
import { nanoid } from 'nanoid'
import { MESSAGE_ROLES, MESSAGE_STATUS, ERROR_MESSAGES } from '../constants'
import type {
  Message,
  MessageVersion,
  ChatCompletionMessage,
  ContentPart,
} from '../types'
import {
  completeAssistantTiming,
  completeReasoningTiming,
  startReasoningTiming,
} from './message-timing-utils'

/**
 * 创建一条新的消息版本。
 */
export function createMessageVersion(content: string): MessageVersion {
  return {
    id: nanoid(),
    content,
  }
}

/**
 * 读取消息当前版本；Playground 当前始终使用第一版作为活跃内容。
 */
export function getCurrentVersion(message: Message): MessageVersion {
  return message.versions[0] || { id: 'default', content: '' }
}

/**
 * 更新消息当前版本内容。
 */
export function updateCurrentVersionContent(
  message: Message,
  content: string
): Message {
  const currentVersion = getCurrentVersion(message)
  return {
    ...message,
    versions: [{ ...currentVersion, content }],
  }
}

/**
 * 创建 user 消息，并记录本地提交时间。
 */
export function createUserMessage(
  content: string,
  createdAt: number = Date.now()
): Message {
  return {
    key: nanoid(),
    from: MESSAGE_ROLES.USER,
    versions: [createMessageVersion(content)],
    createdAt,
  }
}

/**
 * 创建等待中的 assistant 占位消息，并以 startedAt 作为响应耗时起点。
 */
export function createLoadingAssistantMessage(
  startedAt: number = Date.now()
): Message {
  return {
    key: nanoid(),
    from: MESSAGE_ROLES.ASSISTANT,
    versions: [createMessageVersion('')],
    createdAt: startedAt,
    startedAt,
    reasoning: undefined,
    isReasoningComplete: false,
    isContentComplete: false,
    isReasoningStreaming: false,
    status: MESSAGE_STATUS.LOADING,
  }
}

/**
 * 构建包含可选图片的消息内容。
 */
export function buildMessageContent(
  text: string,
  imageUrls: string[] = []
): string | ContentPart[] {
  const validImages = imageUrls.filter((url) => url.trim() !== '')

  if (validImages.length === 0) {
    return text
  }

  const parts: ContentPart[] = [
    {
      type: 'text',
      text: text || '',
    },
    ...validImages.map((url) => ({
      type: 'image_url' as const,
      image_url: { url: url.trim() },
    })),
  ]

  return parts
}

/**
 * 从消息内容中提取文本片段。
 */
export function getTextContent(content: string | ContentPart[]): string {
  if (typeof content === 'string') {
    return content
  }

  if (Array.isArray(content)) {
    const textPart = content.find((part) => part.type === 'text')
    return textPart?.text || ''
  }

  return ''
}

/**
 * 将内部消息格式转为上游 Chat Completions 请求格式。
 */
export function formatMessageForAPI(message: Message): ChatCompletionMessage {
  const currentVersion = getCurrentVersion(message)
  return {
    role: message.from,
    content: currentVersion.content,
  }
}

/**
 * 判断消息是否可以进入上游请求上下文。
 *
 * loading/streaming assistant 占位消息和空 assistant 内容不能发送给上游。
 */
export function isValidMessage(message: Message): boolean {
  if (!message || !message.from || !message.versions.length) return false

  const content = message.versions[0]?.content
  if (content === undefined) return false

  // 排除空 assistant 消息，避免把前端占位状态发送到上游。
  if (message.from === 'assistant' && !content.trim()) return false

  return true
}

/**
 * 解析 `<think>` 内容，把推理过程从可见正文中拆出来。
 *
 * 兼容完整和未闭合的 `<think>` 标签，流式过程中未闭合部分会进入
 * reasoning buffer，而不是提前显示到正文里。
 */
export function parseThinkTags(content: string): {
  visibleContent: string
  reasoning: string
  hasUnclosedTag: boolean
} {
  if (!content.includes('<think>')) {
    return { visibleContent: content, reasoning: '', hasUnclosedTag: false }
  }

  const visibleParts: string[] = []
  const reasoningParts: string[] = []
  let currentPos = 0
  let hasUnclosed = false

  while (true) {
    // 查找下一段 `<think>` 标签。
    const openPos = content.indexOf('<think>', currentPos)

    if (openPos === -1) {
      // 没有更多标签时，剩余内容全部作为可见正文。
      if (currentPos < content.length) {
        visibleParts.push(content.substring(currentPos))
      }
      break
    }

    // 标签之前的内容保持为可见正文。
    if (openPos > currentPos) {
      visibleParts.push(content.substring(currentPos, openPos))
    }

    // 查找匹配的闭合标签。
    const closePos = content.indexOf('</think>', openPos + 7)

    if (closePos === -1) {
      // 未闭合标签表示当前剩余内容仍在推理流中。
      reasoningParts.push(content.substring(openPos + 7))
      hasUnclosed = true
      break
    }

    // 提取完整标签内部的推理内容。
    reasoningParts.push(content.substring(openPos + 7, closePos))
    currentPos = closePos + 8
  }

  return {
    visibleContent: visibleParts.join('').trim(),
    reasoning: reasoningParts.join('\n\n').trim(),
    hasUnclosedTag: hasUnclosed,
  }
}

/**
 * 将最后一条 assistant 消息更新为错误态。
 */
export function updateAssistantMessageWithError(
  messages: Message[],
  errorMessage: string,
  errorCode?: string
): Message[] {
  return updateLastAssistantMessage(messages, (message) => {
    const updatedMessage = updateCurrentVersionContent(
      message,
      `${ERROR_MESSAGES.API_REQUEST_ERROR}: ${errorMessage}`
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

/**
 * 处理流式正文增量，并实时拆分 `<think>` 推理内容。
 *
 * 流式期间 `versions[0].content` 保留带标签的原始内容，完成时再清理为可见正文。
 */
export function processStreamingContent(
  message: Message,
  contentChunk?: string
): Message {
  const currentVersion = getCurrentVersion(message)
  const fullContent = contentChunk
    ? currentVersion.content + contentChunk
    : currentVersion.content

  const { reasoning, hasUnclosedTag } = parseThinkTags(fullContent)

  // 没有 `<think>` 标签时，保留 API reasoning_content 已写入的推理内容。
  const reasoningTiming =
    reasoning || message.reasoning ? startReasoningTiming(message) : undefined
  const finalReasoning = reasoningTiming
    ? { ...reasoningTiming, content: reasoning || reasoningTiming.content }
    : undefined

  return {
    ...updateCurrentVersionContent(message, fullContent),
    reasoning: finalReasoning,
    isReasoningStreaming: hasUnclosedTag,
  }
}

/**
 * 在流式或非流式响应完成后稳定消息内容。
 *
 * 这里统一清理 `<think>` 标签、合并 API reasoning_content 和流式 reasoning，
 * 并补齐 assistant/reasoning 的完成时间，保证停止生成和错误恢复前后的消息结构一致。
 */
export function finalizeMessage(
  message: Message,
  apiReasoningContent?: string
): Message {
  const currentVersion = getCurrentVersion(message)
  const { visibleContent, reasoning } = parseThinkTags(currentVersion.content)

  // 推理内容优先级：
  // 1. 非流式响应显式传入的 API reasoning_content；
  // 2. 流式 reasoning_content 已累积到 message.reasoning；
  // 3. 从 `<think>` 标签提取的推理内容。
  const finalReasoning =
    apiReasoningContent || message.reasoning?.content || reasoning || ''

  const finalizedMessage: Message = {
    ...updateCurrentVersionContent(message, visibleContent),
    reasoning: finalReasoning
      ? {
          content: finalReasoning,
          duration: message.reasoning?.duration || 0,
          startedAt: message.reasoning?.startedAt,
          completedAt: message.reasoning?.completedAt,
          durationMs: message.reasoning?.durationMs,
        }
      : undefined,
    isReasoningStreaming: false,
  }

  return completeAssistantTiming(completeReasoningTiming(finalizedMessage))
}

/**
 * 清理从本地存储恢复的消息。
 *
 * 浏览器刷新或异常退出后，最后一条 assistant 可能停在 loading/streaming；
 * 恢复时将其稳定为 complete 或 error，避免页面长期显示“正在响应”。
 */
export function sanitizeMessagesOnLoad(messages: Message[]): Message[] {
  let targetIndex = -1
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i]
    if (
      m?.from === MESSAGE_ROLES.ASSISTANT &&
      (m?.status === MESSAGE_STATUS.LOADING ||
        m?.status === MESSAGE_STATUS.STREAMING)
    ) {
      targetIndex = i
      break
    }
  }

  if (targetIndex === -1) return messages

  const finalized = finalizeMessage(messages[targetIndex])
  const hasContent = finalized.versions?.[0]?.content?.trim()
  const hasReasoning = finalized.reasoning?.content?.trim()

  const sanitized: Message =
    hasContent || hasReasoning
      ? {
          ...finalized,
          status: MESSAGE_STATUS.COMPLETE,
          isReasoningStreaming: false,
        }
      : {
          ...updateCurrentVersionContent(
            finalized,
            `${ERROR_MESSAGES.API_REQUEST_ERROR}: ${ERROR_MESSAGES.INTERRUPTED}`
          ),
          status: MESSAGE_STATUS.ERROR,
          isReasoningStreaming: false,
        }

  const result = [...messages]
  result[targetIndex] = sanitized
  return result
}
