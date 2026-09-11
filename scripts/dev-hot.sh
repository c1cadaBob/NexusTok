#!/bin/bash
# NexusTok 热更新开发环境启动脚本

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="${PROJECT_ROOT}/docker-compose.hot.yml"
HOT_PORT="${HOT_PORT:-3003}"
export HOT_PORT

cd "${PROJECT_ROOT}"

echo "=========================================="
echo "NexusTok 热更新开发环境"
echo "=========================================="
echo ""

if ! command -v docker >/dev/null 2>&1; then
    echo "错误：未找到 Docker，请先安装 Docker"
    exit 1
fi

if ! docker info >/dev/null 2>&1; then
    echo "错误：Docker 服务未运行，请先启动 Docker"
    exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
    echo "错误：未找到 Docker Compose，请先安装 Docker Compose"
    exit 1
fi

if command -v lsof >/dev/null 2>&1 && lsof -Pi ":${HOT_PORT}" -sTCP:LISTEN -t >/dev/null 2>&1; then
    echo "错误：端口 ${HOT_PORT} 已被占用，请先停止占用该端口的服务"
    exit 1
fi

wait_for_postgres() {
    echo "等待 PostgreSQL 健康检查通过..."
    for _ in $(seq 1 60); do
        health_status="$(docker inspect -f '{{.State.Health.Status}}' nexustok-hot-pg 2>/dev/null || true)"
        if [ "${health_status}" = "healthy" ]; then
            return 0
        fi
        sleep 2
    done

    echo "错误：等待 PostgreSQL 健康检查超时"
    docker compose -f "${COMPOSE_FILE}" ps
    return 1
}

wait_for_frontend() {
    echo "等待前端产物发布..."
    for _ in $(seq 1 90); do
        if [ -f "${PROJECT_ROOT}/web/dist/index.html" ] && \
            [ -f "${PROJECT_ROOT}/web/dist/.nexustok-hot-dist" ] && \
            [ ! -e "${PROJECT_ROOT}/tmp/frontend-dist-publish.lock" ]; then
            return 0
        fi
        sleep 2
    done

    echo "错误：等待前端产物发布超时"
    docker compose -f "${COMPOSE_FILE}" logs --tail=80 frontend-watch
    return 1
}

wait_for_api() {
    echo "等待 NexusTok API 就绪..."
    for _ in $(seq 1 90); do
        if curl -fsS "http://localhost:${HOT_PORT}/api/status" 2>/dev/null | \
            grep -q '"success":[[:space:]]*true'; then
            return 0
        fi
        sleep 2
    done

    echo "错误：等待 NexusTok API 就绪超时"
    docker compose -f "${COMPOSE_FILE}" logs --tail=100 nexustok
    return 1
}

if ! command -v curl >/dev/null 2>&1; then
    echo "错误：启动脚本等待 API 就绪需要 curl"
    exit 1
fi

echo "拉取热更新环境所需基础镜像..."
docker compose -f "${COMPOSE_FILE}" pull frontend-watch postgres redis

echo "构建后端热更新镜像..."
docker compose -f "${COMPOSE_FILE}" build nexustok

echo "启动 PostgreSQL、Redis、后端和前端监听服务..."
docker compose -f "${COMPOSE_FILE}" up -d --force-recreate --remove-orphans

wait_for_postgres
wait_for_frontend
wait_for_api

echo ""
echo "服务状态："
docker compose -f "${COMPOSE_FILE}" ps

echo ""
echo "访问地址："
echo "  项目首页：http://localhost:${HOT_PORT}"
echo "  API 状态：http://localhost:${HOT_PORT}/api/status"
echo ""
echo "常用命令："
echo "  查看全部日志：docker compose -f docker-compose.hot.yml logs -f"
echo "  查看后端日志：docker compose -f docker-compose.hot.yml logs -f nexustok"
echo "  查看前端日志：docker compose -f docker-compose.hot.yml logs -f frontend-watch"
echo "  停止服务：docker compose -f docker-compose.hot.yml down"
echo "  清理环境：bash scripts/cleanup-hot.sh --purge-data"
echo ""
echo "提示：修改 Go 或 web 源码后，相关容器会自动重新构建并加载。"
