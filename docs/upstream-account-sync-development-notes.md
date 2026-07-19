# 上游账号同步功能开发记录

## 需求澄清

1. 上游账号同步与手动凭证配置是互斥入口，不能同时展示。
2. 上游账号同步后，密钥、分组、倍率、余额需要按账号级别展示和编辑。
3. 账号同步后的保存分为两类：
   - 重新拉取上游账号数据，属于 refresh。
   - 保存当前已同步 key 的本地配置，属于 local save。
4. 本地保存不能强依赖 preview snapshot，否则会出现“同步后无法保存”的问题。
5. 已同步 key 的本地配置应能独立维护，不要求每次重新登录上游。

## 影响范围

- `web/default/src/features/channels/components/drawers/channel-mutate-drawer.tsx`
- `web/default/src/features/channels/lib/channel-form.ts`
- `web/default/src/features/channels/hooks/use-channel-mutate-form.ts`
- `web/default/src/features/channels/api.ts`
- `service/upstreamaccount/create.go`
- `service/upstreamaccount/refresh.go`
- `controller/channel_account.go`
- `service/upstreamaccount/sync_metadata.go`
- `web/default/src/i18n/locales/*`

## 风险评估

1. 同步保存流程如果错误复用 refresh snapshot，可能覆盖已有本地账号配置。
2. 账号列表分页超过 100 时，前端只加载部分 key 就保存会造成遗漏。
3. 本地保存逐 key 配置时，若没有正确处理启停权限，可能绕过运维权限边界。
4. 如果把“刷新上游”和“保存本地配置”混成一个按钮，用户会持续误解保存失败原因。
5. 通用渠道更新接口已禁止携带 `status`，同步保存如果复用完整表单 payload 会被后端拒绝为“无效的参数”。

## 当前方案

- 已同步渠道编辑页增加账号列表读取，用于回填逐 key 配置。
- 保存按钮在没有 refresh snapshot 时，走本地逐 key 配置保存。
- refresh 入口保留，用于重新获取上游账号、余额和倍率。
- 保存本地配置时会同步更新渠道聚合模型/分组，避免路由能力丢失。
- 上游账号同步和手动 API 凭证是互斥入口：同步渠道编辑页只展示同步密钥配置与可选刷新区，不展示手动 API Key 配置区。
- 本地保存只向 `PUT /api/channel/` 提交渠道聚合能力字段；每个同步 key 的模型、分组、优先级、权重和启停状态分别写入 `ChannelAccount` 接口。
- 渠道启停状态和同步 key 启停状态分别走专用状态接口，不混入普通保存请求。

## 验证计划

1. 用 `bun run typecheck`、`bun run build`、`go test` 做基础校验。
2. 用浏览器访问 `http://192.168.0.202:3003/`，实际打开渠道编辑抽屉检查：
   - 同步模式和手动凭证是否互斥。
   - 同步 key 是否能直接保存本地配置。
   - 预览刷新是否仍可用。
3. 必要时检查 `/api/channel/:id/accounts` 和 `/api/channel/:id/upstream-account/refresh` 请求。

## 2026-07-18 深度缺陷审计

### 已确认缺陷

1. 已同步渠道编辑页虽然已经隐藏了手动 API Key 和手动凭证区，但“同步密钥配置”和“刷新上游账号”两个同步相关区块同时展开。用户会把刷新区的账号密码输入误认为另一套手动凭证入口。
2. 主保存按钮在存在刷新预览时会自动走刷新应用分支，导致“保存当前 key 配置”和“重新同步上游账号”混在一起。
3. 已同步 key 的本地配置索引曾使用数据库 `account.id`，而刷新预览使用上游 `sync_id/external_id/masked_key`。两套索引不一致时，刷新预览会把本地模型、分组、优先级和权重草稿重置为上游建议值。
4. 后端刷新已有 `ChannelAccount` 时仍会按快照重写 `models/group`，这会覆盖管理员在 NexusTok 内对每个 key 维护的独立模型和分组。

### 修复方案

