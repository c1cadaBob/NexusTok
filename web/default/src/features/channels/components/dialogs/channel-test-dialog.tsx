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
import {
  Check,
  ChevronDown,
  Copy,
  Info,
  KeyRound,
  Loader2,
  Search,
  Settings,
} from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
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
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from '@/components/ui/drawer'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
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
import {
  formatUpstreamRatioCompact,
  getUpstreamRatioDisplayValue,
} from '../../lib/upstream-sync'
import { getChannelAccounts } from '../../api'
import { CHANNEL_STATUS } from '../../constants'
import { useChannelPermissions } from '../../hooks/use-channel-permissions'
import { useChannels } from '../channels-provider'
import type { ChannelAccount } from '../../types'

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

type AccountAvailability = {
  unavailable: boolean
  label: string
  reason?: string
  variant: 'success' | 'warning' | 'danger' | 'neutral'
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

function parseAccountModels(value: string | null | undefined): string[] {
  if (!value) return []
  return value
    .split(',')
    .map((model) => model.trim())
    .filter(Boolean)
}

function getAccountDisplayName(account: ChannelAccount): string {
  return account.name?.trim() || `#${account.id}`
}

function getAccountRatioText(account: ChannelAccount): string {
  const ratio = getUpstreamRatioDisplayValue(account)
  return ratio == null ? '-' : `${formatUpstreamRatioCompact(ratio)}x`
}

function getAccountModelCount(account: ChannelAccount): number {
  return parseAccountModels(account.models).length
}

function getAccountGroupText(account: ChannelAccount): string {
  const parts = [account.group, account.access_groups]
    .map((value) => value?.trim())
    .filter(Boolean)
  return parts.length ? parts.join(' / ') : '-'
}

function getAccountAvailability(
  account: ChannelAccount,
  nowSeconds: number,
  t: (key: string) => string
): AccountAvailability {
  if (account.status !== CHANNEL_STATUS.ENABLED) {
    const label =
      account.status === CHANNEL_STATUS.AUTO_DISABLED
        ? t('Auto Disabled')
        : t('Disabled')
    return {
      unavailable: true,
      label,
      reason: account.disabled_reason || account.last_error || label,
      variant:
        account.status === CHANNEL_STATUS.AUTO_DISABLED ? 'danger' : 'neutral',
    }
  }

  const cooldownUntil = Math.max(
    account.rate_limited_until || 0,
    account.overload_until || 0,
    account.temp_disabled_until || 0
  )
  if (cooldownUntil > nowSeconds) {
    return {
      unavailable: true,
      label: t('Cooling down'),
      reason: account.disabled_reason || t('Cooling down'),
      variant: 'warning',
    }
  }

  return {
    unavailable: false,
    label: t('Available'),
    variant: 'success',
  }
}

export function ChannelTestDialog({
  open,
  onOpenChange,
}: ChannelTestDialogProps) {
  const { t } = useTranslation()
  const { currentRow } = useChannels()
  const [endpointType, setEndpointType] = useState('auto')
  const [selectedAccountId, setSelectedAccountId] = useState('auto')
  const [accountSelectorOpen, setAccountSelectorOpen] = useState(false)
  const [accountSearch, setAccountSearch] = useState('')
  const [accountStatusNow, setAccountStatusNow] = useState(0)
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

  const resetModelTestState = useCallback(() => {
    setSearchTerm('')
    setTestResults({})
    setRowSelection({})
    setTestingModels(() => new Set())
    setIsBatchTesting(false)
    setFailureDetails(null)
    setPagination({ pageIndex: 0, pageSize: 10 })
  }, [])

  const resetState = useCallback(() => {
    setEndpointType('auto')
    setSelectedAccountId('auto')
    setAccountSelectorOpen(false)
    setAccountSearch('')
    setIsStreamTest(false)
    resetModelTestState()
  }, [resetModelTestState])

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

  const testAccounts = useMemo(
    () => channelAccountsQuery.data?.data?.accounts.items ?? [],
    [channelAccountsQuery.data?.data?.accounts.items]
  )

  const selectedAccount =
    selectedAccountId === 'auto'
      ? null
      : testAccounts.find((account) => String(account.id) === selectedAccountId)

  const handleSelectedAccountChange = useCallback(
    (value: string) => {
      setSelectedAccountId(value)
      setAccountSelectorOpen(false)
      setAccountSearch('')
      resetModelTestState()
    },
    [resetModelTestState]
  )

  useEffect(() => {
    if (
      selectedAccountId !== 'auto' &&
      !testAccounts.some((account) => String(account.id) === selectedAccountId)
    ) {
      handleSelectedAccountChange('auto')
    }
  }, [handleSelectedAccountChange, selectedAccountId, testAccounts])

  useEffect(() => {
    if (streamDisabled) {
      setIsStreamTest(false)
    }
  }, [streamDisabled])

  useEffect(() => {
    if (open || accountSelectorOpen) {
      setAccountStatusNow(Math.floor(Date.now() / 1000))
    }
  }, [accountSelectorOpen, open])

  const modelsValue =
    selectedAccountId === 'auto'
      ? (currentRow?.models ?? '')
      : (selectedAccount?.models ?? '')
  const defaultTestModel = currentRow?.test_model?.trim()

  const models = useMemo(() => {
    return parseAccountModels(modelsValue)
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

  const modelSectionTitle =
    selectedAccountId === 'auto' ? t('Channel models') : t('Synced key models')
  const modelSectionDescription =
    selectedAccountId === 'auto'
      ? t('Select models to run batch tests.')
      : t('Only models synced for the selected upstream key are shown.')
  const emptyModelsMessage =
    selectedAccountId === 'auto'
      ? t('This channel has no configured models.')
      : t('This upstream key has no synced models.')

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
                  <Label htmlFor='upstream-account-trigger'>
                    {t('Upstream key for this test')}
                  </Label>
                  <UpstreamAccountSelector
                    open={accountSelectorOpen}
                    onOpenChange={setAccountSelectorOpen}
                    accounts={testAccounts}
                    selectedAccountId={selectedAccountId}
                    selectedAccount={selectedAccount}
                    searchValue={accountSearch}
                    onSearchValueChange={setAccountSearch}
                    onValueChange={handleSelectedAccountChange}
                    loading={channelAccountsQuery.isFetching}
                    nowSeconds={accountStatusNow}
                  />
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
                  <p className='text-sm font-medium'>{modelSectionTitle}</p>
                  <p className='text-muted-foreground text-xs'>
                    {modelSectionDescription}
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
                <div
                  className='overflow-hidden rounded-md border'
                  role='region'
                >
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
                                : emptyModelsMessage}
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

function UpstreamAccountSelector({
  open,
  onOpenChange,
  accounts,
  selectedAccountId,
  selectedAccount,
  searchValue,
  onSearchValueChange,
  onValueChange,
  loading,
  nowSeconds,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  accounts: ChannelAccount[]
  selectedAccountId: string
  selectedAccount: ChannelAccount | null | undefined
  searchValue: string
  onSearchValueChange: (value: string) => void
  onValueChange: (value: string) => void
  loading?: boolean
  nowSeconds: number
}) {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const normalizedSearch = searchValue.trim().toLowerCase()
  const filteredAccounts = useMemo(() => {
    if (!normalizedSearch) return accounts
    return accounts.filter((account) => {
      const fields = [
        account.name,
        String(account.id),
        account.key,
        account.models,
        account.group,
        account.access_groups,
        getAccountRatioText(account),
      ]
      return fields
        .filter(Boolean)
        .some((field) => String(field).toLowerCase().includes(normalizedSearch))
    })
  }, [accounts, normalizedSearch])

  const triggerContent = (
    <AccountSelectorTriggerContent
      selectedAccountId={selectedAccountId}
      selectedAccount={selectedAccount}
    />
  )
  const tableContent = (
    <UpstreamAccountSelectorTable
      accounts={filteredAccounts}
      selectedAccountId={selectedAccountId}
      loading={loading}
      searchValue={searchValue}
      onSearchValueChange={onSearchValueChange}
      onValueChange={onValueChange}
      nowSeconds={nowSeconds}
    />
  )

  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerTrigger asChild>
          <Button
            id='upstream-account-trigger'
            type='button'
            variant='outline'
            className='h-auto min-h-12 w-full justify-between gap-2 px-3 py-2 text-left'
          >
            {triggerContent}
          </Button>
        </DrawerTrigger>
        <DrawerContent className='h-[85dvh] max-h-[85dvh] p-0'>
          <div className='flex min-h-0 flex-1 flex-col overflow-hidden'>
            <DrawerHeader className='border-b px-4 py-3 text-left'>
              <DrawerTitle>{t('Choose upstream key')}</DrawerTitle>
              <DrawerDescription>
                {t('Upstream key for this test')}
              </DrawerDescription>
            </DrawerHeader>
            <div className='min-h-0 flex-1 overflow-hidden'>{tableContent}</div>
          </div>
        </DrawerContent>
      </Drawer>
    )
  }

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger
        render={
          <Button
            id='upstream-account-trigger'
            type='button'
            variant='outline'
            role='combobox'
            aria-expanded={open}
            className='h-auto min-h-12 w-full justify-between gap-2 px-3 py-2 text-left'
          />
        }
      >
        {triggerContent}
      </PopoverTrigger>
      <PopoverContent
        align='start'
        className='w-[min(920px,calc(100vw-2rem))] overflow-hidden p-0'
        onWheel={(event) => event.stopPropagation()}
        onTouchMove={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        {tableContent}
      </PopoverContent>
    </Popover>
  )
}

function AccountSelectorTriggerContent({
  selectedAccountId,
  selectedAccount,
}: {
  selectedAccountId: string
  selectedAccount: ChannelAccount | null | undefined
}) {
  const { t } = useTranslation()
  const accountName = selectedAccount
    ? getAccountDisplayName(selectedAccount)
    : ''
  const ratioText = selectedAccount ? getAccountRatioText(selectedAccount) : ''

  return (
    <>
      <span className='flex min-w-0 flex-1 flex-col gap-0.5'>
        <span className='truncate text-sm font-medium'>
          {selectedAccountId === 'auto' || !selectedAccount
            ? t('Automatic selection')
            : `${accountName} #${selectedAccount.id}`}
        </span>
        <span className='text-muted-foreground truncate text-xs'>
          {selectedAccountId === 'auto' || !selectedAccount
            ? t('Normal account pool routing')
            : `${selectedAccount.key || '-'} · ${t('Conversion ratio')} ${ratioText}`}
        </span>
      </span>
      <ChevronDown data-icon='inline-end' className='shrink-0 opacity-60' />
    </>
  )
}

function UpstreamAccountSelectorTable({
  accounts,
  selectedAccountId,
  loading,
  searchValue,
  onSearchValueChange,
  onValueChange,
  nowSeconds,
}: {
  accounts: ChannelAccount[]
  selectedAccountId: string
  loading?: boolean
  searchValue: string
  onSearchValueChange: (value: string) => void
  onValueChange: (value: string) => void
  nowSeconds: number
}) {
  const { t } = useTranslation()

  return (
    <div className='flex h-full min-h-0 flex-col'>
      <div className='border-b p-3'>
        <div className='relative'>
          <Search
            className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2'
            aria-hidden='true'
          />
          <Input
            value={searchValue}
            onChange={(event) => onSearchValueChange(event.target.value)}
            placeholder={t('Search upstream keys...')}
            className='pl-8'
          />
        </div>
      </div>
      <div className='min-h-0 flex-1 overflow-auto'>
        <Table className='min-w-[760px]'>
          <TableHeader>
            <TableRow>
              <TableHead className='w-12'>{t('Select')}</TableHead>
              <TableHead>{t('Account')}</TableHead>
              <TableHead>{t('Key')}</TableHead>
              <TableHead>{t('Key status')}</TableHead>
              <TableHead className='text-right'>{t('Models count')}</TableHead>
              <TableHead className='text-right'>
                {t('Conversion ratio')}
              </TableHead>
              <TableHead>{t('Group / Access Groups')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow
              className='cursor-pointer'
              data-state={selectedAccountId === 'auto' ? 'selected' : undefined}
              tabIndex={0}
              onClick={() => onValueChange('auto')}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault()
                  onValueChange('auto')
                }
              }}
            >
              <TableCell>
                {selectedAccountId === 'auto' ? (
                  <Check className='text-success' aria-hidden='true' />
                ) : (
                  <span className='text-muted-foreground'>-</span>
                )}
              </TableCell>
              <TableCell>
                <div className='flex min-w-0 items-center gap-2'>
                  <KeyRound className='text-muted-foreground shrink-0' />
                  <div className='min-w-0'>
                    <p className='truncate text-sm font-medium'>
                      {t('Automatic selection')}
                    </p>
                    <p className='text-muted-foreground truncate text-xs'>
                      {t('Normal account pool routing')}
                    </p>
                  </div>
                </div>
              </TableCell>
              <TableCell className='text-muted-foreground text-xs'>-</TableCell>
              <TableCell>
                <StatusBadge
                  label={t('Available')}
                  variant='success'
                  copyable={false}
                />
              </TableCell>
              <TableCell className='text-muted-foreground text-right text-xs'>
                -
              </TableCell>
              <TableCell className='text-muted-foreground text-right text-xs'>
                -
              </TableCell>
              <TableCell className='text-muted-foreground text-xs'>-</TableCell>
            </TableRow>
            {loading ? (
              <TableRow>
                <TableCell colSpan={7} className='h-14 text-center'>
                  <span className='text-muted-foreground inline-flex items-center gap-2 text-sm'>
                    <Loader2 className='animate-spin' aria-hidden='true' />
                    {t('Loading...')}
                  </span>
                </TableCell>
              </TableRow>
            ) : accounts.length ? (
              accounts.map((account) => {
                const availability = getAccountAvailability(
                  account,
                  nowSeconds,
                  t
                )
                const selected = selectedAccountId === String(account.id)

                return (
                  <TableRow
                    key={account.id}
                    className={cn(
                      !availability.unavailable && 'cursor-pointer',
                      availability.unavailable &&
                        'cursor-not-allowed opacity-60'
                    )}
                    data-state={selected ? 'selected' : undefined}
                    tabIndex={availability.unavailable ? -1 : 0}
                    aria-disabled={availability.unavailable}
                    title={availability.reason}
                    onClick={() => {
                      if (!availability.unavailable) {
                        onValueChange(String(account.id))
                      }
                    }}
                    onKeyDown={(event) => {
                      if (
                        !availability.unavailable &&
                        (event.key === 'Enter' || event.key === ' ')
                      ) {
                        event.preventDefault()
                        onValueChange(String(account.id))
                      }
                    }}
                  >
                    <TableCell>
                      {selected ? (
                        <Check className='text-success' aria-hidden='true' />
                      ) : (
                        <span className='text-muted-foreground'>-</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <div className='min-w-0'>
                        <p className='truncate text-sm font-medium'>
                          {getAccountDisplayName(account)}
                        </p>
                        <p className='text-muted-foreground font-mono text-xs'>
                          #{account.id}
                        </p>
                      </div>
                    </TableCell>
                    <TableCell>
                      <span className='font-mono text-xs'>
                        {account.key || '-'}
                      </span>
                    </TableCell>
                    <TableCell>
                      <div className='flex min-w-0 flex-col gap-0.5'>
                        <StatusBadge
                          label={availability.label}
                          variant={availability.variant}
                          copyable={false}
                        />
                        {availability.reason && (
                          <span className='text-muted-foreground max-w-36 truncate text-[11px]'>
                            {availability.reason}
                          </span>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs tabular-nums'>
                      {getAccountModelCount(account)}
                    </TableCell>
                    <TableCell className='text-right font-mono text-xs tabular-nums'>
                      {getAccountRatioText(account)}
                    </TableCell>
                    <TableCell>
                      <span className='text-muted-foreground block max-w-40 truncate text-xs'>
                        {getAccountGroupText(account)}
                      </span>
                    </TableCell>
                  </TableRow>
                )
              })
            ) : (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className='text-muted-foreground h-14 text-center text-sm'
                >
                  {searchValue.trim()
                    ? t('No upstream keys matched your search.')
                    : t('No upstream keys were found for this account.')}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  )
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
          <p>
            {disabled ? disabledReason : t('Run tests for the selected models')}
          </p>
        </TooltipContent>
      </Tooltip>
    </BulkActionsToolbar>
  )
}
