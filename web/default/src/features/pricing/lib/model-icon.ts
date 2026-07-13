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
import type { PricingModel } from '../types'

type PricingModelIconFields = Pick<PricingModel, 'icon' | 'vendor_icon'>

function normalizeIconKey(value: string | undefined | null): string | undefined {
  const trimmed = value?.trim()
  return trimmed ? trimmed : undefined
}

// 模型广场优先展示模型自身图标；模型未配置图标时回退到供应商图标，避免自定义模型品牌被供应商品牌覆盖。
export function getPricingModelIconKey(
  model: PricingModelIconFields
): string | undefined {
  return normalizeIconKey(model.icon) ?? normalizeIconKey(model.vendor_icon)
}
