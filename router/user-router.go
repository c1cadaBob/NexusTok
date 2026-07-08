// Package router - user-router.go
// 该文件集中注册用户管理的管理员路由。
//
// 用户 Admin 路由先经过 AdminAuth，再按 authz 权限表执行 read/operate/write/
// sensitive_write 二次校验。公开登录、用户自助资料、充值支付和 webhook 路径仍留在
// api-router.go，避免把普通用户路径错误纳入管理员资源权限表。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerUserAdminRoutes 注册 /api/user 下的管理员子路由。
func registerUserAdminRoutes(userRoute *gin.RouterGroup) {
	adminRoute := userRoute.Group("/")
	adminRoute.Use(middleware.AdminAuth())

	registerPermissionRoutes(adminRoute, userAdminPermissionRoutes)
}

var userAdminPermissionRoutes = []permissionRoute{
	// 用户查询
	{method: http.MethodGet, path: "/", permission: authz.UserRead, handler: controller.GetAllUsers},
	{method: http.MethodGet, path: "/topup", permission: authz.UserRead, handler: controller.GetAllTopUps},
	{method: http.MethodGet, path: "/search", permission: authz.UserRead, handler: controller.SearchUsers},
	{method: http.MethodGet, path: "/:id/oauth/bindings", permission: authz.UserRead, handler: controller.GetUserOAuthBindingsByAdmin},
	{method: http.MethodGet, path: "/:id", permission: authz.UserRead, handler: controller.GetUser},
	{method: http.MethodGet, path: "/2fa/stats", permission: authz.UserRead, handler: controller.Admin2FAStats},

	// 用户运营动作。ManageUser 是复合动作，删除、升降级和额度调整会在 handler 内额外要求敏感写权限。
	{method: http.MethodDelete, path: "/:id/oauth/bindings/:provider_id", permission: authz.UserOperate, handler: controller.UnbindCustomOAuthByAdmin},
	{method: http.MethodDelete, path: "/:id/bindings/:binding_type", permission: authz.UserOperate, handler: controller.AdminClearUserBinding},
	{method: http.MethodPost, path: "/manage", permission: authz.UserOperate, handler: controller.ManageUser},
	{method: http.MethodDelete, path: "/:id/reset_passkey", permission: authz.UserOperate, handler: controller.AdminResetPasskey},
	{method: http.MethodDelete, path: "/:id/2fa", permission: authz.UserOperate, handler: controller.AdminDisable2FA},

	// 用户普通写
	{method: http.MethodPut, path: "/", permission: authz.UserWrite, handler: controller.UpdateUser},

	// 用户敏感写
	{method: http.MethodPost, path: "/topup/complete", permission: authz.UserSensitiveWrite, handler: controller.AdminCompleteTopUp},
	{method: http.MethodPost, path: "/", permission: authz.UserSensitiveWrite, handler: controller.CreateUser},
	{method: http.MethodDelete, path: "/:id", permission: authz.UserSensitiveWrite, handler: controller.DeleteUser},
}
