# 渠道能力矩阵

> 文档状态：代码事实基线
> 事实基线日期：2026-09-26
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
| New API 平台站点 | 账号密码、自动配置、Access Token、Admin Key、Cookie；支持 `/api/user/login/2fa`、`/api/user/login/verify`；传统刷新与 Dashboard Auth Bundle 均需按结构校验 | `/api/status`、`/api/user/self`、用户组、`/api/pricing`、Token 分页/Key 详情、必要时 Admin API | `/v1/models` 按单个密钥确认；`supported_endpoint` 等 pricing 数据进入端点能力诊断 | 2FA、安全验证、Bundle 不完整或 Admin 资源拒绝只影响对应资源状态，保留最近成功快照 |
| Sub2API 平台站点 | 账号密码、自动配置、Access Token、Admin Key、Cookie；支持 `/api/v1/auth/login/2fa`；Refresh Token 必须完整轮换 | `/api/v1/auth/me`、profile、usage、groups、keys；Admin Key 额外读取 admin accounts/data | `/v1/models` 按单个密钥确认；页面 `api_base_url` 分离管理和 Relay 地址 | Refresh 轮换不确定时不重放旧令牌；step-up 拒绝不当作密钥不存在，保留旧快照 |

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

### 3.2 2026-09-26 实现校准

**变更前**：矩阵只表达 New API/Sub2API 的 Relay Adaptor 能力，无法区分平台站点管理面
认证、资源接口、端点发现和资源失败回退。

**变更后**：平台站点能力以管理面和 Relay 面分开记录。New API 管理面覆盖 status、
self、groups、user models、pricing、Token 分页/Key 详情以及可选 Admin channel
接口；Sub2API 管理面覆盖 auth/me、profile、usage、groups、keys 以及可选 Admin
accounts/data。两者均以实际密钥访问 `/v1/models` 确认子密钥模型能力，New API
`supported_endpoint` 只进入端点能力诊断。

认证矩阵现在区分 New API 传统刷新与 Dashboard Auth Bundle、Sub2API Refresh Token
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
