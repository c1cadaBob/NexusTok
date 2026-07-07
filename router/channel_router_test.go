package router

import (
	"net/http"
	"reflect"
	"runtime"
	"testing"

	"github.com/c1cada/NexusTok/controller"

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

func assertRouteHandler(t *testing.T, engine *gin.Engine, method string, path string, handler gin.HandlerFunc) {
	t.Helper()
	route := requireRoute(t, engine, method, path)
	assert.Equal(t, handlerName(handler), route.Handler)
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
