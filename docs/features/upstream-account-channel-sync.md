# 上游账号同步渠道开发台账

本文档记录“通过 new-api / sub2api 账号密码同步密钥、倍率、余额并生成可调度渠道账号”的需求分析、影响范围、风险评估、方案评审和开发进度。后续实现过程中每个阶段都应更新本文档，避免长上下文导致目标偏移。

## 背景与目标

管理员在新增上游渠道时，除了手动填写单个 API Key 或批量 Key，还需要能够直接填写目标平台账号密码。NexusTok 登录目标平台后自动读取该账号名下所有密钥、密钥所在分组、模型倍率以及账号余额/已用额度，并把这些密钥归入同一个渠道名称下，同时允许每个密钥在 NexusTok 中独立配置模型、分组、优先级、权重、Base URL、参数覆盖、Header 覆盖、状态码映射和状态。

首批目标平台：

| 平台 | 登录地址 | 本地项目 | 用途 |
| --- | --- | --- | --- |
| new-api | `http://118.31.248.175:3000/` | `/opt/project/new-api-main` | 学习账号登录、Token/分组/倍率/余额接口 |
| sub2api | `http://38.12.20.53:18088/login` | `/opt/project/sub2api-main` | 学习账号登录、订阅/密钥/倍率/余额接口 |

本项目验证环境：

| 项 | 值 |
| --- | --- |
| NexusTok 页面 | `http://192.168.0.202:3003/` |
| 登录账号 | `c1cada` |
| 登录密码 | 已由需求提供，仅用于本地验证，不写入代码或文档 |
| 部署方式 | Docker 容器热更新；每次修改后必须访问 3003 页面确认更新，页面未变化时先重启容器再验证 |

## 需求拆解

### R1：添加上游渠道时支持账号密码同步

新增渠道表单需要提供“手动密钥”和“平台账号同步”两种凭证来源。选择平台账号同步时，管理员选择平台类型 `new-api` 或 `sub2api`，填写平台 Base URL、用户名和密码，点击创建或刷新后由后端登录目标平台，拉取当前账号包含的所有密钥及对应分组模型倍率。

创建或刷新都必须走后端代理请求，前端不得直接向目标平台发送账号密码，避免浏览器跨域、凭证泄露和审计缺失。

### R2：支持按倍率自动设置优先级和权重，也支持手动调整

后端同步返回的密钥条目需要包含可展示的倍率摘要。前端提供策略选项：

| 策略 | 行为 |
| --- | --- |
| 自动 | 根据密钥/分组/模型倍率生成建议 `priority` 和 `weight`，并在保存时应用到渠道账号 |
| 手动 | 页面展示倍率，管理员逐条编辑 `priority` 和 `weight` |

建议默认采用保守自动策略：倍率越低优先级越高；同优先级内按权重分流。由于不同平台的倍率含义可能不同，自动策略必须先保存为“建议值”，允许用户在提交前覆盖。

### R3：账号密码同步时同步已使用余额和剩余余额

选择平台账号同步时，渠道的 `used_quota` 和 `balance` 需要按目标平台账号真实情况更新。若目标平台能区分已用和剩余，则分别写入；若只能提供总余额或剩余额度，则只更新可确认字段，并在同步结果中提示缺失字段。

余额同步必须复用现有渠道余额字段，不应把目标平台登录密码长期保存在渠道顶层明文字段中。

### R4：账号密码模式允许先不填模型和类型

当前新增渠道校验要求 `type` 和 `models` 存在。账号密码同步模式下需要放宽：

- 前端表单允许先选择“平台账号同步”，只填写平台、Base URL、账号、密码和渠道名称。
- 后端新增接口或新增模式必须绕开传统 `validateChannel` 对模型的强制要求，待同步结果返回后再由管理员确认模型和渠道类型。
- 最终落库前仍要有可调度的 `type`、`models` 和 `group`；若平台返回的数据不足以推断，渠道可先保存为禁用草稿或要求用户补全后提交。优先选择“预览后确认创建”，避免引入半成品渠道状态。

### R5：自动获取密钥放在同一渠道名称下，并允许每个密钥独立配置

