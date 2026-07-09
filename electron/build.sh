#!/bin/bash
# build.sh - NexusTok Electron 桌面应用构建脚本
#
# 功能：
# - 构建默认前端（web/default，React + Rsbuild）
# - 构建经典前端（web/classic，Vite）
# - 构建 Go 后端二进制，并嵌入两套前端 dist
# - 调用 electron-builder 生成当前平台的桌面安装包
#
# 输出目录：electron/dist/

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION_VALUE=""

if [ -f "${REPO_ROOT}/VERSION" ]; then
  VERSION_VALUE="$(cat "${REPO_ROOT}/VERSION")"
fi

echo "Building NexusTok Electron App..."

echo "Step 1: Building default frontend..."
cd "${REPO_ROOT}/web/default"
bun install
DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION="${VERSION_VALUE}" bun run build

echo "Step 2: Building classic frontend..."
cd "${REPO_ROOT}/web/classic"
bun install
VITE_REACT_APP_VERSION="${VERSION_VALUE}" bun run build

echo "Step 3: Building Go backend..."
cd "${REPO_ROOT}"

GO_LDFLAGS="-s -w -X github.com/c1cada/NexusTok/common.Version=${VERSION_VALUE}"

if [[ "${OSTYPE}" == "darwin"* ]]; then
  echo "Building for macOS..."
  CGO_ENABLED=1 go build -ldflags="${GO_LDFLAGS}" -o nexustok
  cd "${SCRIPT_DIR}"
  npm install
  npm run build:mac
elif [[ "${OSTYPE}" == "linux-gnu"* ]]; then
  echo "Building for Linux..."
  CGO_ENABLED=1 go build -ldflags="${GO_LDFLAGS}" -o nexustok
  cd "${SCRIPT_DIR}"
  npm install
  npm run build:linux
elif [[ "${OSTYPE}" == "msys" || "${OSTYPE}" == "cygwin" || "${OSTYPE}" == "win32" ]]; then
  echo "Building for Windows..."
  CGO_ENABLED=1 go build -ldflags="${GO_LDFLAGS}" -o nexustok.exe
  cd "${SCRIPT_DIR}"
  npm install
  npm run build:win
else
  echo "Unknown OS, building for current platform..."
  CGO_ENABLED=1 go build -ldflags="${GO_LDFLAGS}" -o nexustok
  cd "${SCRIPT_DIR}"
  npm install
  npm run build
fi

echo "Build complete! Check electron/dist/ for output."
