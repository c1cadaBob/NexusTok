# 上游渠道与平台站点设计

## 1. 目标与范围

现有渠道统一称为上游渠道。上游渠道分为两类：

- **密钥渠道**：现有 OpenAI、Anthropic、AWS 等官方渠道。对外 API、现有密钥配置和转发协议保持兼容。
- **平台站点**：NewAPI、Sub2API，后续可扩展 AnyRouter、OneAPI。一个平台账号或一组凭据对应一个父渠道，父渠道下同步多个上游密钥。

首期平台站点需要同步：

- 站点账号信息、用户分组和共享余额；
- 上游密钥及其外部 ID、名称、指纹；
- 密钥实际值；
- 密钥倍率、已用额度、剩余额度和过期时间；
- 密钥支持的模型；
- 平台认证状态、同步时间和脱敏错误信息。

平台站点的同步结果只服务于管理员配置、后端可用性过滤和上游路由。普通用户不能看到真实上游平台、密钥、余额来源或当前请求使用的具体密钥。

## 2. 已确定的业务规则

### 2.1 优先级与权重

- 父渠道优先级数值越大越优先。
- 密钥优先级数值越大越优先，默认值为 `0`。
- 密钥权重数值越大越优先。
- 现有渠道权重字段保留用于 API 和数据兼容，但新路由不再使用，管理端隐藏且不允许编辑。
- 权重范围为 `[0, 2000]`。
- 转换倍率为 `充值金额 / 实际到账金额`。
- 转换倍率为 `0` 时表示免费密钥，权重固定为 `2000`。
- 权重 `0` 表示最低选择权重，不表示免费。

自动权重公式：

```text
weight = clamp(round(1000 + (1 - conversion_ratio) / 0.001), 0, 2000)
```

等价公式：

```text
weight = clamp(round(2000 - conversion_ratio * 1000), 0, 2000)
```

示例：

| 转换倍率 | 自动权重 |
| ---: | ---: |
| `0` | `2000` |
| `0.07` | `1930` |
| `0.1` | `1900` |
| `1` | `1000` |
| `2` | `0` |

平台站点子密钥的权重由倍率自动计算。官方密钥渠道默认倍率为 `1`、权重为 `1000`，管理员可以覆盖倍率或设置显式权重覆盖。

### 2.2 路由顺序

1. 按请求模型、分组、父渠道状态、子密钥状态、过期时间和剩余额度过滤。
2. 选择最大的父渠道优先级。
3. 将该优先级下不同父渠道的所有可用密钥合并为同一级候选。
4. 选择最大的密钥优先级。
5. 对剩余候选按密钥权重加权选择。
6. 所有候选权重为零时，使用随机或轮询兜底，避免路由因权重全零而失败。

密钥渠道在内部作为一个虚拟密钥候选处理，但管理界面不显示第二层。平台站点的余额视为账号级共享余额，不将每个子密钥额度重复累加。

## 3. 数据库设计

### 3.1 `channels`

保留现有 `channels` 表作为父渠道，新增或逐步引入：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `upstream_kind` | varchar | `key_channel` 或 `platform_site` |
| `key_priority` | bigint | 密钥渠道的虚拟密钥优先级，默认 `0` |
| `conversion_ratio` | decimal/float | 密钥渠道转换倍率，默认 `1` |
| `key_weight_override` | bigint nullable | 官方密钥渠道可选的权重覆盖 |

现有 `priority` 继续表示父渠道优先级。现有 `weight` 字段保留，但新路由忽略它。

旧渠道数据默认视为 `key_channel`。外部渠道 API 不要求客户端立即增加新字段。

### 3.2 `platform_site_accounts`

每个父平台渠道对应一条平台账号记录：

| 字段 | 说明 |
| --- | --- |
| `id` | 主键 |
| `channel_id` | 父渠道 ID，唯一 |
| `platform` | `newapi`、`sub2api` |
| `base_url` | 规范化站点地址 |
| `auth_type` | `password`、`access_token`、`admin_key`、`cookie` |
| `credential_ciphertext` | 加密后的凭据对象 |
| `credential_key_version` | 凭据加密密钥版本 |
| `credential_fingerprint` | 不可逆凭据指纹 |
| `recharge_amount` | 充值金额 |
| `credited_amount` | 实际到账金额 |
| `conversion_ratio` | 充值金额除以到账金额 |
| `balance` | 站点共享余额 |
| `used_quota` | 站点已用额度 |
| `last_sync_at` | 最近成功或失败同步时间 |
| `sync_status` | `idle`、`running`、`success`、`failed` |
| `last_sync_error` | 脱敏错误摘要 |
| `consecutive_failures` | 连续失败次数 |
| `disabled_at` | 站点禁用时间 |
| `disabled_reason` | 禁用原因 |

