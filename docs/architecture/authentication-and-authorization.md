# 鉴权、会话与授权原理

> 文档状态：代码事实基线
> 事实基线日期：2026-09-25
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
| `user_sessions` | 数据库中的登录会话控制面，记录 SID、设备、IP、方式、过期和撤销 | Session 服务和 Redis Session 快照 |
| API Token | Relay API 的调用凭据，关联用户、分组、模型限制、额度和状态 | `middleware.TokenAuth`、Token 模型查询/缓存 |
| PAT | 面向个人/管理自动化的长期个人访问凭据，权限由对应用户和 Token 状态决定 | Token 相关 Controller/Service |
| OAuth/OIDC | 外部身份提供商登录、绑定或回调，最终进入统一 Session 签发出口 | `oauth/`、用户 Controller、Session 服务 |
| Passkey/WebAuthn | 无密码或二次验证凭据，验证成功后进入统一登录或敏感操作流程 | WebAuthn Controller/Service |
| TOTP/2FA | 密码登录或敏感操作的附加因子 | 2FA/安全验证服务 |
| Security Proof | 由 `SESSION_SECRET` 派生的短期敏感操作证明，不是永久登录凭据 | 敏感操作 Controller/Service |
| 任务产物 Capability URL | 仅用于任务 Artifact 读取，由 `CRYPTO_SECRET` 签名 | `middleware/task_artifact_access.go`、插件产物服务 |

`SESSION_SECRET` 为 Access Token、Security Proof、Refresh Token 摘要和 AuthFlow 摘要派生不同用途的密钥。多节点必须保持一致；`CRYPTO_SECRET` 影响缓存键摘要和 Artifact Capability，多节点共享 Redis 或任务产物访问时也必须一致。

### 2.1 平台站点双凭据与自动恢复

平台站点的账号密码认证与面板登录不是同一套用户 Session。平台站点在
`platform_site_accounts.credential_ciphertext` 中加密保存
`PlatformSiteCredential`，其中账号密码和上一次成功登录得到的登录态是两类并存凭据：

- `Username`/`Password`：登录态全部失效后的最终恢复凭据；
- `AccessToken`/`RefreshToken`/`TokenExpiresAt`/`UserID`：日常同步优先复用的登录态。

`auth_type=password` 时，编辑保存会保留这两类字段；保存页面没有提交令牌字段时，
不会用空值覆盖已有登录态。切换到 `access_token`、`admin_key` 或 `cookie` 时仍按
认证方式清理不属于该模式的字段，避免认证方式漂移。

平台站点同步的认证顺序由
`service/upstream_site_adapters.go` 的 `tryCachedPlatformSiteCredential` 统一控制：

1. 未明确过期的 `AccessToken` 先调用当前用户接口验证；
2. 访问令牌失败且存在 `RefreshToken` 时调用平台刷新接口；
3. 刷新成功后使用新访问令牌再次验证当前用户，并产生 `CredentialUpdate`；
4. 只有密码模式的访问令牌和刷新令牌都不可用时，才回退账号密码登录；
5. `access_token` 模式刷新失败直接认证失败，不读取账号密码回退。

NewAPI 使用 `/api/user/self` 等当前用户接口和 `/api/user/auth/refresh`，Sub2API
使用 `/api/v1/auth/me` 等当前用户接口和 `/api/v1/auth/refresh`。账号密码登录或
刷新产生的新令牌会写入 `PlatformSiteSession.CredentialUpdate`；宿主
`syncPlatformSite` 在余额、用量、模型和子密钥快照同步前立即加密写回数据库。因此
令牌轮换后即使后续快照失败，下次同步仍使用最新令牌。登录响应没有新的
`RefreshToken` 时会清除旧刷新令牌，避免继续使用已经失效的旧值。

账号密码、令牌、Cookie 和 Admin Key 只存在加密凭据字段或运行时请求头中，不写入
同步状态、系统日志、接口响应或诊断摘要。需要验证码、Turnstile、Cloudflare/WAF
或其它交互验证时，后台不尝试绕过；只有缓存登录态无法自动恢复时才需要浏览器采集
Access Token 或 Cookie。

## 3. 面板登录与 Session 生命周期

登录、Passkey、OAuth、WeChat、Telegram 和 2FA 成功路径最终都应通过统一 Session 签发出口，生成 `user_sessions` 记录、Access Token 和 Refresh Cookie。Session 数据库状态是最终权威；Redis 保存用户鉴权快照和 Session 快照，缓存未命中或未启用 Redis 时回源数据库。

Refresh 采用轮换策略。客户端通过内存保存的 SID 和 `X-Auth-Session` 辅助避免 Cookie 与当前标签页身份错配；服务端在不匹配时返回冲突，不执行轮换/撤销。前端使用 Web Locks 和 BroadcastChannel 或 storage 事件协调同一浏览器配置文件中的刷新，但不跨标签传递 Access Token 或 Refresh Token。

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
- OAuth、Passkey、TOTP、PAT 和 Security Proof 的端到端组合场景不能由单个入口文件证明；尚未覆盖的组合场景应标记“待核查”，不能据名称推断。
- 平台站点的账号密码能否自动恢复，仍取决于上游登录接口是否可用以及是否要求交互验证；
  本地保留双凭据不会绕过上游风控，也不保证所有派生平台都支持相同的刷新协议。

## 9. 维护时需要同步的关联模块

修改登录、刷新、退出、Session、用户状态、鉴权版本、授权策略、Token 模型、OAuth/OIDC、Passkey、TOTP 或安全证明时，必须同步本文档、[`authentication.md`](../authentication.md)、必要时的[`rate-limiting.md`](../rate-limiting.md)和偏差登记。修改 Redis 键、TTL、缓存回退或多节点传播时同步[`data-cache-and-background-jobs.md`](./data-cache-and-background-jobs.md)。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立凭据分层、Session 生命周期、授权和 Redis 拓扑说明 | `middleware/`、`service/auth*`、`model/user_session.go`、`oauth/` | `docs/authentication.md`、`model/user_session.go`、`service/auth_session.go` 静态核对 |
| 2026-09-25 | 缺陷修复 | 平台站点密码模式保存时可能清空登录态，且同步每次优先重新登录；认证成功后的令牌更新依赖快照同步成功 | 密码与登录态双凭据并存，优先复用访问令牌、再刷新、最后回退账号密码；认证产生的令牌在快照前立即持久化 | NewAPI/Sub2API 平台站点同步、账号密码恢复、令牌轮换和敏感信息保护 | `controller/upstream_channel.go`、`service/upstream_site_adapters.go`、`service/upstream_site.go`；服务和控制器回归测试 |
