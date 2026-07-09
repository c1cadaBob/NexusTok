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
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { MESSAGE_ACTION_LABELS } from '../constants'

/**
 * 在生成进行中保护消息动作。
 *
 * Playground 的重试、编辑和删除都会改变会话上下文；如果当前仍在生成中，
 * 这些动作必须被拦截，避免同一轮对话同时出现多个竞争中的请求。
 */
export function useMessageActionGuard(isGenerating: boolean) {
  const { t } = useTranslation()

  const guardAction = useCallback(
    (action: () => void) => {
      return () => {
        if (isGenerating) {
          toast.warning(t(MESSAGE_ACTION_LABELS.WAIT_GENERATION))
          return
        }
        action()
      }
    },
    [isGenerating, t]
  )

  return { guardAction }
}
