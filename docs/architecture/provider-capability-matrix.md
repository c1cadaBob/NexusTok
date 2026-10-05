# 渠道能力矩阵

> 文档状态：代码事实基线
> 事实基线日期：2026-10-05
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
| New API 平台站点 | 账号密码、自动配置；Access Token、Admin Key、Cookie 仅作为历史渠道兼容能力读取和同步，新版不提供手动录入；支持 `/api/user/login/2fa`、`/api/user/login/verify`；传统刷新与 Dashboard Auth Bundle 均需按结构校验；邮箱输入仅在明确 401 凭据错误后兼容 email/混合主体 | 管理面读取 `/api/status`、当前用户、分组/倍率、价格、Token 分页和完整 Key；Token 资源最多 1000 页；Admin 资源按权限读取 | Relay `/v1/models` 按单个完整 Key 确认；`supported_endpoint`/pricing 只进入端点诊断，账号级模型不能复制给所有 Key | 2FA、安全验证、Bundle 不完整、Admin 资源拒绝、部分分页或单 Key 模型失败只影响对应资源状态，保留最近成功快照 |
| Sub2API 平台站点 | 账号密码、自动配置；Access Token、Admin Key、Cookie 仅作为历史渠道兼容能力读取和同步，新版不提供手动录入；支持 `/api/v1/auth/login/2fa`；邮箱/用户名主体受限兼容；Refresh Token 必须完整轮换 | 管理面按 auth/me、Profile、Groups、Group Rates、Dashboard Usage、Usage Stats、Keys 读取；普通/Admin Key 资源均最多 1000 页；Admin Key 额外读取 accounts/data；参考平台还有 `/api/v1/user/platform-quotas`，当前适配器未调用 | Relay `/v1/models` 或兼容 `/models` 按单个完整 Key 确认；页面 `api_base_url` 分离管理和 Relay 地址；当前无独立账号级模型实现 | Refresh 轮换不确定、step-up、权限不足、WAF、分页失败或单 Key 模型失败不当作密钥不存在，保留最近成功快照 |

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

- Sub2API 密码登录默认不读取公开设置，首请求严格按参考 DTO 发送邮箱/密码或用户名/
  密码。只有初次登录响应明确包含条款要求 marker 时，才读取
  `GET /api/v1/settings/public`；
- `login_agreement_enabled=true` 且 `login_agreement_revision` 非空时，使用相同登录
  路由和相同主体携带 `agreed_revision` 重试一次。设置失败、404、网络错误、响应格式
  异常、条款关闭或 revision 缺失时直接返回条款错误，不扩展主体或路由组合；
- 2FA 阶段重新读取公开设置，并将当时最新 revision 发送到
  `/api/v1/auth/login/2fa`。条款 marker 独立归类为 `login_agreement_required`，
  优先于凭据错误和安全验证错误，不触发主体回退；
- 后台同步写入 `sync_status=failed`、`auth_status=login_agreement_required`、
  脱敏原因和递增失败次数，保留最近成功余额、模型、密钥和能力快照。前端状态复用
  资源面板危险状态样式；
- 本能力只属于 Sub2API。New API 不读取 `/api/v1/settings/public`，不发送
  `agreed_revision` 或 `not_in_cn_confirmed`；NexusTok 不接受客户端条款版本、不持久化
  “已同意”记录，也不会猜测参考源未确认的额外字段。

### 2.3 2026-10-03 Sub2API Relay 地址与登录回退能力

**变更前**

- 页面重定向后的 HTML 可能仍按原始管理地址解析相对 `api_base_url`，独立 Relay
  域名可能被丢弃；
- 登录请求可能带有混合主体或无条件条款字段，严格 DTO 部署会返回
  `400 INVALID_REQUEST`，路由回退和主体回退边界不够明确。

**变更后**

- 页面请求使用最终响应 URL；相对 `api_base_url` 按最终页面地址解析。页面明确声明的
  Relay 只有在与最终页面来源或原始管理地址同协议、同主机、同有效端口，或与原始
  管理地址满足严格直接 `api.` 父子域关系时才接受，并继续执行 SSRF、端口和重定向
  安全校验；
