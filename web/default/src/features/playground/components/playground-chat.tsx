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
import { useEffect, useMemo, useState } from 'react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
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
import { Loader } from '@/components/ai-elements/loader'
import { Message, MessageContent } from '@/components/ai-elements/message'
import {
  Reasoning,
  ReasoningContent,
  ReasoningTrigger,
} from '@/components/ai-elements/reasoning'
import { Response } from '@/components/ai-elements/response'
import { Shimmer } from '@/components/ai-elements/shimmer'
import {
  Source,
  Sources,
  SourcesContent,
  SourcesTrigger,
} from '@/components/ai-elements/sources'
import { MESSAGE_ROLES, MESSAGE_STATUS } from '../constants'
import { getMessageContentStyles } from '../lib/message-styles'
import { parseThinkTags } from '../lib/message-utils'
import type { Message as MessageType } from '../types'
import { MessageActions } from './message-actions'
import { MessageError } from './message-error'
import { MessageErrorActions } from './message-error-actions'

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
}

/**
 * 从错误 assistant 消息向前查找最近的一条用户消息。
 *
 * 错误恢复里的 Edit 应该编辑触发失败的 prompt，而不是编辑 assistant
 * 错误占位消息本身；按索引倒序查找可以兼容中间插入 system/source 等消息的情况。
 */
