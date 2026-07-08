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

func TestRegisterTaskLogRoutesKeepsCoreHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerTaskLogRoutes(api)
	})

	assertRouteHandler(t, engine, http.MethodGet, "/api/mj/self", controller.GetUserMidjourney)
	assertRouteHandler(t, engine, http.MethodGet, "/api/mj/", controller.GetAllMidjourney)
	assertRouteHandler(t, engine, http.MethodGet, "/api/task/self", controller.GetUserTask)
	assertRouteHandler(t, engine, http.MethodGet, "/api/task/", controller.GetAllTask)
}

func TestTaskLogPermissionRoutesClassifyAdminReads(t *testing.T) {
	assertMidjourneyLogPermissionRoute(t, http.MethodGet, "/", authz.UsageLogRead)
	assertAsyncTaskLogPermissionRoute(t, http.MethodGet, "/", authz.UsageLogRead)
}

func TestTaskLogPermissionRoutesStayOnUsageLogResource(t *testing.T) {
	require.NotEmpty(t, midjourneyLogPermissionRoutes)
	for _, route := range midjourneyLogPermissionRoutes {
		assert.Equal(t, authz.ResourceUsageLog, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}

	require.NotEmpty(t, asyncTaskLogPermissionRoutes)
	for _, route := range asyncTaskLogPermissionRoutes {
		assert.Equal(t, authz.ResourceUsageLog, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func TestTaskLogPermissionRoutesDoNotCaptureSelfPaths(t *testing.T) {
	for _, route := range midjourneyLogPermissionRoutes {
		assert.NotContains(t, route.path, "/self", "%s %s", route.method, route.path)
	}
	for _, route := range asyncTaskLogPermissionRoutes {
		assert.NotContains(t, route.path, "/self", "%s %s", route.method, route.path)
	}
}

func assertMidjourneyLogPermissionRoute(t *testing.T, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requireMidjourneyLogPermissionRoute(t, method, path)
	assert.Equal(t, permission, route.permission)
}

func requireMidjourneyLogPermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range midjourneyLogPermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("midjourney log permission route %s %s not found", method, path)
	return permissionRoute{}
}

func assertAsyncTaskLogPermissionRoute(t *testing.T, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requireAsyncTaskLogPermissionRoute(t, method, path)
	assert.Equal(t, permission, route.permission)
}

func requireAsyncTaskLogPermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range asyncTaskLogPermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("async task log permission route %s %s not found", method, path)
	return permissionRoute{}
}
