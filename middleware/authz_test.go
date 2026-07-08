package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequirePermissionAllowsGrantedRoles(t *testing.T) {
	recorder := performRequirePermissionRequest(common.RoleRootUser, authz.ChannelSensitiveWrite)
	require.Equal(t, http.StatusNoContent, recorder.Code)

	recorder = performRequirePermissionRequest(common.RoleAdminUser, authz.ChannelWrite)
	require.Equal(t, http.StatusNoContent, recorder.Code)
}

func TestRequirePermissionRejectsDeniedAndUnknownPermissions(t *testing.T) {
	recorder := performRequirePermissionRequest(common.RoleAdminUser, authz.ChannelSensitiveWrite)
	require.Equal(t, http.StatusForbidden, recorder.Code)

	recorder = performRequirePermissionRequest(common.RoleCommonUser, authz.ChannelRead)
	require.Equal(t, http.StatusForbidden, recorder.Code)

	recorder = performRequirePermissionRequest(common.RoleRootUser, authz.Permission{Resource: authz.ResourceChannel, Action: "unknown"})
	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func performRequirePermissionRequest(role int, permission authz.Permission) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/protected",
		func(c *gin.Context) {
			c.Set("id", 42)
			c.Set("role", role)
			c.Next()
		},
		RequirePermission(permission),
		func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		},
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	router.ServeHTTP(recorder, request)
	return recorder
}
