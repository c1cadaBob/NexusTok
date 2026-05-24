// Package middleware - logger.go
// 该文件实现了日志相关的中间件
// 包括路由标签中间件和请求日志格式化
//
// 日志格式：
// [GIN] 时间 | 标签 | 请求ID | 状态码 | 耗时 | 客户端IP | 方法 路径
//
// 示例：
// [GIN] 2024/01/01 - 12:00:00 | relay | abc123 | 200 | 1.234ms | 192.168.1.1 | POST /v1/chat/completions
package middleware

import (
	"fmt"

	"github.com/c1cada/NexusTok/common" // 公共工具包
	"github.com/gin-gonic/gin"          // Gin 框架
)

// RouteTagKey 路由标签的上下文键名
// 用于在 Gin 上下文中存储和获取路由标签
const RouteTagKey = "route_tag"

// RouteTag 路由标签中间件
// 为请求设置路由标签，用于日志分类和监控
//
// 常用标签：
// - "api": API 接口
// - "relay": 中继接口
// - "web": Web 前端
// - "old_api": 旧版 API（兼容）
//
// 参数：
//   - tag: 路由标签名称
//
// 返回值：
//   - gin.HandlerFunc: Gin 中间件函数
func RouteTag(tag string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 将标签存储到上下文中
		c.Set(RouteTagKey, tag)
		// 继续处理请求
		c.Next()
	}
}

// SetUpLogger 设置请求日志
// 配置 Gin 的日志格式，输出结构化的请求日志
//
// 日志格式说明：
// [GIN] 时间 | 标签 | 请求ID | 状态码 | 耗时 | 客户端IP | 方法 路径
//
// 各字段含义：
// - 时间：请求处理的时间戳
// - 标签：路由标签（api/relay/web 等）
// - 请求ID：唯一标识请求的 ID
// - 状态码：HTTP 响应状态码
// - 耗时：请求处理耗时
// - 客户端IP：发起请求的客户端 IP 地址
// - 方法：HTTP 请求方法（GET/POST 等）
// - 路径：请求路径
//
// 参数：
//   - server: Gin 引擎实例
func SetUpLogger(server *gin.Engine) {
	// 使用自定义日志格式
	server.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// 获取请求 ID
		var requestID string
		if param.Keys != nil {
			requestID, _ = param.Keys[common.RequestIdKey].(string)
		}

		// 获取路由标签，默认为 "web"
		tag, _ := param.Keys[RouteTagKey].(string)
		if tag == "" {
			tag = "web"
		}

		// 格式化日志输出
		return fmt.Sprintf("[GIN] %s | %s | %s | %3d | %13v | %15s | %7s %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"), // 时间
			tag,              // 标签
			requestID,        // 请求 ID
			param.StatusCode, // 状态码
			param.Latency,    // 耗时
			param.ClientIP,   // 客户端 IP
			param.Method,     // 方法
			param.Path,       // 路径
		)
	}))
}
