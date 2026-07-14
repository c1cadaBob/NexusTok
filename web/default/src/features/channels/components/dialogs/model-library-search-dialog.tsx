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
import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { SearchIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useDebounce } from '@/hooks'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { searchModels } from '@/features/models/api'
import {
  dedupeModelNames,
  getMissingModelSearchMatches,
  getModelSearchModelNames,
  getModelSearchUnscannedResultCount,
  mergeModelNames,
  normalizeModelSearchKey,
} from '../../lib'

const MODEL_LIBRARY_SEARCH_PAGE_SIZE = 50
const MODEL_LIBRARY_SCAN_PAGE_SIZE = 100

type ModelLibrarySearchDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  vendor: string
  currentModels: string[]
  channelName?: string | null
  canEdit: boolean
  noPermissionMessage: string
  onAddModels: (models: string[]) => void
}

async function fetchAllModelLibrarySearchNames(
  keyword: string,
  vendor: string
): Promise<string[]> {
  const trimmedKeyword = keyword.trim()
  const trimmedVendor = vendor.trim()
  const names: string[] = []
  let page = 1
  let total = 0

  while (true) {
    const response = await searchModels({
      keyword: trimmedKeyword,
      vendor: trimmedVendor || undefined,
      p: page,
      page_size: MODEL_LIBRARY_SCAN_PAGE_SIZE,
    })

    if (!response.success) {
      throw new Error(response.message || '')
    }

    const items = response.data?.items ?? []
    total = response.data?.total ?? total
    names.push(...getModelSearchModelNames(items, trimmedKeyword))

    if (
      items.length === 0 ||
      page * MODEL_LIBRARY_SCAN_PAGE_SIZE >= total
    ) {
      break
    }

    page += 1
  }

  return dedupeModelNames(names)
}

function splitExistingModelSearchMatches(
  searchMatches: readonly string[],
  currentModels: readonly string[]
) {
  const currentModelKeys = new Set(
    currentModels.map(normalizeModelSearchKey).filter(Boolean)
  )
  return dedupeModelNames(searchMatches).filter((model) =>
    currentModelKeys.has(normalizeModelSearchKey(model))
  )
}

function ModelCheckboxList({
  models,
  selectedModels,
  onToggle,
  emptyText,
  disabled = false,
}: {
  models: string[]
  selectedModels: string[]
  onToggle: (model: string) => void
  emptyText: string
  disabled?: boolean
}) {
  if (models.length === 0) {
    return (
      <ModelLibrarySearchEmpty title={emptyText} className='min-h-28 border' />
    )
  }

  return (
    <div className='grid max-h-80 gap-2 overflow-y-auto pr-1 sm:grid-cols-2'>
      {models.map((model, index) => {
        const id = `model-library-search-${disabled ? 'existing' : 'new'}-${index}`
        const labelId = `${id}-label`
        const checked = selectedModels.includes(model)
        return (
          <div
            key={model}
            className='hover:bg-muted/50 flex min-w-0 items-center gap-2 rounded-md border px-3 py-2'
          >
            <Checkbox
              id={id}
              checked={checked}
              disabled={disabled}
              aria-labelledby={labelId}
              onCheckedChange={() => onToggle(model)}
            />
            <span
              id={labelId}
              className='min-w-0 flex-1 cursor-pointer truncate text-sm data-disabled:cursor-not-allowed data-disabled:opacity-60'
              data-disabled={disabled ? '' : undefined}
              onClick={disabled ? undefined : () => onToggle(model)}
              title={model}
            >
              {model}
            </span>
          </div>
        )
      })}
    </div>
  )
}

function ModelLibrarySearchEmpty({
  title,
  description,
  loading = false,
  className,
}: {
  title: string
  description?: string
  loading?: boolean
  className?: string
}) {
  return (
    <Empty className={className}>
      <EmptyHeader>
        {loading && (
          <EmptyMedia variant='icon'>
            <Spinner />
          </EmptyMedia>
        )}
        <EmptyTitle>{title}</EmptyTitle>
        {description != null && (
          <EmptyDescription>{description}</EmptyDescription>
        )}
      </EmptyHeader>
    </Empty>
  )
}

