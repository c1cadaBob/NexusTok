# 异步任务与 Sobek 插件原理

> 文档状态：代码事实基线
> 事实基线日期：2026-09-25
> 主要代码来源：`pkg/jsplugin/engine.go`、`pkg/jsplugin/registry.go`、`pkg/jsplugin/routing.go`、`controller/task_plugin.go`、`controller/plugin_protocol.go`、`service/task_polling.go`、`service/task_plugin_audit.go`、`model/task.go`、`model/task_plugin.go`、`relay/plugin_protocol.go`、`router/task-router.go`、`router/task-plugin-protocol-router.go`
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

## 8. 当前限制和实际偏差

- 插件契约声明的是“可调用的 Host 能力”，不是供应商所有接口的清单；`meta.models`、`protocols.supports` 和实际绑定渠道共同决定可用范围。
- Generation number 是节点本地值；诊断跨节点时还要比较数据库 revision 和插件重建结果。
- 产物 Capability 没有普通 URL 的短期过期语义，但 `CRYPTO_SECRET` 轮换和任务/所有者/插件校验仍可使其不可用。
- 历史任务平台映射和新 Task Plugin 路径并存，不能根据某一个平台 key 推断所有任务操作都支持。

## 9. 维护时需要同步的关联模块

修改任务状态、轮询分类、超时、CAS、任务计费、产物访问、插件 Manifest、协议操作、路由、Generation Pinning、并发限制或版本激活时，必须同步本文档、[`plugin-api/README.md`](../plugin-api/README.md)、[`plugin-api/v1.md`](../plugin-api/v1.md)、计费文档、能力矩阵和偏差登记。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立任务生命周期、插件边界、协议、轮询、Generation Pinning 和 Artifact 说明 | `pkg/jsplugin/`、`plugins/tasks/`、`service/task_polling.go`、`router/`、`model/task*` | 插件 API 详细文档、任务路由和轮询代码静态核对 |
