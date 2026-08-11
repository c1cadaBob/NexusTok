#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

# 这段检查用于阻止运行时数据被误提交到仓库，或者被打进 Docker 构建上下文。
# 模型仓库是允许随镜像分发的业务数据，其余数据库、日志和密钥都必须来自部署环境。
fail() {
  printf '::error::%s\n' "$*" >&2
  exit 1
}

is_allowed_runtime_like_path() {
  case "$1" in
    modelcatalog/repository/*)
      return 0
      ;;
  esac
  return 1
}

tracked_matches=()
while IFS= read -r path; do
  [ -z "${path}" ] && continue
  if is_allowed_runtime_like_path "${path}"; then
    continue
  fi
  case "${path}" in
    *.db|*.db-shm|*.db-wal|*.db-journal|\
    *.sqlite|*.sqlite-shm|*.sqlite-wal|*.sqlite-journal|\
    *.sqlite3|*.sqlite3-shm|*.sqlite3-wal|*.sqlite3-journal|\
    data/*|logs/*|*/session_secret|session_secret)
      tracked_matches+=("${path}")
      ;;
  esac
done < <(git ls-files)

if [ "${#tracked_matches[@]}" -gt 0 ]; then
  printf '发现不应纳入 git 的运行时数据：\n' >&2
  printf '  %s\n' "${tracked_matches[@]}" >&2
  fail "请把运行时数据库、日志和密钥文件移出仓库。"
fi

context_matches=()
required_ignores=(
  "data"
  "logs"
  "*.db"
  "*.db-journal"
  "*.db-shm"
  "*.db-wal"
  "*.sqlite"
  "*.sqlite-journal"
  "*.sqlite-shm"
  "*.sqlite-wal"
  "*.sqlite3"
  "*.sqlite3-journal"
  "*.sqlite3-shm"
  "*.sqlite3-wal"
  "session_secret"
  "**/session_secret"
)

for pattern in "${required_ignores[@]}"; do
  if ! grep -Fxq "${pattern}" .dockerignore; then
    context_matches+=(".dockerignore missing ${pattern}")
  fi
done

if [ "${#context_matches[@]}" -gt 0 ]; then
  printf 'Docker ignore 规则未能完整保护运行时数据：\n' >&2
  printf '  %s\n' "${context_matches[@]}" >&2
  fail "请先在 Docker 构建上下文中忽略运行时数据。"
fi

printf '运行时数据隔离检查通过。\n'
