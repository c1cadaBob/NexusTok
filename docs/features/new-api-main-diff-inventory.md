# NexusTok 与 new-api-main 全量文件差异索引

本文是 `new-api-main-diff-analysis.md` 的可复核补充，用来回答“是否覆盖所有差异”。主文档按功能和页面解释差异，本文记录文件级差异的生成方式、统计口径和分类规则。因为两个仓库同路径但内容不同的文件超过千个，本文不直接内嵌所有路径，而是提供可重复生成的清单。

## 统计口径

对比目录：

| 项目 | 路径 |
|------|------|
| NexusTok | `/opt/project/NexusTok` |
| new-api-main | `/opt/project/new-api-main` |

忽略范围：

| 模式 | 原因 |
|------|------|
| `node_modules` | 依赖目录，不代表项目功能。 |
| `dist`、`build` | 构建产物，可由源码再生成。 |
| `tmp`、`logs`、`*.log` | 临时文件和运行日志。 |

当前快照统计（2026-07-16 重新执行 `./scripts/compare-new-api-main.sh`）：

| 差异类别 | 文件数 | 说明 |
|----------|--------|------|
| NexusTok 独有 | 1244 | 主要来自内嵌 `modules/`、账号池、渠道账号、文档、默认前端账号池/计费页、热更新/部署脚本，以及 NexusTok 原生 SystemInfo/SystemTask、Authz、计费与中继增强相关新增文件。 |
| new-api-main 独有 | 114 | 主要集中在 `web/` 内部的 DataTable 分层、RichContent、Playground 重构残余文件，以及少量 `service/`、`relay/`、`docs/`、`electron/` 等仍未被 NexusTok 原生吸收的资产。 |
| 同路径但内容不同 | 1988 | 主要来自默认前端、classic 前端、Relay、controller、service、model、common、setting 等共享模块的实现差异；这部分仍是后续原生化的主要工作面。 |

## 重新生成清单

在 NexusTok 根目录执行：

```bash
./scripts/compare-new-api-main.sh
```

默认输出到：

```text
tmp/new-api-main-diff/
```

本次 2026-07-16 快照已归档到：

```text
docs/features/new-api-main-diff-files/
```

生成文件：

| 文件 | 内容 |
|------|------|
| `nexustok-files.txt` | NexusTok 参与对比的全部文件。 |
| `new-api-main-files.txt` | new-api-main 参与对比的全部文件。 |
| `nexustok-only.txt` | 只在 NexusTok 存在的文件。 |
| `new-api-main-only.txt` | 只在 new-api-main 存在的文件。 |
| `common-files.txt` | 两边同路径都存在的文件。 |
| `common-changed.txt` | 两边同路径都存在但内容不同的文件。 |
| `summary.md` | 三类差异数量和顶层目录分布。 |

如果 new-api-main 路径不同，可以覆盖环境变量：

```bash
NEW_API_ROOT=/path/to/new-api-main ./scripts/compare-new-api-main.sh
```

## 顶层目录分布

### NexusTok 独有文件

| 顶层目录 | 文件数 | 主要含义 |
|----------|--------|----------|
| `modules` | 894 | NexusTok 内嵌账号池/CLIProxy/CPA 管理相关模块，是本项目账号池方向的重要原生资产。 |
| `web` | 150 | 默认前端账号池、计费设置、繁体中文支持、classic 账号池/计费页、表格与交互增强等。 |
| `service` | 60 | 账号池服务、渠道账号服务、Codex OAuth、openai compat、SystemTask runner、Authz 与治理增强等。 |
| `docs` | 27 | NexusTok 自有功能文档、SDK 文档和差异索引。 |
| `controller` | 37 | 账号池、渠道账号、Codex OAuth、OAuth provider、系统任务和治理相关控制器。 |
| `router` | 24 | 渠道、系统信息、系统任务、Authz、订阅支付等路由与测试。 |
| `model` | 19 | 账号池、账号池健康、渠道账号、模型定价配置、任务与计费相关模型测试等。 |
| `common` | 7 | 会话密钥、请求体限制、配额与 SSRF 安全增强等公共工具。 |
| `middleware` | 6 | NexusTok 自有中间件测试、请求体限制测试和分发相关能力。 |
| `relay` | 5 | 渠道协议与适配器扩展文件。 |
| 其它 | 15 | 账号池 sidecar Docker、热更新脚本、差异生成脚本、服务配置等。 |

### new-api-main 独有文件

| 顶层目录 | 文件数 | 主要含义 |
|----------|--------|----------|
| `web` | 84 | DataTable 分层、RichContent、Playground 重构、Waffo Pancake 设置向导、UsageLogs 移动端卡片等残余前端资产。 |
| `service` | 16 | Authz enforcement、SystemTask 其它业务 handlers、RelayConvert、任务轮询测试等。 |
| `relay` | 4 | OpenAI/Gemini/Advanced Custom 相关残余适配器或测试文件。 |
| `docs` | 4 | 多语言 README、Electron 文档之外的差异化文档资产。 |
| `output` | 2 | new-api-main 目录内的额外输出文件。 |
| 其它 | 4 | `new-api.service`、`model`、`electron`、`dto` 等零散独有文件。 |

### 同路径但内容不同

