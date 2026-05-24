#!/bin/bash
# build.sh - NexusTok Electron 桌面应用构建脚本
#
# 功能：
# - 构建前端（React + Rsbuild）
# - 构建 Go 后端（根据操作系统选择构建目标）
# - 构建 Electron 应用（macOS/Linux/Windows）
#
# 输出目录：electron/dist/

set -e

echo "Building NexusTok Electron App..."

# 步骤 1：构建前端
echo "Step 1: Building frontend..."
cd ../web
DISABLE_ESLINT_PLUGIN='true' bun run build
cd ../electron

# 步骤 2：根据操作系统构建 Go 后端和 Electron 应用
echo "Step 2: Building Go backend..."
cd ..

if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "Building for macOS..."
    CGO_ENABLED=1 go build -ldflags="-s -w" -o nexustok
    cd electron
    npm install
    npm run build:mac
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "Building for Linux..."
    CGO_ENABLED=1 go build -ldflags="-s -w" -o nexustok
    cd electron
    npm install
    npm run build:linux
elif [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" || "$OSTYPE" == "win32" ]]; then
    echo "Building for Windows..."
    CGO_ENABLED=1 go build -ldflags="-s -w" -o nexustok.exe
    cd electron
    npm install
    npm run build:win
else
    echo "Unknown OS, building for current platform..."
    CGO_ENABLED=1 go build -ldflags="-s -w" -o nexustok
    cd electron
    npm install
    npm run build
fi

echo "Build complete! Check electron/dist/ for output."