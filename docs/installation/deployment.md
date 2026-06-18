# NexusTok 启动与部署维护手册

本文档用于说明 NexusTok 在不同场景下的启动方式、构建链路、关键目录和排障入口，便于后期维护、部署交接和问题定位。

## 总览

NexusTok 后端是单个 Go 服务，前端有三套构建产物会被嵌入到 Go 二进制中：

| 前端 | 目录 | 构建工具 | 嵌入路径 | 用途 |
|------|------|----------|----------|------|
| 默认前端 | `web/default` | Bun + Rsbuild | `web/default/dist` | 新版 React 19 前端 |
| 经典前端 | `web/classic` | Bun + Vite | `web/classic/dist` | 经典 Semi UI 前端 |
| 账号池管理器 | `modules/cpa-manager` | npm/Bun + Vite | `modules/cpa-manager/dist` | `/account-pool/manager/` 嵌入页面 |

Go 入口文件 `main.go` 使用 `//go:embed` 嵌入以上构建产物。生产部署时只需要运行最终的 `nexustok` 二进制或 Docker 镜像，不需要额外提供前端文件。

运行时主题由配置项 `theme.frontend` 控制：

| 值 | 效果 |
|----|------|
| `default` | 使用 `web/default/dist`，新版默认前端 |
| `classic` | 使用 `web/classic/dist`，经典前端 |

可通过接口确认当前主题：

```bash
curl -sS http://127.0.0.1:3000/api/status | grep -o '"theme":"[^"]*"'
```

如果页面样式没有变化，优先确认 `theme.frontend` 是否仍为 `classic`，以及 Go 服务是否已经重新编译嵌入了最新 `dist`。

## 推荐生产部署：Docker Compose

生产环境推荐使用仓库根目录的 `docker-compose.yml`：

```bash
git clone https://github.com/c1cada/NexusTok.git
cd NexusTok

# 首次部署前必须检查并修改默认密码和密钥。
vim docker-compose.yml

docker compose up -d
```

旧版 Docker Compose 可使用：

```bash
docker-compose up -d
```

默认暴露端口：

| 服务 | 容器名 | 容器端口 | 宿主机端口 | 说明 |
|------|--------|----------|------------|------|
| NexusTok | `nexustok` | `3000` | `3000` | 主 Web、管理后台和 Relay API |
| Redis | `redis` | `6379` | 不对外暴露 | 缓存、限流、分布式状态 |
| PostgreSQL | `postgres` | `5432` | 不对外暴露 | 主数据库 |
| CLIProxyAPI | `nexustok-account-pool` | `8317` | 不对外暴露 | 账号池 Sidecar |
| CPA Usage | `nexustok-account-pool-usage` | `18317` | 不对外暴露 | 账号池请求监控服务 |

常用命令：

```bash
# 查看服务状态
docker compose ps

# 查看主服务日志
docker logs -f nexustok

# 查看账号池 Sidecar 日志
docker logs -f nexustok-account-pool

# 查看请求监控服务日志
docker logs -f nexustok-account-pool-usage

# 重启主服务
docker compose restart nexustok

# 停止服务，保留数据卷
docker compose down

# 停止服务并删除 Compose 管理的数据卷，谨慎使用
docker compose down -v
```

生产环境必须修改的默认值：

| 配置 | 位置 | 说明 |
|------|------|------|
| `SESSION_SECRET` 或 `SESSION_SECRET_FILE` | `environment` | 多实例部署必须固定；否则登录态会失效 |
| `CRYPTO_SECRET` | `environment` | 使用 Redis 或多实例时建议固定，避免加密数据无法解密 |
| `SQL_DSN` | `environment` | 数据库连接串，支持 SQLite、MySQL、PostgreSQL |
| `REDIS_CONN_STRING` | `environment` | Redis 连接串 |
| PostgreSQL 密码 | `postgres.environment` | 默认 `123456` 只能用于本地测试 |
| Redis 密码 | `redis.command` | 默认 `123456` 只能用于本地测试 |
| `ACCOUNT_POOL_MANAGEMENT_KEY` | `.env` 或环境变量 | 账号池内部管理密钥，生产必须改 |
| `ACCOUNT_POOL_CLI_PROXY_RELAY_KEY` | `.env` 或环境变量 | 账号池 Relay 密钥，生产必须改 |

## 单容器部署

如果不需要 Compose 编排，可以直接运行官方镜像。

SQLite 模式：

```bash
docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET="$(openssl rand -hex 32)" \
  -v "$PWD/data:/data" \
  c1cada/nexustok:latest
```

外部 PostgreSQL 示例：

```bash
docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET="请替换为固定随机字符串" \
  -e SQL_DSN="postgresql://user:password@host:5432/nexustok?sslmode=disable" \
  -e REDIS_CONN_STRING="redis://:password@host:6379/0" \
  -v "$PWD/data:/data" \
  c1cada/nexustok:latest
```

