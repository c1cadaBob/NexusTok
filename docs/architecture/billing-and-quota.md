# 计费、额度与结算原理

> 文档状态：代码事实基线
> 事实基线日期：2026-10-14
> 主要代码来源：`service/billing.go`、`service/billing_session.go`、`service/quota.go`、`service/tiered_settle.go`、`relay/helper/price.go`、`relay/common/relay_info.go`、`common/quota_math.go`、`service/log_info_generate.go`、`pkg/billingexpr/`
> 关联详细文档：[`../key-routing-strategy.md`](../key-routing-strategy.md)、[`../plugin-api/README.md`](../plugin-api/README.md)、[`../../pkg/billingexpr/expr.md`](../../pkg/billingexpr/expr.md)、[`relay-routing-and-conversion.md`](./relay-routing-and-conversion.md)、[`tasks-and-plugins.md`](./tasks-and-plugins.md)

## 1. 功能目标和边界

计费系统将模型 Token、音频、图片、任务时长/数量、固定请求价格和上游 Usage 转换为内部 quota，并从钱包或订阅资金来源中预扣、结算、退款和记录日志。账务权威由数据库更新和 `BillingSession` 生命周期共同构成；前端显示和普通缓存不是扣费权威。

表达式语言、Token 正规化、版本和详细函数语义以[`pkg/billingexpr/expr.md`](../../pkg/billingexpr/expr.md)为准，本文件只描述整体调用链和安全不变量。

## 2. 价格来源和计费模式

| 模式 | 入口/来源 | 当前作用 |
| --- | --- | --- |
| 旧倍率计费 | `setting/ratio_setting/`、`relay/helper/price.go` | 模型倍率、补全倍率、音频倍率、分组倍率和用户组倍率 |
| 内置模型价格 | `setting/billing_setting/builtin_billing.go` | 以 USD/百万 Token 表达的内置模型价格 |
| Tiered Billing Expression | `pkg/billingexpr/`、`service/tiered_settle.go` | 按上下文长度、输入/输出、缓存和其它变量分段计算 |
| 图片价格 | Image Relay 的 `PriceData` 和图片参数 | 数量、分辨率、质量等计费乘数 |
| 音频价格 | Audio/Realtime Usage 和音频倍率/价格 | 文本 Token、音频 Token、时长或固定单价 |
| 任务价格 | 任务插件 `usageSchema`、视频/音乐任务结算 | second、count、token、credit 等主机认可单位 |
| 固定请求价格 | 请求或渠道价格配置 | 与 Token Usage 无关或最低消费的单次价格 |

管理员价格覆盖优先于内置价格；新内置模型应写在内置表达式来源，不应重新扩展已废弃的旧模型倍率表。

## 3. 一次同步 Relay 的数据流

```text
请求校验
  -> EstimateBilling / PriceData / Tiered RequestInput
  -> quota 转换（common/quota_math.go）
  -> PreConsumeBilling
     -> WalletFunding 或 SubscriptionFunding
     -> Token quota 预扣
     -> 特定钱包信任额度旁路
  -> 上游请求/流式/Usage
  -> actual quota 重新计算
  -> BillingSession.Settle(actualQuota)
     -> 资金来源 delta
     -> Token quota delta
     -> 订阅使用量和通知
  -> 消费日志和管理员诊断
```

`relay/common/RelayInfo` 保存 `PriceData`、`FinalPreConsumedQuota`、`Billing`、资金来源、订阅信息、`RequestId`、`QuotaClamp` 和日志所需的 Usage。`BillingSession` 对同一请求的预扣、补扣、返还和退款提供幂等保护。

## 4. 预扣、信任额度、结算和退款

`PreConsumeBilling` 先拒绝负 quota 和已经发生的 quota 饱和，再创建 `BillingSession`。正常预扣顺序是：

1. 判断是否命中特定钱包场景的信任额度旁路。
2. 预扣 Token quota。
3. 预扣钱包或订阅资金来源。
4. 任一步失败时回滚已完成的 Token/资金预扣。

