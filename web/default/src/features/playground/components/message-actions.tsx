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
import {
  Check,
  Copy,
  Edit,
  FileCode2,
  MoreHorizontal,
  RefreshCw,
  Trash2,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { TooltipProvider } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { MESSAGE_ACTION_LABELS } from '../constants'
import { useMessageActionGuard } from '../hooks/use-message-action-guard'
import {
  canRegenerateMessage,
  canToggleMessageSource,
  getMessageActionsVisibilityClass,
  getMessageActionState,
  getMessageSourceToggleLabel,
} from '../lib'
import type { Message } from '../types'
import { MessageActionButton } from './message-action-button'

interface MessageActionsProps {
  message: Message
  onCopy?: (message: Message) => void
  onRegenerate?: (message: Message) => void
  onToggleSource?: (message: Message) => void
  onEdit?: (message: Message) => void
  onDelete?: (message: Message) => void
  isSourceVisible?: boolean
  isGenerating?: boolean
  alwaysVisible?: boolean
  className?: string
}

type MessageActionItem = {
  className?: string
  disabled?: boolean
  icon: LucideIcon
  label: string
  onClick: () => void
  variant?: 'default' | 'destructive'
}

export function MessageActions({
  message,
  onCopy,
  onRegenerate,
  onToggleSource,
  onEdit,
  onDelete,
  isSourceVisible = false,
  isGenerating = false,
  alwaysVisible = false,
  className = '',
}: MessageActionsProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const { guardAction } = useMessageActionGuard(isGenerating)

  const { content, hasContent, isAssistant, isLoading, isUser } =
    getMessageActionState(message)
  const isCopied = copiedText === content

  const handleCopy = () => {
    if (!content) {
      toast.warning(t(MESSAGE_ACTION_LABELS.NO_CONTENT))
      return
    }
    copyToClipboard(content)
    onCopy?.(message)
  }

  const handleRegenerate = guardAction(() => onRegenerate?.(message))
  const handleToggleSource = () => onToggleSource?.(message)
  const handleEdit = guardAction(() => onEdit?.(message))
  const handleDelete = guardAction(() => onDelete?.(message))

  const visibilityClass = getMessageActionsVisibilityClass(alwaysVisible)
  const showSourceToggle = canToggleMessageSource({
    hasContent,
    hasToggleHandler: Boolean(onToggleSource),
    isAssistant,
    isLoading,
  })
  const showRegenerate = canRegenerateMessage({
    hasContent,
    hasRegenerateHandler: Boolean(onRegenerate),
    isAssistant,
    isLoading,
    isUser,
  })
  const actions: MessageActionItem[] = []

  if (hasContent) {
    actions.push({
      className: isCopied ? 'text-green-600' : '',
      icon: isCopied ? Check : Copy,
      label: isCopied
        ? MESSAGE_ACTION_LABELS.COPIED
        : MESSAGE_ACTION_LABELS.COPY,
      onClick: handleCopy,
    })
  }

  if (showSourceToggle) {
    actions.push({
      icon: FileCode2,
      label: getMessageSourceToggleLabel(isSourceVisible),
      onClick: handleToggleSource,
    })
  }

  if (showRegenerate) {
    actions.push({
      disabled: isGenerating,
      icon: RefreshCw,
      label: MESSAGE_ACTION_LABELS.REGENERATE,
      onClick: handleRegenerate,
    })
  }

  if (hasContent && onEdit) {
    actions.push({
      disabled: isGenerating,
      icon: Edit,
      label: MESSAGE_ACTION_LABELS.EDIT,
      onClick: handleEdit,
    })
  }

  if (onDelete) {
    actions.push({
      disabled: isGenerating,
      icon: Trash2,
      label: MESSAGE_ACTION_LABELS.DELETE,
      onClick: handleDelete,
      variant: 'destructive',
    })
  }

  if (actions.length === 0) {
    return null
  }

  return (
    <>
      <TooltipProvider delay={300}>
        <div
          className={cn(
            'hidden items-center gap-0.5 transition-opacity md:flex',
            visibilityClass,
            className
          )}
        >
          {actions.map((action) => (
            <MessageActionButton
              className={action.className}
              disabled={action.disabled}
              icon={action.icon}
              key={action.label}
              label={t(action.label)}
              onClick={action.onClick}
              variant={action.variant}
            />
          ))}
        </div>
      </TooltipProvider>

      <div className={cn('md:hidden', className)}>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                aria-label={t('Open menu')}
                className='data-popup-open:bg-muted text-muted-foreground hover:text-foreground size-11'
                size='icon'
                type='button'
                variant='ghost'
              />
            }
          >
            <MoreHorizontal className='size-4' />
            <span className='sr-only'>{t('Open menu')}</span>
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' className='w-44'>
            {actions.map((action) => {
              const Icon = action.icon

              return (
                <DropdownMenuItem
                  className='min-h-11'
                  disabled={action.disabled}
                  key={action.label}
                  onClick={action.onClick}
                  variant={action.variant}
                >
                  {t(action.label)}
                  <DropdownMenuShortcut>
                    <Icon className='size-4' />
                  </DropdownMenuShortcut>
                </DropdownMenuItem>
              )
            })}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </>
  )
}
