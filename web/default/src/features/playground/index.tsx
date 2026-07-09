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
import { useCallback, useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getUserModels, getUserGroups } from './api'
import { PlaygroundChat } from './components/playground-chat'
import { PlaygroundInput } from './components/playground-input'
import { usePlaygroundState, useChatHandler } from './hooks'
import { createUserMessage, createLoadingAssistantMessage } from './lib'
import type { Message as MessageType } from './types'

export function Playground() {
  const {
    config,
    parameterEnabled,
    messages,
    models,
    groups,
    updateMessages,
    clearMessages,
    setModels,
    setGroups,
    updateConfig,
  } = usePlaygroundState()

  const { sendChat, stopGeneration, isGenerating } = useChatHandler({
    config,
    parameterEnabled,
    onMessageUpdate: updateMessages,
  })

  // 编辑状态只保存当前消息 key，避免复制整条消息对象导致内容陈旧。
  const [editingMessageKey, setEditingMessageKey] = useState<string | null>(
    null
  )

  // 加载当前用户可用模型列表。
  const { data: modelsData, isLoading: isLoadingModels } = useQuery({
    queryKey: ['playground-models'],
    queryFn: getUserModels,
  })

  // 加载当前用户可用分组列表。
  const { data: groupsData } = useQuery({
    queryKey: ['playground-groups'],
    queryFn: getUserGroups,
  })

  // 模型列表变化后校正当前模型，避免继续使用已不可见模型。
  useEffect(() => {
    if (!modelsData) return

    setModels(modelsData)

    const isCurrentModelValid = modelsData.some((m) => m.value === config.model)
    if (modelsData.length > 0 && !isCurrentModelValid) {
      updateConfig('model', modelsData[0].value)
    }
  }, [modelsData, config.model, setModels, updateConfig])

  // 分组列表变化后优先回退 default 分组，再回退第一项。
  useEffect(() => {
    if (!groupsData) return

    setGroups(groupsData)

    const hasCurrentGroup = groupsData.some((g) => g.value === config.group)
    if (!hasCurrentGroup && groupsData.length > 0) {
      const fallback =
        groupsData.find((g) => g.value === 'default')?.value ??
        groupsData[0].value
      updateConfig('group', fallback)
    }
  }, [groupsData, setGroups, config.group, updateConfig])

  const handleSendMessage = (text: string) => {
    const userMessage = createUserMessage(text)
    const assistantMessage = createLoadingAssistantMessage()

    const newMessages = [...messages, userMessage, assistantMessage]
    updateMessages(newMessages)

    // 发送请求时直接复用当前消息数组，确保本地展示和上游上下文一致。
    sendChat(newMessages)
  }

  const handleCopyMessage = (message: MessageType) => {
    // 复制动作已经由 MessageActions 处理，这里只保留可观测回调。
    // eslint-disable-next-line no-console
    console.log('Message copied:', message.key)
  }

  const handleRegenerateMessage = (message: MessageType) => {
    const messageIndex = messages.findIndex((m) => m.key === message.key)
    if (messageIndex === -1) return

    const messagesUpToHere = messages.slice(0, messageIndex)
    const loadingMessage = createLoadingAssistantMessage()
    const newMessages = [...messagesUpToHere, loadingMessage]

    updateMessages(newMessages)
    sendChat(newMessages)
  }

  const handleEditMessage = useCallback((message: MessageType) => {
    setEditingMessageKey(message.key)
  }, [])

  const handleEditOpenChange = useCallback((open: boolean) => {
    if (!open) setEditingMessageKey(null)
  }, [])

  // 保存编辑后可选择从该用户消息重新提交，后续消息会被新的 assistant 占位替换。
  const applyEdit = useCallback(
    (newContent: string, submit: boolean) => {
      if (!editingMessageKey) return
      const index = messages.findIndex((m) => m.key === editingMessageKey)
      if (index === -1) return

      const updated = messages.map((m) =>
        m.key === editingMessageKey
          ? { ...m, versions: [{ ...m.versions[0], content: newContent }] }
          : m
      )

      setEditingMessageKey(null)

      if (!submit || updated[index].from !== 'user') {
        updateMessages(updated)
        return
      }

      const toSubmit = [
        ...updated.slice(0, index + 1),
        createLoadingAssistantMessage(),
      ]
      updateMessages(toSubmit)
      sendChat(toSubmit)
    },
    [editingMessageKey, messages, updateMessages, sendChat]
  )

  const handleDeleteMessage = (message: MessageType) => {
    const newMessages = messages.filter((m) => m.key !== message.key)
    updateMessages(newMessages)
  }

  const handleClearMessages = useCallback(() => {
    setEditingMessageKey(null)
    clearMessages()
  }, [clearMessages])

  return (
    <div className='relative flex size-full flex-col overflow-hidden'>
      {/* 全宽滚动容器让侧边空白区域也能滚动消息列表。 */}
      <div className='flex flex-1 flex-col overflow-hidden'>
        <PlaygroundChat
          messages={messages}
          onCopyMessage={handleCopyMessage}
          onRegenerateMessage={handleRegenerateMessage}
          onEditMessage={handleEditMessage}
          onDeleteMessage={handleDeleteMessage}
          isGenerating={isGenerating}
          editingKey={editingMessageKey}
          onCancelEdit={handleEditOpenChange}
          onSaveEdit={(newContent) => applyEdit(newContent, false)}
          onSaveEditAndSubmit={(newContent) => applyEdit(newContent, true)}
          onSelectPrompt={handleSendMessage}
        />
      </div>

      {/* 输入区与消息容器保持同一最大宽度。 */}
      <div className='mx-auto w-full max-w-4xl'>
        <PlaygroundInput
          disabled={isGenerating}
          groups={groups}
          groupValue={config.group}
          isGenerating={isGenerating}
          isModelLoading={isLoadingModels}
          modelValue={config.model}
          models={models}
          hasMessages={messages.length > 0}
          onGroupChange={(value) => updateConfig('group', value)}
          onModelChange={(value) => updateConfig('model', value)}
          onClearMessages={handleClearMessages}
          onStop={stopGeneration}
          onSubmit={handleSendMessage}
        />
      </div>
    </div>
  )
}