金额和倍率必须进行服务端校验。到账金额为零时拒绝保存，倍率不能为负数、NaN 或 Inf。

### 3.3 `upstream_keys`

| 字段 | 说明 |
| --- | --- |
| `id` | 主键 |
| `channel_id` | 父平台渠道 ID |
| `external_id` | 上游平台内密钥 ID |
| `name` | 上游密钥名称 |
| `secret_ciphertext` | 加密后的实际密钥 |
| `secret_fingerprint` | 密钥指纹 |
| `models` | 同步得到的模型列表 |
| `key_priority` | 密钥优先级 |
| `source_conversion_ratio` | 上游平台返回的密钥或分组原始倍率，空值按 `1` 处理 |
| `conversion_ratio` | 当前实际生效倍率 |
| `conversion_ratio_override` | 管理员设置的最终倍率覆盖，空值表示使用自动倍率 |
| `weight` | 按生效倍率计算出的自动权重 |
| `weight_override` | 管理员设置的权重覆盖，空值表示使用自动权重 |
| `used_quota` | 已用额度 |
| `remain_quota` | 剩余额度 |
| `expires_at` | 过期时间 |
| `status` | 启用、手动禁用、自动禁用、缺失 |
| `disabled_reason` | 禁用原因 |
| `last_sync_at` | 最近同步时间 |
| `missing_since` | 最近一次完整同步中未出现的时间 |

唯一约束为 `channel_id + external_id`。同步只使用完整分页成功的数据处理缺失密钥，部分失败不得清理现有密钥。

平台站点子密钥的自动倍率为：

```text
effective_conversion_ratio = platform_site_accounts.conversion_ratio * upstream_keys.source_conversion_ratio
```

当 `conversion_ratio_override` 非空时，`conversion_ratio` 直接使用管理员覆盖值，后续同步只更新 `source_conversion_ratio`、模型、额度和状态，不覆盖管理员设置。管理员清除覆盖后，系统重新按站点倍率和上游原始倍率计算 `conversion_ratio` 与 `weight`。

迁移规则：

- `source_conversion_ratio IS NULL` 的既有记录初始化为 `1`；
- 既有 `conversion_ratio` 保留为当前生效倍率；
- 迁移使用 `migration.upstream_key_defaults.v1` 标记，必须可重复执行；
- `AutoMigrate` 和显式默认迁移必须同时支持 SQLite、MySQL 与 PostgreSQL。

### 3.4 `upstream_key_abilities`

保存平台站点子密钥的模型能力：

| 字段 | 说明 |
| --- | --- |
| `upstream_key_id` | 上游密钥 ID |
| `group` | 分组 |
| `model` | 模型名称 |
| `enabled` | 是否可路由 |

唯一约束为 `upstream_key_id + group + model`。现有官方渠道继续使用 `abilities` 表。

## 4. 适配器设计

适配器只处理平台协议和字段转换，不处理数据库、计费或路由：

```go
type PlatformSiteAdapter interface {
    Platform() string
    Authenticate(ctx context.Context, baseURL string, credential model.PlatformSiteCredential) (*PlatformSiteSession, error)
    FetchSnapshot(ctx context.Context, session *PlatformSiteSession) (PlatformSiteSnapshot, error)
}
```

`FetchSnapshot` 在适配器内部完成站点信息、分页密钥、密钥详情和模型能力的协议聚合，宿主服务只接收统一的 `PlatformSiteSnapshot`，负责事务写入、状态处理、缓存刷新和路由。这样可以将平台接口差异限制在适配器内，避免数据库和路由逻辑泄漏到平台协议实现中。

统一认证类型：

- 账号密码；
- 访问令牌；
- Admin Key；
- Cookie。

NewAPI 首期协议范围：

- `/api/user/login`
- `/api/user/auth/refresh`
- `/api/user/self`
- `/api/user/self/groups`
- `/api/group`
- `/api/user/models`
- `/api/token/`
- `/api/token/{id}/key`
- `/api/channel/`
- `/api/channel/{id}`
- `/api/channel/fetch_models/{id}`
- `/api/pricing`

Sub2API 首期协议范围：

- `/api/v1/auth/login`
- `/api/v1/auth/me`
- `/api/v1/auth/refresh`
- `/api/v1/groups/available`
- `/api/v1/groups/rates`
- `/api/v1/keys`
- `/api/v1/admin/accounts`
- `/api/v1/admin/accounts/{id}`
- `/api/v1/admin/accounts/data`
- `/v1/models`

