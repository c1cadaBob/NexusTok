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
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  type ColumnDef,
  type RowSelectionState,
  type Table as TanStackTable,
  flexRender,
  getCoreRowModel,
  getPaginationRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { Check, Copy, Info, Loader2, Settings } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
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
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { DataTablePagination } from '@/components/data-table/pagination'
import { StatusBadge } from '@/components/status-badge'
import {
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { useIsMobile } from '@/hooks/use-mobile'
import {
  CHANNEL_TEST_MODEL_PRICE_ERROR_CODE,
  formatResponseTime,
  getChannelTestFailureDisplay,
  handleTestChannel,
  isUpstreamAccountSyncChannel,
} from '../../lib'
import { getChannelAccounts } from '../../api'
import { useChannelPermissions } from '../../hooks/use-channel-permissions'
import { useChannels } from '../channels-provider'

type ChannelTestDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type ModelRow = {
  model: string
}

type TestStatus = 'idle' | 'testing' | 'success' | 'error'

type TestResult = {
  status: TestStatus
  responseTime?: number
  error?: string
  errorCode?: string
}

type FailureDetailsState = {
  model: string
  summary: string
  details: string
}

const endpointTypeOptions: Array<{ value: string; label: string }> = [
  { value: 'auto', label: 'Auto detect (default)' },
  { value: 'openai', label: 'OpenAI (/v1/chat/completions)' },
  { value: 'openai-response', label: 'OpenAI Responses (/v1/responses)' },
  {
    value: 'openai-response-compact',
    label: 'OpenAI Response Compaction (/v1/responses/compact)',
  },
  { value: 'anthropic', label: 'Anthropic (/v1/messages)' },
  {
    value: 'gemini',
    label: 'Gemini (/v1beta/models/{model}:generateContent)',
  },
  { value: 'jina-rerank', label: 'Jina Rerank (/v1/rerank)' },
  {
    value: 'image-generation',
    label: 'Image Generation (/v1/images/generations)',
  },
  { value: 'embeddings', label: 'Embeddings (/v1/embeddings)' },
]

const STREAM_INCOMPATIBLE_ENDPOINTS = new Set([
  'embeddings',
  'image-generation',
  'jina-rerank',
  'openai-response-compact',
])

export function ChannelTestDialog({
  open,
  onOpenChange,
}: ChannelTestDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const [endpointType, setEndpointType] = useState('auto')
  const [selectedAccountId, setSelectedAccountId] = useState('auto')
  const [isStreamTest, setIsStreamTest] = useState(false)
  const [searchTerm, setSearchTerm] = useState('')
  const [testResults, setTestResults] = useState<Record<string, TestResult>>({})
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
  const [testingModels, setTestingModels] = useState<Set<string>>(
    () => new Set()
  )
  const [isBatchTesting, setIsBatchTesting] = useState(false)
  const [failureDetails, setFailureDetails] =
    useState<FailureDetailsState | null>(null)
  const [pagination, setPagination] = useState({
    pageIndex: 0,
    pageSize: 10,
  })
  const permissions = useChannelPermissions()
  const noPermissionMessage = t("You don't have necessary permission")

  const resetState = useCallback(() => {
    setEndpointType('auto')
    setSelectedAccountId('auto')
    setIsStreamTest(false)
    setSearchTerm('')
    setTestResults({})
    setRowSelection({})
    setTestingModels(() => new Set())
    setIsBatchTesting(false)
    setFailureDetails(null)
    setPagination({ pageIndex: 0, pageSize: 10 })
  }, [])

  useEffect(() => {
    if (open && currentRow) {
      resetState()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, currentRow?.id, resetState])

  const streamDisabled = STREAM_INCOMPATIBLE_ENDPOINTS.has(endpointType)

  const isSyncedAccountPool =
    Boolean(currentRow?.channel_info?.credential_mode === 'account_pool') &&
    isUpstreamAccountSyncChannel(currentRow)

  const channelAccountsQuery = useQuery({
    queryKey: ['channel-test-accounts', currentRow?.id],
    queryFn: () =>
      getChannelAccounts(currentRow!.id, {
        p: 1,
        page_size: 100,
      }),
    enabled: open && isSyncedAccountPool && Boolean(currentRow?.id),
    staleTime: 30_000,
  })

  const testAccounts =
    channelAccountsQuery.data?.data?.accounts.items ?? []

  useEffect(() => {
    if (streamDisabled) {
      setIsStreamTest(false)
    }
  }, [streamDisabled])

  const modelsValue = currentRow?.models ?? ''
  const defaultTestModel = currentRow?.test_model?.trim()

  const models = useMemo(() => {
    if (!modelsValue) return []
    return modelsValue
      .split(',')
      .map((model) => model.trim())
      .filter(Boolean)
  }, [modelsValue])

  const filteredModels = useMemo(() => {
    if (!searchTerm) return models
    const keyword = searchTerm.toLowerCase()
    return models.filter((model) => model.toLowerCase().includes(keyword))
  }, [models, searchTerm])

  useEffect(() => {
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }, [searchTerm, modelsValue])

  const tableData = useMemo<ModelRow[]>(
    () => filteredModels.map((model) => ({ model })),
    [filteredModels]
  )

  const markModelTesting = useCallback((key: string, isTesting: boolean) => {
    setTestingModels((prev) => {
      const next = new Set(prev)
      if (isTesting) {
        next.add(key)
      } else {
        next.delete(key)
      }
      return next
    })
  }, [])

  const updateTestResult = useCallback((key: string, result: TestResult) => {
    setTestResults((prev) => ({
      ...prev,
      [key]: result,
    }))
  }, [])

  const testSingleModel = useCallback(
    async (model: string) => {
      if (!currentRow) return
      if (!permissions.canOperate) {
        updateTestResult(model, {
          status: 'error',
          error: noPermissionMessage,
        })
        return
      }

      markModelTesting(model, true)
      updateTestResult(model, { status: 'testing' })

      try {
        await handleTestChannel(
          currentRow.id,
          {
            testModel: model,
            endpointType: endpointType === 'auto' ? undefined : endpointType,
            stream: isStreamTest || undefined,
            accountId:
              selectedAccountId === 'auto'
                ? undefined
                : Number(selectedAccountId),
          },
          (success, responseTime, error, errorCode) => {
            updateTestResult(model, {
              status: success ? 'success' : 'error',
              responseTime,
              error,
              errorCode,
            })
          }
        )
      } catch (error: unknown) {
        updateTestResult(model, {
          status: 'error',
          error: error instanceof Error ? error.message : 'Test failed',
        })
      } finally {
        markModelTesting(model, false)
      }
    },
    [
      currentRow,
      endpointType,
      isStreamTest,
      markModelTesting,
      noPermissionMessage,
      permissions.canOperate,
      selectedAccountId,
      updateTestResult,
    ]
  )

  const handleBatchTest = useCallback(
    async (modelsToTest: string[]) => {
      if (!modelsToTest.length) return

      setIsBatchTesting(true)
      try {
        await Promise.allSettled(
          modelsToTest.map((modelName) => testSingleModel(modelName))
        )
      } finally {
        setIsBatchTesting(false)
        setRowSelection({})
      }
    },
    [testSingleModel]
  )

  const handleClose = () => {
    resetState()
    onOpenChange(false)
  }

  const isAnyTesting = testingModels.size > 0 || isBatchTesting

  const columns = useMemo<ColumnDef<ModelRow>[]>(
    () => [
      {
        id: 'select',
        header: ({ table }) => (
          <Checkbox
            checked={table.getIsAllPageRowsSelected()}
            indeterminate={table.getIsSomePageRowsSelected()}
            onCheckedChange={(value) =>
              table.toggleAllPageRowsSelected(!!value)
            }
            aria-label='Select all models'
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(value) => row.toggleSelected(!!value)}
            aria-label={`Select model ${row.original.model}`}
          />
        ),
        enableSorting: false,
        enableHiding: false,
        size: 40,
      },
      {
        accessorKey: 'model',
        header: t('Model'),
        cell: ({ row }) => {
          const model = row.original.model
          const isDefault = defaultTestModel === model

          return (
            <div className='flex min-w-0 items-center gap-2'>
              <span className='truncate font-medium' title={model}>
                {model}
              </span>
              {isDefault && (
                <StatusBadge
                  label={t('Default')}
                  variant='info'
                  size='sm'
                  copyable={false}
                />
              )}
            </div>
          )
        },
      },
      {
        id: 'status',
        header: t('Status'),
        cell: ({ row }) => {
          const model = row.original.model
          const result = testResults[model]
          return <TestStatusCell result={result} />
        },
        enableSorting: false,
        size: 112,
      },
      {
        id: 'result',
        header: t('Result'),
        cell: ({ row }) => {
          const model = row.original.model
          const result = testResults[model]
          return (
            <TestResultCell
              result={result}
              model={model}
              onOpenDetails={setFailureDetails}
            />
          )
        },
        enableSorting: false,
        size: 320,
      },
      {
        id: 'actions',
        header: t('Actions'),
        cell: ({ row }) => {
          const model = row.original.model
          const isTestingModel = testingModels.has(model)

          return (
            <Button
              variant='outline'
              size='sm'
              onClick={() => testSingleModel(model)}
              disabled={
                !permissions.canOperate || isTestingModel || isBatchTesting
              }
              title={permissions.canOperate ? undefined : noPermissionMessage}
            >
              {isTestingModel && (
                <Loader2 data-icon='inline-start' className='animate-spin' />
              )}
              {t('Test')}
            </Button>
          )
        },
        enableSorting: false,
        size: 120,
      },
    ],
    [
      defaultTestModel,
      isBatchTesting,
      noPermissionMessage,
      permissions.canOperate,
      t,
      testResults,
      testingModels,
      testSingleModel,
    ]
  )

  const table = useReactTable({
    data: tableData,
    columns,
    state: {
      rowSelection,
      pagination,
    },
    enableRowSelection: true,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    onRowSelectionChange: setRowSelection,
    onPaginationChange: setPagination,
  })

  if (!currentRow) {
    return null
  }

  return (
    <>
      <Dialog open={open} onOpenChange={handleClose}>
        <DialogContent className='max-h-[90vh] overflow-hidden sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{t('Test Channel Connection')}</DialogTitle>
          <DialogDescription>
            {t('Test connectivity for:')} <strong>{currentRow.name}</strong>
          </DialogDescription>
        </DialogHeader>

        <div className='max-h-[78vh] space-y-4 overflow-y-auto py-4 pr-1'>
          <div className='grid gap-4 md:grid-cols-2'>
            {isSyncedAccountPool ? (
              <div className='grid gap-2 md:col-span-2'>
                <Label htmlFor='upstream-account'>
                  {t('Upstream key for this test')}
                </Label>
                <Select
                  items={[
                    { value: 'auto', label: t('Automatic selection') },
                    ...testAccounts.map((account) => ({
                      value: String(account.id),
                      label: `${account.name || `#${account.id}`} (${account.key})`,
                    })),
                  ]}
                  value={selectedAccountId}
                  onValueChange={(value) =>
                    value !== null && setSelectedAccountId(value)
                  }
                >
                  <SelectTrigger id='upstream-account' className='w-full'>
                    <SelectValue placeholder={t('Automatic selection')} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      <SelectItem value='auto'>
                        {t('Automatic selection')}
                      </SelectItem>
                      {testAccounts.map((account) => {
                        const now = Math.floor(Date.now() / 1000)
                        const unavailable =
                          account.status !== 1 ||
                          account.rate_limited_until > now ||
                          account.overload_until > now ||
                          account.temp_disabled_until > now
                        const statusLabel =
                          account.status !== 1
                            ? t('Disabled')
                            : unavailable
                              ? t('Cooling down')
                              : t('Available')
                        return (
                          <SelectItem
                            key={account.id}
                            value={String(account.id)}
                            disabled={unavailable}
                          >
                            <span className='flex min-w-0 items-center justify-between gap-3'>
                              <span className='truncate'>
                                {account.name || `#${account.id}`} ({account.key})
                              </span>
                              <span className='text-muted-foreground text-xs'>
                                {statusLabel}
                              </span>
                            </span>
                          </SelectItem>
                        )
                      })}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'Choose a specific upstream key for single-model and batch tests. Automatic selection keeps the normal account pool routing.'
                  )}
                </p>
              </div>
            ) : null}
            <div className='grid gap-2'>
              <Label htmlFor='endpoint-type'>{t('Endpoint Type')}</Label>
              <Select
                items={[
                  ...endpointTypeOptions.map((option) => {
                    const itemValue = option.value
                    return { value: itemValue, label: t(option.label) }
                  }),
                ]}
                value={endpointType}
                onValueChange={(v) => v !== null && setEndpointType(v)}
              >
                <SelectTrigger id='endpoint-type'>
                  <SelectValue placeholder={t('Auto detect (default)')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    {endpointTypeOptions.map((option) => {
                      const itemValue = option.value
                      return (
                        <SelectItem key={itemValue} value={itemValue}>
                          {t(option.label)}
                        </SelectItem>
                      )
                    })}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Override the endpoint used for testing. Leave empty to auto detect.'
                )}
              </p>
            </div>
            <div className='grid gap-2'>
              <Label htmlFor='stream-toggle'>{t('Stream Mode')}</Label>
              <div className='flex items-center gap-2'>
                <Switch
                  id='stream-toggle'
                  checked={isStreamTest}
                  onCheckedChange={setIsStreamTest}
                  disabled={streamDisabled}
                />
                <span className='text-sm'>
                  {isStreamTest ? t('Enabled') : t('Disabled')}
                </span>
              </div>
              <p className='text-muted-foreground text-xs'>
                {t('Enable streaming mode for the test request.')}
              </p>
            </div>
          </div>

          <div className='space-y-3 max-sm:has-[div[role="toolbar"]]:pb-16'>
            <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
              <div>
                <p className='text-sm font-medium'>{t('Channel models')}</p>
                <p className='text-muted-foreground text-xs'>
                  {t('Select models to run batch tests.')}
                </p>
              </div>
              <Input
                placeholder={t('Filter models...')}
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
                className='sm:w-64'
              />
            </div>

            <div className='space-y-3'>
              <div className='overflow-hidden rounded-md border' role='region'>
                <div className='max-h-[360px] overflow-y-auto'>
                  <Table>
                    <TableHeader>
                      {table.getHeaderGroups().map((headerGroup) => (
                        <TableRow key={headerGroup.id}>
                          {headerGroup.headers.map((header) => (
                            <TableHead key={header.id}>
                              {header.isPlaceholder
                                ? null
                                : flexRender(
                                    header.column.columnDef.header,
                                    header.getContext()
                                  )}
                            </TableHead>
                          ))}
                        </TableRow>
                      ))}
                    </TableHeader>
                    <TableBody>
                      {table.getRowModel().rows.length ? (
                        table.getRowModel().rows.map((row) => (
                          <TableRow
                            key={row.id}
                            data-state={
                              row.getIsSelected() ? 'selected' : undefined
                            }
                          >
                            {row.getVisibleCells().map((cell) => (
                              <TableCell key={cell.id}>
                                {flexRender(
                                  cell.column.columnDef.cell,
                                  cell.getContext()
                                )}
                              </TableCell>
                            ))}
                          </TableRow>
                        ))
                      ) : (
                        <TableRow>
                          <TableCell
                            colSpan={table.getVisibleLeafColumns().length}
                            className='text-muted-foreground h-16 text-center text-sm'
                          >
                            {models.length
                              ? t('No models matched your search.')
                              : t('This channel has no configured models.')}
                          </TableCell>
                        </TableRow>
                      )}
                    </TableBody>
                  </Table>
                </div>
              </div>

              <DataTablePagination table={table} />
            </div>

            <TestModelsBulkActions
              table={table}
              disabled={isAnyTesting || !permissions.canOperate}
              disabledReason={noPermissionMessage}
              onTestSelected={handleBatchTest}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={handleClose}>
            {t('Close')}
          </Button>
        </DialogFooter>
        </DialogContent>
      </Dialog>
      <FailureDetailsSheet
        details={failureDetails}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) {
            setFailureDetails(null)
          }
        }}
      />
    </>
  )
}