信任额度旁路可能令实际预扣为零，但不代表请求不计费；后续 `Settle` 仍根据实际 quota 从资金来源和 Token quota 调整。异步任务在请求返回后仍会运行，因此通过 `RelayInfo.ForcePreConsume` 强制全额预扣，不能把同步信任语义直接复制到任务。

结算时：

- `actualQuota > preConsumed`：补扣差额；
- `actualQuota < preConsumed`：从资金来源和 Token quota 返还差额；
- 相等：不调整；
- 资金来源结算成功而 Token 调整失败时，代码记录错误并避免再次退款，管理员需要依据日志和数据库对账。

请求失败且资金来源尚未结算时，`BillingSession.Refund` 异步退还资金和 Token quota，并保证重复调用不重复退款。已经进入 `fundingSettled` 的请求不会再次执行资金退款。

## 5. 钱包与订阅

钱包资金来源使用用户 quota；钱包结算可以在余额不足时形成临时负余额，以保持扣费和日志实际变动一致，数据库错误才会中断。订阅资金来源使用订阅项的额度/次数，`SubscriptionPreConsumed`、`SubscriptionPostDelta` 和订阅 ID 会进入 `RelayInfo` 和日志。

订阅预扣不足、没有可用订阅、钱包不足和 Token quota 不足会映射为不同的内部错误原因，但对外通常都属于额度不足/禁止继续请求。订阅额度通知和钱包额度通知的计算来源不同。

### 5.1 管理员钱包中的上游平台总余额（2026-10-14）

**变更前**：钱包统计只展示当前 NexusTok 用户的余额、累计使用量和请求数，不展示
平台站点账号的共享余额。

**变更后**：管理员钱包增加“上游平台总余额”统计项。后端只在管理员或超级管理员的
`GET /api/user/self` 响应中追加 `upstream_platform_balance`，按
`PlatformSiteAccount` 汇总 NewAPI/Sub2API 的本地余额快照，并通过
`common.QuotaPerUnit` 转换为系统 quota。每个账号只统计一次，不按平台子密钥重复
累加；手动禁用、自动禁用和同步失败但仍保留余额快照的账号仍计入，异常负数、NaN、
Inf 和无法安全转换的值不计入总额，但账号数量仍表示已配置账号数。

钱包页面同时显示纳入统计的账号数量和最近同步时间。读取钱包不会登录或请求上游，
Redis 也不参与该聚合；普通用户、普通 API Token 和非管理员上下文不会收到该字段。
余额聚合失败只记录脱敏错误并保留普通钱包响应。本次不新增数据库字段、迁移或公开
独立接口。

### 5.2 平台站点请求级上游成本（2026-10-14）

**变更前**：Sub2API 和 NewAPI 成本由 UTC 五分钟窗口任务在请求完成后批量查询，
无法匹配的估算值最多延迟三个周期重试；错误日志没有稳定的成本映射。

**变更后**：平台站点请求在统一结算日志路径中即时处理一次成本。成功响应若已由平台
类型解析阶段确认 `actual_cost` 或 `quota`，直接保存上游成本；否则只调用一次对应的
Sub2API 优先访问当前登录用户的普通使用日志接口 `/api/v1/usage`，仅在路由返回
`404/405` 时回退 `/api/v1/admin/usage`；NewAPI 普通账号访问 `/api/log/self`，仅
`404/405` 时回退 `/api/log/`，并按
`upstream_request_id`、`upstreamRequestId`、`request_id`、`requestId` 的兼容优先级进行匹配。
Sub2API 平台站点不发送会被上游忽略的客户端 ID，而采用响应头返回的
`X-Client-Request-ID` 并规范化为 `client:<id>` 精确匹配使用日志；NewAPI 使用标准上游
请求 ID。Sub2API 普通使用日志接口不支持按请求 ID过滤，故按模型/日期分页直到精确命中、
越过三秒时间窗或达到查询上限。只有缺少权威关联 ID时，才按同渠道、模型和三秒内唯一的
最近时间匹配。Sub2API 的美元
`actual_cost` 使用 `common.QuotaPerUnit` 转换，
NewAPI 的 `quota` 使用本项目额度单位。

