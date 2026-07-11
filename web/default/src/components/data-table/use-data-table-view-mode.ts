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

export const DATA_TABLE_VIEW_MODES = {
  TABLE: 'table',
  CARD: 'card',
} as const

export type DataTableViewMode =
  (typeof DATA_TABLE_VIEW_MODES)[keyof typeof DATA_TABLE_VIEW_MODES]

type ViewModeStorage = Pick<Storage, 'getItem' | 'setItem'>

type UseDataTableViewModeOptions = {
  storageKey?: string
  defaultMode?: DataTableViewMode
}

export function isDataTableViewMode(
  value: unknown
): value is DataTableViewMode {
  return (
    value === DATA_TABLE_VIEW_MODES.TABLE ||
    value === DATA_TABLE_VIEW_MODES.CARD
  )
}

export function readDataTableViewMode(
  storageKey: string | undefined,
  fallback: DataTableViewMode,
  storage: ViewModeStorage | undefined = typeof window === 'undefined'
    ? undefined
    : window.localStorage
): DataTableViewMode {
  if (!storageKey || !storage) return fallback

  try {
    const raw = storage.getItem(storageKey)
    return isDataTableViewMode(raw) ? raw : fallback
  } catch {
    return fallback
  }
}

export function writeDataTableViewMode(
  storageKey: string | undefined,
  mode: DataTableViewMode,
  storage: ViewModeStorage | undefined = typeof window === 'undefined'
    ? undefined
    : window.localStorage
) {
  if (!storageKey || !storage) return

  try {
    storage.setItem(storageKey, mode)
  } catch {
    // localStorage 在隐私模式或受限 iframe 中可能不可写，视图切换仍应在内存中生效。
  }
}

export function useDataTableViewMode(
  options: UseDataTableViewModeOptions = {}
): [DataTableViewMode, (mode: DataTableViewMode) => void] {
  const defaultMode = options.defaultMode ?? DATA_TABLE_VIEW_MODES.TABLE
  const storageKey = options.storageKey

  const [viewMode, setViewModeState] = React.useState<DataTableViewMode>(() =>
    readDataTableViewMode(storageKey, defaultMode)
  )
  const hydratedStorageKeyRef = React.useRef(storageKey)

  React.useEffect(() => {
    if (storageKey === hydratedStorageKeyRef.current) return
    hydratedStorageKeyRef.current = storageKey
    setViewModeState(readDataTableViewMode(storageKey, defaultMode))
  }, [defaultMode, storageKey])

  const setViewMode = React.useCallback(
    (mode: DataTableViewMode) => {
      setViewModeState(mode)
      writeDataTableViewMode(storageKey, mode)
    },
    [storageKey]
  )

  return [viewMode, setViewMode]
}
