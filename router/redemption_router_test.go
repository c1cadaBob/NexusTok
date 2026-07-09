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

func TestRegisterRedemptionRoutesKeepsCoreHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerRedemptionRoutes(api)
	})

	assertRouteHandler(t, engine, http.MethodGet, "/api/redemption/", controller.GetAllRedemptions)
	assertRouteHandler(t, engine, http.MethodGet, "/api/redemption/search", controller.SearchRedemptions)
	assertRouteHandler(t, engine, http.MethodGet, "/api/redemption/:id", controller.GetRedemption)
	assertRouteHandler(t, engine, http.MethodPost, "/api/redemption/:id/key", controller.GetRedemptionKey)
	assertRouteHandler(t, engine, http.MethodPost, "/api/redemption/", controller.AddRedemption)
	assertRouteHandler(t, engine, http.MethodPut, "/api/redemption/", controller.UpdateRedemption)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/redemption/invalid", controller.DeleteInvalidRedemption)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/redemption/:id", controller.DeleteRedemption)
}

func TestRedemptionPermissionRoutesClassifyCoreActions(t *testing.T) {
	assertRedemptionPermissionRoute(t, http.MethodGet, "/", authz.RedemptionRead)
	assertRedemptionPermissionRoute(t, http.MethodGet, "/search", authz.RedemptionRead)
	assertRedemptionPermissionRoute(t, http.MethodGet, "/:id", authz.RedemptionRead)
	assertRedemptionPermissionRoute(t, http.MethodPost, "/:id/key", authz.RedemptionSecretView)
	assertRedemptionPermissionRoute(t, http.MethodPost, "/", authz.RedemptionWrite)
	assertRedemptionPermissionRoute(t, http.MethodPut, "/", authz.RedemptionWrite)
	assertRedemptionPermissionRoute(t, http.MethodDelete, "/invalid", authz.RedemptionSensitiveWrite)
	assertRedemptionPermissionRoute(t, http.MethodDelete, "/:id", authz.RedemptionSensitiveWrite)
}

func TestRedemptionPermissionRoutesStayOnRedemptionResource(t *testing.T) {
	require.NotEmpty(t, redemptionPermissionRoutes)
	for _, route := range redemptionPermissionRoutes {
		assert.NotEmpty(t, route.method)
		assert.NotEmpty(t, route.path)
		assert.NotNil(t, route.handler)
		assert.Equal(t, authz.ResourceRedemption, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func TestRedemptionKeyRevealRequiresSensitiveMiddlewares(t *testing.T) {
	keyRoute := requireRedemptionPermissionRoute(t, http.MethodPost, "/:id/key")
	require.Empty(t, keyRoute.before)
	require.Len(t, keyRoute.after, 3)
	assert.Equal(t, authz.RedemptionSecretView, keyRoute.permission)
}

func assertRedemptionPermissionRoute(t *testing.T, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requireRedemptionPermissionRoute(t, method, path)
	assert.Equal(t, permission, route.permission)
}

func requireRedemptionPermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range redemptionPermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("redemption permission route %s %s not found", method, path)
	return permissionRoute{}
}
