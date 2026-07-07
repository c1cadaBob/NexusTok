# NexusTok 与 new-api-main 差异分析及原生化路线

本文档记录当前项目 `NexusTok` 与 `/opt/project/new-api-main` 文件快照的功能、页面和基础设施差异，并给出把 `new-api-main` 优势转换为 NexusTok 原生能力的建议。本文只做差异分析和演进规划，不把 `new-api-main` 作为运行时依赖。

## 对比基线

| 项目 | 基线 | 说明 |
|------|------|------|
| NexusTok | `f184363` | 当前仓库存在账号池相关未提交工作区改动，本文不修改这些文件。 |
| `/opt/project/new-api-main` | 当前目录快照 | 该目录没有 `.git` 元数据，无法绑定提交哈希，只能按现有文件内容对比。 |

本次核对范围：

1. 后端：`router/`、`controller/`、`service/`、`model/`、`middleware/`、`common/`、`relay/`、`setting/`、`pkg/`。
2. 默认前端：`web/default/src/routes`、`web/default/src/features`、`web/default/src/components`、`web/default/src/hooks`、`web/default/src/lib`。
3. 经典前端：`web/classic/src/App.jsx`、侧边栏、页面目录和关键弹窗。
4. 文档与依赖：`docs/features`、`go.mod`、`web/default/package.json`。

## 总体结论

NexusTok 当前已经在账号池方向形成了明显原生优势：有 `/api/account-pool` 完整管理接口、默认前端 `/account-pool` 多分区页面、经典前端 `/console/account-pool`、渠道原生账号池绑定、Codex OAuth/账号池凭据导入、账号检测任务、使用日志、状态审计和健康看板等能力。这些能力在 `new-api-main` 中不存在。

`new-api-main` 的优势更偏平台治理和基础设施：细粒度管理权限、系统任务中心、多实例状态、请求体限制、顶栏模块鉴权、额度饱和保护、受保护的 Fetch/SSRF 客户端、ClickHouse 日志测试、仪表盘流量桑基图、移动端日志卡片、系统信息页面、Waffo Pancake 产品绑定体验，以及更模块化的新 Playground / DataTable / Rich Content 组件。

推荐演进方向不是复制 `new-api-main`，而是把这些优势收敛到 NexusTok 的原生域模型：

1. 账号池继续作为 NexusTok 的核心差异化能力，承接渠道账号、Codex OAuth、检测任务、状态审计和健康看板。
2. 权限系统应优先围绕账号池、渠道、用户、模型、订阅、系统设置做资源级授权，而不是只有 Admin/Root 粗粒度角色。
3. 系统任务应成为账号池检测、日志清理、批量渠道测试、上游模型同步、订阅重置等后台工作的统一执行和观测框架。
4. 系统信息应服务多节点运维：展示主/从节点、版本、资源、最后心跳，并与账号池自动检测和任务调度联动。
5. 额度安全、请求体限制和 SSRF 保护属于底层安全债，应优先吸收，避免支付、计费、拉取外部内容、匿名接口成为风险面。

## 后端功能差异

### NexusTok 已有而 new-api-main 缺少

