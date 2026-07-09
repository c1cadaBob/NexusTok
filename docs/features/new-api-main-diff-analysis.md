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
| OpenAI Realtime / image edit / image stream 等 relay 文件 | `relay/channel/openai/relay_realtime.go`、`relay_image.go` | Realtime 和 image edit 已在现有文件中实现；image stream 与图片计费边界待增强 | 逐个核对上游协议支持，再按 channel 能力原生接入；本轮优先原生化 OpenAI image stream、`stream` 显式零值和 `n` 上限保护。 |
| Advanced Custom Channel | `relay/channel/advancedcustom/adaptor.go` | 缺少 | 可作为高级渠道改写能力参考，但必须接入 NexusTok 的参数覆盖、安全过滤和计费快照。 |
| Waffo Pancake SDK 绑定体验 | `waffo-pancake-sdk-go`、catalog/pair/save/product 接口 | NexusTok 有自研 Waffo Pancake 充值 | 吸收“自动验证凭据、拉取商店/商品、创建配对”的 UX，不强制替换当前支付链路。 |
| 订阅余额支付 | `SubscriptionRequestBalancePay`、`allow_balance_pay` | 后端与默认前端已落地 | 已新增 `/api/subscription/balance/pay`，在模型事务内完成钱包扣款、订阅创建和成功订单落账；默认前端已在套餐抽屉、套餐表和购买弹窗消费 `allow_balance_pay`。 |
| 订阅钱包溢出控制 | `allow_wallet_overflow` | 后端与默认前端已落地 | 已在套餐与用户订阅快照保存钱包溢出策略；`subscription_first` 下订阅额度不足时，任一活跃订阅禁止溢出即阻断钱包 fallback；默认前端套餐抽屉已提供钱包兜底开关。 |
| 用户权限回传 | `user.AdminPermissions = authz.Capabilities(...)` | 后端、默认前端入口消费与渠道页按钮级消费已落地 | `/api/user/self` 已返回 `permissions.admin_permissions`，默认前端已新增权限 helper/hook，并让管理侧边栏与渠道、账号池、模型、用户、订阅等入口路由按 read 能力判断；渠道管理页已按 `write`、`operate`、`sensitive_write`、`secret_view` 控制新建、编辑、测试、启停、批量操作、密钥查看、多 Key、渠道账号池和上游模型应用入口。当前矩阵来自系统角色基线，用户 override 和服务端 enforcement 待后续接入。 |

## 默认前端页面差异

### 两边共有页面

以下路由两边都存在，功能大体一致，但具体实现有差异：

| 路由 | 说明 | 主要差异 |
|------|------|----------|
| `/` | 首页 | 两边都有公开首页，NexusTok 品牌和样式独立。 |
| `/setup` | 初始化向导 | new-api-main 对 setup POST 增加匿名请求体限制。 |
| `/sign-in`、`/login` 等认证页 | 登录 | 两边都有；NexusTok 已补齐 `(auth)/register.tsx` 兼容路由文件。 |
| `/sign-up`、`/register` | 注册 | 两边都有；NexusTok 的 `/register` 已 replace 跳转 `/sign-up` 并保留查询参数。 |
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
| `(auth)/register.tsx` | `routes/(auth)/register.tsx` | 注册路由兼容文件。 | 已原生化：NexusTok 默认前端已新增 `/register` 到 `/sign-up` 的 replace 跳转并保留查询参数。 |

`/system-info` 已在 NexusTok 默认前端落地，因此不再作为 `new-api-main` 独有页面统计；当前差异转为同路径实现细节和后续任务类型接入差异。

### 默认前端功能模块差异

| 模块 | NexusTok 状态 | new-api-main 状态 | 原生化建议 |
|------|---------------|------------------|------------|
| Channels | 有账号池绑定、Codex OAuth 弹窗、渠道账号池管理 | 有移动端 `channel-card`、表单 section 拆分、权限上下文、Advanced Custom 编辑器 | 合并方向：保留 NexusTok 账号池能力，吸收 new-api-main 的表单拆分和移动端卡片。 |
| Dashboard | 有 overview/models/users 等面板 | 额外有 `dashboard/components/flow` 和 flow selection 测试 | 引入 `/api/data/flow` 后再接入流量 Sankey，不先做纯前端页面。 |
| Playground | 扁平组件结构，已有消息/输入/存储能力 | 更细拆成 `chat/`、`input/`、`message/`、`options/`、`streaming/`、`storage/` | 逐步重构，不改变接口协议；先迁移错误展示、消息编辑、stream utils。 |
| Usage Logs | 有通用/绘图/任务日志 | 额外有 `logs-filter-toolbar`、移动端卡片、schema | 先吸收移动端卡片和筛选工具条，再把账号池日志也纳入一致交互。 |
| Subscriptions | 有计划管理、reset dialog、余额支付、钱包溢出、Waffo Pancake 产品字段 | 具备同类能力 | 余额支付与钱包兜底已原生化；后续重点转为订阅重置交互 polish、权限细分和支付产品绑定体验。 |
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
| ClassicFrontendDeprecationBanner | 已接入 classic 全局布局 | 有 | 已原生化为可关闭退场提示；Root 用户可直接切换到默认前端，非 Root 用户看到联系管理员提示。 |
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
| NTLM 邮件 | 已落地 `github.com/Azure/go-ntlmssp`、`common/email_ntlm_auth.go` | `github.com/Azure/go-ntlmssp`、`common/email_ntlm_auth.go` | NexusTok 已新增 PLAIN/LOGIN/NTLM 自适应 SMTP 认证，企业 SMTP 仅开放 `AUTH NTLM` 时可完成协商。 |
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
| `(auth)/register.tsx` | `/register` 兼容注册页 | 已落地：默认前端新增兼容路由并保留 `aff` 等查询参数。 |

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

## 本轮实施评审：控制台内容列表编辑器保存与开关 Authz 消费

### 需求分析

系统设置的“控制台内容”列表编辑器页面已经具备 Announcements、API Addresses、FAQ 和 Uptime Kuma 四类可视化草稿编辑能力。这些页面的本地新增、编辑、删除和批量删除只修改 React state，只有最终 `Save Settings` 才通过 `useUpdateOption()` 写入 `PUT /api/option/`。此外，每个页面顶部的 `Enabled` 开关会立即写入对应的启用 option，实际也是系统设置敏感写入入口。

本轮目标：

1. 将 `/system-settings/content/announcements` 的最终保存按钮与 `console_setting.announcements_enabled` 开关接入 `system_setting.sensitive_write`。
2. 将 `/system-settings/content/api-info` 的最终保存按钮与 `console_setting.api_info_enabled` 开关接入同一权限。
3. 将 `/system-settings/content/faq` 的最终保存按钮与 `console_setting.faq_enabled` 开关接入同一权限。
4. 将 `/system-settings/content/uptime-kuma` 的最终保存按钮与 `console_setting.uptime_kuma_enabled` 开关接入同一权限。
5. 保持列表项新增、编辑、删除、批量删除、表单校验、删除确认、排序/展示、JSON 序列化格式和 option key 不变，避免把本地草稿整理误判为后端写操作。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Announcements 列表编辑器 | `web/default/src/features/system-settings/content/announcements-section.tsx` | 最终保存按钮和启用开关消费统一 option 保存权限；本地公告草稿编辑不变。 |
| API Addresses 列表编辑器 | `content/api-info-section.tsx` | 最终保存按钮和启用开关消费统一 option 保存权限；URL、路由、颜色等表单校验不变。 |
| FAQ 列表编辑器 | `content/faq-section.tsx` | 最终保存按钮和启用开关消费统一 option 保存权限；FAQ 草稿增删改不变。 |
| Uptime Kuma 列表编辑器 | `content/uptime-kuma-section.tsx` | 最终保存按钮和启用开关消费统一 option 保存权限；分组字段和监控项 JSON 序列化不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收方式，并补充能力落地清单。 |

### 风险评估

1. 四个页面的本地列表操作不直接调用后端，本轮不禁用这些草稿操作，避免无权限管理员失去查看、整理和临时编辑配置的能力；真正持久化由保存按钮与 hook/后端双层控制。
2. `Enabled` 开关是立即写入入口，若只处理最终保存按钮会留下可点击写路径；本轮在 handler 开头增加权限前置检查，并在 Switch 控件层禁用，确保无权限时不发起 `PUT /api/option/`。
3. 本轮不改变列表数据解析、`JSON.stringify` 输出、字段 schema、日期处理、颜色枚举、Uptime Kuma 分组结构和删除确认流程，因此不会影响现有控制台内容渲染。
4. `useUpdateOption()` 已经在提交前做权限兜底，本轮 UI 层只是提前表达同一权限语义；即使未来出现遗漏入口，后端路由权限表和 hook 仍会阻断无权限写入。

### 方案评审

采用“真实写入入口统一消费 `system_setting.sensitive_write`，本地草稿操作保持可用”的方案。四个页面继续复用 `useUpdateOption()`，最终保存按钮在缺少权限时禁用并展示统一 `disabledReason`，`handleSaveAll` 开头也执行同一权限前置检查，避免按钮外调用路径产生重复错误提示；`handleToggleEnabled` 开头先检查 `updateOption.canUpdate`，无权限时只显示 toast，不更新本地开关状态、不调用后端。Switch 控件增加禁用条件，防止用户在无权限或保存中的状态下触发即时写入。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 保存 Announcements、API Addresses、FAQ、Uptime Kuma 列表配置 | `system_setting.sensitive_write` | `PUT /api/option/` |
| 切换四个列表页面的 `Enabled` 开关 | `system_setting.sensitive_write` | `PUT /api/option/` |
| 本地新增、编辑、删除、批量删除、打开弹窗、字段校验 | 本地交互，无写权限要求 | 不调用后端写接口 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认四个列表编辑器类型通过。
2. `go test ./service/authz ./middleware ./router ./controller` 确认系统设置权限路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. `cd web/default && node scripts/sync-i18n.mjs` 确认未产生遗漏翻译 key。
5. 使用 MCP 访问 `http://192.168.0.202:3003/`，再打开 `/system-settings/content/announcements`、`/system-settings/content/api-info`、`/system-settings/content/faq`、`/system-settings/content/uptime-kuma`，确认页面加载、`/api/option/` 只读请求正常、保存按钮和开关渲染正常、控制台无错误；不触发真实保存或开关写入。

## 本轮实施评审：模型配置卡片保存按钮 Authz 消费

### 需求分析

系统设置的“模型与路由”分组中，Global Model Configuration、Gemini、Claude 和 Grok 四个配置卡片都会通过 `useUpdateOption()` 写入 `PUT /api/option/`。这些卡片已经具备表单校验、JSON 规范化、仅保存差异字段等原生行为，但最终 `Save Changes` 按钮仍只按 `updateOption.isPending` 禁用；缺少 `system_setting.sensitive_write` 的管理员会看到可提交入口，直到提交后才由 hook 或后端阻断。

本轮目标：

1. 将 `/system-settings/models/global` 的最终保存按钮和 `onSubmit` 前置检查接入 `updateOption.canUpdate/disabledReason`。
2. 将 `/system-settings/models/gemini` 的最终保存按钮和 `onSubmit` 前置检查接入同一权限。
3. 将 `/system-settings/models/claude` 的最终保存按钮和 `onSubmit` 前置检查接入同一权限。
4. 将 `/system-settings/models/grok` 的最终保存按钮和 `onSubmit` 前置检查接入同一权限。
5. 保持表单内 Switch、JSON 格式化、示例填充、字段校验、差异字段计算和 option key 不变，因为这些操作只修改本地表单状态，不直接调用后端写接口。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 全局模型配置 | `web/default/src/features/system-settings/models/global-settings-card.tsx` | 保存全局透传、thinking 黑名单、Chat Completions 到 Responses 策略和 keep-alive ping 设置时消费统一 option 保存权限。 |
| Gemini 配置 | `models/gemini-settings-card.tsx` | 保存 Gemini safety/version/imagine/thinking/function response 设置时消费统一 option 保存权限。 |
| Claude 配置 | `models/claude-settings-card.tsx` | 保存 Anthropic header、default max tokens 和 thinking adapter 设置时消费统一 option 保存权限。 |
| Grok 配置 | `models/grok-settings-card.tsx` | 保存 xAI 违规扣费开关和扣费金额时消费统一 option 保存权限。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收方式，并补充能力落地清单。 |

### 风险评估

1. 本轮不改模型定价、倍率、分组倍率、工具价格或表达式计费页面，因此不涉及 `pkg/billingexpr/expr.md` 所描述的计费表达式语义变更。
2. 四个卡片的表单 Switch 都是本地草稿字段，和内容列表页的即时写入开关不同；本轮不禁用本地 Switch，避免无权限管理员无法查看或整理草稿配置。
3. Global/Gemini/Claude 的 JSON 字段存在规范化和差异比较逻辑，本轮只在提交入口前加权限判断，不改变 JSON parse/stringify、默认值 fallback、schema 校验或差异字段计算。
4. Grok 违规扣费会影响计费结算行为，但本轮只提前表达后端已有 `system_setting.sensitive_write` 保存权限，不改变扣费金额单位、开关语义或后端结算路径。

### 方案评审

采用“模型配置卡片最终保存入口统一消费 `system_setting.sensitive_write`”的方案。四个卡片继续复用 `useUpdateOption()`；`onSubmit` 开头先检查 `updateOption.canUpdate`，无权限时只显示统一无权限 toast，不进入差异字段计算后的 option 写入循环；最终保存按钮缺少权限时禁用并展示 `disabledReason`。表单内的 Switch、Format JSON、Fill example 等按钮继续作为本地草稿交互，不扩大权限禁用范围。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 保存 Global、Gemini、Claude、Grok 模型配置 option | `system_setting.sensitive_write` | `PUT /api/option/` |
| 本地表单 Switch、JSON 格式化、示例填充、字段编辑和校验 | 本地交互，无写权限要求 | 不调用后端写接口 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认四个模型设置卡片类型通过。
2. `go test ./service/authz ./middleware ./router ./controller` 确认系统设置权限路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. `cd web/default && node scripts/sync-i18n.mjs` 确认未产生遗漏翻译 key。
5. 使用 MCP 访问 `http://192.168.0.202:3003/`，再打开 `/system-settings/models/global`、`/system-settings/models/gemini`、`/system-settings/models/claude`、`/system-settings/models/grok`，确认页面加载、`/api/option/` 只读请求正常、保存按钮渲染正常、控制台无错误；不触发真实保存。

## 本轮实施评审：io.net 连接测试按钮 Authz 消费

### 需求分析

系统设置的“模型与路由 → 模型部署”页面复用了 io.net 部署配置组件。保存 `model_deployment.ionet.enabled` 与 `model_deployment.ionet.api_key` 已经消费 `system_setting.sensitive_write`，但 `Test Connection` 按钮会调用 `/api/deployments/settings/test-connection`，该后端路由属于模型部署诊断能力，并在路由权限表中要求 `model.operate`。当前按钮只按 `testState.loading` 和 `updateOption.isPending` 禁用，缺少模型操作权限的管理员会看到可点击的测试入口，直到提交后才被后端阻断。

本轮目标：

1. 在 `IoNetDeploymentSettingsSection` 中引入模型权限矩阵，单独读取 `model.operate`。
2. `handleTestConnection` 开头增加权限前置检查，缺少 `model.operate` 时只显示无权限 toast，不调用后端测试接口。
3. `Test Connection` 按钮缺少 `model.operate` 时禁用并展示统一无权限原因。
4. 保持 io.net 设置保存按钮继续由 `system_setting.sensitive_write` 控制，不把模型操作权限错误套到系统 option 保存上。
5. 不改 api key 草稿、启用开关、保存 payload、测试请求体或后端部署接口。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| io.net 模型部署设置 | `web/default/src/features/system-settings/integrations/ionet-deployment-settings-section.tsx` | `Test Connection` 按钮和 handler 消费 `model.operate`；保存按钮继续消费 `system_setting.sensitive_write`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录跨资源权限需求、风险、方案和验收方式，并补充能力落地清单。 |

### 风险评估

1. 连接测试不写系统 option，但会携带当前草稿中的 api key 调用后端诊断接口；按后端 `model.operate` 权限提前禁用，可以避免无权限用户探测部署凭据和上游连接状态。
2. 保存 io.net 配置仍是 `PUT /api/option/`，本轮不改变其 `system_setting.sensitive_write` 语义，避免把“保存设置”和“模型部署操作”混为一个权限。
3. 本轮不改变 `testDeploymentConnectionWithKey` 的请求路径、payload、错误处理和成功/失败展示，只在 UI 和 handler 前置权限判断。
4. 如果当前环境未启用 io.net deployments，测试按钮不会出现在页面上；验收时仍需要验证页面加载和只读 option 请求，且不通过本地切换触发任何真实测试请求。

### 方案评审

采用“跨资源按钮按后端路由权限表消费”的方案：组件同时使用 `useUpdateOption()` 和 `useModelPermissions()`。保存路径仍由 `updateOption.canUpdate` 控制；连接测试路径由 `modelPermissions.canOperate` 控制。按钮层禁用负责提前表达权限，handler 前置检查负责防止脚本调用或未来按钮遗漏路径绕过 UI。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 保存 io.net enabled/api key option | `system_setting.sensitive_write` | `PUT /api/option/` |
| 测试 io.net 部署连接 | `model.operate` | `POST /api/deployments/settings/test-connection` |
| 本地启用开关、api key 草稿编辑、打开 io.net API Keys 链接 | 本地/外链交互，无后端写权限要求 | 不调用系统 option 写接口 |

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认跨资源 hook 引入后类型通过。
2. `go test ./service/authz ./middleware ./router ./controller` 确认模型与系统设置权限路由仍可构建。
3. `git diff --check` 确认无空白错误。
4. `cd web/default && node scripts/sync-i18n.mjs` 确认未产生遗漏翻译 key。
5. 使用 MCP 访问 `http://192.168.0.202:3003/` 和 `/system-settings/models/model-deployment`，确认页面加载、`/api/option/` 只读请求正常、保存按钮和连接测试区域渲染正常、控制台无错误；不点击 `Test Connection`，不触发真实测试请求或保存。

## 本轮实施评审：Waffo Pancake 配置原子保存后端原生化

### 需求分析

`new-api-main` 的 Waffo Pancake 管理端已经从“手工填写 Store/Product ID 后逐项保存”演进为配置向导：管理员可以用 Merchant ID 与 Private Key 拉取 Pancake catalog、选择已有 Store/Product、一键创建 Store + OnetimeProduct，并在最终保存时通过 `/api/option/waffo-pancake/save` 一次性提交绑定关系。NexusTok 当前已经具备 Waffo Pancake 用户侧充值、金额试算、结账会话创建、Webhook 验签与入账能力，也有默认前端的 Waffo Pancake 设置表单；但保存仍通过多次 `PUT /api/option/` 循环写入，存在部分 option 已写入、后续 option 失败导致支付配置短暂不一致的风险。

本轮目标选择低风险切片，先把 `new-api-main` 中“最终保存原子化”的优势转成 NexusTok 原生能力：

1. 在模型层补齐跨 SQLite、MySQL、PostgreSQL 的 `UpdateOptionsBulk`，用 GORM transaction 统一保存多个 option，数据库全部成功后再更新内存 `OptionMap`。
2. 在 Waffo Pancake service/controller 中新增 `SaveWaffoPancakeConfig` 与 `POST /api/option/waffo-pancake/save`，让支付网关绑定字段一次提交。
3. 让默认前端 Waffo Pancake 设置页优先调用原子保存接口，保留现有字段校验、密钥留空表示保留旧值、webhook key 留空表示保留旧值、开关/货币/单价/最低充值等原生配置能力。
4. 暂不引入 `waffo-pancake-sdk-go`，暂不实现 catalog、pair、subscription-product 和 subscription-product-options，避免把外部 Pancake 资源创建、订阅计划字段、SDK 版本升级与支付保存一致性混成一个高风险提交。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 系统选项模型 | `model/option.go` | 新增批量 option 保存事务；数据库成功后再逐项刷新内存配置，失败时不污染 `OptionMap`。 |
| Waffo Pancake service | `service/waffo_pancake.go` | 新增保存配置的领域函数，校验 Merchant/Store/Product 必填，空私钥或空 webhook key 保持已有密钥。 |
| Waffo Pancake controller | `controller/topup_waffo_pancake.go` | 新增管理端保存接口请求结构、错误处理和返回数据；用户充值与 webhook 入口不变。 |
| 系统设置路由 | `router/system-setting-router.go` | 将 `POST /api/option/waffo-pancake/save` 纳入现有 RootAuth + `system_setting.sensitive_write` 权限表。 |
| 默认前端支付设置 | `web/default/src/features/system-settings/integrations/waffo-pancake-settings-section.tsx` 及可能新增 API helper | 保存按钮改为调用原子保存接口；按钮权限仍消费 `useUpdateOption()` 暴露的系统设置敏感写权限。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收结果，后续 catalog/一键创建继续在同一报告追踪。 |

### 风险评估

1. 支付配置属于高敏感配置，若 Merchant ID、Store ID、Product ID、Private Key 或 Webhook 公钥分批保存失败，可能导致用户侧支付发起失败或 webhook 验签失败；原子保存可以降低配置不一致风险。
2. `model.UpdateOptionsBulk` 会改变同一批 option 的写入方式，必须只使用 GORM transaction、`FirstOrCreate`、`Save` 等三库兼容 API，不写数据库专用 SQL。
3. 数据库事务成功后再更新内存 `OptionMap`，可避免数据库失败但内存已更新；但如果内存更新阶段某个 option 解析失败，数据库已提交。本轮只写现有已支持的 Waffo Pancake option key，并保留逐项 `updateOptionMap` 错误返回，风险可控。
4. 前端仍需要允许空私钥、空生产 webhook 公钥和空测试 webhook 公钥表示“保留已保存密钥”，不能因原子接口而把这些密钥强制清空；新增 service 必须只在输入非空时写入密钥类 option。
5. catalog、pair 和 subscription product 依赖 Waffo Pancake SDK 和外部商户资源创建，失败会产生外部资源残留；本轮暂不做，避免影响现有自研 HTTP/RSA checkout 实现和已上线支付链路。
6. 新接口必须挂载在现有系统设置权限表中，权限语义与通用 `PUT /api/option/` 一致，仍要求 RootAuth 保底和 `system_setting.sensitive_write` 二次授权。

### 方案评审

采用“先原子保存，再分阶段接入 catalog/创建向导”的方案。后端新增 `model.UpdateOptionsBulk(values map[string]string)`：当 `values` 为空直接返回；非空时在 `DB.Transaction` 中逐项 `FirstOrCreate` option 并 `Save` value；事务完成后再逐项调用现有 `updateOptionMap`，复用全部已有全局变量同步逻辑。这样不会引入新的表结构、迁移或数据库方言分支。

Waffo Pancake service 新增 `SaveWaffoPancakeConfig`，接收现有前端完整表单字段。必填校验只在支付绑定核心字段上执行：启用状态下前端已校验 Merchant/Store/Product 和当前环境 webhook key；后端保存函数则保证 Merchant/Store/Product 不能为空，防止管理端直接调用保存出不可用绑定。`WaffoPancakePrivateKey`、`WaffoPancakeWebhookPublicKey`、`WaffoPancakeWebhookTestKey` 仅在输入非空时写入，延续“密钥不回显，留空保留”的原生安全体验。

