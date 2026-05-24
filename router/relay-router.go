// Package router - relay-router.go
// 该文件定义了所有 AI API 中继/代理路由
// 中继路由负责将客户端请求转发到上游 AI 提供商
// 支持多种 AI API 格式：OpenAI、Claude、Gemini、Midjourney 等
package router

import (
	"github.com/c1cada/NexusTok/constant"   // 常量定义
	"github.com/c1cada/NexusTok/controller" // 控制器
	"github.com/c1cada/NexusTok/middleware"  // 中间件
	"github.com/c1cada/NexusTok/relay"      // 中继层
	"github.com/c1cada/NexusTok/types"      // 类型定义

	"github.com/gin-gonic/gin" // Gin Web 框架
)

// SetRelayRouter 设置所有 AI API 中继路由
// 中继路由是系统的核心功能，负责将客户端的 AI API 请求转发到上游提供商
//
// 支持的 API 格式：
// - OpenAI 格式（/v1/chat/completions 等）
// - Claude 格式（/v1/messages）
// - Gemini 格式（/v1beta/models/*）
// - Midjourney 格式（/mj/*）
// - Suno 音乐生成（/suno/*）
//
// 中间件链：
// 1. CORS - 跨域支持
// 2. DecompressRequest - 请求体解压
// 3. BodyStorageCleanup - 清理临时存储
// 4. StatsMiddleware - 统计中间件
// 5. TokenAuth - Token 认证
// 6. ModelRequestRateLimit - 模型请求限流
// 7. Distribute - 负载均衡分发
func SetRelayRouter(router *gin.Engine) {
	// 注册全局中间件
	router.Use(middleware.CORS())                      // CORS 跨域支持
	router.Use(middleware.DecompressRequestMiddleware()) // 请求体解压（支持 gzip、deflate、br）
	router.Use(middleware.BodyStorageCleanup())         // 清理请求体临时存储
	router.Use(middleware.StatsMiddleware())            // 请求统计

	// ========================================
	// 模型列表路由 - /v1/models
	// 兼容 OpenAI、Claude、Gemini 的模型列表接口
	// 根据请求头自动识别 API 格式
	// ========================================
	// 参考：https://platform.openai.com/docs/api-reference/introduction
	modelsRouter := router.Group("/v1/models")
	modelsRouter.Use(middleware.RouteTag("relay")) // 标记为中继路由
	modelsRouter.Use(middleware.TokenAuth())        // Token 认证
	{
		// 获取模型列表
		// 根据请求头自动判断 API 格式：
		// - x-api-key + anthropic-version -> Claude 格式
		// - x-goog-api-key 或 key 参数 -> Gemini 格式
		// - 其他 -> OpenAI 格式
		modelsRouter.GET("", func(c *gin.Context) {
			switch {
			case c.GetHeader("x-api-key") != "" && c.GetHeader("anthropic-version") != "":
				// Claude API 格式
				controller.ListModels(c, constant.ChannelTypeAnthropic)
			case c.GetHeader("x-goog-api-key") != "" || c.Query("key") != "":
				// Gemini API 格式
				controller.RetrieveModel(c, constant.ChannelTypeGemini)
			default:
				// OpenAI API 格式（默认）
				controller.ListModels(c, constant.ChannelTypeOpenAI)
			}
		})

		// 获取单个模型详情
		modelsRouter.GET("/:model", func(c *gin.Context) {
			switch {
			case c.GetHeader("x-api-key") != "" && c.GetHeader("anthropic-version") != "":
				controller.RetrieveModel(c, constant.ChannelTypeAnthropic)
			default:
				controller.RetrieveModel(c, constant.ChannelTypeOpenAI)
			}
		})
	}

	// ========================================
	// Gemini 模型列表路由 - /v1beta/models
	// 原生 Gemini API 格式
	// ========================================
	geminiRouter := router.Group("/v1beta/models")
	geminiRouter.Use(middleware.RouteTag("relay"))
	geminiRouter.Use(middleware.TokenAuth())
	{
		geminiRouter.GET("", func(c *gin.Context) {
			controller.ListModels(c, constant.ChannelTypeGemini)
		})
	}

	// ========================================
	// Gemini 兼容 OpenAI 格式路由 - /v1beta/openai/models
	// 提供 OpenAI 兼容的模型列表接口
	// ========================================
	geminiCompatibleRouter := router.Group("/v1beta/openai/models")
	geminiCompatibleRouter.Use(middleware.RouteTag("relay"))
	geminiCompatibleRouter.Use(middleware.TokenAuth())
	{
		geminiCompatibleRouter.GET("", func(c *gin.Context) {
			controller.ListModels(c, constant.ChannelTypeOpenAI)
		})
	}

	// ========================================
	// Playground 路由 - /pg
	// 用于前端 Playground 功能
	// ========================================
	playgroundRouter := router.Group("/pg")
	playgroundRouter.Use(middleware.RouteTag("relay"))
	playgroundRouter.Use(middleware.SystemPerformanceCheck()) // 系统性能检查
	playgroundRouter.Use(middleware.UserAuth(), middleware.Distribute()) // 用户认证和负载均衡
	{
		playgroundRouter.POST("/chat/completions", controller.Playground) // Playground 聊天补全
	}

	// ========================================
	// OpenAI 兼容 API 路由 - /v1
	// 这是主要的 API 中继路由，支持多种 AI 功能
	// ========================================
	relayV1Router := router.Group("/v1")
	relayV1Router.Use(middleware.RouteTag("relay"))
	relayV1Router.Use(middleware.SystemPerformanceCheck()) // 系统性能检查
	relayV1Router.Use(middleware.TokenAuth())              // Token 认证
	relayV1Router.Use(middleware.ModelRequestRateLimit())   // 模型请求限流
	{
		// ========================================
		// WebSocket 路由（统一到 Relay）
		// ========================================
		wsRouter := relayV1Router.Group("")
		wsRouter.Use(middleware.Distribute())
		wsRouter.GET("/realtime", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIRealtime)
		})
	}
	{
		// ========================================
		// HTTP 路由
		// ========================================
		httpRouter := relayV1Router.Group("")
		httpRouter.Use(middleware.Distribute()) // 负载均衡分发

		// ----------------------------------------
		// Claude 相关路由
		// ----------------------------------------
		httpRouter.POST("/messages", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatClaude) // Claude 消息接口
		})

		// ----------------------------------------
		// 聊天相关路由
		// ----------------------------------------
		httpRouter.POST("/completions", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAI) // 文本补全
		})
		httpRouter.POST("/chat/completions", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAI) // 聊天补全
		})

		// ----------------------------------------
		// Response 相关路由
		// ----------------------------------------
		httpRouter.POST("/responses", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIResponses) // OpenAI Responses API
		})
		httpRouter.POST("/responses/compact", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIResponsesCompaction) // Responses 压缩版
		})

		// ----------------------------------------
		// 图像相关路由
		// ----------------------------------------
		httpRouter.POST("/edits", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIImage) // 图像编辑
		})
		httpRouter.POST("/images/generations", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIImage) // 图像生成
		})
		httpRouter.POST("/images/edits", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIImage) // 图像编辑
		})

		// ----------------------------------------
		// Embedding 相关路由
		// ----------------------------------------
		httpRouter.POST("/embeddings", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatEmbedding) // 文本嵌入
		})

		// ----------------------------------------
		// 音频相关路由
		// ----------------------------------------
		httpRouter.POST("/audio/transcriptions", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIAudio) // 音频转录
		})
		httpRouter.POST("/audio/translations", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIAudio) // 音频翻译
		})
		httpRouter.POST("/audio/speech", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAIAudio) // 语音合成
		})

		// ----------------------------------------
		// Rerank 相关路由
		// ----------------------------------------
		httpRouter.POST("/rerank", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatRerank) // 重排序
		})

		// ----------------------------------------
		// Gemini 中继路由
		// ----------------------------------------
		httpRouter.POST("/engines/:model/embeddings", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatGemini) // Gemini 嵌入
		})
		httpRouter.POST("/models/*path", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatGemini) // Gemini 模型操作
		})

		// ----------------------------------------
		// 其他中继路由
		// ----------------------------------------
		httpRouter.POST("/moderations", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatOpenAI) // 内容审核
		})

		// ----------------------------------------
		// 未实现的路由（返回 501 Not Implemented）
		// ----------------------------------------
		httpRouter.POST("/images/variations", controller.RelayNotImplemented)         // 图像变体
		httpRouter.GET("/files", controller.RelayNotImplemented)                     // 文件列表
		httpRouter.POST("/files", controller.RelayNotImplemented)                    // 上传文件
		httpRouter.DELETE("/files/:id", controller.RelayNotImplemented)              // 删除文件
		httpRouter.GET("/files/:id", controller.RelayNotImplemented)                 // 获取文件
		httpRouter.GET("/files/:id/content", controller.RelayNotImplemented)         // 获取文件内容
		httpRouter.POST("/fine-tunes", controller.RelayNotImplemented)               // 微调
		httpRouter.GET("/fine-tunes", controller.RelayNotImplemented)                // 微调列表
		httpRouter.GET("/fine-tunes/:id", controller.RelayNotImplemented)            // 获取微调
		httpRouter.POST("/fine-tunes/:id/cancel", controller.RelayNotImplemented)    // 取消微调
		httpRouter.GET("/fine-tunes/:id/events", controller.RelayNotImplemented)     // 微调事件
		httpRouter.DELETE("/models/:model", controller.RelayNotImplemented)          // 删除模型
	}

	// ========================================
	// Midjourney 路由 - /mj
	// 支持 Midjourney 图像生成
	// ========================================
	relayMjRouter := router.Group("/mj")
	relayMjRouter.Use(middleware.RouteTag("relay"))
	relayMjRouter.Use(middleware.SystemPerformanceCheck())
	registerMjRouterGroup(relayMjRouter) // 注册 Midjourney 路由组

	// 带模式的 Midjourney 路由 - /:mode/mj
	relayMjModeRouter := router.Group("/:mode/mj")
	relayMjModeRouter.Use(middleware.RouteTag("relay"))
	relayMjModeRouter.Use(middleware.SystemPerformanceCheck())
	registerMjRouterGroup(relayMjModeRouter)

	// ========================================
	// Suno 音乐生成路由 - /suno
	// ========================================
	relaySunoRouter := router.Group("/suno")
	relaySunoRouter.Use(middleware.RouteTag("relay"))
	relaySunoRouter.Use(middleware.SystemPerformanceCheck())
	relaySunoRouter.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		relaySunoRouter.POST("/submit/:action", controller.RelayTask)      // 提交 Suno 任务
		relaySunoRouter.POST("/fetch", controller.RelayTaskFetch)          // 获取任务状态
		relaySunoRouter.GET("/fetch/:id", controller.RelayTaskFetch)       // 获取指定任务状态
	}

	// ========================================
	// Gemini 原生 API 路由 - /v1beta
	// 支持 Gemini 原生 API 格式
	// ========================================
	relayGeminiRouter := router.Group("/v1beta")
	relayGeminiRouter.Use(middleware.RouteTag("relay"))
	relayGeminiRouter.Use(middleware.SystemPerformanceCheck())
	relayGeminiRouter.Use(middleware.TokenAuth())
	relayGeminiRouter.Use(middleware.ModelRequestRateLimit())
	relayGeminiRouter.Use(middleware.Distribute())
	{
		// Gemini API 路径格式: /v1beta/models/{model_name}:{action}
		relayGeminiRouter.POST("/models/*path", func(c *gin.Context) {
			controller.Relay(c, types.RelayFormatGemini)
		})
	}
}

