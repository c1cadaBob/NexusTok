#!/bin/sh
# docker-hot-backend.sh - Docker 热重载后端启动脚本
#
# 功能：
# - 监控 Go 源代码和前端构建产物的变化
# - 自动重新编译并重启后端服务
# - 支持生产模式下的开发调试
#
# 环境变量：
#   APP_BIN - 编译后的二进制文件路径（默认: /app/tmp/hot/nexustok）
#   HOT_RELOAD_INTERVAL - 文件变化检查间隔秒数（默认: 2）
set -eu

# 应用二进制文件路径
APP_BIN=${APP_BIN:-/app/tmp/hot/nexustok}
# 当前后端进程 PID
APP_PID=""
# 上次文件快照哈希值
LAST_SNAPSHOT=""
# 重载检查间隔（秒）
RELOAD_INTERVAL=${HOT_RELOAD_INTERVAL:-2}

# cleanup_app 清理当前运行的后端进程
cleanup_app() {
  if [ -n "${APP_PID}" ] && kill -0 "${APP_PID}" 2>/dev/null; then
    kill "${APP_PID}" 2>/dev/null || true
    wait "${APP_PID}" 2>/dev/null || true
  fi
  APP_PID=""
}

# cleanup 信号处理函数，退出时清理进程
cleanup() {
  cleanup_app
}

trap cleanup EXIT INT TERM

# wait_for_dist 等待前端构建产物就绪
wait_for_dist() {
  until [ -f /app/web/default/dist/index.html ] && [ -f /app/web/classic/dist/index.html ] && [ -f /app/modules/cpa-manager/dist/index.html ]; do
    echo "[hot] waiting for production frontend dist..."
    sleep 2
  done
}

# snapshot 计算项目源代码和构建产物的哈希快照
# 用于检测文件是否发生变化
snapshot() {
  find /app \
    -path /app/.git -prune -o \
    -path /app/.cache -prune -o \
    -path /app/.gocache -prune -o \
    -path /app/.gomodcache -prune -o \
    -path /app/data -prune -o \
    -path /app/logs -prune -o \
    -path /app/modules/cliproxyapi -prune -o \
    -path /app/modules/cpa-manager/node_modules -prune -o \
    -path /app/modules/cpa-manager/usage-service -prune -o \
    -path /app/tmp -prune -o \
    -path /app/upload -prune -o \
    -path /app/web/default/node_modules -prune -o \
    -path /app/web/classic/node_modules -prune -o \
    -path /app/web/node_modules -prune -o \
    -type f \( \
      -name '*.go' -o \
      -name 'go.mod' -o \
      -name 'go.sum' -o \
      -path '/app/web/default/dist/*' -o \
      -path '/app/web/classic/dist/*' -o \
      -path '/app/modules/cpa-manager/dist/*' \
    \) -print \
    | sort \
    | while IFS= read -r file; do
        [ -f "$file" ] && sha256sum "$file"
      done \
    | sha256sum \
    | awk '{print $1}'
}

# build_and_restart 编译后端并重启服务
# 流程：等待前端就绪 -> 编译 Go 后端 -> 杀死旧进程 -> 启动新进程
build_and_restart() {
  wait_for_dist
  mkdir -p "$(dirname "${APP_BIN}")" /app/logs /data

  VERSION_VALUE=""
  if [ -f /app/VERSION ]; then
    VERSION_VALUE=$(cat /app/VERSION)
  fi

  echo "[hot] building backend in production/release mode..."
  if (cd /app && go build -ldflags "-s -w -X github.com/c1cada/NexusTok/common.Version=${VERSION_VALUE}" -o "${APP_BIN}" .); then
    echo "[hot] build succeeded; restarting backend..."
    cleanup_app
    (cd /data && "${APP_BIN}" --log-dir /app/logs) &
    APP_PID=$!
    echo "[hot] backend started with pid ${APP_PID}"
  else
    echo "[hot] build failed; keeping current backend process if available"
  fi
}

wait_for_dist

while :; do
  CURRENT_SNAPSHOT=$(snapshot)

  if [ "${CURRENT_SNAPSHOT}" != "${LAST_SNAPSHOT}" ] || [ -z "${APP_PID}" ]; then
    build_and_restart
    LAST_SNAPSHOT=$(snapshot)
  fi

  if [ -n "${APP_PID}" ] && ! kill -0 "${APP_PID}" 2>/dev/null; then
    wait "${APP_PID}" 2>/dev/null || true
    APP_PID=""
  fi

  sleep "${RELOAD_INTERVAL}"
done
