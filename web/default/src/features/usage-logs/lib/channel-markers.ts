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

interface ChannelMarkerAdminInfo {
  is_multi_key?: unknown
  multi_key_index?: unknown
  use_channel?: unknown
  account_pool?: unknown
  channel_account_id?: unknown
  channel_account_name?: unknown
  pool_account_id?: unknown
  pool_account_name?: unknown
}

export interface UsageLogChannelMarkers {
  retryChannels: string[]
  retryChain?: string
  hasRetryChain: boolean
  multiKeyIndex?: number
  channelAccount?: {
    id: string
    name?: string
  }
}

function normalizeRetryChannels(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value
    .map((item) => String(item ?? '').trim())
    .filter((item) => item.length > 0)
}

function normalizeMultiKeyIndex(
  info: ChannelMarkerAdminInfo
): number | undefined {
  if (info.is_multi_key !== true) return undefined
  if (typeof info.multi_key_index !== 'number') return undefined
  if (!Number.isFinite(info.multi_key_index)) return undefined
  return info.multi_key_index
}

function normalizeChannelAccount(
  info: ChannelMarkerAdminInfo
): UsageLogChannelMarkers['channelAccount'] {
  const rawId = info.channel_account_id ?? info.pool_account_id
  const id = String(rawId ?? '').trim()
  if (!id || id === '0' || id === '<nil>') return undefined

  const rawName = info.channel_account_name ?? info.pool_account_name
  const name = String(rawName ?? '').trim()

  return {
    id,
    name: name.length > 0 && name !== '<nil>' ? name : undefined,
  }
}

/**
 * 从日志 admin_info 中提取渠道列的可视化标记。
 *
 * 后端写入的 use_channel 可能只包含当前命中渠道，也可能包含多次重试链路。
 * 只有两个及以上有效渠道值才视为 Retry Chain，避免在普通单渠道日志上制造误导。
 * multi_key_index 仅在后端明确标记 is_multi_key=true 且序号为有限数字时展示。
 * 账号池命中信息只取后端写入的账号 ID 和名称，不读取或展示任何完整 key。
 */
export function getUsageLogChannelMarkers(
  info: ChannelMarkerAdminInfo | undefined | null
): UsageLogChannelMarkers {
  if (!info) {
    return {
      retryChannels: [],
      hasRetryChain: false,
    }
  }

  const retryChannels = normalizeRetryChannels(info.use_channel)
  const hasRetryChain = retryChannels.length > 1

  return {
    retryChannels,
    retryChain: hasRetryChain ? retryChannels.join(' → ') : undefined,
    hasRetryChain,
    multiKeyIndex: normalizeMultiKeyIndex(info),
    channelAccount: normalizeChannelAccount(info),
  }
}
