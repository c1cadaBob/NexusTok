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
2. 权限系统应优先围绕账号池、渠道、用户、模型、订阅、兑换码、日志、用量数据和系统设置做资源级授权，而不是只有 Admin/Root 粗粒度角色。
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
| 账号检测任务 | `POST /api/account-pool/groups/:id/accounts/check-tasks`、`GET /api/account-pool/check-tasks`、`account_pool_check` SystemTask | 缺少 | 已迁入统一 SystemTask 执行层；继续保留账号池专用任务历史和脱敏明细接口，避免破坏现有页面契约。 |
| 账号批量状态/删除/导出 | `/groups/:id/accounts/status`、`delete`、`export` | 缺少 | 保留，后续加细粒度权限和安全验证。 |
| 账号池 OAuth/设备授权 | `/groups/:id/oauth/:provider/start`、`/device/:provider/start`、Codex OAuth | 缺少 | 作为 NexusTok 原生凭据接入入口，后续抽象为 provider registry。 |
| 渠道账号 | `model/channel_account.go`、`controller/channel_account.go`、`service/channel_account_*` | 缺少 | 与账号池区分：渠道账号适合单渠道多 Key，账号池适合跨渠道共享和调度。 |
| 渠道账号池绑定 | `constant/channel_credential_mode.go`、渠道 `account_pool_group_id` | 缺少 | 保留为 Relay 热路径原生能力，后续统一校验分组状态、平台和认证类型。 |
| Codex 渠道 OAuth | `controller/codex_oauth.go`、`service/codex_oauth.go` | 渠道侧用量重置已落地 | 保留 NexusTok 的渠道和账号池双入口；渠道侧已补齐 `/codex/usage/reset-credits` 与 `/codex/usage/reset` 后端 API，并已在默认前端 Codex 用量弹窗接入 reset credits 明细、确认重置和重置后刷新。Classic 前端同等体验可低优先级补齐。 |
| 账号池任务提交级限制 | `service/account_pool_task_limit.go`、`RelayTaskSubmit`、账号池组 `task_*` 字段 | 缺少 | 已原生化为异步任务提交级并发/RPM/等待策略，按 `account_pool_group_id + platform + action` 生效，并在预扣费前拦截；后续再评估完整持久化队列和任务完成后释放账号占用。 |
| 模型定价配置表 | `model/model_pricing_config.go` | 缺少 | 保留，用于承载 NexusTok 独立计费配置和后续表达式定价演进。 |
| 账号池文档 | `docs/features/account-pool.md`、`account-pool-roadmap.md` | 缺少同等文档 | 继续作为账号池开发准绳。 |

### new-api-main 已有而 NexusTok 缺少或较弱

| 功能 | new-api-main 文件/接口 | NexusTok 状态 | 原生化建议 |
|------|------------------------|---------------|------------|
| 细粒度授权 Authz | `service/authz/*`、`model/authz_role.go`、`model/casbin_rule.go`、`router/authz-router.go`、`GET /api/authz/catalog` | catalog、自身份权限回传、默认前端入口/按钮消费与渠道/账号池/订阅 Admin 路由 enforcement 已持续落地 | 已新增 NexusTok 原生权限 catalog，覆盖渠道、账号池、用户、模型、订阅、兑换码、日志、用量数据和系统设置九类资源，并返回 Root/Admin 基线矩阵；`/api/user/self` 已在 `permissions.admin_permissions` 回传同一矩阵，默认前端已让管理入口、Usage Logs、Dashboard 和渠道页关键按钮消费该矩阵；`/api/channel`、`/api/account-pool`、`/api/subscription/admin`、`/api/models`、`/api/user`、`/api/redemption`、`/api/log` 和 `/api/data` 已按同一矩阵做服务端二次校验。当前仍基于系统角色基线；Casbin 存储、用户 override 和系统设置等更多路由 enforcement 待后续分批接入。 |
| 渠道路由权限表 | `router/channel-router.go`、`middleware.RequirePermission` | 已落地首个 enforcement 切片 | `/api/channel` 已迁移为权限表注册，读、操作、写、敏感写和密钥查看分别挂接 `authz.Channel*` permission；原有 `AdminAuth`、`RootAuth`、安全验证、限流和禁缓存边界继续保留。后续账号池路由也应采用同样模式。 |
| 渠道敏感字段 fail-closed | `controller/channel_authz.go` | 已部分落地 | 已先为渠道更新接口建立敏感/非敏感/操作/只读字段分类，未知字段默认敏感；敏感写二次校验已改为 `authz.Can(..., ChannelSensitiveWrite)`，当前基线仍等价 Root 权限。账号池路由已先按接口级敏感度收紧，字段级分类待后续跟进。 |
| 管理操作审计兜底 | `middleware/audit.go`、`controller/audit.go` | 账号池有状态审计，但全局管理审计较弱 | 保留账号池专用审计，同时新增全局操作审计兜底，用 action + params 支持前端 i18n。 |
| 系统任务中心 | `model/system_task.go`、`service/system_task.go`、`controller/system_task.go`、`controller/system_task_handlers.go`、`/api/system-task/*` | 后端模型、租约锁、runner、Root 查询接口、日志清理任务、批量渠道测试任务、上游模型同步任务、账号池检测任务、订阅维护任务、Midjourney/通用异步任务轮询和默认前端任务面板已落地 | 继续把后续新增长耗时后台动作统一接入 SystemTask，并在 `/system-info` 统一观测。 |
| 系统实例心跳 | `model/system_instance.go`、`service/system_instance.go`、`GET /api/system-info/instances` | 后端心跳、Root 只读接口和默认前端 `/system-info` 实例面板已落地 | 已引入 `NODE_NAME`/主机名兜底、主从节点、CPU/内存/磁盘/版本心跳、实例页面和同页 SystemTask 任务面板。 |
| 后台任务锁 | `SystemTaskLock`、租约续期、过期失败标记 | 已按 NexusTok 三库兼容模型原生化 | 已用数据库锁实现跨节点互斥、租约续期、过期失败标记和锁丢失保护；后续任务 handler 要支持 context cancellation。 |
| 匿名请求体限制 | `common/request_body_limit.go`、`middleware/request_body_limit.go` | 已落地 | 已用于注册、登录、setup、OAuth 绑定、Webhook 等匿名入口，默认 512KB，可用 `ANONYMOUS_REQUEST_BODY_LIMIT_KB=0` 禁用。 |
| 顶栏模块鉴权 | `middleware/header_nav.go` | 已落地 | 已让 `/api/pricing`、`/api/rankings` 和 pricing 辅助性能接口跟随 `HeaderNavModules.enabled/requireAuth`，页面隐藏和接口鉴权一致。 |
| 额度饱和保护 | `common/quota_math.go`、`QuotaClamp`、相关测试 | 已部分落地 | 已新增 NexusTok 原生 `common/quota_math.go`，接入表达式计费、文本/音频/WSS 结算、标准预扣、按次预扣、任务差额结算、工具调用附加费、违规费用、充值入账、视频任务重算和渠道测试日志；饱和事件已写入 `other.admin_info.quota_saturation`。 |
| 受保护 Fetch / SSRF | `service/protected_fetch_client.go` | 已部分落地 | 已新增用户可控 URL 专用 protected fetch client，并接入下载、Webhook、Bark/Gotify 通知；Relay 全局 client 暂不替换，避免误伤内网模型渠道。 |
| ClickHouse 日志兼容测试 | `model/clickhouse_log_test.go`、`gorm.io/driver/clickhouse` | 缺少依赖 | 仅在明确支持 ClickHouse 日志库时引入；否则先记录为可选能力，避免增加部署复杂度。 |
| 流量账本查询 | `model/usedata_flow.go`、`controller.GetAllFlowQuotaDates`、`/api/data/flow` | 后端已落地 | 已扩展 `quota_data` 记录 node/token/group/channel 维度，并新增 `/api/data/flow`、`/api/data/flow/self`；后续接入仪表盘 Sankey 图。 |
| Relay 转换包整理 | `service/relayconvert/*`、更多 Responses/Gemini/OpenAI 测试 | NexusTok 使用 `service/openaicompat/*` 且测试较少 | 逐步整理转换包命名和测试，不一次性改热路径。 |
| OpenAI Realtime / image edit / image stream 等 relay 文件 | `relay/channel/openai/relay_realtime.go`、`relay_image.go` | 缺少部分文件 | 逐个核对上游协议支持，再按 channel 能力原生接入。 |
| Advanced Custom Channel | `relay/channel/advancedcustom/adaptor.go` | 缺少 | 可作为高级渠道改写能力参考，但必须接入 NexusTok 的参数覆盖、安全过滤和计费快照。 |
| Waffo Pancake SDK 绑定体验 | `waffo-pancake-sdk-go`、catalog/pair/save/product 接口 | NexusTok 有自研 Waffo Pancake 充值 | 吸收“自动验证凭据、拉取商店/商品、创建配对”的 UX，不强制替换当前支付链路。 |
| 订阅余额支付 | `SubscriptionRequestBalancePay`、`allow_balance_pay` | 后端已落地 | 已新增 `/api/subscription/balance/pay`，在模型事务内完成钱包扣款、订阅创建和成功订单落账；`allow_balance_pay` 控制套餐是否允许余额购买。前端按钮/表单待接入。 |
| 订阅钱包溢出控制 | `allow_wallet_overflow` | 后端已落地 | 已在套餐与用户订阅快照保存钱包溢出策略；`subscription_first` 下订阅额度不足时，任一活跃订阅禁止溢出即阻断钱包 fallback。前端管理字段待接入。 |
| 用户权限回传 | `user.AdminPermissions = authz.Capabilities(...)` | 后端、默认前端入口消费与渠道页按钮级消费已落地 | `/api/user/self` 已返回 `permissions.admin_permissions`，默认前端已新增权限 helper/hook，并让管理侧边栏与渠道、账号池、模型、用户、订阅等入口路由按 read 能力判断；渠道管理页已按 `write`、`operate`、`sensitive_write`、`secret_view` 控制新建、编辑、测试、启停、批量操作、密钥查看、多 Key、渠道账号池和上游模型应用入口。当前矩阵来自系统角色基线，用户 override 和服务端 enforcement 待后续接入。 |

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
| `(auth)/register.tsx` | `routes/(auth)/register.tsx` | 注册路由兼容文件。 | 低优先级，仅在需要兼容旧链接时添加。 |

`/system-info` 已在 NexusTok 默认前端落地，因此不再作为 `new-api-main` 独有页面统计；当前差异转为同路径实现细节和后续任务类型接入差异。

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
| 安全数学 | 已有 `common/quota_math.go`，主要扣费链路开始接入 | 有 `common/quota_math.go` | 已继续覆盖工具调用附加费、违规费用、充值入账、视频任务重算和渠道测试日志；后续覆盖异步任务倍率调整等剩余裸 `int(...)` 计费转换。 |
| 请求体限制 | 无 | 有匿名请求体限制 | 高优先级，适合快速原生化。 |

## 文档、配置与测试资产差异补充

本轮对文档、部署、配置、OpenAPI、README、脚本和测试资产做了补充核对，结论如下：

1. OpenAPI 路径层面基本齐平：`docs/openapi/api.json` 均为 131 个 paths，`docs/openapi/relay.json` 均为 35 个 paths；当前重点不是接口补齐，而是生成/校验流程和品牌化维护。
2. NexusTok 已覆盖或重组了多项 new-api-main 文档：i18n 术语表迁入 `docs/i18n/`，渠道 other settings 迁入 `docs/configuration/channel-other-settings.md`，`ionet-client.md` 内容已覆盖，不应重复搬运。
3. new-api-main 仍有值得吸收的文档资产：
   - 多语言 README：`README.en.md`、`README.zh_CN.md`、`README.zh_TW.md`、`README.fr.md`、`README.ja.md`。NexusTok 应生成本项目品牌化版本，而不是直接复制。
   - `electron/README.md`：适合补成 NexusTok 桌面端开发/打包说明。
   - `constant/README.md`：适合补充 constant 包维护约束，说明该包不应依赖业务层。
   - `web/default/src/components/data-table/README.md`：适合在后续统一 DataTable 分层时补齐本地组件说明。
   - `.github/ISSUE_TEMPLATE/*`：new-api-main 的 issue 前置约束更清晰，可品牌化吸收。
4. 配置/脚本资产方面，new-api-main 的 `docker-compose.dev.yml`、`makefile`、`web/package.json`、前端格式化保护脚本和任意分支 Docker 镜像工作流可以作为工程化增强参考；其中 PostgreSQL dev compose 镜像声明、开发任务和 i18n literal 白名单最适合优先评估。
5. 测试资产方面，new-api-main 仍有高价值回归测试可转化：
   - `service/relayconvert/*` 的 Responses/Chat 转换与显式零值测试。
   - `relay/channel/openai/*` 的 Responses、image stream、image edit 测试。
   - `relay/channel/gemini/*` 的 Gemini Responses 测试。
   - `pkg/billingexpr/settle_clamp_test.go` 的计费表达式溢出 clamp 测试。
   - `model/user_update_test.go`、`model/redemption_test.go`、`model/locking_test.go`、`setting/operation_setting/monitor_setting_test.go` 等三库/权限/并发边界测试。
   - `web/default/src/features/dashboard/lib/flow*.test.ts`，后续迁移 dashboard flow/Sankey 前应先吸收纯函数测试。

## 子 Agent 复核后的后续优先队列

本轮并行只读分析进一步确认：NexusTok 不能目录级覆盖 `new-api-main`，应把上游优势按安全边界和原生业务语义分批吸收。

### 后端 P0/P1

| 优先级 | 能力 | new-api-main 参考 | NexusTok 风险/迁移方式 |
|--------|------|-------------------|-------------------------|
| P0 | GORM v2 行锁 helper | `model/locking.go` | NexusTok 多处仍使用 `tx.Set("gorm:query_option", "FOR UPDATE")`，在 GORM v2 下存在被忽略风险；应新增三库兼容 `lockForUpdate(tx)`，逐步替换订阅、充值、兑换码、用户额度等事务热点，并补并发测试。 |
| P1 | OpenAI Responses 反向兼容 | `service/relayconvert/responses_request_to_chat.go`、`relay/channel/openai/responses_via_chat.go` | NexusTok 现有 `service/openaicompat` 只覆盖部分方向；建议保留本项目包名，补 Responses→Chat 与 Chat→Responses 转换，不改热路径包结构。 |
| P1 | Gemini Responses | `relay/channel/gemini/adaptor_responses.go`、`relay/channel/gemini/relay_responses.go` | 依赖 Responses/Chat 通用转换，迁移时必须明确 custom/freeform tool 的降级或拒绝策略。 |
| P1/P2 | 管理操作审计兜底 | `middleware/audit.go`、`controller/audit.go` | NexusTok 已有账号池专项审计和额度饱和审计；应新增全局管理写操作审计，但 action 表必须覆盖账号池、渠道账号、Codex OAuth、订阅、SystemTask 等本项目独有路由。 |
| P1/P2 | 完整 Authz/Casbin enforcement | `service/authz/*`、`model/casbin_rule.go`、`router/channel-router.go` | NexusTok 已有 catalog 和前端消费矩阵；下一步应先定义本项目原生资源：`channel_account`、`account_pool_auth_file` 等，再灰度接入服务端路由 enforcement。 |
| P2 | Advanced Custom Channel | `relay/channel/advancedcustom/*`、`dto/channel_settings.go` | 高价值但高风险；必须纳入 Root/敏感写权限、SSRF/URL 安全、凭证脱敏和计费快照，不能直接开放给普通 Admin。 |
| P2 | Waffo Pancake SDK 与订阅支付 | `service/waffo_pancake.go`、`controller/subscription_payment_waffo_pancake.go` | NexusTok 当前手写充值链路需继续兼容；可先新增 SDK 路径和订阅支付灰度入口，保留旧 webhook 幂等。 |

### 前端 P0/P1

| 优先级 | 能力 | new-api-main 参考 | NexusTok 迁移方式 |
|--------|------|-------------------|-------------------|
| P0 | 会话守卫仅 401 登出 | `_authenticated/route.tsx` | NexusTok 当前 `getSelf()` 失败更容易误登出；应只在 401 清登录，网络错误/5xx 暂时放行并后续重验。 |
| P0/P1 | HeaderNavModules 健壮解析 | `lib/nav-modules.ts`、`hooks/use-top-nav-links.ts` | 吸收 boolean/number/string/object 兼容解析，避免 `"0"`、`"false"` 被误判；需保留 NexusTok 当前 `disabled`/权限链路。 |
| P1 | Usage Logs 移动端卡片与筛选工具条 | `usage-logs-mobile-card.tsx`、`logs-filter-toolbar.tsx` | 后端依赖小，适合先迁普通使用日志，再统一账号池日志体验。 |
| P1 | 渠道编辑抽屉分区化 | `channels/components/drawers/sections/*` | 保留 NexusTok `credential_mode`、账号池、Codex OAuth、渠道账号字段，只吸收分区导航、表单错误收敛和敏感字段提交裁剪。 |
| P1/P2 | Dashboard Flow/Sankey | `dashboard/components/flow/*`、`dashboard/lib/flow.ts` | NexusTok 后端已原生化 `/api/data/flow*`，前端迁移前应先吸收 `flow*.test.ts` 纯函数测试。 |
| P2 | Playground 消息/流式/Markdown 渲染增强 | `features/playground/*`、`components/ai-elements/response-renderer.tsx` | 价值高但依赖和安全面大，需评估 DOMPurify/KaTeX/CodeMirror 包体与 XSS 边界。 |
| P2/P3 | DataTable 分层 | `components/data-table/core/*`、`layout/*`、`toolbar/*` | 全局影响大，不做一次性替换；新页面先兼容导出，旧页面逐步迁移。 |

### 文档与工程化 P1/P2

| 优先级 | 能力 | 参考路径 | 迁移方式 |
|--------|------|----------|----------|
| P1 | 多语言 README | `README.*.md` | 生成 NexusTok 品牌化多语言 README，补根 README 语言入口。 |
| P1 | Electron/constant 包文档 | `electron/README.md`、`constant/README.md` | 补桌面端开发/打包说明和 constant 包边界说明。 |
| P1 | 开发 Compose/Makefile 修补 | `docker-compose.dev.yml`、`makefile` | 优先核对 PostgreSQL dev 镜像声明、`dev-api-rebuild`、`reset-setup` 等开发任务，避免破坏现有热更新部署。 |
| P2 | Issue 模板与 CI 工作流 | `.github/ISSUE_TEMPLATE/*`、`.github/workflows/docker-image-branch.yml` | 品牌化吸收 issue 前置约束；任意分支镜像工作流只在确有临时预览需求时引入。 |

## 原生化路线建议

### P0：保护并完成 NexusTok 已有账号池主线

1. 保持 `AccountPoolGroup`、`PoolAccount`、`AccountPoolAuthFile` 为原生一等模型。
2. 不引入外部 sub2api/CPAMC/Sidecar 作为运行依赖。
3. 账号池检测任务已迁移到统一 SystemTask 执行层，但 API 响应继续兼容现有前端。
4. 账号池导出、状态日志和使用日志继续脱敏，不向通用审计泄露完整凭据。
5. 渠道表单继续以 `credential_mode` + `account_pool_group_id` 表示账号池模式。

### P1：平台治理能力

1. 引入 Authz：
   - 只读 catalog 已落地：`channel`、`account_pool`、`user`、`model`、`subscription`、`system_setting`。
   - 动作已先统一为 `read`、`operate`、`write`、`sensitive_write`、`secret_view`。
   - `/api/user/self` 已回传 `permissions.admin_permissions`，默认前端已用同一 schema 过滤管理侧边栏与入口级路由守卫。
   - Root 基线拥有全部权限，Admin 基线拥有非敏感 read/operate/write；默认前端渠道页已开始消费页面内按钮和敏感字段权限，用户级 override、Casbin 存储、其它管理页面按钮消费和路由 enforcement 待后续接入。
2. 拆分路由注册：
   - 渠道路由已先从巨型 `api-router.go` 抽出到 `router/channel-router.go`，保持现有权限行为不变。
   - 后续学习 `new-api-main/router/channel-router.go` 的权限表模式，为每条写接口绑定权限常量，并补 route permission 测试。
   - 账号池路由仍在主路由文件中，待 Authz 设计稳定后再抽出。
3. 引入全局操作审计：
   - 账号池仍保留专用状态日志。
   - 其它管理写操作走中间件兜底审计，使用稳定 action + params，前端本地化展示。

### P1：统一后台任务和系统信息

1. `SystemInstance` 后端心跳、Root 只读 `/api/system-info/instances` 和默认前端 `/system-info` 实例面板已落地。
2. `SystemTask`、`SystemTaskLock` 后端模型、runner、Root 只读 `/api/system-task/list`、`/api/system-task/current`、`/api/system-task/:task_id` 已落地，并通过 SQLite 测试与 MCP 接口验证。
3. 日志清理已接入 `POST /api/system-task/log-cleanup`，由 runner 异步执行并写入进度与结果。
4. 批量渠道测试已接入 `channel_test` SystemTask：
   - `/api/channel/test` 不再在当前进程内直接启动 goroutine，而是创建 `channel_test` pending 任务并返回 `task_id`；
   - 同类型 pending/running 任务存在时返回 `409 Conflict` 和既有任务 ID，避免管理员重复触发；
   - 主节点 runner 按数据库租约执行任务，写入 `{total, processed, progress}` 进度和 tested/succeeded/failed/disabled/enabled 结果；
   - `monitor_setting.channel_test_mode` 已支持 `scheduled_all` 和 `passive_recovery`，`CHANNEL_TEST_ENABLED` 可单独控制自动测试开关。
5. 上游模型同步已接入 `model_update` SystemTask：
   - `POST /api/channel/upstream_updates/detect_all` 不再在 HTTP 请求内同步扫描，而是创建 `model_update` pending 任务并返回 `task_id`；
   - 手动任务使用 `Manual=true`，强制重新检测但不自动应用模型，继续要求管理员审阅后显式 apply；
   - 定时任务沿用 `CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED` 和 `CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_INTERVAL_MINUTES`，由主节点 runner 按数据库租约执行；
   - 执行结果写入 checked/changed/detected/failed/auto_added 汇总，任务取消时不会误标记为 succeeded；
   - MCP 已验证手动 200 入队、slave/pending 场景下重复触发 409、`/api/system-task/list` 和 `/api/system-task/:task_id` 可观测任务历史。
6. 账号池批量检测已接入 `account_pool_check` SystemTask：
   - `POST /api/account-pool/groups/:id/accounts/check-tasks` 仍返回原 `PoolAccountCheckTask` 视图，账号池页面和历史接口无需重构；
   - 每个账号池检测任务创建独立 ActiveKey 的 `account_pool_check` pending 系统任务，允许同类型多任务排队；
   - 执行租约仍按 `account_pool_check` 类型串行获取，避免多节点同时检测同一批账号；
   - 进度写入 `{total, processed, progress}`，SystemTask result 只保存 `check_task_id` 和聚合计数，账号级明细继续走账号池脱敏接口；
   - queued 任务重启恢复会重新确保系统任务入口存在，已认领但业务任务仍 queued 的旧系统任务会释放后重排。
7. 订阅维护已接入 `subscription_maintenance` SystemTask：
   - 旧 `StartSubscriptionQuotaResetTask` 不再启动独立 ticker，只负责主节点启动时创建首次 pending 系统任务；
   - 后续周期由 SystemTask scheduler 每分钟按类型去重创建，跨节点执行仍依赖数据库租约；
   - 任务阶段覆盖过期订阅、周期额度重置和预消费幂等记录清理，state 写入 phase/expired/reset/cleanup/progress，result 写入聚合计数；
   - 原有批处理函数 `ExpireDueSubscriptions`、`ResetDueSubscriptions`、`CleanupSubscriptionPreConsumeRecords` 保持 GORM 三库兼容实现。
8. Midjourney 和通用异步任务轮询已接入 SystemTask：
   - `midjourney_poll` 将旧 `UpdateMidjourneyTaskBulk` 无限循环拆成一次 pass 和周期 handler，启动入口只创建首次系统任务；
   - `async_task_poll` 将旧 `TaskPollingLoop` 无限循环拆成一次 pass 和周期 handler，保留超时清理、空上游任务 ID 修复、Suno/视频任务分发和原有计费结算路径；
   - 两类轮询均由 scheduler 每 15 秒按类型去重创建，执行期通过 `SystemTaskLock` 保证多节点互斥，并在 state/result 中记录进度、待处理数量和聚合结果；
   - `main.go` 已将任务适配器工厂注入提前到 runner 启动前，避免 scheduler 首轮执行时找不到 relay 适配器。
9. 默认前端系统任务面板已接入 `/system-info`，展示活动任务、历史任务、进度、执行节点、错误和日志清理/批量渠道测试/上游模型同步/账号池检测/订阅维护/Midjourney 轮询/异步任务轮询结果；runner 已在主节点启动，但允许多节点心跳展示所有实例。

### P1：安全边界

1. 增加匿名请求体限制，并挂到 setup、register、login、2FA login、passkey login、password reset、OAuth bind、Webhook。
2. 增加 SSRF 保护客户端，所有用户可控外部 URL 拉取必须走统一 client；Relay 上游地址保留普通 client，避免误伤合法内网渠道。
3. 增加 HeaderNavModuleAuth，让公开页面的可见性和接口访问权限一致。
4. 为渠道和账号池敏感字段做 fail-closed 分类，新增字段未分类时默认需要 `sensitive_write`；渠道更新接口已先落地字段分类和 Root 兜底，账号池表单待后续接入 Authz 时统一覆盖。

### P2：计费和额度安全

处理计费表达式或动态计费时必须继续遵循 `pkg/billingexpr/expr.md` 的“一条表达式定义计费合同”原则。

已从 `new-api-main/common/quota_math.go` 吸收基础模式，并按 NexusTok 包边界落地为 `common/quota_math.go`：

1. 已增加 `QuotaFromFloat`、`QuotaRound`、`QuotaFromDecimal`、`QuotaFromDecimalTruncated` 及 `*Checked` 版本。
2. 已将表达式计费 `billingexpr.QuotaRound`、文本/音频/WSS 实际结算、标准模型预扣、按次模型预扣、任务 token 差额结算、工具调用附加费、违规费用、充值入账、视频任务重算和渠道测试日志改为统一 helper。
3. 发生 overflow/underflow/NaN 时，结果饱和到 int32 范围，并写入后台错误日志。
4. `*Checked` 返回的 `QuotaClamp` 已写入消费/任务日志 `other.admin_info.quota_saturation`，普通用户日志视图会移除 `admin_info`。
5. 待继续覆盖异步任务倍率调整等剩余裸转换；前端日志详情后续可展示 quota saturation，方便管理员排查异常请求或攻击流量。

### P2：订阅和支付体验

1. 订阅余额支付后端已落地：`POST /api/subscription/balance/pay` 复用钱包扣费和订阅创建事务，成功后写入订阅订单和用户日志，余额不足、套餐禁用、购买上限与第三方支付入口保持一致。
2. `allow_balance_pay` 和 `allow_wallet_overflow` 后端已落地：
   - `allow_balance_pay` 控制是否允许用余额购买套餐；
   - `allow_wallet_overflow` 在购买时快照到用户订阅，控制套餐额度用尽后是否继续消耗钱包；
   - 后续需要默认前端套餐表单和用户订阅页展示这两个字段。
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

## 附录：精确差异清单

文件级全量索引见 `docs/features/new-api-main-diff-inventory.md`。主文档负责解释功能、页面和原生化路线；inventory 文档负责记录可重复生成的全量文件清单、统计口径和目录分布，避免把几千个同路径实现差异直接塞进主文档。

### 默认前端路由文件

NexusTok 独有：

| 文件 | 对应页面 | 处理 |
|------|----------|------|
| `_authenticated/account-pool/index.tsx` | `/account-pool` | 保留，账号池一级入口。 |
| `_authenticated/account-pool/$section.tsx` | `/account-pool/:section` | 保留，承载 overview/credentials/groups/history。 |
| `_authenticated/pricing-settings/index.tsx` | `/pricing-settings` | 保留，Root 组倍率和工具价格页。 |

`new-api-main` 独有：

| 文件 | 对应页面 | 处理 |
|------|----------|------|
| `(auth)/register.tsx` | `/register` 兼容注册页 | 低优先级，仅做旧链接兼容时迁移。 |

两边共有的默认前端路由文件如下，迁移时应以“保留 NexusTok 业务语义，吸收 new-api-main 组件化实现”为原则：

```text
(auth)/forgot-password.tsx
(auth)/oauth.tsx
(auth)/otp.tsx
(auth)/reset.tsx
(auth)/route.tsx
(auth)/sign-in.tsx
(auth)/sign-up.tsx
(auth)/user/reset.tsx
(errors)/401.tsx
(errors)/403.tsx
(errors)/404.tsx
(errors)/500.tsx
(errors)/503.tsx
__root.tsx
_authenticated/channels/index.tsx
_authenticated/chat/$chatId.tsx
_authenticated/chat2link.tsx
_authenticated/dashboard/$section.tsx
_authenticated/dashboard/index.tsx
_authenticated/errors/$error.tsx
_authenticated/keys/index.tsx
_authenticated/models/$section.tsx
_authenticated/models/index.tsx
_authenticated/playground/index.tsx
_authenticated/profile/index.tsx
_authenticated/redemption-codes/index.tsx
_authenticated/route.tsx
_authenticated/subscriptions/index.tsx
_authenticated/system-info/index.tsx
_authenticated/system-settings/auth/$section.tsx
_authenticated/system-settings/auth/index.tsx
_authenticated/system-settings/billing/$section.tsx
_authenticated/system-settings/billing/index.tsx
_authenticated/system-settings/content/$section.tsx
_authenticated/system-settings/content/index.tsx
_authenticated/system-settings/index.tsx
_authenticated/system-settings/models/$section.tsx
_authenticated/system-settings/models/index.tsx
_authenticated/system-settings/operations/$section.tsx
_authenticated/system-settings/operations/index.tsx
_authenticated/system-settings/route.tsx
_authenticated/system-settings/security/$section.tsx
_authenticated/system-settings/security/index.tsx
_authenticated/system-settings/site/$section.tsx
_authenticated/system-settings/site/index.tsx
_authenticated/usage-logs/$section.tsx
_authenticated/usage-logs/index.tsx
_authenticated/users/index.tsx
_authenticated/wallet/index.tsx
about/index.tsx
console/log.tsx
console/topup.tsx
index.tsx
oauth/$provider.tsx
pricing/$modelId/index.tsx
pricing/index.tsx
privacy-policy.tsx
rankings/index.tsx
setup/index.tsx
user-agreement.tsx
```

