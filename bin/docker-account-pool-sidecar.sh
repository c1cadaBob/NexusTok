#!/bin/sh
set -eu

DATA_DIR=${ACCOUNT_POOL_CLI_PROXY_DATA_DIR:-/data/account-pool/cliproxyapi}
CONFIG_PATH=${ACCOUNT_POOL_CLI_PROXY_CONFIG:-${DATA_DIR}/config.yaml}
AUTH_DIR=${ACCOUNT_POOL_CLI_PROXY_AUTH_DIR:-${DATA_DIR}/auths}
LOG_DIR=${ACCOUNT_POOL_CLI_PROXY_LOG_DIR:-${DATA_DIR}/logs}
PORT=${ACCOUNT_POOL_CLI_PROXY_PORT:-8317}

mkdir -p "${DATA_DIR}" "${AUTH_DIR}" "${LOG_DIR}" "$(dirname "${CONFIG_PATH}")"

if [ ! -f "${CONFIG_PATH}" ]; then
  # 首次启动只生成 NexusTok sidecar 所需的最小配置，后续由 CPA-Manager 通过管理 API 维护。
  cat >"${CONFIG_PATH}" <<EOF
host: ""
port: ${PORT}
remote-management:
  allow-remote: true
  secret-key: ""
  disable-control-panel: true
  disable-auto-update-panel: true
auth-dir: "${AUTH_DIR}"
debug: false
logging-to-file: true
logs-max-total-size-mb: 256
error-logs-max-files: 20
usage-statistics-enabled: true
redis-usage-queue-retention-seconds: 300
request-retry: 3
max-retry-credentials: 0
max-retry-interval: 30
routing:
  strategy: "round-robin"
  session-affinity: false
ws-auth: true
EOF
fi

export MANAGEMENT_PASSWORD="${MANAGEMENT_PASSWORD:-nexustok-account-pool-local}"

if [ -n "${ACCOUNT_POOL_CLI_PROXY_BINARY:-}" ]; then
  exec "${ACCOUNT_POOL_CLI_PROXY_BINARY}" -config "${CONFIG_PATH}"
fi

if command -v CLIProxyAPI >/dev/null 2>&1; then
  exec CLIProxyAPI -config "${CONFIG_PATH}"
fi

if [ -x /CLIProxyAPI/CLIProxyAPI ]; then
  exec /CLIProxyAPI/CLIProxyAPI -config "${CONFIG_PATH}"
fi

cd /app/modules/cliproxyapi
exec go run ./cmd/server -config "${CONFIG_PATH}"
