# 密钥调度策略与日志可观测性

> 文档状态：代码事实基线
> 事实基线日期：2026-09-25
> 主要代码来源：`model/routing_key.go`、`model/upstream_routing.go`、`middleware/distributor.go`、`service/channel_select.go`、`controller/channel-test.go`、`model/channel_cache.go`
> 关联架构文档：[`docs/architecture/relay-routing-and-conversion.md`](architecture/relay-routing-and-conversion.md)、[`docs/architecture/provider-capability-matrix.md`](architecture/provider-capability-matrix.md)、[`docs/architecture/implementation-deviations.md`](architecture/implementation-deviations.md)

本文档整理 NexusTok 当前密钥调度策略、迁移前历史行为、统一 `key_id` 后的候选选择、重试切换、固定渠道/显式密钥测试、模型获取与管理员密钥配置，以及使用日志中密钥诊断字段的可见性。

本文档是密钥调用方式和调度规则的同步基准。凡是相关代码行为发生变化，都必须在同一个功能提交中更新本文档，并核对接口参数、候选过滤条件、回退行为和管理员可见字段。

## 1. 历史调度策略

统一密钥 ID 迁移前，系统里存在两类不同的密钥来源：

1. **普通密钥渠道**
   - 单密钥渠道直接使用 `channels.key` 作为请求凭据。
   - 多密钥渠道仍把所有密钥存放在 `channels.key`，按换行或 Vertex AI JSON 数组拆分。
   - 多密钥状态存放在 `channels.channel_info`：
     - `multi_key_status_list` 记录某个 `key_index` 的启用、手动禁用或自动禁用状态；
     - `multi_key_disabled_reason` 和 `multi_key_disabled_time` 记录禁用原因和时间；
     - `multi_key_mode=random` 时在启用密钥中随机选择；
     - `multi_key_mode=polling` 时使用渠道级轮询游标选择下一个启用密钥。
   - 调度时普通渠道先按渠道优先级进入候选，渠道内部多密钥选择不会形成全局可观察的密钥身份。

2. **平台站点渠道**
   - 平台站点账号保存在 `platform_site_accounts`。
   - 站点同步出的子密钥保存在 `upstream_keys`，每条子密钥有自己的状态、模型能力、倍率、权重、额度、过期时间和最后使用时间。
   - 平台站点调度时会展开子密钥候选，按子密钥 `key_priority` 和权重选择。
   - 历史日志中仅平台站点能看到 `upstream_key_id`，普通多密钥只能看到 `multi_key_index`，不能稳定定位到某一条具体密钥。

历史策略的问题是：普通密钥渠道和平台站点子密钥不是同一套身份体系，普通多密钥只有索引没有全局 ID；当密钥被追加、删除或重排后，日志里的索引不再是稳定排查线索。

## 2. 统一密钥身份

迁移后，所有可调度密钥都有统一的全局数字 `key_id`。该 ID 来自 `routing_keys.id`，管理员后续可用它在使用日志、测试弹窗、密钥管理面板和数据库中定位同一条密钥。

### 2.1 全局身份表

`routing_keys` 是统一身份表：

- `id`：全局 `key_id`，自增数字；
- `channel_id`：所属渠道；
- `source`：密钥来源；
  - `key_channel`：普通密钥渠道的密钥；
  - `platform_site`：平台站点同步出的子密钥；
- `source_ref_id`：来源表内部记录 ID。

### 2.2 普通密钥渠道

普通密钥渠道的真实凭据迁移到 `channel_keys`：

- `routing_key_id`：对应的全局 `key_id`；
- `channel_id`：所属渠道；
- `key_index`：兼容旧多密钥索引；
- `secret_ciphertext`：加密后的真实密钥；
- `secret_fingerprint`：密钥指纹；
- `status`、`disabled_reason`、`disabled_time`：密钥状态与禁用信息；
- `key_priority`：密钥优先级，服务端限制为 `0..99`；
- `conversion_ratio`、`weight`、`weight_override`：倍率与权重；
- `last_used_at`：最后使用时间。

迁移后 `channels.key` 会被清空，生产调度、手动测试、余额查询、模型同步和任务轮询都不再把 `channels.key` 当作凭据来源。旧列仅作为升级过程中的临时兼容输入：如果发现历史数据尚未迁移，系统会把它迁移到 `channel_keys` 后再使用。

### 2.3 平台站点渠道

