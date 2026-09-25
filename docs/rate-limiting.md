# 限流与并发保护

> 文档状态：代码事实基线
> 事实基线日期：2026-09-25
> 主要代码来源：`middleware/rate-limit.go`、`middleware/model-rate-limit.go`、`middleware/task_artifact_access.go`、`controller/plugin_protocol_limiter.go`、`model/user_session.go`
> 关联架构文档：[`docs/architecture/authentication-and-authorization.md`](architecture/authentication-and-authorization.md)、[`docs/architecture/data-cache-and-background-jobs.md`](architecture/data-cache-and-background-jobs.md)、[`docs/architecture/system-overview.md`](architecture/system-overview.md)

本文按当前代码实现整理 NexusTok 的请求限流、模型请求限流、会话限制、并发 admission、业务发送限制和请求体大小保护。

这些机制并不都是“每分钟允许多少次请求”：

- **时间窗口限流**：在固定时间窗口内限制请求次数。
- **固定窗口限流**：Redis 计数器按窗口计数，窗口边界两侧可能产生突发流量。
- **令牌桶**：按速率补充令牌，同时限制桶容量。
- **并发 admission**：只限制当前正在处理的连接或请求数量。
- **Session 数量限制**：限制账户可以持有或签发的登录 Session 数量。
- **业务发送限额**：限制验证码、通知等业务动作的发送次数。
- **请求体大小限制**：按请求体字节数拒绝过大的请求，不按频率计数。

## 1. 策略总览

