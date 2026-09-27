# 鉴权、会话与授权原理

> 文档状态：代码事实基线
> 事实基线日期：2026-09-27
> 主要代码来源：`middleware/auth.go`、`middleware/token_auth.go`、`service/auth_session.go`、`model/user_session.go`、`service/authz/`、`controller/`、`oauth/`
> 关联详细文档：[`../authentication.md`](../authentication.md)、[`../rate-limiting.md`](../rate-limiting.md)、[`system-overview.md`](./system-overview.md)

## 1. 功能目标和边界

系统同时服务管理面板、统一 Relay API、任务和插件协议，因此“登录用户”“API Token 用户”“任务产物 Capability 持有者”不是同一种凭据。服务端负责最终鉴权、Session 状态、Token 状态、用户状态、授权策略和敏感操作验证；前端只负责保存短期状态、刷新和展示。

本文档记录架构关系。Cookie 参数、响应字段、限流桶和安全边界以[`authentication.md`](../authentication.md)与[`rate-limiting.md`](../rate-limiting.md)为详细契约。

## 2. 凭据和身份关系

| 凭据/机制 | 实际作用 | 服务端校验位置或入口 |
| --- | --- | --- |
| Access Token | 面板登录后的短期 Bearer JWT，放在浏览器内存 | 用户鉴权中间件和 JWT 校验 |
| Refresh Cookie | HttpOnly 不透明值，用于轮换 Access Token 和 Refresh Token | `/api/user/auth/refresh`、`service/auth_session.go` |
| `nexustok_session_id` | HttpOnly 的 SID 定位 Cookie，只在登录凭据已完成校验后帮助复用候选 Session，不能单独鉴权 | 登录出口、`service/auth_session.go`、`model/user_session.go` |
| `user_sessions` | 数据库中的登录会话控制面，记录 SID、设备、IP、方式、过期和撤销 | Session 服务和 Redis Session 快照 |
| API Token | Relay API 的调用凭据，关联用户、分组、模型限制、额度和状态 | `middleware.TokenAuth`、Token 模型查询/缓存 |
| PAT | 面向个人/管理自动化的长期个人访问凭据，权限由对应用户和 Token 状态决定 | Token 相关 Controller/Service |
| OAuth/OIDC | 外部身份提供商登录、绑定或回调，最终进入统一 Session 签发出口 | `oauth/`、用户 Controller、Session 服务 |
| Passkey/WebAuthn | 无密码或二次验证凭据，验证成功后进入统一登录或敏感操作流程 | WebAuthn Controller/Service |
| TOTP/2FA | 密码登录或敏感操作的附加因子 | 2FA/安全验证服务 |
| Security Proof | 由 `SESSION_SECRET` 派生的短期敏感操作证明，不是永久登录凭据 | 敏感操作 Controller/Service |
| 任务产物 Capability URL | 仅用于任务 Artifact 读取，由 `CRYPTO_SECRET` 签名 | `middleware/task_artifact_access.go`、插件产物服务 |

`SESSION_SECRET` 为 Access Token、Security Proof、Refresh Token 摘要和 AuthFlow 摘要派生不同用途的密钥。多节点必须保持一致；`CRYPTO_SECRET` 影响缓存键摘要和 Artifact Capability，多节点共享 Redis 或任务产物访问时也必须一致。

## 3. 面板登录与 Session 生命周期

登录、Passkey、OAuth、WeChat、Telegram 和 2FA 成功路径最终都应通过统一 Session 签发出口，生成 Access Token 和 Refresh Cookie。登录出口会先用 `nexustok_session_id` 定位当前用户、当前鉴权版本、active 且未过期的候选 Session；匹配时在原 `user_sessions` 行内轮换 SID、Refresh Secret 摘要和 Session Version，保留 `CreatedAt`，否则按新 Session 流程执行。Session 数据库状态是最终权威；Redis 保存用户鉴权快照和 Session 快照，缓存未命中或未启用 Redis 时回源数据库。

Refresh 采用轮换策略。客户端通过内存保存的 SID 和 `X-Auth-Session` 辅助避免 Cookie 与当前标签页身份错配；服务端在不匹配时返回冲突，不执行轮换/撤销。前端使用 Web Locks 和 BroadcastChannel 或 storage 事件协调同一浏览器配置文件中的刷新，但不跨标签传递 Access Token 或 Refresh Token。

