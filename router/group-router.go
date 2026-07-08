// Package router - group-router.go
// 该文件集中注册管理端分组和预填充分组路由。
//
// `/api/group` 返回模型/计费组名称，`/api/prefill_group` 维护模型、标签和
// 端点预填模板。两者都服务模型与渠道配置，因此复用 model 权限资源；普通用户
// 自助 `/api/user/self/groups` 路径继续保留在用户路由中，不进入管理员权限表。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerGroupRoutes 注册管理端分组和预填充分组路由组。
func registerGroupRoutes(apiRouter *gin.RouterGroup) {
	groupRoute := apiRouter.Group("/group")
	groupRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(groupRoute, groupPermissionRoutes)

	prefillGroupRoute := apiRouter.Group("/prefill_group")
	prefillGroupRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(prefillGroupRoute, prefillGroupPermissionRoutes)
}

var groupPermissionRoutes = []permissionRoute{
	// 管理端读取模型/计费组名称，供用户、订阅、渠道和模型配置下拉选项复用。
	{method: http.MethodGet, path: "/", permission: authz.ModelRead, handler: controller.GetGroups},
}

var prefillGroupPermissionRoutes = []permissionRoute{
	// 预填模板查询属于模型配置元数据读取。
	{method: http.MethodGet, path: "/", permission: authz.ModelRead, handler: controller.GetPrefillGroups},

	// 创建和编辑预填模板是模型配置辅助写操作；删除共享模板按敏感写处理。
	{method: http.MethodPost, path: "/", permission: authz.ModelWrite, handler: controller.CreatePrefillGroup},
	{method: http.MethodPut, path: "/", permission: authz.ModelWrite, handler: controller.UpdatePrefillGroup},
	{method: http.MethodDelete, path: "/:id", permission: authz.ModelSensitiveWrite, handler: controller.DeletePrefillGroup},
}