平台站点继续使用 `upstream_keys` 保存站点子密钥详情，并新增 `routing_key_id`：

- `upstream_keys.id` 仍是平台站点密钥表内部 ID；
- `upstream_keys.routing_key_id` 才是统一调度和日志使用的全局 `key_id`；
- 站点同步新增密钥时创建新的 `routing_keys`；
- 已有密钥重新同步时保留原 `routing_key_id`；
- 缺失或恢复的密钥不丢失身份，除非该来源记录被删除。

平台站点子密钥的 Routing Key 关系必须同时满足：

```text
upstream_keys.routing_key_id = routing_keys.id
upstream_keys.channel_id = routing_keys.channel_id
routing_keys.source = "platform_site"
routing_keys.source_ref_id = upstream_keys.id
```

启动迁移和运行时读取都会校验这组关系。`routing_key_id` 为零、目标不存在、
目标属于其他渠道、来源不是 `platform_site` 或 `source_ref_id` 不匹配时，系统优先
查找当前渠道和当前 `upstream_keys.id` 的正确 Routing Key，找不到则创建并更新
当前子密钥。旧的错误 Routing Key 不删除。这样既能修复历史跨渠道引用，也不会把
仍被其他记录使用的 Routing Key 误删。

## 3. 迁移规则

数据库迁移会在表结构自动迁移后执行：

1. 为缺少 `routing_key_id` 的 `upstream_keys` 创建或复用 `routing_keys`。
2. 对 `channels.key` 非空且 `upstream_kind=key_channel` 的渠道，按历史拆分规则生成 `channel_keys` 和 `routing_keys`。
3. 迁移旧多密钥状态：
   - 旧 `key_index` 保留为 `channel_keys.key_index`；
   - 旧禁用状态、原因和时间迁移到对应 `channel_keys` 字段；
   - 旧渠道级 `key_priority`、`conversion_ratio`、`key_weight_override` 复制到每条迁移出的密钥。
4. 迁移完成后清空对应渠道的 `channels.key`。
5. 迁移是幂等的：
   - 已存在 `channel_keys` 的渠道不会重复创建密钥；
   - 已存在 `routing_key_id` 的平台子密钥不会重新分配 ID；
   - 重复启动不会重复生成身份记录。

## 4. 统一调度策略

迁移后，调度候选不再是单纯“渠道”，而是“渠道 + 密钥”的组合。每个候选都会解析成统一的 `RoutableKey`：

- `key_id`：全局密钥 ID；
- `key_source`：`key_channel` 或 `platform_site`；
- `channel_id`：所属渠道；
- `secret`：解密后的真实请求凭据；
- `key_priority`：密钥优先级；
- `key_weight`：实际参与随机选择的权重；
- `effective_priority`：有效优先级；
- 兼容诊断字段：
  - 普通多密钥保留 `multi_key_index`；
  - 平台站点保留 `upstream_key_id`。

### 4.1 有效优先级

有效优先级计算公式为：

```text
effective_priority = channel_priority * 100 + key_priority
```

其中：

- `channel_priority` 来自渠道优先级；
- `key_priority` 来自具体密钥，服务端限制为 `0..99`；
- 这样同一渠道内的密钥可以在渠道优先级范围内做精细排序，但不会越过下一个渠道优先级区间。

### 4.2 权重选择

同一个 `effective_priority` 下，系统按密钥权重做加权随机选择：

- 平台站点密钥继续根据转换倍率自动计算权重，并允许管理员覆盖；
- 普通密钥渠道密钥迁移后也有自己的权重；
- 权重越高，被选中的概率越大；
- 如果同一有效优先级内所有密钥权重都为 `0`，系统退化为等概率随机选择，并记录系统告警，便于管理员发现异常配置。

### 4.3 普通密钥渠道候选

普通密钥渠道中，每条启用的 `channel_keys` 都是独立候选：

- 模型能力继承父渠道；
- 分组能力继承父渠道；
- 渠道状态必须启用；
- 密钥状态必须启用；
- 密钥健康状态不能是“禁用”或“失效”；
- 真实请求凭据来自 `channel_keys.secret_ciphertext` 解密结果。

### 4.4 平台站点候选

平台站点渠道继续按站点快照和子密钥能力过滤：

