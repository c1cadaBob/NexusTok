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
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { Loader } from '@/components/ai-elements/loader'
import { MessageContent } from '@/components/ai-elements/message'
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
import { getMessageContentState } from '../lib'
import { getMessageContentStyles } from '../lib/message-styles'
import type { Message } from '../types'
import { MessageError } from './message-error'
import { MessageMetadata } from './message-metadata'

interface PlaygroundMessageContentProps {
  actions: ReactNode
  errorActions?: ReactNode
  isSourceVisible?: boolean
  message: Message
  versionContent: string
}

export function PlaygroundMessageContent({
  actions,
  errorActions,
  isSourceVisible = false,
  message,
  versionContent,
}: PlaygroundMessageContentProps) {
  const { t } = useTranslation()
  const {
    displayContent,
    hasReasoning,
    hasSources,
    isError,
    isMessageFinal,
    reasoningContent,
    showLoader,
    showMessageContent,
    sources,
  } = getMessageContentState(message, versionContent)

  return (
    <>
      {/* 来源列表保留在消息内容之前，便于用户先判断引用数量。 */}
      {hasSources && (
        <Sources>
          <SourcesTrigger count={sources.length} />
          <SourcesContent>
            {sources.map((source, sourceIndex) => (
              <Source
                href={source.href}
                key={`${message.key}-source-${sourceIndex}`}
                title={source.title}
              />
            ))}
          </SourcesContent>
        </Sources>
      )}

      {/* 推理内容与正文分离，流式状态继续交给 Reasoning 组件处理。 */}
      {hasReasoning && (
        <Reasoning
          defaultOpen={true}
          isStreaming={message.isReasoningStreaming}
        >
          <ReasoningTrigger />
          <ReasoningContent>{reasoningContent ?? ''}</ReasoningContent>
        </Reasoning>
      )}

      {/* 没有正文 delta 时显示加载提示，避免空白 assistant 消息让用户误判。 */}
      {showLoader && (
        <div className='flex items-center gap-2 py-2'>
          <Loader />
          <Shimmer className='text-sm' duration={1}>
            {t('Responding...')}
          </Shimmer>
        </div>
      )}

      {/* 错误消息使用专门恢复动作，普通消息继续使用通用动作组。 */}
      {isError ? (
        <>
          <MessageError message={message} className='mb-2' />
          <MessageMetadata message={message} />
          {errorActions}
        </>
      ) : (
        <>
          {showMessageContent &&
            (isSourceVisible ? (
              <div className='flex w-full flex-col gap-2'>
                <div className='text-muted-foreground text-xs font-medium'>
                  {t('Raw response')}
                </div>
                <CodeBlock
                  code={versionContent}
                  className='my-0 max-h-[min(520px,60vh)] max-w-full overflow-auto'
                  language='markdown'
                  showLineNumbers
                >
                  <CodeBlockCopyButton aria-label={t('Copy')} />
                </CodeBlock>
              </div>
            ) : (
              <MessageContent
                variant='flat'
                className={cn(getMessageContentStyles())}
              >
                <Response final={isMessageFinal}>{displayContent}</Response>
              </MessageContent>
            ))}
          {(showMessageContent || hasReasoning) && (
            <MessageMetadata message={message} />
          )}
          {showMessageContent && actions}
        </>
      )}
    </>
  )
}