单容器模式不会自动启动内置账号池 Sidecar 和 CPA Usage Service。需要账号池功能时，优先使用 `docker-compose.yml`。

## 从源码构建生产镜像

仓库根目录的 `Dockerfile` 是完整生产镜像构建流程：

1. 构建默认前端：`web/default -> web/default/dist`
2. 构建经典前端：`web/classic -> web/classic/dist`
3. 构建账号池管理器：`modules/cpa-manager -> modules/cpa-manager/dist`
4. 编译 Go 后端并嵌入上述构建产物
5. 输出精简运行镜像

构建命令：

```bash
docker build -t nexustok:local .
```

运行：

```bash
docker run --name nexustok-local -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET="$(openssl rand -hex 32)" \
  -v "$PWD/data:/data" \
  nexustok:local
```

跨平台构建可传入 `TARGETOS`、`TARGETARCH`：

```bash
docker build \
  --build-arg TARGETOS=linux \
  --build-arg TARGETARCH=amd64 \
  -t nexustok:linux-amd64 .
```

## 本地开发：后端容器 + 前端开发服务器

前后端分离开发使用 `docker-compose.dev.yml` 和前端开发服务器。

启动后端依赖：

```bash
docker compose -f docker-compose.dev.yml up -d
```

启动默认前端开发服务器：

```bash
cd web/default
bun install
bun run dev
```

默认前端 Rsbuild 会把 `/api`、`/mj`、`/pg` 代理到 `VITE_REACT_APP_SERVER_URL`，未设置时为 `http://localhost:3000`。

启动经典前端开发服务器：

```bash
cd web/classic
bun install
bun run dev
```

也可以使用 Makefile：

```bash
make dev-api
make dev-web
make dev-web-classic
```

注意：`Dockerfile.dev` 只给 Go embed 创建占位前端页面，真正页面应通过前端开发服务器访问。

## 热重载部署：docker-compose.hot.yml

`docker-compose.hot.yml` 适合服务器上边开发边验证，当前目录源码会被挂载进容器。

启动：

```bash
docker compose -f docker-compose.hot.yml up -d --build
```

默认端口：

| 服务 | 容器名 | 端口 |
|------|--------|------|
| 主服务 | `nexustok-api-hot` | 宿主机 `3003` -> 容器 `3000` |
| 前端构建监听 | `nexustok-frontend-watch` | 不暴露端口 |
| 账号池 Sidecar | `nexustok-account-pool-hot` | Docker 网络内 `8317` |
| CPA Usage | `nexustok-account-pool-usage-hot` | Docker 网络内 `18317` |
| PostgreSQL | `nexustok-hot-pg` | Docker 网络内 `5432` |
| Redis | `nexustok-hot-redis` | Docker 网络内 `6379` |

可通过 `HOT_PORT` 改宿主机端口：

```bash
HOT_PORT=3005 docker compose -f docker-compose.hot.yml up -d --build
```

热重载链路：

1. `nexustok-frontend-watch` 执行三套前端的 `build --watch`。
2. `bin/docker-hot-backend.sh` 等待三个 `dist/index.html` 存在。
3. 脚本监听 Go 文件、`go.mod`、`go.sum` 和三套 `dist`。
4. 检测到变化后执行 `go build`。
5. 编译成功后重启 `/app/tmp/hot/nexustok` 子进程。

常用命令：

```bash
# 看前端构建监听日志
docker logs -f nexustok-frontend-watch

# 看后端热重载日志
docker logs -f nexustok-api-hot

# 查看热重载子进程
docker exec nexustok-api-hot sh -lc "ps -ef | grep '/app/tmp/hot/nexustok' | grep -v grep"

# 手动杀掉子进程，守护脚本会自动拉起
docker exec nexustok-api-hot sh -lc 'pid=$(ps -ef | awk "/\/app\/tmp\/hot\/nexustok/ && !/awk/ {print \$1; exit}"); [ -n "$pid" ] && kill "$pid"'

# 查看当前主题
curl -sS http://127.0.0.1:3003/api/status | grep -o '"theme":"[^"]*"'
```

切换热环境主题为默认前端：

```bash
docker exec nexustok-hot-pg psql -U root -d nexustok \
  -c "insert into options(key, value) values ('theme.frontend', 'default') on conflict (key) do update set value=excluded.value;"

docker compose -f docker-compose.hot.yml restart nexustok
```

切换回经典前端：

```bash
docker exec nexustok-hot-pg psql -U root -d nexustok \
  -c "insert into options(key, value) values ('theme.frontend', 'classic') on conflict (key) do update set value=excluded.value;"

docker compose -f docker-compose.hot.yml restart nexustok
```

## systemd 二进制部署

