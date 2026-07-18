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
