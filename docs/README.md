# NexusTok 文档目录

本目录汇集了项目的所有文档资源，按类别组织。

## 目录结构

```
docs/
├── README.md                      ← 本文件
├── configuration/                 ← 配置相关文档
├── features/                      ← 核心功能技术文档
├── i18n/                          ← 国际化与翻译
├── images/                        ← 文档图片资源
├── installation/                  ← 安装部署指南
├── openapi/                       ← OpenAPI 规范文件
└── sdk/                           ← CLIProxyAPI SDK 文档
```

## 文档索引

### 项目概览

| 文档 | 说明 |
|------|------|
| [README.md](../README.md) | 项目说明、快速开始、部署指南 |
| [THIRD-PARTY-LICENSES.md](../THIRD-PARTY-LICENSES.md) | 第三方依赖许可证 |

### 开发指南

| 文档 | 说明 |
|------|------|
| [CLAUDE.md](../CLAUDE.md) | Claude Code 项目规范与编码约定 |
| [AGENTS.md](../AGENTS.md) | Claude Agent SDK 配置 |
| [web/default/AGENTS.md](../web/default/AGENTS.md) | 前端代理配置 |

### SDK 文档（CLIProxyAPI）

| 文档 | 说明 |
|------|------|
| [sdk-access.md](sdk/sdk-access.md) / [中文](sdk/sdk-access_CN.md) | 入站访问认证 SDK |
| [sdk-usage.md](sdk/sdk-usage.md) / [中文](sdk/sdk-usage_CN.md) | SDK 嵌入使用指南 |
| [sdk-advanced.md](sdk/sdk-advanced.md) / [中文](sdk/sdk-advanced_CN.md) | 执行器与翻译器扩展 |
| [sdk-watcher.md](sdk/sdk-watcher.md) / [中文](sdk/sdk-watcher_CN.md) | Watcher 集成说明 |

### 安装部署

| 文档 | 说明 |
|------|------|
| [installation/BT.md](installation/BT.md) | 宝塔面板安装教程 |

### 配置说明

| 文档 | 说明 |
|------|------|
| [configuration/channel-other-settings.md](configuration/channel-other-settings.md) | 渠道额外设置（force_format、proxy、thinking_to_content） |

### 国际化

| 文档 | 说明 |
|------|------|
| [i18n/translation-glossary.md](i18n/translation-glossary.md) | 翻译术语表（英文基准） |
| [i18n/translation-glossary.fr.md](i18n/translation-glossary.fr.md) | 翻译术语表（法语） |
| [i18n/translation-glossary.ru.md](i18n/translation-glossary.ru.md) | 翻译术语表（俄语） |

### 模块文档

| 文档 | 说明 |
|------|------|
| [modules/cpa-manager/README.md](../modules/cpa-manager/README.md) / [中文](../modules/cpa-manager/README_CN.md) | CPA-Manager 管理面板文档 |
| [modules/MANIFEST.md](../modules/MANIFEST.md) | 模块清单 |
| [modules/cliproxyapi/CLAUDE.md](../modules/cliproxyapi/CLAUDE.md) | CLIProxyAPI 子模块规范 |

### 架构文档

| 文档 | 说明 |
|------|------|
| [pkg/billingexpr/expr.md](../pkg/billingexpr/expr.md) | 计费表达式系统设计 |

### 核心功能

| 文档 | 说明 |
|------|------|
| [features/provider-adaptor.md](features/provider-adaptor.md) | 渠道适配器系统 — 40+ 上游提供商的请求/响应转换 |
| [features/billing-expression.md](features/billing-expression.md) | 计费表达式系统 — 表达式定义模型计费逻辑 |
| [features/channel-routing.md](features/channel-routing.md) | 渠道选择与亲和性路由 — 分发、粘滞路由、跨分组重试 |
| [features/account-pool.md](features/account-pool.md) | 账号池系统 — 共享凭据管理、负载均衡、自动刷新 |
| [features/streaming-sse.md](features/streaming-sse.md) | 流式响应与 SSE 处理 — StreamScanner、WebSocket |
| [features/model-config.md](features/model-config.md) | 模型配置系统 — 分提供商设置、Thinking 适配器 |
| [features/auth-rate-limit.md](features/auth-rate-limit.md) | 认证与限流中间件 — 多方式认证、模型级限流 |
| [features/hybrid-cache.md](features/hybrid-cache.md) | 混合缓存系统 — Redis + 内存 LRU 双层缓存 |

### API 规范

| 文档 | 说明 |
|------|------|
| [openapi/api.json](openapi/api.json) | API OpenAPI 规范 |
| [openapi/relay.json](openapi/relay.json) | Relay API OpenAPI 规范 |

### 使用指南

| 文档 | 说明 |
|------|------|
| [ionet-client.md](ionet-client.md) | io.net 客户端集成 |

### 社区规范

| 文档 | 说明 |
|------|------|
| [.github/CODE_OF_CONDUCT.md](../.github/CODE_OF_CONDUCT.md) / [中文](../.github/CODE_OF_CONDUCT_CN.md) | 贡献者行为准则 |
| [.github/SECURITY.md](../.github/SECURITY.md) / [中文](../.github/SECURITY_CN.md) | 安全政策 |
| [.github/PULL_REQUEST_TEMPLATE.md](../.github/PULL_REQUEST_TEMPLATE.md) | PR 模板 |
| [.github/ISSUE_TEMPLATE/](../.github/ISSUE_TEMPLATE/) | Issue 模板（中英文） |

### Agent 技能定义

| 文档 | 说明 |
|------|------|
| [.agents/skills/classic-to-default-sync/SKILL.md](../.agents/skills/classic-to-default-sync/SKILL.md) | 经典前端同步技能 |
| [.agents/skills/i18n-translate/SKILL.md](../.agents/skills/i18n-translate/SKILL.md) | i18n 翻译技能 |
| [.agents/skills/shadcn-ui/SKILL.md](../.agents/skills/shadcn-ui/SKILL.md) | shadcn/ui 技能 |
