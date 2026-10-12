# 上游渠道与平台站点设计

> 文档状态：代码事实基线
> 事实基线日期：2026-10-05
> 主要代码来源：`model/upstream_channel.go`、`model/routing_key.go`、`service/upstream_site.go`、`controller/upstream_channel.go`、`controller/channel-test.go`
> 关联架构文档：[`docs/architecture/relay-routing-and-conversion.md`](architecture/relay-routing-and-conversion.md)、[`docs/architecture/provider-capability-matrix.md`](architecture/provider-capability-matrix.md)、[`docs/architecture/data-cache-and-background-jobs.md`](architecture/data-cache-and-background-jobs.md)、[`平台站点资源获取比较`](platform-site-resource-acquisition-comparison.md)

## 1. 目标与范围

### 1.1 参考源与实现核对

涉及 NewAPI、Sub2API 平台站点的认证、会话刷新、资源获取、密钥同步、模型能力、
端点发现和管理端交互时，必须以本机参考源的实际路由、请求参数、响应结构、权限
中间件和失败语义为准：

- Sub2API：`/opt/project/sub2api-main`
- New API：`/opt/project/new-api-main`
- all-api-hub：`/opt/project/all-api-hub-main`

用户最初将 New API 路径重复写成 Sub2API 路径；本机实际 New API 源码为
`/opt/project/new-api-main`。参考源中的凭据、Cookie、Token、测试账号和环境变量
不得复制到 NexusTok。

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

适配器不能绕过验证码、交互式二次验证或站点风控。密码登录无法完成时返回可识别的认证状态。
后端仍兼容读取历史 Cookie 或令牌认证渠道，但新版渠道表单不再提供这些凭据的手动录入
或替换入口。

账号密码登录的交互式二次验证不再作为长期认证类型。服务端以一次性短期
`auth_flow_id` 承载待验证状态，绑定管理员、平台、规范化 Origin、管理地址和可选
渠道 ID，默认 TTL 为 5 分钟；密码、TOTP 和临时上游会话只保存在短期服务端缓存中，
流程成功、取消、过期、失败或重复消费后均不可继续使用。New API 的
`/api/user/login/2fa`、`/api/user/login/verify` 和 Sub2API 的
`/api/v1/auth/login/2fa` 均由适配器按参考源响应结构处理，前端只获得短期流程 ID。
New API Passkey、浏览器安全证明和敏感操作安全证明继续通过浏览器自动配置或人工
浏览器验证，不在 NexusTok 中重新实现 WebAuthn。

New API 会话刷新同时识别传统 Access Token/Refresh Token 和 Dashboard Auth Bundle。
可识别的现代 Bundle 必须完整包含 `success`、`data.access_token`、
`data.token_type`、`data.access_expires_at`、当前 `data.session.sid` 和
`data.user`；结构不完整时标记为需要重新认证，不降级为旧协议。Sub2API Refresh
Token 采用轮换语义，响应必须同时返回新的 Access Token、新 Refresh Token 和正数
`expires_in`；响应不完整或网络结果不确定时不重放旧 Refresh Token，并保留已有资源快照。

平台站点表单提供两种认证方式：`password` 和 `auto`（自动配置）。账号密码认证
由管理员手动输入用户名和密码；自动配置通过短期 Capture Session 和目标站浏览器
脚本自动判断并采集 Access Token、Admin Key 或 Cookie。采集会话绑定管理员、目标
Origin、平台、认证类型和可选渠道，默认有效期 10 分钟，完成保存后消费，结果只在
短期缓存中传递，最终仍使用现有整体加密字段保存。`auto` 只存在于表单和采集会话
期间，成功后按实际采集结果持久化为 `access_token`、`admin_key` 或 `cookie`。

2026-10-03 登录兼容修复不改变上述凭据加密边界。New API 密码登录主请求使用
`username/password`；邮箱输入的兼容主体仅在明确凭据错误或 HTTP 401 后尝试。Sub2API
邮箱登录首请求严格使用 `email/password`，仅在明确凭据错误或 HTTP 401 后最多一次
尝试 `username/password`，不再发送混合主体。Sub2API 首路由
`/api/v1/auth/login` 只有明确返回 404/405 时才回退 `/auth/login`；400
`INVALID_REQUEST`、403、429、WAF、安全验证、权限、网络和 5xx 都不触发路由或主体
回退。只有上游明确返回条款要求时才读取公开设置并携带 `agreed_revision`。

浏览器脚本只读取明确命名的 Access Token、Refresh Token、Admin Key、Cookie 和
用户对象。NewAPI 优先读取当前 Dashboard 登录态和明确的 Access Token，再尝试
Admin Key、Cookie；Sub2API 优先读取 `auth_token`、`auth_user`、Refresh Token 和
浏览器 Session Restore，再尝试其它可验证登录态。候选在提交前通过
`/api/user/self`、`/api/v1/auth/me` 或 Admin 管理权限接口验证；Admin Key 或
HttpOnly Cookie 无法被脚本读取时直接跳过。三者都不可用时失败，不手动补填、不把一种
凭据冒充成另一种认证方式，也不保存混合认证结果。前端只提交 `capture_id`，不提交
原始令牌、Cookie 或 Admin Key。

Sub2API 管理地址和转发地址分离处理：账号、分组和密钥接口使用管理站地址；页面配置
中的 `api_base_url` 用于发现 OpenAI 兼容转发地址。`api_base_url` 可以是绝对 URL，
也可以是相对路径。相对路径会按最终 HTML 页面地址解析，并继续执行 SSRF、重定向和
来源校验。页面声明的 Relay 地址只有在与最终 HTML 页面来源或原始管理地址使用相同
协议、主机和有效端口时才接受；另外允许严格的直接 `api.` 父子域关系（协议和有效
端口仍必须一致），不再以同一注册域名作为放宽条件。发现到的转发地址保存到
`relay_base_url`，写入渠道 `base_url` 前会移除结尾 `/v1`，避免转发时形成
`/v1/v1/...`。

当管理页面从 `hhw1231.com` 跳转到 `hengwenapi.com`，且最终页面明确声明
`https://hengwenapi.com` 时，本轮管理请求会直接使用最终可信页面来源，避免跨主机
重定向丢失认证 Header；持久化的管理地址仍保持 `https://hhw1231.com`，Relay 地址
单独保存。`aiapipay.com` 与页面明确声明的 `api.aiapipay.com` 属于允许的直接
`api.` 父子域关系，管理接口仍使用 `aiapipay.com`，单 Key 模型探测才使用 Relay。

Relay 模型探测只发送该 Key 的 `Authorization` 和 `x-api-key`，不复制管理会话的
Cookie、Origin、Referer、X-Requested-With、X-Auth-Session 或 Cookie Jar。分组/账号
模型目录不能替代单 Key `/v1/models` 或 `/models` 的成功确认；未确认模型的 Key
保留资源记录但不可路由。

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

Sub2API 普通用户接口如果只能返回掩码密钥，不会伪造真实密钥。列表已经返回完整
密钥时不再请求详情；只有密钥缺失或掩码时才读取 `/api/v1/keys/{id}`。无法取得真实
密钥的新记录会自动禁用；已有密钥保留旧密文并继续按同步状态过滤。

### 4.1 2026-10-01 Sub2API 登录服务条款兼容

**变更前**

- Sub2API 账号密码登录没有读取公开服务条款设置，遇到启用登录条款的部署时只能按旧
  协议提交主体和密码；
- 二次验证阶段不会重新读取条款版本，登录前后条款版本变化或条款检查延迟到
  `/api/v1/auth/login/2fa` 的部署无法兼容；
- 条款拒绝可能被归入普通凭据错误或安全验证错误，后台同步状态无法区分条款要求，
  也没有单独说明快照保留边界。

**变更后**

- 仅 Sub2API 的账号密码登录在已规范化且已通过 SSRF、重定向和渠道代理校验的管理地址
  上，复用当前平台 Session 的 HTTP Client、CookieJar、超时和重定向策略读取
  `GET /api/v1/settings/public`；
- 只有初次登录响应明确包含条款要求 marker 时，才读取
  `GET /api/v1/settings/public`；响应中的 `login_agreement_enabled` 严格为 `true`，
  且 `login_agreement_revision` 是非空字符串时，才使用相同登录路由和相同主体重试，
  并增加 `agreed_revision`。设置接口失败、404、网络错误、响应格式异常、条款关闭或
  revision 缺失时直接返回条款错误，不扩展主体或路由组合；
