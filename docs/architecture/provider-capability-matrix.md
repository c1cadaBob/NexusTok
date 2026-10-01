# 渠道能力矩阵

> 文档状态：代码事实基线
> 事实基线日期：2026-10-01
> 主要代码来源：`constant/channel.go`、`constant/api_type.go`、`common/api_type.go`、`relay/relay_adaptor.go`、`relay/common/relay_info.go`、`relay/channel/*/adaptor.go`、`router/relay-router.go`、`router/task-plugin-protocol-router.go`
> 关联详细文档：[`README.md`](./README.md)、[`relay-routing-and-conversion.md`](./relay-routing-and-conversion.md)、[`implementation-deviations.md`](./implementation-deviations.md)、[`../key-routing-strategy.md`](../key-routing-strategy.md)

## 1. 阅读规则

本矩阵覆盖 `constant.ChannelTypeNames` 当前登记的全部渠道类型（包括 `Unknown`，不包括仅用于计数的 `ChannelTypeDummy`）。表中的“API 映射”来自 `common.ChannelType2APIType`，“Adaptor”来自 `relay.GetAdaptor`；“任务映射”来自 `taskPluginKeys`。没有专门 API 映射的渠道，代码通常返回 OpenAI API 类型并把 `success` 设为 `false`，但 Task Plugin 类型明确返回无映射，不能误当成 OpenAI。

“支持”只表示存在当前代码入口或具体转换方法；它不表示供应商全部模型或所有参数都可用。某个 Adaptor 方法显式返回 `not implemented` 时，仅记录该方法能力，不扩大为整个渠道不可用。

## 2. 矩阵

