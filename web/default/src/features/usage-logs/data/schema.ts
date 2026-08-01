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

export interface UsageLog {
  id: number
  type: number
  user_id: number
  username?: string
  channel: number
  channel_name?: string
  token_id?: number
  token_name?: string
  model_name: string
  prompt_tokens: number
  completion_tokens: number
  quota: number
  content?: string
  request_id?: string
  upstream_request_id?: string
  use_time: number
  is_stream: boolean
  group?: string
  ip?: string
  other: string
  created_at?: number
  created_time?: number
  timestamp?: number
}