推荐复用现有 `ChannelAccount` 能力，而不是扩展 `Channel.Key` 的多 Key 字符串：

| 层级 | 保存内容 |
| --- | --- |
| `Channel` | 渠道名称、类型、Base URL、默认模型、默认分组、默认倍率策略、余额、已用额度 |
| `ChannelAccount` | 每个同步到的密钥、密钥名称、密钥分组、模型限制、优先级、权重、状态、Base URL 覆盖、参数/Header 覆盖、状态码映射 |

渠道创建后设置 `channel_info.credential_mode = account_pool`，启用渠道级账号池调度。这样“同一个渠道名称下多个密钥”天然表现为一个渠道下多条渠道账号，且已支持账号级配置和错误禁用。

### R6：修复渠道失效后的自动降级

现有请求链路包括 `middleware.Distribute` 首次选择渠道、`controller.Relay` 重试时调用 `getChannel` 再选渠道、`service.DisableChannel` 自动禁用渠道、`model.CacheUpdateChannelStatus` 更新缓存。风险点在于：

- 请求失败后异步禁用渠道，下一次重试可能在缓存更新前再次选中同一渠道。
- `GetRandomSatisfiedChannel` 的 `retry` 只按优先级层级切换，不排除当前请求已经失败的渠道；同一优先级内可能重新随机到失败渠道。
- 当 `retry >= len(uniquePriorities)` 时会钳制到最低优先级，导致重试次数多于优先级数时反复打最低优先级渠道。
- 亲和性命中失败时可能受 `skip_retry_on_failure` 配置影响直接中止，需要与普通降级路径区分。
- 内存缓存路径从 `group2model2channels` 取出渠道 ID 后没有二次校验 `channel.Status == enabled`；如果缓存列表残留旧 ID，会返回已禁用渠道。
- 数据库 fallback 路径当前主要依赖 `abilities.enabled`，没有显式 join/filter `channels.status = enabled`，如果能力表与渠道状态不同步，仍可能选中禁用渠道。
- multi-key 渠道禁用某个 key 和禁用整个渠道的边界需要更清晰；`usingKey` 为空或未匹配时不能默认操作第一个 key，否则可能误禁用 key 0。
- `specific_channel_id` 语义需要明确：管理员指定渠道调试时不应自动降级；现有 `types.IsChannelError` 判断顺序可能让部分 channel 错误绕过“不重试指定渠道”的限制。

修复方向：

- 在当前请求上下文中维护已失败渠道 ID 集合，重试选择时排除这些渠道。
- 渠道发生可重试错误后，先同步把当前渠道加入排除集合，再进入下一轮选择；自动禁用仍可异步执行。
- 渠道选择函数新增可选排除参数，内存缓存和数据库查询路径都要一致过滤。
- 重试次数不应仅等同优先级索引；应按“优先级层级 + 当前请求排除集合”选择仍可用候选。当当前优先级无候选时降到下一优先级。
- `CacheUpdateChannelStatus` 禁用时需要移除候选列表中的所有匹配 ID，并在缓存构建时去重。
- DB fallback 需要和内存缓存路径保持一致，过滤渠道状态、排除本请求失败渠道，并保持 SQLite/MySQL/PostgreSQL 兼容。
- 补充测试覆盖同优先级权重降级、跨优先级降级、自动禁用缓存更新、亲和性失效、指定渠道不降级、multi-key key 匹配失败不误伤、账号池账号失败后同渠道换账号等路径。

## 影响范围分析

### 后端

| 模块 | 影响 |
| --- | --- |
| `controller/channel.go` | 新增账号同步预览/创建/刷新入口，调整新增渠道校验分支 |
| `controller/channel_account.go` | 可能复用批量创建渠道账号逻辑，或增加同步 upsert 入口 |
| `model/channel.go` | 可能扩展 `ChannelInfo` 或 `OtherSettings` 存储同步来源元数据 |
| `model/channel_account.go` | 可能新增外部平台 token id / digest 字段；若避免迁移，可先写入 `OtherSettings` |
| `service/channel_select.go` | 增加当前请求排除渠道后的选择逻辑 |
| `model/channel_cache.go` | 内存缓存选择支持排除集合和优先级降级 |
| `controller/relay.go` | 错误后记录失败渠道，保证重试不再命中同一渠道 |
| `controller/channel-billing.go` | 平台账号同步余额可能新增专用来源，不宜混入现有 provider key 余额查询 |
| `router/channel-router.go` | 新增平台账号同步相关 API 路由 |
| `common/json.go` 使用约束 | 所有 JSON marshal/unmarshal 继续通过 `common.*` 封装 |

