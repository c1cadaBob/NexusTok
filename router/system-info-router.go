// Package router - system-info-router.go
// 该文件集中注册系统信息路由。系统信息包含节点心跳和后续系统任务观测，
// 属于运维敏感数据，当前统一要求 Root 权限。
package router

import (
	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"

	"github.com/gin-gonic/gin"
)

// registerSystemInfoRoutes 注册 /api/system-info 路由组。
func registerSystemInfoRoutes(apiRouter *gin.RouterGroup) {
	systemInfoRoute := apiRouter.Group("/system-info")
	systemInfoRoute.Use(middleware.RootAuth())
	{
		systemInfoRoute.GET("/instances", controller.ListSystemInstances)
	}
}