- 管理地址继续负责登录、用户、分组、用量和 Key；Relay 只负责单 Key
  `/v1/models`/`/models` 探测和最终转发。`hhw1231.com -> hengwenapi.com` 属于允许的
  “最终页面明确声明”场景，未经页面声明的第三方地址仍拒绝；
- 对 `hhw1231.com -> hengwenapi.com`，本轮管理请求使用最终可信页面来源，持久化管理
  地址仍保持原始地址；Relay 探测清空 Cookie Jar，只发送当前 Key 的 Bearer 和
  `x-api-key`，不复制管理 Cookie、Origin、Referer、`X-Requested-With` 或
  `X-Auth-Session`。分组/账号模型目录不能替代单 Key 能力确认，未确认模型的 Key
  不进入路由；
- 邮箱首请求只包含 `email/password`；`/api/v1/auth/login` 只在 404/405 时回退
  `/auth/login`，只有 401 凭据错误才允许邮箱到用户名的一次回退。400
  `INVALID_REQUEST`、403、429、WAF、安全验证、权限、网络和 5xx 不触发额外尝试；
- 资源、分页或单 Key 模型失败继续使用最近成功快照，不把认证成功后的资源错误
  写成凭据失效或错误执行 Key 缺失判定。

### 2.4 2026-10-04 浏览器 Capture Helper 兼容能力

**变更前**：自动配置在页面尚未登录时可能立即结束，NewAPI 只有 Cookie 和
`localStorage.uid` 的旧版页面缺少稳定的 `New-Api-User` 兼容，Sub2API 旧版
Session Restore 所需的 IndexedDB 客户端 ID 也可能无法读取。

**变更后**：Capture Helper `1.7.0` 在 `DOMContentLoaded` 后启动，并以固定间隔和手动
按钮等待登录态形成。NewAPI 候选依次检查 Dashboard Refresh、Access Token、Admin Key
和 Cookie；Cookie 验证使用数字 `New-Api-User`，用户 ID 来源包括 `localStorage.uid`、
用户状态对象、页面状态和 JWT，同源脚本 API 路径发现只用于兼容部署前缀。Sub2API
继续支持 localStorage、hash、页面状态、Refresh 和 Session Restore，并从
`sub2api-auth-coordination/values/sub2api_auth_client_id` 读取客户端 ID，发送
`X-Sub2API-Auth-Client`，恢复 Token 后必须通过 `/api/v1/auth/me`。

候选必须通过当前用户或 Admin 权限验证才可完成采集；暂时未登录保持 `pending`，不向
完成接口提交失败。浏览器 Capture 登录态不执行后台注销，诊断只保留脱敏阶段、路径、
存在性、版本和错误类别。Helper 合并可见与 `GM_cookie` 的同站 Cookie，并只提交扁平
身份字段；管理地址路径从 handoff 保持。Cloudflare/Turnstile、WAF、验证码和 Passkey 仍属于上游
浏览器人工验证能力，不在矩阵中宣称由 NexusTok 自动绕过。

无扩展时，`capture_bridge_url` 返回与 UserScript 相同的采集核心。桥接脚本优先读取
当前页面 handoff；登录重定向丢失查询参数时，通过 `window.opener.postMessage` 向活动
管理页请求一次性 handoff。跨源 `javascript:` 导航被浏览器安全策略拒绝时，管理页复制
不含敏感值的短启动片段；片段在上游页面上下文请求桥接脚本后再执行。该降级不会绕过
上游 Turnstile/WAF，也不会放宽 Origin、窗口、Capture ID 或版本校验。

### 2.5 2026-10-05 旧版浏览器采集路径兼容能力

**变更前**：能力矩阵已记录 Capture Helper 的候选顺序和 IndexedDB Client ID，但
Dashboard Refresh、Refresh Token、Session Restore 与反向代理子路径的同源路由发现没有
覆盖到全部分支。

**变更后**：

- NewAPI 的 Dashboard Refresh 同时尝试固定 `/api/user/auth/refresh`、同源 JavaScript
  发现路径和部署前缀候选；请求保留页面 Origin、Cookie 和可用数字 `New-Api-User`，
  Bundle 必须包含成功标志、Bearer、当前 Session、用户身份、Access Token 和有效期；