| 策略 | 作用域/标识 | 计数或并发维度 | 默认值 | 保护范围 | 超限响应 | 配置方式 | 是否需重启 | 实现位置 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Web 全局 `GW` | `rateLimit:v2:ip:GW:<ip>` | 客户端 IP、固定窗口 | 10 次/60 秒 | Web 页面请求；`/static/`、`/assets/` 静态资源不计入 | HTTP `429`，Redis 模式带 `Retry-After` | 环境变量 | 是 | `middleware/rate-limit.go`、`router/web-router.go` |
| API 全局 `GA` | `rateLimit:v2:ip:GA:<ip>` | 客户端 IP、固定窗口 | 10 次/60 秒 | 普通 API、匿名 API 和未豁免请求 | HTTP `429` | 环境变量 | 是 | `middleware/rate-limit.go`、`router/api-router.go` |
| 关键操作 `CT` | `rateLimit:v2:ip:CT[:scope]:<ip>` | 客户端 IP、固定窗口 | 20 次/20 分钟 | 登录、注册、找回密码、OAuth、支付、会话退出等关键操作 | HTTP `429` | 环境变量 | 是 | `middleware/rate-limit.go` |
| 登录分桶 `CT:auth-login` | `CT:auth-login` | 客户端 IP、固定窗口 | 继承 `CT` | 密码登录、2FA、登录验证、Passkey 登录 | HTTP `429` | 环境变量 | 是 | `router/api-router.go` |
| 注册分桶 `CT:auth-register` | `CT:auth-register` | 客户端 IP、固定窗口 | 继承 `CT` | 注册 | HTTP `429` | 环境变量 | 是 | `router/api-router.go` |
| 恢复分桶 `CT:auth-recovery` | `CT:auth-recovery` | 客户端 IP、固定窗口 | 继承 `CT` | 找回密码、重置密码 | HTTP `429` | 环境变量 | 是 | `router/api-router.go` |
| OAuth 分桶 `CT:auth-oauth` | `CT:auth-oauth` | 客户端 IP、固定窗口 | 继承 `CT` | OAuth、微信、Telegram 登录或绑定 | HTTP `429` | 环境变量 | 是 | `router/api-router.go` |
| Refresh `CT:auth-refresh` | `rateLimit:v2:ip:CT:auth-refresh:<ip>` | 客户端 IP、固定窗口 | 60 次/20 分钟 | Refresh Cookie 刷新登录状态 | HTTP `429`，带 `Retry-After` | 环境变量；受 `CRITICAL_RATE_LIMIT_ENABLE` 总开关影响 | 是 | `middleware/rate-limit.go` |
| 用户关键操作 `UC` | `rateLimit:v2:user:UC:<scope>:<user_id>` | 用户 ID、固定窗口 | 20 次/20 分钟 | Access Token、Passkey、2FA、安全验证、推广额度转移等 | HTTP `429` | 复用关键操作环境变量 | 是 | `middleware/rate-limit.go`、`router/api-router.go` |
| 用户搜索 `SR` | `rateLimit:v2:user:SR:<user_id>` | 用户 ID、固定窗口 | 10 次/60 秒 | Token 搜索、日志搜索等 | HTTP `429` | 环境变量 | 是 | `middleware/rate-limit.go`、`router/api-router.go` |
| 模型总请求 `MRRL` | `rateLimit:MRRL:<user_id>` 或进程内键 | 认证用户 ID、时间窗口或令牌桶 | 关闭；开启后总请求数为 0（不限制） | `/v1`、`/v1beta`、任务插件模型请求 | HTTP `429` | 管理员后台 | 否 | `middleware/model-rate-limit.go` |
| 模型成功请求 `MRRLS` | `rateLimit:MRRLS:<user_id>` 或进程内键 | 认证用户 ID、成功请求窗口 | 1000 次/1 分钟 | 返回状态小于 400 的模型请求 | HTTP `429` | 管理员后台 | 否 | `middleware/model-rate-limit.go` |
| 邮箱验证码 `EV` | `rateLimit:v2:ip:EV:<ip>` | 客户端 IP、固定窗口 | 2 次/30 秒 | 发送邮箱验证码 | HTTP `429` | 当前为代码固定值 | 不适用 | `middleware/email-verification-rate-limit.go` |
| 通知发送限额 | `notify_limit:<user_id>:<type>:<hour>` | 用户 ID + 通知类型 + 时间桶 | 2 次/10 分钟 | 重复发送通知 | 由调用方返回业务错误；不保证统一为 `429` | 环境变量 | 是 | `service/notify-limit.go` |
| 任务产物无效访问 | 进程内 IP 窗口 | 客户端 IP、1 分钟窗口 | 60 次/分钟 | 无效或伪造的匿名任务产物 capability | 先伪装为 `404`；超过限制返回 `429` | 环境变量 | 是 | `middleware/task_artifact_access.go` |
| 任务产物并发 | 进程内计数器 | 全局、客户端 IP、任务产物对象 | 128/64/16 个并发 | 带有效 capability 的任务产物内容访问 | HTTP `429` | 环境变量 | 是 | `middleware/task_artifact_access.go` |
| 任务插件观察并发 | 进程内计数器 | 全局、插件、用户、Token | 128/32/4/2 个并发 | 异步任务插件协议观察连接 | HTTP `429`，错误码 `rate_limit_exceeded` | 当前为代码固定值 | 不适用 | `controller/plugin_protocol_limiter.go` |
| 活跃登录 Session | 数据库 `user_sessions` | 用户 ID、未过期 active Session | 5 个 | 限制同时保留的登录设备 | HTTP `409`，`AUTH_SESSION_LIMIT` | 环境变量 | 是 | `model/user_session.go`、`service/auth_session.go` |
| Session 签发窗口 | 数据库 `user_sessions` | 用户 ID、创建时间窗口 | 100 个/24 小时 | 限制重复创建 Session | HTTP `429`，`AUTH_SESSION_ISSUANCE_LIMIT` | 环境变量 | 是 | `model/user_session.go`、`service/auth_session.go` |
| 请求体大小 | 请求体字节数 | 全局解压后大小；匿名接口单独限制 | 全局 128 MB；匿名 512 KB | 防止超大请求和压缩包解压导致资源耗尽 | HTTP `413` | 环境变量 | 是 | `middleware/gzip.go`、`middleware/request_body_limit.go` |

## 2. Web 和 API 全局 IP 限流

### 2.1 Web 全局限流 `GW`

配置项：

```env
GLOBAL_WEB_RATE_LIMIT_ENABLE=true
GLOBAL_WEB_RATE_LIMIT=10
GLOBAL_WEB_RATE_LIMIT_DURATION=60
```

默认值是每个客户端 IP 在 60 秒内 10 次。Web 页面请求会消耗额度，但以下静态资源不会消耗 `GW`：

```text
/static/*
/assets/*
```

Redis 可用时使用 Redis 原子固定窗口；Redis 不可用时使用当前 API 进程内存。超过限制返回 `429`，Redis 路径能够返回剩余窗口的 `Retry-After`。

### 2.2 API 全局限流 `GA`

配置项：

```env
GLOBAL_API_RATE_LIMIT_ENABLE=true
GLOBAL_API_RATE_LIMIT=10
GLOBAL_API_RATE_LIMIT_DURATION=60
```

