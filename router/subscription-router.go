// Package router - subscription-router.go
// 该文件集中注册订阅管理路由。
//
// 用户购买和支付回调仍保留在主 API 路由中；这里仅处理 /api/subscription/admin，
// 并在 AdminAuth 后按 subscription 资源动作执行二次权限校验。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerSubscriptionAdminRoutes 注册 /api/subscription/admin 管理路由组。
func registerSubscriptionAdminRoutes(apiRouter *gin.RouterGroup) {
	subscriptionAdminRoute := apiRouter.Group("/subscription/admin")
	subscriptionAdminRoute.Use(middleware.AdminAuth())
	registerPermissionRoutes(subscriptionAdminRoute, subscriptionPermissionRoutes)
}

var subscriptionPermissionRoutes = []permissionRoute{
	// 套餐管理
	{method: http.MethodGet, path: "/plans", permission: authz.SubscriptionRead, handler: controller.AdminListSubscriptionPlans},
	{method: http.MethodPost, path: "/plans", permission: authz.SubscriptionWrite, handler: controller.AdminCreateSubscriptionPlan},
	{method: http.MethodPut, path: "/plans/:id", permission: authz.SubscriptionWrite, handler: controller.AdminUpdateSubscriptionPlan},
	{method: http.MethodPatch, path: "/plans/:id", permission: authz.SubscriptionOperate, handler: controller.AdminUpdateSubscriptionPlanStatus},
	{method: http.MethodPost, path: "/plans/:id/subscriptions/reset", permission: authz.SubscriptionOperate, handler: controller.AdminResetPlanSubscriptions},

	// 用户订阅管理。删除会直接移除权益记录，按敏感写处理；失效与创建仍属于运营动作。
	{method: http.MethodPost, path: "/bind", permission: authz.SubscriptionOperate, handler: controller.AdminBindSubscription},
	{method: http.MethodGet, path: "/users/:id/subscriptions", permission: authz.SubscriptionRead, handler: controller.AdminListUserSubscriptions},
	{method: http.MethodPost, path: "/users/:id/subscriptions", permission: authz.SubscriptionOperate, handler: controller.AdminCreateUserSubscription},
	{method: http.MethodPost, path: "/users/:id/subscriptions/reset", permission: authz.SubscriptionOperate, handler: controller.AdminResetUserSubscriptionsByPlan},
	{method: http.MethodPost, path: "/user_subscriptions/:id/invalidate", permission: authz.SubscriptionOperate, handler: controller.AdminInvalidateUserSubscription},
	{method: http.MethodDelete, path: "/user_subscriptions/:id", permission: authz.SubscriptionSensitiveWrite, handler: controller.AdminDeleteUserSubscription},
}
