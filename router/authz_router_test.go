package router

import (
	"net/http"
	"testing"

	"github.com/c1cada/NexusTok/controller"
	"github.com/gin-gonic/gin"
)

func TestRegisterAuthzRoutesKeepsCatalogHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	registerAuthzRoutes(api)

	assertRouteHandler(t, engine, http.MethodGet, "/api/authz/catalog", controller.GetPermissionCatalog)
}
