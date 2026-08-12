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
import type { UsageLog } from '../data/schema'
import { parseLogOther } from './format'
import { isDisplayableLogType } from './utils'

function validPositiveNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

function validNonNegativeNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

function legacyEffectiveGroupRatio(
  other: ReturnType<typeof parseLogOther>
): number | null {
  if (validPositiveNumber(other?.user_group_ratio)) {
    return other.user_group_ratio
  }
  if (other?.user_group_ratio != null && other.user_group_ratio !== -1) {
    return null
  }
  if (validPositiveNumber(other?.group_ratio)) {
    return other.group_ratio
  }
  if (other?.group_ratio != null) {
    return null
  }
  return 1
}

// getUpstreamCost 计算管理员成本列展示值。
//
// 新日志优先使用后端写入的 standard_billing_quota，确保下游 group_ratio /
// user_group_ratio 不会影响上游成本。旧日志没有精确字段时，才按最终费用除以
// 有效分组倍率做近似回退。
export function getUpstreamCost(log: UsageLog): number | null {
  if (!isDisplayableLogType(log.type)) return null

  const other = parseLogOther(log.other)
  const ratioConversion = other?.admin_info?.ratio_conversion
  if (!validPositiveNumber(ratioConversion)) {
    return null
  }

  const standardBillingQuota = other?.admin_info?.standard_billing_quota
  if (validNonNegativeNumber(standardBillingQuota)) {
    return standardBillingQuota * ratioConversion
  }

  const quota = Number(log.quota)
  if (!Number.isFinite(quota)) return null

  const groupRatio = legacyEffectiveGroupRatio(other)
  if (groupRatio == null) return null

  return (quota / groupRatio) * ratioConversion
}