### 前端 `web/default`

| 模块 | 影响 |
| --- | --- |
| `features/channels/types.ts` | 新增同步平台、预览结果、同步策略类型 |
| `features/channels/api.ts` | 新增登录同步预览、创建、刷新 API |
| `features/channels/lib/channel-form.ts` | 表单校验按凭证来源放宽模型/类型要求 |
| `features/channels/components/drawers/channel-mutate-drawer.tsx` | 新增账号同步入口、预览表格、倍率策略和逐密钥配置 |
| `features/channels/components/dialogs/channel-account-pool-dialog.tsx` | 复用或增强每个密钥的独立配置界面 |
| `src/i18n/locales/*.json` | 新增 UI 文案需要同步 en/zh/fr/ja/ru/vi |

### 数据库兼容性

优先不新增字段，使用现有 `settings` / `OtherSettings` JSON 文本保存同步元数据，降低 SQLite、MySQL、PostgreSQL 迁移风险。若后续需要查询外部平台 token id，则新增字段必须使用 GORM AutoMigrate 可兼容类型，避免数据库专用 JSON/JSONB。

## 目标平台接口初查

以下结论来自本地项目代码静态分析，后续实现前还要使用真实测试平台和 MCP/浏览器请求确认响应格式。

### new-api

new-api 使用 `/api` 前缀，标准响应为 `{"success": true, "message": "", "data": ...}`；很多业务错误仍返回 HTTP 200，需要同时检查 HTTP 状态和 `success`。new-api 登录成功后会在服务端 session 写入 `id`、`username`、`role`、`status` 和 `group`。NexusTok 后端适配时需要维护 cookie jar；受保护接口除 session 或用户 access token 外，还需要带 `New-Api-User: <user_id>`。

| 能力 | 方法与路径 | 鉴权 | 代码位置 | 备注 |
| --- | --- | --- | --- | --- |
| 状态/单位 | `GET /api/status` | 无 | `/opt/project/new-api-main/controller/misc.go` | 返回 `quota_per_unit`、Turnstile 状态等；余额换算必须读取实例配置 |
| 登录 | `POST /api/user/login?turnstile=` | 无，可能受 Turnstile/全局限速影响 | `/opt/project/new-api-main/controller/user.go` | 请求体 `username`、`password`；Turnstile token 在 query；成功返回用户基础信息 |
| 2FA 补全 | `POST /api/user/login/2fa` | pending session cookie | `/opt/project/new-api-main/controller/twofa.go` | 首次登录若返回 `require_2fa`，保留 cookie 后提交 `code` |
| 当前用户 | `GET /api/user/self` | session/access token + `New-Api-User` | `/opt/project/new-api-main/controller/user.go` | 返回 `quota`、`used_quota`、`group` 等字段，可用于余额同步 |
| 用户可用分组 | `GET /api/user/self/groups` | session/access token + `New-Api-User` | `/opt/project/new-api-main/controller/group.go` | 返回分组 map，值包含 `ratio` 和 `desc` |
| 用户模型 | `GET /api/user/models?group=` | session/access token + `New-Api-User` | `/opt/project/new-api-main/controller/user.go` | 可辅助推断模型列表 |
| 用户 access token | `GET /api/user/token` | session + `New-Api-User` | `/opt/project/new-api-main/controller/user.go` | 会生成/更新用户 access token；不应每次同步都调用 |
| Token 列表 | `GET /api/token/?p=&page_size=` | session/access token + `New-Api-User` | `/opt/project/new-api-main/controller/token.go` | 返回脱敏 `key`、`name`、`group`、`remain_quota`、`used_quota`、`model_limits` 等，单页最大 100 |
| Token 完整 Key | `POST /api/token/:id/key` | session/access token + `New-Api-User` | `/opt/project/new-api-main/controller/token.go` | 逐条读取完整 Key，属高敏操作 |
| Token 批量 Key | `POST /api/token/batch/keys` | session/access token + `New-Api-User` | `/opt/project/new-api-main/controller/token.go` | 请求体 `{"ids":[...]}`；最多 100 个 id |
| 倍率配置 | `GET /api/ratio_config` | 无登录也可访问，但依赖 `ExposeRatioEnabled` | `/opt/project/new-api-main/controller/ratio_config.go` | 返回 `model_ratio`、`completion_ratio`、`cache_ratio`、`create_cache_ratio`、`model_price` |
| API Key 自身用量 | `GET /api/usage/token/` | `Authorization: Bearer <sk-key>` | `/opt/project/new-api-main/controller/token.go` | 返回 `total_granted`、`total_used`、`total_available`、模型限制等 |
| OpenAI 兼容余额 | `GET /dashboard/billing/subscription`、`GET /dashboard/billing/usage` | TokenAuth，不是 session | `/opt/project/new-api-main/controller/billing.go` | 更适合已有 API Key 余额查询，不适合账号密码同步首选 |

