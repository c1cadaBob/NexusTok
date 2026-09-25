# 实现偏差登记

> 文档状态：代码事实基线
> 事实基线日期：2026-09-25
> 主要代码来源：`router/relay-router.go`、`common/api_type.go`、`relay/relay_adaptor.go`、`relay/channel/*/adaptor.go`、`router/task-plugin-protocol-router.go`、`docs/architecture/*.md`
> 关联详细文档：[`README.md`](./README.md)、[`relay-routing-and-conversion.md`](./relay-routing-and-conversion.md)、[`provider-capability-matrix.md`](./provider-capability-matrix.md)、[`../plugin-api/README.md`](../plugin-api/README.md)

## 1. 登记原则

本表只登记能够由当前代码直接证明的偏差。渠道名称、README 宣传、外部供应商能力或单个 Adaptor 的存在都不能作为整个供应商“已支持”的证明。

状态含义：

- **已确认偏差**：预期/外部说明与运行代码的不一致已有直接证据。
- **已实现**：原先登记的缺口已由代码和验证关闭。
- **待核查**：存在风险线索，但尚未完成端到端核查，不能下结论。

## 2. 已确认偏差

| 编号 | 功能模块 | 预期或外部说明 | 当前代码实际行为 | 影响范围 | 状态 | 证据代码路径 | 最近核查时间 | 后续处理建议 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| DEV-001 | OpenAI Images | OpenAI 风格图片接口通常包含 variations | `/v1/images/variations` 路由显式绑定 `controller.RelayNotImplemented` | 该 Endpoint 不可用；不影响 generations/edits 的已实现路径 | 已确认偏差 | `router/relay-router.go` | 2026-09-25 | 实现并补充 Image DTO、Adaptor、计费和测试，或在专项文档明确不支持 |
| DEV-002 | Files API | OpenAI 风格 API 通常支持 Files 上传/查询/内容读取 | `/v1/files`、`/v1/files/:id`、`content` 的 GET/POST/DELETE 都绑定 `RelayNotImplemented` | Files 相关接口不可用 | 已确认偏差 | `router/relay-router.go` | 2026-09-25 | 若实现，需同步存储、权限、上游转换和日志；否则保持明确未实现 |
| DEV-003 | Fine-tuning API | OpenAI 风格 API 通常包含 Fine-tunes 生命周期和事件 | `/v1/fine-tunes` 及其详情、取消、事件接口均显式未实现 | Fine-tune 相关接口不可用 | 已确认偏差 | `router/relay-router.go` | 2026-09-25 | 设计任务/计费/文件依赖后再实现，不要只添加路由 |
| DEV-004 | 删除模型 | OpenAI 风格 Models API 可能允许删除模型 | `DELETE /v1/models/:model` 显式返回 `RelayNotImplemented` | 删除模型接口不可用 | 已确认偏差 | `router/relay-router.go` | 2026-09-25 | 明确仅提供列表/详情，或补充管理语义和权限校验 |
| DEV-005 | Claude Token Count | Claude API 通常提供 `/messages/count_tokens` | 路由代码保留注释，`controller.CountClaudeTokens` 未挂载为公开路由 | 不能通过当前公开 Relay 路由调用 Token Count | 已确认偏差 | `router/relay-router.go` | 2026-09-25 | 若重新开放，补充限流、计费和请求校验 |
| DEV-006 | AIProxyLibrary Adaptor | `ChannelTypeAIProxyLibrary` 在 API 类型映射中有独立 API 类型，按注册直觉应有实现 | `common.ChannelType2APIType` 返回 `APITypeAIProxyLibrary`，但 `relay.GetAdaptor` 没有该 API 类型分支，返回 nil | 该渠道的通用 Relay 能力不能由当前 Adaptor 工厂提供；端到端行为待核查 | 已确认偏差 | `common/api_type.go`、`constant/api_type.go`、`relay/relay_adaptor.go` | 2026-09-25 | 确认是否应复用 OpenAI Adaptor；补充工厂分支、能力矩阵和测试 |
| DEV-007 | Task Plugin API | 普通 OpenAI Responses 入口可被理解为固定内置 Relay | 普通 Relay 路由只显式注册 `/v1/responses/compact`；`/v1/responses` create/retrieve 由插件协议动态挂载 | 未绑定支持协议的插件时，完整 Responses create 不可用；Chat-to-Responses 是内部转换，不是公开入口 | 已确认偏差 | `router/relay-router.go`、`router/task-plugin-protocol-router.go`、`relay/chat_completions_via_responses.go` | 2026-09-25 | 文档和客户端集成必须区分 compact、插件协议和内部转换 |
| DEV-008 | PaLM Adaptor | API 类型/渠道名称可能使调用方认为支持完整 Gemini/PaLM Relay | `palm.Adaptor` 的 Gemini、Audio、Image、Embedding、Responses 转换方法直接返回 `not implemented` | 具体这些 Relay Format 不可用，OpenAI 路径需按方法另行判断 | 已确认偏差 | `relay/channel/palm/adaptor.go` | 2026-09-25 | 按具体方法补实现或在渠道能力 UI 中细分显示 |
| DEV-009 | Replicate Adaptor | Replicate 渠道名称可能被理解为可接收通用 Chat/Embedding/Audio 请求 | `replicate.Adaptor` 明确只实现特定路径；OpenAI、Rerank、Embedding、Audio、Responses、Claude、Gemini 转换方法返回未实现 | 不能把 Replicate 渠道当作通用 OpenAI/Claude/Gemini 渠道 | 已确认偏差 | `relay/channel/replicate/adaptor.go` | 2026-09-25 | 能力矩阵按具体 Endpoint 展示，路由前增加必要能力判断 |
| DEV-010 | 多个 Adaptor 的局部方法 | Adaptor 接口完整不代表所有方法都支持 | 多个 `relay/channel/*/adaptor.go` 对不适用 Format 直接返回 `not implemented` | 某些渠道的 Image、Audio、Embedding、Responses、Rerank 或 Gemini 能力是局部缺失 | 已确认偏差 | 具体 Adaptor 文件；矩阵列出代表性证据 | 2026-09-25 | 新增/修改 Format 时同步矩阵和具体偏差，避免按渠道总称下结论 |
| DEV-011 | 平台子密钥 Routing Key 关系 | 每条平台子密钥应使用当前渠道、`platform_site` 来源且 `source_ref_id` 指向自身的全局 Routing Key | 历史数据中可能出现跨渠道、来源错误、目标不存在或 `source_ref_id` 不匹配；显式 `key_id` 查询此前会直接得到 `record not found`，自动路由却可能继续使用子密钥内容 | 平台站点密钥列表、自动路由、模型获取、渠道 1 这类指定密钥测试 | 已实现 | 修复入口：`model/routing_key.go`、`model/main.go`、`model/upstream_channel.go`、`model/upstream_routing.go`；回归：`model/upstream_channel_test.go` | 2026-09-25 | 保持启动和访问时自愈，后续新增平台密钥关系字段时同步校验与三数据库验证 |
| DEV-012 | Sub2API 登录非 JSON 响应诊断 | 登录接口返回 JSON，非 JSON 响应应能区分网页、验证页、代理文本或非法 JSON | 原先只返回“Sub2API 登录响应格式错误”，没有 HTTP 状态、最终 URL、Content-Type、重定向或响应类别 | 平台站点同步失败排障；无法直接判断管理地址、WAF/验证页或反向代理问题 | 已实现 | `service/upstream_site.go:platformSiteRequest`、`SafePlatformSiteError`、`service/upstream_site_test.go` | 2026-09-25 | 继续禁止记录完整响应体和认证信息；若上游协议变化，补充专用适配器测试 |
| DEV-013 | 平台站点双凭据保存 | `password` 模式应同时保留账号密码和上次成功登录凭据，页面编辑不应清空登录态 | 保存密码模式配置时曾清空 `AccessToken`、`RefreshToken`、`UserID` 和过期时间，导致下一次同步只能重新登录或重新采集 | 平台站点编辑、后台同步、Sub2API/NewAPI 登录频率和上游风控 | 已实现 | `controller/upstream_channel.go`、`model/upstream_channel.go`、`controller/channel_upstream_update_test.go` | 2026-09-25 | 修改凭据保存或认证方式时继续验证字段合并、模式切换清理和加密存储 |
| DEV-014 | 认证凭据更新时机 | 登录或刷新得到的令牌应在认证成功后立即保存，即使后续快照请求失败也应保留轮换结果 | 认证产生的 `CredentialUpdate` 曾在完整余额、模型和子密钥快照成功后才写回，快照失败会丢失新令牌并造成重复登录 | 平台站点同步重试、令牌轮换、上游登录限制和后台错误恢复 | 已实现 | `service/upstream_site.go:syncPlatformSite`、`service/upstream_site_adapters.go`、`service/upstream_site_test.go` | 2026-09-25 | 保持凭据更新与快照写库分阶段；凭据写回失败必须单独报告且不得泄露敏感值 |

