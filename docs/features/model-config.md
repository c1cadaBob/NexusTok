# 模型配置系统

模型配置系统提供分层的模型行为配置，支持每个提供商的独立设置、thinking 适配器、API 格式转换策略和自定义请求头。

## 架构概览

```
全局配置管理器（setting/config/）
  ├── 通用模型设置（model_setting/global.go）
  ├── Claude 设置（model_setting/claude.go）
  ├── Gemini 设置（model_setting/gemini.go）
  ├── Grok 设置（model_setting/grok.go）
  ├── Qwen 设置（model_setting/qwen.go）
  ├── 计费设置（billing_setting/）
  └── 倍率设置（ratio_setting/）
```

## 通用模型设置

### Pass-Through 模式

启用后，请求体直接透传到上游 API，不做格式转换。适用于：
- 上游 API 已经是 OpenAI 兼容格式
- 需要传递系统不识别的自定义字段

### ChatCompletions → Responses API 转换策略

当上游只支持 Responses API 时，系统自动将 Chat Completions 格式转换为 Responses 格式。

配置方式（按优先级）：

| 策略 | 说明 |
|------|------|
| 按渠道 ID | 为特定渠道 ID 启用转换 |
| 按渠道类型 | 为特定渠道类型启用转换 |
| 按模型模式 | 按模型名称模式匹配启用转换 |

## Claude 专用设置

### Thinking 适配器

Claude 的扩展思考（Extended Thinking）功能配置：

| 配置项 | 说明 |
|--------|------|
| `thinking_adapter_enabled` | 是否启用 thinking 适配器 |
| `thinking_budget_tokens_percentage` | 思考预算 token 占比（0-1） |
| `thinking_budget_tokens_min` | 最小思考预算 token 数 |
| `thinking_budget_tokens_max` | 最大思考预算 token 数 |

**工作原理：**
1. 客户端请求中包含 `thinking` 参数时，适配器自动处理
2. 根据 `max_tokens` 和配置的百分比计算思考预算
3. 将预算注入上游请求的 `budget_tokens` 字段

### 自定义请求头

为 Claude 模型配置自定义 HTTP 请求头：

```json
{
  "custom_headers": {
    "anthropic-beta": "prompt-caching-2024-07-31",
    "x-custom-header": "value"
  }
}
```

### 模型名称后缀

处理 thinking 变体的模型名称后缀：
- `claude-3-opus:thinking` → 自动添加 thinking 相关参数
- 支持自定义后缀映射

## Gemini 专用设置

### 安全设置

配置 Gemini 的内容安全过滤级别：

```json
{
  "safety_settings": [
    {"category": "HARM_CATEGORY_HARASSMENT", "threshold": "BLOCK_NONE"},
    {"category": "HARM_CATEGORY_HATE_SPEECH", "threshold": "BLOCK_NONE"}
  ]
}
```

### API 版本映射

支持将模型名称映射到不同的 Gemini API 版本：
- `gemini-pro` → `v1beta`
- `gemini-2.0-flash` → `v1`

### 图像生成配置

Gemini 图像生成的特殊参数配置。

## 倍率设置

### 模型倍率

配置每个模型的价格倍率：

| 倍率类型 | 说明 |
|----------|------|
| `CompletionRatio` | 输出 token 相对于输入 token 的价格倍率 |
| `GroupRatio` | 分组倍率（不同用户组的价格差异） |
| `CacheRatio` | 缓存 token 的价格倍率 |

### 倍率与计费表达式的关系

- 使用**倍率模式**时：`quota = (prompt_tokens * ratio + completion_tokens * completion_ratio * ratio) * group_ratio`
- 使用**计费表达式模式**时：表达式直接定义价格，倍率设置被忽略

## 配置存储

所有模型配置存储在数据库的 `options` 表中，通过 `setting/config/` 的全局配置管理器统一管理。

配置注册流程：
1. 在对应的 `model_setting/` 文件中定义配置键
2. 通过 `config.Register()` 注册到全局管理器
3. 前端通过 `/api/option/` 接口读写配置

## 关键文件

| 文件 | 说明 |
|------|------|
| `setting/model_setting/global.go` | 通用模型设置 |
| `setting/model_setting/claude.go` | Claude 专用设置 |
| `setting/model_setting/gemini.go` | Gemini 专用设置 |
| `setting/model_setting/grok.go` | Grok 专用设置 |
| `setting/model_setting/qwen.go` | Qwen 专用设置 |
| `setting/config/config.go` | 全局配置注册系统 |
| `setting/billing_setting/tiered_billing.go` | 计费模式配置 |
| `setting/ratio_setting/` | 倍率设置 |
