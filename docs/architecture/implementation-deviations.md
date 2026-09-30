# 实现偏差登记

> 文档状态：代码事实基线
> 事实基线日期：2026-09-30
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

## 3. 待核查项目

| 编号 | 功能模块 | 待核查问题 | 当前证据 | 状态 | 最近核查时间 | 下一步 |
| --- | --- | --- | --- | --- | --- | --- |
| CHECK-001 | 渠道端到端能力 | 所有 `ChannelTypeNames` 是否都能在当前配置下成功完成其声明的模型获取、请求、Usage 和计费 | 常量和 Adaptor 注册已核对，但没有对每个上游执行真实端到端测试 | 待核查 | 2026-09-25 | 以测试渠道和最小请求逐类型验证，不以名称推断 |
| CHECK-002 | Task Plugin 历史平台映射 | `taskPluginKeys` 中的每个平台是否都有启用版本、模型绑定和轮询覆盖 | 代码有平台到插件 key 的映射 | 待核查 | 2026-09-25 | 检查运行时 Registry、数据库覆盖、协议支持和实际插件版本 |
| CHECK-003 | 流式选项 | `streamSupportedChannels` 中的支持是否覆盖每个具体 Relay Format 的上游语义 | 代码有渠道级 map，但不是每个 Endpoint 的完整能力矩阵 | 待核查 | 2026-09-25 | 按 Chat/Claude/Gemini/Responses/Realtime 分 Format 验证 |
| CHECK-004 | 模型获取 | 每个 Adaptor 的模型获取结果是否与渠道 `models`、映射和 Routing Key 能力一致 | 有 `GetModelList`/模型管理入口，但外部上游响应取决于配置和网络 | 待核查 | 2026-09-25 | 对模型发现、缓存刷新和失败回退分别做验证 |

## 3.1 平台站点新增偏差