## 3. 待核查项目

| 编号 | 功能模块 | 待核查问题 | 当前证据 | 状态 | 最近核查时间 | 下一步 |
| --- | --- | --- | --- | --- | --- | --- |
| CHECK-001 | 渠道端到端能力 | 所有 `ChannelTypeNames` 是否都能在当前配置下成功完成其声明的模型获取、请求、Usage 和计费 | 常量和 Adaptor 注册已核对，但没有对每个上游执行真实端到端测试 | 待核查 | 2026-09-25 | 以测试渠道和最小请求逐类型验证，不以名称推断 |
| CHECK-002 | Task Plugin 历史平台映射 | `taskPluginKeys` 中的每个平台是否都有启用版本、模型绑定和轮询覆盖 | 代码有平台到插件 key 的映射 | 待核查 | 2026-09-25 | 检查运行时 Registry、数据库覆盖、协议支持和实际插件版本 |
| CHECK-003 | 流式选项 | `streamSupportedChannels` 中的支持是否覆盖每个具体 Relay Format 的上游语义 | 代码有渠道级 map，但不是每个 Endpoint 的完整能力矩阵 | 待核查 | 2026-09-25 | 按 Chat/Claude/Gemini/Responses/Realtime 分 Format 验证 |
| CHECK-004 | 模型获取 | 每个 Adaptor 的模型获取结果是否与渠道 `models`、映射和 Routing Key 能力一致 | 有 `GetModelList`/模型管理入口，但外部上游响应取决于配置和网络 | 待核查 | 2026-09-25 | 对模型发现、缓存刷新和失败回退分别做验证 |

