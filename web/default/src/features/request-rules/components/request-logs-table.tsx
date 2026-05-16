import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
  type PaginationState,
} from '@tanstack/react-table'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { DataTablePage } from '@/components/data-table'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  getRequestLogs,
  getRequestLogDetail,
  deleteRequestLogs,
  requestLogsQueryKeys,
} from '../api'
import type { RequestLog } from '../types'

export function RequestLogsTable() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  // 分页状态
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 20,
  })

  // 详情对话框
  const [detailLog, setDetailLog] = useState<RequestLog | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)

  // 清理确认
  const [cleanupOpen, setCleanupOpen] = useState(false)

  // 获取日志列表
  const { data, isLoading, isFetching } = useQuery({
    queryKey: requestLogsQueryKeys.list({
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
    }),
    queryFn: () =>
      getRequestLogs({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }),
    placeholderData: (previousData) => previousData,
  })

  // 获取日志详情
  const detailQuery = useQuery({
    queryKey: requestLogsQueryKeys.detail(detailLog?.id ?? 0),
    queryFn: () => getRequestLogDetail(detailLog!.id),
    enabled: detailOpen && detailLog !== null,
  })

  // 清理日志 mutation
  const cleanupMutation = useMutation({
    mutationFn: () => deleteRequestLogs(),
    onSuccess: (res) => {
      if (res.success) {
        toast.success(t('Logs cleaned up successfully'))
        queryClient.invalidateQueries({ queryKey: requestLogsQueryKeys.all })
      }
    },
  })

  const handleViewDetail = (log: RequestLog) => {
    setDetailLog(log)
    setDetailOpen(true)
  }

  const handleCleanup = () => {
    cleanupMutation.mutate()
    setCleanupOpen(false)
  }

  // 格式化时间戳
  const formatTime = (ts: number) => {
    if (!ts) return '-'
    return new Date(ts * 1000).toLocaleString()
  }

  // 格式化延迟
  const formatLatency = (ms: number) => {
    if (ms < 1000) return `${ms}ms`
    return `${(ms / 1000).toFixed(2)}s`
  }

  // 列定义
  const columns: ColumnDef<RequestLog>[] = [
    {
      accessorKey: 'id',
      header: 'ID',
      cell: ({ row }) => (
        <span className='font-mono text-xs'>{row.original.id}</span>
      ),
    },
    {
      accessorKey: 'request_rule_id',
      header: t('Rule ID'),
      cell: ({ row }) => (
        <Badge variant='outline'>{row.original.request_rule_id}</Badge>
      ),
    },
    {
      accessorKey: 'model_name',
      header: t('Model'),
      cell: ({ row }) => (
        <span className='font-mono text-xs'>{row.original.model_name}</span>
      ),
    },
    {
      accessorKey: 'relay_format',
      header: t('Format'),
      cell: ({ row }) => (
        <Badge variant='secondary'>
          {row.original.relay_format || '-'}
        </Badge>
      ),
    },
    {
      accessorKey: 'status_code',
      header: t('Status Code'),
      cell: ({ row }) => {
        const code = row.original.status_code
        const variant =
          code >= 200 && code < 300
            ? 'default'
            : code >= 400
              ? 'destructive'
              : 'secondary'
        return <Badge variant={variant}>{code}</Badge>
      },
    },
    {
      accessorKey: 'latency',
      header: t('Latency'),
      cell: ({ row }) => (
        <span className='text-xs'>
          {formatLatency(row.original.latency)}
        </span>
      ),
    },
    {
      accessorKey: 'created_at',
      header: t('Time'),
      cell: ({ row }) => (
        <span className='text-muted-foreground text-xs'>
          {formatTime(row.original.created_at)}
        </span>
      ),
    },
    {
      id: 'actions',
      header: t('Actions'),
      cell: ({ row }) => (
        <Button
          variant='ghost'
          size='xs'
          onClick={() => handleViewDetail(row.original)}
        >
          {t('Detail')}
        </Button>
      ),
    },
  ]

  const logs = data?.data?.items || []
  const totalCount = data?.data?.total || 0

  const table = useReactTable({
    data: logs,
    columns,
    pageCount: Math.ceil(totalCount / pagination.pageSize),
    state: { pagination },
    onPaginationChange: setPagination,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
  })

  const logDetail = detailQuery.data?.data

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('No Logs Found')}
        emptyDescription={t('No request logs recorded yet.')}
        toolbar={
          <div className='flex items-center justify-end'>
            <Button
              variant='destructive'
              onClick={() => setCleanupOpen(true)}
              disabled={logs.length === 0}
            >
              {t('Clean Up Logs')}
            </Button>
          </div>
        }
      />

      {/* 日志详情对话框 */}
      <Dialog open={detailOpen} onOpenChange={setDetailOpen}>
        <DialogContent className='max-h-[85vh] overflow-y-auto sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>
              {t('Request Log Detail')} #{detailLog?.id}
            </DialogTitle>
          </DialogHeader>
          {detailQuery.isLoading ? (
            <div className='text-muted-foreground py-8 text-center text-sm'>
              {t('Loading...')}
            </div>
          ) : logDetail ? (
            <div className='space-y-4'>
              {/* 基础信息 */}
              <div className='grid grid-cols-2 gap-2 text-sm'>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Request ID')}:
                  </span>{' '}
                  <span className='font-mono text-xs'>
                    {logDetail.request_id}
                  </span>
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Rule ID')}:
                  </span>{' '}
                  {logDetail.request_rule_id}
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('User ID')}:
                  </span>{' '}
                  {logDetail.user_id}
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Token ID')}:
                  </span>{' '}
                  {logDetail.token_id}
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Channel ID')}:
                  </span>{' '}
                  {logDetail.channel_id}
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Model')}:
                  </span>{' '}
                  {logDetail.model_name}
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Status Code')}:
                  </span>{' '}
                  {logDetail.status_code}
                </div>
                <div>
                  <span className='text-muted-foreground'>
                    {t('Latency')}:
                  </span>{' '}
                  {formatLatency(logDetail.latency)}
                </div>
              </div>

              {/* 请求体 */}
              {logDetail.request_body && (
                <div>
                  <h4 className='mb-1 text-sm font-medium'>
                    {t('Request Body')}
                  </h4>
                  <pre className='bg-muted max-h-60 overflow-auto rounded-lg p-3 text-xs'>
                    {logDetail.request_body}
                  </pre>
                </div>
              )}

              {/* 响应体 */}
              {logDetail.response_body && (
                <div>
                  <h4 className='mb-1 text-sm font-medium'>
                    {t('Response Body')}
                  </h4>
                  <pre className='bg-muted max-h-60 overflow-auto rounded-lg p-3 text-xs'>
                    {logDetail.response_body}
                  </pre>
                </div>
              )}
            </div>
          ) : (
            <div className='text-muted-foreground py-8 text-center text-sm'>
              {t('No data available')}
            </div>
          )}
        </DialogContent>
      </Dialog>

      {/* 清理确认对话框 */}
      <AlertDialog open={cleanupOpen} onOpenChange={setCleanupOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Clean Up Logs')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Are you sure you want to delete all request logs? This action cannot be undone.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleCleanup}
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
            >
              {t('Delete All')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
