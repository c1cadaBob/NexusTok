# Sub2API 与 New API 平台站点资源获取链路分析及版本差异

> 文档状态：源码事实分析
> 分析日期：2026-10-10
> 适用范围：旧版备份、Sub2API/New API/all-api-hub 本机参考源、当前 NexusTok
> 安全边界：本文只记录接口契约、字段语义、代码入口和失败处理，不记录任何真实密码、Cookie、Access Token、Refresh Token、Admin Key、测试账号、环境变量或完整密钥。

本文用于回答一个具体问题：平台站点的账号余额、累计用量、分组倍率、API Key、Key 额度、过期状态、Key 模型和 Relay 模型，是如何从上游平台被读取、转换、保存、刷新并最终影响 NexusTok 路由的。

文档将三个事实层次分开：

1. **参考平台事实**：从本机 Sub2API、New API 源码的真实路由、DTO、响应 envelope、中间件和失败处理得出。
2. **旧版 NexusTok 事实**：从 `/opt/project/backup_projects/NexusTok-0.1-archive` 的 `service/upstreamaccount` 得出。
3. **当前 NexusTok 事实**：从当前 `service/`、`model/`、`controller/`、`router/` 和测试得出。

除非明确标注“需要真实站点验证”，本文不把平台名称、README、Handler 注释或单个适配器方法推断为完整能力。

## 1. 分析范围、版本和证据等级

### 1.1 核对对象

| 对象 | 本次使用的路径 | 用途 |
| --- | --- | --- |
| 旧版 NexusTok | `/opt/project/backup_projects/NexusTok-0.1-archive` | 核对历史预览、创建、刷新、Key 模型同步和后台同步链路 |
| 当前 NexusTok | `/opt/project/NexusTok` | 核对当前控制器、路由、适配器、资源模型、快照和路由影响 |
| Sub2API 参考源 | `/opt/project/sub2api-main` | 核对真实用户路由、管理员账号路由、step-up 2FA、平台窗口额度和敏感导出 |
| New API 参考源 | `/opt/project/new-api-main` | 核对真实登录、2FA、用户、Token、分组、价格、模型和渠道路由 |
| all-api-hub 辅助参考 | `/opt/project/all-api-hub-main` | 核对 New API/Sub2API 浏览器采集、Refresh 和前端请求组合的辅助事实 |

当前仓库基线包含平台站点同步修复、渠道代理接入、旧版协议校准、2026-09-30
New API/Sub2API 旧版资源同步迁移和脱敏真实站点黑盒验收。本文不恢复旧版明文
凭据处理方式，也不把参考项目或真实站点中的运行凭据复制到当前项目。

### 1.2 证据等级

| 等级 | 含义 | 本文写法 |
| --- | --- | --- |
| A | 当前代码中有真实路由注册、请求构造、响应解析和持久化或测试证据 | “已实现”“当前调用” |
| B | 参考项目有真实路由、DTO、中间件或 Handler，但 NexusTok 仅部分适配 | “参考平台存在”“当前未完整接入” |
| C | 只有兼容字段、注释、名称或局部解析逻辑，没有端到端证据 | “待核查” |
| D | 需要真实部署、WAF、代理、版本或人工安全验证才能确认 | “需要真实站点验证” |

### 1.3 术语约定

- **管理地址**：用于登录、当前用户、余额、用量、分组、Token/Key 和管理员资源接口的站点根地址。
- **Relay 地址**：用于使用单个完整 Key 请求 `/v1/models` 或最终模型转发的 OpenAI 兼容地址。
- **账号级模型**：平台针对用户、分组或管理员渠道返回的模型目录。它不能自动证明每个 API Key 都具备同样能力。
- **Key 模型**：某一条完整 API Key 独立请求 Relay 模型接口得到的模型集合，或平台 Token/Key 记录明确声明的单 Key 模型限制。
- **完整 Key**：可实际作为 `Authorization` 凭据发送的未脱敏值。本文只用“完整 Key”描述，不展示其内容。
- **最近成功快照**：上一次完整成功同步持久化的身份、额度、Key、模型、倍率、权重和能力数据。

## 2. 资源对象和额度单位

### 2.1 账号余额

账号余额是平台账户当前可消费的余额，通常以 USD 或平台自身货币单位表示：

- New API 当前适配器从 `/api/user/self` 及兼容当前用户接口读取 `quota`、`balance`、`money` 或 `credit`，再结合 `/api/status` 的 `quota_per_unit` 将内部 quota 换算为展示余额。
- Sub2API 当前适配器从 `/api/v1/auth/me` 和 `/api/v1/user/profile` 读取 `balance`、`quota` 或 `credit`，按 USD 语义保存。
- 余额不是 NexusTok 本地用户余额，也不是当前请求的本地扣费结果。
- 当前 `PlatformSiteAccount.Balance`、`PlatformSiteIdentity.Balance` 和父 `Channel.Balance` 保存的是最近同步得到的上游账号余额。

### 2.2 账号累计已用额度

账号累计已用额度表示平台账号已经消费的上游额度，可能包含 NexusTok 以外发生的消费：

- New API 原始 `used_quota`、`quota_used`、`total_used` 等字段通常是内部 quota；当前适配器直接按内部 quota 保存，并不先除以 `quota_per_unit`。
- Sub2API 原始 `used_quota`、`quota_used`、`total_actual_cost` 或 `total_cost` 通常按 USD 读取，再乘以 `common.QuotaPerUnit` 转成 NexusTok 内部 quota。
- 如果 Sub2API 账号用量接口没有累计字段，旧版和当前实现都可以将同一轮成功读取的 Key 已用额度求和作为近似回退，并将资源标为 partial 或记录 warning。
- 当前 `Channel.UsedQuota` 对平台站点表示平台同步的上游累计用量；这与旧版 `CreateFromPreview` 把 `Channel.UsedQuota` 初始化为 `0` 的语义不同，详见第 12 节。

### 2.3 平台窗口额度

平台窗口额度是按时间窗口限制的用量和上限，不等于账号余额，也不等于单个 API Key quota。Sub2API 参考平台的用户路由返回以下字段：

| 字段 | 含义 |
| --- | --- |
| `platform` | 上游平台或窗口所属平台 |
| `daily_usage_usd` / `daily_limit_usd` | 日窗口已用和上限 |
| `daily_window_resets_at` | 日窗口重置时间 |
| `weekly_usage_usd` / `weekly_limit_usd` | 周窗口已用和上限 |
| `weekly_window_resets_at` | 周窗口重置时间 |
| `monthly_usage_usd` / `monthly_limit_usd` | 月窗口已用和上限 |
| `monthly_window_resets_at` | 月窗口重置时间 |

参考平台真实接口为 `GET /api/v1/user/platform-quotas`，当前 NexusTok Sub2API 适配器没有调用该接口，因此当前平台窗口额度不是已实现的完整资源字段。不能把 Key 的 `quota`、`quota_used` 或账号 `balance` 伪装成 5 小时、日、周、月窗口额度。

### 2.4 分组和倍率

- New API 分组来自 `/api/user/self/groups`、兼容 `/api/user/groups`，或派生站点的 `/api/groupPro/selectable`；倍率配置来自 `/api/ratio_config`，价格和 Endpoint 信息还可能来自 `/api/pricing`。
- Sub2API 分组来自 `/api/v1/groups/available`，用户实际倍率可由 `/api/v1/groups/rates` 补充或覆盖。
- Key 记录自身有倍率字段时优先使用 Key 倍率；没有时使用分组倍率。
- 当前 NexusTok 保存 `PlatformSiteGroup`、`UpstreamKey.SourceConversionRatio`、`ConversionRatio` 和父渠道转换倍率。路由权重基于有效转换倍率计算，不把分组倍率直接当作模型能力。

### 2.5 API Key 和 Key 状态

一个平台站点账号可以有多条 API Key。每条 Key 至少涉及：

- 外部 ID；
- 名称；
- 完整 Key 是否可读取；
- 脱敏 Key 预览；
- 所属分组；
- 上游状态；
- 已用额度；
- 剩余额度或无限额度；
- 过期时间；
- Key 模型；
- NexusTok 本地启用、手动禁用、自动禁用、缺失和可路由状态；
- 最近同步和最近使用时间。

当前 `UpstreamKey` 的完整凭据存放在 `SecretCiphertext`，管理接口通过 `KeyPreview` 返回脱敏值，不把 `SecretCiphertext` 或解密后的完整 Secret 返回给前端。

### 2.6 Key 剩余额度、已用额度、无限额度和过期时间

- New API Key 的 `used_quota` 是已用内部 quota；有限 Key 的 `remain_quota` 是剩余内部 quota；没有直接剩余值时可由 `quota` 和 `used_quota` 推导。
- New API `unlimited_quota`、`unlimitedQuota` 或 `unlimited` 为真时，剩余量和上限必须保持缺失，不能把上游 `0` 误解释成“额度耗尽”。
- Sub2API Key 的 `quota` 和 `quota_used` 按 USD 解释，`quota == 0` 表示无限；有限 Key 的剩余量为 `max(quota - quota_used, 0)`。
- Sub2API 若直接返回 `remain_quota`，当前适配器将其作为剩余 USD 再转换为内部 quota。
- `expires_at`、`expired_at`、`expire_at` 或 New API 的 `expired_time` 解析为时间。空值表示没有可确认的过期时间，不表示已过期。
- 路由只在剩余量明确为 `<= 0`、Key 明确过期、Key 没有真实模型能力或被禁用时排除；权限不足和资源读取失败不能转成“额度为零”。

### 2.7 Key 模型能力与 Relay 能力

模型能力分三层：

1. **平台账号级模型**：New API `/api/user/models` 等接口或 Admin channel 模型，适合诊断和展示。
2. **Token/Key 记录模型**：New API `model_limits`/`models`、Sub2API Key 的 `models` 等明确附着于单 Key 的字段。
3. **Relay 探测模型**：用某一条完整 Key 访问其 Relay 地址的 `/v1/models`，必要时回退 `/models`。

当前路由只允许使用第 2 或第 3 层确认的真实单 Key 能力。账号级目录不能复制给所有子 Key。父渠道模型是已确认子 Key 模型的并集，不是平台全局模型目录的无条件复制。

## 3. 旧版统一调用链

### 3.1 从管理员输入到平台快照

旧版入口和主要文件：

- `service/upstreamaccount/preview.go`
- `service/upstreamaccount/client.go`
- `service/upstreamaccount/newapi.go`
- `service/upstreamaccount/sub2api.go`
- `service/upstreamaccount/sub2api_endpoint.go`
- `service/upstreamaccount/types.go`

完整链路如下：

1. 管理员输入平台、管理地址、账号密码、Cookie、Access Token，或通过浏览器 Capture/Auth Flow 采集登录态。
2. `Preview` 规范化平台和认证方式，并优先尝试从已有渠道读取已保存的同步凭据。
3. `NewPlatformClient` 创建 `NewAPIClient` 或 `Sub2APIClient`。
4. New API/Sub2API 客户端先规范化管理地址；Sub2API 还会从页面 HTML 发现 Relay 地址。
5. 客户端尝试复用保存的 Session、Access Token、Refresh Token 或 Cookie；不能复用时执行密码登录。
6. 上游要求 2FA 时，`Preview` 创建短期 challenge，前端提交验证码后由 `CompletePreview2FA` 恢复 Cookie Jar 或临时会话继续请求。
7. 登录完成后读取用户身份、余额、账号用量、分组、倍率、Key 分页、Key 额度、Key 状态和 Key 模型。
8. Key 没有模型字段或模型字段为空时，`syncSnapshotKeyModels` 调用每条完整 Key 的 Relay 模型接口补齐模型。
9. `SavePreviewSnapshot` 将包含完整 Key 的快照放入 `upstream-account-preview` 缓存，但返回前端的副本由 `sanitizeSnapshot` 清空 `SyncedKey.Key`，只保留 `MaskedKey`。
10. 前端后续创建或刷新请求只提交 `preview_id` 及管理配置，不再次提交完整 Key。
11. `CreateFromPreview` 使用一次性 `ConsumePreviewRecord` 取出完整快照，创建父 `Channel` 和多个 `ChannelAccount`。
12. 刷新时 `RefreshChannelFromSnapshot` 将新快照与已有账号匹配，更新 Key、模型、分组、额度和同步元数据。

### 3.2 旧版预览缓存和凭据边界