| 功能 | NexusTok 文件/接口 | new-api-main 状态 | 原生化保留策略 |
|------|-------------------|------------------|----------------|
| 原生账号池 | `model/account_pool.go`、`controller/account_pool.go`、`service/account_pool_*`、`/api/account-pool/*` | 缺少 | 继续作为核心域模型，不退回外部 Sidecar/CPAMC/CLIProxyAPI。 |
| 认证文件管理 | `AccountPoolAuthFile`、`POST /api/account-pool/auth-files`、`POST /api/account-pool/auth-files/import` | 缺少 | 维持“凭证全局 + 组内 PoolAccount 派生”模型，后续再评估真正账号多对多关系。 |
| 账号池健康看板 | `GET /api/account-pool/health`、`model/account_pool_health.go` | 缺少 | 后续可接入系统信息页，形成实例维度和账号池维度的统一健康视图。 |
| 账号池使用日志 | `GET /api/account-pool/usage-logs`、`service/account_pool_usage_service.go` | 缺少 | 作为账号池审计主线，后续接入权限系统，限制非 Root 查看敏感账号。 |
| 账号池状态审计 | `GET /api/account-pool/state-logs`、`audit-summary`、`export` | 缺少 | 保留独立审计模型，后续与全局管理审计统一展示，但不要丢失账号粒度字段。 |
| 账号检测任务 | `POST /api/account-pool/groups/:id/accounts/check-tasks`、`GET /api/account-pool/check-tasks` | 缺少 | 优先迁移到统一 SystemTask 框架，避免账号池自己维护一套任务生命周期。 |
| 账号批量状态/删除/导出 | `/groups/:id/accounts/status`、`delete`、`export` | 缺少 | 保留，后续加细粒度权限和安全验证。 |
| 账号池 OAuth/设备授权 | `/groups/:id/oauth/:provider/start`、`/device/:provider/start`、Codex OAuth | 缺少 | 作为 NexusTok 原生凭据接入入口，后续抽象为 provider registry。 |
| 渠道账号 | `model/channel_account.go`、`controller/channel_account.go`、`service/channel_account_*` | 缺少 | 与账号池区分：渠道账号适合单渠道多 Key，账号池适合跨渠道共享和调度。 |
| 渠道账号池绑定 | `constant/channel_credential_mode.go`、渠道 `account_pool_group_id` | 缺少 | 保留为 Relay 热路径原生能力，后续统一校验分组状态、平台和认证类型。 |
| Codex 渠道 OAuth | `controller/codex_oauth.go`、`service/codex_oauth.go` | new-api-main 也有部分 Codex 使用查询，但无账号池 Codex 接入 | 保留 NexusTok 的渠道和账号池双入口，并补齐 new-api-main 的 reset-credits 接口能力。 |
| 账号池任务级限制 | `service/account_pool_task_limit.go` | 缺少 | 继续围绕任务类 Relay 做并发/频率/等待策略，不与普通渠道限流混淆。 |
| 模型定价配置表 | `model/model_pricing_config.go` | 缺少 | 保留，用于承载 NexusTok 独立计费配置和后续表达式定价演进。 |
| 账号池文档 | `docs/features/account-pool.md`、`account-pool-roadmap.md` | 缺少同等文档 | 继续作为账号池开发准绳。 |

### new-api-main 已有而 NexusTok 缺少或较弱

