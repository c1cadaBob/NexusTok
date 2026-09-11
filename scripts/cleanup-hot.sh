#!/bin/bash
# NexusTok 热更新环境清理脚本
#
# 默认只清理 NexusTok 热更新 Docker 资源并保留宿主机数据。
# 使用 --purge-data 时，同时清理热更新数据库卷、data、logs、前端依赖和构建产物。

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="${PROJECT_ROOT}/docker-compose.hot.yml"
PROJECT_NAME="nexustok-hot"
PURGE_DATA=false

if [ "$#" -gt 1 ] || { [ "$#" -eq 1 ] && [ "$1" != "--purge-data" ]; }; then
    echo "用法：bash scripts/cleanup-hot.sh [--purge-data]"
    exit 1
fi

if [ "$#" -eq 1 ]; then
    PURGE_DATA=true
fi

cd "${PROJECT_ROOT}"

echo "=========================================="
echo "NexusTok 热更新环境清理"
echo "=========================================="
echo ""

echo "停止并删除 ${PROJECT_NAME} Compose 项目资源..."
if [ "${PURGE_DATA}" = true ]; then
    docker compose -p "${PROJECT_NAME}" -f "${COMPOSE_FILE}" down --volumes --remove-orphans
else
    docker compose -p "${PROJECT_NAME}" -f "${COMPOSE_FILE}" down --remove-orphans
fi

echo "清理同一 Compose 项目的残留容器..."
mapfile -t leftover_containers < <(
    docker ps -aq --filter "label=com.docker.compose.project=${PROJECT_NAME}"
)
if [ "${#leftover_containers[@]}" -gt 0 ]; then
    docker rm -f "${leftover_containers[@]}"
fi

for container in \
    nexustok-api-hot \
    nexustok-frontend-watch \
    nexustok-hot-pg \
    nexustok-hot-redis \
    nexustok-dev \
    nexustok-dev-api \
    nexustok-dev-pg \
    nexustok-dev-postgres \
    nexustok-dev-redis \
    nexustok-dev-frontend; do
    if docker container inspect "${container}" >/dev/null 2>&1; then
        docker rm -f "${container}"
    fi
done

echo "清理 NexusTok 热更新专用镜像..."
for image in nexustok-api-hot:local nexustok-dev:local; do
    if docker image inspect "${image}" >/dev/null 2>&1; then
        docker image rm "${image}"
    fi
done

echo "检查并清理未被其他容器使用的热更新基础镜像..."
for image in postgres:15 postgres:15-alpine redis:7-alpine oven/bun:1; do
    if ! docker image inspect "${image}" >/dev/null 2>&1; then
        continue
    fi

    used_by_other_container=false
    while IFS='|' read -r container_name container_image; do
        if [ "${container_image}" = "${image}" ]; then
            case "${container_name}" in
                nexustok-api-hot|nexustok-frontend-watch|nexustok-hot-pg|nexustok-hot-redis)
                    ;;
                *)
                    used_by_other_container=true
                    ;;
            esac
        fi
    done < <(docker ps -a --format '{{.Names}}|{{.Image}}')

    if [ "${used_by_other_container}" = false ]; then
        docker image rm "${image}" || true
    else
        echo "保留 ${image}：仍被其他项目容器使用"
    fi
done

for network in \
    nexustok-hot_hot-network \
    nexustok-hot_default; do
    if docker network inspect "${network}" >/dev/null 2>&1; then
        docker network rm "${network}" >/dev/null
    fi
done

if [ "${PURGE_DATA}" = true ]; then
    echo "清理热更新宿主机数据、日志、前端依赖和构建产物..."
    docker run --rm --network none --user 0:0 \
        --mount "type=bind,src=${PROJECT_ROOT},dst=/workspace" \
        alpine:3.20 \
        sh -c '
            for directory in \
                /workspace/data \
                /workspace/logs; do
                if [ -d "${directory}" ]; then
                    find "${directory}" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} +
                fi
            done
            rm -rf /workspace/web/dist /workspace/web/node_modules
        '

    mkdir -p "${PROJECT_ROOT}/data" "${PROJECT_ROOT}/logs"
fi

echo ""
echo "热更新环境清理完成。"