- 缓存命名空间：`upstream-account-preview`。
- TTL：10 分钟。
- Redis 与进程内 Hybrid Cache 同时支持；进程内缓存只作为已有缓存实现的回退。
- `ConsumePreviewRecord` 使用消费锁并读取后删除，避免同一个包含完整 Key 的快照被重复创建。
- `sanitizeSnapshot` 不返回密码、Cookie、Access Token、Refresh Token、临时令牌、完整 Key 或 Auth Session。
- 完整 Key 只在后端内存、短期缓存和最终写库阶段存在；文档、普通响应和日志不应出现其内容。

### 3.3 旧版创建和刷新

`service/upstreamaccount/create.go`：

- `CreateFromPreview` 消费预览快照并开启数据库事务。
- 创建的父渠道使用账号池模式，父渠道模型和分组由启用账号的能力汇总。
- 旧版创建时 `Channel.UsedQuota` 固定初始化为 `0`，表示 NexusTok 本地消费累计，不直接写入上游账号累计用量。
- 每条 `ChannelAccount.UsedQuota` 可以写入该 Key 的已用额度换算值。
- 启用账号必须有完整 Key、模型和访问分组；这是旧版创建/刷新逻辑的约束，不能直接
  推断当前后台同步的成功判定。当前实现对已有成功快照的 Sub2API 账号允许在管理面
  资源成功、但所有单 Key 模型探测因余额不足或分组停用失败时更新身份、额度和 Key
  状态，保留最近成功模型能力并标记 `keys=partial`、`models=stale`；没有历史成功
  快照时仍要求真实单 Key 模型能力，不使用模型广场或分组目录替代。

`service/upstreamaccount/refresh.go`：

- `RefreshChannelFromCredential` 支持直接凭据同步或消费 `preview_id`。
- `RefreshChannelFromSnapshot` 在事务内加载已有账号。
- 匹配优先使用平台、管理地址/Relay 地址、外部 ID 等同步身份；没有同步元数据时再使用完整 Key digest。
- `DisableMissingKey` 只有在调用方明确要求且快照完整时才允许生效。
- 失败时不应凭空把未返回的 Key 当作已删除。

### 3.4 旧版后台刷新

`service/upstreamaccount/auto_sync.go` 的 `RunUpstreamAccountSync` 遍历自动同步渠道：

- 每个渠道独立开始、成功或失败；
- 单个渠道失败不会阻塞其它渠道；
- 日志只写渠道、阶段、数量、状态和脱敏错误；
- 成功时更新已同步账号和额度；
- 失败时保留已有账号配置和可用快照；
- 刷新匹配继续复用同步 ID、管理地址、Relay 地址和 Key digest。

## 4. 旧版 New API 完整请求表

### 4.1 响应 envelope 和公共请求行为

旧版 `service/upstreamaccount/client.go` 按 New API envelope 解包：

```json
{
  "success": true,
  "message": "...",
  "data": {}
}
```

公共行为：

- 请求使用管理地址作为根地址；
- JSON 请求使用 `Content-Type: application/json`；
- 已认证请求使用 `Authorization: Bearer <access-token>`；
- 需要派生站点用户标识时附加 `New-Api-User`；
- Cookie 登录沿用 Cookie Jar 和显式 Cookie；
- 只有兼容路径的 HTTP 404/405 才尝试下一条路径；
- 401、403、429、HTML/WAF、安全验证和网络错误不应触发无条件兼容重试。

### 4.2 New API 请求清单

| 阶段 | 方法与路径 | Query | Body | 认证 Header | 读取字段和用途 | 失败语义 |
| --- | --- | --- | --- | --- | --- | --- |
| 状态 | `GET /api/status` | 无 | 无 | 可带已有登录态 | `data.quota_per_unit` 或 `quotaPerUnit`；用于内部 quota 与 USD 换算 | 旧版失败使用 `500000` 默认值并记录 warning，不直接使账号认证失败 |
| 密码登录 | `POST /api/user/login?turnstile=` | `turnstile` 为空查询参数 | 主线 `{"username":"<username>","password":"<password>"}`；旧版还兼容 email 或混合字段 | 浏览器请求头、Cookie Jar | `data` 用户、Access Token、`require_2fa` | 业务失败或明确凭据错误结束登录；`require_2fa` 进入 challenge |
| 2FA | `POST /api/user/login/2fa` | 无 | `{"code":"<totp-code>"}` | 复用第一次登录的 Cookie Jar | 完成登录后返回用户和 Token | challenge 过期、验证码错误或 Cookie 丢失时失败，不重新猜测账号密码 |
| 当前用户主路径 | `GET /api/user/self` | 无 | 无 | Bearer、`New-Api-User`、Cookie | 用户 ID、用户名、邮箱、分组、`quota`、`used_quota` | 仅 404/405 才尝试兼容路径 |
| 当前用户兼容 | `GET /api/user/me`、`/api/user/profile`、`/api/user/info` | 无 | 无 | 同上 | 同上 | 非路由缺失错误不继续尝试 |
| 用户分组主路径 | `GET /api/user/self/groups` | 无 | 无 | Bearer、用户 Header | 分组名称、描述、ratio | 旧版可记录 warning 并继续使用 Key 自身分组；当前按资源状态记录 |
| 用户分组兼容 | `GET /api/user/groups` | 无 | 无 | 同上 | 同上 | 仅路由缺失回退 |
| 分组/倍率补充 | `GET /api/ratio_config` | 无 | 无 | 视站点要求 | `model_ratio`、`completion_ratio`、`cache_ratio`、`create_cache_ratio`、`model_price` | 旧版作为可选资源；失败不抹掉已读取 Key |
| 价格/Endpoint | `GET /api/pricing` | 无 | 无 | 视站点要求 | 分组、可用组、`supported_endpoint`、价格信息 | 当前用于端点能力和分组诊断，不作为单 Key 模型证明 |
| Token 第 1 页 | `GET /api/token/?p=1&page_size=100` | `p=1`、`page_size=100` | 无 | Bearer、用户 Header | Token ID、名称、分组、状态、`model_limits`/`models`、`remain_quota`、`used_quota`、`unlimited_quota` | 空页结束；总数不足时结束；旧版安全上限 1000 页 |
| Token 兼容分页 | `GET /api/token?p=<page>&page_size=100` 或 `GET /api/tokens?...` | 一基页码 | 无 | 同上 | 同上 | 仅 404/405 回退 |
| 批量完整 Key | `POST /api/token/batch/keys` | 无 | `{"ids":["<id>", "..."]}` | Bearer、用户 Header | `data.keys` 或 ID 到完整 Key 的映射 | 批量路由缺失时对缺失 ID 使用单条接口；其它错误保留失败状态 |
| 单条完整 Key | `POST /api/token/<id>/key` | 路径 ID | 无 | Bearer、用户 Header | `data.key` 或等价完整 Key 字段 | 只有 POST 404/405 时 GET 回退；空值或脱敏值不可用 |
| 单条兼容 Key | `GET /api/token/<id>/key` | 路径 ID | 无 | 同上 | 同上 | 其它状态码不继续猜测 |
| 账号级模型 | `GET /api/user/models` | 无 | 无 | Bearer、用户 Header | 账号可见模型目录 | 兼容 `/api/user/available_models`、`/api/user/available_model/`；只作账号诊断 |
| Relay 模型 | `GET /v1/models`，必要时 `GET /models` | 无 | 无 | 使用当前完整 Key 的 Bearer | 单 Key 真实模型列表 | 该 Key 单独失败只影响该 Key 模型确认，不等价于账号无 Key |

### 4.3 旧版 New API quota 换算

旧版 Token 记录中的 `remain_quota` 和 `used_quota` 是平台内部 quota，`/api/status` 的 `quota_per_unit` 用于转换：

```text
quota_limit_usd = (remain_quota + used_quota) / quota_per_unit
quota_used_usd = used_quota / quota_per_unit
quota_remaining_usd = remain_quota / quota_per_unit
```

如果 `unlimited_quota=true`：

- `quota_used_usd` 仍可保留；
- `quota_limit_usd` 和 `quota_remaining_usd` 置空；
- 不能把上游 `0` 显示为“剩余 0 美元”；
- 不能因为剩余字段缺失就标记 Key 耗尽。

## 5. 旧版 Sub2API 完整请求表

### 5.1 响应 envelope 和地址关系

旧版 `service/upstreamaccount/client.go` 按 Sub2API envelope 解包：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

旧版将管理地址和 Relay 地址分开：

- 管理地址请求登录、`auth/me`、Profile、Usage、Groups 和 Keys；
- 页面 HTML 中的 `api_base_url` 作为 Relay 地址候选；
- Relay 地址用于 Key 的 `/v1/models`；
- 若输入的是 `api.` 子域名，旧版会尝试去掉 `api.`，并探测管理接口；
- 管理地址和 Relay 地址必须同源或同注册域名，跨站点候选不发送凭据。

### 5.2 旧版 Sub2API 请求清单

| 阶段 | 方法与路径 | Query | Body | 认证 Header | 读取字段和用途 | 失败语义 |
| --- | --- | --- | --- | --- | --- | --- |
| 密码登录 | `POST /api/v1/auth/login` | 无 | 当输入是邮箱时依次尝试 `email/password`、`username/password`、混合字段；非邮箱输入使用 `username/password` | 页面 Origin、Referer、Cookie Jar | `access_token`、`refresh_token`、`expires_in`、用户和 2FA 状态 | 只有明确凭据错误或 HTTP 401 才尝试下一种主体；WAF、安全验证、限流、权限和网络错误立即停止 |
| 2FA | `POST /api/v1/auth/login/2fa` | 无 | `{"temp_token":"<temp-token>","totp_code":"<totp-code>"}` | 复用临时登录 Cookie | 正式 Access Token、Refresh Token、过期时间 | challenge 失效或验证码错误不当作 Key 缺失 |
| 刷新 | `POST /api/v1/auth/refresh` | 无 | `{"refresh_token":"<refresh-token>"}` | 可带 Cookie | 新 Access Token、新 Refresh Token、`expires_in` | 当前实现要求完整有效的 Token Pair；轮换结果不完整时标记不确定 |
| 当前用户 | `GET /api/v1/auth/me` | 无 | 无 | `Authorization: Bearer <access-token>` | 用户 ID、邮箱、用户名、余额 | 当前主路径；仅 404/405 才回退 Profile |
| Profile | `GET /api/v1/user/profile` | 无 | 无 | Bearer | 补充余额、邮箱、用户名 | 失败时保留 auth/me 的基础身份，作为可选资源 |
| 用户分组 | `GET /api/v1/groups/available` | 无 | 无 | Bearer | 分组 ID、名称、平台、倍率、描述 | 当前主要分组来源；失败影响分组资源，不应清空 Key |
| 用户分组倍率 | `GET /api/v1/groups/rates` | 无 | 无 | Bearer | 用户实际分组倍率映射 | 可选覆盖；失败保留 available 返回的倍率 |
| 账号用量主路径 | `GET /api/v1/usage/dashboard/stats` | 无 | 无 | Bearer | `total_actual_cost`、`total_cost`、当天用量 | 不同版本字段可能不同；缺少累计字段时 partial |
| 账号用量兼容 | `GET /api/v1/usage/stats` | 无 | 无 | Bearer | 同上 | 旧版和当前实现按可用字段回退 |
| Key 第 1 页 | `GET /api/v1/keys?page=1&page_size=100` | 一基 `page`、`page_size=100` | 无 | Bearer | ID、名称、完整/脱敏 Key、状态、分组、模型、`quota`、`quota_used` | 旧版和当前资源同步安全上限均为 1000 页；中途失败保留最近成功快照 |
| Key 详情 | `GET /api/v1/keys/<id>` | 路径 ID | 无 | Bearer | 完整 Key 或详情模型字段 | 详情失败只影响该 Key 的完整凭据/模型状态；不能清空旧 Key |
| Relay 模型 | `GET /v1/models`，回退 `GET /models` | 无 | 无 | 使用该 Key 的 Bearer | 单 Key 模型列表 | 该 Key 探测失败标记模型不可用或 partial，保留最近成功能力 |

### 5.3 旧版 Sub2API Key 额度

旧版 `fetchKeys` 的语义：

- `quota` 按 USD 解释；
- `quota_used` 按 USD 解释；
- 有限额度剩余为 `max(quota - quota_used, 0)`；
- `quota == 0` 视为无限额度；
- `models` 从 Key 列表记录读取；
- Key 列表没有完整 Key 或模型时，后续详情和 Relay 探测负责补齐；
- 账号累计用量优先使用 Usage 接口；Usage 缺失时将成功读取的 Key 已用 USD 求和作为近似回退，并标记 partial。

