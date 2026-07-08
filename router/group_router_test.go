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

func TestRegisterGroupRoutesKeepsCoreHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerGroupRoutes(api)
	})

	assertRouteHandler(t, engine, http.MethodGet, "/api/group/", controller.GetGroups)
	assertRouteHandler(t, engine, http.MethodGet, "/api/prefill_group/", controller.GetPrefillGroups)
	assertRouteHandler(t, engine, http.MethodPost, "/api/prefill_group/", controller.CreatePrefillGroup)
	assertRouteHandler(t, engine, http.MethodPut, "/api/prefill_group/", controller.UpdatePrefillGroup)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/prefill_group/:id", controller.DeletePrefillGroup)
}

func TestGroupPermissionRoutesClassifyCoreActions(t *testing.T) {
	assertGroupPermissionRoute(t, http.MethodGet, "/", authz.ModelRead)

	assertPrefillGroupPermissionRoute(t, http.MethodGet, "/", authz.ModelRead)
	assertPrefillGroupPermissionRoute(t, http.MethodPost, "/", authz.ModelWrite)
	assertPrefillGroupPermissionRoute(t, http.MethodPut, "/", authz.ModelWrite)
	assertPrefillGroupPermissionRoute(t, http.MethodDelete, "/:id", authz.ModelSensitiveWrite)
}

func TestGroupPermissionRoutesStayOnModelResource(t *testing.T) {
	routes := append([]permissionRoute{}, groupPermissionRoutes...)
	routes = append(routes, prefillGroupPermissionRoutes...)

	require.NotEmpty(t, routes)
	for _, route := range routes {
		assert.NotEmpty(t, route.method)
		assert.NotEmpty(t, route.path)
		assert.NotNil(t, route.handler)
		assert.Equal(t, authz.ResourceModel, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func assertGroupPermissionRoute(t *testing.T, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requireGroupPermissionRoute(t, groupPermissionRoutes, method, path)
	assert.Equal(t, permission, route.permission)
}

func assertPrefillGroupPermissionRoute(t *testing.T, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requireGroupPermissionRoute(t, prefillGroupPermissionRoutes, method, path)
	assert.Equal(t, permission, route.permission)
}

func requireGroupPermissionRoute(t *testing.T, routes []permissionRoute, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range routes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("group permission route %s %s not found", method, path)
	return permissionRoute{}
}
