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

func TestRegisterModelRoutesKeepsCoreHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	require.NotPanics(t, func() {
		registerModelRoutes(api)
	})

	assertRouteHandler(t, engine, http.MethodGet, "/api/vendors/", controller.GetAllVendors)
	assertRouteHandler(t, engine, http.MethodGet, "/api/vendors/:id", controller.GetVendorMeta)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/vendors/:id", controller.DeleteVendorMeta)

	assertRouteHandler(t, engine, http.MethodGet, "/api/models/", controller.GetAllModelsMeta)
	assertRouteHandler(t, engine, http.MethodPost, "/api/models/sync_upstream", controller.SyncUpstreamModels)
	assertRouteHandler(t, engine, http.MethodGet, "/api/models/:id/pricing", controller.GetModelPricingConfig)
	assertRouteHandler(t, engine, http.MethodPut, "/api/models/:id/pricing", controller.UpdateModelPricingConfig)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/models/:id", controller.DeleteModelMeta)

	assertRouteHandler(t, engine, http.MethodGet, "/api/deployments/", controller.GetAllDeployments)
	assertRouteHandler(t, engine, http.MethodPost, "/api/deployments/test-connection", controller.TestIoNetConnection)
	assertRouteHandler(t, engine, http.MethodGet, "/api/deployments/:id/containers/:container_id", controller.GetContainerDetails)
	assertRouteHandler(t, engine, http.MethodPost, "/api/deployments/:id/extend", controller.ExtendDeployment)
	assertRouteHandler(t, engine, http.MethodDelete, "/api/deployments/:id", controller.DeleteDeployment)
}

func TestModelPermissionRoutesClassifyVendorActions(t *testing.T) {
	assertModelPermissionRoute(t, vendorPermissionRoutes, http.MethodGet, "/", authz.ModelRead)
	assertModelPermissionRoute(t, vendorPermissionRoutes, http.MethodGet, "/search", authz.ModelRead)
	assertModelPermissionRoute(t, vendorPermissionRoutes, http.MethodGet, "/:id", authz.ModelRead)
	assertModelPermissionRoute(t, vendorPermissionRoutes, http.MethodPost, "/", authz.ModelWrite)
	assertModelPermissionRoute(t, vendorPermissionRoutes, http.MethodPut, "/", authz.ModelWrite)
	assertModelPermissionRoute(t, vendorPermissionRoutes, http.MethodDelete, "/:id", authz.ModelSensitiveWrite)
}

func TestModelPermissionRoutesClassifyModelActions(t *testing.T) {
	assertModelPermissionRoute(t, modelPermissionRoutes, http.MethodGet, "/sync_upstream/preview", authz.ModelOperate)
	assertModelPermissionRoute(t, modelPermissionRoutes, http.MethodPost, "/sync_upstream", authz.ModelWrite)
	assertModelPermissionRoute(t, modelPermissionRoutes, http.MethodGet, "/missing", authz.ModelRead)
	assertModelPermissionRoute(t, modelPermissionRoutes, http.MethodGet, "/", authz.ModelRead)
	assertModelPermissionRoute(t, modelPermissionRoutes, http.MethodGet, "/search", authz.ModelRead)
	assertModelPermissionRoute(t, modelPermissionRoutes, http.MethodGet, "/:id/pricing", authz.ModelRead)
	assertModelPermissionRoute(t, modelPermissionRoutes, http.MethodPut, "/:id/pricing", authz.ModelWrite)
	assertModelPermissionRoute(t, modelPermissionRoutes, http.MethodPost, "/", authz.ModelWrite)
	assertModelPermissionRoute(t, modelPermissionRoutes, http.MethodPut, "/", authz.ModelWrite)
	assertModelPermissionRoute(t, modelPermissionRoutes, http.MethodDelete, "/:id", authz.ModelSensitiveWrite)
}

func TestModelPermissionRoutesClassifyDeploymentActions(t *testing.T) {
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodGet, "/settings", authz.ModelRead)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodPost, "/settings/test-connection", authz.ModelOperate)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodGet, "/", authz.ModelRead)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodGet, "/search", authz.ModelRead)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodPost, "/test-connection", authz.ModelOperate)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodGet, "/hardware-types", authz.ModelRead)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodGet, "/locations", authz.ModelRead)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodGet, "/available-replicas", authz.ModelRead)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodPost, "/price-estimation", authz.ModelOperate)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodGet, "/check-name", authz.ModelRead)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodPost, "/", authz.ModelWrite)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodGet, "/:id", authz.ModelRead)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodGet, "/:id/logs", authz.ModelRead)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodGet, "/:id/containers", authz.ModelRead)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodGet, "/:id/containers/:container_id", authz.ModelRead)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodPut, "/:id", authz.ModelWrite)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodPut, "/:id/name", authz.ModelWrite)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodPost, "/:id/extend", authz.ModelOperate)
	assertModelPermissionRoute(t, deploymentPermissionRoutes, http.MethodDelete, "/:id", authz.ModelSensitiveWrite)
}

func TestModelPermissionRoutesStayOnModelResource(t *testing.T) {
	routes := append([]permissionRoute{}, vendorPermissionRoutes...)
	routes = append(routes, modelPermissionRoutes...)
	routes = append(routes, deploymentPermissionRoutes...)

	require.NotEmpty(t, routes)
	for _, route := range routes {
		assert.NotEmpty(t, route.method)
		assert.NotEmpty(t, route.path)
		assert.NotNil(t, route.handler)
		assert.Equal(t, authz.ResourceModel, route.permission.Resource, "%s %s", route.method, route.path)
		assert.NotEmpty(t, route.permission.Action, "%s %s", route.method, route.path)
	}
}

func assertModelPermissionRoute(t *testing.T, routes []permissionRoute, method string, path string, permission authz.Permission) {
	t.Helper()
	route := requireModelPermissionRoute(t, routes, method, path)
	assert.Equal(t, permission, route.permission)
}

func requireModelPermissionRoute(t *testing.T, routes []permissionRoute, method string, path string) permissionRoute {
	t.Helper()
	for _, route := range routes {
		if route.method == method && route.path == path {
			return route
		}
	}
	t.Fatalf("model permission route %s %s not found", method, path)
	return permissionRoute{}
}
