// Package router - video-router.go
// 该文件定义了视频生成相关的路由
// 支持多种视频生成提供商：
// - OpenAI 兼容格式
// - Kling（快手 Kling）
// - 即梦（Jimeng，字节跳动）
package router

import (
	"github.com/c1cada/NexusTok/controller" // 控制器
	"github.com/c1cada/NexusTok/middleware"  // 中间件

	"github.com/gin-gonic/gin" // Gin 框架
)

// SetVideoRouter 设置视频生成路由
// 支持多种视频生成 API 格式
//
// 路由：
// - /v1/videos/* - OpenAI 兼容格式
// - /kling/v1/videos/* - Kling 格式
// - /jimeng/* - 即梦格式
//
// 中间件：
// - RouteTag - 路由标签（标记为 relay）
// - TokenAuth - Token 认证
// - Distribute - 负载均衡分发
// - KlingRequestConvert - Kling 请求格式转换
// - JimengRequestConvert - 即梦请求格式转换
func SetVideoRouter(router *gin.Engine) {
	// ========================================
	// 视频代理路由 - /v1/videos
	// 接受会话认证（仪表盘）或 Token 认证（API 客户端）
	// ========================================
	videoProxyRouter := router.Group("/v1")
	videoProxyRouter.Use(middleware.RouteTag("relay"))      // 标记为中继路由
	videoProxyRouter.Use(middleware.TokenOrUserAuth())       // Token 或用户认证
	{
		videoProxyRouter.GET("/videos/:task_id/content", controller.VideoProxy) // 视频内容代理
	}

	// ========================================
	// 视频生成路由 - /v1
	// ========================================
	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute()) // Token 认证和负载均衡
	{
		// 自定义视频生成 API
		videoV1Router.POST("/video/generations", controller.RelayTask)           // 提交视频生成任务
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTaskFetch) // 获取任务状态
		videoV1Router.POST("/videos/:video_id/remix", controller.RelayTask)      // 视频混剪
	}

	// OpenAI 兼容 API 视频路由
	// 参考：https://platform.openai.com/docs/api-reference/videos/create
	{
		videoV1Router.POST("/videos", controller.RelayTask)                      // 创建视频任务
		videoV1Router.GET("/videos/:task_id", controller.RelayTaskFetch)         // 获取任务状态
	}

	// ========================================
	// Kling 视频生成路由 - /kling/v1
	// Kling 是快手旗下的 AI 视频生成服务
	// ========================================
	klingV1Router := router.Group("/kling/v1")
	klingV1Router.Use(middleware.RouteTag("relay"))
	klingV1Router.Use(middleware.KlingRequestConvert(), middleware.TokenAuth(), middleware.Distribute()) // 请求格式转换、Token 认证、负载均衡
	{
		klingV1Router.POST("/videos/text2video", controller.RelayTask)           // 文本生成视频
		klingV1Router.POST("/videos/image2video", controller.RelayTask)          // 图片生成视频
		klingV1Router.GET("/videos/text2video/:task_id", controller.RelayTaskFetch) // 获取文本视频任务状态
		klingV1Router.GET("/videos/image2video/:task_id", controller.RelayTaskFetch) // 获取图片视频任务状态
	}

	// ========================================
	// 即梦官方 API 路由 - /jimeng
	// 即梦是字节跳动旗下的 AI 视频生成服务
	// ========================================
	jimengOfficialGroup := router.Group("jimeng")
	jimengOfficialGroup.Use(middleware.RouteTag("relay"))
	jimengOfficialGroup.Use(middleware.JimengRequestConvert(), middleware.TokenAuth(), middleware.Distribute()) // 请求格式转换、Token 认证、负载均衡
	{
		// 映射到官方 API：
		// /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31
		// /?Action=CVSync2AsyncGetResult&Version=2022-08-31
		jimengOfficialGroup.POST("/", controller.RelayTask) // 提交或查询任务
	}
}
