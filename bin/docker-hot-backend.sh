#!/bin/sh
set -eu

APP_BIN=${APP_BIN:-/app/tmp/hot/nexustok}
APP_PID=""
LAST_SNAPSHOT=""
RELOAD_INTERVAL=${HOT_RELOAD_INTERVAL:-2}

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
  until [ -f /app/web/default/dist/index.html ] && [ -f /app/web/classic/dist/index.html ]; do
    echo "[hot] waiting for production frontend dist..."
    sleep 2
  done
}

snapshot() {
  find /app \
    -path /app/.git -prune -o \
    -path /app/.cache -prune -o \
    -path /app/.gocache -prune -o \
    -path /app/.gomodcache -prune -o \
    -path /app/data -prune -o \
    -path /app/logs -prune -o \
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
      -path '/app/web/classic/dist/*' \
    \) -print \
    | sort \
    | while IFS= read -r file; do
        [ -f "$file" ] && sha256sum "$file"
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
