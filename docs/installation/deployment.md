# NexusTok 启动与部署维护手册

本文档用于说明 NexusTok 在不同场景下的启动方式、构建链路、关键目录和排障入口，便于后期维护、部署交接和问题定位。

## 总览

NexusTok 后端是单个 Go 服务，前端有两套构建产物会被嵌入到 Go 二进制中：

| 前端 | 目录 | 构建工具 | 嵌入路径 | 用途 |
|------|------|----------|----------|------|
| 默认前端 | `web/default` | Bun + Rsbuild | `web/default/dist` | 新版 React 19 前端 |
| 经典前端 | `web/classic` | Bun + Vite | `web/classic/dist` | 经典 Semi UI 前端 |

Go 入口文件 `main.go` 使用 `//go:embed` 嵌入以上构建产物。生产部署时只需要运行最终的 `nexustok` 二进制或 Docker 镜像，不需要额外提供前端文件。

账号池已收敛为主服务内置的原生能力。原生账号池分组、池账号和认证文件由 NexusTok 主服务直接读写主数据库；默认前端账号池页面的 **Auth Files** 视图可导入原生、`sub2`、`newapi` 和 Sub2api 批量 JSON 凭据。CPAMC/CLIProxyAPI/Sidecar 管理器不再作为运行时入口，也不会在默认部署中启动。

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
git clone https://github.com/c1cadaBob/NexusTok.git
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

常用命令：

```bash
# 查看服务状态
docker compose ps

# 查看主服务日志
docker logs -f nexustok

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

## 多节点高并发部署基线

高并发生产环境建议采用 **PostgreSQL 主库 + Redis 热路径 + 独立日志库/ClickHouse + Caddy/Nginx 反向代理** 的渐进方案。SQLite 仍适合快速体验和低流量单机部署，但不建议作为多节点生产主库。

推荐基础环境变量：

```yaml
environment:
  - SQL_DSN=postgresql://user:password@postgres:5432/nexustok?sslmode=disable
  - REDIS_CONN_STRING=redis://:password@redis:6379/0
  - REDIS_POOL_SIZE=256
  - SQL_MAX_IDLE_CONNS=100
  - SQL_MAX_OPEN_CONNS=1000
  - SQL_MAX_LIFETIME=60
  - SESSION_SECRET=请替换为固定随机字符串
  - CRYPTO_SECRET=请替换为固定随机字符串
  - RELAY_MAX_IDLE_CONNS=1000
  - RELAY_MAX_IDLE_CONNS_PER_HOST=200
  - RELAY_MAX_CONNS_PER_HOST=0
  - RELAY_RESPONSE_HEADER_TIMEOUT=300
  - RELAY_PROXY_CLIENT_CACHE_TTL=900
  - RELAY_PROXY_CLIENT_CACHE_MAX_SIZE=4096
```

关键约束：

- 主业务库只支持 SQLite、MySQL 和 PostgreSQL；多节点高并发优先使用 PostgreSQL。
- Redis 在多节点部署中建议作为必选组件，用于分布式限流、缓存、账号池并发状态和轮询游标等热路径。
- 每个实例的 `SESSION_SECRET`、`CRYPTO_SECRET` 必须保持一致；`NODE_NAME` 建议显式设置为唯一名称，便于系统实例心跳和排障。
- `SystemTask` 继续使用主数据库表和租约锁，不需要额外引入 Kafka/RabbitMQ；系统更新、日志清理、账号检测等低频/中频任务仍由现有 runner 承载。

### ClickHouse 消费日志库

如果消费日志写入或统计查询已经影响主库，可以把通用 `logs` 表拆到 ClickHouse：

```yaml
environment:
  - LOG_SQL_DSN=clickhouse://default:password@clickhouse:9000/nexustok_logs
  - LOG_SQL_CLICKHOUSE_TTL_DAYS=30
```

ClickHouse 在 NexusTok 中只作为独立消费日志库使用：

- 只承载高频通用 `logs` 表，账号池使用日志、账号池状态日志等管理审计辅助表会保留在主业务库。
- `LOG_SQL_CLICKHOUSE_TTL_DAYS=30` 会给 `logs` 表同步 TTL；设置为 `0` 表示不启用自动清理。
- 主库 `SQL_DSN` 不能配置为 ClickHouse；如果需要强一致事务、权限、订阅、余额、账号池和 SystemTask，仍必须使用 PostgreSQL/MySQL/SQLite。

`docker-compose.yml` 已内置注释版 ClickHouse 服务。启用步骤：

1. 取消 `LOG_SQL_DSN=clickhouse://...` 和 `LOG_SQL_CLICKHOUSE_TTL_DAYS` 的注释。
2. 在 `depends_on` 中取消 `clickhouse` 的注释。
3. 取消 `clickhouse` 服务和 `clickhouse_data` 卷的注释。
4. 修改默认密码后执行 `docker compose up -d`。

