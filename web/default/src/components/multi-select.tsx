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
  name?: string
  disabled?: boolean
  copyChipOnClick?: boolean
  isLoading?: boolean
  loadingText?: string
  searchValue?: string
  onSearchChange?: (value: string) => void
  open?: boolean
  onOpenChange?: (open: boolean) => void
  onSearchSubmit?: () => void
  contentHeader?: React.ReactNode
  contentFooter?: React.ReactNode
  allowCreateWithMatches?: boolean
  allowCreateDuringSearchLoading?: boolean
  compactInput?: boolean
  preserveSelectedOnEmptyRemovalKey?: boolean
  hideSelectedOptionsWhenSearching?: boolean
  submitSearchOnEnterWithMatches?: boolean
  submitSearchOnEnterWhenHighlighted?: boolean
  clearSearchOnSelect?: boolean
  'aria-labelledby'?: string
  'aria-describedby'?: string
  'aria-invalid'?: boolean
}

const COMMA_REGEX = /[,，\n]/

export function normalizeMultiSelectValueKey(value: string): string {
  return value.trim().toLowerCase()
}

// 多选值在交互层统一按 trim + lower 判断重复，并保留首次出现的展示形式。
// 渠道模型搜索、逗号批量输入和自定义创建都会经过这里，避免大小写差异产生重复 chip。
export function dedupeMultiSelectValues(values: readonly string[]): string[] {
  const seenKeys = new Set<string>()
  const dedupedValues: string[] = []

  for (const rawValue of values) {
    const value = rawValue.trim()
    const key = normalizeMultiSelectValueKey(value)
    if (!key || seenKeys.has(key)) continue
    seenKeys.add(key)
    dedupedValues.push(value)
  }

  return dedupedValues
}

export function getNewMultiSelectValues({
  selected,
  incoming,
}: {
  selected: readonly string[]
  incoming: readonly string[]
}): string[] {
  const seenKeys = new Set(selected.map(normalizeMultiSelectValueKey))
  const nextValues: string[] = []

  for (const rawValue of incoming) {
    const value = rawValue.trim()
    const key = normalizeMultiSelectValueKey(value)
    if (!key || seenKeys.has(key)) continue
    seenKeys.add(key)
    nextValues.push(value)
  }

  return nextValues
}

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