- 如果初次登录返回 2FA challenge，验证前重新读取公开设置，并把当时最新的
  `agreed_revision` 发送到 `POST /api/v1/auth/login/2fa`。不在 NexusTok 本地持久化
  “已同意”记录，不接受客户端传入的条款版本，不发送未由参考源确认的
  `not_in_cn_confirmed`；
- Sub2API 条款 marker（包括 `login_agreement`、`agreed_revision`、`terms of service`、
  `service terms`、`agreement required`、`accept terms`、`agree to terms` 和
  `policy acceptance`）优先归类为 `login_agreement_required`，不触发邮箱、用户名或
  混合主体回退，也不归类为账号密码错误或安全验证错误。New API 不读取该公开设置接口、
  不发送 `agreed_revision`，也不推断或发送同名字段；
- 条款拒绝通过 `ErrSub2APILoginAgreement` 和脱敏中文提示返回，提示已尝试提交当前
  条款版本但上游仍拒绝，并引导检查上游条款配置或使用浏览器采集登录态；
- 后台同步遇到条款拒绝时写入 `sync_status=failed`、
  `auth_status=login_agreement_required`、脱敏 `auth_status_reason`，递增连续失败
  次数；不清理现有密钥、模型能力、余额或最近成功快照。本次不新增数据库表、字段或
  迁移，前端复用资源面板现有危险状态样式。

## 5. 同步任务

- 保存平台站点后排队一次初始同步。
- 默认每 15 分钟同步。
- 同一站点使用互斥锁。
- 支持管理员手动同步。
- 分组、分页和必要密钥读取满足当前完整性边界后，才执行缺失判定并开启数据库事务
  upsert；资源部分失败仍可写入已成功的身份/用量状态，但不清理最近成功 Key。
- 同步开始只切换为 `running`，失败时只记录 `failed`、脱敏错误和连续失败次数；
  不删除旧密钥、不清理旧模型能力、不标记旧密钥 `missing`、不覆盖旧余额，
  并继续复用最近一次成功快照。
- 账号密码同步被 Sub2API 服务条款拒绝时，额外写入
  `auth_status=login_agreement_required`；该状态与 `credentials_invalid`、
  `secure_verification_required`、权限不足和资源失败分开处理，且不能触发主体回退。
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

同步结果还按身份、分组、端点、密钥、模型和用量分别记录。某一资源类型失败时，
保留该类型最近成功快照并写入资源级失败状态；安全验证或 Admin Key step-up 拒绝
读取密钥时，不把密钥标记为缺失或删除。只有完整分页、资源校验和密钥读取成功后，
才依据“本次未返回”标记密钥缺失。余额同时区分当前值、最近成功值、来源接口和
是否完整，管理地址与 Relay/API 地址独立保存。

同步日志只记录站点 ID、平台、结果、数量、耗时和脱敏错误，不记录任何凭据或实际密钥。

## 6. API

现有对外渠道接口保持兼容。新增管理员接口：

```text
POST  /api/channel/:id/upstream-sync
GET   /api/channel/:id/upstream-sync
GET   /api/channel/:id/upstream-keys
PATCH /api/channel/:id/upstream-keys/:keyId
POST  /api/channel/:id/upstream-keys/batch-status
POST  /api/channel/platform-site/auth-flow/start
POST  /api/channel/platform-site/auth-flow/:flowID/verify
DELETE /api/channel/platform-site/auth-flow/:flowID
GET   /api/channel/:id/upstream-resources
POST  /api/channel/:id/upstream-resources/sync
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

`GET /api/channel/:id/upstream-keys` 返回管理员管理和资源诊断所需的字段：密钥 ID、父渠道 ID、外部 ID、名称、脱敏 `KeyPreview`、`Models`、`ModelsSynced`、`UsedQuota`、`RemainQuota`、`ExpiresAt`、优先级、上游原始倍率、生效倍率、倍率覆盖、有效权重、自动权重、权重覆盖、`Status`、禁用摘要、同步时间、最近使用时间、可路由状态和不可用原因。真实完整 Secret 不返回；`KeyPreview` 只用于脱敏展示，不能作为上游请求凭据。管理端模型列成功同步时只展示模型列表；模型尚未同步时显示不可用提示，不额外显示 `Synced`。

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

平台站点认证失败提示会区分 Sub2API 服务条款要求；条款拒绝只说明 NexusTok 已尝试
提交公开设置提供的当前 revision，不代表 NexusTok 代替用户完成或记录法律同意。

`routable_key_count` 只统计父渠道启用、平台快照可用、子密钥状态可路由、
模型能力已确认且实际密钥能够解密的子密钥。无法解密的历史密钥、缺少模型能力
或仅保留展示快照的记录不会计入可路由数量。

所有接口必须经过管理员权限校验。敏感凭据查看必须经过现有安全验证机制，默认只返回指纹和掩码。

`PlatformSiteInput` 保存账号密码认证时只接受一次性 `auth_flow_id`；服务端消费流程
后再加密保存最终凭据，不接受前端重复提交密码或验证码。资源查询只返回掩码密钥、
身份、额度、分组倍率、端点、模型能力和资源级状态，不返回完整密钥。

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

实现和测试参考 OWASP ASVS 5.0.0（2025-05 发布）以及认证、会话、密码存储、
CSRF、日志、密码学存储和 SSRF Prevention Cheat Sheet。2026-09-27 复核时，本次
功能只确认了与平台站点凭据、Dashboard 会话、管理员资源权限、外部站点请求和安全
诊断直接相关的控制主题，未将未经逐条核对的 requirement ID 写入本文，也不宣称覆盖
ASVS 全部要求：

- 认证失败、重新认证和旧凭据失效时的有限回退；
- Session ID、Refresh Cookie、Bearer Token 的轮换、生命周期和加密保存；
- 管理员资源授权、安全验证 step-up、权限错误与认证错误的区分；
- 管理地址 URL 校验、HTTPS/重定向边界、响应大小和 SSRF 防护；
- 管理员诊断、审计和错误信息中的敏感字段脱敏。

同时参考 OWASP Authentication、Session Management、Password Storage、OAuth、CSRF、Logging、Cryptographic Storage 和 Server Side Request Forgery Prevention Cheat Sheet。审计事件只记录站点 ID、渠道 ID、平台类型、操作类型、结果、操作者、请求 ID 和脱敏错误，不记录密码、Cookie、令牌、Admin Key、刷新令牌或实际密钥。

2026-09-27 复核时通过 OWASP 官方项目页和 ASVS 仓库确认 ASVS 稳定版本仍为
`5.0.0`，发布日期为 2025-05；安全核对范围仅覆盖本功能涉及的平台凭据保存、
管理员操作、外部站点请求边界、脱敏错误和审计日志，不宣称覆盖项目全部认证/会话
实现。

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

认证方式只显示账号密码和自动配置，平台站点由基本信息中的 NewAPI/Sub2API 渠道类型
派生；凭证区域不再显示重复的平台选择、高级认证入口或 Access Token、Admin Key、
Cookie 手动输入。认证方式和站点地址在宽屏使用约 `2fr 8fr` 栅格，窄屏纵向排列。
账号密码编辑时从同步状态接口的安全 `username` 字段回填用户名，密码保持为空并由
后端合并未修改的已保存密码。账号密码及 TOTP 只存在表单内存；2FA 验证只提交到
认证流程接口，成功后仅保存短期 `auth_flow_id`，刷新、关闭、取消、失败和保存完成后
清理密码、验证码和流程 ID，不写入 Local Storage、Session Storage、URL 或持久化
Query Cache。资源面板展示管理/Relay 地址、平台身份、余额、已用额度、额度单位、
当前分组及倍率、协议端点、密钥统计、最近成功同步时间、旧快照和资源级失败原因。

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

认证流程测试必须覆盖 2FA challenge、错误次数、过期、重放、管理员/Origin/平台
绑定、New API 两种刷新形态、现代 Bundle 不完整拒绝降级、Sub2API 轮换不确定结果、
Sub2API 条款版本读取失败回退、初次登录和 2FA 的最新 revision、条款错误不触发主体
回退、`login_agreement_required` 状态和敏感字段脱敏，以及安全验证拒绝读取密钥时的
旧快照保留。数据库模型变更必须实际验证
SQLite、MySQL 和 PostgreSQL 的新建、升级、幂等迁移、索引约束、删除清理与事务回滚；
未完成矩阵时不得声明数据库兼容已完成。

安全核对基于 OWASP ASVS 5.0.0 以及 Authentication、Session Management、MFA、
CSRF、Cryptographic Storage 和 SSRF Prevention Cheat Sheet。当前实现明确不覆盖
New API Passkey/WebAuthn 和上游安全证明的自动完成能力。

## 10. 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-26 | 认证与资源同步基线 | 账号密码遇到 2FA 时只返回普通认证错误；快照主要覆盖余额、模型和密钥 | 增加短期认证流程、两类会话刷新契约、资源级失败/快照保留、管理/Relay 地址分离和新增资源接口设计基线 | NewAPI/Sub2API 认证、同步、管理端和测试 | 参考源路由核对；`service/upstream_site*.go`、`controller/upstream_channel.go`、参考项目认证实现 |
| 2026-10-01 | Sub2API 登录服务条款兼容 | Sub2API 账号密码登录未读取公开条款设置，2FA 不携带最新版本，条款拒绝与凭据/安全验证状态边界不清 | 仅 Sub2API 密码登录按 `GET /api/v1/settings/public` 的启用标记和非空 revision 可选发送 `agreed_revision`；设置失败回退旧协议，2FA 重新读取，条款拒绝独立分类并保留最近成功快照；New API 不读取或发送该字段，不发送 `not_in_cn_confirmed`，不新增数据库结构 | Sub2API 认证、2FA、后台同步状态、快照和资源面板 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/platform_site_auth_flow.go`、`service/upstream_site_test.go`；Sub2API 参考源公开 DTO；SQLite 3.50.4、MySQL 8.2.0、PostgreSQL 15.19 矩阵 |