### Relay 连接池参数

Relay 到上游模型服务的连接复用会直接影响 P95/P99 延迟。建议先使用默认值压测，再按上游数量和代理数量调整：

| 环境变量 | 默认值 | 建议 |
|----------|--------|------|
| `RELAY_MAX_IDLE_CONNS` | `500` | 高并发可提升到 `1000` 或更高 |
| `RELAY_MAX_IDLE_CONNS_PER_HOST` | `100` | 单个上游或代理并发高时可提升到 `200` |
| `RELAY_MAX_CONNS_PER_HOST` | `0` | 默认不限制；需要保护代理或上游时设置明确上限 |
| `RELAY_RESPONSE_HEADER_TIMEOUT` | `0` | 可设置 `300`，防止上游长时间不返回响应头 |
| `RELAY_PROXY_CLIENT_CACHE_TTL` | `900` | 多代理场景下回收长时间不用的 proxy client |
| `RELAY_PROXY_CLIENT_CACHE_MAX_SIZE` | `4096` | 代理 URL 很多时限制缓存规模 |

### 反向代理配置

Caddy 示例：

```caddyfile
example.com {
  encode zstd gzip

  reverse_proxy 127.0.0.1:3000 {
    header_up X-Real-IP {remote_host}
    header_up X-Forwarded-For {remote_host}
    header_up X-Forwarded-Proto {scheme}

    transport http {
      keepalive 120s
      keepalive_idle_conns 256
      compression off
    }
  }
}
```

Nginx 关键项：

```nginx
location / {
  proxy_pass http://127.0.0.1:3000;
  proxy_http_version 1.1;
  proxy_set_header Connection "";
  proxy_set_header Host $host;
  proxy_set_header X-Real-IP $remote_addr;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  proxy_set_header X-Forwarded-Proto $scheme;
  proxy_buffering off;
  proxy_request_buffering off;
  proxy_read_timeout 600s;
  proxy_send_timeout 600s;
  client_max_body_size 32m;
}
```

流式响应/SSE 场景下最重要的是关闭响应缓冲、保留长超时和上游 keepalive。反向代理品牌不是关键，关键是这些参数要和实际流式请求、上游延迟和文件上传大小匹配。

## 多节点压测与观测

仓库提供一套专用的轻量压测环境，用于在继续优化热路径前建立可重复的基线。该环境使用独立 Compose project 和独立 Docker volume，包含：

| 服务 | 作用 |
|------|------|
| `nexustok-a` / `nexustok-b` | 两个 NexusTok 实例，使用相同 `SESSION_SECRET` / `CRYPTO_SECRET`，不同 `NODE_NAME` |
| `postgres` | 压测主库 |
| `redis` | 压测缓存、限流和多节点热路径状态 |
| `clickhouse` | 压测消费日志库，默认 `LOG_SQL_CLICKHOUSE_TTL_DAYS=30` |
| `caddy` | 入口反向代理，默认宿主机端口 `3100`，轮询转发到两个 NexusTok 实例 |
| `mock-openai` | 本地 OpenAI-compatible 上游，支持非流式、SSE 流式、错误率和 429 注入 |
| `k6` | 压测执行容器 |

> 该环境只用于压测。不要复用生产数据库、生产 Redis、生产 ClickHouse、生产密钥或真实上游 key。默认压测请求只打本地 `mock-openai`，不会产生真实模型费用。

### 启动压测环境

```bash
docker compose -f docker-compose.loadtest.yml up -d --build
docker compose -f docker-compose.loadtest.yml ps
curl -sS http://127.0.0.1:3100/api/status
```

如果宿主机已有 `3100` 或 `18080` 端口占用，可在 `docker-compose.loadtest.yml` 中调整 `caddy` 或 `mock-openai` 的端口映射。容器网络内的服务名不要改，bootstrap 默认会把渠道指向 `http://mock-openai:8080`。