## 4. 维护规则

新发现偏差必须先确认“预期来源”与“代码实际行为”都能引用，再新增编号。代码修复时在同一功能提交中：

1. 更新对应架构文档和能力矩阵；
2. 在本表把原记录改为“已实现”或补充新的实际差异；
3. 写入实际日期和变更前/变更后；
4. 运行最小必要验证，并在变更记录或提交说明中留下依据。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 登记路由显式未实现、Adaptor 局部未实现、AIProxyLibrary 工厂缺口和 Responses 入口差异，并区分待核查项 | Relay 路由、Adaptor 工厂、插件协议和后续维护流程 | `router/relay-router.go`、`common/api_type.go`、`relay/relay_adaptor.go`、`relay/channel/` 静态核对 |
| 2026-09-25 | 缺陷修复 | 平台子密钥可能跨渠道引用 Routing Key；Sub2API 非 JSON 登录响应只有笼统错误 | 新增关系一致性自愈、显式旧 `key_id` 兼容和安全响应诊断；原两项缺口标记为已实现并保留证据路径 | 平台站点同步、路由、模型获取、渠道测试和管理员排障 | 模型/服务回归测试、`git diff --check`、SQLite 兼容迁移验证 |
| 2026-09-25 | 缺陷修复 | `password` 模式未稳定保留上次登录凭据，且认证令牌更新依赖完整快照成功 | 同时保留账号密码和登录态，优先复用、刷新并在失效后自动密码恢复；认证成功后在快照前立即持久化令牌 | 平台站点保存、NewAPI/Sub2API 同步、后台任务重试和上游登录限制 | `controller/upstream_channel.go`、`service/upstream_site_adapters.go`、`service/upstream_site.go`；服务/控制器测试 |
