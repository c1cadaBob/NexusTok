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
import { Edit, RefreshCw, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { TooltipProvider } from '@/components/ui/tooltip'
import { MessageActionButton } from './message-action-button'

interface MessageErrorActionsProps {
  disabled?: boolean
  onDelete?: () => void
  onEditPrompt?: () => void
  onRetry?: () => void
}

/**
 * 渲染错误消息的恢复动作。
 *
 * 这些按钮只服务失败态：重试当前 assistant 错误消息、编辑上一条用户 prompt、
 * 或删除错误消息本身，避免和普通消息动作混在一起造成语义歧义。
 */
export function MessageErrorActions({
  disabled = false,
  onDelete,
  onEditPrompt,
  onRetry,
}: MessageErrorActionsProps) {
  const { t } = useTranslation()

  if (!onRetry && !onEditPrompt && !onDelete) {
    return null
  }

  return (
    <TooltipProvider delay={300}>
      <div className='flex flex-wrap items-center gap-0.5 pt-2'>
        {onRetry && (
          <MessageActionButton
            disabled={disabled}
            icon={RefreshCw}
            label={t('Retry')}
            onClick={onRetry}
          />
        )}

        {onEditPrompt && (
          <MessageActionButton
            disabled={disabled}
            icon={Edit}
            label={t('Edit')}
            onClick={onEditPrompt}
          />
        )}

        {onDelete && (
          <MessageActionButton
            disabled={disabled}
            icon={Trash2}
            label={t('Delete')}
            onClick={onDelete}
            variant='destructive'
          />
        )}
      </div>
    </TooltipProvider>
  )
}
