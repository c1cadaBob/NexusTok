// Package router 负责 HTTP 路由配置
// 该包定义了所有的 API 路由、中继路由、仪表盘路由和 Web 前端路由
// 路由采用分层设计，将不同功能的路由分离到不同的文件中
package router

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/c1cada/NexusTok/common"     // 公共工具包
	"github.com/c1cada/NexusTok/middleware" // 中间件包

	"github.com/gin-gonic/gin" // Gin Web 框架
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

// SetRouter 设置所有 HTTP 路由
// 这是路由配置的主入口函数，负责注册所有路由组：
// 1. API 路由 - 用户认证、渠道管理、令牌管理等
// 2. 仪表盘路由 - 管理后台
// 3. 中继路由 - AI API 代理转发
// 4. 视频路由 - 视频生成相关 API
// 5. Web 前端路由 - 静态文件服务和 SPA 路由
//
// 参数：
//   - router: Gin 引擎实例
//   - assets: 前端主题资源
func SetRouter(router *gin.Engine, assets ThemeAssets) {
	// 设置 API 路由（/api/ 前缀）
	SetApiRouter(router)

	// 设置仪表盘路由（/dashboard/ 前缀）
	SetDashboardRouter(router)

	// 设置中继路由（用于转发 AI API 请求）
	SetRelayRouter(router)

	// 设置视频路由（视频生成相关）
	SetVideoRouter(router)

	// 获取前端基础 URL 配置
	// 如果配置了 FRONTEND_BASE_URL，前端将使用外部服务
	frontendBaseUrl := os.Getenv("FRONTEND_BASE_URL")

	// 主节点忽略 FRONTEND_BASE_URL 配置
	// 主节点必须使用内置的前端资源
	if common.IsMasterNode && frontendBaseUrl != "" {
		frontendBaseUrl = ""
		common.SysLog("FRONTEND_BASE_URL is ignored on master node")
	}

	// 根据前端 URL 配置决定路由策略
	if frontendBaseUrl == "" {
		// 未配置外部前端 URL，使用内置前端资源
		SetWebRouter(router, assets)
	} else {
		// 配置了外部前端 URL，将所有未匹配的请求重定向到外部前端
		frontendBaseUrl = strings.TrimSuffix(frontendBaseUrl, "/")

		// NoRoute 处理器：处理所有未匹配的路由
		router.NoRoute(func(c *gin.Context) {
			// 设置路由标签，用于日志和监控
			c.Set(middleware.RouteTagKey, "web")
			// 使用 301 永久重定向到外部前端
			c.Redirect(http.StatusMovedPermanently, fmt.Sprintf("%s%s", frontendBaseUrl, c.Request.RequestURI))
		})
	}
}
