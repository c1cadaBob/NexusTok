// Package router - dashboard.go
// 该文件定义了仪表盘路由
// 仪表盘路由提供 OpenAI 兼容的计费和使用量查询接口
// 这些接口主要用于与 OpenAI SDK 兼容
package router

import (
	"github.com/c1cada/NexusTok/controller" // 控制器
	"github.com/c1cada/NexusTok/middleware"  // 中间件

	"github.com/gin-contrib/gzip" // gzip 压缩
	"github.com/gin-gonic/gin"    // Gin 框架
)

// SetDashboardRouter 设置仪表盘路由
// 提供 OpenAI 兼容的计费和使用量查询接口
//
// 路由：
// - /dashboard/billing/subscription - 获取订阅信息
// - /dashboard/billing/usage - 获取使用量
// - /v1/dashboard/billing/subscription - 获取订阅信息（v1 前缀兼容）
// - /v1/dashboard/billing/usage - 获取使用量（v1 前缀兼容）
//
// 中间件：
// - RouteTag - 路由标签（标记为 old_api）
// - gzip - 压缩响应
// - GlobalAPIRateLimit - 全局 API 限流
// - CORS - 跨域支持
// - TokenAuth - Token 认证
func SetDashboardRouter(router *gin.Engine) {
	// 创建仪表盘路由组（根路径）
	apiRouter := router.Group("/")

	// 注册中间件
	apiRouter.Use(middleware.RouteTag("old_api"))                    // 标记为旧 API
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))               // gzip 压缩
	apiRouter.Use(middleware.GlobalAPIRateLimit())                   // 全局 API 限流
	apiRouter.Use(middleware.CORS())                                 // CORS 跨域支持
	apiRouter.Use(middleware.TokenAuth())                            // Token 认证
	{
		// 订阅信息查询（OpenAI 兼容）
		apiRouter.GET("/dashboard/billing/subscription", controller.GetSubscription)
		apiRouter.GET("/v1/dashboard/billing/subscription", controller.GetSubscription)

		// 使用量查询（OpenAI 兼容）
		apiRouter.GET("/dashboard/billing/usage", controller.GetUsage)
		apiRouter.GET("/v1/dashboard/billing/usage", controller.GetUsage)
	}
}
