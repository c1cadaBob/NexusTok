import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'

import type { RoutingKeyHealthSample } from '../types'

type RoutingKeyHealthTooltipProps = {
  children: ReactNode
  samples?: RoutingKeyHealthSample[]
  reason?: string
}

function getHealthReasonLabel(reason?: string): string | null {
  switch (reason) {
    case 'credential_unavailable':
      return 'Credential unavailable'
    case 'models_unavailable':
      return 'Model capability unavailable'
    case 'snapshot_only':
      return 'Historical snapshot'
    case 'missing':
      return 'Missing'
    case 'expired':
      return 'Expired'
    case 'quota_exhausted':
      return 'Quota exhausted'
    case 'upstream_disabled':
      return 'Upstream disabled'
    case 'manual_disabled':
      return 'Manually disabled'
    case 'site_sync_unavailable':
      return 'Platform snapshot unavailable'
    case 'not_enough_samples':
      return 'Not enough recent samples'
    case 'stale_samples':
      return 'No recent key usage'
    case 'no_health_record':
      return 'No health samples yet'
    case 'recent_failures':
      return 'Recent failures exceeded threshold'
    case 'recent_degraded':
      return 'Recent success rate degraded'
    case 'high_first_latency':
      return 'First response latency exceeded threshold'
    default:
      return null
  }
}

export function RoutingKeyHealthTooltip({
  children,
  samples,
  reason,
}: RoutingKeyHealthTooltipProps) {
  const { t, i18n } = useTranslation()
  const healthSamples = samples?.slice(-5).reverse()
  const reasonLabel = getHealthReasonLabel(reason)
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  if ((!healthSamples || healthSamples.length === 0) && !reasonLabel) {
    return children
  }

  const sampleKeys = new Map<string, number>()
  const keyedSamples = healthSamples?.map((sample) => {
    const identity = [
      sample.created_at,
      sample.success,
      sample.first_latency_ms,
      sample.error_summary ?? '',
    ].join('-')
    const occurrence = sampleKeys.get(identity) ?? 0
    sampleKeys.set(identity, occurrence + 1)
    return { sample, key: `${identity}-${occurrence}` }
  })

  return (
    <TooltipProvider delay={100}>
      <Tooltip>
        <TooltipTrigger render={<span className='inline-flex cursor-help' />}>
          {children}
        </TooltipTrigger>
        {keyedSamples && keyedSamples.length > 0 ? (
          <TooltipContent
            side='top'
            className='max-h-60 max-w-md flex-col items-start overflow-y-auto whitespace-normal'
          >
            {keyedSamples.map(({ sample, key }) => {
              let sampleStatus = 'Success'
              if (!sample.success) {
                sampleStatus = 'Failure'
              } else if (sample.first_latency_ms >= 10_000) {
                sampleStatus = 'High latency'
              }

              const timestamp =
                sample.created_at > 0
                  ? new Date(sample.created_at * 1000).toLocaleString(locale)
                  : '-'
              const latency = Number.isFinite(sample.first_latency_ms)
                ? sample.first_latency_ms.toLocaleString(locale)
                : '0'
              const errorSummary = sample.success
                ? '-'
                : sample.error_summary?.trim() || '-'

              return (
                <div key={key} className='w-full font-mono text-xs break-words'>
                  {timestamp} - {t(sampleStatus)} - {latency} {t('ms')} -{' '}
                  {errorSummary}
                </div>
              )
            })}
          </TooltipContent>
        ) : (
          reasonLabel && (
            <TooltipContent side='top'>{t(reasonLabel)}</TooltipContent>
          )
        )}
      </Tooltip>
    </TooltipProvider>
  )
}
