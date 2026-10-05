# 数据库、缓存与后台任务

> 文档状态：代码事实基线
> 事实基线日期：2026-10-05
> 主要代码来源：`model/main.go`、`common/database.go`、`common/redis.go`、`model/channel_cache.go`、`model/sync.go`、`service/system_task.go`、`service/task_polling.go`、`main.go`
> 关联详细文档：[`system-overview.md`](./system-overview.md)、[`authentication-and-authorization.md`](./authentication-and-authorization.md)、[`tasks-and-plugins.md`](./tasks-and-plugins.md)、[`../rate-limiting.md`](../rate-limiting.md)

## 1. 功能目标和边界

NexusTok 将主业务数据库、日志数据库、Redis 和进程内缓存分开使用。数据库保存用户、渠道、Token、任务、插件、选项、订阅和日志等权威状态；缓存用于降低查询成本和提供限流/路由索引；后台任务负责周期性维护、同步和异步任务推进。

缓存命中不能被视为数据库事实。文档中的“回退”表示代码存在回源路径，不表示所有节点都能自动获得相同的内存状态。

### 7.5 2026-10-02 平台站点 Capture 会话与跨节点占用

**变更前**：平台站点自动配置的短期凭据缓存只有记录级删除语义；同一
`capture_id` 被两个并发渠道保存请求解析时，Redis 节点之间没有统一的占用者校验。

**变更后**：

- Capture 记录继续位于 `platform-site-capture` 命名空间，TTL 为 10 分钟，凭据使用现有
  `PlatformSiteCredential` 加密结构保存，不新增数据库列或持久化 claim 字段。
- `HybridCache` 增加 `TryClaim`、`ClaimMatches`、`ReleaseClaim` 和 `DeleteClaim`：
  Redis 使用命名空间下的 `<capture_id>:claim`、`SET NX`、TTL 和比较删除；无 Redis
  时由同一进程的互斥锁和过期时间提供互斥。过期记录清理会同时删除记录和 claim。
- `ResolvePlatformSiteCapture` 解密并校验成功后才申请内部 claim token；只有持有该 token
  的请求可以消费记录。新建或编辑渠道在后续校验、数据库保存和入队失败时释放 claim，
  成功保存后删除记录并释放 claim。claim token 使用 `json:"-"`，不发送给浏览器。
- 后台资源同步不依赖 Capture claim。它按渠道同步锁执行，密码会话清理失败只告警，
  不覆盖已成功资源快照或将凭据改为失效；资源失败、部分分页失败仍按最近成功快照
  回退规则处理。

### 7.6 2026-10-03 平台认证流程与临时密码材料

**变更前**：认证流程完成后的表单字段和后台密码同步的临时材料边界没有在缓存与后台
任务文档中分开描述，容易把流程输入、长期凭据和本轮会话材料混为同一份缓存数据。

**变更后**：

- 有 `platform_site_auth_flow_id` 时，前端只提交流程元数据；后端以流程解析结果生成
  最终账号和加密凭据。旧客户端残留用户名不改变流程身份，密码、Token、Cookie、
  Session ID、Admin Key 等真实材料不能与流程混用；
- 后台密码同步仍按渠道同步锁执行，每轮直接用账号密码建立临时会话；临时
  Access/Refresh Token、Cookie 和 Session ID 只保留在本轮内存会话，资源读取和快照
  写入路径结束后清理，不写入长期凭据或 Capture 缓存；
- NewAPI/Sub2API 的 Cleanup 告警不删除缓存中的最近成功资源快照，也不把账号直接
  改为凭据失效。Capture 登录态属于用户既有浏览器会话，仍由 Capture 缓存和一次性
  claim 管理，不进入后台密码会话清理；
- 本次没有新增数据库字段、缓存键或迁移；现有 Redis/内存回退语义不变。

### 7.7 2026-10-03 Sub2API Relay 地址与资源快照边界

**变更前**：后台同步只按原始管理地址解析页面配置，页面重定向到独立 Relay
域名时，相对 `api_base_url` 可能解析错误；严格站点的登录请求还可能因额外字段返回
`400 INVALID_REQUEST`，失败阶段与最近成功快照的关系不够明确。

**变更后**：

- 页面请求保留最终 URL，相对 `api_base_url` 以最终 HTML 页面地址解析。页面声明的
  Relay 只有在与最终页面来源或原始管理地址协议、主机和有效端口一致，或与原始管理
  地址满足严格直接 `api.` 父子域关系时才进入会话；未经页面明确声明的第三方地址
  继续拒绝，并保留 SSRF、端口和重定向校验；
- 管理地址用于登录、当前用户、分组、用量和 Key；若最终页面来源与明确 Relay 同源，
  本轮管理请求根地址使用该可信最终来源，但持久化管理地址不变。Relay 地址只用于
  单 Key `/v1/models` 或最终转发。Relay 模型探测成功后才更新 Key 能力、资源表和
  父渠道模型并集；Relay 请求清空 Cookie Jar，只发送当前 Key 的认证 Header；
- Sub2API 普通邮箱登录首请求只发送 `email/password`，首路由仅在 404/405 时回退
  `/auth/login`；400 `INVALID_REQUEST` 不触发路由或主体重试。公开设置只在上游明确
  要求条款时读取；
- 资源读取、分页、模型探测或同步写库失败时，清理本轮密码会话但不删除最近成功
  快照、不把资源错误改写为凭据失效，也不执行错误的密钥缺失判定。

### 7.8 2026-10-04 Capture Helper 等待与浏览器会话生命周期

