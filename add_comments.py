#!/usr/bin/env python3
"""Add Chinese comments to Go source files in the specified directories."""

import os
import re
import sys

# Map of file paths to their package-level comments
PACKAGE_COMMENTS = {
    # access directory
    "access/config_access/provider.go": "// config_access - provider.go\n// 基于配置文件中预定义 API 密钥的访问控制提供者实现。\n// 该文件实现了 SDK 访问提供者接口，通过比对请求中的 API 密钥\n// (来自 Authorization 头、X-Goog-Api-Key 头、X-Api-Key 头或查询参数)\n// 与配置文件中注册的密钥列表来完成身份验证。",
    "access/reconcile.go": "// access - reconcile.go\n// 访问提供者协调（Reconcile）模块。\n// 负责在配置热重载时比较新旧配置的差异，智能地增删改访问提供者实例，\n// 避免不必要的重新创建，同时记录变更明细供日志追踪。",

    # api directory
    "api/buffered_conn.go": "// api - buffered_conn.go\n// 带缓冲读取器的网络连接包装器。\n// 在协议多路复用场景中，需要先 peek 连接的前几个字节来判断协议类型。\n// bufferedConn 在 peek 操作消耗掉部分数据后，仍能通过内部 bufio.Reader\n// 将已读取的数据正确地传递给后续的 HTTP 处理器。",
    "api/mux_listener.go": "// api - mux_listener.go\n// 协议多路复用虚拟监听器。\n// muxListener 实现了 net.Listener 接口，用于在单一 TCP 端口上同时服务\n// HTTP 和 Redis 协议流量。它通过 channel 在连接路由层和 HTTP 服务器之间\n// 传递已分类的连接。",
    "api/protocol_multiplexer.go": "// api - protocol_multiplexer.go\n// 协议多路复用连接路由模块。\n// 在同一个 TCP 端口上同时支持 HTTP/HTTPS 和 Redis RESP 协议。\n// 通过 peek 连接的首字节来判断协议类型，然后将连接分发到对应的处理器。\n// 支持 TLS 握手、ALPN 协议协商以及连接级别的读超时保护。",
    "api/redis_queue_protocol.go": "// api - redis_queue_protocol.go\n// 内嵌的 Redis RESP 协议兼容服务端实现。\n// 允许外部工具通过标准 Redis 客户端协议连接本服务，获取用量统计数据。\n// 支持的命令：AUTH（认证）、SUBSCRIBE（订阅用量频道）、\n// LPOP/RPOP（弹出队列中的用量记录）。\n// 该实现并非完整的 Redis 服务器，而是仅支持与用量监控相关的子集命令。",
    "api/server.go": "// api - server.go\n// CLI Proxy API 的主 HTTP 服务器实现。\n// 包含服务器结构体定义、路由注册、CORS 中间件、认证中间件、\n// 管理面板路由、WebSocket 路由、Keep-Alive 心跳、\n// 以及与各 AI API 处理器（OpenAI、Claude、Gemini）的集成。\n// 支持配置热重载、TLS/HTTPS、HTTP/2 和 Home 模式。",

    # management handlers
    "api/handlers/management/handler.go": "// management - handler.go\n// 管理 API 处理器核心定义。\n// 包含 Handler 结构体、中间件认证逻辑（支持本地密码、环境变量密钥、\n// bcrypt 哈希密钥、IP 封禁防暴力破解）、配置持久化、\n// 以及通用的字段更新辅助方法（布尔、整数、字符串类型）。",
    "api/handlers/management/api_key_usage.go": "// management - api_key_usage.go\n// API 密钥使用统计端点。\n// 提供按 provider 分组、按 base_url|api_key 复合键索引的\n// 请求成功/失败计数和最近请求时间桶数据。",
    "api/handlers/management/api_tools.go": "// management - api_tools.go\n// 管理 API 的通用工具和代理请求功能。\n// 包含：\n// - APICall: 代理 HTTP 请求，支持 $TOKEN$ 变量替换和自动 token 刷新\n// - OAuth token 刷新逻辑（Gemini CLI、Antigravity）\n// - 代理传输层构建（支持凭据级和全局代理 URL）\n// - API Key 配置解析和匹配",
    "api/handlers/management/auth_files.go": "// management - auth_files.go\n// 认证文件管理端点集合。\n// 提供认证文件（auth files）的完整生命周期管理：\n// - 列表查询（从内存或磁盘）\n// - 文件上传（单文件/批量 multipart）\n// - 文件下载和删除（单文件/批量/全量）\n// - 状态切换（启用/禁用）\n// - 字段编辑（prefix、proxy_url、headers、priority、note、account_groups）\n// - OAuth 流程管理（Claude、Gemini CLI、Codex、Antigravity、xAI、Kimi）\n// - GCP 项目发现和 Cloud AI API 启用",
    "api/handlers/management/config_auth_index.go": "// management - config_auth_index.go\n// 配置与认证索引关联模块。\n// 将各 provider 的 API Key 配置项与运行时认证记录的 auth_index 进行关联，\n// 使管理 API 返回的配置列表中能附带对应的认证索引，\n// 便于前端/UI 进行关联展示和操作。",
    "api/handlers/management/config_basic.go": "// management - config_basic.go\n// 基础配置管理端点集合。\n// 提供对服务器基本配置字段的 CRUD 操作，包括：\n// 配置文件读写（YAML）、Debug 开关、日志相关设置、\n// 请求日志、WebSocket 认证、请求重试、模型前缀强制、\n// 路由策略、代理 URL 等。",
    "api/handlers/management/config_lists.go": "// management - config_lists.go\n// 列表型配置管理端点集合。\n// 提供各类 API Key 和兼容性配置的列表管理，包括：\n// - api-keys（API 密钥字符串列表）\n// - gemini-api-key / claude-api-key / codex-api-key / vertex-api-key\n// - openai-compatibility（OpenAI 兼容性配置）\n// - oauth-excluded-models / oauth-model-alias\n// - ampcode 相关配置（upstream URL、model mappings、API keys）\n// 每类均支持 GET/PUT/PATCH/DELETE 操作。",
    "api/handlers/management/logs.go": "// management - logs.go\n// 日志管理端点集合。\n// 提供日志查看、删除和下载功能：\n// - GetLogs: 增量加载日志行，支持时间戳截断和行数限制\n// - DeleteLogs: 清除轮转日志文件并截断活动日志\n// - GetRequestErrorLogs: 列出请求错误日志文件\n// - GetRequestLogByID: 按请求 ID 查找并下载日志\n// - DownloadRequestErrorLog: 下载指定错误日志文件",
    "api/handlers/management/model_definitions.go": "// management - model_definitions.go\n// 静态模型定义查询端点。\n// 根据渠道（channel）名称返回该渠道支持的静态模型元数据列表，\n// 用于前端 UI 展示可用模型信息。",
    "api/handlers/management/oauth_callback.go": "// management - oauth_callback.go\n// OAuth 回调处理端点。\n// 接收来自 OAuth 提供者的回调请求（包含 code、state 或 error），\n// 验证 state 参数的有效性和会话状态，\n// 然后将回调数据写入临时文件以通知等待中的认证 goroutine。",
    "api/handlers/management/oauth_sessions.go": "// management - oauth_sessions.go\n// OAuth 会话状态管理模块。\n// 维护内存中的 OAuth 认证会话状态机（pending -> completed/error），\n// 支持会话注册、状态查询、错误设置、完成确认和批量清理。\n// 包含 state 参数安全验证和 provider 名称标准化逻辑。\n// 会话默认 TTL 为 10 分钟，过期自动清理。",
    "api/handlers/management/quota.go": "// management - quota.go\n// 配额超限策略端点。\n// 管理当 API 配额耗尽时的降级策略开关，包括：\n// - switch-project: 是否自动切换 GCP 项目\n// - switch-preview-model: 是否切换到预览模型",
    "api/handlers/management/usage.go": "// management - usage.go\n// 用量队列查询端点。\n// 从 Redis 用量队列中弹出指定数量的用量记录并返回。\n// 用量记录为原始 JSON 字节数组，直接透传给调用方。",
    "api/handlers/management/vertex_import.go": "// management - vertex_import.go\n// Vertex AI 服务账号凭据导入端点。\n// 接收 Vertex AI 服务账号 JSON 文件，验证并规范化后\n// 创建认证记录保存到存储中，支持指定 GCP 区域（location）。",
}