- 父渠道必须启用；
- 站点快照必须可用；
- 子密钥状态必须启用；
- 子密钥不能过期；
- 子密钥剩余额度不能为 0 或负数；
- 子密钥模型能力必须已同步；
- 子密钥必须支持请求模型；
- 子密钥凭据必须可解密。
- 子密钥健康状态不能是“禁用”或“失效”。

### 4.5 密钥健康状态

密钥健康状态保存在 `routing_key_healths`，按全局 `routing_key_id` 维护最近 5 次样本窗口：

- `routing_key_id`：全局密钥 ID，主键；
- `channel_id` 和 `source`：定位密钥所属渠道和来源；
- `samples_json`：最近最多 5 条样本，使用 TEXT JSON 保存；
- `sample_count`、`success_count`：当前窗口样本数和成功次数；
- `max_first_latency_ms`、`last_first_latency_ms`：窗口内最大首字/首包延迟和最近一次延迟；
- `last_sample_at`、`updated_at`：最近样本时间和记录更新时间。

样本来自普通 API 请求和已经选中具体 `key_id` 的管理员手动模型测试。成功样本记录首字/首包延迟；失败样本记录失败结果。已有密钥没有健康记录时默认显示“启用”，保持兼容并继续参与调度。

健康状态定义如下：

| 状态 | 判定规则 | 是否参与自动调度 |
| --- | --- | --- |
| 禁用 | 管理员手动禁用密钥 | 否 |
| 启用 | 无健康记录、样本不足 5 次，或最近样本超过 24 小时 | 是 |
| 正常 | 最近 5 次全部成功，且窗口内最大首字/首包延迟小于 10 秒 | 是 |
| 降级 | 最近 5 次成功次数为 2-4 次，或全部成功但首字/首包延迟达到 10 秒及以上 | 是 |
| 失效 | 未获取到密钥、凭据不可用、过期、额度耗尽、模型能力不可用、站点快照不可用，或最近 5 次成功次数小于等于 1 次 | 否 |

因此生产自动调度会过滤“禁用”和“失效”，但“启用”“正常”“降级”继续按既有 `effective_priority` 和权重规则参与候选选择。管理员显式测试某个密钥时，只要密钥不是手动禁用且凭据仍可用，可以对当前健康状态为“失效”的密钥做恢复性测试；测试成功会写入新的健康样本，后续可恢复为“启用”“降级”或“正常”。

## 5. 重试与分组切换

### 5.1 普通分组

普通分组内，重试按有效优先级从高到低切换：

1. 第一次请求选择最高 `effective_priority` 的候选集合；
2. 第一次重试选择第二高 `effective_priority`；
3. 依次类推；
4. 如果重试索引超过当前分组可用有效优先级数量，则视为该分组无更多可用候选。

### 5.2 `auto` 分组

`auto` 分组保留原有跨分组逻辑，但每个分组内部改为耗尽有效优先级：

1. 先进入第一个自动分组；
2. 在当前分组内按有效优先级从高到低重试；
3. 当前分组没有更多有效优先级后，再进入下一个自动分组；
4. 跨分组重试仍受现有令牌配置、可用分组和请求上下文约束。

### 5.3 固定渠道与亲和命中

固定渠道、任务来源渠道、渠道亲和命中后，不再直接使用父渠道旧凭据，而是在目标渠道内部按同一套密钥规则选择具体密钥：

- 如果目标是普通密钥渠道，则在该渠道的 `channel_keys` 中选择；
- 如果目标是平台站点渠道，则在该渠道的可路由 `upstream_keys` 中选择；
- 若目标渠道没有可用密钥，请求返回无可用密钥错误。

## 6. 显式 key_id 调度与测试

渠道测试接口支持 `key_id` 参数：

- `key_id` 是全局密钥 ID；
- 旧 `upstream_key_id` 仅作为平台站点兼容参数保留；
- 当传入 `key_id` 时，服务端会校验：
  - 密钥存在；
  - 密钥属于当前渠道；
  - 密钥来源与来源表记录匹配；
  - 密钥状态启用；
  - 密钥支持测试模型；
  - 密钥凭据可解密。

校验通过后，测试请求一定携带该密钥的真实凭据，并在日志 `admin_info.key_id` 中记录同一个 ID。校验失败时会返回明确错误，不会静默回退自动路由。