| 编号 | 功能模块 | 变更前 | 变更后 | 状态 | 证据路径 | 最近核查时间 |
| --- | --- | --- | --- | --- | --- | --- |
| DEV-011 | New API 2FA | 账号密码遇到 `require_2fa` 或安全验证时仅返回普通认证失败 | 通过短期服务端认证流程识别 2FA；Passkey/安全证明仍回退浏览器流程 | 已实现/待上游验证 | `service/upstream_site_adapters.go`、`controller/upstream_channel.go`、New API `controller/twofa.go` | 2026-09-26 |
| DEV-012 | New API Dashboard Auth Bundle | 刷新逻辑只按 Access Token/Refresh Token 解析，可能错误降级现代响应 | 严格校验 `success`、Token、过期时间、当前 Session 和 user；结构不完整时重新认证 | 已实现/待上游验证 | all-api-hub `apiService/newApi/dashboardAuth.ts`、NexusTok 刷新适配器 | 2026-09-26 |
| DEV-013 | Sub2API Refresh Token 轮换 | 刷新响应缺少新 Refresh Token 时没有记录轮换不确定性 | 不重放旧 Refresh Token，标记重新认证并保留资源快照 | 已实现/待上游验证 | all-api-hub `apiService/sub2api/tokenRefresh.ts`、Sub2API `auth_handler.go` | 2026-09-26 |
| DEV-014 | 管理员安全验证与资源失败 | Admin/step-up 拒绝读取资源可能被当作普通空列表 | 资源类型独立记录安全验证要求，旧密钥、额度和模型快照不删除 | 已实现/待上游验证 | Sub2API step-up middleware、平台站点资源同步实现 | 2026-09-26 |
| DEV-015 | 渠道 2、4、5 上游响应诊断与登录回退 | HTML/Turnstile、HTTP 200 业务失败、401 凭据错误和网络不可达可能被合并为响应格式错误；兼容请求可能在非 404/405 后继续尝试 | 固定真实登录 DTO，按认证/交互验证/WAF/路由缺失/传输错误分类，只有 404/405 才回退；同步失败保留最近成功快照并区分 `credentials_invalid` 与 `secure_verification_required` | 已实现/待真实站点持续核验 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/upstream_site_test.go`、参考项目真实 DTO | 2026-09-26 |
| DEV-016 | NewAPI Dashboard Session 与资源权限 | Refresh Cookie、Session ID、Bearer Token 和 CookieJar 轮换优先级未统一；资源 403/分页失败可能污染凭据状态或触发密钥缺失判定 | 统一 `new_api_refresh` Cookie、`X-Auth-Session` 和 Bearer Token 请求；Jar 同名 Cookie 优先持久化；现代 Bundle 不完整时不降级；认证成功后按资源保存快照，权限/安全验证/部分分页失败不标记账号凭据无效或密钥缺失 | 已实现/待真实站点持续核验 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/platform_site_auth_flow.go`、New API/all-api-hub 参考源 | 2026-09-27 |
| DEV-017 | 平台站点渠道代理 | 平台站点同步、密码登录和 2FA 使用直连客户端，客户端合并还可能覆盖平台会话 CookieJar、超时和重定向校验 | 按渠道 `setting.proxy`、HTTP 协议和连接分片复用 Transport；只替换底层 Transport，保留 CookieJar、30 秒超时和管理面重定向校验；代理配置、网络、认证、安全验证和资源失败分层 | 已实现/待真实站点持续核验 | `service/upstream_site.go`、`service/platform_site_auth_flow.go`、`service/upstream_site_test.go` | 2026-09-27 |
| DEV-018 | 旧版与当前平台站点数据模型 | 旧版以 `Channel`、`ChannelAccount` 和 `upstream-account-preview` 快照承载账号/Key 资源，资源状态和能力边界集中在临时快照中 | 当前拆分为 `PlatformSiteAccount`、Identity、Group、Endpoint、ResourceSync、`UpstreamKey`、`UpstreamKeyAbility` 和 `RoutingKey`，并把资源级状态与最近成功快照持久化 | 已确认偏差 | 旧版 `service/upstreamaccount/`、`model/upstream_channel.go`、`model/platform_site_resources.go`、`service/upstream_site.go` | 2026-09-29 |
| DEV-019 | 父渠道 `UsedQuota` 写入语义 | 旧版 `CreateFromPreview` 新建父 `Channel.UsedQuota` 为 `0`，上游 Key 已用额度主要保存在 `ChannelAccount` | 当前优先写入平台账号级已用额度；缺少账号级字段时安全求和子 Key 已用额度，并同步写入 `PlatformSiteAccount`、Identity 和父 `Channel`；该字段表示最近上游累计用量，不等于本地 Relay 消费累计 | 已确认偏差 | 旧版 `service/upstreamaccount/create.go`、当前 `service/upstream_site.go:persistPlatformSiteSnapshot`、`model/upstream_channel.go` | 2026-09-29 |
| DEV-020 | Sub2API 平台窗口额度 | 参考平台提供 `/api/v1/user/platform-quotas`，可返回时间窗口、上限、已用和剩余额度 | 旧版和当前适配器均未完整调用该接口；当前只读取账号/Key 余额与用量，5 小时、日、周、月等窗口额度未进入资源快照 | 已确认偏差 | Sub2API `router/`、`controller/`、`service/` 中 platform-quotas 路由；旧版/当前 `service/upstream_site_adapters.go` | 2026-09-29 |
| DEV-021 | Sub2API 账号级模型 | 平台可能提供账号级模型目录，名称上容易被当作所有 Key 的能力 | 当前 `fetchSub2APIModels` 直接返回 `nil`，没有独立账号级模型能力；主要通过每条完整 Key 请求 Relay `/v1/models` 或 `/models` 探测，不能把账号级目录复制给子 Key | 已确认偏差 | `service/upstream_site_adapters.go:2571`、`model/upstream_channel.go`、`service/upstream_site.go` | 2026-09-29 |
| DEV-022 | 平台 Key 分页上限 | 旧版 New API/Sub2API Key 分页最多尝试 1000 页，每页 100 条 | New API Token、Sub2API 普通 Key 和 Sub2API Admin Key 各自最多尝试 1000 页、每页 100 条；分页中途失败时保留最近成功快照且不执行缺失判定。New API Admin channel 等非本次迁移的管理资源继续使用原独立分页限制 | 已实现/待持续容量验证 | 旧版 `service/upstreamaccount/`、`service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/upstream_site_test.go` | 2026-09-30 |

### 3.2 2026-09-26 实现核对结果

本次对上述偏差进行代码与参考源复核：

