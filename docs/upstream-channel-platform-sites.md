# 上游渠道与平台站点设计

> 文档状态：代码事实基线
> 事实基线日期：2026-09-25
> 主要代码来源：`model/upstream_channel.go`、`model/routing_key.go`、`service/upstream_site.go`、`controller/upstream_channel.go`、`controller/channel-test.go`
> 关联架构文档：[`docs/architecture/relay-routing-and-conversion.md`](architecture/relay-routing-and-conversion.md)、[`docs/architecture/provider-capability-matrix.md`](architecture/provider-capability-matrix.md)、[`docs/architecture/data-cache-and-background-jobs.md`](architecture/data-cache-and-background-jobs.md)

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

平台站点地址支持 `http://` 和 `https://`，也支持 `localhost`、回环地址、
RFC1918 私有 IPv4、私有 IPv6 以及内网 DNS 名称。该放宽只作用于平台站点专用
同步客户端；普通下载、Webhook、视频代理和其他用户可控 URL 仍使用全局 SSRF
策略。平台同步客户端保留超时、响应大小、重定向次数和每次重定向校验。

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
| `relay_base_url` | Sub2API 页面发现的实际 OpenAI 兼容转发地址；管理接口仍使用 `base_url` |
| `auth_type` | 持久化为 `password`、`access_token`、`admin_key`、`cookie`；前端采集阶段另有临时值 `auto` |
| `credential_ciphertext` | 加密后的凭据对象 |
| `credential_key_version` | 凭据加密密钥版本 |
| `credential_fingerprint` | 不可逆凭据指纹 |
| `recharge_amount` | 充值金额 |
| `credited_amount` | 实际到账金额 |
| `conversion_ratio` | 充值金额除以到账金额 |
| `balance` | 站点共享余额 |
| `used_quota` | 站点已用额度 |
| `last_sync_at` | 最近一次成功同步时间；失败或运行状态不会覆盖该时间 |
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
| `models_synced` | 是否已经确认该子密钥的实际模型能力；为 `false` 时不可路由 |
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

平台站点快照的可用性由父渠道状态和同步状态共同决定：

- 父渠道被管理员禁用时永远不可路由；
- `success` 可使用当前快照；
- `failed` 或 `running` 且 `last_sync_at > 0` 时继续使用最近一次成功快照；
- `idle` 或从未成功同步的 `failed`/`running` 不可路由。

`disabled_at` 和 `disabled_reason` 不再因为一次自动同步失败而阻断已有成功快照；
管理员对父渠道的禁用仍以 `channels.status` 为准。

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

平台站点能力记录只保存上游密钥实际返回或使用该密钥访问 `/v1/models`
确认的模型，`group` 固定为空字符串。平台站点父渠道的 `channels.group`
仍然是下游用户组限制；上游 token 的分组只参与倍率解析和诊断，不覆盖
下游用户组，也不与下游用户组直接比较。平台同步不会调用普通
`channel.AddAbilities`，因此不会把父渠道模型和分组生成笛卡尔积。

父渠道 `channels.models` 只保存所有 `models_synced=true` 且具有有效能力的
子密钥模型去重并集，用于管理端展示、搜索和路由索引；它不是单个子密钥的
能力来源。没有确认实际模型能力的子密钥保留在数据库中但不可路由。

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

统一快照中的每个 `UpstreamKeySnapshot` 必须单独携带：

- `Models`：该子密钥的真实模型能力；
- `ModelsSynced`：模型能力是否确认成功；
- `SyncError`：该子密钥的脱敏同步错误类别。

站点级 `/api/user/models`、分组模型列表或 `/v1/models` 只能作为账号诊断
或单个子密钥能力的备用来源，不能把全局模型列表复制给所有子密钥。

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

部分 NewAPI 派生站点登录成功后不会返回访问令牌，而是依赖 Cookie 会话和
用户 ID 兼容请求头访问 `/api/user/self`、`/api/token/` 等接口。密码登录
响应中如果能解析到 `id`、`user_id` 或 `userId`，适配器会把同一用户 ID
写入 `New-API-User`、`X-ModelFlare-User`、`Veloera-User`、`X-Api-User`、
`voapi-user`、`User-id`、`Rix-Api-User` 和 `neo-api-user`，用于兼容 NewAPI、
ModelFlare、Veloera、V-API、VoAPI、Super-API、Rix-Api 和 Neo-API 派生站点。