为了兼容历史数据，平台站点显式测试遇到旧的全局 `key_id` 时，会检查当前渠道
是否仍有 `upstream_keys.routing_key_id` 指向该 ID。如果有，先修复该子密钥的
Routing Key 关系，再用修复后的全局 `key_id` 测试；如果无法从当前渠道关系推断
目标子密钥，则返回“所选密钥已失效，请刷新密钥列表后重新选择”，不返回裸的
数据库 `record not found`。`upstream_key_id` 仍只表示 `upstream_keys.id`，不会与
`key_id` 互换。

平台站点前端测试弹窗仍可用“自动路由”入口；当从具体子密钥操作入口打开时，前端发送全局 `key_id`，后端按显式密钥测试。

显式密钥测试不会因为该密钥近期健康状态为“失效”而静默切换到其他密钥；它会继续使用指定密钥做恢复性测试。若密钥已手动禁用、来源不属于当前渠道、凭据缺失或无法解密，则测试会明确失败。

## 7. 手动获取模型

渠道级和子密钥级获取模型使用同一个接口，但密钥选择规则不同：

```text
GET /api/channel/fetch_models/:id
GET /api/channel/fetch_models/:id?key_id=<routing_key_id>
```

### 7.1 渠道级入口

当请求不带 `key_id` 时：

- 保持渠道级获取模型行为；
- 普通渠道使用现有自动路由选择密钥；
- 平台站点使用当前可用子密钥自动选择；
- 渠道编辑抽屉和渠道主行入口不会携带之前子密钥操作残留的 `key_id`。

### 7.2 子密钥入口

当请求带 `key_id` 时：

- `key_id` 必须是 `routing_keys.id`；
- 服务端校验该密钥属于当前渠道；
- 校验密钥状态、过期时间、额度和凭据；
- 使用该密钥解密后的真实凭据构造上游模型请求；
- 请求失败时直接返回当前密钥的错误，不静默切换其他密钥。

普通 OpenAI 兼容渠道、Gemini、Ollama、Advanced Custom 和 Codex 的模型获取路径都遵循这一规则。Codex 在模型请求返回 401 后刷新凭据时，也继续使用当前显式 `key_id`，不会切换到其他普通密钥。

### 7.3 平台站点模型同步

平台站点子密钥获取成功后：

- 返回的模型保存到对应 `upstream_keys.models`；
- `models_synced` 标记为 `true`，即使上游成功返回空模型列表也表示“同步已完成但当前没有模型”；
- 重建该子密钥的真实 `upstream_key_abilities`；
- 按所有子密钥的有效模型重建父渠道模型并集；
- 刷新相关渠道、子密钥和路由缓存。

管理员配置的允许模型列表不会被模型同步覆盖。成功获取到空模型时，子密钥会因为没有有效模型而不可路由，但不会继续被误判为“尚未同步”。

## 8. 平台站点子密钥允许模型

平台站点子密钥可以配置管理员允许使用的模型集合。该配置只限制该子密钥，不改变上游同步得到的真实模型能力：

- `allowed_models = NULL`：未配置限制，允许全部真实同步模型；
- `allowed_models = []`：明确禁止该子密钥的全部模型；
- `allowed_models = ["model-a", "model-b"]`：只允许列表中且真实同步存在的模型。

实际可路由模型为：

```text
effective_models = synced_models ∩ allowed_models
```

未配置限制时，`effective_models` 等于全部真实同步模型。允许列表中的未知模型会被管理接口拒绝；模型名比较继续复用现有模型归一化规则。

管理接口：

```text
PATCH /api/channel/:id/upstream-keys/:keyId
```

请求规则：

- 省略 `allowed_models` 表示不修改；
- `allowed_models: []` 表示明确禁用全部模型；
- `clear_allowed_models: true` 清除限制并恢复允许全部同步模型；
- `allowed_models` 和 `clear_allowed_models` 不能同时出现；
- 变更后立即重建子密钥可路由状态、父渠道模型并集和相关缓存。

模型同步只更新真实模型和真实能力记录，不会把管理员允许列表写入能力表，也不会覆盖允许列表配置。

## 9. 管理员渠道列表的按模型最低倍率

管理员渠道列表支持按指定模型计算最低倍率并排序：

```text
GET /api/channel?model=<model>&sort_by=model_ratio&sort_order=asc
GET /api/channel/search?model=<model>&sort_by=model_ratio&sort_order=asc
```

这项能力只影响管理界面的筛选、显示和排序，不参与实际请求调度，也不会改变密钥选择顺序。