- New API 2FA 通过短期认证流程承接，New API Passkey/WebAuthn 和安全证明仍为浏览器
  自动配置边界；
- New API Bundle 识别后严格校验结构，不完整响应不会回退为传统刷新；
- Sub2API 轮换响应缺少新 Refresh Token 或有效 `expires_in` 时不重放旧令牌，并把结果
  保存为需要重新认证/轮换不确定；
- 资源同步保存 `platform_site_*` 规范化资源表，按资源类型处理部分成功和安全验证，
  只有完整分页成功才标记缺失密钥；
- 资源查询和管理端资源面板只返回掩码/存在性、额度、模型和端点诊断，不返回完整密钥
  或认证 Token。

状态仍标记为“已实现/待上游验证”，因为本地自动化验证使用了脱敏的 HTTP fixture 和
本地数据库，未对所有上游部署版本逐一执行真实登录或安全证明流程。

### 3.3 2026-09-27 NewAPI 类平台偏差复核

**变更前**：现代 Dashboard Refresh 的真实请求契约与旧派生站点 Token 协议共用宽泛
解析；CookieJar 轮换值可能重复发送或没有持久化。资源接口的权限错误、安全验证和
分页失败也可能沿认证错误路径处理，影响旧密钥、模型能力和最近成功同步时间。

**变更后**：现代 Bundle 只要出现 Dashboard 结构标志就必须完整校验；刷新请求固定
携带 `new_api_refresh`、`X-Auth-Session` 和旧 Bearer Token，Set-Cookie 轮换值写回
加密凭据，只有确认旧凭据失效的 401 才可触发一次密码回退。`BaseURL` 保留管理路径
前缀，不盲目从 Relay 地址推断管理地址，兼容路径只在 404/405 回退。身份和余额成功
但 Token/Key/模型资源失败时，资源表记录状态并保留最近成功路由快照；管理员可见错误
只含脱敏 URL、状态、Content-Type、响应类别、阶段和回退标记。

本偏差仍标记“待真实站点持续核验”，因为当前测试使用脱敏 HTTP fixture，未复制或使用
参考项目的真实凭据、Cookie、Token、Refresh Token 或环境变量。

### 3.4 2026-09-27 平台站点代理出站复核

**变更前**：渠道代理只用于普通 Relay/其它出站路径，平台站点管理面认证和资源同步
没有读取渠道配置；渠道 4 即使保存了局域网代理，也会绕过代理直连上游。

**变更后**：后台同步以及密码 Auth Flow 的开始和二次验证都按 `channel_id` 获取渠道
代理。适配器只接收代理 Transport，平台站点会话继续拥有自己的 CookieJar、超时和
重定向安全策略。空代理保持原有直连行为；代理配置错误不会伪装成账号密码错误。真实
上游成功与否仍需在能访问渠道 4 局域网代理的运行网络中验收。

### 3.5 2026-09-27 NewAPI 旧版协议校准

**变更前**：当前适配器的 NewAPI Token 主路径曾使用 `p=0&size=100`，旧版
`service/upstreamaccount` 的 status、登录、self、groups、ratio、Token 和 Key 请求
顺序没有完整保留；批量 Key 的部分响应可能导致重复读取，账号级模型也可能被误解为
每个 Key 的能力。

**变更后**：NewAPI 主分页固定使用 `p=1&page_size=100`，密码登录只发送
`username/password`，`/api/status` 失败使用默认换算并单独记录 warning，
`/api/ratio_config` 为可选资源。批量 Key 只补偿缺失 ID，单条 POST 只有 404/405 才
GET 回退，掩码值视为不可用。Token 自身模型字段优先，账号级模型不复制给子密钥；
认证成功、完整分页和至少一个可用 Key/模型才构成父渠道成功条件。

该偏差标记为“已实现/待真实站点持续核验”：测试使用脱敏 HTTP fixture 和本地数据库，
没有读取或提交任何真实平台凭据；如果真实上游返回明确的凭据错误，仍保持
`credentials_invalid`，不能通过兼容路径绕过。

### 3.6 2026-09-30 真实站点黑盒验收

