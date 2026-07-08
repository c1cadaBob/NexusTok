// Package router - system-info-router.go
// 该文件集中注册系统信息路由。
//
// 系统信息包含节点心跳、角色、版本和资源占用，属于运维敏感数据；路由继续
// 保留 RootAuth，并叠加 system_setting.read 权限表，方便后续用户级 override
// 复用同一套资源动作语义。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerSystemInfoRoutes 注册 /api/system-info 路由组。
func registerSystemInfoRoutes(apiRouter *gin.RouterGroup) {
	systemInfoRoute := apiRouter.Group("/system-info")
	systemInfoRoute.Use(middleware.RootAuth())
	registerPermissionRoutes(systemInfoRoute, systemInfoPermissionRoutes)
}

var systemInfoPermissionRoutes = []permissionRoute{
	// 节点心跳会暴露主机名、角色、版本和资源状态，继续保持 RootAuth 保底。
	{method: http.MethodGet, path: "/instances", permission: authz.SystemSettingRead, handler: controller.ListSystemInstances},
}