### 5.3.1 Sub2API Key 限流窗口字段

这组字段与第 5.4 节的用户级平台窗口额度不是同一个对象。参考平台把它们附着在
单个 API Key 的 DTO 上，当前源码中的字段定义位于
`backend/internal/handler/dto/types.go`、`backend/ent/apikey.go` 和
`backend/internal/handler/dto/mappers.go`：

| 字段 | 参考平台语义 | 旧版 NexusTok |
| --- | --- | --- |
| `rate_limit_5h`、`rate_limit_1d`、`rate_limit_7d` | 单 Key 在 5 小时、1 天、7 天窗口内的 USD 限额；`0` 表示不限流 | `sub2APIKey` 没有解析，未进入旧版快照 |
| `usage_5h`、`usage_1d`、`usage_7d` | 当前 5 小时、1 天、7 天窗口已使用的 USD；窗口过期后 DTO 映射会按有效窗口返回归零后的值 | 未解析，不能从旧版 `quota_used` 推断 |
| `window_5h_start`、`window_1d_start`、`window_7d_start` | 当前窗口起点 | 未解析 |
| `reset_5h_at`、`reset_1d_at`、`reset_7d_at` | 参考平台 DTO 根据窗口起点和固定窗口长度计算出的预计重置时间；不是 `quota` 的过期时间 | 未解析 |

因此，旧版 Sub2API 适配器实际获取的是 Key 总额度、已用额度、推导的剩余额度、
过期时间、状态和模型字段，不是完整的 5 小时/日/7 日限流窗口。`quota_used` 表示
Key 生命周期累计消耗，`usage_5h` 等表示当前限流窗口消耗，二者不能互换。

### 5.4 参考平台已有但旧版未完整接入的窗口额度

Sub2API 参考平台普通用户路由还提供：

```text
GET /api/v1/user/platform-quotas
```

它返回用户级平台窗口额度，而旧版 NexusTok `Sub2APIClient` 只读取 `auth/me`、Profile、Usage、Groups 和 Keys，没有把该接口纳入快照。因此：

- 旧版没有完整的平台窗口额度对象；
- 当前也不能从旧版字段推断平台窗口额度已接入；
- 该接口需要单独增加 DTO、资源类型、权限失败语义、窗口过期处理和快照回退后，才能视为实现。

## 6. 参考平台真实路由和权限核对

### 6.1 Sub2API 普通用户路由

真实注册位置：`/opt/project/sub2api-main/backend/internal/server/routes/user.go`。

认证组使用 JWT、中间件用户模式限制、全局面板限流和审计：

- `GET /api/v1/user/profile`
- `GET /api/v1/user/platform-quotas`
- `GET /api/v1/keys`
- `GET /api/v1/keys/:id`
- `GET /api/v1/groups/available`
- `GET /api/v1/groups/rates`
- `GET /api/v1/usage/stats`
- `GET /api/v1/usage/dashboard/stats`

`api_key_handler.go` 的 Handler 注释仍写着 `/api/v1/api-keys`，但真实注册组是 `v1.Group("/keys")`，实际路径为 `/api/v1/keys`。因此核对平台能力时必须以路由注册文件为准，不能只抄 Handler 注释。

### 6.2 Sub2API 管理员账号列表和敏感数据导出

真实注册位置：`/opt/project/sub2api-main/backend/internal/server/routes/admin.go`。

- `GET /api/v1/admin/accounts`：管理员账号列表，可分页、排序和按 `type=apikey` 筛选。
- `GET /api/v1/admin/accounts/data`：敏感账号数据导出，可能包含上游账号凭据原文，支持 `ids=<id>` 和 `include_proxies=false` 等查询参数。
- `GET /api/v1/admin/accounts/data` 注册时额外经过 `stepUpAuth`。

NexusTok 当前 Sub2API Admin Key 路径正是先读账号列表，再按账号 ID 读取 `accounts/data`，优先解析 `data.accounts[0].credentials.api_key`，再兼容其它字段。该流程只能描述字段和权限，不复制或输出真实账号数据。

### 6.3 Sub2API step-up 2FA 语义

真实实现：`/opt/project/sub2api-main/backend/internal/server/middleware/step_up.go`。

当 step-up 开关开启时，敏感导出必须同时满足：

1. 请求来自真人 JWT 会话；
2. Admin API Key 一律拒绝；
3. 当前用户已经启用 TOTP；
4. 当前会话近期完成过 TOTP step-up；
5. step-up 服务可用，否则 fail-closed。

失败响应包括 Admin API Key 禁止、TOTP 未启用、需要近期 step-up 和服务不可用等不同错误码。NexusTok 必须把这些情况记录为安全验证或权限失败，不能把空的 `accounts/data` 解释为“账号没有 Key”。

### 6.4 New API 用户、Token、分组、价格和渠道路由

真实注册位置：

- `/opt/project/new-api-main/router/api-router.go`
- `/opt/project/new-api-main/router/channel-router.go`

关键路由和中间件：

| 路由 | 注册事实 |
| --- | --- |
| `GET /api/status` | 公共状态接口 |
| `GET /api/pricing` | 受价格模块 Header 导航权限保护 |
| `GET /api/ratio_config` | 价格倍率接口 |
| `POST /api/user/login` | 限流、禁缓存、Turnstile 检查 |
| `POST /api/user/login/2fa` | 限流、禁缓存 |
| `POST /api/user/login/verify` | 兼容验证入口 |
| `POST /api/user/auth/refresh` | Session Cookie Origin Guard、限流、禁缓存 |
| `GET /api/user/groups` | 用户分组接口 |
| `GET /api/user/self` | 用户认证组 |
| `GET /api/user/self/groups` | 用户认证组 |
| `GET /api/user/models` | 用户认证组 |
| `GET /api/token/` | `UserAuth`、Token 操作审计 |
| `GET /api/token/:id` | `UserAuth`、Token 操作审计 |
| `POST /api/token/:id/key` | `UserAuth`、关键操作限流、禁缓存、审计 |
| `POST /api/token/batch/keys` | `UserAuth`、关键操作限流、禁缓存、审计 |
| `/api/channel/...` | AdminAuth、权限、RootAuth 或 SecureVerification 按路由保护 |

New API Handler 注释、兼容站点和真实注册路径也可能不同，核对必须沿 `api-router.go` 的实际 `apiRouter` 和 `tokenRoute` 挂载关系进行。

## 7. 当前 NexusTok 统一同步链路

### 7.1 Controller 和 Router 入口

当前真实路由注册在 `router/channel-router.go`，均先经过 `AdminAuth`，再经过细粒度权限：

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| `GET` | `/api/channel/:id/upstream-sync` | 查询站点同步状态 |
| `GET` | `/api/channel/:id/upstream-resources` | 查询身份、分组、端点、资源同步和额度 |
| `GET` | `/api/channel/:id/upstream-keys` | 查询脱敏 Key、额度、模型和可路由状态 |
| `POST` | `/api/channel/:id/upstream-sync` | 立即同步 |
| `POST` | `/api/channel/:id/upstream-resources/sync` | 立即资源同步 |
| `POST` | `/api/channel/:id/upstream-sync/queue` | 创建排队同步任务 |
| `PATCH` | `/api/channel/:id/upstream-keys/:keyId` | 修改 Key 优先级、倍率、权重或允许模型 |
| `POST` | `/api/channel/:id/upstream-keys/batch-status` | 批量启停 Key |
| `GET` | `/api/channel/fetch_models/:id` | 管理员触发模型获取 |

保存或更新平台站点配置后，Controller 会保存加密凭据，并按当前保存流程触发或允许后续同步。立即同步由 `SyncUpstreamSiteNow` 设置 30 秒上下文超时后调用 `service.SyncUpstreamSite`。

### 7.2 `SyncUpstreamSite` 到持久化

当前核心入口：`service/upstream_site.go`。

1. `SyncUpstreamSite` 调用 `syncPlatformSite`。
2. 按渠道 ID 获取同步锁，避免同一渠道并发刷新。
3. 读取 `PlatformSiteAccount`，解密 `CredentialCiphertext`。
4. 根据渠道 `setting.proxy`、HTTP 协议和连接分片创建平台站点 HTTP Client。
5. 根据 `Platform` 选择 `NewAPIAdapter` 或 `Sub2APIAdapter`。
6. 适配器认证：`password` 每轮直接使用账号密码登录，不读取已保存的 Session、
   Cookie、Access Token、Refresh Token 或 Dashboard Refresh；历史 `access_token`、
   `admin_key`、`cookie` 和浏览器 Capture 凭据才按各自认证方式使用已有登录态。
7. 将非密码认证的轮换凭据放入 `session.CredentialUpdate`；密码同步只把真实用户名、
   密码、可选 User ID、认证时间和正常状态加密回写，不保存本轮临时 Token、Cookie、
   Session ID 或过期信息。
8. 调用 `FetchSnapshot`，得到身份、余额、账号用量、分组、端点、Key、Key 模型和资源状态。
9. `persistPlatformSiteSnapshot` 开启事务，写入平台资源表、`UpstreamKey`、`UpstreamKeyAbility`、父渠道余额、父渠道用量和模型并集。
10. 刷新渠道缓存、Routing Key 和路由模型索引。
11. 失败时写入脱敏失败类别和资源级状态，但保留最近成功 Key、额度、模型、倍率、权重和能力。
12. 资源快照读取、部分分页失败和快照写库结束后，密码会话由适配器 `Cleanup` 尽力
    注销；Cleanup 失败只告警，不覆盖已获取快照或把账号标记为凭据失效。

当前安全边界：

- `PlatformSiteAccount.CredentialCiphertext` 加密保存站点凭据；
- `UpstreamKey.SecretCiphertext` 加密保存完整上游 Key；
- `SecretFingerprint` 使用 HMAC；
- 完整凭据只在后端解密和构造 HTTP 请求时短暂存在；
- 管理端只返回 `KeyPreview` 和资源状态；
- 日志只保存脱敏 URL、HTTP 状态、Content-Type、阶段、错误类别和回退信息。

### 7.3 手动同步、排队同步和后台任务

- 手动同步：`POST /api/channel/:id/upstream-sync`，同步完成后直接返回状态。
- 资源同步：`POST /api/channel/:id/upstream-resources/sync`，使用相同平台适配器和快照持久化。
- 排队同步：`POST /api/channel/:id/upstream-sync/queue` 创建 `SystemTask`，避免 HTTP 请求长期占用。
- 后台同步：`SyncUpstreamSites` 可以同步一个渠道或所有平台站点；单渠道错误归档，不阻塞其它渠道。
- 系统任务 Runner 负责租约、重试和任务状态，业务同步锁负责同一渠道的并发互斥。

### 7.4 当前管理端资源响应

`GET /api/channel/:id/upstream-keys` 返回 `UpstreamKeyResponse`：

- `KeyID`、`ID`、`ExternalID`、`Name`；
- `KeyPreview`，脱敏；
- `Models`、`AllowedModels`、`ModelsSynced`；
- `UsedQuota`、`RemainQuota`、`ExpiresAt`；
- `Status`、`DisabledReason`；
- `ConversionRatio`、`SourceConversionRatio`、`Weight`、`WeightOverride`；
- `Routable`、`AvailabilityReason`；
- `LastSyncAt`、`LastUsedAt`；
- `SnapshotOnly`、`CredentialUnavailable` 和健康样本字段。

该接口不返回完整 Secret。`GET /api/channel/:id/upstream-resources` 还返回管理地址、Relay 地址、身份、分组、端点、资源同步状态、账号余额和账号累计用量。

## 8. 当前 New API 适配器链路

### 8.1 认证方式和请求头

实现主要位于：

- `service/upstream_site_adapters.go`
- `service/platform_site_auth_flow.go`
- `model/upstream_channel.go`

当前支持：

- `password`
- `access_token`
- `admin_key`
- `cookie`

请求头按场景组合：

- `Authorization: Bearer <access-token-or-admin-key>`；
- `New-Api-User`；
- 派生站点兼容用户头：`New-API-User`、`X-ModelFlare-User`、`Veloera-User`、`X-Api-User`、`voapi-user`、`User-id`、`Rix-Api-User`、`neo-api-user`；
- `Cookie`；
- `X-Auth-Session`；
- Admin Key 模式额外使用 `x-api-key` 和 `New-Api-Key`。