# Map of struct names to their comments
STRUCT_COMMENTS = {
    "access/config_access/provider.go": {
        "type provider struct": "// provider 是基于配置文件 API 密钥的访问控制提供者。\n// 维护一组预定义的 API 密钥，通过比对请求中的凭据来验证身份。",
    },
    "access/reconcile.go": {},
    "api/buffered_conn.go": {
        "type bufferedConn struct": "// bufferedConn 是带有缓冲读取器的网络连接包装器。\n// 在协议探测阶段，首字节 peek 会消耗掉部分数据，\n// bufferedConn 通过内部的 bufio.Reader 保留这些数据，\n// 确保后续的 HTTP 处理器仍能读取完整的请求内容。",
    },
    "api/mux_listener.go": {
        "type muxListener struct": "// muxListener 是协议多路复用的虚拟监听器。\n// 实现 net.Listener 接口，通过 channel 机制在连接路由层\n// 和 HTTP 服务器之间传递已分类的网络连接。",
    },
    "api/redis_queue_protocol.go": {
        "type redisSubscriptionCommand struct": "// redisSubscriptionCommand 表示 Redis 订阅模式下的客户端命令。\n// 包含解析后的参数列表和可能的读取错误。",
    },
    "api/server.go": {
        "type serverOptionConfig struct": "// serverOptionConfig 存储通过 ServerOption 函数式选项模式\n// 传入的服务器构建参数。",
        "type homeModelEntry struct": "// homeModelEntry 表示 Home 模式下的模型条目信息。",
    },
    "api/handlers/management/handler.go": {
        "type attemptInfo struct": "// attemptInfo 记录单个 IP 的认证失败尝试信息。\n// 包含失败计数、封禁截止时间和最后活动时间。",
        "type Handler struct": "// Handler 是管理 API 的核心处理器。\n// 聚合了配置引用、持久化路径、认证管理器、\n// 失败尝试追踪和各种辅助方法。",
    },
    "api/handlers/management/api_key_usage.go": {
        "type apiKeyUsageEntry struct": "// apiKeyUsageEntry 表示单个 API 密钥的使用统计条目。\n// 包含成功/失败请求计数和最近请求的时间桶快照。",
    },
    "api/handlers/management/api_tools.go": {
        "type apiCallRequest struct": "// apiCallRequest 是代理 API 调用请求体结构。\n// 支持多种 auth_index 字段命名风格（snake_case、camelCase、PascalCase）。",
        "type apiCallResponse struct": "// apiCallResponse 是代理 API 调用响应体结构。\n// 包含上游服务返回的状态码、响应头和响应体。",
        "type apiKeyConfigEntry interface": "// apiKeyConfigEntry 定义了 API Key 配置条目的通用接口。\n// 用于泛型匹配函数，支持 Gemini、Claude、Codex 等不同 provider 的配置。",
    },
    "api/handlers/management/auth_files.go": {
        "type callbackForwarder struct": "// callbackForwarder 管理 OAuth 回调转发服务器的生命周期。\n// 在 Web UI 模式下，将本地回调端口的请求转发到主服务器的管理端口。",
        "type projectSelectionRequiredError struct": "// projectSelectionRequiredError 表示 Gemini CLI 需要用户选择项目的错误。",
    },
    "api/handlers/management/config_auth_index.go": {
        "type geminiKeyWithAuthIndex struct": "// geminiKeyWithAuthIndex 附加 auth-index 字段的 Gemini API Key 配置。",
        "type claudeKeyWithAuthIndex struct": "// claudeKeyWithAuthIndex 附加 auth-index 字段的 Claude API Key 配置。",
        "type codexKeyWithAuthIndex struct": "// codexKeyWithAuthIndex 附加 auth-index 字段的 Codex API Key 配置。",
        "type vertexCompatKeyWithAuthIndex struct": "// vertexCompatKeyWithAuthIndex 附加 auth-index 字段的 Vertex 兼容 Key 配置。",
        "type openAICompatibilityAPIKeyWithAuthIndex struct": "// openAICompatibilityAPIKeyWithAuthIndex 附加 auth-index 字段的 OpenAI 兼容 API Key。",
        "type openAICompatibilityWithAuthIndex struct": "// openAICompatibilityWithAuthIndex 附加 auth-index 字段的 OpenAI 兼容性配置。",
    },
    "api/handlers/management/config_basic.go": {},
    "api/handlers/management/config_lists.go": {},
    "api/handlers/management/logs.go": {
        "type logAccumulator struct": "// logAccumulator 是日志行累积器。\n// 负责按时间戳截断、行数限制来收集日志行，\n// 并追踪最新时间戳和总行数。",
    },
    "api/handlers/management/oauth_callback.go": {
        "type oauthCallbackRequest struct": "// oauthCallbackRequest 是 OAuth 回调请求体结构。\n// 包含 provider、redirect_url、code、state 和 error 字段。",
    },
    "api/handlers/management/oauth_sessions.go": {
        "type oauthSession struct": "// oauthSession 表示一个 OAuth 认证会话的状态。\n// 包含 provider 名称、状态消息、创建时间和过期时间。",
        "type oauthSessionStore struct": "// oauthSessionStore 是线程安全的 OAuth 会话存储。\n// 使用读写锁保护并发访问，支持自动过期清理。",
        "type oauthCallbackFilePayload struct": "// oauthCallbackFilePayload 是写入 OAuth 回调临时文件的数据结构。",
    },
    "api/handlers/management/usage.go": {
        "type usageQueueRecord []byte": "// usageQueueRecord 是用量队列中的原始记录类型。\n// 实现了 json.Marshaler 接口，确保有效的 JSON 直接透传，\n// 无效的 JSON 作为字符串包装输出。",
    },
    "api/handlers/management/vertex_import.go": {},
    "api/middleware/request_logging.go": {},
    "api/middleware/response_writer.go": {
        "type RequestInfo struct": "// RequestInfo 保存传入 HTTP 请求的关键信息，用于日志记录。\n// 包含 URL、HTTP 方法、请求头、请求体、请求 ID 和时间戳。",
        "type ResponseWriterWrapper struct": "// ResponseWriterWrapper 包装标准的 gin.ResponseWriter，\n// 用于拦截和记录响应数据。支持标准响应和流式响应，\n// 确保日志操作不会阻塞客户端响应。",
    },
    "api/modules/modules.go": {
        "type Context struct": "// Context 封装了路由模块注册时所需的依赖。\n// 包含 Gin 引擎、共享的 BaseAPIHandler、当前配置和认证中间件。",
    },
    "api/modules/amp/amp.go": {
        "type AmpModule struct": "// AmpModule 实现了 RouteModuleV2 接口，提供 Amp CLI 集成功能。\n// 功能包括：反向代理到 Amp 控制平面、Provider 路由别名、\n// 自动 gzip 解压缩、模型映射路由。",
    },
    "api/modules/amp/fallback_handlers.go": {
        "type FallbackHandler struct": "// FallbackHandler 包装标准处理器，提供向 ampcode.com 的回退逻辑。\n// 当请求的模型在本地没有可用的 provider 时，\n// 自动转发到 Amp 代理服务。",
    },
}

