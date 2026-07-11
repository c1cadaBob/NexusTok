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
import { GridIcon, Table01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  DATA_TABLE_VIEW_MODES,
  type DataTableViewMode,
} from './use-data-table-view-mode'

export type DataTableViewModeToggleProps = {
  value: DataTableViewMode
  onChange: (mode: DataTableViewMode) => void
  className?: string
}

type ViewModeSegment = {
  value: DataTableViewMode
  icon: typeof GridIcon
  tooltip: string
}

export function DataTableViewModeToggle(props: DataTableViewModeToggleProps) {
  const { t } = useTranslation()
  const segments: ViewModeSegment[] = [
    {
      value: DATA_TABLE_VIEW_MODES.CARD,
      icon: GridIcon,
      tooltip: t('Card view'),
    },
    {
      value: DATA_TABLE_VIEW_MODES.TABLE,
      icon: Table01Icon,
      tooltip: t('Table view'),
    },
  ]

  return (
    <div
      role='group'
      aria-label={t('View mode')}
      className={cn(
        'bg-muted/60 inline-flex h-8 items-center rounded-lg border p-0.5',
        props.className
      )}
    >
      {segments.map((segment) => {
        const isActive = segment.value === props.value
        return (
          <Tooltip key={segment.value}>
            <TooltipTrigger
              render={
                <button
                  type='button'
                  onClick={() => props.onChange(segment.value)}
                  aria-pressed={isActive}
                  className={cn(
                    'inline-flex h-full w-7 items-center justify-center rounded-md text-xs font-medium transition-all',
                    isActive
                      ? 'bg-primary text-primary-foreground shadow-sm'
                      : 'text-muted-foreground hover:text-foreground'
                  )}
                >
                  <HugeiconsIcon
                    icon={segment.icon}
                    strokeWidth={2}
                    className='size-3.5'
                    aria-hidden='true'
                  />
                </button>
              }
            />
            <TooltipContent side='bottom' className='text-xs'>
              {segment.tooltip}
            </TooltipContent>
          </Tooltip>
        )
      })}
    </div>
  )
}