2026-09-30 旧版迁移后，New API 密码登录恢复兼容主体顺序：主请求仍为
`username/password`，当管理员输入看起来是邮箱时，只在前一轮明确属于凭据错误或
HTTP 401 时继续尝试 `email/password` 和混合主体。安全验证、WAF、限流、权限不足、
网络错误、非 401 HTTP 错误和普通资源失败不会触发登录主体重放。

### 8.2 Dashboard Auth Bundle 和 Refresh Cookie

现代 New API Dashboard Refresh 使用：

```text
POST /api/user/auth/refresh
Cookie: new_api_refresh=<opaque-cookie>
X-Auth-Session: <session-id>
Authorization: Bearer <current-access-token>
```

Refresh Cookie 不放入 JSON Body。当前适配器严格要求现代 Bundle 同时满足：

- 顶层 `success == true`；
- `data.access_token` 存在；
- `data.token_type == "Bearer"`；
- `data.access_expires_at` 尚未过期；
- `data.session.sid` 存在；
- `data.session.current == true`；
- `data.user` 存在且能解析用户 ID。

如果响应被识别为现代 Bundle 但结构不完整，不能降级成旧的宽松 Token 解析。`new_api_refresh` Cookie 的同名值必须轮换；没有轮换或轮换结果不确定时：

- 设置 `RefreshUncertain`；
- 设置 `RefreshStatus=uncertain_rotation` 或 `ReauthRequired`；
- 不安全重放旧会话；
- 保留最近成功资源快照；
- 管理端看到需要重新认证或 Refresh 不确定，而不是 Key 不存在。

### 8.3 New API 当前资源请求和转换

当前 `NewAPIAdapter.FetchSnapshot` 的顺序是：

1. `GET /api/status`，读取 `quota_per_unit`，失败使用默认 `500000` 并将失败写入 usage 资源状态。
2. 读取 `/api/user/self`，兼容 `/api/user/me`、`/api/user/profile`、`/api/user/info`。
3. 解析用户身份、余额和账号累计已用 quota。
4. 读取 `/api/user/self/groups`、`/api/user/groups` 或 `/api/groupPro/selectable`。
5. 读取 `/api/ratio_config` 和 `/api/pricing`，建立分组、价格和 `supported_endpoint` 诊断。
6. 从 `GET /api/token/` 的 `p=1&page_size=100` 开始分页，兼容 `/api/token` 和 `/api/tokens`，Token 资源独立上限为 1000 页。
7. 对缺少完整 Key 的 Token 优先批量 `POST /api/token/batch/keys`，然后逐条 `POST /api/token/<id>/key`，只有 404/405 才 `GET` 回退。
8. Token 的 `model_limits`/`models` 优先作为该 Key 模型；没有模型时使用该 Key 调用 Relay `/v1/models` 或 `/models`。
9. 读取账号级 `/api/user/models`，兼容 `/api/user/available_models`、`/api/user/available_model/`，只作为账号模型诊断和父资源展示，不复制给所有 Token。
10. 对每个 Key 生成 `UpstreamKeySnapshot`，保存额度、过期、状态、分组、倍率和模型同步状态。
11. 资源分页或单 Key 模型部分失败时，将 Key/Models 资源标为 partial、stale 或 `secure_verification_required`，不清空旧快照。

### 8.4 New API quota、USD 和内部 quota

当前 New API 有三种数值语义：

| 层次 | 典型字段 | 当前含义 |
| --- | --- | --- |
| 上游内部 quota | `quota`、`used_quota`、`remain_quota`、`quota_used` | 按平台内部单位读取；当前 `UsedQuota`/`RemainQuota` 直接作为 NexusTok 内部 quota |
| USD 展示余额 | 账号 `quota` 经 `normalizeNewAPIQuota` | 只有账号余额展示使用 `quota_per_unit` 做除法 |
| NexusTok 内部 quota | `common.QuotaPerUnit` 体系 | Key 和账号累计用量进入本地计费、路由和管理模型的整数 quota 字段 |

当前适配器不会把 New API Key 的 `used_quota` 再除一次 `quota_per_unit`。如果文档、页面或上游字段明确提供 USD 语义，则必须由对应适配器转换，而不能仅根据字段名猜测。

### 8.5 New API Admin Key 和账号级模型边界

New API Admin Key 可以附带：

- `/api/channel/`
- `/api/channel/<id>`
- `/api/channel/fetch_models/<id>`

当前适配器将 Admin channel 模型并入站点诊断模型集合，并仍然通过 Token/Key 自身字段或该 Key 的 Relay 探测确认单 Key 能力。Admin channel 全局模型不能复制给每条 Key。

## 9. 当前 Sub2API 适配器链路

### 9.1 管理地址、Relay 地址和页面发现

当前实现同时位于：

- `service/upstream_site_adapters.go`
- `model/upstream_channel.go`
- `service/upstream_site.go`

地址处理顺序：

1. `normalizeSub2APIBaseURL` 去掉明确的 `/login`、`/dashboard`、`/register`、`/setup`、`/home` 页面后缀，不破坏未知反向代理路径前缀。
2. 从管理页面 HTML 中读取 `api_base_url`，作为 Relay 地址候选。
3. 如果输入是 `api.` 子域名，尝试恢复去掉 `api.` 的管理地址。
4. 恢复候选时要求相同协议、相同端口，并且页面明确回指原始 API 地址。
5. 页面声明的 Relay 地址必须与最终 HTML 页面来源或原始管理地址使用相同协议、主机
   和有效端口，或与原始管理地址构成严格的直接 `api.` 父子域关系；localhost、IP、
   跨来源且未经页面明确声明的地址不发送凭据。
6. `NormalizeSub2APIRelayBaseURL` 去掉结尾 `/v1`，因为 Relay 请求自身会追加 `/v1`。

因此：

- `PlatformSiteAccount.BaseURL` 保存管理地址；
- `PlatformSiteAccount.RelayBaseURL` 保存页面发现或验证后的 Relay 地址；
- 管理接口通常使用管理地址；若页面重定向后的最终 HTML 来源与明确声明的 Relay
  同源，本轮请求根地址切换到该最终可信来源，持久化管理地址不变；
- `/v1/models` 和最终 Relay 请求使用 Relay 地址；
- 管理地址缺失或 Relay 发现失败时，不把一方无条件当成另一方。
- HTML 请求保留最终响应 URL，相对 `api_base_url` 按最终页面地址解析。管理页面从
  `hhw1231.com` 跳转到 `hengwenapi.com` 时，若最终页面明确声明
  `https://hengwenapi.com`，该地址可作为 Relay；本轮登录、当前用户、分组、额度和
  Key 请求直接使用最终可信页面来源，持久化的管理地址仍为 `hhw1231.com`。
- `aiapipay.com` 页面明确声明 `api.aiapipay.com` 时，直接 `api.` 父子域关系允许
  Relay；管理请求仍使用管理地址。Sub2API Relay 模型探测只发送当前完整 Key 的
  `Authorization: Bearer`，不发送 `x-api-key`，清空 Cookie Jar，不复制管理 Cookie、
  管理 Bearer、Origin、Referer、`X-Requested-With` 或 `X-Auth-Session`。

### 9.2 当前认证和 Refresh

密码登录：

```text
POST /api/v1/auth/login
Body: {"email":"<email>","password":"<password>"} 或 {"username":"<identity>","password":"<password>"}
```

当前实现按参考源真实 DTO 收敛请求：当输入是邮箱时首请求只发
`email/password`，仅在明确凭据错误或 HTTP 401 后最多一次尝试
`username/password`，不发送混合主体；非邮箱输入只发 `username/password`。首路由
`/api/v1/auth/login` 只有明确返回 404/405 时才回退 `/auth/login`。400
`INVALID_REQUEST`、403、429、WAF、安全验证、权限拒绝、网络错误和 5xx 不触发路由或
主体重试。只有上游响应明确要求条款时，才读取 `/api/v1/settings/public` 并使用相同
路由、相同主体携带 `agreed_revision` 重试一次；普通登录成功时不主动读取公开设置。
成功后从响应中读取 Access Token；登录要求交互验证时立即停止。

Access Token：

- 有 Refresh Token 时调用 `POST /api/v1/auth/refresh`；
- Body 为 `{"refresh_token":"<refresh-token>"}`；
- 需要新的 Access Token、Refresh Token 和有效 `expires_in`；
- 新 Refresh Token 缺失时不安全复用旧 Refresh Token，标记重新认证或不确定；
- 没有 Refresh Token 时直接使用尚未过期的 Access Token。

Admin Key：

- `Authorization: Bearer <admin-key>`；
- `x-api-key: <admin-key>`；
- 读取管理员账号列表和敏感账号数据。

Cookie：

- 发送显式 Cookie 和 Cookie Jar；
- Cookie 只能作为会话材料，不等于已确认的完整 Key；
- WAF、HTML 或安全验证响应必须独立分类。

### 9.3 当前资源请求顺序

当前 `Sub2APIAdapter.FetchSnapshot` 主要按以下顺序读取：

1. `GET /api/v1/auth/me`，404/405 时兼容 `/api/v1/user/profile`。
2. 读取余额、账号身份和账号累计用量。
3. 尝试 `GET /api/v1/user/profile` 补充余额、邮箱、用户名和用量字段。
4. `GET /api/v1/groups/available` 和 `GET /api/v1/groups/rates` 读取分组和倍率；`available` 失败是核心资源失败，停止 Key 同步并保留快照。
5. 先读 `GET /api/v1/usage/dashboard/stats`，缺少累计用量字段时再按条件回退 `GET /api/v1/usage/stats`；字段缺失标记 partial，不伪造 0。
6. 普通用户使用 `GET /api/v1/keys?page=<page>&page_size=100` 分页，资源上限为 1000 页。
7. Admin Key 使用 `GET /api/v1/admin/accounts?page=<page>&page_size=100&sort_by=name&sort_order=asc&type=apikey`，资源上限为 1000 页。
8. 列表已返回完整 Key 时不读取详情；缺失或掩码时普通 Key 读取 `GET /api/v1/keys/<id>`，Admin Key 读取 `GET /api/v1/admin/accounts/data?ids=<id>&include_proxies=false`。
9. Key 列表或详情没有模型字段时，才使用完整 Key 请求 Relay `/v1/models`，必要时 `/models` 回退。
10. `fetchSub2APIModels` 当前直接返回 `nil`，当前没有独立的 Sub2API 账号级模型接口实现，不能把账号级目录复制给所有 Key。
11. 将 Key 级模型并入父渠道模型集合，保存为 `UpstreamKeyAbility`。

### 9.4 Sub2API Key 额度和窗口额度边界

当前 `sub2APIQuotaToInternal` 把上游 USD 乘以 `common.QuotaPerUnit` 并使用安全的 quota rounding。Key 读取：

- `quota_used`、`used_quota` 等转为 `UsedQuota`；
- `quota` 与 `quota_used` 推导有限 Key 的 `RemainQuota`；
- `unlimited`、`unlimited_quota` 或 `unlimitedQuota` 为真时 `RemainQuota=nil`；
- 负剩余量钳制为零；
- 过大、NaN、Inf 或负数被视为响应无效。

当前没有读取或持久化 Sub2API Key 的 `rate_limit_5h`、`rate_limit_1d`、
`rate_limit_7d`、`usage_5h`、`usage_1d`、`usage_7d`、窗口起点和重置时间。
因此当前 `UpstreamKey.RemainQuota` 只表示总额度剩余，不表示 5 小时、日或 7 日
限流窗口剩余；当前也没有把这些字段用于路由过滤或可用权重计算。

当前未调用：

```text
GET /api/v1/user/platform-quotas
```

所以当前 Sub2API 适配器已实现的是账号余额、账号累计用量和 Key 额度，不是平台窗口额度。

### 9.5 Sub2API 模型边界

- 当前没有账号级模型读取实现，`fetchSub2APIModels` 返回 `nil`。
- Key 列表或 Key 详情中的 `models` 字段优先。
- 缺少模型字段时，当前主要通过完整 Key 调用 Relay `/v1/models`。
- 单 Key Relay 失败只把该 Key 标记为模型能力不可用或保留最近成功模型，不影响其它 Key。
- 不能把 Sub2API 管理页面、分组或平台窗口额度中的模型信息复制给所有 Key；分组
  模型目录不能替代单 Key 能力确认，未确认模型的 Key 不进入可路由集合。

## 10. Key 生命周期和刷新匹配