仓库提供 `nexustok.service` 模板，适合已经手动构建二进制的服务器。

构建前端和后端：

```bash
make build-all-frontends
go build -ldflags "-s -w -X github.com/c1cada/NexusTok/common.Version=$(cat VERSION)" -o nexustok .
```

安装服务文件：

```bash
sudo cp nexustok.service /etc/systemd/system/nexustok.service
sudo systemctl daemon-reload
sudo systemctl enable nexustok
sudo systemctl start nexustok
```

使用前必须修改 `nexustok.service` 中的：

| 字段 | 说明 |
|------|------|
| `User` | 运行服务的系统用户 |
| `WorkingDirectory` | 项目或二进制所在目录 |
| `ExecStart` | 二进制路径、端口、日志目录 |

查看状态和日志：

```bash
sudo systemctl status nexustok
journalctl -u nexustok -f
```

## 启动顺序

应用启动的关键顺序在 `main.go`：

1. 加载 `.env` 和环境变量。
2. 初始化日志、模型倍率、HTTP 客户端、Token 编码器。
3. 初始化主数据库，自动检查和迁移表结构。
4. 加载系统选项到内存，包括 `theme.frontend`。
5. 初始化定价数据、日志数据库和 Redis。
6. 初始化监控、i18n、OAuth、Relay 通道。
7. 注册 API 路由、Web 静态资源路由和账号池管理器路由。
8. 启动 HTTP 服务，默认监听 `3000`。

Web 静态资源路由会根据 `common.GetTheme()` 选择默认前端或经典前端。根路径和 SPA 路由由 `router/web-router.go` 的 `NoRoute` 回退到对应主题的 `index.html`。

## 后台定时任务

主服务启动后会在主节点上启动多类后台任务，包括渠道缓存同步、系统配置热更新、数据看板刷新、渠道可用性测试、订阅配额维护、账号池凭据刷新、Codex 凭据刷新、上游模型巡检，以及 models.dev 模型目录同步。

models.dev 模型目录同步用于每天凌晨从 `https://models.dev/catalog.json` 拉取公开模型目录，并补齐本地缺失的模型和供应商。该任务只创建本地不存在的记录，不覆盖管理员手动编辑的模型描述、图标、标签、供应商、状态或价格倍率。

相关环境变量：

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `MODELS_DEV_AUTO_SYNC_ENABLED` | `true` | 是否启用 models.dev 每日模型目录同步 |
| `MODELS_DEV_AUTO_SYNC_TIME` | `02:00` | 每日运行时间，格式 `HH:mm`，使用进程当前时区 |
| `MODELS_DEV_SYNC_BASE` | `https://models.dev` | models.dev 基础地址；内网镜像或代理可覆盖 |

日志中出现以下内容表示任务已经启动：

```text
models.dev model sync task started: time=02:00 source=https://models.dev/catalog.json
```

如果需要临时关闭自动同步：

```yaml
environment:
  - MODELS_DEV_AUTO_SYNC_ENABLED=false
```

## 常用排障

### 页面没有变化或仍显示旧版首页

检查顺序：

```bash
# 1. 确认服务实际返回的主题
curl -sS http://127.0.0.1:3000/api/status | grep -o '"theme":"[^"]*"'

# 2. 确认 HTML 加载的是新版 static 资源还是经典 assets 资源
curl -sS http://127.0.0.1:3000/ | sed -n '1,80p'

# 3. 热重载环境检查默认前端 dist
ls -lah web/default/dist
sed -n '1,80p' web/default/dist/index.html

# 4. 查看后端是否重新编译
docker logs --tail 120 nexustok-api-hot
```

判断标准：

| 现象 | 可能原因 | 处理 |
|------|----------|------|
| `/api/status` 返回 `theme=classic` | 当前启用经典主题 | 将 `theme.frontend` 设置为 `default` 并重启服务 |
| HTML 引用 `/assets/semi-ui...` | 返回的是经典前端产物 | 检查主题配置和重启状态 |
| HTML 引用 `/static/js/index...` | 返回的是默认前端产物 | 浏览器强制刷新或清理 CDN/反代缓存 |
| `web/default/dist` 时间旧 | 前端没有重新构建 | 执行 `cd web/default && bun run build` 或检查 `frontend-watch` |
| dist 已更新但页面旧 | Go 二进制未重新编译嵌入 | 重新 `go build` 或重启热重载主服务 |

### 接口正常但浏览器访问失败

检查监听端口和容器端口映射：

```bash
ss -ltnp | grep -E ':3000|:3003|:8080'
docker ps --format '{{.Names}} {{.Ports}}'
```

如果经过 Nginx、Caddy、宝塔或云厂商端口转发，还需要确认外部端口映射到哪个内部服务。排查时优先直连宿主机本地端口，例如 `127.0.0.1:3000` 或 `127.0.0.1:3003`，再排查外部反向代理。

