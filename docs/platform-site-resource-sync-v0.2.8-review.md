# 17 个平台站点重新登录与资源同步评审报告

评审日期：2026-10-10
目标版本：v0.2.8
报告状态：发布与远端验收进行中

## 验收口径

目标为截图中的 17 个平台站点。逐站核对管理地址后，在渠道编辑页执行本轮重新登录和平台资源同步。只有 `identity`、`groups`、`endpoints`、`usage`、`keys`、`models` 六项在本轮均显示 `success`，且未仅沿用历史快照，才记为通过。`partial`、`stale`、`failed`、`secure_verification_required`、未执行或只使用历史快照均不通过。

遇到验证码、Turnstile、TOTP、Passkey、人工安全验证或凭据无效时，停止该站点自动操作，保留脱敏诊断并继续其它站点；不绕过上游安全验证。报告不记录账号、密码、Cookie、Access/Refresh Token、API Key、完整响应或截图文件名。

## 初始问题分析

| 站点 | 截图中观察到的问题 |
| --- | --- |
| `gpttap.top` | 可选 `/api/ratio_config` 返回权限错误，但端点资源被标为失败 |
| `134.175.71.62` | Key/模型读取遇到上游安全验证 |
| `988665.xyz` | `/api/ratio_config` 权限错误；Key/模型状态不完整 |
| `ahriapi.com` | Key/模型状态不完整；旧 Refresh Token 无效 |
| `ai.hexuan.cc` | Key/模型状态不完整 |
| `ai.lt4net.org` | `/api/ratio_config` 权限错误；Key/模型读取遇到安全验证 |
| `api.apichatgpt.top` | `/api/ratio_config` 权限错误导致端点状态异常 |
| `api.luciferai.cc` | Key/模型状态不完整 |
| `api.lyy.team` | Key/模型状态不完整 |
| `dahlo.live` | `/api/ratio_config` 权限错误导致端点状态异常 |
| `hhw1231.com` | Key/模型状态不完整 |
| `proxygpt.cc.cd` | Key/模型状态不完整 |
| `superapi.buzz` | `/api/ratio_config` 权限错误导致端点状态异常 |
| `tk.shour.bond` | Key/模型状态不完整；管理地址与页面声明 Relay 地址须保持分离 |
| `wdai.site` | `/api/ratio_config` 权限错误导致端点状态异常 |
| `www.aiapibank.com` | Key/模型状态不完整；旧 Refresh Token 无效 |
| `www.sorry.ink` | `/api/ratio_config` 权限错误；Key/模型读取遇到安全验证 |

初始截图用于问题分类，不作为本轮成功证据。重新登录后必须重新读取上游资源；不得把数据库旧状态当作本轮同步结果。

## 实现与本地验证

本轮发布包含已核对的平台站点同步边界修复：

- New API 的 `/api/ratio_config` 是可选诊断；有效 `/api/pricing` 信息用于端点能力判断，重复 Endpoint 去重。
- New API 子密钥模型必须由对应完整 Key 的 Relay 探测确认；账号级模型目录不冒充单 Key 能力。
- Sub2API 管理地址与页面声明 Relay 地址分离，单 Key 探测仅使用该 Key 的 Bearer，不携带管理会话请求头；兼容 `/v1/models` 到 `/models` 回退。
- 六类资源状态每轮补齐；只有事务开始前已有成功资源才可报告使用历史快照。权限、安全验证和部分失败不转换为密钥不存在，并保留最近成功快照。
- 密码同步每次重新登录；浏览器 Capture 与后台密码会话的清理边界不变。

代码事实入口：`service/platform_site_auth_flow.go`、`service/upstream_site.go`、`service/upstream_site_adapters.go`、`model/platform_site_resources.go`、`controller/upstream_channel.go`。New API、Sub2API 和 all-api-hub 参考源码已按本机项目约定核对。

| 验证项 | 命令/证据 | 结果 |
| --- | --- | --- |
| 平台站点定向 Service 测试 | `go test ./service -run 'PlatformSite|NewAPI|Sub2API' -count=1` | 通过 |
| Controller/Model 定向测试 | `go test ./controller ./model -run 'PlatformSite|UpstreamChannel|Channel' -count=1` | 通过 |
| 根模块无执行测试编译 | `go test ./... -run '^$'` | 通过 |
| 数据库/Schema 变更 | 无新增字段、迁移或数据库行为变更 | 不适用 |

## 发布与远端更新

| 项目 | 结果 |
| --- | --- |
| 发布 commit | 待记录 |
| `v0.2.8` tag | 待创建并推送；不得覆盖已有 tag |
| GitHub Release 与构建工作流 | 待标签触发并核验 |
| Docker amd64/arm64 镜像、manifest 与签名 | 待工作流完成并核验 |
| Electron 工作流 | 待标签触发并核验 |
| 远端更新检查页“应用更新” | 待执行 |
| 远端更新任务终态及服务恢复 | 待确认 |

远端操作只在确认发布产物完成后进行。更新后需确认远端实际版本为本轮目标版本，再进入渠道列表逐一验收。

## 逐站重新登录与资源结果

各站点按渠道编辑页中核对的管理地址匹配，不仅依赖渠道名称。若需浏览器登录，使用独立临时浏览器上下文，仅将凭据填入对应上游登录表单并提交；不导出或持久化浏览器配置文件，不修改渠道启用、代理或路由设置。

| 站点 | 重新登录 | identity | groups | endpoints | usage | keys | models | 本轮结论/脱敏诊断 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `gpttap.top` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `134.175.71.62` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `988665.xyz` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `ahriapi.com` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `ai.hexuan.cc` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `ai.lt4net.org` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `api.apichatgpt.top` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `api.luciferai.cc` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `api.lyy.team` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `dahlo.live` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `hhw1231.com` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `proxygpt.cc.cd` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `superapi.buzz` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `tk.shour.bond` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `wdai.site` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `www.aiapibank.com` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |
| `www.sorry.ink` | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待验证 | 待远端操作 |

## 阻塞与残余风险

尚未执行远端更新和逐站本轮同步，因此目前没有站点可记录为通过，也尚未确认实际阻塞站点。后续如遇账号密码失效或人工安全验证，按站点记录脱敏错误类别和管理员需完成的动作；相关资源不得记为 `success`。

截图及浏览器网络记录不纳入仓库。最终更新本报告时，记录每站的同步时间、六项状态、是否使用快照及必要的脱敏诊断，不保存任何可用凭据或完整上游响应。