- Sub2API 的 Refresh Token 支持 `/api/v1/auth/refresh`、`/api/auth/refresh`、
  `/auth/refresh` 及同源发现路径；Session Restore 继续携带
  `X-Sub2API-Auth-Client`，恢复后通过三条兼容 `auth/me` 路径验证；
- handoff、`api_base_url`、页面配置、当前页面和存储状态中的 `/api`/`/api/v1` 后缀
  会被剥离，只保留反向代理部署前缀，避免重复拼接 API 根；
- 这只是浏览器登录态采集能力的兼容扩展，不恢复旧版账号池、Preview 或
  ChannelAccount 同步体系，不改变 Relay Adaptor、单 Key 模型能力、最近成功快照或
  后台密码同步 Cleanup。站点关联仍限制为同 Host 或严格 `api.` 对等 Host。

### 2.6 2026-10-05 Sub2API `custom_endpoints` Relay 能力

**变更前**：Sub2API 能力矩阵默认把 `api_base_url` 作为管理和模型地址；对
`api_base_url` 为空、页面公开 `custom_endpoints` 指向独立 Relay 的定制部署，没有
区分管理认证地址、Relay 模型地址和服务端信任复核。

**变更后**：

| 能力 | 当前实现 |
| --- | --- |
| 页面发现 | Helper 与服务端读取 `custom_endpoints`、`customEndpoints`，支持最终页面解析的相对地址；`api_base_url` 优先，首个合法自定义端点回退 |
| 管理认证 | Sub2API 管理地址继续访问 `/api/v1/auth/me`、分组、用量、Key 和兼容路径；管理地址仍受同 Host 或严格 `api.` 关系限制 |
| Relay 模型 | 页面声明的 Relay 用于每条完整 Key 的 `/v1/models` 或 `/models`；请求隔离管理 Cookie、Bearer、Origin、Referer、`X-Requested-With` 和 `X-Auth-Session` |
| 安全复核 | 外部 Relay 必须通过 URL/SSRF 检查、匿名页面重读和规范化完全匹配；客户端单独回传、同注册域名或未声明地址均拒绝 |
| 真实验收 | 指定站点管理地址与 `api-image.shour.bond` Relay 已拆分；渠道启用，7 条 Key 中 7 条 `ModelsSynced=true`，资源部分失败继续使用最近成功快照 |