**变更前**：Helper 在上游 DOM/SPA 登录态形成前立即执行，临时未登录可能调用完成接口
提交错误，短期 Capture Session 从 `pending` 变为 `failed`；页面脚本来源、Helper 重试
和 Sub2API IndexedDB 客户端 ID 的生命周期没有在缓存文档中分开记录。

**变更后**：Capture Session 仍使用现有短期加密缓存和一次性 claim，不增加数据库字段。
Helper 在 `DOMContentLoaded` 后启动并持续等待；候选为空、当前用户接口暂时失败、
Cookie 尚未形成或 Cloudflare/Turnstile 尚未结束时只显示脱敏状态并自动/手动重试，
不会改写缓存状态。会话过期、来源/版本不匹配和完成回传硬错误才进入失败路径。Capture
成功后凭据继续使用现有加密结构，渠道保存成功消费记录，校验或入队失败释放 claim。

NewAPI 的浏览器态候选允许从 `uid`、页面用户状态和 JWT 得到数字用户 ID，并使用
`New-Api-User + Cookie` 验证当前用户；Helper `1.7.0` 会合并当前页面可见 Cookie
与同站 `GM_cookie` 结果，回传只保留扁平身份字段。Sub2API 的 Session Restore 允许读取
`sub2api-auth-coordination/values/sub2api_auth_client_id` 并发送
`X-Sub2API-Auth-Client`。浏览器 Capture 的 Cookie、Session 和 Token 属于用户已有会话，
不进入后台账号密码同步清理，也不写入长期诊断。

无扩展页面桥接的脚本不把 Capture Secret 放入脚本地址。若上游登录重定向导致 handoff
查询参数丢失，桥接脚本会通过 `window.opener.postMessage` 向活动管理页请求一次性
handoff；管理页仅在窗口、Origin 和 Capture ID 匹配时回复。跨源 `javascript:` 导航
被浏览器拒绝时，前端复制不含敏感值的短启动片段；片段在上游页面上下文请求一次性桥接
脚本后再执行。该复制降级不会改变 Capture 缓存的 TTL、claim 或一次性消费语义。

### 7.9 2026-10-05 旧版浏览器采集路径与缓存边界

**变更前**：Capture Session 的缓存和 claim 生命周期已经独立于后台同步，但
Dashboard Refresh、Sub2API Refresh Token 与 Session Restore 仍有固定路径假设，反向
代理子路径未在所有浏览器认证分支共享。

**变更后**：所有浏览器认证分支继续读写同一个短期加密 Capture Session，不新增缓存键、
数据库字段或持久化明文凭据。NewAPI Refresh、Sub2API Refresh 和 Session Restore 统一
从 handoff、`api_base_url`、页面配置、当前页面和存储状态推导 API 前缀，并通过同源
JavaScript/性能资源发现兼容路径。候选调用只扩大路径兼容性，不改变一次性 claim、10
分钟 TTL、用户/渠道绑定、Origin/Helper 版本校验或桥接来源校验。

未登录、SPA 状态尚未形成、WAF/Turnstile 或临时上游错误仍保持 `pending`，不会写入
永久失败，也不会删除最近成功资源快照。成功保存后仍消费 Capture 记录；后续校验、数据库
保存或入队失败仍释放 claim。浏览器登录态不进入后台密码同步 Cleanup。

### 7.10 2026-10-05 Sub2API 页面声明 Relay 与资源快照

**变更前**：缓存和后台任务只描述了管理地址页面发现，没有明确
`custom_endpoints` Relay 声明的匿名复核，也没有把管理面成功、Relay 模型探测失败和
最近成功快照的关系写清楚。

**变更后**：Capture 完成时服务端不把客户端 `relay_base_url` 当作事实，而是匿名重读
`record.BaseURL` 页面并解析 `custom_endpoints`/`customEndpoints`；页面未声明、协议/端口
不匹配或页面不可读时拒绝外部 Relay。持久化仍只使用已有加密凭据、短期 Capture 缓存和
一次性 claim，不新增数据库字段或明文凭据缓存。

后台同步将管理地址用于 `/api/v1/...` 身份、分组、用量和 Key 请求，将页面声明 Relay
用于 `/v1/models`；Relay 请求不复用管理 Cookie、Bearer、Origin、Referer、
`X-Requested-With` 或 `X-Auth-Session`。Key/模型部分失败只写入资源级
`partial/stale`、`using_snapshot` 和脱敏原因，保留最近成功快照。指定站点验收读取到
7 条 Key 且 7 条 `ModelsSynced=true`，渠道仍启用并保留可用模型；本轮未在日志、
诊断或额外明文缓存中保存敏感值，认证凭据和完整 Key 仍遵循既有整体加密保存边界。

## 2. 数据库选择和迁移

### 主数据库

`model.chooseDB("SQL_DSN", false)`支持：

- SQLite：未设置 `SQL_DSN` 或使用本地配置时使用 `common.SQLitePath`；
- MySQL：使用 `SQL_DSN`，补充 `parseTime=true`；
- PostgreSQL：使用 `postgres://`/`postgresql://`，关闭不兼容连接代理的隐式预处理配置；
- ClickHouse：主数据库明确拒绝。

### 日志数据库

`model.InitLogDB`通过 `LOG_SQL_DSN` 选择日志存储，ClickHouse 只允许用于日志库。主库和日志库分别记录数据库类型和保留字引用，业务 SQL 必须通过数据库分支兼容 SQLite、MySQL 和 PostgreSQL。

### GORM 和迁移

GORM 模型位于 `model/`。Master 启动时执行 AutoMigrate 和兼容迁移；SQLite 使用 `ADD COLUMN` 等可用路径，PostgreSQL 使用双引号保留字，MySQL/SQLite 使用反引号。涉及模型、索引、约束、Scanner/Valuer、日志库或锁的变更必须验证三种主数据库和适用的日志库。

