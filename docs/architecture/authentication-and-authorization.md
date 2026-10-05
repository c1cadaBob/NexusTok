# 鉴权、会话与授权原理

> 文档状态：代码事实基线
> 事实基线日期：2026-10-05
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
- 系统维护更新、回滚和重启是 Root 专属敏感操作，不能由普通 Admin 或前端状态判断替代
  服务端鉴权；二次确认只用于降低误操作，最终权限边界仍由 `RootAuth()` 执行。

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

2026-09-30 旧版兼容主体迁移仅作用于上游平台登录请求，不改变 NexusTok 面板 Session
的轮换和凭据边界：New API 主体顺序为 `username/password`，邮箱输入仅在明确凭据错误
或 HTTP 401 后兼容 `email/password` 和混合主体；Sub2API 邮箱输入依次兼容
`email/password`、`username/password` 和混合主体，非邮箱输入使用 `username/password`。
安全验证、WAF、限流、权限拒绝、网络错误和其它非 401 结果不触发主体重试。所有上游
密码、Cookie、Access Token、Refresh Token、Admin Key 和完整 Key 仍不写入审计、日志、
普通响应或文档。

### 8.2 2026-10-01 Sub2API 登录服务条款边界

**变更前**

- Sub2API 账号密码登录没有读取公开服务条款设置，条款启用时只能提交旧版登录请求；
- 2FA 验证阶段不重新读取条款 revision，条款检查延迟或版本更新时无法携带当前版本；
- 条款拒绝无法与凭据错误、安全验证、权限不足区分，后台同步也无法给出专用状态。

**变更后**

- 仅 Sub2API 密码认证在 `prepareSub2APIManagementSession` 完成管理地址规范化、渠道
  Transport 注入、CookieJar、30 秒超时和重定向校验后，使用同一个 `PlatformSiteSession`
  请求 `GET /api/v1/settings/public`；
- 同时满足 `login_agreement_enabled=true` 和非空字符串
  `login_agreement_revision` 才向每个 `/api/v1/auth/login` 候选请求增加
  `agreed_revision`。公开设置接口 404、网络失败、响应格式不符合预期、条款关闭或
  revision 缺失时保持旧登录协议，并不改变邮箱、用户名、混合主体顺序和仅在明确凭据
  错误或 HTTP 401 后重试的边界；
- 初次登录返回 2FA challenge 时，`/api/v1/auth/login/2fa` 前重新读取公开设置并发送
  最新 revision。revision 只在当前认证请求生命周期内使用，不接受客户端提供的条款
  版本，不写入本地“已同意”记录，不发送未经参考源确认的 `not_in_cn_confirmed`；
- 条款 marker 优先归类为独立的 `login_agreement_required`，通过
  `ErrSub2APILoginAgreement` 返回，不触发主体回退，也不归类为
  `ErrPlatformSiteCredentials` 或 `ErrPlatformSiteSecurity`。同步失败写入
  `sync_status=failed`、`auth_status=login_agreement_required`、脱敏原因并递增失败次数；
  初次登录或 2FA 条款失败均不得清理最近成功密钥、模型能力、余额或其它资源快照；
