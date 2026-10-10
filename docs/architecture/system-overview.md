# 系统总览与请求生命周期

> 文档状态：代码事实基线
> 事实基线日期：2026-10-08
> 主要代码来源：`main.go`、`router/main.go`、`router/`、`middleware/`、`controller/`、`service/`、`model/main.go`、`common/`
> 关联详细文档：[`README.md`](./README.md)、[`authentication-and-authorization.md`](./authentication-and-authorization.md)、[`data-cache-and-background-jobs.md`](./data-cache-and-background-jobs.md)

## 1. 功能目标和边界

NexusTok 是一个 Go AI API Gateway。Go 进程对外提供管理 API、统一 Relay API、任务和插件协议，负责身份识别、权限、渠道选择、上游协议适配、计费、日志和后台任务。`web/` 是管理面板，`electron/` 是桌面封装，`relaykit/` 是独立 Go 模块，三者都不是主数据库或计费权威。

本文档描述模块边界和请求总链路，不替代鉴权、路由、计费、插件协议的参数级契约。

## 2. 进程启动与资源初始化

入口是 `main.main`。当命令行为 `plugin` 时进入 `pkg/jsplugin.RunCLI`；普通启动顺序如下：

1. 设置 `relaykit` 日志和系统错误回调。
2. `InitResources` 加载 `.env`，初始化环境变量、日志、倍率设置、HTTP 客户端、Token 编码器。
3. `model.InitDB` 选择主数据库并在 Master 节点执行迁移；初始化 Casbin 授权、密码加密和 Setup 状态。
4. 初始化选项、日志数据库、Redis、性能指标、系统监控和 i18n。
5. 启动鉴权产物清理、渠道缓存、选项同步、插件同步、授权策略同步、数据看板。
6. 按配置启动渠道自动更新、Codex 凭据刷新、订阅额度重置、节点状态上报。
7. 注册渠道检测、上游模型更新、异步任务轮询和系统维护更新/回滚等系统任务，启动
   `service.StartSystemTaskRunner`。
8. 创建 Gin Server，配置可信代理、Request ID、版本、i18n、日志和静态前端，再由
   `router.SetRouter` 安装路由。以 `system-update-helper` 子命令启动时走最小初始化路径，
   只为 Docker 更新 helper 准备数据库、任务租约和 Docker 操作，不启动 HTTP、Redis
   runner 或其它后台服务。

主节点和从节点的职责由 `common.IsMasterNode` 及各个任务实现共同控制。系统任务 Runner 只在 Master 节点启动；从节点可以提供请求服务，但不能被文档理解为拥有独立的迁移、调度或账务权威。

## 3. 模块职责边界

| 层 | 当前职责 | 不应假设拥有的权威 |
| --- | --- | --- |
| `router/` | 声明路径、HTTP 方法、中间件顺序和协议入口 | 不负责最终鉴权、渠道选择或结算 |
| `middleware/` | CORS、解压、请求体保护、Token/Session、限流、分发、插件 pinning | 不替代 Controller 的参数业务校验 |
| `controller/` | 解析请求、组装 DTO、调用 Relay/Service、写 HTTP 响应 | 不应被视为上游协议实现本身 |
| `service/` | 计费、鉴权会话、任务轮询、系统任务、系统更新和跨模型业务流程 | 不直接改变 Relay DTO 的协议语义 |
| `model/` | GORM 模型、数据库查询、迁移、锁、缓存索引和持久化状态 | 缓存不是数据库最终权威 |
| `relay/` | Relay 生命周期、Adaptor 调用、协议转换、流式处理、Usage 和错误映射 | 不拥有数据库连接或插件持久化权限 |
| `relaykit/` | DTO、Relay Format、转换器和独立公共类型 | 不依赖根模块的数据库、配置、鉴权或计费 |
| `pkg/jsplugin/` | Sobek 编译、校验、注册、路由和 Generation Pinning | 不拥有 fetch、文件系统、数据库、计费和结算 |
| `web/` | 管理页面、配置和用户操作展示 | 前端状态不能替代服务端校验 |
| `electron/` | 桌面壳和本地启动/访问体验 | 不改变 Go 网关的业务权威 |

## 4. 路由和请求总链路

普通管理/API 请求通常经过：

```text
HTTP
  -> router/main.go 挂载的具体路由
  -> 通用中间件（Request ID、版本、i18n、CORS、解压、请求体限制）
  -> TokenAuth / UserAuth / AdminAuth / Session 相关中间件
  -> 限流和性能检查
  -> Controller
  -> Service / Model
  -> JSON 或流式响应
```

