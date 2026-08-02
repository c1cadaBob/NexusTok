<div align="center">

![nexustok](/web/default/public/logo.png)

# NexusTok

🍥 **新一代大模型网关与 AI 资产管理系统**

<p align="center">
  <a href="./README.md">项目首页</a> |
  <a href="./README.en.md">English</a> |
  <strong>简体中文</strong> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.fr.md">Français</a> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="#-快速开始">快速开始</a> •
  <a href="#-核心能力">核心能力</a> •
  <a href="#-部署">部署</a> •
  <a href="#-文档">文档</a> •
  <a href="#-许可证">许可证</a>
</p>

</div>

## 📝 项目说明

> [!IMPORTANT]
> - NexusTok 仅面向合法授权的 AI API 网关、组织内部鉴权、多提供商路由、用量分析、成本核算、计费和私有化部署场景。
> - 使用者必须合法取得上游 API Key、账号、模型服务或接口权限，并遵守上游服务条款及适用法律法规。
> - 面向公众提供生成式人工智能服务时，使用者应自行完成所在司法辖区要求的备案、许可、内容安全、实名、日志留存、税务、支付和上游授权等合规义务。

NexusTok 将 OpenAI 兼容接口、Claude、Gemini、Azure、AWS Bedrock 等上游 AI 服务聚合到统一 API 之下，并提供账号池、额度与计费、模型定价、系统任务观测、流量分析和管理后台，适合作为自托管的 AI 控制平面。

---

## ✨ 核心能力

| 领域 | 能力 |
|------|------|
| 提供商网关 | 聚合 40+ 上游 AI 提供商，支持 OpenAI 兼容、Claude、Gemini、Azure、AWS Bedrock、Responses 和任务类接口。 |
| 账号池 | 原生凭据池、账号分组、OAuth/设备授权、健康检查、使用日志、状态审计和渠道级账号池绑定。 |
| 计费与额度 | Token/额度核算、模型定价组、动态计费表达式、订阅套餐、钱包余额、兑换码和饱和保护额度计算。 |
| 治理与权限 | JWT、OAuth、WebAuthn/Passkeys、管理员权限 catalog、路由级 Authz enforcement、敏感字段保护和管理审计日志。 |
| 运维能力 | SystemTask runner、跨节点任务锁、系统实例心跳、日志清理、渠道测试、内置模型仓库、上游模型同步、账号池检测和异步任务轮询。 |
| 数据分析 | 总览看板、模型/用户用量图、Dashboard Flow 桑基图、使用日志、登录审计、额度饱和可观测性和记录导出。 |
| 支付能力 | EPay、Stripe、Creem、Waffo、Waffo Pancake 配置，catalog/pair 绑定、托管支付准备、订阅商品绑定和 webhook 幂等入账。 |
| 前端体验 | React 19 默认控制台、classic 兼容主题、移动端日志/渠道视图、安全富文本渲染、Playground 增强和 en/zh/fr/ja/ru/vi 国际化。 |
| 部署方式 | SQLite、MySQL >= 5.7.8、PostgreSQL >= 9.6，Redis 或内存缓存，Docker Compose、二进制部署、热更新开发环境和 Electron 桌面端打包说明。 |

---

## 🚀 快速开始

### Docker Compose（推荐）

```bash
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok

# 生产使用前请先检查 docker-compose.yml 配置。
nano docker-compose.yml

docker-compose up -d
```

启动后访问 `http://localhost:3000`，按初始化向导完成配置。

<details>
<summary><strong>Docker 命令</strong></summary>

```bash
docker pull c1cadabob/nexustok:latest

docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest
```

`-v ./data:/data` 会把 SQLite 和运行时数据保存到当前目录的 `data` 文件夹。生产部署建议使用绝对路径。`/var/run/docker.sock` 用于系统维护页内自动拉取镜像并重建容器，等同宿主机 Docker 管理权限，只应在可信管理员环境中挂载；不挂载时仍可检查更新，但不能在页面内应用 Docker 更新。

Docker 镜像只包含 NexusTok 应用，不内置 PostgreSQL 服务。单容器默认使用 SQLite；外接 MySQL 或 PostgreSQL 时通过 `SQL_DSN` 指定连接串，例如：

```bash
-e SQL_DSN="postgresql://user:password@host:5432/nexustok?sslmode=disable"
-e REDIS_CONN_STRING="redis://:password@host:6379/0"
```

</details>

---

