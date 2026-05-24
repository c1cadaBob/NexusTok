// 包 logging - requestid.go
// 该文件提供了请求 ID 的生成和管理功能。
package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"
)

// requestIDKey 是用于存储/检索请求 ID 的上下文键。
type requestIDKey struct{}

// ginRequestIDKey 是 Gin 上下文中请求 ID 的键。
const ginRequestIDKey = "__request_id__"

// GenerateRequestID 创建新的 8 字符十六进制请求 ID。
func GenerateRequestID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}

// WithRequestID 返回附加了请求 ID 的新上下文。
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// GetRequestID 从上下文中检索请求 ID。
// 未找到时返回空字符串。
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// SetGinRequestID 在 Gin 上下文中存储请求 ID。
func SetGinRequestID(c *gin.Context, requestID string) {
	if c != nil {
		c.Set(ginRequestIDKey, requestID)
	}
}

// GetGinRequestID 从 Gin 上下文中检索请求 ID。
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