### 生产默认数据库拓扑（2026-09-30）

**变更前**：生产 Compose 使用未固定的数据库/缓存镜像标签和仓库内默认密码，
应用、PostgreSQL、Redis 的启动顺序没有以健康状态为条件，数据目录也不是固定的
生产持久化路径。

**变更后**：生产 Compose 默认使用 `postgres:15-alpine` 作为主库，固定通过
`SQL_DSN=postgresql://...@postgres:5432/nexustok` 连接；Master 启动时仍由现有
`model.InitDB` 执行 GORM 迁移。`scripts/deploy.sh` 首次生成 `.env` 中的
`POSTGRES_PASSWORD`，后续不覆盖，数据库和 Redis 不映射宿主机端口。Compose 只负责
全新 PostgreSQL 生产实例的初始化，不负责把现有 SQLite 文件自动转换为 PostgreSQL；
已有数据切换必须先备份并单独执行数据迁移。

## 3. Redis 与内存缓存

Redis 由 `common.InitRedisClient` 初始化，未配置 `REDIS_CONN_STRING` 时关闭。`SYNC_FREQUENCY` 控制多种缓存/同步周期，默认和非法值回退为 60 秒。Redis 可用于：

- 用户鉴权和 Session 快照、版本栅栏和撤销 tombstone；
- Token、订阅、选项或其它业务缓存；
- 渠道/模型和 Routing Key 辅助缓存；
- 全局/用户/关键操作限流；
- 通知和其它短期计数。

启用 Redis 时 `main.go` 同时启用内存缓存兼容路径；未启用 Redis 时部分路径只使用内存或数据库。限流、Session 和渠道缓存的回退语义不同，维护时必须分别查看对应模块。

### 生产默认缓存拓扑（2026-09-30）

生产 Compose 默认使用固定版本的 `redis:7-alpine`，以 `REDIS_PASSWORD` 配置
`requirepass`，并向应用注入 `REDIS_CONN_STRING=redis://:...@redis:6379/0`。
Redis 服务通过健康检查后才允许 NexusTok 启动。Redis 仍然是缓存和共享控制面，不能替代
主数据库；Redis 未配置时的关闭和内存/数据库回退逻辑保持不变。单容器 `docker run`
命令不创建 Redis，因此只表示无 Redis 兼容模式。

### v0.2.3 部署文档与依赖边界（2026-10-01）

**变更前**：多语言 README 的生产端口、镜像和单容器边界不一致，单机与多机的
PostgreSQL/Redis、Session、限流、备份和滚动升级说明不完整；发布工作流 Secret 名称
仍使用旧接口。

**变更后**：部署文档统一说明 Compose 的 PostgreSQL 15 + Redis 7 默认拓扑、内部网络
访问、`.env` 随机密码、`3030` 健康检查、named volume/逻辑备份、SQLite 不自动迁移、
多机共享外部数据库与 Redis、主节点迁移/系统任务和从节点 `NODE_TYPE=slave`。共享
Redis、独立 Redis 和关闭 Redis 的 Session/限流差异继续按现有代码事实描述。v0.2.3
依赖修复只涉及 web/Electron 依赖；未修改 GORM、数据库驱动、数据库连接、模型、迁移、
索引、约束或日志数据库边界，因此不新增数据库兼容性结论。

### v0.2.4 部署故障修复与数据清理边界（2026-10-01）

**变更前**：已有 PostgreSQL named volume 使用旧密码时，修改 `.env` 只会改变新容器
注入的连接密码，不会改变数据库角色密码；旧版部署脚本可能先启动 NexusTok 并进入
重启循环。Redis 使用容器匿名数据卷时，重建会丢失缓存和临时 Session/限流状态，容易
被误认为数据库数据仍然存在。

**变更后**：`docker-compose.yml` 的 PostgreSQL 健康检查使用当前环境变量执行 TCP
认证和 `SELECT 1`；`scripts/deploy.sh` 先启动 PostgreSQL/Redis，执行 Compose 网络
认证，认证失败时停止且不启动应用，再等待 PostgreSQL 健康后启动 NexusTok。`.env`、
`/opt/nexustok/data`、`/opt/nexustok/logs`、PostgreSQL named volume 与 Redis 缓存的
备份和生命周期边界在部署文档中明确。全新部署只能删除已核对的 NexusTok 容器、卷、
网络和目录，不得使用 `docker system prune -a`、`docker volume prune` 或误删其它监控
资源的清理方式。

本次新增的 PostgreSQL 兼容迁移只转换订阅预消费 `request_id` 的已知历史唯一对象，
不新增模型、字段或业务协议。真实 SQLite、MySQL 和 PostgreSQL 实例的版本、命令和
结果必须单独记录；未完成矩阵前不宣称三数据库兼容验证完成。

## 4. 主要缓存生命周期

| 状态 | 权威来源 | 缓存/索引 | 失效或刷新方式 |
| --- | --- | --- | --- |
| 渠道、模型和分组 | 主数据库 `Channel`、`Ability`、Routing Key | `model/channel_cache.go`、模型能力索引 | 启动预热、`SyncChannelCache`、渠道变更和模型更新 |
| 定价 | 配置、数据库价格、内置表达式 | `model.GetPricing`、价格快照 | 启动预热、价格/配置变更 |
| 用户鉴权 | `User`、Token、Session 数据库 | 用户/Token/Session Redis 快照 | auth version、Session 撤销、TTL 和回源 |
| Session | `user_sessions` | Redis Hash/快照和 tombstone | 撤销、版本变更、TTL、清理任务 |
| 限流状态 | Redis 或进程内计数器 | 固定窗口、令牌桶、并发计数 | 时间窗口、请求结束、进程重启 |
| 任务状态 | `Task`/`SystemTask` | 任务私有数据、插件生成和数据库租约 | CAS、终态写入、锁续租/过期 |
| 授权策略 | Casbin/数据库策略 | `service/authz` 内存模型 | 周期性策略同步和进程重载 |