### 10.1 2026-09-26 实现校准

**变更前**

- 账号密码认证完成后，管理端无法统一承接 New API 的 2FA challenge；
- 资源状态主要依赖父渠道同步状态，不能分别说明身份、分组、端点、用量、密钥和模型的失败；
- New API 账号模型与管理员渠道模型的来源边界不清晰；
- 管理地址、Relay 地址和模型地址在 Sub2API 页面发现后没有统一展示；
- 资源面板只能看到密钥基本信息，不能查看端点能力、额度、过期时间和旧快照状态。

**变更后**

- `POST /api/channel/platform-site/auth-flow/start`、`verify` 和 `DELETE` 提供绑定管理员、
  平台、规范化站点地址和可选渠道的 5 分钟短期流程。账号密码只在流程启动请求中提交，
  2FA 只提交到 verify 接口，流程成功后保存渠道只提交一次性 `auth_flow_id`；
- New API 支持 `/api/user/login/2fa`、`/api/user/login/verify`，Sub2API 支持
  `/api/v1/auth/login/2fa`，均记录失败次数、验证码次数和消费状态。Sub2API 2FA 请求携带
  同源 `Origin`、`Referer`、`User-Agent` 和 `X-Requested-With`；
- New API 刷新同时识别传统 Token 对和 Dashboard Auth Bundle。现代 Bundle 一旦被识别但
  结构不完整，必须进入重新认证，不得降级按旧协议解释；Sub2API 刷新缺少新 Refresh Token
  或有效 `expires_in` 时记录 `uncertain_rotation`，不重放旧令牌；
- `platform_site_identities`、`platform_site_groups`、`platform_site_endpoints`、
  `platform_site_endpoint_capabilities` 和 `platform_site_resource_syncs` 由迁移统一管理。
  同步事务同时更新规范化资源表与现有 `UpstreamKey`/`UpstreamKeyAbility` 路由主数据；
- New API 当前读取 `/api/status`、`/api/user/self`、用户组、`/api/user/models`、
  `/api/pricing`、Token 分页和 Token Key 详情，并在 Admin Key 可用时合并
  `/api/channel/`、详情和 `fetch_models` 的模型。`supported_endpoint` 仅作为端点诊断能力
  保存，不会把管理员渠道模型复制给每个子密钥；
- Sub2API 当前按 auth/me、Profile、Groups、Group Rates、Dashboard Usage、Usage Stats、
  Keys 的顺序读取资源；Admin Key 额外读取 accounts/data。列表已经返回完整 Key 时跳过
  详情，缺失或掩码时才补读详情，并将页面 `api_base_url` 解析为管理地址与 Relay 地址，
  使用每个实际密钥请求 `/v1/models` 确认模型能力；
- 资源同步按类型保存最近尝试、最近成功、来源、数量、失败原因、部分成功和安全验证状态。
  安全验证拒绝或部分分页失败时保留旧密钥、额度和模型快照，只有完整分页和校验成功后
  才能把本次未返回的密钥标记为缺失；
- 管理端新增 `GET /api/channel/:id/upstream-resources` 和
  `POST /api/channel/:id/upstream-resources/sync`，资源面板显示管理/Relay 地址、身份、
  额度单位、分组倍率、协议端点、端点能力、密钥额度和资源级状态；普通响应仍不返回完整
  平台密钥、Cookie、Token 或 Refresh Token。

本次验证使用 SQLite 3.50.4、MySQL 8.2.0 和 PostgreSQL 15.19 的真实实例，完成新表建表、
资源快照写入、额度回退、旧渠道迁移和重复迁移测试。未覆盖 New API Passkey/WebAuthn、
Turnstile、Security Proof 及站点风控的自动完成。

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

### 9.2 2026-09-26 渠道 2、4、5 同步回归修复

**变更前**

- 平台站点上游返回 HTTP 错误、HTML/WAF 页面和 HTTP 200 业务失败时，部分路径只保留
  “响应格式错误”或普通认证失败，管理员无法区分账号密码错误、Turnstile/验证码、
  WAF 和网络不可达；
- New API 与 Sub2API 的密码登录可能通过多组字段或兼容路径重复尝试，Sub2API 非法
  账号格式会被延迟到上游响应阶段才暴露；
- 兼容资源接口在认证失败、安全验证或网络错误后可能继续尝试其它路径，增加上游请求
  和限流风险；
- 同步失败只更新父站点错误字段，无法稳定表达 `credentials_invalid` 与
  `secure_verification_required` 的差异。

**变更后**

- `service/upstream_site.go` 保存脱敏的 HTTP 状态、最终 URL、Content-Type、响应类型、
  响应类别和有限错误码；完整响应正文、密码、Cookie、Access Token、Refresh Token 和
  临时令牌均不进入错误摘要或日志；
- New API 密码登录主请求使用 `/api/user/login?turnstile=` 的 `username/password`；
  当输入为邮箱时，仅在明确凭据错误或 HTTP 401 后兼容其它主体；
- Sub2API 使用 `/api/v1/auth/login`，邮箱首请求只包含 `email/password`，仅在明确凭据
  错误或 HTTP 401 后最多一次使用 `username/password`；只有 404/405 才回退到
  `/auth/login`，400 `INVALID_REQUEST` 不触发任何额外登录请求；
- 只有明确的 404/405 才继续兼容资源路径；认证失败、Turnstile/验证码、WAF、step-up
  安全验证和网络错误立即停止重复请求；
- HTTP 200 的 `success=false`、数值错误码和 HTML/挑战页分别归类为业务认证失败、
  交互验证、WAF 或路由缺失。同步状态仍记录失败和连续失败次数，但不覆盖
  `last_sync_at`，不删除密钥、模型、额度、倍率、权重或能力快照；
- 账号密码错误使用 `auth_status=credentials_invalid`，安全验证使用
  `auth_status=secure_verification_required`。已有浏览器 Capture/Auth Flow 作为人工
  验证承接，不绕过 Turnstile、验证码、Passkey 或 WAF；浏览器不可用时仍返回安全验证
  状态并保留最近成功快照；
- 管理员可见诊断只包含站点、阶段、状态码、Content-Type、响应类别、同步阶段、失败
  次数和快照回退标记，不显示任何可用认证材料。

本轮自动化回归覆盖 Sub2API HTML/Turnstile、非法邮箱、New API HTTP 200 业务失败、
401 凭据错误、404/405 兼容路径、网络超时和失败同步快照保留。真实站点验证只记录
脱敏请求路径、HTTP 状态、Content-Type、响应类别和页面状态。

### 9.3 2026-09-27 NewAPI Dashboard Session 与资源失败边界

**变更前**