| 功能 | new-api-main 文件/接口 | NexusTok 状态 | 原生化建议 |
|------|------------------------|---------------|------------|
| 细粒度授权 Authz | `service/authz/*`、`model/authz_role.go`、`model/casbin_rule.go`、`router/authz-router.go`、`GET /api/authz/catalog` | 缺少 | 引入 NexusTok 原生 `AdminPermission` 能力，先覆盖渠道、账号池、用户、模型、订阅、系统设置六类资源。 |
| 渠道路由权限表 | `router/channel-router.go`、`middleware.RequirePermission` | NexusTok 仍在 `api-router.go` 里直接挂 AdminAuth/RootAuth | 抽出 `registerChannelRoutes`，将读、操作、写、敏感写拆开；账号池路由也应采用同样模式。 |
| 渠道敏感字段 fail-closed | `controller/channel_authz.go` | 缺少 | 对渠道和账号池表单都做字段分类：新增字段未分类时默认视为敏感写，防止权限绕过。 |
| 管理操作审计兜底 | `middleware/audit.go`、`controller/audit.go` | 账号池有状态审计，但全局管理审计较弱 | 保留账号池专用审计，同时新增全局操作审计兜底，用 action + params 支持前端 i18n。 |
| 系统任务中心 | `model/system_task.go`、`service/system_task.go`、`controller/system_task.go`、`/api/system-task/*` | 缺少统一框架，账号池检测任务为独立实现 | 作为 P1 原生化重点，把日志清理、账号池批量检测、渠道批量测试、模型同步任务统一迁入。 |
| 系统实例心跳 | `model/system_instance.go`、`service/system_instance.go`、`GET /api/system-info/instances` | 缺少 | 引入 `NODE_NAME`、主从节点、CPU/内存/磁盘/版本心跳，为多节点账号池调度和任务锁提供观测。 |
| 后台任务锁 | `SystemTaskLock`、租约续期、过期失败标记 | 缺少 | 用数据库锁实现跨节点互斥，必须兼容 SQLite/MySQL/PostgreSQL；任务 handler 要支持 context cancellation。 |
| 匿名请求体限制 | `common/request_body_limit.go`、`middleware/request_body_limit.go` | 缺少 | 用于注册、登录、setup、OAuth 绑定、Webhook 等匿名入口，默认 512KB，可配置。 |
| 顶栏模块鉴权 | `middleware/header_nav.go` | NexusTok 默认价格/排行更偏 TryUserAuth | 让公开页模块支持 `enabled + requireAuth`，页面隐藏和接口鉴权一致。 |
| 额度饱和保护 | `common/quota_math.go`、`QuotaClamp`、相关测试 | NexusTok 未见同等集中实现 | 将所有计费路径的裸 `int(...)`、`math.Round(...)` 转换收敛到统一 helper，并把饱和事件写入 admin-only 日志。 |
| 受保护 Fetch / SSRF | `service/protected_fetch_client.go` | 缺少或分散 | 所有远程拉取 URL、Discovery、模型同步、文件解析入口统一走 SSRF 保护客户端。 |
| ClickHouse 日志兼容测试 | `model/clickhouse_log_test.go`、`gorm.io/driver/clickhouse` | 缺少依赖 | 仅在明确支持 ClickHouse 日志库时引入；否则先记录为可选能力，避免增加部署复杂度。 |
| 流量账本查询 | `model/usedata_flow.go`、`controller.GetAllFlowQuotaDates`、`/api/data/flow` | 缺少 | 可用于账号池和渠道成本归因，建议接入仪表盘的流量 Sankey 图。 |
| Relay 转换包整理 | `service/relayconvert/*`、更多 Responses/Gemini/OpenAI 测试 | NexusTok 使用 `service/openaicompat/*` 且测试较少 | 逐步整理转换包命名和测试，不一次性改热路径。 |
| OpenAI Realtime / image edit / image stream 等 relay 文件 | `relay/channel/openai/relay_realtime.go`、`relay_image.go` | 缺少部分文件 | 逐个核对上游协议支持，再按 channel 能力原生接入。 |
| Advanced Custom Channel | `relay/channel/advancedcustom/adaptor.go` | 缺少 | 可作为高级渠道改写能力参考，但必须接入 NexusTok 的参数覆盖、安全过滤和计费快照。 |
| Waffo Pancake SDK 绑定体验 | `waffo-pancake-sdk-go`、catalog/pair/save/product 接口 | NexusTok 有自研 Waffo Pancake 充值 | 吸收“自动验证凭据、拉取商店/商品、创建配对”的 UX，不强制替换当前支付链路。 |
| 订阅余额支付 | `SubscriptionRequestBalancePay`、`allow_balance_pay` | NexusTok 订阅购买缺少余额支付入口 | 可原生化为钱包余额购买订阅，并明确余额不足、退款和订阅重置语义。 |
| 订阅钱包溢出控制 | `allow_wallet_overflow` | NexusTok 缺少 | 用于控制订阅额度耗尽后是否继续消耗钱包，建议加入计划字段。 |
| 用户权限回传 | `user.AdminPermissions = authz.Capabilities(...)` | 缺少 | 默认前端可据此隐藏不可操作按钮，减少 403 弹窗。 |

## 默认前端页面差异

### 两边共有页面

以下路由两边都存在，功能大体一致，但具体实现有差异：

| 路由 | 说明 | 主要差异 |
|------|------|----------|
| `/` | 首页 | 两边都有公开首页，NexusTok 品牌和样式独立。 |
| `/setup` | 初始化向导 | new-api-main 对 setup POST 增加匿名请求体限制。 |
| `/sign-in`、`/login` 等认证页 | 登录 | 两边都有，new-api-main 额外保留 `(auth)/register.tsx` 路由文件。 |
| `/sign-up`、`/register` | 注册 | 两边都有，new-api-main 多一个兼容 register 文件。 |
| `/forgot-password`、`/reset`、`/user/reset` | 密码重置 | new-api-main 匿名入口有限流和请求体限制。 |
| `/oauth/:provider` | OAuth 回调 | 两边都有统一 provider 路由。 |
| `/dashboard/overview`、`/dashboard/models` | 概览/仪表盘 | new-api-main 多流量分析组件，NexusTok 已有模型和总览看板。 |
| `/keys` | API Keys | 两边都有。 |
| `/usage-logs/common`、`/usage-logs/task` | 使用日志/任务日志 | new-api-main 多移动端卡片和统一筛选工具条；NexusTok 账号池日志更强。 |
| `/wallet` | 钱包/充值 | NexusTok 已有 Waffo Pancake 充值；new-api-main 订阅支付和产品绑定更完善。 |
| `/profile` | 个人设置 | 两边都有 2FA、Passkey、绑定能力。 |
| `/channels` | 渠道管理 | NexusTok 多账号池绑定弹窗和渠道账号；new-api-main 多权限控制、卡片移动端、表单拆分。 |
| `/models/metadata` | 模型管理 | 两边都有；new-api-main 的模型设置多路由可靠性 section。 |
| `/redemption-codes` | 兑换码 | 两边都有。 |
| `/subscriptions` | 订阅管理 | new-api-main 多订阅重置弹窗、余额支付、钱包溢出字段。 |
| `/system-settings/*` | 系统设置 | 两边都有；new-api-main 的 models/settings 子页更偏平台治理，NexusTok 多定价独立页。 |
| `/playground` | Playground | new-api-main 已重构为 chat/input/message/options/storage 子目录，NexusTok 仍是较扁平实现。 |
| `/chat/:chatId`、`/chat2link` | Chat 预设 | 两边都有。 |
| `/pricing`、`/pricing/:modelId` | 模型价格公开页 | 两边都有；NexusTok 多模型详情能力拆分组件。 |
| `/rankings` | 排行榜 | 两边都有；new-api-main 通过 HeaderNavModuleAuth 可控制公开/登录访问。 |
| `/about`、`/privacy-policy`、`/user-agreement` | 公开文档页 | 两边都有。 |

