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
import type {
  SystemInstanceListResponse,
  SystemTaskResponse,
  SystemTaskListResponse,
} from './types'

export async function listSystemInstances() {
  const res = await api.get<SystemInstanceListResponse>(
    '/api/system-info/instances'
  )
  return res.data
}

export async function listSystemTasks(limit = 20) {
  const res = await api.get<SystemTaskListResponse>('/api/system-task/list', {
    params: { limit },
  })
  return res.data
}

export async function getCurrentSystemTask(type: string) {
  const res = await api.get<SystemTaskResponse>('/api/system-task/current', {
    params: { type },
    disableDuplicate: true,
  } as Record<string, unknown>)
  return res.data
}

export async function getSystemTask(taskId: string) {
  const res = await api.get<SystemTaskResponse>(`/api/system-task/${taskId}`, {
    disableDuplicate: true,
  } as Record<string, unknown>)
  return res.data
}