### 容器启动失败

查看主服务日志：

```bash
docker logs --tail 200 nexustok
```

热重载环境：

```bash
docker logs --tail 200 nexustok-api-hot
docker logs --tail 200 nexustok-frontend-watch
```

常见原因：

| 日志特征 | 原因 | 处理 |
|----------|------|------|
| 数据库连接失败 | `SQL_DSN`、数据库账号或网络错误 | 检查连接串、数据库容器健康状态 |
| Redis 连接失败 | `REDIS_CONN_STRING` 或 Redis 密码错误 | 检查 Redis 配置，不使用 Redis 时移除连接串 |
| `waiting for production frontend dist` | 热重载环境缺少前端构建产物 | 查看 `nexustok-frontend-watch` 构建日志 |
| `go build` 失败 | Go 代码编译错误或依赖下载失败 | 查看错误行，检查 `GOPROXY` 和代码改动 |

### 登录状态频繁失效

多实例或容器重建后登录失效，通常是会话密钥变化。

处理方式：

```yaml
environment:
  - SESSION_SECRET=请替换为固定随机字符串
```

或使用持久化文件：

```yaml
environment:
  - SESSION_SECRET_FILE=/data/session_secret
volumes:
  - ./data:/data
```

### 数据没有持久化

确认 `/data` 已挂载到宿主机：

```bash
docker inspect nexustok --format '{{json .Mounts}}'
```

Compose 部署默认挂载：

```yaml
volumes:
  - ./data:/data
  - ./logs:/app/logs
```

### 账号池管理器不可用

检查 Sidecar 和 Usage Service：

```bash
docker logs --tail 200 nexustok-account-pool
docker logs --tail 200 nexustok-account-pool-usage
```

热重载环境对应容器：

```bash
docker logs --tail 200 nexustok-account-pool-hot
docker logs --tail 200 nexustok-account-pool-usage-hot
```

确认主服务环境变量指向 Docker 网络内地址：

```yaml
ACCOUNT_POOL_CLI_PROXY_URL=http://cliproxyapi:8317
ACCOUNT_POOL_USAGE_SERVICE_URL=http://account-pool-usage:18317
```

### 静态资源 404

检查当前主题对应的资源路径：

| 主题 | 典型资源路径 |
|------|--------------|
| `default` | `/static/js/...`、`/static/css/...` |
| `classic` | `/assets/...` |

如果资源路径不存在，说明构建产物和当前主题不匹配，或 Go 二进制嵌入的是旧产物。重新构建对应前端并重新编译后端。

### models.dev 模型目录没有自动同步

检查任务是否启动：

```bash
docker logs --tail 200 nexustok | grep 'models.dev model sync'
```

热重载环境：

```bash
docker logs --tail 200 nexustok-api-hot | grep 'models.dev model sync'
```

检查项：

| 现象 | 可能原因 | 处理 |
|------|----------|------|
| 没有启动日志 | 非主节点或 `MODELS_DEV_AUTO_SYNC_ENABLED=false` | 确认主服务节点和环境变量 |
| 提示 `invalid MODELS_DEV_AUTO_SYNC_TIME` | 时间格式不是 `HH:mm` 或超出范围 | 改为例如 `02:00`、`03:30` |
| 拉取失败 | 服务器无法访问 `https://models.dev/catalog.json` | 检查 DNS、代理、防火墙，或配置 `MODELS_DEV_SYNC_BASE` |
| 同步后模型没有变化 | 本地已存在这些模型，任务不会覆盖已有记录 | 在后台模型页面检查模型是否已存在，必要时使用手动编辑或覆盖同步 |

管理员也可以通过后台登录态调用预览接口确认 models.dev 源是否可访问：

```bash
curl -H "NexusTok-User: <管理员用户ID>" \
  "http://127.0.0.1:3000/api/models/sync_upstream/preview?source=models.dev"
```

## 更新流程

生产 Compose 更新：

```bash
git pull
docker compose pull nexustok
docker compose up -d --build
docker compose ps
curl -sS http://127.0.0.1:3000/api/status
```

热重载环境更新：

```bash
git pull
docker compose -f docker-compose.hot.yml up -d --build
docker logs -f nexustok-frontend-watch
docker logs -f nexustok-api-hot
```

源码二进制更新：

```bash
git pull
make build-all-frontends
go build -ldflags "-s -w -X github.com/c1cada/NexusTok/common.Version=$(cat VERSION)" -o nexustok .
sudo systemctl restart nexustok
```

更新后验证：

```bash
curl -sS http://127.0.0.1:3000/api/status
curl -sS http://127.0.0.1:3000/ | sed -n '1,80p'
```

如果经过反向代理或 CDN，请再用浏览器强制刷新，并检查网络面板中 HTML、JS、CSS 的资源哈希是否已经更新。
