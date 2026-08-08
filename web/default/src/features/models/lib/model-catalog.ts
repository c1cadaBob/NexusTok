/*
Copyright (C) 2023-2026 c1cada

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@c1cada.dev
*/
import { searchModels } from '../api'
import type { Model } from '../types'

/**
 * 模型目录接口单页最多返回 100 条记录。选择器需要看到完整模型仓库，
 * 因此由前端模块自动补齐后续页，而不是把某一页误当成全量候选。
 */
export const MODEL_CATALOG_PAGE_SIZE = 100

type ModelCatalogItem = Pick<
  Model,
  'model_name' | 'name_rule' | 'matched_models' | 'matched_count'
>

export function normalizeModelCatalogName(value: string): string {
  return value.trim().toLowerCase()
}

export function dedupeModelCatalogNames(
  values: readonly string[]
): string[] {
  const seen = new Set<string>()
  const result: string[] = []

  for (const rawValue of values) {
    const value = rawValue.trim()
    const key = normalizeModelCatalogName(value)
    if (!key || seen.has(key)) continue
    seen.add(key)
    result.push(value)
  }

  return result
}

/**
 * 将模型元数据中的普通模型名和名称规则展开结果转换为选择器候选。
 * 规则模型优先使用后端已经计算出的真实匹配模型，避免把搜索占位符当成可路由模型。
 */
export function getModelCatalogNames(
  items: readonly ModelCatalogItem[]
): string[] {
  const names: string[] = []

  for (const item of items) {
    const modelName = item.model_name?.trim() ?? ''
    const matchedModels = Array.isArray(item.matched_models)
      ? item.matched_models
      : []
    const isRuleModel = typeof item.name_rule === 'number' && item.name_rule > 0

    if (isRuleModel && matchedModels.length > 0) {
      names.push(...matchedModels)
      continue
    }

    if (modelName) names.push(modelName)
    if (!isRuleModel) names.push(...matchedModels)
  }

  return dedupeModelCatalogNames(names)
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

/**
 * 模型下拉框使用“安全文字正则”：输入会先转义，再做不区分大小写的包含匹配。
 * 这样 `gpt-5.5` 中的点只代表普通字符，不会意外匹配其它模型。
 */
export function filterModelCatalogNames(
  values: readonly string[],
  inputValue: string
): string[] {
  const keyword = inputValue.trim()
  if (!keyword) return [...values]

  const matcher = new RegExp(escapeRegExp(keyword), 'i')
  return values.filter((value) => matcher.test(value))
}

/**
 * 加载模型仓库全部分页，供所有模型选择器共享 TanStack Query 缓存。
 * 上游实时模型获取不经过此函数，保证两个来源在数据流和交互上保持独立。
 */
export async function fetchAllModelCatalogNames(): Promise<string[]> {
  const firstPage = await searchModels({
    p: 1,
    page_size: MODEL_CATALOG_PAGE_SIZE,
  })

  if (!firstPage.success) {
    throw new Error(firstPage.message || 'Failed to load model catalog')
  }

  const firstItems = firstPage.data?.items ?? []
  const total = firstPage.data?.total ?? firstItems.length
  const totalPages = Math.max(
    1,
    Math.ceil(total / MODEL_CATALOG_PAGE_SIZE)
  )

  if (totalPages === 1) {
    return getModelCatalogNames(firstItems)
  }

  const remainingPages = await Promise.all(
    Array.from({ length: totalPages - 1 }, (_, index) =>
      searchModels({
        p: index + 2,
        page_size: MODEL_CATALOG_PAGE_SIZE,
      })
    )
  )

  const failedPage = remainingPages.find((page) => !page.success)
  if (failedPage) {
    throw new Error(failedPage.message || 'Failed to load model catalog')
  }

  const allItems = [
    ...firstItems,
    ...remainingPages.flatMap((page) => page.data?.items ?? []),
  ]

  return getModelCatalogNames(allItems)
}