本次验收使用独立隔离浏览器会话和后端一次性内存 HTTP 会话，凭据只在进程内短期使用。
验收范围限定为登录、账号/分组/用量/Key 资源读取、必要的 Key 详情读取和无计费的
`/v1/models` 探测；没有调用聊天、补全、图片、视频或其它产生费用的接口，也没有创建、
删除或修改上游资源。验收结束后关闭隔离页面，未保存截图、网络捕获、响应文件或临时测试
文件。

| 平台 | 脱敏资源结果 | 真实请求观察 | 失败语义核对 | 验收边界 |
| --- | --- | --- | --- | --- |
| New API | 读取 11 条 Key、32 个聚合模型 | 登录、Overview、API Keys、Model Analytics、Usage Logs、Profile 页面；适配器观察到 `/api/user/login`、`/api/user/self`、`/api/status`、分组、Token 和账号级模型资源 | 部分单 Key `/v1/models` 返回 HTTP 403；Key/模型进入安全验证或最近成功快照保护状态，不被当作 Key 不存在 | 只读黑盒验收；不记录账号、密码、Cookie、Access Token、Refresh Token、Admin Key 或完整上游 Key |
| Sub2API | 读取 10 条 Key、18 个聚合模型 | 完成条款确认、登录、Dashboard、API Keys、Usage、Profile 页面；适配器观察到 `/api/v1/auth/login`、`/api/v1/auth/me`、Profile、Groups、Group Rates、Dashboard Stats、Usage Stats、Keys 和页面配置中的 Relay 地址 | 部分单 Key `/v1/models` 返回 HTTP 403；Key 资源为 `partial`、模型资源为 `stale`，保留最近成功快照 | 只读黑盒验收；不记录账号、密码、Cookie、Access Token、Refresh Token、Admin Key 或完整上游 Key |

上述真实站点结果仅作为 2026-09-30 的人工/黑盒验收证据，不作为 CI 输入；后续仍需在
不同上游部署版本和大规模分页数据上持续验证容量与兼容性。

### 3.7 2026-09-30 系统维护更新与可重复回滚

**变更前**：维护页面由浏览器直接请求 GitHub Release，只能展示版本说明和客户端可见的
发布信息；后端没有统一的版本检查、更新、回滚、重启或 Docker 容器管理任务。旧版裸机
回滚会直接消耗 `.backup`，一次回滚后无法可靠地继续切换到上一个版本。

**变更后**：维护页面通过 Root 专属后端接口读取 GitHub Release，并由 `SystemTask`
执行更新、回滚和重启。后端使用约 20 分钟缓存，支持 `v` 前缀、预发布和构建元数据的
语义版本比较，按运行平台匹配发布资产，校验 HTTPS/GitHub 官方资产域名、文件大小和
SHA256 checksum；GitHub 404 转换为无已发布版本，不把原始响应体交给管理页面。源码或
开发构建、Windows 正在运行的二进制、缺少匹配资产或 checksum 的场景保持手动更新提示。

裸机更新在当前可执行文件同目录的临时目录内下载和校验，提交时使用临时交换文件保留
稳定的 `<executable>.backup`。回滚同样交换当前文件和 `.backup`，成功后备份仍存在，
因此可以连续执行回滚或版本切换；任一重命名失败都尝试恢复交换前状态。旧版“把
`.backup` 直接改名为当前文件并消耗备份”的行为不再保留。

Docker 更新和回滚通过 Docker Engine socket 及独立 `system-update-helper` 执行，保留
当前容器的环境、挂载、网络、端口、入口参数、重启策略和 Compose 标签。候选容器使用
唯一 staging/failed 名称，更新前不覆盖稳定备份；有 Healthcheck 时必须达到 `healthy`，
没有 Healthcheck 时只确认容器保持运行并标记降级。主容器停止前转移系统任务租约，helper
启动失败时尝试转回原 runner；候选创建、启动、探活或回滚失败时恢复原容器并写入失败终态。
未挂载 Docker socket 时仅展示脱敏的手动命令提示。