export function ModelLibrarySearchDialog({
  open,
  onOpenChange,
  vendor,
  currentModels,
  channelName,
  canEdit,
  noPermissionMessage,
  onAddModels,
}: ModelLibrarySearchDialogProps) {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const [selectedModels, setSelectedModels] = useState<string[]>([])
  const [scannedModelNames, setScannedModelNames] = useState<string[] | null>(
    null
  )
  const [isScanningAll, setIsScanningAll] = useState(false)
  const trimmedKeyword = keyword.trim()
  const debouncedKeyword = useDebounce(trimmedKeyword, 300)
  const isWaitingForDebounce =
    trimmedKeyword.length > 0 && trimmedKeyword !== debouncedKeyword
  const vendorLabel = vendor.trim() || t('All Vendors')

  const { data, isFetching } = useQuery({
    queryKey: ['channel-model-library-search', debouncedKeyword, vendor],
    queryFn: () =>
      searchModels({
        keyword: debouncedKeyword,
        vendor: vendor.trim() || undefined,
        p: 1,
        page_size: MODEL_LIBRARY_SEARCH_PAGE_SIZE,
      }),
    enabled: open && debouncedKeyword.length > 0,
    staleTime: 30_000,
  })

  useEffect(() => {
    if (!open) {
      setKeyword('')
      setSelectedModels([])
      setScannedModelNames(null)
      setIsScanningAll(false)
    }
  }, [open])

  useEffect(() => {
    setScannedModelNames(null)
  }, [debouncedKeyword, vendor])

  const firstPageModelNames = useMemo(() => {
    if (!debouncedKeyword || !data?.success) return []
    return getModelSearchModelNames(data.data?.items ?? [], debouncedKeyword)
  }, [data, debouncedKeyword])

  const searchModelNames = scannedModelNames ?? firstPageModelNames
  const newModels = useMemo(
    () => getMissingModelSearchMatches(searchModelNames, currentModels),
    [currentModels, searchModelNames]
  )
  const existingModels = useMemo(
    () => splitExistingModelSearchMatches(searchModelNames, currentModels),
    [currentModels, searchModelNames]
  )
  const unscannedResultCount = getModelSearchUnscannedResultCount({
    isResultCurrent: Boolean(debouncedKeyword) && scannedModelNames == null,
    loadedResultCount: data?.data?.items?.length ?? 0,
    backendResultTotal: data?.data?.total ?? 0,
  })

  useEffect(() => {
    setSelectedModels(newModels)
  }, [newModels])

  const toggleModel = (model: string) => {
    setSelectedModels((previous) =>
      previous.includes(model)
        ? previous.filter((item) => item !== model)
        : mergeModelNames(previous, [model])
    )
  }

  const handleScanAll = async () => {
    if (!debouncedKeyword) return
    setIsScanningAll(true)
    try {
      const allNames = await fetchAllModelLibrarySearchNames(
        debouncedKeyword,
        vendor
      )
      setScannedModelNames(allNames)
      setSelectedModels(getMissingModelSearchMatches(allNames, currentModels))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Refresh failed'))
    } finally {
      setIsScanningAll(false)
    }
  }

  const handleAddSelected = () => {
    if (!canEdit) {
      toast.error(noPermissionMessage)
      return
    }

    const modelsToAdd = getMissingModelSearchMatches(
      selectedModels,
      currentModels
    )
    if (modelsToAdd.length === 0) {
      toast.info(t('No models selected'))
      return
    }

    onAddModels(modelsToAdd)
    toast.success(
      t('Added {{count}} model(s) from search', { count: modelsToAdd.length })
    )
    onOpenChange(false)
  }

  const handleOpenChange = (nextOpen: boolean) => {
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className='max-w-3xl'>
        <DialogHeader>
          <div className='flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between'>
            <div className='flex flex-col gap-2'>
              <DialogTitle>{t('Search model library')}</DialogTitle>
              <DialogDescription>
                {t(
                  'Search model metadata and add selected matches to this channel draft.'
                )}
              </DialogDescription>
            </div>
            <div className='flex flex-wrap gap-2'>
              {channelName && (
                <Badge variant='outline' className='w-fit'>
                  {channelName}
                </Badge>
              )}
              <Badge variant='secondary' className='w-fit'>
                {t('Vendor')}: {vendorLabel}
              </Badge>
            </div>
          </div>
        </DialogHeader>

        <div className='flex flex-col gap-4'>
          <InputGroup>
            <InputGroupAddon>
              <HugeiconsIcon
                icon={SearchIcon}
                strokeWidth={2}
                className='shrink-0 opacity-50'
              />
            </InputGroupAddon>
            <InputGroupInput
              value={keyword}
              onChange={(event) => setKeyword(event.target.value)}
              placeholder={t('Search models...')}
              aria-label={t('Search models...')}
              autoFocus
            />
          </InputGroup>

          {trimmedKeyword.length === 0 ? (
            <ModelLibrarySearchEmpty
              title={t('Search models...')}
              description={t(
                'Search model metadata and add selected matches to this channel draft.'
              )}
              className='min-h-36 border'
            />
          ) : (isWaitingForDebounce || isFetching) &&
            firstPageModelNames.length === 0 ? (
            <ModelLibrarySearchEmpty
              title={t('Searching model metadata...')}
              loading
              className='min-h-36 border'
            />
          ) : !data?.success ? (
            <ModelLibrarySearchEmpty
              title={data?.message || t('No models found')}
              className='min-h-36 border'
            />
          ) : (
            <>
              <div className='flex flex-wrap items-center gap-2 text-xs'>
                <Badge variant='secondary'>
                  {t('{{matched}} matched · {{addable}} new · {{existing}} already selected', {
                    matched: newModels.length + existingModels.length,
                    addable: newModels.length,
                    existing: existingModels.length,
                  })}
                </Badge>
                {unscannedResultCount > 0 && (
                  <span className='text-muted-foreground'>
                    {t(
                      '{{count}} more result(s) will be checked before adding all matches',
                      { count: unscannedResultCount }
                    )}
                  </span>
                )}
              </div>

              <Tabs
                key={`${debouncedKeyword}-${scannedModelNames ? 'all' : 'page'}`}
                defaultValue={newModels.length > 0 ? 'new' : 'existing'}
              >
                <TabsList className='grid w-full grid-cols-2'>
                  <TabsTrigger value='new'>
                    {t('New Models ({{count}})', { count: newModels.length })}
                  </TabsTrigger>
                  <TabsTrigger value='existing'>
                    {t('Existing Models ({{count}})', {
                      count: existingModels.length,
                    })}
                  </TabsTrigger>
                </TabsList>
                <TabsContent value='new'>
                  <ModelCheckboxList
                    models={newModels}
                    selectedModels={selectedModels}
                    onToggle={toggleModel}
                    emptyText={t('No new search results to add')}
                  />
                </TabsContent>
                <TabsContent value='existing'>
                  <ModelCheckboxList
                    models={existingModels}
                    selectedModels={existingModels}
                    onToggle={() => undefined}
                    emptyText={t('No models found')}
                    disabled
                  />
                </TabsContent>
              </Tabs>
            </>
          )}
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button
            variant='outline'
            onClick={handleScanAll}
            disabled={
              !canEdit ||
              isScanningAll ||
              isFetching ||
              unscannedResultCount === 0
            }
            title={canEdit ? undefined : noPermissionMessage}
          >
            {isScanningAll && <Spinner data-icon='inline-start' />}
            {t('Scan all search results')}
          </Button>
          <Button
            onClick={handleAddSelected}
            disabled={!canEdit || selectedModels.length === 0}
            title={canEdit ? undefined : noPermissionMessage}
          >
            {t('Add selected models')}
            {selectedModels.length > 0 ? ` (${selectedModels.length})` : ''}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
