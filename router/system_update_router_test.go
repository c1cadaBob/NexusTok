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

func TestRegisterSystemUpdateRoutesKeepsHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	registerSystemUpdateRoutes(api)

	assertRouteHandler(t, engine, http.MethodGet, "/api/system-update/latest", controller.GetLatestSystemUpdate)
	assertRouteHandler(t, engine, http.MethodPost, "/api/system-update/apply", controller.ApplySystemUpdate)
	assertRouteHandler(t, engine, http.MethodPost, "/api/system-update/rollback", controller.RollbackSystemUpdate)
	assertRouteHandler(t, engine, http.MethodPost, "/api/system-update/restart", controller.RestartSystemUpdate)
}

func TestSystemUpdateRoutesRequireRootAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("system-update-router-test"))))
	api := engine.Group("/api")
	registerSystemUpdateRoutes(api)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/system-update/latest", nil)
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestSystemUpdatePermissionRoutesClassifyActions(t *testing.T) {
	assertSystemUpdatePermissionRoute(t, http.MethodGet, "/latest", authz.SystemSettingRead)
	assertSystemUpdatePermissionRoute(t, http.MethodPost, "/apply", authz.SystemSettingSensitiveWrite)
	assertSystemUpdatePermissionRoute(t, http.MethodPost, "/rollback", authz.SystemSettingSensitiveWrite)
	assertSystemUpdatePermissionRoute(t, http.MethodPost, "/restart", authz.SystemSettingSensitiveWrite)
}

func TestSystemUpdatePermissionRoutesStayOnSystemSettingResource(t *testing.T) {
	require.NotEmpty(t, systemUpdatePermissionRoutes)
	for _, route := range systemUpdatePermissionRoutes {
		assert.Equal(t, authz.ResourceSystemSetting, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func assertSystemUpdatePermissionRoute(t *testing.T, method string, path string, permission authz.Permission) permissionRoute {
	t.Helper()
	route := requireSystemUpdatePermissionRoute(t, method, path)
	assert.Equal(t, permission, route.permission)
	return route
}

func requireSystemUpdatePermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range systemUpdatePermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("system update permission route %s %s not found", method, path)
	return permissionRoute{}
}