路由新增到 `optionPermissionRoutes`，路径为 `POST /waffo-pancake/save`，权限为 `system_setting.sensitive_write`。前端新增或内联 API helper 调用该路径；保存按钮继续用 `useUpdateOption()` 的 `canUpdate/disabledReason` 表达权限，因为它和新接口共享同一后端权限。

后续分阶段计划：

| 阶段 | 能力 | 前置条件 |
|------|------|----------|
| 阶段 1 | 原子保存 Waffo Pancake 配置 | 本轮完成，不引入外部 SDK。 |
| 阶段 2 | catalog 凭证验证与 Store/Product 选择 | 评审 `waffo-pancake-sdk-go` 与 NexusTok 当前自研 auth service 的兼容性。 |
| 阶段 3 | 一键创建 Store + OnetimeProduct | 明确外部资源创建失败的 orphan store 展示与重试策略。 |
| 阶段 4 | 订阅计划 Pancake product 支持 | 先补 `SubscriptionPlan.WaffoPancakeProductId`、迁移、支付入口和 webhook 订单解析，再接 UI。 |

验收方式：

1. `go test ./service/authz ./middleware ./router ./controller ./model` 确认系统设置路由、controller 和 option 模型构建通过。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认 Waffo Pancake 设置页类型通过。
3. `cd web/default && node scripts/sync-i18n.mjs` 确认新增前端文案没有遗漏翻译 key。
4. `git diff --check` 确认无空白错误。
5. 使用 MCP 访问 `http://192.168.0.202:3003/` 和 `/system-settings/billing/payment`，确认页面更新生效、Waffo Pancake 设置区渲染正常、`/api/option/` 只读请求正常、控制台无错误；不填写真实 Waffo 凭证、不触发真实支付或外部 Pancake 创建。

## 本轮实施评审：Waffo Pancake catalog 与一键创建 Store/Product 原生化

### 需求分析

`new-api-main` 的 Waffo Pancake 设置页已经具备管理端配置向导能力：管理员输入 Merchant ID 和 Private Key 后，可以拉取 Pancake catalog、选择已有 Store + OnetimeProduct，或者一键创建默认 Store + OnetimeProduct，再将选中的绑定一次性保存。NexusTok 上一轮已经补齐了原子保存接口，但默认前端仍只能手填 Store ID 与 Product ID；这会让管理员必须离开 NexusTok 去 Pancake 后台复制 ID，容易填错，也无法在 NexusTok 内确认凭证是否有效。

本轮目标是在不替换现有充值 checkout/webhook 运行时链路的前提下，把 `new-api-main` 的管理端优势转成 NexusTok 原生能力：

1. 引入 `github.com/waffo-com/waffo-pancake-sdk-go`，仅用于管理端 catalog 查询和 Store/Product 创建，不触碰现有自研 HTTP/RSA checkout 与 webhook 验签。
2. 新增 Waffo Pancake catalog 查询能力，支持用前端临时输入的 Merchant ID/Private Key 做凭证验证并返回 Store + active OnetimeProduct 列表。
3. 新增一键创建默认 Store + wallet top-up OnetimeProduct 的能力，成功后回填 Store ID 与 Product ID；如果 Store 创建成功但 Product 创建失败，需要返回 orphan store 信息，便于管理员重试或手工接管。
4. 默认前端 Waffo Pancake 设置区增加“验证并拉取 catalog”“使用已存在 Store/Product”“一键创建 Store/Product”的原生工作流，同时保留手填 ID 兜底。
5. 订阅计划级 Pancake product、订阅支付入口和 webhook 订阅订单分流仍放到后续阶段，避免把外部商品创建与订阅 schema 迁移混在一个提交里。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Go 依赖 | `go.mod`、`go.sum` | 新增 Waffo Pancake SDK，仅被管理端辅助函数使用。 |
| Waffo Pancake service | `service/waffo_pancake.go` | 新增 SDK client、catalog 查询、默认 Store 创建、默认 OnetimeProduct 创建、pair 创建和 catalog 响应 DTO。 |
| Waffo Pancake controller | `controller/topup_waffo_pancake.go` | 新增 catalog 与 pair 管理端接口；临时凭证优先，空凭证回退已保存配置。 |
| 系统设置路由 | `router/system-setting-router.go` | 新增 `/api/option/waffo-pancake/catalog` 与 `/api/option/waffo-pancake/pair`，继续 RootAuth 保底并接入 Authz 权限表。 |
| 默认前端支付设置 | `web/default/src/features/system-settings/{api.ts,types.ts,integrations/waffo-pancake-settings-section.tsx}` | 增加 catalog/pair API 调用、选择控件、状态提示和一键创建按钮；保存仍走上轮原子保存接口。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增 UI 文案翻译并运行同步脚本。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮评审、能力落地与后续订阅阶段边界。 |

### 风险评估

1. `catalog` 使用临时 private key 验证凭证，如果沿用 new-api-main 的 GET query 形式会把密钥暴露在 URL、代理日志和浏览器历史中；本轮前端使用 `POST /api/option/waffo-pancake/catalog` 请求体传参，后端可保留 GET 只读已保存凭证目录作为兼容入口。
2. `pair` 会在外部 Pancake 账户创建 Store 和 OnetimeProduct，虽然不写本地数据库，也会产生外部资源副作用；因此权限不按普通 read，而按 `system_setting.sensitive_write` 处理，并在前端缺少保存权限时禁用。
3. 引入 SDK 不能影响当前生产充值流程。现有 `CreateWaffoPancakeCheckoutSession`、`VerifyConfiguredWaffoPancakeWebhook`、`ResolveWaffoPancakeTradeNo` 不改调用链；SDK 只用于新增管理端函数。
4. Store 创建成功但 Product 创建失败会产生 orphan store。后端必须返回 `orphan_store/store_id/store_name`，前端展示可理解的失败提示并回填 Store ID，避免管理员失去已创建资源线索。
5. Catalog 返回可能为空、Store 可能没有 active OnetimeProduct、或者已保存 Product 已被上游删除。前端需要保留手填 ID 和已保存 raw ID 兜底，不强制清空当前配置。
6. 新增文案必须进入六语 locale，避免默认前端构建后出现缺失翻译 key。

### 方案评审

采用“管理端 SDK 辅助、运行时链路隔离”的方案。后端新增 `newWaffoPancakeAdminClientFromCreds`，只由 catalog/pair 函数使用。`ListWaffoPancakeCatalog` 通过 SDK GraphQL 查询 `stores(limit: 100)` 及其 `onetimeProducts`，并过滤非 active 产品；pair 创建沿用 new-api-main 的默认命名思路，创建 `nexustok-store` 与 `nexustok-charge-product`，产品价格使用 1.00 USD + `saas` tax category，实际充值仍由现有 checkout 的 `PriceSnapshot` 覆盖。

路由权限：

| 路由 | 方法 | 权限 | 原因 |
|------|------|------|------|
| `/api/option/waffo-pancake/catalog` | `POST` | `system_setting.operate` | 使用临时凭证探测外部 Pancake catalog，不写本地配置。 |
| `/api/option/waffo-pancake/catalog` | `GET` | `system_setting.read` | 仅使用已保存凭证读取 catalog，便于回访页面。 |
| `/api/option/waffo-pancake/pair` | `POST` | `system_setting.sensitive_write` | 会创建外部 Store/Product，属于支付配置强副作用动作。 |

前端在现有 Waffo Pancake 表单内增加辅助区：`Refresh catalog` 用当前表单里的 Merchant ID/Private Key 触发 POST catalog；catalog 成功后展示 Store 和 Product 下拉，选择时回填原有 `WaffoPancakeStoreID/ProductID` 表单字段；`Create store + product` 调用 pair，成功后回填并刷新 catalog。保存按钮仍调用 `saveWaffoPancakeConfig`，因此最终落库保持原子性。

验收方式：

1. `go test ./service ./service/authz ./middleware ./router ./controller ./model` 或聚焦相关包，确认 SDK 引入和新增路由构建通过。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认前端类型通过。
3. `cd web/default && node scripts/sync-i18n.mjs` 并检查报告，确认新增文案进入六语 locale。
4. `git diff --check` 确认无空白错误。
5. 使用 MCP 访问 `http://192.168.0.202:3003/system-settings/billing/payment`，确认 Waffo Pancake 辅助区渲染正常、无控制台错误；用空凭证或无效凭证调用 catalog，确认接口返回业务失败且不保存配置；不点击一键创建真实外部资源。

## 本轮实施评审：Waffo Pancake 订阅套餐产品绑定原生化

### 需求分析

`new-api-main` 在订阅套餐管理里已经把 Waffo Pancake 作为计划级支付商品来源：每个 `SubscriptionPlan` 可保存独立的 `waffo_pancake_product_id`，管理端可以从已保存 Store 下拉选择 active OnetimeProduct，也可以根据套餐标题和价格一键创建并发布一个新的 OnetimeProduct。NexusTok 当前已经具备订阅计划、余额购买、Stripe/Creem/EPay 订阅支付、Waffo Pancake 钱包充值、Pancake catalog 和默认 Store/Product 一键创建能力，但订阅套餐仍缺少 Pancake product 绑定字段，管理员无法在套餐层沉淀 Pancake 商品关系。

本轮目标是把 `new-api-main` 的计划级产品绑定优势转成 NexusTok 原生管理能力，但不立即开放用户侧 Waffo Pancake 订阅支付：

1. 在 `SubscriptionPlan` 中新增 `waffo_pancake_product_id`，与已有 `stripe_price_id`、`creem_product_id` 保持同级语义。
2. 后端管理 API 支持创建、更新、查询套餐时保留该字段，并补齐 SQLite 手写建表与补列逻辑，确保 SQLite、MySQL、PostgreSQL 均可迁移。
3. 新增管理端辅助接口：用已保存的 Pancake Merchant/PrivateKey/Store 创建订阅套餐专属 OnetimeProduct，并列出已保存 Store 下的 active OnetimeProducts 供套餐表单选择。
4. 默认前端订阅套餐抽屉支持选择/手填 Waffo Pancake Product ID，并提供“按当前套餐标题和价格创建 product”的按钮。
5. 用户侧 `POST /api/subscription/waffo-pancake/pay`、checkout `OrderMerchantExternalID`、buyer identity、webhook 前缀分流、金额/币种校验和订阅订单完成逻辑单独评审，不在本轮混入。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 订阅模型 | `model/subscription.go` | 新增 `WaffoPancakeProductId` 字段，缓存序列化会自然携带该字段。 |
| 数据库迁移 | `model/main.go` | SQLite 建表和 required columns 增加 `waffo_pancake_product_id`；MySQL/PostgreSQL 继续由 GORM AutoMigrate 处理。 |
| 订阅 Admin API | `controller/subscription.go` | 创建和更新套餐时 trim 并保存 Waffo Pancake product ID，更新 map 必须包含空字符串以支持清空绑定。 |
| Pancake service | `service/waffo_pancake.go` | 新增 plan 专属 OnetimeProduct 创建函数，复用现有管理端 SDK client，不触碰 checkout/webhook 运行时链路。 |
| Pancake controller | `controller/topup_waffo_pancake.go` | 新增 subscription-product 创建接口和 subscription-product-options 只读接口。 |
| 系统设置路由 | `router/system-setting-router.go`、`router/system_setting_router_test.go` | 新增两条 `/api/option/waffo-pancake/*` 管理辅助路由并补权限表测试。 |
| 默认前端订阅管理 | `web/default/src/features/subscriptions/*` | 类型、表单默认值、payload、API helper、套餐抽屉和列表支付渠道标记接入 Pancake product 字段。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增订阅 Pancake product 相关按钮、提示和错误文案。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案、验收和落地结果。 |

### 风险评估

1. 最主要风险是把“订阅套餐绑定 Pancake product”误解成“用户已经可以用 Pancake 购买订阅”。本轮只沉淀管理字段和商品辅助创建，购买弹窗不会新增 Pancake 按钮，webhook 不会新增订阅分流，避免支付成功后权益无法正确发放。
2. `waffo_pancake_product_id` 是数据库新增列，SQLite 不能使用 `ALTER COLUMN`；必须沿用当前 `ensureSubscriptionPlanTableSQLite` 的 `ALTER TABLE ... ADD COLUMN` 补列模式。MySQL/PostgreSQL 不写方言 SQL，依赖 GORM 结构体迁移。
3. Product 创建会在外部 Waffo Pancake 账号产生真实 OnetimeProduct，属于强副作用动作；接口需要继续挂在 RootAuth 后的 `system_setting.sensitive_write`，前端缺少系统设置敏感写权限时禁用按钮。
4. Product options 查询使用已保存 Merchant/PrivateKey/Store 访问外部服务，虽然不写本地数据，但会探测外部资源；可按 `system_setting.read` 处理，和已保存凭证 catalog GET 一致。若后续要求把外部访问统一归入 operate，可单独调整权限表。
5. 创建 product 使用套餐当前表单标题和价格，并固定创建 OnetimeProduct 而非 Pancake 自动续费 SubscriptionProduct。原因是 NexusTok 当前没有外部续费、取消、past_due、退款撤销权益等生命周期模型，使用自动续费会导致上游扣费但本地权益不续期。
6. 前端 catalog 可能为空，已保存 product 也可能不在当前 Store 的 active 列表里；表单必须保留手填/原始 ID 兜底，不因 options 缺失清空管理员已有绑定。
7. 新增前端文案必须补齐六语 locale 并运行同步脚本；当前环境 `bun` 不可用时使用 `node scripts/sync-i18n.mjs` 作为等价同步方式。

### 方案评审

采用“先字段与管理辅助，后支付闭环”的方案。后端为 `SubscriptionPlan` 增加 `WaffoPancakeProductId string`，创建和更新套餐统一调用 trim，更新 map 显式写入 `waffo_pancake_product_id`，让管理员可以清空绑定。SQLite 建表 SQL 与 required columns 都补列，保持三库兼容。

Pancake 管理端继续复用上一轮接入的 `waffo-pancake-sdk-go`，新增 `CreateWaffoPancakeProductForPlan(ctx, merchantID, privateKey, storeID, name, amount, returnURL)`。它只创建并发布 `OnetimeProduct`，价格使用套餐金额的两位小数字符串和 USD，`SuccessURL` 复用已保存 ReturnURL。该函数不改 `CreateWaffoPancakeCheckoutSession`、`VerifyConfiguredWaffoPancakeWebhook`、`ResolveWaffoPancakeTradeNo`，运行时充值链路完全隔离。

新增路由：

| 路由 | 方法 | 权限 | 说明 |
|------|------|------|------|
| `/api/option/waffo-pancake/subscription-product-options` | `GET` | `system_setting.read` | 使用已保存凭证和 Store 读取 active OnetimeProducts，用于订阅套餐表单下拉。 |
| `/api/option/waffo-pancake/subscription-product` | `POST` | `system_setting.sensitive_write` | 根据表单标题和价格创建外部 OnetimeProduct，返回 product ID 给管理员确认后保存套餐。 |

默认前端在订阅套餐抽屉第三方支付配置区新增 Waffo Pancake 字段。打开抽屉时 best-effort 拉取 product options；下拉为空或请求失败时不阻断表单，仍允许手填 ID。创建按钮只在标题非空、价格大于 0 且用户具备 `system_setting.sensitive_write` 时启用；保存套餐仍由订阅写权限控制，外部建品和本地保存两步分离，避免管理员误以为创建外部 product 已自动保存套餐。

验收方式：

1. `go test ./model ./service ./service/authz ./middleware ./router ./controller` 确认模型迁移、Pancake service、controller 和路由权限表构建通过。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认默认前端类型通过。
3. `cd web/default && node scripts/sync-i18n.mjs` 确认新增文案六语同步。
4. `git diff --check` 确认无空白错误。
5. 使用 MCP 打开 `http://192.168.0.202:3003/subscriptions`，打开创建或编辑套餐抽屉，确认 Waffo Pancake Product ID 下拉、手填输入和创建按钮渲染正常，控制台无错误。
6. 使用 MCP 在浏览器上下文调用 `GET /api/option/waffo-pancake/subscription-product-options`；在未配置真实凭证时应返回业务失败且不写本地配置。不点击真实创建 product 按钮，避免产生外部资源。

## 本轮实施评审：Waffo Pancake 订阅支付闭环原生化

### 需求分析

上一轮已把 `new-api-main` 的 Waffo Pancake 订阅套餐 product 绑定转成 NexusTok 原生管理能力：套餐可保存 `waffo_pancake_product_id`，管理员可选择或创建 Pancake OnetimeProduct。但用户侧购买弹窗仍不会展示 Pancake，后端也没有 `/api/subscription/waffo-pancake/pay` 和 webhook 订阅订单分流。这样管理员虽然能绑定 product，用户仍无法通过 Pancake 购买订阅。

`new-api-main` 的成熟做法不是只新增一个 pay 接口，而是把 Pancake checkout 和 webhook 映射都收紧为“本地业务单号驱动”：

1. 创建 checkout 时传入稳定的 `OrderMerchantExternalID = trade_no`，让 webhook 可直接拿本地订单号完成 `SubscriptionOrder` 或 `TopUp`。
2. 创建 checkout 时传入稳定的 buyer identity，webhook 回来后用 `MerchantProvidedBuyerIdentity` 校验订单所属用户，避免只靠邮箱或上游 `orderId` 串单。
3. webhook 按本地 trade_no 前缀区分充值和订阅：`WAFFO_PANCAKE_SUB-` 走订阅完成，`WAFFO_PANCAKE-` 走钱包充值。
4. 使用 Pancake `OnetimeProduct` 购买 NexusTok 内部订阅，而不是 Pancake 自动续费 product，避免上游续费但本地权益没有续期。

NexusTok 当前 Waffo Pancake 充值 checkout 仍走自研 auth-service HTTP/RSA 请求，payload 只有 `buyerEmail` 和上游 `orderId`，没有 `OrderMerchantExternalID` 与 buyer identity。若直接新增订阅 pay 接口，会导致付款成功后 webhook 映射不可靠。因此本轮目标是一次性把 Waffo Pancake 运行时 checkout 迁到官方 SDK 的 Authenticated checkout，并同时保持充值和订阅两条路径兼容。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Pancake service | `service/waffo_pancake.go` | `CreateWaffoPancakeCheckoutSession` 改用 SDK Authenticated checkout；请求/响应 DTO 增加 `BuyerIdentity`、`OrderMerchantExternalID`、`Token`、`TokenExpiresAt`；webhook data 增加 `orderMerchantExternalId` 和 `merchantProvidedBuyerIdentity`；新增充值/订阅 trade_no 解析校验。 |
| Pancake 充值 controller | `controller/topup_waffo_pancake.go` | 充值 checkout 也传入本地 trade_no 与 buyer identity；webhook 按 `WAFFO_PANCAKE_SUB-` 分流订阅，否则继续充值。 |
| 充值信息 controller | `controller/topup.go` | 在保留 `enable_waffo_pancake_topup` 钱包充值开关的同时，新增 `enable_waffo_pancake_subscription`，让前端能区分“可购买订阅”和“可充值钱包”。 |
| 订阅 Pancake controller | `controller/subscription_payment_waffo_pancake.go` | 新增用户侧订阅 pay 接口，创建 pending `SubscriptionOrder` 后拉起 Pancake checkout。 |
| 路由 | `router/api-router.go`、相关路由测试 | 新增 `POST /api/subscription/waffo-pancake/pay`，继续 `UserAuth + CriticalRateLimit`。 |
| Webhook 可用性 | `controller/payment_webhook_availability.go` | `isWaffoPancakeWebhookEnabled` 不再强依赖钱包充值全局 ProductID，允许“只卖订阅、不开放钱包充值”的配置。 |
| 默认前端购买弹窗 | `web/default/src/features/subscriptions/*` | 增加 Pancake 订阅支付 API 和购买按钮；仅在网关开启且套餐有 `waffo_pancake_product_id` 时展示。 |
| i18n | `web/default/src/i18n/locales/*.json` | 如新增可见文案需补齐六语翻译。 |
| 测试 | `service/waffo_pancake_test.go`、`controller/payment_webhook_availability_test.go`、`router/*test.go` | 覆盖 trade_no 外部单号映射、buyer identity mismatch、订阅订单分流和 webhook 可用性拆分。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、风险、方案和验收标准。 |

### 风险评估

1. 运行时 checkout 从自研 auth-service HTTP/RSA 切到 SDK Authenticated checkout，会改变充值拉起支付的底层请求方式。风险收益比仍可接受：SDK 已在上一轮 catalog/pair/product 创建中引入，且它支持私钥标准化、`OrderMerchantExternalID`、buyer identity 和 token；这是接入订阅分流的必要前置。
2. 充值 webhook 映射从 `event.Data.OrderID` 改为 `event.Data.OrderMerchantExternalID`，旧的未支付 pending 订单如果是在切换前创建的，webhook 可能仍只带上游 `orderId`。为降低兼容风险，解析函数可在没有外部单号时回退旧 `OrderID` 查找，但新路径必须优先使用外部单号并校验 buyer identity。
3. buyer identity mismatch 必须拒绝订单完成。否则攻击者可能利用可编辑邮箱或上游订单信息造成串单。对没有 identity 的旧订单回调，只在旧 `OrderID` fallback 路径兼容充值，不用于订阅订单。
4. webhook 订阅完成需要先按本地 trade_no 判断订单类型，不能让订阅订单落入 `RechargeWaffoPancake`，否则会错误增加钱包余额而非创建订阅权益。
5. 金额/币种/product/store 校验仍未完全覆盖。`CompleteSubscriptionOrder` 当前只按 trade_no 和 provider 完成订单。本轮会先建立稳定映射和 provider guard，金额/币种/product 校验可作为下一步加固项；但订阅 checkout 的 trade_no 是服务端创建并回传，已有 provider、订单状态和 buyer identity 多重约束。
6. 购买次数上限在创建订单和 webhook 完成时都可能检查。若用户并发创建多个 pending 订单并都付款，后到 webhook 可能因为购买上限失败，需要人工退款或补偿；这是现有 Stripe/Creem/EPay 订阅支付也存在的业务风险，本轮不扩大该风险面。
7. 前端新增 Pancake 按钮必须只在套餐绑定 product 且 Waffo Pancake 网关开启时展示；未绑定 product 的套餐不能误导用户点击。

### 方案评审

采用“SDK Authenticated checkout + 本地 trade_no 分流 + 前端灰度展示”的方案。

后端 service 层将 `WaffoPancakeCreateSessionParams` 扩展为同时支持充值和订阅所需字段：`ProductID`、`Currency`、`PriceSnapshot`、`BuyerEmail`、`SuccessURL`、`ExpiresInSeconds`、`BuyerIdentity`、`OrderMerchantExternalID`。创建会话时使用 `pancake.AuthenticatedCheckoutParams`，`OrderMerchantExternalID` 始终传本地 `trade_no`，`BuyerIdentity` 统一由 `WaffoPancakeBuyerIdentityFromUserID(userID)` 生成。