管理员通过渠道列表现有的“按模型筛选”输入框指定最低倍率计算模型。该输入框支持搜索当前可用模型候选，也允许手动输入尚未出现在候选列表中的模型。筛选不含 `*` 或 `?` 时使用大小写不敏感的包含匹配；包含通配符时按完整模型名匹配，`*` 匹配零个或多个字符，`?` 匹配一个字符，例如 `gpt-*` 匹配 `gpt-4o`，需要包含匹配时使用 `*gpt-*`。启用最低倍率排序后，选中的模型只作为倍率计算目标，不再作为渠道或子密钥的前端过滤条件；未启用最低倍率排序时，输入框继续保持渠道和子密钥模型筛选行为。

计算规则：

- 普通密钥渠道只统计包含指定模型的可路由密钥；
- 平台站点只统计支持指定模型且当前可路由的子密钥；
- 可路由密钥会排除“禁用”和“失效”健康状态；
- 同一渠道取符合条件密钥中的最小生效倍率；
- 不支持指定模型、没有可用密钥或没有有效倍率的渠道仍保留，`model_ratio` 为空；
- 升序和降序时空倍率都排在最后；
- 同倍率使用渠道 ID 作为稳定排序条件；
- 未指定模型时使用渠道当前全部可用密钥中的最低倍率。

普通筛选和最低倍率排序使用同一套大小写不敏感的模型匹配规则，但用途不同：普通筛选决定管理列表中的渠道和展开子密钥是否显示；最低倍率排序只把匹配指定模型且当前可路由的密钥纳入倍率计算，展开子密钥表仍显示全部已加载密钥。模型候选使用上游密钥同步模型与 `allowed_models` 的最终有效模型交集；数据库查询只做候选预筛选，最终按模型 token 在应用层确认，并转义 SQL 的 `%`、`_`、`!` 和反斜杠，保证 SQLite、MySQL、PostgreSQL 结果一致。通配符仅用于管理员页面的搜索、展示和倍率计算，不参与真实请求模型匹配或密钥调度。

平台站点的余额和已使用额度以平台同步接口返回的 `balance`、`used_quota` 为准，同时保存到 `PlatformSiteAccount` 和父渠道；执行同步渠道或更新余额时会一起刷新这两个字段。`used_quota` 明确返回 `0` 时仍视为有效上游结果；仅当上游没有返回账号级已用额度时，才将本次同步中明确返回的子密钥已用额度求和作为回退值。管理端状态查询返回后优先展示同步结果，不使用本地请求日志或本地累计值替代。Sub2API 如果使用 `api.` 转发地址，只有同协议、同端口的去 `api.` 候选管理站页面明确回指原始 API 地址时才会自动纠正管理地址；失败时保留原配置并不发送凭据。

平台站点可以使用 `http://`、`localhost`、回环地址、私有 IPv4/IPv6 和内网 DNS
地址。私有地址和 HTTP 仅由平台站点专用同步客户端放行，其他用户可控 URL 仍受
全局 SSRF 规则约束。Sub2API 的管理地址与转发地址分开保存：管理接口使用
`PlatformSiteAccount.base_url`，页面配置发现的 OpenAI 兼容地址使用
`relay_base_url`，父渠道基础地址使用实际转发地址并避免重复 `/v1`。

平台站点的 `password` 认证继续手动输入用户名和密码；密码模式也可以提交
`capture_id`，用于把浏览器采集到的 Access Token/Refresh Token 合并为该密码凭据的
缓存登录态，认证方式仍保持 `password`，不接受 Admin Key 或 Cookie 混入。其他认证
统一通过“自动配置”入口和短期、管理员绑定、Origin 精确匹配、一次性消费的浏览器
Capture Session 采集。自动配置过程中按 Access Token/Refresh Token、Admin Key、
Cookie 的顺序自动选择；采集不到任何可用凭据时失败，不允许手动补填或静默改变认证
类型。前端表单只提交 `capture_id`，凭据最终仍整体加密保存。同步失败继续保留最近
一次成功快照。

