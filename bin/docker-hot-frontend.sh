#!/bin/sh
# NexusTok 前端热更新构建与发布脚本
#
# 前端先构建到临时目录，构建成功后再原子发布到 web/dist。
# 后端通过发布标记感知前端变化，避免在构建过程中读取不完整的产物。

set -eu

RELOAD_INTERVAL=${HOT_RELOAD_INTERVAL:-2}
PROJECT_DIR=${HOT_FRONTEND_PROJECT_DIR:-/app/web}
STAGE_NAME=${HOT_FRONTEND_STAGE_DIR:-dist.hot}
LOCK_FILE=${HOT_FRONTEND_LOCK_FILE:-/app/tmp/frontend-dist-publish.lock}
HOT_MARKER=.nexustok-hot-dist

cleanup() {
  rm -f "${LOCK_FILE}" 2>/dev/null || true
}

trap cleanup EXIT INT TERM

version_value() {
  if [ -f /app/VERSION ]; then
    cat /app/VERSION
  fi
}

ensure_deps() {
  cd "${PROJECT_DIR}"
  if [ ! -x "node_modules/.bin/rsbuild" ]; then
    echo "[热更新] 正在安装前端依赖..."
    bun install --frozen-lockfile
  else
    echo "[热更新] 前端依赖已就绪"
  fi
}

source_snapshot() {
  {
    [ -f /app/VERSION ] && sha256sum /app/VERSION
    find "${PROJECT_DIR}" \
      -path "${PROJECT_DIR}/node_modules" -prune -o \
      -path "${PROJECT_DIR}/dist" -prune -o \
      -path "${PROJECT_DIR}/${STAGE_NAME}" -prune -o \
      -path "${PROJECT_DIR}/dist.next" -prune -o \
      -path "${PROJECT_DIR}/dist.prev" -prune -o \
      -path "${PROJECT_DIR}/.rsbuild" -prune -o \
      -type f -print \
      | sort \
      | while IFS= read -r file; do
          [ -f "${file}" ] && sha256sum "${file}"
        done
  } | sha256sum | awk '{print $1}'
}

dist_snapshot() {
  dist_dir="$1"

  find "${dist_dir}" \
    -type f ! -name "${HOT_MARKER}" -print \
    | sort \
    | while IFS= read -r file; do
        [ -f "${file}" ] && sha256sum "${file}"
      done \
    | sha256sum | awk '{print $1}'
}

publish_dist() {
  src_dir="${PROJECT_DIR}/${STAGE_NAME}"
  dest_dir="${PROJECT_DIR}/dist"
  next_dir="${PROJECT_DIR}/dist.next"
  prev_dir="${PROJECT_DIR}/dist.prev"

  if [ ! -f "${src_dir}/index.html" ]; then
    echo "[热更新] 前端构建产物缺少 index.html，跳过发布"
    return 1
  fi

  mkdir -p "$(dirname "${LOCK_FILE}")"
  : > "${LOCK_FILE}"

  rm -rf "${next_dir}" "${prev_dir}"
  if ! cp -a "${src_dir}" "${next_dir}"; then
    cleanup
    echo "[热更新] 复制前端构建产物失败"
    return 1
  fi

  {
    echo "mode=hot"
    echo "generated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "dist_sha256=$(dist_snapshot "${next_dir}")"
  } > "${next_dir}/${HOT_MARKER}"

  if [ -d "${dest_dir}" ]; then
    mv "${dest_dir}" "${prev_dir}"
  fi
  mv "${next_dir}" "${dest_dir}"
  rm -rf "${prev_dir}"
  rm -f "${LOCK_FILE}"

  echo "[热更新] 前端产物发布完成"
}

build_frontend() {
  echo "[热更新] 开始构建前端..."
  if ! ensure_deps; then
    echo "[热更新] 前端依赖安装失败"
    return 1
  fi
  rm -rf "${PROJECT_DIR}/${STAGE_NAME}"
  if ! (
    cd "${PROJECT_DIR}"
    NEXUSTOK_HOT_RELOAD=true \
      NEXUSTOK_DIST_ROOT="${STAGE_NAME}" \
      DISABLE_ESLINT_PLUGIN=true \
      VITE_REACT_APP_VERSION="$(version_value)" \
      bun run build
  ); then
    echo "[热更新] 前端构建失败"
    return 1
  fi
  publish_dist
}

needs_publish() {
  [ ! -f "${PROJECT_DIR}/dist/index.html" ] && return 0
  [ ! -f "${PROJECT_DIR}/dist/${HOT_MARKER}" ] && return 0
  return 1
}

ensure_deps

LAST_SNAPSHOT=""

while :; do
  CURRENT_SNAPSHOT=$(source_snapshot)

  if [ "${CURRENT_SNAPSHOT}" != "${LAST_SNAPSHOT}" ] || needs_publish; then
    if build_frontend; then
      LAST_SNAPSHOT=$(source_snapshot)
    else
      echo "[热更新] 前端构建失败，保留当前已发布产物"
    fi
  fi

  sleep "${RELOAD_INTERVAL}"
done
