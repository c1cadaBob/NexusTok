// Package router - system-task-router.go
// 该文件集中注册系统任务路由。
//
// 系统任务会暴露后台任务输入、进度、结果和错误，属于运维敏感数据；路由继续
// 保留 RootAuth，并叠加 system_setting 权限表。日志清理任务还会删除历史日志和
// 审计证据，因此额外要求 usage_log.sensitive_write。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerSystemTaskRoutes 注册 /api/system-task 路由组。
func registerSystemTaskRoutes(apiRouter *gin.RouterGroup) {
	systemTaskRoute := apiRouter.Group("/system-task")
	systemTaskRoute.Use(middleware.RootAuth())
	registerPermissionRoutes(systemTaskRoute, systemTaskPermissionRoutes)
}

var systemTaskPermissionRoutes = []permissionRoute{
	// 日志清理是异步任务创建入口，但最终会删除历史请求日志和审计证据，需同时满足
	// 系统敏感写与日志删除权限；当前 RootAuth 保底下不会改变既有 Root-only 行为。
	{
		method:     http.MethodPost,
		path:       "/log-cleanup",
		permission: authz.SystemSettingSensitiveWrite,
		after:      []gin.HandlerFunc{middleware.RequirePermission(authz.UsageLogSensitiveWrite)},
		handler:    controller.CreateLogCleanupSystemTask,
	},

	// 系统任务观测会暴露任务 payload、进度、结果和错误，只按只读接口开放。
	{method: http.MethodGet, path: "/list", permission: authz.SystemSettingRead, handler: controller.ListSystemTasks},
	{method: http.MethodGet, path: "/current", permission: authz.SystemSettingRead, handler: controller.GetCurrentSystemTask},
	{method: http.MethodGet, path: "/:task_id", permission: authz.SystemSettingRead, handler: controller.GetSystemTask},
}
