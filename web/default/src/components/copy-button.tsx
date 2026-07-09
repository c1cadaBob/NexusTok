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
import { type ReactNode, useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

interface CopyButtonProps {
  value?: string
  resolveValue?: () => Promise<string | null | undefined>
  children?: ReactNode
  className?: string
  iconClassName?: string
  variant?: 'ghost' | 'outline' | 'default' | 'secondary' | 'destructive'
  size?: 'default' | 'sm' | 'lg' | 'icon'
  tooltip?: string
  successTooltip?: string
  disabled?: boolean
  'aria-label'?: string
}

export function CopyButton({
  value,
  resolveValue,
  children,
  className,
  iconClassName,
  variant = 'ghost',
  size = 'icon',
  tooltip,
  successTooltip,
  disabled = false,
  'aria-label': ariaLabel,
}: CopyButtonProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const [pending, setPending] = useState(false)
  const [lastCopiedText, setLastCopiedText] = useState<string | null>(null)
  const isCopied = Boolean(
    copiedText && (copiedText === value || copiedText === lastCopiedText)
  )
  const resolvedTooltip = tooltip ?? t('Copy to clipboard')
  const resolvedSuccessTooltip = successTooltip ?? t('Copied!')
  const resolvedAriaLabel = ariaLabel ?? resolvedTooltip
  const copiedAriaLabel = t('Copied')

  const handleCopy = async () => {
    if (disabled || pending) return
    setPending(true)
    try {
      const nextValue = resolveValue ? await resolveValue() : value
      if (!nextValue) return
      const copied = await copyToClipboard(nextValue)
      if (copied) {
        setLastCopiedText(nextValue)
      }
    } finally {
      setPending(false)
    }
  }

  const button = (
    <Button
      variant={variant}
      size={size}
      className={cn('shrink-0', className)}
      onClick={handleCopy}
      disabled={disabled || pending}
      aria-label={isCopied ? copiedAriaLabel : resolvedAriaLabel}
    >
      {pending ? (
        <Spinner className={cn(iconClassName)} />
      ) : isCopied ? (
        <Check className={cn('text-success', iconClassName)} />
      ) : (
        <Copy className={cn(iconClassName)} />
      )}
      {children}
    </Button>
  )

  if (tooltip || successTooltip) {
    return (
      <Tooltip>
        <TooltipTrigger render={button}></TooltipTrigger>
        <TooltipContent>
          <p>{isCopied ? resolvedSuccessTooltip : resolvedTooltip}</p>
        </TooltipContent>
      </Tooltip>
    )
  }

  return button
}
