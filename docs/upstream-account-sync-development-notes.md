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
