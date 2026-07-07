# NexusTok 与 new-api-main 全量文件差异索引

本文是 `new-api-main-diff-analysis.md` 的可复核补充，用来回答“是否覆盖所有差异”。主文档按功能和页面解释差异，本文记录文件级差异的生成方式、统计口径和分类规则。因为两个仓库同路径但内容不同的文件超过千个，本文不直接内嵌所有路径，而是提供可重复生成的清单。

## 统计口径

对比目录：

| 项目 | 路径 |
|------|------|
| NexusTok | `/opt/project/NexusTok` |
| new-api-main | `/opt/project/new-api-main` |

忽略范围：

| 模式 | 原因 |
|------|------|
| `node_modules` | 依赖目录，不代表项目功能。 |
| `dist`、`build` | 构建产物，可由源码再生成。 |
| `tmp`、`logs`、`*.log` | 临时文件和运行日志。 |

当前快照统计：

| 差异类别 | 文件数 | 说明 |
|----------|--------|------|
| NexusTok 独有 | 1060 | 主要来自内嵌 `modules/`、账号池、渠道账号、文档、默认前端账号池/计费页，以及差异索引脚本、请求体限制测试和 NexusTok 原生 SystemInfo/SystemTask 路由与测试。 |
| new-api-main 独有 | 226 | 主要来自 Authz enforcement/SystemTask 其它业务 handlers、Flow 前端图表、Waffo Pancake catalog/pair 体验、DataTable/RichContent/Playground 重构。SystemInstance 后端心跳、SystemInfo 实例页面、SystemTask runner/日志清理/批量渠道测试/上游模型同步/订阅维护/Midjourney 与异步任务轮询后端、流量账本查询后端、订阅余额支付/钱包溢出后端、Authz catalog 与用户权限矩阵回传、SystemTask 前端面板、QuotaMath 与额度饱和审计测试已在 NexusTok 落地为原生实现差异。 |
| 同路径但内容不同 | 1877 | 主要来自默认前端、classic 前端、Relay、controller、service、model、common、setting 等共享模块的实现差异；QuotaMath、SystemInstance、SystemTask runner、SystemTask 前端面板、channel_test/model_update/subscription_maintenance/midjourney_poll/async_task_poll handler 与 quota_saturation/log_format 测试已按 NexusTok 语义原生化。 |

## 重新生成清单

在 NexusTok 根目录执行：

```bash
./scripts/compare-new-api-main.sh
```

默认输出到：

```text
tmp/new-api-main-diff/
```

生成文件：

| 文件 | 内容 |
|------|------|
| `nexustok-files.txt` | NexusTok 参与对比的全部文件。 |
| `new-api-main-files.txt` | new-api-main 参与对比的全部文件。 |
| `nexustok-only.txt` | 只在 NexusTok 存在的文件。 |
| `new-api-main-only.txt` | 只在 new-api-main 存在的文件。 |
| `common-files.txt` | 两边同路径都存在的文件。 |
| `common-changed.txt` | 两边同路径都存在但内容不同的文件。 |
| `summary.md` | 三类差异数量和顶层目录分布。 |

如果 new-api-main 路径不同，可以覆盖环境变量：

```bash
NEW_API_ROOT=/path/to/new-api-main ./scripts/compare-new-api-main.sh
```

## 顶层目录分布

### NexusTok 独有文件

| 顶层目录 | 文件数 | 主要含义 |
|----------|--------|----------|
| `modules` | 894 | NexusTok 内嵌账号池/CLIProxy/CPA 管理相关模块，是本项目账号池方向的重要原生资产。 |
| `web` | 53 | 默认前端账号池、计费设置、classic 账号池/计费页、数据表格旧结构等。 |
| `service` | 36 | 账号池服务、渠道账号服务、Codex OAuth、openai compat、SystemTask runner 等。 |
| `docs` | 27 | NexusTok 自有功能文档、SDK 文档和差异索引。 |
| `controller` | 20 | 账号池、渠道账号、Codex OAuth、OAuth provider、模型同步任务等控制器。 |
| `model` | 9 | 账号池、账号池健康、渠道账号、模型定价配置、日志清理测试等。 |
| `router` | 4 | 渠道路由、系统信息路由、系统任务路由及对应测试。 |
| `middleware` | 4 | NexusTok 自有中间件测试、请求体限制测试和分发相关能力。 |
| `common` | 3 | 会话密钥、secret 等公共工具。 |
| 其它 | 14 | 账号池 sidecar Docker、热更新脚本、差异生成脚本、服务配置等。 |

### new-api-main 独有文件

