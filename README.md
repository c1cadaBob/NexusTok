<div align="center">

![NexusTok](/web/public/logo.png)

# NexusTok

🍥 **新一代大模型网关与AI资产管理系统**

<p align="center">
  简体中文 |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.md">English</a> |
  <a href="./README.fr.md">Français</a> |
  <a href="./README.ja.md">日本語</a>
</p>

<p align="center">
  <a href="https://raw.githubusercontent.com/c1cadabob/nexustok/main/LICENSE">
    <img src="https://img.shields.io/github/license/c1cadabob/nexustok?color=brightgreen" alt="license">
  </a><!--
  --><a href="https://github.com/c1cadabob/nexustok/releases/latest">
    <img src="https://img.shields.io/github/v/release/c1cadabob/nexustok?color=brightgreen&include_prereleases" alt="release">
  </a><!--
  --><a href="https://hub.docker.com/r/c1cadabob/nexustok">
    <img src="https://img.shields.io/badge/docker-dockerHub-blue" alt="docker">
  </a>
</p>

<p align="center">
  <a href="#-快速开始">快速开始</a> •
  <a href="#-主要特性">主要特性</a> •
  <a href="#-部署">部署</a>
</p>

</div>

## 📝 项目说明

> [!IMPORTANT]
> - 本项目仅面向合法授权的 AI API 网关、组织内部鉴权、多模型管理、用量统计、成本核算和私有化部署场景。
> - 使用者必须合法取得上游 API Key、账号、模型服务或接口权限，并遵守上游服务条款及适用法律法规。
> - 使用者应确保其使用方式符合上游服务条款及适用法律法规。
> - 面向公众提供生成式人工智能服务时，使用者应遵守[《生成式人工智能服务管理暂行办法》](http://www.cac.gov.cn/2023-07/13/c_1690898327029107.htm)等监管要求，自行完成所在司法辖区要求的备案、许可、内容安全、实名、日志留存、税务和上游授权等合规义务。

---

## 🚀 快速开始

### 使用 Docker Compose（推荐）

```bash
# 克隆项目
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok

# 一键部署生产环境（PostgreSQL + Redis）
bash scripts/deploy.sh
```

脚本首次运行会生成权限为 `0600` 的 `.env`，自动写入随机的
`POSTGRES_PASSWORD` 和 `REDIS_PASSWORD`；后续运行不会覆盖已有密码。
生产应用端口为 `3030`，PostgreSQL 和 Redis 只加入 Compose 内部网络，不映射宿主机端口。

**或手动启动：**

```bash
# 如需预设密码，创建 .env：
# POSTGRES_PASSWORD=请填写随机密码
# REDIS_PASSWORD=请填写随机密码

docker compose config
docker compose pull
docker compose up -d
```

<details>
<summary><strong>使用 Docker 命令</strong></summary>

单条 `docker run` 命令不会启动 PostgreSQL 和 Redis，不能作为完整的生产部署入口。
下面的命令仅用于单容器兼容模式：未设置 `SQL_DSN` 时使用 SQLite，未设置
`REDIS_CONN_STRING` 时关闭 Redis。

```bash
mkdir -p /opt/nexustok/data /opt/nexustok/logs
docker run --name nexustok -d --restart always \
  -p 3030:3030 \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest
```

> **注意：** SQLite 不会自动迁移到 PostgreSQL。已有生产数据切换到 PostgreSQL
> 前必须先完成备份，并单独执行经过验证的数据迁移。

</details>

---

## 🔧 开发环境

### 热更新开发环境（推荐用于本地开发）

```bash
# 启动热更新开发环境
bash scripts/dev-start.sh
```

**特性：**
- ✅ Go 代码自动热更新（基于 Air）
- ✅ 前端代码自动热更新（基于 Vite HMR）
- ✅ PostgreSQL + Redis 完整开发环境
- ✅ 独立的数据卷，不影响生产环境
- ✅ 实时日志输出

**访问地址：**
- API 服务：http://localhost:3030
- 前端开发：http://localhost:5173

**常用命令：**
```bash
# 查看后端日志
docker logs -f nexustok-dev-api

# 查看前端日志
docker logs -f nexustok-dev-frontend

# 停止服务
bash scripts/dev-stop.sh
```

📖 完整的开发环境文档请参考 [DEV-SETUP.md](DEV-SETUP.md)

---

🎉 生产 Compose 部署完成后，访问 `http://localhost:3030` 即可使用！

> [!WARNING]
> 将本项目作为面向公众的生成式 AI 服务或 API 转售服务运营时，使用者应先完成备案、内容安全、实名、日志留存、税务、支付和上游授权等合规义务。

📖 更多部署方式请参考 [部署指南](https://docs.nexustok.ai/zh/docs/installation)

## ✨ 主要特性

> 详细特性请参考 [特性说明](https://docs.nexustok.ai/zh/docs/guide/wiki/basic-concepts/features-introduction)

### 🎨 核心功能

| 特性 | 说明 |
|------|------|
| 🎨 全新 UI | 现代化的用户界面设计 |
| 🌍 多语言 | 支持中文、英文、法语、日语 |
| 🔄 数据兼容 | 完全兼容原版 One API 数据库 |
| 📈 数据看板 | 可视化控制台与统计分析 |
| 🔒 权限管理 | 令牌分组、模型限制、用户管理 |

### 💰 授权用量与成本管理

- ✅ 合法授权场景下的内部充值与额度分配（易支付、Stripe）
- ✅ 组织内按次、按量或缓存命中成本核算
- ✅ 支持 OpenAI、Azure、DeepSeek、Claude、Qwen 等模型的缓存计费统计
- ✅ 面向内部管理或企业客户的灵活计费策略配置

### 🔐 授权与安全

- 😈 Discord 授权登录
- 🤖 LinuxDO 授权登录
- 📱 Telegram 授权登录
- 🔑 OIDC 统一认证
- 🔍 Key 查询使用额度（配合 [NexusTok-key-tool](https://github.com/c1cadabob/nexustok-key-tool)）

### 🚀 高级功能

**API 格式支持：**
- ⚡ [OpenAI Responses](https://docs.nexustok.ai/zh/docs/api/ai-model/chat/openai/create-response)
- ⚡ [OpenAI Realtime API](https://docs.nexustok.ai/zh/docs/api/ai-model/realtime/create-realtime-session)（含 Azure）
- ⚡ [Claude Messages](https://docs.nexustok.ai/zh/docs/api/ai-model/chat/create-message)
- ⚡ [Google Gemini](https://docs.nexustok.ai/api/google-gemini-chat)
- 🔄 [Rerank 模型](https://docs.nexustok.ai/zh/docs/api/ai-model/rerank/create-rerank)（Cohere、Jina）

**智能路由：**
- ⚖️ 渠道加权随机
- 🔄 失败自动重试
- 🚦 用户级别模型限流

**格式转换：**
- 🔄 **OpenAI Compatible ⇄ Claude Messages**
- 🔄 **OpenAI Compatible → Google Gemini**
- 🔄 **Google Gemini → OpenAI Compatible** - 仅支持文本，暂不支持函数调用
- 🚧 **OpenAI Compatible ⇄ OpenAI Responses** - 开发中
- 🔄 **思考转内容功能**

**Reasoning Effort 支持：**

<details>
<summary>查看详细配置</summary>

**OpenAI 系列模型：**
- `o3-mini-high` - High reasoning effort
- `o3-mini-medium` - Medium reasoning effort
- `o3-mini-low` - Low reasoning effort
- `gpt-5-high` - High reasoning effort
- `gpt-5-medium` - Medium reasoning effort
- `gpt-5-low` - Low reasoning effort

**Claude 思考模型：**
- `claude-3-7-sonnet-20250219-thinking` - 启用思考模式

**Google Gemini 系列模型：**
- `gemini-2.5-flash-thinking` - 启用思考模式
- `gemini-2.5-flash-nothinking` - 禁用思考模式
- `gemini-2.5-pro-thinking` - 启用思考模式
- `gemini-2.5-pro-thinking-128` - 启用思考模式，并设置思考预算为128tokens
- 也可以直接在 Gemini 模型名称后追加 `-low` / `-medium` / `-high` 来控制思考力度（无需再设置思考预算后缀）

</details>

## 🚢 部署

> [!TIP]
> **最新版 Docker 镜像：** `c1cadabob/nexustok:latest`
>
> **v0.2.3 镜像：** `c1cadabob/nexustok:v0.2.3`

### 📋 部署要求

| 组件 | 要求 |
|------|------|
| **生产默认数据库** | PostgreSQL 15（应用同时兼容 PostgreSQL ≥ 9.6） |
| **生产默认缓存** | Redis 7（Compose 内部网络） |
| **兼容数据库** | MySQL ≥ 5.7.8、SQLite |
| **容器引擎** | Docker / Docker Compose |
| **系统架构** | 仅支持 64 位系统（amd64 / arm64），不支持 32 位系统 |

> **💡 生产环境建议：**
> - 使用仓库中的 Docker Compose 和 `bash scripts/deploy.sh`，默认部署 PostgreSQL + Redis
> - 生产对外端口为 `3030`；PostgreSQL 和 Redis 默认不暴露宿主机端口
> - `.env` 中的数据库和 Redis 密码由部署脚本首次生成，也可以预先安全提供
> - 多节点部署必须使用共享数据库（PostgreSQL/MySQL）和 Redis
> - SQLite 适合单容器、开发和测试；不会自动迁移到 PostgreSQL

### ⚙️ 环境变量配置

<details>
<summary>常用环境变量配置</summary>

| 变量名 | 说明                                                           | 默认值 |
|--------|--------------------------------------------------------------|--------|
| `SESSION_SECRET` | 鉴权签名密钥；所有节点必须保持一致                                           | - |
| `SESSION_COOKIE_SECURE` | `false`/未配置时关闭 refresh/logout OriginGuard 以兼容本地 HTTP 开发代理；`true` 时启用 Secure Cookie 和严格 Origin 校验 | `false` |
| `SESSION_COOKIE_TRUSTED_URL` | Secure 模式必填：允许调用 refresh/logout 的精确 HTTPS Origin，多个用英文逗号分隔；不是 relay CORS 白名单 | - |
| `TRUSTED_PROXIES` | 未配置/留空时信任回环、RFC1918 和 IPv6 ULA 并输出启动告警；`none` 不信任任何代理；显式代理 IP/CIDR 列表完全替代默认值 | `127.0.0.0/8, ::1, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7` |
| `USER_SESSION_ACTIVE_LIMIT` | 单用户最大活跃登录 Session 数 | `50` |
| `USER_SESSION_ISSUANCE_LIMIT` | 单用户在签发窗口内可创建的 Session 总数，包含已撤销 Session | `100` |
| `USER_SESSION_ISSUANCE_WINDOW_SECONDS` | Session 签发计数窗口（秒）；高于 revoked 保留期时自动钳制 | `86400` |
| `USER_SESSION_REVOKED_RETENTION_DAYS` | revoked Session 用于审计和签发计数的保留天数 | `7` |
| `USER_SESSION_HOURLY_ALERT_THRESHOLD` | 全局每小时 Session 签发告警阈值；只告警，不拒绝登录 | `5000` |
| `CRYPTO_SECRET` | 缓存键 HMAC 密钥；共享 Redis 的节点必须使用相同有效值 | 默认跟随 `SESSION_SECRET` |
| `SQL_DSN` | 数据库连接字符串                                                     | - |
| `REDIS_CONN_STRING` | Redis 连接字符串                                                  | - |
| `STREAMING_TIMEOUT` | 流式超时时间（秒）                                                    | `300` |
| `STREAM_SCANNER_MAX_BUFFER_MB` | 流式扫描器单行最大缓冲（MB），图像生成等超大 `data:` 片段（如 4K 图片 base64）需适当调大 | `64` |
| `MAX_REQUEST_BODY_MB` | 请求体最大大小（MB，**解压后**计；防止超大请求/zip bomb 导致内存暴涨），超过将返回 `413` | `32` |
| `AZURE_DEFAULT_API_VERSION` | Azure API 版本                                                 | `2025-04-01-preview` |
| `ERROR_LOG_ENABLED` | 错误日志开关                                                       | `false` |
| `PYROSCOPE_URL` | Pyroscope 服务地址                                            | - |
| `PYROSCOPE_APP_NAME` | Pyroscope 应用名                                        | `NexusTok` |
| `PYROSCOPE_BASIC_AUTH_USER` | Pyroscope Basic Auth 用户名                        | - |
| `PYROSCOPE_BASIC_AUTH_PASSWORD` | Pyroscope Basic Auth 密码                  | - |
| `PYROSCOPE_MUTEX_RATE` | Pyroscope mutex 采样率                               | `5` |
| `PYROSCOPE_BLOCK_RATE` | Pyroscope block 采样率                               | `5` |
| `HOSTNAME` | Pyroscope 标签里的主机名                                          | `NexusTok` |

📖 **完整配置：** [环境变量文档](https://docs.nexustok.ai/zh/docs/installation/config-maintenance/environment-variables)

</details>

### 🖥️ 单机部署（PostgreSQL + Redis）

适用于一台 64 位 Linux 主机。准备 Docker Engine、Docker Compose v2、Git 和
`curl`，防火墙只开放 `3030`；使用 HTTPS 反向代理时只开放代理使用的 `80/443`。
PostgreSQL `5432` 和 Redis `6379` 默认只在 Compose 内部网络可访问，不要开放到公网。

```bash
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok
bash scripts/deploy.sh
```

首次执行会创建权限为 `0600` 的 `.env`，随机生成 `POSTGRES_PASSWORD` 和
`REDIS_PASSWORD`；后续执行不会覆盖已有密码。脚本会先执行
`docker compose config`，再拉取 `c1cadabob/nexustok:latest`，启动 PostgreSQL、
Redis 和 NexusTok，并等待三个健康检查通过。

```bash
docker compose ps
curl http://127.0.0.1:3030/api/status
curl http://127.0.0.1:3030/api/setup
```

确认状态接口成功后，访问 `http://服务器地址:3030`，按设置向导完成初始化并创建管理员
账户。生产端口是 `3030`；反向代理必须转发 SSE 流、WebSocket、长连接和
`X-Forwarded-*` 头。使用 HTTPS 时设置 `SESSION_COOKIE_SECURE=true`，并将
`SESSION_COOKIE_TRUSTED_URL` 设置为实际的 HTTPS Origin；`TRUSTED_PROXIES` 只填写
可信反向代理的 IP/CIDR，不要把任意公网地址加入信任列表。

单容器命令只代表 SQLite/无 Redis 兼容模式，不会启动 PostgreSQL 或 Redis：

```bash
mkdir -p /opt/nexustok/data /opt/nexustok/logs
docker run --name nexustok -d --restart always \
  -p 3030:3030 \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:v0.2.3
```

升级前先备份 `.env`、`/opt/nexustok/data`、`/opt/nexustok/logs` 和 PostgreSQL
named volume；优先使用一致性导出：

```bash
docker compose exec -T postgres pg_dump -U root nexustok > nexustok-$(date +%F).sql
cp .env /secure-backup/nexustok.env
docker compose pull
docker compose up -d
```

回滚时将 Compose 的 `nexustok` 镜像固定为上一已验证的版本标签，执行
`docker compose up -d`，并保留数据库备份。
不要删除 PostgreSQL named volume。SQLite 文件不会自动迁移到 PostgreSQL；已有生产
数据切换前必须备份，并单独完成经过验证的数据迁移。

常见问题：`3030` 被占用时停止冲突进程或修改反向代理入口；PostgreSQL/Redis 不健康
时查看 `docker compose logs postgres redis`，检查 `.env` 密码是否为空或被改动；
权限不足时检查 `/opt/nexustok/data`、`/opt/nexustok/logs` 的属主和 SELinux/AppArmor；
镜像架构不匹配时确认主机为 `amd64` 或 `arm64`；SSE/WebSocket 断开时检查代理超时和
Upgrade 头；Docker socket 等同宿主机 Docker 管理权限，只应在可信管理员实例中挂载。

故障排查参考：[常见问题](https://docs.nexustok.ai/zh/docs/support/faq)。

### 🌐 多机部署

多机部署不要在每个应用节点启动本文件内置的 PostgreSQL 和 Redis。应准备一套受
内网访问控制保护的共享 PostgreSQL 和共享 Redis，并为外部连接启用 TLS。每个应用节点
使用相同的 `SQL_DSN`、`REDIS_CONN_STRING`、`SESSION_SECRET` 和 `CRYPTO_SECRET`，
但使用唯一的 `NODE_NAME`。

主节点不设置 `NODE_TYPE=slave`，负责数据库迁移和系统任务；从节点设置
`NODE_TYPE=slave`，只提供请求服务。所有节点必须时钟同步，负载均衡器把流量转发到
各节点，并使用 `/api/status` 做健康检查；不健康节点应先摘除再处理。

每个节点可通过外部数据库/Redis环境文件启动应用容器：

```bash
docker run --name nexustok-node-1 -d --restart always \
  -p 3030:3030 \
  --env-file /etc/nexustok/node.env \
  -e NODE_NAME=node-1 \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:v0.2.3
```

`node.env` 至少包含 `SQL_DSN`、`REDIS_CONN_STRING`、`SESSION_SECRET` 和
`CRYPTO_SECRET`；从节点再增加 `NODE_TYPE=slave`。主节点迁移完成并通过健康检查后，
按“摘除一个从节点、更新、检查、重新加入”的顺序滚动升级，最后处理主节点。回滚前
确认应用版本与数据库迁移兼容，并准备恢复 PostgreSQL 备份。

共享 Redis 可共享 Session、限流和缓存控制面；每节点独立 Redis 会造成状态传播延迟
和节点级限流；不使用 Redis 时 Session 回源数据库、限流退回进程内存，集群不具备
全局一致的限流计数。Docker socket、节点本地日志和 `/data` 不会自动成为跨节点共享
存储；集中日志、共享文件或任务产物必须另行设计。

### 🔄 渠道重试与缓存

**重试配置：** `设置 → 运营设置 → 通用设置 → 失败重试次数`

**缓存配置：**
- `REDIS_CONN_STRING`：Redis 缓存（推荐）
- `MEMORY_CACHE_ENABLED`：内存缓存

## 📜 许可证

本项目采用 [GNU Affero 通用公共许可证 v3.0 (AGPLv3)](./LICENSE) 授权。

本项目为开源项目，在 [One API](https://github.com/songquanpeng/one-api)（MIT 许可证）的基础上进行二次开发。

如果您所在的组织政策不允许使用 AGPLv3 许可的软件，或您希望规避 AGPLv3 的开源义务，请发送邮件至：[support@c1cadabob.dev](mailto:support@c1cadabob.dev)

<div align="center">

### 💖 感谢使用 NexusTok

如果这个项目对你有帮助，欢迎给我们一个 ⭐️ Star！

**[问题反馈](https://github.com/c1cadabob/nexustok/issues)** • **[最新发布](https://github.com/c1cadabob/nexustok/releases)**

<sub>Built with ❤️ by c1cadaBob</sub>

</div>
