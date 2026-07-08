// Package router - authz-router.go
// 该文件注册管理权限 catalog 路由。catalog 只包含资源/动作 schema 和内置角色基线，
// 暂不暴露用户 override，也不改变现有 AdminAuth 行为；路由叠加 system_setting.read，
// 让权限 schema 只读入口和系统配置元数据进入同一套灰度权限表。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerAuthzRoutes 注册 /api/authz 路由组。
func registerAuthzRoutes(apiRouter *gin.RouterGroup) {
	authzRoute := apiRouter.Group("/authz")
	authzRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(authzRoute, authzPermissionRoutes)
}

var authzPermissionRoutes = []permissionRoute{
	// 权限 catalog 暴露资源、动作和角色基线矩阵，属于系统权限配置的只读元数据。
	{method: http.MethodGet, path: "/catalog", permission: authz.SystemSettingRead, handler: controller.GetPermissionCatalog},
}