模型 Relay 请求的核心链路是：

```text
客户端请求
  -> TokenAuth
  -> ModelRequestRateLimit
  -> Distribute
     -> 解析模型、Token 模型限制和渠道约束
     -> 选择用户组、Auto Group、亲和渠道和可用渠道
     -> 建立 RelayInfo / ChannelMeta
  -> Controller.Relay 或具体 Handler
  -> Relay Format DTO 解析和请求校验
  -> 渠道模型映射、参数/Header 覆盖和协议转换
  -> Adaptor.DoRequest / WebSocket / Task Plugin
  -> 上游响应转换、流式输出、Usage 统计和错误归一化
  -> BillingSession 结算或退款
  -> 消费日志、请求日志和管理员诊断字段
```

`middleware/distributor.go` 负责把选择结果放入 Context；`relay/common/relay_info.go` 的 `RelayInfo` 承载单次请求的模型、渠道、计费、转换链、Usage 和日志信息。重试时会重新初始化按尝试作用域的数据，但请求级计费、错误和诊断仍属于同一次请求。

## 5. 部署边界

### 主数据库

`model.chooseDB` 允许主数据库使用 SQLite、MySQL 或 PostgreSQL。`model.InitDB` 设置数据库类型、初始化列引用和 GORM 连接；Master 节点执行迁移，SQLite 使用本地文件，MySQL/PostgreSQL 使用 `SQL_DSN`。

### 日志数据库

日志库通过 `LOG_SQL_DSN` 单独选择；可使用 SQLite、MySQL、PostgreSQL 或 ClickHouse。主库不允许配置 ClickHouse。日志模型和日志查询必须根据日志数据库方言处理保留字和布尔值。

### Redis 与内存回退

`common.InitRedisClient` 根据 `REDIS_CONN_STRING` 决定是否启用 Redis；不少缓存和限流逻辑在 Redis 不可用时回退到进程内存或数据库。共享 Redis 才能让需要全局共享的缓存/计数在节点间共享，独立 Redis 会形成节点局部状态，具体 Session 语义见[`authentication-and-authorization.md`](./authentication-and-authorization.md)。

### 生产默认部署（2026-09-30）

**变更前**：生产 Compose 使用浮动的 PostgreSQL/Redis 镜像标签、仓库内默认密码和相对
数据目录；NexusTok 只等待服务启动，没有按数据库和缓存健康状态编排。单条 `docker run`
示例容易被误解为同时提供 PostgreSQL 和 Redis。

**变更后**：`docker-compose.yml` 是生产推荐入口，固定使用
`postgres:15-alpine`、`redis:7-alpine` 和 `c1cadabob/nexustok:latest`。部署脚本首次运行
生成权限为 `0600` 的 `.env`，保存 `POSTGRES_PASSWORD` 与 `REDIS_PASSWORD`，Compose
据此注入 `SQL_DSN` 和 `REDIS_CONN_STRING`；已有密码不会被覆盖。PostgreSQL 和 Redis
只加入 Compose 内部网络，NexusTok 依赖两个服务的 `service_healthy` 条件启动，生产
对外端口统一为 `3030`。

应用数据和由 `SESSION_SECRET_FILE=/data/session_secret` 自动生成的会话密钥持久化到
`/opt/nexustok/data`，日志持久化到 `/opt/nexustok/logs`。未设置 `SQL_DSN` 或
`REDIS_CONN_STRING` 时，代码仍分别回退到 SQLite 或关闭 Redis，以兼容裸机、测试和
既有外部数据库部署。单条 `docker run` 不会创建 PostgreSQL/Redis，只表示 SQLite/无
Redis 兼容模式。SQLite 文件不会自动迁移到 PostgreSQL；已有生产数据切换前必须备份，
并单独执行经过验证的数据迁移。

本次发布目标镜像为 `c1cadabob/nexustok:v0.2.8` 和 `c1cadabob/nexustok:latest`，Dockerfile
通过当前 `web/` 的 Bun/Rsbuild `bun run build` 生成并嵌入新版前端，不新增路由、DTO、
数据库模型或字段。`v0.2.8` 包含平台站点资源同步边界修复；GitHub Release、Docker 多架构
镜像、Cosign 签名及 Electron 产物以标签触发后的远端工作流结果为准。17 个目标站点的
重新登录和六类资源全成功仍须在远端更新后逐站验证，不由本地测试或旧快照代替。