### NexusTok 默认前端独有页面

| 页面/路由 | 文件 | 说明 | 保留/完善建议 |
|-----------|------|------|----------------|
| `/account-pool` | `routes/_authenticated/account-pool/index.tsx` | 账号池默认入口，会重定向到默认分区。 | 保留为管理员侧边栏一级能力。 |
| `/account-pool/:section` | `routes/_authenticated/account-pool/$section.tsx` | 账号池多分区页面。 | 分区应继续保持 `overview`、`credentials`、`groups`、`history` 四段。 |
| Account Pool / Overview | `features/account-pool` | 账号池健康和异常概览。 | 可接入系统任务与系统实例状态，展示检测任务是否滞后。 |
| Account Pool / Credentials | `features/account-pool/components/auth-files-panel.tsx` | 管理全局凭证/认证文件。 | 明确 AuthFile 与 PoolAccount 的区别，避免误删。 |
| Account Pool / Groups | `features/account-pool` | 管理账号池分组和组内账号。 | 后续接入细粒度权限、批量安全验证和系统任务。 |
| Account Pool / History | `features/account-pool` | 使用日志、状态日志、检测任务历史。 | 与全局审计统一入口，但保留账号池专用筛选。 |
| `/pricing-settings` | `routes/_authenticated/pricing-settings/index.tsx` | Root 专用组倍率和工具价格页。 | 保留为 NexusTok 计费管理入口，并补齐表达式/请求规则的可视化能力。 |

注意：`web/default/src/features/request-rules` 目前只是空目录，没有路由和页面入口，不能视为已实现功能。

### new-api-main 默认前端独有页面

| 页面/路由 | 文件 | 说明 | 原生化建议 |
|-----------|------|------|------------|
| `/system-info` | `routes/_authenticated/system-info/index.tsx`、`features/system-info/*` | Root 查看实例心跳和系统任务。 | P1 引入，命名保持“系统信息”，内容接入 NexusTok 节点和账号池任务。 |
| `(auth)/register.tsx` | `routes/(auth)/register.tsx` | 注册路由兼容文件。 | 低优先级，仅在需要兼容旧链接时添加。 |

### 默认前端功能模块差异

