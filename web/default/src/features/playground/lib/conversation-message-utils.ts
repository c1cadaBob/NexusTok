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
import { MESSAGE_ROLES } from '../constants'
import type { Message } from '../types'
import {
  createLoadingAssistantMessage,
  createUserMessage,
  updateCurrentVersionContent,
} from './message-utils'

interface ApplyMessageEditResult {
  messages: Message[]
  shouldSend: boolean
}

interface ChatMessageRenderState {
  alwaysShowActions: boolean
  content: string
  isEditing: boolean
}

/**
 * 创建一次用户输入对应的 user + loading assistant 消息对。
 *
 * Playground 发送请求时会直接把返回数组交给 `sendChat`，所以这里必须确保
 * UI 展示数组和上游上下文数组完全一致。
 */
export function appendUserMessagePair(
  messages: Message[],
  content: string
): Message[] {
  return [
    ...messages,
    createUserMessage(content),
    createLoadingAssistantMessage(),
  ]
}

/**
 * 根据目标消息生成重试上下文。
 *
 * assistant 重试需要删除当前 assistant 及其之后的旧上下文；如果未来允许
 * 对 user 消息直接重试，则保留该 user prompt 并在其后追加新的 assistant 占位。
 */
export function createRegeneratedMessages(
  messages: Message[],
  messageKey: string
): Message[] | null {
  const messageIndex = messages.findIndex(
    (message) => message.key === messageKey
  )

  if (messageIndex === -1) {
    return null
  }

  if (messages[messageIndex].from === MESSAGE_ROLES.USER) {
    return [
      ...messages.slice(0, messageIndex + 1),
      createLoadingAssistantMessage(),
    ]
  }

  return [...messages.slice(0, messageIndex), createLoadingAssistantMessage()]
}

/**
 * 删除指定消息；如果 key 不存在，返回一个内容等价的新数组，保持调用侧无分支。
 */
export function removeMessageByKey(
  messages: Message[],
  messageKey: string
): Message[] {
  return messages.filter((message) => message.key !== messageKey)
}

/**
 * 从错误 assistant 消息向前查找最近的一条用户消息。
 *
 * 错误恢复里的 Edit Prompt 应该编辑触发失败的 prompt，而不是编辑 assistant
 * 错误占位消息本身；倒序查找可以兼容中间插入 system/source 等消息的情况。
 */
export function getPreviousUserMessage(
  messages: Message[],
  beforeIndex: number
): Message | null {
  for (let index = beforeIndex - 1; index >= 0; index--) {
    const candidate = messages[index]

    if (candidate?.from === MESSAGE_ROLES.USER) {
      return candidate
    }
  }

  return null
}

/**
 * 应用消息编辑结果，并在用户选择重新提交时截断后续上下文。
 *
 * Save 只更新当前消息；Save & Submit 仅对 user 消息生效，并会丢弃该 user
 * 之后的旧消息，追加新的 loading assistant，确保上游只看到新的上下文。
 */
export function applyMessageEdit(
  messages: Message[],
  messageKey: string,
  content: string,
  shouldSubmit: boolean
): ApplyMessageEditResult | null {
  const messageIndex = messages.findIndex(
    (message) => message.key === messageKey
  )

  if (messageIndex === -1) {
    return null
  }

  const updatedMessages = messages.map((message) =>
    message.key === messageKey
      ? updateCurrentVersionContent(message, content)
      : message
  )

  if (
    !shouldSubmit ||
    updatedMessages[messageIndex].from !== MESSAGE_ROLES.USER
  ) {
    return {
      messages: updatedMessages,
      shouldSend: false,
    }
  }

  return {
    messages: [
      ...updatedMessages.slice(0, messageIndex + 1),
      createLoadingAssistantMessage(),
    ],
    shouldSend: true,
  }
}

/**
 * 读取当前正在编辑的消息内容。
 *
 * 编辑状态只保存 key，组件每次进入编辑态时从最新 messages 中取内容，
 * 避免复制整条消息对象后出现陈旧内容。
 */
export function getEditingMessageContent(
  messages: Message[],
  editingKey?: string | null
): string {
  if (!editingKey) {
    return ''
  }

  const message = messages.find((item) => item.key === editingKey)
  return message?.versions[0]?.content ?? ''
}

/**
 * 汇总单条消息渲染所需的派生状态，减少聊天组件中的重复判断。
 */
export function getChatMessageRenderState(
  messages: Message[],
  message: Message,
  messageIndex: number,
  editingKey?: string | null
): ChatMessageRenderState {
  return {
    alwaysShowActions:
      messageIndex === messages.length - 1 &&
      message.from === MESSAGE_ROLES.ASSISTANT,
    content: message.versions[0]?.content ?? '',
    isEditing: editingKey === message.key,
  }
}