## 5. 系统任务 Runner

`service.StartSystemTaskRunner`只在 Master 节点启动。Runner 每约 15 秒或收到唤醒信号执行：

1. 清理过期系统任务锁；
2. 按调度间隔创建启用的周期任务；
3. 为每种任务 Claim 一个待执行记录；
4. 通过数据库租约和锁续租执行 Handler；
5. 写入进度、成功/失败结果和终态。

系统任务的 Type、Payload、State、Result 和 active key 由 `model.SystemTask` 持久化。单一类型已有 active 任务时通常不会重复创建；上游站点同步等任务可使用 `type:channel_id` 作为 active key。系统更新和回滚分别使用
`system_update`、`system_rollback` 类型，但共享 `system_binary_update` active key，
因此两者不能并发执行，且并发请求会复用同一条活动任务。

系统维护任务的更新、回滚和 Docker helper 都通过同一条数据库租约推进。主容器停止前，
更新 runner 创建唯一名称的 helper 容器并调用 `TransferSystemTaskLock` 转移租约；helper
启动失败时将租约转回原 runner，helper 结束时写入 `succeeded` 或 `failed` 终态。租约
丢失时旧 runner 不能继续覆盖进度或结果。

## 6. 当前已接入的后台任务

启动代码和 Controller 注册的任务包括：

- 渠道检测/定期渠道测试；
- 上游渠道模型更新和平台站点同步；
- Midjourney、Suno、视频等异步任务轮询；
- 日志清理；
- 订阅额度日/周/月/自定义周期重置；
- Session、鉴权 Artifact 和过期状态清理；
- Codex 凭据每 10 分钟检查，在临近过期时刷新；
- 渠道缓存、选项、插件、授权策略和系统节点状态同步；
- 可选的批量更新、性能监控和 pprof。
- 系统维护版本检查、二进制更新、可重复回滚、Docker 更新/回滚和 helper 接管。

每个任务是否启用、运行间隔、Master 限制和失败重试由各自 Handler 的 `Enabled`、配置和系统任务实现决定，不能只根据任务名称判断正在运行。

## 7. 日志存储与多节点一致性

业务日志可以单独落到日志数据库，管理员查询与普通用户视图还会依据字段清理管理员信息。计费饱和标记、密钥诊断和插件诊断等敏感信息应遵守可见性边界。

主数据库共享保证了用户、渠道、任务和系统任务租约的一致性；Redis 共享保证了依赖 Redis 的缓存/限流共享。没有共享 Redis 时，内存缓存、限流和部分 Session 观察会出现节点局部语义。

## 8. 当前限制和实际偏差

- 数据库迁移只有 Master 负责，Slave 启动成功不等于已完成迁移。
- Redis 缺失时不止一种回退：有的功能回数据库，有的只在进程内工作，有的功能会被关闭；必须按模块核查。
- 系统任务 Runner 的数据库锁和 Handler 的业务幂等是两层保护，锁丢失或重复唤醒不能被当作绝对的单次执行保证。
- 日志库支持 ClickHouse 不代表主业务库支持 ClickHouse。
- GitHub Release 检查结果仅在进程内缓存约 20 分钟，`force=true` 才会强制刷新；缓存不是
  发布事实，网络失败时只在存在最近检查结果时返回带警告的缓存。
- 系统维护不新增数据库字段或迁移。任务仍复用 `SystemTask` 的 Payload、State、Result、
  ActiveKey 和租约字段；本次涉及的 ActiveKey 幂等、租约转移和终态写入需要按现有
  SQLite/MySQL/PostgreSQL 兼容规则验证。

### 8.1 平台站点资源快照（2026-09-26）

平台站点同步按身份、分组、端点、密钥、模型和用量资源独立记录尝试时间、成功时间、
状态、来源接口、数量、脱敏失败原因、部分成功和安全验证要求。同步失败时不覆盖最近
成功值；只有完整分页和资源校验成功后才允许将未返回密钥标记为缺失。安全验证或
Admin Key step-up 拒绝读取密钥时保留旧密钥和模型快照，平台路由继续使用最近成功
快照。资源查询接口读取规范化资源表和现有 `UpstreamKey` 路由主数据，不把短期认证
流程或任何敏感凭据写入缓存响应。

旧版预览流程另有短期缓存边界：`upstream-account-preview` 默认 TTL 为 10 分钟，
预览创建的完整 Key 只在后端缓存和创建阶段使用，前端预览响应只返回脱敏 Key；前端
最终只提交 `preview_id`，一次性消费后不能重复创建。当前实现不复用该预览缓存，而是
将平台账号凭据和完整子 Key 分别加密保存，并用凭据指纹、Key HMAC 指纹和外部 ID/地址
进行刷新匹配。手动同步、排队同步和系统任务后台同步都经过同一站点锁和
`SyncUpstreamSite`，资源表记录本次尝试与最近成功时间。

平台站点资源表、`UpstreamKey`、`UpstreamKeyAbility` 和渠道模型索引构成当前路由快照；
`PlatformSiteResourceSync.UsingSnapshot=true` 或父站点 `failed/running` 但存在
`LastSyncAt` 时，表示当前读取失败而仍使用最近成功值。权限不足、step-up、安全验证、
WAF、分页中途失败或单 Key 模型探测失败不能清空旧 Key、额度、过期时间、模型和能力
快照；只有完整分页、权限有效和必要 Key 详情完成后，才允许把未返回 Key 标记为缺失。

