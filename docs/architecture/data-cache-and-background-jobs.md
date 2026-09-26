# 数据库、缓存与后台任务

> 文档状态：代码事实基线
> 事实基线日期：2026-09-26
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

## 8. 当前限制和实际偏差

- 数据库迁移只有 Master 负责，Slave 启动成功不等于已完成迁移。
- Redis 缺失时不止一种回退：有的功能回数据库，有的只在进程内工作，有的功能会被关闭；必须按模块核查。
- 系统任务 Runner 的数据库锁和 Handler 的业务幂等是两层保护，锁丢失或重复唤醒不能被当作绝对的单次执行保证。
- 日志库支持 ClickHouse 不代表主业务库支持 ClickHouse。

### 8.1 平台站点资源快照（2026-09-26）

平台站点同步按身份、分组、端点、密钥、模型和用量资源独立记录尝试时间、成功时间、
状态、来源接口、数量、脱敏失败原因、部分成功和安全验证要求。同步失败时不覆盖最近
成功值；只有完整分页和资源校验成功后才允许将未返回密钥标记为缺失。安全验证或
Admin Key step-up 拒绝读取密钥时保留旧密钥和模型快照，平台路由继续使用最近成功
快照。资源查询接口读取规范化资源表和现有 `UpstreamKey` 路由主数据，不把短期认证
流程或任何敏感凭据写入缓存响应。

## 9. 维护时需要同步的关联模块

修改数据库 DSN、GORM 模型、迁移、锁、缓存键/TTL、Redis 回退、日志字段、后台任务 Type/interval/lease、任务调度或多节点职责时，必须同步本文档、系统总览、任务/插件文档、鉴权/限流专项文档和偏差登记。数据库变更还要按根目录规则完成三数据库验证并记录版本与结果。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立数据库选择、缓存生命周期、系统任务 Runner 和后台任务入口说明 | `model/`、`common/`、`service/system_task.go`、`main.go` | 数据库初始化、Redis、Runner 和启动注册代码静态核对 |
| 2026-09-26 | 平台资源同步补充 | 平台站点同步主要以单个总快照和站点状态表示 | 增加资源类型独立状态、最近成功快照保留、完整分页缺失判定和安全验证失败回退基线 | 平台站点缓存、同步任务、资源查询 | `service/upstream_site.go` 当前快照逻辑与新增资源模型设计核对 |
