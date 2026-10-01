# NexusTok 功能原理与实现偏差文档

> 文档状态：代码事实基线
> 事实基线日期：2026-10-01
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
8. [`../platform-site-resource-acquisition-comparison.md`](../platform-site-resource-acquisition-comparison.md)：旧版备份、Sub2API/New API 参考源和当前 NexusTok 的平台站点资源获取链路与差异。
9. [`implementation-deviations.md`](./implementation-deviations.md)：可由代码直接证明的缺口和偏差登记。

## 功能到文档的映射

| 维护主题 | 首要文档 | 必须一并核对 |
| --- | --- | --- |
| 进程启动、模块边界、主从节点 | [`system-overview.md`](./system-overview.md) | `main.go`、`router/`、`model/main.go` |
| 登录、Session、Token、OAuth、Passkey、TOTP | [`authentication-and-authorization.md`](./authentication-and-authorization.md) | [`authentication.md`](../authentication.md)、`middleware/`、`service/auth*`、`model/user_session.go` |
| IP/用户/模型限流和并发 | [`authentication-and-authorization.md`](./authentication-and-authorization.md) | [`rate-limiting.md`](../rate-limiting.md)、`middleware/*limit*` |
| 渠道、密钥、模型获取和路由 | [`relay-routing-and-conversion.md`](./relay-routing-and-conversion.md) | [`key-routing-strategy.md`](../key-routing-strategy.md)、[`upstream-channel-platform-sites.md`](../upstream-channel-platform-sites.md)、`middleware/distributor.go`、`model/upstream_routing.go` |
| New API/Sub2API 平台站点资源链路 | [`../platform-site-resource-acquisition-comparison.md`](../platform-site-resource-acquisition-comparison.md) | [`upstream-channel-platform-sites.md`](../upstream-channel-platform-sites.md)、`service/upstream_site.go`、`service/upstream_site_adapters.go`、`controller/upstream_channel.go`、本机参考源和旧版备份 |
| 新 Relay Format、Endpoint、请求转换 | [`relay-routing-and-conversion.md`](./relay-routing-and-conversion.md) | `router/relay-router.go`、`relaykit/`、`relay/channel/` |
| 价格、倍率、表达式、额度 | [`billing-and-quota.md`](./billing-and-quota.md) | `pkg/billingexpr/expr.md`、`service/billing*.go`、`common/quota_math.go` |
| 视频、音乐、异步任务和任务产物 | [`tasks-and-plugins.md`](./tasks-and-plugins.md) | `service/task_polling.go`、`model/task.go`、`router/task-router.go`、`router/video-router.go` |
| Sobek 插件、协议和插件版本 | [`tasks-and-plugins.md`](./tasks-and-plugins.md) | [`plugin-api/README.md`](../plugin-api/README.md)、[`plugin-api/v1.md`](../plugin-api/v1.md)、`pkg/jsplugin/` |
| 数据库、缓存、迁移、后台任务 | [`data-cache-and-background-jobs.md`](./data-cache-and-background-jobs.md) | `model/main.go`、`common/redis.go`、`service/system_task.go` |
| 系统维护、版本更新与可重复回滚 | [`system-overview.md`](./system-overview.md)、[`tasks-and-plugins.md`](./tasks-and-plugins.md) | `controller/system_update.go`、`service/system_update.go`、`service/system_update_docker.go`、`model/system_task.go`、维护页面 |
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
- `controller/system_update.go`、`service/system_update*.go`：Root 维护接口、GitHub Release
  检查、裸机二进制交换、Docker Engine helper、重启探活和任务终态。