function TestStatusCell({ result }: { result?: TestResult }) {
  const { t } = useTranslation()

  if (!result || result.status === 'idle') {
    return (
      <StatusBadge label={t('Not tested')} variant='neutral' copyable={false} />
    )
  }

  if (result.status === 'testing') {
    return (
      <div className='text-muted-foreground flex min-w-0 items-center gap-2 text-sm'>
        <Loader2 className='size-4 shrink-0 animate-spin' />
        <span className='truncate'>{t('Testing...')}</span>
      </div>
    )
  }

  if (result.status === 'success') {
    return (
      <StatusBadge label={t('Success')} variant='success' copyable={false} />
    )
  }

  return <StatusBadge label={t('Failed')} variant='danger' copyable={false} />
}

function TestResultCell({
  result,
  model,
  onOpenDetails,
}: {
  result?: TestResult
  model: string
  onOpenDetails: (details: FailureDetailsState) => void
}) {
  const { t } = useTranslation()

  if (!result || result.status === 'idle') {
    return <span className='text-muted-foreground text-sm'>-</span>
  }

  if (result.status === 'testing') {
    return <span className='text-muted-foreground text-sm'>-</span>
  }

  if (result.status === 'success') {
    return typeof result.responseTime === 'number' ? (
      <span className='text-muted-foreground text-sm'>
        {formatResponseTime(result.responseTime, t)}
      </span>
    ) : (
      <span className='text-muted-foreground text-sm'>-</span>
    )
  }

  return (
    <FailureResultContent
      result={result}
      model={model}
      onOpenDetails={onOpenDetails}
    />
  )
}

