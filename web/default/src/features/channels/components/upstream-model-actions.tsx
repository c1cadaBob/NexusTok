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
import {
  useState,
  type ComponentProps,
  type MouseEvent,
  type PointerEvent,
  type ReactNode,
} from 'react'
import { Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
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

type RenderFetchButtonOptions = {
  className?: string
  disabled?: boolean
  title?: string
  variant?: ButtonVariant
  size?: ButtonSize
}

type FetchActionRenderProps = {
  renderButton: (options?: RenderFetchButtonOptions) => ReactNode
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
  children?: (props: FetchActionRenderProps) => ReactNode
}

export type UpstreamModelActionsProps = FetchActionProps

// UpstreamModelActions 统一渲染模型来源动作。
// 组件内部稳定挂载 FetchModelsDialog，按钮可以被渲染到 Combobox footer；
// 即使下拉框随后因焦点变化关闭，弹窗状态也不会因为 footer 卸载而丢失。
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

  const handleOpenFetchDialog = (event: MouseEvent<HTMLButtonElement>) => {
    event.preventDefault()
    event.stopPropagation()
    if (disabled) return
    if (props.onBeforeOpen && !props.onBeforeOpen()) return
    setFetchOpen(true)
  }

  const handleFetchButtonPointerDown = (
    event: PointerEvent<HTMLButtonElement>
  ) => {
    event.preventDefault()
    event.stopPropagation()
  }

  const renderButton = (options: RenderFetchButtonOptions = {}) => {
    const resolvedDisabled = options.disabled ?? disabled
    return (
      <Button
        type='button'
        variant={options.variant ?? variant}
        size={options.size ?? size}
        onPointerDown={handleFetchButtonPointerDown}
        onClick={handleOpenFetchDialog}
        disabled={resolvedDisabled}
        title={options.title ?? title}
        className={cn(buttonClassName, options.className)}
      >
        <Sparkles data-icon='inline-start' />
        {t('Fetch from Upstream')}
      </Button>
    )
  }

  const dialog = (
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
  )

  if (props.children) {
    return (
      <>
        {props.children({ renderButton })}
        {dialog}
      </>
    )
  }

  return (
    <div className={cn('flex flex-wrap gap-2', className)}>
      {renderButton()}
      {dialog}
    </div>
  )
}
