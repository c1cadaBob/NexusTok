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
  open?: boolean
  onOpenChange?: (open: boolean) => void
  contentHeader?: React.ReactNode
  contentFooter?: React.ReactNode
  allowCreateWithMatches?: boolean
  preserveSelectedOnEmptyRemovalKey?: boolean
  hideSelectedOptionsWhenSearching?: boolean
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
  allowCreateWithMatches = true,
  hasMatchingOption = false,
}: {
  allowCreate: boolean
  inputValue: string
  selected: readonly string[]
  options: readonly Option[]
  isLoading?: boolean
  allowCreateWithMatches?: boolean
  hasMatchingOption?: boolean
}): boolean {
  const trimmedInput = inputValue.trim()
  if (!allowCreate || isLoading || trimmedInput.length === 0) return false

  const normalizedInput = trimmedInput.toLowerCase()
  const selectedSet = new Set(
    selected.map((value) => value.trim().toLowerCase())
  )
  const hasExactDuplicate =
    selectedSet.has(normalizedInput) ||
    options.some(
      (option) =>
        option.value.toLowerCase() === normalizedInput ||
        option.label.toLowerCase() === normalizedInput
    )

  if (hasExactDuplicate) return false

  // 渠道模型搜索会输入系列前缀，例如 gpt-5.6。存在真实候选时，
  // 该输入应被解释为筛选/补齐意图，而不是创建一个不完整自定义模型。
  if (!allowCreateWithMatches && hasMatchingOption) return false

  return true
}

export function shouldPreventEmptyInputChipRemoval({
  preserveSelectedOnEmptyRemovalKey,
  inputValue,
  key,
  selectedLength,
}: {
  preserveSelectedOnEmptyRemovalKey: boolean
  inputValue: string
  key: string
  selectedLength: number
}): boolean {
  if (!preserveSelectedOnEmptyRemovalKey || selectedLength === 0) return false
  if (key !== 'Backspace' && key !== 'Delete') return false

  // 渠道模型字段把输入框用作搜索过滤器。空搜索时 Backspace/Delete 继续传给
  // Base UI 会删除已有模型，容易让“搜索后追加”误变成“替换整个模型列表”。
  return inputValue.trim().length === 0
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
  open: controlledOpen,
  onOpenChange,
  contentHeader,
  contentFooter,
  allowCreateWithMatches = true,
  preserveSelectedOnEmptyRemovalKey = false,
  hideSelectedOptionsWhenSearching = false,
}: MultiSelectProps) {
  const { t } = useTranslation()
  const resolvedPlaceholder = placeholder ?? t('Select items...')
  const resolvedEmptyText = emptyText ?? t('No matching items')
  const resolvedLoadingText = loadingText ?? t('Searching...')
  const chipsAnchorRef = useComboboxAnchor()
  const [internalInputValue, setInternalInputValue] = React.useState('')
  const [internalOpen, setInternalOpen] = React.useState(false)
  const [expanded, setExpanded] = React.useState(false)
  const inputValue = searchValue ?? internalInputValue
  const open = controlledOpen ?? internalOpen

  const updateOpen = React.useCallback(
    (nextOpen: boolean) => {
      if (controlledOpen === undefined) {
        setInternalOpen(nextOpen)
      }
      onOpenChange?.(nextOpen)
    },
    [controlledOpen, onOpenChange]
  )

  const labelMap = React.useMemo(() => {
    const map = new Map<string, string>()
    for (const option of options) {
      map.set(option.value, option.label)
    }
    return map
  }, [options])

  const trimmedInput = inputValue.trim()
  const selectedKeys = React.useMemo(
    () => new Set(selected.map((value) => value.trim().toLowerCase())),
    [selected]
  )

  const baseItems = React.useMemo(() => {
    const set = new Set<string>(options.map((option) => option.value))
    for (const value of selected) {
      set.add(value)
    }
    return Array.from(set)
  }, [options, selected])

  const hasMatchingOption = React.useMemo(
    () =>
      trimmedInput.length > 0 &&
      filterMultiSelectItems(baseItems, inputValue, labelMap).length > 0,
    [baseItems, inputValue, labelMap, trimmedInput]
  )

  // 默认只在精确重复时隐藏“创建自定义值”入口；渠道模型搜索等场景可通过
  // allowCreateWithMatches=false 将“有候选”解释为筛选/补齐意图。
  const canCreate = canCreateMultiSelectValue({
    allowCreate,
    inputValue,
    selected,
    options,
    isLoading,
    allowCreateWithMatches,
    hasMatchingOption,
  })

  const items = React.useMemo(() => {
    const set = new Set(baseItems)
    if (canCreate) {
      set.add(trimmedInput)
    }
    return Array.from(set)
  }, [baseItems, canCreate, trimmedInput])

  // Base UI 的 Combobox Collection 不会替代业务侧过滤；渠道模型列表可能包含数百个
  // 静态模型，必须在这里按输入值收敛候选，才能让远程同步模型命中稳定浮到前面。
  const visibleItems = React.useMemo(() => {
    const filteredItems = filterMultiSelectItems(items, inputValue, labelMap)
    if (!hideSelectedOptionsWhenSearching || trimmedInput.length === 0) {
      return filteredItems
    }

    return filteredItems.filter(
      (item) => !selectedKeys.has(item.trim().toLowerCase())
    )
  }, [
    hideSelectedOptionsWhenSearching,
    inputValue,
    items,
    labelMap,
    selectedKeys,
    trimmedInput,
  ])

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

  const handleRemovalKeyDownCapture = (
    event: React.KeyboardEvent<HTMLInputElement>
  ) => {
    if (
      shouldPreventEmptyInputChipRemoval({
        preserveSelectedOnEmptyRemovalKey,
        inputValue,
        key: event.key,
        selectedLength: selected.length,
      })
    ) {
      event.preventDefault()
      event.stopPropagation()
      return
    }
  }

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
      onOpenChange={updateOpen}
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
          onKeyDownCapture={handleRemovalKeyDownCapture}
          onKeyDown={handleKeyDown}
          aria-label={resolvedPlaceholder}
        />
      </ComboboxChips>

      <ComboboxContent anchor={chipsAnchorRef}>
        {contentHeader && (
          <>
            <div
              className='p-2'
              onPointerDown={(event) => event.stopPropagation()}
            >
              {contentHeader}
            </div>
            <ComboboxSeparator />
          </>
        )}
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