普通匿名 API 和普通用户 API 按客户端 IP 计数。已验证、启用状态的管理员或 Root 请求可以跳过 `GA`，但这个豁免只适用于全局 API 桶，不会跳过登录、Refresh、Session 签发、搜索、模型请求、支付或其他专用策略。

以下认证启动和公开状态接口不会消耗 `GA`：

```text
/api/setup
/api/status
/api/uptime/status
/api/user/auth/refresh
/api/user/login
/api/user/login/encryption-key
/api/user/login/2fa
/api/user/login/verify
/api/user/login/passkey/begin
/api/user/login/passkey/finish
/api/user/passkey/login/begin
/api/user/passkey/login/finish
```

注册、找回密码、OAuth 和其他普通匿名 API 仍然会受到 `GA` 或对应的 `CT` 专用分桶保护。

## 3. 关键操作、登录和用户级限流

### 3.1 关键操作 IP 限流 `CT`

配置项：

```env
CRITICAL_RATE_LIMIT_ENABLE=true
CRITICAL_RATE_LIMIT=20
CRITICAL_RATE_LIMIT_DURATION=1200
```

默认是每个 IP、每个作用域 20 次/20 分钟。认证请求按用途拆分为独立桶，因此登录、注册、密码恢复和 OAuth 不会互相消耗同一个作用域的额度：

| 作用域 | 主要请求 |
| --- | --- |
| `CT:auth-login` | 密码登录、2FA、登录验证、Passkey 登录 |
| `CT:auth-register` | 注册 |
| `CT:auth-recovery` | 找回密码、重置密码 |
| `CT:auth-oauth` | OAuth、微信、Telegram 登录或绑定 |
| `CT:auth-session` | 登录会话退出 |
| `CT` | 尚未拆分的其他关键操作 |

关闭 `CRITICAL_RATE_LIMIT_ENABLE` 会关闭 `CT` 和复用它的 `UC`；当前 Refresh 中间件也会先检查这个总开关，因此该开关关闭时 `CT:auth-refresh` 也不会执行。

### 3.2 Refresh 专用限流

配置项：

```env
AUTH_REFRESH_RATE_LIMIT=60
AUTH_REFRESH_RATE_LIMIT_DURATION=1200
```

默认是每个 IP 60 次/20 分钟。缺少 Refresh Cookie 或 Refresh Cookie 无效的请求也会先消耗 Refresh 桶，防止通过伪造刷新请求反复探测。

该策略仍依赖 `CRITICAL_RATE_LIMIT_ENABLE=true`。`AUTH_REFRESH_RATE_LIMIT` 和持续时间配置为 0、负数或无法解析时，会回退默认值并记录启动告警。

### 3.3 用户级关键操作 `UC`

`UC` 在用户身份认证后执行，按用户 ID 而不是 IP 计数。当前主要作用域包括：

```text
UC:security-verification
UC:access-token
UC:aff-transfer
UC:account-security
```

它与 `CT` 共享以下配置，没有独立的每个作用域环境变量：

```env
CRITICAL_RATE_LIMIT_ENABLE
CRITICAL_RATE_LIMIT
CRITICAL_RATE_LIMIT_DURATION
```

### 3.4 用户级搜索 `SR`

配置项：

```env
SEARCH_RATE_LIMIT_ENABLE=true
SEARCH_RATE_LIMIT=10
SEARCH_RATE_LIMIT_DURATION=60
```

默认是每个登录用户 10 次/60 秒。搜索限流按用户 ID 计数，因此用户更换代理 IP 不会获得一份新的额度。

## 4. 模型请求限流

模型请求限流是当前唯一提供完整管理员后台动态配置界面的主要限流策略。

入口：

```text
系统设置 -> 安全 -> 限流
```

### 4.1 全局配置

| 配置项 | 默认值 | 含义 |
| --- | ---: | --- |
| `ModelRequestRateLimitEnabled` | `false` | 是否启用模型请求限流 |
| `ModelRequestRateLimitDurationMinutes` | `1` | 时间窗口，单位为分钟 |
| `ModelRequestRateLimitCount` | `0` | 时间窗口内总请求数；`0` 表示不限制 |
| `ModelRequestRateLimitSuccessCount` | `1000` | 时间窗口内成功请求数 |
| `ModelRequestRateLimitGroup` | 空对象 | 用户组或 Token 组的覆盖规则 |

总请求数在请求进入模型路由时统计，包含失败请求。成功请求数只在响应状态小于 400 时记录。

### 4.2 分组配置