| 模块 | NexusTok 状态 | new-api-main 状态 | 原生化建议 |
|------|---------------|------------------|------------|
| Channels | 有账号池绑定、Codex OAuth 弹窗、渠道账号池管理 | 有移动端 `channel-card`、表单 section 拆分、权限上下文、Advanced Custom 编辑器 | 合并方向：保留 NexusTok 账号池能力，吸收 new-api-main 的表单拆分和移动端卡片。 |
| Dashboard | 有 overview/models/users 等面板 | 额外有 `dashboard/components/flow` 和 flow selection 测试 | 引入 `/api/data/flow` 后再接入流量 Sankey，不先做纯前端页面。 |
| Playground | 扁平组件结构，已有消息/输入/存储能力 | 更细拆成 `chat/`、`input/`、`message/`、`options/`、`streaming/`、`storage/` | 逐步重构，不改变接口协议；先迁移错误展示、消息编辑、stream utils。 |
| Usage Logs | 有通用/绘图/任务日志 | 额外有 `logs-filter-toolbar`、移动端卡片、schema | 先吸收移动端卡片和筛选工具条，再把账号池日志也纳入一致交互。 |
| Subscriptions | 有计划管理 | 额外有 reset dialog、余额支付、钱包溢出、Waffo Pancake 产品字段 | 需要后端字段先落地，再改表单。 |
| System Settings / Models | 缺少 routing-reliability section | 有 `RoutingReliabilitySection` | 将重试、自动禁用、自动启用、自动测试统一挪到“路由可靠性”。 |
| System Settings / Request Limits | 缺少 `token-limit-section.tsx` | 有 token limit 设置页 | 后端已有全局 max token 边界时再做设置页。 |
| System Settings / Integrations | 有 Waffo Pancake 基础设置 | new-api-main 有 catalog、pair、product options 体验 | 保留 NexusTok 支付配置字段，增加“验证并绑定商品”的向导。 |
| Layout Sidebar | NexusTok 侧边栏有 Account Pool、Group & Tool Pricing | new-api-main 侧边栏有 System Info | 合并后 Admin 区建议顺序：Channels、Account Pool、Models、Pricing、Users、Redemptions、Subscriptions、System Info、System Settings。 |

## 经典前端页面差异

| 页面 | NexusTok | new-api-main | 建议 |
|------|----------|--------------|------|
| `/console/account-pool` | 有 `pages/AccountPool/index.jsx` 和侧边栏入口 | 无 | 保留，作为 classic 兼容管理入口。 |
| `/console/pricing-setting` | 有 `pages/PricingSetting/index.jsx`，Root 可见 | 无 | 保留，避免 classic 管理员丢失组倍率/工具价格设置。 |
| 渠道弹窗 | 有 `ChannelAccountPoolModal.jsx`、`CodexOAuthModal.jsx` | 无账号池弹窗 | 保留 NexusTok 能力，后续只做维护性修复。 |
| ClassicFrontendDeprecationBanner | 无 | 有 | 可低优先级引入，用于提示 classic 前端逐步退场。 |
| `helpers/frontendTheme.js` | 无 | 有 | 如果要统一 classic/default 主题变量，可参考；不是核心能力。 |
| Midjourney 命名 | `Midjourney` | `MjProxy` | 无需迁移，保持本项目命名一致。 |

## 通用前端组件与工程工具差异

| 能力 | NexusTok 独有/较强 | new-api-main 独有/较强 | 建议 |
|------|------------------|-----------------------|------|
| DataTable | NexusTok 有旧版 `components/data-table/*` | new-api-main 有 README、static/core/layout/mobile card、更完整抽象 | 逐步统一到 new-api-main 的 DataTable 分层，避免一次性替换所有表格。 |
| Rich Content / Markdown | NexusTok 有 `react-markdown`、`streamdown` | new-api-main 有 `rich-content.tsx`、`html-content.tsx`、`json-code-editor.tsx`、CodeMirror/KaTeX/DOMPurify | 对日志详情、模型说明、Playground 输出优先引入富文本安全渲染。 |
| AI Elements | new-api-main response renderer 更完整 | NexusTok 目前已有部分 ai-elements | 优先迁移 response renderer 的安全解析和表格/图片/details 渲染。 |
| Dialog/Drawer Layout | NexusTok 使用项目现有组件 | new-api-main 有 `dialog.tsx`、`drawer-layout.ts` 公共布局 | 可吸收为长表单统一布局，尤其渠道和账号池编辑器。 |
| Sidebar View | NexusTok 无 `use-sidebar-view.ts` | new-api-main 有 | 如果账号池、系统设置、模型设置继续多层导航，可引入嵌套侧边栏视图。 |
| 前端缓存 | NexusTok 无 `frontend-cache.ts` | new-api-main 有 | 可用于状态接口、系统配置和低频元数据缓存，需避免缓存权限敏感数据。 |
| 构建/检查 | NexusTok 使用 ESLint/Prettier/tsc | new-api-main 使用 oxlint/oxfmt/tsgo | 不建议立刻切换；可单独评估速度收益，避免扰动现有格式化规则。 |

## 后端依赖和基础设施差异

