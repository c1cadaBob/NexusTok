# 系统总览与请求生命周期

> 文档状态：代码事实基线
> 事实基线日期：2026-09-25
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
7. 注册渠道检测、上游模型更新和异步任务轮询等系统任务，启动 `service.StartSystemTaskRunner`。
8. 创建 Gin Server，配置可信代理、Request ID、版本、i18n、日志和静态前端，再由 `router.SetRouter` 安装路由。

主节点和从节点的职责由 `common.IsMasterNode` 及各个任务实现共同控制。系统任务 Runner 只在 Master 节点启动；从节点可以提供请求服务，但不能被文档理解为拥有独立的迁移、调度或账务权威。

## 3. 模块职责边界

| 层 | 当前职责 | 不应假设拥有的权威 |
| --- | --- | --- |
| `router/` | 声明路径、HTTP 方法、中间件顺序和协议入口 | 不负责最终鉴权、渠道选择或结算 |
| `middleware/` | CORS、解压、请求体保护、Token/Session、限流、分发、插件 pinning | 不替代 Controller 的参数业务校验 |
| `controller/` | 解析请求、组装 DTO、调用 Relay/Service、写 HTTP 响应 | 不应被视为上游协议实现本身 |
| `service/` | 计费、鉴权会话、任务轮询、系统任务和跨模型业务流程 | 不直接改变 Relay DTO 的协议语义 |
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

### Master/Slave

多节点必须共享主数据库。Master 负责迁移和系统任务调度，系统任务模型通过数据库租约、Claim 和状态更新避免多个 Master 重复执行。节点报告、授权策略同步和缓存同步仍有各自的刷新周期，不能把“请求可由多个节点处理”理解为所有内存状态自动一致。

## 6. 对外接口、配置和数据模型

- 管理和用户 API：`router/api-router.go`。
- Relay API：`router/relay-router.go`、`router/video-router.go`、`router/task-router.go`。
- 动态插件协议：`router/task-plugin-protocol-router.go`、`router/plugin-router.go`。
- Web 静态资源：`router/web-router.go` 和 `web/dist` 嵌入资源。
- 主数据模型：`model/*.go`，数据库类型和迁移入口在 `model/main.go`。
- 请求上下文和单次请求数据：`RelayInfo`、Gin Context keys、`model.Channel`、`model.Token`、`model.Task`、`model.SystemTask`。
- 运行配置：`common.InitEnv`、`setting/`、环境变量和管理端 Option。

## 7. 当前限制和实际偏差

- 路由注册不等于 Endpoint 完整实现；显式 `RelayNotImplemented` 的接口见[`implementation-deviations.md`](./implementation-deviations.md)。
- 统一请求入口中，内置 `/v1/responses` 创建入口不是普通静态 Relay 路由；Responses 插件协议由插件动态绑定，另有 `/v1/responses/compact` 和 Chat-to-Responses 内部转换，详见[`relay-routing-and-conversion.md`](./relay-routing-and-conversion.md)。
- 独立 Redis、内存缓存和多节点部署会影响限流、缓存传播和插件生成观察窗口，不能只根据单节点测试判断集群语义。
- `relaykit/` 必须独立构建，根模块的配置、数据库和计费不能下沉到该模块。

## 8. 维护时需要同步的关联模块

修改启动顺序、节点职责、路由安装或中间件顺序时，必须同步 `main.go`、`router/main.go`、对应路由和本文档。修改数据库/Redis/日志边界时同步 `model/main.go`、`common/redis.go`、缓存模型和[`data-cache-and-background-jobs.md`](./data-cache-and-background-jobs.md)。修改用户请求链路时同步鉴权、路由、计费和能力矩阵文档。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立启动、分层、请求链路、部署边界和限制说明 | `main.go`、`router/`、`middleware/`、`model/`、`service/`、`relay/` | `main.go`、`model/main.go`、`router/` 静态核对 |
