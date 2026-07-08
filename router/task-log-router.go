// Package router - task-log-router.go
// 该文件集中注册绘图任务与通用异步任务日志路由。
//
// `/self` 路径只查询当前用户自己的任务历史，继续保持 UserAuth；管理员根路径会
// 暴露跨用户任务、渠道、费用、上游任务 ID 和失败原因，因此归入 usage_log.read
// 权限表，同时保留 AdminAuth 作为当前灰度阶段的基础边界。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerTaskLogRoutes 注册 /api/mj 与 /api/task 任务日志路由组。
func registerTaskLogRoutes(apiRouter *gin.RouterGroup) {
	mjSelfRoute := apiRouter.Group("/mj")
	mjSelfRoute.GET("/self", middleware.UserAuth(), controller.GetUserMidjourney)

	mjAdminRoute := apiRouter.Group("/mj")
	mjAdminRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(mjAdminRoute, midjourneyLogPermissionRoutes)

	taskSelfRoute := apiRouter.Group("/task")
	taskSelfRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)

	taskAdminRoute := apiRouter.Group("/task")
	taskAdminRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(taskAdminRoute, asyncTaskLogPermissionRoutes)
}

var midjourneyLogPermissionRoutes = []permissionRoute{
	// 管理员跨用户绘图任务日志查询会暴露用户、渠道、成本、结果 URL 和失败原因。
	{method: http.MethodGet, path: "/", permission: authz.UsageLogRead, handler: controller.GetAllMidjourney},
}

var asyncTaskLogPermissionRoutes = []permissionRoute{
	// 管理员跨用户异步任务日志查询会暴露上游任务 ID、渠道、用户和失败原因。
	{method: http.MethodGet, path: "/", permission: authz.UsageLogRead, handler: controller.GetAllTask},
}