2026-09-30 资源同步迁移后，New API Token、Sub2API 普通 Key 和 Sub2API Admin Key
分页均使用每页 100 条、最多 1000 页；其它管理资源继续使用自己的分页限制。New API
批量 Key 只补偿缺失 ID，Sub2API 列表已经返回完整 Key 时跳过详情，缺失或掩码时才
读取详情。分页或详情失败不触发 missing 判定，单 Key 模型失败只影响当前轮次该 Key
能力并保留最近成功值。该改动不新增数据库模型、字段或迁移，因此本次未新增三数据库
实例迁移矩阵；既有数据库兼容记录不能当作本次新运行结果。

### 8.2 系统维护更新缓存与租约（2026-09-30）

变更前维护页直接读取 GitHub Release，浏览器承担网络、CORS 和限流风险；系统任务没有
更新/回滚专用类型，旧版回滚会消耗唯一的 `.backup` 文件。

变更后版本检查由后端访问 `api.github.com`，默认仓库为 `c1cadaBob/NexusTok`，
支持 `SYSTEM_UPDATE_GITHUB_REPO` 覆盖，缓存约 20 分钟并返回语义版本比较、平台资产、
checksum 和脱敏警告。更新文件先写入当前可执行文件同目录的临时目录，校验大小、权限和
SHA256 后再进行事务式交换；`.backup` 保持为稳定回滚槽位，回滚通过交换当前文件和
`.backup`，成功后仍可继续回滚。

Docker 模式使用 Engine socket 和独立 helper，不启动第二个 HTTP/Redis runner。候选容器
使用唯一 preflight/staging/failed 名称，旧 backup 在候选容器通过健康检查前不覆盖。
2026-10-02 起，bridge/Compose 网络先启动不发布宿主端口、无 Compose 服务标签和网络别名的
preflight 容器；通过后删除 preflight，再在旧容器停止并改名后启动保留原端口和 Compose
元数据的正式 staging 容器，避免自更新时与旧容器争用 `3030`。存在 Healthcheck 时必须为
`healthy`，没有 Healthcheck 时只确认容器持续运行并标记降级；`host` 和 `container:<id>`
网络无法安全并行预检，走停旧后启动正式容器的降级路径。Docker 更新和回滚失败会删除候选
容器、恢复原容器并再次确认其运行状态。helper、runner 和任务终态都只保存脱敏错误，不保存
敏感环境变量值。source/development build、Windows 正在运行的二进制和未挂载 Docker socket
的部署继续显示手动更新提示。

本次系统任务兼容性验证（2026-09-30）执行：
`TEST_MYSQL_DSN='<临时测试 DSN>' TEST_POSTGRES_DSN='<临时测试 DSN>' go test ./model -run '^TestSystemTaskDatabaseCompatibility$' -count=1 -v`。
SQLite、MySQL 8.2.0 和 PostgreSQL 15.19 均通过 AutoMigrate 二次执行、ActiveKey 并发租约、
租约转移、状态更新、终态写入和清理验证。MySQL 5.7.8、PostgreSQL 9.6 及独立日志库本次
未单独启动，不能将本次结果解释为这些最低版本或日志数据库的实测。

## 9. 维护时需要同步的关联模块

