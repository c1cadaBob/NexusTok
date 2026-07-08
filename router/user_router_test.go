package router

import (
	"net/http"
	"testing"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterUserAdminRoutesKeepsCoreHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")
	user := api.Group("/user")

	require.NotPanics(t, func() {
		registerUserAdminRoutes(user)
	})

	assertRouteHandler(t, engine, http.MethodGet, "/api/user/", controller.GetAllUsers)
	assertRouteHandler(t, engine, http.MethodGet, "/api/user/topup", controller.GetAllTopUps)
	assertRouteHandler(t, engine, http.MethodPost, "/api/user/topup/complete", controller.AdminCompleteTopUp)
	assertRouteHandler(t, engine, http.MethodGet, "/api/user/search", controller.SearchUsers)
	assertRouteHandler(t, engine, http.MethodGet, "/api/user/:id/oauth/bindings", controller.GetUserOAuthBindingsByAdmin)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/user/:id/oauth/bindings/:provider_id", controller.UnbindCustomOAuthByAdmin)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/user/:id/bindings/:binding_type", controller.AdminClearUserBinding)
	assertRouteHandler(t, engine, http.MethodGet, "/api/user/:id", controller.GetUser)
	assertRouteHandler(t, engine, http.MethodPost, "/api/user/", controller.CreateUser)
	assertRouteHandler(t, engine, http.MethodPost, "/api/user/manage", controller.ManageUser)
	assertRouteHandler(t, engine, http.MethodPut, "/api/user/", controller.UpdateUser)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/user/:id", controller.DeleteUser)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/user/:id/reset_passkey", controller.AdminResetPasskey)
	assertRouteHandler(t, engine, http.MethodGet, "/api/user/2fa/stats", controller.Admin2FAStats)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/user/:id/2fa", controller.AdminDisable2FA)
}

func TestUserAdminPermissionRoutesClassifyCoreActions(t *testing.T) {
	assertUserAdminPermissionRoute(t, http.MethodGet, "/", authz.UserRead)
	assertUserAdminPermissionRoute(t, http.MethodGet, "/topup", authz.UserRead)
	assertUserAdminPermissionRoute(t, http.MethodGet, "/search", authz.UserRead)
	assertUserAdminPermissionRoute(t, http.MethodGet, "/:id/oauth/bindings", authz.UserRead)
	assertUserAdminPermissionRoute(t, http.MethodGet, "/:id", authz.UserRead)
	assertUserAdminPermissionRoute(t, http.MethodGet, "/2fa/stats", authz.UserRead)

	assertUserAdminPermissionRoute(t, http.MethodDelete, "/:id/oauth/bindings/:provider_id", authz.UserOperate)
	assertUserAdminPermissionRoute(t, http.MethodDelete, "/:id/bindings/:binding_type", authz.UserOperate)
	assertUserAdminPermissionRoute(t, http.MethodPost, "/manage", authz.UserOperate)
	assertUserAdminPermissionRoute(t, http.MethodDelete, "/:id/reset_passkey", authz.UserOperate)
	assertUserAdminPermissionRoute(t, http.MethodDelete, "/:id/2fa", authz.UserOperate)

	assertUserAdminPermissionRoute(t, http.MethodPut, "/", authz.UserWrite)

	assertUserAdminPermissionRoute(t, http.MethodPost, "/topup/complete", authz.UserSensitiveWrite)
	assertUserAdminPermissionRoute(t, http.MethodPost, "/", authz.UserSensitiveWrite)
	assertUserAdminPermissionRoute(t, http.MethodDelete, "/:id", authz.UserSensitiveWrite)
}

func TestUserAdminPermissionRoutesStayOnUserResource(t *testing.T) {
	require.NotEmpty(t, userAdminPermissionRoutes)
	for _, route := range userAdminPermissionRoutes {
		assert.NotEmpty(t, route.method)
		assert.NotEmpty(t, route.path)
		assert.NotNil(t, route.handler)
		assert.Equal(t, authz.ResourceUser, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func assertUserAdminPermissionRoute(t *testing.T, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requireUserAdminPermissionRoute(t, method, path)
	assert.Equal(t, permission, route.permission)
}

func requireUserAdminPermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range userAdminPermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("user admin permission route %s %s not found", method, path)
	return permissionRoute{}
}