本次登录会话变更的代码事实入口为 `service/auth_session.go`、`service/login_verification.go`、
`model/user_session.go`、`model/login_verification.go`、`controller/user.go` 和
`controller/login_verification.go`；产品默认名称及旧配置兼容迁移入口为
`common/constants.go`、`model/frontend_option_migration.go` 和 `main.go`。

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

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立架构文档总索引、维护入口、状态定义和偏差判断方法 | `docs/architecture/`、后续功能文档维护流程 | `main.go`、`router/`、`middleware/`、`service/`、`model/`、`relay/`、`pkg/` 静态核对 |
| 2026-09-26 | 平台站点同步回归修复 | 渠道 2、4、5 的登录响应分类、兼容回退边界和失败快照语义未在总索引中记录 | 记录真实登录 DTO、交互验证/WAF/网络诊断、404/405 回退和旧快照保留规则，并链接专项文档与测试入口 | 平台站点认证、同步、资源快照、路由可用性和管理员诊断 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/upstream_site_test.go`、`docs/upstream-channel-platform-sites.md` |
| 2026-09-27 | 面板 Session 复用与品牌默认值 | 登录 Session 和默认产品名的当前事实入口未在总索引中明确 | 登记浏览器 SID 定位、原行复用/凭据轮换、默认活跃上限 `50`、AuthFlow 回滚以及 `NexusTok` 默认名称和旧配置迁移 | 登录、会话限制、前端展示和配置初始化 | 认证专项文档、限流专项文档、`model/frontend_option_migration.go`、回归测试 |
| 2026-09-27 | NewAPI 类平台渠道同步修复 | Dashboard Refresh Cookie、Session ID、Bearer Token、管理地址边界和资源失败回退未在索引中统一登记，渠道 4、5 的失败难以区分认证与资源权限 | 登记 CookieJar 优先级、现代 Bundle 严格校验、仅 404/405 兼容回退、认证成功后保存身份/余额及最近成功资源快照，并链接实现偏差和路由文档 | NewAPI 派生平台认证、资源同步、快照、路由和管理员诊断 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`docs/upstream-channel-platform-sites.md`、`docs/architecture/implementation-deviations.md` |
| 2026-09-27 | 平台站点渠道代理接入 | 平台站点同步和 Auth Flow 未读取渠道代理，通用客户端注入可能覆盖平台会话 CookieJar、超时和重定向边界 | NewAPI/Sub2API 按渠道 `setting.proxy`、HTTP 协议和连接分片复用 Transport；平台会话策略保持不变，认证、网络、安全验证和资源失败继续分层 | 渠道 4 局域网代理同步、渠道 5 认证诊断、平台站点管理面请求 | `service/upstream_site.go`、`service/platform_site_auth_flow.go`、`service/upstream_site_test.go`、`docs/upstream-channel-platform-sites.md` |
| 2026-09-27 | 按旧版协议校准 NewAPI 同步 | 当前 NewAPI Token 主分页使用零起始参数，旧版主请求顺序、批量 Key 补偿和核心成功边界未在总索引中明确 | 固定 `p=1&page_size=100` 一基分页、旧版登录字段和 status/self/groups/ratio/token/key 顺序；只在 404/405 回退，部分资源失败保留最近成功快照，账号级模型不复制给子密钥 | NewAPI 及派生平台站点同步、渠道状态和路由模型能力 | `service/upstream_site_adapters.go`、`service/upstream_site.go`、`service/upstream_site_test.go`、`docs/upstream-channel-platform-sites.md` |
| 2026-09-29 | 增加平台站点资源获取链路索引 | 架构总索引只有平台站点专项设计和能力矩阵，无法直接定位旧版备份、参考平台真实路由与当前资源快照的逐项比较 | 增加平台站点资源获取比较文档，明确参考平台、旧版 NexusTok 和当前实现三类证据，并说明管理端额度、Key 模型和失败回退边界 | 平台站点认证、资源同步、Key 生命周期、模型能力和路由快照 | [`../platform-site-resource-acquisition-comparison.md`](../platform-site-resource-acquisition-comparison.md)、`service/upstream_site.go`、`controller/upstream_channel.go`、本机参考源静态核对 |
| 2026-09-30 | 恢复系统维护更新与可重复回滚 | 维护页面直接从浏览器请求 GitHub Release，只能查看说明；没有后端更新、回滚、重启任务边界，旧版回滚会消耗 `.backup` | 后端经 RootAuth 检查 Release 并通过 SystemTask 应用更新、回滚、重启；裸机使用稳定 `.backup` 交换，Docker 使用 socket/helper 和健康检查；未修改数据库结构 | Root 管理、版本检查、二进制/Docker 更新、任务租约、审计和前端维护面板 | `controller/system_update.go`、`service/system_update.go`、`service/system_update_docker.go`、`model/system_task.go`、维护页测试；未进行生产容器切换 |
| 2026-09-30 | v0.2.2 生产默认部署与发布入口 | 生产 Compose 的数据库/缓存版本、密码和对外端口不够明确，单容器示例没有说明不包含外部数据库/缓存 | 生产推荐入口固定为 Compose，默认 PostgreSQL 15 + Redis 7，应用端口 `3030`，密码由 `.env`/部署脚本生成，发布镜像为 `c1cadabob/nexustok:v0.2.2` 与 `latest`；SQLite/无 Redis 回退和旧前端路由兼容逻辑保持不变 | `docker-compose.yml`、`scripts/deploy.sh`、`Dockerfile`、`.github/workflows/docker-*.yml`、中文部署文档 | `docker compose config`、脚本语法检查、隔离三服务栈和真实 SQLite 3.50.4/MySQL 8.2.0/PostgreSQL 15.19 矩阵已通过；完整生产 Dockerfile 构建因 `proxy.golang.org` 超时未完成，最低版本、独立日志库和远端发布待标签工作流验证 |
| 2026-10-01 | v0.2.3 安全依赖、部署文档与发布入口 | README 多语言仍包含独立宣传栏目和过时的单容器部署说明；Docker Hub Secret 名称与当前项目环境变量不一致；安全依赖修复和发布验证边界未集中记录 | 六种 README 删除指定宣传栏目/图片并保留部署必需链接；补充单机、多机、宝塔、备份、回滚和故障排查边界；工作流统一使用 `DOCKER_USERNAME`/`DOCKER_PASSWORD`；前端和 Electron 直接依赖按本地审计结果做最小升级；发布目标为 `c1cadabob/nexustok:v0.2.3` 和 `latest`，未新增数据库模型、字段、迁移或业务 API | `README*.md`、`docs/installation/BT.md`、`.github/workflows/docker-*.yml`、`web/package.json`、`electron/package.json`、发布说明 | 已完成本地 Bun/npm 审计和前端聚焦回归；远端 Dependabot 需要认证 API，真实开放警报、三服务生产镜像拉取、多架构 manifest 和 Docker Hub 发布待环境凭据/工作流验证 |
