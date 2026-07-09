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
import type { KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { getMessageEditorState } from '../lib'
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
  const { canSave, showSaveAndSubmit } = getMessageEditorState(
    message,
    editText,
    originalText
  )

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      onCancelEdit?.(false)
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

  return (
    <div className='flex flex-col gap-2'>
      <Textarea
        className='font-mono text-sm'
        onChange={(event) => onEditTextChange(event.target.value)}
        onKeyDown={handleKeyDown}
        rows={8}
        value={editText}
      />
      <div className='flex flex-wrap gap-2'>
        {/* Save & Submit 只对用户消息有意义，assistant 消息仅保存草稿。 */}
        {showSaveAndSubmit && (
          <Button
            disabled={!canSave}
            onClick={() => onSaveEditAndSubmit?.(editText)}
            size='sm'
          >
            {t('Save & Submit')}
          </Button>
        )}
        <Button
          disabled={!canSave}
          onClick={() => onSaveEdit?.(editText)}
          size='sm'
        >
          {t('Save')}
        </Button>
        <Button
          onClick={() => onCancelEdit?.(false)}
          size='sm'
          variant='outline'
        >
          {t('Cancel')}
        </Button>
      </div>
    </div>
  )
}
