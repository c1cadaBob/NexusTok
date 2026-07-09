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
import type { Message } from '../types'
import { parseThinkTags } from './message-reasoning-utils'

type MessageSources = NonNullable<Message['sources']>

interface MessageContentState {
  displayContent: string
  hasReasoning: boolean
  hasSources: boolean
  isError: boolean
  isMessageFinal: boolean
  reasoningContent?: string
  showLoader: boolean
  showMessageContent: boolean
  sources: MessageSources
}

/**
 * 判断 assistant 占位是否应该显示加载提示。
 *
 * 只有在没有 reasoning 流、且正文还未到达时显示 loading，避免 reasoning
 * 正在流式输出时同时显示正文加载动画造成重复反馈。
 */
function shouldShowMessageLoader(
  message: Message,
  isAssistant: boolean,
  versionContent: string
): boolean {
  return (
    isAssistant &&
    !message.isReasoningStreaming &&
    (message.status === MESSAGE_STATUS.LOADING ||
      (message.status === MESSAGE_STATUS.STREAMING && !versionContent))
  )
}

/**
 * 判断当前版本正文是否应该进入 Response 渲染。
 *
 * reasoning 流式阶段先展示 reasoning 区域，正文为空或尚未可见时不渲染
 * Response，避免 Markdown renderer 处理无意义空内容。
 */
function shouldShowMessageContent(
  message: Message,
  versionContent: string
): boolean {
  return (
    (message.from === MESSAGE_ROLES.USER || !message.isReasoningStreaming) &&
    versionContent.length > 0
  )
}

/**
 * 取得最终交给 Response 的可见正文。
 *
 * assistant 的原始版本内容可能带 `<think>` 标签；推理内容单独显示在
 * Reasoning 中，正文区域只保留用户真正要阅读的 visible content。
 */
function getDisplayContent(message: Message, versionContent: string): string {
  if (message.from !== MESSAGE_ROLES.ASSISTANT) {
    return versionContent
  }

  return parseThinkTags(versionContent).visibleContent
}

/**
 * 汇总单条 Playground 消息正文渲染需要的派生状态。
 */
export function getMessageContentState(
  message: Message,
  versionContent: string
): MessageContentState {
  const isAssistant = message.from === MESSAGE_ROLES.ASSISTANT
  const sources = message.sources ?? []
  const reasoningContent = isAssistant ? message.reasoning?.content : undefined
  const isError = message.status === MESSAGE_STATUS.ERROR

  return {
    displayContent: getDisplayContent(message, versionContent),
    hasReasoning: Boolean(reasoningContent),
    hasSources: sources.length > 0,
    isError,
    isMessageFinal:
      message.status !== MESSAGE_STATUS.LOADING &&
      message.status !== MESSAGE_STATUS.STREAMING,
    reasoningContent,
    showLoader: shouldShowMessageLoader(message, isAssistant, versionContent),
    showMessageContent: shouldShowMessageContent(message, versionContent),
    sources,
  }
}
