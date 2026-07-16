# NexusTok DataTable 组件

`web/default/src/components/data-table/` 是默认前端列表页的公共表格工具包。功能页面应优先从 `@/components/data-table` 导入公开能力，保持 `index.ts` 作为稳定入口，避免直接依赖内部文件路径。

## 目录职责

| 文件                          | 说明                                                                                        |
| ----------------------------- | ------------------------------------------------------------------------------------------- |
| `data-table-page.tsx`         | 列表页通用组合层，负责工具条、桌面表格、移动端列表、批量操作、分页、加载态和空态的排列。    |
| `card-grid.tsx`               | 桌面端卡片网格视图，复用列 `meta` 生成通用卡片内容。                                        |
| `card-row-content.tsx`        | 卡片行内容渲染，按 `mobileTitle`、`mobileBadge`、`mobileHidden` 和 `mobileOrder` 组织字段。 |
| `toolbar.tsx`                 | 搜索、列筛选、展开筛选、重置、显式 Search 按钮、列显示按钮和自定义 action 区域。            |
| `view-mode-toggle.tsx`        | 表格/卡片视图切换控件。                                                                     |
| `use-data-table-view-mode.ts` | 表格/卡片视图状态管理和可选 localStorage 持久化。                                           |
| `pagination.tsx`              | TanStack Table 分页控件，支持页码、首页/尾页、上一页/下一页和每页行数。                     |
| `mobile-card-list.tsx`        | 手机端列表视图，按列 meta 自动生成标题、状态徽标、字段网格或键值 fallback。                 |
| `bulk-actions.tsx`            | 桌面端批量操作浮层，通常与行选择列配合使用。                                                |
| `column-header.tsx`           | 可排序列头，封装排序状态和列操作入口。                                                      |
| `faceted-filter.tsx`          | 列枚举筛选控件，支持多选和单选。                                                            |
| `view-options.tsx`            | 列显示/隐藏控制。                                                                           |
| `badge-cell.tsx`              | 单个或少量 badge 的表格单元格容器，统一处理收缩、截断和 overflow 边界。                    |
| `badge-list-cell.tsx`         | 多 badge 列表单元格，支持前 N 个展示、`+N` 折叠和完整列表 tooltip。                         |
| `static-data-table.tsx`       | 静态数组表格，适合弹窗预览、状态清单和其它不需要 TanStack 状态的简单表格。                  |
| `truncated-cell.tsx`          | DataTable 单行文本截断单元格，通过 tooltip 暴露完整文本。                                  |
| `table-empty.tsx`             | 桌面表格空态行。                                                                            |
| `table-skeleton.tsx`          | 桌面表格加载骨架。                                                                          |
| `index.ts`                    | 对外导出入口，新增通用能力时应优先在这里保持兼容导出。                                      |

## 稳定导入

功能页面请使用统一入口：

```tsx
import {
  DataTablePage,
  DataTableColumnHeader,
  DataTableBulkActions,
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
} from '@/components/data-table'
```

避免在业务页面中导入 `./data-table-page`、`./toolbar` 等内部路径。这样后续如果把当前扁平目录逐步拆成更细的 `core`、`layout`、`toolbar`、`static` 或 `hooks` 分层，可以通过 `index.ts` 继续保持页面侧 import 稳定。

## 推荐组合

大多数列表页应使用 `DataTablePage` 作为外层组合，而不是在页面里重复拼桌面表格、手机列表和分页：

```tsx
<DataTablePage
  table={table}
  columns={columns}
  isLoading={isLoading}
  isFetching={isFetching}
  emptyTitle={t('No Channels Found')}
  emptyDescription={t('Create a channel to start routing requests.')}
  toolbarProps={{
    searchPlaceholder: t('Filter channels...'),
    searchKey: 'name',
    filters,
  }}
  bulkActions={<ChannelBulkActions table={table} />}
  getRowClassName={(row, ctx) =>
    row.original.status === 0
      ? ctx.isMobile
        ? DISABLED_ROW_MOBILE
        : DISABLED_ROW_DESKTOP
      : undefined
  }
/>
```

`DataTablePage` 的常用扩展点：

| Prop                 | 用途                                                                |
| -------------------- | ------------------------------------------------------------------- |
| `toolbarProps`       | 使用默认工具条，并传入搜索、筛选、展开筛选、Search 和 Reset 行为。  |
| `toolbar`            | 完全替换默认工具条，适合页面有特殊筛选布局时使用。                  |
| `bulkActions`        | 桌面端行选择后的批量操作区。                                        |
| `mobile`             | 完全替换默认手机列表，适合需要高度定制卡片的页面。                  |
| `mobileProps`        | 调整默认手机列表的 row key 或 className。                           |
| `getRowClassName`    | 同时控制桌面行和手机卡片样式，可通过 `ctx.isMobile` 区分。          |
| `renderRow`          | 替换默认桌面行渲染，适合展开行、聚合行或整行跳转。                  |
| `afterTable`         | 在表格和分页之间插入汇总信息或说明。                                |
| `paginationInFooter` | 控制分页是否放入页面底部 portal。                                   |
| `enableCardView`     | 为桌面端启用表格/卡片视图切换；默认关闭，未显式接入的页面行为不变。 |
| `viewModeStorageKey` | 按页面持久化用户选择的桌面视图模式。                                |
| `renderCard`         | 覆盖默认卡片内容，适合需要定制卡片布局的页面。                      |