修改数据库 DSN、GORM 模型、迁移、锁、缓存键/TTL、Redis 回退、日志字段、后台任务 Type/interval/lease、任务调度或多节点职责时，必须同步本文档、系统总览、任务/插件文档、鉴权/限流专项文档和偏差登记。数据库变更还要按根目录规则完成三数据库验证并记录版本与结果。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立数据库选择、缓存生命周期、系统任务 Runner 和后台任务入口说明 | `model/`、`common/`、`service/system_task.go`、`main.go` | 数据库初始化、Redis、Runner 和启动注册代码静态核对 |
| 2026-09-26 | 平台资源同步补充 | 平台站点同步主要以单个总快照和站点状态表示 | 增加资源类型独立状态、最近成功快照保留、完整分页缺失判定和安全验证失败回退基线 | 平台站点缓存、同步任务、资源查询 | `service/upstream_site.go` 当前快照逻辑与新增资源模型设计核对 |
| 2026-09-27 | NewAPI 类平台资源失败持久化 | 认证成功后的 Token 分页失败可能阻断身份/余额保存，Cookie 轮换和资源失败状态无法稳定写入；失败轮次与最近成功时间边界不清晰 | 先保存认证轮换凭据，再按身份、用量、密钥和模型写入可用部分快照；分页/权限/安全验证失败保留旧密钥和能力，不执行缺失判定，不更新 `last_sync_at`，资源表保留最近成功时间和本次失败状态 | 平台站点数据库快照、后台同步、渠道缓存和路由模型索引 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`model/platform_site_resources.go`、SQLite 回归测试 |
| 2026-09-27 | 平台站点代理出站与快照边界 | 平台站点同步没有使用渠道代理，代理导致的网络失败无法与资源快照状态分开核对 | 同步、认证和 2FA 使用渠道 Transport；代理配置错误在同步阶段单独记录，认证成功后的资源失败仍按资源类型落库并保留最近成功快照；不修改资源表结构 | 渠道 4 代理同步、渠道 5 认证诊断、平台站点资源缓存和后台任务 | `service/upstream_site.go`、`service/platform_site_auth_flow.go`、`service/upstream_site_test.go` |
| 2026-09-29 | 补充旧版预览缓存与当前平台资源快照关系 | 文档只描述当前资源表和失败回退，未把旧版 `upstream-account-preview`、完整 Key 临时边界、加密存储、HMAC 指纹与后台同步方式放在同一条缓存链路中 | 明确旧版预览缓存 10 分钟 TTL 和一次性消费；当前凭据/完整 Key 加密保存并以指纹和外部 ID 匹配；手动、排队、后台同步共用站点锁；资源失败保留最近成功 Key、额度、过期时间、模型和能力快照 | 平台站点缓存、资源查询、路由候选和系统任务 | `service/upstream_site.go`、`model/upstream_channel.go`、`model/platform_site_resources.go`、旧版 `service/upstreamaccount/`、[`平台站点资源获取比较`](../platform-site-resource-acquisition-comparison.md) |
| 2026-09-30 | 旧版资源分页与脱敏 fixture 验收边界 | New API/Sub2API Key 资源最多 100 页，完整 Key 详情和分页失败边界未完全记录；脱敏资源失败和地址边界未纳入缓存快照说明 | 三类 Key 资源恢复独立 1000 页上限；完整列表优先、缺失详情补齐、资源失败保留最近成功快照；使用脱敏 fixture 验证资源状态和路由边界；不写入凭据或临时捕获文件 | 平台资源缓存、后台同步、路由快照和安全交付 | `service/upstream_site_adapters.go`、`service/upstream_site_test.go`、脱敏 HTTP fixture 和 SQLite；无数据库结构变更 |
| 2026-09-30 | 系统维护任务与缓存边界 | 维护页直连 GitHub，未有更新/回滚任务，旧版回滚会消耗 `.backup`；Docker helper 和租约接管未登记 | 后端缓存 Release 并经 Root 任务执行更新、回滚和重启；裸机稳定 `.backup` 可重复交换，Docker 使用 socket/helper、唯一候选容器和健康检查；不修改 SystemTask 表字段或新增迁移 | GitHub 缓存、SystemTask、任务租约、裸机文件交换、Docker 容器生命周期 | `service/system_update.go`、`service/system_update_docker.go`、`model/system_task.go`、`service/system_update_test.go`；未执行真实生产容器切换 |
| 2026-10-02 | Docker 自动更新端口预检 | Docker 更新在旧容器仍占用宿主端口时直接启动同端口 staging 容器，可能在任务 55% 处失败并保持旧版本 | bridge/Compose 网络增加无宿主端口 preflight 探活，正式容器只在旧容器停止并改名后启动；端口冲突错误脱敏并提示检查其它宿主进程或容器 | Docker Engine helper、系统任务终态、维护页失败原因 | `go test ./service -run 'Docker(Update\|Updated\|Readiness\|Helper\|Pull\|Staging)' -count=1`；不新增数据库表、字段或迁移 |
| 2026-10-03 | 平台认证流程与临时会话材料 | Auth Flow 表单残留字段、密码同步临时令牌和最近成功资源快照的生命周期边界未集中记录 | 流程保存只消费服务端流程身份；密码同步临时材料只在本轮会话内使用并清理；认证/清理失败不覆盖最近成功快照；Capture 登录态继续由独立 claim 管理 | 平台站点 Auth Flow、后台同步、Redis/内存缓存和资源快照 | `controller/upstream_channel.go`、`service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/newapi_password_encryption.go`、相关定向测试；无数据库结构变更 |
| 2026-10-05 | 旧版浏览器采集路径迁移 | Capture 缓存文档未记录 Dashboard Refresh、Sub2API Refresh/Session Restore 对同源脚本发现和反向代理前缀的共享边界 | 刷新与恢复分支复用 `expandedAPIPaths`、同源 JS/性能资源发现和 API 后缀剥离；保持短期加密缓存、一次性 claim、pending 重试、最近成功快照和后台 Cleanup 隔离 | Capture Session、Redis/内存 claim、浏览器桥接和平台站点资源同步 | `service/platform_site_capture.go`、`service/upstream_site_test.go`、架构/专项文档和定向测试 |
| 2026-09-30 | v0.2.2 生产数据库与缓存默认值 | Compose 使用浮动 Redis/PostgreSQL 标签、默认密码和未固定的应用数据路径；单容器与完整生产拓扑边界不清晰 | Compose 固定 PostgreSQL 15 + Redis 7，密码由 `.env`/部署脚本生成，服务健康后启动应用，端口为 `3030`，SQLite/无 Redis 回退和既有数据库兼容行为保持不变 | `docker-compose.yml`、`scripts/deploy.sh`、`model/main.go`、`common/redis.go`、中文部署文档 | `docker compose config`、脚本语法检查、隔离三服务栈和真实 SQLite 3.50.4/MySQL 8.2.0/PostgreSQL 15.19 矩阵已通过；生产 Dockerfile 完整构建因 `proxy.golang.org` 超时未完成，最低版本和独立日志库矩阵未覆盖 |
| 2026-10-01 | v0.2.3 部署文档与依赖边界 | 文档没有完整记录单机/多机的共享服务、健康检查、备份和故障边界；Secret 名称与项目执行环境不一致 | 文档明确 Compose 生产默认、外部多机 DSN/Redis、主从任务职责、Redis 拓扑差异和数据迁移限制；Docker 工作流改用 `DOCKER_USERNAME`/`DOCKER_PASSWORD`；web/Electron 依赖最小安全升级不改变缓存、任务和数据库代码 | `README*.md`、`docs/installation/BT.md`、`.github/workflows/docker-*.yml`、web/Electron lockfile | Bun/npm 审计、govulncheck 和隔离 Compose 三服务健康检查已通过；未修改 GORM、数据库驱动、迁移或缓存代码；推送反馈仍有 1 条中等级 Dependabot 警报，认证 API、发布镜像和多架构 manifest 待验证 |
| 2026-10-01 | v0.2.3 发布收尾 | 一次性 Dependabot 处理工作流仍在仓库，缓存与后台任务文档仍记录 #107 开放及正式发布待验证 | 删除一次性工作流；GitHub 远端安全页面确认 #107 已完成处理且没有开放警报；本次只保留发布后镜像、Release 和 Electron 工作流核验，不改变 Redis、Session、任务租约或数据库兼容行为 | `.github/release-notes/v0.2.3.md`、`.github/workflows/dependabot-v023-release.yml`、发布工作流 | 远端安全页面无开放 Dependabot 警报；正式标签推送后核验多架构 manifest、Cosign、Release 产物和镜像运行状态；未修改缓存代码、后台任务代码或数据库 Schema |
| 2026-10-01 | v0.2.4 部署故障修复 | `.env` 密码变化与已有数据库角色密码的差异可能在应用启动后才暴露；Redis 临时卷、应用数据、日志和数据库卷的清理边界不够明确 | 部署前通过 Compose 网络完成 PostgreSQL 密码认证；Redis 仍是缓存/控制面，重建时明确会丢失临时状态；文档区分 `/opt/nexustok/data`、`/opt/nexustok/logs`、PostgreSQL named volume 和 Redis 数据；全新清理不使用全局 prune 且保留非 NexusTok 资源 | `docker-compose.yml`、`scripts/deploy.sh`、`README*.md`、`docs/installation/BT.md` | SQLite、MySQL 8.2、PostgreSQL 15 的实际迁移测试、Compose 配置和脚本验证按 v0.2.4 发布报告记录；最低版本、发布工作流和远端清理未完成前不作完成声明 |

