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

func TestRegisterAccountPoolRoutesKeepsCoreHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerAccountPoolRoutes(api)
	})

	assertRouteHandler(t, engine, http.MethodGet, "/api/account-pool/health", controller.GetAccountPoolHealth)
	assertRouteHandler(t, engine, http.MethodGet, "/api/account-pool/groups", controller.ListAccountPoolGroups)
	assertRouteHandler(t, engine, http.MethodGet, "/api/account-pool/groups/options", controller.ListAccountPoolGroupOptions)
	assertRouteHandler(t, engine, http.MethodGet, "/api/account-pool/groups/:id/accounts", controller.ListPoolAccounts)
	assertRouteHandler(t, engine, http.MethodGet, "/api/account-pool/auth-files/:auth_file_id", controller.GetAccountPoolAuthFile)
	assertRouteHandler(t, engine, http.MethodPost, "/api/account-pool/groups/:id/accounts/check-tasks", controller.StartPoolAccountCheckTask)
	assertRouteHandler(t, engine, http.MethodPost, "/api/account-pool/accounts/:account_id/refresh", controller.RefreshPoolAccountCredential)
	assertRouteHandler(t, engine, http.MethodPost, "/api/account-pool/oauth/codex/complete", controller.CompleteAccountPoolCodexOAuth)
}

func TestAccountPoolPermissionRoutesClassifyCoreActions(t *testing.T) {
	assertAccountPoolPermissionRoute(t, http.MethodGet, "/health", authz.AccountPoolRead)
	assertAccountPoolPermissionRoute(t, http.MethodGet, "/auth-files", authz.AccountPoolAuthFileRead)
	assertAccountPoolPermissionRoute(t, http.MethodPost, "/auth-files", authz.AccountPoolAuthFileSensitiveWrite)
	assertAccountPoolPermissionRoute(t, http.MethodPost, "/auth-files/import", authz.AccountPoolAuthFileSensitiveWrite)
	assertAccountPoolPermissionRoute(t, http.MethodGet, "/auth-files/:auth_file_id", authz.AccountPoolAuthFileRead)
	assertAccountPoolPermissionRoute(t, http.MethodPut, "/auth-files/:auth_file_id", authz.AccountPoolAuthFileSensitiveWrite)
	assertAccountPoolPermissionRoute(t, http.MethodDelete, "/auth-files/:auth_file_id", authz.AccountPoolAuthFileSensitiveWrite)
	assertAccountPoolPermissionRoute(t, http.MethodGet, "/state-logs/export", authz.AccountPoolOperate)
	assertAccountPoolPermissionRoute(t, http.MethodPost, "/check-tasks/cleanup", authz.AccountPoolOperate)
	assertAccountPoolPermissionRoute(t, http.MethodPost, "/groups", authz.AccountPoolWrite)
	assertAccountPoolPermissionRoute(t, http.MethodDelete, "/groups/:id", authz.AccountPoolSensitiveWrite)
	assertAccountPoolPermissionRoute(t, http.MethodGet, "/groups/:id/accounts", authz.AccountPoolRead)
	assertAccountPoolPermissionRoute(t, http.MethodPost, "/groups/:id/accounts/status", authz.AccountPoolOperate)
	assertAccountPoolPermissionRoute(t, http.MethodPost, "/groups/:id/accounts/export", authz.AccountPoolOperate)
	assertAccountPoolPermissionRoute(t, http.MethodPost, "/groups/:id/accounts/delete", authz.AccountPoolSensitiveWrite)
	assertAccountPoolPermissionRoute(t, http.MethodPut, "/accounts/:account_id", authz.AccountPoolSensitiveWrite)
	assertAccountPoolPermissionRoute(t, http.MethodPost, "/accounts/:account_id/runtime/reset", authz.AccountPoolOperate)
	assertAccountPoolPermissionRoute(t, http.MethodPost, "/oauth/codex/complete", authz.AccountPoolSensitiveWrite)
}

func TestAccountPoolPermissionRoutesUseExpectedResources(t *testing.T) {
	require.NotEmpty(t, accountPoolPermissionRoutes)
	for _, route := range accountPoolPermissionRoutes {
		assert.NotEmpty(t, route.method)
		assert.NotEmpty(t, route.path)
		assert.NotNil(t, route.handler)
		expectedResource := authz.ResourceAccountPool
		if isAccountPoolAuthFileRoute(route.path) {
			expectedResource = authz.ResourceAccountPoolAuthFile
		}
		assert.Equal(t, expectedResource, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func isAccountPoolAuthFileRoute(path string) bool {
	return path == "/auth-files" ||
		path == "/auth-files/import" ||
		path == "/auth-files/:auth_file_id"
}

func assertAccountPoolPermissionRoute(t *testing.T, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requireAccountPoolPermissionRoute(t, method, path)
	assert.Equal(t, permission, route.permission)
}

func requireAccountPoolPermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range accountPoolPermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("account pool permission route %s %s not found", method, path)
	return permissionRoute{}
}