function FailureResultContent({
  result,
  model,
  onOpenDetails,
}: {
  result: TestResult
  model: string
  onOpenDetails: (details: FailureDetailsState) => void
}) {
  const { t } = useTranslation()
  const errorText = result.error?.trim()
  const isModelPriceError =
    result.errorCode === CHANNEL_TEST_MODEL_PRICE_ERROR_CODE
  const modelPriceSummary = t(
    'Model price is not configured. Please complete pricing in Models → Sync Source Models.'
  )
  const { summary, details } = getChannelTestFailureDisplay({
    errorText,
    fallbackSummary: t('Test failed'),
    isModelPriceError,
    modelPriceSummary,
  })

  return (
    <div className='flex min-w-0 items-center gap-2 text-xs whitespace-normal'>
      <p className='text-muted-foreground line-clamp-2 min-w-0 flex-1 leading-snug break-words'>
        {summary}
      </p>
      <div className='flex shrink-0 flex-wrap items-center justify-end gap-1.5'>
        {isModelPriceError && (
          <Button
            variant='outline'
            size='sm'
            className='h-7 w-fit px-2 text-xs'
            onClick={() => window.open('/models/metadata', '_blank')}
          >
            <Settings data-icon='inline-start' />
            {t('Go to Settings')}
          </Button>
        )}
        {details && (
          <Button
            variant='ghost'
            size='sm'
            className='h-7 w-fit px-2 text-xs'
            aria-haspopup='dialog'
            onClick={() => onOpenDetails({ model, summary, details })}
          >
            <Info data-icon='inline-start' />
            {t('Details')}
          </Button>
        )}
      </div>
    </div>
  )
}

