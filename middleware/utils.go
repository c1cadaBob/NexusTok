// Package middleware - utils.go
// 该文件包含了中间件中使用的辅助函数
// 提供统一的错误响应格式化功能
package middleware

import (
	"fmt"

	"github.com/c1cada/NexusTok/common" // 公共工具包
	"github.com/c1cada/NexusTok/logger"  // 日志
	"github.com/c1cada/NexusTok/types"   // 类型定义
	"github.com/gin-gonic/gin"          // Gin 框架
)

// abortWithOpenAiMessage 返回 OpenAI 格式的错误响应并中止请求
// 用于返回与 OpenAI API 兼容的错误格式
//
// 错误响应格式：
//
//	{
//	  "error": {
//	    "message": "<错误信息> (request id: <请求ID>)",
//	    "type": "nexustok_error",
//	    "code": "<错误代码>"
//	  }
//	}
//
// 参数：
//   - c: Gin 上下文
//   - statusCode: HTTP 状态码
//   - message: 错误信息
//   - code: 错误代码（可选）
func abortWithOpenAiMessage(c *gin.Context, statusCode int, message string, code ...types.ErrorCode) {
	// 获取错误代码
	codeStr := ""
	if len(code) > 0 {
		codeStr = string(code[0])
	}

	// 获取用户 ID（用于日志）
	userId := c.GetInt("id")

	// 返回错误响应
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": common.MessageWithRequestId(message, c.GetString(common.RequestIdKey)),
			"type":    "nexustok_error",
			"code":    codeStr,
		},
	})

	// 中止请求
	c.Abort()

	// 记录错误日志
	logger.LogError(c.Request.Context(), fmt.Sprintf("user %d | %s", userId, message))
}

// abortWithMidjourneyMessage 返回 Midjourney 格式的错误响应并中止请求
// 用于返回与 Midjourney API 兼容的错误格式
//
// 错误响应格式：
//
//	{
//	  "description": "<错误描述>",
//	  "type": "nexustok_error",
//	  "code": <错误代码>
//	}
//
// 参数：
//   - c: Gin 上下文
//   - statusCode: HTTP 状态码
//   - code: Midjourney 错误代码
//   - description: 错误描述
func abortWithMidjourneyMessage(c *gin.Context, statusCode int, code int, description string) {
	// 返回错误响应
	c.JSON(statusCode, gin.H{
		"description": description,
		"type":        "nexustok_error",
		"code":        code,
	})

	// 中止请求
	c.Abort()

	// 记录错误日志
	logger.LogError(c.Request.Context(), description)
}