- NewAPI Dashboard Refresh 主要依赖显式 Cookie、Session ID 和 Bearer Token，但
  CookieJar 中的轮换 Cookie 可能与显式 `Cookie` 头重复，且轮换值不一定回写到持久化
  凭据；
- 登录、刷新和认证流程入口对现代 Dashboard Auth Bundle 的判断不完全一致，结构不完整
  的响应存在落入旧 Token 协议兜底的风险；
- `/api/token/` 分页或密钥明文读取失败时，身份和余额快照可能无法与资源失败分开保存；
- 管理地址、Relay 地址和带路径前缀的站点地址边界不清晰，错误信息难以判断最终请求地址。

**变更后**

- `BaseURL` 对 NewAPI 类平台表示管理 API 根地址，只去除末尾 `/`，保留用户填写的路径
  前缀；不会把 Relay 地址、`/v1` 地址或其它候选地址猜测为管理地址。兼容资源路径仅在
  明确收到 HTTP 404 或 405 时回退；
- NewAPI Dashboard Refresh 使用 `POST /api/user/auth/refresh`，以
  `new_api_refresh` Cookie、`X-Auth-Session` 和当前 Bearer Token 组成会话请求，不把
  Refresh Cookie 放入 JSON body。CookieJar 中同名 Cookie 优先，显式头只保留 Jar 中没有
  的其它 Cookie，响应 `Set-Cookie` 的轮换值回写加密凭据；
- 现代 Bundle 必须同时满足 `success`、`data.access_token`、`data.token_type`、
  `data.access_expires_at`、当前 `data.session.sid/current` 和 `data.user` 身份字段。
  已出现现代结构标志但结构不完整时重新认证或标记刷新不确定，不降级旧协议；密码模式
  成功后仍保留密码认证类型，Access Token、Session ID、Token 类型和过期时间只作为会话
  状态保存；
- 认证成功后，身份、余额/用量和分组等已成功资源可以先写入；Token 分页、明文 Key、
  单密钥模型或 Admin 资源失败时写入资源级失败/安全验证状态，保留最近成功密钥、模型、
  倍率、权重和能力，不执行缺失判定，不覆盖 `last_sync_at`；
- 管理员诊断只保留同步阶段、脱敏最终 URL、HTTP 状态、Content-Type、响应类型、错误码、
  资源状态、快照回退和人工安全验证标记，不记录密码、Cookie、Access Token、Refresh
  Token 或完整响应正文。

### 9.4 2026-09-27 渠道代理与认证链路

**变更前**

- 平台站点同步和账号密码 Auth Flow 只根据站点 `BaseURL` 创建直连客户端，没有读取
  渠道 `setting.proxy`；需要局域网代理的站点会在认证阶段直接连接失败；
- 注入通用客户端时可能覆盖平台站点自己的 CookieJar、30 秒超时和内网重定向校验，
  使认证会话和安全边界出现不一致；
- NewAPI 返回 `Username or password...` 或 `user has been banned` 时，错误分类不够
  准确，管理员只能看到普通认证失败。

**变更后**

- `syncPlatformSite`、NewAPI/Sub2API 适配器和平台站点账号密码/二次验证流程按
  `channel_id` 读取 `model.Channel.setting`，使用已有 `GetHttpClientWithProxySettings`
  的 `proxy`、HTTP 协议和连接分片配置；空代理继续使用平台站点直连客户端。渠道 4
  的代理地址由数据库配置提供，代码不硬编码 `http://192.168.0.100:7897`；
- 注入渠道客户端时只替换底层 `Transport`，保留平台站点 CookieJar、30 秒超时、
  允许管理站点内网目标的重定向校验以及显式 Cookie 与 Jar 的合并规则。响应
  `Set-Cookie` 仍会回写 `new_api_refresh` 和其它会话 Cookie；
- `Username or password`、`user has been banned` 等真实 NewAPI 登录消息归类为凭据
  错误；代理配置错误、网络失败、安全验证、WAF、权限不足和资源级失败继续保持
  独立状态，不会互相降级，也不会将密码、Cookie、Token 或完整响应写入错误摘要；
- 管理 API 的 `BaseURL` 仍是管理根地址并保留路径前缀，Relay 地址不用于猜测管理
  路由；兼容接口只在明确 HTTP 404/405 时回退，认证成功但资源失败时继续保留最近
  成功的密钥、模型、倍率、权重和能力快照。

### 9.5 2026-09-27 旧版 NewAPI 主请求契约校准

**变更前**

- NewAPI 密钥分页主请求曾使用 `p=0&size=100`，部分派生站点会拒绝、归一化或误判
  分页边界；
- 登录、身份、分组、倍率、Token、Key 和模型读取的失败边界没有完全沿用旧版
  `service/upstreamaccount` 的主链路，资源探测失败可能被误解为账号认证失败；
- 批量明文 Key 返回部分结果时，兼容读取可能重复请求已经成功的密钥；掩码值也可能
  被误当作可用凭据；
- 账号级模型目录与子密钥模型能力边界不够明确，存在把账号模型复制给每个子密钥的
  风险。

**变更后**

- NewAPI 用户态同步按旧版核心顺序处理：尽力读取 `GET /api/status`，失败时使用
  默认额度换算并记录用量资源 warning；密码登录主请求使用
  `POST /api/user/login?turnstile=` 的 `username/password`，邮箱输入只在明确凭据错误
  或 HTTP 401 后兼容 email 和混合主体；登录后读取 `/api/user/self`、
  `/api/user/self/groups`、`/api/ratio_config`；
- Token 主分页固定从 `GET /api/token/?p=1&page_size=100` 开始，后续使用 `p=2`、
  `p=3` 等一基页码，并依据 `total`、页码、页大小和条目数判断是否完成；主路径不再
  使用 `p=0&size=100`。`/api/token` 和 `/api/tokens` 只在明确 HTTP 404 或 405 时作为
  路由兼容回退；
- `POST /api/token/batch/keys` 返回部分 ID 时只补偿缺失 ID；单条
  `POST /api/token/{id}/key` 只有明确 404/405 才尝试 GET。空值、空白值和包含 `*`
  的掩码值都不写入真实密钥，也不触发无意义的逐条重放；
- Token 资源分页上限从当前旧值 100 页恢复为独立 1000 页；Sub2API 普通 Key 和
  Admin Key 分页同样从 100 页恢复为独立 1000 页，其它管理资源继续使用自己的限制；
- Token 记录中的 `model_limits`、`models` 等字段优先作为对应子密钥能力。只有该
  子密钥没有模型字段时，才使用该密钥独立访问 `/v1/models` 探测；账号级模型接口
  只用于管理端诊断和展示，不复制给所有子密钥；
- 兼容分组、模型和 Key 路径只在 404/405 回退。401、403、429、WAF、Turnstile、
  安全验证、网络超时和普通业务错误停止当前阶段，不重放密码、Refresh Cookie 或
  旧 Access Token；
- 认证成功、Token 分页完整、至少一个子密钥具有完整 Secret 且确认了真实模型能力时，
  父渠道才可标记 `success` 并更新 `last_sync_at`。可选 status、分组、倍率、价格、
  Endpoint、Admin 资源和个别 Key 失败独立记录资源状态；`KeysComplete=false` 时不
  执行缺失 Key 判定，并保留最近成功密钥、模型、倍率、权重和能力快照；
- 当前前端页面、管理接口和资源展示模型保持不变。NewAPI `BaseURL` 仍只表示管理
  API 根地址，保留用户填写的路径前缀，Relay 地址不参与管理路由猜测。Dashboard
  Refresh 继续同时使用 `new_api_refresh` Cookie、`X-Auth-Session` 和 Bearer Token，
  CookieJar 轮换值加密回写凭据。

### 9.6 2026-09-30 旧版资源同步迁移与脱敏验收

**变更前**

- New API Token、Sub2API 普通 Key 和 Admin Key 资源分页统一最多 100 页，真实大账号
  可能无法完整同步；
- New API 和 Sub2API 登录主体、Sub2API 资源请求顺序、完整 Key 详情依赖和模型能力
  来源没有完整复刻旧版；
- 本次范围不使用真实平台账号或真实站点请求，旧版兼容行为缺少集中化的脱敏
  HTTP fixture、路由和地址回归记录。

**变更后**

- New API Token、Sub2API 普通 Key 和 Admin Key 分页各自最多 1000 页，每页 100 条；
  分页中途失败、空 envelope、权限不足或安全验证不触发 Key 缺失判定；