### 默认前端 feature 目录

NexusTok 独有：

| 目录 | 状态 | 处理 |
|------|------|------|
| `account-pool` | 已有实际页面、接口和组件 | 核心保留。 |
| `pricing-settings` | 已有 Root 页面 | 保留并与系统设置计费页保持边界。 |
| `request-rules` | 当前为空目录 | 不计为已实现页面，后续若做请求规则编辑器再补齐。 |

两边共有 feature 目录：

```text
about
auth
channels
chat
dashboard
errors
home
keys
legal
models
performance-metrics
playground
pricing
profile
rankings
redemption-codes
setup
subscriptions
system-info
system-settings
usage-logs
users
wallet
```

其中差异较大的共有目录：

| 目录 | new-api-main 优势 | NexusTok 优势 | 建议 |
|------|------------------|---------------|------|
| `channels` | 表单 section 拆分、移动端卡片、权限上下文 | 账号池绑定、渠道账号、Codex OAuth | 先吸收表单结构和移动端卡片。 |
| `dashboard` | `flow` 图表和选择逻辑 | 现有总览和模型数据；`/api/data/flow` 后端已落地 | 后续迁移 Sankey 图和选择逻辑。 |
| `playground` | 分层更清晰，stream/message/input/storage 工具完整 | 已接入当前项目调试链路 | 按工具函数渐进迁移。 |
| `subscriptions` | 余额支付、钱包溢出、重置弹窗 | 已有订阅基础管理 | 后端字段先行。 |
| `system-info` | 系统实例和系统任务观测入口 | 已接入实例面板、系统任务面板、日志清理、批量渠道测试、上游模型同步、账号池检测、订阅维护、Midjourney 轮询和异步任务轮询结果展示 | 后续新增后台任务继续复用同一观测面板。 |
| `system-settings` | routing-reliability、token-limit、Waffo Pancake 绑定体验 | 独立计费页和 NexusTok 设置结构 | 逐 section 合并。 |
| `usage-logs` | 移动端卡片、统一筛选工具条 | 账号池日志体系 | 将账号池日志也纳入一致筛选体验。 |

### 经典前端页面

| 差异 | 文件 | 处理 |
|------|------|------|
| NexusTok 独有账号池页 | `web/classic/src/pages/AccountPool/index.jsx` | 保留，classic 兼容入口。 |
| NexusTok 独有计费设置页 | `web/classic/src/pages/PricingSetting/index.jsx` | 保留，Root 计费管理入口。 |
| `new-api-main` 没有独有 classic 页面文件 | - | 不需要从 classic 迁移页面，只可参考退场提示和主题 helper。 |

### 后端独有文件：NexusTok

| 范围 | 文件 | 对应能力 |
|------|------|----------|
| controller | `account_pool.go`、`account_pool_*_test.go`、`account_pool_proxy.go`、`account_pool_usage_proxy.go` | 账号池管理、检测、导出、代理、审计。 |
| controller | `channel_account.go`、`channel_account_pool_test.go` | 渠道账号与账号池绑定。 |
| controller | `codex_oauth.go`、`discord.go`、`github.go`、`linuxdo.go`、`oidc.go` | OAuth 和 Codex 接入。 |
| controller | `models_dev_sync_task.go`、`model_sync_test.go` | models.dev 同步任务。 |
| model | `account_pool.go`、`account_pool_health.go`、`account_pool_state_log_audit_test.go` | 账号池数据模型和审计。 |
| model | `channel_account.go`、`model_pricing_config.go` | 渠道账号和定价配置。 |
| service | `account_pool_*`、`accountauth/*`、`account_pool_task_limit.go` | 账号池选择、刷新、认证文件、检测、任务限制。 |
| service | `channel_account_*`、`codex_oauth_test.go` | 渠道账号选择与 Codex OAuth 测试。 |
| constant | `channel_credential_mode.go` | 渠道凭据模式枚举。 |

### 后端独有文件：new-api-main

| 范围 | 文件 | 对应能力 | 原生化优先级 |
|------|------|----------|--------------|
| router | `authz-router.go` | 权限 catalog API | P1 |
| router | `channel-router.go`、`channel_router_test.go` | 渠道路由权限表和测试 | P1 |
| controller | `audit.go`、`authz.go`、`channel_authz.go` | 审计、权限 catalog、渠道敏感字段分类 | P1 |
| controller | `system_task.go`、`system_task_handlers.go` | 系统任务 API | 查询接口、日志清理创建入口、批量渠道测试、上游模型同步和账号池检测 handler 已原生化；其它 handler 待接入。 |
| controller | `system_info.go` | 系统实例列表 API | 已按 NexusTok Root 只读接口和默认前端实例面板原生化。 |
| controller | `subscription_payment_waffo_pancake.go` | 订阅 Waffo Pancake 支付 | P2 |
| controller | `usedata_flow_test.go` | 流量账本接口测试 | 后端已原生化 `/api/data/flow` 与 `/api/data/flow/self`，前端图表待迁移。 |
| model | `authz_role.go`、`casbin_rule.go` | 权限存储 | P1 |
| model | `system_task.go` | 系统任务 | 已原生化 SystemTask/SystemTaskLock，日志清理、批量渠道测试、上游模型同步和账号池检测共用同一租约/进度/历史模型。 |
| model | `system_instance.go` | 系统实例心跳 | 已按 NexusTok 三库兼容模型原生化。 |
| model | `locking.go` | 通用锁能力 | P1/P2，需确认是否仍需要独立于 SystemTaskLock 的通用锁。 |
| model | `usedata_flow.go` | 分组/渠道/用户流量聚合 | P2 |
| model | `clickhouse_log_test.go` | ClickHouse 日志兼容测试 | P3/可选 |
| service | `authz/*` | Casbin 权限服务 | P1 |
| service | `system_task.go` | 任务 runner | runner、日志清理 handler 和同类型多 ActiveKey 排队能力已原生化；异步轮询 handler 待接入。 |
| service | `system_instance.go` | 节点心跳 runner | 已按 NexusTok `NODE_NAME`/主机名兜底和资源快照原生化。 |
| service | `protected_fetch_client.go` | SSRF 保护 HTTP client | P1 |
| service | `relayconvert/*` | Relay 格式转换整理 | P3 |
| service | `task_polling_test.go` | 异步任务轮询测试 | P2 |

### API 差异摘要

NexusTok 独有 API 族：

| API 前缀 | 能力 | 处理 |
|----------|------|------|
| `/api/account-pool/*` | 原生账号池、认证文件、组、账号、检测、健康、状态审计、使用日志 | 保留并扩展权限。 |
| `/api/channel/:id/accounts*` | 渠道账号管理 | 保留，与账号池形成互补。 |
| `/api/channel/codex/oauth/*`、`/api/channel/:id/codex/oauth/*`、`/api/channel/:id/codex/usage/*` | Codex 渠道 OAuth 与用量查询 | 保留；后端已补 reset-credits 查询和 reset 消费接口，默认前端 Codex 用量弹窗已接入重置额度明细和确认重置流程。 |

`new-api-main` 独有或更完整 API 族：

| API 前缀 | 能力 | 处理 |
|----------|------|------|
| `/api/authz/catalog` | 权限资源/角色 catalog | P1 引入。 |
| `/api/system-task/*` | 后台任务创建、查询、当前任务 | 查询、当前任务和日志清理创建入口已引入；其它创建接口需等真实 handler 接入后开放。 |
| `/api/system-info/instances` | 多节点实例心跳 | 后端和默认前端实例面板已引入。 |
| `/api/data/flow`、`/api/data/flow/self` | 流量账本聚合 | 已引入；Root/Admin/User 按角色返回不同维度。 |
| `/api/subscription/balance/pay` | 余额购买订阅 | P2 引入。 |
| `/api/subscription/admin/*/reset` | 订阅重置 | P2 引入。 |
| `/api/waffo-pancake/webhook`、`/api/waffo-pancake/webhook/:env` | Waffo Pancake webhook | NexusTok 已有无环境路径的支付实现，应先补齐无环境回调路由；分环境路径后续评估。 |
| `/api/option/waffo-pancake/*` | Waffo Pancake catalog/pair/product 绑定 | P2 引入为设置向导。 |

## 验收建议

每个功能点迁移时至少满足：

1. 后端使用 `common.Marshal`/`common.Unmarshal`，不直接调用 `encoding/json` marshal/unmarshal。
2. 数据库迁移同时支持 SQLite、MySQL、PostgreSQL。
3. 涉及计费表达式或额度换算时，先重读 `pkg/billingexpr/expr.md`。
4. 前端新增文案走 i18n，默认前端用 Bun 执行 `bun run typecheck` 和必要的 `bun run i18n:sync`。
5. 涉及页面、交互、接口或联调时，用 MCP 浏览器真实验证页面、请求、响应和控制台。
6. 每个独立功能点单独提交，提交信息使用中文。

## 已落地原生化记录

## 本轮实施评审：订阅余额支付与钱包溢出前端原生化

### 需求分析

`new-api-main` 的订阅模块已经把余额购买套餐、套餐是否允许余额购买、订阅额度耗尽后是否允许钱包兜底做成可配置能力。NexusTok 后端已经原生化了对应能力：`POST /api/subscription/balance/pay`、`allow_balance_pay`、`allow_wallet_overflow` 和订阅购买时的钱包溢出快照，但默认前端仍只暴露 Stripe、Creem、Epay 等第三方支付入口，管理员也无法在套餐表单中配置这两个策略字段。结果是后端能力已经存在，用户侧和管理侧却不能完整使用。

本轮目标是只补齐已落地后端能力的默认前端入口：

1. 管理员在订阅套餐创建/编辑抽屉中配置 `allow_balance_pay` 和 `allow_wallet_overflow`。
2. 套餐列表展示余额购买能力，方便管理员核对策略。
3. 用户在钱包订阅购买弹窗中看到钱包余额、所需余额额度，并可直接调用余额购买接口。
4. 余额购买成功后刷新当前用户信息与订阅列表，避免页面继续显示旧余额。

本轮不迁移 `new-api-main` 的 Waffo Pancake 订阅商品绑定、订阅批量重置弹窗和相关未知接口，避免把尚未确认的后端能力强行接入默认前端。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 订阅类型/API | `web/default/src/features/subscriptions/types.ts`、`api.ts` | 增加两个套餐策略字段和余额支付 API 调用。 |
| 管理端套餐表单 | `web/default/src/features/subscriptions/lib/plan-form.ts`、`components/subscriptions-mutate-drawer.tsx` | 表单默认值、编辑回填、提交 payload 和两个策略开关。 |
| 管理端套餐列表 | `web/default/src/features/subscriptions/components/subscriptions-columns.tsx` | 增加余额支付状态徽标，便于审核套餐支付能力。 |
| 用户购买弹窗 | `web/default/src/features/subscriptions/components/dialogs/subscription-purchase-dialog.tsx` | 展示余额支付所需额度、可用余额和错误状态，调用余额支付接口。 |
| 钱包订阅卡片 | `web/default/src/features/wallet/components/subscription-plans-card.tsx` | 将当前用户余额传入购买弹窗，并在购买成功后刷新用户与订阅。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增用户可见文案的六语翻译。 |

### 风险评估

1. 余额支付会触发真实扣款和订阅创建，因此 UI 必须在余额不足、套餐禁止余额购买、购买次数上限时禁用按钮，并保留后端最终校验。
2. 余额支付所需额度按前端当前 `quota_per_unit` 估算展示，后端仍以模型层 `calcSubscriptionBalanceQuota` 为准；前端展示只作提示，不参与扣费决策。
3. `allow_wallet_overflow` 是购买时快照字段，修改套餐不会追溯修改既有订阅；文案需要表达这是“后续购买策略”而不是全局强制刷新。
4. 管理端表单只新增两个布尔字段，默认值与后端 `NormalizeDefaults` 一致为 `true`，不会改变旧套餐在未触碰时的兼容语义。
5. 本轮不改后端数据库、计费事务和订阅创建逻辑，因此核心业务风险主要来自前端字段遗漏或提交 payload 不正确，可通过 typecheck、i18n sync 和 MCP 页面/API 验证覆盖。

### 方案评审

采用小步原生化方案：只消费 NexusTok 已经存在的后端接口与字段，不复制 `new-api-main` 的整套订阅页面。管理端继续保留当前抽屉结构，只在基础信息区增加两个开关；用户端继续保留当前购买弹窗，只把“余额支付”作为独立支付块放在第三方支付前面。这样可以最小化对现有 Stripe/Creem/Epay 流程、套餐管理表格和钱包页面布局的影响。

验收方式：

1. `bun run typecheck` 确认 TypeScript 类型正确。
2. `bun run i18n:sync` 确认新增文案进入六语 locale。
3. 使用 MCP 打开 `http://192.168.0.202:3003/`，登录后访问钱包订阅入口和管理端订阅页，确认页面更新、控制台无新增错误、余额支付接口路径正确。
4. 更新本文件“已落地原生化记录”，标记本轮前端能力落地。

## 本轮实施评审：GORM v2 行锁原生化

### 需求分析

`new-api-main` 已经把事务行锁封装为 GORM v2 的 `clause.Locking` helper，并在 SQLite 下跳过 `FOR UPDATE`。NexusTok 的支付、订阅预消费、兑换码和邀请额度转移路径仍保留 GORM v1 写法 `Set("gorm:query_option", "FOR UPDATE")`。该写法在 GORM v2 中不会生成真实行锁，导致 MySQL/PostgreSQL 部署下的并发回调、并发补单、并发预消费和重复兑换场景存在锁失效风险。

本轮目标是把 `new-api-main` 的优势转换为 NexusTok 原生模型层能力：

1. 新增模型层私有 `lockForUpdate(tx *gorm.DB) *gorm.DB`，统一声明“下一次查询需要行锁”。
2. SQLite 保持无 `FOR UPDATE`，避免语法不兼容；MySQL/PostgreSQL 使用 GORM v2 原生 `clause.Locking{Strength: "UPDATE"}`。
3. 替换模型层全部旧 `gorm:query_option` 写法，覆盖订阅订单、订阅余额购买、订阅预消费/退款/重置、充值订单、兑换码和邀请额度转移。
4. 补充 dry-run SQL 单元测试，防止后续重新引入无效旧写法。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 行锁 helper | `model/locking.go` | 新增模型层私有 helper，按数据库类型生成或跳过 `FOR UPDATE`。 |
| 行锁测试 | `model/locking_test.go` | 使用 GORM dummy dialector 验证 MySQL/PostgreSQL 标记下生成 `FOR UPDATE`，SQLite 标记下不生成。 |
| 订阅事务 | `model/subscription.go` | 订单完成/过期、余额购买、订阅管理、预消费、退款、周期重置和用量回写改用新 helper。 |
| 充值事务 | `model/topup.go` | Stripe、Creem、Waffo、Waffo Pancake、管理员补单等充值订单锁定改用新 helper。 |
| 兑换与用户额度 | `model/redemption.go`、`model/user.go` | 兑换码消费和邀请额度转余额的并发保护改用新 helper。 |

### 风险评估

1. MySQL/PostgreSQL 上 SQL 会从“实际未加锁”变为真实 `SELECT ... FOR UPDATE`，这是本轮的预期修复；可能让高并发支付回调等待事务结束，但能避免重复入账或重复消费。
2. SQLite 不支持 `FOR UPDATE`，必须继续跳过锁子句；SQLite 仍依赖单写者事务模型和写冲突处理保证最终一致性。
3. `clause.Locking` 只影响紧随其后的查询，替换时必须保持 helper 与 `Where(...).First/Find(...)` 在同一查询链上，避免锁声明被无关查询消费。
4. 本轮不改订单状态机、额度换算、缓存刷新、日志审计和路由入口，因此业务语义保持不变；主要风险集中在漏替换或 SQL 生成不符合三库预期。
5. 测试会临时切换 `common.UsingSQLite/UsingMySQL/UsingPostgreSQL`，必须在测试清理阶段恢复，避免污染同包其它测试。

### 方案评审

采用最小侵入原生化方案：在 `model` 包内新增私有 helper，而不是把锁语义暴露给 controller/service。旧写法全部替换为 `lockForUpdate(tx).Where(...).First/Find(...)`，保留原有查询条件、错误处理和事务边界。测试层使用 GORM `utils/tests.DummyDialector` 的 dry-run 模式，只验证 SQL 生成，不依赖真实 MySQL/PostgreSQL 服务，降低验证成本。

验收方式：

1. `rg 'gorm:query_option' model` 确认模型层旧锁写法清零。
2. `go test ./model -run 'TestLockForUpdate|TestRedeem|TestCompleteSubscription|TestPurchaseSubscriptionWithBalance|TestManualCompleteTopUp'` 覆盖行锁 helper 和关键支付/兑换热点。
3. `go test ./model ./controller ./service` 做模型、控制器、服务层回归。
4. 使用 MCP 打开 `http://192.168.0.202:3003/`，登录后确认页面仍正常加载、控制台无新增错误；本轮是后端模型层并发修复，页面不应出现可见交互变化。

## 本轮实施评审：会话守卫仅 401 登出

### 需求分析

`new-api-main` 的默认前端在 `_authenticated` 路由预检 `/api/user/self` 时，只把 HTTP 401 视为明确的 session 失效；网络错误、超时、后端 5xx 或反向代理短暂异常会返回空结果并暂时放行，下一次导航继续重验。NexusTok 当前守卫把 `getSelf()` 的任意异常都当成认证失败，直接清除本地用户并跳转登录页。对生产后台来说，这会把短暂网络抖动放大成误登出，尤其是在管理员打开渠道、日志、系统任务等长时间页面时体验较差。

本轮目标是吸收该 P0 前端稳定性优势，并保持 NexusTok 原有安全边界：

1. 本地没有用户信息时仍立即跳转登录页。
2. `/api/user/self` 返回 401 时仍清理本地认证状态并跳转登录页。
3. `/api/user/self` 因网络错误、超时、5xx 等非 401 异常失败时，不清理本地用户，不设置 `sessionVerified`，保留后续导航重验机会。
4. 全局 axios 拦截器对普通业务请求的 401 行为保持不变，避免把真实失效 session 悄悄放行。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 认证路由守卫 | `web/default/src/routes/_authenticated/route.tsx` | 区分 `getSelf()` 的 401 与临时故障，非 401 故障不误清登录态。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和落地状态。 |

### 风险评估

1. 非 401 故障临时放行依赖本地缓存用户信息，因此必须限制在 `_authenticated` 路由预检阶段；普通 API 请求继续由后端鉴权和全局 401 拦截器兜底。
2. 如果后端返回业务层 `success=false` 但 HTTP 状态不是 401，守卫会按“服务端已明确回应但认证未通过”处理，仍清登录态；这样保留原有明确失败语义。
3. `sessionVerified` 只能在 `getSelf()` 成功并回传用户数据后设置为 `true`；临时故障不能设置，否则会让本会话停止重验。
4. 实现应避免新增依赖或新测试工具链，降低前端热更新与构建风险。

### 方案评审

采用最小侵入方案：只修改 `_authenticated` 路由守卫，把错误状态解析封装为本文件内的小函数，读取 Axios 风格错误对象上的 `response.status`。当状态为 401 时返回明确失败对象，让既有清理和重定向逻辑继续执行；当状态不是 401 或没有 HTTP 响应时返回 `null`，守卫保持本地登录态并等待下一次导航重试。全局 `api` 拦截器、React Query 错误处理、登录/登出流程和权限矩阵不变。

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认 TypeScript 类型正确。
2. 使用 MCP 打开 `http://192.168.0.202:3003/`，登录后进入 `/dashboard/overview`，确认页面加载正常、`/api/user/self` 返回 200、控制台无新增错误。
3. 通过浏览器上下文拦截 `/api/user/self` 返回 500，访问受保护页面，确认不会被重定向到 `/sign-in`。
4. 通过浏览器上下文拦截 `/api/user/self` 返回 401，访问受保护页面，确认会清登录态并重定向到 `/sign-in`。

## 本轮实施评审：HeaderNavModules 前端健壮解析

### 需求分析

`new-api-main` 已经把 `HeaderNavModules` 的前端解析集中到 `lib/nav-modules.ts`，兼容 JSON bool、数字、字符串和对象字段。NexusTok 后端 `middleware/header_nav.go` 已具备类似兼容能力，但默认前端仍存在三套解析逻辑：`use-top-nav-links.ts` 兼容了顶层字符串/数字，`lib/nav-modules.ts` 只兼容布尔和对象中的布尔字段，系统设置表单又用 `Boolean(value)` 转换。结果是历史配置或手动写入的 `"false"`、`"0"`、`0` 可能在顶部导航、公开页面守卫和设置页里表现不一致，例如页面守卫认为 rankings 启用，设置页也把 `"false"` 显示为开启。

本轮目标是把 `new-api-main` 的集中解析优势转换成 NexusTok 默认前端原生能力：

1. `HeaderNavModules` 支持原始对象或字符串化 JSON 两种输入。
2. `true/false`、`1/0`、`"true"/"false"`、`"1"/"0"` 在顶层模块和 `pricing/rankings.enabled/requireAuth` 中语义一致。
3. 顶部导航、`/rankings` 页面守卫、系统设置表单共用同一个解析器，避免三处 drift。
4. 保留 NexusTok 当前 `disabled` 链路：未登录且 `requireAuth=true` 的导航项继续展示为禁用，而不是直接隐藏。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 导航配置解析 | `web/default/src/lib/nav-modules.ts` | 新增导出的类型、默认值、布尔解析和完整 `parseHeaderNavModules`。 |
| 顶部导航 hook | `web/default/src/hooks/use-top-nav-links.ts` | 改为复用 `parseHeaderNavModulesFromStatus`，避免重复解析逻辑。 |
| 系统设置表单 | `web/default/src/features/system-settings/maintenance/config.ts`、`header-navigation-section.tsx` | 复用统一解析类型，设置页对历史字符串/数字配置正确回填。 |
| 前端测试 | `web/default/src/lib/nav-modules.test.ts` | 使用项目现有 Node 原生测试风格覆盖字符串、数字、对象和 fallback。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和落地状态。 |

### 风险评估

1. 配置解析改变会影响公开导航显示、`/rankings` 守卫和设置页回填；必须保持空配置默认全启用、解析失败回退默认值。
2. `pricing/rankings` 的 `requireAuth` 只在对象格式中有效；历史顶层布尔/字符串只表达 enabled，不应意外启用登录要求。
3. 系统设置保存时会序列化为标准对象格式，可能把历史短格式规范化；这是可接受的迁移效果，但必须避免丢失未知顶层模块。
4. 本轮不改后端 `HeaderNavModuleAuth`，因为后端已经兼容字符串和数字；前端行为与后端对齐即可。

### 方案评审

采用集中解析方案：以 `web/default/src/lib/nav-modules.ts` 作为单一事实来源，导出 `HeaderNavModules`、`ModuleAccess`、`HEADER_NAV_DEFAULT`、`parseHeaderNavBoolean`、`parseHeaderNavModules` 和 `parseHeaderNavModulesFromStatus`。`useTopNavLinks` 和系统设置维护配置改为复用该模块，设置表单只消费已标准化的 `HeaderNavModules`。测试使用当前项目已有的 `node:test` + `node:assert/strict` 风格，不引入新依赖。

验收方式：

1. `cd web/default && node --import tsx src/lib/nav-modules.test.ts` 覆盖解析逻辑。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认类型正确。
3. 使用 MCP 打开 `http://192.168.0.202:3003/` 并登录，确认首页/后台加载正常、控制台无新增错误。
4. 使用已登录 root 会话通过 `PUT /api/option/` 临时写入 `HeaderNavModules={"pricing":"0","rankings":{"enabled":"false","requireAuth":"1"}}`，刷新未登录首页，确认顶栏不显示 Model Square/Rankings，并在验证后恢复原值。
5. 使用已登录 root 会话通过 `PUT /api/option/` 临时写入 `HeaderNavModules={"pricing":{"enabled":"1","requireAuth":"1"},"rankings":{"enabled":1,"requireAuth":0}}`，刷新未登录首页，确认 Model Square 存在但禁用，Rankings 正常显示，并在验证后恢复原值。

## 本轮实施评审：Usage Logs 移动端卡片与筛选工具条

### 需求分析

`new-api-main` 的默认前端已经把 Usage Logs 在手机端改成按日志语义组织的卡片，并把长筛选条件收纳到移动端 Drawer 中。NexusTok 当前日志页虽然已经有通用 `DataTablePage` 和 `MobileCardList`，但 Usage Logs 的 common、drawing、task 三类日志字段密度高、含义差异明显，通用移动卡片只能按列顺序堆叠，手机端很难快速扫到模型/成本/状态/任务 ID/失败原因等关键字段；同时 common 日志筛选条件较多，手机端日期、模型、分组、类型、Token、用户名、渠道和请求 ID 会挤在同一工具条里。

本轮目标是吸收 `new-api-main` 的移动端体验优势，并转成 NexusTok 原生能力：

1. common 日志在手机端以模型与成本为首屏重点，突出时间、类型、渠道、用户、Token、耗时、Token 数和详情。
2. drawing 日志在手机端突出任务类型/提交结果、提交时间、渠道、任务 ID、耗时、图片、Prompt 和失败原因。
3. task 日志在手机端突出任务 ID/状态、提交时间、用户和结果详情。
4. 手机端筛选工具条固定展示日期范围，其他条件放入 Drawer；桌面端继续保留高密度横向筛选与可展开高级筛选。
5. 保留现有 URL search、React Query 失效、管理员条件、敏感信息开关、列显隐和分页行为，不改后端接口、不改变日志查询语义。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 移动端日志卡片 | `web/default/src/features/usage-logs/components/usage-logs-mobile-card.tsx` | 新增 Usage Logs 专用移动端列表，根据日志分类渲染语义卡片，复用现有列 cell、状态徽章和空态组件。 |
| 日志筛选工具条 | `web/default/src/features/usage-logs/components/logs-filter-toolbar.tsx` | 新增 desktop/mobile 自适应筛选容器；移动端用 Drawer 承载非固定筛选项。 |
| common 筛选栏 | `web/default/src/features/usage-logs/components/common-logs-filter-bar.tsx` | 接入新工具条，复用现有筛选 state、`buildSearchParams`、敏感信息开关和统计栏。 |
| task/drawing 筛选栏 | `web/default/src/features/usage-logs/components/task-logs-filter-bar.tsx` | 接入新工具条，移动端只固定时间范围，任务 ID 与渠道进入 Drawer。 |
| 日志表格入口 | `web/default/src/features/usage-logs/components/usage-logs-table.tsx` | 通过 `DataTablePage.mobile` slot 使用专用移动端列表，桌面表格渲染保持不变。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增 Drawer 说明文案的六语翻译。 |

### 风险评估

1. Usage Logs 是高频排障页面，必须避免桌面表格回退；因此只在 `DataTablePage.mobile` slot 接入新列表，桌面 `renderRow`、sticky header 和行 tint 保持原状。
2. common 日志类型 `0` 在 NexusTok 仍代表 `Unknown`，不是“全部”；本轮不迁移 `new-api-main` 的 `LOG_TYPE_ALL_VALUE='0'` 语义，继续沿用当前空类型表示全部，避免误改后端查询。
3. 移动端卡片复用现有列 cell，列显隐仍会影响卡片内容；这是与当前表格列视图能力一致的原生行为，但卡片需对缺失 cell 提供空值兜底。
4. Drawer 筛选会关闭后再触发查询，如果状态同步与 URL search 不一致，可能出现旧筛选值；因此保留当前筛选栏的本地 state 与 `useEffect` 同步逻辑，不引入额外 draft 语义。
5. 新增移动端布局要避免文本溢出、按钮挤压和无标题 Drawer；类型检查、桌面/移动 MCP 验证和真实网络请求检查是本轮必要验收。

### 方案评审

采用局部原生化方案：不改通用 `DataTablePage`，而是新增 Usage Logs 专用 `UsageLogsMobileList` 和 `LogsFilterToolbar`，通过现有 slot 与筛选栏接入。视觉方向采用 Swiss 运维界面：中性底色、1px 边框、左对齐、紧凑字段网格和清晰状态线，不引入新的主题色或营销式卡片。这样既能吸收 `new-api-main` 的手机端扫描效率，也避免把通用数据表和后端日志接口拉入不必要重构。

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认 TypeScript 类型正确。
2. `git diff --check` 确认没有空白错误。
3. 使用 MCP 打开 `http://192.168.0.202:3003/` 并登录，验证 `/usage-logs/common` 桌面表格正常，移动端显示专用日志卡片。
4. 移动端点击 Filter 打开 Drawer，验证 common 日志的模型、分组、类型、Token、用户名、渠道、Request ID、Upstream Request ID 能搜索和重置。
5. 验证 `/usage-logs/drawing` 和 `/usage-logs/task` 移动端任务 ID/渠道筛选、卡片内容、网络请求和控制台状态正常。

## 本轮实施评审：Dashboard Flow 流量账本可视化

### 需求分析

