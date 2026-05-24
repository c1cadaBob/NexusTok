// Package middleware - cors.go
// 该文件实现了跨域资源共享（CORS）和响应头相关的中间件
//
// CORS（Cross-Origin Resource Sharing）是一种安全机制
// 用于控制浏览器是否允许从不同源（域名、协议、端口）的服务器请求资源
//
// 该文件提供两个中间件：
// 1. CORS - 跨域资源共享中间件
// 2. PoweredBy - 添加版本信息响应头中间件
package middleware

import (
	"github.com/c1cada/NexusTok/common" // 公共工具包
	"github.com/gin-contrib/cors"       // Gin CORS 中间件
	"github.com/gin-gonic/gin"          // Gin 框架
)

// CORS 跨域资源共享中间件
// 允许来自任何源的请求访问 API
//
// 配置说明：
// - AllowAllOrigins: true - 允许所有源访问（生产环境建议限制特定域名）
// - AllowCredentials: true - 允许携带认证信息（Cookie、Authorization 头等）
// - AllowMethods: 允许的 HTTP 方法
// - AllowHeaders: 允许的请求头（"*" 表示所有）
//
// 注意：AllowAllOrigins=true 和 AllowCredentials=true 不能同时使用
// 如果需要携带认证信息，应该设置 AllowOrigins 为特定域名列表
//
// 返回值：
//   - gin.HandlerFunc: Gin 中间件函数
func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true    // 允许所有源
	config.AllowCredentials = true   // 允许携带认证信息
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"} // 允许的 HTTP 方法
	config.AllowHeaders = []string{"*"} // 允许所有请求头
	return cors.New(config)
}

// PoweredBy 添加版本信息响应头中间件
// 在每个响应中添加 X-NexusTok-Version 头，标识当前系统版本
//
// 用途：
// 1. 调试：便于识别当前运行的系统版本
// 2. 监控：可以按版本统计请求
// 3. 客户端适配：客户端可以根据版本调整行为
//
// 返回值：
//   - gin.HandlerFunc: Gin 中间件函数
func PoweredBy() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 添加版本信息响应头
		c.Header("X-NexusTok-Version", common.Version)
		// 继续处理请求
		c.Next()
	}
}
