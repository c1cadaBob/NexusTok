package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAuditTestRouter(t *testing.T, role int) (*gin.Engine, []*http.Cookie) {
	t.Helper()

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.RedisEnabled = oldRedisEnabled
	})

	require.NoError(t, model.DB.Create(&model.User{
		Id:       1,
		Username: "root",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("audit-test"))))
	router.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("username", "root")
		session.Set("role", role)
		session.Set("id", 1)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "default")
		if err := session.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false})
			return
		}
		c.Status(http.StatusNoContent)
	})

	router.POST("/api/option/rest_model_ratio", RootAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	router.GET("/api/option/rest_model_ratio", RootAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	router.POST("/api/option/fail", RootAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "failed"})
	})
	router.POST("/api/option/http_fail", RootAuth(), func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"success": true})
	})
	router.POST("/api/option/manual", RootAuth(), func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyAuditLogged, true)
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	router.POST("/api/token/", UserAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	channelRoute := router.Group("/api/channel")
	channelRoute.Use(AdminAuth())
	channelRoute.POST("/:id/key", RootAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodGet, "/login", nil)
	router.ServeHTTP(loginRecorder, loginRequest)
	require.Equal(t, http.StatusNoContent, loginRecorder.Code)

	return router, loginRecorder.Result().Cookies()
}

func performAuditTestRequest(t *testing.T, router *gin.Engine, cookies []*http.Cookie, method string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("NexusTok-User", "1")
	for _, ck := range cookies {
		request.AddCookie(ck)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func auditLogCount(t *testing.T) int64 {
	t.Helper()
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeManage).Count(&count).Error)
	return count
}

func lastAuditLogOther(t *testing.T) map[string]interface{} {
	t.Helper()
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeManage).Order("id desc").First(&log).Error)
	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	return other
}

func TestAdminAuditRecordsWriteRequest(t *testing.T) {
	router, cookies := setupAuditTestRouter(t, common.RoleRootUser)

	recorder := performAuditTestRequest(t, router, cookies, http.MethodPost, "/api/option/rest_model_ratio", `{}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.EqualValues(t, 1, auditLogCount(t))
	other := lastAuditLogOther(t)
	op := other["op"].(map[string]interface{})
	require.Equal(t, "option.reset_model_ratio", op["action"])
	adminInfo := other["admin_info"].(map[string]interface{})
	require.Equal(t, "root", adminInfo["admin_username"])
	auditInfo := other["audit_info"].(map[string]interface{})
	require.Equal(t, "/api/option/rest_model_ratio", auditInfo["route"])
	require.Equal(t, true, auditInfo["success"])
}

func TestAdminAuditSkipsReadRequest(t *testing.T) {
	router, cookies := setupAuditTestRouter(t, common.RoleRootUser)

	recorder := performAuditTestRequest(t, router, cookies, http.MethodGet, "/api/option/rest_model_ratio", "")

	require.Equal(t, http.StatusOK, recorder.Code)
	require.EqualValues(t, 0, auditLogCount(t))
}

func TestAdminAuditDetectsBusinessFailure(t *testing.T) {
	router, cookies := setupAuditTestRouter(t, common.RoleRootUser)

	recorder := performAuditTestRequest(t, router, cookies, http.MethodPost, "/api/option/fail", `{}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.EqualValues(t, 1, auditLogCount(t))
	other := lastAuditLogOther(t)
	auditInfo := other["audit_info"].(map[string]interface{})
	require.Equal(t, false, auditInfo["success"])
}

func TestAdminAuditDetectsHTTPFailure(t *testing.T) {
	router, cookies := setupAuditTestRouter(t, common.RoleRootUser)

	recorder := performAuditTestRequest(t, router, cookies, http.MethodPost, "/api/option/http_fail", `{}`)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.EqualValues(t, 1, auditLogCount(t))
	other := lastAuditLogOther(t)
	auditInfo := other["audit_info"].(map[string]interface{})
	require.Equal(t, false, auditInfo["success"])
	require.Equal(t, float64(http.StatusBadRequest), auditInfo["status"])
}

func TestAdminAuditSkipsManuallyLoggedRequest(t *testing.T) {
	router, cookies := setupAuditTestRouter(t, common.RoleRootUser)

	recorder := performAuditTestRequest(t, router, cookies, http.MethodPost, "/api/option/manual", `{}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.EqualValues(t, 0, auditLogCount(t))
}

func TestAdminAuditNestedRootAuthRecordsOnce(t *testing.T) {
	router, cookies := setupAuditTestRouter(t, common.RoleRootUser)

	recorder := performAuditTestRequest(t, router, cookies, http.MethodPost, "/api/channel/12/key", `{}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.EqualValues(t, 1, auditLogCount(t))
	other := lastAuditLogOther(t)
	op := other["op"].(map[string]interface{})
	require.Equal(t, "channel.key_view", op["action"])
	auditInfo := other["audit_info"].(map[string]interface{})
	params := auditInfo["params"].(map[string]interface{})
	require.Equal(t, "12", params["id"])
}

func TestUserAuthWriteDoesNotRecordManageAudit(t *testing.T) {
	router, cookies := setupAuditTestRouter(t, common.RoleRootUser)

	recorder := performAuditTestRequest(t, router, cookies, http.MethodPost, "/api/token/", `{}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.EqualValues(t, 0, auditLogCount(t))
}

func TestAdminAuditAuthzRoutesHaveStableActions(t *testing.T) {
	expected := map[string]string{
		"POST /api/authz/roles":              "authz.role_create",
		"PUT /api/authz/roles/:key":          "authz.role_update",
		"DELETE /api/authz/roles/:key":       "authz.role_delete",
		"PUT /api/authz/roles/:key/policies": "authz.role_policies_update",
		"POST /api/authz/policies/import":    "authz.policies_import",
	}

	for route, action := range expected {
		require.Equal(t, action, auditRouteActions[route], route)
	}
}