- New API 恢复受限的 username/email/混合主体兼容、批量 Key 缺失补偿和单条 POST/GET
  回退；Sub2API 恢复 email/username/混合主体兼容、Profile/Groups/Usage/Keys 顺序、
  列表完整 Key 优先、详情补齐和缺少模型时的 Relay `/v1/models` 探测；
- 凭据加密、Sub2API Refresh Token 完整轮换、Bundle 完整性、安全验证分类和最近成功
  快照保留规则继续生效；Sub2API 平台窗口额度、Key 5 小时/日/7 日窗口字段和账号级
  模型目录仍未进入当前数据模型；
- 平台站点规范 `key_id` 使用 `RoutingKey.ID`；历史 `UpstreamKey.ID` 只允许同渠道
  兼容回退。NewAPI 和 Sub2API 的管理 `BaseURL`、发现的 `RelayBaseURL` 与渠道
  Relay 根地址分离保存，渠道请求根地址去除末尾 `/v1`，避免生成 `/v1/v1/...`；
- 本次只完成脱敏 `httptest` 的登录/资源 envelope、Key 详情、`/v1/models` 请求
  路径、Bearer 完整 Secret、模型映射和快照边界回归；未使用真实账号、密码、Cookie、
  Access Token、Refresh Token、Admin Key、完整上游 Key、网络捕获或截图，未调用
  任何计费接口。

### 9.7 2026-10-02 密码同步会话清理与自动配置边界

**变更前**

- NewAPI/Sub2API 后台密码同步可能依赖历史 Access Token、Refresh Token、Cookie、
  Session ID 或 Dashboard Refresh，资源读取完成后也没有统一的临时会话注销契约；
- 失败清理可能与资源同步结果耦合，存在把已取得的身份、额度或最近成功 Key 快照
  覆盖为凭据失效的风险；
- 自动配置文档把手动凭据入口、高级认证、平台选择和浏览器采集登录态混为一层，
  Capture 记录没有跨节点的一次性占用说明。

**变更后**

- 密码认证在登录前清除内存中的历史 Authorization、Cookie、`X-Auth-Session`、
  Access Token、Refresh Token、Session ID、过期信息和刷新状态，每次直接提交账号
  密码。密码同步成功后只保存认证方式、真实用户名、密码、可选 User ID、最后认证
  时间和正常状态；临时登录材料不会写回长期凭据。
- NewAPI 资源读取完成后调用 `POST /api/user/auth/logout`，随后以本轮 SID 调用
  `DELETE /api/user/sessions/{sid}` 精确清理；`AUTH_SESSION_MISMATCH`、401、403、
  404、405 视为幂等完成，绝不调用 `/api/user/sessions/revoke-others`。请求携带本轮
  Bearer、`X-Auth-Session`、Cookie 和正确 `Origin`。
- Sub2API 资源读取完成后调用 `POST /api/v1/auth/logout`，请求体仅提交本轮
  `refresh_token`，绝不调用 `/api/v1/auth/revoke-all-sessions`。缺少 Refresh Token、
  网络错误或意外 5xx 只写脱敏告警，临时材料仍清除，不覆盖已经获得的快照或把账号
  直接标记为凭据失效。
- Cleanup 延迟路径覆盖认证失败、资源失败、部分分页失败和快照写库前后；浏览器
  Capture 登录态属于用户已有会话，不执行后台注销。Capture Helper `1.7.0` 先做当前
  用户或 Admin 权限验证，诊断只保存候选存在性、存储键、尝试接口、版本、失败阶段
  和来源；结果通过现有加密凭据结构保存。
- `ResolvePlatformSiteCapture` 成功后使用 Redis `SET NX` 或进程内 claim 独占记录，
  后续渠道保存失败释放 claim，成功保存删除记录并释放 claim。claim token 不返回
  前端；历史 Access Token、Admin Key、Cookie 渠道继续可读可同步，但新版界面不再
  新增手动录入入口。

### 9.8 2026-10-03 认证流程保存兼容与 NewAPI 加密登录

**变更前**

- 账号密码认证流程完成后，渠道表单仍可能把用户名和密码残留在
  `platform_site_auth_flow_id` 请求中；后端把用户名误判为流程与手动凭据混用，导致
  创建或保存渠道时在认证流程解析前失败；
- NewAPI 新版密码登录已经要求读取加密密钥，但平台同步客户端仍可能按旧版明文
  `password` 字段登录，未覆盖 RSA-OAEP 和长密码信封；
- Sub2API 的邮箱主体优先、凭据错误后才尝试用户名主体，以及 Refresh Token 精确登出
  的事实边界没有在本专项文档中集中记录。

**变更后**

- 账号密码认证已经取得 `auth_flow_id` 时，前端只提交平台、站点地址、认证方式、
  流程 ID 和金额配置，不再提交用户名、密码或其它登录材料。后端兼容旧版客户端
  残留的用户名字段，但以一次性认证流程解析出的真实用户名、User ID 和凭据为准；
  密码、User ID、Access/Refresh Token、Token 过期信息、Token 类型、Session ID、
  Admin Key、Cookie 和 Capture ID 仍与流程同时提交时拒绝；
- 认证流程解析成功后继续由服务端写入最终账号、真实用户名、User ID 和加密凭据，
  并执行一次性消费。未完成认证流程的账号密码请求仍提交用户名和密码；编辑已有
  密码渠道时，空密码仍由后端合并已保存密码；
- Sub2API 密码登录先提交 `email`，仅在明确凭据错误或 HTTP 401 时回退
  `username`；条款、安全验证、WAF、限流、权限和网络错误不触发主体盲目重试。
  当前用户读取 `/api/v1/auth/me`，后台清理只向 `/api/v1/auth/logout` 提交本轮
  Refresh Token，不撤销其它会话；上游只返回邮箱时，邮箱作为统一用户名回填值；
- NewAPI 密码登录先读取 `GET /api/user/login/encryption-key`。启用加密时，短密码
  使用 RSA-OAEP-SHA256，长密码使用 `v2.<wrapped-key>.<nonce>.<ciphertext>` 的
  AES-GCM 信封，并使用 `password-v2` OAEP 标签和 `password-v2:<kid>` AAD；请求
  只包含 `username`、`password_encrypted` 和 `encryption_key_id`。加密路由明确返回
  404 或 405 时才回退旧版明文 `password`，网络错误、403、5xx、无效公钥或无效
  配置均不静默降级；
- 测试和诊断只使用脱敏 HTTP fixture 与本机参考源码。密码、Token、Cookie、Admin
  Key、完整上游响应和真实站点账号不会写入日志、错误响应、文档、测试 fixture 或
  Git 提交。

### 9.9 2026-10-03 Sub2API Relay 发现与严格登录请求

**变更前**

- 页面声明的相对 `api_base_url` 按原始管理地址解析，页面重定向到独立 Relay
  域名时会被错误丢弃；
- Relay 地址校验曾以同源或同注册域名作为主要边界，无法覆盖
  `hhw1231.com -> hengwenapi.com` 这类由最终页面明确声明的跨域 Relay；
- 普通登录可能在首请求中携带条款 revision、混合 `email`/`username` 字段，严格
  校验的站点会返回 `400 INVALID_REQUEST`。

**变更后**

- HTML 请求保留最终响应 URL；相对 `api_base_url` 以最终页面地址解析；
- 页面明确声明的 Relay 仅在与最终 HTML 页面来源或原始管理地址协议、主机和有效端口
  一致时接受，并继续执行 HTTP/HTTPS、端口、重定向和 SSRF 校验；未经页面声明的
  第三方地址仍拒绝；
- 管理地址继续用于登录、当前用户、分组、额度和 Key 接口，Relay 地址只用于
  `/v1/models` 和最终转发。认证成功但 Key 或模型资源失败时仍保留最近成功快照，
  不把资源失败误判为凭据失效；
- Sub2API 邮箱首请求严格只发送 `email/password`；`/api/v1/auth/login` 只有
  404/405 才回退 `/auth/login`，400 `INVALID_REQUEST`、403、429、WAF、安全验证、
  网络和 5xx 不触发路由或主体重试。只有响应明确要求条款时，才读取公开设置并使用
  同一路由、同一主体携带 revision 重试一次。

### 9.10 2026-10-04 Sub2API 单 Key 模型探测失败与历史快照

**变更前**

