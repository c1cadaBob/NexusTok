// Package router - redemption-router.go
// 该文件集中注册兑换码管理路由。
//
// 兑换码会直接影响钱包额度发放，因此从用户管理权限中拆成独立 redemption 资源。
// 路由仍先经过 AdminAuth，再按 read/write/sensitive_write/secret_view 做第二层校验。
package router

import (
	"net/http"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/middleware"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
)

// registerRedemptionRoutes 注册 /api/redemption 路由组。
func registerRedemptionRoutes(apiRouter *gin.RouterGroup) {
	redemptionRoute := apiRouter.Group("/redemption")
	redemptionRoute.Use(middleware.AdminAuth())

	registerPermissionRoutes(redemptionRoute, redemptionPermissionRoutes)
}

var redemptionPermissionRoutes = []permissionRoute{
	// 兑换码查询
	{method: http.MethodGet, path: "/", permission: authz.RedemptionRead, handler: controller.GetAllRedemptions},
	{method: http.MethodGet, path: "/search", permission: authz.RedemptionRead, handler: controller.SearchRedemptions},
	{method: http.MethodGet, path: "/:id", permission: authz.RedemptionRead, handler: controller.GetRedemption},

	// 完整兑换码等价于可入账凭据，必须显式 secret_view 且通过安全验证后才 reveal。
	{
		method:     http.MethodPost,
		path:       "/:id/key",
		permission: authz.RedemptionSecretView,
		after:      []gin.HandlerFunc{middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.SecureVerificationRequired()},
		handler:    controller.GetRedemptionKey,
	},

	// 兑换码维护。创建和编辑保持 Admin write 兼容；删除会移除可审计记录，按敏感写处理。
	{method: http.MethodPost, path: "/", permission: authz.RedemptionWrite, handler: controller.AddRedemption},
	{method: http.MethodPut, path: "/", permission: authz.RedemptionWrite, handler: controller.UpdateRedemption},
	{method: http.MethodDelete, path: "/invalid", permission: authz.RedemptionSensitiveWrite, handler: controller.DeleteInvalidRedemption},
	{method: http.MethodDelete, path: "/:id", permission: authz.RedemptionSensitiveWrite, handler: controller.DeleteRedemption},
}