认证/权限/网络失败、空结果、日志尚未生成、非法成本或歧义匹配不影响本地计费，直接
进入一次性兜底：本地费用乘以上游密钥倍率快照；缺少可靠快照时写入 `0 estimated`。
错误请求不访问上游费用接口，固定写入 `0 estimated`。同一请求上下文不会重复查询，
上游成本可以覆盖先前的估算值，估算值不能覆盖上游值。

成本映射继续只保存在主数据库，独立日志数据库和 ClickHouse 日志表不变，Redis 不参与
成本持久化。管理员日志通过 `other.admin_info` 查看成本；普通用户和普通令牌日志移除
整个 `admin_info`。估算来源使用黄色显示，鼠标悬停、键盘聚焦或触屏激活时显示安全的
兜底原因；上游真实成本为绿色且不显示兜底说明。

### 5.2.1 2026-10-16 Sub2API 普通用户成本接口修正

**变更前**：成本查询统一调用仅管理员可访问的 `/api/v1/admin/usage`，普通 `user`
身份返回 `403` 后，成功请求全部使用本地倍率估算；原有两秒匹配窗口还可能漏掉实际晚约
两秒的上游使用记录。

**变更后**：普通 Sub2API 账号优先调用 `/api/v1/usage`，只有 `404/405` 路由缺失时才
回退管理员接口；`403` 不触发回退。模型和时间匹配窗口扩大为三秒，并保持多候选时拒绝
自动匹配。查询失败日志只记录脱敏诊断，不记录响应正文、凭据或会话信息。

NewAPI 与 Sub2API 成本查询复用按渠道隔离的内存会话并串行访问，密码会话材料不写回凭据；
已知 Token 过期或认证失效时才重新认证，`429 AUTH_SESSION_ISSUANCE_LIMIT` 使用失败冷却，
避免登录风暴。响应头捕获保留 `X-Oneapi-Request-Id` 优先级，并兼容标准
`X-Request-ID`/`X-Request-Id`；Sub2API 客户端关联 ID来自响应头，NewAPI 匹配不使用该字段。

### 5.2.2 2026-10-16 Sub2API 请求关联与估算原因

**变更前**：Sub2API usage 的 `request_id` 使用上游自行生成的
`client:<X-Client-Request-ID>`。Relay 曾发送无效的自生成请求头并保留本地值，忽略响应头中
真正的上游 ID，导致精确匹配失败；估算成本也未明确区分缺少上游 ID与已有 ID无匹配。

**变更后**：Sub2API 不再发送会被上游忽略的 `X-Client-Request-ID`；从响应头读取上游
生成的 `X-Client-Request-ID` 并规范化为 `client:<id>`，通用上游响应 ID独立保存。普通
`/api/v1/usage` 不支持按请求 ID筛选，因此按模型和日期分页，精确命中即停止，未命中则
继续到越过三秒时间窗或达到超时/页数上限。仅响应未提供权威 ID时才尝试唯一模型/时间匹配；
NewAPI 仍使用其自身精确请求 ID。实时失败原因码及可选 HTTP
状态/平台错误码保存在原始日志 `other.admin_info`，不新增日志表字段，不写请求 ID、凭据、
Cookie、Token 或响应正文；普通用户和 Token 日志仍剥离 `admin_info`。黄色成本 Tooltip
支持鼠标、键盘和触屏，旧记录无原因时标注为历史估算。NewAPI `429
AUTH_SESSION_ISSUANCE_LIMIT` 不触发管理员路由回退或绕过认证，需上游管理员清理旧会话。

## 6. 任务计费

任务提交前需要根据任务平台或插件 `usageSchema` 估计费用并强制预扣；轮询到终态后由任务结算路径根据最终 Usage、失败状态、超时和取消策略进行结算或退款。`model.Task`、任务私有数据、`service/task_polling.go` 和任务计费函数共同维持任务状态与账务关联。