适配器不能绕过验证码、交互式二次验证或站点风控。密码登录无法完成时返回可识别的认证状态，管理员可以切换为 Cookie 或令牌认证。

适配器会把登录或刷新接口返回的新访问令牌、刷新令牌和过期时间写入 `PlatformSiteSession.CredentialUpdate`。宿主同步流程只在完整快照成功后加密保存这组旋转后的凭据；密码模式仍在每次同步时重新登录，不把明文密码写入日志或响应。

Sub2API 普通用户接口如果只能返回掩码密钥，不会伪造真实密钥。无法取得真实密钥的新记录会自动禁用；已有密钥保留旧密文并继续按同步状态过滤。

## 5. 同步任务

- 保存平台站点后排队一次初始同步。
- 默认每 15 分钟同步。
- 同一站点使用互斥锁。
- 支持管理员手动同步。
- 分页全部成功后再开启数据库事务并 upsert。
- 同步失败保留旧密钥、旧模型和旧余额。
- 完整同步后未出现的密钥标记 `missing` 并自动过滤，不立即删除。
- 过期、剩余额度小于等于零、平台禁用和长期未同步的密钥自动过滤。
- 父站点认证失败只影响父站点；单个子密钥错误只禁用子密钥。
- 管理员倍率覆盖和权重覆盖在后续同步中保留；清除覆盖后才恢复自动计算。
- 免费倍率 `0` 的密钥权重固定为 `2000`，同步和 PATCH 都会清除不适用的权重覆盖。
- 同步成功后使路由候选和模型能力缓存失效。

同步日志只记录站点 ID、平台、结果、数量、耗时和脱敏错误，不记录任何凭据或实际密钥。

## 6. API

现有对外渠道接口保持兼容。新增管理员接口：

```text
POST  /api/channel/:id/upstream-sync
GET   /api/channel/:id/upstream-sync
GET   /api/channel/:id/upstream-keys
PATCH /api/channel/:id/upstream-keys/:keyId
POST  /api/channel/:id/upstream-keys/batch-status
```

接口能力：

- 创建、修改和删除平台站点配置；
- 手动同步和查询同步状态；
- 查询脱敏子密钥；
- 修改密钥优先级；
- 设置或清除最终转换倍率覆盖；
- 设置或清除平台子密钥权重覆盖；
- 批量启用和禁用子密钥。

`PATCH /api/channel/:id/upstream-keys/:keyId` 的字段语义：

| 字段 | 语义 |
| --- | --- |
| `key_priority` | 更新密钥优先级 |
| `conversion_ratio` | 保存管理员最终倍率覆盖，并重算自动权重 |
| `clear_conversion_ratio` | 清除管理员倍率覆盖，恢复“站点倍率 × 上游原始倍率” |
| `weight_override` | 保存管理员权重覆盖 |
| `clear_weight` | 清除管理员权重覆盖，恢复自动权重 |

不允许同时提交 `conversion_ratio` 和 `clear_conversion_ratio`。免费倍率密钥不允许设置权重覆盖；如果提交 `clear_weight` 或清除倍率覆盖后恢复为免费倍率，后端会自动清除不适用的权重覆盖并将权重固定为 `2000`。

`GET /api/channel/:id/upstream-keys` 只返回管理员管理所需的脱敏字段：密钥 ID、父渠道 ID、外部 ID、名称、脱敏预览、模型列表、密钥优先级、上游原始倍率、生效倍率、倍率覆盖、有效权重、权重覆盖、状态、禁用摘要和同步时间。真实密钥、剩余额度、过期时间和最近使用时间不返回；这些字段只保留在后端模型中参与路由过滤。

所有接口必须经过管理员权限校验。敏感凭据查看必须经过现有安全验证机制，默认只返回指纹和掩码。

## 7. 安全要求

- 平台密码、Cookie、访问令牌、Admin Key、刷新令牌和上游 API Key 不能明文保存。
- 使用 AES-GCM 或项目现有等价的信封加密能力，包含 nonce、认证标签和密钥版本。
- 数据库只保存密文和不可逆指纹。
- 支持加密密钥版本轮换。
- 审计事件不能包含密码、Cookie、令牌、API Key、验证码、TOTP、私钥或可使用的会话信息。
- 站点 URL 必须进行 SSRF、DNS rebinding、内网地址和恶意重定向防护。
- 普通用户响应、普通用户日志和错误信息不暴露上游站点与密钥。
- 敏感查看、凭据替换、同步、禁用和倍率修改都要记录管理员审计事件。