- Sub2API 管理面登录、当前用户、分组、用量和 Key 列表均成功，但所有单 Key
  `/v1/models` 因余额不足或分组停用失败时，父渠道仍会被判定为资源同步失败；
- 已有成功同步的 Key、模型能力和路由快照无法通过“剩余额度刷新”继续保留，管理面
  的身份和额度更新也会被整体失败状态遮蔽。

**变更后**

- `syncPlatformSite` 区分管理资源成功与单 Key 模型探测失败。已有 `last_sync_at` 的
  Sub2API 渠道在本轮取得完整 Secret、但模型探测返回 `INSUFFICIENT_BALANCE`、
  `GROUP_DISABLED` 等资源条件错误时，继续更新身份、余额、用量、分组和 Key 状态；
- `keys` 资源标记为 `partial`，`models` 资源标记为 `stale`，保留最近成功的
  `UpstreamKey`、`UpstreamKeyAbility`、父渠道模型并集和路由候选，父渠道同步状态
  允许为成功；该情况不标记为凭据失效；
- 没有历史成功快照的新渠道仍必须等待至少一个真实单 Key 模型能力确认。模型广场、
  分组模型目录和账号级模型不会被复制为单 Key 能力。

### 9.11 2026-10-04 Capture Helper 登录态等待与 NewAPI/Sub2API 兼容

**变更前**

- Capture Helper 在 `document-start` 阶段直接执行一次采集，页面尚未完成登录、
  SPA 尚未写入登录态或 Cloudflare 验证尚未结束时，会把暂时未登录提交为永久失败；
- handoff 由后端使用下划线字段生成，但 Helper 启动校验只读取部分驼峰字段，可能把
  有效采集会话误判为参数无效；
- NewAPI Cookie 验证没有稳定携带 `New-Api-User`，仅有 `uid + Cookie` 的老版本站点
  无法通过 `/api/user/self`；Sub2API Session Restore 没有完整读取 IndexedDB 客户端 ID。

**变更后**

- Helper 在 DOM 就绪后启动，采集失败只显示脱敏等待状态并每 3 秒自动重试，同时提供
  手动“重新采集”按钮；暂时未登录、Token 尚未写入、Cookie 尚未形成、WAF 或网络
  短暂失败不会调用完成接口提交 `error`，Capture Session 继续保持 `pending`；
- Helper 同时兼容 `capture_secret`/`captureSecret`、`complete_url`/`completeURL`、
  `capture_id`/`captureID`、`expires_at`/`expiresAt` 和版本字段，修复真实 handoff
  字段与启动校验不一致；
- NewAPI 自动采集从 `localStorage.uid`、用户对象和 JWT 解析数字用户 ID，并在
  `/api/user/self`、Dashboard Refresh 和 `/api/user/token` 请求中携带
  `New-Api-User`；Cookie 验证成功后才尝试读取 Access Token；
- Sub2API 继续读取 localStorage、hash 和页面状态，并增加
  `sub2api-auth-coordination/values/sub2api_auth_client_id` IndexedDB 读取，通过
  `X-Sub2API-Auth-Client` 调用 Session Restore；浏览器 Capture 登录态仍不进入后台
  密码会话 Cleanup。
- Helper `1.7.0` 同时合并页面可见 Cookie 与同站 `GM_cookie.list` 结果并按名称去重，
  不再在已有可见 Cookie 时跳过 HttpOnly Cookie 读取；回传前删除完整 `auth_user`，
  `base_url` 和 `management_base_url` 保留 handoff 中的管理地址路径。

### 9.12 2026-10-04 无扩展页面桥接与采集会话过期处理

**变更前**

- 自动配置主要依赖浏览器 UserScript 扩展。没有 Tampermonkey/Violentmonkey 时，
  上游页面不会执行 Helper，前端仍可能持续轮询已过期的 Capture Session；
- HTTPS 上游页面直接请求本机 NexusTok 回调地址时，可能受到 CORS 或 Private
  Network Access 限制，已验证登录态无法回传。

**变更后**

- Capture Helper 版本升级为 `1.7.0`，保留原有 UserScript 路径，并新增带一次性
  `install_token` 的 `bridge.js` 页面桥接地址。桥接脚本复用同一套 NewAPI/Sub2API
  候选、用户验证、`uid + New-Api-User`、IndexedDB Session Restore 和脱敏诊断逻辑；
- 无扩展时，NexusTok 在用户点击后从短期 `bridge.js` 地址获取采集脚本，并复制不含
  Capture Secret 的短启动片段。用户在已打开的上游页面上下文运行片段后，片段通过
  `window.opener.postMessage` 请求脚本；NexusTok 仅向当前活动窗口返回桥接脚本。上游
  页面只在 `window.opener` 存在且当前用户验证成功后，通过 `postMessage` 把一次性采集
  结果交给 NexusTok 同源页面；NexusTok 校验窗口、Origin、Capture ID 和 Helper 版本后
  再调用完成接口，成功后返回定向确认；
- 上游站点登录重定向导致地址栏 handoff 参数丢失时，桥接脚本会向活动 opener 请求
  一次性 handoff。管理页只向当前 Capture Session 的活动窗口、匹配 Origin 和匹配
  Capture ID 响应；浏览器拒绝跨源 `javascript:` 导航是预期安全边界，前端保留短启动
  片段复制入口，用户仍需在上游页面上下文执行该片段；
- 桥接脚本 URL 不携带 Capture Secret，脚本响应不保存 Token、Cookie、密码或完整
  响应。Capture Secret 只在当前页面内存消息和后端一次性校验链路中使用；
- Capture Session 查询返回“不存在/已过期”时，前端停止轮询、清理无效 ID 和桥接状态，
  显示重新创建入口，不再继续显示“正在检测 Capture Helper”。上游 Turnstile、WAF、
  验证码和 Passkey 仍必须由用户完成，桥接不绕过安全验证；
- `capture_helper` 与 `capture_bridge` 都必须通过现有用户或 Admin 验证、加密保存和
  一次性消费流程；浏览器登录态仍不进入后台密码会话 Cleanup，资源同步失败也不覆盖
  最近成功快照。

### 9.13 2026-10-05 旧版浏览器采集策略迁移校准

**变更前**

- 当前 `PlatformSiteCapture` 已复刻旧版大部分浏览器采集方式，但 NewAPI Dashboard
  Refresh 仍只尝试固定 `/api/user/auth/refresh`，Sub2API Refresh Token 也只调用固定
  `/api/v1/auth/refresh`，反向代理子路径或同源脚本中暴露的兼容路径无法参与这两个刷新
  分支；
- Sub2API 页面 `api_base_url` 指向带路径的相对 API 根时，脚本只保留 Origin，后续
  `auth/me`、Session Restore 和 Refresh 候选需要依赖其它前缀推断，容易和旧版“按页面
  子路径补全 API 路由”的采集方式不一致。

**变更后**

- 本次只迁移旧版浏览器登录态采集方式，不恢复旧版 `upstreamaccount` 账号池、Preview
  或 ChannelAccount 同步链路。采集结果仍通过当前 Capture Session、
  `PlatformSiteCredential` 整体加密、绑定用户/渠道和一次性 claim 保存；
- NewAPI Dashboard Refresh 加入同源脚本路径发现和子路径前缀扩展，继续向
  `POST /api/user/auth/refresh` 请求携带页面 Origin、当前站点 Cookie 和可用的数字
  `New-Api-User`，兼容新版不要求 `New-Api-User` 的 Bearer 路径和旧版需要用户头的
  Cookie 路径；
- Sub2API Refresh Token 分支加入 `/api/v1/auth/refresh`、`/api/auth/refresh`、
  `/auth/refresh` 及同源脚本发现路径，只有明确存在 Refresh Token 且需要恢复 Access
  Token 时调用；恢复后仍必须通过 `auth/me` 验证当前用户；
- API 前缀候选同时读取 handoff 管理地址、回传 `api_base_url`、当前页面 URL、页面
  `__APP_CONFIG__.api_base_url/apiBaseUrl` 和本地 `api_base_url` 存储值。若候选路径以
  `/api` 或 `/api/v1` 结尾，会剥离 API 后缀后再拼接 `auth/me`、Session Restore 或
  Refresh 路径，保留代理子路径但不生成重复 `/api/v1/api/v1/...`；