1. 刷新上游账号区块默认折叠，主区块只展示同步 key 的本地配置。刷新仍可用，但需要管理员显式展开并点击“应用刷新”。
2. 同步渠道的底部主按钮固定为“保存同步密钥配置”，只保存当前本地 key 配置，不再自动应用刷新预览。
3. 前端配置索引统一优先使用上游同步标识，刷新预览会合并已有本地草稿，不会无条件覆盖。
4. 后端 refresh 对已有账号保留本地 `models/group`；只有请求显式提交非空 `models/group` 时才覆盖。

### 风险评估

1. 因为 `AccountCreateConfig.Models/Group` 仍是 string，后端无法区分“字段缺失”和“显式清空”。刷新接口本轮采用保守语义：空值视为未覆盖，保留已有本地配置；需要显式清空时继续通过 `PUT /api/channel/:id/accounts/:account_id` 本地保存接口完成。
2. 折叠刷新区块会减少误操作，但管理员仍能手动展开刷新，保留重新同步新 key、倍率和余额的能力。
3. 配置索引依赖上游 `external_id` 或脱敏 key 兜底。目标平台如果同时更换 external_id 和 key，刷新会按新 key 创建新账号，这是符合“上游身份改变”的保守行为。

### 验证补充

1. 后端测试覆盖刷新保留本地 `models/group`，以及显式传入非空配置时覆盖。
2. 前端测试覆盖已同步账号使用上游同步标识作为配置 key，并在刷新快照时复用已有本地配置。
3. MCP 页面验证需要确认：已同步渠道编辑页手动凭证区不可见，刷新区默认折叠，底部按钮文案为“保存同步密钥配置”，修改 key 的模型/分组后保存成功。

## 2026-07-18 深度缺陷审计补充：同步 key 启停与渠道能力不一致

### 已确认缺陷

同步渠道的渠道顶层 `models/group` 会写入 `abilities` 表，是普通请求进入 Relay 前的路由能力来源。上一轮本地保存和刷新虽然允许逐密钥启停，但渠道顶层能力汇总仍可能把已禁用或已跳过的同步 key 也统计进去；同时普通 `Channel.UpdateAbilities` 会把渠道级模型和分组做笛卡尔积，无法表达“某个模型只属于某个 key 的某个分组”的真实组合。

典型场景：

1. 上游账号同步出两个 key：A 支持 `gpt-a/default`，B 支持 `gpt-b/vip`。
2. 管理员在 NexusTok 中禁用 B，或创建/刷新时选择不启用 B。
3. 渠道顶层仍汇总为 `gpt-a,gpt-b` 与 `default,vip`。
4. `abilities` 表继续认为该渠道支持 `gpt-b/vip`；如果 A 和 B 的模型/分组不同，还可能额外生成 `gpt-a/vip`、`gpt-b/default` 这类任何启用 key 都不支持的组合。请求会先被分发到这个渠道，然后在账号池选择阶段才发现没有可用账号。

### 影响范围

1. 前端同步保存：`saveSyncedAccountLocalConfigs` 先根据表格汇总渠道顶层模型/分组，再逐条写账号配置。如果汇总没有过滤 `enabled=false` 的 key，会把禁用 key 的能力写入渠道。
2. 后端创建：`CreateFromPreview` 过去先从快照或请求渠道字段推断顶层模型/分组，再创建账号。被 `accounts[].enabled=false` 跳过的 key 仍可能影响渠道能力。
3. 后端刷新：`RefreshChannelFromSnapshot` 过去先按快照更新渠道顶层模型/分组，再 upsert/禁用账号。最终账号状态与渠道能力可能不一致。
4. 路由降级：请求级渠道排除和账号级排除已经能处理失败后的降级，但不一致的 `abilities` 会制造额外错误重试，管理员也会误以为禁用 key 或调整 key 分组后路由立即消失但实际仍被候选命中。

### 方案评审

1. 前端汇总渠道顶层能力时跳过 `enabled=false` 的同步 key；页面保存后，渠道能力只代表当前启用 key 的并集。
2. 后端创建在生成账号后，基于最终会落库的启用账号重新计算渠道顶层 `models/group`，并按每个启用账号的真实模型/分组组合写入 `Ability`。
3. 后端刷新在完成创建、更新和缺失 key 禁用后，再读取最终账号列表，并基于启用账号重建渠道 `models/group` 与 `Ability`。
4. 若最终没有任何启用同步账号，渠道顶层 `models` 清空、`group` 保留默认或历史分组。这样能力表不会继续暴露不可用模型；渠道仍可在管理端保存和后续重新启用 key。

