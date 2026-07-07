// Package router - system-task-router.go
// 该文件集中注册系统任务路由。系统任务会暴露后台任务输入、进度、结果和错误，
// 属于运维敏感数据，因此统一要求 Root 权限。
package router

import (
	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"

	"github.com/gin-gonic/gin"
)

// registerSystemTaskRoutes 注册 /api/system-task 路由组。
func registerSystemTaskRoutes(apiRouter *gin.RouterGroup) {
	systemTaskRoute := apiRouter.Group("/system-task")
	systemTaskRoute.Use(middleware.RootAuth())
	{
		systemTaskRoute.GET("/list", controller.ListSystemTasks)
		systemTaskRoute.GET("/current", controller.GetCurrentSystemTask)
		systemTaskRoute.GET("/:task_id", controller.GetSystemTask)
	}
}