充值路径创建 `TopUp` 后调用同一个 checkout 函数，并把 trade_no 传入外部单号；订阅路径新增 `SubscriptionRequestWaffoPancakePay`，校验合规、套餐启用、`waffo_pancake_product_id`、凭证、webhook key、用户和购买上限，创建 pending `SubscriptionOrder` 后用套餐 product 创建 checkout。订阅 trade_no 前缀固定为 `WAFFO_PANCAKE_SUB-`。

webhook 验签继续保留 NexusTok 当前 RSA 验证逻辑，以避免一次性替换签名校验边界；只扩展 payload struct 解析新字段。验签成功后：

1. 只处理 `order.completed`；
2. 优先读取 `event.Data.OrderMerchantExternalID`；
3. 前缀为 `WAFFO_PANCAKE_SUB-` 时调用 `ResolveWaffoPancakeSubscriptionTradeNo` 与 `CompleteSubscriptionOrder`；
4. 其它订单调用 `ResolveWaffoPancakeTradeNo` 与 `RechargeWaffoPancake`；
5. 两个 Resolve 函数都校验 provider，且新外部单号路径校验 buyer identity。

前端 `GetTopUpInfo` 消费独立的 `enable_waffo_pancake_subscription` 能力标记，购买弹窗再结合套餐自身的 `waffo_pancake_product_id` 展示 Pancake 支付按钮；返回 `checkout_url` 后先校验只允许 `http/https`，再与 Creem 保持同样的新窗口打开体验。用户付款后的权益发放仍依赖 webhook，不在前端乐观刷新订阅。

验收方式：

1. `go test ./service ./model ./controller ./router ./middleware` 覆盖后端关键路径和路由。
2. `cd web/default && ./node_modules/.bin/tsc -b` 通过。
3. `cd web/default && node scripts/sync-i18n.mjs` 通过。
4. `git diff --check` 通过。
5. MCP 打开 `http://192.168.0.202:3003/wallet` 或订阅购买入口，确认 Pancake 按钮只在 `enable_waffo_pancake_subscription=true` 且套餐有 product 绑定时展示；管理端订阅表单仍正常。
6. MCP 在浏览器上下文调用 `POST /api/subscription/waffo-pancake/pay`，在未配置真实凭证或套餐无 product 时返回业务失败，不创建外部支付；不触发真实支付。

## 本轮实施评审：订阅额度手动重置原生化

### 需求分析

`new-api-main` 已提供管理员手动重置订阅额度的能力：可以按套餐批量重置所有有效订阅，也可以在某个用户的订阅列表中按套餐重置该用户的有效订阅。该能力解决的是运营补偿、人工售后、套餐规则调整后的额度恢复等场景。NexusTok 当前已经有周期性订阅维护任务，会自动处理到期和 `next_reset_time` 到达的重置，但缺少“管理员立即触发”的安全入口；管理员只能删除/失效/重新绑定订阅，容易改变有效期、分组快照或订单历史，操作粒度过粗。

本轮目标是把 `new-api-main` 的手动重置优势转成 NexusTok 原生能力：

1. 后端提供套餐级和用户级两个管理接口，只重置有效订阅的 `amount_used`，不改变订阅总额、有效期、来源、分组快照和钱包溢出策略。
2. 支持 `advance_reset_time` 开关：默认推进下一次周期重置时间，避免管理员刚手动补偿后又被周期任务立刻二次重置；关闭时仅清零已用额度，保留原 `last_reset_time/next_reset_time`。
3. 路由接入现有 `subscription.operate` 权限表，因为这是运营动作，不应按只读或普通写入处理，也不需要提升到删除级敏感写。
4. 前端订阅管理页增加按套餐重置入口，用户订阅弹窗增加按用户+套餐重置入口，并复用已有权限 hook 控制按钮可见性。
5. 记录用户管理日志和管理审计信息，保证人工重置可追溯。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 订阅模型 | `model/subscription.go`、新增/更新测试 | 新增 `SubscriptionResetResult`、用户级和套餐级重置事务；使用 GORM v2 行锁 helper，保持 SQLite/MySQL/PostgreSQL 兼容。 |
| 订阅 controller | `controller/subscription.go` | 新增 `AdminResetPlanSubscriptions` 与 `AdminResetUserSubscriptionsByPlan`，解析 `advance_reset_time`，记录用户日志和管理审计。 |
| 路由与权限 | `router/subscription-router.go`、路由测试 | 新增 `POST /api/subscription/admin/plans/:id/subscriptions/reset` 和 `POST /api/subscription/admin/users/:id/subscriptions/reset`，权限使用 `subscription.operate`。 |
| 默认前端 API | `web/default/src/features/subscriptions/api.ts`、`types.ts` | 新增重置请求/响应类型和两个 API 函数。 |
| 默认前端订阅页 | `web/default/src/features/subscriptions/components/*` | 套餐行操作增加“重置额度”按钮与确认弹窗；用户订阅弹窗在每个有效订阅计划旁提供用户级重置按钮。 |
| i18n | `web/default/src/i18n/locales/*.json` | 新增确认弹窗、按钮和结果提示文案的六语翻译。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮评审、风险和验收标准。 |

### 风险评估

1. 额度重置是高影响运营操作，误点会立即改变用户可用订阅额度。前端必须使用确认弹窗，并展示“是否推进下一次重置时间”的明确选项；后端必须只挂在 AdminAuth + `subscription.operate` 后面。
2. 并发风险主要来自周期维护任务、用户请求预消费和管理员手动重置同时写同一订阅。模型层应在事务中查询有效订阅并加行锁；SQLite 不支持 `FOR UPDATE`，现有 `lockForUpdate` helper 会自动跳过不兼容语法，保持三库兼容。
3. 套餐级重置可能影响多个用户。返回结果需要包含匹配订阅数和用户数，日志按受影响用户分别写入，便于后续审计。
4. `advance_reset_time=false` 适合临时补偿，但若 `next_reset_time` 已经过期，周期维护任务下一轮可能再次清零。前端默认启用 `advance_reset_time=true`，并在文案中强调该默认行为。
5. 有效订阅筛选必须使用 `status='active' AND end_time > now`。已到期但尚未被维护任务标记为 expired 的记录不应被手动重置，避免给事实上已过期的订阅续命。
6. 该能力只重置订阅额度，不退款、不撤销订单、不修改钱包余额，不影响 Waffo/Stripe/Creem/EPay 支付链路。

### 方案评审

采用“模型事务重置 + subscription.operate 路由 + 前端确认弹窗”的方案。

模型层新增 `resetUserSubscriptionTx` 作为唯一重置原语：将 `AmountUsed` 置零；当 `advanceResetTime=true` 时，用现有 `calcNextResetTime(time.Unix(now, 0), plan, sub.EndTime)` 重新计算 `NextResetTime`，有周期时同步 `LastResetTime=now`，无周期时清零两者。用户级入口 `AdminResetUserSubscriptionsByPlan(userId, planId, advanceResetTime)` 只重置该用户该套餐下仍有效的 active 订阅，找不到匹配项返回业务错误；套餐级入口 `AdminResetPlanSubscriptions(planId, advanceResetTime)` 重置该套餐下所有有效 active 订阅，零匹配时返回成功且计数为 0，便于管理员安全执行批量操作。

Controller 层复用 `common.DecodeJson`/`ShouldBindJSON` 的现有习惯不涉及手动 JSON marshal；响应使用 `common.ApiSuccess`。审计维度保持 NexusTok 现有模式：每个受影响用户写 `model.LogTypeManage` 用户日志，套餐级操作写系统管理审计，用户级操作写目标用户管理审计。

前端不直接复制 `new-api-main` 文件，而是在 NexusTok 当前订阅权限 hook 和现有表格结构上原生化：行操作菜单/按钮在具备 `subscription.operate` 时展示重置入口；确认弹窗使用已有确认组件或 shadcn Dialog/AlertDialog，默认勾选“推进下一次重置时间”，成功后刷新列表并 toast 展示重置数量。用户订阅弹窗内只对 active 且未到期的订阅展示用户级重置入口。

验收方式：

1. `go test ./model ./controller ./router ./middleware` 覆盖模型事务、controller/路由权限注册和订阅相关中间件影响面。
2. `cd web/default && ./node_modules/.bin/tsc -b` 通过。
3. `cd web/default && node scripts/sync-i18n.mjs` 通过，新增文案没有缺失翻译 key。
4. `git diff --check` 通过。
5. MCP 打开 `http://192.168.0.202:3003/subscriptions`，确认订阅管理页、重置入口和确认弹窗渲染正常，控制台无错误。
6. MCP 在浏览器上下文对新增接口做参数错误或空数据负例调用，确认返回业务响应，不误改真实订阅数据。

## 本轮实施评审：Token Limits 默认前端入口原生化

### 需求分析

`new-api-main` 默认前端在系统设置安全页提供 `Token Limits` 配置，用于管理 `token_setting.max_user_tokens`。NexusTok 后端已经具备同名原生能力：`setting/operation_setting/token_setting.go` 注册了 `token_setting.max_user_tokens`，`model/token.go` 的 Token 搜索保护和创建数量限制已经读取该值；classic 前端也已经能配置该字段。但默认前端目前只展示 Rate Limiting、Sensitive Words 和 SSRF Protection，导致 Root 用户无法在当前主前端中调整每用户最大 API Token 数，只能依赖默认值或 classic 页面。

本轮目标是将 `new-api-main` 的默认前端入口吸收为 NexusTok 原生安全设置页能力：

1. 在 `/system-settings/security` 下新增 `token-limits` section，展示并保存 `token_setting.max_user_tokens`。
2. 复用现有 `SettingsPage`、`SettingsSection`、`useUpdateOption` 和系统设置权限保护，不新增后端接口、不绕过 `system_setting.sensitive_write`。
3. 表单只允许正整数，默认值保持 1000；保存时仅在值变化时调用 `PUT /api/option/`。
4. 补齐默认前端六语翻译，并更新差异报告实施记录。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 默认前端安全设置 | `web/default/src/features/system-settings/security/*`、`request-limits/token-limit-section.tsx` | 新增 Token Limits 分区和表单入口，读写现有 `token_setting.max_user_tokens`。 |
| 默认前端类型 | `web/default/src/features/system-settings/types.ts` | `SecuritySettings` 增加 `token_setting.max_user_tokens` 类型声明。 |
| i18n | `web/default/src/i18n/locales/*.json` | 增加 Token Limits 页面标题、说明、字段和保存按钮文案。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮评审、风险和验收方式。 |

### 风险评估

1. 该字段会影响用户创建 API Token 的数量上限，也影响超量用户搜索 Token 时是否允许模糊查询。错误地设置过低可能阻止用户创建新 Token；错误地设置过高可能降低超量搜索保护效果。前端必须保留正整数校验和说明文案。
2. 本轮不修改 `setting/operation_setting/token_setting.go`、`model/token.go` 或创建 Token 的后端逻辑，避免触碰核心认证和密钥生成路径。
3. 保存仍走统一 `useUpdateOption()`，缺少 `system_setting.sensitive_write` 时前端不发请求，后端路由也已有 Root/Authz 边界。
4. 该设置不是公共 status 依赖，不需要刷新 `/api/status`；保存后刷新 `system-options` 即可。
5. 若线上配置缺失该 key，默认前端使用 1000，与后端默认配置一致。

### 方案评审

采用“仅默认前端补入口”的方案。新增 `TokenLimitSection` 参考 `new-api-main` 的字段语义，但使用 NexusTok 当前页面结构、版权头、`useUpdateOption` 权限封装和现有按钮保存模式。`SecuritySettings` 类型与默认值补上 `'token_setting.max_user_tokens': 1000`，`security/section-registry.tsx` 增加 `token-limits` 导航项，并让路由校验自然接受 `/system-settings/security/token-limits`。

表单保存时先比较当前值与默认值；没有变化时提示 `No changes to save`，有变化时调用 `updateOption.mutateAsync({ key: 'token_setting.max_user_tokens', value })`。不新增后端测试，因为业务逻辑已存在且本轮不改后端；验证重点放在 TypeScript、i18n 同步和 MCP 页面/接口真实调用。

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 通过。
2. `cd web/default && node scripts/sync-i18n.mjs` 通过，新增 `t()` 文案无缺失翻译。
3. `git diff --check` 通过。
4. MCP 打开 `http://192.168.0.202:3003/system-settings/security/token-limits`，确认页面渲染、字段默认值、无控制台错误。
5. MCP 在浏览器上下文调用 `PUT /api/option/` 的无效权限/校验或读取 `GET /api/option/`，确认现有 option key 可被当前前端识别；不随意修改线上真实上限值。

## 本轮实施评审：Routing Reliability 默认前端聚合入口原生化

### 需求分析

`new-api-main` 在模型设置中提供 `Routing Reliability` 分区，将请求重试次数、自动重试状态码、渠道自动禁用、渠道自动恢复、自动渠道测试和测试模式集中到一个面向模型路由运维的页面。NexusTok 后端已经具备这些 option 和运行逻辑，当前默认前端也能在 System Behavior、Monitoring & Alerts、Operations 等位置配置其中多数能力；但相关字段分散在不同设置页，管理员排查“模型路由为什么重试、为什么禁用、什么时候恢复”时需要跨页面跳转，认知成本偏高。

本轮目标是把 `new-api-main` 的聚合视图吸收为 NexusTok 原生能力：

1. 在 `/system-settings/models` 下新增 `routing-reliability` 分区，把模型路由可靠性相关 option 汇总展示。
2. 复用 NexusTok 现有后端 option、`useUpdateOption()`、状态码规则校验和 `system_setting.sensitive_write` 权限，不新增后端接口、不改变 Relay 热路径。
3. 保留旧入口，不迁移、不删除 System Behavior 和 Monitoring & Alerts 中已有字段，避免影响管理员现有操作路径。
4. 新增 `monitor_setting.channel_test_mode` 默认前端入口，支持 `scheduled_all` 与 `passive_recovery`，对齐后端监控任务模式语义。
5. 补齐默认前端六语翻译，并在差异报告实施记录中标注该能力已完成默认前端原生化。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 默认前端模型设置 | `web/default/src/features/system-settings/models/*` | 新增 Routing Reliability 分区、注册导航项，并把现有可靠性 option 接入模型设置默认值。 |
| 默认前端类型 | `web/default/src/features/system-settings/types.ts` | `ModelSettings` 增加重试、渠道自动禁用/恢复、自动测试与 `channel_test_mode` 字段声明。 |
| 默认前端表单校验 | `web/default/src/features/system-settings/models/routing-reliability-section.tsx` | 复用 `parseHttpStatusCodeRules`，校验 HTTP 状态码列表和范围；数值字段保持非负或正整数约束。 |
| i18n | `web/default/src/i18n/locales/*.json` | 新增 Routing Reliability 页面标题、说明、分组标题、测试模式和保存按钮文案。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审和验收方式。 |

### 风险评估

1. 同一个 option 会出现在多个设置入口，可能造成管理员认知重复。本轮保留旧入口是有意的兼容策略：新增模型路由聚合视图用于提升运维效率，不改变原有页面和用户习惯；后续若要收敛旧入口，需要单独做迁移评审和文案提示。
2. `RetryTimes`、自动重试状态码和渠道自动禁用规则会直接影响 Relay 可用性。错误配置可能导致请求过度重试、故障渠道被保留或正常渠道被禁用。前端必须保留 0-10 重试次数限制、状态码规则校验和自动规范化提示。
3. `AutomaticDisableKeywords` 可能根据上游错误内容禁用渠道，属于高影响运维规则。表单只做换行规范化，不改变后端匹配语义，避免引入隐藏行为差异。
4. 自动测试模式影响 SystemTask/监控任务的渠道检测范围。`scheduled_all` 适合定时检测所有非手动禁用渠道，`passive_recovery` 适合只恢复自动禁用渠道；本轮只提供配置入口，不修改后端 runner。
5. 保存仍走 `useUpdateOption()`，缺少 `system_setting.sensitive_write` 时按钮禁用且 hook 拦截提交；后端 `/api/option` 仍保留 Root/Authz 边界。
6. 该切片只改默认前端和文档，不涉及数据库迁移、计费、账号池、支付或 Relay 请求转换，核心业务风险较低。

### 方案评审

采用“新增聚合入口、复用现有 option、保留旧入口”的方案。新增 `RoutingReliabilitySection` 使用当前 NexusTok 设置页已有 `Form`、`SettingsSection`、`Input`、`Switch`、`Textarea`、`Select` 和 `Button` 组合，不引入 `new-api-main` 中尚未在本项目落地的 `SettingsForm`/浮动保存动作，避免一次性扩大 UI 基建变更。

表单 schema 包含：

1. `RetryTimes`：0-10 的整数重试次数。
2. `AutomaticRetryStatusCodes`：自动重试状态码，支持逗号和闭区间，保存前规范化。
3. `monitor_setting.auto_test_channel_enabled`、`monitor_setting.auto_test_channel_minutes`、`monitor_setting.channel_test_mode`：自动测试开关、测试间隔和测试模式。
4. `AutomaticEnableChannelEnabled`：成功检测后自动恢复渠道。
5. `AutomaticDisableChannelEnabled`、`ChannelDisableThreshold`、`AutomaticDisableStatusCodes`、`AutomaticDisableKeywords`：自动禁用规则。

保存时先把表单值规范化，与进入页面时的 baseline 对比；没有变化时只提示 `No changes to save`，不发 `PUT /api/option/`；有变化时逐项调用 `updateOption.mutateAsync`。状态码字段通过 `parseHttpStatusCodeRules` 校验和保存规范化结果，`channel_test_mode` 对缺失或未知值兜底为 `scheduled_all`。Base UI `Select` 使用根节点 `items` prop，并把 `SelectItem` 放在 `SelectGroup` 内，符合当前组件约束。

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 通过。
2. `cd web/default && node scripts/sync-i18n.mjs` 通过，新增 `t()` 文案无缺失翻译。
3. `git diff --check` 通过。
4. MCP 打开 `http://192.168.0.202:3003/`，确认热更新后的首页可访问且未被旧缓存误导。
5. MCP 打开 `http://192.168.0.202:3003/system-settings/models/routing-reliability`，确认导航项、表单字段、默认值、测试模式下拉和状态码规范化提示渲染正常，控制台无错误。
6. MCP 点击无改动保存，确认只提示 `No changes to save`，不误写线上真实配置；若页面未更新，先重启容器再复验。

## 本轮实施评审：Classic 前端退场提示原生化

### 需求分析

`new-api-main` 在 classic 前端全局布局中提供 `ClassicFrontendDeprecationBanner`，用于提醒用户旧版前端即将停止维护，并为 Root 用户提供一键切换到新版默认前端的入口。NexusTok 已经在 classic 的系统设置页提供“切换到新版前端”按钮，但入口隐藏较深，普通用户也无法明确知道 classic 体验可能缺少新功能；这与当前默认前端持续增强、classic 仅保留兼容入口的演进方向不一致。

本轮目标是把该优势转成 NexusTok 原生能力：

1. 在 classic 全局 `PageLayout` 中增加可关闭的维护提示，仅影响 classic 前端，不改变默认前端和后端业务链路。
2. Root 用户看到“切换到新版前端”按钮，复用现有 `PUT /api/option/` 写入 `theme.frontend=default` 的语义；非 Root 用户只看到联系管理员的提示。
3. 关闭提示只写入当前浏览器 `localStorage`，不修改服务端配置，不影响其他用户。
4. 文案补齐 classic 已支持的 `zh-CN`、`zh-TW`、`en`、`fr`、`ja`、`ru`、`vi` 语言包。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Classic 布局 | `web/classic/src/components/layout/PageLayout.jsx` | 在 header 与页面内容之间插入退场提示 banner。 |
| Classic 组件 | `web/classic/src/components/layout/ClassicFrontendDeprecationBanner.jsx` | 新增可关闭提示、Root 切换按钮和本地关闭确认。 |
| Classic helper | `web/classic/src/helpers/frontendTheme.js`、`web/classic/src/helpers/index.js` | 抽出切换到默认前端的确认逻辑，供 banner 复用；设置页旧逻辑本轮不强制迁移，避免扩大行为面。 |
| Classic 样式 | `web/classic/src/index.css` | 增加 banner 布局、内容换行和移动端收缩样式。 |
| Classic i18n | `web/classic/src/i18n/locales/*.json` | 补齐 banner 标题、正文、跳转成功文案和缓存提示。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审和验收方式。 |

### 风险评估

1. Classic 前端使用固定 header 与固定侧边栏，新增 banner 可能挤压内容或造成移动端重叠。样式应只在 banner 可见时占位，并在小屏下改为纵向布局，避免遮挡已有控制台内容。
2. 切换前端会写入全局 `theme.frontend`，属于 Root 级系统设置动作。前端只对 Root 用户展示按钮，后端 `/api/option/` 仍保留既有 Root/Authz 边界；非 Root 点击路径不存在。
3. 关闭提示是浏览器本地状态，如果用户误关闭，只影响当前浏览器；不写服务端 option，避免把“是否显示提示”变成全局配置。
4. Classic 语言包比默认前端多 `zh-CN`/`zh-TW`，不能只补默认前端六语；否则部分 classic 用户会看到源文案。
5. 本轮不删除 classic 入口、不改变 `theme.frontend` 默认值、不调整系统设置页原有按钮，避免引发不可逆迁移。

### 方案评审

采用“全局提示 + Root 显式切换 + 本地可关闭”的方案。新增 `ClassicFrontendDeprecationBanner` 使用 Semi UI `Banner`、`Button`、`Modal` 和现有 i18n，不引入新依赖；视觉上沿用 classic 现有 Semi warning 语义，保持密集后台界面的一致性。

切换逻辑抽到 `helpers/frontendTheme.js`：弹出确认框，说明切换后会刷新并进入新版前端；确认后调用 `API.put('/api/option/', { key: 'theme.frontend', value: 'default' })`，成功后跳转首页，失败时复用 `showError`。这个 helper 先供 banner 使用，设置页已有逻辑保持不动，后续如果需要可以再单独去重。

验收方式：

1. `cd web/classic && npm run build` 或可用的本地构建命令通过；若环境缺包管理器能力，则至少执行 Vite/ESBuild 可用检查并说明。
2. `git diff --check` 通过。
3. MCP 打开 `http://192.168.0.202:3003/`，确认热更新入口仍可访问。
4. MCP 打开 classic 控制台路径，例如 `http://192.168.0.202:3003/console`，确认 banner 渲染、Root 切换按钮存在、控制台内容未被遮挡、控制台无新增错误。
5. MCP 点击关闭按钮，确认出现关闭确认弹窗；取消关闭后 banner 仍存在，不写服务端配置、不切换前端。

## 本轮实施评审：OpenAI 图片流式与计费边界增强

### 需求分析

