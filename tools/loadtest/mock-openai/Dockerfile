# syntax=docker/dockerfile:1

# 压测 mock upstream 独立构建镜像。该工具只依赖 Go 标准库，避免下载主项目全量依赖。
FROM golang:1.26.1-alpine AS builder

WORKDIR /src
COPY tools/loadtest/mock-openai ./tools/loadtest/mock-openai
RUN GO111MODULE=off CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /mock-openai ./tools/loadtest/mock-openai

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=builder /mock-openai /mock-openai
EXPOSE 8080
ENTRYPOINT ["/mock-openai"]
