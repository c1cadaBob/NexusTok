# 数据库、缓存与后台任务

> 文档状态：代码事实基线
> 事实基线日期：2026-09-25
> 主要代码来源：`model/main.go`、`common/database.go`、`common/redis.go`、`model/channel_cache.go`、`model/sync.go`、`service/system_task.go`、`service/task_polling.go`、`main.go`
> 关联详细文档：[`system-overview.md`](./system-overview.md)、[`authentication-and-authorization.md`](./authentication-and-authorization.md)、[`tasks-and-plugins.md`](./tasks-and-plugins.md)、[`../rate-limiting.md`](../rate-limiting.md)

## 1. 功能目标和边界

NexusTok 将主业务数据库、日志数据库、Redis 和进程内缓存分开使用。数据库保存用户、渠道、Token、任务、插件、选项、订阅和日志等权威状态；缓存用于降低查询成本和提供限流/路由索引；后台任务负责周期性维护、同步和异步任务推进。

缓存命中不能被视为数据库事实。文档中的“回退”表示代码存在回源路径，不表示所有节点都能自动获得相同的内存状态。

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

启动迁移中的 `migrateRoutingKeys` 会遍历全部平台站点 `upstream_keys`，不再只处理
`routing_key_id = 0` 的记录。它校验渠道、来源和 `source_ref_id` 三元关系，发现
历史跨渠道引用、来源错误、目标不存在或引用错误时，在事务内为当前子密钥复用或创建
正确的 `platform_site` Routing Key，并更新 `upstream_keys.routing_key_id`。旧的
Routing Key 不做删除，以避免误删其他渠道仍在使用的历史记录。迁移重复执行保持幂等。

## 3. Redis 与内存缓存

Redis 由 `common.InitRedisClient` 初始化，未配置 `REDIS_CONN_STRING` 时关闭。`SYNC_FREQUENCY` 控制多种缓存/同步周期，默认和非法值回退为 60 秒。Redis 可用于：

- 用户鉴权和 Session 快照、版本栅栏和撤销 tombstone；
- Token、订阅、选项或其它业务缓存；
- 渠道/模型和 Routing Key 辅助缓存；
- 全局/用户/关键操作限流；
- 通知和其它短期计数。

启用 Redis 时 `main.go` 同时启用内存缓存兼容路径；未启用 Redis 时部分路径只使用内存或数据库。限流、Session 和渠道缓存的回退语义不同，维护时必须分别查看对应模块。

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
| 平台子密钥 Routing Key 关系 | `upstream_keys`、`routing_keys` | 渠道缓存和路由候选 | 启动迁移、子密钥读取、自动路由和显式选择时一致性校验 |
| 平台站点登录态 | `platform_site_accounts.credential_ciphertext` | `PlatformSiteSession` 内存会话 | 同步认证时访问令牌复用、刷新或密码恢复；令牌更新立即写回 |

## 5. 系统任务 Runner

`service.StartSystemTaskRunner`只在 Master 节点启动。Runner 每约 15 秒或收到唤醒信号执行：

1. 清理过期系统任务锁；
2. 按调度间隔创建启用的周期任务；
3. 为每种任务 Claim 一个待执行记录；
4. 通过数据库租约和锁续租执行 Handler；
5. 写入进度、成功/失败结果和终态。

系统任务的 Type、Payload、State、Result 和 active key 由 `model.SystemTask` 持久化。单一类型已有 active 任务时通常不会重复创建；上游站点同步等任务可使用 `type:channel_id` 作为 active key。

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

每个任务是否启用、运行间隔、Master 限制和失败重试由各自 Handler 的 `Enabled`、配置和系统任务实现决定，不能只根据任务名称判断正在运行。

## 7. 日志存储与多节点一致性

业务日志可以单独落到日志数据库，管理员查询与普通用户视图还会依据字段清理管理员信息。计费饱和标记、密钥诊断和插件诊断等敏感信息应遵守可见性边界。

主数据库共享保证了用户、渠道、任务和系统任务租约的一致性；Redis 共享保证了依赖 Redis 的缓存/限流共享。没有共享 Redis 时，内存缓存、限流和部分 Session 观察会出现节点局部语义。