## 🚢 部署

| 组件 | 要求 |
|------|------|
| 本地数据库 | SQLite，并持久化挂载 `/data` 目录 |
| 远程数据库 | MySQL >= 5.7.8 或 PostgreSQL >= 9.6 |
| 缓存 | 推荐 Redis；小规模部署可使用内存缓存 |
| 高并发推荐 | PostgreSQL 主库 + Redis，消费日志可拆分到独立日志库或 ClickHouse |
| 运行方式 | Docker / Docker Compose 或 Go 二进制部署 |

常用环境变量：

| 变量 | 说明 |
|------|------|
| `SESSION_SECRET` | 多实例部署必须设置，保证会话在节点间一致。 |
| `CRYPTO_SECRET` | 启用 Redis 或共享加密数据时必须设置。 |
| `SQL_DSN` | MySQL 或 PostgreSQL 连接字符串；为空时使用 SQLite。 |
| `LOG_SQL_DSN` | 独立日志库连接字符串；支持 MySQL/PostgreSQL，消费日志可使用 ClickHouse。 |
| `LOG_SQL_CLICKHOUSE_TTL_DAYS` | ClickHouse 消费日志保留天数，`0` 表示不启用 TTL。 |
| `REDIS_CONN_STRING` | Redis 连接字符串。 |
| `REDIS_POOL_SIZE` | Redis 连接池大小；多节点/高并发建议从 `128` 或 `256` 起步压测。 |
| `RELAY_MAX_IDLE_CONNS` | Relay HTTP 客户端最大空闲连接数。 |
| `RELAY_MAX_IDLE_CONNS_PER_HOST` | Relay HTTP 客户端每个上游主机最大空闲连接数。 |
| `RELAY_MAX_CONNS_PER_HOST` | Relay HTTP 客户端每个上游主机最大总连接数，`0` 表示不限制。 |
| `RELAY_RESPONSE_HEADER_TIMEOUT` | Relay 等待上游响应头超时时间（秒），流式响应开始后不受该项限制。 |
| `RELAY_PROXY_CLIENT_CACHE_TTL` | 按代理 URL 缓存 HTTP client 的空闲回收时间（秒）。 |
| `RELAY_PROXY_CLIENT_CACHE_MAX_SIZE` | 按代理 URL 缓存 HTTP client 的最大数量，`0` 表示不限制。 |
| `MAX_REQUEST_BODY_MB` | 解压后的请求体大小限制。 |
| `STREAMING_TIMEOUT` | 流式响应超时时间（秒）。 |
| `STREAM_SCANNER_MAX_BUFFER_MB` | 大型流式片段的扫描缓冲上限。 |

生产部署、热重载开发、二进制部署、主题切换和排障请参考 [启动与部署维护手册](./docs/installation/deployment.md)。

---

## 📚 文档

| 主题 | 链接 |
|------|------|
| 启动与部署维护 | [docs/installation/deployment.md](./docs/installation/deployment.md) |
| 宝塔面板安装 | [docs/installation/BT.md](./docs/installation/BT.md) |
| 渠道高级设置 | [docs/configuration/channel-other-settings.md](./docs/configuration/channel-other-settings.md) |
| 后端 OpenAPI | [docs/openapi/api.json](./docs/openapi/api.json) |
| Relay OpenAPI | [docs/openapi/relay.json](./docs/openapi/relay.json) |
| Electron 桌面端 | [electron/README.md](./electron/README.md) |
| 问题反馈 | [GitHub Issues](https://github.com/c1cadaBob/NexusTok/issues) |
| 最新发布 | [GitHub Releases](https://github.com/c1cadaBob/NexusTok/releases) |

---

## 📜 许可证

NexusTok 采用 [GNU Affero 通用公共许可证 v3.0](./LICENSE) 授权。

本项目基于 [One API](https://github.com/songquanpeng/one-api)（MIT 许可证）进行二次开发。

如果您所在的组织政策不允许使用 AGPLv3 许可的软件，或您需要商业授权，请联系 [support@c1cada.dev](mailto:support@c1cada.dev)。

---

<div align="center">

### 感谢使用 NexusTok

**[项目仓库](https://github.com/c1cadaBob/NexusTok)** • **[问题反馈](https://github.com/c1cadaBob/NexusTok/issues)** • **[最新发布](https://github.com/c1cadaBob/NexusTok/releases)**

<sub>由 c1cada 与贡献者共同构建。</sub>

</div>
