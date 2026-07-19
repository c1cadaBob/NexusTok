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
import type { ChannelAccount, UpstreamAccountKey } from '../types'

type RatioDisplaySource = {
  ratio_conversion?: number | null
  effective_ratio?: number | null
  group_ratio?: number | null
}

export function formatUpstreamRatioCompact(value: number | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  if (Number.isInteger(value)) return value.toString()
  return value.toFixed(8).replace(/\.?0+$/, '')
}

export function getUpstreamRatioDisplayValue(
  value: RatioDisplaySource | null | undefined
): number | undefined {
  if (!value) return undefined
  const candidates = [
    value.ratio_conversion,
    value.effective_ratio,
    value.group_ratio,
  ]
  return candidates.find(
    (candidate) => candidate != null && Number.isFinite(candidate)
  ) as number | undefined
}

export function formatUpstreamModelRatioDetails(
  ratios?: Record<string, number>
): string {
  if (!ratios) return ''
  return Object.entries(ratios)
    .filter(([modelName, ratio]) => modelName.trim() && Number.isFinite(ratio))
    .sort(([left], [right]) => left.localeCompare(right))
    .map(
      ([modelName, ratio]) =>
        `${modelName}: ${formatUpstreamRatioCompact(ratio)}x`
    )
    .join('\n')
}

export function getUpstreamKeyGroupLabel(
  value: Pick<
    ChannelAccount,
    'key_group_name' | 'key_group_id' | 'group'
  > | Pick<UpstreamAccountKey, 'group_name' | 'group_id'>
): string {
  if ('group' in value) {
    return value.key_group_name || value.key_group_id || value.group || ''
  }
  return value.group_name || value.group_id || ''
}