### 风险评估

1. 清空 `models` 会让该渠道暂时不参与普通模型路由，这是预期行为，因为没有启用同步 key 可承接请求。
2. API 直接调用 create/refresh 时也会被后端重新汇总，避免绕过前端产生不一致状态。
3. 已有“刷新保留本地模型/分组”的语义仍保留在账号级；本次只改变渠道顶层能力汇总和能力表来源，使其匹配最终启用账号集合及其真实模型/分组组合。

### 验证补充

1. 后端创建测试覆盖：禁用一个同步 key 后，渠道顶层 `models/group` 和 `abilities` 只包含启用 key。
2. 后端刷新测试覆盖：刷新时显式禁用一个同步 key 后，最终渠道能力表只包含启用账号。
3. 前端测试覆盖：`upstreamAccountValuesToString` 汇总时忽略 `enabled=false` 的 key。
4. Model 层测试覆盖：账号池渠道只生成启用账号真实支持的模型/分组组合，不生成渠道级笛卡尔积中的虚假能力。

## 2026-07-18 交互与保存闭环缺陷审计

### 已确认缺陷

1. 创建渠道页的“手动 API 凭证 / 上游账号同步”本应是互斥入口，但凭证来源卡片只依赖 Base UI radio 的内部点击事件。实际页面验证中点击“上游账号同步”没有可靠切换，导致手动 API 地址、凭证模式和 API Key 区块仍停留在页面上。
2. 已同步渠道的逐 key 保存会调用 `PUT /api/channel/:id/accounts/:account_id` 和状态接口，但 `controller/channel_account.go` 在账号新增、更新、删除、启停后没有重建账号池渠道的顶层 `models/group` 与 `abilities`。账号表可能已经保存成功，路由能力和列表聚合仍停留在旧值，表现为“保存后配置没有生效”。
3. 编辑同步渠道时，“刷新上游账号”虽然默认折叠，但仍位于“凭证”区块里，且标题和账号密码输入容易被理解为另一套手动 API 凭证配置入口。

### 影响范围

1. 前端创建渠道抽屉：凭证来源卡片点击、互斥区块卸载、同步模式下模型与分组共享面板隐藏。
2. 前端编辑同步渠道抽屉：同步密钥本地配置与重新登录上游刷新需要更清晰分区，避免管理员把保存本地 key 配置误认为必须重新填写账号密码。
3. 后端渠道账号控制器：单个账号新增/更新/删除、批量导入、多 Key 导入、账号状态更新都可能改变账号池渠道能力。

### 方案评审

1. 凭证来源卡片增加显式 `onClick` 兜底，点击卡片任意区域都调用 `handleUpstreamSyncEnabledChange`。Base UI radio 仍保留为可访问控件，视觉状态由同一个 `upstreamSyncEnabled` 驱动。
2. 编辑同步渠道的刷新入口更名为“重新同步上游账号”，并用说明强调这是可选操作；普通保存只保存当前已同步 key 的模型、分组、优先级、权重和启停。
3. 后端新增统一的 `syncChannelAccountCapabilitiesIfNeeded`，在渠道内账号池且无 fallback 时调用 `model.SyncChannelAccountPoolCapabilities`，随后刷新渠道缓存和代理客户端缓存。该函数放在账号变更成功后调用，避免影响非账号池渠道。
4. 账号状态只清除冷却时间且未改变 status 时不重建能力；启用/禁用账号时重建能力。

### 风险评估

1. 账号变更后多一次能力重建查询，影响只限账号池渠道；普通单 Key、多 Key 渠道不会额外重建账号池能力。
2. 批量导入大量账号后一次性重建能力，避免每个账号单独重建造成额外数据库压力。
3. 前端卡片 `onClick` 必须尊重敏感写权限，权限不足时不切换，不清空已有草稿。
