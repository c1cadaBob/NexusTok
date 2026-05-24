# 渠道选择与亲和性路由

渠道选择系统决定每个入站请求由哪个上游渠道处理。支持随机选择、优先级重试、亲和性粘滞路由和跨分组自动重试。

## 架构概览

```
请求到达
  → 分发中间件解析模型名称
  → 验证 Token 访问权限
  → 渠道选择（亲和性 > 特定渠道 > 随机选择）
  → 渠道配置写入上下文
  → Relay 处理请求
  → 请求成功后更新亲和性缓存
```

## 渠道选择策略

### 1. 特定渠道（管理员专用）

请求中指定 `specific_channel_id` 时，直接使用该渠道。用于调试和问题排查。

### 2. 渠道亲和性（Channel Affinity）

将特定请求路由到固定的渠道，利用上游的缓存机制（如 prompt cache）降低延迟和成本。

**核心机制：**

1. **规则匹配** — 根据模型正则、路径正则、用户代理等条件匹配请求
2. **亲和值提取** — 从请求上下文或请求体中提取亲和值（如 `prompt_cache_key`）
3. **缓存查找** — 在 Redis/内存混合缓存中查找之前绑定的渠道 ID
4. **绑定记录** — 请求成功后将渠道 ID 缓存，后续相同亲和值的请求路由到同一渠道
5. **模板覆盖** — 支持为匹配规则的请求附加参数覆盖模板

**亲和值来源：**

| 来源类型 | 说明 | 示例 |
|----------|------|------|
| `gjson` | 从请求体中按 gjson 路径提取 | `prompt_cache_key`、`user.id` |
| `context_int` | 从 Gin 上下文中提取整数值 | 渠道 ID、用户 ID |
| `context_string` | 从 Gin 上下文中提取字符串值 | 用户名、会话 ID |

**缓存策略：**
- 使用 `cachex.HybridCache`（Redis + 内存 LRU）
- 可配置 TTL（秒）
- 缓存键格式：`{namespace}:{rule_name}:{fingerprint}`

### 3. 随机选择（默认）

通过 `CacheGetRandomSatisfiedChannel` 从满足条件的渠道中随机选择：
- 按渠道优先级排序
- 支持权重随机
- 失败后自动重试下一个渠道

### 4. 跨分组自动重试（Auto Group）

当所有当前分组的渠道都失败时，自动尝试其他分组中的渠道：
- 仅在 Token 配置了 `auto` 分组时生效
- 按分组优先级依次尝试
- 支持配置最大重试次数

## 配置示例

### 亲和性规则配置

```json
{
  "rules": [
    {
      "name": "claude-prompt-cache",
      "enabled": true,
      "model_regex": "^claude-3",
      "path_regex": "/v1/messages",
      "key_source_type": "gjson",
      "key_source_path": "prompt_cache_key",
      "ttl_seconds": 3600,
      "skip_retry_on_failure": true,
      "param_override_template": {
        "anthropic-beta": "prompt-caching-2024-07-31"
      }
    }
  ]
}
```

### 配置项说明

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 规则名称（唯一标识） |
| `enabled` | bool | 是否启用 |
| `model_regex` | string | 模型名称正则匹配 |
| `path_regex` | string | 请求路径正则匹配 |
| `user_agent_regex` | string | User-Agent 正则匹配 |
| `key_source_type` | string | 亲和值来源类型 |
| `key_source_path` | string | 亲和值提取路径（gjson 路径） |
| `ttl_seconds` | int | 缓存过期时间（秒） |
| `skip_retry_on_failure` | bool | 失败时是否跳过重试 |
| `param_override_template` | map | 参数覆盖模板 |

## 使用场景

### Prompt Cache 优化

Claude、Gemini 等提供商支持 prompt cache，相同前缀的请求可以复用缓存。通过亲和性路由，确保相同 `prompt_cache_key` 的请求发送到同一渠道，最大化缓存命中率。

### 会话粘滞

对于需要上下文连续性的场景（如多轮对话），可以按用户 ID 或会话 ID 设置亲和性，确保同一用户的请求路由到同一渠道。

## 关键文件

| 文件 | 说明 |
|------|------|
| `middleware/distributor.go` | 分发中间件（入口） |
| `service/channel_select.go` | 渠道选择逻辑 |
| `service/channel_affinity.go` | 亲和性路由实现 |
| `setting/operation_setting/channel_affinity_setting.go` | 亲和性规则配置 |
| `service/auto_group.go` | 跨分组自动重试 |
| `pkg/cachex/hybrid_cache.go` | 混合缓存（亲和性存储后端） |
