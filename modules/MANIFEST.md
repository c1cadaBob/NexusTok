# 外部账号池模块迁移清单

## 来源

- `modules/cliproxyapi/` 来源：`/opt/project/CLIProxyAPI`
- `modules/cpa-manager/` 来源：`/opt/project/CPA-Manager`
- 复制日期：2026-05-21

## 用途

这些目录用于把 CLIProxyAPI 后端服务和 CPA-Manager 管理页面作为 NexusTok 的独立账号池管理模块集成。

## 边界

- 保留原项目代码结构和文件层级。
- 不直接把 CLIProxyAPI import 到 NexusTok 主 Go module，避免 module path、Go 版本和依赖冲突。
- 不提交 `.git`、`node_modules`、`dist`、缓存和临时运行产物。
- NexusTok 只负责统一登录态、管理员权限校验、同源静态托管和管理 API 代理。