| 顶层目录 | 文件数 | 主要含义 |
|----------|--------|----------|
| `web` | 145 | DataTable 分层、RichContent、Playground 重构、Waffo Pancake 设置向导、UsageLogs 移动端卡片。SystemInfo/SystemTask 默认前端观测页已转为同路径实现差异。 |
| `service` | 20 | Authz enforcement、SystemTask 其它业务 handlers、RelayConvert、任务轮询测试等。Authz catalog、用户自身份权限矩阵回传、SystemInstance 最小心跳层、SystemTask runner/日志清理/批量渠道测试/上游模型同步/订阅维护/异步任务轮询、订阅钱包溢出回退控制、ProtectedFetch 与额度饱和审计已按 NexusTok 边界原生化。 |
| `relay` | 21 | OpenAI Realtime/Image、Gemini Responses、Advanced Custom、更多 Relay 测试。 |
| `model` | 11 | Authz、Casbin、Flow 前端依赖数据的其它测试、Locking、ClickHouse 测试。SystemInstance、SystemTask、流量账本聚合、订阅余额支付字段/订单事务与日志脱敏测试已按 NexusTok 语义补齐。 |
| `controller` | 5 | Authz enforcement、审计、订阅 Waffo Pancake 等；Authz catalog、用户自身份权限矩阵回传、SystemInfo 实例列表接口、SystemTask 只读接口、批量渠道测试 handler、Midjourney 轮询 handler、流量账本查询接口和订阅余额支付接口已按 Root/角色边界落地。 |
| `common` | 4 | 节点身份、邮件 NTLM 等。匿名请求体限制、QuotaMath 和部分 SSRF helper 已在 NexusTok 落地，但实现语义按本项目调整。 |
| `middleware` | 1 | 审计。匿名请求体限制和 HeaderNav 模块鉴权已在 NexusTok 落地，但测试仍按本项目补充。 |
| `router` | 1 | Authz catalog 路由、Channel 路由拆分及 SystemInfo/SystemTask 路由已按 NexusTok 边界原生化；权限 enforcement 路由表仍待接入。 |
| 其它 | 18 | 多语言 README、Electron 文档、poster、计费表达式测试等。 |

### 同路径但内容不同

| 顶层目录 | 文件数 | 主要含义 |
|----------|--------|----------|
| `web` | 1268 | 绝大部分页面/组件同路径但实现不同，需要按页面和 feature 逐项吸收；SystemInfo/SystemTask 默认前端已落地后转为同路径差异。 |
| `relay` | 196 | Relay 能力、请求转换、测试覆盖和上游协议支持差异。 |
| `controller` | 71 | 管理接口、支付、用户、渠道、日志、模型同步等业务差异；批量渠道测试、上游模型同步、订阅维护、Midjourney 轮询和流量账本查询已接入 NexusTok 原生边界，继续保留为同路径实现差异。 |
| `service` | 62 | 计费、订阅、任务、OAuth、Relay 转换和外部请求安全等实现差异；SystemTask runner、日志清理、批量渠道测试、上游模型同步、订阅维护、异步任务轮询和额度饱和审计已落地为同路径差异。 |
| `setting` | 49 | 系统设置、计费设置、支付设置、模型设置差异。 |
| `common` | 50 | 公共工具、SSRF、请求体、配额、环境初始化差异；SSRF 已增加 Dial 阶段保护 helper，QuotaMath 已落地为 NexusTok 原生版本，但与 new-api-main 仍有实现边界差异。 |
| `model` | 42 | 数据模型、日志、订阅、用户、渠道、任务等差异；日志清理批量删除、流量账本维度扩展和日志脱敏测试已落地为同路径差异。 |
| `dto` | 29 | 请求/响应 DTO 差异，迁移时必须遵守显式零值规则。 |
| `middleware` | 24 | 鉴权、限流、日志、请求处理等中间件差异。 |
| `pkg` | 18 | 计费表达式、缓存、内部包实现差异。 |
| 其它 | 68 | 路由、i18n、electron、构建和仓库元数据差异。 |

## 重点目录精细统计