- `PlatformSiteSession.Platform` 将该 marker 分类限制在 Sub2API，New API 不请求
  `/api/v1/settings/public`、不携带 `agreed_revision`，不推断同名字段。Controller 和
  `SafePlatformSiteError` 仅返回脱敏中文提示，说明已尝试提交当前条款版本但上游仍拒绝，
  并建议检查上游配置或使用浏览器采集登录态。

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
| 2026-09-30 | 旧版平台登录主体与脱敏 fixture 验收 | 上游登录主体兼容顺序与旧版不一致，脱敏资源和认证失败边界未在鉴权架构记录 | 恢复受限主体兼容；使用脱敏 fixture 验证登录 envelope、资源权限失败和快照边界；保留 Refresh 轮换、Bundle 完整性和安全验证分类 | NewAPI/Sub2API 上游认证、资源读取和安全审计 | `service/upstream_site_adapters.go`、`service/upstream_site_test.go`、脱敏 HTTP fixture；未使用真实账号或站点 |
| 2026-09-30 | 系统维护 Root 敏感操作 | 维护页只有浏览器侧 GitHub 读取，更新、回滚和重启没有 Root/审计/脱敏边界 | 后端经 `RootAuth()` 检查 Release 并创建系统任务，更新、回滚、重启沿用管理审计；管理响应、任务、日志和 helper 边界不输出敏感环境或认证值 | 系统维护接口、Root 权限、管理审计、任务响应和 Docker helper | `router/api-router.go`、`middleware/audit.go`、`controller/system_update.go`、`model/system_task.go`、OWASP ASVS 5.0.0 与认证/会话/日志 Cheat Sheet |
| 2026-10-01 | Sub2API 登录服务条款兼容 | Sub2API 密码认证没有读取公开条款 revision，2FA 和后台同步无法区分条款拒绝与凭据/安全验证错误 | 仅 Sub2API 密码登录按公开设置可选发送 `agreed_revision`，2FA 重新读取最新 revision；条款错误独立分类、停止主体回退、写入 `login_agreement_required` 并保留成功快照；New API 不读取或发送条款字段 | Sub2API 上游认证、Auth Flow、后台同步、资源状态和脱敏提示 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/platform_site_auth_flow.go`、`service/upstream_site_test.go`；Sub2API 参考源；OWASP ASVS 5.0.0、Authentication/Session Management/Logging/SSRF Cheat Sheet |
| 2026-10-03 | 平台认证流程保存与密码登录协议 | 完成认证流程后客户端仍可能提交用户名和密码，后端在解析流程前误报凭据冲突；New API 新版密码加密请求、旧版路由回退和 Sub2API 邮箱主体边界未集中登记 | `auth_flow_id` 请求仅保留流程材料，后端兼容用户名残留并以服务端流程身份为准；New API 读取加密密钥并使用 RSA-OAEP/v2 信封，仅 404/405 回退明文；Sub2API 仅在明确凭据错误/401 后从邮箱回退用户名，后台只撤销本轮 Refresh Token | 平台站点认证流程、NewAPI/Sub2API 密码同步、凭据保护和错误分类 | `controller/upstream_channel.go`、`service/newapi_password_encryption.go`、`service/upstream_site_adapters.go`、`web/src/features/channels/lib/channel-form.ts`、定向测试和本机参考源 |

### 9.1 2026-09-26 实现校准

**变更前**：平台站点账号密码、自动配置和高级凭据路径没有统一的短期 2FA 承接语义；
会话刷新只需要关注访问令牌是否存在，资源管理接口也没有明确区分管理员资源拒绝与普通
账号失效。

**变更后**：默认管理端入口只显示账号密码和自动配置；历史 Access Token、Admin Key
和 Cookie 渠道仍可读取和同步，但新版表单不再提供这些登录态的手动录入入口。账号密码
登录通过服务端 opaque flow ID 承接 New API/Sub2API 的 TOTP challenge，流程绑定管理员、
平台、规范化 Base URL 和 Origin，TTL 为 5 分钟并限制验证码尝试次数；密码、验证码、
Cookie、Token、Refresh Token 和 Session ID 不进入日志、审计字段、URL、普通响应或前端
持久化存储。

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

### 9.4 2026-09-27 NewAPI 旧版登录与 Dashboard Refresh 契约

**变更前**：NewAPI 密码认证和资源同步的真实旧版请求顺序没有在鉴权文档中明确，
Token 分页参数、兼容回退和现代 Dashboard 会话刷新容易被宽泛的旧 Token 解析混用。

**变更后**：

- 密码登录请求固定为 `POST /api/user/login?turnstile=`，JSON 只包含
  `username` 和 `password`；不会因 WAF、Turnstile、网络错误或普通业务失败重放
  其它密码字段组合；
- Dashboard Refresh 固定携带 `Cookie: new_api_refresh=...`、
  `X-Auth-Session` 和当前 Bearer Token，不把 Refresh Cookie 放入 JSON。CookieJar
  同名值优先，响应轮换 Cookie、Access Token、Token 类型、过期时间、Session ID
  和用户 ID 一并加密持久化；
- 现代 Bundle 出现结构标志但字段不完整时进入重新认证/刷新不确定，不降级到旧
  Token 协议；只有明确 HTTP 401 且可确认旧会话失效时才允许一次密码回退；
- 认证成功不等于所有资源成功。Token、Key、模型、分组、倍率和 Admin 资源分别记录
  状态；权限不足、安全验证、WAF、网络错误和部分分页失败不会把有效凭据改写成
  `credentials_invalid`，也不会触发密码重放；
- Auth Flow 开始、2FA 验证和后台同步按 `ChannelID` 读取渠道代理；代理配置错误
  单独分类，平台会话仍保留 CookieJar、30 秒超时和管理站点重定向校验。密码、Cookie、
  Token、Refresh Token 和验证码不进入认证审计事件。

### 9.5 2026-09-30 系统维护敏感操作边界

**变更前**：维护页面直接从浏览器请求 GitHub Release，只能显示版本说明；没有后端更新、
回滚、重启入口，也没有统一的 Root 权限和操作审计边界。

**变更后**：`/api/system-update/latest`、`/api/system-update/apply`、
`/api/system-update/rollback` 和 `/api/system-update/restart` 均挂载在
`RootAuth()` 下。前端 ConfirmDialog 是误操作保护，服务端仍会重新检查 Release、
资产、checksum、当前部署方式、Docker socket 和回滚槽位。POST 操作沿用管理审计中间件，
记录 action、操作者、结果、路由和非秘密上下文，不新增 Casbin 权限类别。

系统任务响应只返回版本、镜像摘要、阶段、进度、结果摘要和脱敏错误；密码、Cookie、
Session Secret、数据库 DSN、Redis 连接串、Access/Refresh Token、Admin Key 和完整
上游 Key 不进入管理响应、审计参数、任务错误或日志。helper 只在进程内继承更新所需环境，
不会把环境变量值放入手动命令或对外响应。安全核对依据为 OWASP ASVS 5.0.0、
Authentication Cheat Sheet、Session Management Cheat Sheet 和 Logging Cheat Sheet；
本次未改变 NexusTok 面板 Session、Refresh Token 或 CSRF 边界。

### 9.6 2026-10-02 平台站点密码会话所有权与自动配置

**变更前**：NewAPI 和 Sub2API 的平台站点后台同步可能沿用已保存的 Access Token、
Refresh Token、Cookie 或 Session ID；资源读取完成后没有统一的“本轮登录会话”注销契约。
渠道表单还把平台选择、高级认证和手动登录态输入混在凭证区域，自动采集完成后缺少
统一的验证、诊断和一次性消费边界。

**变更后**：

- `PlatformSiteAdapter` 统一提供 `Cleanup(context.Context, *PlatformSiteSession) error`。
  `password` 同步在任何上游资源请求前清除旧的 Authorization、Cookie、`X-Auth-Session`、
  Access Token、Refresh Token、Session ID、Cookie 和刷新状态，然后直接调用密码登录；
  `access_token`、`admin_key`、`cookie` 以及浏览器 Capture 登录态不执行后台注销。
- NewAPI 同步完成资源读取后调用 `POST /api/user/auth/logout`，携带本轮 Bearer、
  `X-Auth-Session`、Cookie 和基于管理地址计算的 `Origin`；有 SID 时继续调用
  `DELETE /api/user/sessions/{sid}`。`AUTH_SESSION_MISMATCH`、401、403、404、405
  按幂等完成处理，不调用 `/api/user/sessions/revoke-others`。
- Sub2API 同步完成资源读取后调用 `POST /api/v1/auth/logout`，请求体只包含本轮
  `refresh_token`，不调用 `/api/v1/auth/revoke-all-sessions`。没有 Refresh Token
  或注销网络失败时只写脱敏告警并清除本地临时材料，不覆盖已获得的快照，也不把账号
  标记为凭据失效。
- 清理通过同步函数的延迟路径覆盖认证成功、认证失败、资源失败、部分分页失败以及
  快照写库前后的返回路径。密码认证最终只持久化认证方式、真实用户名、密码、可选
  User ID、最后认证时间和正常状态；临时 Token、Cookie、Session ID、Token 过期信息
  和刷新状态不会写回长期凭据。当前用户用户名统一从 `username`、`user_name`、`login`
  或邮箱字段提取，并通过同步状态接口安全回填编辑表单。
- 自动配置只对外提交 `auth_type=auto` 和已完成的 `capture_id`。Capture Helper
  版本为 `1.7.0`，候选登录态在提交前通过当前用户接口或 Admin 权限验证，诊断只保留
  存在性、存储键、验证接口、版本和失败阶段。采集结果继续使用现有加密凭据结构；
  浏览器会话属于用户既有登录态，不纳入后台密码会话清理。
- Capture 会话被解析后由 Redis `SET NX` 或单进程内存 claim 原子占用，成功保存渠道
  后一次性消费；后续校验、数据库保存或同步入队失败会释放 claim，claim token 不返回
  给前端或普通 API 响应。

### 9.7 2026-10-04 Capture Helper 登录态等待与平台兼容

**变更前**：Capture Helper 在 `document-start` 阶段立即探测上游登录态，页面尚未完成
登录、SPA 尚未写入 Token/Cookie 或 Cloudflare/Turnstile 尚未完成时，可能把可恢复的
未登录状态提交给完成接口并使 Capture Session 进入失败。NewAPI 只覆盖部分存储键，
Sub2API Session Restore 也没有稳定读取旧版 IndexedDB 客户端 ID。

**变更后**：

- Helper 在 `DOMContentLoaded` 后启动 `runCapture`。候选为空、当前用户接口暂时失败、
  Cookie 尚未形成或登录流程仍在进行时，只显示脱敏等待状态并通过 `scheduleRetry`
  每 3 秒重试，同时保留手动重新采集按钮；不会向完成接口提交 `error`。会话过期、
  来源/版本不匹配或回传服务硬错误才停止重试。
- NewAPI 按 `localStorage.uid`、用户状态对象、页面状态和 JWT 解析数字用户 ID；当前
  用户、Dashboard Refresh 和 `/api/user/token` 请求在可用时携带
  `New-Api-User`、Bearer 和当前站点 Cookie。只有当前用户验证成功并获得可保存 Token
  后才提交采集结果。页面脚本 API 路径只从同源静态脚本发现，保持当前站点来源限制。
- Sub2API 继续读取 `auth_token`、Access/Refresh Token、`auth_user`、hash 和页面状态，
  并读取 `sub2api-auth-coordination` 数据库 `values` 仓库中的
  `sub2api_auth_client_id`，通过 `X-Sub2API-Auth-Client` 调用 Session Restore；恢复
  得到 Access Token 后仍必须通过 `/api/v1/auth/me` 验证。
- Capture 浏览器会话属于用户已有登录态，不进入后台账号密码同步 Cleanup；诊断只保存
  来源、版本、存在性、存储键、验证接口、失败阶段和脱敏错误类别，不保存密码、完整
  Cookie、Token、Session ID、Admin Key 或完整上游响应。Turnstile、WAF、验证码和
  Passkey 仍由目标站浏览器边界处理，NexusTok 不绕过或伪造验证结果。
- Helper 读取 Cookie 时合并页面可见 Cookie 与当前目标站点的 `GM_cookie.list` 结果，
  按名称去重且只允许当前站点或严格 `api.` 父子域；回传只包含扁平身份字段，不携带
  完整 `auth_user` 响应，并保留 handoff 中的管理地址路径。
- 无扩展浏览器使用 `capture_bridge_url` 时，桥接脚本在上游页面通过
  `window.opener.postMessage` 请求一次性 handoff；这样即使登录重定向丢失地址栏参数，
  仍可从管理页恢复同一 Capture Session。管理页只响应活动窗口、目标 Origin 和
  Capture ID 均匹配的请求。由于浏览器禁止跨源 `javascript:` 导航，前端先获取桥接
  脚本并复制不含敏感凭据的短启动片段；用户在上游页面上下文运行片段后，管理页才通过
  `postMessage` 返回桥接脚本，浏览器不会允许 NexusTok 代替用户执行跨源脚本。

### 9.7 2026-10-03 平台认证流程兼容与 NewAPI 密码加密

**变更前**：认证流程 ID 与表单残留用户名、密码的边界没有区分，导致完成账号密码
认证后保存渠道时，后端可能把用户名当作与流程并存的手动凭据并提前失败。NewAPI
新版上游启用密码加密时，NexusTok 也没有在认证架构中明确密钥读取、长密码封装和旧版
回退条件。

**变更后**：前端在账号密码流程完成后只提交 `auth_flow_id` 等流程元数据；后端允许
旧客户端残留用户名，但拒绝密码、User ID、Access/Refresh Token、Cookie、Session ID、
Admin Key 和其它真实认证材料，并使用流程服务端解析出的真实身份写入最终凭据。未有
流程 ID 的新建请求仍使用账号密码，编辑已有密码渠道的空密码仍由后端合并旧密码。

NewAPI 密码同步先读取 `/api/user/login/encryption-key`。启用加密时，短密码使用
RSA-OAEP-SHA256，长密码使用 `password-v2` OAEP 标签和
`password-v2:<kid>` AAD 的 AES-GCM 信封；只有密钥路由明确返回 404/405 才使用明文
协议，其它网络、权限、5xx 或无效配置直接失败。Sub2API 保持邮箱优先、明确凭据错误
或 401 后才回退用户名的顺序，并只用本轮 Refresh Token 执行精确登出。所有路径均
不把密码、Token、Cookie、Session ID 或完整上游响应写入日志、审计字段或普通响应。

### 9.8 2026-10-05 旧版浏览器采集认证候选迁移

**变更前**：浏览器 Capture 已有当前用户验证、Cookie 合并、Session Restore 和
一次性保存边界，但 NewAPI Dashboard Refresh、Sub2API Refresh Token 以及带反向代理
子路径的认证请求仍存在固定路径假设；旧版同源脚本 API 路径发现没有覆盖全部认证候选。

**变更后**：

- NewAPI 自动认证先尝试 Dashboard Refresh，再尝试 Access Token、Admin Key 和 Cookie。
  Refresh 请求继续发送页面 Origin、可用 Cookie 和数字 `New-Api-User`，同时支持从同源
  JavaScript 资源发现 `/api/user/auth/refresh` 的部署路径。新版只需要 Bearer 的
  `/api/user/self` 等接口不强制要求用户头，旧版 Cookie 接口仍可使用数字用户头；
- Sub2API 自动认证继续按 Auth Token、Auth User、Refresh Token、Browser Restore、
  Access Token、Admin Key、Cookie 顺序工作。Refresh Token 只在需要恢复 Access Token
  时请求 `/api/v1/auth/refresh` 及兼容路径，成功后重新调用 `/api/v1/auth/me`、
  `/api/auth/me` 或 `/auth/me` 验证；
- 认证路径会从 handoff 管理地址、回传 `api_base_url`、页面 URL、
  `__APP_CONFIG__` 和存储状态生成候选，剥离 `/api` 与 `/api/v1` 后保留部署前缀。
  同源脚本发现只扫描当前页面同源 JavaScript 和性能资源，不能扩大到任意注册域；
- 认证来源、Origin、Helper 版本、Capture ID、Capture Secret 和用户/管理员验证仍由
  服务端校验。自动模式只接受 `AuthUserVerified` 或 `AdminKeyVerified` 对应的结果，
  不改变完整凭据加密保存、一次性 claim、桥接来源校验或诊断脱敏规则。

### 9.9 2026-10-05 Sub2API Relay 声明复核与采集凭据边界

**变更前**：自动采集验证了登录态和 `auth/me`，但 `api_base_url` 为空的定制
Sub2API 站点可能无法把页面公开 Relay 配置带入完成结果；外部 Relay 的信任判断与
管理会话鉴权边界没有集中说明。

**变更后**：

- Helper 可从 handoff、`__APP_CONFIG__`、页面初始化状态和明确命名的浏览器存储读取
  `custom_endpoints`/`customEndpoints`，只回传规范化存在性和 `relay_base_url`，不回传
  完整页面响应、密码、Cookie 或诊断中的完整 `auth_user`；
- 服务端 Capture 完成校验使用请求 `context.Context` 匿名读取管理页面，要求外部
  Relay 通过现有 URL/SSRF 校验、页面明确声明和规范化完全匹配；管理地址仍受严格
  Host/`api.` 关系约束，不能凭客户端字段、同注册域名或用户 Origin 绕过；
- 指定站点 `https://tk.shour.bond` 的脱敏验收已确认管理地址与
  `https://api-image.shour.bond` Relay 地址分离，登录态通过当前用户验证后保存到加密
  `PlatformSiteCredential`。浏览器 Capture 仍属于用户已有会话，不执行后台密码
  Cleanup；资源失败仍不覆盖最近成功快照。