| 类型 | 渠道名称 | API 类型 / Adaptor | Relay Format 或端点 | 流式选项 | 模型获取 | 异步任务/插件 | 明确限制或核查状态 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 0 | Unknown | 回退 OpenAI / `openai.Adaptor` | 未作为正常渠道入口 | 否 | 待核查 | 否 | 仅常量占位，不能视为可用渠道 |
| 1 | OpenAI | OpenAI / `openai.Adaptor` | OpenAI Chat、Image、Audio、Embedding、Realtime、Responses compact 等 | 是 | Adaptor/模型管理 | `sora` 任务映射 | Responses 创建主要由插件协议提供；具体方法以 Adaptor 为准 |
| 2 | Midjourney | 回退 OpenAI / 通用 Adaptor；任务平台由专用路由处理 | `/mj/*` Midjourney 专用入口 | 否 | 任务/渠道模型 | `mj` 历史任务平台 | 非普通 OpenAI Chat 能力，不能由回退映射推断 |
| 3 | Azure | 回退 OpenAI / `openai.Adaptor` | OpenAI 兼容入口 | 是 | Adaptor/配置 | 否 | 无独立 API 映射，Azure 特殊配置需单独核查 |
| 4 | Ollama | Ollama / `ollama.Adaptor` | OpenAI 兼容 Chat 等 | 是 | Adaptor | 否 | 非 Ollama 所有原生接口均已覆盖 |
| 5 | MidjourneyPlus | 回退 OpenAI / `openai.Adaptor` | 依赖渠道配置，具体任务入口待核查 | 否 | 待核查 | 待核查 | 仅类型注册不足以证明完整能力 |
| 6 | OpenAIMax | 回退 OpenAI / `openai.Adaptor` | OpenAI 兼容入口 | 否 | Adaptor/配置 | 否 | 专用渠道差异待核查 |
| 7 | OhMyGPT | 回退 OpenAI / `openai.Adaptor` | OpenAI 兼容入口 | 否 | Adaptor/配置 | 否 | 专用渠道差异待核查 |
| 8 | Custom | 回退 OpenAI / `openai.Adaptor` | OpenAI 兼容自定义入口 | 否 | Adaptor/配置 | 否 | 具体上游契约由管理员配置，不能从类型推出 |
| 9 | AILS | 回退 OpenAI / `openai.Adaptor` | OpenAI 兼容入口 | 否 | Adaptor/配置 | 否 | 待核查 |
| 10 | AIProxy | 回退 OpenAI / `openai.Adaptor` | OpenAI 兼容入口 | 否 | Adaptor/配置 | 否 | 待核查 |
| 11 | PaLM | PaLM / `palm.Adaptor` | PaLM/Gemini 旧入口 | 否 | Adaptor | 否 | `ConvertGeminiRequest`、Audio、Image、Embedding、Responses 显式未实现 |
| 12 | API2GPT | 回退 OpenAI / `openai.Adaptor` | OpenAI 兼容入口 | 否 | Adaptor/配置 | 否 | 待核查 |
| 13 | AIGC2D | 回退 OpenAI / `openai.Adaptor` | OpenAI 兼容入口 | 否 | Adaptor/配置 | 否 | 待核查 |
| 14 | Anthropic | Anthropic / `claude.Adaptor` | Claude Messages | 是 | Adaptor | 否 | Audio、Image、Embedding 显式未实现；Responses 需核对具体转换 |
| 15 | Baidu | Baidu / `baidu.Adaptor` | OpenAI/百度转换入口 | 否 | Adaptor | 否 | Gemini、Audio、Image、Responses 等方法存在未实现 |
| 16 | Zhipu | Zhipu / `zhipu.Adaptor` | OpenAI/智谱转换入口 | 否 | Adaptor | 否 | 具体 Format 覆盖需按方法核查 |
| 17 | Ali | Ali / `ali.Adaptor` | OpenAI/Claude/图像等具体 Adaptor 入口 | 是 | Adaptor | `alibaba` 任务映射 | Gemini、Audio 等单个方法未实现；任务插件实际绑定类型为 17 |
| 18 | Xunfei | Xunfei / `xunfei.Adaptor` | OpenAI 兼容入口 | 否 | Adaptor | 否 | Gemini、Audio、Image、Embedding、Responses 显式未实现 |
| 19 | 360 | 回退 OpenAI / `openai.Adaptor` | OpenAI 兼容入口 | 否 | Adaptor/配置 | 否 | 待核查 |
| 20 | OpenRouter | OpenRouter / `openai.Adaptor` | OpenAI 兼容 Chat | 否 | Adaptor | 否 | API 类型专用映射但复用 OpenAI Adaptor |
| 21 | AIProxyLibrary | AIProxyLibrary / 无 `GetAdaptor` 分支 | 仅类型/API 映射存在 | 否 | 待核查 | 否 | API 类型已注册但当前 Adaptor 工厂无对应分支，属于待核查/缺口 |
| 22 | FastGPT | 回退 OpenAI / `openai.Adaptor` | OpenAI 兼容入口 | 否 | Adaptor/配置 | 否 | 待核查 |
| 23 | Tencent | Tencent / `tencent.DispatchAdaptor` | 腾讯模型转换入口 | 是 | Adaptor | 否 | 由 DispatchAdaptor 再按模型/请求分发 |
| 24 | Gemini | Gemini / `gemini.Adaptor` | Gemini 原生和兼容入口 | 是 | Adaptor | `google` 任务映射 | 具体原生动作需按 Adaptor 方法核查 |
| 25 | Moonshot | Moonshot / `moonshot.Adaptor` | Claude/Chat 兼容转换 | 是 | Adaptor | 否 | Responses 显式未实现；注释说明使用 Claude API |
| 26 | ZhipuV4 | ZhipuV4 / `zhipu_4v.Adaptor` | 智谱 V4 入口 | 是 | Adaptor | 否 | 具体模型能力需按 Adaptor 核查 |
| 27 | Perplexity | Perplexity / `perplexity.Adaptor` | OpenAI/Claude 转换入口 | 否 | Adaptor | 否 | Gemini、Audio、Image、Embedding 等单个方法未实现 |
| 31 | LingYiWanWu | 回退 OpenAI / `openai.Adaptor` | OpenAI 兼容入口 | 否 | Adaptor/配置 | 否 | 待核查 |
| 33 | AWS | AWS / `aws.Adaptor` | Bedrock/AWS 适配入口 | 是 | Adaptor | 否 | 具体区域、模型和流式行为需按 Adaptor 核查 |
| 34 | Cohere | Cohere / `cohere.Adaptor` | Cohere/Chat/Rerank 入口 | 否 | Adaptor | 否 | Gemini、Audio、Image、Responses、Embedding 等单个方法未实现 |
| 35 | MiniMax | MiniMax / `minimax.Adaptor` | Chat/Audio/任务关联入口 | 否 | Adaptor | `hailuo` 任务映射 | Gemini/部分其他 Format 需按方法核查 |
| 36 | SunoAPI | 回退 OpenAI / `openai.Adaptor`；任务由插件 key | 任务路由而非普通 Chat | 否 | 任务插件 | `sunoapi` | 普通 API 映射不足以证明 Suno 任务完整 |
| 37 | Dify | Dify / `dify.Adaptor` | OpenAI/Dify Chat | 否 | Adaptor | 否 | Gemini、Audio、Image、Embedding、Responses 等未实现 |
| 38 | Jina | Jina / `jina.Adaptor` | Embedding/Rerank/Chat 相关 | 否 | Adaptor | 否 | Gemini、Audio、Image、Responses 等未实现 |
| 39 | Cloudflare | Cloudflare / `cloudflare.Adaptor` | OpenAI/Claude 等具体入口 | 是 | Adaptor | 否 | Image 显式未实现，其他 Format 需按方法核查 |
| 40 | SiliconFlow | SiliconFlow / `siliconflow.Adaptor` | Chat、Image、Embedding 等 | 否 | Adaptor | 否 | Responses 显式未实现 |
| 41 | VertexAI | VertexAI / `vertex.Adaptor` | Gemini/Claude/Vertex 入口 | 否 | Adaptor | `vertex-ai` 任务映射 | Audio、Embedding、Responses 显式未实现 |
| 42 | Mistral | Mistral / `mistral.Adaptor` | OpenAI/Claude 兼容入口 | 否 | Adaptor | 否 | Gemini、Audio、Image、Embedding、Responses 未实现 |
| 43 | DeepSeek | DeepSeek / `deepseek.Adaptor` | OpenAI 兼容 Chat | 是 | Adaptor | 否 | 具体 Format 需按 Adaptor 核查 |
| 44 | MokaAI | MokaAI / `mokaai.Adaptor` | OpenAI 兼容入口 | 否 | Adaptor | 否 | 具体 Format 需按 Adaptor 核查 |
| 45 | VolcEngine | VolcEngine / `volcengine.Adaptor` | 豆包/火山入口 | 是 | Adaptor | `doubao` 任务映射 | 任务映射与普通 API 类型是两条能力路径 |
| 46 | BaiduV2 | BaiduV2 / `baidu_v2.Adaptor` | 百度 V2 入口 | 是 | Adaptor | 否 | 具体 Format 需按 Adaptor 核查 |
| 47 | Xinference | Xinference / `openai.Adaptor` | OpenAI 兼容入口 | 否 | Adaptor | 否 | API 类型复用 OpenAI Adaptor |
| 48 | xAI | xAI / `xai.Adaptor` | OpenAI 兼容入口 | 是 | Adaptor | 否 | 具体模型和 Responses 支持需按方法核查 |
| 49 | Coze | Coze / `coze.Adaptor` | OpenAI 兼容 Chat | 否 | Adaptor | 否 | Image、Responses、Rerank 等单个方法未实现 |
| 50 | Kling | 回退 OpenAI / 任务插件映射 | 视频任务入口 | 否 | 任务插件/平台 | `kling` | 插件 `channelTypes` 声明类型 50；不应根据 OpenAI 回退映射判断同步 Chat |
| 51 | Jimeng | Jimeng / `jimeng.Adaptor` | 图像/任务相关入口 | 否 | Adaptor | `jimeng` 任务映射 | 插件 `channelTypes` 声明类型 51；Chat、Claude、Rerank、Embedding、Audio、Responses 等方法有未实现 |
| 52 | Vidu | 回退 OpenAI / 任务插件映射 | 视频任务入口 | 否 | 任务插件/平台 | `vidu` | 任务能力由插件和渠道绑定决定 |
| 53 | Submodel | Submodel / `submodel.Adaptor` | OpenAI/Claude/Gemini 等转换 | 是 | Adaptor | 否 | 具体上游模型能力需按配置和方法核查 |
| 54 | DoubaoVideo | 回退 OpenAI / 任务插件映射 | 视频任务入口 | 否 | 任务插件/平台 | `doubao` | 与 VolcEngine 共用任务插件 key，不等于相同普通 API |
| 55 | Sora | 回退 OpenAI / 任务插件映射 | OpenAI Video/Responses 插件协议 | 否 | 任务插件 | `sora` | 普通 API 回退仅为兼容路径，任务协议决定实际能力 |
| 56 | Replicate | Replicate / `replicate.Adaptor` | Image/异步预测相关 | 否 | Adaptor | 待核查 | OpenAI、Rerank、Embedding、Audio、Responses、Claude、Gemini 显式未实现；异步预测端到端任务绑定待核查 |
| 57 | ChatGPT Subscription (Codex) | Codex / `codex.Adaptor` | Codex Responses/兼容入口 | 是 | Adaptor | 无 `taskPluginKeys` 直接映射 | 凭据刷新、模型限制和专用 Header 依赖 Codex 逻辑；`sora` 插件声明的 channel type 是 55 和 1，不是 57 |
| 58 | Advanced Custom | AdvancedCustom / `advancedcustom.Adaptor` | OpenAI、Claude、Gemini、Responses、Image 等 | 是 | Adaptor/缓存推断 | 否 | 路由设置和端点推断依赖配置缓存 |
| 59 | Sub2API | Sub2API / `sub2api.Adaptor` | OpenAI 兼容/Responses 等 | 是 | Adaptor | 否 | 平台站点同步和子密钥能力需结合专项文档 |
| 60 | New API | NewAPI / `newapi.Adaptor` | OpenAI、Claude、Gemini、Image、Embedding 等 | 否 | Adaptor | 否 | 平台站点同步和子密钥能力需结合专项文档 |
| 61 | Task Plugin | 无 API 映射 / `GetAdaptor` 不回退 | `openai_responses`、`openai_video`、原生插件路由、通用任务 | 由协议声明 | 插件 Manifest/模型绑定 | 是 | 必须有插件 Generation 和渠道绑定；不能作为普通 OpenAI 渠道 |

