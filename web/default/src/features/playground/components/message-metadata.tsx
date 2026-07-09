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
import { useTranslation } from 'react-i18next'
import { formatDuration, formatMessageTime } from '../lib'
import type { Message } from '../types'

interface MessageMetadataProps {
  message: Message
}

/**
 * 展示 Playground 消息的轻量时间元信息。
 *
 * timing 字段是可选的，历史 localStorage 消息缺字段时不渲染任何内容，
 * 避免为了展示元信息而引入存储迁移或历史消息兼容风险。
 */
export function MessageMetadata({ message }: MessageMetadataProps) {
  const { t } = useTranslation()
  const translateDuration = (
    key: string,
    options?: Record<string, string | number>
  ) => t(key, options)
  const messageTime = formatMessageTime(message.createdAt)
  const duration = formatDuration(message.durationMs, translateDuration)

  if (!messageTime && !duration) {
    return null
  }

  return (
    <div className='text-muted-foreground mt-1 flex min-h-4 flex-wrap items-center gap-1.5 text-xs leading-none'>
      {messageTime && <time>{messageTime}</time>}
      {duration && (
        <>
          {messageTime && <span aria-hidden='true'>·</span>}
          <span>{t('Response time: {{duration}}', { duration })}</span>
        </>
      )}
    </div>
  )
}
