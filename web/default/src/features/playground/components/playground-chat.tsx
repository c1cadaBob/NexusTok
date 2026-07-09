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
import { useEffect, useState } from 'react'
import {
  Branch,
  BranchMessages,
  BranchNext,
  BranchPage,
  BranchPrevious,
  BranchSelector,
} from '@/components/ai-elements/branch'
import {
  Conversation,
  ConversationContent,
  ConversationScrollButton,
} from '@/components/ai-elements/conversation'
import { Message } from '@/components/ai-elements/message'
import { MESSAGE_STATUS } from '../constants'
import {
  getChatMessageRenderState,
  getEditingMessageContent,
  isMessageSourceVisible,
  getPreviousUserMessage,
  toggleMessageSourceKey,
} from '../lib'
import type { Message as MessageType } from '../types'
import { MessageActions } from './message-actions'
import { MessageErrorActions } from './message-error-actions'
import { PlaygroundEmptyState } from './playground-empty-state'
import { PlaygroundMessageContent } from './playground-message-content'
import { PlaygroundMessageEditor } from './playground-message-editor'

interface PlaygroundChatProps {
  messages: MessageType[]
  onCopyMessage?: (message: MessageType) => void
  onRegenerateMessage?: (message: MessageType) => void
  onEditMessage?: (message: MessageType) => void
  onDeleteMessage?: (message: MessageType) => void
  isGenerating?: boolean
  editingKey?: string | null
  onSaveEdit?: (newContent: string) => void
  onCancelEdit?: (open: boolean) => void
  onSaveEditAndSubmit?: (newContent: string) => void
  onSelectPrompt?: (prompt: string) => void
}

export function PlaygroundChat({
  messages,
  onCopyMessage,
  onRegenerateMessage,
  onEditMessage,
  onDeleteMessage,
  isGenerating = false,
  editingKey,
  onSaveEdit,
  onCancelEdit,
  onSaveEditAndSubmit,
  onSelectPrompt,
}: PlaygroundChatProps) {
  const [editText, setEditText] = useState('')
  const [originalText, setOriginalText] = useState('')
  const [sourceMessageKeys, setSourceMessageKeys] = useState<
    ReadonlySet<string>
  >(() => new Set())

  function handleToggleMessageSource(message: MessageType): void {
    setSourceMessageKeys((currentKeys) =>
      toggleMessageSourceKey(currentKeys, message.key)
    )
  }

  useEffect(() => {
    if (!editingKey) return
    const content = getEditingMessageContent(messages, editingKey)
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setEditText(content)

    setOriginalText(content)
  }, [editingKey, messages])

  const chatContent =
    messages.length === 0 && onSelectPrompt ? (
      <PlaygroundEmptyState onSelectPrompt={onSelectPrompt} />
    ) : (
      messages.map((message, messageIndex) => {
        const { versions = [] } = message
        const { alwaysShowActions, isEditing } = getChatMessageRenderState(
          messages,
          message,
          messageIndex,
          editingKey
        )
        const previousUserMessage =
          message.status === MESSAGE_STATUS.ERROR
            ? getPreviousUserMessage(messages, messageIndex)
            : null
        const sourceVisible = isMessageSourceVisible(
          sourceMessageKeys,
          message.key
        )
        const actions = (
          <MessageActions
            message={message}
            onCopy={onCopyMessage}
            onRegenerate={onRegenerateMessage}
            onToggleSource={handleToggleMessageSource}
            onEdit={onEditMessage}
            onDelete={onDeleteMessage}
            isSourceVisible={sourceVisible}
            isGenerating={isGenerating}
            alwaysVisible={alwaysShowActions}
            className='mt-1'
          />
        )
        const errorActions =
          message.status === MESSAGE_STATUS.ERROR ? (
            <MessageErrorActions
              disabled={isGenerating}
              onRetry={
                onRegenerateMessage
                  ? () => onRegenerateMessage(message)
                  : undefined
              }
              onEditPrompt={
                onEditMessage && previousUserMessage
                  ? () => onEditMessage(previousUserMessage)
                  : undefined
              }
              onDelete={
                onDeleteMessage ? () => onDeleteMessage(message) : undefined
              }
            />
          ) : undefined

        return (
          <Branch defaultBranch={0} key={message.key}>
            <BranchMessages>
              {versions.map((version, versionIndex) => (
                <Message
                  className='group flex-row-reverse'
                  from={message.from}
                  key={`${message.key}-${version.id}-${versionIndex}`}
                >
                  <div className='w-full min-w-0 flex-1 basis-full py-1'>
                    {isEditing ? (
                      <PlaygroundMessageEditor
                        editText={editText}
                        message={message}
                        onCancelEdit={onCancelEdit}
                        onEditTextChange={setEditText}
                        onSaveEdit={onSaveEdit}
                        onSaveEditAndSubmit={onSaveEditAndSubmit}
                        originalText={originalText}
                      />
                    ) : (
                      <PlaygroundMessageContent
                        actions={actions}
                        errorActions={errorActions}
                        isSourceVisible={sourceVisible}
                        message={message}
                        versionContent={version.content}
                      />
                    )}
                  </div>
                </Message>
              ))}
            </BranchMessages>

            {/* 多版本消息仍保留分支切换，不参与本轮错误恢复动作。 */}
            {versions.length > 1 && (
              <BranchSelector className='px-0' from={message.from}>
                <BranchPrevious />
                <BranchPage />
                <BranchNext />
              </BranchSelector>
            )}
          </Branch>
        )
      })
    )

  return (
    <Conversation>
      {/* 外层不加 padding，内层居中容器负责和输入框对齐。 */}
      <ConversationContent className='p-0'>
        <div className='mx-auto w-full max-w-4xl px-4 py-4'>{chatContent}</div>
      </ConversationContent>
      <ConversationScrollButton />
    </Conversation>
  )
}
