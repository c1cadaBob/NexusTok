package router

import (
	"net/http"
	"reflect"
	"runtime"
	"testing"

	"github.com/c1cada/NexusTok/controller"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterChannelRoutesKeepsCoreHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerChannelRoutes(api)
	})

	assertRouteHandler(t, engine, http.MethodGet, "/api/channel/", controller.GetAllChannels)
	assertRouteHandler(t, engine, http.MethodPut, "/api/channel/", controller.UpdateChannel)
	assertRouteHandler(t, engine, http.MethodPost, "/api/channel/", controller.AddChannel)
	assertRouteHandler(t, engine, http.MethodGet, "/api/channel/:id/accounts", controller.ListChannelAccounts)
	assertRouteHandler(t, engine, http.MethodPost, "/api/channel/:id/codex/refresh", controller.RefreshCodexChannelCredential)
	assertRouteHandler(t, engine, http.MethodGet, "/api/channel/:id/codex/usage/reset-credits", controller.GetCodexChannelRateLimitResetCredits)
	assertRouteHandler(t, engine, http.MethodPost, "/api/channel/:id/codex/usage/reset", controller.ResetCodexChannelUsage)
	assertRouteHandler(t, engine, http.MethodPost, "/api/channel/upstream_updates/apply", controller.ApplyChannelUpstreamModelUpdates)
}

func TestRegisterChannelRoutesKeepsRootProtectedHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")
	registerChannelRoutes(api)

	keyRoute := requireRoute(t, engine, http.MethodPost, "/api/channel/:id/key")
	assert.Equal(t, handlerName(controller.GetChannelKey), keyRoute.Handler)

	fetchModelsRoute := requireRoute(t, engine, http.MethodPost, "/api/channel/fetch_models")
	assert.Equal(t, handlerName(controller.FetchModels), fetchModelsRoute.Handler)
}

func TestChannelPermissionRoutesClassifyCoreActions(t *testing.T) {
	assertPermissionRoute(t, http.MethodGet, "/", authz.ChannelRead)
	assertPermissionRoute(t, http.MethodGet, "/models", authz.ChannelRead)
	assertPermissionRoute(t, http.MethodGet, "/test/:id", authz.ChannelOperate)
	assertPermissionRoute(t, http.MethodGet, "/update_balance/:id", authz.ChannelOperate)
	assertPermissionRoute(t, http.MethodPut, "/", authz.ChannelWrite)
	assertPermissionRoute(t, http.MethodPut, "/tag", authz.ChannelWrite)
	assertPermissionRoute(t, http.MethodPost, "/", authz.ChannelSensitiveWrite)
	assertPermissionRoute(t, http.MethodDelete, "/:id", authz.ChannelSensitiveWrite)
	assertPermissionRoute(t, http.MethodPost, "/:id/key", authz.ChannelSecretView)
}

func TestChannelPermissionRoutesClassifyChannelAccountActions(t *testing.T) {
	assertPermissionRoute(t, http.MethodGet, "/:id/accounts", authz.ChannelAccountRead)
	assertPermissionRoute(t, http.MethodGet, "/:id/accounts/:account_id", authz.ChannelAccountRead)
	assertPermissionRoute(t, http.MethodPost, "/:id/accounts/:account_id/status", authz.ChannelAccountOperate)
	assertPermissionRoute(t, http.MethodPost, "/:id/accounts", authz.ChannelAccountSensitiveWrite)
	assertPermissionRoute(t, http.MethodPost, "/:id/accounts/batch", authz.ChannelAccountSensitiveWrite)
	assertPermissionRoute(t, http.MethodPost, "/:id/accounts/import-multikey", authz.ChannelAccountSensitiveWrite)
	assertPermissionRoute(t, http.MethodPut, "/:id/accounts/:account_id", authz.ChannelAccountWrite)
	assertPermissionRoute(t, http.MethodDelete, "/:id/accounts/:account_id", authz.ChannelAccountSensitiveWrite)
}

func TestChannelPermissionRoutesKeepLegacySensitiveMiddlewares(t *testing.T) {
	keyRoute := requirePermissionRoute(t, http.MethodPost, "/:id/key")
	require.NotEmpty(t, keyRoute.before)
	require.Len(t, keyRoute.after, 3)

	fetchModelsRoute := requirePermissionRoute(t, http.MethodPost, "/fetch_models")
	require.NotEmpty(t, fetchModelsRoute.before)
	require.Empty(t, fetchModelsRoute.after)
}

func assertRouteHandler(t *testing.T, engine *gin.Engine, method string, path string, handler gin.HandlerFunc) {
	t.Helper()
	route := requireRoute(t, engine, method, path)
	assert.Equal(t, handlerName(handler), route.Handler)
}

func assertPermissionRoute(t *testing.T, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requirePermissionRoute(t, method, path)
	assert.Equal(t, permission, route.permission)
}

func requirePermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range channelPermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("permission route %s %s not found", method, path)
	return permissionRoute{}
}

func requireRoute(t *testing.T, engine *gin.Engine, method string, path string) gin.RouteInfo {
	t.Helper()
	for _, route := range engine.Routes() {
		if route.Method == method && route.Path == path {
			return route
		}
	}
	t.Fatalf("route %s %s not found", method, path)
	return gin.RouteInfo{}
}

func handlerName(handler gin.HandlerFunc) string {
	return runtimeFuncName(reflect.ValueOf(handler).Pointer())
}

func runtimeFuncName(pointer uintptr) string {
	fn := runtime.FuncForPC(pointer)
	if fn == nil {
		return ""
	}
	return fn.Name()
}