分组配置使用以下 JSON 格式：

```json
{
  "default": [200, 100],
  "vip": [0, 1000]
}
```

每个数组的含义是：

```text
[maxRequests, maxSuccess]
```

分组规则优先于全局规则。例如：

- `default` 组最多 200 次请求，其中最多 100 次成功；
- `vip` 组总请求数不限制，成功请求最多 1000 次。

分组规则使用同一个时间窗口。成功请求上限必须为正数，总请求上限可以设为 `0` 表示不限制。

### 4.3 存储和运行时行为

- 配置保存到 `Option` 表。
- Root 管理员通过 `/api/option` 修改。
- `updateOptionMap` 修改内存中的运行时配置。
- 保存后不需要重启。
- Redis 模式下，成功请求使用时间戳列表，总请求使用令牌桶。
- 无 Redis 时回退当前进程内存限流器，多节点不会共享额度。
- 当前模型请求限流覆盖 `/v1`、`/v1beta`、任务插件模型请求和相关插件协议模型端点。

## 5. 邮箱验证码和通知发送限制

### 5.1 邮箱验证码 `EV`

当前代码固定为：

```text
每个客户端 IP 2 次/30 秒
```

Redis 可用时使用 Redis 固定窗口；Redis 不可用时使用进程内存。超过限制返回 `429`，响应消息会提示稍后重试。

当前没有环境变量和管理员后台入口。要调整默认值，需要修改代码中的 `EmailVerificationMaxRequests` 和 `EmailVerificationDuration`。

### 5.2 通知发送业务限额

配置项：

```env
NOTIFY_LIMIT_COUNT=2
NOTIFICATION_LIMIT_DURATION_MINUTE=10
```

默认是每个用户、每种通知类型 2 次/10 分钟。实现使用用户 ID、通知类型和按小时生成的时间桶作为计数维度：

```text
用户 ID + 通知类型 + 时间桶
```

Redis 可用时使用 Redis；否则回退到进程内存。它是业务层的发送检查，不是统一挂载在 HTTP 路由上的中间件，因此达到限制后的 HTTP 状态码由具体调用方决定，不能一概认为一定返回 `429`。

由于 Redis Key 使用当前小时作为分桶标识，边界附近不应将它理解为严格的滑动 10 分钟窗口；需要严格窗口语义时应单独调整实现。

## 6. 任务产物访问保护

配置项：

```env
TASK_ARTIFACT_INVALID_RATE_LIMIT_PER_MINUTE=60
TASK_ARTIFACT_GLOBAL_CONCURRENCY=128
TASK_ARTIFACT_IP_CONCURRENCY=64
TASK_ARTIFACT_OBJECT_CONCURRENCY=16
```

默认值：

| 配置项 | 默认值 | 维度 |
| --- | ---: | --- |
| `TASK_ARTIFACT_INVALID_RATE_LIMIT_PER_MINUTE` | 60 | 每个 IP 每分钟的无效 capability 尝试次数 |
| `TASK_ARTIFACT_GLOBAL_CONCURRENCY` | 128 | 所有任务产物内容请求的活跃并发数 |
| `TASK_ARTIFACT_IP_CONCURRENCY` | 64 | 单个 IP 的活跃并发数 |
| `TASK_ARTIFACT_OBJECT_CONCURRENCY` | 16 | 单个任务产物对象的活跃并发数 |

这组限制在服务启动时读取，非法或非正数会回退到默认值，当前没有管理员后台入口。

行为说明：

- 无效、重复或过长的 capability 通常返回伪装的 `404`，避免暴露任务或产物是否存在。
- 同一 IP 的无效尝试达到每分钟上限后返回 `429`。
- 有效 capability 对应的请求如果触发全局、IP 或对象并发上限，也返回 `429`，并带 `Retry-After: 60`。
- 该 limiter 使用当前进程内存，不依赖 Redis；多节点部署时每个节点分别计数。

## 7. 任务插件协议观察并发

任务插件协议观察连接使用进程内互斥计数器，默认限制如下：

| 维度 | 默认并发 |
| --- | ---: |
| 全局 | 128 |
| 单插件 | 32 |
| 单用户 | 4 |
| 单 Token | 2 |

它限制的是仍处于观察状态的活跃连接，不是时间窗口请求次数。达到任一维度上限时返回：

```json
{
  "error": {
    "type": "rate_limit_exceeded"
  }
}
```