复核 `new-api-main` 与 NexusTok 的 OpenAI relay 差异后可以确认：NexusTok 已经在 `relay/channel/openai/adaptor.go` 和 `relay/channel/openai/relay-openai.go` 中具备 Realtime WebSocket 代理、图片编辑 multipart 重组和基础图片 usage 计费能力，并非完全缺少 Realtime/image edit 功能。但 `new-api-main` 仍有一组值得吸收的图片链路增强：OpenAI Images `stream` 字段使用指针保留显式 `false`，multipart 图片编辑可解析并继续复读请求体，`n` 增加上限避免超大值进入计费倍率，图片 usage 统一把 `input_tokens/output_tokens/input_tokens_details` 映射到 NexusTok 结算字段，并支持 `text/event-stream` 图片流式返回。

本轮目标是把这部分优势转换为 NexusTok 原生 OpenAI 图片能力：

1. `dto.ImageRequest` 新增 `Stream *bool`，满足上游 relay 请求 DTO “字段缺失省略、显式 false 仍保留”的项目规则。
2. 图片请求解析支持 JSON 和 multipart 两种入口的 `stream` 与 `n` 校验；multipart 解析必须保持请求体可复读，避免后续 `ConvertImageRequest` 和重试链路拿不到文件。
3. 为图片生成数量 `n` 设置 `dto.MaxImageN` 上限，拒绝负数、超大数和超过上限的值，避免计费倍率溢出或变成异常负计费。
4. 图片非流式响应在写回客户端前先识别 OpenAI error body；正常响应统一标准化 usage，避免 `input_tokens` 与 `prompt_tokens` 双重累加或丢失 image/text token 明细。
5. 图片流式响应支持真实 SSE 转发、JSON 响应包装为伪 SSE completed 事件、SSE error event 软错误记录和 `[DONE]` 终止事件，保留现有上游状态码、重试、计费和日志链路。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 图片 DTO | `dto/openai_image.go`、`dto/openai_request_zero_value_test.go` | 新增 `MaxImageN` 和 `Stream *bool`，补显式零值保留测试。 |
| 请求解析 | `relay/helper/valid_request.go`、`relay/helper/openai_image_request_test.go` | multipart 图片编辑改用可复读表单解析，解析/校验 `stream` 与 `n`；JSON 图片请求增加 `n` 上限校验。 |
| OpenAI 图片响应 | `relay/channel/openai/relay-openai.go`、`relay/channel/openai/image_stream_test.go` | 新增图片 usage 标准化、图片 SSE/JSON-as-SSE 响应处理和 error body 检测；DoResponse 按 `info.IsStream` 分发。 |
| 流状态观测 | `relay/helper/stream_scanner.go`、`relay/helper/stream_scanner_test.go` | 保留调用方预初始化的 `StreamStatus`，避免图片流式 error event 或上层诊断信息在扫描器启动时被覆盖。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收标准，同时修正文档中“缺少 Realtime/image edit”的过时表述。 |

### 风险评估

1. 图片编辑请求通常是 multipart/form-data，并且中继重试会多次读取 body；解析阶段如果直接消费 `c.Request.Body` 会导致后续上游请求没有文件。本轮必须使用 `common.ParseMultipartFormReusable` 和 `common.GetBodyStorage`，并把解析后的 form 回写到 `c.Request.MultipartForm/PostForm`。
2. `stream` 是可选标量，必须使用 `*bool`，否则客户端显式传 `false` 会被 `omitempty` 丢弃，违反项目 Rule 6；multipart 中 `stream=notabool` 应明确拒绝。
3. `n` 既影响 OpenAI 请求又影响 NexusTok `OtherRatio("n")` 计费倍率；负数、超大整数或超过上限的值不能进入计费阶段。默认值仍为 1，避免改变既有请求行为。
4. 图片 API 的 usage 字段不同于 Chat：OpenAI Images 常返回 `input_tokens/output_tokens/input_tokens_details`。本轮只在图片路径做覆盖式标准化，不复用到 Chat/Embedding，避免其它响应因字段同时存在而被覆盖。
5. 图片流式响应返回 SSE 后可能已经向客户端写出部分内容；中途上游 error event 只能记录到 `StreamStatus` 并继续把 payload 转发，不能伪造未发送过的 JSON 错误响应。
6. `helper.StreamScannerHandler` 已有测试要求在 `info.StreamStatus` 预初始化时保留既有软错误；图片流式路径也依赖该对象记录上游 error event。因此只把初始化逻辑改为 nil 时创建，避免清空调用方已挂载的诊断状态。
7. 本轮不改数据库、模型定价配置、渠道选择、账号池、前端页面和 Realtime WebSocket 逻辑；核心风险集中在图片请求解析、usage 归一和图片流式响应处理。

### 方案评审

采用“小切片增强”方案：不新增新的 relay 包，也不重排 OpenAI adaptor 文件；在现有 `dto.ImageRequest`、请求解析和 `OpenaiHandlerWithUsage` 所在文件内补齐图片流式与计费边界。OpenAI 图片流式处理复用现有 `helper.StreamScannerHandler`，从 JSON `type` 字段重建 SSE `event:` 行，获得与 Chat/Responses 流式路径一致的超时、断连和清理行为。

实现后 `DoResponse` 对 `RelayModeImagesGenerations/ImagesEdits` 按 `info.IsStream` 分发：流式走 `OpenaiImageStreamHandler`，非流式走 `OpenaiImageHandler`。非流式和 JSON-as-SSE 两个入口都先解析 OpenAI error body，只有正常响应才写回客户端并结算 usage。这样可以吸收 `new-api-main` 的协议兼容与安全边界，同时保持 NexusTok 现有图片编辑、渠道适配器、价格倍率和日志结构不变。

验收方式：

1. `go test ./dto ./relay/helper ./relay/channel/openai` 覆盖显式零值、图片请求解析、`n` 上限、multipart body 复读、图片非流式 error、SSE 转发、JSON-as-SSE 和 error event。
2. `go test ./relay ./controller` 确认图片中继入口和 controller 构建不受影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 打开 `http://192.168.0.202:3003/`，确认热更新页面仍正常加载、控制台无新增错误。
5. 在浏览器上下文带 `NexusTok-User: 1` 调用一个不会命中真实上游的非法图片请求（例如缺少模型或 `n` 超上限），确认接口返回明确错误而不是 500；不触发真实图片生成或真实扣费。

## 本轮实施评审：渠道新增缓存刷新补齐

### 需求分析

上一轮用 MCP 验证 OpenAI 图片请求边界时，临时创建了一个只支持 `gpt-image-1` 的 OpenAI 渠道，但随后 relay 请求仍被 Distribute 返回“分组 default 下模型 gpt-image-1 无可用渠道”。复核代码后确认：`AddChannel` 成功写入数据库和 abilities 后只调用 `service.ResetProxyClientCache()`，没有刷新 `model/channel_cache.go` 中的分发缓存；而 `DeleteChannel`、`UpdateChannel`、标签批量操作、上游模型同步等路径都会调用 `model.InitChannelCache()`。这会导致新增渠道在当前进程中无法立即参与模型分发，需要重启或其它缓存刷新动作后才生效。

本轮目标是补齐新增渠道后的缓存刷新，使“创建渠道”成为即时可用的原生管理能力：

1. `POST /api/channel/` 在 `model.BatchInsertChannels` 成功后刷新渠道缓存，确保新渠道及其 abilities 立即参与 Distribute。
2. 保留现有代理客户端缓存重置，避免新渠道代理配置或 BaseURL 变更后复用旧客户端。
3. 不改变渠道写入事务、批量新增、多 Key 拆分、账号池凭证模式、权限和安全验证边界。
4. 用单元测试覆盖新增渠道后调用缓存刷新和代理客户端缓存刷新，防止后续回归。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 渠道新增控制器 | `controller/channel.go` | `AddChannel` 成功后调用 `model.InitChannelCache()`，让新渠道立即进入分发缓存。 |
| 渠道测试 | `controller/channel_test.go` 或相邻测试文件 | 新增控制器级测试，验证新增成功后刷新渠道缓存；失败路径不刷新。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录 MCP 暴露的问题、方案和验收标准。 |

### 风险评估

1. `model.InitChannelCache()` 会重新读取渠道和 abilities，新增渠道成功后调用一次属于已有更新/删除路径的既有行为，风险低；但批量新增大量渠道时会多一次全量缓存构建，因此必须只在 `BatchInsertChannels` 成功后执行。
2. 如果 `InitChannelCache` 内部 panic 或数据库异常，会影响新增接口响应。当前项目在启动和部分任务里已有 panic 保护，但控制器其它更新路径直接调用该函数；本轮保持一致，不新增吞错逻辑，避免新增渠道写入成功但缓存失败被静默隐藏。
3. 该修改不触碰数据库 schema、渠道选择策略、计费、账号池绑定、密钥脱敏和前端表单；核心风险集中在新增渠道后的进程内缓存状态。
4. 测试需要避免依赖真实数据库缓存全局状态。可以使用 monkey patch 或包内可替换 hook 封装缓存刷新调用；若新增 hook，应保持未导出、默认指向原函数，避免生产路径分叉。

### 方案评审

采用最小热路径修复：在 `AddChannel` 的成功分支中，在 `model.BatchInsertChannels(channels)` 成功返回后调用 `model.InitChannelCache()`，然后继续调用 `service.ResetProxyClientCache()`。该顺序与“数据库能力先落盘、分发缓存再重建、HTTP 客户端缓存再清理”的运行语义一致。

为便于测试且不引入外部依赖，在 controller 包内增加未导出的缓存刷新函数变量（默认指向 `model.InitChannelCache` 与 `service.ResetProxyClientCache`），测试中临时替换为计数函数，验证成功路径调用、校验失败路径不调用。生产行为仍调用原有函数，不改变公共 API。

验收方式：

1. `go test ./controller` 覆盖新增渠道成功/失败路径。
2. `git diff --check` 确认无空白错误。
3. 使用 MCP 打开 `http://192.168.0.202:3003/`，确认页面正常加载。
4. 在浏览器上下文创建临时渠道和临时 token，再发起不会命中真实上游的非法图片请求，确认请求不再因为新增渠道缓存未刷新而返回 `model_not_found`；随后删除临时 token 和渠道，并确认无残留。

## 本轮实施评审：额度饱和日志前端可观测性

### 需求分析

NexusTok 后端已经把异常额度转换的饱和保护结果写入 `other.admin_info.quota_saturation`，普通用户日志视图会由后端剥离 `admin_info`，管理员日志视图则保留该审计字段。对比 `new-api-main` 后发现，其默认前端会在使用日志表格的详情摘要和详情弹窗中高亮“Quota clamped”，帮助管理员把异常超大倍率、NaN 或 int32 边界钳制事件与对应请求关联起来；当前 NexusTok 默认前端只解析 `admin_info` 的渠道、计费来源、支付和操作审计字段，缺少 `quota_saturation` 类型定义和显式展示。

本轮目标是补齐额度安全闭环的前端可观测性：

1. 在使用日志 `LogOtherData.admin_info` 中声明 `quota_saturation` 结构，匹配后端 `common.QuotaClamp.AuditMap()` 输出。
2. 管理员查看消费日志时，在表格详情摘要中优先展示危险态“Quota clamped”，让异常请求在列表中可见。
3. 管理员打开详情弹窗时展示钳制类型、原始值、钳制结果和操作来源；普通用户即使收到异常数据也不展示该审计块。
4. 补齐 en、zh、fr、ja、ru、vi 翻译，并用现有 i18n 同步脚本校验。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 使用日志类型 | `web/default/src/features/usage-logs/types.ts` | 新增 `quota_saturation` 管理员审计字段类型，不改变 API 结构。 |
| 使用日志列表 | `web/default/src/features/usage-logs/components/columns/common-logs-columns.tsx` | 管理员消费日志摘要优先显示危险态钳制标记；保留既有违规费、动态计费、缓存、stream 状态等展示。 |
| 使用日志详情 | `web/default/src/features/usage-logs/components/dialogs/details-dialog.tsx` | 新增管理员可见危险态详情区块，展示钳制字段。 |
| 前端 i18n | `web/default/src/i18n/locales/*.json` | 新增六语 UI 文案。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮迁移原因、风险和验收方式。 |

### 风险评估

1. 该字段位于后端已定义的管理员审计命名空间 `admin_info` 下；前端仍同时检查 `props.isAdmin`，即使未来后端脱敏逻辑变化，也不会在非管理员 UI 中展示。
2. 使用日志列表是高密度表格，新增摘要必须只在异常字段存在时出现，避免影响正常日志行的阅读和排序。
3. `original` 可能是极大数或 NaN 来源转出的数值，本轮仅以字符串展示，不参与计算和格式化，避免二次溢出或本地化误读。
4. 新增文案需要覆盖所有默认前端语言；当前环境没有 `bun`，可使用项目现有 `node scripts/sync-i18n.mjs` 和 `tsc -b` 作为替代验证。

### 方案评审

采用最小 UI 接入：复用现有 `DetailSegment.danger`、`DetailSection`、`DetailRow` 和 `AlertTriangle`，不新增组件依赖、不改日志查询 API、不改后端脱敏逻辑。列表层在管理员且存在 `quota_saturation` 时把“Quota clamped”插到详情摘要首位；详情弹窗在请求转换/计费信息之后、拒绝原因之前显示危险态区块。钳制类型映射为 `Overflow`、`Underflow`、`Invalid (NaN)`，未知值按原文展示，保证兼容后续后端扩展。

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 通过。
2. `cd web/default && node scripts/sync-i18n.mjs` 通过并无缺失 key。
3. 使用 MCP 打开 `http://192.168.0.202:3003/usage-logs`，确认页面正常加载、控制台无新增错误；若环境中没有真实饱和日志，可在浏览器上下文用临时样例校验新增文案存在于构建资源和 i18n 中。

## 本轮实施评审：Max Token 家族字段上限保护

### 需求分析

new-api-main 在 relay 请求解析层统一限制了客户端可指定的最大输出 token 字段，避免 `max_tokens`、`max_completion_tokens`、Claude `max_tokens/max_tokens_to_sample`、Gemini `maxOutputTokens/max_output_tokens` 和 Responses `max_output_tokens` 进入后续 token 估算、预消费和 `int(...)` 转换时造成溢出或异常扣费。NexusTok 当前只在 OpenAI 通用文本请求中校验了旧字段 `max_tokens > math.MaxInt32/2`，没有覆盖 `max_completion_tokens`、Claude、Gemini 和 Responses 的同类字段。由于这些字段最终会被 `GetTokenCountMeta()` 或 provider 适配器转换为 `int`，超大 `uint` 一旦漏过验证，会放大为预扣费、动态计费和日志审计链路的安全风险。

本轮目标是把该边界发展成 NexusTok 原生 relay 请求验证能力：

1. 在 `relay/helper/valid_request.go` 定义统一的最大输出 token 边界，使用 `math.MaxInt32 / 2` 作为保守上限。
2. 在 OpenAI 通用文本请求中同时检查 `max_tokens` 和 `max_completion_tokens`。
3. 在 Claude 请求中检查 `max_tokens` 和 `max_tokens_to_sample`。
4. 在 Gemini 请求中检查 `generationConfig.maxOutputTokens`，包括已有 Unmarshal 兼容的 `max_output_tokens`。
5. 在 OpenAI Responses 请求中检查 `max_output_tokens`。
6. 补单元测试覆盖超大值拒绝和正常值接受，锁住计费安全边界。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| relay 请求验证 | `relay/helper/valid_request.go` | 新增统一 token 上限 helper，并接入 OpenAI、Claude、Gemini、Responses 请求解析。 |
| relay helper 测试 | `relay/helper/max_tokens_bounds_test.go` | 覆盖 OpenAI `max_tokens/max_completion_tokens`、Claude、Gemini、Responses 的异常/正常边界。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险和验收方式。 |

### 风险评估

1. 该变更会新增 400/invalid_request 类拒绝路径，但只拒绝超过 `math.MaxInt32/2` 的极端输出 token 参数，不影响正常大上下文模型的输入长度，也不改变模型、消息、prompt 等既有校验。
2. `math.MaxInt32/2` 已被 NexusTok 现有 OpenAI `max_tokens` 校验使用，本轮只是把同一上限扩展到同语义字段，避免字段名迁移绕过安全边界。
3. Claude 和 Gemini 后续会把这些 `uint` 转为 `int` 或参与思考预算计算；提前拒绝可以避免平台差异导致的整数溢出或负数语义。
4. 错误文案保持现有风格：OpenAI/Claude 返回 `max_tokens is invalid`，Responses 返回 `max_output_tokens is invalid`，Gemini 返回 `maxOutputTokens is invalid`；不暴露底层整数类型细节。

### 方案评审

采用最小后端修复：在 `valid_request.go` 中新增 `maxTokensLimit` 和 `exceedsMaxTokensLimit(values ...*uint)`，并在四个请求解析函数完成 JSON 解析和必填字段校验附近调用。该 helper 只读指针值，不改变 DTO，不影响显式零值保留语义。测试使用 `gin.CreateTestContext` 构造 JSON body，保持 `common.UnmarshalBodyReusable` 的真实解析路径；超大值使用 `18446744073686646784`，用于覆盖可被 `uint` 解析但远超安全上限的输入。

验收方式：

1. `go test ./relay/helper` 覆盖新增上限测试。
2. `go test ./relay ./controller` 确认 relay/controller 相关包不回归。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 打开 `http://192.168.0.202:3003/`，并在浏览器上下文调用一个无 token relay 请求和一个带临时 token/临时渠道的超大 `max_completion_tokens` 请求；前者应仍按认证边界返回 401，后者应在请求解析阶段返回明确 `max_tokens is invalid`，不触发真实上游。

## 本轮实施评审：Secure Session Cookie 环境配置

### 需求分析

new-api-main 支持通过 `SESSION_COOKIE_SECURE=true` 让 Gin session cookie 只在 HTTPS 下发送，并要求同时提供 `SESSION_COOKIE_TRUSTED_URL` 作为可信 HTTPS 站点列表，避免生产环境误用非安全 Cookie。NexusTok 当前在 `main.go` 中固定 `Secure: false`，即使部署在 HTTPS 反代之后，也无法通过环境变量启用浏览器的 Secure Cookie 保护。考虑到 NexusTok 已有长期 session、Passkey、2FA、OAuth 和账号池登录会话，该配置属于低侵入但高价值的部署安全增强。

本轮目标是把 Secure session cookie 配置发展成 NexusTok 原生环境能力：

1. 新增 `common.SessionCookieSecure` 和 `common.SessionCookieTrustedURLs` 全局配置，默认保持 false/空列表，确保本地 HTTP 和现有热更新容器行为不变。
2. 新增 `common.InitSessionCookieSettings()`，读取 `SESSION_COOKIE_SECURE` 和 `SESSION_COOKIE_TRUSTED_URL`，并校验组合关系。
3. `SESSION_COOKIE_SECURE=true` 时必须提供一个或多个 HTTPS URL；非 HTTPS、空 URL、secure=false 却配置 trusted URL 均视为启动配置错误。
4. `main.go` 的 session store 使用 `common.SessionCookieSecure`，不改变 Path、MaxAge、HttpOnly 和 SameSite。
5. 补单元测试覆盖默认、缺失组合、非 HTTPS、多 URL 和尾逗号等边界。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 环境初始化 | `common/init.go`、`common/session_cookie.go` | 解析并校验 Secure session cookie 环境变量，配置错误启动即失败。 |
| 全局配置 | `common/session_cookie.go` | 新增 session cookie secure 和 trusted URL 配置变量。 |
| HTTP session store | `main.go` | 将 `sessions.Options.Secure` 从固定 false 改为配置值。 |
| 测试 | `common/session_cookie_test.go` | 覆盖 env 解析和校验边界。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险和验收方式。 |

### 风险评估

1. 默认值仍为 `Secure=false`，不会影响当前 `http://192.168.0.202:3003/` 热更新环境和普通本地开发；只有显式开启环境变量时才改变 Cookie 行为。
2. 如果开启 Secure 后仍通过 HTTP 访问，浏览器不会发送 session cookie，这是 HTTPS 部署的预期行为；因此必须在文档和错误信息中明确 `SESSION_COOKIE_SECURE=true` 需要 HTTPS trusted URL。
3. `SESSION_COOKIE_TRUSTED_URL` 本轮只作为配置校验和后续扩展预留，不接入 CORS、OAuth 回调或 redirect 逻辑，避免扩大影响面。
4. 初始化错误使用 `log.Fatal` 中止启动，能够让错误部署尽早失败，而不是以半安全状态运行。

### 方案评审

采用最小配置增强：新增独立 `common/session_cookie.go`，不混入 session secret 文件；`InitEnv()` 在解析 `SessionMaxAge` 后调用 `InitSessionCookieSettings()`，失败时 `log.Fatal`。URL 校验只接受 `https` scheme 和非空 host，逗号分隔列表会 trim 后保存原始 URL 字符串。`main.go` 只替换 `Secure: false` 为 `Secure: common.SessionCookieSecure`，其它 session 选项保持不变。

验收方式：

1. `go test ./common` 覆盖环境变量解析。
2. `go test ./controller` 确认会话相关控制器测试不受默认值影响。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 打开 `http://192.168.0.202:3003/`，确认默认 Secure=false 下页面仍正常加载、`/api/status` 和 `/api/user/self` 正常、控制台无新增错误。

## 本轮实施评审：转换后上游 JSON 请求体存储与 Content-Length

### 需求分析

new-api-main 在 relay 转换请求后，不再直接把已序列化的 JSON `[]byte` 包装成 `bytes.Buffer` 或 `bytes.Reader` 交给上游 HTTP 请求，而是通过 `BodyStorage` 统一持有转换后的请求体，并把最终 body 字节数写入 `RelayInfo.UpstreamRequestBodySize`。这样一来，当图片、Responses、Gemini、Claude 或 Embedding 请求包含较大的 base64 内容时，请求体可以按现有阈值落到临时文件，减少等待上游响应期间的堆内存驻留；同时，由于 `ReaderOnly(BodyStorage)` 会隐藏具体 reader 类型，需要在构造上游 `http.Request` 后显式设置 `ContentLength`，避免部分上游因为缺少 `Content-Length` 而拒绝 chunked 请求。

NexusTok 已具备 `common.BodyStorage`、`common.ReaderOnly` 和请求体大小限制能力，但转换后的上游 JSON body 仍散落在多个 handler 中使用 `bytes.NewBuffer(jsonData)` 或 `bytes.NewReader(jsonData)`。本轮目标是把 new-api-main 的优势原生化为 NexusTok 的 relay 基础能力：

