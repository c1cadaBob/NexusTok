# Relay 路由、渠道选择与协议转换

> 文档状态：代码事实基线
> 事实基线日期：2026-09-25
> 主要代码来源：`router/relay-router.go`、`router/video-router.go`、`router/task-router.go`、`router/task-plugin-protocol-router.go`、`middleware/distributor.go`、`service/channel_select.go`、`model/channel_cache.go`、`model/upstream_routing.go`、`relay/relay_adaptor.go`、`relay/common/relay_info.go`、`relaykit/`
> 关联详细文档：[`../key-routing-strategy.md`](../key-routing-strategy.md)、[`../upstream-channel-platform-sites.md`](../upstream-channel-platform-sites.md)、[`provider-capability-matrix.md`](./provider-capability-matrix.md)

## 1. 功能目标和边界

Relay 层将多个上游供应商统一到 OpenAI、Claude、Gemini、Responses、Realtime、Embedding、Audio、Image、Rerank、Midjourney 和任务协议等入口。请求先由统一分发器选定可用渠道，再由 API 类型对应的 Adaptor 完成模型映射、参数转换、上游请求、响应转换和 Usage 归一化。

“渠道类型注册”“API 类型映射”“Adaptor 存在”“具体 Endpoint 支持”是四个独立事实。本文件和[`provider-capability-matrix.md`](./provider-capability-matrix.md)始终按这四层记录。

## 2. 对外入口

| 入口类别 | 当前路由/入口 | Relay Format 或处理方式 |
| --- | --- | --- |
| OpenAI Chat/Completions | `/v1/chat/completions`、`/v1/completions`、`/v1/moderations` | `RelayFormatOpenAI` |
| Claude | `/v1/messages` | `RelayFormatClaude` |
| Gemini 原生/兼容 | `/v1beta/models/*path`、`/v1/engines/:model/embeddings`、`/v1beta/openai/models` | `RelayFormatGemini` 或模型列表 |
| OpenAI Responses | 普通内置入口 `/v1/responses/compact`；插件协议动态注册 `/v1/responses` 和检索 | 内置 compact、插件 `openai_responses`、内部 Chat-to-Responses 转换 |
| Realtime | `/v1/realtime` WebSocket | `RelayFormatOpenAIRealtime` |
| Image | `/v1/images/generations`、`/v1/images/edits`、`/v1/edits` | `RelayFormatOpenAIImage` |
| Embedding | `/v1/embeddings` | `RelayFormatEmbedding` |
| Audio | `/v1/audio/transcriptions`、`/v1/audio/translations`、`/v1/audio/speech` | `RelayFormatOpenAIAudio` |
| Rerank | `/v1/rerank` | `RelayFormatRerank` |
| Midjourney | `/mj/*`、`/:mode/mj/*` | Midjourney 专用 Controller 和任务流程 |
| 视频/通用任务 | `/v1/video/generations`、`/v1/video/generations/:task_id`、`/v1/tasks/:key` 等 | Task Plugin 或历史任务平台 |
| 模型列表 | `/v1/models`、`/v1beta/models` | 根据请求 Header/路径决定 OpenAI、Claude 或 Gemini 列表 |

Claude `/messages/count_tokens` 当前在路由中被注释；文件、Fine-tune、图片 variations 和删除模型等入口显式返回未实现，详见偏差登记。

## 3. 渠道分发顺序

`middleware/distributor.go` 的实际职责包括：

1. 解析请求模型和 Relay Format 需要的模型字段。
2. 检查 Token 的模型限制、用户组和请求级渠道约束。
3. 解析用户组、Auto Group、分组倍率和可用能力。
4. 读取渠道缓存/模型能力索引，过滤禁用、模型不匹配、状态不允许或余额/过期不满足的渠道。
5. 优先使用命中的渠道亲和规则；亲和渠道失效时按配置决定清理亲和缓存或是否跳过一次普通重试。
6. 选择渠道、设置 Context keys，并初始化 `RelayInfo`/`ChannelMeta`。
7. 进入 Controller/Relay；失败后由 Relay 层根据可重试错误、渠道亲和和 Auto Group 状态继续尝试。

Auto Group 的跨分组重试在 `service/channel_select.go`；渠道排序、模型/分组索引和缓存主要在 `model/channel_cache.go`；普通密钥和平台站点子密钥候选在 `model/upstream_routing.go`。

## 4. 渠道和密钥选择