### 10.1 分页上限

| 实现 | 单页大小 | 最大页数 | 超过上限 |
| --- | ---: | ---: | --- |
| 旧版 New API | 100 | 1000 | 返回分页错误 |
| 旧版 Sub2API | 100 | 1000 | 返回分页错误 |
| 当前 New API Token 资源 | 100 | 1000 | 返回分页错误并保留最近成功快照 |
| 当前 Sub2API 普通 Key 资源 | 100 | 1000 | 返回分页错误并保留最近成功快照 |
| 当前 Sub2API Admin Key 资源 | 100 | 1000 | 返回分页错误并保留最近成功快照 |
| 当前其它管理资源分页 | 100 | 100 | 按各自资源限制返回错误或停止 |

2026-09-30 迁移后，Token/Key 资源已恢复旧版 1000 页独立上限；Admin channel
等非本次 Key 资源分页继续沿用原独立限制。分页响应的 `total`、`pages`、
`page_size` 和当前页数量共同决定是否继续，不能把第一页空列表等同于整站没有 Key，
除非平台明确表示总数为零且分页完整。

### 10.2 完整 Key 获取优先级

New API：

1. Token 列表中已存在且不含脱敏标记的完整 Key；
2. `POST /api/token/batch/keys`；
3. 对批量响应缺失的 ID 调用 `POST /api/token/<id>/key`；
4. 仅 404/405 时对单条接口使用 GET 回退；
5. 仍为空、被掩码、权限不足、安全验证或响应失败时标记该 Key Secret 不可用。

Sub2API：

1. `/api/v1/keys` 列表中已存在的完整 Key；
2. `GET /api/v1/keys/<id>` 详情；
3. Admin Key 模式的 `/api/v1/admin/accounts/data`；
4. 仍为空或掩码时标记 Secret 不可用；
5. 已有最近成功 Secret 时保留旧记录，不把本轮失败当成删除。

### 10.3 存储、指纹和管理响应

- 站点凭据写入 `PlatformSiteAccount.CredentialCiphertext`；
- 子 Key 写入 `UpstreamKey.SecretCiphertext`；
- `SecretFingerprint` 使用 HMAC，刷新匹配不依赖明文索引；
- `KeyPreview` 只用于管理展示；
- 完整 Key 不进入普通 JSON 响应、日志、错误信息和文档；
- Relay 请求前才短暂解密，并在请求结束后不持久化明文。

### 10.4 外部 ID、地址和 digest 匹配

当前 `persistPlatformSiteSnapshot` 和路由模型通过以下信息保持 Key 身份：

1. 平台类型；
2. 管理地址；
3. Sub2API Relay 地址；
4. 上游外部 ID；
5. 完整 Key 的 HMAC/摘要；
6. 数据库中的 `UpstreamKey.ExternalID`、`RoutingKeyID` 和 `SecretFingerprint`。

Sub2API 管理地址和 Relay 地址发生规范化变化时，仍尝试使用两个地址对应的同步身份匹配；外部 ID 变化而完整 Key 未变时，digest 可作为回退。

### 10.5 何时可以判定 Key 缺失

只有下列条件同时满足，才允许把“本次未返回”的旧 Key 标为 missing：

- 全部 Key 分页成功；
- 分页总数和结束条件已确认；
- 批量/详情 Key 读取达到当前校验要求；
- 需要探测的模型资源已明确完成或明确判定；
- 没有权限不足、安全验证、WAF、网络错误或 Refresh 不确定；
- `snapshot.KeysComplete == true`。

Key 详情失败、单 Key 模型探测失败、Admin step-up 拒绝、分页中途失败或 Refresh 结果不确定时，必须保留旧 Key，不得执行 missing 判定。

## 11. 失败语义和最近成功快照

### 11.1 资源状态

当前资源表 `PlatformSiteResourceSync` 使用：

- `success`：本次资源完整成功并写入新值；
- `failed`：本次请求或响应失败；
- `partial`：部分记录成功，部分详情、模型或统计字段失败；
- `secure_verification_required`：需要人工安全验证、step-up、Turnstile、验证码或上游安全证明；
- `stale`：本次没有新鲜确认，继续使用最近成功值。

`UsingSnapshot=true` 表示当前管理和路由读取的是最近成功快照，不代表本轮请求成功。

### 11.2 失败分类

| 失败类别 | 例子 | 是否等同于 Key 不存在 | 当前处理 |
| --- | --- | --- | --- |
| 认证失败 | 明确账号密码错误、Access Token 失效 | 否 | 标记 `credentials_invalid` 或重新认证，保留最近成功资源 |
| Refresh 不确定 | Cookie 没轮换、缺少新 Refresh Token、Bundle 不完整 | 否 | 标记 `RefreshUncertain`/`reauth_required`，不重放旧会话 |
| WAF/HTML | 返回登录页、Turnstile、WAF HTML | 否 | 标记交互或 WAF，保留旧快照 |
| 安全验证 | Sub2API step-up、管理员敏感导出需要 TOTP | 否 | `secure_verification_required`，不清空旧 Key |
| 权限不足 | 401/403 访问资源或 Admin API | 否 | 资源级 failed/partial，保留旧 Key、额度、模型 |
| 路由缺失 | 404/405 兼容路径缺失 | 否 | 只尝试明确允许的兼容路径；仍缺失则 stale |
| 部分页失败 | 第 2 页失败、总数无法确认 | 否 | `partial`/`failed`，禁止 missing 判定 |
| Key 详情失败 | 单条 `/key`、`keys/:id` 失败 | 否 | 该 Key 变为 Secret 不可用或使用旧记录 |
| 单 Key 模型失败 | `/v1/models` 超时、空列表、403 | 否 | 该 Key 模型能力失败；保留旧模型，不复制账号模型 |
| 传输失败 | DNS、TLS、代理、超时 | 否 | 记录网络/代理失败，等待重试，保留最近成功快照 |

### 11.3 持久化和路由回退

当前 `syncPlatformSite` 在认证成功但资源阶段失败时可以先落身份、余额或资源状态，再保留旧 Key/模型。`persistPlatformSiteResources` 对非 `success` 资源设置 `UsingSnapshot`，资源查询能同时显示本次失败和最近成功时间。对于已有 `last_sync_at` 的 Sub2API 渠道，如果管理面和 Key 列表成功、但所有单 Key `/v1/models` 因 `INSUFFICIENT_BALANCE`、`GROUP_DISABLED` 等资源条件失败，本轮父渠道仍可标记同步成功；Key 资源为 `partial`、模型资源为 `stale`，旧模型能力继续路由。没有历史快照的新渠道仍保持失败，避免把未确认模型加入路由。

父站点是否可路由由以下事实共同决定：

- 凭据是否仍可解密；
- `SyncStatus` 是否为 `success`，或 `failed/running` 但存在 `LastSyncAt`；
- 子 Key 是否启用；
- 子 Key 是否有完整 Secret；
- `ModelsSynced` 和有效模型是否存在；
- Key 是否过期或额度耗尽；
- Key 是否被管理员手动禁用。

失败轮次不会自动删除旧 Key、旧额度、旧模型、旧倍率、旧权重或 `last_sync_at`。没有任何成功快照的 `idle` 或首次失败站点不可路由。

### 11.4 安全验证和权限不足的特别边界

Sub2API Admin Key 读取 `/api/v1/admin/accounts/data` 时，step-up 拒绝、TOTP 未启用或真人会话条件不满足，都表示“当前凭据没有权限完成敏感读取”，不是“平台没有 API Key”。

New API `/api/token/batch/keys`、单条 Key、账号级模型或 Admin channel 读取失败时，也不能把脱敏列表或部分列表解释成不存在完整 Key。只有明确返回空且分页完整、权限有效、资源成功的场景，才可以将空结果作为平台当前确实没有可用 Key 的事实。

## 12. 旧版与当前版逐项差异矩阵

| 维度 | 旧版 NexusTok | 当前 NexusTok | 影响 |
| --- | --- | --- | --- |
| 数据模型 | `Channel` + 多个 `ChannelAccount`；预览完整快照保存在短期缓存 | `PlatformSiteAccount`、`PlatformSiteIdentity`、`PlatformSiteGroup`、`PlatformSiteEndpoint`、`PlatformSiteResourceSync`、`UpstreamKey`、`UpstreamKeyAbility` 和 `RoutingKey` 分层保存 | 当前可独立显示资源状态和回退来源 |
| 统一同步入口 | `service/upstreamaccount/Preview`、`CreateFromPreview`、`RefreshChannelFromSnapshot` | `controller/upstream_channel.go` + `service.SyncUpstreamSite` + 平台适配器 | 当前支持手动、资源级、排队和后台同步 |
| 认证方式 | 密码、Cookie、Access Token、浏览器采集；旧版认证会话放入旧版快照结构 | `password`、`access_token`、`admin_key`、`cookie`，凭据整体加密保存 | 当前机器凭据和站点凭据边界更明确 |
| New API 2FA | challenge 缓存 Cookie，`/api/user/login/2fa` 后继续读取快照 | Auth Flow 和适配器都支持 2FA；还严格识别 Dashboard Bundle | 当前能区分 Bundle 不完整、Refresh 不确定和普通认证失败 |
| Sub2API Refresh | 保存 Access/Refresh/Expires，旧版可在 Session 过期时刷新 | 要求新 Token Pair 和有效过期；轮换不完整时 `RefreshUncertain` | 不安全重放旧 Refresh Token |
| 管理地址与 Relay 地址 | Sub2API 页面 `api_base_url` 发现和 `api.` 恢复已存在 | `BaseURL`、`RelayBaseURL` 分字段落库，并加强同源/同注册域名校验 | 管理请求与 Relay 请求不再混用 |
| 账号余额 | 快照 Balance 转写父渠道或账号 | 写入 `PlatformSiteAccount`、Identity 和父渠道，并记录更新时间 | 管理端可查看来源和快照时间 |
| 账号累计用量 | 旧版快照可展示上游用量，但创建时父 `Channel.UsedQuota=0` | 快照上游累计用量写入 `PlatformSiteAccount.UsedQuota`、Identity 和 `Channel.UsedQuota` | 当前父渠道字段不再只表示本地消费累计 |
| Sub2API 用量缺失 | Key 已用额度求和作为近似回退并标 partial | 保留该回退，并保存资源级状态和最近成功值 | 不把缺失统计伪装成 0 |
| 平台窗口额度 | 未完整读取 `/api/v1/user/platform-quotas` | 仍未调用该接口 | 5 小时、日、周、月窗口额度未覆盖 |
| Sub2API Key 限流窗口 | 参考平台存在 `rate_limit_*`、`usage_*`、窗口起点和重置字段，但旧版未解析 | 当前同样只读取总 quota、已用/剩余 quota、过期时间和模型；未持久化 Key 限流窗口 | 不能用 `RemainQuota` 代替 5 小时、日或 7 日窗口剩余 |
| Key 列表 | New API/Sub2API 均最多 1000 页，单页 100 | 2026-09-30 已恢复 New API Token、Sub2API 普通 Key 和 Admin Key 资源最多 1000 页；其它管理分页继续使用独立限制 | 超大账号仍需持续容量验证 |
| New API 完整 Key | 批量 Key 后逐条补偿，单条兼容 GET | 恢复列表完整值优先、批量 Key、缺失 ID 单条 POST、POST 404/405 后 GET 回退；掩码和空值不可写入 | 部分失败不再覆盖旧 Secret |
| Sub2API 完整 Key | 用户 Key 详情和 Admin accounts/data | 列表完整 Key 优先；缺失或掩码时用户 `/keys/:id`、Admin `/admin/accounts/data`，安全验证单独分类 | 权限失败不判定 Key 缺失 |
| Key 模型 | Token/Key 字段优先，缺失时逐 Key Relay `/v1/models` | 保留并落库 `UpstreamKeyAbility` | 当前路由按单 Key 能力过滤 |
| New API 账号模型 | 可从 `/api/user/models` 读取 | 可读取，但仅账号诊断和展示 | 不能复制给所有 Key |
| Sub2API 账号模型 | 没有完整接入 | `fetchSub2APIModels` 返回 `nil` | 当前主要依赖单 Key Relay 探测 |
| 管理端 Key 返回 | 旧版预览响应只返回脱敏 Key | `upstream-keys` 返回 `KeyPreview`，不返回完整 Secret | 防止完整凭据进入前端 |
| 加密和指纹 | 旧版使用渠道账号池和同步元数据 | 站点凭据、Key Secret 加密，指纹 HMAC | 当前刷新匹配不依赖明文 Key |
| 失败快照 | 旧版刷新失败保留已有账号，完整分页边界较少分层 | 资源类型独立状态，最近成功快照可路由 | 权限/WAF/部分失败不清空资源 |
| 后台同步 | `RunUpstreamAccountSync` 按渠道逐个刷新 | SystemTask 排队、同步锁、手动和后台统一入口 | 当前任务状态可查询、失败可重试 |
| 路由调度 | `ChannelAccount` 模型和访问组参与账号池路由 | `UpstreamKey`、`UpstreamKeyAbility`、Routing Key、额度和健康状态参与路由 | 子 Key 具备稳定全局身份 |
| 父渠道模型 | 从启用同步账号能力汇总 | 从启用且 `ModelsSynced` 的 `UpstreamKey` 有效模型并集汇总 | 平台全局模型不再污染父渠道路由能力 |

