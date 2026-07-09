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
import { Check, RotateCcw, Send, X, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CodeBlockEditor } from '@/components/ai-elements/code-block'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { getMessageEditorState, getMessageEditorStyles } from '../lib'
import type { Message } from '../types'

interface PlaygroundMessageEditorProps {
  editText: string
  message: Message
  onCancelEdit?: (open: boolean) => void
  onEditTextChange: (text: string) => void
  onSaveEdit?: (newContent: string) => void
  onSaveEditAndSubmit?: (newContent: string) => void
  originalText: string
}

interface EditorIconButtonProps {
  disabled?: boolean
  icon: LucideIcon
  label: string
  onClick: () => void
  variant?: 'default' | 'ghost'
}

function EditorIconButton({
  disabled = false,
  icon: Icon,
  label,
  onClick,
  variant = 'ghost',
}: EditorIconButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            aria-label={label}
            disabled={disabled}
            onClick={onClick}
            size='icon-sm'
            type='button'
            variant={variant}
          />
        }
      >
        <Icon aria-hidden='true' />
      </TooltipTrigger>
      <TooltipContent>
        <p>{label}</p>
      </TooltipContent>
    </Tooltip>
  )
}

export function PlaygroundMessageEditor({
  editText,
  message,
  onCancelEdit,
  onEditTextChange,
  onSaveEdit,
  onSaveEditAndSubmit,
  originalText,
}: PlaygroundMessageEditorProps) {
  const { t } = useTranslation()
  const { canSave, hasChanged, showSaveAndSubmit } = getMessageEditorState(
    message,
    editText,
    originalText
  )

  const handleCancel = () => {
    if (
      hasChanged &&
      !window.confirm(
        t('You have unsaved changes. Are you sure you want to leave?')
      )
    ) {
      return
    }

    onCancelEdit?.(false)
  }

  const handleKeyDown = (event: globalThis.KeyboardEvent) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      handleCancel()
      return
    }

    if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
      event.preventDefault()
      if (!canSave) return

      if (showSaveAndSubmit) {
        onSaveEditAndSubmit?.(editText)
      } else {
        onSaveEdit?.(editText)
      }
    }
  }

  const editorActions = (
    <TooltipProvider delay={300}>
      <div className='flex items-center gap-1'>
        {/* Save & Submit 只对用户消息有意义，assistant 消息仅保存草稿。 */}
        {showSaveAndSubmit && (
          <EditorIconButton
            disabled={!canSave}
            icon={Send}
            label={t('Save & Submit')}
            onClick={() => onSaveEditAndSubmit?.(editText)}
            variant='default'
          />
        )}
        <EditorIconButton
          disabled={!canSave}
          icon={Check}
          label={t('Save')}
          onClick={() => onSaveEdit?.(editText)}
          variant={showSaveAndSubmit ? 'ghost' : 'default'}
        />
        {hasChanged && (
          <EditorIconButton
            icon={RotateCcw}
            label={t('Reset')}
            onClick={() => onEditTextChange(originalText)}
          />
        )}
        <EditorIconButton icon={X} label={t('Cancel')} onClick={handleCancel} />
      </div>
    </TooltipProvider>
  )

  return (
    <CodeBlockEditor
      actions={editorActions}
      ariaLabel={t('Edit')}
      className={getMessageEditorStyles()}
      language='markdown'
      onChange={onEditTextChange}
      onKeyDown={handleKeyDown}
      rows={8}
      title={
        <span className='inline-flex items-center gap-2'>
          <span>{t('Edit')}</span>
          <span className='text-muted-foreground/80 normal-case'>
            {hasChanged ? t('Unsaved changes') : t('No changes')}
          </span>
        </span>
      }
      value={editText}
    />
  )
}
