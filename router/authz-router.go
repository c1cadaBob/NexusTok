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
	// 权限策略导出包含完整角色与 Casbin 兼容策略快照，继续保持 RootAuth 保底；
	// secret_view 作为权限表中的二次边界，便于后续审计和用户级 override 统一分类。
	{method: http.MethodGet, path: "/policies/export", permission: authz.SystemSettingSecretView, before: []gin.HandlerFunc{middleware.RootAuth()}, handler: controller.ExportPermissionPolicies},
	// 角色策略矩阵会暴露完整管理边界，读取也继续保持 RootAuth 保底。
	{method: http.MethodGet, path: "/roles", permission: authz.SystemSettingSecretView, before: []gin.HandlerFunc{middleware.RootAuth()}, handler: controller.ListPermissionRoles},
	// 自定义角色模板创建只改变权限模板元数据，仍属于高风险权限配置写操作。
	{method: http.MethodPost, path: "/roles", permission: authz.SystemSettingSensitiveWrite, before: []gin.HandlerFunc{middleware.RootAuth()}, handler: controller.CreatePermissionRole},
	// 内置 Root/Admin 模板由 service 层保护为只读；路由层统一保持 RootAuth 保底。
	{method: http.MethodPut, path: "/roles/:key", permission: authz.SystemSettingSensitiveWrite, before: []gin.HandlerFunc{middleware.RootAuth()}, handler: controller.UpdatePermissionRole},
	// 删除自定义角色模板会同步清理该模板的 role:* 策略，继续归类为敏感写操作。
	{method: http.MethodDelete, path: "/roles/:key", permission: authz.SystemSettingSensitiveWrite, before: []gin.HandlerFunc{middleware.RootAuth()}, handler: controller.DeletePermissionRole},
	// 角色策略更新是高风险写操作：service 层默认 dry-run，路由层仍以 RootAuth 保底。
	{method: http.MethodPut, path: "/roles/:key/policies", permission: authz.SystemSettingSensitiveWrite, before: []gin.HandlerFunc{middleware.RootAuth()}, handler: controller.UpdatePermissionRolePolicies},
	// 权限策略导入会写入角色、role:* 策略和用户 override，必须继续 RootAuth 保底；
	// sensitive_write 用作权限表中的写入分类，默认 dry-run 由 service 层兜底。
	{method: http.MethodPost, path: "/policies/import", permission: authz.SystemSettingSensitiveWrite, before: []gin.HandlerFunc{middleware.RootAuth()}, handler: controller.ImportPermissionPolicies},
}
