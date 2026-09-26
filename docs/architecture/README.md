# NexusTok 功能原理与实现偏差文档

> 文档状态：代码事实基线
> 事实基线日期：2026-09-25
> 主要代码来源：`main.go`、`router/`、`middleware/`、`controller/`、`service/`、`model/`、`relay/`、`pkg/`、`constant/`
> 关联详细文档：[`docs/authentication.md`](../authentication.md)、[`docs/rate-limiting.md`](../rate-limiting.md)、[`docs/key-routing-strategy.md`](../key-routing-strategy.md)、[`docs/upstream-channel-platform-sites.md`](../upstream-channel-platform-sites.md)、[`docs/plugin-api/`](../plugin-api/)

## 文档目的

本目录以当前仓库代码为事实基线，记录 NexusTok 的主要功能原理、调用边界、数据流和已确认的实现偏差。它不是对未来设计的承诺，也不是将 README、外部文档或渠道名称推断为已实现能力的清单。

阅读本文档时，优先以“当前代码实际行为”为准，并在需要查看参数级契约、配置项和安全边界时进入对应的专项文档。若实现与设计目标不一致，应在同一次功能变更中更新专项文档和 [`implementation-deviations.md`](./implementation-deviations.md)。

## 推荐阅读顺序

1. [`system-overview.md`](./system-overview.md)：进程、模块和请求总链路。
2. [`authentication-and-authorization.md`](./authentication-and-authorization.md)：身份、会话、授权和敏感操作。
3. [`relay-routing-and-conversion.md`](./relay-routing-and-conversion.md)：渠道选择、协议入口和请求转换。
4. [`billing-and-quota.md`](./billing-and-quota.md)：预扣、结算、退款和额度安全。
5. [`tasks-and-plugins.md`](./tasks-and-plugins.md)：异步任务与 Sobek 插件边界。
6. [`data-cache-and-background-jobs.md`](./data-cache-and-background-jobs.md)：数据库、缓存和后台任务。
7. [`provider-capability-matrix.md`](./provider-capability-matrix.md)：渠道注册、Adaptor 和 Endpoint 能力矩阵。
8. [`implementation-deviations.md`](./implementation-deviations.md)：可由代码直接证明的缺口和偏差登记。

## 功能到文档的映射

| 维护主题 | 首要文档 | 必须一并核对 |
| --- | --- | --- |
| 进程启动、模块边界、主从节点 | [`system-overview.md`](./system-overview.md) | `main.go`、`router/`、`model/main.go` |
| 登录、Session、Token、OAuth、Passkey、TOTP | [`authentication-and-authorization.md`](./authentication-and-authorization.md) | [`authentication.md`](../authentication.md)、`middleware/`、`service/auth*`、`model/user_session.go` |
| IP/用户/模型限流和并发 | [`authentication-and-authorization.md`](./authentication-and-authorization.md) | [`rate-limiting.md`](../rate-limiting.md)、`middleware/*limit*` |
| 渠道、密钥、模型获取和路由 | [`relay-routing-and-conversion.md`](./relay-routing-and-conversion.md) | [`key-routing-strategy.md`](../key-routing-strategy.md)、[`upstream-channel-platform-sites.md`](../upstream-channel-platform-sites.md)、`middleware/distributor.go`、`model/upstream_routing.go` |
| 新 Relay Format、Endpoint、请求转换 | [`relay-routing-and-conversion.md`](./relay-routing-and-conversion.md) | `router/relay-router.go`、`relaykit/`、`relay/channel/` |
| 价格、倍率、表达式、额度 | [`billing-and-quota.md`](./billing-and-quota.md) | `pkg/billingexpr/expr.md`、`service/billing*.go`、`common/quota_math.go` |
| 视频、音乐、异步任务和任务产物 | [`tasks-and-plugins.md`](./tasks-and-plugins.md) | `service/task_polling.go`、`model/task.go`、`router/task-router.go`、`router/video-router.go` |
| Sobek 插件、协议和插件版本 | [`tasks-and-plugins.md`](./tasks-and-plugins.md) | [`plugin-api/README.md`](../plugin-api/README.md)、[`plugin-api/v1.md`](../plugin-api/v1.md)、`pkg/jsplugin/` |
| 数据库、缓存、迁移、后台任务 | [`data-cache-and-background-jobs.md`](./data-cache-and-background-jobs.md) | `model/main.go`、`common/redis.go`、`service/system_task.go` |
| 新增或调整渠道能力 | [`provider-capability-matrix.md`](./provider-capability-matrix.md) | `constant/channel.go`、`common/api_type.go`、`relay/relay_adaptor.go` |
| 已确认的未实现或行为不一致 | [`implementation-deviations.md`](./implementation-deviations.md) | 具体路由、Adaptor 方法和测试证据 |

## 代码来源目录

- `main.go`：资源初始化、HTTP Server、路由安装和后台任务启动。
- `router/`：Gin 路由挂载、协议入口和中间件顺序。
- `middleware/`：Token、Session、分发、限流、插件 pinning 和请求保护。
- `controller/`：HTTP 参数解析、业务入口、响应和错误映射。
- `service/`：计费、鉴权会话、任务轮询、系统任务和跨模块业务流程。
- `model/`：GORM 模型、数据库访问、缓存索引、迁移和锁。
- `relay/`：Relay 运行时、Adaptor、协议转换、上游请求和 Usage。
- `relaykit/`：独立的协议 DTO、转换器和公共 Relay 类型。
- `pkg/jsplugin/`、`plugins/tasks/`：插件运行时、注册表、路由声明和内置任务插件。
- `constant/`、`common/`、`setting/`、`types/`：类型、配置、倍率、公共安全和账务约束。
- `web/`、`electron/`：管理面板和桌面封装；它们调用 Go 网关，不拥有后端鉴权、计费或数据库权威。

