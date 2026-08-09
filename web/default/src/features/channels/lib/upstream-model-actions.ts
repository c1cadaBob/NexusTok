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
import { dedupeModelNames } from './model-search'

export type UpstreamModelFetchStatus = 'empty' | 'fetched'

export type UpstreamModelFetchResult = {
  status: UpstreamModelFetchStatus
  models: string[]
  count: number
}

// resolveUpstreamModelFetchResult 只整理“从上游获取”弹窗的数据结果。
// 它不判断当前表单是否已经选中这些模型，也不直接回填；管理员始终通过
// FetchModelsDialog 查看上游列表、调整勾选项，再显式保存到当前草稿。
export function resolveUpstreamModelFetchResult(
  sourceModels: readonly string[]
): UpstreamModelFetchResult {
  const models = dedupeModelNames(sourceModels)
  if (models.length === 0) {
    return { status: 'empty', models, count: 0 }
  }

  return { status: 'fetched', models, count: models.length }
}
