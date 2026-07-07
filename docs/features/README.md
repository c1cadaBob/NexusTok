# 核心功能文档

本目录包含 NexusTok 核心功能的详细技术文档。

## 功能列表

| 文档 | 说明 |
|------|------|
| [provider-adaptor.md](provider-adaptor.md) | 渠道适配器系统 — 40+ 上游 AI 提供商的请求/响应转换 |
| [billing-expression.md](billing-expression.md) | 计费表达式系统 — 用表达式定义模型计费逻辑 |
| [channel-routing.md](channel-routing.md) | 渠道选择与亲和性路由 — 请求分发、粘滞路由、跨分组重试 |
| [account-pool.md](account-pool.md) | 账号池系统 — 原生认证文件、共享凭据管理、负载均衡、自动刷新 |
| [streaming-sse.md](streaming-sse.md) | 流式响应与 SSE 处理 — StreamScanner、WebSocket、格式转换 |
| [model-config.md](model-config.md) | 模型配置系统 — 分提供商设置、Thinking 适配器、API 转换策略 |
| [model-sync-pricing.md](model-sync-pricing.md) | 模型同步与价格同步 — 上游模型目录、models.dev provider 价格、手动价格保护 |
| [auth-rate-limit.md](auth-rate-limit.md) | 认证与限流中间件 — 多方式认证、模型级限流、安全验证 |
| [hybrid-cache.md](hybrid-cache.md) | 混合缓存系统 — Redis + 内存 LRU 双层缓存 |
| [new-api-main-diff-analysis.md](new-api-main-diff-analysis.md) | new-api-main 对比分析 — 功能、页面、基础设施差异与 NexusTok 原生化路线 |

## 阅读建议

- **新开发者**：从 [provider-adaptor.md](provider-adaptor.md) 开始，了解系统核心架构
- **运维人员**：重点关注 [channel-routing.md](channel-routing.md)、[account-pool.md](account-pool.md)、[auth-rate-limit.md](auth-rate-limit.md)、[model-config.md](model-config.md)、[model-sync-pricing.md](model-sync-pricing.md)
- **计费相关**：阅读 [billing-expression.md](billing-expression.md) 了解表达式语法和计费流程，并参考 [model-sync-pricing.md](model-sync-pricing.md) 了解模型级价格同步
- **性能优化**：参考 [hybrid-cache.md](hybrid-cache.md) 和 [streaming-sse.md](streaming-sse.md)