## 文档状态定义

| 状态 | 含义 | 写法要求 |
| --- | --- | --- |
| 已实现 | 当前代码中存在可追溯入口，并能由路由、服务、模型或测试直接证明 | 写明代码路径和关键方法 |
| 未实现 | 代码显式返回 `RelayNotImplemented`、`not implemented`，或没有对应运行入口且有直接证据 | 只描述具体 Endpoint 或方法，不扩大为整个供应商 |
| 设计目标 | 现有详细文档、协议或注释中表达的目标，但尚未由运行代码完全证明 | 与“已实现”分栏，不能作为实际能力结论 |
| 待核查 | 目前只有名称、局部代码或间接线索，证据不足以判断端到端行为 | 不得写成支持或不支持 |
| 已确认偏差 | 预期与当前代码可以同时被具体证据证明不一致 | 登记到偏差表，附最近核查日期 |

## 判断偏差的方法

1. 先定位实际路由，再定位中间件顺序和 Controller。
2. 沿 `RelayInfo`、Context key、服务调用和模型查询追踪到具体 Adaptor 或插件。
3. 检查成功路径、重试路径、失败回退、Usage 和日志路径，不能只看类型注册。
4. 将“渠道已注册”“Adaptor 存在”“模型可以被选中”“某个 Endpoint 已实现”分开判断。
5. 对“未实现”只引用显式代码证据；对没有证据的内容标记“待核查”。
6. 变更后在文档中记录实际日期，并用“变更前/变更后”描述行为差异。

## 后续维护入口

新增渠道时至少同步 `constant/channel.go`、`common/api_type.go`、`relay/relay_adaptor.go`、流式支持列表、模型获取入口、能力矩阵和偏差登记。新增 Endpoint 或 Relay Format 时同步路由、`relaykit` DTO/转换器、Usage/计费入口、能力矩阵和相关专项文档。调整密钥路由、模型限制、平台站点子密钥或日志可观测字段时，以 [`key-routing-strategy.md`](../key-routing-strategy.md) 为细节基准。

新增配置、数据库模型、缓存键、后台任务或插件协议时，同时更新对应功能文档、代码来源和变更记录。只修改文档时，也必须静态检查链接、路径、方法名、接口参数和偏差状态。

## 2026-09-25 平台站点同步事实

本次 NewAPI/Sub2API 优化前，`password` 模式主要依赖账号密码或一次性浏览器采集结果，缓存登录态未完整合并，登录、快照读取和失败回退也没有统一的阶段状态。优化后，密码模式同时保留用户名、密码和浏览器或登录流程得到的 Access Token、Refresh Token、过期时间、User ID、Cookie；恢复优先级固定为 Access Token、Refresh Token、Cookie、账号密码。`access_token`、`admin_key` 和 `cookie` 模式不回退账号密码。

同步被拆为 `authentication`、`current_user`、`balance_usage`、`groups_rates`、
`key_pagination`、`key_secrets`、`key_models`、`sub2api_endpoints` 八个固定阶段，
状态保存于 `platform_site_accounts.sync_stages` 的 TEXT JSON。成功、警告、失败、等待
和跳过均可由管理员状态接口查看；可选阶段失败保留旧余额、用量、倍率、地址、模型能力
或密钥快照，密钥分页失败不清理旧密钥。

Turnstile、CAPTCHA、Cloudflare、WAF 和其它交互验证不由后台绕过，也不接入第三方打码
服务。认证遇到验证页或验证错误时停止备用登录和无意义重试，使用真实浏览器完成验证
后通过 Capture Session 采集合法登录态，或创建五分钟 Challenge。Challenge 只在缓存中
保存必要的加密 pending Cookie 或短期临时令牌，最多三次，绑定渠道、平台、Origin 和
手动创建管理员；Redis 使用随机锁值和 Lua 校验释放，Redis 不可用时退回进程内锁。
验证码成功消费 Challenge 后才继续完整同步；后台同步在等待期间不再次登录，过期后只
提示管理员重新同步。

同步诊断只允许保存脱敏错误摘要、HTTP 状态、脱敏 URL、Content-Type、重定向状态和
响应类别。`2xx` HTML、验证页、纯文本代理响应和非法 JSON 分开分类；密码、OTP、
Cookie、Access Token、Refresh Token、Admin Key 和完整响应体不进入
`last_sync_error`、系统日志、阶段 JSON 或管理员接口。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立架构文档总索引、维护入口、状态定义和偏差判断方法 | `docs/architecture/`、后续功能文档维护流程 | `main.go`、`router/`、`middleware/`、`service/`、`model/`、`relay/`、`pkg/` 静态核对 |
| 2026-09-25 | 平台站点同步优化 | 密码模式、交互验证、快照失败回退和阶段可观测性分散在局部实现 | 统一双凭据恢复顺序、八阶段状态、Challenge 生命周期、验证页分类和敏感信息边界 | NewAPI、Sub2API、管理员同步、后台任务和密钥路由 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/upstream_site_challenge.go`、`model/upstream_site_sync_stages.go` |
