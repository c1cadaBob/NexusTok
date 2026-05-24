// middleware - request_logging.go
// HTTP 请求日志中间件。
// 该模块捕获完整的请求和响应数据（包括头部和正文），通过 RequestLogger 接口记录。
// 支持 zstd 压缩请求体的自动解压。当日志功能未启用时，仅在错误响应时捕获请求正文。
// 管理端点的日志会被跳过以避免泄露敏感信息。
package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
)

// maxErrorOnlyCapturedRequestBodyBytes 是仅错误日志模式下捕获请求体的最大字节数（1 MiB）。
// 超过此大小的请求体在日志功能未启用时不会被捕获，以避免内存峰值。
const maxErrorOnlyCapturedRequestBodyBytes int64 = 1 << 20 // 1 MiB

// RequestLoggingMiddleware 创建一个 Gin 中间件，用于记录 HTTP 请求和响应。
// 该中间件会：
//  1. 检查是否需要跳过该请求（GET 请求、管理端点等）
//  2. 捕获请求信息（URL、方法、头部、正文）
//  3. 包装 ResponseWriter 以捕获响应数据
//  4. 在请求处理完成后完成日志记录
//
// 参数 logger 为 nil 时中间件直接放行。
func RequestLoggingMiddleware(logger logging.RequestLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if logger == nil {
			c.Next()
			return
		}

		if shouldSkipMethodForRequestLogging(c.Request) {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		if !shouldLogRequest(path) {
			c.Next()
			return
		}

		loggerEnabled := logger.IsEnabled()

		// Capture request information
		requestInfo, err := captureRequestInfo(c, shouldCaptureRequestBody(loggerEnabled, c.Request))
		if err != nil {
			// Log error but continue processing
			// In a real implementation, you might want to use a proper logger here
			c.Next()
			return
		}

		// Create response writer wrapper
		wrapper := NewResponseWriterWrapper(c.Writer, logger, requestInfo)
		if !loggerEnabled {
			wrapper.logOnErrorOnly = true
		}
		c.Writer = wrapper

		// Process the request
		c.Next()

		// Finalize logging after request processing
		if err = wrapper.Finalize(c); err != nil {
			// Log error but don't interrupt the response
			// In a real implementation, you might want to use a proper logger here
		}
	}
}

// shouldSkipMethodForRequestLogging 判断是否应跳过该请求的日志记录。
// GET 请求默认跳过，除非是 WebSocket 升级请求（/v1/responses 路径）。
func shouldSkipMethodForRequestLogging(req *http.Request) bool {
	if req == nil {
		return true
	}
	if req.Method != http.MethodGet {
		return false
	}
	return !isResponsesWebsocketUpgrade(req)
}

// isResponsesWebsocketUpgrade 检查请求是否为 /v1/responses 路径的 WebSocket 升级请求。
func isResponsesWebsocketUpgrade(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	if req.URL.Path != "/v1/responses" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(req.Header.Get("Upgrade")), "websocket")
}

// shouldCaptureRequestBody 判断是否应该捕获请求正文。
// 日志功能启用时总是捕获；未启用时仅捕获已知大小且不超过 1 MiB 的非 multipart 请求。
func shouldCaptureRequestBody(loggerEnabled bool, req *http.Request) bool {
	if loggerEnabled {
		return true
	}
	if req == nil || req.Body == nil {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return false
	}
	if req.ContentLength <= 0 {
		return false
	}
	return req.ContentLength <= maxErrorOnlyCapturedRequestBodyBytes
}

// captureRequestInfo 从传入的 HTTP 请求中提取相关信息用于日志记录。
// 捕获的信息包括：URL（敏感查询参数会被掩码）、HTTP 方法、请求头部和请求正文。
// 请求正文读取后会恢复到请求中，确保后续处理器能正常处理。
func captureRequestInfo(c *gin.Context, captureBody bool) (*RequestInfo, error) {
	// Capture URL with sensitive query parameters masked
	maskedQuery := util.MaskSensitiveQuery(c.Request.URL.RawQuery)
	url := c.Request.URL.Path
	if maskedQuery != "" {
		url += "?" + maskedQuery
	}

	// Capture method
	method := c.Request.Method

	// Capture headers
	headers := make(map[string][]string)
	for key, values := range c.Request.Header {
		headers[key] = values
	}

	// Capture request body
	var body []byte
	if captureBody && c.Request.Body != nil {
		// Read the body
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return nil, err
		}

		// Restore the body for the actual request processing
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		body = decodeCapturedRequestBodyForLog(bodyBytes, c.Request.Header.Get("Content-Encoding"))
	}

	return &RequestInfo{
		URL:       url,
		Method:    method,
		Headers:   headers,
		Body:      body,
		RequestID: logging.GetGinRequestID(c),
		Timestamp: time.Now(),
	}, nil
}

// decodeCapturedRequestBodyForLog 解码捕获的请求正文用于日志记录。
// 如果解码失败，返回原始字节而不报错。
func decodeCapturedRequestBodyForLog(raw []byte, encoding string) []byte {
	if len(raw) == 0 {
		return raw
	}

	decoded, errDecode := decodeCapturedRequestBody(raw, encoding)
	if errDecode != nil {
		return raw
	}
	return decoded
}

// decodeCapturedRequestBody 根据 Content-Encoding 头解码请求正文。
// 支持的编码：identity（无压缩）、zstd。
// 多个编码按逆序依次解码（符合 HTTP 编码链规范）。
func decodeCapturedRequestBody(raw []byte, encoding string) ([]byte, error) {
	encoding = strings.TrimSpace(encoding)
	if encoding == "" || strings.EqualFold(encoding, "identity") {
		return raw, nil
	}

	parts := strings.Split(encoding, ",")
	body := raw
	for i := len(parts) - 1; i >= 0; i-- {
		enc := strings.ToLower(strings.TrimSpace(parts[i]))
		switch enc {
		case "", "identity":
			continue
		case "zstd":
			decoded, errDecode := decodeCapturedZstdRequestBody(body)
			if errDecode != nil {
				return nil, errDecode
			}
			body = decoded
		default:
			return nil, fmt.Errorf("unsupported request content encoding: %s", enc)
		}
	}
	return body, nil
}

// decodeCapturedZstdRequestBody 使用 zstd 算法解码请求正文。
func decodeCapturedZstdRequestBody(raw []byte) ([]byte, error) {
	decoder, errNewReader := zstd.NewReader(bytes.NewReader(raw))
	if errNewReader != nil {
		return nil, fmt.Errorf("failed to create zstd request decoder: %w", errNewReader)
	}
	defer decoder.Close()

	decoded, errRead := io.ReadAll(decoder)
	if errRead != nil {
		return nil, fmt.Errorf("failed to decode zstd request body: %w", errRead)
	}
	return decoded, nil
}

// shouldLogRequest 判断指定路径的请求是否应该被记录日志。
// 管理端点（/v0/management、/management）不记录以避免泄露敏感信息。
// /api 路径下仅记录 /api/provider 相关的请求。
// 其他所有路由都会被记录。
func shouldLogRequest(path string) bool {
	if strings.HasPrefix(path, "/v0/management") || strings.HasPrefix(path, "/management") {
		return false
	}

	if strings.HasPrefix(path, "/api") {
		return strings.HasPrefix(path, "/api/provider")
	}

	return true
}
