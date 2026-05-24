# 认证与限流中间件

中间件链处理每个 API 请求，包括多方式认证、模型级限流、CORS 处理和安全验证。中间件顺序对系统正确运行至关重要。

## 中间件链顺序

```
请求到达
  → CORS 处理
  → i18n 语言检测
  → Session 恢复
  → 认证（Auth）
  → 渠道分发（Distribute）
  → 模型级限流（ModelRateLimit）
  → Relay 处理
  → 响应返回
```

## 认证方式

系统支持三种认证方式，按请求类型自动选择：

### 1. Session 认证（Cookie）

用于 Web 管理前端。

- 基于 Cookie 的会话认证
- 支持配置会话密钥（`SESSION_SECRET`）
- 支持配置会话最大存活时间（`SESSION_MAX_AGE`）

### 2. Access Token 认证（Header）

用于 API 调用。

- 通过 `Authorization` 头传递
- 格式：`Authorization: Bearer <access_token>`
- 支持旧版头名称兼容（`New-Api-User`）

### 3. API Token 认证（Bearer Token）

用于中继 API 调用。

- 通过 `Authorization: Bearer <api_token>` 传递
- 支持多种 Token 类型：
  - 普通 Token（绑定用户和模型权限）
  - 临时 Token（用于特定场景）
  - Root Token（管理员权限）

### 认证流程

1. 检查请求头中的 `Authorization`
2. 如果是 `Bearer <token>`，解析 Token
3. 验证 Token 有效性和权限
4. 检查 Token 对请求模型的访问权限
5. 将用户信息和 Token 配置写入上下文

## 模型级限流

基于令牌桶和滑动窗口算法的限流系统。

### 算法

| 算法 | 用途 | 后端 |
|------|------|------|
| 令牌桶（Token Bucket） | 限制总请求数 | Redis / 内存 |
| 滑动窗口（Sliding Window） | 限制成功请求数 | Redis / 内存 |

### 配置

每个用户可独立配置：

| 配置项 | 说明 |
|--------|------|
| `RequestRateLimit` | 总请求速率限制（次/分钟） |
| `SuccessRateLimit` | 成功请求速率限制（次/分钟） |
| `RateLimitKey` | 限流键（默认按用户 ID） |

### 限流维度

- **全局限流** — 限制所有请求的总速率
- **模型级限流** — 限制对特定模型的请求速率
- **用户级限流** — 每个用户独立的限流计数

## CORS 处理

跨域资源共享配置：

- 支持配置允许的来源域名
- 支持配置允许的 HTTP 方法
- 支持配置允许的请求头
- 预检请求（OPTIONS）自动响应

## Cloudflare Turnstile 验证

支持 Cloudflare Turnstile 人机验证：

- 用于保护公开部署的管理接口
- 配置 Turnstile Site Key 和 Secret Key
- 验证失败时拒绝请求

## 安全验证中间件

额外的安全检查：

- IP 白名单/黑名单
- 请求频率异常检测
- 可疑请求标记

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `SESSION_SECRET` | - | 会话密钥（多机部署必须） |
| `SESSION_MAX_AGE` | `7776000` | 会话最大存活时间（秒，默认 90 天） |
| `CRYPTO_SECRET` | - | 加密密钥（Redis 必须） |

## 关键文件

| 文件 | 说明 |
|------|------|
| `middleware/auth.go` | 多方式认证 |
| `middleware/model-rate-limit.go` | 模型级限流 |
| `middleware/rate-limit.go` | 全局限流 |
| `middleware/distributor.go` | 渠道分发 |
| `middleware/cors.go` | CORS 处理 |
| `middleware/turnstile-check.go` | Turnstile 验证 |
| `middleware/secure_verification.go` | 安全验证 |
| `middleware/i18n.go` | 国际化中间件 |