### 2.1 平台站点能力补充（2026-09-26）

| 平台 | 认证与刷新 | 资源来源 | 端点与模型能力 | 失败回退 |
| --- | --- | --- | --- | --- |
| New API 平台站点 | 账号密码、自动配置、Access Token、Admin Key、Cookie；支持 `/api/user/login/2fa`、`/api/user/login/verify`；传统刷新与 Dashboard Auth Bundle 均需按结构校验；邮箱输入仅在明确 401 凭据错误后兼容 email/混合主体 | 管理面读取 `/api/status`、当前用户、分组/倍率、价格、Token 分页和完整 Key；Token 资源最多 1000 页；Admin 资源按权限读取 | Relay `/v1/models` 按单个完整 Key 确认；`supported_endpoint`/pricing 只进入端点诊断，账号级模型不能复制给所有 Key | 2FA、安全验证、Bundle 不完整、Admin 资源拒绝、部分分页或单 Key 模型失败只影响对应资源状态，保留最近成功快照 |
| Sub2API 平台站点 | 账号密码、自动配置、Access Token、Admin Key、Cookie；支持 `/api/v1/auth/login/2fa`；邮箱/用户名主体受限兼容；Refresh Token 必须完整轮换 | 管理面按 auth/me、Profile、Groups、Group Rates、Dashboard Usage、Usage Stats、Keys 读取；普通/Admin Key 资源均最多 1000 页；Admin Key 额外读取 accounts/data；参考平台还有 `/api/v1/user/platform-quotas`，当前适配器未调用 | Relay `/v1/models` 或兼容 `/models` 按单个完整 Key 确认；页面 `api_base_url` 分离管理和 Relay 地址；当前无独立账号级模型实现 | Refresh 轮换不确定、step-up、权限不足、WAF、分页失败或单 Key 模型失败不当作密钥不存在，保留最近成功快照 |