`new-api-main` 的默认前端已经提供 Dashboard Flow/Sankey 视图，可以把请求从用户、节点、Token、分组、模型到渠道的成本流向串起来。NexusTok 当前后端已经完成 `/api/data/flow` 与 `/api/data/flow/self` 聚合接口，但默认前端 Dashboard 仍只有 Overview、Models、Users 三个分区，管理员无法在页面上直接观察“哪类用户、分组、模型和渠道形成主要成本流”，普通用户也无法查看自己的 Token/group/model 流向。

本轮目标是把 `new-api-main` 的 Flow 优势转换为 NexusTok 原生前端能力：

1. 新增 Dashboard `flow` 分区，直接消费现有 `/api/data/flow` 和 `/api/data/flow/self`，不新增后端权限或查询接口。
2. Root 视图展示 `user -> node -> token -> group -> model -> channel`，Admin 视图展示 `user -> group -> model -> channel`，普通用户视图展示 `token -> group -> model`，与后端角色裁剪保持一致。
3. 支持按 quota、tokens、requests 切换 Sankey 宽度指标，并支持 Top 节点限制、溢出聚合/隐藏、用户筛选、节点筛选、阶段显隐和点击高亮。
4. 对敏感节点标签保留可遮罩能力，避免在后续嵌入共享截图或非敏感视图时泄露用户、Token、节点、分组和渠道名称。
5. 补齐算法测试和六语翻译，确保 Flow 的聚合、过滤、Top 限制、隐藏阶段、点击高亮和空/错误状态可回归验证。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Dashboard 类型与 API | `web/default/src/features/dashboard/types.ts`、`api.ts` | 新增 Flow 数据结构、角色、节点、筛选、Sankey 图数据类型和 `getFlowQuotaDates`。 |
| Flow 数据处理 | `web/default/src/features/dashboard/lib/flow.ts`、`flow-selection.ts`、`lib/index.ts` | 新增角色路径构建、节点聚合、Top 限制、溢出策略、敏感标签遮罩、点击高亮和 VChart Sankey spec 构建。 |
| Flow 前端组件 | `web/default/src/features/dashboard/components/flow/*` | 新增 Flow 图表、指标切换、用户/节点筛选、阶段显隐、空态、错误态和加载态。 |
| Dashboard 入口 | `web/default/src/features/dashboard/index.tsx`、`section-registry.tsx` | 新增 `/dashboard/flow` 分区入口，复用现有日期范围和 Dashboard 子分区切换。 |
| 通用选择器兼容 | `web/default/src/components/multi-select.tsx` | 增强大量用户筛选时的 chip 收起、空态提示和自定义汇总，不影响旧调用。 |
| 图表颜色与测试 | `web/default/src/features/dashboard/lib/flow.ts`、`flow*.test.ts` | Flow 内置稳定色板并补充纯函数测试，避免测试依赖 VChart 内部 ESM 子路径。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增 Flow 页面、控件、tooltip、空态和阶段说明文案。 |

### 风险评估

1. Flow 是高维 Sankey 图，真实生产数据可能节点很多；必须提供 Top 节点限制和溢出聚合，避免一次渲染过多节点造成卡顿。
2. `/api/data/flow` 的角色裁剪由后端决定，前端只能选择正确接口和正确角色路径；普通用户必须走 `/api/data/flow/self`，不能通过前端筛选暴露渠道、节点或其它用户维度。
3. Sankey 依赖 `@visactor/react-vchart` 与主题初始化，必须覆盖 `themeReady`、空数据、错误响应、加载中和图表点击事件，避免白屏。
4. 新增 Dashboard 分区会影响路由校验、顶部子分区 tabs 和侧边导航；需要保持 Overview 默认入口和 Admin-only Users 分区行为不变。
5. `MultiSelect` 增强属于共享组件改动，必须保持原有 props 可选、旧调用不变，并通过类型检查和页面验证确认未影响其它选择器。
6. 本轮不改 Go 后端接口、数据库聚合和权限中间件，因此核心业务数据写入与账务结算不变；主要风险集中在前端展示、性能和多语言遗漏。

### 方案评审

采用小步原生化方案：保留 NexusTok 已有后端接口和 Dashboard 布局，把 `new-api-main` 的 Flow 聚合算法迁入默认前端并改为当前项目版权头、中文注释和现有组件风格。视觉方向采用 Swiss 运维看板：中性底色、1px 边框、左对齐、紧凑控件和明确字段，不引入营销式说明页，也不伪造演示数据。实现上先完整接入核心 Sankey、指标切换、Top 限制、用户/节点筛选、阶段显隐和点击高亮；后续若需要可在同一能力上扩展保存偏好、导出和分享视图。

验收方式：

1. `cd web/default && node --import tsx src/features/dashboard/lib/flow.test.ts` 覆盖 Flow 聚合与 Sankey spec。
2. `cd web/default && node --import tsx src/features/dashboard/lib/flow-selection.test.ts` 覆盖选择与展示状态 helper。
3. `cd web/default && ./node_modules/.bin/tsc -b` 确认 TypeScript 类型正确。
4. `git diff --check` 确认没有空白错误。
5. 使用 MCP 打开 `http://192.168.0.202:3003/` 并登录，访问 `/dashboard/flow`，确认页面更新、请求 `/api/data/flow` 或 `/api/data/flow/self`、图表/空态/错误态正常且控制台无新增错误。
6. 使用 MCP 切换桌面和移动视口，验证 Flow 控件不重叠，指标切换、Top 限制、溢出模式、节点筛选和阶段显隐能触发图表重新渲染。

## 本轮实施评审：Responses 请求反向兼容与 Gemini Responses

### 需求分析

`new-api-main` 已经补齐 OpenAI Responses 与 Chat Completions 的双向兼容链路：Chat 请求可以转 Responses，上游 Responses 响应也能转回 Chat，同时 Responses 请求还能降级为 Chat 请求供尚未原生支持 `/v1/responses` 的渠道使用。NexusTok 当前已经有 `service/openaicompat/chat_to_responses.go` 与 `responses_to_chat.go`，也具备 OpenAI/Codex/Cloudflare 等 Responses 直连路径，但仍缺少 `ResponsesRequest -> ChatCompletionsRequest` 的通用转换；Gemini 渠道的 `ConvertOpenAIResponsesRequest` 仍返回 `not implemented`，导致用户用 `/v1/responses` 调 Gemini 渠道时无法复用现有 Gemini Chat 适配和响应处理。

本轮目标是把 `new-api-main` 的 P1 Responses 兼容优势转换为 NexusTok 原生中继能力：

1. 在 `service/openaicompat` 内新增 Responses 请求到 Chat Completions 请求的转换，保留 NexusTok 现有包名和 service 薄封装，不引入 `service/relayconvert` 新包，避免改变热路径包结构。
2. 转换必须保留显式零值：`stream`、`store`、`temperature`、`top_p`、`top_logprobs`、`max_output_tokens`、`parallel_tool_calls` 等指针或 RawMessage 字段不能因为零值/false 被丢弃。
3. 对 stateful Responses 字段采用 fail-closed：`conversation`、`previous_response_id`、`prompt`、`context_management` 无法在 stateless Chat 请求中可靠表达，应返回明确错误，而不是静默丢语义。
4. Gemini 渠道先接入安全子集：过滤 Responses custom/freeform tool 及其输出，function tool 走现有 Gemini Chat 工具转换；这与 `new-api-main` 策略一致，避免把 Gemini 无法安全表达的工具语义伪装成普通文本。
5. Gemini Responses 响应复用现有 Gemini Chat 响应转换为 OpenAI Chat，再转 Responses 格式返回，保持计费 usage、空候选、PromptFeedback block reason 和流式完成事件语义。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Responses 请求转换 | `service/openaicompat/responses_request_to_chat.go`、`service/openai_chat_responses_compat.go` | 新增 Responses 请求降级 Chat 请求能力与 service 层导出函数。 |
| 转换测试 | `service/openaicompat/responses_request_to_chat_test.go` | 覆盖 string/array input、system instructions、content parts、function call、tool output、工具定义、tool_choice、response_format 和显式零值保留。 |
| Gemini Responses 预处理 | `relay/channel/gemini/adaptor_responses.go` | 过滤 Gemini 暂不支持的 custom/freeform Responses 工具和对应输出，避免语义错配。 |
| Gemini 渠道接入 | `relay/channel/gemini/adaptor.go`、`relay_responses.go` | `ConvertOpenAIResponsesRequest` 复用 Responses->Chat->Gemini；`DoResponse` 在 Responses 模式下输出 Responses 格式。 |
| Gemini 测试 | `relay/channel/gemini/adaptor_responses_test.go`、`relay_responses_test.go` | 覆盖 custom/freeform 过滤、Responses 请求转 Gemini、非流式 JSON 和流式 SSE 转 Responses。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和落地状态。 |

### 风险评估

1. Responses API 比 Chat Completions 更宽，特别是 stateful 字段、custom/freeform tools、conversation 和 prompt template；本轮只做可安全表达的无状态子集，遇到不能表达的字段必须明确报错。
2. Gemini Chat 的工具和多模态能力与 Responses 不完全等价，custom tool 直接透传会误导上游；因此本轮选择过滤并保留 function tool，不追求一次性覆盖所有 Responses 工具。
3. 转换层属于 relay 热路径，不能直接导入 `encoding/json` 做 marshal/unmarshal；所有 JSON 操作必须继续使用 `common.*` 包装。
4. Gemini 响应处理必须保持现有 PromptFeedback、空 candidates、usage 估算和流式 SSE 行为，不改变 Chat/Gemini 原生路径。
5. 本轮不改数据库、计费表达式、权限、渠道配置 schema 和前端页面，主要风险集中在中继请求转换和特定 Gemini Responses 路由。

### 方案评审

采用小步原生化方案：先把 `new-api-main` 的 Responses->Chat 转换移入 NexusTok 已有 `service/openaicompat` 包，使用当前项目 DTO、`common.*` JSON 包装和中文注释；再让 Gemini 的 Responses 请求通过该转换进入现有 `CovertOpenAI2Gemini`，响应侧把 Gemini Chat response 转为 Chat Completions 后再转 Responses。这样可以复用 NexusTok 已有 OpenAI Chat/Gemini 转换、Responses usage 结构和错误处理，不引入新路由、新数据库字段或新权限边界。

验收方式：

1. `go test ./service/openaicompat` 覆盖 Responses->Chat 转换和显式零值保留。
2. `go test ./relay/channel/gemini` 覆盖 Gemini Responses 请求预处理、请求转换和响应转换。
3. `go test ./relay/channel/openai ./relay` 确认现有 OpenAI Responses/Chat 兼容路径不被破坏。
4. `git diff --check` 确认无空白错误。
5. 使用 MCP 打开 `http://192.168.0.202:3003/`，确认页面仍正常加载、`/api/user/self` 返回 200、控制台无新增错误；本轮是后端 relay 能力增强，页面不应出现可见变化。

## 本轮实施评审：全局管理操作审计兜底

### 需求分析

`new-api-main` 已经通过 `middleware/audit.go` 和 `controller/audit.go` 建立了管理写操作审计兜底：管理员或 Root 通过认证后，对 `POST/PUT/PATCH/DELETE` 管理接口自动捕获路由、路径参数、HTTP 状态、业务 `success=false` 响应、操作者和客户端 IP，并在没有手动审计标记时写入管理日志。NexusTok 当前已经有 `LogTypeManage`、充值/额度饱和的 `other.admin_info` 脱敏机制，以及账号池专项状态审计，但全局管理接口仍依赖零散的 `RecordLog`/`RecordLogWithAdminInfo` 调用，渠道、账号池、订阅、模型、兑换码、系统任务等写操作缺少统一兜底记录。

本轮目标是把 `new-api-main` 的审计优势转为 NexusTok 原生治理能力：

1. 在 `AdminAuth`/`RootAuth` 鉴权成功后，只对管理写方法启用审计兜底，普通用户接口和 relay 热路径不纳入本轮范围。
2. 审计内容只记录操作者、路由、路径参数、状态码、业务成功状态和 action，不记录请求体，避免 API Key、OAuth token、password、secret、Base URL 等敏感字段进入日志。
3. 对已有手动审计预留 `ContextKeyAuditLogged`，后续 controller 可以标记已记录，兜底中间件不重复写同一次操作。
4. 审计日志写入 `LogTypeManage`，`Other` 保持 NexusTok 既有 `admin_info` 管理员脱敏约定，并新增 `audit_info`/`op` 结构供管理员详情页查看。
5. 默认前端 Usage Logs 详情页增加轻量的管理审计信息块，展示 action、结果、路由、状态码和 IP；桌面/移动日志列表、查询接口和普通用户日志脱敏行为保持不变。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 审计上下文键 | `constant/context_key.go` | 新增 `ContextKeyAuditLogged`，供手动审计函数与兜底中间件去重。 |
| 管理审计模型 | `model/log.go` | 新增 `RecordOperationAuditLog` 与审计参数结构，统一写入 `LogTypeManage`、`admin_info`、`op` 和 `audit_info`。 |
| 审计中间件 | `middleware/audit.go`、`middleware/auth.go` | 在 Admin/Root 鉴权成功后包装写方法响应，按路由 action map 自动记录管理操作。 |
| 手动审计辅助 | `controller/audit.go` | 提供 controller 内可复用的手动审计 helper，后续订阅重置、账号池高风险操作可逐步接入。 |
| 后端测试 | `middleware/audit_test.go`、`model/log_audit_test.go` | 覆盖写方法记录、GET 不记录、HTTP 200 但 `success=false` 记失败、已标记去重、普通用户接口不记录、普通用户日志视图移除 `admin_info`/`audit_info`。 |
| 前端日志详情 | `web/default/src/features/usage-logs/types.ts`、`details-dialog.tsx` | 管理员查看 type=3 日志时展示结构化审计结果；普通用户不可见 `admin_info`。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增管理审计信息块文案六语翻译。 |

### 风险评估

1. `middleware/auth.go` 是登录态和权限边界核心文件，变更必须保持原有未登录、Access Token、`NexusTok-User` 头、角色不足和用户禁用分支行为不变；审计只能在认证通过后运行。
2. 兜底审计若记录请求体会泄露密钥、密码、OAuth code、provider secret 或渠道 Base URL；本轮明确只记录路径参数和响应结果，不采集 body。
3. 异步写日志可能让测试不稳定，也可能在高写入场景积压；本轮先以模型层同步写入保证可测性，后续再评估是否在中间件侧引入有限队列或 gopool。
4. action map 不可能一次覆盖所有管理写接口；未命中的写接口必须降级为 `generic`，并记录 method、route、path，避免因漏表导致审计缺口。
5. 前端新增展示块必须只在管理员和 `LogTypeManage` 下渲染，不改变消费日志、充值日志和任务日志的详情结构。
6. 本轮不改变数据库 schema、不改变管理接口权限、不接入 Casbin enforcement，核心业务写入路径只增加审计副作用；如日志库写入失败，只记录系统日志，不影响原业务响应。

### 方案评审

采用小步原生化方案：复用 NexusTok 现有 `Log` 表和 `LogTypeManage`，新增一个只在 Admin/Root 写方法后置执行的审计中间件；用本项目路由 action map 覆盖用户、渠道、渠道账号、账号池、订阅、系统设置、系统任务、模型、厂商、部署、兑换码和 Codex 相关写操作；未命中路由自动使用 `generic`。controller 层提供手动审计 helper，但本轮不大规模修改业务 handler，以免把审计能力与具体业务改动混在一起。

视觉方向沿用当前 Usage Logs 的 Swiss 运维界面：中性底色、1px 边框、紧凑字段行和 `StatusBadge` 状态徽章，只增加真实审计字段，不加入说明性装饰文案或伪造数据。这样既能吸收 `new-api-main` 的审计兜底优势，也保持 NexusTok 的账号池专项审计、额度饱和审计和日志脱敏规则不被覆盖。

验收方式：

1. `go test ./model ./middleware ./controller` 覆盖审计记录、去重和脱敏。
2. `go test ./router` 确认路由/鉴权测试不受影响。
3. `cd web/default && ./node_modules/.bin/tsc -b` 确认前端类型正确；若当前环境缺少 Bun/依赖，记录限制并用可用脚本替代。
4. `git diff --check` 确认无空白错误。
5. 使用 MCP 打开 `http://192.168.0.202:3003/`，确认 Dashboard/Usage Logs 页面更新生效、`/api/user/self` 正常、控制台无新增错误。
6. 在不破坏真实数据的前提下，通过 MCP 或 httptest 触发一个低风险管理写接口，确认成功/失败审计能写入 `LogTypeManage`，并在 Usage Logs 详情页展示结构化审计信息。

## 本轮实施评审：渠道 Authz 路由权限表灰度 enforcement

### 需求分析

`new-api-main` 已经把 `/api/channel` 管理路由从粗粒度 Admin/Root 角色迁移为资源动作权限表：读、操作、普通写、敏感写和密钥查看分别挂接不同 `authz.Permission`，再由 `middleware.RequirePermission` 统一判定。NexusTok 当前已经有原生 `service/authz` catalog、`/api/authz/catalog`、`/api/user/self` 权限矩阵回传、默认前端入口显隐和渠道页按钮级权限消费，但服务端渠道路由仍只依赖 `AdminAuth`/`RootAuth`，导致前端隐藏的高风险按钮仍可能被普通 Admin 直接调用接口绕过。

本轮目标是把 `new-api-main` 的渠道路由权限表优势转换为 NexusTok 原生服务端 enforcement，但只做一个低风险切片：

1. 在现有 `service/authz` 中补齐渠道资源的稳定 `Permission` 常量和 `Can(userID, systemRole, permission)` 判定；当前仍只基于 Root/Admin 系统角色基线，不引入 Casbin、不引入用户 override、不新增数据库表。
2. 新增 `middleware.RequirePermission(permission)`，放在 `AdminAuth` 之后作为第二层权限门禁；未知资源或未知动作必须 fail-closed。
3. 将 `/api/channel` 路由迁移为权限表注册，保持现有 controller、HTTP method、path 和 handler 不变。
4. 保留已有 `RootAuth`、`CriticalRateLimit`、`DisableCache`、`SecureVerificationRequired` 边界；`/:id/key`、`/fetch_models` 等旧 Root-only 路由只能加严，不能放宽。
5. 渠道更新仍先以 `channel.write` 放行非敏感字段，再由 `controller/channel_authz.go` 对 Key、BaseURL、请求覆盖、凭证模式、账号池绑定和未知字段执行敏感写二次校验，避免影响普通 Admin 的日常模型/分组/权重维护。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz 判定 | `service/authz/permission.go`、`service/authz/registry.go` | 新增资源常量、渠道 permission 变量、`Can` 和已知权限检查；`Capabilities` 继续复用同一基线矩阵。 |
| 权限中间件 | `middleware/auth.go` | 新增 `RequirePermission`，读取认证上下文中的 user id 和 role，权限不足时返回 i18n 权限不足响应。 |
| 渠道路由 | `router/channel-router.go` | 以 `channelPermissionRoutes` 表注册读、操作、写、敏感写、密钥查看相关接口；保留旧 RootAuth/SecureVerification。 |
| 渠道敏感写桥接 | `controller/channel_authz.go` | 将敏感字段二次校验从硬编码 Root 过渡到 `authz.Can(..., ChannelSensitiveWrite)`，当前基线效果仍等价 Root。 |
| 后端测试 | `service/authz/permission_test.go`、`middleware/authz_test.go`、`router/channel_router_test.go` | 覆盖 Root/Admin/普通用户权限判定、未知权限 fail-closed、RequirePermission allow/deny，以及核心渠道路由挂接的 permission。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险、方案和落地状态，保持后续账号池/用户/系统设置 enforcement 的迁移依据。 |

### 风险评估

1. 这是从“前端显隐”进入“服务端 enforcement”的权限边界变化，普通 Admin 直接调用新建渠道、删除渠道、复制渠道、多 Key 管理、Codex 凭证刷新、Ollama 拉取/删除和上游模型应用等敏感接口会被拒绝；这与当前前端权限矩阵一致，但需要通过测试确认不会误拦截查询、测试、余额刷新和非敏感编辑。
2. 不引入 Casbin 意味着本轮没有用户级 override；Root 全部允许、Admin 只允许 read/operate/write 的语义必须与现有 catalog 和前端 hook 保持一致，避免 UI 和 API 产生双轨规则。
3. `/:id/key` 已经有 RootAuth、限流、禁缓存和安全验证；本轮只能叠加 `channel.secret_view`，不能替换这些旧边界，避免密钥查看被权限表误放宽。
4. 渠道更新接口仍是复合 payload，如果只按路由级 `channel.write` 放行会留下敏感字段绕过风险；因此必须保留字段级 fail-closed 二次校验，并把未知字段继续视为敏感。
5. 路由表重排可能影响 Gin 的静态路由和参数路由匹配；需要用路由结构测试确认 `/models`、`/models_enabled`、`/:id/accounts`、`/:id/codex/*`、`/upstream_updates/*` 等路径仍绑定原 handler。
6. 本轮不改数据库、不改前端页面、不改 Casbin 存储、不改账号池路由；核心风险集中在渠道管理 API 权限收紧和测试覆盖不足。

### 方案评审

采用灰度 enforcement 方案：先在 NexusTok 已有 `authz` catalog 上增加最小可执行判定，把 `/api/channel` 作为第一块真实路由权限表，不引入新依赖和新表。路由仍先经过 `AdminAuth`，因此未登录、普通用户、禁用用户和旧 `NexusTok-User` 头校验完全复用现有认证链路；`RequirePermission` 只在认证通过后按资源动作二次拦截。敏感旧边界继续由 `RootAuth` 和字段级敏感写校验兜底，后续接入 Casbin/user override 时只需要替换 `authz.Can` 内部实现，不必再次重写渠道路由。

验收方式：

1. `go test ./service/authz ./middleware ./router ./controller` 覆盖权限矩阵、权限中间件、渠道路由表和渠道字段敏感写二次校验。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认现有前端权限类型和页面构建不受影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 打开 `http://192.168.0.202:3003/`，确认页面更新生效、`/dashboard/overview` 和 `/channels` 正常加载、`/api/user/self` 200、控制台无新增 error/warn。
5. 使用 MCP 在浏览器上下文调用低风险渠道读接口，例如 `GET /api/channel/models` 或 `GET /api/channel/models_enabled`，确认登录 Root 仍能访问；高风险写接口只通过 httptest 验证 Admin 被拒绝，避免破坏真实数据。

## 本轮实施评审：账号池 Authz 路由权限表灰度 enforcement

### 需求分析

NexusTok 的账号池是当前项目相对 `new-api-main` 的核心原生优势：它覆盖认证文件、分组、账号、OAuth/设备授权、检测任务、使用日志、状态审计和健康看板。但这些能力目前集中注册在 `router/api-router.go` 的 `AdminAuth` 组内，服务端还没有消费已经回传给默认前端的 `account_pool` 权限矩阵。`new-api-main` 虽然没有账号池域模型，却已经提供了可复用的路由权限表模式：管理资源必须在服务端按 read/operate/write/sensitive_write/secret_view 分层 enforcement，而不是只依赖前端按钮显隐。

本轮目标是把上一轮渠道 Authz enforcement 扩展到 NexusTok 独有账号池资源，形成更完整的原生权限能力：

1. 为 `account_pool` 补齐稳定 permission 变量：`AccountPoolRead`、`AccountPoolOperate`、`AccountPoolWrite`、`AccountPoolSensitiveWrite`、`AccountPoolSecretView`，继续复用现有 Root/Admin 基线与 `authz.Can`。
2. 将 `/api/account-pool` 从 `router/api-router.go` 抽到独立路由文件，并用权限表注册全部现有路径，保持 HTTP method、path、handler 和业务返回不变。
3. 将只读查询、健康、脱敏使用日志、脱敏状态审计、分组选项、账号详情、检测任务详情归为 `account_pool.read`。
4. 将检测、状态更新、运行时重置、检测任务清理、登录会话取消和脱敏导出归为 `account_pool.operate`；这些动作会改变运行时状态或生成安全导出，但不直接写入凭证密文。
5. 将分组创建/更新归为 `account_pool.write`；将认证文件导入/更新/删除、账号创建/删除/绑定、OAuth/设备授权完成、凭证刷新等会触碰凭证材料或账号生命周期的入口归为 `account_pool.sensitive_write`。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz 判定 | `service/authz/permission.go`、`service/authz/permission_test.go` | 新增账号池 permission 变量并覆盖 Admin/Root/普通用户基线测试。 |
| 账号池路由 | `router/account_pool-router.go`、`router/api-router.go` | 新增独立账号池路由注册表，主路由只保留 `registerAccountPoolRoutes(apiRouter)` 调用。 |
| 路由测试 | `router/account_pool_router_test.go` | 覆盖核心账号池 handler、权限分类和全路由表非空 permission。 |
| 既有中间件 | `middleware/auth.go` | 继续复用上一轮 `RequirePermission`，本轮不改认证流程。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录账号池作为 NexusTok 独有能力接入服务端权限 enforcement 的需求、风险、方案和落地状态。 |

### 风险评估

1. 账号池承载真实凭证，服务端 enforcement 会让普通 Admin 无法直接调用凭证导入、账号创建/删除、凭证刷新、OAuth 完成等高风险接口；这是与前端 `account_pool.sensitive_write` 矩阵一致的预期收紧，但必须确保健康、日志、分组列表、账号列表和检测历史仍可读。
2. `/api/account-pool/groups/:id/accounts/export` 与 `/api/account-pool/state-logs/export` 是脱敏导出，当前响应明确 `credentials_exported=false` 并列出脱敏字段；本轮归为 `operate`，不误标为 `secret_view`，避免破坏已有排障工作流。
3. 账号池路由包含大量静态路径和参数路径，抽出后必须保持 Gin 路由匹配不被 `/:id`、`/:account_id` 这类参数路径抢占；需要路由结构测试覆盖 `/auth-files/:auth_file_id`、`/groups/options`、`/groups/:id/accounts`、`/check-tasks/:check_task_id` 等关键路径。
4. 账号池前端当前主要按 read 能力显示入口，按钮级细分尚未完全消费；服务端先落地 enforcement 后，后续要补默认前端按钮/表单级显隐，避免普通 Admin 看到不可用的高风险操作。
5. 本轮不改 controller、model、service、数据库迁移、审计模型和前端页面，因此核心业务路径只增加权限中间件，不应改变账号池调度、检测、OAuth、导入和导出的业务语义。

### 方案评审

采用与渠道路由相同的灰度方案：复用 `middleware.RequirePermission`，在 `AdminAuth` 后按账号池权限表做二次校验；不引入 Casbin、不新增数据库表、不改变现有 Root/Admin 基线。路由表放在独立文件内，便于后续账号池按钮级权限、用户 override 和 Casbin policy 接入时统一维护。分类上优先保护凭证生命周期：任何写入、替换、删除或刷新凭证材料的入口都归入 `sensitive_write`；只影响运行时健康检查或脱敏导出的动作归入 `operate`；常规分组配置为 `write`；查询与脱敏日志为 `read`。

验收方式：

1. `go test ./service/authz ./middleware ./router ./controller` 覆盖账号池 permission、权限中间件、路由结构和 controller 构建。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认默认前端类型和账号池页面不受影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 打开 `http://192.168.0.202:3003/`，访问 `/account-pool/overview` 或 `/account-pool/groups`，确认页面更新生效、`/api/account-pool/health` 或 `/api/account-pool/groups/options` 正常返回、控制台无新增 error/warn。
5. 不在真实环境触发账号删除、凭证导入或 OAuth 完成等高风险写接口；这些权限收紧只通过 httptest/路由表测试验证。

## 本轮实施评审：订阅 Admin Authz 路由权限表灰度 enforcement

### 需求分析

`new-api-main` 的平台治理方向已经把管理资源从粗粒度 Admin/Root 角色逐步拆成资源动作权限。NexusTok 当前已把渠道与账号池接入服务端权限表，但 `/api/subscription/admin` 仍是一个整体 `AdminAuth` 路由组：管理员可直接调用套餐创建/更新、状态切换、绑定用户订阅、创建用户订阅、失效和删除用户订阅等接口。默认前端和 `/api/user/self` 已经有 `subscription.read/operate/write/sensitive_write` 权限矩阵，服务端还需要消费同一语义，避免前端隐藏后仍可直接调用高风险订阅管理接口。

本轮目标是把已有订阅管理能力纳入 NexusTok 原生 Authz enforcement：

1. 为 `subscription` 补齐稳定 permission 变量：`SubscriptionRead`、`SubscriptionOperate`、`SubscriptionWrite`、`SubscriptionSensitiveWrite`。
2. 将 `/api/subscription/admin` 从主 `api-router.go` 抽到独立路由文件，用权限表注册当前 NexusTok 已有接口，保持 path、method、handler 和业务逻辑不变。
3. 管理员读取套餐和用户订阅归为 `subscription.read`；套餐创建/更新/启停归为 `subscription.write`；绑定订阅、为用户创建订阅、失效订阅归为 `subscription.operate`。
4. 删除用户订阅归为 `subscription.sensitive_write`，因为它会直接移除用户权益记录和审计链路，普通 Admin 不应仅凭前端绕过调用。
5. 本轮不新增 `new-api-main` 的计划/用户订阅 reset 接口，也不改用户侧购买、支付回调、余额支付和钱包溢出逻辑；这些属于后续独立功能评审。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz 判定 | `service/authz/permission.go`、`service/authz/permission_test.go` | 新增订阅 permission 变量并补 Root/Admin/普通用户基线断言。 |
| 订阅 Admin 路由 | `router/subscription-router.go`、`router/api-router.go` | 新增独立订阅 Admin 路由权限表，主路由只保留注册调用；用户购买和支付回调路由不变。 |
| 路由测试 | `router/subscription_router_test.go` | 覆盖套餐、用户订阅管理 handler 和权限分类。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和落地状态，同时保留 reset 接口为后续能力项。 |

### 风险评估

