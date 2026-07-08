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

func TestRegisterSubscriptionAdminRoutesKeepsCoreHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerSubscriptionAdminRoutes(api)
	})

	assertRouteHandler(t, engine, http.MethodGet, "/api/subscription/admin/plans", controller.AdminListSubscriptionPlans)
	assertRouteHandler(t, engine, http.MethodPost, "/api/subscription/admin/plans", controller.AdminCreateSubscriptionPlan)
	assertRouteHandler(t, engine, http.MethodPut, "/api/subscription/admin/plans/:id", controller.AdminUpdateSubscriptionPlan)
	assertRouteHandler(t, engine, http.MethodPatch, "/api/subscription/admin/plans/:id", controller.AdminUpdateSubscriptionPlanStatus)
	assertRouteHandler(t, engine, http.MethodPost, "/api/subscription/admin/bind", controller.AdminBindSubscription)
	assertRouteHandler(t, engine, http.MethodGet, "/api/subscription/admin/users/:id/subscriptions", controller.AdminListUserSubscriptions)
	assertRouteHandler(t, engine, http.MethodPost, "/api/subscription/admin/users/:id/subscriptions", controller.AdminCreateUserSubscription)
	assertRouteHandler(t, engine, http.MethodPost, "/api/subscription/admin/user_subscriptions/:id/invalidate", controller.AdminInvalidateUserSubscription)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/subscription/admin/user_subscriptions/:id", controller.AdminDeleteUserSubscription)
}

func TestSubscriptionPermissionRoutesClassifyCoreActions(t *testing.T) {
	assertSubscriptionPermissionRoute(t, http.MethodGet, "/plans", authz.SubscriptionRead)
	assertSubscriptionPermissionRoute(t, http.MethodPost, "/plans", authz.SubscriptionWrite)
	assertSubscriptionPermissionRoute(t, http.MethodPut, "/plans/:id", authz.SubscriptionWrite)
	assertSubscriptionPermissionRoute(t, http.MethodPatch, "/plans/:id", authz.SubscriptionWrite)
	assertSubscriptionPermissionRoute(t, http.MethodPost, "/bind", authz.SubscriptionOperate)
	assertSubscriptionPermissionRoute(t, http.MethodGet, "/users/:id/subscriptions", authz.SubscriptionRead)
	assertSubscriptionPermissionRoute(t, http.MethodPost, "/users/:id/subscriptions", authz.SubscriptionOperate)
	assertSubscriptionPermissionRoute(t, http.MethodPost, "/user_subscriptions/:id/invalidate", authz.SubscriptionOperate)
	assertSubscriptionPermissionRoute(t, http.MethodDelete, "/user_subscriptions/:id", authz.SubscriptionSensitiveWrite)
}

func TestSubscriptionPermissionRoutesStayOnSubscriptionResource(t *testing.T) {
	require.NotEmpty(t, subscriptionPermissionRoutes)
	for _, route := range subscriptionPermissionRoutes {
		assert.NotEmpty(t, route.method)
		assert.NotEmpty(t, route.path)
		assert.NotNil(t, route.handler)
		assert.Equal(t, authz.ResourceSubscription, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func assertSubscriptionPermissionRoute(t *testing.T, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requireSubscriptionPermissionRoute(t, method, path)
	assert.Equal(t, permission, route.permission)
}

func requireSubscriptionPermissionRoute(t *testing.T, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range subscriptionPermissionRoutes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("subscription permission route %s %s not found", method, path)
	return permissionRoute{}
}