这里的“模型获取”列只描述当前实际进入路由的能力来源：New API 的账号模型、价格或
Admin channel 模型用于诊断和展示，不能替代单 Key 能力；Sub2API 当前
`fetchSub2APIModels` 直接返回 `nil`，主要依赖每条完整 Key 请求 Relay `/v1/models`
或 `/models` 探测。参考平台的 Sub2API `/api/v1/user/platform-quotas` 目前不在
NexusTok 适配器请求清单中，因此平台窗口额度仍属于未完整覆盖能力。

### 2.2 2026-10-01 Sub2API 登录服务条款能力

**变更前**

- Sub2API 账号密码能力只登记 `/api/v1/auth/login` 和 `/api/v1/auth/login/2fa`，
  没有公开条款设置读取、revision 可选字段和专用失败状态；
- 条款拒绝可能沿凭据错误或安全验证路径处理，主体候选回退和最近成功快照边界不够明确；
- New API 与 Sub2API 的条款字段隔离没有在能力矩阵中记录。

**变更后**

- Sub2API 密码登录会在规范化管理 Session 上读取 `GET /api/v1/settings/public`。仅当
  `login_agreement_enabled` 为 `true` 且 `login_agreement_revision` 为非空字符串时，
  `/api/v1/auth/login` 的邮箱、用户名和混合主体候选才携带 `agreed_revision`；
