// 包 logging - gin_logger.go
// 该文件提供了 Gin 中间件，用于 HTTP 请求日志记录和 panic 恢复。
// 将 Gin Web 框架与 logrus 集成，用于结构化记录 HTTP 请求、响应和错误处理。
package logging

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
	log "github.com/sirupsen/logrus"
)

// aiAPIPrefixes 定义了应该具有请求 ID 跟踪的 AI API 请求路径前缀。
var aiAPIPrefixes = []string{
	"/v1/chat/completions",
	"/v1/completions",
	"/v1/images",
	"/v1/videos",
	"/v1/messages",
	"/v1/responses",
	"/v1beta/models/",
	"/api/provider/",
}

const (
	// skipGinLogKey 是用于标记跳过日志记录的 Gin 上下文键
	skipGinLogKey = "__gin_skip_request_logging__"
	// creditsUsedKey 是用于标记使用积分的 Gin 上下文键
	creditsUsedKey = "__antigravity_credits_used__"
)

// GinLogrusLogger 返回一个 Gin 中间件处理器，使用 logrus 记录 HTTP 请求和响应。
// 捕获请求详细信息，包括方法、路径、状态码、延迟、客户端 IP 和任何错误消息。
// 仅为 AI API 请求添加请求 ID。
//
// 输出格式（AI API）：[2025-12-23 20:14:10] [info ] | a1b2c3d4 | 200 |       23.559s | ...
// 输出格式（其他）：  [2025-12-23 20:14:10] [info ] | -------- | 200 |       23.559s | ...
//
// 返回：
//   - gin.HandlerFunc: 用于请求日志记录的中间件处理器
func GinLogrusLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := util.MaskSensitiveQuery(c.Request.URL.RawQuery)

		// 仅为 AI API 路径生成请求 ID
		var requestID string
		if isAIAPIPath(path) {
			requestID = GenerateRequestID()
			SetGinRequestID(c, requestID)
			ctx := WithRequestID(c.Request.Context(), requestID)
			c.Request = c.Request.WithContext(ctx)
		}

		c.Next()

		if shouldSkipGinRequestLogging(c) {
			return
		}

		if raw != "" {
			path = path + "?" + raw
		}

		latency := time.Since(start)
		if latency > time.Minute {
			latency = latency.Truncate(time.Second)
		} else {
			latency = latency.Truncate(time.Millisecond)
		}

		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		errorMessage := c.Errors.ByType(gin.ErrorTypePrivate).String()

		if requestID == "" {
			requestID = "--------"
		}
		logLine := fmt.Sprintf("%3d | %13v | %15s | %-7s \"%s\"", statusCode, latency, clientIP, method, path)
		if creditsUsed(c) {
			logLine += " [credits]"
		}
		if errorMessage != "" {
			logLine = logLine + " | " + errorMessage
		}

		entry := log.WithField("request_id", requestID)

		switch {
		case statusCode >= http.StatusInternalServerError:
			entry.Error(logLine)
		case statusCode >= http.StatusBadRequest:
			entry.Warn(logLine)
		default:
			entry.Info(logLine)
		}
	}
}

// isAIAPIPath 检查给定路径是否为应具有请求 ID 跟踪的 AI API 端点。
func isAIAPIPath(path string) bool {
	for _, prefix := range aiAPIPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// GinLogrusRecovery 返回一个 Gin 中间件处理器，用于从 panic 中恢复并使用 logrus 记录。
// 当发生 panic 时，捕获 panic 值、堆栈跟踪和请求路径，然后向客户端返回 500 内部服务器错误响应。
//
// 返回：
//   - gin.HandlerFunc: 用于 panic 恢复的中间件处理器
func GinLogrusRecovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		if err, ok := recovered.(error); ok && errors.Is(err, http.ErrAbortHandler) {
			// 让 net/http 处理 ErrAbortHandler，以便连接被中止而不会产生嘈杂的堆栈日志。
			panic(http.ErrAbortHandler)
		}

		log.WithFields(log.Fields{
			"panic": recovered,
			"stack": string(debug.Stack()),
			"path":  c.Request.URL.Path,
		}).Error("recovered from panic")

		c.AbortWithStatus(http.StatusInternalServerError)
	})
}

// SkipGinRequestLogging 标记提供的 Gin 上下文，使 GinLogrusLogger
// 跳过为关联的请求发出日志行。
func SkipGinRequestLogging(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(skipGinLogKey, true)
}

func shouldSkipGinRequestLogging(c *gin.Context) bool {
	if c == nil {
		return false
	}
	val, exists := c.Get(skipGinLogKey)
	if !exists {
		return false
	}
	flag, ok := val.(bool)
	return ok && flag
}

func creditsUsed(c *gin.Context) bool {
	if c == nil {
		return false
	}
	val, exists := c.Get(creditsUsedKey)
	if !exists {
		return false
	}
	flag, ok := val.(bool)
	return ok && flag
}