- 同源脚本发现仍只扫描当前页面同源 JS 资源，服务端仍只接受同 Host 或严格 `api.`
  父子 Host 的管理/API 地址关系，不恢复旧版任意同注册域名放宽。浏览器采集态仍不进入
  后台密码同步 Cleanup，资源失败继续保留最近成功快照。

## 与架构文档的关系

本文保留平台站点、账号同步、子密钥字段、倍率/权重公式、权限、SSRF 和测试验收等详细规则；架构文档描述平台站点如何进入渠道过滤、Routing Key 和 Relay 转发。新增平台类型或调整同步/路由语义时，必须同时更新本文、能力矩阵和偏差表。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 补充架构索引与元信息 | 已有平台站点详细设计没有统一事实基线和架构入口 | 增加事实基线、代码来源、架构分工和变更记录；保留同步、倍率、权重和兼容性细节 | NewAPI、Sub2API、平台账号、上游密钥和路由 | `model/upstream_channel.go`、`model/routing_key.go`、`service/upstream_site.go` 静态核对 |
| 2026-09-26 | 修复渠道 2、4、5 同步回归 | 上游 HTML/安全验证/HTTP 200 业务失败和网络错误可能被归为响应格式错误或覆盖同步状态；密码登录存在兼容字段漂移 | 固定 New API/Sub2API 登录 DTO，恢复脱敏响应诊断和错误分类，限制 404/405 回退，失败保留最近成功快照并区分凭据错误与安全验证 | 平台站点认证、资源同步、快照、管理员诊断和路由可用性 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/upstream_site_test.go`、本机三套参考源核对 |
| 2026-09-27 | 修复 NewAPI 类平台会话与资源同步 | Refresh Cookie、Session ID 和 Bearer Token 的真实 Dashboard 契约未在所有入口统一；Cookie 轮换、资源失败和管理/Relay 地址边界可能导致渠道 4、5 同步失败 | 统一 CookieJar/显式 Cookie 优先级和轮换持久化，严格区分现代 Bundle，按资源保存部分快照并限制 404/405 回退，诊断显示脱敏最终地址和资源状态 | NewAPI 及派生平台认证、刷新、身份/余额、密钥、模型、路由快照和管理员诊断 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/platform_site_auth_flow.go`、`service/upstream_site_test.go`、New API/Sub2API/all-api-hub 参考源 |
| 2026-09-27 | 接入渠道代理与 NewAPI 错误分类 | 平台站点同步、密码登录和 2FA 没有读取渠道代理；通用客户端注入可能覆盖平台会话 CookieJar、超时和重定向策略；真实账号错误消息分类不完整 | 按渠道配置复用代理 Transport，保留平台会话策略和 Cookie 轮换；同步与 Auth Flow 统一使用代理；识别 `username or password`/封禁消息并保持认证、网络、安全验证和资源失败分层 | 渠道 4 代理同步、渠道 5 认证诊断、NewAPI/Sub2API 平台站点认证和资源请求 | `service/upstream_site.go`、`service/platform_site_auth_flow.go`、`service/upstream_site_test.go`；代理、CookieJar、Auth Flow 和同步回归 |
| 2026-09-29 | 平台站点资源获取比较文档与管理端字段校准 | 专项文档将 `GET /api/channel/:id/upstream-keys` 描述为不返回额度、过期时间和最近使用时间，且没有链接旧版/参考源/当前实现的完整资源链路比较 | 明确该接口实际返回 `UsedQuota`、`RemainQuota`、`ExpiresAt`、`Models`、`ModelsSynced`、`Status`、可路由诊断和脱敏 `KeyPreview`；完整 Secret 仍不返回，并增加完整比较文档入口 | 平台站点资源查询、管理员诊断、额度和模型能力边界 | `controller/upstream_channel.go:75-104`、`controller/upstream_channel.go:667-710`、[`docs/platform-site-resource-acquisition-comparison.md`](platform-site-resource-acquisition-comparison.md) |
| 2026-10-02 | 密码同步会话清理与自动配置边界 | 密码同步可能复用历史登录态；NewAPI/Sub2API 注销路径、资源失败快照保护、浏览器会话所有权和自动配置一次性消费边界未统一 | 每轮密码同步直接登录并清理本轮会话；NewAPI 登出后精确删除 SID，Sub2API 仅 Refresh Token 登出，禁止撤销其他会话；清理失败不覆盖快照；自动配置完成验证、脱敏诊断、现有加密保存和 Redis/内存 claim 一次性消费，表单仅保留账号密码/自动配置 | 平台站点认证、资源同步、Capture Helper、前端表单和缓存 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/platform_site_capture.go`、`pkg/cachex/hybrid_cache.go`、`controller/channel.go`、前端平台站点组件、本机参考源和定向测试 |
| 2026-10-03 | 认证流程保存兼容与 NewAPI 加密登录 | 认证流程请求中的残留用户名被误判为手动凭据；NewAPI 新版密码加密登录协议和 Sub2API 邮箱主体/精确登出边界未在专项文档集中登记 | 有 `auth_flow_id` 时前端不再提交用户名/密码，后端仅兼容用户名残留并以流程真实身份为准；NewAPI 实现加密密钥读取、RSA-OAEP/v2 信封和仅 404/405 明文回退；Sub2API 保持邮箱优先、凭据错误后回退用户名和本轮 Refresh Token 登出 | 渠道保存、NewAPI/Sub2API 密码认证、认证流程一次性消费、脱敏诊断和最近成功快照 | `web/src/features/channels/lib/channel-form.ts`、`controller/upstream_channel.go`、`service/newapi_password_encryption.go`、`service/upstream_site_adapters.go`、相关 Go/React 定向测试；New API/Sub2API/all-api-hub 本机参考源 |
| 2026-10-04 | Capture Helper 等待重试与浏览器登录态兼容 | Helper 过早采集、下划线 handoff 字段被驼峰校验误判；NewAPI `uid + Cookie` 缺少 `New-Api-User`，Sub2API 缺少 IndexedDB 客户端 ID 兼容 | Helper DOM 就绪后等待登录并自动/手动重试，不把暂时未登录标记为失败；NewAPI 携带数字用户 ID 请求头并在 Cookie 验证后探测 Token；Sub2API 读取 IndexedDB Session Client ID；浏览器登录态不进入后台 Cleanup | 自动配置采集、NewAPI/Sub2API 登录态验证和渠道保存 | `service/platform_site_capture.go`、`service/upstream_site_test.go`、旧版 `service/upstreamaccount/capture.go`、New API/Sub2API/all-api-hub 参考源 |
| 2026-10-05 | 旧版浏览器自动采集方式迁移校准 | Dashboard Refresh、Sub2API Refresh 和带路径 API 根仍有固定路径/Origin 化处理，旧版同源脚本路径发现和代理子路径补全没有覆盖所有采集分支 | Refresh 分支纳入 `expandedAPIPaths` 与同源 JS 路径发现；API 前缀从管理地址、回传 API 地址、页面配置和当前页面共同推导，并剥离 `/api`/`/api/v1` 后缀；保留当前加密、一次性 claim、桥接和严格 Host 关系 | 自动配置采集、NewAPI Dashboard Refresh、Sub2API Refresh/Session Restore、反向代理子路径和渠道保存 | `service/platform_site_capture.go`、`service/upstream_site_test.go`、旧版 `service/upstreamaccount/capture.go`、New API/Sub2API/all-api-hub 本机参考源、定向 Go/React 测试 |

### 10.2 2026-10-05 Sub2API `custom_endpoints` Relay 与真实站点验收

**变更前**

- Sub2API 页面 `api_base_url` 为空时，Helper 和服务端没有稳定读取
  `custom_endpoints`/`customEndpoints`，管理地址和 Relay 地址容易被当成同一个地址；
- 页面声明的独立 Relay 可能被严格站点关系校验拒绝，或者仅凭客户端回传的
  `relay_base_url` 进入资源同步，服务端没有重新匿名核对页面公开配置；
- 资源同步没有明确“管理面读取 Key、Relay 面读取模型”的双地址边界，也没有把
  管理会话头与 Relay 探测请求隔离写入平台站点规则。

**变更后**

- Capture Helper 从 handoff、`window.__APP_CONFIG__`、页面初始化状态以及明确命名的
  localStorage/sessionStorage 配置读取 `custom_endpoints` 和 `customEndpoints`，
  支持绝对地址与按最终页面地址解析的相对地址；只接受 HTTP/HTTPS、有效端口和通过
  SSRF 校验的地址，并将第一个合法端点回传为 `relay_base_url`；
- 服务端使用受限 JSON 对象解析发现页面配置，保留 `api_base_url` 优先级，并在
  Capture 完成时以请求上下文匿名重新读取 `record.BaseURL` 页面。外部 Relay 必须是
  页面明确声明、规范化后与回传值完全匹配的地址；管理地址仍只允许同 Host 或严格
  `api.` 父子 Host 关系，不能通过同注册域名或客户端自报来源放宽；
- 对 `https://tk.shour.bond`，管理地址保存为该站点，页面声明的 Relay 保存为
  `https://api-image.shour.bond`。Sub2API 管理请求继续访问管理站点的
  `/api/v1/...`，单 Key 模型能力使用 Relay 的 `/v1/models`；Relay 请求只发送当前
  完整 Key 的 `Authorization` 和 `x-api-key`，不发送管理 Cookie、Bearer、Origin、
  Referer、`X-Requested-With` 或 `X-Auth-Session`；
