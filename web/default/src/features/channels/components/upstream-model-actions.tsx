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
import { useState, type ComponentProps } from 'react'
import { Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { resolveUpstreamModelApplyResult } from '../lib/upstream-model-actions'
import { FetchModelsDialog } from './dialogs/fetch-models-dialog'

type ButtonVariant = ComponentProps<typeof Button>['variant']
type ButtonSize = ComponentProps<typeof Button>['size']

type CommonActionProps = {
  className?: string
  buttonClassName?: string
  disabled?: boolean
  title?: string
  variant?: ButtonVariant
  size?: ButtonSize
}

type ApplySnapshotActionProps = CommonActionProps & {
  mode: 'applySnapshot'
  sourceModels: readonly string[]
  selectedModels: readonly string[]
  onApply: (models: string[]) => void
}

type FetchActionProps = CommonActionProps & {
  mode: 'fetch'
  onBeforeOpen?: () => boolean
  customFetcher: () => Promise<string[]>
  existingModelsOverride: string[]
  channelName?: string | null
  redirectModels?: string[]
  redirectSourceModels?: string[]
  onModelsSelected: (models: string[]) => void
  requireOperatePermission?: boolean
  requireWritePermission?: boolean
}

export type UpstreamModelActionsProps =
  | ApplySnapshotActionProps
  | FetchActionProps

// UpstreamModelActions 统一渲染模型来源动作。
// mode=fetch 复用完整的 FetchModelsDialog；mode=applySnapshot 只应用同步快照里的模型。
export function UpstreamModelActions(props: UpstreamModelActionsProps) {
  const { t } = useTranslation()
  const [fetchOpen, setFetchOpen] = useState(false)
  const {
    className,
    buttonClassName,
    disabled,
    title,
    variant = 'outline',
    size = 'sm',
  } = props

  if (props.mode === 'fetch') {
    const handleOpenFetchDialog = () => {
      if (disabled) return
      if (props.onBeforeOpen && !props.onBeforeOpen()) return
      setFetchOpen(true)
    }

    return (
      <div className={cn('flex flex-wrap gap-2', className)}>
        <Button
          type='button'
          variant={variant}
          size={size}
          onClick={handleOpenFetchDialog}
          disabled={disabled}
          title={title}
          className={buttonClassName}
        >
          <Sparkles data-icon='inline-start' />
          {t('Fetch from Upstream')}
        </Button>
        <FetchModelsDialog
          open={fetchOpen}
          onOpenChange={setFetchOpen}
          customFetcher={props.customFetcher}
          existingModelsOverride={props.existingModelsOverride}
          channelName={props.channelName}
          redirectModels={props.redirectModels}
          redirectSourceModels={props.redirectSourceModels}
          requireOperatePermission={props.requireOperatePermission}
          requireWritePermission={props.requireWritePermission}
          onModelsSelected={props.onModelsSelected}
        />
      </div>
    )
  }

  const handleApplySnapshotModels = () => {
    if (disabled) return

    const result = resolveUpstreamModelApplyResult(
      props.sourceModels,
      props.selectedModels
    )
    if (result.status === 'empty') {
      toast.info(t('No upstream models returned for this key'))
      return
    }
    if (result.status === 'same') {
      toast.info(t('Upstream models are already applied'))
      return
    }

    props.onApply(result.models)
    toast.success(
      t('Applied {{count}} upstream model(s)', { count: result.count })
    )
  }

  return (
    <div className={cn('flex flex-wrap gap-2', className)}>
      <Button
        type='button'
        variant={variant}
        size={size}
        onClick={handleApplySnapshotModels}
        disabled={disabled}
        title={title}
        className={buttonClassName}
      >
        <Sparkles data-icon='inline-start' />
        {t('Use Upstream Models')}
      </Button>
    </div>
  )
}
