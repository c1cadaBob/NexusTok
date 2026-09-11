#!/bin/bash
# NexusTok 一键部署脚本
# 用于生产环境快速部署 NexusTok + PostgreSQL + Redis

set -e

echo "=========================================="
echo "NexusTok 一键部署脚本"
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

echo -e "${GREEN}✓${NC} Docker 环境检查通过"
echo ""

# 检查 docker-compose.yml 是否存在
if [ ! -f "docker-compose.yml" ]; then
    echo -e "${RED}错误: 未找到 docker-compose.yml 文件${NC}"
    echo "请确保在项目根目录下运行此脚本"
    exit 1
fi

echo -e "${GREEN}✓${NC} 配置文件检查通过"
echo ""

# 检查端口占用
echo "检查端口占用..."
PORTS=(3000 5432 6379)
PORT_NAMES=("NexusTok API" "PostgreSQL" "Redis")
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
    echo -e "${YELLOW}提示: 如果这些端口被旧版本的 NexusTok 占用，请先停止旧容器${NC}"
    read -p "是否继续部署? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "已取消部署"
        exit 1
    fi
fi

echo -e "${GREEN}✓${NC} 端口检查完成"
echo ""

# 拉取最新镜像
echo "拉取最新镜像..."
docker compose pull

echo -e "${GREEN}✓${NC} 镜像拉取完成"
echo ""

# 启动服务
echo "启动服务..."
docker compose up -d

echo -e "${GREEN}✓${NC} 服务已启动"
echo ""

# 等待服务就绪
echo "等待服务就绪..."
MAX_RETRIES=30
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if docker compose ps | grep -q "healthy"; then
        break
    fi
    echo -n "."
    sleep 2
    RETRY_COUNT=$((RETRY_COUNT + 1))
done

echo ""

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo -e "${YELLOW}警告: 服务启动超时，请检查日志${NC}"
else
    echo -e "${GREEN}✓${NC} 服务就绪"
fi

echo ""

# 检查服务状态
echo "服务状态:"
docker compose ps

echo ""
echo -e "${GREEN}=========================================="
echo "部署完成!"
echo "==========================================${NC}"
echo ""
echo -e "${BLUE}访问地址:${NC}"
echo "  • NexusTok:   http://localhost:3000"
echo "  • API 状态:   http://localhost:3000/api/status"
echo ""
echo -e "${BLUE}数据库连接信息:${NC}"
echo "  • PostgreSQL: localhost:5432"
echo "    - 数据库: nexustok"
echo "    - 用户: root"
echo "    - 密码: 123456 (⚠️ 生产环境请修改密码)"
echo "  • Redis:      localhost:6379"
echo "    - 密码: 123456 (⚠️ 生产环境请修改密码)"
echo ""
echo -e "${RED}⚠️  安全提示:${NC}"
echo "  1. 请立即修改 docker-compose.yml 中的默认密码"
echo "  2. 建议配置防火墙，限制数据库端口的外部访问"
echo "  3. 定期备份 PostgreSQL 数据库"
echo "  4. 生产环境请设置 SESSION_SECRET 环境变量"
echo ""
echo -e "${BLUE}常用命令:${NC}"
echo "  • 查看日志:     docker compose logs -f"
echo "  • 查看 API:     docker compose logs -f nexustok"
echo "  • 停止服务:     docker compose down"
echo "  • 重启服务:     docker compose restart"
echo "  • 备份数据库:   docker exec postgres pg_dump -U root nexustok > backup.sql"
echo ""
echo -e "${YELLOW}首次使用:${NC}"
echo "  1. 访问 http://localhost:3000"
echo "  2. 按照设置向导完成初始化"
echo "  3. 创建管理员账户"
echo "  4. 配置上游 API 渠道"
echo ""
