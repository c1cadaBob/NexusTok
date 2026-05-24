#!/bin/sh
# docker-account-pool-sidecar.sh - 账号池 Sidecar 容器启动脚本
#
# 功能：
# - 初始化 CLIProxyAPI 的数据目录和配置文件
# - 自动生成最小配置（首次启动）
# - 确保 API Key 已配置
# - 启动 CLIProxyAPI 服务
#
# 环境变量：
#   ACCOUNT_POOL_CLI_PROXY_DATA_DIR - 数据目录（默认: /data/account-pool/cliproxyapi）
#   ACCOUNT_POOL_CLI_PROXY_CONFIG - 配置文件路径
#   ACCOUNT_POOL_CLI_PROXY_AUTH_DIR - 认证文件目录
#   ACCOUNT_POOL_CLI_PROXY_LOG_DIR - 日志目录
#   ACCOUNT_POOL_CLI_PROXY_PORT - 服务端口（默认: 8317）
#   ACCOUNT_POOL_CLI_PROXY_RELAY_KEY - 中继 API Key
#   MANAGEMENT_PASSWORD - 管理密码（默认: nexustok-account-pool-local）
set -eu

# 数据目录配置
DATA_DIR=${ACCOUNT_POOL_CLI_PROXY_DATA_DIR:-/data/account-pool/cliproxyapi}
CONFIG_PATH=${ACCOUNT_POOL_CLI_PROXY_CONFIG:-${DATA_DIR}/config.yaml}
AUTH_DIR=${ACCOUNT_POOL_CLI_PROXY_AUTH_DIR:-${DATA_DIR}/auths}
LOG_DIR=${ACCOUNT_POOL_CLI_PROXY_LOG_DIR:-${DATA_DIR}/logs}
PORT=${ACCOUNT_POOL_CLI_PROXY_PORT:-8317}
RELAY_KEY=${ACCOUNT_POOL_CLI_PROXY_RELAY_KEY:-nexustok-account-pool-relay-local}

# 创建必要的目录
mkdir -p "${DATA_DIR}" "${AUTH_DIR}" "${LOG_DIR}" "$(dirname "${CONFIG_PATH}")"

# 首次启动时生成最小配置文件
# 后续由 CPA-Manager 通过管理 API 维护配置
if [ ! -f "${CONFIG_PATH}" ]; then
  cat >"${CONFIG_PATH}" <<EOF
host: ""
port: ${PORT}
remote-management:
  allow-remote: true
  secret-key: ""
  disable-control-panel: true
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
api-keys:
  - "${RELAY_KEY}"
EOF
fi

# 确保配置文件中包含 RELAY_KEY
# 如果 api-keys 列表存在但缺少 RELAY_KEY，则插入
# 如果 api-keys 列表不存在，则追加
if ! grep -Fq "${RELAY_KEY}" "${CONFIG_PATH}"; then
  if grep -Eq '^[[:space:]]*api-keys:[[:space:]]*$' "${CONFIG_PATH}"; then
    tmp_config="${CONFIG_PATH}.tmp"
    awk -v relay_key="${RELAY_KEY}" '
      {
        print
        if (!inserted && $0 ~ /^[[:space:]]*api-keys:[[:space:]]*$/) {
          print "  - \"" relay_key "\""
          inserted = 1
        }
      }
    ' "${CONFIG_PATH}" >"${tmp_config}"
    mv "${tmp_config}" "${CONFIG_PATH}"
  else
    cat >>"${CONFIG_PATH}" <<EOF
api-keys:
  - "${RELAY_KEY}"
EOF
  fi
fi

# 设置管理密码（可通过环境变量覆盖）
export MANAGEMENT_PASSWORD="${MANAGEMENT_PASSWORD:-nexustok-account-pool-local}"

# 按优先级查找并启动 CLIProxyAPI
# 1. 自定义二进制路径
# 2. PATH 中的 CLIProxyAPI
# 3. 固定路径 /CLIProxyAPI/CLIProxyAPI
# 4. 从源码编译运行

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
