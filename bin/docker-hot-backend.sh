#!/bin/sh
# NexusTok 后端热更新编译与重启脚本
#
# 监听 Go 源文件、模块文件和前端发布标记。
# 新版本编译失败时保留当前正在运行的后端进程。

set -eu

APP_BIN=${APP_BIN:-/app/tmp/hot/nexustok}
APP_PID=""
LAST_SNAPSHOT=""
RELOAD_INTERVAL=${HOT_RELOAD_INTERVAL:-2}
FRONTEND_LOCK_FILE=${HOT_FRONTEND_LOCK_FILE:-/app/tmp/frontend-dist-publish.lock}
FRONTEND_MARKER=/app/web/dist/.nexustok-hot-dist

cleanup_app() {
  if [ -n "${APP_PID}" ] && kill -0 "${APP_PID}" 2>/dev/null; then
    kill "${APP_PID}" 2>/dev/null || true
    wait "${APP_PID}" 2>/dev/null || true
  fi
  APP_PID=""
}

cleanup() {
  cleanup_app
}

trap cleanup EXIT INT TERM

wait_for_dist() {
  while [ ! -f /app/web/dist/index.html ] || \
    [ ! -f "${FRONTEND_MARKER}" ] || \
    [ -f "${FRONTEND_LOCK_FILE}" ]; do
    echo "[热更新] 等待前端产物发布..."
    sleep 2
  done
}

snapshot() {
  {
    find /app \
      -path /app/.git -prune -o \
      -path /app/.cache -prune -o \
      -path /app/.gocache -prune -o \
      -path /app/.gomodcache -prune -o \
      -path /app/data -prune -o \
      -path /app/logs -prune -o \
      -path /app/tmp -prune -o \
      -path /app/upload -prune -o \
      -path /app/web/node_modules -prune -o \
      -type f \( \
        -name '*.go' -o \
        -name 'go.mod' -o \
        -name 'go.sum' \
      \) -print
    [ -f "${FRONTEND_MARKER}" ] && printf '%s\n' "${FRONTEND_MARKER}"
  } \
    | sort -u \
    | while IFS= read -r file; do
        [ -f "${file}" ] && sha256sum "${file}"
      done \
    | sha256sum \
    | awk '{print $1}'
}

build_and_restart() {
  wait_for_dist
  mkdir -p "$(dirname "${APP_BIN}")" /app/logs /data

  VERSION_VALUE=""
  if [ -f /app/VERSION ]; then
    VERSION_VALUE=$(cat /app/VERSION)
  fi

  echo "[热更新] 正在编译后端..."
  if (
    cd /app
    GOWORK=off go build -buildvcs=false \
      -ldflags "-s -w -X github.com/c1cadaBob/NexusTok/common.Version=${VERSION_VALUE}" \
      -o "${APP_BIN}" .
  ); then
    echo "[热更新] 后端编译成功，正在重启..."
    cleanup_app
    (
      cd /data
      "${APP_BIN}" --log-dir /app/logs
    ) &
    APP_PID=$!
    echo "[热更新] 后端已启动，进程号 ${APP_PID}"
    return 0
  fi

  echo "[热更新] 后端编译失败，保留当前进程"
  return 1
}

wait_for_dist

while :; do
  wait_for_dist
  CURRENT_SNAPSHOT=$(snapshot)

  if [ "${CURRENT_SNAPSHOT}" != "${LAST_SNAPSHOT}" ] || [ -z "${APP_PID}" ]; then
    if build_and_restart; then
      LAST_SNAPSHOT=$(snapshot)
    fi
  fi

  if [ -n "${APP_PID}" ] && ! kill -0 "${APP_PID}" 2>/dev/null; then
    wait "${APP_PID}" 2>/dev/null || true
    APP_PID=""
  fi

  sleep "${RELOAD_INTERVAL}"
done