当前没有环境变量或管理员后台配置。要调整需要修改 `controller/plugin_protocol_limiter.go` 中的代码固定值。

## 8. 登录 Session 限制

Session 限制不是普通 HTTP 请求速率限流，但它是登录 `429`、登录 `409` 和重复登录问题的直接保护机制。

### 8.1 活跃 Session 上限

配置项：

```env
USER_SESSION_ACTIVE_LIMIT=5
```

默认每个用户最多保留 5 个未过期且状态为 `active` 的 Session。创建新 Session 前，服务端在同一数据库事务中串行化检查、清理和插入。

达到上限时返回：

```text
HTTP 409
AUTH_SESSION_LIMIT
```

系统不会回收仍有活动的会话。只有同时满足以下条件的 Session 才会在新登录时清理：

- 创建时间超过 2 小时；
- 从未推进过 `last_active_at`；
- 状态仍为 `active`；
- 尚未过期。

“超过 2 小时”的清理阈值是代码固定值，不是环境变量。近期创建的 Session 和已经产生认证活动的 Session 不会被这项清理撤销。

### 8.2 Session 签发窗口

配置项：

```env
USER_SESSION_ISSUANCE_LIMIT=100
USER_SESSION_ISSUANCE_WINDOW_SECONDS=86400
```

默认是每个用户 24 小时内最多创建 100 个 Session。统计包括仍 active、已撤销和旧鉴权版本的 Session，因此反复登录、登出并不会立即释放签发额度。

达到签发上限时返回：

```text
HTTP 429
AUTH_SESSION_ISSUANCE_LIMIT
```

### 8.3 保留期和告警阈值

```env
USER_SESSION_REVOKED_RETENTION_DAYS=7
USER_SESSION_HOURLY_ALERT_THRESHOLD=5000
```

- `USER_SESSION_REVOKED_RETENTION_DAYS` 默认保留 revoked Session 7 天，用于审计和签发计数。
- `USER_SESSION_ISSUANCE_WINDOW_SECONDS` 不能有效超过 revoked 保留期；超过时启动阶段会将实际窗口钳制到保留期并记录告警。
- `USER_SESSION_HOURLY_ALERT_THRESHOLD` 默认是全局每小时 5000 个新 Session，只记录告警，不拒绝登录。
- Session 活跃数和签发计数以数据库为权威，不依赖 Redis 限流桶。

## 9. 请求体大小保护

### 9.1 全局请求体

配置项：

```env
MAX_REQUEST_BODY_MB=128
```

默认限制为解压后的 128 MB。该保护同时覆盖未压缩请求和 gzip、Brotli、zstd 请求，目的是防止过大的请求或压缩包解压导致内存占用失控。

超过限制通常返回：

```text
HTTP 413 Request Entity Too Large
```

`MAX_REQUEST_BODY_MB` 不是限流值，也不能通过设置为 0 来可靠地关闭：不同读取路径对非正数有内部回退值。生产环境应配置明确的正整数。

### 9.2 匿名接口请求体

配置项：

```env
ANONYMOUS_REQUEST_BODY_LIMIT_KB=512
```

默认限制为 512 KB，主要用于登录、注册、初始化、密码恢复和 Webhook 等匿名接口。

- 读取为负数时回退到 512 KB。
- 读取为 0 时，匿名请求体预读限制会被跳过。
- 该设置只控制匿名路由的额外限制，不能取消全局解压和请求体保护。

## 10. 环境变量配置清单

下表汇总当前可以通过 `.env` 或启动环境变量调整的项目。除模型请求限流外，这些配置通常在启动时读取，修改后需要重启 API 服务。