平台站点账号同步使用双凭据模型：`password` 模式保留账号密码作为最终恢复凭据，
同时保留 `AccessToken`、`RefreshToken`、`TokenExpiresAt` 和 `UserID` 作为日常登录态。
这些登录态可以来自后台账号密码登录、刷新令牌轮换，或密码模式下明确提交的浏览器
`capture_id`。同步优先验证缓存访问令牌，失效后尝试刷新令牌，两个登录态都失效时
才使用账号密码；`access_token` 模式刷新失败不会读取账号密码。认证期间产生的令牌
更新在快照同步前立即加密保存，快照失败不会丢失令牌轮换结果。该凭据生命周期只决定
平台站点同步能否建立会话，不改变 `key_id`、`upstream_key_id`、Routing Key 或子密钥
候选的语义。
平台站点同步认证失败时保留最近一次成功快照和已有子密钥路由能力；如果 Sub2API
登录接口返回明确的 Turnstile/验证码交互验证错误，后台不会清除这些快照，也不会
继续用网页登录页错误覆盖真实原因。只有登录 API 路由不存在时才尝试备用路径。

标签模式以标签内最小倍率作为标签排序值，按标签分页并返回该页标签下的全部渠道。没有有效倍率的标签排在最后。

前端最低倍率列、渠道卡片和标签聚合优先使用后端计算的 `model_ratio`。指定模型不存在时显示 `-`，不把渠道的其他模型倍率误显示为指定模型倍率。
最低倍率排序启用时，展开的平台站点子密钥表仍展示全部已加载子密钥；排序目标不会被误用为子密钥列表筛选条件。后端只把包含该模型且当前可路由的密钥纳入倍率计算，因此不支持该模型的渠道或密钥不会影响排序，但渠道本身仍保留在列表中。

## 10. 密钥优先级与管理入口

平台站点子密钥和普通多密钥都支持在密钥列表中直接使用加减号调整 `key_priority`：

- 服务端范围统一为 `0..99`；
- 平台站点调用 `PATCH /api/channel/:id/upstream-keys/:keyId`；
- 普通多密钥调用 `POST /api/channel/multi_key/manage`，`action=update_key`；
- 普通多密钥优先使用全局 `key_id`，`key_index` 只作为兼容回退；
- 前端连续点击使用防抖合并请求；
- 更新失败时恢复服务端值。

编辑弹窗仍保留，用于精确输入倍率、权重和允许模型等配置。

平台站点子密钥展开后，子表根区域与渠道主表滚动容器的可视宽度对齐，不随整张渠道宽表扩展。桌面端不再创建独立的子密钥横向滚动容器，渠道主表滚动容器统一承载子密钥宽表的左右移动；普通表格模式使用 `table-container`，分体表头模式使用 `data-table-scroll-container` 作为渠道级滚动基准。子表根区域只限制可视宽度并使用普通相对定位，不使用 `sticky left-0` 固定整张子密钥宽表，因此密钥 ID、名称、密钥、状态、模型、倍率、优先级、权重和同步时间等普通字段会随渠道主表横向滚动。子密钥操作列固定在当前可视区域右侧，并使用不透明背景和较高层级覆盖滚动中的字段内容。纵向滚动时，固定行为只作用于展开子表范围，不提升为页面级固定。子表进入渠道页面时默认折叠，展开状态不写入或读取 localStorage；用户手动展开只在当前页面生命周期内有效。子表不提供多选、全选或批量启停工具栏，保留每个密钥行的单独启用/停用、编辑、测试、余额、拉取模型和删除操作。移动端继续使用移动端子密钥列表自己的布局。

## 11. 无可用密钥与自动禁用

当渠道内没有可用密钥时：

- 普通请求会返回无可用渠道或无可用密钥错误；
- 普通密钥渠道的单条密钥发生可自动禁用错误时，优先自动禁用该 `key_id` 对应的 `channel_keys`；
- 如果普通密钥渠道所有密钥都不可用，父渠道会进入自动禁用状态并关闭能力；
- 平台站点子密钥发生可自动禁用错误时，优先禁用对应 `upstream_keys`，父渠道是否仍可路由取决于其他子密钥是否可用。

手动渠道测试失败不会触发自动禁用；自动健康检查仍沿用原自动禁用策略。

## 12. 使用日志可观测字段

成功请求和可记录的失败请求都会通过 `other.admin_info` 记录管理员诊断字段：

- `key_id`：全局密钥 ID；
- `key_source`：`key_channel` 或 `platform_site`；
- `key_priority`：密钥优先级；
- `key_weight`：本次选择使用的密钥权重；
- `effective_priority`：渠道优先级与密钥优先级合成后的有效优先级；
- `multi_key_index`：普通多密钥兼容索引；
- `upstream_key_id`：平台站点子密钥内部 ID；
- `upstream_key_name`、`upstream_key_ratio`、`upstream_key_weight`：平台站点兼容诊断字段。

