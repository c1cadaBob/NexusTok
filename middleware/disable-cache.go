// Package middleware - disable-cache.go
// 该文件实现了禁用缓存中间件
// 用于完全禁止浏览器和代理服务器缓存响应
//
// 适用场景：
// - 敏感数据接口（如用户信息、支付信息）
// - 实时数据接口（如状态查询、日志查询）
// - 包含动态内容的页面
package middleware

import "github.com/gin-gonic/gin" // Gin 框架

// DisableCache 禁用缓存中间件
// 设置多个响应头，确保浏览器和代理服务器不会缓存响应
//
// 响应头说明：
// 1. Cache-Control: no-store, no-cache, must-revalidate, private, max-age=0
//    - no-store: 不存储任何响应
//    - no-cache: 每次使用缓存前必须向服务器验证
//    - must-revalidate: 缓存过期后必须向服务器验证
//    - private: 只允许浏览器缓存，不允许代理服务器缓存
//    - max-age=0: 立即过期
//
// 2. Pragma: no-cache
//    - HTTP/1.0 兼容，确保旧版浏览器也不缓存
//
// 3. Expires: 0
//    - 设置过期时间为 0，表示立即过期
//
// 返回值：
//   - gin.HandlerFunc: Gin 中间件函数
func DisableCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 设置缓存控制头
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")

		// HTTP/1.0 兼容
		c.Header("Pragma", "no-cache")

		// 设置过期时间为 0
		c.Header("Expires", "0")

		// 继续处理请求
		c.Next()
	}
}
