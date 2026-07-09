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

/**
 * 切换消息原始响应可见状态。
 *
 * source 视图属于当前页面会话的临时 UI 状态，不写入消息结构或
 * localStorage；这里始终返回新的 Set，保证 React 能稳定感知状态变化。
 */
export function toggleMessageSourceKey(
  currentKeys: ReadonlySet<string>,
  messageKey: string
): ReadonlySet<string> {
  const nextKeys = new Set(currentKeys)

  if (nextKeys.has(messageKey)) {
    nextKeys.delete(messageKey)
  } else {
    nextKeys.add(messageKey)
  }

  return nextKeys
}

/**
 * 判断指定消息当前是否处于原始响应视图。
 */
export function isMessageSourceVisible(
  currentKeys: ReadonlySet<string>,
  messageKey: string
): boolean {
  return currentKeys.has(messageKey)
}