# Map of function signatures to their comments (for key functions without existing comments)
FUNCTION_COMMENTS = {
    "access/config_access/provider.go": {
        "func Register(": "// Register 向访问管理器注册或注销基于配置的 API Key 提供者。\n// 当配置为 nil 或密钥列表为空时，注销该提供者；\n// 否则创建新的提供者实例并注册。",
        "func newProvider(": "// newProvider 创建一个新的配置 API Key 访问控制提供者实例。\n// 将密钥列表转换为集合（map）以加速查找。",
        "func (p *provider) Identifier(": "// Identifier 返回提供者的标识名称。",
        "func (p *provider) Authenticate(": "// Authenticate 从 HTTP 请求中提取 API 密钥并与已注册的密钥进行比对。\n// 支持的凭据来源：\n// - Authorization: Bearer <token> 头\n// - X-Goog-Api-Key 头（Google API）\n// - X-Api-Key 头（Anthropic API）\n// - URL 查询参数 key\n// - URL 查询参数 auth_token",
        "func extractBearerToken(": "// extractBearerToken 从 Authorization 头中提取 Bearer token。\n// 如果头格式不是 \"Bearer xxx\"，则原样返回整个头值。",
        "func normalizeKeys(": "// normalizeKeys 对 API 密钥列表进行标准化处理。\n// 去除首尾空白、过滤空值、去除重复项。",
    },
    "access/reconcile.go": {
        "func ReconcileProviders(": "// ReconcileProviders 比较新旧配置，生成最小化的提供者变更集。\n// 返回最终的提供者列表以及新增、更新、移除的提供者标识列表。\n// 内联提供者（default）的变更不会出现在变更列表中。",
        "func ApplyAccessProviders(": "// ApplyAccessProviders 将配置中的访问提供者与当前注册的提供者进行协调，\n// 并更新管理器。记录变更摘要日志，返回是否有任何提供者发生变更。",
        "func identifierFromProvider(": "// identifierFromProvider 从提供者实例中提取并标准化标识名称。",
        "func providerInstanceEqual(": "// providerInstanceEqual 比较两个提供者实例是否相同。\n// 优先使用指针比较，否则使用 reflect.DeepEqual。",
    },
    "api/buffered_conn.go": {
        "func (c *bufferedConn) Read(": "// Read 从缓冲读取器中读取数据。\n// 如果内部 reader 已初始化，则从 bufio.Reader 读取（保留 peek 过的数据）；\n// 否则直接从底层连接读取。",
        "func (c *bufferedConn) ConnectionState(": "// ConnectionState 返回 TLS 连接状态。\n// 如果底层连接支持 ConnectionState() 接口，则代理调用。",
    },
    "api/mux_listener.go": {
        "func newMuxListener(": "// newMuxListener 创建一个新的协议多路复用虚拟监听器。\n// buffer 参数指定连接 channel 的缓冲大小，最小为 1。",
        "func (l *muxListener) Put(": "// Put 将一个网络连接放入监听器的 channel 中。\n// 如果监听器已关闭，返回 net.ErrClosed。",
        "func (l *muxListener) Accept(": "// Accept 从 channel 中接收一个连接，实现 net.Listener 接口。\n// 阻塞等待直到有连接到达或监听器关闭。",
        "func (l *muxListener) Close(": "// Close 关闭监听器，使用 sync.Once 确保只关闭一次。",
        "func (l *muxListener) Addr(": "// Addr 返回监听器的网络地址，实现 net.Listener 接口。",
    },
    "api/protocol_multiplexer.go": {
        "func normalizeHTTPServeError(": "// normalizeHTTPServeError 将 HTTP 服务退出时的预期错误（如连接关闭、\n// 服务器关闭）标准化为 nil，避免误报。",
        "func normalizeListenerError(": "// normalizeListenerError 将监听器退出时的预期错误标准化为 nil。",
        "func (s *Server) acceptMuxConnections(": "// acceptMuxConnections 在共享的 TCP 监听器上循环接受新连接，\n// 并为每个连接启动独立的 goroutine 进行协议检测和路由。\n// 这避免了慢速/空闲客户端阻塞 accept 循环。",
        "func (s *Server) routeMuxConnection(": "// routeMuxConnection 执行单个连接的协议检测和路由。\n// 首先尝试 TLS 握手并检查 ALPN 协议，\n// 然后 peek 首字节判断是 Redis RESP 还是 HTTP 协议。\n// 设置读超时防止空闲连接泄漏。",
    },
    "api/redis_queue_protocol.go": {
        "func isRedisRESPPrefix(": "// isRedisRESPPrefix 检查字节是否为 Redis RESP 协议的类型前缀。\n// 支持的前缀：* (数组)、$ (批量字符串)、+ (简单字符串)、- (错误)、: (整数)。",
        "func (s *Server) handleRedisConnection(": "// handleRedisConnection 处理一个 Redis RESP 协议连接的完整生命周期。\n// 包括认证、命令解析和响应。支持的命令：AUTH、SUBSCRIBE、LPOP、RPOP。",
        "func (s *Server) streamRedisUsageSubscription(": "// streamRedisUsageSubscription 处理用量数据的 Redis Pub/Sub 订阅流。\n// 同时监听来自客户端的命令（PING、UNSUBSCRIBE、QUIT）\n// 和来自内部的消息推送。",
        "func readRedisSubscriptionCommands(": "// readRedisSubscriptionCommands 在独立 goroutine 中持续读取客户端命令，\n// 并通过 channel 传递给订阅流处理器。",
        "func handleRedisSubscriptionCommand(": "// handleRedisSubscriptionCommand 处理订阅模式下的客户端命令。\n// 返回 false 表示应关闭连接。",
        "func resolveRemoteIP(": "// resolveRemoteIP 从网络地址中解析出 IP 字符串和是否为本地客户端。",
        "func parseAuthPassword(": "// parseAuthPassword 从 AUTH 命令参数中提取密码。\n// 支持 AUTH <password> 和 AUTH <username> <password> 两种格式。",
        "func parseSubscribeChannel(": "// parseSubscribeChannel 从 SUBSCRIBE 命令参数中提取频道名。",
        "func parsePopCount(": "// parsePopCount 从 LPOP/RPOP 命令参数中提取弹出数量。\n// 无 count 参数时返回 1。",
        "func readRESPArray(": "// readRESPArray 从 Redis RESP 流中读取一个数组类型的数据。",
        "func readRESPString(": "// readRESPString 从 Redis RESP 流中读取一个字符串（支持批量字符串和简单字符串）。",
        "func readRESPBulkString(": "// readRESPBulkString 从 Redis RESP 流中读取一个批量字符串。",
        "func readRESPLine(": "// readRESPLine 从 Redis RESP 流中读取一行（去除 CRLF）。",
        "func writeRedisSimpleString(": "// writeRedisSimpleString 向 RESP 流写入简单字符串响应（+前缀）。",
        "func writeRedisError(": "// writeRedisError 向 RESP 流写入错误响应（-前缀）。",
        "func writeRedisNilBulkString(": "// writeRedisNilBulkString 向 RESP 流写入空批量字符串（$-1）。",
        "func writeRedisBulkString(": "// writeRedisBulkString 向 RESP 流写入批量字符串。",
        "func writeRedisArrayOfBulkStrings(": "// writeRedisArrayOfBulkStrings 向 RESP 流写入批量字符串数组。",
        "func writeRedisInteger(": "// writeRedisInteger 向 RESP 流写入整数响应（:前缀）。",
        "func writeRedisArrayHeader(": "// writeRedisArrayHeader 向 RESP 流写入数组头（*N）。",
        "func writeRedisPubSubSubscribe(": "// writeRedisPubSubSubscribe 向 RESP 流写入 SUBSCRIBE 确认响应。",
        "func writeRedisPubSubUnsubscribe(": "// writeRedisPubSubUnsubscribe 向 RESP 流写入 UNSUBSCRIBE 确认响应。",
        "func writeRedisPubSubMessage(": "// writeRedisPubSubMessage 向 RESP 流写入 Pub/Sub 消息推送。",
        "func writeRedisPubSubPong(": "// writeRedisPubSubPong 向 RESP 流写入 PONG 响应。",
    },
    "api/handlers/management/api_key_usage.go": {
        "func mergeRecentRequestBuckets(": "// mergeRecentRequestBuckets 合并两组最近请求时间桶的数据。\n// 逐桶累加成功和失败计数。",
        "func (h *Handler) GetAPIKeyUsage(": "// GetAPIKeyUsage 返回所有内存中 api_key 认证的最近请求统计。\n// 按 provider 分组，以 \"base_url|api_key\" 为键索引。",
    },
    "api/handlers/management/config_basic.go": {
        "func (h *Handler) GetConfig(": "// GetConfig 返回当前服务器配置的 JSON 表示。",
        "func WriteConfig(": "// WriteConfig 将配置数据写入文件，规范化注释缩进。",
        "func (h *Handler) PutConfigYAML(": "// PutConfigYAML 接收完整的 YAML 配置，验证后写入文件并重新加载。",
        "func (h *Handler) GetConfigYAML(": "// GetConfigYAML 返回原始 config.yaml 文件内容，保留注释和格式。",
        "func normalizeRoutingStrategy(": "// normalizeRoutingStrategy 将路由策略字符串标准化为 canonical 形式。\n// 支持 round-robin 和 fill-first 两种策略。",
    },
    "api/handlers/management/usage.go": {
        "func (h *Handler) GetUsageQueue(": "// GetUsageQueue 从用量队列中弹出指定数量的记录。\n// 通过 count 查询参数控制弹出数量，默认为 1。",
        "func parseUsageQueueCount(": "// parseUsageQueueCount 解析用量队列的弹出数量参数。",
    },
    "api/handlers/management/vertex_import.go": {
        "func (h *Handler) ImportVertexCredential(": "// ImportVertexCredential 处理 Vertex AI 服务账号 JSON 的上传和导入。\n// 验证 JSON 格式、提取 project_id 和 client_email，\n// 创建认证记录并持久化存储。",
        "func valueAsString(": "// valueAsString 将任意类型的值转换为字符串。",
        "func sanitizeVertexFilePart(": "// sanitizeVertexFilePart 清理文件名中的特殊字符，\n// 将 /、\\、: 替换为下划线，空格替换为连字符。",
        "func labelForVertex(": "// labelForVertex 生成 Vertex 凭据的显示标签，格式为 \"projectID (email)\"。",
    },
    "api/modules/modules.go": {
        "func RegisterModule(": "// RegisterModule 注册路由模块，自动适配 V1 或 V2 接口。\n// 优先尝试 V2 接口，回退到 V1 接口。",
    },
    "api/modules/amp/amp.go": {
        "func New(": "// New 使用选项模式创建 Amp 路由模块实例。",
        "func NewLegacy(": "// NewLegacy 使用旧的构造函数签名创建 Amp 模块（已废弃）。",
        "func (m *AmpModule) Name(": "// Name 返回模块标识名称 \"amp-routing\"。",
        "func (m *AmpModule) Register(": "// Register 注册 Amp 路由（实现 RouteModuleV2 接口）。\n// 通过 sync.Once 确保幂等注册。\n// 包括 Provider 别名路由、管理代理路由和上游代理路由。",
        "func (m *AmpModule) OnConfigUpdated(": "// OnConfigUpdated 处理配置热重载。\n// 仅更新实际发生变化的组件，支持增量刷新：\n// 模型映射、上游 API Key、上游 URL、localhost 限制。",
        "func (m *AmpModule) enableUpstreamProxy(": "// enableUpstreamProxy 启用上游代理功能。\n// 创建 MultiSourceSecret 和 MappedSecretSource 密钥源，\n// 并初始化反向代理。",
        "func (m *AmpModule) getProxy(": "// getProxy 线程安全地获取当前反向代理实例。",
        "func (m *AmpModule) setProxy(": "// setProxy 线程安全地更新反向代理实例。",
    },
    "api/modules/amp/fallback_handlers.go": {
        "func NewFallbackHandler(": "// NewFallbackHandler 创建回退处理器，接收延迟代理获取函数。",
        "func NewFallbackHandlerWithMapper(": "// NewFallbackHandlerWithMapper 创建带回退和模型映射功能的处理器。",
        "func (fh *FallbackHandler) WrapHandler(": "// WrapHandler 将标准处理器包装为带回退逻辑的处理器。\n// 处理流程：\n// 1. 读取并清理请求体\n// 2. 提取模型名称\n// 3. 检查模型映射（根据 forceModelMappings 决定优先级）\n// 4. 查找本地 provider\n// 5. 无 provider 时回退到 ampcode.com 代理",
        "func logAmpRouting(": "// logAmpRouting 记录 Amp 请求的路由决策日志。\n// 包含路由类型、模型、provider 和费用信息。",
        "func filterAntropicBetaHeader(": "// filterAntropicBetaHeader 过滤 Anthropic-Beta 头中的特殊订阅功能。\n// 在使用本地 provider 时需要移除需要特殊订阅的功能标记。",
        "func rewriteModelInRequest(": "// rewriteModelInRequest 替换 JSON 请求体中的 model 字段值。",
        "func extractModelFromRequest(": "// extractModelFromRequest 从请求中提取模型名称。\n// 支持 JSON body 中的 model 字段、Gemini URL 路径中的 action 参数、\n// 以及 AMP CLI 格式的 URL 路径。",
    },
    "api/modules/amp/gemini_bridge.go": {
        "func createGeminiBridgeHandler(": "// createGeminiBridgeHandler 创建 AMP CLI Gemini 路径桥接处理器。\n// 将 AMP CLI 的非标准路径格式（/publishers/google/models/{model}:method）\n// 转换为标准 Gemini 格式（设置 :action 参数），\n// 以便标准 Gemini 处理器能正确处理。",
    },
    "api/modules/amp/amp.go": {
        "func (m *AmpModule) Name(": "// Name 返回模块标识名称 \"amp-routing\"。",
    },
}