1. `/api/subscription/admin` 是计费权益管理入口，路由级权限收紧会影响普通 Admin 直接删除用户订阅的能力；这是预期的敏感写保护，但必须确保套餐列表、套餐维护、绑定、创建和失效仍按现有 Admin 基线可用。
2. 套餐更新包含 `allow_balance_pay`、`allow_wallet_overflow`、金额、额度、时长和购买限制；本轮仍归入 `write`，不改业务字段校验，避免把订阅日常配置全部提升为 Root-only。
3. 删除用户订阅可能影响用户权益和钱包 fallback 行为，因此归入 `sensitive_write`；如果后续需要 Admin 删除能力，应通过 Casbin/user override 明确授予，而不是继续依赖粗粒度 Admin。
4. 路由抽出可能造成 `/subscription/admin/plans/:id`、`/users/:id/subscriptions` 和 `/user_subscriptions/:id` 匹配错误；需要路由结构测试覆盖。
5. 本轮不改数据库、不改 payment callback、不改用户侧购买和扣费事务；核心风险集中在管理 API 权限分类和前端按钮显隐未来需要同步。

### 方案评审

采用与渠道、账号池相同的灰度 enforcement 方案：复用 `middleware.RequirePermission` 和 `registerPermissionRoutes`，新增订阅 Admin 路由表，并保持 `subscriptionRoute` 用户侧购买路由继续使用 `UserAuth`、支付回调继续匿名签名/回调校验。这样能把 `new-api-main` 的资源动作权限优势转成 NexusTok 原生订阅治理能力，同时不改变已有计费交易路径。后续如果补齐 reset 接口，可直接在同一 `subscriptionPermissionRoutes` 中以 `SubscriptionOperate` 注册。

验收方式：

1. `go test ./service/authz ./middleware ./router ./controller` 覆盖订阅 permission、权限中间件、路由结构和 controller 构建。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认默认前端订阅管理类型不受影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 打开 `http://192.168.0.202:3003/subscriptions`，确认页面更新生效，`/api/subscription/admin/plans` 正常返回，控制台无新增 error/warn。
5. 不在真实环境触发删除用户订阅等高风险写接口；敏感写收紧通过路由表测试验证。

## 本轮实施评审：模型管理 Authz 路由权限表灰度 enforcement

### 需求分析

`new-api-main` 和 NexusTok 在模型管理基础路由上基本同源：两者都提供 `/api/vendors`、`/api/models`、`/api/deployments` 三组管理接口，用来维护厂商元数据、模型元数据、上游模型同步和 io.net 部署生命周期。NexusTok 在此基础上已经扩展了模型级定价配置接口 `/api/models/:id/pricing`，并且默认前端已经按 `model.read` 能力展示模型管理入口；但服务端仍只用粗粒度 `AdminAuth` 保护整组接口，尚未消费已经注册在 Authz catalog 中的 `model` 权限动作。

本轮目标是把 `new-api-main` 的平台治理方向转成 NexusTok 原生模型管理能力，而不是迁移新的页面框架：

1. 为 `model` 资源补齐稳定 permission 变量：`ModelRead`、`ModelOperate`、`ModelWrite`、`ModelSensitiveWrite`，让路由、中间件测试和未来前端按钮级权限使用同一常量。
2. 将 `/api/vendors`、`/api/models`、`/api/deployments` 从主 `api-router.go` 抽到独立模型管理路由文件，并用权限表注册全部现有路径，保持 HTTP method、path、handler 和业务返回不变。
3. 厂商、模型、部署列表和详情查询归为 `model.read`；连接测试、价格估算、名称检查、上游同步预览、部署扩容这类运行操作归为 `model.operate`。
4. 厂商/模型/部署创建和编辑、模型定价配置更新、上游同步落库归为 `model.write`；厂商、模型和部署删除归为 `model.sensitive_write`，避免普通 Admin 通过直接调用 API 移除关键元数据或部署资源。
5. 本轮不改变 controller、model、service、数据库迁移和前端页面；默认前端按钮级细分权限可以在后续独立切片中继续补齐。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz 判定 | `service/authz/permission.go`、`service/authz/permission_test.go` | 新增模型管理 permission 变量，并补充 Root/Admin/普通用户基线断言，保证未知权限仍 fail-closed。 |
| 模型管理路由 | `router/model-router.go`、`router/api-router.go` | 新增独立模型管理路由权限表，主路由只保留 `registerModelRoutes(apiRouter)` 调用，避免主路由继续堆积管理资源细节。 |
| 路由测试 | `router/model_router_test.go` | 覆盖 vendors/models/deployments 核心 handler、权限分类和全部路由资源归属。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收标准，作为后续模型页按钮级权限和系统设置路由迁移依据。 |

### 风险评估

1. 模型管理接口影响平台可用模型、价格和部署资源。服务端 enforcement 会让普通 Admin 直接调用删除模型、删除厂商、删除部署接口时被拒绝；这是预期的敏感写保护，但必须确认查询、连接测试、同步预览、创建和日常编辑仍按现有 Admin 基线可用。
2. `/api/models/sync_upstream` 会从上游同步并写入模型/厂商元数据，因此不能仅视为 operate；本轮归为 `model.write`，而只读预览 `/sync_upstream/preview` 保持 `model.operate`。
3. `/api/models/:id/pricing` 是 NexusTok 相对 `new-api-main` 的增强能力，更新模型定价会直接影响计费，但它属于管理员日常配置项；本轮归为 `model.write`，不提升到 Root-only，避免破坏现有模型管理工作流。更细的计费权限拆分留给后续系统设置/计费设置权限切片。
4. `/api/deployments/:id/extend` 会改变部署运行容量但不删除资源，本轮归为 `model.operate`；创建、更新、重命名部署归为 `model.write`，删除部署归为 `model.sensitive_write`。
5. 三个路由组包含 `/search`、`/settings`、`/:id`、`/:id/pricing`、`/:id/containers/:container_id` 等静态和参数路径，抽出后必须通过路由结构测试确认 Gin 匹配顺序不变。
6. 本轮不改数据库、不改部署 service、不触发真实部署删除或扩容；核心风险集中在路由权限分类、handler 挂接错误和默认前端页面读取接口被误拦截。

### 方案评审

采用与渠道、账号池和订阅 Admin 相同的灰度 enforcement 方案：复用 `permissionRoute`、`registerPermissionRoutes` 和 `middleware.RequirePermission`，仍由 `AdminAuth` 负责登录态、用户状态和基础系统角色边界，权限表只在认证通过后做资源动作二次校验。这样能把 `new-api-main` 的管理资源分层治理优势沉淀为 NexusTok 原生 Authz 能力，并为后续 Casbin/user override 留出单一替换点。

路由按资源动作分类如下：

| 路由组 | 权限 | 路径 |
|------|------|------|
| `/api/vendors` | `model.read` | `GET /`、`GET /search`、`GET /:id` |
| `/api/vendors` | `model.write` | `POST /`、`PUT /` |
| `/api/vendors` | `model.sensitive_write` | `DELETE /:id` |
| `/api/models` | `model.read` | `GET /missing`、`GET /`、`GET /search`、`GET /:id/pricing`、`GET /:id` |
| `/api/models` | `model.operate` | `GET /sync_upstream/preview` |
| `/api/models` | `model.write` | `POST /sync_upstream`、`PUT /:id/pricing`、`POST /`、`PUT /` |
| `/api/models` | `model.sensitive_write` | `DELETE /:id` |
| `/api/deployments` | `model.read` | `GET /settings`、`GET /`、`GET /search`、`GET /hardware-types`、`GET /locations`、`GET /available-replicas`、`GET /check-name`、`GET /:id`、`GET /:id/logs`、`GET /:id/containers`、`GET /:id/containers/:container_id` |
| `/api/deployments` | `model.operate` | `POST /settings/test-connection`、`POST /test-connection`、`POST /price-estimation`、`POST /:id/extend` |
| `/api/deployments` | `model.write` | `POST /`、`PUT /:id`、`PUT /:id/name` |
| `/api/deployments` | `model.sensitive_write` | `DELETE /:id` |

验收方式：

1. `go test ./service/authz ./middleware ./router ./controller` 覆盖模型 permission、权限中间件、路由结构和 controller 构建。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认默认前端模型页面类型不受影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 打开 `http://192.168.0.202:3003/models/metadata`，确认页面更新生效，`/api/models/`、`/api/vendors/` 等读取请求正常返回，控制台无新增 error/warn。
5. 不在真实环境触发删除模型、删除厂商、删除部署或扩容部署等高风险操作；这些权限分类通过路由表测试验证。

## 本轮实施评审：Waffo Pancake 用户侧支付路由补齐

### 需求分析

对照 `/opt/project/new-api-main` 的用户路由发现，new-api 已在用户自助充值路由中注册 `POST /api/user/waffo-pancake/amount` 和 `POST /api/user/waffo-pancake/pay`。NexusTok 当前已经具备 Waffo Pancake 的 controller、service、支付可用性判断、webhook、系统设置表单和默认前端钱包调用，但主路由只注册了 `/api/waffo-pancake/webhook` 回调，没有注册用户侧金额试算和发起支付入口。默认前端 `web/default/src/features/wallet/api.ts` 已经调用这两个路径，因此当前页面在启用 Waffo Pancake 充值时会命中 404 或无法进入支付链路。

本轮目标是把 new-api 已有的用户侧支付入口补齐为 NexusTok 原生充值能力：

1. 在 `/api/user` 的用户认证路由组中注册 `POST /waffo-pancake/amount` 和 `POST /waffo-pancake/pay`。
2. 金额试算接口复用 `controller.RequestWaffoPancakeAmount`，支付发起接口复用 `controller.RequestWaffoPancakePay` 并保留 `CriticalRateLimit`，与 Waffo、Stripe、Creem 等支付入口保持一致。
3. 只补路由，不改 Waffo Pancake 签名、webhook 验签、订单创建、充值入账、额度饱和保护、系统设置表单和前端文案。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 用户支付路由 | `router/api-router.go` | 在已认证用户自助支付路由组下补齐 Waffo Pancake 金额试算和支付发起入口。 |
| 路由测试 | `router/api_router_payment_test.go` | 覆盖新路由绑定到正确 handler；`CriticalRateLimit` 通过代码路径与其他支付入口保持同一注册模式。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录该差异来源、风险边界、方案和落地状态。 |

### 风险评估

1. 这是补齐缺失路由，不改变 handler 内部支付逻辑；如果 Waffo Pancake 未配置或未启用，handler 仍按现有业务规则返回“未启用/配置缺失”，不会绕过配置开关。
2. `/waffo-pancake/pay` 会创建支付会话，因此必须与其他支付发起接口一样保留 `CriticalRateLimit`，避免用户侧高频创建订单。
3. 路由位于 `selfRoute` 内，仍需要 `UserAuth`；匿名用户不能直接发起金额试算或支付。
4. Webhook 仍是匿名回调入口并已有 `anonymousRequestBodyLimit` 与验签逻辑，本轮不触碰，避免把用户侧路由和上游回调边界混淆。
5. 本轮不触发真实支付，不创建真实订单；验收时只验证页面加载和接口到达 handler，若当前环境未配置支付，则业务返回配置缺失属于预期。

### 方案评审

采用最小补齐方案：在现有 `/api/user` 自助支付路由组中，紧邻 Waffo 支付路由注册 Waffo Pancake 的金额试算和支付发起接口。该方案与 new-api 路由保持兼容，也与默认前端当前 API 调用保持一致，不新增抽象、不改数据库、不改支付状态机。路由结构测试只验证 handler 绑定，真实支付行为继续由已有 controller/service 测试和 MCP 直调低风险响应覆盖。

验收方式：

1. `go test ./router ./controller` 覆盖路由注册和 controller 构建。
2. `go test ./service/authz ./middleware ./router ./controller` 作为本轮路由改动的目标包回归。
3. `cd web/default && ./node_modules/.bin/tsc -b` 确认钱包前端类型不受影响。
4. `git diff --check` 确认无空白错误。
5. 使用 MCP 打开 `http://192.168.0.202:3003/wallet`，确认页面更新生效、控制台无新增 error/warn；在浏览器上下文带 `NexusTok-User: 1` 调用 `POST /api/user/waffo-pancake/amount`，确认不再是 404/未注册路由，且在当前配置状态下返回业务层结果。

## 本轮实施评审：用户管理 Authz 路由权限表灰度 enforcement

### 需求分析

`new-api-main` 和 NexusTok 的 `/api/user` 管理员接口基本同源，均覆盖用户列表、搜索、详情、创建、更新、删除、启停、额度调整、充值记录、管理员完成充值、Passkey 重置、2FA 强制禁用和第三方绑定清理。NexusTok 已经在 Authz catalog 中注册了 `user.read/operate/write/sensitive_write`，默认前端侧边栏也已经按 `user.read` 控制“用户”入口，但服务端管理员子路由仍是整组 `AdminAuth`，没有消费同一套资源动作权限。用户管理是平台治理中风险最高的管理资源之一，直接关系到账户、额度、认证绑定和管理员身份边界，应继续吸收 new-api 的管理资源分层思路，转成 NexusTok 原生服务端 enforcement。

本轮目标：

1. 为 `user` 资源补齐稳定 permission 变量：`UserRead`、`UserOperate`、`UserWrite`、`UserSensitiveWrite`。
2. 将 `/api/user` 中的管理员子路由抽成独立权限表注册，保留公开登录、用户自身操作、支付回调和用户自助支付路由原有边界。
3. 用户列表、搜索、详情、充值记录、OAuth 绑定查看、2FA 统计归为 `user.read`。
4. 重置 Passkey、强制禁用 2FA、解绑 OAuth/账号绑定、普通启停等运营动作归为 `user.operate`。
5. 更新普通用户资料、分组、状态和非越权角色字段归为 `user.write`，继续依赖 controller 内已有“不能操作同级或更高级用户”的业务校验。
6. 创建用户、硬删除用户、管理员完成充值、以及 `POST /api/user/manage` 中的删除、升降级和额度调整归为 `user.sensitive_write`；由于 `manage` 是复合动作，路由级先按 `user.operate` 放行，再在 handler 内按请求体动作做敏感写二次校验。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz 判定 | `service/authz/permission.go`、`service/authz/permission_test.go` | 新增用户管理 permission 变量，并补 Root/Admin/普通用户基线断言。 |
| 用户 Admin 路由 | `router/user-router.go`、`router/api-router.go` | 将管理员子路由从主路由内联迁移到权限表注册；公开登录、自助资料、充值支付和 webhook 不变。 |
| 复合动作二次校验 | `controller/user_authz.go`、`controller/user.go`、`controller/user_authz_test.go` | 为 `ManageUser` 的删除、升降级和额度调整增加 `user.sensitive_write` 二次校验，避免单一路由权限误放宽高风险动作。 |
| 路由测试 | `router/user_router_test.go` | 覆盖用户 Admin 路由 handler 保持不变、权限分类和资源归属。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收标准，作为后续用户页按钮级权限消费依据。 |

### 风险评估

1. 用户管理接口包含账户和额度高风险动作；服务端 enforcement 会让普通 Admin 直接调用创建用户、硬删除用户、管理员完成充值，以及 `manage` 里的删除、升降级、额度调整时被拒绝。这是预期的敏感写收紧，Root 仍可执行。
2. `POST /api/user/manage` 同时承担启用/禁用、删除、升降级和额度调整。如果整条路由直接标为 `user.sensitive_write`，会把普通启停也提升为 Root-only；如果只标为 `user.operate`，又会放宽删除和额度动作。因此必须增加 handler 内基于 `req.Action` 的二次校验。
3. `UpdateUser` 仍归为 `user.write`，并保留 controller 内同级/更高级用户校验和目标角色校验；本轮不扩大 Admin 对管理员或 Root 的操作能力。
4. `AdminResetPasskey` 目前没有像 2FA 禁用那样显式检查目标用户层级；路由级归为 `user.operate` 后仍保持现状，不在本轮顺手改变业务语义。后续可单独评审是否补同级/更高级保护。
5. 用户自助支付、登录、注册、Passkey 自助、2FA 自助、OAuth 自助绑定、充值 webhook 不属于管理员资源权限表，本轮不改，避免影响普通用户路径。
6. 默认前端用户页当前主要按 `user.read` 控制入口，按钮级细分尚未全部消费；服务端先收紧后，后续需要补用户页按钮/菜单级显隐，避免普通 Admin 看到不可用的敏感动作。

### 方案评审

采用与渠道、账号池、订阅和模型管理相同的灰度 enforcement：复用 `permissionRoute`、`registerPermissionRoutes` 和 `middleware.RequirePermission`，保留 `AdminAuth` 作为第一层认证边界。管理员子路由独立到 `router/user-router.go`，主路由只负责公开、自助和支付路径，降低 `api-router.go` 的管理资源堆积。复合动作 `ManageUser` 使用 `userManageActionNeedsSensitiveWrite` 做动作级敏感分类，后续接入 Casbin/user override 时会自动复用 `authz.Can` 的统一判定。

路由按资源动作分类如下：

| 路由 | 权限 | 说明 |
|------|------|------|
| `GET /api/user/`、`GET /api/user/search`、`GET /api/user/:id` | `user.read` | 用户列表、搜索和详情。 |
| `GET /api/user/topup` | `user.read` | 管理员查看充值记录。 |
| `GET /api/user/:id/oauth/bindings`、`GET /api/user/2fa/stats` | `user.read` | 查看认证绑定和 2FA 统计。 |
| `DELETE /api/user/:id/oauth/bindings/:provider_id`、`DELETE /api/user/:id/bindings/:binding_type` | `user.operate` | 清理绑定，仍保留 controller 内目标用户层级校验。 |
| `DELETE /api/user/:id/reset_passkey`、`DELETE /api/user/:id/2fa` | `user.operate` | 账号安全运营动作。 |
| `POST /api/user/manage` | `user.operate` + 动作级二次校验 | 启停为 operate；删除、升降级、额度调整额外要求 sensitive_write。 |
| `PUT /api/user/` | `user.write` | 编辑普通用户资料/分组/状态等，继续保留层级校验。 |
| `POST /api/user/`、`DELETE /api/user/:id`、`POST /api/user/topup/complete` | `user.sensitive_write` | 创建/删除账号和手工完成充值属于高风险写。 |

验收方式：

1. `go test ./service/authz ./middleware ./router ./controller` 覆盖用户 permission、权限中间件、路由结构和 `ManageUser` 敏感动作分类。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认默认前端类型不受影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 打开 `http://192.168.0.202:3003/users`，确认页面更新生效，`/api/user/` 和 `/api/user/search` 等读取请求正常返回，控制台无新增 error/warn。
5. 不在真实环境触发创建、删除、完成充值、升降级和额度调整等高风险动作；这些敏感写收紧通过单元测试和路由表测试验证。

## 本轮实施评审：兑换码 Authz 独立资源与路由权限表灰度 enforcement

### 需求分析

`new-api-main` 和 NexusTok 都提供 `/api/redemption` 管理接口，用于创建、查询、更新和删除兑换码。兑换码会把预设额度发放给最终用户，是钱包与额度体系的一部分；NexusTok 当前默认前端已经有独立的“兑换码”页面，但权限入口仍复用 `user.read`，服务端路由也仍是整组 `AdminAuth`。这会让用户管理权限和兑换码管理权限耦合在一起，不利于后续把用户账户治理、钱包额度治理和兑换码发行治理拆成原生能力。

本轮目标是把兑换码管理从用户资源中拆出来，成为 NexusTok Authz catalog 的独立 `redemption` 资源：

1. 新增 `redemption` 资源及稳定 permission 变量：`RedemptionRead`、`RedemptionOperate`、`RedemptionWrite`、`RedemptionSensitiveWrite`。
2. 将 `/api/redemption` 从主 `api-router.go` 抽到独立路由文件，用权限表注册所有现有管理接口，保持 path、method、handler 和业务返回不变。
3. 默认前端权限 helper 和侧边栏/路由守卫改为读取 `redemption.read`，不再把兑换码入口绑在 `user.read` 上。
4. 兑换码列表、搜索、详情归为 `redemption.read`；启停状态更新和删除无效兑换码等维护动作归为 `redemption.operate` 或 `redemption.sensitive_write`；创建和普通编辑归为 `redemption.write`，以保持当前 Admin 维护流程兼容；删除单个兑换码归为 `redemption.sensitive_write`。
5. 本轮不改变用户兑换兑换码的普通用户路径、不改兑换事务、不改额度入账、不改兑换码数据模型和数据库迁移。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz catalog | `service/authz/permission.go`、`service/authz/registry.go`、`service/authz/permission_test.go` | 新增 `redemption` 资源、permission 变量和 Root/Admin/普通用户基线测试。 |
| 兑换码 Admin 路由 | `router/redemption-router.go`、`router/api-router.go`、`router/redemption_router_test.go` | 将 `/api/redemption` 迁移为权限表注册，主路由只保留注册调用。 |
| 默认前端权限入口 | `web/default/src/lib/admin-permissions.ts`、`web/default/src/hooks/use-sidebar-data.ts`、`web/default/src/routes/_authenticated/redemption-codes/index.tsx`、`web/default/src/components/layout/components/app-sidebar.tsx`、`web/default/src/lib/admin-permissions.test.ts` | 增加 `redemption` 资源常量和 Admin fallback，侧边栏与页面守卫按 `redemption.read` 判断。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收标准，作为后续兑换码按钮级权限消费依据。 |

### 风险评估

1. 新增 Authz 资源会改变 `/api/authz/catalog` 和 `/api/user/self` 返回的权限矩阵；前端必须同步新增 `redemption` 资源 fallback，避免旧角色用户在缺少显式矩阵时丢失兑换码入口。
2. 兑换码创建会生成可兑换额度，理论上可以进一步提升到 `sensitive_write`；但当前 Admin 已经能创建兑换码，默认前端也没有按钮级 `redemption.sensitive_write` 消费。本轮先归入 `redemption.write` 保持兼容，后续可在按钮级权限和审批流补齐后再做更细收紧。
3. 删除单个兑换码会移除可审计记录，归入 `redemption.sensitive_write`；删除无效兑换码是批量清理 used/disabled/expired 记录，仍有审计影响，本轮同样归入 `redemption.sensitive_write`，不在真实环境触发。
4. `PUT /api/redemption/` 同时承担普通编辑和 `status_only=true` 状态启停，路由级无法按 query 参数分拆；本轮统一归入 `redemption.write`，保持兼容。后续可拆专用状态接口或在 handler 内做动作级二次校验。
5. 兑换码管理路由包含 `/search`、`/invalid` 和 `/:id`，抽出后需要测试确认静态路径不会被参数路径抢占。

### 方案评审

采用独立资源方案，而不是继续复用 `user` 或 `system_setting`：兑换码既不是用户资料，也不是系统全局配置，而是钱包额度发行工具。后端复用已有 `permissionRoute` 和 `registerPermissionRoutes`，路由仍先经过 `AdminAuth`，再按 `redemption` 动作二次校验。前端只调整权限资源常量、侧边栏和页面守卫，不新增文案、不改 UI 结构。这样能把 new-api 的兑换码管理能力转成 NexusTok 更精确的原生治理边界。

路由分类：

| 路由 | 权限 | 说明 |
|------|------|------|
| `GET /api/redemption/`、`GET /api/redemption/search`、`GET /api/redemption/:id` | `redemption.read` | 列表、搜索和详情。 |
| `POST /api/redemption/`、`PUT /api/redemption/` | `redemption.write` | 创建、普通编辑和状态更新；保持当前 Admin 工作流兼容。 |
| `DELETE /api/redemption/invalid`、`DELETE /api/redemption/:id` | `redemption.sensitive_write` | 删除兑换码记录或批量清理无效记录。 |

验收方式：

1. `go test ./service/authz ./middleware ./router ./controller` 覆盖 permission、权限中间件、兑换码路由结构和 controller 构建。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认默认前端权限资源类型和兑换码页面守卫不受影响。
3. `node --test src/lib/admin-permissions.test.ts` 或等价前端测试覆盖 Admin fallback。
4. `git diff --check` 确认无空白错误。
5. 使用 MCP 打开 `http://192.168.0.202:3003/redemption-codes`，确认页面更新生效，`/api/redemption/` 读取请求正常返回，控制台无新增 error/warn；只直调低风险读接口，不触发真实删除。

## 本轮实施评审：日志与用量数据 Authz 独立资源灰度 enforcement

### 需求分析

`new-api-main` 对 `/api/log` 的历史日志删除已提升到 Root 边界，同时 NexusTok 近期已经原生化了全局管理操作审计兜底、Usage Logs 移动端体验和 Dashboard Flow 流量账本可视化。NexusTok 当前 `/api/log` 与 `/api/data` 仍在主 `api-router.go` 中内联注册，管理员跨用户日志查询、统计、搜索、渠道亲和用量缓存统计，以及跨用户用量聚合/flow 查询都只依赖 `AdminAuth`。随着 Authz catalog 已覆盖渠道、账号池、订阅、模型、用户和兑换码，日志与用量数据也应从泛化 Admin 角色中拆出，形成 NexusTok 原生的审计/统计读取边界。

本轮目标：

1. 新增 `usage_log` 与 `usage_data` 两个 Authz 资源，分别承载跨用户请求日志/审计日志读取、历史日志清理，以及跨用户用量统计/flow 聚合读取。
2. 将 `/api/log` 管理员接口和 `/api/data` 管理员接口从主路由抽到独立路由文件，用现有权限表模式注册。
3. 保留 `/api/log/self`、`/api/log/self/stat`、`/api/log/self/search`、`/api/log/token`、`/api/data/self`、`/api/data/flow/self` 的原认证边界，避免普通用户自助查询被管理员资源权限表误拦截。
4. `GET /api/log/`、`GET /api/log/stat`、`GET /api/log/search`、`GET /api/log/channel_affinity_usage_cache` 归为 `usage_log.read`；`DELETE /api/log/` 归为 `usage_log.sensitive_write`，与 new-api 的 Root-only 删除意图对齐。
5. `GET /api/data/`、`GET /api/data/users`、`GET /api/data/flow` 归为 `usage_data.read`；本轮不新增数据写接口。
6. 默认前端在 Usage Logs 和 Dashboard 中判断“跨用户视图”时读取对应 read 权限，而不是只看角色是否为 Admin，为后续 Casbin/user override 提前消除前后端分叉。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz catalog | `service/authz/permission.go`、`service/authz/registry.go`、`service/authz/permission_test.go`、`controller/user_authz_test.go` | 新增 `usage_log`、`usage_data` 资源和 permission 变量，补 Root/Admin/普通用户基线矩阵断言。 |
| 日志/数据路由 | `router/log-data-router.go`、`router/api-router.go`、`router/log_data_router_test.go` | 将管理员日志和管理员用量数据路由迁移到权限表注册；普通用户 self 与 token 路径保留在主路由。 |
| 默认前端权限消费 | `web/default/src/lib/admin-permissions.ts`、`web/default/src/lib/admin-permissions.test.ts`、`web/default/src/features/usage-logs/*`、`web/default/src/features/dashboard/*` | 增加资源 fallback，并让 Usage Logs/Dashboard 的跨用户请求按 `usage_log.read`、`usage_data.read` 判断。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收标准，作为后续日志删除按钮、flow 用户/节点筛选和管理审计页面权限消费依据。 |

### 风险评估

1. `/api/log` 同时服务普通用户 Usage Logs、管理员全站 Usage Logs、系统设置日志清理和 classic 前端旧入口；路由迁移时必须明确 self/token 路径不进入管理员权限表，避免普通用户日志页回退。
2. `DELETE /api/log/` 会实际删除历史请求日志和审计日志，是不可逆数据清理。本轮将其从普通 Admin 能力收紧为 `usage_log.sensitive_write`，等价于当前 Root 基线；这会让非 Root Admin 直接调用旧同步删除接口时被拒绝。默认前端系统设置仍是 Root-only，推荐继续优先使用 `/api/system-task/log-cleanup`。
3. `/api/log/channel_affinity_usage_cache` 位于系统设置的渠道亲和缓存弹窗中，但返回的是特定规则/key 的用量缓存统计，属于日志/用量观测而非系统设置写操作；归入 `usage_log.read` 后，缺少该权限的未来自定义管理员不应看到跨用户缓存详情。
4. `/api/data/flow` 中 Root 可以看到 node/token/channel 维度，Admin 会在 controller/model 层隐藏 token/node 维度；本轮不改变这些脱敏规则，只在路由前增加 `usage_data.read`。
5. Dashboard 和 Usage Logs 前端从角色判断切到权限判断后，旧后端缺少新资源矩阵时需要依赖本地 Admin fallback，否则热更新过程中可能把管理员误降级为普通用户视图。
6. 本轮不改日志模型、统计 SQL、flow 聚合、审计写入和数据库迁移，避免影响 Relay 热路径与已有报表结果。
7. 子 Agent 复核额外发现 `/api/data/self` 与 `/api/data/?username=...` 的基础用量查询仍可能返回比 Dashboard 所需更细的原始 `quota_data` 维度，后续应参考 `new-api-main` 的聚合写法做单独评审；`SumUsedQuota` 空结果未统一 `COALESCE` 也应作为统计稳定性修复单独处理。

### 方案评审

采用独立资源方案，而不是继续复用 `system_setting.read`：日志和用量数据是平台审计/运营观测资源，既会暴露跨用户请求、成本、IP、Token 名称、渠道链路和管理员操作审计，也会服务 Dashboard Flow 等非系统配置页面。把它们从系统设置中拆出，有利于后续创建“审计员/财务观察员”等只读管理角色。后端仍复用 `permissionRoute`、`registerPermissionRoutes` 和 `middleware.RequirePermission`；路由先过 `AdminAuth`，再按 `usage_log`/`usage_data` 动作二次校验。前端只调整权限判断，不新增文案、不改变页面布局。

路由分类：

