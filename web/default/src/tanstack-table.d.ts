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
import '@tanstack/react-table'

declare module '@tanstack/react-table' {
  // 公共表格列元数据，用于排序提示、移动端列表和桌面卡片视图。
  interface ColumnMeta<_TData, _TValue> {
    // 手机端和卡片视图字段标签；没有 label 时会尝试回退到字符串 header。
    label?: string
    // 可在 tooltip 或帮助文案中展示的列说明。
    description?: string
    // 是否允许排序；业务列可用它覆盖默认判断。
    sortable?: boolean
    // 应用于列单元格的自定义 className。
    className?: string
    // 将该列作为卡片标题，通常用于名称、令牌、模型或渠道。
    mobileTitle?: boolean
    // 将该列作为卡片标题行右侧徽标，通常用于状态或类型。
    mobileBadge?: boolean
    // 在手机端和通用卡片视图中隐藏低价值或已重复表达的列。
    mobileHidden?: boolean
    // 控制卡片字段区域的展示顺序；未设置的字段排在已设置字段之后。
    mobileOrder?: number
  }
}