- 公开设置失败、404、网络错误、响应格式异常、条款未启用或 revision 缺失时回退旧登录
  请求，不改变现有主体顺序，也不改变仅在明确凭据错误或 HTTP 401 后重试的边界；
- 2FA 阶段重新读取公开设置，并将当时最新 revision 发送到
  `/api/v1/auth/login/2fa`。条款 marker 独立归类为 `login_agreement_required`，
  优先于凭据错误和安全验证错误，不触发主体回退；
- 后台同步写入 `sync_status=failed`、`auth_status=login_agreement_required`、
  脱敏原因和递增失败次数，保留最近成功余额、模型、密钥和能力快照。前端状态复用
  资源面板危险状态样式；
- 本能力只属于 Sub2API。New API 不读取 `/api/v1/settings/public`，不发送
  `agreed_revision` 或 `not_in_cn_confirmed`；NexusTok 不接受客户端条款版本、不持久化
  “已同意”记录，也不会猜测参考源未确认的额外字段。

## 3. 维护解释

- “流式选项”只对应 `streamSupportedChannels`，不表示所有流式 Endpoint 或所有上游事件都可用。
- “模型获取”记录的是代码入口的存在；动态渠道、插件和管理员覆盖模型可能仍受认证、配置、渠道状态和模型限制影响。
- “异步任务/插件”记录 `taskPluginKeys` 或 Task Plugin 路由映射，不表示该渠道的普通同步 Adaptor 具备任务能力。
- 对表中的“未实现”应优先查看具体 `relay/channel/*/adaptor.go` 方法和对应测试；新增能力时关闭具体偏差，不要删除整个渠道的记录。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 覆盖 `ChannelTypeNames` 当前全部渠道类型，并区分 API 映射、Adaptor、可路由能力和任务插件 | `constant/channel.go`、`common/api_type.go`、`relay/relay_adaptor.go`、`relay/channel/`、路由 | 渠道常量、API 映射、Adaptor 工厂、任务映射和流式列表静态核对 |
| 2026-09-26 | 平台站点资源能力补充 | NewAPI/Sub2API 仅以渠道类型和适配器入口描述，未区分登录、刷新、资源和失败回退 | 以参考源真实路由登记 2FA、Bundle/轮换、身份/额度/分组/端点/密钥/模型资源及快照回退 | 平台站点管理与路由前置资源同步 | 参考项目路由、DTO、权限和失败语义静态核对 |
| 2026-09-27 | NewAPI 类平台认证与资源能力校准 | Dashboard Refresh Cookie、Session ID 和管理/Relay 地址边界未列入矩阵；资源权限失败可能被误判为凭据失败 | 列明真实 Refresh 请求契约、CookieJar 轮换优先级、现代 Bundle 拒绝降级、资源级状态和最近成功快照保留规则 | NewAPI 派生平台管理面、子密钥路由模型能力和同步诊断 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/upstream_site_test.go`、本机参考源 |
| 2026-09-27 | 平台站点渠道代理接入 | 管理面请求、密码登录和 2FA 未使用渠道代理，客户端合并可能改变会话安全策略 | 按渠道配置复用代理 Transport，保留 CookieJar、超时和重定向策略，并在矩阵中区分代理配置错误、认证错误、网络错误、安全验证和资源权限失败 | NewAPI/Sub2API 管理面、渠道同步和 Auth Flow | `service/upstream_site.go`、`service/platform_site_auth_flow.go`、`service/upstream_site_test.go` |
| 2026-09-29 | 校准平台站点管理面、Relay 面和模型边界 | 矩阵只按渠道类型描述平台能力，Sub2API 平台窗口额度、账号级模型和单 Key 模型来源边界不够明确 | 明确管理接口与 Relay 接口分层；Sub2API `/api/v1/user/platform-quotas` 为参考平台存在但当前未调用；New API 账号级模型不得复制给所有 Key；当前主要以单 Key Relay 模型探测作为路由能力依据；权限/分页失败保留快照 | New API/Sub2API 资源同步、模型能力、Admin 资源和失败回退 | `service/upstream_site_adapters.go`、`controller/upstream_channel.go`、`model/platform_site_resources.go`、本机参考源和[`平台站点资源获取比较`](../platform-site-resource-acquisition-comparison.md) |
| 2026-09-30 | 旧版资源同步迁移与脱敏 fixture 验收 | Key 资源分页、登录主体兼容、Sub2API 资源顺序和完整 Key 读取优先级与旧版不完全一致；脱敏路由和地址验收未集中记录 | New API Token、Sub2API 普通 Key/Admin Key 分页恢复独立 1000 页；主体兼容、列表完整 Key 优先、详情补齐、单 Key 模型探测、RoutingKey 关系修复、模型映射和 Relay 地址规范化与旧版对齐 | 平台站点管理面、路由前置资源、Key 能力、请求地址和容量验证 | `service/upstream_site_adapters.go`、`service/upstream_site_test.go`、`model/upstream_channel_test.go`、脱敏 HTTP fixture 和 SQLite；未使用真实账号或站点 |
| 2026-10-01 | Sub2API 登录服务条款兼容能力 | 矩阵只登记 Sub2API 的一般密码/2FA 入口，未记录公开条款设置、可选 revision、错误分类和 New API 隔离 | 增加 `GET /api/v1/settings/public` 的条件读取、`agreed_revision` 发送和 2FA 最新 revision；设置故障回退旧协议，条款拒绝独立分类并保留快照；不发送 `not_in_cn_confirmed`，New API 不读取或发送同名字段 | Sub2API 认证、Auth Flow、同步状态、资源面板和平台能力边界 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/platform_site_auth_flow.go`、`service/upstream_site_test.go`；Sub2API 参考源 DTO/路由；SQLite 3.50.4、MySQL 8.2.0、PostgreSQL 15.19 矩阵 |