1. 在 `relay/common` 增加转换后 JSON body 的统一包装 helper，复用现有 `common.CreateBodyStorage` 和 `common.ReaderOnly`。
2. 在 `RelayInfo` 中记录转换后上游请求体大小，作为通用请求构造层回填 `ContentLength` 的唯一来源。
3. 在 `relay/channel` 的通用 HTTP 请求构造路径中，如果请求体来自转换后的 `BodyStorage`，显式设置 `req.ContentLength`。
4. 将当前主要 JSON 转换 handler 接入该 helper，包括 Chat Completions、Responses、Gemini、Gemini Embedding、Claude、Embedding、Image JSON、Rerank，以及 Chat Completions via Responses；不改变 multipart、原始透传 body、WebSocket 和任务接口的语义。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| relay 公共请求体 | `relay/common/outbound_body.go` | 新增转换后 JSON 请求体包装 helper，统一返回 reader、size 和 closer。 |
| relay 上下文 | `relay/common/relay_info.go` | 新增 `UpstreamRequestBodySize`，仅用于通用上游请求构造层回填 `ContentLength`。 |
| 上游请求构造 | `relay/channel/api_request.go` | 在 `DoApiRequest`、`DoFormRequest`、`DoTaskApiRequest` 中按 `RelayInfo` 回填 `ContentLength`；对未设置 size 的请求保持 net/http 默认行为。 |
| relay handler | `relay/compatible_handler.go`、`relay/responses_handler.go`、`relay/gemini_handler.go`、`relay/claude_handler.go`、`relay/embedding_handler.go`、`relay/image_handler.go`、`relay/rerank_handler.go`、`relay/chat_completions_via_responses.go` | 将转换后的 JSON body 从 `bytes.*` reader 切换为 `BodyStorage` 包装；请求完成后关闭 storage。 |
| 测试 | `relay/common/*_test.go`、`relay/channel/api_request_test.go` | 覆盖 helper 的 size/read/close 语义和 `ContentLength` 回填规则。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险、方案和验收方式。 |

### 风险评估

1. 该变更位于 relay 热路径，风险主要是请求体生命周期、Content-Length 计算和关闭时机。handler 必须在 `adaptor.DoRequest` 返回后再 `defer closer.Close()`，不能提前关闭 `BodyStorage`，否则上游传输可能读不到完整 body。
2. 透传请求体仍使用客户端原始 `BodyStorage`，不设置 `UpstreamRequestBodySize`，避免对原始 body、multipart 文件上传和 WebSocket 产生额外语义变化。
3. `BodyStorage` 可能落盘，调试日志不能为了释放内存再调用 `Bytes()` 读取磁盘内容；本轮只在已有 debug 分支保留原字符串日志，并在包装后置空 `jsonData`，让大 payload 更快被 GC。
4. `ContentLength` 只在 `info.UpstreamRequestBodySize > 0` 且 `req.ContentLength <= 0` 时设置；如果某些特殊请求已经由 `net/http` 自动识别长度，则保留原值。
5. 自定义 adaptor 中直接调用 `http.NewRequest` 的少数路径不在本轮覆盖范围内；本轮优先强化通用 `DoApiRequest` 路径和主要 JSON handler，后续可按差异报告继续细化非通用 adaptor。

### 方案评审

采用 new-api-main 已验证的最小底座方案，但按 NexusTok 现有中文注释和包结构落地：新增 `relay/common.NewOutboundJSONBody(data []byte)`，返回 `io.Reader`、字节数、`io.Closer` 和错误。调用方在完成 `common.Marshal`、字段裁剪、参数覆写和日志输出后调用该 helper，随后 `jsonData = nil`，把 size 写入 `info.UpstreamRequestBodySize`，并 `defer closer.Close()`。

`relay/channel/api_request.go` 新增未导出的 `applyUpstreamContentLength(req, info)`，集中解释为什么 `ReaderOnly(BodyStorage)` 需要手动回填长度。`DoApiRequest`、`DoFormRequest`、`DoTaskApiRequest` 统一调用该函数。这样 handler 只负责声明“转换后的 body 有多大”，请求构造层负责把它映射为 HTTP 协议字段，边界清晰且便于测试。

验收方式：

1. `go test ./relay/common ./relay/channel` 覆盖新增 helper 与 Content-Length 回填。
2. `go test ./relay ./controller` 确认 relay/controller 相关路径不回归。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 打开 `http://192.168.0.202:3003/` 并调用一个认证边界内的 relay 请求，确认页面仍正常、接口仍按预期响应；若 MCP 浏览器工具不可用，则使用 `curl` 访问页面和 `/api/status` 作为替代验证，并在最终报告中说明。

验证记录：

1. `go test -count=1 ./relay/common ./relay/channel` 通过，覆盖 `NewOutboundJSONBody` 和 `ContentLength` 回填规则。
2. `go test -count=1 ./relay ./controller` 通过，确认 relay 入口和 controller 相关包不回归。
3. `go test -count=1 ./relay/...` 中本轮相关包均通过，但存量 `relay/channel/claude/message_delta_usage_patch_test.go` 缺少 `testing`、`dto`、`require`、`gjson` 等导入，导致 `relay/channel/claude` 包构建失败；该文件不属于本轮改动范围。
4. MCP Chrome DevTools 无法连接 `http://127.0.0.1:9222/json/version`，本轮使用 `curl` 替代访问 `http://192.168.0.202:3003/`，主页返回 200；`/api/status` 返回 200；无 token 请求 `/v1/chat/completions` 返回 401，认证边界正常。

## 本轮实施评审：登录成功审计日志

### 需求分析

new-api-main 在用户完成登录后会写入 `LogTypeLogin=7` 的成功登录审计日志，记录登录方式、客户端 IP 和 User-Agent，并在默认前端 Usage Logs 中提供 Login 类型、筛选项和详情展示。NexusTok 当前只更新 `last_login_at`，无法让用户或管理员在统一日志页面追踪账号近期成功登录来源。考虑到 NexusTok 已支持密码、2FA、Passkey、标准 OAuth、微信和 Telegram 登录，成功登录日志能补齐账号安全排障闭环：当用户发现异常消费或会话变化时，可以先在日志里确认是否存在异常登录。

本轮目标是把该能力发展成 NexusTok 原生日志能力：

1. 在后端日志类型中新增固定值 `LogTypeLogin=7`，不使用 `iota`，避免改变既有日志类型值。
2. 新增 `model.RecordLoginLog`，复用现有 `logs.other` 文本 JSON 保存 `login_method`、`user_agent` 和语言无关的 `op` 字段，不新增表和字段，保持 SQLite、MySQL、PostgreSQL 兼容。
3. 在 `setupLogin` 成功保存 session 后记录登录审计；2FA、Passkey、OAuth、微信、Telegram 等最终都会走该函数，因此只需单点接入。失败登录不记录，避免用户名枚举和日志噪声。
4. 前端 Usage Logs 增加 Login 类型、筛选值、详情入口，并在详情弹窗展示登录方式、IP 和 User-Agent；这些字段对日志归属用户可见，不放入 `admin_info`。
5. 补齐 en、zh、fr、ja、ru、vi 翻译，并用现有 i18n 同步和 TypeScript 构建验证。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 后端日志类型 | `model/log.go` | 新增 `LogTypeLogin=7`、`RecordLoginLog`，普通用户日志脱敏逻辑保留 login 字段。 |
| 登录控制器 | `controller/user.go` | 新增登录方式推导和成功登录审计；`setupLogin` session 保存成功后写日志。 |
| 后端测试 | `model/log_audit_test.go`、`controller/*` 相关测试 | 覆盖登录日志 JSON 结构，保证日志类型、IP、方法和 User-Agent 写入正确。 |
| 前端日志常量 | `web/default/src/features/usage-logs/constants.ts`、`common-logs-filter-bar.tsx` | 增加 Login 类型和筛选值，确保 type=7 可被查询。 |
| 前端日志类型/详情 | `web/default/src/features/usage-logs/types.ts`、`details-dialog.tsx` | 声明 login 审计字段，并展示登录信息详情。 |
| 前端 i18n | `web/default/src/i18n/locales/*.json` | 新增 Login、Login Info、Login Method、User Agent 等六语文案。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. 登录成功日志会增加 `logs` 写入量，但仅在成功登录时写入一条轻量记录，不受 `LogConsumeEnabled` 影响；若日志库写入失败，只写系统日志，不阻断用户登录。
2. 仅记录成功登录，不记录失败用户名、失败 IP 或密码错误原因，避免让日志变成账号枚举信号源。
3. `User-Agent` 可能较长，本轮只写入 `Other` 文本字段并在详情弹窗 mono 展示，不参与索引和查询，避免扩大数据库索引负担。
4. `login_method` 来自路由模板和 provider 参数，属于稳定技术标识；标准 OAuth provider 使用 `oauth:<provider>`，便于后续筛选或统计。
5. 前端 type=7 不加入 `DISPLAYABLE_LOG_TYPES`，避免 Token、Cost、Channel 等用量列解析无关字段；详情列本身始终可打开，因此登录审计信息仍可查看。

### 方案评审

采用单点接入方案：`setupLogin` 是密码登录、2FA 登录、Passkey 登录和 OAuth 登录最终落点，因此在 session 保存成功后调用 `recordLoginAudit(user, c)`。`recordLoginAudit` 通过 `c.FullPath()` 推导登录方式，构造 `content="Logged in successfully via <method>"` 作为导出/旧前端兜底文本，并调用 `model.RecordLoginLog` 写入结构化 `other`。日志归属 `user.Id`，`Username` 使用登录流程已持有的用户对象，避免额外数据库查询。

前端复用现有 `DetailSection`、`DetailRow` 和 `StatusBadge`，只新增 `LogIn` 图标和登录审计区块，不新增组件依赖。Login 类型加入 `LOG_TYPES` 和 `logTypeValues`，但不加入 `DISPLAYABLE_LOG_TYPES`，让登录日志只参与类型徽标、筛选和详情展示，不进入消费用量列。详情弹窗在管理审计区块之后展示 Login Info，普通用户和管理员查看该用户日志时都可见。翻译直接补齐所有默认语言。

验收方式：

1. `go test -count=1 ./model ./controller` 覆盖后端日志结构和登录控制器编译。
2. `cd web/default && node scripts/sync-i18n.mjs` 确认新增 key 同步到所有 locale。
3. `cd web/default && ./node_modules/.bin/tsc -b` 确认默认前端类型通过。
4. 使用 MCP 打开 `http://192.168.0.202:3003/` 并执行一次真实登录，再访问 Usage Logs 验证出现 Login 类型记录和详情；若 MCP 浏览器不可用，则用 `curl` 登录、携带 cookie 调用 `/api/log/self` 或 `/api/log` 验证日志记录。

验证记录：

1. `go test -count=1 ./model ./controller` 通过，覆盖 `RecordLoginLog` 的结构化 JSON 写入。
2. `cd web/default && node scripts/sync-i18n.mjs` 通过，所有 locale missing/extras 均为 0；untranslated 为存量。
3. `cd web/default && ./node_modules/.bin/tsc -b` 通过，Usage Logs 路由 search schema 已同步允许 type=7。
4. MCP Chrome DevTools 仍无法连接 `http://127.0.0.1:9222/json/version`，本轮用 `curl` 替代验证：访问 `http://192.168.0.202:3003/` 返回 200；调用 `/api/user/login` 真实登录账号 `c1cada` 成功；携带 session cookie 与 `NexusTok-User: 1` 调用 `/api/log/self?type=7` 返回最新登录日志，字段包含 `type=7`、`content=Logged in successfully via password`、`login_method=password`、`user_agent=curl/8.18.0` 和 `op.action=login`。

## 本轮实施评审：Pricing 路由最新配置守卫

### 需求分析

new-api-main 在 `/pricing` 与 `/pricing/:modelId` 进入路由前会调用 `getStatus()` 刷新 `HeaderNavModules`，再按最新的 `pricing.enabled` 和 `pricing.requireAuth` 决定是否允许访问或跳转登录。NexusTok 已经原生化了后端 `HeaderNavModuleAuth` 和前端健壮解析，但 Pricing 路由当前仍只读取 `localStorage.status` 的旧缓存。这样当 Root 在系统设置中临时关闭价格页，或把价格页从公开改为登录可见时，已经打开过站点的浏览器可能继续按旧状态进入页面，直到其它代码刷新 status。

本轮目标是把 new-api-main 的路由级新鲜配置检查转成 NexusTok 默认前端原生能力：

1. 在 `web/default/src/lib/nav-modules.ts` 增加 `getFreshModuleAccess(module)`，通过 `getStatus()` 获取最新系统状态，成功后写回本地 `status` 缓存，并复用现有 `getModuleAccessFromStatus()` 解析语义。
2. 为 `getCachedStatus()` 增加服务端/构建期 `window` guard，避免路由预加载或测试环境访问 `window.localStorage` 抛错。
3. 在 `/pricing` 与 `/pricing/$modelId` 的 TanStack Router `beforeLoad` 中调用新 helper：关闭时跳转首页，要求登录且当前无用户时跳转 `/sign-in` 并携带原始 `redirect`。
4. 不改 Pricing 页面数据请求、筛选 search schema、顶部导航渲染、后端鉴权中间件和模型定价接口，避免把路由守卫与定价业务混在一起。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 顶栏模块配置 helper | `web/default/src/lib/nav-modules.ts` | 新增最新 status 拉取与缓存写回能力，保留既有缓存读取 API。 |
| Pricing 列表路由 | `web/default/src/routes/pricing/index.tsx` | 进入页面前按最新 `pricing` 模块配置进行跳转。 |
| Pricing 详情路由 | `web/default/src/routes/pricing/$modelId/index.tsx` | 与列表页保持同一访问控制语义。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. 每次进入 Pricing 路由会多一次 `/api/status` 请求，属于低频公开页面导航，不进入 Relay 热路径，也不改变 Pricing 业务 API 的缓存和分页行为。
2. 当 `/api/status` 网络失败时，本轮按 new-api-main 的 fail-closed 语义返回 `{ enabled: false, requireAuth: true }`，路由会跳转首页或登录页，避免配置不可确认时误开放受限页面；代价是短暂网络故障可能让价格页不可达。
3. `getFreshModuleAccess()` 会写回本地 `status`，可能刷新顶部导航和后续页面守卫看到的配置，这是预期行为；写缓存失败会被吞掉，不影响当前访问判定。
4. 未登录且 `requireAuth=true` 时只检查 `useAuthStore` 的本地用户状态，后端 `/api/pricing` 仍有 `HeaderNavModuleAuth` 兜底；如果本地用户过期，后端会继续按已有认证边界拒绝。
5. 本轮不触碰 i18n 文案、UI 结构、Pricing 详情组件和系统设置保存逻辑，因此风险集中在 import、路由跳转类型和 TypeScript 编译。

### 方案评审

采用最小前端路由守卫方案：`nav-modules.ts` 只新增 `getStatus` 依赖和两个 helper，不改变 `parseHeaderNavModules`、`getModuleAccess` 和 `isSidebarModuleEnabled` 既有导出。`cacheStatus()` 只在浏览器环境且 status 存在时写入 localStorage，保持 SSR/测试环境安全。两个 Pricing 路由复用同一段 `beforeLoad` 逻辑，按 TanStack Router 的 `redirect()` 抛出跳转，`redirect` 参数使用 `location.href` 保留用户原本筛选、模型详情路径和 query。

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认默认前端路由和类型通过。
2. `git diff --check` 确认无空白错误。
3. 使用 MCP 打开 `http://192.168.0.202:3003/`，访问 `/pricing` 与一个 `/pricing/:modelId` 路径，确认页面按当前配置加载或跳转；如 MCP Chrome 仍不可用，则用 `curl` 访问主页、`/api/status` 和 `/pricing` 验证热更新页面可达，并在最终回复说明替代验证。

验证记录：

1. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认新增 `beforeLoad`、`redirect` 和 auth store 读取类型正确。
2. `git diff --check` 通过，无空白错误。
3. MCP Chrome DevTools 仍无法连接 `http://127.0.0.1:9222/json/version`，本轮使用 `curl` 替代真实访问：`http://192.168.0.202:3003/` 返回 200，`/api/status` 返回 200 且当前 `HeaderNavModules` 为空字符串，`/pricing` 返回 200，符合默认公开配置。
4. 通过 `curl` 拉取 `http://192.168.0.202:3003/static/js/index.js`，确认当前 3003 提供的前端 bundle 已刷新并包含 `/api/status`、`requireAuth`、`sign-in` 与 `pricing` 等守卫相关片段；本轮路由行为是客户端跳转，无法用纯 HTML 响应直接观察跳转状态。

## 本轮实施评审：`/v1/models` 动态 `owned_by` 原生化

### 需求分析

new-api-main 在 `/v1/models` 返回 OpenAI 兼容模型列表时，不再只使用内置适配器静态归属或未知模型的 `custom`，而是根据当前用户/Token 实际可用分组里的能力表，选择该模型优先路由到的启用渠道，并把渠道类型转换成 `owned_by`。这能让客户端、SDK、监控和排障工具看到更接近真实调度路径的归属，例如自定义 `gpt-5.4` 如果实际由 Codex 渠道承载，就返回 `owned_by=codex`，而不是笼统的 `custom`。

NexusTok 当前 `/v1/models` 已能按用户组、Token 模型限制和计费配置过滤可见模型，但 `owned_by` 对动态模型仍只回落为 `custom`，对静态模型也不能反映当前分组中实际优先渠道。考虑到 NexusTok 已原生化账号池、渠道账号、SystemTask 和模型元数据同步，模型列表的归属字段也应体现当前平台调度能力，而不是只暴露内置默认值。

本轮目标：

1. 在模型层新增三库兼容的 `GetPreferredModelOwnerChannelTypes(modelNames, groups)`，只读 `abilities` 和 `channels`，按启用状态、渠道状态、优先级、权重和 channel_id 稳定选择每个模型的首选渠道类型。
2. 在 controller 层新增 `channelOwnerName`、`getPreferredModelOwners` 和 `buildOpenAIModel`，把渠道类型转换成适配器 channel name，并集中构造 OpenAI 模型响应。
3. 重构 `ListModels` 为“先收集模型名，再统一构造响应”，保持现有分组、`auto` 分组、Token 模型限制、`AcceptUnsetRatioModel` 和 `SupportedEndpointTypes` 语义。
4. 对 Token 模型限制路径保持兼容：如果测试或特殊调用没有用户分组上下文，仍返回模型列表，只是不做动态 `owned_by` 覆盖，避免比当前行为更严格。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 模型层查询 | `model/model_meta.go` | 新增模型首选渠道类型查询 helper，使用 GORM join 和 `IN ?`，不新增表字段。 |
| 模型层测试 | `model/model_owner_test.go` | 覆盖优先级、权重、channel_id 稳定排序、分组过滤、禁用能力和禁用渠道。 |
| 模型列表控制器 | `controller/model.go` | `/v1/models` OpenAI 格式响应按实际可用渠道覆盖 `owned_by`，Claude/Gemini 派生响应继续复用同一可见模型集合。 |
| 控制器测试 | `controller/model_owned_by_test.go`、`controller/model_list_test.go` | 覆盖 owner name 转换、未知模型回落、分组解析和 `/v1/models` 返回动态 `owned_by`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. `/v1/models` 是 Token 鉴权后的公开兼容接口，新增一次能力表与渠道表查询会增加轻量读负载；查询只按当前响应模型名和分组过滤，不进入 Relay 请求热路径。
2. `owned_by` 字段会从静态值变为更贴近调度路径的动态值，依赖该字段做展示的客户端会看到更准确的 provider；若无启用渠道或上下文不足，则保留旧值，避免误报。
3. 查询排序必须和调度语义一致：优先级高者优先，同优先级权重大者优先，再用 channel_id 升序稳定打破完全相同配置，避免同一模型每次列表归属抖动。
4. 只查询 `channels.status = enabled` 和 `abilities.enabled = true`，不把手动禁用渠道或禁用能力暴露成 owner；这与模型实际可用性一致。
5. 本轮不改变模型过滤、计费表达式、Token 权限、渠道分发和数据库 schema，因此核心业务风险集中在响应字段和查询性能。

### 方案评审

采用 new-api-main 的已验证方案，但按 NexusTok 当前测试和兼容性收敛：模型层新增 `normalizeLookupValues` 以去空、去重并保留顺序；`GetPreferredModelOwnerChannelTypes` 使用 `DB.Table("abilities")` join `channels`，并复用 `commonGroupCol` 引用保留字列。controller 层先收集 `userModelNames`，再在能确定 `ownerGroups` 时查询动态 owner；构造响应统一走 `buildOpenAIModel`，避免 Token 限制路径和普通分组路径继续复制静态/custom 分支。

验收方式：

1. `go test -count=1 ./model ./controller` 覆盖新增 owner 查询、模型列表响应和现有 controller 逻辑。
2. `git diff --check` 确认无空白错误。
3. 使用 MCP 打开 `http://192.168.0.202:3003/` 并通过浏览器上下文调用 `/v1/models`，确认认证边界和模型列表响应正常；若 MCP Chrome 仍不可用，则用 `curl` 访问主页、`/api/status` 和无 token `/v1/models` 401 作为替代验证，并说明无法直接用浏览器执行带真实 Token 的模型列表调用。

验证记录：

1. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./model` 通过，覆盖 `GetPreferredModelOwnerChannelTypes` 的优先级、权重、稳定排序、分组过滤和禁用状态。
2. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./controller -run 'Test(ListModels|ChannelOwnerName|BuildOpenAIModel|GetModelListGroups)'` 通过，覆盖本轮 controller helper 和 `/v1/models` 动态 `owned_by` 响应。
3. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./controller` 首次在沙箱内因存量 `model_sync_test.go` 的 `httptest.NewServer` 无法监听本地端口失败；按权限规则提升后完整 `./controller` 通过。
4. `git diff --check` 通过，无空白错误。
5. MCP Chrome DevTools 仍无法连接 `http://127.0.0.1:9222/json/version`，本轮使用 `curl` 替代访问热更新站点：`http://192.168.0.202:3003/` 返回 200，`/api/status` 返回 200，无 Token `/v1/models` 返回 401 `Invalid token`，认证边界未放宽。

## 本轮实施评审：Rankings 路由最新配置守卫

### 需求分析

new-api-main 的 `/rankings` 页面和 `/pricing` 一样，进入路由前会调用 `getFreshModuleAccess('rankings')` 拉取最新 `/api/status`，再根据 `HeaderNavModules.rankings.enabled/requireAuth` 决定是否允许访问。NexusTok 当前已经具备后端 `HeaderNavModuleAuth`、前端集中解析和 Rankings 缓存守卫，但页面守卫仍只读取 `localStorage.status`：当管理员刚关闭排行榜或改成登录可见时，已经打开过站点的浏览器可能继续按旧缓存进入页面，随后才由接口鉴权或导航刷新表现出新配置。

本轮目标是把 new-api-main 的 Rankings 新鲜配置检查转成 NexusTok 默认前端原生能力：

1. `/rankings` 进入路由前调用已有 `getFreshModuleAccess('rankings')`，与 Pricing 使用同一配置读取语义。
2. `enabled=false` 时跳转首页；`requireAuth=true` 且本地无用户时跳转 `/sign-in`，并用 `location.href` 保留原始查询参数。
3. 保留 NexusTok 自有 `period=all` 搜索参数，不回退到 new-api-main 只支持 `today/week/month/year` 的 schema。
4. 不改排行榜页面组件、数据 API、后端 HeaderNavModuleAuth 和顶部导航渲染，避免把路由守卫与排行统计逻辑混在一起。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Rankings 路由 | `web/default/src/routes/rankings/index.tsx` | 从缓存访问守卫改为进入路由时刷新最新 status；登录跳转保留完整 location。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. 每次进入 Rankings 路由会多一次 `/api/status` 请求，和 Pricing 保持一致；该请求是低频导航配置读取，不影响排行榜数据聚合和 Relay 热路径。
2. `/api/status` 失败时 `getFreshModuleAccess` 会 fail-closed，页面可能短暂跳转首页或登录页；这比配置不可确认时误开放受控页面更安全。
3. `useAuthStore` 仍只做前端本地用户判断，服务端 `/api/rankings` 的 HeaderNavModuleAuth 继续作为最终边界；本轮不放宽任何后端鉴权。
4. 保留 `period=all` 可避免破坏 NexusTok 当前排行榜页面已有的“全部时间”视图和 URL 兼容性。

