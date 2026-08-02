# 宝塔面板部署教程

本文档提供使用宝塔面板 Docker 功能部署 NexusTok 的图文教程。

> 📖 官方链接：[NexusTok 仓库](https://github.com/c1cadaBob/NexusTok)

***

## 前置要求

| 项目    | 要求                                 |
| ----- | ---------------------------------- |
| 宝塔面板  | ≥ 9.2.0 版本                         |
| 推荐系统  | CentOS 7+、Ubuntu 18.04+、Debian 10+ |
| 服务器配置 | 至少 1 核 2G 内存                       |

***

## 步骤一：安装宝塔面板

1. 前往 [宝塔面板官网](https://www.bt.cn/new/download.html) 下载适合您系统的安装脚本
2. 运行安装脚本安装宝塔面板
3. 安装完成后，使用提供的地址、用户名和密码登录宝塔面板

***

## 步骤二：安装 Docker

1. 登录宝塔面板后，在左侧菜单栏找到并点击 **Docker**
2. 首次进入会提示安装 Docker 服务，点击 **立即安装**
3. 按照提示完成 Docker 服务的安装

***

## 步骤三：安装 NexusTok

### 方法一：使用宝塔应用商店（推荐）

1. 在宝塔面板 Docker 功能中，点击 **应用商店**
2. 搜索并找到 **NexusTok**
3. 点击 **安装**
4. 配置以下基本选项：
   - **容器名称**：可自定义，默认为 `nexustok`
   - **端口映射**：默认为 `3000:3000`
   - **环境变量**：
     - `SESSION_SECRET` 或 `SESSION_SECRET_FILE`：会话密钥（**必填**，多机部署时必须一致）
     - `CRYPTO_SECRET`：加密密钥（使用 Redis 时必填）
5. 点击 **确认** 开始安装
6. 等待安装完成后，访问 `http://您的服务器IP:3000` 即可使用

### 方法二：使用 Docker Compose

1. 在宝塔面板中创建网站目录，如 `/www/wwwroot/nexustok`
2. 创建 `docker-compose.yml` 文件：

```yaml
version: '3'
services:
  nexustok:
    image: c1cadabob/nexustok:latest
    container_name: nexustok
    restart: always
    ports:
      - "3000:3000"
    volumes:
      - ./data:/data
      - ./logs:/app/logs
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - SESSION_SECRET_FILE=/data/session_secret
      - TZ=Asia/Shanghai
```

1. 在终端中进入目录并启动：

```bash
cd /www/wwwroot/nexustok
docker-compose up -d
```

### 方法三：终端直接启动单容器

如果已经成功拉取 `c1cadabob/nexustok:latest`，也可以在服务器终端中直接启动 SQLite 单容器：

```bash
mkdir -p /opt/nexustok/data /opt/nexustok/logs
docker rm -f nexustok 2>/dev/null || true

docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest
```

启动后检查：

```bash
docker ps | grep nexustok
docker logs -f nexustok
curl -sS http://127.0.0.1:3000/api/status
```

访问 `http://您的服务器IP:3000`。如果无法访问，请在宝塔面板 **安全** 和云服务器安全组中同时放行 TCP `3000` 端口。

`/var/run/docker.sock` 用于后台 **系统维护 → 检查更新** 页面自动拉取镜像并重建当前容器，等同宿主机 Docker 管理权限，只应在可信管理员环境中挂载。如果旧容器启动时没有挂载它，维护页会显示一次性重建命令；执行后仍复用 `/opt/nexustok/data` 和 `/opt/nexustok/logs`，不会删除数据。

***

## 配置说明

### 必要环境变量

| 变量名                 | 说明                 | 是否必填   |
| ------------------- | ------------------ | ------ |
| `SESSION_SECRET` / `SESSION_SECRET_FILE` | 会话密钥或持久化密钥文件，多机部署必须一致 | **必填** |
| `CRYPTO_SECRET`     | 加密密钥，使用 Redis 时必填  | 条件必填   |
| `SQL_DSN`           | 数据库连接字符串（使用外部数据库时） | 可选     |
| `REDIS_CONN_STRING` | Redis 连接字符串        | 可选     |

### 高并发部署建议

宝塔单容器 SQLite 适合快速安装、测试和低流量站点。多节点或高并发生产环境建议改用 Docker Compose，并至少启用 PostgreSQL 和 Redis：

```yaml
environment:
  - SQL_DSN=postgresql://user:password@postgres:5432/nexustok?sslmode=disable
  - REDIS_CONN_STRING=redis://:password@redis:6379/0
  - REDIS_POOL_SIZE=256
  - SESSION_SECRET=请替换为固定随机字符串
  - CRYPTO_SECRET=请替换为固定随机字符串
```

NexusTok Docker 镜像只包含应用本身，不内置 PostgreSQL/MySQL/Redis 服务。宝塔单容器模式默认使用 SQLite；如果要使用外部 PostgreSQL，请在容器环境变量中设置 `SQL_DSN=postgresql://user:password@host:5432/nexustok?sslmode=disable`。ClickHouse 只用于 `LOG_SQL_DSN` 独立日志库，不能作为主业务库。

如果消费日志量很大，可在 Compose 中额外启用 ClickHouse，并设置 `LOG_SQL_DSN=clickhouse://...` 与 `LOG_SQL_CLICKHOUSE_TTL_DAYS=30`。ClickHouse 只用于通用消费 `logs` 表，主业务库仍必须使用 PostgreSQL、MySQL 或 SQLite。经过宝塔反向代理、Nginx 或 Caddy 时，请关闭响应缓冲并把 `proxy_read_timeout` 调大，避免流式响应被代理中断。

### 生成随机密钥（可选）

默认示例使用 `SESSION_SECRET_FILE=/data/session_secret`，服务会在首次启动时自动生成并保存密钥。只有改用 `SESSION_SECRET` 环境变量时，才需要手动生成固定随机字符串：

```bash
# 生成 SESSION_SECRET
openssl rand -hex 16

# 或使用 Linux 命令
head -c 16 /dev/urandom | xxd -p
```

***

## 常见问题

### Q1：无法访问 3000 端口？

1. 检查服务器防火墙是否开放 3000 端口
2. 在宝塔面板 **安全** 中放行 3000 端口
3. 检查云服务器安全组是否开放端口

### Q2：登录后提示会话失效？

确保设置了 `SESSION_SECRET` 环境变量，或使用 `SESSION_SECRET_FILE=/data/session_secret` 并持久化挂载 `/data` 目录。

### Q3：数据如何持久化？

使用 Docker 卷映射数据目录：

```yaml
volumes:
  - ./data:/data
```

### Q4：如何更新版本？

Docker Compose 部署：

```bash
# 拉取最新镜像
docker pull c1cadabob/nexustok:latest

# 重启容器
docker-compose down && docker-compose up -d
```

单容器部署：

```bash
docker pull c1cadabob/nexustok:latest
docker stop nexustok
docker rm nexustok

# 使用原来的数据目录重新启动，不要删除 /opt/nexustok/data。
docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  c1cadabob/nexustok:latest
```

***

## 相关链接

- [NexusTok 仓库](https://github.com/c1cadaBob/NexusTok)
- [README](https://github.com/c1cadaBob/NexusTok#readme)
- [问题反馈](https://github.com/c1cadaBob/NexusTok/issues)
- [GitHub 仓库](https://github.com/c1cadaBob/NexusTok)

***

## 截图示例

![宝塔面板 Docker 安装](https://github.com/user-attachments/assets/7a6fc03e-c457-45e4-b8f9-184508fc26b0)

> ⚠️ 注意：请务必固定会话密钥。单容器和 Compose 示例默认使用 `SESSION_SECRET_FILE=/data/session_secret`，因此不要删除持久化的 `data` 目录。
