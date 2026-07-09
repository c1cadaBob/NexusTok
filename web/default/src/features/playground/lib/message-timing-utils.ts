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

type TranslationFn = (
  key: string,
  options?: Record<string, string | number>
) => string

/**
 * 完成 assistant 消息的耗时计算。
 *
 * startedAt 优先使用请求创建时写入的时间；历史消息缺少该字段时用
 * createdAt 兜底，仍然缺失则使用完成时间，保证不会产生 NaN 或负数。
 */
export function completeAssistantTiming(
  message: Message,
  completedAt: number = Date.now()
): Message {
  if (message.from !== MESSAGE_ROLES.ASSISTANT) {
    return message
  }

  const startedAt = message.startedAt ?? message.createdAt ?? completedAt

  return {
    ...message,
    startedAt,
    completedAt,
    durationMs: Math.max(0, completedAt - startedAt),
  }
}

/**
 * 确保 reasoning 开始时间存在。
 *
 * reasoning 可能来自独立 `reasoning_content` delta，也可能来自 `<think>` 标签；
 * 首次看到 reasoning 时记录 startedAt，后续增量只追加内容不重置计时。
 */
export function startReasoningTiming(
  message: Message,
  startedAt: number = Date.now()
): NonNullable<Message['reasoning']> {
  return {
    content: message.reasoning?.content ?? '',
    duration: message.reasoning?.duration ?? 0,
    startedAt: message.reasoning?.startedAt ?? startedAt,
    completedAt: message.reasoning?.completedAt,
    durationMs: message.reasoning?.durationMs,
  }
}

/**
 * 完成 reasoning 耗时计算。
 *
 * 已经完成过的 reasoning 不重复计算，避免后续 finalize 或 sanitize
 * 多次运行导致 duration 被刷新。
 */
export function completeReasoningTiming(
  message: Message,
  completedAt: number = Date.now()
): Message {
  if (!message.reasoning || message.reasoning.durationMs !== undefined) {
    return message
  }

  const startedAt =
    message.reasoning.startedAt ?? message.startedAt ?? completedAt
  const durationMs = Math.max(0, completedAt - startedAt)

  return {
    ...message,
    reasoning: {
      ...message.reasoning,
      startedAt,
      completedAt,
      durationMs,
      duration: Math.ceil(durationMs / 1000),
    },
  }
}

/**
 * 格式化消息发送时间；缺失或非法时间不展示。
 */
export function formatMessageTime(timestamp?: number): string | undefined {
  if (typeof timestamp !== 'number' || !Number.isFinite(timestamp)) {
    return undefined
  }

  const date = new Date(timestamp)
  if (Number.isNaN(date.getTime())) {
    return undefined
  }

  return new Intl.DateTimeFormat(undefined, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date)
}

/**
 * 格式化耗时，毫秒级保留整数，秒级保留两位小数。
 */
export function formatDuration(
  durationMs: number | undefined,
  t: TranslationFn
): string | undefined {
  if (typeof durationMs !== 'number' || !Number.isFinite(durationMs)) {
    return undefined
  }

  if (durationMs < 1000) {
    return t('{{value}}ms', { value: Math.max(1, Math.round(durationMs)) })
  }

  return t('{{value}}s', { value: (durationMs / 1000).toFixed(2) })
}
