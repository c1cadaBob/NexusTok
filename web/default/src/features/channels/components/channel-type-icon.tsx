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
import { Network, Repeat2, Server } from 'lucide-react'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'
import { CHANNEL_TYPE_OPTIONS } from '../constants'
import { getChannelTypeIcon } from '../lib/channel-utils'

type ChannelTypeIconProps = {
  type: number
  size?: number
  className?: string
}

// new-api 和 sub2api 是上游账号同步平台，不应继续展示 OpenAI 图标。
// 它们在 Relay 层走 OpenAI 兼容协议，但管理端图标要表达平台身份，
// 避免管理员把“请求协议兼容”误读成“只允许 OpenAI 官方格式”。
export function ChannelTypeIcon({
  type,
  size = 16,
  className,
}: ChannelTypeIconProps) {
  if (type === 59 || type === 60) {
    const Icon = type === 59 ? Network : Repeat2
    return (
      <Icon
        className={cn('text-foreground shrink-0', className)}
        style={{ width: size, height: size }}
        aria-hidden='true'
      />
    )
  }

  const isKnownType = CHANNEL_TYPE_OPTIONS.some(
    (option) => option.value === type
  )
  if (!isKnownType) {
    return (
      <Server
        className={cn('text-muted-foreground shrink-0', className)}
        style={{ width: size, height: size }}
        aria-hidden='true'
      />
    )
  }

  return (
    <span className={cn('inline-flex shrink-0', className)}>
      {getLobeIcon(`${getChannelTypeIcon(type)}.Color`, size)}
    </span>
  )
}
