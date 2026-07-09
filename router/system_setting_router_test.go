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

func TestRegisterSystemSettingRoutesKeepsCoreHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerSystemSettingRoutes(api)
	})

	assertRouteHandler(t, engine, http.MethodGet, "/api/status/test", controller.TestStatus)

	assertRouteHandler(t, engine, http.MethodGet, "/api/option/", controller.GetOptions)
	assertRouteHandler(t, engine, http.MethodPut, "/api/option/", controller.UpdateOption)
	assertRouteHandler(t, engine, http.MethodPost, "/api/option/payment_compliance", controller.ConfirmPaymentCompliance)
	assertRouteHandler(t, engine, http.MethodPost, "/api/option/waffo-pancake/save", controller.SaveWaffoPancakeConfig)
	assertRouteHandler(t, engine, http.MethodGet, "/api/option/channel_affinity_cache", controller.GetChannelAffinityCacheStats)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/option/channel_affinity_cache", controller.ClearChannelAffinityCache)
	assertRouteHandler(t, engine, http.MethodPost, "/api/option/rest_model_ratio", controller.ResetModelRatio)
	assertRouteHandler(t, engine, http.MethodPost, "/api/option/migrate_console_setting", controller.MigrateConsoleSetting)

	assertRouteHandler(t, engine, http.MethodPost, "/api/custom-oauth-provider/discovery", controller.FetchCustomOAuthDiscovery)
	assertRouteHandler(t, engine, http.MethodGet, "/api/custom-oauth-provider/", controller.GetCustomOAuthProviders)
	assertRouteHandler(t, engine, http.MethodGet, "/api/custom-oauth-provider/:id", controller.GetCustomOAuthProvider)
	assertRouteHandler(t, engine, http.MethodPost, "/api/custom-oauth-provider/", controller.CreateCustomOAuthProvider)
	assertRouteHandler(t, engine, http.MethodPut, "/api/custom-oauth-provider/:id", controller.UpdateCustomOAuthProvider)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/custom-oauth-provider/:id", controller.DeleteCustomOAuthProvider)

	assertRouteHandler(t, engine, http.MethodGet, "/api/performance/stats", controller.GetPerformanceStats)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/performance/disk_cache", controller.ClearDiskCache)
	assertRouteHandler(t, engine, http.MethodPost, "/api/performance/reset_stats", controller.ResetPerformanceStats)
	assertRouteHandler(t, engine, http.MethodPost, "/api/performance/gc", controller.ForceGC)
	assertRouteHandler(t, engine, http.MethodGet, "/api/performance/logs", controller.GetLogFiles)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/performance/logs", controller.CleanupLogFiles)

	assertRouteHandler(t, engine, http.MethodGet, "/api/ratio_sync/channels", controller.GetSyncableChannels)
	assertRouteHandler(t, engine, http.MethodPost, "/api/ratio_sync/fetch", controller.FetchUpstreamRatios)
}

func TestSystemSettingPermissionRoutesClassifyCoreActions(t *testing.T) {
	assertSystemSettingPermissionRoute(t, statusDiagnosticPermissionRoutes, http.MethodGet, "/test", authz.SystemSettingRead)

	assertSystemSettingPermissionRoute(t, optionPermissionRoutes, http.MethodGet, "/", authz.SystemSettingRead)
	assertSystemSettingPermissionRoute(t, optionPermissionRoutes, http.MethodPut, "/", authz.SystemSettingSensitiveWrite)
	assertSystemSettingPermissionRoute(t, optionPermissionRoutes, http.MethodPost, "/payment_compliance", authz.SystemSettingSensitiveWrite)
	assertSystemSettingPermissionRoute(t, optionPermissionRoutes, http.MethodPost, "/waffo-pancake/save", authz.SystemSettingSensitiveWrite)
	assertSystemSettingPermissionRoute(t, optionPermissionRoutes, http.MethodGet, "/channel_affinity_cache", authz.SystemSettingRead)
	assertSystemSettingPermissionRoute(t, optionPermissionRoutes, http.MethodDelete, "/channel_affinity_cache", authz.SystemSettingOperate)
	assertSystemSettingPermissionRoute(t, optionPermissionRoutes, http.MethodPost, "/rest_model_ratio", authz.SystemSettingSensitiveWrite)
	assertSystemSettingPermissionRoute(t, optionPermissionRoutes, http.MethodPost, "/migrate_console_setting", authz.SystemSettingSensitiveWrite)

	assertSystemSettingPermissionRoute(t, customOAuthProviderPermissionRoutes, http.MethodPost, "/discovery", authz.SystemSettingOperate)
	assertSystemSettingPermissionRoute(t, customOAuthProviderPermissionRoutes, http.MethodGet, "/", authz.SystemSettingRead)
	assertSystemSettingPermissionRoute(t, customOAuthProviderPermissionRoutes, http.MethodGet, "/:id", authz.SystemSettingRead)
	assertSystemSettingPermissionRoute(t, customOAuthProviderPermissionRoutes, http.MethodPost, "/", authz.SystemSettingSensitiveWrite)
	assertSystemSettingPermissionRoute(t, customOAuthProviderPermissionRoutes, http.MethodPut, "/:id", authz.SystemSettingSensitiveWrite)
	assertSystemSettingPermissionRoute(t, customOAuthProviderPermissionRoutes, http.MethodDelete, "/:id", authz.SystemSettingSensitiveWrite)

	assertSystemSettingPermissionRoute(t, performancePermissionRoutes, http.MethodGet, "/stats", authz.SystemSettingRead)
	assertSystemSettingPermissionRoute(t, performancePermissionRoutes, http.MethodDelete, "/disk_cache", authz.SystemSettingOperate)
	assertSystemSettingPermissionRoute(t, performancePermissionRoutes, http.MethodPost, "/reset_stats", authz.SystemSettingOperate)
	assertSystemSettingPermissionRoute(t, performancePermissionRoutes, http.MethodPost, "/gc", authz.SystemSettingOperate)
	assertSystemSettingPermissionRoute(t, performancePermissionRoutes, http.MethodGet, "/logs", authz.SystemSettingRead)
	assertSystemSettingPermissionRoute(t, performancePermissionRoutes, http.MethodDelete, "/logs", authz.SystemSettingSensitiveWrite)

	assertSystemSettingPermissionRoute(t, ratioSyncPermissionRoutes, http.MethodGet, "/channels", authz.SystemSettingRead)
	assertSystemSettingPermissionRoute(t, ratioSyncPermissionRoutes, http.MethodPost, "/fetch", authz.SystemSettingOperate)
}

func TestSystemSettingPermissionRoutesStayOnSystemSettingResource(t *testing.T) {
	routes := append([]permissionRoute{}, statusDiagnosticPermissionRoutes...)
	routes = append(routes, optionPermissionRoutes...)
	routes = append(routes, customOAuthProviderPermissionRoutes...)
	routes = append(routes, performancePermissionRoutes...)
	routes = append(routes, ratioSyncPermissionRoutes...)

	require.NotEmpty(t, routes)
	for _, route := range routes {
		assert.NotEmpty(t, route.method)
		assert.NotEmpty(t, route.path)
		assert.NotNil(t, route.handler)
		assert.Equal(t, authz.ResourceSystemSetting, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func assertSystemSettingPermissionRoute(t *testing.T, routes []permissionRoute, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requireSystemSettingPermissionRoute(t, routes, method, path)
	assert.Equal(t, permission, route.permission)
}

func requireSystemSettingPermissionRoute(t *testing.T, routes []permissionRoute, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range routes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("system setting permission route %s %s not found", method, path)
	return permissionRoute{}
}
