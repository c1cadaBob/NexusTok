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

func TestRegisterAuthzRoutesKeepsCatalogHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	registerAuthzRoutes(api)

	assertRouteHandler(t, engine, http.MethodGet, "/api/authz/catalog", controller.GetPermissionCatalog)
	assertRouteHandler(t, engine, http.MethodGet, "/api/authz/policies/export", controller.ExportPermissionPolicies)
}

func TestAuthzPermissionRoutesClassifyCatalog(t *testing.T) {
	route := requireAuthzPermissionRoute(t, http.MethodGet, "/catalog")
	assert.Equal(t, authz.SystemSettingRead, route.permission)
	assert.Empty(t, route.before)
	assert.Empty(t, route.after)

	exportRoute := requireAuthzPermissionRoute(t, http.MethodGet, "/policies/export")
	assert.Equal(t, authz.SystemSettingSecretView, exportRoute.permission)
	assert.Len(t, exportRoute.before, 1)
	assert.Empty(t, exportRoute.after)
}

func TestAuthzPermissionRoutesStayOnSystemSettingResource(t *testing.T) {
	require.NotEmpty(t, authzPermissionRoutes)
	for _, route := range authzPermissionRoutes {
		assert.Equal(t, authz.ResourceSystemSetting, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func requireAuthzPermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range authzPermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("authz permission route %s %s not found", method, path)
	return permissionRoute{}
}