### 12.1 父渠道 `UsedQuota` 的特别说明

旧版 `service/upstreamaccount/create.go` 明确将新建父 `Channel.UsedQuota` 设为 `0`，而 `ChannelAccount.UsedQuota` 可以保存上游 Key 已用量。

当前 `service/upstream_site.go:persistPlatformSiteSnapshot`：

1. 优先使用快照中明确的账号级 `UsedQuota`；
2. 明确返回 `0` 时仍视为有效值；
3. 如果账号级用量没有设置，且本轮 Key 有已用额度，则安全求和作为回退；
4. 将结果写入 `PlatformSiteAccount`、`PlatformSiteIdentity` 和父 `Channel`；
5. 子 Key 仍保存自己的 `UsedQuota`。

因此当前父渠道 `UsedQuota` 是“最近一次平台同步的上游累计用量”，不应被文档或管理页面解释为单纯的 NexusTok 本地 Relay 消费日志累计。两者需要不同字段或不同查询来源。

## 13. 当前未覆盖能力和真实站点验证事项

### 13.1 当前未覆盖或不完整

- Sub2API `/api/v1/user/platform-quotas` 的日、周、月或其它窗口额度；
- Sub2API API Key 的 `rate_limit_5h`、`rate_limit_1d`、`rate_limit_7d`、`usage_*`
  和窗口重置时间；
- Sub2API 账号级模型目录；
- 将平台窗口额度与 Key quota、账号余额统一展示的资源模型；
- 所有真实站点版本的字段别名、分页 envelope 和权限差异；
- 需要真人 JWT + TOTP step-up 的敏感账号导出自动完成；
- New API Passkey、Security Proof、Turnstile 和 WAF 的自动完成；
- 不同反向代理路径前缀下的管理地址和 Relay 地址发现；
- 站点返回空模型但实际转发可用，或 `/models` 与 `/v1/models` 语义不一致的部署差异。

### 13.2 需要真实站点验证的结论

以下结论当前主要来自静态源码、脱敏 HTTP fixture 和本地数据库测试，不能替代真实站点验证：

- New API 各派生版本是否接受 `username/password`、`email/password` 或混合登录 Body；
- New API Dashboard Refresh 是否始终轮换 `new_api_refresh` Cookie；
- New API `access_expires_at`、`session.sid` 和 `session.current` 是否在所有部署版本中存在；
- Sub2API 当前部署是否要求页面发现的 `api_base_url`；
- `api.` 子域名、管理域名和 Relay 域名是否满足同注册域名校验；
- Sub2API Admin accounts/data 是否必须真人 step-up；
- 平台不同版本的 Key 详情字段、模型字段和 unlimited 字段；
- 1000 页资源上限在超大真实账号中的容量和耗时表现；
- 代理、WAF、HTML 登录页、TLS 和跨域 Cookie 的真实行为；
- 上游接口返回 200 业务失败时的错误码和资源影响范围。

真实验证必须使用专门的测试站点、脱敏凭据和最小权限，不得把真实运行凭据写入 NexusTok 代码、文档、测试、日志或提交。

### 13.3 2026-09-30 真实站点黑盒验收

本次验收使用两个专用测速站点和独立隔离浏览器会话完成，只记录脱敏后的路径、状态、
资源数量和失败分类；没有保存截图、网络捕获文件、响应正文、Cookie、Access Token、
Refresh Token、Admin Key、完整上游 Key 或测试账号凭据。验收只执行登录、资源读取、
Key 详情和 `/v1/models` 探测，未调用聊天、补全、图片、视频或其它会产生费用的接口。

New API 站点验收结果：

- 匿名入口返回 HTTP 200，响应头暴露版本 `v1.0.0-rc.19-i18nfix.2`；
- 浏览器真实路径完成首页、登录、Overview、API Keys、Model Analytics、Usage Logs
  和 Profile；观察到 `POST /api/user/login?turnstile=`、`GET /api/user/self`、
  `GET /api/status`、`GET /api/user/self/groups`、`GET /api/token/?p=1&size=10`
  和 `GET /api/user/models` 均返回 HTTP 200；
- NexusTok 适配器只读同步认证成功，读取到 11 条 Key，聚合模型 32 个；
- 部分单 Key `/v1/models` 返回 HTTP 403，因此 Key 和模型资源进入
  `secure_verification_required` 或旧快照保护状态，`KeysComplete=false`，
  但已成功读取身份、余额、分组、倍率、Token 列表、批量 Key 和可访问 Key 的模型。

Sub2API 站点验收结果：

- 匿名入口返回 HTTP 200，页面配置包含 `api_base_url` 指向同站 Relay 地址；页面配置
  显示 Turnstile 未启用，Profile 页面显示当前测试会话未启用 TOTP；
- 浏览器真实路径完成服务条款接受、登录、Dashboard、API Keys、Usage 和 Profile；
  观察到 `POST /api/v1/auth/login`、`GET /api/v1/auth/me`、
  `GET /api/v1/keys?page=1&page_size=100`、`GET /api/v1/groups/available`、
  `GET /api/v1/groups/rates`、`GET /api/v1/usage/dashboard/stats`、
  `GET /api/v1/usage/stats` 和 `GET /api/v1/user/platform-quotas` 均返回 HTTP 200；
- NexusTok 适配器只读同步认证成功，管理地址和 Relay 地址解析一致，读取到 10 条 Key，
  聚合模型 18 个；
- 多条 Key 的 `/v1/models` 返回 HTTP 403，因此 Key 资源为 partial、模型资源为
  stale；单 Key 模型失败只影响本轮该 Key 的能力，不清除旧模型、旧 Secret 或旧额度。

真实站点验收是人工/黑盒验证，不作为 CI 输入，也不把专用账号、凭据、Cookie、Token
或完整 Key 写入仓库。

## 14. 文件路径索引

### 14.1 旧版 NexusTok

| 文件 | 事实入口 |
| --- | --- |
| `/opt/project/backup_projects/NexusTok-0.1-archive/service/upstreamaccount/preview.go` | `Preview`、2FA 完成、预览缓存、消费和脱敏 |
| `/opt/project/backup_projects/NexusTok-0.1-archive/service/upstreamaccount/create.go` | `CreateFromPreview`、父渠道和账号池创建、Quota 写入 |
| `/opt/project/backup_projects/NexusTok-0.1-archive/service/upstreamaccount/refresh.go` | `RefreshChannelFromCredential`、`RefreshChannelFromSnapshot`、匹配和缺失处理 |
| `/opt/project/backup_projects/NexusTok-0.1-archive/service/upstreamaccount/key_model_sync.go` | 预览后逐 Key 模型同步和失败回退 |
| `/opt/project/backup_projects/NexusTok-0.1-archive/service/upstreamaccount/preview_key_models.go` | 预览快照中的单 Key 模型获取 |
| `/opt/project/backup_projects/NexusTok-0.1-archive/service/upstreamaccount/auto_sync.go` | 后台逐渠道刷新和失败汇总 |
| `/opt/project/backup_projects/NexusTok-0.1-archive/service/upstreamaccount/client.go` | HTTP、envelope、脱敏、数值和 URL 公共逻辑 |
| `/opt/project/backup_projects/NexusTok-0.1-archive/service/upstreamaccount/newapi.go` | 旧版 New API 登录、分组、倍率、Token、Key 和余额 |
| `/opt/project/backup_projects/NexusTok-0.1-archive/service/upstreamaccount/sub2api.go` | 旧版 Sub2API 登录、Refresh、用户、用量、分组、Key 和余额 |
| `/opt/project/backup_projects/NexusTok-0.1-archive/service/upstreamaccount/sub2api_endpoint.go` | Sub2API 页面 `api_base_url`、管理/Relay 地址和同源校验 |
| `/opt/project/backup_projects/NexusTok-0.1-archive/service/upstreamaccount/types.go` | 快照、Key、Session、余额和同步类型定义 |

### 14.2 当前 NexusTok

| 文件 | 事实入口 |
| --- | --- |
| `router/channel-router.go` | 平台站点同步、资源、Key、Auth Flow 和模型管理路由 |
| `controller/upstream_channel.go` | 保存配置、同步入口、资源查询、Key 响应和脱敏 |
| `service/upstream_site.go` | 统一同步、HTTP 客户端、错误分类、快照持久化和失败回退 |
| `service/upstream_site_adapters.go` | New API/Sub2API 认证、Refresh、资源请求、Key 和模型 |
| `service/platform_site_auth_flow.go` | 浏览器采集、密码登录、2FA、短期 Auth Flow 和一次性消费 |
| `model/upstream_channel.go` | 平台凭据、站点账号、UpstreamKey、模型能力、加密和可路由判定 |
| `model/platform_site_resources.go` | 身份、分组、端点、端点能力和资源同步状态 |
| `service/upstream_site_test.go` | 认证、Cookie 轮换、Refresh 不确定、资源失败和快照回退测试 |

### 14.3 Sub2API 参考源

| 文件 | 事实入口 |
| --- | --- |
| `/opt/project/sub2api-main/backend/internal/server/routes/user.go` | 用户、Keys、Groups、Usage、Platform Quotas 实际注册 |
| `/opt/project/sub2api-main/backend/internal/server/routes/admin.go` | Admin accounts、accounts/data 和 step-up 挂载 |
| `/opt/project/sub2api-main/backend/internal/server/middleware/step_up.go` | Admin API Key、TOTP、真人 JWT 和近期 step-up 条件 |
| `/opt/project/sub2api-main/backend/internal/handler/auth_handler.go` | 登录、2FA、Refresh DTO 和响应 |
| `/opt/project/sub2api-main/backend/internal/handler/api_key_handler.go` | Key 列表和详情，注释路径与真实路径差异 |
| `/opt/project/sub2api-main/backend/internal/handler/dto/types.go` | API Key 额度、过期时间和 5 小时/日/7 日限流窗口 DTO |
| `/opt/project/sub2api-main/backend/internal/handler/dto/mappers.go` | API Key 响应字段映射和窗口重置时间计算 |
| `/opt/project/sub2api-main/backend/ent/apikey.go` | API Key 持久化字段和窗口字段定义 |
| `/opt/project/sub2api-main/backend/internal/handler/usage_handler.go` | Usage Stats 和 Dashboard Stats |
| `/opt/project/sub2api-main/backend/internal/handler/user_handler.go` | Profile、Platform Quotas 和窗口字段 |
| `/opt/project/sub2api-main/backend/internal/handler/admin/account_handler.go` | Admin accounts 列表和敏感账号管理 |
| `/opt/project/sub2api-main/backend/internal/handler/admin/account_data.go` | accounts/data 导出字段和敏感数据边界 |
| `/opt/project/sub2api-main/backend/internal/handler/dto/types.go` | 用户、Key、窗口额度和响应 DTO |

### 14.4 New API 参考源

| 文件 | 事实入口 |
| --- | --- |
| `/opt/project/new-api-main/router/api-router.go` | `/api/status`、用户、Token、Usage、Ratio Config 路由和中间件 |
| `/opt/project/new-api-main/router/channel-router.go` | Admin Channel、Channel Model 和 SecureVerification 路由 |
| `/opt/project/new-api-main/controller/user.go` | Login、2FA、Verify、Self、Groups、Models 和 Refresh Handler |
| `/opt/project/new-api-main/controller/token.go` | Token 列表、详情、单条 Key 和批量 Key |
| `/opt/project/new-api-main/controller/group.go` | 用户分组和倍率 |
| `/opt/project/new-api-main/controller/pricing.go` | `/api/pricing` |
| `/opt/project/new-api-main/controller/ratio_config.go` | `/api/ratio_config` |
| `/opt/project/new-api-main/controller/misc.go` | `/api/status` |
| `/opt/project/new-api-main/model/token.go` | Token 字段、额度、无限额度、模型限制和过期字段 |