function FailureDetailsSheet({
  details,
  onOpenChange,
}: {
  details: FailureDetailsState | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })

  return (
    <Sheet open={Boolean(details)} onOpenChange={onOpenChange}>
      <SheetContent
        side={isMobile ? 'bottom' : 'right'}
        className={
          isMobile
            ? sideDrawerContentClassName('h-auto max-h-[85dvh] rounded-t-xl')
            : sideDrawerContentClassName('sm:max-w-lg')
        }
      >
        {details && (
          <>
            <SheetHeader className={sideDrawerHeaderClassName('sm:px-5')}>
              <SheetTitle className='pr-10'>{t('Details')}</SheetTitle>
              <SheetDescription className='pr-10 break-words'>
                {details.model}
              </SheetDescription>
            </SheetHeader>
            <div className={sideDrawerFormClassName('gap-4 sm:px-5')}>
              <section className='space-y-1'>
                <div className='text-muted-foreground text-xs font-medium'>
                  {t('Model')}
                </div>
                <p className='text-sm font-medium break-all'>{details.model}</p>
              </section>
              <section className='space-y-1'>
                <div className='text-muted-foreground text-xs font-medium'>
                  {t('Failed')}
                </div>
                <p className='text-muted-foreground text-sm leading-relaxed break-words'>
                  {details.summary}
                </p>
              </section>
              <section className='space-y-2'>
                <div className='text-muted-foreground text-xs font-medium'>
                  {t('Details')}
                </div>
                <pre className='bg-muted/30 text-muted-foreground m-0 max-w-full rounded-md border p-3 text-xs leading-relaxed break-words whitespace-pre-wrap'>
                  {details.details}
                </pre>
              </section>
            </div>
            <SheetFooter className={sideDrawerFooterClassName('sm:px-5')}>
              <Button
                variant='outline'
                className='w-full sm:w-auto'
                onClick={() => copyToClipboard(details.details)}
              >
                {copiedText === details.details ? (
                  <Check data-icon='inline-start' className='text-success' />
                ) : (
                  <Copy data-icon='inline-start' />
                )}
                {t('Copy')}
              </Button>
            </SheetFooter>
          </>
        )}
      </SheetContent>
    </Sheet>
  )
}

function TestModelsBulkActions({
  table,
  disabled,
  disabledReason,
  onTestSelected,
}: {
  table: TanStackTable<ModelRow>
  disabled?: boolean
  disabledReason?: string
  onTestSelected: (models: string[]) => void
}) {
  const { t } = useTranslation()
  const selectedRows = table.getFilteredSelectedRowModel().rows
  const selectedModels = selectedRows.map((row) => row.original.model)

  const buttonLabel =
    selectedModels.length > 0
      ? `Test ${selectedModels.length} selected`
      : 'Test selected models'

  return (
    <BulkActionsToolbar table={table} entityName='model'>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              size='sm'
              onClick={() => onTestSelected(selectedModels)}
              disabled={disabled || selectedModels.length === 0}
              title={disabled ? disabledReason : undefined}
            />
          }
        >
          {disabled ? (
            <>
              <Loader2 className='mr-2 h-4 w-4 animate-spin' />
              {t('Testing...')}
            </>
          ) : (
            buttonLabel
          )}
        </TooltipTrigger>
        <TooltipContent>
          <p>{disabled ? disabledReason : t('Run tests for the selected models')}</p>
        </TooltipContent>
      </Tooltip>
    </BulkActionsToolbar>
  )
}
