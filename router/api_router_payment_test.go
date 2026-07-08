package router

import (
	"net/http"
	"testing"

	"github.com/c1cada/NexusTok/controller"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetApiRouterRegistersWaffoPancakeUserPaymentRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	require.NotPanics(t, func() {
		SetApiRouter(engine)
	})

	assertRouteHandler(t, engine, http.MethodPost, "/api/user/waffo-pancake/amount", controller.RequestWaffoPancakeAmount)
	assertRouteHandler(t, engine, http.MethodPost, "/api/user/waffo-pancake/pay", controller.RequestWaffoPancakePay)
}