### 9.1 2026-09-26 实现校准

**变更前**：平台站点同步失败只有父渠道级错误，身份、分组、端点、用量、密钥和模型
无法分别判断是否成功；部分密钥读取失败也可能被误解为本次资源为空。

**变更后**：`PlatformSiteResourceSync` 按资源类型保存尝试时间、成功时间、来源接口、
记录数量、脱敏失败原因、部分成功、安全验证要求和是否使用旧快照。资源同步在单个数据库
事务中写入规范化资源表、`UpstreamKey`、`UpstreamKeyAbility`、渠道余额和路由模型索引；
事务失败不会产生半套新快照。

只有 Token/Key 分页和详情、模型能力全部满足当前校验时，才根据“本次未返回”更新密钥
缺失状态。New API 安全验证或 Sub2API Admin Key step-up 拒绝读取密钥时，只标记对应资源
为 `secure_verification_required` 或部分成功，保留最近成功密钥、额度和模型快照，路由
继续使用可用的旧快照。短期认证流程使用 Redis 加密缓存并以内存缓存作为既有回退，流程
TTL 为 5 分钟，取消、过期、失败和消费后均不能继续读取。

数据库验证已运行真实 SQLite 3.50.4、MySQL 8.2.0、PostgreSQL 15.19，覆盖新表建表、
资源快照事务、唯一键、删除清理、旧渠道升级和连续迁移。MySQL 5.7.8 和 PostgreSQL 9.6
最低版本本次未单独启动，不能将本次结果解释为最低版本实测。

### 9.2 2026-09-27 NewAPI 资源快照失败回退

**变更前**：平台站点认证和资源同步以单次总结果为主，`/api/user/self` 已成功而
`/api/token/` 分页失败时，身份、余额和已用额度可能随同资源错误丢失；CookieJar
轮换状态也可能只留在 HTTP 客户端内。权限不足、安全验证、部分分页失败和真正的账号
凭据失效没有足够明确的持久化边界。

**变更后**：认证成功后立即加密保存 `CredentialUpdate`。NewAPI FetchSnapshot 先保存
身份、余额/用量、分组和端点等已成功资源；Token 分页失败写入 `failed` 或
`secure_verification_required` 资源状态并使用最近成功快照，明文 Key 或单密钥模型
失败只标记对应资源为部分/安全验证状态。`UpstreamKey`、`UpstreamKeyAbility` 和
父渠道模型不会因空响应或非完整分页被删除或标记缺失，失败轮次不覆盖账号
`last_sync_at`。资源表中的 `AttemptedAt`、`SucceededAt`、`UsingSnapshot` 和安全验证
标记分别表达本次尝试和最近成功值。

本次没有新增数据库模型或迁移；验证以现有 SQLite 资源快照事务和服务回归为主。MySQL
和 PostgreSQL 既有资源模型兼容性验证仍按仓库数据库规则执行，未将本次服务逻辑测试
冒充最低版本数据库实测。

### 9.3 2026-09-27 平台站点代理失败与最近成功快照

**变更前**：后台平台站点同步未读取渠道 `setting.proxy`，渠道 4 的代理网络失败会在
直连阶段发生；认证和资源请求无法确认是否使用同一出站链路。

**变更后**：`syncPlatformSite` 按渠道读取代理、HTTP 协议和连接分片，并把同一 Transport
注入认证、Refresh、身份、余额、Token、Key、模型和 Admin 资源请求。代理配置无效单独
记录为渠道代理配置错误；认证失败、网络失败、安全验证、权限不足和资源失败不覆盖最近
成功 `last_sync_at`、密钥、模型、倍率、权重和能力快照。该改动未增加表、字段或迁移，
现有 SQLite/MySQL/PostgreSQL 数据结构保持不变。

### 9.4 2026-09-27 按旧版协议写入平台站点快照

**变更前**：NewAPI Token 分页从零起始参数开始时，部分站点会返回不完整或被归一化
的数据；分页、批量 Key、密钥模型和资源失败边界不清晰，可能造成旧密钥被误标缺失。

**变更后**：