def find_package_line(content):
    """Find the line number of the 'package' declaration."""
    for i, line in enumerate(content.split('\n')):
        stripped = line.strip()
        if stripped.startswith('package '):
            return i
    return -1


def has_comment_above(content, line_idx):
    """Check if there's already a comment above the given line."""
    lines = content.split('\n')
    if line_idx <= 0:
        return False
    prev = lines[line_idx - 1].strip()
    return prev.startswith('//') or prev.startswith('/*') or prev == '*/' or prev.endswith('*/')


def add_package_comment(content, comment):
    """Add package-level comment above the package declaration."""
    lines = content.split('\n')
    pkg_idx = find_package_line(content)
    if pkg_idx < 0:
        return content

    # Check if there's already a package comment
    if pkg_idx > 0:
        # Look backwards for existing comments
        check_idx = pkg_idx - 1
        while check_idx >= 0 and (lines[check_idx].strip().startswith('//') or lines[check_idx].strip() == ''):
            if lines[check_idx].strip().startswith('//'):
                # Already has a package comment, don't add another
                return content
            check_idx -= 1

    # Find the position to insert (after any blank lines before package)
    insert_idx = pkg_idx
    # Skip blank lines immediately before package
    while insert_idx > 0 and lines[insert_idx - 1].strip() == '':
        insert_idx -= 1

    comment_lines = comment.split('\n')
    for i, cl in enumerate(comment_lines):
        lines.insert(insert_idx + i, cl)

    return '\n'.join(lines)


