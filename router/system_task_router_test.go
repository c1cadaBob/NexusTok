package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/controller"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
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
