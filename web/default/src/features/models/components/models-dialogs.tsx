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
import { useModelPermissions } from '../hooks/use-model-permissions'
import { DescriptionDialog } from './dialogs/description-dialog'
import { MissingModelsDialog } from './dialogs/missing-models-dialog'
import { PrefillGroupManagement } from './dialogs/prefill-group-management'
import { SyncWizardDialog } from './dialogs/sync-wizard-dialog'
import { UpstreamConflictDialog } from './dialogs/upstream-conflict-dialog'
import { VendorMutateDialog } from './dialogs/vendor-mutate-dialog'
import { ModelMutateDrawer } from './drawers/model-mutate-drawer'
import { useModels } from './models-provider'

export function ModelsDialogs() {
  const { t } = useTranslation()
  const {
    open,
    setOpen,
    currentRow,
    currentVendor,
    descriptionData,
    setDescriptionData,
  } = useModels()
  const permissions = useModelPermissions()

  return (
    <>
      {/* 模型创建/更新抽屉 */}
      <ModelMutateDrawer
        open={open === 'create-model' || open === 'update-model'}
        onOpenChange={(v) => !v && setOpen(null)}
        currentRow={currentRow}
      />

      {/* 厂商创建/更新弹窗 */}
      <VendorMutateDialog
        open={open === 'create-vendor' || open === 'update-vendor'}
        onOpenChange={(v) => !v && setOpen(null)}
        currentVendor={open === 'update-vendor' ? currentVendor : null}
      />

      {/* 缺失模型弹窗 */}
      <MissingModelsDialog
        open={open === 'missing-models'}
        onOpenChange={(v) => !v && setOpen(null)}
      />

      {/* 上游同步向导弹窗 */}
      <SyncWizardDialog
        open={open === 'sync-wizard'}
        onOpenChange={(v) => !v && setOpen(null)}
      />

      {/* 上游冲突处理弹窗 */}
      <UpstreamConflictDialog
        open={open === 'upstream-conflict'}
        onOpenChange={(v) => !v && setOpen(null)}
      />

      {/* 预填组管理弹窗 */}
      <PrefillGroupManagement
        open={open === 'prefill-groups'}
        onOpenChange={(v) => !v && setOpen(null)}
        canWrite={permissions.canWrite}
        canSensitiveWrite={permissions.canSensitiveWrite}
        disabledReason={t("You don't have necessary permission")}
      />

      {/* 模型描述弹窗 */}
      <DescriptionDialog
        open={open === 'description'}
        onOpenChange={(v) => {
          if (!v) {
            setOpen(null)
            setDescriptionData(null)
          }
        }}
        modelName={descriptionData?.modelName || ''}
        description={descriptionData?.description || ''}
      />
    </>
  )
}
