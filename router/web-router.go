// Package router - web-router.go
// 该文件定义了 Web 前端静态文件路由
// 负责将前端构建产物（React 应用）提供给浏览器
// 支持多主题切换和 SPA（单页应用）路由回退
package router

import (
	"embed"    // Go 嵌入文件系统
	"net/http" // HTTP 状态码
	"strings"  // 字符串操作

	"github.com/c1cada/NexusTok/common"     // 公共工具包
	"github.com/c1cada/NexusTok/controller" // 控制器
	"github.com/c1cada/NexusTok/middleware" // 中间件

	"github.com/gin-contrib/gzip"   // gzip 压缩
	"github.com/gin-contrib/static" // 静态文件服务
	"github.com/gin-gonic/gin"      // Gin 框架
)

// ThemeAssets 前端主题资源结构体
// 包含所有前端主题的嵌入文件系统和入口页面
// 用于支持多主题切换功能
type ThemeAssets struct {
	DefaultBuildFS   embed.FS // 默认主题的完整前端资源文件系统
	DefaultIndexPage []byte   // 默认主题的入口 HTML 页面
	ClassicBuildFS   embed.FS // 经典主题的完整前端资源文件系统
	ClassicIndexPage []byte   // 经典主题的入口 HTML 页面
}

// SetWebRouter 设置 Web 前端路由
// 将前端构建产物作为静态文件提供，并处理 SPA 路由回退
//
// 功能：
// 1. gzip 压缩 - 减少传输大小
// 2. 全局限流 - 防止滥用
// 3. 缓存中间件 - 优化性能
// 4. 静态文件服务 - 提供前端资源
// 5. SPA 路由回退 - 未匹配的路由返回 index.html
//
// 参数：
//   - router: Gin 引擎实例
//   - assets: 前端主题资源
func SetWebRouter(router *gin.Engine, assets ThemeAssets) {
	// 创建嵌入文件系统实例
	// 将 Go embed.FS 转换为 Gin 可用的静态文件系统
	defaultFS := common.EmbedFolder(assets.DefaultBuildFS, "web/default/dist")
	classicFS := common.EmbedFolder(assets.ClassicBuildFS, "web/classic/dist")

	// 创建主题感知的文件系统
	// 根据当前主题配置自动选择对应的文件系统
	themeFS := common.NewThemeAwareFS(defaultFS, classicFS)

	// 注册 gzip 压缩中间件
	router.Use(gzip.Gzip(gzip.DefaultCompression))

	// 注册全局 Web 限流中间件
	router.Use(middleware.GlobalWebRateLimit())

	// 注册缓存中间件
	router.Use(middleware.Cache())

	// 注册静态文件服务
	// "/" 路径下的所有请求都会尝试从 themeFS 中查找文件
	router.Use(static.Serve("/", themeFS))

	// NoRoute 处理器：SPA 路由回退
	// 当没有匹配的路由时，返回 index.html 让前端路由处理
	router.NoRoute(func(c *gin.Context) {
		// 设置路由标签
		c.Set(middleware.RouteTagKey, "web")

		// 如果是 API 或静态资源请求，返回 404
		if strings.HasPrefix(c.Request.RequestURI, "/v1") ||
			strings.HasPrefix(c.Request.RequestURI, "/api") ||
			strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}

		// 设置不缓存响应头
		c.Header("Cache-Control", "no-cache")

		// 根据当前主题返回对应的 index.html
		if common.GetTheme() == "classic" {
			c.Data(http.StatusOK, "text/html; charset=utf-8", assets.ClassicIndexPage)
		} else {
			c.Data(http.StatusOK, "text/html; charset=utf-8", assets.DefaultIndexPage)
		}
	})
}
