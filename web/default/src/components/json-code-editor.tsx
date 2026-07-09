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
import { useMemo, type ComponentProps, type ReactNode } from 'react'
import { Code2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { CodeBlockEditor } from '@/components/ai-elements/code-block'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'

type JsonCodeEditorProps = Omit<
  ComponentProps<'div'>,
  'onChange' | 'onKeyDown' | 'title'
> & {
  ariaLabel?: string
  disabled?: boolean
  onChange: (value: string) => void
  rows?: number
  title?: ReactNode
  value: string
}

type JsonStatus = {
  canFormat: boolean
  label: string
  variant: StatusVariant
}

/**
 * 面向设置页的受控 JSON 代码编辑器。
 *
 * 组件只负责编辑体验、格式化和语法状态提示；业务字段的结构校验、保存
 * 权限和接口提交仍由调用方表单负责，避免 UI 层改变配置语义。
 */
export function JsonCodeEditor({
  ariaLabel,
  className,
  disabled = false,
  onChange,
  rows = 8,
  title,
  value,
  ...props
}: JsonCodeEditorProps) {
  const { t } = useTranslation()
  const resolvedTitle = title ?? t('JSON')
  const resolvedAriaLabel =
    ariaLabel ??
    (typeof resolvedTitle === 'string' ? resolvedTitle : t('JSON Editor'))

  const jsonStatus = useMemo<JsonStatus>(() => {
    const trimmed = value.trim()
    if (!trimmed) {
      return {
        canFormat: false,
        label: t('JSON'),
        variant: 'neutral',
      }
    }

    try {
      JSON.parse(trimmed)
      return {
        canFormat: true,
        label: t('JSON'),
        variant: 'success',
      }
    } catch {
      return {
        canFormat: false,
        label: t('Invalid JSON'),
        variant: 'danger',
      }
    }
  }, [t, value])

  const handleFormatJson = () => {
    const trimmed = value.trim()
    if (!trimmed) return

    try {
      onChange(JSON.stringify(JSON.parse(trimmed), null, 2))
    } catch {
      // 非法草稿保持原样，由状态 badge 和表单校验继续提示管理员修正。
    }
  }

  return (
    <CodeBlockEditor
      actions={
        <>
          <StatusBadge
            copyable={false}
            label={jsonStatus.label}
            variant={jsonStatus.variant}
          />
          <Button
            disabled={disabled || !jsonStatus.canFormat}
            onClick={handleFormatJson}
            size='xs'
            type='button'
            variant='ghost'
          >
            <Code2 data-icon='inline-start' />
            {t('Format JSON')}
          </Button>
        </>
      }
      ariaLabel={resolvedAriaLabel}
      autoFocus={false}
      className={cn(
        'aria-invalid:border-destructive aria-invalid:ring-destructive/20 aria-invalid:ring-3',
        disabled && 'opacity-70',
        className
      )}
      language='json'
      onChange={onChange}
      readOnly={disabled}
      rows={rows}
      title={resolvedTitle}
      value={value}
      {...props}
    />
  )
}
