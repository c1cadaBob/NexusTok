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
| `conversion_ratio` | 当前密钥倍率 |
| `weight` | 自动或覆盖后的有效权重 |
| `weight_override` | 预留的显式覆盖值 |
| `used_quota` | 已用额度 |
| `remain_quota` | 剩余额度 |
| `expires_at` | 过期时间 |
| `status` | 启用、手动禁用、自动禁用、缺失 |
| `disabled_reason` | 禁用原因 |
| `last_sync_at` | 最近同步时间 |
| `missing_since` | 最近一次完整同步中未出现的时间 |

唯一约束为 `channel_id + external_id`。同步只使用完整分页成功的数据处理缺失密钥，部分失败不得清理现有密钥。

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
    Authenticate(ctx context.Context, credential PlatformCredential) (PlatformSession, error)
    FetchSiteInfo(ctx context.Context, session PlatformSession) (SiteInfo, error)
    FetchKeys(ctx context.Context, session PlatformSession, page Page) (KeyPage, error)
    FetchKeyDetail(ctx context.Context, session PlatformSession, key ExternalKey) (KeyDetail, error)
    FetchModels(ctx context.Context, session PlatformSession, key ExternalKey) ([]string, error)
}
```

统一认证类型：

- 账号密码；
- 访问令牌；
- Admin Key；
- Cookie。

NewAPI 首期协议范围：

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
- 修改转换倍率；
- 设置或清除官方密钥权重覆盖；
- 批量启用和禁用子密钥。

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

实现和测试按照 OWASP ASVS 5.0.0 的认证、会话、授权、密码学、安全通信、配置和安全日志要求执行。

## 8. 前端交互

渠道页面更名为“上游渠道”，表单第一项选择：

- 平台站点；
- 密钥渠道。

平台站点只显示 NewAPI 和 Sub2API；密钥渠道显示现有官方平台类型。

平台站点表单包含：

- 站点地址；
- 认证方式；
- 用户名、密码、访问令牌、Admin Key 或 Cookie；
- 充值金额和到账金额；
- 转换倍率预览；
- 手动同步和同步状态。

列表使用父行和子行：

- 父渠道默认显示；
- 平台站点子密钥默认折叠；
- 支持搜索、模型筛选、状态筛选和批量操作；
- 子行显示脱敏名称、模型数、状态、优先级、倍率、自动权重、额度状态、过期状态和同步时间；
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