| 环境变量 | 默认值 | 单位/语义 | 是否可关闭 | 非法值处理 |
| --- | ---: | --- | --- | --- |
| `GLOBAL_WEB_RATE_LIMIT_ENABLE` | `true` | Web 全局 IP 限流开关 | 是 | 按布尔配置解析 |
| `GLOBAL_WEB_RATE_LIMIT` | `10` | Web IP 请求数 | 不建议用 `0` | 解析失败回退默认值 |
| `GLOBAL_WEB_RATE_LIMIT_DURATION` | `60` | Web 窗口秒数 | 不建议用 `0` | 解析失败回退默认值 |
| `GLOBAL_API_RATE_LIMIT_ENABLE` | `true` | API 全局 IP 限流开关 | 是 | 按布尔配置解析 |
| `GLOBAL_API_RATE_LIMIT` | `10` | API IP 请求数 | 不建议用 `0` | 解析失败回退默认值 |
| `GLOBAL_API_RATE_LIMIT_DURATION` | `60` | API 窗口秒数 | 不建议用 `0` | 解析失败回退默认值 |
| `CRITICAL_RATE_LIMIT_ENABLE` | `true` | CT/UC 总开关 | 是 | 按布尔配置解析 |
| `CRITICAL_RATE_LIMIT` | `20` | 关键操作次数 | 不建议用 `0` | 解析失败回退默认值 |
| `CRITICAL_RATE_LIMIT_DURATION` | `1200` | 关键操作窗口秒数 | 不建议用 `0` | 解析失败回退默认值 |
| `AUTH_REFRESH_RATE_LIMIT` | `60` | Refresh IP 请求数 | 无独立关闭开关 | 非正数或解析失败回退默认值 |
| `AUTH_REFRESH_RATE_LIMIT_DURATION` | `1200` | Refresh 窗口秒数 | 无独立关闭开关 | 非正数或解析失败回退默认值 |
| `SEARCH_RATE_LIMIT_ENABLE` | `true` | 搜索限流开关 | 是 | 按布尔配置解析 |
| `SEARCH_RATE_LIMIT` | `10` | 每用户搜索次数 | 不建议用 `0` | 解析失败回退默认值 |
| `SEARCH_RATE_LIMIT_DURATION` | `60` | 搜索窗口秒数 | 不建议用 `0` | 解析失败回退默认值 |
| `NOTIFY_LIMIT_COUNT` | `2` | 每用户/通知类型次数 | 无独立关闭开关 | 解析失败回退默认值 |
| `NOTIFICATION_LIMIT_DURATION_MINUTE` | `10` | 通知窗口分钟数 | 无独立关闭开关 | 解析失败回退默认值 |
| `TASK_ARTIFACT_INVALID_RATE_LIMIT_PER_MINUTE` | `60` | 无效 capability/IP/分钟 | 否 | 非正数回退默认值 |
| `TASK_ARTIFACT_GLOBAL_CONCURRENCY` | `128` | 任务产物全局活跃并发 | 否 | 非正数回退默认值 |
| `TASK_ARTIFACT_IP_CONCURRENCY` | `64` | 任务产物单 IP 活跃并发 | 否 | 非正数回退默认值 |
| `TASK_ARTIFACT_OBJECT_CONCURRENCY` | `16` | 单产物对象活跃并发 | 否 | 非正数回退默认值 |
| `USER_SESSION_ACTIVE_LIMIT` | `5` | 每用户活跃 Session 数 | 否 | 非正数回退默认值 |
| `USER_SESSION_ISSUANCE_LIMIT` | `100` | 每用户签发窗口 Session 数 | 否 | 非正数回退默认值 |
| `USER_SESSION_ISSUANCE_WINDOW_SECONDS` | `86400` | 签发窗口秒数 | 否 | 非正数回退默认值，超过保留期会钳制 |
| `USER_SESSION_REVOKED_RETENTION_DAYS` | `7` | revoked Session 保留天数 | 否 | 非正数回退默认值 |
| `USER_SESSION_HOURLY_ALERT_THRESHOLD` | `5000` | 全局每小时签发告警阈值 | 否 | 非正数回退默认值 |
| `MAX_REQUEST_BODY_MB` | `128` | 全局解压后请求体 MB | 不建议关闭 | 非正数在不同读取路径使用内部回退值 |
| `ANONYMOUS_REQUEST_BODY_LIMIT_KB` | `512` | 匿名接口请求体 KB | `0` 可跳过额外匿名预读限制 | 负数回退默认值 |

`CRITICAL_RATE_LIMIT_ENABLE=false` 会同时影响 `CT`、`UC` 以及当前实现中的 Refresh 限流。需要只调整 Refresh 时，应保持关键操作总开关开启，只修改 `AUTH_REFRESH_RATE_LIMIT` 和对应窗口。

## 11. 管理员后台可以调整什么

当前管理员后台明确提供的是模型请求限流：

```text
系统设置 -> 安全 -> 限流
```

可以动态调整：

```text
ModelRequestRateLimitEnabled
ModelRequestRateLimitDurationMinutes
ModelRequestRateLimitCount
ModelRequestRateLimitSuccessCount
ModelRequestRateLimitGroup
```

修改保存到 `Option` 表并热更新内存，不需要重启。

当前没有管理员后台配置入口的项目包括：