new-api 的余额单位是 quota 整数，不能假设固定比例。适配时先读 `/api/status.data.quota_per_unit`，再按 `usd = quota / quota_per_unit` 换算，同时保留 raw quota。密钥级余额可以由 Token 的 `remain_quota + used_quota`、`used_quota`、`remain_quota` 计算；账号级余额优先使用 `/api/user/self` 的 `quota` 和 `used_quota`。

### sub2api

sub2api 使用 `/api/v1` 前缀，标准响应为 `{"code": 0, "message": "success", "data": ...}`；错误使用真实 HTTP 状态码。分页响应为 `data.items/total/page/page_size/pages`。前端默认 Axios 请求使用 Bearer token，并带 `withCredentials`。登录成功返回 `access_token`、`refresh_token`、`expires_in` 和 `user`。NexusTok 适配时应使用 Bearer token，不依赖浏览器本地存储。

| 能力 | 方法与路径 | 鉴权 | 代码位置 | 备注 |
| --- | --- | --- | --- | --- |
| 登录 | `POST /api/v1/auth/login` | 无，可能受 Turnstile 设置影响 | `/opt/project/sub2api-main/backend/internal/handler/auth_handler.go` | 请求体 `email`、`password`、可选 `turnstile_token`；可能返回 2FA 临时态 |
| 2FA 补全 | `POST /api/v1/auth/login/2fa` | 临时 token | `/opt/project/sub2api-main/backend/internal/handler/auth_handler.go` | 首次登录若返回 `requires_2fa`，提交 `temp_token` 和 `totp_code` |
| 刷新 token | `POST /api/v1/auth/refresh` | refresh token | `/opt/project/sub2api-main/backend/internal/handler/auth_handler.go` | access token 过期或 401 时刷新 |
| 当前用户 | `GET /api/v1/auth/me` | Bearer | `/opt/project/sub2api-main/backend/internal/handler/auth_handler.go` | 返回当前用户信息 |
| 用户资料 | `GET /api/v1/user/profile` | Bearer | `/opt/project/sub2api-main/frontend/src/api/user.ts` | 用户类型包含 `balance` |
| 用户平台额度 | `GET /api/v1/user/platform-quotas` | Bearer | `/opt/project/sub2api-main/frontend/src/api/user.ts` | 返回平台维度限额/用量，用于余额或额度补充 |
| 用户可用分组 | `GET /api/v1/groups/available` | Bearer | `/opt/project/sub2api-main/frontend/src/api/groups.ts` | `Group` 含 `rate_multiplier`、`peak_rate_multiplier`、平台和订阅类型 |
| 用户专属分组倍率 | `GET /api/v1/groups/rates` | Bearer | `/opt/project/sub2api-main/frontend/src/api/groups.ts` | 返回 `group_id -> rate_multiplier` 覆盖 |
| API Key 列表 | `GET /api/v1/keys?page=&page_size=` | Bearer | `/opt/project/sub2api-main/frontend/src/api/keys.ts` | 前端封装使用 `/keys`；后端注释里存在旧名 `/api/v1/api-keys`，实现前要以路由注册为准 |
| API Key 详情 | `GET /api/v1/keys/:id` | Bearer | `/opt/project/sub2api-main/frontend/src/api/keys.ts` | `ApiKey` 类型包含 `key`、`group_id`、`quota`、`quota_used`、`group` |
| 用量统计 | `GET /api/v1/usage/dashboard/stats`、`POST /api/v1/usage/dashboard/api-keys-usage` 等 | Bearer | `/opt/project/sub2api-main/frontend/src/api/usage.ts` | 可用于交叉校验账号/Key 用量；批量 key 用量最多 100 个 id |
| 可用渠道与模型 | `GET /api/v1/channels/available` | Bearer | `/opt/project/sub2api-main/frontend/src/api/channels.ts` | 返回用户可见渠道、平台、分组和模型定价 |

