/*
Copyright (C) 2023-2026 c1cada

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@c1cada.dev
*/
import { useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Delete02Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { MultiSelect, type MultiSelectProps } from '@/components/multi-select'
import {
  dedupeModelCatalogNames,
  fetchAllModelCatalogNames,
  filterModelCatalogNames,
} from '../lib/model-catalog'

type ModelCatalogMultiSelectProps = Omit<
  MultiSelectProps,
  | 'options'
  | 'isLoading'
  | 'loadingText'
  | 'emptyText'
  | 'selected'
  | 'onChange'
  | 'open'
  | 'onOpenChange'
  | 'openOnFocus'
  | 'excludeSelectedOptions'
  | 'filterItems'
  | 'contentFooter'
> & {
  selected: string[]
  onChange: (values: string[]) => void
  extraModels?: readonly string[]
  enabled?: boolean
  contentFooter?: ReactNode
}

const MODEL_CATALOG_QUERY_KEY = ['model-catalog', 'all'] as const

export function ModelCatalogMultiSelect({
  selected,
  onChange,
  extraModels = [],
  enabled = true,
  contentFooter,
  ...props
}: ModelCatalogMultiSelectProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const catalogQuery = useQuery({
    queryKey: MODEL_CATALOG_QUERY_KEY,
    queryFn: fetchAllModelCatalogNames,
    enabled: enabled && open && props.disabled !== true,
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
  })

  const options = useMemo(
    () =>
      dedupeModelCatalogNames([
        ...(catalogQuery.data ?? []),
        ...extraModels,
        ...selected,
      ]).map((model) => ({
        value: model,
        label: model,
      })),
    [catalogQuery.data, extraModels, selected]
  )

  const clearFooter = (
    <div className='flex flex-wrap items-center justify-between gap-2'>
      <Button
        type='button'
        variant='ghost'
        size='sm'
        disabled={selected.length === 0}
        onPointerDown={(event) => event.preventDefault()}
        onClick={() => onChange([])}
      >
        <HugeiconsIcon
          icon={Delete02Icon}
          strokeWidth={2}
          data-icon='inline-start'
          aria-hidden='true'
        />
        {t('Clear All')}
      </Button>
      {contentFooter}
    </div>
  )

  return (
    <MultiSelect
      {...props}
      options={options}
      selected={selected}
      onChange={onChange}
      open={open}
      onOpenChange={setOpen}
      openOnFocus
      excludeSelectedOptions
      filterItems={(items, inputValue) =>
        filterModelCatalogNames(items, inputValue)
      }
      isLoading={catalogQuery.isFetching}
      loadingText={t('Loading model catalog...')}
      emptyText={
        catalogQuery.isError
          ? t('Failed to load model catalog')
          : t('No matching models')
      }
      allowCreate
      allowCreateDuringSearchLoading
      allowCreateWithMatches
      contentFooter={clearFooter}
    />
  )
}
