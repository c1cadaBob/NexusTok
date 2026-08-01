// Package router - system-update-router.go
// 该文件集中注册系统维护更新路由。更新、回滚和重启都可能改变正在运行的二进制和
// 服务生命周期，因此继续要求 RootAuth，并叠加 system_setting 权限表。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerSystemUpdateRoutes 注册 /api/system-update 路由组。
func registerSystemUpdateRoutes(apiRouter *gin.RouterGroup) {
	systemUpdateRoute := apiRouter.Group("/system-update")
	systemUpdateRoute.Use(middleware.RootAuth())
	registerPermissionRoutes(systemUpdateRoute, systemUpdatePermissionRoutes)
}

var systemUpdatePermissionRoutes = []permissionRoute{
	{method: http.MethodGet, path: "/latest", permission: authz.SystemSettingRead, handler: controller.GetLatestSystemUpdate},
	{method: http.MethodPost, path: "/apply", permission: authz.SystemSettingSensitiveWrite, handler: controller.ApplySystemUpdate},
	{method: http.MethodPost, path: "/rollback", permission: authz.SystemSettingSensitiveWrite, handler: controller.RollbackSystemUpdate},
	{method: http.MethodPost, path: "/restart", permission: authz.SystemSettingSensitiveWrite, handler: controller.RestartSystemUpdate},
}
