# 账号池系统

账号池管理共享的上游账号凭据，支持多个渠道引用同一池中的账号，实现凭据共享、负载均衡、并发控制、自动刷新和认证文件级维护。

当前项目同时保留两条路径：

- **原生账号池**：由 NexusTok 主服务直接管理 `AccountPoolGroup`、`PoolAccount` 和 `AccountPoolAuthFile`，数据保存在主数据库中，Relay 热路径直接从本地账号池选择账号。
- **CLIProxyAPI / CPA-Manager 兼容路径**：用于继续接入旧的 Sidecar 管理器和请求监控服务，适合迁移过渡或仍依赖外部管理器能力的部署。

新功能优先围绕原生账号池演进。认证文件导入后会生成可调度的 `PoolAccount`，因此渠道只需要绑定账号池分组即可使用导入的官方账号或 API Key。

## 架构概览

```
认证文件导入/手动创建账号
  → 保存 AccountPoolAuthFile（加密原文、文件级分组/代理/优先级）
  → 生成或更新 PoolAccount（加密凭据、调度字段）
  → 归属 AccountPoolGroup
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

`Source` 的常见取值：

| 来源 | 说明 |
|------|------|
| `native` | NexusTok 主服务原生维护的分组 |
| `cliproxyapi` | 从 CLIProxyAPI Sidecar 同步或引用的兼容分组 |

### 池账号（PoolAccount）

存储单个账号的凭据和状态：

| 字段 | 说明 |
|------|------|
| `PoolGroupId` | 所属分组 ID |
| `Name` | 账号名称 |
| `Credentials` | 凭据（加密存储） |
| `Status` | 状态（启用/禁用/冷却中） |
| `Schedulable` | 是否参与调度 |
| `Priority` | 优先级，数值越大越靠前 |
| `Weight` | 加权轮询时的权重 |
| `MaxConcurrency` | 最大并发数，`0` 表示不限制 |
| `Models` | 账号支持的模型列表 |
| `Group` | 账号级调用分组，逗号分隔 |
| `Proxy` | 账号级代理地址 |
| `BaseURL` | 账号级 API 基础地址覆盖 |
| `UsedQuota` | 已使用配额 |
| `SuccessCount` | 成功计数 |
| `FailedCount` | 失败计数 |
| `LastUsedTime` | 最后使用时间 |
| `RateLimitedUntil` | 限流冷却截止时间 |
| `OverloadUntil` | 上游过载冷却截止时间 |
| `TempDisabledUntil` | 临时禁用截止时间 |
| `NextRetryTime` | 下次重试时间 |

`PoolAccount` 是 Relay 调用时真正使用的热路径实体。无论账号来自手动录入、OAuth 登录还是认证文件导入，最终都会以 `PoolAccount` 的形式参与选择、并发预留、失败冷却和用量统计。

### 认证文件（AccountPoolAuthFile）

认证文件把导入的 JSON 凭据提升为一等管理对象，便于按“文件”维护官方账号、API Key、调用分组和代理配置。

| 字段 | 说明 |
|------|------|
| `Name` | 认证文件显示名称 |
| `SourcePlatform` | 来源平台，例如 `sub2`、`newapi`、`native` |
| `Format` | 解析格式，例如 `native`、`sub2`、`newapi` |
| `Provider` | 凭据提供方，例如 `codex`、`xai` |
| `Platform` | 本地调度平台，通常与 `Provider` 一致 |
| `AuthType` | 认证类型 |
| `PoolGroupId` | 关联的账号池分组 |
| `PoolAccountId` | 由该认证文件生成的池账号 |
| `FileDigest` | 原始 JSON 内容的 SHA256，用于去重 |
| `EncryptedContent` | 加密后的认证文件原文，不会在接口响应中返回 |
| `CredentialSummary` | 脱敏摘要，用于列表展示 |
| `CredentialMetadata` | 解析出的凭据元数据 JSON |
| `CredentialAttrs` | 解析出的属性 JSON |
| `AccountGroups` | 文件级调用分组，逗号分隔 |
| `Models` | 文件级模型限制 |
| `Proxy` | 文件级代理 |
| `BaseURL` | 文件级基础地址覆盖 |
| `Priority` | 文件级优先级，会同步到关联账号 |
| `Weight` | 文件级权重，会同步到关联账号 |

导入认证文件时，系统会同时保存两份加密数据：

- 认证文件原文加密保存到 `AccountPoolAuthFile.EncryptedContent`，用于文件级追溯和后续编辑。
- 解析后的规范化凭据 JSON 加密保存到关联 `PoolAccount.Credentials`，用于 Relay 调用。

列表、详情和变更接口只返回脱敏摘要及元数据，不返回加密原文。

## 认证类型

| 类型 | 说明 | 凭据格式 |
|------|------|----------|
| `api_key` | API Key 认证 | API Key 字符串 |
| `official_oauth` | 官方 OAuth 认证 | OAuth Token JSON |
| `cookie` | Cookie 认证 | Cookie 字符串 |
| `service_account` | 服务账号认证 | 服务账号 JSON |
| `custom_json` | 自定义 JSON 认证 | 任意 JSON |

Codex 认证文件会自动识别并归一化 `access_token`、`refresh_token`、`id_token`、`email`、`account_id`、`last_refresh` 和 `expired` 等字段。如果只导入 `access_token`，系统会把凭据标记为 `access_token_only`，该账号可以参与调用，但不能依赖刷新任务自动续期。

`grok`、`x.ai`、`x-ai` 会统一归一化为 `xai`；`codex-cli`、`openai-codex`、`chatgpt` 会统一归一化为 `codex`。

## 认证文件格式

原生认证文件导入接口接受单个 JSON 对象。支持三类常见形态：

### 原生凭据对象

```json
{
  "provider": "codex",
  "access_token": "eyJ...",
  "refresh_token": "...",
  "email": "user@example.com",
  "account_groups": ["codex-prod", "team-a"],
  "models": ["gpt-5-codex"],
  "proxy_url": "http://user:pass@proxy.example.com:8080",
  "priority": 10,
  "weight": 2
}
```

### API Key 凭据对象

```json
{
  "provider": "xai",
  "api_key": "xai-...",
  "models": "grok-4,grok-4-fast",
  "group": "xai-prod"
}
```

### 包装格式

兼容 `sub2`、`newapi` 等来源常见的包装字段。系统会从 `auth`、`auth_file`、`authFile`、`credential`、`credentials`、`data`、`account`、`token`、`token_data`、`tokenData` 中选择最像凭据的对象。

```json
{
  "source_platform": "sub2",
  "auth": {
    "type": "codex",
    "token_data": {
      "access_token": "eyJ...",
      "refresh_token": "...",
      "id_token": "eyJ..."
    }
  },
  "account_groups": ["codex-prod"],
  "proxy": "socks5://127.0.0.1:1080"
}
```

```json
{
  "format": "newapi",
  "credential": {
    "platform": "xai",
    "api_key": "xai-..."
  }
}
```

当前原生接口面向“单个认证对象”导入。Sub2api 的 `sub2api-data` / `sub2api-bundle` 这类包含 `accounts[]` 和 `proxies[]` 的批量导出包，后续应通过批量导入能力统一映射；在该能力完成前，可以先把每个账号拆成单个认证对象导入。

## 认证文件 API

以下接口均需要管理员登录态，路径前缀为 `/api/account-pool`。

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/auth-files` | 分页查询认证文件 |
| `POST` | `/auth-files` | 导入认证文件，并生成关联池账号 |
| `GET` | `/auth-files/:auth_file_id` | 获取单个认证文件摘要 |
| `PUT` | `/auth-files/:auth_file_id` | 更新认证文件或文件级调度字段 |
| `DELETE` | `/auth-files/:auth_file_id` | 删除认证文件，默认同时删除关联池账号 |