### 3.2 2026-09-26 实现校准

**变更前**：矩阵只表达 New API/Sub2API 的 Relay Adaptor 能力，无法区分平台站点管理面
认证、资源接口、端点发现和资源失败回退。

**变更后**：平台站点能力以管理面和 Relay 面分开记录。New API 管理面覆盖 status、
self、groups、user models、pricing、Token 分页/Key 详情以及可选 Admin channel
接口；Sub2API 管理面覆盖 auth/me、profile、usage、groups、keys 以及可选 Admin
accounts/data。两者均以实际密钥访问 `/v1/models` 确认子密钥模型能力，New API
`supported_endpoint` 只进入端点能力诊断。

认证矩阵现在区分 New API 传统刷新与 Dashboard Auth Bundle、受限登录主体兼容、Sub2API Refresh Token
轮换和不确定结果；资源矩阵区分身份、余额/用量、分组倍率、端点、密钥和模型资源。
管理员资源被拒绝、安全验证未完成或分页部分失败时使用最近成功快照，不将权限错误标记为
密钥缺失。该矩阵描述当前代码事实，不代表上游 Passkey/WebAuthn、安全证明、Turnstile
或站点风控已由 NexusTok 自动实现。

### 3.3 2026-09-26 渠道 2、4、5 诊断与回退校准

**变更前**：平台站点适配器对 HTML/WAF、HTTP 200 业务失败、网络超时和凭据失败的
可观测类别不完整，兼容请求可能在非 404/405 错误后继续发送；站点同步失败的管理员
状态容易混淆为普通响应格式错误。

**变更后**：New API 使用 `/api/user/login` 的 `username/password`，Sub2API 使用
`/api/v1/auth/login` 的 `email/password`。适配器按 authentication、
interactive_verification、waf_blocked、route_missing 和 transport 分类上游结果，
仅在 404/405 时尝试兼容路径。渠道同步失败保留最近成功密钥、模型、额度、倍率、权重
和能力，并以 `credentials_invalid`、`secure_verification_required`、失败计数和脱敏
响应诊断向管理员展示；渠道 4 网络不可达时保持快照可路由并等待后续重试。