- Web/API 全局 IP 限流；
- `CT`、登录、注册、密码恢复、OAuth 和 Refresh 限流；
- `UC` 用户级敏感操作限流；
- `SR` 搜索限流；
- 邮箱验证码限流；
- 通知发送限额；
- 任务产物访问限额和并发；
- 任务插件协议观察并发；
- Session 活跃上限、签发窗口和保留期；
- 请求体大小限制。

## 12. Redis 和多节点行为

### 12.1 共享 Redis

当多个 API 节点连接同一个 Redis 时，以下策略的 Redis 计数可以在节点间共享：

- `GW`、`GA`；
- `CT`、`CT:auth-refresh`；
- `UC`、`SR`；
- 模型请求限流的 Redis 分支；
- 邮箱验证码和通知限额的 Redis 分支。

Session 数量限制仍然以主数据库为权威，不依赖 Redis 计数。

### 12.2 独立 Redis 或没有 Redis

没有共享 Redis 时，通用 IP/用户限流和模型限流会回退到当前进程内存，任务产物和插件协议并发本来就是进程内计数。因此多节点部署时，每个节点会各自放行一份额度，集群总额度可能接近“单节点阈值 × 节点数”。

生产多节点部署应使用共享 Redis；否则不要把单节点配置值理解为整个集群的严格总上限。

### 12.3 固定窗口边界

通用 Redis IP/用户限流使用原子固定窗口，而不是严格滑动窗口。窗口边界两侧可以分别通过一次完整额度，因此极短时间内的通过量可能接近配置值的两倍。这是当前外部可观察行为。

### 12.4 客户端 IP 和可信代理

IP 限流依赖 Gin 对客户端 IP 的识别。`TRUSTED_PROXIES` 决定哪些代理地址可以提供可信的转发链：

```env
TRUSTED_PROXIES=none
```

表示不信任任何代理，使用 TCP 直连地址；显式填写 IP/CIDR 时只信任列出的反向代理。未配置时会默认信任回环和常见私网代理，并输出启动告警。代理配置错误可能导致多个用户共享一个代理 IP 的额度，或让攻击者伪造客户端 IP。

## 13. 登录出现 `429` 或 `409` 时的排查

建议先查看响应状态、业务错误码和 `Retry-After`，再按以下顺序判断。

| 现象 | 常见策略 | 说明 |
| --- | --- | --- |
| 登录、2FA、Passkey 返回 `429` | `CT:auth-login` | 默认 20 次/20 分钟/IP |
| Refresh 返回 `429` | `CT:auth-refresh` | 默认 60 次/20 分钟/IP；还要检查 `CRITICAL_RATE_LIMIT_ENABLE` |
| 登录页面或页面资源加载失败 | `GW` | 页面请求受 Web 全局桶保护；静态资源路径会跳过 `GW` |
| 普通匿名 API 返回 `429` | `GA` | 登录和 Refresh 等公开认证启动接口已排除，但其他匿名 API 仍可能受限 |
| 搜索返回 `429` | `SR` | 按用户 ID 计数 |
| 模型调用返回 `429` | `MRRL/MRRLS` | 检查管理员后台“系统设置 -> 安全 -> 限流” |
| 重复登录返回 `429 AUTH_SESSION_ISSUANCE_LIMIT` | Session 签发窗口 | 默认 100 个/24 小时/用户 |
| 登录返回 `409 AUTH_SESSION_LIMIT` | 活跃 Session 上限 | 默认 5 个；清理后仍有活动会话时不会强制回收 |
| Refresh/Logout 返回 `409 AUTH_SESSION_MISMATCH` | Session 标识不一致 | 客户端持有的 SID 与 Refresh Cookie 不一致 |
| 请求返回 `413` | 请求体大小保护 | 检查 `MAX_REQUEST_BODY_MB` 或 `ANONYMOUS_REQUEST_BODY_LIMIT_KB` |

管理员跳过的只有 `GA` 全局 API 桶，不会跳过登录、Refresh、Session 签发、模型请求和其他专用策略。

## 14. 配置示例

### 14.1 默认安全配置

