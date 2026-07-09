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
import { useCallback, useState } from 'react'
import {
  appendUserMessagePair,
  applyMessageEdit,
  createRegeneratedMessages,
  removeMessageByKey,
  type MessageStateUpdater,
} from '../lib'
import type { Message } from '../types'

type UsePlaygroundConversationOptions = {
  messages: Message[]
  updateMessages: (updater: MessageStateUpdater) => void
  sendChat: (messages: Message[]) => void
}

/**
 * 管理 Playground 会话操作和编辑态。
 *
 * 容器组件只负责把回调传给视图；消息数组截断、重试和编辑后重提交流程
 * 继续复用已测试的纯函数，避免多处手写同一套请求上下文变换规则。
 */
export function usePlaygroundConversation({
  messages,
  updateMessages,
  sendChat,
}: UsePlaygroundConversationOptions) {
  const [editingMessageKey, setEditingMessageKey] = useState<string | null>(
    null
  )

  const handleSendMessage = useCallback(
    (text: string) => {
      const nextMessages = appendUserMessagePair(messages, text)
      updateMessages(nextMessages)
      sendChat(nextMessages)
    },
    [messages, updateMessages, sendChat]
  )

  const handleRegenerateMessage = useCallback(
    (message: Message) => {
      const nextMessages = createRegeneratedMessages(messages, message.key)
      if (!nextMessages) return

      updateMessages(nextMessages)
      sendChat(nextMessages)
    },
    [messages, updateMessages, sendChat]
  )

  const handleEditMessage = useCallback((message: Message) => {
    setEditingMessageKey(message.key)
  }, [])

  const handleEditOpenChange = useCallback((open: boolean) => {
    if (!open) {
      setEditingMessageKey(null)
    }
  }, [])

  const applyEdit = useCallback(
    (newContent: string, shouldSubmit: boolean) => {
      if (!editingMessageKey) return

      const editResult = applyMessageEdit(
        messages,
        editingMessageKey,
        newContent,
        shouldSubmit
      )
      if (!editResult) return

      setEditingMessageKey(null)
      updateMessages(editResult.messages)

      if (editResult.shouldSend) {
        sendChat(editResult.messages)
      }
    },
    [editingMessageKey, messages, updateMessages, sendChat]
  )

  const handleDeleteMessage = useCallback(
    (message: Message) => {
      updateMessages((previousMessages) =>
        removeMessageByKey(previousMessages, message.key)
      )
    },
    [updateMessages]
  )

  const resetEditingMessage = useCallback(() => {
    setEditingMessageKey(null)
  }, [])

  return {
    editingMessageKey,
    handleSendMessage,
    handleRegenerateMessage,
    handleEditMessage,
    handleEditOpenChange,
    applyEdit,
    handleDeleteMessage,
    resetEditingMessage,
  }
}
