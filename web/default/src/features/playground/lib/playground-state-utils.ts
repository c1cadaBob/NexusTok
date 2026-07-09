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
import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../constants'
import type { Message, ParameterEnabled, PlaygroundConfig } from '../types'
import { loadConfig, loadMessages, loadParameterEnabled } from './storage'

export type MessageStateUpdater =
  | Message[]
  | ((previousMessages: Message[]) => Message[])

/**
 * 从本地存储恢复 Playground 配置，并用默认配置补齐缺失字段。
 */
export function getInitialPlaygroundConfig(): PlaygroundConfig {
  return { ...DEFAULT_CONFIG, ...loadConfig() }
}

/**
 * 从本地存储恢复参数开关，并用默认开关补齐缺失字段。
 */
export function getInitialParameterEnabled(): ParameterEnabled {
  return { ...DEFAULT_PARAMETER_ENABLED, ...loadParameterEnabled() }
}

/**
 * 从本地存储恢复消息历史；存储损坏或为空时返回空会话。
 */
export function getInitialMessages(): Message[] {
  return loadMessages() || []
}

/**
 * 应用消息状态更新器，兼容直接数组和函数式 updater 两种调用方式。
 */
export function applyMessageStateUpdate(
  previousMessages: Message[],
  updater: MessageStateUpdater
): Message[] {
  return typeof updater === 'function' ? updater(previousMessages) : updater
}