Sub2API 首期协议范围：

- `/api/v1/auth/login`
- `/api/auth/login`
- `/auth/login`
- `/api/v1/auth/me`
- `/api/v1/user/profile`
- `/api/v1/auth/refresh`
- `/api/v1/groups/available`
- `/api/v1/groups/rates`
- `/api/v1/keys`
- `/api/v1/keys/{id}`
- `/api/v1/admin/accounts`
- `/api/v1/admin/accounts/{id}`
- `/api/v1/admin/accounts/data`
- `/v1/models`

Sub2API 密码登录和后续管理接口要求上游返回 JSON。登录请求收到 `2xx` 但
响应体是 HTML、纯文本或非法 JSON 时，代码仍归类为 `ErrSub2APILoginResponse`，
不会把网页内容当成模型响应或凭据。管理员同步状态和系统日志会记录安全诊断摘要：
HTTP 状态码、脱敏后的最终协议/主机/路径、Content-Type、是否发生重定向，以及
`html`、`plain_text`、`invalid_json` 或 `empty` 等响应类别。查询参数、响应体、密码、
Cookie、Access Token、Refresh Token 和 Admin Key 不会写入诊断。

因此“Sub2API 登录响应格式错误”通常表示管理端 URL 命中了网页登录页、Cloudflare/
WAF/Turnstile 验证页、反向代理文本错误，或登录 API 路径发生变化，不是下游模型
输出格式错误。处理顺序是检查管理端 URL、反向代理和 API 路径；如果需要交互验证，
使用已有平台站点浏览器采集能力获取 Access Token 或 Cookie。后台同步不尝试绕过
验证码、Turnstile、Cloudflare 或其他上游安全策略。

适配器不能绕过验证码、交互式二次验证或站点风控。密码登录无法完成时返回可识别的认证状态，管理员可以切换为 Cookie 或令牌认证。

平台站点表单提供两种认证方式：`password` 和 `auto`（自动配置）。账号密码认证
由管理员手动输入用户名和密码；自动配置通过短期 Capture Session 和目标站浏览器
脚本自动判断并采集 Access Token、Admin Key 或 Cookie。采集会话绑定管理员、目标
Origin、平台、认证类型和可选渠道，默认有效期 10 分钟，完成保存后消费，结果只在
短期缓存中传递，最终仍使用现有整体加密字段保存。`auto` 只存在于表单和采集会话
期间，成功后按实际采集结果持久化为 `access_token`、`admin_key` 或 `cookie`。

浏览器脚本只读取明确命名的 Access Token、Refresh Token、Admin Key、Cookie 和
用户对象。Sub2API 的 `auth_token`/`refresh_token` 会在临近过期且存在刷新令牌时
尝试刷新，并用 `/api/v1/auth/me` 或兼容接口确认登录态；NewAPI 会读取数字用户
ID，用于需要兼容用户请求头的派生站点。Admin Key 或 HttpOnly Cookie 无法被脚本
读取时直接跳过该候选；自动配置按 Access Token/Refresh Token、Admin Key、Cookie
的顺序选择首个可用凭据。三者都不可用时失败，不手动补填、不把一种凭据冒充成
另一种认证方式，也不保存混合认证结果。前端只提交 `capture_id`，不提交原始令牌、
Cookie 或 Admin Key。

Sub2API 管理地址和转发地址分离处理：账号、分组和密钥接口始终使用
`base_url` 指向的管理站地址；页面配置中的 `api_base_url` 用于发现 OpenAI
兼容转发地址。`api_base_url` 可以是绝对 URL，也可以是相对路径。相对路径会按
同源解析，并继续执行 SSRF、重定向和关联主机校验。发现到的转发地址保存到
`relay_base_url`，写入渠道 `base_url` 前会移除结尾 `/v1`，避免转发时形成
`/v1/v1/...`。

如果管理员保存的是以 `api.` 开头的 Sub2API 转发地址，首次同步只会匿名探测去掉
首个 `api.` 标签后的同协议、同端口主机。只有该候选站点返回 HTML，且页面配置中的
`api_base_url` 明确指向原始 `api.` 地址时，系统才会自动将候选地址纠正为管理地址；
否则保留原地址，不向未经验证的候选地址发送凭据。成功纠正后，`base_url` 保存管理
地址，`relay_base_url` 和父渠道 `base_url` 保存原始转发地址。

