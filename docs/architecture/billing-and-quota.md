# 计费、额度与结算原理

> 文档状态：代码事实基线
> 事实基线日期：2026-09-25
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