### 初始化测试数据

```bash
scripts/loadtest/bootstrap.sh
```

默认行为：

- 等待 `http://127.0.0.1:3100/api/status` 可用。
- 如系统未初始化，调用 `POST /api/setup` 创建 Root 用户：用户名 `root`，密码 `LoadtestRoot123!`。
- 登录 Root，保存 session cookie 到 `.loadtest/cookie`。
- 幂等创建或复用渠道 `loadtest-mock-openai`：`type=1`，`base_url=http://mock-openai:8080`，`models=gpt-loadtest`。
- 幂等创建或复用无限额度 Token `loadtest-token`，并把完整 `sk-*` 写入 `.loadtest/token`。

可覆盖的常用变量：

```bash
LOADTEST_BASE_URL=http://127.0.0.1:3100 \
LOADTEST_ROOT_PASSWORD='更换后的Root密码' \
LOADTEST_UPSTREAM_URL=http://mock-openai:8080 \
MODEL=gpt-loadtest \
scripts/loadtest/bootstrap.sh
```

### 运行 k6 场景

默认混合场景为 80% 非流式、20% 流式：

```bash
scripts/loadtest/run.sh mixed
```

其它场景：

```bash
scripts/loadtest/run.sh status_smoke
scripts/loadtest/run.sh chat_non_stream
scripts/loadtest/run.sh chat_stream
```

常用压测参数：

```bash
VUS=50 DURATION=5m scripts/loadtest/run.sh mixed
RAMPING=true VUS=100 DURATION=10m scripts/loadtest/run.sh mixed
```

k6 会生成带时间戳的结果目录，例如：

```text
loadtest-results/20260802T120000Z/
  summary.json
  summary.md
```

阈值默认值：

| 指标 | 默认阈值 |
|------|----------|
| HTTP 错误率 | `< 1%` |
| 非流式 P95 | `< 1500ms` |
| 流式首包近似 P95 | `< 1000ms` |
| `/api/status` P95 | `< 100ms` |

说明：k6 标准 HTTP API 会缓冲响应体，因此流式首包使用 `res.timings.waiting` 作为轻量近似值。后续如果要做严格 TTFT，可再引入专用 streaming client 或自定义 xk6 扩展。

### 采集观测报告

```bash
scripts/loadtest/collect.sh
```

采集内容会写入 `loadtest-results/<timestamp>/report.md`，并包含：

- `/api/status`
- `/api/perf-metrics/summary?hours=1`
- `/api/system-info/instances`
- `/api/system-task/list`
- `mock-openai` 的 `/metrics`
- `docker stats --no-stream`
- PostgreSQL 连接数和基础表统计
- Redis `INFO stats`、`INFO clients`、`INFO memory`
- ClickHouse `system.parts`、`system.mutations` 和 `logs` 表行数
- 最近一次 k6 `summary.json` / `summary.md`，如果存在

初次看报告时建议按这个顺序排查：

1. 先看 k6 错误率和 P95/P99。
2. 再看 `mock-metrics.json`，确认上游是否注入了错误或 429。
3. 如果 mock 延迟稳定但 NexusTok 延迟升高，检查 `docker-stats.txt`、`postgres-connections.txt` 和 Redis ops/内存。
4. 如果 ClickHouse `logs` 表没有写入，先查 `LOG_SQL_DSN` 初始化日志，再判断是否需要继续调整日志库。
5. 如果只有一个 NexusTok 实例有心跳或流量，检查 Caddy 健康状态和 `system-instances.json`。

### Mock 上游参数

`mock-openai` 通过环境变量控制行为，可直接修改 `docker-compose.loadtest.yml` 后重建：

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `MOCK_OPENAI_MODEL` | `gpt-loadtest` | 固定返回模型 |
| `MOCK_OPENAI_TTFT_MS` | `80` | 首包/首 token 延迟 |
| `MOCK_OPENAI_TOKEN_DELAY_MS` | `20` | 每个输出 token 间隔 |
| `MOCK_OPENAI_OUTPUT_TOKENS` | `64` | 输出 token 数 |
| `MOCK_OPENAI_ERROR_RATE` | `0` | 注入 500 错误比例，`0-1` |
| `MOCK_OPENAI_RATE_LIMIT_RATE` | `0` | 注入 429 比例，`0-1` |
| `MOCK_OPENAI_BODY_BYTES` | `0` | 非流式响应额外填充大小 |

