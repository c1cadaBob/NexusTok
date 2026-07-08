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

func TestRegisterLogDataAdminRoutesKeepsCoreHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerLogDataAdminRoutes(api)
	})

	assertRouteHandler(t, engine, http.MethodGet, "/api/log/", controller.GetAllLogs)
	assertRouteHandler(t, engine, http.MethodGet, "/api/log/stat", controller.GetLogsStat)
	assertRouteHandler(t, engine, http.MethodGet, "/api/log/search", controller.SearchAllLogs)
	assertRouteHandler(t, engine, http.MethodGet, "/api/log/channel_affinity_usage_cache", controller.GetChannelAffinityUsageCacheStats)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/log/", controller.DeleteHistoryLogs)
	assertRouteHandler(t, engine, http.MethodGet, "/api/data/", controller.GetAllQuotaDates)
	assertRouteHandler(t, engine, http.MethodGet, "/api/data/users", controller.GetQuotaDatesByUser)
	assertRouteHandler(t, engine, http.MethodGet, "/api/data/flow", controller.GetAllFlowQuotaDates)
}

func TestLogDataPermissionRoutesClassifyCoreActions(t *testing.T) {
	assertLogPermissionRoute(t, http.MethodGet, "/", authz.UsageLogRead)
	assertLogPermissionRoute(t, http.MethodGet, "/stat", authz.UsageLogRead)
	assertLogPermissionRoute(t, http.MethodGet, "/search", authz.UsageLogRead)
	assertLogPermissionRoute(t, http.MethodGet, "/channel_affinity_usage_cache", authz.UsageLogRead)
	assertLogPermissionRoute(t, http.MethodDelete, "/", authz.UsageLogSensitiveWrite)

	assertDataPermissionRoute(t, http.MethodGet, "/", authz.UsageDataRead)
	assertDataPermissionRoute(t, http.MethodGet, "/users", authz.UsageDataRead)
	assertDataPermissionRoute(t, http.MethodGet, "/flow", authz.UsageDataRead)
}

func TestLogDataPermissionRoutesStayOnDedicatedResources(t *testing.T) {
	require.NotEmpty(t, logPermissionRoutes)
	for _, route := range logPermissionRoutes {
		assert.Equal(t, authz.ResourceUsageLog, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}

	require.NotEmpty(t, dataPermissionRoutes)
	for _, route := range dataPermissionRoutes {
		assert.Equal(t, authz.ResourceUsageData, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func TestLogDataPermissionRoutesDoNotCaptureSelfOrTokenPaths(t *testing.T) {
	for _, route := range logPermissionRoutes {
		assert.NotContains(t, route.path, "/self", "%s %s", route.method, route.path)
		assert.NotEqual(t, "/token", route.path, "%s %s", route.method, route.path)
	}
	for _, route := range dataPermissionRoutes {
		assert.NotContains(t, route.path, "/self", "%s %s", route.method, route.path)
	}
}

func assertLogPermissionRoute(t *testing.T, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requireLogPermissionRoute(t, method, path)
	assert.Equal(t, permission, route.permission)
}

func requireLogPermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range logPermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("log permission route %s %s not found", method, path)
	return permissionRoute{}
}

func assertDataPermissionRoute(t *testing.T, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requireDataPermissionRoute(t, method, path)
	assert.Equal(t, permission, route.permission)
}

func requireDataPermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range dataPermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("data permission route %s %s not found", method, path)
	return permissionRoute{}
}
