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

function normalizeModelSearchKey(model: string): string {
  return model.trim().toLowerCase()
}

type ModelSearchItemLike = {
  model_name?: string | null
}

// 从模型搜索接口返回项中提取可用于渠道模型选择器的真实模型名。
// /api/models/search 会匹配 description/tags；渠道模型补齐只应该使用
// model_name 本身包含关键词的条目，避免把标签命中的无关模型加入渠道。
export function getModelSearchModelNames(
  searchItems: readonly ModelSearchItemLike[],
  keyword: string
): string[] {
  const normalizedKeyword = keyword.trim().toLowerCase()
  const seenKeys = new Set<string>()
  const names: string[] = []

  for (const item of searchItems) {
    const name = item.model_name?.trim()
    if (!name) continue

    const key = normalizeModelSearchKey(name)
    if (seenKeys.has(key)) continue
    if (normalizedKeyword && !key.includes(normalizedKeyword)) continue

    seenKeys.add(key)
    names.push(name)
  }

  return names
}

// 计算当前模型库搜索命中里还没有加入渠道的模型。
// 该函数只负责前端草稿层面的去重：同一搜索结果大小写不同只保留首次出现，
// 已经存在于渠道模型列表中的项不会再次返回，避免批量追加时重复写入。
export function getMissingModelSearchMatches(
  searchMatches: readonly string[],
  currentModels: readonly string[]
): string[] {
  const currentModelKeys = new Set(
    currentModels.map(normalizeModelSearchKey).filter(Boolean)
  )
  const seenSearchKeys = new Set<string>()
  const missingModels: string[] = []

  for (const rawModel of searchMatches) {
    const model = rawModel.trim()
    const modelKey = normalizeModelSearchKey(model)
    if (!modelKey || seenSearchKeys.has(modelKey)) continue
    seenSearchKeys.add(modelKey)
    if (currentModelKeys.has(modelKey)) continue
    missingModels.push(model)
  }

  return missingModels
}
