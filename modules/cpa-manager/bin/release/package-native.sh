#!/usr/bin/env bash
# package-native.sh - CPA-Manager 原生平台打包脚本
#
# 功能：
# - 为多个平台（Linux/macOS/Windows, amd64/arm64）交叉编译 CPA-Manager
# - 将前端构建产物嵌入 Go 二进制
# - 生成压缩包（.tar.gz 或 .zip）和校验和文件
#
# 环境变量：
#   VERSION - 版本号（默认: dev）
#   OUT_DIR - 输出目录（默认: dist/native）
#   WEB_HTML - 前端 HTML 文件路径（默认: dist/index.html）
#
# 输出：
#   dist/native/cpa-manager_{version}_{os}_{arch}.tar.gz (Linux/macOS)
#   dist/native/cpa-manager_{version}_{os}_{arch}.zip (Windows)
#   dist/native/checksums.txt (SHA256 校验和)
set -euo pipefail

# 项目根目录
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# 版本号
version="${VERSION:-dev}"
# 输出目录
out_dir="${OUT_DIR:-"${repo_root}/dist/native"}"
# 前端 HTML 文件路径
web_html="${WEB_HTML:-"${repo_root}/dist/index.html"}"
# 二进制文件名
binary_name="cpa-manager"

# 检查前端构建产物是否存在
if [ ! -f "${web_html}" ]; then
  echo "missing ${web_html}; run npm run build first" >&2
  exit 1
fi

# 创建临时工作目录
mkdir -p "${repo_root}/bin/tmp/release"
work_dir="$(mktemp -d "${repo_root}/bin/tmp/release/native.XXXXXX")"
trap 'rm -rf "${work_dir}"' EXIT

# 清理并创建输出目录
rm -rf "${out_dir}"
mkdir -p "${out_dir}"

# 复制 Go 源码和前端产物到工作目录
cp -R "${repo_root}/usage-service" "${work_dir}/usage-service"
cp "${web_html}" "${work_dir}/usage-service/internal/httpapi/web/management.html"

# 目标平台列表
targets=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
  "windows amd64"
  "windows arm64"
)

# 遍历所有目标平台进行交叉编译
for target in "${targets[@]}"; do
  read -r goos goarch <<<"${target}"
  package_name="${binary_name}_${version}_${goos}_${goarch}"
  package_dir="${work_dir}/${package_name}"
  exe_name="${binary_name}"

  if [ "${goos}" = "windows" ]; then
    exe_name="${binary_name}.exe"
  fi

  mkdir -p "${package_dir}"
  (
    cd "${work_dir}/usage-service"
    CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" go build -trimpath -ldflags "-s -w" -o "${package_dir}/${exe_name}" ./cmd/cpa-manager
  )

  cp "${repo_root}/README.md" "${package_dir}/README.md"
  cp "${repo_root}/README_CN.md" "${package_dir}/README_CN.md"
  cp "${repo_root}/LICENSE" "${package_dir}/LICENSE"

  if [ "${goos}" = "windows" ]; then
    (
      cd "${work_dir}"
      zip -qr "${out_dir}/${package_name}.zip" "${package_name}"
    )
  else
    (
      cd "${work_dir}"
      tar -czf "${out_dir}/${package_name}.tar.gz" "${package_name}"
    )
  fi
done

# 生成 SHA256 校验和文件
(
    sha256sum ./* > checksums.txt
  else
    shasum -a 256 ./* > checksums.txt
  fi
)
