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

interface MessageEditorState {
  canSave: boolean
  hasChanged: boolean
  showSaveAndSubmit: boolean
}

/**
 * 计算消息编辑器的按钮状态。
 *
 * Save 与 Save & Submit 都必须满足“内容非空且已经变更”；其中
 * Save & Submit 只对 user 消息开放，避免 assistant 消息保存后意外重新请求上游。
 */
export function getMessageEditorState(
  message: Message,
  editText: string,
  originalText: string
): MessageEditorState {
  const hasChanged = editText !== originalText

  return {
    canSave: editText.trim().length > 0 && hasChanged,
    hasChanged,
    showSaveAndSubmit: message.from === MESSAGE_ROLES.USER,
  }
}
