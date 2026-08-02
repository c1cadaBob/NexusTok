#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

SCENARIO="${1:-${SCENARIO:-mixed}}"
BASE_URL="${BASE_URL:-${LOADTEST_BASE_URL:-http://127.0.0.1:3100}}"
MODEL="${MODEL:-$(tr -d '\r\n' < .loadtest/model 2>/dev/null || printf 'gpt-loadtest')}"
VUS="${VUS:-20}"
DURATION="${DURATION:-2m}"
RAMPING="${RAMPING:-false}"

if [[ -z "${API_TOKEN:-}" ]]; then
  if [[ ! -f .loadtest/token ]]; then
    echo "未找到 .loadtest/token，请先执行 scripts/loadtest/bootstrap.sh" >&2
    exit 1
  fi
  API_TOKEN="$(tr -d '\r\n' < .loadtest/token)"
fi

TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
HOST_RESULTS_DIR="loadtest-results/${TIMESTAMP}"
mkdir -p "$HOST_RESULTS_DIR"
chmod 0777 "$HOST_RESULTS_DIR"
ln -sfn "$TIMESTAMP" loadtest-results/latest

echo "运行 k6 场景: ${SCENARIO}"
echo "结果目录: ${HOST_RESULTS_DIR}"

docker compose -f docker-compose.loadtest.yml run --rm \
  -e BASE_URL="$BASE_URL" \
  -e API_TOKEN="$API_TOKEN" \
  -e MODEL="$MODEL" \
  -e SCENARIO="$SCENARIO" \
  -e VUS="$VUS" \
  -e DURATION="$DURATION" \
  -e RAMPING="$RAMPING" \
  -e RESULTS_DIR="/results/${TIMESTAMP}" \
  k6 run /scripts/k6/chat.js