### 方案评审

采用最小路由改动：`rankings/index.tsx` 将 `getModuleAccess` 替换为 `getFreshModuleAccess`，`beforeLoad` 改为 async 并接收 `location`，未登录跳转时使用 `location.href` 保存原始 URL。由于 `getFreshModuleAccess` 已在 Pricing 切片中实现并验证，本轮不新增 helper、不改 `nav-modules.ts`，只让 Rankings 复用同一原生能力。

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 确认默认前端路由类型通过。
2. `git diff --check` 确认无空白错误。
3. 使用 MCP 打开 `http://192.168.0.202:3003/rankings`，验证页面按当前配置加载或跳转；若 MCP Chrome 仍不可用，则用 `curl` 访问主页、`/api/status`、`/rankings` 和前端 bundle 片段作为替代验证。

验证记录：

1. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 Rankings 路由的 async `beforeLoad` 和跳转参数类型正确。
2. `git diff --check` 通过，无空白错误。
3. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用 `curl` 替代访问热更新站点。
4. `curl` 访问 `http://192.168.0.202:3003/`、`/api/status`、`/rankings` 和 `/static/js/index.js` 均返回 `HTTP/1.1 200 OK`，确认 3003 页面与资源可访问。
5. 前端 bundle 中可检索到 `/api/status`、`rankings`、`requireAuth`、`sign-in` 片段，确认热更新资源包含本轮配置刷新守卫相关逻辑。

## 本轮实施评审：注册兼容路由与已登录跳转

### 需求分析

new-api-main 在默认前端额外提供 `(auth)/register.tsx`，访问 `/register` 时会保留当前查询参数并替换跳转到 `/sign-up`。NexusTok 当前只有 `/sign-up` 路由，但钱包邀请链接 `generateAffiliateLink` 已经生成 `/register?aff=...`，这会让邀请注册入口依赖一个不存在的前端路由。另一方面，new-api-main 的 `/sign-up` 在已登录状态下会直接跳转 `/dashboard`，避免用户在已有会话里误触注册页。

本轮目标是把 new-api-main 的兼容入口沉淀为 NexusTok 默认前端原生能力：

1. 新增 `/register` 前端路由，进入后 `replace` 到 `/sign-up`，并完整保留 `?aff=...` 等查询参数。
2. 为 `/sign-up` 增加已登录用户跳转 `/dashboard` 的轻量守卫，避免已登录状态误进入注册表单。
3. 不改 `SignUp` 组件、注册表单、`/api/user/register` 后端接口、邀请参数解析和钱包邀请链接生成逻辑。
4. 更新 TanStack Router 生成路由树，确保类型安全导航中包含 `/register`。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 注册兼容路由 | `web/default/src/routes/(auth)/register.tsx` | 新增 `/register` 到 `/sign-up` 的 replace 跳转，并保留查询参数。 |
| 注册页守卫 | `web/default/src/routes/(auth)/sign-up.tsx` | 已登录用户访问注册页时跳转 `/dashboard`。 |
| 生成路由树 | `web/default/src/routeTree.gen.ts` | TanStack Router 类型注册新增 `/register`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. `/register` 只做前端 replace 跳转，不新增页面和接口调用，不会改变后端注册开关、邀请校验、密码策略和 OAuth 注册逻辑。
2. 查询参数必须原样传递给 `/sign-up`，否则邀请注册的 `aff` 参数会丢失；本轮以 `location.search` 作为跳转 search，复用 TanStack Router 对 search 的处理。
3. 已登录用户跳转 `/dashboard` 与 new-api-main 行为一致，但可能改变少量用户手动访问注册页时看到表单的旧体验；该场景本身不应继续注册新账号，跳转更符合现有会话语义。
4. `routeTree.gen.ts` 是生成文件，必须由现有 TanStack Router 插件刷新，避免手工遗漏类型映射。

### 方案评审

采用最小兼容方案：新增 `(auth)/register.tsx`，复用 `createFileRoute('/(auth)/register')` 和 `redirect({ to: '/sign-up', search: location.search, replace: true })`；`sign-up.tsx` 只增加 `useAuthStore` 检查和 `/dashboard` 跳转。该方案吸收 new-api-main 的兼容优势，同时保留 NexusTok 自己的钱包邀请链接格式，不引入注册页重构。

验收方式：

1. 运行路由生成/构建相关命令，确认 `routeTree.gen.ts` 包含 `/register`。
2. `cd web/default && ./node_modules/.bin/tsc -b` 确认默认前端类型通过。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 打开 `http://192.168.0.202:3003/register?aff=demo`，确认跳转到 `/sign-up?aff=demo`；若 MCP Chrome 仍不可用，则用 `curl` 访问 `/register?aff=demo`、主页和前端 bundle 片段作为替代验证，并说明工具限制。

验证记录：

1. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，并刷新 `web/default/src/routeTree.gen.ts`，生成路由树中已包含 `/register`、`/(auth)/register` 和 `authRegisterRoute`。
2. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 `/register` 跳转 search 参数、`/sign-up` 已登录跳转和生成路由类型一致。
3. `git diff --check` 通过，无空白错误。
4. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用 `curl` 替代访问热更新站点。
5. `curl` 访问 `http://192.168.0.202:3003/`、`/api/status`、`/register?aff=demo`、`/sign-up?aff=demo` 和 `/static/js/index.js` 均返回 `HTTP/1.1 200 OK`，确认 3003 页面和兼容入口可访问。
6. 前端 bundle 中可检索到 `/register`、`/sign-up`、`aff`、`dashboard` 片段，确认热更新资源包含本轮兼容路由和已登录跳转逻辑。

## 本轮实施评审：SMTP NTLM 自适应认证

### 需求分析

new-api-main 的邮件发送层提供 `AutoSMTPAuth`，会根据 SMTP 服务器 EHLO 返回的认证机制在 `PLAIN`、`LOGIN` 和 `NTLM` 之间选择。NexusTok 当前只在配置或 Outlook 账号命中时使用 `LOGIN`，其它情况使用 `PLAIN`；如果企业 Exchange、Microsoft 365 或内网 SMTP 只开放 `AUTH NTLM`，注册验证、密码重置、配额预警和用户通知邮件会在认证阶段失败。

本轮目标是把 new-api-main 的 SMTP NTLM 兼容能力转成 NexusTok 原生基础设施能力：

1. 新增 SMTP 自适应认证对象，优先遵守 `SMTPForceAuthLogin`，并在服务器只支持或明确支持 `NTLM` 时使用 NTLM 协商。
2. 保留现有 `shouldUseSMTPLoginAuth()` 的兼容语义：Outlook/onmicrosoft 或 `EmailLoginAuthServerList` 仍优先 LOGIN，除非服务器只声明 NTLM。
3. 继续在标准服务器上使用 `PLAIN`，不改变 `SMTPServer`、`SMTPPort`、`SMTPSSLEnabled`、`SMTPAccount`、`SMTPToken` 等配置模型。
4. 不引入前端设置项、不改邮件模板、不改验证码或通知业务流程。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| SMTP 认证 | `common/email.go`、`common/email_ntlm_auth.go` | `getSMTPAuth()` 改为自适应认证，新增 NTLM 协商实现。 |
| 测试 | `common/email_ntlm_auth_test.go` | 覆盖 PLAIN、LOGIN、NTLM 和 Outlook/onmicrosoft 仅 NTLM 场景的认证机制选择。 |
| 依赖 | `go.mod`、`go.sum` | 新增 `github.com/Azure/go-ntlmssp`，仅用于 NTLM 消息生成。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. SMTP 认证发生在邮件发送路径，失败会影响注册验证、密码重置和通知；本轮只替换 `smtp.Auth` 实现，不改变连接建立、收件人、邮件内容和错误返回。
2. `smtp.Auth.Start` 会收到服务器支持的机制列表；若服务器支持 `PLAIN`，仍保持原路径，减少对常规邮件服务商的扰动。
3. NTLM 依赖新增第三方库，必须固定版本并用单元测试覆盖协商入口，避免只在真实企业 SMTP 环境中才暴露编译或选择错误。
4. `SMTPForceAuthLogin=true` 仍强制 LOGIN；这是管理员显式配置，不能被自动探测覆盖。

### 方案评审

采用最小后端基础设施改动：保留现有 `LoginAuth`，新增 `AutoSMTPAuth` 包装选择逻辑，`getSMTPAuth()` 统一返回 `AutoSMTPAuth(SMTPAccount, SMTPToken)`。这样已有手动 LOGIN、Outlook 兼容和 PLAIN 服务器行为保持不变，只为服务器能力列表里出现 NTLM 的场景补上协商分支。测试不搭完整 SMTP 服务，而是直接构造 `smtp.ServerInfo` 验证 `Start` 选择的机制和初始响应，保证切片稳定可重复。

验收方式：

1. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./common -run 'TestAutoSMTPAuth|TestGetSMTPAuth'` 覆盖 SMTP 认证机制选择。
2. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./common` 做 common 包回归。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 打开 `http://192.168.0.202:3003/` 确认页面仍可访问；若 MCP Chrome 仍不可用，则用 `curl` 访问主页和 `/api/status` 作为替代验证。

验证记录：

1. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./common -run 'TestAutoSMTPAuth|TestGetSMTPAuth'` 通过，覆盖 PLAIN、强制 LOGIN、历史 LOGIN 服务器、仅 NTLM 服务器和 onmicrosoft 仅 NTLM 场景。
2. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./common` 通过，确认 common 包回归正常。
3. `git diff --check` 通过，无空白错误。
4. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用 `curl` 替代访问热更新站点。
5. `curl` 访问 `http://192.168.0.202:3003/` 和 `/api/status` 均返回 `HTTP/1.1 200 OK`，确认本轮后端基础设施改动未影响 3003 页面与状态接口可用性。

## 本轮实施评审：Moonshot kimi-k2.6 temperature 兼容

### 需求分析

new-api-main 在 Moonshot adapter 中针对 `kimi-k2.6` 增加了 temperature 兼容规则：该上游模型只接受 `temperature=1.0`，当客户端显式传入其它 temperature 时应在转发前改为 `1.0`；如果客户端没有传入 temperature，则保持字段省略，避免破坏“缺省值由上游决定”的语义。NexusTok 当前 Moonshot 的 `ConvertOpenAIRequest` 直接透传请求，遇到 `kimi-k2.6` 且客户端显式传入 `0`、`0.7` 等值时，可能被 Moonshot 上游拒绝。

本轮目标是把该 provider 兼容规则转成 NexusTok 原生能力：

1. 仅在 Moonshot OpenAI 兼容请求路径中处理 `kimi-k2.6`。
2. 优先使用 `RelayInfo.ChannelMeta.UpstreamModelName` 判断上游真实模型名，缺失时回退请求体 `model`。
3. 仅当 `Temperature` 指针非 nil 且值不等于 `1.0` 时修正为 `1.0`；nil 保持 nil，遵守上游请求 DTO 显式零值保留规则。
4. 不修改通用 OpenAI adapter、Claude/Gemini/Responses 路径、计费、模型映射和请求参数覆盖逻辑。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Moonshot adapter | `relay/channel/moonshot/adaptor.go` | 新增 `kimi-k2.6` temperature 修正 helper，并在 `ConvertOpenAIRequest` 中调用。 |
| Moonshot 测试 | `relay/channel/moonshot/adaptor_test.go` | 覆盖 temperature nil、`1.0`、`0`、`0.7`、非目标模型和上游模型名覆盖本地模型名。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. 这是单 provider 局部改动，风险集中在模型名判断错误或误改非 `kimi-k2.6` 请求；通过 helper 和单元测试约束范围。
2. `Temperature` 是指针字段，nil 表示客户端未传入；本轮不把 nil 改成 `1.0`，避免与上游默认值语义冲突。
3. 显式 `temperature=0` 会被改为 `1.0`，这是本轮预期行为，因为上游限制优先于客户端采样偏好。
4. `UpstreamModelName` 覆盖本地模型名时应以真实上游为准，否则模型映射到 `kimi-k2.6` 的请求仍可能失败。

### 方案评审

采用最小 provider 兼容方案：在 `moonshot/adaptor.go` 内新增 `moonshotUpstreamModelName` 和 `isMoonshotTemperatureOneOnlyModel`，`ConvertOpenAIRequest` 根据上游模型名判断是否需要修正 temperature。使用 `common.GetPointer[float64](1.0)` 创建新指针，避免修改原指针指向的外部变量；不引入新依赖，也不改请求 marshal 逻辑。

验收方式：

1. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./relay/channel/moonshot` 覆盖 Moonshot adapter 行为。
2. `git diff --check` 确认无空白错误。
3. 使用 MCP 打开 `http://192.168.0.202:3003/` 确认页面仍可访问；若 MCP Chrome 仍不可用，则用 `curl` 访问主页和 `/api/status` 作为替代验证。

验证记录：

1. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./relay/channel/moonshot` 通过，覆盖 `kimi-k2.6` temperature 省略、允许值、显式零值、非目标模型、模型映射和未初始化 ChannelMeta fallback。
2. `git diff --check` 通过，无空白错误。
3. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用 `curl` 替代访问热更新站点。
4. `curl` 访问 `http://192.168.0.202:3003/` 和 `/api/status` 均返回 `HTTP/1.1 200 OK`，确认本轮 provider 兼容改动未影响 3003 页面与状态接口可用性。

## 本轮实施评审：Ollama 非流式 tool_calls 响应兼容

### 需求分析

new-api-main 的 Ollama adapter 已将 Ollama 原生 `message.tool_calls` 转换为 OpenAI 兼容响应中的 `message.tool_calls`，并在发生工具调用时把 `finish_reason` 修正为 `tool_calls`。NexusTok 当前流式路径已有部分工具调用转换，但非流式 `ollamaChatHandler` 只聚合文本和 reasoning，不会把上游 tool calls 写回 OpenAI 响应，导致支持工具调用的本地 Ollama 模型在非流式调用时表现为普通空文本结束，客户端无法继续执行工具。

本轮目标是把该能力转成 NexusTok 原生 provider 兼容能力：

1. 复用 Ollama 自身 DTO，将 `[]OllamaToolCall` 统一转换为 `[]dto.ToolCallResponse`。
2. 非流式聊天响应和多行 NDJSON fallback 均收集 `message.tool_calls`，写入 `dto.Message.ToolCalls`。
3. 非流式存在工具调用时，`finish_reason` 固定为 `tool_calls`，即使上游 `done_reason` 返回 `stop`。
4. 流式路径沿用同一转换 helper，并在已发送工具调用后把结束帧 `finish_reason` 修正为 `tool_calls`。
5. 触碰同文件时同步把实际 JSON marshal/unmarshal 调用改为 `common.*` 封装，仅保留 `encoding/json` 作为 `json.RawMessage` 类型来源。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Ollama 响应转换 | `relay/channel/ollama/stream.go` | 新增工具调用转换 helper；非流式收集 tool calls 并输出 OpenAI 兼容字段；流式结束原因与工具调用语义对齐。 |
| Ollama 单元测试 | `relay/channel/ollama/stream_test.go` | 新增非流式 compact JSON、pretty JSON fallback、多行 NDJSON 聚合、`arguments:null` 回退和流式 finish reason 测试。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. 该改动集中在 Ollama provider 响应转换层，不触碰通用 OpenAI adapter、请求分发、计费公式和数据库。
2. `tool_calls` 参数需要保持 JSON 字符串语义，转换时必须将 `arguments` marshal 为字符串；若上游缺少 arguments，则使用 `{}`，避免返回空字符串导致客户端 JSON 解析失败。
3. 非流式 `message.content` 可能为空，NexusTok 现有 `contentPtr` 会省略空 content；本轮不改变该行为，仅补充 tool calls。
4. 流式工具调用需要保留 `index`，非流式 message 级工具调用不带 `index`；测试需分别约束，避免混用导致客户端兼容性下降。
5. 同文件内已有直接 `encoding/json` marshal/unmarshal 调用；本轮替换为 `common.*` 时要避免改变 `json.RawMessage` 类型定义和已有 reasoning fallback 语义。

### 方案评审

采用局部 provider 兼容方案：在 `ollama/stream.go` 内新增 `ollamaToolCallsToOpenAI(toolCalls, startIndex, includeIndex)`，流式调用时 `includeIndex=true`，非流式调用时 `includeIndex=false`。非流式解析 compact JSON 和 pretty JSON fallback 时统一收集工具调用，最终写入 `msg.ToolCalls`；只要存在工具调用，完成原因改为 `constant.FinishReasonToolCalls`。实际 JSON 解析和序列化统一改为 `common.UnmarshalJsonStr`、`common.Unmarshal` 和 `common.Marshal`。

验收方式：

1. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./relay/channel/ollama` 覆盖 Ollama 非流式和流式 tool calls 行为。
2. `git diff --check` 确认无空白错误。
3. 使用 MCP 打开 `http://192.168.0.202:3003/` 确认页面仍可访问；若 MCP Chrome 仍不可用，则用 `curl` 访问主页和 `/api/status` 作为替代验证。

验证记录：

1. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./relay/channel/ollama` 通过，覆盖非流式 compact JSON、pretty JSON fallback、多行 NDJSON 聚合、`arguments:null` 回退为 `{}`、非流式不带 `index`、流式 tool call delta 带 `index` 和结束帧 `finish_reason=tool_calls`。
2. `rg -n "json\\.(Marshal|Unmarshal|NewDecoder|NewEncoder)" relay/channel/ollama/stream.go relay/channel/ollama/stream_test.go` 无结果，确认本轮触碰文件中的实际 JSON 编解码已切到 `common.*` 封装；`encoding/json` 仅保留作 `json.RawMessage` 类型来源。
3. `git diff --check` 通过，无空白错误。
4. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用 `curl` 替代访问热更新站点。
5. `curl` 访问 `http://192.168.0.202:3003/` 和 `/api/status` 均返回 `HTTP/1.1 200 OK`，确认本轮 provider 响应转换改动未影响 3003 页面与状态接口可用性。

## 本轮实施评审：视频任务时长上限与直连 image 标准化

### 需求分析

new-api-main 在异步视频任务入口增加了 `MaxTaskDurationSeconds=3600`，用于限制用户可控的 `duration` / `seconds`。该字段会进入 `OtherRatios["seconds"]`，影响任务预扣费和结算快照；如果没有边界，极端大值可能在后续额度计算中放大风险。NexusTok 当前任务入口会接受负数或超大时长，Sora、Gemini/Veo、Ali 等任务 provider 的计费估算也可能从 metadata 或历史任务数据中拿到绕过入口校验的时长。

new-api-main 同时修复了 `/v1/video/generations` 直连 JSON 请求中只传 `image` 字段时没有标准化到 `Images` 的问题。NexusTok 当前 `ValidateMultipartDirect` 只处理 `input_reference`，没有把单图 `image` 转成 `Images`，可能导致图生视频请求被误判为纯文本生成动作。

本轮目标是把这两个相邻任务入口优势转成 NexusTok 原生能力：

1. 在任务通用校验层统一限制 `duration` / `seconds` 为非负且不超过 3600 秒，正常缺省值继续由各 provider 使用现有默认。
2. `ValidateMultipartDirect` 将直连 JSON 的 `image` 标准化为 `Images`，保持与 `ValidateBasicTaskRequest` 的单图兼容行为一致。
3. 对历史 remix 数据、Gemini/Veo metadata、Ali metadata 等可能绕过通用入口校验的计费倍率路径做上限兜底。
4. 不修改任务数据库模型、任务轮询、任务状态机、表达式计费引擎和实际上游请求体字段。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 任务通用校验 | `relay/common/relay_utils.go` | 新增 `MaxTaskDurationSeconds` 和 `validateTaskDurationBounds`；直连 JSON `image` 标准化到 `Images`；普通任务与直连任务校验时拒绝负数或超过 3600 秒。 |
| remix 计费兜底 | `relay/relay_task.go` | 历史任务数据缺少 BillingContext 时，从旧 task data 解析出的 seconds 在写入 OtherRatios 前做上限钳制。 |
| task provider 计费兜底 | `relay/channel/task/gemini/billing.go`、`relay/channel/task/ali/adaptor.go` | Gemini/Veo 和 Ali 估算计费使用的时长倍率做 3600 秒兜底，防止 metadata 覆盖绕过通用校验。 |
| 任务校验测试 | `relay/common/relay_utils_test.go` | 覆盖直连 `image` 标准化、超大 duration、超大 seconds、负数 duration 和正常秒数。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. 入口超限请求会从“可能提交到上游”变成 400 本地错误，这是预期的安全边界变化；正常 1 到 3600 秒请求不受影响。
2. 缺省或非正正常化语义保持不变：未传时仍由 provider 适配器使用现有默认，`seconds<=0` 的默认回退逻辑不被本轮改写。
3. Gemini/Veo 和 Ali 的 metadata 兜底只钳制计费倍率，不改发送给上游的请求体；目的是防止计费乘数越界，不改变 provider 原有参数映射。
4. remix 历史任务缺少 BillingContext 时，旧 task data 中保存的极端 seconds 会被钳制到 3600；这只影响异常历史数据的重试/混剪预扣费，避免无界放大。
5. 已阅读 `pkg/billingexpr/expr.md`。本轮不修改表达式语法、变量归一化、预消费表达式计算或结算表达式计算，只保护进入任务 OtherRatios 的时长倍率边界。

### 方案评审

采用最小任务入口保护方案：在 `relay/common/relay_utils.go` 增加常量和校验 helper，`ValidateMultipartDirect` 与 `ValidateBasicTaskRequest` 在提示词校验后统一调用；直连 JSON 的单图 `image` 按现有 Basic 路径语义标准化为 `Images`。对不一定经过通用校验的计费兜底路径，使用同一上限常量做 `min`/条件钳制，保证所有 seconds 进入 `OtherRatios` 前有稳定上界。

验收方式：

1. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./relay/common ./relay/channel/task/gemini ./relay/channel/task/ali` 覆盖任务校验和 provider 编译回归。
2. `git diff --check` 确认无空白错误。
3. 使用 MCP 打开 `http://192.168.0.202:3003/` 确认页面仍可访问；若 MCP Chrome 仍不可用，则用 `curl` 访问主页和 `/api/status` 作为替代验证。

验证记录：

1. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./relay/common ./relay/channel/task/gemini ./relay/channel/task/ali` 通过，覆盖直连 `image` 标准化、超大 `duration`、超大 `seconds`、负数 `duration`、正常 `seconds`、Gemini/Veo 时长钳制和 Ali metadata 时长钳制。
2. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./relay/channel/task/...` 通过，确认所有 task provider 子包编译回归正常。
3. `GOCACHE=/tmp/nexustok-go-build go test -count=1 ./relay` 通过，确认 remix 兜底所在 relay 包编译正常。
4. `git diff --check` 通过，无空白错误。
5. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用 `curl` 替代访问热更新站点。
6. `curl` 访问 `http://192.168.0.202:3003/` 和 `/api/status` 均返回 `HTTP/1.1 200 OK`，确认本轮任务校验和计费倍率边界改动未影响 3003 页面与状态接口可用性。

## 本轮实施评审：默认前端安全富文本渲染

### 需求分析

new-api-main 默认前端提供了 `HtmlContent`、`RichContent` 和基于 DOMPurify 的 Markdown/HTML 安全渲染路径，并在首页自定义内容、About/Legal 文档、通知公告等入口统一消费。NexusTok 当前 `Markdown` 组件启用了 `rehypeRaw`，公告、FAQ、通知、首页自定义内容、About/Legal 等管理员可配置或远端内容会允许原始 HTML 进入渲染链；About/Legal 还存在直接 `dangerouslySetInnerHTML`。这些入口都属于管理员或远端可控内容，一旦配置被误填、导入外部 HTML 或遇到恶意链接，浏览器侧风险会被放大。

本轮目标是把 new-api-main 的安全富文本优势转成 NexusTok 原生前端基础能力：

1. 新增 `HtmlContent`，所有显式 HTML 先经 DOMPurify 清洗；`target="_blank"` 链接强制补 `noopener noreferrer`，iframe 清除 `srcdoc` 并加 sandbox/referrerpolicy。
2. 新增 `RichContent` 统一 Markdown/HTML 入口；Markdown 默认不再执行原始 HTML，HTML 只在调用方显式选择 `mode="html"` 时渲染。
3. 新增 `content-format` helper，统一判断 HTTP(S) URL 与疑似 HTML，替换 About/Legal/Home 中重复且语义不一致的本地判断。
4. 先接入公开内容、通知公告和 Dashboard 公告详情等高风险入口；不在本轮重构 Playground response renderer，以免同时改变模型输出渲染和消息流行为。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 富文本基础组件 | `web/default/src/components/html-content.tsx`、`web/default/src/components/rich-content.tsx`、`web/default/src/components/ui/markdown.tsx` | 新增 DOMPurify HTML 清洗入口；Markdown 移除 `rehypeRaw` 原始 HTML 执行；保留现有 prose 样式。 |
| 内容格式 helper | `web/default/src/lib/content-format.ts` | 统一 URL/HTML 判定，避免页面各自实现。 |
| 公开内容入口 | `web/default/src/features/home/index.tsx`、`web/default/src/features/about/index.tsx`、`web/default/src/features/legal/legal-document.tsx` | URL 继续 iframe/外链；Markdown 和 HTML 改走 `RichContent`。 |
| 通知与公告入口 | `web/default/src/components/notification-dialog.tsx`、`web/default/src/features/dashboard/components/overview/announcement-detail-dialog.tsx`、`web/default/src/features/dashboard/components/overview/faq-panel.tsx` | 通知、公告、FAQ 内容改走 `RichContent`；Markdown 不执行 raw HTML，疑似 HTML 先经 DOMPurify 清洗再 inline 渲染。 |
| 页脚 HTML 入口 | `web/default/src/components/layout/components/footer.tsx` | 自定义页脚 HTML 从直接 `dangerouslySetInnerHTML` 改为 `HtmlContent`，保留现有布局并补齐清洗边界。 |
| 依赖 | `web/default/package.json`、`web/default/bun.lock` | 将当前锁文件中已有的 DOMPurify 提升为直接依赖，并移除不再被源码使用的直接 `rehype-raw` 依赖，明确安全渲染组件的依赖来源。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. 这是默认前端渲染层改动，不触碰后端 API、数据库、计费和权限边界。
2. Markdown 不再执行原始 HTML，可能改变少量把 HTML 混在 Markdown 中的旧配置显示方式；显式 HTML 内容会通过 `RichContent mode="html"` 渲染并被清洗，常见 About/Legal/Home HTML 页面仍可展示。
3. DOMPurify 会移除危险标签和属性；如果管理员配置依赖脚本、`srcdoc` 或不安全事件属性，本轮会主动阻断，这是预期安全边界。
4. isolated HTML 会复制当前样式表到 shadow root，可能增加少量渲染成本；本轮只在 About/Legal/Home 等完整内容区域使用 isolated，通知/FAQ 保持 Markdown。
5. 遵循现有 Swiss 运维界面，不新增可见文案、不引入营销式说明，也不改信息层级。

### 方案评审

采用小步安全基础设施方案：直接吸收 new-api-main 的 `HtmlContent`/`RichContent` 思路，但保留 NexusTok 当前 `Markdown` 视觉 class 和 Base UI 组件体系。所有 HTML 必须显式通过 `RichContent mode="html"` 或 `HtmlContent` 进入 DOMPurify；普通 Markdown 继续用 ReactMarkdown + GFM，但不启用 `rehypeRaw`。About/Legal/Home 通过 `isHttpUrl`、`isLikelyHtml` 选择 URL、HTML 或 Markdown，完整页面 HTML 使用 isolated shadow root；通知、公告、FAQ 体量较小，疑似 HTML 仅 inline 清洗渲染，不允许再借 Markdown raw 链路执行。

验收方式：

1. `cd web/default && ./node_modules/.bin/tsc -b` 覆盖 TypeScript 类型。
2. `cd web/default && ./node_modules/.bin/rsbuild build` 覆盖生产构建。
3. `git diff --check` 确认无空白错误。
4. 使用 MCP 打开 `http://192.168.0.202:3003/` 检查页面；若 MCP Chrome 仍不可用，则用 `curl` 访问主页和 `/api/status` 作为替代验证。

验证记录：

1. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 `HtmlContent`、`RichContent`、`Markdown breaks` 和所有页面接入点类型正确。
2. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，确认新增 DOMPurify 直接依赖、移除直接 `rehype-raw` 后默认前端生产构建正常。
3. `git diff --check` 通过，无空白错误。
4. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求替代页面验证。
5. `curl -H 'Cache-Control: no-cache' http://192.168.0.202:3003/` 返回 `HTTP/1.1 200 OK`，`/api/status` 也返回 `HTTP/1.1 200 OK`，确认热更新站点主页和状态接口可用。
6. 从 3003 拉取 `/static/js/index.js` 及相关 async chunk，确认当前服务出的 chunk 中包含 `FORCE_BODY`、`allow-popups-to-escape-sandbox`、`srcdoc` 清理、`custom-home-content`、`custom-footer` 等本轮新增渲染逻辑特征，且未命中 `rehypeRaw` / `rehype-raw`。
7. `./node_modules/.bin/eslint` 针对本轮触碰的前端文件运行无错误；`src/components/ui/markdown.tsx` 按项目 ignore 规则被跳过并仅产生 warning。
8. 全量 `bun run lint` 和 `bun run copyright:check` 仍受既有未触碰文件影响失败，主要涉及 `risk-acknowledgement-dialog.tsx`、`flow-charts.tsx`、`system-info/*`、`usage-logs/*` 等旧问题；本轮未将这些无关修复混入提交。

## 本轮实施评审：Playground AI 响应受控渲染

### 需求分析

new-api-main 默认前端已经把模型响应渲染从第三方黑盒组件迁移为本地可审计的 AST 渲染链路：`Response` 使用 `stream-markdown-parser` 解析 Markdown，再由 `response-renderer-*` 文件按节点类型渲染文本、段落、代码块、图片、表格、脚注、GitHub alert、`details/summary` 等内容。HTML block/inline 默认文本化，只对白名单内的 `details` 走受控组件；图片 URL 经过 `sanitizeImageSrc`；超长响应超过 20,000 字符时降级为纯文本，避免大响应解析拖慢页面。

NexusTok 当前 `web/default/src/components/ai-elements/response.tsx` 仍直接包装 `streamdown`，只做自定义包装标签和 `<think>` 标签移除。虽然 `streamdown` 自身带有安全依赖，但项目侧无法逐节点审计 HTML、图片、脚注、表格、details 等行为，也没有本地解析长度上限。Playground 是管理员和用户调试上游模型的重要页面，响应内容由模型或上游返回，可控性和降级策略应成为 NexusTok 原生能力。

本轮目标：

1. 引入 new-api-main 的本地 Response 解析和渲染思路，保留 NexusTok 现有 `<Response>{content}</Response>` 调用 API。
2. HTML block/inline 默认文本化，仅 `details/summary` 走受控渲染；图片 URL 通过 parser 提供的 `sanitizeImageSrc` 过滤，失败时降级显示 alt 文本。
3. 支持表格、脚注、GitHub alert 风格 blockquote、checkbox、硬换行、水平线和基础数学文本降级；代码块复用 NexusTok 当前 `CodeBlock`，不在本轮升级 CodeMirror 代码块。
4. 对超过 20,000 字符的响应不解析 AST，直接文本展示，避免极端输出影响 Playground 交互。
5. 补齐新增可见/可访问文案的六语 i18n 翻译。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Response 主入口 | `web/default/src/components/ai-elements/response.tsx` | 从 `streamdown` wrapper 改为本地 `stream-markdown-parser` AST 渲染；保留 `children`、`className`、`final` API。 |
| Response 解析和类型 | `web/default/src/components/ai-elements/response-content.ts`、`response-types.ts`、`response-node-guards.ts` | 统一清理 AI wrapper 标签、规范 Markdown 示例 fence、拆分 footnote/body 节点，并提供节点类型守卫。 |
| Response 节点渲染 | `web/default/src/components/ai-elements/response-renderer*.tsx` | 受控渲染段落、标题、列表、代码块、链接、图片、表格、脚注、alert、details；HTML 默认文本化。 |
| 依赖 | `web/default/package.json`、`web/default/bun.lock` | 新增 `stream-markdown-parser`，移除 Response 不再直接使用的 `streamdown`。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增 `Image not available`、`Back to footnote {{id}} reference`、`Note`、`Tip`、`Important` 等文案。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式，并补充落地清单。 |

### 风险评估

1. 本轮只改 AI 响应展示组件，不改 Playground 消息流、请求参数、存储 schema、Relay 接口、计费和权限。
2. Markdown 渲染行为会从 `streamdown` 的内置策略变为 NexusTok 本地策略，少量边缘 Markdown 样式可能有差异；收益是 HTML、图片、details、脚注等行为可审计。
3. new-api-main 的 `renderCodeBlock` 依赖新版 `CodeBlock` 的折叠、toolbar、title 等 props；NexusTok 当前 `CodeBlock` 不兼容。本轮采用适配版 `renderCodeBlock`，继续使用当前 Shiki 代码块和复制按钮，避免引入 CodeMirror 依赖与更大 UI 变更。
4. 数学节点本轮按代码/预格式化文本降级，不引入 KaTeX；这保持依赖克制，避免把 Markdown 能力扩展和安全渲染混成一个大改。
5. 新增 `stream-markdown-parser` 后需要完整跑 TypeScript 和生产构建；如果 parser 类型与当前 React 版本存在差异，优先通过本地类型守卫和适配层处理，不修改调用页面。

### 方案评审

采用“本地 AST 渲染器 + 代码块适配”的小步原生化方案：复制 new-api-main 的 Response 解析结构和节点渲染拆分，版权头改为 c1cada；`response-renderer-blocks.tsx` 中的代码块渲染改成 NexusTok 当前 `CodeBlock` 支持的 `code/language/showLineNumbers` props；表格、提示块和分隔线落到当前默认前端已有的 `Table`、`Alert`、`Separator` 基础组件上；保留 `Response` 的 `memo` 和 `final` 语义。依赖层仅新增 `stream-markdown-parser` 并移除直接 `streamdown`，不迁移新版 `CodeBlock`、不新增 CodeMirror/KaTeX、不重构 Playground 页面目录结构。

验收方式：

1. `cd web/default && bun run i18n:sync` 确认新增 key 进入六语翻译文件并保持顺序。
2. `cd web/default && ./node_modules/.bin/tsc -b` 覆盖 Response 类型和调用点。
3. `cd web/default && ./node_modules/.bin/rsbuild build` 覆盖生产构建和 chunk 分包。
4. 针对本轮触碰文件运行 `eslint`，并记录全量 lint 中的既有问题是否仍与本轮无关。
5. 使用 MCP 打开 `http://192.168.0.202:3003/playground` 检查页面；若 MCP Chrome 仍不可用，则用 `curl` 访问 3003 主页、`/api/status`，并拉取相关 chunk 确认新 Response 代码已热更新。

验证记录：

1. `cd web/default && bun run i18n:sync` 通过，六语 locale 已包含新增的图片降级、脚注返回和 alert 标题文案。
2. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 `stream-markdown-parser` 节点类型、本地守卫、`Response final` 调用点和 `CodeBlock` 未知语言回退类型正确。
3. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，确认新增依赖、移除直接 `streamdown` 后生产构建正常。
4. 针对本轮触碰文件运行 `./node_modules/.bin/eslint ...` 通过；`git diff --check` 通过，无空白错误。
5. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；因此本轮按约定使用真实 3003 HTTP 请求替代页面验证。
6. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP/1.1 200 OK`，确认指定热更新站点入口、状态接口和 Playground 路由可访问。
7. 从 `http://192.168.0.202:3003` 拉取 `/static/js/index.js`、`/static/js/async/2459.js`、`/static/js/async/5395.js`，确认线上静态资源包含 `nexustok-response`、`Image not available`、`Back to footnote`、`footnote-ref` 等本轮 Response renderer 特征，说明热更新页面已加载新代码。

## 本轮实施评审：Playground 流式错误处理底座

### 需求分析

new-api-main 的 Playground 将流式响应解析、SSE readyState 错误判断和普通请求错误解析拆到了 `lib/streaming/*`，`use-stream-request` 只负责连接生命周期和回调编排。NexusTok 当前仍把 JSON chunk 解析、错误 payload 解析、HTTP readyState 判断和 `isStreaming` 状态直接写在 hook 内；同时 `isStreaming` 由 ref 推导，不会主动触发 React 重渲染，可能导致输入框 Stop/Send 状态、消息动作禁用状态和实际 SSE 生命周期短暂不同步。

本轮目标是把 new-api-main 的低风险结构优势转成 NexusTok 原生 Playground 底座：

1. 新增本地 `stream-utils` 与 `request-error-utils`，统一解析 SSE `[DONE]`、delta 更新、错误 payload 和非流式请求错误。
2. `useStreamRequest` 改为显式 `useState` 跟踪 `isStreaming`，开始连接、关闭、错误和停止时都同步更新 UI 状态。
3. 新请求开始前主动关闭旧 SSE，避免用户连续发送或异常重试时出现旧连接残留。
4. 不修改 `/pg/chat/completions` payload、不改消息存储 schema、不迁移整个 Playground 目录结构，不引入新依赖。
5. 错误 Alert 只做必要清理：fallback 文案走 i18n，间距使用 `flex/gap`；模型价格错误按钮跳转到 `/system-settings/billing/model-pricing`，避免继续指向 classic `/console` 路径。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 流式解析工具 | `web/default/src/features/playground/lib/stream-utils.ts` | 新增 `[DONE]`、chunk delta、readyState 和 stream error payload 解析 helper。 |
| 请求错误工具 | `web/default/src/features/playground/lib/request-error-utils.ts` | 新增非流式请求错误 message/code 解析 helper，兼容 `{message}` 与 `{error:{message,code}}` 响应。 |
| hook 生命周期 | `web/default/src/features/playground/hooks/use-stream-request.ts` | 改为响应式 `isStreaming`，统一 close/error/stop 路径，复用 stream helper。 |
| 非流式错误 | `web/default/src/features/playground/hooks/use-chat-handler.ts` | catch 分支复用 request error helper，避免 stream/non-stream 错误解析分叉。 |
| 错误展示 | `web/default/src/features/playground/components/message-error.tsx` | fallback 文案进入 i18n，移除 raw orange 和 `space-y-*` 样式，模型价格错误的设置入口改向默认前端模型价格页。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增 `An unknown error occurred` 翻译。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. 本轮只改默认前端 Playground 的客户端状态和错误解析，不触碰后端 Relay、计费、权限、数据库和模型请求 DTO。
2. `isStreaming` 从 ref 推导变成 state 后，UI 会更及时进入/退出生成态；正常请求 payload 与 SSE 消息处理不变，风险集中在关闭连接时机。
3. 新请求前关闭旧连接会中断残留 SSE，这是预期保护；不会影响用户点击 Stop 的现有语义，Stop 仍把当前 assistant 消息 finalize 为 complete。
4. 错误解析会优先展示上游 `error.message` 或业务 `message`，比旧逻辑更完整；如果错误体不是 JSON，仍回退 raw data 或通用错误。
5. 本轮不新增可执行 HTML、不改 Markdown renderer、不改模型输出内容处理。

### 方案评审

采用“工具函数抽离 + hook 生命周期修正”的最小方案：参考 new-api-main 的 `stream-utils`、`request-error-utils` 和 `use-stream-request` 结构，但保留 NexusTok 当前 API endpoint、类型、toast 和消息更新函数。`useStreamRequest` 在 `sendStreamRequest` 前关闭旧连接，创建新 SSE 后设置 `isStreaming=true`，所有关闭路径统一经过 `closeActiveStream`，确保输入控件和消息动作响应式更新。非流式 catch 改用同一错误解析 helper，避免后续维护两套错误字段兼容逻辑。

验收方式：

1. `cd web/default && bun run i18n:sync` 确认新增 fallback key 进入六语翻译文件。
2. `cd web/default && ./node_modules/.bin/eslint src/features/playground/lib/stream-utils.ts src/features/playground/lib/request-error-utils.ts src/features/playground/hooks/use-stream-request.ts src/features/playground/hooks/use-chat-handler.ts src/features/playground/components/message-error.tsx` 定向检查本轮文件。
3. `cd web/default && ./node_modules/.bin/tsc -b` 覆盖 Playground 类型、hook 回调和新增工具导出。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 覆盖生产构建。
5. 使用 MCP 打开 `http://192.168.0.202:3003/playground` 检查页面；若 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取 chunk 确认 `parseStreamMessageUpdates`、`parseRequestErrorDetails` 等特征已热更新。

验证记录：

1. `cd web/default && bun run i18n:sync` 通过，新增 `An unknown error occurred` 已进入 en、zh、fr、ja、ru、vi 六语 locale。
2. `cd web/default && ./node_modules/.bin/eslint src/features/playground/lib/stream-utils.ts src/features/playground/lib/request-error-utils.ts src/features/playground/lib/index.ts src/features/playground/hooks/use-stream-request.ts src/features/playground/hooks/use-chat-handler.ts src/features/playground/components/message-error.tsx` 通过。
3. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认新增工具导出、hook state、SSE 事件类型和 request error 解析类型正确。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，确认默认前端生产构建正常。
5. `git diff --check` 通过，无空白错误。
6. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求替代页面验证。
7. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status`、`/playground` 和 `/system-settings/billing/model-pricing` 均返回 `HTTP/1.1 200 OK`。
8. 容器 `nexustok-api-hot` 日志显示热更新已发布前端 dist、重新构建并重启后端；从 3003 拉取 `/static/js/index.js` 与 `/static/js/async/9979.js`，确认线上静态资源包含 `An unknown error occurred` 和 `/system-settings/billing/model-pricing` 等本轮可观察特征。生产构建会压缩函数名，因此不依赖 helper 函数名作为线上特征。

## 本轮实施评审：Playground 错误消息动作原生化

### 需求分析

new-api-main 的 Playground 在 assistant 消息失败时，会在错误卡片下方提供专门的 `Retry`、`Edit` 和 `Delete` 动作。这个设计把“重试失败请求”“回到上一条用户 prompt 修改后再发”“清理错误消息”三种恢复路径直接放在错误现场，用户不需要先理解普通消息动作对错误消息的含义。NexusTok 当前错误消息仍复用普通 `MessageActions`，可以删除错误消息，也可能对空内容 assistant 消息隐藏复制/编辑等动作，但缺少面向失败态的“编辑上一条 user prompt”入口，错误恢复链路不够直接。

本轮目标是吸收 new-api-main 的低风险交互优势，但保持 NexusTok 现有 Playground 结构：

1. 新增默认前端本地 `MessageErrorActions`，只暴露 `Retry`、`Edit`、`Delete` 三个错误恢复动作。
2. 错误消息的 `Retry` 继续调用现有 `onRegenerateMessage(errorMessage)`，不改变请求 payload、消息状态机和 SSE 生命周期。
3. 错误消息的 `Edit` 指向该错误消息之前最近的一条 user 消息，让用户直接修改导致失败的 prompt；没有可编辑 user 消息时隐藏该动作。
4. 错误消息的 `Delete` 继续调用现有 `onDeleteMessage(errorMessage)`，不新增确认弹窗，不修改删除语义。
5. 不迁移 new-api-main 的完整 message 目录、源码预览、移动端动作菜单、历史消息裁剪和本地存储 schema，避免把一个恢复按钮切片扩大成 Playground 重构。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 错误动作组件 | `web/default/src/features/playground/components/message-error-actions.tsx` | 新增错误消息专用按钮组，复用当前 `MessageActionButton` 和已有 `Retry/Edit/Delete` i18n key。 |
| 消息渲染 | `web/default/src/features/playground/components/playground-chat.tsx` | 错误消息分支改用 `MessageErrorActions`；新增查找上一条 user 消息的轻量 helper；顺手把触碰到的旧 `space-y-*` 编辑布局改为 `flex/gap`。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 已有 `Retry`、`Edit`、`Delete` 翻译；本轮仅运行同步脚本确认无缺失。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. 本轮只改默认前端 Playground 的错误消息 UI，不触碰后端 Relay、计费、权限、数据库、模型请求 DTO 和流式解析工具。
2. `Retry` 复用现有 regenerate 回调，风险集中在按钮出现时机；禁用态跟随 `isGenerating`，避免生成中重复发起恢复动作。
3. `Edit` 改为编辑上一条 user 消息，而不是编辑错误 assistant 消息本身，这是 new-api-main 的关键体验差异；实现时必须从当前完整 messages 数组按索引向前查找，不能假定错误消息一定紧跟 user 消息。
4. 错误分支不再展示普通 `MessageActions`，避免同一错误卡片同时出现两套删除/编辑入口；普通完成消息、流式消息和用户消息动作不变。
5. 本轮不新增翻译 key，但新增 `t()` 调用后仍需跑 `bun run i18n:sync` 和定向 ESLint，确认所有 locale key 已覆盖。

### 方案评审

采用“专用错误动作组件 + 本地上一条 user helper”的最小原生化方案：`MessageErrorActions` 放在当前 `components/` 目录，与现有 `MessageActions` 并列；按钮继续使用 Playground 已封装的 `MessageActionButton`，保持尺寸、tooltip、destructive 样式和现有动作体系一致。`playground-chat.tsx` 在 `message.status === 'error'` 分支中计算 `previousUserMessage`，只把错误恢复动作传给错误卡片，不改普通消息内容渲染。设计方向延续当前默认前端的工具台式紧凑布局，使用已有 icon button 和 tooltip，不新增卡片、说明文案或营销式提示。

验收方式：

1. `cd web/default && bun run i18n:sync` 确认 `Retry/Edit/Delete` 在六语 locale 中无缺失并保持文件顺序。
2. `cd web/default && ./node_modules/.bin/eslint src/features/playground/components/message-error-actions.tsx src/features/playground/components/playground-chat.tsx` 定向检查本轮文件。
3. `cd web/default && ./node_modules/.bin/tsc -b` 覆盖新增组件 props、上一条 user helper 和 Playground 消息类型。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 覆盖生产构建和 chunk 分包。
5. 使用 MCP 打开 `http://192.168.0.202:3003/playground` 检查页面与控制台；若 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 确认 `Retry` 错误动作相关代码已热更新。