2FA/Passkey 登录完成时，一次性 AuthFlow 的消费和 Session 复用/新建在同一事务内提交；会话上限或数据库写入失败会回滚 AuthFlow 消费。SID 失效、过期、撤销、跨用户或用户鉴权版本不匹配时不能复用，也不能因为 SID 本身获得任何权限。

用户密码、状态、角色、安全因子或其他安全相关属性变更时，`auth_version` 递增，旧会话失效。订阅导致用户组变化只刷新授权缓存，不会退出所有登录设备。Session 版本和撤销 tombstone 防止旧缓存重新授权。

账户的活跃 Session 数量和 Session 签发窗口在数据库上计数，默认值和错误码详见[`authentication.md`](../authentication.md)。这与 IP 限流是不同控制面。

## 4. Access Token、API Token 和用户状态

Relay API 进入 `middleware.TokenAuth` 后解析 API Token/PAT，确认 Token 存在、状态可用、所属用户状态可用，并把用户、Token、分组和模型限制写入请求上下文。随后 `ModelRequestRateLimit`、`Distribute` 和计费使用这些上下文。

Token 可以携带：

- 所属用户和 Token ID；
- 允许的用户组；
- 模型允许/拒绝列表；
- 无限额度或剩余额度；
- 渠道/模型请求需要的其它限制。

Token 限制不是前端 UI 的提示，而是在服务端分发和预扣/扣减路径中再次生效。模型限制命中失败时不能根据管理端显示的渠道数量推断可用能力。

## 5. Casbin 授权和敏感操作

`service/authz/` 负责 Casbin 策略的加载、评估和周期同步。Controller 的 Admin/Root 权限中间件与具体资源检查共同决定管理操作是否允许。授权缓存和策略同步存在刷新窗口，修改权限后需结合节点拓扑验证传播时间。

敏感操作可能要求当前登录、用户级关键操作限流、Security Proof、TOTP、Passkey 或重新验证。安全审计不能写入密码、验证码、恢复码、私钥或可用 Token；审计应记录用户、操作、资源、结果和足够的非秘密上下文。

## 6. Redis 拓扑和失效传播

| 拓扑 | Session 传播 | 限流/其它缓存 |
| --- | --- | --- |
| 所有节点共享 Redis | 撤销 tombstone、版本栅栏和缓存读写可在节点间即时共享 | Redis 限流和共享缓存可形成集群语义 |
| 每节点独立 Redis | Session 缓存最多在有效 `SYNC_FREQUENCY` 后回源收敛，旧 Token/Session 可能短暂返回 401 | 限流和其它缓存按节点分别计数或缓存 |
| 不使用 Redis | Session 每次回数据库；依赖数据库权威 | 使用进程内限流器和缓存，集群不共享 |

`SYNC_FREQUENCY` 默认和非法值回退为 60 秒。Session 缓存读取不会续期，TTL 受 Session 剩余寿命和同步周期约束。此语义只覆盖 Session 鉴权，不能推广到整个控制面。

## 7. 对外接口、配置和数据模型

- 面板登录/刷新/退出：`router/api-router.go` 下的 `/api/user/auth/*`。
- Session 管理：`/api/user/sessions` 及撤销接口。
- Relay Token：`router/relay-router.go`、`middleware/token_auth.go`。
- 用户和凭据：`model/user.go`、Token 模型、`model/user_session.go`、AuthFlow 模型。
- 授权策略：`service/authz/`、Casbin 初始化和策略同步。
- 配置：`SESSION_SECRET`、`CRYPTO_SECRET`、`SYNC_FREQUENCY`、Redis 连接配置及认证相关 Option。

## 8. 当前限制和实际偏差

- Refresh Cookie、Session 数据库、Redis 快照和 Access Token 的失效传播时间不同，不能只用 JWT 过期时间判断“已退出”。
- 独立 Redis 节点会带来有界陈旧 Session 和节点局部限流；部署说明必须明确拓扑。
- 已有详细文档描述了大量安全契约，本架构文档不重复定义所有 Cookie 属性和接口字段；两者冲突时应以代码和最新专项契约核对。
- 频繁重新登录同一浏览器不会新增 `user_sessions` 行，但每次认证仍会轮换 SID 和 Refresh Secret 摘要；这不是延长旧凭据有效期的豁免。
- OAuth、Passkey、TOTP、PAT 和 Security Proof 的端到端组合场景不能由单个入口文件证明；尚未覆盖的组合场景应标记“待核查”，不能据名称推断。