管理地址纠正的候选探测始终使用不带 Authorization、Cookie 或用户凭据的匿名请求。
只有候选站点返回 `2xx` HTML/XHTML，且页面配置中的 `api_base_url` 明确回指原始
API 地址的同协议、同主机和同端口来源时，才允许切换；验证失败时保留原地址。

`PlatformSiteCredential.auth_type` 是非敏感的认证方式标识，保存后固定使用该分支：

- `password` 每次同步重新使用账号密码登录，登录返回的会话令牌只用于当前同步；
- `access_token` 只使用访问令牌和刷新令牌，刷新失败不回退账号密码；
- `admin_key` 只使用 Admin Key；
- `cookie` 只使用 Cookie。

账号密码模式不会因为登录响应包含刷新令牌而改写成令牌模式。更新凭据时，
服务端只保存当前认证方式需要的字段，避免残留字段再次触发认证方式漂移。

只有 `access_token` 模式会把刷新接口返回的新访问令牌、刷新令牌和过期时间写入
`PlatformSiteSession.CredentialUpdate`。宿主同步流程只在完整快照成功后加密保存这组
旋转后的凭据；密码模式不会持久化登录响应中的令牌，不把明文密码写入日志或响应。

Sub2API 普通用户接口如果只能返回掩码密钥，不会伪造真实密钥。无法取得真实密钥的新记录会自动禁用；已有密钥保留旧密文并继续按同步状态过滤。

## 5. 同步任务

- 保存平台站点后排队一次初始同步。
- 默认每 15 分钟同步。
- 同一站点使用互斥锁。
- 支持管理员手动同步。
- 分页全部成功后再开启数据库事务并 upsert。
- 同步开始只切换为 `running`，失败时只记录 `failed`、脱敏错误和连续失败次数；
  不删除旧密钥、不清理旧模型能力、不标记旧密钥 `missing`、不覆盖旧余额，
  并继续复用最近一次成功快照。
- 单个子密钥的模型能力获取失败时保留最近一次模型快照，但设置
  `models_synced=false` 并自动禁用该子密钥；新密钥没有真实能力时只创建为
  不可路由记录。
- 完整同步后未出现的密钥标记 `missing` 并自动过滤，不立即删除。
- 过期、剩余额度小于等于零、平台禁用和长期未同步的密钥自动过滤。
- 父站点认证失败只影响父站点；单个子密钥错误只禁用子密钥。
- 管理员倍率覆盖和权重覆盖在后续同步中保留；清除覆盖后才恢复自动计算。
- 免费倍率 `0` 的密钥权重固定为 `2000`，同步和 PATCH 都会清除不适用的权重覆盖。
- 同步成功后使路由候选和模型能力缓存失效。
- 同步完成后以成功确认的子密钥模型生成父渠道模型并集，并更新父渠道余额、
  已用额度和 `balance_updated_time`。同步不修改父渠道优先级、分组、模型映射、
  子密钥优先级、倍率覆盖或权重覆盖。
- 父渠道已用额度优先采用站点账号或用量接口明确返回的值；明确返回 `0` 也视为
  有效结果，不使用旧快照覆盖。若本次完整同步没有任何账号级已用额度字段，则
  将本次快照中明确返回已用额度的子密钥值求和，作为父渠道和站点账号的回退值。

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
- 单独启用和禁用子密钥。`batch-status` 接口保留为后端兼容入口，渠道页面不再提供多选或批量启停操作。

`PATCH /api/channel/:id/upstream-keys/:keyId` 的字段语义：

| 字段 | 语义 |
| --- | --- |
| `key_priority` | 更新密钥优先级 |
| `conversion_ratio` | 保存管理员最终倍率覆盖，并重算自动权重 |
| `clear_conversion_ratio` | 清除管理员倍率覆盖，恢复“站点倍率 × 上游原始倍率” |
| `weight_override` | 保存管理员权重覆盖 |
| `clear_weight` | 清除管理员权重覆盖，恢复自动权重 |

不允许同时提交 `conversion_ratio` 和 `clear_conversion_ratio`。免费倍率密钥不允许设置权重覆盖；如果提交 `clear_weight` 或清除倍率覆盖后恢复为免费倍率，后端会自动清除不适用的权重覆盖并将权重固定为 `2000`。