### 14.5 all-api-hub 辅助参考

| 路径 | 用途 |
| --- | --- |
| `/opt/project/all-api-hub-main/src/services/apiService/newApi/dashboardAuth.ts` | New API Dashboard Auth Bundle、Refresh Cookie 和 Session 组合 |
| `/opt/project/all-api-hub-main/src/services/apiService/sub2api/tokenRefresh.ts` | Sub2API Refresh Token 轮换辅助实现 |
| `/opt/project/all-api-hub-main` 中的 New API/Sub2API API Service | 浏览器端请求顺序、管理地址和 Relay 地址辅助核对 |

## 15. 分析结论

1. 旧版和当前都不是“登录后读一个总接口”，而是先认证，再按身份、余额、用量、分组、倍率、Key、Key 详情和单 Key 模型分阶段读取。
2. New API 的内部 quota 与 USD 展示必须依赖 `quota_per_unit` 区分；Sub2API 的 Key quota 目前按 USD 读取后转内部 quota。
3. Sub2API 参考平台存在平台窗口额度接口，但旧版和当前适配器都没有完整接入。
4. New API 账号级模型和 Admin channel 模型不能复制给所有子 Key；当前真正进入路由的模型能力必须能追溯到单 Key 字段或单 Key Relay 探测。
5. 资源读取失败不等于 Key 删除。只有完整分页、权限有效、详情和必要模型确认完成后，才允许执行缺失判定。
6. 当前版本增加了资源级快照、加密 Secret、HMAC 指纹、管理/Relay 地址分离、Refresh 不确定状态和管理端脱敏响应，运行行为与旧版数据模型和父渠道 `UsedQuota` 语义存在实质差异。

## 16. 2026-09-30 变更记录

| 日期 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- |
| 2026-09-30 | 当前 New API Token、Sub2API 普通 Key 和 Admin Key 资源分页统一受 100 页上限影响；New API 登录和 Sub2API 登录主体兼容、完整 Key 补偿、资源顺序和模型来源边界与旧版不完全一致 | 迁移旧版已验证契约：三类资源分页独立恢复为最多 1000 页；New API 恢复 `username`、`email` 和混合主体的受限兼容、批量 Key 缺失补偿及单条 POST/GET 回退；Sub2API 恢复 email/username/混合主体、Profile/Groups/Usage/Keys 顺序、列表完整 Key 优先、详情补齐和单 Key Relay 模型探测；凭据加密、Refresh 轮换、安全验证分类和最近成功快照保持不变 | New API/Sub2API 认证、资源快照、Key 生命周期、模型能力和路由候选 | `service/upstream_site_adapters.go`、`service/upstream_site.go`、`service/upstream_site_test.go`；本机三套参考源；两个真实站点隔离会话黑盒验收 |
| 2026-09-30 | 真实站点验收记录容易混入凭据或完整响应 | 只保留脱敏方法、路径、状态、资源数量和资源状态；New API 验收到 11 条 Key/32 个聚合模型，Sub2API 验收到 10 条 Key/18 个聚合模型；两站点均只执行登录、资源读取、Key 详情和 `/v1/models` 探测 | 交付文档、审计边界和后续回归 | Chrome DevTools MCP 隔离上下文；验收结束已关闭真实站点页面且未生成临时文件 |

## 17. 2026-10-02 平台站点认证、会话清理与自动配置

**变更前**

- 当前同步链路可能把历史 Access Token、Refresh Token、Cookie、Session ID 或
  NewAPI Dashboard Refresh 作为密码同步的 fallback；
- NewAPI/Sub2API 资源获取完成后没有统一记录本轮会话注销、精确 SID/Refresh Token
  边界，清理失败与快照状态的关系不清楚；
- 自动配置的候选顺序、用户验证、Helper 版本、诊断字段和跨节点一次性消费规则
  没有与现有资源快照链路统一。

**变更后**

- NewAPI 密码登录固定从 `POST /api/user/login?turnstile=` 开始，清除内存旧认证
  材料，不调用 Dashboard Refresh；Sub2API 密码登录固定调用
  `POST /api/v1/auth/login`。NewAPI 结束后调用 `POST /api/user/auth/logout`，有
  SID 时继续调用 `DELETE /api/user/sessions/{sid}`；`AUTH_SESSION_MISMATCH` 和
  401/403/404/405 属于幂等完成，禁止 `/api/user/sessions/revoke-others`。
- Sub2API 结束后只调用 `POST /api/v1/auth/logout` 并提交本轮 `refresh_token`，
  禁止 `/api/v1/auth/revoke-all-sessions`。无 Refresh Token、网络错误或意外 5xx
  只产生脱敏告警，清理仍清除本地临时材料，最近成功快照不被覆盖。
- 两个平台的 `PlatformSiteAdapter.Cleanup` 在认证错误、资源错误、部分分页失败和
  快照写库前后执行；Access Token、Admin Key、Cookie 和浏览器 Capture 登录态不执行
  后台注销。自动配置 Helper 当前版本为 `1.7.0`，先验证当前用户或 Admin 管理权限，
  仅保留存在性、存储键、验证接口、版本、失败阶段和来源诊断。
- Capture 结果继续使用 `PlatformSiteCredential` 的现有加密字段。解析后使用
  Redis `SET NX` 或单进程内存 claim 独占，渠道保存成功后一次性消费，后续失败释放
  claim；claim token 不进入 API 响应，历史旧凭据继续可读但不再提供新的手动输入入口。
- 资源失败、分页失败、权限不足、安全验证和 Cleanup 告警不会删除或降级已有
  `UpstreamKey`、模型能力、额度、倍率、权重和 RoutingKey 候选；只有完整资源同步
  允许执行密钥缺失判定。

本次没有新增数据库字段。当前本地矩阵仅覆盖真实 SQLite 3.50.4、MySQL 8.2.0 和
PostgreSQL 15.19；MySQL 5.7.8、PostgreSQL 9.6、独立日志数据库和真实生产上游注销
结果仍按“待持续核验”记录。

## 18. 2026-10-03 Sub2API 资源同步与登录兼容修复

**变更前**

- 重定向后的管理页面仍按原始输入地址解析相对 `api_base_url`，并以同注册域名规则
  处理 Relay，导致页面从管理域跳转到独立 Relay 域名时无法探测单 Key 模型；
- 普通邮箱登录可能携带 `agreed_revision`、`username` 等参考 DTO 未声明的字段，
  严格部署会返回 `400 INVALID_REQUEST`；400 之后的主体/路由边界不够明确。

**变更后**

- `discoverSub2APIPageModelBaseURL` 使用 HTTP 响应最终 URL；相对地址按最终 HTML
  页面解析。页面声明的 Relay 只有在与最终页面来源或原始管理地址协议、主机和有效
  端口一致，或与原始管理地址满足严格直接 `api.` 父子域关系时接受，并继续执行
  SSRF、重定向和协议/端口校验；
- 管理地址与 Relay 地址保持职责分离。管理地址用于认证、用户、分组、用量和 Key；
  Relay 只用于每条完整 Key 的 `/v1/models` 或 `/models` 探测和转发。模型探测成功后
  才写入 Key 能力、`upstream_keys` 和父渠道模型并集；
- Sub2API 首次邮箱登录只发送 `email/password`，`/api/v1/auth/login` 只在 404/405
  回退 `/auth/login`；400 `INVALID_REQUEST` 不被当作密码错误，也不触发路由或主体
  重试。只有明确条款 marker 才读取公开设置并提交 revision；
- 资源、分页或单 Key 模型失败仍保留最近成功快照；认证成功但资源失败只记录资源级
  失败或 partial/stale 状态，不把账号直接标记为凭据失效，也不执行错误的 Key 缺失
  判定。
- 可信重定向页面来源在本轮会话中作为管理请求根地址使用，避免 Go 客户端跨主机重定向
  时删除 Authorization；Relay 请求与管理会话 Header/Cookie 隔离。

本次使用脱敏 HTTP fixture、参考源静态核对和现有本地数据库测试；未在文档、测试、
日志或 Git 中保存真实账号、密码、Token、Cookie、Admin Key、完整响应或完整密钥。

## 19. 2026-10-04 自动配置 Capture Helper 兼容修复

**变更前**

- Helper 在目标站 DOM 尚未就绪或用户尚未完成登录时立即探测，失败回调会把临时未登录
  状态提交为 Capture Session `failed`；
- NewAPI 仅有页面 Cookie 和 `localStorage.uid` 时，`/api/user/self` 请求缺少
  `New-Api-User`；Sub2API Session Restore 只读取页面存储，不能覆盖旧版 IndexedDB
  客户端 ID。

**变更后**

- Helper 在 DOM 就绪后运行，认证候选失败只在页面显示脱敏等待信息并按 3 秒间隔自动
  重试，同时提供手动重试按钮；只有已验证登录态才提交完成请求，暂时未登录不改变
  后端 Capture Session 状态；
- handoff 同时兼容下划线和驼峰字段，避免真实后端生成的 `capture_secret`、
  `complete_url`、`capture_id`、`expires_at` 被误判为缺失；
- NewAPI 从 `uid`、用户对象和 JWT 解析数字用户 ID，把 `New-Api-User` 与 Cookie/
  Bearer 一起发送到用户验证、Dashboard Refresh 和用户 Token 接口；Sub2API 读取
  `sub2api-auth-coordination` 数据库的 `values/sub2api_auth_client_id` 并通过
  `X-Sub2API-Auth-Client` 进行 Session Restore；
- 浏览器 Capture 登录态仍属于用户既有会话，不进入后台密码同步 Cleanup；诊断不保存
  Token、Cookie、密码、完整响应或完整请求头。
  Helper 还会合并页面可见 Cookie 与同站 `GM_cookie` 结果，并在回传前删除完整
  `auth_user`，仅保留安全身份字段；handoff 中的管理地址路径用于 `base_url` 和
  `management_base_url`，避免部署在子路径时丢失地址。

## 20. 2026-10-04 无扩展页面桥接

**变更前**：自动配置只能依赖 UserScript 扩展执行；MCP 或普通浏览器没有安装
Tampermonkey/Violentmonkey 时，上游页面不会运行采集逻辑，HTTPS 页面访问本机回调还
可能被 CORS/PNA 拦截；过期 Capture Session 仍可能在前端显示为持续检测。

**变更后**：Capture Helper 升级为 `1.7.0`，在保留 UserScript 的同时，为每个短期
Capture Session 返回带 `install_token` 的 `capture_bridge_url`。桥接代码在上游页面中
复用现有 NewAPI/Sub2API 采集策略，通过 `window.opener.postMessage` 将经过当前用户或
Admin 验证的结果交给 NexusTok 页面；NexusTok 校验来源窗口、Origin、Capture ID 和
版本后调用同源完成接口，并向上游页面发送一次性成功/失败回执。桥接地址不包含
Capture Secret，诊断和消息不保存密码、完整 Cookie、Token 或上游响应。

若上游登录重定向丢失 handoff 查询参数，桥接脚本会通过 opener 向管理页请求同一
Capture Session 的一次性 handoff；管理页只向活动上游窗口、匹配 Origin 和匹配 Capture
ID 响应。浏览器拒绝跨源 `javascript:` 导航时，前端先获取桥接脚本并提供不含敏感值的
短启动片段复制入口；片段在上游页面上下文运行后再通过 opener 请求脚本，仍不允许管理页
直接跨源执行代码。

Capture Session 状态查询遇到不存在或过期时，前端立即停止轮询并清理无效状态，允许
重新创建会话；未完成登录、Turnstile/WAF 或验证码仍只保持 pending 并等待重试，桥接
不绕过上游安全验证。浏览器采集态与后台账号密码同步会话继续分离，资源失败仍保留
最近成功快照。

## 21. 2026-10-05 旧版浏览器采集方式迁移校准

**变更前**

- 旧版 `service/upstreamaccount/capture.go` 的浏览器采集会从同源脚本发现真实 API
  路由，并根据目标页面路径补全代理前缀；当前平台站点实现虽然已合并 DOM 等待、重试、
  NewAPI `New-Api-User` 和 Sub2API IndexedDB Client ID，但 Dashboard Refresh 和
  Refresh Token 分支仍存在固定路径假设；
