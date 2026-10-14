# 异步任务与 Sobek 插件原理

> 文档状态：代码事实基线
> 事实基线日期：2026-09-30
> 主要代码来源：`pkg/jsplugin/engine.go`、`pkg/jsplugin/registry.go`、`pkg/jsplugin/routing.go`、`controller/task_plugin.go`、`controller/plugin_protocol.go`、`service/task_polling.go`、`service/task_plugin_audit.go`、`service/system_task.go`、`service/system_update.go`、`service/system_update_docker.go`、`model/task.go`、`model/system_task.go`、`model/task_plugin.go`、`relay/plugin_protocol.go`、`router/task-router.go`、`router/task-plugin-protocol-router.go`
> 关联详细文档：[`../plugin-api/README.md`](../plugin-api/README.md)、[`../plugin-api/v1.md`](../plugin-api/v1.md)、[`../plugin-api/v1.schema.json`](../plugin-api/v1.schema.json)、[`../plugin-api/v1.d.ts`](../plugin-api/v1.d.ts)、[`billing-and-quota.md`](./billing-and-quota.md)

## 1. 功能目标和边界

任务系统同时支持同步 Relay、异步视频/音乐/图像任务、历史平台任务和声明式 Task Plugin。Go Host 负责网络连接、持久化、轮询、重试、鉴权、计费、结算、状态 CAS 和产物访问；Sobek JavaScript 插件只负责同步数据转换和协议呈现。

插件不是一个拥有数据库和网络权限的脚本运行环境。插件返回的 URL、Header、任务数据和 Usage 都需要 Host 校验并在 Host 约束下使用。

## 2. 任务入口和生命周期

| 阶段 | 实际入口 | 主要职责 |
| --- | --- | --- |
| 提交 | `/v1/video/generations`、`/v1/tasks/:key`、插件协议 create、历史 `/mj/*` | 鉴权、渠道选择、插件 pinning、计费预扣、提交上游 |
| 创建响应 | 同步完成、pending Response 或 Task 对象 | Host 规范化 ID、状态、模型、时间和必要的私有字段 |
| 轮询 | `service/task_polling.go`、系统任务 `async_task_poll` | 批量或单任务查询、状态解析、失败分类、CAS 更新 |
| 终态 | `SUCCESS`、失败、取消、超时等 | 结算/退款、消费日志、任务结果和 Artifact 投影 |
| 读取 | `/v1/video/generations/:task_id`、`/v1/tasks/:key`、Responses retrieve | 读取最新持久化快照，不把读取当成再次上游请求 |
| 产物 | `/v1/tasks/:key/artifacts`、`.../content`、Video content | Capability 校验、所有者/插件/任务检查、代理下载 |

异步任务的状态更新使用条件更新/CAS 思路，避免过期轮询结果覆盖较新的终态。系统任务 Runner 通过数据库租约执行批量轮询，失败分类和重试阈值以 `service/task_polling.go` 为准。

系统维护另有 Root 专属的系统任务链路：`system_update` 和 `system_rollback` 共享
`system_binary_update` ActiveKey，更新/回滚只能存在一条活动任务。任务 Handler 负责将
版本检查、下载、checksum、文件交换或 Docker 候选容器状态写入 State，并在成功或失败
时写入 Result/Error 终态。Docker 更新在主容器停止前创建 helper 并转移租约，helper
只初始化数据库、租约和 Docker Engine 操作，不启动 HTTP 服务、Redis runner 或插件。
该链路与普通用户异步任务的计费、Artifact 和插件状态无关。

`routing_key_recovery` 是另一类按分钟调度的系统任务。它由 Master 节点通过现有
`SystemTask` 类型租约单实例执行，扫描统一 `routing_key_id` 的到期恢复候选，显式
选择普通多密钥或平台站点子密钥执行一次渠道测试，并把脱敏的测试数量写入 Result。
恢复阶段和下次到期时间保存在 `routing_key_healths`，不使用 Redis；手动禁用密钥和
手动禁用父渠道不进入候选。

## 3. 插件编译、校验和注册

插件由 `pkg/jsplugin` 读取 ECMAScript 模块，经编译、Manifest Schema/类型校验、协议/路由冲突检查后进入 Registry。内置插件位于 `plugins/tasks/`；管理员上传插件是高信任操作，启用前需要通过校验和路由生成检查。