`GET /api/channel/:id/upstream-keys` 只返回管理员管理所需的脱敏字段：密钥 ID、父渠道 ID、外部 ID、名称、脱敏预览、模型列表、模型能力同步状态、密钥优先级、上游原始倍率、生效倍率、倍率覆盖、有效权重、自动权重、权重覆盖、状态、禁用摘要和同步时间。真实密钥、剩余额度、过期时间和最近使用时间不返回；这些字段只保留在后端模型中参与路由过滤。管理端模型列成功同步时只展示模型列表；模型尚未同步时显示不可用提示，不额外显示 `Synced`。

`GET /api/channel/update_balance/:id` 对平台站点复用完整只读同步，保持顶层
`success` 和 `balance` 兼容，并额外返回：

```json
{
  "used_quota": 0,
  "balance_updated_time": 0,
  "sync_status": "success",
  "key_count": 0,
  "routable_key_count": 0
}
```

其中 `used_quota` 来自本次上游同步结果，并已同步保存到
`PlatformSiteAccount` 和父渠道；只有上游没有返回账号级已用额度时，才使用本次
同步中各子密钥已用额度的求和作为回退值。

失败时返回脱敏错误，保留最近一次成功余额、模型和密钥快照，不把上游响应正文
或凭据内容传递给前端。

`routable_key_count` 只统计父渠道启用、平台快照可用、子密钥状态可路由、
模型能力已确认且实际密钥能够解密的子密钥。无法解密的历史密钥、缺少模型能力
或仅保留展示快照的记录不会计入可路由数量。

所有接口必须经过管理员权限校验。敏感凭据查看必须经过现有安全验证机制，默认只返回指纹和掩码。

## 7. 安全要求

- 平台密码、Cookie、访问令牌、Admin Key、刷新令牌和上游 API Key 不能明文保存。
- 使用 AES-GCM 或项目现有等价的信封加密能力，包含 nonce、认证标签和密钥版本。
- 数据库只保存密文和不可逆指纹。
- `SESSION_SECRET` 优先于 `SESSION_SECRET_FILE`；未显式设置时从持久化文件读取或
  首次生成并以 `0600` 权限保存。`CRYPTO_SECRET` 优先于
  `CRYPTO_SECRET_FILE`，否则回退到会话密钥，保证容器重启后旧凭据可解密。
- 启动迁移会为能够正常解密的历史平台账号补齐固定认证方式
  `PlatformSiteAccount.auth_type`；无法解密的历史密文不会被当作明文、不会尝试猜测旧密钥，
  管理员需要按原认证方式重新保存一次凭据，重新使用当前稳定密钥加密。
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

2026-09-17 复核时通过 OWASP 官方项目页和 ASVS 仓库确认 ASVS 最新稳定版本
仍为 `5.0.0`，发布日期为 2025-05；安全核对范围仅覆盖本功能涉及的平台凭据
保存、管理员操作、外部站点请求边界、脱敏错误和审计日志，不宣称覆盖项目全部
认证/会话实现。

## 8. 前端交互

渠道页面更名为“上游渠道”，表单第一项选择：

- 平台站点；
- 密钥渠道。

平台站点表单包含：

- 站点地址；
- 认证方式；
- 账号密码认证下的用户名和密码；访问令牌、Admin Key 和 Cookie 通过浏览器脚本采集；
- 充值金额和到账金额；
- 转换倍率预览；
- 手动同步和同步状态。

点击自动配置入口后，管理端创建短期会话。前端会在点击事件同步阶段预开标签页，
再把创建成功的 handoff 地址导航到该标签页；如果浏览器仍拦截弹窗，则保留会话并
显示手动打开采集页按钮。Capture Helper 或一次性 userscript 在目标站页面内采集
登录态并通过一次性 secret 回传。状态查询只返回
认证类型、管理/转发地址、掩码 Access Token、Refresh Token/Admin Key/Cookie
是否存在和令牌过期时间，不返回原始敏感值。已有渠道采集完成后自动保存，新渠道
创建时携带 `capture_id`，只有保存成功后才消费会话。

列表使用父行和子行：

