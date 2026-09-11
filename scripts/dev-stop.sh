#!/bin/bash

# NexusTok 开发环境停止脚本

set -e

echo "🛑 停止 NexusTok 开发环境..."
echo ""

# 进入项目根目录
cd "$(dirname "$0")/.."

# 询问是否删除数据卷
echo "是否删除数据卷（包括数据库数据）？"
echo "  1) 仅停止服务（保留数据）"
echo "  2) 停止服务并删除数据卷"
echo ""
read -p "请选择 [1/2，默认为 1]: " choice
choice=${choice:-1}

echo ""

if [ "$choice" = "2" ]; then
    echo "📦 停止服务并删除数据卷..."
    docker-compose -f docker-compose.dev.yml down -v
    echo ""
    echo "✅ 服务已停止，数据卷已删除"
else
    echo "📦 停止服务（保留数据卷）..."
    docker-compose -f docker-compose.dev.yml down
    echo ""
    echo "✅ 服务已停止，数据卷已保留"
    echo ""
    echo "💡 提示: 下次启动时数据仍然存在"
fi

echo ""
echo "📊 剩余容器:"
docker ps -a | grep nexustok-dev || echo "   无 NexusTok 开发容器"
echo ""
echo "📦 剩余数据卷:"
docker volume ls | grep nexustok || echo "   无 NexusTok 数据卷"
echo ""
