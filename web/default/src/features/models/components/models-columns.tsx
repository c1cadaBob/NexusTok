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
import { type ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'
import { formatTimestampToDate } from '@/lib/format'
import { getLobeIcon } from '@/lib/lobe-icon'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { BadgeCell, BadgeListCell } from '@/components/data-table'
import { DataTableColumnHeader } from '@/components/data-table/column-header'
import { GroupBadge } from '@/components/group-badge'
import { ProviderBadge } from '@/components/provider-badge'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import {
  getModelStatusConfig,
  getNameRuleConfig,
  getQuotaTypeConfig,
} from '../constants'
import { parseModelTags, formatEndpointsDisplay } from '../lib'
import type { Model, Vendor } from '../types'
import { DataTableRowActions } from './data-table-row-actions'
import { DescriptionCell } from './description-cell'

/**
 * 生成模型表格列配置。
 */
export function useModelsColumns(vendors: Vendor[] = []): ColumnDef<Model>[] {
  const { t } = useTranslation()

  // 获取需要随语言变化的列配置。
  const NAME_RULE_CONFIG = getNameRuleConfig(t)
  const MODEL_STATUS_CONFIG = getModelStatusConfig(t)
  const QUOTA_TYPE_CONFIG = getQuotaTypeConfig(t)

  const vendorMap: Record<number, Vendor> = {}
  vendors.forEach((v) => {
    vendorMap[v.id] = v
  })

  return [
    // 选择列
    {
      id: 'select',
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          indeterminate={table.getIsSomePageRowsSelected()}
          onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
          aria-label='Select all'
        />
      ),
      cell: ({ row }) => (
        <Checkbox
          checked={row.getIsSelected()}
          onCheckedChange={(value) => row.toggleSelected(!!value)}
          aria-label='Select row'
        />
      ),
      enableSorting: false,
      enableHiding: false,
      size: 40,
    },

    // ID 列
    {
      accessorKey: 'id',
      meta: { label: t('ID'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title='ID' />
      ),
      cell: ({ row }) => {
        const id = row.getValue('id') as number
        return <TableId value={id} copyable />
      },
      size: 80,
    },

    // 图标列
    {
      accessorKey: 'icon',
      meta: { label: t('Icon'), mobileHidden: true },
      header: t('Icon'),
      cell: ({ row }) => {
        const model = row.original
        const iconKey =
          model.icon ||
          vendorMap[model.vendor_id || 0]?.icon ||
          model.model_name?.[0] ||
          'N'
        const icon = getLobeIcon(iconKey, 20)

        return <div className='flex items-center justify-center'>{icon}</div>
      },
      size: 70,
      enableSorting: false,
    },

    // 模型名称列
    {
      accessorKey: 'model_name',
      meta: { label: t('Model Name'), mobileTitle: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Model Name')} />
      ),
      cell: ({ row }) => {
        const name = row.getValue('model_name') as string
        return (
          <BadgeCell>
            <StatusBadge
              label={name}
              variant='neutral'
              copyText={name}
              size='sm'
              className='font-mono'
            />
          </BadgeCell>
        )
      },
      minSize: 200,
    },

    // 匹配类型列
    {
      accessorKey: 'name_rule',
      meta: { label: t('Match Type') },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Match Type')} />
      ),
      cell: ({ row }) => {
        const rule = row.getValue('name_rule') as 0 | 1 | 2 | 3
        const model = row.original
        const config = NAME_RULE_CONFIG[rule]

        let label = config.label
        if (rule !== 0 && model.matched_count) {
          label = `${config.label} (${model.matched_count})`
        }

        const badge = (
          <StatusBadge
            label={label}
            variant={
              (config.color === 'error' ? 'danger' : config.color) as
                | 'neutral'
                | 'success'
                | 'warning'
                | 'danger'
                | 'info'
            }
            size='sm'
          />
        )

        // 非精确匹配规则展示已匹配模型列表。
        if (
          rule !== 0 &&
          model.matched_models &&
          model.matched_models.length > 0
        ) {
          const matchedBadges = model.matched_models.map((m, idx) => (
            <StatusBadge key={idx} label={m} autoColor={m} size='sm' />
          ))

          return (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger render={<div />}>{badge}</TooltipTrigger>
                <TooltipContent
                  side='top'
                  className='border-border bg-popover max-h-48 max-w-[320px] overflow-y-auto p-2'
                >
                  <div className='flex flex-wrap gap-1'>{matchedBadges}</div>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )
        }

        return badge
      },
      size: 140,
      enableSorting: false,
    },

    // 状态列
    {
      accessorKey: 'status',
      meta: { label: t('Status'), mobileBadge: true },
      header: t('Status'),
      cell: ({ row }) => {
        const status = row.getValue('status') as number
        const config =
          MODEL_STATUS_CONFIG[status as 0 | 1] || MODEL_STATUS_CONFIG[0]

        return (
          <BadgeCell>
            <StatusBadge
              label={config.label}
              variant={config.variant}
              showDot={config.showDot}
              size='sm'
              copyable={false}
            />
          </BadgeCell>
        )
      },
      filterFn: (row, id, value) => {
        if (!value || value.length === 0 || value.includes('all')) return true
        const status = row.getValue(id) as number
        if (value.includes('enabled')) return status === 1
        if (value.includes('disabled')) return status !== 1
        return false
      },
      size: 120,
      enableSorting: false,
    },

    // 供应商列
    {
      accessorKey: 'vendor_id',
      meta: { label: t('Vendor') },
      header: t('Vendor'),
      cell: ({ row }) => {
        const vendorId = row.getValue('vendor_id') as number
        const vendor = vendorMap[vendorId]

        if (!vendor) {
          return <span className='text-muted-foreground text-xs'>-</span>
        }

        return (
          <BadgeCell>
            <ProviderBadge
              iconKey={vendor.icon}
              label={vendor.name}
              className='max-w-[150px]'
            />
          </BadgeCell>
        )
      },
      filterFn: (row, id, value) => {
        if (!value || value.length === 0 || value.includes('all')) return true
        return value.includes(String(row.getValue(id)))
      },
      size: 150,
      enableSorting: false,
    },

    // 描述列
    {
      accessorKey: 'description',
      meta: { label: t('Description'), mobileHidden: true },
      header: t('Description'),
      cell: ({ row }) => {
        const description = row.getValue('description') as string
        const modelName = row.getValue('model_name') as string

        return (
          <DescriptionCell modelName={modelName} description={description} />
        )
      },
      size: 150,
      enableSorting: false,
    },

    // 标签列
    {
      accessorKey: 'tags',
      meta: { label: t('Tags'), mobileHidden: true },
      header: t('Tags'),
      cell: ({ row }) => {
        const tags = row.getValue('tags') as string
        const tagArray = parseModelTags(tags)

        if (tagArray.length === 0) {
          return <span className='text-muted-foreground text-xs'>-</span>
        }

        const tagBadges = tagArray.map((tag, idx) => (
          <StatusBadge key={idx} label={tag} autoColor={tag} size='sm' />
        ))

        return <BadgeListCell items={tagBadges} />
      },
      size: 150,
      enableSorting: false,
    },

    // 端点列
    {
      accessorKey: 'endpoints',
      meta: { label: t('Endpoints'), mobileHidden: true },
      header: t('Endpoints'),
      cell: ({ row }) => {
        const endpoints = row.getValue('endpoints') as string
        const endpointArray = formatEndpointsDisplay(endpoints)

        if (endpointArray.length === 0) {
          return <span className='text-muted-foreground text-xs'>-</span>
        }

        const endpointBadges = endpointArray.map((ep, idx) => (
          <StatusBadge key={idx} label={ep} autoColor={ep} size='sm' />
        ))

        return <BadgeListCell items={endpointBadges} />
      },
      size: 150,
      enableSorting: false,
    },

    // 绑定渠道列
    {
      accessorKey: 'bound_channels',
      meta: { label: t('Bound Channels'), mobileHidden: true },
      header: t('Bound Channels'),
      cell: ({ row }) => {
        const channels = row.getValue('bound_channels') as Array<{
          id: number
          name: string
          type?: number
          status?: number
        }>

        if (!channels || channels.length === 0) {
          return <span className='text-muted-foreground text-xs'>-</span>
        }

        const channelBadges = channels.map((c, idx) => (
          <StatusBadge
            key={idx}
            label={`${c.name} (${c.type})`}
            autoColor={c.name}
            size='sm'
          />
        ))

        return <BadgeListCell items={channelBadges} />
      },
      size: 150,
      enableSorting: false,
    },

    // 可用分组列
    {
      accessorKey: 'enable_groups',
      meta: { label: t('Enable Groups'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Enable Groups')} />
      ),
      cell: ({ row }) => {
        const groups = row.getValue('enable_groups') as string[]

        if (!groups || groups.length === 0) {
          return <span className='text-muted-foreground text-xs'>-</span>
        }

        const groupBadges = groups.map((g) => (
          <GroupBadge key={g} group={g} size='sm' />
        ))

        return <BadgeListCell items={groupBadges} />
      },
      size: 150,
      enableSorting: false,
    },

    // 计费类型列
    {
      accessorKey: 'quota_types',
      meta: { label: t('Quota Types'), mobileHidden: true },
      header: t('Quota Types'),
      cell: ({ row }) => {
        const quotaTypes = row.getValue('quota_types') as number[]

        if (!quotaTypes || quotaTypes.length === 0) {
          return <span className='text-muted-foreground text-xs'>-</span>
        }

        const quotaBadges = quotaTypes.map((qt, idx) => {
          const config = QUOTA_TYPE_CONFIG[qt]
          return (
            <StatusBadge
              key={idx}
              label={config?.label || String(qt)}
              variant={
                (config?.color === 'error' ? 'danger' : config?.color) as
                  | 'neutral'
                  | 'success'
                  | 'warning'
                  | 'danger'
                  | 'info'
              }
              size='sm'
            />
          )
        })

        return <BadgeListCell items={quotaBadges} />
      },
      size: 150,
      enableSorting: false,
    },

    // 官方同步列
    {
      accessorKey: 'sync_official',
      meta: { label: t('Official Sync'), mobileHidden: true },
      header: t('Official Sync'),
      cell: ({ row }) => {
        const syncOfficial = row.getValue('sync_official') as number
        return (
          <BadgeCell>
            <StatusBadge
              label={syncOfficial === 1 ? t('Official Sync') : t('No Sync')}
              variant={syncOfficial === 1 ? 'success' : 'warning'}
              size='sm'
              copyable={false}
            />
          </BadgeCell>
        )
      },
      filterFn: (row, id, value) => {
        if (!value || value.length === 0 || value.includes('all')) return true
        const syncOfficial = row.getValue(id) as number
        if (value.includes('yes')) return syncOfficial === 1
        if (value.includes('no')) return syncOfficial !== 1
        return false
      },
      size: 120,
      enableSorting: false,
    },

    // 创建时间列
    {
      accessorKey: 'created_time',
      meta: { label: t('Created'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Created')} />
      ),
      cell: ({ row }) => {
        const timestamp = row.getValue('created_time') as number
        return (
          <div className='min-w-[140px] font-mono text-sm'>
            {formatTimestampToDate(timestamp)}
          </div>
        )
      },
      size: 180,
    },

    // 更新时间列
    {
      accessorKey: 'updated_time',
      meta: { label: t('Updated'), mobileHidden: true },
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Updated')} />
      ),
      cell: ({ row }) => {
        const timestamp = row.getValue('updated_time') as number
        return (
          <div className='min-w-[140px] font-mono text-sm'>
            {formatTimestampToDate(timestamp)}
          </div>
        )
      },
      size: 180,
    },

    // 操作列
    {
      id: 'actions',
      cell: ({ row }) => {
        return <DataTableRowActions row={row} />
      },
      size: 100,
      enableSorting: false,
      enableHiding: false,
    },
  ]
}