sub2api 的分组倍率来自 `Group.rate_multiplier` 和用户专属 `/groups/rates` 覆盖；API Key 的剩余可由 `quota - quota_used` 推导，`quota = 0` 表示无限制。用户账号余额来自 `User.balance`。如果目标平台启用了 2FA 或 Turnstile，第一版账号密码同步应返回明确错误，提示需要关闭或后续扩展交互式认证。

sub2api 的金额与配额字段是 USD float，不需要 `QuotaPerUnit` 换算。它没有单独的用户 `used_balance` 字段，累计已用优先采用 `/usage/dashboard/stats.total_actual_cost`；密钥级用量优先采用 API Key 的 `quota_used`，需要更精确趋势时再调用 usage dashboard 接口。

### 平台关键差异

| 维度 | new-api | sub2api |
| --- | --- | --- |
| API 前缀 | `/api` | `/api/v1` |
| 响应 envelope | `success/message/data`，业务错误常见 HTTP 200 | `code/message/data`，错误使用真实 HTTP 状态 |
| 登录字段 | `username`、`password` | `email`、`password` |
| Turnstile | query `turnstile` | JSON `turnstile_token` |
| 2FA | pending session cookie + `code` | `temp_token` + `totp_code` |
| 后台鉴权 | session/access token + `New-Api-User` | JWT Bearer |
| Key 暴露 | 列表脱敏，需 reveal 单个或批量真实 key | 当前本地代码列表直接返回完整 key |
| 余额单位 | quota int，需 `quota_per_unit` 换算 | USD float |
| 分组标识 | group name 字符串 | group id + group name + platform |
| 倍率/价格 | 可选暴露 `ratio_config` | 分组倍率 + 用户倍率覆盖 + 模型 pricing |
| 已用余额 | 用户 `used_quota` 直接可用 | 需 usage 聚合或 key `quota_used` |

## 初步接口方案