### v0.2.8 平台站点资源同步边界（2026-10-10）

**变更前**：New API 可选 `/api/ratio_config` 权限失败可能被误报为端点失败；账号级模型
目录、重复 pricing Endpoint 或本轮刚写入的资源可能使单 Key 能力、端点状态和
`using_snapshot` 标记失真。Sub2API 管理请求与 Relay 探测也需要严格隔离。

**变更后**：New API 单 Key 模型必须由对应完整 Key 的 Relay 探测确认，`/api/pricing`
按有效定价/分组/端点信息判断且能力去重；Sub2API 管理地址与页面声明 Relay 地址分离，
单 Key 探测不继承管理会话请求头。六类资源状态逐轮补齐，历史快照只按事务开始前数据
识别；权限、安全验证、分页或部分探测失败保留最近成功快照，不转换为密钥不存在。

本版本不新增数据库字段、迁移或明文凭据结构。远端更新和 17 个真实站点的重新登录及
六类资源验收结果由 `docs/platform-site-resource-sync-v0.2.8-review.md` 记录；未逐项确认
前不声称验收完成。

### v0.2.3 部署文档与发布边界（2026-10-01）

**变更前**：六种 README 的部署内容不一致，部分语言仍使用 `3000`、旧版镜像或
明文 MySQL 示例；独立文档没有完整说明 Compose 单机拓扑、多机共享服务、备份回滚、
反向代理和 Docker socket 风险。Docker Hub 工作流使用的 Secret 名称也与执行环境不一致。

**变更后**：README 和宝塔部署文档统一以 `3030`、Compose、PostgreSQL 15、Redis 7、
`.env` 中的随机密码和 `c1cadabob/nexustok:v0.2.4`/`latest` 为生产事实基线；单容器
命令明确为 SQLite/无 Redis 兼容模式。多机文档要求共享 `SQL_DSN`、`REDIS_CONN_STRING`、
`SESSION_SECRET`、`CRYPTO_SECRET`，使用唯一 `NODE_NAME`，主节点负责迁移和系统任务，
从节点使用 `NODE_TYPE=slave`。反向代理、SSE/WebSocket、可信代理、Cookie 安全、备份、
滚动升级和故障摘除边界均记录在部署文档中。Docker 工作流统一读取
`DOCKER_USERNAME`/`DOCKER_PASSWORD`。

本版本只修改 web/Electron 依赖、发布工作流和文档，没有修改数据库代码、GORM、数据库
驱动、Schema、迁移、路由、DTO 或 API 契约；因此不新增数据库兼容性结论。截至
2026-10-01，GitHub 远端安全页面已显示 Dependabot #107 完成处理且没有开放警报。
Docker 多架构发布和真实生产切换仍以 GitHub Actions 结果和实际部署验证为准。

### v0.2.3 发布收尾（2026-10-01）

**变更前**：仓库中保留一次性 Dependabot 处理与标签创建工作流，文档仍记录 #107
开放、v0.2.3 标签和发布镜像尚未验证。

**变更后**：删除一次性工作流；根据 GitHub 远端安全页面确认 #107 已完成处理且没有
开放警报。正式发布通过 `v0.2.3` 标签触发 Docker 多架构、GitHub Release 和 Electron
工作流，发布后继续核验镜像 manifest、Cosign、Release 产物和独立端口运行状态。

### v0.2.4 部署故障修复与全新部署边界（2026-10-01）

**变更前**：PostgreSQL 历史唯一约束可能以 `idx_subscription_pre_consume_records_request_id`、
`uni_subscription_pre_consume_records_request_id` 或 PostgreSQL 默认名称存在，GORM
启动迁移可能尝试删除不存在的约束并以 `SQLSTATE 42704` 失败。部署脚本会先启动应用，
已有 PostgreSQL named volume 与 `.env` 密码不一致时，应用可能反复重启。

**变更后**：主库 `AutoMigrate` 前只对 PostgreSQL 检查订阅预消费 `request_id` 的已知
历史唯一对象，在锁表事务中安全转换为独立唯一索引；未知、复合、部分、表达式、延迟或
未验证对象直接报错，不猜测删除。Compose PostgreSQL 健康检查和部署脚本都执行 TCP
密码认证，认证失败时应用不会启动，并提示恢复原密码、先同步数据库密码或执行全新
清理。全新清理只允许精确删除 NexusTok 容器、卷、网络和 `/opt/nexustok/data`、
`/opt/nexustok/logs`，不得使用无范围的 Docker prune，也不得影响 Komari 等其它资源。

