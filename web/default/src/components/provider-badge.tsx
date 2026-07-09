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
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'
import { StatusBadge, type StatusBadgeProps } from './status-badge'

type ProviderBadgeProps = Omit<
  StatusBadgeProps,
  'children' | 'className' | 'label'
> & {
  badgeClassName?: string
  className?: string
  colorText?: boolean
  iconKey?: string | null
  iconSize?: number
  label: string
}

export function ProviderBadge({
  autoColor,
  badgeClassName,
  className,
  colorText = true,
  copyable = false,
  iconKey,
  iconSize = 14,
  label,
  size = 'sm',
  variant,
  ...badgeProps
}: ProviderBadgeProps) {
  const icon = iconKey ? getLobeIcon(iconKey, iconSize) : null

  return (
    <span
      data-slot='provider-badge'
      className={cn(
        'inline-flex max-w-full min-w-0 items-center gap-1.5',
        className
      )}
    >
      {icon && <span className='flex shrink-0 items-center'>{icon}</span>}
      <StatusBadge
        {...badgeProps}
        label={label}
        autoColor={colorText ? (autoColor ?? label) : undefined}
        variant={colorText ? variant : (variant ?? 'neutral')}
        size={size}
        copyable={copyable}
        className={cn(
          'min-w-0 max-w-full shrink overflow-hidden',
          badgeClassName
        )}
      />
    </span>
  )
}