渠道路由必须同时考虑父渠道和密钥层：

```text
请求模型/用户组/约束
  -> 父渠道 enabled、模型能力、过期和平台状态
  -> 父渠道 priority
  -> Routing Key / Channel Key / Platform Site 子密钥过滤
  -> key_priority
  -> weight 加权选择
  -> 上游密钥解密和 Header/参数准备
```

统一 `routing_keys.id` 是稳定的 `key_id`；普通密钥和平台站点子密钥的详细规则、迁移兼容和管理员可见字段以[`key-routing-strategy.md`](../key-routing-strategy.md)和[`upstream-channel-platform-sites.md`](../upstream-channel-platform-sites.md)为准。没有证据时不能把渠道名称推断为可用模型集合。

平台站点需要区分两个 ID：

- `key_id`：全局 `routing_keys.id`，用于路由、日志、模型获取和渠道测试；
- `upstream_key_id`：`upstream_keys.id`，只表示平台站点子密钥记录本身。

平台子密钥的 `upstream_keys.routing_key_id` 只有在以下关系同时成立时才有效：
`routing_keys.id` 等于该字段、`routing_keys.channel_id` 等于
`upstream_keys.channel_id`、`source` 为 `platform_site`，并且
`source_ref_id` 等于 `upstream_keys.id`。启动迁移、平台子密钥列表、自动路由和按
子密钥查询都会校验并修复这组关系；修复时只更新当前子密钥，不删除旧的其他渠道
Routing Key。

自动路由直接从当前渠道的 `upstream_keys` 展开候选，因此历史上可能在错误
`routing_key_id` 存在时仍能成功。显式 `key_id` 测试则按“全局 Routing Key +
当前渠道”严格查询。当前子密钥仍引用旧 ID 时，显式测试会先依据这条当前渠道的
引用关系自愈，再使用新 `key_id` 继续；无法从当前渠道子密钥关系恢复时，返回
“所选密钥已失效，请刷新密钥列表后重新选择”，不会把其他渠道的 Routing Key 直接
挪到当前渠道。接口参数含义和成功响应格式保持不变。

渠道亲和会根据请求 Header、Body、模型或操作设置抽取 key，缓存到渠道亲和索引，并在候选过滤后尝试命中。Codex 等运行时 Header 覆盖和参数覆盖会记录到 `RelayInfo` 的诊断字段，普通用户日志不会自动获得管理员专用信息。

## 5. DTO、映射和转换

Relay 层通常按以下阶段处理请求：

1. Controller 将 JSON、multipart 或 WebSocket 数据解析为 `relaykit/dto` 类型。
2. 请求校验限制 Token、图片数量、任务时长等用户可控计费乘数。
3. `RelayInfo` 记录原始模型、计费模型、目标上游模型、Relay Format 和转换链。
4. Adaptor 根据 API 类型调用 `ConvertOpenAIRequest`、`ConvertClaudeRequest`、`ConvertGeminiRequest`、Image/Audio/Embedding/Rerank/Responses 等转换方法。
5. 应用渠道参数覆盖、运行时参数覆盖、Header 覆盖和渠道认证。
6. `DoRequest`、WebSocket 或插件 Host 发起上游请求。
7. 将上游状态、错误、流式事件、Usage 和模型信息转换为统一下游格式。

`relaykit/` 的转换器独立于根模块；根模块只负责 HTTP、鉴权、数据库、分发和计费。对于可选标量，Relay DTO 使用指针和 `omitempty` 区分“未提供”和显式零值。

## 6. 流式、Usage 和错误

流式选项不是所有渠道都支持，实际支持列表由`relay/common/relay_info.go`的 `streamSupportedChannels` 控制；能力矩阵只记录当前列表，不根据供应商宣传推断。Adaptor 可以返回普通 JSON、SSE、WebSocket 或任务事件，Usage 可能来自上游、转换器估算或任务插件 `usage`。

错误经过 `types.NewAPIError`、Adaptor 和 Controller 归一化；是否重试取决于错误类别、渠道亲和、请求是否已产生下游响应和计费状态。已开始流式输出后不能把所有上游错误都当成可透明重试。

## 7. 模型获取和接口

模型列表入口由 `controller.ListModels`、`controller.RetrieveModel` 和各个 Adaptor 的 `GetModelList`/模型获取逻辑共同实现。内存渠道缓存会把渠道、分组、模型能力和优先级建立索引；管理员触发模型获取、渠道检测或后台更新时还会经过同一套渠道认证和 Adaptor 能力。