插件可声明：

- `meta.routes`：插件拥有的原生厂商路径；
- `meta.protocols`：声明使用 Host 拥有的 `openai_responses` 或 `openai_video` 协议；
- `models`、`channelTypes`、`allowedHosts`、`baseUrl` 和 Usage Schema；
- `listArtifacts`、`buildContentRequest` 等产物方法。

Registry 发布是 generation-atomic。请求在开始时 pin 一个插件生成/端点/路由；请求生命周期内不能因后台重载自动切换插件对象。后台轮询可以使用较新的 active 版本，但新版本必须能够继续解析在途任务。

## 4. Sobek 运行时权限边界

当前插件契约是同步 ECMAScript 模块。插件不能直接使用 `fetch`、文件系统、`require`、`import` 或 `async`；网络请求由 Go Host 发起，持久化、重试、轮询、计费和结算由 Go 代码拥有。

Host 对插件提交请求执行：

1. 依据固定生成选择插件和渠道。
2. 调用 `buildSubmitRequest`/协议 Hook。
3. 校验 URL 是否落在渠道 Base URL 或插件允许的 Host。
4. 由 Go HTTP 客户端发起请求并解析响应。
5. 调用插件解析 Hook，再由 Host 保存任务和账务状态。

插件协议 Hook 可转换请求、响应、错误、事件和 Usage，但不能降低 Host 的认证、SSRF、请求体、并发、计费或产物授权限制。

## 5. Responses、Video 和任务产物

`openai_responses.create` 可声明 `stream`、`sync`、`background` 模式；模式能力在渠道选择前校验，不支持的形式不会进入插件 Hook，也不会计费。`GET /v1/responses/:response_id` 是统一检索入口；任务成功后 Host 执行插件的 `listArtifacts`，通过长效签名 Capability URL 将 Artifact map 注入 `renderEvents`/`renderFinal`。

Capability URL 不是上游 URL。读取时 Host 仍要加载任务、用户和插件，并按 `access` 查询参数、请求方法、Range/条件 Header 子集和 SSRF 规则执行代理。轮换 `CRYPTO_SECRET` 会使已签发 URL 失效。非终态或失败任务不会获得 Artifact map。

插件可以通过 `task.data` 暴露上游快照，但 Host 会覆盖 `id`、`object`、`model`、`status`、进度和时间等公共字段，并移除私有/遗留字段。该过程是字段级投影，不是字节级透传。

## 6. 任务轮询、超时和结算

`service/task_polling.go` 将轮询响应分类为成功、其它客户端错误、Not Found、认证错误、临时错误、无法识别状态、Hook 错误和传输错误。非终态的临时/可重试错误会累积失败次数；超过规则后任务失败并进入退款或结算路径。

任务提交前 `RelayInfo.ForcePreConsume` 强制预扣，终态时根据插件 Usage、任务状态和计费快照执行结算。轮询任务应携带插件版本/生成相关信息，以确保在插件升级后仍使用能够解释已保存数据的版本。

## 7. 对外接口、配置和数据模型

- 通用任务：`router/task-router.go`、`controller/task_plugin.go`。
- OpenAI Video：`router/video-router.go`、`router/task-plugin-protocol-router.go`。
- Responses：`router/task-plugin-protocol-router.go`、`controller/plugin_protocol.go`、`relay/plugin_protocol.go`。
- 插件原生动态路由：`router/plugin-router.go`、`pkg/jsplugin/routing.go`。
- 插件状态：`model/task_plugin.go`、插件 Registry Generation。
- 任务状态：`model/task.go`、私有任务数据和 Artifact 投影。
- 插件契约：[`plugin-api/v1.md`](../plugin-api/v1.md)、Schema 和 TypeScript 声明。
- 并发和 Origin：`controller/plugin_protocol_limiter.go`、任务 Artifact 中间件及相关环境变量。
- 系统维护任务：`controller/system_update.go`、`service/system_update.go`、
  `service/system_update_docker.go`、`model/system_task.go`。

## 8. 当前限制和实际偏差

