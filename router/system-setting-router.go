// Package router - system-setting-router.go
// 该文件集中注册系统设置和运行维护相关路由。
//
// 这些接口会读取或修改全局配置、OAuth 登录入口、性能缓存、服务器日志和上游
// 倍率同步数据。除 `/api/status/test` 这个只读诊断接口保持 Admin 边界外，其余
// 路由继续保留 RootAuth，并在旧认证边界后叠加 system_setting 权限表二次校验。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerSystemSettingRoutes 注册系统设置与运行维护路由。
func registerSystemSettingRoutes(apiRouter *gin.RouterGroup) {
	statusRoute := apiRouter.Group("/status")
	statusRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(statusRoute, statusDiagnosticPermissionRoutes)

	optionRoute := apiRouter.Group("/option")
	optionRoute.Use(middleware.RootAuth())
	registerPermissionRoutes(optionRoute, optionPermissionRoutes)

	customOAuthRoute := apiRouter.Group("/custom-oauth-provider")
	customOAuthRoute.Use(middleware.RootAuth())
	registerPermissionRoutes(customOAuthRoute, customOAuthProviderPermissionRoutes)

	performanceRoute := apiRouter.Group("/performance")
	performanceRoute.Use(middleware.RootAuth())
	registerPermissionRoutes(performanceRoute, performancePermissionRoutes)

	ratioSyncRoute := apiRouter.Group("/ratio_sync")
	ratioSyncRoute.Use(middleware.RootAuth())
	registerPermissionRoutes(ratioSyncRoute, ratioSyncPermissionRoutes)
}

var statusDiagnosticPermissionRoutes = []permissionRoute{
	// 只读诊断接口保持 Admin 可访问，避免把旧的状态检测工作流误收紧为 Root-only。
	{method: http.MethodGet, path: "/test", permission: authz.SystemSettingRead, handler: controller.TestStatus},
}

var optionPermissionRoutes = []permissionRoute{
	// 系统选项读取和缓存统计
	{method: http.MethodGet, path: "/", permission: authz.SystemSettingRead, handler: controller.GetOptions},
	{method: http.MethodGet, path: "/channel_affinity_cache", permission: authz.SystemSettingRead, handler: controller.GetChannelAffinityCacheStats},

	// 通用 option 更新可能触碰密钥、支付、OAuth 和安全配置，路由层按敏感写处理。
	{method: http.MethodPut, path: "/", permission: authz.SystemSettingSensitiveWrite, handler: controller.UpdateOption},
	{method: http.MethodPost, path: "/payment_compliance", permission: authz.SystemSettingSensitiveWrite, handler: controller.ConfirmPaymentCompliance},
	// Waffo Pancake 支付配置需要一次提交多项敏感 option，沿用系统设置敏感写权限。
	{method: http.MethodPost, path: "/waffo-pancake/save", permission: authz.SystemSettingSensitiveWrite, handler: controller.SaveWaffoPancakeConfig},
	{method: http.MethodPost, path: "/rest_model_ratio", permission: authz.SystemSettingSensitiveWrite, handler: controller.ResetModelRatio},
	{method: http.MethodPost, path: "/migrate_console_setting", permission: authz.SystemSettingSensitiveWrite, handler: controller.MigrateConsoleSetting},

	// 清理渠道亲和缓存是运行维护动作，不直接修改持久化配置。
	{method: http.MethodDelete, path: "/channel_affinity_cache", permission: authz.SystemSettingOperate, handler: controller.ClearChannelAffinityCache},
}

var customOAuthProviderPermissionRoutes = []permissionRoute{
	// 自定义 OAuth provider 会影响登录入口和访问准入策略，继续保留 RootAuth。
	{method: http.MethodGet, path: "/", permission: authz.SystemSettingRead, handler: controller.GetCustomOAuthProviders},
	{method: http.MethodGet, path: "/:id", permission: authz.SystemSettingRead, handler: controller.GetCustomOAuthProvider},
	{method: http.MethodPost, path: "/discovery", permission: authz.SystemSettingOperate, handler: controller.FetchCustomOAuthDiscovery},
	{method: http.MethodPost, path: "/", permission: authz.SystemSettingSensitiveWrite, handler: controller.CreateCustomOAuthProvider},
	{method: http.MethodPut, path: "/:id", permission: authz.SystemSettingSensitiveWrite, handler: controller.UpdateCustomOAuthProvider},
	{method: http.MethodDelete, path: "/:id", permission: authz.SystemSettingSensitiveWrite, handler: controller.DeleteCustomOAuthProvider},
}

var performancePermissionRoutes = []permissionRoute{
	// 运行状态与日志文件列表读取
	{method: http.MethodGet, path: "/stats", permission: authz.SystemSettingRead, handler: controller.GetPerformanceStats},
	{method: http.MethodGet, path: "/logs", permission: authz.SystemSettingRead, handler: controller.GetLogFiles},

	// 性能维护动作
	{method: http.MethodDelete, path: "/disk_cache", permission: authz.SystemSettingOperate, handler: controller.ClearDiskCache},
	{method: http.MethodPost, path: "/reset_stats", permission: authz.SystemSettingOperate, handler: controller.ResetPerformanceStats},
	{method: http.MethodPost, path: "/gc", permission: authz.SystemSettingOperate, handler: controller.ForceGC},

	// 删除服务器日志会移除运维证据，按敏感写处理。
	{method: http.MethodDelete, path: "/logs", permission: authz.SystemSettingSensitiveWrite, handler: controller.CleanupLogFiles},
}

var ratioSyncPermissionRoutes = []permissionRoute{
	// 上游倍率同步属于系统设置页中的运维工具，读取渠道候选与拉取差异分开授权。
	{method: http.MethodGet, path: "/channels", permission: authz.SystemSettingRead, handler: controller.GetSyncableChannels},
	{method: http.MethodPost, path: "/fetch", permission: authz.SystemSettingOperate, handler: controller.FetchUpstreamRatios},
}
