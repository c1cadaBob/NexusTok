#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

export LOADTEST_BASE_URL="${LOADTEST_BASE_URL:-${BASE_URL:-http://127.0.0.1:3100}}"
export LOADTEST_ROOT_PASSWORD="${LOADTEST_ROOT_PASSWORD:-LoadtestRoot123!}"
export LOADTEST_UPSTREAM_URL="${LOADTEST_UPSTREAM_URL:-http://mock-openai:8080}"
export LOADTEST_STATE_DIR="${LOADTEST_STATE_DIR:-.loadtest}"

go run ./tools/loadtest/bootstrap "$@"
