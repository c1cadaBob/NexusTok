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
