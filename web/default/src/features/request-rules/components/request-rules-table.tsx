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
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { DataTablePage } from '@/components/data-table'
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
  getRequestRules,
  deleteRequestRule,
  updateRequestRuleStatus,
  requestRulesQueryKeys,
} from '../api'
import type { RequestRule } from '../types'
import {
  RELAY_FORMAT_OPTIONS,
  RELAY_FORMAT_ALL_VALUE,
  MODEL_MATCH_MODE_OPTIONS,
} from '../types'

type RequestRulesTableProps = {
  onCreateRule: () => void
  onEditRule: (rule: RequestRule) => void
}

export function RequestRulesTable(props: RequestRulesTableProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  // 分页状态
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 20,
  })

  // 删除确认弹窗
  const [deleteId, setDeleteId] = useState<number | null>(null)

  // 获取规则列表
  const { data, isLoading, isFetching } = useQuery({
    queryKey: requestRulesQueryKeys.list({
      p: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
    }),
    queryFn: () =>
      getRequestRules({
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }),
    placeholderData: (previousData) => previousData,
  })

  // 更新状态 mutation
  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: number; status: number }) =>
      updateRequestRuleStatus(id, status),
    onSuccess: (res) => {
      if (res.success) {
        toast.success(t('Status updated successfully'))
        queryClient.invalidateQueries({ queryKey: requestRulesQueryKeys.all })
      }
    },
  })

  // 删除 mutation
  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteRequestRule(id),
    onSuccess: (res) => {
      if (res.success) {
        toast.success(t('Rule deleted successfully'))
        queryClient.invalidateQueries({ queryKey: requestRulesQueryKeys.all })
      }
    },
  })

  const handleDelete = () => {
    if (deleteId !== null) {
      deleteMutation.mutate(deleteId)
      setDeleteId(null)
    }
  }

  // 获取 relay format 标签
  const getRelayFormatLabel = (value: string) => {
    // 空字符串映射到 'all'
    const lookupValue = value || RELAY_FORMAT_ALL_VALUE
    const option = RELAY_FORMAT_OPTIONS.find((o) => o.value === lookupValue)
    return option ? option.labelKey : value || t('All')
  }

  // 获取 match mode 标签
  const getMatchModeLabel = (value: number) => {
    const option = MODEL_MATCH_MODE_OPTIONS.find((o) => o.value === value)
    return option ? t(option.labelKey) : String(value)
  }

  // 列定义
  const columns: ColumnDef<RequestRule>[] = [
    {
      accessorKey: 'name',
      header: t('Name'),
      cell: ({ row }) => (
        <div className='flex flex-col'>
          <span className='font-medium'>{row.original.name}</span>
          {row.original.description && (
            <span className='text-muted-foreground text-xs'>
              {row.original.description}
            </span>
          )}
        </div>
      ),
    },
    {
      accessorKey: 'status',
      header: t('Status'),
      cell: ({ row }) => (
        <Switch
          size='sm'
          checked={row.original.status === 1}
          onCheckedChange={(checked) =>
            statusMutation.mutate({
              id: row.original.id,
              status: checked ? 1 : 0,
            })
          }
          disabled={statusMutation.isPending}
        />
      ),
    },
    {
      accessorKey: 'priority',
      header: t('Priority'),
      cell: ({ row }) => (
        <Badge variant='outline'>{row.original.priority}</Badge>
      ),
    },
    {
      accessorKey: 'relay_format',
      header: t('Format'),
      cell: ({ row }) => (
        <Badge variant='secondary'>
          {getRelayFormatLabel(row.original.relay_format)}
        </Badge>
      ),
    },
    {
      accessorKey: 'model_pattern',
      header: t('Model Pattern'),
      cell: ({ row }) => (
        <div className='flex items-center gap-1.5'>
          <span className='font-mono text-xs'>
            {row.original.model_pattern || '*'}
          </span>
          <Badge variant='outline' className='text-[10px]'>
            {getMatchModeLabel(row.original.model_match_mode)}
          </Badge>
        </div>
      ),
    },
    {
      id: 'actions',
      header: t('Actions'),
      cell: ({ row }) => (
        <div className='flex items-center gap-1'>
          <Button
            variant='ghost'
            size='xs'
            onClick={() => props.onEditRule(row.original)}
          >
            {t('Edit')}
          </Button>
          <Button
            variant='ghost'
            size='xs'
            className='text-destructive'
            onClick={() => setDeleteId(row.original.id)}
          >
            {t('Delete')}
          </Button>
        </div>
      ),
    },
  ]

  const rules = data?.data?.items || []
  const totalCount = data?.data?.total || 0

  const table = useReactTable({
    data: rules,
    columns,
    pageCount: Math.ceil(totalCount / pagination.pageSize),
    state: { pagination },
    onPaginationChange: setPagination,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
  })

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('No Rules Found')}
        emptyDescription={t(
          'No request rules configured. Create your first rule to get started.'
        )}
        toolbar={
          <div className='flex items-center justify-end'>
            <Button onClick={props.onCreateRule}>{t('Create Rule')}</Button>
          </div>
        }
      />

      {/* 删除确认对话框 */}
      <AlertDialog
        open={deleteId !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteId(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Delete Rule')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'Are you sure you want to delete this rule? This action cannot be undone.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
            >
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