### 8.1 平台站点认证边界（2026-09-26）

平台站点账号密码登录由 NewAPI/Sub2API 适配器完成，上游返回 2FA challenge 时转入
服务端短期认证流程。流程绑定管理员、平台、规范化 Origin、管理地址和可选渠道，
使用不可预测 opaque token、5 分钟 TTL、失败次数限制和单次消费；密码、TOTP、
Cookie、Token、Refresh Token 和 Session ID 不写入日志、审计字段、URL 或普通响应。
账号密码和 TOTP 只用于完成当前流程，流程成功后只通过 `auth_flow_id` 交给保存渠道
逻辑。Passkey/WebAuthn 和上游安全证明保持浏览器自动配置/人工验证边界。

平台站点会话不等同于 NexusTok 面板 Session。NewAPI 既可能返回传统刷新令牌，也可能
返回 Dashboard Auth Bundle；可识别但结构不完整的现代 Bundle 必须重新认证，不得
静默按旧协议处理。Sub2API Refresh Token 轮换响应缺少新令牌或有效过期时间时，结果
标记为不确定，不重放旧令牌，并继续保留最近成功资源快照。

## 9. 维护时需要同步的关联模块

修改登录、刷新、退出、Session、用户状态、鉴权版本、授权策略、Token 模型、OAuth/OIDC、Passkey、TOTP 或安全证明时，必须同步本文档、[`authentication.md`](../authentication.md)、必要时的[`rate-limiting.md`](../rate-limiting.md)和偏差登记。修改 Redis 键、TTL、缓存回退或多节点传播时同步[`data-cache-and-background-jobs.md`](./data-cache-and-background-jobs.md)。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立凭据分层、Session 生命周期、授权和 Redis 拓扑说明 | `middleware/`、`service/auth*`、`model/user_session.go`、`oauth/` | `docs/authentication.md`、`model/user_session.go`、`service/auth_session.go` 静态核对 |
| 2026-09-26 | 平台站点认证补充 | 平台站点密码认证与系统面板 Session 的边界未单独说明，2FA/刷新失败语义未登记 | 明确短期上游认证流程、NewAPI Bundle 校验、Sub2API 轮换不确定结果和未覆盖的 Passkey/安全证明边界 | `service/upstream_site*.go`、`controller/upstream_channel.go`、平台站点资源接口 | NewAPI/Sub2API/all-api-hub 参考源路由和认证实现静态核对 |
| 2026-09-27 | 面板登录 Session 复用 | 架构只描述统一签发，未说明重复登录会增长 Session 行和签发计数 | 增加浏览器 SID 定位 Cookie、复用条件、SID/Refresh Secret 轮换、AuthFlow 原子性和失败回退边界 | `service/auth_session.go`、`model/user_session.go`、`model/login_verification.go`、登录 Controller | 服务层回归测试、AuthFlow 事务测试、OWASP ASVS 5.0.0 与认证/会话 Cheat Sheet 核对 |
| 2026-09-27 | NewAPI Dashboard 会话刷新 | 平台站点 Refresh Cookie、Session ID、Bearer Token 的组合和轮换持久化未统一；非 401 刷新失败可能被理解为可重试凭据错误 | 严格发送 `new_api_refresh` Cookie、`X-Auth-Session` 和旧 Bearer Token，Jar 轮换值优先持久化；现代 Bundle 不完整时拒绝旧协议降级，只有确认 401 凭据失效才允许密码回退，网络/WAF/安全验证/非 401 结果标记不确定 | 平台站点密码、Access Token、Cookie、Auth Flow 和凭据加密保存 | `service/upstream_site_adapters.go`、`service/platform_site_auth_flow.go`、`service/upstream_site_test.go`、OWASP ASVS 5.0.0、Authentication/Session Management Cheat Sheet |
| 2026-09-27 | 平台站点渠道代理认证 | 平台站点密码登录、2FA 和资源请求没有读取渠道代理，客户端合并可能覆盖平台 CookieJar、超时或重定向校验 | 所有后台同步和 Auth Flow 按 `channel_id` 复用渠道 Transport；只替换底层 Transport，保留 CookieJar、30 秒超时、管理站点重定向校验和 Cookie 轮换；代理配置、网络、凭据、安全验证和资源失败分层 | NewAPI/Sub2API 平台站点认证、Refresh、资源同步和敏感凭据保护 | `service/upstream_site.go`、`service/platform_site_auth_flow.go`、`service/upstream_site_test.go`、OWASP 认证/会话与 SSRF 指引 |

