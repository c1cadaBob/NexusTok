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

	"github.com/gin-contrib/gzip"     // gzip 压缩
	"github.com/gin-contrib/sessions" // 会话管理
	"github.com/gin-contrib/static"   // 静态文件服务
	"github.com/gin-gonic/gin"        // Gin 框架
)

// ThemeAssets 前端主题资源结构体
// 包含所有前端主题的嵌入文件系统和入口页面
// 用于支持多主题切换功能
type ThemeAssets struct {
	DefaultBuildFS      embed.FS // 默认主题的完整前端资源文件系统
	DefaultIndexPage    []byte   // 默认主题的入口 HTML 页面
	ClassicBuildFS      embed.FS // 经典主题的完整前端资源文件系统
	ClassicIndexPage    []byte   // 经典主题的入口 HTML 页面
	CPAManagerBuildFS   embed.FS // CPA 管理器模块的前端资源文件系统
	CPAManagerIndexPage []byte   // CPA 管理器模块的入口 HTML 页面
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
	cpaManagerFS := common.EmbedFolder(assets.CPAManagerBuildFS, "modules/cpa-manager/dist")

	// 创建主题感知的文件系统
	// 根据当前主题配置自动选择对应的文件系统
	themeFS := common.NewThemeAwareFS(defaultFS, classicFS)

	// 注册 gzip 压缩中间件
	router.Use(gzip.Gzip(gzip.DefaultCompression))

	// 注册全局 Web 限流中间件
	router.Use(middleware.GlobalWebRateLimit())

	// 注册缓存中间件
	router.Use(middleware.Cache())

	// 设置账号池管理器路由
	setAccountPoolManagerRouter(router, cpaManagerFS, assets.CPAManagerIndexPage)

	// 设置账号池请求监控 Usage Service 代理路由
	// 这些路径必须注册在静态文件服务之前，否则 /usage-service、/status 和 /v0/management/*
	// 会被前端 SPA 回退吞掉，CPAMC 的请求监控页面就只能看到“服务不可用”。
	setAccountPoolUsageServiceProxyRouter(router)

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

// setAccountPoolManagerRouter 设置账号池管理器路由
// 账号池管理器是一个独立的前端模块，用于管理 AI 账号池
// 需要管理员权限才能访问
//
// 路由：
// - /account-pool/manager - 重定向到带斜杠的路径
// - /account-pool/manager/*path - 提供静态文件或回退到 index.html
//
// 参数：
//   - router: Gin 引擎实例
//   - cpaManagerFS: CPA 管理器静态文件系统
//   - indexPage: CPA 管理器入口页面
func setAccountPoolManagerRouter(router *gin.Engine, cpaManagerFS static.ServeFileSystem, indexPage []byte) {
	// 重定向不带斜杠的路径到带斜杠的路径
	router.GET("/account-pool/manager", accountPoolManagerSessionAuth(), func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/account-pool/manager/")
	})

	// 处理账号池管理器的所有请求
	router.GET("/account-pool/manager/*path", accountPoolManagerSessionAuth(), func(c *gin.Context) {
		// 设置路由标签
		c.Set(middleware.RouteTagKey, "web")

		// 获取请求路径并移除前导斜杠
		requestPath := strings.TrimPrefix(c.Param("path"), "/")

		// 如果请求的是具体文件且文件存在，直接返回文件
		if requestPath != "" && cpaManagerFS.Exists("/", requestPath) {
			c.FileFromFS(requestPath, cpaManagerFS)
			return
		}

		// 设置不缓存响应头
		c.Header("Cache-Control", "no-cache")

		// 返回 index.html（SPA 路由回退）
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	})
}

// setAccountPoolUsageServiceProxyRouter 设置 CPAMC 请求监控 Usage Service 的同源代理路由。
//
// CPA-Manager 原生请求监控功能默认访问 Usage Service 的根路径端点：
// - /usage-service/info 和 /usage-service/config 用于探测与读取采集器配置；
// - /status 用于读取采集器和数据库状态；
// - /v0/management/usage、model-prices、api-key-aliases 用于读取监控数据和辅助配置。
//
// NexusTok 嵌入 CPAMC 后，浏览器同源是主项目而不是 Usage Service。本路由只允许
// 已登录管理员访问，并由 controller.AccountPoolUsageServiceProxy 在服务端注入内部
// management key，从而让页面保留原请求路径，同时继续复用 NexusTok 的登录态与权限。
func setAccountPoolUsageServiceProxyRouter(router *gin.Engine) {
	auth := accountPoolManagerSessionAuth()
	handler := controller.AccountPoolUsageServiceProxy

	router.Any("/usage-service", auth, handler)
	router.Any("/usage-service/*path", auth, handler)
	router.Any("/status", auth, handler)
	router.Any("/v0/management/usage", auth, handler)
	router.Any("/v0/management/usage/*path", auth, handler)
	router.Any("/v0/management/model-prices", auth, handler)
	router.Any("/v0/management/model-prices/*path", auth, handler)
	router.Any("/v0/management/api-key-aliases", auth, handler)
	router.Any("/v0/management/api-key-aliases/*path", auth, handler)
}

// accountPoolManagerSessionAuth 账号池管理器会话认证中间件
// 验证用户是否已登录、状态是否正常、是否有管理员权限
//
// 认证流程：
// 1. 检查会话中是否有用户信息
// 2. 检查用户状态是否为禁用
// 3. 检查用户角色是否为管理员
//
// 返回值：
//   - gin.HandlerFunc: Gin 中间件函数
func accountPoolManagerSessionAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取会话
		session := sessions.Default(c)

		// 从会话中获取用户角色和状态
		role, roleOK := session.Get("role").(int)
		status, statusOK := session.Get("status").(int)

		// 检查用户是否已登录
		if session.Get("id") == nil || !roleOK || !statusOK {
			// 未登录，重定向到登录页面
			loginPath := "/sign-in"
			if common.GetTheme() == "classic" {
				loginPath = "/login"
			}
			redirect := loginPath + "?redirect=/account-pool/manager/"

			// 设置不缓存响应头
			c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")

			c.Redirect(http.StatusFound, redirect)
			c.Abort()
			return
		}

		// 检查用户状态是否为禁用
		if status == common.UserStatusDisabled {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		// 检查用户角色是否为管理员
		if role < common.RoleAdminUser {
			c.AbortWithStatus(http.StatusForbidden)
			return
		}

		// 认证通过，继续处理请求
		c.Next()
	}
}
