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

export function normalizeModelSearchKey(model: string): string {
  return model.trim().toLowerCase()
}

// 渠道模型列表在前端草稿中统一按 trim + lower 去重，并保留首次出现的展示形式。
// 这样搜索追加、自定义输入、预设分组和手动选择不会因为大小写差异生成重复模型。
export function dedupeModelNames(models: readonly string[]): string[] {
  const seenKeys = new Set<string>()
  const dedupedModels: string[] = []

  for (const rawModel of models) {
    const model = rawModel.trim()
    const key = normalizeModelSearchKey(model)
    if (!key || seenKeys.has(key)) continue
    seenKeys.add(key)
    dedupedModels.push(model)
  }

  return dedupedModels
}

export function mergeModelNames(
  existingModels: readonly string[],
  incomingModels: readonly string[]
): string[] {
  return dedupeModelNames([...existingModels, ...incomingModels])
}

const MODEL_DRAFT_SEPARATOR_REGEX = /[,，\n]+/

type ModelSearchItemLike = {
  model_name?: string | null
}

export type ModelSearchAppendPlan = {
  missingModels: string[]
  previewModels: string[]
  omittedCount: number
  totalCount: number
}

export type ModelSearchAppendSummary = {
  matchedCount: number
  addableCount: number
  existingCount: number
}

// 解析管理员在“自定义模型”输入框中录入的模型列表。
// 支持英文逗号、中文逗号和换行，按大小写不敏感去重并保留首次录入的展示形式。
export function parseModelDraftList(value: string): string[] {
  const seenKeys = new Set<string>()
  const models: string[] = []

  for (const rawModel of value.split(MODEL_DRAFT_SEPARATOR_REGEX)) {
    const model = rawModel.trim()
    const key = normalizeModelSearchKey(model)
    if (!key || seenKeys.has(key)) continue
    seenKeys.add(key)
    models.push(model)
  }

  return models
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

// 生成渠道编辑页“搜索后批量追加”操作所需的展示计划。
// 该函数只处理已经提取出的真实模型名，不再读取 description/tags 命中的条目，
// 保证按钮文案、预览列表和最终追加数量使用同一套去重规则。
export function buildModelSearchAppendPlan(
  searchMatches: readonly string[],
  currentModels: readonly string[],
  previewLimit = 6
): ModelSearchAppendPlan {
  const missingModels = getMissingModelSearchMatches(
    searchMatches,
    currentModels
  )
  const normalizedPreviewLimit = Math.max(0, previewLimit)
  const previewModels = missingModels.slice(0, normalizedPreviewLimit)

  return {
    missingModels,
    previewModels,
    omittedCount: missingModels.length - previewModels.length,
    totalCount: missingModels.length,
  }
}

// 汇总搜索结果与当前渠道模型的关系，供编辑页明确展示“命中/可新增/已存在”。
// 该统计只描述当前已加载的真实模型名；点击追加时仍会重新按最新表单值计算缺失项。
export function buildModelSearchAppendSummary(
  searchMatches: readonly string[],
  currentModels: readonly string[]
): ModelSearchAppendSummary {
  const uniqueMatches = getMissingModelSearchMatches(searchMatches, [])
  const addableMatches = getMissingModelSearchMatches(
    uniqueMatches,
    currentModels
  )

  return {
    matchedCount: uniqueMatches.length,
    addableCount: addableMatches.length,
    existingCount: uniqueMatches.length - addableMatches.length,
  }
}