- 插件契约声明的是“可调用的 Host 能力”，不是供应商所有接口的清单；`meta.models`、`protocols.supports` 和实际绑定渠道共同决定可用范围。
- Generation number 是节点本地值；诊断跨节点时还要比较数据库 revision 和插件重建结果。
- 产物 Capability 没有普通 URL 的短期过期语义，但 `CRYPTO_SECRET` 轮换和任务/所有者/插件校验仍可使其不可用。
- 历史任务平台映射和新 Task Plugin 路径并存，不能根据某一个平台 key 推断所有任务操作都支持。
- 系统维护自动更新只支持具备可验证 Release/checksum 的裸机发布构建或挂载 Docker socket
  的容器；source/development build、Windows 正在运行的二进制和无 socket 容器只提供
  手动提示。Docker 容器更新在 bridge/Compose 网络下先用不发布宿主端口的 preflight
  容器探活，再停旧并启动正式容器；`host` 和 `container:<id>` 网络走停旧后探活的降级路径。

## 9. 维护时需要同步的关联模块

修改任务状态、轮询分类、超时、CAS、任务计费、产物访问、插件 Manifest、协议操作、路由、Generation Pinning、并发限制或版本激活时，必须同步本文档、[`plugin-api/README.md`](../plugin-api/README.md)、[`plugin-api/v1.md`](../plugin-api/v1.md)、计费文档、能力矩阵和偏差登记。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立任务生命周期、插件边界、协议、轮询、Generation Pinning 和 Artifact 说明 | `pkg/jsplugin/`、`plugins/tasks/`、`service/task_polling.go`、`router/`、`model/task*` | 插件 API 详细文档、任务路由和轮询代码静态核对 |
| 2026-10-13 | 密钥自动恢复测试 | 没有统一的密钥级恢复系统任务，失效/自动禁用密钥只能依赖真实请求或人工操作产生健康样本 | 新增 `routing_key_recovery` 定时任务，复用 SystemTask 租约扫描普通多密钥和平台站点子密钥，恢复请求只写入健康样本并按首次失败 30 分钟、首次成功后 5 次/5 分钟确认推进 | `controller/system_task_handlers.go`、`model/routing_key_health.go`、`model/routing_key.go`、`service/upstream_key_usage.go` | SQLite 定向模型/控制器测试和前端 37 个受影响用例通过；MySQL/PostgreSQL DSN 未配置 |
| 2026-10-13 | 按发布版本选择系统维护目标 | 系统任务没有保存发布版本目标，更新/回滚依赖 `.backup` 文件或稳定备份容器 | `SystemTask` 的 JSON Payload/State/Result 保存目标版本、目标镜像、数据备份 ID、备份/恢复状态；更新默认选择最新可用 Release，回滚接收并校验 `target_version`；Docker 镜像拉取和 preflight 后才备份，备份失败不停止正式容器，切换已改变运行实例后失败才恢复数据；不新增数据库字段或迁移 | `service/system_update.go`、`service/system_update_backup.go`、`service/system_update_docker.go`、`model/system_task.go`、维护页 | 系统更新 Go 定向测试、Docker 备份/恢复边界测试、任务脱敏测试和前端定向测试；未执行生产切换和完整三数据库矩阵 |
| 2026-10-14 | 平台站点请求级使用日志成本 | 平台站点成本由独立 SystemTask 按 UTC 五分钟窗口查询，无法及时匹配时还会延迟重试 | 移除新的平台成本窗口任务注册；请求成功或失败记日志时只处理一次，响应携带成本优先，否则一次性读取 Sub2API `/api/v1/admin/usage` 或 NewAPI `/api/log/`，失败立即写估算值；保留历史窗口模型和表结构，日志数据库、ClickHouse、Redis 不参与成本持久化 | `controller/system_task_handlers.go`、`service/platform_site_usage_cost.go`、`model/platform_site_usage_cost.go`、`controller/log.go` | Go 定向服务/模型/控制器测试、前端成本展示测试；真实上游管理员接口及完整三数据库矩阵需按最终报告核对 |
| 2026-10-02 | Docker 更新任务端口预检 | 更新任务在旧容器仍绑定宿主端口时启动同端口 staging 容器，可能因端口占用失败 | 增加无端口 preflight 探活和正式容器停旧后启动流程；失败时恢复旧容器并保留稳定 backup | `service/system_update_docker.go`、系统更新任务状态和失败终态 | `go test ./service -run 'Docker(Update\|Updated\|Readiness\|Helper\|Pull\|Staging)' -count=1` |
