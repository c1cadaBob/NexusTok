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
import * as React from 'react'
import { Add01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { copyToClipboard } from '@/lib/copy-to-clipboard'
import { cn } from '@/lib/utils'
import {
  Combobox,
  ComboboxChip,
  ComboboxChips,
  ComboboxChipsInput,
  ComboboxCollection,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxItem,
  ComboboxList,
  ComboboxSeparator,
  ComboboxValue,
  useComboboxAnchor,
} from '@/components/ui/combobox'

export type Option = {
  label: string
  value: string
}

interface MultiSelectProps {
  options: Option[]
  selected: string[]
  onChange: (values: string[]) => void
  placeholder?: string
  emptyText?: string
  maxVisibleChips?: number
  renderSelectedSummary?: (values: string[]) => React.ReactNode
  className?: string
  allowCreate?: boolean
  createLabel?: string
  id?: string
  disabled?: boolean
  copyChipOnClick?: boolean
  isLoading?: boolean
  loadingText?: string
  searchValue?: string
  onSearchChange?: (value: string) => void
  contentFooter?: React.ReactNode
}

const COMMA_REGEX = /[,，\n]/

export function filterMultiSelectItems(
  items: string[],
  inputValue: string,
  labelMap: ReadonlyMap<string, string> = new Map()
): string[] {
  const normalizedInput = inputValue.trim().toLowerCase()
  if (!normalizedInput) return items

  return items.filter((item) => {
    const label = labelMap.get(item) ?? item
    return (
      item.toLowerCase().includes(normalizedInput) ||
      label.toLowerCase().includes(normalizedInput)
    )
  })
}

function splitDraft(value: string): { completed: string[]; draft: string } {
  if (!COMMA_REGEX.test(value)) {
    return { completed: [], draft: value }
  }

  const normalized = value.replaceAll('，', ',').replaceAll('\n', ',')
  const parts = normalized.split(',')
  const draft = parts.at(-1) ?? ''
  const completed = parts
    .slice(0, -1)
    .map((part) => part.trim())
    .filter(Boolean)

  return { completed, draft }
}

export function canCreateMultiSelectValue({
  allowCreate,
  inputValue,
  selected,
  options,
  isLoading = false,
}: {
  allowCreate: boolean
  inputValue: string
  selected: readonly string[]
  options: readonly Option[]
  isLoading?: boolean
}): boolean {
  const trimmedInput = inputValue.trim()
  if (!allowCreate || isLoading || trimmedInput.length === 0) return false

  const normalizedInput = trimmedInput.toLowerCase()
  const selectedSet = new Set(
    selected.map((value) => value.trim().toLowerCase())
  )
  return !(
    selectedSet.has(normalizedInput) ||
    options.some(
      (option) =>
        option.value.toLowerCase() === normalizedInput ||
        option.label.toLowerCase() === normalizedInput
    )
  )
}

// 芯片式多选。它基于项目 Base UI Combobox，保证输入值会参与真实过滤，
// 同时允许调用方按输入内容发起远程搜索并把结果合并进 options。
export function MultiSelect({
  options,
  selected,
  onChange,
  placeholder,
  emptyText,
  maxVisibleChips,
  renderSelectedSummary,
  className,
  allowCreate = false,
  createLabel,
  id,
  disabled = false,
  copyChipOnClick = false,
  isLoading = false,
  loadingText,
  searchValue,
  onSearchChange,
  contentFooter,
}: MultiSelectProps) {
  const { t } = useTranslation()
  const resolvedPlaceholder = placeholder ?? t('Select items...')
  const resolvedEmptyText = emptyText ?? t('No matching items')
  const resolvedLoadingText = loadingText ?? t('Searching...')
  const chipsAnchorRef = useComboboxAnchor()
  const [internalInputValue, setInternalInputValue] = React.useState('')
  const [open, setOpen] = React.useState(false)
  const [expanded, setExpanded] = React.useState(false)
  const inputValue = searchValue ?? internalInputValue

  const labelMap = React.useMemo(() => {
    const map = new Map<string, string>()
    for (const option of options) {
      map.set(option.value, option.label)
    }
    return map
  }, [options])

  const trimmedInput = inputValue.trim()

  // 搜索候选只是帮助定位已有选项，不能阻止用户添加完整自定义值。
  // 只有输入值与已选项或候选项精确重复时，才隐藏“创建自定义值”入口。
  const canCreate = canCreateMultiSelectValue({
    allowCreate,
    inputValue,
    selected,
    options,
    isLoading,
  })

  const items = React.useMemo(() => {
    const set = new Set<string>(options.map((option) => option.value))
    for (const value of selected) {
      set.add(value)
    }
    if (canCreate) {
      set.add(trimmedInput)
    }
    return Array.from(set)
  }, [canCreate, options, selected, trimmedInput])

  // Base UI 的 Combobox Collection 不会替代业务侧过滤；渠道模型列表可能包含数百个
  // 静态模型，必须在这里按输入值收敛候选，才能让远程同步模型命中稳定浮到前面。
  const visibleItems = React.useMemo(
    () => filterMultiSelectItems(items, inputValue, labelMap),
    [inputValue, items, labelMap]
  )

  const updateInputValue = React.useCallback(
    (value: string) => {
      if (searchValue === undefined) {
        setInternalInputValue(value)
      }
      onSearchChange?.(value)
    },
    [onSearchChange, searchValue]
  )

  const addValues = React.useCallback(
    (values: string[]) => {
      const next: string[] = []
      const seen = new Set<string>(selected)

      for (const raw of values) {
        const value = raw.trim()
        if (!value || seen.has(value)) continue
        seen.add(value)
        next.push(value)
      }

      if (next.length === 0) return
      onChange([...selected, ...next])
    },
    [onChange, selected]
  )

  const handleInputValueChange = (value: string) => {
    if (!allowCreate) {
      updateInputValue(value)
      return
    }

    const parsed = splitDraft(value)
    if (parsed.completed.length > 0) {
      addValues(parsed.completed)
      updateInputValue(parsed.draft)
      return
    }

    updateInputValue(value)
  }

  const handleValueChange = (next: string[]) => {
    onChange(next)
    // 选中候选后清空搜索词，方便连续选择多个模型。
    if (next.length > selected.length) {
      updateInputValue('')
    }
  }

  const handleCopyChip = React.useCallback(
    async (
      event: React.MouseEvent<HTMLButtonElement>,
      value: string,
      label: string
    ) => {
      event.preventDefault()
      event.stopPropagation()

      const ok = await copyToClipboard(value)
      if (ok) {
        toast.success(t('Copied: {{model}}', { model: label }))
      } else {
        toast.error(t('Failed to copy'))
      }
    },
    [t]
  )

  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== 'Enter' || !allowCreate || !canCreate) return

    const popup = document.querySelector<HTMLElement>(
      '[data-slot="combobox-content"][data-open]'
    )
    const hasHighlight = popup?.querySelector('[data-highlighted]') != null
    if (hasHighlight) return

    event.preventDefault()
    addValues([trimmedInput])
    updateInputValue('')
  }

  return (
    <Combobox
      multiple
      items={visibleItems}
      value={selected}
      onValueChange={handleValueChange}
      inputValue={inputValue}
      onInputValueChange={handleInputValueChange}
      open={open}
      onOpenChange={setOpen}
      disabled={disabled}
    >
      <ComboboxChips ref={chipsAnchorRef} className={cn('w-full', className)}>
        <ComboboxValue>
          {(values: string[]) => {
            const shouldLimit = typeof maxVisibleChips === 'number' && !expanded
            const visibleValues = shouldLimit
              ? values.slice(0, maxVisibleChips)
              : values
            const hiddenCount = values.length - visibleValues.length

            if (hiddenCount > 0 && renderSelectedSummary) {
              return (
                <span className='bg-muted text-muted-foreground flex h-[calc(--spacing(5.25))] w-fit items-center justify-center rounded-sm px-1.5 font-mono text-xs font-medium whitespace-nowrap'>
                  {renderSelectedSummary(values)}
                </span>
              )
            }

            return (
              <>
                {visibleValues.map((value) => {
                  const label = labelMap.get(value) ?? value
                  return (
                    <ComboboxChip key={value}>
                      {copyChipOnClick ? (
                        <button
                          type='button'
                          onClick={(event) =>
                            handleCopyChip(event, value, label)
                          }
                          onPointerDown={(event) => event.stopPropagation()}
                          title={t('Click to copy')}
                          className='max-w-[16rem] cursor-pointer truncate rounded-sm hover:underline'
                        >
                          {label}
                        </button>
                      ) : (
                        <span className='max-w-[16rem] truncate'>{label}</span>
                      )}
                    </ComboboxChip>
                  )
                })}
                {hiddenCount > 0 && (
                  <button
                    type='button'
                    onClick={(event) => {
                      event.preventDefault()
                      event.stopPropagation()
                      setExpanded(true)
                    }}
                    onPointerDown={(event) => event.stopPropagation()}
                    title={t('Show All')}
                    className='bg-muted text-muted-foreground hover:bg-muted/80 hover:text-foreground flex h-[calc(--spacing(5.25))] w-fit cursor-pointer items-center justify-center rounded-sm px-1.5 text-xs font-medium whitespace-nowrap transition-colors'
                  >
                    {t('+{{count}} more', { count: hiddenCount })}
                  </button>
                )}
                {expanded &&
                  typeof maxVisibleChips === 'number' &&
                  values.length > maxVisibleChips && (
                    <button
                      type='button'
                      onClick={(event) => {
                        event.preventDefault()
                        event.stopPropagation()
                        setExpanded(false)
                      }}
                      onPointerDown={(event) => event.stopPropagation()}
                      title={t('Collapse')}
                      className='bg-muted text-muted-foreground hover:bg-muted/80 hover:text-foreground flex h-[calc(--spacing(5.25))] w-fit cursor-pointer items-center justify-center rounded-sm px-1.5 text-xs font-medium whitespace-nowrap transition-colors'
                    >
                      {t('Collapse')}
                    </button>
                  )}
              </>
            )
          }}
        </ComboboxValue>
        <ComboboxChipsInput
          id={id}
          placeholder={selected.length === 0 ? resolvedPlaceholder : undefined}
          onKeyDown={handleKeyDown}
          aria-label={resolvedPlaceholder}
        />
      </ComboboxChips>

      <ComboboxContent anchor={chipsAnchorRef}>
        <ComboboxList>
          <ComboboxCollection>
            {(item: string) => {
              const isCreate = canCreate && item === trimmedInput
              const label = labelMap.get(item) ?? item
              return (
                <ComboboxItem
                  key={item}
                  value={item}
                  className={isCreate ? 'text-foreground' : undefined}
                >
                  {isCreate ? (
                    <>
                      <HugeiconsIcon
                        icon={Add01Icon}
                        strokeWidth={2}
                        className='text-muted-foreground'
                        aria-hidden='true'
                      />
                      <span className='truncate'>
                        {createLabel
                          ? t(createLabel, { value: item })
                          : t('Add "{{value}}"', { value: item })}
                      </span>
                    </>
                  ) : (
                    <span className='truncate'>{label}</span>
                  )}
                </ComboboxItem>
              )
            }}
          </ComboboxCollection>
        </ComboboxList>
        <ComboboxEmpty>
          {isLoading ? resolvedLoadingText : resolvedEmptyText}
        </ComboboxEmpty>
        {contentFooter && (
          <>
            <ComboboxSeparator />
            <div
              className='p-2'
              onPointerDown={(event) => event.stopPropagation()}
            >
              {contentFooter}
            </div>
          </>
        )}
      </ComboboxContent>
    </Combobox>
  )
}
