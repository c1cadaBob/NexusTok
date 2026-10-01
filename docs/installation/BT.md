# 宝塔面板部署教程

本文档说明如何使用宝塔面板管理 NexusTok 的单机生产部署，也说明多机部署时宝塔
应用节点的配置边界。生产推荐入口是仓库中的 Docker Compose，默认拓扑为 NexusTok、
PostgreSQL 15 和 Redis 7。

> 📖 官方安装入口：[部署方法](https://docs.nexustok.ai/zh/docs/installation/deployment-methods/bt-docker-installation)

## 一、前置要求

| 项目 | 要求 |
| --- | --- |
| 操作系统 | 64 位 Linux，支持 `amd64` 或 `arm64` |
| 宝塔面板 | 9.2.0 或更高版本 |
| Docker | Docker Engine 和 Docker Compose v2 |
| 工具 | Git、`curl`；脚本生成密钥时优先使用 `openssl` |
| 资源 | 生产环境至少 2 vCPU、4 GB 内存，并按上游流量扩容 |
| 防火墙 | 直接访问只开放 `3030`；使用 HTTPS 反向代理时只开放 `80/443` |

PostgreSQL `5432` 和 Redis `6379` 默认只在 Compose 内部网络访问，不应在宝塔安全组、
云安全组或公网防火墙中开放。部署前确认 `3030` 没有被其它服务占用。

## 二、安装宝塔和 Docker

1. 从[宝塔官网](https://www.bt.cn/new/download.html)选择与系统匹配的安装脚本。
2. 登录宝塔面板，在 **Docker** 功能中安装 Docker Engine。
3. 在终端确认版本：

```bash
docker version
docker compose version
git --version
curl --version
```

4. 在宝塔 **安全** 和云厂商安全组中只放行所需端口。不要为了调试临时长期开放
   `5432` 或 `6379`。

## 三、单机生产部署（推荐）

### 3.1 克隆仓库

选择宝塔网站目录，例如 `/www/wwwroot/NexusTok`：

```bash
cd /www/wwwroot
git clone https://github.com/c1cadaBob/NexusTok.git
cd NexusTok
```

### 3.2 启动 Compose

```bash
bash scripts/deploy.sh
```

脚本会在首次运行时创建权限为 `0600` 的 `.env`，自动生成：

- `POSTGRES_PASSWORD`
- `REDIS_PASSWORD`

后续运行不会覆盖已有密码。脚本会依次执行 `docker compose config`、镜像拉取、启动
PostgreSQL 和 Redis、真实网络密码认证、启动应用和健康检查。Compose 会使用
`c1cadabob/nexustok:latest`、`postgres:15-alpine` 和 `redis:7-alpine`，并把应用映射到
宿主机 `3030`。

如果要在执行脚本前预置密码，直接创建普通文件 `.env`，不要使用符号链接：

```dotenv
POSTGRES_PASSWORD=请替换为随机长密码
REDIS_PASSWORD=请替换为随机长密码
```

```bash
chmod 600 .env
docker compose config
docker compose up -d
```

### 3.3 验证服务和完成初始化

```bash
docker compose ps
curl http://127.0.0.1:3030/api/status
curl http://127.0.0.1:3030/api/setup
```

确认三个服务均为 `healthy` 且 `/api/status` 返回成功后，访问
`http://服务器地址:3030`，按照设置向导完成初始化并创建管理员账户。

### 3.4 宝塔反向代理和 HTTPS

需要域名和 HTTPS 时，在宝塔网站中配置反向代理到 `127.0.0.1:3030`，并确保代理：

- 支持 SSE，关闭或调大响应缓冲和读取超时；
- 支持 WebSocket Upgrade；
- 传递 `Host`、`X-Forwarded-For`、`X-Forwarded-Proto` 等头；
- 不对流式接口强制缓存或提前关闭长连接。

在 Compose 的 `nexustok.environment` 中按实际域名配置：

```yaml
SESSION_COOKIE_SECURE: "true"
SESSION_COOKIE_TRUSTED_URL: "https://nexustok.example.com"
TRUSTED_PROXIES: "127.0.0.1/32"
```

`TRUSTED_PROXIES` 只能填写真实可信的反向代理 IP/CIDR。`SESSION_COOKIE_TRUSTED_URL`
必须是允许调用 refresh/logout 的精确 HTTPS Origin，不是通用 CORS 白名单。

## 四、单容器兼容模式

宝塔应用商店或单条 `docker run` 只适合 SQLite/无 Redis 兼容模式，不会创建 PostgreSQL
和 Redis，不是完整生产入口：

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
  c1cadabob/nexustok:v0.2.4
```

`/var/run/docker.sock` 等同授予容器宿主机 Docker 管理权限，只能在可信管理员可访问的
实例中挂载。SQLite 文件不会自动迁移到 PostgreSQL；已有数据切换前必须备份，并单独
完成经过验证的数据迁移。

## 五、多机部署

多机部署时不要在每个应用节点启动内置 PostgreSQL 和 Redis。应准备一套共享 PostgreSQL
和一套共享 Redis，使用内网访问控制、最小权限和 TLS。每个应用节点必须配置相同的：

- `SQL_DSN`
- `REDIS_CONN_STRING`
- `SESSION_SECRET`
- `CRYPTO_SECRET`

每个节点必须配置唯一的 `NODE_NAME`。主节点不设置 `NODE_TYPE=slave`，负责数据库迁移
和系统任务；从节点设置 `NODE_TYPE=slave`，只提供请求服务。

应用节点可以使用外部服务环境文件：

```bash
mkdir -p /etc/nexustok
chmod 700 /etc/nexustok
vi /etc/nexustok/node.env
```

`node.env` 示例：

```dotenv
SQL_DSN=postgresql://nexustok:替换为数据库密码@postgres.internal:5432/nexustok?sslmode=require
REDIS_CONN_STRING=rediss://:替换为Redis密码@redis.internal:6379/0
SESSION_SECRET=所有节点相同的随机密钥
CRYPTO_SECRET=所有共享Redis节点相同的随机密钥
NODE_NAME=node-1
```

从节点增加 `NODE_TYPE=slave`。用负载均衡器或反向代理把请求转发到各应用节点，并将
`/api/status` 作为健康检查。所有节点保持时钟同步。共享 Redis 可以共享 Session、限流
和缓存控制面；每节点独立 Redis 会产生传播延迟和节点级限流；不使用 Redis 时 Session
回源数据库、限流退回进程内存，不能得到全局一致的限流计数。

Docker socket、节点本地日志和 `/data` 不会自动成为跨节点共享存储。集中日志、共享文件
和任务产物需要单独设计。

## 六、升级、回滚和备份

升级前至少备份：

- `.env`，并以受控方式保存；
- `/opt/nexustok/data`；
- `/opt/nexustok/logs`；
- PostgreSQL named volume，或更推荐的逻辑备份。

```bash
docker compose exec -T postgres pg_dump -U root nexustok > /secure-backup/nexustok-$(date +%F).sql
cp .env /secure-backup/nexustok.env
docker compose pull
docker compose up -d
docker compose ps
```

连续执行 `docker compose up -d` 应保持迁移幂等。回滚时把 NexusTok 镜像固定到上一已
验证版本，保留 PostgreSQL volume 和备份，不要用空数据库覆盖现有数据。多机升级时先
摘除从节点，再逐台更新并健康检查，最后在迁移窗口处理主节点；回滚前确认应用版本和
数据库迁移兼容。

## 七、常见问题

### 3030 端口被占用

```bash
ss -ltnp | grep ':3030'
docker compose ps
```

停止冲突服务，或让反向代理监听公网端口并只把内部流量转到 `3030`。

### PostgreSQL 或 Redis 不健康

```bash
docker compose logs --tail=120 postgres redis
docker compose config
```

检查 `.env` 中密码不为空、没有出现多个同名变量，确认磁盘空间和 Docker 网络正常。
修改 `.env` 中的 `POSTGRES_PASSWORD` 不会自动修改已有 PostgreSQL named volume 中的
数据库角色密码。脚本会在启动应用前执行真实网络认证；认证失败时请恢复原密码，或
先完成逻辑备份再单独同步数据库角色密码。不要在未备份数据库的情况下删除 PostgreSQL
named volume。Redis 当前没有独立 named volume，重建 Redis 会丢失缓存和临时 Session、
限流状态。

### `.env` 权限或密码问题

```bash
stat -c '%a %n' .env
chmod 600 .env
```

脚本拒绝符号链接 `.env`，这是为了避免把密码写入非预期位置。修改数据库或 Redis 密码
前先规划数据迁移和服务重建，并确认所有连接字符串一致。

### 全新部署清理

全量重置前必须确认 PostgreSQL 逻辑备份可恢复，并停止所有 NexusTok 应用请求。只删除
已核对属于 NexusTok 的容器、PostgreSQL named volume、Redis 容器的匿名数据卷、
Compose 网络以及 `/opt/nexustok/data` 和 `/opt/nexustok/logs`。保留 `.env` 的受控备份，
不要删除其它项目的容器、卷、网络或 `komari-agent`。

不要执行 `docker system prune -a`、`docker volume prune` 或未确认范围的
`docker compose down -v`。这些命令可能删除其它项目资源或生产数据库。清理完成后由
管理员重新克隆仓库，检查 `VERSION`/标签，再自行运行 `bash scripts/deploy.sh`；本项目
不会从清理脚本自动恢复仓库或执行部署。

### 反向代理访问异常

检查 HTTPS 证书、`X-Forwarded-*`、SSE 缓冲、WebSocket Upgrade、读取超时和
`SESSION_COOKIE_TRUSTED_URL`。若代理 IP 不在 `TRUSTED_PROXIES` 中，应用可能拒绝或错误
解析转发来源。

### 镜像架构不匹配

```bash
uname -m
docker image inspect c1cadabob/nexustok:v0.2.4 --format '{{.Architecture}}'
```

主机应为 `x86_64`/`amd64` 或 `aarch64`/`arm64`。老旧 CPU、32 位系统和受限的镜像仓库
代理可能导致拉取或启动失败。

### Docker socket 风险

不需要后台 Docker 自动更新时，应评估是否可以移除 socket 挂载；一旦挂载，容器内的
Root 管理功能等同拥有宿主机 Docker 控制权。

## 八、相关链接

- [环境变量配置](https://docs.nexustok.ai/zh/docs/installation/config-maintenance/environment-variables)
- [常见问题](https://docs.nexustok.ai/zh/docs/support/faq)
- [GitHub 仓库](https://github.com/c1cadaBob/NexusTok)

## 九、宝塔截图

![宝塔面板 Docker 安装](https://github.com/user-attachments/assets/7a6fc03e-c457-45e4-b8f9-184508fc26b0)

> 截图仅用于说明面板入口；生产环境仍应按本文的 Compose、健康检查和备份流程执行。