验证记录：

1. `cd web/default && bun run i18n:sync` 通过，`Retry`、`Edit`、`Delete` 六语 locale 已存在，未产生额外翻译文件变更。
2. `cd web/default && ./node_modules/.bin/eslint src/features/playground/components/message-error-actions.tsx src/features/playground/components/playground-chat.tsx` 通过。
3. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认新增错误动作组件、上一条 user 消息查找 helper 和 Playground 消息回调类型正确。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，确认生产构建和 Playground lazy chunk 正常生成；`git diff --check` 通过。
5. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求替代页面验证。
6. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP/1.1 200 OK`。
7. 从 3003 拉取 `/static/js/async/3786.js`，确认线上 chunk 包含 `onEditPrompt`、`onRetry`、`Retry` 和 `flex flex-wrap items-center gap-0.5 pt-2` 等本轮错误动作特征；线上 chunk 与本地 `web/default/dist/static/js/async/3786.js` 完全一致。

## 本轮实施评审：Playground 输入区恢复能力原生化

### 需求分析

new-api-main 的 Playground 输入区已经把文本提交、模型/分组选择、附件/搜索工具和会话清理入口拆成独立组件，并在空会话时提供 starter prompts。NexusTok 当前仍把这些逻辑集中在 `playground-input.tsx`：输入提交校验、选择器禁用、附件 toast、搜索 toast 和底部建议条都混在一起；用户如果想清空本浏览器保存的 Playground 会话，只能逐条删除消息；空会话区域也没有说明当前页面可以直接测试模型或选择 starter prompt。

本轮目标是把 new-api-main 的输入恢复体验转成 NexusTok 原生能力，同时保持当前 Relay 调试链路稳定：

1. 抽出输入提交和按钮状态 helper，避免后续继续把 selector、submit、stop 的禁用规则散落在组件 JSX 中。
2. 抽出附件/搜索工具 helper，统一 toast 文案来源，并保留当前“功能开发中”的占位行为，不引入真实文件上传。
3. 新增清空会话按钮，使用现有 `ConfirmDialog` 二次确认后调用 `clearMessages()`，只清理本浏览器 `playground_messages`，不影响服务端数据。
4. 新增空会话 starter prompts，用户点击后直接走现有 `handleSendMessage(prompt)`，不新增请求字段、不改消息存储 schema。
5. 移除旧输入区中 raw icon color suggestion，实现所有新增可见文案走 i18n 六语翻译。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 输入 helper | `web/default/src/features/playground/lib/input-control-utils.ts`、`web/default/src/features/playground/lib/input-tool-utils.ts`、`web/default/src/features/playground/lib/index.ts` | 新增提交文本、选择器/按钮禁用、附件动作和搜索动作 notice helper。 |
| 输入组件 | `web/default/src/features/playground/components/playground-input.tsx`、`playground-input-controls.tsx`、`playground-input-tools.tsx` | 输入区拆成容器、控制区和工具区；新增清空会话确认入口；保留模型/分组选择和 Stop/Send 语义。 |
| 空状态 | `web/default/src/features/playground/components/playground-empty-state.tsx`、`playground-chat.tsx`、`index.tsx` | 空消息时显示 starter prompts；点击后复用现有发送函数；输入区接收 `hasMessages` 和 `onClearMessages`。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增 starter prompts、清空会话、空状态说明等六语翻译。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式。 |

### 风险评估

1. 本轮只改默认前端 Playground UI 和本地状态操作，不触碰后端 Relay、计费、权限、数据库、模型请求 DTO、SSE 解析和本地存储 key。
2. 清空会话按钮会删除当前浏览器保存的 Playground 消息；必须有二次确认，并且生成中禁用，避免用户在 SSE 进行中清掉仍在更新的消息数组。
3. starter prompt 点击会直接发起一次现有 chat 请求，这是用户主动动作；它复用 `handleSendMessage`，因此请求体、模型选择、分组选择和参数启用规则保持原样。
4. 输入区拆分可能影响移动端工具栏排布；需要生产构建后访问 3003，并用线上 chunk 特征确认热更新。若 MCP 可用，应额外检查控制台和页面布局。
5. 本轮会新增多个 i18n key，必须跑 `bun run i18n:sync`、定向 ESLint、TypeScript 和生产构建；若发现旧 locale 存在未翻译存量，不纳入本轮扩散修复。

### 方案评审

采用“输入区组件拆分 + 空状态最小接线”的方案：参考 new-api-main 的 `PlaygroundInputControls`、`PlaygroundInputTools` 和 `PlaygroundEmptyState` 结构，但版权头、注释和文案按 NexusTok 规范重写；按钮继续使用当前 `PromptInputButton`、`DropdownMenu`、`Tooltip`、`ConfirmDialog` 和 `ModelGroupSelector`，不新增依赖。`playground-chat.tsx` 仅新增 `onSelectPrompt`，空消息时渲染空状态；`index.tsx` 将 `clearMessages` 和 `handleSendMessage` 传入输入区/空状态。设计保持当前默认前端工具台语义：紧凑、扫描友好、无营销化说明，避免把输入区改成落地页。

验收方式：

1. `cd web/default && bun run i18n:sync` 确认新增 UI key 进入 en、zh、fr、ja、ru、vi。
2. 针对本轮触碰文件运行定向 ESLint。
3. `cd web/default && ./node_modules/.bin/tsc -b` 覆盖新增 props、helper 导出和 i18n 调用。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 覆盖生产构建和 Playground lazy chunk。
5. 使用 MCP 打开 `http://192.168.0.202:3003/playground` 检查空状态、清空确认弹窗、控制台错误和网络请求；若 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 确认 `Clear chat history`、`Start a playground chat` 等特征已热更新。

验证记录：

1. `cd web/default && node scripts/add-playground-input-i18n.mjs && bun run i18n:sync` 通过，随后已删除临时脚本；en、zh、fr、ja、ru、vi 各新增 9 个输入区/空状态翻译 key。
2. `cd web/default && ./node_modules/.bin/eslint src/features/playground/lib/input-control-utils.ts src/features/playground/lib/input-tool-utils.ts src/features/playground/lib/index.ts src/features/playground/components/playground-input.tsx src/features/playground/components/playground-input-controls.tsx src/features/playground/components/playground-input-tools.tsx src/features/playground/components/playground-empty-state.tsx src/features/playground/components/playground-chat.tsx src/features/playground/index.tsx` 通过。
3. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认新增输入 helper、组件 props、空状态接线和 i18n 调用类型正确。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，确认生产构建和 Playground lazy chunk 正常生成；`git diff --check` 通过。
5. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求替代页面验证。
6. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP/1.1 200 OK`。
7. 从 3003 拉取 `/static/js/index.js` 与 `/static/js/async/3854.js`，确认线上资源包含 `Clear chat history`、`Conversation cleared`、`Start a playground chat`、`Analyze data` 等本轮特征；旧 `/static/js/async/3786.js` 已返回 404，说明 Playground chunk 已切换到新构建。
8. 线上 `/static/js/async/3854.js` 与本地 `web/default/dist/static/js/async/3854.js` 完全一致，确认热更新页面加载的是本轮构建产物。

| 日期 | 能力 | 文件 | 说明 |
|------|------|------|------|
| 2026-07-09 | Playground 输入区恢复能力 | `web/default/src/features/playground/components/{playground-input.tsx,playground-input-controls.tsx,playground-input-tools.tsx,playground-empty-state.tsx,playground-chat.tsx}`、`web/default/src/features/playground/lib/{input-control-utils.ts,input-tool-utils.ts,index.ts}`、`web/default/src/features/playground/index.tsx`、`web/default/src/i18n/locales/*.json` | 原生化 new-api-main 的输入区拆分、空会话 starter prompts 和清空本地会话能力；新增清空确认弹窗，输入工具状态集中到 helper，移除旧 raw color 建议按钮，所有新增文案补齐六语翻译。 |
| 2026-07-09 | Playground 错误消息动作 | `web/default/src/features/playground/components/{message-error-actions.tsx,playground-chat.tsx}` | 原生化 new-api-main 的失败态恢复入口，错误 assistant 消息下方显示专用 Retry/Edit/Delete 动作；Retry 复用 regenerate，Edit 定位到上一条 user prompt，Delete 清理错误消息，普通消息动作和请求协议保持不变。 |
| 2026-07-09 | Playground 流式错误处理底座 | `web/default/src/features/playground/lib/{stream-utils.ts,request-error-utils.ts,index.ts}`、`web/default/src/features/playground/hooks/{use-stream-request.ts,use-chat-handler.ts}`、`web/default/src/features/playground/components/message-error.tsx`、`web/default/src/i18n/locales/*.json` | 原生化 new-api-main 的 Playground stream/request error 工具拆分，SSE 连接状态改为响应式 `isStreaming`，新请求前关闭旧流，统一解析流式和非流式错误；模型价格错误设置入口改向默认前端模型价格页，并补齐错误 fallback 六语翻译。 |
| 2026-07-09 | Playground AI 响应受控渲染 | `web/default/src/components/ai-elements/{response.tsx,response-content.ts,response-types.ts,response-node-guards.ts,response-renderer*.tsx}`、`web/default/src/components/ai-elements/code-block.tsx`、`web/default/src/features/playground/components/playground-chat.tsx`、`web/default/src/i18n/locales/*.json`、`web/default/package.json`、`web/default/bun.lock` | 原生化 new-api-main 的 AST Response renderer，移除直接 `streamdown`，由 `stream-markdown-parser` 解析并本地受控渲染表格、脚注、图片、alert、details、checkbox 和代码块；HTML 默认文本化、图片 URL 过滤、超过 20,000 字符纯文本降级，Playground streaming 消息按 `final` 状态解析。 |
| 2026-07-09 | 默认前端安全富文本渲染 | `web/default/src/components/html-content.tsx`、`web/default/src/components/rich-content.tsx`、`web/default/src/components/ui/markdown.tsx`、`web/default/src/lib/content-format.ts`、`web/default/src/features/home/index.tsx`、`web/default/src/features/about/index.tsx`、`web/default/src/features/legal/legal-document.tsx`、`web/default/src/components/notification-dialog.tsx`、`web/default/src/features/dashboard/components/overview/announcement-detail-dialog.tsx`、`web/default/src/features/dashboard/components/overview/faq-panel.tsx`、`web/default/src/components/layout/components/footer.tsx`、`web/default/package.json`、`web/default/bun.lock` | 原生化 new-api-main 的 `HtmlContent`/`RichContent` 安全渲染能力，Markdown 移除 raw HTML 执行；首页、About、Legal、通知、公告、FAQ 和自定义页脚 HTML 统一进入 DOMPurify 清洗链路，完整 HTML 页面使用 shadow root 隔离，同时移除源码不再使用的直接 `rehype-raw` 依赖。 |
| 2026-07-09 | 视频任务时长上限与直连 `image` 标准化 | `relay/common/relay_utils.go`、`relay/relay_task.go`、`relay/channel/task/gemini/billing.go`、`relay/channel/task/ali/adaptor.go`、`relay/common/relay_utils_test.go`、`relay/channel/task/gemini/billing_test.go`、`relay/channel/task/ali/adaptor_test.go` | 任务入口拒绝负数和超过 3600 秒的 `duration`/`seconds`，直连 JSON `image` 会标准化为图生视频输入；历史 remix、Gemini/Veo metadata 和 Ali metadata 进入 OtherRatios 前统一钳制时长倍率。 |
| 2026-07-09 | Ollama 非流式 `tool_calls` 响应兼容 | `relay/channel/ollama/stream.go`、`relay/channel/ollama/stream_test.go` | Ollama 非流式响应会把上游 `message.tool_calls` 转为 OpenAI 兼容 `message.tool_calls`，保留工具参数显式零值，空参数回退 `{}`；存在工具调用时非流式和流式结束原因统一为 `tool_calls`。 |
| 2026-07-09 | Moonshot `kimi-k2.6` temperature 兼容 | `relay/channel/moonshot/adaptor.go`、`relay/channel/moonshot/adaptor_test.go` | Moonshot OpenAI 兼容请求在真实上游模型为 `kimi-k2.6` 且客户端显式传入非 `1.0` temperature 时修正为 `1.0`；未传字段继续省略，非目标模型保持原值。 |
| 2026-07-09 | SMTP NTLM 自适应认证 | `common/email.go`、`common/email_ntlm_auth.go`、`common/email_ntlm_auth_test.go`、`go.mod`、`go.sum` | 邮件发送路径新增 PLAIN/LOGIN/NTLM 自适应认证，标准 SMTP 保持 PLAIN，强制 LOGIN 和 Outlook 兼容语义保留，企业 SMTP 仅开放 `AUTH NTLM` 时可完成 NTLM 协商。 |
| 2026-07-09 | 注册兼容路由与已登录跳转 | `web/default/src/routes/(auth)/register.tsx`、`web/default/src/routes/(auth)/sign-up.tsx`、`web/default/src/routeTree.gen.ts` | 默认前端新增 `/register` 到 `/sign-up` 的 replace 跳转并保留 `aff` 等查询参数；已登录用户访问注册页时跳转 `/dashboard`，避免邀请注册链接和会话状态出现前端路由断点。 |
| 2026-07-09 | Rankings 路由最新配置守卫 | `web/default/src/routes/rankings/index.tsx` | 默认前端 Rankings 路由进入前刷新 `/api/status`，按最新 `HeaderNavModules.rankings` 判断关闭跳转首页、登录可见跳转登录；保留 NexusTok 自有 `period=all` 查询参数。 |
| 2026-07-09 | `/v1/models` 动态 `owned_by` | `model/model_meta.go`、`controller/model.go`、`model/model_owner_test.go`、`controller/model_*test.go` | OpenAI 兼容模型列表按当前用户/Token 可用分组查询首选启用渠道，并用实际渠道归属覆盖 `owned_by`；未知模型仍回落 `custom`，Token 模型限制路径缺少分组上下文时保持旧兼容行为。 |
| 2026-07-09 | Pricing 路由最新配置守卫 | `web/default/src/lib/nav-modules.ts`、`web/default/src/routes/pricing/*` | 默认前端 Pricing 列表与详情路由进入前刷新 `/api/status`，按最新 `HeaderNavModules.pricing` 判断关闭跳转首页、登录可见跳转登录，并写回本地 status 缓存；后端 HeaderNavModuleAuth 仍是最终接口边界。 |
| 2026-07-09 | 登录成功审计日志 | `model/log.go`、`controller/user.go`、`web/default/src/features/usage-logs/*`、`web/default/src/i18n/locales/*.json` | 新增 `LogTypeLogin=7` 与成功登录审计记录，登录落点写入登录方式、IP、User-Agent 和结构化 `op`；默认前端 Usage Logs 支持 Login 类型筛选与详情展示，并补齐六语翻译。 |
| 2026-07-09 | 转换后上游 JSON 请求体存储与 Content-Length | `relay/common/outbound_body.go`、`relay/common/relay_info.go`、`relay/channel/api_request.go`、`relay/*_handler.go` | 新增转换后 JSON body 的 `BodyStorage` 包装，记录最终上游 body 字节数并在通用 HTTP 请求构造层回填 `ContentLength`；主要 JSON 转换 handler 已接入，透传、multipart 和 WebSocket 路径保持原语义。 |
| 2026-07-09 | Secure Session Cookie 环境配置 | `common/session_cookie.go`、`common/init.go`、`main.go`、`common/session_cookie_test.go` | 新增 `SESSION_COOKIE_SECURE` 与 `SESSION_COOKIE_TRUSTED_URL` 配置，默认保持 HTTP 开发兼容；开启 Secure 时要求可信 HTTPS URL，并让 session store 使用配置值。 |
| 2026-07-09 | Max Token 家族字段上限保护 | `relay/helper/valid_request.go`、`relay/helper/max_tokens_bounds_test.go` | 统一限制 OpenAI `max_tokens/max_completion_tokens`、Claude `max_tokens/max_tokens_to_sample`、Gemini `maxOutputTokens/max_output_tokens` 和 Responses `max_output_tokens`，避免极端输出 token 参数绕过预扣费与 int 转换安全边界。 |
| 2026-07-09 | 额度饱和日志前端可观测性 | `web/default/src/features/usage-logs/*`、`web/default/src/i18n/locales/*.json` | 管理员使用日志列表与详情弹窗展示 `other.admin_info.quota_saturation`，补齐钳制类型、原始值、钳制结果和操作来源；新增六语翻译并保持非管理员不可见。 |
| 2026-07-09 | 渠道新增缓存刷新 | `controller/channel.go`、`controller/channel_add_cache_test.go` | 新增渠道成功后同步刷新分发缓存和代理客户端缓存，避免当前进程仍按旧缓存返回 `model_not_found`；测试覆盖成功新增与校验失败路径。 |
| 2026-07-09 | OpenAI 图片流式与计费边界 | `dto/openai_image.go`、`relay/helper/valid_request.go`、`relay/channel/openai/relay-openai.go`、`relay/channel/openai/adaptor.go`、`relay/helper/stream_scanner.go` | 原生化 Images `stream` 显式零值、JSON/multipart `n` 上限、multipart body 复读、图片 usage 标准化、SSE/JSON-as-SSE 图片流和 error event 记录；补齐 dto/helper/openai 单元测试。 |
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
| 2026-07-09 | Waffo Pancake catalog 与一键建品 | `service/waffo_pancake.go`、`controller/topup_waffo_pancake.go`、`router/system-setting-router.go`、`web/default/src/features/system-settings/integrations/waffo-pancake-settings-section.tsx`、`web/default/src/i18n/locales/*.json` | 默认前端支付设置页支持拉取 Pancake Store/OnetimeProduct catalog、选择已有绑定和一键创建默认 Store/Product；后端新增 catalog/pair 管理接口，权限区分 read/operate/sensitive_write，保存仍走原子配置接口。 |
| 2026-07-09 | Waffo Pancake 订阅套餐产品绑定 | `model/subscription.go`、`model/main.go`、`controller/subscription.go`、`service/waffo_pancake.go`、`controller/topup_waffo_pancake.go`、`router/system-setting-router.go`、`web/default/src/features/subscriptions/*`、`web/default/src/i18n/locales/*.json` | 订阅套餐新增 `waffo_pancake_product_id`，管理端可读取已保存 Store 下的 active OnetimeProduct、手填 Product ID 或按套餐标题和价格创建专属 OnetimeProduct；用户侧 Pancake 订阅支付、checkout 外部单号和 webhook 分流仍保留为后续独立评审。 |
| 2026-07-09 | 控制台内容基础表单保存按钮 Authz 消费 | `web/default/src/features/system-settings/content/{dashboard-section.tsx,chat-settings-section.tsx,drawing-settings-section.tsx}` | Dashboard、Chat Presets 和 Drawing 三个基础内容设置保存按钮消费 `useUpdateOption()` 暴露的保存权限字段；列表编辑器页面保留给后续单独切片。 |
| 2026-07-09 | 控制台内容列表编辑器保存与开关 Authz 消费 | `web/default/src/features/system-settings/content/{announcements-section.tsx,api-info-section.tsx,faq-section.tsx,uptime-kuma-section.tsx}` | Announcements、API Addresses、FAQ 和 Uptime Kuma 的最终保存按钮及启用开关消费 `system_setting.sensitive_write`；本地列表草稿新增、编辑、删除和批量删除保持原交互。 |
| 2026-07-09 | 模型配置卡片保存按钮 Authz 消费 | `web/default/src/features/system-settings/models/{global-settings-card.tsx,gemini-settings-card.tsx,claude-settings-card.tsx,grok-settings-card.tsx}` | Global、Gemini、Claude 和 Grok 模型配置卡片最终保存按钮及提交 handler 消费 `system_setting.sensitive_write`；表单内 Switch、JSON 格式化和示例填充继续作为本地草稿操作。 |
| 2026-07-09 | io.net 连接测试按钮 Authz 消费 | `web/default/src/features/system-settings/integrations/ionet-deployment-settings-section.tsx` | 模型部署设置中的 `Test Connection` 按钮和 handler 消费后端路由对应的 `model.operate`；io.net option 保存仍由 `system_setting.sensitive_write` 控制。 |
| 2026-07-09 | Waffo Pancake 配置原子保存 | `model/option.go`、`service/waffo_pancake.go`、`controller/topup_waffo_pancake.go`、`router/system-setting-router.go`、`web/default/src/features/system-settings/{api.ts,types.ts,integrations/waffo-pancake-settings-section.tsx}` | 对齐 new-api-main 的 Waffo Pancake 保存优势，新增三库兼容 `UpdateOptionsBulk` 与 `POST /api/option/waffo-pancake/save`，支付设置页改为一次性提交 Waffo Pancake 配置；catalog、pair 和订阅产品能力保留为后续 SDK 阶段。 |
| 2026-07-09 | 订阅额度手动重置 | `model/subscription.go`、`controller/subscription.go`、`router/subscription-router.go`、`web/default/src/features/subscriptions/*`、`web/default/src/i18n/locales/*.json` | 原生化 new-api-main 的管理员手动重置订阅额度能力，支持套餐级和用户级重置、`advance_reset_time`、`subscription.operate` 权限、用户管理日志和管理审计。 |
| 2026-07-09 | Token Limits 默认前端入口 | `web/default/src/features/system-settings/{security,request-limits,types.ts}`、`web/default/src/i18n/locales/*.json` | 默认前端安全设置新增 `token-limits` 分区，Root 可查看并保存现有 `token_setting.max_user_tokens`，复用 `system_setting.sensitive_write` 权限和统一 option 保存链路。 |
| 2026-07-09 | Routing Reliability 默认前端聚合入口 | `web/default/src/features/system-settings/models/*`、`web/default/src/features/system-settings/types.ts`、`web/default/src/i18n/locales/*.json` | 默认前端模型设置新增 `routing-reliability` 分区，聚合重试、自动重试状态码、渠道自动禁用/恢复和自动测试模式，复用现有 option 保存与 `system_setting.sensitive_write` 权限。 |
| 2026-07-09 | Classic 前端退场提示 | `web/classic/src/components/layout/*`、`web/classic/src/helpers/frontendTheme.js`、`web/classic/src/i18n/locales/*.json` | classic 全局布局新增可关闭维护提示，Root 用户可从提示中切换到默认前端；关闭状态仅保存在当前浏览器，默认前端和后端核心链路不受影响。 |