function getPreviousUserMessage(
  messages: MessageType[],
  messageIndex: number
): MessageType | null {
  for (let index = messageIndex - 1; index >= 0; index--) {
    const candidate = messages[index]

    if (candidate?.from === MESSAGE_ROLES.USER) {
      return candidate
    }
  }

  return null
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
}: PlaygroundChatProps) {
  const [editText, setEditText] = useState('')
  const [originalText, setOriginalText] = useState('')

  useEffect(() => {
    if (!editingKey) return
    const message = messages.find((m) => m.key === editingKey)
    const content = message?.versions?.[0]?.content || ''
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setEditText(content)

    setOriginalText(content)
  }, [editingKey, messages])

  const isEditing = (key: string) => editingKey === key
  const isEmpty = useMemo(() => !editText.trim(), [editText])
  const isChanged = useMemo(
    () => editText !== originalText,
    [editText, originalText]
  )
  return (
    <Conversation>
      {/* 外层不加 padding，内层居中容器负责和输入框对齐。 */}
      <ConversationContent className='p-0'>
        <div className='mx-auto w-full max-w-4xl px-4 py-4'>
          {messages.map((message, messageIndex) => {
            const { versions = [] } = message
            const isLastAssistantMessage =
              messageIndex === messages.length - 1 &&
              message.from === MESSAGE_ROLES.ASSISTANT
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
                        {isEditing(message.key) ? (
                          <div className='flex flex-col gap-2'>
                            <Textarea
                              value={editText}
                              onChange={(e) => setEditText(e.target.value)}
                              className='font-mono text-sm'
                              rows={8}
                            />
                            <div className='flex gap-2'>
                              {/* Save & Submit 只对用户消息有意义，assistant 消息仅保存草稿。 */}
                              {message.from === MESSAGE_ROLES.USER && (
                                <Button
                                  size='sm'
                                  onClick={() =>
                                    onSaveEditAndSubmit?.(editText)
                                  }
                                  disabled={isEmpty || !isChanged}
                                >
                                  Save & Submit
                                </Button>
                              )}
                              <Button
                                size='sm'
                                onClick={() => onSaveEdit?.(editText)}
                                disabled={isEmpty || !isChanged}
                              >
                                Save
                              </Button>
                              <Button
                                size='sm'
                                variant='outline'
                                onClick={() => onCancelEdit?.(false)}
                              >
                                Cancel
                              </Button>
                            </div>
                          </div>
                        ) : (
                          <>
                            {(() => {
                              const isAssistant =
                                message.from === MESSAGE_ROLES.ASSISTANT
                              const hasSources = !!message.sources?.length
                              const showReasoning =
                                isAssistant && !!message.reasoning?.content
                              const showLoader =
                                isAssistant &&
                                !message.isReasoningStreaming &&
                                (message.status === 'loading' ||
                                  (message.status === 'streaming' &&
                                    !version.content))
                              const showMessageContent =
                                (message.from === MESSAGE_ROLES.USER ||
                                  !message.isReasoningStreaming) &&
                                !!version.content

                              // assistant 消息会把 <think> 内容放入 reasoning 区域，正文只渲染可见部分。
                              const displayContent = isAssistant
                                ? parseThinkTags(version.content).visibleContent
                                : version.content
                              const previousUserMessage =
                                message.status === MESSAGE_STATUS.ERROR
                                  ? getPreviousUserMessage(
                                      messages,
                                      messageIndex
                                    )
                                  : null

                              const actions = (
                                <MessageActions
                                  message={message}
                                  onCopy={onCopyMessage}
                                  onRegenerate={onRegenerateMessage}
                                  onEdit={onEditMessage}
                                  onDelete={onDeleteMessage}
                                  isGenerating={isGenerating}
                                  alwaysVisible={isLastAssistantMessage}
                                  className='mt-1'
                                />
                              )

                              return (
                                <>
                                  {/* 来源列表保留在消息内容之前，便于用户先判断引用数量。 */}
                                  {hasSources && (
                                    <Sources>
                                      <SourcesTrigger
                                        count={message.sources!.length}
                                      />
                                      <SourcesContent>
                                        {message.sources!.map(
                                          (source, sourceIndex) => (
                                            <Source
                                              href={source.href}
                                              key={`${message.key}-source-${sourceIndex}`}
                                              title={source.title}
                                            />
                                          )
                                        )}
                                      </SourcesContent>
                                    </Sources>
                                  )}

                                  {/* 推理内容与正文分离，流式状态继续交给 Reasoning 组件处理。 */}
                                  {showReasoning && (
                                    <Reasoning
                                      defaultOpen={true}
                                      isStreaming={message.isReasoningStreaming}
                                    >
                                      <ReasoningTrigger />
                                      <ReasoningContent>
                                        {message.reasoning!.content}
                                      </ReasoningContent>
                                    </Reasoning>
                                  )}

                                  {/* 没有正文 delta 时显示加载提示，避免空白 assistant 消息让用户误判。 */}
                                  {showLoader && (
                                    <div className='flex items-center gap-2 py-2'>
                                      <Loader />
                                      <Shimmer className='text-sm' duration={1}>
                                        Responding...
                                      </Shimmer>
                                    </div>
                                  )}

                                  {/* 错误消息使用专门恢复动作，普通消息继续使用通用动作组。 */}
                                  {message.status === 'error' ? (
                                    <>
                                      <MessageError
                                        message={message}
                                        className='mb-2'
                                      />
                                      <MessageErrorActions
                                        disabled={isGenerating}
                                        onRetry={
                                          onRegenerateMessage
                                            ? () =>
                                                onRegenerateMessage(message)
                                            : undefined
                                        }
                                        onEditPrompt={
                                          onEditMessage && previousUserMessage
                                            ? () =>
                                                onEditMessage(
                                                  previousUserMessage
                                                )
                                            : undefined
                                        }
                                        onDelete={
                                          onDeleteMessage
                                            ? () => onDeleteMessage(message)
                                            : undefined
                                        }
                                      />
                                    </>
                                  ) : (
                                    showMessageContent && (
                                      <>
                                        <MessageContent
                                          variant='flat'
                                          className={cn(
                                            getMessageContentStyles()
                                          )}
                                        >
                                          <Response
                                            final={
                                              message.status !==
                                              MESSAGE_STATUS.STREAMING
                                            }
                                          >
                                            {displayContent}
                                          </Response>
                                        </MessageContent>
                                        {actions}
                                      </>
                                    )
                                  )}
                                </>
                              )
                            })()}
                          </>
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
          })}
        </div>
      </ConversationContent>
      <ConversationScrollButton />
    </Conversation>
  )
}
