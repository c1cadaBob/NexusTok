#!/bin/bash
# NexusTok 热更新环境清理脚本
# 用于清理旧的热更新开发环境

set -e

echo "=========================================="
echo "NexusTok 热更新环境清理脚本"
echo "=========================================="
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查是否有运行的容器
echo "检查运行中的热更新容器..."
CONTAINERS=$(docker ps -a --filter "name=nexustok-hot" --filter "name=nexustok-api-hot" --filter "name=nexustok-frontend-watch" --format "{{.Names}}" | sort)

if [ -z "$CONTAINERS" ]; then
    echo -e "${GREEN}✓${NC} 没有找到热更新相关容器"
else
    echo -e "${YELLOW}找到以下容器:${NC}"
    echo "$CONTAINERS" | while read -r container; do
        echo "  - $container"
    done
    echo ""

    read -p "是否停止并删除这些容器? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "停止容器..."
        echo "$CONTAINERS" | xargs -r docker stop
        echo -e "${GREEN}✓${NC} 容器已停止"

        echo "删除容器..."
        echo "$CONTAINERS" | xargs -r docker rm
        echo -e "${GREEN}✓${NC} 容器已删除"
    else
        echo "跳过容器清理"
    fi
fi

echo ""

# 检查数据卷
echo "检查热更新相关数据卷..."
VOLUMES=$(docker volume ls --filter "name=nexustok-hot" --filter "name=nexustok_hot" --format "{{.Name}}" | sort)

if [ -z "$VOLUMES" ]; then
    echo -e "${GREEN}✓${NC} 没有找到热更新相关数据卷"
else
    echo -e "${YELLOW}找到以下数据卷:${NC}"
    echo "$VOLUMES" | while read -r volume; do
        echo "  - $volume"
    done
    echo ""
    echo -e "${RED}警告: 删除数据卷将永久删除其中的数据库数据!${NC}"
    read -p "是否删除这些数据卷? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "删除数据卷..."
        echo "$VOLUMES" | xargs -r docker volume rm
        echo -e "${GREEN}✓${NC} 数据卷已删除"
    else
        echo "跳过数据卷清理"
    fi
fi

echo ""

# 检查自定义镜像
echo "检查热更新相关镜像..."
IMAGES=$(docker images --filter "reference=nexustok-api-hot" --filter "reference=nexustok-hot" --format "{{.Repository}}:{{.Tag}}" | grep -E "nexustok.*hot" || true)

if [ -z "$IMAGES" ]; then
    echo -e "${GREEN}✓${NC} 没有找到热更新相关镜像"
else
    echo -e "${YELLOW}找到以下镜像:${NC}"
    echo "$IMAGES" | while read -r image; do
        SIZE=$(docker images --format "{{.Size}}" "$image")
        echo "  - $image ($SIZE)"
    done
    echo ""

    read -p "是否删除这些镜像? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "删除镜像..."
        echo "$IMAGES" | xargs -r docker rmi
        echo -e "${GREEN}✓${NC} 镜像已删除"
    else
        echo "跳过镜像清理"
    fi
fi

echo ""

# 清理未使用的网络
echo "清理未使用的 Docker 网络..."
docker network prune -f --filter "label=com.docker.compose.project=nexustok-hot" 2>/dev/null || true
echo -e "${GREEN}✓${NC} 未使用的网络已清理"

echo ""
echo -e "${GREEN}=========================================="
echo "清理完成!"
echo "==========================================${NC}"
echo ""
echo "提示: 如需启动新的热更新开发环境，请运行:"
echo "  bash scripts/dev-hot.sh"
