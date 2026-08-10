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
import { api } from '@/lib/api'
import type { UsageLog } from '@/features/usage-logs/data/schema'

export type AuditLogsParams = {
  p?: number
  page_size?: number
  start_timestamp?: number
  end_timestamp?: number
  username?: string
  request_id?: string
}

export type AuditLogsResponse = {
  success: boolean
  message?: string
  data?: {
    items: UsageLog[]
    total: number
    page: number
    page_size: number
  }
}

// getAuditLogs 查询管理员操作和成功登录审计记录。
//
// 服务端会固定日志类型和权限范围；此处只负责把页面筛选条件编码为请求参数。
export async function getAuditLogs(
  params: AuditLogsParams
): Promise<AuditLogsResponse> {
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== '') {
      query.set(key, String(value))
    }
  }
  const res = await api.get(`/api/audit-log/?${query.toString()}`)
  return res.data
}