浏览器 Capture/Auth Flow 只承接人工验证，不绕过上游 Turnstile、验证码、Passkey 或
WAF；当前容器缺少浏览器时不把该环境错误显示为响应格式错误。

### 3.4 2026-09-27 NewAPI 类平台管理面契约校准

**变更前**：能力矩阵只登记了 NewAPI 管理资源和一般刷新方式，没有明确
Dashboard Refresh 的 Cookie、Session ID、Bearer Token 组合，也没有明确管理地址与
Relay 地址的边界。资源接口返回 401/403 或分页失败时，容易与站点账号认证失败混淆。

**变更后**：NewAPI 及派生平台的 `BaseURL` 只表示管理 API 根地址并保留路径前缀；
`/api/user/auth/refresh` 使用 `new_api_refresh` Cookie、`X-Auth-Session` 和当前
Bearer Token，CookieJar 的同名轮换值优先持久化。现代 Bundle 结构不完整时不再降级为
旧 Token 协议。身份、余额/用量、分组、价格、Token、明文 Key、模型和 Admin 渠道资源
独立记录状态；权限不足、安全验证或部分分页失败只影响对应资源并使用最近成功快照，
只有明确 404/405 才执行兼容路径回退。该能力矩阵仍不宣称自动完成上游 Passkey、
Security Proof、Turnstile 或 WAF。

### 3.5 2026-09-27 平台站点渠道代理与认证客户端

**变更前**：NewAPI/Sub2API 平台站点管理请求按 `BaseURL` 使用直连客户端，渠道
`setting.proxy` 没有覆盖认证、Refresh、资源和 Auth Flow；注入通用客户端还可能覆盖
平台会话的 CookieJar、30 秒超时和管理站点重定向校验。

**变更后**：后台同步、账号密码登录、2FA 和资源请求按 `channel_id` 读取渠道的
`proxy`、HTTP 协议和连接分片配置，并只替换平台会话底层 Transport。平台会话自己的
CookieJar、显式 Cookie 合并、30 秒超时和允许内网管理地址的重定向校验继续生效。空代理
仍使用平台站点直连客户端，渠道 4 的局域网代理由配置提供，不写入代码。代理配置错误、
网络失败、认证失败、安全验证、权限不足和资源失败分别记录，不能互相降级。

兼容地址规则保持不变：`BaseURL` 是管理 API 根地址并保留路径前缀，Relay 地址不参与
管理路由猜测；只有 HTTP 404/405 才允许继续兼容路径。认证成功但资源失败时保留最近成功
密钥、模型、倍率、权重和能力快照。

### 3.6 2026-09-27 NewAPI 旧版协议与能力边界校准

**变更前**：NewAPI 平台站点矩阵只描述资源类型和一般回退，未明确旧版一基 Token
分页、`/api/status` 的尽力读取、`/api/ratio_config` 的可选状态以及批量 Key 的
部分结果语义；账号级模型与子密钥真实模型能力的边界也不够明确。

**变更后**：

- NewAPI 用户态同步使用旧版主契约：`/api/status` 尽力读取并在失败时使用默认换算，
  `/api/user/login?turnstile=` 只发送 `username/password`，随后读取 `self`、主分组、
  `ratio_config`、Token 分页和 Key；
- Token 主请求为 `/api/token/?p=1&page_size=100`，后续页按一基页码递增，不再以
  `p=0&size=100` 作为主路径；`/api/token`、`/api/tokens`、分组和单条 Key 的兼容
  路径只在 404/405 回退；
- 批量 Key 缺少部分 ID 时仅补偿缺失项，掩码 Key 不能进入路由；Token 自带的
  `model_limits`/`models` 优先作为单个 Key 能力，账号级模型目录只作为诊断和展示；
- 认证成功但可选资源失败时，资源矩阵分别记录 identity、usage、groups、endpoints、
  keys、models 和 admin 状态，保留最近成功快照。父渠道成功要求至少一个完整 Key
  和已确认模型能力；否则保持 failed 但不删除身份、余额和旧快照；
- Dashboard Refresh 仍要求 `new_api_refresh`、`X-Auth-Session` 和旧 Bearer Token，
  渠道代理只从 `setting.proxy` 读取，管理 `BaseURL` 与 Relay 地址继续分离。当前
  前端资源模型不变。