| 顶层目录 | 文件数 | 主要含义 |
|----------|--------|----------|
| `web` | 1329 | 绝大部分页面/组件同路径但实现不同，需要按页面和 feature 逐项吸收。 |
| `relay` | 212 | Relay 能力、请求转换、测试覆盖和上游协议支持差异。 |
| `controller` | 76 | 管理接口、支付、用户、渠道、日志、模型同步等业务差异。 |
| `service` | 66 | 计费、订阅、任务、OAuth、Relay 转换和外部请求安全等实现差异。 |
| `common` | 54 | 公共工具、SSRF、请求体、配额、环境初始化差异。 |
| `model` | 52 | 数据模型、日志、订阅、用户、渠道、任务等差异。 |
| `setting` | 50 | 系统设置、计费设置、支付设置、模型设置差异。 |
| `dto` | 29 | 请求/响应 DTO 差异，迁移时必须遵守显式零值规则。 |
| `middleware` | 25 | 鉴权、限流、日志、请求处理等中间件差异。 |
| `pkg` | 19 | 计费表达式、缓存、内部包实现差异。 |
| 其它 | 76 | 路由、i18n、electron、构建和仓库元数据差异。 |

## 重点目录精细统计

| 目录 | NexusTok 独有 | new-api-main 独有 | 同路径不同 | 处理策略 |
|------|---------------|-------------------|------------|----------|
| `router` | 24 | 0 | 9 | 渠道、系统信息、系统任务、Authz、订阅支付等路由与测试已大量原生化；后续继续围绕资源粒度和 Casbin runtime 收口。 |
| `controller` | 37 | 0 | 76 | 账号池、Codex OAuth、SystemTask、Authz 和治理类控制器已经明显扩展；剩余差异主要是同路径实现与边界细化。 |
| `service` | 60 | 16 | 66 | 账号池、Codex OAuth、Authz、SystemTask、ProtectedFetch 与 relay compat 已形成大量原生文件；仍需继续吸收 new-api-main 在细分治理和测试上的优势。 |
| `model` | 19 | 1 | 52 | SystemInstance、SystemTask、流量账本、订阅支付、Authz 等模型已大幅原生化；三库兼容仍需逐项守住。 |
| `middleware` | 6 | 0 | 25 | 请求体限制、HeaderNav 模块鉴权和治理中间件已吸收；剩余差异在细节实现和配套测试。 |
| `common` | 7 | 0 | 54 | 请求体限制、QuotaMath、SSRF 防护和辅助工具已增强；剩余差异仍集中在共享 helper 的实现细节。 |
| `relay` | 5 | 4 | 212 | 逐协议迁移仍是高差异面，必须补测试并检查 StreamOptions 支持。 |
| `setting` | 0 | 1 | 49 | 不直接覆盖设置结构，按页面和 API 逐项合并。 |
| `pkg` | 1 | 0 | 19 | 涉及计费表达式时继续遵守 `pkg/billingexpr/expr.md`。 |
| `web/default/src/routes` | 3 | 0 | 61 | `/register` 兼容路由已被 NexusTok 吸收，new-api-main 不再有独有默认前端路由文件。 |
| `web/default/src/features` | 97 | 35 | 567 | 账号池、Authz、SystemInfo/SystemTask、计费与 Playground 能力已显著扩展；仍需继续分批吸收 DataTable、日志与治理体验。 |
| `web/default/src/components` | 32 | 37 | 174 | 优先迁移安全渲染、通用表格布局、Dialog/Drawer 长表单布局，并继续保持与现有设计系统一致。 |
| `web/default/src/hooks` | 1 | 1 | 20 | `use-sidebar-view` 等治理型 hook 仍有迁移价值。 |
| `web/default/src/lib` | 4 | 0 | 31 | Admin permissions、frontend cache 等底座已经原生化，剩余差异主要是内容格式和工具性库。 |
| `web/classic/src` | 4 | 0 | 399 | Classic 保留账号池/计费入口，剩余主要是同路径实现差异。 |

## 页面级覆盖结论

默认前端路由文件当前只有 3 个 NexusTok 独有页面入口：

| 项目 | 独有路由文件 | 页面 |
|------|--------------|------|
| NexusTok | `_authenticated/account-pool/index.tsx` | `/account-pool` |
| NexusTok | `_authenticated/account-pool/$section.tsx` | `/account-pool/:section` |
| NexusTok | `_authenticated/pricing-settings/index.tsx` | `/pricing-settings` |

`/register` 兼容路由已在 NexusTok 默认前端吸收，`/system-info` 也已落地，因此 new-api-main 当前不再有默认前端独有路由文件。其它默认前端页面大多属于“同路径但实现不同”，已经在主文档按路由、feature 目录和组件能力列出。后续迁移时应以同路径页面为单位做局部对比，而不是简单复制 new-api-main 文件。

Classic 前端没有 new-api-main 独有业务页面。NexusTok 独有 classic 账号池页和计费设置页必须保留，new-api-main 的 classic 差异主要是退场提示和主题 helper。

## 原生化执行原则

1. 对独有文件差异，先判断是 NexusTok 原生优势还是 new-api-main 优势；NexusTok 账号池相关文件默认保留。
2. 对同路径不同文件，不做整文件覆盖；先定位页面/接口的业务意图，再把优势能力抽成 NexusTok 原生实现。
3. 后端新增能力必须遵守本项目 JSON 封装、三数据库兼容、中文注释和提交规范。
4. 前端新增页面必须先确认后端接口已经原生化，新增文案走 i18n。
5. 每完成一个独立能力，更新主差异文档或本文的迁移状态，形成可审计的演进轨迹。
