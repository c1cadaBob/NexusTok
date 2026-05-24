// logging - requestid.go
// 本文件提供请求 ID 的生成和在 context 中的存取功能。
// 请求 ID 用于在日志中关联同一请求的所有处理步骤，方便问题排查。
// 同时支持标准 context 和 Gin 框架 context 两种存储方式。
package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

// requestIDKey 是用于在标准 context 中存储请求 ID 的键类型。
type requestIDKey struct{}

// ginRequestIDKey 是用于在 Gin context 中存储请求 ID 的键名。
const ginRequestIDKey = "__request_id__"

// GenerateRequestID 生成一个新的 8 字符十六进制随机请求 ID。
// 使用加密安全的随机数生成器确保 ID 的唯一性。
//
// 返回值：
//   - string: 8 字符的十六进制请求 ID
func GenerateRequestID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}

// WithRequestID 将请求 ID 存入标准 context 中。
//
// 参数：
//   - ctx: 父 context
//   - requestID: 要存入的请求 ID
//
// 返回值：
//   - context.Context: 包含请求 ID 的新 context
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// GetRequestID 从标准 context 中获取请求 ID。
//
// 参数：
//   - ctx: 包含请求 ID 的 context
//
// 返回值：
//   - string: 请求 ID，未找到时返回空字符串
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// SetGinRequestID 将请求 ID 存入 Gin context 中。
//
// 参数：
//   - c: Gin context
//   - requestID: 要存入的请求 ID
func SetGinRequestID(c *gin.Context, requestID string) {
	if c != nil {
		c.Set(ginRequestIDKey, requestID)
	}
}

// GetGinRequestID 从 Gin context 中获取请求 ID。
//
// 参数：
//   - c: Gin context
//
// 返回值：
//   - string: 请求 ID，未找到时返回空字符串
func GetGinRequestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if id, exists := c.Get(ginRequestIDKey); exists {
		if s, ok := id.(string); ok {
			return s
		}
	}
	return ""
}