### 9.10 2026-10-05 Sub2API Access Token 优先与刷新边界

**变更前**：Capture Helper 可能同时提交 Access Token、Refresh Token 和 Auth User；
适配器无条件优先 Refresh 时，失效的旧 Refresh Token 会让有效 Access Token 无法完成
当前用户验证。

**变更后**：

- Sub2API Access Token 存在时先调用当前用户接口，只有明确 HTTP 401 且 Refresh
  Token 存在才刷新；仅有 Refresh Token 时直接刷新。刷新响应成功后保存轮换令牌和
  过期时间，并重新调用 `auth/me` 验证身份；
- 当前用户和刷新保留 `/api/v1`、`/api`、无前缀兼容路径，但只有 404/405 路由缺失
  才继续候选；网络、WAF、权限和非 401 业务失败不触发刷新或主体重试；
- 管理地址与页面声明 Relay 分离，管理请求使用已验证的会话，Relay 模型探测只携带
  完整 Key 的 `Authorization` 和 `x-api-key`。密码、Cookie、Access Token、Refresh
  Token、Session ID、Admin Key 和完整上游响应不进入日志、审计字段或普通响应；
- 真实站点脱敏验收确认 Capture Session 完成并成功验证当前用户；部分资源失败仍
  保留最近成功快照。浏览器采集会话不由后台密码同步 Cleanup，未新增数据库字段、
  迁移或明文凭据结构。

