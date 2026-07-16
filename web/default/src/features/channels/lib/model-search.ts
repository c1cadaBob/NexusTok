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
// 这样自定义输入、预设分组、模型映射补齐和手动选择不会因为大小写差异生成重复模型。
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
  description?: string | null
  tags?: string | null
  name_rule?: number | null
  matched_models?: string[] | null
  matched_count?: number | null
}

export type ModelSearchModelNameResult = {
  names: string[]
  unresolvedMatchedCount: number
}

const CHANNEL_TYPE_MODEL_SEARCH_VENDORS: Record<number, string> = {
  1: 'OpenAI',
  3: 'Azure',
  14: 'Anthropic',
  16: 'Zhipu AI',
  17: 'Alibaba',
  23: 'tencent',
  24: 'Google',
  25: 'Moonshot AI',
  26: 'Zhipu AI',
  27: 'Perplexity',
  34: 'Cohere',
  35: 'MiniMax',
  41: 'Google',
  42: 'Mistral',
  43: 'DeepSeek',
  48: 'xAI',
  55: 'OpenAI',
  56: 'Replicate',
  57: 'OpenAI',
}

// 将渠道类型收窄到模型元数据里的默认供应商。
// Custom、Advanced Custom、OpenAI-compatible 聚合网关等渠道可能代理任意上游，
// 因此没有明确供应商时返回空字符串，继续使用全局模型库搜索。
export function getModelSearchVendorForChannelType(
  channelType: number
): string {
  return CHANNEL_TYPE_MODEL_SEARCH_VENDORS[channelType] ?? ''
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
// /api/models/search 已经在后端按 model_name、description、tags 完成过滤；
// 前端不能再要求模型名自身包含关键词，否则搜索供应商、标签或描述时会丢失真实命中模型。
// 名称规则模型优先使用运行时展开的 matched_models，避免把规则占位名误加入渠道。
// matched_count 可能大于当前返回的 matched_models 数量；这种情况只作为候选缺口信息，
// 页面只展示已经返回的真实模型名，避免把不可见或不可调用的占位结果写入渠道。
export function getModelSearchModelNameResult(
  searchItems: readonly ModelSearchItemLike[],
  keyword: string
): ModelSearchModelNameResult {
  const normalizedKeyword = keyword.trim().toLowerCase()
  const seenKeys = new Set<string>()
  const names: string[] = []
  let unresolvedMatchedCount = 0

  for (const item of searchItems) {
    const matchedModels = Array.isArray(item.matched_models)
      ? item.matched_models
      : []
    const isRuleModel = typeof item.name_rule === 'number' && item.name_rule > 0
    const modelName = item.model_name?.trim()
    const ruleFallback =
      isRuleModel &&
      matchedModels.length === 0 &&
      modelName &&
      (!normalizedKeyword ||
        normalizeModelSearchKey(modelName).includes(normalizedKeyword))
        ? [modelName]
        : []
    const candidates = isRuleModel
      ? [...matchedModels, ...ruleFallback]
      : [modelName, ...matchedModels]
    const expectedMatchedCount =
      typeof item.matched_count === 'number' && item.matched_count > 0
        ? item.matched_count
        : 0
    if (isRuleModel && expectedMatchedCount > matchedModels.length) {
      unresolvedMatchedCount += expectedMatchedCount - matchedModels.length
    }
    for (const candidate of candidates) {
      const name = candidate?.trim()
      if (!name) continue

      const key = normalizeModelSearchKey(name)
      if (seenKeys.has(key)) continue

      seenKeys.add(key)
      names.push(name)
    }
  }

  return { names, unresolvedMatchedCount }
}

export function getModelSearchModelNames(
  searchItems: readonly ModelSearchItemLike[],
  keyword: string
): string[] {
  return getModelSearchModelNameResult(searchItems, keyword).names
}