平台站点同步分为认证阶段和快照阶段。认证阶段先解密账号的双凭据，按缓存
`AccessToken`、`RefreshToken`、密码回退的顺序建立 `PlatformSiteSession`；如果访问令牌
或刷新令牌发生轮换，`syncPlatformSite` 会在余额、用量、模型和子密钥快照开始前调用
`persistPlatformSiteCredential` 立即加密写回。这样快照失败不会回滚已经成功保存的新令牌，
后续同步会从数据库继续使用最新登录态。凭据写回失败会作为独立的“凭据更新”阶段失败
记录，不能继续假定快照可以安全使用。

快照阶段失败时，`PlatformSiteAccount.last_sync_at` 和已有 `upstream_keys` 快照不会被
失败请求覆盖；账号只更新失败状态、连续失败次数和脱敏错误摘要。Sub2API 的非 JSON
响应摘要可能包含 HTTP 状态、脱敏 URL、Content-Type、重定向状态和响应类别，但不包含
响应体、密码、Cookie 或令牌。管理员需要检查站点管理地址、反向代理/API 路径，或通过
已有浏览器采集能力改用 Access Token/Cookie；后台同步不会绕过验证页面。若 Sub2API
登录接口返回 `TURNSTILE_VERIFICATION_FAILED` 等明确的交互验证原因，系统会保留该
JSON 原因，不再让后续网页登录页 HTML 覆盖错误；只有登录路由确认不存在时才尝试
备用路径。只要数据库中已有的 Access Token 或 Refresh Token 仍可用，后台同步会继续
复用缓存登录态，不会因为密码登录暂时需要交互验证而清除最近一次成功快照。

## 8. 当前限制和实际偏差

- 数据库迁移只有 Master 负责，Slave 启动成功不等于已完成迁移。
- Redis 缺失时不止一种回退：有的功能回数据库，有的只在进程内工作，有的功能会被关闭；必须按模块核查。
- 系统任务 Runner 的数据库锁和 Handler 的业务幂等是两层保护，锁丢失或重复唤醒不能被当作绝对的单次执行保证。
- 日志库支持 ClickHouse 不代表主业务库支持 ClickHouse。

## 9. 维护时需要同步的关联模块

修改数据库 DSN、GORM 模型、迁移、锁、缓存键/TTL、Redis 回退、日志字段、后台任务 Type/interval/lease、任务调度或多节点职责时，必须同步本文档、系统总览、任务/插件文档、鉴权/限流专项文档和偏差登记。数据库变更还要按根目录规则完成三数据库验证并记录版本与结果。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立数据库选择、缓存生命周期、系统任务 Runner 和后台任务入口说明 | `model/`、`common/`、`service/system_task.go`、`main.go` | 数据库初始化、Redis、Runner 和启动注册代码静态核对 |
| 2026-09-25 | 缺陷修复 | 启动迁移只处理空 `routing_key_id`；平台同步非 JSON 登录响应无法区分网页、验证页和代理文本 | 启动及访问路径校验并修复全部平台子密钥 Routing Key 关系；同步失败保留成功快照并写入安全响应诊断摘要 | Routing Key 一致性、平台站点同步、缓存刷新、管理员排障 | `model/main.go`、`model/routing_key.go`、`service/upstream_site.go`、模型/服务回归测试 |
| 2026-09-25 | 缺陷修复 | 平台站点认证令牌只在完整快照成功后保存，快照失败会丢失令牌轮换结果 | 认证与快照分阶段处理，令牌更新在快照前立即持久化；快照失败保留最新登录态和旧成功快照 | 平台站点同步、数据库回源和后台任务重试 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`TestSyncPlatformSitePersistsRotatedCredentialBeforeSnapshot` |
| 2026-09-25 | 缺陷修复 | Sub2API 明确的 Turnstile JSON 错误会被备用登录路径的 HTML 响应覆盖，缓存快照诊断不准确 | 按错误类别决定是否切换登录路径；交互验证错误立即停止并安全展示，缓存登录态和最近成功快照继续保留 | 渠道 2 同步任务、登录失败重试、缓存快照和管理员排障 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/upstream_site_test.go` |