### 9.11 2026-10-05 Sub2API 过期时间标量与恢复前置条件

**变更前**：Sub2API localStorage 的 `token_expires_at` 可能以数字标量保存，采集
逻辑只按对象字段读取时会丢失有效期；缺少 Client ID 或恢复路由时，Session Restore
也可能被误当作必需认证步骤。

**变更后**：Helper 的命名存储读取支持对象和标量 JSON，并将毫秒值归一化为 Unix 秒；
Sub2API 有 Access Token 时先验证 `/api/v1/auth/me`，Browser Restore 只有在发现
`sub2api_auth_client_id` 和同源 `session/restore` 路由后才发送请求。否则仅保留
`not_attempted` 诊断，不产生空请求。该边界不改变 Capture Secret、Origin、Helper
Version、用户/渠道绑定、加密凭据和一次性 claim 校验。

### 9.12 2026-10-05 Sub2API Cookie Client ID 与配置解析边界

**变更前**：认证架构只明确了 localStorage/sessionStorage 和 IndexedDB 的
`sub2api_auth_client_id` 来源，未说明 Cookie 回退和 `JSON.parse` 配置包裹的限制。

**变更后**：Browser Restore 仅按精确 Cookie 名读取 `sub2api_auth_client_id`，不读取
任意 Cookie，也不把值写入诊断；没有 Client ID 或同源 `session/restore` 路由时保持
`not_attempted`。页面 Relay 配置只解析明确命名的 `data`/`config` 等有限包裹字段，
服务端重新以结构化 JSON 解码核对声明；hash Token 仅记录存在性，所有认证材料仍经
当前用户验证、来源校验和整体加密保存。