def add_struct_comments(content, struct_comments):
    """Add comments above struct definitions."""
    lines = content.split('\n')
    result = []
    i = 0
    while i < len(lines):
        stripped = lines[i].strip()
        matched = False
        for pattern, comment in struct_comments.items():
            if stripped.startswith(pattern):
                # Check if there's already a comment above
                if i > 0 and (result[-1].strip().startswith('//') or result[-1].strip().startswith('/*')):
                    # Already has a comment, skip
                    pass
                else:
                    comment_lines = comment.split('\n')
                    for cl in comment_lines:
                        result.append(cl)
                matched = True
                break
        result.append(lines[i])
        i += 1
    return '\n'.join(result)


def add_function_comments(content, func_comments):
    """Add comments above function definitions."""
    lines = content.split('\n')
    result = []
    i = 0
    while i < len(lines):
        stripped = lines[i].strip()
        matched = False
        for pattern, comment in func_comments.items():
            if pattern in stripped and ('func ' in stripped or stripped.startswith('func')):
                # Check if there's already a comment above
                if len(result) > 0 and (result[-1].strip().startswith('//') or result[-1].strip().startswith('/*')):
                    # Already has a comment, skip
                    pass
                else:
                    comment_lines = comment.split('\n')
                    for cl in comment_lines:
                        result.append(cl)
                matched = True
                break
        result.append(lines[i])
        i += 1
    return '\n'.join(result)


