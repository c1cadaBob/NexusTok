// Package router - log-data-router.go
// 该文件集中注册管理员日志与用量数据路由。
//
// 日志和用量数据会暴露跨用户请求、成本、渠道、Token 名称、审计信息和流量
// 聚合维度，因此从泛化 AdminAuth 中拆成 usage_log/usage_data 独立资源。普通
// 用户 self 路径和 Token 自身日志查询不在这里注册，避免被管理员权限表误拦截。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerLogDataAdminRoutes 注册跨用户日志和用量数据的管理员路由。
func registerLogDataAdminRoutes(apiRouter *gin.RouterGroup) {
	logRoute := apiRouter.Group("/log")
	logRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(logRoute, logPermissionRoutes)

	auditLogRoute := apiRouter.Group("/audit-log")
	auditLogRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(auditLogRoute, auditLogPermissionRoutes)

	dataRoute := apiRouter.Group("/data")
	dataRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(dataRoute, dataPermissionRoutes)
}

var logPermissionRoutes = []permissionRoute{
	// 管理员跨用户日志查询和统计。search 是兼容旧前端的废弃入口，仍按只读处理。
	{method: http.MethodGet, path: "/", permission: authz.UsageLogRead, handler: controller.GetAllLogs},
	{method: http.MethodGet, path: "/stat", permission: authz.UsageLogRead, handler: controller.GetLogsStat},
	{method: http.MethodGet, path: "/search", permission: authz.UsageLogRead, handler: controller.SearchAllLogs},
	{method: http.MethodGet, path: "/channel_affinity_usage_cache", permission: authz.UsageLogRead, handler: controller.GetChannelAffinityUsageCacheStats},

	// 同步删除历史日志会移除请求与管理审计证据，按敏感写处理。
	{method: http.MethodDelete, path: "/", permission: authz.UsageLogSensitiveWrite, handler: controller.DeleteHistoryLogs},
}

var auditLogPermissionRoutes = []permissionRoute{
	// 审计日志会暴露管理员操作路径、操作者和登录来源 IP，沿用 usage_log.read 统一授权。
	{method: http.MethodGet, path: "/", permission: authz.UsageLogRead, handler: controller.GetAuditLogs},
}

var dataPermissionRoutes = []permissionRoute{
	// 管理员跨用户用量趋势、用户排行和流量账本聚合查询。
	{method: http.MethodGet, path: "/", permission: authz.UsageDataRead, handler: controller.GetAllQuotaDates},
	{method: http.MethodGet, path: "/users", permission: authz.UsageDataRead, handler: controller.GetQuotaDatesByUser},
	{method: http.MethodGet, path: "/flow", permission: authz.UsageDataRead, handler: controller.GetAllFlowQuotaDates},
}
