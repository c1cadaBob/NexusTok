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
import { createFileRoute, redirect } from '@tanstack/react-router'
import { ModelSettings } from '@/features/system-settings/models'
import {
  MODELS_DEFAULT_SECTION,
  MODELS_SECTION_IDS,
} from '@/features/system-settings/models/section-registry.tsx'

export const Route = createFileRoute(
  '/_authenticated/system-settings/models/$section'
)({
  beforeLoad: ({ params }) => {
    // 兼容旧版模型倍率入口：模型价格已经统一迁移到“模型 / 同步源模型”，
    // 历史 section 不能再落到分组与工具价格页，否则管理员会找不到模型定价入口。
    if (params.section === 'model-ratio') {
      throw redirect({
        to: '/models/$section',
        params: { section: 'metadata' },
      })
    }

    const validSections = MODELS_SECTION_IDS as unknown as string[]
    if (!validSections.includes(params.section)) {
      throw redirect({
        to: '/system-settings/models/$section',
        params: { section: MODELS_DEFAULT_SECTION },
      })
    }
  },
  component: ModelSettings,
})