| 目录 | NexusTok 独有 | new-api-main 独有 | 同路径不同 | 处理策略 |
|------|---------------|-------------------|------------|----------|
| `router` | 4 | 1 | 8 | 渠道、系统信息、系统任务和 Authz catalog 路由已抽出并保持现有权限行为；Codex 用量 reset API 与订阅余额支付 API 已挂载，Authz 权限表和账号池路由拆分继续分批推进。 |
| `controller` | 20 | 5 | 71 | 账号池控制器保留；ChannelAuthz 已先按 Root 兜底吸收渠道敏感字段 fail-closed，Codex 用量 reset、Authz catalog、用户自身份权限矩阵回传、系统实例列表、SystemTask 只读接口、批量渠道测试 handler、上游模型同步 handler、Midjourney 轮询 handler、流量账本查询接口和订阅余额支付接口已补，剩余治理类控制器按 P1/P2 原生化。 |
| `service` | 36 | 20 | 62 | 账号池服务保留；Authz catalog、ProtectedFetch、SystemInstance 心跳、SystemTask runner/日志清理/批量渠道测试/上游模型同步/订阅维护/异步任务轮询、订阅钱包溢出回退控制和额度饱和审计已按本项目边界迁移，工具调用附加费、违规费用、充值入账、视频任务、渠道测试和 Codex 用量 reset 已补保护/接口，Authz enforcement/SystemTask 其它业务 handler 继续分批推进。 |
| `model` | 9 | 11 | 42 | SystemInstance、SystemTask、quota_data 流量账本维度扩展、订阅余额支付字段和用户订阅钱包溢出快照已确认 SQLite/MySQL/PostgreSQL AutoMigrate 兼容并落地；日志清理批量删除已改为先查 ID 再删除以保持三库一致；迁移后续 Authz 新模型时仍需逐项确认三库兼容。 |
| `middleware` | 4 | 1 | 24 | 请求体限制与 HeaderNav 模块鉴权已吸收；全局审计需要配套权限/设置。 |
| `common` | 3 | 4 | 50 | 请求体限制已吸收；SSRF/Fetch 安全已补 Dial 阶段保护；QuotaMath 已吸收为 NexusTok 原生 helper，后续继续覆盖剩余裸转换。 |
| `relay` | 0 | 21 | 196 | 逐协议迁移，必须补测试并检查 StreamOptions 支持。 |
| `setting` | 0 | 1 | 49 | 不直接覆盖设置结构，按页面和 API 逐项合并。 |
| `pkg` | 0 | 1 | 18 | 涉及计费表达式时继续遵守 `pkg/billingexpr/expr.md`。 |
| `web/default/src/routes` | 3 | 1 | 60 | `/system-info` 已在 NexusTok 默认前端落地，剩余独有路由主要是注册兼容入口；页面清单已在主文档逐路由列出。 |
| `web/default/src/features` | 21 | 69 | 533 | 账号池/计费保留；SystemInfo/SystemTask 与 Flow 后端已吸收，DataTable、UsageLogs、Playground 和 dashboard Flow 图表继续分批迁移。 |
| `web/default/src/components` | 15 | 56 | 155 | 优先迁移安全渲染、通用表格布局、Dialog/Drawer 长表单布局。 |
| `web/default/src/hooks` | 0 | 1 | 20 | `use-sidebar-view` 可服务多层设置和系统信息。 |
| `web/default/src/lib` | 0 | 4 | 27 | Admin permissions 已先以后端回传契约和 auth store 类型承接；frontend cache、content format 可按治理能力继续迁移。 |
| `web/classic/src` | 4 | 2 | 397 | Classic 保留账号池/计费入口；退场提示低优先级。 |

## 页面级覆盖结论

默认前端路由文件只有 4 个独有页面入口：

| 项目 | 独有路由文件 | 页面 |
|------|--------------|------|
| NexusTok | `_authenticated/account-pool/index.tsx` | `/account-pool` |
| NexusTok | `_authenticated/account-pool/$section.tsx` | `/account-pool/:section` |
| NexusTok | `_authenticated/pricing-settings/index.tsx` | `/pricing-settings` |
| new-api-main | `(auth)/register.tsx` | `/register` 兼容页 |

`/system-info` 已在 NexusTok 默认前端落地，现在属于“同路径但实现不同”的页面。其它默认前端页面大多也是“同路径但实现不同”，已经在主文档按路由、feature 目录和组件能力列出。后续迁移时应以同路径页面为单位做局部对比，而不是简单复制 new-api-main 文件。

Classic 前端没有 new-api-main 独有业务页面。NexusTok 独有 classic 账号池页和计费设置页必须保留，new-api-main 的 classic 差异主要是退场提示和主题 helper。

## 原生化执行原则

1. 对独有文件差异，先判断是 NexusTok 原生优势还是 new-api-main 优势；NexusTok 账号池相关文件默认保留。
2. 对同路径不同文件，不做整文件覆盖；先定位页面/接口的业务意图，再把优势能力抽成 NexusTok 原生实现。
3. 后端新增能力必须遵守本项目 JSON 封装、三数据库兼容、中文注释和提交规范。
4. 前端新增页面必须先确认后端接口已经原生化，新增文案走 i18n。
5. 每完成一个独立能力，更新主差异文档或本文的迁移状态，形成可审计的演进轨迹。