模型映射通常由渠道 `model_mapping`、特殊渠道设置或插件协议声明决定。映射后的上游名称只参与上游调用；`RelayInfo.BillingModelName` 保持计费身份与上游名称分离，避免虚拟别名改变渠道选择或价格查找。

## 8. 当前限制和实际偏差

- `/v1/responses` 主要由 `openai_responses` Task Plugin 协议动态提供；不能把 `/v1/responses/compact`、Chat-to-Responses 内部转换和完整 Responses Endpoint 混为一谈。
- `ChannelType2APIType` 对没有专门映射的渠道通常回退 OpenAI API 类型，但 Task Plugin 明确不回退；这只说明默认 Adaptor 路径，不说明每个 Relay Format 都可用。
- 某些 Adaptor 的单个转换方法显式返回 `not implemented`，只能记录为具体方法能力缺口，不能推出整个供应商不可用。
- 传统 Midjourney、视频、音乐和新 Task Plugin 的任务能力由 Task Plugin/任务平台映射决定，和普通同步聊天 Adaptor 是不同入口。
- 平台站点同步依赖上游登录接口返回 JSON。若管理地址命中网页、Cloudflare/WAF
  验证页或反向代理文本，Sub2API 会保持“登录响应格式错误”分类，并在管理员同步
  状态和系统日志中增加 HTTP 状态、脱敏后的最终 URL、Content-Type、是否重定向及
  `html`/`plain_text`/`invalid_json` 等响应类别；完整响应体和认证信息不会保存。

平台站点同步完成认证后才更新密钥候选和能力索引。八个同步阶段分别记录认证、当前用户、
余额/用量、分组倍率、完整密钥分页、Secret、模型能力和 Sub2API 地址发现；可选阶段
告警使用旧快照，密钥分页失败则保留旧密钥集合，不执行缺失密钥处理。Secret 不可用
或模型能力不可确认的子密钥仍可被管理员看到，但过滤出自动路由候选；旧密钥的加密
Secret 和旧模型能力不被失败请求清空。

`waiting_verification` 不等同于普通渠道失败：它表示上游需要真实浏览器完成
Turnstile、CAPTCHA、Cloudflare/WAF 或其它交互验证。系统不伪造验证 Token、不使用
无头浏览器绕过、不接入 CapMonster 等打码服务。Challenge 成功后才刷新 Routing Key
候选和能力缓存；等待或过期期间继续使用最近成功且仍可路由的快照。

## 9. 维护时需要同步的关联模块

修改路由、Relay Format、模型字段、渠道约束、亲和、优先级/权重、Routing Key、平台站点同步、参数/Header 覆盖、流式、Usage、错误重试或模型获取时，必须同步本文档、[`key-routing-strategy.md`](../key-routing-strategy.md)、[`upstream-channel-platform-sites.md`](../upstream-channel-platform-sites.md)、能力矩阵和偏差表。修改计费入口时还要同步[`billing-and-quota.md`](./billing-and-quota.md)。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立入口、分发、密钥选择、转换、流式和 Usage 的实现说明 | `router/`、`middleware/distributor.go`、`model/*routing*`、`relay/`、`relaykit/` | 路由、Adaptor 注册、流式列表和路由选择代码静态核对 |
| 2026-09-25 | 缺陷修复 | 平台子密钥的 `routing_key_id` 可能跨渠道或来源不匹配；Sub2API 非 JSON 登录响应只有笼统错误 | 路由相关入口统一校验并自愈平台子密钥 Routing Key；显式旧 `key_id` 可在当前子密钥引用仍存在时恢复；非 JSON 响应记录安全诊断摘要 | 平台站点自动路由、显式密钥测试、模型获取、Sub2API 同步状态和日志 | `model/routing_key.go`、`model/main.go`、`model/upstream_routing.go`、`service/upstream_site.go` 及对应回归测试 |
| 2026-09-25 | 平台同步编排 | 平台同步失败和交互验证状态会被当作普通路由失败，阶段读取和密钥快照边界不清晰 | 路由只使用成功确认的子密钥；可选阶段告警保留旧值，分页失败保留旧密钥，`waiting_verification` 保留最近成功快照并等待管理员处理 | 渠道过滤、Routing Key、模型能力、余额刷新和 Relay 重试 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`model/upstream_routing.go`、`controller/upstream_channel.go` |