- 父渠道默认显示；
- 平台站点子密钥进入页面时默认折叠，展开状态不持久化；
- 支持搜索、模型筛选和状态筛选；
- 搜索覆盖父渠道名称、ID、密钥预览和模型名称；
- 子行显示脱敏名称、脱敏密钥预览、模型、状态、密钥优先级、生效倍率、覆盖标记、有效权重、权重覆盖标记和同步时间；
- 子行显示模型能力是否已同步、自动权重和最终权重的覆盖状态；
- 子行不展示真实密钥、过期时间、剩余额度或最近使用时间；
- 子密钥编辑弹窗展示生效倍率、上游原始倍率、倍率覆盖开关、最终倍率覆盖输入、密钥优先级和权重覆盖；
- 子密钥不支持多选和批量启停，保留每行单独启用/停用以及其他单密钥操作；
- 平台渠道状态列直接显示最近同步的相对时间；悬停时间后查看成功、失败、运行中、快照回退、凭据不可用、连续失败次数、完整同步时间和错误详情；
- 密钥渠道不显示第二层；
- 平台站点不显示普通渠道的手动模型必填选择器。模型列表只读展示同步后
  的子密钥模型并集；模型映射仍然可以把下游模型名映射到上游模型名，
  下游用户组仍然由父渠道分组字段限制；
- 复用现有 DataTable、Dialog、ConfirmDialog 和 CopyButton；
- 所有文案通过 `useTranslation()` 和 `t(...)` 提供七语言翻译。

## 9. 测试与验证

单元测试覆盖：

- 倍率和权重边界；
- NewAPI/Sub2API 三类认证；
- 分页、upsert、幂等和缺失密钥处理；
- 模型、余额、过期和状态过滤；
- `models_synced=false` 的子密钥不可路由，站点级模型不得回退分配；
- 渠道优先级、密钥优先级和权重选择；
- 子密钥错误不误伤父站点；
- 成功快照后的认证失败、凭据解密失败和站点接口失败继续路由旧快照；
- 密码、令牌、Cookie 和 Admin Key 模式不会互相漂移；
- 会话/加密密钥文件重启后仍能解密旧平台凭据；
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

并使用 MCP/Playwright 检查桌面端和移动端的折叠、搜索、表单切换、单密钥操作、无重叠和无溢出。

真实站点测试只从本地环境变量读取凭据，执行登录、会话刷新、站点信息、余额、密钥和模型的只读请求。不得执行充值、删除密钥、修改上游配置或其他破坏性操作。测试结果不得记录密码、Cookie、令牌或实际密钥。

模型能力验收至少覆盖：一个站点包含两个子密钥且模型集合不同，确认父渠道
模型是两者并集，而每个子密钥只允许路由到自己的模型；站点全局模型列表
即使包含更多模型，也不能被复制给任意子密钥。

当前 v1 验证记录应在最终交付说明中填写：

- Go：`go test ./model ./service ./controller`；
- 前端：`bun run test -- src/features/channels/components/__tests__/upstream-keys-subtable.test.tsx`、`bun run test -- src/features/channels/lib/__tests__/channel-table-row-id.test.ts src/features/channels/lib/__tests__/new-api-channel.test.ts`、`bun run typecheck`、涉及文件 `oxlint`、`bun run i18n:sync`、`bun run build`；
- 数据库：SQLite、MySQL、PostgreSQL 的真实版本、迁移命令、连续两次迁移结果、upsert 和缺失密钥回滚结果；
- 真实站点：NewAPI 站点倍率 `0.1`、Sub2API 站点倍率 `1`，只读采集余额、密钥、模型和倍率，输出必须脱敏；
- 前端页面：MCP/Chrome DevTools 或 Playwright 桌面端、移动端检查结果。

### 9.1 2026-09-17 修复验证记录

本轮修复覆盖 NewAPI/Sub2API 平台站点同步凭据、认证方式固定、失败快照复用、
Sub2API 转发地址发现、平台/type 表单联动、测试弹窗选取真实可路由子密钥和
余额刷新可路由数量统计。

已完成的自动化验证：

```bash
go test -p 1 ./common ./model ./service ./controller ./relay/channel/sub2api -count=1
cd web && bun run test -- src/features/channels/lib/__tests__/new-api-channel.test.ts
cd web && bun run test -- src/features/channels/components/__tests__/upstream-keys-subtable.test.tsx src/features/channels/components/__tests__/balance-refresh.test.tsx src/features/channels/components/drawers/__tests__/platform-site-fields.test.tsx
cd web && bun run typecheck
cd web && bunx oxlint src/features/channels/components/drawers/channel-mutate-drawer.tsx src/features/channels/components/drawers/platform-site-fields.tsx src/features/channels/lib/channel-form.ts src/features/channels/lib/__tests__/new-api-channel.test.ts
```

