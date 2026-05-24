# 计费表达式系统

计费表达式系统允许管理员用一个表达式字符串完全定义模型的计费逻辑——定价、阶梯条件、缓存/图片/音频区分、时段折扣、请求感知倍率——全部在一行内完成。

## 设计哲学

**一个表达式，一个真相。** 表达式是管理员与系统之间的计费合约。你写什么，系统就执行什么。

### 核心原则

1. **表达式自包含** — 表达式字符串单独决定计费，无外部倍率表、无隐式规则
2. **变量按需使用** — `p`（输入）和 `c`（输出）是基础变量，缓存/图片/音频变量可选
3. **价格是真实价格** — 系数是实际的 $/1M tokens 价格，无需转换
4. **上游无关** — 表达式无需知道上游是 OpenAI 格式还是 Claude 格式，系统自动标准化
5. **版本感知** — 表达式可携带版本标签（`v1:`），控制编译环境和转换公式

## 表达式语言

基于 [expr-lang/expr](https://github.com/expr-lang/expr)，编译后缓存执行。

### Token 变量

**输入侧：**

| 变量 | 含义 |
|------|------|
| `p` | 输入 token 数（计价用）。自动排除表达式中单独计价的子类别 |
| `len` | 输入上下文总长度（条件判断用）。不受自动排除影响 |
| `cr` | 缓存命中（读取）token 数 |
| `cc` | 缓存创建 token 数（5 分钟 TTL） |
| `cc1h` | 缓存创建 token 数（1 小时 TTL，Claude 专用） |
| `img` | 图片输入 token 数 |
| `ai` | 音频输入 token 数 |

**输出侧：**

| 变量 | 含义 |
|------|------|
| `c` | 输出 token 数。自动排除单独计价的子类别 |
| `img_o` | 图片输出 token 数 |
| `ao` | 音频输出 token 数 |

### `p` 和 `c` 的自动排除机制

`p` 和 `c` 是"兜底变量"——代表所有没有被表达式单独定价的 token。系统根据表达式实际使用的变量，自动从 `p`/`c` 中减去对应的子类别 token，避免重复计费。

**示例（上游返回 prompt_tokens=1000，其中 200 cache read、100 image）：**

| 表达式 | `p` 的值 | 说明 |
|--------|---------|------|
| `p * 3 + c * 15` | 1000 | 没用 `cr`/`img`，全部按 $3 计费 |
| `p * 3 + c * 15 + cr * 0.3` | 800 | 缓存 200 从 `p` 扣除，按 $0.3 单独计费 |
| `p * 3 + c * 15 + cr * 0.3 + img * 2` | 700 | 缓存和图片都扣除，各自计费 |

> **重要：** 阶梯条件应使用 `len` 而非 `p`，以避免缓存命中导致 `p` 降低而误判档位。

### 内置函数

| 函数 | 签名 | 用途 |
|------|------|------|
| `tier` | `tier(name, value) → float64` | 记录匹配的定价档位 |
| `param` | `param(path) → any` | 读取请求体中的 JSON 字段（gjson 路径） |
| `header` | `header(key) → string` | 读取请求头 |
| `has` | `has(source, substr) → bool` | 子串检查 |
| `hour` | `hour(tz) → int` | 当前小时（0-23） |
| `weekday` | `weekday(tz) → int` | 星期几（0=周日，6=周六） |
| `max/min` | `max(a, b) → float64` | 数学最大/最小值 |
| `ceil/floor` | `ceil(x) → float64` | 向上/向下取整 |

### 表达式示例

```
# 简单统一定价
tier("base", p * 2.5 + c * 15 + cr * 0.25)

# 多阶梯（Claude Sonnet 风格）— 阶梯条件用 len
len <= 200000
  ? tier("standard", p * 3 + c * 15 + cr * 0.3 + cc * 3.75 + cc1h * 6)
  : tier("long_context", p * 6 + c * 22.5 + cr * 0.6 + cc * 7.5 + cc1h * 12)

# 图像模型
tier("base", p * 2 + c * 8 + img * 2.5)

# 多模态（含音频）
tier("base", p * 0.43 + c * 3.06 + img * 0.78 + ai * 3.81 + ao * 15.11)
```

### 请求规则（`|||` 分隔符）

在表达式后追加 `|||` 和请求条件倍率：

```
tier("base", p * 5 + c * 25)|||when(header("anthropic-beta") has "fast-mode") * 6
```

## 数据流

```
前端编辑器 → 存储 → 预扣费 → 结算 → 日志展示
```

### 1. 存储

两个选项 Map 存储在 `options` 表：
- `ModelBillingMode`: `{ "model-name": "tiered_expr" }` — 激活阶梯计费
- `ModelBillingExpr`: `{ "model-name": "tier(\"base\", p * 2.5 + c * 15)" }` — 表达式

保存时验证：编译检查语法 → 烟雾测试确保非负结果。

### 2. 预扣费（配额估算）

请求到达时：
1. 加载表达式
2. 构建 `RequestInput`（headers + body）供 `param()`/`header()` 使用
3. 用估算 token 数运行表达式
4. 转换为配额：`rawCost / 1,000,000 * QuotaPerUnit`
5. 创建 `BillingSnapshot`（冻结状态）存储在 `RelayInfo`

### 3. 结算（实际计费）

上游返回实际 token 后：
1. 构建实际 token 参数（自动处理 OpenAI/Claude 格式差异）
2. 用实际 token 重新运行表达式
3. 通过 `quotaConversion()` 转换为配额
4. 返回实际配额（与预扣费的差额自动结算）

## 关键文件

| 层级 | 文件 |
|------|------|
| 表达式引擎 | `pkg/billingexpr/compile.go`、`run.go`、`settle.go`、`types.go` |
| 存储 | `setting/billing_setting/tiered_billing.go` |
| 预扣费 | `relay/helper/price.go`、`relay/helper/billing_expr_request.go` |
| 结算 | `service/tiered_settle.go`、`service/quota.go` |
| 前端编辑器 | `web/src/pages/Setting/Ratio/components/TieredPricingEditor.jsx` |