- 当前脚本解析 Sub2API `api_base_url` 时优先保留 Origin，部署在 `/base/api/v1` 等
  子路径下的站点可能需要依赖额外路径猜测，和旧版“API 后缀剥离后保留部署前缀”的
  自动配置方式不完全一致。

**变更后**

- 仅迁移旧版浏览器登录态采集策略，不恢复旧版账号池、预览和 ChannelAccount 资源链路。
  采集输出继续落入当前 `PlatformSiteCredential`，由 Capture Session 绑定管理员、渠道、
  Origin 和 Helper 版本，并通过短期加密缓存和一次性 claim 消费；
- NewAPI Dashboard Refresh 候选改为同源脚本发现和路径前缀扩展后的
  `/api/user/auth/refresh`，仍发送页面 Origin、Cookie 和可用数字 `New-Api-User`，并严格
  校验 Bundle 的 `success`、Bearer 类型、当前 Session、用户身份、Access Token 和过期
  时间；
- Sub2API Refresh Token 候选改为 `/api/v1/auth/refresh`、`/api/auth/refresh`、
  `/auth/refresh` 加同源脚本发现路径。只有存在 Refresh Token 且需要恢复 Access Token
  时调用，刷新成功后继续通过 `auth/me` 验证；
- API 前缀来源扩展为 handoff 管理地址、回传 API 地址、当前页面、`__APP_CONFIG__` 和
  `api_base_url` 存储。路径以 `/api` 或 `/api/v1` 结尾时只保留部署前缀，用于生成
  `/base/api/v1/auth/me`、`/base/api/v1/auth/session/restore`、`/base/api/v1/auth/refresh`
  等候选，避免重复 API 后缀；
- 同源脚本发现仍限制在当前 Origin，服务端地址关系继续使用同 Host 或严格 `api.` 父子
  Host。浏览器 Capture 登录态仍不参与后台密码会话 Cleanup，资源失败、分页失败、安全
  验证和权限不足继续保留最近成功快照。

## 22. 2026-10-05 Sub2API 页面声明 Relay 与管理/模型双地址

**变更前**

- 资源比较只记录了 `api_base_url` 和管理面地址，未把页面
  `custom_endpoints`/`customEndpoints` 作为独立的 Relay 声明来源；
- `api_base_url` 为空的 Sub2API 定制站点无法稳定发现模型地址，管理请求和
  `/v1/models` 探测容易错误复用同一 Host；
- 外部 Relay 的信任边界没有明确要求服务端重新读取匿名页面，也没有把管理会话头与
  Relay 探测请求隔离写入比较基线。

**变更后**

- 页面发现按 `api_base_url` 优先、首个合法 `custom_endpoints[].endpoint` 次之的
  顺序选择地址；绝对和相对地址都按最终 HTML URL 解析，并经过协议、端口、重定向和
  SSRF 校验。服务端只使用结构化配置/受限 JSON 对象扫描，不使用宽泛任意 URL 正则；
- Capture 完成时，Sub2API 外部 Relay 必须通过 `record.BaseURL` 的匿名页面复核，
  与页面明确声明的 Relay 地址规范化后完全匹配；未声明、协议/端口不匹配、页面读取
  失败或仅有客户端自报地址时拒绝。管理地址仍保持同 Host 或严格 `api.` 关系；
- 平台账号 `base_url` 保存管理地址，`relay_base_url` 保存页面声明 Relay，渠道
  `base_url` 使用去除末尾 `/v1` 的 Relay 根地址。管理端按 `/api/v1/...` 获取
  identity、groups、usage 和 keys，Relay 端按 `/v1/models` 确认每条完整 Key 的
  模型能力；
- Relay 请求使用独立客户端和请求头，只携带当前完整 Key 的
  `Authorization: Bearer`，不发送 `x-api-key`，不携带管理 Cookie、Bearer、Origin、Referer、`X-Requested-With`、
  `X-Auth-Session` 或其它管理会话材料。Key/模型资源部分失败时继续标记
  `partial/stale` 并保留最近成功快照，不把空响应当作密钥删除；
- 指定站点脱敏验收结果为：管理地址与 `api-image.shour.bond` Relay 地址已拆分，
  渠道启用并成功读取 7 条 Key，7 条 Key 的 `ModelsSynced` 为真；本轮资源表保留
  `keys=partial`、`models=stale` 的历史快照，未输出完整凭据。

## 23. 2026-10-05 Sub2API Access Token 优先与资源同步复核

**变更前**

- 页面同时存在 `auth_token` 和 `refresh_token` 时，资源获取前可能先刷新令牌。旧
  Refresh Token 失效会阻断仍可调用管理接口的 Access Token，导致 Key 和模型资源
  无法进入本轮同步；
- 资源比较未明确“先验证 Access Token、仅 HTTP 401 才刷新”的边界，也没有把刷新后
  的 `auth/me` 重新验证作为当前用户确认步骤。

**变更后**

- Sub2API 管理面按 Access Token 优先：先调用当前用户接口，只有明确 HTTP 401 且
  存在 Refresh Token 才调用刷新；仅有 Refresh Token 时直接刷新。刷新响应必须包含
  可用的 Access Token、Refresh Token 和有效过期信息，随后再次调用 `auth/me`；
- 当前用户接口保留 `/api/v1/auth/me`、`/api/auth/me`、`/api/v1/user/profile`、
  `/auth/me` 兼容路径，只有路由缺失才继续候选，不把 HTML、WAF、权限和非 401 业务
  错误当成可刷新凭据；
- 管理地址继续读取 `/api/v1/...` 的身份、分组、用量和 Key；页面公开声明的
  `custom_endpoints`/`customEndpoints` Relay 继续由服务端匿名复核，单 Key 模型继续
  只请求 Relay `/v1/models`，不携带管理 Cookie、Bearer、Origin、Referer、
  `X-Requested-With` 或 `X-Auth-Session`；
- 真实站点复核中，本轮管理面读取到 12 条 Key，7 条 Key 的 `ModelsSynced=true`，
  渠道模型和可路由 Key 保持可用。其余资源失败仍记录 `partial/stale` 并使用最近
  成功快照，不把部分分页或单 Key 探测失败当作凭据不存在；
- 该链路仍使用 Capture Session、用户/渠道绑定、一次性 claim 和加密
  `PlatformSiteCredential`，不恢复旧版账号池、Preview 或 `ChannelAccount` 同步，
  浏览器采集登录态不进入后台密码 Cleanup。

## 24. 2026-10-05 Sub2API 标量过期时间与 Session Restore 触发边界

**变更前**：Sub2API 浏览器存储中的 `token_expires_at` 可能是 JSON 标量，通用的
嵌套对象读取逻辑会把数字当成没有字段的对象；同时，缺少
`sub2api_auth_client_id` 或同源 `session/restore` 路由时仍可能产生空恢复请求，
使诊断把“没有恢复材料”误认为恢复失败。

**变更后**：采集脚本的命名字段读取同时支持对象、数字、布尔值和字符串，并将毫秒
时间归一化为 Unix 秒；只有明确存在 Client ID 且同源资源发现出真实
`session/restore` 路由时才执行 Browser Restore，否则记录 `not_attempted` 并保持
pending。指定站点实际走 `auth_token + auth_user + /api/v1/auth/me`，仍能保存过期
时间并完成管理地址/Relay 地址拆分。该修复不改变单 Key `/v1/models` 探测、最近成功
快照、加密凭据或旧版账号池不恢复的边界。

## 25. 2026-10-05 Sub2API 配置包裹与 Cookie Client ID

**变更前**：资源获取比较只覆盖 localStorage/sessionStorage 和 IndexedDB Client ID；
页面公开配置被 `data`、`config` 或 `JSON.parse("...")` 包裹时，Relay 声明可能无法被
Helper 或服务端发现。

**变更后**：Capture Helper 从明确命名的 localStorage/sessionStorage 配置、有限命名
状态包裹和 `window.__APP_CONFIG__` 读取 `custom_endpoints`/`customEndpoints`；服务端
使用 `common.Unmarshal` 解析受限的 JSON.parse/转义对象。Browser Restore 额外支持精确
`sub2api_auth_client_id` Cookie，再回退 IndexedDB，并继续只有存在同源恢复路由才请求。
Cookie、Token、完整页面配置和 `auth_user` 内容不进入诊断；管理面仍获取 Key，Relay
仍只做单 Key `/v1/models`，失败继续使用最近成功快照。

## 26. 2026-10-05 Sub2API CSP nonce 页面桥接

**变更前**：无扩展桥接收到页面桥接脚本后立即插入内联 `script`。对于
`https://tk.shour.bond` 这类 `script-src 'self'` 且 nonce 动态生成的页面，页面尚未
提供 nonce 时脚本被 CSP 拦截，资源比较链路尚未进入管理面认证和 Relay 模型探测。

**变更后**：页面桥接先查找 `script[nonce]`、脚本元素的 `nonce` 属性和
`getAttribute('nonce')`，缺失时等待 DOM ready、脚本节点变化和有限重试；只有取得
nonce 才执行一次 `bridge.js`，超时以校验过的桥接失败消息反馈，不执行无 nonce 脚本。
失败消息不调用完成接口，不清理有效 Capture Session。该修复只改变浏览器脚本传输，
不改变已确认的 `auth_token`/`auth/me` 管理认证、`custom_endpoints` Relay 声明复核、
  `https://tk.shour.bond` 管理地址与 `https://api-image.shour.bond` Relay 的双地址
  资源获取、管理/Relay 请求头隔离、最近成功 Key/模型快照或浏览器 Cleanup 边界；真实
  浏览器验收使用新的 Capture Session，未把账号、密码、Token、Cookie 或完整 Key 写入
  比较结果。

## 27. 2026-10-09 17 个站点资源同步修复与验收边界

**变更前**：NewAPI `/api/ratio_config` 的可选权限失败可能把有效
`/api/pricing` 误记为端点失败；Sub2API 快照没有始终补齐 `endpoints`，管理地址、
Relay 地址和单 Key 模型探测的认证边界不够严格；资源失败时首次同步可能被错误标记
为使用快照。

**变更后**：

- NewAPI 只把有效 `/api/pricing` 端点信息作为 `endpoints=success` 的依据，
  `/api/ratio_config` 失败仅保留内部诊断，不覆盖价格端点结果；
- Sub2API 的 `endpoints` 资源明确记录管理接口和页面确认的 Relay 接口。单 Key
  `/v1/models` 失败时只回退 `/models`，禁止将请求重定向到管理路径；请求仅带当前
  完整 Key 的 Bearer，不带 `x-api-key`、管理 Cookie、管理 Bearer、Origin、
  Referer、`X-Requested-With` 或 `X-Auth-Session`；
- `platform_site_resource_syncs` 每轮都覆盖 `identity`、`groups`、`endpoints`、
  `usage`、`keys`、`models`。真实失败不改写为成功，只有已有资源表/能力表/
  账号成功时间等证据时才记录 `using_snapshot=true`；
- 代码入口为 `service/upstream_site.go`、`service/upstream_site_adapters.go` 和
  `service/upstream_site_test.go`；本次无模型或迁移结构变化。

截至 2026-10-09，17 个站点的真实登录、Capture、安全验证、六类资源全成功、完整
Key 的模型能力确认和低成本渠道测试仍未全部完成；脱敏 fixture、SQLite/容器验证
以及热环境健康检查不能替代真实站点验收，因此不能据此声称远端验收完成。v0.2.8
可按分阶段发布流程部署到指定远端进行真实验收，逐站结果以发布评审报告为准。

## 28. 2026-10-10 v0.2.8 远端发布验收口径

**变更前**：本节将真实站点未全部完成与“不得发布 v0.2.8”绑定，未区分用于远端测试的
分阶段发布和最终站点验收；这与先发布、再应用更新、最后逐站同步的操作流程不一致。

**变更后**：允许将 v0.2.8 发布到指定远端进行真实测试，但本地 fixture、历史数据库状态、
容器健康或发布成功均不构成站点通过。每站必须在本轮重新登录后确认 `identity`、
`groups`、`endpoints`、`usage`、`keys`、`models` 六项全部为 `success` 且未依赖历史快照。
遇到安全验证、凭据失效或部分失败时记录脱敏阻塞原因，不绕过验证，也不将站点记为成功。
逐站状态在 [`docs/platform-site-resource-sync-v0.2.8-review.md`](platform-site-resource-sync-v0.2.8-review.md)
中维护。
