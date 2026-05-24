# 账号池系统

账号池管理共享的上游账号凭据，支持多个渠道引用同一池中的账号，实现凭据共享、负载均衡和自动刷新。

## 架构概览

```
渠道配置 → 引用账号池分组
  → 调度策略选择账号
  → 并发控制（信号量）
  → 使用账号凭据调用上游
  → 更新使用统计
  → 失败时进入冷却
  → 定时刷新 OAuth 凭据
```

## 核心概念

### 账号池分组（AccountPoolGroup）

定义一组账号的公共配置：

| 字段 | 说明 |
|------|------|
| `Name` | 分组名称 |
| `Platform` | 平台标识（openai、claude、codex 等） |
| `AuthType` | 认证类型 |
| `Strategy` | 调度策略 |
| `Models` | 支持的模型列表（逗号分隔） |
| `Group` | 关联的渠道分组 |
| `ModelMapping` | 模型映射（JSON） |

### 池账号（PoolAccount）

存储单个账号的凭据和状态：

| 字段 | 说明 |
|------|------|
| `PoolGroupId` | 所属分组 ID |
| `Name` | 账号名称 |
| `Credential` | 凭据（加密存储） |
| `Status` | 状态（启用/禁用/冷却中） |
| `Priority` | 优先级（用于加权调度） |
| `QuotaUsed` | 已使用配额 |
| `RequestCount` | 请求计数 |
| `SuccessCount` | 成功计数 |
| `FailCount` | 失败计数 |
| `LastUsedTime` | 最后使用时间 |
| `CooldownUntil` | 冷却截止时间 |

## 认证类型

| 类型 | 说明 | 凭据格式 |
|------|------|----------|
| `api_key` | API Key 认证 | API Key 字符串 |
| `official_oauth` | 官方 OAuth 认证 | OAuth Token JSON |
| `cookie` | Cookie 认证 | Cookie 字符串 |
| `service_account` | 服务账号认证 | 服务账号 JSON |
| `custom_json` | 自定义 JSON 认证 | 任意 JSON |

## 调度策略

| 策略 | 说明 | 适用场景 |
|------|------|----------|
| `round_robin` | 轮询调度 | 通用场景，均匀分配 |
| `weighted` | 加权调度 | 按账号优先级/配额分配 |
| `fill_first` | 优先填满 | 优先用完一个账号再用下一个 |
| `least_used` | 最少使用 | 优先使用请求数最少的账号 |

## 并发控制

使用 Redis/内存信号量限制每个账号的并发请求数：

- 每个账号可配置最大并发数
- 超出限制时自动选择下一个可用账号
- 支持 Redis 分布式信号量（多实例部署）和内存信号量（单实例）

## 冷却机制

账号请求失败后自动进入冷却状态：

- 冷却时间可配置（默认 60 秒）
- 冷却期间不参与调度
- 冷却到期后自动恢复

## 凭据刷新

支持 OAuth 类型账号的自动凭据刷新：

### Codex OAuth 流程

1. **PKCE 流程** — 适用于交互式授权
2. **Device Code 流程** — 适用于无头环境

刷新任务定期运行：
- 检查 Token 过期时间
- 在过期前自动刷新
- 刷新失败时标记账号为异常

## 使用流程

1. 创建账号池分组，配置平台、认证类型、调度策略
2. 添加池账号，填入凭据
3. 在渠道配置中引用账号池分组
4. 请求到达时，系统自动从池中选择账号
5. 请求成功后更新使用统计
6. 请求失败时账号进入冷却

## 关键文件

| 文件 | 说明 |
|------|------|
| `model/account_pool.go` | 数据模型定义 |
| `service/account_pool_select.go` | 账号选择与负载均衡 |
| `service/account_pool_refresh_task.go` | 凭据刷新任务 |
| `service/account_pool_quota.go` | 配额管理 |
| `service/accountauth/` | 认证提供者实现 |
| `service/codex_oauth.go` | Codex OAuth 2.0 流程 |
| `service/codex_credential_refresh.go` | 凭据刷新逻辑 |
