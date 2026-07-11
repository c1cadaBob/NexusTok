package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/authz"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAuthzAuditControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldLogDB := model.LOG_DB
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
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Log{},
		&model.AuthzUserOverride{},
		&model.AuthzRole{},
		&model.CasbinRule{},
	))
	require.NoError(t, db.Create(&model.User{
		Id:       1,
		Username: "root",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}).Error)

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func newAuthzAuditContext(t *testing.T, method string, path string, body interface{}) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	var payload []byte
	switch value := body.(type) {
	case nil:
	case string:
		payload = []byte(value)
	default:
		data, err := common.Marshal(value)
		require.NoError(t, err)
		payload = data
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	ctx.Set("username", "root")
	ctx.Set("role", common.RoleRootUser)
	return ctx, recorder
}

func lastAuthzAuditOperation(t *testing.T) map[string]interface{} {
	t.Helper()

	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeManage).Order("id desc").First(&log).Error)
	require.Equal(t, "root", log.Username)

	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	op, ok := other["op"].(map[string]interface{})
	require.True(t, ok)
	return op
}

func requireSingleAuthzAuditLog(t *testing.T) map[string]interface{} {
	t.Helper()

	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeManage).Count(&count).Error)
	require.EqualValues(t, 1, count)
	return lastAuthzAuditOperation(t)
}

func TestCreatePermissionRoleWritesAuditSummary(t *testing.T) {
	setupAuthzAuditControllerTestDB(t)
	ctx, recorder := newAuthzAuditContext(t, http.MethodPost, "/api/authz/roles", authz.RoleTemplateMutationRequest{
		Key:         "auditor",
		Name:        "Auditor",
		Description: "Read-only audit role",
		Enabled:     authzAuditBoolPtr(false),
		Sort:        authzAuditIntPtr(90),
	})

	CreatePermissionRole(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyAuditLogged))
	op := requireSingleAuthzAuditLog(t)
	require.Equal(t, "authz.role_create", op["action"])
	params := op["params"].(map[string]interface{})
	require.Equal(t, "auditor", params["role_key"])
	require.Equal(t, "Auditor", params["role_name"])
	require.Equal(t, false, params["enabled"])
	require.EqualValues(t, 90, params["sort"])
	require.EqualValues(t, 0, params["policy_count"])
	require.Equal(t, false, params["built_in"])
	require.Equal(t, false, params["runtime_managed"])
}

func TestUpdatePermissionRoleWritesAuditSummary(t *testing.T) {
	db := setupAuthzAuditControllerTestDB(t)
	require.NoError(t, db.Create(&model.AuthzRole{
		Key:         "auditor",
		Name:        "Auditor",
		Description: "Before",
		Enabled:     true,
		Sort:        40,
	}).Error)
	ctx, recorder := newAuthzAuditContext(t, http.MethodPut, "/api/authz/roles/auditor", authz.RoleTemplateMutationRequest{
		Name:        "External Auditor",
		Description: "Reviews usage records",
		Enabled:     authzAuditBoolPtr(true),
		Sort:        authzAuditIntPtr(70),
	})
	ctx.Params = gin.Params{{Key: "key", Value: "auditor"}}

	UpdatePermissionRole(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	op := requireSingleAuthzAuditLog(t)
	require.Equal(t, "authz.role_update", op["action"])
	params := op["params"].(map[string]interface{})
	require.Equal(t, "auditor", params["role_key"])
	require.Equal(t, "External Auditor", params["role_name"])
	require.Equal(t, true, params["enabled"])
	require.EqualValues(t, 70, params["sort"])
}

func TestUpdatePermissionRolePoliciesWritesCountsWithoutGrants(t *testing.T) {
	db := setupAuthzAuditControllerTestDB(t)
	require.NoError(t, db.Create(&model.AuthzRole{
		Key:     authz.BuiltInRoleAdmin,
		Name:    "Admin",
		BuiltIn: true,
		Enabled: true,
		Sort:    10,
	}).Error)
	require.NoError(t, db.Create(&[]model.CasbinRule{
		{
			Ptype: "p",
			V0:    authz.RoleSubject(authz.BuiltInRoleAdmin),
			V1:    authz.ResourceChannel,
			V2:    authz.ActionRead,
			V3:    authz.EffectAllow,
		},
		{
			Ptype: "p",
			V0:    authz.RoleSubject(authz.BuiltInRoleAdmin),
			V1:    authz.ResourceModel,
			V2:    authz.ActionRead,
			V3:    authz.EffectAllow,
		},
	}).Error)
	ctx, recorder := newAuthzAuditContext(t, http.MethodPut, "/api/authz/roles/admin/policies", authz.RolePolicyUpdateRequest{
		Grants: authz.PermissionsMap{
			authz.ResourceChannel: {
				authz.ActionRead: true,
			},
		},
	})
	ctx.Params = gin.Params{{Key: "key", Value: authz.BuiltInRoleAdmin}}

	UpdatePermissionRolePolicies(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	op := requireSingleAuthzAuditLog(t)
	require.Equal(t, "authz.role_policies_update", op["action"])
	params := op["params"].(map[string]interface{})
	require.Equal(t, authz.BuiltInRoleAdmin, params["role_key"])
	require.Equal(t, true, params["dry_run"])
	require.Equal(t, false, params["applied"])
	require.Equal(t, false, params["reloaded"])
	require.EqualValues(t, 2, params["old_policy_count"])
	require.EqualValues(t, 1, params["new_policy_count"])
	require.EqualValues(t, 0, params["created_policy_count"])
	require.EqualValues(t, 1, params["deleted_policy_count"])
	require.EqualValues(t, 1, params["unchanged_policy_count"])
	_, hasGrants := params["grants"]
	require.False(t, hasGrants)
}

func authzAuditBoolPtr(value bool) *bool {
	return &value
}

func authzAuditIntPtr(value int) *int {
	return &value
}
