# 渠道适配器系统

渠道适配器是 NexusTok 的核心扩展点，负责将统一的请求格式转换为 40+ 上游 AI 服务提供商的特定格式，并将响应转换回统一格式。

## 架构概览

```
客户端请求（OpenAI/Claude/Gemini 格式）
  → 分发中间件选择渠道
  → 根据 API 类型选择适配器
  → 适配器转换请求格式
  → 发送到上游 API
  → 适配器解析响应
  → 返回统一格式给客户端
```

## 适配器接口

### 标准适配器（Adaptor）

用于同步 API（Chat、Embedding、Image、Audio 等）。

**生命周期：**

1. `Init(info)` — 初始化适配器，注入中继信息（渠道类型、Key、URL 等）
2. `GetRequestURL(info)` — 构建上游 API 请求 URL
3. `SetupRequestHeader(c, req, info)` — 设置请求头（认证、Content-Type 等）
4. `ConvertXxxRequest(c, info, request)` — 将统一请求转换为上游格式
5. `DoRequest(c, info, requestBody)` — 发送请求到上游
6. `DoResponse(c, resp, info)` — 处理上游响应，返回使用量信息

**请求转换方法：**

| 方法 | 用途 |
|------|------|
| `ConvertOpenAIRequest` | Chat Completion、Completion 等接口 |
| `ConvertClaudeRequest` | Claude 格式请求 |
| `ConvertGeminiRequest` | Gemini 格式请求 |
| `ConvertEmbeddingRequest` | Embedding 接口 |
| `ConvertImageRequest` | 图像生成接口（DALL-E 等） |
| `ConvertAudioRequest` | 音频接口（TTS、STT） |
| `ConvertRerankRequest` | Rerank 接口 |
| `ConvertOpenAIResponsesRequest` | OpenAI Responses API |

### 任务适配器（TaskAdaptor）

用于异步任务型 API（Midjourney、Suno、视频生成等）。

**生命周期：**

1. `Init(info)` — 初始化
2. `ValidateRequestAndSetAction(c, info)` — 验证请求并设置任务动作
3. `EstimateBilling(c, info)` — 估算计费（预扣费），返回倍率乘数
4. `BuildRequestBody(c, info)` — 构建请求体
5. `DoRequest(c, info, requestBody)` — 提交任务到上游
6. `DoResponse(c, resp, info)` — 处理提交响应，获取任务 ID
7. `FetchTask(baseUrl, key, body, proxy)` — 轮询任务状态
8. `ParseTaskResult(respBody)` — 解析任务结果
9. `AdjustBillingOnComplete(task, taskResult)` — 最终结算

**计费钩子：**

| 钩子 | 调用时机 | 返回值 |
|------|----------|--------|
| `EstimateBilling` | 验证请求后、价格计算前 | `map[string]float64` 倍率乘数 |
| `AdjustBillingOnSubmit` | 上游提交成功后 | 调整后的倍率（实际参数与估算不同时） |
| `AdjustBillingOnComplete` | 任务完成时 | 实际配额（触发差额结算） |

## API 类型与渠道类型

**API 类型**标识 API 接口规范（如 OpenAI、Claude、Gemini），**渠道类型**标识具体的 AI 服务提供商。多个渠道可能使用同一种 API 类型。

支持的 API 类型（共 35 种）：

| API 类型 | 说明 |
|----------|------|
| `OpenAI` | OpenAI API（GPT 系列） |
| `Anthropic` | Anthropic API（Claude 系列） |
| `Gemini` | Google Gemini API |
| `Aws` | AWS Bedrock API |
| `DeepSeek` | DeepSeek API |
| `VertexAi` | Google Vertex AI API |
| `Codex` | Codex API |
| ... | 其他 28 种 API 类型 |

## 添加新提供商

1. 在 `relay/channel/` 下创建新目录
2. 实现 `Adaptor` 接口的所有方法
3. 在 `constant/api_type.go` 中添加 API 类型常量
4. 在 `relay/relay_adaptor.go` 的 `GetAdaptor()` 中注册映射
5. 在 `constant/channel_type.go` 中添加渠道类型

## 关键文件

| 文件 | 说明 |
|------|------|
| `relay/channel/adapter.go` | 适配器接口定义 |
| `relay/relay_adaptor.go` | 适配器工厂（API 类型 → 适配器映射） |
| `constant/api_type.go` | API 类型常量 |
| `constant/channel_type.go` | 渠道类型常量 |
| `relay/channel/openai/` | OpenAI 适配器实现 |
| `relay/channel/claude/` | Claude 适配器实现 |
| `relay/channel/gemini/` | Gemini 适配器实现 |
