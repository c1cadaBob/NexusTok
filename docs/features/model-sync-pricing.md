# 模型同步与价格同步

NexusTok 的模型同步用于把上游模型目录导入到本地模型库，并可选择把 models.dev 中的 provider 价格同步到模型级定价配置。该功能主要服务于管理员后台的模型管理页面，以及每日 models.dev 自动同步任务。

## 功能入口

后台入口：

1. 进入 `Models` 管理页面。
2. 点击 `Sync Upstream Models`。
3. 选择同步源、语言和价格同步策略。
4. 如预览发现冲突，先在冲突弹窗中选择需要覆盖的字段，再应用同步。

接口入口：

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/models/sync_upstream/preview` | 预览上游与本地模型元数据差异 |
| `POST` | `/api/models/sync_upstream` | 执行模型、供应商和可选价格同步 |
| `GET` | `/api/models/:id/pricing` | 获取单个模型聚合后的定价配置 |
| `PUT` | `/api/models/:id/pricing` | 保存单个模型定价配置 |

以上 `/api/models/*` 管理接口需要管理员登录态。

## 同步源

| 来源 | `source` | 数据内容 | 适用场景 |
|------|----------|----------|----------|
| 官方仓库 | `official` | 项目维护的模型和供应商元数据 | 需要沿用旧版官方元数据仓库时 |
| models.dev | `models.dev` | models.dev catalog 中的 canonical models、provider 目录和 provider 价格 | 推荐；可补齐模型、纠正模型原厂归属并同步 provider 价格 |
| 配置文件 | `config` | 预留入口 | 当前前端禁用，后续用于本地配置导入 |

语言参数 `locale` 来自后台展示的 `zh`、`en`、`ja` 选项。官方仓库会在后端识别到对应 locale 时读取 i18n 路径，否则回退默认路径；models.dev 使用同一份 catalog，语言只用于保持请求结构一致。

## 元数据同步规则

同步模型元数据时会处理以下字段：

| 字段 | 说明 |
|------|------|
| `model_name` | 模型唯一标识 |
| `description` | 模型描述 |
| `icon` | 图标 |
| `tags` | 标签 |
| `vendor` | 供应商 |
| `name_rule` | 名称匹配规则 |
| `status` | 启用状态 |

默认手动同步的补齐范围按来源区分：`models.dev` 会按完整 catalog 补齐所有本地不存在的模型和供应商；`official` 沿用旧行为，只补齐当前能力表中已经被引用但缺少元信息的模型。每日 models.dev 自动同步同样按完整上游目录补齐本地不存在的模型和供应商。已有模型不会被静默覆盖，除非管理员在冲突预览中明确选择覆盖字段。

接口调用方可以通过 `create_all` 显式控制补齐范围：`true` 表示按上游完整目录补齐本地缺失模型，`false` 表示只处理当前能力表缺失项。页面上的 models.dev 同步默认等价于 `create_all=true`，因此像 `gpt-5.6-luna`、`gpt-5.6-sol`、`gpt-5.6-terra` 这类同系列模型会一次性补齐。

models.dev 同步会优先使用 canonical models 的归属方作为本地供应商。例如 provider 目录中某个 OpenAI 模型可能由第三方服务商提供，但本地模型供应商仍应归属到 OpenAI，而不是误写成 serving provider。

本地模型如果关闭了 `sync_official`，同步时会跳过该模型，避免覆盖管理员明确排除的记录。

## 价格同步策略

models.dev 来源支持价格同步。官方仓库目前没有统一 provider 价格结构，因此不会应用价格同步策略。

请求体示例：

```json
{
  "locale": "zh",
  "source": "models.dev",
  "create_all": true,
  "pricing": {
    "enabled": true,
    "overwrite_manual": false,
    "provider_order": ["openai", "anthropic", "google", "azure"]
  }
}
```

字段说明：

| 字段 | 默认行为 | 说明 |
|------|----------|------|
| `pricing.enabled` | 后台 models.dev 同步默认开启 | 是否把选中的 provider 价格写入模型定价配置 |
| `pricing.overwrite_manual` | `false` | 是否允许上游价格覆盖管理员手动确认过的价格 |
| `pricing.provider_order` | 空列表 | provider 降级顺序；为空时使用 models.dev provider 的稳定排序 |

provider 匹配支持 provider ID 或 provider name，比较时会忽略大小写和首尾空白。后台输入框中每行一个 provider，也支持接口传数组。

价格同步只会给本地已经存在的模型写入价格，避免产生没有模型记录的孤儿定价键。同步结果会返回：

| 字段 | 说明 |
|------|------|
| `pricing_updated` | 成功写入价格的模型数 |
| `pricing_skipped` | 因本地不存在、价格不可转换或策略保护而跳过的模型数 |
| `pricing_list` | 成功更新价格的模型名列表 |

## 手动价格保护

模型级定价来源保存在 `options` 表的 `ModelPricingSource` 中：

| 来源 | `kind` | 说明 |
|------|--------|------|
| 管理员手动保存 | `manual` | 在模型编辑页保存的价格；默认不允许上游同步覆盖 |
| 上游同步写入 | `upstream` | 由 models.dev 价格同步写入；后续同步可以按策略更新 |

历史版本没有 `ModelPricingSource` 元数据。为了避免升级后每日同步静默改价，只要旧的 `options` 中存在模型级定价覆盖，且 `overwrite_manual=false`，同步也会按手动配置保护。

只有在管理员明确开启 `overwrite_manual=true` 时，上游同步才会覆盖手动价格或历史覆盖价格。

## 价格换算规则

models.dev provider 价格通常以美元每 1M tokens 表示。NexusTok 的 relay 热路径仍沿用现有 `ModelRatio`、`CompletionRatio`、`CacheRatio` 等 options，因此同步时会转换为 ratio 模式保存。

换算关系：

| models.dev 字段 | 本地字段 | 规则 |
|-----------------|----------|------|
| `cost.input` | `input_price_per_million` / `ModelRatio` | `ModelRatio = input / 2` |
| `cost.output` | `output_price_per_million` / `CompletionRatio` | `CompletionRatio = output / input` |
| `cost.cache_read` | `CacheRatio` | `cache_read / input` |
| `cost.cache_write` | `CreateCacheRatio` | `cache_write / input` |
| `cost.input_audio` | `AudioRatio` | `input_audio / input` |
| `cost.output_audio` | `AudioCompletionRatio` | `output_audio / input_audio` |

如果 `input` 缺失、不是有效数字，或 `input=0` 但输出价格非零，当前 ratio 模式无法准确表达该价格，系统会跳过该 provider 候选并尝试下一个 provider。

模型编辑页中的“每 1M 输入 token 价格”和“每 1M 输出 token 价格”是管理员友好的展示层；保存时仍会转换为底层 options。实际调用计费会读取这些 options，进入 `ModelPriceHelper` 和结算流程。

## 自动同步

主服务启动后，主节点会启动 models.dev 每日自动同步任务。自动任务使用 `models.dev` 来源，默认启用价格同步，但不会覆盖手动价格。

相关环境变量：

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `MODELS_DEV_AUTO_SYNC_ENABLED` | `true` | 是否启用每日 models.dev 同步 |
| `MODELS_DEV_AUTO_SYNC_TIME` | `02:00` | 每日运行时间，格式 `HH:mm`，使用进程当前时区 |
| `MODELS_DEV_SYNC_BASE` | `https://models.dev` | models.dev 基础地址；可替换为内网镜像或代理 |
| `MODELS_DEV_PRICING_PROVIDER_ORDER` | 空 | 自动价格同步的 provider 降级顺序，逗号分隔，例如 `openai,anthropic,google,azure` |

自动任务日志示例：

```text
models.dev model sync task started: time=02:00 source=https://models.dev/catalog.json
models.dev model sync completed: created_models=12 created_vendors=3 updated_models=0 pricing_updated=20 pricing_skipped=4 skipped_models=0 source=https://models.dev/catalog.json
```

## 常见问题

### 手动价格为什么没有被同步覆盖？

默认策略会保护 `manual` 来源和历史定价覆盖。需要覆盖时，在后台同步弹窗中开启“允许覆盖手动定价”，或接口传入：

```json
{
  "source": "models.dev",
  "pricing": {
    "enabled": true,
    "overwrite_manual": true
  }
}
```

### 为什么同步结果里有 `pricing_skipped`？

常见原因：

- 本地没有对应模型记录；
- provider 没有有效的 `cost.input`；
- 价格结构无法用 ratio 模式表达；
- 当前策略保护了手动价格；
- `pricing.enabled=false` 或同步源不是 `models.dev`。

### 为什么 provider 顺序没有命中？

确认输入的是 models.dev 中的 provider ID 或 provider name，例如 `openai`、`anthropic`、`google`、`azure`。后台按行填写；环境变量使用逗号分隔。

### models.dev 自动同步没有运行？

参考 [启动与部署维护手册](../installation/deployment.md#modelsdev-模型目录没有自动同步) 检查主节点、环境变量、网络访问和任务日志。
