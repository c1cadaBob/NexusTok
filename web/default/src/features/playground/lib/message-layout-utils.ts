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
import type { Message, PlaygroundMessageLayoutMode } from '../types'

export type MessageAlignment = 'left' | 'right'

/**
 * 根据 Playground 布局模式计算单条消息的视觉对齐方向。
 *
 * `alternating` 是当前默认体验：用户消息靠右，assistant/system 消息靠左；
 * `left` 为后续“全部左对齐”设置预留底座，所有角色都回到左侧。
 */
export function getMessageAlignment(
  message: Message,
  layoutMode: PlaygroundMessageLayoutMode
): MessageAlignment {
  if (layoutMode === 'left') {
    return 'left'
  }

  return message.from === MESSAGE_ROLES.USER ? 'right' : 'left'
}

/**
 * 将对齐语义转换为 Tailwind 布局类。
 *
 * 该函数只负责内容列和轻量元信息的对齐，不改公共 `Message` 组件的
 * role-based flex 规则，避免影响 Playground 之外的 AI element 使用方。
 */
export function getMessageAlignmentClass(alignment: MessageAlignment): string {
  return alignment === 'right'
    ? 'items-end text-right'
    : 'items-start text-left'
}
