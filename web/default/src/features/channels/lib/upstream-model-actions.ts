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

export type UpstreamModelApplyStatus = 'empty' | 'same' | 'applied'

export type UpstreamModelApplyResult = {
  status: UpstreamModelApplyStatus
  models: string[]
  count: number
}

function modelSetKey(models: readonly string[]) {
  return models
    .map((model) => model.trim().toLowerCase())
    .filter(Boolean)
    .sort()
    .join('\n')
}

// resolveUpstreamModelApplyResult 统一处理“使用上游返回模型”的回填语义。
// 同步预览、编辑抽屉和账号池都必须走这里，避免某个入口静默无效或错误禁用。
export function resolveUpstreamModelApplyResult(
  sourceModels: readonly string[],
  selectedModels: readonly string[]
): UpstreamModelApplyResult {
  const models = dedupeModelNames(sourceModels)
  if (models.length === 0) {
    return { status: 'empty', models: [], count: 0 }
  }

  if (modelSetKey(models) === modelSetKey(dedupeModelNames(selectedModels))) {
    return { status: 'same', models, count: models.length }
  }

  return { status: 'applied', models, count: models.length }
}
