# constant 包 (`/constant`)

该目录用于放置全局可复用的常量、轻量类型和常量映射。它不承载业务流程，不访问数据库，不调用第三方服务，也不依赖项目内其他自定义包。

## 当前文件

| 文件 | 说明 |
|------|------|
| `api_type.go` | 定义上游 API 协议类型，例如 OpenAI、Anthropic、Gemini、AWS Bedrock、Responses 兼容协议等，用于适配器选择和请求/响应格式转换。 |
| `azure.go` | 定义 Azure OpenAI 兼容行为相关常量，例如 `AzureNoRemoveDotTime`。该文件只使用 Go 标准库。 |
| `cache_key.go` | 定义用户、Token 等缓存键格式字符串和缓存字段名，统一 Redis/内存缓存命名规则。 |
| `channel.go` | 定义渠道类型、渠道基础 URL、渠道映射和渠道归属展示信息，是 relay 分发和渠道管理的基础枚举。 |
| `channel_credential_mode.go` | 定义渠道凭证模式和账号池选择模式，例如单 Key、多 Key、专属账号池、全局账号池、轮询、随机和优先填充。 |
| `context_key.go` | 定义 `ContextKey` 类型以及请求链路中使用的上下文键，覆盖 Token、Channel、User、分组、模型映射和响应元信息等。 |
| `endpoint_type.go` | 定义统一端点类型，例如 OpenAI Chat Completions、OpenAI Responses、Anthropic Messages、Gemini、Jina Rerank、Images、Embeddings 等。 |
| `env.go` | 定义启动阶段由环境变量或配置注入的全局运行参数，例如流式超时、请求体限制、任务开关、Token 计数和日志开关。 |
| `finish_reason.go` | 定义统一的模型完成原因常量，例如 `stop`、`tool_calls`、`length`、`function_call` 和 `content_filter`。 |
| `midjourney.go` | 定义 Midjourney 错误码、动作常量和模型到动作的映射。 |
| `multi_key_mode.go` | 定义多 Key 选择模式，例如随机和轮询。 |
| `setup.go` | 定义系统是否完成初始化安装的全局标记 `Setup`。 |
| `task.go` | 定义异步任务平台、任务动作和模型到动作的映射，例如 Suno 和 Midjourney 任务。 |
| `waffo_pay_method.go` | 定义 Waffo 支付方式展示信息与 Waffo API 参数映射。 |

## 使用约定

1. `constant` 包只能被其他包引用，禁止在此包中引用项目内其他自定义包；确有必要时，仅允许引用 Go 标准库。
2. 不要在此目录编写业务流程、数据库操作、缓存读写、HTTP 调用、provider SDK 调用、配置解析或权限判断逻辑。
3. 新增常量文件时，请保持命名语义清晰，并在本 README 的“当前文件”表格中补充说明。
4. 如果某个值需要在运行期间从数据库、环境变量或后台设置动态变更，应放在 `setting/`、`common/`、`service/` 或对应业务包中，不应放入 `constant`。
5. 如果某段逻辑需要根据常量执行分支，请把分支逻辑留在调用方或领域服务里，`constant` 只提供稳定标识。

违反以上约定会让底层常量包反向依赖业务层，增加循环依赖、测试初始化成本和后续差异同步风险。提交新增常量前，请先确认这里是否真的是最小、最稳定的归属位置。