任务轮询失败不能简单等同于上游业务失败：传输错误、认证错误、Not Found、临时错误、无法识别状态和插件 Hook 错误可能采用不同的重试/失败阈值。结算日志应记录任务 ID、请求 ID、预扣/实际 quota 和失败原因，但不能写入可用凭据。

## 7. 额度安全不变量

- 所有用户控制的计费乘数必须在到达 quota 计算前限制范围；图片 `n`、视频时长、分辨率/质量倍率、批量数量、`max_tokens` 等不能仅依赖前端。
- `common/quota_math.go` 集中执行 `QuotaFromFloat`、`QuotaRound`、`QuotaFromDecimal` 和钱包边界转换；禁止对不受控浮点/Decimal 直接 `int(...)`。
- 单请求 quota 饱和到 int32 边界；钱包额度使用 JavaScript 安全边界 `MaxWalletQuota`。
- 非法倍率、非正数、NaN、正无穷和溢出不能产生负费用或用户额度倒灌。
- `types.PriceData.AddOtherRatio` 是倍率地图的校验入口，不能直接写入 `OtherRatios`。
- `*Checked` 转换返回 `QuotaClamp`；计费路径将其写入 `RelayInfo.QuotaClamp`，`attachQuotaSaturation` 将标记放入消费/任务日志的 `other.admin_info.quota_saturation` 并写告警。
- 预扣和结算必须使用同一计费身份、资金来源和请求 ID 语义；自动分组重试会刷新组依赖的价格快照。
- 异步任务必须在提交前锁定所需额度，避免任务已经上游运行后才发现无法扣费。

## 8. 对外接口、配置和数据模型

- 计费会话：`service.BillingSession`、`relaycommon.BillingSettler`。
- 请求账务状态：`relay/common/relay_info.go` 的 `RelayInfo`。
- 价格：`types.PriceData`、`relay/helper/price.go`、`setting/billing_setting/`、`pkg/billingexpr/`。
- 用户和 Token 额度：`model.User`、Token 模型、`model.DecreaseTokenQuota`/`IncreaseTokenQuota`。
- 订阅：`model/subscription.go` 及 `SubscriptionFunding`。
- 日志：`service/log_info_generate.go`、消费日志和任务结算日志。
- 关键配置：分组倍率、模型价格、订阅策略、任务计费配置以及环境变量中的 billing 相关选项。

## 9. 当前限制和实际偏差

- 旧倍率和表达式计费并存，文档或新增模型不能只看一套价格表。
- 资金来源提交成功后 Token 调整失败会记录错误，但不能自动假设数据库已经完成完整补偿；这属于对账和运维关注点。
- 任务最终价格依赖任务插件 Usage 提供和轮询终态，插件示例价格不等同于实际店铺价格。
- 只有代码能直接证明的溢出、未实现或结算差异才登记为偏差；尚未完成的供应商价格核对标记为“待核查”。

## 10. 维护时需要同步的关联模块

修改价格来源、倍率、表达式、Usage、预扣、结算、退款、订阅、Token quota、任务计费、日志字段或安全边界时，必须同步本文档、[`pkg/billingexpr/expr.md`](../../pkg/billingexpr/expr.md)、任务/Relay 文档和偏差登记。涉及新渠道或模型价格时还要同步能力矩阵与[`key-routing-strategy.md`](../key-routing-strategy.md)中的模型获取/日志说明。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 初次建立 | 仓库中没有统一功能原理基线 | 建立价格来源、BillingSession、资金来源、任务结算和额度安全不变量说明 | `service/`、`relay/`、`common/quota_math.go`、`pkg/billingexpr/` | 计费会话、quota helper、日志和表达式文档静态核对 |
| 2026-10-14 | 管理员钱包平台余额 | 钱包只显示 NexusTok 用户自身额度和使用量 | 管理员从 `/api/user/self` 读取按平台账号去重的 NewAPI/Sub2API 本地余额快照总额；禁用账号仍计入，不触发上游同步，普通用户不可见 | `model/platform_site_balance.go`、`controller/user.go`、`web/src/features/wallet/` | SQLite 汇总/管理员权限定向测试、钱包组件测试、TypeScript、lint 和 i18n 同步 |
