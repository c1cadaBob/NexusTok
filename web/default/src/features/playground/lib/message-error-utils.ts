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
import { MESSAGE_STATUS } from '../constants'
import type { Message } from '../types'
import { getMessageContent } from './message-utils'

export const FALLBACK_ERROR_CONTENT = 'An unknown error occurred'
export const MODEL_PRICING_SETTINGS_PATH =
  '/system-settings/billing/model-pricing'

const MODEL_PRICE_ERROR_CODE = 'model_price_error'

export type MessageErrorState = {
  content: string
  kind: 'generic' | 'model-price'
  showSettingsLink: boolean
}

/**
 * 判断当前角色是否具备管理端设置入口展示资格。
 *
 * 这里保持 NexusTok 现有前端语义：Root/Admin 的角色值不低于 10。
 * 该 helper 仅控制 Playground 错误卡片上的快捷入口显示，服务端权限仍是最终边界。
 */
export function isAdminRole(role?: number | null): boolean {
  return role != null && role >= 10
}

/**
 * 判断消息是否为 Playground 错误消息。
 */
export function isErrorMessage(message: Message): boolean {
  return message.status === MESSAGE_STATUS.ERROR
}

/**
 * 计算错误卡片需要的展示状态。
 *
 * 返回 null 表示消息不是错误态；content 为空时使用稳定 fallback key，
 * 由组件层决定是否交给 i18n 翻译，避免 helper 依赖 React/i18next。
 */
export function getMessageErrorState(
  message: Message,
  isAdmin: boolean
): MessageErrorState | null {
  if (!isErrorMessage(message)) {
    return null
  }

  const content = getMessageContent(message) || FALLBACK_ERROR_CONTENT
  const isModelPriceError = message.errorCode === MODEL_PRICE_ERROR_CODE

  return {
    content,
    kind: isModelPriceError ? 'model-price' : 'generic',
    showSettingsLink: isModelPriceError && isAdmin,
  }
}
