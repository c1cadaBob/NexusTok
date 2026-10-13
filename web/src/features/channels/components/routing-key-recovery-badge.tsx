import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatTimestampToDate } from '@/lib/format'

type RoutingKeyRecoveryBadgeProps = {
  recoveryPhase?: 'probing' | 'confirming'
  recoveryNextTestAt?: number
  recoveryConfirmationsDone?: number
  recoveryConfirmationsTotal?: number
}

export function RoutingKeyRecoveryBadge(
  props: RoutingKeyRecoveryBadgeProps
) {
  const { t } = useTranslation()
  if (!props.recoveryPhase) {
    return null
  }

  const total = props.recoveryConfirmationsTotal || 5
  const done = props.recoveryConfirmationsDone || 0
  const label =
    props.recoveryPhase === 'confirming'
      ? t('Recovery confirmation {{done}}/{{total}}', { done, total })
      : t('Recovery test')
  const nextTest = props.recoveryNextTestAt
    ? formatTimestampToDate(props.recoveryNextTestAt)
    : t('Waiting for recovery test')

  return (
    <TooltipProvider delay={100}>
      <Tooltip>
        <TooltipTrigger
          render={
            <StatusBadge
              label={label}
              variant='warning'
              size='sm'
              copyable={false}
            />
          }
        />
        <TooltipContent side='top'>
          {t('Next recovery test: {{time}}', { time: nextTest })}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
