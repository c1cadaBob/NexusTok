# NexusTok 开发环境设置指南

## 概述

本指南介绍如何在本地搭建 NexusTok 的开发环境，使用 **PostgreSQL + Redis + 热更新** 实现高效的开发体验。

## 架构

开发环境包含以下服务：

- **API 服务**：使用 Air 实现 Go 代码热更新
- **PostgreSQL**：主数据库
- **Redis**：缓存和会话存储
- **Frontend**：Vite 开发服务器（支持 HMR）

## 快速开始

### 1. 前置要求

- Docker 20.10+
- Docker Compose 2.0+
- Git

### 2. 启动开发环境

```bash
# 一键启动所有服务
bash scripts/dev-start.sh
```

启动后，服务地址：

- **API**: http://localhost:3000
- **前端**: http://localhost:5173
- **PostgreSQL**: localhost:5432
- **Redis**: localhost:6379

### 3. 查看日志

```bash
# 查看 API 日志（实时）
docker logs -f nexustok-dev-api

# 查看前端日志
docker logs -f nexustok-dev-frontend

# 查看所有服务日志
docker-compose -f docker-compose.dev.yml logs -f
```

### 4. 停止服务

```bash
# 停止所有服务
docker-compose -f docker-compose.dev.yml down

# 停止并删除数据卷
docker-compose -f docker-compose.dev.yml down -v
```

## 热更新功能

### 后端热更新

后端使用 [Air](https://github.com/cosmtrek/air) 实现热更新：

1. 修改 `.go` 文件
2. Air 自动检测变化
3. 自动重新编译
4. 自动重启服务

**配置文件**: `.air.toml`

**监控的文件类型**: `go`, `tpl`, `tmpl`, `html`

**排除的目录**: `assets`, `tmp`, `vendor`, `testdata`, `web`, `docs`, `.git`, `data`, `logs`, `relaykit`

### 前端热更新

前端使用 Vite 的 HMR（热模块替换）：

1. 修改 `.tsx`、`.ts`、`.css` 等文件
2. Vite 自动检测变化
3. 浏览器自动刷新（无需手动刷新）

## 环境变量

开发环境的配置文件：`.env.dev`

```bash
# 数据库连接
SQL_DSN=host=nexustok-dev-postgres port=5432 user=nexustok password=nexustok_dev_2024 dbname=nexustok sslmode=disable TimeZone=Asia/Shanghai

# Redis 连接
REDIS_CONN_STRING=redis://nexustok-dev-redis:6379

# 会话密钥（生产环境请修改）
SESSION_SECRET=dev-session-secret-change-in-production

# API 端口
PORT=3000
```

## 数据库管理

### 连接到 PostgreSQL

```bash
# 使用 psql 连接
docker exec -it nexustok-dev-postgres psql -U nexustok -d nexustok

# 查看所有表
\dt

# 退出
\q
```

### 数据库迁移

应用会在启动时自动执行数据库迁移，无需手动操作。

### 重置数据库

```bash
# 停止服务并删除数据卷
docker-compose -f docker-compose.dev.yml down -v

# 重新启动（会创建新的数据库）
bash scripts/dev-start.sh
```

## Redis 管理

### 连接到 Redis

```bash
# 使用 redis-cli 连接
docker exec -it nexustok-dev-redis redis-cli

# 测试连接
ping
# 应返回: PONG

# 查看所有 key
keys *

# 退出
exit
```

### 清空 Redis 缓存

```bash
docker exec -it nexustok-dev-redis redis-cli FLUSHALL
```

## 常见问题

### 1. 端口被占用

如果端口 3000、5173、5432 或 6379 被占用：

```bash
# 查看端口占用
lsof -i :3000
lsof -i :5173
lsof -i :5432
lsof -i :6379

# 修改 docker-compose.dev.yml 中的端口映射
```

### 2. 容器启动失败

```bash
# 查看详细日志
docker-compose -f docker-compose.dev.yml logs

# 重新构建镜像
docker-compose -f docker-compose.dev.yml build --no-cache

# 重新启动
docker-compose -f docker-compose.dev.yml up -d
```

### 3. 热更新不工作

```bash
# 检查 Air 是否正常运行
docker logs nexustok-dev-api | grep -i "watching"

# 重启 API 容器
docker-compose -f docker-compose.dev.yml restart api

# 检查文件权限
ls -la /opt/project/NexusTok
```

### 4. 数据库连接失败

```bash
# 检查 PostgreSQL 容器状态
docker ps | grep postgres

# 检查健康状态
docker inspect nexustok-dev-postgres | grep -i health

# 查看 PostgreSQL 日志
docker logs nexustok-dev-postgres
```

## 开发工作流

### 典型的开发流程

1. **启动开发环境**
   ```bash
   bash scripts/dev-start.sh
   ```

2. **修改代码**
   - 后端：编辑 `.go` 文件，Air 自动重启
   - 前端：编辑 `.tsx` 文件，浏览器自动刷新

3. **测试功能**
   - 访问 http://localhost:5173 查看前端
   - 使用 curl 或 Postman 测试 API

4. **查看日志**
   ```bash
   docker logs -f nexustok-dev-api
   ```

5. **提交代码**
   ```bash
   git add .
   git commit -m "feat: 新功能描述"
   git push
   ```

### 调试技巧

**后端调试**：

```bash
# 进入容器
docker exec -it nexustok-dev-api sh

# 查看 Go 模块
go list -m all

# 手动运行
go run .
```

**前端调试**：

在浏览器中打开开发者工具（F12），查看 Console 和 Network 面板。

## 性能优化

### Go 模块缓存

首次启动时，Go 会下载所有依赖，这可能需要几分钟。后续启动会使用缓存的模块。

### Docker 卷挂载

开发环境使用卷挂载实现代码同步，性能略低于生产环境。如果遇到性能问题，可以考虑：

1. 使用 Docker 的 `:cached` 或 `:delegated` 挂载选项
2. 仅挂载必要的目录
3. 排除不需要同步的目录（如 `node_modules`）

## 生产环境部署

开发环境配置 **不适合** 生产环境。生产环境部署请参考：

- `docker-compose.yml`：生产环境配置
- `README.md`：完整的部署文档

## 贡献指南

在提交 PR 之前：

1. 确保代码通过 lint 检查
2. 运行所有测试
3. 更新相关文档
4. 使用有意义的 commit 信息

## 技术栈

- **后端**: Go 1.25.1
- **数据库**: PostgreSQL 15
- **缓存**: Redis 7
- **前端**: React + TypeScript + Vite
- **热更新工具**: Air (后端), Vite HMR (前端)

## 相关文档

- [README.md](README.md) - 项目概述和生产部署
- [AGENTS.md](AGENTS.md) - 开发规范
- [docker-compose.dev.yml](docker-compose.dev.yml) - 开发环境配置
- [.air.toml](.air.toml) - Air 热更新配置

## 许可证

MIT License