这些字段只对管理员可见：

- 后端普通用户日志会剥离整个 `admin_info`；
- 前端也会按管理员视图做防御性隐藏；
- 管理员可在使用日志表格和详情弹窗中查看 key ID 与调度参数。

## 13. 为什么余额不足测试失败以前没有使用日志

历史行为中有两条不同路径：

1. **手动渠道/模型测试失败**
   - 旧实现只在测试成功后写消费日志；
   - 测试失败时直接返回接口错误；
   - 因此“模型测试提示余额不足”这类失败不会进入使用日志。

2. **普通 API 请求预扣费余额不足**
   - 普通请求在预扣费阶段发现用户余额、令牌额度或订阅额度不足时，会返回带 `NoRecordErrorLog` 选项的错误；
   - 这是为了避免用户余额不足产生大量错误日志噪声；
   - 因此普通 API 请求的预扣费余额不足仍不会写错误日志。

本次调整只改变手动渠道/密钥测试失败：

- 手动测试失败会写 `LogTypeError`；
- quota 为 `0`；
- 不触发自动禁用；
- 管理员日志中会记录本次测试使用的 `key_id` 和调度诊断字段。

普通 API 请求的余额不足预扣失败仍保持不写错误日志。

## 与架构文档的关系

本文是密钥身份、候选过滤、优先级/权重、平台站点子密钥、模型获取和日志可观测字段的详细同步基准；架构文档只描述这些规则在完整 Relay 链路中的位置。任何路由或日志行为变更都必须先更新本文，再更新架构文档和偏差登记。

## 变更记录

| 日期 | 变更类型 | 变更前 | 变更后 | 影响范围 | 验证依据 |
| --- | --- | --- | --- | --- | --- |
| 2026-09-25 | 补充架构索引与元信息 | 已有密钥调度详细规则没有统一事实基线和架构入口 | 增加事实基线、代码来源、架构分工和变更记录；保留统一 `key_id`、候选和日志细节 | Routing Key、平台站点、模型获取、测试和管理员日志 | `model/upstream_routing.go`、`model/routing_key.go`、`middleware/distributor.go` 静态核对 |
| 2026-09-25 | 缺陷修复 | 平台子密钥可能引用其他渠道或错误来源的 Routing Key；显式旧 `key_id` 失败时返回裸 `record not found` | 统一校验 `channel_id`、`source`、`source_ref_id` 并在启动/读取/路由时自愈；可恢复的旧 `key_id` 先修复再测试，不可恢复时返回明确失效提示 | 平台站点路由、密钥列表、模型获取、自动测试和指定密钥测试 | `model/routing_key.go`、`model/main.go`、`controller/channel-test.go`、模型回归测试 |
| 2026-09-25 | 缺陷修复 | 平台站点同步每次优先账号密码登录，令牌轮换结果可能在快照失败后丢失 | 账号密码与登录态并存并按缓存优先恢复；认证更新在快照前保存，保持 `key_id` 与 `upstream_key_id` 语义不变 | 平台站点同步、子密钥快照、路由候选和管理员测试 | `service/upstream_site_adapters.go`、`service/upstream_site.go`、`controller/upstream_channel.go`；服务/控制器测试 |
| 2026-09-25 | 缺陷修复 | 渠道 2 的 Turnstile 登录错误会被后续 HTML 路径覆盖，失败时难以判断是否应继续测试或重新采集 | 交互验证错误立即停止路径切换并保留现有快照；仅路由不存在时继续兼容路径，`key_id`/`upstream_key_id` 语义不变 | 平台站点密钥同步、可路由子密钥保留、管理员测试和错误排障 | `service/upstream_site.go`、`service/upstream_site_adapters.go`、`service/upstream_site_test.go` |
| 2026-09-25 | 功能优化 | 密码模式无法把一次浏览器验证后的登录态并入原密码凭据，管理员可能为避开上游验证切换到自动配置认证方式 | 密码模式可携带 `capture_id` 合并 Access Token/Refresh Token 缓存登录态，认证方式、子密钥身份和路由候选语义不变 | 平台站点密码凭据、采集会话、子密钥同步和后续自动恢复 | `controller/upstream_channel.go`、`service/platform_site_capture.go`、`web/src/features/channels/`；前后端回归测试 |