该能力不改变“单 Key 模型能力确认后才进入路由”的规则，也不恢复旧版账号池、Preview、
`ChannelAccount` 同步链路；浏览器采集态不参与后台密码同步 Cleanup。

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
| 2026-10-02 | 平台站点密码会话清理与自动配置能力 | 密码同步复用历史登录态的边界、NewAPI/Sub2API 精确注销路径、浏览器采集所有权和表单隐藏旧凭据入口未在矩阵中统一登记 | 密码同步每次重新登录；NewAPI 登出后精确删除 SID，Sub2API 仅 Refresh Token 登出；资源失败保留快照；自动配置完成验证后加密保存并一次性消费，平台从渠道类型派生 | 平台站点认证、资源同步、Capture Helper、渠道表单和跨节点缓存 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/platform_site_capture.go`、`web/src/features/channels/components/drawers/platform-site-fields.tsx`、本机参考源和定向测试 |
| 2026-10-03 | 认证流程保存兼容与 NewAPI 加密登录 | 完成认证流程后残留用户名可能被当作手动凭据；NewAPI 加密登录密钥、RSA-OAEP/v2 信封和明文回退边界未在矩阵中登记 | 有流程 ID 时只提交流程材料，后端兼容用户名残留并使用流程真实身份；NewAPI 按新版协议读取密钥并仅在 404/405 回退明文；Sub2API 保持邮箱优先和本轮 Refresh Token 精确登出 | 平台站点创建/编辑、账号密码认证、错误回退和会话清理 | `web/src/features/channels/lib/channel-form.ts`、`controller/upstream_channel.go`、`service/newapi_password_encryption.go`、`service/upstream_site_adapters.go`、定向测试和本机 New API/Sub2API/all-api-hub 参考源 |
| 2026-10-05 | 旧版浏览器自动采集路径迁移 | Capture 刷新/恢复分支仍部分固定 API 路径，反向代理子路径和同源脚本路由发现没有覆盖所有浏览器采集分支 | NewAPI Dashboard Refresh、Sub2API Refresh/Session Restore 与 `auth/me` 统一使用 `expandedAPIPaths`，合并固定候选、页面配置、当前页面和同源资源发现；不改变加密凭据、一次性 claim、严格 Host 关系、最近成功快照或后台 Cleanup | Capture Helper、Capture Bridge、NewAPI/Sub2API 浏览器登录态和反向代理部署 | `service/platform_site_capture.go`、`service/upstream_site_test.go`、旧版 `service/upstreamaccount/capture.go`、本机参考源 |
| 2026-10-03 | Sub2API Relay 发现与严格登录请求 | 重定向页面的相对 Relay 地址和独立 Relay 域名无法稳定进入 Key 模型探测；无条件条款/混合主体字段可能导致 `400 INVALID_REQUEST`；管理态可能被带到 Relay | 使用最终 HTML URL 解析相对地址，接受最终页面或原始管理地址明确声明的同协议/主机/有效端口 Relay，以及严格 `api.` 父子域；可信最终页面来源作为本轮管理请求根地址但不覆盖持久化地址；Relay 请求隔离管理 Header/Cookie Jar；邮箱首请求严格 `email/password`，400 不回退，只有 404/405 回退路由、401 凭据错误回退主体；未确认单 Key 模型不进入路由，资源失败保留快照 | Sub2API 管理/Relay 地址、认证、Key 能力和路由可用性 | `service/upstream_site_adapters.go`、`service/upstream_site_test.go`、Sub2API `auth_handler.go`、all-api-hub 真实站点辅助实现和脱敏 fixture |

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
  keys、models 和 admin 状态，保留最近成功快照。没有历史成功快照时，父渠道成功
  要求至少一个完整 Key 和已确认模型能力；已有 `last_sync_at` 的 Sub2API 渠道如果
  管理面和 Key 列表成功、但所有单 Key `/v1/models` 因 `INSUFFICIENT_BALANCE`、
  `GROUP_DISABLED` 等资源条件失败，则父渠道仍可成功更新管理资源，`keys` 为
  `partial`、`models` 为 `stale`，继续使用旧 Key 能力和路由候选；模型广场或分组
  模型目录不能替代单 Key 能力。其它没有历史快照的情况保持 failed，但不删除身份、
  余额和旧快照；
- Dashboard Refresh 仍要求 `new_api_refresh`、`X-Auth-Session` 和旧 Bearer Token，
  渠道代理只从 `setting.proxy` 读取，管理 `BaseURL` 与 Relay 地址继续分离。当前
  前端资源模型不变。

### 3.7 2026-10-02 平台站点密码会话与 Capture 所有权

**变更前**：能力矩阵没有明确后台密码同步是否复用已保存 Token、Cookie、Refresh
Token 和 Session ID，也没有记录资源读取之后的注销路径；自动配置和历史登录态的
清理范围容易与用户浏览器会话混淆。

**变更后**：密码模式每轮只使用账号密码，NewAPI 以 `POST /api/user/auth/logout`
和精确 `DELETE /api/user/sessions/{sid}` 清理本轮会话，Sub2API 以只含本轮
Refresh Token 的 `POST /api/v1/auth/logout` 清理；两者均不撤销其他设备会话。浏览器
Capture 属于用户已有会话，不进入后台 Cleanup。自动配置候选在提交前验证当前用户或
Admin 权限，诊断脱敏，Capture 记录用 Redis/内存 claim 防止并发重复消费；无数据库
字段变更。

### 3.8 2026-10-03 认证流程与密码协议能力

**变更前**：平台能力矩阵只描述了账号密码认证和会话清理，没有标出认证流程保存请求
与旧客户端残留用户名的兼容边界，也没有区分 NewAPI 新版加密登录和旧版明文协议的
降级条件。

**变更后**：

- 认证流程完成后，前端仅提交平台、管理地址、认证方式、一次性流程 ID 和金额配置；
  后端允许仅有用户名残留的旧请求继续解析流程，但拒绝任何密码、Token、Cookie、
  Session ID、Admin Key 或其它真实登录材料；
- Sub2API 账号密码请求首选严格的 `email/password`，只有明确凭据错误或 HTTP 401
  才最多一次尝试 `username/password`；首路由只有 404/405 才回退
  `/auth/login`，400 `INVALID_REQUEST`、条款、安全验证、WAF、限流、权限和网络错误
  不触发主体或路由重试。资源完成后只以本轮 Refresh Token 调用
  `/api/v1/auth/logout`；
- NewAPI 先读取 `/api/user/login/encryption-key`。加密开启时使用 RSA-OAEP-SHA256，
  长密码使用 AES-GCM v2 信封；只有 404/405 的缺路由才发送旧版明文字段，403、5xx、
  网络错误和无效加密配置不降级；
- 上述协议不改变历史 Access Token、Admin Key、Cookie 和浏览器采集渠道的读取兼容性，
  这些登录态不属于密码同步创建的临时会话，也不执行后台注销。

### 3.9 2026-10-05 Sub2API Access Token 优先与资源同步能力

**变更前**：能力矩阵未明确浏览器采集同时包含 Access Token、Refresh Token 时的优先级；
旧 Refresh Token 失效可能导致有效 Access Token 被提前判定为无效，资源能力无法刷新。

**变更后**：

- Sub2API 自动配置优先验证 Access Token；只有 `auth/me` 明确 HTTP 401 且存在
  Refresh Token 才调用刷新，只有 Refresh Token 时直接刷新，轮换后重新验证当前用户；
- 管理接口继续使用管理地址和已验证登录态，页面公开声明的 Relay 仅用于完整 Key 的
  `/v1/models` 能力确认。Relay 请求不带管理 Cookie、Bearer、Origin、Referer、
  `X-Requested-With` 或 `X-Auth-Session`；
- 真实站点复核已确认 Capture Session、身份、管理地址、Relay 地址和至少一条本轮
  `ModelsSynced=true` 的 Key 能力均可用；其余部分失败保持 `partial/stale` 最近成功
  快照，不把未确认模型加入路由；
- 当前实现仍保留 Capture Session、加密 `PlatformSiteCredential`、一次性 claim、
  Helper/Bridge 来源校验，不恢复旧版账号池、Preview 或 `ChannelAccount` 同步，浏览器
  采集登录态不参与后台密码 Cleanup。

### 3.10 2026-10-05 Sub2API 标量过期时间与 Session Restore 能力

**变更前**：能力矩阵只记录了 Sub2API 的 Token、Refresh 和 IndexedDB 恢复来源，
没有记录 `token_expires_at` 的数字标量兼容，也没有限定恢复请求必须具备 Client ID
和真实同源路由。

**变更后**：Capture Helper 可读取对象或标量形式的 `token_expires_at`，并归一化毫秒
时间；Sub2API 只有在存在 `sub2api_auth_client_id` 且同源脚本发现
`session/restore` 时才执行 Browser Restore。真实站点没有 Client ID 时直接沿
`auth_token + auth_user + auth/me` 主路径完成，缺少恢复材料只标记
`not_attempted`，不影响管理资源、Relay 模型探测或最近成功快照。

### 3.11 2026-10-05 Sub2API Cookie Client ID 与配置包裹能力

**变更前**：能力矩阵只登记 localStorage/sessionStorage 和 IndexedDB 的恢复材料；
`custom_endpoints` 被 JSON.parse 或有限配置包裹时的发现边界未单独登记。

**变更后**：Sub2API Helper 按 localStorage、精确 `sub2api_auth_client_id` Cookie、
IndexedDB 顺序读取恢复 Client ID，仍要求同源 `session/restore` 路由；页面配置只递归
明确命名的包裹字段，服务端用 `common.Unmarshal` 解析转义 JSON。该能力不扩展
`@connect`、不传播管理会话、不改变管理/Relay 双地址、已验证 Key 模型能力和快照
保留规则。