### 9.1 2026-09-26 实现校准

**变更前**：平台站点账号密码、自动配置和高级凭据路径没有统一的短期 2FA 承接语义；
会话刷新只需要关注访问令牌是否存在，资源管理接口也没有明确区分管理员资源拒绝与普通
账号失效。

**变更后**：默认管理端入口仍显示账号密码和自动配置，高级区域提供 Access Token、
Admin Key 和 Cookie。账号密码登录通过服务端 opaque flow ID 承接 New API/Sub2API 的
TOTP challenge，流程绑定管理员、平台、规范化 Base URL 和 Origin，TTL 为 5 分钟并限制
验证码尝试次数；密码、验证码、Cookie、Token、Refresh Token 和 Session ID 不进入日志、
审计字段、URL、普通响应或前端持久化存储。

New API 的传统刷新和 Dashboard Auth Bundle 分开解析。Bundle 必须同时满足 `success`、
`data.access_token`、`data.token_type`、未来的 `data.access_expires_at`、当前
`data.session.sid/current` 和 `data.user` 身份字段；已识别但结构不完整时标记重新认证。
Sub2API 只有在响应同时包含新 Access Token、新 Refresh Token 和有效 `expires_in` 时才替换
旧令牌；响应不完整或网络结果不确定时标记轮换不确定，不重放旧 Refresh Token。

本次安全核对记录为 OWASP ASVS 5.0.0，以及 Authentication、Session Management、MFA、
CSRF、Cryptographic Storage、Logging 和 SSRF Prevention Cheat Sheet。已验证流程过期、
重复消费、管理员/平台/站点绑定、验证码限流、刷新不确定和敏感字段脱敏。New API
Passkey/WebAuthn、上游 Security Proof、Turnstile 和站点风控仍由浏览器自动配置或人工
流程处理，NexusTok 未声明自动完成这些能力。

### 9.2 2026-09-27 NewAPI Dashboard Refresh 认证边界

**变更前**：NewAPI 新版 Dashboard 会话刷新所需的 `new_api_refresh` Cookie、
`X-Auth-Session`、Bearer Token、Session Bundle 和轮换 Cookie 没有在密码认证、令牌
认证、Cookie 认证和短期 Auth Flow 入口统一；刷新遇到非 401 错误时存在重复密码登录
风险。

**变更后**：所有 NewAPI 认证入口都按管理地址调用 `/api/user/auth/refresh`。请求携带
当前 Bearer Token、可信的 Session ID 和 Cookie；CookieJar 中同名 Cookie 优先，显式
Cookie 只补充 Jar 没有的值，响应轮换 Cookie 回写加密凭据。现代 Bundle 的
`access_token`、`token_type`、`access_expires_at`、当前 `session.sid/current` 和
用户身份字段必须完整，已识别但不完整时返回重新认证/刷新不确定，不进入旧 Token 兜底。
密码模式只有在刷新明确返回 401 且错误属于旧凭据失效时才允许一次密码登录；网络错误、
WAF、Turnstile、权限不足、HTML、非 401 或结构不完整结果不会重放密码。

该边界遵循 OWASP ASVS 5.0.0 中的认证失败处理、会话轮换、凭据保护和日志脱敏要求；
本项目只保存加密后的平台凭据，不把密码、Cookie、Access Token、Refresh Token 或
Session ID 写入日志、URL、普通响应或认证审计事件。

### 9.3 2026-09-27 平台站点渠道代理边界

**变更前**：平台站点认证客户端只按管理地址创建，渠道 `setting.proxy` 未覆盖密码登录、
二次验证、Dashboard Refresh 和资源读取；需要局域网代理的渠道会在服务端直连失败。

**变更后**：账号密码 Auth Flow 的开始与二次验证、后台同步的认证与资源请求都按
`channel_id` 读取渠道代理、HTTP 协议和连接分片配置。客户端合并只替换 Transport，保留
平台会话 CookieJar、30 秒超时、管理地址允许内网目标的重定向校验，以及显式 Cookie 与
Jar 的合并和轮换持久化。空代理仍使用直连平台会话，代理地址不写入源码。

认证失败、代理配置错误、网络错误、WAF/交互验证、权限不足和资源失败继续分别记录；
任何错误路径都不得把密码、Cookie、Access Token、Refresh Token、Session ID 或完整上游
响应写入日志、URL、审计字段或普通响应。
