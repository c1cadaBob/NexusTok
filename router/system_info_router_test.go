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

func TestRegisterSystemInfoRoutesKeepsInstancesHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	registerSystemInfoRoutes(api)

	assertRouteHandler(t, engine, http.MethodGet, "/api/system-info/instances", controller.ListSystemInstances)
}

func TestSystemInfoPermissionRoutesClassifyInstances(t *testing.T) {
	route := requireSystemInfoPermissionRoute(t, http.MethodGet, "/instances")
	assert.Equal(t, authz.SystemSettingRead, route.permission)
	assert.Empty(t, route.before)
	assert.Empty(t, route.after)
}

func TestSystemInfoPermissionRoutesStayOnSystemSettingResource(t *testing.T) {
	require.NotEmpty(t, systemInfoPermissionRoutes)
	for _, route := range systemInfoPermissionRoutes {
		assert.Equal(t, authz.ResourceSystemSetting, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func requireSystemInfoPermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range systemInfoPermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("system info permission route %s %s not found", method, path)
	return permissionRoute{}
}
