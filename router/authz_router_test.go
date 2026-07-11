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
	assertRouteHandler(t, engine, http.MethodGet, "/api/authz/roles", controller.ListPermissionRoles)
	assertRouteHandler(t, engine, http.MethodPut, "/api/authz/roles/:key/policies", controller.UpdatePermissionRolePolicies)
	assertRouteHandler(t, engine, http.MethodPost, "/api/authz/policies/import", controller.ImportPermissionPolicies)
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

	rolesRoute := requireAuthzPermissionRoute(t, http.MethodGet, "/roles")
	assert.Equal(t, authz.SystemSettingSecretView, rolesRoute.permission)
	assert.Len(t, rolesRoute.before, 1)
	assert.Empty(t, rolesRoute.after)

	updateRoleRoute := requireAuthzPermissionRoute(t, http.MethodPut, "/roles/:key/policies")
	assert.Equal(t, authz.SystemSettingSensitiveWrite, updateRoleRoute.permission)
	assert.Len(t, updateRoleRoute.before, 1)
	assert.Empty(t, updateRoleRoute.after)

	importRoute := requireAuthzPermissionRoute(t, http.MethodPost, "/policies/import")
	assert.Equal(t, authz.SystemSettingSensitiveWrite, importRoute.permission)
	assert.Len(t, importRoute.before, 1)
	assert.Empty(t, importRoute.after)
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
