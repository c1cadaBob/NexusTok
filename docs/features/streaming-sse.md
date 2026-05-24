# 流式响应与 SSE 处理

流式响应是 Chat Completion 的主要模式，系统通过 StreamScanner 处理 SSE（Server-Sent Events）数据，支持可配置的缓冲区、心跳保活、客户端断连检测和多种结束原因。

## 架构概览

```
上游 API 响应（SSE 流）
  → StreamScanner 逐行扫描
  → 解析 "data:" 前缀数据行
  → 通过回调传递给适配器处理
  → 并发发送心跳 Ping
  → 检测客户端断连
  → 处理 [DONE] 结束标记
  → 返回统一格式给客户端
```

## StreamScanner

### 缓冲区配置

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `InitialScannerBufferSize` | 64KB | 初始缓冲区大小 |
| `DefaultMaxScannerBufferSize` | 64MB | 最大缓冲区大小 |
| `StreamScannerMaxBufferMB` | 64 | 可通过环境变量覆盖的最大缓冲区（MB） |

当单行 SSE 数据超过初始缓冲区时，Scanner 自动扩展至最大限制。图像生成等场景可能产生超大 `data:` 片段（如 4K 图片 base64），需适当调大 `StreamScannerMaxBufferMB`。

### 心跳保活

- 默认间隔：10 秒（`DefaultPingInterval`）
- 在无数据传输时发送心跳，防止代理或负载均衡器超时断开
- 使用 goroutine 池并发发送，避免阻塞数据处理

### 流结束原因

| 原因 | 说明 |
|------|------|
| `StreamEndDone` | 收到 `[DONE]` 标记 |
| `StreamEndEOF` | 连接正常关闭（EOF） |
| `StreamEndTimeout` | 流式超时（默认 300 秒） |
| `StreamEndClientDisconnect` | 客户端断开连接 |
| `StreamEndScanError` | 扫描错误 |
| `StreamEndPingFailure` | 心跳发送失败 |
| `StreamEndPanic` | 处理过程中的 panic |

### 并发模型

使用 `gopool`（字节跳动的 goroutine 池）实现并发安全的数据处理：

- 数据处理和 Ping 发送在独立的 goroutine 中执行
- 避免为每个 SSE 行创建新 goroutine
- 防止 goroutine 泄漏

## WebSocket 中继

对于实时 API（如 OpenAI Realtime API），系统支持 WebSocket 协议中继：

- 双向 WebSocket 连接
- 支持二进制和文本帧
- 自动处理连接升级

## Chat Completions ↔ Responses API 转换

系统支持 Chat Completions API 和 OpenAI Responses API 之间的双向格式转换：

- 上游只支持 Responses API 时，客户端可用 Chat Completions 格式请求
- 上游只支持 Chat Completions 时，客户端可用 Responses API 格式请求
- 自动处理流式和非流式两种模式

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `STREAMING_TIMEOUT` | `300` | 流式超时时间（秒） |
| `STREAM_SCANNER_MAX_BUFFER_MB` | `64` | 流式扫描器单行最大缓冲（MB） |
| `MAX_REQUEST_BODY_MB` | `32` | 请求体最大大小（MB，解压后计） |

## 调试建议

- **空补全问题**：增大 `STREAMING_TIMEOUT` 值
- **大数据截断**：增大 `STREAM_SCANNER_MAX_BUFFER_MB`
- **连接频繁断开**：检查代理/负载均衡器的超时配置，确保大于 `STREAMING_TIMEOUT`
- **内存占用过高**：减小 `MAX_REQUEST_BODY_MB` 限制超大请求

## 关键文件

| 文件 | 说明 |
|------|------|
| `relay/helper/stream_scanner.go` | SSE 流扫描框架 |
| `relay/helper/stream_result.go` | 流结果处理 |
| `relay/websocket.go` | WebSocket 中继 |
| `relay/chat_completions_via_responses.go` | Chat Completions ↔ Responses API 转换 |
| `relay/common/stream_status.go` | 流状态跟踪 |
| `service/openaicompat/` | 格式转换服务 |
