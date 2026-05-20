<div align="center">

![nexustok](/web/default/public/logo.png)

# NexusTok

🍥 **新一代大模型网关与AI资产管理系统**

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

### 使用 Docker Compose（推荐）

```bash
# 克隆项目
git clone https://github.com/c1cada/NexusTok.git
cd NexusTok

# 编辑 docker-compose.yml 配置
nano docker-compose.yml

# 启动服务
docker-compose up -d
```

<details>
<summary><strong>使用 Docker 命令</strong></summary>

```bash
# 拉取最新镜像
docker pull c1cada/nexustok:latest

# 使用 SQLite（默认）
docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  c1cada/nexustok:latest

# 使用 MySQL
docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/oneapi" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  c1cada/nexustok:latest
```

> **💡 提示：** `-v ./data:/data` 会将数据保存在当前目录的 `data` 文件夹中，你也可以改为绝对路径如 `-v /your/custom/path:/data`

</details>

---

## 🚢 部署

> [!TIP]
> **最新版 Docker 镜像：** `c1cada/nexustok:latest`

### 📋 部署要求

| 组件 | 要求 |
|------|------|
| **本地数据库** | SQLite（Docker 需挂载 `/data` 目录）|
| **远程数据库** | MySQL ≥ 5.7.8 或 PostgreSQL ≥ 9.6 |
| **容器引擎** | Docker / Docker Compose |

### ⚙️ 环境变量配置

<details>
<summary>常用环境变量配置</summary>

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `SESSION_SECRET` | 会话密钥（多机部署必须） | - |
| `CRYPTO_SECRET` | 加密密钥（Redis 必须） | - |
| `SQL_DSN` | 数据库连接字符串 | - |
| `REDIS_CONN_STRING` | Redis 连接字符串 | - |
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

📖 **完整配置：** [NexusTok 仓库](https://github.com/c1cada/NexusTok)

</details>

### 🔧 部署方式

<details>
<summary><strong>方式 1：Docker Compose（推荐）</strong></summary>

```bash
# 克隆项目
git clone https://github.com/c1cada/NexusTok.git
cd NexusTok

# 编辑配置
nano docker-compose.yml

# 启动服务
docker-compose up -d
```

</details>

<details>
<summary><strong>方式 2：Docker 命令</strong></summary>

**使用 SQLite：**
```bash
docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  c1cada/nexustok:latest
```

**使用 MySQL：**
```bash
docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/oneapi" \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  c1cada/nexustok:latest
```

> **💡 路径说明：**
> - `./data:/data` - 相对路径，数据保存在当前目录的 data 文件夹
> - 也可使用绝对路径，如：`/your/custom/path:/data`

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

**[NexusTok 仓库](https://github.com/c1cada/NexusTok)** • **[问题反馈](https://github.com/c1cada/NexusTok/issues)** • **[最新发布](https://github.com/c1cada/NexusTok/releases)**

<sub>Built with ❤️ by c1cada</sub>

</div>