列表查询参数：

| 参数 | 说明 |
|------|------|
| `p` | 页码，响应体中返回为 `page` |
| `page_size` / `ps` / `size` | 每页数量 |
| `status` | 按状态筛选 |
| `pool_group_id` | 按账号池分组筛选 |
| `provider` | 按 provider 筛选 |
| `search` | 按名称、provider、platform、摘要等字段搜索 |

导入请求示例：

```json
{
  "name": "Codex production account",
  "content": "{\"provider\":\"codex\",\"access_token\":\"eyJ...\"}",
  "group_name": "Codex 官方账号",
  "account_groups": ["codex-prod"],
  "models": "gpt-5-codex",
  "proxy": "http://user:pass@proxy.example.com:8080",
  "priority": 10,
  "weight": 2,
  "status": 1
}
```

可覆盖字段：

| 字段 | 说明 |
|------|------|
| `name` | 覆盖认证文件名称 |
| `content` | JSON 认证文件原文，创建时必填，更新时可选 |
| `pool_group_id` | 指定已有账号池分组 |
| `group_name` | 未指定分组时自动创建或复用的分组名 |
| `provider` | 覆盖解析出的 provider |
| `platform` | 覆盖本地调度平台 |
| `auth_type` | 覆盖认证类型 |
| `account_group` | 单个调用分组 |
| `account_groups` | 多个调用分组 |
| `models` | 模型限制 |
| `proxy` | 代理地址 |
| `base_url` | API 基础地址覆盖 |
| `priority` | 优先级 |
| `weight` | 权重 |
| `status` | 状态 |

更新接口中 `content` 为空时，只修改文件级字段，并同步到关联的 `PoolAccount`；`content` 非空时，会重新解析凭据、替换加密原文和账号凭据，并重置相关运行时错误字段。

删除接口默认同时删除关联池账号：

```bash
DELETE /api/account-pool/auth-files/123
```

如果只删除认证文件记录并保留池账号：

```bash
DELETE /api/account-pool/auth-files/123?delete_account=false
```

## 调度策略

