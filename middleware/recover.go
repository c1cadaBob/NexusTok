// Package middleware - recover.go
// 该文件实现了 panic 恢复中间件
// 用于捕获请求处理过程中发生的 panic，防止服务器崩溃
//
// panic 恢复的重要性：
// 1. 防止单个请求的 panic 导致整个服务器崩溃
// 2. 记录 panic 信息，便于问题排查
// 3. 返回友好的错误响应，而不是连接中断
package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/c1cada/NexusTok/common" // 公共工具包
	"github.com/gin-gonic/gin"          // Gin 框架
)

// RelayPanicRecover 中继 panic 恢复中间件
// 捕获请求处理过程中发生的 panic，记录错误日志并返回友好的错误响应
//
// 处理流程：
// 1. 使用 defer + recover 捕获 panic
// 2. 记录 panic 信息和堆栈跟踪到日志
// 3. 返回 HTTP 500 错误响应
// 4. 中止请求处理
//
// 错误响应格式（OpenAI 兼容）：
//
//	{
//	  "error": {
//	    "message": "Panic detected, error: <panic信息>. Please submit a issue here: ...",
//	    "type": "nexustok_panic"
//	  }
//	}
//
// 返回值：
//   - gin.HandlerFunc: Gin 中间件函数
func RelayPanicRecover() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 使用 defer + recover 捕获 panic
		defer func() {
			if err := recover(); err != nil {
				// 记录 panic 信息
				common.SysLog(fmt.Sprintf("panic detected: %v", err))

				// 记录堆栈跟踪
				common.SysLog(fmt.Sprintf("stacktrace from panic: %s", string(debug.Stack())))

				// 返回友好的错误响应
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{
						"message": fmt.Sprintf("Panic detected, error: %v. Please submit a issue here: https://github.com/c1cadaBob/NexusTok", err),
						"type":    "nexustok_panic",
					},
				})

				// 中止请求处理
				c.Abort()
			}
		}()

		// 继续处理请求
		c.Next()
	}
}