export function getVisibleMultiSelectItems({
  items,
  inputValue,
  labelMap = new Map(),
  hideSelectedOptionsWhenSearching,
  selected,
}: {
  items: string[]
  inputValue: string
  labelMap?: ReadonlyMap<string, string>
  hideSelectedOptionsWhenSearching: boolean
  selected: readonly string[]
}): string[] {
  const filteredItems = filterMultiSelectItems(items, inputValue, labelMap)
  const trimmedInput = inputValue.trim()
  if (!hideSelectedOptionsWhenSearching || trimmedInput.length === 0) {
    return filteredItems
  }

  const selectedKeys = new Set(
    selected.map((value) => value.trim().toLowerCase())
  )
  return filteredItems.filter(
    (item) => !selectedKeys.has(item.trim().toLowerCase())
  )
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

export function shouldSubmitMultiSelectSearchOnEnter({
  submitSearchOnEnterWithMatches,
  hasSearchSubmit,
  key,
  inputValue,
  isLoading,
  hasHighlightedOption = false,
  submitSearchOnEnterWhenHighlighted = false,
  canCreateValue = false,
}: {
  submitSearchOnEnterWithMatches: boolean
  hasSearchSubmit: boolean
  key: string
  inputValue: string
  isLoading: boolean
  hasHighlightedOption?: boolean
  submitSearchOnEnterWhenHighlighted?: boolean
  canCreateValue?: boolean
}): boolean {
  if (!submitSearchOnEnterWithMatches || !hasSearchSubmit) return false
  if (key !== 'Enter') return false
  if (inputValue.trim().length === 0) return false
  // 只要当前输入仍然可以创建自定义值，就让后续 keydown 处理器完成创建。
  // 这样搜索结果存在但没有高亮候选时，Enter 不会被误解为“批量追加搜索结果”。
  if (canCreateValue) return false
  if (hasHighlightedOption && !submitSearchOnEnterWhenHighlighted) return false

  // 没有可创建值时，只有远程搜索仍在进行才交给调用方处理。
  // 已返回候选时由 Base UI 的高亮选择负责消费 Enter，批量追加通过独立按钮完成。
  return isLoading
}

export function shouldPreventMultiSelectEnterFormSubmit({
  key,
  inputValue,
}: {
  key: string
  inputValue: string
}): boolean {
  return key === 'Enter' && inputValue.trim().length > 0
}

export function shouldClearMultiSelectSearchAfterChange({
  clearSearchOnSelect,
  previousSelectedLength,
  nextSelectedLength,
}: {
  clearSearchOnSelect: boolean
  previousSelectedLength: number
  nextSelectedLength: number
}): boolean {
  return clearSearchOnSelect && nextSelectedLength > previousSelectedLength
}

export function shouldRestoreMultiSelectSearchAfterSelection({
  clearSearchOnSelect,
  inputValue,
  previousSelectedLength,
  nextSelectedLength,
}: {
  clearSearchOnSelect: boolean
  inputValue: string
  previousSelectedLength: number
  nextSelectedLength: number
}): boolean {
  return (
    !clearSearchOnSelect &&
    inputValue.trim().length > 0 &&
    nextSelectedLength > previousSelectedLength
  )
}

function hasHighlightedComboboxOption(): boolean {
  if (typeof document === 'undefined') return false
  const popup = document.querySelector<HTMLElement>(
    '[data-slot="combobox-content"][data-open]'
  )
  return popup?.querySelector('[data-highlighted]') != null
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
  name,
  disabled = false,
  copyChipOnClick = false,
  isLoading = false,
  loadingText,
  searchValue,
  onSearchChange,
  open: controlledOpen,
  onOpenChange,
  onSearchSubmit,
  contentHeader,
  contentFooter,
  allowCreateWithMatches = true,
  allowCreateDuringSearchLoading = false,
  compactInput = false,
  preserveSelectedOnEmptyRemovalKey = false,
  hideSelectedOptionsWhenSearching = false,
  submitSearchOnEnterWithMatches = false,
  submitSearchOnEnterWhenHighlighted = false,
  clearSearchOnSelect = true,
  'aria-labelledby': ariaLabelledBy,
  'aria-describedby': ariaDescribedBy,
  'aria-invalid': ariaInvalid,
}: MultiSelectProps) {
  const { t } = useTranslation()
  const resolvedPlaceholder = placeholder ?? t('Select items...')
  const resolvedEmptyText = emptyText ?? t('No matching items')
  const resolvedLoadingText = loadingText ?? t('Searching...')
  const chipsAnchorRef = useComboboxAnchor()
  const [internalInputValue, setInternalInputValue] = React.useState('')
  const [internalOpen, setInternalOpen] = React.useState(false)
  const [expanded, setExpanded] = React.useState(false)
  const restoreSearchAfterSelectRef = React.useRef<string | null>(null)
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
    isLoading: isLoading && !allowCreateDuringSearchLoading,
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
    return getVisibleMultiSelectItems({
      items,
      inputValue,
      labelMap,
      hideSelectedOptionsWhenSearching,
      selected,
    })
  }, [hideSelectedOptionsWhenSearching, inputValue, items, labelMap, selected])

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
      const next = getNewMultiSelectValues({ selected, incoming: values })
      if (next.length === 0) return
      onChange([...selected, ...next])
    },
    [onChange, selected]
  )

  const handleInputValueChange = (value: string) => {
    if (
      restoreSearchAfterSelectRef.current !== null &&
      value.trim().length === 0
    ) {
      const restoredInputValue = restoreSearchAfterSelectRef.current
      restoreSearchAfterSelectRef.current = null
      updateInputValue(restoredInputValue)
      updateOpen(true)
      return
    }

    restoreSearchAfterSelectRef.current = null

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
    const dedupedNext = dedupeMultiSelectValues(next)
    onChange(dedupedNext)
    const shouldRestoreSearch = shouldRestoreMultiSelectSearchAfterSelection({
      clearSearchOnSelect,
      inputValue,
      previousSelectedLength: selected.length,
      nextSelectedLength: dedupedNext.length,
    })

    // 默认延续现有多选体验：选中后清空搜索词。
    // 调用方可显式关闭该行为，用于在同一关键词下连续选择多个远程候选。
    if (
      shouldClearMultiSelectSearchAfterChange({
        clearSearchOnSelect,
        previousSelectedLength: selected.length,
        nextSelectedLength: dedupedNext.length,
      })
    ) {
      updateInputValue('')
      return
    }

    if (shouldRestoreSearch) {
      restoreSearchAfterSelectRef.current = inputValue
      updateInputValue(inputValue)
      updateOpen(true)
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

    if (
      shouldSubmitMultiSelectSearchOnEnter({
        submitSearchOnEnterWithMatches,
        hasSearchSubmit: Boolean(onSearchSubmit),
        key: event.key,
        inputValue,
        isLoading: isLoading && !allowCreateDuringSearchLoading,
        hasHighlightedOption: hasHighlightedComboboxOption(),
        submitSearchOnEnterWhenHighlighted,
        canCreateValue: allowCreate && canCreate,
      })
    ) {
      event.preventDefault()
      event.stopPropagation()
      onSearchSubmit?.()
    }
  }

  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.defaultPrevented) return
    if (event.key !== 'Enter') return

    const popup = document.querySelector<HTMLElement>(
      '[data-slot="combobox-content"][data-open]'
    )
    const hasHighlight = popup?.querySelector('[data-highlighted]') != null
    if (hasHighlight) return

    if (allowCreate && canCreate) {
      event.preventDefault()
      addValues([trimmedInput])
      updateInputValue('')
      return
    }

    if (onSearchSubmit && trimmedInput.length > 0) {
      event.preventDefault()
      onSearchSubmit()
      return
    }

    if (
      shouldPreventMultiSelectEnterFormSubmit({
        key: event.key,
        inputValue,
      })
    ) {
      // 多选搜索框位于表单中时，未被候选选择或自定义创建消费的 Enter
      // 只应停留在搜索交互内，不能冒泡成父表单提交。
      event.preventDefault()
    }
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
      <ComboboxChips
        ref={chipsAnchorRef}
        className={cn('max-w-full min-w-0', className)}
      >
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
                          className='max-w-[16rem] min-w-0 cursor-pointer truncate rounded-sm hover:underline'
                        >
                          {label}
                        </button>
                      ) : (
                        <span className='max-w-[16rem] min-w-0 truncate'>
                          {label}
                        </span>
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
          name={name}
          placeholder={selected.length === 0 ? resolvedPlaceholder : undefined}
          onKeyDownCapture={handleRemovalKeyDownCapture}
          onKeyDown={handleKeyDown}
          aria-label={resolvedPlaceholder}
          aria-labelledby={ariaLabelledBy}
          aria-describedby={ariaDescribedBy}
          aria-invalid={ariaInvalid}
          autoComplete='off'
          className={compactInput ? 'flex-[1_1_2rem]' : undefined}
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
