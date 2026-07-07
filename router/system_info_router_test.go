package router

import (
	"net/http"
	"testing"

	"github.com/c1cada/NexusTok/controller"

	"github.com/gin-gonic/gin"
)

func TestRegisterSystemInfoRoutesKeepsInstancesHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	registerSystemInfoRoutes(api)

	assertRouteHandler(t, engine, http.MethodGet, "/api/system-info/instances", controller.ListSystemInstances)
}
