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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { toast } from 'sonner'
import { updateSystemOption } from '../api'
import type { UpdateOptionRequest } from '../types'
import { useSystemSettingPermissions } from './use-system-setting-permissions'

// 这些配置会影响公共状态、导航或计费展示，保存成功后需要同步刷新 status。
const STATUS_RELATED_KEYS = [
  'theme.frontend',
  'HeaderNavModules',
  'SidebarModulesAdmin',
  'Notice',
  'LogConsumeEnabled',
  'QuotaPerUnit',
  'USDExchangeRate',
  'DisplayInCurrencyEnabled',
  'DisplayTokenStatEnabled',
  'general_setting.quota_display_type',
  'general_setting.custom_currency_symbol',
  'general_setting.custom_currency_exchange_rate',
]

export function useUpdateOption() {
  const queryClient = useQueryClient()
  const permissions = useSystemSettingPermissions()
  const disabledReason = i18next.t("You don't have necessary permission")

  const mutation = useMutation({
    mutationFn: (request: UpdateOptionRequest) => {
      if (!permissions.canSensitiveWrite) {
        throw new Error(disabledReason)
      }
      return updateSystemOption(request)
    },
    onSuccess: (data, variables) => {
      if (data.success) {
        // 系统 option 是多个设置页的共享数据源，任意保存成功后都需要刷新。
        queryClient.invalidateQueries({ queryKey: ['system-options'] })

        // 前台展示相关配置还会影响全局 status，避免导航、品牌和计费展示滞后。
        if (STATUS_RELATED_KEYS.includes(variables.key)) {
          queryClient.invalidateQueries({ queryKey: ['status'] })
        }

        toast.success(i18next.t('Setting updated successfully'))
      } else {
        toast.error(data.message || i18next.t('Failed to update setting'))
      }
    },
    onError: (error: Error) => {
      toast.error(error.message || i18next.t('Failed to update setting'))
    },
  })

  return Object.assign(mutation, {
    canUpdate: permissions.canSensitiveWrite,
    disabledReason,
  })
}
