// Package middleware - gzip.go
// 该文件实现了请求体解压缩中间件
//
// 支持的压缩格式：
// - gzip：标准 gzip 压缩（Content-Encoding: gzip）
// - br：Brotli 压缩（Content-Encoding: br）
//
// 中间件功能：
// 1. 自动检测请求体的 Content-Encoding 头
// 2. 根据压缩格式选择对应的解压器
// 3. 解压后替换请求体，供下游处理器使用
// 4. 对解压后的请求体大小进行限制（防止解压炸弹）
// 5. 删除 Content-Encoding 头，避免下游重复处理
package middleware

import (
	"compress/gzip"
	"io"
	"net/http"

	"github.com/c1cada/NexusTok/constant"
	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
)

// readCloser 自定义 io.ReadCloser 包装器
// 将 io.Reader 和关闭函数组合为 io.ReadCloser 接口
// 用于在解压缩场景中正确管理资源的释放顺序
type readCloser struct {
	io.Reader        // 嵌入 Reader 接口，提供 Read 方法
	closeFn func() error // 自定义关闭函数，用于链式关闭多个资源
}

// Close 关闭读取器并释放相关资源
// 调用自定义的 closeFn 函数执行资源清理
func (rc *readCloser) Close() error {
	if rc.closeFn != nil {
		return rc.closeFn()
	}
	return nil
}

// DecompressRequestMiddleware 请求体解压缩中间件工厂函数
// 创建并返回一个 Gin 中间件，用于自动解压请求体
//
// 处理逻辑：
// 1. 跳过 GET 请求和空请求体
// 2. 根据 Content-Encoding 头选择解压方式：
//    - gzip：使用 compress/gzip 解压
//    - br：使用 brotli 解压
//    - 其他：不解压，但仍限制请求体大小
// 3. 使用 http.MaxBytesReader 限制解压后的请求体大小（防止解压炸弹）
// 4. 替换原始请求体并删除 Content-Encoding 头
func DecompressRequestMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil || c.Request.Method == http.MethodGet {
			c.Next()
			return
		}
		maxMB := constant.MaxRequestBodyMB
		if maxMB <= 0 {
			maxMB = 32
		}
		maxBytes := int64(maxMB) << 20

		origBody := c.Request.Body
		wrapMaxBytes := func(body io.ReadCloser) io.ReadCloser {
			return http.MaxBytesReader(c.Writer, body, maxBytes)
		}

		switch c.GetHeader("Content-Encoding") {
		case "gzip":
			gzipReader, err := gzip.NewReader(origBody)
			if err != nil {
				_ = origBody.Close()
				c.AbortWithStatus(http.StatusBadRequest)
				return
			}
			// Replace the request body with the decompressed data, and enforce a max size (post-decompression).
			c.Request.Body = wrapMaxBytes(&readCloser{
				Reader: gzipReader,
				closeFn: func() error {
					_ = gzipReader.Close()
					return origBody.Close()
				},
			})
			c.Request.Header.Del("Content-Encoding")
		case "br":
			reader := brotli.NewReader(origBody)
			c.Request.Body = wrapMaxBytes(&readCloser{
				Reader: reader,
				closeFn: func() error {
					return origBody.Close()
				},
			})
			c.Request.Header.Del("Content-Encoding")
		default:
			// Even for uncompressed bodies, enforce a max size to avoid huge request allocations.
			c.Request.Body = wrapMaxBytes(origBody)
		}

		// Continue processing the request
		c.Next()
	}
}