- Token 列表从 `p=1&page_size=100` 开始，只有完整分页成功后才允许执行未返回 Key
  的缺失判定；分页中途失败、Key 批量读取失败或安全验证时保留旧记录；
- 批量 Key 只对缺失 ID 做单条补偿，掩码/空值不写入；旧快照中的 Secret、模型、
  倍率、权重和能力继续可用于路由；
- 认证成功后的身份、余额和用量可独立落库，分组、倍率、Endpoint、价格和 Admin
  资源失败写入 `platform_site_resource_syncs` 的独立状态；失败轮次不更新
  `last_sync_at`；
- 父渠道在没有历史成功快照时，只有本轮至少有一个完整 Secret 且模型能力已确认
  才标记成功；没有可用 Key/模型时保持 failed，但不清理身份、余额和最近成功快照。
  如果已有 `last_sync_at`，且管理面认证、身份、余额/用量和 Key 列表成功，但所有
  完整 Key 的 `/v1/models` 因 `INSUFFICIENT_BALANCE`、`GROUP_DISABLED` 等上游资源
  条件失败，则本轮仍可更新管理资源并标记父渠道同步成功；`keys` 标记为
  `partial`、`models` 标记为 `stale`，继续使用最近成功的 Key、模型能力和路由候选。
  该例外不等同于凭据失效，也不允许使用模型广场或分组目录冒充单 Key 能力。当前
  前端资源模型和数据库表结构不变，渠道代理仍复用已有 `setting.proxy` 配置。

### 9.10 2026-10-04 Sub2API 单 Key 模型探测失败与历史快照

**变更前**：Sub2API 管理面登录、Key 列表和额度读取成功，但所有单 Key
`/v1/models` 因余额不足或分组停用失败时，父渠道仍会被判定为资源同步失败，导致
已有可路由模型快照无法通过本轮“剩余额度刷新”继续使用。

**变更后**：`syncPlatformSite` 区分管理资源成功与单 Key 模型探测失败。已有
`last_sync_at` 的渠道在本轮取得完整 Secret、但模型探测全部返回
`INSUFFICIENT_BALANCE`、`GROUP_DISABLED` 等资源条件错误时，保留最近成功的
`UpstreamKey`、`UpstreamKeyAbility` 和父渠道模型并集，同时更新身份、余额、用量、
分组和 Key 资源状态。资源表记录 `keys=partial`、`models=stale`，父渠道同步状态
为成功；没有历史成功快照的新渠道仍必须等待至少一个真实单 Key 模型能力确认。
模型广场、分组模型目录和账号级模型不会被复制为单 Key 能力。

### 9.5 2026-09-30 系统维护任务与缓存边界

**变更前**：维护页直连 GitHub，未有更新/回滚任务，旧版回滚会消耗 `.backup`；Docker
helper 和租约接管未登记。

**变更后**：后端缓存 Release 并经 Root 任务执行更新、回滚和重启；裸机稳定 `.backup`
可重复交换，Docker 使用 socket/helper、唯一候选容器和健康检查；不修改 SystemTask 表
字段或新增迁移。

### 9.6 2026-10-02 Docker 自动更新端口预检

**变更前**：Docker 自更新在旧容器仍运行并发布宿主 `3030` 时，直接启动复用同一
`PortBindings` 的 staging 容器；Docker 会在网络编程阶段返回 `port is already allocated`，
任务停在重建容器阶段，旧容器虽然仍可运行但无法完成应用内更新。

**变更后**：bridge/Compose 网络先用移除宿主端口、Compose 服务标签和网络别名的 preflight
容器完成探活，通过后删除 preflight，再创建保留原始端口、网络别名、重启策略和 Compose
标签的正式容器。正式容器只在旧容器停止并改名后启动；启动或探活失败会移除候选容器、
恢复并重启旧容器，稳定 backup 不被覆盖。`host` 和 `container:<id>` 网络不执行并行
preflight，走停旧后启动正式容器的降级路径。错误原因继续脱敏，不写入数据库 DSN、Redis
密码、Cookie、Token、API Key 或完整环境变量值。

### 2026-10-05 Sub2API Access Token 优先与快照边界

**变更前**：平台站点缓存和后台同步没有区分“Access Token 尚可验证、Refresh Token
已失效”和“Access Token 也已失效”的场景；同步可能在刷新阶段提前失败。

**变更后**：Sub2API 先验证 Access Token，只有当前用户接口明确 HTTP 401 且存在
Refresh Token 才进入轮换；只有 Refresh Token 时直接轮换，轮换成功后再次调用
`auth/me`。因此旧 Refresh Token 不会覆盖有效的管理会话。管理面身份、分组、用量和
Key 成功时，Relay 单 Key 模型探测按独立资源状态记录；部分 Key/模型失败仍写入
`partial/stale`、`using_snapshot` 并保留最近成功 Key、能力和渠道模型。Capture
Session、加密凭据、一次性 claim 和浏览器 Cleanup 隔离不变，也不恢复旧版账号池同步。

### 2026-10-05 Sub2API 采集标量过期时间与恢复请求抑制

**变更前**：Capture 缓存记录只能稳定保留对象形式的浏览器认证字段；数字形式的
`token_expires_at` 可能丢失，且没有 Client ID 的页面可能产生无效 Session Restore
请求。

**变更后**：Helper 接受 JSON 标量认证字段并在写入加密 Capture Credential 前归一化
毫秒过期时间；只有明确存在 `sub2api_auth_client_id`、并由当前页面同源资源发现
`session/restore` 路由时才执行恢复，否则记录脱敏 `not_attempted`。该逻辑保持
Capture Session TTL、Redis/内存一次性 claim、失败 pending 语义和最近成功资源快照
不变，浏览器登录态不进入后台密码 Cleanup。