实现和测试按照 OWASP ASVS 5.0.0（2025-05 最新稳定版）的认证、会话、授权、密码学、安全通信、配置和安全日志要求执行。适用控制项记录为：

- `v5.0.0-3.*`：会话令牌、刷新令牌、Cookie 与凭据生命周期；
- `v5.0.0-4.*`：管理员接口授权、渠道权限和敏感操作保护；
- `v5.0.0-6.*`：平台凭据、刷新令牌和上游 API Key 加密保存；
- `v5.0.0-9.*`：站点 URL、HTTPS、重定向与 SSRF 防护；
- `v5.0.0-10.*`：响应大小、依赖与配置安全边界；
- `v5.0.0-14.*`：管理员审计日志脱敏和安全事件可追踪。

同时参考 OWASP Authentication、Session Management、Password Storage、OAuth、CSRF、Logging、Cryptographic Storage 和 Server Side Request Forgery Prevention Cheat Sheet。审计事件只记录站点 ID、渠道 ID、平台类型、操作类型、结果、操作者、请求 ID 和脱敏错误，不记录密码、Cookie、令牌、Admin Key、刷新令牌或实际密钥。

## 8. 前端交互

渠道页面更名为“上游渠道”，表单第一项选择：

- 平台站点；
- 密钥渠道。

平台站点表单包含：

- 站点地址；
- 认证方式；
- 用户名、密码、访问令牌、Admin Key 或 Cookie；
- 充值金额和到账金额；
- 转换倍率预览；
- 手动同步和同步状态。

列表使用父行和子行：

- 父渠道默认显示；
- 平台站点子密钥默认折叠，展开状态按父渠道 ID 持久化；
- 支持搜索、模型筛选、状态筛选和批量操作；
- 搜索覆盖父渠道名称、ID、密钥预览和模型名称；
- 子行显示脱敏名称、脱敏密钥预览、模型、状态、密钥优先级、生效倍率、覆盖标记、有效权重、权重覆盖标记和同步时间；
- 子行不展示真实密钥、过期时间、剩余额度或最近使用时间；
- 子密钥编辑弹窗展示生效倍率、上游原始倍率、倍率覆盖开关、最终倍率覆盖输入、密钥优先级和权重覆盖；
- 子密钥支持选择、全选当前可见项、批量启用、批量禁用和清空选择；
- 密钥渠道不显示第二层；
- 复用现有 DataTable、Dialog、ConfirmDialog 和 CopyButton；
- 所有文案通过 `useTranslation()` 和 `t(...)` 提供七语言翻译。

## 9. 测试与验证

单元测试覆盖：

- 倍率和权重边界；
- NewAPI/Sub2API 三类认证；
- 分页、upsert、幂等和缺失密钥处理；
- 模型、余额、过期和状态过滤；
- 渠道优先级、密钥优先级和权重选择；
- 子密钥错误不误伤父站点；
- 凭据加密和审计脱敏；
- SSRF 和重定向防护。

数据库必须分别验证 SQLite、MySQL 和 PostgreSQL，包括全新迁移、旧库升级、连续两次启动迁移、索引约束、事务和同步 upsert。

前端必须执行：

```text
bun run i18n:sync
bun run typecheck
bun run lint
bun run build
```

并使用 MCP/Playwright 检查桌面端和移动端的折叠、搜索、表单切换、批量操作、无重叠和无溢出。

真实站点测试只从本地环境变量读取凭据，执行登录、会话刷新、站点信息、余额、密钥和模型的只读请求。不得执行充值、删除密钥、修改上游配置或其他破坏性操作。测试结果不得记录密码、Cookie、令牌或实际密钥。

当前 v1 验证记录应在最终交付说明中填写：

- Go：`go test ./model ./service ./controller`；
- 前端：`bun run test -- src/features/channels/components/__tests__/upstream-keys-subtable.test.tsx`、`bun run test -- src/features/channels/lib/__tests__/channel-table-row-id.test.ts src/features/channels/lib/__tests__/new-api-channel.test.ts`、`bun run typecheck`、涉及文件 `oxlint`、`bun run i18n:sync`、`bun run build`；
- 数据库：SQLite、MySQL、PostgreSQL 的真实版本、迁移命令、连续两次迁移结果、upsert 和缺失密钥回滚结果；
- 真实站点：NewAPI 站点倍率 `0.1`、Sub2API 站点倍率 `1`，只读采集余额、密钥、模型和倍率，输出必须脱敏；
- 前端页面：MCP/Chrome DevTools 或 Playwright 桌面端、移动端检查结果。
