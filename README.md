<div align="center">

![nexustok](/web/default/public/logo.png)

# NexusTok

🍥 **新一代大模型网关与AI资产管理系统**

<p align="center">
  <strong>简体中文</strong> |
  <a href="./README.en.md">English</a> |
  <a href="./README.zh_TW.md">繁體中文</a> |
  <a href="./README.fr.md">Français</a> |
  <a href="./README.ja.md">日本語</a> |
  <a href="./README.zh_CN.md">简体中文完整页</a>
</p>

</div>

---

## 📝 项目说明

> [!IMPORTANT]
> - 本项目仅面向合法授权的 AI API 网关、组织内部鉴权、多模型管理、用量统计、成本核算和私有化部署场景。
> - 使用者必须合法取得上游 API Key、账号、模型服务或接口权限，并遵守上游服务条款及适用法律法规。
> - 使用者应确保其使用方式符合上游服务条款及适用法律法规。
> - 面向公众提供生成式人工智能服务时，使用者应遵守[《生成式人工智能服务管理暂行办法》](http://www.cac.gov.cn/2023-07/13/c_1690898327029107.htm)等监管要求，自行完成所在司法辖区要求的备案、许可、内容安全、实名、日志留存、税务和上游授权等合规义务。

---

## 🚀 快速开始

### 使用 Docker Compose（推荐：PostgreSQL + Redis）

```bash
# 克隆项目
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok

# 编辑 docker-compose.yml 配置；Compose 默认启动 PostgreSQL + Redis，生产前请修改密码和密钥。
nano docker-compose.yml

# 启动服务
docker-compose up -d
```

<details>
<summary><strong>使用 Docker 命令（生产推荐外接 PostgreSQL + Redis）</strong></summary>

```bash
# 拉取最新镜像
docker pull c1cadabob/nexustok:latest

# 本地体验/小规模 fallback：使用 SQLite，数据和日志保存在 /opt/nexustok
mkdir -p /opt/nexustok/data /opt/nexustok/logs
docker rm -f nexustok 2>/dev/null || true

docker run --name nexustok -d --restart always \
  -p 3030:3030 \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest

# 使用 MySQL（外接数据库）
docker run --name nexustok -d --restart always \
  -p 3030:3030 \
  -e SQL_DSN="root:password@tcp(host:3306)/nexustok" \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest

# 生产单容器推荐：使用外接 PostgreSQL + Redis
docker run --name nexustok -d --restart always \
  -p 3030:3030 \
  -e SQL_DSN="postgresql://user:password@host:5432/nexustok?sslmode=disable" \
  -e REDIS_CONN_STRING="redis://:password@host:6379/0" \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest

# 启动后检查
docker ps | grep nexustok
docker logs -f nexustok
curl -sS http://127.0.0.1:3030/api/status
```

> **💡 提示：** 访问地址为 `http://服务器IP:3030`。云服务器还需要在安全组和系统防火墙中放行 TCP `3030` 端口。`/opt/nexustok/data` 保存 SQLite、会话密钥文件和运行时数据，更新容器时不要删除。Docker 镜像只包含 NexusTok 应用本身和内置模型仓库，不应该打包用户、渠道、Token、账号池、日志、session secret 或任何 SQLite / MySQL / PostgreSQL 运行时数据；如果生产环境里看到了“开发服数据”，优先检查是否复用了旧的 `/opt/nexustok/data` 挂载卷、数据库 volume，或者 `SQL_DSN` 仍然指向旧库。需要数据库容器时请使用 Docker Compose，单容器模式则通过 `SQL_DSN` 连接外部 MySQL 或 PostgreSQL。`/var/run/docker.sock` 用于系统维护页内自动拉取镜像并重建容器，等同宿主机 Docker 管理权限，只应在可信管理员环境中挂载；不挂载时仍可检查更新，但不能在页面内应用 Docker 更新。旧部署如果必须继续使用容器内 `3000`，请显式加 `-e PORT=3000 -p 宿主端口:3000`。

</details>

---

## 🚢 部署

> [!TIP]
> **最新版 Docker 镜像：** `c1cadabob/nexustok:latest`

### 📋 部署要求

| 组件 | 要求 |
|------|------|
| **生产主数据库** | PostgreSQL ≥ 9.6（推荐）|
| **兼容数据库** | SQLite、MySQL ≥ 5.7.8；SQLite 适合本地体验和小规模单机部署 |
| **生产缓存** | Redis（推荐）|
| **小规模 fallback** | SQLite + 内存缓存，无需外部数据库和 Redis |
| **高并发推荐** | PostgreSQL 主库 + Redis 热路径，消费日志可拆分到独立日志库或 ClickHouse |
| **容器引擎** | Docker / Docker Compose |

### ⚙️ 环境变量配置

<details>
<summary>常用环境变量配置</summary>

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `SESSION_SECRET` / `SESSION_SECRET_FILE` | 会话密钥或持久化密钥文件（多机部署必须固定） | - |
| `CRYPTO_SECRET` | 加密密钥（Redis 必须） | - |
| `SQL_DSN` | 数据库连接字符串 | - |
| `LOG_SQL_DSN` | 独立日志库连接字符串；支持 MySQL/PostgreSQL，消费日志可使用 ClickHouse | - |
| `LOG_SQL_CLICKHOUSE_TTL_DAYS` | ClickHouse 消费日志保留天数，`0` 表示不启用 TTL | `0` |
| `REDIS_CONN_STRING` | Redis 连接字符串 | - |
| `REDIS_POOL_SIZE` | Redis 连接池大小；多节点/高并发建议从 `128` 或 `256` 起步压测 | `10` |
| `RELAY_MAX_IDLE_CONNS` | Relay HTTP 客户端最大空闲连接数 | `500` |
| `RELAY_MAX_IDLE_CONNS_PER_HOST` | Relay HTTP 客户端每个上游主机最大空闲连接数 | `100` |
| `RELAY_MAX_CONNS_PER_HOST` | Relay HTTP 客户端每个上游主机最大总连接数，`0` 表示不限制 | `0` |
| `RELAY_RESPONSE_HEADER_TIMEOUT` | Relay 等待上游响应头超时时间（秒），流式响应开始后不受该项限制 | `0` |
| `RELAY_PROXY_CLIENT_CACHE_TTL` | 按代理 URL 缓存 HTTP client 的空闲回收时间（秒） | `900` |
| `RELAY_PROXY_CLIENT_CACHE_MAX_SIZE` | 按代理 URL 缓存 HTTP client 的最大数量，`0` 表示不限制 | `4096` |
| `STREAMING_TIMEOUT` | 流式超时时间（秒） | `300` |
| `STREAM_SCANNER_MAX_BUFFER_MB` | 流式扫描器单行最大缓冲（MB），图像生成等超大 `data:` 片段（如 4K 图片 base64）需适当调大 | `64` |
| `MAX_REQUEST_BODY_MB` | 请求体最大大小（MB，**解压后**计；防止超大请求/zip bomb 导致内存暴涨），超过将返回 `413` | `32` |
| `AZURE_DEFAULT_API_VERSION` | Azure API 版本 | `2025-04-01-preview` |
| `ERROR_LOG_ENABLED` | 错误日志开关 | `false` |
| `PYROSCOPE_URL` | Pyroscope 服务地址 | - |
| `PYROSCOPE_APP_NAME` | Pyroscope 应用名 | `nexustok` |
| `PYROSCOPE_BASIC_AUTH_USER` | Pyroscope Basic Auth 用户名 | - |
| `PYROSCOPE_BASIC_AUTH_PASSWORD` | Pyroscope Basic Auth 密码 | - |
| `PYROSCOPE_MUTEX_RATE` | Pyroscope mutex 采样率 | `5` |
| `PYROSCOPE_BLOCK_RATE` | Pyroscope block 采样率 | `5` |
| `HOSTNAME` | Pyroscope 标签里的主机名 | `nexustok` |

📖 **完整配置：** [NexusTok 仓库](https://github.com/c1cadaBob/NexusTok)

</details>

### 🔧 部署方式

📘 **维护手册：** [启动与部署维护手册](./docs/installation/deployment.md) 汇总了生产部署、热重载环境、二进制部署、主题切换和常见排障流程。

<details>
<summary><strong>方式 1：Docker Compose（推荐：PostgreSQL + Redis）</strong></summary>

```bash
# 克隆项目
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok

# 编辑配置
nano docker-compose.yml

# 启动服务
docker-compose up -d
```

</details>

<details>
<summary><strong>方式 2：Docker 命令</strong></summary>

**本地体验/小规模 fallback：使用 SQLite：**
```bash
docker pull c1cadabob/nexustok:latest
mkdir -p /opt/nexustok/data /opt/nexustok/logs
docker rm -f nexustok 2>/dev/null || true

docker run --name nexustok -d --restart always \
  -p 3030:3030 \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest

docker ps | grep nexustok
docker logs -f nexustok
curl -sS http://127.0.0.1:3030/api/status
```

**使用 MySQL：**
```bash
docker run --name nexustok -d --restart always \
  -p 3030:3030 \
  -e SQL_DSN="root:password@tcp(host:3306)/nexustok" \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest
```

**生产单容器推荐：使用外接 PostgreSQL + Redis：**
```bash
docker run --name nexustok -d --restart always \
  -p 3030:3030 \
  -e SQL_DSN="postgresql://user:password@host:5432/nexustok?sslmode=disable" \
  -e REDIS_CONN_STRING="redis://:password@host:6379/0" \
  -e TZ=Asia/Shanghai \
  -e PORT=3030 \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest
```

> **💡 路径说明：**
> - `/opt/nexustok/data:/data` - SQLite、会话密钥文件和运行时数据
> - `/opt/nexustok/logs:/app/logs` - 主服务日志
> - `/var/run/docker.sock:/var/run/docker.sock` - 系统维护页 Docker 自动更新入口；等同宿主机 Docker 管理权限，只给可信管理员环境使用
> - 访问 `http://服务器IP:3030` 前，请确认云服务器安全组已放行 TCP `3030` 端口。
> - Docker 镜像不内置 PostgreSQL / MySQL / Redis，也不包含用户、渠道、Token、账号池或日志等运行时业务数据。生产看到历史数据时，先检查是否复用了旧卷，或者 `SQL_DSN` 仍然指向旧数据库；单容器模式通过 `SQL_DSN` 连接外部 MySQL/PostgreSQL。
> - 旧部署如果必须继续使用容器内 `3000`，请显式加 `-e PORT=3000 -p 宿主端口:3000`；新部署推荐 `PORT=3030` 和 `3030:3030`。

</details>

<details>
<summary><strong>方式 3：宝塔面板</strong></summary>

1. 安装宝塔面板（≥ 9.2.0 版本）
2. 在应用商店搜索 **NexusTok**
3. 一键安装

📖 [图文教程](./docs/installation/BT.md)

</details>

### ⚠️ 多机部署注意事项

> [!WARNING]
> - **必须设置** `SESSION_SECRET` - 否则登录状态不一致
> - **公用 Redis 必须设置** `CRYPTO_SECRET` - 否则数据无法解密

### 🔄 渠道重试与缓存

**重试配置：** `设置 → 运营设置 → 通用设置 → 失败重试次数`

**缓存配置：**
- `REDIS_CONN_STRING`：Redis 缓存（推荐）
- `MEMORY_CACHE_ENABLED`：内存缓存

---

## 📜 许可证

本项目采用 [GNU Affero 通用公共许可证 v3.0 (AGPLv3)](./LICENSE) 授权。

本项目为开源项目，在 [One API](https://github.com/songquanpeng/one-api)（MIT 许可证）的基础上进行二次开发。

如果您所在的组织政策不允许使用 AGPLv3 许可的软件，或您希望规避 AGPLv3 的开源义务，请发送邮件至：[support@c1cada.dev](mailto:support@c1cada.dev)

---

<div align="center">

### 💖 感谢使用 NexusTok

如果这个项目对你有帮助，欢迎给我们一个 ⭐️ Star！

**[NexusTok 仓库](https://github.com/c1cadaBob/NexusTok)** • **[问题反馈](https://github.com/c1cadaBob/NexusTok/issues)** • **[最新发布](https://github.com/c1cadaBob/NexusTok/releases)**

<sub>Built with ❤️ by c1cada</sub>

</div>
