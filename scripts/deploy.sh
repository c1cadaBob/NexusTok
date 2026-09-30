#!/usr/bin/env bash
# NexusTok 生产一键部署脚本
# 用于部署 NexusTok + PostgreSQL + Redis

set -Eeuo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

cd "$(dirname "$0")/.."

echo "=========================================="
echo "NexusTok 生产一键部署脚本"
echo "=========================================="
echo ""

if ! command -v docker >/dev/null 2>&1; then
    echo -e "${RED}错误: 未找到 Docker，请先安装 Docker${NC}"
    exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
    echo -e "${RED}错误: 未找到 Docker Compose，请先安装 Docker Compose${NC}"
    exit 1
fi

if [ ! -f "docker-compose.yml" ]; then
    echo -e "${RED}错误: 未找到 docker-compose.yml 文件${NC}"
    exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
    echo -e "${RED}错误: 未找到 curl，健康检查需要使用 curl${NC}"
    exit 1
fi

echo -e "${GREEN}✓${NC} Docker、Compose 和配置文件检查通过"

if [ -L ".env" ]; then
    echo -e "${RED}错误: .env 不能是符号链接，请先移除或改为普通文件${NC}"
    exit 1
fi

umask 077
if [ ! -e ".env" ]; then
    : > .env
fi
chmod 600 .env

generate_password() {
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex 32
        return
    fi
    od -An -N32 -tx1 /dev/urandom | tr -d '[:space:]'
}

ensure_password() {
    local name="$1"
    local line_count
    local line
    local value

    line_count="$(grep -cE "^${name}=" .env || true)"
    if [ "$line_count" -gt 1 ]; then
        echo -e "${RED}错误: .env 中存在多个 ${name} 配置，请保留一个${NC}"
        exit 1
    fi

    if [ "$line_count" -eq 1 ]; then
        line="$(grep -E "^${name}=" .env)"
        value="${line#*=}"
        if [ -z "$value" ]; then
            echo -e "${RED}错误: .env 中的 ${name} 为空，请设置后重试${NC}"
            exit 1
        fi
        echo -e "${GREEN}✓${NC} 保留已有 ${name}"
        return
    fi

    value="$(generate_password)"
    if [ -z "$value" ]; then
        echo -e "${RED}错误: 无法生成 ${name}${NC}"
        exit 1
    fi
    printf '%s=%s\n' "$name" "$value" >> .env
    echo -e "${GREEN}✓${NC} 已生成 ${name}"
}

ensure_password POSTGRES_PASSWORD
ensure_password REDIS_PASSWORD
chmod 600 .env
mkdir -p /opt/nexustok/data /opt/nexustok/logs

echo -e "${GREEN}✓${NC} .env 权限已设置为 0600，数据目录已准备"

is_port_in_use() {
    local port="$1"
    if command -v lsof >/dev/null 2>&1; then
        lsof -nP -iTCP:"$port" -sTCP:LISTEN -t >/dev/null 2>&1
        return
    fi
    if command -v ss >/dev/null 2>&1; then
        ss -ltnH | awk -v port=":$port" '$4 ~ port "$" { found = 1 } END { exit !found }'
        return
    fi
    return 1
}

echo "检查应用端口..."
if is_port_in_use 3030; then
    echo -e "${YELLOW}警告: 端口 3030 已被占用${NC}"
    if [ ! -t 0 ]; then
        echo -e "${RED}错误: 非交互环境无法确认是否继续${NC}"
        exit 1
    fi
    read -r -p "是否继续部署并让 Compose 报告端口冲突? (y/N): " reply
    if [[ ! "$reply" =~ ^[Yy]$ ]]; then
        echo "已取消部署"
        exit 1
    fi
fi
echo -e "${GREEN}✓${NC} 应用端口检查完成"

echo "校验 Compose 配置..."
docker compose config >/dev/null
echo -e "${GREEN}✓${NC} Compose 配置校验通过"

echo "拉取生产镜像..."
docker compose pull
echo -e "${GREEN}✓${NC} 镜像拉取完成"

echo "启动服务..."
docker compose up -d
echo -e "${GREEN}✓${NC} 服务启动命令已完成"

service_is_healthy() {
    local service="$1"
    local container_id
    local state
    local health

    container_id="$(docker compose ps -q "$service")"
    [ -n "$container_id" ] || return 1
    state="$(docker inspect -f '{{.State.Status}}' "$container_id")"
    health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container_id")"
    [ "$state" = "running" ] && [ "$health" = "healthy" ]
}

echo "等待 postgres、redis 和 nexustok 健康..."
max_retries=60
retry_count=0
while [ "$retry_count" -lt "$max_retries" ]; do
    if service_is_healthy postgres && service_is_healthy redis && service_is_healthy nexustok; then
        break
    fi
    printf '.'
    sleep 2
    retry_count=$((retry_count + 1))
done
echo ""

if [ "$retry_count" -eq "$max_retries" ]; then
    echo -e "${RED}错误: 服务健康检查超时${NC}"
    docker compose ps
    docker compose logs --tail=80 nexustok postgres redis
    exit 1
fi

status_response="$(curl --fail --silent --show-error --max-time 10 http://127.0.0.1:3030/api/status)"
if ! grep -q '"success":[[:space:]]*true' <<< "$status_response"; then
    echo -e "${RED}错误: /api/status 未返回成功响应${NC}"
    printf '%s\n' "$status_response"
    exit 1
fi

echo -e "${GREEN}✓${NC} 三个服务均已健康，API 状态检查通过"
echo ""
docker compose ps
echo ""
echo -e "${GREEN}=========================================="
echo "部署完成!"
echo "==========================================${NC}"
echo ""
echo -e "${BLUE}访问地址:${NC}"
echo "  • NexusTok:   http://localhost:3030"
echo "  • API 状态:   http://localhost:3030/api/status"
echo "  • 初始化向导: http://localhost:3030/setup"
echo ""
echo -e "${BLUE}持久化目录:${NC}"
echo "  • 应用数据和会话密钥: /opt/nexustok/data"
echo "  • 应用日志:           /opt/nexustok/logs"
echo "  • 数据库和 Redis:     Docker named volume / 内部网络"
echo ""
echo -e "${YELLOW}首次使用:${NC}"
echo "  1. 访问 http://localhost:3030"
echo "  2. 按照设置向导完成初始化"
echo "  3. 创建管理员账户"
echo "  4. 配置上游 API 渠道"
echo ""
echo -e "${BLUE}常用命令:${NC}"
echo "  • 查看日志:     docker compose logs -f"
echo "  • 查看 API:     docker compose logs -f nexustok"
echo "  • 停止服务:     docker compose down"
echo "  • 重启服务:     docker compose up -d"
echo "  • 备份数据库:   docker compose exec -T postgres pg_dump -U root nexustok > backup.sql"
echo ""
echo -e "${YELLOW}安全提示:${NC}"
echo "  • .env 含数据库和 Redis 密码，权限已设置为 0600，请纳入安全备份但不要提交到 Git"
echo "  • PostgreSQL 和 Redis 未映射宿主机端口，仅在 Compose 内部网络可访问"
echo "  • Docker socket 等同宿主机 Docker 管理权限，只应部署在可信管理员可访问的服务器"
echo "  • SQLite 数据不会自动迁移到 PostgreSQL，已有数据切换前请先备份并单独完成迁移"
echo ""
