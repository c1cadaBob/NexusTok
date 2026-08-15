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
import { useMemo, useState } from 'react'
import { type Column } from '@tanstack/react-table'
import {
  ArrowDown as ArrowDownIcon,
  ArrowUp as ArrowUpIcon,
  ChevronsUpDown as CaretSortIcon,
  EyeOff as EyeNoneIcon,
  Filter,
  X,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'

type MinimumRatioModelSelectorProps = {
  modelOptions: string[]
  selectedModel: string
  onModelChange: (model: string) => void
}

type MinimumRatioColumnHeaderProps<TData, TValue> =
  MinimumRatioModelSelectorProps & {
    column: Column<TData, TValue>
  }

function RatioModelMenuBody({
  modelOptions,
  selectedModel,
  onModelChange,
}: MinimumRatioModelSelectorProps) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const filteredModels = useMemo(() => {
    const normalized = search.trim().toLowerCase()
    if (!normalized) return modelOptions
    return modelOptions.filter((model) =>
      model.toLowerCase().includes(normalized)
    )
  }, [modelOptions, search])

  return (
    <div className='flex min-w-0 flex-col gap-2 p-2'>
      <div className='flex items-center justify-between gap-2'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Ratio model')}
        </span>
        {selectedModel && (
          <Button
            variant='ghost'
            size='sm'
            className='h-7 px-2'
            onClick={() => onModelChange('')}
          >
            <X data-icon='inline-start' />
            {t('Clear ratio model')}
          </Button>
        )}
      </div>
      <Input
        value={search}
        onChange={(event) => setSearch(event.target.value)}
        onClick={(event) => event.stopPropagation()}
        onKeyDown={(event) => event.stopPropagation()}
        placeholder={t('Select ratio model')}
        className='h-8'
      />
      <div className='max-h-64 overflow-y-auto'>
        <DropdownMenuGroup>
          <DropdownMenuItem onClick={() => onModelChange('')}>
            <span
              className={cn(
                'min-w-0 flex-1 truncate',
                !selectedModel && 'font-medium'
              )}
            >
              {t('All synced key models')}
            </span>
          </DropdownMenuItem>
          {filteredModels.length === 0 ? (
            <div className='text-muted-foreground px-2 py-3 text-xs'>
              {t('No synced key models found')}
            </div>
          ) : (
            filteredModels.map((model) => (
              <DropdownMenuItem key={model} onClick={() => onModelChange(model)}>
                <span
                  className={cn(
                    'min-w-0 flex-1 truncate font-mono text-xs',
                    selectedModel === model && 'font-semibold'
                  )}
                  title={model}
                >
                  {model}
                </span>
              </DropdownMenuItem>
            ))
          )}
        </DropdownMenuGroup>
      </div>
    </div>
  )
}

export function MinimumRatioColumnHeader<TData, TValue>({
  column,
  modelOptions,
  selectedModel,
  onModelChange,
}: MinimumRatioColumnHeaderProps<TData, TValue>) {
  const { t } = useTranslation()
  const title = selectedModel
    ? t('Minimum ratio for {{model}}', { model: selectedModel })
    : t('Minimum Ratio')

  return (
    <div className='flex min-w-0 items-center gap-2'>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant='ghost'
              size='sm'
              className='data-popup-open:bg-accent -ms-3 h-8 max-w-[13rem]'
              title={title}
            />
          }
        >
          <span className='min-w-0 truncate'>
            {selectedModel ? (
              <>
                {t('Minimum Ratio')}
                <span className='text-muted-foreground'>
                  {' '}
                  · {selectedModel}
                </span>
              </>
            ) : (
              t('Minimum Ratio')
            )}
          </span>
          {column.getIsSorted() === 'desc' ? (
            <ArrowDownIcon data-icon='inline-end' />
          ) : column.getIsSorted() === 'asc' ? (
            <ArrowUpIcon data-icon='inline-end' />
          ) : (
            <CaretSortIcon data-icon='inline-end' />
          )}
        </DropdownMenuTrigger>
        <DropdownMenuContent align='start' className='w-72'>
          <DropdownMenuGroup>
            <DropdownMenuItem onClick={() => column.toggleSorting(false)}>
              <ArrowUpIcon />
              {t('Asc')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => column.toggleSorting(true)}>
              <ArrowDownIcon />
              {t('Desc')}
            </DropdownMenuItem>
            {column.getCanHide() && (
              <DropdownMenuItem onClick={() => column.toggleVisibility(false)}>
                <EyeNoneIcon />
                {t('Hide')}
              </DropdownMenuItem>
            )}
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <RatioModelMenuBody
            modelOptions={modelOptions}
            selectedModel={selectedModel}
            onModelChange={onModelChange}
          />
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

export function MinimumRatioModelFilterButton({
  modelOptions,
  selectedModel,
  onModelChange,
}: MinimumRatioModelSelectorProps) {
  const { t } = useTranslation()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant='outline'
            size='sm'
            className='h-8 max-w-full justify-start'
            title={
              selectedModel
                ? t('Minimum ratio for {{model}}', { model: selectedModel })
                : t('Ratio model')
            }
          />
        }
      >
        <Filter data-icon='inline-start' />
        <span className='truncate'>{selectedModel || t('Ratio model')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='start' className='w-72'>
        <RatioModelMenuBody
          modelOptions={modelOptions}
          selectedModel={selectedModel}
          onModelChange={onModelChange}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