本版本未新增数据库模型、字段、API 路由、DTO、Token 格式或前端路由契约。发布镜像为
`c1cadabob/nexustok:v0.2.4` 和 `c1cadabob/nexustok:latest`；用户需要在清理后自行
重新克隆仓库并执行 `bash scripts/deploy.sh`，本次不会自动部署。

### v0.2.5 Sub2API 登录服务条款兼容与发布基线（2026-10-02）

**变更前**：Sub2API 账号密码登录没有读取公开服务条款设置，二次验证阶段不会重新
读取条款版本；条款拒绝无法与凭据错误和安全验证错误稳定区分，后台同步也没有专用
状态。New API 是否读取或发送同名条款字段的边界没有在发布基线中明确。

**变更后**：仅 Sub2API 账号密码登录复用已规范化并通过安全校验的管理 Session，
读取 `GET /api/v1/settings/public`。只有 `login_agreement_enabled=true` 且
`login_agreement_revision` 非空时，才向初次登录和二次验证请求发送 `agreed_revision`；
设置接口不可用时回退旧协议。条款拒绝归类为 `login_agreement_required`，不触发主体
回退，后台同步保留最近成功资源快照。New API 不读取该接口、不发送条款字段；本版本
不发送 `not_in_cn_confirmed`，不新增数据库结构。发布镜像为
`c1cadabob/nexustok:v0.2.5` 和 `c1cadabob/nexustok:latest`。

### v0.2.6 Docker 自动更新端口预检（2026-10-02）

**变更前**：`v0.2.5` 的应用内 Docker 自动更新会复制当前容器的宿主端口映射，并在旧容器
仍运行时启动 staging 容器；Compose 默认部署中旧容器已经绑定 `3030`，因此 Docker 可能
返回 `port is already allocated`，更新任务停在重建容器阶段。

**变更后**：bridge/Compose 网络先使用不发布宿主端口、无 Compose 服务标签和网络别名的
preflight 容器完成健康检查；通过后删除 preflight，再创建保留原端口、网络别名、重启策略
和 Compose 标签的正式容器。正式容器只在旧容器停止并改名后启动，失败时恢复旧容器并保留
稳定 backup。`host` 和 `container:<id>` 网络走停旧后启动正式容器的降级路径。发布镜像为
`c1cadabob/nexustok:v0.2.6` 和 `c1cadabob/nexustok:latest`；本版本不新增数据库结构。

### v0.2.7 Sub2API 浏览器采集与页面桥接（2026-10-08）

**变更前**：Sub2API 浏览器采集主要依赖页面中的认证字段和管理 API 地址，页面公开
`custom_endpoints` Relay 配置未被统一发现和复核，管理地址与 Relay 地址可能无法明确
分离；目标站点使用动态 CSP nonce 时，页面桥接脚本在 nonce 尚未暴露时立即注入，可能
被浏览器拦截，导致 Capture Session 无法完成。

**变更后**：自动采集按 `auth_token`、`auth_user`、Refresh Token 和浏览器恢复信息的
顺序读取，并通过 `/api/v1/auth/me` 及兼容路径验证当前登录态；服务端只接受重新读取页面
后明确声明的 `custom_endpoints` Relay 地址。`tk.shour.bond` 的管理地址为
`https://tk.shour.bond`，Relay 地址为页面公开声明的
`https://api-image.shour.bond`。页面桥接等待目标页面提供 CSP nonce 后再执行，失败时
通过校验窗口来源、Origin 和 Capture ID 的消息反馈。Capture Session、加密
`PlatformSiteCredential`、一次性 claim、用户/渠道绑定和最近成功资源快照保护保持不变；
资源同步失败不会清空最近成功的 Key、模型和可路由能力，浏览器采集登录态不参与后台密码
同步 Cleanup。

本版本不恢复旧版完整 `upstreamaccount`、Preview 或 `ChannelAccount` 同步体系，不新增
数据库字段、迁移或明文凭据结构。发布验证只记录实际执行的本地命令和远端工作流结果，
不以标签推送前的静态检查代替 GitHub Release、Docker 多架构镜像、Cosign 或 Electron
产物的实际验收。

### Master/Slave