本次不新增数据库字段或迁移；更新、回滚共用 `system_binary_update` ActiveKey，RootAuth、
管理审计、确认对话框和服务端校验仍然有效。管理响应、任务错误、日志、helper 参数和
手动 Docker 命令不得包含 `SESSION_SECRET`、数据库 DSN、Redis 连接串、Cookie、Token、
Admin Key 或完整上游 Key。尚未执行真实生产 Docker 容器切换，source/development build
也不承诺从维护页面自动替换。

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
| 2026-09-26 | 平台站点认证与资源偏差登记 | 平台站点 2FA、现代 Session Bundle、Refresh Token 轮换和安全验证失败回退没有单独记录 | 新增 New API 2FA/Bundle、Sub2API 轮换、管理员 step-up 和资源级失败偏差记录 | 平台站点认证、同步、资源面板 | 参考项目路由和本项目适配器静态核对 |
| 2026-09-26 | 渠道 2、4、5 同步回归核对 | 响应诊断、登录 DTO 和失败快照回退与当前参考源及真实站点表现不一致 | 完成脱敏响应分类、固定登录字段、404/405 回退边界和快照保留回归；安全验证仍需人工浏览器承接 | 平台站点登录、同步失败、管理员诊断 | `service/upstream_site_test.go`；Sub2API/New API/all-api-hub 本机源码；MCP 脱敏观察 |
| 2026-09-27 | NewAPI 类平台 Dashboard 会话与资源状态偏差 | Refresh Cookie/Session ID/Bundle、管理地址边界和资源权限失败语义未在偏差表中单独登记 | 新增 DEV-016，明确 CookieJar 优先级、现代 Bundle 严格校验、401 密码回退限制、资源快照保留和真实站点待核查范围 | NewAPI 派生平台认证、密钥/模型同步、管理员诊断和路由回退 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/upstream_site_test.go`、本机参考源静态核对 |
| 2026-09-27 | 平台站点渠道代理偏差关闭 | 平台站点管理请求未应用渠道代理，导致代理网络和直连网络行为不一致 | 新增 DEV-017 并完成代码、代理/CookieJar/Auth Flow 回归；保留真实上游站点待核验状态，不把本地网络限制当作代码失败 | NewAPI/Sub2API 管理面、渠道 4/5 同步与管理员诊断 | `service/upstream_site.go`、`service/platform_site_auth_flow.go`、`service/upstream_site_test.go`、渠道代理 fixture |
| 2026-09-29 | 旧版与当前平台站点资源差异登记 | 偏差表已有认证、刷新、代理和失败回退记录，但没有单独登记数据模型、父渠道额度写入、Sub2API 平台窗口额度、账号级模型和分页上限差异 | 新增 DEV-018 至 DEV-022，明确当前实现与旧版/参考平台的事实差异；已修复的旧版协议偏差继续保持“已实现/待真实站点持续核验”，不把静态核对写成真实站点验证 | 平台站点资源模型、额度、Key 生命周期、模型能力和缺失判定 | `docs/platform-site-resource-acquisition-comparison.md`、`service/upstream_site.go`、`service/upstream_site_adapters.go`、`model/upstream_channel.go`、本机参考源静态核对 |
| 2026-09-30 | 旧版资源同步迁移与真实站点黑盒验收 | New API/Sub2API 三类 Key 资源仍受 100 页上限影响，偏差表未记录迁移后的容量边界和真实站点只读结果 | 三类 Key 资源独立恢复最多 1000 页；补充 New API 11 条 Key/32 个聚合模型、Sub2API 10 条 Key/18 个聚合模型的脱敏验收结果，并明确部分 `/v1/models` 的 HTTP 403 只进入安全验证、partial 或 stale 快照状态 | 平台站点分页、完整 Key 读取、模型能力、资源快照和安全交付 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/upstream_site_test.go`、隔离浏览器 DevTools MCP；未调用计费接口 |
| 2026-09-30 | 系统维护更新与可重复回滚 | 维护页直连 GitHub，只能查看 Release；没有 Root 后端更新、回滚、重启任务，旧版回滚会消耗唯一 `.backup` | 增加 RootAuth 后端版本检查、SystemTask 更新、回滚、重启、GitHub 缓存与 checksum 校验；裸机和 Docker 均使用稳定备份交换并支持重复回滚；失败时恢复原状态并记录脱敏终态 | 维护页面、Root 管理、系统任务、裸机文件、Docker helper、管理审计和错误日志 | `controller/system_update.go`、`service/system_update.go`、`service/system_update_docker.go`、`model/system_task.go`、前端维护测试；SQLite、MySQL 8.2.0、PostgreSQL 15.19 系统任务兼容性用例通过；未单独验证最低版本和真实生产 Docker 切换 |
