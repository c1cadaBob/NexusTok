#!/bin/sh
# docker-hot-frontend.sh - Docker 热更新前端构建发布脚本
#
# 设计目标：
# - 前端先构建到临时目录，构建成功后再发布到 Go embed 使用的 dist 目录。
# - 发布完成后写入标记文件，后端热重载只在标记更新后重新编译。
# - 如果开发者手动执行普通生产构建覆盖了 dist，本脚本会检测标记缺失并自动重建热更新产物。
#
# 环境变量：
#   HOT_RELOAD_INTERVAL - 文件变化检查间隔秒数（默认: 2）
#   HOT_FRONTEND_LOCK_FILE - dist 发布锁文件路径
set -eu

RELOAD_INTERVAL=${HOT_RELOAD_INTERVAL:-2}
LOCK_FILE=${HOT_FRONTEND_LOCK_FILE:-/app/tmp/frontend-dist-publish.lock}
DEFAULT_PROJECT=/app/web/default
CLASSIC_PROJECT=/app/web/classic
DEFAULT_STAGE=dist.hot
CLASSIC_STAGE=dist.hot
HOT_MARKER=.nexustok-hot-dist

cleanup() {
  rm -f "${LOCK_FILE}" 2>/dev/null || true
}

trap cleanup EXIT INT TERM

ensure_deps() {
  project_dir="$1"
  build_bin="$2"

  cd "${project_dir}"
  if [ ! -x "node_modules/.bin/${build_bin}" ]; then
    echo "[hot] installing dependencies for ${project_dir}"
    bun install --frozen-lockfile
  else
    echo "[hot] using existing dependencies for ${project_dir}"
  fi
}

version_value() {
  if [ -f /app/VERSION ]; then
    cat /app/VERSION
  fi
}

source_snapshot() {
  project_dir="$1"

  {
    [ -f /app/VERSION ] && sha256sum /app/VERSION
    find "${project_dir}" \
      -path "${project_dir}/node_modules" -prune -o \
      -path "${project_dir}/dist" -prune -o \
      -path "${project_dir}/dist.hot" -prune -o \
      -path "${project_dir}/dist.next" -prune -o \
      -path "${project_dir}/dist.prev" -prune -o \
      -path "${project_dir}/.rsbuild" -prune -o \
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
  label="$1"
  project_dir="$2"
  stage_name="$3"

  src_dir="${project_dir}/${stage_name}"
  dest_dir="${project_dir}/dist"
  next_dir="${project_dir}/dist.next"
  prev_dir="${project_dir}/dist.prev"

  if [ ! -f "${src_dir}/index.html" ]; then
    echo "[hot] ${label} build output missing index.html; skip publish"
    return 1
  fi

  mkdir -p "$(dirname "${LOCK_FILE}")"
  : > "${LOCK_FILE}"

  rm -rf "${next_dir}" "${prev_dir}"
  cp -a "${src_dir}" "${next_dir}"

  {
    echo "mode=hot"
    echo "label=${label}"
    echo "generated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "dist_sha256=$(dist_snapshot "${next_dir}")"
  } > "${next_dir}/${HOT_MARKER}"

  if [ -d "${dest_dir}" ]; then
    mv "${dest_dir}" "${prev_dir}"
  fi
  mv "${next_dir}" "${dest_dir}"
  rm -rf "${prev_dir}"
  rm -f "${LOCK_FILE}"

  echo "[hot] published ${label} dist"
}

build_default() {
  echo "[hot] building default frontend..."
  rm -rf "${DEFAULT_PROJECT}/${DEFAULT_STAGE}"
  (
    cd "${DEFAULT_PROJECT}"
    NEXUSTOK_HOT_RELOAD=true \
      NEXUSTOK_DIST_ROOT="${DEFAULT_STAGE}" \
      DISABLE_ESLINT_PLUGIN=true \
      VITE_REACT_APP_VERSION="$(version_value)" \
      bun run build
  )
  publish_dist "default" "${DEFAULT_PROJECT}" "${DEFAULT_STAGE}"
}

build_classic() {
  echo "[hot] building classic frontend..."
  rm -rf "${CLASSIC_PROJECT}/${CLASSIC_STAGE}"
  (
    cd "${CLASSIC_PROJECT}"
    NEXUSTOK_HOT_RELOAD=true \
      NEXUSTOK_DIST_ROOT="${CLASSIC_STAGE}" \
      VITE_REACT_APP_VERSION="$(version_value)" \
      bun run build
  )
  publish_dist "classic" "${CLASSIC_PROJECT}" "${CLASSIC_STAGE}"
}

needs_publish() {
  project_dir="$1"

  [ ! -f "${project_dir}/dist/index.html" ] && return 0
  [ ! -f "${project_dir}/dist/${HOT_MARKER}" ] && return 0
  return 1
}

ensure_deps "${DEFAULT_PROJECT}" rsbuild
ensure_deps "${CLASSIC_PROJECT}" vite

LAST_DEFAULT_SNAPSHOT=""
LAST_CLASSIC_SNAPSHOT=""

while :; do
  CURRENT_DEFAULT_SNAPSHOT=$(source_snapshot "${DEFAULT_PROJECT}")
  CURRENT_CLASSIC_SNAPSHOT=$(source_snapshot "${CLASSIC_PROJECT}")

  if [ "${CURRENT_DEFAULT_SNAPSHOT}" != "${LAST_DEFAULT_SNAPSHOT}" ] || needs_publish "${DEFAULT_PROJECT}"; then
    if build_default; then
      LAST_DEFAULT_SNAPSHOT=$(source_snapshot "${DEFAULT_PROJECT}")
    fi
  fi

  if [ "${CURRENT_CLASSIC_SNAPSHOT}" != "${LAST_CLASSIC_SNAPSHOT}" ] || needs_publish "${CLASSIC_PROJECT}"; then
    if build_classic; then
      LAST_CLASSIC_SNAPSHOT=$(source_snapshot "${CLASSIC_PROJECT}")
    fi
  fi

  sleep "${RELOAD_INTERVAL}"
done
