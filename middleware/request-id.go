// Package middleware - request-id.go
// 该文件实现了请求 ID 中间件
// 为每个 HTTP 请求生成唯一的请求 ID，用于日志追踪和问题排查
//
// 请求 ID 格式：时间戳 + 构建标识 + 随机字符串
// 示例：20240101120000_abc12345_xyzwabcd
//
// 请求 ID 的用途：
// 1. 日志追踪：将请求 ID 附加到所有日志中，便于追踪请求链路
// 2. 错误排查：当出现错误时，可以通过请求 ID 快速定位相关日志
// 3. 性能分析：结合请求 ID 分析请求的处理时间
// 4. 分布式追踪：在微服务架构中，请求 ID 可以跨服务传递
package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"runtime/debug"

	"github.com/c1cada/NexusTok/common" // 公共工具包
	"github.com/gin-gonic/gin"          // Gin 框架
)

// _bp 构建路径标识符（build path identifier）
// 用于在请求 ID 中标识不同的构建版本
//
// 生成逻辑：
// 1. 尝试从 Go 构建信息中获取模块路径
// 2. 对模块路径进行 SHA256 哈希，取前 4 字节（8 个十六进制字符）
// 3. 如果无法获取构建信息，使用随机字符串
//
// 这个标识符在同一构建版本的所有实例中是相同的
// 有助于区分不同版本的实例生成的请求 ID
var _bp = func() string {
	// 尝试读取 Go 构建信息
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Path != "" {
		// 对模块路径进行 SHA256 哈希
		h := sha256.Sum256([]byte(bi.Main.Path))
		// 取前 4 字节，转换为十六进制字符串（8 个字符）
		return hex.EncodeToString(h[:4])
	}
	// 无法获取构建信息时，使用随机字符串
	return common.GetRandomString(8)
}()

// RequestId 请求 ID 中间件工厂函数
// 创建并返回一个 Gin 中间件函数
//
// 请求 ID 生成规则：
// 时间戳（精确到秒） + 构建标识（8 字符） + 随机字符串（8 字符）
// 例如：20240101120000_abc12345_xyzwabcd
//
// 中间件功能：
// 1. 生成唯一的请求 ID
// 2. 将请求 ID 存储到 Gin 上下文中
// 3. 将请求 ID 附加到请求的 Context 中（可用于下游服务）
// 4. 在响应头中返回请求 ID
//
// 返回值：
//   - func(c *gin.Context): Gin 中间件函数
func RequestId() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 生成请求 ID：时间戳 + 构建标识 + 随机字符串
		id := common.GetTimeString() + _bp + common.GetRandomString(8)

		// 将请求 ID 存储到 Gin 上下文中
		// 后续的处理器和中间件可以通过 c.GetString(common.RequestIdKey) 获取
		c.Set(common.RequestIdKey, id)

		// 将请求 ID 附加到请求的 Context 中
		// 这样下游的服务（如数据库查询、HTTP 客户端）也可以获取请求 ID
		ctx := context.WithValue(c.Request.Context(), common.RequestIdKey, id)
		c.Request = c.Request.WithContext(ctx)

		// 在响应头中返回请求 ID
		// 客户端可以通过响应头获取请求 ID，便于问题反馈
		c.Header(common.RequestIdKey, id)

		// 继续处理请求
		c.Next()
	}
}