### 后端 API 草案

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/channel/upstream-account/preview` | 使用账号密码登录目标平台，返回密钥、倍率、余额预览，不落库 |
| `POST` | `/api/channel/upstream-account/create` | 基于预览结果和用户配置创建一个渠道及多条 `ChannelAccount` |
| `POST` | `/api/channel/:id/upstream-account/refresh` | 使用重新输入的账号密码刷新已存在渠道的密钥、倍率、余额 |

账号密码不长期保存。刷新时要求管理员重新输入密码，或者后续单独设计加密凭证存储和轮换策略。

### 适配层草案

新增 `service/upstreamaccount` 包，按平台隔离实现 `NewAPIAdapter` 和 `Sub2APIAdapter`。统一接口既要支持登录挑战，也要支持分页、倍率和用量快照：

```go
type PlatformClient interface {
    Login(ctx context.Context, credential LoginCredential, challenge *LoginChallenge) (*Session, *LoginChallenge, error)
    Refresh(ctx context.Context, session *Session) error
    FetchAccountSnapshot(ctx context.Context, session *Session) (*AccountSnapshot, error)
}
```

统一快照结构：

```go
type AccountSnapshot struct {
    Platform string
    BaseURL string
    Keys []SyncedKey
    Groups []SyncedGroup
    Rates *RateSnapshot
    Usage *UsageSnapshot
    Balance *BalanceSnapshot
}
```

`SyncedKey` 至少包含：

| 字段 | 说明 |
| --- | --- |
| `ExternalID` | 目标平台 token/key id |
| `Name` | 密钥名称 |
| `Key` | 完整 API Key，仅在预览创建链路内短暂返回给后端使用；前端展示必须脱敏 |
| `Groups` | 密钥可用分组 |
| `Models` | 可用模型列表 |
| `ModelRatios` | 模型倍率 |
| `GroupRatios` | 分组倍率 |
| `SuggestedPriority` | 自动策略建议优先级 |
| `SuggestedWeight` | 自动策略建议权重 |

统一结构必须保留 raw 字段，避免丢失平台特有信息：

| 结构 | 关键字段 |
| --- | --- |
| `Session` | `platform`、`base_url`、`user_id`、`access_token`、`refresh_token`、`cookie_jar`、`expires_at`、`auth_headers` |
| `LoginChallenge` | `turnstile_required`、`two_factor_required`、`temp_token`、`cookie_required`、`message` |
| `SyncedKey` | `external_id`、`key`、`masked_key`、`name`、`status`、`group_id`、`group_name`、`quota_limit_usd`、`quota_used_usd`、`quota_raw`、`expires_at`、`unlimited`、`raw` |
| `SyncedGroup` | `id`、`name`、`platform`、`ratio`、`description`、`limits`、`peak_rate`、`raw` |
| `RateSnapshot` | `model_ratios`、`completion_ratios`、`cache_ratios`、`model_prices`、`group_rates`、`channel_model_pricing`、`source`、`partial` |
| `UsageSnapshot` | `total_actual_cost_usd`、`total_cost_usd`、`today_actual_cost_usd`、`by_key`、`by_platform`、`raw` |

适配层需要提供公共辅助：

| 辅助 | 说明 |
| --- | --- |
| `EnvelopeDecoder` | 分别处理 new-api 的 `success/message/data` 与 sub2api 的 `code/message/data` |
| `Paginator` | new-api 使用 `p/page_size`，sub2api 使用 `page/page_size` |
| `SecretRedactor` | 请求/响应日志必须屏蔽 `password`、`access_token`、`refresh_token`、完整 API Key |
| `QuotaNormalizer` | new-api raw quota 转 USD，sub2api 直接使用 USD |
| `FeatureProbe` | 探测 `ratio_config`、`channels/available`、Key 是否脱敏、是否需要 Turnstile |

推荐同步流程：

1. new-api：先 `GET /api/status` 读取 `quota_per_unit` 和 Turnstile 状态，再登录；若出现 2FA 返回 `LoginChallenge`；登录后拉 `/api/user/self`、`/api/user/self/groups`、`/api/user/models`、`/api/ratio_config`，分页拉 `/api/token/` 后按 100 个一组调用 `/api/token/batch/keys` 补齐真实 Key。
2. sub2api：登录后保存 token pair；401 或即将过期时用 `/api/v1/auth/refresh`；拉 `/auth/me` 或 `/user/profile`、`/user/platform-quotas`、分页 `/keys`、`/groups/available`、`/groups/rates`、`/channels/available`，并用 `/usage/dashboard/stats` 补充累计已用。

### 自动优先级/权重建议

第一版建议规则：

1. 为每个密钥计算有效倍率摘要，优先使用该密钥所在分组下目标模型的最小倍率。
2. 按倍率升序分桶，最低倍率桶 `priority = 100`，后续桶依次递减，例如 `90`、`80`。
3. 同一倍率桶内 `weight = max(1, round(最低倍率 / 当前倍率 * 100))`，保证低倍率更多流量。
4. 倍率缺失时使用 `priority = 0`、`weight = 1`，并在 UI 标记“缺少倍率，需人工确认”。

该策略只生成建议值，不直接覆盖用户手动编辑。

## 风险评估

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| 目标平台接口差异或版本变化 | 同步失败或字段缺失 | 适配层按平台隔离，接口失败返回结构化错误和原始状态码摘要 |
| 账号密码处理不当 | 凭证泄露 | 账号密码只在后端请求生命周期中使用，不写日志、不落库、不回显 |
| 前端预览暴露完整密钥 | 安全风险 | 预览响应默认只返回脱敏 Key；创建动作使用后端临时会话或一次性确认 payload |
| `preview_id` 被重复提交 | 重复创建同一批渠道账号 | 创建接口消费式读取快照；Redis 模式使用 Lua 原子读取并删除，内存模式加同 ID 互斥 |
| 渠道创建半成品影响生产调度 | 请求错误 | 采用“预览后确认创建”；未补齐类型/模型前不落可用渠道，或落库为手动禁用 |
| 自动权重策略误导调度 | 成本或稳定性异常 | UI 明确展示倍率来源和建议值，保存前可逐条覆盖 |
| 修复降级改变现有重试语义 | 可能绕开亲和性或 specific channel 预期 | `specific_channel_id` 不自动降级；亲和性按配置决定是否跳过重试；普通自动选择才排除失败渠道 |
| 数据库迁移不兼容 | 三库部署失败 | 第一阶段尽量不新增字段；如需新增字段使用 GORM 抽象和 TEXT 类型 |
| 热更新缓存导致验证误判 | 页面或接口仍是旧版本 | 每次修改后访问 3003；页面未变化先重启容器再验证 |

## 方案评审结论

第一阶段不直接修改核心渠道表结构，先实现可验证的同步适配层和预览接口，再把确认创建落到现有 `Channel + ChannelAccount` 模型。这样能最大化复用当前渠道级账号池调度、账号级错误冷却、独立配置和审计权限，减少对 Relay 热路径的侵入。

渠道失效降级修复应独立于账号同步功能先完成，因为它影响现有核心业务稳定性。修复时只扩展“当前请求内排除失败渠道”的选择条件，不改变渠道权重和优先级的基础语义。

## 开发阶段计划

### 阶段 0：需求与代码现状确认

- [x] 创建本文档作为开发台账。
- [x] 确认当前新增渠道、渠道账号池和渠道选择链路的主要文件。
- [x] 完成 new-api/sub2api 本地接口初查，补充接口字段清单。
- [x] 使用真实测试平台确认接口响应、2FA/Turnstile 状态和完整 Key 读取权限边界：sub2api 可登录并返回账号、余额、分组和空密钥列表；new-api 当前测试账号被目标平台登录页与 NexusTok preview 同时拒绝，返回账号/密码错误或用户被禁用。
- [x] 提交需求分析文档。

### 阶段 1：修复渠道失效降级

- [x] 增加请求级失败渠道排除集合。
- [x] 改造缓存/数据库选择逻辑，排除本请求已失败渠道，并二次过滤渠道状态。
- [x] 保持 `specific_channel_id` 不降级。
- [x] 修复 setup 阶段无可用 key/账号时直接失败的问题，改为排除该渠道后继续选路。
- [x] 修复 multi-key key 匹配失败和全部 key 禁用后的缓存同步边界，避免 usingKey 为空或不匹配时误禁用第一个 Key，并在全部 Key 禁用后同步禁用渠道能力和缓存候选。
- [x] 补充后端单元测试和现有重试测试。
- [x] 验证 `go test ./model ./service ./controller`。

### 阶段 2：平台账号同步适配层

- [x] 新建平台客户端接口和 new-api/sub2api 实现。
- [x] 实现登录、密钥列表、分组倍率、余额快照归一化。
- [x] 增加预览接口和错误响应结构。
- [x] 补充适配层单元测试，使用 httptest 模拟目标平台。
- [x] 验证 `go test ./service/upstreamaccount ./controller ./router`。

### 阶段 3：创建/刷新渠道和渠道账号

- [x] 创建确认接口，把一个渠道和多条 `ChannelAccount` 写入同一事务。
- [x] 支持刷新时新增、更新、禁用缺失密钥的策略。
- [x] 写入 `balance`、`used_quota`、同步时间和同步来源元数据。
- [x] 补充 SQLite 路径下的模型/控制器测试，创建流程使用 GORM 事务保证三库兼容。
- [x] 账号同步创建时允许管理员先不选择类型；后端默认按 OpenAI 兼容渠道落库。
- [x] 账号同步创建时允许模型为空；若目标平台无法推断模型，则不写入空 `Ability`，避免空模型污染调度。
- [x] 为同步账号写入 `platform/base_url/external_id/key_digest/synced_at` 元数据，刷新时优先按元数据匹配，旧账号按 key digest 兜底。
- [x] 刷新已有渠道时保留 NexusTok 本地覆盖字段；只有请求显式提交的账号覆盖配置才会写入对应字段。
- [x] 创建成功后一次性消费 `preview_id`，避免同一个完整 Key 快照在 TTL 内重复创建相同渠道。

### 阶段 4：前端渠道表单

- [x] 新增账号同步模式和平台选择。
- [x] 新增预览结果表格，展示密钥、分组、建议优先级/权重和余额摘要。
- [x] 支持自动应用建议和手动逐条编辑。
- [x] 放宽账号同步模式下模型的前端校验；普通渠道仍保持模型必填。
- [x] 新增 i18n 翻译并运行 `bun run i18n:sync`。
- [x] 在编辑已有同步渠道时提供刷新入口，要求重新输入上游账号密码，不保存平台密码。

### 阶段 5：真实环境验证

- [x] 访问 `http://192.168.0.202:3003/` 登录并确认页面更新。
- [x] 使用 MCP 浏览器验证新增渠道页面账号同步流程。
- [x] 调用 preview/create/refresh 接口验证请求体、响应体、错误提示和权限。
- [x] 使用真实 sub2api 测试账号触发 preview；当前账号返回 0 个密钥，页面展示“未找到此账号的上游密钥。”并禁止基于空密钥快照继续创建。
- [x] 使用真实 new-api 测试地址触发 preview，并在目标 new-api 登录页复核；当前测试账号被目标平台拒绝，preview 正确透传“账号/密码错误或用户被禁用”，因此尚无法完成 new-api 成功读取密钥的正向验证。
- [x] 用 relay 请求级测试验证渠道失败后按优先级/权重降级：`TestRelayRetriesToNextChannelAfterChannelFailure` 使用两个 OpenAI 兼容 httptest 上游，首个高优先级渠道返回 401 后，同一次请求排除失败渠道并降级到第二渠道成功返回。
- [ ] 页面未更新时重启容器后重新验证。