说明：

- NewAPI 和 Sub2API 真实站点只读适配器测试通过本地临时环境变量执行，验证内容包含账号密码登录、余额、密钥分页、真实子密钥模型能力和站点倍率；测试不记录真实凭据、Cookie、Token 或实际密钥。
- `go test ./common ./model ./service ./controller ./relay/channel/sub2api -count=1` 在多包并行模式下曾受历史测试全局数据库状态互相干扰，controller 单包和 `-p 1` 串行模式通过。
- 当前仓库不存在 `src/features/channels/components/dialogs/__tests__/channel-test-dialog.test.tsx`，因此本轮未运行该文件；测试弹窗能力由后端 `controller/channel-test.go`、模型路由测试和现有前端交互检查继续覆盖。
- 三数据库平台站点兼容测试已通过：
  - SQLite：临时文件库；
  - MySQL：`mysql 8.2.0` 测试容器；
  - PostgreSQL：`PostgreSQL 15.19` 热环境容器；
  - 命令：
    `TEST_MYSQL_DSN=... TEST_POSTGRES_DSN=... go test ./model -run TestUpstreamChannelDatabaseCompatibility -count=1 -v`。
- 完整后端测试已通过：`go test -p 1 ./... -count=1`。首次在前端构建前运行根包测试时因 `web/dist/index.html` 尚不存在失败，执行 `cd web && bun run build` 后复跑通过。
- 开发容器已通过 `docker compose -f docker-compose.hot.yml up -d --build nexustok` 重建，`nexustok-api-hot` 健康检查为 `healthy`。
- MCP/Chrome 页面验收已完成：
  - 登录本地热环境后，在上游渠道页重新保存 NewAPI 和 Sub2API 的密码模式凭据，表单没有覆盖用户新输入的敏感字段；
  - NewAPI 保存后通过余额刷新触发完整只读同步，历史 `credential_unavailable` 子密钥恢复为真实可解密子密钥，子密钥模型能力、倍率和权重刷新成功，站点倍率为 `0.1`，自动权重示例包含 `1900`；
  - Sub2API 保存后通过余额刷新触发完整只读同步，余额刷新成功，子密钥恢复为可路由状态，站点倍率为 `1`，存在倍率为 `1` 的不可路由模型能力缺失子密钥展示为权重 `1000`；
  - 测试弹窗默认使用自动路由，并且模型列表来自可路由子密钥的真实模型能力；NewAPI 测试请求已路由到上游但因上游账号余额不足返回脱敏的上游错误，Sub2API 测试请求已路由到上游但目标模型返回上游临时不可用错误；
- 余额刷新按钮具备加载禁用状态，刷新成功后列表、父渠道状态和子密钥状态同步更新；
- 桌面端和移动端均无白屏，Chrome 控制台无 error，未观察到保存前请求风暴。

## 与架构文档的关系

本文保留平台站点、账号同步、子密钥字段、倍率/权重公式、权限、SSRF 和测试验收等详细规则；架构文档描述平台站点如何进入渠道过滤、Routing Key 和 Relay 转发。新增平台类型或调整同步/路由语义时，必须同时更新本文、能力矩阵和偏差表。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 补充架构索引与元信息 | 已有平台站点详细设计没有统一事实基线和架构入口 | 增加事实基线、代码来源、架构分工和变更记录；保留同步、倍率、权重和兼容性细节 | NewAPI、Sub2API、平台账号、上游密钥和路由 | `model/upstream_channel.go`、`model/routing_key.go`、`service/upstream_site.go` 静态核对 |
| 2026-09-25 | 缺陷修复 | Sub2API `2xx` 非 JSON 登录响应只显示笼统格式错误；平台子密钥 Routing Key 关系缺少运行时一致性说明 | 记录 HTML/纯文本/非法 JSON 的安全诊断字段和可操作排障方向；补充平台子密钥 Routing Key 自愈规则 | Sub2API 同步、管理员排障、平台子密钥路由和渠道测试 | `service/upstream_site.go`、`model/routing_key.go`、服务/模型回归测试 |