修改后执行：

```bash
docker compose -f docker-compose.loadtest.yml up -d --build mock-openai
```

### 真实上游 smoke

真实上游 smoke 默认不会运行，必须显式传入真实地址、key 和模型，建议只跑 10-20 次请求：

```bash
docker compose -f docker-compose.loadtest.yml run --rm \
  -e REAL_BASE_URL="https://your-real-upstream.example" \
  -e REAL_API_KEY="sk-..." \
  -e REAL_MODEL="your-model" \
  -e REAL_ITERATIONS=10 \
  k6 run /scripts/k6/real-upstream-smoke.js
```

真实上游 smoke 只用于上线前确认链路可用，不纳入默认压测，也不要在 CI 或巡检任务中自动执行。

### 停止和清理

停止压测环境并保留压测 volume：

```bash
docker compose -f docker-compose.loadtest.yml down
```

删除压测环境和压测数据卷：

```bash
docker compose -f docker-compose.loadtest.yml down -v
```

`down -v` 只会删除 `docker-compose.loadtest.yml` 声明的独立压测卷，不会删除生产 `docker-compose.yml` 的默认卷。执行前仍建议确认当前目录和 Compose 文件，避免误操作。

## 单容器部署

如果不需要 Compose 编排，可以直接运行官方镜像。

SQLite 模式：

```bash
docker pull c1cadabob/nexustok:latest
mkdir -p /opt/nexustok/data /opt/nexustok/logs
docker rm -f nexustok 2>/dev/null || true

docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  c1cadabob/nexustok:latest
```

启动后检查：

```bash
docker ps | grep nexustok
docker logs -f nexustok
curl -sS http://127.0.0.1:3000/api/status
```

浏览器访问 `http://服务器IP:3000`。如果部署在云服务器上，请确认安全组和系统防火墙已经放行 TCP `3000` 端口。`/opt/nexustok/data` 保存 SQLite、会话密钥文件和运行时数据，更新容器时不要删除。

外部 PostgreSQL 示例：

```bash
docker pull c1cadabob/nexustok:latest
mkdir -p /opt/nexustok/data /opt/nexustok/logs
docker rm -f nexustok 2>/dev/null || true

docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -e SQL_DSN="postgresql://user:password@host:5432/nexustok?sslmode=disable" \
  -e REDIS_CONN_STRING="redis://:password@host:6379/0" \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  c1cadabob/nexustok:latest
```

Docker 镜像只包含 NexusTok 应用进程，不会在同一个容器里内置 PostgreSQL、MySQL 或 Redis。单容器默认使用 SQLite；外接 MySQL/PostgreSQL 时通过 `SQL_DSN` 指向外部服务；需要同时编排 PostgreSQL 和 Redis 时，请优先使用本仓库的 Docker Compose。ClickHouse 只能配置为 `LOG_SQL_DSN` 日志库，不能作为主业务库。

单容器模式和 Compose 模式都使用 NexusTok 原生账号池。账号池分组、池账号和认证文件 API 均由主服务直接提供，不需要额外启动 CLIProxyAPI Sidecar 或 CPA Usage Service。

## 从源码构建生产镜像

仓库根目录的 `Dockerfile` 是完整生产镜像构建流程：

