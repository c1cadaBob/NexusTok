package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterSystemTaskRoutesKeepsHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	registerSystemTaskRoutes(api)

	assertRouteHandler(t, engine, http.MethodPost, "/api/system-task/log-cleanup", controller.CreateLogCleanupSystemTask)
	assertRouteHandler(t, engine, http.MethodGet, "/api/system-task/list", controller.ListSystemTasks)
	assertRouteHandler(t, engine, http.MethodGet, "/api/system-task/current", controller.GetCurrentSystemTask)
	assertRouteHandler(t, engine, http.MethodGet, "/api/system-task/:task_id", controller.GetSystemTask)
}

func TestSystemTaskRoutesRequireRootAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("system-task-router-test"))))
	api := engine.Group("/api")
	registerSystemTaskRoutes(api)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/system-task/list", nil)
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestSystemTaskPermissionRoutesClassifyCoreActions(t *testing.T) {
	logCleanupRoute := assertSystemTaskPermissionRoute(t, http.MethodPost, "/log-cleanup", authz.SystemSettingSensitiveWrite)
	require.Len(t, logCleanupRoute.after, 1)

	assertSystemTaskPermissionRoute(t, http.MethodGet, "/list", authz.SystemSettingRead)
	assertSystemTaskPermissionRoute(t, http.MethodGet, "/current", authz.SystemSettingRead)
	assertSystemTaskPermissionRoute(t, http.MethodGet, "/:task_id", authz.SystemSettingRead)
}

func TestSystemTaskPermissionRoutesStayOnSystemSettingResource(t *testing.T) {
	require.NotEmpty(t, systemTaskPermissionRoutes)
	for _, route := range systemTaskPermissionRoutes {
		assert.Equal(t, authz.ResourceSystemSetting, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func assertSystemTaskPermissionRoute(t *testing.T, method string, path string, permission authz.Permission) permissionRoute {
	t.Helper()
	route := requireSystemTaskPermissionRoute(t, method, path)
	assert.Equal(t, permission, route.permission)
	return route
}

func requireSystemTaskPermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range systemTaskPermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("system task permission route %s %s not found", method, path)
	return permissionRoute{}
}
