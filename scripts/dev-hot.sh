#!/bin/bash
# NexusTok 热更新开发环境启动脚本

set -e

echo "=========================================="
echo "NexusTok 热更新开发环境"
echo "=========================================="
echo ""

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 检查 Docker 和 Docker Compose
if ! command -v docker &> /dev/null; then
    echo -e "${RED}错误: 未找到 Docker，请先安装 Docker${NC}"
    exit 1
fi

if ! docker compose version &> /dev/null; then
    echo -e "${RED}错误: 未找到 Docker Compose，请先安装 Docker Compose${NC}"
    exit 1
fi

# 检查端口占用
echo "检查端口占用..."
PORTS=(3000 5432 6379 5173)
PORT_NAMES=("API" "PostgreSQL" "Redis" "Frontend")
PORTS_OK=true

for i in "${!PORTS[@]}"; do
    PORT=${PORTS[$i]}
    NAME=${PORT_NAMES[$i]}
    if lsof -Pi :$PORT -sTCP:LISTEN -t >/dev/null 2>&1 ; then
        echo -e "${YELLOW}警告: 端口 $PORT ($NAME) 已被占用${NC}"
        PORTS_OK=false
    fi
done

if [ "$PORTS_OK" = false ]; then
    echo ""
    read -p "是否继续启动? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "已取消启动"
        exit 1
    fi
fi

echo -e "${GREEN}✓${NC} 端口检查完成"
echo ""

# 构建开发镜像
echo "构建热更新开发镜像..."
docker compose -f docker-compose.hot.yml build

echo -e "${GREEN}✓${NC} 镜像构建完成"
echo ""

# 启动服务
echo "启动开发环境..."
docker compose -f docker-compose.hot.yml up -d

echo -e "${GREEN}✓${NC} 服务已启动"
echo ""

# 等待服务就绪
echo "等待服务就绪..."
sleep 5

# 检查服务状态
echo ""
echo "服务状态:"
docker compose -f docker-compose.hot.yml ps

echo ""
echo -e "${GREEN}=========================================="
echo "开发环境启动成功!"
echo "==========================================${NC}"
echo ""
echo -e "${BLUE}访问地址:${NC}"
echo "  • API 服务:   http://localhost:3000"
echo "  • 前端开发:   http://localhost:5173"
echo "  • API 状态:   http://localhost:3000/api/status"
echo ""
echo -e "${BLUE}数据库连接:${NC}"
echo "  • PostgreSQL: localhost:5432"
echo "    - 用户: root"
echo "    - 密码: 123456"
echo "    - 数据库: nexustok"
echo "  • Redis:      localhost:6379"
echo "    - 密码: 123456"
echo ""
echo -e "${BLUE}常用命令:${NC}"
echo "  • 查看日志:   docker compose -f docker-compose.hot.yml logs -f"
echo "  • 查看 API:   docker compose -f docker-compose.hot.yml logs -f nexustok"
echo "  • 查看前端:   docker compose -f docker-compose.hot.yml logs -f frontend"
echo "  • 停止服务:   docker compose -f docker-compose.hot.yml down"
echo "  • 重启服务:   docker compose -f docker-compose.hot.yml restart"
echo "  • 清理环境:   bash scripts/cleanup-hot.sh"
echo ""
echo -e "${YELLOW}提示:${NC}"
echo "  • 修改 Go 代码后会自动重新编译（Air 热更新）"
echo "  • 修改前端代码后会自动刷新页面（Bun HMR）"
echo "  • 数据库数据保存在 Docker 卷中，不会因为重启而丢失"
echo ""

# 询问是否查看日志
read -p "是否查看实时日志? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    docker compose -f docker-compose.hot.yml logs -f
fi
