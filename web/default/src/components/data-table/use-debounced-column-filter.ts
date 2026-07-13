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
import {
  type ChangeEvent,
  type CompositionEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react'
import type { ColumnFiltersState, OnChangeFn } from '@tanstack/react-table'
import { useDebounce } from '@/hooks/use-debounce'

type UseDebouncedColumnFilterOptions = {
  columnFilters: ColumnFiltersState
  columnId: string
  onColumnFiltersChange: OnChangeFn<ColumnFiltersState>
  delay?: number
}

// 将表格列过滤输入拆成“本地显示值”和“待提交过滤值”：
// 组合输入法输入过程中只更新本地显示值，等 compositionend 后再把完整文本写入列过滤。
export function useDebouncedColumnFilter({
  columnFilters,
  columnId,
  onColumnFiltersChange,
  delay = 500,
}: UseDebouncedColumnFilterOptions) {
  const value =
    (columnFilters.find((filter) => filter.id === columnId)?.value as
      | string
      | undefined) ?? ''
  const [inputValue, setInputValue] = useState(value)
  const [pendingValue, setPendingValue] = useState(value)
  const isComposingRef = useRef(false)
  const debouncedValue = useDebounce(pendingValue, delay)
  const onColumnFiltersChangeRef = useRef(onColumnFiltersChange)
  onColumnFiltersChangeRef.current = onColumnFiltersChange

  useEffect(() => {
    if (!isComposingRef.current) {
      setInputValue(value)
    }
    setPendingValue(value)
  }, [value])

  useEffect(() => {
    if (debouncedValue === value) return

    onColumnFiltersChangeRef.current((previous) => {
      const filters = previous.filter((filter) => filter.id !== columnId)
      return debouncedValue
        ? [...filters, { id: columnId, value: debouncedValue }]
        : filters
    })
  }, [columnId, debouncedValue, value])

  const updateInputValue = useCallback((nextValue: string) => {
    setInputValue(nextValue)

    if (!isComposingRef.current) {
      setPendingValue(nextValue)
    }
  }, [])

  const handleChange = useCallback(
    (event: ChangeEvent<HTMLInputElement>) => {
      updateInputValue(event.target.value)
    },
    [updateInputValue]
  )

  const handleCompositionStart = useCallback(() => {
    isComposingRef.current = true
  }, [])

  const handleCompositionEnd = useCallback(
    (event: CompositionEvent<HTMLInputElement>) => {
      isComposingRef.current = false
      const nextValue = event.currentTarget.value
      setInputValue(nextValue)
      setPendingValue(nextValue)
    },
    []
  )

  const resetInput = useCallback(() => {
    isComposingRef.current = false
    setInputValue('')
    setPendingValue('')
  }, [])

  return {
    value,
    inputValue,
    setInputValue: updateInputValue,
    onChange: handleChange,
    onCompositionStart: handleCompositionStart,
    onCompositionEnd: handleCompositionEnd,
    resetInput,
  }
}