// registerMjRouterGroup 注册 Midjourney 路由组
// Midjourney 是一个 AI 图像生成服务，支持多种操作：
// - 想象（imagine）：根据文本描述生成图像
// - 变化（change）：对生成的图像进行变化
// - 描述（describe）：根据图像生成文本描述
// - 混合（blend）：混合多张图像
// - 编辑（edits）：编辑图像
// - 视频（video）：生成视频
//
// 参数：
//   - relayMjRouter: Midjourney 路由组
func registerMjRouterGroup(relayMjRouter *gin.RouterGroup) {
	// 图片代理路由（无需认证，用于获取生成的图片）
	relayMjRouter.GET("/image/:id", relay.RelayMidjourneyImage)

	// 以下路由需要 Token 认证和负载均衡
	relayMjRouter.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		// 提交任务路由
		relayMjRouter.POST("/submit/action", controller.RelayMidjourney)         // 通用动作
		relayMjRouter.POST("/submit/shorten", controller.RelayMidjourney)        // 缩短提示词
		relayMjRouter.POST("/submit/modal", controller.RelayMidjourney)          // 模态操作
		relayMjRouter.POST("/submit/imagine", controller.RelayMidjourney)        // 想象（文本生成图像）
		relayMjRouter.POST("/submit/change", controller.RelayMidjourney)         // 变化
		relayMjRouter.POST("/submit/simple-change", controller.RelayMidjourney)  // 简单变化
		relayMjRouter.POST("/submit/describe", controller.RelayMidjourney)       // 描述（图像生成文本）
		relayMjRouter.POST("/submit/blend", controller.RelayMidjourney)          // 混合
		relayMjRouter.POST("/submit/edits", controller.RelayMidjourney)          // 编辑
		relayMjRouter.POST("/submit/video", controller.RelayMidjourney)          // 视频

		// 任务查询路由
		relayMjRouter.GET("/task/:id/fetch", controller.RelayMidjourney)         // 获取任务状态
		relayMjRouter.GET("/task/:id/image-seed", controller.RelayMidjourney)    // 获取图像种子
		relayMjRouter.POST("/task/list-by-condition", controller.RelayMidjourney) // 按条件查询任务

		// InsightFace 换脸
		relayMjRouter.POST("/insight-face/swap", controller.RelayMidjourney)     // 换脸操作

		// 上传 Discord 图片
		relayMjRouter.POST("/submit/upload-discord-images", controller.RelayMidjourney)
	}
}