| 类别 | NexusTok | new-api-main | 建议 |
|------|----------|--------------|------|
| Go module | `github.com/c1cada/NexusTok` | `github.com/QuantumNous/new-api` | 禁止直接复制 import，迁移时必须改为 NexusTok 包路径。 |
| Casbin | 无 | `github.com/casbin/casbin/v2`、`casbin_rule.go` | 引入权限系统时再加依赖，不提前污染。 |
| ClickHouse | 无 | `gorm.io/driver/clickhouse` | 仅当确定要支持 ClickHouse 日志库时引入。 |
| Waffo Pancake SDK | `github.com/waffo-com/waffo-go v1.3.1` | `waffo-go v1.3.2` + `waffo-pancake-sdk-go` | 先升级/补 SDK 适配层，保持现有充值接口兼容。 |
| NTLM 邮件 | 无 | `github.com/Azure/go-ntlmssp`、`common/email_ntlm_auth.go` | 低优先级，只有企业 SMTP 需要 NTLM 时再引入。 |
| 安全数学 | 无集中 `quota_math.go` | 有 `common/quota_math.go` | 高优先级，计费和任务结算必须统一使用。 |
| 请求体限制 | 无 | 有匿名请求体限制 | 高优先级，适合快速原生化。 |

## 原生化路线建议

### P0：保护并完成 NexusTok 已有账号池主线

1. 保持 `AccountPoolGroup`、`PoolAccount`、`AccountPoolAuthFile` 为原生一等模型。
2. 不引入外部 sub2api/CPAMC/Sidecar 作为运行依赖。
3. 将账号池检测任务从独立实现逐步迁移到统一 SystemTask，但 API 响应应兼容现有前端。
4. 账号池导出、状态日志和使用日志继续脱敏，不向通用审计泄露完整凭据。
5. 渠道表单继续以 `credential_mode` + `account_pool_group_id` 表示账号池模式。

### P1：平台治理能力

1. 引入 Authz：
   - 资源建议：`channel`、`account_pool`、`user`、`model`、`subscription`、`system_setting`、`audit_log`。
   - 动作建议：`read`、`operate`、`write`、`sensitive_write`、`export`。
   - 先让 Root 拥有全部权限，Admin 默认拥有非敏感操作，再支持用户级 override。
2. 拆分路由注册：
   - 学习 `new-api-main/router/channel-router.go`，把渠道和账号池从巨型 `api-router.go` 抽出。
   - 每条写接口绑定权限常量，补 route permission 测试。
3. 引入全局操作审计：
   - 账号池仍保留专用状态日志。
   - 其它管理写操作走中间件兜底审计，使用稳定 action + params，前端本地化展示。

### P1：统一后台任务和系统信息

1. 引入 `SystemTask`、`SystemTaskLock`、`SystemInstance` 三个模型，并确认 SQLite/MySQL/PostgreSQL AutoMigrate 兼容。
2. 新增 `/api/system-task/list`、`/api/system-task/current`、`/api/system-task/:task_id`。
3. 新增 `/api/system-info/instances` 和默认前端 `/system-info` 页面。
4. 迁移候选任务：
   - 账号池批量检测；
   - 日志清理；
   - 批量渠道测试；
   - 上游模型同步；
   - 订阅周期重置；
   - Midjourney/异步任务轮询。
5. 系统任务 runner 只在主节点调度，但允许多节点心跳展示所有实例。

### P1：安全边界

1. 增加匿名请求体限制，并挂到 setup、register、login、2FA login、passkey login、password reset、OAuth bind、Webhook。
2. 增加 SSRF 保护客户端，所有外部 URL 拉取必须走统一 client。
3. 增加 HeaderNavModuleAuth，让公开页面的可见性和接口访问权限一致。
4. 为渠道和账号池敏感字段做 fail-closed 分类，新增字段未分类时默认需要 `sensitive_write`。

### P2：计费和额度安全

处理计费表达式或动态计费时必须继续遵循 `pkg/billingexpr/expr.md` 的“一条表达式定义计费合同”原则。

建议从 `new-api-main/common/quota_math.go` 吸收以下模式：

1. 增加 `QuotaFromFloat`、`QuotaRound`、`QuotaFromDecimal` 及 `*Checked` 版本。
2. 所有计费乘法、表达式结算、图片/视频/音频倍率、任务结算不得裸转 `int`。
3. 发生 overflow/underflow/NaN 时，结果饱和到 int32 范围，并写入后台错误日志。
4. `*Checked` 返回的 `QuotaClamp` 应写入消费日志 `other.admin_info.quota_saturation`，只对管理员可见。
5. 前端日志详情应能展示 quota saturation，方便排查异常请求或攻击流量。

