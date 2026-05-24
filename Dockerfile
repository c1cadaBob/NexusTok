# Dockerfile - NexusTok 生产环境构建
#
# 多阶段构建流程：
# 1. builder: 构建默认前端（React 19 + Rsbuild + Bun）
# 2. builder-classic: 构建经典前端（React 18 + Vite）
# 3. builder-cpa-manager: 构建 CPA-Manager 前端（React + Vite）
# 4. builder2: 构建 Go 后端（嵌入前端产物）
# 5. 最终镜像: 基于 Debian slim 的精简运行时
#
# 构建参数：
#   TARGETOS - 目标操作系统（默认: linux）
#   TARGETARCH - 目标架构（默认: amd64）
#
# 暴露端口：3000

# 阶段 1：构建默认前端
FROM oven/bun:1@sha256:0733e50325078969732ebe3b15ce4c4be5082f18c4ac1a0f0ca4839c2e4e42a7 AS builder

WORKDIR /build
COPY web/default/package.json .
COPY web/default/bun.lock .
RUN bun install
COPY ./web/default .
COPY ./VERSION .
RUN DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat VERSION) bun run build

# 阶段 2：构建经典前端
FROM oven/bun:1@sha256:0733e50325078969732ebe3b15ce4c4be5082f18c4ac1a0f0ca4839c2e4e42a7 AS builder-classic

WORKDIR /build
COPY web/classic/package.json .
COPY web/classic/bun.lock .
RUN bun install
COPY ./web/classic .
COPY ./VERSION .
RUN VITE_REACT_APP_VERSION=$(cat VERSION) bun run build

# 阶段 3：构建 CPA-Manager 前端
FROM node:24-alpine AS builder-cpa-manager

WORKDIR /build
COPY modules/cpa-manager/package.json modules/cpa-manager/package-lock.json ./
RUN npm ci
COPY modules/cpa-manager .
RUN VITE_NEXUSTOK_EMBEDDED=true npm run build

# 阶段 4：构建 Go 后端（嵌入所有前端产物）
FROM golang:1.26.1-alpine@sha256:2389ebfa5b7f43eeafbd6be0c3700cc46690ef842ad962f6c5bd6be49ed82039 AS builder2
ENV GO111MODULE=on CGO_ENABLED=0

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}
ENV GOEXPERIMENT=greenteagc

WORKDIR /build

ADD go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=builder /build/dist ./web/default/dist
COPY --from=builder-classic /build/dist ./web/classic/dist
COPY --from=builder-cpa-manager /build/dist ./modules/cpa-manager/dist
RUN go build -ldflags "-s -w -X 'github.com/c1cada/NexusTok/common.Version=$(cat VERSION)'" -o nexustok

# 阶段 5：最终运行时镜像（基于 Debian slim）
FROM debian:bookworm-slim@sha256:f06537653ac770703bc45b4b113475bd402f451e85223f0f2837acbf89ab020a

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata libasan8 wget \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates

COPY --from=builder2 /build/nexustok /
COPY LICENSE NOTICE THIRD-PARTY-LICENSES.md /licenses/
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/nexustok"]