| 路由 | 权限 | 说明 |
|------|------|------|
| `GET /api/log/`、`GET /api/log/stat`、`GET /api/log/search` | `usage_log.read` | 管理员跨用户日志列表、统计和兼容搜索入口。 |
| `GET /api/log/channel_affinity_usage_cache` | `usage_log.read` | 渠道亲和用量缓存详情，暴露跨用户/key 维度观测数据。 |
| `DELETE /api/log/` | `usage_log.sensitive_write` | 同步删除历史日志；与 new-api 的 Root-only 删除边界对齐。 |
| `GET /api/data/`、`GET /api/data/users`、`GET /api/data/flow` | `usage_data.read` | 管理员跨用户用量趋势、用户排行和流量账本聚合查询。 |
| `GET /api/log/self*`、`GET /api/log/token`、`GET /api/data/self`、`GET /api/data/flow/self` | 不纳入管理员权限表 | 普通用户或 Token 自身认证路径，继续保持原有最小可见范围。 |

验收方式：

1. `go test ./service/authz ./middleware ./router ./controller` 覆盖 permission、权限中间件、日志/数据路由结构和 controller 构建。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认 Usage Logs/Dashboard 权限判断类型正确。
3. `cd web/default && ./node_modules/.bin/tsx --test src/lib/admin-permissions.test.ts` 覆盖 Admin fallback。
4. `git diff --check` 确认无空白错误。
5. 使用 MCP 打开 `http://192.168.0.202:3003/usage-logs/common` 与 `http://192.168.0.202:3003/dashboard/flow`，确认页面更新生效、管理员读接口返回 200、控制台无新增 error/warn；只验证低风险读接口，不触发 `DELETE /api/log/`。

## 本轮实施评审：基础用量查询聚合与日志统计空值保护

### 需求分析

子 Agent 复核和本地代码对照都确认：`new-api-main` 已经把基础 Dashboard 用量查询聚合到 `user/model/hour` 维度，而 NexusTok 当前 `GetQuotaDataByUsername` 与 `GetQuotaDataByUserId` 仍直接返回 `quota_data` 原始行。由于 NexusTok 为 Flow/Sankey 已扩展同一张表的 `node_name`、`token_id`、`use_group`、`channel_id` 等维度，基础 `/api/data/self` 和 `/api/data?username=...` 如果继续直接 `Find`，会把 Dashboard 不需要的细维度行带回前端，也可能让同一用户同一模型同一小时因为不同 token/channel/group 拆成多行，造成基础图表重复统计或泄露路由拓扑细节。

同时，`new-api-main` 在日志统计里使用 `COALESCE(sum(...), 0)`，而 NexusTok 的 `SumUsedQuota` 仍使用裸 `sum(...)`，`SumUsedToken` 还使用 PostgreSQL 不支持的 `ifnull(...)`。在空结果、筛选条件无匹配或 PostgreSQL 日志库场景下，统计接口应稳定返回 0，而不是依赖数据库方言或扫描空值的偶然行为。

本轮目标：

1. 将 `GetQuotaDataByUsername` 和 `GetQuotaDataByUserId` 改为聚合查询，只返回 `user_id`、`username`、`model_name`、`created_at` 与 count/quota/token 汇总。
2. 保持 `/api/data/flow` 和 `/api/data/flow/self` 的 Flow 专用维度不变；基础 Dashboard 查询与 Flow 查询分工明确。
3. 将 `SumUsedQuota` 的 quota、rpm/tpm 查询改为 `COALESCE`，确保无匹配日志时返回 0。
4. 将 `SumUsedToken` 的 `ifnull` 改为跨 SQLite/MySQL/PostgreSQL 通用的 `COALESCE`。
5. 不修改日志写入、quota_data 写入、Flow 聚合、前端页面结构和权限中间件。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 用量数据模型 | `model/usedata.go`、`model/usedata_flow_test.go` | 基础用户/用户名用量查询改为聚合，补测试确认不会返回 token/channel/node/use_group 细维度。 |
| 日志统计模型 | `model/log.go`、`model/log_cleanup_test.go` 或独立日志统计测试 | 日志统计 sum 使用 `COALESCE`，补空结果和跨字段统计测试。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录该安全/稳定性差异、方案、风险和验收标准。 |

### 风险评估

1. 基础用量查询从原始行改为聚合行后，返回行数可能减少，这是预期行为；Dashboard 图表本来只需要模型/时间聚合，不应依赖 token/channel/group 细节。
2. 管理员带 `username` 查询会保留 `user_id` 和 `username`，普通 `/api/data/self` 也会保留当前用户自己的 `user_id` 和 `username`，但不会返回 node/token/channel/use_group 明细。
3. Flow/Sankey 仍继续使用 `GetFlowQuotaData`，Root/Admin/User 维度脱敏规则不变，本轮不会削弱已经落地的 Flow 观测能力。
4. `COALESCE` 是 SQLite、MySQL 和 PostgreSQL 都支持的标准函数，比 `ifnull` 更符合本项目三库兼容要求。
5. 本轮不处理日志搜索通配语义和 ClickHouse 日志库兼容；这些属于更大范围的日志后端设计，避免和基础统计稳定性混在一起。

### 方案评审

采用最小模型层修复：参考 `new-api-main` 的聚合字段和 `GROUP BY` 维度，在 NexusTok 保留现有函数名与 controller 契约，只改变基础用量查询的 SQL 投影；同时把日志统计的空值保护集中到 `model/log.go`。这样前端 API 路径、响应结构、权限边界和数据库迁移都不变，风险集中在聚合结果语义，且可用模型层单元测试直接覆盖。

验收方式：

