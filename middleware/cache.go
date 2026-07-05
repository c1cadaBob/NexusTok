// Package middleware - cache.go
// 该文件实现了缓存控制中间件
// 用于设置 HTTP 响应的 Cache-Control 头，控制浏览器和 CDN 的缓存行为
//
// 缓存策略：
// - 首页（/）：不缓存（no-cache），确保用户总是获取最新的 HTML
// - 其他资源：缓存 1 周（max-age=604800），包括 JS、CSS、图片等静态资源
package middleware

import "github.com/gin-gonic/gin" // Gin 框架

// Cache 缓存控制中间件
// 根据请求路径设置不同的缓存策略
//
// 缓存策略说明：
// 1. 首页（/）：设置为 no-cache
//   - 浏览器每次都会向服务器验证缓存是否过期
//   - 确保用户总是获取最新的 HTML（SPA 入口页面）
//   - 因为 HTML 中引用的 JS/CSS 文件名可能随版本变化
//
// 2. 其他资源：设置为 max-age=604800（1 周）
//   - 浏览器会直接使用缓存，不会向服务器验证
//   - 适用于静态资源（JS、CSS、图片、字体等）
//   - 这些资源的文件名通常包含哈希值，内容变化时文件名也会变化
//
// 3. Cache-Version 头：
//   - 用于强制刷新 CDN 缓存
//   - 当需要清除 CDN 缓存时，修改这个值即可
//
// 返回值：
//   - func(c *gin.Context): Gin 中间件函数
func Cache() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 首页不缓存，确保获取最新的 HTML
		requestPath := c.Request.URL.Path
		if requestPath == "/" {
			c.Header("Cache-Control", "no-cache")
		} else if requestPath == "/logo.png" || requestPath == "/favicon.ico" {
			// Logo 和 favicon 是稳定文件名的品牌资源，不能像带哈希的 JS/CSS 一样长期强缓存；
			// 否则换图后浏览器会继续显示旧图，直到一周缓存自然过期。
			c.Header("Cache-Control", "no-cache")
		} else {
			// 其他资源缓存 1 周
			c.Header("Cache-Control", "max-age=604800") // one week
		}

		// 设置缓存版本号，用于清除 CDN 缓存
		// 修改此值可以强制 CDN 重新获取资源
		c.Header("Cache-Version", "b688f2fb5be447c25e5aa3bd063087a83db32a288bf6a4f35f2d8db310e40b14")

		// 继续处理请求
		c.Next()
	}
}
