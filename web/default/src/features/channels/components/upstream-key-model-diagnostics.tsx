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
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { UpstreamAccountKey } from '../types'
import { getUpstreamKeyModelSyncSourceLabel } from '../lib/upstream-sync'

type UpstreamKeyModelDiagnosticsProps = {
  accountKey: UpstreamAccountKey
  modelCount: number
  enabled: boolean
}

export function UpstreamKeyModelDiagnostics({
  accountKey,
  modelCount,
  enabled,
}: UpstreamKeyModelDiagnosticsProps) {
  const { t } = useTranslation()
  const sourceLabel = getUpstreamKeyModelSyncSourceLabel(
    accountKey.key_models_sync_source
  )
  const syncError = accountKey.key_models_sync_error?.trim() || ''
  const disabledReason =
    accountKey.disabled_reason?.trim() || accountKey.last_error?.trim() || ''
  const pendingRepair = modelCount === 0 && Boolean(syncError)

  if (!sourceLabel && !syncError && !disabledReason) {
    return null
  }

  return (
    <div className='flex min-w-0 flex-col gap-1'>
      {sourceLabel ? (
        <span className='text-muted-foreground truncate text-[11px]'>
          {t('Model source')}: {t(sourceLabel)}
        </span>
      ) : null}
      {pendingRepair ? (
        <span
          className='text-muted-foreground truncate text-[11px]'
          title={syncError}
        >
          {t('Imported, pending model repair')}
        </span>
      ) : null}
      {syncError ? (
        <span
          className={cn(
            'truncate text-[11px]',
            pendingRepair || !enabled
              ? 'text-muted-foreground'
              : 'text-destructive'
          )}
          title={syncError}
        >
          {t('Model sync error')}: {syncError}
        </span>
      ) : null}
      {disabledReason && disabledReason !== syncError ? (
        <span
          className='text-muted-foreground truncate text-[11px]'
          title={disabledReason}
        >
          {t('Disabled reason')}: {disabledReason}
        </span>
      ) : null}
    </div>
  )
}