def process_file(rel_path, base_dir):
    """Process a single Go file."""
    full_path = os.path.join(base_dir, rel_path)
    if not os.path.exists(full_path):
        print(f"SKIP (not found): {full_path}")
        return

    # Skip test files
    if full_path.endswith('_test.go'):
        print(f"SKIP (test): {full_path}")
        return

    with open(full_path, 'r', encoding='utf-8') as f:
        content = f.read()

    original = content

    # Normalize the rel_path for lookup (strip leading directories to match our keys)
    lookup_key = rel_path

    # Add package comment
    if lookup_key in PACKAGE_COMMENTS:
        content = add_package_comment(content, PACKAGE_COMMENTS[lookup_key])

    # Add struct comments
    if lookup_key in STRUCT_COMMENTS:
        content = add_struct_comments(content, STRUCT_COMMENTS[lookup_key])

    # Add function comments
    if lookup_key in FUNCTION_COMMENTS:
        content = add_function_comments(content, FUNCTION_COMMENTS[lookup_key])

    if content != original:
        with open(full_path, 'w', encoding='utf-8') as f:
            f.write(content)
        print(f"UPDATED: {full_path}")
    else:
        print(f"NO CHANGE: {full_path}")


def main():
    base_dir = "/opt/project/NexusTok/modules/cliproxyapi/internal"

    # Collect all Go files (non-test)
    go_files = []
    for root, dirs, files in os.walk(base_dir):
        for f in files:
            if f.endswith('.go') and not f.endswith('_test.go'):
                full = os.path.join(root, f)
                rel = os.path.relpath(full, base_dir)
                go_files.append(rel)

    go_files.sort()

    for rel in go_files:
        process_file(rel, base_dir)


if __name__ == '__main__':
    main()