## 验证清单

后端：

- `go test ./model ./service ./controller ./middleware`
- 针对新增适配层执行包级测试。
- 验证 SQLite 默认测试通过；若有原始 SQL，必须审查 MySQL/PostgreSQL 兼容性。

前端：

- `cd web/default && bun run build`
- `cd web/default && bun run i18n:sync`
- MCP 浏览器打开 3003 页面验证渠道新增/刷新主流程。

人工联调：

- new-api 登录成功，能读取密钥、分组倍率、余额。（当前测试账号被目标平台拒绝，需可登录账号后补正向验证。）
- sub2api 登录成功，能读取密钥、分组倍率、余额。
- 同步创建后，一个渠道下生成多条渠道账号，且每个账号可独立编辑。
- 自动策略生成的优先级/权重可被手动覆盖。
- 失败渠道不会在同一次请求重试中再次命中，且可降到下一优先级。

## 当前未决问题

1. new-api 正向联调受当前测试账号状态阻塞；目标 new-api 登录页与 NexusTok preview 均返回账号/密码错误或用户被禁用，需要可登录账号后再验证密钥、分组倍率和余额的成功快照。
2. 是否允许保存平台账号密码用于后台定时刷新尚未确定。第一版默认不保存，刷新时重新输入。
3. 预览接口已通过后端短期缓存保存完整 Key，前端只接收脱敏快照；确认创建时引用一次性 `preview_id`。
4. 目标平台的“模型倍率”可能是系统级配置而非密钥级配置，需要确认密钥和分组的关联关系后再计算建议策略。
5. 平台账号没有任何密钥时，创建流程阻止落库；前端预览页展示无密钥提示并禁用基于建议倍率的配置入口。
6. 同步身份当前写入账号 `settings` 文本，已避免新增数据库字段；若后续需要按外部 key id 查询或批量统计，再评估新增三库兼容字段。