多节点必须共享主数据库。Master 负责迁移和系统任务调度，系统任务模型通过数据库租约、Claim 和状态更新避免多个 Master 重复执行。节点报告、授权策略同步和缓存同步仍有各自的刷新周期，不能把“请求可由多个节点处理”理解为所有内存状态自动一致。

## 6. 对外接口、配置和数据模型

- 管理和用户 API：`router/api-router.go`。
- Relay API：`router/relay-router.go`、`router/video-router.go`、`router/task-router.go`。
- 动态插件协议：`router/task-plugin-protocol-router.go`、`router/plugin-router.go`。
- Web 静态资源：`router/web-router.go` 和 `web/dist` 嵌入资源。
- 主数据模型：`model/*.go`，数据库类型和迁移入口在 `model/main.go`。
- 系统维护接口：`/api/system-update/latest`、`/api/system-update/apply`、
  `/api/system-update/rollback`、`/api/system-update/restart`；均由 `RootAuth()` 保护，
  使用现有 `SystemTask` 表，不新增数据库字段或迁移。
- 请求上下文和单次请求数据：`RelayInfo`、Gin Context keys、`model.Channel`、`model.Token`、`model.Task`、`model.SystemTask`。
- 运行配置：`common.InitEnv`、`setting/`、环境变量和管理端 Option。

## 7. 当前限制和实际偏差

- 路由注册不等于 Endpoint 完整实现；显式 `RelayNotImplemented` 的接口见[`implementation-deviations.md`](./implementation-deviations.md)。
- 统一请求入口中，内置 `/v1/responses` 创建入口不是普通静态 Relay 路由；Responses 插件协议由插件动态绑定，另有 `/v1/responses/compact` 和 Chat-to-Responses 内部转换，详见[`relay-routing-and-conversion.md`](./relay-routing-and-conversion.md)。
- 独立 Redis、内存缓存和多节点部署会影响限流、缓存传播和插件生成观察窗口，不能只根据单节点测试判断集群语义。
- `relaykit/` 必须独立构建，根模块的配置、数据库和计费不能下沉到该模块。

### 7.1 系统维护更新边界（2026-09-30）

**变更前**：维护页面在浏览器中直接请求 GitHub Release API，只能显示版本说明；
后端没有统一的更新、回滚、重启和任务进度入口，旧版裸机回滚会把唯一 `.backup`
重命名为当前文件，成功后回滚槽位消失。

**变更后**：Root 管理员通过后端检查默认仓库 `c1cadaBob/NexusTok`，也可用
`SYSTEM_UPDATE_GITHUB_REPO` 覆盖。Release 检查缓存约 20 分钟，强制检查可绕过缓存；
版本比较支持 `v` 前缀、预发布和构建元数据。资产只接受 HTTPS 的 GitHub 官方域名，
下载和 checksum 有大小上限，二进制替换前必须完成 SHA256 校验。

裸机更新先在当前可执行文件同目录的临时目录下载和验证，再以临时交换文件提交当前
文件与稳定 `<executable>.backup`；回滚交换两者而不是消耗备份，因此可以连续切换。
源码/开发构建和 Windows 正在运行的二进制不强行自动替换，页面返回手动更新提示。

Docker 部署通过 Docker Engine socket 和独立 helper 重建候选容器，保存环境、挂载、
端口、网络、重启策略、入口参数和 Compose 标签。2026-10-02 起，普通 bridge/Compose
网络先创建不发布宿主端口、无 Compose 服务标签和网络别名的 preflight 容器完成健康
检查；通过后删除 preflight，再创建保留原始端口和 Compose 元数据的正式 staging
容器，停旧容器并改名后才启动正式容器，避免旧容器仍占用 `3030` 时触发 Docker
`port is already allocated`。`host` 和 `container:<id>` 网络无法安全并行预检，按停旧后
启动正式容器的降级路径执行。helper 在主容器停止前接管系统任务租约，启动失败时把租约
转回原 runner。管理响应、任务错误、日志和手动命令不包含 `SESSION_SECRET`、`SQL_DSN`、
`REDIS_CONN_STRING`、Cookie、Token 或完整 Key。

## 8. 维护时需要同步的关联模块