## 移动端列 Meta

`MobileCardList` 会读取 TanStack ColumnDef 的 `meta` 字段。新表格如果需要良好的手机端体验，应优先标记标题列和状态列：

```tsx
{
  accessorKey: 'name',
  header: t('Name'),
  cell: ({ row }) => <span>{row.original.name}</span>,
  meta: {
    label: t('Name'),
    mobileTitle: true,
  },
}
```

支持的 meta：

| 字段           | 说明                                                                           |
| -------------- | ------------------------------------------------------------------------------ |
| `label`        | 手机端字段标签。没有 `label` 时，如果 `header` 是字符串，会回退使用 `header`。 |
| `mobileTitle`  | 把该列作为手机卡片标题，通常用于名称、令牌名、模型名或渠道名。                 |
| `mobileBadge`  | 把该列放在标题行右侧，通常用于状态、类型或 provider 徽标。                     |
| `mobileHidden` | 手机端隐藏该列，适合选择框、低价值字段或已在标题/状态中表达的信息。            |
| `mobileOrder`  | 控制卡片字段区域的展示顺序；未设置的字段排在已设置字段之后。                   |

如果没有任何列设置 `mobileTitle` 或 `mobileBadge`，手机端会回退为紧凑的键值列表。该 fallback 可读性不如专门设计的标题/状态/字段网格，因此新页面应主动配置 meta。

## 桌面卡片视图

对信息密集、需要快速扫读的列表页，可以显式启用桌面卡片视图：

```tsx
<DataTablePage
  table={table}
  columns={columns}
  enableCardView
  viewModeStorageKey='models-table-view-mode'
/>
```

卡片视图默认使用与手机端一致的列 meta 语义，因此大多数页面只需要补齐 `mobileTitle`、`mobileBadge`、`mobileHidden` 和 `label`。如果页面已有特殊卡片设计，可以传入 `renderCard` 覆盖内容，同时继续使用 `DataTablePage` 的分页、筛选、加载和空态能力。

## 组件边界

公共 DataTable 目录只承载跨页面复用的表格基础能力。以下内容应保留在功能目录内：

1. 业务列定义，例如渠道余额、账号池状态、订阅周期、用户额度等。
2. 行级操作菜单和确认弹窗。
3. 页面专属筛选项、批量操作和导出逻辑。
4. 接口请求、权限判定、缓存 key 和业务状态转换。

当同一段表格展示逻辑被两个以上页面复用，并且不依赖具体业务接口时，再考虑沉淀到公共 DataTable 或其它公共组件目录。

## Badge 单元格

单个 badge 或少量 badge 的表格列优先使用 `BadgeCell`，它只处理表格单元格的宽度、收缩和 overflow 边界，不处理业务状态：

```tsx
<BadgeCell>
  <StatusBadge label={t('Enabled')} variant='success' copyable={false} />
</BadgeCell>
```

当一列可能包含多个业务 badge，并需要只展示前 N 个、用 `+N` 折叠剩余项时，继续使用 `BadgeListCell`。

## 静态数组表格

弹窗里的导入预览、状态清单、批量操作结果等本地数组，不需要 TanStack Table 的排序、筛选、列显示和分页状态。此类场景优先使用 `StaticDataTable`：

```tsx
<StaticDataTable
  columns={[
    {
      id: 'name',
      header: t('Name'),
      cellClassName: staticDataTableClassNames.mediumCell,
      cell: (item) => item.name,
    },
  ]}
  data={items}
  emptyContent={t('No items found')}
/>
```

`StaticDataTable` 的边界：

1. 只负责静态数组渲染、空态行和默认文本截断。
2. 不承载排序、筛选、列显示、行选择、分页和数据请求。
3. 操作按钮、确认弹窗、权限判断和接口调用仍放在业务组件中。
4. 需要完整列表页体验时继续使用 `DataTablePage`。

## 后续分层原则

NexusTok 会逐步吸收更清晰的 DataTable 分层，但不做一次性目录级替换。后续拆分时遵循这些不变量：

1. 保持 `@/components/data-table` 作为公开导入入口。
2. 每次拆分只移动一类职责，例如先拆核心渲染，再拆 toolbar，不混入页面级重构。
3. 移动端卡片、批量操作和分页行为需要先有回归验证，再迁移高频页面。
4. 静态数组表格和 TanStack 状态表格应分开抽象，避免让简单页面承担不必要的状态成本。
5. 任何新公共组件都需要在至少两个真实页面中有复用价值，避免为了目录整齐而增加抽象。
