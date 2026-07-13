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
| 细粒度授权 Authz | `service/authz/*`、`model/authz_role.go`、`model/casbin_rule.go`、`router/authz-router.go`、`GET /api/authz/catalog` | catalog、自身份权限回传、用户级 override、默认前端入口/按钮消费、多组 Admin 路由 enforcement、持久策略底座、自定义角色模板 CRUD、用户角色分配、Casbin 影子 runtime 和 mismatch 可视化已持续落地 | 已新增 NexusTok 原生权限 catalog，覆盖渠道、账号池、用户、模型、订阅、兑换码、日志、用量数据和系统设置等资源，并返回 Root/Admin 基线矩阵；`/api/user/self` 已在 `permissions.admin_permissions` 回传同一矩阵，`AuthzUserOverride` 已支持管理用户 allow/deny 覆盖并同步维护 `casbin_rule` 兼容镜像；默认前端已让管理入口、Usage Logs、Dashboard、渠道、账号池、系统设置等页面消费该矩阵；`/api/channel`、`/api/account-pool`、`/api/subscription/admin`、`/api/models`、`/api/user`、`/api/redemption`、`/api/log`、`/api/data`、`/api/option`、`/api/system-task`、`/api/system-info` 等已按同一矩阵做服务端二次校验；`authz_roles` 与 `casbin_rule` 已作为角色/策略持久化底座落地，`GET /api/authz/policies/export`、`POST /api/authz/policies/import`、`GET /api/authz/roles`、`POST /api/authz/roles`、`PUT /api/authz/roles/:key`、`DELETE /api/authz/roles/:key` 和 `PUT /api/authz/roles/:key/policies` 已提供 Root-only 策略导出、导入、角色模板管理和角色策略编辑后端合同；默认前端 `/system-settings/security/role-policies` 已接入角色模板创建/编辑/删除、模板创建/更新时间展示、角色策略矩阵、dry-run 预览和二次保存确认；Root 现可在用户编辑页为系统 Admin 分配启用的自定义角色模板作为授权基线，运行时 `Can()` / `CapabilitiesForUser()` 会读取 `users.authz_role` 并叠加用户级 override；Authz 角色/策略写操作已写入结构化管理审计摘要；Casbin `SyncedEnforcer` 已以只读影子模式加载 `casbin_rule` 并通过 `GET /api/authz/shadow/role-mismatches` 与 Role Policies 页面展示一致性对比，但真实授权热路径暂不切换到 Casbin runtime。真正剩余的是更细资源拆分、完整 shadow 审计报表，以及评估何时把真实授权切到 runtime，而不是基础 Authz enforcement。 |
| 渠道路由权限表 | `router/channel-router.go`、`middleware.RequirePermission` | 已落地并扩展到多组管理路由 | `/api/channel` 已迁移为权限表注册，读、操作、写、敏感写和密钥查看分别挂接 `authz.Channel*` permission；同一 `permissionRoute` 模式已扩展到账户池、订阅、模型、用户、兑换码、日志/用量、系统设置、系统任务和系统信息等路由。后续重点转为 Casbin runtime/角色编辑和资源粒度继续拆分。 |
| 渠道/账号池敏感字段 fail-closed | `controller/channel_authz.go`、`controller/account_pool_authz.go` | 已落地主要更新路径 | 渠道更新接口已建立敏感/非敏感/操作/只读字段分类，未知字段默认敏感；账号池分组更新已对 `platform`、`auth_type`、`model_mapping`、`settings` 和未知字段做敏感写二次校验，并让默认前端账号池页消费 `write/operate/sensitive_write`。后续新增字段必须进入分类测试。 |
| 管理操作审计兜底 | `middleware/audit.go`、`controller/audit.go` | 已落地 | 保留账号池专用审计，同时已新增全局操作审计兜底；管理写操作记录 action、params、状态码、业务成功状态和操作者，不记录请求体，前端 Usage Logs 详情可展示结构化审计信息。 |
| 系统任务中心 | `model/system_task.go`、`service/system_task.go`、`controller/system_task.go`、`controller/system_task_handlers.go`、`/api/system-task/*` | 后端模型、租约锁、runner、Root 查询接口、日志清理任务、批量渠道测试任务、上游模型同步任务、账号池检测任务、订阅维护任务、Midjourney/通用异步任务轮询和默认前端任务面板已落地 | 继续把后续新增长耗时后台动作统一接入 SystemTask，并在 `/system-info` 统一观测。 |
| 系统实例心跳 | `model/system_instance.go`、`service/system_instance.go`、`GET /api/system-info/instances` | 后端心跳、Root 只读接口和默认前端 `/system-info` 实例面板已落地 | 已引入 `NODE_NAME`/主机名兜底、主从节点、CPU/内存/磁盘/版本心跳、实例页面和同页 SystemTask 任务面板。 |
| 后台任务锁 | `SystemTaskLock`、租约续期、过期失败标记 | 已按 NexusTok 三库兼容模型原生化 | 已用数据库锁实现跨节点互斥、租约续期、过期失败标记和锁丢失保护；后续任务 handler 要支持 context cancellation。 |
| 匿名请求体限制 | `common/request_body_limit.go`、`middleware/request_body_limit.go` | 已落地 | 已原生化为匿名入口通用中间件，并用于注册、登录、setup、OAuth 绑定、Webhook 等入口，默认 512KB，可用 `ANONYMOUS_REQUEST_BODY_LIMIT_KB=0` 禁用；后续新增匿名 POST 必须显式评估是否挂载。 |
| 顶栏模块鉴权 | `middleware/header_nav.go` | 已落地 | 已原生化为后端 `HeaderNavModuleAuth` 与默认前端集中解析/路由守卫，让 `/api/pricing`、`/api/rankings` 和 pricing 辅助性能接口跟随 `HeaderNavModules.enabled/requireAuth`，页面隐藏和接口鉴权一致。 |
| 额度饱和保护 | `common/quota_math.go`、`QuotaClamp`、相关测试 | 已部分落地 | 已新增 NexusTok 原生 `common/quota_math.go`，接入表达式计费、文本/音频/WSS 结算、标准预扣、按次预扣、任务差额结算、工具调用附加费、违规费用、充值入账、视频任务重算和渠道测试日志；饱和事件已写入 `other.admin_info.quota_saturation`。 |
| 受保护 Fetch / SSRF | `service/protected_fetch_client.go` | 已落地首批 | 已新增用户可控 URL 专用 protected fetch client，并接入下载、Webhook、Bark/Gotify 通知；Relay 全局 client 明确保留普通 client，避免误伤合法内网模型渠道，新增用户可控 URL 拉取必须复用该客户端或在评审中说明例外。 |
| ClickHouse 日志兼容测试 | `model/clickhouse_log_test.go`、`gorm.io/driver/clickhouse` | 缺少依赖 | 仅在明确支持 ClickHouse 日志库时引入；否则先记录为可选能力，避免增加部署复杂度。 |
| 流量账本查询 | `model/usedata_flow.go`、`controller.GetAllFlowQuotaDates`、`/api/data/flow` | 后端与默认前端 Flow/Sankey 已落地 | 已扩展 `quota_data` 记录 node/token/group/channel 维度，并新增 `/api/data/flow`、`/api/data/flow/self`；默认前端 `/dashboard/flow` 已接入 Sankey 图、指标切换、Top 节点限制、节点筛选、阶段显隐和点击高亮。 |
| Relay 转换包整理 | `service/relayconvert/*`、更多 Responses/Gemini/OpenAI 测试 | NexusTok 使用 `service/openaicompat/*`；Responses 请求降级、Chat/Responses 响应互转和流式状态机已持续原生化 | 保留 `openaicompat` 作为本项目原生命名，不创建 `relayconvert`；剩余 ClickHouse、Advanced Custom、Authz 持久化等差异继续分批评审。 |
| OpenAI Realtime / image edit / image stream 等 relay 文件 | `relay/channel/openai/relay_realtime.go`、`relay_image.go` | Realtime、image edit、image stream 与图片计费边界已在现有文件中实现 | 已补齐 `dto.ImageRequest.Stream *bool` 显式零值保留、`n` 上限校验、OpenAI 图片 SSE 转发、JSON-as-SSE 兜底和 usage 结算测试；后续新增 OpenAI 子协议时继续逐项核对 StreamOptions 和计费快照。 |
| Advanced Custom Channel | `relay/channel/advancedcustom/adaptor.go` | 后端基础层与默认前端编辑入口已落地 | 已新增 type 58、`settings.advanced_custom` schema、严格校验、relay adaptor、Visual/JSON 双模式编辑器和权限控制；后续如继续增强，应聚焦模板库、导入导出和更细审计，而不是从零迁移 adaptor。 |
| Waffo Pancake SDK 绑定体验 | `waffo-pancake-sdk-go`、catalog/pair/save/product 接口 | 充值、订阅支付、catalog/pair/save/product 与订阅套餐商品绑定已落地 | 已在系统设置接入 Waffo Pancake catalog 刷新、store/product 创建和原子保存；用户充值、订阅支付、订阅套餐商品创建/选择、Webhook 分流均已原生化，继续保留 NexusTok 旧充值链路兼容。 |
| 订阅余额支付 | `SubscriptionRequestBalancePay`、`allow_balance_pay` | 后端与默认前端已落地 | 已新增 `/api/subscription/balance/pay`，在模型事务内完成钱包扣款、订阅创建和成功订单落账；默认前端已在套餐抽屉、套餐表和购买弹窗消费 `allow_balance_pay`。 |
| 订阅钱包溢出控制 | `allow_wallet_overflow` | 后端与默认前端已落地 | 已在套餐与用户订阅快照保存钱包溢出策略；`subscription_first` 下订阅额度不足时，任一活跃订阅禁止溢出即阻断钱包 fallback；默认前端套餐抽屉已提供钱包兜底开关。 |
| 用户权限回传 | `user.AdminPermissions = authz.Capabilities(...)` | 后端、用户级 override、默认前端入口消费与页面按钮级消费已落地 | `/api/user/self` 已返回 `permissions.admin_permissions`，默认前端已新增权限 helper/hook，并让管理侧边栏、Usage Logs、Dashboard、渠道、账号池、模型、用户、订阅等入口和关键按钮按资源动作判断；用户管理接口已支持 Root 设置管理用户的 `admin_permissions` override。后续重点是角色模板/Casbin 存储与更多细粒度页面矩阵编辑体验。 |

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
| 前端构建元数据与缓存版本清理 | `frontend-cache.ts`、`build-metadata.ts`、`main.tsx` 已接入 | new-api-main 有同类能力 | 已原生化为 NexusTok 命名空间：运行时暴露 `window.__NEXUSTOK_BUILD__`、DOM/meta/CSS/localStorage 构建指纹；缓存版本变化时清理旧 UI 缓存，同时保留登录、语言、系统状态、通知已读、表格视图、渠道模式和 Playground 草稿等关键状态。 |
| 构建/检查 | NexusTok 使用 ESLint/Prettier/tsc | new-api-main 使用 oxlint/oxfmt/tsgo | 不建议立刻切换；可单独评估速度收益，避免扰动现有格式化规则。 |

## 后端依赖和基础设施差异

| 类别 | NexusTok | new-api-main | 建议 |
|------|----------|--------------|------|
| Go module | `github.com/c1cada/NexusTok` | `github.com/QuantumNous/new-api` | 禁止直接复制 import，迁移时必须改为 NexusTok 包路径。 |
| Casbin | 已引入只读影子 runtime 和 mismatch 可视化，真实授权热路径仍由 NexusTok 原生快照判定 | `github.com/casbin/casbin/v2`、`casbin_rule.go` | `casbin_rule` 已作为持久策略表落地；Casbin `SyncedEnforcer`、只读 GORM adapter 和周期 reload 已用于观察 `casbin_rule` 与当前角色权限矩阵是否一致；Root 可通过 `/api/authz/shadow/role-mismatches` 和 Role Policies 页面查看 shadow 对比状态。`Can()` 暂不读取 Casbin runtime，避免在用户角色分配和完整差异审计完成前改变核心授权语义。 |
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

## 2026-07-10 复核后的后续优先队列

本轮按当前工作树重新核对后确认：早期队列中的行锁、Responses/Gemini Responses、全局管理审计、会话守卫、HeaderNavModules、Usage Logs 移动端、Dashboard Flow、Advanced Custom、Waffo Pancake 商品绑定、Playground 多个消息/输入/编辑切片、README/Issue 模板/开发 Compose 等已经在后续实施记录中落地。后续开发不应再按旧队列重复迁移，应聚焦仍未被当前代码和验证记录证明完成的能力。

### 后端与治理 P1/P2

| 优先级 | 能力 | 参考路径 | 当前状态与迁移方式 |
|--------|------|----------|--------------------|
| P1 | 角色模板与 Casbin/策略持久化 | `service/authz/*`、`model/authz_role.go`、`model/casbin_rule.go` | 已完成持久化底座：`authz_roles`、`casbin_rule`、Root/Admin 内置角色种子、Admin 默认策略写入、持久角色策略快照、显式 reload、周期同步、用户 override 的 `casbin_rule` 兼容镜像、用户角色分配、静态基线 fallback、Root-only 策略导出、带 dry-run/确认保护的策略导入接口、Root-only 角色策略列表/更新 API、自定义角色模板 CRUD API、角色/策略写操作审计摘要，以及默认前端角色策略矩阵、角色模板管理 UI、模板生命周期时间展示和 Casbin shadow mismatch 摘要；Casbin runtime/enforcer 已以只读影子模式引入，真实授权结果仍由现有快照、`users.authz_role` 基线和用户 override 判定。下一步应围绕完整 shadow 审计报表、运行时切换开关和切换前回归做独立评审，而不是再重做基础权限表。 |
| P1/P2 | Authz 资源继续细分 | `router/channel-router.go`、账号池/渠道账号/凭证路由 | 账号池认证文件已拆为独立 `account_pool_auth_file` 资源，`GET /api/account-pool/auth-files*` 走 read，导入/更新/删除走 sensitive_write；渠道内账号已拆为独立 `channel_account` 资源，列表/详情走 read，启停/清冷却走 operate，新增/批量导入/更新/删除走 sensitive_write；`channel_account.read` 响应已做基础脱敏，只有 sensitive_write 用户可见上游地址、组织 ID、请求覆盖、模型映射、provider settings 和原始错误详情；`account_pool` 继续覆盖全局分组、账号生命周期、日志、检测和运行态操作。后续如需要更细运营分权，可继续评估 `channel_account.write` 非敏感写边界、字段级审计增强和角色编辑 UI，而不是重复拆资源。 |
| P2 | ClickHouse 日志库真接入 | `model/clickhouse_log_test.go`、`gorm.io/driver/clickhouse` | 当前只完成日志查询准备层护栏、LIKE 转义、TTL SQL helper 和 fail-fast 提示，尚未引入 driver 或运行时写入。只有明确要支持 ClickHouse 部署时再扩展依赖、迁移、写入和查询矩阵。 |
| P2 | 账号池任务持久队列与占用释放 | `service/account_pool_task_limit.go`、SystemTask | 已有提交级并发/RPM/等待策略；完整持久队列、任务完成后释放账号占用和更细调度观测仍可作为账号池主线后续增强。 |
| P2/P3 | Advanced Custom 模板库与审计增强 | `relay/channel/advancedcustom/*`、默认前端 editor | 基础 adaptor、后端校验和 Visual/JSON 编辑器已落地；后续可做可信模板、导入导出、路由级审计和更清晰的敏感字段变更预览。 |

### 前端体验 P1/P3

| 优先级 | 能力 | 参考路径 | 当前状态与迁移方式 |
|--------|------|----------|--------------------|
| P1 | 账号池日志与全局 Usage Logs 体验统一 | `usage-logs-mobile-card.tsx`、账号池 History | 已完成首批账号池专用移动端卡片与移动筛选 Drawer：Usage Logs、State Logs、Check History 在手机端不再横向滚动表格，常用搜索保留在页面，低频筛选收纳进底部 Drawer，并保留账号粒度、状态迁移、检测任务详情和脱敏语义；后续可继续推进 DataTable 分层。 |
| P2 | DataTable 渐进分层 | `components/data-table/core/*`、`layout/*`、`toolbar/*` | NexusTok 已补 DataTable README，并新增桌面表格/卡片视图切换底座，先后在 `/models/metadata` 模型表和 `/channels` 渠道列表启用；new-api-main 的 core/layout/static/hooks 全量目录拆分仍未迁移。后续新页面或大改页面可继续复用该底座，避免一次性替换全站表格。 |
| P2/P3 | Playground 目录结构 polish | `features/playground/components/{chat,input,message}`、`lib/{message,streaming,storage}` | 消息渲染、错误、输入、编辑器、CodeMirror、流式 helper 和大量纯函数测试已原生化；剩余差异主要是目录层级组织，不应为了路径一致而重构，除非后续功能开发自然触碰。 |
| P2/P3 | Classic 前端同等体验补齐 | `web/classic` | 默认前端是主要承载面；Classic 仍可按低优先级补齐 Codex 用量、部分订阅/支付和账号池历史体验，但不阻塞默认前端原生能力。 |

### 文档与工程化 P2/P3

| 优先级 | 能力 | 参考路径 | 当前状态与迁移方式 |
|--------|------|----------|--------------------|
| P2 | ClickHouse/高级部署文档 | `docs/installation/*` | 多语言 README、Electron、constant、开发 Compose、Makefile、任意分支 Docker 镜像和 Issue 模板已品牌化；若后续真接入 ClickHouse，再补部署和回滚文档。 |
| P3 | CI 工具链评估 | `oxlint`、`oxfmt`、`tsgo` | 当前保留 Bun/ESLint/TypeScript/Rsbuild 工具链；只有在现有 lint/build 成本成为瓶颈时再评估，不作为功能差异迁移前置条件。 |

## 原生化路线建议

### P0：保护并完成 NexusTok 已有账号池主线

1. 保持 `AccountPoolGroup`、`PoolAccount`、`AccountPoolAuthFile` 为原生一等模型。
2. 不引入外部 sub2api/CPAMC/Sidecar 作为运行依赖。
3. 账号池检测任务已迁移到统一 SystemTask 执行层，但 API 响应继续兼容现有前端。
4. 账号池导出、状态日志和使用日志继续脱敏，不向通用审计泄露完整凭据。
5. 渠道表单继续以 `credential_mode` + `account_pool_group_id` 表示账号池模式。

### P1：平台治理能力

1. 引入 Authz：
   - 只读 catalog 已扩展到渠道、账号池、用户、模型、订阅、兑换码、日志、用量数据、系统设置、系统任务和系统信息等管理资源。
   - 动作已先统一为 `read`、`operate`、`write`、`sensitive_write`、`secret_view`。
   - `/api/user/self` 已回传 `permissions.admin_permissions`，默认前端已用同一 schema 过滤管理侧边栏、入口级路由守卫和多处页面按钮。
   - Root 基线拥有全部权限，Admin 基线拥有非敏感 read/operate/write；`authz_roles` 与 `casbin_rule` 已持久化内置角色和 Admin 默认策略，授权判定优先读取持久角色策略并保留静态基线 fallback；`AuthzUserOverride` 已支持管理用户级 allow/deny 覆盖，并把用户级策略同步镜像到 `casbin_rule`；Root 可把启用的自定义角色模板分配给系统 Admin 用户作为 `users.authz_role` 授权基线；`GET /api/authz/policies/export`、`POST /api/authz/policies/import`、`GET /api/authz/roles`、`POST /api/authz/roles`、`PUT /api/authz/roles/:key`、`DELETE /api/authz/roles/:key` 与 `PUT /api/authz/roles/:key/policies` 已形成 Root-only 策略迁移、角色模板管理和角色策略编辑后端闭环，默认前端安全设置页已提供角色模板创建/编辑/删除、模板生命周期时间、角色策略矩阵、dry-run 预览和二次保存确认。Casbin runtime/enforcer 已以只读影子模式接入，用于加载 `casbin_rule` 并通过 API 与页面摘要对比当前角色权限矩阵；后续重点是更深角色策略审计增强、真实授权切换方案和更细资源拆分。
2. 拆分路由注册：
   - 渠道、账号池、订阅、模型、用户、兑换码、日志/用量、系统设置、系统信息、系统任务和 Authz catalog 路由都已按 `permissionRoute` 模式拆分或接入权限表。
   - 后续新增管理路由必须先进入资源动作分类和路由结构测试，避免退回只依赖粗粒度 Admin/Root 的注册方式。
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

1. 匿名请求体限制已完成首批原生化，并挂到 setup、register、login、2FA login、passkey login、password reset、OAuth bind、Webhook 等入口；后续新增匿名 POST 路由时，必须在路由评审里明确挂载 `AnonymousRequestBodyLimit()` 或说明无需限制的原因。
2. SSRF 保护客户端已完成首批和后续补强审计，下载、Webhook、Bark/Gotify、Uptime Kuma、自定义 OAuth Discovery、OAuth 登录端点和历史 OIDC 控制器等用户/管理员可控外部 URL 拉取已走统一 protected client 或等价 URL 校验；Relay 上游地址继续保留普通 client，避免误伤合法内网渠道。
3. HeaderNavModuleAuth 已完成后端鉴权、默认前端健壮解析和 pricing/rankings 路由最新配置守卫；后续新增公开导航模块时，必须同步维护 `HeaderNavModules` 解析、前端守卫和后端 API 鉴权。
4. 渠道和账号池敏感字段 fail-closed 已覆盖主要更新路径：渠道更新和账号池分组更新均按原始请求字段做分类，未知字段默认敏感；账号池凭证、账号生命周期、OAuth 和批量导入路径继续在路由级要求 `account_pool.sensitive_write`。后续新增字段必须先补分类和测试，再开放给前端。

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
   - 默认前端套餐抽屉、套餐表和购买弹窗已经消费这两个策略字段。
3. Waffo Pancake 保留 NexusTok 当前充值链路，同时已吸收 new-api-main 的 catalog/pair/product 绑定向导、配置原子保存、用户充值支付入口、订阅支付入口和订阅套餐商品绑定。
4. 订阅管理页已补齐余额支付、钱包溢出、Waffo Pancake 商品、显式降级分组等关键能力；后续重点是权限矩阵编辑 polish、订单诊断体验和 Classic 前端低优先级补齐。

### P2：仪表盘和日志体验

1. `/api/data/flow` 与默认前端 `/dashboard/flow` 已落地，后续 Flow 增强应聚焦保存偏好、导出和更细角色遮罩策略。
2. 普通 Usage Logs 已吸收 `logs-filter-toolbar` 和移动端卡片；后续可把账号池专用日志在保留脱敏字段和账号粒度的前提下统一体验。
3. 模型设置已新增“路由可靠性”页面，把自动禁用、自动启用、重试状态码、自动测试集中管理；后续新增可靠性选项继续归入该 section。
4. 公共 DataTable 逐步向 new-api-main 的 core/layout/static 分层靠拢，避免每个页面重复实现移动端卡片。

### P3：Relay 和 Playground 演进

1. OpenAI Realtime、Image Edit/Image Stream、Gemini Responses、Advanced Custom adaptor 等高价值 relay 差异已持续原生化；后续逐协议引入时仍需先核对请求 DTO 显式零值、StreamOptions、计费快照和响应错误语义。
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
2. 安全小步快跑第一批和多轮补强已完成：匿名请求体限制、HeaderNavModuleAuth、SSRF 客户端、QuotaMath helper、渠道/账号池字段级 fail-closed、用户级 Authz override、多组权限路由表、账号池认证文件独立权限资源、Authz 持久策略底座、策略导出/导入接口、角色策略编辑 API、用户角色分配、Casbin 影子 runtime 和 shadow mismatch 摘要已原生化；下一批聚焦完整 shadow 审计报表、真实授权切换评审和新增公开模块的测试契约。
3. 系统任务/系统信息已形成后端模型、接口、租约执行和 `/system-info` 页面闭环；后续新增长耗时任务直接接入 SystemTask。
4. 权限系统已覆盖 channel/account_pool/account_pool_auth_file/user/model/subscription/redemption/log/data/system_setting 等资源，并已经具备 `authz_roles`/`casbin_rule` 持久策略底座、周期 reload、用户 override 兼容镜像、用户角色分配、Root-only 策略导出/导入、角色策略编辑 API、Casbin 影子 runtime 和 shadow mismatch 摘要；后续从“有没有 enforcement”转为“完整 shadow 审计报表、真实授权切换评审、继续拆分细资源和权限编辑体验”。
5. 订阅/支付增强已覆盖余额支付、钱包溢出、Waffo Pancake 商品绑定和订阅支付；后续转向诊断、Classic parity 和支付异常自助排查。
6. 仪表盘和日志体验已覆盖 flow 数据、Sankey、移动端卡片、统一筛选工具条；后续优先统一账号池日志体验。
7. Relay/Playground 深水区已完成 Responses/Gemini/OpenAI image/Advanced Custom 和 Playground 多个关键切片；后续继续逐协议、逐组件小步演进，不做大爆炸迁移。

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
| `/api/authz/catalog` | 权限资源/角色 catalog | 已引入。 |
| `/api/authz/policies/export` | 角色与 Casbin 兼容策略快照导出 | 已引入 Root-only 只读导出，用于审计、备份和导入前 diff。 |
| `/api/authz/policies/import` | 角色与策略快照导入 | 已引入 Root-only 写入接口，默认 dry-run，replace 模式必须确认；导入 `user:<id>` 策略时同步 `authz_user_overrides` 与 `casbin_rule` 镜像。 |
| `/api/authz/roles`、`/api/authz/roles/:key`、`/api/authz/roles/:key/policies`、`/api/authz/shadow/role-mismatches` | 角色模板管理、角色策略查看/编辑和 Casbin shadow 一致性观测 | 已引入 Root-only 后端合同：读取返回持久角色模板、创建/更新时间、superuser/runtime 标记和 grants 矩阵；`POST /roles`、`PUT /roles/:key`、`DELETE /roles/:key` 管理非内置自定义角色模板，删除、禁用或导入替换已分配角色时会拒绝，避免破坏 `users.authz_role` 授权基线；策略更新默认 dry-run，显式 `dry_run:false` 后替换目标 `role:<key>` allow 策略并 reload 持久策略快照；`GET /shadow/role-mismatches` 返回 shadow runtime 可用状态、mismatch count 和明细。默认前端 `/system-settings/security/role-policies` 已接入角色模板创建/编辑/删除、生命周期时间展示、角色选择、完整权限矩阵、差异统计、dry-run 预览、二次保存确认和 Casbin shadow 摘要；用户编辑页已提供 Root-only 的 Admin 授权角色基线选择。 |
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

## 本轮实施评审：Authz 默认前端角色策略编辑 UI 原生化

### 需求分析

`new-api-main` 的平台治理优势不仅在于后端有角色策略接口，也在于 Root 管理员能在管理后台直接维护角色基线。NexusTok 已经具备 `GET /api/authz/roles` 与 `PUT /api/authz/roles/:key/policies`，但如果默认前端没有入口，实际运维仍只能依赖导入导出或 curl，无法把角色策略维护沉淀为原生后台能力。

本轮目标是在系统设置安全页新增 `role-policies` 分区，让 Root 可以查看 Root/Admin/自定义角色模板，编辑非 superuser 角色的完整权限矩阵，先 dry-run 预览策略差异，再二次确认保存。自定义角色创建、角色分配和完整 Casbin runtime/enforcer 不纳入本轮，避免把前端入口扩展成新的权限模型迁移。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 系统设置 API/types | `web/default/src/features/system-settings/api.ts`、`types.ts` | 新增 Authz 角色列表、角色策略更新和权限 catalog 的前端类型/调用封装，写接口使用 `skipBusinessError` 后由页面统一 toast。 |
| 安全设置分区 | `web/default/src/features/system-settings/security/section-registry.tsx`、`role-policy-section.tsx` | 新增 `Role Policies` 分区，Root-only 查询角色策略；提供角色选择、badge、统计、权限矩阵、行批量开关、dry-run 预览、保存确认和刷新。 |
| 权限矩阵工具 | `web/default/src/features/system-settings/security/role-policy-utils.ts` | 以后端 catalog 补齐完整矩阵，计算 enabled/changed 数量、矩阵签名和行批量开关，避免只提交局部授权。 |
| 前端测试 | `web/default/src/features/system-settings/security/role-policy-utils.test.ts` | 覆盖矩阵归一化、行批量开关不丢其它资源、差异计数和签名稳定性。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增角色策略页面静态文案和动态 badge 六语翻译。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新 Authz 队列状态、API 摘要、实施索引和本轮评审记录。 |

本轮不修改 Go 后端、数据库 schema、Casbin 表结构、运行时授权判定、用户 override 保存语义、角色创建/分配链路或已有系统设置保存接口。

### 风险评估

1. 误保存权限风险：角色策略保存会影响所有使用该基线且没有用户 override 的管理员；本轮默认只做 dry-run 预览，只有预览与当前草稿签名一致时才允许二次确认保存。
2. 非 Root 暴露风险：后端接口已 Root-only，前端也在非 Root 或缺少 `system_setting.secret_view` 时不发查询，只展示权限提示。
3. 局部矩阵丢失风险：前端如果只提交当前可见 checkbox，会把未渲染资源误清空；本轮通过 catalog 归一化完整矩阵，并用单元测试锁定。
4. Root 角色误编辑风险：Root 是 superuser 语义，不应转为普通 allow 列表；页面展示 Root 但所有控件禁用，并提示只读。
5. Admin 空策略风险：后端已拒绝空 Admin 策略，前端也在 enabled action 为 0 时禁用预览与保存，避免触发静态 fallback 误解。
6. i18n 漏 key 风险：角色 badge 有动态 `t(badge)`，普通源码扫描不一定发现；本轮手动补齐 `Superuser`、`Runtime-managed`、`Template only` 等动态 key。

### 方案评审

采用“系统设置安全页内新增独立分区”的低风险方案：不新增路由、不改变权限模型，只消费现有 Authz API 和 catalog。UI 保持后台高密度表格风格，使用现有 Button/Badge/Alert/Checkbox/Select/Table/Skeleton/Spinner/AlertDialog 组件；角色选择使用下拉，权限矩阵使用横向可滚动表格，移动端不重排成新数据结构。

权限编辑采用三段式：

1. 根据 `GET /api/authz/roles` 和 `GET /api/authz/catalog` 生成完整矩阵。
2. 用户修改草稿后先调用 `PUT /api/authz/roles/:key/policies` 且 `dry_run:true`，展示 created/deleted/unchanged 摘要。
3. 只有当前草稿签名与预览签名一致时，才允许打开确认框并用 `dry_run:false` 保存；保存成功后刷新角色策略。

### 实施结果

已完成 Authz 默认前端角色策略 UI 原生化：

- 系统设置安全页新增 `Role Policies` 分区。
- Root 可选择角色并查看 built-in、superuser、runtime-managed/template-only、disabled 等状态。
- 非 superuser 且启用的角色可编辑完整 resource/action 矩阵；Root 角色只读。
- 页面展示当前 enabled 数量、持久策略数量、草稿 changed 数量和 enable/disable 差异。
- 行级 `Select row` / `Clear row` 只影响当前 resource，不会丢失其它资源授权。
- 保存前必须先 dry-run，预览过期后无法 apply。
- 前端补齐 en/zh/fr/ja/ru/vi 翻译，动态 badge 文案也已覆盖。

### 验证记录

1. `cd web/default && bun test src/features/system-settings/security/role-policy-utils.test.ts` 通过。
2. `cd web/default && bun run typecheck` 通过。
3. `cd web/default && bunx eslint src/features/system-settings/types.ts src/features/system-settings/api.ts src/features/system-settings/security/section-registry.tsx src/features/system-settings/security/role-policy-section.tsx src/features/system-settings/security/role-policy-utils.ts src/features/system-settings/security/role-policy-utils.test.ts` 通过。
4. `cd web/default && bunx prettier --check src/features/system-settings/types.ts src/features/system-settings/api.ts src/features/system-settings/security/section-registry.tsx src/features/system-settings/security/role-policy-section.tsx src/features/system-settings/security/role-policy-utils.ts src/features/system-settings/security/role-policy-utils.test.ts` 通过。
5. `cd web/default && bun run i18n:sync` 通过；新增角色策略 key 已存在于 `en.json`，同步报告显示各 locale `missingCount=0`。
6. 3003 页面验证：Chrome/CDP 访问 `http://192.168.0.202:3003/system-settings/security/role-policies`，页面加载 `Role Policies` 分区；切换到 Admin 后显示 `policy_count=25`、`25/41` enabled actions 和 41 个可编辑 checkbox；点击一个 checkbox 后预览按钮启用且保存按钮保持禁用。
7. 3003 dry-run 联调：页面触发 `PUT /api/authz/roles/admin/policies`，请求体包含 `dry_run:true` 和完整 grants 矩阵；返回后页面显示 `预览已就绪`、`新增策略: 0`、`删除策略: 1`、`未变化策略: 24`，保存按钮才启用，但未点击保存确认。
8. 3003 防污染复查：dry-run 后再次调用 `GET /api/authz/roles`，Admin 仍为 `policy_count=25`，`channel.read/operate/write=true`，确认真实角色策略未被页面草稿污染；Chrome/CDP 期间未捕获 console error 或 runtime exception。

## 本轮实施评审：Authz 角色策略编辑接口原生化

### 需求分析

`new-api-main` 的 Authz 优势不止是能导入导出整份 Casbin 策略，还包括 Root 管理员可以围绕角色模板维护权限矩阵。NexusTok 已经完成权限 catalog、用户 override、路由 enforcement、`authz_roles`/`casbin_rule` 持久底座、策略 reload、导出和导入，但日常运维如果只依赖整包导入，会把“调整 Admin 是否可写某个资源”这种小变更变成高风险快照操作。

本轮目标是在不引入完整 Casbin runtime、不新增默认前端 UI 的前提下，先补齐角色策略编辑的后端合同：

1. `GET /api/authz/roles` 返回持久化角色模板、superuser/runtime 标记、策略数量和 grants 矩阵。
2. `PUT /api/authz/roles/:key/policies` 接收 catalog 同构的 grants 矩阵，默认 dry-run。
3. 显式 `dry_run:false` 时，只替换目标角色的 `role:<key>` allow 策略，并在事务提交后 reload 持久角色策略快照。
4. Root 角色只读，不能被降级成普通 allow 列表；Admin 角色不能被更新为空策略，避免触发当前静态 fallback 语义造成“看似全禁、实际回退默认授权”的误解。
5. 未知 resource/action 继续 fail-closed，拒绝写入 `casbin_rule`。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz service | `service/authz/role_policy.go` | 新增角色策略 DTO、持久角色列表、策略校验、dry-run 摘要、事务替换和 reload。 |
| Controller | `controller/authz.go` | 新增 `ListPermissionRoles` 和 `UpdatePermissionRolePolicies`，写接口使用 `common.DecodeJson`。 |
| Router | `router/authz-router.go` | 新增 `GET /api/authz/roles` 与 `PUT /api/authz/roles/:key/policies`，均叠加 `RootAuth`；读取归类为 `system_setting.secret_view`，写入归类为 `system_setting.sensitive_write`。 |
| 测试 | `service/authz/role_policy_test.go`、`router/authz_router_test.go` | 覆盖角色列表、dry-run 默认值、写入 reload、Root 只读、未知 permission 拒绝、禁用角色拒绝和路由分类。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新 P1 Authz 状态、API 摘要、实施索引和本轮评审记录。 |

### 风险评估

1. 角色策略编辑会改变管理员运行态权限，必须保持 Root-only，不能只依赖可配置权限矩阵。
2. Root 是 superuser，没有也不应有全量 allow 策略；允许编辑 Root 会导致 UI 和授权语义分叉。
3. 当前持久策略读取在“无有效 Admin policy”时会回退静态基线；因此 Admin 角色更新为空策略必须拒绝，避免误导管理员。
4. 直接替换 `casbin_rule` 需要事务保护；任一步失败都不能留下半更新策略。
5. 写库后若不 reload，当前节点 `Can()` 与 `/api/user/self` 会继续使用旧快照；本轮事务提交后立即 reload。
6. 自定义角色目前还没有系统角色分配链路；接口返回 `runtime_managed`，让后续 UI 明确区分“可保存模板”和“已接入运行时角色”。

### 方案评审

采用“小步后端合同优先”的方案：复用 `authz_roles` 和 `casbin_rule`，不新增表，不引入 `casbin.SyncedEnforcer`，不改变 `Can()` 的用户 override 和角色 fallback 热路径。读取接口直接从数据库角色模板生成矩阵；写接口把 grants 规范化为 catalog 中已知 permission，只把 `true` 转换成 `p, role:<key>, resource, action, allow`，再事务内删除旧策略并创建新策略。

该方案把 new-api 的角色策略可维护性转为 NexusTok 原生后端能力，同时把更大范围的 UI、自定义角色创建、角色分配和完整 runtime/enforcer 留给后续切片。

### 实施结果

已完成以下能力：

- 新增 `GET /api/authz/roles`，Root 可读取 Root/Admin/自定义角色模板和 grants 矩阵。
- 新增 `PUT /api/authz/roles/:key/policies`，默认 dry-run，返回 old/new/created/deleted/unchanged 策略摘要。
- 显式 apply 后只替换目标角色 `role:<key>` 策略，并立即 reload 持久策略快照。
- Root 角色更新被拒绝；禁用角色更新被拒绝；未知 permission 被拒绝。
- Admin 角色不能被置为空策略，避免当前 fallback 语义造成保存结果和运行态不一致。
- 自定义角色可作为持久模板读取和保存策略，但 `runtime_managed=false` 明确表示尚未接入系统角色分配。

### 验证记录

1. `go test ./service/authz ./router ./controller` 覆盖 service、路由和 controller 编译回归。
2. `git diff --check` 确认无空白错误。
3. `cd web/default && bunx prettier --check ../../docs/features/new-api-main-diff-analysis.md` 通过。
4. 使用账号 `c1cada` 登录 3003 后调用 `GET /api/authz/roles` 返回 HTTP 200，响应包含 Root/Admin 两个角色；Root `superuser=true`、Admin `policy_count=25`，Admin `channel.write=true`、`system_setting.secret_view=false`。
5. 使用 3003 调用 `PUT /api/authz/roles/admin/policies`，请求体为 `{"grants":{"channel":{"read":true}}}`，未显式传 `dry_run:false`，返回 `dry_run=true`、`applied=false`、`old_policy_count=25`、`new_policy_count=1`、`deleted_policy_count=24`、`unchanged_policy_count=1`。
6. dry-run 后再次调用 `GET /api/authz/roles`，Admin 仍为 `policy_count=25` 且 `channel.write=true`，确认运行态验证没有污染真实权限策略。
7. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 HTTP 200，确认本轮后端接口改动后 3003 入口仍正常。

## 本轮实施评审：Authz 策略导入接口原生化

### 需求分析

上一轮已经把 `authz_roles` 和 `casbin_rule` 做成 Root-only 稳定导出，但导出只能用于审计和备份，不能把策略快照安全导回当前数据库。`new-api-main` 的 Casbin 体系优势不只是“能看到策略”，还包括策略可迁移、可回滚和可作为角色编辑 UI 的底层合同。NexusTok 需要在不引入完整 Casbin runtime 的前提下，先补齐导入入口。

本轮目标：

1. `POST /api/authz/policies/import` 接收与导出接口兼容的 JSON 快照。
2. 默认只 dry-run，返回将创建/更新/删除的角色、策略和用户 override 摘要。
3. `merge` 模式只新增/更新角色、补充缺失 role 策略，并按用户+资源替换导入中出现的用户 override。
4. `replace` 模式会替换所有 `role:*` 策略、清理导入快照中缺失的非内置角色，并把用户 override 重建为导入快照；真正写库必须传确认字符串。
5. 导入 `user:<id>` 策略时同步写入 `authz_user_overrides` 和 `casbin_rule` 镜像，避免当前授权主存储和未来 Casbin 兼容表分叉。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz service | `service/authz/policy_import.go` | 新增导入请求/结果 DTO、校验、dry-run 摘要、事务写入、用户 override 同步和 reload。 |
| Controller | `controller/authz.go` | 新增 `ImportPermissionPolicies`，使用 `common.DecodeJson` 解析请求。 |
| Router | `router/authz-router.go` | 新增 `POST /api/authz/policies/import`，叠加 `RootAuth` 与 `system_setting.sensitive_write`。 |
| 测试 | `service/authz/policy_import_test.go`、`router/authz_router_test.go` | 覆盖 dry-run 默认值、merge 写入、replace 确认、replace reload、用户 override 清理和内置角色保护。 |

### 风险评估

1. 导入是权限写路径，必须保留 Root-only 边界，且 replace 模式必须额外确认，避免误清空策略。
2. 禁用或删除内置 Root/Admin 角色会破坏管理安全边界；导入校验要求两者存在、`built_in=true` 且 `enabled=true`。
3. 直接写 `casbin_rule` 的 `user:<id>` 策略会绕过当前授权主存储，因此导入时必须同步 `authz_user_overrides`。
4. 导入未知资源/动作会让授权语义不可预测；本轮只接受当前 catalog 中已注册的 permission。
5. 写库后必须 reload 持久角色策略快照，否则当前节点会继续使用旧授权视图；事务提交后立即调用 `ReloadPersistentPolicies()`。

### 方案评审

采用“导出 JSON 兼容 + 默认 dry-run + 显式 apply”的方案，不引入 `casbin.SyncedEnforcer`，不新增表，不改变现有 `Can()` 热路径。导入 service 先把角色和策略规整、去重并校验，再基于当前数据库计算摘要；只有 `dry_run:false` 才开启事务写入。`merge` 适合安全补充，`replace` 适合恢复快照或跨环境迁移，且必须传 `confirm: "IMPORT_AUTHZ_POLICIES"`。

验收方式：

1. `go test ./service/authz` 覆盖导入数据契约、用户 override 同步和 reload 后授权变化。
2. `go test ./router -run Authz` 覆盖导入路由 handler、permission 和 RootAuth 前置。
3. 通过 3003 真实登录后调用 `/api/authz/policies/import` dry-run，确认返回摘要且不写库。
4. `git diff --check` 与 Prettier 文档检查确认格式无问题。

## 本轮实施评审：Authz 策略导出接口原生化

### 需求分析

`new-api-main` 的 Authz 体系围绕 Casbin 兼容策略存储做权限治理。NexusTok 已经完成权限 catalog、用户级 override、服务端路由 enforcement、`authz_roles`/`casbin_rule` 持久化底座、策略 reload 和用户 override 镜像，但仍缺少一个稳定的策略快照出口，Root 管理员无法直接审计当前数据库里的角色模板和策略记录，也不方便为后续导入、回滚和角色编辑 UI 做基线对比。

本轮目标是新增只读导出能力：

1. 导出 `authz_roles` 中的角色业务字段。
2. 导出 `casbin_rule` 中的 `ptype/v0..v5` 策略字段。
3. 不导出数据库自增 ID、创建时间和更新时间，避免跨环境不可复用。
4. 仅 Root 可访问，避免普通 Admin 通过接口读取完整权限策略。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz service | `service/authz/policy_export.go` | 新增 `ExportPersistentPolicies()` 和稳定导出 DTO。 |
| Controller | `controller/authz.go` | 新增 `ExportPermissionPolicies` 只读 handler。 |
| Router | `router/authz-router.go` | 新增 `GET /api/authz/policies/export`，叠加 `RootAuth` 与 `system_setting.secret_view`。 |
| 测试 | `service/authz/policy_export_test.go`、`router/authz_router_test.go` | 覆盖导出内容、排序、不导出 ID/时间戳、路由 handler 和 RootAuth 前置。 |

### 风险评估

1. 权限策略属于敏感管理元数据，必须保留 Root-only 边界，不能仅依赖 `system_setting.secret_view`。
2. 导出结果若包含数据库 ID，会让备份在不同环境间难以复用，因此只导出业务稳定字段。
3. 导出只读，不触发 seed、reload 或策略写入，避免影响现有 `Can()` 热路径和后台周期同步。
4. 排序固定为角色 `sort/key`、策略 `ptype/v0/v1/v2/v3/v4/v5`，降低审计和备份 diff 噪声。

### 方案评审

采用小步原生化方案：不引入 Casbin runtime/enforcer，不新增数据库表，不做导入写路径；先把已经落地的持久策略表做成稳定可观测能力。导出接口仍挂在 `/api/authz` 下，沿用 `permissionRoute` 注册模式，先经过 `AdminAuth`，再通过 `RootAuth` 保底，最后执行 `RequirePermission(system_setting.secret_view)`，保证后续权限矩阵和现有 Root-only 边界同时生效。

验收方式：

1. `go test ./service/authz` 覆盖导出数据契约和用户 override 镜像策略。
2. `go test ./router -run Authz` 覆盖路由 handler、permission 和 RootAuth 前置。
3. `git diff --check` 与 Prettier 文档检查确认格式无问题。
4. 通过 3003 真实登录后调用 `/api/authz/policies/export`，确认返回 `success=true`、包含 `roles/policies`，且策略导出不包含数据库 ID。

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

订阅管理后端 `/api/subscription/admin` 已按 `subscription.read/write/operate/sensitive_write` 接入权限表：套餐列表走 read，套餐创建/编辑走 write，套餐启停已在后续切片收窄为 operate，用户订阅创建/失效走 operate，用户订阅删除走 sensitive_write。上一轮已经为用户行内订阅弹窗补齐跨资源 `subscription.*` 消费；订阅管理页面本身仍只在路由入口校验 `subscription.read`，`Create Plan`、套餐行 `Edit`、套餐抽屉保存需要按 `subscription.write` 做按钮禁用与提交前保护，套餐 `Enable/Disable` 和启停确认需要按 `subscription.operate` 做按钮禁用与提交前保护。

本轮目标：

1. 复用 `useSubscriptionPermissions`，让订阅套餐页主按钮、行菜单、创建/编辑抽屉和启停确认弹窗按后端动作消费 `subscription.write`/`subscription.operate`。
2. 保留现有支付合规确认逻辑，不把 `subscription.write` 与合规状态混为一个条件；按钮禁用原因仍优先体现权限不足。
3. 该切片自身不修改订阅后端路由、套餐 payload、支付合规系统设置接口、用户购买流程和用户订阅弹窗逻辑；启停路由权限已在后续“订阅套餐启停操作权限原生化”切片同步调整。
4. 不新增可见文案，继续复用既有 `You don't have necessary permission`。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 订阅主按钮 | `web/default/src/features/subscriptions/components/subscriptions-primary-buttons.tsx` | `Create Plan` 需要 `subscription.write`，并保留支付合规确认约束。 |
| 订阅行操作 | `data-table-row-actions.tsx` | `Edit` 需要 `subscription.write`；`Enable/Disable` 已在后续切片收窄为 `subscription.operate`，查看或其他只读行为不变。 |
| 套餐抽屉 | `subscriptions-mutate-drawer.tsx` | 创建/编辑提交前检查 `subscription.write`，保存按钮按权限禁用。 |
| 启停确认弹窗 | `dialogs/toggle-status-dialog.tsx` | 确认启停前检查 `subscription.operate`，确认按钮按操作权限禁用。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验收标准，并补充能力落地清单。 |

### 风险评估

1. 本轮只改变前端按钮状态和提交前保护，不降低后端 `RequirePermission` enforcement；后端仍是最终授权边界。
2. 订阅套餐创建/编辑同时受支付合规确认约束和 `subscription.write` 约束；本轮仅增加权限约束，不改变合规确认流程。
3. 默认 Admin 拥有 `subscription.write` 和 `subscription.operate`，因此现有管理员工作流保持可用；用户级 override 可以分别关闭套餐编辑和套餐启停操作。
4. 用户订阅创建/失效/删除已在上一轮用户切片接入 `subscription.operate/sensitive_write`，本轮避免重复修改以降低冲突面。

### 方案评审

采用“复用订阅权限 hook + 按后端套餐路由动作映射”的方案。订阅套餐创建、编辑归入 `subscription.write`；套餐启停已在后续切片随路由权限表一起调整为 `subscription.operate`，避免前端放行而服务端 403。

动作映射：

| UI 操作 | 权限 | 后端对应路由 |
|---------|------|--------------|
| 查看订阅套餐列表 | `subscription.read` | `GET /api/subscription/admin/plans` |
| 创建套餐 | `subscription.write` | `POST /api/subscription/admin/plans` |
| 编辑套餐 | `subscription.write` | `PUT /api/subscription/admin/plans/:id` |
| 启用/禁用套餐 | `subscription.operate` | `PATCH /api/subscription/admin/plans/:id` |

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

## 本轮实施评审：Playground 消息编辑与会话操作工具化

### 需求分析

new-api-main 的 Playground 已把消息编辑器、编辑状态计算、上一条 user prompt 查找、重试截断、删除和编辑后提交等逻辑拆成独立组件与纯函数。NexusTok 当前虽然已经具备编辑、重试和错误恢复能力，但 `playground-chat.tsx` 仍直接持有 `editText`、`originalText`、编辑按钮 JSX 和上一条 user 查找；`index.tsx` 也直接拼接消息数组。这会让后续继续吸收 new-api-main 的消息内容拆分、来源折叠、历史裁剪和会话 hook 时，每一步都需要在大组件里反复改动同一块状态逻辑。

本轮目标是把 new-api-main 的低风险结构优势转成 NexusTok 原生能力：

1. 新增消息编辑状态 helper，集中计算 `canSave`、`hasChanged` 和“用户消息才显示 Save & Submit”的规则。
2. 新增会话消息数组 helper，集中处理发送、重试、删除、查找错误消息对应 user prompt、编辑保存和编辑后重新提交。
3. 新增 `PlaygroundMessageEditor` 组件，把编辑态 textarea 与 Save/Save & Submit/Cancel 按钮从 `playground-chat.tsx` 中移出。
4. 保持现有 `Textarea`、`Button`、消息类型、请求 payload、localStorage key 和视觉布局，不引入 new-api-main 的 `CodeBlockEditor`、历史裁剪、来源折叠或完整 message 目录迁移。
5. 为纯函数补充定向测试，覆盖编辑、重试和删除这些最容易影响 Playground 请求上下文的边界。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 消息 helper | `web/default/src/features/playground/lib/conversation-message-utils.ts`、`message-editor-utils.ts`、`lib/index.ts` | 新增可测试纯函数，封装消息数组变更和编辑状态计算。 |
| 消息编辑组件 | `web/default/src/features/playground/components/playground-message-editor.tsx`、`playground-chat.tsx` | 编辑态 JSX 从聊天组件中拆出，聊天组件只负责选择编辑/展示分支。 |
| Playground 容器 | `web/default/src/features/playground/index.tsx` | 发送、重试、删除、编辑保存改为调用 helper，避免在容器里手写数组截断。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 若将已有硬编码 `Save & Submit` 改为 `t()`，需补齐六语 key。 |
| 测试 | `web/default/src/features/playground/lib/conversation-message-utils.test.ts` | 覆盖消息追加、重试截断、上一条 user 查找、编辑保存和编辑后重新提交。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式，并补充落地清单。 |

### 风险评估

1. 本轮只移动默认前端 Playground 客户端消息编辑和数组操作，不触碰后端 Relay、计费、权限、数据库、模型请求 DTO、SSE 解析、存储 key 和参数配置。
2. 重试/编辑后提交会决定发给 `/pg/chat/completions` 的上下文数组，必须用纯函数测试确认：assistant 重试截断到该 assistant 之前，user 消息重新提交保留该 user 并追加新的 loading assistant。
3. 编辑保存必须保持原语义：空文本不可保存，未变化不可保存；assistant 消息只能保存，不触发重新提交；user 消息 Save & Submit 会丢弃该 user 之后的旧消息并追加新的 assistant 占位。
4. 拆出 `PlaygroundMessageEditor` 可能影响移动端按钮布局；本轮保留原来的文字按钮和 `Textarea`，不引入图标按钮或确认弹窗，减少视觉回归。
5. 新增测试使用 Bun/Node 内置测试风格，不新增测试依赖；若环境无法运行，应至少通过 ESLint、TypeScript、生产构建和 3003 资源验证。

### 方案评审

采用“纯函数先行 + 组件薄拆分”的方案：`conversation-message-utils.ts` 负责所有会话数组变更，`message-editor-utils.ts` 负责编辑器按钮状态，`PlaygroundMessageEditor` 只接收 `editText`、`originalText`、`message` 和回调。`playground-chat.tsx` 保留当前 `Branch`、`MessageContent`、`Response`、`Reasoning`、错误动作和空状态结构，只把编辑态渲染替换为新组件，并从 helper 获取上一条 user prompt；`index.tsx` 用 helper 生成新消息数组后再调用现有 `updateMessages` 和 `sendChat`。这样能为后续继续拆 `PlaygroundMessageContent` 或引入会话 hook 打基础，同时把行为变化控制在可测试函数内。

验收方式：

1. `cd web/default && bun test src/features/playground/lib/conversation-message-utils.test.ts` 覆盖本轮新增消息数组 helper。
2. `cd web/default && bun run i18n:sync` 确认新增或复用的编辑器文案进入 en、zh、fr、ja、ru、vi。
3. 针对本轮触碰文件运行定向 ESLint。
4. `cd web/default && ./node_modules/.bin/tsc -b` 覆盖新增 helper、组件 props 和容器调用。
5. `cd web/default && ./node_modules/.bin/rsbuild build` 覆盖生产构建和 Playground lazy chunk。
6. 使用 MCP 打开 `http://192.168.0.202:3003/playground` 检查编辑器按钮状态、保存、取消、错误消息 Edit Prompt 和控制台错误；若 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 确认新组件/文案特征已热更新。

验证记录：

1. `cd web/default && bun test src/features/playground/lib/conversation-message-utils.test.ts` 通过，8 条测试覆盖发送消息对、assistant/user 重试、上一条 user prompt 查找、编辑保存、编辑后提交、assistant 编辑不提交、删除和读取编辑内容。
2. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en、zh、fr、ja、ru、vi 均 `missingCount: 0`、`extrasCount: 0`，本轮新增 `Save & Submit` 六语 key 已归档。
3. `cd web/default && ./node_modules/.bin/eslint src/features/playground/lib/message-editor-utils.ts src/features/playground/lib/conversation-message-utils.ts src/features/playground/lib/conversation-message-utils.test.ts src/features/playground/lib/index.ts src/features/playground/components/playground-message-editor.tsx src/features/playground/components/playground-chat.tsx src/features/playground/index.tsx` 通过。
4. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认新增 helper、测试类型、编辑器 props 和容器接线类型正确。
5. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，Playground lazy chunk 切换为 `/static/js/async/9942.js`；`git diff --check` 通过。
6. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求替代页面验证。
7. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP/1.1 200 OK`。
8. 从 3003 拉取 `/static/js/index.js` 与 `/static/js/async/9942.js`，确认线上资源包含 `Save & Submit` 与新 chunk 编号 `9942`；线上资源与本地 `web/default/dist` 完全一致，旧 `/static/js/async/3854.js` 已返回 404，确认页面加载的是本轮构建产物。

## 本轮实施评审：Playground 消息内容渲染组件化

### 需求分析

new-api-main 的 Playground 已把单条消息的来源列表、推理内容、加载提示、错误卡片、Markdown 响应和消息动作整合到 `PlaygroundMessageContent` 组件，并用 `getMessageContentState` helper 集中计算显示状态。NexusTok 当前仍在 `playground-chat.tsx` 内部通过一段立即执行函数直接计算 `hasSources`、`showReasoning`、`showLoader`、`showMessageContent`、`displayContent`，然后内联渲染 `Sources`、`Reasoning`、`Loader/Shimmer`、`MessageError`、`Response` 和动作按钮。上一轮已经把编辑器和会话数组操作拆出，但聊天组件仍同时承担消息列表遍历和消息正文渲染，后续继续接入 new-api-main 的 message metadata、来源折叠、原始响应查看、消息布局模式时仍会反复触碰同一大块 JSX。

本轮目标是把 new-api-main 的消息内容拆分优势转成 NexusTok 原生能力，同时保持当前用户可见行为稳定：

1. 新增 `message-content-utils.ts`，集中计算消息来源、推理、加载、错误、正文可见性和 assistant `<think>` 正文剥离结果。
2. 新增 `PlaygroundMessageContent` 组件，封装来源列表、推理块、加载提示、错误卡片、正常响应和动作区。
3. `playground-chat.tsx` 只负责消息遍历、编辑态选择、错误恢复动作拼装和分支切换，不再直接内联正文渲染细节。
4. 保留现有 `MessageContent`、`Response`、`Reasoning`、`Sources`、`MessageError` 和 `MessageErrorActions` 行为；不迁移 new-api-main 的 raw response CodeBlock、message metadata、layout mode、source toggle 和历史裁剪。
5. 将已有硬编码 `Responding...` 纳入 i18n 六语翻译，避免新组件继续裸写英文。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 内容 helper | `web/default/src/features/playground/lib/message-content-utils.ts`、`message-content-utils.test.ts`、`lib/index.ts` | 新增可测试的消息正文状态计算，覆盖来源、推理、加载、正文和错误状态。 |
| 内容组件 | `web/default/src/features/playground/components/playground-message-content.tsx`、`playground-chat.tsx` | 单条消息正文渲染从聊天列表组件拆出，聊天组件变薄。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增 `Responding...` 六语翻译。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式，并补充落地清单。 |

### 风险评估

1. 本轮只改默认前端 Playground 的消息展示层，不触碰后端 Relay、计费、权限、数据库、请求 DTO、SSE 解析、消息存储 key 和会话数组生成逻辑。
2. `showLoader`、`showMessageContent` 和 `<think>` 正文剥离是流式体验关键路径，必须用定向测试覆盖：纯推理流不显示正文、空 streaming assistant 显示 loading、assistant 完整正文剥离 `<think>` 后再交给 `Response`。
3. 错误消息必须继续只显示 `MessageError` 和错误恢复动作，不能被正常 `Response` 分支同时渲染。
4. 来源列表必须保留在正文之前，且 key 需要稳定，避免来源重复或重新排序导致 React 误复用。
5. 不引入 raw response/metadata/layout mode 是本轮稳定性边界；这些能力依赖 CodeBlock 扩展、Message 类型时间字段和布局策略，需后续单独评审。

### 方案评审

采用“状态 helper + 内容组件”的小步原生化方案：`getMessageContentState(message, versionContent)` 输出 `sources`、`hasSources`、`hasReasoning`、`reasoningContent`、`showLoader`、`showMessageContent`、`displayContent`、`isError`、`isMessageFinal` 等派生状态；`PlaygroundMessageContent` 接收 `message`、当前 version 内容、普通动作和错误动作，内部按现有顺序渲染来源、推理、加载、错误或正文。`playground-chat.tsx` 仍保留 `Branch`/多版本切换、编辑器选择、上一条 user prompt 查找和动作回调拼装，避免一次性迁移 new-api-main 的完整 message 目录。设计方向延续当前工具台界面，组件化是结构增强，不改变卡片样式、布局密度和请求行为。

验收方式：

1. `cd web/default && bun test src/features/playground/lib/message-content-utils.test.ts` 覆盖新增消息内容状态 helper。
2. `cd web/default && bun run i18n:sync` 确认 `Responding...` 进入 en、zh、fr、ja、ru、vi。
3. 针对本轮触碰文件运行定向 ESLint。
4. `cd web/default && ./node_modules/.bin/tsc -b` 覆盖新增 helper、组件 props 和聊天组件接线。
5. `cd web/default && ./node_modules/.bin/rsbuild build` 覆盖生产构建和 Playground lazy chunk。
6. 使用 MCP 打开 `http://192.168.0.202:3003/playground` 检查普通消息、推理消息、错误消息和加载状态；若 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 确认 `Responding...` 和新 chunk 已热更新。

验证记录：

1. `cd web/default && bun test src/features/playground/lib/message-content-utils.test.ts` 通过，6 条测试覆盖 user 内容保留、assistant `<think>` 剥离、reasoning 展示、loading assistant、reasoning streaming 与来源/错误状态。
2. `cd web/default && bun test src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/conversation-message-utils.test.ts` 通过，Playground 消息相关 14 条测试全部通过。
3. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en、zh、fr、ja、ru、vi 均 `missingCount: 0`、`extrasCount: 0`，本轮新增 `Responding...` 六语 key 已归档。
4. `cd web/default && ./node_modules/.bin/eslint src/features/playground/lib/message-content-utils.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/index.ts src/features/playground/components/playground-message-content.tsx src/features/playground/components/playground-chat.tsx` 通过。
5. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认新增 helper、测试类型、内容组件 props 和聊天组件接线类型正确。
6. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，Playground lazy chunk 切换为 `/static/js/async/157.js`；`git diff --check` 通过。
7. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求替代页面验证。
8. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP/1.1 200 OK`。
9. 从 3003 拉取 `/static/js/index.js` 与 `/static/js/async/157.js`，确认线上资源包含 `Responding...`、`Reasoning`、`Save & Submit`、`Start a playground chat` 等特征；线上资源与本地 `web/default/dist` 完全一致，旧 `/static/js/async/9942.js` 已返回 404，确认页面加载的是本轮构建产物。

## 本轮实施评审：Playground 消息时间元信息原生化

### 需求分析

new-api-main 的 Playground 会在消息对象上记录 `createdAt`、`startedAt`、`completedAt`、`durationMs`，并通过 `MessageMetadata` 在消息下方展示发送时间和响应耗时。NexusTok 当前 Playground 已能稳定发送、编辑、重试和展示消息内容，但缺少消息级时间元信息；管理员或用户调试上游模型时，无法直接从 Playground 判断某次响应耗时，只能依赖浏览器网络面板或后端日志。这个差异影响调试效率，也让后续接入会话历史、消息裁剪和响应性能分析时缺少基础字段。

本轮目标是把 new-api-main 的时间元信息能力转成 NexusTok 原生能力，同时保持存储与请求兼容：

1. 将 `Message` 的 `createdAt`、`startedAt`、`completedAt`、`durationMs` 做成可选字段，新消息写入时间，旧 localStorage 消息不强制迁移。
2. 新增 `message-timing-utils.ts`，集中计算 assistant 完成耗时、reasoning 开始/完成耗时和元信息格式化。
3. 在流式、非流式、停止生成和错误路径调用 timing helper，确保完成、停止和错误消息都有稳定的耗时信息。
4. 新增 `MessageMetadata` 组件，在 `PlaygroundMessageContent` 的错误和正常正文下方显示消息时间与响应耗时。
5. 补齐 `Response time: {{duration}}`、`{{value}}s` 等 i18n 文案，并补定向测试覆盖 timing 计算和格式化。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 类型与消息工具 | `web/default/src/features/playground/types.ts`、`lib/message-utils.ts`、`lib/conversation-message-utils.ts` | 新增可选 timing 字段；创建 user/assistant 消息时写入 `createdAt/startedAt`；编辑后提交重置 user 时间。 |
| timing helper | `web/default/src/features/playground/lib/message-timing-utils.ts`、`message-timing-utils.test.ts`、`lib/index.ts` | 集中计算 assistant/reasoning 耗时和展示文案。 |
| 请求生命周期 | `web/default/src/features/playground/hooks/use-chat-handler.ts` | 流式完成、非流式完成、停止生成和错误落盘时补齐 `completedAt/durationMs`。 |
| 元信息组件 | `web/default/src/features/playground/components/message-metadata.tsx`、`playground-message-content.tsx` | 在消息内容下方展示发送时间和响应耗时。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 补齐响应耗时相关文案。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式，并补充落地清单。 |

### 风险评估

1. 本轮只在前端 Playground 消息对象上增加可选字段，不修改后端 Relay、计费、权限、数据库、请求 DTO、API payload 和 storage key。
2. localStorage 里旧消息没有 timing 字段，`MessageMetadata` 必须在字段缺失时返回 `null`，不能导致历史消息渲染异常。
3. 耗时计算必须以 assistant 的 `startedAt` 为准；如果历史消息缺字段，则以完成时间兜底，避免 `NaN` 或负数展示。
4. 错误和停止路径也要写入耗时，否则用户最需要排查的问题请求反而没有时间线。
5. UI 元信息必须保持低视觉权重，不能挤压消息正文或改变现有气泡/Markdown 布局；移动端需要保留换行空间。

### 方案评审

采用“可选字段 + helper + 轻量元信息”的方案：`Message` 增加可选 timing 字段，`createUserMessage` 和 `createLoadingAssistantMessage` 只在新消息上写入时间；`completeAssistantTiming` 和 `completeReasoningTiming` 负责完成态计算，`formatMessageTime` 和 `formatDuration` 负责展示文本。`use-chat-handler` 在原有 `finalizeMessage` 外层包 timing helper，不改变 SSE 解析和请求 payload；`updateAssistantMessageWithError` 内部也补完成耗时，保证错误路径一致。`MessageMetadata` 只在有 `createdAt` 或 `durationMs` 时显示，并放在 `PlaygroundMessageContent` 的错误/正文之后、动作之前。暂不引入 new-api-main 的 layout alignment 参数，NexusTok 当前仍保持现有消息布局。

验收方式：

1. `cd web/default && bun test src/features/playground/lib/message-timing-utils.test.ts` 覆盖 timing 计算和格式化。
2. `cd web/default && bun test src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/conversation-message-utils.test.ts src/features/playground/lib/message-timing-utils.test.ts` 覆盖本轮与既有消息 helper。
3. `cd web/default && bun run i18n:sync` 确认响应耗时文案进入 en、zh、fr、ja、ru、vi。
4. 针对本轮触碰文件运行定向 ESLint。
5. `cd web/default && ./node_modules/.bin/tsc -b` 覆盖新增字段、helper、组件 props 和 hook 接线。
6. `cd web/default && ./node_modules/.bin/rsbuild build` 覆盖生产构建和 Playground lazy chunk。
7. 使用 MCP 打开 `http://192.168.0.202:3003/playground` 检查消息时间/响应耗时显示；若 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 确认 `Response time` 等特征已热更新。

验证记录：

1. `cd web/default && bun test src/features/playground/lib/message-timing-utils.test.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/conversation-message-utils.test.ts` 通过，共 21 个用例，覆盖 timing 计算、消息内容状态和会话编辑/重提交流程。
2. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en、zh、fr、ja、ru、vi 的 `missingCount` 和 `extrasCount` 均为 0，本轮新增 `Response time: {{duration}}` 已写入六语 locale，`{{value}}s` 已存在并保持同步。
3. `cd web/default && ./node_modules/.bin/eslint src/features/playground/types.ts src/features/playground/lib/message-timing-utils.ts src/features/playground/lib/message-timing-utils.test.ts src/features/playground/lib/message-utils.ts src/features/playground/lib/conversation-message-utils.ts src/features/playground/lib/conversation-message-utils.test.ts src/features/playground/lib/index.ts src/features/playground/hooks/use-chat-handler.ts src/features/playground/components/message-metadata.tsx src/features/playground/components/playground-message-content.tsx` 通过。
4. `cd web/default && ./node_modules/.bin/tsc -b` 通过，新增可选字段、helper、组件 props 和 hook 接线类型检查正常。
5. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，Playground lazy chunk 切换为 `/static/js/async/1397.js`。
6. `git diff --check` 通过。
7. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求替代页面验证。
8. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP/1.1 200 OK`，其中 `/api/status` 返回 `success: true`。
9. 从 3003 拉取 `/static/js/index.js` 与 `/static/js/async/1397.js` 后，与本地 `web/default/dist` 完全一致：`index.js` SHA256 均为 `85fccd17941b0f2579c71cb7d18b4c02a2515108e75c7ace361d1258a5594c5e`，`1397.js` SHA256 均为 `e81e11e69520b91b54c02b6151421ea12629cc19d0fd0f3859adf4f4e12b1c3d`。
10. 线上 `/static/js/async/1397.js` 已包含 `Response time`、`durationMs`、`createdAt` 特征；上一轮旧 chunk `/static/js/async/157.js` 返回 `HTTP/1.1 404 Not Found`，确认 3003 页面加载的是本轮构建产物。

## 本轮实施评审：Playground 本地存储 schema 与容量保护

### 需求分析

new-api-main 的 Playground 本地存储已经把配置、参数开关和消息历史包进带版本号的 envelope，并用 Zod schema 校验加载内容；消息历史还包含数量上限、原始 localStorage 字节上限、单条内容上限、总加载内容上限、重复 Markdown 快照折叠和 pending assistant 稳定化。NexusTok 当前 `storage.ts` 仍直接 `JSON.parse` 裸 JSON，只在消息数组加载后做轻量 `sanitizeMessagesOnLoad`。如果用户浏览器中存在损坏 JSON、过大的 `playground_messages`、被流式刷新留下的长篇重复快照，页面初始化可能被卡住，或者每次进入 Playground 都重复处理异常数据。

本轮目标是把 new-api-main 的存储韧性转成 NexusTok 原生能力，同时保持现有使用习惯和兼容性：

1. 新增 Playground storage schema，使用当前前端已存在的 `zod` 校验配置、参数开关和消息结构。
2. 保存时写入 `{ version: 1, data }` envelope；加载时兼容旧版裸 JSON，不改 `STORAGE_KEYS`，不要求用户手动清缓存。
3. 对 `playground_messages` 增加原始大小、消息数量、单条正文、reasoning 内容和总体加载内容保护，避免异常 localStorage 拖垮页面。
4. 对含有内容或 reasoning 的 pending assistant 消息在加载时补齐完成态和耗时；对空 pending assistant 继续交给现有中断清理逻辑转成错误态。
5. 不新增用户可见文案、不改变 `/pg/chat/completions` payload、不触碰后端 Relay、计费、权限、数据库或模型配置。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| storage schema | `web/default/src/features/playground/lib/storage-schema.ts` | 新增 storage 版本、容量上限常量，以及配置、参数开关和消息历史 Zod schema。 |
| storage 读写 | `web/default/src/features/playground/lib/storage.ts` | 读取兼容 envelope 与旧裸 JSON；保存写 envelope；消息加载增加大小限制、裁剪、重复快照折叠和 pending 状态稳定化。 |
| storage 测试 | `web/default/src/features/playground/lib/storage.test.ts` | 覆盖旧数据兼容、envelope 写入、无效数据兜底、消息数量裁剪、超大存储清理、内容截断和 pending 消息恢复。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式，并补充落地清单。 |

### 风险评估

1. 旧用户浏览器里已经存在裸 JSON 配置、参数开关和消息数组，schema 兼容必须足够宽松；加载失败应返回默认空状态而不是阻断页面。
2. 消息 schema 过严会导致历史合法消息丢弃；本轮只校验当前 `Message` 类型中真实使用的字段，所有 timing、reasoning 和错误字段保持可选。
3. 容量保护必须只影响 Playground 本地历史，不影响当前请求上下文、后端协议或计费；保存失败只记录 `console.error`，不能打断页面交互。
4. 裁剪长内容可能丢失很旧或超长的本地历史；但限制只在加载和保存历史时生效，优先保留最近消息，目标是恢复页面可用性。
5. pending assistant 稳定化必须避免刷新后继续显示“正在响应”；有内容的消息转完成态，空占位转现有中断错误态，以减少误导。

### 方案评审

采用“schema/envelope 原生化 + 加载期防护”的方案：新增 `storage-schema.ts` 承载版本号、容量上限和 Zod schema；`storage.ts` 继续作为唯一读写入口，内部通过 `unwrapStoredValue` 兼容旧裸 JSON，通过 `writeStoredValue` 统一写 `{ version, data }`。消息加载流程为：先检查原始 `playground_messages` 字符串是否超过 1MB，过大则删除并返回空历史；再解包并用 schema 校验；随后按单条 40,000 字符和总计 120,000 字符裁剪正文/reasoning，折叠重复 Markdown section 快照；最后调用现有 `sanitizeMessagesOnLoad` 处理空 pending assistant。保存消息时最多保留最近 100 条并写入 envelope。该方案不新增 UI、不新增路由、不改变 storage key，不会影响核心 Relay 与计费路径。

验收方式：

1. `cd web/default && bun test src/features/playground/lib/storage.test.ts src/features/playground/lib/message-timing-utils.test.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/conversation-message-utils.test.ts` 覆盖本轮 storage 逻辑和既有 Playground helper。
2. `cd web/default && bun run i18n:sync` 确认无新增可见文案且六语 locale 仍同步。
3. 针对本轮触碰文件运行定向 ESLint。
4. `cd web/default && ./node_modules/.bin/tsc -b` 覆盖 schema、storage helper 和测试类型。
5. `cd web/default && ./node_modules/.bin/rsbuild build` 覆盖生产构建和 Playground lazy chunk。
6. `git diff --check` 检查空白与补丁格式。
7. 使用 MCP 打开 `http://192.168.0.202:3003/playground` 并检查页面加载；若 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 与本地 `dist` 比对，确认 3003 页面加载的是本轮构建产物。

验证记录：

1. `cd web/default && bun test src/features/playground/lib/storage.test.ts src/features/playground/lib/message-timing-utils.test.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/conversation-message-utils.test.ts` 通过，共 31 个用例，覆盖 storage envelope、旧裸 JSON 兼容、无效配置兜底、参数开关保存、消息数量裁剪、超大消息存储清理、单条/总体内容裁剪、重复 Markdown section 快照折叠和 pending assistant 恢复。
2. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en、zh、fr、ja、ru、vi 的 `missingCount` 和 `extrasCount` 均为 0，本轮未新增用户可见文案。
3. `cd web/default && ./node_modules/.bin/eslint src/features/playground/lib/storage-schema.ts src/features/playground/lib/storage.ts src/features/playground/lib/storage.test.ts src/features/playground/lib/message-utils.ts src/features/playground/lib/message-timing-utils.ts` 通过。
4. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 storage schema、localStorage 读写 helper 和测试类型正常。
5. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，Playground 相关 lazy chunk 切换为 `/static/js/async/9008.js`。
6. `git diff --check` 通过。
7. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求替代页面验证。
8. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP/1.1 200 OK`，其中 `/api/status` 返回 `success: true`。
9. 从 3003 拉取 `/static/js/index.js` 与 `/static/js/async/9008.js` 后，与本地 `web/default/dist` 完全一致：`index.js` SHA256 均为 `25ea44dc944c7a99a20f34bcaecaf0f1ce5cf470a7adf7b1e06fe9e2a76aa69c`，`9008.js` SHA256 均为 `e72a3b97e2c58bc626261832d5ed80e458060b839866161048776b02feebd0f7`。
10. 线上 `/static/js/async/9008.js` 已包含 `playground_messages`、`playground_config` 特征；上一轮旧 chunk `/static/js/async/1397.js` 返回 `HTTP/1.1 404 Not Found`，确认 3003 页面加载的是本轮构建产物。

## 本轮实施评审：Playground 选项加载与会话 hook 原生化

### 需求分析

new-api-main 的 Playground 已把“模型/分组选项加载”和“会话消息操作”拆成 `usePlaygroundOptions`、`usePlaygroundConversation` 以及对应的纯函数工具。NexusTok 当前仍在 `web/default/src/features/playground/index.tsx` 里直接写 `useQuery`、模型/分组 fallback、发送、重试、编辑、删除等逻辑；同时 `/api/user/models` 无 `group` 查询时只返回用户所有可用分组的模型集合，切换 Playground 分组后模型下拉不能按当前分组收敛。这个差异会让后续继续吸收 new-api 的 layout、reasoning、raw response、会话历史能力时反复修改容器组件，也会让用户在某些分组下选到该分组不可用的模型。

本轮目标是把 new-api-main 的低风险结构优势转成 NexusTok 原生能力：

1. 后端 `/api/user/models` 增加可选 `group` 查询参数；参数为空时保持旧聚合行为，参数为用户不可用分组时返回空数组，避免泄露其它分组模型。
2. 前端 `getUserModels(group?)` 支持按分组请求模型，`usePlaygroundOptions` 使用 `['playground-models', currentGroup]` 查询 key，让分组变化能刷新模型候选。
3. 新增 option helper，集中处理模型 fallback、分组 fallback、当前分组无模型时清空模型，以及加载错误消息提取。
4. 新增 `usePlaygroundConversation`，把发送、重试、编辑、删除和编辑态 key 从 `index.tsx` 移到专用 hook；容器组件只负责接线。
5. 对新增错误 toast 文案补齐 en、zh、fr、ja、ru、vi，并保持无新增页面布局或交互形态。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 后端模型查询 | `controller/user.go`、相关 Go 测试 | `/api/user/models?group=<name>` 返回当前用户可用分组内启用模型；无参数兼容旧行为。 |
| Playground API | `web/default/src/features/playground/api.ts` | `getUserModels` 接收可选 group 并作为 query params 传给后端。 |
| Playground options hook | `web/default/src/features/playground/hooks/use-playground-options.ts`、`lib/playground-option-utils.ts` | 分离模型/分组查询、错误 toast、fallback 和清空模型逻辑。 |
| Playground conversation hook | `web/default/src/features/playground/hooks/use-playground-conversation.ts`、`index.tsx` | 发送、重试、编辑、删除和编辑态由 hook 管理，容器组件瘦身。 |
| 状态 helper | `web/default/src/features/playground/lib/playground-state-utils.ts`、`hooks/use-playground-state.ts` | 初始配置、参数开关、消息加载和消息 updater 应用集中为纯函数。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增模型/分组加载失败 toast 文案。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式，并补充落地清单。 |

### 风险评估

1. 后端 `group` 参数必须是可选兼容增强；旧前端、第三方调用和无参数请求仍返回所有用户可用分组模型集合。
2. 传入用户不可用分组时必须返回空数组而不是 fallback 到全部模型，否则可能误导前端并扩大模型信息可见范围。
3. 前端分组变化会触发模型重查；如果当前模型不在新列表中，应优先切到新列表第一项；如果新列表为空则清空模型，避免继续向上游发送旧分组不可用模型。
4. 错误 toast 只在 query error 时出现，不能把业务成功但空列表误报为失败；新增文案必须补齐六语。
5. 会话 hook 只移动现有消息操作，不改变请求 payload、localStorage key、消息结构和 UI 布局；删除消息继续用 updater 读取最新状态，避免闭包陈旧。

### 方案评审

采用“后端兼容增强 + 前端 hook 瘦身”的方案：后端 `GetUserModels` 在读取用户可用分组后检查 `c.Query("group")`，合法分组直接返回 `model.GetGroupEnabledModels(group)`，非法分组返回空数组，无参数沿用旧去重聚合逻辑。前端新增 `playground-option-utils.ts` 与 `usePlaygroundOptions`，模型查询 key 绑定当前 group，分组查询独立；当模型列表变化时先设置 `models`，再用 helper 判断是否 fallback 或清空。新增 `usePlaygroundConversation` 复用既有 `appendUserMessagePair`、`createRegeneratedMessages`、`applyMessageEdit` 和 `removeMessageByKey`，不引入新的消息语义。`index.tsx` 移除内联 `useQuery/useEffect/useState`，仅组合 state、chat handler、options hook 和 conversation hook。

验收方式：

1. Go 测试覆盖 `/api/user/models?group=...` 的合法分组、不可用分组和无参数兼容行为。
2. `cd web/default && bun test` 定向覆盖 option/state/conversation helper 和既有 Playground helper。
3. `cd web/default && bun run i18n:sync` 确认新增 toast 文案进入六语 locale。
4. 针对本轮触碰文件运行 Go/TS 定向 ESLint 与 `go test`。
5. `cd web/default && ./node_modules/.bin/tsc -b` 和 `./node_modules/.bin/rsbuild build` 覆盖前端类型与生产构建。
6. `git diff --check` 检查补丁空白。
7. 使用 MCP 打开 `http://192.168.0.202:3003/playground` 并检查模型/分组请求；若 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并在登录态可用时调用 `/api/user/models?group=default`；同时拉取线上 chunk 与本地 `dist` 比对，确认 3003 页面加载的是本轮构建产物。

验证记录：

1. `go test ./controller -run 'TestBuildUserModelsForGroups'` 通过，覆盖合法分组过滤、不可用分组返回空数组和无参数聚合去重兼容行为。
2. `cd web/default && bun test src/features/playground/lib/playground-option-utils.test.ts src/features/playground/lib/playground-state-utils.test.ts src/features/playground/lib/storage.test.ts src/features/playground/lib/message-timing-utils.test.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/conversation-message-utils.test.ts` 通过，共 40 个用例，覆盖 option fallback、模型清空、消息 updater、storage 和既有消息 helper。
3. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en、zh、fr、ja、ru、vi 的 `missingCount` 和 `extrasCount` 均为 0，本轮新增 `Failed to load playground models`、`Failed to load playground groups` 已补齐六语。
4. `cd web/default && ./node_modules/.bin/eslint src/features/playground/api.ts src/features/playground/index.tsx src/features/playground/hooks/use-playground-options.ts src/features/playground/hooks/use-playground-conversation.ts src/features/playground/hooks/use-playground-state.ts src/features/playground/hooks/index.ts src/features/playground/lib/playground-option-utils.ts src/features/playground/lib/playground-option-utils.test.ts src/features/playground/lib/playground-state-utils.ts src/features/playground/lib/playground-state-utils.test.ts src/features/playground/lib/index.ts` 通过。
5. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 `getUserModels(group?)`、`usePlaygroundOptions`、`usePlaygroundConversation` 和状态 helper 类型正确。
6. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，Playground 相关 lazy chunk 切换为 `/static/js/async/1750.js`。
7. `git diff --check` 通过。
8. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求替代页面验证。
9. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP/1.1 200 OK`，其中 `/api/status` 返回 `success: true`。
10. 使用账号 `c1cada` 登录 `http://192.168.0.202:3003/api/user/login` 成功，随后携带 session cookie 与 `NexusTok-User: 1` 调用 `/api/user/models?group=default` 返回 `["gpt-5.4","gpt-5.5","gpt-5.6-sol"]`；调用 `/api/user/models?group=__not_allowed__` 返回空数组；无参数 `/api/user/models` 仍返回聚合模型列表，确认 3003 后端 group 过滤兼容增强已生效。
11. 从 3003 拉取 `/static/js/index.js` 与 `/static/js/async/1750.js` 后，与本地 `web/default/dist` 完全一致：`index.js` SHA256 均为 `1c99b1e25173e4fc843bb58569f125980f97c94039080b660669d6347d0c9b3e`，`1750.js` SHA256 均为 `7aa87ffb2bb90423df6c0527e72dd5e305941e6c196848b87b403a76a5a1d5c4`。
12. 线上 `/static/js/async/1750.js` 已包含 `playground-models`、`Failed to load playground models` 特征；上一轮旧 chunk `/static/js/async/9008.js` 返回 `HTTP/1.1 404 Not Found`，确认 3003 页面加载的是本轮构建产物。

## 本轮实施评审：Playground 消息基础工具拆分

### 需求分析

new-api-main 的 Playground 将消息基础能力继续拆成 `message-reasoning-utils`、`message-update-utils` 和 `message-action-utils`：`<think>` 标签解析、最后一条 assistant 更新、错误消息落盘、动作按钮可见性和内容状态都由独立纯函数提供。NexusTok 当前这些逻辑仍集中在 `message-utils.ts` 和 `MessageActions` 组件中；虽然已经具备功能，但职责过重，后续继续接入 raw response/source toggle、移动端动作菜单、layout mode 或更完整 reasoning 展示时，会反复触碰同一大文件和 JSX 内联判断。

本轮目标是继续吸收 new-api-main 的结构优势，但不改变用户可见行为：

1. 新增 `message-reasoning-utils.ts`，承载 `<think>` 完整/未闭合标签解析，供 streaming、finalize、消息内容状态和 storage 共享。
2. 新增 `message-update-utils.ts`，承载 `updateLastAssistantMessage` 和 `updateAssistantMessageWithError`，并保留当前错误标题、errorCode、reasoning timing 和 assistant timing 语义。
3. 新增 `message-action-utils.ts`，集中计算 MessageActions 所需的 `content/hasContent/isAssistant/isUser/isLoading` 和 hover 可见性 class。
4. `message-utils.ts` 回归消息创建、版本读取、内容更新、API 格式化和请求有效性判断；不改消息结构、不改 storage key、不改 `/pg/chat/completions` payload。
5. 为三类纯函数补定向测试，覆盖 think 标签边界、最后一条 assistant 更新、错误消息完成时间、动作状态和可见性 class。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| reasoning helper | `web/default/src/features/playground/lib/message-reasoning-utils.ts`、`message-reasoning-utils.test.ts` | 从 `message-utils.ts` 拆出 `<think>` 解析，供消息展示、流式处理和存储恢复复用。 |
| update helper | `web/default/src/features/playground/lib/message-update-utils.ts`、`message-update-utils.test.ts` | 从 `message-utils.ts` 拆出 assistant 错误更新和最后一条 assistant 更新逻辑。 |
| action helper | `web/default/src/features/playground/lib/message-action-utils.ts`、`message-action-utils.test.ts` | 从 `MessageActions` 内联状态拆出可测试动作状态和可见性 class。 |
| 调用点 | `message-utils.ts`、`message-content-utils.ts`、`storage.ts`、`components/message-actions.tsx`、`lib/index.ts` | 改为引用拆分后的 helper；MessageActions 只使用 helper 结果，不改变按钮渲染结构。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验收方式，并补充落地清单。 |

### 风险评估

1. `<think>` 解析是流式 reasoning、最终正文清理和本地存储恢复共同依赖的核心逻辑；拆分时必须保持完整标签、多段标签和未闭合标签的旧语义。
2. `updateAssistantMessageWithError` 位于流式/非流式错误路径，必须继续补齐 assistant/reasoning 完成时间，避免错误消息缺少耗时元信息。
3. MessageActions 的 `hasContent` 从工具函数读取时，应与消息当前版本一致，不能让 loading 占位消息显示复制/编辑动作；按钮外观、tooltip 和禁用逻辑保持原状。
4. 本轮不引入 new-api-main 的 raw response/source toggle、移动端 DropdownMenu 或 layout mode，避免把纯函数拆分扩大成可见 UI 改造。
5. 不新增用户可见文案，不触碰 i18n；如测试发现现有未翻译文案，不在本轮顺手扩散。

### 方案评审

采用“先抽纯函数，再替换调用点”的小步方案：`message-reasoning-utils.ts` 提供 `parseThinkTags`，`message-utils.ts` 内部导入它继续处理 streaming/finalize；`message-update-utils.ts` 导入 `updateCurrentVersionContent` 与 timing helper，实现错误更新并由 `lib/index.ts` 导出；`message-action-utils.ts` 通过 `getCurrentVersion` 读取当前内容，供 `MessageActions` 组件使用。测试先覆盖新 helper，再跑既有 Playground message/storage 测试，确认行为不变。视觉层只替换内联判断，不新增 shadcn 组件、不改变 class 结构、不改文案。

验收方式：

1. `cd web/default && bun test src/features/playground/lib/message-reasoning-utils.test.ts src/features/playground/lib/message-update-utils.test.ts src/features/playground/lib/message-action-utils.test.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/storage.test.ts src/features/playground/lib/conversation-message-utils.test.ts src/features/playground/lib/message-timing-utils.test.ts` 覆盖本轮 helper 与既有 Playground 消息链路。
2. 针对本轮触碰文件运行定向 ESLint。
3. `cd web/default && ./node_modules/.bin/tsc -b` 覆盖拆分后的导出和调用点类型。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 覆盖生产构建和 Playground lazy chunk。
5. `git diff --check` 检查补丁空白。
6. 使用 MCP 打开 `http://192.168.0.202:3003/playground` 并检查页面加载；若 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 与本地 `dist` 比对，确认 3003 页面加载的是本轮构建产物。

验证记录：

1. `cd web/default && bun test src/features/playground/lib/message-reasoning-utils.test.ts src/features/playground/lib/message-update-utils.test.ts src/features/playground/lib/message-action-utils.test.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/storage.test.ts src/features/playground/lib/conversation-message-utils.test.ts src/features/playground/lib/message-timing-utils.test.ts` 通过，共 43 个用例，覆盖 think 标签解析、assistant 错误更新、动作状态、消息内容状态、storage 恢复和既有会话 helper。
2. `cd web/default && ./node_modules/.bin/eslint src/features/playground/lib/message-reasoning-utils.ts src/features/playground/lib/message-reasoning-utils.test.ts src/features/playground/lib/message-update-utils.ts src/features/playground/lib/message-update-utils.test.ts src/features/playground/lib/message-action-utils.ts src/features/playground/lib/message-action-utils.test.ts src/features/playground/lib/message-utils.ts src/features/playground/lib/message-content-utils.ts src/features/playground/lib/storage.ts src/features/playground/lib/index.ts src/features/playground/components/message-actions.tsx` 通过。
3. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认拆分后的 helper 导出、MessageActions 调用和 storage/message content 引用类型正确。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，Playground 相关 lazy chunk 切换为 `/static/js/async/6948.js`。
5. `git diff --check` 通过。
6. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求替代页面验证。
7. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP/1.1 200 OK`，其中 `/api/status` 返回 `success: true`。
8. 从 3003 拉取 `/static/js/index.js` 与 `/static/js/async/6948.js` 后，与本地 `web/default/dist` 完全一致：`index.js` SHA256 均为 `0d746516191d50557c44fcf327acac696aa690b64e0388d2edff12d0163800a2`，`6948.js` SHA256 均为 `7e21046a08f310bc54f1795b4668159bde13c2bd38d0bbfa91fd89b1bba94d0a`。
9. 线上 `/static/js/async/6948.js` 已包含 `hasUnclosedTag`、`max-md:opacity-100` 特征；上一轮旧 chunk `/static/js/async/1750.js` 返回 `HTTP/1.1 404 Not Found`，确认 3003 页面加载的是本轮构建产物。

## 本轮实施评审：Playground 原始响应切换原生化

### 需求分析

new-api-main 的 Playground 在 assistant 消息完成后提供 source/preview 切换：默认展示 Markdown 预览，用户需要排查模型原始输出、Markdown 解析差异、工具返回片段或流式拼接问题时，可以切到 raw response 视图查看当前版本的原始内容。NexusTok 当前已经具备受控 Markdown 渲染、推理块、来源块、错误恢复、消息时间元信息和基础动作栏，但仍缺少“原始响应”这一调试视角；管理员或高级用户只能复制预览内容，无法在页面内直接对比解析前文本。

本轮目标是吸收 new-api-main 的低风险交互优势，同时保持 NexusTok 当前 Playground 的原生结构：

1. 为非 loading/streaming 的 assistant 消息新增“Show source / Show preview”动作，只有消息有正文且调用方提供切换能力时展示。
2. 在 `PlaygroundChat` 内按 message key 维护原始响应可见集合；该状态只影响当前页面会话展示，不写入 localStorage，避免污染消息协议和历史兼容。
3. 在 `PlaygroundMessageContent` 内支持 `isSourceVisible`，切换后使用现有 `CodeBlock` 展示当前版本 `versionContent`，保留复制按钮和行号。
4. 新增动作状态 helper，集中判断 source toggle 是否可用、按钮文案和 source key set 更新，减少 JSX 内联条件。
5. 补齐 `Show source`、`Show preview`、`Raw response` 六语翻译，并用定向测试覆盖动作可见性和 source key set 切换。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Playground 聊天编排 | `web/default/src/features/playground/components/playground-chat.tsx` | 新增原始响应可见状态，并把切换回调、当前可见状态传给消息动作和内容组件。 |
| 消息动作栏 | `web/default/src/features/playground/components/message-actions.tsx`、`message-action-button.tsx` | 增加 source/preview 切换动作；按钮仍复用现有 Tooltip + ghost icon button，不引入移动端 DropdownMenu。 |
| 消息内容 | `web/default/src/features/playground/components/playground-message-content.tsx` | 在 source 可见时用 `CodeBlock` 渲染原始版本内容；默认预览、推理、来源、错误和时间元信息保持原语义。 |
| 纯函数工具 | `web/default/src/features/playground/lib/message-action-utils.ts`、`message-source-utils.ts` 及测试 | 把按钮可见条件、文案选择、source key set 切换收敛为可测试 helper。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增 raw/source toggle 相关文案。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验收方式与后续验证结果。 |

### 风险评估

1. source toggle 不能出现在 loading/streaming 消息上，否则用户会看到半截 raw 文本并误以为最终响应异常；必须等 assistant 消息完成且有正文后才展示。
2. source 可见状态若写入消息结构或 localStorage，会带来历史 schema 迁移和跨版本兼容成本；本轮只使用组件内 UI 状态。
3. `versionContent` 是当前分支版本的原始文本，多版本消息切换时必须跟随 branch 渲染内容，不应改写 `getMessageContentState` 的预览解析结果。
4. 当前 NexusTok `CodeBlock` 不支持 new-api-main 的 `title/collapsedLines/showToolbar` props；直接照搬会造成类型错误，因此本轮采用现有 CodeBlock 能力，并用轻量标题文本承载 `Raw response`。
5. 本轮不迁移 new-api-main 的 mobile DropdownMenu、message layout mode 和 alignment metadata，避免把调试能力和布局重构耦合到同一改动。
6. 新增 UI 文案必须补齐六语并跑 i18n sync；按钮 tooltip 需要使用翻译后的文案，避免英文硬编码漏出。

### 方案评审

采用“小步原生化”方案：新增 `message-source-utils.ts` 提供 `toggleMessageSourceKey`，`message-action-utils.ts` 增加 `canToggleMessageSource` 和 `getMessageSourceToggleLabel`；`PlaygroundChat` 使用 `useState<ReadonlySet<string>>` 维护可见 key 集合，并将 `handleToggleMessageSource` 传给 `MessageActions`。动作栏新增可选 `onToggleSource` 与 `isSourceVisible` props，只有 assistant、非 loading、有内容时渲染 source 按钮。内容组件新增 `isSourceVisible`，source 模式下在消息内容位置渲染 `Raw response` 标题和 `CodeBlock code={versionContent} language="markdown" showLineNumbers`，继续显示元信息与通用动作。视觉层沿用现有操作台密度和 shadcn Base UI 组合，不新增卡片套卡片，不改变消息布局。

验收方式：

1. `cd web/default && bun test src/features/playground/lib/message-action-utils.test.ts src/features/playground/lib/message-source-utils.test.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/conversation-message-utils.test.ts src/features/playground/lib/message-timing-utils.test.ts src/features/playground/lib/storage.test.ts` 覆盖 source toggle helper、动作可见性和既有消息链路。
2. `cd web/default && bun run i18n:sync`，确认 en、zh、fr、ja、ru、vi 无 missing/extras。
3. 针对本轮触碰文件运行定向 ESLint。
4. `cd web/default && ./node_modules/.bin/tsc -b` 和 `./node_modules/.bin/rsbuild build` 覆盖类型与生产构建。
5. `git diff --check` 检查补丁空白。
6. 优先使用 MCP 打开 `http://192.168.0.202:3003/playground` 验证按钮和 raw response 切换；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 与本地 `dist` 比对，确认 3003 页面加载的是本轮构建产物。

验证记录：

1. `cd web/default && bun test src/features/playground/lib/message-action-utils.test.ts src/features/playground/lib/message-source-utils.test.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/conversation-message-utils.test.ts src/features/playground/lib/message-timing-utils.test.ts src/features/playground/lib/storage.test.ts` 通过，共 39 个用例，覆盖 source toggle 条件、source key set 不可变更新、消息内容状态、会话 helper、timing 和 storage 兼容链路。
2. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en、zh、fr、ja、ru、vi 的 `missingCount` 和 `extrasCount` 均为 0。本轮新增 `Raw response`、`Show preview`、`Show source` 已补齐六语；报告中的 untranslated 计数为仓库既有存量。
3. `cd web/default && ./node_modules/.bin/eslint src/features/playground/constants.ts src/features/playground/components/message-actions.tsx src/features/playground/components/playground-chat.tsx src/features/playground/components/playground-message-content.tsx src/features/playground/lib/message-action-utils.ts src/features/playground/lib/message-action-utils.test.ts src/features/playground/lib/message-source-utils.ts src/features/playground/lib/message-source-utils.test.ts src/features/playground/lib/index.ts src/i18n/static-keys.ts` 通过。
4. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 `MessageActions` 新增 props、`PlaygroundMessageContent` source 模式和 helper 导出类型正确。
5. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，Playground 相关 lazy chunk 切换为 `/static/js/async/92.js`。
6. `git diff --check` 通过。
7. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求与线上资源比对替代页面验证。
8. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP 200`；`/api/status` 返回 `success: true`。
9. 从 3003 拉取 `/static/js/index.js` 与 `/static/js/async/92.js` 后，与本地 `web/default/dist` 完全一致：`index.js` SHA256 均为 `e8b7995c396e7ee0ead4f34816ef97e5fb1c79a85f980335b42763fcb9f28956`，`92.js` SHA256 均为 `4ded5331fdb8d0e8fec0157c4be125f1545b57d6a1b3c3e4292a98010c8caf3e`。
10. 线上 `/static/js/async/92.js` 已包含 `Raw response`、`Show source` 特征；上一轮旧 chunk `/static/js/async/6948.js` 返回 `HTTP 404`，确认 3003 页面加载的是本轮构建产物。

## 本轮实施评审：Playground 推理耗时展示原生化

### 需求分析

new-api-main 的 Playground 在渲染 reasoning 块时会把 `message.reasoning?.duration` 传给公共 `Reasoning` 组件，用户可以看到“Thinking...”或“Thought for N seconds”这类推理耗时提示。NexusTok 前序轮次已经原生化了 `reasoning.startedAt`、`reasoning.completedAt`、`reasoning.durationMs` 和 `reasoning.duration` 的记录逻辑，也已经在消息元信息里展示整体响应耗时，但 `PlaygroundMessageContent` 当前没有把 `message.reasoning?.duration` 继续传入 `Reasoning`，导致推理块标题只能落回组件内部默认耗时，历史消息与非即时渲染场景无法准确体现已计算的 reasoning 时长。

本轮目标是把已有 timing 数据闭环到 UI 层，同时顺手消除公共 `Reasoning` 组件中的英文硬编码：

1. `PlaygroundMessageContent` 在存在 reasoning 时向 `Reasoning` 传入 `duration={message.reasoning?.duration}`，复用现有消息结构，不新增协议字段。
2. `ReasoningTrigger` 使用 `useTranslation()` 渲染 `Thinking...`、`Thought for a few seconds` 和 `Thought for {{duration}} seconds`，避免默认前端在非英文语言下泄漏英文。
3. 当 duration 缺失或为 0 时仍展示泛化的“几秒”提示；只有 duration 为正数时展示精确秒数。
4. 补齐 en、zh、fr、ja、ru、vi 六语翻译，并通过 i18n sync 校验 key 完整性。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 公共 AI 元素 | `web/default/src/components/ai-elements/reasoning.tsx` | 引入 `useTranslation`，将 trigger 的 thinking/thought 文案从英文硬编码改为 i18n；保留组件 props、context 和自动展开/收起行为。 |
| Playground 消息内容 | `web/default/src/features/playground/components/playground-message-content.tsx` | 将已记录的 `message.reasoning?.duration` 传给 `Reasoning`；不改变来源、错误、原始响应切换、正文渲染和元信息顺序。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`、`web/default/src/i18n/static-keys.ts` | 新增 reasoning trigger 的三条文案，保持 flat JSON key 规则和变量占位符一致。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验收方式和最终验证记录。 |

### 风险评估

1. 本轮只读取并展示已有 `message.reasoning.duration`，不改 SSE 解析、`<think>` 解析、finalize、storage schema、请求体、计费或上游 relay，核心业务链路风险低。
2. `Reasoning` 是公共组件，文案 i18n 化会影响所有使用它的页面；但输出语义保持原样，且 key 使用英文源字符串，符合默认前端 i18n 体系。
3. duration 缺失、旧 localStorage 消息、非流式响应或耗时小于 1 秒时可能得到 0；保留原有泛化提示，避免显示 “0 seconds” 造成误读。
4. `useTranslation()` 放在 `ReasoningTrigger` 中会让该 trigger 订阅语言变化；组件已是 client component，额外渲染成本极小。
5. 新增 `{{duration}}` 占位符必须在六语中完整保留，否则 i18n sync 或运行时插值会出现异常。

### 方案评审

采用“小闭环展示 + 公共文案 i18n”方案：`PlaygroundMessageContent` 只在现有 `<Reasoning>` 上补 `duration={message.reasoning?.duration}`；`reasoning.tsx` 将 `getThinkingMessage` 改为接收 `t`，streaming 时返回 `<Shimmer>{t('Thinking...')}</Shimmer>`，无有效 duration 时返回 `t('Thought for a few seconds')`，duration 为正数时返回 `t('Thought for {{duration}} seconds', { duration })`。这个方案不会引入新状态，也不会把展示逻辑拆入 Playground 私有 helper，原因是 thinking 文案属于公共 `Reasoning` trigger 的职责；同时保持当前 shadcn/Base UI 的 `Collapsible` 组合、视觉密度和图标结构不变。

验收方式：

1. `cd web/default && bun run i18n:sync`，确认 en、zh、fr、ja、ru、vi 无 missing/extras，并检查新增 key 的占位符一致。
2. `cd web/default && bun test src/features/playground/lib/message-timing-utils.test.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/message-action-utils.test.ts src/features/playground/lib/storage.test.ts`，覆盖 timing、内容状态和既有动作/storage 链路。
3. 针对 `reasoning.tsx`、`playground-message-content.tsx` 和 i18n static keys 运行定向 ESLint。
4. `cd web/default && ./node_modules/.bin/tsc -b` 和 `./node_modules/.bin/rsbuild build`，覆盖公共组件类型和生产构建。
5. `git diff --check` 检查补丁空白。
6. 优先使用 MCP 打开 `http://192.168.0.202:3003/playground` 验证页面加载和控制台状态；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 与本地 `dist` 比对，确认 3003 页面加载的是本轮构建产物。

验证记录：

1. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en、zh、fr、ja、ru、vi 的 `missingCount` 和 `extrasCount` 均为 0。本轮新增 `Thinking...`、`Thought for a few seconds`、`Thought for {{duration}} seconds` 已补齐六语；报告中的 untranslated 计数为仓库既有存量。
2. `cd web/default && bun test src/features/playground/lib/message-timing-utils.test.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/message-action-utils.test.ts src/features/playground/lib/storage.test.ts` 通过，共 29 个用例，覆盖 timing、消息内容状态、动作状态和 storage 兼容链路。
3. `cd web/default && ./node_modules/.bin/eslint src/components/ai-elements/reasoning.tsx src/features/playground/components/playground-message-content.tsx src/i18n/static-keys.ts` 通过。
4. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 `ReasoningTrigger` 的 i18n 类型、公共组件 props 和 Playground 传参类型正确。
5. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，Playground 相关 lazy chunk 为 `/static/js/async/92.a93384736f.js`。
6. `git diff --check` 通过。
7. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求与线上资源比对替代页面验证。
8. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP 200`；`/api/status` 返回 `success: true`。
9. 从 3003 拉取 `/static/js/index.61905bfd48.js` 与 `/static/js/async/92.a93384736f.js` 后，与本地 `web/default/dist` 完全一致：`index.61905bfd48.js` SHA256 均为 `5200dc91f6adf5c23d1b14235f588001ef46184103fe13be3fc2f802a8ef1140`，`92.a93384736f.js` SHA256 均为 `3a3679d17448a6eee3ef542b3a1bcd37ab345d19409c1a89e440f9c1f991b61f`。
10. 线上 `/static/js/async/92.a93384736f.js` 已包含 `Thinking...`、`Thought for {{duration}} seconds`、`Raw response`、`Show source` 特征；上一轮旧 chunk `/static/js/async/92.4ded5331fd.js` 返回 `HTTP 404`，确认 3003 页面加载的是本轮构建产物。

## 本轮实施评审：Playground 移动端消息动作菜单原生化

### 需求分析

new-api-main 的 Playground 已将单条消息动作先收敛为动作数组：桌面端继续展示 tooltip 图标按钮，移动端则用 `DropdownMenu` 折叠到一个 `MoreHorizontal` 菜单中。NexusTok 当前仍在 `MessageActions` 内直接横向渲染 Copy、Source/Preview、Regenerate、Edit、Delete 图标组，且通过 `max-md:opacity-100` 在移动端常显；在窄屏下，多版本分支、元信息、原始响应切换和错误恢复动作叠加后，图标组容易占用消息正文宽度，也不利于触控目标保持稳定。

本轮目标是吸收 new-api-main 的移动端交互优势，同时保留 NexusTok 已有 source toggle、动作守卫和会话 helper：

1. `MessageActions` 先根据消息状态生成统一动作数组，桌面端复用现有 `MessageActionButton` + `TooltipProvider`，移动端使用现有 Base UI `DropdownMenu` 渲染同一组动作。
2. 移动端只显示一个 `MoreHorizontal` 触发按钮，菜单项展示动作文案和右侧图标，避免多枚 28px 图标在小屏横向挤压。
3. Regenerate 可见条件与 new-api-main 对齐：user/assistant 消息只要有正文、非 loading/streaming 且存在 handler，都可以展示重试入口；底层 `createRegeneratedMessages` 已有 user 消息重提交流程和测试覆盖。
4. 现有 Copy、Copied、Regenerate、Edit、Delete、Show source、Show preview、Open menu 等文案全部走 i18n；补齐当前尚未进入 locale 的 `No content to copy` 和 `Please wait for the current generation to complete` toast 文案。
5. 本轮不改变错误消息专用动作栏、不改变请求 payload、不改变 localStorage schema，也不迁移 message alignment/layout mode。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 消息动作栏 | `web/default/src/features/playground/components/message-actions.tsx` | 动作收敛为数组；桌面图标组保持原样，移动端新增 `DropdownMenu` 菜单；动作 label 统一翻译。 |
| 动作守卫 | `web/default/src/features/playground/hooks/use-message-action-guard.ts` | 生成中禁止操作的 toast 改为 i18n，并把触碰到的英文注释改为中文。 |
| 动作状态 helper | `web/default/src/features/playground/lib/message-action-utils.ts`、`message-action-utils.test.ts` | 新增 regenerate 可见性判断 helper，覆盖 user/assistant、loading、无正文、无 handler 等边界。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`、`web/default/src/i18n/static-keys.ts` | 新增两个 toast 文案 key；其他动作文案复用已有 key。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验收方式和最终验证记录。 |

### 风险评估

1. 移动端菜单若与桌面按钮使用两套条件，容易出现同一条消息桌面可操作、移动端不可操作的割裂；本轮用同一动作数组驱动两种展示。
2. `DropdownMenuItem` 点击后会执行原有 handler；动作守卫仍包裹 regenerate/edit/delete，生成中不会发出新的重试、编辑或删除请求。
3. user 消息 regenerate 虽然此前 UI 未展示，但 `createRegeneratedMessages` 已支持并有测试覆盖；本轮只是暴露已有能力，不新增请求协议。
4. 移动端触发按钮使用 `Open menu` 作为无障碍名称，避免只靠图标表达；菜单项继续使用已有动作文案，不新增解释性 in-app 文案。
5. `No content to copy` 和等待生成 toast 使用动态常量传给 `t()`，必须写入 `static-keys.ts`，否则静态同步工具无法发现。
6. 本轮仅触碰 Playground 前端局部组件，不影响后端 relay、计费、权限、数据库和用户会话核心链路。

### 方案评审

采用“统一动作模型 + 响应式展示”的方案：`MessageActions` 内部构造 `MessageActionItem[]`，每项包含 `icon`、`label`、`onClick`、`disabled`、`variant` 和可选 class；桌面端 `hidden md:flex` 渲染现有 `MessageActionButton`，移动端 `md:hidden` 渲染 `DropdownMenu`。source/preview 可见性继续使用 `canToggleMessageSource` 和 `getMessageSourceToggleLabel`；新增 `canRegenerateMessage` 只负责判断重试入口是否可见。设计方向保持当前工具台密度，不新增卡片、不改变消息布局、不引入新颜色；`DropdownMenu`、`Button`、`Tooltip` 均复用已安装 Base UI 组件。

验收方式：

1. `cd web/default && bun run i18n:sync`，确认 en、zh、fr、ja、ru、vi 无 missing/extras。
2. `cd web/default && bun test src/features/playground/lib/message-action-utils.test.ts src/features/playground/lib/conversation-message-utils.test.ts src/features/playground/lib/message-source-utils.test.ts src/features/playground/lib/storage.test.ts`，覆盖动作可见性、user regenerate 会话链路、source key 和 storage 兼容。
3. 针对 `message-actions.tsx`、`use-message-action-guard.ts`、`message-action-utils.ts`、`message-action-utils.test.ts` 和 i18n static keys 运行定向 ESLint。
4. `cd web/default && ./node_modules/.bin/tsc -b` 和 `./node_modules/.bin/rsbuild build`，覆盖 Base UI DropdownMenu 类型和生产构建。
5. `git diff --check` 检查补丁空白。
6. 优先使用 MCP 打开 `http://192.168.0.202:3003/playground` 验证移动端菜单和桌面按钮；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 与本地 `dist` 比对，确认 3003 页面加载的是本轮构建产物。

验证记录：

1. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en、zh、fr、ja、ru、vi 的 `missingCount` 和 `extrasCount` 均为 0。本轮新增 `No content to copy` 和 `Please wait for the current generation to complete` 已补齐六语；报告中的 untranslated 计数为仓库既有存量。
2. `cd web/default && bun test src/features/playground/lib/message-action-utils.test.ts src/features/playground/lib/conversation-message-utils.test.ts src/features/playground/lib/message-source-utils.test.ts src/features/playground/lib/storage.test.ts` 通过，共 27 个用例，覆盖 source toggle、user/assistant regenerate 可见性、user regenerate 会话重提交流程、source key 和 storage 兼容链路。
3. `cd web/default && ./node_modules/.bin/eslint src/features/playground/components/message-actions.tsx src/features/playground/hooks/use-message-action-guard.ts src/features/playground/lib/message-action-utils.ts src/features/playground/lib/message-action-utils.test.ts src/i18n/static-keys.ts` 通过。
4. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 Base UI `DropdownMenu` 组合、动作数组类型和 i18n hook 类型正确。
5. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，Playground 相关 lazy chunk 为 `/static/js/async/92.js`。
6. `git diff --check` 通过。
7. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求与线上资源比对替代页面验证。
8. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP 200`；`/api/status` 返回 `success: true`。
9. 从 3003 拉取 `/static/js/index.js` 与 `/static/js/async/92.js` 后，与本地 `web/default/dist` 完全一致：`index.js` SHA256 均为 `5b8c02af7d9fb2746f34f6ff26b4ce4dd35febc715797f26099b18fcf5abb7cb`，`92.js` SHA256 均为 `03ca49a192056920c67ce6e073c0d836fc10bcd8db57242074d1de811e812e2b`。
10. 线上 `/static/js/async/92.js` 已包含 `Open menu`、`No content to copy`、`Please wait for the current generation to complete`、`Show source`、`Regenerate` 特征；上一轮旧 chunk `/static/js/async/92.a93384736f.js` 返回 `HTTP 404`，确认 3003 页面加载的是本轮构建产物。

## 本轮实施评审：Playground 消息布局对齐底座原生化

### 需求分析

new-api-main 的 Playground 已把消息布局抽象为 `PlaygroundMessageLayoutMode = 'alternating' | 'left'`，并通过 `getMessageAlignment()` 和 `getMessageAlignmentClass()` 统一决定 user/assistant/system 消息的左右对齐。NexusTok 当前仍在 `PlaygroundChat` 里固定给每条 `Message` 套 `group flex-row-reverse` 和一层全宽内容容器，`PlaygroundMessageContent` 与 `MessageMetadata` 不知道消息对齐语义；后续如果要继续吸收 new-api-main 的“全部左对齐”模式、用户偏好或调试布局，就会反复触碰聊天列表、消息内容和元信息组件。

本轮目标是只补齐对齐底座，不增加用户设置入口、不改本地存储 schema、不改变请求 payload：

1. 在 Playground 类型层新增 `PlaygroundMessageLayoutMode`，让消息布局模式成为显式类型，而不是组件内部散落字符串。
2. 新增可测试的 message layout helper：`alternating` 下 user 右对齐、非 user 左对齐；`left` 下所有角色统一左对齐。
3. `PlaygroundChat` 新增可选 `messageLayoutMode` prop，默认保持 `alternating`，并在保留 NexusTok 多版本 `Branch` 能力的前提下把 alignment 传给消息内容。
4. `PlaygroundMessageContent` 用 alignment 包裹来源、推理、加载、错误、正文、元信息和动作区域；`MessageMetadata` 根据 alignment 决定左/右侧展示，避免时间和耗时元信息与正文方向割裂。
5. 本轮不迁移 new-api-main 的消息气泡样式、不调整 `Message` AI element 的基础布局，也不把 layout mode 持久化到 localStorage，降低视觉和兼容风险。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Playground 类型 | `web/default/src/features/playground/types.ts` | 新增 `PlaygroundMessageLayoutMode`，为后续设置入口和布局模式接入提供稳定类型。 |
| 布局 helper | `web/default/src/features/playground/lib/message-layout-utils.ts`、`message-layout-utils.test.ts`、`lib/index.ts` | 新增左右对齐纯函数和测试覆盖，并从统一 lib 入口导出。 |
| 聊天列表 | `web/default/src/features/playground/components/playground-chat.tsx` | 新增 `messageLayoutMode` prop，默认 `alternating`；每条消息计算 alignment 后传给内容组件，保留 Branch 多版本切换。 |
| 消息内容 | `web/default/src/features/playground/components/playground-message-content.tsx` | 外层按 alignment 设置 `items-*` 和 `text-*`，让来源、推理、raw/preview、错误、动作区域拥有一致对齐语义。 |
| 消息元信息 | `web/default/src/features/playground/components/message-metadata.tsx` | 接收 alignment 并在右对齐时 `justify-end`，历史缺 timing 字段时仍不渲染。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验收方式和最终验证记录。 |

### 风险评估

1. `Message` 公共 AI element 已有 role-based flex class；本轮不重写该组件，只让 Playground 内容列和元信息消费 alignment，避免把公共消息组件行为一起改动。
2. NexusTok 仍保留多版本 `Branch` 渲染，而 new-api-main 当前实现没有同等分支层；本轮只在每个 version 渲染时传递同一个 alignment，不改变分支选择、key 或版本内容。
3. 默认 `alternating` 与现有 user 右侧、assistant 左侧的视觉语义一致；新增 `left` 只是底座能力，未暴露为用户可见设置，因此不会突然改变当前用户界面。
4. 本轮不改 localStorage、storage schema、SSE 流式处理、消息生成、计费、权限、后端 relay 或数据库，核心业务链路风险低。
5. `text-right` 可能影响 Markdown 列表、代码块或 raw response 的内部排版；本轮仅把它作为 alignment class 作用在内容列，`CodeBlock` 仍保持自身块级滚动与等宽渲染，验证时需要关注 raw response 和普通 Markdown 的宽度是否稳定。
6. 新增 helper 要避免从 new-api-main 直接带入 `QuantumNous` 版权头；在 NexusTok 落地时使用本项目版权、中文注释和现有常量路径。

### 方案评审

采用“类型 + 纯函数 + 渲染透传”的最小底座方案：在 `types.ts` 定义 `PlaygroundMessageLayoutMode`；新增 `message-layout-utils.ts` 暴露 `MessageAlignment`、`getMessageAlignment()` 和 `getMessageAlignmentClass()`；`PlaygroundChat` 默认 `messageLayoutMode='alternating'`，对每条消息计算 alignment 后传入 `PlaygroundMessageContent`；内容组件增加一层 `flex w-full min-w-0 flex-col` 包裹现有来源、推理、加载、错误、正文、元信息和动作；`MessageMetadata` 用 `cn()` 根据 alignment 添加 `justify-end`。设计方向保持当前 Playground 工具台密度和既有色彩，不新增 in-app 文案、不新增依赖、不改变移动端动作菜单。

验收方式：

1. `cd web/default && bun test src/features/playground/lib/message-layout-utils.test.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/conversation-message-utils.test.ts src/features/playground/lib/message-action-utils.test.ts`，覆盖新 helper 和近期已触碰的消息内容/会话/动作链路。
2. 针对 `playground-chat.tsx`、`playground-message-content.tsx`、`message-metadata.tsx`、`message-layout-utils.ts`、`message-layout-utils.test.ts`、`types.ts` 和 `lib/index.ts` 运行定向 ESLint。
3. `cd web/default && ./node_modules/.bin/tsc -b` 和 `./node_modules/.bin/rsbuild build`，覆盖新增 prop、alignment 类型和生产构建。
4. `git diff --check` 检查补丁空白。
5. 优先使用 MCP 打开 `http://192.168.0.202:3003/playground` 验证页面加载、消息对齐和控制台状态；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 与本地 `dist` 比对，确认 3003 页面加载的是本轮构建产物。

验证记录：

1. `cd web/default && bun test src/features/playground/lib/message-layout-utils.test.ts src/features/playground/lib/message-content-utils.test.ts src/features/playground/lib/conversation-message-utils.test.ts src/features/playground/lib/message-action-utils.test.ts` 通过，共 25 个用例，覆盖 alternating/left 对齐、消息内容状态、会话编辑/重试和动作可见性链路。
2. `cd web/default && ./node_modules/.bin/eslint src/features/playground/components/playground-chat.tsx src/features/playground/components/playground-message-content.tsx src/features/playground/components/message-metadata.tsx src/features/playground/lib/message-layout-utils.ts src/features/playground/lib/message-layout-utils.test.ts src/features/playground/types.ts src/features/playground/lib/index.ts` 通过。
3. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 `PlaygroundMessageLayoutMode`、`MessageAlignment`、`messageLayoutMode` prop 和 `MessageMetadata` 传参类型正确。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，生产构建生成新的本地 `dist`；Playground 相关代码已进入 `/static/js/async/4604.js` 等 async chunk。
5. `git diff --check` 通过。
6. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求与线上资源比对替代页面验证。
7. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP 200`；`/api/status` 返回 `success: true`。
8. 从 3003 拉取 `/static/js/index.js`、`/static/js/6870.js`、`/static/js/async/4604.js`、`/static/js/async/2191.js`、`/static/js/async/762.js`、`/static/js/async/4474.js`、`/static/js/async/4050.js` 和 `/static/js/async/1325.js` 后，与本地 `web/default/dist` 完全一致；其中 `index.js` SHA256 均为 `841af9a5bfe3370052d02e504024b748bc678d63c7dc152cc91b7542cc4dfaeb`，`4604.js` SHA256 均为 `c539d6857bbdba73e616bf10b11064f56c6518a483f93033e5320e054c27473c`。
9. 线上 `/static/js/async/4604.js` 已包含 `items-end text-right`、`items-start text-left` 和 `messageLayoutMode` 特征，确认本轮消息对齐底座已进入 3003 页面资源。
10. 上一轮旧 Playground chunk `/static/js/async/92.js` 返回 `HTTP 404`，确认 3003 未继续加载旧缓存；页面资源已随本轮构建更新，无需重启容器。

## 本轮实施评审：Playground 消息流式状态工具原生化

### 需求分析

new-api-main 的 Playground 将 reasoning/content 流式增量合并、assistant 完成态判断、非流式 choice 应用和重复 chunk 防护收敛到 `message-streaming-utils.ts`，让 `use-chat-handler` 只负责请求生命周期编排。NexusTok 当前已经有 `processStreamingContent()`、`finalizeMessage()`、`updateLastAssistantMessage()` 和 stream/request 错误工具，但 `use-chat-handler.ts` 内仍直接拼接 reasoning delta、手写完成态判断、手写非流式 choice 写回；后续继续吸收消息重构能力时，这些热路径逻辑仍会让 hook 变厚。

本轮目标是把流式消息状态变更继续沉到可测试纯函数里，同时不改变 `/pg/chat/completions` payload、SSE 连接实现和消息存储格式：

1. 新增 `message-streaming-utils.ts`，提供 `applyStreamingChunk()`、`completeAssistantMessage()`、`isAssistantMessageFinal()`、`isAssistantMessagePending()`、`applyChatCompletionChoice()` 和 `applyChatCompletionResponse()`。
2. `applyStreamingChunk()` 复用 NexusTok 已有 `<think>` 解析和 timing helper，并加入 appendable chunk 保护：如果上游误返回累计内容而不是纯 delta，只追加新增部分，避免正文或 reasoning 重复。
3. `use-chat-handler.ts` 改为调用上述 helper，保留现有 toast、错误落盘、payload builder、`useStreamRequest()` 和停止生成语义。
4. 补充 `message-streaming-utils.test.ts`，覆盖 reasoning/content delta、累计 chunk 去重、错误消息保护、完成态清理和非流式响应应用。
5. 本轮不迁移 `sanitizeMessagesOnLoad()`，因为 NexusTok 已在 `message-utils.ts` 和 `storage.ts` 形成稳定兼容路径；后续若整理目录结构，再单独迁移，避免一次性触碰本地存储恢复链路。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 消息流式 helper | `web/default/src/features/playground/lib/message-streaming-utils.ts`、`message-streaming-utils.test.ts`、`lib/index.ts` | 新增流式增量、完成态、非流式响应应用的纯函数和测试导出。 |
| Chat hook | `web/default/src/features/playground/hooks/use-chat-handler.ts` | 用 helper 替代内联 reasoning/content 拼接、完成态判断和 choice 写回；请求生命周期和错误 toast 不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验收方式和最终验证记录。 |

### 风险评估

1. `use-chat-handler.ts` 是 Playground 发送热路径；本轮只移动消息状态变更到纯函数，不改请求创建、SSE 事件监听、错误 toast、停止生成和 payload 构造，降低行为回归风险。
2. appendable chunk 去重会改变极少数“上游返回累计内容”的表现：从重复拼接变为只追加新增部分；正常 OpenAI SSE delta 不受影响。
3. `applyStreamingChunk()` 在 `<think>` 标签闭合后可提前完成 reasoning timing，但最终 `completeAssistantMessage()` 仍会兜底，旧消息和无 reasoning 响应保持兼容。
4. 非流式响应无 choice 时继续保留当前 assistant 占位消息，不主动标错；这个语义与当前 hook 一致，避免把“空 choices”处理策略混入本轮。
5. 本轮不触碰后端 relay、计费、权限、数据库、i18n 文案、localStorage schema 和 UI 结构。

### 方案评审

采用“新增消息流式纯函数 + hook 薄化”的方案：`message-streaming-utils.ts` 复用现有 `processStreamingContent()`、`finalizeMessage()`、`updateCurrentVersionContent()`、`startReasoningTiming()` 和 timing helper；`use-chat-handler.ts` 在 `handleStreamUpdate`、`handleStreamComplete`、`sendNonStreamingChat` 和 `stopGeneration` 中只调用这些 helper。这样保留 NexusTok 现有中文注释、消息版本结构和错误恢复能力，同时吸收 new-api-main 的 streaming helper 分层与重复 chunk 防护。

验收方式：

1. `cd web/default && bun test src/features/playground/lib/message-streaming-utils.test.ts src/features/playground/lib/message-update-utils.test.ts src/features/playground/lib/message-reasoning-utils.test.ts src/features/playground/lib/message-timing-utils.test.ts`，覆盖新增 helper 和既有 timing/update/reasoning 边界。
2. 针对 `use-chat-handler.ts`、`message-streaming-utils.ts`、`message-streaming-utils.test.ts` 和 `lib/index.ts` 运行定向 ESLint。
3. `cd web/default && ./node_modules/.bin/tsc -b` 和 `./node_modules/.bin/rsbuild build`，覆盖 hook 类型、响应类型和生产构建。
4. `git diff --check` 检查补丁空白。
5. 优先使用 MCP 打开 `http://192.168.0.202:3003/playground` 验证页面加载和控制台状态；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 与本地 `dist` 比对，确认 3003 页面加载的是本轮构建产物。

验证记录：

1. `cd web/default && bun test src/features/playground/lib/message-streaming-utils.test.ts src/features/playground/lib/message-update-utils.test.ts src/features/playground/lib/message-reasoning-utils.test.ts src/features/playground/lib/message-timing-utils.test.ts` 通过，共 24 个用例，覆盖流式增量、累计 chunk 去重、错误态保护、非流式响应写回、reasoning 解析和 timing 边界。
2. `cd web/default && ./node_modules/.bin/eslint src/features/playground/hooks/use-chat-handler.ts src/features/playground/lib/message-streaming-utils.ts src/features/playground/lib/message-streaming-utils.test.ts src/features/playground/lib/index.ts` 通过。
3. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 hook 回调类型、`MessageStreamChunkType` 和 `ChatCompletionResponse` choice 应用类型正确。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，生产构建生成新的本地 `dist`；Playground 相关 async chunk 切换为 `/static/js/async/5192.js`。
5. `git diff --check` 通过。
6. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求与线上资源比对替代页面验证。
7. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP 200`；`/api/status` 返回 `success: true`，`system_name` 为 `NexusTok`。
8. 3003 首页已引用 `/static/js/index.7787b8d1af.js`；线上 `/static/js/index.js` 与本地 `web/default/dist/static/js/index.js` SHA256 均为 `6aa08836fd8204c839c09bf3099410c2b687561995e233c5c9da6d443696b7bf`，线上 `/static/js/index.7787b8d1af.js` 与本地 hash 入口 SHA256 均为 `380189ab25297387f3eeef401716da5f8ae3a7d5948a98f4161fcc92621d48ae`。
9. 线上 `/static/js/async/5192.js` 与本地 `web/default/dist/static/js/async/5192.js`、`5192.bf9c0f05f6.js` SHA256 均为 `13a56115eae01b75af8d9154181a946c9c6250ee680b1375beced774295a5b50`，确认 3003 页面加载的是本轮构建产物。
10. 上一轮旧 Playground chunk `/static/js/async/4604.js` 返回 `HTTP 404`，确认 3003 未继续加载旧缓存；页面资源已随本轮构建更新，无需重启容器。

## 本轮实施评审：Playground 错误状态 helper 原生化

### 需求分析

new-api-main 的 Playground 已将消息错误分类、模型价格错误判断、管理员设置入口可见性和 fallback 错误文案收敛到 `message-error-utils.ts`，`MessageError` 组件只负责渲染 `Alert`。NexusTok 当前 `message-error.tsx` 仍内联 `MODEL_PRICE_ERROR_CODE`、`MODEL_PRICING_SETTINGS_PATH`、`FALLBACK_ERROR_CONTENT`、`message.status` 判断和 `role >= 10` 管理员判断；后续继续吸收错误动作、错误详情、模型价格入口或权限矩阵时，会让展示组件继续承担业务状态判断。

本轮目标是把错误状态判断抽成可测试纯函数，同时保留 NexusTok 当前错误卡片视觉和动作布局：

1. 新增 `message-error-utils.ts`，提供 `FALLBACK_ERROR_CONTENT`、`MODEL_PRICING_SETTINGS_PATH`、`isAdminRole()`、`isErrorMessage()` 和 `getMessageErrorState()`。
2. `getMessageErrorState()` 返回 `content`、`kind` 和 `showSettingsLink`，让组件不再直接判断 `message.status`、`errorCode` 和用户角色。
3. `MessageError` 继续使用当前 `Alert`/`Button` 组合、`text-warning` 语义色和 `data-icon='inline-start'` 图标写法；本轮不移动错误恢复动作进 Alert 内部，避免和当前 metadata/errorActions 顺序一起改变。
4. 为 helper 补测试，覆盖非错误消息、普通错误、模型价格错误、管理员和非管理员设置入口可见性、空内容 fallback。
5. 本轮不新增 i18n 文案；`An unknown error occurred`、`Model Price Not Configured`、`Go to Settings`、`Error` 继续复用现有翻译 key。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 错误状态 helper | `web/default/src/features/playground/lib/message-error-utils.ts`、`message-error-utils.test.ts`、`lib/index.ts` | 新增错误分类和管理员可见性纯函数，并从统一 lib 入口导出。 |
| 错误组件 | `web/default/src/features/playground/components/message-error.tsx` | 组件改为消费 helper 的 `MessageErrorState`，保留现有 Alert/Button 结构和视觉语义。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验收方式和最终验证记录。 |

### 风险评估

1. `MessageError` 是 Playground 错误展示入口；本轮只移动状态判断到 helper，不改变错误消息写入、errorCode 来源、恢复动作、请求链路、SSE 解析或 toast 行为。
2. 管理员判断仍保持当前 `role >= 10` 语义，不接入权限矩阵，避免把前端展示 helper 与后续 Authz 改造绑定在一起。
3. fallback 文案从组件内常量迁入 helper；组件仍只在 fallback 命中时调用 `t(FALLBACK_ERROR_CONTENT)`，真实上游错误内容保持原样展示。
4. 模型价格设置入口路径继续使用 `/system-settings/billing/model-pricing`，不改当前默认前端路由，也不影响渠道测试弹窗里的同类错误处理。
5. 本轮不改 UI 文案、不新增 locale key、不触碰后端、数据库、计费、权限和 localStorage schema，核心业务风险低。

### 方案评审

采用“纯函数抽离 + 组件瘦身”的方案：按 NexusTok 版权头和中文注释新增 `message-error-utils.ts`；`MessageError` 读取 `useAuthStore` 后只调用 `getMessageErrorState(message, isAdminRole(user?.role))`，根据 `kind` 渲染普通 destructive Alert 或模型价格 warning Alert。设计方向保持当前默认前端工具台风格，继续使用已安装的 `Alert`、`Button` 和 lucide 图标，不新增卡片、不调整错误动作位置、不引入新依赖。

验收方式：

1. `cd web/default && bun test src/features/playground/lib/message-error-utils.test.ts src/features/playground/lib/message-update-utils.test.ts src/features/playground/lib/message-action-utils.test.ts`，覆盖错误状态 helper、错误写回和消息动作边界。
2. 针对 `message-error.tsx`、`message-error-utils.ts`、`message-error-utils.test.ts` 和 `lib/index.ts` 运行定向 ESLint。
3. `cd web/default && ./node_modules/.bin/tsc -b` 和 `./node_modules/.bin/rsbuild build`，覆盖 helper 导出、组件类型和生产构建。
4. `git diff --check` 检查补丁空白。
5. 优先使用 MCP 打开 `http://192.168.0.202:3003/playground` 验证页面加载和控制台状态；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 与本地 `dist` 比对，确认 3003 页面加载的是本轮构建产物。

验证记录：

1. `cd web/default && bun test src/features/playground/lib/message-error-utils.test.ts src/features/playground/lib/message-update-utils.test.ts src/features/playground/lib/message-action-utils.test.ts` 通过，共 17 个用例，覆盖错误状态 helper、错误写回和消息动作边界。
2. `cd web/default && ./node_modules/.bin/eslint src/features/playground/components/message-error.tsx src/features/playground/lib/message-error-utils.ts src/features/playground/lib/message-error-utils.test.ts src/features/playground/lib/index.ts` 通过。
3. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 `MessageErrorState`、helper 导出和 `MessageError` 组件类型正确。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，生产构建生成新的本地 `dist`；Playground 相关 async chunk 切换为 `/static/js/async/6675.js`。
5. `git diff --check` 通过。
6. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求与线上资源比对替代页面验证。
7. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP 200`；`/api/status` 返回 `success: true`，`system_name` 为 `NexusTok`。
8. 从 3003 拉取 `/static/js/index.js` 与 `/static/js/async/6675.js` 后，与本地 `web/default/dist` 完全一致：`index.js` SHA256 均为 `c452d3907d348546052e086ff18dc22a5516aa085a76975a78d12a8610337394`，`6675.js` SHA256 均为 `c77cd7a9f0966488e11a0b19f2ca791616e66a5152cb8d59cd451265124fca0e`。
9. 线上 `/static/js/async/6675.js` 已包含 `/system-settings/billing/model-pricing`、`An unknown error occurred`、`model_price_error` 和 `messageLayoutMode` 特征，确认本轮错误状态 helper 和前序 Playground 能力已进入 3003 页面资源。
10. 上一轮旧 Playground chunk `/static/js/async/5192.js` 返回 `HTTP 404`，确认 3003 未继续加载旧缓存；页面资源已随本轮构建更新，无需重启容器。

## 本轮实施评审：Playground 消息阅读样式原生化

### 需求分析

new-api-main 的 Playground 已将用户气泡和 assistant 回复的阅读样式收敛到 `message-styles.ts`：assistant 内容按文档列宽限制在 `78ch` 左右，用户消息保持紧凑气泡，回复字体回到当前 UI 字体轴，避免长回答在宽屏上横向铺满。NexusTok 当前同名 helper 仍保留 `max-w-none`、`font-serif`、`rounded-3xl`、`bg-secondary` 和 `dark:` 专用覆盖；在已经完成消息布局对齐、原始响应、错误状态和流式状态工具化后，这块样式会继续影响 Playground 的阅读密度和默认前端视觉一致性。

本轮目标是只原生化消息内容阅读样式，不扩大到编辑器、输入区、布局偏好或主题系统：

1. 将 assistant 正文从无限宽改为 `max-w-[78ch]`，保留 `w-full` 参与当前消息行布局，降低超宽屏阅读负担。
2. 移除 `font-serif`，改为使用 NexusTok 当前主题中的 `font-sans` 字体轴，保持 Base Nova/默认前端字体策略一致。
3. 将用户消息气泡调整为轻量边框和 `muted` surface，使用语义 token，移除手写 `dark:` 背景覆盖。
4. 补充 `message-styles.test.ts`，用纯字符串断言锁住关键阅读样式，防止后续回退到无限宽或 serif。
5. 本轮不新增任何 UI 文案，因此不涉及 i18n；不改变消息协议、localStorage schema、请求链路和 Markdown 渲染组件。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 消息样式 helper | `web/default/src/features/playground/lib/message-styles.ts` | 调整 `getMessageContentStyles()` 返回的 Tailwind class 列表，并把触碰到的主体注释改为中文。 |
| 样式回归测试 | `web/default/src/features/playground/lib/message-styles.test.ts` | 新增关键 class 断言，覆盖 assistant 列宽、字体、用户气泡 surface、字号和深色模式覆盖移除。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验收方式和最终验证记录。 |

### 风险评估

1. 这是纯前端 class 调整，不改变消息数据、发送逻辑、错误处理、计费、权限、数据库和后端接口，核心业务风险低。
2. assistant 最大宽度从无限制变为 `78ch` 后，超宽屏长回答会更像文档列；但代码块、原始响应和多版本内容可能需要继续依赖内部 overflow 处理，本轮会通过生产构建和 3003 静态资源验证确认样式进入页面。
3. 用户气泡从 `bg-secondary/dark:bg-muted` 改为 `bg-muted/70 + border`，视觉上会比旧版更轻；这属于目标内的阅读体验提升，但需要避免引入 raw color 和额外主题覆盖。
4. `font-sans` 依赖当前默认前端的 `--app-font-sans` 主题定义；若未来主题变量重构，测试会捕获关键 class，实际渲染仍由全局 CSS token 决定。
5. 本轮不动编辑器样式。new-api-main 的编辑器也使用了 `max-w-[78ch]`，但 NexusTok 编辑器已在前几轮形成独立组件，后续可单独评审，避免一次视觉变更影响编辑输入体验。

### 方案评审

采用“直接收敛现有 helper class + 单元测试锁定关键 class”的方案：保留 `getMessageContentStyles()` 的调用方式和 `PlaygroundMessageContent` 结构，仅调整 class 列表。设计方向以当前默认前端 Base Nova 工具台为边界，吸收 new-api-main 的文档列宽、紧凑用户气泡和主题字体优点，不引入新组件、不新增文案、不修改全局 CSS。

验收方式：

1. `cd web/default && bun test src/features/playground/lib/message-styles.test.ts src/features/playground/lib/message-layout-utils.test.ts src/features/playground/lib/message-content-utils.test.ts`，覆盖新增样式 helper 和相邻布局/内容 helper。
2. 针对 `message-styles.ts` 和 `message-styles.test.ts` 运行定向 ESLint。
3. `cd web/default && ./node_modules/.bin/tsc -b` 和 `./node_modules/.bin/rsbuild build`，确认类型和生产构建通过。
4. `git diff --check` 检查补丁空白。
5. 优先使用 MCP 打开 `http://192.168.0.202:3003/playground` 验证页面加载和控制台状态；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 与本地 `dist` 比对，确认 3003 页面加载的是本轮构建产物。

验证记录：

1. `cd web/default && bun test src/features/playground/lib/message-styles.test.ts src/features/playground/lib/message-layout-utils.test.ts src/features/playground/lib/message-content-utils.test.ts` 通过，共 14 个用例，覆盖新增消息样式 helper、布局 alignment helper 和消息内容状态 helper。
2. `cd web/default && ./node_modules/.bin/eslint src/features/playground/lib/message-styles.ts src/features/playground/lib/message-styles.test.ts` 通过。
3. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认新增测试和样式 helper 类型正确。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，生产构建生成新的本地 `dist`；Playground 相关 async chunk 仍为 `/static/js/async/6675.js`，内容 hash 已随本轮样式变更更新。
5. `git diff --check` 通过。
6. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求与线上资源比对替代页面验证。
7. `curl --noproxy '*' -H 'Cache-Control: no-cache' http://192.168.0.202:3003/`、`/api/status` 和 `/playground` 均返回 `HTTP 200`；`/api/status` 返回 `success: true`，`system_name` 为 `NexusTok`。
8. 从 3003 拉取 `/static/js/index.js`、`/static/js/async/6675.js` 和 `/static/css/index.css` 后，与本地 `web/default/dist` 完全一致：`index.js` SHA256 均为 `c452d3907d348546052e086ff18dc22a5516aa085a76975a78d12a8610337394`，`6675.js` SHA256 均为 `29523081f9b6a7421ad2862eb1c82acfbb1feb18615d42fc857714923bc7de77`，`index.css` SHA256 均为 `e641984672d30452ed7b325c5f3e350de58062ec0c6d4916b6c0a20ec659a6fa`。
9. 线上 `/static/js/async/6675.js` 已包含 `max-w-[78ch]`、`font-sans`、`rounded-br-md` 和 `bg-muted/70`，且未检出本轮移除的 `font-serif`、`max-w-none`、`dark:group-[.is-user]:bg-muted` 特征，确认 3003 页面资源已加载本轮消息阅读样式。
10. 本轮 chunk 路径未变化，线上入口、CSS 和 Playground chunk 均已与本地构建一致，无需重启容器。

## 本轮实施评审：Playground 消息编辑器状态安全原生化

### 需求分析

new-api-main 的 Playground 消息编辑器相比 NexusTok 当前实现有四类优势：编辑区域宽度与消息阅读列宽一致，按钮收敛为图标动作，未保存变更在取消或按 Escape 时会二次确认，且提供一键重置到原文。NexusTok 已有 `PlaygroundMessageEditor` 组件和 `getMessageEditorState()` 纯函数，具备 Save、Save & Submit、Cancel 与 Ctrl/⌘+Enter 提交，但当前编辑器仍使用整行 `Textarea` 和文字按钮；如果用户误按 Escape 或 Cancel，会直接丢弃本地编辑内容。

本轮目标是吸收 new-api-main 的编辑器安全和布局优势，同时保持 NexusTok 当前依赖与组件体系：

1. 不引入 CodeMirror、不新增 `@codemirror/*` 依赖，继续使用现有 `Textarea`，避免把公共 `CodeBlock` 组件和构建体积一起纳入本轮。
2. 为 `message-styles.ts` 增加 `getMessageEditorStyles()`，让编辑器和消息正文共享 `78ch` assistant 列宽、user 最大宽度规则，避免编辑态横向铺满。
3. `PlaygroundMessageEditor` 使用 `hasChanged` 派生状态，取消和 Escape 在存在未保存变更时调用 `window.confirm(t('You have unsaved changes. Are you sure you want to leave?'))`。
4. 编辑器动作改为图标按钮：Save & Submit、Save、Reset、Cancel，均保留 `aria-label` 和现有 i18n key；Reset 仅在内容变更后展示，并把编辑文本恢复到 `originalText`。
5. 补充 `message-editor-utils.test.ts` 和扩展 `message-styles.test.ts`，锁定编辑器状态与样式 helper，避免后续回退。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 编辑器组件 | `web/default/src/features/playground/components/playground-message-editor.tsx` | 增加取消确认、重置动作、图标按钮和编辑区域宽度 class；请求、保存回调和键盘提交语义保持不变。 |
| 样式 helper | `web/default/src/features/playground/lib/message-styles.ts`、`message-styles.test.ts` | 新增编辑器宽度 helper，并补测试覆盖 assistant/user 编辑态宽度。 |
| 编辑状态测试 | `web/default/src/features/playground/lib/message-editor-utils.test.ts` | 新增纯函数测试，覆盖未变更、空内容、user/assistant Save & Submit 可见性和 `hasChanged`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验收方式和最终验证记录。 |

### 风险评估

1. 本轮只改 Playground 前端编辑态，不改变消息结构、localStorage schema、发送 payload、SSE、错误恢复、计费、权限和后端接口，核心业务风险低。
2. `window.confirm` 会新增一次浏览器原生确认交互，仅在内容有变更且用户主动 Cancel/Escape 时触发；无变更取消、保存、Save & Submit 和重置动作不受影响。
3. 图标按钮会减少编辑器底部文字宽度压力，但依赖 `aria-label` 与 tooltip 外的浏览器辅助能力；本轮保留现有 i18n key，不新增文案。
4. 不引入 CodeMirror 意味着本轮没有 new-api-main 的 Markdown 高亮编辑体验；这是有意拆分，避免依赖、公共组件和构建体积风险混入编辑器安全能力。
5. 编辑器宽度 class 进入 helper 后会和上一轮消息正文列宽保持一致；若后续暴露“全部左对齐”布局偏好，再单独评审编辑态的 alignment 传入方式。

### 方案评审

采用“现有 Textarea 增强 + 可测试样式 helper”的方案：`PlaygroundMessageEditor` 继续消费 `getMessageEditorState()`，新增 `handleCancel()` 统一处理 Cancel 与 Escape；按钮改用现有 `Button` 的 `icon-sm` 尺寸和 lucide 图标，遵守当前 Playground 已有图标体系；`getMessageEditorStyles()` 只负责 role-based 宽度约束，不改组件结构和保存回调。这样能把 new-api-main 的误取消保护、重置入口和阅读列宽优势转为 NexusTok 原生能力，同时把 CodeMirror 编辑器留给后续更大范围的 rich editor 切片。

验收方式：

1. `cd web/default && bun test src/features/playground/lib/message-editor-utils.test.ts src/features/playground/lib/message-styles.test.ts src/features/playground/lib/conversation-message-utils.test.ts`，覆盖编辑状态、编辑器样式和编辑保存数组行为。
2. 针对 `playground-message-editor.tsx`、`message-editor-utils.test.ts`、`message-styles.ts` 和 `message-styles.test.ts` 运行定向 ESLint。
3. `cd web/default && ./node_modules/.bin/tsc -b` 和 `./node_modules/.bin/rsbuild build`，确认组件类型、图标导入和生产构建通过。
4. `git diff --check` 检查补丁空白。
5. 优先使用 MCP 打开 `http://192.168.0.202:3003/playground` 验证页面加载、编辑器动作和取消确认；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 与本地 `dist` 比对，确认 3003 页面加载的是本轮构建产物。

验证记录：

1. `cd web/default && bun test src/features/playground/lib/message-editor-utils.test.ts src/features/playground/lib/message-styles.test.ts src/features/playground/lib/conversation-message-utils.test.ts` 通过，共 17 个用例，覆盖编辑器状态、编辑器宽度样式和保存/提交消息数组行为。
2. `cd web/default && ./node_modules/.bin/eslint src/features/playground/components/playground-message-editor.tsx src/features/playground/lib/message-editor-utils.test.ts src/features/playground/lib/message-styles.ts src/features/playground/lib/message-styles.test.ts` 通过。
3. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 `PlaygroundMessageEditor`、Tooltip、图标按钮和新增测试类型正确。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 通过，生产构建生成新的本地 `dist`；Playground 相关 async chunk 仍为 `/static/js/async/6675.js`，内容 hash 已随本轮编辑器改造更新。
5. `git diff --check` 通过。
6. MCP Chrome DevTools 仍无法连接，错误为无法从 `http://127.0.0.1:9222/json/version` 获取 browser WebSocket URL；本轮按约定使用真实 3003 HTTP 请求与线上资源比对替代页面验证。
7. 第一次 `curl --noproxy '*' http://192.168.0.202:3003/` 出现一次瞬时连接失败；随后重试 `/` 成功返回 `HTTP 200`，`/playground` 和 `/api/status` 也均返回 `HTTP 200`；`/api/status` 返回 `success: true`，`system_name` 为 `NexusTok`。
8. 从 3003 拉取 `/static/js/index.js`、`/static/js/async/6675.js` 和 `/static/css/index.css` 后，与本地 `web/default/dist` 完全一致：`index.js` SHA256 均为 `c452d3907d348546052e086ff18dc22a5516aa085a76975a78d12a8610337394`，`6675.js` SHA256 均为 `f28c912face1c18d45642a9400ab7a7bb386a4f7d088845e70302fb7bc7bbaad`，`index.css` SHA256 均为 `e641984672d30452ed7b325c5f3e350de58062ec0c6d4916b6c0a20ec659a6fa`。
9. 线上 `/static/js/async/6675.js` 已包含 `You have unsaved changes. Are you sure you want to leave?`、`Unsaved changes`、`No changes`、`Save & Submit`、`Reset`、`group-[.is-assistant]:max-w-[78ch]` 和 `group-[.is-user]:max-w-[85%]` 特征，确认 3003 页面资源已加载本轮编辑器安全状态和宽度样式。
10. 本轮复用已有六语 i18n key，没有新增 locale 文案；线上入口、CSS 和 Playground chunk 均已与本地构建一致，无需重启容器。

## 本轮实施评审：Playground CodeMirror 编辑器底座原生化

### 需求分析

new-api-main 的 Playground 消息编辑器不只是普通多行输入框，而是复用公共 `CodeBlockEditor`，基于 CodeMirror 提供 Markdown 编辑、行号、键盘事件接入、固定行高和代码块式 frame。这对于长 prompt、Markdown、工具调用 JSON 和 assistant 原始响应修订都更接近“代码/结构化文本编辑器”，也是 new-api-main 在 Rich Content / Playground 编辑体验上的明确优势。NexusTok 当前已经完成编辑器误取消保护、重置动作和宽度对齐，但仍使用 `Textarea`，缺少 CodeMirror 的行级编辑体验。

本轮目标是把 CodeMirror 编辑器作为 NexusTok 原生公共能力引入，同时控制公共代码块展示链路的风险：

1. 在 `web/default/src/components/ai-elements/code-block.tsx` 中新增 `CodeBlockEditor`、`CodeMirrorCodeView` 和必要的 CodeMirror helper，供编辑场景使用。
2. 保留现有只读 `CodeBlock` 的 Shiki 高亮渲染、copy 入口和调用 API，不把 pricing、AI response、tool output 等所有代码块展示一次性迁移到 CodeMirror。
3. 为 `web/default` 新增 CodeMirror 依赖：`@codemirror/lang-markdown`、`@codemirror/language`、`@codemirror/state`、`@codemirror/view` 和 `@lezer/highlight`。
4. `PlaygroundMessageEditor` 改为使用 `CodeBlockEditor`，保留上一轮已落地的未保存确认、Reset、图标动作、Save & Submit 和宽度 helper。
5. 补充轻量纯函数/样式测试继续锁住编辑状态和宽度；CodeMirror DOM 行为主要通过 TypeScript、生产构建和 3003 资源验证确认。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 公共编辑器底座 | `web/default/src/components/ai-elements/code-block.tsx` | 新增 CodeMirror 编辑器能力；只读 `CodeBlock` 保持 Shiki 实现和现有 props，不迁移公共展示链路。 |
| Playground 编辑器 | `web/default/src/features/playground/components/playground-message-editor.tsx` | 从 `Textarea` 切换为 `CodeBlockEditor`，复用现有动作、确认和宽度状态。 |
| 前端依赖 | `web/default/package.json`、`web/default/bun.lock` | 新增 CodeMirror 相关依赖，用 Bun 更新 lockfile。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验收方式和最终验证记录。 |

### 风险评估

1. `code-block.tsx` 是 AI response、pricing API 示例和 tool output 的公共组件；本轮只新增 `CodeBlockEditor`，不改变现有 `CodeBlock` 渲染路径，降低跨页面回归风险。
2. 新增 CodeMirror 依赖会增加前端包体和 Playground 相关 chunk 体积；本轮通过生产构建观察 chunk 变化，并只在编辑器路径引用，避免影响不进入 Playground 编辑态的核心 API/计费/管理流程。
3. CodeMirror 通过 `EditorView` 管理 DOM，必须在组件卸载时销毁；本轮实现需要明确 `destroy()`，并在外部 value 变化时做受控同步，避免内存泄漏和重置动作失效。
4. CodeMirror 的 `keydown` 事件类型为 `globalThis.KeyboardEvent`，与 React `KeyboardEvent` 不同；`PlaygroundMessageEditor` 的快捷键处理需要改用浏览器事件类型，保持 Escape 与 Ctrl/⌘+Enter 语义不变。
5. 本轮不新增 UI 文案，复用 `Edit`、`Unsaved changes`、`No changes`、`Save`、`Save & Submit`、`Reset`、`Cancel` 等已有六语 key；不触碰后端、数据库、权限、计费和 localStorage schema。

### 方案评审

采用“公共编辑器新增、只读展示不迁移”的方案：从 new-api-main 吸收 CodeMirror 编辑器的核心结构，但保持 NexusTok 当前 Shiki `CodeBlock` 作为只读代码展示实现。`CodeMirrorCodeView` 负责创建、销毁和同步 `EditorView`；`CodeBlockEditor` 负责 frame、toolbar actions 与受控 value。`PlaygroundMessageEditor` 只替换输入控件，上一轮的误取消保护、Reset、本地状态提示和保存回调全部保留。这样能把 new-api-main 的结构化编辑优势真正转为 NexusTok 原生能力，同时把跨页面代码块展示升级留给后续单独评审。

验收方式：

1. `cd web/default && bun test src/features/playground/lib/message-editor-utils.test.ts src/features/playground/lib/message-styles.test.ts src/features/playground/lib/conversation-message-utils.test.ts`，确认编辑状态、宽度和保存数组语义不变。
2. 针对 `src/components/ai-elements/code-block.tsx`、`playground-message-editor.tsx` 和相关测试运行定向 ESLint。
3. `cd web/default && ./node_modules/.bin/tsc -b`，确认 CodeMirror 类型、浏览器事件类型和受控 editor 同步类型正确。
4. `cd web/default && ./node_modules/.bin/rsbuild build`，确认生产构建与新增依赖 chunk 通过。
5. `git diff --check` 检查补丁空白。
6. 优先使用 MCP 打开 `http://192.168.0.202:3003/playground` 验证编辑器加载、输入、Reset、Cancel 确认和控制台状态；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/api/status`、`/playground`，并拉取线上 chunk 与本地 `dist` 比对，确认 3003 页面加载的是本轮构建产物和 CodeMirror 特征。

验证记录：

1. `cd web/default && bun test src/features/playground/lib/message-editor-utils.test.ts src/features/playground/lib/message-styles.test.ts src/features/playground/lib/conversation-message-utils.test.ts` 已通过，17 个 Playground 编辑/样式/消息数组测试全部通过。
2. 定向 ESLint 已通过：`src/components/ai-elements/code-block.tsx`、`src/features/playground/components/playground-message-editor.tsx`、`message-editor-utils.test.ts`、`message-styles.ts`、`message-styles.test.ts` 均无报错。
3. `cd web/default && ./node_modules/.bin/tsc -b` 已通过，CodeMirror 受控同步、浏览器键盘事件和公共编辑器导出类型无类型错误。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 已通过；生产构建总量约 `24246.5 kB / 9209.8 kB gzip`，`async/6675.js` 约 `79.1 kB / 23.5 kB gzip`，`async/5099.js` 约 `85.2 kB / 24.0 kB gzip`，`async/7217.js` 约 `612.3 kB / 206.4 kB gzip`。
5. `git diff --check` 已通过，无补丁空白问题。
6. 实现过程中发现若把 `onKeyDown` 直接放入 CodeMirror 扩展依赖，React 重渲染会导致 `EditorView` 在输入时反复重建；本轮已改为对 `EditorView.dom` 单独挂载原生 `keydown` 监听，保持输入、Reset、切换消息和快捷键语义稳定。
7. 已尝试 MCP Chrome DevTools 验证，但当前环境无法连接 Chrome：`Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮按项目约定执行替代验证。
8. 使用 `curl --noproxy '*'` 真实访问 `http://192.168.0.202:3003/`、`/playground`、`/api/status` 均返回 `200`，确认 3003 页面和接口可访问。
9. 已从 3003 拉取 `/static/js/index.js`、`/static/css/index.css`、`/static/js/async/5099.js`、`/static/js/async/6675.js`、`/static/js/async/7217.js`，与本地 `web/default/dist` 逐字 `cmp` 一致；核心 sha256 分别为 `e72d29e4663c70716e8b74b3a3d1b6917ee399918f681be2aabe5554410bfce4`（index.js）、`9e3e63369896ea21103f6dc231a576871579eda776c22c7fdf5be24b9892a5d9`（index.css）、`8b37e747f92eaaa64a61aa692ec420d11f6c1c7e3fcdf74fb9559aad62c557a3`（5099.js）、`8613b3913b0e129683f7b2074133cc61d251ae2402c4e6691359a883f9fb7e88`（6675.js）、`88150dee012e8d947c80fc06a9513fc228da23f95ae79ee57b6e84cd3d4a1a15`（7217.js）。
10. 线上 chunk 已包含 `cm-editor`、`cm-content`、`addEventListener("keydown"`、`Unsaved changes`、`No changes`、`Save & Submit`、`Reset`、`group-[.is-assistant]:max-w-[78ch]` 等特征，确认 3003 页面资源已加载本轮 CodeMirror 编辑器底座、编辑状态和宽度样式；本轮未新增 UI 文案，无需更新 locale。

## 本轮实施评审：渠道移动端专用卡片原生化

### 需求分析

new-api-main 的渠道管理页有专用 `channel-card.tsx`，移动端不再机械压缩桌面表格，而是把选择框、渠道类型、名称/备注、状态、余额、优先级、权重、响应时间、最近测试时间、分组和行操作重新组织为适合触摸浏览的卡片。NexusTok 当前已经具备 DataTable 默认移动卡片和完整渠道权限/账号池能力，但渠道页仍主要依赖列 meta 自动折叠，复杂字段在手机上扫描成本较高。

本轮目标是把 new-api-main 的“复用列 cell 渲染的渠道专用移动卡片”转为 NexusTok 原生能力：

1. 新增 `web/default/src/features/channels/components/channel-card.tsx`，通过 `flexRender` 复用现有列 cell，不重新实现渠道状态、余额、权限动作、多 Key、账号池和 tag 聚合逻辑。
2. `ChannelsTable` 在 `DataTablePage.mobile` slot 中使用专用 `ChannelsMobileList`，桌面表格、筛选、分页、排序、批量操作和接口请求保持不变。
3. 移动端卡片保留已落地的权限控制：动作 cell 仍走 `DataTableRowActions` / `DataTableTagRowActions`，不绕过 `useChannelPermissions`。
4. 不引入 new-api-main 的渠道表单分区、Advanced Custom 编辑器或 `ChannelRowActionsLayoutContext`，避免把大范围渠道编辑器重构混入本轮。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 渠道移动端列表 | `web/default/src/features/channels/components/channel-card.tsx` | 新增移动端专用 list/card、加载骨架和空状态；复用现有列 cell 和分组 badge。 |
| 渠道表格接入 | `web/default/src/features/channels/components/channels-table.tsx` | 在移动端使用专用卡片；桌面 DataTable、toolbar、pagination 和 bulk actions 不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验收方式和验证记录。 |

### 风险评估

1. 渠道页是高频管理入口，不能改变查询参数、分页、批量选择、权限判断和行操作；本轮只替换移动端展示层，所有数据和动作继续来自同一 TanStack table row。
2. 复用列 cell 能最大化继承 NexusTok 独有能力，但部分 cell 原本为桌面表格设计，移动端必须用稳定 grid/flex 容器限制宽度，避免长渠道名、模型 badge、分组或余额文本挤破布局。
3. Tag 聚合行没有普通渠道 ID 和选择语义，卡片需要继续区分 `isTagAggregateRow`，避免为 tag 行展示无意义的选择框或敏感信息。
4. 本轮不新增 UI 文案，复用 `Priority`、`Weight`、`Used / Remaining`、`Response`、`Last Tested`、`No Channels Found` 等已有 i18n key；不需要修改 locale。
5. MCP Chrome 当前可能仍不可用；页面验证需先尝试 MCP，失败时用 3003 真实页面、资源 hash 和线上 chunk 特征作为替代，并明确记录无法完成真实浏览器交互的原因。

### 方案评审

采用“专用移动端展示、列 cell 复用”的方案，而不是引入 new-api-main 的完整渠道表单和 DataTable 分层。`ChannelCard` 负责单条移动端布局，`ChannelsMobileList` 负责 loading/empty/row 容器；`ChannelsTable` 只把它接入 `DataTablePage.mobile`。这样可以吸收 new-api-main 在移动端渠道管理上的优势，同时保持 NexusTok 已有账号池、Codex、权限矩阵、多 Key 和上游模型同步操作全部由现有列与 action 组件承载。

验收方式：

1. 针对 `channel-card.tsx`、`channels-table.tsx` 和相关渠道表格文件运行定向 ESLint。
2. `cd web/default && ./node_modules/.bin/tsc -b`，确认 TanStack row/cell 类型和移动端组件 props 正确。
3. `cd web/default && ./node_modules/.bin/rsbuild build`，确认生产构建通过。
4. `git diff --check` 检查补丁空白。
5. 优先使用 MCP 打开 `http://192.168.0.202:3003/channels`，以移动端 viewport 验证渠道列表卡片、筛选工具条、行操作菜单和控制台状态；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/channels`、`/api/status`，并拉取线上 `index.js`、CSS 和渠道相关 chunk 与本地 `dist` 比对，确认 3003 页面加载本轮构建产物。

验证记录：

1. `cd web/default && ./node_modules/.bin/eslint src/features/channels/components/channel-card.tsx src/features/channels/components/channels-table.tsx` 通过。
2. `cd web/default && ./node_modules/.bin/tsc -b` 通过。
3. `cd web/default && ./node_modules/.bin/rsbuild build` 通过；生产构建总量为 `24251.2 kB / 9210.7 kB gzip`，渠道相关 async chunk 为 `dist/static/js/async/3020.3a9a61b0c9.js`（`251.2 kB / 58.0 kB gzip`）。
4. `git diff --check` 通过。
5. 已优先尝试 MCP Chrome 打开 `http://192.168.0.202:3003/channels`，但当前环境仍无法连接 Chrome 远程调试端：`Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮按约定使用 3003 真实 HTTP 页面和资源 hash 兜底验证。
6. `curl --noproxy '*'` 访问 `http://192.168.0.202:3003/`、`/channels`、`/api/status` 均返回 `200`；`/api/status` 返回正常 JSON，确认后端状态接口可用。
7. 已从 3003 拉取 `/static/js/index.js`、`/static/css/index.css`、`/static/js/async/3020.js`，分别与本地 `web/default/dist/static/js/index.js`、`web/default/dist/static/css/index.css`、`web/default/dist/static/js/async/3020.js` 逐字 `cmp` 一致；sha256 分别为 `5c661e0ebdbeef2da067b03ac9a5e913d1ced912d24255aa76d1d666c73e2596`（index.js）、`22eb2d6118acd2bf3ceab39c799258d25f8c7cc1faef805b9bd57365b1fe9b64`（index.css）、`db02f700719db20f4c3b4ad7b55aa511eba9c0c3a52e2a504f64807ae8c38d3f`（3020.js）。
8. 线上渠道 chunk 已包含 `Used / Remaining`、`Last Tested`、`No Channels Found`、`No channels available. Create your first channel to get started.` 等移动端卡片特征，确认 3003 页面资源已加载本轮渠道卡片构建产物；本轮未新增 UI 文案，无需更新 locale。

## 本轮实施评审：模型倍率 JSON 模式 CodeMirror 编辑器原生化

### 需求分析

new-api-main 在 `web/default/src/components/json-code-editor.tsx` 中把系统设置里的 JSON 文本编辑从普通 `Textarea` 提升为带行号、JSON 状态和格式化按钮的代码编辑体验。NexusTok 当前模型倍率页已经具备更完整的可视化编辑器、计费表达式系统和 CodeMirror 底座，但在 `ModelRatioForm` 的 JSON 模式中仍使用多个普通 `Textarea`，管理员编辑大体量模型价格、倍率、缓存倍率、音频倍率和表达式映射时缺少结构化编辑反馈。

本轮目标是把 new-api-main 的“JSON 模式结构化编辑”转为 NexusTok 原生能力：

1. 新增公共 `web/default/src/components/json-code-editor.tsx`，复用 NexusTok 已落地的 `CodeBlockEditor` / CodeMirror 底座，不额外引入 CodeMirror 依赖。
2. 在 `web/default/src/features/system-settings/models/model-ratio-form.tsx` 的 JSON 模式中替换原始 `Textarea`，保留每个 `FormField`、字段名、保存按钮、重置按钮、schema 校验和后台保存 API 不变。
3. 提供 JSON 语法状态和 `Format JSON` 动作；空值保持可编辑且视为未填写，非法草稿不被格式化覆盖，最终保存仍由现有表单校验兜底。
4. 计费表达式系统相关字段 `BillingMode`、`BillingExpr` 的存储语义不变；本轮只改 JSON map 外层编辑体验，不改表达式语言、变量、`p/c/len` 归一化、预消费或结算逻辑。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 公共 JSON 编辑器 | `web/default/src/components/json-code-editor.tsx` | 新增基于 CodeMirror 的 JSON 编辑器，包含标题、状态和格式化动作。 |
| 模型倍率设置 | `web/default/src/features/system-settings/models/model-ratio-form.tsx` | JSON 模式从 `Textarea` 切换到公共编辑器；可视化模式和保存/重置逻辑不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险、方案、验收方式和验证记录。 |

### 风险评估

1. 模型倍率设置直接影响计费，不能在 UI 层修改字段值的含义；本轮只在用户输入、JSON 格式化按钮和表单受控值之间同步字符串，不改任何字段名称或提交结构。
2. JSON 格式化可能改变空白但不能改变数据结构；实现中只在 `JSON.parse` 成功时执行 `JSON.stringify(parsed, null, 2)`，非法草稿保持原样并显示错误状态。
3. `BillingExpr` 字段本身是 JSON map，value 可能包含表达式字符串和 `|||` request rule；格式化 JSON 只能处理外层 JSON，表达式字符串内容保持原值，避免破坏计费表达式系统的一表达式一真相原则。
4. 复用 `CodeBlockEditor` 会沿用 Playground 的 CodeMirror 生命周期；需要通过 TypeScript 和构建确认多个编辑器实例不会影响表单渲染。
5. 本轮复用已有 `JSON`、`Invalid JSON`、`Format JSON` 等 i18n key，不新增文案；如实际实现新增文案，则必须补齐 `en/zh/fr/ja/ru/vi` 并运行 `bun run i18n:sync`。
6. MCP Chrome 当前可能仍不可用；完成后仍需先尝试 MCP 打开系统设置模型页，失败时使用 3003 页面、接口状态、资源 hash 和线上 chunk 特征兜底验证。

### 方案评审

采用“公共 JSON 编辑器 + 局部替换 Textarea”的方案，而不是搬运 new-api-main 的手写行号编辑器。NexusTok 已有 `CodeBlockEditor`，继续复用可避免重复维护滚动同步、行号、键盘事件和 editor 生命周期；`JsonCodeEditor` 只负责 JSON 状态、格式化动作和表单无障碍属性转发。`ModelRatioForm` 通过一个本地 helper 统一渲染 JSON 字段，减少八个 `FormField` 的重复，同时保持每个字段的 label、description、rows 和 `FormMessage`。

验收方式：

1. 定向 ESLint 检查 `json-code-editor.tsx` 和 `model-ratio-form.tsx`。
2. `cd web/default && ./node_modules/.bin/tsc -b`，确认公共编辑器 props、react-hook-form 字段和 CodeMirror 受控值类型正确。
3. `cd web/default && ./node_modules/.bin/rsbuild build`，确认生产构建通过。
4. 如无新增 i18n 文案，扫描确认使用的 `JSON`、`Invalid JSON`、`Format JSON` 已存在于各 locale；如有新增则运行 `bun run i18n:sync`。
5. `git diff --check` 检查补丁空白。
6. 优先使用 MCP 打开 `http://192.168.0.202:3003/pricing-settings`，切到 JSON 模式验证编辑器、格式化按钮和控制台状态；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、目标页面、`/api/status`，并拉取线上 `index.js`、CSS 和定价设置相关 chunk 与本地 `dist` 比对，确认 3003 页面加载本轮构建产物。

验证记录：

1. 已按计费表达式系统要求先阅读 `pkg/billingexpr/expr.md`，确认本轮只改模型倍率 JSON map 的编辑体验，不改变 `BillingMode`、`BillingExpr`、表达式语言、`p/c/len` 归一化、预消费或结算逻辑。
2. `cd web/default && ./node_modules/.bin/eslint src/components/json-code-editor.tsx src/components/ai-elements/code-block.tsx src/features/system-settings/models/model-ratio-form.tsx` 通过。
3. `cd web/default && ./node_modules/.bin/tsc -b` 通过。
4. `cd web/default && ./node_modules/.bin/rsbuild build` 通过；生产构建总量为 `24257.7 kB / 9213.4 kB gzip`，本轮相关 chunk 包括 `dist/static/js/async/9522.e6a9f9e1c2.js`、`dist/static/js/async/8130.f0a8779084.js`、`dist/static/js/async/5099.7a9a672d2b.js`、`dist/static/js/async/6675.cf4e7544f7.js`。
5. 已扫描 `en/zh/fr/ja/ru/vi` locale，确认 `JSON`、`JSON Editor`、`Invalid JSON`、`Format JSON` 均已存在；本轮无新增 UI 文案，无需运行 `bun run i18n:sync`。
6. `git diff --check` 通过。
7. 已优先尝试 MCP Chrome 打开 `http://192.168.0.202:3003/pricing-settings`，但当前环境仍无法连接 Chrome 远程调试端：`Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮按约定使用 3003 真实 HTTP 页面和资源 hash 兜底验证。
8. `curl --noproxy '*'` 访问 `http://192.168.0.202:3003/`、`/pricing-settings`、`/api/status` 均返回 `200`；`/api/status` 返回正常 JSON，确认后端状态接口可用。
9. 已从 3003 拉取 `/static/js/index.js`、`/static/css/index.css`、`/static/js/async/9522.js`、`/static/js/async/8130.js`、`/static/js/async/5099.js`、`/static/js/async/6675.js`，与本地 `web/default/dist` 逐字 `cmp` 一致；sha256 分别为 `f6534ac2678517a15323d0c1ea6a8215626f9f434820a192ae4c759015ca7616`（index.js）、`22eb2d6118acd2bf3ceab39c799258d25f8c7cc1faef805b9bd57365b1fe9b64`（index.css）、`b619df6e2f4b221d80a4d84d656670134717fedb8de04a15fcf9e3c3b817a668`（9522.js）、`9f5daf4ca878611947801a82cb90ade4d870e8d2f997ae3cce432769930a1367`（8130.js）、`a4a086d056ab93f9302c0aa14efc4ae7c1387b225c3790dd6e1b06fb5db98c8d`（5099.js）、`638a900f61cfed997859b1585408580f1847215067b0cf81ea608d278bb1b548`（6675.js）。
10. 线上 chunk 已包含 `Format JSON`、`Invalid JSON`、`Model fixed pricing`、`Advanced bulk pricing`、`aria-readonly`、`readOnly`、`autoFocus` 等特征，确认 3003 页面资源已加载本轮 JSON CodeMirror 编辑器、格式化动作和多编辑器非自动聚焦/readOnly 支持。

## 本轮实施评审：表格展示原子组件原生化

### 需求分析

new-api-main 在 `web/default/src/components/table-id.tsx`、`provider-badge.tsx` 和 `truncated-text.tsx` 中把表格里高频出现的 ID、供应商/渠道类型 badge、长文本截断提示抽成公共原子组件。NexusTok 当前有 `StatusBadge`、`LongText`、`getLobeIcon` 等基础能力，但模型表和渠道表仍在列定义里重复手写 ID badge、provider 图标、长名称截断和 tooltip。随着渠道账号池、模型部署、Usage Logs、Pricing 等页面继续增长，这类展示逻辑如果继续分散，会让后续 DataTable 分层迁移和移动端卡片复用成本变高。

本轮目标是把 new-api-main 的表格展示组件优势转为 NexusTok 原生能力：

1. 新增 `TableId`，用稳定的 mono/tabular 数字样式展示表格 ID；默认不复制，避免改变 `StatusBadge` 的点击复制语义，确需复制的列可显式打开。
2. 新增 `ProviderBadge`，复用 NexusTok 现有 `StatusBadge` 和 `getLobeIcon`，把 provider/vendor/channel type 的图标、颜色和不可复制默认值统一起来。
3. 新增 `TruncatedText`，基于 NexusTok 现有 `LongText` 做薄封装，保留桌面 Tooltip 和移动端 Popover 的行为，同时给表格列提供统一 `maxWidth`/`side` 参数。
4. 先接入模型管理表和渠道管理表这两个高频页面：模型表的 ID/vendor 列和渠道表的 ID/name/type 列；不一次性改 users、subscriptions、redemptions、usage logs 等更多表格。
5. 保留 NexusTok 已有账号池、多 Key、Codex 用量、IO.NET deployment、tag 聚合、上游模型同步和权限动作逻辑，不搬运 new-api-main 的 DataTable 分层、`BadgeCell`、`TruncatedCell` 或渠道 action context。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 公共表格 ID 展示 | `web/default/src/components/table-id.tsx` | 新增可复用 ID 原子组件，默认只展示，显式 `copyable` 时才允许点击复制。 |
| 公共 provider badge | `web/default/src/components/provider-badge.tsx` | 新增 provider/vendor/channel type badge，统一图标、颜色、截断和默认不可复制语义。 |
| 公共长文本截断 | `web/default/src/components/truncated-text.tsx`、`web/default/src/components/long-text.tsx` | 新增表格长文本薄封装；`LongText` 支持响应式 overflow 重新测量，避免初始渲染后列宽变化导致 tooltip 状态陈旧。 |
| 模型表格 | `web/default/src/features/models/components/models-columns.tsx` | ID 和 Vendor 列使用公共组件；模型名称、状态、权限动作和其它列不变。 |
| 渠道表格 | `web/default/src/features/channels/components/channels-columns.tsx` | ID、Name、Type 列使用公共组件；账号池、多 Key、IO.NET、tag 聚合、余额和操作列不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险、方案、验收方式和验证记录。 |

### 风险评估

1. 表格列是管理后台高频入口，不能改变排序、筛选、选择、批量操作、权限判断和 API 调用；本轮只改 cell 内部展示组件，不改 column accessor、filterFn、row action 或数据请求。
2. `StatusBadge` 默认可复制，new-api-main 的 `ProviderBadge` 如果直接复制会让点击 provider 意外复制文本；本轮 `ProviderBadge` 默认 `copyable=false`，同时保留显式 override。
3. 渠道名称列承载 pass-through 警告、多 Key 数量、账号池统计和上游模型同步标签；接入 `TruncatedText` 必须只替换名称/备注文本，不移动或删除这些 NexusTok 自有 badge。
4. 渠道 type 列承载多 Key 轮询/随机图标和 IO.NET deployment 跳转；接入 `ProviderBadge` 只能替换 provider 名称 badge，不改变 multi-key tooltip 和 IO.NET 点击路径。
5. `LongText` 之前只在 mount 时检测 overflow，列宽、字体加载或数据刷新后可能出现 tooltip 状态滞后；加入 `ResizeObserver` 要注意浏览器兼容和卸载清理，避免列表频繁渲染造成泄漏。
6. 本轮不新增 UI 文案，复用现有列标题和 badge 文本；不需要补 locale 或运行 `bun run i18n:sync`。
7. MCP Chrome 当前可能仍不可用；完成后仍需先尝试 MCP 打开 `http://192.168.0.202:3003/channels` 和 `/models/metadata`，失败时使用 3003 页面、状态接口、资源 hash 和线上 chunk 特征兜底验证。

### 方案评审

采用“公共展示组件 + 两张高频表小范围接入”的方案，而不是一次性迁移 new-api-main 的 DataTable core/layout/toolbar 分层。`TableId`、`ProviderBadge`、`TruncatedText` 都放在当前 `components/` 根目录，继续使用 NexusTok 已有 Base UI Tooltip/Popover、`StatusBadge`、`getLobeIcon` 和 `cn` 工具。设计方向延续默认前端 Base Nova 管理台：紧凑、可扫描、语义色来自现有 design token，不新增装饰性颜色、卡片或说明文案。

验收方式：

1. 定向 ESLint 检查新增组件和两个接入列文件。
2. `cd web/default && ./node_modules/.bin/tsc -b`，确认公共组件 props、TanStack column cell 类型和 `ResizeObserver` 类型正确。
3. `cd web/default && ./node_modules/.bin/rsbuild build`，确认生产构建通过。
4. `git diff --check` 检查补丁空白。
5. 优先使用 MCP 打开 `http://192.168.0.202:3003/channels` 和 `/models/metadata`，验证表格 ID、渠道名称截断、渠道类型 badge、模型 vendor badge、移动端渠道卡片复用列 cell 后仍正常显示；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/`、`/channels`、`/models/metadata`、`/api/status`，并拉取线上 `index.js`、CSS 和相关 async chunk 与本地 `dist` 比对，确认 3003 页面加载本轮构建产物。

验证记录：

1. `cd web/default && ./node_modules/.bin/eslint src/components/table-id.tsx src/components/provider-badge.tsx src/components/truncated-text.tsx src/components/long-text.tsx src/features/models/components/models-columns.tsx src/features/channels/components/channels-columns.tsx` 通过，且无 warning。
2. `cd web/default && ./node_modules/.bin/tsc -b` 通过，确认 `TableId`、`ProviderBadge`、`TruncatedText`、`LongTextSide`、Base UI `side` 参数和 TanStack column cell 类型正确。
3. `cd web/default && ./node_modules/.bin/rsbuild build` 通过；生产构建总量为 `24261.3 kB / 9214.6 kB gzip`。本轮相关 chunk 包括公共 `provider-badge` chunk `dist/static/js/async/2252.js`、渠道表 chunk `dist/static/js/async/4516.js`、模型管理 chunk `dist/static/js/async/4050.js`，以及复用 `LongText` 的共享/页面 chunk。
4. `git diff --check` 通过。
5. 已优先尝试 MCP Chrome 打开 `http://192.168.0.202:3003/channels`，但当前环境仍无法连接 Chrome 远程调试端：`Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮按约定使用 3003 真实 HTTP 页面、资源 hash 和线上 chunk 特征兜底验证。
6. `curl --noproxy '*'` 访问 `http://192.168.0.202:3003/`、`/channels`、`/models/metadata`、`/api/status` 均返回 `200`；`/api/status` 返回正常 JSON，确认后端状态接口可用。
7. 已从 3003 拉取 `/static/js/index.js`、`/static/css/index.css`、`/static/js/async/2252.js`、`/static/js/async/4516.js`、`/static/js/async/4050.js`、`/static/js/async/8579.js` 等相关资源，并与本地 `web/default/dist` 逐字 `cmp` 一致；关键 sha256 为 `6b49f4293b8af999d03b4c4a7ed18a26f2735bddc55001bba9d491ca2a260e94`（index.js）、`73ffa1bdcc142cb2fc260dc83a39f1866f232fa30c59d1049e0a6b113e658814`（index.css）、`52c67886c73376734ebac2ba5797fcf1a62c76c0ca1629cf3cffa95f03ac4f89`（2252.js）、`b9fbd19484070d289165bef7be31ea1ff9ba2c77a1f24d669f589cdae27116f6`（4516.js）、`389ce483e57da353da5746735f234d2ff1f166d4177508712ba276aa9492205d`（4050.js）、`1cad09ab198db1e89f3656fdf0f813da5fe214a5c0a5da5294a3732c612c06fa`（8579.js）。
8. 线上 chunk 特征确认：`2252.js` 包含 `provider-badge`；`4516.js` 包含 `ResizeObserver`、`Tag Aggregate`、`Multi-key: Random rotation`、`From IO.NET deployment`、`Model Name`；`4050.js` 包含 `Model Name`、`Vendor`；`index.js` 路由表包含模型管理和渠道管理相关加载配置。确认 3003 页面资源已加载本轮表格展示原子组件和两个高频表格接入产物。
9. 本轮未新增 UI 文案，只新增组件标识和中文维护注释；无需修改 `en/zh/fr/ja/ru/vi` locale，也无需运行 `bun run i18n:sync`。

## 本轮实施评审：节点身份公共化原生化

### 需求分析

new-api-main 新增了 `common/node_identity.go`，把节点身份解析统一沉到 `common` 层：优先使用运维显式配置的 `NODE_NAME`，未配置时回退主机名，并暴露来源、是否手动配置、是否建议手动配置等元信息。NexusTok 当前已经有系统实例心跳和前端 `Configure NODE_NAME` 提示，但解析逻辑仍主要写在 `service/system_instance.go`；`common.NodeName` 本身只读取环境变量，导致充值审计、普通请求日志、`QuotaData` 用量流和 SystemTask runner 在未配置 `NODE_NAME` 时仍可能写入空节点名，而系统实例列表却显示 hostname。这个差异会让多实例排障和日志关联不一致。

本轮目标是把 new-api-main 的节点身份公共化能力转为 NexusTok 原生能力：

1. 在 `common` 层新增 `NodeIdentity`、`GetNodeIdentity()` 和初始化 helper，集中管理 `NodeName`、来源和手动配置标记。
2. `InitEnv()` 初始化时调用节点身份 helper，使 `common.NodeName` 在未配置 `NODE_NAME` 时也有 hostname 兜底；审计日志、用量导出、SystemTask runner 和系统实例心跳共享同一个非空身份。
3. `service/system_instance.go` 复用 `common.NodeIdentity`，保留 `ResolveSystemInstanceNode()` 作为服务层校验包装，避免上报空节点名。
4. 保持前端系统实例页面现有 `Configure NODE_NAME` 提示和六语文案不变；本轮不新增 UI 文案。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 公共节点身份 | `common/constants.go`、`common/init.go`、`common/node_identity.go` | 新增节点名来源常量、手动配置标记和统一身份查询；未配置 `NODE_NAME` 时回退 hostname。 |
| 系统实例上报 | `service/system_instance.go`、`service/system_instance_test.go` | 心跳上报复用 common 身份，测试覆盖手动配置、hostname 兜底和空身份错误路径。 |
| 公共身份测试 | `common/node_identity_test.go` | 覆盖 `NODE_NAME` 手动配置和 hostname fallback 的全局变量语义。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险、方案、验收方式和验证记录。 |

### 风险评估

1. `common.NodeName` 从“未配置时为空”变为“未配置时 hostname”，会改变新增日志、充值审计、用量导出和 SystemTask runner ID 的节点名字段；这是预期修复，但需要确认没有测试依赖全局 `common.NodeName == ""` 的旧语义。
2. hostname 不是多实例生产环境的稳定身份，系统实例页面仍必须通过 `should_configure_manually=true` 提示运维配置 `NODE_NAME`；本轮保留该字段，不把 hostname 当成手动配置。
3. `InitEnv()` 运行在进程启动早期，节点身份 helper 不能依赖数据库、Redis、logger 或复杂配置，避免初始化环路；实现只读取环境变量和 `os.Hostname()`。
4. `os.Hostname()` 在极端环境可能失败或返回空；此时 `NodeName` 保持空，`ReportCurrentSystemInstance()` 继续返回错误，不写入空主键的实例心跳。
5. 本轮不改数据库结构、不改路由、不改系统实例 API response schema；前端类型和页面已经兼容 `name/source/manually_configured/should_configure_manually`。
6. 后端改动没有新增前端文案，不需要更新 locale；但仍需按项目要求尝试 MCP/3003 验证 `/system-info` 和 `/api/status`。

### 方案评审

采用“common 层统一身份 + service 层轻校验”的方案，而不是只在系统实例服务里继续复制 hostname fallback。`common.InitEnv()` 初始化 `NodeName`、`NodeNameSource`、`NodeNameManuallyConfigured`，所有使用 `common.NodeName` 的现有路径自然获得一致身份；`service.ResolveSystemInstanceNode()` 保留并复用 `common.GetNodeIdentity()`，只负责上报前的空值兜底和错误返回。这样能吸收 new-api-main 的基础设施优势，同时不改变数据库迁移、API schema、系统信息页面和日志写入调用点。

验收方式：

1. `go test ./common ./service -run 'Test(NodeIdentity|ResolveSystemInstanceNode)'`，验证节点身份解析和系统实例服务包装。
2. `go test ./model -run TestSystemInstance`，确认系统实例模型读写仍兼容。
3. `go test ./service -run TestSystemTask`，确认 SystemTask runner 相关逻辑仍可构建。
4. `go test ./common ./service ./model`，覆盖本轮相关包的常规测试。
5. `git diff --check` 检查补丁空白。
6. 优先使用 MCP 打开 `http://192.168.0.202:3003/system-info`，确认页面和控制台状态；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 访问 `/system-info`、`/api/system-info/instances` 和 `/api/status`，确认 3003 运行页面和后端接口可访问，并记录无法真实浏览器验证的原因。

验证记录：

1. 单元测试通过：`go test ./common -run TestInitNodeNameIdentity` 覆盖 `NODE_NAME` 手动配置与 hostname fallback 的公共身份初始化语义。
2. 定向测试通过：`go test ./common ./service -run 'Test(NodeIdentity|ResolveSystemInstanceNode)'`。其中 `common` 包正则未命中测试名，已由上一条命令单独覆盖；`service` 包覆盖系统实例节点解析包装。
3. 相关服务测试通过：`go test ./service -run 'Test(ResolveSystemInstanceNode|SystemTask)'`，确认系统实例节点解析和 SystemTask runner 相关逻辑仍可构建。
4. 模型测试通过：`go test ./model -run TestSystemInstance`，确认系统实例心跳模型写入、列表和响应转换不受影响。
5. 相关包整包测试通过：`go test ./common ./service ./model`。
6. 补丁空白检查通过：`git diff --check`。
7. MCP 浏览器验证已按要求尝试打开 `http://192.168.0.202:3003/system-info`，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮采用同一 3003 服务的真实 HTTP 请求兜底验证。
8. 3003 页面与接口验证通过：`/` 返回 200，`/system-info` 返回 200，`/api/status` 返回 200；未登录访问 `/api/system-info/instances` 返回 401，消息为 `Unauthorized, not logged in and no access token provided`。
9. 登录态接口验证通过：使用账号 `c1cada` 登录 `/api/user/login?turnstile=` 返回 `success=true`，随后按默认前端 `web/default/src/lib/api.ts` 的约定携带 `NexusTok-User: 1` 和 session cookie 访问 `/api/system-info/instances` 返回 200。返回数据包含 1 个在线实例，节点字段为 `name=nexustok-hot-node-1`、`source=manual`、`manually_configured=true`、`should_configure_manually=false`，确认系统实例页面消费的新结构已在热更新容器生效。

## 本轮实施评审：兑换码状态筛选原生化

### 需求分析

new-api-main 在兑换码管理里已经把 `status` 作为搜索接口参数下沉到模型层，支持后端分页下按可用、禁用、已使用和虚拟过期状态筛选。NexusTok 默认前端已经有兑换码状态筛选 UI，也定义了 `expired` 虚拟状态，但当前 `searchRedemptions()` 只发送 `keyword`，后端 `controller.SearchRedemptions()` 和 `model.SearchRedemptions()` 也只接收关键词。这会造成两个问题：一是管理员选择状态筛选时只可能依赖前端当前页数据，无法在后端分页总量里得到完整结果；二是 `Expired` 这类由 `expired_time` 计算出的虚拟状态不能被当前 API 正确分页筛选。

同时，new-api-main 的 `Redeem` 在事务内使用 `WHERE id = ? AND status = enabled` 的 compare-and-swap 更新兑换码状态，只有成功把兑换码从 enabled 翻转为 used 的事务才会给用户加额度。NexusTok 当前依赖 `lockForUpdate` 读锁后先增加用户额度、再保存兑换码状态；在 SQLite 或锁语义退化的环境中，这类额度路径缺少显式 CAS 兜底。兑换码属于资金/额度入口，本轮应一并吸收该幂等保护。

本轮目标是把 new-api-main 的兑换码状态筛选能力转成 NexusTok 原生能力：

1. 后端 `GET /api/redemption/search` 接收可选 `status` 查询参数。
2. 模型层 `SearchRedemptions` 支持关键词与状态组合过滤；`expired` 表示 `status=enabled` 且 `expired_time` 已过当前时间；`enabled` 排除已过期兑换码。
3. 默认前端状态筛选与关键词搜索都走后端 search API，并把状态值写入 query key 和请求参数，分页总数以服务端结果为准。
4. 保留现有 API 兼容性：未传 `status` 时搜索行为不变；状态值非法或空字符串时不额外过滤。
5. `Redeem` 使用兑换码状态 CAS 作为最终幂等门闩，避免同一兑换码在并发场景下重复入账。
6. 补齐模型测试覆盖关键词、状态、过期虚拟状态、分页总数、重复兑换和并发兑换只入账一次。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 兑换码模型查询与兑换 | `model/redemption.go`、`model/redemption_test.go` | `SearchRedemptions` 增加状态参数与组合过滤；`Redeem` 增加状态 CAS 幂等保护；新增回归测试。 |
| 兑换码控制器 | `controller/redemption.go` | 搜索接口读取 `status` 并传入模型层，保持分页响应结构不变。 |
| 默认前端兑换码列表 | `web/default/src/features/redemption-codes/{api.ts,types.ts,components/redemptions-table.tsx}` | 搜索请求追加 `status`，列表按状态筛选时走后端分页；状态筛选设置为单选。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险、方案、验收方式和验证记录。 |

### 风险评估

1. `SearchRedemptions` 签名变化会影响所有调用点，需要同步控制器和测试；当前只有兑换码搜索控制器直接调用，改动面可控。
2. `expired` 是虚拟状态，不是数据库真实 `status` 值；实现必须与前端 `isRedemptionExpired` 语义一致，只把已启用且 `expired_time != 0 && expired_time < now` 的兑换码视为过期。
3. `enabled` 筛选应排除已过期兑换码，否则 `Unused` 和 `Expired` 会在列表中重叠；这与 new-api-main 行为一致。
4. 查询条件必须使用 GORM `Where` 和参数绑定，避免数据库方言差异和 SQL 注入风险；不新增迁移，不改变兑换码表结构。
5. 前端从局部过滤转为服务端过滤后，`pageCount` 和 `total` 应完全来自后端；需要设置手动分页/过滤，避免服务端结果再被当前页二次过滤导致空页。
6. `Redeem` 改为先 CAS 更新兑换码状态、再增加用户额度；两步必须在同一个事务中，确保后续用户额度更新失败时兑换码状态也会回滚。
7. CAS 条件只判断当前记录仍是 enabled，不能跳过过期时间检查；过期兑换码仍要在 CAS 前返回失败。
8. 本轮新增测试与前端类型变更，不新增 UI 文案；无需更新 i18n locale，但仍需运行前端类型检查/构建并访问 3003 `/redemption-codes` 验证热更新页面和接口。

### 方案评审

采用“后端搜索能力补齐 + 前端筛选参数透传 + 兑换 CAS 幂等保护”的方案。后端保持 `GetAllRedemptions` 不变，仅增强 `/api/redemption/search`，这样未筛选列表仍走旧路径；当关键词或状态任一存在时，前端统一走搜索接口。模型层用同一个 GORM 查询对象叠加关键词和状态条件，分页前先 `Count` 得到完整总数，再按 `id desc` 分页查询。兑换路径继续保留 `lockForUpdate` 作为支持行锁数据库的第一道保护，同时增加状态 CAS 作为跨数据库最终门闩，并且把 CAS 与用户额度增加放在同一个事务内。前端只复用现有状态筛选 UI 和 URL 状态，不新增文案、不新增路由。

验收方式：

1. `go test ./model -run 'TestSearchRedemptions|TestRedeem'`，验证兑换码搜索、状态筛选、分页和兑换并发保护。
2. `go test ./controller -run TestRedemption`，确认控制器和路由相关测试仍通过。
3. `go test ./model ./controller`，覆盖本轮后端相关包。
4. 在 `web/default/` 执行 `bun run lint`、`bun run typecheck`、`bun run build`。
5. `git diff --check` 检查补丁空白。
6. 优先用 MCP 打开 `http://192.168.0.202:3003/redemption-codes`，登录后切换状态筛选并观察网络请求；如 MCP 仍不可用，则用 `curl --noproxy '*'` 登录后验证 `/api/redemption/search?keyword=&status=1`、`status=expired`、`status=2`、`status=3` 的响应状态和分页结构，并访问 `/` 与 `/redemption-codes` 确认热更新页面可达。

验证记录：

1. 模型定向测试通过：`go test ./model -run 'TestSearchRedemptions|TestRedeem'`，覆盖关键词搜索、状态筛选、`expired` 虚拟状态、分页总数、重复兑换和并发兑换只入账一次。
2. 后端相关包测试通过：`go test ./model ./controller`、`go test ./controller`、`go test ./router -run TestRedemption`。
3. 前端定向 ESLint 通过：`bunx eslint src/features/redemption-codes/api.ts src/features/redemption-codes/types.ts src/features/redemption-codes/components/redemptions-table.tsx`。
4. 前端类型检查通过：`bun run typecheck`。
5. 前端生产构建通过：`bun run build`，总量 `24261.3 kB / 9214.7 kB gzip`。
6. 补丁空白检查通过：`git diff --check`。
7. 全量 `bun run lint` 已执行，但失败在本轮未触碰的既有文件上，包括 `risk-acknowledgement-dialog.tsx`、`dashboard/components/flow/flow-charts.tsx`、`keys/components/api-keys-dialogs.tsx`、`system-info/components/system-instances-panel.tsx`、`system-settings/models/*`、`usage-logs/components/*`、`theme-radius.ts` 等 React Hooks/Query 规则和一个重复 import；本轮触碰的兑换码文件定向 ESLint 已通过。
8. MCP 浏览器验证已按要求尝试打开 `http://192.168.0.202:3003/redemption-codes`，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮采用同一 3003 服务的真实 HTTP 请求和资源一致性兜底验证。
9. 3003 页面与基础接口验证通过：`/` 返回 200，`/redemption-codes` 返回 200，`/api/status` 返回 200。
10. 登录态兑换码搜索接口验证通过：使用账号 `c1cada` 登录 `/api/user/login?turnstile=` 返回 `success=true`，随后携带 session cookie 与 `NexusTok-User: 1` 访问 `/api/redemption/search?keyword=&status=1&p=1&page_size=5`、`status=expired`、`status=2`、`status=3` 均返回 200，响应结构均为 `success=true`、`page=1`、`page_size=5`、`total=0`、`items=[]`。当前热更新环境没有兑换码数据，筛选语义由模型单元测试覆盖。
11. 3003 前端资源验证通过：线上 `/static/js/async/4474.js` 与本地 `web/default/dist/static/js/async/4474.js` SHA256 均为 `c355d8e4db3b3cdfbf3be8fa1c5bb627599c785df4b8c445bc2a7d473f4870c0`，且 chunk 同时包含 `redemption/search?`、`URLSearchParams` 和 `manualFiltering`；线上 `/static/js/index.js` 与本地 `web/default/dist/static/js/index.js` SHA256 均为 `6b49f4293b8af999d03b4c4a7ed18a26f2735bddc55001bba9d491ca2a260e94`，确认热更新页面加载的是本轮构建产物。

## 本轮实施评审：监控配置环境变量覆盖测试

### 需求分析

new-api-main 补充了 `setting/operation_setting/monitor_setting_test.go`，用于锁定渠道自动测试配置的环境变量覆盖语义：`CHANNEL_TEST_FREQUENCY` 可以覆盖自动测试频率并启用定时测试，`CHANNEL_TEST_ENABLED=false` 即使在配置已启用时也必须关闭自动测试，`CHANNEL_TEST_ENABLED=true` 也可以开启原本禁用的配置。NexusTok 当前 `GetMonitorSetting()` 已具备这些运行时行为，并额外维护 `ChannelTestMode`，但缺少对应测试保护；后续调整 Routing Reliability、SystemTask 或渠道自动测试时容易误改环境变量优先级。

本轮目标是把 new-api-main 的监控配置测试优势转为 NexusTok 原生回归保护：

1. 新增 `setting/operation_setting/monitor_setting_test.go`，覆盖 `CHANNEL_TEST_ENABLED=false` 覆盖已启用配置。
2. 覆盖 `CHANNEL_TEST_ENABLED=true` 可以启用禁用配置。
3. 覆盖 `CHANNEL_TEST_FREQUENCY` 有效时会更新分钟数，并保持默认 `scheduled_all` 模式。
4. 测试结束后恢复包级 `monitorSetting`，避免污染其他配置测试。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 运营监控配置测试 | `setting/operation_setting/monitor_setting_test.go` | 新增环境变量覆盖行为的单元测试，不改运行时逻辑。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险、方案、验收方式和验证记录。 |

### 风险评估

1. 本轮只新增测试，不修改生产代码、数据库、路由或前端页面，运行时风险极低。
2. `monitorSetting` 是包级变量，测试必须用 `t.Cleanup` 恢复原值；否则会影响同包后续测试或并行测试。
3. 测试使用 `t.Setenv` 注入环境变量，不能与 `t.Parallel` 混用；本轮不启用并行测试。
4. `GetMonitorSetting()` 会规范化未知 `ChannelTestMode` 为 `scheduled_all`，测试应显式断言该不变量，避免后续新增模式时破坏默认行为。
5. 该改动不涉及页面和接口，但按项目要求仍需访问 3003 `/` 和 `/api/status`，确认热更新服务可达；MCP 如仍不可用则记录并使用 HTTP 兜底。

### 方案评审

采用“纯测试补齐”的方案，不改变当前 `GetMonitorSetting()` 实现。测试直接设置 `monitorSetting` 初始状态和环境变量，调用 `GetMonitorSetting()` 后断言启用状态、分钟数和 `ChannelTestMode`。这样能吸收 new-api-main 的配置语义保护，同时保持 NexusTok 已有 Routing Reliability、SystemTask 定时任务和渠道自动测试运行路径不变。

验收方式：

1. `go test ./setting/operation_setting -run TestGetMonitorSetting`。
2. `go test ./setting/operation_setting`。
3. `git diff --check`。
4. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/` 和 `/api/status` 返回 200。

验证记录：

1. 定向测试通过：`go test ./setting/operation_setting -run TestGetMonitorSetting`。
2. 包测试通过：`go test ./setting/operation_setting`。
3. 补丁空白检查通过：`git diff --check`。
4. MCP 浏览器验证已按要求尝试打开 `http://192.168.0.202:3003/`，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮采用同一 3003 服务的真实 HTTP 请求兜底验证。
5. 3003 兜底验证通过：`/` 返回 200，`/api/status` 返回 200。该切片只新增后端测试，不改变页面或接口行为。

## 本轮实施评审：常量包边界文档

### 需求分析

new-api-main 在 `constant/README.md` 中明确了 `constant` 包的职责边界：该目录只放全局可复用常量定义，不承载业务流程、数据库操作、第三方服务调用，也不引用项目内其他自定义包。NexusTok 当前 `constant/` 已经承载 API 类型、渠道类型、上下文键、端点类型、环境变量、任务、Waffo 支付方式等基础常量，但缺少目录级说明。后续继续从 new-api-main 同步 channel、endpoint、account pool 和 relay 能力时，如果没有边界文档，容易把设置解析、provider 适配逻辑或缓存逻辑塞入 `constant`，增加包耦合。

本轮目标是把 new-api-main 的常量包约定转为 NexusTok 原生文档能力：

1. 新增 `constant/README.md`，说明 `constant` 只承载常量、轻量类型和常量映射。
2. 按 NexusTok 当前文件列表更新“当前文件”表格，避免照搬 new-api-main 已不存在或尚未引入的文件。
3. 明确使用约定：禁止引入项目内自定义包，禁止放业务流程/数据库/第三方调用逻辑，新增文件需同步 README。
4. 该切片只新增文档，不改变运行时代码和接口。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 常量包文档 | `constant/README.md` | 新增目录职责、当前文件说明和使用约定。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险、方案、验收方式和验证记录。 |

### 风险评估

1. 本轮只新增 Markdown 文档，不影响编译、接口、数据库或前端页面，运行时风险极低。
2. README 表格必须与 NexusTok 当前 `constant/*.go` 文件对应；照搬 new-api-main 的 `channel_setting.go`、`user_setting.go` 等不存在文件会造成误导。
3. `constant/azure.go` 当前允许标准库 `time`，README 需说明“项目内自定义包禁止引用”，而不是绝对禁止任何 import。
4. 文档约定不会自动阻止错误依赖，后续如果需要可再增加静态检查；本轮只做项目维护指引。
5. 按用户要求仍需访问 3003 `/` 和 `/api/status`，确认热更新服务可达；MCP 如仍不可用则记录并使用 HTTP 兜底。

### 方案评审

采用“按当前文件列表改写 README”的方案，不创建脚本、不加入 CI 规则、不移动常量。README 保留 new-api-main 的核心边界约束，同时补充 NexusTok 自有文件如 `api_type.go`、`channel.go`、`channel_credential_mode.go`、`endpoint_type.go`、`multi_key_mode.go`、`waffo_pay_method.go`。这样可以低风险吸收其维护优势，并给后续差异同步提供清晰落点。

验收方式：

1. `go test ./constant`，确认新增文档不影响常量包编译。
2. `git diff --check`。
3. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/` 和 `/api/status` 返回 200。

验证记录：

1. 常量包编译通过：`go test ./constant`（无测试文件）。
2. 补丁空白检查通过：`git diff --check`。
3. MCP 浏览器验证已按要求尝试打开 `http://192.168.0.202:3003/`，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮采用同一 3003 服务的真实 HTTP 请求兜底验证。
4. 3003 兜底验证通过：`/` 返回 200，`/api/status` 返回 200。该切片只新增文档，不改变页面或接口行为。

## 本轮实施评审：SMTP STARTTLS 与 TLS 校验配置原生化

### 需求分析

new-api-main 已经把 SMTP 连接层拆成可测试的 `newSMTPClient()`，支持显式 STARTTLS、隐式 TLS 兼容、TLS 证书校验策略和空凭证跳过认证。NexusTok 当前虽然已有更适合企业 Exchange 场景的 NTLM 自适应认证，但连接层仍存在三类短板：

1. 端口 587 等普通 SMTP 连接完全依赖 `smtp.SendMail` 的默认行为，无法由管理员明确要求 STARTTLS，也无法在服务端不支持 STARTTLS 时给出确定错误。
2. 端口 465 或 `SMTPSSLEnabled=true` 时固定 `InsecureSkipVerify: true`，默认跳过 TLS 证书校验，安全基线低于 new-api-main。
3. SMTP 账号或令牌为空时仍会尝试构造并发送认证流程，无法兼容匿名内网 SMTP relay 或只需要 envelope sender 的企业网关。

本轮目标是吸收 new-api-main 的连接层优势，并转成 NexusTok 原生能力：

1. 新增 `SMTPStartTLSEnabled` 配置，管理员可以显式要求 STARTTLS；即使端口设置为 465，只要启用该开关也按 STARTTLS 方式连接。
2. 新增 `SMTPInsecureSkipVerify` 配置，默认进行证书校验；只有管理员明确开启时才跳过校验，用于自签名或内网 SMTP 兼容。
3. 抽出 `newSMTPClient()` 和 `smtpTLSConfig()`，让隐式 TLS、显式 STARTTLS 和普通明文连接路径可测试、可维护。
4. 新增 `shouldAuthenticateSMTP()`，仅当账号与令牌都非空时才执行 `AUTH`，保留现有 `AutoSMTPAuth()`、LOGIN 强制和 NTLM 自适应选择。
5. 在默认前端 SMTP 设置页补充两个开关，并把配置纳入 `OperationsSettings`、默认值、section registry 和六语言 i18n。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| SMTP 全局配置 | `common/constants.go`、`common/init.go` | 新增 STARTTLS 与 TLS 证书跳过校验变量，支持环境变量覆盖。 |
| 邮件发送链路 | `common/email.go` | 改造连接创建、TLS 配置和认证触发条件；保留现有邮件头、Message-ID、LOGIN/NTLM 认证策略和发送流程。 |
| SMTP 回归测试 | `common/email_test.go` | 新增 fake SMTP server 覆盖 STARTTLS、证书校验、隐式 TLS 465、空凭证跳过认证和 NTLM 发送路径。 |
| 系统 option | `model/option.go` | 新增两个 option 默认值和运行时更新分支，保持三库兼容，不引入迁移。 |
| 默认前端设置页 | `web/default/src/features/system-settings/{types.ts,operations/index.tsx,operations/section-registry.tsx,integrations/email-settings-section.tsx}` | SMTP Email 分区新增 STARTTLS 与 TLS 校验兼容开关，继续复用现有 option 保存权限与逐项更新链路。 |
| 前端国际化 | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 为新增 UI 文案补齐六语言翻译，新增文案均为直接 `t()` 调用，无需补充静态 key。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. TLS 默认行为会从“隐式 TLS 一律跳过证书校验”提升为“默认校验证书”。这能提高安全性，但使用自签名证书或内网域名不匹配证书的管理员需要显式开启 `SMTPInsecureSkipVerify` 才能继续发送邮件。
2. `SMTPPort=465` 的历史行为必须保留：未开启 `SMTPStartTLSEnabled` 时仍走隐式 TLS，避免破坏传统 SMTPS 配置；开启 STARTTLS 时才按显式升级连接处理。
3. 明文连接下继续调用 `smtp.PlainAuth` 可能被 Go 标准库拒绝远程明文认证，本轮不绕过该安全保护；管理员需要启用 STARTTLS/SSL 或使用本地 relay。
4. 空账号或空令牌跳过 AUTH 会改变旧的失败路径，但这是为了兼容匿名 SMTP relay；只要账号和令牌齐全，现有 LOGIN/NTLM/PLAIN 自适应语义不变。
5. 新增 option 字段不需要数据库迁移，但需要确认 `InitOptionMap()`、`UpdateOption()`、前端 `getOptionValue()` 默认值一致，避免设置页出现 `undefined` 开关。
6. 前端新增文案必须补齐六语言并运行 `bun run i18n:sync`，否则 i18n 同步报告会出现 missing key。
7. 按用户要求，完成后必须优先使用 MCP 访问 3003 设置页和相关接口；若 MCP 仍因 Chrome DevTools 连接失败不可用，需要记录错误并使用 3003 真实 HTTP 请求、登录态 option 接口和前端资源 hash 兜底验证。

### 方案评审

采用“连接层原生化 + 配置透出 + 回归测试保护”的低耦合方案：后端先补两个配置变量和环境变量解析，再把 `SendEmail()` 的连接创建迁移到 `newSMTPClient()`，由 `SMTPSSLEnabled`、`SMTPPort` 和 `SMTPStartTLSEnabled` 共同决定隐式 TLS、显式 STARTTLS 或普通 SMTP。TLS 配置统一走 `smtpTLSConfig()`，默认校验证书，管理员兼容场景通过 `SMTPInsecureSkipVerify` 显式放开。认证层只增加空凭证跳过判断，保留 `AutoSMTPAuth()` 已有 NTLM 自适应和 `SMTPForceAuthLogin` 语义。

前端只在已有 SMTP Email 分区新增两个 Switch，不新增页面、不改变路由、不改变保存 hook 权限模型。配置仍逐项提交到 `/api/option/`，与其他系统设置保持一致。

验收方式：

1. `go test ./common -run 'TestSendEmail|TestNewSMTPClient|TestSMTPPlainAuth'`。
2. `go test ./common`。
3. `go test ./model -run 'TestUpdateOptionsBulk'`，再执行 `go test ./model`。
4. `cd web/default && bun run i18n:sync`。
5. `cd web/default && bunx eslint src/features/system-settings/integrations/email-settings-section.tsx src/features/system-settings/operations/index.tsx src/features/system-settings/operations/section-registry.tsx src/features/system-settings/types.ts`。
6. `cd web/default && bun run typecheck`。
7. `cd web/default && bun run build`。
8. `git diff --check`。
9. 优先用 MCP 打开 `http://192.168.0.202:3003/system-settings/operations/email` 并检查设置页、控制台错误和 option 接口；如 MCP 不可用，则用 `curl --noproxy '*'` 登录 3003，验证 `/`、`/api/status`、登录态 `/api/option/` 返回包含 `SMTPStartTLSEnabled` 与 `SMTPInsecureSkipVerify`，并比对线上静态资源是否包含新增文案。

验证记录：

1. SMTP 定向测试通过：`go test ./common -run 'TestSendEmail|TestNewSMTPClient|TestSMTPPlainAuth'`。覆盖显式 STARTTLS、服务端不支持 STARTTLS、STARTTLS 关闭时不自动升级、远程明文 PLAIN AUTH 被拒绝、465 端口显式 STARTTLS、465 端口隐式 TLS 兼容、空凭证/不完整凭证跳过 AUTH、NTLM 发送路径和默认拒绝不可信证书。
2. `common` 包测试通过：`go test ./common`。
3. option 保存测试通过：`go test ./model -run 'TestUpdateOptionsBulk'`，新增断言覆盖 `SMTPStartTLSEnabled` 与 `SMTPInsecureSkipVerify` 保存后同步刷新 `common.OptionMap` 和运行时全局变量。
4. `model` 包全量测试通过：`go test ./model`。
5. i18n 同步通过：`cd web/default && bun run i18n:sync`；同步报告显示 en/fr/ja/ru/vi/zh 的 `missingCount=0`、`extrasCount=0`，现有 untranslatedCount 为历史存量。
6. 前端触碰文件定向 ESLint 通过：`cd web/default && bunx eslint src/features/system-settings/integrations/email-settings-section.tsx src/features/system-settings/operations/index.tsx src/features/system-settings/operations/section-registry.tsx src/features/system-settings/types.ts`。
7. 前端类型检查通过：`cd web/default && bun run typecheck`。
8. 前端生产构建通过：`cd web/default && bun run build`，总量 `24265.2 kB / 9215.8 kB gzip`。
9. 补丁空白检查通过：`git diff --check`。
10. MCP 浏览器验证已按要求尝试打开 `http://192.168.0.202:3003/system-settings/operations/email`，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮采用同一 3003 服务的真实 HTTP 请求和资源一致性兜底验证。
11. 3003 页面与基础接口验证通过：`/` 返回 200，`/system-settings/operations/email` 返回 200，`/api/status` 返回 200 且 `success=true`。
12. 3003 登录态 option 验证通过：使用账号 `c1cada` 登录 `/api/user/login?turnstile=` 返回 `success=true`，随后携带 session cookie 与 `NexusTok-User: 1` 访问 `/api/option/` 返回 `success=true`、`total=227`，其中 `SMTPStartTLSEnabled=false`、`SMTPInsecureSkipVerify=false`、`SMTPSSLEnabled=false`、`SMTPForceAuthLogin=false`。
13. 3003 前端资源验证通过：线上 `/static/js/index.js` 与本地 `web/default/dist/static/js/index.js` SHA256 均为 `a610251c5d4a53d7487ece3884e798a8885dac487bc159cb94d29fcf0fd9f6a8`，线上 `/static/js/async/1043.js` 与本地 `web/default/dist/static/js/async/1043.js` SHA256 均为 `d7c6a70f6b24c7b34548520c0767fd189a9c50f75a170f094f495787a9a8799a`；线上资源包含 `SMTPStartTLSEnabled`、`SMTPInsecureSkipVerify`、`Use STARTTLS`、`Skip SMTP TLS certificate verification`、`Require STARTTLS upgrade for non-SSL SMTP connections` 和 `Allow self-signed or mismatched SMTP certificates`，确认热更新页面加载的是本轮构建产物。

## 本轮实施评审：计费表达式结算 clamp 回归测试

### 需求分析

按照项目规则，本轮涉及计费表达式系统前已完整阅读 `pkg/billingexpr/expr.md`。new-api-main 在 `pkg/billingexpr/settle_clamp_test.go` 中补充了一个专门的安全回归测试文件，用于锁定两条计费结算不变量：

1. 分层/动态计费表达式在实际结算时如果输出异常放大，最终配额必须通过 `QuotaRoundChecked` 饱和到 int32 上限，不能 wraparound 成负数或用户退款。
2. 正常范围内的表达式结算不能误报 `Clamp`，避免日志和审计路径在常规请求中产生噪音。

NexusTok 当前 `pkg/billingexpr/settle.go` 已经与 new-api-main 等价，使用 `common.QuotaRoundChecked(quotaBeforeGroup * snap.GroupRatio)` 并把 `Clamp` 放入 `TieredResult`；`pkg/billingexpr/billingexpr_test.go` 也已有一个超大表达式 clamp 测试。但缺少独立的 settle clamp 测试资产，尤其缺少“正常范围 `Clamp == nil`”这一条反向断言。后续调整 `quotaConversion()`、`GroupRatio` 处理、`QuotaRoundChecked` 或 `TieredResult` 字段时，可能只保留溢出路径而误伤普通结算。

本轮目标是把 new-api-main 的测试优势转为 NexusTok 原生回归保护：

1. 新增 `pkg/billingexpr/settle_clamp_test.go`，用中文注释说明计费安全不变量。
2. 覆盖异常超大结算饱和到 `math.MaxInt32`，并断言 `Clamp.Kind`、`Clamp.Clamped`。
3. 覆盖正常范围结算 `Clamp == nil`，并断言普通计费结果保持稳定。
4. 不修改计费表达式运行时逻辑，不改数据库、接口、前端或配置。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 计费表达式测试 | `pkg/billingexpr/settle_clamp_test.go` | 新增结算饱和保护与正常路径无 clamp 的单元测试。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 本轮只新增测试，不改变生产代码、数据库 schema、接口和前端页面，运行时风险极低。
2. 测试必须使用 NexusTok 的 `common.MaxQuota` / `common.QuotaClampOverflow` 常量，避免硬编码和当前公共饱和策略脱节。
3. 超大表达式应选择可读、稳定且足够超过 int32 上限的输入，避免依赖平台相关的 int 溢出行为；断言应落在 `QuotaRoundChecked` 的饱和输出上。
4. 正常路径测试需要覆盖 `Clamp == nil`，防止未来把普通四舍五入也误标记成审计事件。
5. 该切片不涉及页面和接口，但按用户要求仍需访问 3003 `/` 和 `/api/status`；MCP 如仍不可用则记录并使用 HTTP 兜底。

### 方案评审

采用“测试资产原生化”的方案：保留现有 `ComputeTieredQuota()` 实现不动，只新增一个专门测试文件承载 new-api-main 的 clamp 语义。超大结算测试使用 `tier("base", p * 1000000000)` 和 `P=1_000_000_000`，确保表达式输出、quota 换算和分组后金额远超 int32 上限；正常路径测试使用 `tier("base", p * 2 + c * 10)`，断言 `Clamp` 为 nil。这样既能补齐 new-api-main 的明确测试优势，又不会在计费热路径上引入额外改动。

验收方式：

1. `go test ./pkg/billingexpr -run 'TestComputeTieredQuota_(ClampOnOverflow|NoClampInRange)'`。
2. `go test ./pkg/billingexpr`。
3. `git diff --check`。
4. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/` 和 `/api/status` 返回 200。

验证记录：

1. 定向测试通过：`go test ./pkg/billingexpr -run 'TestComputeTieredQuota_(ClampOnOverflow|NoClampInRange)'`。
2. 计费表达式包测试通过：`go test ./pkg/billingexpr`。
3. 补丁空白检查通过：`git diff --check`。
4. MCP 浏览器验证已按要求尝试打开 `http://192.168.0.202:3003/`，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮采用同一 3003 服务的真实 HTTP 请求兜底验证。
5. 3003 兜底验证通过：`/` 返回 200，`/api/status` 返回 200 且 `success=true`。该切片只新增后端测试，不改变页面或接口行为。

## 本轮实施评审：用户更新并发字段保护与邮箱唯一性原生化

### 需求分析

`new-api-main` 的 `model/user_update_test.go` 针对用户模型补充了一组安全回归测试，覆盖用户资料更新、个人设置保存、邮箱规范化、无密码用户和邮箱找回密码等核心路径。NexusTok 当前已经有更丰富的用户管理、Authz、Passkey、OAuth 和默认前端设置能力，但用户模型仍存在几处可以吸收 new-api-main 优势的风险点：

1. `User.Update(false)` 会先复制调用方传入的旧用户对象，再执行全字段 `Updates`。如果该对象是在扣费、消费日志或并发请求前读取的旧快照，保存显示名、邮箱或设置时可能把最新 `quota`、`used_quota`、`request_count` 覆盖回旧值。
2. `controller.UpdateUserSetting` 当前通过 `user.SetSetting(settings)` 后调用 `user.Update(false)`，个人设置保存理论上也会经过上述全字段更新路径，可能误覆盖并发计费字段。
3. 邮箱判断仍使用精确匹配：`CheckUserExistOrDeleted`、`IsEmailAlreadyTaken`、注册和邮箱绑定无法稳定防住 `Taken@Example.com` 与 `taken@example.com` 这种大小写重复。
4. 密码重置按 `email = ?` 批量更新，历史数据如果已有大小写重复邮箱，接口可能更新 0 个或多个账号，缺少“唯一活跃匹配”的保护。
5. OAuth/passwordless 用户允许空密码插入是合理能力，但密码登录必须明确拒绝空密码账号，避免未来密码校验函数行为变化导致无密码账号被“补登录”。
6. `model/user.go` 当前直接使用 `encoding/json` 做设置序列化，触碰该文件时必须同步切换到 `common.Marshal` / `common.Unmarshal`，符合项目 JSON 封装规则。

本轮目标是把 new-api-main 的用户安全回归能力转成 NexusTok 原生能力：模型层提供邮箱规范化、邮箱唯一性查询、只更新设置列、安全 `UpdateWithTx` 和唯一邮箱密码重置；控制器层将个人设置保存、邮箱绑定、邮箱验证码和密码重置接入这些模型能力；同时补齐回归测试，锁定余额字段不被旧对象覆盖。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 用户模型 | `model/user.go`、`model/errors.go` | 新增邮箱规范化/唯一性 helper、`UpdateUserSetting`、`UpdateWithTx`，`Update` 避免覆盖计费字段，密码重置要求唯一邮箱匹配，用户设置 JSON 改用 `common` 封装。 |
| 用户缓存 | `model/user_cache.go` | 复用现有 `updateUserSettingCache`，设置保存只刷新设置缓存字段，不重写余额缓存。 |
| 用户控制器 | `controller/user.go`、`controller/misc.go` | 注册、邮箱绑定、验证码发送、密码重置和个人设置保存接入规范化邮箱与模型层安全入口。 |
| 回归测试 | `model/user_update_test.go` | 原生化 new-api-main 的用户更新安全测试，覆盖并发字段保护、邮箱大小写唯一性、passwordless 登录拒绝和邮箱重置歧义。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 用户模型是核心路径，影响密码注册、OAuth 注册、邮箱绑定、个人设置保存、管理员用户编辑、密码登录和找回密码。实现必须保持原接口响应结构、会话行为和缓存策略不变。
2. 邮箱规范化会把新写入邮箱保存为 `strings.ToLower(strings.TrimSpace(email))`。这会牺牲邮箱原始大小写展示，但换来登录、注册、绑定和找回密码的一致语义；历史大小写重复数据不会自动合并，密码重置遇到重复时返回歧义错误并拒绝更新。
3. `Update` 避免覆盖 `quota`、`used_quota`、`request_count` 后，调用方如果确实要调整额度，必须继续使用 `IncreaseUserQuota`、`DecreaseUserQuota`、管理员额度接口或明确的 `DB.Model(...).Update("quota", ...)`。这是更安全的边界，但需要测试覆盖避免误伤已有资料编辑。
4. `UpdateUserSetting` 只更新 `setting` 列，缓存只更新设置字段。这样能避免并发余额覆盖，但如果未来某个调用方期望保存设置时顺带保存其他用户字段，应改为显式字段更新，不应依赖副作用。
5. 三库兼容方面，本轮只使用 GORM、`LOWER(email) = ?`、事务和普通 `UPDATE`，不引入 PostgreSQL advisory lock 或 MySQL `FOR UPDATE` email gap lock；这能降低跨库风险。极端并发同邮箱写入仍建议后续用数据库索引或专门锁切片加固。
6. 密码重置会从“邮箱存在即发送重置邮件”改为“只有唯一匹配时发送邮件，重复或数据库异常只记录日志并对外保持成功”，避免泄露账号存在性和避免重复账号误重置。
7. `controller/misc.go` 触碰 JSON 解码时应改用 `common.DecodeJson`，并移除不再需要的 `encoding/json` import，防止继续扩大项目规范债务。

### 方案评审

采用“模型层收敛、控制器最小接入、测试先锁语义”的方案：

1. 在 `model/errors.go` 增加 `ErrEmailAlreadyTaken`、`ErrEmailNotFound`、`ErrEmailAmbiguous`，让控制器和测试能用 `errors.Is` 区分邮箱占用、缺失和歧义。
2. 在 `model/user.go` 增加 `NormalizeEmail`、`emailQuery`、`CountUsersByEmail`、`IsEmailAvailable`、`EnsureEmailAvailable`、`ensureEmailAvailableWithTx`、`GetUniqueUserByEmail`。这些 helper 统一使用规范化邮箱和大小写不敏感匹配。
3. 提取 `prepareForInsert`，`Insert` 和 `InsertWithTx` 在写入前统一规范化邮箱、检查邮箱可用、仅在密码非空时哈希密码；无密码 OAuth 用户继续允许空密码。
4. 新增 `UpdateUserSetting(userId, setting)`，只 marshal 设置 JSON、更新 `setting` 列并刷新设置缓存；`controller.UpdateUserSetting` 改用该函数。
5. 新增 `UpdateWithTx(tx, updatePassword)`，`Update` 调用它并继续刷新完整用户缓存；更新时 `Omit("quota", "used_quota", "request_count")`，完成后重新读取当前用户，确保调用方对象反映数据库最新余额字段。
6. `ValidateAndFill` 在验证密码前明确拒绝数据库中空密码的用户；这不会影响 OAuth 登录路径，只影响密码登录。
7. `ResetUserPasswordByEmail` 改为先 `GetUniqueUserByEmail`，只更新唯一用户 ID；重复邮箱和未找到邮箱返回明确错误。
8. `SendEmailVerification`、`SendPasswordResetEmail`、`ResetPassword`、`Register`、`EmailBind` 接入 `NormalizeEmail` / `EnsureEmailAvailable` / `GetUniqueUserByEmail`，并保持对外错误尽量与当前行为兼容。
9. 原生化 `model/user_update_test.go`，按 NexusTok 包路径和现有测试基础设施调整；测试不依赖外部 Redis、邮件服务或真实数据库。

验收方式：

1. `go test ./model -run 'Test(UserUpdate|UpdateUserSetting|EnsureEmail|Insert|ValidateAndFill|ResetUserPassword)'`。
2. `go test ./model`。
3. 如控制器改动影响编译，运行 `go test ./controller -run 'Test.*User|Test.*Reset|Test.*Email'`，无匹配时至少运行相关包编译测试。
4. `git diff --check`。
5. 优先用 MCP 打开 `http://192.168.0.202:3003/` 并验证低风险页面/接口；如 MCP Chrome 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status`、登录、`/api/user/self`，并在登录态下调用一个低风险用户设置保存请求确认接口仍能返回业务响应。

验证记录：

1. 定向用户模型测试通过：`go test ./model -run 'Test(UserUpdate|UpdateUserSetting|EnsureEmail|Insert|ValidateAndFill|ResetUserPassword)'`。
2. 模型包完整测试通过：`go test ./model`。
3. 控制器包测试通过：`go test ./controller`。
4. 用户/订阅相关定向编译测试通过：`go test ./model ./controller -run 'Test.*Subscription|Test.*User|Test.*Reset|Test.*Email'`。
5. 补丁空白检查通过：`git diff --check`。
6. 仓库级 `go test ./...` 已执行，但被既有 `relay/channel/claude/message_delta_usage_patch_test.go` 编译问题阻断：该测试文件缺少 `testing`、`dto`、`require`、`gjson` 等 import，失败与本轮用户模型改动无关。本轮先记录该验证阻断，并将 Claude 测试导入修复作为独立验证修复提交处理。
7. MCP 浏览器验证已按要求尝试连接 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮采用同一 3003 服务的真实 HTTP 请求兜底验证。
8. 3003 页面与状态接口验证通过：`/` 返回 200，`/api/status` 返回 200。
9. 3003 登录态接口验证通过：使用账号 `c1cada` 登录 `/api/user/login?turnstile=` 返回 `success=true`，随后携带 session cookie 与 `NexusTok-User: 1` 访问 `/api/user/self` 返回 200。
10. 3003 设置写入口低风险验证通过：对当前账号调用 `PUT /api/user/self` 写入原有 `language=zh` 返回 200 和 `success=true`，再次读取 `/api/user/self` 后 `setting` 仍为 `{"gotify_priority":0,"language":"zh"}`，确认用户设置控制器路径可用且未改变真实偏好值。

## 本轮附带验证修复：Claude message_delta usage patch 测试导入修复

### 需求分析

在验证“用户更新并发字段保护与邮箱唯一性”切片时，仓库级 `go test ./...` 被 `relay/channel/claude/message_delta_usage_patch_test.go` 的编译错误阻断。该测试文件已经包含完整测试逻辑和中文注释，但文件顶部只有导入分组占位注释，缺少实际 import，导致 `testing`、`dto`、`relaycommon`、`model_setting`、`assert`、`require`、`gjson` 等符号无法解析。

本轮附带修复目标很窄：只补齐该测试文件的 import 列表，让 Claude message_delta usage patch 回归测试能参与仓库级验证。不修改 Claude 渠道生产逻辑、不调整测试断言、不改变 relay 行为。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Claude 渠道测试 | `relay/channel/claude/message_delta_usage_patch_test.go` | 补齐标准库、第三方库和项目内部包 imports，恢复测试编译。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录该验证修复的需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 该修复只修改 `_test.go` import 列表，不影响生产二进制、接口、数据库、前端或运行时逻辑。
2. import 必须与测试实际引用一致，避免引入未使用依赖导致新的编译失败。
3. 修复后需要至少运行 `go test ./relay/channel/claude`，并重新运行 `go test ./...`，确认此前仓库级验证阻断已解除。
4. 该切片不涉及页面和接口，但按用户要求仍需访问 3003 `/` 与 `/api/status`；MCP 如仍不可用则继续记录并使用 HTTP 兜底。

### 方案评审

采用最小修复方案：保留原测试内容不动，只把占位导入注释替换为真实 import 块。标准库导入 `testing`，第三方导入 `gjson`、`assert`、`require`，项目内部导入 `dto`、`relay/common` 和 `setting/model_setting`。完成后运行 Claude 包测试和全仓库 Go 测试，确认用户切片之外的既有编译阻断已清除。

验收方式：

1. `go test ./relay/channel/claude`。
2. `go test ./...`。
3. `git diff --check`。
4. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/` 和 `/api/status` 返回 200。

验证记录：

1. 补齐 import 后，`relay/channel/claude/message_delta_usage_patch_test.go` 的 `testing`、`dto`、`relaycommon`、`model_setting`、`assert`、`require`、`gjson` 符号均可解析。
2. `go test ./relay/channel/claude` 不再因该文件编译失败退出，但继续暴露出既有 OpenAI `file` 内容转 Claude 内容块的行为断言失败；该失败已拆分为下一小节的生产转换修复。

## 本轮附带验证修复：Claude OpenAI 文件内容转换语义修复

### 需求分析

补齐 `message_delta_usage_patch_test.go` 导入后，`go test ./relay/channel/claude` 继续暴露出 3 个既有失败，集中在 `RequestOpenAI2ClaudeMessage` 的 OpenAI 复合内容 `file` 转 Claude 内容块路径：

1. `.bin` 等未知文件扩展名当前会被当成 `image` 内容块发送给 Claude，测试期望忽略未知二进制，只保留文本内容。
2. `.pdf` 文件当前因为 `MediaContent.ToFileSource()` 丢失 `filename`，最小 PDF 片段 MIME 回退为 `application/octet-stream`，最终被错误转成 `image`；测试期望按文件名转换为 Claude `document`。
3. `.txt` 文件当前同样被转成 `image`，测试期望 base64 解码为 Claude `text` 内容块。

这不是导入问题，而是 Claude 文件转换能力缺口。修复目标是把已存在的测试语义变成真实生产行为：OpenAI `file` 内容进入 Claude 时只支持 Claude 能理解的文本、图片和 PDF；未知二进制跳过，不构造无效 `image` 内容块。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Claude 请求转换 | `relay/channel/claude/relay-claude.go` | 在复合内容转换中专门处理 `ContentTypeFile`：按文件名扩展名识别 text/pdf/image，未知类型跳过。 |
| Claude 渠道测试 | `relay/channel/claude/relay_claude_test.go`、`relay/channel/claude/message_delta_usage_patch_test.go` | 复用既有失败测试作为验收，不新增或放宽断言。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录该行为修复的需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 该修复触碰 relay 生产转换路径，影响 OpenAI 兼容请求转 Claude 上游请求的多模态内容。必须严格限制在 `file` 类型和 MIME 判定，不改变文本、tool、system、assistant tool_calls 等路径。
2. 当前旧行为会把 unknown/audio/video/octet-stream 文件也包装成 Claude `image`，这很可能是错误请求。修复后这些类型被跳过，可能让依赖错误行为的请求少一个无效内容块，但能避免上游 400 和错误媒体类型。
3. PDF 和图片通过 `service.GetBase64Data` 继续复用现有文件缓存、data URL 清理和 base64 处理；文本文件只在 `text/*` 扩展名时解码为字符串，避免误把任意二进制转文本。
4. 依赖文件名扩展名识别内联 `file_data` 的 MIME，是因为当前 `MessageFile` 没有独立 MIME 字段。后续如 DTO 增加 `mime_type`，应优先使用显式 MIME。
5. 修改后需要运行 Claude 包测试和仓库级测试，确认此前失败的断言恢复，且没有引入新的 relay 编译问题。

### 方案评审

采用局部转换 helper 方案：在 `relay/channel/claude/relay-claude.go` 增加 `convertOpenAIFileContentForClaude()`，只服务 `RequestOpenAI2ClaudeMessage` 内的 `ContentTypeFile`。helper 先通过 `service.GetMimeTypeByExtension()` 按 `filename` 推断 MIME：

1. `text/*`：清理可能存在的 data URL 前缀，base64 解码为字符串，构造 Claude `text` 内容块。
2. `application/pdf`：用带 MIME 的 `types.FileSource` 进入 `service.GetBase64Data()`，构造 Claude `document` 内容块。
3. `image/*`：同样复用 `service.GetBase64Data()`，构造 Claude `image` 内容块。
4. `application/octet-stream` 或其他类型：返回 `ok=false`，调用方跳过，不向 Claude 发送未知二进制。

同时在原默认媒体转换路径中只允许 `application/pdf` 和 `image/*` 被追加，防止 audio/video/unknown 继续被误包装成 image。

验收方式：

1. `go test ./relay/channel/claude`。
2. `go test ./...`。
3. `git diff --check`。
4. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/` 和 `/api/status` 返回 200。

验证记录：

1. Claude 包测试通过：`go test ./relay/channel/claude`。
2. 仓库级 Go 测试通过：`go test ./...`。
3. 补丁空白检查通过：`git diff --check`。
4. MCP 浏览器验证已按要求再次尝试连接 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮继续采用同一 3003 服务的真实 HTTP 请求兜底验证。
5. 3003 兜底验证通过：`/` 返回 200，`/api/status` 返回 200 且 `success=true`。该切片只修改后端 relay 转换和测试，不改变前端页面。

## 本轮实施评审：Responses 请求转 Chat 兼容测试原生化

### 需求分析

`new-api-main` 在 `service/relayconvert/responses_request_to_chat_test.go` 中覆盖了 OpenAI Responses 请求降级为 Chat Completions 请求的多个边界场景。NexusTok 已经把该能力原生化到 `service/openaicompat`，且生产实现与 new-api-main 的 `relayconvert` 基本同构；但当前 NexusTok 的 `responses_request_to_chat_test.go` 测试粒度偏粗，仍缺少以下 new-api-main 已锁定的回归点：

1. `instructions`、字符串 `input`、`stream_options.include_usage`、`max_output_tokens`、`temperature=0`、`top_p`、`parallel_tool_calls=true`、`prompt_cache_key`、`reasoning.effort`、`user`、`store=false`、`metadata` 等标量字段的组合映射。
2. Responses 多模态 `input` 中 `input_file`、`input_audio`、`input_video` 降级为 Chat 内容块时的类型和 payload 保留。
3. assistant `output_text` 与随后独立的 `function_call` 共存时，Chat assistant 消息既保留文本，又能追加 `tool_calls`。
4. 只有 `function_call` 而没有 preceding assistant message 时，应自动构造 assistant 消息承载工具调用。
5. `tools`、`tool_choice`、`text.format=json_schema` 到 Chat `tools/tool_choice/response_format` 的细节映射。
6. `custom_tool_call` 要同时保留 Chat tool call 的 `custom` 原始 shape 和 `Function.Arguments` 的 freeform 输入。

本轮目标是把 new-api-main 的测试资产迁移到 NexusTok 原生 `service/openaicompat` 包下，作为 Responses 兼容层的回归护栏。优先只补测试；若测试暴露生产行为缺口，再按最小影响修复转换逻辑。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Responses 兼容测试 | `service/openaicompat/responses_request_to_chat_test.go` | 增加更细的 Responses 请求降级 Chat 转换测试，覆盖标量、多模态、工具调用、custom 工具和 response_format。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 本轮预计只新增/扩展测试，不改生产代码，运行时风险低。
2. 测试必须使用 NexusTok 的 `service/openaicompat` 包名和 `github.com/c1cada/NexusTok/...` import，不引入已废弃的 `relayconvert` 目录。
3. 测试断言应匹配 NexusTok DTO 当前结构。例如 Chat 多模态内容可能以 `[]any` 存储，但应通过 `dto.Message.ParseContent()` 验证最终可解析语义，避免只锁定 map 的 Go 运行时形态。
4. 新增断言涉及显式零值和 JSON 原始字段，必须继续遵守项目 JSON 规则，测试 helper 使用 `common.Marshal`。
5. 如果新增测试失败，优先判断是否为测试迁移路径差异还是生产能力缺口；只有确认能力缺口会影响 Responses 兼容语义时才改生产逻辑。
6. 该切片不涉及页面和真实外部上游，但按用户要求仍需尝试 MCP 访问 3003；MCP 不可用时记录错误并使用 `/`、`/api/status` HTTP 兜底。

### 方案评审

采用“测试资产原生化”的方案：在现有 `service/openaicompat/responses_request_to_chat_test.go` 中追加 new-api-main 缺失的细粒度用例，复用现有 `mustRawMessage()` helper，并补充 `assert`/`gjson` 断言工具。测试覆盖保持在转换函数纯逻辑层，不需要启动服务、不触发数据库和外部网络。这样能把 `relayconvert` 的优势转为 NexusTok `openaicompat` 的原生质量门禁，同时避免目录/包名回退。

验收方式：

1. `go test ./service/openaicompat -run 'TestResponsesRequestToChatCompletionsRequest'`。
2. `go test ./service/openaicompat`。
3. `go test ./...`。
4. `git diff --check`。
5. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/` 和 `/api/status` 返回 200。

验证记录：

1. Responses 请求转 Chat 定向测试通过：`go test ./service/openaicompat -run 'TestResponsesRequestToChatCompletionsRequest'`。
2. OpenAI 兼容服务包测试通过：`go test ./service/openaicompat`。
3. 仓库级 Go 测试通过：`go test ./...`。
4. 补丁空白检查通过：`git diff --check`。
5. MCP 浏览器验证已按要求尝试连接 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮继续采用同一 3003 服务的真实 HTTP 请求兜底验证。
6. 3003 兜底验证通过：`/` 返回 200，`/api/status` 返回 200 且 `success=true`。该切片只新增后端纯逻辑测试，不改变前端页面或接口行为。

## 本轮实施评审：Chat 与 Responses relay handler 兼容原生化

### 需求分析

`new-api-main` 在 Chat Completions 与 Responses 双向兼容链路中补齐了三类 NexusTok 当前仍不完整的能力：

1. `chatCompletionsViaResponses` 能区分客户端是否请求 stream 与上游是否返回 `text/event-stream`。当客户端请求非流式、但上游 Responses 只能返回 SSE 时，new-api-main 会把 SSE 事件缓冲并还原为普通 Chat JSON；NexusTok 当前只要上游 `Content-Type` 以 `text/event-stream` 开头就把 `info.IsStream` 置为 true，可能把非流式客户端请求错误响应成 SSE。
2. `OaiResponsesToChatStreamHandler` 复用 service 层状态机处理 Responses SSE，包括 `response.done`、`response.incomplete`、`response.reasoning_text.delta`、`custom_tool_call`、工具参数 delta 早于 output item、terminal response output 兜底等复杂事件；NexusTok 当前 openai channel 仍是手写状态，覆盖面窄于 new-api-main。
3. `OaiChatToResponsesStreamHandler` 能把 Chat Completions SSE 还原为 Responses SSE，供“客户端请求 `/v1/responses`，上游/适配层只返回 Chat SSE”的兼容路径复用。NexusTok 已经有 `service/openaicompat` 的 Chat stream -> Responses event 状态机，并在 Gemini Responses 中使用，但 OpenAI channel 尚未提供对应 handler。

本轮目标是把这些优势转为 NexusTok 原生能力：保留 `service/openaicompat` 作为本项目兼容包名，不引入 `service/relayconvert`；在 service 层补齐 Responses SSE -> Chat chunks 的状态机和 buffered accumulator；在 OpenAI channel 增加 buffered stream handler 与 Chat SSE -> Responses SSE handler；同时补齐 new-api-main 的 relay/service 回归测试。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Responses SSE -> Chat 状态机 | `service/openaicompat/responses_to_chat.go`、`service/openai_chat_responses_compat.go` | 新增 Responses stream event 转 Chat stream chunks、终态 flush、buffered accumulator 和 service 层薄封装。 |
| Chat/Responses relay handler | `relay/chat_completions_via_responses.go`、`relay/channel/openai/chat_via_responses.go` | 区分 client stream 与 upstream stream；非流式客户端遇到上游 SSE 时输出普通 JSON；新增 Chat SSE 转 Responses SSE handler。 |
| 回归测试 | `service/openaicompat/*_test.go`、`relay/chat_completions_via_responses_test.go`、`relay/channel/openai/chat_via_responses_test.go` | 原生化 new-api-main 对 stream 顺序、usage、tool args、buffered JSON 和 Content-Type 判断的测试资产。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验证结果和完成清单。 |

### 风险评估

1. 本轮触碰 relay 响应转换热路径，用户可见行为包括响应是否为 SSE、Chat chunk 顺序、tool_calls、finish_reason 和 usage 帧，必须用定向测试锁定。
2. `clientStream=false && upstreamStream=true` 从历史 SSE 输出改为 JSON 输出，属于兼容性修复：它更符合客户端非流式请求语义，但可能改变依赖旧错误行为的调用方。该分支只在 Chat via Responses 兼容模式且上游返回 SSE 时触发。
3. 状态机必须继续使用 `common.Marshal`、`common.UnmarshalJsonStr`，不能引入直接 `encoding/json` 调用；新增测试同样遵守项目 JSON 规则。
4. 不改数据库、不改计费表达式、不新增前端文案和路由权限，三库兼容风险低；主要风险集中在内存累积上游 SSE。buffered 模式只服务客户端非流式请求，且仅累积当前响应文本、reasoning 与工具调用参数，不持久化。
5. Stream scanner 会关闭上游 body，新增 handler 不应重复读取 body 或丢失 `service.CloseResponseBodyGracefully` 语义；buffered handler 用 scanner 顺序读取并在函数返回时关闭 body。
6. `response.failed`/`response.error` 必须 fail-closed 返回上游错误，不允许静默转成成功 Chat 响应。
7. 该切片不涉及前端页面，但按用户要求仍需尝试 MCP 访问 3003；MCP 不可用时记录错误，并用真实 HTTP 请求验证 `/` 与 `/api/status`。

### 方案评审

采用“service 状态机原生化 + relay handler 最小接入”的方案：

1. 在 `service/openaicompat/responses_to_chat.go` 补齐 `ResponsesToChatStreamState`、`ResponsesStreamEventToChatChunks`、`FinalizeResponsesToChatStream` 和 `ResponsesBufferedAccumulator`，复用现有常量、DTO 和中文注释，保留 NexusTok 已有 usage 细节增强。
2. 在 `service/openai_chat_responses_compat.go` 增加薄封装，让 relay 层仍只依赖 `service` 包，不直接耦合 `openaicompat` 内部实现。
3. 将 `OaiResponsesToChatStreamHandler` 改为消费 service 状态机，统一处理 Responses SSE 事件；新增 `OaiResponsesToChatBufferedStreamHandler`，把上游 SSE 缓冲为 Responses 终态对象后复用非流式 Chat 转换逻辑。
4. 新增 `OaiChatToResponsesStreamHandler`，复用现有 Chat stream -> Responses event 状态机，输出标准 `event:` + `data:` SSE。
5. 修改 `chatCompletionsViaResponses` 的 stream 分支判断：先记录 `clientStream`，再用大小写不敏感的 `isResponsesEventStreamContentType()` 判断 `upstreamStream`；只有双方都是 stream 时走 SSE 转 SSE，只有上游 stream 时走 buffered JSON，否则走普通 JSON。

验收方式：

1. `go test ./service/openaicompat -run 'TestResponsesStreamEventToChatChunks|TestResponsesBufferedAccumulator|TestChatCompletionsStreamToResponsesEvents'`。
2. `go test ./relay -run 'TestIsResponsesEventStreamContentType'`。
3. `go test ./relay/channel/openai -run 'TestOai.*Responses.*Chat|TestOaiChatToResponses'`。
4. `go test ./service/openaicompat ./relay ./relay/channel/openai`。
5. `go test ./...`。
6. `git diff --check`。
7. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/` 和 `/api/status` 返回 200，并登录后至少读取 `/api/user/self`。

验证记录：

1. Responses SSE 状态机定向测试通过：`go test ./service/openaicompat -run 'TestResponsesStreamEventToChatChunks|TestResponsesBufferedAccumulator|TestChatCompletionsStreamToResponsesEvents'`。
2. Chat via Responses Content-Type 判断测试通过：`go test ./relay -run 'TestIsResponsesEventStreamContentType'`。
3. OpenAI Chat/Responses relay handler 定向测试通过：`go test ./relay/channel/openai -run 'TestOai.*Responses.*Chat|TestOaiChatToResponses'`。
4. 相关包整体测试通过：`go test ./service/openaicompat`、`go test ./relay`、`go test ./relay/channel/openai`。
5. 仓库级 Go 测试通过：`go test ./...`。
6. 补丁空白检查通过：`git diff --check`。
7. MCP 浏览器验证已按要求尝试连接 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。因此本轮继续采用同一 3003 服务的真实 HTTP 请求兜底验证。
8. 3003 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。该切片只修改后端 relay 转换和测试，不改变前端页面。

## 本轮实施评审：日志查询与 ClickHouse 准备层护栏

### 需求分析

`new-api-main` 在日志模型中补齐了两类优势：一类是 ClickHouse 作为独立日志库时需要的 DSN 识别、TTL SQL、排序和 LIKE 转义；另一类是即使不启用 ClickHouse，也能让日志查询的文本过滤与请求 ID 更稳定。NexusTok 当前已经有 `common.UsingClickHouse` 标记和独立 `LOG_SQL_DSN` 入口，但尚未真正接入 ClickHouse driver；日志查询中也存在若干可提前吸收的稳健性差异：

1. 主业务库不能误配置为 ClickHouse。ClickHouse 适合日志分析，不适合 NexusTok 的事务型主库；如果 `SQL_DSN` 使用 `clickhouse://`、`tcp://`、`http://` 或 `https://`，应 fail-fast 返回明确错误。
2. 日志文本过滤需要统一处理普通三库和未来 ClickHouse 的 LIKE 转义。当前 `GetUserLogs` 和 `SumUsedQuota` 使用 `sanitizeLikePattern()`，但 `GetAllLogs` 对 `model_name` 仍直接拼 `LIKE ?`，没有统一逃逸路径。
3. 日志记录应尽量补齐 `request_id`，为后续 ClickHouse 排序、跨库日志排障和审计定位提供稳定键。
4. ClickHouse 的日志排序不能依赖自增 `id`，应使用 `created_at, request_id` 的稳定顺序；即使本轮不接入 driver，也可以先提供排序 helper 和测试护栏。
5. ClickHouse TTL 建表 SQL 属于未来可选日志库能力，本轮只实现纯字符串 helper 和测试，不执行真实迁移，不引入 `gorm.io/driver/clickhouse`，避免增加部署依赖和三库测试复杂度。

本轮目标是把 new-api-main 的日志基础设施优势转成 NexusTok 原生准备层：保留当前 SQLite/MySQL/PostgreSQL 主路径，增加 ClickHouse DSN 识别和日志查询护栏；真实 ClickHouse driver、迁移执行和运行时连接仍作为后续独立评审项。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 数据库类型 helper | `common/database.go` | 新增 `DatabaseTypeClickHouse`、日志库类型 getter/setter 和 `UsingLogDatabase()`，兼容现有 `LogSqlType` 与 `UsingClickHouse` 标记。 |
| 数据库初始化 | `model/main.go` | 新增 ClickHouse DSN 识别和主库 fail-fast；保留 `LOG_SQL_DSN` 不支持 ClickHouse 的明确错误，不接入 driver。 |
| 日志模型 | `model/log.go` | 新增日志 LIKE 条件 helper、ClickHouse LIKE 转义、request_id 补齐、ClickHouse 排序/TTL SQL 字符串 helper；让日志查询复用统一文本过滤。 |
| LIKE 校验复用 | `model/token.go` | 将原有 `sanitizeLikePattern()` 内部通配符校验抽为 `validateLikePattern()`，保持令牌搜索语义不变，同时供 ClickHouse 日志 LIKE 清洗复用。 |
| 回归测试 | `model/clickhouse_log_test.go`、相关日志测试 | 原生化 new-api-main 的 ClickHouse 准备层测试，并按 NexusTok 当前“不引入 driver”策略调整连接测试。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 本轮不引入 ClickHouse driver，不改变默认数据库支持矩阵；SQLite、MySQL、PostgreSQL 仍是唯一正式支持的主业务数据库。
2. `SQL_DSN=http://...` 过去可能会被错误当作 MySQL DSN 尝试连接；本轮会 fail-fast 并提示主库不支持 ClickHouse。该行为是配置纠错，不影响合法 MySQL/PostgreSQL/SQLite DSN。
3. `GetAllLogs`、`GetUserLogs` 和统计查询的文本过滤会统一走 `applyExplicitLogTextFilter()`：无 `%` 时等值匹配，有 `%` 时按 LIKE 搜索并转义 `_` 与 `!`。这会让含下划线的模型名不再被误当作单字符通配符，属于更精确的搜索语义。
4. 为日志记录补齐 `request_id` 会让新写入日志多一个稳定标识，不改变表结构；已有空 request_id 不迁移。
5. ClickHouse TTL SQL/helper 只在测试中生成字符串，不执行真实 `ALTER TABLE`，不会影响线上数据库。
6. 该切片不涉及前端页面，但按用户要求仍需尝试 MCP 访问 3003；MCP 不可用时记录错误并用真实 HTTP 请求验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 方案评审

采用“准备层先行、driver 后置”的方案：

1. 在 `common/database.go` 保持现有全局变量兼容，新增 `DatabaseTypeClickHouse` 和日志库类型访问函数，避免大范围重构 `UsingSQLite/UsingMySQL/UsingPostgreSQL`。
2. 在 `model/main.go` 增加 `isClickHouseDSN()` 与 `normalizeClickHouseDSN()`。主库发现 ClickHouse DSN 直接返回错误；日志库发现 ClickHouse DSN 也返回“当前构建未启用 ClickHouse driver”的明确错误，后续真正接入 driver 时再替换此分支。
3. 在 `model/log.go` 新增 `applyExplicitLogTextFilter()`、`buildLogLikeCondition()`、`sanitizeClickHouseLikePattern()`、`ensureLogRequestId()`、`createLog()`、`clickHouseLogOrder()` 与 TTL SQL helper；现有日志写入统一调用 `createLog()`。
4. 将 `GetAllLogs`、`GetUserLogs`、`SumUsedQuota` 的 model/username 文本过滤改为同一 helper，减少三库与未来 ClickHouse 分叉。
5. 增加 `model/clickhouse_log_test.go`，覆盖 DSN 识别、https secure 参数补齐、主库拒绝 ClickHouse、当前构建拒绝 ClickHouse 日志库、TTL SQL 字符串、LIKE 转义、request_id 补齐和展示 ID 分配。

验收方式：

1. `go test ./model -run 'Test(IsClickHouseDSN|NormalizeClickHouseDSN|ChooseDB.*ClickHouse|ClickHouse|BuildLogLikeCondition|EnsureLogRequestId|AssignDisplayLogIds)'`。
2. `go test ./model`。
3. `go test ./...`。
4. `git diff --check`。
5. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `go test ./model -run 'Test(IsClickHouseDSN|NormalizeClickHouseDSN|ChooseDB.*ClickHouse|ClickHouse|BuildLogLikeCondition|EnsureLogRequestId|AssignDisplayLogIds)'` 通过。
2. `go test ./model` 通过。
3. `go test ./...` 通过。
4. `git diff --check` 通过。
5. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
6. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。该切片不涉及前端页面改动，页面访问用于确认当前热部署服务仍可用。

## 本轮实施评审：开发 Compose 与 Makefile 修补

### 需求分析

`new-api-main` 的工程化差异中，`docker-compose.dev.yml` 和 `makefile` 提供了更完整的本地开发闭环：PostgreSQL dev 服务显式声明镜像、后端容器可单独重建、初始化向导状态可快速重置。NexusTok 当前已经有同名 dev compose 和 makefile，但存在几个可直接原生化的问题：

1. `docker-compose.dev.yml` 的 `postgres` 服务没有 `image` 字段。Docker Compose 对没有 `image` 且没有 `build` 的 service 会报配置错误，导致本地 PostgreSQL dev 栈无法稳定启动。
2. `makefile` 只有 `dev-api`，没有只重建后端服务的 `dev-api-rebuild`。Go 依赖或 Dockerfile 变更后，开发者需要手写 compose 命令，和文档中“开发任务可复用”的目标不一致。
3. 当前没有 `reset-setup` 任务。调试初始化向导、root 用户创建和演示模式时，开发者需要手工进入 PostgreSQL 或 SQLite 执行 SQL，容易误删非开发环境数据。
4. NexusTok 没有 `web/package.json` 工作区，不应照搬 new-api-main 的 `cd ./web && bun install --filter ...`；应保留 `web/default`、`web/classic` 的独立 Bun 使用方式。
5. new-api-main 的 `oxlint`、`oxfmt`、`tsgo` 和分支镜像工作流属于更大工具链切换，本轮不引入，避免扰动现有前端格式化、CI 和热更新容器。

本轮目标是把 new-api-main 的开发体验优势转为 NexusTok 原生开发任务：修复 dev compose PostgreSQL 启动缺口，增加后端重建和初始化重置入口，并更新部署文档。该切片不改变生产部署、热更新 `docker-compose.hot.yml`、运行时业务逻辑或前端页面。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Dev Compose | `docker-compose.dev.yml` | 为 `postgres` 服务补齐 `postgres:15-alpine` 镜像和容器名；补充 Secure Session Cookie 的 HTTPS 开发注释，保留 NexusTok 数据库名和服务名。 |
| 开发命令 | `makefile` | 增加可配置变量、`dev-api-rebuild` 和 `reset-setup`，保留 `web/default` 与 `web/classic` 独立 Bun 安装方式。 |
| 部署文档 | `docs/installation/deployment.md` | 补充 `make dev-api-rebuild`、`make reset-setup` 和重置任务的安全边界说明。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. `docker-compose.dev.yml` 只用于本地前后端分离开发，不影响用户要求的 3003 热更新部署，也不修改 `docker-compose.hot.yml`。
2. 补齐 PostgreSQL `image` 是配置修复；不会修改数据库 schema、迁移逻辑或默认生产数据库选择。
3. `reset-setup` 会删除 dev 数据库中的 `setups` 记录、root 用户和两个演示模式 option，因此必须限定在本地 dev compose 或显式指定的本地 SQLite 文件；任务文案和文档会明确这是开发重置，不用于生产。
4. Makefile 的原有目标继续保留，新增目标不改变 `make dev`、`make dev-api`、`make dev-web` 的行为。
5. 本轮不执行 `make reset-setup`，避免改动当前本地 `nexustok.db` 或 Docker dev 数据；只用 `docker compose config` 和 Makefile dry-run 验证命令生成。

### 方案评审

采用“开发工具最小补齐”的方案：

1. 在 `docker-compose.dev.yml` 的 `postgres` 服务增加 `image: postgres:15-alpine` 和稳定 `container_name: nexustok-dev-pg`；继续使用 `nexustok` 服务名、`nexustok` 数据库名、`nexustok-dev` 镜像名和现有网络/卷。
2. 在 `docker-compose.dev.yml` 中保留 Secure Session Cookie 的注释配置，和当前后端 `SESSION_COOKIE_SECURE` / `SESSION_COOKIE_TRUSTED_URL` 能力对齐，但默认不启用。
3. 在 `makefile` 中引入 `DEV_COMPOSE_FILE`、`DEV_API_SERVICE`、`DEV_POSTGRES_SERVICE`、`DEV_POSTGRES_DB`、`DEV_POSTGRES_USER`、`DEV_SQLITE_PATH` 变量；新增 `dev-api-rebuild` 和 `reset-setup`。
4. `reset-setup` 优先检测正在运行的 dev PostgreSQL service，存在时通过 `docker compose exec -T postgres psql ...` 删除初始化状态并重启 dev API；否则回退到本地 SQLite 文件，默认 `nexustok.db`，并允许通过 `SQLITE_PATH` 或 `DEV_SQLITE_PATH` 指定。
5. 更新 `docs/installation/deployment.md` 的本地开发段落，记录新增开发命令和安全说明。

验收方式：

1. `docker compose -f docker-compose.dev.yml config`。
2. `make -n dev-api-rebuild`。
3. `make -n reset-setup`。
4. `git diff --check`。
5. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `docker compose -f docker-compose.dev.yml config` 通过，配置中 `postgres` 服务已解析为 `postgres:15-alpine`，容器名为 `nexustok-dev-pg`。
2. `make -n dev-api-rebuild` 通过，展开命令为 `docker compose -f docker-compose.dev.yml up -d --build nexustok`。
3. `make -n reset-setup` 通过，只做 dry-run，未执行数据库重置；展开逻辑会优先检测 dev PostgreSQL，否则回退到本地 SQLite。
4. `make -n build-frontend`、`make -n build-frontend-classic`、`make -n dev-web`、`make -n dev-web-classic`、`make -n dev-api` 和 `make -n dev` 均通过，确认 Makefile 目录和端口参数符合 NexusTok 当前结构。
5. `git diff --check` 通过。
6. 本轮只修改开发 compose、Makefile 和文档，不涉及 Go/TypeScript 运行时代码，因此未额外执行 `go test ./...` 或前端 build。
7. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
8. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：Electron 桌面端文档与构建脚本修复

### 需求分析

`new-api-main` 的 `electron/README.md` 明确说明了桌面端开发、二进制准备、打包输出和数据目录。NexusTok 当前已经有 `electron/` 包装层、托盘图标、错误日志和 `electron-builder` 配置，但缺少桌面端 README；同时本地代码对照发现两个会直接影响桌面端原生能力的问题：

1. `electron/build.sh` 会执行 `cd ../web && bun run build`。NexusTok 当前没有 `web/package.json`，默认前端和经典前端分别位于 `web/default`、`web/classic`，因此脚本会在第一步失败。
2. Go 主程序通过 `//go:embed web/default/dist` 和 `//go:embed web/classic/dist` 嵌入前端资源。Electron 构建脚本必须先构建这两套前端，再构建 Go 二进制，否则打包出的桌面应用可能嵌入旧资源或缺少 classic 兼容入口。
3. `electron/main.js` 开发模式提示仍写着 `cd web && bun dev`，并把 `DEV_FRONTEND_PORT` 注释为 Vite dev server port；NexusTok 默认前端已经是 Rsbuild，且上一轮 Makefile 将默认前端开发端口统一为 `3001`。
4. `electron/package.json` 的 macOS `extraResources` 仍引用 `../web/dist`。该路径在 NexusTok 不存在，且生产 Electron 会启动内嵌了前端资源的 Go 二进制，不需要再额外挂载 `web/dist`。
5. 桌面端数据目录、开发模式依赖、打包输出、许可证资源和常见问题都需要品牌化成 NexusTok 说明，不能直接复制 new-api-main 文案。

本轮目标是把 new-api-main 的 Electron 文档优势转为 NexusTok 原生维护能力，并顺手修复当前构建脚本与目录结构不匹配的问题。该切片不修改后端业务逻辑、不改变 Web 前端页面、不启动 Electron 打包产物。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Electron 构建脚本 | `electron/build.sh` | 按 NexusTok 目录分别构建 `web/default` 与 `web/classic`，再构建 Go 二进制和平台安装包；注入 `common.Version`。 |
| Electron 主进程开发提示 | `electron/main.js` | 将开发前端端口与 Makefile 对齐为 3001，并修正开发启动提示为 `make dev-api` / `make dev-web` 或 `web/default` 路径。 |
| Electron Builder 配置 | `electron/package.json` | 移除 macOS 对不存在 `../web/dist` 的资源引用，保留二进制和许可证资源。 |
| Electron 文档 | `electron/README.md` | 新增 NexusTok 桌面端开发、构建、数据目录和排障说明。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. `electron/build.sh` 属于桌面端打包脚本，不参与普通后端启动、Docker 热更新或 3003 线上验证链路。
2. 分别构建 default/classic 前端会增加桌面端打包耗时，但符合 Go embed 的真实资源需求，避免打包出缺资源或旧资源的二进制。
3. 移除 `../web/dist` 资源引用只影响 macOS Electron 额外资源；后端二进制已经嵌入 `web/default/dist` 与 `web/classic/dist`，Windows/Linux 配置也没有该资源，语义更一致。
4. 将 Electron 开发前端端口调整为 3001 会要求开发者使用 `make dev-web` 或显式 `bun run dev -- --port 3001`；这与当前开发文档和 Makefile 保持一致。
5. 本轮不执行完整 Electron 打包，避免下载/签名/平台构建带来的环境依赖波动；使用脚本语法、JSON 解析、路径检查和 3003 HTTP 验证作为轻量验收。

### 方案评审

采用“脚本修复 + 文档补齐”的方案：

1. 重写 `electron/build.sh` 的路径处理：以脚本所在目录推导仓库根目录，先读取 `VERSION`，再分别在 `web/default` 和 `web/classic` 中执行 Bun 构建，最后回到根目录构建 `nexustok` 或 `nexustok.exe`。
2. Go build 继续使用 `CGO_ENABLED=1`，并通过 `-X github.com/c1cada/NexusTok/common.Version=...` 注入版本号，保持与 Docker 热更新构建脚本一致。
3. Electron 依赖安装仍在 `electron/` 目录执行 `npm install`，平台构建命令继续使用既有 `npm run build:mac` / `build:linux` / `build:win`。
4. `electron/main.js` 只修正开发模式提示与端口常量，不重构启动逻辑和错误分析逻辑。
5. `electron/README.md` 使用 NexusTok 品牌、路径和命令，明确开发模式与生产模式的数据目录差异，以及许可证资源的打包要求。

验收方式：

1. `bash -n electron/build.sh`。
2. `node -e "JSON.parse(require('fs').readFileSync('electron/package.json', 'utf8')); console.log('ok')"`。
3. `rg -n "cd web &&|../web/dist|Vite dev server" electron`，确认旧路径和旧提示已消除。
4. `git diff --check`。
5. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `bash -n electron/build.sh` 通过。
2. `node -e "JSON.parse(require('fs').readFileSync('electron/package.json', 'utf8')); console.log('ok')"` 通过。
3. `node --check electron/main.js` 与 `node --check electron/preload.js` 通过。
4. `rg -n "cd web &&|../web/dist|Vite dev server|cd ../web|bun dev \\(port 5173\\)|5173" electron` 无匹配，旧路径和旧端口提示已消除。
5. `git diff --check` 通过。
6. 本轮未执行完整 Electron 打包，原因是 `electron-builder` 跨平台打包会下载平台依赖并涉及安装包产物，超出本轮轻量修复验收范围；脚本语法、JSON 配置、Node 语法和路径残留已覆盖当前修复点。
7. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
8. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：DataTable 组件维护文档补齐

### 需求分析

`new-api-main` 的 `web/default/src/components/data-table/README.md` 用很短的篇幅说明了公共 DataTable 包的导入边界和 `core`、`layout`、`toolbar`、`static`、`hooks` 分层职责。NexusTok 当前已经有 `web/default/src/components/data-table/*` 公共组件，并且在渠道、模型、日志、订阅、用户等页面持续复用，但目录仍是扁平结构，缺少 README 说明：

1. 新页面应从 `@/components/data-table` 通过 `index.ts` 导入，还是直接引用内部文件，目前没有明确约定。
2. `DataTablePage`、`DataTableToolbar`、`MobileCardList`、`DataTablePagination`、`DataTableBulkActions` 等组件的职责边界需要文档化，方便后续页面继续复用而不是复制局部表格代码。
3. NexusTok 已经在 `MobileCardList` 中支持 `mobileTitle`、`mobileBadge`、`mobileHidden` 列 meta，但该移动端卡片约定没有集中说明。
4. `new-api-main` 的分层结构值得吸收，但 NexusTok 当前表格调用面很广，直接目录级拆分会造成大范围 import 变更和回归风险。
5. 本轮适合先把文档资产原生化：明确当前稳定 API、组件职责、扩展点和后续分层迁移原则，为将来逐步拆分 `core/layout/toolbar/static/hooks` 做准备。

本轮目标是新增 NexusTok 品牌化的 DataTable README，不修改运行时代码、不改变任何页面行为、不新增依赖。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| DataTable 组件文档 | `web/default/src/components/data-table/README.md` | 新增当前组件目录职责、稳定导入入口、常用组合方式、移动端列 meta 和后续迁移边界。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 本轮只新增/更新 Markdown 文档，不参与前端打包产物和后端运行路径。
2. 文档如果过度承诺未来目录结构，可能误导后续开发者一次性重构；因此明确当前仍以扁平目录和 `index.ts` 公共导出为准。
3. 文档中的示例必须使用 NexusTok 已存在的组件和 prop，不引用 `new-api-main` 当前才有的 `core/*`、`layout/*`、`static/*` 文件，避免形成不存在的 API。
4. README 不能复制 new-api-main 的英文说明，而应结合 NexusTok 当前 `DataTablePage`、`DataTableToolbar` 和 `MobileCardList` 实际能力进行说明。

### 方案评审

采用“文档先行、结构后移”的方案：

1. 新增 `web/default/src/components/data-table/README.md`，说明该目录是默认前端列表页的公共表格工具包，稳定导入入口为 `@/components/data-table`。
2. 按当前文件列出职责：页面包装、工具条、分页、空态、骨架、列头、筛选、列显示、批量操作、移动端列表。
3. 给出 `DataTablePage` 的最小使用示例，覆盖 `table`、`columns`、`isLoading`、`toolbarProps`、`bulkActions` 等高频组合。
4. 单独说明移动端列 meta 的约定，鼓励新列表优先提供 `mobileTitle` 和 `mobileBadge`，避免手机端退化成难读的键值列表。
5. 记录后续吸收 new-api-main 分层的原则：只在真实复用需求出现时逐步拆出 `core/layout/toolbar/static/hooks`，每一步保持 `index.ts` 导出兼容。

验收方式：

1. `sed -n '1,260p' web/default/src/components/data-table/README.md` 人工检查文档内容。
2. `rg -n "@/components/data-table/(core|layout|static|toolbar|hooks)|new-api-main" web/default/src/components/data-table/README.md` 确认没有引用不存在的分层 API 或上游项目名。
3. `git diff --check`。
4. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `sed -n '1,260p' web/default/src/components/data-table/README.md` 已人工检查，README 只描述 NexusTok 当前存在的扁平 DataTable 组件、公开导入入口和移动端列 meta。
2. `rg -n "@/components/data-table/(core|layout|static|toolbar|hooks)|new-api-main" web/default/src/components/data-table/README.md` 无匹配，未引用不存在的分层 import 或上游项目名。
3. `git diff --check` 通过。
4. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
5. 3003 首次 HTTP 探测遇到热更新重启窗口，`192.168.0.202:3003` 短暂返回连接失败；随后确认 `nexustok-api-hot` 容器仍运行，`127.0.0.1:3003/` 返回 200，容器内 `127.0.0.1:3000/api/status` 可用。
6. 3003 指定地址重试验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：Issue 模板前置约束增强

### 需求分析

`new-api-main` 的 Issue 模板在“提交前必读”和提交确认项上更具体，能提前过滤使用、配置、接入、透传上游行为、逆向渠道和 coding plan 等非缺陷/非功能请求类内容。NexusTok 当前已经有中英文 bug/feature 模板和 `config.yml` 联系链接，但确认项相对笼统：

1. “我已确认目前没有类似 issue”没有提供仓库 Issues 搜索链接。
2. “已查看文档和 README”没有明确要求先排除使用、配置或接入问题。
3. Bug 模板没有提示转发/Relay 问题需要附渠道类型、转换格式、上游原生支持依据和服务端日志。
4. Bug 模板没有提示计费问题附 `usage` 示例，后续排查动态计费、钱包兜底和模型倍率时成本较高。
5. Feature 模板没有明确拒绝 coding plan、逆向渠道、绕过上游限制等技术支持类 issue。

本轮目标是把 new-api-main 的模板治理优势转成 NexusTok 原生协作规范，增强四份 Issue 模板；不修改 GitHub workflow，不修改 Issue Chooser `config.yml`，不改变代码运行行为。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 中文 Bug 模板 | `.github/ISSUE_TEMPLATE/bug_report.md` | 增加透传/Relay、技术支持边界、非重复搜索、使用问题排除、转发和计费信息提示。 |
| 英文 Bug 模板 | `.github/ISSUE_TEMPLATE/bug_report_en.md` | 与中文 Bug 模板保持语义等价。 |
| 中文功能请求模板 | `.github/ISSUE_TEMPLATE/feature_request.md` | 增加技术支持边界、非重复搜索和使用/配置/接入问题排除确认。 |
| 英文功能请求模板 | `.github/ISSUE_TEMPLATE/feature_request_en.md` | 与中文功能请求模板保持语义等价。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 模板变更只影响 GitHub 新建 issue 时的默认文本，不参与运行时代码、前端页面或 Docker 热更新链路。
2. 约束写得过硬可能劝退有效反馈，因此保留“尽可能”“when possible”等措辞，只明确拒绝纯技术支持、coding plan、逆向渠道和绕过上游限制请求。
3. NexusTok 与 new-api-main 的项目身份、文档链接和仓库 Issues 地址不同，必须品牌化替换为 `c1cada/NexusTok`，不能复制上游链接。
4. 中英文模板需要保持同等约束，避免不同入口提交质量不一致。

### 方案评审

采用“模板文字增强，不改入口配置”的方案：

1. 在四份模板的“提交前必读 / Read This First”中补充 Relay/pass-through 上游行为边界和技术支持类 issue 边界。
2. 将提交确认项改成带加粗标签的结构化描述，包含非重复 issue、提交前必读、模板完整和维护成本。
3. Bug 模板在“问题描述 / Issue Description”下补充排查提示：问题现象、影响范围、为什么判断为 NexusTok 问题；转发问题附渠道/转换/上游依据/日志；计费问题附 `usage` 示例。
4. 功能请求模板只增强前置约束，不额外增加复杂字段，避免提高有效功能建议的提交门槛。
5. 保持 `.github/ISSUE_TEMPLATE/config.yml` 不变，因为 NexusTok 当前文档和 DeepWiki contact links 已正确品牌化。

验收方式：

1. `sed -n '1,220p' .github/ISSUE_TEMPLATE/{bug_report.md,bug_report_en.md,feature_request.md,feature_request_en.md}` 人工检查模板。
2. `rg -n "QuantumNous|docs.newapi.ai|newapi" .github/ISSUE_TEMPLATE` 确认没有上游品牌残留。
3. `git diff --check`。
4. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `sed -n '1,220p' .github/ISSUE_TEMPLATE/bug_report.md .github/ISSUE_TEMPLATE/bug_report_en.md .github/ISSUE_TEMPLATE/feature_request.md .github/ISSUE_TEMPLATE/feature_request_en.md` 已人工检查，四份模板均保留 NexusTok 链接并完成中英文等价增强。
2. `rg -n "QuantumNous|docs\\.newapi\\.ai|newapi" .github/ISSUE_TEMPLATE` 无匹配，未残留上游品牌或文档地址。
3. `git diff --check` 通过。
4. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
5. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：i18n 同步脚本字面量白名单与报告清理

### 需求分析

已按 `i18n-translate` 技能要求确认默认前端 i18n 维护流程：`web/default/scripts/sync-i18n.mjs` 是 locale 排序、缺失 key、extras 和未翻译报告的统一入口。`new-api-main` 的同名脚本相比 NexusTok 当前脚本有两个值得吸收的维护能力：

1. 有 `BRAND_AND_LITERAL_KEYS` 白名单，用于跳过品牌名、协议名、URL、密钥占位符、模型/渠道厂商名等不应翻译的英文值。
2. 对 URL、邮箱、API path、环境变量、模型名、`price_xxx`、`whsec_xxx`、JSON 示例等代码化字符串有额外模式过滤，避免被误报为未翻译。
3. 当某个 locale 已经没有 extras 或 untranslated 时，会删除旧报告文件，避免 stale 报告长期留在仓库中误导维护者。
4. NexusTok 当前 `_reports/*.untranslated.json` 中仍有大量真实句子未翻译，本轮不做批量翻译，只降低品牌/字面量噪声，保留真正需要翻译的自然语言句子。
5. 白名单必须品牌化，不能照搬 `New API`、`QuantumNous` 等上游标识；应加入 `NexusTok`、`c1cada`、`Waffo Pancake`、当前 provider 名称和技术占位符。

本轮目标是增强 i18n 同步脚本的误报过滤和 stale 报告清理能力，不新增 UI 文案，不改 `t()` 调用，不手工编辑六语 locale 文案。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| i18n 同步脚本 | `web/default/scripts/sync-i18n.mjs` | 新增 NexusTok 品牌化字面量白名单、代码化字符串过滤和 stale reports 清理逻辑。 |
| i18n 报告产物 | `web/default/src/i18n/locales/_reports/*` | 运行 `bun run i18n:sync` 后，未翻译计数会排除品牌/字面量噪声；无内容的旧报告会自动删除。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 白名单过宽会隐藏真实未翻译句子，因此只加入精确品牌/厂商/协议/占位符和明显代码化模式；普通自然语言句子仍会进入 untranslated 报告。
2. `bun run i18n:sync` 会重写 locale JSON 和报告文件；本轮需要检查 diff，确认没有意外改动翻译内容。
3. 删除 stale report 文件是维护行为变化，但只在对应报告为空时发生，不影响运行时前端。
4. 脚本是 Node ESM，不涉及后端 JSON 规则、数据库兼容性或 Docker 热更新逻辑。
5. 该能力只提升 i18n 检查精度，不能替代后续真实翻译工作；剩余未翻译自然语言仍需分批补齐。

### 方案评审

采用“保守白名单 + 报告清理”的方案：

1. 在 `sync-i18n.mjs` 中新增 `BRAND_AND_LITERAL_KEYS`，只放 NexusTok 当前使用的品牌、provider、支付平台、OAuth/URL/密钥字段、模型示例和占位符。
2. 在 `isLikelyUntranslated` 中优先检查白名单，再过滤 URL、API path、邮箱、环境变量、模型名、checkout 域名、footer key、全大写技术标识、JSON/数组示例和 HTML 换行实体。
3. 将该函数内触碰到的英文注释改为中文，符合项目中文注释约定。
4. 写入 extras/untranslated 报告时，如果本轮没有内容则删除对应旧文件，保持 `_reports` 和 `_extras` 只反映当前状态。
5. 运行 `bun run i18n:sync` 验证脚本，并检查 report 计数和 diff，确认没有隐藏真实自然语言未翻译项。

验收方式：

1. `node --check scripts/sync-i18n.mjs`。
2. `bun run i18n:sync`。
3. `cat src/i18n/locales/_reports/_sync-report.json`，确认 missing/extras/untranslated 统计可读。
4. `git diff --check`。
5. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `node --check scripts/sync-i18n.mjs` 通过。
2. `bun run i18n:sync` 通过，并生成最新 `src/i18n/locales/_reports/_sync-report.json`。
3. 最新同步报告：所有 locale 的 `missingCount=0`、`extrasCount=0`；`untranslatedCount` 为 `fr=34`、`ja=85`、`ru=97`、`vi=36`、`zh=1`。其中 zh 仅剩 `Quota:`，ja/ru 主要保留自然语言句子，URL、provider、密钥占位符和代码化示例噪声已被过滤。
4. `git diff --check` 通过。
5. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
6. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：中文 i18n Quota 漏翻清理

### 需求分析

在完成 i18n 同步脚本字面量白名单后，最新 `_sync-report.json` 显示 zh locale 仅剩 1 个未翻译项：`Quota:`。进一步核对六语当前值：

| locale | 当前值 |
|--------|--------|
| en | `Quota:` |
| zh | `Quota:` |
| fr | `Quota :` |
| ja | `クォータ：` |
| ru | `Квота:` |
| vi | `Hạn ngạch:` |

这说明 `Quota:` 不是应白名单跳过的品牌或代码化字面量，而是中文 locale 的真实漏翻。修复后可以验证本轮新增的 stale report 清理能力：当 zh 无未翻译项时，`zh.untranslated.json` 应被自动删除。

本轮目标是只修复 zh 中 `Quota:` 的翻译，并运行 `bun run i18n:sync` 更新报告；不新增 UI key，不修改其它语言，不改业务代码。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 中文 locale | `web/default/src/i18n/locales/zh.json` | 将 `Quota:` 翻译为 `额度：`。 |
| i18n 报告 | `web/default/src/i18n/locales/_reports/_sync-report.json`、`web/default/src/i18n/locales/_reports/zh.untranslated.json` | zh `untranslatedCount` 归零，并由同步脚本删除空的 zh untranslated 报告。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 只修改一个中文翻译值，不影响 API、计费、权限或数据库。
2. `Quota:` 是短标签，中文翻译使用全角冒号 `额度：`，与日文 `クォータ：` 的标点风格一致。
3. 运行 `bun run i18n:sync` 会重写报告文件；需要检查 diff，确认只有 zh locale 和 zh report 清理相关变化。
4. 该改动会让 zh 未翻译报告消失，这是脚本新增清理能力的预期行为。

### 方案评审

采用最小翻译修复：

1. 在 `zh.json` 中把 `Quota:` 的值从 `Quota:` 改为 `额度：`。
2. 运行 `bun run i18n:sync`，让脚本重算 `_sync-report.json` 并删除空的 `zh.untranslated.json`。
3. 检查 `_sync-report.json`，确认 zh 的 `untranslatedCount=0`，其它 locale 计数不因本轮翻译变化而异常波动。

验收方式：

1. `bun run i18n:sync`。
2. `jq -r '.translation["Quota:"]' src/i18n/locales/zh.json`。
3. `cat src/i18n/locales/_reports/_sync-report.json`。
4. `test ! -f src/i18n/locales/_reports/zh.untranslated.json`。
5. `git diff --check`。
6. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `bun run i18n:sync` 通过。
2. `jq -r '.translation["Quota:"]' src/i18n/locales/zh.json` 返回 `额度：`。
3. 最新 `_sync-report.json` 中 zh `missingCount=0`、`extrasCount=0`、`untranslatedCount=0`；fr/ja/ru/vi 计数保持为后续真实翻译待办。
4. `test ! -f src/i18n/locales/_reports/zh.untranslated.json` 通过，空报告已由同步脚本清理。
5. `git diff --check` 通过。
6. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
7. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：多语言 README 品牌化补齐

### 需求分析

`new-api-main` 在根目录提供 `README.en.md`、`README.zh_CN.md`、`README.zh_TW.md`、`README.fr.md`、`README.ja.md`，并在 README 顶部提供语言切换入口。NexusTok 当前根目录只有中文 `README.md`，对非中文用户、海外部署者和跨语言协作者不够友好，也无法完整展示 NexusTok 已经原生化出来的差异优势：

1. NexusTok 已形成多提供商 AI API 网关、账号池、统一权限治理、系统任务、系统信息、Dashboard Flow、Waffo Pancake、订阅/钱包、三库兼容等原生能力，但 README 仍偏部署说明，缺少能力总览。
2. 当前 README 没有语言切换入口；`new-api-main` 的多语言 README 结构值得吸收，但其中包含 New API 品牌、合作伙伴、徽章、外部 docs.newapi 链接，不应直接复制。
3. 多语言 README 应以 NexusTok 当前仓库文档为权威入口：`docs/installation/deployment.md`、OpenAPI 文件、GitHub Issues/Releases，而不是引用上游文档站点。
4. 本轮只补项目文档，不修改前端/后端运行时代码，不新增 UI 文案，不影响 Docker 热更新链路。

本轮目标是生成 NexusTok 品牌化的英文、简体中文、繁体中文、法文、日文 README，并在根 `README.md` 顶部加入语言切换入口；同时避免上游品牌和不存在的承诺残留。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 根 README | `README.md` | 增加语言切换入口，并保持当前简体中文内容可读。 |
| 多语言 README | `README.en.md`、`README.zh_CN.md`、`README.zh_TW.md`、`README.fr.md`、`README.ja.md` | 新增 NexusTok 品牌化项目说明、能力总览、快速开始、部署、文档和许可证说明。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 文档翻译可能出现术语不一致；本轮将保留 API、JWT、OAuth、WebAuthn、Redis、Waffo Pancake、SystemTask、Dashboard Flow 等固定术语，不强行翻译。
2. 多语言 README 可能与后续功能演进发生漂移，因此只描述当前已经落地或仓库已有的能力，不承诺未完成的 Casbin 持久化、Advanced Custom Channel 或完整 SDK 替换。
3. 根 README 增加语言入口不会影响构建、测试、Docker 镜像或页面运行。
4. 不迁移 new-api-main 的合作伙伴、Trendshift/ProductHunt/HelloGitHub 徽章和外部文档链接，避免品牌错配或不存在资源。
5. README 是仓库首页文档，链接错误会影响协作者体验；需要用脚本检查新增 README 中是否残留 `QuantumNous`、`Calcium-Ion`、`docs.newapi`、`calciumion/new-api` 等上游标识。

### 方案评审

采用“根 README 加入口 + 多语言文件品牌化生成”的方案：

1. 在现有 `README.md` 标题区增加语言切换行：简体中文、English、繁體中文、Français、日本語。
2. 新增 `README.zh_CN.md`，内容与根 README 同为简体中文，但加入语言切换和能力总览，作为明确的简中语言目标。
3. 新增 `README.en.md`、`README.zh_TW.md`、`README.fr.md`、`README.ja.md`，使用同一信息结构：Project description、Key capabilities、Quick start、Deployment、Documentation、License。
4. 所有命令、镜像名、仓库地址、维护手册、Issue/Releases 链接均使用 NexusTok 当前路径和 `c1cada/nexustok`。
5. 文档中对合规义务保持与根 README 一致的风险提示，不增加法律建议性质的承诺。

验收方式：

1. `find . -maxdepth 1 -type f -iname 'README*' -print | sort`，确认根目录 README 语言文件齐全。
2. `rg -n "QuantumNous|Calcium-Ion|docs\\.newapi|calciumion/new-api|New API" README*.md`，确认没有上游品牌残留。
3. `rg -n "README\\.(en|zh_CN|zh_TW|fr|ja)\\.md" README.md README*.md`，确认语言入口互链存在。
4. `git diff --check`。
5. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `find . -maxdepth 1 -type f -iname 'README*' -print | sort` 已确认根目录包含 `README.md`、`README.en.md`、`README.zh_CN.md`、`README.zh_TW.md`、`README.fr.md`、`README.ja.md`。
2. `rg -n "QuantumNous|Calcium-Ion|docs\\.newapi|calciumion/new-api|New API" README*.md` 无匹配，未残留上游品牌、镜像名或文档站链接。
3. `rg -n "README\\.(en|zh_CN|zh_TW|fr|ja)\\.md" README.md README*.md` 已确认语言入口互链存在。
4. 本地 README 链接检查通过：新增 README 中的相对链接和仓库根路径链接均能在当前仓库中解析到目标文件。
5. 代码块 fence 计数检查通过：新增 README 的 fenced code block 数量均为偶数。
6. `git diff --check` 通过。
7. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
8. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：手动任意分支 Docker 镜像工作流

### 需求分析

`new-api-main` 提供 `.github/workflows/docker-image-branch.yml`，维护者可以在 GitHub Actions 中手动输入分支名，为任意分支生成多架构 Docker Hub 预览镜像。NexusTok 当前已经有三条 Docker 发布链路：

1. `docker-build.yml`：面向正式 tag 和 `latest` 的多架构发布。
2. `docker-image-alpha.yml`：面向 `alpha` 分支的自动/手动预发布，推送 Docker Hub 和 GHCR，并启用 cosign。
3. `docker-image-nightly.yml`：面向 `nightly` 分支的自动/手动每日构建，只推送 Docker Hub。

缺口是：当某个功能分支需要交付给测试环境、Docker 用户或远程机器快速试运行时，维护者只能先合并到固定分支或手写本地构建命令。`new-api-main` 的优势在于把“指定分支 -> 生成稳定分支标签和日期 SHA 标签 -> 合并多架构 manifest -> 签名”的路径固定为可审计的 CI 能力。

本轮目标是把该能力原生化到 NexusTok：新增一个手动触发的任意分支 Docker 镜像工作流，继续使用 `c1cada/nexustok` 镜像名、现有 Dockerfile、多架构 runner、SBOM/provenance 和 cosign 签名。为避免覆盖 `latest`、`alpha`、`nightly` 或正式版本标签，本项目不会照搬上游“分支名直接作为 Docker tag”的策略，而是统一发布为 `branch-{sanitized-branch}` 和 `branch-{sanitized-branch}-{YYYYMMDD}-{SHORT_SHA}`。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| GitHub Actions | `.github/workflows/docker-image-branch.yml` | 新增手动触发的任意分支 Docker Hub 多架构构建、推送、manifest 和 cosign 签名。 |
| Docker 发布规范 | Docker Hub `c1cada/nexustok` | 新增 `branch-*` 预览镜像标签，不改变 `latest`、正式版本、`alpha`、`nightly` 的现有语义。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险、方案、验收方式和验证记录。 |

### 风险评估

1. 该 workflow 只通过 `workflow_dispatch` 手动触发，不监听所有分支推送，避免每个开发分支自动消耗 runner 和污染镜像仓库。
2. 任意分支构建会发布该分支当前 Dockerfile 产物。能触发 workflow 的维护者需要确认分支可信；workflow 不向 Docker build context 传入 Docker Hub token、GitHub token 或 cosign 凭证，但恶意 Dockerfile 仍可能发布不可用镜像或消耗构建资源。
3. 直接使用分支名作为 tag 会有覆盖稳定 tag 的风险。本轮强制加 `branch-` 前缀，并限制清洗后的分支主体长度，确保附加日期、短 SHA 和架构后仍不超过 Docker tag 长度限制。
4. 该变更只新增 CI 文件，不修改应用运行时代码、Dockerfile、Compose、数据库迁移、热更新容器或 3003 服务。
5. Docker Hub secrets 缺失时 workflow 会在登录阶段失败，这是发布环境配置问题，不影响本地开发和已部署服务。
6. `ubuntu-24.04-arm` runner 依赖 GitHub 当前 arm64 托管 runner 可用性；该风险与现有 alpha/nightly 工作流一致。

### 方案评审

采用“手动触发 + `branch-*` 标签命名 + Docker Hub only + cosign”的保守方案：

1. 新增 `.github/workflows/docker-image-branch.yml`，触发条件仅为 `workflow_dispatch.inputs.branch`。
2. `prepare` job 使用 `actions/checkout` 检出 `refs/heads/{branch}`，生成固定 commit SHA、清洗后的 tag 主体、`branch-{tag}` 前缀和 `branch-{tag}-{YYYYMMDD}-{SHORT_SHA}` 版本号；若分支名无法转为有效 tag 主体则 fail-fast。
3. `build_single_arch` job 复用现有 `amd64` / `arm64` 原生 runner、Docker Buildx、metadata labels、GHA cache、`provenance: mode=max`、`sbom: true`，分别推送 `branch-*-amd64` 和 `branch-*-arm64` 单架构镜像。
4. `create_manifests` job 合并 `branch-*` 和带日期 SHA 的两个多架构 manifest，并对 manifest tag 执行 cosign 签名。
5. 不同步推送 GHCR，原因是任意分支预览镜像数量更不可控，优先保持 Docker Hub 单仓库可清理；后续如确有 GHCR 预览需求，再按 alpha 工作流模式独立评审。

验收方式：

1. 检查 `.github/workflows/docker-image-branch.yml` 中不存在 `calciumion`、`new-api`、`QuantumNous` 等上游品牌残留。
2. 检查 workflow 包含 `workflow_dispatch`、`branch` input、`refs/heads/` checkout、`branch-` tag 前缀、`c1cada/nexustok`、`cosign` 和 `docker buildx imagetools create`。
3. 使用本地脚本解析 workflow 的 `uses:` 行，确认新增 workflow 使用的 action 均固定到 commit SHA。
4. `git diff --check`。
5. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `python3` + PyYAML 解析 `.github/workflows/docker-image-branch.yml` 通过，workflow 语法可被本地 YAML parser 读取；当前环境未安装 `actionlint` 和 `ruby`，因此未执行 actionlint 或 Ruby YAML 检查。
2. `rg -n "calciumion|QuantumNous|docs\\.newapi|New API|new-api" .github/workflows/docker-image-branch.yml` 无匹配，新增 workflow 未残留上游品牌、镜像名或文档站链接。
3. `rg -n "workflow_dispatch|branch:|refs/heads/|branch-|c1cada/nexustok|cosign|imagetools create|provenance: mode=max|sbom: true" .github/workflows/docker-image-branch.yml` 已确认手动触发、分支输入、分支引用、`branch-*` tag、NexusTok 镜像名、cosign、manifest、SBOM 和 provenance 配置存在。
4. 本地 Python 脚本检查新增 workflow 的全部 `uses:` 行均固定到 40 位 commit SHA，没有使用浮动 `@v*` 引用。
5. tag 清洗边界检查通过：`feature/account-pool` -> `branch-feature-account-pool`，`Alpha` -> `branch-alpha`，`release/v1.2.3` -> `branch-release-v1.2.3`，纯标点或纯非 ASCII 分支名会被判定为无效；最长清洗主体为 98 字符时，带日期、短 SHA 和 `-amd64` 的完整 tag 长度为 128。
6. `git diff --check` 通过。
7. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
8. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：Prettier 版权头保护包装器

### 需求分析

`new-api-main` 的默认前端把格式化命令封装为 `scripts/format-with-protected-headers.mjs`：格式化前临时剥离项目版权头，格式化完成后再恢复，避免 formatter 或 import 排序工具把 AGPL 版权头移动、重排或误判为普通块注释。NexusTok 当前 `web/default` 仍使用 Prettier + `@trivago/prettier-plugin-sort-imports`，且大量源码文件已经有 `Copyright (C) 2023-2026 c1cada` 头部，因此同样需要保护“版权头永远处于文件最前方”的不变量。

需要注意的是，当前 `web/default` 的既有 `bun run format:check` 基线已经失败，Prettier 报告 89 个历史格式问题。本轮不能把全仓库格式化混入工程化脚本改造，否则会产生大量不相关 diff，影响 review 和热更新稳定性。本轮只把 new-api-main 的保护思路原生化为 NexusTok 的 Prettier wrapper，保持现有 Prettier、ESLint、TypeScript 和 Bun 工具链不变。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 前端格式化脚本 | `web/default/scripts/format-with-protected-headers.mjs` | 新增 NexusTok 品牌化的 Prettier 包装器，保护 `c1cada` 版权头并支持 `--check` / `--write`。 |
| 前端 package scripts | `web/default/package.json` | 将 `format:check` 和 `format` 从直接调用 Prettier 改为调用包装器；其它脚本、依赖和锁文件不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险、方案、验收方式和验证记录。 |

### 风险评估

1. 该变更只影响开发者执行 `bun run format` / `bun run format:check` 时的行为，不参与前端构建产物、后端 API、数据库、计费或热更新容器运行。
2. check 模式会像 new-api-main 一样临时执行 formatter 写入，因此必须在脚本中先保存快照，并在 `finally` 中恢复，避免失败检查污染工作区。
3. 当前 `format:check` 既有基线失败。本轮验收需要确认新的 wrapper 仍然失败并列出格式问题，同时不留下任何未预期文件改动；不把历史格式化债和脚本改造合并到一个 commit。
4. 不引入 `oxfmt`、`oxlint`、`tsgo` 或 web workspace catalog，避免改变依赖安装方式、CI 速度特征和现有 lint/typecheck 规则。
5. 脚本只保护 NexusTok 当前 `c1cada` 版权头，不再复制上游 `QuantumNous` 品牌匹配；历史上游版权头已经由 `add-copyright.mjs` 负责识别和迁移。

### 方案评审

采用“复用现有 Prettier，新增保护层”的方案：

1. 新增 `web/default/scripts/format-with-protected-headers.mjs`，使用 NexusTok AGPL 版权头，内部只调用本项目已有 `prettier --write .`。
2. wrapper 在 `--check` 模式下记录 10MB 以下文件快照，剥离 `.js/.ts/.tsx/.css/.scss` 等文件开头的 `c1cada` 项目版权头，执行 Prettier，恢复版权头，再对比快照列出会被格式化改动的文件，最后恢复原始快照。
3. wrapper 在 `--write` 模式下剥离版权头、执行 Prettier、恢复版权头，保留正常格式化输出。
4. `web/default/package.json` 的 `format:check` 和 `format` 改为调用该 wrapper；`build:check`、`typecheck`、`lint`、`copyright` 和 `i18n:sync` 不变。

验收方式：

1. `node --check scripts/format-with-protected-headers.mjs`。
2. 用 `/tmp` 临时 fixture 验证 `--check` 会失败但还原文件，`--write` 会保留版权头并格式化内容。
3. `bun run format:check`：预期仍因当前 89 个历史格式问题失败，但不留下工作区改动。
4. `git diff --check`。
5. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `node --check scripts/format-with-protected-headers.mjs` 通过。
2. `/tmp` fixture 验证通过：`--check` 对未格式化文件返回 1 并恢复原文件；`--write` 后版权头仍在首行，正文被格式化为 `const demo = { value: 1 }`。
3. 首次真实 `bun run format:check` 暴露脚本边界：wrapper 快照包含 `dist.hot` 热更新产物，检查期间文件消失会触发 `ENOENT` 并留下临时格式化 diff。本轮已修复：排除 `dist.hot`、`dist.next`、`dist.prev`、`dist-ssr`，恢复快照时重建父目录，恢复版权头时跳过已删除文件。
4. 修复后再次执行 `bun run format:check`，预期返回 1；失败点变为历史格式差异列表，并输出 `Format issues found in protected-header-safe check:`，不再出现 `ENOENT`，且工作区未新增临时格式化改动。
5. `git diff --check` 通过。
6. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
7. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：Secure Session Cookie 示例补齐

### 需求分析

前序切片已经把 `new-api-main` 的 Secure Session Cookie 配置原生化到 NexusTok：`common/session_cookie.go` 支持 `SESSION_COOKIE_SECURE` 与 `SESSION_COOKIE_TRUSTED_URL`，`common/session_cookie_test.go` 覆盖默认 HTTP 兼容、成对配置校验、HTTPS URL 校验和多 URL trim，`main.go` 的 session store 已消费 `common.SessionCookieSecure`。后续开发 Compose 修补也已经在 `docker-compose.dev.yml` 里补充 HTTPS 开发注释。

当前尾差是 `.env.example` 仍只展示 `SESSION_SECRET`，没有暴露这两个已经生效的部署安全变量。生产环境使用 HTTPS 反代时，维护者不容易从示例配置中发现 Secure Cookie 能力；而直接照抄上游 `.env.example` 中未接线的变量又会制造“文档声明但运行无效”的风险。因此本轮只补已经落地的 Secure Session Cookie 示例，不补 `RELAY_IDLE_CONN_TIMEOUT` 等尚未接线的变量。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 环境变量示例 | `.env.example` | 在 `SESSION_SECRET` 附近补充 `SESSION_COOKIE_SECURE` 和 `SESSION_COOKIE_TRUSTED_URL` 注释示例。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录该文档尾差的需求、影响、风险、方案和验证方式。 |

### 风险评估

1. `.env.example` 是配置示例文件，不改变默认运行时配置、热更新容器、数据库、前端构建或 API 行为。
2. 示例必须明确 `SESSION_COOKIE_SECURE=true` 需要可信 HTTPS 入口地址；如果用户在纯 HTTP 环境启用 Secure Cookie，浏览器不会携带 session cookie，这是预期但容易误用。
3. 本轮只补后端已经支持并有测试覆盖的变量，不补 `RELAY_IDLE_CONN_TIMEOUT`，避免出现无效环境变量示例。
4. 文案应保持 NexusTok 品牌和中文说明，不复制上游项目名、邮箱或无关配置。

### 方案评审

采用最小示例补齐：

1. 在 `.env.example` 的 `SESSION_SECRET` 后增加 Secure session cookie 注释。
2. 默认示例保持注释状态：`# SESSION_COOKIE_SECURE=false`，不影响本地 HTTP。
3. `SESSION_COOKIE_TRUSTED_URL` 示例使用 `https://example.com,https://admin.example.com`，与现有后端校验和 dev compose 注释保持一致。

验收方式：

1. `rg -n "SESSION_COOKIE_SECURE|SESSION_COOKIE_TRUSTED_URL" .env.example docker-compose.dev.yml common/session_cookie.go docs/features/new-api-main-diff-analysis.md`。
2. `git diff --check`。
3. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `rg -n "SESSION_COOKIE_SECURE|SESSION_COOKIE_TRUSTED_URL" .env.example docker-compose.dev.yml common/session_cookie.go docs/features/new-api-main-diff-analysis.md` 已确认示例、dev compose 注释、后端配置和差异报告均能对应到同一组环境变量。
2. `git diff --check` 通过。
3. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
4. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：本地 MCP 与临时测试忽略项补齐

### 需求分析

`new-api-main` 的 `.gitignore` 已经补充 `.playwright-mcp`、`.local-tests/` 和本地 live probe 测试文件，用来隔离浏览器 MCP 状态、临时联调脚本和只在开发者机器运行的真实上游探针。NexusTok 当前项目约定要求前后端完成后尽量用 MCP 浏览器验证，且我们在持续原生化过程中会反复创建 `/tmp` 或本地 scratch 探针；如果这些文件落在仓库内，容易被误加入提交。

NexusTok 已经有 `service/openaicompat/*` 原生命名，不应照搬上游仅存在于 `service/relayconvert` 的路径。本轮目标是吸收上游忽略策略，但按 NexusTok 当前目录和历史兼容需求补齐忽略项：保留 `service/relayconvert/chat_responses_live_local_test.go` 的上游路径防误放，同时增加本项目实际的 `service/openaicompat/chat_responses_live_local_test.go`。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Git 忽略规则 | `.gitignore` | 增加 MCP 浏览器状态目录、临时测试工作区和本地 live probe 测试文件忽略项。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险、方案和验收方式。 |

### 风险评估

1. `.gitignore` 只影响未跟踪文件的默认显示和误提交风险，不改变任何运行时代码、构建产物、数据库、前端页面或 3003 热更新行为。
2. 忽略规则不能覆盖正式测试文件；因此只忽略带 `live_local_test.go` 的明确本地探针文件，不忽略整个 `service/openaicompat` 或 `service/relayconvert` 目录。
3. `.local-tests/` 是专门的 scratch 工作区；如果未来需要提交其中内容，应先移出该目录或调整规则。
4. `.playwright-mcp` 只用于本地 MCP/浏览器状态，不应进入仓库。

### 方案评审

采用最小忽略项补齐：

1. 在 `.gitignore` 末尾增加 `.playwright-mcp`。
2. 增加注释分组 `# Local-only live probes and scratch test workspaces.`，并忽略 `.local-tests/`。
3. 增加 `service/relayconvert/chat_responses_live_local_test.go` 和 `service/openaicompat/chat_responses_live_local_test.go`，同时兼容上游路径和 NexusTok 当前原生命名路径。

验收方式：

1. `rg -n "playwright-mcp|local-tests|live_local_test" .gitignore docs/features/new-api-main-diff-analysis.md`。
2. 创建临时路径并用 `git check-ignore -v` 确认 `.playwright-mcp/state.json`、`.local-tests/probe.txt`、`service/openaicompat/chat_responses_live_local_test.go` 和 `service/relayconvert/chat_responses_live_local_test.go` 均被忽略，随后删除临时文件。
3. `git diff --check`。
4. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `rg -n "playwright-mcp|local-tests|live_local_test" .gitignore docs/features/new-api-main-diff-analysis.md` 已确认 `.gitignore` 和差异报告均记录本轮忽略项。
2. `git check-ignore -v` 验证通过：`.playwright-mcp/state.json`、`.local-tests/probe.txt`、`service/openaicompat/chat_responses_live_local_test.go` 和 `service/relayconvert/chat_responses_live_local_test.go` 均命中预期规则；临时验证文件已删除。
3. `git diff --check` 通过。
4. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
5. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：Relay HTTP 空闲连接超时配置

### 需求分析

`new-api-main` 支持 `RELAY_IDLE_CONN_TIMEOUT`，并把它应用到默认 Relay HTTP client、HTTP/HTTPS 代理 client、SOCKS5 代理 client 和 SSRF protected fetch client 的 `http.Transport.IdleConnTimeout`。NexusTok 当前已经支持 `RELAY_MAX_IDLE_CONNS` 与 `RELAY_MAX_IDLE_CONNS_PER_HOST`，但自建 transport 没有设置 `IdleConnTimeout`，实际语义变成“空闲连接永不因 transport 自身超时关闭”，不同于 Go 标准库默认的 90 秒。

长期运行的 API 网关会频繁连接多种上游 provider、代理和用户可控 URL 下载端点。为连接池补上可配置空闲超时，可以减少长期空闲连接占用、让代理/上游连接回收更可控，并与 new-api-main 的工程化配置保持一致。

本轮目标是将该能力原生化到 NexusTok：新增 `common.RelayIdleConnTimeout`，默认 90 秒，对齐 Go 标准库和 new-api-main；允许通过 `RELAY_IDLE_CONN_TIMEOUT=0` 显式关闭空闲连接超时；同时补 `.env.example` 示例和 transport 配置测试。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 全局配置 | `common/constants.go`、`common/init.go` | 新增 `RelayIdleConnTimeout`，从 `RELAY_IDLE_CONN_TIMEOUT` 读取，默认 90 秒。 |
| 环境变量示例 | `.env.example` | 在 Relay timeout 相关配置下新增空闲连接超时说明。 |
| HTTP client | `service/http_client.go` | 默认 client、HTTP/HTTPS proxy client、SOCKS5 proxy client 的 transport 设置 `IdleConnTimeout`。 |
| SSRF protected fetch | `service/protected_fetch_client.go` | 用户可控 URL 专用 transport 设置同一空闲连接超时。 |
| 回归测试 | `service/http_client_test.go`、`service/protected_fetch_client_test.go` | 覆盖默认 client、HTTP proxy、SOCKS5 proxy 和 protected fetch transport 的配置传播。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求、影响、风险、方案和验收方式。 |

### 风险评估

1. 默认值从自建 transport 的零值“不主动关闭空闲连接”变为 90 秒，行为更接近 Go 标准库默认值，但仍可能改变少数依赖超长空闲连接复用的部署；因此提供 `RELAY_IDLE_CONN_TIMEOUT=0` 作为显式关闭开关。
2. 所有新增设置只影响新创建的 transport；已运行进程中的现有连接池需要重启或重新初始化 client 后才会使用新配置。
3. 代理 client 有缓存，配置变更后需要 `ResetProxyClientCache()` 或进程重启才能让缓存代理 client 生效；这与现有 max idle 配置一致。
4. SSRF protected fetch 的 transport 是按代理地址缓存的，新增字段必须覆盖 direct 和 proxied 两类 transport。
5. 不修改 controller 中已有的模型同步/倍率同步专用短连接 transport；这些路径已经手动设置了 90 秒和额外 header timeout，本轮只覆盖通用 relay/protected fetch client。

### 方案评审

采用与 new-api-main 等价但保留 NexusTok 命名和测试边界的实现：

1. 在 `common/constants.go` 增加 `RelayIdleConnTimeout int`，注释说明单位为秒、0 表示不限制。
2. 在 `common.InitEnv()` 中读取 `RELAY_IDLE_CONN_TIMEOUT`，默认值为 90。
3. 在 `.env.example` 的 `RELAY_TIMEOUT` 后补充 `RELAY_IDLE_CONN_TIMEOUT=90` 示例。
4. 在 `service/http_client.go` 的三个 `http.Transport` 字面量中设置 `IdleConnTimeout: time.Duration(common.RelayIdleConnTimeout) * time.Second`。
5. 在 `service/protected_fetch_client.go` 的 `newTransport` 中设置同一字段。
6. 新增 `service/http_client_test.go` 覆盖默认 client、HTTP proxy 和 SOCKS5 proxy；扩展 `TestProtectedFetchRoundTripperReusesTransportPerProxy` 覆盖 protected fetch direct/proxy transport 的 `IdleConnTimeout`。

验收方式：

1. `go test ./common ./service -run 'Test.*(RelayIdleConnTimeout|ProtectedFetchRoundTripperReusesTransportPerProxy)'`。
2. `go test ./service`。
3. `rg -n "RelayIdleConnTimeout|RELAY_IDLE_CONN_TIMEOUT|IdleConnTimeout" common service .env.example docs/features/new-api-main-diff-analysis.md`。
4. `git diff --check`。
5. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `go test ./common ./service -run 'Test.*(RelayIdleConnTimeout|ProtectedFetchRoundTripperReusesTransportPerProxy)'` 通过。
2. `go test ./service` 通过。
3. `go test ./common` 通过。
4. `rg -n "RelayIdleConnTimeout|RELAY_IDLE_CONN_TIMEOUT|IdleConnTimeout" common service .env.example docs/features/new-api-main-diff-analysis.md` 已确认配置变量、环境示例、默认/代理/protected fetch transport 和测试均已接入。
5. `git diff --check` 通过。
6. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
7. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：前端依赖安全护栏

### 需求分析

`new-api-main` 在默认前端 `package.json` 中维护了一组 `overrides`，用于把部分高频安全敏感或工具链传递依赖固定到更高补丁版本。NexusTok 默认前端当前已经原生化了安全富文本渲染、Playground 受控 Markdown 渲染、CodeMirror 编辑器和 MCP/shadcn 工具链，但 `web/default/package.json` 尚无依赖 override 护栏，`bun.lock` 中仍存在若干比 `new-api-main` 低的包版本，例如 `dompurify@3.3.3`、`fast-uri@3.1.0`、`hono@4.12.12`、`js-cookie@3.0.5`、`mermaid@11.14.0`、`postcss@8.5.9` 和 `qs@6.15.1`。

本轮目标不是照搬 `new-api-main` 的完整 overrides，而是把它的“前端依赖安全护栏”转化为 NexusTok 原生、可维护、与现有依赖树兼容的能力：直接升级 DOMPurify 到 `3.4.11`，并只为当前依赖范围内同主版本兼容的传递依赖添加 overrides，避免把不兼容主版本强行压到全局。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 前端依赖声明 | `web/default/package.json` | 将直接依赖 `dompurify` 升级到 `3.4.11`，新增最小兼容 `overrides` 护栏。 |
| 前端锁文件 | `web/default/bun.lock` | 由 Bun 重新解析依赖树，锁定 DOMPurify 和兼容传递依赖的新版本。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录依赖护栏的需求、影响范围、风险、方案和验证结果。 |

### 风险评估

1. `overrides` 是全局依赖解析规则，如果照搬 `brace-expansion: 2.1.1` 会把当前 `minimatch@10` 需要的 `brace-expansion@^5.0.5` 强行降到不兼容主版本；如果照搬 `uuid: 14.0.0` 会覆盖 `@lobehub/ui` 的 `uuid@^13` 和 `mermaid` 的 `uuid@^11`。这两项本轮不接入。
2. `ip-address@10.2.0` 虽然与现有 `10.1.0` 同主版本，但当前锁文件中 `express-rate-limit` 使用精确依赖 `10.1.0`，不是 semver 范围；为避免覆盖上游精确约束，本轮暂不加入 override。
3. `mermaid`、`hono`、`postcss`、`qs` 等包主要来自 dev 工具链或可视化/文档相关传递依赖，补丁升级理论上兼容，但仍可能带来锁文件解析差异；必须执行 `bun install`、类型/生产构建和静态页面验证。
4. DOMPurify 是前端安全渲染链路的核心依赖，升级后必须确认 `HtmlContent`、`RichContent` 和 Playground Response renderer 构建正常，避免清洗 API 或类型变化造成页面白屏。
5. 本轮只调整依赖解析，不新增运行时代码、不改变 API、数据库、计费或权限逻辑；若构建或 3003 验证暴露不兼容，应收窄 overrides，而不是扩大升级范围。

### 方案评审

采用最小兼容方案：

1. `web/default/package.json` 中把直接依赖 `dompurify` 从 `3.3.3` 升级为 `3.4.11`。
2. 新增 `overrides`，只包含当前依赖树范围兼容的包：`dompurify@3.4.11`、`fast-uri@3.1.2`、`hono@4.12.22`、`js-cookie@3.0.7`、`mermaid@11.15.0`、`minimist@1.2.8`、`postcss@8.5.15`、`qs@6.15.2`。
3. 暂缓 `brace-expansion`、`uuid` 和 `ip-address` 三项，原因分别是不兼容主版本、跨多个主版本依赖路径和上游精确依赖约束。
4. 使用 `cd web/default && bun install` 更新 `bun.lock`，不手写锁文件。
5. 验证 `package.json` 与 `bun.lock` 中目标版本生效，并执行 `bun run build:check` 覆盖 TypeScript 与生产构建。
6. 优先使用 MCP 打开 3003；若 MCP 仍不可用，则使用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

验收方式：

1. `cd web/default && bun install`。
2. `cd web/default && bun run build:check`。
3. `rg -n '"(dompurify|fast-uri|hono|js-cookie|mermaid|minimist|postcss|qs)"|overrides' web/default/package.json web/default/bun.lock`。
4. `git diff --check`。
5. 优先用 MCP 打开 `http://192.168.0.202:3003/`；如 MCP 仍不可用，则用 `curl --noproxy '*'` 验证 `/`、`/api/status` 和登录后的 `/api/user/self`。

### 本轮验证记录

1. `cd web/default && bun install` 通过，并更新 `web/default/bun.lock`。
2. `cd web/default && bun install --frozen-lockfile` 通过，确认 `package.json` 与 `bun.lock` 一致。
3. `cd web/default && bun run build:check` 通过，覆盖 TypeScript 编译和 Rsbuild 生产构建。
4. `rg -n '"(dompurify|fast-uri|hono|js-cookie|mermaid|minimist|postcss|qs)"|overrides' web/default/package.json web/default/bun.lock` 已确认 DOMPurify 直接依赖和 overrides 生效；`brace-expansion@5.x`、`uuid@13` 与 `ip-address@10.1.0` 未被不兼容覆盖。
5. `git diff --check` 通过。
6. 已尝试 MCP 浏览器验证 3003，但 Chrome DevTools MCP 仍无法连接，错误为 `Could not connect to Chrome. Check if Chrome is running. Cause: Failed to fetch browser webSocket URL from http://127.0.0.1:9222/json/version: fetch failed`。
7. 3003 真实 HTTP 兜底验证通过：`/` 返回 200；`/api/status` 返回 200 且 `success=true`；使用账号 `c1cada` 登录返回 `success=true`；登录后 `GET /api/user/self` 返回 `success=true` 且用户名为 `c1cada`。

## 本轮实施评审：models.dev 手动同步全量缺失补齐

### 需求分析

用户在默认前端模型元信息页搜索 `gpt-5.6` 后发现本地只同步到 `gpt-5.6-sol`，而 models.dev 当前公开目录中同一系列实际包含 `openai/gpt-5.6-luna`、`openai/gpt-5.6-sol` 和 `openai/gpt-5.6-terra` 三条 canonical model。排查发现 NexusTok 已经具备 models.dev canonical 解析能力，后台每日自动任务也会以 `CreateAllUpstream=true` 按上游目录补齐本地缺失模型；但管理页面的手动 `Sync Upstream Models` 仍沿用旧 official 同步语义，只创建当前能力表里出现但元信息缺失的模型。

这会造成一个用户可见的不一致：models.dev 被前端标记为推荐来源，文案也强调同步模型元数据和供应商价格，但手动点击同步时只补齐当前渠道/能力已经出现的模型。对于刚发布的模型系列，如果能力表只因为某个渠道暴露了 `sol`，`luna` 和 `terra` 即使存在于 models.dev canonical catalog 中也不会被创建。

本轮目标是修复 models.dev 手动同步和预览的目标集合：选择 models.dev 来源时，默认按上游 catalog 补齐所有本地缺失模型；旧 official 来源继续保持“只补能力表缺失模型”的历史行为。这样用户在页面手动同步后，`gpt-5.6` 系列会一次性补齐三条模型记录。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 同步请求结构 | `controller/model_sync.go` | 增加可选 `create_all` 逃生开关；models.dev 未显式指定时默认全量缺失补齐。 |
| 手动同步核心 | `controller/model_sync.go` | `syncUpstreamModelsCore` 对 models.dev 源使用完整上游目录计算待创建模型。 |
| 同步预览接口 | `controller/model_sync.go` | `/api/models/sync_upstream/preview?source=models.dev` 的 `missing` 改为展示所有上游存在、本地不存在的模型。 |
| 回归测试 | `controller/model_sync_test.go` | 覆盖 GPT-5.6 Luna/Sol/Terra canonical 形态，以及 models.dev 手动同步默认补齐同系列剩余模型。 |
| 功能文档 | `docs/features/model-sync-pricing.md` | 更新手动 models.dev 同步语义，避免文档仍描述成只补能力表缺失项。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证。 |

### 风险评估

1. models.dev 手动同步的创建范围会从“当前能力表缺失项”扩大为“models.dev catalog 中本地缺失项”，首次执行可能创建更多模型元信息。这与每日自动同步已有语义一致，但对只想补单个能力缺口的管理员来说感知会更明显。
2. 旧 official 来源仍保持原行为，避免一次性导入 legacy 元数据仓库中所有模型，降低对已有部署的默认行为冲击。
3. 只创建本地不存在的模型，不静默覆盖已有模型的描述、标签、状态、供应商等字段；已有冲突仍通过预览和人工覆盖流程处理。
4. `create_all` 作为可选请求字段保留手动控制能力：未来如需在脚本或调试场景回退到旧范围，可显式传 `false`。
5. 实际调用 3003 同步接口会改动运行中数据库，属于本轮修复目标的一部分；验证时必须确认 `gpt-5.6-luna`、`gpt-5.6-sol`、`gpt-5.6-terra` 均可通过模型 API 查到。

### 方案评审

采用后端语义修复为主、前端无需新增交互的方案：

1. 在 `syncRequest` 中增加 `CreateAll *bool`，JSON 字段为 `create_all`。
2. 新增 `resolveSyncUpstreamOptions`：若请求显式带 `create_all`，优先使用该值；否则当 `source=models.dev` 时默认 `CreateAllUpstream=true`，official 仍为 false。
3. `syncUpstreamModelsCore` 在标准化 source 后统一调用该 helper，保证 HTTP 手动同步、冲突覆盖路径和测试路径行为一致。
4. `SyncUpstreamPreview` 同样使用该 helper 计算 `missing`，使预览弹窗和实际写入目标一致；同时把 models.dev 状态对比切到 `chooseSyncModelStatus`，避免 deprecated 状态在预览里被误当成启用。
5. 保留自动同步任务现有 `CreateAllUpstream=true`，不改调度、价格同步、供应商纠偏和人工覆盖逻辑。
6. 测试加入当前 models.dev GPT-5.6 canonical 数据形态，确认转换层能得到 Luna/Sol/Terra 三条；再模拟本地已存在 `gpt-5.6-sol` 的场景，确认手动 models.dev 默认同步会创建缺失的 `luna` 和 `terra`。

验收方式：

1. `go test ./controller -run 'Test(ConvertModelsDevCatalogPreservesGPT56Series|SyncUpstreamModelsCoreModelsDevDefaultsToFullCatalog|SyncUpstreamModelsCoreCreatesModelsDevCatalogModels|SyncUpstreamModelsCoreCorrectsExistingModelsDevVendor)'`。
2. `go test ./controller -run 'Test.*ModelsDev'`。
3. `go test ./controller`。
4. `cd web/default && bun run typecheck`。
5. `cd web/default && bun run build`。
6. `git diff --check`。
7. 优先用 MCP 打开 `http://192.168.0.202:3003/` 并触发/检查模型同步；如 MCP 工具受限，则用 `curl --noproxy '*'` 登录后调用 `/api/models/sync_upstream` 和模型搜索接口，确认 `gpt-5.6-luna`、`gpt-5.6-sol`、`gpt-5.6-terra` 均存在，并比对 models.dev catalog 与本地模型集合差集。

### 本轮验证记录

1. `curl --noproxy '*' https://models.dev/catalog.json` 确认当前 models.dev canonical catalog 包含 `openai/gpt-5.6-luna`、`openai/gpt-5.6-sol` 和 `openai/gpt-5.6-terra` 三条。
2. `go test ./controller -run 'Test(ConvertModelsDevCatalogPreservesGPT56Series|SyncUpstreamModelsCoreModelsDevDefaultsToFullCatalog|SyncUpstreamModelsCoreCreatesModelsDevCatalogModels|SyncUpstreamModelsCoreCorrectsExistingModelsDevVendor)'` 通过。
3. `go test ./controller -run 'Test.*ModelsDev'` 通过。
4. `go test ./controller` 通过。
5. `cd web/default && bun run typecheck` 通过。
6. `cd web/default && bun run build` 通过。
7. `git diff --check` 通过。
8. MCP 已连接到 3003 页面并获取首页快照，页面可访问；当前 MCP 工具集缺少导航/点击/表单输入工具，接口验证改用 3003 真实 HTTP 调用完成。
9. 3003 登录验证通过：使用账号 `c1cada` 登录 `/api/user/login` 返回 `success=true`。
10. 3003 `GET /api/models/search?keyword=gpt-5.6` 返回 3 条：`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`。三者 `vendor_id=1`，供应商接口确认 `id=1` 为 `OpenAI`；其中 `sol` 已有 `openai/default` 能力绑定，`luna` 和 `terra` 仅完成元信息同步，尚未绑定能力。
11. 3003 `GET /api/models/sync_upstream/preview?source=models.dev&create_all=true` 返回 `success=true`、`missing=0`、`conflicts=0`。
12. 3003 `POST /api/models/sync_upstream` 使用页面默认语义（`source=models.dev`，不显式传 `create_all`）返回 `success=true`、`created_models=0`、`updated_models=0`、`created_vendors=0`、`skipped_models=0`；说明手动 models.dev 同步入口已按全量 catalog 计算并且当前库无缺失。
13. 将 models.dev canonical keys 按同步规则去掉 owner 前缀后与 3003 `/api/models/search` 分页结果做集合比对：models.dev 期望唯一模型数 244，本地唯一模型数 244，缺失数 0。

## 本轮实施评审：差异报告优先队列状态校准

### 需求分析

持续推进差异吸收时发现，文档早期的“后续优先队列”和部分顶部总表仍保留旧状态：GORM v2 行锁 helper、Responses/Gemini Responses、管理操作审计、会话守卫、HeaderNavModules、Usage Logs 移动端、Dashboard Flow、Advanced Custom、Waffo Pancake、OpenAI 图片流式、用户级 Authz override 和多组权限路由表等已经在后续切片中落地，但早期段落仍把它们写成“缺少”或“待迁移”。这会让后续开发重复选择已经完成的能力，偏离“持续把 new-api-main 优势发展成 NexusTok 原生能力”的真实目标。

本轮目标是只做报告状态校准，不改运行时代码：

1. 按当前工作树和后续实施记录，把顶部差异总表中已完成的能力改为“已原生化/后续维护点”。
2. 将旧“子 Agent 复核后的后续优先队列”替换为 2026-07-10 复核后的真实剩余队列。
3. 校准“原生化路线建议”和“建议的落地顺序”，让后续计划从“基础迁移”转为“角色策略持久化、细资源拆分、账号池日志体验、DataTable 渐进分层、ClickHouse 真接入”等剩余能力。
4. 保留完整目标未完成的判断，不把报告校准误写成整个 new-api-main 差异吸收已经结束。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| 顶部差异总表 | `docs/features/new-api-main-diff-analysis.md` | 校准 Authz、管理审计、Flow、OpenAI 图片流式、Advanced Custom、Waffo Pancake、用户权限回传等状态。 |
| 后续优先队列 | `docs/features/new-api-main-diff-analysis.md` | 用当前证据替换早期 P0/P1/P2 队列，避免重复开发已完成切片。 |
| 原生化路线建议 | `docs/features/new-api-main-diff-analysis.md` | 更新平台治理、安全边界、订阅支付、仪表盘日志、Relay/Playground 的下一步方向。 |
| 实施索引 | `docs/features/new-api-main-diff-analysis.md` | 新增本轮状态校准记录，便于后续回溯。 |

### 风险评估

1. 最大风险是把尚未完成的能力误标为完成。本轮只校准已有明确代码证据和报告实施记录的项，例如 `model/locking.go`、`service/openaicompat/*`、`relay/channel/advancedcustom/*`、`web/default/src/features/dashboard/components/flow/*`、`web/default/src/features/usage-logs/components/usage-logs-mobile-card.tsx`、`model/authz_user_override.go` 和各组 `router/*_router.go` 权限表。
2. 本轮只修改文档，不影响 Go/前端编译、数据库 schema、权限判定、支付、Relay 热路径或 3003 运行态行为。
3. 文档中的剩余队列不能过度收窄总目标；因此仍保留 Casbin/角色策略持久化、ClickHouse 真接入、账号池日志体验、DataTable 分层、Classic parity 等未完成方向。
4. 由于报告很长，局部校准可能仍有少量旧表述散落在历史实施评审中；历史评审保留当时语境，本轮优先修正会影响后续决策的总表、队列和路线建议。

### 方案评审

采用“证据驱动的文档校准”方案：不根据记忆判断完成度，而是用当前仓库文件、路由、测试文件和后续实施记录作为证据。总表只改能力当前状态和后续迁移方式；早期历史评审段落不回写改动当时的需求背景，避免破坏时间线。优先队列不再保留已完成项，而是重新列出仍能从当前代码中确认未完全实现的后续能力。

验收方式：

1. `rg` 确认早期队列不再把 GORM 行锁、Responses、Gemini Responses、管理审计、Usage Logs、Dashboard Flow、Advanced Custom 和 Waffo Pancake 写成待迁移主项。
2. `rg` 确认新增的 2026-07-10 复核队列和本轮实施评审存在。
3. `git diff --check` 确认文档无空白错误。
4. 按热更新要求访问 `http://192.168.0.202:3003/` 和 `/api/status`，确认本轮文档修改没有影响运行态服务可用性。

### 实施结果

已完成差异报告状态校准：

- 顶部总表已把 Authz、权限路由表、管理审计、Flow/Sankey、OpenAI image stream、Advanced Custom、Waffo Pancake 和用户权限回传改为当前已落地状态，并明确剩余方向。
- 早期“子 Agent 复核后的后续优先队列”已替换为“2026-07-10 复核后的后续优先队列”，重点转向 Casbin/角色策略持久化、细资源拆分、ClickHouse 真接入、账号池任务持久队列、账号池日志体验、DataTable 渐进分层和 Classic parity。
- 原生化路线建议已从“先做基础能力”更新为“已完成基础闭环后的维护契约和下一层能力”，避免后续重复实现已经落地的切片。
- 本轮没有修改运行时代码、前端资源或数据库迁移。

### 验证记录

本轮修改后已执行以下验证：

1. `rg -n "2026-07-10 复核后的后续优先队列|差异报告优先队列状态校准|角色模板与 Casbin|账号池日志与全局 Usage Logs" docs/features/new-api-main-diff-analysis.md`，确认复核队列、本轮实施评审和实施索引均已写入。
2. `rg -n "P0 \\| GORM v2 行锁 helper|P1 \\| OpenAI Responses 反向兼容|P1 \\| Gemini Responses|P1 \\| Usage Logs 移动端|P1/P2 \\| Dashboard Flow" docs/features/new-api-main-diff-analysis.md` 无输出，确认旧优先队列主项已移除。
3. `git diff --check` 通过。
4. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 `HTTP/1.1 200 OK`。
5. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回正常状态 JSON，`success=true`。

| 日期 | 能力 | 文件 | 说明 |
|------|------|------|------|
| 2026-07-11 | 渠道复制结构化审计补齐 | `controller/channel.go`、`controller/audit.go`、`model/channel.go`、`controller/channel_copy_audit_test.go` | 补齐 new-api-main 已有的 `channel.copy` 业务审计参数：复制成功后手动记录 `sourceId/id/name`，避免只依赖中间件兜底的 route/action；同时让单条 `Insert()` 事务化并回填真实新渠道 ID，响应、ability 和审计一致。 |
| 2026-07-11 | Advanced Custom 请求路径感知选路 | `middleware/distributor.go`、`controller/relay.go`、`service/channel_select.go`、`model/channel_cache.go`、`model/ability.go`、`model/channel_path_select_test.go` | 原生化 new-api-main 的 type 58 path-aware routing：初选、亲和性复用、relay 重试、内存缓存和 DB fallback 均携带 `RequestPath`；仅 Advanced Custom 渠道按 `settings.advanced_custom` 的 `incoming_path` 过滤候选，普通渠道保持原分组/模型/状态语义。 |
| 2026-07-11 | 订阅套餐启停操作权限原生化 | `router/subscription-router.go`、`router/subscription_router_test.go`、`web/default/src/features/subscriptions/components/data-table-row-actions.tsx`、`web/default/src/features/subscriptions/components/dialogs/toggle-status-dialog.tsx` | 对齐 new-api-main 独立 `PATCH /api/subscription/admin/plans/:id` 状态接口的操作语义，并发展为 NexusTok 原生 Authz 边界：套餐启停从 `subscription.write` 收窄到 `subscription.operate`，编辑套餐内容仍保留 `subscription.write`，前端行操作和确认弹窗同步消费 operate 权限。 |
| 2026-07-11 | 渠道状态专用 API 与编辑解耦 | `controller/channel.go`、`controller/channel_authz.go`、`router/channel-router.go`、`web/default/src/features/channels/api.ts`、`web/default/src/features/channels/lib/channel-actions.ts` | 原生化 new-api-main 的渠道状态操作边界：新增 `POST /api/channel/:id/status` 与 `/api/channel/status/batch`，通用 `PUT /api/channel/` 拒绝 `status` 字段，前端单个/批量启停改走 `channel.operate` 专用接口；只允许手动启用/禁用，不允许伪造自动禁用状态。 |
| 2026-07-11 | Authz 角色策略审计摘要增强 | `controller/authz.go`、`controller/audit.go`、`middleware/audit.go`、`controller/authz_audit_test.go`、`middleware/audit_test.go` | 角色模板创建/更新/删除、角色策略 dry-run/apply 和策略导入成功后写入 `authz.*` 结构化管理审计摘要；失败路径通过中间件稳定 action 兜底，审计只记录角色 key、策略计数、dry-run/applied/reloaded 等非敏感摘要，不写完整 grants 矩阵。 |
| 2026-07-11 | Authz 默认前端角色策略编辑 UI | `web/default/src/features/system-settings/security/role-policy-section.tsx`、`role-policy-utils.ts`、`web/default/src/features/system-settings/api.ts`、`web/default/src/i18n/locales/*.json` | 在系统设置安全页新增 `role-policies` 分区，Root 可查看角色模板、编辑非 superuser 角色权限矩阵、先 dry-run 预览再二次确认保存；矩阵以后端 catalog 补齐完整 grants，动态 badge 和页面文案补齐六语。 |
| 2026-07-11 | Authz 角色策略编辑接口 | `service/authz/role_policy.go`、`controller/authz.go`、`router/authz-router.go`、`service/authz/role_policy_test.go` | 新增 Root-only `GET /api/authz/roles` 与 `PUT /api/authz/roles/:key/policies`；读取返回持久角色模板、superuser/runtime 标记和 grants 矩阵，更新默认 dry-run，显式 apply 后替换目标 `role:<key>` allow 策略并 reload 持久策略快照。 |
| 2026-07-11 | Authz 策略导入接口 | `service/authz/policy_import.go`、`controller/authz.go`、`router/authz-router.go`、`service/authz/policy_import_test.go` | 新增 Root-only `POST /api/authz/policies/import`，兼容导出 JSON，默认 dry-run；merge 补充角色/策略，replace 需确认并重建 role 策略与用户 override；导入 `user:<id>` 策略时同步 `authz_user_overrides` 和 `casbin_rule` 镜像，写入后 reload 持久策略快照。 |
| 2026-07-11 | Authz 策略导出接口 | `service/authz/policy_export.go`、`controller/authz.go`、`router/authz-router.go`、`service/authz/policy_export_test.go` | 新增 Root-only `GET /api/authz/policies/export`，导出 `authz_roles` 与 `casbin_rule` 的可迁移字段，不包含数据库自增 ID、创建时间和更新时间，用于审计、备份和导入前 diff。 |
| 2026-07-11 | Authz 用户 override Casbin 兼容镜像 | `model/authz_user_override.go`、`service/authz/override.go`、`service/authz/persistent_policy.go`、`model/authz_user_override_test.go`、`service/authz/override_test.go` | 将用户级 allow/deny override 继续保存在 `authz_user_overrides` 的同时，同步维护 `casbin_rule` 中的 `p, user:<id>, resource, action, effect` 镜像；当前授权读取仍以 legacy override 表为准，完整 Casbin runtime/enforcer 和角色编辑 UI 留作后续切片。 |
| 2026-07-11 | 账号池认证文件权限资源拆分 | `service/authz/*`、`router/account_pool-router.go`、`web/default/src/features/account-pool/*` | 将账号池认证文件从粗粒度 `account_pool` 中拆为独立 `account_pool_auth_file` 资源：认证文件列表/详情走 read，导入/更新/删除走 sensitive_write；默认 Admin 只保留读权限，凭证面板和添加账号的选择凭证弹窗按新资源开关请求，避免无权限时触发 403。 |
| 2026-07-11 | Authz 持久策略状态校准 | `docs/features/new-api-main-diff-analysis.md` | 基于当前 `service/authz/persistent_policy.go`、`model/authz_role.go`、`model/casbin_rule.go` 和 new-api-main `service/authz/enforcer.go` 重新校准 P1 队列：NexusTok 已完成角色/策略持久化底座；后续切片已继续补齐周期 reload 和用户 override 兼容镜像，剩余重点收窄为 Casbin runtime/enforcer、角色编辑 UI、自定义角色和策略导入导出。 |
| 2026-07-11 | 账号池 History 移动端筛选 Drawer | `web/default/src/features/account-pool/index.tsx`、`web/default/src/features/account-pool/components/account-pool-history-filter-drawer.tsx` | 原生化 new-api-main 的移动筛选收纳优势：账号池 Usage/State/Check History 手机端保留搜索框，将状态、action/source、request/time/account 等低频筛选收进 85dvh Drawer；State 使用原生 select 避免 Base Select portal 与 Vaul Drawer transform 冲突，Check status 使用按钮组。桌面筛选栏保留原样。 |
| 2026-07-10 | 差异报告优先队列状态校准 | `docs/features/new-api-main-diff-analysis.md` | 基于当前工作树证据校准早期总表、优先队列和路线建议；把已原生化的行锁、Responses/Gemini、管理审计、Flow、Usage Logs、Advanced Custom、Waffo Pancake、OpenAI image stream、用户级 Authz override 和多组权限路由从待办中移出，保留 Casbin/角色策略、ClickHouse 真接入、DataTable 分层、账号池日志体验等真实剩余项。 |
| 2026-07-10 | 订阅到期显式降级分组 | `model/subscription.go`、`model/main.go`、`controller/subscription.go`、`web/default/src/features/subscriptions/*`、`web/classic/src/components/table/subscriptions/*`、`web/default/src/i18n/locales/*.json` | 原生化 new-api-main 的 `downgrade_group` 能力：套餐可配置订阅结束后显式降级到指定分组；用户订阅购买时快照该策略，到期/作废/删除时显式降级优先，空值继续回退购买前分组，并保留其它有效升级订阅的分组权益。 |
| 2026-07-10 | 账号池分组字段级 fail-closed | `controller/account_pool.go`、`controller/account_pool_authz.go`、`controller/account_pool_authz_test.go`、`web/default/src/features/account-pool/*` | 账号池分组更新读取原始 JSON 并按字段分类做敏感写二次校验：`platform`、`auth_type`、`model_mapping`、`settings` 和未知字段默认需要 `account_pool.sensitive_write`；普通字段仍允许 `account_pool.write`。默认前端账号池页和凭证面板消费 `write/operate/sensitive_write`，编辑既有分组时禁用敏感字段。 |
| 2026-07-10 | 安全边界能力状态校准 | `docs/features/new-api-main-diff-analysis.md` | 复核匿名请求体限制、HeaderNavModuleAuth、受保护 Fetch/SSRF 客户端和 `/api/status/test` 权限路由现状，将报告中仍按“待增加”描述的安全能力更新为“已原生化首批 + 后续维护契约”，避免后续重复迁移。 |
| 2026-07-09 | models.dev 手动同步全量缺失补齐 | `controller/model_sync.go`、`controller/model_sync_test.go`、`web/default/src/features/models/api.ts`、`docs/features/model-sync-pricing.md` | 修复默认前端手动 models.dev 同步仍按旧 official 缺失能力表范围创建模型的问题；models.dev 来源默认按完整 catalog 补齐本地缺失模型，预览和实际同步目标一致，并保留 `create_all` 显式控制。回归覆盖 `gpt-5.6-luna`、`gpt-5.6-sol`、`gpt-5.6-terra` 三模型场景。 |
| 2026-07-09 | 前端依赖安全护栏 | `web/default/package.json`、`web/default/bun.lock` | 原生化 new-api-main 的前端 overrides 安全护栏，但不全量照搬；DOMPurify 升级到 `3.4.11`，并只锁定同主版本兼容的 `fast-uri`、`hono`、`js-cookie`、`mermaid`、`minimist`、`postcss` 和 `qs`，暂缓不兼容主版本或精确依赖路径的 `brace-expansion`、`uuid`、`ip-address`。 |
| 2026-07-09 | Relay HTTP 空闲连接超时配置 | `common/constants.go`、`common/init.go`、`.env.example`、`service/http_client.go`、`service/protected_fetch_client.go`、`service/http_client_test.go`、`service/protected_fetch_client_test.go` | 原生化 new-api-main 的 `RELAY_IDLE_CONN_TIMEOUT` 能力，默认 90 秒并允许 0 表示不限制；默认 Relay client、HTTP/HTTPS proxy、SOCKS5 proxy 和 SSRF protected fetch transport 均使用同一配置，测试覆盖配置传播。 |
| 2026-07-09 | 本地 MCP 与临时测试忽略项 | `.gitignore` | 原生化 new-api-main 的本地验证产物隔离策略，忽略 `.playwright-mcp`、`.local-tests/` 和明确命名的 chat responses live local 探针文件；同时覆盖 NexusTok 当前 `service/openaicompat` 原生命名路径，降低 MCP/真实上游临时验证文件误提交风险。 |
| 2026-07-09 | Secure Session Cookie 示例补齐 | `.env.example` | 补齐已经落地的 `SESSION_COOKIE_SECURE` 与 `SESSION_COOKIE_TRUSTED_URL` 示例说明，让 HTTPS 反代部署者能从环境样例发现 Secure Cookie 开关；不补尚未接线的 relay idle timeout 变量，不改变默认运行时行为。 |
| 2026-07-09 | Prettier 版权头保护包装器 | `web/default/scripts/format-with-protected-headers.mjs`、`web/default/package.json` | 原生化 new-api-main 的格式化版权头保护优势，但继续使用 NexusTok 当前 Prettier/ESLint/tsc 工具链；`format`/`format:check` 通过 wrapper 临时剥离并恢复 `c1cada` AGPL 版权头，check 模式会恢复快照并排除热更新构建目录，避免历史格式检查污染工作区。 |
| 2026-07-09 | 手动任意分支 Docker 预览镜像 | `.github/workflows/docker-image-branch.yml` | 原生化 new-api-main 的任意分支镜像发布优势，新增维护者手动输入分支名的 Docker Hub 多架构构建；统一发布 `branch-*` 和 `branch-*-YYYYMMDD-SHA` 标签，避免覆盖 `latest`、`alpha`、`nightly` 和正式版本 tag，并保留 SBOM/provenance 与 cosign 签名。 |
| 2026-07-09 | 多语言 README 品牌化补齐 | `README.md`、`README.en.md`、`README.zh_CN.md`、`README.zh_TW.md`、`README.fr.md`、`README.ja.md` | 原生化 new-api-main 的多语言 README 入口优势，按 NexusTok 当前能力生成英文、简中、繁中、法文、日文 README；保留本项目仓库、镜像、部署维护手册和合规提示，不迁移上游品牌、合作伙伴、徽章和外部文档站点。 |
| 2026-07-09 | 中文 i18n Quota 漏翻清理 | `web/default/src/i18n/locales/zh.json`、`web/default/src/i18n/locales/_reports/*` | 将 zh locale 中唯一剩余的 `Quota:` 翻译为 `额度：`，并通过 `bun run i18n:sync` 验证 stale untranslated report 清理能力；中文未翻译计数归零。 |
| 2026-07-09 | i18n 同步脚本字面量白名单与报告清理 | `web/default/scripts/sync-i18n.mjs`、`web/default/src/i18n/locales/_reports/*` | 原生化 new-api-main 的 i18n literal 白名单优势，过滤品牌、provider、URL、密钥占位符、模型名和代码化字符串的误报；同步脚本在 locale 无 extras/untranslated 时清理旧报告，剩余自然语言未翻译项继续保留给后续翻译切片。 |
| 2026-07-09 | Issue 模板前置约束增强 | `.github/ISSUE_TEMPLATE/{bug_report.md,bug_report_en.md,feature_request.md,feature_request_en.md}` | 原生化 new-api-main 的 issue 治理优势，四份模板补充非重复搜索、使用/配置/接入排除、Relay/pass-through 上游行为边界、coding plan/逆向渠道技术支持边界，以及转发/计费问题排查信息提示；保留 NexusTok 品牌链接和现有 Issue Chooser 配置。 |
| 2026-07-09 | DataTable 组件维护文档 | `web/default/src/components/data-table/README.md` | 原生化 new-api-main 的 DataTable 包说明优势，但不直接迁移其 `core/layout/toolbar/static/hooks` 目录；文档按 NexusTok 当前扁平组件结构记录稳定导入入口、组件职责、`DataTablePage` 组合方式、移动端列 meta 和后续渐进分层原则。 |
| 2026-07-09 | Electron 桌面端文档与构建脚本修复 | `electron/build.sh`、`electron/main.js`、`electron/package.json`、`electron/README.md` | 原生化 new-api-main 的 Electron 文档资产并修复 NexusTok 目录不匹配问题：桌面构建先分别生成 default/classic 前端 dist，再构建嵌入资源的 Go 二进制；开发模式前端端口与 `make dev-web` 对齐为 3001；移除不存在的 `../web/dist` 资源引用，并补充桌面端开发、打包、数据目录和排障说明。 |
| 2026-07-09 | 开发 Compose 与 Makefile 修补 | `docker-compose.dev.yml`、`makefile`、`docs/installation/deployment.md` | 原生化 new-api-main 的本地开发工程化优势：dev PostgreSQL 显式使用 `postgres:15-alpine`，新增 `dev-api-rebuild` 和开发专用 `reset-setup`，并在部署文档补充后端重建、初始化状态重置和生产禁用说明；不引入 oxlint/tsgo，不修改热更新部署。 |
| 2026-07-09 | 日志查询与 ClickHouse 准备层护栏 | `common/database.go`、`model/main.go`、`model/log.go`、`model/token.go`、`model/clickhouse_log_test.go` | 原生化 new-api-main 的 ClickHouse 日志准备层优势：主库 ClickHouse DSN fail-fast、当前构建对 `LOG_SQL_DSN` ClickHouse 给出明确未启用 driver 错误、日志 LIKE 过滤统一转义、日志写入补齐 `request_id`、ClickHouse 排序与 TTL SQL helper 进入测试契约；不引入 ClickHouse driver，不改变 SQLite/MySQL/PostgreSQL 主路径。 |
| 2026-07-09 | Chat 与 Responses relay handler 兼容原生化 | `service/openaicompat/responses_to_chat.go`、`service/openai_chat_responses_compat.go`、`relay/chat_completions_via_responses.go`、`relay/channel/openai/chat_via_responses.go` | 原生化 new-api-main 的 Responses SSE -> Chat chunks 状态机、buffered SSE -> Chat JSON、Chat SSE -> Responses SSE 和大小写不敏感的 event-stream 判断；客户端非流式请求遇到上游 Responses SSE 时返回普通 JSON，流式路径保留 tool_calls、reasoning、usage 和终态事件顺序。 |
| 2026-07-09 | Responses 请求转 Chat 兼容测试 | `service/openaicompat/responses_request_to_chat_test.go` | 原生化 new-api-main `service/relayconvert` 的 Responses request 降级 Chat 回归测试资产；覆盖 instructions/scalar 字段、显式零值、stream options、多模态 file/audio/video、assistant text 与 function_call 共存、only function_call 自动构造 assistant、tools/tool_choice/text.format 和 custom_tool_call 原始 shape，确认 NexusTok `openaicompat` 已具备对应原生能力。 |
| 2026-07-09 | Claude OpenAI 文件内容转换语义修复 | `relay/channel/claude/relay-claude.go`、`relay/channel/claude/message_delta_usage_patch_test.go` | 修复仓库级测试暴露的 Claude 文件转换缺口；OpenAI `file` 内容按 filename 扩展名转换，`.txt` 解码为 Claude text，`.pdf` 转 document，图片转 image，未知二进制跳过，避免把 unsupported file 错误包装成 image 发给上游；同时补齐 message_delta usage patch 测试 imports。 |
| 2026-07-09 | 用户更新并发字段保护与邮箱唯一性 | `model/{user.go,errors.go,user_update_test.go}`、`controller/{user.go,misc.go,subscription.go}` | 原生化 new-api-main 的用户更新安全回归能力；用户资料更新不再覆盖并发变化的 `quota`、`used_quota`、`request_count`，用户设置/语言/sidebar/订阅计费偏好只写 `setting` 列，邮箱注册/绑定/找回密码统一规范化并按大小写不敏感唯一性校验，passwordless 用户保持空密码且密码登录明确拒绝。 |
| 2026-07-09 | 计费表达式结算 clamp 回归测试 | `pkg/billingexpr/settle_clamp_test.go` | 原生化 new-api-main 的分层计费结算饱和测试资产；锁定异常超大表达式结算必须饱和到 int32 上限并返回 `Clamp`，正常范围结算不误报 `Clamp`，防止动态计费后续调整破坏安全审计不变量。 |
| 2026-07-09 | SMTP STARTTLS 与 TLS 校验配置 | `common/{email.go,email_test.go,constants.go,init.go}`、`model/{option.go,option_bulk_test.go}`、`web/default/src/features/system-settings/{types.ts,operations/*,integrations/email-settings-section.tsx}`、`web/default/src/i18n/locales/*.json` | 原生化 new-api-main 的 SMTP 连接层安全与兼容能力；新增显式 STARTTLS、默认 TLS 证书校验、管理员可选跳过校验、465 隐式 TLS 兼容和空凭证跳过 AUTH，并在默认前端 SMTP 设置页提供原生开关。 |
| 2026-07-09 | 常量包边界文档 | `constant/README.md` | 原生化 new-api-main 的 `constant` 包维护约定；按 NexusTok 当前文件列表记录职责说明，并明确常量包禁止承载业务流程、数据库操作、第三方调用和项目内反向依赖。 |
| 2026-07-09 | 监控配置环境变量覆盖测试 | `setting/operation_setting/monitor_setting_test.go` | 原生化 new-api-main 的渠道自动测试配置回归保护；测试覆盖 `CHANNEL_TEST_FREQUENCY`、`CHANNEL_TEST_ENABLED=false/true` 的优先级和默认 `scheduled_all` 模式，防止后续 Routing Reliability 或 SystemTask 调整误改环境变量语义。 |
| 2026-07-09 | 兑换码状态筛选与兑换 CAS | `model/redemption.go`、`model/redemption_test.go`、`controller/redemption.go`、`web/default/src/features/redemption-codes/{api.ts,types.ts,components/redemptions-table.tsx}` | 原生化 new-api-main 的兑换码服务端状态筛选与兑换幂等保护；搜索接口支持 `status` 参数和 `expired` 虚拟状态，默认前端状态筛选走后端分页，`Redeem` 通过状态 CAS 确保同一兑换码并发场景只入账一次。 |
| 2026-07-09 | 节点身份公共化 | `common/node_identity.go`、`common/constants.go`、`common/init.go`、`service/system_instance.go`、`service/system_instance_test.go` | 原生化 new-api-main 的节点身份解析能力；`common.NodeName` 优先使用 `NODE_NAME`，未配置时回退 hostname，并暴露来源和手动配置标记；系统实例心跳复用公共身份，日志、用量导出和 SystemTask runner 共享同一节点名语义。 |
| 2026-07-09 | 表格展示原子组件 | `web/default/src/components/{table-id.tsx,provider-badge.tsx,truncated-text.tsx,long-text.tsx}`、`web/default/src/features/{channels,models}/components/*-columns.tsx` | 原生化 new-api-main 的表格 ID、provider badge 和长文本截断优势；新增公共 `TableId`、`ProviderBadge`、`TruncatedText`，并让 `LongText` 支持响应式 overflow 重新测量；渠道和模型两张高频表先行接入，保留账号池、多 Key、IO.NET、上游模型同步、权限动作、筛选和 API 语义不变。 |
| 2026-07-09 | 模型倍率 JSON 模式 CodeMirror 编辑器 | `web/default/src/components/json-code-editor.tsx`、`web/default/src/components/ai-elements/code-block.tsx`、`web/default/src/features/system-settings/models/model-ratio-form.tsx` | 原生化 new-api-main 的 JSON 结构化编辑体验；新增公共 `JsonCodeEditor` 复用现有 CodeMirror 底座，模型倍率 JSON 模式从普通 `Textarea` 切换为带行号、状态和格式化动作的编辑器，同时保持八个倍率字段、保存校验、计费表达式语义和提交接口不变。 |
| 2026-07-09 | 渠道移动端专用卡片 | `web/default/src/features/channels/components/{channel-card.tsx,channels-table.tsx}` | 原生化 new-api-main 的渠道移动端卡片体验；移动端列表改用专用卡片重组类型、名称、状态、余额、优先级、权重、响应时间、最近测试、分组和行操作，继续复用现有列 cell、权限动作、tag 聚合和桌面表格逻辑。 |
| 2026-07-09 | Playground CodeMirror 编辑器底座 | `web/default/src/components/ai-elements/code-block.tsx`、`web/default/src/features/playground/components/playground-message-editor.tsx`、`web/default/package.json`、`web/default/bun.lock` | 原生化 new-api-main 的 CodeMirror 结构化编辑能力；新增公共 `CodeBlockEditor`，Playground 消息编辑器切换为 Markdown/行号编辑器，并把键盘事件与 `EditorView` 生命周期解耦，保留 Shiki 只读代码块展示和现有保存/确认/Reset 语义。 |
| 2026-07-09 | Playground 消息编辑器状态安全 | `web/default/src/features/playground/components/playground-message-editor.tsx`、`web/default/src/features/playground/lib/{message-editor-utils.test.ts,message-styles.ts,message-styles.test.ts}` | 原生化 new-api-main 的编辑器误取消保护、重置动作、图标动作和编辑态宽度对齐；继续使用现有 `Textarea`，不引入 CodeMirror 依赖，保存、Save & Submit、消息数组和请求协议保持不变。 |
| 2026-07-09 | Playground 消息阅读样式 | `web/default/src/features/playground/lib/{message-styles.ts,message-styles.test.ts}` | 原生化 new-api-main 的消息阅读样式优势；assistant 正文限制为 `78ch` 文档列并使用当前 `font-sans` 字体轴，user 消息切换为语义 muted 边框气泡，移除旧 `font-serif`、无限宽和手写深色背景覆盖，消息协议、存储和请求链路不变。 |
| 2026-07-09 | Playground 错误状态 helper | `web/default/src/features/playground/components/message-error.tsx`、`web/default/src/features/playground/lib/{message-error-utils.ts,message-error-utils.test.ts,index.ts}` | 原生化 new-api-main 的 message error state helper；错误分类、模型价格错误、管理员设置入口可见性和 fallback 文案进入可测试纯函数，`MessageError` 只保留 Alert/Button 渲染，现有错误动作位置、请求链路和 i18n key 不变。 |
| 2026-07-09 | Playground 消息流式状态工具 | `web/default/src/features/playground/hooks/use-chat-handler.ts`、`web/default/src/features/playground/lib/{message-streaming-utils.ts,message-streaming-utils.test.ts,index.ts}` | 原生化 new-api-main 的 message streaming helper 分层；流式 reasoning/content 增量、累计 chunk 去重、assistant 完成态判断和非流式 choice 写回进入可测试纯函数，`use-chat-handler` 只保留请求生命周期、toast 和错误落盘编排。 |
| 2026-07-09 | Playground 消息布局对齐底座 | `web/default/src/features/playground/types.ts`、`web/default/src/features/playground/components/{playground-chat.tsx,playground-message-content.tsx,message-metadata.tsx}`、`web/default/src/features/playground/lib/{message-layout-utils.ts,message-layout-utils.test.ts,index.ts}` | 原生化 new-api-main 的 `PlaygroundMessageLayoutMode` 和消息 alignment helper；默认保持 alternating，user 消息右对齐、assistant/system 左对齐，并为后续全部左对齐或用户偏好设置提供原生底座；保留 Branch 多版本、消息动作、请求协议和 localStorage schema。 |
| 2026-07-09 | Playground 移动端消息动作菜单 | `web/default/src/features/playground/components/message-actions.tsx`、`web/default/src/features/playground/hooks/use-message-action-guard.ts`、`web/default/src/features/playground/lib/{message-action-utils.ts,message-action-utils.test.ts}`、`web/default/src/i18n/locales/*.json`、`web/default/src/i18n/static-keys.ts` | 原生化 new-api-main 的移动端消息动作菜单；单条消息动作先收敛为同一动作数组，桌面继续显示 tooltip 图标组，移动端折叠到 `DropdownMenu`；user/assistant 完成态消息均可展示 regenerate，动作 label 与 toast 补齐 i18n。 |
| 2026-07-09 | Playground 推理耗时展示 | `web/default/src/components/ai-elements/reasoning.tsx`、`web/default/src/features/playground/components/playground-message-content.tsx`、`web/default/src/i18n/locales/*.json`、`web/default/src/i18n/static-keys.ts` | 原生化 new-api-main 的 reasoning duration 展示闭环；Playground 将已记录的 `message.reasoning.duration` 传入公共 `Reasoning`，trigger 按流式、未知耗时和精确秒数展示六语 i18n 文案，旧消息和 0 秒耗时仍回落泛化提示。 |
| 2026-07-09 | Playground 原始响应切换 | `web/default/src/features/playground/components/{message-actions.tsx,playground-chat.tsx,playground-message-content.tsx}`、`web/default/src/features/playground/lib/{message-action-utils.ts,message-action-utils.test.ts,message-source-utils.ts,message-source-utils.test.ts,index.ts}`、`web/default/src/features/playground/constants.ts`、`web/default/src/i18n/locales/*.json`、`web/default/src/i18n/static-keys.ts` | 原生化 new-api-main 的 assistant 消息 source/preview toggle；完成态 assistant 消息可在 Markdown 预览和当前版本原始响应代码块之间切换，source 状态仅保留在页面会话中，不改消息协议和 localStorage；新增 raw/source 文案补齐六语。 |
| 2026-07-09 | Playground 消息基础工具拆分 | `web/default/src/features/playground/lib/{message-reasoning-utils.ts,message-reasoning-utils.test.ts,message-update-utils.ts,message-update-utils.test.ts,message-action-utils.ts,message-action-utils.test.ts,message-utils.ts,message-content-utils.ts,storage.ts,index.ts}`、`web/default/src/features/playground/components/message-actions.tsx` | 原生化 new-api-main 的 message reasoning/update/action 工具拆分；`<think>` 解析、assistant 错误更新、动作栏状态和可见性 class 进入可测试纯函数，MessageActions 只消费派生状态，现有按钮结构、请求协议和本地存储 key 保持不变。 |
| 2026-07-09 | Playground 选项加载与会话 hook | `controller/user.go`、`controller/user_models_test.go`、`web/default/src/features/playground/{api.ts,index.tsx,hooks/use-playground-{options,conversation,state}.ts,hooks/index.ts,lib/playground-{option,state}-utils.ts,lib/playground-{option,state}-utils.test.ts,lib/index.ts}`、`web/default/src/i18n/locales/*.json` | 原生化 new-api-main 的 Playground options/conversation 拆分；`/api/user/models?group=` 支持按用户可用分组过滤模型，不可用分组返回空数组，无参数保持旧聚合行为；前端按当前分组刷新模型候选、加载失败 toast 补齐六语，发送/重试/编辑/删除逻辑收敛到专用 hook。 |
| 2026-07-09 | Playground 本地存储 schema 与容量保护 | `web/default/src/features/playground/lib/{storage-schema.ts,storage.ts,storage.test.ts}` | 原生化 new-api-main 的 Playground 本地存储 envelope、Zod schema 校验和容量保护；保存继续使用原 storage key，读取兼容旧裸 JSON，损坏/超大/流式中断消息会被裁剪、清理或稳定化。 |
| 2026-07-09 | Playground 消息时间元信息 | `web/default/src/features/playground/types.ts`、`web/default/src/features/playground/lib/{message-timing-utils.ts,message-timing-utils.test.ts,message-utils.ts,conversation-message-utils.ts,conversation-message-utils.test.ts,index.ts}`、`web/default/src/features/playground/hooks/use-chat-handler.ts`、`web/default/src/features/playground/components/{message-metadata.tsx,playground-message-content.tsx}`、`web/default/src/i18n/locales/*.json` | 原生化 new-api-main 的消息 `createdAt`、`startedAt`、`completedAt`、`durationMs` 和 `MessageMetadata` 能力，新消息记录发送时间与响应耗时，流式/非流式/停止/错误路径统一补完成时间；旧 localStorage 消息缺 timing 字段时不渲染元信息，避免存储迁移。 |
| 2026-07-09 | Playground 消息内容渲染组件化 | `web/default/src/features/playground/components/{playground-message-content.tsx,playground-chat.tsx}`、`web/default/src/features/playground/lib/{message-content-utils.ts,message-content-utils.test.ts,index.ts}`、`web/default/src/i18n/locales/*.json` | 原生化 new-api-main 的消息内容拆分方式，来源、推理、加载、错误和正文显示状态统一由 helper 计算，单条消息正文渲染交给 `PlaygroundMessageContent`，聊天组件只保留消息列表、编辑态和动作编排；`Responding...` 补齐六语翻译。 |
| 2026-07-09 | Playground 消息编辑与会话操作工具化 | `web/default/src/features/playground/components/{playground-message-editor.tsx,playground-chat.tsx}`、`web/default/src/features/playground/lib/{message-editor-utils.ts,conversation-message-utils.ts,conversation-message-utils.test.ts,index.ts}`、`web/default/src/features/playground/index.tsx`、`web/default/src/i18n/locales/*.json` | 原生化 new-api-main 的消息编辑器拆分和会话消息数组 helper；发送、重试、删除、上一条 user 查找、编辑保存与编辑后重新提交统一由纯函数处理并补定向测试，聊天组件和容器不再手写关键数组截断逻辑。 |
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
| 2026-07-09 | 渠道编辑页分段布局与模型搜索添加 | `web/default/src/components/multi-select.tsx`、`web/default/src/components/drawer-layout.ts`、`web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`、`web/default/src/i18n/locales/*.json` | 默认前端渠道编辑抽屉对齐 new-api-main 最新分段体验：5xl 右侧抽屉、左侧状态导航、基本信息/凭证/模型与分组/高级设置锚点、固定底部操作；模型多选改为 Base UI Combobox 芯片多选，支持显式过滤、自定义模型、多值粘贴、chip 收敛，并按关键词查询 `/api/models/search` 合并模型元信息候选，修复 `gpt-5.6` Luna/Terra/Sol 不能稳定搜索添加的问题。 |
| 2026-07-10 | 渠道模型搜索追加弹层关闭与编辑页导航对齐 | `web/default/src/components/multi-select.tsx`、`web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 运行态复现确认 `gpt-5.6` 后端搜索和追加数据正确，但批量追加后 Combobox 因搜索词清空仍保持打开并铺出完整模型库；本轮让 `MultiSelect` 支持可选受控 open，渠道模型选择器在 `Add {{count}} new model(s)` 后立即关闭弹层，并补齐 new-api 编辑页左侧导航 sticky 层级。 |
| 2026-07-10 | 结果媒体代理 SSRF Dial 阶段补强 | `controller/video_proxy.go`、`relay/mjproxy_handler.go`、`docs/features/new-api-main-diff-analysis.md` | 继续执行用户可控 URL 覆盖缺口审计，确认下载、Webhook、Bark/Gotify 已走 protected client；本轮补齐视频代理和 Midjourney 图片代理直连路径，URL 预校验后实际请求也使用 `GetSSRFProtectedHTTPClient()`，通过 Dial 阶段解析 IP 校验抵御 DNS rebinding；显式渠道代理继续保留一次性 URL 校验。 |
| 2026-07-10 | Uptime Kuma 状态接口 SSRF 防护补强 | `controller/uptime_kuma.go`、`controller/uptime_kuma_test.go`、`docs/features/new-api-main-diff-analysis.md` | 公开 `/api/uptime/status` 会根据系统设置拉取 Uptime Kuma 状态页和心跳 API；本轮在每个最终 URL 发出前接入 FetchSetting 预校验，实际请求改用 `GetSSRFProtectedHTTPClient()` 的 Dial 阶段校验并保留 10 秒单请求超时，同时补齐私网目标请求前拦截与关闭 SSRF 后兼容的回归测试。 |
| 2026-07-10 | 自定义 OAuth Discovery SSRF 防护补强 | `controller/custom_oauth.go`、`controller/custom_oauth_test.go`、`docs/features/new-api-main-diff-analysis.md` | Root 可操作的 `/api/custom-oauth-provider/discovery` 会根据管理员输入的 well-known/issuer URL 拉取 OIDC discovery；本轮在基础 http/https 校验后接入 FetchSetting 预校验，实际请求改用 protected client 的 Dial 阶段校验并保留 20 秒超时，测试覆盖私网目标请求前拦截和关闭 SSRF 后兼容行为。 |
| 2026-07-10 | OAuth 登录端点 SSRF 防护补强 | `oauth/outbound_http.go`、`oauth/generic.go`、`oauth/oidc.go`、`oauth/outbound_http_test.go`、`docs/features/new-api-main-diff-analysis.md` | 活跃 `/api/oauth/:provider` 登录回调中，系统 OIDC 与自定义 OAuth 的 token/userinfo endpoint 来自管理员配置；本轮在 token 和 userinfo 请求前接入 FetchSetting 预校验，实际请求复用 protected client 的 Dial 阶段校验，保留 OIDC 5 秒和 Generic 20 秒超时，并用测试确认私网端点在请求前被拦截。 |
| 2026-07-10 | 旧 OIDC 控制器安全基线补齐 | `controller/oidc.go`、`controller/oidc_legacy_test.go`、`docs/features/new-api-main-diff-analysis.md` | `controller.OidcAuth/OidcBind` 当前未被后端路由直接注册，但仍在 controller 包编译范围内；本轮防御性补齐 token/userinfo endpoint 的 FetchSetting 预校验、protected client 实际请求和 `common.DecodeJson` 解码，避免未来恢复旧路由时重新引入 SSRF 或 JSON 封装回退。 |
| 2026-07-09 | Authz 用户级 override 基础层 | `model/authz_user_override.go`、`model/main.go`、`service/authz/*`、`controller/user.go`、`middleware/authz_test.go`、`controller/user_authz_test.go` | 原生化 new-api-main 的用户级 allow/deny override 优势，但不直接引入 Casbin；新增三库兼容 `authz_user_overrides` 表，Admin 授权先读用户 override 再回退角色基线，Root 不受 deny 影响，普通用户不能被 override 提升，`/api/user/self` 回传与服务端 `Can` 共享同一权限语义。 |
| 2026-07-09 | 兑换码完整值安全查看 | `model/redemption.go`、`controller/redemption.go`、`router/redemption-router.go`、`service/authz/*`、`web/default/src/features/redemption-codes/*`、`web/default/src/i18n/locales/*.json` | 兑换码列表、搜索、详情和更新响应默认返回脱敏 `key` 与 `key_redacted=true`；新增 `POST /api/redemption/:id/key`，通过 `redemption.secret_view`、关键限流、禁缓存和安全验证 reveal 完整码；默认前端行内查看/复制与批量复制改为按需 reveal，避免普通 read 权限泄露可兑换额度凭据。 |

## 本轮实施评审：渠道编辑页分段布局与模型搜索添加修复

### 需求分析

当前默认前端渠道编辑页的“模型”多选输入在输入 `gpt-5.6` 时，候选列表仍优先展示 `gpt-3.5-*`、`gpt-5.4*` 等无关模型，无法稳定找到已从 models.dev 同步到模型元信息库的 `gpt-5.6-luna`、`gpt-5.6-sol`、`gpt-5.6-terra`。这会阻断管理员把已存在的模型元信息添加到渠道能力中的关键路径。

对照 `/opt/project/new-api-main` 最新默认前端，编辑渠道页面已经升级为更清晰的右侧大抽屉：顶部展示 provider 图标和标题，主体采用“基本信息 / 凭证 / 模型与分组 / 高级设置”的分段结构，左侧提供状态导航，底部固定取消和保存按钮。该体验适合吸收为 NexusTok 原生能力，但不能替换 NexusTok 现有的权限矩阵、敏感字段裁剪、账号池组、多 Key、模型映射校验、状态码风险确认和安全验证逻辑。

### 影响范围分析

- `web/default/src/components/multi-select.tsx`：修复多选搜索过滤，增强为支持内联创建、自定义值、多值粘贴和 chip 数量收敛的可复用控件。
- `web/default/src/components/drawer-layout.ts`：新增通用侧边编辑抽屉布局 helper，沉淀 new-api-main 的页面结构优势，后续其他抽屉可复用。
- `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`：仅调整默认前端渠道编辑抽屉的布局骨架、分段导航和模型候选来源，保留表单字段、提交 payload 转换和权限控制。
- `web/default/src/features/models/api.ts`、`web/default/src/features/models/types.ts`：复用现有 `/api/models/search` 模型元信息接口，不新增后端路由。
- `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`：补齐新增界面文案六语翻译。

### 风险评估

- 渠道编辑表单字段多且存在敏感配置，改动不得破坏 `channel.write` 与 `channel.sensitive_write` 的前端裁剪语义，也不得绕过后端敏感字段 fail-closed 校验。
- 模型搜索需要区分“模型元信息库”和“渠道能力列表”：搜索候选可以来自 `/api/models/search`，但选择候选只写入当前渠道的 `models` 字段，不自动创建模型、不自动修改 vendor、endpoint、group 或计费配置。
- `getAllModels()` 仍来自 `/api/channel/models`，用于快捷填充和兼容旧能力列表；远程模型元信息搜索只作为候选增强，接口失败时必须降级为本地候选，不阻断渠道编辑。
- 页面布局对齐 new-api-main 时不能复制其品牌、权限模型和不适配 NexusTok 的字段；需要保留账号池组模式、多 Key、Codex OAuth、Doubao 隐藏解锁、上游模型检测等 NexusTok 现有能力。
- MultiSelect 是共享组件，增强时必须保持旧调用兼容：默认不允许创建自定义项，已有 `maxVisibleChips`、`renderSelectedSummary` 行为不能回退。

### 方案评审

采用低风险原生化方案：先增强共享 `MultiSelect`，显式按输入过滤候选并支持 `allowCreate`、逗号/换行批量添加、chip 收敛和点击复制；再在渠道编辑抽屉中接入 `/api/models/search?keyword=` 的防抖远程查询，把模型元信息候选与 `/api/channel/models`、当前渠道历史模型合并去重。输入 `gpt-5.6` 时应能直接展示 `gpt-5.6-luna`、`gpt-5.6-sol`、`gpt-5.6-terra`。

渠道编辑页布局吸收 new-api-main 的右侧 Drawer 结构：新增顶部 provider 标识、主体两列布局、左侧状态导航、右侧分段表单和固定底部按钮。高级设置保持现有折叠区，左侧导航点击高级项时自动展开并滚动定位。提交逻辑、表单 schema、权限判断、敏感字段裁剪、风险确认弹窗和现有接口保持原样，降低核心业务回归风险。

### 实施结果

- `MultiSelect` 从旧的 `cmdk` 下拉升级为项目 Base UI Combobox 芯片多选，显式过滤输入值；保留旧调用兼容，同时新增 `allowCreate`、`onSearchChange`、`isLoading`、`copyChipOnClick` 和多值粘贴能力。
- 渠道编辑抽屉新增模型元信息远程候选：输入关键词后防抖调用 `/api/models/search`，并按“搜索结果、静态能力列表、当前已选模型”的顺序合并候选；选择后只更新渠道 `models` 字段，不自动创建模型或修改供应商/计费/分组。
- 渠道编辑抽屉对齐 new-api-main 最新布局：宽度提升为 `sm:max-w-5xl`，左侧增加 provider 摘要与状态导航，右侧保留现有字段并挂载 `Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings` 锚点，高级子项可定位到路由策略、内部备注、覆盖规则、额外设置、字段透传和上游模型检测。
- 保留 NexusTok 原有能力边界：`channel.write/channel.sensitive_write` 前端权限裁剪、账号池组模式、多 Key 模式、Codex OAuth、密钥安全验证、模型映射风险确认、状态码风险确认和提交 payload 转换均未替换。

### 验证记录

- `cd web/default && bun run i18n:sync`：通过；六语 locale `missingCount=0`，本轮新增 key 已补齐。
- `cd web/default && bun run typecheck`：通过。
- `cd web/default && bun run build`：通过。
- `git diff --check`：通过。
- MCP 浏览器验证：本轮工具可发现 Chrome DevTools MCP，但连接 `http://127.0.0.1:9222/json/version` 失败，错误为无法连接 Chrome 调试实例；因此使用真实 3003 HTTP 请求作为替代验证。
- `GET http://192.168.0.202:3003/`：返回 200，页面 HTML 正常。
- `POST http://192.168.0.202:3003/api/user/login?turnstile=`：账号 `c1cada` 登录成功。
- `GET /api/models/search?keyword=gpt-5.6&p=1&page_size=20`：返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三条模型元信息，`vendor_id=1`，Sol 已绑定 `openai/default`，Luna/Terra 保持仅元信息状态。
- `GET /api/channel/models`：确认静态能力列表仍以 `gpt-3.5-*` 等旧模型开头，证明截图问题源于旧候选源；前端已改为额外合并 `/api/models/search` 结果。
- `GET /static/js/async/2817.js`：返回 200，且服务端当前 chunk 包含 `channel_model_meta_search`、`Searching model metadata`、`Add custom model`，确认 3003 页面资源已热更新生效，无需重启容器。

### 补充修复：搜索匹配时错误创建自定义模型

用户复测后指出“搜索添加时不正确”。重新在 3003 打开渠道编辑页复现：`/api/models/search?keyword=gpt-5.6&p=1&page_size=50` 已正确返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三条模型元信息，但 `MultiSelect` 的“创建自定义值”判断只检查输入是否与候选完全相等。由于输入 `gpt-5.6` 并不等于任一具体变体，前端仍展示 `Add custom model "gpt-5.6"`，管理员快速按 Enter 或点击创建项时会把不存在的基础名加入渠道能力，偏离“根据已有模型添加能力”的目标。

影响范围限定在默认前端：

- `web/default/src/components/multi-select.tsx`：创建自定义值前新增“是否存在匹配候选”的判断，并在远程搜索加载中禁用创建项，避免真实候选返回前误创建。
- `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`：对齐 `/opt/project/new-api-main` 最新编辑渠道页的分段说明和模型区操作结构，新增 Basic/Credentials/Models 描述，并把填入、拉取、复制、清空和预设组操作独立为 `Quick actions` 操作区。
- `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json`：补齐新增说明文案六语翻译。

风险评估：该修复不改变后端同步、模型元信息、渠道保存 payload 和权限裁剪逻辑；只改变模型多选的候选渲染和编辑抽屉布局。`allowCreate` 仍然保留，但只有在没有任何匹配候选且搜索不在加载中时才出现，因此自定义模型能力不被移除，错误创建 `gpt-5.6` 这类系列前缀的风险被降低。

验证记录：

1. `cd web/default && bun run i18n:sync` 通过；`en/zh` missing 与 untranslated 均为 0，`fr/ja/ru/vi` 无 missing，本轮新增 key 已翻译，剩余 untranslated 为历史技术项。
2. `cd web/default && bun run typecheck` 通过。
3. MCP 打开 `http://192.168.0.202:3003/channels`，强制刷新后打开渠道 `11111` 的编辑抽屉，页面已显示 `Name, provider type, and availability.`、`Published models, groups, and model remapping rules.` 和 `Quick actions`，确认热更新已生效，无需重启容器。
4. 在模型输入框输入 `gpt-5.6`，网络请求 `GET /api/models/search?keyword=gpt-5.6&p=1&page_size=50` 返回 200，响应包含 Terra/Luna/Sol 三条模型。
5. 直接读取下拉 DOM，候选为 `["gpt-5.6-terra","gpt-5.6-luna","gpt-5.6-sol"]`，`hasCustomCreate=false`，确认不再展示错误的 `Add custom model "gpt-5.6"`。
6. 点击 `gpt-5.6-terra` 后表单 chip 计数从 `Selected 3` 变为 `Selected 4`，新增 chip 为 `gpt-5.6-terra`；随后点击 Cancel 关闭抽屉，网络记录中没有 `PUT /api/channel/` 保存请求，未改动运行态渠道数据。
7. MCP console error/warn 为空。

### 补充修复：渠道模型搜索结果批量追加与 new-api-main 模型区操作对齐

用户继续反馈“搜索添加时不正确”，并要求编辑渠道页面继续向最新版 `/opt/project/new-api-main` 的编辑渠道页对齐。重新在 3003 真实页面复现后确认：输入 `gpt-5.6` 时，运行页 DOM 已能展示 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个模型元信息候选，且不再展示错误的 `Add custom model "gpt-5.6"`；但当前交互仍只能逐个点选候选，用户在“根据已有模型添加到渠道能力”的场景下容易误以为搜索添加只同步/添加了其中一个模型。

#### 需求分析

- 渠道编辑页需要把“搜索到的已同步模型”转化为可直接追加到当前渠道 `models` 字段的批量操作，尤其覆盖 `gpt-5.6` Luna/Sol/Terra 这种同系列模型。
- 批量添加只应使用 `/api/models/search` 返回的真实 `model_name`，不能把搜索关键词本身写入渠道能力。
- 页面继续吸收 new-api-main 最新模型区的低风险交互：映射目标误暴露时可一键移除，映射源缺失时可一键追加，预设组从普通快捷按钮堆中独立展示。
- 改动必须保持未保存状态只修改前端表单，不主动调用渠道更新接口；最终仍由用户点击保存触发原有提交、权限和风险确认链路。

#### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 新增搜索结果去重、批量追加按钮、搜索命中数量提示、映射风险快捷修复、预设组独立展示。 |
| 前端 i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 补齐 7 个新增文案，保持六语 locale `missingCount=0`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验证结果。 |

#### 风险评估

- 不改后端同步、模型元信息、渠道能力保存接口、数据库结构和计费逻辑，核心业务风险低。
- 批量追加按钮使用 `modelSearchAddableNames`，先从搜索接口结果中 trim、去空、去重，再排除当前已选模型；因此不会重复追加，也不会把 `gpt-5.6` 这类系列前缀误写入。
- `/api/models/search` 可能命中 description/tags，本轮进一步限定批量追加只接收 `model_name` 本身包含当前关键词的结果，避免描述或标签误命中被加入渠道能力。
- 按钮受 `canEditBasicFields` 控制，和手动选择模型、快捷填充模型保持同一权限边界。
- 输入值等待 debounce 生效期间也视为搜索中，MultiSelect 不展示自定义创建项，避免远程候选返回前按 Enter 抢先创建裸模型名。
- 映射目标移除和缺失源追加只改当前表单值，仍需用户显式保存；如果用户取消抽屉，不会产生 `PUT /api/channel/` 请求。
- 搜索接口失败或无结果时，页面仍保留现有本地候选和自定义添加能力，不阻断渠道编辑。

#### 方案评审

采用前端最小闭环方案：复用现有 `searchModels` 查询结果，新增 `modelSearchAddableNames` 派生状态和 `handleAddModelSearchResults` 事件。模型输入框下方在有搜索关键词时展示“Search results”状态条，显示搜索命中数量，并提供“Add {{count}} search result(s)”按钮一次性追加所有尚未加入的真实模型。该方案不引入新的后端 API，不改变 MultiSelect 的共享行为，避免影响其他多选场景。

页面对齐方面，只吸收 new-api-main 中与模型区稳定性直接相关的低风险交互：`Remove mapped targets`、`Add missing models`、`Preset groups` 独立展示和空模型时禁用 `Copy All`。暂不迁移 new-api-main 的 `ChannelModelsSection` 拆分和更深层映射编辑器候选能力，因为当前 NexusTok 仍有账号池组、Codex OAuth、权限裁剪等本地差异，直接重构会扩大风险。

#### 实施结果

- 输入 `gpt-5.6` 后，渠道编辑页会显示搜索命中数量，例如 `3 synced model(s) matched "gpt-5.6"`。
- 搜索结果面板会展示即将追加的前 6 个具体模型名，超过 6 个时显示 `+N more`，管理员可以在点击前确认会写入哪些能力。
- 当三条候选中只有 `gpt-5.6-sol` 已在当前渠道里时，新增按钮显示可追加 2 条搜索结果；点击后会把 `gpt-5.6-terra`、`gpt-5.6-luna` 追加到表单，并保留已有 `gpt-5.6-sol`。
- 如果全部搜索结果都已存在，按钮禁用，并在调用时提示 `No new search results to add`。
- 模型映射风险提示补齐两个快捷动作：移除已暴露的上游目标模型、追加缺失的映射源模型。
- 预设组按钮从 `Quick actions` 主操作堆中分离，减少填充/拉取/复制/清空等命令和预设项混在一起的误操作概率。

#### 验证记录

1. `cd web/default && bun run typecheck`：通过。
2. `cd web/default && bun run build`：通过。
3. `cd web/default && bun run i18n:sync`：通过；`en/zh/fr/ja/ru/vi` 均无缺失 key，本轮新增 7 个文案已补齐六语翻译。
4. `git diff --check`：通过。
5. MCP 打开并强刷 `http://192.168.0.202:3003/channels`，页面资源已包含本轮新版渠道编辑代码，未触发容器重启。
6. 在渠道 `11111` 的编辑抽屉中输入 `gpt-5.6`，页面显示 `Search results`、`3 synced model(s) matched "gpt-5.6"`，预览区展示 `gpt-5.6-terra`、`gpt-5.6-luna`，按钮显示 `Add 2 search result(s)`。
7. 网络请求 `GET /api/models/search?keyword=gpt-5.6&p=1&page_size=50` 返回 3 条真实模型名：`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`；其中 `gpt-5.6-sol` 已绑定当前渠道，Terra/Luna 未绑定。
8. 点击 `Add 2 search result(s)` 后，toast 显示 `Added 2 model(s) from search`，表单选择数量从 `Selected 3` 变为 `Selected 5`，新增 chip 为 `gpt-5.6-terra`、`gpt-5.6-luna`，按钮随即变为 `Add 0 search result(s)` 并禁用。
9. 验证过程中没有出现 `PUT /api/channel/` 请求，说明批量追加只更新未保存的前端表单状态，没有改动运行态渠道数据。

## 本轮实施评审：Authz 用户级 override 基础层

### 需求分析

NexusTok 已经具备管理权限 catalog、角色基线矩阵、`middleware.RequirePermission` 路由权限表和默认前端按钮级权限消费，但当前 `service/authz.Can(userID, systemRole, permission)` 中的 `userID` 仍只是预留参数，`/api/user/self` 返回的 `permissions.admin_permissions` 也只按系统角色计算。这意味着管理员权限只能按 Root/Admin 两档粗粒度控制，无法吸收 `/opt/project/new-api-main` 中“按用户显式 allow/deny override”的优势。

本轮目标是先实现 NexusTok 原生的用户级 override 基础层：允许在数据库中为某个管理用户记录特定资源动作的 allow/deny 覆盖，授权判定时 Root 仍保持 superuser，未知权限 fail-closed，普通用户不能通过 override 获得管理权限；Admin 用户先读显式 override，未命中再回退现有角色基线。这样可以为后续用户编辑页权限矩阵、审计、导入导出和更细粒度运营分权打基础。

### 影响范围分析

- `model` 层：新增三库兼容的用户权限 override 表与基础读写 helper，使用 GORM 迁移，不引入数据库方言 SQL，不使用 JSON 列。
- `model/main.go`：将新模型加入普通迁移和快速迁移，确保 SQLite、MySQL、PostgreSQL 都能自动建表。
- `service/authz`：新增 allow/deny effect 常量、用户 override 解析与管理 helper；`Can` 和新的 `CapabilitiesForUser` 共享同一语义，旧 `Capabilities(systemRole)` 保持兼容包装。
- `controller/user.go`：`GetSelf` 读取当前用户 ID 后返回包含用户 override 的 `admin_permissions`，让前端显隐与服务端路由判定保持一致。
- 测试：补充 service/controller/middleware 覆盖，验证 allow 提权到 Admin 敏感动作、deny 覆盖 Admin 基线、Root 不受 deny 覆盖、普通用户不被 override 提升、未知权限失败关闭。

### 风险评估

- 权限系统属于核心安全边界，override 查询失败时不能扩大权限。本轮设计中查询错误会记录系统日志并回退角色基线，避免数据库瞬时错误导致管理后台整体不可用；后续若增加缓存初始化状态，可以再收紧为显式 deny 优先的 fail-closed 模式。
- 普通用户被误写入 override 不能获得管理权限，因此 `roleForSystemRole` 返回 nil 时直接拒绝，override 只在 Admin 角色上参与细分授权。
- Root 用户不能被用户级 deny 降权，避免误配置锁死唯一超级管理员。
- 新表需要跨数据库兼容，字段仅使用整数、短字符串和 bigint 时间戳；唯一约束使用 GORM index tag，避免手写 `ON CONFLICT` 或方言专用 DDL。
- 本轮不新增前端编辑入口和公开 API，避免一次改动同时触碰用户编辑表单、权限矩阵 UI、审计和批量保存流程；基础层稳定后再分批开放管理能力。

### 方案评审

采用“轻量原生 DB override”方案，而不是直接迁入 new-api-main 的 Casbin enforcer、adapter 和 policy 初始化链路。原因是 NexusTok 目前已经有稳定的内置权限 catalog 与路由权限表，引入 Casbin 会显著扩大启动初始化、缓存一致性、迁移和依赖风险；而本轮需要的核心优势是 per-user allow/deny 语义，可以先用一张原生表和小型 service helper 实现。

执行策略：

1. 新增 `authz_user_overrides` 表，唯一键为 `user_id + resource + action`，`effect` 只允许 `allow` 或 `deny`。
2. `SetUserPermissions` 只保存相对 Admin 基线有差异的动作：与基线一致的项不写入，降低冗余并对齐 new-api-main “只存 override”的优势。
3. `Can` 的顺序为：校验 permission 已注册 -> 解析角色 -> Root 直接允许 -> 非 Admin 拒绝 -> 查询用户 override -> 回退 Admin 基线。
4. `CapabilitiesForUser(userID, systemRole)` 预加载该用户 override map 后构建矩阵，避免为每个动作重复查询数据库。
5. `Capabilities(systemRole)` 继续保留旧签名，作为无用户上下文的兼容入口。
6. 本轮测试只使用 SQLite 内存库，但实现坚持 GORM 抽象和通用字段类型，确保迁移路径兼容 MySQL/PostgreSQL。

### 验收方式

1. `go test ./service/authz`。
2. `go test ./middleware -run TestRequirePermission`。
3. `go test ./controller -run 'TestGetSelfReturnsAdminPermissions|TestGetSelfIncludesAuthzUserOverrides'`。
4. `go test ./model -run Authz`。
5. `git diff --check`。
6. 访问 `http://192.168.0.202:3003/` 并登录，调用 `/api/user/self` 确认 `permissions.admin_permissions` 仍返回完整权限矩阵；如本轮未开放管理 UI，则不通过页面写入 override，只验证现有页面和接口未回归。

### 实施结果

- 新增 `model.AuthzUserOverride`，使用 `user_id + resource + action` 唯一索引保存用户级 allow/deny override，字段均为跨数据库兼容的整数、短字符串和 bigint 时间戳。
- 普通迁移与快速迁移均纳入 `AuthzUserOverride`，容器热更新后 3003 使用的 PostgreSQL 数据库已存在 `authz_user_overrides` 表和唯一索引。
- `service/authz` 新增 `EffectAllow`、`EffectDeny`、`SetUserPermissions`、`SetUserPermissionsInTx`、`ClearUserAuthorization`、`ExplicitUserOverrides` 和 `CapabilitiesForUser`；保存时只持久化相对 Admin 基线不同的动作，降低冗余。
- `Can(userID, systemRole, permission)` 现在按“权限已注册 -> 角色解析 -> Root superuser -> 用户级 override -> 角色基线”的顺序判定；未知权限继续失败关闭，普通用户即使存在误写 override 也不会获得管理权限。
- `/api/user/self` 改为使用 `authz.CapabilitiesForUser(id, role)`，默认前端按钮显隐矩阵与后端路由权限表共享同一 override 语义。
- 本轮未新增用户编辑页权限矩阵 UI 和管理 API，避免一次性扩大到用户表单、审计和批量保存流程；后续可在该基础层之上分批开放管理入口。

### 验证记录

- `go test ./service/authz`：通过。
- `go test ./middleware -run TestRequirePermission`：通过。
- `go test ./controller -run 'TestGetSelfReturnsAdminPermissions|TestGetSelfIncludesAuthzUserOverrides'`：通过。
- `go test ./model -run Authz`：通过。
- `go test ./service/authz ./middleware ./router`：通过。
- `go test ./controller -run 'Authz|GetSelf|Permission'`：通过。
- `go test ./model -run 'Authz|Migrate'`：通过。
- `go test ./...`：通过。
- `git diff --check`：通过。
- 3003 `GET /` 返回 200，账号 `c1cada` 登录成功。
- 3003 `GET /api/user/self` 返回 200，`permissions.admin_permissions.channel` 和 `permissions.admin_permissions.system_setting` 对 Root 均保持完整 `read/operate/write/sensitive_write/secret_view=true`，确认 Root 能力未被 override 基础层回退。
- 3003 运行态数据库验证：`nexustok-hot-pg` 中 `to_regclass('public.authz_user_overrides')` 返回 `authz_user_overrides`，字段包含 `id/user_id/resource/action/effect/created_at/updated_at`，索引包含 `idx_authz_user_override_unique`。
- MCP 浏览器验证：打开 `http://192.168.0.202:3003/`，在浏览器上下文登录并调用 `/api/user/self` 成功；跳转 `/channels` 后渠道管理页正常渲染，网络请求均为 200/301 正常跳转，控制台无新增错误，仅有 i18next 信息日志。

## 本轮实施评审：用户编辑页权限矩阵 override 管理入口

### 需求分析

上一轮已经把用户级 Authz override 基础层原生化到 NexusTok：数据库可保存 allow/deny，`authz.Can` 和 `/api/user/self` 已能消费用户级矩阵。但当前用户管理页面仍无法查看或编辑这些 override：`GET /api/user/:id` 不返回 `admin_permissions`，`POST/PUT /api/user/` 不接收矩阵，默认前端用户抽屉也没有权限矩阵区块。这样 Root 虽然具备底层能力，却无法通过管理后台完成细粒度分权。

对照 `/opt/project/new-api-main`，用户编辑页会在目标用户为 Admin 时展示“Admin Permissions”矩阵，并在保存时提交完整矩阵，后端只保存相对 Admin 基线不同的 override。本轮目标是把这条管理闭环按 NexusTok 当前权限 catalog、Base UI 组件和用户抽屉风格原生化，形成“Root 可为 Admin 配置细粒度权限；普通 Admin 不能配置权限；普通用户不显示矩阵”的最小完整能力。

### 影响范围分析

- `model.User`：新增 `AdminPermissions` 非持久化字段，供详情接口和管理保存 payload 使用，不改变用户表结构。
- `model.User.Edit`：补充事务版本 `EditWithTx`，让用户资料更新与 authz override 保存可在同一事务内完成。
- `controller/user.go`：`GetUser` 返回目标用户当前有效 `admin_permissions`；`CreateUser` 和 `UpdateUser` 在 Root 操作 Admin 用户时保存矩阵，普通用户或非 Root 操作者不能写入矩阵。
- `web/default/src/lib/admin-permissions.ts`：补充权限 catalog 类型、矩阵归一化和默认矩阵工具，继续服务现有按钮级权限 helper。
- `web/default/src/features/users/*`：用户详情类型、表单 schema、payload 转换、API、编辑抽屉接入权限 catalog 和矩阵区块。
- `web/default/src/i18n/locales/*.json`：补齐新增 UI 文案六语翻译。

### 风险评估

- 权限矩阵编辑属于高风险管理能力，必须只允许 Root 提交；普通 Admin 即使构造 `admin_permissions` payload 也应被后端拒绝或忽略，不能靠前端隐藏。
- 更新用户资料和更新权限 override 必须保持事务一致性：用户资料保存失败不能写入权限；权限保存失败不能只更新用户资料。
- 目标用户如果不是 Admin，后端必须清理或忽略 override，避免普通用户未来被升为 Admin 后意外继承旧权限。
- 前端矩阵需要基于 `/api/authz/catalog` 构造完整资源动作集合；catalog 加载失败时不提交矩阵，避免把不完整矩阵保存为覆盖规则。
- 现有用户抽屉较窄，权限矩阵不能造成移动端横向溢出；采用折叠资源组和 checkbox 列表，优先保证可读和可操作。

### 方案评审

采用“后端闭环 + 前端 Root 可见矩阵”的低风险方案：后端新增 `updateAdminPermissionsForUserInTx`，只在 Root 且目标角色为 Admin 时调用 `authz.SetUserPermissionsInTx`；目标角色低于 Admin 时调用 `authz.ClearUserAuthorizationInTx` 清理旧 override；非 Root 提交矩阵直接返回权限错误。`GetUser` 使用 `authz.CapabilitiesForUser(user.Id, user.Role)` 返回当前有效矩阵，让前端初始状态与服务端判定一致。

前端不复制 new-api-main 整个用户抽屉，只吸收其权限矩阵能力。新增 `getPermissionCatalog` API，复用已有 `Checkbox`、`Accordion` 和 `Alert` 组件，在编辑 Admin 用户且当前操作者拥有 `user.sensitive_write` 时展示矩阵。保存 payload 只有在目标角色为 Admin 且 catalog 已加载时才带 `admin_permissions`；否则省略字段，让后端保留或清理既定规则。视觉上保持 NexusTok 现有紧凑后台风格，不新增装饰色或营销文案。

### 验收方式

1. `go test ./controller -run 'Test(GetUserReturnsAdminPermissions|UpdateUserAdminPermissions|CreateUserAdminPermissions)'`。
2. `go test ./model -run TestUserEditWithTx`。
3. `go test ./service/authz`。
4. `cd web/default && bun run i18n:sync`。
5. `cd web/default && bun run typecheck`。
6. `cd web/default && bun run build`。
7. `git diff --check`。
8. 访问 `http://192.168.0.202:3003/`，登录后进入 `/users`，打开 Admin 用户编辑抽屉，确认权限矩阵渲染；调用 `/api/authz/catalog` 与 `/api/user/:id` 确认返回矩阵；不实际保存生产账号权限，避免改变当前运行态管理能力。

## 本轮实施评审：兑换码 secret_view 与完整码 reveal 契约

### 需求分析

前几轮已经把兑换码拆成独立 `redemption` 权限资源，并完成后端路由权限表和默认前端按钮级消费。但当前兑换码列表、搜索和详情接口仍直接返回完整 `key`，导致只具备 `redemption.read` 的管理员也能查看和复制所有未使用兑换码。兑换码本质上是可兑换额度的 bearer secret：拥有完整码即可在用户侧入账，因此它不应和普通列表元数据处在同一权限层级。

对照 `new-api-main` 的平台治理方向，敏感材料应通过更细粒度的资源动作、二次验证和按需 reveal 交互来控制。NexusTok 已经拥有原生 `secret_view` 动作、安全验证中间件、渠道密钥 reveal 模式和用户级 override，本轮目标是把这些能力延伸到兑换码域：列表默认脱敏，完整兑换码只能通过 `redemption.secret_view` + 安全验证后的 reveal 接口获取。

### 影响范围分析

- `service/authz`：为 `redemption` 补齐 `secret_view` 动作和 `RedemptionSecretView` permission 变量；Root 默认拥有，Admin 默认不拥有，可通过用户级 override 显式授权。
- `controller/redemption.go`：列表、搜索、详情和更新响应统一返回脱敏兑换码；新增完整码 reveal handler，只返回 `{ key }`，并记录查看日志。
- `router/redemption-router.go`：新增 `POST /api/redemption/:id/key`，挂接 `redemption.secret_view`、关键限流、禁缓存和安全验证，不改变用户兑换路径。
- `web/default/src/features/redemption-codes/*`：兑换码列不再把列表 `key` 当完整值复制；行内复制/查看与批量复制选中码改为先 reveal，再复制或展示。
- `web/default/src/i18n/locales/*.json`：补齐 reveal、验证、权限提示相关新增文案。
- `docs/features/new-api-main-diff-analysis.md`：记录本轮方案、风险、结果和验证记录。

### 风险评估

- 兑换码列表从完整值改为脱敏值会影响现有管理员复制工作流；必须在前端提供按需 reveal 和复制入口，并在无 `secret_view` 时给出明确禁用/提示，而不是复制一堆掩码。
- `POST /api/redemption/:id/key` 不能只依赖前端隐藏按钮；后端必须同时检查 `redemption.secret_view` 和安全验证 session，且需要禁缓存和关键限流，避免完整码被浏览器或代理缓存。
- 创建兑换码接口当前会返回本次新建的完整 keys，便于管理员立即分发。本轮不改变创建返回契约，避免破坏既有工作流；但创建后的再次查看必须走 reveal 契约。
- Admin 默认不拥有 `secret_view`，因此普通 Admin 的历史体验会从“可直接复制完整码”收紧为“只能查看脱敏码”。这是安全提升，但需要通过 Root 用户实际页面验证确认 Root 工作流正常。
- 批量复制会对选中行逐个调用 reveal 接口；应限制在管理员明确选择后触发，并复用一次安全验证 session，避免页面加载时批量泄露。

### 方案评审

采用“脱敏列表 + reveal endpoint + 前端安全验证”的小步原生化方案。后端新增轻量响应 DTO，不改 `model.Redemption` 表结构和兑换事务；`GetAllRedemptions`、`SearchRedemptions`、`GetRedemption` 和 `UpdateRedemption` 在出站前将 `key` 替换为 `model.MaskTokenKey` 风格的脱敏值，并标记 `key_redacted=true`。新增 `GetRedemptionKey` 只按 ID 读取完整 `key`，返回最小 `{ key }` payload，并记录管理日志。

路由层沿用 NexusTok 的权限表模式：`POST /api/redemption/:id/key` 先经过 `AdminAuth`，再经过 `RequirePermission(redemption.secret_view)`，然后执行 `CriticalRateLimit`、`DisableCache` 和 `SecureVerificationRequired`。这里不额外加 `RootAuth`，因为 `secret_view` 已支持 Root 基线和用户级 override；Root 可以把该能力显式授予受信 Admin，但 Admin 默认仍无权查看。

前端复用现有 Base UI、`SecureVerificationDialog`、权限 helper 和复制组件，不新增视觉体系。兑换码列展示脱敏值，复制按钮点击时调用 reveal；无权限时按钮禁用并显示无权限提示。批量复制选中码也先通过安全验证，再逐条 reveal 并写入剪贴板，避免复制脱敏占位符。

### 验收方式

1. `go test ./service/authz ./router ./controller ./model`，重点覆盖 `redemption.secret_view` catalog、路由权限分类、列表/详情脱敏和 reveal 返回完整码。
2. `cd web/default && bun run i18n:sync`，确认新增文案六语补齐。
3. `cd web/default && bun run typecheck`。
4. `cd web/default && bun run build`。
5. `git diff --check`。
6. MCP 打开 `http://192.168.0.202:3003/redemption-codes`，确认列表渲染脱敏码、无控制台错误；调用列表接口确认响应不包含完整 32 位兑换码。
7. MCP 在登录态浏览器上下文调用 `POST /api/redemption/:id/key`：未通过安全验证时应返回需要验证；完成安全验证后 Root 可 reveal 完整码。若当前账号未配置 2FA/Passkey，则记录“页面按钮可见但 reveal 被安全验证前置阻断”，不强行修改生产账号安全设置。

### 实施结果

已完成兑换码完整值安全查看的原生化闭环。后端为 `redemption` 资源新增 `secret_view` 动作和 `RedemptionSecretView` 权限常量，Root 默认拥有，Admin 默认不拥有，可通过用户级 override 授权；`/api/redemption` 的列表、搜索、详情和更新响应统一构造脱敏 DTO，保留创建接口一次性返回新建完整码的既有分发契约。完整码查看改为 `POST /api/redemption/:id/key`，路由同时经过 `AdminAuth`、`RequirePermission(redemption.secret_view)`、`CriticalRateLimit`、`DisableCache` 和 `SecureVerificationRequired`，handler 只返回 `{ key }` 并写入管理日志。

默认前端兑换码表格不再把列表 `key` 当完整值使用。行内“查看/复制”按钮和批量复制选中码都会先检查 `redemption.secret_view`，再复用 `SecureVerificationDialog` 触发安全验证，验证通过后按需调用 reveal 接口并复制或展示完整码；无权限时按钮禁用并复用统一无权限提示。`CopyButton` 同步支持异步解析复制内容，静态复制调用保持兼容；新增 reveal 文案和权限 catalog 文案已补齐 en、zh、fr、ja、ru、vi 六语。

### 验证记录

1. `go test ./...` 通过。
2. `cd web/default && bun run i18n:sync && bun run typecheck && bun run build` 通过。
3. `git diff --check` 通过。
4. MCP 打开 `http://192.168.0.202:3003/redemption-codes`，页面可访问；首次点击发现热更新仍服务旧 bundle，按运行态验证约定重启 `nexustok-api-hot` 与 `nexustok-frontend-watch` 后重新硬刷新，确认页面加载到新版兑换码行内组件，最终空表状态控制台无 error/warn。
5. MCP 在登录态浏览器上下文创建临时兑换码并调用 `/api/redemption/search`：接口返回脱敏 `key` 和 `key_redacted=true`，响应体不包含创建接口返回的完整 32 位码。
6. MCP 调用 `POST /api/redemption/:id/key`：未完成安全验证时返回 HTTP 403，业务 `code=VERIFICATION_REQUIRED`，响应头包含 `no-store/no-cache`，未返回完整码；当前账号未启用 2FA/Passkey，前端继续提示需先启用安全验证方式，没有强行修改生产账号安全设置。
7. MCP 硬刷新表格后确认临时兑换码行内只显示脱敏码和复制按钮；随后通过 `DELETE /api/redemption/:id` 删除临时记录，再次刷新确认列表恢复为空，测试数据无遗留。

## 本轮实施评审：渠道编辑页对齐 new-api 与模型搜索添加修复

### 需求分析

用户反馈渠道编辑页“搜索添加时不正确”，并希望参考 `/opt/project/new-api-main` 最新默认前端的编辑渠道页面进行对齐。运行态复核显示，当前 3003 环境中 `/api/models/search?keyword=gpt-5.6&p=1&page_size=50` 正确返回 3 个 OpenAI 同步模型：`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`；当前唯一渠道 `11111` 只绑定 `gpt-5.6-sol`，因此前端正确行为应是清晰展示 3 个搜索命中，并允许一次性追加尚未绑定的 `terra/luna` 两个真实模型。

当前页面已经能在模型输入框下方展示 `Add 2 search result(s)`，但仍存在两个体验问题：第一，远程搜索结果没有作为 MultiSelect 下拉候选稳定呈现，用户看到的是空下拉加外部结果条，容易误判搜索没有进入选择器；第二，点击批量追加后表单确实新增模型，但搜索关键词和结果状态残留，页面显示 `Add 0 search result(s)`，让用户误以为追加动作异常或仍需继续操作。

对照 new-api 最新页面，本轮不追求目录级迁移，而是把其优势转为 NexusTok 原生能力：保留 NexusTok 的账号池凭证模式、Codex OAuth、权限裁剪、敏感字段提交裁剪和现有保存链路，只吸收 new-api 渠道编辑抽屉的分区结构、紧凑 section 视觉与模型区稳定交互。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 通用 MultiSelect | `web/default/src/components/multi-select.tsx` | 增加受控搜索值和搜索清空能力；保持旧调用不传新参数时行为不变。 |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 模型搜索输入改为受控；批量追加后清空搜索状态；将 Basic/Credentials/Models/Advanced 视觉结构对齐 new-api 的 section 形态。 |
| 渠道抽屉 section | `web/default/src/features/channels/components/drawers/sections/*` | 新增 NexusTok 原生 section 包装组件，复用现有 `drawer-layout` 公共能力，后续便于继续拆分长表单。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验证结果。 |

### 风险评估

- 本轮只改默认前端，不改后端模型同步、渠道保存接口、数据库结构、计费逻辑和 relay 热路径，核心业务风险低。
- MultiSelect 新增受控搜索参数必须保持向后兼容：不传 `searchValue` 时继续使用内部状态；传入时由调用方控制输入框内容，并通过 `onSearchChange` 回传用户输入。
- 搜索批量追加仍只使用 `/api/models/search` 返回的 `model_name`，并排除当前已选模型；不会把裸关键词 `gpt-5.6` 写入渠道能力。
- 点击批量追加后清空搜索输入和结果面板，只改变未保存表单状态，不会触发 `PUT /api/channel/`；最终保存仍由用户点击 `Update Channel` 走原权限、敏感字段和风险确认链路。
- 对齐 new-api 页面时不能覆盖 NexusTok 特有账号池字段、Codex 凭证刷新和权限禁用逻辑；本轮仅提取 section 包装，不引入 new-api 的 Advanced Custom 和提交 hook。
- 视觉结构从卡片化变为 section 化会影响抽屉局部布局，需要在 3003 上实际打开编辑渠道页验证文字不重叠、按钮可点击、模型搜索和分组编辑仍可用。

### 方案评审

采用“小步原生化对齐”方案。

1. 在 `MultiSelect` 中新增可选 `searchValue`，当调用方传入该值时组件以受控方式展示输入内容；不传时继续使用内部 `inputValue`。同时保留 `isLoading`、`loadingText` 和 `onSearchChange`，让渠道模型搜索可以明确清空输入。
2. 渠道编辑页把 `modelSearchKeyword` 传给 MultiSelect，`handleAddModelSearchResults` 成功追加后调用统一 `clearModelSearch()`，清空输入、debounce 关键词和搜索结果面板，避免 `Add 0 search result(s)` 残留。
3. 保留搜索结果状态条作为“批量追加全部真实命中”的确认区，同时让远程返回的真实模型继续合并进 `modelOptions`，供用户单选某个具体模型。
4. 新增 `ChannelBasicSection`、`ChannelApiAccessSection`、`ChannelAuthSection`、`ChannelModelsSection`、`ChannelAdvancedSection` 等 NexusTok 本地组件，视觉和结构对齐 new-api 最新编辑渠道页；当前长表单先使用这些 wrapper，不拆分业务逻辑，降低回归风险。
5. 验收时以运行态 3003 为准：确认 `gpt-5.6` 搜索显示 3 个真实模型、按钮追加 2 个、追加后搜索状态清空、未点击保存时无 `PUT /api/channel/` 请求，并检查页面和控制台没有新增错误。

### 验收方式

1. `cd web/default && bun run typecheck`。
2. `cd web/default && bun run build`。
3. 如新增用户可见文案，执行 `cd web/default && bun run i18n:sync` 并补齐六语；若无新增文案，确认现有 key 均已存在。
4. `git diff --check`。
5. MCP 打开并强刷 `http://192.168.0.202:3003/channels`，打开渠道 `11111` 编辑抽屉。
6. 在模型输入框中搜索 `gpt-5.6`，确认页面显示 3 个同步模型命中、可追加数量为 2，并且追加后表单模型数从 3 变 5。
7. 追加后确认搜索输入和结果条被清空，页面不再显示 `Add 0 search result(s)`。
8. 通过网络请求确认验证过程中未触发 `PUT /api/channel/`；取消抽屉后运行态渠道数据保持未保存状态。

### 实施结果

已完成渠道编辑抽屉的本轮原生化对齐与模型搜索添加修复。`MultiSelect` 新增可选受控搜索输入 `searchValue`，旧调用不传该参数时继续使用内部状态，保持兼容；渠道模型选择器改为由抽屉统一维护搜索关键词，批量追加远程命中模型后立即清空搜索输入和结果状态，避免追加成功后残留 `Add 0 search result(s)`。

渠道编辑页结构对齐 new-api 最新编辑渠道页的优势，但没有硬迁移其提交逻辑。当前 `Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings` 均通过 NexusTok 本地 section wrapper 渲染，保留账号池凭证模式、Codex OAuth、权限禁用、敏感字段裁剪、风险确认和原有保存链路。`gpt-5.6` 搜索仍基于 `/api/models/search` 的真实 `model_name`，不会把裸关键词写入渠道能力；当前渠道已绑定 `gpt-5.6-sol` 时，批量追加只加入未绑定的 `gpt-5.6-terra` 和 `gpt-5.6-luna`。

本轮未改后端、数据库、relay、计费或模型同步任务，核心业务路径保持不变。

### 验证记录

1. `cd web/default && bun run i18n:sync` 通过；本轮无新增用户可见翻译 key。
2. `cd web/default && bun run typecheck` 通过。
3. `cd web/default && bun run build` 通过。
4. `git diff --check` 通过；本轮触碰的前端文件已用 `bunx prettier --check` 验证通过。全量 `bun run format:check` 仍会因仓库既有历史格式问题失败，本轮未做无关格式化。
5. MCP 打开并强刷 `http://192.168.0.202:3003/channels`，打开渠道 `11111` 编辑抽屉，确认 `Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings` 分区正常渲染。
6. MCP 在模型输入框搜索 `gpt-5.6`，确认运行态页面展示 `3 synced model(s) matched "gpt-5.6"`，包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，并显示 `Add 2 search result(s)`。
7. MCP 点击 `Add 2 search result(s)` 后，表单内模型数从 `Selected 3` 变为 `Selected 5`，新增 `gpt-5.6-terra` 和 `gpt-5.6-luna`；追加后搜索输入和结果条清空，页面不再显示 `Add 0 search result(s)`。
8. MCP 网络面板确认验证过程中只有 `GET /api/models/search?keyword=gpt-5.6&p=1&page_size=50`、`GET /api/channel/1` 等读取请求，没有触发 `PUT /api/channel/`。
9. MCP 在浏览器上下文调用 `GET /api/channel?p=1&page_size=10` 回查运行态渠道，渠道 `11111` 的持久化 `models` 仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认未点击保存时不会写入测试追加结果。
10. MCP 控制台无新增 `error` 或 `warn`；仅观察到 i18next 信息日志和浏览器既有表单可访问性 issue 提示，本轮未新增阻断性前端错误。

## 本轮实施评审：Advanced Custom Channel 后端基础层原生化

### 需求分析

`new-api-main` 已提供 Advanced Custom Channel，用于把一个渠道内的多个客户端入口路径映射到不同上游 URL，并可选择在 OpenAI Chat、Anthropic Messages、Gemini GenerateContent 和 OpenAI Responses 之间做协议转换。NexusTok 当前只有普通 `Custom`、参数覆写和请求头覆写，缺少“按请求路径精确路由到上游端点”的原生能力；这使管理员在接入非标准代理、混合协议网关或只暴露部分兼容端点的上游时，只能通过新增多个普通渠道绕行，配置成本高，且无法在同一渠道内统一模型、分组、权重和计费策略。

该能力价值高，但风险也高：它允许管理员把客户端请求路径和上游地址重新绑定，若不做后端校验，会带来 SSRF、凭证泄露、协议转换错配、计费快照不一致和绕过现有 provider 边界等问题。因此本轮目标不是一次性开放完整前端编辑器，而是先把 Advanced Custom 的后端基础层原生化：新增独立渠道类型、配置 schema、严格校验、relay adaptor、注册映射和回归测试，让后续 UI 编辑器能建立在后端 fail-closed 能力之上。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 渠道/API 常量 | `constant/channel.go`、`constant/api_type.go`、`common/api_type.go` | 新增 `ChannelTypeAdvancedCustom=58` 与 `APITypeAdvancedCustom`，不改变既有类型编号和映射。 |
| 渠道设置 DTO | `dto/channel_settings.go`、新增 DTO 测试 | 在 `ChannelOtherSettings` 中加入 `advanced_custom` 配置；实现 route、auth、converter schema、路径匹配和后端校验。 |
| 渠道保存校验 | `model/channel.go` / `controller/channel.go` | Advanced Custom 渠道保存时必须存在并通过 `advanced_custom` 校验；普通渠道如携带该配置也会先校验 schema，避免无效 JSON 被持久化。 |
| Relay 适配器 | `relay/channel/advancedcustom/*`、`relay/relay_adaptor.go`、`relay/common/relay_info.go` | 新增 adaptor，复用 OpenAI/Claude/Gemini 原生转换与响应处理；支持 URL 拼接、`{model}` 模板、header/query auth、Gemini streaming URL 切换和 Responses/Chat 互转。 |
| 默认前端识别层 | `web/default/src/features/channels/constants.ts` 等 | 仅补齐 type label/icon/提示，保证后端已有 Advanced Custom 渠道能被识别；完整可视化编辑器另行评审。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、验证结果和后续边界。 |

### 风险评估

- Advanced Custom 的 `upstream_path` 支持绝对 URL 和相对 URL。相对 URL 必须依赖渠道 `base_url`，绝对 URL 必须限定为 `http`/`https`，并拒绝 `//host/path` 这类容易被误判的协议相对 URL。
- `incoming_path` 必须以 `/` 开头、不能包含 query，且同一配置内必须唯一；带 `{model}` 的模板只允许匹配单个路径段，避免吞掉额外路径导致越权路由。
- converter 必须与入口路径匹配，例如 Responses 转 Chat 只能挂在 `/v1/responses`，OpenAI Chat 转 Claude/Gemini/Responses 只能挂在 `/v1/chat/completions`，Gemini 原生入口必须包含 `:generateContent` 或 `:streamGenerateContent`。
- auth 支持 `none`、`header`、`query`。`header/query` 必须提供非空 name/value，value 支持 `{api_key}` 模板；默认无 auth 时使用 `Authorization: Bearer <key>`，保持 OpenAI 兼容预期。
- 本轮不新增数据库字段，不修改计费表和日志表；Advanced Custom 继续走现有渠道选择、模型映射、参数覆写、请求体透传、预扣费和响应 usage 处理链路。
- 暂不开放完整可视化编辑器，避免在后端基础未验证前把任意路由映射暴露为常规表单入口。后续 UI 需要继续接入 `channel.sensitive_write`、二次确认和配置模板。

### 方案评审

采用“后端基础层先行、前端仅识别”的原生化方案。

1. 在 Go DTO 层新增 `AdvancedCustomConfig`、`AdvancedCustomRoute` 和 `AdvancedCustomRouteAuth`，并把 `Validate` 设计为保存前和 relay 前都可复用的 fail-closed 校验。
2. 在 `ChannelOtherSettings` 中加入 `AdvancedCustom *AdvancedCustomConfig`，并在渠道校验中要求 `ChannelTypeAdvancedCustom` 必须配置有效 `advanced_custom`；普通渠道如果保留未知/高级 settings 字段，只有 `advanced_custom` 非空时才执行 schema 校验。
3. 新增 `relay/channel/advancedcustom` adaptor，复用 NexusTok 现有 `openai`、`claude`、`gemini` adaptor 和 `service/openaicompat` 转换函数，不复制一套协议转换逻辑。
4. 注册 `APITypeAdvancedCustom`，让中继按新类型走独立 adaptor；同时把该类型加入 StreamOptions 支持表，因为它可能承载 OpenAI/Claude/Gemini 的流式 chat 路径。
5. 前端本轮只补渠道类型常量、图标和 key/base URL 提示，使页面不会显示 Unknown；完整 `advanced_custom` 编辑器、模板和 JSON/visual 双模式另行分批。
6. 测试优先覆盖 DTO 校验和 adaptor 的 URL/auth/转换行为，包括相对 upstream path、缺失 base URL、query/header auth、Gemini 流式 URL、路径不匹配和 Responses 转 Chat。

### 验收方式

1. `go test ./dto -run AdvancedCustom`。
2. `go test ./relay/channel/advancedcustom`。
3. `go test ./model -run AdvancedCustom`（如新增模型层校验测试）。
4. `go test ./common -run ChannelType2APIType`（如新增映射测试）。
5. `go test ./relay -run AdvancedCustom` 或定向构建注册相关包。
6. `cd web/default && bun run i18n:sync`、`bun run typecheck`、`bun run build`（如改前端识别层）。
7. `git diff --check`。
8. MCP 打开 `http://192.168.0.202:3003/` 与 `/channels`，确认热更新页面可访问、渠道列表仍正常渲染，现有 Codex 渠道未受影响；本轮不在运行态创建真实 Advanced Custom 渠道，避免写入高风险生产配置。

### 实施结果

已完成 Advanced Custom Channel 后端基础层的原生化落地。后端新增 `ChannelTypeAdvancedCustom=58`、`APITypeAdvancedCustom` 和渠道类型映射，`ChannelOtherSettings` 支持 `settings.advanced_custom`；保存渠道时，type 58 必须携带有效 Advanced Custom 配置，普通渠道如果携带该配置也会先执行 schema 校验，避免错误配置悄悄落库。

新增 `AdvancedCustomConfig`/`AdvancedCustomRoute`/`AdvancedCustomRouteAuth` 后端 schema，保存前与 relay 前复用同一套 fail-closed 校验：`incoming_path` 必须是无 query 的绝对路径且唯一，`upstream_path` 只能是 http/https 绝对 URL 或单 `/` 开头的相对路径，拒绝协议相对 URL、userinfo 和不匹配的 converter；header/query auth 必须提供安全的 name/value，value 支持 `{api_key}` 模板。`{model}` 路径模板只匹配单一路径段，Gemini `:generateContent` 与 `:streamGenerateContent` 做了兼容匹配。

Relay 层新增 `relay/channel/advancedcustom` adaptor，并在统一 adaptor 注册表中接入。该 adaptor 只负责 route 解析、URL/auth 组装和 converter 分发，协议转换与响应处理复用 NexusTok 现有 OpenAI、Claude、Gemini adaptor 以及 `service/openaicompat` 转换函数；已支持默认 Bearer auth、header/query auth、相对 upstream path 拼接、`{model}` 替换、Claude 默认 header、Gemini 流式 URL 切换、OpenAI Chat 与 Responses 的双向转换，以及原生 OpenAI/Claude/Gemini 响应处理。

默认前端完成识别层接入：渠道类型列表显示 `Advanced Custom`，图标使用 OpenAI 兼容标识，API Key 提示、安全警告和六语 i18n key 已补齐。运行态创建渠道弹窗中可搜索并选中 `Advanced Custom`，选中后显示“Advanced Custom routes can change upstream URLs and credentials. Only use trusted configurations.”安全提示。本轮没有开放完整可视化编辑器，避免在高风险 route/auth 配置上绕过后端先行校验和后续权限评审。

### 验证记录

1. `go test ./dto -run AdvancedCustom` 通过。
2. `go test ./relay/channel/advancedcustom` 通过。
3. `go test ./model -run AdvancedCustom` 通过。
4. `go test ./common -run ChannelType2APIType` 通过。
5. `go test ./relay -run AdvancedCustom` 通过。
6. `go test ./relay ./relay/common ./relay/channel/advancedcustom ./controller -run 'AdvancedCustom|TestGetAdaptorAdvancedCustom|TestChannelType2APITypeAdvancedCustom'` 通过。
7. `go test ./...` 通过。
8. `cd web/default && bun run i18n:sync` 通过；本轮新增 Advanced Custom 文案已补齐 en、zh、fr、ja、ru、vi，仓库既有 fr/ja/ru/vi untranslated 统计仍存在但非本轮新增。
9. `cd web/default && bun run typecheck` 通过。
10. `cd web/default && bun run build` 通过。
11. `git diff --check` 通过；本轮触碰的前端文件已用 `bunx prettier --check` 验证通过。
12. MCP 打开并强刷 `http://192.168.0.202:3003/channels`，确认渠道列表正常渲染，现有 Codex 渠道 `11111` 未受影响；当前页面网络请求均最终返回 200，只有 `/api/channel` 与 `/api/prefill_group` 的尾斜杠 301 跳转。
13. MCP 打开创建渠道弹窗，搜索并点选 `Advanced Custom`，确认类型候选、弹窗标题、类型名和安全警告均已进入运行态页面；未点击保存，未创建真实 Advanced Custom 渠道。
14. MCP 访问 `http://192.168.0.202:3003/`，确认首页仍正常渲染。
15. MCP 在登录态浏览器上下文调用 `POST /api/channel/`，尝试创建 type 58 但不携带 `settings.advanced_custom` 的渠道；接口返回 HTTP 200、业务 `success=false`，错误为 `渠道额外设置[channel setting] 格式错误：advanced_custom is required`，确认后端保存校验已生效且不会进入创建成功路径。
16. MCP 控制台未发现本轮新增阻断性错误；仅保留浏览器报告的既有表单 autocomplete/label 可访问性 issue。

## 本轮复核评审：渠道编辑页搜索添加修复与 Advanced Custom 编辑入口

### 需求分析

用户继续反馈“搜索添加时不正确”，并要求参考 `/opt/project/new-api-main` 最新编辑渠道页面继续对齐。重新打开 3003 运行态页面复现后确认：当前渠道编辑抽屉已经具备 Basic/Credentials/Models/Advanced 分区，但模型 `MultiSelect` 在输入 `gpt-5.6` 时，下拉候选仍会铺出大量静态模型，远程模型元信息命中没有稳定成为优先候选；这会让管理员误以为搜索没有按关键词工作，也会弱化外部“Add search results”批量追加按钮的可信度。

同时，上一轮 Advanced Custom 只完成了后端基础层和前端类型识别。对照 new-api 最新页面，编辑渠道页还缺少 `settings.advanced_custom` 的可视化/JSON 双模式配置入口。由于后端已经 fail-closed 校验 type 58，本轮可以把 new-api 的优势转成 NexusTok 原生能力：在现有抽屉中暴露受权限控制的 Advanced Custom Routes 编辑器，让管理员能用模板快速配置 route/auth/converter，并在提交前由前端和后端双重校验。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 通用多选 | `web/default/src/components/multi-select.tsx` | 显式按输入过滤候选，并让远程命中/当前已选模型在搜索时优先展示；不改变未搜索状态的历史行为。 |
| Advanced Custom helper | `web/default/src/features/channels/lib/advanced-custom.ts` | 新增前端配置模板、归一化、校验、统计和 auth 构造工具，与后端 schema 保持一致。 |
| Advanced Custom 弹窗 | `web/default/src/features/channels/components/dialogs/advanced-custom-editor-dialog.tsx` | 新增 visual/json 双模式编辑器，支持模板填充、追加、格式化、route 增删、auth 配置和保存前校验。 |
| 渠道表单转换 | `web/default/src/features/channels/lib/channel-form.ts` | 增加 `advanced_custom` 表单字段，从 `settings.advanced_custom` 回填；保存时仅 type 58 写入，切换其它类型时清理该字段。 |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 接入 Advanced Custom Routes 配置区、统计 badge、前端校验、权限禁用和弹窗保存。 |
| i18n | `web/default/src/i18n/*` | 补齐新增用户可见文案六语翻译。 |

### 风险评估

- `MultiSelect` 是共享组件，过滤逻辑必须保持未输入关键词时展示完整候选，避免影响分组选择、其它多选和历史模型回填。
- 渠道能力只能追加真实模型名；搜索批量追加继续只读取 `/api/models/search` 返回的 `model_name`，不能把裸关键词写入 `models`。
- Advanced Custom 能改变上游 URL、auth 和协议转换，必须只在 type 58 展示并受 `channel.sensitive_write` 控制；没有敏感写权限时不能编辑或提交相关 settings。
- 前端校验不能代替后端校验，保存 payload 仍必须让后端 `settings.advanced_custom` schema 做最终裁决。
- 编辑器从 new-api 迁移时必须改为 NexusTok 版权头、中文注释和当前 Base UI 组件约定，不能硬搬提交 hook 或破坏 NexusTok 的账号池凭证模式、权限裁剪、Codex OAuth 和原有保存链路。
- `settings` 需要保留未知字段；写入 `advanced_custom` 时不得抹掉上游模型更新、字段透传、AWS/Vertex/Azure 等既有配置。

### 方案评审

采用“共享组件小修 + Advanced Custom 前端入口原生化”的低风险方案：

1. 在 `MultiSelect` 内部计算 `visibleItems`：无搜索词时保持原列表；有搜索词时只展示包含关键词的候选、已选命中和可创建项，远程命中因已合并进 `options` 会自然优先出现在下拉里。
2. 新增 `advanced-custom.ts`，迁移并本地化 new-api 的 route 模板、converter/auth/path 约束、JSON parse/stringify、validate 和 stats helper，保证前端配置形态与后端 DTO 对齐。
3. 新增 `AdvancedCustomEditorDialog`，提供 visual/json 双模式：visual 模式用于常规 route/auth/converter 配置，JSON 模式用于高级批量编辑；保存前统一 normalize + validate。
4. 在 `channelFormSchema` 增加 `advanced_custom` 字符串字段和 type 58 前端校验；`transformChannelToFormDefaults` 从 `settings.advanced_custom` 回填；`buildSettingsJSON` 仅在 type 58 时写入 normalized config，其它类型切换时清理。
5. 在渠道编辑抽屉的凭证/基础 URL区域展示 Advanced Custom Routes 配置块，显示 route 数、入口类型和 incomplete 状态；按钮受敏感权限控制，保存弹窗只更新表单值，不直接调用后端。
6. 修改后执行前端类型检查、构建、i18n sync、必要的 helper 测试和 MCP 真实页面验证；若热更新未生效，按约定重启容器后复验。

### 验收方式

1. `cd web/default && bun test src/components/multi-select.test.tsx src/features/channels/lib/advanced-custom.test.ts`（如测试环境支持 React 测试则执行；否则至少执行 helper 单测）。
2. `cd web/default && bun run i18n:sync`。
3. `cd web/default && bun run typecheck`。
4. `cd web/default && bun run build`。
5. `git diff --check`。
6. MCP 打开 `http://192.168.0.202:3003/channels`，编辑渠道 `11111`，输入 `gpt-5.6`，确认下拉与搜索结果只围绕 `gpt-5.6` 真实模型展示，批量追加后表单从 3 个模型变为 5 个且不触发保存请求。
7. MCP 打开创建渠道抽屉，选择 `Advanced Custom`，确认页面出现 Advanced Custom Routes 配置入口；应用模板并保存弹窗后 route 数和类型 badge 更新；未点击创建渠道，避免写入高风险运行态配置。

### 实施结果

已完成本轮渠道编辑页对齐与搜索添加修复。`MultiSelect` 新增 `filterMultiSelectItems` 显式过滤层：没有搜索词时保持原候选顺序，有搜索词时只展示 value 或 label 命中的候选，因此渠道模型输入 `gpt-5.6` 时，下拉候选收敛为 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，不再混入大量 `gpt-3.5`、`gpt-4` 或其它静态模型。创建自定义模型的旧能力仍保留，但加载中或存在真实匹配候选时不会展示裸关键词创建项。

已把 new-api 最新编辑渠道页中 Advanced Custom route 编辑能力原生化到 NexusTok 默认前端。新增 `advanced-custom.ts` 前端 helper，提供模板、converter/auth/path 约束、归一化、校验、序列化和摘要统计；新增 `AdvancedCustomEditorDialog`，支持 Visual/JSON 双模式、模板填充/追加、route 增删、incoming/upstream path、converter 和 auth 配置。弹窗使用 NexusTok 当前 Base UI 组件与 Hugeicons 图标库，不直接复用 new-api 提交流程。

渠道表单已增加 `advanced_custom` 字段，并在 type 58 时进行前端校验：无有效 route 会标记 incomplete，route 使用相对 upstream path 且未配置 Base URL 时会提示 Base URL 必填。保存 payload 构造时仅 `Advanced Custom` 渠道写入 `settings.advanced_custom`，其它渠道类型切换时清理该高风险配置，同时保留 `settings` 中其它未知字段，避免影响 AWS/Vertex/Azure、字段透传、上游模型更新等既有设置。

渠道编辑抽屉已接入 Advanced Custom Routes 配置区：选择 `Advanced Custom` 后在 Credentials/Base URL 附近显示安全提示、route 数量、入口类型 badge 和 `Configure routes` 按钮；按钮受 `channel.sensitive_write` 权限保护，弹窗保存只更新当前表单状态，不直接调用后端创建或更新接口。该能力与当前页面的 Basic Information、Credentials、Models & Groups、Advanced Settings 分段布局保持一致，继续保留 NexusTok 的账号池、Codex OAuth、权限裁剪和风险确认链路。

### 验证记录

1. `cd web/default && bun test src/features/channels/lib/advanced-custom.test.ts src/components/multi-select.test.ts` 通过，覆盖 MultiSelect 搜索过滤、`gpt-5.6` 三模型候选、label 命中、Advanced Custom 模板 clone/parse/stringify、重复 path 拒绝、converter/path mismatch 拒绝、stats 与相对 upstream path 检测。
2. `cd web/default && bun run i18n:sync` 通过，报告已生成；本轮新增文案已补齐 en、zh、fr、ja、ru、vi，历史 fr/ja/ru/vi untranslated 统计不属于本轮新增。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. MCP 打开 `http://192.168.0.202:3003/channels`，编辑渠道 `11111`，当前模型为 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`；在模型输入框输入 `gpt-5.6` 后，DOM 中真实下拉候选为 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，搜索结果条显示 `3 synced model(s) matched "gpt-5.6"` 和 `Add 2 search result(s)`。
7. MCP 点击批量追加后，表单内模型数从 `Selected 3` 变为 `Selected 5`，新增 `gpt-5.6-terra` 与 `gpt-5.6-luna`，搜索输入和结果条被清空；验证期间未点击 `Update Channel`。
8. MCP 在浏览器上下文携带 `NexusTok-User: 1` 调用 `GET /api/channel?p=1&page_size=10&tag_mode=false&id_sort=false` 返回 HTTP 200、`success=true`，渠道 `11111` 持久化模型仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认搜索追加验证没有写入运行态数据。
9. MCP 打开创建渠道抽屉，选择 `Advanced Custom`，页面出现安全提示、`Advanced Custom Routes`、`Routes: 0`、`Incomplete` 与 `Configure routes`；点击配置后弹窗显示 Visual/JSON 模式、模板选择和默认 OpenAI Chat route。
10. MCP 在 Advanced Custom 弹窗点击 `Fill Template` 并 `Save changes` 后，外层表单更新为 `Routes: 1` 和 `OpenAI Chat` badge，`Incomplete` 消失；未点击 `Save changes` 创建渠道，避免写入高风险运行态配置。
11. MCP 刷新 `http://192.168.0.202:3003/channels` 后页面正常渲染；控制台 error/warn 为空，说明热更新页面已生效且无需重启容器。

## 本轮复核评审：渠道编辑页继续对齐 new-api 最新交互

### 需求分析

用户继续指出“搜索添加时不正确”，并要求把当前渠道编辑页面进一步对齐 `/opt/project/new-api-main` 最新版编辑渠道页。运行态复现确认：当前输入 `gpt-5.6` 后确实能命中 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个真实模型，但由于 `gpt-5.6-sol` 已经在渠道中，按钮显示 `Add 2 search result(s)`，容易被理解为只同步到了两个模型，和用户预期的“三种模型都同步到能力中”产生歧义。

对照 new-api 最新源码，NexusTok 当前编辑抽屉已经具备分区导航、账号池凭证、Codex OAuth、密钥安全验证、模型映射保护和 Advanced Custom 入口，但仍缺少三个体验保护：编辑详情加载中防覆盖的骨架屏、敏感字段只读时的全局提示、以及校验失败时只展开包含错误的高级设置。上述能力适合以 NexusTok 原生方式补齐，不应直接覆盖当前提交链路。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 调整模型搜索结果的命中/新增/已存在计数展示，加入详情加载态、敏感字段只读提示和更精准的 invalid 展开逻辑。 |
| 渠道编辑加载态 | `web/default/src/features/channels/components/drawers/sections/channel-editor-loading-state.tsx` | 新增 new-api 同类骨架屏，避免详情未加载完成时展示默认空表单。 |
| 表单错误归类 | `web/default/src/features/channels/lib/channel-form-errors.ts` | 抽出高级设置字段集合，供 invalid handler 判断是否需要展开高级设置。 |
| 高级配置状态 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 复用 new-api 的空值判断，避免 `{}`、`[]`、`null` 这类空 JSON 被误标记为已配置。 |
| i18n | `web/default/src/i18n/*` | 补齐新增提示、计数和加载文案六语翻译。 |

### 风险评估

- 搜索添加只能修改表单内 `models` 字段，不能直接保存渠道；运行态验证不得点击 `Update Channel`，避免误改现有渠道。
- `gpt-5.6-sol` 已在渠道中时，按钮应只追加未存在的模型，但必须明确展示“3 个命中、2 个可新增、1 个已存在”，避免再次让用户误判同步缺失。
- 详情加载态只应在编辑模式且详情请求首次加载时显示；如果 React Query 已有缓存，不应闪烁或阻塞正常编辑。
- 敏感字段只读提示不能改变现有权限裁剪语义：普通写权限仍可编辑模型、分组、优先级、权重等非敏感字段，敏感字段继续由 `canSensitiveWrite` 控制。
- 高级设置展开逻辑只影响前端可见状态，不替代后端 schema 和权限校验；提交 payload 的敏感字段过滤、账号池模式和 Codex OAuth 链路保持不变。

### 方案评审

本轮采用“小切片 UX 对齐”方案：

1. 模型搜索区域保留现有远程搜索和批量追加逻辑，但把文案拆成命中数、可新增数和已在渠道中的数量；按钮改为“Add {{count}} new model(s)”，只表示实际会新增的模型。
2. 从 new-api 引入 `ChannelEditorLoadingState` 的交互思想并本地化为 NexusTok 文件：编辑详情加载中显示骨架屏和防覆盖提示。
3. 新增 `hasAdvancedSettingsErrors` helper，`onInvalid` 只在高级字段出错时展开高级设置；普通 Basic/Credentials/Models 错误不再强制展开高级区。
4. 追加敏感字段只读 Alert：当用户只有非敏感写权限时，明确提示仍可编辑模型、分组、优先级、权重等非敏感操作字段。
5. 调整高级配置“已配置”判断，空 JSON、`null` 和空数组不再触发 configured 状态；保留 NexusTok 当前的账号池、Advanced Custom、状态码风险确认和模型映射保护逻辑。

### 验收方式

1. `cd web/default && bun test src/features/channels/lib/advanced-custom.test.ts src/components/multi-select.test.ts`。
2. `cd web/default && bun run i18n:sync`。
3. `cd web/default && bun run typecheck`。
4. `cd web/default && bun run build`。
5. `git diff --check`。
6. MCP 访问 `http://192.168.0.202:3003/` 与 `/channels`，编辑渠道 `11111`，输入 `gpt-5.6`，确认页面显示 3 个命中、2 个可新增、1 个已存在；点击新增后表单变为 5 个模型但不点击保存。
7. MCP 刷新页面并检查控制台 error/warn；若热更新未生效，按约定重启容器后复验。

### 实施结果

已完成渠道编辑页的继续对齐。模型搜索添加区域不再用容易误解的 `Add {{count}} search result(s)` 表达真实行为，而是拆成 `{{matched}} matched · {{addable}} new · {{existing}} already selected`。例如现有渠道已经包含 `gpt-5.6-sol` 时，输入 `gpt-5.6` 会显示 3 个真实模型命中、2 个可新增、1 个已存在，按钮只显示 `Add 2 new model(s)`，明确说明实际只会新增 terra/luna，不会重复写入 sol。

已新增 `ChannelEditorLoadingState`，编辑详情首次加载时展示骨架屏和“等待详情加载，避免覆盖已保存值”的提示；主表单在加载期间隐藏，提交按钮同步禁用，避免默认表单值被误提交。该实现吸收 new-api 最新编辑器的防覆盖思路，但保留 NexusTok 当前 React Query、账号池和渠道权限链路。

已新增 `channel-form-errors.ts`，集中维护高级设置字段集合；校验失败时只有高级设置字段出错才自动展开高级区，Basic/Credentials/Models 的错误不会再把用户带到无关区域。同时高级设置“已配置”判断改为解析空 JSON：`null`、`{}`、`[]` 不再被误认为已配置，保留非空对象/数组和非法但非空 JSON 的提示价值。

已为敏感字段只读场景新增全局提示。当用户可以编辑非敏感字段但没有 `channel.sensitive_write` 时，页面会提示敏感设置只读，并说明模型、分组、优先级、权重等非敏感字段仍可编辑。现有字段级禁用、提交 payload 裁剪、密钥安全验证、账号池模式、Codex OAuth、Advanced Custom 配置入口、状态码风险确认和模型映射保护均保持不变。

### 验证记录

1. `cd web/default && bun test src/features/channels/lib/advanced-custom.test.ts src/components/multi-select.test.ts` 通过，7 个用例全部通过。
2. `cd web/default && bun run i18n:sync` 通过；新增 6 条文案已补齐 en、zh、fr、ja、ru、vi，并记录到 `static-keys.ts`。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. MCP 打开 `http://192.168.0.202:3003/`，首页正常渲染；随后打开 `/channels`，渠道列表正常渲染。
7. MCP 编辑渠道 `11111`，输入 `gpt-5.6` 后，页面显示 `3 matched · 2 new · 1 already selected`，候选列表为 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，按钮显示 `Add 2 new model(s)`。
8. MCP 点击新增按钮后，表单显示 `Selected 5`，新增 `gpt-5.6-terra` 与 `gpt-5.6-luna`；验证期间未点击 `Update Channel`。
9. MCP 在浏览器上下文调用 `GET /api/channel?p=1&page_size=10&tag_mode=false&id_sort=false` 返回 HTTP 200、`success=true`，渠道 `11111` 持久化模型仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认本轮验证没有写入后端。
10. MCP 控制台未出现新增 error/warn；仅保留浏览器既有 issue：一个表单 autocomplete 提示和三个 label 关联提示。

## 本轮实施评审：模型定价价格格式化稳定化

### 需求分析

继续对照 `/opt/project/new-api-main` 的系统设置模型定价实现后确认：new-api 已将模型定价的核心计算、快照、输入组件和价格格式化拆分为独立 helper，其中 `pricing-format.ts` 对浮点计算结果做了漂移吸附。NexusTok 当前已经具备完整的模型定价可视化编辑器、阶梯表达式编辑器和批量 JSON 编辑入口，但模型定价抽屉与批量模型定价表格仍各自使用简单的 `toFixed(12)` 格式化。

这会在管理员输入常见十进制价格或倍率时留下展示风险：例如倍率与价格相乘、输入价反推倍率、音频输出价反推音频输入倍率时，JavaScript 浮点误差可能把本应显示为 `0.3`、`3.75` 或 `15.11` 的价格变成带尾巴的近似值。该问题不改变后端计费表达式执行，但会影响保存预览、列表摘要和管理员对实际配置的判断，适合吸收 new-api 的稳定格式化能力作为 NexusTok 原生能力。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 价格格式 helper | `web/default/src/features/system-settings/models/pricing-format.ts` | 新增独立格式化函数，集中处理空值、非法值、显示精度和浮点漂移吸附。 |
| 模型定价抽屉 | `web/default/src/features/system-settings/models/model-pricing-sheet.tsx` | 替换本地 `formatNumber`，让输入价、分项价、倍率反推和保存预览使用同一格式化规则。 |
| 批量模型定价表格 | `web/default/src/features/system-settings/models/model-ratio-visual-editor.tsx` | 替换列表摘要里的本地 `formatPrice`，保证表格摘要与编辑抽屉展示一致。 |
| 模型管理抽屉 | `web/default/src/features/models/components/drawers/model-mutate-drawer.tsx` | 替换模型级定价编辑中的本地格式化实现，保证 `/models/metadata` 的可达定价入口同步受益。 |
| 前端测试 | `web/default/src/features/system-settings/models/pricing-format.test.ts` | 新增针对整数、小数、浮点漂移、空值和非法值的单测，防止后续回退。 |

### 风险评估

- 本轮只改变前端展示和由展示输入反推的字符串格式，不调整后端计费、保存接口、数据库结构、表达式版本和 quota 转换公式。
- `formatPricingNumber` 仍保留最多 12 位展示小数，不会截断管理员输入的合理高精度价格；只在误差落入极小容忍区间时吸附到更短的十进制结果。
- 模型定价抽屉中输入框仍保留原有 `numericDraftRegex`，不会接受负数、科学计数法或其它原先不允许的格式。
- 批量表格的价格摘要只读展示会变得更稳定，但排序仍基于摘要字符串；本轮不改表格排序语义，避免额外行为变化。
- 涉及计费表达式周边文件前已阅读 `pkg/billingexpr/expr.md`，本轮不修改表达式语言、变量归一化、预消费或结算逻辑。

### 方案评审

采用“共享 helper + 定向替换 + 单测锁定”的低风险方案：

1. 从 new-api 吸收 `formatPricingNumber` 的核心思想，在 NexusTok 下新增本地 `pricing-format.ts`，保留项目版权头和当前 TypeScript 风格。
2. 将 `model-pricing-sheet.tsx` 的本地 `formatNumber` 调用替换为 `formatPricingNumber`，覆盖 `ratioToBasePrice`、`deriveLanePrice`、`deriveLaneRatio` 和输入价反推倍率。
3. 将 `model-ratio-visual-editor.tsx` 的本地 `formatPrice` 替换为同一个 helper，覆盖价格摘要与详情。
4. 将 `/models/metadata` 使用的模型管理抽屉同步切到共享 helper，避免系统设置批量入口与模型管理单模型入口出现价格显示差异。
5. 新增 helper 单测，覆盖 `0.1 + 0.2`、`0.1 * 0.2`、`1.005`、空值和非法值等边界。
6. 本轮不搬迁 new-api 的整套 `model-pricing-core` / `model-pricing-snapshots` 文件，以免在无必要时重写批量编辑器结构；后续若继续优化模型定价页，再以独立切片做快照/列定义拆分。

### 验收方式

1. `cd web/default && bun test src/features/system-settings/models/pricing-format.test.ts`。
2. `cd web/default && bun run i18n:sync`。
3. `cd web/default && bun run typecheck`。
4. `cd web/default && bun run build`。
5. `git diff --check`。
6. MCP 打开 `http://192.168.0.202:3003/`、`/pricing-settings` 和 `/models/metadata`，确认价格设置页与模型管理页正常渲染；如热更新未体现，先重启容器再复验。

### 实施结果

已完成模型定价价格格式化稳定化。本轮新增 `pricing-format.ts`，集中提供 `formatPricingNumber`：空值、`false`、非法数字返回空字符串；有效数字最多保留 12 位展示小数；对于 `0.1 + 0.2`、`0.1 * 0.2`、`3.7500000000000004` 这类 JavaScript 浮点漂移，会在极小容忍区间内吸附为管理员预期的小数结果。该 helper 只服务前端展示和表单字符串生成，不改变后端计费表达式、预消费、结算或 quota 转换。

模型系统设置中的 `model-pricing-sheet.tsx` 已移除本地 `formatNumber`，输入价、分项价、倍率反推和保存预览统一使用 `formatPricingNumber`。批量模型定价表格 `model-ratio-visual-editor.tsx` 已移除本地 `formatPrice`，价格摘要和详情与编辑抽屉共用同一规则；同一逻辑块中原英文注释已改为中文说明，明确 tiered_expr 模型保留 ratio/price 的多实例同步兼容背景。

实际可达的模型管理页面 `/models/metadata` 也已同步接入该 helper。`model-mutate-drawer.tsx` 之前有一份独立的 `formatPricingNumber` 本地实现，本轮已删除并改为复用共享 helper，避免模型管理单模型定价入口与系统设置批量模型定价入口在价格展示和反推倍率时出现不一致。

### 验证记录

1. 已按计费表达式规则先阅读 `pkg/billingexpr/expr.md`；本轮未修改表达式语言、变量归一化、预消费、结算或日志注入。
2. `cd web/default && bun test src/features/system-settings/models/pricing-format.test.ts` 通过，覆盖空值/非法值、常见浮点漂移吸附和高精度保留。
3. `cd web/default && bun run i18n:sync` 通过；本轮没有新增用户可见文案。
4. `cd web/default && bun run typecheck` 通过。
5. `cd web/default && bun run build` 通过。
6. `git diff --check` 通过。
7. `rg -n "function formatPricingNumber|toFixed\\(12\\)|formatNumber\\(|formatPrice\\(" web/default/src/features/system-settings/models web/default/src/features/models/components/drawers/model-mutate-drawer.tsx -S` 只剩共享 helper 的导出定义，确认旧本地实现已移除。
8. MCP 打开 `http://192.168.0.202:3003/`，首页正常渲染。
9. MCP 打开并强刷 `http://192.168.0.202:3003/pricing-settings`，价格设置页正常渲染；当前生产入口只展示 `Group ratios` 和 `Tool prices`，`Model prices` 标签未对外暴露，因此未在该页写入或保存任何模型价格配置。
10. MCP 打开 `http://192.168.0.202:3003/models/metadata`，模型列表正常渲染；选择 `gpt-5.6-terra` 打开编辑抽屉，`Pricing Configuration` 正常显示 `Per-token`、`Per-request`、`Expression` 三种模式，输入价格展示为干净小数字符串，如 `2.5`、`20`、`0.25`、`3.125`。
11. 验证期间未点击 `Update Model` 或任何保存按钮，未写入运行态模型价格数据。
12. MCP 控制台无新增 error/warn；仅保留既有表单可访问性 issue：一个字段缺少 label、一个字段缺少 id/name、三个 label 关联提示。

## 本轮实施评审：系统设置数字输入 NaN 防护

### 需求分析

继续对照 `/opt/project/new-api-main` 的系统设置差异后确认，new-api 独有 `utils/numeric-field.ts`，并在定价展示、支付配置、数据面板、性能监控、路由可靠性等页面复用 `safeNumberFieldProps`。该 helper 的核心价值不是视觉改版，而是避免 `<input type="number">` 在清空输入框、只输入 `-` 或输入非完成态小数时把 `valueAsNumber === NaN` 写入 react-hook-form。

NexusTok 当前多个系统设置字段仍直接执行 `field.onChange(event.target.valueAsNumber)`，字段一旦进入 `NaN`，Zod `z.number()` / `z.coerce.number()` 校验可能在提交阶段失败，但页面上不一定出现清晰 toast，管理员会感知为“保存按钮没反应”。该问题属于系统设置稳定性能力缺口，适合吸收 new-api 的数字字段安全适配器，并以当前项目原生方式逐步覆盖关键数字输入。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 数字字段 helper | `web/default/src/features/system-settings/utils/numeric-field.ts` | 新增 `safeNumberFieldProps`，只向表单写入有限数字，非有限值显示为空字符串并忽略变更。 |
| helper 测试 | `web/default/src/features/system-settings/utils/numeric-field.test.ts` | 覆盖有限数字写入、`NaN`/`Infinity` 忽略、非有限表单值显示为空。 |
| 定价显示设置 | `web/default/src/features/system-settings/general/pricing-section.tsx` | `USDExchangeRate` 和自定义汇率输入不再把 `NaN` 写入表单。 |
| 数据面板设置 | `web/default/src/features/system-settings/content/dashboard-section.tsx` | 数据导出刷新间隔输入使用安全数字绑定。 |
| 支付配置 | `web/default/src/features/system-settings/integrations/payment-settings-section.tsx` | Epay/Stripe 单价与最低充值金额使用安全数字绑定，避免支付配置清空输入后进入不可保存状态。 |
| Creem 产品弹窗 | `web/default/src/features/system-settings/integrations/creem-product-dialog.tsx` | 产品价格和额度输入使用安全数字绑定。 |
| 监控配置 | `web/default/src/features/system-settings/integrations/monitoring-settings-section.tsx` | 自动测试间隔输入使用安全数字绑定。 |
| 路由可靠性 | `web/default/src/features/system-settings/models/routing-reliability-section.tsx` | 重试次数和自动测试间隔使用安全数字绑定。 |
| Grok 设置 | `web/default/src/features/system-settings/models/grok-settings-card.tsx` | 违规扣费金额输入使用安全数字绑定；Grok 表单 dotted key 结构修复另列后续切片。 |

### 风险评估

- 本轮只处理前端表单数字输入的 `value`/`onChange` 绑定，不改后端 options 存储、不改接口字段名、不改默认值和提交 payload 结构。
- `safeNumberFieldProps` 会忽略 `NaN` 更新，React 受控输入会回到上一个有效值；这与 new-api/Semi `InputNumber` 的“无效输入不进入表单状态”行为一致，但意味着管理员不能在这些字段里临时保留非法草稿。
- 空输入会显示为空字符串，但不会立刻写入 `NaN`；必填/最小值错误仍由原有 Zod schema 在提交或校验阶段处理。
- 支付配置字段较敏感，本轮只替换数字输入绑定，不触碰支付方法、产品、Webhook、优惠配置、风险确认或保存流程。
- 部分系统设置仍有字符串型数字字段或自定义数字编辑器，本轮不一次性替换所有数字输入，避免扩大风险；后续继续按差异文档分批覆盖。

### 方案评审

采用“helper 原生化 + 关键字段定向接入”的方案：

1. 在 NexusTok 新增 `safeNumberFieldProps`，版权头和注释改为项目规范的中文说明，解释为什么不能把 `NaN` 写入表单状态。
2. 对 `PricingSection`、`DashboardSection`、`PaymentSettingsSection`、`CreemProductDialog`、`MonitoringSettingsSection`、`RoutingReliabilitySection`、`GrokSettingsCard` 中直接使用 `valueAsNumber` 或直接展开数字 field 的关键字段改为展开 `safeNumberFieldProps(field)`，保留原 `min`、`max`、`step`、`disabled` 等属性。
3. 对可选自定义汇率字段保持“空值可选”的语义：安全 helper 负责不写 `NaN`，schema 仍决定 `CUSTOM` 模式下是否必填。
4. 新增 Node 单测覆盖 helper 行为，不引入浏览器测试依赖。
5. 执行定向单测、i18n sync、typecheck、build、diff check，并用 MCP 访问 3003 的系统设置/价格配置页面，确认关键页面仍正常渲染；验证期间不保存支付或系统配置。

### 验收方式

1. `cd web/default && bun test src/features/system-settings/utils/numeric-field.test.ts`。
2. `cd web/default && bun run i18n:sync`。
3. `cd web/default && bun run typecheck`。
4. `cd web/default && bun run build`。
5. `git diff --check`。
6. MCP 打开 `http://192.168.0.202:3003/`、`/system-settings/site`、`/pricing-settings`、`/system-settings/models/routing-reliability`，确认页面正常渲染；如热更新未体现，先重启容器再复验。

### 实施结果

已完成系统设置数字输入 NaN 防护原生化。本轮新增 `utils/numeric-field.ts`，提供 `getSafeNumberDisplayValue` 和 `safeNumberFieldProps`：只有有限数字会进入 react-hook-form 状态，`NaN`、`Infinity`、字符串、空值等非有限值在展示层兜底为空字符串，并且不会污染表单值。

系统设置中的定价显示、数据面板、支付网关、Creem 产品弹窗、监控规则、路由可靠性、Grok 违规扣费金额等关键数字输入已接入统一 helper，保留原有 `min`、`max`、`step`、`disabled` 和字段名，不改变后端 options payload、保存接口或数据库结构。本轮同时移除了 `routing-reliability-section.tsx` 里旧的本地 `numberInputValue` 辅助函数，避免同类逻辑继续分散。

新增 `numeric-field.test.ts` 覆盖有限数字展示、非有限值展示为空、有限数字写入表单状态、`NaN`/`Infinity` 不写入表单状态。该能力与 new-api 的系统设置稳定性优势对齐，但以 NexusTok 当前 Base UI/react-hook-form 表单结构落地。

### 验证记录

1. `cd web/default && bun test src/features/system-settings/utils/numeric-field.test.ts` 通过，3 个用例全部成功。
2. `cd web/default && bun run i18n:sync` 通过；本轮没有新增用户可见文案。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. `rg -n "valueAsNumber|safeNumberFieldProps|numberInputValue" ...` 复核后，业务页面只保留 `safeNumberFieldProps` 调用，直接读取 `valueAsNumber` 的逻辑集中在 helper 内部。
7. MCP 打开 `http://192.168.0.202:3003/`，首页正常渲染。
8. MCP 打开 `http://192.168.0.202:3003/system-settings/site`，实际落到 `/system-settings/site/system-info`，系统信息页正常渲染。
9. MCP 打开 `http://192.168.0.202:3003/pricing-settings`，Group & Tool Pricing 页面正常渲染，分组倍率数字输入可见。
10. MCP 打开 `http://192.168.0.202:3003/system-settings/billing/payment`，Payment Gateway 页面正常渲染，Epay、Stripe、Creem、Waffo、Waffo Pancake 相关数字输入可见；验证期间未点击保存。
11. MCP 打开 `http://192.168.0.202:3003/system-settings/content/dashboard`，Data Dashboard 页面正常渲染，刷新间隔数字输入可见。
12. MCP 打开 `http://192.168.0.202:3003/system-settings/operations/monitoring`，Monitoring & Alerts 页面正常渲染，测试间隔等数字输入可见。
13. MCP 打开 `http://192.168.0.202:3003/system-settings/models/routing-reliability`，Routing Reliability 页面正常渲染，重试次数和测试间隔数字输入可见。
14. MCP 打开 `http://192.168.0.202:3003/system-settings/models/grok`，Grok Settings 页面正常渲染，违规扣费金额数字输入可见。
15. MCP 控制台检查 `error`、`warn`、`issue` 类型消息，无新增控制台错误、警告或浏览器 issue。

## 本轮实施评审：渠道编辑页模型搜索添加对齐 new-api

### 需求分析

用户反馈“搜索添加时不正确”，并明确指出最新版 new-api 的编辑渠道页面体验更好，需要将当前项目编辑渠道页面与 `/opt/project/new-api-main` 对齐。结合页面实际验证，当前项目在 `ChannelMutateDrawer` 的模型选择框里额外加入了“远程模型元信息搜索 + Add N new model(s)”批量添加面板；最新版 new-api 已经没有这套入口，而是保留更清晰的三种模型维护路径：在 MultiSelect 中搜索/选择已有模型、按输入值添加自定义模型、通过 Fetch Models 弹窗从上游拉取并确认。

当前项目 `/api/models/search?keyword=gpt-5.6&p=1&page_size=50` 实测返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个模型，说明后端模型元信息搜索没有缺数据。问题更可能来自 UI 语义混淆：管理员在同一个输入框里既能点单个下拉候选，又能点批量添加搜索结果，还可能被“部分匹配候选存在时隐藏自定义添加项”的逻辑影响，最终感知为搜索后只添加了一个模型或添加了非预期内容。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 移除渠道模型选择区的远程模型元信息搜索批量添加面板；保留 `/api/models/search` 作为 MultiSelect 候选补全，使已同步但尚未加入渠道的模型仍可单个选择；新建/编辑模式均通过 Fetch Models 弹窗确认上游模型。 |
| 上游模型弹窗 | `web/default/src/features/channels/components/dialogs/fetch-models-dialog.tsx` | 对齐 new-api，支持 `customFetcher`、`existingModelsOverride` 和 `channelName`，使新建渠道也能先弹窗选择，再写回表单而不是直接合并。 |
| 通用 MultiSelect | `web/default/src/components/multi-select.tsx` | 保留当前项目已有的本地过滤和 chip 复制能力，但将自定义添加判断调整为只在精确重复时隐藏创建项，避免部分匹配阻止添加完整自定义模型名。 |
| i18n | `web/default/src/i18n/static-keys.ts` | 清理不再使用的搜索批量添加静态文案，保留实际仍使用的通用文案，并运行 `i18n:sync` 校验。 |
| 报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案和验证结果，延续差异原生化报告。 |

### 风险评估

- 本轮只修改前端表单交互和弹窗确认流程，不改后端 Channel API、模型元信息 API、数据库结构或 relay 行为。
- 移除“远程模型元信息一键添加”会改变当前项目独有增强入口；但该入口正是本轮反馈的主要问题来源，且最新版 new-api 已移除同类设计。本轮保留远程模型元信息搜索作为候选补全，避免 `gpt-5.6-luna`、`gpt-5.6-terra` 这类已同步模型因为尚未出现在 `/api/channel/models` 而不可选。
- Fetch Models 新建模式从“直接合并到表单”改为“弹窗中勾选后写回表单”，会多一步确认，但可避免误覆盖和误加模型，是更稳定的管理员工作流。
- 编辑模式的 Fetch Models 弹窗需要读取表单当前未保存的模型值作为初始选择，否则会用数据库旧值覆盖管理员刚做的草稿；本轮将显式传入 `existingModelsOverride`。
- MultiSelect 自定义添加判断回到精确匹配语义后，输入 `gpt-5.6` 时可能同时看到自定义创建项和 `gpt-5.6-*` 候选。该行为与 new-api 一致，且不会再出现隐藏自定义入口导致管理员只能选择非预期候选的情况。
- 权限仍沿用当前项目的 `useChannelPermissions`，没有照搬 new-api 的权限模块，避免破坏 NexusTok 已有账号池、Codex OAuth 和敏感字段控制。

### 方案评审

采用“最小对齐 + 保留 NexusTok 本地能力”的方案：

1. 参考 `/opt/project/new-api-main/web/classic/src/components/table/channels/modals/EditChannelModal.jsx`：new-api 使用 `Form.Select allowCreate` 负责模型搜索/选择/创建，`获取模型列表` 只打开 `ModelSelectModal` 确认上游返回模型，没有额外的“搜索结果批量添加”面板。
2. 在 `ChannelMutateDrawer` 中移除搜索结果统计、搜索结果预览和 `handleAddModelSearchResults`，模型选择框只负责搜索/选择/创建；保留 `searchModels`、`useDebounce`、`modelSearchKeyword` 和 `modelSearchData`，但仅用于把 `/api/models/search` 返回的真实 `model_name` 合并为下拉候选。
3. `modelOptions` 合并 `modelSearchModelNames`、`allModelsList` 与 `currentModelsArray`：远程搜索候选优先进入列表，系统模型和历史模型仍完整保留；远程接口失败或为空时自动退回本地候选，不阻断渠道编辑。
4. `handleFetchModels` 对新建和编辑渠道统一打开 `FetchModelsDialog`；新建模式只在缺少 key 时拦截，不再直接拉取并合并。
5. 给 `FetchModelsDialog` 增加 `customFetcher` 和 `existingModelsOverride`：新建渠道用表单内 type/key/base_url 拉取上游模型；编辑渠道用当前表单内 models 初始化选择，避免未保存草稿被旧数据覆盖。
6. 调整 `MultiSelect` 的 `canCreate`，保留本地候选过滤和受控搜索扩展能力，但只在输入值与已有 option/selected 精确相等时隐藏创建项；部分匹配候选不再阻止输入值本身作为自定义模型添加。
7. 执行前端单测、i18n sync、typecheck、build、diff check，并通过 MCP 打开 `http://192.168.0.202:3003/channels` 实测编辑渠道模型搜索、Fetch Models 弹窗和控制台错误。

### 验收方式

1. `cd web/default && bun test src/components/multi-select.test.ts`。
2. `cd web/default && bun run i18n:sync`。
3. `cd web/default && bun run typecheck`。
4. `cd web/default && bun run build`。
5. `git diff --check`。
6. MCP 打开 `http://192.168.0.202:3003/channels`，编辑现有渠道，确认输入 `gpt-5.6` 时不再出现额外 `Search results / Add N new model(s)` 批量添加面板，候选包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，同时仍显示 `Add custom model "gpt-5.6"`，且不保存也不会写入运行态数据。
7. MCP 验证 Fetch Models 入口会打开弹窗，弹窗取消不改变渠道；控制台无新增 error/warn。

### 实施结果

已完成渠道编辑页模型搜索添加对齐 new-api。本轮没有照搬 new-api 的 Semi UI 表单，而是将其更清晰的模型维护语义转换为 NexusTok 默认前端的原生能力：模型 MultiSelect 负责搜索、选择和自定义创建，Fetch Models 负责从上游拉取后弹窗确认，取消原先容易造成误解的“Search results / Add N new model(s)”批量追加面板。

`ChannelMutateDrawer` 现在保留 `/api/models/search` 远程模型元信息查询，但只把真实 `model_name` 合并进下拉候选，不再提供批量写入按钮。这样输入 `gpt-5.6` 时，已经同步到模型元信息库的 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 会稳定出现在候选里，同时 `Add custom model "gpt-5.6"` 仍可用于添加完整自定义模型名。该实现既保留当前项目模型元信息库的优势，也对齐 new-api 不把搜索结果批量入口混在模型输入框下方的页面结构。

`FetchModelsDialog` 已扩展为新建和编辑渠道共用：编辑模式按现有 channel id 拉取上游模型，新建模式通过表单中的 type/key/base_url 调用后端代理；两种模式都先在弹窗里确认，再回填表单草稿，避免拉取动作直接覆盖模型列表。通用 `MultiSelect` 的自定义创建判定也已抽成 `canCreateMultiSelectValue` 并补充单测，部分匹配候选不再阻止自定义添加，精确重复和大小写不同的已选重复仍会被拦截。

本轮清理了不再使用的搜索批量添加静态 i18n key。现有 locale 文件未被强制删除历史 extra key，遵循项目 `i18n:sync` 当前的保守同步策略。

### 验证记录

1. `cd web/default && bun test src/components/multi-select.test.ts` 通过，7 个用例全部成功，覆盖候选过滤、部分匹配可创建、精确重复不可创建、已选重复大小写不敏感等场景。
2. `cd web/default && bun run i18n:sync` 通过，生成同步报告。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. `rg -n "Search results|Add {{count}} new model(s)|No new search results|Added {{count}} model(s) from search|{{matched}} matched" ...` 无源码命中，确认旧批量搜索添加入口已从默认前端移除。
7. MCP 强刷 `http://192.168.0.202:3003/channels` 后打开渠道 `11111` 编辑抽屉，模型输入框输入 `gpt-5.6`，下拉候选包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 和 `Add custom model "gpt-5.6"`。
8. 同一 MCP 验证中确认页面正文和弹层不再出现 `Search results`、`Add N new model(s)`、`matched · ... already selected` 等旧批量面板文案；验证期间未点击 `Update Channel`，没有保存运行态数据。
9. MCP 在浏览器上下文直接请求 `/api/models/search?keyword=gpt-5.6&p=1&page_size=50`，响应状态 200，返回模型名为 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，说明后端模型元信息同步结果完整，前端候选展示已正确消费。
10. 当前 `11111` 渠道处于 Account Pool 凭证模式，编辑抽屉内按既有规则不展示 `Fetch from Upstream`；通过行菜单 `Fetch Models` 验证弹窗可打开并显示 `Fetch available models for: 11111`，随后关闭弹窗，未保存任何变更。该账号池渠道的 `/api/channel/fetch_models/1` 在验证环境中保持 pending，属于上游探测运行态行为，本轮未修改后端探测逻辑。
11. MCP 打开 `http://192.168.0.202:3003/`，首页正常渲染；根页面控制台无 error/warn/issue。
12. `/channels` 验证期间控制台无 error/warn；DevTools Issue 面板仍显示既有表单可访问性提示（autocomplete 和 label 关联），与本轮模型搜索添加修复无直接关系，未阻断渠道编辑流程。

## 本轮实施评审：Grok 设置 dotted key 表单修复

### 需求分析

继续对照 `/opt/project/new-api-main` 默认前端后确认，new-api 已修复 `GrokSettingsCard` 中 react-hook-form dotted key 的状态树问题：`FormField name='grok.violation_deduction_enabled'` 会被 RHF 解析为嵌套路径 `grok.violation_deduction_enabled`，如果 Zod schema 和 defaultValues 仍使用扁平 key `'grok.violation_deduction_enabled'`，表单内部会同时存在扁平值和嵌套值，提交时可能读取不到用户刚刚切换/输入的值，最终表现为 Grok 违规扣费开关或金额保存无效。

NexusTok 当前 `web/default/src/features/system-settings/models/grok-settings-card.tsx` 仍然使用扁平 schema 和扁平 defaultValues，而页面字段名使用 dotted path。上一轮已完成数字输入 NaN 防护，但并没有修复 dotted key 与 RHF 路径语义不一致的问题；这会影响 Grok 违规扣费设置的保存可靠性，属于可以从 new-api 吸收的明确稳定性优势。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| Grok 设置表单 | `web/default/src/features/system-settings/models/grok-settings-card.tsx` | 将表单内部状态改为嵌套 `grok` 对象，提交前再规范化为后端 option 需要的扁平 key。 |
| Grok 表单 helper | `web/default/src/features/system-settings/models/grok-settings-utils.ts` | 新增纯函数负责扁平默认值、嵌套表单值、保存 payload 之间的转换，便于单测覆盖。 |
| Grok helper 测试 | `web/default/src/features/system-settings/models/grok-settings-utils.test.ts` | 覆盖默认值构建、表单值归一化、变更 key 识别和无变更判断。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证记录。 |

### 风险评估

- 本轮只修改默认前端 Grok 系统设置表单，不改后端 Grok setting、option 存储、违规扣费服务、relay 错误标准化或配额扣费逻辑。
- 后端 option key 仍保持 `'grok.violation_deduction_enabled'` 和 `'grok.violation_deduction_amount'`，因此数据库兼容性和现有配置读取路径不变。
- 表单内部从扁平 key 改为嵌套对象后，需要确保 `defaultValues` 变化时重置的是嵌套结构；否则系统 option 刷新后页面可能显示旧值。
- 保存时只提交实际变化的扁平 option key。无变化时显示 `No changes to save`，避免点击保存后产生空请求或误导性成功 toast。
- 数字金额字段继续使用上一轮引入的 `safeNumberFieldProps`，本轮不改变空输入/非法数字处理语义。

### 方案评审

采用“new-api 语义 + NexusTok 本地表单布局”的低风险方案：

1. 新增 `grok-settings-utils.ts`，定义 `FlatGrokSettings`、`GrokFormInput`、`GrokFormValues`、`buildGrokFormDefaults`、`normalizeGrokFormValues` 和 `getChangedGrokSettingKeys`。
2. `GrokSettingsCard` 的 Zod schema 改为 `z.object({ grok: z.object(...) })`，`useForm` 使用嵌套 defaultValues，并保留当前 `SettingsSection`、`FormField`、`Switch`、`Input`、`Button` 布局，不迁移 new-api 的 `SettingsForm`/`SettingsPageFormActions`。
3. 提交时把嵌套 values 规范化为扁平 option key，和当前默认值比较后逐项调用 `useUpdateOption().mutateAsync`；保存成功后把 baseline 更新为已保存值并重置表单，避免重复点击继续提交同一变更。
4. 新增纯函数单测，证明默认值映射和变更识别正确，不依赖浏览器环境。
5. 执行定向单测、i18n sync、typecheck、build、diff check，并通过 MCP 打开 `http://192.168.0.202:3003/system-settings/models/grok` 和首页确认运行态页面正常；验证期间不点击保存。

### 验收方式

1. `cd web/default && bun test src/features/system-settings/models/grok-settings-utils.test.ts`。
2. `cd web/default && bun run i18n:sync`。
3. `cd web/default && bun run typecheck`。
4. `cd web/default && bun run build`。
5. `git diff --check`。
6. MCP 打开 `http://192.168.0.202:3003/system-settings/models/grok`，确认 Grok 设置页正常渲染，开关与金额字段仍可见；验证期间不保存设置。
7. MCP 打开 `http://192.168.0.202:3003/`，确认首页正常渲染；控制台无新增 error/warn。

### 实施结果

已完成 Grok 设置 dotted key 表单修复。本轮把 Grok 设置的内部表单状态从扁平 key 改成嵌套 `grok` 对象，使 `FormField name='grok.violation_deduction_enabled'` 与 react-hook-form 的路径语义一致；提交时再通过 `normalizeGrokFormValues` 转回后端 option 需要的 `'grok.violation_deduction_enabled'` 和 `'grok.violation_deduction_amount'` 两个扁平 key。

新增 `grok-settings-utils.ts` 固化这套转换关系，包含默认值构建、提交值归一化和变更 key 识别。`GrokSettingsCard` 保存时只提交相对 baseline 发生变化的 key，无变化时提示 `No changes to save`；保存成功后会更新 baseline 并重置表单，避免重复点击保存继续提交同一变更。默认值刷新仍使用序列化快照保护，父组件重渲染但实际设置值未变化时不会擦掉管理员草稿。

该能力吸收了 new-api 默认前端的 dotted key 修复思路，但保留 NexusTok 当前 `SettingsSection`、权限 hook、`useUpdateOption`、`safeNumberFieldProps` 和保存按钮布局，没有改后端配置结构、违规扣费服务、relay 错误标准化或配额计算。

### 验证记录

1. `cd web/default && bun test src/features/system-settings/models/grok-settings-utils.test.ts` 通过，4 个用例全部成功。
2. `cd web/default && bun run i18n:sync` 通过；本轮没有新增用户可见文案。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. MCP 打开 `http://192.168.0.202:3003/system-settings/models/grok`，Grok 设置页正常渲染，`Enable violation deduction` 开关、`Violation deduction amount` 数字输入和保存按钮可见；验证期间未点击保存，未写入运行态系统 option。
7. MCP 在 Grok 设置页浏览器上下文检查数字输入，实际 DOM `input.value` 为 `0.05`，`input.valueAsNumber` 为 `0.05`，说明页面展示值保持干净。
8. MCP 打开 `http://192.168.0.202:3003/`，首页正常渲染。
9. MCP 验证 Grok 设置页和首页控制台均无 error/warn/issue。

## 本轮实施评审：渠道编辑页搜索补齐与 new-api 页面结构再对齐

### 需求分析

用户继续反馈“搜索添加时不正确”，并指出最新版 new-api 的编辑渠道页面体验较好，需要将当前编辑渠道页面继续向 `/opt/project/new-api-main` 对齐。MCP 复现当前运行态后确认：渠道 `11111` 的模型列表只有 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`，而 `/api/models/search?keyword=gpt-5.6&p=1&page_size=50` 实际返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个模型。也就是说，后端模型库和 OpenAI 供应商模型元信息完整，问题在编辑页搜索后没有提供清晰的“补齐本次搜索命中的缺失模型”动作，管理员只能逐个点候选，容易出现“三个模型只加了一个”的结果。

上一轮为了避免旧批量搜索面板造成语义混淆，移除了 `Search results / Add N new model(s)` 面板；但当前场景证明 NexusTok 的原生模型库搜索比 new-api 更强，应当保留这项优势，只是需要用更明确、更局部、更安全的方式落地：搜索框仍负责单个选择/自定义添加，搜索命中的缺失模型则在模型搜索弹层内部显示一个轻量提示和批量追加按钮，按钮只追加当前关键词命中的缺失项，不覆盖已有模型，不直接保存渠道。

同时对照 `/opt/project/new-api-main/web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`，最新版 new-api 将 `Models`、`Model Mapping`、`Groups` 拆成三个独立卡片；当前 NexusTok 的 `Groups` 被放在 `Model Mapping` 卡片内部，页面结构比 new-api 更拥挤。本轮需要将 `Groups` 独立成单独卡片，并保留 NexusTok 已有权限控制、账号池、模型映射 guardrail、Codex 等本地能力。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 新增当前搜索命中的缺失模型集合、批量追加 handler 和弹层内补齐提示；将 `Groups` 从 `Model Mapping` 卡片拆出，结构对齐 new-api。 |
| 通用 MultiSelect | `web/default/src/components/multi-select.tsx` | 新增可选 `contentFooter` 插槽，使调用方能把搜索补齐动作放在 Combobox 弹层内部；继续保持远程搜索受控输入、候选过滤和自定义添加能力。 |
| i18n | `web/default/src/i18n/locales/*.json`、`static-keys.ts` | 优先复用已有 `Add {{count}} new model(s)`、`Added {{count}} model(s) from search`、`No new models to add` 等翻译；如新增文案则运行 `i18n:sync`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮二次评审、实施结果和验证记录，说明为什么从“移除批量面板”调整为“受控搜索补齐”。 |

### 风险评估

- 本轮只修改默认前端编辑抽屉，不改后端模型搜索接口、渠道保存接口、数据库结构、relay 路由或计费逻辑。
- 批量追加按钮只写入前端表单草稿，不直接调用 `updateChannel`；管理员仍需点击 `Update Channel` 才会保存，因此不会因搜索动作误改运行态渠道。
- 按钮只追加 `model_name` 包含当前关键词、且当前渠道尚未包含的模型；已经存在的 `gpt-5.6-sol` 不会重复追加，`gpt-5.6-terra`、`gpt-5.6-luna` 会被补齐。
- 若远程搜索还在 debounce/loading，按钮不会提前显示，避免把不完整结果当成最终结果。
- `Groups` 拆卡片只影响视觉和 DOM 结构，不改变字段名、表单值、校验 schema 或提交 payload。
- 权限仍沿用 `canEditBasicFields`，无权限时批量追加按钮禁用并使用既有无权限提示。

### 方案评审

采用“new-api 页面结构 + NexusTok 原生模型库补齐”的方案：

1. 保留 `searchModels` 查询和 `modelSearchModelNames` 过滤，继续只接受真实 `model_name` 包含关键词的条目，避免 tags/description 命中的无关模型进入渠道模型候选。
2. 新增 `modelSearchMissingModelNames`：以大小写不敏感方式比较当前渠道模型，只保留本次搜索命中但尚未加入的模型。
3. 在模型 MultiSelect 弹层内部显示轻量提示：搜索关键词存在、远程搜索完成且存在缺失模型时，显示 `Search results`、缺失模型预览和 `Add {{count}} new model(s)` 按钮；点击后调用 `updateModels(models, true)` 追加并去重，清空搜索词，toast 使用 `Added {{count}} model(s) from search`。MCP 真实点击验证发现，若按钮放在 Combobox 弹层外部，会被 Base UI DismissLayer 拦截指针事件，因此最终方案改为弹层内 footer。
4. 远程搜索完成但没有缺失模型时不显示额外提示，避免在普通单选/自定义添加场景产生噪音。
5. 将 `Groups` `FormField` 从 `Model Mapping` 卡片中移出，包裹在独立 `border-border/60 rounded-lg border p-4` 卡片内；保留 NexusTok 当前 `FormDescription`、权限拦截和 `MultiSelect`。
6. 补充单测覆盖搜索缺失模型计算，避免未来再次出现“三个命中只补一个”的回归。
7. 执行前端单测、i18n sync、typecheck、build、diff check，并通过 MCP 在 3003 页面实测 `gpt-5.6` 搜索能显示两个缺失模型、一键追加后表单中包含三种 `gpt-5.6` 模型；验证时不点击保存，避免改动运行态渠道。

### 验收方式

1. `cd web/default && bun test src/components/multi-select.test.ts`。
2. 如新增渠道 helper 测试，执行对应 `bun test`。
3. `cd web/default && bun run i18n:sync`。
4. `cd web/default && bun run typecheck`。
5. `cd web/default && bun run build`。
6. `git diff --check`。
7. MCP 打开 `http://192.168.0.202:3003/channels`，编辑渠道 `11111`，输入 `gpt-5.6`，确认搜索接口返回 3 个模型，页面显示可追加的缺失模型为 `gpt-5.6-terra` 和 `gpt-5.6-luna`，点击批量追加后表单草稿包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`。
8. MCP 确认 `Groups` 在模型映射下方独立成卡片，页面无新增 console error/warn/issue；验证期间不点击 `Update Channel`。

### 实施结果

已完成渠道编辑页模型搜索补齐修复，并继续向最新版 new-api 编辑渠道页面结构对齐。本轮没有修改后端模型搜索接口、渠道保存接口、数据库结构或 relay 逻辑；所有改动集中在默认前端。

模型搜索补齐方面，新增 `getMissingModelSearchMatches` 纯函数，用大小写不敏感方式计算“本次搜索命中但当前渠道尚未包含”的模型，并新增单测覆盖 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 场景。`updateModels` 现在在合并模式下从 `form.getValues('models')` 读取最新表单草稿再追加，避免 Combobox 关闭或旧闭包把新增模型覆盖回旧值；返回值也改为实际新增数量，toast 能准确显示 `Added 2 model(s) from search`。

MCP 真实点击首次复测发现：把 `Search results / Add {{count}} new model(s)` 放在模型字段下方时，提示内容能显示，但按钮实际点击会被 Base UI Combobox 的 DismissLayer 接走，页面停留在 `Selected 3`。因此本轮进一步给通用 `MultiSelect` 增加可选 `contentFooter` 插槽，将搜索补齐提示渲染在 `ComboboxContent` 内部；这样按钮和候选列表处于同一个弹层交互上下文，真实鼠标点击可以稳定进入 handler。该插槽是可选能力，默认不渲染，不影响其他多选使用方。

页面结构方面，`Groups` 已从 `Model Mapping` 卡片内部拆出为独立卡片，和 `/opt/project/new-api-main/web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 的 `Models`、`Model Mapping`、`Groups` 三段结构一致；同时保留 NexusTok 当前账号池凭证模式、模型映射 guardrail、Codex 渠道、权限控制和快捷填充能力。

本轮验证只操作前端表单草稿，没有点击 `Update Channel`，因此渠道 `11111` 的运行态配置未被保存修改。

### 验证记录

1. `cd web/default && bun test src/features/channels/lib/model-search.test.ts src/components/multi-select.test.ts` 通过，10 个用例全部成功。
2. `cd web/default && bun run i18n:sync` 通过；本轮复用已有 `Search results`、`Add {{count}} new model(s)`、`Added {{count}} model(s) from search` 等翻译键。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. 已重启 `nexustok-frontend-watch` 与 `nexustok-api-hot`，确认 `dist.hot` 重新发布并且 `http://192.168.0.202:3003/api/status` 返回 200 后，再用 MCP 忽略缓存刷新 `http://192.168.0.202:3003/channels`。
7. MCP 打开渠道 `11111` 编辑抽屉，初始模型为 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`，显示 `Selected 3`。
8. MCP 在模型搜索框输入 `gpt-5.6`，网络请求 `GET /api/models/search?keyword=gpt-5.6&p=1&page_size=50` 返回 `total: 3`，三条模型分别是 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，均为 `icon: OpenAI.Color`、`vendor_id: 1`、`sync_official: 1`，说明 OpenAI 供应商模型已完整同步。
9. MCP 确认补齐提示位于 Combobox 下拉弹层内部，显示 `Search results`、`gpt-5.6-terra, gpt-5.6-luna` 和 `Add 2 new model(s)`。
10. MCP 真实点击 `Add 2 new model(s)` 后，页面 toast 显示 `Added 2 model(s) from search`；浏览器上下文读取表单文本确认 `Selected 5`，并同时包含 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`。
11. MCP 点击 `Cancel` 返回渠道列表；验证期间未点击 `Update Channel`，未保存渠道变更。
12. MCP `/channels` 控制台无 error/warn；DevTools Issue 面板仍显示既有表单 `autocomplete` 与部分复合编辑器 `label for` 关联提示，定位到 `name`、`priority`、`Model Mapping`、`Status Code Mapping`、`Parameter Override` 等既有字段，与本轮模型搜索补齐和 `contentFooter` 改动无直接关系。

## 本轮实施评审：渠道编辑页搜索添加二次修复与 new-api 最新结构对齐

### 需求分析

用户继续反馈“搜索添加时不正确”，并要求参考 `/opt/project/new-api-main` 最新版编辑渠道页面继续对齐。复核当前默认前端后确认，上一轮已经把模型搜索结果 footer 放入 `MultiSelect` 弹层，但 `MultiSelect` 的自定义添加入口仍会在存在搜索候选时展示 `Add custom model "{{value}}"`。当管理员输入 `gpt-5.6` 这类系列关键词时，页面同时存在“批量补齐搜索命中模型”和“把关键词本身当作自定义模型添加”两个动作；按 Enter 或误点自定义入口会添加 `gpt-5.6` 这个不完整关键词，而不是补齐 `gpt-5.6-terra`、`gpt-5.6-luna` 等真实模型。这与“搜索添加”语义不一致。

同时对照最新版 new-api 的编辑渠道页面，当前 NexusTok advanced 区域还有两个结构偏差：`Routing & Overrides` 外层误用了 `ADVANCED_SETTINGS_SECTION_IDS.extraSettings` 作为锚点，导致高级导航的 `Channel Extra Settings` 锚点定位不稳定；已配置的高级子区块虽然已计算 `routingStrategyConfigured`、`internalNotesConfigured`、`overrideRulesConfigured` 等状态，但渲染没有使用 new-api 的已配置高亮容器，页面扫描感弱于 new-api。

本轮目标是二次修复搜索添加的歧义，并把高级设置区域的锚点、分区和已配置态视觉向 new-api 最新实现对齐，同时保留 NexusTok 当前已有的权限控制、账号池、Codex OAuth、模型库搜索和字段级 fail-closed 能力。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 通用 MultiSelect | `web/default/src/components/multi-select.tsx` | 新增可选策略：当当前搜索词已经匹配候选项时，可隐藏自定义创建项，避免把系列搜索词误加入为模型名；默认行为保持兼容。 |
| MultiSelect 单测 | `web/default/src/components/multi-select.test.ts` | 补充“存在候选时禁止创建自定义值”的纯函数测试，固定搜索添加语义。 |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 模型选择器启用新策略；修正 advanced 子区块锚点；引入 new-api 风格的 configured 高亮容器；补齐 Codex/OpenAI 类型的字段透传区块显示条件。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮二次修复的需求分析、影响范围、风险评估、方案评审和验证记录。 |

### 风险评估

- 本轮只修改默认前端交互和布局，不修改后端模型搜索接口、渠道保存接口、数据库结构、relay 路由、计费或权限路由。
- `MultiSelect` 新策略是可选 prop，默认仍允许“部分匹配候选不阻止自定义添加”，避免影响密钥页、用户页、模型预设等其它多选使用场景。
- 渠道模型选择器启用该策略后，存在候选时不再展示 `Add custom model "gpt-5.6"`，但没有候选时仍允许添加真正的自定义模型。
- 搜索补齐按钮仍只修改前端表单草稿，不直接调用 `updateChannel`；管理员必须保存渠道才会写入运行态配置。
- advanced 锚点修复会改变 DOM 定位和视觉边框，但不改变字段名、表单值、校验 schema、提交 payload 或权限判断。
- `Field passthrough controls` 对 `currentType === 57` 显示条件与已有 `fieldPassthroughConfigured` 计算保持一致，属于补齐已有表单能力的可见性，不新增后端语义。

### 方案评审

采用“小步兼容 + 页面结构对齐”的方案：

1. 在 `MultiSelect` 中新增 `allowCreateWithMatches`，默认 `true`；当调用方传 `false` 时，如果输入值能匹配任何候选值或候选 label，则隐藏自定义创建项。
2. `canCreateMultiSelectValue` 增加 `hasMatchingOption` 与 `allowCreateWithMatches` 参数，继续保持精确重复、已选重复和 loading 禁止创建的既有规则。
3. 渠道模型选择器传入 `allowCreateWithMatches={false}`，使 `gpt-5.6` 这类搜索词优先用于筛选/批量补齐真实模型；只有没有候选时才作为自定义模型创建。
4. 保留弹层内 `Search results / Add {{count}} new model(s)` footer，不改搜索接口和 `getMissingModelSearchMatches` 的缺失模型计算。
5. 将 advanced 区域的第一个大块从错误的 `extraSettings` 锚点改为普通 section；`routingStrategy`、`internalNotes`、`overrideRules` 用 new-api 的 `configuredAdvancedSectionClassName` 标记已配置态。
6. 将 `Channel Extra Settings` 外层改为真实 `extraSettings` 锚点和 new-api section 容器；`fieldPassthrough`、`upstreamModelDetection` 子块也使用 configured 高亮，方便管理员快速扫出已启用配置。
7. 运行前端单测、i18n sync、typecheck、build、diff check，并通过 3003 页面/接口确认热更新服务可访问；由于当前会话没有 MCP 浏览器工具，若无法用真实浏览器工具操作页面，最终报告中明确说明替代验证方式。

### 验收方式

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts`。
2. `cd web/default && bun run i18n:sync`。
3. `cd web/default && bun run typecheck`。
4. `cd web/default && bun run build`。
5. `git diff --check`。
6. 访问 `http://192.168.0.202:3003/` 和 `/api/status` 确认热更新部署可访问；如页面未更新则重启相关容器后再验证。
7. 若可用浏览器验证，打开 `/channels` 编辑渠道，在模型搜索框输入 `gpt-5.6`，确认存在候选时不再出现 `Add custom model "gpt-5.6"`，批量追加按钮仍能补齐缺失真实模型；advanced 导航点击 `Channel Extra Settings` 能定位到正确区块。

### 实施结果

已完成渠道编辑页搜索添加二次修复，并继续向最新版 new-api 编辑渠道页面结构对齐。本轮没有修改后端模型搜索接口、渠道保存接口、数据库结构、relay 路由、计费或权限路由；改动仍集中在默认前端交互和布局。

通用 `MultiSelect` 新增 `allowCreateWithMatches` 可选策略，默认值为 `true`，保持其它调用方原有“部分匹配候选仍可创建自定义值”的行为。渠道模型选择器显式传入 `allowCreateWithMatches={false}`：当管理员输入 `gpt-5.6` 且候选列表中已经存在 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 等真实模型时，下拉列表不再展示 `Add custom model "gpt-5.6"`，避免把系列搜索词误加入渠道模型；如果搜索词没有任何候选，仍保留自定义模型添加能力。

`canCreateMultiSelectValue` 已增加 `hasMatchingOption` 判定，并补充纯函数测试固定三种语义：默认兼容旧行为、调用方可要求存在候选时隐藏自定义创建、无候选时仍允许真正自定义值。该策略与上一轮已落地的 `Search results / Add {{count}} new model(s)` footer 配合后，`gpt-5.6` 的正确操作路径变成“搜索真实模型并一键补齐缺失项”，而不是添加 `gpt-5.6` 这个不完整关键词。

编辑渠道页面 advanced 区域已继续对齐 new-api 最新结构：`Routing & Overrides` 外层不再占用 `extraSettings` 锚点，`Channel Extra Settings` 才是 `ADVANCED_SETTINGS_SECTION_IDS.extraSettings` 的真实定位目标；`routingStrategy`、`internalNotes`、`overrideRules`、`fieldPassthrough`、`upstreamModelDetection` 均使用已配置高亮容器，管理员能更快扫出已启用的高级配置。`Field passthrough controls` 也补齐 `currentType === 57` 的显示条件，使 Codex/OpenAI 类型渠道在可见性上与已有配置状态计算一致。

本轮保留 NexusTok 原生能力：渠道字段级权限、账号池凭据模式、Codex OAuth、模型映射 guardrail、模型库搜索补齐 footer、敏感字段 fail-closed 和现有保存 payload 均未被削弱。页面验证期间只操作表单草稿，没有点击 `Update Channel`，因此渠道运行态配置未被保存修改。

### 验证记录

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts` 通过，12 个用例全部成功，覆盖 `MultiSelect` 自定义创建策略和渠道模型搜索缺失项计算。
2. `cd web/default && bun run i18n:sync` 通过；本轮没有新增用户可见翻译 key。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回首页 200；`curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回状态 JSON。
7. `docker logs --tail 80 nexustok-frontend-watch` 显示 `[hot] published default dist`，说明热更新产物已发布到当前 3003 服务使用的 `dist.hot`。
8. 当前会话未暴露 MCP 浏览器工具，已改用 Chrome headless + CDP 做真实页面验证。登录账号 `c1cada` 后打开 `http://192.168.0.202:3003/channels`，页面成功进入渠道列表，未出现登录页回退。
9. Chrome headless/CDP 打开渠道 `11111` 编辑抽屉，初始模型显示 `Selected 3`，包含 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`，页面同时显示 `Models & Groups`、`Model Mapping`、`Groups` 和 `Advanced Settings`。
10. 在模型搜索框输入 `gpt-5.6` 后，真实触发 `GET /api/models/search?keyword=gpt-5.6&p=1&page_size=50` 并返回 200；下拉候选显示 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，其中 `gpt-5.6-sol` 已勾选。
11. 页面显示 `Search results`、`gpt-5.6-terra, gpt-5.6-luna` 和 `Add 2 new model(s)`；未出现 `Add custom model "gpt-5.6"` 或 `Add "gpt-5.6"`，确认“搜索添加时误加关键词本身”的问题已修复。
12. Chrome headless/CDP 真实点击 `Add 2 new model(s)` 后，表单草稿变为 `Selected 5`，并同时包含 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`；验证期间未点击 `Update Channel`，未保存渠道变更。

## 本轮报告校准：安全边界能力状态复核

### 需求分析

持续对照 `/opt/project/new-api-main` 的过程中，差异报告顶部总表已经把匿名请求体限制、HeaderNavModuleAuth、受保护 Fetch/SSRF 和系统设置路由权限表标记为已落地或部分落地，但中部 `P1：安全边界` 与“建议的落地顺序”仍保留“增加匿名请求体限制”“增加 SSRF 保护客户端”“增加 HeaderNavModuleAuth”等待办式描述。该表述会误导后续迁移计划，让已经原生化的安全能力被重复纳入待开发范围。

本轮目标不是新增业务能力，而是基于当前仓库证据校准报告状态：只把已经能从代码、路由和测试中确认的能力改为“已原生化首批 + 后续维护契约”。账号池字段级 fail-closed 已在后续实施切片补齐，Authz override/Casbin、SSRF 覆盖缺口审计继续保留为后续待办。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 差异报告顶部总表 | `docs/features/new-api-main-diff-analysis.md` | 将匿名请求体限制、顶栏模块鉴权和受保护 Fetch/SSRF 的原生化建议改为维护契约，明确新增入口的后续约束。 |
| P1 安全边界路线 | `docs/features/new-api-main-diff-analysis.md` | 把已完成首批的安全能力从“待增加”改为“已落地，后续审计/维护”；账号池字段级 fail-closed 已在后续实施切片补齐。 |
| 建议落地顺序 | `docs/features/new-api-main-diff-analysis.md` | 标注安全小步快跑第一批已完成，下一批聚焦权限和覆盖缺口。 |
| 实施记录索引 | `docs/features/new-api-main-diff-analysis.md` | 新增 2026-07-10 状态校准记录，便于后续回溯本轮只改文档、不改业务代码。 |

### 风险评估

- 本轮只修改 Markdown 文档，不改变 Go/React 代码、路由注册、数据库迁移、环境变量默认值、热更新构建产物或运行态配置。
- 主要风险是把尚未完成的能力误标为完成；因此只校准有明确证据的条目：匿名请求体限制、HeaderNavModuleAuth、用户可控 URL 的 protected fetch 首批接入，以及 `/api/status/test` 已归入系统设置权限路由。
- SSRF 条目不声明“所有网络请求已替换”，而是明确限定为“用户可控 URL 专用 client 首批已落地”；Relay 全局 client 继续作为显式例外保留。
- HeaderNav 条目不只看后端中间件，也结合默认前端集中解析、pricing/rankings 路由守卫和实施记录，避免把单点能力误判为全链路完成。
- Authz override/Casbin、SSRF 覆盖缺口审计和新增公开模块的测试契约仍保留为后续待办，避免过度乐观。

### 方案评审

采用“证据驱动的文档校准”方案：

1. 以当前仓库文件为准复核能力状态，不根据旧计划或主观印象改状态。
2. 匿名请求体限制以 `router/api-router.go` 中 setup、register、login、2FA、Passkey、OAuth bind、Webhook 等路由挂载 `anonymousRequestBodyLimit`，以及 `middleware/request_body_limit_test.go` 回归测试为完成依据。
3. HeaderNavModuleAuth 以后端 `middleware/header_nav.go`、`router/api-router.go` pricing/rankings 挂载、`middleware/header_nav_test.go`、默认前端 `nav-modules.ts` 和 pricing/rankings 路由守卫实施记录为完成依据。
4. SSRF 保护以 `service/protected_fetch_client.go`、`service/download.go`、`service/webhook.go`、`service/user_notify.go` 和 `service/protected_fetch_client_test.go` 为首批完成依据，同时在报告中保留 Relay 上游地址不替换的例外说明。
5. `/api/status/test` 以 `router/system-setting-router.go`、`router/system_setting_router_test.go` 和前文系统设置路由权限表实施记录为完成依据，不再重复列为待迁移散落路由。
6. 文档只更新已证实条目，未证实或仍处于部分完成状态的功能不改为完成。

### 实施结果

已完成安全边界能力状态校准：顶部总表、`P1：安全边界`、建议落地顺序和实施记录索引均已更新。报告现在明确表达：

1. 匿名请求体限制已经是 NexusTok 原生匿名入口保护能力，后续新增匿名 POST 路由必须显式评估是否挂载。
2. HeaderNavModuleAuth 已形成后端接口鉴权、前端配置解析和页面路由守卫的闭环，后续新增公开导航模块必须同步维护三处契约。
3. 受保护 Fetch/SSRF 已覆盖首批用户可控 URL 拉取链路，Relay 全局 client 保留为显式例外，后续重点是覆盖缺口审计。
4. 安全小步快跑第一批已完成；账号池字段级 fail-closed 已在后续切片补齐，下一批重点转向 SSRF 覆盖缺口审计、Authz override/Casbin 和公开模块新增时的测试契约。

### 验证记录

1. `rg -n "AnonymousRequestBodyLimit|HeaderNavModuleAuth|GetSSRFProtectedHTTPClient|/test|TestStatus" common middleware router service controller -S` 已复核对应代码、路由和测试存在。
2. `rg -n "P1：安全边界|匿名请求体|HeaderNav|SSRF|status/test|建议的落地顺序" docs/features/new-api-main-diff-analysis.md` 已定位并校准旧表述。
3. 本轮无业务代码改动，因此不需要运行 Go 单测、前端 typecheck 或构建；执行 `git diff --check` 作为文档格式检查。
4. 当前会话未暴露浏览器 MCP/DevTools 工具，已通过工具发现确认不可用；本轮改用 `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 验证当前热更新部署首页返回 200。由于本轮仅修改文档，页面不会出现业务 UI 变化。

## 本轮实施评审：账号池分组字段级 fail-closed

### 需求分析

前面已经将 `/api/account-pool` 接入 `account_pool` 权限表：查询、操作、普通写和敏感写在路由级已经区分。但分组更新接口 `PUT /api/account-pool/groups/:id` 仍是一个混合 payload，只要求 `account_pool.write`。该 payload 同时包含普通展示/调度字段和会改变上游语义的高风险字段，例如 `platform`、`auth_type`、`model_mapping`、`settings`。普通 Admin 具备 `account_pool.write` 但默认不具备 `account_pool.sensitive_write`，如果不做字段级二次校验，就会比渠道编辑页已经落地的 fail-closed 策略更松。

本轮目标是把 new-api-main 的细粒度权限优势转为 NexusTok 原生账号池保护能力：分组更新时，普通字段仍允许具备 `account_pool.write` 的管理员维护；一旦请求修改敏感字段或携带未知字段，后端必须额外要求 `account_pool.sensitive_write`。默认前端也应消费同一权限矩阵，在编辑既有分组时禁用敏感字段，避免用户先填后被服务端拒绝。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 账号池分组后端鉴权 | `controller/account_pool_authz.go`、`controller/account_pool.go` | 新增分组更新字段分类和敏感变更判定；`PUT /api/account-pool/groups/:id` 在写入前进行敏感写二次校验。 |
| 账号池分组更新兼容 | `controller/account_pool.go` | 读取原始 JSON，区分字段缺失和显式清空；缺失的 `models`、`group`、`settings` 继承原值，避免局部更新误清空文本字段。 |
| 后端回归测试 | `controller/account_pool_authz_test.go` 或现有账号池测试 | 覆盖普通字段放行、敏感字段拒绝、未知字段 fail-closed、字段分类完整性和局部更新保留文本字段。 |
| 默认前端账号池页 | `web/default/src/features/account-pool/index.tsx` | 消费 `account_pool.write/sensitive_write` 权限；新建/保存/删除/账号生命周期按钮按既有路由权限禁用；编辑既有分组时禁用敏感字段。 |
| i18n | `web/default/src/i18n/locales/*.json` | 尽量复用已有 `You don't have necessary permission` 文案；若新增说明文本则运行 `bun run i18n:sync` 并补齐六语。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审、实施结果和验证记录。 |

### 风险评估

- 分组更新是账号池调度核心接口，误判敏感字段可能阻断 Admin 的日常维护；因此敏感集合先收敛到 `platform`、`auth_type`、`model_mapping`、`settings` 和未知字段，不把名称、模型列表、用户组、状态、策略、限流、每日额度、自动检测、预热和任务提交限流直接升级为敏感写。
- `settings` 是开放 JSON 扩展，未来可能承载代理、额外请求参数或其它高风险配置，必须按敏感字段处理；但历史 `settings.max_concurrency` 清理属于 `max_concurrency` 显式字段的兼容逻辑，不应让普通限流调整被误判为敏感。
- 当前前端编辑分组会发送完整 payload；后端读取原始 JSON 后仍要兼容这种完整提交，同时防止外部客户端只传 `name` 时被旧 update map 误清空 `models`、`group`、`settings`。
- 新建分组仍保持 `account_pool.write`，因为它不会直接修改既有运行态绑定；凭证创建、导入、OAuth、账号生命周期等路径已经在路由级要求 `account_pool.sensitive_write`。
- 前端按钮禁用只是体验层保护，后端字段级校验才是最终边界；受限管理员如果绕过页面发请求，服务端必须拒绝敏感字段变更。

### 方案评审

采用“后端 fail-closed + 前端权限消费”的小步方案：

1. 新增账号池分组字段分类：敏感字段为 `platform`、`auth_type`、`model_mapping`、`settings`；只读字段包括 `id`、来源、统计、用量和时间戳；其它分组维护字段归为普通写。
2. `UpdateAccountPoolGroup` 改为先 `GetRawData()`，用 `common.Unmarshal` 同时解析 DTO 和 `map[string]any`，再基于 requestData 判断字段是否出现，避免未知字段被 Gin bind 静默忽略。
3. 敏感变更判定只在实际值变化时触发；`platform/auth_type` 按模型层规范化比较，`model_mapping` 按 nil 与空字符串等价比较，`settings` 对历史 `max_concurrency` 清理做兼容。
4. 如果 requestData 中存在未分类字段，后端默认按敏感变更处理；Root 或拥有 `account_pool.sensitive_write` override 的管理员可以继续修改。
5. 前端账号池页新增本地权限计算：`write` 控制新建/编辑保存，`operate` 控制检测、状态、导出和清理，`sensitive_write` 控制删除、账号创建/批量导入、凭证/OAuth/刷新和既有分组的敏感字段编辑。
6. 编辑既有分组时，缺少敏感写权限则禁用 `Platform`、`Auth Type`、`Model Mapping`、`Settings JSON`；保存仍允许提交普通字段，后端会兜底拒绝任何绕过禁用态的敏感字段变更。

### 验收方式

1. `go test ./controller -run 'TestAccountPoolGroup'`。
2. `go test ./router -run TestAccountPoolRoutes`。
3. `cd web/default && bun run i18n:sync`。
4. `cd web/default && bun run typecheck`。
5. `cd web/default && bun run build`。
6. `git diff --check`。
7. 优先使用 MCP 打开 `http://192.168.0.202:3003/account-pool/groups` 验证账号池页面仍正常；如当前会话仍无浏览器 MCP 工具，则使用 `curl --noproxy '*'` 访问首页和 `/api/status`，并记录工具限制。

### 实施结果

已完成账号池分组字段级 fail-closed 原生化。本轮新增 `controller/account_pool_authz.go`，把账号池分组更新字段分为敏感、普通写和只读三类：`platform`、`auth_type`、`model_mapping`、`settings` 以及未分类字段默认要求 `account_pool.sensitive_write`；名称、状态、策略、模型列表、用户组、限流、每日额度、自动检测、预热和任务提交限流继续由 `account_pool.write` 维护。

`UpdateAccountPoolGroup` 已改为读取原始 JSON 后分别解析 DTO 和 `requestData`，从而能区分字段缺失、显式清空和未知字段。为避免旧 update map 在局部更新时误清空 `models`、`group`、`settings`，本轮在进入 update map 前用原始分组补齐缺失文本字段；但客户端显式传空字符串时仍保留“清空”语义。历史 `settings.max_concurrency` 在显式提交 `max_concurrency` 时继续按兼容逻辑移除，不会被误判为 settings 敏感变更。

默认前端账号池页已开始消费 `account_pool.write/operate/sensitive_write`：新建/保存分组走 write；检测、状态、导出、清理和运行时重置走 operate；删除分组、创建/更新/删除账号、批量导入、凭证文件导入/编辑/删除、Codex OAuth/Device 和凭证刷新走 sensitive_write。编辑既有分组时，缺少敏感写权限会禁用 `Platform`、`Auth Type`、`Model Mapping`、`Settings JSON`，普通字段仍可维护。

### 验证记录

1. `go test ./controller -run 'TestAccountPoolGroup'` 通过，覆盖普通字段放行、敏感字段拒绝、未知字段 fail-closed、字段分类完整性、局部更新保留文本字段和 legacy `settings.max_concurrency` 清理兼容。
2. `go test ./router -run 'Test(RegisterAccountPoolRoutesKeepsCoreHandlers|AccountPoolPermissionRoutesClassifyCoreActions|AccountPoolPermissionRoutesStayOnAccountPoolResource)'` 通过，确认账号池路由权限表仍保持 read/operate/write/sensitive_write 分类。
3. `cd web/default && bun run i18n:sync` 通过；本轮复用已有 `You don't have necessary permission` 文案，没有新增 locale key。
4. `cd web/default && bun run typecheck` 通过。
5. `cd web/default && bun run build` 通过。
6. `git diff --check` 通过。
7. 当前会话仍未暴露浏览器 MCP/DevTools 工具；已使用 `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 验证首页返回 200，并使用 `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 验证状态接口可访问。

## 本轮实施评审：订阅到期显式降级分组

### 需求分析

当前 NexusTok 已经具备订阅套餐购买后升级用户分组的原生能力：管理员可以在套餐上配置 `upgrade_group`，购买成功后用户会升级到目标分组；订阅到期、取消或删除时，系统会根据订阅快照回退到购买前分组。该逻辑能覆盖“临时升级权益”的基础场景，但它把到期后的目标分组完全绑定到用户购买前状态，无法表达更明确的运营策略。

`/opt/project/new-api-main` 最新版在此基础上增加了 `downgrade_group`：管理员可以为套餐配置到期后显式降级到某个指定分组；若留空，则继续回退到购买前分组。这个能力适合试用套餐、活动套餐和统一降级策略，例如试用结束后统一回到 `default`，而不是依赖用户购买前的历史分组快照。

本轮目标是把该能力转换为 NexusTok 的原生订阅能力：保留当前“空值回退购买前分组”的兼容行为，新增“显式降级分组优先”的业务语义，并让后台套餐编辑页可以配置该字段。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 订阅套餐模型 | `model/subscription.go` | `SubscriptionPlan` 增加 `DowngradeGroup`；购买时快照到 `UserSubscription`。 |
| 用户订阅快照 | `model/subscription.go` | `UserSubscription` 增加 `DowngradeGroup`，到期、取消、删除时用于决定分组回退目标。 |
| 到期任务与人工失效 | `model/subscription.go` | `ExpireDueSubscriptions` 与 `downgradeUserGroupForSubscriptionTx` 调整为显式降级优先，空值保持旧逻辑。 |
| 数据库迁移 | `model/main.go` | SQLite 建表/补列增加 `downgrade_group`；GORM AutoMigrate 负责 MySQL/PostgreSQL 常规列补齐，保持三库兼容。 |
| 管理端订阅控制器 | `controller/subscription.go` | 创建/更新套餐时 trim 并校验降级分组存在；更新 map 写入显式空值以支持清空。 |
| 后端回归测试 | `model/subscription_balance_test.go` 等 | 覆盖购买快照、显式降级优先、空值旧行为、存在其它有效升级订阅时不降级。 |
| 默认前端订阅页 | `web/default/src/features/subscriptions/*` | 类型、表单 schema、默认值、payload 和套餐编辑抽屉增加 `Downgrade Group`。 |
| 前端国际化 | `web/default/src/i18n/locales/*.json` | 增加 `Downgrade Group` 与 `Downgrade to pre-purchase group` 六语文案，并运行 i18n 同步。 |

### 风险评估

- 该能力直接影响用户分组，属于权限和计费权益敏感逻辑；如果到期降级判断错误，可能造成用户权益提前丢失或过期权益残留。
- 空值兼容必须严格保持：`downgrade_group` 为空时，历史套餐和历史订阅仍按当前项目的“回退购买前分组”行为运行。
- 显式降级分组必须在创建和更新时校验存在，避免订阅到期后把用户写入不存在的分组。
- 用户可能同时拥有多个订阅；如果仍有其它有效且会升级分组的订阅，过期其中一个订阅时不能覆盖当前有效权益。
- `UserSubscription.AllowWalletOverflow` 在 NexusTok 中是指针，用于保留显式 `false`，与 new-api-main 的 bool 实现不同；本轮只迁移 `downgrade_group` 业务语义，不改变钱包溢出快照不变量。
- SQLite 不支持复杂列变更；本轮只追加 varchar 列，使用既有 `ensureSubscriptionPlanTableSQLite` 补列模式和 GORM AutoMigrate，避免数据库专用 DDL。

### 方案评审

采用“小字段、强兼容、显式优先”的方案：

1. `SubscriptionPlan` 增加 `DowngradeGroup string`，含义为套餐到期后的显式目标分组，空值表示沿用旧行为。
2. `UserSubscription` 增加 `DowngradeGroup string`，购买、余额购买、管理员绑定订阅时均从套餐快照，避免管理员后续改套餐影响已购订阅。
3. `downgradeUserGroupForSubscriptionTx` 先读取订阅快照中的 `DowngradeGroup` 和 `UpgradeGroup`：二者都为空时直接返回；存在其它有效升级订阅时不降级；显式降级非空时直接作为目标，否则才检查当前分组是否仍等于升级分组并回退到 `PrevUserGroup`。
4. `ExpireDueSubscriptions` 查询最近到期的分组迁移订阅时纳入 `(downgrade_group <> '' OR upgrade_group <> '')`；显式降级优先，空值继续回退购买前分组。
5. 控制器新增统一分组校验 helper，创建和更新套餐同时校验 `upgrade_group` 与 `downgrade_group`，并允许空字符串表示“不配置”。
6. 默认前端订阅套餐表单在 `Upgrade Group` 旁边增加 `Downgrade Group` Select；选项中的 `__none__` 展示为 `Downgrade to pre-purchase group`，提交时转换为空字符串。
7. 本轮不改变用户侧购买弹窗的权益展示口径，避免把“到期降级”误读成购买后立即生效的正向权益；管理员编辑页负责配置该策略。

### 验收方式

1. `go test ./model -run 'TestSubscription|TestPurchaseSubscriptionWithBalance|TestUserActiveSubscriptionsAllowWalletOverflow'`。
2. `go test ./controller -run 'TestSubscriptionPlanGroupValidation'` 或同等覆盖创建/更新分组校验的控制器单测。
3. `cd web/default && bun run i18n:sync`。
4. `cd web/default && bun run typecheck`。
5. `cd web/default && bun run build`。
6. `git diff --check`。
7. 修改后访问 `http://192.168.0.202:3003/`，并打开订阅管理页确认套餐编辑抽屉出现 `Downgrade Group`；如果页面未更新，先重启容器再验证。当前会话若仍无 MCP 浏览器工具，则使用 Chrome headless/CDP 或 `curl --noproxy '*'` 做替代验证，并在结果中说明限制。

### 实施结果

已完成订阅到期显式降级分组原生化。`SubscriptionPlan` 和 `UserSubscription` 均新增 `downgrade_group` 字段：套餐用于管理员配置策略，用户订阅用于购买时快照，避免后续修改套餐影响已购订阅的到期行为。余额购买、第三方支付完成和管理员新增订阅都会通过 `CreateUserSubscriptionFromPlanTx` 统一快照该字段。

分组回退状态机已调整为“显式降级优先，空值旧行为兼容”。订阅到期任务、管理员作废订阅和管理员删除订阅在处理分组时都会先检查是否还有其它有效升级订阅；若存在则保留当前分组。没有其它有效升级订阅时，非空 `downgrade_group` 会作为目标分组；为空时才继续要求当前分组仍等于 `upgrade_group`，并回退到 `prev_user_group`。因此历史套餐和历史订阅在未配置新字段时仍保持原行为。

管理端创建和更新套餐共用 `normalizeAndValidateSubscriptionPlanGroups`：`upgrade_group` 与 `downgrade_group` 都会 trim，非空值必须存在于分组倍率配置中；更新 map 显式写入 `downgrade_group`，支持管理员清空该配置。SQLite 建表/补列逻辑增加 `downgrade_group`，MySQL/PostgreSQL 继续依赖 GORM AutoMigrate 的常规追加列能力。

默认前端订阅套餐编辑抽屉已在 `Upgrade Group` 旁增加 `Downgrade Group` Select，空选项显示 `Downgrade to pre-purchase group` 并提交为空字符串。默认前端类型、表单 schema、默认值、表单回填、payload 和六语翻译均已补齐。classic 前端也补齐了同字段的表单回填、payload 和列表/详情展示，避免管理员从旧版前端编辑套餐时误清空新策略。

### 验证记录

1. `go test ./controller -run 'TestSubscriptionPlanGroupValidation'` 通过，覆盖已知分组 trim 放行、空分组兼容、未知升级分组拒绝和未知降级分组拒绝。
2. `go test ./model -run 'Test(CreateUserSubscriptionSnapshotsDowngradeGroup|ExpireDueSubscriptions|DowngradeUserGroupForSubscriptionTxSupportsExplicitDowngradeOnly|PurchaseSubscriptionWithBalance|UserActiveSubscriptionsAllowWalletOverflow)'` 通过，覆盖购买快照、显式降级优先、空值回退购买前分组、其它有效升级订阅保护、仅配置显式降级目标和钱包溢出快照旧逻辑。
3. `go test ./model ./controller` 通过；本轮顺手修正 model 包测试夹具，在 `TestMain` 设置 SQLite 标记后调用 `initCol()`，避免未走完整 `InitDB` 时 `group/key` 保留字列名为空。
4. `cd web/default && bun run i18n:sync` 通过；默认前端 i18n 报告显示六语 `missingCount=0`、`extrasCount=0`。
5. `cd web/default && bun run typecheck` 通过。
6. `cd web/default && bun run build` 通过。
7. `cd web/classic && bun run build` 通过；仅出现既有 Browserslist 数据过期、`lottie-web` eval 和 chunk size 警告。
8. `git diff --check` 通过。
9. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200；`curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回 `success=true`。
10. 当前会话未暴露浏览器 MCP 工具，已用 Chrome headless/CDP 替代：登录账号 `c1cada` 成功，写入前端 `uid/user` 本地状态后打开 `http://192.168.0.202:3003/subscriptions`，页面显示 `Subscription Management`、`Create Plan` 和空套餐列表；点击 `Create Plan` 后抽屉真实渲染 `Downgrade Group` 与 `Downgrade to pre-purchase group`。
11. 使用登录 cookie 与 `NexusTok-User: 1` 调用运行态 3003：`PUT /api/subscription/admin/plans/999999` 传入 `downgrade_group="__missing_group__"` 返回 `{"message":"降级分组不存在","success":false}`，确认后端热更新已生效且请求在写库前被校验拦截。

## 本轮实施评审：渠道模型搜索追加弹层关闭与编辑页导航对齐

### 需求分析

用户继续反馈“搜索添加时不正确”，并要求参考 `/opt/project/new-api-main` 最新版编辑渠道页面继续对齐。重新访问 3003 运行态复现后确认：后端数据和前端追加结果本身是正确的，`/api/models/search?keyword=gpt-5.6&p=1&page_size=50` 返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个 OpenAI 同步模型；渠道 `11111` 当前已有 `gpt-5.6-sol`，所以批量追加按钮正确显示 `Add 2 new model(s)`，点击后表单草稿也确实增加 `terra/luna`。

真正的问题是追加成功后的弹层状态：`handleAddModelSearchMatches` 会清空搜索词，但通用 `MultiSelect` 内部的 Combobox 仍保持打开。搜索词清空后，候选过滤回到“显示完整模型库”，页面立即铺出大量模型名称，视觉上像搜索添加乱跳或添加到了错误列表。这个问题不属于模型同步缺失，也不应通过后端补数据解决；应该把搜索补齐动作完成后的弹层关闭纳入渠道编辑页的原生交互。

同时对照 new-api 最新编辑渠道页，当前 NexusTok 已经具备大抽屉、左侧状态导航、Basic/Credentials/Models/Advanced 分区和高级折叠结构；本轮只补齐导航 sticky 层级，避免右侧长内容滚动或弹层交互时导航被局部内容覆盖，不覆盖 NexusTok 已有的账号池、Codex OAuth、字段级权限和模型搜索补齐能力。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 通用 MultiSelect | `web/default/src/components/multi-select.tsx` | 新增可选 `open` 与 `onOpenChange` 受控 props；未传入时继续使用内部状态，保持其它调用方行为不变。 |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 模型选择器接管 Combobox open 状态；批量追加搜索缺失模型后关闭弹层并清空搜索词，避免完整模型库突然展开。 |
| 渠道编辑导航 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 左侧 `ChannelEditorNav` 补齐 `lg:z-20` sticky 层级，对齐 new-api 编辑页导航稳定性。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮复现结论和方案边界，明确问题不是 `gpt-5.6` 模型未同步。 |

### 风险评估

- `MultiSelect` 是通用组件，如果直接改变默认关闭策略，可能影响用户管理、分组选择、其它模型选择器或自定义标签输入；因此本轮只新增可选受控能力，默认仍走原内部状态。
- 批量追加仍只修改表单草稿，不触发 `updateChannel`；运行态验证必须避免点击 `Update Channel`，防止把测试草稿写入渠道配置。
- 搜索补齐按钮继续保留在 Combobox 弹层内部，因为上一轮复测证明放在弹层外会被 Base UI 的 dismiss 交互抢走点击；本轮只在 handler 成功后关闭弹层，不改变按钮所在交互上下文。
- 清空搜索词后关闭弹层可能让管理员需要重新聚焦输入才能继续搜索，但这比展示完整模型库更符合“本次搜索补齐已完成”的预期；逐个点选候选的旧能力仍可用。
- 本轮不修改后端 `/api/models/search`、模型元信息同步、渠道保存 payload、数据库、relay 或计费路径，核心业务风险较低。

### 方案评审

采用“通用组件可选受控 + 渠道页局部接管”的方案：

1. `MultiSelectProps` 增加 `open?: boolean` 与 `onOpenChange?: (open: boolean) => void`。
2. `MultiSelect` 内部保留 `internalOpen`；调用方没有传 `open` 时行为与此前一致，传入后只把状态变更通过 `onOpenChange` 通知外部。
3. 渠道编辑抽屉新增 `modelSelectOpen` 状态，并将其绑定到模型 `MultiSelect`。
4. `handleAddModelSearchMatches` 在成功合并缺失模型后先 `setModelSelectOpen(false)`，再清空 `modelSearchKeyword`，防止清空搜索词后的完整候选列表保持可见。
5. 抽屉关闭时同步清空 `modelSelectOpen`，避免下次打开时复用旧弹层状态。
6. `ChannelEditorNav` 补齐 `lg:z-20`，对齐 new-api 侧栏导航在长抽屉中的层级处理。
7. 验证重点放在 `gpt-5.6` 搜索追加后弹层是否关闭、表单是否变为 5 个模型、运行态渠道是否没有被保存修改。

### 实施结果

已完成渠道模型搜索追加后的弹层关闭修复。`MultiSelect` 现在支持可选受控打开状态：其它调用方不传 `open/onOpenChange` 时继续由组件内部管理，渠道编辑页则显式接管模型选择器弹层。管理员输入 `gpt-5.6` 后仍能看到三个真实候选和 `Add 2 new model(s)`；点击追加后，表单草稿增加 `gpt-5.6-terra` 与 `gpt-5.6-luna`，同时关闭候选弹层，不再因为搜索词清空而展示完整模型库。

编辑渠道页的 new-api 结构对齐保持为“吸收页面信息架构，不覆盖 NexusTok 原生能力”：侧栏导航、分区、固定底部操作和高级折叠结构继续保留；账号池凭据模式、Codex OAuth、权限禁用、敏感字段裁剪、模型映射 guardrail、状态码风险确认和搜索补齐 footer 均未被削弱。本轮仅补齐左侧导航 sticky 层级，减少长抽屉滚动时的遮挡风险。

### 验收方式

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts`。
2. `cd web/default && bun run i18n:sync`。
3. `cd web/default && bun run typecheck`。
4. `cd web/default && bun run build`。
5. `git diff --check`。
6. 访问 `http://192.168.0.202:3003/` 和 `/api/status`，确认热更新服务可访问。
7. 用运行态页面登录 `c1cada`，打开 `/channels` 编辑渠道 `11111`，输入 `gpt-5.6`，确认三条候选、两条可新增，点击 `Add 2 new model(s)` 后表单草稿变为 5 个模型且弹层关闭；不点击 `Update Channel`。

### 验证记录

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts` 通过，12 个用例全部成功。
2. `cd web/default && bun run i18n:sync` 通过；本轮没有新增用户可见文案，六语 `missingCount=0`、`extrasCount=0`。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200；`curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回状态 JSON。
7. `docker logs --tail 120 nexustok-frontend-watch` 显示 `[hot] published default dist`，说明热更新产物已发布到 3003 当前服务。
8. 当前会话未暴露 MCP 浏览器工具，已使用 Chrome headless/CDP 替代真实页面验证。登录账号 `c1cada` 后打开 `http://192.168.0.202:3003/channels/`，渠道列表正常显示 `11111`。
9. Chrome headless/CDP 打开渠道 `11111` 编辑抽屉，模型区初始显示 `Selected 3`，包含 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`。
10. 在模型输入框输入 `gpt-5.6` 后，页面候选包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，未出现 `Add custom model "gpt-5.6"` 或 `Add "gpt-5.6"`。
11. Chrome headless/CDP 真实点击 `Add 2 new model(s)` 后，表单草稿显示 `Selected 5`，chip 列表包含 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`；`[data-slot="combobox-content"][data-open]` 不存在，页面没有铺出完整模型库。
12. 验证期间未点击 `Update Channel`。随后使用登录 cookie 与 `NexusTok-User: 1` 调用 `GET /api/channel/?p=0&page_size=5`，运行态渠道 `11111` 仍为 `models:"gpt-5.4,gpt-5.5,gpt-5.6-sol"`，确认没有保存测试草稿。
13. 同样使用运行态接口调用 `GET /api/models/search?keyword=gpt-5.6&p=1&page_size=50`，返回 `total=3`，items 为 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，再次确认模型同步数据完整，问题已定位在前端弹层状态而非供应商或模型元信息缺失。

## 本轮实施评审：结果媒体代理 SSRF Dial 阶段补强

### 需求分析

持续对照 `/opt/project/new-api-main` 的安全边界实现时，报告中仍保留“用户可控 URL 覆盖缺口审计”作为 SSRF 后续待办。本轮重新搜索 Go 侧 HTTP 外连调用后确认：下载代理、Webhook、Bark、Gotify 已经在 URL 预校验后使用 `GetSSRFProtectedHTTPClient()`，能够在真正 Dial 前重新解析并校验目标 IP，抵御 URL 校验和连接之间的 DNS rebinding。

但仍有两个用户可访问的结果媒体代理只做了请求前 URL 预校验：`controller.VideoProxy` 会代理任务结果视频，`relay.RelayMidjourneyImage` 会代理 Midjourney 任务图片。两者的 URL 来源来自上游任务结果或上游回调，最终会由 NexusTok 服务端代用户发起 GET 请求。如果攻击者能影响结果 URL 的域名解析，在预校验后把域名解析到内网地址，普通 `GetHttpClient()` 或显式代理 client 不会在 Dial 阶段再次按 FetchSetting 校验解析 IP。

本轮目标是把 new-api 的 protected fetch 优势继续原生化到 NexusTok 的结果媒体代理：直连路径使用 `GetSSRFProtectedHTTPClient()`，保留原有 URL 预校验和错误返回；显式渠道代理路径由于连接由代理侧建立，无法在 NexusTok 进程内校验最终目标 IP，因此继续保留请求前一次性 URL 校验，并在代码注释中说明边界。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 视频结果代理 | `controller/video_proxy.go` | 无渠道代理时，`VideoProxy` 的实际 GET client 从普通 relay client 改为用户可控 URL 专用 protected client；Gemini、Vertex、OpenAI/Sora 和历史 `result_url` 分支共享同一保护。 |
| Midjourney 图片代理 | `relay/mjproxy_handler.go` | 无渠道代理时，图片代理实际 GET client 从普通 relay client 改为 protected client；显式渠道代理仍用 `NewProxyHttpClient(proxy)`。 |
| SSRF 覆盖报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮审计结论、例外边界和验证记录，避免后续把 Relay 上游 client 或渠道代理误替换。 |

### 风险评估

- 直连路径的行为会更严格：如果 FetchSetting 开启 SSRF 防护且目标域名在 Dial 阶段解析到私有、保留或黑名单 IP，请求会被拒绝。这是本轮预期安全增强，可能暴露少量历史任务结果 URL 配置不安全的问题。
- Relay/provider 上游请求不能全局替换成 protected client，因为 Ollama、自建网关、私有模型服务和内网代理都可能是合法上游；本轮只处理结果媒体代理，不触碰 `service.GetHttpClient()` 的全局语义。
- 显式渠道代理路径无法在本进程内验证代理最终连接的 IP；如果强行替换成 protected client 也只能校验代理地址，无法校验目标。因此本轮保留请求前 URL 校验，并在注释中固定这个例外。
- 视频代理涉及多 provider 认证头，不能改变 `x-goog-api-key`、Vertex URL 解析、OpenAI/Sora Bearer token 或缓存头；本轮只改 client 选择。
- Midjourney 图片代理只改无代理 client，不改任务查询、Content-Type 复制、响应体转发和失败返回。

### 方案评审

采用“结果媒体直连 protected client + 显式代理例外”的小步方案：

1. `VideoProxy` 默认使用 `service.GetSSRFProtectedHTTPClient()`。
2. 当渠道配置了 `proxy` 时，`VideoProxy` 继续使用 `service.GetHttpClientWithProxy(proxy)`，因为目标连接由代理侧建立；请求前 `ValidateURLWithFetchSetting` 保留。
3. `RelayMidjourneyImage` 记录渠道 `proxy` 字符串；无代理时使用 `service.GetSSRFProtectedHTTPClient()`，有代理时保持 `service.NewProxyHttpClient(proxy)`。
4. 两个调用点继续在 `client.Do` 前执行现有 `common.ValidateURLWithFetchSetting`，让错误尽早以 403 返回；直连实际请求时由 protected client 再做 RoundTrip 和 Dial 阶段校验。
5. 不新增路由、不改数据库、不改 FetchSetting schema、不改变 Worker 下载、Webhook、Bark/Gotify、Relay 上游和账号池 Sidecar 代理边界。

### 验收方式

1. `go test ./service -run 'TestProtectedFetch'`，确认 protected client 的 DNS rebinding、代理和禁用 fallback 契约仍通过。
2. `go test ./controller -run 'TestVideoProxy'`，如当前包内存在相关测试则覆盖视频代理；没有匹配测试时以 package compile 作为最低验证。
3. `go test ./relay -run 'TestRelayMidjourney'`，如当前包内存在相关测试则覆盖 Midjourney 代理；没有匹配测试时以 package compile 作为最低验证。
4. `go test ./controller ./relay ./service`，确认本轮触碰的三个包编译与既有单测通过。
5. `git diff --check`。
6. 访问 `http://192.168.0.202:3003/` 与 `/api/status`，确认热更新服务仍可访问。
7. 当前会话无 MCP 浏览器工具时，使用 `curl --noproxy '*'` 和必要的 Chrome headless/CDP 作为替代，并在最终回复中说明限制。

### 实施结果

已完成结果媒体代理 SSRF Dial 阶段补强。`controller.VideoProxy` 现在默认使用 `service.GetSSRFProtectedHTTPClient()` 发起任务结果视频请求；当渠道显式配置 `proxy` 时，继续使用 `service.GetHttpClientWithProxy(proxy)`，并保留请求前 URL 预校验作为代理路径的可执行边界。这样无代理直连视频代理会在 URL 预校验后，于真正拨号前再次解析和校验目标 IP，降低 DNS rebinding 风险。

`relay.RelayMidjourneyImage` 也完成同类调整：无渠道代理时使用 protected client 代理任务图片；有渠道代理时继续使用 `service.NewProxyHttpClient(proxy)`，因为最终目标连接由代理侧建立。两处均未修改任务查询、provider 认证头、响应体转发、Content-Type、缓存头、Worker 下载、Webhook/Bark/Gotify 或 Relay 全局上游 client。SSRF 覆盖缺口审计结论同步写入实施索引：当前首批用户可控 URL 直连链路已覆盖下载、Webhook、Bark/Gotify、视频结果代理和 Midjourney 图片代理；Relay 上游、渠道显式代理、账号池 Sidecar、models.dev 同步和固定 OAuth/支付服务继续作为明确例外管理。

### 验证记录

1. `go test ./service -run 'TestProtectedFetch'` 通过，确认 protected client 的私网回绑定阻断、混合解析 IP 阻断、允许地址拨号、私网白名单、禁用解析校验和关闭保护 fallback 契约仍有效。
2. `go test ./controller -run 'TestVideoProxy'` 通过，当前 controller 包没有匹配的专项测试，命令完成 package compile。
3. `go test ./relay -run 'TestRelayMidjourney'` 通过，当前 relay 包没有匹配的专项测试，命令完成 package compile。
4. `go test ./controller ./relay ./service` 通过，确认本轮触碰的三个包既有测试和编译均通过。
5. `git diff --check` 通过。
6. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200；`curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回 `success=true` 状态 JSON。
7. 当前会话未暴露 MCP 浏览器工具，本轮安全修复没有新增页面入口；运行态验证将使用 `curl --noproxy '*'` 替代页面连通性确认。

## 本轮实施评审：Uptime Kuma 状态接口 SSRF 防护补强

### 需求分析

继续执行用户可控/配置可控 URL 覆盖缺口审计时，发现 `controller.GetUptimeKumaStatus` 仍使用普通 `http.Client{Timeout: 10 * time.Second}` 拉取外部状态页数据。该接口注册在公开 API 路由 `GET /api/uptime/status`，任何访客都可以触发服务端读取系统设置中的 Uptime Kuma `url` 与 `slug`，并并发请求 `/api/status-page/{slug}` 与 `/api/status-page/heartbeat/{slug}`。

对照 `/opt/project/new-api-main/controller/uptime_kuma.go` 后确认，上游当前实现与 NexusTok 原实现基本一致，同样没有 FetchSetting 预校验和 Dial 阶段 DNS rebinding 防护。NexusTok 已经在下载、Webhook、Bark/Gotify、视频结果代理和 Midjourney 图片代理上形成了原生 protected fetch 能力；因此本轮不是机械复制 new-api-main，而是把已吸收形成的优势继续扩展到公开状态接口，让 Uptime Kuma 出站请求进入同一安全边界。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| Uptime Kuma 控制器 | `controller/uptime_kuma.go` | 每个最终 API URL 发出前执行 `FetchSetting` 预校验；实际请求 client 改为复用 `GetSSRFProtectedHTTPClient()`，获得 Dial 阶段 IP 校验；保留 10 秒单请求超时。 |
| JSON 解码规范 | `controller/uptime_kuma.go` | 移除直接 `encoding/json.NewDecoder` 调用，改用项目统一 `common.DecodeJson`，符合 `AGENTS.md` 的 JSON 封装约定。 |
| 回归测试 | `controller/uptime_kuma_test.go` | 覆盖启用 SSRF 时私网 URL 被请求前拦截、公开 IP 放行、关闭 SSRF 后兼容本地测试服务拉取。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮对比证据、例外边界、风险和验证，后续审计不再把 Uptime Kuma 漏列为普通外连。 |

### 风险评估

- 行为会变严格：如果管理员当前把 Uptime Kuma 配置为内网地址、回环地址或默认 FetchSetting 未允许的端口，公开状态接口会按安全策略拦截并返回空监控组。该变化符合 SSRF 防护预期，但需要管理员在可信部署中显式配置 `allow_private_ip`、IP 白名单或允许端口。
- `fetchGroupData` 目前对任一请求失败采用静默降级并返回空 `monitors`，本轮保持该 API 形态，避免公开首页/仪表盘因为某个监控源不可达而变成错误响应；后续可以单独评审是否把安全拦截原因暴露给 Root 诊断页。
- Uptime Kuma 与 Relay 上游不同：它不是模型服务上游，不需要合法访问内网 Ollama/自建模型网关，因此可以纳入用户可触发 URL 的 protected fetch 范围。
- `GetSSRFProtectedHTTPClient()` 的全局 client 可能带有不同的 `RelayTimeout`，本轮通过复制 client 并覆盖 `Timeout=10s` 保持原有单请求超时，不改变整体 30 秒并发聚合超时。
- 不修改 Uptime Kuma 配置结构、前端设置页、数据库、仪表盘组件和返回 JSON schema，降低页面兼容风险。

### 方案评审

采用“最终 URL 预校验 + protected client 实际请求 + 保持接口降级语义”的方案：

1. 新增 `validateUptimeKumaFetchURL`，从 `system_setting.GetFetchSetting()` 读取当前策略，并调用 `common.ValidateURLWithFetchSetting` 校验每个拼接后的最终 Uptime Kuma API URL。
2. 新增 `newUptimeKumaHTTPClient`，复用 `service.GetSSRFProtectedHTTPClient()` 的 Transport 和 redirect 校验能力，复制 client 后覆盖 `Timeout` 为原来的 10 秒。
3. `getAndDecode` 在 `http.NewRequestWithContext` 前先校验 URL；通过后才发出请求，响应解码改为 `common.DecodeJson`。
4. `GetUptimeKumaStatus` 使用专用 protected client，并保留 `requestTimeout=30s`、并发拉取多个 group、失败返回空 group 的旧行为。
5. 单测不依赖真实外网 DNS：用 IP 字面量验证私网/公网校验，用 `httptest.NewServer` 验证私网目标在 handler 被调用前被拦截，以及关闭 SSRF 时旧兼容行为仍可用。

### 验收方式

1. `go test ./controller -run 'UptimeKuma|Uptime'`。
2. `go test ./controller`。
3. `git diff --check`。
4. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/`。
5. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status`。
6. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/uptime/status`。
7. 当前会话若仍无 MCP 浏览器工具，则使用 `curl` 替代公开页面/接口验证，并在最终回复说明限制。

### 实施结果

已完成 Uptime Kuma 状态接口 SSRF 防护补强。`/api/uptime/status` 在拉取每个 Uptime Kuma group 时，会对最终的状态页 API 和 heartbeat API URL 执行 FetchSetting 预校验；无论 URL 来自哪个配置项，只要 SSRF 防护开启，就会统一执行协议、域名/IP、私网地址和端口策略。

实际请求 client 已从裸 `http.Client` 切换为复用 `service.GetSSRFProtectedHTTPClient()` 的 Uptime Kuma 专用 client，保留原来的 10 秒单请求超时。这样即使 URL 预校验与真正连接之间发生 DNS rebinding，直连 Dial 阶段仍会重新解析并校验目标 IP。响应 JSON 解码也已从直接 `encoding/json` 调用切换为 `common.DecodeJson`，符合项目 JSON 封装规范。

### 验证记录

1. `go test ./controller -run 'UptimeKuma|Uptime'` 通过，覆盖私网目标拦截、公开 IP 放行、请求前拦截不触达 handler、关闭 SSRF 后兼容本地测试服务。
2. `go test ./controller` 通过，确认控制器包既有测试和本轮新增测试均通过。
3. `go test ./service -run 'TestProtectedFetch'` 通过，确认 protected fetch client 的 DNS rebinding 与禁用 fallback 契约仍保持。
4. `go test ./controller ./service` 通过，确认本轮接入的控制器和复用的 service 包共同编译与测试通过。
5. `git diff --check` 通过。
6. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200。
7. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回 `success=true` 状态 JSON。
8. `curl --noproxy '*' -sS --max-time 20 http://192.168.0.202:3003/api/uptime/status` 返回 `{"data":[],"message":"","success":true}`，确认公开 Uptime Kuma 状态接口在当前配置下仍保持成功响应。
9. 当前会话未暴露 MCP 浏览器工具；本轮后端安全修复没有新增页面入口，已按项目热更新要求使用 `curl --noproxy '*'` 验证 3003 页面与接口。

## 本轮实施评审：自定义 OAuth Discovery SSRF 防护补强

### 需求分析

继续审计配置可控 URL 的服务端出站请求时，发现 `controller.FetchCustomOAuthDiscovery` 仍使用裸 `http.Client{Timeout: 20 * time.Second}` 拉取 OIDC Discovery 文档。该接口位于 `POST /api/custom-oauth-provider/discovery`，路由已经保留 `RootAuth` 并叠加 `system_setting.operate`，不是公开匿名接口；但它读取的是管理员在系统设置页输入的 `well_known_url` 或 `issuer_url`，最终仍由 NexusTok 后端主动访问外部地址。

对照 `/opt/project/new-api-main/controller/custom_oauth.go` 后确认，上游当前实现与 NexusTok 原实现一致：只校验 URL 解析、host 和 http/https scheme，没有接入 FetchSetting，也没有 Dial 阶段 DNS rebinding 防护。NexusTok 已经把 protected fetch 原生化为用户/配置可控 URL 的共享能力，本轮目标是继续把该能力用于 OIDC Discovery，避免登录入口配置工具成为 SSRF 例外。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 自定义 OAuth Discovery | `controller/custom_oauth.go` | 在已有 http/https 基础校验后，对最终 discovery URL 执行 FetchSetting 预校验；实际请求 client 改为 protected client 并保留 20 秒超时。 |
| 自定义 OAuth 管理 | `controller/custom_oauth.go` | provider 创建、更新、删除、绑定/解绑、OAuth 回调和敏感字段脱敏逻辑不变。 |
| 回归测试 | `controller/custom_oauth_test.go` | 覆盖私网 discovery URL 请求前拦截、公开 IP URL 放行、关闭 SSRF 后本地测试 discovery 仍可拉取。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录 Root 操作类出站请求的防护边界，避免后续只关注公开接口而遗漏系统设置工具。 |

### 风险评估

- 行为会变严格：如果管理员使用内网 Keycloak、GitLab、Authentik 或自建 OIDC provider 的 discovery URL，在默认 `allow_private_ip=false` 且端口不在允许列表时会被拒绝。该行为符合 FetchSetting 的安全策略，可信内网部署需要显式配置允许私网、IP 白名单或端口。
- 该接口是 Root 操作，不是公开接口，风险低于 Uptime Kuma；但 Root 页面常用于调试外部登录入口，错误 URL 仍可能造成服务端访问受限网络，纳入统一防护更一致。
- 不修改 provider 保存结构和前端回填语义，避免影响现有自定义 OAuth 配置流程；Discovery 失败仍通过现有 `ApiErrorMsg` 返回 `success=false`。
- protected client 的基础超时可能受 Relay 配置影响，本轮通过复制 client 并覆盖 20 秒 Timeout 保持旧行为。
- OAuth provider 的实际登录/回调链路可能访问管理员配置的 token/userinfo endpoint；这些属于后续独立审计范围，本轮只处理系统设置页的 discovery 拉取入口。

### 方案评审

采用“基础 URL 校验不变 + FetchSetting 预校验 + protected client 实际请求”的方案：

1. 新增 `validateCustomOAuthDiscoveryURL`，从 `system_setting.GetFetchSetting()` 读取策略，调用 `common.ValidateURLWithFetchSetting` 校验最终 URL。
2. 新增 `newCustomOAuthDiscoveryHTTPClient`，复用 `service.GetSSRFProtectedHTTPClient()` 的 Transport 与 redirect 校验能力，复制 client 后把 Timeout 固定为 20 秒。
3. `FetchCustomOAuthDiscovery` 仍先完成空值、issuer 拼接和 http/https scheme 校验；通过后再执行 FetchSetting 安全校验，失败时返回 `Discovery URL 安全校验失败`。
4. 成功路径继续设置 `Accept: application/json`，读取 HTTP 200 响应并用 `common.DecodeJson` 解析，返回 `{ well_known_url, discovery }`，不改变前端消费结构。
5. 单测用 IP 字面量覆盖策略 helper，用 `httptest.NewServer` 验证私网目标不会触达 handler，并验证关闭 SSRF 防护后旧本地 discovery 调试行为仍可用。

### 验收方式

1. `go test ./controller -run 'CustomOAuth|Discovery'`。
2. `go test ./controller`。
3. `go test ./service -run 'TestProtectedFetch'`。
4. `go test ./controller ./service`。
5. `git diff --check`。
6. 访问 `http://192.168.0.202:3003/` 与 `/api/status`，确认热更新服务可访问。
7. 使用登录 cookie 与 `NexusTok-User: 1` 调用 3003 的 `/api/custom-oauth-provider/discovery`，确认私网 discovery URL 在运行态被安全校验拒绝。
8. 当前会话若仍无 MCP 浏览器工具，则使用 `curl --noproxy '*'` 替代页面/接口验证，并在最终回复说明限制。

### 实施结果

已完成自定义 OAuth Discovery SSRF 防护补强。`POST /api/custom-oauth-provider/discovery` 在拼接并解析最终 URL 后，会读取系统 FetchSetting 并执行协议、端口、私网 IP、域名/IP 黑白名单校验；校验失败时请求不会发出，上游 discovery endpoint 不会被触达。

实际请求 client 已从裸 `http.Client` 切换为复用 `service.GetSSRFProtectedHTTPClient()` 的 Discovery 专用 client，并保留 20 秒单请求超时。这样即使 URL 预校验后发生 DNS rebinding，直连 Dial 阶段仍会重新解析目标并按 FetchSetting 校验 IP。provider 的创建/更新/删除、登录回调、绑定解绑、前端回填字段和路由权限分类均保持不变。

### 验证记录

1. `go test ./controller -run 'CustomOAuth|Discovery'` 通过，覆盖私网 URL 拒绝、公开 IP 放行、私网目标请求前拦截和关闭 SSRF 后兼容本地 discovery。
2. `go test ./controller` 通过，确认控制器包既有测试和本轮新增测试均通过。
3. `go test ./service -run 'TestProtectedFetch'` 通过，确认 protected fetch client 的 DNS rebinding 与禁用 fallback 契约仍保持。
4. `go test ./controller ./service` 通过，确认本轮接入的控制器和复用的 service 包共同编译与测试通过。
5. `git diff --check` 通过。
6. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200。
7. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回 `success=true` 状态 JSON。
8. 使用账号 `c1cada` 登录 3003 后，携带 session cookie 与 `NexusTok-User: 1` 调用 `POST /api/custom-oauth-provider/discovery`，请求体 `{"well_known_url":"http://127.0.0.1/.well-known/openid-configuration"}` 返回 `{"message":"Discovery URL 安全校验失败: private IP address not allowed: 127.0.0.1","success":false}`，确认运行态已在请求前按 FetchSetting 拦截私网 discovery URL。
9. 当前会话未暴露 MCP 浏览器工具；本轮后端安全修复没有新增页面入口，已按项目热更新要求使用 `curl --noproxy '*'` 验证 3003 页面与接口。

## 本轮实施评审：OAuth 登录端点 SSRF 防护补强

### 需求分析

前两轮已经补齐系统设置页的 OIDC Discovery 拉取和公开 Uptime Kuma 状态接口，但继续审计登录回调链路时发现：活跃标准 OAuth 路由 `GET /api/oauth/:provider` 会通过 `oauth.Provider` 接口处理 GitHub、Discord、LinuxDo、OIDC 和自定义 OAuth provider。其中 GitHub、Discord、LinuxDo 是固定官方端点，属于固定第三方 API；系统 OIDC 的 `TokenEndpoint`、`UserInfoEndpoint` 以及自定义 OAuth 的 `TokenEndpoint`、`UserInfoEndpoint` 则来自管理员配置。

对照 `/opt/project/new-api-main/oauth/generic.go` 与 `/opt/project/new-api-main/oauth/oidc.go` 后确认，上游和 NexusTok 原实现一样，在 token 与 userinfo 阶段都直接创建 `http.Client` 发起请求，没有 FetchSetting 预校验，也没有 Dial 阶段 DNS rebinding 防护。由于这些请求发生在真实登录/绑定回调中，如果管理员误配置或配置被污染，攻击者可以通过触发登录回调让后端访问回环、内网或受限端口。本轮目标是把 NexusTok 已经形成的 protected fetch 能力继续扩展到管理员配置的 OAuth 登录端点。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| OAuth 出站请求 helper | `oauth/outbound_http.go` | 新增 oauth 包内部 helper，统一读取 FetchSetting 校验配置端点，并返回复制超时后的 protected client。 |
| 自定义 OAuth 登录 | `oauth/generic.go` | `ExchangeToken` 和 `GetUserInfo` 在请求前校验 token/userinfo endpoint，实际请求改用 protected client；认证方式、响应格式 fallback、访问策略评估不变。 |
| 系统 OIDC 登录 | `oauth/oidc.go` | `ExchangeToken` 和 `GetUserInfo` 在请求前校验 token/userinfo endpoint，实际请求改用 protected client；超时仍为 5 秒，响应字段和用户映射不变。 |
| JSON 解码规范 | `oauth/oidc.go` | 移除直接 `encoding/json.NewDecoder`，改为 `common.DecodeJson`，符合项目 JSON 封装约定。 |
| 回归测试 | `oauth/outbound_http_test.go` | 覆盖 helper、Generic token/userinfo、OIDC token/userinfo 的私网请求前拦截，以及关闭 SSRF 后本地 token endpoint 兼容。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录登录链路补强和边界：固定 provider 不动，旧 `controller/oidc.go` 暂列非活跃历史路径。 |

### 风险评估

- 行为会变严格：如果系统 OIDC 或自定义 OAuth provider 部署在内网，默认 `allow_private_ip=false` 会让登录回调失败。该行为符合 FetchSetting 的安全策略，可信内网 SSO 需要管理员显式允许私网地址、IP 白名单或对应端口。
- 登录链路是高频入口，必须保持错误形态兼容。本轮把安全校验失败包装为现有 `MsgOAuthConnectFailed`，前端仍按 OAuth 网络连接失败路径展示，不引入新响应 schema。
- Generic provider 支持 token JSON 和 URL-encoded fallback，也支持 Basic Auth 和访问策略；本轮只改请求发出前的安全边界和 HTTP client，不改认证参数、字段映射和策略判断。
- OIDC provider 原来直接使用 `encoding/json.NewDecoder`，本轮改为 `common.DecodeJson`，语义等价但符合项目约定。
- `controller/oidc.go` 中存在旧 OIDC 控制器和直接 HTTP 请求，但当前路由只注册统一 `controller.HandleOAuth` 的 `/api/oauth/:provider`，没有直接引用 `OidcAuth/OidcBind`；本轮不扩大到历史路径，避免改动未路由代码造成额外回归面。后续可以单独删除或迁移旧控制器。

### 方案评审

采用“活跃 provider 接口小范围接入 + 固定 provider 保持原状”的方案：

1. 在 `oauth/outbound_http.go` 新增 `validateConfiguredOAuthEndpointURL`，读取当前 FetchSetting 并调用 `common.ValidateURLWithFetchSetting`。
2. 同文件新增 `newConfiguredOAuthHTTPClient(timeout)`，复用 `service.GetSSRFProtectedHTTPClient()` 的 Transport 和 redirect 校验能力，复制 client 后按调用方传入的 timeout 覆盖超时。
3. `GenericOAuthProvider.ExchangeToken` 在构造请求前校验 `p.config.TokenEndpoint`；`GetUserInfo` 在构造请求前校验 `p.config.UserInfoEndpoint`；失败时写日志并返回现有 OAuth connect failed 错误。
4. `OIDCProvider.ExchangeToken` 与 `GetUserInfo` 同样校验系统设置中的 token/userinfo endpoint，并保持原来的 5 秒超时。
5. 不修改 GitHub、Discord、LinuxDo 等固定 endpoint provider，不修改微信/Telegram 等非标准登录路径，不修改 OAuth 路由、session、state 校验、用户创建/绑定事务。
6. 单测用 `httptest.NewServer` 锁定“请求前拦截”而不仅是 helper 结果：私网 endpoint 被拒绝时 handler 不应被调用；关闭 SSRF 后 Generic token endpoint 仍可请求本地测试服务。

### 验收方式

1. `go test ./oauth -run 'ConfiguredOAuth|GenericOAuth|OIDC'`。
2. `go test ./oauth`。
3. `go test ./service -run 'TestProtectedFetch'`。
4. `go test ./controller ./oauth ./service`。
5. `git diff --check`。
6. 访问 `http://192.168.0.202:3003/` 与 `/api/status`，确认热更新服务可访问。
7. 当前 3003 的 `oidc_enabled=false`，无法在不改全局登录配置的情况下真实触发 OIDC token/userinfo 拉取；运行态验证以服务可访问和 `/api/oauth/oidc` 路由前置状态校验为主，端点拦截由单元测试覆盖。
8. 当前会话若仍无 MCP 浏览器工具，则使用 `curl --noproxy '*'` 替代页面/接口验证，并在最终回复说明限制。

### 实施结果

已完成活跃 OAuth 登录端点 SSRF 防护补强。系统 OIDC 和自定义 OAuth provider 的 token/userinfo 请求现在都会在构造 HTTP 请求前执行 FetchSetting 校验；如果目标是回环、私网、受限端口或命中黑名单，请求不会发出，错误会通过既有 OAuth 连接失败路径返回。

实际请求 client 已切换为复用 `service.GetSSRFProtectedHTTPClient()` 的 OAuth 专用 client，并保留各 provider 原有超时：系统 OIDC 仍为 5 秒，自定义 OAuth 仍为 20 秒。这样即使 URL 预校验后发生 DNS rebinding，直连 Dial 阶段仍会重新解析目标并按 FetchSetting 校验 IP。固定官方 OAuth provider、微信/Telegram、OAuth state/session、用户创建/绑定事务、自定义 OAuth 访问策略和前端回调结构均未改变。

### 验证记录

1. `go test ./oauth -run 'ConfiguredOAuth|GenericOAuth|OIDC'` 通过，覆盖 helper、Generic token/userinfo、OIDC token/userinfo 的私网请求前拦截，以及关闭 SSRF 后 Generic token endpoint 的本地兼容行为。
2. `go test ./oauth` 通过，确认 oauth 包既有逻辑和本轮新增测试均通过。
3. `go test ./service -run 'TestProtectedFetch'` 通过，确认 protected fetch client 的 DNS rebinding 与禁用 fallback 契约仍保持。
4. `go test ./controller ./oauth ./service` 通过，确认登录控制器、OAuth provider 和复用的 service 包共同编译与测试通过。
5. `git diff --check` 通过。
6. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200。
7. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回 `success=true` 状态 JSON，且当前运行态 `oidc_enabled=false`。
8. `curl --noproxy '*' -sS --max-time 15 'http://192.168.0.202:3003/api/oauth/oidc?state=invalid&code=test'` 返回 `{"message":"State parameter is empty or mismatched","success":false}`，确认 OAuth 路由仍保持前置 state 校验；当前未启用 OIDC，未修改全局登录配置去触发真实 token/userinfo 拉取。
9. 当前会话未暴露 MCP 浏览器工具；本轮后端登录链路安全修复没有新增页面入口，已按项目热更新要求使用 `curl --noproxy '*'` 验证 3003 页面与接口，端点 SSRF 拦截由 oauth 包单元测试覆盖。

## 本轮实施评审：旧 OIDC 控制器安全基线补齐

### 需求分析

上一轮已经补齐活跃 `/api/oauth/:provider` 登录链路中的系统 OIDC 和自定义 OAuth provider，但继续复核后发现 `controller/oidc.go` 仍保留一套旧的 `OidcAuth/OidcBind/getOidcUserInfoByCode` 实现。当前后端路由只注册统一 `controller.HandleOAuth`，没有直接注册 `OidcAuth/OidcBind`；不过该文件仍在 controller 包编译范围内，并且仍使用裸 `http.Client` 访问系统设置里的 OIDC token/userinfo endpoint，还直接调用 `encoding/json.NewDecoder` 解码。

本轮目标不是恢复旧路由，也不是改变当前登录入口，而是把这条历史路径拉回同一安全基线：如果未来误接回旧控制器，不能重新带出配置端点 SSRF 和 JSON 封装违规；同时报告中明确它是未路由历史实现，后续可单独评审删除或迁移。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 旧 OIDC 控制器 | `controller/oidc.go` | `getOidcUserInfoByCode` 在 token/userinfo 请求前执行 FetchSetting 预校验，实际请求复用 protected client 并保持 5 秒超时。 |
| JSON 解码规范 | `controller/oidc.go` | 移除直接 `encoding/json.NewDecoder`，改为 `common.DecodeJson`。 |
| 回归测试 | `controller/oidc_legacy_test.go` | 覆盖旧控制器 helper、token endpoint 私网请求前拦截、userinfo endpoint 私网请求前拦截，以及关闭 SSRF 后本地 token/userinfo 兼容。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 明确旧控制器当前未路由，属于防御性安全基线补齐。 |

### 风险评估

- 当前后端路由没有直接使用 `OidcAuth/OidcBind`，因此运行态行为风险低；改动主要影响编译范围内的历史函数和未来可能恢复的路径。
- 如果未来恢复旧路径，内网 OIDC endpoint 会按 FetchSetting 策略被拦截，需要管理员显式放行可信内网 SSO；这与活跃 `oauth/oidc.go` 行为保持一致。
- 保持旧函数签名、旧用户错误文案和旧 5 秒超时，避免未来仍有内部调用时出现响应结构变化。
- 不删除旧控制器，避免在本安全切片里扩大到路由/前端清理；后续可以单独做 dead code 清理评审。

### 方案评审

采用“最小防御性补齐”的方案：

1. 在 `controller/oidc.go` 内新增 `validateLegacyOIDCEndpointURL`，复用 `common.ValidateURLWithFetchSetting`。
2. 新增 `newLegacyOIDCHTTPClient`，复用 `service.GetSSRFProtectedHTTPClient()` 的 Transport 和 redirect 校验能力，复制 client 后覆盖为旧路径原有 5 秒超时。
3. `getOidcUserInfoByCode` 在 token 请求和 userinfo 请求前分别校验 endpoint；失败时写系统日志并沿用旧的“无法连接至 OIDC 服务器”错误文案。
4. JSON 解码改为 `common.DecodeJson`，不改变 `OidcResponse`、`OidcUser` 和后续用户创建/绑定逻辑。
5. 新增测试直接调用未导出的 `getOidcUserInfoByCode`，确认安全拦截发生在请求发出前。

### 验收方式

1. `go test ./controller -run 'LegacyOIDC|Oidc'`。
2. `go test ./controller`。
3. `go test ./service -run 'TestProtectedFetch'`。
4. `go test ./controller ./service`。
5. `git diff --check`。
6. 访问 `http://192.168.0.202:3003/` 与 `/api/status`，确认热更新服务可访问。
7. 当前旧 OIDC 控制器未路由，不做运行态旧路径调用；以单元测试覆盖其安全边界。

### 实施结果

已完成旧 OIDC 控制器安全基线补齐。`getOidcUserInfoByCode` 在访问 token endpoint 和 userinfo endpoint 前都会执行 FetchSetting 校验，实际请求使用 protected client，保留 5 秒超时。JSON 解码也已切换为 `common.DecodeJson`。当前活跃 `/api/oauth/:provider` 路径不受本轮影响；旧控制器仍未被路由直接注册。

### 验证记录

1. `go test ./controller -run 'LegacyOIDC|Oidc'` 通过，覆盖旧 OIDC 私网 token/userinfo endpoint 请求前拦截和关闭 SSRF 后本地兼容行为。
2. `go test ./controller` 通过，确认 controller 包既有测试和本轮新增测试均通过。
3. `go test ./service -run 'TestProtectedFetch'` 通过，确认 protected fetch client 的 DNS rebinding 与禁用 fallback 契约仍保持。
4. `go test ./controller ./service` 通过，确认旧控制器和复用的 service 包共同编译与测试通过。
5. `git diff --check` 通过。
6. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200。
7. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回 `success=true` 状态 JSON，且当前运行态 `oidc_enabled=false`。
8. `curl --noproxy '*' -sS --max-time 15 'http://192.168.0.202:3003/api/oauth/oidc?state=invalid&code=test'` 返回 `{"message":"State parameter is empty or mismatched","success":false}`，确认当前活跃 OAuth 路由仍保持前置 state 校验。
9. 当前会话未暴露 MCP 浏览器工具；本轮旧控制器未路由，已按项目热更新要求使用 `curl --noproxy '*'` 验证 3003 页面与接口，旧路径安全边界由 controller 包单元测试覆盖。

## 本轮实施评审：渠道编辑页搜索添加与 new-api 编辑页对齐复核

### 需求分析

用户反馈“搜索添加时不正确”，并要求把当前渠道编辑页继续向 `/opt/project/new-api-main` 最新版编辑渠道页面对齐。结合前几轮关于 `gpt-5.6` 三个 OpenAI 模型只追加不完整的上下文，本轮需要同时确认两件事：

1. 渠道模型搜索输入 `gpt-5.6` 时，前端必须基于 `/api/models/search` 返回的真实 `model_name` 追加模型，而不是把 `gpt-5.6` 前缀当成自定义模型，也不能因为当前渠道已有 `gpt-5.6-sol` 就漏掉 `gpt-5.6-terra` 和 `gpt-5.6-luna`。
2. 当前编辑渠道抽屉需要保持 new-api 最新页面的分段编辑体验，包括左侧导航、Basic Information、Credentials、Models & Groups、Advanced Settings 等结构，同时保留 NexusTok 已原生化的账号池、Codex、权限分级、模型映射防护和上游模型同步等能力。

复核代码和运行态后确认，直接输入 `gpt-5.6` 的主路径已经可以显示 `Add 2 new model(s)` 并追加 `terra/luna`；不过真实页面交互暴露了一个边界：当模型搜索框为空时按 Backspace/Delete，Base UI Combobox 会把按键解释为删除最后一个已选芯片，可能让管理员在清理搜索词或重复搜索时误删已有模型。独立子 agent 还指出搜索追加只使用第一页 `page_size=50` 结果，若未来某个模型系列超过第一页容量，仍可能出现“搜索结果没有加全”。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 通用多选组件 | `web/default/src/components/multi-select.tsx` | 新增可选的空搜索删除键保护，默认关闭，仅由渠道模型字段启用；不会改变其它 MultiSelect 的键盘删除行为。 |
| 多选组件测试 | `web/default/src/components/multi-select.test.ts` | 覆盖 Backspace/Delete 空搜索保护、存在搜索词时仍允许删除输入、默认关闭保护的兼容行为。 |
| 渠道模型搜索 helper | `web/default/src/features/channels/lib/model-search.ts` | 抽出模型搜索候选提取函数，只保留 `model_name` 本身包含关键词的真实模型，并按大小写不敏感去重。 |
| 渠道模型搜索测试 | `web/default/src/features/channels/lib/model-search.test.ts` | 覆盖 description/tags 命中但 `model_name` 不匹配时不进入候选，以及 `gpt-5.6` 缺失项计算。 |
| 编辑渠道抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 启用模型字段空搜索删除保护；普通下拉搜索继续轻量拉第一页；点击“添加搜索结果”时再按 `page_size=100` 拉全所有搜索页后追加缺失模型。 |
| 运行态验证 | `http://192.168.0.202:3003/channels` | 登录后打开渠道 `11111` 编辑抽屉，验证 new-api 分段结构、空 Backspace 不误删、`gpt-5.6` 追加后 Selected 5。 |

### 风险评估

- 空搜索 Backspace/Delete 保护只在渠道模型选择器启用。风险是该字段不再支持通过空输入框键盘删除最后一个模型，但管理员仍可通过芯片移除按钮、候选反选、Clear All 等明确操作删除模型；这比隐式按键误删生产渠道模型更可控。
- 点击追加时会额外请求 `/api/models/search?page_size=100`，并在 `total` 未拉完时继续翻页。该逻辑只在管理员显式点击追加按钮时发生，普通输入仍只拉第一页用于下拉展示，避免每次键入都触发多页请求。
- `/api/models/search` 会按 `model_name OR description OR tags` 搜索。前端继续只将 `model_name` 自身包含关键词的结果加入渠道，避免标签命中导致无关模型被追加。
- 追加逻辑只修改前端表单草稿，不会自动保存渠道；管理员仍需要点击 Update Channel 才会写入后端。
- 本轮未引入新前端文案，未改变后端接口和数据库结构，不涉及 JSON/GORM/计费表达式规则。

### 方案评审

采用“默认兼容 + 渠道模型字段定向防护 + 点击时拉全页”的方案：

1. `MultiSelect` 新增 `preserveSelectedOnEmptyRemovalKey`，默认 `false`，用纯函数 `shouldPreventEmptyInputChipRemoval` 判断是否阻止空输入 Backspace/Delete。
2. 在 `ComboboxChipsInput` 的 `onKeyDownCapture` 阶段拦截空输入删除键。第一次仅在 bubble 阶段拦截时，页面验证发现 Base UI 已先删除芯片，因此必须前移到 capture 阶段。
3. 渠道编辑页的模型 MultiSelect 启用该保护，其它页面不受影响。
4. 抽出 `getModelSearchModelNames`，统一处理搜索结果中的真实模型名提取，避免组件内重复写过滤规则。
5. 普通远程搜索仍使用 `page_size=50`，只服务下拉候选展示；点击 `Add {{count}} new model(s)` 时调用 `fetchAllModelSearchModelNames`，使用后端最大 `page_size=100` 按 `total` 翻页拉完，再基于当前表单最新 `models` 计算缺失项并合并。
6. 追加期间禁用按钮，避免重复点击造成并发追加。

### 实施结果

已完成渠道编辑页模型搜索追加修复和边界补强：

- 输入 `gpt-5.6` 后，下拉候选展示 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，搜索结果提示只显示当前渠道尚未包含的 `terra/luna`，按钮为 `Add 2 new model(s)`。
- 点击追加后，表单从 `Selected 3` 变为 `Selected 5`，保留 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`，新增 `gpt-5.6-terra` 和 `gpt-5.6-luna`。
- 空搜索框按 Backspace 不再删除已有模型，避免“搜索添加”操作链中误删原有模型后变成替换列表。
- 点击追加会发起 `page_size=100` 的补齐请求，未来同系列模型超过第一页候选时也会按分页拉全再追加。
- 编辑渠道抽屉运行态仍呈现 new-api 风格的四段结构：`Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings`；NexusTok 的 Account Pool Group、Codex 渠道、权限控制和模型映射防护继续保留。

### 验证记录

1. `cd web/default && bun test src/features/channels/lib/model-search.test.ts src/components/multi-select.test.ts` 通过，共 18 条测试。
2. `cd web/default && bun run typecheck` 通过。
3. `cd web/default && bun run build` 通过。
4. `git diff --check` 通过。
5. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200。
6. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回 `success=true` 状态 JSON。
7. 使用账号 `c1cada` 登录 3003 后，通过 Chrome headless/CDP 打开 `http://192.168.0.202:3003/channels`，打开渠道 `11111` 编辑抽屉，确认页面结构包含 `Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings`，初始模型为 `Selected 3`。
8. 在模型输入框为空时发送 Backspace，页面仍为 `Selected 3`，且 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol` 均保留。
9. 在模型输入框输入 `gpt-5.6`，页面保持 `Selected 3`，候选和搜索结果包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，按钮显示 `Add 2 new model(s)`。
10. 点击追加按钮后，页面显示 `Selected 5`，并同时包含 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`。本次验证未点击 `Update Channel`，没有保存运行态渠道数据。
11. Chrome headless/CDP 网络记录显示普通下拉搜索请求为 `/api/models/search?keyword=gpt-5.6&p=1&page_size=50`，点击追加时额外请求 `/api/models/search?keyword=gpt-5.6&p=1&page_size=100`，确认全页补齐路径生效。
12. 当前会话未暴露 MCP 浏览器工具；本轮前端交互按项目热更新要求访问了 `http://192.168.0.202:3003/`，并使用 Chrome headless/CDP 完成页面级验证和网络请求核对。

## 本轮实施评审：渠道编辑表单校验原生化

### 需求分析

在完成 `gpt-5.6` 搜索追加修复后，继续对照 `/opt/project/new-api-main/web/default/src/features/channels/lib/channel-form.ts` 与最新版编辑渠道页，发现 new-api 在编辑提交前还做了更完整的 schema 兜底：JSON 配置字段必须是对象、模型映射必须是 string-to-string 对象、状态码映射必须是合法 HTTP 状态码、Codex 手动凭证必须包含 `access_token/account_id`、Vertex AI JSON key 必须是 JSON、部分渠道类型必须填写 `base_url` 或 `other` 附加配置。

NexusTok 当前页面虽然已经在 UI 上用 `*` 标出这些字段，但 schema 层缺少部分兜底，管理员仍可能通过空值或结构错误的 JSON 进入提交链路。本轮目标是把 new-api 的“提交前阻断明显坏配置”能力转成 NexusTok 原生前端校验能力，并保留 NexusTok 自己的账号池组模式、权限体系和既有页面结构。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 渠道表单 schema | `web/default/src/features/channels/lib/channel-form.ts` | 新增 JSON object、模型映射、状态码映射、Codex、Vertex、`base_url`、`other` 等校验 helper 和 `superRefine` 规则。 |
| 渠道错误消息常量 | `web/default/src/features/channels/constants.ts` | 将新增 schema 错误集中到 `ERROR_MESSAGES`，便于 i18n 和后续复用。 |
| 通用表单消息 | `web/default/src/components/ui/form.tsx` | `FormMessage` 对 schema message 执行 `t()`，缺少翻译时仍按原文回退；不改变表单布局和 Base UI 组合方式。 |
| 前端 i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 补齐本轮错误文案六语翻译，并由 `bun run i18n:sync` 规范排序。 |
| 回归测试 | `web/default/src/features/channels/lib/channel-form.test.ts` | 覆盖 JSON object、模型映射、状态码映射、Codex、Vertex、`base_url`、`other`、账号池组豁免与账号池组必选规则。 |

### 风险评估

- 行为会更严格：历史上能提交的错误 JSON、数组型配置、非字符串模型映射、非法状态码映射、缺少区域/API version/Account ID 等配置，现在会在前端阻断。这符合 new-api 编辑页的安全边界，也能减少后端保存后才暴露的渠道不可用问题。
- `base_url` 和 `other` 必填不能机械照搬 new-api。NexusTok 有 `global_account_pool` 模式，账号池组会托管上游凭据和部分配置；本轮对账号池组模式做豁免，避免把 UI 已隐藏的本地凭据字段错误标为必填。
- `FormMessage` 走 `t()` 是通用组件变化，但使用英文原文作为 key 且 i18next 缺失时回退原文，因此对未翻译的既有错误消息保持兼容。
- 本轮只做前端 schema 和文案，不修改后端 DTO、数据库、relay 和保存接口；`disable_task_polling_sleep` 这类需要后端任务轮询服务消费的 new-api 差异不混入本轮，已在后续“异步任务轮询延迟开关原生化”切片补齐。

### 方案评审

采用“schema 原生化 + 账号池语义适配 + 文案集中翻译”的方案：

1. 移植 new-api 的 `parseOptionalJson`、JSON object 判断、模型映射判断、状态码映射判断、Codex 凭证判断和 Vertex JSON key 判断。
2. `setting`、`param_override`、`header_override`、`settings` 只允许空值或 JSON object；避免数组/字符串被误保存为对象配置。
3. `model_mapping` 只允许空值或值为字符串的 JSON object；`status_code_mapping` 只允许 `100-599` 的整数状态码映射。
4. Codex 渠道禁用批量创建，手动 key 非空时必须是含 `access_token` 与 `account_id` 的 JSON object。
5. Vertex AI 服务账号 key 模式要求 JSON object 或 object array；API Key 模式禁用批量创建。
6. 对 new-api 的 `base_url` 与 `other` 必填规则做 NexusTok 化：普通模式下要求对应类型必填；`global_account_pool` 模式下豁免本地配置字段，但仍要求选择账号池组。
7. 新增错误文案进入 `ERROR_MESSAGES` 和六语 locale，`FormMessage` 统一翻译 schema message。

### 实施结果

已完成渠道编辑表单校验原生化：

- JSON 配置字段、模型映射和状态码映射现在能在提交前阻断结构错误。
- Codex 与 Vertex AI 的特殊凭据规则和批量创建限制已与 new-api 对齐。
- Azure、Custom、SunoAPI、VolcEngine 的 `base_url` 必填兜底已补齐；Azure、Xunfei、AI Proxy Library、Cloudflare、Vertex、Coze 的 `other` 附加配置必填兜底已补齐。
- 账号池组模式保留 NexusTok 原生语义：不强制填写被隐藏的本地 `base_url/other`，但仍必须选择有效账号池组。
- 表单错误消息现在可按当前语言显示；新增文案已覆盖 en、zh、fr、ja、ru、vi。
- 子 agent 只读复核指出的两项缺失规则（`base_url`、`other`）已纳入本轮修复；`disable_task_polling_sleep` 已在后续切片补齐为后端真正消费的渠道级能力，而不是纯前端开关。

### 验证记录

1. `cd web/default && bun test src/features/channels/lib/channel-form.test.ts src/features/channels/lib/model-search.test.ts src/components/multi-select.test.ts` 通过，共 31 条测试。
2. `cd web/default && bun run typecheck` 通过。
3. `cd web/default && bun run i18n:sync` 通过，报告显示所有 locale `missingCount=0`、`extrasCount=0`。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200。
7. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回 `success=true` 状态 JSON。
8. 使用账号 `c1cada` 登录 3003 后，通过 Chrome headless/CDP 打开 `http://192.168.0.202:3003/channels`，确认渠道列表显示 `11111`，行操作菜单包含 `Edit`。
9. 打开 `11111` 编辑抽屉，确认页面结构包含 `Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings`，模型字段显示 `Selected 3`，并保留 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`。
10. 在 `Model Mapping` 中切换到 `JSON Mode`，输入 `{"client":123}` 并点击 `Update Channel`，页面显示 `Model mapping must be a JSON object with string values` 与顶部 `Please fix the highlighted fields before saving`，Chrome/CDP 网络记录没有出现 `/api/channel` 的 POST/PUT/PATCH 保存请求，确认前端校验已在提交前阻断。
11. 当前会话未暴露 MCP 浏览器工具；本轮前端交互按项目热更新要求访问了 `http://192.168.0.202:3003/`，并使用 Chrome headless/CDP 完成页面级验证和网络请求核对。

## 本轮实施评审：异步任务轮询延迟开关原生化

### 需求分析

继续对照 `/opt/project/new-api-main` 后发现，new-api 在渠道 `settings` JSON 中提供了 `disable_task_polling_sleep` 开关，并在视频类异步任务轮询中真正消费该字段。该能力的价值不是“禁用任务轮询”，而是允许管理员在可信上游或高吞吐渠道上关闭同一渠道内多个视频任务逐个查询之间的 1 秒保护性等待，从而降低任务积压时的单渠道轮询延迟。

NexusTok 此前已经完成了编辑渠道页面对齐、模型搜索追加修复和表单校验原生化，但 `disable_task_polling_sleep` 仍未形成完整闭环。本轮目标是把它转换成 NexusTok 原生能力：前端可编辑、保存到 `settings`，后端任务轮询服务按渠道读取并生效，默认行为保持保护性等待，避免影响现有核心业务稳定性。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 渠道设置 DTO | `dto/channel_settings.go` | `ChannelOtherSettings` 新增 `DisableTaskPollingSleep`，继续复用 `settings` JSON，不新增数据库列。 |
| 异步任务轮询 | `service/task_polling.go` | `updateVideoTasks` 按渠道读取开关；默认在非最后一个 task 后等待 1 秒，开启后跳过等待；等待期间响应 `ctx.Done()`。 |
| 后端测试 | `service/task_polling_test.go` | 补充默认等待、按渠道关闭等待、等待期间响应 context 取消三类回归测试。 |
| 渠道表单类型和转换 | `web/default/src/features/channels/types.ts`、`web/default/src/features/channels/lib/channel-form.ts` | schema、默认值、编辑回填和更新 payload 均支持 `disable_task_polling_sleep`，保存到 `settings` JSON。 |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 在 `Advanced Settings` → `Channel Extra Settings` 中新增开关，受敏感字段权限控制。 |
| 高级设置错误归类 | `web/default/src/features/channels/lib/channel-form-errors.ts` | 将该字段纳入高级设置字段集合，与 new-api 保持一致。 |
| 前端 i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 补齐开关标题和说明六语翻译。 |
| 前端测试 | `web/default/src/features/channels/lib/channel-form.test.ts` | 覆盖从 `settings` 回填和保存到 `settings` JSON。 |

### 风险评估

- 速率限制风险：默认的 1 秒等待是保护性节流。关闭后同一渠道会连续查询上游，可能更容易触发 429 或供应商限制。因此默认必须为 `false`，并且 UI 放在高级设置/敏感字段权限下。
- 语义误解风险：字段名是负向开关，容易被误读为“禁用任务轮询”。本轮 UI 文案明确为 `Skip async task polling delay`，说明文案强调只是“不在轮询异步任务之间等待 1 秒”。
- 作用范围风险：本轮只影响视频类/default 异步任务路径，不影响 Suno 批量轮询，不改变 Midjourney 轮询，不改变任务状态机和计费结算路径。
- 调度行为风险：new-api 的 `UpdateVideoTasks` 还包含“按渠道并发轮询”的更大调度差异；NexusTok 当前仍按渠道串行处理。本轮只原生化渠道级 sleep 开关，不混入并发调度改造，避免一次性扩大任务调度行为变更。
- 数据库兼容风险低：字段仍存储在已有 `settings` TEXT JSON 中，SQLite、MySQL、PostgreSQL 都不需要迁移。

### 方案评审

采用“小闭环、低侵入”的原生化方案：

1. 后端 `ChannelOtherSettings` 增加 `DisableTaskPollingSleep bool`，复用 `model.Channel.GetOtherSettings()` 的 JSON 解析路径，字段缺失自然回落为 `false`。
2. `updateVideoTasks` 在初始化 adaptor 后读取渠道设置，逐个任务处理时只在“未关闭等待且不是最后一个 task”时等待 1 秒。
3. 将原来的无条件 `time.Sleep(1 * time.Second)` 改为 `select { case <-ctx.Done(): ...; case <-time.After(...) }`，让 SystemTask 租约撤销、超时或取消时能及时退出。
4. 前端把开关作为 `settings` JSON 的一部分处理，覆盖默认值、回填、保存和类型定义；不写入旧 `setting` JSON，避免与渠道通用扩展设置混淆。
5. UI 放入 `Channel Extra Settings`，与 `force_format`、`thinking_to_content`、`pass_through_body_enabled` 等渠道扩展开关同区展示，并受 `canEditSensitiveFields` 控制。
6. 子 agent 只读复核确认：当前最小闭环已与 new-api 的单渠道 sleep 开关语义等价；更完整的“跨渠道并发轮询”已在后续独立切片补齐。

### 实施结果

已完成 `disable_task_polling_sleep` 的 NexusTok 原生化：

- 后端现在会从渠道 `settings` JSON 读取 `disable_task_polling_sleep`，开启后跳过同一渠道视频类异步任务之间的 1 秒保护性等待。
- 默认缺失字段或 `false` 时仍保留等待，只是不再在最后一个 task 后做无意义等待。
- 默认等待期间会响应 `ctx.Done()`，避免任务取消、租约撤销或超时后仍阻塞在 sleep。
- 前端编辑页能从 `settings` 回填该开关，并在保存时写回 `settings.disable_task_polling_sleep`。
- `Advanced Settings` → `Channel Extra Settings` 已新增开关和说明文案，六语翻译已补齐。
- 高级设置错误字段集合已纳入该字段，和 new-api 的表单错误归类保持一致。
- 本轮不修改数据库 schema，不修改任务计费，不修改 Suno/Midjourney 轮询路径。

### 后续差异与建议

new-api 还将视频任务按 channel id 排序后并发执行，每个渠道的 1 秒等待只阻塞本渠道；该调度差异已在后续“异步视频任务跨渠道并发轮询原生化”切片补齐。后续如果继续扩展任务轮询调度，应重点关注真实 adaptor 是否可安全并发、普通渠道错误是否仍只记录日志、以及 SystemTask 成败口径是否需要单独调整。

### 验证记录

1. `go test ./service -run 'TestUpdateVideoTasks.*PollingSleep'` 通过，覆盖默认等待、context 取消、按渠道关闭等待。
2. `go test ./service` 通过。
3. `cd web/default && bun test src/features/channels/lib/channel-form.test.ts src/features/channels/lib/model-search.test.ts src/components/multi-select.test.ts` 通过，共 33 条相关前端测试。
4. `cd web/default && bun run i18n:sync` 通过，报告显示所有 locale `missingCount=0`、`extrasCount=0`。
5. `cd web/default && bun run typecheck` 通过。
6. `cd web/default && bun run build` 通过。
7. `git diff --check` 通过。
8. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200，响应头包含新的 `Cache-Version`。
9. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回 `success=true` 状态 JSON。
10. 使用 Chrome headless/CDP 登录 `c1cada`，打开 `http://192.168.0.202:3003/channels`，确认渠道列表包含 `11111`，并打开其 `Edit Channel` 抽屉。
11. 在运行态编辑抽屉中打开 `Advanced Settings` → `Channel Extra Settings`，确认页面包含 `Skip async task polling delay`、`Do not wait one second between polling async tasks for this channel` 和对应 switch。
12. 页面级验证未点击 `Update Channel`，Chrome/CDP 网络记录没有 `/api/channel/` 保存请求；这样避免改变真实渠道的轮询压力配置，payload 行为由单元测试覆盖。
13. 当前会话未暴露 MCP 浏览器工具；本轮按项目热更新要求访问了 `http://192.168.0.202:3003/`，并使用 Chrome headless/CDP 完成页面级验证和网络请求核对。

## 本轮实施评审：异步视频任务跨渠道并发轮询原生化

### 需求分析

继续对照 `/opt/project/new-api-main/service/task_polling.go` 后确认，new-api 的 `UpdateVideoTasks` 不只是支持 `disable_task_polling_sleep`，还会先收集并排序 channel id，再按渠道并发执行视频任务轮询。其核心收益是让某个渠道内部的 1 秒保护性等待、慢上游请求或临时阻塞只影响该渠道，不拖慢其他渠道的异步视频任务状态更新。

NexusTok 上一轮已完成渠道级关闭轮询等待开关，但 `UpdateVideoTasks` 外层仍按 `taskChannelM` 串行遍历。为了把 new-api 的调度优势转为 NexusTok 原生能力，本轮目标是只迁移“跨渠道并发调度”这一层：保留单渠道内部逐任务串行、保留默认 1 秒保护性等待、保留 `ctx.Done()` 取消、保留普通渠道错误只打日志不向上聚合的现有语义。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 异步任务轮询调度 | `service/task_polling.go` | `UpdateVideoTasks` 从串行 map 遍历改为排序 channel id 后按渠道并发执行；每个 worker 仍调用原 `updateVideoTasks`。 |
| 日志基础设施 | `logger/logger.go` | 并发渠道日志会同时触发 `logHelper`，本轮为 `logCount/setupLogWorking` 增加互斥保护，消除并发日志写入 race。 |
| 后端测试 | `service/task_polling_test.go` | 补充默认 sleep 不阻塞其他渠道、慢渠道不阻塞快渠道、混合 sleep 设置三类测试；内存 SQLite 测试库限制为单连接，避免并发 goroutine 看到不同 `:memory:` 数据库。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新上一节待办状态，并记录本轮评审、风险、结果和验证。 |

### 风险评估

- 并发更新风险：`taskM` 在本调度层中按 task id 只读查表；正常数据不应让同一个 upstream task id 出现在多个渠道。如果存在脏数据导致跨渠道重复，同一个 `*model.Task` 可能被多个 worker 同时处理，这属于任务分组数据不变量问题，本轮不扩大为重写任务归属校验。
- adaptor 线程安全风险：`GetTaskAdaptorFunc(platform)` 每个渠道都会获取并初始化 adaptor。new-api 已采用此模式；NexusTok 保持同样调用方式，不共享本轮新增状态。后续若某个真实 adaptor 返回共享有状态实例，需要单独用 race/集成测试审计。
- 错误语义风险：如果把渠道错误聚合向上返回，单个渠道临时失败会把整个 SystemTask 标记失败，改变现有运维口径。本轮保持 NexusTok/new-api 共同语义：普通渠道错误只记录日志，`ctx.Err()` 才向上返回。
- 取消语义风险：并发后需要等待所有 worker 收束再返回。单渠道内部已有 `ctx.Done()` 检查和可取消 sleep，本轮保留这些能力，并在 `wg.Wait()` 后返回 `ctx.Err()`。
- 日志 race 风险：跨渠道并发会让原来“基本串行”的日志计数状态变成真实并发读写。race 测试已暴露该问题，本轮同步为日志轮转状态加锁。

### 方案评审

采用“只并发渠道层，不改任务更新层”的低侵入方案：

1. `UpdateVideoTasks` 收集 `taskChannelM` 的 channel id 并 `sort.Ints`，让 worker 提交顺序稳定，避免 Go map 遍历顺序影响调度。
2. 对每个非空渠道复制一份 `taskIds` 切片，再通过 `gopool.Go` 提交 worker，使用 `sync.WaitGroup` 等待全部渠道完成。
3. worker 内继续调用现有 `updateVideoTasks(ctx, platform, channelId, taskIds, taskM)`，单渠道内部的 adaptor 初始化、逐任务轮询、sleep、结算和错误日志不重写。
4. `wg.Wait()` 后只检查 `ctx.Err()` 并返回；普通渠道错误仍在 worker 内记录，不聚合。
5. `logger.logHelper` 使用 `logStateMu` 保护日志计数和轮转状态，保留原有 `SetupLogger` 轮转机制。
6. 后端测试覆盖跨渠道并发行为，且通过 race 定向测试确认新并发路径没有数据竞争。

### 实施结果

已完成异步视频任务跨渠道并发轮询原生化：

- `UpdateVideoTasks` 现在会按 channel id 排序并发处理不同渠道；同一渠道内部仍按原顺序逐任务轮询。
- 默认 1 秒保护性等待只阻塞本渠道，不再拖住其他渠道。
- 慢渠道的 `FetchTask` 阻塞时，其他渠道仍能继续完成轮询。
- 混合配置下，关闭 `disable_task_polling_sleep` 的 fast 渠道不会被默认 sleep 渠道拖慢。
- `ctx` 取消后仍会等待 worker 收束，并向上返回 `context.DeadlineExceeded`/`ctx.Err()`，保持 NexusTok SystemTask 失败记录能力。
- 并发日志写入不再对 `logCount/setupLogWorking` 产生数据竞争。

### 验证记录

1. `go test ./service -run 'TestUpdateVideoTasks'` 通过，覆盖单渠道 sleep、context 取消、关闭 sleep、跨渠道默认 sleep、慢渠道阻塞、混合 sleep 设置。
2. `go test -race ./service -run 'TestUpdateVideoTasks'` 通过，确认并发轮询和日志轮转状态无 race。
3. `go test ./service` 通过。
4. `git diff --check` 通过。
5. 本轮为后端调度与日志基础设施改动，未修改前端资源；仍按热更新要求访问 `http://192.168.0.202:3003/` 和 `/api/status` 进行运行态存活确认。

## 本轮实施评审：渠道模型搜索添加纠偏与编辑渠道页原生对齐

### 需求分析

用户再次反馈“搜索添加时不正确”，并指出 `/opt/project/new-api-main` 最新版编辑渠道页面体验更好，应继续对齐。结合前几轮验证，本地模型库已经完整同步 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，当前渠道 `11111` 已包含 `gpt-5.6-sol`，因此批量追加时实际应新增 `terra/luna` 两个缺失模型。

本轮复核确认，搜索数据本身没有缺失，`/api/models/search?keyword=gpt-5.6` 会返回三个 OpenAI 供应商模型；真正的问题在前端交互：模型搜索框同时承担“搜索已有模型”和“创建自定义模型”的心理模型，且搜索命中列表会混入已选模型，管理员点击 `gpt-5.6-sol` 时可能反而触发取消选择，造成“只同步到一个”或“搜索添加不正确”的误判。

new-api 最新编辑渠道页的优势不是某个单独按钮，而是长表单分区更清晰、模型操作入口更收敛、自定义模型输入与已有模型选择分工明确。NexusTok 应吸收这一点，但不能退回 new-api 的简化能力；必须保留账号池凭据模式、Codex OAuth、字段级权限、模型映射 guardrail、远程模型元信息搜索和搜索翻页补齐等原生增强。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 通用多选组件 | `web/default/src/components/multi-select.tsx` | 新增可选 `hideSelectedOptionsWhenSearching`，搜索状态下可隐藏已选项；默认 `false`，其它调用方不受影响。 |
| 渠道模型搜索 helper | `web/default/src/features/channels/lib/model-search.ts` | 新增 `parseModelDraftList()` 和 `buildModelSearchAppendSummary()`；统一自定义模型解析、大小写不敏感去重，以及“命中/可新增/已存在”统计。 |
| 渠道模型搜索测试 | `web/default/src/features/channels/lib/model-search.test.ts` | 增加自定义模型解析、`gpt-5.6` 三模型摘要统计测试，固定“当前已有 sol 时只新增 terra/luna”的语义。 |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 模型搜索框只搜索已有/远程同步模型，不再作为自定义创建入口；新增独立自定义模型输入；搜索弹层展示命中、可新增和已存在数量；快捷操作收敛为主按钮加 `More` 菜单。 |
| 前端 i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 本轮文案已由 `i18n:sync` 校验完整；相关 key 覆盖六语，当前工作树没有新增 locale diff。 |

### 风险评估

- `MultiSelect` 是共享组件，不能全局改变搜索结果过滤规则。本轮新增的隐藏已选项能力默认关闭，只在渠道模型字段显式启用。
- 模型搜索框禁用 `allowCreate` 后，管理员不能再在同一个输入框里创建任意自定义模型。为避免能力回退，本轮提供独立“自定义模型（逗号分隔）”输入，支持英文逗号、中文逗号和换行。
- 搜索统计必须区分“搜索命中总数”和“实际可新增数”。当前渠道已有 `gpt-5.6-sol` 时，按钮只能显示并追加 2 个新模型，不能再把 3 个命中误写成 3 个新增。
- 批量追加仍只写入前端表单草稿，不直接调用 `updateChannel`，管理员必须点击 `Update Channel` 才会保存运行态渠道配置；页面验证期间不得改变真实渠道数据。
- `/api/models/search` 会匹配 `model_name/description/tags`。本轮继续只把 `model_name` 自身包含关键词的结果纳入渠道模型，避免标签命中带入无关模型。
- 点击追加时仍可能遇到搜索结果分页或表单值已被其它交互改变。本轮保留点击时重新拉取全量搜索结果并基于 `form.getValues('models')` 现算缺失项的策略，避免旧闭包、首屏预览或重复点击导致实际写入数量偏差。

### 方案评审

采用“搜索同步与自定义录入拆分 + 可新增数量显性化”的方案：

1. 渠道模型 MultiSelect 改为纯搜索已有模型：`allowCreate={false}`、placeholder 改为 `Search models...`，避免 `gpt-5.6` 这类系列前缀被误当成自定义模型创建。
2. 仅渠道模型字段启用 `hideSelectedOptionsWhenSearching`。搜索 `gpt-5.6` 时，下拉候选只显示还可以新增的 `gpt-5.6-terra` 和 `gpt-5.6-luna`，不再展示已选的 `gpt-5.6-sol`。
3. 搜索弹层顶部继续使用 `Search results`，但统计改为 `{{matched}} matched · {{addable}} new · {{existing}} already selected`，按钮改为 `Add {{count}} new model(s)`，其中 `count` 永远来自实际可新增数量。
4. 点击追加时调用 `fetchAllModelSearchModelNames(debouncedModelSearchKeyword)` 拉取全量搜索结果，再用当前表单最新模型列表计算缺失项，追加成功后清空搜索词并关闭弹层。
5. 新增独立自定义模型输入 `Custom model (comma-separated)`，用 `parseModelDraftList()` 支持逗号、中文逗号和换行分隔，并复用 `getMissingModelSearchMatches()` 去重。
6. 快捷操作向 new-api 编辑页靠拢：保留外露主操作 `Fill Related Models` 与可用时的 `Fetch from Upstream`，把 `Fill All Models`、`Copy All`、`Clear All` 和预设组收进 `More` 下拉菜单。
7. 保留 NexusTok 原生能力边界：账号池、Codex、权限、模型映射防护、搜索翻页补齐、保存前校验和多语言文案都不削弱。

### 实施结果

已完成渠道模型搜索添加纠偏和编辑渠道页原生对齐：

- 输入 `gpt-5.6` 后，弹层顶部显示 `命中 3 个 · 可新增 2 个 · 已存在 1 个`，预览只列出 `gpt-5.6-terra, gpt-5.6-luna`，按钮为 `添加 2 个新模型`。
- 已选模型在搜索状态下隐藏，避免用户点击 `gpt-5.6-sol` 造成误取消；空搜索时仍能正常看到和管理已选模型。
- 点击添加后，表单草稿从 `已选 3 个` 变为 `已选 5 个`，保留 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`，新增 `gpt-5.6-terra` 和 `gpt-5.6-luna`。
- 自定义模型添加从搜索框拆出，支持一次输入多个模型并按大小写不敏感去重；重复模型会提示没有新模型可添加。
- `Fill All Models`、`Copy All`、`Clear All` 和预设组已收敛到 `More` 菜单，编辑渠道页的模型区扫描成本更接近 new-api 最新版，但仍保留 NexusTok 的远程模型搜索、账号池和权限增强。
- 本轮只修改前端表单草稿，不自动保存运行态渠道数据。

### 验证记录

1. `cd web/default && bun test src/features/channels/lib/model-search.test.ts src/components/multi-select.test.ts` 通过，23 个用例全部成功，覆盖自定义模型解析、`gpt-5.6` 三模型摘要、搜索过滤、创建和删除保护。
2. `cd web/default && bun run i18n:sync` 通过；同步报告显示所有语言 `missingCount=0`、`extrasCount=0`，en/zh `untranslatedCount=0`。fr/ja/ru/vi 仍有历史 untranslated 项，不是本轮新增导致。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200，热更新页面可访问。
7. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回 `success=true` 状态 JSON。
8. 使用登录态接口调用 `GET /api/models/search?keyword=gpt-5.6&p=1&page_size=50`，返回 `total=3`，items 为 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，`vendor_id=1`，确认供应商为 OpenAI 且三种模型均已同步。
9. 使用 Chrome headless/CDP 登录 `c1cada`，打开 `http://192.168.0.202:3003/channels`，确认渠道 `11111` 可见，并通过行菜单打开 `Edit Channel` 抽屉。
10. 编辑抽屉初始模型字段显示 `已选 3 个`，包含 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`；页面包含独立字段 `自定义模型（逗号分隔）` 和 `更多` 菜单。
11. 在模型搜索框输入 `gpt-5.6` 后，弹层顶部显示 `搜索结果`、`命中 3 个 · 可新增 2 个 · 已存在 1 个`、预览 `gpt-5.6-terra, gpt-5.6-luna` 和按钮 `添加 2 个新模型`；下方候选只显示 `gpt-5.6-terra`、`gpt-5.6-luna`，不会显示已选的 `gpt-5.6-sol`。
12. 真实点击 `添加 2 个新模型` 后，toast 显示 `已从搜索结果添加 2 个模型`，表单草稿显示 `已选 5 个`，包含 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`。
13. 验证期间未点击 `更新渠道`，因此没有保存测试草稿，不改变运行态渠道数据。
14. 当前会话未暴露 MCP 浏览器工具；本轮按项目热更新要求访问了 `http://192.168.0.202:3003/`，并使用 Chrome headless/CDP 完成真实页面交互和接口验证。

## 本轮实施评审：Authz 角色与策略持久化底座原生化

### 需求分析

继续对照 `/opt/project/new-api-main/service/authz` 后确认，new-api 的细粒度授权不只返回静态 catalog，还把角色模板保存到 `authz_roles`，把角色和用户策略保存到 `casbin_rule`，启动时种子内置 Root/Admin 角色并持久化 Admin 默认策略。NexusTok 当前已经有权限 catalog、路由 `RequirePermission`、用户级 override 和前端权限矩阵，但角色基线仍完全由内存里的 `DefaultRoles` 推导，缺少可审计、可导出、可后续编辑的策略存储。

本轮目标是先把 new-api 的“角色与策略持久化”优势转换成 NexusTok 原生底座：新增跨数据库兼容的 `authz_roles` 与 `casbin_rule` 表，主节点启动时种子内置角色和 Admin 默认策略，权限计算优先读取数据库策略，读取失败或表尚未准备好时回退当前静态基线。完整的角色模板 UI、Casbin 运行时依赖和自定义角色分配暂不混入本轮，避免一次性扩大授权面。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 数据模型 | `model/authz_role.go`、`model/casbin_rule.go` | 新增 `AuthzRole` 与 `CasbinRule`，表名对齐 new-api；字段只使用字符串、整数、布尔和 bigint 时间戳，保持 SQLite/MySQL/PostgreSQL 兼容。 |
| 数据库迁移 | `model/main.go` | `migrateDB` 与 `migrateDBFast` 纳入两张新表；不修改现有用户、渠道、账号池和日志表。 |
| 权限服务 | `service/authz/*` | 新增角色 subject、策略 subject、种子、读取和 fallback；`Roles()`、`CapabilitiesForUser()`、`Can()` 在数据库策略可用时优先使用持久化策略。 |
| 启动流程 | `main.go` | 主库初始化后调用 Authz 持久策略初始化；失败时 fail-fast，避免迁移或种子异常后服务带着不完整策略运行。 |
| 测试 | `service/authz/*_test.go`、必要的 controller 测试 setup | 增加策略种子、角色列表、策略优先级、未知策略过滤、用户 override 叠加和表缺失 fallback 的回归覆盖。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮评审、边界、实施结果和验证。 |

### 风险评估

- 授权放大风险：如果数据库策略被误读为默认允许，会直接扩大 Admin 权限。本轮只接受已注册 catalog 中的 `allow` 策略，未知 resource/action/effect 全部忽略，Root 仍仅通过已知权限短路，未知权限继续 fail-closed。
- 启动兼容风险：老库没有新表时，服务启动迁移会创建表并种子策略；测试或从节点临时未迁移时，权限计算回退静态基线，避免因为表不存在导致所有 Admin 页面瞬间不可用。
- 多节点风险：本轮种子只应在主节点执行，避免从节点并发写策略；从节点读取策略失败时回退静态基线。后续如果开放角色编辑，需要再做策略变更广播或周期 reload。
- 语义迁移风险：当前用户级 override 保存在 `authz_user_overrides`，直接迁到 `casbin_rule` 会影响既有测试和前端接口。本轮保留用户 override 表，只把角色基线策略持久化，后续再评估是否合并用户策略存储。
- 数据库兼容风险：不得使用 PostgreSQL JSONB、MySQL 专用 upsert 或 SQLite 不支持的 ALTER COLUMN；种子使用 GORM 查询后 create/update 的通用路径。

### 方案评审

采用“持久策略优先 + 静态基线兜底”的低侵入方案：

1. 新增 `AuthzRole` 与 `CasbinRule` 模型，表结构对齐 new-api 的 `authz_roles` 和 `casbin_rule`，但时间戳用 NexusTok 统一的 Unix 秒字段和 GORM hook。
2. 新增 `RoleSubject(roleKey)` 和 `UserSubject(userID)`，角色策略使用 `p, role:admin, resource, action, allow` 形式保存；Root 是 superuser，不写全量策略。
3. 主节点启动后调用 `SeedPersistentPolicies()`：先 upsert 内置角色，再重置内置角色策略，最后按 catalog 中 `DefaultRoles` 种子 Admin allow 策略。
4. `Roles()` 返回内置角色描述时，角色 key/name/superuser 仍以代码内置定义为准，避免数据库误配置关闭内置管理角色；授权矩阵优先读取持久化策略，数据库不可用时继续返回当前静态矩阵。
5. `roleGrantsWithOverrides()` 和 `Can()` 对非 Root 角色优先查持久策略；策略表不可用、角色没有持久策略或读取错误时回退 `DefaultRoles`，确保升级期间行为与旧版本一致。
6. 用户级 override 保持当前 `AuthzUserOverride` 语义，仍然优先于角色基线；普通用户即使存在 override 也不能被提升为管理角色。
7. 后续角色编辑 UI、自定义角色、用户角色分配和用户 override 迁入 `casbin_rule` 作为独立切片继续推进。

### 实施结果

已完成 Authz 角色与策略持久化底座原生化：

- 新增 `model.AuthzRole` 和 `model.CasbinRule`，表名分别为 `authz_roles` 与 `casbin_rule`，字段与 new-api/Casbin adapter 兼容，并使用 NexusTok 统一 Unix 秒时间戳 hook。
- `migrateDB()` 和 `migrateDBFast()` 均已纳入两张新表，保持 SQLite、MySQL、PostgreSQL 通用 GORM 迁移路径，不使用数据库专用 upsert 或 JSONB。
- 新增 `RoleSubject(roleKey)` 和 `UserSubject(userID)`，角色策略统一保存为 `p, role:<key>, <resource>, <action>, allow`；Root 作为 superuser 不写全量 allow 策略。
- `SeedPersistentPolicies()` 会 upsert 内置 Root/Admin 角色模板，重置内置角色策略，再按 catalog 中的 Admin `DefaultRoles` 种子默认 allow 策略。
- `InitResources()` 在主节点数据库初始化成功后执行 Authz 持久策略种子；种子失败时 fail-fast，避免服务带着半初始化授权表运行。
- `Roles()`、`CapabilitiesForUser()`、`Can()` 对非 Root 管理角色优先读取 `casbin_rule` 中的可用策略；策略表缺失、读取错误、角色没有任何可用 allow 策略时回退静态 Admin 基线。
- 用户级 override 继续存储在 `authz_user_overrides`，并且仍优先于角色基线；差异计算现在会优先对比持久 Admin 基线，后续角色策略可编辑时，用户级 allow/deny 仍能表达“相对当前角色基线”的覆盖。
- 未知 resource/action、非 allow effect 和脏策略会被忽略；如果只剩未知策略，会被视为策略缺失并回退静态基线，降低升级与误写风险。
- 本轮没有引入 Casbin 运行时依赖，没有开放角色编辑 UI、自定义角色、用户角色分配，也没有把用户 override 迁入 `casbin_rule`；这些仍作为后续独立切片推进。

### 验证记录

1. `go test ./service/authz` 通过，覆盖策略种子、Root/Admin 角色、持久策略优先、用户 override 优先、策略表缺失回退、无有效策略回退和原有 override 语义。
2. `go test ./controller -run 'TestGetSelfReturnsAdminPermissions|TestGetSelfIncludesAuthzUserOverrides|TestGetUserReturnsAdminPermissions|TestUpdateUserAdminPermissions|TestUpdateUserRejectsNonRootAdminPermissions|TestCreateUserAdminPermissions|TestUpdateUserClearsAdminPermissionsWhenTargetIsCommon|TestDemoteClearsAdminPermissions'` 通过，确认用户详情、自身份权限和用户管理接口仍返回正确权限矩阵。
3. `go test ./model` 通过，确认新增模型迁移没有破坏现有模型层回归。
4. `go test ./service/authz ./controller` 通过。
5. `go test ./router -run Authz` 通过。
6. `go test ./...` 通过，确认新增 Authz 持久化底座没有破坏全仓 Go 回归。
7. `git diff --check` 通过。
8. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200，热更新入口可访问。
9. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回 `success=true` 状态 JSON，后端运行态正常。
10. Docker 日志确认热更新后端已 rebuild/restart，`start_time` 已更新；运行态使用 PostgreSQL 数据库 `nexustok`。
11. 在 PostgreSQL 运行态确认 `authz_roles` 已种子 `root`、`admin` 两条内置角色；`casbin_rule` 中 `role:admin` 默认 allow 策略共 21 条。
12. 在运行态确认 Admin 的 `channel` 策略包含 `read`、`write`、`operate` 三条 allow，和静态 Admin 基线保持一致。
13. 当前会话未暴露 MCP 浏览器工具；本轮为后端授权底座改动，已按热更新要求访问 `http://192.168.0.202:3003/`，并使用 curl、Docker 日志和 PostgreSQL 查询完成运行态验证。

## 本轮实施评审：编辑渠道页 new-api 对齐与模型搜索追加二次补强

### 需求分析

用户继续反馈“搜索添加时不正确”，并明确指出 `/opt/project/new-api-main` 最新版编辑渠道页面的体验更好，应把当前编辑渠道页继续向它对齐。结合前几轮 `gpt-5.6` 供应商模型同步上下文，本轮重新验证后确认：模型库和接口数据本身完整，OpenAI 供应商下已有 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个模型；渠道 `11111` 当前只保存了 `gpt-5.6-sol`，因此搜索 `gpt-5.6` 后应只把缺失的 `terra/luna` 两个模型追加到表单草稿。

本轮真正需要补强的是交互边界与页面组织：

1. 搜索追加不能被旧的异步请求污染。管理员切换渠道、关闭抽屉或修改搜索词后，旧请求返回时不能再写入当前表单。
2. 搜索追加不能只依赖第一页预览。当第一页没有可新增模型但后端仍有未扫描结果时，按钮应允许“扫描全部搜索结果”后再判断。
3. 模型字段仍应支持 new-api 习惯里的“快速录入模型”，但必须避免有真实候选时把 `gpt-5.6` 这种系列前缀误创建成自定义模型。
4. 编辑渠道页应吸收 new-api 最新页面“核心配置优先、模型操作靠近字段、高级配置可继续下钻”的优势，同时保留 NexusTok 已经原生化的账号池、Codex、权限、模型映射 guardrail、上游检测和多语言能力。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 模型搜索 helper | `web/default/src/features/channels/lib/model-search.ts` | 新增统一的模型名 trim + 大小写不敏感去重、合并工具，供搜索追加、自定义创建、预设填充和手动选择复用。 |
| 模型搜索测试 | `web/default/src/features/channels/lib/model-search.test.ts` | 补充去重与合并测试，继续固定 `gpt-5.6` 三模型里“已有 sol 时只新增 terra/luna”的语义。 |
| 共享 MultiSelect | `web/default/src/components/multi-select.tsx` | 抽出可测试的候选可见性计算；支持“搜索时隐藏已选项”；允许调用方在有匹配候选时禁止自定义创建。 |
| MultiSelect 测试 | `web/default/src/components/multi-select.test.ts` | 覆盖搜索 `gpt-5.6` 时隐藏已选 `sol`、保留 `terra/luna`，以及空搜索仍显示已选项。 |
| 编辑渠道抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 搜索追加加入请求序列、同步锁和上下文校验；支持未扫描结果提示；模型框改为“无候选时可创建”；抽屉加宽，左侧导航改为顶部横向锚点条；上游检测模型预览改为 Tooltip 完整展示。 |
| 前端 i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 增加 `Scan all search results` 与未扫描结果提示文案，六语完整同步。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案、实施结果和运行态验收。 |

### 风险评估

- 表单污染风险：搜索全量拉取是异步操作，若不校验当前 `open/channelId/keyword`，旧请求可能在用户切换渠道后写入错误表单。本轮用请求序列和上下文快照双重校验，失效请求只结束自身，不再写草稿。
- 重复提交风险：React state 更新存在同 tick 空窗，仅依赖 `isAddingModelSearchMatches` 可能挡不住连续 pointer/click。本轮增加同步 ref 锁，避免重复拉取和重复追加。
- 数据完整性风险：接口搜索可能分页。按钮状态不能只看当前页的可新增数，否则未来大模型系列可能被误判为“没有新结果”。本轮用后端 `total` 与当前已加载真实模型数计算未扫描数量，并允许扫描全量页。
- 自定义模型回归风险：如果完全禁用模型框创建，会让管理员录入冷门模型更绕；如果有真实候选时仍允许创建，又会把 `gpt-5.6` 前缀当成模型。本轮只在无真实候选时展示创建入口，避免两端能力互相伤害。
- 页面布局风险：把侧边导航改成顶部 sticky 横向条可能遮挡长表单或移动端溢出。本轮仅在抽屉内部使用横向滚动条和稳定宽度，保留锚点状态，不改变保存 payload 和后端接口。
- Tooltip 可用性风险：上游检测完整列表可能很长。本轮只把完整列表放到 Tooltip，原地仍显示前 8 个预览，不扩大表单默认占高。

### 方案评审

采用“保留 NexusTok 宽抽屉能力 + 吸收 new-api 编辑页交互”的方案，而不是直接复制 new-api classic 的窄抽屉：

1. 在 `model-search.ts` 统一模型名归一化、去重和合并规则，所有模型列表写入前都按同一套 trim + lower key 处理。
2. 搜索追加按钮点击时重新拉取全部搜索页，再基于表单最新 `models` 计算缺失项；写入前校验请求序列、抽屉打开状态、渠道 ID 和搜索词仍一致。
3. 当前页如果已没有可新增项但后端还有未扫描结果，弹层显示未扫描数量，按钮切换为 `Scan all search results`，让管理员可以触发全量确认。
4. 模型 MultiSelect 开启 `allowCreate`，但设置 `allowCreateWithMatches={false}`：无候选时显示 `Press Enter to use "{{value}}"`，有候选时只做筛选/补齐。
5. 顶部 sticky 锚点条替代左侧 sticky aside，抽屉从 `sm:max-w-5xl` 扩到 `sm:max-w-6xl`，让模型区和高级配置获得更宽的工作面。
6. 上游检测的 `Last detected addable models` 保持预览，同时通过 Tooltip 展示完整检测列表。
7. 全部新增 UI 文案进入六语 locale，并用 `i18n:sync` 校验。

### 实施结果

已完成编辑渠道页 new-api 对齐和模型搜索追加二次补强：

- `fetchAllModelSearchModelNames()` 对空 `items` 做一致处理，翻页终止不再访问可能为空的 `data.items.length`。
- `handleAddModelSearchMatches()` 增加同步 ref 锁、请求序列和 `open/channelId/keyword` 校验，旧请求不会污染当前抽屉；关闭抽屉或切换渠道会立即让 pending 请求失效。
- 模型列表写入统一经过 `dedupeModelNames()` / `mergeModelNames()`，大小写不同或带空白的重复模型不会被写入表单草稿。
- 搜索弹层在后端仍有未扫描结果时提示 `{{count}} more result(s) will be checked when adding`，并允许点击 `Scan all search results` 做全量检查。
- 模型选择框在搜索时隐藏已选项；搜索 `gpt-5.6` 时只显示可新增的 `gpt-5.6-terra`、`gpt-5.6-luna`，不再展示已选 `gpt-5.6-sol`。
- 模型选择框恢复无候选时创建自定义模型的能力；输入 `custom-test-model-xyz` 时显示 `Press Enter to use "custom-test-model-xyz"`，但输入 `gpt-5.6` 这种已有候选前缀时不会误创建。
- 编辑渠道抽屉改为更宽的 `sm:max-w-6xl`，原左侧导航改为顶部横向 sticky 锚点条，保留 Basic Information、Credentials、Models & Groups、Advanced Settings 及高级子项导航。
- 上游检测完整模型列表通过 Tooltip 展示，默认页面只保留短预览，避免高级设置区域被长列表撑开。
- 本轮没有修改后端接口、保存 payload、权限判断、账号池/Codex 逻辑，也没有保存真实渠道配置。

### 验证记录

1. `cd web/default && bun test src/features/channels/lib/model-search.test.ts src/components/multi-select.test.ts` 通过，27 个用例全部成功。
2. `cd web/default && bun run i18n:sync` 通过，locale 同步报告生成成功；本轮新增文案已覆盖 en、zh、fr、ja、ru、vi。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200；页面已可访问，未触发容器重启。
7. 由于当前会话没有暴露 MCP 浏览器工具，使用 Google Chrome headless + CDP 替代真实浏览器验证；验证入口仍为 `http://192.168.0.202:3003/`。
8. 使用账号 `c1cada` 登录后进入 `/channels`，打开渠道 `11111` 的 `Edit Channel` 抽屉，初始模型为 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`，显示 `Selected 3`。
9. 抽屉运行态宽度为 1152px，内部无旧的左侧 `aside` 导航；顶部横向锚点条显示 Basic Information、Credentials、Models & Groups、Advanced Settings 和高级子项。
10. 在模型框搜索 `gpt-5.6` 后，弹层显示 `3 matched · 2 new · 1 already selected`，预览 `gpt-5.6-terra, gpt-5.6-luna`，按钮为 `Add 2 new model(s)`，候选只显示 `terra/luna`。
11. 点击 `Add 2 new model(s)` 后，表单草稿显示 `Selected 5`，包含 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`；验证期间未点击 `Update Channel`。
12. 输入 `custom-test-model-xyz` 后，截图确认弹层显示 `Press Enter to use "custom-test-model-xyz"`，证明无候选时自定义创建入口生效。
13. Chrome/CDP 记录没有运行时异常、控制台 error/warning 或非取消型网络失败。
14. 接口复查 `GET /api/channel/1` 返回 `models="gpt-5.4,gpt-5.5,gpt-5.6-sol"`，确认页面验证未保存表单草稿、未改变运行态渠道数据。
15. 验证截图保存于 `/tmp/nexustok-channel-edit-after.png` 与 `/tmp/nexustok-channel-custom-create.png`。

## 本轮实施评审：账号池 History 移动端日志卡片原生化

### 需求分析

对照 `/opt/project/new-api-main` 的 Usage Logs 移动端卡片和筛选体验，NexusTok 普通 Usage Logs 已经完成语义化移动卡片，但账号池 `history` 分区仍使用手写宽表格。账号池日志包含 NexusTok 独有的账号组、账号名、认证类型、状态迁移、检测任务进度和脱敏审计字段，不能直接复用普通 Usage Logs 的 column card，否则会丢失账号粒度和运行态排障语义。

本轮目标是先把 new-api 的“手机端按日志语义阅读”优势转成 NexusTok 原生能力：账号池 `Usage Logs`、`State Logs`、`Check History` 在手机端渲染专用卡片，桌面端继续保留当前表格、分页、导出和清理动作。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 账号池移动端日志卡片 | `web/default/src/features/account-pool/components/account-pool-log-mobile-cards.tsx` | 新增账号池专用移动端列表，分别渲染使用日志、状态日志和检测任务，不复用全局 Usage Logs 的列定义。 |
| 账号池 History 接入 | `web/default/src/features/account-pool/index.tsx` | 三个 History 子视图在 `md` 以下使用移动端卡片，`md` 及以上保留原桌面表格；账号池内部 Tabs 改为横向滚动，避免手机端换行重叠。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新后续优先队列，并记录本轮需求、方案、风险、实施和运行态验证。 |

本轮没有修改后端接口、数据库、权限、计费、relay、账号池调度、导出接口或检测任务清理逻辑。

### 风险评估

- 字段丢失风险：账号池日志比普通 Usage Logs 多账号组、账号认证类型、before/after 状态、检测任务进度等字段。本轮新增账号池专用组件，避免把账号池数据强行映射成全局日志列。
- 桌面回归风险：账号池 History 是管理排障高频页面。本轮只在 `md` 以下展示卡片，桌面表格 DOM 和列结构保留，减少运营习惯和宽屏批量扫描回归。
- 权限与写操作风险：`Export audit`、`Cleanup`、检测任务 `View` 等原按钮继续由原页面控制；移动卡片只读展示，并复用现有 `viewCheckTask`、request/account 筛选回调，不新增写入路径。
- 移动端布局风险：账号池主分区 Tabs 原先在手机端换行后可能重叠。本轮改为横向滚动，稳定 32px 高度，避免文字和按钮互相遮挡。
- 热更新验证风险：必须以 `http://192.168.0.202:3003/` 的真实页面为准。本轮用 Chrome headless/CDP 登录 3003 并设置 390px 手机视口验证页面，而不是只依赖本地 build。

### 方案评审

采用“账号池专用卡片 + 桌面表格保留”的低风险方案：

1. 新增 `AccountPoolUsageLogsMobileList`，首行展示模型和账号，字段区展示时间、request、账号、渠道、quota/token/use time、stream 状态、失败状态码、错误信息和 retry index。
2. 新增 `AccountPoolStateLogsMobileList`，展示 action/source/actor、账号、reason、before/after 状态快照和 next retry，继续支持点击 request/account 复用现有状态日志筛选回调。
3. 新增 `AccountPoolCheckTasksMobileList`，展示任务 ID、账号组、状态、进度条、通过/失败/跳过数量、actor、created/finished、request、message 和 `View` 按钮。
4. 对三个列表添加 `data-account-pool-log-mobile-list=usage/state/check` 只读标记，方便运行态验证移动端路径是否真实生效。
5. 桌面表格容器改为 `hidden md:block`，移动卡片容器使用 `md:hidden`，保持两个端点互不干扰。
6. 账号池页面内部 Tabs 从 `flex-wrap` 改为横向滚动，避免手机端主分区导航换行重叠。
7. 不在本轮迁移 `LogsFilterToolbar` 或 `DataTablePage`，因为它们会扩大状态、列显隐和 URL 同步影响面；移动筛选 Drawer 已在后续切片补齐，DataTable 分层继续留作后续独立切片。

### 实施结果

已完成账号池 History 移动端日志卡片原生化：

- 账号池 Usage Logs 在手机端显示专用空态或卡片，不再显示横向滚动宽表格；当前 3003 运行态 Usage Logs 为空时显示账号池专用空态。
- 账号池 State Logs 在手机端显示真实状态变更卡片，保留 action/source/actor、账号组、账号认证类型、reason、before/after 状态与 request 筛选入口。
- 账号池 Check History 在手机端显示真实检测任务卡片，保留进度条、结果统计、actor、created/finished、request、message 和 `View` 入口。
- 手机端三个 History 子视图可见表格数均为 0，证明移动端路径不再依赖桌面表格横向滚动。
- 账号池主分区 Tabs 在 390px 视口下高度稳定为 32px，`overflow-x=auto`，不再发生换行重叠。
- 本轮没有新增 UI 文案；所有使用到的文案均已存在于六语 locale，`i18n:sync` 后无 locale 文件变更。

### 验证记录

1. `cd web/default && bun run typecheck` 通过。
2. `cd web/default && bun run build` 通过。
3. `cd web/default && bun run i18n:sync` 通过，未产生 locale 变更。
4. `cd web/default && bunx prettier --check src/features/account-pool/index.tsx src/features/account-pool/components/account-pool-log-mobile-cards.tsx` 通过。
5. `git diff --check` 通过。
6. `cd web/default && bun run format:check` 未通过，但失败列表为仓库既有大量无关格式债；本轮两个账号池文件的 targeted Prettier check 已通过。
7. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200，热更新入口可访问。
8. 当前会话没有暴露 MCP 浏览器工具；本轮使用 Google Chrome headless + CDP 替代真实浏览器验证，入口仍为 `http://192.168.0.202:3003/`。
9. 使用账号 `c1cada` 登录后，以 390px 手机视口打开 `/account-pool/history`，等待 skeleton 消失。
10. Usage Logs 视图验证：`data-account-pool-log-mobile-list="usage"` 可见，当前运行态为空态，`visibleTables=0`。
11. State Logs 视图验证：`data-account-pool-log-mobile-list="state"` 可见，真实卡片包含 `Manual delete`、`Admin operations`、actor、request、账号和 before/after 状态，`visibleTables=0`。
12. Check History 视图验证：`data-account-pool-log-mobile-list="check"` 可见，真实卡片包含任务 `#6`、`Completed`、进度、结果统计、actor、request、message 和 `View` 按钮，`visibleTables=0`。
13. 账号池主分区 Tabs 验证：手机视口下第一组 Tabs `clientHeight=32`、`overflowX=auto`、`scrollWidth > clientWidth`，不再重叠。
14. Chrome/CDP 记录没有 console error/warning，也没有非取消型网络失败。
15. 验证截图保存于 `/tmp/nexustok-account-pool-history-mobile-ready.png` 与 `/tmp/nexustok-account-pool-history-mobile-tabs.png`。

## 本轮实施评审：账号池 History 移动端筛选 Drawer 原生化

### 需求分析

上一轮已经把账号池 `History` 的三类日志在手机端改成语义卡片，但筛选区仍然直接堆在页面上。尤其是 `State Logs` 同时包含 action、source、request、起止时间和账号筛选，手机端会把日志卡片推到很深的位置；这与 `/opt/project/new-api-main` 最新页面中“主搜索留在页面、低频筛选收纳到移动抽屉”的体验优势仍有差距。

本轮目标是把该优势转成 NexusTok 原生能力：账号池 History 手机端保留高频搜索输入，其余筛选进入底部 Drawer；桌面端继续保持现有筛选栏，不改变后端查询参数、权限、导出、清理、检测任务详情或日志脱敏语义。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 移动筛选 Drawer | `web/default/src/features/account-pool/components/account-pool-history-filter-drawer.tsx` | 新增账号池 History 专用底部筛选 Drawer，统一触发按钮、激活计数、Reset/Done、焦点释放和 85dvh 工作面。 |
| 账号池 History 接入 | `web/default/src/features/account-pool/index.tsx` | Usage/State/Check 三个子视图在 `md` 以下使用“搜索框 + Filter Drawer”；桌面筛选栏改为 `hidden md:flex` 保留原行为。 |
| 移动筛选控件 | `web/default/src/features/account-pool/index.tsx` | Usage/Check 使用按钮组避免小屏下菜单遮挡；State 的 action/source 在移动 Drawer 内使用 `NativeSelect`，避免 Base Select portal 与 Vaul Drawer transform 组合后点不中选项。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新前端体验队列、实施索引和本轮评审记录。 |

本轮没有修改 `/api/account-pool/*` 请求参数、后端接口、数据库、权限判定、账号池调度、导出接口、清理接口或检测任务详情逻辑。

### 风险评估

- 筛选可发现性风险：移动端把低频筛选收进 Drawer 后，管理员可能不知道当前已有筛选。本轮在触发按钮上显示激活数量 badge，搜索框仍保留在页面首行。
- Reset 误清风险：Usage、State、Check 三组筛选状态不同。本轮分别提供 `clearUsageLogFilters`、`clearStateLogFilters`、`clearCheckTaskFilters`，只清当前子视图的筛选，并把对应页码复位到 1。
- Drawer 焦点风险：Vaul 打开 Drawer 时会隐藏页面其它区域；如果触发按钮仍持焦点，浏览器可能产生 aria-hidden 焦点警告。本轮打开前主动 blur 当前焦点，并在 Drawer 打开后把焦点送到第一个可交互控件。
- Select portal 风险：运行态验证发现 Base Select 的 portal popup 在 Vaul Drawer transform 场景中会被 Drawer 内容层覆盖，`elementFromPoint` 命中 Drawer header/body 而不是选项。本轮移动 Drawer 内改用本地控件，避免把共享 Select 层级改动扩散到全站。
- 桌面回归风险：账号池 History 是排障高频页面。本轮只在 `md` 以下启用新 Drawer，桌面端原 Select、Input、时间筛选、账号筛选 badge、导出和清理入口保持原 DOM 路径。
- 热更新验证风险：必须以 `http://192.168.0.202:3003/` 真实页面为准。本轮每次关键调整后都通过 Chrome headless/CDP 打开 3003 实测，而不是只看本地类型检查。

### 方案评审

采用“移动端专用筛选 Drawer + 桌面筛选栏保留”的低风险方案：

1. 新增 `AccountPoolHistoryFilterDrawer`，只负责移动端筛选容器、触发按钮、激活计数、Reset/Done 和焦点管理，不感知具体筛选字段。
2. Usage Logs 手机端保留搜索框，`success/all/failed` 状态放入 Drawer 按钮组；桌面端原三按钮 + 搜索框不变。
3. State Logs 手机端保留搜索框，action/source 使用 `NativeSelect`，request/start/end/account filter 放入 Drawer；桌面端仍使用原 Base Select 和原布局。
4. Check History 手机端保留搜索框，status 使用按钮组；桌面端仍使用原 Base Select。
5. Drawer 使用稳定 `85dvh` 高度和内部滚动，避免短内容底部抽屉让控件贴近屏幕边缘，也为后续继续加入筛选字段留出空间。
6. 不迁移全局 `LogsFilterToolbar`、URL 同步或 DataTable 分层，避免把移动端体验修复扩大成表格架构重构。

### 实施结果

已完成账号池 History 移动端筛选 Drawer 原生化：

- 新增 `AccountPoolHistoryFilterDrawer`，触发按钮带 `data-account-pool-history-filter-trigger=usage/state/check`，运行态可稳定定位验证。
- Usage Logs 手机端展示搜索框 + Filter；点击 Filter 后 Drawer 中显示 `All`、`Success`、`Failed`，选择失败状态后触发按钮显示计数 `1`，Reset 后恢复无计数。
- State Logs 手机端展示搜索框 + Filter；Drawer 中包含 action、source、request id、start/end datetime 和账号筛选显示/清理区域。action/source 在移动端使用 `NativeSelect`，避免 Base Select portal 在 Vaul Drawer 内被内容层遮挡。
- Check History 手机端展示搜索框 + Filter；Drawer 中显示 `All statuses`、`Queued`、`Running`、`Completed`、`Failed` 按钮组，选择 `Failed` 后触发按钮显示计数 `1`。
- 三组移动筛选计数均包含搜索框：搜索文本仍属于当前视图的激活筛选，Reset 会同时清空搜索和低频筛选。
- Drawer 打开前会 blur 当前焦点，打开后自动聚焦第一个输入/按钮/combobox，避免 aria-hidden 焦点警告。
- 桌面端移动 Drawer trigger 不可见，原筛选输入仍可见；本轮没有改变桌面筛选栏、分页、导出、清理或详情查看行为。

### 验证记录

1. `cd web/default && bunx prettier --write src/features/account-pool/index.tsx src/features/account-pool/components/account-pool-history-filter-drawer.tsx` 通过。
2. `cd web/default && bunx prettier --check src/features/account-pool/index.tsx src/features/account-pool/components/account-pool-history-filter-drawer.tsx src/features/account-pool/components/account-pool-log-mobile-cards.tsx` 通过。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run i18n:sync` 通过，未产生新增翻译缺口；本轮复用已有文案 key。
5. `cd web/default && bun run build` 通过。
6. `git diff --check` 通过。
7. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 `HTTP/1.1 200 OK`，热更新入口可访问。
8. 当前会话没有暴露 MCP 浏览器工具；本轮使用 Google Chrome headless + CDP 替代真实浏览器验证，入口仍为 `http://192.168.0.202:3003/`。
9. 使用账号 `c1cada` 登录后，以 390px 手机视口打开 `/account-pool/history`，初始 `visibleTables=0`，Usage mobile Filter trigger 可见。
10. Usage Logs 验证：打开 `data-account-pool-history-filter-trigger="usage"`，Drawer 高度约 675px，包含 `All/Success/Failed`；点击 `Failed` 后 trigger 文本为 `Filter\n1`，Reset 后恢复 `Filter`。
11. State Logs 验证：打开 `data-account-pool-history-filter-trigger="state"`，Drawer 内可见 2 个 `NativeSelect`、`Request ID`、`Start time`、`End time`；把 action 设为 `manual_delete` 后 trigger 文本为 `Filter\n1`，Reset 后恢复 `Filter`；最终复测确认 Drawer 打开后焦点落在 `SELECT[data-slot="native-select"]`。
12. Check History 验证：打开 `data-account-pool-history-filter-trigger="check"`，Drawer 内可见 `Queued`、`Running`、`Completed`、`Failed` 按钮且没有 `NativeSelect`；点击 `Failed` 后 trigger 文本为 `Filter\n1`。
13. 桌面视口 1280px 验证：usage/state/check 三个移动 Drawer trigger 均不可见，原桌面筛选输入仍可见。
14. Chrome/CDP 记录没有 console error/warning，也没有非取消型网络失败；此前发现的 Base Select portal 与 Drawer transform 点击冲突已通过移动端本地控件规避。
15. 验证截图保存于 `/tmp/nexustok-account-pool-history-filter-drawer.png`。

## 本轮实施评审：Authz 持久策略状态校准

### 需求分析

继续推进差异报告时发现，顶部 `2026-07-10 复核后的后续优先队列` 仍把 `角色模板与 Casbin/策略持久化` 按早期待办状态描述。但当前仓库已经有 `model.AuthzRole`、`model.CasbinRule`、`service/authz.SeedPersistentPolicies()`、启动时主节点种子、持久角色策略优先读取、静态 Admin 基线 fallback 和相关测试；最近实施记录也已经包含“Authz 角色与策略持久化底座原生化”。

本轮目标不是新增授权热路径，而是校准报告状态：把已经能由当前代码证明的“角色/策略持久化底座”标记为已完成，同时明确 new-api-main 仍领先的部分是 Casbin runtime/enforcer、策略周期 reload、用户 override 同表迁移、角色编辑 UI、自定义角色和策略导入导出。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 后续优先队列 | `docs/features/new-api-main-diff-analysis.md` | 将 P1 Authz 行从早期待办状态改为“持久化底座已完成，剩余 runtime/enforcer/UI/导入导出”。 |
| 平台治理路线 | `docs/features/new-api-main-diff-analysis.md` | 更新 P1 平台治理和建议落地顺序，避免后续重复实现 `authz_roles`、`casbin_rule` 与默认 Admin 策略种子。 |
| 实施索引 | `docs/features/new-api-main-diff-analysis.md` | 新增 2026-07-11 Authz 状态校准记录，便于后续回溯。 |

本轮没有修改 Go 代码、前端代码、数据库迁移、路由权限表、用户 override 存储、运行时授权判定或 3003 运行态服务。

### 风险评估

- 最大风险是误把 Casbin runtime 也标成完成。报告现在明确写出 NexusTok 未引入 `casbin.SyncedEnforcer`、未做策略周期 reload、用户 override 仍保留在 `authz_user_overrides`，角色编辑 UI 和导入导出仍是后续能力。
- 第二个风险是只看提交名而不看代码。本轮直接核对 `service/authz/persistent_policy.go`、`service/authz/permission.go`、`model/authz_role.go`、`model/casbin_rule.go`，并对照 `/opt/project/new-api-main/service/authz/{enforcer.go,seed.go,override.go}` 后再改文档。
- 授权安全风险为零到低：本轮不修改运行时代码，也不改变任何权限判定、角色基线、路由注册或数据库写入路径。
- 报告一致性风险仍存在：该文档很长，历史实施评审会保留当时语境；本轮只修正会影响后续决策的总表、路线建议和实施索引。

### 方案评审

采用“证据驱动的状态校准”方案：

1. 当前项目证据：`AuthzRole` 固定表名 `authz_roles`，`CasbinRule` 固定表名 `casbin_rule`，`SeedPersistentPolicies()` 会种子 Root/Admin 内置角色并写入 Admin 默认 allow 策略。
2. 授权读取证据：`Can()` 对非 Root 管理角色会先读用户 override，再读持久角色策略，策略缺失或无有效 allow 时回退 catalog 静态基线。
3. new-api-main 差异证据：new-api-main 已有 `casbin.SyncedEnforcer`、GORM adapter、`ReloadPolicy()`、`StartPolicySync()` 和基于 `casbin_rule` 的用户策略写入；这些仍未在 NexusTok 完整落地。
4. 文档策略：把“策略持久化底座”从待办中移出，但保留 Casbin runtime、策略同步、角色编辑、自定义角色、导入导出和用户 override 同表迁移为后续项。
5. 不把本轮扩大成授权代码变更，避免为了修报告而触碰高风险授权热路径。

### 实施结果

已完成 Authz 持久策略状态校准：

- 后续优先队列的 `角色模板与 Casbin/策略持久化` 行已更新为：`authz_roles`、`casbin_rule`、Root/Admin 种子、Admin 默认策略、持久策略优先读取和静态 fallback 已原生化。
- 同一行保留明确剩余差异：Casbin runtime/enforcer、策略周期 reload、用户 override 迁入 `casbin_rule`、角色编辑 UI、自定义角色和策略导入导出。
- `P1：平台治理能力` 已改为表达当前持久策略底座已落地，后续重点转向 runtime、同表迁移、角色编辑和更细资源拆分。
- `建议的落地顺序` 已从“下一批聚焦 Casbin/角色策略持久化”改为“下一批聚焦 Casbin runtime/enforcer、角色编辑/导入导出和新增公开模块测试契约”。
- 实施记录索引已新增 `2026-07-11 | Authz 持久策略状态校准`。

### 验证记录

1. `rg -n "Authz 持久策略状态校准|authz_roles|casbin_rule|Casbin runtime/enforcer" docs/features/new-api-main-diff-analysis.md` 用于确认新状态和剩余边界已写入。
2. `rg -n "SeedPersistentPolicies|persistentRoleAllows|persistentRoleGrants|type AuthzRole|type CasbinRule" service/authz model` 确认当前项目确实存在持久策略底座实现。
3. `rg -n "casbin.SyncedEnforcer|StartPolicySync|SetUserPermissionsInTx" /opt/project/new-api-main/service/authz` 确认剩余差异来自 new-api-main 的 Casbin runtime/enforcer 和策略同步实现。
4. `git diff --check` 通过。
5. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 `HTTP/1.1 200 OK`；本轮只改文档，运行态入口保持可访问。

## 本轮实施评审：账号池认证文件权限资源拆分原生化

### 需求分析

`new-api-main` 的管理权限优势在于资源边界更细，敏感配置不会长期混在同一个粗粒度资源里。NexusTok 已经把账号池作为原生一等能力实现，但 `/api/account-pool/auth-files*` 仍挂在 `account_pool` 资源下，导致认证文件、账号生命周期、分组、日志、检测和运行态操作共用同一组权限。认证文件承载真实上游凭证 JSON，风险等级高于普通账号池分组和日志查看；如果后续要给运营角色开放账号池健康、分组和历史记录，就需要先把凭证文件从大资源中拆出来。

本轮目标是把 new-api-main 的细粒度授权优势转换为 NexusTok 原生账号池保护能力：新增 `account_pool_auth_file` 资源，只覆盖认证文件列表/详情读取和导入/更新/删除，不改变接口 URL、数据模型、账号池调度、OAuth、账号生命周期或凭证导入格式。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| Authz catalog | `service/authz/permission.go`、`service/authz/registry.go` | 新增 `account_pool_auth_file` 资源和 `read/sensitive_write` 两个实际使用动作；默认 Admin 只授予 read，Root 仍是 superuser。 |
| 持久策略种子 | `service/authz/persistent_policy_test.go` | 锁定 Admin 持久策略会写入 `account_pool_auth_file.read`，不会写入 `account_pool_auth_file.sensitive_write`。 |
| 账号池路由权限 | `router/account_pool-router.go`、`router/account_pool_router_test.go` | `/auth-files` 列表/详情切到新 read，创建、导入、更新、删除切到新 sensitive_write；其它账号池路由继续归属 `account_pool`。 |
| 用户权限回传 | `controller/user_authz_test.go` | `/api/user/self` 的 `permissions.admin_permissions` 必须包含新资源，普通 Admin 默认可读、不可敏感写，普通用户不可读。 |
| 默认前端权限矩阵 | `web/default/src/lib/admin-permissions.ts`、`admin-permissions.test.ts` | 新增前端资源常量和 legacy Admin 静态 fallback，避免旧登录态缺后端矩阵时误判。 |
| 账号池凭证页面 | `web/default/src/features/account-pool/index.tsx`、`components/auth-files-panel.tsx` | 凭证面板按新 read 权限开关查询；导入/编辑/删除按新 sensitive_write 禁用；添加账号弹窗的“选择凭证”查询同样受新 read 保护。 |
| i18n | `web/default/src/i18n/static-keys.ts`、`locales/*.json` | 新增权限 catalog 动态文案六语翻译；旧的账号池敏感写描述改为不再包含 auth files。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新后续队列、建议路线、实施索引和本轮评审记录。 |

本轮没有修改数据库 schema、账号池认证文件模型、加解密逻辑、导入解析器、账号池调度、Relay 热路径、OAuth/设备授权、渠道账号池绑定或 classic 前端。

### 风险评估

- 权限收紧风险：已有用户 override 里如果只配置了 `account_pool.read`，不会自动继承到 `account_pool_auth_file.read`。本轮用 Admin 默认 read 和前端静态 fallback 保持普通 Admin 读取兼容，但显式 override 场景会按新资源收紧，这是拆分资源的预期行为。
- 敏感写误开放风险：如果直接复用 `managementActions()` 并给 Admin 默认 write，可能误把认证文件导入/删除开放给普通 Admin。本轮只注册 read 和 sensitive_write，且测试锁定 Admin 不具备 sensitive_write。
- 页面 403 风险：凭证面板和添加账号弹窗原先打开即查询 auth-files。本轮对两个查询都加 `enabled: canReadAccountPoolAuthFile`，无读权限时不发请求，避免无权限用户进入页面后产生 403 噪音。
- 路由顺序风险：`/auth-files/import` 必须继续排在 `/auth-files/:auth_file_id` 之前，避免 Gin 把 `import` 当成参数。本轮只改 permission，不调整路由顺序，并补齐 6 条 auth-files 路由分类测试。
- 入口独立性风险：本轮把 `account_pool_auth_file` 定义为账号池页面内的附加资源，进入 `/account-pool` 仍需要 `account_pool.read`；后续如果要允许“只有凭证读权限、没有账号池读权限”的角色进入，需要单独做 section-aware route guard 和侧边栏子项过滤评审。
- i18n 风险：权限 catalog 文案是后端动态返回，普通源码扫描不会自动发现。本轮把动态 key 写入 `static-keys.ts` 并手动补齐 en/zh/fr/ja/ru/vi。

### 方案评审

采用“小资源拆分 + URL 不变 + 页面查询开关”的低风险方案：

1. 新增 `ResourceAccountPoolAuthFile = "account_pool_auth_file"`，并只暴露 `AccountPoolAuthFileRead` 与 `AccountPoolAuthFileSensitiveWrite` 两个当前路由实际消费的 permission。
2. catalog 中新增 `Account Pool Auth Files`，动作只包含 `Read account pool auth files` 和 `Edit account pool auth files`；Admin 默认只拥有 read。
3. `/api/account-pool/auth-files`、`/api/account-pool/auth-files/:auth_file_id` 切到 `account_pool_auth_file.read`。
4. `/api/account-pool/auth-files` POST、`/auth-files/import`、PUT/DELETE `/:auth_file_id` 切到 `account_pool_auth_file.sensitive_write`。
5. 前端新增 `ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL_AUTH_FILE`，凭证面板读取和敏感写按钮分别消费新资源，不再复用 `account_pool.sensitive_write`。
6. 添加账号弹窗中的“选择凭证”只在有新 read 权限时请求认证文件；无读权限时默认切到 `Manual`，并禁用 `Select Credentials`。
7. 不迁移审计 action 名、不改 API URL、不做用户 override 迁移，避免把本轮权限资源拆分扩大到审计报表和数据迁移。

### 实施结果

已完成账号池认证文件权限资源拆分：

- 后端 catalog 返回新资源 `account_pool_auth_file`，仅包含 `read` 和 `sensitive_write` 两个动作。
- 默认 Admin 基线和持久策略种子只授予 `account_pool_auth_file.read`；Root 仍按 superuser 拥有全部已注册动作。
- `/api/account-pool/auth-files*` 六条路由已经从 `account_pool` 拆出；其余账号池分组、账号、日志、检测、OAuth 和运行态操作仍保持原 `account_pool` 权限。
- 默认前端凭证面板、导入按钮、编辑/删除按钮和添加账号弹窗的选择凭证路径均消费新权限。
- 动态权限 catalog 文案已补齐六语：`Account Pool Auth Files`、`Read account pool auth files`、`Edit account pool auth files` 及其描述。
- 差异报告的 P1/P2 Authz 资源继续细分队列已更新：认证文件已拆出；后续追加实施已继续拆出 `channel_account`，当前剩余重点转为返回字段脱敏、审计增强、非敏感写边界和角色编辑 UI。

### 验证记录

1. `go test ./service/authz -run 'TestCatalogIncludesNexusTokCoreResources|TestRolesExposeRootAndAdminBaselines|TestCapabilitiesFollowExistingSystemRoles|TestCanFollowsRoleBaselinesAndFailsClosed|TestSeedPersistentPoliciesStoresBuiltInRolesAndAdminPolicies'` 通过。
2. `go test ./router -run 'TestRegisterAccountPoolRoutesKeepsCoreHandlers|TestAccountPoolPermissionRoutes'` 通过。
3. `go test ./controller -run 'TestGetSelfReturnsAdminPermissions|TestGetSelfIncludesAuthzUserOverrides|TestGetUserReturnsAdminPermissions|TestUpdateUserAdminPermissions'` 通过。
4. `go test ./service/authz ./router ./controller -run 'Authz|AccountPool|GetSelfReturnsAdminPermissions|GetSelfIncludesAuthzUserOverrides|GetUserReturnsAdminPermissions|UpdateUserAdminPermissions'` 通过；`service/authz` 在该宽泛正则下无匹配测试，精确 authz 测试见第 1 条。
5. `cd web/default && bun test src/lib/admin-permissions.test.ts` 通过。
6. `cd web/default && bunx prettier --check src/features/account-pool/index.tsx src/features/account-pool/components/auth-files-panel.tsx src/lib/admin-permissions.ts src/lib/admin-permissions.test.ts src/i18n/static-keys.ts src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` 通过。
7. `cd web/default && bun run i18n:sync` 通过，报告显示 en/zh/fr/ja/ru/vi `missingCount=0`、`extrasCount=0`。
8. `cd web/default && bun run typecheck` 通过。
9. `cd web/default && bun run build` 通过。
10. `git diff --check` 通过。
11. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 `HTTP/1.1 200 OK`，热更新入口可访问，未重启容器。
12. 当前会话没有暴露 MCP 浏览器工具；本轮使用 Google Chrome headless + DevTools Protocol 替代真实浏览器验证，入口仍为 `http://192.168.0.202:3003/`。
13. 使用账号 `c1cada` 登录后打开 `/account-pool/credentials`，运行态 `/api/authz/catalog` 返回 `account_pool_auth_file`，actions 为 `read` 和 `sensitive_write`。
14. `/api/user/self` 中 `permissions.admin_permissions.account_pool_auth_file` 对 Root 用户显示 `read=true`、`sensitive_write=true`；普通 Admin 默认敏感写 false 已由后端和前端单元测试覆盖。
15. 运行态凭证页显示 `Account Credentials`、搜索框 placeholder `Search credentials`、`Import Credential` 按钮和空态 `No credentials found`；没有出现无权限状态。
16. 运行态 `/api/account-pool/auth-files?p=1&page_size=10` 返回 HTTP 200 且 `success=true`。
17. Chrome/CDP 记录没有 console warning/error、运行时 exception 或非取消型网络失败。

## 本轮实施评审：编辑渠道页左侧分区导航对齐与模型搜索追加复验

### 需求分析

用户继续反馈“搜索添加时不正确”，并指出 `/opt/project/new-api-main` 最新版编辑渠道页面体验更好，希望 NexusTok 的编辑渠道页面继续向该页面对齐。结合前几轮 `gpt-5.6` 模型同步上下文，本轮先以 3003 运行态复现，再决定是否修改搜索逻辑。

复现结论如下：

1. 运行态 `/api/models/search?keyword=gpt-5.6&p=1&page_size=50` 返回 3 条模型：`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，三者 `vendor_id=1`，供应商为 OpenAI。
2. 运行态渠道 `11111` 当前保存的模型为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，因此搜索 `gpt-5.6` 时应只新增缺失的 `terra/luna` 两个模型，而不是新增 3 个。
3. 真实页面复现显示模型搜索弹层为 `3 matched · 2 new · 1 already selected`，候选列表只显示 `gpt-5.6-terra` 和 `gpt-5.6-luna`，没有出现 `Press Enter to use "gpt-5.6"` 或把前缀创建为自定义模型的入口。
4. 因此搜索追加主逻辑当前是正确的；本轮更需要处理的是“编辑渠道页与 new-api 图二的结构差异”：NexusTok 桌面端仍使用顶部横向分区导航，而 new-api 最新页面使用左侧垂直分区导航，更适合长表单扫描、定位和高级设置展开。

本轮目标是把 new-api 最新编辑渠道页的左侧信息架构转换为 NexusTok 原生能力，同时保留 NexusTok 已有的权限拆分、账号池、Codex OAuth、模型搜索补齐、自定义模型输入、上游模型检测和高级配置能力。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 编辑渠道抽屉导航 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | `lg` 及以上视口改为 new-api 风格左侧垂直导航；`lg` 以下保留顶部横向导航，避免手机端失去快速分区入口。 |
| 编辑渠道抽屉布局 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 表单内容区从单列改为桌面两列：左侧 17rem 导航，右侧主表单。 |
| 模型搜索追加 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`、`web/default/src/components/multi-select.tsx` | 本轮不改算法，只复验运行态行为，确认当前已正确区分命中数、可新增数和已存在数。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求分析、影响范围、风险评估、方案评审、实施结果和运行态验证。 |

本轮没有修改后端接口、数据库 schema、模型同步任务、供应商元数据、渠道保存 payload、权限 catalog、计费、relay、账号池调度或模型搜索 API。

### 风险评估

- 长表单布局风险：编辑渠道页包含身份、凭证、模型、映射、高级设置、参数覆盖和上游模型检测等大量字段。直接覆盖成 new-api 文件会丢失 NexusTok 独有能力；本轮只移植导航结构，不做整文件替换。
- 移动端回归风险：new-api 桌面左侧导航在小屏隐藏。若 NexusTok 也完全隐藏，小屏编辑长表单会失去快速跳转能力。本轮在 `lg` 以下保留原顶部横向导航。
- 滚动定位风险：左侧导航与表单滚动容器必须继续共用现有 `handleEditorNavNavigate` 和 active section 计算。本轮不改锚点 id、scrollIntoView 逻辑或高级设置展开逻辑。
- 搜索追加误判风险：当前渠道已有 `gpt-5.6-sol` 时，按钮显示 `Add 2 new model(s)` 是正确行为；报告中明确写出数据依据，避免把“命中 3 条”误解为“应新增 3 条”。
- 热更新验证风险：必须以 `http://192.168.0.202:3003/` 真实页面为准。本轮使用 Chrome headless + CDP 登录 3003 验证，未仅依赖本地构建。

### 方案评审

采用“桌面对齐 new-api 左侧导航 + 小屏保留 NexusTok 横向导航 + 搜索追加复验不改算法”的方案：

1. `ChannelEditorNav` 抽出统一的导航按钮和状态标记渲染逻辑，避免桌面/移动两套导航状态分叉。
2. `lg:hidden` 分支保留原顶部横向导航，适合小屏横向滚动。
3. `lg:block` 分支新增左侧垂直 `aside`，显示 provider、状态进度、主分区和高级设置子分区，结构对齐 new-api 最新编辑页。
4. 表单主体外层改为 `lg:grid-cols-[17rem_minmax(0,1fr)]`，在桌面端形成左侧导航、右侧表单的稳定布局；小屏仍为单列。
5. 不改 `getModelSearchModelNames`、`buildModelSearchAppendSummary`、`fetchAllModelSearchModelNames` 或 `MultiSelect` 创建策略，避免把已验证正确的搜索补齐路径引入新风险。

### 实施结果

已完成编辑渠道页左侧导航对齐：

- 1440px 桌面视口下，编辑渠道抽屉显示左侧垂直导航：provider 为 `Codex`，状态为 `Enabled · 3/3`，分区为 `Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings`。
- 桌面端顶部横向导航隐藏，避免与左侧导航重复。
- 小屏端仍保留顶部横向导航，继续支持移动端快速分区跳转。
- 现有搜索追加逻辑保持不变：输入 `gpt-5.6` 时显示 `3 matched · 2 new · 1 already selected`，只追加缺失的 `gpt-5.6-terra` 和 `gpt-5.6-luna`。
- 点击追加后表单草稿从 `Selected 3` 变为 `Selected 5`，并同时包含 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`；验证过程中未点击 `Update Channel`，未保存运行态渠道。

### 验证记录

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts` 通过，27 个模型搜索与 MultiSelect 用例全部成功。
2. `cd web/default && bunx prettier --check src/features/channels/components/drawers/channel-mutate-drawer.tsx` 通过。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `cd web/default && bun run i18n:sync` 通过，本轮未新增 UI 文案。
6. `git diff --check` 通过。
7. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 HTTP 200，热更新入口可访问。
8. 当前会话没有暴露 MCP 浏览器工具；本轮使用 Google Chrome headless + DevTools Protocol 替代真实浏览器验证，入口仍为 `http://192.168.0.202:3003/`。
9. 使用账号 `c1cada` 登录 3003 后打开 `/channels`，编辑渠道 `11111`。
10. 桌面 1440x1000 视口验证：`sideNavVisible=true`、`topNavVisible=false`、抽屉宽度约 1152px，左侧导航文本包含 `Codex`、`Enabled · 3/3`、`Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings`。
11. 同一页面输入 `gpt-5.6` 后，弹层文本为 `Search results`、`3 matched · 2 new · 1 already selected`、`gpt-5.6-terra, gpt-5.6-luna` 和 `Add 2 new model(s)`，没有自定义创建 `gpt-5.6` 的入口。
12. 点击 `Add 2 new model(s)` 后，表单显示 `Selected 5`，并包含 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`；弹层关闭。
13. Chrome/CDP 网络记录显示 `/api/channel/models`、`/api/channel/?tag_mode=false&id_sort=false&p=1&page_size=20`、`/api/channel/1`、`/api/models/search?keyword=gpt-5.6&p=1&page_size=50`、`/api/models/search?keyword=gpt-5.6&p=1&page_size=100` 均返回 200。
14. Chrome/CDP 记录没有 console error/warning，也没有非取消型网络失败。
15. 运行态验证截图保存于 `/tmp/nexustok-channel-left-nav-search-add.png`。

## 本轮实施评审：渠道账号权限资源拆分与页面操作对齐

### 需求分析

继续推进 `new-api-main` 差异吸收时，`channel_account` 是上一轮复核队列中的高价值细分资源。只读对比后确认：`new-api-main` 本身没有 NexusTok 的渠道内账号池接口和页面能力，它只有较粗的 `channel` 管理资源；NexusTok 反而已经具备更完整的渠道内账号模型、`/api/channel/:id/accounts*` API 和默认前端弹窗。

因此本轮不是复制 `new-api-main` 的缺失实现，而是把它的“资源级授权治理”优势发展成 NexusTok 原生能力：将渠道内账号从 `channel` 和全局 `account_pool` 的混合语义中拆出来，新增独立 `channel_account` 权限资源。

本轮目标：

1. 新增 `channel_account` resource，并只注册当前实际需要的 `read`、`operate`、`sensitive_write` 三类动作。
2. `/api/channel/:id/accounts*` URL 和 handler 保持不变，但路由权限从 `channel.*` 迁移到 `channel_account.*`。
3. 默认 Admin 保持可读和可操作，避免现有管理员失去渠道账号日常运维入口；敏感写仍默认关闭，需要 Root 或显式授权。
4. 前端渠道行菜单和 `ChannelAccountPoolDialog` 按新资源拆分：读列表、启停/清冷却、新增/导入/编辑/删除分别使用不同能力。
5. 动态权限 catalog 文案补齐六语，并把实施结果写回差异报告。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 后端权限 catalog | `service/authz/permission.go`、`service/authz/registry.go` | 新增 `ResourceChannelAccount`、`ChannelAccountRead`、`ChannelAccountOperate`、`ChannelAccountSensitiveWrite`；catalog 注册 `Channel Accounts`。 |
| 持久策略种子 | `service/authz/persistent_policy.go` 间接受影响，测试在 `persistent_policy_test.go` | `SeedPersistentPolicies()` 启动时会按 catalog 重置内置 Admin 默认策略，本轮测试锁定会写入 `channel_account.read/operate`，不会写入 sensitive_write。 |
| 渠道路由权限表 | `router/channel-router.go` | 仅迁移 `/api/channel/:id/accounts*` 八条子路由；渠道 CRUD、测试、余额、Codex OAuth、Ollama、multi-key 和 upstream updates 不变。 |
| 用户权限回传 | `controller/user_authz_test.go` | `/api/user/self`、`/api/user/:id` 的权限矩阵随 catalog 自动包含新资源，测试补齐 Root/Admin/普通用户断言。 |
| 前端权限兜底 | `web/default/src/lib/admin-permissions.ts` | 静态 Admin baseline 新增 `channel_account.read/operate=true`，`sensitive_write=false`，用于旧会话或 catalog 临时不可用兜底。 |
| 渠道页面权限 hook | `web/default/src/features/channels/hooks/use-channel-permissions.ts` | 新增 `canReadChannelAccount`、`canOperateChannelAccount`、`canSensitiveWriteChannelAccount`。 |
| 渠道行菜单 | `data-table-row-actions.tsx` | `Account Pool` 入口从 `account_pool.read + channel.sensitive_write` 改为 `channel_account.read`。 |
| 渠道账号弹窗 | `channel-account-pool-dialog.tsx` | 查询开关、刷新、启停/清冷却、新增/批量导入/编辑/删除分别接新权限。 |
| i18n | `static-keys.ts`、`locales/{en,zh,fr,ja,ru,vi}.json` | 新增动态 catalog 文案翻译。 |

### 风险评估

- 既有管理员入口回归风险：如果默认 Admin 不授予 `channel_account.read`，普通管理员会突然看不到渠道账号弹窗。本轮默认授予 read/operate，锁定 sensitive_write 不默认授予。
- 权限过宽风险：如果继续复用 `channel.sensitive_write`，授予渠道账号维护能力会顺带授予渠道创建、删除、复制和敏感渠道配置修改。本轮用独立 `channel_account.sensitive_write` 收窄该风险。
- 前后端不一致风险：原弹窗状态启停使用 `channel.sensitive_write`，而后端旧路由是 `channel.operate`。本轮统一迁移为 `channel_account.operate`。
- 全局账号池混淆风险：原行菜单入口要求 `account_pool.read`，会把全局账号池查看权限和渠道内账号查看权限绑在一起。本轮移除该耦合。
- 热更新策略风险：运行态若后端未热更新，会看不到 `channel_account` catalog。本轮已访问 3003 并确认 `/api/authz/catalog` 与 `/api/user/self` 都包含新资源；无需重启容器。
- 返回字段风险：子 agent 指出渠道账号 read 响应仍可能包含 `base_url`、`other/settings`、请求覆盖等字段。当前接口本轮不改响应结构，后续可以单独评审“read 字段脱敏”和是否新增 `channel_account.write`。

### 方案评审

采用“小资源拆分 + URL 不变 + 前端按动作消费 + 文案补齐”的方案：

1. 不改数据库 schema，不改 controller payload，不改 API URL，降低对 Relay 热路径和现有页面的冲击。
2. 后端只注册三类动作：`read` 覆盖列表和详情；`operate` 覆盖启停/清冷却；`sensitive_write` 覆盖新增、批量创建、multi-key 导入、更新和删除。
3. 不注册 `channel_account.write`，因为渠道账号字段大多直接或间接影响凭证、路由和请求改写，非敏感写边界尚未充分评审。
4. 前端入口允许只读用户打开弹窗查看列表；弹窗内部再按动作禁用按钮并在直接调用时 toast 无权限。
5. 保持 `Account Pool` 现有弹窗标题和菜单文案，避免本轮 UI 文案大范围重命名；权限 catalog 中使用更准确的 `Channel Accounts`。

### 实施结果

已完成渠道账号权限资源拆分：

- `authz.Catalog()` 新增 `channel_account`，actions 为 `read`、`operate`、`sensitive_write`。
- Root 对新资源仍为 superuser；Admin 默认具备 `read/operate`，不具备 `sensitive_write`。
- `SeedPersistentPolicies()` 的默认 Admin 策略会写入 `channel_account.read` 和 `channel_account.operate`，不会写入 `channel_account.sensitive_write`。
- `/api/channel/:id/accounts` 和 `/api/channel/:id/accounts/:account_id` 改为 `channel_account.read`。
- `/api/channel/:id/accounts/:account_id/status` 改为 `channel_account.operate`。
- `/api/channel/:id/accounts`、`/batch`、`/import-multikey`、PUT/DELETE `/:account_id` 改为 `channel_account.sensitive_write`。
- 默认前端行菜单 `Account Pool` 入口改为只依赖 `canReadChannelAccount`。
- `ChannelAccountPoolDialog` 查询列表时要求 `canReadChannelAccount`；启停/清冷却要求 `canOperateChannelAccount`；新增、批量导入、从 multi-key 导入、编辑、删除要求 `canSensitiveWriteChannelAccount`。
- 动态权限 catalog 文案已补齐 en/zh/fr/ja/ru/vi。
- 顶部后续优先队列已更新：`channel_account` 资源拆分不再作为待评估项，后续转向字段脱敏、非敏感写边界、审计增强和角色编辑 UI。

### 验证记录

1. `go test ./service/authz -run 'TestCatalogIncludesNexusTokCoreResources|TestRolesExposeRootAndAdminBaselines|TestCapabilitiesFollowExistingSystemRoles|TestCanFollowsRoleBaselinesAndFailsClosed|TestSeedPersistentPoliciesStoresBuiltInRolesAndAdminPolicies|TestPersistentPolicyFallbackWhenPolicyTableMissing'` 通过。
2. `go test ./router -run 'TestRegisterChannelRoutesKeepsCoreHandlers|TestRegisterChannelRoutesKeepsRootProtectedHandlers|TestChannelPermissionRoutesClassifyCoreActions|TestChannelPermissionRoutesClassifyChannelAccountActions|TestChannelPermissionRoutesKeepLegacySensitiveMiddlewares'` 通过。
3. `go test ./controller -run 'TestGetSelfReturnsAdminPermissions|TestGetSelfIncludesAuthzUserOverrides|TestGetUserReturnsAdminPermissions|TestUpdateUserAdminPermissions'` 通过。
4. `go test ./service/authz ./router ./controller` 通过。
5. `cd web/default && bun test src/lib/admin-permissions.test.ts` 通过。
6. `cd web/default && bunx prettier --check src/lib/admin-permissions.ts src/lib/admin-permissions.test.ts src/features/channels/hooks/use-channel-permissions.ts src/features/channels/components/data-table-row-actions.tsx src/features/channels/components/dialogs/channel-account-pool-dialog.tsx src/i18n/static-keys.ts src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` 通过。
7. `cd web/default && bun run i18n:sync` 通过，报告显示所有语言 `missingCount=0`、`extrasCount=0`。
8. `cd web/default && bun run typecheck` 通过。
9. `cd web/default && bun run build` 通过。
10. `git diff --check` 通过。
11. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 HTTP 200。
12. 运行态登录 `c1cada` 后访问 `/api/user/self`，返回 `permissions.admin_permissions.channel_account = { read: true, operate: true, sensitive_write: true }`，符合 Root superuser 预期。
13. 运行态 `/api/authz/catalog` 返回 `channel_account`，动作和描述为 `read / operate / sensitive_write`。
14. 当前会话没有暴露 MCP 浏览器工具；本轮使用 Google Chrome headless + DevTools Protocol 替代真实浏览器验证，入口仍为 `http://192.168.0.202:3003/`。
15. 使用真实浏览器打开 `/channels`，点击渠道 `11111` 行菜单中的 `Account Pool`，菜单项可点击且未禁用。
16. 渠道账号弹窗成功打开，页面显示 `Account Pool`、`Total: 0`、`Enabled: 0`、`Refresh`、`Batch Import`、`Add Account`、`No accounts found`。
17. 浏览器网络记录显示 `/api/channel/1/accounts?p=1&page_size=10` 返回 HTTP 200。
18. Chrome/CDP 记录没有 console error/warning，也没有非取消型网络失败。
19. 运行态验证截图保存于 `/tmp/nexustok-channel-account-dialog.png`。

## 本轮实施评审：渠道账号只读响应脱敏

### 需求分析

上一轮已经把渠道内账号拆成独立 `channel_account` 资源，并让默认 Admin 具备 `read/operate`。继续复核后发现：`ListChannelAccounts` 和 `GetChannelAccount` 的 read 响应虽然已经对 `key` 做脱敏，但仍会返回上游地址、OpenAI 组织 ID、`other/settings`、模型映射、参数覆盖、请求头覆盖、状态码映射和原始错误详情。这些字段足以暴露私有上游配置或请求改写策略，不应仅凭 `channel_account.read` 暴露。

本轮目标是把 `channel_account.read` 收敛为“查看运行态账号列表和脱敏元数据”，只有具备 `channel_account.sensitive_write` 的管理员才能读取完整高级配置，以匹配新增资源的权限语义。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 渠道账号响应 | `controller/channel_account.go` | 新增按上下文权限构造响应；基础响应不包含高级敏感配置，敏感写用户仍可获得完整字段。 |
| 单元测试 | `controller/channel_account_response_test.go` | 新增普通 Admin 与 Root 两类响应断言，锁定只读脱敏和 Root 可见完整配置。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 顶部后续队列和本轮实施记录同步更新。 |

本轮不改 API URL、路由权限、数据库 schema、前端页面、保存 payload、Relay 选择逻辑或账号字段存储方式。

### 风险评估

- 编辑回填风险：如果拥有敏感写权限的用户也拿不到高级配置，会影响后续编辑。实现中使用 `authz.Can(userID, role, authz.ChannelAccountSensitiveWrite)` 判断，Root 和显式敏感写用户仍可见完整字段。
- 只读页面回归风险：渠道账号弹窗表格当前只展示名称、脱敏 key、状态、模型、分组、优先级、权重、冷却等基础字段，本轮全部保留。
- 错误详情泄露风险：`last_error` 可能包含上游地址、供应商原文或请求上下文，因此归入敏感字段，仅敏感写用户可见。
- 权限重复判断风险：路由中间件已经检查过 read，本轮 controller 再判断 sensitive_write 仅用于字段可见性，不改变请求通过/拒绝结果。

### 方案评审

采用“基础响应 + 敏感字段追加”的低风险方案：

1. 新增 `channelAccountResponseForContext(c, account)`，从 Gin 上下文读取 `id` 和 `role`，并调用 `authz.Can` 判断是否具备 `ChannelAccountSensitiveWrite`。
2. `channelAccountResponse(account, includeSensitive)` 默认返回基础字段：id、channel_id、name、脱敏 key、status、models、group、priority、weight、last_used_time、used_quota、冷却字段、disabled_reason、max_concurrency、created_time。
3. `includeSensitive=true` 时才追加：base_url、openai_organization、other、setting、settings、model_mapping、param_override、header_override、status_code_mapping、last_error。
4. 创建和更新接口仍返回完整字段，因为它们本身已经受 `channel_account.sensitive_write` 路由权限保护；只读和详情接口会根据调用者是否额外拥有敏感写权限自动脱敏。

### 实施结果

已完成渠道账号只读响应脱敏：

- 普通 Admin 在只读上下文中只能拿到脱敏 key 和运行态元数据。
- Root 或具备 `channel_account.sensitive_write` 的用户仍能拿到完整高级配置。
- `last_error` 已从基础 read 响应中移除，避免泄露上游原始错误细节。
- 前端渠道账号弹窗不需要调整，因为表格使用的字段都在基础响应中保留。

### 验证记录

1. `go test ./controller -run 'TestChannelAccountResponse|TestRegister|TestGetSelfReturnsAdminPermissions'` 通过。
2. `go test ./controller` 通过。
3. `go test ./service/authz ./router ./controller` 通过。
4. `git diff --check` 通过。
5. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 HTTP 200。
6. 运行态登录 `c1cada` 后调用 `/api/channel/1/accounts?p=1&page_size=10`，返回 HTTP 200，当前渠道账号为空，响应为 `items=[]` 和全 0 stats。
7. 当前会话没有暴露 MCP 浏览器工具；本轮使用 Google Chrome headless + DevTools Protocol 替代真实浏览器验证，入口仍为 `http://192.168.0.202:3003/`。
8. 使用真实浏览器打开 `/channels`，点击渠道 `11111` 行菜单中的 `Account Pool`，渠道账号弹窗成功打开。
9. 页面显示 `Account Pool`、`Total: 0`、`Enabled: 0`、`Batch Import`、`Add Account`、`No accounts found`。
10. 浏览器网络记录显示 `/api/channel/1/accounts?p=1&page_size=10` 返回 HTTP 200。
11. Chrome/CDP 记录没有 console error/warning，也没有非取消型网络失败。

## 本轮实施评审：渠道模型搜索规则展开与编辑渠道页对齐复核

### 需求分析

用户继续反馈“搜索添加时不正确”，并再次确认 `gpt-5.6` 应有 3 种 OpenAI 模型。本轮先复核 3003 运行态接口与最新版 `/opt/project/new-api-main` 编辑渠道页结构，再决定最小改动范围。

复核结论如下：

1. 运行态 `/api/models/search?keyword=gpt-5.6&p=1&page_size=50` 返回 `total=3`，三条精确模型分别为 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，`vendor_id=1`，供应商为 OpenAI。
2. 渠道 `11111` 当前后端保存的模型仍是 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，因此页面搜索 `gpt-5.6` 时正确行为是命中 3 个、实际新增缺失的 `terra/luna` 两个，不能把“命中 3 个”误写成“新增 3 个”。
3. 继续对照 `/opt/project/new-api-main/web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 和 `sections/channel-models-section.tsx` 后确认，NexusTok 当前编辑渠道页已经具备 new-api 最新页面的核心信息架构：右侧大抽屉、`Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings`、左侧状态导航和分段滚动定位。
4. NexusTok 当前页面在 new-api 基础上还有原生增强：`Credential Mode`、全局账号池组、Codex OAuth、模型元信息搜索追加、独立自定义模型输入、Quick actions/More 操作收敛、字段级权限、模型映射 guardrail、上游模型检测和高级设置子导航。因此本轮不做整页替换，避免把已增强能力退回 new-api 的简化版本。
5. 新发现的搜索边界在前端候选提取 helper：模型搜索接口除精确模型外，还可能返回 `name_rule > 0` 的规则模型，并在 `matched_models` 中携带展开后的真实模型名。旧逻辑只读取 `model_name`，会漏掉规则展开出的具体模型，或把规则模型占位名错误加入渠道。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 渠道模型搜索 helper | `web/default/src/features/channels/lib/model-search.ts` | `getModelSearchModelNames()` 支持 `name_rule` 与 `matched_models`：规则模型只使用展开后的真实匹配模型；精确模型继续保留 `model_name`。 |
| 渠道模型搜索测试 | `web/default/src/features/channels/lib/model-search.test.ts` | 增加规则模型展开、规则占位名不入选、精确模型继续保留自身名称的测试。 |
| 编辑渠道页面 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 本轮不改页面结构；复核确认当前页面已经与 new-api 最新编辑渠道页的信息架构对齐，并保留 NexusTok 原生增强。 |
| 后端与数据 | 无 | 不改模型同步、供应商、渠道保存接口、数据库 schema、relay、计费或权限。 |

### 风险评估

- 规则占位名误加入风险：`name_rule > 0` 的 `model_name` 可能是系列、正则或规则名称，不一定是可暴露给 `/v1/models` 的真实模型。本轮规则模型只取 `matched_models`，避免把占位名写入渠道能力。
- 标签命中污染风险：`/api/models/search` 仍可能因为 description/tags 命中无关模型。本轮对所有候选名再次按关键词过滤，只把候选名本身包含搜索词的模型交给追加逻辑。
- 精确模型回归风险：当前 3003 上 `gpt-5.6-terra/luna/sol` 都是 `name_rule=0` 精确模型。本轮保留精确模型 `model_name`，不会影响当前运行态路径。
- 页面大改风险：直接复制 new-api 编辑渠道页会丢失 NexusTok 独有的账号池、权限和搜索增强。本轮只修搜索候选提取边界，并把页面对齐结论写入报告。
- 保存数据风险：页面验证只操作表单草稿并点击搜索追加按钮，未点击 `Update Channel`；后端渠道模型保持原值。

### 方案评审

采用“候选提取增强 + 页面结构复核不替换”的低风险方案：

1. `ModelSearchItemLike` 增加可选 `name_rule` 和 `matched_models`，与模型搜索接口现有响应兼容。
2. `getModelSearchModelNames()` 对 `name_rule > 0` 的结果只遍历 `matched_models`；对精确模型遍历 `model_name + matched_models`，并继续做 trim、大小写不敏感去重和关键词过滤。
3. 保持 `getMissingModelSearchMatches()`、`buildModelSearchAppendPlan()`、`fetchAllModelSearchModelNames()` 和渠道保存 payload 不变，继续让“命中数/新增数/已存在数”来自同一套去重规则。
4. 当前编辑渠道页已吸收 new-api 最新页面的左侧分区导航和四段式信息架构，本轮不重复重构 UI；后续若要进一步靠拢，只应按具体交互差异逐项评审。

### 实施结果

已完成渠道模型搜索规则展开修复：

- 搜索结果项为规则模型时，会把 `matched_models` 中的真实模型纳入候选。
- 规则模型自身的 `model_name` 不再被当作渠道模型名加入，避免出现 `gpt-5.6` 这类系列前缀或规则占位名。
- 精确模型仍保留 `model_name`，当前 3003 上的 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 路径不受影响。
- 搜索结果继续按关键词过滤，description/tags 命中的无关模型不会进入渠道追加列表。
- 编辑渠道页复核通过：当前页面已经呈现 new-api 最新编辑页的核心分段结构，并保留 NexusTok 的账号池、Codex、权限、模型映射和上游检测增强。

### 验证记录

1. `cd web/default && bun test src/features/channels/lib/model-search.test.ts` 通过，14 个用例全部成功，新增覆盖规则模型 `matched_models` 展开、规则占位名排除、精确模型保留。
2. `cd web/default && bunx prettier --check src/features/channels/lib/model-search.ts src/features/channels/lib/model-search.test.ts` 通过。
3. `cd web/default && bun run typecheck` 通过。
4. `git diff --check` 通过。
5. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 HTTP 200，热更新入口可访问。
6. 运行态 `/api/models/search?keyword=gpt-5.6&p=1&page_size=50` 返回 3 条 OpenAI 精确模型：`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，均为 `vendor_id=1`、`name_rule=0`。
7. 当前会话没有暴露 MCP 浏览器工具；本轮使用 Google Chrome headless + DevTools Protocol 替代真实浏览器验证，入口仍为 `http://192.168.0.202:3003/`。
8. 使用账号 `c1cada` 登录 3003 后进入 `/channels`，打开渠道 `11111` 的 `Edit Channel` 抽屉；页面包含 `Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings`，初始模型显示 `Selected 3`。
9. 在模型搜索输入中输入 `gpt-5.6`，页面显示 `Search results`、`3 matched · 2 new · 1 already selected`、预览 `gpt-5.6-terra, gpt-5.6-luna` 和按钮 `Add 2 new model(s)`；未出现 `Press Enter to use "gpt-5.6"` 或自定义创建前缀入口。
10. 点击 `Add 2 new model(s)` 后，表单草稿显示 `Selected 5`，并包含 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`。
11. 验证后通过页面上下文重新请求 `/api/channel/?p=0&page_size=5`，渠道 `11111` 后端保存值仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认未误保存草稿。
12. Chrome/CDP 网络记录显示 `/api/channel/models`、`/api/channel/?tag_mode=false&id_sort=false&p=1&page_size=20`、`/api/channel/1`、`/api/models/search?keyword=gpt-5.6&p=1&page_size=50`、点击追加时的 `/api/models/search?keyword=gpt-5.6&p=1&page_size=100`、以及复查 `/api/channel/?p=0&page_size=5` 均返回 HTTP 200。
13. Chrome/CDP 记录没有 console error/warning、运行时 exception 或非取消型网络失败。
14. 运行态验证截图保存于 `/tmp/nexustok-channel-model-search-rule-fix.png`。

## 本轮实施评审：Authz 持久角色策略快照与周期同步

### 需求分析

继续推进 `new-api-main` 差异吸收时，当前最高优先级仍是角色模板与 Casbin/策略持久化。对照 `/opt/project/new-api-main/service/authz` 后确认，new-api 已经具备 `adapter.go`、`enforcer.go`、`resolver.go`、`role.go`、`seed.go` 和 `assignment.go`，通过 Casbin enforcer 将 `casbin_rule` 加载到内存，并提供 `ReloadPolicy()` 和 `StartPolicySync()` 周期刷新。

NexusTok 当前已经有 `authz_roles`、`casbin_rule`、Root/Admin 内置角色种子和 Admin 默认策略写入，也让 `Can()` / `Roles()` 读取持久角色策略。但旧实现每次权限判定都直接读数据库，缺少运行态策略快照、显式 reload 和多节点周期同步语义；这会让后续角色编辑 UI 或策略导入导出缺少可靠的“写库后刷新”边界。

本轮目标不是一次性引入完整 Casbin runtime/enforcer，也不新增角色编辑 UI，而是先把 new-api 的“内存策略快照 + reload + 周期同步”优势转换成 NexusTok 原生能力，为后续 Casbin runtime、角色 UI、自定义角色和用户 override 迁移打底。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| Authz 持久策略服务 | `service/authz/persistent_policy.go` | 新增角色策略内存快照、`ReloadPersistentPolicies()`、`StartPersistentPolicySync()`，并让 `persistentRoleGrants()` 优先读取快照。 |
| 启动初始化 | `main.go` | 主节点 seed 后自动 reload；从节点启动时只 reload；所有节点按 `common.SyncFrequency` 周期刷新策略快照。 |
| Authz 测试 | `service/authz/persistent_policy_test.go`、`service/authz/override_test.go` | 锁定“数据库策略变更必须 reload 后生效”的运行态语义，并清理全局快照避免测试互相污染。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新 P1 队列状态并记录本轮需求、风险、方案、实施和验证。 |

本轮不改变 `authz_roles` / `casbin_rule` 表结构，不引入 `github.com/casbin/casbin/v2`，不改变任何管理路由权限分类，不改前端权限矩阵和用户 override 保存格式。

### 风险评估

- 授权语义风险：快照引入后，直接修改 `casbin_rule` 不再立即影响 `Can()`，必须 reload。这与 new-api enforcer 语义一致，也更适合后续多节点部署；测试已明确锁定该行为。
- 启动风险：主节点仍先完成数据库 migration 和内置策略 seed，再 reload；从节点只加载现有策略，加载失败只记录日志并由后台同步重试，避免从节点因短暂迁移时序直接退出。
- 性能风险：旧实现每次读取角色策略都查库，快照后可以降低授权路径数据库压力；用户级 override 暂时仍按请求读库，保持现有实时性和兼容性。
- 回退风险：如果策略表缺失、清空或只存在未知策略，仍保留静态 Admin 基线 fallback，避免把管理员瞬间降为全拒绝。
- 范围风险：完整 Casbin runtime、用户 override 迁入 `casbin_rule` 和角色编辑 UI 都是更大范围变更，本轮明确不混入，降低对核心管理面的冲击。

### 方案评审

采用“轻量快照层，暂不引入 Casbin runtime”的方案：

1. 保留现有 `casbin_rule` 表约定和 `role:<key>` subject 命名。
2. 新增进程内 `persistentPolicySnapshot`，保存内置非 superuser 角色的权限矩阵。
3. `SeedPersistentPolicies()` 完成内置角色和 Admin 默认策略写入后立即 `ReloadPersistentPolicies()`，让主节点启动即使用快照。
4. 从节点启动不重置内置策略，只 `ReloadPersistentPolicies()`；失败时日志告警，后台周期同步继续重试。
5. `persistentRoleGrants()` 优先读快照；快照未初始化时继续读数据库，以兼容单元测试、迁移过渡和未经过启动流程的调用点。
6. 后续角色编辑 UI 或导入导出只需要写入 `casbin_rule` 后调用 reload 或等待同步，不需要再改所有授权调用点。

### 实施结果

已完成 Authz 持久角色策略快照原生化：

- 新增 `ReloadPersistentPolicies()`，可从 `casbin_rule` 重新构建角色策略快照。
- 新增 `StartPersistentPolicySync(frequency)`，按 `common.SyncFrequency` 周期刷新策略快照。
- `Can()`、`Roles()` 和 `CapabilitiesForUser()` 通过 `persistentRoleGrants()` 优先消费内存快照。
- 主节点启动时 seed 内置策略并 reload；从节点启动时只 reload；所有节点启动后台同步循环。
- 单元测试已覆盖数据库策略删除在 reload 前不影响快照、reload 后才影响 Admin 基线的语义。
- 测试 helper 清理全局快照，避免 Authz 测试之间串状态。

### 验证记录

1. `go test ./service/authz` 通过。
2. `go test ./router ./controller` 通过，确认路由权限表和控制器权限回传未回归。
3. `go test ./model -run 'TestAuthz|TestUserDeleteClearsAuthz|TestUserEditDoesNotOverwrite'` 通过，确认用户生命周期与 authz override 清理仍稳定。
4. `gofmt -w service/authz/persistent_policy.go service/authz/persistent_policy_test.go service/authz/override_test.go main.go` 已执行。
5. `go test .` 通过，确认根包启动代码编译正常。
6. `git diff --check` 通过。
7. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 HTTP 200，热更新入口可访问。
8. 当前会话没有暴露 MCP 浏览器工具；本轮使用 `curl` 登录 3003 并调用真实后端接口验证。
9. 使用账号 `c1cada` 登录 `POST /api/user/login?turnstile=` 返回 HTTP 200、`success=true`、`role=100`。
10. 携带登录 cookie 和 `NexusTok-User: 1` 调用 `GET /api/authz/catalog` 返回 HTTP 200，响应包含 `channel`、`channel_account`、`account_pool`、`account_pool_auth_file`、`user`、`model`、`subscription`、`redemption` 等资源和 Root/Admin 角色基线，确认 Authz catalog 运行态接口未回归。

## 本轮实施评审：编辑渠道页搜索追加上下文与 new-api 页面体验对齐

### 2026-07-11 补充复核

用户再次反馈“搜索添加时不正确”，并要求继续按 `/opt/project/new-api-main` 最新编辑渠道页对齐。复核 3003 运行态后确认：`/api/models/search?keyword=gpt-5.6&p=1&page_size=50` 返回的是 3 条 OpenAI 精确模型，分别为 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，`vendor_id=1`、`name_rule=0`。因此本轮不改模型同步和供应商归属，而是修正默认前端的搜索追加交互层。

上一轮已补充搜索上下文校验和全量分页扫描，但批量追加按钮仍放在 `MultiSelect` 下拉内容里。这个位置会与候选选中、弹层关闭、输入清空等 Combobox 行为争抢状态，用户搜索 `gpt-5.6` 时容易只选中一个候选，误以为“搜索添加”只同步了一个模型。本轮将远程模型库搜索从模型多选弹层中独立出来，新增 `Search model library` 输入、稳定的 `Search results` 摘要和独立追加按钮；模型多选本身继续负责本地选择/自定义模型，避免把模型族前缀 `gpt-5.6` 误写为一个真实模型。

页面对齐方面，本轮继续吸收 new-api 最新编辑渠道页的低风险结构细节：渠道类型图标抽象为 `ChannelTypeLogo`，未知自定义类型回退为 `Server` 图标；抽屉标题、左侧导航和 Type 输入框使用同一 Logo 表达；编辑渠道时不再在基础信息区展示 `Enabled` 开关，保持编辑页更聚焦配置内容，渠道启停仍由渠道列表操作入口承担。NexusTok 的细粒度权限、账号池、Codex OAuth、模型映射 guardrail、上游模型检测和高级设置子导航继续保留。

### 需求分析

本轮用户反馈集中在两个点：第一，编辑渠道时搜索 `gpt-5.6` 后“搜索添加”不正确；第二，希望将当前编辑渠道页面与 `/opt/project/new-api-main` 最新编辑渠道页面对齐。

运行态复核确认，3003 环境的 `/api/models/search?keyword=gpt-5.6` 返回 3 个 OpenAI 精确模型：`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，均为 `vendor_id=1`、`name_rule=0`。渠道 `11111` 当前只保存了 `gpt-5.6-sol`，因此正确行为应是搜索 `gpt-5.6` 时展示 `3 matched · 2 new · 1 already selected`，点击追加只把缺失的 `gpt-5.6-terra`、`gpt-5.6-luna` 合并到表单草稿中，不应添加前缀 `gpt-5.6`，也不应漏掉同供应商下的其他已同步模型。

对照 new-api 最新编辑渠道页后确认，可吸收的优势主要是页面密度和局部交互约束：更收敛的抽屉宽度、较窄的左侧导航、基础信息中先选 `Type` 再填 `Name`、Codex 与 Vertex API Key 新建时只允许单 key 创建、模型映射编辑器给源模型和目标模型提供候选补全。不能直接照搬 new-api 整页，因为 NexusTok 当前已经具备全局账号池、Codex OAuth、字段级权限、非敏感字段白名单、模型元信息搜索追加、模型映射 guardrail、上游模型检测和高级设置子导航等原生增强。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 编辑渠道抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 修复搜索追加上下文，避免旧关键词、旧渠道或已关闭抽屉的异步结果污染当前表单；对齐 new-api 抽屉宽度、左侧导航宽度、基础字段顺序、Codex/Vertex API Key 的单 key 创建限制，并给模型映射编辑器传入候选模型。 |
| 模型搜索 helper | `web/default/src/features/channels/lib/model-search.ts` | 新增搜索追加上下文纯函数，统一判断抽屉打开状态、渠道 ID 和关键词是否仍匹配当前请求。 |
| 模型搜索测试 | `web/default/src/features/channels/lib/model-search.test.ts` | 增加异步搜索追加上下文测试，覆盖关键词大小写/空格容忍、抽屉关闭、渠道切换和关键词变化。 |
| 模型映射编辑器 | `web/default/src/features/channels/components/model-mapping-editor.tsx` | 增加源模型和目标模型候选 `datalist`，使映射编辑时可复用当前渠道模型和系统模型候选。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、风险评估、方案评审、实施结果和验证记录。 |

本轮不修改后端模型同步逻辑、供应商归属、渠道保存接口、数据库 schema、relay、计费或权限模型；现有 Authz 未提交改动也不纳入本轮提交。

### 风险评估

- 旧关键词污染风险：模型搜索输入使用 debounce，请求返回可能晚于用户继续输入。旧实现存在点击追加时使用旧 debounced 关键词或旧搜索结果的风险；本轮改为读取当前输入框关键词，并在 debounce 未完成时只提示 `Searching model metadata...`。
- 跨渠道污染风险：管理员可能在搜索请求返回前切换编辑渠道。旧异步结果如果继续合并，会把 A 渠道搜索结果写入 B 渠道草稿；本轮追加前校验渠道 ID。
- 抽屉关闭污染风险：搜索全量扫描期间抽屉可能关闭。旧响应继续 setValue 会污染下一次打开的表单；本轮校验抽屉仍处于打开状态，并用请求序号丢弃过期响应。
- 整页照搬风险：直接覆盖为 new-api 页面会丢失 NexusTok 已有的账号池、Codex OAuth、权限白名单和模型搜索增强；本轮只吸收明确有价值且低耦合的页面体验。
- 历史渠道兼容风险：若已有历史渠道处于 `multi_key`，编辑时强行隐藏选项可能造成不可恢复。本轮只在新建且当前类型不支持多 key 创建时自动回落；编辑模式或历史值仍保留显示。
- 表单保存风险：搜索追加只更新前端草稿，不自动调用保存接口；验证中刻意不点击 `Update Channel`，确认后端渠道模型保持原值。

### 方案评审

采用“上下文校验修复 + 局部对齐 new-api 优势”的方案：

1. 新增 `isModelSearchAppendContextCurrent()`，把搜索追加的有效性条件收敛为纯函数：抽屉必须打开、渠道 ID 必须一致、关键词 trim 后大小写不敏感一致。
2. `handleAddModelSearchMatches()` 改为读取 `modelSearchKeywordRef.current`，并在当前输入还未完成 debounce 时拒绝用旧结果追加。
3. 搜索结果展示只在 `trimmedModelSearchKeyword === debouncedModelSearchKeyword` 时生效，避免页面上展示旧结果摘要。
4. 保留 `fetchAllModelSearchModelNames()` 的分页全量扫描能力，点击追加时仍按搜索接口真实分页补齐全部匹配模型，而不是只添加首屏候选。
5. 对齐 new-api 编辑页的局部体验：抽屉宽度改为 `sm:max-w-5xl`，左侧导航改为 `lg:grid-cols-[13rem_minmax(0,1fr)]`，基础信息字段改为 `Type` 在前、`Name` 在后。
6. 对齐 new-api 的新建限制：Codex 类型以及 Vertex `api_key` 模式不展示批量/多 key 创建；编辑历史渠道时保留现有值，避免破坏存量配置。
7. 模型映射编辑器增加源/目标模型候选，但不改变映射 JSON 存储格式和 guardrail 逻辑。

### 实施结果

已完成以下改动：

- 修复模型搜索追加使用旧关键词、旧搜索结果或旧渠道上下文的问题。
- 将远程模型库搜索从模型多选弹层内部迁移为独立的 `Search model library` 输入和 `Search results` 摘要，避免批量追加按钮被 Combobox 候选选中/关闭行为干扰。
- `gpt-5.6` 这类模型族关键词会展示 `3 matched · 2 new · 1 already selected`，追加按钮只合并未选择的 `gpt-5.6-terra`、`gpt-5.6-luna`，不会写入前缀 `gpt-5.6`。
- 搜索结果摘要与追加按钮只在当前关键词对应的搜索结果有效时出现。
- 点击 `Add 2 new model(s)` 会全量扫描搜索接口分页结果，并只追加当前渠道缺失的模型。
- 多选框继续允许选择已有模型和添加自定义模型，但在已有匹配候选时不会把搜索前缀误创建为自定义模型。
- 编辑渠道抽屉与 new-api 最新页面在抽屉宽度、左侧导航宽度和基础字段顺序上对齐。
- 渠道类型 Logo 对齐 new-api 的统一展示方式，并对未知自定义渠道类型提供稳定 fallback。
- 编辑态基础信息区隐藏 `Enabled` 开关，渠道运行态启停继续保留在渠道列表操作中。
- Codex 新建渠道时 `Credential Mode` 不再展示 `Multi-Key Rotation`，`Add Mode` 只保留 `Single Key`。
- Vertex API Key 新建渠道同样会自动回落到单 key 创建，避免提交后被表单校验拒绝。
- 模型映射可从当前渠道模型中补全源模型，并可从系统模型候选中补全目标模型。
- 保留 NexusTok 原生能力：全局账号池、Codex OAuth、权限白名单、模型映射 guardrail、上游模型检测和高级设置子导航均未被回退。

### 验证记录

1. `git diff --check` 通过。
2. `cd web/default && bun test src/features/channels/lib/model-search.test.ts` 通过，模型搜索相关 16 个用例全部成功。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run i18n:sync` 通过，确认新增文案已进入六语 locale。
5. `cd web/default && bunx eslint src/features/channels/components/drawers/channel-mutate-drawer.tsx` 通过。
6. `cd web/default && bunx prettier --check src/features/channels/components/drawers/channel-mutate-drawer.tsx src/i18n/locales/en.json src/i18n/locales/zh.json src/i18n/locales/fr.json src/i18n/locales/ja.json src/i18n/locales/ru.json src/i18n/locales/vi.json ../../docs/features/new-api-main-diff-analysis.md` 通过。
7. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 HTTP 200，热更新入口可访问。
8. 当前会话没有暴露 MCP 浏览器工具；本轮使用 Google Chrome headless + DevTools Protocol 替代真实浏览器验证，入口仍为 `http://192.168.0.202:3003/`。
9. 使用账号 `c1cada` 登录 3003 后进入 `/channels`，打开渠道 `11111` 的 `Edit Channel` 抽屉；页面验证到 `sm:max-w-5xl`、`lg:grid-cols-[13rem_minmax(0,1fr)]` 已生效，基础字段顺序为 `Type *`、`Name *`，编辑态基础信息区不展示 `Enabled` 表单 label。
10. 抽屉中可见独立 `Search model library` 输入，placeholder 为 `Search synced models...`，初始追加按钮禁用。
11. 在 `Search model library` 输入中真实输入 `gpt-5.6`，页面请求 `/api/models/search?keyword=gpt-5.6&p=1&page_size=50`，展示 `Search results`、`3 matched · 2 new · 1 already selected`、`Add 2 new model(s)`，并包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`。
12. 点击 `Add 2 new model(s)` 后，页面请求 `/api/models/search?keyword=gpt-5.6&p=1&page_size=100` 做追加扫描，表单草稿显示 `Selected 5`，包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`。
13. 验证后通过页面上下文请求 `/api/channel/?p=1&page_size=5`，渠道 `11111` 后端保存值仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认没有误保存草稿。
14. 新建渠道场景中真实选择 `Codex` 类型后，`Credential Mode` 下拉选项为 `Single Key`、`Account Pool`，不包含 `Multi-Key Rotation`；`Add Mode` 下拉仅包含 `Single Key`。
15. Chrome/CDP 验证期间没有 console error、运行时 exception 或网络失败；唯一 console info 为 i18next/Locize 提示，不属于本次变更错误。

## 本轮实施评审：Authz 用户 override Casbin 兼容镜像

### 需求分析

继续推进 `new-api-main` 的 Authz 差异时，当前剩余缺口之一是用户级策略最终应与角色策略共享 `casbin_rule`。`new-api-main/service/authz/override.go` 的 `SetUserPermissionsInTx()` 会直接写入 `casbin_rule`，并以 `p, user:<id>, resource, action, effect` 形式表达每个用户相对 Admin 基线的 allow/deny override。

NexusTok 当前已经有稳定运行的 `authz_user_overrides` 表、用户详情接口、`/api/user/self` 权限矩阵回传、服务端 `Can()` 判定和多组管理路由 enforcement。直接把授权读取切换到 Casbin runtime 会扩大影响范围，也会让现有前端和测试一次性承受角色编辑、自定义角色和策略 reload 的复杂度。因此本轮目标是做兼容镜像：保留 `authz_user_overrides` 作为当前授权主存储和 UI 语义来源，同时在同一个事务内同步维护 `casbin_rule` 中的用户策略镜像，为后续完整 Casbin runtime/enforcer、策略导入导出和角色编辑 UI 铺路。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 用户 override 模型 | `model/authz_user_override.go` | 替换某个资源的用户 override 时，同步删除并重建对应 `casbin_rule` 用户策略；清空用户授权时同步清理该用户所有 `casbin_rule` 策略。 |
| Authz override 服务 | `service/authz/override.go` | `SetUserPermissionsInTx()` 计算用户 override 差异时复用事务内 Admin 基线读取，避免全局连接与事务互锁。 |
| 持久策略读取 | `service/authz/persistent_policy.go` | 新增事务内 Admin 基线读取 helper；策略快照已加载时继续优先使用快照，未加载时复用调用方事务读取 `casbin_rule`。 |
| 模型测试 | `model/authz_user_override_test.go` | 覆盖替换、清空、软删除和硬删除用户时 Casbin 镜像同步清理。 |
| Authz 测试 | `service/authz/override_test.go` | 覆盖 `SetUserPermissions()` 写入 Casbin 镜像、清空授权清理镜像、事务回滚不残留镜像。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新 P1 队列状态、实施索引和本轮评审记录。 |

本轮不改变 `authz_user_overrides` 表结构，不改管理路由权限分类，不引入 `github.com/casbin/casbin/v2`，不把 `Can()` / `ExplicitUserOverrides()` 的读取主路径切到 `casbin_rule`。

### 风险评估

- 授权语义漂移风险：如果本轮直接让 `Can()` 读取 `casbin_rule`，可能与现有用户 override UI 和缓存语义不一致。本轮只做双写镜像，授权读取仍以 `authz_user_overrides` 为准。
- 事务一致性风险：用户 override 写入和 Casbin 镜像写入必须在同一事务内完成，否则失败时会出现两张表不一致。本轮把镜像替换放在 `ReplaceAuthzUserResourceOverridesInTx()` 内，任一步失败都会回滚。
- SQLite 锁风险：`SetUserPermissionsInTx()` 在事务中计算差异时，如果再通过全局 `model.DB` 查询持久 Admin 基线，SQLite 内存库会互锁。本轮新增事务内读取 helper，已用 `go test -v -timeout 60s ./service/authz` 验证不再卡住。
- 历史数据兼容风险：既有 `authz_user_overrides` 历史记录不会自动回填 `casbin_rule` 镜像。本轮只保证后续写入、替换、清空和用户删除路径一致；如果未来切换授权读取到 Casbin runtime，需要单独做迁移或重建脚本。
- 未来策略类型风险：`clearAuthzUserCasbinPoliciesInTx()` 会删除该用户 `ptype=p` 的所有用户策略，这符合当前“用户级管理权限 override 全部清空”的语义；未来若增加非管理域用户策略，需要先拆 subject 或 ptype。

### 方案评审

采用“legacy 表为主、Casbin 表同步镜像”的低风险方案：

1. `ReplaceAuthzUserResourceOverridesInTx()` 保持先删除旧 legacy override，再同步替换 `casbin_rule` 用户策略，最后写入新 legacy override；全部在同一事务内执行。
2. 用户策略 subject 统一为 `user:<id>`，策略格式为 `p, user:<id>, <resource>, <action>, <effect>`，与 new-api 的 Casbin 策略布局兼容。
3. `ClearAuthzUserOverridesInTx()` 在清理 legacy 表后，同步删除该用户全部 `ptype=p` 的 `casbin_rule` 策略，保证删除用户和清空授权不会留下镜像残留。
4. `SetUserPermissionsInTx()` 计算差异时仍相对当前 Admin 基线；如果持久策略快照已加载，按快照计算，保持运行态一致性；如果未加载，复用当前事务读取 `casbin_rule`，避免 SQLite 锁。
5. 不使用数据库专用 upsert、JSONB 或原始方言语法；继续通过 GORM 的 `Where/Delete/Create/Count` 保持 SQLite、MySQL、PostgreSQL 兼容。

### 实施结果

已完成 Authz 用户 override Casbin 兼容镜像：

- 用户级 allow/deny override 写入时，会同步生成 `casbin_rule` 用户策略镜像。
- 同一用户同一资源的 override 被替换时，对应 `casbin_rule` 镜像也会被替换，不会留下旧 action。
- 清空用户授权、软删除用户、硬删除用户时，legacy override 和 Casbin 镜像都会被清理。
- 事务回滚场景下，legacy override 和 Casbin 镜像都不会残留。
- 事务内差异计算不再通过全局连接读取持久 Admin 基线，修复 SQLite 内存库测试卡住的问题。
- 当前授权读取路径不变：`Can()` 与 `ExplicitUserOverrides()` 仍从 `authz_user_overrides` 读取用户 override；`casbin_rule` 用户策略只作为后续 runtime/enforcer 和导入导出的兼容镜像。

### 验证记录

1. `go test -v -timeout 60s ./service/authz` 通过，覆盖用户 override 写入镜像、清空镜像、事务回滚、持久角色策略 reload 和 fallback。
2. `go test ./service/authz ./model` 通过，确认 Authz 服务层与模型层完整回归稳定。
3. `go test ./controller -run 'TestGetSelfReturnsAdminPermissions|TestGetSelfIncludesAuthzUserOverrides|TestGetUserReturnsAdminPermissions|TestUpdateUserAdminPermissions|TestUpdateUserRejectsNonRootAdminPermissions|TestCreateUserAdminPermissions|TestUpdateUserClearsAdminPermissionsWhenTargetIsCommon|TestDemoteClearsAdminPermissions'` 通过，确认用户权限接口仍与服务端授权语义一致。
4. `git diff --check` 通过。
5. 首次运行 `go test ./service/authz` 曾卡在 `TestSetUserPermissionsStoresOnlyOverrides`，timeout 堆栈定位到事务内通过全局 `model.DB` 读取 `casbin_rule` 导致 SQLite 内存库互锁；已通过 `persistentAdminBaselineInTx()` 修复，并在第 1 条验证中确认通过。
6. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 HTTP 200，热更新入口可访问。
7. `curl --noproxy '*' -sS --max-time 15 http://192.168.0.202:3003/api/status` 返回正常状态 JSON，`success=true`。
8. 本轮是后端 Authz 存储一致性改动，没有新增或修改前端页面；当前会话没有暴露 MCP 浏览器工具，因此使用 curl 验证 3003 运行态入口和状态接口。

## 本轮实施评审：Authz 自定义角色模板 CRUD 原生化

### 需求分析

继续推进 `new-api-main` 平台治理差异时，Authz 角色模板仍缺少 Root 管理员可操作的自定义模板创建、编辑和删除能力。NexusTok 已有 `authz_roles`、`casbin_rule`、Root/Admin 内置角色种子、角色策略矩阵 UI、策略导入导出和 dry-run 保存保护；如果没有自定义模板 CRUD，管理员仍只能围绕内置 Admin 做权限调参，无法先沉淀可复用的审计员、运营员、只读巡检等模板。

本轮目标是补齐“模板管理”而不是“运行时角色分配”：自定义角色创建后显示为 `Template only`，可以继续使用现有权限矩阵编辑策略，但不会自动授予任何用户，也不改变 `Can()` 授权热路径。用户角色分配、完整 Casbin runtime/enforcer 和更细审计增强继续留作后续切片。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| Authz 角色服务 | `service/authz/role_policy.go` | 新增自定义角色模板创建、更新、删除 DTO 与服务函数；限制 key/name，禁止修改内置角色，删除时清理 `role:<key>` 策略并 reload。 |
| Controller / Router | `controller/authz.go`、`router/authz-router.go` | 新增 `POST /api/authz/roles`、`PUT /api/authz/roles/:key`、`DELETE /api/authz/roles/:key`，均保持 `RootAuth` + `system_setting.sensitive_write`。 |
| 模型注释 | `model/authz_role.go` | 更新 `AuthzRole` 说明，明确自定义角色当前为模板，不是运行时分配角色。 |
| 后端测试 | `service/authz/role_policy_test.go`、`router/authz_router_test.go` | 覆盖创建、非法/保留/重复 key、更新自定义模板、内置拒绝、删除清理策略和路由分类。 |
| 默认前端 API/types | `web/default/src/features/system-settings/api.ts`、`types.ts` | 新增角色模板创建、更新、删除 API 封装和响应类型。 |
| 默认前端 Role Policies | `web/default/src/features/system-settings/security/role-policy-section.tsx` | 在现有策略矩阵页增加创建/编辑/删除模板对话框；非内置模板才允许编辑/删除；创建后自动选中新模板。 |
| i18n | `web/default/src/i18n/locales/*.json` | 新增角色模板管理文案的 en/zh/fr/ja/ru/vi 翻译。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新 Authz 当前状态、API 摘要和本轮评审记录。 |

本轮不修改数据库 schema、`Can()` 授权热路径、用户 override 读取语义、角色分配、relay、计费或系统任务。所有数据库操作继续通过 GORM `Create/First/Updates/Delete/Transaction` 实现，保持 SQLite、MySQL、PostgreSQL 兼容。

### 风险评估

- 内置角色误改风险：Root/Admin 是运行时核心角色，不能通过模板 CRUD 修改元数据或删除。本轮 service 层检查 `BuiltIn` 并拒绝更新/删除；Root 策略只读保护保持不变。
- 角色 key 污染风险：key 会进入 `role:<key>` 策略 subject。本轮限制为 `^[a-z][a-z0-9_-]{1,62}$`，拒绝 `root/admin` 和 `role:`、`user:` 前缀，避免 subject 混淆。
- 孤儿策略风险：删除自定义模板时，如果只删 `authz_roles` 会留下不可见的 `casbin_rule` 策略。本轮在同一事务删除模板和 `role:<key>` 策略，提交后 reload。
- 授权热路径风险：自定义模板尚未接入用户分配，若本轮让 `Can()` 读取所有自定义角色会扩大行为面。本轮只管理模板和策略，不改变运行时授权。
- 前端误操作风险：删除模板是破坏性操作；本轮使用确认框展示目标 key 与策略数量。编辑/删除按钮只在选中非内置角色且具备敏感写权限时可用。
- i18n 风险：新增页面文案必须六语完整；本轮新增 key 已写入所有 locale，并通过 `bun run i18n:sync` 检查 `missingCount=0`。

### 方案评审

采用“模板 CRUD 与策略编辑解耦”的低风险方案：

1. `POST /api/authz/roles` 只创建 `BuiltIn=false` 的模板，默认 `enabled=true`，sort 未传时排在当前列表之后。
2. `PUT /api/authz/roles/:key` 只更新 name、description、enabled、sort，不允许重命名 key，也不修改策略矩阵。
3. `DELETE /api/authz/roles/:key` 只允许删除非内置模板，并在同一事务清理该模板的 `casbin_rule` allow 策略。
4. 现有 `PUT /api/authz/roles/:key/policies` 继续负责权限矩阵，仍默认 dry-run，禁用角色不能编辑策略，Root 只读。
5. 前端复用 `/system-settings/security/role-policies`，不新增路由；操作区增加 `Create role`、`Edit role`、`Delete role`，表单字段与后端 DTO 一致。
6. 创建后自动选中新角色，管理员可立即在同一页面配置权限矩阵；删除后清理本地草稿和预览状态。

### 实施结果

已完成 Authz 自定义角色模板 CRUD 原生化：

- 新增 Root-only `POST /api/authz/roles`、`PUT /api/authz/roles/:key`、`DELETE /api/authz/roles/:key`。
- 自定义角色模板 key 会 trim + lowercase，并限制为安全字符；`root/admin` 和 subject 前缀被拒绝。
- 内置 Root/Admin 模板拒绝元数据编辑和删除。
- 删除自定义模板会同步删除 `casbin_rule` 中 `role:<key>` 的策略，并 reload 持久策略快照。
- 默认前端 Role Policies 页新增 `Create role`、`Edit role`、`Delete role` 操作；创建/编辑对话框支持 Key、Name、Description、Enabled、Sort；删除确认展示目标和策略数量。
- 创建后的模板保持 `Template only` 标记，不会误导为已可分配的运行时角色。
- 角色策略矩阵、dry-run 预览、二次保存确认和 Admin 空策略保护均保留。

### 验证记录

1. `go test ./service/authz ./router -run 'Role|Authz'` 通过。
2. `go test ./service/authz ./router ./controller -run 'Authz|Role|Permission'` 通过。
3. `cd web/default && bun test src/features/system-settings/security/role-policy-utils.test.ts` 通过。
4. `cd web/default && bunx eslint src/features/system-settings/security/role-policy-section.tsx src/features/system-settings/types.ts src/features/system-settings/api.ts` 通过。
5. `cd web/default && bun run typecheck` 通过。
6. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en/zh/fr/ja/ru/vi 全部 `missingCount=0`。
7. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 HTTP 200，热更新入口可访问。
8. 使用账号 `c1cada` 登录 3003 后，通过 `POST /api/authz/roles` 创建 `api-smoke-*` 短命角色模板成功，随后 `DELETE /api/authz/roles/:key` 返回 `reloaded=true`，确认新增后端路由已加载，无需重启容器。
9. 使用 Chrome headless + DevTools Protocol 打开 `http://192.168.0.202:3003/system-settings/security/role-policies`，页面可见 `Create role`、`Edit role`、`Delete role` 操作按钮。
10. 在页面创建 `ui-role-test-*` 自定义模板，接口回查显示 `built_in=false`、`runtime_managed=false`、`enabled=true`、`sort=31`、`policy_count=0`，页面展示 `Template only` 标记。
11. 在页面编辑同一模板名称、说明和排序值，接口回查显示 name/description/sort 已更新，角色 key 保持不变。
12. 在页面删除同一模板后，`GET /api/authz/roles` 已查不到该 key，页面也不再显示测试角色；删除 toast 显示已移除 0 条策略，说明无策略模板的清理路径正常。
13. Chrome/CDP 验证期间 `/api/authz/catalog`、`/api/authz/roles`、`/api/authz/roles/:key` 均返回 HTTP 200；没有 console error、runtime exception 或网络失败，唯一 console info 为 i18next/Locize 提示。

## 本轮实施评审：Authz 角色模板生命周期时间展示

### 需求分析

自定义角色模板 CRUD 落地后，Root 管理员已经能创建、编辑、删除模板并维护策略矩阵，但页面仍缺少模板生命周期信息。`authz_roles` 表本身已有 `created_at` 和 `updated_at`，如果 API 和前端不展示这些字段，管理员难以判断模板是最近创建、最近修改，还是历史遗留配置。

本轮目标是把角色模板生命周期时间作为治理能力的一部分原生化：`GET /api/authz/roles` 返回 `created_at/updated_at`，默认前端 Role Policies 页在角色摘要和统计区展示创建/更新时间。该增强只读，不改变角色策略、授权判定或模板 CRUD 行为。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| Authz 角色 DTO | `service/authz/role_policy.go` | `RolePolicyDescriptor` 增加 `created_at`、`updated_at`；持久角色从 `authz_roles` 带出，fallback 内置角色保持 0。 |
| Authz 测试 | `service/authz/role_policy_test.go` | 固定持久角色和新建自定义模板会返回非零生命周期时间。 |
| 默认前端 types/UI | `web/default/src/features/system-settings/types.ts`、`role-policy-section.tsx` | 类型补齐时间字段；Role Summary 展示 Created/Updated，Stats 增加 Last updated。 |
| i18n | `web/default/src/i18n/locales/*.json` | 新增 `Last updated` 六语翻译；复用已有 `Created`、`Updated` 文案。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新 Authz 当前状态和本轮评审记录。 |

本轮不修改数据库 schema、角色创建/更新请求 DTO、策略保存、`Can()`、用户 override、Casbin 兼容策略、路由权限或任何 Relay/计费逻辑。

### 风险评估

- 时间字段为 0 的兼容风险：早期 fallback 内置角色没有数据库记录时会返回 0。前端统一显示 `-`，不伪造时间。
- 前端本地化风险：时间格式使用浏览器 `Intl.DateTimeFormat`，不引入新依赖；新增静态 label 走六语 i18n。
- API 兼容风险：新增 JSON 字段对旧前端兼容；前端类型只读取字段，不写入创建/更新请求。
- 授权语义风险：该切片只展示审计信息，不读取或修改策略矩阵，不影响 Root/Admin/custom template 授权行为。

### 方案评审

采用“只读字段透传 + 页面轻量展示”的方案：

1. 后端 `RolePolicyDescriptor` 增加 `CreatedAt/UpdatedAt`，持久化角色由 `describePersistentRole()` 带出。
2. 现有创建/更新钩子继续由 GORM model 自动维护时间戳，不新增迁移。
3. 前端 `AuthzRolePolicy` 类型补字段，新增 `formatRoleTimestamp()`，0 或缺失显示 `-`。
4. `RoleSummary` 展示 Created/Updated；`RolePolicyStats` 增加 Last updated，帮助管理员快速扫描当前模板状态。
5. 不新增页面、不改变按钮布局、不改变策略 dry-run 流程。

### 实施结果

已完成 Authz 角色模板生命周期时间展示：

- `GET /api/authz/roles` 返回的每个持久角色现在包含 `created_at` 和 `updated_at`。
- 角色策略页面在选中角色摘要中展示 `Created` 与 `Updated`。
- 统计区新增 `Last updated`，与 Enabled/Current policies/Changed/Enable disable 统计并列。
- 内置 fallback 或异常 0 值显示为 `-`，不会生成误导性时间。
- 差异报告已把 Authz 角色模板管理状态更新为包含生命周期时间展示。

### 验证记录

1. `go test ./service/authz -run 'PersistentRoles|CreateRoleTemplate'` 通过，覆盖角色列表与新建模板时间字段。
2. `cd web/default && bunx eslint src/features/system-settings/security/role-policy-section.tsx src/features/system-settings/types.ts` 通过。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en/zh/fr/ja/ru/vi 全部 `missingCount=0`。
5. `cd web/default && bunx prettier --check src/features/system-settings/security/role-policy-section.tsx src/features/system-settings/types.ts src/i18n/locales/en.json src/i18n/locales/zh.json src/i18n/locales/fr.json src/i18n/locales/ja.json src/i18n/locales/ru.json src/i18n/locales/vi.json ../../docs/features/new-api-main-diff-analysis.md` 通过。
6. `git diff --check` 通过。
7. `curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 HTTP 200，热更新入口可访问。
8. 使用账号 `c1cada` 登录 3003 后调用 `GET /api/authz/roles`，响应包含 `created_at` 与 `updated_at` 字段；Root/Admin 样例均返回非零时间戳。
9. 使用 Chrome headless + DevTools Protocol 打开 `http://192.168.0.202:3003/system-settings/security/role-policies`，页面展示 `创建时间`、`更新时间` 和 `最后更新`，Root 角色示例显示真实时间值。
10. Chrome/CDP 验证期间 `/api/authz/catalog` 与 `/api/authz/roles` 均返回 HTTP 200；没有 console error、runtime exception 或网络失败，唯一 console info 为 i18next/Locize 提示。

## 本轮实施评审：Authz 角色策略审计摘要增强

### 需求分析

`new-api-main` 的 Authz 治理链路强调策略变更可追踪。NexusTok 当前已经完成权限 catalog、角色模板、策略导入导出、dry-run 预览、角色策略矩阵和全局管理审计兜底，但 `/api/authz/roles*` 写操作仍主要依赖兜底审计。兜底审计不会读取请求体，且本轮前这些新增 Authz 写路由没有稳定 action 映射，失败路径会退回 `generic`，成功路径也缺少“角色 key、策略数量、dry-run/applied/reloaded”等业务摘要。

本轮目标是把角色模板和角色策略变更审计变成 NexusTok 原生治理能力：Root 在创建/更新/删除角色模板、dry-run/apply 角色策略、导入权限策略后，管理日志中能看到稳定 `authz.*` action 和非敏感摘要，便于 Usage Logs、导出和后续审计报表消费。

### 影响范围

| 范围 | 文件 | 影响 |
|------|------|------|
| Authz controller | `controller/authz.go` | 在角色模板 CRUD、角色策略更新和策略导入成功路径写入手动管理审计，并标记已审计，避免兜底重复记录。 |
| 审计模板 | `controller/audit.go` | 新增 `authz.role_create`、`authz.role_update`、`authz.role_delete`、`authz.role_policies_update` 和 `authz.policies_import` 兜底内容模板。 |
| 审计中间件 | `middleware/audit.go` | 为 Authz 写路由补稳定 action 映射，失败路径仍由兜底审计记录 HTTP/业务失败状态。 |
| 回归测试 | `controller/authz_audit_test.go`、`middleware/audit_test.go`、`middleware/authz_test.go` | 覆盖成功路径审计摘要、不写完整 grants 矩阵、路由 action 不退回 generic，并补齐用户 override 测试库中的 Casbin 镜像表迁移。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新 Authz 当前状态、后续队列和本轮评审记录。 |

本轮不修改数据库 schema、`Can()` 授权热路径、Casbin 快照、用户 override、角色分配、前端页面、Relay 或计费逻辑。

### 风险评估

1. 角色策略 grants 矩阵可能很大，也可能暴露完整管理边界。本轮只记录角色 key、dry-run/applied/reloaded、旧/新/新增/删除/不变策略数量和导入摘要，不把完整 grants 写入日志。
2. 手动审计会设置 `audit_logged` 标记。若成功路径手写日志后兜底仍执行，会产生重复管理日志；本轮复用现有 `recordManageAudit()`，由中间件检查该标记避免重复。
3. 失败路径不能因为手写审计而丢失。controller 只在 service 成功后写手动日志；解析失败、权限失败、service 错误仍由全局审计兜底记录路由、状态和业务 success。
4. 策略导入可能一次性涉及多个角色和用户 override。日志只记录计数摘要和 mode/dry_run/applied，不记录导入原文或策略明细，避免日志膨胀和敏感配置泄露。

### 方案评审

采用“成功路径业务摘要 + 失败路径中间件兜底”的小步方案：

1. Authz 写 handler 在 service 成功返回后调用 `recordManageAudit()`。
2. 角色模板审计记录 `role_key`、`role_name`、`enabled`、`sort`、`built_in`、`runtime_managed` 和 `policy_count`。
3. 角色策略更新审计记录 `dry_run`、`applied`、`reloaded`、`old_policy_count`、`new_policy_count`、`created_policy_count`、`deleted_policy_count` 和 `unchanged_policy_count`。
4. 角色删除审计记录 `role_key`、`deleted_policy_count` 和 `reloaded`。
5. 策略导入审计记录 mode、dry-run/apply 状态、角色/策略/用户策略数量和创建/更新/删除摘要。
6. `middleware/audit.go` 同步补 Authz 写路由 action 映射，保证失败路径不退回 `generic`。

### 验收方式

1. `go test ./controller -run 'Test(CreatePermissionRoleWritesAuditSummary|UpdatePermissionRoleWritesAuditSummary|UpdatePermissionRolePoliciesWritesCountsWithoutGrants)'`。
2. `go test ./middleware -run 'TestAdminAudit(AuthzRoutesHaveStableActions|RecordsWriteRequest|SkipsManuallyLoggedRequest)'`。
3. `go test ./controller ./middleware ./router -run 'Authz|Audit|Permission|Role'`。
4. `git diff --check`。
5. 登录 3003 后调用 Authz 角色接口创建/删除短命角色模板，随后通过 `/api/log/?type=3` 查询管理日志，确认存在 `authz.role_create` 和 `authz.role_delete` 结构化 action。
6. 访问 `http://192.168.0.202:3003/` 确认运行态仍可访问；本轮没有新增前端页面时，以真实 HTTP/API 验证替代 MCP 页面操作。

### 实施结果

已完成 Authz 角色策略审计摘要增强：

- `POST /api/authz/roles`、`PUT /api/authz/roles/:key`、`DELETE /api/authz/roles/:key`、`PUT /api/authz/roles/:key/policies` 和 `POST /api/authz/policies/import` 成功后会写入 `authz.*` 管理审计 action。
- 审计参数只包含角色 key、角色名称、启用状态、排序、策略计数、dry-run/apply 状态和导入摘要，不包含完整 grants 矩阵。
- Authz 写路由的中间件兜底 action 已补齐；失败路径会保留稳定 action、路由、状态码和业务 success。
- 该增强不改变授权判定、角色策略保存、策略导入事务、前端角色矩阵或现有日志表结构。

### 验证记录

1. `go test ./controller -run 'Test(CreatePermissionRoleWritesAuditSummary|UpdatePermissionRoleWritesAuditSummary|UpdatePermissionRolePoliciesWritesCountsWithoutGrants)'` 通过。
2. `go test ./middleware -run 'TestAdminAudit(AuthzRoutesHaveStableActions|RecordsWriteRequest|SkipsManuallyLoggedRequest)'` 通过。
3. `go test ./middleware` 通过，补齐 `TestRequirePermissionHonorsUserOverrides` 测试库中的 `casbin_rule` 迁移后，用户 override 的 Casbin 镜像写入不再缺表。
4. `go test ./controller ./middleware ./router -run 'Authz|Audit|Permission|Role'` 通过。
5. `go test ./service/authz -run 'Role|Policy|Persistent|Override'` 通过。
6. `git diff --check` 通过。
7. 3003 首页验证通过：`curl --noproxy '*' -I --max-time 10 http://192.168.0.202:3003/` 返回 HTTP 200。
8. 使用账号 `c1cada` 登录 3003 成功，创建临时角色模板 `audit-smoke-*` 后立即删除，接口均返回 `success=true`。
9. 调用 `GET /api/log/?type=3&p=1&page_size=20` 确认最新两条管理日志分别为 `authz.role_create` 和 `authz.role_delete`，`Other.op.params` 包含 `role_key`、`role_name`、`enabled`、`sort`、`policy_count`、`deleted_policy_count` 和 `reloaded` 等非敏感摘要；没有完整 grants 矩阵。
10. 再次调用 `GET /api/authz/roles`，确认临时 `audit-smoke-*` 模板已删除且无测试数据残留。

## 本轮实施评审：渠道编辑页模型搜索追加与 new-api 编辑流对齐

### 需求分析

用户反馈渠道编辑页“搜索添加时不正确”，并要求参考 `/opt/project/new-api-main` 最新编辑渠道页面，将当前项目的编辑渠道页体验向 new-api-main 对齐。结合运行态数据验证，当前 3003 服务中 `/api/models/search?keyword=gpt-5.6` 已返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个模型元数据，但现有渠道 `11111` 只包含 `gpt-5.6-sol`。后端模型库没有丢失，问题集中在默认前端将模型库搜索结果追加到渠道 `models` 草稿时的提取和展示语义。

`new-api-main` 的渠道编辑页优势是核心配置流更接近管理员实际工作顺序：先选择模型并用快捷操作填充，再补充自定义模型，随后设置可用分组，最后处理模型重定向。NexusTok 当前 default 前端已经具备更强的原生能力，包括权限锁、敏感字段保护、Codex OAuth、模型映射风险提示、上游拉取弹窗和搜索模型库补齐。本轮目标不是照搬 Semi UI，而是在保留这些原生增强的前提下修复搜索追加逻辑，并把模型区顺序调整为更接近 new-api 的配置流程。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道模型搜索纯函数 | `web/default/src/features/channels/lib/model-search.ts` | 搜索结果提取改为尊重后端 `/api/models/search` 已完成的 model_name/description/tags 过滤，不再要求返回模型名本身包含关键词。 |
| 渠道模型搜索测试 | `web/default/src/features/channels/lib/model-search.test.ts` | 增加供应商/标签/描述命中仍可追加模型的覆盖；保留大小写去重、缺失项计算、上下文防旧请求污染等既有覆盖。 |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 搜索追加按钮在存在未扫描分页时显示“添加全部匹配模型”语义；搜索输入清空后不再使用旧搜索 total 驱动按钮状态；模型区顺序调整为模型选择/搜索/自定义/快捷操作 -> 分组 -> 模型映射。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审和验证结果。 |

本轮不修改后端 API、数据库 schema、Relay、计费、渠道权限接口、模型同步接口或现有模型元数据记录。

### 风险评估

1. 搜索追加范围扩大：此前前端二次要求模型名包含关键词，搜索 `OpenAI`、`Reasoning` 或描述字段时会误丢模型。本轮改为信任后端搜索结果，管理员输入过宽关键词时可能追加更多模型。为降低风险，页面仍展示命中、新增、已存在和未扫描分页数量，并在真正追加前按当前表单模型列表再次去重。
2. 规则模型占位名风险：名称规则模型如果没有运行时展开结果，直接加入规则占位名可能误导渠道能力。本轮保留保护逻辑：规则模型优先使用 `matched_models`，只有规则名自身命中关键词且无展开结果时才保留占位名。
3. 表单顺序调整风险：分组块从模型映射后移动到模型映射前，字段名、表单控制器和提交 payload 不变；只影响视觉顺序和 `MultiSelect` 禁用态，不改变保存语义。
4. 旧搜索状态残留风险：追加成功后搜索输入会被清空，如果继续使用旧的 `modelSearchData.total`，按钮可能显示旧搜索的“添加全部匹配模型”。本轮将后端 total 与当前 debounced 关键词一致性绑定，避免空输入状态继续引用旧结果。
5. i18n 风险：本轮复用已有 `Add all {{count}} matched model(s)` 等翻译键，没有新增缺失翻译。`bun run i18n:sync` 报告 missingCount 为 0。

### 方案评审

采用“小范围原生对齐”的方案：

1. 将 `getModelSearchModelNames()` 改为消费后端搜索结果集合，精确模型直接提取 `model_name`，不再按关键词二次过滤。
2. 名称规则模型仍优先提取 `matched_models`，并保留无展开结果时的谨慎 fallback。
3. 搜索追加按钮在当前页之外还有未扫描结果时，使用已有文案 `Add all {{count}} matched model(s)`，避免按钮只显示当前页新增数但实际追加更多分页结果。
4. 搜索输入清空或关键词变化时，搜索 total 和待追加状态不再读取旧请求缓存，避免按钮文案与当前输入状态不一致。
5. 渠道编辑模型区顺序对齐 new-api-main 的核心配置流：模型选择、搜索补齐、自定义模型和快捷操作之后先设置分组，再配置模型映射。
6. 不引入新组件库、不重写抽屉框架、不移除 NexusTok 已有权限和安全能力。

### 实施结果

已完成渠道编辑页模型搜索追加修复与 new-api 编辑流对齐：

- 搜索 `gpt-5.6` 时，前端可从模型库搜索结果中识别全部三个模型：`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`。
- 搜索供应商、标签或描述命中的模型时，不再因为模型名不包含关键词而被前端过滤掉。
- 搜索结果存在分页未扫描项时，追加按钮展示“添加全部匹配模型”语义，和实际全分页扫描追加行为一致。
- 追加完成后搜索输入清空，按钮回到禁用的普通 `Add` 状态，不再被旧搜索 total 误导为仍可追加全部匹配模型。
- 模型区配置顺序调整为：模型选择和搜索补齐、自定义模型、快捷操作、分组、模型映射，更贴近 new-api-main 编辑渠道页的核心配置路径。
- 现有权限锁、敏感字段保护、Codex OAuth、模型映射缺失模型提示、上游拉取弹窗和分组表单控制均保留。

### 验证记录

1. 使用 3003 真实服务登录后调用 `GET /api/models/search?keyword=gpt-5.6&p=1&page_size=20`，确认返回 `total=3`，包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`。
2. 使用 3003 真实服务调用 `GET /api/channel/?p=1&page_size=5`，确认当前渠道 `11111` 仍只配置 `gpt-5.6-sol`，说明本轮问题是渠道编辑追加能力，不是模型库同步丢失。
3. `cd web/default && bun test src/features/channels/lib/model-search.test.ts` 通过，覆盖模型草稿解析、搜索候选提取、供应商/标签/描述命中、批量追加计划和旧请求上下文保护。
4. `cd web/default && bunx eslint src/features/channels/lib/model-search.ts src/features/channels/lib/model-search.test.ts src/features/channels/components/drawers/channel-mutate-drawer.tsx` 通过。
5. `cd web/default && bun run typecheck` 通过。
6. `cd web/default && bun run i18n:sync` 通过，报告显示 en/zh/fr/ja/ru/vi 均 `missingCount=0`。
7. `cd web/default && bun run build` 通过，确认前端生产构建无错误。
8. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200；本环境未暴露浏览器 MCP 工具，因此使用 Chrome Headless + DevTools Protocol 在真实 3003 页面完成等价页面操作验证。
9. 在 3003 页面使用账号 `c1cada` 登录，进入 `/channels`，打开渠道 `11111` 的编辑抽屉；初始模型显示 `Selected 3`，包含 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`。
10. 在 “Search model library” 输入 `gpt-5.6` 后，页面显示 `3 matched · 2 new · 1 already selected`，预览新增模型为 `gpt-5.6-terra`、`gpt-5.6-luna`，按钮显示 `Add 2 new model(s)`。
11. 点击追加后未保存表单，草稿模型显示 `Selected 5`，包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三种 `gpt-5.6` 模型；搜索输入清空后 Add 按钮回到禁用状态，不再显示旧的 `Add all 3 matched model(s)`。
12. DevTools Network 捕获到 `/api/models/search?keyword=gpt-5.6&p=1&page_size=50` 与全量扫描 `/api/models/search?keyword=gpt-5.6&p=1&page_size=100` 均返回 HTTP 200；Console 和 Network 未发现错误。
13. 编辑抽屉中的顺序验证为：`Models *` -> `Quick actions` -> `Groups *` -> `Model Mapping`，符合本轮对齐方案。
14. 关闭抽屉后再次调用 `GET /api/channel/?p=1&page_size=5`，确认渠道 `11111` 后端 `models` 仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，没有因页面验证污染真实配置。

## 本轮实施评审：可灵任务扣费 token 计数饱和转换

### 需求分析

继续对照 `/opt/project/new-api-main` 的异步任务计费安全实现后发现，new-api-main 在可灵任务结果解析中会将 `final_unit_deduction` 先 `math.Ceil`，再通过 `common.QuotaFromFloat` 转为整数，避免上游返回异常大扣费积分时在 `int(...)` 转换阶段发生回绕。NexusTok 当前异步任务额度重算链路已经使用 `common.QuotaFromFloatChecked` 记录 `quota_saturation`，但可灵 adaptor 仍在结果解析阶段使用裸 `int(math.Ceil(tokens))`，异常值可能在进入重算前就变成不可审计的 token 数。

本轮目标是补齐这个小而关键的安全边界：保持可灵扣费积分“向上取整”的历史语义，同时复用 NexusTok 已有 quota 饱和转换，防止异常上游值污染异步任务重算。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 可灵任务结果解析 | `relay/channel/task/kling/adaptor.go` | `final_unit_deduction` 从裸 `int(math.Ceil(...))` 改为 `common.QuotaFromFloat(math.Ceil(...))`。 |
| 可灵 adaptor 测试 | `relay/channel/task/kling/adaptor_test.go` | 覆盖正常小数向上取整、超大值饱和到 `common.MaxQuota`、NaN 不产生正 token。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审和验证结果。 |

本轮不修改数据库 schema、任务状态机、任务轮询调度、渠道配置、前端页面、路由、模型倍率、分组倍率或最终扣费公式。

### 风险评估

1. 正常计费语义回归风险：可灵返回 `1.2` 这类小数积分时仍必须按 `ceil` 记为 2 个 token，不能改成四舍五入或向零截断。
2. 异常上游值风险：`1e100`、`+Inf` 等异常值不能在 `int` 转换时回绕为负数或平台相关结果，应饱和到 `common.MaxQuota`，后续额度重算仍会继续使用 checked quota 转换。
3. NaN 风险：NaN 不能产生正 token；`common.QuotaFromFloat` 会兜底为 0，当前 `rounded > 0` 条件会保持不写入 token。
4. 审计范围风险：`ParseTaskResult` 没有 `RelayInfo` 或 `Task` 上下文，无法在该层写入 `quota_saturation`；本轮只保证 token 计数不回绕，最终额度异常仍由后续 `RecalculateTaskQuota` 的 checked 转换记录。

### 方案评审

采用与 new-api-main 等价的最小方案：保留 `math.Ceil(tokens)` 的上游积分取整语义，只把最终整数转换替换为 `common.QuotaFromFloat(...)`。该 helper 已在 NexusTok 的额度安全体系中使用，具备 NaN、上溢和下溢保护，不引入新依赖，也不改变任务成功/失败状态映射。

### 实施结果

已完成可灵任务扣费 token 计数饱和转换：

- 可灵 `final_unit_deduction` 正常小数仍按向上取整写入 `CompletionTokens` 与 `TotalTokens`。
- 超大扣费积分会饱和到 `common.MaxQuota`，避免裸 `int` 转换回绕。
- NaN 或非正结果不会写入正 token，保持任务结果解析安全。

### 验证记录

1. `go test ./relay/channel/task/kling` 通过，覆盖正常小数、超大值和 NaN 三类结果解析。
2. `go test ./relay/channel/task/...` 通过，确认任务 channel 包整体构建和既有测试不受影响。
3. `go test ./service -run 'Task|Quota|Billing'` 通过，确认服务侧任务重算、退款和额度安全相关测试不受影响。
4. `git diff --check` 通过。
5. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200；本轮为后端任务解析安全修复，没有新增前端页面或可直接手动触发的公开交互入口。

## 本轮实施评审：音频时长 token 估算饱和转换

### 需求分析

继续对照 `/opt/project/new-api-main` 的计费边界后发现，new-api-main 在音频 token 估算中已经把 `ceil(duration) / 60 * 1000` 的结果交给 `common.QuotaRound` 转换，并在请求预消费估算里先把负时长钳为 0。NexusTok 当前已经有统一的 quota 饱和转换体系，但音频转写/翻译预消费估算和 OpenAI TTS fallback usage 仍使用裸 `int(math.Round(...))`。

音频时长来自用户上传文件或上游音频元数据，正常情况下数值很小，但异常文件、损坏元数据或上游返回异常值时，裸 `int` 转换可能出现平台相关回绕或负 token。由于这些 token 会进入预扣费、usage 展示和后续额度计算链路，本轮目标是把 new-api-main 的安全边界原生化到 NexusTok：保持原有“每分钟 1000 token、秒数先向上取整”的语义，同时对负值、NaN 和超大值进行统一保护。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 音频时长 token helper | `service/token_counter.go` | 新增 `EstimateAudioDurationTokens()`，统一执行负时长钳制、`ceil(duration) / 60 * 1000` 公式和 `common.QuotaRound` 饱和转换。 |
| 请求 token 预估 | `service/token_counter.go` | 音频转写/翻译模式下的预消费 token 改为调用统一 helper。 |
| OpenAI TTS usage fallback | `relay/channel/openai/audio.go` | 上游未返回 usage 时，按音频时长估算 completion audio token 的路径改为调用统一 helper。 |
| 回归测试 | `service/audio_token_test.go` | 覆盖正常小数秒、负时长、NaN 和超大时长。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审和验证结果。 |

本轮不修改数据库 schema、模型倍率、分组倍率、日志结构、Relay 请求/响应 DTO、音频解码方式、文件上传流程或上游请求协议。

### 风险评估

1. 正常计费语义回归风险：正常音频仍必须保持 `ceil(duration) / 60 * 1000` 后四舍五入的历史公式。例如 `60.1s` 应估算为 `1017` token。
2. 负时长风险：异常元数据返回负数时不能产生负 token，否则会低估预扣费甚至影响余额保护。本轮先将负 duration 钳为 0。
3. 超大时长风险：极端 duration 不能在 `int` 转换阶段回绕，应饱和到 `common.MaxQuota`，后续额度计算仍会沿用已有 quota 安全链路。
4. NaN 风险：NaN 不代表真实使用量，`common.QuotaRound` 会兜底为 0；本轮测试明确锁定该行为。
5. 复用范围风险：helper 放在 service 层，OpenAI relay 包本来已经依赖 service，因此不会引入新的反向依赖或包环。

### 方案评审

采用“抽取统一 helper + 两处替换”的小步方案：

1. 在 `service/token_counter.go` 新增 `EstimateAudioDurationTokens(duration float64) int`。
2. helper 内部先处理 `duration < 0`，再按现有公式 `math.Ceil(duration) / 60.0 * 1000` 计算，最后交给 `common.QuotaRound`。
3. `EstimateRequestToken` 的音频转写/翻译预消费估算改为调用 helper。
4. `OpenaiTTSHandler` 的时长 fallback completion token 估算改为调用 helper。
5. 保留按响应体大小估算 token 的 fallback 路径不变，因为 new-api-main 当前也未改变该路径，本轮不扩大行为面。

### 实施结果

已完成音频时长 token 估算饱和转换：

- 音频转写/翻译预消费估算和 OpenAI TTS fallback usage 使用同一个 helper。
- 正常音频时长仍保持原有分钟计费公式。
- 负时长会返回 0，不再可能生成负 token。
- NaN 返回 0，超大时长饱和到 `common.MaxQuota`，避免裸 `int` 转换回绕。

### 验证记录

1. `go test ./service -run TestEstimateAudioDurationTokens` 通过，覆盖正常小数秒、负时长、NaN 和超大时长。
2. `go test ./relay/channel/openai` 通过，确认 OpenAI audio relay 包复用 helper 后构建和既有测试不受影响。
3. `go test ./service ./relay/channel/openai` 通过，确认 service 和 OpenAI relay 受影响包完整测试不受影响。
4. `git diff --check` 通过。
5. `cd web/default && bunx prettier --check ../../docs/features/new-api-main-diff-analysis.md` 通过。
6. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200；本轮为后端音频 token 估算安全修复，没有新增前端页面或可直接手动触发的公开交互入口。

## 本轮实施评审：异步任务 OtherRatios 配额饱和转换

### 需求分析

继续对照 `/opt/project/new-api-main` 的异步任务提交流程后发现，new-api-main 在 `RelayTaskSubmit` 中会把任务适配器估算出的 `OtherRatios` 先按 float 累乘，再通过 `common.QuotaFromFloatChecked` 写回 `PriceData.Quota`，并把饱和事件记录到 `RelayInfo.QuotaClamp`。提交后 `AdjustBillingOnSubmit` 重新计算 quota 时也使用同一套饱和转换。NexusTok 当前已经具备 `RelayInfo.NoteQuotaClamp`、任务日志 `quota_saturation` 和多条计费饱和保护链路，但异步任务提交中的 `OtherRatios` 仍使用裸 `int(float64(quota) * ratio)`，提交后重算也直接 `int(result)`。

`OtherRatios` 通常来自视频时长、分辨率、动作类型或基于历史任务的 remix/continuation 信息。正常请求下倍率较小，但异常历史数据、适配器解析错误或上游/请求组合导致的极端倍率，可能在预扣费前发生整数回绕，使任务以不可审计的错误 quota 进入后续扣费链路。本轮目标是把 new-api-main 的任务倍率安全边界原生化到 NexusTok：保持历史“向零截断”语义和 `TaskPricePatches` 跳过逻辑，同时让异常倍率进入统一饱和转换与审计记录。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 异步任务提交配额 | `relay/relay_task.go` | `OtherRatios` 应用改为 `applyTaskOtherRatios()`，通过 `common.QuotaFromFloatChecked` 饱和转换并记录 `RelayInfo.QuotaClamp`。 |
| 提交后配额重算 | `relay/relay_task.go` | `recalcQuotaFromRatios()` 从旧倍率恢复基础 quota 后，使用 float 重新应用新倍率，并通过 checked quota 转换返回。 |
| 回归测试 | `relay/relay_task_test.go` | 覆盖正常倍率、超大倍率饱和、`TaskPricePatches` 跳过和提交后重算饱和。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审和验证结果。 |

本轮不修改数据库 schema、任务状态机、任务适配器请求构造、账号池任务限流、预扣费/退款接口、任务轮询调度、前端页面或模型价格配置格式。

### 风险评估

1. 正常倍率回归风险：`OtherRatios` 正常组合仍需保持历史按 float 乘积后向零截断的语义，不能改成四舍五入。
2. `TaskPricePatches` 重复计费风险：补丁模型已经由适配器做专门价格修正，本轮必须保留跳过倍率应用逻辑。
3. 极端倍率风险：超大倍率不能在 `int` 转换时回绕，应饱和到 `common.MaxQuota`，并记录 `QuotaClamp`，后续任务消费日志可通过已有链路写入 `admin_info.quota_saturation`。
4. 提交后调整风险：`AdjustBillingOnSubmit` 可能基于上游返回值调整 duration/size 等倍率，重算时要先从当前 quota 中除掉原有正倍率恢复基础 quota，再应用新倍率。
5. 审计覆盖风险：同一次请求可能多个阶段触发饱和；本轮复用 `RelayInfo.NoteQuotaClamp` 的“首个事件优先”规则，避免后续转换覆盖根因。

### 方案评审

采用“抽取倍率应用 helper + 重算 checked 转换”的小步方案：

1. 新增 `applyTaskOtherRatios(info, modelName)`，集中处理异步任务提交前的 `OtherRatios` 应用。
2. 非 `TaskPricePatches` 模型将 `PriceData.Quota` 转为 float 后累乘所有非 1 倍率，再用 `common.QuotaFromFloatChecked` 转回 int。
3. helper 通过 `info.NoteQuotaClamp(clamp)` 记录首个饱和事件，并把最终 quota 写回 `info.PriceData.Quota`。
4. `recalcQuotaFromRatios()` 改为 float 恢复基础 quota 和重算结果，最终同样使用 checked quota 转换。
5. 不改变 `OtherRatios` 的来源、响应 header、预扣费入口或任务成功后的结算流程。

### 实施结果

已完成异步任务 `OtherRatios` 配额饱和转换：

- 任务提交前的时长、分辨率等倍率应用不再使用裸 `int` 转换。
- 提交后基于上游实际返回值调整倍率时，也会通过统一 quota 饱和转换返回最终 quota。
- 正常倍率仍保持向零截断语义。
- `TaskPricePatches` 中的特殊模型继续跳过倍率应用，避免重复计费。
- 超大倍率会饱和到 `common.MaxQuota` 并记录 `RelayInfo.QuotaClamp`。

### 验证记录

1. `go test ./relay -run 'TestApplyTaskOtherRatios|TestRecalcQuotaFromRatiosUsesSaturatingConversion'` 通过，覆盖正常倍率、超大倍率、补丁跳过和提交后重算。
2. `go test ./relay` 通过，确认 relay 包整体构建和既有测试不受影响。
3. `go test ./service -run 'Task|Quota|Billing'` 通过，确认任务、额度和计费相关服务测试不受影响。
4. `go test ./relay/channel/task/...` 通过，确认异步任务适配器包整体构建和既有测试不受影响。
5. `git diff --check` 通过。
6. `cd web/default && bunx prettier --check ../../docs/features/new-api-main-diff-analysis.md` 通过。
7. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200；本轮为后端任务配额安全修复，没有新增前端页面或可直接手动触发的公开交互入口。

## 本轮实施评审：实时音频 token 计数饱和转换

### 需求分析

继续对照 `/opt/project/new-api-main` 的音频 token 计数实现后发现，new-api-main 在 `CountAudioTokenInput` 和 `CountAudioTokenOutput` 中已经把音频时长换算结果交给 `common.QuotaFromFloat`，避免 duration 异常大时通过裸 `int(...)` 转换产生平台相关回绕。NexusTok 上一轮已经完成音频转写/翻译预消费和 OpenAI TTS fallback 的分钟计费保护，但实时/多模态音频输入输出 token 计数仍然使用裸 `int(duration / ...)`。

这两个函数的 duration 来自用户提供或上游返回的音频元数据，正常情况下数值很小，但异常文件、损坏元数据或解析器异常值仍可能导致 token 计数越界或变成负值。本轮目标是把 new-api-main 的实时音频 token 安全边界原生化到 NexusTok：保持现有输入/输出公式不变，同时对负值、NaN 和超大值使用统一 quota 转换兜底。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 实时音频 token 计数 | `service/token_counter.go` | `CountAudioTokenInput` 和 `CountAudioTokenOutput` 改为调用可测试 helper，转换阶段使用 `common.QuotaFromFloat`。 |
| 音频 token 测试 | `service/audio_token_test.go` | 增加实时音频输入/输出正常公式、负时长、NaN 和超大时长测试。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审和验证结果。 |

本轮不修改音频解析函数、Base64 解码、请求/响应 DTO、模型倍率、分组倍率、日志结构、前端页面或上游协议。

### 风险评估

1. 正常公式回归风险：输入音频仍使用 `duration / 60 * 100 / 0.06`，输出音频仍使用 `duration / 60 * 200 / 0.24`，并保持向零截断语义。
2. 负时长风险：负 duration 不代表真实使用量，不能产生负 token。本轮按 0 处理，避免低估预扣费或生成负 usage。
3. NaN 风险：NaN 不能表示真实 token，`common.QuotaFromFloat` 会兜底为 0。
4. 超大时长风险：极端 duration 不能在裸 `int` 转换时回绕，应饱和到 `common.MaxQuota`。
5. 行为面风险：本轮只替换 duration 到 token 的转换，不改变空音频返回 0、解析失败返回 error 的既有行为。

### 方案评审

采用“可测试 helper + 两处替换”的最小方案：

1. 新增 `estimateRealtimeAudioInputTokens(duration)` 和 `estimateRealtimeAudioOutputTokens(duration)`。
2. helper 内部先将负 duration 钳为 0，再按原公式计算。
3. 使用 `common.QuotaFromFloat` 转为 int，保持历史向零截断语义并获得 NaN/上下溢保护。
4. `CountAudioTokenInput/Output` 只替换最后的 return，不改解析流程。

### 实施结果

已完成实时音频 token 计数饱和转换：

- 输入音频和输出音频 token 公式保持不变。
- 负时长返回 0，不再可能产生负 token。
- NaN 返回 0，超大时长饱和到 `common.MaxQuota`。
- 空音频和解析错误路径保持原样。

### 验证记录

1. `go test ./service -run 'TestEstimateAudioDurationTokens|TestEstimateRealtimeAudioTokens'` 通过，覆盖实时音频正常公式和异常 duration。
2. `go test ./service` 通过，确认 service 包整体构建和既有测试不受影响。
3. `git diff --check` 通过。
4. `cd web/default && bunx prettier --check ../../docs/features/new-api-main-diff-analysis.md` 通过。
5. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200；本轮为后端音频 token 计数安全修复，没有新增前端页面或可直接手动触发的公开交互入口。

## 本轮实施评审：渠道亲和失效清理与 Header 键来源

### 需求分析

继续对照 `/opt/project/new-api-main` 的渠道亲和实现后发现，new-api-main 在两个细节上比 NexusTok 当前实现更稳：

1. `ChannelAffinityKeySource` 支持 `request_header`，管理员可以用租户 ID、会话 ID、客户端 trace ID 等 HTTP Header 建立亲和绑定；NexusTok 当前只支持 `context_int`、`context_string` 和 `gjson`。
2. 当亲和缓存命中的渠道已禁用、已删除或不再支持当前分组/模型时，new-api-main 默认会清理当前命中的亲和缓存，避免后续请求反复命中过期渠道；同时提供 `keep_on_channel_disabled` 开关，让管理员在确实希望等待渠道恢复时保留旧缓存。NexusTok 当前会在不可用时走随机兜底或按 `SkipRetryOnFailure` 返回 403，但没有清理旧缓存，容易让后续请求继续命中同一个坏渠道。

本轮目标是把这两个 new-api 优势原生化到 NexusTok 的渠道亲和能力中：扩展 key source 类型，同时补上失效缓存清理的默认保护，保持现有成功亲和、参数覆盖模板、使用缓存统计和 `SkipRetryOnFailure` 语义不变。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道亲和配置 | `setting/operation_setting/channel_affinity_setting.go` | `ChannelAffinityKeySource.Type` 增加 `request_header` 说明；`ChannelAffinitySetting` 增加 `keep_on_channel_disabled`，默认 false。 |
| 渠道亲和 service | `service/channel_affinity.go` | `extractChannelAffinityValue()` 支持从 HTTP Header 提取亲和值；新增 `ClearCurrentChannelAffinityCache()` 和 `ShouldKeepChannelAffinityOnChannelDisabled()`。 |
| 分发中间件 | `middleware/distributor.go` | 亲和命中但最终不可用时，按配置清理当前缓存；禁用渠道且规则要求跳过重试时，本次请求仍返回 403。 |
| 回归测试 | `service/channel_affinity_template_test.go` | 覆盖 header 键来源、header 规则缓存命中、当前缓存清理、keep 开关。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审和验证结果。 |

本轮不修改数据库 schema、渠道表结构、缓存命名空间、默认亲和规则、路由权限、前端页面、Relay 请求构造或账号池调度。

### 风险评估

1. 兼容性风险：新增 `keep_on_channel_disabled` 默认 false，与 new-api-main 对齐；旧配置缺失该字段时 Go 零值为 false，因此会启用清理失效缓存的默认保护。
2. `SkipRetryOnFailure` 风险：Codex/Claude CLI 这类规则仍需要在亲和渠道禁用时跳过重试。本轮先记录是否需要 403，再按配置清理缓存，最后仍返回原有 403 响应，不改变本次请求语义。
3. Header 来源风险：Header 名称来自管理员配置，只读取请求头，不写入响应或日志明文；日志仍使用已有 `KeyHint/KeyFingerprint`，避免暴露完整亲和值。
4. 缓存键风险：`ClearCurrentChannelAffinityCache()` 只删除本次请求上下文里已匹配的 cache key，不做按规则批量删除，不会误删其他租户/模型/分组的亲和绑定。
5. 兜底选择风险：亲和命中但渠道不可用且未跳过重试时，本次请求继续进入随机选择渠道兜底；清理旧缓存只影响后续请求，避免反复命中过期渠道。

### 方案评审

采用“小范围扩展配置 + 复用上下文 cache key”的方案：

1. `ChannelAffinityKeySource.Type` 支持 `request_header`，通过 `c.GetHeader(src.Key)` 获取并 `TrimSpace`。
2. `ChannelAffinitySetting` 增加 `KeepOnChannelDisabled`，JSON 键为 `keep_on_channel_disabled`。
3. 新增 `ClearCurrentChannelAffinityCache(c)`，从 `getChannelAffinityContext()` 获取当前 cache key，并调用 `HybridCache.DeleteMany()` 删除；清理成功或尝试后会把上下文中的 skip retry 标记设为 false，避免当前上下文继续传播失效语义。
4. 分发中间件在亲和命中但 `affinityUsable=false` 时，如果未开启 keep 开关，则清理当前缓存；若命中的渠道已禁用且规则要求跳过重试，则仍按原逻辑返回 403。
5. 不改变 `RecordChannelAffinity()` 的写入逻辑，不改变成功请求更新亲和绑定的时机。

### 实施结果

已完成渠道亲和失效清理与 Header 键来源原生化：

- 管理员现在可以配置 `request_header` 作为渠道亲和值来源。
- `keep_on_channel_disabled` 默认 false，亲和命中渠道不可用时会清理当前缓存。
- 显式开启 `keep_on_channel_disabled=true` 后会保留旧缓存，适合短暂维护场景。
- `SkipRetryOnFailure` 的本次请求行为保持不变；命中禁用渠道时仍可返回 403，但默认会先清理失效缓存，避免后续请求持续失败。
- 缓存清理只作用于本次请求命中的单个亲和 key，不影响其他亲和绑定。

### 验证记录

1. `go test ./service -run 'ChannelAffinity|ShouldKeepChannelAffinityOnChannelDisabled|ClearCurrentChannelAffinityCache|RequestHeader'` 通过，覆盖 Header 键来源、缓存命中、当前缓存清理和 keep 开关。
2. `go test ./middleware ./service` 通过，确认分发中间件和 service 包整体测试不受影响。
3. `go test ./setting/operation_setting ./middleware ./service` 通过，确认配置包、分发中间件和 service 包整体构建与测试不受影响。
4. `git diff --check` 通过。
5. `cd web/default && bunx prettier --check ../../docs/features/new-api-main-diff-analysis.md` 通过。
6. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200；本轮为后端渠道亲和调度能力修复，没有新增前端页面或可直接手动触发的公开交互入口。

## 本轮实施评审：渠道编辑页模型搜索供应商收窄与页面对齐

### 需求分析

用户继续反馈“搜索添加时不正确”，并指出最新版 `new-api-main` 的编辑渠道页面体验更好，希望 NexusTok 的编辑渠道页面与其对齐。结合前一轮 `gpt-5.6` 三模型同步问题继续核对后发现，NexusTok 当前已经能搜索到 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个 OpenAI 模型，但编辑渠道页的模型库搜索默认是全局搜索：它会按关键词在模型名称、描述和标签中检索所有供应商模型。对于 OpenAI/Codex/Sora 等明确归属 OpenAI 供应商的渠道，搜索补齐应优先限定在 OpenAI 模型库，否则搜索供应商、标签或描述时可能把其它 provider 的兼容路由模型也追加到渠道能力里。

同时，真实接口验证发现 `/api/models/search?keyword=...&vendor=OpenAI` 在 PostgreSQL 下会报 `column reference "description" is ambiguous`。原因是搜索带供应商过滤时会 JOIN `vendors` 表，而 `models` 和 `vendors` 都有 `description` 字段，模型搜索条件没有显式限定 `models.description`。因此本轮必须同时修复后端搜索接口和前端编辑页搜索语义，才能把“根据当前渠道供应商添加模型”发展成稳定的原生能力。

最新版 `new-api-main` 的编辑渠道页面在 `Models & Groups` 区域采用更清晰的工作流：先配置模型，再配置模型映射，最后配置分组。NexusTok 在此基础上已经新增了模型库搜索、自定义批量添加、权限控制和账号池模式等原生增强。本轮页面对齐的目标不是回退这些增强，而是吸收 new-api-main 的配置顺序和清晰分区，让管理员能明确看到搜索范围、预览结果和最终追加行为。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 模型搜索接口 | `model/model_meta.go` | `SearchModels()` 中关键词条件显式限定 `models.model_name`、`models.description`、`models.tags`，修复带 vendor JOIN 时的跨库歧义。 |
| 后端测试 | `model/model_meta_search_test.go` | 新增 SQLite 内存库测试，覆盖关键词 + vendor 搜索只返回对应供应商模型。 |
| 渠道模型搜索纯函数 | `web/default/src/features/channels/lib/model-search.ts` | 新增渠道类型到默认模型供应商的映射，并让异步追加上下文校验包含 vendor。 |
| 前端测试 | `web/default/src/features/channels/lib/model-search.test.ts` | 覆盖 OpenAI/Codex 默认 OpenAI、Anthropic/Gemini/DeepSeek 等明确供应商映射，以及 vendor 变化时拒绝旧搜索结果。 |
| 编辑渠道抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 模型库预览搜索和“添加全部匹配”全量分页扫描使用同一 vendor 参数；页面显示当前搜索范围；`Models & Groups` 区域顺序调整为 `Models -> Model Mapping -> Groups`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审和验证结果。 |

本轮不修改数据库 schema、模型同步逻辑、渠道保存 payload、渠道能力表结构、Relay 调度逻辑、账号池能力或模型价格配置。自定义渠道、Advanced Custom、OpenAI-compatible 聚合网关等无法明确供应商的渠道继续使用全局模型库搜索。

### 风险评估

1. 供应商映射过窄风险：Custom、Advanced Custom、OpenRouter、AI Proxy、Submodel 等渠道可能代理多个供应商，如果强行按渠道名称过滤会漏掉真实可用模型。本轮只对 OpenAI、Codex、Sora、Azure、Anthropic、Gemini/Vertex、DeepSeek、Mistral、Cohere、MiniMax、Perplexity、xAI、Replicate 等可明确归属的渠道启用默认 vendor 过滤，其它渠道保持全局搜索。
2. 预览和最终追加不一致风险：如果预览搜索和点击追加时的分页扫描参数不同，管理员看到的 “new/already selected” 统计可能与最终写入不一致。本轮把 `vendor` 同时放进 query key、预览请求、全量分页扫描和上下文校验。
3. 旧搜索结果污染风险：管理员切换渠道、切换渠道类型或修改关键词后，旧异步搜索结果不应继续写入当前表单。本轮在原有 `open/channelId/keyword` 校验基础上增加 `vendor` 校验，供应商范围变化会拒绝旧结果。
4. PostgreSQL 兼容风险：后端修复必须使用 GORM 条件和表名前缀，不能引入数据库专用 SQL。本轮只把列名限定到 `models.*`，兼容 SQLite、MySQL 和 PostgreSQL。
5. 页面回归风险：`Models & Groups` 区域重排不能影响已有权限禁用、复制模型、自定义模型、上游拉取、预设分组、模型映射守卫和分组必填校验。本轮只调整展示顺序和搜索参数，不改变保存转换函数。

### 方案评审

采用“后端搜索修复 + 前端供应商收窄 + 页面顺序对齐”的小步方案：

1. 修复 `SearchModels()` 的歧义列条件，使带 `vendor` 过滤的搜索接口在 PostgreSQL 下可用。
2. 新增 `getModelSearchVendorForChannelType()`，作为渠道编辑页模型搜索的默认供应商推导函数。
3. `useQuery` 的模型库预览搜索 key 加入 `modelSearchVendor`，请求参数带上 `vendor`；点击“添加”时的 `fetchAllModelSearchModelNames()` 同样带 `vendor`。
4. `isModelSearchAppendContextCurrent()` 把 vendor 纳入上下文一致性判断，避免渠道类型变化后误用旧搜索结果。
5. 页面上在“Search model library”旁显示当前搜索范围，例如 `Vendor: OpenAI` 或 `All Vendors`。
6. `Models & Groups` 区域按 new-api-main 最新源码调整为 `Models -> Model Mapping -> Groups`，保留 NexusTok 的模型库补齐、自定义模型和快捷操作增强。

### 实施结果

已完成渠道编辑页模型搜索供应商收窄与页面对齐：

- OpenAI/Codex/Sora 渠道的模型库搜索默认限定到 OpenAI 供应商，`gpt-5.6` 系列搜索和批量追加会围绕 OpenAI 模型库执行。
- 明确供应商渠道会按对应供应商收窄搜索；无法明确供应商的渠道继续全局搜索，避免漏掉聚合网关和自定义路由模型。
- 后端 `/api/models/search` 带 vendor 参数时不再因 `description` 歧义报错。
- 模型库搜索结果预览、按钮可用状态和最终全量追加使用同一套 keyword + vendor 规则。
- 编辑渠道页 `Models & Groups` 区域顺序与 new-api-main 最新页面对齐为 `Models -> Model Mapping -> Groups`，并显示当前搜索范围。

### 验证记录

1. `go test ./model -run 'TestSearchModelsWithVendorQualifiesModelColumns'` 通过，确认关键词 + vendor 搜索不会因跨表 `description` 歧义失败，且只返回对应供应商模型。
2. `cd web/default && bun test src/features/channels/lib/model-search.test.ts src/components/multi-select.test.ts` 通过，共 36 个测试，覆盖模型搜索追加、供应商映射、上下文校验和 MultiSelect 搜索行为。
3. `go test ./model` 通过，确认模型元数据、渠道能力归属、任务模型等 model 包既有测试不受影响。
4. `cd web/default && bun run typecheck` 通过，确认编辑渠道抽屉和模型搜索工具函数的 TypeScript 类型无回归。
5. `git diff --check` 通过，确认本轮修改没有空白错误。
6. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200，并通过真实接口调用 `GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=20`，返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个 OpenAI 供应商模型，不再出现 PostgreSQL `description` 歧义错误。
7. 由于当前环境没有浏览器 MCP 工具，本轮使用本机 Chrome Headless + DevTools Protocol 作为替代真实页面验证：登录 `c1cada` 后打开 `http://192.168.0.202:3003/channels`，进入渠道 `11111` 的编辑抽屉，确认页面显示 `Search model library`、`Vendor: OpenAI`，且没有控制台错误。
8. 在同一编辑抽屉里输入 `gpt-5.6`，页面显示 `3 matched · 2 new · 1 already selected` 和待追加预览 `gpt-5.6-terra, gpt-5.6-luna`；点击 `Add 2 new model(s)` 后，未保存的表单草稿显示 `Selected 5`，包含 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`，验证搜索添加逻辑已按 OpenAI 供应商正确补齐。

## 本轮实施评审：渠道状态专用 API 与通用编辑解耦

### 需求分析

继续对照 `/opt/project/new-api-main` 的渠道管理接口后发现，new-api-main 已经把渠道启用/禁用拆成 `POST /api/channel/:id/status` 和 `POST /api/channel/status/batch`，并且通用 `UpdateChannel` 会拒绝请求体中的 `status` 字段。这样做的核心优势是把“运行期操作”从“配置编辑”中解耦：启停渠道只需要操作权限，不应走完整渠道写权限，也不应和密钥、BaseURL、模型、分组等配置字段混在同一个 payload 中。

NexusTok 当前已经有 `model.UpdateChannelStatus()`，并且该函数会同步渠道状态、能力表启用状态、缓存和多 Key 状态；缺口主要在 controller/router/frontend 调用层：前端单个启停与批量启停仍通过 `PUT /api/channel/` 携带 `status`。本轮目标是把 new-api-main 的状态操作边界原生化到 NexusTok，同时保留现有模型层能力、审计日志、权限体系和页面交互。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道控制器 | `controller/channel.go` | 新增单个/批量状态请求结构和 handler；通用 `UpdateChannel` 提前拒绝 `status` 字段；专用接口只允许手动启用与手动禁用。 |
| 字段权限分类 | `controller/channel_authz.go` | 更新 `status` 字段说明：该字段已迁移到专用操作入口，通用编辑接口不再接受它。 |
| 路由权限表 | `router/channel-router.go` | 新增 `/api/channel/:id/status` 与 `/api/channel/status/batch`，权限均为 `channel.operate`。 |
| 前端 API 与动作 | `web/default/src/features/channels/api.ts`、`web/default/src/features/channels/lib/channel-actions.ts` | 新增 `updateChannelStatus()` 和 `batchUpdateChannelStatus()`；行内启停和批量启停改走专用接口。 |
| 回归测试 | `controller/channel_authz_test.go`、`router/channel_router_test.go` | 覆盖通用编辑拒绝 `status`、单个状态接口同步能力表、自动禁用状态拒绝、批量状态实际变更计数和路由权限。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审和验证结果，并更新实施索引。 |

本轮不修改数据库 schema、渠道表结构、能力表结构、调度策略、标签启停接口、账号池状态接口、多 Key 管理接口或渠道编辑表单保存 payload。

### 风险评估

1. 旧客户端兼容风险：如果旧前端或脚本继续通过 `PUT /api/channel/` 携带 `status`，现在会得到 `success=false` 的参数错误。这是有意收紧，用于避免状态操作绕过 `channel.operate` 边界。
2. 自动禁用语义风险：`ChannelStatusAutoDisabled` 表示系统按错误率或运行时治理自动熔断，管理端不能伪造该状态。本轮只允许 `ChannelStatusEnabled` 和 `ChannelStatusManuallyDisabled`。
3. 能力表同步风险：状态变化必须同步 `abilities.enabled`，否则 `/v1/models`、调度和 owner 展示会不一致。本轮复用 `model.UpdateChannelStatus()`，继续使用既有能力同步逻辑。
4. 批量操作统计风险：批量接口返回的是实际发生变化的数量，已经处于目标状态的渠道不会重复计数；前端 toast 使用该数量，避免误报。
5. 运行态验证风险：页面点击启停会短暂改变真实渠道状态。本轮验证先记录原始状态，点击后立即通过专用接口恢复，并最终通过接口和页面刷新确认渠道仍为启用。

### 方案评审

采用“薄 controller + 复用模型层 + 前端调用切换”的低风险方案：

1. 在 `controller/channel.go` 中新增 `ChannelStatusRequest`、`ChannelStatusBatchRequest`、`UpdateChannelStatus()`、`BatchUpdateChannelStatus()` 和 `isManageableChannelStatus()`。
2. `UpdateChannel()` 读取原始 JSON 后先检查 `status` 字段，发现即返回参数错误，避免后续字段级权限分类误放行。
3. 两个专用接口均调用 `model.UpdateChannelStatus(id, "", status, "...")`，只有实际发生变化时刷新渠道缓存和代理客户端缓存。
4. 路由表将两个状态接口绑定到 `authz.ChannelOperate`，其中 `/status/batch` 放在 `/:id/status` 前，避免动态路径影响静态批量路径。
5. 默认前端新增专用 API wrapper，并让行内启停和批量启停走新接口；标签启停继续使用已有 tag 专用接口。

### 实施结果

已完成渠道状态专用 API 与通用编辑解耦：

- `POST /api/channel/:id/status` 支持单个渠道手动启用/禁用，返回 `data=true/false` 表示是否实际变更。
- `POST /api/channel/status/batch` 支持批量手动启用/禁用，返回实际变更数量。
- `PUT /api/channel/` 请求体包含 `status` 时返回参数错误，不再允许通过配置编辑接口改运行状态。
- 自动禁用状态只能由系统运行时流程产生，管理端专用状态接口会拒绝 `status=3`。
- 前端行内启停和批量启停已改走专用接口，页面权限语义继续使用 `channel.operate`。

### 验证记录

1. `go test ./controller -run 'TestUpdateChannelRejectsStatusField|TestUpdateChannelStatus|TestBatchUpdateChannelStatus|TestChannelHasSensitiveChanges|TestChannelFieldsAreClassified'` 通过，覆盖通用编辑拒绝、状态接口、批量接口和字段分类。
2. `go test ./router -run 'TestRegisterChannelRoutesKeepsCoreHandlers|TestChannelPermissionRoutesClassifyCoreActions'` 通过，确认路由 handler 与 `ChannelOperate` 权限注册正确。
3. `go test ./controller ./router` 通过，确认 controller/router 包既有测试不受影响。
4. `cd web/default && bun run typecheck` 通过，确认默认前端 API 和启停动作类型无回归。
5. `cd web/default && bunx prettier --check src/features/channels/api.ts src/features/channels/lib/channel-actions.ts` 通过。
6. `git diff --check` 通过。
7. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200；登录账号 `c1cada` 后调用 `POST /api/channel/1/status` 且 payload 为 `{"status":1}` 返回 `success=true,data=false`，确认专用接口已生效且同状态 no-op 不改数据。
8. 同一运行态调用 `PUT /api/channel/` 且 payload 为 `{"id":1,"status":1}` 返回 `success=false,message="无效的参数"`；调用 `POST /api/channel/1/status` 且 payload 为 `{"status":3}` 同样返回参数错误，确认通用编辑和自动禁用伪造均被拒绝。
9. 当前环境没有暴露浏览器 MCP 工具，本轮使用 Chrome Headless + DevTools Protocol 进行真实页面验证：通过登录表单进入 `http://192.168.0.202:3003/channels`，点击渠道 `11111` 行内“禁用”按钮，网络捕获到 `POST /api/channel/1/status`，payload 为 `{"status":2}`，没有出现旧的 `PUT /api/channel/` 状态更新请求。
10. 页面验证后立即调用 `POST /api/channel/1/status` 恢复原始 `status=1`；随后通过 `GET /api/channel/1` 和刷新 `/channels` 页面确认渠道 `11111` 最终仍为 `已启用`，没有留下错误启停状态。

## 本轮实施评审：订阅套餐启停操作权限原生化

### 需求分析

继续对照 `/opt/project/new-api-main` 的订阅套餐管理实现后确认，new-api-main 已将套餐启停拆成独立 `PATCH /api/subscription/admin/plans/:id` 状态接口，请求体只包含 `enabled`。NexusTok 当前后端已经有同形接口，前端也已经通过 `patchPlanStatus()` 调用该接口；缺口不在接口路径，而在 NexusTok 原生 Authz 语义：当前路由权限和默认前端都把启停归到 `subscription.write`，和编辑套餐名称、价格、额度、有效期、支付产品 ID 等配置字段混在一起。

套餐启停属于运行期运营动作：它只控制套餐是否对用户展示，不修改套餐价格、权益和第三方支付绑定。上一轮已经把渠道启停从通用编辑接口中拆出并归到 `channel.operate`，本轮沿用同一治理原则，将套餐启停发展为 `subscription.operate` 能力，让“编辑套餐内容”和“运营启停套餐”在权限模型上可独立授权。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 订阅管理路由权限表 | `router/subscription-router.go` | `PATCH /api/subscription/admin/plans/:id` 的权限从 `authz.SubscriptionWrite` 调整为 `authz.SubscriptionOperate`。 |
| 路由回归测试 | `router/subscription_router_test.go` | 更新套餐启停路由的权限断言，确保后续不会误回退到 write。 |
| 订阅套餐行操作 | `web/default/src/features/subscriptions/components/data-table-row-actions.tsx` | 启停菜单项从 `canWritePlan` 改为 `canOperatePlan`，点击守卫从 `guardWrite()` 改为 `guardOperate()`。 |
| 启停确认弹窗 | `web/default/src/features/subscriptions/components/dialogs/toggle-status-dialog.tsx` | 确认前和确认按钮禁用态改用 `permissions.canOperate`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审和验证结果，并更新实施索引。 |

本轮不修改数据库 schema、套餐表结构、套餐创建/编辑 payload、订阅购买流程、支付回调、余额支付、钱包溢出、订阅到期分组降级、用户订阅重置或用户订阅失效逻辑。

### 风险评估

1. 权限收紧风险：只拥有 `subscription.write` 但没有 `subscription.operate` 的管理员将不能启停套餐。这是本轮的预期边界修正；编辑套餐内容仍由 `subscription.write` 控制，Root 和默认 Admin 基线仍具备 operate 能力。
2. 页面入口不一致风险：如果行操作和确认弹窗只改一处，会出现按钮可点但确认失败，或按钮禁用但接口可用的割裂。本轮同步修改入口和确认两个位置。
3. 接口兼容风险：接口路径、HTTP method、请求体和响应体均不变，现有自动化脚本只要具备 operate 权限即可继续调用。
4. 运行数据风险：3003 当前没有订阅套餐；验证时不能为了制造测试条件创建真实套餐，避免污染业务数据。本轮使用参数校验、页面渲染和路由测试证明行为，并明确记录空数据约束。
5. Authz catalog 风险：本轮没有新增资源或动作，不需要迁移角色 catalog；只是让已有订阅套餐启停路由消费现有 `subscription.operate` 动作。

### 方案评审

采用“权限归类修正 + 前端守卫同步”的最小方案：

1. 保留 new-api-main 已对齐的 `PATCH /api/subscription/admin/plans/:id` 接口形态，不增加新接口。
2. 将该路由从 `SubscriptionWrite` 调整为 `SubscriptionOperate`，与绑定订阅、创建用户订阅、重置订阅额度和失效用户订阅保持同一操作语义。
3. 默认前端套餐行操作和确认弹窗都改用 `canOperate`，避免按钮状态与后端权限不一致。
4. 保留套餐创建和完整编辑的 `canWrite` 语义，不扩大 write 权限的操作面。
5. 使用路由权限测试锁定后端分类，用 TypeScript typecheck 和真实 3003 页面验证确认前端无回归。

### 实施结果

已完成订阅套餐启停操作权限原生化：

- `PATCH /api/subscription/admin/plans/:id` 现在归属于 `subscription.operate`。
- `POST /api/subscription/admin/plans` 和 `PUT /api/subscription/admin/plans/:id` 仍归属于 `subscription.write`，套餐内容编辑边界不变。
- 订阅套餐行操作中的启用/禁用入口改用 `canOperatePlan`。
- 启停确认弹窗的确认守卫和禁用态改用 `permissions.canOperate`。
- new-api-main 的独立状态接口优势已经和 NexusTok 的细粒度 Authz 能力结合：接口形态保持兼容，权限表达更精确。

### 验证记录

1. `go test ./router -run 'TestRegisterSubscriptionAdminRoutesKeepsCoreHandlers|TestSubscriptionPermissionRoutesClassifyCoreActions|TestSubscriptionPermissionRoutesStayOnSubscriptionResource'` 通过，确认订阅套餐启停路由 handler 保持不变，权限断言已更新为 `SubscriptionOperate`。
2. `go test ./controller -run 'TestSubscriptionPlanGroupValidation'` 通过，确认订阅 controller 既有套餐分组校验测试不受影响。
3. `cd web/default && bun run typecheck` 通过，确认默认前端订阅套餐行操作和启停确认弹窗类型无回归。
4. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200。
5. 使用账号 `c1cada` 登录 3003 后调用 `GET /api/subscription/admin/plans` 返回 `success=true` 且当前套餐数为 `0`；因此本轮没有创建临时套餐，也没有执行会污染业务数据的启停切换。
6. 同一登录态调用 `PATCH /api/subscription/admin/plans/1` 且 payload 为 `{}` 返回 `success=false,message="参数错误"`；调用 `PATCH /api/subscription/admin/plans/0` 且 payload 为 `{"enabled":true}` 返回 `success=false,message="无效的ID"`，确认状态接口仍执行参数校验。
7. 当前环境没有暴露浏览器 MCP 工具，本轮使用 Chrome Headless + DevTools Protocol 进行真实页面验证：写入登录态后打开 `http://192.168.0.202:3003/subscriptions`，页面渲染 `Subscription Management`、`Create Plan` 和空状态 `No subscription plans yet`，接口探针 `/api/subscription/admin/plans` 返回 `success=true,count=0`。
8. 同一页面验证中没有捕获到 JavaScript exception 或 Network loadingFailed；Console 仅有 i18next/Locize 的信息级提示。

## 本轮实施评审：Advanced Custom 请求路径感知选路

### 需求分析

继续对照 `/opt/project/new-api-main` 的 Advanced Custom 运行链路后确认，new-api-main 不只提供 type 58 adaptor 和 route 配置，还在渠道选择阶段按当前请求路径过滤 Advanced Custom 候选。该能力解决的是运行正确性问题：同一模型可能同时配置在多个 Advanced Custom 渠道上，但某个渠道只支持 `/v1/chat/completions`，另一个只支持 `/v1/responses` 或 Gemini native path。如果分发层只看 group/model ability，就可能把 `/v1/responses` 请求选到只配置 chat route 的渠道，最终由 adaptor 返回 route 不匹配错误，甚至触发错误重试和亲和性污染。

NexusTok 当前已经完成 Advanced Custom 的后端 schema、严格校验、relay adaptor 和默认前端编辑器，`dto.AdvancedCustomConfig` 也已有 `SupportsPath(requestPath)`。缺口集中在分发和选路：亲和性复用、随机选路、relay 重试和非内存缓存 DB fallback 都没有携带 request path，因此无法在候选阶段排除 path 不匹配的 type 58 渠道。本轮目标是把 new-api-main 的 path-aware routing 转成 NexusTok 原生能力，并保持普通渠道原行为不变。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 分发中间件 | `middleware/distributor.go` | 亲和性复用前增加 Advanced Custom path 支持判断；随机选路参数携带 `c.Request.URL.Path`。 |
| Relay 重试 | `controller/relay.go` | 标准 relay 和 task relay 的 `RetryParam` 携带请求路径，重试选择渠道时继续过滤 path 不匹配的 type 58 候选。 |
| 选路服务参数 | `service/channel_select.go` | `RetryParam` 新增 `RequestPath`，auto 分组和普通分组选路都向 model 层传递该值。 |
| 内存缓存选路 | `model/channel_cache.go` | 缓存 type 58 解析后的 `AdvancedCustomConfig`，并在候选进入优先级/权重计算前按 path 过滤。 |
| DB fallback 选路 | `model/ability.go` | 非内存缓存路径查询候选渠道后，仅对 Advanced Custom 候选按 `SupportsPath` 过滤。 |
| 回归测试 | `model/channel_path_select_test.go` | 覆盖内存缓存过滤、空 path 兼容和 SQLite DB fallback path 过滤。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、实施结果和验证记录，并更新实施索引。 |

本轮不修改数据库 schema、渠道保存 payload、`settings.advanced_custom` JSON 结构、relay adaptor、协议转换器、计费逻辑、模型映射、账号池、渠道状态或前端编辑器。

### 风险评估

1. 误过滤风险：普通渠道不能因为 request path 被过滤；本轮只检查 `ChannelTypeAdvancedCustom`，OpenAI、Codex、Custom、账号池等既有渠道仍沿用原 group/model/status 选路。
2. 空路径兼容风险：后台维护、测试或非 relay 调用可能没有 request path；本轮在 requestPath 为空时跳过过滤，保持旧行为。
3. 配置解析风险：内存缓存路径在 `InitChannelCache()` 时缓存 type 58 的 parsed config，避免热路径重复解析 JSON；DB fallback 才按候选渠道临时读取配置。
4. 亲和性污染风险：如果已记录的 Advanced Custom 亲和性渠道不支持当前 path，本轮会跳过复用并清理当前亲和性缓存，让随机兜底重新选择可用渠道。
5. 无可用渠道风险：当同一 group/model 下只有 path 不匹配的 type 58 渠道时，本轮会返回无可用渠道。这是预期 fail-closed 行为，比把请求转发到错误 route 更安全。
6. DB fallback 一致性风险：SQLite/MySQL/PostgreSQL 都继续使用 GORM 查询，不新增 raw SQL；测试覆盖 SQLite 内存库，列名保留字仍沿用已有 `commonGroupCol` 初始化。

### 方案评审

采用“候选阶段过滤，不碰转发阶段”的方案：

1. `service.RetryParam` 增加 `RequestPath`，由 `middleware/distributor.go` 初选和 `controller/relay.go` 重试构造时填入 `c.Request.URL.Path`。
2. 亲和性复用前调用 `channelSupportsRequestPath()`：非 Advanced Custom 直接放行；type 58 必须存在 `settings.advanced_custom` 且 `SupportsPath(requestPath)` 返回 true。
3. 内存缓存路径新增 `channel2advancedCustomConfig`，随 `InitChannelCache()` 全量刷新；`GetRandomSatisfiedChannel()` 先按 path 过滤 exact model 候选，再尝试 normalized model 候选，然后才进入优先级和权重计算。
4. DB fallback 路径在 `GetChannel()` 查询 ability 候选后，通过一次 `id IN ?` 读取候选渠道，仅对 type 58 做 path 支持判断；非 type 58 ability 全部保留。
5. 新增 model 层测试，固定 “/v1/responses 只选 responses route 渠道”、“普通渠道不被 path 过滤”、“空 requestPath 兼容旧行为” 三类语义。

### 实施结果

已完成 Advanced Custom 请求路径感知选路：

- 初次分发和 relay/task relay 重试都会携带 `RequestPath`。
- 渠道亲和性不会再复用 path 不匹配的 Advanced Custom 渠道。
- 内存缓存模式会缓存 type 58 的 `AdvancedCustomConfig`，并在优先级/权重选择前过滤候选。
- 非内存缓存 DB fallback 也会按 request path 过滤 Advanced Custom ability。
- 普通渠道、空 requestPath、模型分组、权重、优先级、账号池和 relay adaptor 行为保持不变。

该能力把 new-api-main 的 type 58 path-aware routing 转为 NexusTok 原生调度能力，并与 NexusTok 已有的 `settings.advanced_custom` 严格校验、route matcher、账号池和细粒度权限体系保持解耦。

### 验证记录

1. `go test ./model -run 'Test(FilterChannelsByRequestPath|GetChannelFiltersAdvancedCustomByRequestPath|ChannelValidateSettings|AdvancedCustom|SearchModelsWithVendor)'` 通过，覆盖 Advanced Custom path 过滤、空 path 兼容、SQLite DB fallback、原有 Advanced Custom 配置校验和模型搜索回归。
2. `go test ./service -run 'Test.*Channel|Test.*Retry'` 通过，确认 service 层现有渠道选择/重试相关测试不受 `RequestPath` 新字段影响。
3. `go test ./middleware ./controller ./model ./service` 通过，确认分发中间件、relay controller、model 和 service 包整体回归正常。
4. `go test ./model -run 'Test(FilterChannelsByRequestPath|GetChannelFiltersAdvancedCustomByRequestPath)' -count=1` 通过，单独固定本轮新增 path-aware 选路测试。
5. `git diff --check` 通过。
6. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200；访问 `http://192.168.0.202:3003/api/status` 返回正常状态 JSON，确认 3003 热更新运行态仍可访问。

## 本轮实施评审：渠道复制结构化审计补齐

### 需求分析

继续对照 `/opt/project/new-api-main` 的渠道复制 handler 与 Usage Logs 展示链路后确认，new-api-main 在复制渠道成功后会手动写入 `channel.copy` 管理审计，并包含 `sourceId`、新渠道 `id` 和新渠道 `name`。NexusTok 当前默认前端 `web/default/src/features/usage-logs/lib/format.ts` 已经具备 `AUDIT_TEMPLATES` 和 `channel.copy` 本地化模板，管理审计中间件也能对 `POST /api/channel/copy/:id` 兜底记录 action；但兜底审计出于安全边界不会读取请求体或业务结果，因此缺少新渠道 ID、名称和来源渠道 ID。

运行态效果是：Usage Logs 有能力渲染 `Copied channel (source ID: ...) to ...`，但后端没有提供这组业务参数时只能回退到 route/action 级审计，无法达到 new-api-main 的结构化展示完整度。本轮目标是补齐复制成功路径的业务审计参数，同时检查复制接口自身是否能返回真实新渠道 ID。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道复制 handler | `controller/channel.go` | 复制成功后调用 `recordManageAudit(c, "channel.copy", ...)`，写入 `sourceId/id/name`。 |
| 审计兜底模板 | `controller/audit.go` | 增加 `channel.copy` 英文兜底模板，供导出、旧前端或结构化渲染失败时使用。 |
| 渠道模型层 | `model/channel.go` | `Channel.Insert()` 改为事务化，保证渠道主记录和 ability 同进同退，同时保留新 ID 回填。 |
| 回归测试 | `controller/channel_copy_audit_test.go` | 覆盖复制响应 ID、复制后 ability、新渠道关键字段、单条管理日志和 `other.op.params`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新实施索引，并记录本轮需求、影响、风险、方案、实施结果和验证记录。 |

本轮不修改数据库 schema、渠道保存/编辑接口、权限路由、Usage Logs 默认前端模板、日志表结构、审计中间件的兜底策略或渠道密钥脱敏规则。

### 风险评估

1. 重复审计风险：复制 handler 手动写日志后，审计中间件如果继续兜底会产生两条管理日志。本轮复用 `recordManageAudit()`，它会设置 `ContextKeyAuditLogged`，中间件将自动跳过兜底。
2. 敏感信息泄露风险：复制渠道会保留密钥、BaseURL、settings 和账号池配置，但审计参数只记录 `sourceId`、新 `id` 和 `name`，不记录任何 key、header、auth file、settings 或 upstream URL。
3. 新 ID 准确性风险：原实现使用 `BatchInsertChannels([]model.Channel{clone})`，调用方的 `clone.Id` 不一定能拿到 GORM 回填的新主键。本轮改为单条 `clone.Insert()`，让响应、审计和 ability 都基于真实新渠道 ID。
4. 能力表一致性风险：如果单条 `Insert()` 先写渠道再写 ability，ability 失败会留下孤立渠道。本轮同步将 `Channel.Insert()` 改为事务化，保持渠道主记录和 ability 同进同退。
5. 运行态数据风险：3003 页面验证不执行真实复制，避免创建额外渠道污染当前环境；本轮通过 SQLite controller 测试证明成功路径。

### 方案评审

采用“成功路径手动审计 + 不动前端模板”的最小方案：

1. 保留复制接口路径、query 参数、响应结构和权限边界。
2. 将复制插入从批量插入改为 `clone.Insert()`，确保新渠道 ID 立即回填。
3. 插入成功并刷新缓存后调用 `recordManageAudit()`，参数只包含可公开的资源标识。
4. `Channel.Insert()` 内部使用事务包裹 `Create(channel)` 和 `AddAbilities(tx)`，避免为了 ID 回填牺牲原批量插入路径的原子性。
5. `controller/audit.go` 增加 `channel.copy` fallback content 模板；默认前端已有 `channel.copy` template，无需新增 i18n key。
6. 新增 controller 测试，固定复制 ID、ability 和审计 JSON，避免后续退回到只有兜底 action 的审计。

### 实施结果

已完成渠道复制结构化审计补齐：

- `POST /api/channel/copy/:id` 成功后会记录 `channel.copy` 管理审计。
- 审计 `other.op.params` 包含来源渠道 `sourceId`、新渠道 `id` 和新渠道 `name`。
- 中间件不会为同一次复制再写兜底日志，避免重复。
- 复制响应中的 `data.id` 现在是实际新渠道 ID，而不是潜在的零值。
- 复制后的 ability 仍通过 `Insert()` 的 `AddAbilities()` 创建，并与渠道主记录处于同一事务。
- Usage Logs 默认前端已有 `channel.copy` 结构化渲染模板，本轮后端参数补齐后即可展示完整复制内容。

### 验证记录

1. `go test ./controller -run 'TestCopyChannelWritesStructuredAuditWithNewChannelID'` 通过，覆盖新渠道 ID 回填、密钥保留、余额重置、ability 创建和 `channel.copy` 审计参数。
2. `go test ./controller ./middleware ./model` 通过，确认 controller 成功路径、管理审计中间件和事务化 `Channel.Insert()` 模型层回归正常。
3. `cd web/default && bunx prettier --check ../../docs/features/new-api-main-diff-analysis.md` 通过，确认差异报告格式正常。
4. `git diff --check` 通过。
5. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200；本轮没有在运行态执行真实渠道复制，避免创建额外渠道污染当前环境。

## 本轮实施评审：渠道编辑页模型搜索内聚与 new-api 页面体验对齐

### 需求分析

用户继续反馈“搜索添加时不正确”，并指出最新版 `/opt/project/new-api-main` 的编辑渠道页面体验更好，希望 NexusTok 的编辑渠道页面与其对齐。重新对照 new-api-main 的 `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 后确认，其 `Models & Groups` 区域以模型多选器作为唯一主入口，配置顺序清晰：模型列表、模型映射、分组。NexusTok 前一轮为了补齐同步模型库搜索能力，额外增加了独立的 `Search model library` 输入框和独立的自定义模型输入框，导致模型选择、搜索追加、自定义添加三套入口并存。

真实 3003 接口验证显示，`GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=20` 已经返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个 OpenAI 模型；问题不在同步数据，也不在后端供应商过滤，而在页面交互入口：管理员在多选器里搜索 `gpt-5.6` 时，只能看到当前渠道/能力已有的模型候选，容易只选到 `gpt-5.6-sol`，或者在没有正确候选时把 `gpt-5.6` 当成一个错误的自定义模型。目标是吸收 new-api-main 的单入口编辑体验，同时保留 NexusTok 已有的“从模型元数据搜索补齐尚未加入渠道能力的模型”优势。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 模型多选器接管同步模型库搜索；移除独立 `Search model library` 和独立自定义模型输入框；候选集合继续合并当前渠道模型、已启用模型和后端搜索结果。 |
| 多选组件 | `web/default/src/components/multi-select.tsx` | 新增 `onSearchSubmit` 可选回调，保留回车触发“添加全部匹配模型”的键盘习惯；高亮候选和自定义创建仍优先走原有行为。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审、实施结果和验证记录。 |

本轮不修改后端模型搜索接口、模型同步逻辑、渠道保存 payload、渠道能力表、权限矩阵、模型映射 JSON、账号池模式、上游拉取模型弹窗或 `gpt-5.6` 模型元数据。

### 风险评估

1. 多选器受控输入风险：模型多选器原先内部维护搜索词，本轮改为由渠道抽屉传入 `searchValue/onSearchChange`。如果处理不当，可能影响选中候选后清空搜索、自定义模型创建和逗号批量录入。
2. 错误自定义模型风险：输入 `gpt-5.6` 这类系列前缀时，如果搜索结果尚未返回，不能立即创建 `gpt-5.6` 这个不完整自定义模型。本轮复用 `isLoading` 与 `allowCreateWithMatches=false`，搜索中和存在候选时都隐藏创建入口。
3. 搜索追加一致性风险：预览候选和点击“添加全部”仍必须使用同一套 keyword + vendor 规则；本轮保留前一轮的分页扫描、request sequence 和 `open/channelId/keyword/vendor` 上下文校验。
4. 页面回归风险：移除独立自定义输入框后，自定义模型仍必须能添加。`MultiSelect` 已支持逗号、中文逗号和换行拆分；本轮不删除该能力。
5. 权限风险：非敏感写/敏感写权限仍由 `canEditBasicFields` 控制，搜索按钮、模型选中、快捷操作和映射修复按钮继续沿用已有权限判断。
6. 运行态数据风险：页面验证只修改未保存的表单草稿，不点击 `Update Channel`，避免污染 3003 的真实渠道模型配置。

### 方案评审

采用“把 NexusTok 增强能力内聚到 new-api-main 的单入口模型多选器”的方案：

1. `MultiSelect` 增加 `onSearchSubmit`，仅在按 Enter、没有高亮候选、也没有可创建自定义值时触发，用于保持旧搜索框的键盘追加习惯。
2. 渠道编辑页将 `modelSearchKeyword` 作为 `MultiSelect.searchValue`，让同一个输入同时承担本地候选过滤和 `/api/models/search` 远程搜索。
3. `modelOptions` 继续合并 `modelSearchModelNames`，因此 `gpt-5.6-terra` 和 `gpt-5.6-luna` 即使尚未绑定任何渠道能力，也能作为可选候选出现。
4. 将“命中/新增/已存在”摘要和“添加全部匹配模型”按钮移入多选器下拉 footer，避免表单主体出现第二个搜索入口。
5. 模型区顶部保留 `Selected {{count}}` 和 `Vendor: OpenAI/All Vendors` badge，让管理员仍能看到搜索范围。
6. 删除独立 `Search model library` 表单项和独立 `Custom model (comma-separated)` 表单项；自定义模型改回在模型多选器中直接输入创建，与 new-api-main 页面结构对齐。

### 实施结果

已完成渠道编辑页模型搜索内聚与 new-api 页面体验对齐：

- 编辑渠道页 `Models & Groups` 区域恢复为以模型多选器为唯一主入口的结构。
- 输入 `gpt-5.6` 时，多选器会触发 OpenAI vendor 范围内的同步模型库搜索，并把 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 合并到候选体系。
- 已选模型在搜索时会被隐藏，未选中的 `gpt-5.6-terra` 和 `gpt-5.6-luna` 能直接选择或通过 footer 一次性追加。
- “添加全部匹配模型”仍会分页扫描所有搜索结果，避免只追加当前第一页或当前可见候选。
- 搜索中和存在真实候选时不会创建 `gpt-5.6` 这种不完整自定义模型；真正没有候选的自定义模型仍可直接创建。
- 页面主体不再显示 `Search model library`、`Search results` 和 `Custom model (comma-separated)`，整体更接近 new-api-main 最新编辑渠道页。

### 验证记录

1. `cd web/default && bun test src/features/channels/lib/model-search.test.ts src/components/multi-select.test.ts` 通过，共 36 个测试，覆盖模型搜索候选提取、`gpt-5.6` 三模型追加、供应商映射、上下文校验和 MultiSelect 搜索/创建行为。
2. `cd web/default && bun run typecheck` 通过，确认 `ChannelMutateDrawer`、`MultiSelect` 和相关类型无回归。
3. `cd web/default && bun run build` 通过，确认 Rsbuild 生产打包无回归。
4. `cd web/default && bunx prettier --check src/features/channels/components/drawers/channel-mutate-drawer.tsx src/components/multi-select.tsx ../../docs/features/new-api-main-diff-analysis.md` 通过。
5. `git diff --check` 通过。
6. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200；`GET /api/status` 返回正常状态 JSON。
7. 通过 3003 真实接口登录 `c1cada`，调用 `GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=20`，确认返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个 OpenAI 模型。
8. 当前环境没有可用浏览器 MCP 工具，本轮使用本机 Chrome Headless + DevTools Protocol 替代真实页面验证：登录后打开 `http://192.168.0.202:3003/channels`，进入渠道 `11111` 编辑抽屉，页面显示 `Vendor: OpenAI`，且不再出现 `Search model library` 和 `Custom model (comma-separated)`。
9. 在同一编辑抽屉的模型多选器中输入 `gpt-5.6`，页面显示 `3 matched · 2 new · 1 already selected`，候选包含 `gpt-5.6-terra` 和 `gpt-5.6-luna`；点击 `Add 2 new model(s)` 后，未保存草稿显示 `Selected 5`，包含 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`，且未创建错误的 `gpt-5.6` 自定义模型。
10. Chrome Headless 验证过程未发现控制台 error/warning；页面验证只改动未保存草稿，没有点击保存，不污染运行态渠道数据。

## 本轮实施评审：Casbin runtime 影子模式原生化

### 需求分析

继续对照 `/opt/project/new-api-main` 的 Authz 运行链路后确认，new-api-main 不仅有 `authz_roles` 和 `casbin_rule` 表，还直接使用 `github.com/casbin/casbin/v2` 的 `SyncedEnforcer` 加载数据库策略，并通过周期 reload 保持多节点策略一致。NexusTok 当前已经完成权限 catalog、服务端路由 enforcement、用户 override、持久角色策略、导入导出、角色模板管理、角色策略编辑 UI 和结构化审计，但真实授权热路径仍由 `Can()` 的持久策略快照与 override 判定承担。

直接把真实授权切换到 Casbin runtime 风险较高：当前还没有用户角色分配模型，Admin 静态 fallback 与 `casbin_rule` 空策略之间也存在刻意保留的兼容语义。如果一次性替换 `Can()`，可能导致旧数据库、未完成导入或策略缺失环境中的管理员权限突然收紧。本轮目标因此定义为“影子模式”：引入 Casbin runtime/enforcer 和只读 adapter，加载同一张 `casbin_rule` 表做旁路观测和一致性对比，但不改变任何真实授权结果。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| Authz 影子 adapter | `service/authz/shadow_adapter.go` | 新增只读 GORM adapter，从 `casbin_rule` 加载策略；写入方法全部拒绝，避免 shadow enforcer 误改数据库。 |
| Authz 影子 enforcer | `service/authz/shadow_enforcer.go` | 新增 Casbin model、`InitShadowEnforcer()`、`ReloadShadowPolicy()`、周期同步、角色权限 shadow 检查和角色矩阵差异对比。 |
| 启动初始化 | `main.go` | 在持久策略 seed/reload 后初始化 shadow enforcer；初始化或对比失败只记录系统日志，不阻塞服务启动。 |
| Go 依赖 | `go.mod`、`go.sum` | 新增 `github.com/casbin/casbin/v2 v2.135.0` 及其间接依赖。 |
| 回归测试 | `service/authz/shadow_enforcer_test.go` | 覆盖策略加载、reload 不影响 `Can()` 快照、历史空 effect 兼容和 fallback mismatch 观测。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新 Authz/Casbin 状态、后续优先队列和本轮评审记录。 |

本轮不修改数据库 schema，不修改 `Can()`、`Capabilities()`、`ExplicitUserOverrides()`、路由权限中间件、用户 override 主存储、角色策略编辑接口、导入导出格式或默认前端页面。

### 风险评估

1. 授权语义漂移风险：如果 shadow enforcer 参与真实 `Can()` 判定，可能改变 Admin fallback、用户 override 和现有持久策略快照语义。本轮所有 shadow API 都只用于观测和测试，真实授权仍走原路径。
2. 数据库写入风险：Casbin enforcer 默认可能通过 adapter 写策略；本轮关闭 AutoSave，且 adapter 的 `SavePolicy/AddPolicy/RemovePolicy/RemoveFilteredPolicy` 全部返回只读错误。
3. 启动可用性风险：Casbin model、adapter 或数据库策略异常不应让主服务启动失败。本轮在 `main.go` 中将 shadow 初始化作为非阻塞增强，失败只写系统日志。
4. 历史策略兼容风险：早期 `casbin_rule` 中可能存在 `ptype=p` 且 `V3` 为空的 allow 策略。本轮加载时把这种三元 allow 规则补齐为 `allow`，避免影子检查误判历史数据。
5. 日志噪声风险：启动时如发现大量 shadow mismatch，逐条打印会污染系统日志。本轮只记录 mismatch count，详细差异保留给后续接口/页面可视化。
6. 依赖引入风险：新增 Casbin 会增加 Go module 依赖面。本轮固定版本 `v2.135.0`，并通过 `go mod tidy` 保持直接/间接依赖关系清晰。

### 方案评审

采用“只读 shadow runtime + 一致性观测”的低风险方案：

1. 使用与 new-api-main 对齐的三元请求模型：`sub,obj,act`，策略为 `sub,obj,act,eft`，仅 `eft=allow` 生效。
2. `shadowGormAdapter` 只实现加载能力，按 `id ASC` 读取 `casbin_rule`，不承担任何保存职责。
3. `InitShadowEnforcer()` 只创建 `casbin.SyncedEnforcer` 并关闭 AutoSave；全局指针用 `RWMutex` 保护，方便周期 reload 和测试隔离。
4. `ShadowRoleAllows()` 只接受已知 permission 和非空 role key，返回 `ok=false` 表示不可用于真实授权。
5. `CompareShadowRolePolicies()` 只比较数据库中已启用、非 superuser 的角色模板，把当前 NexusTok 角色 grants 作为 expected，把 Casbin `Enforce()` 作为 shadow 结果。
6. 服务启动时先完成现有 `SeedPersistentPolicies()`、`ReloadPersistentPolicies()` 和持久策略同步，再启动 shadow runtime，保证影子检查读取的是当前系统已经使用的策略表。

### 实施结果

已完成 Casbin runtime 影子模式原生化：

- NexusTok 已正式引入 `github.com/casbin/casbin/v2`，但只在 `service/authz` shadow 路径使用。
- `casbin_rule` 可被只读 GORM adapter 加载到 `casbin.SyncedEnforcer`。
- shadow runtime 支持手动 reload 和按 `common.SyncFrequency` 周期 reload。
- `ShadowRoleAllows()` 可用于测试或后续观测接口验证某个角色在 Casbin 下的权限结果。
- `CompareShadowRolePolicies()` 可对比当前角色 grants 与 Casbin shadow 结果，提前暴露直接切换 runtime 的风险。
- `main.go` 启动时会初始化 shadow runtime 并记录 mismatch count；失败不影响主服务可用性。
- `Can()`、路由权限中间件、用户 override 和前端权限矩阵完全保持原行为。

### 验证记录

1. `go test ./service/authz -run 'TestShadow|TestReloadShadow|TestCompareShadow'` 通过，覆盖本轮新增 shadow 运行时核心语义。
2. `go test ./service/authz` 通过，确认 Authz 服务层完整回归。
3. `go test ./router ./controller -run 'Test.*Authz|Test.*Permission|Test.*Channel|Test.*Role|Test.*Policy'` 通过，确认路由和 controller 权限/渠道/角色相关测试不受 shadow runtime 影响。
4. 使用 `GOPROXY=https://goproxy.cn,direct go mod tidy` 整理依赖；官方 Go proxy 首次拉取 Casbin 依赖曾超时，本轮改用国内 proxy 后成功。
5. `go test ./middleware ./model ./service -run 'Test.*Authz|Test.*Permission|Test.*Role|Test.*Policy|Test.*Channel'` 通过，确认中间件、模型层和 service 相关回归正常。
6. `go test ./service/authz ./router ./controller ./middleware ./model` 通过，确认受影响后端包完整回归。
7. `cd web/default && bunx prettier --check ../../docs/features/new-api-main-diff-analysis.md` 通过，确认报告格式正常。
8. `git diff --check` 通过，确认没有空白或补丁格式问题。
9. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200；`GET /api/status` 返回正常状态 JSON，`start_time=1783778940`，对应 2026-07-11 14:09:00Z，说明 3003 已在本轮修改后重新加载，不是旧进程缓存。
10. 使用账号 `c1cada` 登录 3003 后调用 `GET /api/authz/roles` 返回 `success=true`，Root/Admin 角色矩阵正常，Admin `policy_count=25`，确认真实授权接口不受 shadow runtime 影响。
11. 扫描 `nexustok-api-hot` 最近 15 分钟容器日志，没有发现 `authz shadow` 初始化/对比/reload 错误，也没有匹配到 `panic` 或 `fatal`。
12. 当前环境没有暴露浏览器 MCP 工具，本轮使用 `curl` 访问页面/API 并结合 Docker 日志确认运行态；由于本轮无前端可见页面改动，没有执行会污染业务数据的页面写操作。

## 本轮实施评审：Casbin shadow mismatch 后台可视化

### 需求分析

上一轮已经把 `new-api-main` 的 Casbin runtime/enforcer 优势转换为 NexusTok 的只读影子运行时：`casbin_rule` 可以加载进 `SyncedEnforcer`，并能在服务层比较当前角色权限矩阵与 Casbin shadow 结果。但该能力仍停留在日志和测试层，Root 管理员在后台无法判断 shadow runtime 是否初始化、当前是否存在 mismatch，也无法在切换真实授权 runtime 前直观看到风险点。

NexusTok 当前已经比 new-api-main 走得更远：有自定义角色模板、角色策略矩阵 UI、导入导出、用户 override 镜像和 Casbin shadow runtime。因此本轮不再照搬 new-api-main 仅按系统角色映射 Root/Admin 的轻量 assignment，而是把“切换风险可观测”发展成 NexusTok 原生治理能力：在已有 Role Policies 页面展示 Casbin shadow 一致性摘要，并提供 Root-only API 供后续审计报表复用。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| Authz shadow service | `service/authz/shadow_enforcer.go` | 新增 `ShadowRolePolicyComparison` 和 `CompareShadowRolePolicyStatus()`，把 shadow 未初始化表示为 `available=false`，避免只读观测能力影响角色页可用性。 |
| Authz controller | `controller/authz.go`、`controller/authz_shadow_test.go` | 新增 `ComparePermissionRoleShadowPolicies` 只读 handler，并测试 shadow 未初始化时仍返回成功状态。 |
| Authz router | `router/authz-router.go`、`router/authz_router_test.go` | 新增 `GET /api/authz/shadow/role-mismatches`，保持 RootAuth + `system_setting.secret_view`，并锁定路由 handler 与权限分类。 |
| 默认前端 API/type | `web/default/src/features/system-settings/{api.ts,types.ts}` | 新增 shadow mismatch 响应类型和查询函数。 |
| Role Policies 页面 | `web/default/src/features/system-settings/security/role-policy-section.tsx` | 在角色策略编辑器中展示 shadow unavailable、passed、mismatch 三种状态；有 mismatch 时展示前 8 条明细。 |
| 前端 i18n | `web/default/src/i18n/locales/*.json` | 补齐 en/zh/fr/ja/ru/vi 六语 shadow 文案。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新 Authz/Casbin 状态、API 摘要、后续队列和本轮评审记录。 |

本轮不修改数据库 schema，不新增写接口，不修改 `Can()`、`Capabilities()`、用户 override、角色策略保存、策略导入导出、Casbin adapter 写入语义、权限 catalog 或真实授权路由中间件。

### 风险评估

1. 授权路径风险：shadow mismatch 只读展示不能参与真实授权判定；本轮只新增 API/UI 观测，`Can()` 热路径不变。
2. 页面可用性风险：shadow runtime 初始化失败不应导致 Role Policies 页面整体失败；本轮后端以 `available=false` 表达未初始化，前端显示提示而非错误中断。
3. 路由冲突风险：已有 `/api/authz/roles/:key` 动态路由会吞掉 `/roles/*` 下的静态路径；本轮选择 `/api/authz/shadow/role-mismatches`，避免动态段冲突。
4. 信息暴露风险：mismatch 明细会暴露角色、资源和动作，因此继续保持 RootAuth 保底，并归类为 `system_setting.secret_view`。
5. UI 过载风险：策略异常时 mismatch 可能很多；本轮页面只展示前 8 条并显示隐藏数量，完整导出/过滤留给后续审计报表。
6. i18n 风险：新增前端文案必须补齐六语；本轮运行 `bun run i18n:sync`，报告中 missingCount 为 0。

### 方案评审

采用“单独只读 API + Role Policies 内嵌摘要”的方案：

1. 后端复用现有 `CompareShadowRolePolicies()`，新增 `CompareShadowRolePolicyStatus()` 作为前端友好 DTO，避免 controller 识别内部错误类型。
2. 新 API 返回 `available`、`mismatch_count` 和 `mismatches`；未初始化时 success=true 且 `available=false`。
3. 路由路径放在 `/api/authz/shadow/role-mismatches`，继续走 `permissionRoute` 统一权限分类。
4. 默认前端使用独立 React Query key `authz-shadow-role-mismatches`，刷新角色策略时同步刷新 catalog、roles 和 shadow 对比结果。
5. 页面状态分三种：未初始化提示、完全一致提示、发现 mismatch 的警示表格。
6. mismatch 表格显示 Role、Permission、Current、Shadow，Current 表示当前 NexusTok 角色矩阵，Shadow 表示 Casbin shadow runtime 结果。

### 实施结果

已完成 Casbin shadow mismatch 后台可视化：

- `GET /api/authz/shadow/role-mismatches` 已提供 Root-only 只读一致性观测入口。
- shadow 未初始化时 API 返回 `available=false`，不会拖垮 Role Policies 页面。
- Role Policies 页面会显示 Casbin shadow runtime 状态；一致时展示通过提示，不一致时展示 mismatch count 和前 8 条明细。
- 页面刷新按钮会同时刷新角色列表、权限 catalog 和 shadow 对比结果。
- 新增 UI 文案已补齐 en、zh、fr、ja、ru、vi。
- 真实授权仍由 NexusTok 当前持久策略快照和用户 override 判定，未接入 Casbin shadow 结果。

### 验证记录

1. `go test ./service/authz ./router ./controller -run 'TestShadow|TestCompareShadow|TestRegisterAuthz|TestAuthzPermission|TestComparePermissionRoleShadow'` 通过，覆盖 shadow status DTO、handler、路由 handler 和权限分类。
2. `cd web/default && bun run typecheck` 通过，确认新增 API/type 和 Role Policies 页面类型无回归。
3. `cd web/default && bun test src/features/system-settings/security/role-policy-utils.test.ts src/lib/admin-permissions.test.ts` 通过，确认角色策略工具函数与权限 helper 回归正常。
4. `cd web/default && bun run i18n:sync` 通过，`_sync-report.json` 中所有 locale 的 `missingCount=0`、`extrasCount=0`；新增 shadow 文案已覆盖六语。
5. `go test ./service/authz ./router ./controller ./middleware ./model` 通过，确认受影响后端包完整回归。
6. `cd web/default && bun run build` 通过，确认默认前端生产打包无回归。
7. `cd web/default && bunx prettier --check src/features/system-settings/api.ts src/features/system-settings/types.ts src/features/system-settings/security/role-policy-section.tsx src/i18n/locales/en.json src/i18n/locales/zh.json src/i18n/locales/fr.json src/i18n/locales/ja.json src/i18n/locales/ru.json src/i18n/locales/vi.json ../../docs/features/new-api-main-diff-analysis.md` 通过。
8. `git diff --check` 通过，确认没有空白或补丁格式问题。
9. 访问 `http://192.168.0.202:3003/` 返回 HTTP 200；`GET /api/status` 返回 `start_time=1783780288`，对应 2026-07-11 14:31:28Z，说明 3003 已在本轮修改后重新加载。
10. 使用账号 `c1cada` 登录 3003 后调用 `GET /api/authz/shadow/role-mismatches`，返回 `success=true,available=true,mismatch_count=0,mismatches=[]`。
11. 使用 Chrome Headless + DevTools Protocol 登录并打开 `http://192.168.0.202:3003/system-settings/security/role-policies`，页面显示 `Role Policies` 和 `Casbin shadow comparison passed`，并展示说明 `The current role matrix matches the Casbin shadow runtime for all enabled non-superuser roles.`。
12. Chrome Headless 验证期间未捕获 console error/warning、runtime exception 或 Network loadingFailed。
13. 扫描 `nexustok-api-hot` 最近 20 分钟容器日志，没有发现 `authz shadow`、`casbin`、`panic`、`fatal` 或 shadow reload/compare 相关错误。
14. 当前环境没有暴露浏览器 MCP 工具，本轮使用 Chrome Headless + DevTools Protocol 与真实 HTTP API 替代 MCP 页面/接口验证。

## 本轮实施评审：渠道编辑页搜索命中完整展示修复

### 需求分析

用户再次反馈“搜索添加时不正确”，并要求继续参考 `/opt/project/new-api-main` 最新编辑渠道页面体验。复查运行态后确认，后端模型元信息完整：`GET /api/models/search?keyword=gpt-5.6&p=1&page_size=50` 返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个 OpenAI 供应商模型。但默认前端渠道编辑抽屉在模型搜索时启用了 `hideSelectedOptionsWhenSearching`，导致已经在渠道模型列表中的 `gpt-5.6-sol` 被下拉候选隐藏，只显示尚未选中的 `gpt-5.6-terra` 和 `gpt-5.6-luna`。

这会产生明显的感知偏差：footer 显示 “3 matched · 2 new · 1 already selected”，但候选列表只看得见两个模型。管理员很容易理解为“搜索添加只同步/识别到了两个模型”，尤其是在前序问题已经围绕“gpt-5.6 有三种模型”反复确认时。本轮目标是让搜索命中完整可见：已选模型保留勾选态出现在候选中，批量按钮仍只补齐缺失模型，避免重复写入。

### 影响范围分析

| 模块 | 文件 | 影响 |
| --- | --- | --- |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 移除渠道模型选择器的 `hideSelectedOptionsWhenSearching` 配置，使搜索命中的已选模型也显示在下拉候选中。 |
| 通用 MultiSelect | `web/default/src/components/multi-select.tsx` | 不改组件默认行为；其它调用方仍可按需隐藏已选项。 |
| 后端/API/数据库 | 无 | 不修改模型搜索、渠道保存、能力写入、数据库结构或 relay。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮复现、风险和验证结果。 |

### 风险评估

- 本轮只改变渠道编辑页模型搜索候选展示，不改变 `updateModels`、`transformFormDataToUpdatePayload`、`/api/models/search` 或 `/api/channel` 保存语义。
- 已选模型显示在候选中后，搜索结果列表会比之前多出已选项，但 Base UI Combobox 会用选中态标记它，反而更接近 new-api “搜索/选择已有模型”的直觉。
- 批量补齐按钮仍按 `getMissingModelSearchMatches` 只追加当前渠道缺失的模型；例如当前已有 `gpt-5.6-sol` 时，点击按钮只追加 `gpt-5.6-terra` 和 `gpt-5.6-luna`，不会重复加入 sol。
- 搜索动作仍只改前端表单草稿；管理员必须点击 `Update Channel` 才会保存运行态渠道能力。
- 当前会话没有浏览器 MCP 工具，页面验证继续使用 Chrome Headless + DevTools Protocol 替代，并在最终记录中明确说明。

### 方案评审

采用最小修复方案：保留 NexusTok 已有远程模型元信息搜索和缺失模型补齐 footer，只取消渠道模型选择器的“搜索时隐藏已选项”策略。这样 `gpt-5.6` 搜索时会同时展示 `gpt-5.6-terra`、`gpt-5.6-luna` 和已选的 `gpt-5.6-sol`；footer 的 “2 new” 只表达可新增数量，不再与候选列表形成视觉矛盾。该方案比移除搜索补齐能力更符合 NexusTok 的原生增强目标，也比继续增加解释性文案更低风险。

### 验收方式

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts`。
2. `cd web/default && bun run typecheck`。
3. `cd web/default && bun run build`。
4. `git diff --check`。
5. 访问 `http://192.168.0.202:3003/` 和 `/api/status` 确认热更新服务可用；如页面未更新则重启容器后再验证。
6. 使用 Chrome Headless + CDP 登录 3003，打开 `/channels` 编辑渠道 `11111`，在模型输入框搜索 `gpt-5.6`，确认候选中同时出现 terra、luna、sol，且 footer 仍显示只可新增两个缺失模型。

### 实施结果

已完成渠道编辑页搜索命中完整展示修复。渠道模型 MultiSelect 不再在搜索时隐藏已选模型，因此 `gpt-5.6` 搜索结果会完整展示 `gpt-5.6-terra`、`gpt-5.6-luna` 和已选中的 `gpt-5.6-sol`。footer 保持原有补齐语义，仍显示 `3 matched · 2 new · 1 already selected` 与 `Add 2 new model(s)`，点击后只追加 terra/luna 两个缺失模型，不重复写入 sol。

该调整保留 NexusTok 相比 new-api 更强的模型元信息搜索补齐能力，同时让页面候选展示更接近 new-api 的选择器直觉：搜索命中是什么就完整显示什么，已选项由勾选态表达，而不是从列表中消失。本轮没有修改后端模型搜索接口、渠道保存接口、数据库结构、权限控制、账号池凭据模式或 relay 行为。

### 验证记录

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts` 通过，36 个用例全部成功。
2. `cd web/default && bun run i18n:sync` 通过；本轮没有新增用户可见翻译 key。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. `docker logs --tail 80 nexustok-frontend-watch` 显示 `[hot] published default dist`，确认热更新产物已发布。
7. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200；`GET /api/status` 返回状态 JSON，3003 服务可访问。
8. Chrome Headless + CDP 使用账号 `c1cada` 登录 3003，打开 `/channels` 并编辑渠道 `11111`；模型搜索框输入 `gpt-5.6` 后真实触发 `GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50`，响应状态 200。
9. 运行态 popup 文本为 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`、`3 matched · 2 new · 1 already selected`、`Add 2 new model(s)` 和缺失模型预览 `gpt-5.6-terra, gpt-5.6-luna`；不再出现 `Add custom model "gpt-5.6"` 误加入口。
10. Chrome Headless + CDP 真实点击 `Add 2 new model(s)` 后，表单草稿变为 `Selected 5`，并同时包含 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`；验证期间未点击 `Update Channel`，没有保存渠道运行态配置。
11. 页面验证期间没有捕获 `Runtime.exceptionThrown` 或 `Network.loadingFailed`。

## 本轮实施评审：渠道编辑页提交逻辑对齐 new-api 分层

### 需求分析

用户要求继续参考 `/opt/project/new-api-main` 最新编辑渠道页面，把当前编辑渠道页面体验和结构对齐。复核后确认，NexusTok 当前已经具备 new-api 最新版的主要编辑页布局能力：右侧 5xl 抽屉、左侧分区导航、Basic/Credentials/Models/Advanced 分区、模型元信息搜索、`gpt-5.6` 三模型完整候选展示和批量补齐能力均已落地。

本轮发现仍有一个结构性差异：new-api 最新版把创建/编辑渠道的提交 mutation 抽到 `useChannelMutateForm`，drawer 本体主要负责页面状态、前端校验和风险确认；NexusTok 的 drawer 仍内联 `createChannel`、`updateChannel`、敏感字段权限裁剪、提交 loading 和错误处理。该内联逻辑让编辑页继续扩展时更难维护，也不利于后续继续吸收 new-api 的编辑页分区组件化经验。

因此本轮目标不是改变页面文案或后端能力，而是把 new-api 的“提交逻辑分层”优势转换为 NexusTok 原生能力：新增 NexusTok 版 `useChannelMutateForm`，保留当前更细的 `write` / `sensitive_write` 权限边界、账号池组模式、多 Key `key_mode`、空 key 不覆盖和后端错误 toast 语义。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道提交 hook | `web/default/src/features/channels/hooks/use-channel-mutate-form.ts` | 新增 create/update mutation，集中处理成功 toast、错误 toast、权限二次兜底、敏感字段裁剪和多 Key `key_mode`。 |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 移除内联 `createChannel` / `updateChannel` 调用、提交 loading state 和非敏感字段白名单；保留提交前 key 校验、状态码风险确认、模型映射确认和页面交互。 |
| 权限裁剪测试 | `web/default/src/features/channels/hooks/use-channel-mutate-form.test.ts` | 新增普通写权限、敏感写权限、多 Key `key_mode` 和未知字段丢弃测试，锁定 NexusTok 权限语义。 |
| 后端/API/数据库 | 无 | 不修改接口、数据库结构、渠道保存 payload 格式、模型搜索或 relay。 |
| i18n | 无新增 key | 仅复用已有 `API key is required`、创建/更新失败和成功文案；运行 `i18n:sync` 确认。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 渠道保存路径风险：提交逻辑虽然只是前端分层迁移，但触及核心渠道保存入口；必须保持创建、编辑、账号池组、模型映射确认和状态码风险确认的行为一致。
2. 权限越界风险：new-api 的 hook 只按敏感字段列表删除字段，而 NexusTok 当前允许普通 `write` 保存模型、分组、权重、标签等非敏感字段；本轮保留原有白名单裁剪，并加测试防止 `key`、`base_url`、`settings`、`param_override`、`header_override` 等敏感字段泄漏。
3. 多 Key 语义风险：编辑多 Key 渠道时 `key_mode=append/replace` 只能在 `sensitive_write` 且渠道为多 Key 时提交；本轮把该条件封装为 `buildAllowedChannelUpdatePayload` 并测试非多 Key不附加 `key_mode`。
4. 空 key 覆盖风险：编辑模式下表单 key 默认为空，转换函数仍只在用户输入新 key 或账号池组模式下携带 key；本轮不改变该转换逻辑。
5. 错误提示风险：mutation `onError` 统一展示后端错误或创建/更新失败文案；drawer 的提交 handler 捕获并吞掉异常，避免 React Hook Form 出现未处理 promise。
6. 页面回归风险：本轮不改 DOM 结构和文案，但 hook 化会影响 `isSubmitting` 来源；需要通过真实页面打开抽屉、搜索模型和检查控制台/网络错误确认。

### 方案评审

采用“小步迁移提交 mutation，不迁移页面校验”的方案：

1. 新增 `useChannelMutateForm`，参数显式传入 `currentRow`、`isEditing`、`isMultiKeyChannel`、`permissions` 和 `onSuccess`，避免 hook 内重新推导页面状态。
2. 把原 drawer 内的 `NON_SENSITIVE_CHANNEL_UPDATE_FIELDS` 移入 hook，并导出 `pickNonSensitiveChannelUpdatePayload` 与 `buildAllowedChannelUpdatePayload` 供测试覆盖。
3. hook 内保留权限二次兜底：创建必须有 `sensitive_write`；编辑至少要有 `write` 或 `sensitive_write`。
4. drawer 仍负责提交前交互式校验：账号池组模式 key 豁免、状态码风险确认、模型映射 JSON 校验、缺失源模型确认和必要时自动补齐模型。
5. drawer 使用 `mutateAsync` 执行最终保存，`isSubmitting` 直接来自 React Query mutation pending 状态。
6. 不引入新文案、不新增依赖、不改变后端接口，降低上线风险。

### 实施结果

已完成渠道编辑页提交逻辑分层：

- 新增 `web/default/src/features/channels/hooks/use-channel-mutate-form.ts`，作为 NexusTok 原生渠道创建/编辑 mutation hook。
- 渠道编辑抽屉移除内联保存实现，页面主文件减少提交路径细节，继续保留 new-api 对齐后的分区布局和 NexusTok 原有高级交互。
- 普通写权限更新 payload 继续只保留非敏感白名单字段；敏感写权限保留完整 payload；多 Key `key_mode` 只在敏感写权限和多 Key 渠道下携带。
- 搜索添加 `gpt-5.6` 的页面能力未回退：三模型完整展示、2 个缺失模型批量追加、已选 `sol` 保留可见。
- 本轮没有修改后端、数据库、运行态渠道配置、模型元信息或 relay。

### 验证记录

1. `cd web/default && bun test src/features/channels/hooks/use-channel-mutate-form.test.ts src/features/channels/lib/model-search.test.ts src/components/multi-select.test.ts` 通过，40 个用例全部成功，覆盖权限 payload 裁剪、`gpt-5.6` 三模型搜索追加和 MultiSelect 搜索行为。
2. `cd web/default && bun run typecheck` 通过。
3. `cd web/default && bun run i18n:sync` 通过；本轮没有新增用户可见翻译 key。
4. `cd web/default && bun run build` 通过。
5. `docker logs --tail 120 nexustok-frontend-watch` 显示 `[hot] published default dist`，确认 3003 热更新产物已发布。
6. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200。
7. 真实接口登录账号 `c1cada` 后调用 `GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=20`，返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个 OpenAI 模型；`gpt-5.6-sol` 仍绑定渠道 `11111`。
8. Chrome Headless + DevTools Protocol 登录 3003，进入 `/channels`，打开渠道 `11111` 编辑抽屉，初始模型为 `Selected 3`。
9. 在模型输入框搜索 `gpt-5.6`，页面同时展示 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，并显示 `3 matched · 2 new · 1 already selected` 与 `Add 2 new model(s)`；没有出现 `Add custom model "gpt-5.6"`。
10. 点击 `Add 2 new model(s)` 后，未保存表单草稿变为 `Selected 5`，包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`。
11. 验证期间网络记录中 `PUT /api/channel` 请求数为 0；再次查询 `GET /api/channel/?p=1&page_size=5&tag_mode=false&id_sort=false`，渠道 `11111` 持久化模型仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认页面验证没有污染运行态配置。
12. Chrome Headless 验证期间未捕获 `Runtime.exceptionThrown`、`Network.loadingFailed` 或 error/warning 级别 Log。
13. 当前环境没有暴露浏览器 MCP 工具，本轮继续使用 Chrome Headless + DevTools Protocol 和真实 HTTP API 作为 MCP 替代验证方式。

## 本轮实施评审：DataTable 桌面卡片视图渐进分层

### 需求分析

`new-api-main` 的默认前端已经把 DataTable 拆成 `core`、`layout`、`toolbar`、`static`、`hooks` 等更清晰的层次，并在多个数据页沉淀出表格/卡片视图切换能力。NexusTok 当前 DataTable 仍是扁平目录，但已经被渠道、模型、用户、订阅、日志等高频管理页复用；如果一次性迁移全站表格，回归面会很大，也容易影响已经落地的账号池、Authz 和渠道权限能力。

本轮选择“渐进分层”的低风险切片：先把 new-api 的桌面表格/卡片视图优势发展成 NexusTok 的公共 DataTable 原生能力，默认不改变任何已有表格，只在 `/models/metadata` 模型表开启。模型表字段密度适中、只读浏览频率高，适合作为第一张桌面卡片视图验证表。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| DataTable 视图状态 | `web/default/src/components/data-table/use-data-table-view-mode.ts` | 新增 `table/card` 两种模式、localStorage 读写和不可用容错。 |
| DataTable 工具栏 | `web/default/src/components/data-table/view-mode-toggle.tsx`、`toolbar.tsx` | 新增 Hugeicons 表格/卡片切换控件，并允许 toolbar 在列显示菜单前插入该控件。 |
| DataTable 桌面卡片布局 | `card-grid.tsx`、`card-row-content.tsx`、`data-table-page.tsx` | 新增桌面卡片 grid 和列 meta 驱动的通用卡片内容；`DataTablePage` 增加可选 `enableCardView`、`renderCard`、`viewModeStorageKey` 等 props。 |
| 模型管理页 | `web/default/src/features/models/components/models-table.tsx` | 仅模型表启用桌面表格/卡片视图切换，持久化 key 为 `models-table-view-mode`。 |
| TanStack 列 meta 类型 | `web/default/src/tanstack-table.d.ts` | 补齐 `mobileTitle`、`mobileBadge`、`mobileHidden`、`mobileOrder` 类型说明，供移动卡片和桌面卡片共用。 |
| DataTable 文档 | `web/default/src/components/data-table/README.md`、`docs/features/new-api-main-diff-analysis.md` | 补充视图切换接入方式、风险边界和本轮验证记录。 |

本轮不修改后端接口、数据库结构、模型查询参数、权限系统、URL 同步、分页、排序、筛选、批量操作或其它表格页面默认行为。

### 风险评估

1. 全站回归风险：`enableCardView` 默认关闭，未显式接入的表格仍渲染原桌面表格和移动端卡片。
2. 状态持久化风险：localStorage 在隐私模式或受限 iframe 中可能不可用；本轮读写均 catch，失败时只回退内存状态。
3. 移动端风险：视图切换只在非移动端显示，移动端继续走已有 `MobileCardList`/自定义 mobile slot。
4. 列渲染风险：通用卡片复用 TanStack cell renderer，不重新解释业务字段；自定义表格后续可通过 `renderCard` 覆盖。
5. 视觉一致性风险：控件遵循当前 shadcn 配置的 Hugeicons 图标库和现有 Base Nova token，不新增一套视觉体系。
6. 数据写入风险：模型表视图切换只操作 localStorage，不发起任何业务写接口；页面验证中需要确认没有 `POST/PUT/PATCH/DELETE` 写请求。

### 方案评审

采用“公共底座 + 单页试点”的方案：

1. `useDataTableViewMode` 负责 `table/card` 模式校验、读取、写入和异常容错。
2. `DataTableViewModeToggle` 作为 toolbar 可插拔控件，不侵入搜索、过滤、列显隐或扩展工具栏。
3. `DataTableCardGrid` 使用现有 `table.getRowModel()` 和可见列集合，保持分页、排序、筛选结果与桌面表格一致。
4. `CardRowContent` 优先读取列 meta 中的 `mobileTitle`、`mobileBadge`、`mobileHidden` 和 `mobileOrder`，让已有移动端列语义可以复用到桌面卡片。
5. `DataTablePage` 新增可选 props，但默认路径仍调用原 `renderDesktop` 表格。
6. `/models/metadata` 作为第一处试点，只传入 `enableCardView` 和 `viewModeStorageKey`；其它表格后续按页面需求逐步接入。

### 实施结果

已完成 NexusTok 原生 DataTable 桌面卡片视图底座：

- 新增表格/卡片视图模式 hook、视图切换控件、通用桌面卡片 grid 和通用卡片内容渲染。
- `DataTableToolbar` 支持渲染可选视图切换控件；未传入时工具栏 DOM 行为不变。
- `DataTablePage` 支持非受控/受控视图模式、本地持久化 key、自定义卡片渲染器和卡片 grid className。
- `/models/metadata` 模型表启用桌面视图切换；默认仍从表格视图开始，用户切换后记住本地偏好。
- DataTable README 已补充视图切换接入示例和“默认关闭、逐页接入”的边界说明。
- 本轮没有修改后端模型接口、模型元信息同步、渠道能力、权限矩阵或数据库。

### 验证记录

1. `cd web/default && bun test src/components/data-table/use-data-table-view-mode.test.ts` 通过，4 个用例覆盖合法模式、非法回退和 storage 不可用容错。
2. `cd web/default && bun run typecheck` 通过。
3. `cd web/default && bun run i18n:sync` 通过；本轮复用已有 `Card view`、`Table view`、`View mode` 等翻译 key，没有新增用户可见文案。
4. `cd web/default && bun run build` 通过。
5. `docker logs --tail 120 nexustok-frontend-watch` 显示 `[hot] published default dist`。
6. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200，响应头包含 `Cache-Version: b688f2fb5be447c25e5aa3bd063087a83db32a288bf6a4f35f2d8db310e40b14`。
7. `GET /api/status` 返回 `start_time=1783784619`，对应 2026-07-11 15:43:39Z，确认 3003 服务处于本轮验证时段内。
8. Chrome Headless + DevTools Protocol 使用账号 `c1cada` 登录 3003，写入前端 auth store 后打开 `http://192.168.0.202:3003/models/metadata`。
9. 模型页初始状态存在 `aria-label="View mode"` 切换控件，桌面表格 `tbody tr` 为 20 行，卡片数为 0。
10. 点击卡片视图后，`[data-slot="data-table-card"]` 为 20 个，`localStorage.models-table-view-mode` 为 `card`，表格行数为 0。
11. 再点击表格视图后，`tbody tr` 恢复为 20 行，卡片数为 0，`localStorage.models-table-view-mode` 为 `table`。
12. Chrome Headless 验证期间未捕获业务写请求、`Runtime.exceptionThrown`、`Network.loadingFailed` 或 error/warning 级别 console 记录。
13. 当前环境没有暴露浏览器 MCP 工具，本轮继续使用 Chrome Headless + DevTools Protocol、真实 HTTP API 和 Docker 热更新日志作为 MCP 替代验证方式。

## 本轮实施评审：渠道模型搜索 Enter 追加与编辑入口稳定性

### 需求分析

用户继续反馈“搜索添加时不正确”，并要求把编辑渠道页面向 `/opt/project/new-api-main` 最新编辑页体验对齐。结合运行态复查后确认：模型元信息已经完整，`GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50` 返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个 OpenAI 模型；当前渠道 `11111` 只持久化 `gpt-5.6-sol`。

进一步用 3003 真实页面复现发现：点击模型搜索弹层底部的 `Add 2 new model(s)` 可以正确把 Terra/Luna 追加到未保存草稿，问题集中在键盘/默认选择路径。管理员在模型多选器输入 `gpt-5.6` 后按 Enter，会触发全量搜索请求，但 Base UI Combobox 的默认键盘选择会同时介入，导致搜索词/弹层状态被消费，最终表单没有追加缺失模型。这会让“搜索添加”看起来只识别或只添加了一个模型。

同时，页面列表入口仍使用 `/api/channel`，后端运行态会先返回 301 再重定向到 `/api/channel/`。这不直接造成模型丢失，但会给编辑入口真实验证增加一次无意义跳转，也容易让列表加载状态和网络排查变得含混。本轮将 new-api 的单入口编辑体验转换为 NexusTok 原生能力时，需要保留当前更强的模型元信息搜索补齐能力，并修复 Enter 路径的语义。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| MultiSelect 键盘行为 | `web/default/src/components/multi-select.tsx` | 新增默认关闭的 `submitSearchOnEnterWithMatches` 开关；搜索有候选或正在搜索时，Enter 可优先提交搜索追加。 |
| MultiSelect 测试 | `web/default/src/components/multi-select.test.ts` | 新增纯函数覆盖，确认启用场景、搜索中场景、自定义模型保留路径和默认关闭行为。 |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 仅渠道模型选择器启用 Enter 搜索追加，不影响分组选择器和其它 MultiSelect 使用方。 |
| 渠道列表 API | `web/default/src/features/channels/api.ts` | `getChannels()` 改为直接请求 `/api/channel/`，减少 301 跳转。 |
| 后端/API/数据库 | 无 | 不修改模型搜索接口、渠道保存接口、渠道能力表、账号池、权限矩阵、relay 或数据库结构。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、实施结果和验证记录。 |

### 风险评估

1. 键盘行为回归风险：MultiSelect 是通用组件，不能让所有多选器的 Enter 都变成搜索提交。本轮新增开关默认关闭，只在渠道模型选择器启用。
2. 自定义模型风险：渠道仍需要支持真正的自定义模型。新逻辑只在存在候选或搜索仍在进行时接管 Enter；没有候选且不在搜索中时继续走原来的自定义创建路径。
3. 异步污染风险：搜索追加仍沿用已有请求序号、渠道 ID、关键词和供应商上下文校验，避免旧搜索结果污染当前表单。
4. 渠道保存风险：本轮只修改未保存草稿层的交互，不触碰 `Update Channel`、保存 payload、权限裁剪或后端写接口；验证必须确认没有 `PUT /api/channel`。
5. 入口路径风险：`getChannels()` 仅从后端最终路径 `/api/channel/` 发起 GET，和当前 301 后的最终资源一致，不改变查询参数、分页、排序或响应解析。
6. 页面视觉风险：不做大面积重绘，不复制 new-api 的简化 MultiSelect；保留 NexusTok 已有的账号池、供应商收窄、三模型完整候选展示和批量补齐能力。

### 方案评审

采用“小范围 opt-in 键盘修复 + 列表入口路径稳定”的方案：

1. 在 MultiSelect 中抽出 `shouldSubmitMultiSelectSearchOnEnter()`，用纯函数明确 Enter 何时由搜索提交接管。
2. 在输入框 `onKeyDownCapture` 阶段拦截符合条件的 Enter，先 `preventDefault` / `stopPropagation`，再调用 `onSearchSubmit`，避免 Base UI Combobox 先按高亮候选消耗事件。
3. 该行为必须同时满足：调用方显式启用、存在 `onSearchSubmit`、按键是 Enter、输入非空、且“正在搜索或已有匹配候选”。否则保持原有候选选择/自定义创建行为。
4. 仅渠道编辑页模型 MultiSelect 传入 `submitSearchOnEnterWithMatches`，其它 MultiSelect 不改变。
5. `getChannels()` 改用 `/api/channel/`，让前端直接请求后端真实 200 路径。
6. 不引入新依赖、不新增用户可见文案、不改后端。

### 实施结果

已完成渠道模型搜索 Enter 追加修复与编辑入口稳定性增强：

- 输入 `gpt-5.6` 后按 Enter 会优先执行搜索补齐，草稿一次性追加缺失的 `gpt-5.6-terra` 和 `gpt-5.6-luna`。
- 页面仍完整展示 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个搜索命中，footer 继续显示 `3 matched · 2 new · 1 already selected`。
- 真正没有候选的自定义模型仍可通过 Enter 添加，避免破坏渠道手工模型能力。
- 渠道列表前端请求直接使用 `/api/channel/`，真实页面网络中不再出现 `/api/channel?` 301 起跳。
- 本轮没有修改后端模型同步、模型搜索、渠道保存、权限、账号池或 relay 行为。

### 验证记录

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts src/features/channels/hooks/use-channel-mutate-form.test.ts` 通过，45 个用例全部成功。
2. `cd web/default && bunx prettier --check src/components/multi-select.tsx src/components/multi-select.test.ts src/features/channels/api.ts src/features/channels/components/drawers/channel-mutate-drawer.tsx` 通过。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run i18n:sync` 通过；本轮没有新增用户可见翻译 key。
5. `cd web/default && bun run build` 通过。
6. `git diff --check` 通过。
7. `docker logs --tail 120 nexustok-frontend-watch` 显示 `[hot] published default dist`。
8. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200，确认 3003 页面可访问。
9. 真实接口登录账号 `c1cada` 后调用 `/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50`，确认返回 Terra/Luna/Sol 三个 OpenAI 模型。
10. 直接调用后端路径确认：无尾斜杠 `/api/channel?tag_mode=false&id_sort=false&p=1&page_size=5` 返回 301，带尾斜杠 `/api/channel/?tag_mode=false&id_sort=false&p=1&page_size=5` 返回 200；前端已改为直接请求后者。
11. Chrome Headless + DevTools Protocol 清缓存登录 3003，打开 `/channels` 并编辑渠道 `11111`；初始模型显示 `Selected 3`，包含 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`。
12. 页面网络记录显示渠道列表直接请求 `GET /api/channel/?tag_mode=false&id_sort=false&p=1&page_size=20`，没有 `/api/channel?` 请求。
13. 在模型输入框输入 `gpt-5.6` 后，页面显示三条模型候选、`3 matched · 2 new · 1 already selected` 和 `Add 2 new model(s)`。
14. 按 Enter 后页面触发 `/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=100` 全量扫描，toast 显示 `Added 2 model(s) from search`，未保存草稿变为 `Selected 5`，包含 `gpt-5.6-terra`、`gpt-5.6-luna` 和 `gpt-5.6-sol`。
15. Chrome Headless 验证期间业务写请求数为 0，没有 `PUT /api/channel`，没有 `Runtime.exceptionThrown`、`Network.loadingFailed`、error/warning console 或 error/warning Log。
16. 验证后再次调用 `GET /api/channel/?tag_mode=false&id_sort=false&p=1&page_size=5`，渠道 `11111` 持久化模型仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认没有污染运行态配置。
17. 当前环境没有暴露浏览器 MCP 工具，本轮继续使用 Chrome Headless + DevTools Protocol、真实 HTTP API 和 Docker 热更新日志作为 MCP 替代验证方式。

## 本轮补充评审：渠道模型搜索追加数量语义修正

### 需求分析

用户继续反馈“搜索添加时不正确”，并要求渠道编辑页继续对齐 `/opt/project/new-api-main` 最新编辑渠道页面。结合前序验证，当前页面已经具备 new-api 风格的渠道编辑分区、模型区平铺快捷操作、模型映射 `Visual / JSON` Tabs、`gpt-5.6` 三模型完整候选展示，以及 Enter/点击批量追加能力。本轮复查发现剩余问题集中在模型元信息搜索的数量语义：前端把 `/api/models/search` 返回的 `total` 当作“可添加模型数”展示，但后端 `total` 实际表示模型元数据行数；规则模型还可能通过 `matched_models` 展开出多个真实模型。

因此，当搜索结果跨分页或包含名称规则模型时，按钮可能显示“添加全部 N 个匹配模型”，但 N 并不等于真实可写入渠道的模型数量。正确语义应当是：当前页已展开的 `model_name + matched_models` 决定“真实命中/可新增/已存在”数量；如果后端还有未扫描的元数据行，按钮只表达“扫描全部搜索结果”，点击后重新读取所有分页并按真实模型名集合追加。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 模型搜索结果解释 | `web/default/src/features/channels/lib/model-search.ts` | 新增 `getModelSearchModelNameResult()`，显式区分已展开真实模型名与未展开匹配数量；保留原 `getModelSearchModelNames()` 兼容调用方。 |
| 模型搜索测试 | `web/default/src/features/channels/lib/model-search.test.ts` | 增加规则模型 `matched_count / matched_models` 用例，覆盖三模型展开和未展开匹配不写入渠道的边界。 |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 分页提示改为按已加载元数据行数计算；有未扫描分页时按钮显示 `Scan all search results`，不再把元数据行数伪装为模型数量。 |
| 后端/API/数据库 | 无 | 不修改 `/api/models/search`、渠道保存接口、渠道能力写入、数据库结构或 relay。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、实施和验证结果。 |

### 风险评估

1. 搜索追加回归风险：不能破坏前序已验证的 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三模型完整展示；本轮不再隐藏已选模型，footer 仍按缺失模型集合只追加未选择项。
2. 数量误导风险：后端 `total` 是元数据行数而非真实模型数，继续用于按钮“Add all {{count}} matched model(s)”会误导管理员。本轮只在分页提示中使用它，并改称“搜索结果”。
3. 数据污染风险：规则模型若声明 `matched_count` 大于 `matched_models`，前端不能凭数量猜测模型名写入渠道；本轮只追加后端实际返回的模型名。
4. 异步污染风险：全分页扫描仍沿用请求序号、渠道 ID、关键词和供应商上下文校验，避免旧搜索结果污染当前表单草稿。
5. 保存风险：本轮只改模型选择草稿和按钮文案，不触发渠道保存；页面验证必须确认没有 `PUT /api/channel`。

### 方案评审

采用“真实模型集合优先，元数据行数只用于扫描提示”的方案：

1. 抽出 `getModelSearchModelNameResult()`，让规则模型的 `matched_models` 展开逻辑有明确返回结构。
2. `getModelSearchModelNames()` 保持原函数签名，避免大范围改调用方。
3. 当前页 summary 继续使用真实模型名集合统计 `matched / new / already selected`。
4. 未扫描数量改为 `backend total - current page item count`，表达还有多少元数据行需要继续扫描。
5. 未扫描分页存在时按钮改为 `Scan all search results`；点击后仍调用 `fetchAllModelSearchModelNames()` 扫描所有分页，再按 `getMissingModelSearchMatches()` 只追加缺失模型。
6. 不新增依赖、不改后端、不改保存 payload、不改权限裁剪。

### 实施结果

- 规则模型搜索结果会以展开出的真实模型名作为追加基准；例如 `gpt-5.6` 规则模型展开为 Terra/Luna/Sol 时，前端统计为 3 个真实模型。
- 规则模型声明了更多 `matched_count` 但未返回具体 `matched_models` 时，前端不会猜测或写入不可见模型名，只记录未展开数量供后续排查。
- 分页场景不再显示误导性的 `Add all {{count}} matched model(s)`；若还有未扫描元数据行，按钮显示 `Scan all search results`。
- 点击扫描仍会读取所有分页，并按当前表单最新模型集合追加缺失项，保留旧搜索结果上下文保护。
- 本轮没有修改后端模型搜索、渠道保存、权限、账号池、数据库或 relay。

### 验证记录

1. `cd web/default && bun test src/features/channels/lib/model-search.test.ts src/components/multi-select.test.ts src/features/channels/hooks/use-channel-mutate-form.test.ts`：47 个测试通过，覆盖模型搜索展开、MultiSelect 搜索/Enter 行为和渠道表单模型变更逻辑。
2. `cd web/default && bun run typecheck`：通过，确认本轮新增 `matched_count`、`getModelSearchModelNameResult()` 与渠道抽屉调用没有 TypeScript 类型错误。
3. `git diff --check`：通过，确认本轮补丁没有空白格式问题。
4. `cd web/default && bun run i18n:sync`：通过；本轮复用既有 `Scan all search results`、`{{count}} more result(s) will be checked when adding` 等文案，没有新增缺失翻译键。
5. `cd web/default && bun run build`：通过，Rsbuild 生产构建完成。
6. `docker logs --tail 80 nexustok-frontend-watch`：确认热更新容器输出 `[hot] published default dist`。
7. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/`：返回 `HTTP/1.1 200 OK`，`Cache-Version: b688f2fb5be447c25e5aa3bd063087a83db32a288bf6a4f35f2d8db310e40b14`，确认 3003 页面可访问且命中新构建。
8. 使用 Chrome Headless + DevTools Protocol 访问 `http://192.168.0.202:3003/`，在页面上下文调用真实登录接口，账号 `c1cada` 登录成功，`/api/user/self` 返回管理员用户 `role: 100`。
9. 在 3003 页面打开 `/channels`，渠道 `11111` 位于第一页；点击行菜单 `Open menu -> Edit` 后进入 `Edit Channel` 抽屉，模型区初始为 `Selected 3`，供应商收窄显示 `Vendor: OpenAI`。
10. 在编辑渠道模型输入框输入 `gpt-5.6`，页面真实触发 `/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50`；候选完整展示 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`。
11. 搜索 footer 显示 `3 matched · 2 new · 1 already selected`，按钮显示 `Add 2 new model(s)`；本场景没有未扫描分页，因此不会出现 `Scan all search results`。
12. 点击 `Add 2 new model(s)` 后表单草稿变为 `Selected 5`，页面中保留 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，说明已选的 `gpt-5.6-sol` 不会重复追加，缺失的 Terra/Luna 会补齐。
13. CDP 网络记录中没有 `PUT /api/channel` 请求；再次调用 `GET /api/channel/?p=1&page_size=5&tag_mode=false&id_sort=false`，渠道 `11111` 持久化模型仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认搜索追加只影响表单草稿，没有污染运行态配置。
14. 额外调用 `GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50`，返回 `total: 3`、`itemCount: 3`，真实模型集合为 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`。
15. CDP 记录中没有 `Network.loadingFailed`，没有 `console.error` 或 `console.warning`。
16. 当前环境没有暴露浏览器 MCP 工具，本轮按项目规则记录限制，并使用 Chrome Headless + DevTools Protocol、真实 HTTP API、Docker 热更新日志作为替代验证方式。

## 本轮实施评审：编辑渠道页面向 new-api 模型区体验对齐

### 需求分析

用户指出“搜索添加时不正确”，并要求参考 `/opt/project/new-api-main` 最新编辑渠道页面，将当前项目的编辑渠道页面与其对齐。上一轮已经确认模型元信息搜索接口能返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个 OpenAI 模型，并修复了 Enter 键路径；本轮继续聚焦页面体验差异，避免仅修接口而让管理员仍然难以发现和使用批量追加能力。

对比 `new-api-main/web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 与当前实现后，确认分段壳基本一致，主要差异集中在模型区：

1. new-api 的模型快捷操作是平铺按钮，`Fill Related Models`、`Fill All Models`、`Fetch from Upstream`、`Copy All`、`Clear All` 和预设分组入口直接可见；当前项目将一部分操作收进 `More` 菜单，降低了编辑效率。
2. new-api 的模型映射编辑器使用 `Tabs` 在可视化/JSON 间切换，并能提示 JSON 格式和重复源模型风险；当前项目仍是单按钮切换，非法 JSON 和重复映射提示较弱。
3. 当前项目已经具备 new-api 没有的供应商收窄、模型元数据搜索、全分页追加、账号池和细粒度权限控制；本轮应吸收 new-api 的直观交互，而不是覆盖这些 NexusTok 原生能力。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道模型区布局 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 将模型快捷操作从 `More` 菜单改为平铺按钮，并把预设分组恢复为可见二级按钮；保留权限禁用、账号池模式和搜索追加逻辑。 |
| 模型映射编辑器 | `web/default/src/features/channels/components/model-mapping-editor.tsx` | 升级为 `Tabs` 可视化/JSON 双模式，新增重复源模型检测、严格 JSON object 校验和错误提示。 |
| 模型映射测试 | `web/default/src/features/channels/components/model-mapping-editor.test.ts` | 新增纯函数测试，覆盖合法解析、非法 JSON、非字符串目标值、空入口过滤和重复源模型检测。 |
| 前端 i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 补齐模型映射错误提示和删除映射 aria-label 的六语种翻译。 |
| 后端/API/数据库 | 无 | 不修改渠道保存接口、模型搜索接口、模型同步逻辑、数据库结构、relay 或权限后端。 |

### 风险评估

1. 搜索追加回归风险：不能为了对齐 new-api 简化模型选择器而删除 NexusTok 已有的模型元数据搜索和全分页追加。本轮保留 `submitSearchOnEnterWithMatches`、供应商 badge、搜索 footer 和全分页追加逻辑。
2. 权限回归风险：new-api 页面按钮默认假定管理员可操作；NexusTok 已有 `canEditBasicFields`、`permissions.canOperate` 和账号池限制。本轮平铺按钮仍复用原禁用条件和无权限提示。
3. 模型映射数据风险：可视化行最终仍写入 `model_mapping` JSON 字符串，重复入口模型会导致 JSON key 覆盖。本轮用重复检测和非法 sentinel 阻止静默保存。
4. 表单同步风险：可视化模式与 JSON 模式切换时需要保持同一份草稿，外部切换渠道或表单 reset 时也要重新解析当前值。
5. i18n 风险：新增错误提示必须覆盖 en、zh、fr、ja、ru、vi，并通过同步脚本确认没有遗漏。

### 方案评审

采用“保留当前原生能力 + 移植 new-api 可用性”的方案：

1. 模型快捷操作恢复为平铺按钮，使常用操作不再隐藏在菜单中；预设分组用 secondary 小按钮展示，和 new-api 的直接操作体验一致。
2. 模型映射编辑器引入 Base UI `Tabs`，默认显示可视化编辑，JSON 模式保留手工编辑能力。
3. 抽出 `parseModelMappingJson()`、`modelMappingRowsToJson()`、`getDuplicateSources()` 三个纯函数，既便于测试，也让 JSON 约束和可视化行转换规则清晰可维护。
4. 对重复源模型直接显示错误，并向父表单写入非法 sentinel，确保后续保存前的 JSON 校验会阻断提交。
5. 保持所有字段名、保存 payload、后端接口和权限裁剪不变，把风险限定在编辑草稿层。

### 实施结果

已完成编辑渠道页模型区对齐：

- `Fill All Models`、`Copy All`、`Clear All` 和预设分组按钮从 `More` 菜单恢复为可见操作，页面更接近 new-api 最新编辑渠道页的直观布局。
- 模型搜索区域继续显示供应商范围、命中/可新增/已存在统计，并继续支持点击或 Enter 一次性追加搜索命中的多个模型。
- 模型映射编辑器升级为 `Visual / JSON` Tabs；非法 JSON、非 object JSON、非字符串目标值和重复源模型都会给出明确提示。
- 可视化映射行仍支持 datalist 补全当前渠道模型和可选目标模型；空入口模型不会写入 JSON，空目标模型会保留为空字符串。
- 本轮没有修改后端模型同步、渠道保存、模型搜索、账号池、权限矩阵或 relay 行为。

### 验证记录

1. `cd web/default && bun test src/features/channels/components/model-mapping-editor.test.ts src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts src/features/channels/hooks/use-channel-mutate-form.test.ts` 通过，49 个用例全部成功。
2. `git diff --check` 通过。
3. `cd web/default && bunx prettier --check src/features/channels/components/model-mapping-editor.tsx src/features/channels/components/model-mapping-editor.test.ts src/features/channels/components/drawers/channel-mutate-drawer.tsx src/i18n/locales/en.json src/i18n/locales/zh.json src/i18n/locales/fr.json src/i18n/locales/ja.json src/i18n/locales/ru.json src/i18n/locales/vi.json` 通过。
4. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en/zh 缺失为 0，本轮新增键均已覆盖六种语言。fr/ja/ru/vi 仍有历史 untranslated 统计，非本轮新增。
5. `cd web/default && bun run typecheck` 通过。
6. `cd web/default && bun run build` 通过。
7. `docker logs --tail 160 nexustok-frontend-watch` 显示 `[hot] published default dist`。
8. `curl --noproxy '*' -I -L --max-time 15 http://192.168.0.202:3003/` 返回 HTTP 200。
9. Chrome Headless + DevTools Protocol 使用账号 `c1cada` 登录 3003，写入 `localStorage.uid=1` 和完整 `/api/user/self` 用户信息后打开 `http://192.168.0.202:3003/channels`。
10. 编辑渠道 `11111` 后，模型区真实页面显示 `Fill Related Models`、`Fill All Models`、`Copy All`、`Clear All` 平铺按钮，页面文本中不再出现模型快捷操作的 `More` 菜单。
11. 模型映射区真实页面显示 `Visual / JSON` Tabs；点击 `JSON` 后出现 8 行 textarea，placeholder 为 `{"original-model": "replacement-model"}`；切回 `Visual` 后继续显示 `Add Mapping`。
12. 渠道模型区域初始显示 `Selected 3`、`Vendor: OpenAI`，已有模型为 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`。
13. 在模型输入框输入 `gpt-5.6` 后，页面命中 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，并显示 `3 matched` 相关统计。
14. 按 Enter 后页面触发 `GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=100` 全量扫描，未保存草稿变为 `Selected 5`，包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`。
15. Chrome Headless 验证期间未捕获业务保存写请求，特别是没有 `PUT /api/channel`；也未捕获 `Runtime.exceptionThrown`、`Network.loadingFailed` 或 error/warning 级别日志。
16. 验证后通过真实接口重新查询渠道列表，渠道 `11111` 持久化模型仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认页面验证没有污染运行态配置。
17. 当前环境没有暴露浏览器 MCP 工具，本轮继续使用 Chrome Headless + DevTools Protocol、真实 HTTP API 和 Docker 热更新日志作为 MCP 替代验证方式。

## 本轮补充评审：渠道模型搜索追加入口前置

### 需求分析

用户继续确认 `gpt-5.6` 应有三种 OpenAI 模型，并反馈“搜索添加时不正确”。结合 3003 真实接口复查，`GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50` 已返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，而渠道 `11111` 当前持久化模型只有 `gpt-5.6-sol`。因此本轮问题不是后端同步缺失，而是编辑渠道页面让管理员“发现并一次性追加全部搜索结果”的交互仍不够直观：批量追加按钮藏在下拉 footer，管理员容易只点击单个候选或误以为搜索只会处理当前可见的一项。

对照 `/opt/project/new-api-main` 最新编辑渠道页面后，new-api 的优势在于模型配置动作更贴近表单主流程：模型选择、自定义模型录入、快捷填充和映射配置都在同一模型区内直接可见。NexusTok 已经具备供应商收窄、全分页扫描、权限锁和模型映射风险提示，本轮应把这些原生能力用更清晰的入口呈现出来，而不是牺牲已有的搜索补齐能力。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 将模型搜索追加摘要和按钮放入 MultiSelect 下拉 `contentHeader`；恢复独立自定义模型输入组；搜索追加不再被 debounce 关键词差异阻塞。 |
| 模型搜索工具函数 | `web/default/src/features/channels/lib/model-search.ts` | 新增未扫描分页数量纯函数，统一按钮提示和实际扫描语义。 |
| 模型搜索测试 | `web/default/src/features/channels/lib/model-search.test.ts` | 覆盖后端 `total` 大于当前已加载结果时的未扫描数量，以及过期结果/异常 total 的边界。 |
| 后端/API/数据库 | 无 | 不修改模型同步、模型搜索接口、渠道保存 payload、渠道能力表、账号池、权限矩阵、数据库结构或 relay。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案、实施和真实页面验证结果。 |

### 风险评估

1. 误保存风险：页面验证只允许点击搜索追加按钮验证表单草稿，不点击 `Update Channel`；验证后必须重新查 `/api/channel/` 确认运行态模型未变化。
2. 旧搜索污染风险：批量追加会异步扫描全部分页，仍必须保留 request 序号、渠道 ID、关键词和供应商上下文校验，避免旧请求回写当前表单。
3. debounce 竞态风险：用户快速输入后按 Enter 时，不能因为 debounced keyword 尚未刷新而直接提示 `Searching model metadata...` 并丢弃追加动作；追加逻辑应以当前输入框关键词为准重新扫描。
4. 重复模型风险：`gpt-5.6-sol` 已存在于渠道时，追加 Terra/Luna 不能重复写入 Sol；仍使用大小写不敏感去重和 `updateModels(..., true)` 合并草稿。
5. UI 回归风险：自定义模型输入必须复用 `InputGroup`，按钮禁用态和无权限提示仍跟随 `canEditBasicFields`，避免绕过渠道编辑权限。

### 方案评审

采用“把 NexusTok 原生搜索补齐能力前置，而不是复制 new-api 旧表单”的方案：

1. MultiSelect 下拉顶部用 `contentHeader` 固定展示 `Search results`、`matched/new/already selected` 摘要和批量追加按钮，搜索命中后管理员第一眼就能看到一次性追加入口。
2. 下拉 footer 只保留未扫描分页提示、待追加预览和空结果提示，减少顶部按钮和底部按钮重复造成的理解成本。
3. `handleAddModelSearchMatches()` 移除对 debounced keyword 的硬阻塞，点击按钮或按 Enter 时都用当前输入框关键词执行全分页扫描。
4. 独立增加 `Custom model (comma-separated)` 输入组，支持英文逗号、中文逗号和换行批量录入，补齐 new-api 页面里“自定义模型名称 + 填入”的直接录入流。
5. 未扫描分页数量抽成纯函数并加测试，避免 UI 文案和最终追加路径对后端 `total` 的解释分叉。

### 实施结果

- 搜索 `gpt-5.6` 时，下拉顶部直接显示 `Search results`、`3 matched · 2 new · 1 already selected` 和 `Add 2 new model(s)`。
- 点击批量追加后，未保存草稿从 `Selected 3` 变为 `Selected 5`，包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`；已有的 `gpt-5.6-sol` 不会重复写入。
- 独立自定义模型输入支持批量粘贴模型名并通过 `Add Models` 合并到当前草稿，0 新增时会提示 `No new models to add`。
- 搜索追加按钮、Enter 提交和自定义模型输入都继续受 `canEditBasicFields` 控制；本轮没有修改任何后端保存、同步或 relay 逻辑。

### 验证记录

1. `GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50` 返回 `total: 3`，items 为 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，确认供应商确为 OpenAI 且模型库三项完整。
2. `GET /api/channel/?p=0&page_size=5` 确认渠道 `11111` 初始持久化模型为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`。
3. 访问 `http://192.168.0.202:3003/` 返回 `HTTP/1.1 200 OK`，`Cache-Version: b688f2fb5be447c25e5aa3bd063087a83db32a288bf6a4f35f2d8db310e40b14`。
4. Chrome Headless + DevTools Protocol 登录账号 `c1cada`，打开 `http://192.168.0.202:3003/channels`，通过行菜单进入渠道 `11111` 的 `Edit Channel` 抽屉。
5. 真实页面显示新增的 `Custom model (comma-separated)` 输入组和 `Add Models` 按钮，证明热更新页面已生效。
6. 在模型搜索输入 `gpt-5.6` 后，下拉顶部显示 `Search results`、`3 matched · 2 new · 1 already selected` 和 `Add 2 new model(s)`；候选列表完整展示 Terra/Luna/Sol 三项。
7. 点击 `Add 2 new model(s)` 后页面 toast 显示 `Added 2 model(s) from search`，表单草稿变为 `Selected 5`，包含 Terra/Luna/Sol。
8. 验证过程中未点击 `Update Channel`；再次调用 `GET /api/channel/?p=0&page_size=5`，渠道 `11111` 持久化模型仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`。
9. `cd web/default && bun test src/features/channels/lib/model-search.test.ts src/components/multi-select.test.ts` 通过，45 个用例成功。
10. `cd web/default && bun run i18n:sync` 通过。
11. `cd web/default && bun run typecheck` 通过。
12. `cd web/default && bun run build` 通过。
13. 定向 ESLint 对本轮渠道文件通过，`src/components/multi-select.tsx` 仅保留 5 个既有 `react-refresh/only-export-components` warning。
14. 当前环境没有暴露浏览器 MCP 工具，本轮按项目规则记录限制，并使用 Chrome Headless + DevTools Protocol、真实 HTTP API 作为替代验证方式。

## 本轮实施评审：Authz 用户角色分配原生化

### 需求分析

前序差异分析已经确认，NexusTok 已完成权限 catalog、路由级 enforcement、用户级 allow/deny override、`authz_roles` / `casbin_rule` 持久策略、角色模板 CRUD、角色策略矩阵 UI、策略导入导出和 Casbin shadow 对比，但仍缺少“把自定义角色模板真正分配给某个 Admin 用户”的闭环。`new-api-main` 的优势在于角色模板不只是可编辑配置，还能作为用户授权基线进入运行时判定。

本轮目标是把“用户角色分配”转为 NexusTok 原生能力：Root 可以在用户编辑页为系统 Admin 选择内置 Admin 基线或启用的自定义角色模板；运行时 `Can()` / `CapabilitiesForUser()` 先读取用户绑定的 `authz_role` 作为基线，再叠加用户级 override。普通用户和 Root 用户不参与该字段分配，避免把模板系统扩展成系统角色提升入口。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 用户模型 | `model/user.go` | 增加 `authz_role` 字段，作为 Admin 用户授权角色模板引用；资料编辑路径仍不直接写该字段。 |
| Authz 运行时 | `service/authz/permission.go`、`service/authz/role_assignment.go`、`service/authz/override.go` | `Can()` / `CapabilitiesForUser()` 读取用户绑定角色；用户级 override 改为相对绑定角色基线计算；清理授权时同时清空 `authz_role`。 |
| 角色模板保护 | `service/authz/role_policy.go`、`service/authz/policy_import.go` | 已分配给用户的自定义角色禁止删除、禁用或通过 replace 导入移除，避免已分配 Admin 的基线突然失效。 |
| 用户接口 | `controller/user.go`、`controller/user_authz.go` | `/api/user/self` 返回 `authz_role`；创建/更新用户时只允许 Root 设置 Admin 用户的授权角色和权限矩阵；字段省略时保留原角色，显式空值时清回内置 Admin。 |
| 用户编辑页 | `web/default/src/features/users/components/users-mutate-drawer.tsx`、`api.ts`、`lib/user-form.ts`、`types.ts`、`stores/auth-store.ts` | Root 编辑 Admin 用户时加载可分配角色模板，展示 `Role baseline` 选择器，并把权限矩阵保存为相对所选基线的 override。 |
| 前端 i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 补齐角色基线选择、加载失败和说明文案。 |
| 测试 | `controller/user_authz_test.go`、`service/authz/*_test.go`、`web/default/src/features/users/lib/user-form.test.ts` | 覆盖角色分配、字段省略/清空、已分配角色保护、stale session fail-closed 和前端 payload 语义。 |

### 风险评估

1. 权限提升风险：`authz_role` 只能分配给系统 Admin 用户，且只允许 Root 设置；内置 Root、`role:*`、`user:*` 和禁用/不存在模板均被拒绝，避免通过 payload 绕过系统角色边界。
2. 失效回退风险：如果绑定角色被手工删除、禁用或数据库不可读，运行时不会回退到完整 Admin，而是 fail-closed，避免“模板失效 = 权限放大”。
3. 角色删除风险：删除、禁用或策略 replace 导入会改变已分配用户基线；本轮在事务内检查 `users.authz_role`，存在引用时拒绝操作。
4. 资料编辑回归风险：用户资料编辑仍只更新用户名、显示名、分组、备注和密码，`authz_role` 由 Authz service 单独写入，避免普通资料保存绕过授权校验。
5. 前端误保存风险：权限 catalog 未加载时前端不提交 `authz_role` 和 `admin_permissions`，避免用不完整矩阵覆盖后端现有设置。
6. 性能风险：`Can()` 现在需要读取用户绑定角色以防 stale session 越权；当前主要影响管理路由授权路径，后续若有明显热点再评估缓存与失效策略。

### 方案评审

采用“系统角色不变，授权基线可分配”的方案：

1. 数据层只新增 `users.authz_role`，不新增用户-角色关联表；空值和 `admin` 都表示沿用内置 Admin 基线。
2. `authz_role` 只指向启用的非内置自定义角色模板；Root 不可分配，Admin 内置基线用空字符串表达。
3. 运行时授权先校验数据库中该用户当前系统角色，防止旧 token 仍携带更高角色；随后用绑定模板的持久策略作为基线，再叠加用户级 override。
4. 用户级 override 保存时改为相对当前绑定基线计算，只存差异，保持 `authz_user_overrides` / `casbin_rule user:<id>` 镜像语义一致。
5. 前端仅在 Root 编辑 Admin 用户且权限 catalog 可用时展示角色基线选择器；选择模板时立即把矩阵切换到该模板 grants，让管理员看到“基线 + 覆盖”的实际保存结果。
6. 已分配模板的删除、禁用和策略 replace 删除全部拒绝，先要求管理员迁移用户到其他基线。

### 实施结果

- `users.authz_role` 已成为 Admin 用户授权基线字段；`/api/user/self` 和用户详情接口能返回该字段。
- Root 可以通过用户编辑页给 Admin 用户选择内置 Admin 基线或启用的自定义角色模板；保存时会把内置 Admin 规范化为空字符串，自定义模板写入规范化小写 key。
- `Can()` / `CapabilitiesForUser()` 会读取用户当前数据库角色和 `authz_role`，旧 session 中角色高于数据库当前角色时会按数据库角色收紧。
- 用户级权限 override 现在相对绑定角色基线保存，避免从自定义基线切换后继续按内置 Admin 计算差异。
- 自定义角色如果已被用户引用，删除、禁用和导入 replace 移除都会返回稳定错误，不会留下悬空授权基线。
- 降级 Admin、清理授权和创建普通用户路径会同步清空历史 `authz_role` 与 override。

### 验证记录

1. `go test ./service/authz ./controller -run 'Test(ImportPersistentPolicies|PersistentRoles|CreateRoleTemplate|DeleteRoleTemplate|UpdateRoleTemplate|UpdateRolePolicies|GetSelf|GetUser|UpdateUser|CreateUser|Demote|UserManageAction|AssignedAuthzRole)'` 通过，覆盖角色分配、用户接口、模板保护和策略导入保护。
2. `go test ./model -run 'Test(UserEdit|AuthzUser|UserDelete)'` 通过，确认用户资料编辑和用户删除清理没有回归。
3. `cd web/default && bun test src/features/users/lib/user-form.test.ts src/features/system-settings/security/role-policy-utils.test.ts` 通过，新增 4 个用户表单授权角色 payload 用例，总计 7 个用例成功。
4. `cd web/default && bun run i18n:sync` 通过；本轮新增角色基线文案已同步到 en、zh、fr、ja、ru、vi。
5. `go test ./service/authz ./controller ./middleware ./router -run 'Authz|Permission|Role|User'` 通过，覆盖授权服务、用户接口、中间件和路由门禁的关键回归。
6. `git diff --check` 通过，确认本轮变更没有空白或补丁格式问题。
7. 3003 真实接口验证：登录 `http://192.168.0.202:3003/` 后调用 `/api/user/self`，返回 `authz_role: ""` 和完整 `permissions.admin_permissions`；调用 `/api/authz/roles` 返回 Root/Admin 内置角色、`runtime_managed=true` 与 grants 矩阵，确认后端热更新已生效。
8. 3003 前端发布验证：访问首页返回 200，静态前端包 `/static/js/index.js` 已包含 `Role baseline` 和 `Built-in Admin baseline` 文案，确认用户编辑页角色基线 UI 已随热更新发布。
9. 当前环境没有暴露浏览器 MCP 工具，本轮按项目规则记录限制，并使用真实 HTTP API、Chrome Headless/CDP 与已发布前端包探针作为替代验证方式。

## 本轮实施评审：编辑渠道页保存语义继续对齐 new-api 最新页

### 需求分析

用户继续反馈“搜索添加时不正确”，并要求参考 `/opt/project/new-api-main` 最新编辑渠道页面继续对齐。重新对照后确认，NexusTok 当前渠道编辑抽屉已经具备 new-api 的主干结构：`Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings` 分区、左侧步骤导航、模型快捷操作和模型映射编辑器均已落地；NexusTok 还保留了账号池组模式、Codex OAuth、细粒度权限和模型元数据搜索追加等原生增强。

本轮真实复现显示，3003 当前页面在模型输入框搜索 `gpt-5.6` 时已经完整展示 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，点击 `添加 2 个新模型` 后未保存草稿会从 `已选 3 个` 变为 `已选 5 个`，搜索追加主路径可用。继续对齐 new-api 最新源代码后发现两个确定缺口：Codex(type 57) 页面显示 OpenAI 兼容字段透传开关，但保存 settings 时没有持久化这些开关；更新 payload 也缺少 new-api 的 Base URL 去尾斜杠和多个 nullable 文本字段显式清空语义。同时，模型搜索 Enter 键在有高亮候选时仍可能被批量追加接管，容易让管理员本想选择单个候选却触发批量补齐。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道表单转换 | `web/default/src/features/channels/lib/channel-form.ts` | Codex(type 57) 保存 OpenAI 兼容字段透传开关；创建/更新 payload 规范化 Base URL；更新时显式发送可清空文本字段。 |
| MultiSelect 键盘行为 | `web/default/src/components/multi-select.tsx` | 搜索弹层已有高亮候选时，Enter 保留给候选选择；没有高亮但存在搜索命中/加载中时，仍允许渠道模型搜索提交批量追加。 |
| 测试 | `web/default/src/features/channels/lib/channel-form.test.ts`、`web/default/src/components/multi-select.test.ts` | 覆盖 Codex 透传保存、历史字段清理、Base URL 规范化、显式清空字段和 Enter 高亮候选保护。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和验证结果。 |

### 风险评估

1. 账号池组模式风险：NexusTok 的 `global_account_pool` 是本项目原生能力，不能被 new-api 的普通 key/base_url 语义覆盖；本轮仅在保存时显式清空本地 `base_url` 并保留 `global_account_pool` 哨兵 key。
2. Codex 设置回归风险：type 57 走 OpenAI 兼容请求链路，保存透传开关应与页面可见控件一致；非 OpenAI 兼容渠道仍需清理历史透传字段，避免跨 provider 污染。
3. 清空字段风险：更新 payload 显式发送空字符串会让管理员清空旧值；这与 new-api 最新页一致，但必须只覆盖 nullable 文本字段，不动 key 的“空值不覆盖旧密钥”保护。
4. 搜索追加风险：不能破坏已经验证的显式按钮批量追加 Terra/Luna/Sol；只调整有高亮候选时的 Enter 语义，保留无高亮 Enter 批量追加和自定义模型创建路径。
5. 运行态数据风险：3003 页面验证只修改未保存表单草稿，不点击 `更新渠道`，避免污染渠道 `11111` 的真实模型配置。

### 方案评审

采用“小范围语义补齐”的方案，不重写渠道编辑抽屉：

1. 在 `buildSettingsJSON()` 中把 type 57 纳入 OpenAI 兼容字段透传保存范围，持久化 `allow_service_tier`、`disable_store`、`allow_safety_identifier`、`allow_include_obfuscation` 和 `allow_inference_geo`。
2. 新增前端 `normalizeBaseUrl()`，创建和更新渠道时去除 Base URL 首尾空白和末尾 `/`，与 new-api 最新页保持一致。
3. 更新渠道时显式发送 `base_url`、`openai_organization`、`test_model`、`tag`、`remark`、`model_mapping`、`status_code_mapping`、`param_override`、`header_override` 的空字符串，确保 UI 清空能写回后端；`key` 仍只在账号池组模式或用户显式输入时提交。
4. `MultiSelect` 在 Enter 捕获阶段检测弹层内是否有 `[data-highlighted]` 候选；有高亮时不接管，让 Base UI 执行候选选择；没有高亮但搜索有命中或仍在加载时，继续执行渠道模型搜索提交。

### 实施结果

- Codex 渠道编辑页可见的 OpenAI 兼容字段透传开关现在能进入 `settings` payload，不再出现“页面能点、保存丢失”的不一致。
- 非 OpenAI/Anthropic/Codex 渠道会清理历史字段透传开关，避免旧 settings 在切换 provider 后继续误生效。
- 渠道创建和更新 payload 会规范化 Base URL；更新 payload 可显式清空组织、测试模型、标签、备注、模型映射和覆写 JSON 字段。
- 模型搜索弹层有高亮候选时 Enter 不再触发批量追加；显式点击 `添加 2 个新模型` 仍会批量补齐缺失模型。

### 验证记录

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/channel-form.test.ts src/features/channels/lib/model-search.test.ts src/features/channels/hooks/use-channel-mutate-form.test.ts` 通过，69 个用例成功。
2. `cd web/default && bunx prettier --check src/components/multi-select.tsx src/components/multi-select.test.ts src/features/channels/lib/channel-form.ts src/features/channels/lib/channel-form.test.ts` 通过。
3. `cd web/default && bun run typecheck` 通过。
4. `cd web/default && bun run build` 通过。
5. `git diff --check` 通过。
6. 3003 真实页面验证：访问 `http://192.168.0.202:3003/` 返回 200，`nexustok-frontend-watch` 日志出现 `[hot] published default dist`，确认本轮前端热更新已发布。
7. Chrome Headless + DevTools Protocol 登录账号 `c1cada`，打开 `http://192.168.0.202:3003/channels`，通过行菜单进入渠道 `11111` 的编辑抽屉；初始模型区为 `已选 3 个`，包含 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`。
8. 在模型输入框搜索 `gpt-5.6`，页面显示 `搜索结果`、`命中 3 个 · 可新增 2 个 · 已存在 1 个`、`添加 2 个新模型`，候选完整包含 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`。
9. 键盘路径验证：搜索后按 ArrowDown 高亮 `gpt-5.6-terra`，再按 Enter 只选择该候选，草稿变为 `已选 4 个`，不会误触发批量追加。
10. 显式按钮路径验证：重新打开编辑抽屉搜索 `gpt-5.6`，点击 `添加 2 个新模型` 后草稿变为 `已选 5 个`，包含 Terra/Luna/Sol 三项。
11. 验证期间未点击 `更新渠道`；随后调用 `GET /api/channel/?p=1&page_size=5&tag_mode=false&id_sort=false`，渠道 `11111` 持久化模型仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认页面验证没有污染运行态配置。
12. 当前环境没有暴露浏览器 MCP 工具，本轮继续使用 Chrome Headless + DevTools Protocol、真实 HTTP API 和 Docker 热更新日志作为 MCP 替代验证方式。

## 本轮实施评审：渠道列表桌面卡片视图原生化

### 需求分析

继续复核 `new-api-main` 默认前端后确认，最新版渠道列表已经在桌面端支持表格/卡片视图切换，管理员可以在宽屏上用更紧凑的卡片扫 provider、状态、余额、响应时间、优先级、权重和分组。NexusTok 当前公共 `DataTablePage` 已经有 `enableCardView`、`viewModeStorageKey`、`renderCard` 和卡片网格底座，并已先在模型元数据表试点，但 `/channels` 渠道列表仍只使用桌面表格 + 移动端卡片。

本轮目标不是复制 new-api 的整个 DataTable `core/layout/toolbar` 目录，也不改渠道后端、筛选接口、编辑抽屉或状态操作，而是把 new-api 的“渠道桌面卡片视图”优势转换成 NexusTok 原生能力：复用现有 `ChannelCard`、列 cell renderer、批量操作和筛选状态，让渠道页也具备可持久化的桌面表格/卡片切换。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道移动/桌面卡片 | `web/default/src/features/channels/components/channel-card.tsx` | 将既有 `ChannelCard` 导出给桌面卡片视图复用；移动端 `ChannelsMobileList` 行为不变。 |
| 渠道列表页面 | `web/default/src/features/channels/components/channels-table.tsx` | 在 `DataTablePage` 上开启 `enableCardView`，使用 `channels-table-view-mode` 持久化桌面视图模式，并通过 `renderCard` 复用 `ChannelCard`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新 DataTable 渐进分层状态，并记录本轮需求、风险、方案、实施和验证结果。 |

### 风险评估

1. 桌面表格工作流风险：渠道页是高频管理页，表格仍必须作为默认视图，避免管理员已有列宽、排序、批量选择和操作习惯被打断。本轮仅新增可选卡片视图，默认 `table` 视图不变。
2. 业务逻辑重复风险：如果重写一套渠道卡片，容易漏掉账号池、multi-key、IO.NET、通行体警告、状态 tooltip、优先级/权重 inline 控件和分组 badge。本轮复用既有 `ChannelCard`，内部继续通过 `flexRender` 调用列 cell renderer。
3. 移动端回归风险：移动端已经有专用 `ChannelsMobileList`，不应被桌面卡片网格替换。本轮 `mobile` slot 保持原样，`enableCardView` 只在非 mobile 时生效。
4. 禁用渠道样式风险：禁用/异常渠道在桌面表格和移动卡片已有左侧边框/背景提示。本轮继续通过 `getRowClassName` 把同一禁用样式传给卡片网格。
5. 热更新环境风险：本轮验证时发现热更新后端扫描和前端构建进程曾卡在内核 `wait_on_buffer` / `blk_mq_get_tag`，导致本地 `tsc -b` 无法稳定完成；因此验证以 Prettier、`git diff --check`、Docker 热更新构建发布、真实 3003 页面 CDP 操作为准，并在记录中保留受限项。

### 方案评审

采用最小接入方案：

1. 将 `ChannelCard` 从 `channel-card.tsx` 导出，不移动文件、不拆分目录，避免触发大范围 import churn。
2. 在 `ChannelsTable` 中引入 `ChannelCard`，为 `DataTablePage` 增加 `enableCardView`、`viewModeStorageKey='channels-table-view-mode'`、三列桌面卡片网格和 `renderCard`。
3. `ChannelCard` 外层由 `DataTableCardGrid` 提供边框、选中态和禁用态，传入 `className='px-0 py-0'` 避免双层内边距；卡片内部仍保留原来的信息层级。
4. 不改变 column definitions、筛选、分页、排序、批量操作、移动端列表、权限按钮或后端 API。

### 实施结果

- `/channels` 桌面端现在和 `/models/metadata` 一样具备表格/卡片视图切换。
- 视图选择写入 `localStorage.channels-table-view-mode`，刷新后可保留管理员偏好。
- 卡片内容复用既有 `ChannelCard`，保留 provider/type、名称/备注、状态、优先级、权重、余额、响应时间、最近测试时间、分组、行操作和选择框。
- 移动端继续走 `ChannelsMobileList`，没有改变手机端布局。
- 本轮继续推进 DataTable 渐进分层，但仍避免一次性迁移 new-api-main 的 `core/layout/static/hooks` 全目录。

### 验证记录

1. `cd web/default && ./node_modules/.bin/prettier --check src/features/channels/components/channel-card.tsx src/features/channels/components/channels-table.tsx` 通过。
2. `git diff --check` 通过。
3. `nexustok-frontend-watch` 日志显示 Rsbuild 完成并输出 `[hot] published default dist`，说明本轮 JSX/TS 变更已被热更新构建链路编译并发布。
4. 访问 `http://192.168.0.202:3003/` 返回 `HTTP/1.1 200 OK`，后端热更新容器在重启后完成数据库迁移并输出 `NexusTok ready`。
5. 发布产物中已确认存在 `enableCardView:!0`、`viewModeStorageKey:"channels-table-view-mode"` 和卡片网格配置，证明 3003 加载的渠道页 chunk 包含本轮改动。
6. Chrome Headless + DevTools Protocol 真实页面验证：登录 `c1cada` 成功，打开 `http://192.168.0.202:3003/channels` 后存在视图切换控件；点击卡片视图按钮后，按钮状态从 `[false,true]` 变为 `[true,false]`，`localStorage.channels-table-view-mode` 为 `card`，DOM 中出现 `[data-slot="data-table-card"]` 卡片节点，桌面表格节点数为 0。
7. `cd web/default && bun run typecheck` 曾多次启动，但 `tsc -b` 卡在内核 `wait_on_buffer` / `folio_wait_bit_common` 超过数分钟，已终止以避免继续阻塞；本轮以已通过的 Prettier、`git diff --check`、容器 Rsbuild 热更新构建和真实页面验证作为替代证据。

## 本轮实施评审：前端构建元数据与缓存版本清理原生化

### 需求分析

`new-api-main` 的 `frontend-cache.ts` 和 `build-metadata.ts` 提供了两个小但很实用的运维能力：前端版本变化时清理旧 UI 缓存，并把当前构建版本写到 `window`、`html data-*`、`meta[name="build-id"]`、CSS 变量和 localStorage 中。NexusTok 当前每次修改后都要求访问 `http://192.168.0.202:3003/` 确认页面更新，但缺少浏览器运行态层面的构建指纹，排查“热更新是否真的加载新包”时仍要靠手动查 chunk 或容器日志。

本轮目标是把这两个能力转换为 NexusTok 原生能力：使用 NexusTok 专属 key 和全局对象名，不复用 `newapi:*` 或 `app:rev`；同时保守处理 localStorage，避免误清登录态、语言、系统状态、通知已读、表格视图、渠道模式和 Playground 草稿。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 前端启动入口 | `web/default/src/main.tsx` | 在应用渲染前安装构建元数据并执行缓存版本同步；不改变 React Router、QueryClient、认证和品牌刷新逻辑。 |
| 构建元数据 | `web/default/src/lib/build-metadata.ts` | 新增 `window.__NEXUSTOK_BUILD__`、`data-build-rev`、`data-app-channel`、`meta[name="build-id"]`、`--app-build-rev` 和 `localStorage.nexustok:default:app-rev`。 |
| 缓存版本清理 | `web/default/src/lib/frontend-cache.ts` | 新增 `nexustok:default:cache-version`，版本变化时清理未保留的 UI 缓存，保留关键用户状态和偏好。 |
| 测试 | `web/default/src/lib/build-metadata.test.ts`、`web/default/src/lib/frontend-cache.test.ts` | 覆盖版本号计算、DOM/window/meta/CSS/localStorage 写入、版本一致不清理、版本变化清理未知缓存并保留关键状态。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 更新前端缓存差异状态并记录本轮评审。 |

### 风险评估

1. 登录态风险：如果清理 `user` 或 `uid`，用户刷新后可能被迫重新登录；本轮明确保留。
2. 语言和系统状态风险：如果清理 `i18nextLng` 或 `status`，首屏语言和系统名可能闪烁；本轮明确保留。
3. 管理员偏好风险：渠道 tag mode、ID sort、表格/卡片视图、设置页 accordion、仪表盘偏好、通知已读和 Playground 草稿都属于有价值工作状态；本轮保留这些 key 或前缀。
4. 旧品牌缓存风险：历史 `system_name`、`logo`、`footer_html` 等已由当前系统状态和固定随包 logo 接管，继续保留反而可能污染首屏；本轮会被版本清理移除。
5. 浏览器存储不可用风险：隐私模式、受限 iframe 或浏览器策略可能让 localStorage 写入抛错；两个初始化函数都使用 best-effort，不阻断应用启动。
6. 观测命名冲突风险：不能使用 new-api 的 `window.__APP_BUILD__` 或 `app:rev`，避免未来嵌入、迁移或调试脚本混淆；本轮使用 `__NEXUSTOK_BUILD__` 和 `nexustok:default:*`。

### 方案评审

采用独立工具模块 + 启动入口接入方案：

1. `build-metadata.ts` 计算 `rv.<envVersion|0000>.nexustok-default`，并把同一个 revision 写入多层观测位置。
2. `frontend-cache.ts` 以 `default-v1` 作为首个 NexusTok 默认前端缓存版本，只有版本不一致时才扫描 localStorage 并清理非保留项。
3. 保留名单按 NexusTok 当前代码实际使用的 key 制定，包括认证、语言、系统状态、通知、DataTable 视图、渠道模式、Dashboard、Playground、系统设置折叠状态和模型定价列偏好。
4. `main.tsx` 在 QueryClient 和 React 渲染前执行两个初始化函数，使构建指纹和缓存清理尽早生效。
5. 测试使用纯内存 storage 和最小 fake document，不引入 JSDOM 或新依赖。

### 实施结果

- 运行中的默认前端现在可以通过 `window.__NEXUSTOK_BUILD__`、`document.documentElement.dataset.buildRev`、`meta[name="build-id"]`、`--app-build-rev` 和 `localStorage['nexustok:default:app-rev']` 观测构建版本。
- 首次升级到该版本后，`localStorage['nexustok:default:cache-version']` 会写入 `default-v1`。
- 版本变化时会清理未知 UI 缓存和旧品牌缓存，但保留登录、语言、系统状态、通知已读、表格视图、渠道模式、Dashboard、Playground 和系统设置偏好。
- 应用启动在 localStorage 或 DOM 写入失败时仍继续渲染。

### 验证记录

1. `cd web/default && bun test src/lib/frontend-cache.test.ts src/lib/build-metadata.test.ts` 通过，4 个用例成功。
2. `cd web/default && ./node_modules/.bin/prettier --check src/lib/frontend-cache.ts src/lib/frontend-cache.test.ts src/lib/build-metadata.ts src/lib/build-metadata.test.ts src/main.tsx` 通过。
3. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
4. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.1 秒内完成生产构建。
5. `git diff --check` 通过。
6. 使用 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/` 成功，根文档、主 JS/CSS、`/api/status`、`/api/setup`、`/api/notice`、`/api/home_page_content` 和 `logo.png` 请求均返回 200。
7. MCP 浏览器上下文读取到运行态构建指纹：`window.__NEXUSTOK_BUILD__.rev`、`html[data-build-rev]`、`meta[name="build-id"]`、`--app-build-rev` 和 `localStorage['nexustok:default:app-rev']` 均为 `rv.0000.nexustok-default`，`html[data-app-channel]` 为 `nexustok-default`，`localStorage['nexustok:default:cache-version']` 为 `default-v1`。
8. MCP 控制台仅有 i18next 提示和 `[nexustok-build] rv.0000.nexustok-default` debug 日志，未发现 error 级别异常。

## 本轮实施评审：渠道编辑页搜索添加与 new-api 最新编辑体验对齐

### 需求分析

用户反馈“搜索添加时不正确”，并明确指出 `gpt-5.6` 有 3 种模型但编辑渠道时只同步/添加了一个。经运行态接口复核，`/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50` 当前返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个 OpenAI 模型，后端数据和供应商过滤本身是正确的；问题集中在编辑渠道页面的模型多选交互上：Base UI Combobox 会高亮第一个候选，按 Enter 时容易被解释为“选择当前高亮候选”，从而只把第一个模型加入草稿，而不是执行“添加全部搜索命中”的操作。

本轮目标是继续吸收 `new-api-main` 最新渠道编辑页的优势：分区清楚、状态导航明确、模型区操作语义直接；同时保留 NexusTok 已有的原生能力，包括账号池凭证模式、Codex OAuth、权限裁剪、模型映射风险提示、模型元数据搜索、分页全量扫描和多语言支持。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 共享多选组件 | `web/default/src/components/multi-select.tsx` | 新增默认关闭的 `submitSearchOnEnterWhenHighlighted`，允许特定场景把 Enter 统一解释为搜索提交；默认行为不变。 |
| 多选组件测试 | `web/default/src/components/multi-select.test.ts` | 覆盖默认高亮候选仍保留单项选择、渠道显式开启后高亮候选也提交搜索追加。 |
| 渠道编辑页模型区 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | Codex/OpenAI 渠道模型搜索中开启该显式开关；按钮文案改为 `Add all {{count}} new match(es)`；页脚展示 `Will add:`，强调批量追加全部新命中。 |
| 前端国际化 | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增三条模型搜索批量追加文案翻译。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案、实施和验证。 |

### 风险评估

1. 共享组件回归风险：`MultiSelect` 被多个页面复用，若直接改变 Enter 语义可能影响普通多选；本轮新增开关并默认关闭，只在渠道模型搜索调用点开启。
2. 单项选择风险：管理员仍可能需要单独点选一个候选；本轮只改变渠道模型搜索里的 Enter 批量语义，鼠标点选候选仍保留单项选择能力。
3. 重复模型风险：批量追加必须继续按大小写不敏感方式识别已有模型；本轮沿用 `getMissingModelSearchMatches`、`dedupeModelNames` 和 `mergeModelNames`。
4. 旧异步结果污染风险：搜索追加会扫描分页结果，输入、渠道或供应商变化后旧请求不应写入当前表单；本轮继续保留 `isModelSearchAppendContextCurrent` 和请求序号保护。
5. 权限风险：没有渠道普通写/敏感写权限时不能修改模型草稿；本轮没有放宽 `canEditBasicFields` 检查。
6. 国际化风险：新增文案必须覆盖 en、zh、fr、ja、ru、vi；本轮运行 `bun run i18n:sync` 并补齐六种语言。

### 方案评审

采用最小但明确的原生化方案：

1. 在 `MultiSelect` 增加 `submitSearchOnEnterWhenHighlighted`，让“是否覆盖高亮候选 Enter 行为”由调用方显式决定。
2. 渠道模型选择器传入 `submitSearchOnEnterWithMatches` 与 `submitSearchOnEnterWhenHighlighted`，因此输入 `gpt-5.6` 后按 Enter 走 `handleAddModelSearchMatches`，会扫描全部分页搜索结果并批量追加所有尚未选择的模型。
3. 保留 `allowCreateWithMatches={false}`，避免管理员把 `gpt-5.6` 这类系列前缀误创建为自定义模型；真正无候选的自定义模型仍可通过自定义模型输入区添加。
4. 将按钮文案从“Add N new model(s)”调整为“Add all N new match(es)”，并在页脚显示 `Will add:`，与 new-api 最新编辑页清晰、直给的操作表达对齐。
5. 不修改后端 `/api/models/search`，因为运行态验证表明其已正确返回 3 个 OpenAI 模型。

### 实施结果

- 渠道编辑页模型搜索现在可以正确把 `gpt-5.6` 的全部新命中加入模型草稿；已有模型不会重复加入。
- 当渠道已经有 `gpt-5.6-sol` 时，搜索 `gpt-5.6` 会显示 `3 matched · 2 new · 1 already selected`，按钮显示 `Add all 2 new match(es)`，页脚列出 `Will add: gpt-5.6-terra, gpt-5.6-luna`。
- 按 Enter 不再被第一个高亮候选截走，而是执行批量搜索追加；鼠标点选候选仍然可用于单项选择。
- 编辑页继续保持 new-api 最新版的分区式体验：顶部 provider/状态、左侧/顶部分区导航、基础信息、凭证、模型与分组、高级设置、Quick actions 和底部固定操作区；同时保留 NexusTok 的账号池、Codex、权限和模型映射增强。

### 验证记录

1. 运行态接口验证：`curl` 登录后请求 `http://192.168.0.202:3003/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50` 返回 3 个模型，分别为 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`，`vendor_id` 均为 1。
2. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts` 通过，47 个用例成功。
3. `cd web/default && bun run i18n:sync` 通过；同步报告显示所有语言 `missingCount` 为 0，本轮新增 key 已写入 en、zh、fr、ja、ru、vi。
4. `cd web/default && ./node_modules/.bin/prettier --check src/components/multi-select.tsx src/components/multi-select.test.ts src/features/channels/components/drawers/channel-mutate-drawer.tsx src/i18n/locales/en.json src/i18n/locales/zh.json src/i18n/locales/fr.json src/i18n/locales/ja.json src/i18n/locales/ru.json src/i18n/locales/vi.json` 通过。
5. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
6. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.2 秒内完成生产构建。
7. `git diff --check` 通过。
8. 使用 Chrome DevTools MCP 登录 `c1cada`，访问 `http://192.168.0.202:3003/channels`，主 bundle `index.50d7f1e4a6.js` 中可检出 `Add all {{count}} new match(es)` 和 `before adding all matches`，确认页面加载了本轮更新后的资源。
9. MCP 打开渠道 `11111` 的编辑抽屉，模型区显示 provider 为 OpenAI，当前模型包含 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`；输入 `gpt-5.6` 后页面显示 `3 matched · 2 new · 1 already selected`、按钮 `Add all 2 new match(es)`、页脚 `Will add: gpt-5.6-terra, gpt-5.6-luna`。
10. MCP 在同一输入框按 Enter 后，草稿模型列表变为 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`，并出现 `Added 2 model(s) from search` 提示；未点击保存按钮，因此未改写后端渠道数据。
11. MCP 网络记录显示 `/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50` 与分页扫描请求 `page_size=100` 均返回 200；控制台未出现 error 级别异常。DevTools 仍提示两个既有表单可访问性 issue（autocomplete/label），本轮未扩大到该独立问题。

## 本轮实施评审：渠道编辑页模型入口二次对齐 new-api 最新体验

### 需求分析

用户继续反馈“搜索添加时不正确”，并指出最新版 `new-api-main` 的编辑渠道页面体验更好，希望 NexusTok 的编辑渠道页向该页面对齐。上一轮已经修复了 `gpt-5.6` 搜索时 Enter 被 Base UI Combobox 高亮候选截走的问题，但当前 NexusTok 模型区仍同时存在两个入口：上方模型 `MultiSelect` 负责搜索、选择和搜索批量追加，下方独立的 `Custom model (comma-separated)` 输入框负责自定义添加。这个布局会让“搜索添加”和“自定义添加”两个概念挤在一起，用户容易把下方 `Add Models` 理解为搜索结果添加按钮。

本轮目标是继续吸收 `new-api-main` 最新编辑页的优势：保持单一、稳定、分区明确的渠道编辑体验；把模型搜索、已有模型选择和自定义模型创建统一回到一个模型输入控件内；保留 NexusTok 已有的模型元数据搜索、供应商过滤、分页全量扫描、账号池凭证模式、Codex 渠道能力和权限控制。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 抽屉布局 | `web/default/src/components/drawer-layout.ts` | 给侧边抽屉表单滚动容器补充 `overflow-x-hidden`，避免窄屏出现底部横向滚动条。 |
| Combobox 芯片控件 | `web/default/src/components/ui/combobox.tsx` | 给芯片容器、单个芯片和芯片输入补充 `min-w-0` / `max-w-full` / 收缩 basis，确保模型名多时仍在容器内稳定换行或收缩。 |
| 通用 MultiSelect | `web/default/src/components/multi-select.tsx` | 让芯片标签可收缩并保持 truncate；继续保留搜索提交和自定义创建能力。 |
| 渠道编辑页 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 移除独立自定义模型输入框与 `Add Models` 按钮；渠道模型选择器开启 `hideSelectedOptionsWhenSearching`，搜索时聚焦展示尚未选择的命中项。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、风险、方案、实施和验证。 |

### 风险评估

1. 自定义模型能力丢失风险：移除独立输入框不能导致管理员无法添加自定义模型；本轮保留 `MultiSelect allowCreate`，无候选时输入模型名并按 Enter 仍可创建，自带逗号/中文逗号/换行批量解析逻辑。
2. 搜索批量追加回归风险：`gpt-5.6` 仍必须按 OpenAI 供应商搜索到 3 个模型，并只追加渠道尚未包含的 `terra/luna`；本轮继续复用上一轮分页扫描、去重和上下文防污染逻辑。
3. 共享 UI 回归风险：`Combobox` / `MultiSelect` 是共享组件，布局收缩不能破坏其它页面；本轮只调整容器尺寸约束和文本 truncate，不改变默认选中、删除和搜索行为。
4. 窄屏布局风险：抽屉内部既有移动端顶部导航，又有芯片式模型输入，容易制造横向滚动；本轮同时在抽屉表单和芯片控件层面限制横向溢出。
5. 权限风险：模型草稿更新仍必须受 `canEditBasicFields` 控制；本轮没有放宽任何权限判断。
6. 热更新验证风险：3003 可能存在旧缓存；本轮访问 `http://192.168.0.202:3003/` 后重新登录并打开 `/channels`，以运行态页面为准验证改动已生效。

### 方案评审

采用“保留原生能力、收敛入口、限制溢出”的方案：

1. 保留 `MultiSelect` 的 `allowCreate`、逗号拆分、远程搜索受控输入、Enter 搜索批量追加、搜索结果分页扫描和大小写不敏感去重。
2. 移除渠道编辑页模型区下方独立的自定义模型小表单，让模型添加只有一个主入口，和 `new-api-main` 最新编辑页的简洁模型区对齐。
3. 渠道模型搜索时开启 `hideSelectedOptionsWhenSearching`，避免搜索下拉把已经选择的模型和可新增模型混在一起；统计区仍显示 `matched/new/already selected`，保持“3 个模型都已同步到模型库”的可见性。
4. 对抽屉表单和芯片组件增加宽度收缩约束，修复 780px 窄宽度下底部横向滚动条和芯片撑宽问题。
5. 不修改后端接口、渠道保存 payload、账号池/Codex 逻辑和模型映射风险保护，避免影响核心业务稳定性。

### 实施结果

- 渠道编辑页模型区删除了 `Custom model (comma-separated)` 和独立 `Add Models` 按钮，模型搜索、选择、搜索批量追加和自定义模型创建统一在 `Select models or add custom ones` 控件内完成。
- 搜索 `gpt-5.6` 时仍显示 `3 matched · 2 new · 1 already selected`，并通过 Enter 追加 `gpt-5.6-terra`、`gpt-5.6-luna`，不会重复已有的 `gpt-5.6-sol`。
- 780px 宽度下，页面 body 不再出现横向滚动；抽屉和芯片输入的 `scrollWidth` 与 `clientWidth` 保持在容器约束内，模型芯片稳定展示。
- NexusTok 特有的账号池凭证、Codex 渠道、权限裁剪、模型映射风险提示和 Quick actions 均保留。

### 验证记录

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts` 通过，47 个用例成功。
2. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en/zh/fr/ja/ru/vi 的 `missingCount` 均为 0。本轮未新增文案，历史 `untranslatedCount` 仍存在于 fr/ja/ru/vi，未在本轮扩大处理。
3. `cd web/default && bunx prettier --write src/components/drawer-layout.ts src/components/ui/combobox.tsx src/components/multi-select.tsx src/features/channels/components/drawers/channel-mutate-drawer.tsx` 已格式化本轮触碰文件；全量 `bun run format:check` 仍会因仓库既有多处无关格式差异失败，本轮未批量改写无关文件。
4. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
5. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.5 秒内完成生产构建。
6. `git diff --check` 通过。
7. 使用 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/`，登录 `c1cada` 成功，运行态构建标识仍为 `rv.0000.nexustok-default`；随后打开 `/channels` 并编辑渠道 `11111`。
8. MCP 页面验证：编辑抽屉模型区不再出现 `Custom model (comma-separated)`；当前模型为 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`；输入 `gpt-5.6` 后显示 `3 matched · 2 new · 1 already selected`、按钮 `Add all 2 new match(es)` 和 `Will add: gpt-5.6-terra, gpt-5.6-luna`。
9. MCP 按 Enter 后，前端草稿显示 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna`，并出现 `Added 2 model(s) from search` 提示；未点击保存。
10. MCP 接口复核 `/api/channel/1` 返回的持久化 `models` 仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认验证过程没有污染后端渠道数据。
11. MCP 窄屏验证：将视口调整为 `780x620`，`document.body.scrollWidth === document.body.clientWidth === 780`，抽屉 `overflowX` 为 `hidden`，模型芯片容器 `scrollWidth === clientWidth`；视觉截图显示模型区无底部横向滚动条。
12. MCP 网络记录显示 `/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50` 与分页扫描请求 `page_size=100` 均返回 200；控制台无 error，仅有 i18next 信息、构建 debug 日志和既有表单可访问性 issue。

## 本轮实施评审：Waffo Pancake 管理端凭据复用语义原生化

### 差异来源

对比 `/opt/project/new-api-main/web/default/src/features/system-settings/integrations/waffo-pancake-settings-section.tsx` 与当前 NexusTok 实现后确认，`new-api-main` 在 Waffo Pancake 管理端页面中有一个值得吸收的细节：由于 `GET /api/option/` 不回显 `PrivateKey`，返回管理员打开页面时如果没有重新粘贴私钥，catalog/pair 辅助接口会发送空凭据，让后端回退到已保存的 `OptionMap` 凭据；如果管理员正在输入新凭据，则必须 `Merchant ID` 和 `API Private Key` 成对填写。

NexusTok 已经具备更多原生能力，包括 `Enable Waffo Pancake`、沙箱模式、生产/测试 webhook key、钱包充值 Store/Product、币种、单价、最低充值、订阅套餐 product 管理和 Authz 权限控制，因此本轮不照搬 `new-api-main` 的整页受控表单结构，只吸收其“私钥不回显时避免混合态请求”的优势语义。

### 需求分析

当前 NexusTok 的 Waffo Pancake 设置页在点击 `Refresh catalog` 时，如果私钥为空会走 `GET /api/option/waffo-pancake/catalog`，能够复用已保存凭据；但点击 `Create store + product` 时会把当前表单里的 `Merchant ID` 和空 `Private Key` 一并提交给 `/api/option/waffo-pancake/pair`。后端 `resolveWaffoPancakeAdminCreds` 的契约是“二者都空表示复用已保存凭据；只传其中一个表示不完整临时凭据并失败”，因此这个前端行为会让已保存私钥的管理员刷新页面后无法直接创建默认 Store/Product。

本轮目标是把 `new-api-main` 的凭据读取策略原生化到 NexusTok：未编辑凭据且已有 Merchant ID 时发送空 payload 复用后端已保存值；一旦编辑 Merchant ID 或 Private Key，就要求两者同时填写；首次配置时两者都为空则在前端给出明确错误提示，避免无意义请求。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| Waffo Pancake 凭据解析 | `web/default/src/features/system-settings/integrations/waffo-pancake-credentials.ts` | 新增纯函数 `resolveWaffoPancakeAdminCredentials`，集中实现“未编辑复用已保存凭据、编辑时成对提交”的判断。 |
| Waffo Pancake 页面 | `web/default/src/features/system-settings/integrations/waffo-pancake-settings-section.tsx` | `Refresh catalog` 和 `Create store + product` 调用同一凭据解析；无效输入在前端 toast 提示，不再发请求。 |
| 单元测试 | `web/default/src/features/system-settings/integrations/waffo-pancake-credentials.test.ts` | 覆盖未编辑已保存、完整新凭据、只填 Merchant ID、只填 Private Key 四类边界。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增 `API Private Key is required` 与 `Merchant ID and API Private Key are required` 六语言翻译。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮差异来源、需求、风险、方案、实施和验证。 |

本轮不修改 Go 后端、数据库迁移、支付下单、webhook、订阅支付、Waffo 聚合支付、Creem、Stripe、Epay 和系统设置其它 section。

### 风险评估

1. 外部资源副作用风险：点击 `Create store + product` 会真实创建上游 Pancake Store/Product。MCP 验证阶段通过浏览器 XHR mock 捕获请求并返回模拟结果，没有向后端/上游发起创建请求，也没有点击保存。
2. 已保存凭据回归风险：如果管理员已保存 Merchant ID 和 Private Key，但页面私钥为空，本轮必须发送 `{ merchant_id: "", private_key: "" }` 让后端复用已保存值；该路径由纯函数单测覆盖。
3. 首次配置体验风险：首次配置时空表单点击刷新或创建不能静默失败；本轮新增双字段必填提示，且不会发出无效网络请求。
4. 临时凭据安全风险：临时 Private Key 仍只通过 POST JSON body 发送，不进入 URL query；本轮保留现有 `listWaffoPancakeCatalog` 在编辑凭据时走 POST、未编辑时走 GET 的语义。
5. 权限风险：`Refresh catalog` 仍受 `permissions.canOperate` 控制，`Create store + product` 仍受 `canUpdateWaffoPancakeSettings` 控制；没有放宽 Authz 权限。
6. 翻译风险：新增文案已补齐 en、zh、fr、ja、ru、vi，并运行 `bun run i18n:sync`，避免运行时缺 key。

### 方案评审

采用“抽纯函数、组件复用、保留 NexusTok 原生配置”的方案：

1. 新增 `resolveWaffoPancakeAdminCredentials(defaults, values)`，只依赖当前默认值和表单值，返回 `payload`、`edited`、`ready` 和缺失字段标记。
2. `edited=false` 且默认 Merchant ID 存在时，payload 固定为 `{ merchant_id: "", private_key: "" }`，对应后端的已保存凭据回退契约。
3. `edited=true` 时，payload 使用表单 trim 后的 Merchant ID / Private Key；任何单边缺失都会在前端 toast 并阻止请求。
4. `Refresh catalog` 仅在 `edited=true` 时传临时凭据；未编辑时继续调用 `GET /catalog`，保留已保存凭据只读回访路径。
5. `Create store + product` 也复用同一解析结果，修复刷新页面后“已有 Merchant ID + 空 Private Key”导致 pair 创建失败的问题。
6. 不把 `new-api-main` 的受控 payment 大表单迁入 NexusTok，因为 NexusTok 已经有独立保存按钮、更多配置字段、权限系统和订阅 product 原生能力。

### 实施结果

- Waffo Pancake 管理端 catalog/pair 两条辅助路径统一使用凭据状态解析，避免混合态请求。
- 首次配置时，空凭据点击 `Refresh catalog` 会提示 `Merchant ID and API Private Key are required`，不会调用 `/api/option/waffo-pancake/*`。
- 只填 `Merchant ID` 不填 `API Private Key` 时提示 `API Private Key is required`，不会发出请求。
- 填完整临时凭据时，catalog 请求体为 `{"merchant_id":"...","private_key":"..."}`；pair 请求体额外包含去尾斜杠后的 `return_url`。
- 保留 NexusTok 的启用/沙箱/webhook key/价格/最低充值/Store/Product/订阅套餐 product 管理和 Authz 权限，不覆盖当前项目原生能力。

### 验证记录

1. `cd web/default && bun test src/features/system-settings/integrations/waffo-pancake-credentials.test.ts` 通过，4 个用例成功。
2. `cd web/default && bun test src/features/system-settings/integrations/waffo-pancake-credentials.test.ts src/features/system-settings/utils/numeric-field.test.ts` 通过，7 个用例成功。
3. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en、zh、fr、ja、ru、vi 的 `missingCount` 与 `extrasCount` 均为 0。本轮新增 key 已写入六种语言；fr/ja/ru/vi 的 `untranslatedCount` 为历史统计，本轮未扩大该问题。
4. `cd web/default && bunx prettier --write src/features/system-settings/integrations/waffo-pancake-credentials.ts src/features/system-settings/integrations/waffo-pancake-credentials.test.ts src/features/system-settings/integrations/waffo-pancake-settings-section.tsx src/i18n/locales/en.json src/i18n/locales/zh.json src/i18n/locales/fr.json src/i18n/locales/ja.json src/i18n/locales/ru.json src/i18n/locales/vi.json` 已格式化本轮触碰文件。
5. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
6. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.2 秒内完成生产构建。
7. `git diff --check` 通过。
8. 使用 Chrome DevTools MCP 登录 `c1cada`，访问 `http://192.168.0.202:3003/system-settings/billing/payment`，Waffo Pancake 区域正常渲染。
9. MCP 调用 `/api/option/` 回查运行态，当前 `WaffoPancakeMerchantID`、`WaffoPancakeStoreID`、`WaffoPancakeProductID`、`WaffoPancakeReturnURL` 均为空，确认没有已保存 Pancake 配置可供真实创建测试使用。
10. MCP 空表单点击 `Refresh catalog`，页面显示 `Merchant ID and API Private Key are required`，`performance` 资源记录中 `/api/option/waffo-pancake/*` 请求增量为 0。
11. MCP 只填写 Waffo Pancake 的 `Merchant ID=MER_runtime_check` 后点击 `Refresh catalog`，页面显示 `API Private Key is required`，`/api/option/waffo-pancake/*` 请求增量仍为 0。
12. MCP 在浏览器上下文 mock `XMLHttpRequest`，填写 `Merchant ID=MER_payload_check`、`API Private Key=KEY_payload_check` 后点击 `Refresh catalog`，捕获到 `POST /api/option/waffo-pancake/catalog`，请求体为 `{"merchant_id":"MER_payload_check","private_key":"KEY_payload_check"}`，mock 响应后页面显示 `No stores found`。
13. MCP 继续 mock `POST /api/option/waffo-pancake/pair`，填写 `Merchant ID=MER_pair_check`、`API Private Key=KEY_pair_check`、`Payment return URL=https://example.com/console/topup/` 后点击 `Create store + product`，捕获请求体为 `{"merchant_id":"MER_pair_check","private_key":"KEY_pair_check","return_url":"https://example.com/console/topup"}`，证明返回地址已去除尾斜杠。
14. MCP pair mock 响应后，页面表单草稿填入 `STO_mocked` / `PROD_mocked` 并显示成功提示；验证过程未点击 `Save Waffo Pancake settings`。
15. MCP 再次调用 `/api/option/` 回查运行态，`WaffoPancakeMerchantID`、`WaffoPancakeStoreID`、`WaffoPancakeProductID`、`WaffoPancakeReturnURL` 仍为空，确认页面验证没有污染后端配置。
16. MCP 控制台无 error/warn；仅剩既有表单可访问性 issue：autocomplete、label 与无关联 label 提示，本轮没有扩大该独立问题。

## 本轮实施评审：渠道模型搜索添加键盘语义对齐 new-api 编辑页

### 差异来源

用户继续反馈渠道编辑页“搜索添加时不正确”，并要求参考 `/opt/project/new-api-main` 最新编辑渠道页面继续对齐。重新对照 `/opt/project/new-api-main/web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 后确认，`new-api-main` 的模型区以模型多选器为主入口：管理员在同一个控件中搜索、选择已有模型或添加自定义模型，批量能力主要通过 `Fetch Models` 弹窗等明确动作完成。

NexusTok 当前已经在 new-api 主结构上原生化了更多能力，包括 OpenAI/Codex 供应商范围提示、模型元信息库搜索、`Search results` 摘要、缺失模型预览和 `Add all {{count}} new match(es)` 显式批量追加按钮。MCP 真实复现显示，搜索 `gpt-5.6` 时接口返回并渲染了 `gpt-5.6-terra`、`gpt-5.6-luna` 两个可新增候选，点击显式按钮会把草稿从 `Selected 3` 正确追加到 `Selected 5`；但之前模型输入框还把 Enter 绑定到批量追加，导致“选择高亮候选”和“追加全部命中”两种动作挤在同一个键盘路径里，容易让用户误以为搜索添加行为不稳定。

### 需求分析

本轮目标不是移除 NexusTok 的模型库搜索优势，而是把它转换成更符合 new-api 编辑页直觉的交互：模型输入框负责搜索、选择具体候选和自定义添加；批量追加全部搜索命中必须通过 `Add all {{count}} new match(es)` 按钮显式触发。这样搜索 `gpt-5.6` 时，管理员仍能看到 3 个已同步模型、2 个可新增模型和 1 个已选模型，但按 Enter 不再绕过候选选择语义直接批量追加。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道编辑页模型区 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 移除模型 MultiSelect 的 `onSearchSubmit` 和两个 Enter 批量提交开关；保留搜索结果 header/footer 和显式批量按钮。 |
| 通用 MultiSelect | `web/default/src/components/multi-select.tsx` | 增加有搜索词时的 Enter 父表单提交防护；候选选择、自定义创建和显式搜索提交仍按原有分支处理。 |
| 单元测试 | `web/default/src/components/multi-select.test.ts` | 补充搜索词 Enter 不冒泡提交父表单的纯函数覆盖。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮复现、需求、风险、方案和验证。 |

本轮不修改 Go 后端、`/api/models/search`、模型同步、渠道保存 payload、账号池、Codex OAuth、权限裁剪、模型映射 guardrail 和上游模型检测配置。

### 风险评估

1. 批量追加能力丢失风险：显式按钮仍调用原来的 `handleAddModelSearchMatches`，继续扫描分页、按供应商过滤、按大小写不敏感去重，并只追加当前渠道尚未包含的模型。
2. 键盘习惯变化风险：Enter 不再触发“追加全部搜索命中”，但这正是本轮要消除的歧义；候选选择和自定义模型创建回归更接近 new-api 的常规输入语义。
3. 共享组件回归风险：`MultiSelect` 本轮只在“有搜索词且 Enter 未被其它分支消费”时阻止父表单提交；不会改变候选点击、Chip 删除、逗号批量创建和显式搜索提交。
4. 页面布局风险：不调整抽屉结构、分区顺序、Quick actions、模型映射和 Groups 区域，因此不会回退已经对齐 new-api 的页面主结构。
5. 权限风险：显式按钮仍受 `canEditBasicFields` 和原有 disabled 条件控制，没有放宽编辑权限。

### 方案评审

采用“显式批量、键盘回归选择”的最小方案：

1. 保留 `Search results` 顶部摘要和 `Will add` 底部预览，让管理员继续清楚看到搜索命中、可新增和已存在数量。
2. 保留 `Add all {{count}} new match(es)` 按钮，作为唯一批量追加入口。
3. 移除 `onSearchSubmit={handleAddModelSearchMatches}`、`submitSearchOnEnterWithMatches` 和 `submitSearchOnEnterWhenHighlighted`，避免 Enter 与候选选择争抢语义。
4. 在通用 `MultiSelect` 中补充 Enter 防护：当输入框仍有搜索词，且没有高亮候选选择、自定义创建或显式搜索提交时，阻止事件冒泡为父表单提交。
5. 不修改 `allowCreateWithMatches={false}`，搜索存在真实候选时仍不显示 `Add custom model "gpt-5.6"`，避免把模型族前缀误写成真实模型。
6. 不修改后端和保存链路，确保本轮只影响前端草稿交互。

### 实施结果

- 渠道模型搜索仍会请求 `/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50`，并将真实候选合并进模型选择器。
- 搜索 `gpt-5.6` 时页面继续展示 `3 matched · 2 new · 1 already selected`，候选列表包含 `gpt-5.6-terra`、`gpt-5.6-luna`，页脚显示 `Will add: gpt-5.6-terra, gpt-5.6-luna`。
- 批量追加只能通过 `Add all 2 new match(es)` 按钮触发；点击后仍正确追加 `terra/luna`，不重复 `gpt-5.6-sol`。
- 搜索输入里按 Enter 不再触发 `PUT /api/channel/` 父表单提交，避免在未点击 `Update Channel` 时产生无意义保存请求。
- 编辑渠道页继续保持 new-api 风格的 `Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings` 分区，并保留 NexusTok 的账号池、Codex、权限控制和模型搜索补齐能力。

### 验证记录

1. 已运行 `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts`，49 个用例通过，覆盖通用多选 Enter 防护、模型搜索过滤、去重、供应商范围和分页提示。
2. 已运行 `cd web/default && bun run i18n:sync`，`en` / `zh` 无缺失、无额外、无未翻译；`fr`、`ja`、`ru`、`vi` 无缺失和额外，本轮未新增翻译债务。
3. 已运行 `cd web/default && bun run typecheck` 与 `cd web/default && bun run build`，类型检查和生产构建通过。
4. 已运行 `git diff --check`，未发现空白错误。
5. 已使用 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/channels`，在真实页面中打开渠道 `11111` 编辑抽屉；初始模型为 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`，页面显示 `Selected 3` 和 `Vendor: OpenAI`。
6. 在模型输入中搜索 `gpt-5.6`，页面显示 `Search results`、`3 matched · 2 new · 1 already selected`、显式按钮 `Add all 2 new match(es)`，页脚预览 `Will add: gpt-5.6-terra, gpt-5.6-luna`。
7. 搜索结果展开时按 Enter 后，页面仍为 `Selected 3`，没有出现 `Added 2 model(s) from search`，网络记录只有 `GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50`，未产生 `PUT /api/channel/`。
8. 再次搜索 `gpt-5.6` 并点击 `Add all 2 new match(es)` 后，页面出现 `Added 2 model(s) from search`，草稿模型变为 `Selected 5`，新增 `gpt-5.6-terra` 与 `gpt-5.6-luna`，未重复 `gpt-5.6-sol`。
9. 验证过程中没有点击 `Update Channel`；通过 `GET /api/channel/1` 回查后端持久化模型仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认页面验证没有污染运行态渠道配置。
10. MCP 网络面板中本轮相关请求均为 GET：`/api/models/search` 的 `page_size=50` 交互搜索、`page_size=100` 显式批量扫描，以及用于验证的 `/api/channel/1` 回查；未出现渠道保存写请求。
11. MCP 控制台无 error/warn；仅保留既有 i18next info、`[nexustok-build] rv.0000.nexustok-default` debug，以及既有 autocomplete / label 可访问性 issue，本轮没有扩大该独立问题。

## 本轮再评审：渠道编辑页搜索添加恢复一致批量动作

### 差异来源

用户再次反馈编辑渠道页“搜索添加时不正确”，并要求继续参考 `/opt/project/new-api-main` 最新编辑渠道页面。重新查看运行态页面和 new-api 源码后确认，当前 NexusTok 已经继承了 new-api 编辑抽屉的主要结构：右侧大抽屉、左侧分段导航、`Basic Information` / `Credentials` / `Models & Groups` / `Advanced Settings` 分区，以及模型区的卡片化信息组织。但 NexusTok 在模型区额外实现了模型元信息搜索、供应商收窄、搜索结果摘要和批量追加按钮，这些属于当前项目的原生增强能力。

MCP 真实复现显示：渠道 `11111` 初始模型为 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`；搜索 `gpt-5.6` 时，接口返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个 OpenAI 模型，页面也正确显示 `3 matched · 2 new · 1 already selected`。点击 `Add all 2 new match(es)` 可以正确追加 `terra/luna`，但在搜索框按 Enter 会清空搜索并关闭候选层，没有执行添加动作。这会让“搜索后添加”出现按钮路径和键盘路径不一致的问题。

### 需求分析

本轮目标是把 NexusTok 的模型搜索增强继续打磨成稳定的原生能力：搜索结果区仍保持 new-api 风格的清晰分区和显式按钮；同时，用户在搜索框中输入模型族前缀后按 Enter，也应执行与 `Add all {{count}} new match(es)` 相同的批量追加动作。这样既不丢失 new-api 编辑页的简洁结构，也满足 NexusTok 对模型库搜索、分页扫描和批量补齐的增强需求。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道编辑页模型区 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 仅在渠道模型搜索 MultiSelect 调用点恢复 `onSearchSubmit={handleAddModelSearchMatches}`，并开启高亮候选时 Enter 也执行批量追加。 |
| 通用 MultiSelect | `web/default/src/components/multi-select.tsx` | 不修改默认行为；继续通过已有可选 props 控制搜索提交语义。 |
| 测试 | `web/default/src/components/multi-select.test.ts`、`web/default/src/features/channels/lib/model-search.test.ts` | 继续复用已有纯函数覆盖：默认高亮候选保留给单项选择，显式开启后高亮候选 Enter 也提交搜索追加。 |

本轮不修改 Go 后端、`/api/models/search`、模型同步数据、渠道保存 payload、账号池、Codex OAuth、权限裁剪、模型映射 guardrail、上游模型检测配置和数据库结构。

### 风险评估

1. 共享组件回归风险：`MultiSelect` 被多个页面复用，本轮不改默认 props，只在渠道模型搜索调用点开启 Enter 批量追加，避免影响普通多选框。
2. 单项选择歧义风险：在渠道模型搜索中，Enter 会优先执行“添加全部搜索命中”，但鼠标点击具体候选仍可单独添加一个模型；显式按钮也保留，便于用户确认追加范围。
3. 自定义模型误创建风险：继续保留 `allowCreateWithMatches={false}`，搜索存在真实候选时不会把 `gpt-5.6` 这样的模型族前缀误写成自定义模型。
4. 异步搜索污染风险：批量追加仍走 `handleAddModelSearchMatches`，保留请求序号、抽屉打开状态、渠道 ID、关键词和供应商校验，避免旧请求写入当前表单。
5. 权限风险：批量追加仍受 `canEditBasicFields` 控制，普通无权用户不会获得模型编辑能力。

### 方案评审

采用“按钮与 Enter 同源”的最小修复方案：

1. 保留当前 new-api 风格页面骨架和 NexusTok 模型搜索增强，不重写抽屉布局。
2. 渠道模型 MultiSelect 传入 `onSearchSubmit={handleAddModelSearchMatches}`，让搜索框 Enter 复用显式按钮的同一业务函数。
3. 同时传入 `submitSearchOnEnterWithMatches` 和 `submitSearchOnEnterWhenHighlighted`，避免 Base UI Combobox 高亮第一个候选时截走 Enter，只添加一个模型或清空搜索。
4. 不改变 `MultiSelect` 默认值，普通表单仍维持“高亮候选 Enter 选择单项”的常规行为。
5. 后续验证以 3003 运行态页面为准：搜索 `gpt-5.6` 后按 Enter 应从 `Selected 3` 变为 `Selected 5`，并且不点击 `Update Channel`，确保不污染后端配置。

### 实施结果

- 渠道编辑页模型 MultiSelect 已恢复 `onSearchSubmit={handleAddModelSearchMatches}`，并仅在该调用点开启 `submitSearchOnEnterWithMatches` 与 `submitSearchOnEnterWhenHighlighted`。
- 搜索 `gpt-5.6` 时，页面仍显示 `3 matched · 2 new · 1 already selected`、`Add all 2 new match(es)` 和 `Will add: gpt-5.6-terra, gpt-5.6-luna`。
- 在搜索框按 Enter 会复用显式按钮同一追加函数，触发分页扫描并把尚未选择的 `gpt-5.6-terra`、`gpt-5.6-luna` 加入前端草稿。
- 通用 MultiSelect 默认行为未改变；只有渠道模型搜索入口启用该批量提交语义。
- 验证过程没有点击 `Update Channel`，后端渠道持久化模型保持不变。

### 验证记录

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts` 通过，49 个用例成功，覆盖搜索过滤、Enter 提交开关、模型搜索去重和供应商范围。
2. `cd web/default && bun run i18n:sync` 通过，本轮没有新增翻译 key。
3. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
4. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 生产构建成功。
5. `git diff --check` 通过，未发现空白错误。
6. 使用 Chrome DevTools MCP 先访问 `http://192.168.0.202:3003/`，根页面正常加载；随后进入 `/channels`，打开渠道 `11111` 的编辑抽屉。
7. MCP 页面验证：初始模型为 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`，模型区显示 `Selected 3` 和 `Vendor: OpenAI`，页面结构仍为 new-api 风格的 `Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings` 分区。
8. MCP 在模型输入中搜索 `gpt-5.6`，页面显示 `3 matched · 2 new · 1 already selected`、按钮 `Add all 2 new match(es)`，页脚预览 `Will add: gpt-5.6-terra, gpt-5.6-luna`。
9. MCP 直接按 Enter 后出现 `Added 2 model(s) from search`，前端草稿变为 `Selected 5`，新增 `gpt-5.6-terra` 与 `gpt-5.6-luna`，未重复已有 `gpt-5.6-sol`。
10. MCP 点击 `Cancel` 后通过 `GET /api/channel/1` 回查，后端持久化 `models` 仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认没有误保存渠道。
11. MCP 网络记录显示本轮相关请求均为 GET：`/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50`、Enter 批量扫描 `page_size=100` 和 `/api/channel/1` 回查；未出现 `PUT /api/channel/`。
12. MCP 控制台无 error/warn；仅有既有 i18next 信息、`[nexustok-build] rv.0000.nexustok-default` debug，以及既有 autocomplete / label 可访问性 issue。

## 本轮实施评审：性能指标摘要趋势原生化

### 差异来源

对照 `/opt/project/new-api-main` 后确认，new-api 的性能摘要能力不仅返回模型整体成功率，还会为模型卡片提供近期成功率趋势，并在摘要层过滤无效分组。NexusTok 已有 `perf_metrics` 采样、详情查询和模型广场展示，但摘要接口只返回整体平均值，前端模型卡片也只能显示单个状态柱；如果历史数据来自已删除分组，摘要层还可能把不可见分组的请求量带入模型广场和仪表盘。

### 需求分析

本轮目标是把 new-api 的优势转换为 NexusTok 原生可观测能力：摘要接口按当前有效分组和 `auto` 分组聚合，返回每个模型最近 3 个时间桶的成功率；前端统一成功率等级、颜色和状态点映射，让模型广场、模型详情和仪表盘使用同一套健康语义。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 后端摘要 DTO | `pkg/perf_metrics/types.go` | `ModelSummary` 新增 `recent_success_rates`，兼容旧客户端的可选字段。 |
| 后端聚合查询 | `model/perf_metric.go`、`pkg/perf_metrics/metrics.go` | 新增按 `model_name,bucket_ts` 的摘要聚合，支持按有效分组过滤，并从数据库桶和内存热桶合并最近 3 个成功率点。 |
| 摘要控制器 | `controller/perf_metrics.go` | `/api/perf-metrics/summary` 传入当前有效分组和 `auto`，避免隐藏分组污染摘要。 |
| 前端可观测格式化 | `web/default/src/features/performance-metrics/lib/format.ts` | 新增共享成功率等级、文本颜色、状态点颜色和图表颜色 helper。 |
| 前端性能展示 | `web/default/src/features/dashboard/*`、`web/default/src/features/pricing/components/*` | 仪表盘、模型卡片、模型详情和 uptime sparkline 统一使用共享成功率语义；模型卡片使用 `recent_success_rates` 展示最近 3 段状态。 |
| 测试 | `pkg/perf_metrics/metrics_test.go`、`web/default/src/features/performance-metrics/lib/format.test.ts` | 覆盖有效分组过滤、最近 3 个成功率点、空分组结果和前端成功率等级。 |

### 风险评估

1. 数据库兼容风险：新增查询继续使用 GORM 聚合，并使用 `clause.IN` 生成分组过滤，避免手写 `group` 保留字 SQL，保持 SQLite、MySQL、PostgreSQL 兼容。
2. 热路径风险：不改 `perf_metrics` 表结构，不改采样写入和 relay 热路径，只改摘要读取与前端展示。
3. 分组过滤风险：`groups=nil` 仍表示不过滤，空切片表示返回空结果；controller 实际传入 `当前 group ratio + auto`，避免误删自动路由分组。
4. 前端兼容风险：`recent_success_rates` 为可选字段，缺失时模型卡片回退到整体 `success_rate`。
5. 视觉回归风险：颜色不照搬 new-api 的 raw Tailwind，而是使用 NexusTok 语义 token，保持当前主题能力。

### 方案评审

采用“后端摘要补趋势、前端共享语义”的方案：

1. 后端聚合从“按模型一次汇总”调整为“按模型和时间桶汇总后再合并”，同时产出总量和最近桶成功率。
2. 内存热桶和数据库历史桶走同一合并函数，保证当前小时未落库数据也能参与趋势。
3. `recentSuccessRates` 只暴露成功率数组，不暴露内部请求量和 bucket 时间戳，减少前端耦合。
4. 前端把成功率等级集中到 `performance-metrics/lib/format.ts`，删除仪表盘和模型详情里的重复阈值函数。
5. 模型卡片只展示最多 3 个状态柱，缺失趋势时用单个整体成功率回退，避免无数据页面出现空白状态。

### 实施结果

- `/api/perf-metrics/summary` 返回的模型摘要支持 `recent_success_rates`，有数据时包含最近 3 个时间桶的成功率。
- 摘要层过滤有效分组，已删除/隐藏分组的历史性能数据不会继续影响模型广场排序和仪表盘健康度。
- 模型广场性能徽章可展示 3 段近期健康趋势；无趋势数据时继续使用整体成功率。
- 仪表盘、模型详情和 uptime sparkline 统一使用 `excellent/good/warning/critical/unknown` 成功率等级。
- 前端性能格式测试改为项目统一的 `node:test`，解除 `tsc -b` 对 `bun:test` 类型的依赖。

### 验证记录

1. `go test ./pkg/perf_metrics ./model ./controller -run 'TestQuerySummaryAll|TestGetPerfMetrics|TestAuthzShadow|TestClickHouse|TestBuildLogLikeCondition'` 通过。
2. `cd web/default && bun test src/features/performance-metrics/lib/format.test.ts src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts` 通过，51 个用例成功。
3. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
4. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 生产构建成功。
5. `git diff --check` 通过，未发现空白错误。
6. 使用 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/`，根页面正常加载。
7. MCP 在浏览器上下文调用 `GET /api/perf-metrics/summary?hours=24`，返回 HTTP 200 且 `success=true`；当前环境 24 小时无性能采样，`models` 数量为 0，空态合理。
8. MCP 打开 `/dashboard/models`，页面正常显示 `No performance data available` 空态，没有渲染错误。
9. MCP 打开 `/pricing`，模型广场正常渲染 3 个模型卡片；由于当前无性能摘要数据，性能徽章保持无数据回退状态。
10. MCP 控制台无 error/warn；仅有既有 i18next 信息、构建 debug 和表单可访问性 issue，本轮没有扩大该独立问题。

## 本轮实施评审：渠道编辑页与 new-api 最新编辑页视觉结构对齐

### 差异来源

用户继续反馈“搜索添加时不正确”，并指出 `/opt/project/new-api-main` 最新编辑渠道页面观感更好，要求当前项目的编辑渠道页面向其对齐。重新对照 `/opt/project/new-api-main/web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 与当前 `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 后确认：

1. NexusTok 已经吸收了 new-api 的主干编辑结构：右侧大抽屉、固定头部和底部、左侧分段导航、`Basic Information` / `Credentials` / `Models & Groups` / `Advanced Settings` 四段表单、模型区卡片、模型映射编辑器和高级配置折叠区。
2. NexusTok 在此基础上增加了原生能力：账号池凭证模式、Codex OAuth 授权、细粒度权限、模型元信息库搜索、供应商范围提示、搜索结果摘要、分页扫描批量追加和模型映射 guardrail。这些不能用 new-api 文件整段覆盖，否则会回退当前项目核心能力。
3. MCP 在 3003 运行态页面复测显示，当前搜索 `gpt-5.6` 已能正确显示 `3 matched · 2 new · 1 already selected`；点击 `Add all 2 new match(es)` 或在搜索框按 Enter 都会把草稿从 `Selected 3` 追加到 `Selected 5`，新增 `gpt-5.6-terra` 与 `gpt-5.6-luna`，不重复已有 `gpt-5.6-sol`。
4. 当前仍存在页面对齐问题：桌面抽屉左侧导航宽度偏窄，英文 `Advanced Settings` 被截断；`Credentials` 在 NexusTok 的账号池/Codex 场景下呈现为裸表单，缺少 new-api 编辑页里凭证配置区域的卡片边界，视觉层级不如模型区清晰。

### 需求分析

本轮目标是把 new-api 最新编辑页的可读性和结构优势继续原生化到 NexusTok，而不是删除 NexusTok 已经实现的增强能力。具体需要：

1. 保持搜索添加的正确批量行为：搜索 `gpt-5.6` 时必须能基于 OpenAI 供应商范围追加全部缺失模型，按钮和 Enter 共享同一逻辑。
2. 提升编辑抽屉左侧导航可读性：分区名称不能因固定宽度被无意义截断，尤其是英文界面下的 `Advanced Settings`。
3. 对齐 new-api 凭证配置区域的卡片化结构：`Credentials` 下的账号池、API Key、多 Key、Codex OAuth 等表单应在同一个边界内组织，和 `Models & Groups` 的卡片层级一致。
4. 保留当前项目原生能力：账号池模式、权限禁用、Codex OAuth、模型搜索批量追加、模型映射校验、上游模型检测和高级设置导航都必须继续可用。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道编辑页布局 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`、`web/default/src/features/channels/components/drawers/sections/channel-auth-section.tsx` | 调整桌面导航列宽和导航标题换行策略；为 `Credentials` 内容增加卡片化容器，并让 `Authentication` 子段在卡片首项时不显示多余顶部分隔线；字段顺序和业务逻辑不变。 |
| 搜索添加行为 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`、`web/default/src/components/multi-select.tsx` | 不改变已验证正确的批量追加函数和 MultiSelect 默认行为，只通过页面复测确认按钮与 Enter 路径仍一致。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 追加本轮需求分析、影响范围、风险评估、方案评审和后续验证记录。 |

本轮不修改 Go 后端、数据库、模型同步接口、`/api/models/search`、渠道保存 payload、账号池后端、Codex 凭证刷新、计费和模型价格能力。

### 风险评估

1. 视觉回归风险：导航列变宽会减少右侧表单宽度；采用小幅增加并保持 `minmax(0,1fr)`，避免移动端和窄桌面出现横向溢出。
2. 表单结构风险：为 `Credentials` 增加容器可能影响滚动定位和间距；只添加一层展示容器，不移动字段、不改变表单 `name`、`disabled`、`onChange`、`watch` 和提交逻辑。
3. 权限风险：卡片化不能绕过 `canEditSensitiveFields`、`permissions.canViewSecret`、`canEditBasicFields` 等判断；所有原有按钮 disabled、toast 和 title 逻辑保持不变。
4. 搜索行为风险：本轮不改搜索业务函数，但布局调整后仍必须用 MCP 重新验证 `gpt-5.6` 搜索、按钮追加和 Enter 追加。
5. new-api 覆盖风险：不从 `/opt/project/new-api-main` 整文件复制，避免丢失 NexusTok 的账号池、权限和 Codex 原生能力。

### 方案评审

采用“结构对齐、逻辑不动”的保守方案：

1. 将桌面编辑导航从 `13rem` 调整为更宽的列，并允许垂直导航标题在必要时换行；移动端横向导航仍保持单行截断，避免顶部导航高度不稳定。
2. 在 `Credentials` section 内增加与模型区一致的 `border-border/60 bg-muted/10 rounded-lg border p-4` 容器，并用内部纵向间距统一各凭证字段、账号池模式和 Codex 授权区块。
3. 保持 `ChannelAuthSection`、`ChannelApiAccessSection`、`FormField`、`Select`、`Textarea`、`Switch` 等现有组件组合，不引入新的 UI 组件和第三方依赖。
4. 保持模型搜索增强现状：`onSearchSubmit={handleAddModelSearchMatches}`、`submitSearchOnEnterWithMatches`、`submitSearchOnEnterWhenHighlighted`、供应商推导和分页扫描逻辑都不改。
5. 完成后执行前端定向测试、typecheck、build、`git diff --check`，并用 MCP 访问 `http://192.168.0.202:3003/` 与 `/channels` 真实复测页面是否热更新生效。

### 实施结果

- 渠道编辑抽屉桌面布局从 `lg:grid-cols-[13rem_minmax(0,1fr)]` 调整为 `lg:grid-cols-[15rem_minmax(0,1fr)]`，左侧导航拥有更稳定的展示空间。
- 桌面侧栏导航标题和描述支持换行，`Advanced Settings` 不再因为固定宽度被截断；移动端横向导航仍保持单行截断，避免高度跳动。
- `Credentials` 内容增加与模型区一致的卡片边界，账号池、API Key、多 Key 和 Codex OAuth 等凭证配置在同一视觉容器中呈现。
- `ChannelAuthSection` 增加 `first:border-t-0 first:pt-0`，当 `Authentication` 是卡片首项时不再显示多余顶部分隔线；存在上游专属字段时仍保留分隔线。
- 模型搜索业务逻辑没有改变：OpenAI/Codex 渠道继续按 `vendor=OpenAI` 搜索，按钮和 Enter 仍复用 `handleAddModelSearchMatches` 批量追加缺失模型。

### 验证记录

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts src/features/channels/hooks/use-channel-mutate-form.test.ts src/features/channels/lib/channel-form.test.ts` 通过，72 个用例成功。
2. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
3. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.3 秒内完成生产构建。
4. `git diff --check` 通过，未发现空白错误。
5. 使用 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/`，根页面正常加载；随后登录 `c1cada` 并进入 `/channels`。
6. MCP 打开渠道 `11111` 编辑抽屉后确认热更新生效：左侧 `Advanced Settings` 完整显示，`Credentials` 区域已经呈现卡片边界，模型区仍显示 `Selected 3` 和 `Vendor: OpenAI`。
7. MCP 搜索 `gpt-5.6`，页面显示 `Search results`、`3 matched · 2 new · 1 already selected`、`Will add: gpt-5.6-terra, gpt-5.6-luna`。
8. MCP 在搜索框按 Enter 后，页面出现 `Selected 5`，新增 `gpt-5.6-terra` 与 `gpt-5.6-luna`，未重复已有 `gpt-5.6-sol`；随后点击 `Cancel`，没有保存。
9. MCP 重新打开编辑抽屉，再次搜索 `gpt-5.6` 并点击 `Add all 2 new match(es)`，页面同样变为 `Selected 5`，证明显式按钮路径仍正确。
10. MCP 点击 `Cancel` 后使用浏览器上下文调用 `GET /api/channel/1` 并携带 `NexusTok-User` 头，返回 `models="gpt-5.4,gpt-5.5,gpt-5.6-sol"`，确认验证过程没有污染后端渠道配置。
11. MCP 网络记录中本轮相关请求均为 `GET`：`/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50`、批量扫描 `page_size=100` 和 `/api/channel/1` 回查；未出现 `PUT /api/channel/`。
12. MCP 控制台无 error/warn。

## 本轮实施评审：渠道编辑页搜索弹层与 new-api 组件体验对齐

### 差异来源

用户继续反馈“搜索添加时不正确”，并要求参考 `/opt/project/new-api-main` 最新编辑渠道页面。重新以 3003 运行态页面和接口为准复查后确认：

1. 模型库数据并未缺失：`/api/models/search?keyword=gpt-5.6&p=1&page_size=100` 返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 三个模型。
2. OpenAI 供应商过滤也正确：`/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=100` 同样返回 3 个模型，三个模型的 `vendor_id` 均为 `1`。
3. 运行态编辑抽屉中，渠道 `11111` 已有 `gpt-5.6-sol`，所以搜索摘要显示 `3 matched · 2 new · 1 already selected` 是正确的；按 Enter 已能追加两个缺失模型，草稿从 `Selected 3` 变为 `Selected 5`。
4. 但搜索弹层在浅色页面中被强制渲染为深色，和 new-api 最新组件不一致，也让“搜索结果”和“Will add”区域在视觉上像独立暗色浮层而不是编辑页原生控件。对照 `/opt/project/new-api-main/web/default/src/components/ui/combobox.tsx`，new-api 的 `ComboboxContent` 没有强制 `dark` 类。
5. new-api 的旧式 `ComboboxInput` 还补充了 `openOnFocus` 和鼠标重新打开逻辑。NexusTok 的渠道类型选择器仍通过 legacy `Combobox` wrapper 使用 `ComboboxInput`，该能力有助于编辑渠道页和其它设置弹窗保持一致的选择器交互。

### 需求分析

本轮目标是让渠道编辑页的搜索添加区域更贴近 new-api 最新编辑页，同时保留 NexusTok 已验证正确的模型搜索批量追加能力：

1. 保持 `gpt-5.6` 搜索结果按 OpenAI 供应商返回 3 个模型，并只追加尚未选择的 `terra/luna`。
2. 保持按钮和 Enter 两条路径都复用 `handleAddModelSearchMatches`，不引入新的模型保存路径。
3. 去掉通用 Combobox 弹层的强制暗色主题，让弹层跟随当前页面主题，和 new-api 最新组件一致。
4. 补回 legacy Combobox 的 `openOnFocus` 兼容能力，减少渠道类型选择、设置弹窗模型选择等旧式输入与 new-api 的交互差距。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 通用 Combobox 弹层 | `web/default/src/components/ui/combobox.tsx` | 移除 `ComboboxContent` 上的强制 `dark` 类；弹层颜色回到 `bg-popover/text-popover-foreground` 语义 token，自动跟随当前主题。 |
| legacy Combobox wrapper | `web/default/src/components/ui/combobox.tsx` | 为 legacy props 暴露并透传 `openOnFocus`，与 new-api 最新组件保持一致。 |
| legacy ComboboxInput | `web/default/src/components/ui/combobox-input.tsx` | 增加 `openOnFocus` 默认值、鼠标按下重新打开逻辑和 pointer focus 标记；不改变默认打开行为。 |
| 渠道模型搜索 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 不改业务逻辑，只通过 MCP 回归验证 `gpt-5.6` 搜索、Enter 追加和按钮追加。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审和验证结果。 |

本轮不修改 Go 后端、数据库、模型同步接口、`/api/models/search` 查询逻辑、渠道保存 payload、账号池、Codex OAuth、权限控制、模型映射和计费逻辑。

### 风险评估

1. 通用组件视觉风险：`ComboboxContent` 被多个页面复用。移除强制 `dark` 后，浅色主题弹层会变浅，深色主题仍由全局主题变量控制；这是与 new-api 对齐的预期变化。
2. 交互回归风险：`openOnFocus` 默认仍为 `true`，和现有“聚焦打开”行为一致；新增参数只允许调用方显式关闭，不会改变现有调用点。
3. 渠道搜索业务风险：本轮不改搜索追加函数、供应商推导、分页扫描和权限判断，避免再次影响已验证的 `Selected 3 -> Selected 5` 流程。
4. 可访问性风险：弹层主题改变不影响 Base UI Combobox 的 `listbox`、`option`、高亮和多选状态；完成后继续通过 MCP 快照和控制台检查。
5. new-api 覆盖风险：只增量吸收组件差异，不覆盖整个渠道抽屉或通用 Combobox 文件，避免丢失 NexusTok 的 chip 宽度、防溢出和模型搜索增强。

### 方案评审

采用“组件主题对齐 + legacy 交互补齐”的小切片方案：

1. 将 `ComboboxContent` className 中的强制 `dark` 移除，保留现有 `bg-popover`、`text-popover-foreground`、动画、尺寸、chips anchor 和输入组样式。
2. 在 `LegacyComboboxProps` 中补充 `openOnFocus?: boolean`，并透传给 `LegacyComboboxInput`。
3. 在 `ComboboxInput` 中按 new-api 最新实现增加 `pointerFocusRef`、`onPointerDown` 和 `openOnFocus` 判断；默认值为 `true`，保持现有体验。
4. 不修改渠道模型 MultiSelect 的业务 props 和 `handleAddModelSearchMatches`。
5. 修改后运行定向测试、typecheck、build、`git diff --check`，并访问 `http://192.168.0.202:3003/` 与 `/channels` 真实复测搜索弹层主题和模型追加结果。

### 实施结果

- `ComboboxContent` 已移除强制 `dark` 类，弹层重新使用 `bg-popover` / `text-popover-foreground` 跟随当前主题；浅色页面中的渠道模型搜索结果弹层不再突兀显示为深色。
- legacy `Combobox` wrapper 已支持 `openOnFocus?: boolean` 并透传给 `ComboboxInput`，与 new-api 最新组件能力对齐。
- `ComboboxInput` 已补充 `pointerFocusRef`、`onPointerDown` 和 `openOnFocus` 判断：默认仍聚焦打开，鼠标点击已聚焦但关闭的输入框也能重新打开下拉。
- 渠道模型搜索业务逻辑未修改；`gpt-5.6` 仍按 `vendor=OpenAI` 搜索并展示 `3 matched · 2 new · 1 already selected`。
- 按 Enter 和点击 `Add all 2 new match(es)` 都继续只追加缺失的 `gpt-5.6-terra`、`gpt-5.6-luna`，不会重复已有 `gpt-5.6-sol`。

### 验证记录

1. `cd web/default && bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts src/features/channels/hooks/use-channel-mutate-form.test.ts src/features/channels/lib/channel-form.test.ts` 通过，72 个用例成功。
2. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
3. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.6 秒内完成生产构建。
4. `git diff --check` 通过，未发现空白错误。
5. 使用 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/`，根页面正常加载；随后进入 `/channels` 并打开渠道 `11111` 编辑抽屉。
6. MCP 在浏览器上下文调用模型搜索接口确认数据完整：`/api/models/search?keyword=gpt-5.6&p=1&page_size=100` 返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`；`/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=100` 同样返回 3 条，三条模型 `vendor_id` 均为 `1`。
7. MCP 在模型输入中搜索 `gpt-5.6`，页面显示 `Search results`、`3 matched · 2 new · 1 already selected`、`Will add: gpt-5.6-terra, gpt-5.6-luna`。
8. MCP 读取弹层 DOM 确认热更新已生效：`[data-slot="combobox-content"]` 不再包含 `dark` 类，计算样式背景为浅色 `bg-popover`，候选项为 `gpt-5.6-terra` 和 `gpt-5.6-luna`。
9. MCP 在搜索框按 Enter 后，页面出现 `Added 2 model(s) from search`，前端草稿从 `Selected 3` 变为 `Selected 5`，新增 `gpt-5.6-terra` 与 `gpt-5.6-luna`。
10. MCP 点击 `Cancel` 后通过 `GET /api/channel/1` 回查，后端持久化 `models` 仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认未误保存。
11. MCP 再次打开编辑抽屉，搜索 `gpt-5.6` 后点击 `Add all 2 new match(es)`，按钮路径同样变为 `Selected 5`，与 Enter 路径一致。
12. MCP 网络记录显示本轮相关请求均为 GET：`/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50`、批量扫描 `page_size=100` 和 `/api/channel/1` 回查；未出现 `PUT /api/channel/`。
13. MCP 控制台无 error/warn；仅保留既有 autocomplete / label 可访问性 issue，本轮没有扩大该独立问题。

## 本轮实施评审：Dashboard Flow 敏感数据显隐原生化

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/features/dashboard/index.tsx` 后确认，new-api 最新 Dashboard 在 `flow` 分区右上角提供了敏感数据显隐按钮，点击后通过 `sensitiveVisible` 控制流量图是否展示真实用户、Token、渠道等节点名称。NexusTok 的 `FlowCharts` 组件内部已经具备 `sensitiveVisible?: boolean` 能力，并会在 `props.sensitiveVisible === false` 时调用 `buildDashboardFlowData(..., { maskSensitive })` 对图节点做遮罩；当前缺口只在 Dashboard 页面入口没有暴露这个开关，管理员查看流量账本时无法按场景快速隐藏敏感信息。

### 需求分析

本轮目标是把 new-api 的 Flow 敏感数据显隐优势转为 NexusTok 原生能力，同时保留 NexusTok 已有的细粒度权限、分区描述和 FlowCharts 扩展能力：

1. 在 `/dashboard/flow` 页面的操作区新增图标按钮，默认展示真实数据，点击后切换为遮罩显示。
2. 按钮需要有可访问的 `aria-label` 和 Tooltip 文案，状态变化时文案在“隐藏敏感数据”和“显示敏感数据”之间切换。
3. 只把显隐状态传给 `LazyFlowCharts`，不改变 Flow 数据查询条件、筛选器、图表交互、权限判断和后端接口。
4. 新增文案必须补齐 `en`、`zh`、`fr`、`ja`、`ru`、`vi` 六种前端语言，避免 i18n 缺键。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| Dashboard 页面入口 | `web/default/src/features/dashboard/index.tsx` | 新增 `flowSensitiveVisible` 状态、Flow 操作区显隐按钮，并把状态传给 `LazyFlowCharts`。 |
| Flow 图表 | `web/default/src/features/dashboard/components/flow/flow-charts.tsx` | 已有 `sensitiveVisible` 支持，本轮不修改内部图表逻辑，只复用现有遮罩能力。 |
| 前端国际化 | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增 `Hide sensitive data` 与 `Show sensitive data` 翻译。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮差异来源、需求分析、影响范围、风险评估、方案评审和后续验证记录。 |

本轮不修改 Go 后端、数据库、日志聚合接口、模型筛选逻辑、用户权限模型、渠道编辑页和模型搜索逻辑。

### 风险评估

1. 权限回退风险：new-api 使用较粗的管理员判断，而 NexusTok 当前 Dashboard 使用 `canReadAdminResource(user, ADMIN_PERMISSION_RESOURCES.USAGE_DATA)` 控制 `users` 分区可见性。本轮只新增 Flow 按钮，不改 `visibleSections` 和 `canReadUsageData`，避免权限模型回退。
2. 数据展示风险：显隐开关只影响前端节点标签遮罩，不影响 `/api/log/flow` 或等价 Flow 接口的查询参数和返回数据；管理员显隐属于本地 UI 状态，不会写入服务端。
3. 交互回归风险：操作区已有 `ModelsFilter`，新增按钮会增加一个紧凑图标控件；采用现有 `Button size='icon'` 和 `TooltipTrigger render` 组合，避免自定义浮层和布局。
4. i18n 风险：新增两个英文 key 后必须同步六种语言并运行 `bun run i18n:sync`，防止缺键或排序漂移。
5. 图标体系风险：项目 shadcn 配置为 hugeicons，但 Dashboard/Flow 现有模块已经大量使用 `lucide-react`。本轮在同一模块沿用 `Eye/EyeOff`，避免跨图标库混排和额外映射风险；按钮内部不添加手写尺寸类，交给 `Button` 控制图标尺寸。

### 方案评审

采用“入口补开关、图表复用已有能力”的最小切片方案：

1. 在 `Dashboard` 组件中新增 `flowSensitiveVisible` 状态，默认值为 `true`，保持现有页面默认行为不变。
2. 在 `dashboardActions` 的 `activeSection === 'flow'` 场景中，在 `ModelsFilter` 前渲染 Tooltip 包裹的 icon Button；按钮点击时切换显隐状态，`aria-label` 和 Tooltip 文案根据状态动态变化。
3. 将 `LazyFlowCharts filters={modelFilters}` 改为 `LazyFlowCharts filters={modelFilters} sensitiveVisible={flowSensitiveVisible}`，复用 FlowCharts 现有遮罩实现。
4. 只新增 `Hide sensitive data`、`Show sensitive data` 两个翻译 key，不引入新的展示文案和设置项。
5. 修改后执行 i18n 同步、typecheck、build、`git diff --check`，并通过 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/` 和 `/dashboard/flow` 真实验证热更新、按钮状态、Tooltip、接口请求和控制台状态。

### 实施结果

- Dashboard `flow` 分区操作区新增敏感数据显隐按钮，默认状态为展示真实标签，按钮 `aria-label` 为 `Hide sensitive data`。
- 点击按钮后本地状态切换为遮罩模式，按钮 `aria-label` 变为 `Show sensitive data`，同时 `aria-pressed=true`，便于辅助技术识别当前遮罩状态。
- `LazyFlowCharts` 已接收 `sensitiveVisible={flowSensitiveVisible}`，复用现有 `maskSensitive` 能力；没有修改 Flow 数据查询、筛选器、节点显示/隐藏、图表点击和后端接口。
- 已补齐 `Hide sensitive data`、`Show sensitive data` 在 `en`、`zh`、`fr`、`ja`、`ru`、`vi` 六种语言中的翻译。
- 保留 NexusTok 原有 Dashboard 细粒度权限判断和分区描述，没有回退到 new-api 的粗粒度管理员逻辑。

### 验证记录

1. `cd web/default && bun run i18n:sync` 通过，`en/zh` missing、extras、untranslated 均为 0；`fr/ja/ru/vi` 没有 missing/extras，本轮新增的两个 key 均已翻译，剩余 untranslated 为既有债务。
2. `cd web/default && bun test src/features/dashboard/lib/flow.test.ts` 通过，11 个 Flow 数据构建用例成功。
3. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
4. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.2 秒内完成生产构建。
5. `git diff --check` 通过，未发现空白错误。
6. 使用 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/`，根页面正常加载；随后在浏览器上下文调用 `/api/user/login` 登录 `c1cada`，返回 HTTP 200、`success=true`、`uid=1`、`role=100`。
7. MCP 跳转 `/dashboard/flow` 后确认热更新生效：页面标题为 `Traffic Flow`，操作区出现按钮 `Hide sensitive data`，旁边仍保留 `Filter` 筛选按钮。
8. MCP 点击显隐按钮后，按钮状态变为 `Show sensitive data` 且 `aria-pressed=true`，证明显隐状态能在真实页面中切换。
9. MCP 网络记录显示页面触发 `GET /api/data/flow?start_timestamp=...&end_timestamp=...&default_time=hour` 并返回 HTTP 200；响应体为 `{"data":[],"message":"","success":true}`，当前环境无 Flow 数据，因此页面显示 `No flow data available` 为空态是合理结果。
10. MCP 控制台无 error、warn、issue，本轮没有引入新的前端运行时问题。

## 本轮实施评审：Pricing 模型级图标 fallback 原生化

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/features/pricing` 后确认，new-api 最新 Pricing 在模型卡片、表格模型列和模型详情头部都会优先展示模型自身 `icon`，仅当模型级图标为空时才回退到供应商 `vendor_icon`。NexusTok 后端 `model/pricing.go` 的 `Pricing` DTO 已经返回 `icon,omitempty`，但当前前端 `PricingModel` 类型没有声明模型级 `icon` 字段，Pricing 展示层也主要只读取 `vendor_icon`，导致 models.dev 或管理员维护的模型级图标无法在模型广场中体现。

### 需求分析

本轮目标是把 new-api 的模型级图标展示优势转成 NexusTok 原生能力，同时保留当前 Pricing 页面已有的价格、性能、动态计费和模型详情能力：

1. 前端 `PricingModel` 类型需要补齐后端已经返回的 `icon?: string` 字段。
2. 模型卡片、Pricing 表格模型列、模型详情页头部都应使用统一优先级：`model.icon` 优先，`model.vendor_icon` 作为 fallback。
3. 空字符串、空白字符串不能覆盖有效供应商图标；应归一为“没有模型图标”并继续 fallback。
4. 不改变供应商筛选、供应商侧栏、Vendor 列、价格计算、动态计费表达式和性能徽章。
5. 用纯函数测试锁定 fallback 规则，避免后续页面局部实现再次分叉。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| Pricing 类型 | `web/default/src/features/pricing/types.ts` | `PricingModel` 新增可选 `icon` 字段，与后端 DTO 和 new-api 类型对齐。 |
| Pricing 图标规则 | `web/default/src/features/pricing/lib/model-icon.ts` | 新增共享 helper，统一选择 `model.icon || model.vendor_icon` 并处理空白值。 |
| Pricing 展示组件 | `web/default/src/features/pricing/components/model-card.tsx`、`pricing-columns.tsx`、`model-details.tsx` | 模型卡片、表格模型列和详情头部改用共享 helper；布局、文案和交互不变。 |
| 测试 | `web/default/src/features/pricing/lib/model-icon.test.ts` | 覆盖模型级图标优先、供应商 fallback、空白模型图标 fallback 和完全缺失。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮差异来源、需求分析、影响范围、风险评估、方案评审和后续验证。 |

本轮不修改 Go 后端、数据库、`/api/pricing` 响应结构、模型同步任务、模型元信息编辑、计费表达式、价格换算、权限和渠道逻辑。

### 风险评估

1. 数据兼容风险：后端已存在 `icon,omitempty`，前端只补类型和展示 fallback；旧响应没有 `icon` 时会继续使用 `vendor_icon`，不会破坏现有页面。
2. 视觉回归风险：只改变模型图标来源，不改容器尺寸、卡片排版、表格列宽、详情页布局和 fallback 首字母显示。
3. 图标 key 风险：`getLobeIcon` 已能处理未知 key 并渲染首字母 fallback；本轮只决定传入哪个 key，不引入新的图标库和资源。
4. 性能风险：helper 是简单字符串选择，渲染成本不变；Pricing 列表已有的 memo、table 和详情逻辑不受影响。
5. 业务风险：供应商侧栏和 Vendor 列仍展示供应商图标，不会因为模型级图标存在而影响供应商归类和筛选。

### 方案评审

采用“类型补齐 + 共享 helper + 三处消费替换”的保守方案：

1. 在 `PricingModel` 中添加 `icon?: string`，与后端 DTO 和 new-api 类型保持一致。
2. 新增 `getPricingModelIconKey(model)`，返回去空白后的模型级图标；模型级图标为空时再返回去空白后的 `vendor_icon`；两者都为空则返回 `undefined`。
3. 模型卡片、表格模型列和模型详情头部统一调用 `getPricingModelIconKey` 后再交给 `getLobeIcon`，不在组件里重复写 fallback 条件。
4. 增加 `model-icon.test.ts`，用 `node:test` 覆盖核心 fallback 语义。
5. 修改后运行 Pricing helper 测试、typecheck、build、`git diff --check`，并通过 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/` 和 `/pricing` 验证页面热更新、模型图标 DOM 状态、接口响应和控制台状态。

### 实施结果

- `PricingModel` 已补齐 `icon?: string`，前端类型与后端 `Pricing.Icon` 和 new-api 类型保持一致。
- 新增 `getPricingModelIconKey(model)`，统一实现 `model.icon` 优先、`model.vendor_icon` fallback，并对空白字符串做归一处理。
- 模型卡片、Pricing 表格模型列、模型详情页头部都已切换到共享 helper，模型级图标会覆盖供应商图标；Vendor 列和供应商侧栏仍展示供应商图标。
- 新增 `model-icon.test.ts`，覆盖模型级优先、供应商 fallback、空白值 fallback 和完全缺失四种场景。
- 没有修改 `/api/pricing`、价格计算、动态计费表达式、性能摘要、筛选或排序逻辑。

### 验证记录

1. `cd web/default && bun test src/features/pricing/lib/model-icon.test.ts` 通过，4 个 fallback 规则用例成功。
2. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
3. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.3 秒内完成生产构建。
4. `git diff --check` 通过，未发现空白错误。
5. 使用 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/`，根页面正常加载；随后跳转 `/pricing`，页面显示 `Model Square` 和 3 个模型卡片。
6. MCP 在浏览器上下文调用 `GET /api/pricing`，返回 HTTP 200、`success=true`、`modelCount=3`；三个模型 `gpt-5.6-sol`、`gpt-5.5`、`gpt-5.4` 均返回 `icon="OpenAI.Color"`，供应商 `vendor_icon="OpenAI"`，预期展示 key 为 `OpenAI.Color`。
7. MCP 确认卡片视图中 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol` 正常渲染并显示模型图标；点击 `gpt-5.4` 的 `Details` 后，详情弹窗头部也正常显示图标和模型名。
8. MCP 切换表格视图后，模型列展示 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`，Vendor 列仍显示 `OpenAI`，证明模型列和供应商列都能正常渲染。
9. MCP 网络记录显示 `/api/pricing`、`/api/perf-metrics/summary?hours=24` 和详情触发的 `/api/perf-metrics?model=gpt-5.4&hours=24` 均返回 HTTP 200。
10. MCP 控制台无 error/warn；DevTools 报告 1 条表单字段缺少 `id/name` 的既有 accessibility issue，本轮没有触碰表单控件。

## 本轮实施评审：Usage Logs 类型筛选语义原生化

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/features/usage-logs` 后确认，new-api 最新版已经把 common logs 的类型筛选语义拆成两层：筛选下拉中的 `type=0` 表示 `All Types`，而表格行中真实日志记录的 `type=0` 仍展示为 `Unknown`。NexusTok 当前仍把 `LOG_TYPE_FILTERS` 直接从 `LOG_TYPES` 派生，导致筛选项里 `0` 显示为 `Unknown`；同时 URL schema 只接受数组形式，`/usage-logs/common?type=0` 和浏览器手动输入的单值 query 容易产生 UI、表格本地过滤和后端查询语义不一致。

早期报告中曾记录“暂不迁移 `LOG_TYPE_ALL_VALUE='0'` 语义”，当时是为了避免误改后端查询。本轮通过重新核对当前后端 list/stat 查询行为和 new-api 前端实现，确认后端 common logs 接口已经把 `type=0` 作为“全部类型”哨兵使用，因此需要将该优势原生化到 NexusTok 前端，修正历史遗留结论。

### 需求分析

1. common logs 类型筛选下拉必须显示 `All Types`、`Top-up`、`Consume`、`Manage`、`System`、`Error`、`Refund`、`Login`，不再把筛选项 `0` 显示成 `Unknown`。
2. 真实日志行的 `type=0` 仍应通过 `LOG_TYPES` 展示为 `Unknown`，不能丢失未知类型日志的可观测性。
3. URL 中 `type=0`、`type[]=0` 或路由解析后的 `['0']` 都应代表“全部类型”；表格本地 column filter 不应因为 `0` 把页面过滤到只剩 Unknown。
4. URL schema 需要同时兼容字符串和数组形式，便于用户手动输入、历史链接、表格状态序列化和 TanStack Router 解析。
5. common logs 筛选栏的草稿状态应从当前 URL 派生；URL 改变时不能保留旧筛选草稿，避免“页面看似 All Types，但搜索仍带旧 type”的幽灵筛选。
6. 非 common 日志页继续清理 `type` 查询参数，避免 drawing/task logs 误用 common logs 的类型筛选。
7. 保留 NexusTok 当前细粒度管理员权限、敏感信息显隐、移动端筛选工具栏、DataTablePage 体系和后端接口，不整文件覆盖 new-api 实现。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 类型常量 | `web/default/src/features/usage-logs/constants.ts` | 新增 `LOG_TYPE_ALL_VALUE`，并让筛选项使用 `All Types` + 非 Unknown 类型列表；行展示配置仍保留 Unknown。 |
| 路由 search schema | `web/default/src/routes/_authenticated/usage-logs/$section.tsx` | `type` 支持字符串或数组输入；非 common 分区继续清理已有 type。 |
| common logs 表格 | `web/default/src/features/usage-logs/components/usage-logs-table.tsx` | URL 中的 `type=0` 进入表格本地筛选前会被反序列化为空筛选，避免前端二次过滤误伤。 |
| common logs 筛选栏 | `web/default/src/features/usage-logs/components/common-logs-filter-bar.tsx` | 使用统一筛选项、URL 派生草稿、All Types 哨兵和重置语义；保留敏感信息显隐按钮与管理员字段。 |
| common logs 列过滤 | `web/default/src/features/usage-logs/components/columns/common-logs-columns.tsx` | 本地 filterFn 兼容 `LOG_TYPE_ALL_VALUE`，即使外部传入 `0` 也视为全部。 |
| 接口参数构建 | `web/default/src/features/usage-logs/lib/utils.ts` | `buildApiParams` 兼容字符串和数组形式的 type，继续把 `0` 发送给后端作为全部类型哨兵。 |
| 测试 | `web/default/src/features/usage-logs/lib/utils.test.ts` | 增加 type 解析和 `type=0` 哨兵相关的纯逻辑覆盖。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮差异来源、需求分析、影响范围、风险评估、方案评审和后续验证记录。 |

本轮不修改 Go 后端、数据库日志模型、日志权限模型、审计/登录日志摘要、任务日志、绘图日志、统计接口、渠道编辑页和模型搜索逻辑。

### 风险评估

1. 后端语义风险：若前端把 `type=0` 当作 Unknown，本地过滤和后端查询会冲突。本轮明确 `LOG_TYPE_ALL_VALUE='0'` 只用于筛选项，行展示仍使用 `LOG_TYPES`，降低语义混淆。
2. URL 兼容风险：历史链接可能是 `?type=2`，也可能是重复 query 形成数组。schema 通过 preprocess 统一成数组，避免 validateSearch 把合法链接清空。
3. 本地过滤风险：DataTable 的 column filter 可能先于远端结果再过滤一次。对 `type=0` 做 deserialize 过滤，并在列 filterFn 中兜底 pass-all，可避免远端返回的非 Unknown 日志被前端隐藏。
4. 状态同步风险：原筛选栏用 `useEffect` 局部 merge URL，URL 清空时旧 draft 可能残留。本轮改为 URL 派生 searchState + draft sourceKey，确保外部导航、重置和浏览器前进后退都能同步。
5. 权限回退风险：new-api 使用 `useIsAdmin`，NexusTok 使用资源级 `useAdminPermission`。本轮只迁移筛选语义，不改变管理员字段的可见性规则。
6. i18n 风险：`All Types` 已是既有 key，本轮不新增文案；修改后仍会通过 typecheck/build 和页面验证观察是否出现缺键。

### 方案评审

采用“语义常量 + URL 归一 + 表格反序列化 + 筛选栏派生草稿”的保守方案：

1. 在 `constants.ts` 中新增 `LOG_TYPE_ALL_VALUE='0'`，`LOG_TYPE_FILTERS` 第一项固定为 `All Types`，其余项过滤掉 `LOG_TYPE_ENUM.UNKNOWN`。
2. 在路由 schema 中为 `type` 添加 preprocess：空值转为 `undefined`，字符串转为单元素数组，数组保持原样并通过枚举校验；非 common 分区判断字符串/数组两种形态并清理。
3. 在 `UsageLogsTable` 中为 `type` column filter 添加 `deserializeLogTypeFilter`，把 URL 的 `0` 哨兵从本地表格过滤状态中移除。
4. 在 `CommonLogsFilterBar` 中移植 new-api 的 URL 派生草稿思路，但保留 NexusTok 的资源级权限和现有敏感信息显隐按钮；搜索和重置都显式写入 `type: [LOG_TYPE_ALL_VALUE]`，让 URL 与后端语义一致。
5. 在 common logs 列 filterFn 中把 `LOG_TYPE_ALL_VALUE` 视为 pass-all，作为 deserialize 之外的防御式兜底。
6. 在 `buildApiParams` 中让 `processType` 同时接受字符串和单元素数组，并只返回有限数字；这能兼容路由 schema、表格 column filter 和手动调用。
7. 修改后运行 usage-logs helper 测试、typecheck、build、`git diff --check`，并通过 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/usage-logs/common?type=0` 验证 All Types 展示、接口请求、特定类型筛选、非 common 分区清理 type 和控制台状态。

### 实施结果

- `LOG_TYPE_ALL_VALUE='0'` 已成为 common logs 筛选层的显式常量；`LOG_TYPE_FILTERS` 现在展示 `All Types` 和 1-7 的具体日志类型，不再在筛选下拉中暴露 `Unknown`。
- `LOG_TYPES` 仍保留 `Unknown`，真实日志行如果返回 `type=0` 仍会展示为 Unknown，筛选语义和数据展示语义已经分离。
- 路由 search schema 已支持字符串、数字和数组形态的 `type`：手动访问 `?type=0` 会规范化为 `type=["0"]`，避免 TanStack Router 把数值 `0` 解析后落入 fallback。
- `CommonLogsFilterBar` 已改为 URL 派生 `searchState` + 草稿 `sourceKey`，搜索、重置、浏览器导航和外部 URL 改变时不会保留旧类型筛选。
- `UsageLogsTable` 已对 `type=0` 做本地 column filter 反序列化，避免后端返回多类型数据后被前端二次过滤成 Unknown。
- common logs `created_at` 列的本地 `filterFn` 已把 `LOG_TYPE_ALL_VALUE` 视为 pass-all，作为防御式兜底。
- `buildApiParams` 已兼容数组、字符串和数值 `type`，并继续把 `0` 透传给后端 list/stat 接口作为“全部类型”哨兵。
- 新增 `web/default/src/features/usage-logs/lib/utils.test.ts`，覆盖 `type=0` 哨兵、字符串 type、数值 type 和列筛选覆盖 URL type。
- 保留 NexusTok 的 `useAdminPermission(USAGE_LOG, READ)` 权限模型、敏感信息显隐按钮、移动端筛选工具栏和当前 DataTablePage 架构，没有回退到 new-api 的粗粒度管理员判断。

### 验证记录

1. `cd web/default && bun test src/features/usage-logs/lib/utils.test.ts` 通过，4 个 `buildApiParams` 类型解析用例成功。
2. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
3. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.6 秒内完成生产构建。
4. `git diff --check` 通过，未发现空白错误。
5. 使用 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/`，在浏览器上下文调用 `/api/user/login` 登录 `c1cada`，返回 HTTP 200、`success=true`、`uid=1`、`role=100`。
6. MCP 首次访问 `http://192.168.0.202:3003/usage-logs/common?type=0` 时发现路由会把裸 `0` 解析为数值并 fallback 成空数组；已补充数字归一逻辑和测试后重新验证通过。
7. MCP 重新访问 `http://192.168.0.202:3003/usage-logs/common?type=0` 后，URL 规范化为 `type=%5B%220%22%5D`；页面 common logs 类型下拉显示 `All Types`，下拉选项为 `All Types`、`Top-up`、`Consume`、`Manage`、`System`、`Error`、`Refund`、`Login`，没有 `Unknown` 筛选项。
8. MCP 网络记录显示 common logs 统计请求为 `/api/log/stat?...&type=0...`，列表请求为 `/api/log/?p=1&page_size=100&type=0...`，均返回 HTTP 200；响应数据中同时包含 `Login` 和 `Manage` 类型记录，证明 `type=0` 没有被本地表格误过滤为 Unknown。
9. MCP 在页面下拉选择 `Manage` 后点击 `Search`，URL 更新为 `type=%5B%223%22%5D`；网络记录显示 `/api/log/stat?...&type=3...` 和 `/api/log/?...&type=3...` 均返回 HTTP 200，表格仅展示 `Manage` 记录。
10. MCP 访问 `http://192.168.0.202:3003/usage-logs/task?type=3` 后页面自动规范化为 `/usage-logs/task`，task logs 请求为 `/api/task/?p=1&page_size=100&start_timestamp=...&end_timestamp=...`，没有携带 `type`。
11. MCP 控制台检查 `error`、`warn`、`issue` 均为空。本轮验证期间 3003 曾在热发布重建窗口短暂返回连接拒绝；Docker 日志确认 `nexustok-api-hot` 完成 frontend dist 发布和 backend restart 后恢复，随后所有页面与接口验证均通过。

## 本轮实施评审：Usage Logs 渠道列重试链与 multi-key 标记原生化

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/features/usage-logs/components/columns/common-logs-columns.tsx` 后确认，new-api 最新 Common Logs 在管理员渠道列中直接展示两类排障信息：当 `other.admin_info.is_multi_key=true` 且存在 `multi_key_index` 时，在渠道 ID 旁显示 key index 小徽标；当 `other.admin_info.use_channel` 包含多个渠道时，在同一行提供 `Retry Chain` 弹层展示完整重试链路。NexusTok 当前详情弹窗已经能展示 `Retry Chain`，但列表页渠道列只显示渠道 ID、渠道名称和 Channel Affinity，管理员无法在列表扫描阶段快速发现多 key 命中或重试链路。

### 需求分析

1. Common Logs 管理员视图的渠道列应在渠道 ID 旁显示 multi-key 命中序号，便于定位同一渠道内实际使用的 key。
2. 当 `use_channel` 包含多个有效渠道值时，渠道列应显示可点击的重试链路图标，点击后用 Popover 展示完整链路。
3. 单个 `use_channel` 只代表当前使用渠道，不应误标为重试链；空值、非数组和无效值应忽略。
4. 现有详情弹窗里的 `Retry Chain` 行保留；列表增强只是提前暴露关键排障信号，不改变后端日志结构。
5. 继续保留 NexusTok 当前更完整的 `DataTableColumnHeader`、`meta.label`、移动端隐藏策略、Channel Affinity 弹窗入口和敏感信息显隐逻辑。
6. 该能力只对管理员日志列生效，不影响普通用户日志、日志查询参数、计费、渠道调度和 relay 请求。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道标记解析 | `web/default/src/features/usage-logs/lib/channel-markers.ts` | 新增纯函数，从 `admin_info` 中归一化 `use_channel`、`is_multi_key` 和 `multi_key_index`。 |
| 纯逻辑测试 | `web/default/src/features/usage-logs/lib/channel-markers.test.ts` | 覆盖多渠道重试链、单渠道忽略、multi-key index 展示和无效 index 忽略。 |
| Common Logs 列 | `web/default/src/features/usage-logs/components/columns/common-logs-columns.tsx` | 管理员渠道列新增 key index 徽标和 `Retry Chain` Popover；保留原渠道 Tooltip 与 Channel Affinity。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮差异来源、需求分析、影响范围、风险评估、方案评审和后续验证。 |

本轮不修改 Go 后端、数据库、日志写入、日志详情弹窗、日志查询参数、权限规则、渠道调度、账号池、模型搜索和计费表达式。

### 风险评估

1. 敏感信息风险：`multi_key_index` 和 `use_channel` 来源于 `admin_info`，本轮只在管理员列中消费；普通用户没有渠道列，也不会看到该信息。Tooltip/Popover 中仍遵循 `sensitiveVisible` 对渠道名称和 affinity group 的遮罩。
2. 误报风险：`use_channel` 只有一个元素时不展示 Retry Chain 图标，避免把正常单渠道命中误判为重试；非数组值会被忽略。
3. 布局风险：渠道列宽度有限，新增 key index 和 branch 图标可能挤压渠道名称。本轮将标记放在渠道 ID 同一行，以小尺寸、可收缩控件展示，渠道名称仍在下一行 truncate。
4. 交互风险：Channel Affinity 原有小图标使用绝对定位；新增的 key index 和 retry icon 不能遮挡它。保持外层 `relative inline-flex`，新增元素位于正常 flex 流，affinity 继续固定在右上角。
5. 组件风险：Popover 使用项目已安装的 Base UI shadcn 组件，`Retry Chain`/`Key`/`Chain` 翻译键已存在，不新增 i18n 文案。
6. 可测试性风险：列表 UI 依赖日志数据，测试环境不一定有 multi-key 日志。本轮把解析逻辑提到纯函数并加单元测试，MCP 侧至少验证页面无运行时错误和普通日志列不回归。

### 方案评审

采用“纯函数归一化 + 渠道列小幅增强”的保守方案：

1. 新增 `getUsageLogChannelMarkers(adminInfo)`，返回 `retryChannels`、`retryChain`、`hasRetryChain` 和 `multiKeyIndex`；只有多个有效渠道才生成 retry chain，只有 `is_multi_key=true` 且 index 为有限数字时才返回 index。
2. 在 common logs 渠道列中用 helper 替代直接读取 `use_channel`，让列表、Tooltip 和 Popover 共享同一解析语义。
3. 渠道 ID `StatusBadge` 改为无 leading dot 的紧凑形态，旁边追加 key index 小圆徽标和 `GitBranch` 图标按钮；点击 retry 按钮打开 `PopoverContent` 展示完整链路。
4. 保留原 `TooltipContent` 中的 `Chain` 文案和 Channel Affinity 详情，并在 Tooltip 中补充 `Key: <index>`。
5. 修改后运行 `channel-markers` 测试、typecheck、build、`git diff --check`，并通过 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/usage-logs/common?type=0` 验证页面热更新、渠道列渲染、接口请求和控制台状态。

### 实施结果

- 新增 `getUsageLogChannelMarkers(adminInfo)`，统一归一化渠道列 marker：`use_channel` 只接受数组并转换成有效字符串列表；两个及以上有效渠道才生成 `retryChain`；`multiKeyIndex` 只在 `is_multi_key=true` 且序号为有限数字时返回。
- Common Logs 管理员渠道列已使用该 helper，不再在列组件内直接拼接 `use_channel`。
- 渠道 ID 徽标已改为更紧凑的无 dot 形态，保留 `copyText={channel}`；当存在 `multiKeyIndex` 时在渠道 ID 旁显示小圆徽标，并在 Tooltip 中补充 `Key: <index>`。
- 当 `hasRetryChain=true` 时，渠道列会显示 `GitBranch` 图标按钮，点击后通过 `Popover` 展示完整 `Retry Chain`。
- 原有 Channel Affinity 小图标、Tooltip 内容、敏感数据显隐、管理员权限控制、详情弹窗 `Retry Chain` 行和日志请求参数均未改变。
- 新增 `channel-markers.test.ts`，覆盖多渠道重试链、单渠道不展示、multi-key 序号展示和无效序号忽略。

### 验证记录

1. `cd web/default && bun test src/features/usage-logs/lib/channel-markers.test.ts` 通过，4 个 marker 解析用例成功。
2. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
3. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.3 秒内完成生产构建。
4. `git diff --check` 通过，未发现空白错误。
5. 使用 Chrome DevTools MCP 打开 `http://192.168.0.202:3003/usage-logs/common?type=0`，调用 `/api/user/login` 登录 `c1cada` 返回 HTTP 200、`success=true`、`uid=1`、`role=100`；随后忽略缓存 reload 页面。
6. MCP 页面快照确认 `Common Logs` 正常渲染、类型下拉为 `All Types`、管理员列包含 `Channel`、`User`、`Token`、`Model`、`Timing`、`Tokens`、`Cost` 和 `Details`，本轮渠道列增强没有破坏表格布局。
7. MCP 网络记录显示页面自身请求 `/api/log/stat?...&type=0...` 返回 HTTP 200，`/api/log/?p=1&page_size=100&type=0...` 返回 HTTP 200（中间 `/api/log` 到 `/api/log/` 的 301 为既有路径规范化）。
8. MCP 分页抽样调用 `/api/log/?p=1..5&page_size=100&type=0&start_timestamp=0&end_timestamp=4102444800`，共扫描当前环境 372 条日志，没有发现 `other.admin_info.use_channel` 或 `multi_key_index` 样例，因此无法在真实页面点击 `Retry Chain` Popover；对应分支已由 `channel-markers.test.ts` 覆盖。
9. MCP 干净 reload 后检查控制台，`error`、`warn`、`issue` 均为空。验证过程中早先两次手写 fetch 因未带 `NexusTok-User` 头产生 401 噪声，已重新 reload 后排除，不属于页面自身错误。

## 本轮实施评审：Usage Logs 审计与登录摘要本地化原生化

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/features/usage-logs` 后确认，new-api 最新版已经在 `lib/format.ts` 中提供 `renderAuditContent(other, t)`，并在 Common Logs 列表摘要与详情弹窗中优先使用 `other.op.action + params` 渲染审计/登录日志的人类可读文案。NexusTok 后端已经写入同样的结构化 `other.op` 字段，登录日志也写入 `login_method` 和 `user_agent`，但当前前端列表只对消费日志、退款日志和异常计费做摘要增强；`type=3` 管理审计与 `type=7` 登录审计仍主要依赖 `content` 或只展示 action code，中文/多语言环境下可读性不足。

本轮不采用整文件覆盖。当前 NexusTok 详情弹窗已经具备 new-api 没有的更完整审计信息：操作者、auth method、fallback audit_info、路径参数、充值审计、额度饱和、动态计费、订阅计费、请求转换、Retry Chain 等，因此只将 new-api 的结构化摘要能力融合为 NexusTok 原生能力。

### 需求分析

1. Common Logs 列表中 `type=3` 管理审计和 `type=7` 登录审计应优先展示本地化的操作摘要，而不是只显示英文 raw content 或空的 Details。
2. 详情弹窗应在现有管理审计块中补充 `Operation` 文案，并将 `changed_fields` 渲染为可读字段名，方便管理员快速识别渠道编辑等操作的变更范围。
3. 登录详情应在现有 Login Info 中补充登录操作摘要，例如 `Logged in successfully via password` 对应的本地化文案。
4. 对后端已经存在但 new-api 模板尚未覆盖的 NexusTok 原生 action（权限治理、账号池、渠道账号、Codex OAuth、部署测试、订阅实例等）需要提供更稳健的 fallback，不能因为模板缺失而丢失可读信息。
5. 模板缺参、未知 action 或老日志缺少 `other.op` 时必须安全回退，不影响日志列表渲染。
6. 新增 UI 文案必须进入 `en`、`zh`、`fr`、`ja`、`ru`、`vi` 六种前端 locale。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 审计摘要渲染 | `web/default/src/features/usage-logs/lib/format.ts` | 新增 `renderAuditContent`、审计模板和 action fallback，复用现有 `LogOtherData`，只处理前端展示。 |
| Common Logs 列表 | `web/default/src/features/usage-logs/components/columns/common-logs-columns.tsx` | `type=3/7` 的 Details 摘要优先使用结构化本地化文本；消费日志摘要、渠道列、筛选、权限不变。 |
| 日志详情弹窗 | `web/default/src/features/usage-logs/components/dialogs/details-dialog.tsx` | 在现有管理/登录审计详情中补充 Operation、Changed Fields 和本地化结果展示；保留现有 audit_info 全量字段。 |
| 类型出口 | `web/default/src/features/usage-logs/lib/index.ts` | 导出 `renderAuditContent`，便于后续同域复用。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 补齐本轮新增审计摘要、字段名和操作标签翻译。 |
| 测试 | `web/default/src/features/usage-logs/lib/format.test.ts` | 覆盖登录摘要、渠道更新摘要、generic fallback、未知 action 安全回退。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮评审、实施结果与验证记录。 |

本轮不修改 Go 后端、数据库日志结构、审计写入逻辑、日志查询接口、权限判断、计费、渠道调度和模型同步。

### 风险评估

1. 运行时缺参风险：审计参数来自历史日志，可能缺少 `name`、`id`、`method` 等字段。渲染函数需要接受缺参，模板无法完整表达时仍返回安全字符串或回退到 raw content，不抛异常。
2. 覆盖不足风险：NexusTok 当前后端 action 数量多于 new-api 模板。方案会先覆盖 new-api 模板和 NexusTok 当前审计 action 中的常见资源，再为未知 action 生成稳定 fallback，避免列表空白。
3. i18n 风险：`renderAuditContent` 使用模板 map 动态传入 `t()`，自动扫描脚本不会发现这些 key。需要显式写入 locale 并运行 `bun run i18n:sync`，确认六语缺失数为 0。
4. 详情回归风险：当前详情弹窗已有比 new-api 更多的审计字段。只在现有块内追加 Operation/Changed Fields，不删除 `Action`、`Result`、`HTTP Method`、`Route`、`Path Parameters` 等已有排障字段。
5. 权限风险：管理审计详情仍受 `isManage && props.isAdmin` 控制；登录信息仍按日志归属用户可见，不把管理员专用 `admin_info` 泄露给普通用户。
6. 性能风险：渲染函数只做对象查表、少量字符串格式化和字段过滤，成本与单行日志渲染相比可以忽略，不引入额外网络请求。

### 方案评审

采用“共享渲染 helper + 列表摘要接入 + 详情增量融合 + i18n 补齐”的保守方案：

1. 在 `format.ts` 中新增 `AUDIT_TEMPLATES`，先吸收 new-api 的模板，再补齐 NexusTok 当前 `controller/audit.go` 和 `middleware/audit.go` 中已有的关键 action；同时提供 `generic` 和未知 action fallback。
2. `renderAuditContent(other, t)` 只读取 `other.op`，识别到模板时用 `op.params` 插值；未知 action 时把 `action` 转为可读标题并追加关键参数，缺少 `op` 时返回 `null` 让调用方继续使用原内容。
3. 在 `common-logs-columns.tsx` 的 `buildTypeDetailSegments` 中对 `log.type === 3 || log.type === 7` 优先展示 `renderAuditContent`，不影响消费日志现有摘要。
4. 在 `details-dialog.tsx` 中复用同一 helper：管理审计块新增 `Operation`、`Changed Fields`，登录信息块新增 `Operation`；保留原始 action code 和 audit_info 详情用于排障。
5. 补充单元测试覆盖：登录摘要、渠道更新 + changed_fields、generic fallback、未知 action fallback、缺 op 返回 null。
6. 按 i18n 技能流程补 locale，运行 `bun run i18n:sync`、单测、typecheck、build、`git diff --check`，最后通过 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/usage-logs/common?type=0` 验证热更新、日志页面、详情弹窗、接口请求和控制台状态。

### 实施结果

- 新增 `renderAuditContent(other, t)`，从 `other.op.action + params` 渲染管理审计和登录审计摘要；缺少 `op` 时返回 `null`，调用方继续使用旧 `content`。
- 审计模板已吸收 new-api 的操作摘要，并补入 NexusTok 当前后端 action：权限治理、账号池、渠道账号、Codex OAuth、Ollama、部署、订阅实例、模型价格、倍率同步和系统任务等。
- 模板参数完整时展示自然语言摘要；模板缺少必要参数或遇到未知 action 时，自动退回到 `Audit operation {{action}}` 的稳定摘要，并追加 `method`、`route`、`status` 等可排障字段，避免历史日志出现空白插值。
- Common Logs 列表中 `type=3` 管理日志和 `type=7` 登录日志已优先展示结构化本地化摘要；消费日志、退款日志、渠道列、类型筛选、敏感信息显隐和权限控制不变。
- 详情弹窗保留现有 NexusTok 管理审计字段，并新增 `Operation` 行；登录详情新增 `Operation` 行；渠道更新日志如果带 `changed_fields`，会展示本地化 `Changed Fields`。
- `web/default/src/features/usage-logs/lib/index.ts` 已导出 `renderAuditContent`，便于 usage-logs 同域继续复用。
- 新增 `format.test.ts`，覆盖登录摘要、渠道更新摘要、generic 请求摘要、模板缺参 fallback、未知 action fallback 和缺 `op` 安全返回。
- 已补齐 `en`、`zh`、`fr`、`ja`、`ru`、`vi` 六种 locale 中 132 个审计摘要相关 key；其中与 new-api 重叠的翻译直接复用 new-api，NexusTok 新增 action 已单独翻译。

### 验证记录

1. `cd web/default && bun run i18n:sync` 通过，`en/zh/fr/ja/ru/vi` 的 `missingCount=0`、`extrasCount=0`；`fr/ja/ru/vi` 的 untranslated 数量仍为既有债务，本轮没有扩大缺失面。
2. 额外脚本检查本轮 132 个审计模板 key 在六种 locale 中均存在，`audit missing=0`。
3. `cd web/default && bun test src/features/usage-logs/lib/format.test.ts src/features/usage-logs/lib/channel-markers.test.ts src/features/usage-logs/lib/utils.test.ts` 通过，14 个用例成功。
4. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
5. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.2 秒内完成生产构建。
6. `git diff --check` 通过，未发现空白错误。
7. 使用 Chrome DevTools MCP 打开 `http://192.168.0.202:3003/`，页面正常加载；在浏览器上下文调用 `/api/user/login` 登录 `c1cada`，返回 HTTP 200、`success=true`、`uid=1`、`role=100`。
8. MCP 设置 `i18nextLng=zh` 后忽略缓存访问 `http://192.168.0.202:3003/usage-logs/common?type=0`，路由规范化为 `type=%5B%220%22%5D`，页面标题显示 `通用日志`，类型筛选显示 `所有类型`。
9. MCP 页面快照确认列表中登录日志 Details 显示 `登录成功（通过 password）`，管理审计日志 Details 在模板缺参时显示 `审计操作 Channel Update · method: PUT, route: /api/channel/, status: 200`。
10. MCP 点开登录日志详情，弹窗 `登录信息` 中出现新增 `操作` 行，值为 `登录成功（通过 password）`，同时保留 raw `内容` 行 `Logged in successfully via password`。
11. MCP 点开管理审计详情，弹窗 `管理审计信息` 中出现新增 `操作` 行，值为结构化 fallback 摘要；原有 `动作=channel.update`、`结果=失败`、`HTTP 方法=PUT`、`状态码=200`、`路由=/api/channel/`、`路径=/api/channel/`、`客户端 IP`、`认证方式=会话` 均保留。
12. MCP 网络记录显示 `/api/log/stat?...type=0...` 返回 HTTP 200，`/api/log/?p=1&page_size=100&type=0...` 返回 HTTP 200（中间 `/api/log` 到 `/api/log/` 的 301 为既有路径规范化）。
13. MCP 读取 `/api/log/` 响应体确认真实后端日志包含 `other.op.action=login`、`params.method=password` 以及 `other.op.action=channel.update`；页面摘要来自这些结构化字段。
14. MCP 控制台检查 `error`、`warn`、`issue` 均为空。本轮验证期间临时启动 headless Chrome 连接 MCP，验证完成后已停止该临时会话。

## 本轮实施评审：Markdown 富内容渲染能力原生化

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/components/ui/markdown.tsx` 与当前 `web/default/src/components/ui/markdown.tsx` 后确认，NexusTok 已经吸收了 `RichContent`、`HtmlContent`、`JsonCodeEditor` 和 AI response renderer 等富内容基础组件，但底层 `Markdown` 仍是简化版 `react-markdown + remark-gfm`。new-api 最新 `Markdown` 组件已经切换为 `marked + DOMPurify + KaTeX`，并提供数学公式、任务列表、emoji shortcode、分页分隔符、轻量 flow/sequence 图表等渲染能力。

该差异影响的不只是 Playground。当前 NexusTok 的首页公告、通知弹窗、Dashboard 公告详情、FAQ、法务文档和更新检查 release body 都通过 `RichContent` 或 `Markdown` 渲染；如果管理员在后台写入数学公式、流程说明、FAQ 图示或 release markdown，当前展示能力会明显弱于 new-api。

### 需求分析

1. 在不改变 `RichContent` 和 `Markdown` 调用 API 的前提下，让 Markdown 支持 DOMPurify 清洗后的 HTML 输出、GFM、任务列表、表格、代码块、数学公式和轻量图表。
2. 保留现有 `breaks` 参数语义，公告、通知和 FAQ 中需要换行即换行的内容仍能工作。
3. 所有 Markdown 输出必须经过 DOMPurify；新增 SVG/MathML 标签和属性只能服务公式/图表渲染，不能放开脚本执行或危险协议。
4. 新增依赖只限 new-api 当前已验证使用的 `marked` 和 `katex`，不引入更重的 Mermaid runtime，避免前台公告和通知渲染成本激增。
5. 该能力是纯前端展示增强，不改变后端公告、通知、法务文档、系统设置、Playground 消息、API 响应或权限语义。
6. 增加轻量测试覆盖，证明数学、流程图和危险 HTML 清洗能够同时成立。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| Markdown 组件 | `web/default/src/components/ui/markdown.tsx` | 从简化 ReactMarkdown 渲染升级为 new-api 的安全 HTML 渲染能力，并保持 `children/className/breaks` API。 |
| 前端依赖 | `web/default/package.json`、`web/default/bun.lock` | 新增 `marked`、`katex`；保留现有依赖，避免顺手移除造成锁文件大幅波动。 |
| 测试 | `web/default/src/components/ui/markdown.test.tsx` | 覆盖 KaTeX 输出、flow 图表输出、危险 HTML/URL 清洗和 breaks 语义。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮评审、实施结果和验证记录。 |

本轮不修改 `HtmlContent`、`RichContent`、通知接口、公告接口、法务文档加载、Playground 消息存储、AI response renderer、Go 后端和数据库。

### 风险评估

1. XSS 风险：new-api 组件允许更多 SVG/MathML 标签和属性。迁移时必须保留 DOMPurify 清洗，并用测试覆盖 `script`、事件属性和 `javascript:` 链接被移除。
2. Bundle 体积风险：`katex` CSS 和 `marked` 会增加前端包体积；相比 Mermaid runtime 更轻，且只替换原有 Markdown 组件，不在每个调用点重复引入。
3. 样式回归风险：ReactMarkdown 原本直接产出 React 节点，new-api 组件产出清洗后的 HTML。需要保留 `prose prose-neutral dark:prose-invert max-w-none` 基础类，并继承当前 `className`。
4. SSR/测试风险：DOMPurify、KaTeX 和 marked 依赖浏览器/DOM 行为。当前应用是客户端 SPA，测试可通过 DOM 环境或组件渲染验证；若单元测试环境无法提供 DOM，应将纯渲染函数以浏览器环境测试或通过 build/MCP 验证兜底。
5. 兼容风险：已有通知和公告中的普通 Markdown、表格和链接不能回退。迁移后用 MCP 实际访问首页、通知弹窗或 Dashboard 公告/FAQ 页面确认真实渲染。
6. 覆盖风险：不直接改 AI response renderer；Playground 助手消息仍走现有 stream markdown parser 和 CodeBlock，不把公告文档组件与模型输出渲染混为一谈。

### 方案评审

采用“底层 Markdown 组件替换 + API 保持 + 安全测试 + 页面验证”的方案：

1. 将 new-api `Markdown` 的 marked/DOMPurify/KaTeX 渲染能力迁入 NexusTok，并替换版权头、品牌命名和必要中文注释。
2. 保持 `MarkdownProps` 的 `children: string`、`className?: string`、`breaks?: boolean` 不变，确保 `RichContent`、FAQ、法务、更新检查等调用点无需改动。
3. 在 `web/default/package.json` 中新增 `marked`、`katex` 并用 Bun 更新锁文件。
4. 增加 `markdown.test.tsx`，至少覆盖公式渲染、flow 图表渲染、危险 HTML 清洗和 `breaks` 换行。
5. 运行 Markdown 单测、typecheck、build、`git diff --check`，并通过 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/` 以及一个真实使用 `RichContent/Markdown` 的页面，确认热更新生效、页面无控制台错误。

### 实施结果

- `web/default/src/components/ui/markdown.tsx` 已从简化 `react-markdown + remark-gfm` 升级为 `marked + DOMPurify + KaTeX` 渲染实现，保持 `children`、`className`、`breaks` 组件 API 不变。
- Markdown 现在支持 GFM、任务列表、表格、emoji shortcode、`[========]` 分页线、块级/行内 KaTeX 数学公式、`flow` 轻量流程图和 `seq` 轻量时序图。
- Markdown 输出仍统一经过 DOMPurify 清洗；为了让 Bun 单测和潜在非 DOM 环境也能执行，新增了非浏览器 fallback 清洗，剥离 `script/style/iframe/object/embed`、事件属性和 `javascript:` URL。真实浏览器运行时仍优先使用 DOMPurify。
- 新增 `renderMarkdownForTest` 作为测试入口，业务调用仍使用 `Markdown` 组件。
- `web/default/package.json` 和 `bun.lock` 已新增 `marked`、`katex`；保留原有 `react-markdown`、`remark-gfm` 依赖，避免在本轮顺手移除造成无关锁文件波动。
- 新增 `web/default/src/components/ui/markdown.test.tsx`，覆盖 KaTeX、flow SVG、危险 HTML 清洗和 breaks 语义。
- 没有修改 `RichContent`、`HtmlContent`、通知/公告接口、法务文档加载、Playground 消息渲染、AI response renderer、Go 后端和数据库。

### 验证记录

1. `cd web/default && bun test src/components/ui/markdown.test.tsx` 通过，4 个 Markdown 渲染与清洗用例成功。
2. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
3. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 11.9 秒内完成生产构建；构建产物包含 KaTeX CSS 与字体资源。
4. `git diff --check` 通过，未发现空白错误。
5. 使用 Chrome DevTools MCP 打开 `http://192.168.0.202:3003/`，页面正常加载；在浏览器上下文调用 `/api/user/login` 登录 `c1cada`，返回 HTTP 200、`success=true`、`uid=1`、`role=100`。
6. MCP 忽略缓存访问 `http://192.168.0.202:3003/about`，About 页面正常渲染 `No About Content Set` 默认内容、项目链接和页脚；该页面路径会加载 `RichContent/Markdown` 相关前端包，本轮 Markdown 替换没有造成页面崩溃。
7. MCP 重新忽略缓存访问 `http://192.168.0.202:3003/` 根页面，首页导航、hero、功能区和页脚均正常显示；请求 `/api/status`、`/api/notice`、`/api/home_page_content` 均返回 HTTP 200。
8. 当前 3003 环境没有配置包含公式或 flow 图表的真实公告/About 内容，因此新增富 Markdown 渲染分支由 `markdown.test.tsx` 覆盖；MCP 负责验证真实页面、热更新和运行时稳定性。
9. MCP 控制台检查 `error`、`warn`、`issue` 均为空。本轮验证期间临时启动 headless Chrome 连接 MCP，验证完成后已停止该临时会话。

## 本轮实施评审：AI 响应代码块折叠与下载体验原生化

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/components/ai-elements/code-block.tsx` 与当前 `web/default/src/components/ai-elements/code-block.tsx` 后确认，两边都已经具备 AI response renderer、Shiki/CodeMirror 代码展示和复制按钮，但 new-api 最新 `CodeBlock` 额外提供代码块 toolbar、长代码自动折叠、展开/收起按钮、下载按钮、标题和最大展开高度控制。NexusTok 当前 `response-renderer-blocks.tsx` 调用 `CodeBlock` 时只传入 `code/language/showLineNumbers`，长代码会完整撑开消息区域，Playground 或模型详情中包含长 JSON、SQL、Python、日志片段时阅读效率弱于 new-api。

该能力属于前端展示层增强，不改变 Playground 请求、流式解析、消息存储、Markdown 清洗、Relay 协议、计费或后端接口。它适合转为 NexusTok 原生 AI 展示能力：保留当前项目的语义色、Base UI 组件和代码高亮 fallback，同时吸收 new-api 的折叠和下载交互。

### 需求分析

1. 长代码块在 AI 响应中应默认折叠，避免单条消息把 Playground 或日志说明区域无限拉长。
2. 用户应能通过明确的图标按钮展开/收起长代码，并能下载代码内容到本地文件，便于调试上游返回的 JSON、脚本和配置片段。
3. 短代码块不应强制出现折叠按钮；复制按钮继续可用，且复制内容仍来自原始代码字符串。
4. `CodeBlockEditor` 仍服务编辑态 JSON/消息编辑器，不应被只读展示逻辑破坏；编辑态保留 `autoFocus/readOnly/onChange` 等 NexusTok 已新增能力。
5. 视觉上沿用当前默认前端的中性运维界面：1px 边界、语义 token、紧凑 toolbar；不引入 new-api 品牌、颜色或整文件覆盖。
6. 增加单元测试覆盖长代码默认折叠、展开按钮和下载按钮，避免后续 AI renderer 退回无折叠状态。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| AI 代码块组件 | `web/default/src/components/ai-elements/code-block.tsx` | 增加 toolbar frame、折叠高度、下载按钮、语言标题和可选折叠参数；保留 Shiki 展示路径与 CodeMirror 编辑路径。 |
| AI 响应渲染 | `web/default/src/components/ai-elements/response-renderer-blocks.tsx` | 对 AI 响应中的长 code fence 启用 new-api 同款默认折叠策略。 |
| 测试 | `web/default/src/components/ai-elements/code-block.test.tsx` | 覆盖长代码默认折叠、点击展开和下载文件名行为。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求、影响、风险、方案和后续验证。 |

本轮不修改 Go 后端、数据库、Relay、计费表达式、模型搜索、渠道编辑页、Markdown 文档渲染、Playground 请求构造和消息持久化。

### 风险评估

1. 公共组件回归风险：`CodeBlock` 被 AI response renderer、工具调用展示、Pricing API 示例和 Playground 专用消息内容复用。方案会保持默认 `showToolbar=false`，只有调用方显式传入时展示 toolbar；普通工具调用和 Pricing 示例默认只获得向后兼容的展示框。
2. Shiki/CodeMirror 行为风险：new-api 当前只读 CodeBlock 改用 CodeMirrorCodeView 会改变高亮路径。NexusTok 已对 Shiki fallback 做过本地适配，本轮保留 Shiki 作为只读展示核心，只在外层 frame 增加折叠高度和 toolbar，降低渲染回归。
3. 下载安全风险：下载只基于当前代码字符串创建 `text/plain` Blob，不使用远程 URL，不读取本地文件，不把内容写入后端。
4. i18n 风险：新增按钮 aria-label 和 tooltip 文案需要进入 en/zh/fr/ja/ru/vi 六语 locale，并运行 `bun run i18n:sync`。
5. 可访问性风险：折叠和下载按钮必须有 `aria-label`、`type="button"` 和 tooltip；复制按钮应补充默认 `aria-label`，避免图标按钮在屏幕阅读器中无名称。
6. 布局风险：折叠遮罩、toolbar 和绝对定位复制按钮不能覆盖代码内容或导致横向滚动异常；测试和 MCP 需要覆盖真实 Playground 页面加载。

### 方案评审

采用“保留 NexusTok Shiki 展示路径 + 吸收 new-api toolbar/折叠交互 + 小范围测试”的方案：

1. 在 `CodeBlock` 中扩展 props：`collapsedLines`、`defaultCollapsed`、`enableCollapse`、`filename`、`maxExpandedLines`、`maxCollapsedLines`、`showToolbar`、`title`，默认行为保持向后兼容。
2. 新增内部 `CodeBlockFrame`，统一只读和编辑态外框，让 toolbar、标题、end actions 和滚动区域复用同一结构；编辑态继续传入 `CodeBlockEditor` 的 `actions/autoFocus/readOnly`。
3. 只读展示继续使用现有 `highlightCode()` + `dangerouslySetInnerHTML`，保留语言 alias、Shiki fallback、语义色高亮和行号 transformer。
4. 在 AI `renderCodeBlock` 中按 new-api 策略启用：超过 14 行默认折叠，预览 14 行，展开最多 44 行，显示 toolbar 和语言标题。
5. 补充 `code-block.test.tsx`，用长代码验证折叠高度、展开后高度和下载文件名；避免测试依赖真实 Shiki worker。
6. 运行相关单测、typecheck、build、`git diff --check`，并通过 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/` 与 `/playground`，确认热更新和页面稳定。

### 实施结果

- `CodeBlock` 已扩展 `collapsedLines`、`defaultCollapsed`、`enableCollapse`、`filename`、`maxExpandedLines`、`maxCollapsedLines`、`showToolbar` 和 `title` 参数，默认 `showToolbar=false`，未显式开启的既有调用继续保持原展示语义。
- 新增 `CodeBlockFrame` 统一只读代码块和 `CodeBlockEditor` 的外框、toolbar、标题区、滚动区域和右侧动作位；编辑态继续保留 `autoFocus`、`readOnly`、`onChange`、`onKeyDown` 等 NexusTok 现有能力。
- 只读代码展示仍走 NexusTok 当前 `highlightCode()` + Shiki fallback，不改成 CodeMirror 只读渲染，避免影响已有语义色高亮和未知语言 fallback。
- AI response renderer 中的 code fence 现在超过 14 行会默认折叠，toolbar 显示语言标题、复制按钮、展开/收起按钮和下载按钮；展开后最大高度为 44 行。
- 复制按钮补齐默认 `aria-label`，调用方传入自定义 `aria-label` 时继续优先使用调用方文案。
- 新增 `resolveCodeBlockDisplayState` 纯函数集中计算折叠状态、预览高度、语言归一化和下载文件名，避免 UI 和测试各自实现规则。
- 新增 `web/default/src/components/ai-elements/code-block.test.ts`，覆盖长代码折叠、展开最大高度、短代码不折叠、关闭折叠和自定义下载文件名。
- 没有修改 Go 后端、数据库、Relay、计费表达式、模型搜索、渠道编辑页、Markdown 文档渲染、Playground 请求构造和消息持久化。

### 验证记录

1. `cd web/default && bun test src/components/ai-elements/code-block.test.ts` 通过，4 个代码块折叠/文件名用例成功。
2. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
3. `cd web/default && bunx prettier --write src/components/ai-elements/code-block.tsx src/components/ai-elements/response-renderer-blocks.tsx src/components/ai-elements/code-block.test.ts ../../docs/features/new-api-main-diff-analysis.md` 已格式化本轮触碰文件。
4. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en、zh、fr、ja、ru、vi 的 `missingCount` 与 `extrasCount` 均为 0，本轮未新增翻译缺口。
5. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 11.9 秒内完成生产构建。
6. `git diff --check` 通过，未发现空白错误。
7. 使用 Chrome DevTools MCP 打开 `http://192.168.0.202:3003/`，页面正常加载；在浏览器上下文调用 `/api/user/login` 登录 `c1cada`，返回 HTTP 200、`success=true`、`uid=1`、`role=100`，运行态构建指纹为 `rv.0000.nexustok-default`。
8. MCP 忽略缓存访问 `http://192.168.0.202:3003/playground`，页面加载成功；请求 `/api/status`、`/api/user/self`、`/api/notice`、`/api/user/models?group=default`、`/api/user/self/groups` 均返回 HTTP 200。
9. MCP 在临时浏览器 profile 中向 `playground_messages` 写入一条本地助手消息，内容包含 20 行 `ts` 代码块；reload 后页面真实渲染出 AI response code block，toolbar、语言标题、复制按钮、展开按钮和下载按钮均存在。
10. MCP 读取代码块 DOM，折叠态滚动区 `max-height` 为 `368px`，对应 14 行预览；点击 `Expand` 后按钮切换为 `Collapse`，`max-height` 切换为 `1088px`，对应 44 行最大展开高度。
11. MCP 拦截下载按钮验证其创建本地 Blob，文件名为 `code.typescript`，Blob URL 被及时 revoke；该操作没有触发后端请求。
12. MCP 验证结束后已恢复/清理临时 `playground_messages`，没有污染运行态用户数据。
13. MCP 控制台检查 `error`、`warn`、`issue` 均为空；本轮相关网络请求均为页面加载和只读 GET，没有触发模型请求或后端写接口。

## 本轮实施评审：渠道编辑页模型搜索与 new-api 编辑体验对齐

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`、`/opt/project/new-api-main/web/default/src/components/multi-select.tsx` 与当前 NexusTok 对应文件后确认，new-api 最新编辑渠道页已经采用右侧 `Sheet`、左侧分段导航、`Basic Information` / `Credentials` / `Models & Groups` / `Advanced Settings` 的分区体验。NexusTok 当前页面已吸收该主骨架，并在此基础上增加了账号池凭证模式、Codex OAuth、细粒度权限提示、模型元数据搜索和分页扫描追加等原生增强。

本轮用户反馈“搜索添加时不正确”，并希望编辑渠道页面向最新版 new-api 对齐。MCP 在 `http://192.168.0.202:3003/channels` 以渠道 `11111` 复现后确认：输入 `gpt-5.6` 并按 Enter 会把缺失的 `gpt-5.6-terra`、`gpt-5.6-luna` 追加到草稿，已存在的 `gpt-5.6-sol` 不重复追加，接口与批量追加主路径是可用的；但当前搜索区域把“逐个选择候选”“批量添加所有搜索命中”“自定义模型创建”放在同一个芯片输入框里，候选隐藏已选项后容易让管理员误以为只同步了一个或只添加了当前高亮项。new-api 的优势在于编辑抽屉结构更清晰、表单分区更克制；NexusTok 应保留远程搜索优势，同时把搜索添加语义做成更明确的原生能力。

### 需求分析

1. 编辑渠道页继续保留 new-api 风格的分段导航、紧凑表单和清晰分区，不回退到旧式长表单。
2. 模型搜索输入时必须清楚区分：当前页命中数、可新增数、已选择数、是否还会扫描更多分页结果，以及最终将添加哪些模型。
3. 按 Enter、点击搜索结果区按钮、存在高亮候选时的行为都应稳定：有真实匹配时优先执行批量补齐；没有匹配时才允许自定义模型创建，避免把 `gpt-5.6` 这种系列前缀误写成一个不可调用模型。
4. 候选列表不应让用户误以为“只会添加可见的一个候选”；批量添加按钮和预览需要使用同一套缺失模型计算结果。
5. 布局上将 `Models & Groups` 内部从嵌套卡片感较强的块状结构调整为更贴近 new-api 的轻量分组：模型选择、快捷操作、模型映射和分组各自清楚，但不制造卡片套卡片。
6. 本轮只修改前端展示和模型搜索草稿交互，不保存表单、不改渠道后端接口、不改模型同步接口、不改数据库和计费。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道编辑抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 调整模型区布局、搜索结果提示、批量追加入口和 Enter 行为提示，保留现有账号池与权限能力。 |
| 多选组件 | `web/default/src/components/multi-select.tsx` | 如需修复高亮候选与搜索提交冲突，只在已有 props 语义内小改，不改变普通多选默认行为。 |
| 模型搜索纯函数 | `web/default/src/features/channels/lib/model-search.ts` | 如需补充“按钮文案/预览/实际添加”一致性的纯函数，保持无副作用。 |
| 测试 | `web/default/src/components/multi-select.test.ts`、`web/default/src/features/channels/lib/model-search.test.ts` | 覆盖 Enter 搜索提交、真实匹配时不创建自定义值、批量追加数量一致性。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 新增或调整搜索提示文案时补齐六语翻译并运行同步。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮评审、实施结果和 MCP 验证。 |

本轮不修改 Go 后端、`/api/models/search`、`/api/channel` 保存逻辑、模型元数据同步、账号池、Relay 调度、计费表达式、数据库迁移和权限后端。

### 风险评估

1. 搜索语义回归风险：MultiSelect 是公共组件，直接改变默认 Enter 或创建行为会影响用户分组、多模型选择和其他表单。方案只通过渠道编辑页已有 props 启用搜索提交逻辑，默认组件行为保持不变。
2. 已选模型误删风险：芯片输入框空值时 Backspace/Delete 已被保护，本轮不能移除该保护；否则管理员搜索后可能误删最后一个模型。
3. 重复模型风险：搜索追加、手动创建、预设填充和复制粘贴都必须继续按 trim + 大小写不敏感去重，不能引入 `GPT-5.6-TERRA` 与 `gpt-5.6-terra` 并存。
4. 分页误导风险：第一页候选可能少于后端 `total`。如果按钮显示“添加全部”，实际追加前仍必须扫描全部分页；UI 需要明确“会检查更多结果”，避免管理员误判。
5. 权限风险：普通写权限只能编辑非敏感字段；模型搜索添加应继续受 `canEditBasicFields` 控制，不能绕过 `canSensitiveWrite` 与只读提示。
6. 布局风险：编辑抽屉本身内容密集，收敛卡片嵌套时不能造成按钮、芯片和说明文字在移动端重叠；需要 MCP 截图检查桌面和至少一个窄宽度。
7. i18n 风险：新增动态文案必须补齐六语 locale；否则 `bun run i18n:sync` 会出现 missing key。

### 方案评审

采用“保留 NexusTok 搜索增强 + 向 new-api 收敛编辑抽屉信息架构 + 小步修复搜索入口”的方案：

1. 保留当前 `Sheet + ChannelEditorNav`、账号池、Codex、权限和模型元数据搜索等 NexusTok 原生能力；不整文件覆盖 new-api。
2. 将模型搜索结果 header 改成更明确的状态行：展示当前已加载命中、可新增、已选择，并在按钮文案中说明“添加搜索匹配”而非笼统 `Add`。
3. Footer 继续展示将添加的缺失模型预览，并在存在后端未扫描分页时提示点击会先扫描全部结果；预览、按钮可用性和实际追加统一使用 `buildModelSearchAppendPlan` / `getMissingModelSearchMatches`。
4. 检查 `MultiSelect` 的 Enter 捕获路径，确保渠道编辑页传入 `submitSearchOnEnterWithMatches` 和 `submitSearchOnEnterWhenHighlighted` 时，有匹配或仍在加载就提交搜索；无匹配时才保留自定义创建。
5. 把 `Models & Groups` 内部布局从大块嵌套卡片调整成轻量分组：模型选择区、快捷操作区、映射区、分组区通过边界和 gap 分隔，避免与外层 `SideDrawerSection` 形成卡片套卡片。
6. 补充/更新单元测试，运行 `bun test`、`bun run typecheck`、`bun run i18n:sync`、`bun run build`、`git diff --check`；最后通过 Chrome DevTools MCP 访问 `http://192.168.0.202:3003/` 和 `/channels`，复现 `gpt-5.6` 搜索添加、截图确认页面已热更新。

### 实施结果

- 编辑渠道页模型搜索批量按钮文案从笼统 `Add` / `Add all {{count}} new match(es)` 调整为更明确的 `Add {{count}} search match(es)`；当命中都已在渠道中时显示禁用态 `No new matches`，避免管理员误以为仍可追加。
- 搜索结果弹层新增说明：`Use the button to add every matching model. Selecting a row below adds only that one model.`，明确“按钮=批量添加所有匹配模型”“候选行=只选择单个模型”的区别。
- `Models & Groups` 内部布局改为轻量分组：模型选择区保留徽标和远程搜索，快捷操作区用紧凑的 muted 分组承载 Fill/Fetch/Copy/Clear，模型映射和用户分组改为边界分隔而不是嵌套卡片。
- 保留 NexusTok 已有的模型搜索原生能力：OpenAI/Codex 渠道默认搜索 OpenAI 模型库，点击批量添加前会扫描全部分页结果，搜索追加继续按 trim + 大小写不敏感去重。
- 未修改 `MultiSelect` 默认行为、后端 `/api/models/search`、渠道保存接口、账号池、Relay 调度、模型同步任务、计费、数据库和权限后端。
- 新增两个按钮状态文案，并已补齐 `en`、`zh`、`fr`、`ja`、`ru`、`vi` 六语翻译。

### 验证记录

1. `cd web/default && bun test src/features/channels/lib/model-search.test.ts src/components/multi-select.test.ts` 通过，49 个模型搜索和 MultiSelect 键盘/创建/去重用例成功。
2. `cd web/default && bun run i18n:sync` 通过；同步报告中 `en`、`zh`、`fr`、`ja`、`ru`、`vi` 的 `missingCount=0`、`extrasCount=0`。
3. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
4. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.3 秒内完成生产构建。
5. `git diff --check` 通过，未发现空白错误。
6. 使用 Chrome DevTools MCP 忽略缓存访问 `http://192.168.0.202:3003/channels`，页面正常加载并能打开渠道 `11111` 的编辑抽屉；本轮无需重启容器，热更新已生效。
7. MCP 在编辑抽屉模型输入框搜索 `gpt-5.6`，弹层显示 `3 matched · 2 new · 1 already selected`、按钮 `Add 2 search match(es)`、说明 `Use the button to add every matching model. Selecting a row below adds only that one model.`，预览将添加 `gpt-5.6-terra, gpt-5.6-luna`。
8. MCP 点击批量按钮后，草稿模型从 3 个变成 5 个，并出现 toast `Added 2 model(s) from search`；再次搜索同一关键词显示 `3 matched · 0 new · 3 already selected`，按钮为禁用态 `No new matches`，footer 显示 `No new search results to add`。
9. MCP 验证后点击 `Cancel` 退出抽屉，未触发 `Update Channel`；随后调用 `/api/channel/1` 读取后端详情，`models` 仍为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`，确认验证草稿没有写入后端。
10. MCP 网络记录显示 `/api/channel/1`、`/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50`、`/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=100` 均返回 HTTP 200；`/api/prefill_group?type=model` 到 `/api/prefill_group/?type=model` 的 301 为既有路径规范化。
11. MCP 桌面截图已保存到 `/tmp/nexustok-channel-edit-after-desktop.png`，390px 窄屏截图已保存到 `/tmp/nexustok-channel-edit-after-mobile.png`；窄屏下搜索结果、按钮和说明完整展示，没有重叠。
12. MCP 控制台没有 JavaScript `error` 或 `warn`；DevTools 仍报告既有表单 a11y issue（`autocomplete`、`label for` 关联），本轮没有引入新的运行时异常。

## 本轮实施评审：系统设置页面统一框架原生化

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/features/system-settings` 与当前 NexusTok 后确认，new-api 默认前端已经把系统设置页升级为统一页面框架：`SettingsPage` 通过 `SectionPageLayout` 渲染当前设置分区标题，`settings-page-context.tsx` 提供标题状态槽和页面动作槽，`FormDirtyIndicator` 以标题旁紧凑状态标识展示未保存变化，`settings-form-layout.tsx` 提供统一表单网格、开关行和控制组布局。

当前 NexusTok 已经吸收了大量系统设置业务能力，例如角色策略、Token Limits、Grok 设置、Waffo/Waffo Pancake、渠道亲和、性能监控等；但页面框架仍停留在旧式实现：`SystemSettings` 外层手动包裹 `Main`，`SettingsPage` 只负责加载数据和渲染内容，子页面标题没有进入统一布局，`content` 与 `operations` 两类设置仍手写加载/滚动框架，未保存提示以表单内 Alert 占据首行。new-api 的优势适合转成 NexusTok 的原生公共能力，但不能覆盖各设置表单的业务逻辑和本项目额外设置项。

### 需求分析

1. 系统设置所有分区页面应使用统一页面框架：顶部展示当前分区标题，内容区使用一致滚动容器，后续保存/重置动作可以进入页面动作槽。
2. 需要保留 NexusTok 当前七类设置入口：站点、认证、模型、安全、计费、内容、运维；不能只覆盖 new-api 中已有的子集。
3. 需要保留各 registry 的 `descriptionKey`，因为当前侧边栏/导航仍可能依赖描述信息；新增 `getSectionMeta` 不能破坏既有 `getSectionNavItems` 与 `getSectionContent`。
4. `content` 与 `operations` 当前有额外解析逻辑：内容页需要兼容旧键 `Announcements`、`ApiInfo`、`FAQ`、`UptimeKumaUrl`/`UptimeKumaSlug`；运维页需要从 `useStatus()` 传入版本和启动时间。这些逻辑应通过 `SettingsPage` 的 `resolveSettings` 和 `extraArgs` 原生承接。
5. 未保存提示应优先显示在标题旁，让设置表单首屏更紧凑；但如果组件脱离 `SettingsPage` 使用，仍需保留旧 Alert fallback，避免提示静默消失。
6. 本轮先建立公共框架和最小接入，不一次性重写所有具体设置表单为 `SettingsForm`，降低业务回归风险。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 系统设置外层路由 | `web/default/src/features/system-settings/index.tsx` | 移除外层 `Main` 包裹，让子页 `SettingsPage` 自己使用 `SectionPageLayout`；避免 Main 嵌套 Main。 |
| 通用页面框架 | `web/default/src/features/system-settings/components/settings-page.tsx`、新增 `settings-page-context.tsx` | 接入 `SectionPageLayout`、标题状态槽、页面动作槽、可选加载文案、`resolveSettings` 和泛型 `extraArgs`。 |
| 通用表单布局 | 新增 `web/default/src/features/system-settings/components/settings-form-layout.tsx` | 提供后续表单原生化所需的统一网格、开关行、控制组和表单 wrapper；本轮不强制迁移所有表单。 |
| 分区 registry | `web/default/src/features/system-settings/utils/section-registry.ts` 与各 `section-registry.tsx` | 保留 `descriptionKey`，新增 `getSectionMeta` 导出，供标题区读取当前分区名称。 |
| 设置入口页面 | `site/auth/models/security/billing/content/operations/index.tsx` | 七个入口统一使用 `SettingsPage`，content/operations 的 legacy fallback 与 status extra args 通过新参数承接。 |
| 未保存提示 | `web/default/src/features/system-settings/components/form-dirty-indicator.tsx` | 标题旁展示 `Unsaved changes` badge；无 provider 时继续使用原 Alert。 |
| i18n | `web/default/src/i18n/locales/*.json` | 复用已有 `Unsaved changes`、`Loading settings...`、`Save Changes`、`Saving...`、`Reset` 等文案；如发现缺口再补齐六语。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮评审、实施结果和真实页面验证。 |

本轮不修改 Go 后端、数据库迁移、系统设置 API、权限后端、计费表达式、模型同步、渠道编辑页、Relay 调度和具体设置项保存语义。

### 风险评估

1. 页面嵌套风险：如果保留当前 `SystemSettings -> Main`，再让 `SettingsPage` 使用 `SectionPageLayout -> Main`，会形成嵌套滚动和重复 header。方案必须同步移除外层 `Main`。
2. 标题缺失风险：所有设置入口都需要传入 `getSectionMeta`；否则统一框架只能显示默认标题或空标题。方案通过 registry 统一暴露 meta，避免每个入口手写映射。
3. content/operations 回归风险：这两个入口当前手写逻辑包含 legacy 键兼容和运行状态参数。迁移时必须使用 `resolveSettings` 和 `extraArgs` 逐项保留。
4. `descriptionKey` 兼容风险：new-api registry 删除了 `descriptionKey`，但 NexusTok 当前保留描述更利于导航和报告追踪。本轮只新增 meta，不删除描述字段。
5. 未保存提示风险：`FormDirtyIndicator` 如果只改成 portal，脱离页面框架时会完全不显示。本轮让 portal 组件返回是否已挂载，未挂载时 fallback 到旧 Alert。
6. 布局风险：标题旁 badge 和动作槽必须在窄屏下换行而不挤压标题；`SectionPageLayout.Title` 的容器需要允许标题占据剩余宽度并安全截断。
7. i18n 风险：新增文案必须完整进入 en/zh/fr/ja/ru/vi；本轮优先复用既有 key，最终仍运行 `bun run i18n:sync` 验证。

### 方案评审

采用“先统一页面框架，再逐步迁移具体表单”的低风险方案：

1. 将 `SectionPageLayout` 与 new-api 对齐，补充 `fixedContent` 能力，并让标题容器 `flex-1`，为标题状态 badge 预留空间。
2. 新增 NexusTok 品牌化的 `settings-page-context.tsx`，提供 `SettingsPageProvider`、`SettingsPageActionsPortal`、`SettingsPageTitleStatusPortal`、`SettingsPageFormActions` 和 `useSuppressSettingsSectionHeader`。按钮图标先沿用 lucide，因为项目当前前端已大量使用 lucide。
3. 修改 `SettingsPage` 使用 `SectionPageLayout`，支持 `getSectionMeta`、`loadingMessage`、`resolveSettings` 和泛型 `extraArgs`；加载态也保留当前分区标题。
4. 新增 `settings-form-layout.tsx` 作为后续表单迁移的公共布局，但本轮只让类型和构建通过，不大规模替换具体表单 JSX。
5. 修改 `SettingsSection` 支持页面框架内隐藏重复 section header，同时保留 `description` prop 兼容当前调用。
6. `createSectionRegistry` 新增 `getSectionMeta`，各分区 registry 导出对应 meta；七个入口页面统一传入 `getSectionMeta`。
7. `content` 入口将 legacy option fallback 提取为 `resolveContentSettings()` 并传给 `SettingsPage`；`operations` 入口通过 `extraArgs` 传递版本和启动时间。
8. `FormDirtyIndicator` 优先把 `Unsaved changes` 渲染到标题状态槽；没有 provider 时继续渲染旧 Alert。
9. 运行 `bun run i18n:sync`、`bun run typecheck`、`bun run build`、相关单测和 `git diff --check`；最后用 Chrome DevTools MCP 登录并访问 `http://192.168.0.202:3003/system-settings/site/system-info`，修改一个字段但不保存，确认标题、动作槽、未保存状态和页面热更新。

### 实施结果

- `SectionPageLayout` 已补齐 `fixedContent` 能力，并让标题区域占据剩余宽度、动作区右对齐，后续需要固定内容或页头动作时可以复用同一布局。
- 新增 `settings-page-context.tsx`，提供 `SettingsPageProvider`、`SettingsPageActionsPortal`、`SettingsPageTitleStatusPortal`、`SettingsPageFormActions`、`useSuppressSettingsSectionHeader` 和标题状态容器 hook。该能力用于后续把设置页保存/重置按钮迁移到页头动作区。
- 新增 `settings-form-layout.tsx`，提供 `SettingsForm`、`SettingsFormGrid`、`SettingsFormGridItem`、`SettingsSwitchItem`、`SettingsSwitchRow`、`SettingsSwitchField`、`SettingsControlGroup`、`SettingsControlChildren` 等公共布局组件，作为后续表单逐步收敛的原生基础。
- `SettingsPage` 已使用 `SectionPageLayout` 渲染当前分区标题，并支持 `getSectionMeta`、`loadingMessage`、`resolveSettings` 和泛型 `extraArgs`。加载态也会保留当前分区标题，不再只显示孤立 loading 文本。
- `SystemSettings` 外层已移除手写 `Main`，避免统一框架接入后形成重复页面壳。
- `createSectionRegistry` 在保留 NexusTok 现有 `descriptionKey` 的基础上新增 `getSectionMeta`；site/auth/models/security/billing/content/operations 七类设置 registry 均已导出对应 meta。
- site/auth/models/security/billing 五个已有 `SettingsPage` 入口已传入 `getSectionMeta`；content 和 operations 两个原手写入口也已迁移到统一 `SettingsPage`。
- content 入口保留旧配置键兼容：`Announcements`、`ApiInfo`、`FAQ`、`UptimeKumaUrl`/`UptimeKumaSlug` 仍会在新键缺失时补入当前设置草稿。
- operations 入口保留 `useStatus()` 运行信息传递，`update-checker` 分区仍能显示当前版本和启动时间。
- `FormDirtyIndicator` 在 `SettingsPage` 内会把 `Unsaved changes` 以紧凑 `Badge` 渲染到标题旁；脱离 `SettingsPageProvider` 时继续 fallback 为旧 Alert，避免提示静默消失。
- 本轮未修改任何系统设置保存 API、后端权限、数据库、计费表达式、模型同步、渠道编辑页或 Relay 行为。

### 验证记录

1. `cd web/default && bun test src/features/system-settings` 通过，17 个系统设置相关单测成功，覆盖数字字段、模型定价格式、Grok 表单转换、角色策略工具和 Waffo Pancake 凭据解析。
2. `cd web/default && bun run typecheck` 通过，`tsc -b` 无类型错误。
3. `cd web/default && bun run i18n:sync` 通过；同步报告显示 en、zh、fr、ja、ru、vi 的 `missingCount=0`、`extrasCount=0`。fr/ja/ru/vi 仍存在既有 `untranslatedCount`，本轮未新增翻译缺口。
4. `cd web/default && bun run build` 通过，Rsbuild v2.0.1 在 12.2 秒内完成生产构建。
5. `git diff --check` 通过，未发现空白错误。
6. 使用 Chrome DevTools MCP 启动临时 Chrome，访问 `http://192.168.0.202:3003/system-settings/site/system-info`，通过 `/api/user/login` 登录 `c1cada`，返回 HTTP 200、`success=true`、`uid=1`、`role=100`。
7. MCP 忽略缓存重新访问 `http://192.168.0.202:3003/system-settings/site/system-info`，页面标题区显示 `System Information`，表单内重复的 `SettingsSection` 标题已被统一框架抑制；表单字段继续正常展示。
8. MCP 修改 `System Name` 字段但不保存，标题变为 `System Information Unsaved changes`，证明 `FormDirtyIndicator` 已进入标题状态槽；页面没有出现旧的表单首行 Alert。
9. MCP 点击 `Reset` 清理草稿后访问 `http://192.168.0.202:3003/system-settings/content/announcements`，页面标题区显示 `Announcements`，原手写 content 入口已统一到新框架，公告列表和按钮仍正常展示。
10. MCP 访问 `http://192.168.0.202:3003/system-settings/operations/update-checker`，页面标题区显示 `System maintenance`，并正常展示 `Current version` 与 `Uptime since`，确认 `extraArgs` 仍把运行状态传给 `UpdateCheckerSection`。
11. MCP 切换 390px 移动视口后访问 `site/system-info`，再次修改 `System Name`，标题区显示 `System Information Unsaved changes`，表单按钮和字段没有重叠；截图保存到 `/tmp/nexustok-system-settings-frame-mobile.png`。
12. MCP 网络记录中系统设置验证相关请求均为 GET：`/api/status`、`/api/user/self`、`/api/notice`、`/api/option/`；未出现系统设置保存接口请求，确认验证过程未污染运行态配置。
13. MCP 控制台没有 JavaScript `error`、`warn` 或 DevTools `issue`；仅有 i18next 宣传 info 和本地构建指纹 debug。

## 本轮实施评审：系统信息表单页头动作与主题切换保护

### 差异来源

在上一轮已经把系统设置页统一到 `SettingsPage` / `SectionPageLayout` 之后，继续对照 `/opt/project/new-api-main/web/default/src/features/system-settings/general/system-info-section.tsx` 与当前 NexusTok 同名文件发现，new-api 的 `SystemInfoSection` 进一步使用了 `SettingsForm`、`SettingsFormGrid`、`SettingsFormGridItem` 和 `SettingsPageFormActions`：保存/重置按钮被迁移到页面标题右侧动作槽，表单本体只保留字段内容；同时，`theme.frontend` 会被延后到最后提交，成功后通过 `window.location.replace('/')` 回首页，避免在系统设置深层路由切换到另一套前端后出现 404 或历史记录回退到失效路由。

当前 NexusTok 已经具备对应公共组件，但 `SystemInfoSection` 仍在表单底部渲染 `Save Changes` / `Reset`，没有使用页头动作槽；主题切换也直接和其它字段一起提交。该页面是系统设置中最能代表整体编辑体验的入口，适合作为第一批表单动作迁移样板。

### 需求分析

1. 将 `System Information` 表单的保存/重置动作迁移到统一页面动作槽，延续 new-api 设置页“标题 + 动作 + 内容”的信息架构。
2. 表单内容改用 NexusTok 已新增的 `SettingsForm` / `SettingsFormGrid`，让站点基本信息字段在桌面端形成更紧凑的两列布局，长文本区域保持全宽。
3. 保留 NexusTok 品牌文案：`SystemName` 占位仍为 `NexusTok`，首页内容占位仍为 `Welcome to our NexusTok...`，不能把 new-api 的 `New API` 文案带入当前项目。
4. 保留现有保存语义：继续只提交 dirty 字段，`ServerAddress` 继续去除尾部 `/`，权限不足时保存按钮禁用并保留禁用原因。
5. 增加前端主题切换保护：如果同时修改多个字段，先保存其它字段，全部成功后再提交 `theme.frontend`；主题切换成功后回首页并 replace 当前历史，避免切换到 classic/default 时深层设置路由失效。
6. 本轮只迁移 `SystemInfoSection`，不一次性修改所有系统设置表单，降低跨设置项回归风险。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 系统信息表单 | `web/default/src/features/system-settings/general/system-info-section.tsx` | 迁移保存/重置到页头动作槽，使用统一表单网格，增加主题切换保存顺序保护。 |
| 设置页动作上下文 | `web/default/src/features/system-settings/components/settings-page-context.tsx` | 复用已有 `SettingsPageFormActions`，补充可选 `saveDisabledReason`，让页头保存按钮迁移后仍能呈现无权限禁用原因。 |
| 设置表单布局 | `web/default/src/features/system-settings/components/settings-form-layout.tsx` | 复用已有网格和全宽项能力，验证其在真实表单中的表现。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮评审、实施结果和 MCP 验证。 |

本轮不修改 Go 后端、数据库迁移、系统设置 API、权限后端、其它系统设置表单、i18n 文案、渠道编辑页、模型同步、计费表达式和 Relay 调度。

### 风险评估

1. 保存触发风险：页头按钮位于 form DOM 内但视觉上通过 portal 渲染到页头。需要确认 `SettingsPageFormActions` 的 `onSave={handleSubmit}` 能正常触发 React Hook Form 校验与提交，而不会触发原生表单重复提交。
2. 权限风险：迁移按钮后不能丢失 `updateOption.canUpdate` 与 `disabledReason`，否则无权限用户可能看到可点保存按钮。
3. 主题切换风险：`theme.frontend` 涉及前端产物切换，错误顺序可能在其它字段失败时先切主题，造成用户停在另一套前端不存在的路由。方案必须先保存其它字段，全部成功后再保存主题。
4. 脏状态风险：主题保存失败或被跳过时需要把提交数据改回已保存默认值，避免 `useSettingsForm` 把未实际保存的主题写入 baseline。
5. 布局风险：两列网格在桌面端更紧凑，但文本域和法律条款必须保持可读，全宽字段不能被挤压；移动端必须单列且按钮不重叠。
6. 品牌回归风险：不能照搬 new-api 的 `New API` placeholder，应保留 NexusTok。
7. 验证风险：真实保存系统名会影响运行环境品牌；MCP 验证应优先修改后重置，不触发保存接口。主题切换保护通过代码审查和非破坏性路径验证覆盖，避免实际切换运行环境前端。

### 方案评审

采用“迁移一个样板表单 + 保留业务语义 + 非破坏性运行态验证”的方案：

1. 从 `SystemInfoSection` 移除底部按钮和 `Button` / `RotateCcw` 直接依赖，导入 `SettingsForm`、`SettingsFormGrid`、`SettingsFormGridItem` 和 `SettingsPageFormActions`。
2. 在 `<Form {...form}>` 内使用 `<SettingsForm onSubmit={handleSubmit}>`，并将 `<SettingsPageFormActions>` 放在表单顶部，让它通过 portal 渲染到页头动作槽。
3. `SettingsPageFormActions` 的保存禁用条件设置为 `isSubmitting || updateOption.isPending || !updateOption.canUpdate`，重置禁用条件为 `!isDirty`；保存按钮继续显示 `Save Changes` / `Saving...`，重置按钮继续显示 `Reset`。
4. 表单字段包进 `SettingsFormGrid`，短字段自然两列，`HomePageContent` 使用 `SettingsFormGridItem span='full'` 保持全宽，其它较长 textarea 依赖布局规则在桌面端自动全宽或按网格安全换行。
5. 迁移 new-api 的主题保存顺序保护，但把英文注释改为中文，解释为何要最后保存主题和成功后回首页。
6. 保留 NexusTok 的 `NexusTok`、`Welcome to our NexusTok...` 占位文案，避免品牌回退。
7. 运行系统设置单测、typecheck、i18n sync、build、`git diff --check`，并用 MCP 访问 `http://192.168.0.202:3003/system-settings/site/system-info`：确认页头有保存/重置按钮、修改字段后出现 `Unsaved changes`、重置后不触发保存接口、桌面和 390px 移动视口布局正常。

### 实施结果

1. `SystemInfoSection` 已使用 `SettingsForm`、`SettingsFormGrid`、`SettingsFormGridItem` 和 `SettingsPageFormActions`，保存/重置按钮从表单底部迁移到页面标题动作槽，表单字段在桌面端保持更紧凑的网格布局，`Home Page Content` 保持全宽。
2. `SettingsPageFormActions` 增加可选 `saveDisabledReason`，保存按钮禁用时继续通过 `title` 暴露权限禁用原因，避免动作迁移后丢失原有权限反馈。
3. `theme.frontend` 已改为最后提交：其它 dirty 字段全部提交成功后才提交主题；主题保存成功后通过 `window.location.replace('/')` 回首页；其它字段失败或主题保存失败时会把提交数据中的主题值恢复为后端已保存默认值，避免表单基线误认为主题已保存。
4. 保留 NexusTok 原生品牌占位：`SystemName` 仍为 `NexusTok`，`HomePageContent` 仍为 `Welcome to our NexusTok...`，没有把 new-api 的品牌文案带入当前项目。
5. 本轮未修改后端接口、数据库迁移、权限判定、其它系统设置表单、渠道编辑页、模型搜索逻辑和计费表达式。

### 验证记录

1. 已运行 `cd web/default && bun test src/features/system-settings`，17 个测试通过。
2. 已运行 `cd web/default && bun run typecheck`，TypeScript 检查通过。
3. 已运行 `cd web/default && bun run i18n:sync`，六语 missing/extras 为 0；法语、日语、俄语、越南语仍有既有 untranslated 计数，本轮未新增。
4. 已运行 `cd web/default && bun run build`，生产构建通过。
5. 已运行 `git diff --check`，未发现空白错误。
6. MCP 打开 `http://192.168.0.202:3003/system-settings/site/system-info`，使用账号 `c1cada` 登录成功，页面标题显示 `System Information`。
7. MCP 桌面视口确认标题右侧存在 `Reset` 和 `Save Changes`，初始 `Reset` 为禁用状态，表单底部不再渲染重复按钮。
8. MCP 修改 `System Name` 为 `NexusTok-verify` 后，标题显示 `System Information Unsaved changes`，`Reset` 变为可用，未点击保存。
9. MCP 点击 `Reset` 后，字段恢复为 `NexusTok`，`Unsaved changes` 消失，`Reset` 回到禁用状态，并出现 `Form reset to saved values` 提示。
10. MCP 网络请求记录仅包含 `GET /api/status`、`GET /api/user/self`、`GET /api/notice`、`GET /api/option/`；没有出现保存接口请求，确认验证未污染运行态配置。
11. MCP 控制台 `error`、`warn`、`issue` 均为空。
12. MCP 保存桌面截图 `/tmp/nexustok-system-info-actions-desktop.png`；切换 390px 移动视口后确认页头动作仍可访问、表单单列展示，截图保存为 `/tmp/nexustok-system-info-actions-mobile.png`。
13. 主题切换保护未在运行态执行真实保存，避免改变当前验证环境的前端主题；该风险通过代码审查、typecheck、构建和非破坏性页面验证覆盖。

## 本轮实施评审：渠道编辑页模型搜索添加与 new-api 模型区对齐

### 差异来源

用户反馈“搜索添加时不正确”，并指出最新版 new-api 的编辑渠道页面体验较好，要求将当前项目的编辑渠道页面与 `/opt/project/new-api-main` 对齐。对照当前 NexusTok 与 new-api 的 `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` 后发现：

1. new-api 的模型区域采用清晰的边界分组：`Models` 与 `Quick actions` 位于同一个浅背景边框块，`Model Mapping` 和 `Groups` 分别使用独立边框块，信息层次更清楚。
2. NexusTok 在 new-api 基础上已经原生增强了模型元数据搜索、供应商过滤、批量追加、预设分组、权限禁用原因、模型映射风险提示和账号池能力；这些是当前项目优势，不能回退或整段覆盖。
3. MCP 真实访问 `http://192.168.0.202:3003/channels` 并编辑渠道 `11111` 时，`/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=100` 返回 3 条：`gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol`。渠道当前已有 `gpt-5.6-sol`，因此页面显示 `3 matched · 2 new · 1 already selected`，批量按钮补齐 2 个新模型是正确结果。
4. 复现中发现更细的交互风险：`MultiSelect` 在渠道模型搜索中开启了 `submitSearchOnEnterWhenHighlighted`，导致有高亮候选时 Enter 也会批量追加全部搜索结果，而不是选择高亮的单个候选。这与页面提示“Selecting a row below adds only that one model.”以及常见 Combobox 键盘预期冲突，容易被理解为“搜索添加不正确”。

### 需求分析

1. 保留当前 NexusTok 的模型库搜索能力：按渠道类型推导供应商、从 `/api/models/search` 拉取模型元信息、识别已选模型、批量补齐所有搜索命中、继续支持自定义模型输入。
2. 修复搜索添加键盘语义：当下拉中存在高亮候选时，Enter 应交给 Combobox 选择该单个候选；当没有高亮候选且输入仍为搜索词时，Enter 才可触发搜索批量追加；点击批量按钮始终追加全部未选命中。
3. 与 new-api 对齐模型区视觉结构：将 `Models` 字段和 `Quick actions` 收进同一个边框分组，`Model Mapping` 与 `Groups` 使用独立边框分组，让编辑渠道页面更接近最新版 new-api 的信息层级。
4. 保留 NexusTok 原生差异：权限禁用原因、账号池模式、模型映射风险提示、搜索统计、供应商徽标、批量追加预览、预设分组、`copyChipOnClick`、隐藏已选搜索结果、已有模型去重均不可丢失。
5. 本轮不保存真实渠道配置；MCP 验证只在抽屉草稿内搜索、添加、取消，避免污染运行态渠道模型列表。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道模型搜索键盘行为 | `web/default/src/components/multi-select.tsx` | 已有通用能力保持不变，渠道页不再为高亮候选开启批量 Enter。若调整单元测试，只覆盖 helper 的预期语义。 |
| 渠道编辑抽屉模型区 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 模型区视觉结构向 new-api 的边框分组对齐，同时保留 NexusTok 的搜索、权限和风险提示能力。 |
| 前端测试 | `web/default/src/components/multi-select.test.ts`、`web/default/src/features/channels/lib/model-search.test.ts` | 用现有测试覆盖模型搜索去重、批量追加和 Enter 高亮行为；视实现增补必要断言。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录需求分析、影响范围、风险评估、方案评审、实施结果和 MCP 验证。 |

本轮不修改 Go 后端、数据库、模型同步接口、渠道保存 API、账号池 API、Relay 调度、计费表达式、模型元数据表结构、权限后端和 new-api 源码。

### 风险评估

1. 键盘行为风险：如果直接禁用 Enter 搜索提交，管理员可能失去“输入前缀后回车批量添加”的快捷路径。方案只在有高亮候选时交给单选，无高亮时仍允许批量提交。
2. 搜索追加风险：批量按钮必须继续按后端搜索全量分页扫描，不能只添加当前第一页或当前可见行；`gpt-5.6` 三模型场景必须保持补两个新模型的正确结果。
3. 自定义模型风险：无候选的自定义模型仍需可输入和创建，不能因为修复搜索追加而禁用 `custom-model` 类场景。
4. 视觉对齐风险：new-api 的边框块不能照搬成破坏 NexusTok 侧栏导航和权限提示的结构；需要用现有 `gap`、`Button`、`Badge`、`Alert`、`MultiSelect` 等组件组合，避免新增不必要抽象。
5. 权限风险：模型区分组移动后，所有涉及写入草稿的按钮仍需遵守 `canEditBasicFields`、`permissions.canOperate` 和禁用原因提示。
6. 验证风险：编辑渠道保存会改变真实配置；本轮 MCP 必须只验证草稿行为并取消抽屉，最后通过接口确认渠道 `models` 仍未被保存修改。

### 方案评审

采用“小范围键盘语义修复 + 模型区结构对齐 + 非破坏性 MCP 验证”的方案：

1. 在渠道模型 `MultiSelect` 调用处移除 `submitSearchOnEnterWhenHighlighted`，保留 `submitSearchOnEnterWithMatches`、`onSearchSubmit` 和批量按钮。这样有高亮候选时 Enter 选择单条，无高亮时 Enter 仍可批量追加搜索命中。
2. 保留 `allowCreateWithMatches={false}`，避免输入 `gpt-5.6` 这类系列前缀时创建一个不完整自定义模型；无候选时仍允许创建自定义模型。
3. 将模型字段和快捷操作包进 `border-border/60 bg-muted/10 rounded-lg border p-4` 的同一分组；`Quick actions` 使用轻量分隔线与模型字段分开，贴近 new-api 的编辑页结构。
4. 将 `Model Mapping` 和 `Groups` 由 `border-t pt-5` 调整为独立 `border-border/60 rounded-lg border p-4` 分组，减少长抽屉中的视觉漂移，提升扫描效率。
5. 不引入新接口、不改后端、不新增依赖；如新增可见文案则补齐六语 i18n 并运行 `bun run i18n:sync`，优先复用现有文案避免翻译面扩大。
6. 验证顺序：先跑 `bun test src/components/multi-select.test.ts src/features/channels/lib/model-search.test.ts src/features/channels/lib/channel-form.test.ts`，再跑 `bun run typecheck`、`bun run i18n:sync`、`bun run build`、`git diff --check`；最后 MCP 登录 3003，编辑渠道 `11111`，搜索 `gpt-5.6`，验证批量按钮补 2 个、候选单击只补 1 个、取消后接口模型列表未变化、控制台无错误。

### 实施结果

1. 渠道编辑页模型搜索保留批量按钮、搜索统计、供应商过滤和全量分页扫描能力，但移除了渠道模型 `MultiSelect` 调用处的 `submitSearchOnEnterWhenHighlighted`。现在有高亮候选时 Enter 由 Combobox 选择单条候选；没有高亮候选时仍可通过原有 `onSearchSubmit` 路径批量追加搜索命中。
2. 模型区视觉结构已对齐 new-api 的分组方式：`Models` 字段与 `Quick actions` 位于同一个浅背景边框块，`Model Mapping` 和 `Groups` 分别使用独立边框块。
3. 保留 NexusTok 原生能力：`Vendor: OpenAI` 徽标、`3 matched · 2 new · 1 already selected` 搜索摘要、`Will add` 预览、批量追加按钮、权限禁用原因、预设分组、模型映射风险提示、账号池模式、隐藏已选搜索结果、点击模型 chip 复制等能力均未回退。
4. 本轮未新增可见文案，未修改 i18n locale 文件中的业务翻译，未修改 Go 后端、数据库、模型同步接口、渠道保存 API、Relay 调度和 new-api 源码。

### 验证记录

1. 已运行 `cd web/default && bun test src/components/multi-select.test.ts`，24 个测试通过。
2. 已运行 `cd web/default && bun test src/features/channels/lib/model-search.test.ts`，25 个测试通过，覆盖 `gpt-5.6` 三模型搜索、已选去重、分页提示和默认供应商。
3. 已运行 `cd web/default && bun test src/features/channels/lib/channel-form.test.ts`，19 个测试通过，确认渠道表单转换和校验未受影响。
4. 已运行 `cd web/default && bun run typecheck`，TypeScript 检查通过。
5. 已运行 `cd web/default && bun run i18n:sync`，六语 missing/extras 为 0；既有 untranslated 计数不属于本轮新增。
6. 已运行 `cd web/default && bun run build`，Rsbuild 生产构建通过。
7. 已运行 `git diff --check`，未发现空白错误。
8. MCP 刷新并访问 `http://192.168.0.202:3003/channels`，确认页面热更新生效；打开渠道 `11111` 编辑抽屉后，模型区已呈现 new-api 风格的边框分组。桌面截图保存为 `/tmp/nexustok-channel-edit-aligned.png`，移动端 390px 截图保存为 `/tmp/nexustok-channel-edit-aligned-mobile.png`。
9. MCP 调用接口确认搜索数据：`/api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=100` 返回 `gpt-5.6-terra`、`gpt-5.6-luna`、`gpt-5.6-sol` 共 3 条；渠道 `11111` 当前模型为 `gpt-5.4,gpt-5.5,gpt-5.6-sol`。
10. MCP 在编辑抽屉搜索 `gpt-5.6` 后，页面显示 `3 matched · 2 new · 1 already selected`、按钮 `Add 2 search match(es)`、预览 `gpt-5.6-terra, gpt-5.6-luna`。
11. MCP 通过键盘事件先 `ArrowDown` 高亮 `gpt-5.6-terra`，再按 Enter，草稿只新增 `gpt-5.6-terra`，选中数从 3 变为 4，没有批量加入 `gpt-5.6-luna`，确认高亮候选不再触发批量搜索追加。
12. MCP 取消草稿后重新打开编辑抽屉，点击 `Add 2 search match(es)`，草稿一次新增 `gpt-5.6-terra` 和 `gpt-5.6-luna`，选中数从 3 变为 5，并显示 `Added 2 model(s) from search`。
13. MCP 取消抽屉后调用 `/api/channel/1`，返回 `models: "gpt-5.4,gpt-5.5,gpt-5.6-sol"`，确认验证过程未保存或污染真实渠道配置。
14. MCP 网络请求记录只包含 GET：`/api/status`、`/api/user/self`、`/api/notice`、`/api/group/`、`/api/channel/`、`/api/channel/1`、`/api/channel/models`、`/api/models/search`、`/api/prefill_group/`、`/api/account-pool/groups/options`、`/api/user/2fa/status`、`/api/user/passkey`；未出现渠道保存请求。
15. MCP 控制台无 JavaScript `error` 或 `warn`。Chrome DevTools 报出两个既有表单 `issue`：`name` 输入缺少 `autocomplete`，以及部分 label 的 `for` 与控件 `id` 不匹配；本轮未修改这些表单基础组件，已记录为后续可访问性改进项。

## 本轮实施评审：模型倍率可视化编辑器保存前草稿提交保护

### 差异来源

继续对照 `/opt/project/new-api-main` 与当前项目的系统设置模型倍率页面后发现，new-api 在 `web/default/src/features/system-settings/models/model-ratio-form.tsx`、`model-ratio-visual-editor.tsx` 和 `model-pricing-sheet.tsx` 中已经加入了“打开中的模型定价编辑器在外层保存前先提交草稿”的保护链路：`ModelRatioForm` 持有 `visualEditorRef`，保存时调用 `commitOpenEditor()`；视觉编辑器再调用右侧 `ModelPricingEditorPanel` 或移动端 `ModelPricingSheet` 的 `commitDraft()`，只有草稿校验通过才继续外层保存。

NexusTok 当前项目已经具备比 new-api 更完整的批量模型定价能力，包括 CodeMirror JSON 模式、计费表达式编辑器、模型定价格式化稳定化、权限控制和可视化批量复制。但当前保存链路仍存在一个细节风险：管理员在可视化模式打开某个模型的右侧定价面板，修改输入价格、缓存价格或表达式草稿后，如果没有点击面板底部 `Update`/`Add`，直接点击外层 `Save model prices`，外层表单只提交已经写入 `react-hook-form` 的 JSON 字符串，打开面板里的未提交草稿不会进入保存 payload。这会让管理员误以为“保存成功但刚才改的模型定价没生效”。

前端设计方向保持当前系统设置页的克制数据工具风格：使用现有表格、边框面板和标准按钮，不引入新的视觉装饰；本轮可见差异主要是行为可靠性，而不是新文案或新布局。

### 需求分析

1. 将 new-api 的保存前草稿提交保护转换为 NexusTok 原生能力：外层模型倍率保存必须先尝试提交当前打开的可视化定价面板草稿。
2. 保留当前 NexusTok 已有的面板级 `Update`/`Add` 行为、批量复制、移动端 Sheet、权限禁用、计费表达式和 JSON 模式，不整段覆盖 new-api 文件。
3. 如果打开面板草稿校验失败，例如模型名为空、依赖价格缺失、音频输出价格缺少音频输入价格，外层保存必须中止，避免把半成品计费配置写入系统 option。
4. 如果没有打开面板，或当前处于 JSON 模式，外层保存行为应保持原样。
5. 本轮不真实保存运行态模型倍率配置；MCP 只验证页面热更新、打开面板、修改草稿和取消路径，保存前提交行为主要由类型检查和代码路径覆盖，避免污染计费环境。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 模型倍率表单保存链路 | `web/default/src/features/system-settings/models/model-ratio-form.tsx` | 增加 `visualEditorRef` 与统一 `handleSave`，可视化模式保存前先提交打开面板草稿；JSON 模式继续使用原表单提交。 |
| 可视化批量模型定价编辑器 | `web/default/src/features/system-settings/models/model-ratio-visual-editor.tsx` | 暴露 `ModelRatioVisualEditorHandle.commitOpenEditor()`，桥接右侧面板或移动端 Sheet 的草稿提交；保留现有表格、批量复制和 `persistPricingData`。 |
| 模型定价面板/Sheet | `web/default/src/features/system-settings/models/model-pricing-sheet.tsx` | 将编辑面板改为 `forwardRef`，暴露 `commitDraft()` 复用现有表单校验和数据构造；面板底部 `Update`/`Add` 仍调用原 `onSave(data)`。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审、实施结果和验证记录。 |

本轮不修改 Go 后端、数据库 schema、`options` 存储结构、`billing_setting` 后端、`pkg/billingexpr` 表达式语言、预消费、结算、日志展示、模型同步接口、分组倍率、工具价格和 new-api 源码。

### 风险评估

1. 计费语义风险：模型倍率和表达式直接影响扣费，任何自动提交草稿都不能改写字段含义。方案只复用现有 `handleSubmit` 校验与 `persistPricingData` 写表单字符串，不改变 `ModelPrice`、`ModelRatio`、`billing_setting.billing_mode`、`billing_setting.billing_expr` 的结构。
2. 校验绕过风险：如果 `commitDraft()` 只读取表单值而不触发校验，外层保存可能写入非法草稿。方案必须先 `form.trigger()` 并执行与原 `handleSubmit` 相同的跨字段校验。
3. 行为重复风险：面板内 `Update`/`Add` 和外层保存都可能提交草稿；方案将草稿构造收敛到同一个 helper，避免两条路径产生不一致 payload。
4. 移动端风险：移动端使用 `ModelPricingSheet` 而不是右侧内嵌面板，必须把同一个 ref 传进 Sheet 内部面板，避免桌面修复而移动端仍丢草稿。
5. 权限风险：外层 `Save model prices` 已受 `canSave` 控制；本轮不新增绕过权限的保存按钮，也不在无权限状态主动写入系统 option。
6. 验证风险：真实保存模型倍率会影响计费，MCP 需要以打开、编辑、取消和页面状态为主；保存前提交的核心不变量由 TypeScript、构建和代码审查验证。

### 方案评审

采用“小范围保存链路增强，不拆分大文件”的方案：

1. 在 `model-pricing-sheet.tsx` 中新增 `ModelPricingEditorPanelHandle`，让 `ModelPricingEditorPanel` 与 `ModelPricingSheet` 都通过 `forwardRef` 暴露 `commitDraft()`。
2. 抽出 `validatePricingValues()` 与 `buildSubmitData()`，让面板底部提交和外层保存前提交复用同一套校验和数据构造；原 `Update`/`Add` 仍然关闭面板并写入表单草稿。
3. 在 `model-ratio-visual-editor.tsx` 中引入 `editorPanelRef`，实现 `commitOpenEditor()`：未打开编辑器时直接返回 `true`；有打开编辑器时调用 `commitDraft()`，失败返回 `false`，成功则调用现有 `persistPricingData(data)` 写入表单 JSON 并保留当前编辑数据。
4. 在 `model-ratio-form.tsx` 中持有 `visualEditorRef`，新增 `handleSave`：可视化模式先 `commitOpenEditor()`，返回 `false` 时中止；成功后再执行 `form.handleSubmit(onSave)()`。JSON 模式没有打开中的可视化面板，继续沿用原表单提交。
5. 不新增可见文案、不修改 i18n locale；如后续为了快照状态新增 `Draft changed`、`New draft` 等徽标，再以单独切片引入翻译和测试。
6. 验证顺序：先跑 `bun run typecheck`、模型相关测试、`bun run i18n:sync`、`bun run build`、`git diff --check`；再用 MCP 访问 `http://192.168.0.202:3003/system-settings/models/model-ratio`，确认页面热更新、可视化编辑面板可打开、草稿修改后可取消且不保存真实配置。

### 实施结果

1. `ModelPricingEditorPanel` 和 `ModelPricingSheet` 已转换为 `forwardRef`，并暴露 `commitDraft()`。该方法会先执行 react-hook-form 字段校验，再执行输入价格依赖和音频输出依赖的跨字段校验，失败时返回 `null` 并阻止外层保存。
2. `ModelPricingEditorPanel` 的面板按钮提交和外层保存前提交现在复用同一个 `buildSubmitData()`，避免 `Update`/`Add` 与外层 `Save model prices` 产生不同的 `ModelRatioData`。
3. `ModelRatioVisualEditor` 已暴露 `commitOpenEditor()`：没有打开编辑面板时直接允许保存；有打开面板时先调用 `commitDraft()`，成功后通过现有 `persistPricingData()` 写入表单 JSON 草稿，真正写入系统 option 仍由父级保存流程统一处理。
4. `ModelRatioForm` 的可视化模式保存按钮已接入 `handleSave`，保存前先提交打开中的模型定价面板草稿；JSON 模式继续沿用原表单提交路径。
5. 本轮未新增可见文案，未修改 i18n locale，未修改后端、数据库、`pkg/billingexpr`、模型倍率 option key、分组倍率或工具价格。

### 验证记录

1. 已运行 `cd web/default && bun test src/features/system-settings`，17 个系统设置相关测试通过，覆盖数字字段、价格格式化、Grok 表单转换、角色策略工具和 Waffo Pancake 凭据解析。
2. 已运行 `cd web/default && bun run typecheck`，TypeScript 检查通过，确认 `forwardRef`、`ModelPricingEditorPanelHandle`、`ModelRatioVisualEditorHandle` 和父子 ref 传递链路类型正确。
3. 已运行 `cd web/default && bun run i18n:sync`，同步通过；本轮没有新增可见翻译 key。
4. 已运行 `cd web/default && bun run build`，Rsbuild 生产构建通过，说明隐藏的模型定价组件和可达的 pricing route 均能被当前打包链路解析。
5. 已运行 `git diff --check`，未发现空白错误。
6. MCP 使用账号 `c1cada` 登录 `http://192.168.0.202:3003/` 成功，访问 `/pricing-settings` 后页面正常显示 `Group & Tool Pricing`、`Group ratios` 和 `Tool prices`，截图保存为 `/tmp/nexustok-pricing-settings-after-draft-commit.png`。
7. MCP 访问 `/system-settings/models/model-ratio` 时当前生产路由会按 `section-registry` 回落到 `/system-settings/models/global`；当前项目没有公开模型价格 tab 的导航入口，本轮未改变导航可见性，避免把保存链路保护与页面暴露策略混在一个切片。
8. MCP 控制台没有 JavaScript `error` 或 `warn`，仅有 i18next 信息和 `nexustok-build` debug；网络请求只有 `/api/status`、`/api/user/self`、`/api/notice`、`/api/option/` 等读取请求，未出现保存 option 的写请求，确认验证未污染运行态计费配置。

## 本轮实施评审：模型定价快照与草稿状态原生化

### 差异来源

继续对照 `/opt/project/new-api-main` 的模型定价实现后发现，new-api 已将模型定价表格的数据解析、价格摘要、模式标签和快照签名拆分到 `model-pricing-snapshots.ts`，并在 `ModelRatioVisualEditor` 中同时构造“已保存快照”和“当前草稿快照”。这样表格行可以标记 `isDraftChanged`、`isDraftNew`、`isDraftDeleted`、`hasConflict`，编辑器打开时也能优先使用当前草稿而不是旧的保存值。

NexusTok 当前项目在上一轮已经具备“保存前提交打开面板草稿”的保护，但可视化编辑器内部仍只根据当前表单 JSON 解析一份行数据。管理员编辑或批量复制模型定价后，表格无法区分“已经保存的配置”和“尚未保存的草稿配置”；后续继续接入 new-api 的草稿徽标、冲突定位或保存前差异审阅时，也缺少稳定的纯函数基座。

本轮前端设计方向仍保持系统设置页的数据工具风格：不引入新布局、不新增装饰、不把隐藏的模型价格 tab 暴露到导航，只在已有表格行中复用现有 `Unsaved changes` 文案标记草稿态。

### 需求分析

1. 把 new-api 的快照解析能力转为 NexusTok 原生纯函数：从八个模型倍率/价格 JSON 字符串和两个计费表达式 JSON 字符串中生成稳定的 `ModelPricingSnapshot`。
2. 增加 saved/draft 对比能力：基于已保存基线和当前表单草稿生成行数据，识别草稿新增、草稿变更和草稿删除。
3. 可视化编辑器展示和编辑应优先使用 draft 行，避免管理员刚修改的未保存草稿在表格摘要或右侧编辑面板中仍显示旧保存值。
4. 保留当前 NexusTok 的计费表达式语义、tiered_expr 兜底 ratio/price 保留逻辑、批量复制、删除、搜索、列显隐、移动端 Sheet 和保存前提交保护。
5. 不修改后端接口、数据库、option key、`pkg/billingexpr`、预消费、结算或实际扣费公式。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 模型定价快照纯函数 | `web/default/src/features/system-settings/models/model-pricing-snapshots.ts` | 新增快照解析、摘要、签名和 saved/draft 行合并能力；逻辑从当前可视化编辑器抽出，便于单测覆盖。 |
| 快照单测 | `web/default/src/features/system-settings/models/model-pricing-snapshots.test.ts` | 覆盖 per-token、per-request、tiered_expr、冲突、草稿新增和草稿变更，不依赖浏览器环境。 |
| 可视化编辑器 | `web/default/src/features/system-settings/models/model-ratio-visual-editor.tsx` | 使用快照 helper 构造行数据；行编辑优先使用 `draft ?? saved ?? row`；草稿态复用 `Unsaved changes` 徽标。 |
| 模型倍率表单与父级基线 | `model-ratio-form.tsx`、`ratio-settings-card.tsx` | 从父级传入 `savedValues`，保存成功或后端默认值刷新后同步更新保存基线。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审、实施结果和验证记录。 |

### 风险评估

1. 计费语义风险：快照 helper 只能解析和展示配置，不能改写 `ModelPrice`、`ModelRatio`、`billing_setting.billing_mode`、`billing_setting.billing_expr` 的保存格式。方案保留现有 `persistPricingData()` 作为唯一写表单 JSON 的函数。
2. 表格显示风险：如果 changed 行展示 saved 值，管理员会看不到自己的草稿；如果删除行仍展示，可能误导管理员。本轮采用 draft 优先并过滤已删除草稿行。
3. tiered_expr 风险：表达式计费模型在多实例同步延迟期间可能保留 ratio/price 作为兜底值。快照解析必须继续保留这些字段，并将 `billingExpr` 拆成纯表达式和请求规则。
4. 翻译风险：新增可见文案会触发六语翻译维护。本轮复用已有 `Unsaved changes`，不新增 locale key。
5. 路由验证风险：当前生产导航没有公开模型价格 tab，MCP 难以直接操作该隐藏组件。本轮以纯函数单测、typecheck、构建和可达 `/pricing-settings` 页面运行态验证共同覆盖。
6. 父级基线风险：如果保存成功后不更新 saved baseline，表格会一直显示未保存；如果保存失败时提前更新，则会掩盖失败。本轮仅在 `updateOption.mutateAsync` 循环全部完成后更新 baseline。

### 方案评审

采用“先抽纯函数并接入草稿状态，不拆列文件、不暴露隐藏入口”的小步方案：

1. 新增 `model-pricing-snapshots.ts`，提供 `buildModelSnapshots()`、`getSnapshotSignature()`、`buildModelRows()`、`getModeLabel()`、`getModeVariant()`、`getPriceSummary()` 和 `getPriceDetail()`。
2. 从 `model-ratio-visual-editor.tsx` 移除内联解析和摘要 helper，改为调用快照 helper；保留当前 DataTable 结构，不迁入 new-api 的 `model-ratio-table-columns.tsx`，避免一次性重写列渲染。
3. `ModelRow` 增加 `saved`、`draft`、`isDraftChanged`、`isDraftNew`、`isDraftDeleted`；表格行展示使用 draft 优先，编辑打开也使用 draft 优先。
4. 在模型名称旁复用已有 `Unsaved changes` 徽标，提示草稿新增或草稿变更；`Conflict`、`Tiered` 和模式标签继续复用现有文案。
5. `RatioSettingsCard` 维护 `savedModelValues`，后端默认值刷新或保存成功后同步；`ModelRatioForm` 将 saved 值传给视觉编辑器。
6. 验证顺序：新增纯函数单测后运行 `bun test src/features/system-settings/models/model-pricing-snapshots.test.ts src/features/system-settings/models/pricing-format.test.ts`、`bun run typecheck`、`bun run i18n:sync`、`bun run build`、`git diff --check`；再用 MCP 访问 `http://192.168.0.202:3003/pricing-settings`，确认可达计费设置页没有运行时错误且没有写请求。

### 实施结果

1. 新增 `model-pricing-snapshots.ts`，把模型定价表格的 JSON 解析、模式标签、价格摘要、表达式请求规则拆分、快照签名和 saved/draft 行合并收敛为纯函数。`tiered_expr` 快照继续保留 ratio/price/cache/audio 等兜底字段，避免多实例同步延迟时误删兼容配置。
2. 新增 `model-pricing-snapshots.test.ts`，覆盖 per-token 摘要、per-request 与 ratio 冲突、tiered_expr 请求规则拆分、草稿新增/变更识别和快照签名稳定性。测试中的请求规则表达式使用当前前端 `combineBillingExpr()` 的真实保存格式：`(<base>) * (<condition> ? multiplier : 1)`。
3. `ModelRatioVisualEditor` 已删除内联快照/摘要解析逻辑，改为调用 `buildModelRows({ saved, draft })`。表格展示和编辑打开均优先使用 draft 行，模型名旁复用已有 `Unsaved changes` 徽标标记草稿新增或草稿变更。
4. `ModelRatioForm` 已接收 `savedValues` 并传入视觉编辑器；`RatioSettingsCard` 已维护 `savedModelValues`，在后端默认值刷新或模型价格保存全部成功后同步保存基线。若保存 API 返回失败或 mutation 抛错，baseline 不会提前更新。
5. 删除当前正在编辑的模型时，右侧编辑面板会同步关闭，避免表格行已消失但面板仍保留旧草稿。
6. 本轮未新增可见文案，未修改 i18n locale 文件；未修改 Go 后端、数据库 schema、option key、`pkg/billingexpr`、预消费、结算、日志展示、模型同步接口、分组倍率、工具价格和 new-api 源码。

### 验证记录

1. 已运行 `cd web/default && bun test src/features/system-settings/models/model-pricing-snapshots.test.ts src/features/system-settings/models/pricing-format.test.ts`，8 个测试通过。
2. 已运行 `cd web/default && bun test src/features/system-settings`，22 个系统设置相关测试通过，覆盖数字字段、价格格式化、Grok 表单转换、模型定价快照、角色策略工具和 Waffo Pancake 凭据解析。
3. 已运行 `cd web/default && bun run typecheck`，TypeScript 检查通过，确认 `savedValues`、`ModelRow`、快照 helper 和视觉编辑器引用链类型正确。
4. 已运行 `cd web/default && bun run i18n:sync`，同步通过；本轮未新增可见翻译 key。
5. 已运行 `cd web/default && bun run build`，Rsbuild 生产构建通过，确认可达 route 与隐藏模型定价组件均能被打包解析。
6. 已运行 `git diff --check`，未发现空白错误。
7. MCP 使用账号 `c1cada` 登录 `http://192.168.0.202:3003/` 成功，访问 `/pricing-settings` 后页面正常显示 `Group & Tool Pricing`、`Group ratios` 和 `Tool prices`，控制台没有 JavaScript `error` 或 `warn`，网络资源记录只包含 `/api/status`、`/api/user/self`、`/api/notice`、`/api/option/` 等读取请求，未出现保存 option 的写请求。
8. MCP 访问 `/system-settings/models/model-ratio` 时仍按当前 `section-registry` 回落到 `/system-settings/models/global`；本轮未暴露隐藏的模型价格 tab，避免把草稿状态能力与导航策略混在一个切片。
9. MCP 验证截图保存为 `/tmp/nexustok-pricing-snapshots-validation.png`。

## 本轮实施评审：模型定价编辑器核心逻辑模块化

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/features/system-settings/models` 后发现，new-api 已将模型定价编辑器中的核心定价类型、价格换算、预览构造和输入组件拆分为 `model-pricing-core.ts` 与 `model-pricing-inputs.tsx`。这让定价编辑器的业务转换逻辑可以脱离 React 面板单独测试，也让后续表格列、编辑面板和模型详情页复用同一套定价单位/价格推导规则。

NexusTok 当前项目已经在 new-api 基础上加入了更完整的编辑体验：折叠式保存预览、取消按钮、批量复制目标提示、保存前草稿提交保护、`tiered_expr` 兜底字段保留、草稿快照状态等。但当前 `model-pricing-sheet.tsx` 仍同时承载类型定义、价格换算、预览构造、表单状态和具体 UI 渲染。对于会影响计费配置的代码来说，这种集中实现会增加后续继续吸收 new-api 功能时的回归风险。

本轮不改变页面视觉，不暴露隐藏模型价格 tab，不新增配置项；只将 new-api 的“核心逻辑可测试化/模块化”优势转为 NexusTok 原生能力。

### 需求分析

1. 将模型定价编辑器中的纯业务逻辑抽出为 `model-pricing-core.ts`，包括 schema、类型、价格 lane 配置、数值草稿校验、倍率与美元价格互转、初始 lane 状态推导、保存预览行构造。
2. 将 `PriceInput` 和 `PriceLane` 抽出为 `model-pricing-inputs.tsx`，但保留 NexusTok 当前使用的 `Field`、`FieldContent`、`FieldDescription`、`Switch`、`InputGroup` 视觉结构，不用 new-api 的 `SettingsControlGroup` 覆盖当前设计。
3. 为核心价格换算和预览构造补充单元测试，覆盖 per-token、per-request、tiered_expr、音频输出依赖音频输入、禁用 lane 清空等关键不变量。
4. 保留现有 `ModelPricingSheet`、`ModelPricingEditorPanel`、`ModelRatioData` 导出路径与外部调用契约，避免影响 `ModelRatioVisualEditor` 和上一轮保存前草稿提交保护。
5. 不修改后端、数据库、计费表达式引擎、option key、保存 payload 格式或真实运行配置。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 模型定价核心逻辑 | `web/default/src/features/system-settings/models/model-pricing-core.ts` | 新增定价编辑器纯函数与共享类型，作为 `model-pricing-sheet.tsx` 的业务核心。 |
| 核心逻辑测试 | `web/default/src/features/system-settings/models/model-pricing-core.test.ts` | 新增不依赖浏览器的单测，覆盖价格换算、预览构造和空值判断。 |
| 价格输入组件 | `web/default/src/features/system-settings/models/model-pricing-inputs.tsx` | 新增 `PriceInput`、`PriceLane`，保留当前 NexusTok 的 Field/Switch/InputGroup 视觉和交互。 |
| 模型定价面板 | `web/default/src/features/system-settings/models/model-pricing-sheet.tsx` | 删除内联核心逻辑和输入组件，改为从 core/inputs 导入；面板 UI、保存行为、Sheet props 保持不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审、实施结果和验证记录。 |

### 风险评估

1. 计费换算风险：per-token 模式依赖 “输入价格 / 2 = ModelRatio” 和 “lane 价格 / 输入价格 = 对应 ratio”，音频输出还依赖音频输入价格。本轮抽取必须保持现有公式完全不变，并用单测覆盖。
2. 表单契约风险：`ModelRatioData` 已被 `ModelRatioVisualEditor` 和快照 helper 使用；导出路径变化可能破坏调用方。方案继续从 `model-pricing-sheet.tsx` re-export `ModelRatioData`，外部无需改 import。
3. UI 回归风险：直接搬 new-api 的 `PriceLane` 会改变当前 Field/Switch 布局和禁用背景。本轮只抽当前实现，不替换视觉结构。
4. 表达式风险：`tiered_expr` 的保存预览必须继续通过当前 `combineBillingExpr()` 组合 request rules；不修改表达式语法、存储格式或后端校验。
5. 翻译风险：抽组件可能移动已有文案，但不新增新 key；仍需运行 `bun run i18n:sync` 确认 locale 状态。
6. 验证风险：模型价格页仍不是公开导航入口，MCP 只能验证可达 `/pricing-settings` 与 `/system-settings/models/global` 无运行时错误；核心不变量主要由单测、typecheck 和构建覆盖。

### 方案评审

采用“抽纯函数 + 保留 UI + 增加单测”的低风险方案：

1. 新建 `model-pricing-core.ts`，从当前 `model-pricing-sheet.tsx` 移出 `createModelPricingSchema`、`ModelPricingFormValues`、`PricingMode`、`LaneKey`、`ModelRatioData`、`PreviewRow`、`numericDraftRegex`、lane 常量、`hasValue()`、`toNumberOrNull()`、`createInitialLaneState()`、`buildPreviewRows()`。
2. 新建 `model-pricing-inputs.tsx`，从当前面板移出 `PriceInput` 和 `PriceLane`，保留 `Field`/`Switch`/`InputGroup` 结构和现有文案。
3. `model-pricing-sheet.tsx` 改为导入 core 与 inputs，保留 `ModelPricingSheet`、`ModelPricingEditorPanel`、`ModelPricingEditorPanelHandle`、保存前 `commitDraft()`、取消、批量目标提示和折叠预览行为。
4. 新增 `model-pricing-core.test.ts`，用纯函数断言初始 lane 状态、预览行和空值/数字解析，避免只有组件构建能覆盖计费换算。
5. 验证顺序：`bun test src/features/system-settings/models/model-pricing-core.test.ts src/features/system-settings/models/model-pricing-snapshots.test.ts src/features/system-settings/models/pricing-format.test.ts`、`bun test src/features/system-settings`、`bun run typecheck`、`bun run i18n:sync`、`bun run build`、`git diff --check`；最后 MCP 访问 `http://192.168.0.202:3003/pricing-settings` 与 `/system-settings/models/model-ratio`，确认页面运行态无错误且没有保存写请求。

### 实施结果

1. 新增 `model-pricing-core.ts`，集中导出模型定价编辑器的 schema、表单类型、计费模式、lane key、保存数据类型、预览行类型、数值草稿正则、lane 常量、lane 配置、空值判断、数字解析、初始 lane 状态推导和保存预览行构造。
2. 新增 `model-pricing-inputs.tsx`，将 `PriceInput` 与 `PriceLane` 从 `model-pricing-sheet.tsx` 中拆出，保留当前 NexusTok 的 `Field`、`FieldContent`、`FieldDescription`、`Switch`、`InputGroup` 结构，没有引入 new-api 的整套视觉实现。
3. `model-pricing-sheet.tsx` 已改为导入 core 与 inputs；`ModelPricingSheet`、`ModelPricingEditorPanel`、`ModelPricingEditorPanelHandle`、保存前 `commitDraft()`、取消按钮、批量目标提示、折叠式保存预览和 `ModelRatioData` re-export 保持原外部契约。
4. 新增 `model-pricing-core.test.ts`，覆盖从保存倍率推导美元价格、空模型 lane 状态副本、per-token 预览、per-request 预览、`tiered_expr` 请求规则组合预览、空值/零值语义和数字草稿正则。
5. 本轮没有修改 Go 后端、数据库 schema、option key、真实保存 payload、`pkg/billingexpr` 表达式引擎、预消费、结算、日志展示、模型同步接口、分组倍率、工具价格和 `/opt/project/new-api-main` 源码。
6. 本轮未新增可见文案，只移动已有组件文案；`bun run i18n:sync` 后没有产生 locale 文件变更。

### 验证记录

1. 已运行 `cd web/default && bun test src/features/system-settings/models/model-pricing-core.test.ts src/features/system-settings/models/model-pricing-snapshots.test.ts src/features/system-settings/models/pricing-format.test.ts`，13 个测试通过，覆盖本轮新增核心逻辑、上一轮草稿快照 helper 和价格格式化。
2. 已运行 `cd web/default && bun test src/features/system-settings`，27 个系统设置相关测试通过，覆盖数字字段、价格格式化、模型定价核心、Grok 表单转换、模型定价快照、角色策略工具和 Waffo Pancake 凭据解析。
3. 已运行 `cd web/default && bun run typecheck`，TypeScript 检查通过，确认 `model-pricing-sheet.tsx` 拆分后的类型导入、`ModelRatioData` re-export 和面板引用链正确。
4. 已运行 `cd web/default && bun run i18n:sync`，同步通过；本轮没有新增需要补翻译的 key。
5. 已运行 `cd web/default && bun run build`，Rsbuild 生产构建通过，确认系统设置路由、定价组件和隐藏模型价格面板均能被打包解析。
6. 已运行 `git diff --check`，未发现空白错误。
7. MCP 访问 `http://192.168.0.202:3003/pricing-settings`，页面正常显示 `Group & Tool Pricing`、`Group ratios` 和 `Tool prices`，控制台没有 JavaScript `error` 或 `warn`；网络请求只有 `/api/status`、`/api/user/self`、`/api/notice`、`/api/option/` 等读取请求以及既有登录请求，未出现保存 option 的写请求。
8. MCP 访问 `http://192.168.0.202:3003/system-settings/models/model-ratio` 后仍按当前 `section-registry` 回落到 `/system-settings/models/global`，页面显示 `Global Model Configuration`，控制台没有 JavaScript `error` 或 `warn`，网络请求只有读取请求。
9. MCP 验证截图保存为 `/tmp/nexustok-pricing-core-validation.png`。

## 本轮实施评审：模型定价表格列定义模块化

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/features/system-settings/models` 后发现，new-api 已将模型定价表格列定义拆分为 `model-ratio-table-columns.tsx`，并在 `model-ratio-visual-editor.tsx` 中只负责数据、状态、编辑器和保存链路。列文件统一封装选择列、模型名列、计费模式列、价格摘要列和操作列，复用 `model-pricing-snapshots.ts` 中的模式标签和价格摘要 helper。

NexusTok 当前项目已经原生化了快照 helper、草稿状态和定价编辑核心模块，但 `model-ratio-visual-editor.tsx` 仍内联完整 `ColumnDef<ModelRow>[]`。这个文件同时处理 JSON 解析、表格状态、列渲染、编辑面板、保存前草稿提交、删除、批量复制等职责。继续吸收 new-api 的表格能力时，内联列定义会让每次 UI 调整都触碰保存链路，增加计费配置页面回归风险。

本轮前端设计方向保持当前系统设置页的数据工具风格：沿用现有 shadcn/base 组件、语义色、表格密度和操作按钮，不新增装饰，不改变页面导航，不引入 new-api 更大范围的 `DataTableView/useDataTable` 通用表格架构。

### 需求分析

1. 新增 NexusTok 原生 `model-ratio-table-columns.tsx`，将模型定价表格列定义从 `model-ratio-visual-editor.tsx` 抽出。
2. 列定义应保留当前项目已有能力：行选择、模型名展示、`Tiered`/`Conflict`/`Unsaved changes` 状态徽标、计费模式筛选、价格摘要、行内编辑与删除按钮。
3. `model-ratio-visual-editor.tsx` 只负责构造表格数据、状态、编辑器、删除、批量复制和保存前草稿提交；列渲染细节通过 `buildModelRatioColumns()` 注入。
4. 不改变表格渲染容器、分页、过滤、行点击、选中态、高亮态、移动端 Sheet、保存 payload、option key 或计费表达式语义。
5. 本轮不迁入 new-api 的 `DataTableView`、`DataTableRow`、`StaticRowActions`、`useDataTable` 等通用表格层；这些属于更大范围的 data-table 架构差异，后续需要单独评审。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 模型定价表格列定义 | `web/default/src/features/system-settings/models/model-ratio-table-columns.tsx` | 新增列构造函数，封装选择列、模型名列、模式列、价格摘要列和操作列。 |
| 模型定价可视化编辑器 | `web/default/src/features/system-settings/models/model-ratio-visual-editor.tsx` | 删除内联列定义，改为调用 `buildModelRatioColumns()`；保存、删除、批量复制、表格状态和渲染容器保持不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审、实施结果和验证记录。 |

### 风险评估

1. 操作列行为风险：抽列后 `handleEdit()` 和 `handleDelete()` 必须仍拿到同一个 `ModelRow`/模型名，避免编辑打开旧数据或删除错误模型。
2. 草稿徽标风险：当前项目比 new-api 的列文件多了 `Unsaved changes` 徽标；抽取时必须保留 `isDraftChanged || isDraftNew` 展示，不退化草稿可见性。
3. 表格交互风险：当前渲染层通过 `button, [role="checkbox"]` 阻止行点击误触发编辑；抽列时操作按钮和 checkbox 仍必须使用原组件语义。
4. 布局回归风险：模型名和价格摘要列已有截断宽度与 `min-w` 控制，抽取时需要保持，避免长模型名或表达式摘要撑破表格。
5. 翻译风险：本轮只移动现有文案，不新增 key；仍运行 `bun run i18n:sync` 确认 locale 状态。
6. 范围风险：如果同时迁入通用 `DataTableView/useDataTable`，会触碰 shared data-table API 和多个页面。本轮只抽模型定价列，后续再单独推进通用表格架构差异。

### 方案评审

采用“列定义模块化，渲染层不变”的低风险方案：

1. 新建 `model-ratio-table-columns.tsx`，导出 `buildModelRatioColumns({ onDelete, onEdit, t })` 和内部 `filterBySelectedValues()`。
2. 列文件从当前 `model-ratio-visual-editor.tsx` 原样迁移列行为，并继续使用当前项目已有 `Button`、`Checkbox`、`DataTableColumnHeader`、`StatusBadge`、`Pencil`、`Trash2`，不引入尚未存在的 `StaticRowActions`。
3. `model-ratio-visual-editor.tsx` 删除 `ColumnDef`、`Checkbox`、`Button` 操作图标、`DataTableColumnHeader`、`StatusBadge`、价格摘要 helper 等列渲染相关导入，只保留状态与行为依赖。
4. 使用 `useMemo(() => buildModelRatioColumns(...), [handleEdit, handleDelete, t])` 生成列，保持现有 `useReactTable` 和手写 `Table` 渲染不变。
5. 验证顺序：`bun test src/features/system-settings/models/model-pricing-core.test.ts src/features/system-settings/models/model-pricing-snapshots.test.ts src/features/system-settings/models/pricing-format.test.ts`、`bun test src/features/system-settings`、`bun run typecheck`、`bun run i18n:sync`、`bun run build`、`git diff --check`；最后 MCP 访问 `http://192.168.0.202:3003/pricing-settings` 与 `/system-settings/models/model-ratio`，确认页面运行态无错误且没有保存写请求。

### 实施结果

1. 新增 `model-ratio-table-columns.tsx`，封装模型定价表格的选择列、模型名列、计费模式列、价格摘要列和操作列。
2. 列文件继续复用 `model-pricing-snapshots.ts` 中的 `getModeLabel()`、`getModeVariant()`、`getPriceSummary()`、`getPriceDetail()`，让表格展示与快照解析保持同一来源。
3. `model-ratio-visual-editor.tsx` 已删除内联 `ColumnDef<ModelRow>[]` 和列渲染相关导入，改为通过 `buildModelRatioColumns({ onDelete, onEdit, t })` 生成列；`useReactTable`、手写 `Table` 渲染、行点击编辑、分页、筛选、选中态和高亮态保持原实现。
4. 当前项目已有的 `Tiered`、`Conflict`、`Unsaved changes` 状态徽标均保留；操作列仍调用原 `handleEdit(row.original)` 与 `handleDelete(row.original.name)`，没有改动删除、批量复制、保存前草稿提交或真实 option 写入流程。
5. 本轮顺手补齐操作列 icon-only 按钮的 `aria-label` 和 `data-icon`，让编辑/删除按钮更符合当前 shadcn 规则；没有新增可见文案或 locale key。
6. 本轮未迁入 new-api 的 `DataTableView`、`DataTableRow`、`StaticRowActions`、`useDataTable` 等通用表格层，避免扩大 shared data-table 影响范围。
7. 本轮没有修改 Go 后端、数据库 schema、option key、保存 payload、`pkg/billingexpr`、预消费、结算、日志展示、模型同步接口、分组倍率、工具价格和 `/opt/project/new-api-main` 源码。

### 验证记录

1. 已运行 `cd web/default && bunx shadcn@latest info --json`，确认当前前端为 Manual + Tailwind v4 + Base UI + hugeicons，已安装 table、checkbox、button 等本轮使用组件；命令没有产生工作区变更。
2. 已运行 `cd web/default && bun test src/features/system-settings/models/model-pricing-core.test.ts src/features/system-settings/models/model-pricing-snapshots.test.ts src/features/system-settings/models/pricing-format.test.ts`，13 个测试通过。
3. 已运行 `cd web/default && bun test src/features/system-settings`，27 个系统设置相关测试通过。
4. 已运行 `cd web/default && bun run typecheck`，TypeScript 检查通过，确认列文件类型、`ModelRow` 引用和 `useReactTable` 列注入链路正确。
5. 已运行 `cd web/default && bun run i18n:sync`，同步通过；本轮没有新增需要补翻译的 key。
6. 已运行 `cd web/default && bun run build`，Rsbuild 生产构建通过。
7. 已运行 `git diff --check`，未发现空白错误。
8. MCP 使用账号 `c1cada` 登录 `http://192.168.0.202:3003/` 成功；访问 `/pricing-settings` 后页面正常显示 `Group & Tool Pricing`、`Group ratios` 和 `Tool prices`，控制台没有 JavaScript `error` 或 `warn`，网络资源记录只包含 `/api/status`、`/api/user/self`、`/api/notice`、`/api/option/` 等读取请求和登录请求，未出现保存 option 的写请求。
9. MCP 访问 `/system-settings/models/model-ratio` 时仍按当前 `section-registry` 回落到 `/system-settings/models/global`；页面显示 `Global Model Configuration`，控制台没有 JavaScript `error` 或 `warn`，网络请求只有读取请求。
10. MCP 验证截图保存为 `/tmp/nexustok-model-ratio-columns-validation.png`。

## 本轮实施评审：DataTable 状态 Hook 原生化

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/components/data-table` 后发现，new-api 已新增 `hooks/use-data-table.ts`，将 TanStack Table 的常用状态和 row model 组合收敛为公共 `useDataTable()`。new-api 中的 usage logs、users、subscriptions、models、channels、pricing、system-settings/model-ratio 等多个页面都通过该 hook 创建表格实例，页面侧只保留业务数据、列定义和少量自定义过滤逻辑。

NexusTok 当前项目的公共 data-table 目录已有 `DataTablePage`、`DataTableToolbar`、`DataTablePagination`、`DataTableBulkActions`、移动端卡片和列显隐控件，但页面仍在多处重复调用 `useReactTable()` 并手动拼装 `getCoreRowModel()`、`getFilteredRowModel()`、`getPaginationRowModel()`、`getFacetedRowModel()`、`getFacetedUniqueValues()`、排序、分页、列显隐、行选择等配置。模型定价表格已经完成列定义模块化，是接入公共 hook 的低风险落点。

本轮不改视觉、不改数据表格目录结构，不迁入 new-api 的 `core/`、`layout/`、`toolbar/` 分层；只把 `useDataTable()` 作为 NexusTok 当前扁平 data-table 目录的原生公共能力，并先在模型定价表格中使用。

### 需求分析

1. 新增 `web/default/src/components/data-table/use-data-table.ts`，提供可控/非可控混合的 TanStack Table 状态管理能力。
2. hook 需要支持当前和后续页面常用能力：排序、列显隐、行选择、展开行、列过滤、全局过滤、分页、可选手动过滤/分页/排序、可选 row model、`getRowId`、`getSubRows`、`globalFilterFn`、`autoResetPageIndex`。
3. 支持 `columnVisibilityStorageKey`，把列显隐 localStorage 读写从业务页面中移出；存储不可用或 JSON 损坏时必须安全降级为空对象。
4. 从 `@/components/data-table` 稳定入口导出 `useDataTable()`，符合当前 README 中“功能页面优先使用统一入口”的约定。
5. 将 `model-ratio-visual-editor.tsx` 从直接 `useReactTable()` 切换为 `useDataTable()`，保留原有排序、模式筛选、全局搜索关闭编辑面板、分页、行选择、列显隐持久化、行点击编辑、批量复制和保存前草稿提交行为。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 公共 DataTable hook | `web/default/src/components/data-table/use-data-table.ts` | 新增 TanStack Table 状态组合 hook，为后续页面逐步去重提供稳定公共能力。 |
| 公共导出入口 | `web/default/src/components/data-table/index.ts` | 导出 `useDataTable`，保持业务页面只依赖 `@/components/data-table`。 |
| 模型定价可视化编辑器 | `web/default/src/features/system-settings/models/model-ratio-visual-editor.tsx` | 使用 `useDataTable()` 替代内联 `useReactTable()` 和手写列显隐 localStorage 读写；表格渲染和业务行为保持不变。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审、实施结果和验证记录。 |

### 风险评估

1. 公共 API 风险：`useDataTable()` 进入公共入口后会被后续页面复用，类型设计必须覆盖当前常见 `useReactTable()` 参数，不应为了模型定价表格写成专用 hook。
2. 行为回归风险：模型定价表格的全局搜索会关闭打开中的编辑面板；切换 hook 后必须继续通过 `handleGlobalFilterChange` 保持该行为。
3. 列显隐风险：当前模型定价表格使用 `model-ratio-column-visibility` localStorage key。迁入 hook 后必须使用相同 key 和相同默认隐藏列，避免用户列显示偏好丢失或反复重置。
4. 行选择风险：批量复制依赖 `table.getFilteredSelectedRowModel()` 和 `table.resetRowSelection()`；hook 内部管理 rowSelection 时必须仍触发表格刷新并保留 TanStack 原 API。
5. 分页风险：hook 默认 pageSize 需要保持 20，且继续开启 `getPaginationRowModel()`，避免模型定价表格一页显示数量变化。
6. 范围风险：一次性替换所有页面的 `useReactTable()` 会跨越 channels、users、logs、keys 等核心页面。本轮只接入模型定价表格，公共 hook 经验证稳定后再逐步迁移其它页面。

### 方案评审

采用“新增公共 hook + 单页面接入”的渐进方案：

1. 新建 `use-data-table.ts`，从 new-api 的 hook 设计迁移核心能力，并改为 NexusTok 版权头和中文注释；保留 `useControllableTableState()`，让调用方既可以传受控 state，也可以交给 hook 内部 state 管理。
2. `readColumnVisibility()` 使用防御式解析，只接受对象中的 boolean 值；localStorage 不存在、不可用或内容异常时返回 `{}`。
3. hook 默认开启本地 row model：filtered、pagination、sorted、faceted；在手动过滤/分页/排序模式下自动关闭对应 row model，同时允许调用方显式覆盖。
4. `index.ts` 导出 `useDataTable`，后续页面可以从 `@/components/data-table` 统一导入。
5. `model-ratio-visual-editor.tsx` 移除 `useReactTable()` 和各 row model 导入，移除本地 `columnVisibility`/`pagination`/`rowSelection` state 与手写 localStorage effect，改为调用 `useDataTable({ initialColumnVisibility, columnVisibilityStorageKey: STORAGE_KEY, ... })`。
6. 验证顺序：`bun test src/features/system-settings/models/model-pricing-core.test.ts src/features/system-settings/models/model-pricing-snapshots.test.ts src/features/system-settings/models/pricing-format.test.ts`、`bun test src/features/system-settings`、`bun run typecheck`、`bun run i18n:sync`、`bun run build`、`git diff --check`；最后 MCP 访问 `http://192.168.0.202:3003/pricing-settings` 与 `/system-settings/models/model-ratio`，确认页面运行态无错误且没有保存写请求。

### 实施结果

1. 新增 `web/default/src/components/data-table/use-data-table.ts`，提供公共 `useDataTable()`，统一 TanStack Table 的排序、列显隐、行选择、展开行、分页、列过滤、全局过滤、可选手动模式和常用 row model 组合。
2. `useDataTable()` 支持受控和非受控状态混用：调用方传入 `sorting`、`columnVisibility`、`rowSelection`、`expanded`、`pagination` 时按外部状态运行；未传入时由 hook 内部管理，并仍会调用对应 `onChange`。
3. 新增 `columnVisibilityStorageKey` 支持，hook 会从 localStorage 防御式读取 boolean 字段，并在列显隐变化时写回；localStorage 不可用或内容异常时安全降级，不影响表格交互。
4. `web/default/src/components/data-table/index.ts` 已导出 `useDataTable`，后续页面可以继续从 `@/components/data-table` 稳定入口引入公共表格能力。
5. `model-ratio-visual-editor.tsx` 已移除直接 `useReactTable()`、`getCoreRowModel()`、`getFilteredRowModel()`、`getPaginationRowModel()`、`getFacetedRowModel()`、`getFacetedUniqueValues()` 等样板配置，改为调用 `useDataTable()`。
6. 模型定价表格继续保留原 `model-ratio-column-visibility` 存储 key、默认隐藏的 cache/image/audio 相关列、搜索时关闭编辑面板、模式筛选、分页、行选择、批量复制、行点击编辑和保存前草稿提交行为。
7. 本轮没有修改 Go 后端、数据库 schema、option key、保存 payload、`pkg/billingexpr`、预消费、结算、日志展示、模型同步接口、分组倍率、工具价格和 `/opt/project/new-api-main` 源码。

### 验证记录

1. 已运行 `cd web/default && bun run typecheck`，TypeScript 检查通过，确认公共 hook 泛型、`useDataTable()` 导出和模型定价表格接入链路类型正确。
2. 已运行 `cd web/default && bun test src/features/system-settings/models/model-pricing-core.test.ts src/features/system-settings/models/model-pricing-snapshots.test.ts src/features/system-settings/models/pricing-format.test.ts`，13 个测试通过。
3. 已运行 `cd web/default && bun test src/features/system-settings`，27 个系统设置相关测试通过。
4. 已运行 `cd web/default && bun run i18n:sync`，同步通过；本轮未新增可见文案或翻译 key。
5. 已运行 `cd web/default && bun run build`，Rsbuild 生产构建通过。
6. 已运行 `git diff --check`，未发现空白错误。
7. MCP 使用账号 `c1cada` 登录 `http://192.168.0.202:3003/` 成功；访问 `/pricing-settings` 后页面正常显示 `Group & Tool Pricing`、`Group ratios` 和 `Tool prices`，控制台没有 JavaScript `error` 或 `warn`，网络资源记录只包含 `/api/status`、`/api/user/self`、`/api/notice`、`/api/option/` 等读取请求和登录请求，未出现保存 option 的写请求。
8. MCP 访问 `/system-settings/models/model-ratio` 时仍按当前 `section-registry` 回落到 `/system-settings/models/global`；页面显示 `Global Model Configuration`，控制台没有 JavaScript `error` 或 `warn`，网络请求只有读取请求。
9. MCP 验证截图保存为 `/tmp/nexustok-use-data-table-validation.png`。

## 本轮实施评审：渠道表格模型筛选防组合输入 Hook 原生化

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/components/data-table/hooks/use-debounced-column-filter.ts` 与 `/opt/project/new-api-main/web/default/src/features/channels/components/channels-table.tsx` 后确认，new-api 最新渠道表格已经把“按模型过滤”的输入框收敛为公共 `useDebouncedColumnFilter()`。该 hook 不只做普通 debounce，还显式处理 `compositionstart` / `compositionend`，避免中文、日文、韩文等输入法组合输入过程中把半成品拼音或候选词提前写入 URL 列过滤状态。

NexusTok 当前渠道编辑抽屉已经具备 new-api 最新编辑渠道页的主干结构：右侧大抽屉、左侧分段导航、`Basic Information` / `Credentials` / `Models & Groups` / `Advanced Settings` 四段、模型区搜索与分页扫描追加、账号池、Codex OAuth、权限控制和模型映射防护均已原生化。但渠道列表页的模型过滤输入仍保留手写 `useState + useDebounce + useEffect` 逻辑，缺少组合输入保护；当管理员在渠道页用模型名筛选再进入编辑页时，筛选状态可能过早变化，容易被误认为“搜索添加不正确”或搜索结果刷新不稳定。

本轮不重新覆盖编辑渠道页，也不修改保存、模型同步、模型库搜索或后端接口；只将 new-api 的防组合输入列过滤能力转为 NexusTok 公共 DataTable 原生能力，并先接入渠道表格的 `model` 列过滤。

### 需求分析

1. 新增 `web/default/src/components/data-table/use-debounced-column-filter.ts`，提供可复用的列过滤输入状态 hook。
2. hook 必须保持输入框即时反馈、URL 列过滤 debounce 写入、外部 URL/back-forward 状态同步和重置能力。
3. hook 必须识别输入法组合状态：组合输入期间只更新本地输入值，不更新待提交值；组合结束后再提交完整文本，避免半成品关键词触发渠道列表查询。
4. 渠道表格的 `Filter by model...` 输入改为复用该 hook，重置表格筛选时同步清空模型输入框。
5. 保留渠道表格现有查询契约：`model` 仍写入 `useTableUrlState` 的 `columnFilters`，`shouldSearch`、`searchChannels()`、分页、排序、状态/type/group 过滤逻辑均不变。
6. 不新增可见文案，不修改 i18n locale，不修改 Go 后端、数据库、模型同步接口、渠道编辑抽屉和 `/opt/project/new-api-main` 源码。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 公共 DataTable hook | `web/default/src/components/data-table/use-debounced-column-filter.ts` | 新增防组合输入列过滤 hook，供渠道、日志、模型等后续表格复用。 |
| 公共导出入口 | `web/default/src/components/data-table/index.ts` | 导出 `useDebouncedColumnFilter`，保持业务页面统一从 `@/components/data-table` 引入。 |
| 渠道列表表格 | `web/default/src/features/channels/components/channels-table.tsx` | 移除本地手写模型筛选 debounce/effect，接入公共 hook，并在 Reset 时同步清空模型筛选输入。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审、实施结果和验证记录。 |

### 风险评估

1. URL 状态回归风险：渠道表格模型过滤依赖 `useTableUrlState` 将 `model` 参数写入地址栏；hook 必须继续通过 `onColumnFiltersChange` 更新同一个 `columnId`，不能改 URL key 或过滤值类型。
2. 查询触发风险：`searchChannels()` 只应在全局搜索或模型过滤非空时触发；hook 返回的 `value` 必须代表已经提交到 column filters 的值，而不是输入法组合中的本地临时值。
3. 重置风险：DataTableToolbar 的 Reset 会清理表格过滤；模型输入本地状态也必须清空，否则 UI 显示和实际 URL 过滤会短暂不一致。
4. 组合输入风险：如果组合开始后外部 URL 变化，不能覆盖用户正在输入的本地文本；组合结束后应以输入框最终值提交。
5. 公共 API 风险：hook 进入 data-table 公共入口后会被后续页面复用，类型应使用 TanStack 的 `ColumnFiltersState` 和 `OnChangeFn<ColumnFiltersState>`，避免写成渠道专用实现。
6. 范围风险：new-api 渠道表格还有批量模式、敏感信息显隐、列显隐持久化等差异；这些会触碰更多权限和表格状态，不应混入本轮小修复。

### 方案评审

采用“公共 hook + 单点接入”的低风险方案：

1. 按 new-api 的 `useDebouncedColumnFilter()` 思路新增 NexusTok 版本，改为本项目版权头和中文注释，继续复用现有 `@/hooks/use-debounce`。
2. hook 内部维护 `inputValue`、`pendingValue`、`isComposingRef` 和 `debouncedValue`；`onColumnFiltersChange` 使用 ref 保存最新回调，避免 effect 依赖导致重复提交。
3. 当外部 `columnFilters` 的目标列值变化时，同步 `pendingValue`，并且只在未组合输入时同步 `inputValue`。
4. 当 `debouncedValue !== value` 时，用函数式 updater 删除旧目标列过滤并按非空值写入新过滤；空值则移除该列过滤。
5. 渠道表格删除 `useDebounce`、`useEffect` 和本地 `modelFilterInput` 逻辑，改为从 `useDebouncedColumnFilter()` 获取 `value`、`inputValue`、`onChange`、组合输入事件和 `resetInput`。
6. `DataTableToolbar` 的 `onReset` 调用 `resetModelFilterInput()`，让 UI 输入框和 URL 过滤状态共同归零。
7. 验证顺序：`bun run typecheck`、渠道相关单测、系统设置回归单测、`bun run i18n:sync`、`bun run build`、`git diff --check`；最后用 MCP 登录并访问 `http://192.168.0.202:3003/channels`，验证模型筛选输入和编辑抽屉页面仍正常渲染。

### 实施结果

1. 新增 `web/default/src/components/data-table/use-debounced-column-filter.ts`，提供 NexusTok 原生 `useDebouncedColumnFilter()`，统一处理列过滤输入的本地显示值、待提交值、debounce 提交、外部 URL 状态同步、组合输入事件和 reset。
2. `useDebouncedColumnFilter()` 使用 `ColumnFiltersState` / `OnChangeFn<ColumnFiltersState>` 作为公共类型，后续其它 DataTable 页面可以复用；实现中用 ref 保存最新 `onColumnFiltersChange`，避免 effect 反复绑定导致旧回调提交。
3. `web/default/src/components/data-table/index.ts` 已导出 `useDebouncedColumnFilter`，继续维持业务页面统一从 `@/components/data-table` 导入公共表格能力。
4. `web/default/src/features/channels/components/channels-table.tsx` 已移除模型筛选的手写 `useDebounce`、本地输入 state 和两段同步 effect，改为复用公共 hook。
5. 渠道页 `Filter by model...` 输入新增 `onCompositionStart` / `onCompositionEnd` 绑定；输入法组合期间不会提前提交 `model` URL 过滤，组合结束后才 debounce 更新。
6. 渠道表格 reset 已调用 `resetModelFilterInput()`，避免清空过滤后模型输入框残留旧搜索词。
7. 本轮没有修改渠道编辑抽屉主结构、模型搜索追加算法、模型同步接口、后端 API、数据库 schema、Go 代码、i18n locale 文件和 `/opt/project/new-api-main` 源码。

### 验证记录

1. 已运行 `cd web/default && bunx shadcn@latest info --json`，确认当前前端仍为 Manual + Tailwind v4 + Base UI + hugeicons，已安装 input/table/button 等本轮复用组件；命令没有产生锁文件或 package 变更。
2. 已运行 `cd web/default && bun run typecheck`，TypeScript 检查通过，确认公共 hook 类型、data-table 导出和渠道表格接入链路正确。
3. 已运行 `cd web/default && bun test src/features/channels`，56 个渠道相关测试通过，覆盖模型搜索追加、`gpt-5.6` 三模型缺失项计算、渠道表单转换、抽屉更新权限裁剪、模型映射编辑器和 Advanced Custom 工具函数。
4. 已运行 `cd web/default && bun test src/features/system-settings`，27 个系统设置相关测试通过，确认本轮公共 data-table 增强没有回归近期模型定价和系统设置能力。
5. 已运行 `cd web/default && bun run i18n:sync`，同步通过；本轮未新增可见翻译 key。
6. 已运行 `cd web/default && bun run build`，Rsbuild 生产构建通过。
7. 已运行 `git diff --check`，未发现空白错误。
8. MCP 使用账号 `c1cada` 登录 `http://192.168.0.202:3003/` 成功，打开 `/channels` 后页面正常显示渠道列表和 `Filter by model...` 输入框。
9. MCP 在 `Filter by model...` 输入框模拟组合输入：`compositionstart` 后输入 `gp` 并等待 700ms，URL 仍为 `/channels`；`compositionend` 后输入完整 `gpt-5.6` 并等待 debounce，URL 正确变为 `/channels?model=gpt-5.6`，证明新 hook 已在热更新页面生效。
10. MCP 打开渠道 `11111` 的 `Edit Channel` 抽屉，确认仍显示 `Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings`，模型区显示 `Selected 3`、`Vendor: OpenAI`，并包含 `gpt-5.4`、`gpt-5.5`、`gpt-5.6-sol`。
11. MCP 在模型选择器中搜索 `gpt-5.6`，页面出现 `gpt-5.6-terra`、`gpt-5.6-luna` 和已有的 `gpt-5.6-sol`，并出现搜索匹配添加入口；网络请求为 `GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50`，验证 OpenAI 供应商范围和三模型候选均已同步到编辑体验。
12. MCP 控制台没有 JavaScript `error` 或 `warn`；仅保留浏览器 DevTools 的既有表单可访问性 `issue` 提示：缺少 autocomplete 属性、部分 label 关联不完全，本轮未扩大范围处理。
13. MCP 验证截图保存为 `/tmp/nexustok-channel-debounced-filter-validation.png`。

## 本轮实施评审：渠道表格接入公共 useDataTable

### 差异来源

继续对照 `/opt/project/new-api-main/web/default/src/features/channels/components/channels-table.tsx` 后确认，new-api 最新渠道表格已经使用公共 `useDataTable()` 组合 TanStack Table 状态，页面侧只保留渠道查询、过滤条件、列定义和少量业务逻辑。NexusTok 已在前序切片新增公共 `useDataTable()`，并先在模型定价表格验证了排序、分页、列显隐、行选择和 localStorage 持久化能力；但渠道表格仍直接调用 `useReactTable()`，并手写 `columnVisibility`、`rowSelection`、`expanded`、`getCoreRowModel()`、`getExpandedRowModel()` 和页码范围 effect。

渠道列表是管理员高频页面，后续还要继续吸收 new-api 的批量模式、敏感信息显隐、列显隐持久化、状态筛选记忆等能力。如果继续让渠道表格保留手写 TanStack 配置，后续每个表格能力都需要重复处理状态组合，回归风险会逐步增大。本轮先把渠道表格接入 NexusTok 原生公共 `useDataTable()`，为后续更细的渠道表格体验对齐打底。

本轮不引入 new-api 的 batch mode UI、敏感信息显隐按钮、状态筛选 localStorage 记忆或权限按钮重排；这些会改变可见交互和权限语义，需要单独评审。本轮只替换表格状态组合层，并保留当前 NexusTok 的移动端 `ChannelsMobileList`、账号池管理入口、权限守卫、批量操作栏和视图模式存储 key。

### 需求分析

1. 将 `web/default/src/features/channels/components/channels-table.tsx` 从直接 `useReactTable()` 切换到公共 `useDataTable()`。
2. 保留当前渠道表格行为：服务端分页、服务端排序、服务端过滤、Tag 聚合展开、非 Tag 行可选择、批量操作栏、移动端自定义列表、桌面表格/卡片视图切换、禁用渠道行样式和页码范围修正。
3. 使用稳定的 `initialColumnVisibility`，默认继续隐藏 `models` 与 `tag` 列。
4. 新增渠道列显隐 localStorage key，把列显隐偏好交给公共 hook 防御式读写，向 new-api 的表格状态持久化能力靠齐。
5. 保留现有 `CHANNELS_VIEW_MODE_STORAGE_KEY = 'channels-table-view-mode'`，避免已有用户桌面表格/卡片视图偏好被重置。
6. 不修改渠道列定义、渠道 API、后端查询参数、权限判断、i18n 文案、移动端列表和 `/opt/project/new-api-main` 源码。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 渠道表格 | `web/default/src/features/channels/components/channels-table.tsx` | 使用 `useDataTable()` 替代内联 `useReactTable()` 与手写表格状态；新增列显隐存储 key。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审、实施结果和验证记录。 |

### 风险评估

1. 分页回归风险：渠道表格使用后端分页，`pageCount` 必须继续来自 `totalCount / pageSize`，并在总数变化时调用 `ensurePageInRange()` 修正越界页。
2. 过滤回归风险：状态、类型、分组、模型和全局搜索仍由 `useTableUrlState` 控制；切换 hook 不能改变 URL 参数或 `searchChannels()` 的触发条件。
3. 排序回归风险：当前只允许 `id`、`name`、`priority`、`balance`、`response_time`、`test_time` 进入服务端排序参数，切换 hook 后必须继续走 `handleSortingChange()`。
4. 展开回归风险：Tag 模式依赖 `getSubRows()` 和 expanded row model；公共 hook 需要显式开启 `withExpandedRowModel`。
5. 选择回归风险：批量操作依赖 `table.getFilteredSelectedRowModel()`；公共 hook 内部 rowSelection 必须保留，且 Tag 聚合行仍不可选择。
6. 列显隐风险：新增 localStorage key 只影响渠道列显示偏好；默认隐藏列必须保持 `models` 和 `tag`，避免第一次加载展示过宽。
7. 范围风险：如果本轮同时加入 batch mode、敏感显隐和状态筛选记忆，会引入新文案、权限体验和更多 UI 分支。本轮先迁状态 Hook，后续再分片评审这些可见能力。

### 方案评审

采用“状态层替换，业务层不变”的渐进方案：

1. 在渠道表格中新增 `CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY = 'channels-column-visibility'` 和稳定常量 `CHANNELS_INITIAL_COLUMN_VISIBILITY`。
2. 删除 `VisibilityState`、`ExpandedState`、`useReactTable()`、`getCoreRowModel()`、`getExpandedRowModel()` 和本地 `columnVisibility` / `rowSelection` / `expanded` state。
3. 调用 `useDataTable({ data, columns, totalCount, sorting, columnFilters, pagination, globalFilter, initialColumnVisibility, columnVisibilityStorageKey, enableRowSelection, onSortingChange, onColumnFiltersChange, onPaginationChange, onGlobalFilterChange, getSubRows, manualPagination, manualSorting, manualFiltering, withExpandedRowModel, ensurePageInRange })`。
4. 保留 `handleSortingChange()`、查询参数构造、过滤选项、`DataTablePage`、`ChannelsMobileList`、`DataTableBulkActions` 和行样式逻辑不变。
5. 验证顺序：`bun run typecheck`、`bun test src/features/channels`、`bun test src/features/system-settings`、`bun run i18n:sync`、`bun run build`、`git diff --check`；最后用 MCP 登录 `http://192.168.0.202:3003/channels` 验证渠道列表、模型筛选、编辑抽屉和控制台/网络。

### 实施结果

1. `web/default/src/features/channels/components/channels-table.tsx` 已从直接 `useReactTable()` 切换为公共 `useDataTable()`。
2. 删除渠道表格本地 `columnVisibility`、`rowSelection`、`expanded` state 和手写页码范围 `useEffect`；这些状态现在由公共 hook 统一管理。
3. 新增 `CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY = 'channels-column-visibility'`，渠道表格列显隐偏好会由 `useDataTable()` 防御式读写 localStorage。
4. 新增稳定常量 `CHANNELS_INITIAL_COLUMN_VISIBILITY`，默认仍隐藏 `models` 与 `tag` 列，保持首次加载宽度和旧页面一致。
5. 渠道表格继续保留服务端分页、服务端排序、服务端过滤、Tag 聚合展开、非 Tag 行可选择、批量操作栏、自定义移动端 `ChannelsMobileList`、桌面卡片/表格视图切换、禁用渠道行样式和模型筛选组合输入保护。
6. 本轮没有修改渠道列定义、渠道 API、后端查询参数、权限判断、账号池管理入口、渠道编辑抽屉、i18n locale 文件和 `/opt/project/new-api-main` 源码。

### 验证记录

1. 已运行 `cd web/default && bun run typecheck`，TypeScript 检查通过，确认 `useDataTable()` 泛型、渠道列定义、行选择和 Tag 子行配置类型正确。
2. 已运行 `cd web/default && bun test src/features/channels`，56 个渠道相关测试通过。
3. 已运行 `cd web/default && bun test src/features/system-settings`，27 个系统设置相关测试通过，确认公共 data-table hook 的新增消费没有回归近期模型定价与系统设置能力。
4. 已运行 `cd web/default && bun run i18n:sync`，同步通过；本轮未新增可见翻译 key。
5. 已运行 `cd web/default && bun run build`，Rsbuild 生产构建通过。
6. 已运行 `git diff --check`，未发现空白错误。
7. MCP 使用账号 `c1cada` 登录 `http://192.168.0.202:3003/` 成功，打开 `/channels` 后页面正常显示渠道列表、`11111` 行和 `Filter by model...` 输入框。
8. MCP 读取浏览器 localStorage，确认新列显隐持久化 key 已写入：`channels-column-visibility = {"models":false,"tag":false}`，说明热更新页面已使用公共 `useDataTable()` 的列显隐持久化能力。
9. MCP 重新验证模型筛选组合输入：`compositionstart` 后输入 `gp` 并等待 700ms，URL 仍为 `/channels`；`compositionend` 后输入 `gpt-5.6` 并等待 debounce，URL 正确变为 `/channels?model=gpt-5.6`。
10. MCP 清空筛选后打开渠道 `11111` 的 `Edit Channel` 抽屉，确认仍显示 `Basic Information`、`Credentials`、`Models & Groups`、`Advanced Settings`，模型区显示 `Selected 3`、`Vendor: OpenAI` 和 `gpt-5.6-sol`。
11. MCP 网络请求均为预期读取请求，包括 `/api/channel/`、`/api/channel/search?keyword=&model=gpt-5.6...` 和 `/api/channel/1`；没有出现保存类写请求。
12. MCP 控制台没有 JavaScript `error` 或 `warn`；仅保留浏览器 DevTools 的既有表单可访问性 `issue` 提示，本轮未扩大范围处理。
13. MCP 验证截图保存为 `/tmp/nexustok-channel-use-data-table-validation.png`。

## 本轮实施评审：编辑渠道页搜索添加修复与新版页面对齐

### 差异来源

用户反馈编辑渠道页“搜索添加时不正确”，并要求继续对齐 `/opt/project/new-api-main` 最新编辑渠道页面。复核当前实现后发现：前序报告曾提出将远程模型库搜索独立为 `Search model library` 输入，但实际代码仍把批量追加入口塞在模型 `MultiSelect` 下拉内容头部。这个实现会让“点击候选行添加单个模型”和“点击批量按钮添加全部搜索匹配”混在同一个弹层里，管理员搜索 `gpt-5.6` 时很容易只选中一个候选，从体验上表现为“搜索添加只同步了一个模型”。

继续对照 new-api 最新编辑渠道页后确认，当前 NexusTok 已经吸收了大部分低风险结构能力，包括分段抽屉、左侧导航、基础信息/凭据/模型/高级设置分区、类型图标和模型映射 guardrail。不能直接整页覆盖 new-api，因为当前项目还保留并增强了 NexusTok 原生能力：细粒度权限、全局账号池、Codex OAuth、模型元信息搜索、上游模型检测、高级设置子导航和账号池专属配置。

### 需求分析

1. 搜索 `gpt-5.6` 时，页面必须明确展示模型库搜索结果摘要，例如 `3 matched · 2 new · 1 already selected`，而不是让管理员只能在多选弹层里逐个点候选。
2. 批量追加按钮必须成为模型区的独立主操作，点击后追加所有搜索匹配中当前渠道尚未包含的模型；不能把前缀 `gpt-5.6` 作为自定义模型写入。
3. 模型 `MultiSelect` 仍保留本地选择和自定义模型能力，但不再承担远程搜索批量追加入口，降低弹层关闭、候选高亮、Enter 键和搜索输入互相覆盖的风险。
4. 页面继续对齐 new-api 最新编辑渠道页的清晰分区和紧凑密度，但必须保留 NexusTok 的权限、账号池、Codex、上游检测和高级设置能力。
5. 本轮不修改后端模型同步、模型库搜索接口、数据库结构、渠道保存接口、relay、计费和权限模型。

### 影响范围分析

| 范围 | 文件 | 影响 |
| --- | --- | --- |
| 编辑渠道抽屉 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 将远程模型库搜索和批量追加入口从 `MultiSelect` 弹层拆出为独立输入、摘要和按钮；保留模型多选的本地候选/自定义能力。 |
| 多选组件消费方式 | `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | 不改 `MultiSelect` 组件本身，只调整编辑渠道页调用参数，降低复用风险。 |
| i18n | `web/default/src/i18n/locales/{en,zh,fr,ja,ru,vi}.json` | 优先复用已有 `Search model library`、`Add {{count}} new model(s)`、搜索摘要和加载态文案；如新增文案需补齐六语。 |
| 差异报告 | `docs/features/new-api-main-diff-analysis.md` | 记录本轮需求分析、影响范围、风险评估、方案评审、实施结果和验证记录。 |

### 风险评估

1. 搜索行为回归风险：拆出独立输入后，`MultiSelect` 内部搜索不应再触发远程模型库查询；远程查询只由 `Search model library` 输入驱动。
2. 添加数量错误风险：按钮文案、预览列表和实际追加必须共用 `buildModelSearchAppendSummary()` / `getMissingModelSearchMatches()` 的去重规则，避免展示 2 个但实际追加 1 个或 3 个。
3. 异步污染风险：全量分页扫描期间可能切换渠道、关闭抽屉或改变关键词；继续使用请求序号和 `isModelSearchAppendContextCurrent()` 丢弃过期结果。
4. 表单提交风险：搜索追加只更新前端草稿，不自动保存渠道；MCP 验证时不得点击 `Update Channel`，避免污染运行态测试数据。
5. 页面密度风险：新增独立搜索区不能把模型区做成卡片套卡片，也不能让文本在移动端挤压重叠；使用现有 `border` 分组、`Badge`、`Input`、`Button` 和稳定 gap。

### 方案评审

采用“搜索批量入口独立化，底层逻辑保持不变”的方案：

1. 在模型区 `MultiSelect` 上方新增 `Search model library` 行，左侧为远程模型库搜索输入，右侧为批量追加按钮。
2. 搜索结果有效时展示摘要 Badge：命中数、可新增数、已存在数、供应商范围；有待追加模型时展示 `Will add` 预览。
3. 批量按钮使用 `Add {{count}} new model(s)`，如果后端还有未扫描分页则继续显示 `Scan all search results`，点击后仍调用 `fetchAllModelSearchModelNames()` 扫描所有分页。
4. `MultiSelect` 移除远程搜索相关的 `searchValue`、`onSearchChange`、`onSearchSubmit`、`contentHeader`、`contentFooter` 和 loading 文案，只负责当前 `modelOptions` 内选择与自定义。
5. 搜索输入 `Enter` 键在可追加时触发批量追加；加载中或无新增项时只阻止表单误提交。
6. 保留当前 `modelOptions` 合并远程搜索结果、系统模型和已选模型的逻辑，使搜索后仍可单独选择某个模型，但批量添加入口不再依赖 Combobox 弹层。

### 实施结果

1. 已在编辑渠道模型区新增独立的 `Search model library` 搜索行，搜索输入、追加按钮、结果摘要和 `Will add` 预览都位于模型多选框之外。
2. 批量追加按钮文案从 `Add {{count}} search match(es)` 调整为 `Add {{count}} new model(s)`，更明确表示只追加当前渠道缺失的模型。
3. `MultiSelect` 不再接管远程模型库搜索状态，移除了 `searchValue`、`onSearchChange`、`onSearchSubmit`、`contentHeader`、`contentFooter`、`isLoading`、`loadingText` 和 `submitSearchOnEnterWithMatches` 等远程追加相关参数。
4. `MultiSelect` 继续保留本地候选选择、自定义模型添加、点击复制 chip、已选项折叠和空搜索时保护已选项不被误删等 NexusTok 原生能力。
5. 关闭抽屉时会清空模型库搜索词和追加请求状态，避免下次打开沿用旧搜索上下文。
6. 点击批量追加后仍会调用全量分页扫描，搜索 `gpt-5.6` 时正确只把 `gpt-5.6-terra`、`gpt-5.6-luna` 合并到前端表单草稿，不会把前缀 `gpt-5.6` 写成自定义模型。
7. 本轮没有修改 `/opt/project/new-api-main` 源码、后端模型搜索接口、渠道保存接口、数据库结构、权限模型、relay 或计费逻辑。

### 验证记录

1. 已运行 `cd web/default && bun run typecheck`，TypeScript 检查通过。
2. 已运行 `cd web/default && bun test src/features/channels`，56 个渠道相关测试全部通过。
3. 已运行 `cd web/default && bun test src/features/system-settings`，27 个系统设置相关测试全部通过。
4. 已运行 `cd web/default && bun run i18n:sync`，同步通过；本轮复用已有六语文案，没有新增缺失 key。
5. 已运行 `cd web/default && bunx eslint src/features/channels/components/drawers/channel-mutate-drawer.tsx`，ESLint 通过。
6. 已运行 `cd web/default && bun run build`，Rsbuild 生产构建通过。
7. 已运行 `git diff --check`，未发现空白错误。
8. MCP 使用账号 `c1cada` 登录 `http://192.168.0.202:3003/`，进入 `/channels` 并打开渠道 `11111` 的 `Edit Channel` 抽屉，确认页面热更新已生效，模型区显示独立 `搜索模型库` 输入和独立追加按钮。
9. MCP 在 `搜索模型库` 输入 `gpt-5.6` 后，页面显示 `命中 3 个 · 可新增 2 个 · 已存在 1 个`，并预览 `将添加: gpt-5.6-terra, gpt-5.6-luna`。
10. MCP 点击 `添加 2 个新模型` 后，表单草稿显示 `已选 5 个`，新增 chip 为 `gpt-5.6-terra` 与 `gpt-5.6-luna`；没有点击 `更新渠道`。
11. MCP 网络请求确认只有读取与模型搜索请求：`GET /api/channel/1`、`GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=50`、`GET /api/models/search?keyword=gpt-5.6&vendor=OpenAI&p=1&page_size=100`，没有渠道保存类写请求。
12. MCP 在浏览器上下文携带 `NexusTok-User: 1` 读取 `GET /api/channel/1`，后端仍返回 `models = "gpt-5.4,gpt-5.5,gpt-5.6-sol"`，确认验证未污染运行态数据。
13. MCP 控制台没有 JavaScript `error` 或 `warn`；仅保留既有表单可访问性 `issue`：缺少 autocomplete 属性、部分 label 关联不完全，本轮未扩大范围处理。
14. MCP 验证截图保存为 `/tmp/nexustok-channel-edit-model-search-validation.png`。