修改启动顺序、节点职责、路由安装或中间件顺序时，必须同步 `main.go`、`router/main.go`、对应路由和本文档。修改数据库/Redis/日志边界时同步 `model/main.go`、`common/redis.go`、缓存模型和[`data-cache-and-background-jobs.md`](./data-cache-and-background-jobs.md)。修改用户请求链路时同步鉴权、路由、计费和能力矩阵文档。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立启动、分层、请求链路、部署边界和限制说明 | `main.go`、`router/`、`middleware/`、`model/`、`service/`、`relay/` | `main.go`、`model/main.go`、`router/` 静态核对 |
| 2026-09-30 | 系统维护更新与回滚 | 前端直连 GitHub 且没有后端运维任务；旧版回滚会消耗唯一备份 | 增加 RootAuth 维护接口、SystemTask 进度、GitHub 缓存/checksum、裸机稳定备份交换、Docker helper/健康检查和重启探活 | `router/api-router.go`、`controller/system_update.go`、`service/system_update*.go`、`main.go`、维护页 | `go test ./service ./model ./controller ./router`、前端定向测试；未进行生产容器切换 |
| 2026-10-02 | Docker 自动更新端口预检 | 更新流程在旧容器仍发布 `3030` 时直接启动同端口 staging 容器，Docker 会返回 `port is already allocated` | bridge/Compose 网络先用无宿主端口、无 Compose 标签/别名的 preflight 容器探活，再停旧并启动保留原端口的正式容器；`host`/`container:<id>` 网络走受控降级 | `service/system_update_docker.go`、系统任务 Docker helper、维护页错误展示 | `go test ./service -run 'Docker(Update\|Updated\|Readiness\|Helper\|Pull\|Staging)' -count=1` |
| 2026-09-30 | v0.2.2 生产默认部署 | Compose 使用浮动依赖、明文默认密码和相对目录，服务依赖未按健康状态编排；单容器示例未明确不包含外部数据库/缓存 | Compose 固定 PostgreSQL 15 + Redis 7，密码由 `.env`/部署脚本生成，应用端口为 `3030`，数据/日志使用 `/opt/nexustok` 持久化目录，单容器仅保留 SQLite/无 Redis 兼容模式 | `docker-compose.yml`、`scripts/deploy.sh`、`Dockerfile`、`VERSION`、Docker 发布工作流和中文部署文档 | `docker compose config`、`bash -n scripts/deploy.sh`、隔离三服务栈和真实 SQLite 3.50.4/MySQL 8.2.0/PostgreSQL 15.19 矩阵已通过；生产 Dockerfile 完整构建因 `proxy.golang.org` 超时未完成，远端发布以标签工作流为准 |
| 2026-10-01 | v0.2.3 安全依赖与部署发布 | README 多语言存在旧端口、旧镜像和宣传栏目；发布工作流 Secret 名称不匹配；部署文档未完整覆盖单机、多机、备份和故障边界 | 统一六种 README、宝塔文档和架构事实为 `3030`、Compose、PostgreSQL 15 + Redis 7；补充多机共享外部服务、主从职责、代理和回滚；工作流使用 `DOCKER_USERNAME`/`DOCKER_PASSWORD`；前端/Electron 依赖按本地审计结果最小升级，数据库代码和 Schema 未修改 | `README*.md`、`docs/installation/BT.md`、`.github/workflows/docker-*.yml`、web/Electron 依赖 | Bun/npm 审计、govulncheck、前端全量回归、Go vet/build/test、relaykit 构建、Compose 配置和隔离三服务健康检查已通过；推送反馈仍有 1 条中等级 Dependabot 警报，认证 API、v0.2.3 镜像、多架构 manifest、Cosign 和远端 Release 待验证 |
| 2026-10-01 | v0.2.4 部署故障修复与发布准备 | PostgreSQL 历史唯一约束可能导致启动迁移以 `SQLSTATE 42704` 失败；部署脚本可能先启动应用再暴露 `.env` 密码不一致；全新部署清理边界未集中记录 | 已知历史唯一对象在 `AutoMigrate` 前转换为独立唯一索引；Compose 健康检查和部署脚本先执行真实 TCP 密码认证，失败时不启动应用；补充 named volume、Redis 临时状态、数据/日志路径、备份、精确清理和 Komari 保护边界；发布目标为 `v0.2.4` 与 `latest` | `model/subscription_pre_consume_migration.go`、`docker-compose.yml`、`scripts/deploy.sh`、部署文档和发布说明 | SQLite 定向迁移测试、Compose 配置和脚本语法待本次完成；真实 PostgreSQL/MySQL 矩阵、发布工作流和远端全量清理结果按最终报告记录，未完成前不宣称三数据库兼容验证完成 |
