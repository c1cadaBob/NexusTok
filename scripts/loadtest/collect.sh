#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.loadtest.yml}"
BASE_URL="${BASE_URL:-${LOADTEST_BASE_URL:-http://127.0.0.1:3100}}"
STATE_DIR="${LOADTEST_STATE_DIR:-.loadtest}"
TIMESTAMP="${LOADTEST_COLLECT_TIMESTAMP:-$(date -u +%Y%m%dT%H%M%SZ)}"
RESULT_DIR="loadtest-results/${TIMESTAMP}"
COOKIE_FILE="${STATE_DIR}/cookie"

mkdir -p "$RESULT_DIR"

curl_json() {
  local path="$1"
  local output="$2"
  curl -fsS "$BASE_URL$path" -o "$output"
}

curl_json_auth() {
  local path="$1"
  local output="$2"
  if [[ -f "$COOKIE_FILE" ]]; then
    curl -fsS -H "Cookie: $(tr -d '\r\n' < "$COOKIE_FILE")" "$BASE_URL$path" -o "$output" || true
  else
    echo '{"success":false,"message":"missing .loadtest/cookie; run scripts/loadtest/bootstrap.sh first"}' > "$output"
  fi
}

compose_exec() {
  docker compose -f "$COMPOSE_FILE" exec -T "$@"
}

echo "采集 NexusTok 和依赖服务指标到 ${RESULT_DIR}"

curl_json "/api/status" "${RESULT_DIR}/status.json" || true
curl_json_auth "/api/perf-metrics/summary?hours=1" "${RESULT_DIR}/perf-summary.json"
curl_json_auth "/api/system-info/instances" "${RESULT_DIR}/system-instances.json"
curl_json_auth "/api/system-task/list" "${RESULT_DIR}/system-tasks.json"

compose_exec mock-openai wget -q -O - http://127.0.0.1:8080/metrics > "${RESULT_DIR}/mock-metrics.json" || true
docker stats --no-stream > "${RESULT_DIR}/docker-stats.txt" || true

compose_exec postgres psql -U root -d nexustok -c "select datname, usename, state, count(*) from pg_stat_activity group by datname, usename, state order by count(*) desc;" > "${RESULT_DIR}/postgres-connections.txt" || true
compose_exec postgres psql -U root -d nexustok -c "select relname, n_live_tup, n_dead_tup from pg_stat_user_tables order by n_live_tup desc limit 20;" > "${RESULT_DIR}/postgres-table-stats.txt" || true

compose_exec redis redis-cli -a 123456 --no-auth-warning INFO stats > "${RESULT_DIR}/redis-info-stats.txt" || true
compose_exec redis redis-cli -a 123456 --no-auth-warning INFO clients > "${RESULT_DIR}/redis-info-clients.txt" || true
compose_exec redis redis-cli -a 123456 --no-auth-warning INFO memory > "${RESULT_DIR}/redis-info-memory.txt" || true

compose_exec clickhouse clickhouse-client --password 123456 --query "SELECT database, table, active, sum(rows) AS rows, sum(bytes_on_disk) AS bytes_on_disk FROM system.parts WHERE database = 'nexustok_logs' GROUP BY database, table, active ORDER BY table, active FORMAT PrettyCompact" > "${RESULT_DIR}/clickhouse-parts.txt" || true
compose_exec clickhouse clickhouse-client --password 123456 --query "SELECT database, table, command, is_done, latest_failed_part, latest_fail_reason FROM system.mutations WHERE database = 'nexustok_logs' ORDER BY create_time DESC LIMIT 20 FORMAT PrettyCompact" > "${RESULT_DIR}/clickhouse-mutations.txt" || true
compose_exec clickhouse clickhouse-client --password 123456 --query "SELECT count() AS logs_count FROM nexustok_logs.logs FORMAT PrettyCompact" > "${RESULT_DIR}/clickhouse-logs-count.txt" || true

if [[ -f loadtest-results/latest/summary.json ]]; then
  cp loadtest-results/latest/summary.json "${RESULT_DIR}/k6-summary.json"
fi
if [[ -f loadtest-results/latest/summary.md ]]; then
  cp loadtest-results/latest/summary.md "${RESULT_DIR}/k6-summary.md"
fi

cat > "${RESULT_DIR}/report.md" <<REPORT
# NexusTok Load Test Report

- Collected at: ${TIMESTAMP}
- Base URL: ${BASE_URL}
- Compose file: ${COMPOSE_FILE}

## Files

- \`status.json\`: public \`/api/status\`
- \`perf-summary.json\`: \`/api/perf-metrics/summary?hours=1\`
- \`system-instances.json\`: \`/api/system-info/instances\`
- \`system-tasks.json\`: \`/api/system-task/list\`
- \`mock-metrics.json\`: mock upstream counters
- \`docker-stats.txt\`: one-shot Docker CPU/memory/network stats
- \`postgres-connections.txt\`: PostgreSQL active connection state
- \`postgres-table-stats.txt\`: PostgreSQL table tuple summary
- \`redis-info-*.txt\`: Redis stats/clients/memory
- \`clickhouse-*.txt\`: ClickHouse parts, mutations and logs row count
- \`k6-summary.json\` / \`k6-summary.md\`: copied from the latest k6 run when present

## First Checks

1. If k6 error rate is above 1%, open \`mock-metrics.json\` first to confirm whether errors were injected intentionally.
2. If P95/P99 rises while mock latency is stable, compare \`docker-stats.txt\`, \`postgres-connections.txt\` and Redis ops/memory.
3. If ClickHouse logs count is zero after chat traffic, inspect NexusTok logs and \`LOG_SQL_DSN\` initialization before tuning relay hot paths.
4. If only one NexusTok instance has traffic, verify Caddy health and \`system-instances.json\`.

REPORT

echo "报告已生成: ${RESULT_DIR}/report.md"