### P2：订阅和支付体验

1. 增加订阅余额支付 `POST /api/subscription/balance/pay`，复用钱包扣费和订阅创建事务。
2. 增加 `allow_balance_pay` 和 `allow_wallet_overflow`：
   - `allow_balance_pay` 控制是否允许用余额购买套餐；
   - `allow_wallet_overflow` 控制套餐额度用尽后是否继续消耗钱包。
3. Waffo Pancake 保留 NexusTok 当前充值链路，吸收 new-api-main 的 catalog/pair/product 绑定向导，减少手填 Store/Product ID 的错误。
4. 订阅管理页补齐重置计划/重置用户订阅能力，并走 SystemTask 或明确的同步事务边界。

### P2：仪表盘和日志体验

1. 引入 `/api/data/flow` 后，再接入仪表盘流量 Sankey 图。
2. 使用日志页吸收 `logs-filter-toolbar` 和移动端卡片，先覆盖普通日志，再覆盖账号池日志。
3. 模型设置新增“路由可靠性”页面，把自动禁用、自动启用、重试状态码、自动测试集中管理。
4. 公共 DataTable 逐步向 new-api-main 的 core/layout/static 分层靠拢，避免每个页面重复实现移动端卡片。

### P3：Relay 和 Playground 演进

1. 逐步核对 `new-api-main/relay` 中缺失的 OpenAI Realtime、Image Edit/Image Stream、Gemini Responses、Advanced Custom adaptor。
2. 每引入一个 channel 或 relay 格式，都必须检查 StreamOptions 支持并加入 `streamSupportedChannels`。
3. Relay 请求 DTO 可选标量继续使用 `*int`、`*float64`、`*bool` + `omitempty`，保留显式零值。
4. Playground 可优先迁移 streaming 错误解析、消息编辑、输入控制和本地存储 schema，不一次性重写页面。

## 不建议直接迁移的内容

| 内容 | 原因 | 处理 |
|------|------|------|
| `new-api-main` 品牌/README/受保护标识 | 与 NexusTok 项目身份不一致 | 不迁移。 |
| 整个 classic 前端退场提示 | 当前 NexusTok classic 仍承载账号池和计费设置 | 等默认前端完全覆盖后再提示。 |
| ClickHouse 依赖 | 会增加部署和测试矩阵 | 先作为可选日志库设计，不进主链路。 |
| oxlint/oxfmt/tsgo 工具链 | 会影响现有前端格式和 CI 习惯 | 单独评估，避免混入功能迁移。 |
| 一次性替换 Playground | 风险高，容易影响调试和 Relay 验证 | 按组件和工具函数渐进迁移。 |
| 一次性替换 DataTable | 页面多、回归成本高 | 新页面先用新结构，旧页面逐步改。 |

## 建议的落地顺序

1. 文档和设计冻结：本文件 + `account-pool-roadmap.md` 作为账号池和平台化能力的共同路线。
2. 安全小步快跑：匿名请求体限制、HeaderNavModuleAuth、SSRF 客户端、QuotaMath helper。
3. 系统任务/系统信息：先后端模型和接口，再 `/system-info` 页面。
4. 权限系统：先 channel/account_pool 两个资源，再扩展到用户、模型、订阅、设置。
5. 订阅/支付增强：余额支付、钱包溢出、Waffo Pancake 商品绑定。
6. 仪表盘和日志体验：flow 数据、移动端卡片、统一筛选工具条。
7. Relay/Playground 深水区：逐协议引入，逐项测试，不做大爆炸迁移。

## 验收建议

每个功能点迁移时至少满足：

1. 后端使用 `common.Marshal`/`common.Unmarshal`，不直接调用 `encoding/json` marshal/unmarshal。
2. 数据库迁移同时支持 SQLite、MySQL、PostgreSQL。
3. 涉及计费表达式或额度换算时，先重读 `pkg/billingexpr/expr.md`。
4. 前端新增文案走 i18n，默认前端用 Bun 执行 `bun run typecheck` 和必要的 `bun run i18n:sync`。
5. 涉及页面、交互、接口或联调时，用 MCP 浏览器真实验证页面、请求、响应和控制台。
6. 每个独立功能点单独提交，提交信息使用中文。
