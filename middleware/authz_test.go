package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/authz"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestRequirePermissionHonorsUserOverrides(t *testing.T) {
	setupRequirePermissionAuthzDB(t)
	require.NoError(t, authz.SetUserPermissions(42, authz.PermissionsMap{
		authz.ResourceChannel: {
			authz.ActionWrite:          false,
			authz.ActionSensitiveWrite: true,
		},
	}))

	recorder := performRequirePermissionRequest(common.RoleAdminUser, authz.ChannelSensitiveWrite)
	require.Equal(t, http.StatusNoContent, recorder.Code)

	recorder = performRequirePermissionRequest(common.RoleAdminUser, authz.ChannelWrite)
	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func setupRequirePermissionAuthzDB(t *testing.T) {
	t.Helper()

	oldDB := model.DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AuthzUserOverride{}, &model.CasbinRule{}))
	require.NoError(t, db.Create(&model.User{
		Id:       42,
		Username: "permission-admin",
		Password: "password",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "permission-admin",
	}).Error)
	model.DB = db

	t.Cleanup(func() {
		model.DB = oldDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
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