1. `go test ./model ./controller` 覆盖基础用量聚合、Flow 维度不回退、日志统计空结果和 controller 构建。
2. `go test ./service/authz ./middleware ./router` 确认上轮 Authz 路由权限表不受影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/`、`/dashboard/models` 与 `/dashboard/flow`，确认页面更新生效；直调 `/api/data/self`、`/api/data/`、`/api/data/flow` 和 `/api/log/stat` 低风险读接口，确认均返回 200/业务 success，不触发任何写操作。

## 本轮实施评审：分组与预填充分组 Authz 路由权限表灰度 enforcement

### 需求分析

`new-api-main` 与 NexusTok 都保留 `/api/group` 和 `/api/prefill_group` 两组管理接口：前者返回系统配置中的用户组/模型组名称，供用户管理、订阅配置、渠道和模型配置选择；后者维护可复用的模型、标签、端点预填模板，是模型和渠道配置效率的一部分。NexusTok 当前已经把 `/api/vendors`、`/api/models` 和 `/api/deployments` 纳入 `model` 资源权限表，但 `/api/group` 与 `/api/prefill_group` 仍在主路由里只挂 `AdminAuth`，导致模型配置相关模板能力没有进入同一套原生 Authz enforcement。

本轮目标：

1. 将 `/api/group` 和 `/api/prefill_group` 从主 `api-router.go` 中抽出，用现有 `permissionRoute` 权限表注册。
2. 复用现有 `model` 资源，不新增独立 `group` 资源：`authz.ModelWrite` 的 catalog 描述已经包含 prefill groups，分组和预填模板都服务模型/渠道配置，不需要扩大前端权限矩阵。
3. `GET /api/group/` 与 `GET /api/prefill_group/` 归为 `model.read`；`POST /api/prefill_group/` 与 `PUT /api/prefill_group/` 归为 `model.write`；`DELETE /api/prefill_group/:id` 归为 `model.sensitive_write`。
4. 保持用户自助 `GET /api/user/self/groups`、`GET /api/user/groups` 原认证边界不变，避免普通用户 Playground/API Key 用户组选项被管理员权限表误拦截。
5. 不修改预填组数据模型、接口响应结构、前端页面布局和渠道/模型表单行为；只收紧管理员管理路径的授权表达。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 分组路由 | `router/group-router.go`、`router/api-router.go`、`router/group_router_test.go` | 新增集中路由注册，将 `/api/group` 和 `/api/prefill_group` 接入 `model` 权限表，并覆盖 handler 与权限分类。 |
| 模型 JSON helper 一致性 | `model/prefill_group.go` | 触碰预填组能力时同步修复 `Scan` fallback 中直接调用 `encoding/json.Marshal` 的项目规范问题，改为 `common.Marshal`；`json.RawMessage` 类型引用保留。 |
| 默认前端 | `web/default/src/features/models/*`、`web/default/src/features/channels/*`、`web/default/src/features/users/api.ts`、`web/default/src/features/subscriptions/api.ts` | 本轮不改 UI；模型页、渠道抽屉、用户和订阅页面继续调用原路径，服务端按 `model` 权限二次校验。后续可单独做模型页按钮级 `write/operate/sensitive_write` 消费。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收标准，并补充能力落地清单。 |

### 风险评估

1. `/api/group` 虽然也被用户管理和订阅管理页面复用，但它返回的是模型/计费组配置中的分组名称，而不是用户详情；归入 `model.read` 后，未来拥有用户读权限但没有模型读权限的自定义管理员将无法拉取完整分组选项。这是更偏最小暴露的选择，后续若出现“用户管理员需要分组列表但不能看模型配置”的角色，可新增只读 group 元数据资源或专用聚合接口。
2. `/api/prefill_group?type=model` 也被渠道编辑抽屉用于快速填充模型列表；渠道管理员如果未来只有 `channel.write` 没有 `model.read`，将看不到预填模板，但仍可以手动输入模型。当前 Admin 基线同时拥有 `model.read`，不会影响现有管理员工作流。
3. 删除预填组不会直接删除模型或渠道，但会移除可复用配置模板，可能影响团队后续配置效率；本轮归入 `model.sensitive_write`，与模型/厂商/部署删除保持相同敏感写边界。
4. `model/prefill_group.go` 中 `JSONValue.Scan` 的 fallback 从 `json.Marshal` 改为 `common.Marshal` 后，外部行为应保持一致；该路径只处理非 `nil`、非 `[]byte`、非 `string` 的数据库驱动返回值，属于低频兼容兜底。
5. 本轮不在真实环境触发预填组创建、更新、删除，只通过单元测试和只读接口验证，避免误改生产配置。

### 方案评审

采用“复用模型资源”的最小收敛方案，而不是新增 `group`/`prefill_group` Authz 资源：这样能让预填模板跟模型元数据、厂商元数据、部署和定价保持同一治理边界，也不会让前端权限矩阵突然增加一类用户难以理解的资源。实现上继续复用 `registerPermissionRoutes`，路由先经过 `AdminAuth`，再按 `model.read/write/sensitive_write` 二次校验；普通用户自助分组路径不进入该路由表。

路由分类：

| 路由 | 权限 | 说明 |
|------|------|------|
| `GET /api/group/` | `model.read` | 管理端读取模型/计费组名称，用于用户、订阅、渠道和模型配置下拉选项。 |
| `GET /api/prefill_group/` | `model.read` | 查看模型、标签、端点预填模板。 |
| `POST /api/prefill_group/`、`PUT /api/prefill_group/` | `model.write` | 创建和编辑预填模板，属于模型配置辅助写操作。 |
| `DELETE /api/prefill_group/:id` | `model.sensitive_write` | 删除可复用模板，按敏感写处理，避免普通 Admin 误删共享配置资产。 |

验收方式：

1. `go test ./model ./controller ./service/authz ./middleware ./router` 覆盖预填组模型构建、控制器构建、权限中间件和新路由表结构。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认前端原 API 调用路径和类型不受影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/` 和 `/models/metadata`，确认页面更新生效；直调 `/api/group/` 与 `/api/prefill_group` 低风险读接口，确认返回正常，不触发预填组写操作。

## 本轮实施评审：系统设置与运行维护 Authz 路由权限表灰度 enforcement

### 需求分析

`new-api-main` 与 NexusTok 都把系统选项、性能维护、自定义 OAuth 提供商、上游倍率同步等接口放在 Root 管理面。NexusTok 目前已经在 Authz catalog 中定义了 `system_setting` 资源，但 `/api/status/test`、`/api/option`、`/api/custom-oauth-provider`、`/api/performance` 和 `/api/ratio_sync` 仍散落在主 `api-router.go` 中，只依赖 `AdminAuth` 或 `RootAuth`。这会让系统设置页面、运行维护按钮和后续用户级 override 之间缺少统一的资源动作表达。

本轮目标：

1. 新增 `SystemSettingRead`、`SystemSettingOperate`、`SystemSettingWrite`、`SystemSettingSensitiveWrite`、`SystemSettingSecretView` permission 变量，复用既有 `system_setting` catalog。
2. 将 `/api/status/test`、`/api/option`、`/api/custom-oauth-provider`、`/api/performance`、`/api/ratio_sync` 从主路由抽到集中路由文件，并使用权限表注册。
3. 保留旧认证边界：`/api/status/test` 继续先过 `AdminAuth`，其他系统设置/维护路由继续先过 `RootAuth`；权限表只做第二层语义标注和未来扩展落点，不放宽任何 Root-only 路径。
4. 只做路由层收敛，不修改 option 更新逻辑、OAuth provider 存储、性能维护动作、倍率同步逻辑、前端系统设置页面和数据库模型。
5. `/api/system-info`、`/api/system-task` 已经有独立路由文件，本轮不混入；后续可以单独把系统信息读取和系统任务观测接入同一 `system_setting` 权限表。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz permission | `service/authz/permission.go` | 新增 `system_setting` 资源的稳定 permission 变量，供路由权限表复用。 |
| 系统设置/维护路由 | `router/system-setting-router.go`、`router/api-router.go`、`router/system_setting_router_test.go` | 抽出 `/status/test`、`/option`、`/custom-oauth-provider`、`/performance`、`/ratio_sync`，保留原 path、method、handler 和 Admin/Root 认证边界。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录系统设置路由接入 Authz 的需求、风险、方案和验收标准。 |

### 风险评估

1. `/api/status/test` 当前是 Admin 可访问的只读诊断接口，检查数据库连接并返回 HTTP 统计。本轮归入 `system_setting.read`，保留 Admin 可访问语义；如果归入 operate，会因当前 Admin baseline 不含 `system_setting.operate` 而改变旧行为。
2. `/api/option/` 的通用更新接口可以修改大量系统选项，包含 OAuth、支付、密钥类配置。路由层无法按 key 精细拆分，因此 `PUT /api/option/` 归入 `system_setting.sensitive_write`，旧 RootAuth 保留。后续如果要允许非 Root 管理员编辑非敏感选项，需要在 controller 内按 option key 做动作级二次校验。
3. 自定义 OAuth provider 创建/更新会触碰 client secret、OIDC 端点、准入策略和用户登录入口，统一归入 `system_setting.sensitive_write`；Discovery 拉取会访问外部 URL，归入 `system_setting.operate`，并继续保持 Root-only。
4. 性能维护中的 GC、磁盘缓存清理、统计重置属于运行操作，归入 `system_setting.operate`；服务器日志文件删除会移除运维证据，归入 `system_setting.sensitive_write`。
5. `ratio_sync/fetch` 会访问上游 URL 或渠道 BaseURL 并计算倍率差异，归入 `system_setting.operate`；`ratio_sync/channels` 返回可同步渠道及 BaseURL，归入 `system_setting.read`，但旧 RootAuth 继续防止普通 Admin 暴露底层地址。
6. 本轮不触发任何系统设置写操作、缓存清理、GC、日志删除或倍率同步；只通过低风险读接口和页面加载验证。

### 方案评审

采用“RootAuth 保底 + system_setting 权限表二次校验”的灰度接入方案：现有安全边界不降低，同时把系统设置与运行维护动作纳入同一套 Authz catalog。实现上沿用 `permissionRoute` 和 `registerPermissionRoutes`，为 Root-only 路由组设置 `RootAuth`，再按 read/operate/sensitive_write 挂权限；`/status/test` 作为只读诊断仍使用 `AdminAuth` + `system_setting.read`。

路由分类：

| 路由 | 权限 | 旧认证边界 | 说明 |
|------|------|------------|------|
| `GET /api/status/test` | `system_setting.read` | Admin | 只读诊断：数据库 ping 和 HTTP 统计。 |
| `GET /api/option/`、`GET /api/option/channel_affinity_cache` | `system_setting.read` | Root | 查看系统选项和缓存统计。 |
| `PUT /api/option/`、`POST /api/option/payment_compliance`、`POST /api/option/rest_model_ratio`、`POST /api/option/migrate_console_setting` | `system_setting.sensitive_write` | Root | 修改全局配置、合规状态、模型倍率和迁移旧配置。 |
| `DELETE /api/option/channel_affinity_cache` | `system_setting.operate` | Root | 清理渠道亲和缓存。 |
| `GET /api/custom-oauth-provider/`、`GET /api/custom-oauth-provider/:id` | `system_setting.read` | Root | 查看自定义 OAuth 提供商配置。 |
| `POST /api/custom-oauth-provider/discovery` | `system_setting.operate` | Root | 拉取 OIDC Discovery 配置。 |
| `POST /api/custom-oauth-provider/`、`PUT /api/custom-oauth-provider/:id`、`DELETE /api/custom-oauth-provider/:id` | `system_setting.sensitive_write` | Root | 创建、更新或删除登录入口和密钥配置。 |
| `GET /api/performance/stats`、`GET /api/performance/logs` | `system_setting.read` | Root | 查看运行性能和服务器日志文件列表。 |
| `DELETE /api/performance/disk_cache`、`POST /api/performance/reset_stats`、`POST /api/performance/gc` | `system_setting.operate` | Root | 运行维护动作。 |
| `DELETE /api/performance/logs` | `system_setting.sensitive_write` | Root | 删除服务器日志文件。 |
| `GET /api/ratio_sync/channels` | `system_setting.read` | Root | 查看可同步渠道和上游地址。 |
| `POST /api/ratio_sync/fetch` | `system_setting.operate` | Root | 拉取上游倍率并计算差异。 |

验收方式：

1. `go test ./service/authz ./middleware ./router ./controller` 覆盖 permission、权限中间件、系统设置路由结构和 controller 构建。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认默认前端系统设置页面不受影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/` 与 `/system-settings/site`，确认页面更新生效；直调 `GET /api/status/test`、`GET /api/option/`、`GET /api/performance/stats`、`GET /api/ratio_sync/channels` 等低风险读接口，确认返回 200/业务 success，不触发写操作、清理或外部同步。

## 本轮实施评审：系统信息与系统任务 Authz 路由权限表灰度 enforcement

### 需求分析

`new-api-main` 的系统信息/系统任务能力强调多实例运维观测和后台任务中心，NexusTok 已经把它们原生化到 `/system-info` 页面和 `/api/system-info`、`/api/system-task` 后端接口。但这两组路由目前仍只有 `RootAuth`，没有进入 `system_setting` 权限表；上一轮已完成系统选项、性能维护和倍率同步路由的 `system_setting` enforcement，本轮需要把同一系统运维域下的实例观测和系统任务观测补齐，避免 Authz catalog 与真实 Root-only 运维入口继续分叉。

本轮目标：

1. 将 `/api/system-info/instances` 接入 `system_setting.read`，保留 `RootAuth`，用于查看节点名、版本、资源占用和心跳状态。
2. 将 `/api/system-task/list`、`/api/system-task/current`、`/api/system-task/:task_id` 接入 `system_setting.read`，保留 `RootAuth`，用于查看后台任务输入、进度、结果和错误。
3. 将 `POST /api/system-task/log-cleanup` 接入 `system_setting.sensitive_write`，同时额外要求 `usage_log.sensitive_write`。该接口会创建异步日志清理任务，最终删除历史请求日志和审计证据，不能只按普通运维操作处理。
4. 不修改 SystemTask runner、任务模型、任务调度、日志清理 handler、系统信息前端页面和数据库迁移。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 系统信息路由 | `router/system-info-router.go`、`router/system_info_router_test.go` | `/api/system-info/instances` 改为 RootAuth + `system_setting.read` 权限表注册，handler 不变。 |
| 系统任务路由 | `router/system-task-router.go`、`router/system_task_router_test.go` | `/api/system-task/*` 改为 RootAuth + 权限表注册；日志清理任务额外叠加 `usage_log.sensitive_write`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收标准，并补充能力落地清单。 |

### 风险评估

1. 系统实例列表暴露节点名、角色、版本、CPU/内存/磁盘占用和最近心跳，仍必须保持 Root-only；本轮只叠加 `system_setting.read`，不放宽入口。
2. 系统任务详情可能包含不同业务域的 payload、progress、result 和 error，例如账号池检测、渠道测试、上游模型同步、订阅维护和日志清理摘要。即使是只读，也必须继续 RootAuth 保底。
3. 日志清理任务创建不是同步删除，但会由 runner 异步删除历史日志。本轮把它归入 `system_setting.sensitive_write`，并额外要求 `usage_log.sensitive_write`，避免未来只拥有系统运维敏感写但没有日志删除权限的角色误触发审计证据清理。
4. 由于当前 Root 角色是 superuser，叠加权限表不会改变现有 Root 管理员行为；普通 Admin 和普通用户仍无法访问这些接口。
5. 本轮 MCP 验证只调用 `GET /api/system-info/instances` 和 `GET /api/system-task/list` 等只读接口，不触发 `POST /api/system-task/log-cleanup`。

### 方案评审

采用“RootAuth 保底 + system_setting 权限表 + 日志清理双权限”的方案。系统信息与系统任务属于系统运维域，主权限使用 `system_setting.read/sensitive_write`；日志清理任务同时跨越日志治理域，因此用 `route.after` 额外挂 `usage_log.sensitive_write`。这样既不改变当前 Root-only 生产行为，又为未来 Casbin/user override 提供更准确的资源动作组合。

路由分类：

| 路由 | 权限 | 旧认证边界 | 说明 |
|------|------|------------|------|
| `GET /api/system-info/instances` | `system_setting.read` | Root | 查看多节点实例心跳和资源状态。 |
| `GET /api/system-task/list`、`GET /api/system-task/current`、`GET /api/system-task/:task_id` | `system_setting.read` | Root | 查看系统任务历史、当前活跃任务和单条任务详情。 |
| `POST /api/system-task/log-cleanup` | `system_setting.sensitive_write` + `usage_log.sensitive_write` | Root | 创建异步日志清理任务，最终删除历史日志/审计证据。 |

验收方式：

1. `go test ./service/authz ./middleware ./router ./controller` 覆盖权限中间件、系统信息/系统任务路由结构和 controller 构建。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认 `/system-info` 页面 API 类型和页面编译不受影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/` 与 `/system-info`，确认页面更新生效；直调 `GET /api/system-info/instances`、`GET /api/system-task/list`、`GET /api/system-task/current?type=channel_test` 等只读接口，确认返回 200/业务 success，不触发日志清理任务。

## 本轮实施评审：绘图与异步任务日志 Authz 路由权限表灰度 enforcement

### 需求分析

`new-api-main` 与 NexusTok 都提供 `/api/mj` 和 `/api/task` 两组任务日志接口：`/self` 路径面向当前用户，根路径面向管理员跨用户查询。NexusTok 默认前端已经把它们原生化为 Usage Logs 下的 `drawing` 和 `task` 页面，并在移动端、筛选条、状态映射等方面进一步增强；但后端跨用户查询仍停留在泛化 `AdminAuth`，没有进入上一轮新增的 `usage_log` 权限表。

本轮目标：

1. 将 `GET /api/mj/` 接入 `usage_log.read`，继续保留 `AdminAuth`，用于跨用户查看 Midjourney/MjProxy 绘图任务、渠道、费用、状态、进度和错误。
2. 将 `GET /api/task/` 接入 `usage_log.read`，继续保留 `AdminAuth`，用于跨用户查看视频、音频、图像等通用异步任务、用户、渠道、上游任务 ID、状态和失败原因。
3. 保持 `GET /api/mj/self` 和 `GET /api/task/self` 只走 `UserAuth`，它们只能查询当前用户自己的任务记录，不进入管理员权限表。
4. 不修改任务表结构、查询参数、分页语义、轮询 SystemTask、计费逻辑、前端 Usage Logs 页面和 Relay 任务提交路径。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 任务日志路由 | `router/task-log-router.go`、`router/api-router.go`、`router/task_log_router_test.go` | `/api/mj` 和 `/api/task` 从主路由抽出；管理员根路径叠加 `usage_log.read`，self 路径保持用户自助认证。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收标准，并补充能力落地清单。 |

### 风险评估

1. 跨用户绘图任务和异步任务日志会暴露用户、渠道、上游任务 ID、生成结果 URL、失败原因和成本信息，属于日志审计域，必须进入 `usage_log.read`，但不能降低既有 `AdminAuth` 门槛。
2. self 路径依赖 `c.GetInt("id")` 做用户隔离；如果误挂到管理员权限表，普通用户可能无法查看自己的任务历史，或未来权限 override 时产生管理权限绕过。本轮明确把 self 路径排除在权限表外。
3. 当前 `usage_log.read` 默认授予 Admin；叠加权限表不会改变现有 Admin 可读行为，但会让未来用户级 override 能单独关闭跨用户任务日志视图。
4. 本轮只处理 GET 读接口，不触发任务提交、轮询、重试、取消、日志清理或任何写操作。

### 方案评审

采用“self 路径原认证 + 管理根路径 AdminAuth + usage_log.read”的方案。绘图任务日志和通用异步任务日志虽然使用独立数据表，但在产品语义上已经归入 Usage Logs；复用 `usage_log.read` 可以避免新增过细资源导致前端入口、侧边栏和后端路由权限语义分叉。

路由分类：

| 路由 | 权限 | 旧认证边界 | 说明 |
|------|------|------------|------|
| `GET /api/mj/self` | 不进入管理员权限表 | User | 当前用户查看自己的 Midjourney/MjProxy 绘图任务日志。 |
| `GET /api/mj/` | `usage_log.read` | Admin | 管理员跨用户查看绘图任务日志。 |
| `GET /api/task/self` | 不进入管理员权限表 | User | 当前用户查看自己的通用异步任务日志。 |
| `GET /api/task/` | `usage_log.read` | Admin | 管理员跨用户查看通用异步任务日志。 |

验收方式：

1. `go test ./service/authz ./middleware ./router ./controller` 覆盖权限中间件、任务日志路由结构和 controller 构建。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认 Usage Logs 前端 API 类型和页面编译不受影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/`、`/usage-logs/drawing` 和 `/usage-logs/task`，确认页面更新生效；直调 `GET /api/mj/?p=0&page_size=5`、`GET /api/task/?p=0&page_size=5`、`GET /api/mj/self?p=0&page_size=5`、`GET /api/task/self?p=0&page_size=5`，确认返回 200/业务 success，不触发任务提交或日志清理。

## 本轮实施评审：Authz catalog 路由权限表灰度 enforcement

### 需求分析

`new-api-main` 提供 `/api/authz/catalog` 作为权限资源/动作 schema 和角色基线的只读入口，NexusTok 已经吸收并原生化为后端 `service/authz` catalog、`/api/user/self` 权限矩阵回传和默认前端按钮/路由显隐。但 catalog 路由本身仍只有 `AdminAuth`，没有进入灰度权限表；随着越来越多管理路由接入 `RequirePermission`，权限 catalog 也应归入同一套可审计的只读系统能力。

本轮目标：

1. 将 `GET /api/authz/catalog` 接入 `system_setting.read`，保留 `AdminAuth`，用于查看权限资源、动作定义和 Root/Admin 基线矩阵。
2. 不新增 `authz` 独立资源；当前 catalog 仍只是系统权限配置的只读元数据，不包含用户 override、策略写入、授权变更或 Casbin 存储。
3. 不修改 `service/authz` 的资源定义、角色基线、`/api/user/self` 回传结构、前端用户抽屉和权限矩阵渲染。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz catalog 路由 | `router/authz-router.go`、`router/authz_router_test.go` | `/api/authz/catalog` 保留 `AdminAuth` 并叠加 `system_setting.read`，handler 和响应结构不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收标准，并补充能力落地清单。 |

### 风险评估

1. catalog 会暴露系统当前有哪些管理资源、动作名称和内置角色授权矩阵，属于后台权限配置元数据；继续保留 `AdminAuth`，不会向普通用户开放。
2. 当前 `system_setting.read` 默认授予 Admin；叠加权限表不会改变现有管理员行为，但未来用户级 override 可以单独控制是否可查看权限 schema。
3. 不新增独立 `authz` 资源可以避免把只读 schema 过早拆成新权限域；等出现用户策略写入、角色编辑或 Casbin 管理 UI 时，再评估是否新增 `authz.write/sensitive_write`。
4. 本轮只调用 GET 只读接口，不触发用户权限修改、系统设置写入或任何授权策略持久化。

### 方案评审

采用“AdminAuth 保底 + system_setting.read”的方案。权限 catalog 是系统配置和运维元数据的一部分，现阶段与系统设置读权限绑定最稳妥；它不会改变现有 Root/Admin 基线，也不会影响前端从 `/api/user/self` 读取当前用户能力矩阵。

路由分类：

| 路由 | 权限 | 旧认证边界 | 说明 |
|------|------|------------|------|
| `GET /api/authz/catalog` | `system_setting.read` | Admin | 查看权限资源、动作、文案 key 和内置角色基线授权矩阵。 |

验收方式：

1. `go test ./service/authz ./middleware ./router ./controller` 覆盖权限 catalog、权限中间件、Authz 路由结构和 controller 构建。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认用户管理前端权限 catalog API 类型不受影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/` 与 `/users`，确认页面更新生效；直调 `GET /api/authz/catalog`，确认返回 200/业务 success 且包含 `resources` 与 `roles`，不执行任何写操作。

## 本轮实施评审：模型管理前端按钮级 Authz 原生消费

### 需求分析

NexusTok 已经把 `/api/vendors`、`/api/models`、`/api/deployments`、`/api/group` 和 `/api/prefill_group` 接入 `model.read/operate/write/sensitive_write` 路由权限表，并在 `/api/user/self` 回传权限矩阵。默认前端模型页目前只在路由入口检查 `model.read`，但页面内的新增模型、同步上游、预填组、厂商管理、模型行编辑/启停/删除、部署创建/更新/扩容/删除等按钮仍未完整消费对应动作权限；这会导致未来用户级 override 场景下，前端展示可点击操作，后端再返回 403，体验和权限语义分叉。

本轮目标：

1. 新增模型模块专用 `useModelPermissions`，集中读取 `model.read/operate/write/sensitive_write`。
2. 将模型元数据页主按钮按权限禁用并做点击前保护：新增模型和厂商管理需要 `model.write`；同步上游需要 `model.operate + model.write`；预填组入口至少需要 `model.write` 或 `model.sensitive_write`。
3. 将模型行操作和批量操作按权限禁用：编辑/启停需要 `model.write`，删除需要 `model.sensitive_write`，复制模型名继续可用。
4. 将部署页操作按权限禁用：创建、配置更新、重命名需要 `model.write`；扩容需要 `model.operate`；删除需要 `model.sensitive_write`；查看日志和详情继续只读。
5. 将预填组弹窗内部的创建/编辑/删除和提交确认补充同一权限保护，避免只在入口按钮防护。
6. 不修改后端路由、API payload、查询缓存 key、i18n 文案集合和模型/部署业务逻辑。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 模型权限 hook | `web/default/src/features/models/hooks/use-model-permissions.ts` | 集中封装模型管理动作能力，复用 `admin-permissions` 矩阵。 |
| 模型元数据按钮 | `models-primary-buttons.tsx`、`data-table-row-actions.tsx`、`data-table-bulk-actions.tsx`、`sync-wizard-dialog.tsx` | 主按钮、行操作、批量操作和同步提交按 `model` 动作权限禁用或拦截。 |
| 部署按钮 | `index.tsx`、`deployments-table.tsx`、`deployments-columns.tsx` | 部署创建/配置/重命名/扩容/删除按钮按权限禁用，查看操作保持可用。 |
| 预填组管理 | `prefill-group-management.tsx`、`prefill-group-management-dialog.tsx`、`prefill-group-form-drawer.tsx` | 创建/编辑/删除和提交前检查分别对应 `model.write` 与 `model.sensitive_write`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收标准，并补充能力落地清单。 |

### 风险评估

1. 本轮只改变前端按钮状态和提交前保护，不降低后端 `RequirePermission` enforcement；如果前端状态判断失误，后端仍 fail-closed。
2. 同步上游包含 preview 和写入两个阶段，入口和提交都要求 `model.operate + model.write`，避免只有预览权限的用户触发写入 403。
3. 预填组列表本身是只读，但当前弹窗定位是管理工具；本轮入口需要写或敏感写，弹窗内部再细分创建/编辑与删除，避免读权限用户进入可写管理面。
4. 默认 Admin 基线拥有 `model.write/operate` 但不拥有 `model.sensitive_write`，因此删除类按钮会禁用；Root 用户保持全量可用。该行为与后端权限表一致，是预期的权限收敛。
5. 本轮不新增任何可见文案，统一复用既有 `You don't have necessary permission`，无需更新六语 i18n。

### 方案评审

采用“前端消费现有权限矩阵 + 后端继续 fail-closed”的方案，不引入新的前端权限资源，也不让 UI 自行推断角色。模型模块新增 hook 后，各组件只关心业务动作能力；按钮禁用使用现有 `disabled/title/toast` 模式，延续渠道页的权限消费风格。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 新增/编辑模型、启停模型、创建/编辑厂商、创建部署、更新部署配置、重命名部署、创建/编辑预填组 | `model.write` | `POST/PUT /api/models`、`POST/PUT /api/vendors`、`POST/PUT /api/deployments`、`POST/PUT /api/prefill_group` |
| 同步上游 | `model.operate + model.write` | `GET /api/models/sync_upstream/preview` + `POST /api/models/sync_upstream` |
| 部署扩容、部署连接测试 | `model.operate` | `POST /api/deployments/:id/extend`、`POST /api/deployments/test-connection` |
| 删除模型、删除部署、删除预填组 | `model.sensitive_write` | `DELETE /api/models/:id`、`DELETE /api/deployments/:id`、`DELETE /api/prefill_group/:id` |
| 查看模型、部署详情、部署日志、缺失模型 | `model.read` | 既有页面入口与只读接口 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认模型页权限 hook 与组件类型通过。
2. `go test ./service/authz ./middleware ./router ./controller` 确认后端权限表和路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/`、`/models/metadata` 和 `/models/deployments`，确认模型主按钮、行操作菜单、批量操作和部署操作渲染正常，页面自身请求 200、控制台无错误；只触发只读页面加载，不执行新增、同步、扩容或删除。

## 本轮实施评审：用户管理前端按钮级 Authz 原生消费

### 需求分析

NexusTok 已经把 `/api/user` 管理员子路由接入 `user.read/operate/write/sensitive_write` 权限表，并在 `ManageUser` 复合接口内对升降级、删除和额度调整执行 `user.sensitive_write` 二次校验。默认前端用户页当前只在路由入口校验 `user.read`，页面内新增用户、编辑用户、启停、升降级、重置 Passkey/2FA、解绑登录方式、额度调整和硬删除等按钮仍主要依赖旧 Root/Admin 直觉；当后续用户级 override 收紧某个动作时，前端会展示可点击操作，最终由后端返回 403，体验与权限语义不一致。

本轮目标：

1. 新增用户模块专用 `useUserPermissions`，集中读取 `user.read/operate/write/sensitive_write`；新增订阅模块 `useSubscriptionPermissions`，供用户页跨入口复用 `subscription.read/operate/sensitive_write`。
2. 将用户页主按钮、行操作和确认弹窗按权限禁用并做提交前保护：创建/删除用户和额度调整需要敏感写；编辑用户资料需要普通写；启停、绑定解绑、Passkey/2FA 重置需要运营权限；升降级需要运营权限和敏感写。
3. 将用户编辑抽屉内部的保存与额度调整入口补充同一权限保护，避免只在行操作或主按钮防护。
4. 将账号绑定管理弹窗中的解绑操作补充 `user.operate` 防护，列表读取继续由页面入口和接口自身 `user.read` 控制。
5. 将用户行内“管理订阅”入口按 `subscription.read` 控制，并让该专用用户订阅弹窗内部按 `subscription.operate/sensitive_write` 控制创建、失效和删除订阅；这是跨资源动作，不能错误归入 `user.*`。
6. 不修改后端路由、用户/订阅 API payload、查询缓存和数据库行为；本轮只消费现有权限矩阵并复用既有无权限文案。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 用户与订阅权限 hook | `web/default/src/features/users/hooks/use-user-permissions.ts`、`web/default/src/features/subscriptions/hooks/use-subscription-permissions.ts` | 集中封装用户页动作能力，并为用户页跨入口订阅弹窗复用订阅动作能力。 |
| 用户页按钮与行操作 | `users-primary-buttons.tsx`、`data-table-row-actions.tsx` | 新增、编辑、启停、升降级、绑定管理、订阅管理、重置安全凭据和删除按权限禁用或拦截。 |
| 用户编辑与删除 | `users-mutate-drawer.tsx`、`user-quota-dialog.tsx`、`users-delete-dialog.tsx` | 保存、额度调整和硬删除在提交前做二次权限检查。 |
| 绑定与用户订阅弹窗 | `dialogs/user-binding-dialog.tsx`、`subscriptions/components/dialogs/user-subscriptions-dialog.tsx` | 解绑账号、创建/失效/删除用户订阅消费各自资源的动作权限。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收标准，并补充能力落地清单。 |

### 风险评估

1. 本轮只改变前端按钮状态和提交前保护，不改变后端 `RequirePermission` 或 `ManageUser` 敏感写二次校验；前端误判时后端仍 fail-closed。
2. `ManageUser` 是历史复合接口，启停只需要 `user.operate`，升降级和额度调整需要 `user.operate + user.sensitive_write`；前端必须按动作细分，不能简单把整个菜单归为一个权限。
3. 创建用户走 `POST /api/user/`，后端归为 `user.sensitive_write`，因此默认 Admin 不再看到可用的创建按钮；这与后端权限表收敛一致。
4. 用户订阅管理虽然从用户行进入，但后端在 `/api/subscription/admin` 下按 `subscription` 资源授权；本轮按跨资源权限处理，避免给拥有 `user.read` 但没有订阅权限的管理员暴露订阅操作面。
5. 当前 Root 用户拥有全量权限，MCP 环境下按钮多数保持可用；验收重点是渲染稳定、请求正常和无控制台错误，不执行高风险写操作。

### 方案评审

采用“用户页本地 hook + 后端 fail-closed + 跨资源动作按归属资源授权”的方案。用户模块 hook 只封装权限读取，不引入角色推断和本地策略存储；各组件在按钮 disabled、点击入口和提交 handler 三层做轻量保护，延续模型/渠道页已经形成的前端权限消费模式。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 查看用户列表、搜索、用户详情、安全统计、绑定列表 | `user.read` | `GET /api/user/*` 管理员只读路由 |
| 启用/禁用用户、重置 Passkey、重置 2FA、解绑内置或自定义 OAuth 绑定 | `user.operate` | `POST /api/user/manage`、`DELETE /api/user/:id/reset_passkey`、`DELETE /api/user/:id/2fa`、`DELETE /api/user/:id/bindings/*` |
| 编辑用户普通资料、分组、备注 | `user.write` | `PUT /api/user/` |
| 创建用户、硬删除用户、升降级、额度调整 | `user.sensitive_write`，其中升降级/额度调整还需 `user.operate` | `POST /api/user/`、`DELETE /api/user/:id`、`POST /api/user/manage` |
| 打开用户订阅管理 | `subscription.read` | `GET /api/subscription/admin/plans`、`GET /api/subscription/admin/users/:id/subscriptions` |
| 创建/失效用户订阅 | `subscription.operate` | `POST /api/subscription/admin/users/:id/subscriptions`、`POST /api/subscription/admin/user_subscriptions/:id/invalidate` |
| 删除用户订阅记录 | `subscription.sensitive_write` | `DELETE /api/subscription/admin/user_subscriptions/:id` |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认用户权限 hook 和跨资源弹窗类型通过。
2. `go test ./service/authz ./middleware ./router ./controller` 确认后端权限表和用户/订阅路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/` 与 `/users`，确认用户主按钮、行操作菜单、绑定管理/用户订阅入口渲染正常，页面自身请求 200、控制台无错误；只触发页面加载和只读弹窗，不执行创建、升降级、解绑、额度调整、删除或订阅变更。

## 本轮实施评审：订阅管理前端按钮级 Authz 原生消费

### 需求分析

订阅管理后端 `/api/subscription/admin` 已按 `subscription.read/write/operate/sensitive_write` 接入权限表：套餐列表走 read，套餐创建、编辑和启停走 write，用户订阅创建/失效走 operate，用户订阅删除走 sensitive_write。上一轮已经为用户行内订阅弹窗补齐跨资源 `subscription.*` 消费；订阅管理页面本身仍只在路由入口校验 `subscription.read`，`Create Plan`、套餐行 `Edit`、套餐 `Enable/Disable`、套餐抽屉保存和启停确认没有按 `subscription.write` 做按钮禁用与提交前保护。

本轮目标：

1. 复用 `useSubscriptionPermissions`，让订阅套餐页主按钮、行菜单、创建/编辑抽屉和启停确认弹窗消费 `subscription.write`。
2. 保留现有支付合规确认逻辑，不把 `subscription.write` 与合规状态混为一个条件；按钮禁用原因仍优先体现权限不足。
3. 不修改订阅后端路由、套餐 payload、支付合规系统设置接口、用户购买流程和用户订阅弹窗逻辑。
4. 不新增可见文案，继续复用既有 `You don't have necessary permission`。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 订阅主按钮 | `web/default/src/features/subscriptions/components/subscriptions-primary-buttons.tsx` | `Create Plan` 需要 `subscription.write`，并保留支付合规确认约束。 |
| 订阅行操作 | `data-table-row-actions.tsx` | `Edit` 与 `Enable/Disable` 需要 `subscription.write`，查看或其他只读行为不变。 |
| 套餐抽屉 | `subscriptions-mutate-drawer.tsx` | 创建/编辑提交前检查 `subscription.write`，保存按钮按权限禁用。 |
| 启停确认弹窗 | `dialogs/toggle-status-dialog.tsx` | 确认启停前检查 `subscription.write`，确认按钮按权限禁用。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收标准，并补充能力落地清单。 |

### 风险评估

1. 本轮只改变前端按钮状态和提交前保护，不降低后端 `RequirePermission` enforcement；后端仍是最终授权边界。
2. 订阅套餐创建/编辑同时受支付合规确认约束和 `subscription.write` 约束；本轮仅增加权限约束，不改变合规确认流程。
3. 默认 Admin 拥有 `subscription.write`，因此现有管理员工作流保持可用；未来用户级 override 可以单独关闭套餐写操作。
4. 用户订阅创建/失效/删除已在上一轮用户切片接入 `subscription.operate/sensitive_write`，本轮避免重复修改以降低冲突面。

### 方案评审

采用“复用订阅权限 hook + 按后端套餐路由动作映射”的方案。订阅套餐创建、编辑和启停后端都归入 `subscription.write`，前端也保持同一映射；如果未来要把启停拆成 `operate`，应先调整后端路由权限表，再改前端按钮映射，避免前端放行而服务端 403。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 查看订阅套餐列表 | `subscription.read` | `GET /api/subscription/admin/plans` |
| 创建套餐 | `subscription.write` | `POST /api/subscription/admin/plans` |
| 编辑套餐 | `subscription.write` | `PUT /api/subscription/admin/plans/:id` |
| 启用/禁用套餐 | `subscription.write` | `PATCH /api/subscription/admin/plans/:id` |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认订阅权限 hook 与组件类型通过。
2. `go test ./service/authz ./middleware ./router ./controller` 确认后端权限表和订阅路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/` 与 `/subscriptions`，确认订阅套餐页加载、主按钮和行操作菜单渲染正常，页面自身请求 200、控制台无错误；只触发页面加载和菜单查看，不执行创建、编辑、启停或删除。

## 本轮实施评审：兑换码前端按钮级 Authz 原生消费

### 需求分析

兑换码后端 `/api/redemption` 已作为独立资源接入 `redemption.read/write/sensitive_write` 权限表：列表、搜索和详情走 read，创建、编辑和状态启停走 write，单个删除和批量清理无效兑换码走 sensitive_write。默认前端兑换码页已经在侧边栏和路由入口消费 `redemption.read`，但页面内 `Create Code`、行菜单 `Edit/Enable/Disable/Delete`、创建/编辑抽屉提交、删除确认和批量 `Delete invalid codes` 仍没有消费动作级权限。

本轮目标：

1. 新增兑换码模块 `useRedemptionPermissions`，集中读取 `redemption.read/write/sensitive_write`。
2. 将创建、编辑、启停和抽屉保存映射到 `redemption.write`；当前后端启停仍走 `PUT /api/redemption/?status_only=true`，不擅自改成 `redemption.operate`。
3. 将单个删除和批量清理无效兑换码映射到 `redemption.sensitive_write`。
4. 保留兑换码复制、查看和列表读取的现状；本轮不引入 `redemption.secret_view` 或 reveal 接口，避免扩大后端契约。
5. 不修改兑换码后端路由、payload、列表查询、批量清理语义和现有 i18n 文案集合。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 兑换码权限 hook | `web/default/src/features/redemption-codes/hooks/use-redemption-permissions.ts` | 集中封装兑换码动作权限。 |
| 主按钮与批量操作 | `redemptions-primary-buttons.tsx`、`data-table-bulk-actions.tsx` | 创建兑换码走 `redemption.write`；批量清理无效码走 `redemption.sensitive_write`；复制选中兑换码保持只读行为。 |
| 行操作与确认弹窗 | `data-table-row-actions.tsx`、`redemptions-delete-dialog.tsx` | 编辑/启停走 write；删除走 sensitive_write，并在确认前二次检查。 |
| 创建/编辑抽屉 | `redemptions-mutate-drawer.tsx` | 保存提交前检查 `redemption.write`，保存按钮按权限禁用。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收标准，并补充能力落地清单。 |

### 风险评估

1. 本轮只改变前端按钮状态和提交前保护，后端 `RequirePermission` 仍是最终授权边界。
2. `service/authz` 中 `redemption.operate` 的描述包含启停/清理，但当前 router 未使用该动作；前端必须对齐实际后端路由，启停按 write、清理按 sensitive_write，否则会出现前端放行而后端 403。
3. 完整兑换码仍随列表数据返回，复制/查看能力无法仅靠前端按钮彻底隔离；真正区分“读元数据”和“查看密钥”需要后续新增 `redemption.secret_view` 或后端 reveal 接口。
4. 默认 Admin 拥有 `redemption.write` 但不拥有 `redemption.sensitive_write`，因此删除/清理类按钮会在受限管理员场景下禁用；Root 仍保持全量可用。

### 方案评审

采用“前端消费现有 `redemption` 权限矩阵 + 后端语义不变”的方案。新增 hook 只读取已有矩阵，不引入新资源或角色推断；所有写入类按钮在 disabled、点击入口和提交/确认 handler 三层做保护。密钥可见性问题作为后续独立后端契约改造项记录，不混入本轮按钮级权限消费。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 查看兑换码列表、搜索、详情 | `redemption.read` | `GET /api/redemption/*` |
| 创建兑换码 | `redemption.write` | `POST /api/redemption/` |
| 编辑兑换码 | `redemption.write` | `PUT /api/redemption/` |
| 启用/禁用兑换码 | `redemption.write` | `PUT /api/redemption/?status_only=true` |
| 删除单个兑换码 | `redemption.sensitive_write` | `DELETE /api/redemption/:id` |
| 批量清理无效兑换码 | `redemption.sensitive_write` | `DELETE /api/redemption/invalid` |
| 复制兑换码 | `redemption.read`（现状） | 依赖列表数据；后续可拆 secret_view |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认兑换码权限 hook 与组件类型通过。
2. `go test ./service/authz ./middleware ./router ./controller` 确认后端权限表和兑换码路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/` 与 `/redemption-codes`，确认兑换码页加载、主按钮、批量工具和行操作菜单渲染正常，页面自身请求 200、控制台无错误；只触发页面加载和菜单查看，不执行创建、编辑、启停、删除或批量清理。

## 本轮实施评审：系统设置前端运行维护 Authz 原生消费

### 需求分析

系统设置后端已经把 `/api/option`、`/api/performance`、`/api/ratio_sync` 和 `/api/custom-oauth-provider` 纳入 `system_setting.read/operate/sensitive_write` 权限表：只读统计走 read，缓存清理、GC、上游倍率拉取和 OIDC discovery 走 operate，通用设置保存、日志文件删除、模型倍率重置、自定义 OAuth provider 增删改走 sensitive_write。默认前端系统设置页当前仍主要依赖 Root 页面边界，页面内运行维护按钮、危险写入按钮和自定义 OAuth 操作没有消费动作级权限；当后续用户级 override 收紧某一类系统设置能力时，前端会展示可点击操作，最终由后端返回 403，体验与服务端权限语义不一致。

本轮目标：

1. 新增系统设置模块 `useSystemSettingPermissions`，集中读取 `system_setting.read/operate/write/sensitive_write/secret_view`；其中 `write/secret_view` 先保留为矩阵完整性字段，当前后端系统设置路由尚未使用普通写和密钥查看动作。
2. 将性能页的运行维护按钮映射到现有后端权限：清理非活跃磁盘缓存、重置统计、强制 GC 需要 `system_setting.operate`；删除服务器日志文件需要 `system_setting.sensitive_write`；性能设置保存继续对齐通用 `PUT /api/option/` 的 sensitive_write。
3. 将模型倍率页的 `Reset prices` 映射到 `system_setting.sensitive_write`，上游倍率拉取映射到 `system_setting.operate`，上游倍率应用映射到 `system_setting.sensitive_write`，避免“可拉取差异但无权写入系统倍率”时误展示可提交动作。
4. 将渠道亲和页的缓存清理映射到 `system_setting.operate`，保存亲和配置映射到 `system_setting.sensitive_write`；规则编辑仍是本地草稿行为，最终保存前再执行敏感写权限保护。
5. 将自定义 OAuth provider 的新增、编辑、删除和弹窗提交映射到 `system_setting.sensitive_write`，OIDC discovery 映射到 `system_setting.operate`。
6. 本轮不全局改造所有 `useUpdateOption()` 表单保存入口，避免一次性收紧站点、认证、支付、内容、请求限制等全部设置页面；这些通用保存入口后续需要单独切片、逐页验证和更完整的管理员兼容评审。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 系统设置权限 hook | `web/default/src/features/system-settings/hooks/use-system-setting-permissions.ts` | 集中封装系统设置动作权限，供运行维护、倍率同步、渠道亲和和自定义 OAuth 复用。 |
| 性能维护页 | `maintenance/performance-section.tsx` | 性能设置保存、缓存清理、统计重置、强制 GC、服务器日志文件清理按后端 system_setting 权限禁用并提交前保护。 |
| 渠道亲和页 | `general/channel-affinity/index.tsx` | 亲和配置保存按敏感写控制，清理全部/单规则缓存按 operate 控制。 |
| 模型/分组倍率与上游同步 | `models/ratio-settings-card.tsx`、`models/model-ratio-form.tsx`、`models/group-ratio-form.tsx`、`models/upstream-ratio-sync.tsx` | 分组倍率保存、模型倍率重置、上游拉取和应用同步分别消费 sensitive_write、sensitive_write、operate、sensitive_write；当前生产路由主要通过 `/pricing-settings` 暴露分组与工具价格页。 |
| 自定义 OAuth | `auth/custom-oauth/*` | provider 新增、编辑、删除、保存和 OIDC discovery 按 sensitive_write/operate 控制。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收方式，并补充能力落地清单。 |

### 风险评估

1. 本轮只改变前端按钮状态和提交前保护，不降低后端 `RequirePermission` enforcement；即使前端判断失误，后端仍 fail-closed。
2. 默认 Admin 仅有 `system_setting.read` 基线，Root/Super Admin 保持全量能力；因此部分系统设置写入按钮会在受限管理员场景下禁用，这是与后端 RootAuth + 权限表二次校验一致的行为。
3. `PUT /api/option/` 当前整体归为 `system_setting.sensitive_write`，但系统设置页面覆盖面极大；本轮只处理与运行维护工具同屏且边界明确的保存入口，避免一次性影响支付、认证、站点内容等关键配置工作流。
4. 上游倍率同步包含两个不同阶段：拉取差异是 operate，应用结果会写入系统 option，必须归为 sensitive_write；前端按钮需要拆分控制，不能只按一个权限处理。
5. 自定义 OAuth 的 client secret、登录入口和访问准入策略都属于高风险配置，新增/编辑/删除统一按 sensitive_write 控制；OIDC discovery 只读取第三方 well-known 并回填表单，按 operate 控制。

### 方案评审

采用“系统设置专用 hook + 明确接口动作映射 + 后端 fail-closed”的方案。前端不推断角色、不缓存策略、不改变 API payload；只在按钮 disabled、点击入口和提交/确认 handler 三层按后端路由权限表做保护。通用系统 option 表单保存后续单独评审和分批接入，避免一次改动跨越太多业务域。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 读取系统 option、性能统计、日志文件信息、自定义 OAuth 列表、上游渠道列表、渠道亲和缓存统计 | `system_setting.read` | `GET /api/option/`、`GET /api/performance/*`、`GET /api/custom-oauth-provider/*`、`GET /api/ratio_sync/channels` |
| 清理非活跃磁盘缓存、重置性能统计、强制 GC、清理渠道亲和缓存、OIDC discovery、拉取上游倍率差异 | `system_setting.operate` | `DELETE /api/performance/disk_cache`、`POST /api/performance/reset_stats`、`POST /api/performance/gc`、`DELETE /api/option/channel_affinity_cache`、`POST /api/custom-oauth-provider/discovery`、`POST /api/ratio_sync/fetch` |
| 保存性能/渠道亲和/模型倍率 option、应用上游倍率同步、重置模型倍率、自定义 OAuth provider 增删改、删除服务器日志文件 | `system_setting.sensitive_write` | `PUT /api/option/`、`POST /api/option/rest_model_ratio`、`POST/PUT/DELETE /api/custom-oauth-provider/*`、`DELETE /api/performance/logs` |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认系统设置权限 hook 和相关组件类型通过。
2. `go test ./service/authz ./middleware ./router ./controller` 确认后端权限表和路由层仍可构建。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/`，再打开 `/system-settings/operations/performance`、`/pricing-settings`、`/system-settings/models/channel-affinity`、`/system-settings/auth/custom-oauth`，确认页面加载、按钮渲染和只读数据请求正常，控制台无错误；只做页面加载和打开弹窗，不触发 GC、日志删除、缓存清理、倍率应用、自定义 OAuth 保存或删除。

## 本轮实施评审：系统设置通用 option 保存前置 Authz 保护

### 需求分析

上一轮已经把性能维护、渠道亲和、上游倍率同步和自定义 OAuth 等边界清晰的系统设置动作接入 `system_setting.operate/sensitive_write`。但默认前端仍有大量站点、认证、支付、内容、安全限制、模型配置和运维表单通过 `useUpdateOption()` 调用 `PUT /api/option/`，该后端路由已经统一归入 `system_setting.sensitive_write`。如果只逐页禁用按钮，很容易遗漏某个表单或某个批量保存路径；更稳妥的原生能力应当先在通用 hook 内建立前置权限保护，让所有系统 option 保存请求在发出前都按同一权限矩阵 fail-closed。

本轮目标：

1. 在 `useUpdateOption()` 内消费 `useSystemSettingPermissions()`，对所有 `updateSystemOption()` 调用做统一 `system_setting.sensitive_write` 前置检查。
2. 缺少权限时不发送 `PUT /api/option/` 请求，直接复用既有 `You don't have necessary permission` 文案提示，避免前端展示可点击后再由后端返回 403。
3. 扩展 `useUpdateOption()` 返回值，向调用方暴露 `canUpdate` 与 `disabledReason`，为后续逐页禁用保存按钮提供兼容的集中字段；本轮不强制一次性修改所有表单按钮，避免跨几十个设置页面引入视觉和交互回归。
4. 保持后端权限表、API payload、查询缓存刷新、状态刷新和 i18n 文件不变。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 通用保存 hook | `web/default/src/features/system-settings/hooks/use-update-option.ts` | 所有 `PUT /api/option/` 前端调用先检查 `system_setting.sensitive_write`，并向调用方暴露 `canUpdate/disabledReason`。 |
| 系统设置权限 hook | `web/default/src/features/system-settings/hooks/use-system-setting-permissions.ts` | 复用上一轮权限矩阵，保持单一授权来源。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮中心化保护的需求、风险、方案和验收方式。 |

### 风险评估

1. 本轮不会放宽任何后端权限；后端 `RequirePermission(system_setting.sensitive_write)` 仍是最终边界。
2. 默认 Root/Super Admin 拥有全量权限，现有 Root 管理工作流保持可用；缺少敏感写的受限管理员会在前端保存前被拦截，行为与后端语义一致。
3. 中心化保护会覆盖所有 `useUpdateOption()` 消费者，包括站点品牌、认证、支付、内容、安全限制和模型配置等页面；这是符合后端 `PUT /api/option/` 当前统一敏感写分类的，但按钮禁用仍需后续分批做更完整的交互 polish。
4. `useUpdateOption()` 当前负责成功后刷新 `system-options` 和部分 `status` 查询；本轮只包裹 mutationFn，不改变成功/失败缓存策略，避免影响页面数据同步。
5. 若未来后端按 option key 细分普通写与敏感写，本轮的中心化 `sensitive_write` 检查会偏保守；届时应先调整后端路由/controller 语义，再把 hook 改成 key-aware 权限判断。

### 方案评审

采用“中心化前置检查 + 兼容返回字段”的方案。`useUpdateOption()` 继续返回 TanStack Query mutation 对象，现有调用方的 `mutateAsync`、`isPending`、`onSuccess` 行为不变；额外通过 `Object.assign` 暴露 `canUpdate` 和 `disabledReason`，让后续页面可以逐步把保存按钮的 disabled/title 从本地判断切到统一字段。缺少权限时 hook 抛出本地错误并由现有 `onError` toast 处理，不发起 API 请求。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 所有通过 `useUpdateOption()` 保存的系统 option | `system_setting.sensitive_write` | `PUT /api/option/` |
| 保存后刷新系统 option/status 缓存 | 成功保存后执行 | 前端 `queryClient.invalidateQueries`，不改变后端路由 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认扩展 mutation 返回值不破坏既有调用方类型。
2. `go test ./service/authz ./middleware ./router ./controller` 确认后端权限表和系统设置路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/`，再打开至少两个典型 `useUpdateOption()` 页面（如 `/system-settings/site/general` 或其实际默认路径、`/system-settings/auth/basic-auth`、`/system-settings/security/rate-limit`），确认页面加载和只读 `/api/option/` 请求正常，控制台无错误；不触发真实保存。

## 本轮实施评审：系统设置高频表单保存按钮 Authz 消费

### 需求分析

上一轮已经在 `useUpdateOption()` 内为所有 `PUT /api/option/` 调用建立 `system_setting.sensitive_write` 前置保护，并向调用方暴露 `canUpdate/disabledReason`。站点信息、基础认证和请求限流是系统设置中访问频率较高、配置影响面明确的三类表单：它们都通过 `useUpdateOption()` 保存 option，但按钮仍只根据 pending/submitting 状态禁用。这样在受限管理员场景下，用户仍会看到可点击保存按钮，点击后才被通用 hook 拦截，交互反馈晚于权限语义。

本轮目标：

1. 将站点信息、基础认证和请求限流三处保存按钮直接消费 `updateOption.canUpdate` 与 `updateOption.disabledReason`。
2. 保留 `useUpdateOption()` 的统一前置拦截作为最终前端保护，按钮禁用只做更早、更清晰的交互表达。
3. 不修改 API payload、表单 schema、默认值归一化、请求限流 JSON 校验、重置按钮和后端权限表。
4. 不新增可见文案，继续复用既有 `You don't have necessary permission`。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 站点信息表单 | `web/default/src/features/system-settings/general/system-info-section.tsx` | 保存按钮在缺少 `system_setting.sensitive_write` 时禁用并展示权限原因；重置按钮保持只受脏状态和提交状态控制。 |
| 基础认证表单 | `web/default/src/features/system-settings/auth/basic-auth-section.tsx` | 保存按钮消费统一 option 保存权限；开关编辑仍允许本地修改草稿，提交入口按权限关闭。 |
| 请求限流表单 | `web/default/src/features/system-settings/request-limits/rate-limit-section.tsx` | 保存限流配置按钮消费统一 option 保存权限；视觉/JSON 模式切换和本地校验不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收方式，并补充能力落地清单。 |

### 风险评估

1. 本轮只增加按钮级 disabled/title，不改变 `useUpdateOption()` 的提交前 fail-closed 保护，也不降低后端 `RequirePermission(system_setting.sensitive_write)` enforcement。
2. Root/Super Admin 拥有敏感写能力，现有系统设置保存工作流保持可用；受限管理员会在按钮层直接看到不可保存状态，体验与后端权限一致。
3. 允许用户在无保存权限时编辑本地表单草稿，不会发出写请求；这避免把权限控制和表单字段可读性混在一起，也便于管理员查看配置。
4. 三个页面覆盖站点品牌、认证开关和限流规则，均属于高影响配置；本轮不触碰支付、OAuth、模型倍率等其他页面，避免一次性扩大视觉回归面。

### 方案评审

采用“通用 hook 兜底 + 高频页面按钮原生消费”的方案。`useUpdateOption()` 继续作为所有 option 保存的统一授权入口，页面按钮只读取其暴露的 `canUpdate/disabledReason`，这样权限判断来源保持单一。如果未来后端按 option key 细分普通写和敏感写，只需要先调整 hook 的 key-aware 判断，再让这些按钮自然继承新的保存能力。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 保存站点信息、基础认证、请求限流 option | `system_setting.sensitive_write` | `PUT /api/option/` |
| 重置本地草稿、切换请求限流编辑模式 | 本地交互，无写权限要求 | 不调用后端写接口 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认三处页面能识别 `useUpdateOption()` 暴露的权限字段。
2. `go test ./service/authz ./middleware ./router ./controller` 确认后端权限表和路由层仍可构建。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/`，再打开 `/system-settings/site/system-info`、`/system-settings/auth/basic-auth`、`/system-settings/security/rate-limit`，确认页面加载、保存按钮渲染和 `/api/option/` 只读请求正常，控制台无错误；不触发真实保存。

## 本轮实施评审：系统设置认证与安全表单保存按钮 Authz 消费

### 需求分析

站点信息、基础认证和请求限流已经开始在按钮层消费 `useUpdateOption()` 暴露的 `canUpdate/disabledReason`。认证与安全分组中仍有 OAuth 集成、Passkey、Bot Protection、敏感词和 SSRF 保护五个表单通过同一 `PUT /api/option/` 保存，但按钮未显式消费 `system_setting.sensitive_write`。这些页面都属于登录入口、请求防护或出站请求安全配置；如果受限管理员只具备读取权限，页面应保持可查看和本地编辑草稿，但保存入口必须直接反映“无权写入系统 option”的状态。

本轮目标：

1. 将 OAuth 集成、Passkey、Bot Protection、敏感词和 SSRF 保护五处保存按钮接入 `updateOption.canUpdate` 与 `updateOption.disabledReason`。
2. 继续保留 `useUpdateOption()` 的统一提交前保护，防止遗漏页面或绕过按钮状态后发出 `PUT /api/option/`。
3. 不改变 OAuth well-known 拉取、Passkey origins 归一化、敏感词文本处理、SSRF 列表/端口归一化和任何 API payload。
4. 不新增可见文案，继续复用既有 `You don't have necessary permission`。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| OAuth 集成 | `web/default/src/features/system-settings/auth/oauth-section.tsx` | 保存按钮缺少敏感写权限时禁用并展示权限原因；本地重置按钮保持原有脏状态逻辑。 |
| Passkey | `web/default/src/features/system-settings/auth/passkey-section.tsx` | 保存按钮新增 pending/权限禁用状态，避免此前提交中没有 loading/权限反馈。 |
| Bot Protection | `web/default/src/features/system-settings/auth/bot-protection-section.tsx` | Turnstile 配置保存按钮消费统一 option 保存权限。 |
| 敏感词 | `web/default/src/features/system-settings/request-limits/sensitive-words-section.tsx` | 敏感词保存按钮消费统一 option 保存权限。 |
| SSRF 保护 | `web/default/src/features/system-settings/request-limits/ssrf-section.tsx` | SSRF 出站请求安全配置保存按钮消费统一 option 保存权限。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收方式，并补充能力落地清单。 |

### 风险评估

1. 本轮仍只改变前端按钮 disabled/title 与 Passkey 按钮的 pending 文案，不改变后端权限表、表单字段、校验和提交 payload。
2. OAuth 集成包含 client secret、Bot Protection 包含 Turnstile secret、SSRF 影响服务端出站请求边界，因此统一要求 `system_setting.sensitive_write` 与当前后端 `PUT /api/option/` 分类一致。
3. OAuth 的 well-known 拉取发生在保存流程内部，当前仍随保存按钮一起受敏感写限制；后续如要把 discovery 单独视为 operate，应先拆出独立接口或前端按钮，再调整权限映射。
4. Root/Super Admin 体验保持不变；受限管理员仍可查看配置和编辑本地草稿，但无法提交系统 option 写入。

### 方案评审

采用“同类系统 option 表单统一按钮语义”的方案。五个页面都继续复用 `useUpdateOption()`，按钮层只读取 hook 暴露的 `canUpdate/disabledReason`，避免每页重新解释角色或权限矩阵。Passkey 原按钮缺少 `isPending` 禁用和 `Saving...` 反馈，本轮顺手对齐同类表单，降低重复点击风险。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 保存 OAuth、Passkey、Bot Protection、敏感词、SSRF option | `system_setting.sensitive_write` | `PUT /api/option/` |
| OAuth 表单重置、Tab 切换、SSRF/敏感词本地编辑 | 本地交互，无写权限要求 | 不调用后端写接口 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认五个页面能识别统一权限字段。
2. `go test ./service/authz ./middleware ./router ./controller` 确认系统设置权限表和路由层仍可构建。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/`，再打开 `/system-settings/auth/oauth`、`/system-settings/auth/passkey`、`/system-settings/auth/bot-protection`、`/system-settings/security/sensitive-words`、`/system-settings/security/ssrf`，确认页面加载、保存按钮渲染和 `/api/option/` 只读请求正常，控制台无错误；不触发真实保存。

## 本轮实施评审：系统设置站点、计费与运维普通表单保存按钮 Authz 消费

### 需求分析

系统设置通用保存 hook 已经统一拦截缺少 `system_setting.sensitive_write` 的 `PUT /api/option/`，并且站点信息、认证、安全表单已分批把保存按钮接入 `canUpdate/disabledReason`。剩余站点、计费和运维分组中仍有一批普通 option 表单只按 pending/submitting 禁用：系统行为、配额、货币显示、签到、系统公告、顶部导航、侧边栏模块和日志记录设置。它们虽然业务域不同，但保存路径一致，均应在按钮层消费同一个敏感写权限，避免受限管理员看到可点击保存入口后才被 hook 拦截。

本轮目标：

1. 将系统行为、配额、货币显示、签到、系统公告、顶部导航、侧边栏模块和日志记录设置的保存按钮接入 `updateOption.canUpdate` 与 `updateOption.disabledReason`。
2. 保持本地草稿编辑、重置到默认、表单重置和导航模块开关编辑能力不变；只有实际保存入口需要系统 option 敏感写权限。
3. 不修改配额合规提示、货币显示序列化、签到额度序列化、导航模块 JSON 序列化、日志记录设置和任何后端接口。
4. 日志维护页的历史日志清理按钮不是 `PUT /api/option/`，后端实际要求系统设置敏感写并额外要求日志敏感写；本轮不混入该危险操作，后续单独评审和实现。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 系统行为 | `web/default/src/features/system-settings/general/system-behavior-section.tsx` | 保存系统默认行为时消费统一 option 保存权限。 |
| 配额设置 | `general/quota-settings-section.tsx` | 保存新用户额度、预消费、邀请奖励、充值/文档链接和免费模型预消费开关时消费统一权限。 |
| 货币显示 | `general/pricing-section.tsx` | 保存货币展示、汇率和 token 统计展示时消费统一权限；本地重置按钮不变。 |
| 签到奖励 | `general/checkin-settings-section.tsx` | 保存签到开关和额度范围时消费统一权限；无脏数据时仍保持禁用。 |
| 系统公告 | `maintenance/notice-section.tsx` | 保存全局公告 option 时消费统一权限。 |
| 顶部导航 | `maintenance/header-navigation-section.tsx` | 保存 HeaderNavModules 时消费统一权限；重置到默认仅改本地草稿。 |
| 侧边栏模块 | `maintenance/sidebar-modules-section.tsx` | 保存 SidebarModulesAdmin 时消费统一权限；重置到默认仅改本地草稿。 |
| 日志记录设置 | `maintenance/log-settings-section.tsx` | 保存 LogConsumeEnabled 时消费统一权限；历史日志清理按钮保持现状，留待独立危险操作权限切片。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收方式，并补充能力落地清单。 |

### 风险评估

1. 本轮只改变保存按钮 disabled/title，不改变表单字段、序列化、合规提示、缓存刷新和后端权限 enforcement。
2. `HeaderNavModules`、`SidebarModulesAdmin`、`Notice`、`LogConsumeEnabled` 等 key 会影响公共页面和全局状态，继续由 `useUpdateOption()` 成功后刷新 status，按钮接线不会改变刷新语义。
3. 签到和配额属于用户激励与余额相关配置，本轮只提前禁用保存入口，不改变额度计算和合规确认。
4. 日志清理会删除审计证据，权限语义比普通保存更严格；本轮明确不修改该按钮，避免只加 `system_setting.sensitive_write` 而漏掉 `usage_log.sensitive_write`。

### 方案评审

采用“同一 `PUT /api/option/` 保存入口，同一按钮权限字段”的方案。页面仍允许查看和编辑本地草稿，保存按钮在缺少敏感写时禁用并展示统一无权限原因；hook 层保留提交前保护，防止快捷键、脚本或遗漏按钮路径绕过前端。危险日志清理另开切片，以同时消费系统设置和日志资源权限。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 保存系统行为、配额、货币显示、签到、公告、顶部导航、侧边栏模块、日志记录 option | `system_setting.sensitive_write` | `PUT /api/option/` |
| 重置表单、重置导航/侧边栏默认值、编辑本地开关 | 本地交互，无写权限要求 | 不调用后端写接口 |
| 清理历史日志 | 后续切片处理 | `POST /api/system-task/log-cleanup`，需要系统设置敏感写和日志敏感写 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认八个页面能识别统一权限字段。
2. `go test ./service/authz ./middleware ./router ./controller` 确认系统设置、系统任务和权限路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/`，再打开 `/system-settings/operations/behavior`、`/system-settings/billing/quota`、`/system-settings/billing/currency`、`/system-settings/billing/checkin`、`/system-settings/site/notice`、`/system-settings/site/header-navigation`、`/system-settings/site/sidebar-modules`、`/system-settings/operations/logs`，确认页面加载、保存按钮渲染和 `/api/option/` 只读请求正常，控制台无错误；不触发真实保存和日志清理。

## 本轮实施评审：日志清理系统任务入口与双资源 Authz 消费

### 需求分析

NexusTok 后端已经把历史日志清理从同步 `DELETE /api/log/` 迁入原生 SystemTask：`POST /api/system-task/log-cleanup` 只创建后台任务，runner 负责分批删除、进度记录和任务历史展示。该入口继续保留 RootAuth，并先要求 `system_setting.sensitive_write`，再额外要求 `usage_log.sensitive_write`，因为日志清理会删除请求记录和管理审计证据。默认前端日志维护页仍调用旧的 `DELETE /api/log/`，并且“清理日志”按钮只按本地 cleaning 状态禁用，没有消费双资源权限；这会绕开 SystemTask 的可观测能力，也会让前端权限语义低于后端路由表。

本轮目标：

1. 将日志维护页历史日志清理 API 从 `DELETE /api/log/` 切到 `POST /api/system-task/log-cleanup?target_timestamp=...`。
2. 清理按钮、确认弹窗和确认动作同时消费 `system_setting.sensitive_write` 与 `usage_log.sensitive_write`；任一权限缺失时不打开确认或不发请求。
3. 成功创建任务后用已有 `Log cleanup` 与 `Task ID:` 文案提示任务 ID，并刷新 `system-info/system-tasks` 查询缓存，方便系统信息页观测进度。
4. 保持日志记录设置保存按钮、日期选择、快速时间按钮、确认弹窗内容和后端路由不变；不触发同步删除。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 日志清理 API | `web/default/src/features/system-settings/api.ts`、`types.ts` | 新增/改用 SystemTask 创建响应，调用 `POST /api/system-task/log-cleanup`。 |
| 日志维护页 | `maintenance/log-settings-section.tsx` | 清理按钮和确认动作消费系统设置敏感写与日志敏感写双权限；成功后提示任务 ID 并刷新系统任务缓存。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录旧同步接口迁移、双权限映射、风险和验收方式。 |

### 风险评估

1. 本轮会改变前端发起日志清理的后端路径，但目标是对齐已经存在的原生 SystemTask API；旧 `DELETE /api/log/` 管理员接口不删除，避免破坏其他潜在调用方。
2. SystemTask 清理是异步执行，用户不再立即看到删除数量；前端改为提示任务 ID，删除数量可在 `/system-info` 的 System Tasks 面板查看。
3. 双权限任一缺失都会禁用清理入口并在点击时提示无权限，避免只满足系统设置权限却缺少日志删除权限的管理员触发危险操作。
4. 不改日志清理 runner、批量大小、幂等防重和后端权限表；并发点击仍由后端 active task 去重。

### 方案评审

采用“前端切换到 SystemTask 创建 API + 双权限前置保护”的方案。日志清理从同步删除变为可观测后台任务，符合 NexusTok 已落地的 SystemTask 原生能力；按钮层和确认 handler 只做前置 UX 保护，后端 `RequirePermission` 仍是最终边界。成功后刷新系统任务列表缓存，不在日志维护页新增任务表，避免把系统信息页已有观测面重复搬进设置页。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 打开日志清理确认弹窗 | `system_setting.sensitive_write` + `usage_log.sensitive_write` | 前端本地检查 |
| 创建日志清理 SystemTask | `system_setting.sensitive_write` + `usage_log.sensitive_write` | `POST /api/system-task/log-cleanup?target_timestamp=...` |
| 查看日志清理任务进度 | `system_setting.read` | `/api/system-task/list`，由系统信息页消费 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认 API 响应类型、权限 hook 和日志维护页类型通过。
2. `go test ./service/authz ./middleware ./router ./controller` 确认系统任务、日志与权限路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 访问 `http://192.168.0.202:3003/` 和 `/system-settings/operations/logs`，确认页面加载、`/api/option/` 只读请求正常、清理日志按钮渲染正常、控制台无错误；不点击确认清理，避免删除真实日志。

## 本轮实施评审：系统设置运维集成与模型部署保存按钮 Authz 消费

### 需求分析

系统设置的运维集成类页面仍有四个通过 `useUpdateOption()` 写入 `PUT /api/option/` 的保存入口只按 pending、dirty 或 submitting 状态禁用：监控告警、SMTP 邮件、Worker 代理和 io.net 模型部署。后端已经将 `PUT /api/option/` 统一纳入 `system_setting.sensitive_write`，hook 也会在提交前拦截无权限写入；这些页面需要在按钮层消费同一个 `canUpdate/disabledReason`，让受限管理员在点击前就看到一致的权限状态。

本轮目标：

1. 将监控告警、SMTP 邮件、Worker 代理和 io.net 模型部署四个保存按钮接入 `updateOption.canUpdate` 与 `updateOption.disabledReason`。
2. 保持本地草稿编辑、表单校验、SMTP token 空值保留、Worker URL/key 处理、监控规则规范化和 io.net 表单脏状态判断不变。
3. io.net 的 `Test Connection` 调用 `testDeploymentConnectionWithKey(apiKey)`，属于模型部署连接测试能力，不是 `PUT /api/option/` 保存动作；本轮只记录边界，不混入该按钮权限语义。
4. 不修改后端接口、权限表、option payload、缓存刷新和任何支付/模型定价逻辑。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 监控告警 | `web/default/src/features/system-settings/integrations/monitoring-settings-section.tsx` | 保存监控阈值、自动测试和自动禁用/重试规则时消费统一 option 保存权限。 |
| SMTP 邮件 | `integrations/email-settings-section.tsx` | 保存 SMTP 主机、端口、账号、发件人、token 和安全开关时消费统一 option 保存权限。 |
| Worker 代理 | `integrations/worker-settings-section.tsx` | 保存 Worker URL、访问 key 和 HTTP 图片请求开关时消费统一 option 保存权限。 |
| io.net 模型部署 | `integrations/ionet-deployment-settings-section.tsx` | 保存 io.net 启用状态和 API key 时消费统一 option 保存权限；连接测试按钮保持原逻辑。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收方式，并补充能力落地清单。 |

### 风险评估

1. 本轮只改变保存按钮的 `disabled/title`，不改变 `onSubmit` 中的更新集合生成、字段清洗、校验规则和异步提交顺序。
2. 监控告警会影响自动渠道测试和自动禁用恢复，本轮不改变规则解析与保存 key，避免影响调度任务。
3. SMTP、Worker 和 io.net API key 都属于敏感配置；按钮层只做提前禁用，真正的权限边界仍由 `useUpdateOption()` 和后端 `RequirePermission` 兜底。
4. io.net 连接测试可能需要单独映射到 `model.operate` 或 `system_setting.operate`，但它不会持久化 option；本轮不改，避免引入未评审的行为变化。

### 方案评审

采用“同一 `PUT /api/option/` 保存入口，同一按钮权限字段”的方案。页面继续允许受限管理员查看与编辑本地草稿，保存按钮在缺少敏感写权限时禁用并展示 hook 提供的原因；即使存在脚本提交或遗漏路径，hook 层仍会阻止实际请求。io.net 连接测试保留原 pending 语义，后续如要权限化，应单独按模型部署操作能力评审。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 保存监控告警、SMTP 邮件、Worker 代理、io.net 模型部署 option | `system_setting.sensitive_write` | `PUT /api/option/` |
| io.net 连接测试 | 本轮不修改，后续单独评审 | `testDeploymentConnectionWithKey(apiKey)` 对应模型部署连接测试 API |
| 本地编辑、表单校验、开关切换 | 本地交互，无写权限要求 | 不调用后端写接口 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认四个页面能识别统一权限字段。
2. `go test ./service/authz ./middleware ./router ./controller` 确认系统设置、模型部署和权限路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. `cd web/default && node scripts/sync-i18n.mjs` 确认未产生遗漏翻译 key。
5. 使用 MCP 访问 `http://192.168.0.202:3003/`，再打开 `/system-settings/operations/monitoring`、`/system-settings/operations/email`、`/system-settings/operations/worker`、`/system-settings/models/model-deployment`，确认页面加载、`/api/option/` 只读请求正常、保存按钮渲染正常、控制台无错误；不触发真实保存和 io.net 连接测试。

## 本轮实施评审：支付配置保存与合规确认按钮 Authz 消费

### 需求分析

支付配置页是系统设置中风险最高的 option 表单之一，包含充值单价、充值金额、支付方式、Epay、Stripe、Creem 凭证以及支付合规确认。后端已经将普通保存入口 `PUT /api/option/` 与合规确认入口 `POST /api/option/payment_compliance` 都纳入 `system_setting.sensitive_write`，但默认前端支付页仍有五个保存按钮和一个合规确认入口只按 pending 或弹窗输入状态禁用。受限管理员会看到可点击入口，直到提交时才被后端或 hook 拦截，权限语义不如其他系统设置页面一致。

本轮目标：

1. 将支付配置页的 `Save general settings`、`Save Epay settings`、`Save Stripe settings`、`Save Creem settings` 和 `Save all settings` 接入 `updateOption.canUpdate` 与 `updateOption.disabledReason`。
2. 将支付合规确认入口和风险确认弹窗最终确认按钮接入同一 `system_setting.sensitive_write` 权限；缺少权限时不打开弹窗、不调用 `POST /api/option/payment_compliance`。
3. 保持支付表单字段、JSON 可视化编辑器、密钥空值保留、Stripe/Creem/Epay 保存拆分、合规确认文案和后端接口不变。
4. 不处理 Waffo 与 Waffo Pancake 子表单，它们有独立组件和保存状态，后续单独评审，避免在一个切片里混入过多支付网关逻辑。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 风险确认弹窗 | `web/default/src/components/risk-acknowledgement-dialog.tsx` | 新增可选确认禁用原因，供支付合规确认在权限不足时禁用最终确认按钮。 |
| 支付配置页 | `web/default/src/features/system-settings/integrations/payment-settings-section.tsx` | 保存按钮和合规确认入口消费 `system_setting.sensitive_write`；本地编辑和 JSON/可视化模式切换不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收方式，并补充能力落地清单。 |

### 风险评估

1. 本轮只改按钮禁用、标题提示和合规确认 mutation 前置检查，不改支付 option 的更新集合生成、字段清洗、JSON 规范化、密钥空值保留和保存顺序。
2. 支付配置直接影响充值入账、订阅、兑换码和邀请奖励，必须保留后端权限作为最终边界；前端按钮只是提前表达同一权限语义。
3. `RiskAcknowledgementDialog` 当前只由支付合规确认使用，新增可选禁用参数保持默认不禁用，避免改变其他调用方行为。
4. Waffo/Waffo Pancake 子组件保存逻辑较独立，且包含自定义 loading 与多方法编辑器；本轮不触碰，降低支付网关配置误改风险。

### 方案评审

采用“支付页统一消费 `system_setting.sensitive_write`，风险弹窗提供通用确认禁用扩展”的方案。普通支付 option 保存继续复用 `useUpdateOption()` 的 `canUpdate/disabledReason`，合规确认入口也复用同一权限结果，并在打开弹窗和最终确认两个阶段都做前置保护。这样页面层、hook 层和后端路由权限表保持一致，同时不改变任何支付业务 payload。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 保存通用支付、Epay、Stripe、Creem、全部支付 option | `system_setting.sensitive_write` | `PUT /api/option/` |
| 打开支付合规确认弹窗 | `system_setting.sensitive_write` | 前端本地检查 |
| 确认支付合规声明 | `system_setting.sensitive_write` | `POST /api/option/payment_compliance` |
| JSON/可视化编辑模式切换、本地字段编辑 | 本地交互，无写权限要求 | 不调用后端写接口 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认支付页和风险确认弹窗类型通过。
2. `go test ./service/authz ./middleware ./router ./controller` 确认系统设置权限路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. `cd web/default && node scripts/sync-i18n.mjs` 确认未产生遗漏翻译 key。
5. 使用 MCP 访问 `http://192.168.0.202:3003/` 和 `/system-settings/billing/payment`，确认页面加载、`/api/option/` 只读请求正常、支付保存按钮和合规确认入口渲染正常、控制台无错误；不触发真实保存、不确认支付合规。

## 本轮实施评审：Waffo 支付子表单保存按钮 Authz 消费

### 需求分析

支付配置主表单已经将通用、Epay、Stripe、Creem、全部保存以及支付合规确认入口接入 `system_setting.sensitive_write`。同一支付页面下的 Waffo 与 Waffo Pancake 子表单仍然通过 `useUpdateOption()` 写入 `PUT /api/option/`，但保存按钮只按本地 `loading` 禁用；缺少系统设置敏感写权限的管理员仍会看到可点击保存入口，直到提交时才被 hook 或后端拦截。

本轮目标：

1. 将 Waffo 子表单的 `Save Changes` 和 Waffo Pancake 子表单的 `Save Waffo Pancake settings` 接入 `updateOption.canUpdate` 与 `updateOption.disabledReason`。
2. 在 `handleSave` 中增加同一权限前置检查，防止脚本调用或按钮遗漏路径发起无权限保存。
3. 保持 Waffo 支付方式本地新增/编辑/删除、图标上传、密钥空值保留、Pancake 必填校验、URL 去尾斜杠和 option key 列表不变。
4. 不处理 Waffo 支付方式弹窗的本地确认按钮，因为它只修改本地草稿，不调用后端写接口。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Waffo 聚合支付 | `web/default/src/features/system-settings/integrations/waffo-settings-section.tsx` | 最终保存按钮和保存 handler 消费统一 option 保存权限；本地支付方式编辑器不变。 |
| Waffo Pancake 托管支付 | `integrations/waffo-pancake-settings-section.tsx` | 最终保存按钮和保存 handler 消费统一 option 保存权限；启用时的必填校验不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收方式，并补充能力落地清单。 |

### 风险评估

1. 本轮只改最终保存按钮 disabled/title 与 handler 权限前置检查，不改任何支付网关字段、序列化、密钥保存条件或校验规则。
2. Waffo 支付方式列表包含图标和支付方法元数据，本轮允许继续本地编辑草稿，避免无权限用户无法查看或临时整理配置；真正持久化仍由保存按钮和 hook/后端兜底。
3. Waffo Pancake 启用校验涉及商户、店铺、商品、Webhook 公钥和价格边界，本轮不改变校验顺序，权限不足时先提示权限，权限满足时才进入原业务校验。
4. 两个组件都保留本地 `loading` 状态，避免多 key 连续保存期间重复点击；同时读取 `updateOption.isPending` 作为额外保护。

### 方案评审

采用“与支付主表单一致的 `system_setting.sensitive_write` 保存语义”的方案。两个子组件继续复用 `useUpdateOption()`，保存按钮缺少权限时禁用并展示统一无权限原因；`handleSave` 开头也做同一检查，让前端行为与后端 `/api/option/` 权限表一致。支付方式弹窗和字段编辑保持本地草稿交互，不扩大权限禁用范围。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 保存 Waffo 聚合支付 option | `system_setting.sensitive_write` | `PUT /api/option/` |
| 保存 Waffo Pancake 托管支付 option | `system_setting.sensitive_write` | `PUT /api/option/` |
| Waffo 支付方式本地新增/编辑/删除、图标上传、字段编辑 | 本地交互，无写权限要求 | 不调用后端写接口 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认两个 Waffo 子表单类型通过。
2. `go test ./service/authz ./middleware ./router ./controller` 确认系统设置权限路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. `cd web/default && node scripts/sync-i18n.mjs` 确认未产生遗漏翻译 key。
5. 使用 MCP 访问 `http://192.168.0.202:3003/` 和 `/system-settings/billing/payment`，确认页面加载、`/api/option/` 只读请求正常、Waffo 与 Waffo Pancake 保存按钮渲染正常、控制台无错误；不触发真实保存。

## 本轮实施评审：控制台内容基础表单保存按钮 Authz 消费

### 需求分析

系统设置的“控制台内容”分组中，Dashboard、Chat Presets 和 Drawing 三个实际路由页面都通过 `useUpdateOption()` 写入 `PUT /api/option/`，但保存按钮仍只按 `updateOption.isPending` 禁用。后端与 hook 已经统一要求 `system_setting.sensitive_write`，这些基础表单需要在按钮层消费 `canUpdate/disabledReason`，与站点、运维、支付等系统设置页面保持同一权限体验。

本轮目标：

1. 将 `/system-settings/content/dashboard` 的 `Save Changes` 接入 `updateOption.canUpdate` 与 `updateOption.disabledReason`。
2. 将 `/system-settings/content/chat` 的 `Save chat settings` 接入同一保存权限字段。
3. 将 `/system-settings/content/drawing` 的 `Save drawing settings` 接入同一保存权限字段。
4. 不处理 FAQ、API Addresses、Announcements、Uptime Kuma 四个列表编辑器页面；它们包含本地批量新增/编辑/删除草稿，后续单独评审。
5. `JsonToggleSection` 当前没有实际调用方，本轮不做无效改动。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Dashboard 内容设置 | `web/default/src/features/system-settings/content/dashboard-section.tsx` | 数据导出开关、间隔和默认粒度保存按钮消费统一 option 保存权限。 |
| Chat Presets | `content/chat-settings-section.tsx` | 视觉/JSON 聊天预设保存按钮消费统一 option 保存权限；编辑模式切换不变。 |
| Drawing 内容设置 | `content/drawing-settings-section.tsx` | 绘图与 Midjourney 相关开关保存按钮消费统一 option 保存权限。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收方式，并补充能力落地清单。 |

### 风险评估

1. 本轮只改保存按钮 disabled/title，不改 Dashboard 导出间隔校验、Chat JSON/可视化编辑、Drawing 开关列表和任何 option key。
2. Dashboard 导出和 Drawing 开关会影响前台功能可见性与后台任务行为，前端按钮只是提前表达后端已有权限；`useUpdateOption()` 仍是提交前保护。
3. Chat 视觉编辑器允许本地调整预设，本轮不禁止本地草稿编辑，避免无权限管理员失去查看和整理能力；真正持久化由保存按钮控制。
4. 列表编辑器页面存在批量草稿和本地删除提示，和本轮基础表单不同；分开处理能减少误伤本地编辑语义的风险。

### 方案评审

采用“同一 `PUT /api/option/` 保存入口，同一按钮权限字段”的方案。三个页面继续允许本地编辑和表单校验，保存按钮在缺少 `system_setting.sensitive_write` 时禁用并展示统一无权限原因；hook 层保留最终前置检查，防止快捷键、脚本或未来遗漏按钮路径绕过前端 UI。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 保存 Dashboard、Chat Presets、Drawing option | `system_setting.sensitive_write` | `PUT /api/option/` |
| 本地开关切换、Chat 视觉/JSON 编辑模式切换、字段编辑 | 本地交互，无写权限要求 | 不调用后端写接口 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认三个内容设置页面类型通过。
2. `go test ./service/authz ./middleware ./router ./controller` 确认系统设置权限路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. `cd web/default && node scripts/sync-i18n.mjs` 确认未产生遗漏翻译 key。
5. 使用 MCP 访问 `http://192.168.0.202:3003/`，再打开 `/system-settings/content/dashboard`、`/system-settings/content/chat`、`/system-settings/content/drawing`，确认页面加载、`/api/option/` 只读请求正常、保存按钮渲染正常、控制台无错误；不触发真实保存。

| 日期 | 能力 | 文件 | 说明 |
|------|------|------|------|
| 2026-07-07 | 全量文件差异索引 | `docs/features/new-api-main-diff-inventory.md`、`scripts/compare-new-api-main.sh` | 新增可重复生成的文件级差异清单，主文档继续承载功能和页面解释。 |
| 2026-07-07 | 匿名请求体限制 | `common/request_body_limit.go`、`middleware/request_body_limit.go`、`router/api-router.go` | 吸收 new-api-main 的匿名入口保护，并补齐 NexusTok 现有 Waffo Pancake webhook 路由。 |
| 2026-07-07 | 受保护 Fetch / SSRF | `common/ssrf_protection.go`、`service/protected_fetch_client.go`、`service/download.go`、`service/webhook.go`、`service/user_notify.go` | 为用户可控 URL 增加 Dial 阶段 DNS 解析校验，阻断 DNS rebinding；不替换 Relay 全局 client。 |
| 2026-07-07 | 额度饱和保护 | `common/quota_math.go`、`pkg/billingexpr/round.go`、`relay/helper/price.go`、`service/text_quota.go` | 新增统一配额转换 helper，表达式计费、文本结算、标准预扣和按次预扣在异常超大值/NaN 下饱和到 int32 边界，避免溢出造成反向计费。 |
| 2026-07-07 | 额度饱和日志审计 | `relay/common/relay_info.go`、`service/log_info_generate.go`、`service/quota.go`、`service/task_billing.go`、`pkg/billingexpr/settle.go` | 将首次配额饱和事件记录到 `RelayInfo` 并写入 `other.admin_info.quota_saturation`，覆盖文本、音频、WSS 与任务差额日志，普通用户视图自动脱敏。 |
| 2026-07-07 | 工具与违规费用额度保护 | `service/tool_billing.go`、`service/violation_fee.go`、`service/quota_saturation_test.go` | 工具调用附加费改用统一 `QuotaRound` 并对总额累加做饱和保护；Grok 违规费用改用 checked decimal 转换，异常饱和写入管理员日志审计。 |
| 2026-07-07 | 充值入账额度保护 | `common/quota_math.go`、`model/topup.go`、`controller/topup.go`、`model/log.go` | 新增 decimal 截断型额度转换，保持充值历史入账语义并对 Stripe、EPay、Creem、Waffo、Waffo Pancake 入账做饱和保护；异常写入充值日志 `admin_info.quota_saturation`。 |
| 2026-07-07 | 视频任务与渠道测试额度保护 | `controller/task_video.go`、`controller/channel-test.go`、`controller/channel_test_internal_test.go` | 视频任务 token 重算改用 checked float 转换并记录管理员审计；渠道测试日志改用统一 `QuotaRound` 并注入 quota saturation，同时把视频响应脱敏 JSON 处理切到 `common.*`。 |
| 2026-07-07 | 顶栏模块后端鉴权 | `middleware/header_nav.go`、`router/api-router.go`、`middleware/header_nav_test.go` | 吸收 HeaderNavModuleAuth，使 pricing/rankings 公开 API 与前端导航 `enabled/requireAuth` 配置保持一致；pricing 关联的 perf-metrics 对匿名访问做同源控制。 |
| 2026-07-07 | 渠道敏感字段 fail-closed | `controller/channel_authz.go`、`controller/channel.go`、`controller/channel_authz_test.go` | 更新渠道时按原始请求字段判断敏感写：Key、BaseURL、请求覆盖、设置、凭证模式和未知字段默认需要敏感权限；完整 Authz 前暂以 Root 权限兜底，普通 Admin 仍可更新模型、分组、优先级等运营字段。 |
| 2026-07-07 | 渠道路由注册拆分 | `router/channel-router.go`、`router/channel_router_test.go`、`router/api-router.go` | 将 `/api/channel` 路由从主 API 路由中抽出，保持现有 AdminAuth/RootAuth 行为不变，并为后续 Authz 权限表接入提供单一落点。 |
| 2026-07-07 | Codex 用量重置后端 API | `controller/codex_usage.go`、`service/codex_wham_usage.go`、`router/channel-router.go`、`service/codex_wham_usage_test.go` | 复用 Codex WHAM 用量查询链路，补齐 reset credits 查询和消耗一次重置额度的后端接口；请求头、路径和 reset 消费请求体已用 httptest 覆盖。 |
| 2026-07-07 | Codex 用量重置默认前端 | `web/default/src/features/channels/api.ts`、`web/default/src/features/channels/components/dialogs/codex-usage-dialog.tsx`、`web/default/src/i18n/locales/*.json` | 默认前端 Codex 用量弹窗已接入 reset credits 查询、明细折叠区、确认重置和重置后刷新；新增文案已补齐 en/zh/fr/ja/ru/vi。 |
| 2026-07-07 | 系统实例心跳后端 | `model/system_instance.go`、`service/system_instance.go`、`controller/system_info.go`、`router/system-info-router.go` | 引入 NexusTok 原生 SystemInstance 心跳：启动后定时上报节点名、主从角色、版本、系统资源和最后心跳；新增 Root 只读 `/api/system-info/instances`，为后续 `/system-info` 页面和 SystemTask 观测打底。 |
| 2026-07-07 | 系统信息实例页面 | `web/default/src/features/system-info/*`、`web/default/src/routes/_authenticated/system-info/index.tsx`、`web/default/src/i18n/locales/*.json` | 默认前端新增 Root-only `/system-info` 页面，展示节点状态、角色、CPU/内存/存储、版本、runtime、启动时间和最近心跳，支持自动刷新与手动刷新。 |
| 2026-07-07 | 系统任务只读后端 | `model/system_task.go`、`controller/system_task.go`、`router/system-task-router.go` | 引入 NexusTok 原生 SystemTask/SystemTaskLock 模型、三库兼容迁移、Root 只读 `/api/system-task/list`、`/api/system-task/current`、`/api/system-task/:task_id` 和锁生命周期测试；暂不开放创建接口，等待真实 runner/handler 接入。 |
| 2026-07-07 | 系统任务 runner 与日志清理 | `service/system_task.go`、`controller/system_task.go`、`model/log.go`、`main.go` | 引入主节点 SystemTask runner、handler 注册、租约 heartbeat、过期租约清理和日志清理 handler；新增 `POST /api/system-task/log-cleanup`，日志清理改为异步任务执行并写入 processed/remaining/progress/result。 |
| 2026-07-07 | 系统任务前端面板 | `web/default/src/features/system-info/*`、`web/default/src/i18n/locales/*.json`、`web/default/src/i18n/static-keys.ts` | 默认前端 `/system-info` 新增 System Tasks 面板，按活动任务和历史任务展示状态、进度、执行节点、更新时间、错误和日志清理结果；活动任务存在时自动轮询，新增文案已补齐 en/zh/fr/ja/ru/vi。 |
| 2026-07-07 | 批量渠道测试系统任务 | `controller/channel-test.go`、`controller/system_task_handlers.go`、`service/system_task.go`、`setting/operation_setting/monitor_setting.go`、`main.go`、`web/default/src/features/channels/api.ts` | `/api/channel/test` 改为创建 `channel_test` SystemTask，手动重复触发返回 409 和既有任务 ID；自动测试从旧进程内 goroutine 迁入 SystemTask scheduler，支持 `scheduled_all`/`passive_recovery` 模式、进度 reporter 和历史 summary。MCP 已验证 200 创建任务、409 去重和 `/api/system-task/list` 可观测 pending 任务。 |
| 2026-07-07 | 上游模型同步系统任务 | `controller/channel_upstream_update.go`、`controller/system_task_handlers.go`、`main.go`、`web/default/src/features/channels/hooks/use-channel-upstream-updates.ts` | `/api/channel/upstream_updates/detect_all` 改为创建 `model_update` SystemTask，返回任务 ID 并在活动任务存在时返回 409；旧启动 goroutine 移除，定时同步进入 SystemTask scheduler。任务执行支持进度 reporter、context cancellation、手动强制检测不自动应用、历史 result 汇总和默认前端 toast/i18n 更新。MCP 已验证 200 入队、409 去重、任务历史详情和 `/api/system-task/list` 可观测。 |
| 2026-07-07 | 账号池检测系统任务 | `service/account_pool_check.go`、`controller/system_task_handlers.go`、`model/system_task.go`、`service/system_task.go` | 账号池批量检测不再依赖进程内检测队列；创建 `PoolAccountCheckTask` 后同步创建独立 ActiveKey 的 `account_pool_check` SystemTask，执行租约仍按类型串行，结果保留账号池专用脱敏历史，同时在 `/system-info` System Tasks 面板可观测进度和聚合结果。 |
| 2026-07-07 | 订阅维护系统任务 | `service/subscription_reset_task.go`、`model/system_task.go`、`web/default/src/features/system-info/components/system-tasks-panel.tsx` | 订阅过期、周期额度重置和预消费记录清理从独立 ticker 迁入 `subscription_maintenance` SystemTask；启动入口只创建首次任务，周期执行由 scheduler 和数据库租约统一管理，并在 `/system-info` 以六语标签展示任务类型。 |
| 2026-07-07 | Midjourney 与异步任务轮询系统任务 | `controller/midjourney.go`、`service/task_polling.go`、`main.go` | 绘图任务和通用异步任务轮询从独立无限 goroutine 迁入 `midjourney_poll`/`async_task_poll` SystemTask；旧启动入口只负责创建首次任务，周期执行由 scheduler 和数据库租约统一管理。保留 Midjourney 失败退款、通用任务超时清理、Suno/视频任务分发和差额结算路径，并补充空上游任务 ID 修复、handler 完成和启动入队测试。 |
| 2026-07-07 | 流量账本查询后端 | `model/usedata.go`、`controller/usedata.go`、`router/api-router.go` | 扩展 `quota_data` 记录 node/token/group/channel 维度，新增 `/api/data/flow` 与 `/api/data/flow/self` 聚合接口；Root 可看节点、Token 和渠道，Admin 隐藏 Token/节点，普通用户仅查看自己的 Token/group/model 聚合，为后续 dashboard Sankey 图提供后端数据。 |
| 2026-07-07 | 订阅余额支付与钱包溢出控制后端 | `model/subscription.go`、`controller/subscription.go`、`router/api-router.go`、`service/billing_session.go`、`service/quota.go` | 新增套餐 `allow_balance_pay`、`allow_wallet_overflow` 和用户订阅钱包溢出快照，提供 `/api/subscription/balance/pay` 余额购买订阅接口；余额扣款、订阅创建和成功订单在同一事务内完成，`subscription_first` 在订阅额度不足时按活跃订阅快照决定是否允许钱包 fallback。 |
| 2026-07-07 | 权限 catalog 后端 | `service/authz/*`、`controller/authz.go`、`router/authz-router.go` | 新增 `/api/authz/catalog` 只读权限 schema，并持续扩展到渠道、账号池、用户、模型、订阅、兑换码、日志、用量数据和系统设置九类资源，返回动作定义与 Root/Admin 基线授权矩阵；当前不引入 Casbin 存储，为后续用户级 override 和路由权限表迁移提供稳定 schema。 |
| 2026-07-07 | 用户权限矩阵回传 | `controller/user.go`、`controller/user_authz_test.go`、`web/default/src/stores/auth-store.ts` | `/api/user/self` 在 `permissions.admin_permissions` 回传 Authz 能力矩阵，并为默认前端用户状态补充类型声明；当前只反映系统角色基线，不引入用户 override 或改变服务端权限判断。 |
| 2026-07-07 | 默认前端入口消费权限矩阵 | `web/default/src/lib/admin-permissions.ts`、`web/default/src/hooks/use-admin-permission.ts`、`web/default/src/hooks/use-sidebar-data.ts`、`web/default/src/routes/_authenticated/*` | 默认前端新增管理权限 helper/hook，以 `permissions.admin_permissions` 为优先来源、旧角色为兼容 fallback；管理侧边栏和渠道、账号池、模型、用户、兑换码、订阅入口路由已按 read 能力显隐/放行，Usage Logs 与 Dashboard 的跨用户视图也开始读取对应 read 能力，Root-only 系统设置、系统信息和计费设置继续保持 Root 边界。 |
| 2026-07-07 | 渠道页按钮级权限消费 | `web/default/src/features/channels/hooks/use-channel-permissions.ts`、`web/default/src/features/channels/components/*` | 默认前端渠道管理页开始消费同一权限矩阵：`channel.operate` 控制测试、启停、余额和检测；`channel.write` 控制普通模型/分组/标签等编辑；`channel.sensitive_write` 控制新建、复制、删除、密钥、多 Key、渠道账号池和上游模型应用；`channel.secret_view` 控制密钥查看。编辑抽屉在缺少敏感写时会裁剪提交 payload，只发送非敏感字段，保持与后端 Root 兜底策略一致。 |
| 2026-07-07 | 订阅余额支付与钱包兜底默认前端 | `web/default/src/features/subscriptions/*`、`web/default/src/features/wallet/components/subscription-plans-card.tsx`、`web/default/src/i18n/locales/*.json` | 默认前端已消费后端 `allow_balance_pay`、`allow_wallet_overflow` 和 `/api/subscription/balance/pay`：管理员可在套餐抽屉配置余额兑换与钱包兜底策略，套餐表展示余额支付能力，用户购买弹窗展示所需/可用余额并支持直接用钱包余额购买，成功后刷新订阅和当前用户余额；新增文案已补齐 en/zh/fr/ja/ru/vi。 |
| 2026-07-08 | GORM v2 行锁原生化 | `model/locking.go`、`model/locking_test.go`、`model/subscription.go`、`model/topup.go`、`model/redemption.go`、`model/user.go` | 新增模型层统一 `lockForUpdate` helper，MySQL/PostgreSQL 使用 GORM v2 `clause.Locking` 生成真实 `FOR UPDATE`，SQLite 自动跳过不兼容语法；订阅、充值、兑换和邀请额度转移热点事务已替换旧 GORM v1 query option 写法，测试覆盖 SQL 生成和关键业务路径。 |
| 2026-07-08 | 会话守卫仅 401 登出 | `web/default/src/routes/_authenticated/route.tsx` | 默认前端受保护路由预检 `/api/user/self` 时只把 HTTP 401 视为明确 session 失效；网络错误、超时或 5xx 不再误清本地用户，也不会设置已验证标记，下一次导航继续重验。全局 API 401 拦截器保持不变，普通业务请求仍会清登录态。 |
| 2026-07-08 | 顶栏模块前端健壮解析 | `web/default/src/lib/nav-modules.ts`、`web/default/src/hooks/use-top-nav-links.ts`、`web/default/src/components/layout/components/public-header.tsx`、`web/default/src/components/layout/components/top-nav.tsx`、`web/default/src/features/system-settings/maintenance/*` | 将 HeaderNavModules 前端解析集中到单一 helper，统一兼容布尔、数字、字符串和对象格式；顶部导航、页面守卫和系统设置表单共享同一语义，未登录且 `requireAuth=true` 时公共 Header 与后台 TopNav 均渲染禁用态。 |
| 2026-07-08 | Usage Logs 移动端卡片与筛选工具条 | `web/default/src/features/usage-logs/components/*`、`web/default/src/i18n/locales/*.json` | 默认前端 Usage Logs 已接入专用移动端语义卡片，common/drawing/task 三类日志在手机端突出模型、成本、状态、任务 ID、渠道、耗时和失败原因；筛选工具条在移动端固定日期范围并将长条件收纳到 Drawer，桌面表格和现有 URL/API 查询语义保持不变。 |
| 2026-07-08 | Dashboard Flow 流量账本可视化 | `web/default/src/features/dashboard/*`、`web/default/src/components/{multi-select,datetime-picker}.tsx`、`web/default/src/i18n/locales/*.json` | 默认前端新增 `/dashboard/flow` 分区，消费 `/api/data/flow*`，支持 quota/tokens/requests 指标、Top 限制、溢出聚合/隐藏、用户/节点筛选、阶段显隐和点击高亮；补齐 Flow 测试、六语翻译和筛选控件可访问性。 |
| 2026-07-08 | Responses 请求反向兼容与 Gemini Responses | `service/openaicompat/*`、`service/openai_chat_responses_compat.go`、`dto/openai_response.go`、`relay/channel/gemini/*` | 新增 Responses 请求降级 Chat 转换、Chat 响应/流式事件回转 Responses，并让 Gemini 渠道支持 `/v1/responses` 安全子集；custom/freeform 工具按 Gemini 能力过滤，测试覆盖显式零值、工具调用、reasoning、incomplete 状态、非流式 JSON 和流式 SSE。 |
| 2026-07-08 | 全局管理操作审计兜底 | `middleware/audit.go`、`middleware/auth.go`、`model/log.go`、`controller/audit.go`、`web/default/src/features/usage-logs/*` | Admin/Root 写操作新增全局审计兜底，记录操作者、action、路由、路径参数、状态码和业务成功状态；不采集请求体，普通用户日志视图剥离 `admin_info`/`audit_info`，Usage Logs 管理日志详情展示结构化审计信息。 |
| 2026-07-08 | 渠道 Authz 路由权限表灰度 enforcement | `service/authz/*`、`middleware/auth.go`、`router/channel-router.go`、`controller/channel_authz.go` | 新增 `authz.Can`、渠道 permission 常量和 `middleware.RequirePermission`，并将 `/api/channel` 注册迁移为读/操作/写/敏感写/密钥查看权限表；Root-only 与安全验证中间件继续保留，渠道敏感字段二次校验复用同一 `ChannelSensitiveWrite` 判定。 |
| 2026-07-08 | 账号池 Authz 路由权限表灰度 enforcement | `service/authz/*`、`router/account_pool-router.go`、`router/api-router.go`、`router/account_pool_router_test.go` | 为 NexusTok 独有 `/api/account-pool` 全量路由接入 account_pool 权限表：查询/脱敏日志走 read，检测/状态/脱敏导出走 operate，分组配置走 write，认证文件、账号生命周期、OAuth 和凭证刷新走 sensitive_write；主路由文件只保留注册调用。 |
| 2026-07-08 | 订阅 Admin Authz 路由权限表灰度 enforcement | `service/authz/*`、`router/subscription-router.go`、`router/api-router.go`、`router/subscription_router_test.go` | 为 `/api/subscription/admin` 接入 subscription 权限表：套餐和用户订阅查询走 read，套餐创建/更新/启停走 write，绑定、创建和失效用户订阅走 operate，删除用户订阅走 sensitive_write；用户购买和支付回调路由保持原有认证/匿名回调边界。 |
| 2026-07-08 | 模型管理 Authz 路由权限表灰度 enforcement | `service/authz/*`、`router/model-router.go`、`router/api-router.go`、`router/model_router_test.go` | 为 `/api/vendors`、`/api/models`、`/api/deployments` 接入 model 权限表：元数据和部署查询走 read，上游预览/连接测试/价格估算/扩容走 operate，厂商/模型/部署创建编辑和定价更新走 write，删除模型、厂商和部署走 sensitive_write；NexusTok 扩展的模型定价接口也纳入同一原生权限体系。 |
| 2026-07-08 | Waffo Pancake 用户侧支付路由补齐 | `router/api-router.go`、`router/api_router_payment_test.go` | 对齐 new-api 的用户自助充值入口，补齐默认前端已调用的 `/api/user/waffo-pancake/amount` 与 `/api/user/waffo-pancake/pay`；金额试算和支付发起复用现有 Waffo Pancake controller，支付发起保留 `CriticalRateLimit`，webhook 验签和入账事务不变。 |
| 2026-07-08 | 用户管理 Authz 路由权限表灰度 enforcement | `service/authz/*`、`router/user-router.go`、`router/api-router.go`、`controller/user_authz.go`、`router/user_router_test.go` | 为 `/api/user` 管理员子路由接入 user 权限表：用户/充值/绑定/2FA 查询走 read，绑定清理、Passkey/2FA 重置和普通启停走 operate，普通用户资料编辑走 write，创建、硬删除、管理员完成充值走 sensitive_write；`ManageUser` 复合接口对删除、升降级和额度调整额外执行 `user.sensitive_write` 二次校验。 |
| 2026-07-08 | 兑换码 Authz 独立资源与路由权限表 | `service/authz/*`、`router/redemption-router.go`、`router/api-router.go`、`web/default/src/lib/admin-permissions.ts`、`web/default/src/routes/_authenticated/redemption-codes/index.tsx` | 新增 `redemption` 权限资源并让 `/api/redemption` 接入独立路由权限表：列表/搜索/详情走 read，创建和编辑走 write，删除单个兑换码与批量清理无效兑换码走 sensitive_write；默认前端侧边栏和页面守卫改为读取 `redemption.read`，不再复用 `user.read`。 |
| 2026-07-08 | 日志与用量数据 Authz 独立资源 | `service/authz/*`、`router/log-data-router.go`、`router/api-router.go`、`web/default/src/features/usage-logs/*`、`web/default/src/features/dashboard/*` | 新增 `usage_log` 与 `usage_data` 权限资源，管理员 `/api/log` 和 `/api/data` 读接口进入独立权限表，历史日志同步删除归入 `usage_log.sensitive_write`；普通用户 self/token 路径保持原认证边界，默认前端 Usage Logs 与 Dashboard 的跨用户视图改为读取对应 read 权限。 |
| 2026-07-08 | 基础用量查询聚合与统计空值保护 | `model/usedata.go`、`model/log.go`、`model/usedata_flow_test.go`、`model/log_stat_test.go` | 对齐 new-api 的 Dashboard 基础用量聚合语义，`/api/data/self` 和按 username 查询只返回 user/model/hour 汇总，不再带出 token/channel/node/group 细维度值；日志统计统一使用跨三库的 `COALESCE`，空结果稳定返回 0。 |
| 2026-07-08 | 分组与预填充分组 Authz 路由权限表 | `router/group-router.go`、`router/api-router.go`、`router/group_router_test.go`、`model/prefill_group.go` | 为管理端 `/api/group` 与 `/api/prefill_group` 接入 model 权限表：分组和预填模板查询走 read，预填模板创建/更新走 write，删除预填模板走 sensitive_write；普通用户自助分组路径不变，并同步将预填组 JSON fallback 序列化切到 `common.Marshal`。 |
| 2026-07-08 | 系统设置与运行维护 Authz 路由权限表 | `service/authz/permission.go`、`router/system-setting-router.go`、`router/api-router.go`、`router/system_setting_router_test.go` | 为 `/api/status/test`、`/api/option`、`/api/custom-oauth-provider`、`/api/performance` 和 `/api/ratio_sync` 接入 system_setting 权限表；只读诊断保持 Admin 边界，其余系统设置和运行维护接口继续 RootAuth 保底，按 read/operate/sensitive_write 做二次校验。 |
| 2026-07-08 | 系统信息与系统任务 Authz 路由权限表 | `router/system-info-router.go`、`router/system-task-router.go`、`router/system_info_router_test.go`、`router/system_task_router_test.go` | 为 `/api/system-info/instances` 和 `/api/system-task` 查询接口接入 `system_setting.read`；日志清理任务创建入口接入 `system_setting.sensitive_write` 并额外要求 `usage_log.sensitive_write`，所有路径继续 RootAuth 保底。 |
| 2026-07-08 | 绘图与异步任务日志 Authz 路由权限表 | `router/task-log-router.go`、`router/api-router.go`、`router/task_log_router_test.go` | 为管理员跨用户 `GET /api/mj/` 与 `GET /api/task/` 接入 `usage_log.read`；`/self` 用户自助查询继续只按当前用户认证隔离，不进入管理员权限表。 |
| 2026-07-08 | Authz catalog 路由权限表 | `router/authz-router.go`、`router/authz_router_test.go` | 为 `GET /api/authz/catalog` 接入 `system_setting.read`，继续保留 `AdminAuth`，让权限 schema 只读入口也进入统一灰度权限表。 |
| 2026-07-08 | 模型管理前端按钮级 Authz 消费 | `web/default/src/features/models/*` | 默认前端模型管理页新增 `useModelPermissions` 并让模型、厂商、预填组和部署的写入/操作/删除按钮消费 `model.write/operate/sensitive_write`，与后端模型路由权限表保持一致。 |
| 2026-07-08 | 用户管理前端按钮级 Authz 消费 | `web/default/src/features/users/*`、`web/default/src/features/subscriptions/hooks/*`、`web/default/src/features/subscriptions/components/dialogs/user-subscriptions-dialog.tsx` | 默认前端用户管理页新增 `useUserPermissions`，让用户资料、生命周期、安全凭据、绑定解绑、额度调整和硬删除消费 `user.write/operate/sensitive_write`；用户行内订阅弹窗按跨资源 `subscription.read/operate/sensitive_write` 控制。 |
| 2026-07-08 | 订阅管理前端按钮级 Authz 消费 | `web/default/src/features/subscriptions/*` | 默认前端订阅套餐页复用 `useSubscriptionPermissions`，让套餐创建、编辑、启停和保存提交消费 `subscription.write`，并保留支付合规确认锁。 |
| 2026-07-08 | 兑换码前端按钮级 Authz 消费 | `web/default/src/features/redemption-codes/*` | 默认前端兑换码页新增 `useRedemptionPermissions`，让兑换码创建、编辑、启停和保存消费 `redemption.write`，单个删除与批量清理无效码消费 `redemption.sensitive_write`。 |
| 2026-07-08 | 系统设置前端运行维护 Authz 消费 | `web/default/src/features/system-settings/*` | 默认前端系统设置运行维护入口新增 `useSystemSettingPermissions` 并让性能维护、渠道亲和缓存、上游倍率同步、模型倍率重置和自定义 OAuth provider 操作消费 `system_setting.operate/sensitive_write`，与后端系统设置路由权限表保持一致。 |
| 2026-07-08 | 系统设置通用 option 保存前置 Authz 保护 | `web/default/src/features/system-settings/hooks/use-update-option.ts` | 默认前端所有通过 `useUpdateOption()` 发起的 `PUT /api/option/` 保存会先检查 `system_setting.sensitive_write`；缺少权限时前端不发请求并复用无权限提示，同时向后续逐页按钮禁用暴露 `canUpdate/disabledReason`。 |
| 2026-07-08 | 系统设置高频表单保存按钮 Authz 消费 | `web/default/src/features/system-settings/{general/system-info-section.tsx,auth/basic-auth-section.tsx,request-limits/rate-limit-section.tsx}` | 站点信息、基础认证和请求限流三处高频表单的保存按钮直接消费 `useUpdateOption()` 暴露的 `canUpdate/disabledReason`，缺少 `system_setting.sensitive_write` 时在按钮层禁用并保留 hook 级提交前保护。 |
| 2026-07-08 | 系统设置认证与安全表单保存按钮 Authz 消费 | `web/default/src/features/system-settings/{auth/oauth-section.tsx,auth/passkey-section.tsx,auth/bot-protection-section.tsx,request-limits/sensitive-words-section.tsx,request-limits/ssrf-section.tsx}` | OAuth、Passkey、Turnstile、敏感词和 SSRF 五类认证/安全表单的保存按钮直接消费 `useUpdateOption()` 暴露的保存权限字段，受限管理员可查看和编辑草稿但不能提交系统 option 写入。 |
| 2026-07-08 | 系统设置站点、计费与运维普通表单保存按钮 Authz 消费 | `web/default/src/features/system-settings/{general,maintenance}/*` | 系统行为、配额、货币显示、签到、公告、顶部导航、侧边栏模块和日志记录设置的保存按钮直接消费 `useUpdateOption()` 暴露的保存权限字段；日志清理危险操作保留为后续双资源权限切片。 |
| 2026-07-08 | 日志清理系统任务入口与双资源 Authz 消费 | `web/default/src/features/system-settings/{api.ts,types.ts,maintenance/log-settings-section.tsx}` | 日志维护页历史日志清理改为创建 `log_cleanup` SystemTask，前端按钮和确认动作同时消费 `system_setting.sensitive_write` 与 `usage_log.sensitive_write`，成功后提示任务 ID 并刷新系统任务缓存。 |
| 2026-07-08 | 系统设置运维集成与模型部署保存按钮 Authz 消费 | `web/default/src/features/system-settings/integrations/*` | 监控告警、SMTP 邮件、Worker 代理和 io.net 模型部署保存按钮直接消费 `useUpdateOption()` 暴露的保存权限字段；io.net 连接测试保持独立操作语义，留待后续单独权限评审。 |
| 2026-07-09 | 支付配置保存与合规确认按钮 Authz 消费 | `web/default/src/features/system-settings/integrations/payment-settings-section.tsx`、`web/default/src/components/risk-acknowledgement-dialog.tsx` | 支付配置页的通用、Epay、Stripe、Creem、全部保存按钮以及支付合规确认入口消费 `system_setting.sensitive_write`；风险确认弹窗支持外部禁用原因，缺少权限时不触发支付合规确认 API。 |
| 2026-07-09 | Waffo 支付子表单保存按钮 Authz 消费 | `web/default/src/features/system-settings/integrations/{waffo-settings-section.tsx,waffo-pancake-settings-section.tsx}` | Waffo 聚合支付和 Waffo Pancake 托管支付最终保存按钮及保存 handler 消费 `system_setting.sensitive_write`；支付方式弹窗继续作为本地草稿编辑，不触发后端写入。 |
| 2026-07-09 | 控制台内容基础表单保存按钮 Authz 消费 | `web/default/src/features/system-settings/content/{dashboard-section.tsx,chat-settings-section.tsx,drawing-settings-section.tsx}` | Dashboard、Chat Presets 和 Drawing 三个基础内容设置保存按钮消费 `useUpdateOption()` 暴露的保存权限字段；列表编辑器页面保留给后续单独切片。 |