```env
GLOBAL_WEB_RATE_LIMIT_ENABLE=true
GLOBAL_WEB_RATE_LIMIT=10
GLOBAL_WEB_RATE_LIMIT_DURATION=60

GLOBAL_API_RATE_LIMIT_ENABLE=true
GLOBAL_API_RATE_LIMIT=10
GLOBAL_API_RATE_LIMIT_DURATION=60

CRITICAL_RATE_LIMIT_ENABLE=true
CRITICAL_RATE_LIMIT=20
CRITICAL_RATE_LIMIT_DURATION=1200
AUTH_REFRESH_RATE_LIMIT=60
AUTH_REFRESH_RATE_LIMIT_DURATION=1200

SEARCH_RATE_LIMIT_ENABLE=true
SEARCH_RATE_LIMIT=10
SEARCH_RATE_LIMIT_DURATION=60
```

### 14.2 本地开发放宽 Web/API 限制

仅适用于可信的本地开发环境：

```env
GLOBAL_WEB_RATE_LIMIT_ENABLE=false
GLOBAL_API_RATE_LIMIT_ENABLE=false
```

不要在公网生产环境直接关闭全局限流。关闭 `GA` 也不会关闭 `CT`、Refresh、模型请求和 Session 限制。

### 14.3 生产多节点

```env
REDIS_CONN_STRING=redis://redis.example.internal:6379/0
TRUSTED_PROXIES=10.0.0.10,10.0.0.11
```

所有 API 节点应连接同一个 Redis，并保持相同的限流环境变量。Redis、数据库和 Session 密钥的多节点要求见[用户鉴权与登录会话](./authentication.md)。

### 14.4 降低模型请求速率

模型请求限流不使用上述环境变量，应在管理员后台配置：

```text
启用：开启
时间窗口：1 分钟
总请求数：200
成功请求数：100
```

需要按用户组区分时，可以配置：

```json
{
  "default": [200, 100],
  "vip": [0, 1000]
}
```

### 14.5 调整 Session 设备数和签发窗口

```env
USER_SESSION_ACTIVE_LIMIT=8
USER_SESSION_ISSUANCE_LIMIT=200
USER_SESSION_ISSUANCE_WINDOW_SECONDS=86400
USER_SESSION_REVOKED_RETENTION_DAYS=7
```

提高这些值会增加设备管理和数据库保留压力。`USER_SESSION_ISSUANCE_WINDOW_SECONDS` 不应超过 revoked Session 的保留时间，服务启动时超出部分会被钳制。

## 15. 当前代码固定值和未挂载能力

当前无法通过 `.env` 或管理员后台调整的主要固定值：

```text
邮箱验证码：2 次/30 秒/IP

任务插件协议观察并发：
全局 128
单插件 32
单用户 4
单 Token 2

未激活登录 Session 自动清理阈值：2 小时
```

另外，`middleware/rate-limit.go` 中仍定义了 `UploadRateLimit()` 和 `DownloadRateLimit()` 工厂，默认值为 10 次/60 秒，但当前路由检索未发现它们被挂载到生效路由。因此不能把这两个工厂当作当前实际生效的独立上传/下载限流策略；如后续挂载路由，应同步补充本文档。

Redis Key 前缀、内存清理周期以及令牌桶内部参数属于实现细节，当前没有独立配置入口。

## 16. 相关实现文件

- `common/init.go`：环境变量读取和启动时初始化。
- `common/constants.go`：通用默认值。
- `middleware/rate-limit.go`：Web/API/关键操作/Refresh/用户级限流。
- `middleware/model-rate-limit.go`：模型总请求和成功请求限流。
- `setting/rate_limit.go`：模型限流配置和分组校验。
- `middleware/email-verification-rate-limit.go`：邮箱验证码限流。
- `service/notify-limit.go`：通知业务限额。
- `setting/system_setting/task_artifact.go`、`middleware/task_artifact_access.go`：任务产物访问保护。
- `controller/plugin_protocol_limiter.go`：任务插件协议并发。
- `model/user_session.go`、`service/auth_session.go`：登录 Session 上限和签发窗口。
- `middleware/gzip.go`、`middleware/request_body_limit.go`：请求体大小保护。

## 与架构文档的关系

本文保留各限流桶、默认值、配置项、响应码和生效路由的详细契约；架构文档只说明限流在鉴权、Relay、任务、缓存和多节点链路中的位置。增加或调整限流策略时，必须同步本文、对应架构文档和变更偏差登记。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 补充架构索引与元信息 | 已有限流详细规则没有统一事实基线和架构入口 | 增加事实基线、代码来源、架构分工和变更记录；保留原有桶、配置和默认值 | Web/API/用户/模型/任务/Session/请求体保护 | `middleware/`、`controller/plugin_protocol_limiter.go`、`model/user_session.go` 静态核对 |