1. 构建默认前端：`web/default -> web/default/dist`
2. 构建经典前端：`web/classic -> web/classic/dist`
3. 编译 Go 后端并嵌入上述构建产物
4. 输出精简运行镜像

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
make dev-api-rebuild
make dev-web
make dev-web-classic
```

注意：`Dockerfile.dev` 只给 Go embed 创建占位前端页面，真正页面应通过前端开发服务器访问。

如果需要重新测试初始化向导，可以使用本地开发专用的重置任务：

```bash
make reset-setup
```

该任务只面向本地开发环境：它会优先检测 `docker-compose.dev.yml` 中正在运行的 PostgreSQL 服务，删除 `setups` 记录、Root 用户和演示模式相关 option 后重启 dev 后端；如果没有运行中的 dev PostgreSQL，则回退到本地 SQLite 文件，默认 `nexustok.db`，也可以通过 `SQLITE_PATH` 或 `DEV_SQLITE_PATH` 指定。不要在生产数据库上执行该任务。

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
7. 注册 API 路由、Web 静态资源路由、原生账号池路由和兼容账号池管理器路由。
8. 启动 HTTP 服务，默认监听 `3000`。

Web 静态资源路由会根据 `common.GetTheme()` 选择默认前端或经典前端。根路径和 SPA 路由由 `router/web-router.go` 的 `NoRoute` 回退到对应主题的 `index.html`。

## 后台定时任务

主服务启动后会在主节点上启动多类后台任务，包括渠道缓存同步、系统配置热更新、数据看板刷新、渠道可用性测试、订阅配额维护、原生账号池凭据刷新、Codex 凭据刷新、上游模型巡检、内置模型仓库 seed，以及 models.dev 模型目录同步。

NexusTok 会把 `modelcatalog/repository` 中已审查的模型、供应商和价格随二进制/Docker 镜像打包。生产启动时主节点只读解析内置仓库，补齐缺失供应商、缺失模型和没有模型级价格的模型，不覆盖手动价格、历史 options 覆盖、表达式计费、固定价格或 `sync_official=0` 的模型。

models.dev 模型目录同步用于每天凌晨优先从 `https://models.dev/catalog.json` 拉取公开模型目录；官网不可用时会降级到 `anomalyco/models.dev` GitHub TOML 目录，GitHub 也不可用时再使用 NexusTok 内置模型仓库兜底。同步时会优先采用 canonical models 的归属方作为本地“供应商”，避免把 Vivgrid 之类的服务商误写成 OpenAI、Anthropic 这类模型原厂。该任务会补齐缺失模型，并在 `sync_official=1` 的官方记录上纠正供应商归属；其它人工维护字段仍保持不动。

相关环境变量：

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `MODELS_DEV_AUTO_SYNC_ENABLED` | `true` | 是否启用 models.dev 每日模型目录同步 |
| `MODELS_DEV_AUTO_SYNC_TIME` | `02:00` | 每日运行时间，格式 `HH:mm`，使用进程当前时区 |
| `MODELS_DEV_SYNC_BASE` | `https://models.dev` | models.dev 基础地址；内网镜像或代理可覆盖 |
| `MODELS_DEV_GITHUB_TREE_URL` | `https://api.github.com/repos/anomalyco/models.dev/git/trees/dev?recursive=1` | 官网失败后的 GitHub TOML tree API；内网镜像可覆盖 |
| `MODELS_DEV_GITHUB_RAW_BASE` | `https://raw.githubusercontent.com/anomalyco/models.dev/dev` | GitHub TOML raw 基础地址；内网镜像可覆盖 |
| `MODELS_DEV_PRICING_PROVIDER_ORDER` | 空 | 自动价格同步的 provider 降级顺序，逗号分隔，例如 `openai,anthropic,google,azure` |
| `MODEL_CATALOG_WRITE_BACK` | `false` | 是否把后台新增/同步模型写回项目内模型仓库；生产必须保持关闭 |
| `MODEL_CATALOG_REPO_DIR` | `modelcatalog/repository` | 开发写回目标目录 |
| `MODEL_CATALOG_GIT_AUTOCOMMIT` | `false` | 预留开关；当前只写文件，不自动 commit/push |

日志中出现以下内容表示任务已经启动：

```text
models.dev model sync task started: time=02:00 source=https://models.dev/catalog.json
```

如果需要临时关闭自动同步：

```yaml
environment:
  - MODELS_DEV_AUTO_SYNC_ENABLED=false
```

如果需要固定自动价格同步的 provider 优先级：

```yaml
environment:
  - MODELS_DEV_PRICING_PROVIDER_ORDER=openai,anthropic,google,azure
```

手动同步和价格策略的完整说明见 [模型同步与价格同步](../features/model-sync-pricing.md)。

开发环境如需把后台新增模型、模型价格保存或 models.dev 同步结果写回仓库文件，可显式开启：

```yaml
environment:
  - MODEL_CATALOG_WRITE_BACK=true
  - MODEL_CATALOG_REPO_DIR=modelcatalog/repository
```

写回只包含模型、供应商和模型价格参数，不保存渠道、账号池、用户、Token、日志、密钥或部署拓扑。提交发布前应审查 `modelcatalog/repository` 的 Git diff，并重新构建镜像或 release 二进制。

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

### Docker 镜像拉取失败

如果 `docker pull c1cadabob/nexustok:latest` 报错：

```text
Get "https://registry-1.docker.io/v2/": net/http: request canceled while waiting for connection
```

