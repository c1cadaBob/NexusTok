package middleware

import (
	"bytes"
	"io"
	"net/http"

	"github.com/c1cada/NexusTok/common"
	"github.com/gin-gonic/gin"
)

// AnonymousRequestBodyLimit 限制匿名 POST 接口的请求体大小。
//
// 中间件会先读取最多 maxBytes+1 字节来判断是否超限；未超限时会把同一份
// 请求体重新写回 c.Request.Body，确保后续 Turnstile、Webhook 验签和 controller
// 仍然读取到完整原始内容。该限制只应挂载在匿名入口，不应影响 Relay、大文件上传
// 或其他已经认证且可能有大请求体的接口。
func AnonymousRequestBodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		maxBytes := common.GetAnonymousRequestBodyLimitBytes()
		if maxBytes <= 0 || c.Request.Body == nil {
			c.Next()
			return
		}

		originalBody := c.Request.Body
		limitedBody, err := readAnonymousRequestBody(originalBody, maxBytes)
		_ = originalBody.Close()
		if err != nil {
			if common.IsRequestBodyTooLargeError(err) {
				c.AbortWithStatus(http.StatusRequestEntityTooLarge)
				return
			}
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}

		c.Request.Body = io.NopCloser(bytes.NewReader(limitedBody))
		c.Request.ContentLength = int64(len(limitedBody))
		c.Next()
	}
}

// readAnonymousRequestBody 读取匿名请求体并在超出限制时返回统一错误。
func readAnonymousRequestBody(body io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, common.ErrRequestBodyTooLarge
	}
	return data, nil
}
