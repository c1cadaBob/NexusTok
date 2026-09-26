import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  CircleAlert,
  ExternalLink,
  KeyRound,
  RefreshCw,
  Server,
  ShieldAlert,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import {
  StaticDataTable,
  type StaticDataTableColumn,
} from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  getPlatformSiteResources,
  syncPlatformSiteResources,
} from '../api'
import type {
  PlatformSiteResourceEndpointCapability,
  PlatformSiteResourceGroup,
  PlatformSiteResourceSync,
  UpstreamKey,
} from '../types'

type PlatformSiteResourcesPanelProps = {
  channelId: number
}

function formatQuota(value: number): string {
  return Number.isFinite(value) ? value.toLocaleString() : '-'
}

function formatTimestamp(value: number | undefined): string {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

function statusVariant(
  status: string
): 'success' | 'warning' | 'danger' | 'info' | 'neutral' {
  if (status === 'success' || status === 'authenticated') return 'success'
  if (status === 'partial' || status === 'stale') return 'warning'
  if (
    status === 'secure_verification_required' ||
    status === 'reauth_required' ||
    status === 'rate_limited'
  ) {
    return 'danger'
  }
  if (status === 'running') return 'info'
  return 'neutral'
}

function keyStatusLabel(key: UpstreamKey, t: (key: string) => string): string {
  if (key.routable) return t('Routable')
  if (key.availability_reason === 'models_unavailable') {
    return t('Models unavailable')
  }
  if (key.availability_reason === 'credential_unavailable') {
    return t('Credential unavailable')
  }
  if (key.availability_reason === 'expired') return t('Expired')
  if (key.availability_reason === 'missing') return t('Missing')
  return key.disabled_reason || t('Unavailable')
}

function ResourceSyncList(props: {
  syncs: PlatformSiteResourceSync[]
}) {
  const { t } = useTranslation()
  if (props.syncs.length === 0) return null

  return (
    <div className='grid gap-2'>
      <h4 className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
        {t('Resource synchronization')}
      </h4>
      <div className='grid gap-2'>
        {props.syncs.map((sync) => (
          <div
            key={sync.resource_type}
            className='border-border/60 flex flex-wrap items-center gap-2 rounded-md border p-2 text-xs'
          >
            <span className='font-medium'>{sync.resource_type}</span>
            <StatusBadge
              label={sync.status}
              variant={statusVariant(sync.status)}
              copyable={false}
            />
            {sync.partial && (
              <span className='text-warning'>{t('Partial')}</span>
            )}
            {sync.using_snapshot && (
              <span className='text-muted-foreground'>
                {t('Using last successful snapshot')}
              </span>
            )}
            {sync.failure_reason && (
              <span className='text-destructive basis-full'>
                {sync.failure_reason}
              </span>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

function GroupTable(props: { groups: PlatformSiteResourceGroup[] }) {
  const { t } = useTranslation()
  const columns: StaticDataTableColumn<PlatformSiteResourceGroup>[] = [
    {
      id: 'name',
      header: t('Group'),
      cell: (group) => (
        <div className='min-w-0'>
          <div className='truncate font-medium'>{group.name}</div>
          <div className='text-muted-foreground truncate text-xs'>
            {group.external_id}
          </div>
        </div>
      ),
    },
    {
      id: 'ratio',
      header: t('Ratio'),
      cell: (group) => group.ratio,
    },
    {
      id: 'status',
      header: t('Status'),
      cell: (group) => (
        <StatusBadge
          label={group.usable ? t('Usable') : t('Unavailable')}
          variant={group.usable ? 'success' : 'neutral'}
          copyable={false}
        />
      ),
    },
    {
      id: 'source',
      header: t('Source'),
      cell: (group) => group.source_endpoint,
    },
  ]

  return (
    <StaticDataTable
      columns={columns}
      data={props.groups}
      getRowKey={(group) => group.external_id}
      emptyContent={t('No groups found')}
    />
  )
}

function KeyTable(props: { keys: UpstreamKey[] }) {
  const { t } = useTranslation()
  const columns: StaticDataTableColumn<UpstreamKey>[] = [
    {
      id: 'name',
      header: t('Key'),
      cell: (key) => (
        <div className='min-w-0'>
          <div className='truncate font-medium'>{key.name || '-'}</div>
          <div className='text-muted-foreground truncate text-xs'>
            {key.key_preview || key.external_id}
          </div>
        </div>
      ),
    },
    {
      id: 'models',
      header: t('Models'),
      cell: (key) => (
        <span className='text-muted-foreground text-xs'>
          {key.models.length > 0 ? key.models.join(', ') : t('None')}
        </span>
      ),
    },
    {
      id: 'ratio',
      header: t('Ratio'),
      cell: (key) => key.conversion_ratio,
    },
    {
      id: 'weight',
      header: t('Weight'),
      cell: (key) => key.weight,
    },
    {
      id: 'quota',
      header: t('Quota'),
      cell: (key) => (
        <div className='text-xs'>
          <div>{formatQuota(key.used_quota ?? 0)}</div>
          <div className='text-muted-foreground'>
            {key.remain_quota == null
              ? t('Unlimited')
              : formatQuota(key.remain_quota)}
          </div>
        </div>
      ),
    },
    {
      id: 'expires',
      header: t('Expires'),
      cell: (key) =>
        key.expires_at ? new Date(key.expires_at).toLocaleString() : '-',
    },
    {
      id: 'status',
      header: t('Status'),
      cell: (key) => (
        <StatusBadge
          label={keyStatusLabel(key, t)}
          variant={key.routable ? 'success' : 'warning'}
          copyable={false}
        />
      ),
    },
  ]

  return (
    <StaticDataTable
      columns={columns}
      data={props.keys}
      getRowKey={(key) => key.id}
      emptyContent={t('No upstream keys found')}
    />
  )
}

function EndpointCapabilityTable(props: {
  capabilities: PlatformSiteResourceEndpointCapability[]
}) {
  const { t } = useTranslation()
  const columns: StaticDataTableColumn<PlatformSiteResourceEndpointCapability>[] =
    [
      {
        id: 'protocol',
        header: t('Protocol'),
        cell: (capability) => capability.protocol || '-',
      },
      {
        id: 'method',
        header: t('HTTP method'),
        cell: (capability) => capability.http_method || '-',
      },
      {
        id: 'path',
        header: t('Path'),
        cell: (capability) => (
          <span className='break-all'>{capability.path || '-'}</span>
        ),
      },
      {
        id: 'supported',
        header: t('Supported'),
        cell: (capability) => (
          <StatusBadge
            label={
              capability.supported ? t('Supported') : t('Unsupported')
            }
            variant={capability.supported ? 'success' : 'neutral'}
            copyable={false}
          />
        ),
      },
      {
        id: 'source',
        header: t('Source data'),
        cell: (capability) => (
          <span className='break-all text-xs'>
            {capability.source_data || '-'}
          </span>
        ),
      },
    ]

  return (
    <StaticDataTable
      columns={columns}
      data={props.capabilities}
      getRowKey={(capability) =>
        `${capability.protocol}:${capability.http_method}:${capability.path}`
      }
      emptyContent={t('No endpoint capabilities')}
    />
  )
}

export function PlatformSiteResourcesPanel(
  props: PlatformSiteResourcesPanelProps
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const resourcesQuery = useQuery({
    queryKey: ['platform-site-resources', props.channelId],
    queryFn: async () =>
      requireServerSuccess(await getPlatformSiteResources(props.channelId)),
  })
  const syncMutation = useMutation({
    mutationFn: () => syncPlatformSiteResources(props.channelId),
    onSuccess: async (response) => {
      if (!response.success) {
        handleServerError(response, t('Failed to synchronize platform resources'))
        return
      }
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['platform-site-resources', props.channelId],
        }),
        queryClient.invalidateQueries({
          queryKey: ['upstream-site-status', props.channelId],
        }),
      ])
      toast.success(t('Platform resources synchronized'))
    },
    onError: (error: unknown) => {
      handleServerError(error, t('Failed to synchronize platform resources'))
    },
  })

  if (resourcesQuery.isLoading) {
    return <LoadingState className='min-h-0 py-8' />
  }
  if (resourcesQuery.isError) {
    return (
      <ErrorState
        className='min-h-0 py-8'
        description={t('Platform resources could not be loaded.')}
        onRetry={() => void resourcesQuery.refetch()}
      />
    )
  }

  const resources = resourcesQuery.data?.data
  if (!resources) {
    return <EmptyState className='min-h-0 py-8' title={t('No platform resources')} />
  }

  return (
    <section className='border-border/60 bg-muted/10 grid gap-4 rounded-lg border p-4'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='flex min-w-0 items-center gap-2'>
          <Server className='text-muted-foreground size-4' aria-hidden='true' />
          <div className='min-w-0'>
            <h3 className='font-medium'>{t('Platform resources')}</h3>
            <p className='text-muted-foreground text-xs'>
              {t('Last successful sync')}: {formatTimestamp(resources.last_sync_at)}
            </p>
          </div>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => syncMutation.mutate()}
          disabled={syncMutation.isPending}
        >
          <RefreshCw
            className={`size-4 ${syncMutation.isPending ? 'animate-spin' : ''}`}
            aria-hidden='true'
          />
          {t('Sync resources')}
        </Button>
      </div>

      <div className='grid gap-3 text-sm sm:grid-cols-2'>
        <div>
          <span className='text-muted-foreground'>{t('Management URL')}</span>
          <p className='break-all'>{resources.management_base_url || '-'}</p>
        </div>
        <div>
          <span className='text-muted-foreground'>{t('Relay URL')}</span>
          <p className='break-all'>{resources.relay_base_url || '-'}</p>
        </div>
      </div>

      <div className='grid gap-3 sm:grid-cols-4'>
        <div>
          <span className='text-muted-foreground text-xs'>
            {t('Balance')}
            {resources.identity?.quota_unit
              ? ` (${resources.identity.quota_unit})`
              : ''}
          </span>
          <p className='font-medium'>{formatQuota(resources.balance)}</p>
        </div>
        <div>
          <span className='text-muted-foreground text-xs'>
            {t('Used quota')}
            {resources.identity?.quota_unit
              ? ` (${resources.identity.quota_unit})`
              : ''}
          </span>
          <p className='font-medium'>{formatQuota(resources.used_quota)}</p>
        </div>
        <div>
          <span className='text-muted-foreground text-xs'>{t('Keys')}</span>
          <p className='font-medium'>
            {resources.routable_key_count} / {resources.key_count}
          </p>
        </div>
        <div>
          <span className='text-muted-foreground text-xs'>{t('Sync status')}</span>
          <StatusBadge
            label={resources.sync_status}
            variant={statusVariant(resources.sync_status)}
            copyable={false}
          />
        </div>
      </div>

      {resources.auth_status &&
        resources.auth_status !== 'authenticated' && (
          <div className='border-warning/40 bg-warning/10 flex gap-2 rounded-md border p-3 text-sm'>
            <ShieldAlert className='text-warning size-4 shrink-0' aria-hidden='true' />
            <div>
              <p className='font-medium'>{resources.auth_status}</p>
              <p className='text-muted-foreground text-xs'>
                {resources.auth_status_reason || t('Reauthentication is required.')}
              </p>
            </div>
          </div>
        )}

      {resources.identity && (
        <div className='grid gap-2'>
          <h4 className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
            {t('Platform identity')}
          </h4>
          <div className='grid gap-2 text-sm sm:grid-cols-3'>
            <span>{resources.identity.display_name || resources.identity.username || '-'}</span>
            <span>{resources.identity.email || resources.identity.username || '-'}</span>
            <span>
              {resources.identity.current_group || '-'}{' '}
              {resources.identity.quota_unit
                ? `(${resources.identity.quota_unit})`
                : ''}
            </span>
          </div>
        </div>
      )}

      <ResourceSyncList syncs={resources.resource_syncs} />

      <div className='grid gap-2'>
        <h4 className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
          {t('Groups and ratios')}
        </h4>
        {resources.groups.length > 0 ? (
          <GroupTable groups={resources.groups} />
        ) : (
          <EmptyState className='min-h-0 py-4' title={t('No groups found')} />
        )}
      </div>

      <div className='grid gap-2'>
        <div className='flex items-center gap-2'>
          <KeyRound className='text-muted-foreground size-4' aria-hidden='true' />
          <h4 className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
            {t('Upstream keys')}
          </h4>
        </div>
        <KeyTable keys={resources.keys} />
      </div>

      {resources.endpoint && (
        <div className='grid gap-2'>
          <h4 className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
            {t('Discovered endpoints')}
          </h4>
          <div className='grid gap-2 text-xs sm:grid-cols-2'>
            {[
              [t('Management'), resources.endpoint.management_url],
              [t('Relay'), resources.endpoint.relay_url],
              [t('Models'), resources.endpoint.models_url],
              [t('Pricing'), resources.endpoint.pricing_url],
              [t('Usage'), resources.endpoint.usage_url],
              [t('Token'), resources.endpoint.token_url],
              [t('Admin'), resources.endpoint.admin_url],
            ].map(([label, value]) => (
              <div key={label} className='flex min-w-0 items-start gap-2'>
                <ExternalLink className='text-muted-foreground mt-0.5 size-3 shrink-0' aria-hidden='true' />
                <span className='text-muted-foreground'>{label}:</span>
                <span className='min-w-0 break-all'>{value || '-'}</span>
              </div>
            ))}
          </div>
          {resources.endpoint.capabilities.length > 0 && (
            <div className='grid gap-2'>
              <div className='text-muted-foreground text-xs'>
                {t('Endpoint capabilities')}: {resources.endpoint.capabilities.length}
              </div>
              <EndpointCapabilityTable
                capabilities={resources.endpoint.capabilities}
              />
            </div>
          )}
        </div>
      )}

      {resources.using_last_snapshot && (
        <div className='text-muted-foreground flex items-center gap-2 text-xs'>
          <CircleAlert className='size-3.5' aria-hidden='true' />
          {t('Some resources use the last successful snapshot.')}
        </div>
      )}
    </section>
  )
}
