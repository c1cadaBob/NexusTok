// Package router - authz-router.go
// 该文件注册管理权限 catalog 路由。catalog 只包含资源/动作 schema 和内置角色基线，
// 暂不暴露用户 override，也不改变现有 AdminAuth/RootAuth 行为。
package router

import (
	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/gin-gonic/gin"
)

// registerAuthzRoutes 注册 /api/authz 路由组。
func registerAuthzRoutes(apiRouter *gin.RouterGroup) {
	authzRoute := apiRouter.Group("/authz")
	authzRoute.Use(middleware.AdminAuth())
	{
		authzRoute.GET("/catalog", controller.GetPermissionCatalog)
	}
}