这通常是服务器到 Docker Hub registry 的网络链路超时，不是 NexusTok 镜像名称错误。可先确认连通性：

```bash
curl -I --connect-timeout 10 https://registry-1.docker.io/v2/
getent hosts registry-1.docker.io
```

国内云服务器建议配置 Docker registry mirror。以阿里云为例，在 **容器镜像服务 ACR → 镜像工具 → 镜像加速器** 获取专属地址后写入：

```bash
mkdir -p /etc/docker
cp /etc/docker/daemon.json /etc/docker/daemon.json.bak.$(date +%Y%m%d%H%M%S) 2>/dev/null || true

cat > /etc/docker/daemon.json <<'EOF'
{
  "registry-mirrors": [
    "https://你的镜像加速器地址"
  ],
  "dns": [
    "223.5.5.5",
    "119.29.29.29"
  ]
}
EOF

systemctl daemon-reload
systemctl restart docker
docker info | grep -A 10 "Registry Mirrors"
docker pull c1cadabob/nexustok:latest
```

如果目标服务器仍无法访问 Docker Hub，可在一台能联网的机器上导出镜像，再传到服务器导入：

```bash
docker pull c1cadabob/nexustok:latest
docker save c1cadabob/nexustok:latest -o nexustok-latest.tar

# 传到目标服务器后执行
docker load -i /root/nexustok-latest.tar
docker images | grep nexustok
```

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

### 原生账号池接口不可用

账号池认证文件、原生分组和池账号 API 都由 NexusTok 主服务提供。排障时先检查主服务日志和 `/api/account-pool/*` 接口：

检查命令示例：

```bash
docker logs --tail 200 nexustok
curl -sS http://127.0.0.1:3000/api/status
```

热重载环境对应主服务容器：

```bash
docker logs --tail 200 nexustok-api-hot
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
| 拉取失败 | 服务器无法访问 `https://models.dev/catalog.json` 且 GitHub fallback 也不可用 | 检查 DNS、代理、防火墙，或配置 `MODELS_DEV_SYNC_BASE`、`MODELS_DEV_GITHUB_TREE_URL`、`MODELS_DEV_GITHUB_RAW_BASE` |
| 页面提示使用内置仓库 | models.dev 官网和 GitHub fallback 均不可用 | 可先使用内置仓库兜底；需要最新模型时修复网络或更新 `modelcatalog/repository` 后重新构建 |
| 同步后模型没有变化 | 本地已存在这些模型，任务不会覆盖已有记录 | 在后台模型页面检查模型是否已存在，必要时使用手动编辑或覆盖同步 |
| 供应商显示成 Vivgrid | 旧数据是在 canonical 归属修复前同步的，或未走 canonical models 路径 | 重新执行 models.dev 同步预览并对相关模型应用覆盖，或在后台模型页手动修正供应商 |
| 价格没有被覆盖 | 模型已有手动价格或历史 options 覆盖，自动同步默认保护本地配置 | 在后台手动同步时开启“允许覆盖手动定价”，或检查 `ModelPricingSource` |
| provider 价格不是期望来源 | 未配置 provider 降级顺序，或 provider ID/name 没有匹配 | 设置 `MODELS_DEV_PRICING_PROVIDER_ORDER`，后台每行一个 provider，环境变量用逗号分隔 |

管理员也可以通过后台登录态调用预览接口确认 models.dev 源是否可访问：

```bash
curl -H "NexusTok-User: <管理员用户ID>" \
  "http://127.0.0.1:3000/api/models/sync_upstream/preview?source=models.dev"
```

## 更新流程

生产 Compose 镜像更新：

```bash
docker compose pull nexustok
docker compose up -d nexustok
docker compose ps
curl -sS http://127.0.0.1:3000/api/status
```

旧版 Docker Compose 可使用：

```bash
docker-compose pull nexustok
docker-compose up -d nexustok
docker-compose ps
```

单容器镜像更新：

```bash
docker pull c1cadabob/nexustok:latest
docker stop nexustok
docker rm nexustok

# 使用原来的 /opt/nexustok/data 和 /opt/nexustok/logs 挂载目录重新启动。
docker run --name nexustok -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -e SESSION_SECRET_FILE=/data/session_secret \
  -v /opt/nexustok/data:/data \
  -v /opt/nexustok/logs:/app/logs \
  c1cadabob/nexustok:latest
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
