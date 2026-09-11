#!/bin/bash

# NexusTok 开发环境启动脚本
# 使用 PostgreSQL + Redis + 热更新

set -e

echo "🚀 启动 NexusTok 开发环境..."
echo ""

# 检查 Docker 是否运行
if ! docker info > /dev/null 2>&1; then
    echo "❌ 错误: Docker 未运行，请先启动 Docker"
    exit 1
fi

# 进入项目根目录
cd "$(dirname "$0")/.."

# 启动所有服务
echo "📦 启动服务容器..."
docker-compose -f docker-compose.dev.yml up -d

# 等待服务就绪
echo ""
echo "⏳ 等待服务启动..."
sleep 5

# 检查服务状态
echo ""
echo "📊 服务状态:"
docker-compose -f docker-compose.dev.yml ps

echo ""
echo "✅ 开发环境启动完成！"
echo ""
echo "📍 服务地址:"
echo "   - API 服务: http://localhost:3000"
echo "   - 前端服务: http://localhost:5173"
echo "   - PostgreSQL: localhost:5432"
echo "   - Redis: localhost:6379"
echo ""
echo "💡 提示:"
echo "   - 修改后端代码会自动热更新（Air）"
echo "   - 修改前端代码会自动热更新（Vite）"
echo "   - 查看后端日志: docker logs -f nexustok-dev-api"
echo "   - 查看前端日志: docker logs -f nexustok-dev-frontend"
echo "   - 停止服务: docker-compose -f docker-compose.dev.yml down"
echo ""
