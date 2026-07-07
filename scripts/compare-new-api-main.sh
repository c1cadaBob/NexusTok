#!/usr/bin/env bash
set -euo pipefail

# 生成 NexusTok 与 new-api-main 的文件级差异清单。
# 该脚本只读取两个项目目录，不会修改任何源码；输出目录默认为当前仓库的
# tmp/new-api-main-diff，便于在 .gitignore 规则下保存临时报告。

NEXUS_ROOT="${NEXUS_ROOT:-/opt/project/NexusTok}"
NEW_API_ROOT="${NEW_API_ROOT:-/opt/project/new-api-main}"
OUTPUT_DIR="${1:-${NEXUS_ROOT}/tmp/new-api-main-diff}"

if ! command -v rg >/dev/null 2>&1; then
  echo "需要 ripgrep(rg) 才能生成差异清单" >&2
  exit 1
fi

if [[ ! -d "${NEXUS_ROOT}" ]]; then
  echo "NexusTok 目录不存在：${NEXUS_ROOT}" >&2
  exit 1
fi

if [[ ! -d "${NEW_API_ROOT}" ]]; then
  echo "new-api-main 目录不存在：${NEW_API_ROOT}" >&2
  exit 1
fi

mkdir -p "${OUTPUT_DIR}"

list_files() {
  local root="$1"
  rg --files \
    -g '!node_modules' \
    -g '!dist' \
    -g '!build' \
    -g '!tmp' \
    -g '!logs' \
    -g '!*.log' \
    "${root}" |
    sed "s#^${root}/##" |
    sort
}

list_files "${NEXUS_ROOT}" >"${OUTPUT_DIR}/nexustok-files.txt"
list_files "${NEW_API_ROOT}" >"${OUTPUT_DIR}/new-api-main-files.txt"

comm -23 "${OUTPUT_DIR}/nexustok-files.txt" "${OUTPUT_DIR}/new-api-main-files.txt" \
  >"${OUTPUT_DIR}/nexustok-only.txt"

comm -13 "${OUTPUT_DIR}/nexustok-files.txt" "${OUTPUT_DIR}/new-api-main-files.txt" \
  >"${OUTPUT_DIR}/new-api-main-only.txt"

comm -12 "${OUTPUT_DIR}/nexustok-files.txt" "${OUTPUT_DIR}/new-api-main-files.txt" \
  >"${OUTPUT_DIR}/common-files.txt"

while IFS= read -r file; do
  if ! cmp -s "${NEXUS_ROOT}/${file}" "${NEW_API_ROOT}/${file}"; then
    printf '%s\n' "${file}"
  fi
done <"${OUTPUT_DIR}/common-files.txt" >"${OUTPUT_DIR}/common-changed.txt"

{
  echo "# NexusTok / new-api-main 文件差异摘要"
  echo
  echo "- NexusTok 独有文件：$(wc -l <"${OUTPUT_DIR}/nexustok-only.txt")"
  echo "- new-api-main 独有文件：$(wc -l <"${OUTPUT_DIR}/new-api-main-only.txt")"
  echo "- 同路径但内容不同：$(wc -l <"${OUTPUT_DIR}/common-changed.txt")"
  echo
  echo "## 顶层目录分布"
  echo
  echo "### NexusTok 独有"
  awk -F/ '{print $1}' "${OUTPUT_DIR}/nexustok-only.txt" | sort | uniq -c | sort -nr
  echo
  echo "### new-api-main 独有"
  awk -F/ '{print $1}' "${OUTPUT_DIR}/new-api-main-only.txt" | sort | uniq -c | sort -nr
  echo
  echo "### 同路径但内容不同"
  awk -F/ '{print $1}' "${OUTPUT_DIR}/common-changed.txt" | sort | uniq -c | sort -nr
} >"${OUTPUT_DIR}/summary.md"

echo "已生成差异清单：${OUTPUT_DIR}"
