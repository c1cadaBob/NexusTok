/*
Copyright (C) 2023-2026 c1cadaBob

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

For commercial licensing, please contact support@c1cadabob.dev
*/
import { Wrench01Icon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatLogQuota } from '@/lib/format'

import { hasToolSurcharge } from '../lib/format'
import type { LogOtherData } from '../types'

interface LogCostDisplayProps {
  quota: number
  other: LogOtherData | null
}

function splitQuotaDisplay(value: string): { prefix: string; amount: string } {
  const match = value.match(/^([^0-9+\-.,\s]+)(.+)$/)
  if (!match) return { prefix: '', amount: value }
  return { prefix: match[1], amount: match[2] }
}

function ToolSurchargeMarker() {
  const { t } = useTranslation()
  const label = t('Includes tool-call surcharge')

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Badge
            variant='warning'
            className='h-5 min-w-5 cursor-help gap-0 rounded-full px-1'
            role='img'
            aria-label={label}
            tabIndex={0}
            data-tool-surcharge-indicator='true'
          >
            <HugeiconsIcon
              icon={Wrench01Icon}
              strokeWidth={2}
              aria-hidden='true'
            />
            <span
              className='text-[9px] leading-none font-bold'
              aria-hidden='true'
            >
              +
            </span>
          </Badge>
        }
      />
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

function QuotaBadge(props: { quota: number }) {
  const quotaDisplay = splitQuotaDisplay(formatLogQuota(props.quota))

  return (
    <span className='border-border/80 bg-muted/60 inline-flex h-6 w-fit items-center rounded-md border px-2 [font-family:var(--font-body)] text-sm leading-none font-semibold tabular-nums'>
      {quotaDisplay.prefix ? (
        <span className='mr-1'>{quotaDisplay.prefix}</span>
      ) : null}
      <span>{quotaDisplay.amount}</span>
    </span>
  )
}

function SubscriptionBadge(props: { quota: number }) {
  const { t } = useTranslation()

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <StatusBadge
            label={t('Subscription')}
            variant='success'
            size='sm'
            copyable={false}
            className='cursor-help'
          />
        }
      />
      <TooltipContent>
        <span>
          {t('Deducted by subscription')}: {formatLogQuota(props.quota)}
        </span>
      </TooltipContent>
    </Tooltip>
  )
}

export function LogCostDisplay(props: LogCostDisplayProps) {
  const isSubscription = props.other?.billing_source === 'subscription'
  const showToolSurcharge = hasToolSurcharge(props.other)

  if (!isSubscription && !showToolSurcharge) {
    return (
      <div className='flex flex-col gap-0.5'>
        <QuotaBadge quota={props.quota} />
      </div>
    )
  }

  return (
    <TooltipProvider>
      <div className='inline-flex items-center gap-1'>
        {isSubscription ? (
          <SubscriptionBadge quota={props.quota} />
        ) : (
          <QuotaBadge quota={props.quota} />
        )}
        {showToolSurcharge ? <ToolSurchargeMarker /> : null}
      </div>
    </TooltipProvider>
  )
}

export function UpstreamCostDisplay(props: { other: LogOtherData | null }) {
  const { t } = useTranslation()
  const [fallbackTooltipOpen, setFallbackTooltipOpen] = useState(false)
  const adminInfo = props.other?.admin_info
  const cost = adminInfo?.upstream_cost
  const source = adminInfo?.upstream_cost_source

  if (
    typeof cost !== 'number' ||
    !Number.isFinite(cost) ||
    (source !== 'upstream' && source !== 'estimated')
  ) {
    return null
  }

  const isUpstream = source === 'upstream'
  const fallbackReason = isUpstream
    ? ''
    : getUpstreamCostFallbackReason(adminInfo?.upstream_cost_fallback_reason, t)
  const accessibilityLabel = isUpstream
    ? formatLogQuota(cost)
    : `${formatLogQuota(cost)}. ${fallbackReason}`
  const badge = (
    <Badge
      variant='outline'
      data-cost-source={source}
      data-cost-tone={isUpstream ? 'green' : 'yellow'}
      aria-label={accessibilityLabel}
      className={
        isUpstream
          ? 'h-auto min-h-6 max-w-full flex-wrap border-emerald-300 bg-emerald-50 px-2 py-1 text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-200'
          : 'h-auto min-h-6 max-w-full flex-wrap border-amber-300 bg-amber-50 px-2 py-1 text-amber-900 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-200'
      }
    >
      <span className='font-semibold tabular-nums'>{formatLogQuota(cost)}</span>
    </Badge>
  )

  if (isUpstream) return badge

  return (
    <TooltipProvider>
      <Tooltip open={fallbackTooltipOpen} onOpenChange={setFallbackTooltipOpen}>
        <TooltipTrigger
          render={
            <button
              type='button'
              onClick={() => setFallbackTooltipOpen(true)}
              aria-label={accessibilityLabel}
              aria-expanded={fallbackTooltipOpen}
              className='inline-flex cursor-help rounded-md border-0 bg-transparent p-0 text-inherit focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-amber-700'
              data-cost-fallback-trigger='true'
            >
              {badge}
            </button>
          }
        />
        <TooltipContent>
          <div className='space-y-1'>
            <div>{fallbackReason}</div>
            {adminInfo?.upstream_cost_fallback_status ? (
              <div>
                {t('HTTP status: {{status}}', {
                  status: adminInfo.upstream_cost_fallback_status,
                })}
              </div>
            ) : null}
            {adminInfo?.upstream_cost_fallback_code ? (
              <div>
                {t('Platform error code: {{code}}', {
                  code: adminInfo.upstream_cost_fallback_code,
                })}
              </div>
            ) : null}
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

function getUpstreamCostFallbackReason(
  reason: string | undefined,
  t: (key: string) => string
): string {
  switch (reason) {
    case 'request_error':
      return t('Cost lookup was skipped because the request failed')
    case 'authentication_failed':
      return t('Platform authentication failed')
    case 'permission_denied':
      return t('Platform denied access to usage logs')
    case 'http_request_failed':
      return t('Platform usage-log request failed')
    case 'empty_result':
      return t('No usage records were returned')
    case 'request_id_no_match':
      return t('No usage record matched the request ID')
    case 'model_or_time_no_match':
      return t('No usage record matched the model and time')
    case 'ambiguous_match':
      return t('Multiple usage records matched this request')
    case 'invalid_cost_record':
      return t('Usage record did not contain a valid cost')
    default:
      return t('Historical estimate; fallback reason was not recorded')
  }
}