- 采集结果继续通过 Capture Session、现有整体加密的 `PlatformSiteCredential`、
  用户/渠道绑定和 Redis/内存一次性 claim 保存，不恢复旧版账号池、Preview 或
  `ChannelAccount` 同步链路。浏览器采集态不参与后台密码同步 Cleanup，资源失败仍保留
  最近成功 Key、模型能力和渠道模型快照；
- 指定站点的脱敏验收已确认 Capture Session 完成、管理地址与 Relay 地址正确拆分；
  渠道处于启用状态，读取到 7 条上游 Key，7 条 `ModelsSynced=true`，渠道模型列表
  可用。资源表同时记录 `keys=partial`、`models=stale` 和 `using_snapshot=true`，
  验证部分资源失败时保留最近成功快照，未在日志、诊断或额外明文结构中输出或持久化
  账号、密码、Token、Cookie 或完整 Key；认证凭据和完整 Key 仍按现有整体加密结构保存。

### 10.3 2026-10-05 Sub2API Access Token 优先与资源同步复核

**变更前**

- 浏览器采集同时回传 Access Token 和 Refresh Token 时，后台认证可能无条件先调用
  Refresh。真实站点的旧 Refresh Token 失效会让仍然有效的 Access Token 在认证前被
  错误判定为不可用；
- Refresh 成功后的令牌轮换与当前用户验证顺序没有在平台站点规则中明确，兼容路径
  失败也可能掩盖真实的 `auth/me` 验证结果。

**变更后**

- Sub2API 有 Access Token 时先按 `/api/v1/auth/me`、`/api/auth/me`、
  `/api/v1/user/profile`、`/auth/me` 兼容顺序验证；只有当前用户接口明确返回 HTTP
  401 且同时存在 Refresh Token 时才刷新。只有 Refresh Token 时才直接刷新，刷新后
  必须重新执行 `auth/me` 验证并保存轮换后的 Access Token、Refresh Token 和过期时间；
- 有效 Access Token 与失效 Refresh Token 并存时不误刷新；网络错误、WAF、权限不足、
  非 401 业务失败不会触发 Refresh。认证成功后仍按管理地址读取身份、分组、用量和
  Key，按页面声明的 Relay 探测单 Key 模型，管理地址与 Relay 地址不混用；
- 对真实站点的脱敏复核确认 Capture Session 已完成，读取到 12 条管理面 Key，其中
  至少 7 条本轮 `ModelsSynced=true` 且可路由；部分 Key 探测失败时继续显示
  `keys=partial`、`models=stale` 和 `using_snapshot=true`，不清空最近成功能力；
- 本轮仍只迁移浏览器登录态采集到 Capture Session/整体加密
  `PlatformSiteCredential`，不恢复旧版 `upstreamaccount` 账号池、Preview、
  `ChannelAccount` 同步链路；浏览器采集不参与后台密码同步 Cleanup，也不新增数据库
  字段、迁移或明文凭据结构。

### 10.4 2026-10-05 Sub2API 标量过期时间与 Session Restore 触发边界

**变更前**

- 真实 Sub2API 页面可能把 `token_expires_at` 作为 localStorage 中的 JSON 数字或数字
  字符串保存；采集脚本的对象字段读取分支可能丢失该过期时间，导致已采集的
  Access Token 缺少有效期诊断；
- Browser Session Restore 在没有 `sub2api_auth_client_id` 或没有从当前页面同源
  JavaScript 发现 `session/restore` 路由时，容易发送空恢复请求或无依据地尝试固定
  兼容地址。

**变更后**

- `readNamed` 同时处理 JSON 对象、数字、布尔值和字符串，`token_expires_at` 经过
  毫秒 Unix 时间归一化后保存；Refresh Token 轮换仍要求 Access Token、Session
  信息和有效过期字段完整；
- Browser Session Restore 只有在存在 IndexedDB/localStorage 的
  `sub2api_auth_client_id` 且发现真实同源 `session/restore` 路由时才发起，并携带
  `X-Sub2API-Auth-Client`；缺少任一条件时记录脱敏的 `not_attempted`，不发送空请求，
  不扩大跨站范围；
- `https://tk.shour.bond` 当前以 `auth_token + auth_user + /api/v1/auth/me` 为
  主路径，页面没有 Client ID 时不进入 Restore 分支，仍可正常完成 Capture、管理面
  资源同步和 Relay 单 Key 模型探测。Capture Session、整体加密凭据、一次性 claim、
  最近成功快照和浏览器 Cleanup 隔离保持不变。

### 10.5 2026-10-05 Sub2API 配置包裹与 Cookie Client ID 兼容

**变更前**：Browser Restore 只从 localStorage/sessionStorage 和 IndexedDB 读取
`sub2api_auth_client_id`；页面配置若被 `data`/`config` 包裹或通过
`JSON.parse("...")` 初始化，Helper 与服务端页面发现可能漏掉 `custom_endpoints`。

**变更后**：Helper 按 localStorage、可见/扩展可读的精确 Cookie、
IndexedDB `sub2api-auth-coordination/values` 的顺序读取 Client ID，只记录存在性，
并仅在发现真实同源 `session/restore` 路由时发送 `X-Sub2API-Auth-Client`。配置读取
只递归有限的命名包裹字段，服务端用 `common.Unmarshal` 解码受限的
`JSON.parse` 字符串和转义 JSON；同时仅记录 hash Token 存在性，不保存其内容。
该兼容不改变 `auth_token + auth_user + auth/me` 主路径、管理/Relay 地址拆分、
Capture Session 加密凭据、一次性 claim、最近成功快照或浏览器 Cleanup 隔离。

### 10.6 2026-10-05 Sub2API CSP nonce 页面桥接

**变更前**

- 无扩展页面桥接收到 `bridge.js` 后立即创建内联 `script` 节点；目标站点
  `script-src 'self'` 同时要求动态 nonce 时，页面尚未暴露 nonce 会直接被 CSP
  拦截，表现为 handoff 参数保留、Helper 面板不出现和 Capture Session 一直
  pending；
- 管理页只有桥接结果和脚本请求分支，没有受校验的“桥接暂不可用”回传，失败时
  容易继续等待而缺少可重试反馈。

**变更后**

- `createCaptureBridgeBootstrap` 先按 `script[nonce]`、`HTMLScriptElement.nonce`
  和 `nonce` 属性读取当前页面 nonce；暂时没有 nonce 时等待 `DOMContentLoaded`，
  使用 `MutationObserver` 监听页面脚本节点和 nonce 属性，并在有限时间内重复查找。
  找到后只创建一次带 nonce 的脚本，超时不执行无 nonce 内联脚本；
- 等待超时只向当前 opener 回传
  `nexustok-upstream-capture-bridge-failed`。管理页继续校验活动窗口、上游
  Origin 和 Capture ID，校验通过后只显示已有“页面桥接暂不可用”提示，不调用
  完成接口、不消费 Capture Session，允许用户重新运行桥接；
- 不修改目标站 CSP，不使用 `unsafe-inline`、`eval`、Blob Script 或通配符
  `@connect`。`https://tk.shour.bond` 的管理地址仍与页面声明的
  `https://api-image.shour.bond` Relay 分离；管理请求、Relay 模型探测、加密
  `PlatformSiteCredential`、浏览器 Cleanup 隔离和资源失败保留最近成功快照的
  语义均不变。代码与前端回归已验证，真实浏览器复验仍需使用新的 Capture Session。