| 策略 | 说明 | 适用场景 |
|------|------|----------|
| `round_robin` | 轮询调度 | 通用场景，均匀分配 |
| `weighted` | 加权调度 | 按账号优先级/配额分配 |
| `fill_first` | 优先填满 | 优先用完一个账号再用下一个 |
| `least_used` | 最少使用 | 优先使用请求数最少的账号 |

选择账号时会先过滤不可用账号：

- 分组状态必须启用；
- 账号状态必须启用且 `Schedulable=true`；
- 账号不能处于限流、过载、临时禁用或重试等待窗口；
- 请求模型必须匹配分组或账号的 `Models` 限制；
- 请求渠道分组必须匹配分组或账号的 `Group` / `AccountGroups` 限制；
- 当前请求中已经失败的账号会被排除，避免同一请求反复命中同一个坏账号。

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

## 渠道接入

渠道可以通过全局账号池模式引用账号池分组：

1. 在账号池中创建或导入账号，确认分组下存在可调度账号。
2. 在渠道配置中启用账号池模式，并选择 `AccountPoolGroupId`。
3. Relay 请求进入渠道后，系统从该分组选择 `PoolAccount`。
4. 账号凭据会被转换为现有 provider adaptor 可识别的 channel key 或 OAuth 凭据。
5. 调用完成后释放并发槽位，并在结算阶段记录账号池维度用量。

如果渠道引用的是 `cliproxyapi` 来源分组，`global_account_pool` 兼容模式会把请求转发到 CLIProxyAPI Sidecar，并通过内部头指定外部分组。原生 `native` 分组不依赖 Sidecar。

## 使用流程

### 原生认证文件导入

1. 准备单个 JSON 认证文件，可以是原生凭据对象，也可以是 `sub2` / `newapi` 包装对象。
2. 调用 `POST /api/account-pool/auth-files`，可指定 `pool_group_id`，也可传 `group_name` 让系统自动创建或复用分组。
3. 系统保存加密原文，生成关联 `PoolAccount`。
4. 在渠道配置中引用该账号池分组。
5. 请求到达时，系统自动从池中选择账号。
6. 请求成功后更新使用统计；请求失败时账号进入冷却或标记异常。

### 手动维护账号

1. 创建账号池分组，配置平台、认证类型、调度策略。
2. 添加池账号，填入凭据、模型限制、调用分组、代理和并发限制。
3. 在渠道配置中引用账号池分组。
4. 后续可按账号刷新凭据、重置运行时状态、调整状态或删除账号。

### 兼容旧 Sidecar

1. 保持 `ACCOUNT_POOL_CLI_PROXY_URL`、`ACCOUNT_POOL_CLI_PROXY_MANAGEMENT_KEY` 和 `ACCOUNT_POOL_CLI_PROXY_RELAY_KEY` 配置可用。
2. 通过 CPA-Manager 或同步任务维护 CLIProxyAPI 分组。
3. NexusTok 中引用 `cliproxyapi` 来源分组。
4. Relay 通过 Sidecar 完成实际账号选择和请求转发。

该路径用于兼容历史部署。新账号建议优先落到原生账号池，便于后续统一管理、审计和调度。

## 安全与迁移注意事项

- 认证文件原文和账号凭据均加密保存，接口响应只返回脱敏摘要。
- `FileDigest` 用原始 JSON 内容计算，用于阻止重复导入完全相同的认证文件。
- 编辑认证文件级 `account_groups`、`models`、`proxy`、`base_url`、`priority`、`weight`、`status` 会同步到关联池账号。
- 认证文件删除默认会删除关联池账号，避免残留可调度凭据；需要保留账号时显式传 `delete_account=false`。
- 从外部账号管理器迁移时，优先确认 provider、认证类型、代理和分组语义是否能一一映射。批量导出包如包含多个账号，应拆分或等待批量导入能力。
- 单容器部署不启动 CLIProxyAPI Sidecar，但原生 `native` 账号池和认证文件 API 仍由主服务提供。只有继续使用 `cliproxyapi` 兼容路径时才需要 Sidecar。

## 关键文件

| 文件 | 说明 |
|------|------|
| `model/account_pool.go` | 数据模型定义 |
| `service/account_pool_auth_file.go` | 原生认证文件解析、导入、更新和关联账号同步 |
| `service/account_pool_select.go` | 账号选择与负载均衡 |
| `service/account_pool_refresh_task.go` | 凭据刷新任务 |
| `service/account_pool_quota.go` | 配额管理 |
| `service/account_pool_cliproxy.go` | CLIProxyAPI Sidecar 兼容集成 |
| `service/accountauth/` | 认证提供者实现 |
| `service/codex_oauth.go` | Codex OAuth 2.0 流程 |
| `service/codex_credential_refresh.go` | 凭据刷新逻辑 |
| `controller/account_pool.go` | 账号池管理 API |
| `controller/account_pool_proxy.go` | 账号池 Relay 入口 |
| `router/api-router.go` | `/api/account-pool` 路由注册 |
