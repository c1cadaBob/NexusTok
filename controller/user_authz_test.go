package controller

import (
	"bytes"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type getSelfAuthzResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Permissions struct {
			SidebarSettings  *bool                `json:"sidebar_settings"`
			AdminPermissions authz.PermissionsMap `json:"admin_permissions"`
		} `json:"permissions"`
	} `json:"data"`
}

type userDetailAuthzResponse struct {
	Success bool       `json:"success"`
	Message string     `json:"message"`
	Data    model.User `json:"data"`
}

type simpleAuthzResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func setupUserAuthzControllerTestDB(t *testing.T) *gorm.DB {
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
		&model.AuthzUserOverride{},
		&model.AuthzRole{},
		&model.CasbinRule{},
	))
	require.NoError(t, db.Create(&model.User{
		Id:          1,
		Username:    "operator-root",
		Password:    "password",
		DisplayName: "operator-root",
		Role:        common.RoleRootUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "operator-root",
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

func TestGetSelfReturnsAdminPermissions(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	users := []model.User{
		{Id: 101, Username: "root", Password: "password", DisplayName: "root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "root"},
		{Id: 102, Username: "admin", Password: "password", DisplayName: "admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "admin"},
		{Id: 103, Username: "user", Password: "password", DisplayName: "user", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "user"},
	}
	require.NoError(t, db.Create(&users).Error)

	rootResp := performGetSelfForAuthz(t, users[0].Id, common.RoleRootUser)
	assert.False(t, *rootResp.Data.Permissions.SidebarSettings)
	assertPermission(t, rootResp.Data.Permissions.AdminPermissions, "channel", authz.ActionSensitiveWrite, true)
	assertPermission(t, rootResp.Data.Permissions.AdminPermissions, "channel_account", authz.ActionSensitiveWrite, true)
	assertPermission(t, rootResp.Data.Permissions.AdminPermissions, "redemption", authz.ActionSecretView, true)
	assertPermission(t, rootResp.Data.Permissions.AdminPermissions, "system_setting", authz.ActionSecretView, true)

	adminResp := performGetSelfForAuthz(t, users[1].Id, common.RoleAdminUser)
	assert.True(t, *adminResp.Data.Permissions.SidebarSettings)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "channel", authz.ActionRead, true)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "channel", authz.ActionWrite, true)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "channel", authz.ActionSensitiveWrite, false)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "channel_account", authz.ActionRead, true)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "channel_account", authz.ActionOperate, true)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "channel_account", authz.ActionSensitiveWrite, false)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "user", authz.ActionRead, true)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "user", authz.ActionOperate, true)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "user", authz.ActionWrite, true)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "user", authz.ActionSensitiveWrite, false)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "redemption", authz.ActionRead, true)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "redemption", authz.ActionWrite, true)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "redemption", authz.ActionSensitiveWrite, false)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "redemption", authz.ActionSecretView, false)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "usage_log", authz.ActionRead, true)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "usage_log", authz.ActionSensitiveWrite, false)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "usage_data", authz.ActionRead, true)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "account_pool_auth_file", authz.ActionRead, true)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "account_pool_auth_file", authz.ActionSensitiveWrite, false)
	assertPermission(t, adminResp.Data.Permissions.AdminPermissions, "system_setting", authz.ActionWrite, false)

	userResp := performGetSelfForAuthz(t, users[2].Id, common.RoleCommonUser)
	assert.True(t, *userResp.Data.Permissions.SidebarSettings)
	assertPermission(t, userResp.Data.Permissions.AdminPermissions, "channel", authz.ActionRead, false)
	assertPermission(t, userResp.Data.Permissions.AdminPermissions, "channel_account", authz.ActionRead, false)
	assertPermission(t, userResp.Data.Permissions.AdminPermissions, "account_pool", authz.ActionWrite, false)
	assertPermission(t, userResp.Data.Permissions.AdminPermissions, "account_pool_auth_file", authz.ActionRead, false)
	assertPermission(t, userResp.Data.Permissions.AdminPermissions, "user", authz.ActionRead, false)
	assertPermission(t, userResp.Data.Permissions.AdminPermissions, "redemption", authz.ActionRead, false)
	assertPermission(t, userResp.Data.Permissions.AdminPermissions, "usage_log", authz.ActionRead, false)
	assertPermission(t, userResp.Data.Permissions.AdminPermissions, "usage_data", authz.ActionRead, false)
	assertPermission(t, userResp.Data.Permissions.AdminPermissions, "system_setting", authz.ActionRead, false)
}

func TestGetSelfIncludesAuthzUserOverrides(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	user := model.User{Id: 201, Username: "scoped-admin", Password: "password", DisplayName: "scoped-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "scoped"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, authz.SetUserPermissions(user.Id, authz.PermissionsMap{
		authz.ResourceChannel: {
			authz.ActionRead:           false,
			authz.ActionSensitiveWrite: true,
		},
	}))

	resp := performGetSelfForAuthz(t, user.Id, common.RoleAdminUser)

	assertPermission(t, resp.Data.Permissions.AdminPermissions, "channel", authz.ActionRead, false)
	assertPermission(t, resp.Data.Permissions.AdminPermissions, "channel", authz.ActionWrite, true)
	assertPermission(t, resp.Data.Permissions.AdminPermissions, "channel", authz.ActionSensitiveWrite, true)
}

func TestGetUserReturnsAdminPermissions(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	user := model.User{Id: 301, Username: "scoped-admin", Password: "password", DisplayName: "scoped-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "scoped-admin"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, authz.SetUserPermissions(user.Id, authz.PermissionsMap{
		authz.ResourceChannel: {
			authz.ActionRead:           false,
			authz.ActionSensitiveWrite: true,
		},
	}))

	resp := performGetUserForAuthz(t, user.Id, common.RoleRootUser)

	assertPermission(t, resp.Data.AdminPermissions, authz.ResourceChannel, authz.ActionRead, false)
	assertPermission(t, resp.Data.AdminPermissions, authz.ResourceChannel, authz.ActionWrite, true)
	assertPermission(t, resp.Data.AdminPermissions, authz.ResourceChannel, authz.ActionSensitiveWrite, true)
}

func TestUpdateUserAdminPermissions(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	user := model.User{Id: 302, Username: "editable-admin", Password: "password", DisplayName: "before", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "editable-admin"}
	require.NoError(t, db.Create(&user).Error)

	resp := performUpdateUserForAuthz(t, common.RoleRootUser, model.User{
		Id:          user.Id,
		Username:    user.Username,
		DisplayName: "after",
		Group:       "vip",
		Remark:      "scoped admin",
		AdminPermissions: authz.PermissionsMap{
			authz.ResourceChannel: {
				authz.ActionRead:           false,
				authz.ActionSensitiveWrite: true,
			},
		},
	})

	require.True(t, resp.Success, resp.Message)
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, "after", stored.DisplayName)
	assert.Equal(t, "vip", stored.Group)
	assertPermission(t, authz.CapabilitiesForUser(user.Id, common.RoleAdminUser), authz.ResourceChannel, authz.ActionRead, false)
	assertPermission(t, authz.CapabilitiesForUser(user.Id, common.RoleAdminUser), authz.ResourceChannel, authz.ActionSensitiveWrite, true)
}

func TestAssignedAuthzRoleBecomesAdminBaseline(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	require.NoError(t, db.Create(&model.AuthzRole{
		Key:     "auditor",
		Name:    "Auditor",
		Enabled: true,
		Sort:    100,
	}).Error)
	require.NoError(t, db.Create(&model.CasbinRule{
		Ptype: "p",
		V0:    authz.RoleSubject("auditor"),
		V1:    authz.ResourceChannel,
		V2:    authz.ActionRead,
		V3:    authz.EffectAllow,
	}).Error)
	require.NoError(t, authz.ReloadPersistentPolicies())

	user := model.User{
		Id:        306,
		Username:  "assigned-admin",
		Password:  "password",
		Role:      common.RoleAdminUser,
		AuthzRole: "auditor",
		Status:    common.UserStatusEnabled,
		Group:     "default",
		AffCode:   "assigned-admin",
	}
	require.NoError(t, db.Create(&user).Error)

	resp := performGetSelfForAuthz(t, user.Id, common.RoleAdminUser)

	assertPermission(t, resp.Data.Permissions.AdminPermissions, authz.ResourceChannel, authz.ActionRead, true)
	assertPermission(t, resp.Data.Permissions.AdminPermissions, authz.ResourceChannel, authz.ActionWrite, false)
	assertPermission(t, resp.Data.Permissions.AdminPermissions, authz.ResourceUser, authz.ActionRead, false)
	assert.True(t, authz.Can(user.Id, common.RoleAdminUser, authz.ChannelRead))
	assert.False(t, authz.Can(user.Id, common.RoleAdminUser, authz.ChannelWrite))
}

func TestUpdateUserAuthzRoleAssignmentAndOverrides(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	require.NoError(t, db.Create(&model.AuthzRole{
		Key:     "auditor",
		Name:    "Auditor",
		Enabled: true,
		Sort:    100,
	}).Error)
	require.NoError(t, db.Create(&model.CasbinRule{
		Ptype: "p",
		V0:    authz.RoleSubject("auditor"),
		V1:    authz.ResourceChannel,
		V2:    authz.ActionRead,
		V3:    authz.EffectAllow,
	}).Error)
	require.NoError(t, authz.ReloadPersistentPolicies())

	user := model.User{
		Id:          307,
		Username:    "role-edit-admin",
		Password:    "password",
		DisplayName: "before",
		Role:        common.RoleAdminUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "role-edit-admin",
	}
	require.NoError(t, db.Create(&user).Error)

	resp := performUpdateUserForAuthz(t, common.RoleRootUser, model.User{
		Id:          user.Id,
		Username:    user.Username,
		DisplayName: "after",
		Group:       "vip",
		AuthzRole:   "auditor",
		AdminPermissions: authz.PermissionsMap{
			authz.ResourceChannel: {
				authz.ActionRead:  true,
				authz.ActionWrite: true,
			},
		},
	})

	require.True(t, resp.Success, resp.Message)
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, "auditor", stored.AuthzRole)
	assertPermission(t, authz.CapabilitiesForUser(user.Id, common.RoleAdminUser), authz.ResourceChannel, authz.ActionRead, true)
	assertPermission(t, authz.CapabilitiesForUser(user.Id, common.RoleAdminUser), authz.ResourceChannel, authz.ActionWrite, true)

	overrides, err := model.GetAuthzUserOverrides(user.Id)
	require.NoError(t, err)
	require.Len(t, overrides, 1)
	assert.Equal(t, authz.ResourceChannel, overrides[0].Resource)
	assert.Equal(t, authz.ActionWrite, overrides[0].Action)
	assert.Equal(t, authz.EffectAllow, overrides[0].Effect)
}

func TestUpdateUserPreservesAuthzRoleWhenFieldOmitted(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	require.NoError(t, db.Create(&model.AuthzRole{
		Key:     "auditor",
		Name:    "Auditor",
		Enabled: true,
		Sort:    100,
	}).Error)
	user := model.User{
		Id:          308,
		Username:    "preserve-role-admin",
		Password:    "password",
		DisplayName: "before",
		Role:        common.RoleAdminUser,
		AuthzRole:   "auditor",
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "preserve-role-admin",
	}
	require.NoError(t, db.Create(&user).Error)

	resp := performUpdateUserForAuthz(t, common.RoleRootUser, model.User{
		Id:          user.Id,
		Username:    user.Username,
		DisplayName: "after",
		Group:       "default",
	})

	require.True(t, resp.Success, resp.Message)
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, "auditor", stored.AuthzRole)
}

func TestUpdateUserClearsAuthzRoleWhenExplicitlyEmpty(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	user := model.User{
		Id:          309,
		Username:    "clear-role-admin",
		Password:    "password",
		DisplayName: "before",
		Role:        common.RoleAdminUser,
		AuthzRole:   "auditor",
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "clear-role-admin",
	}
	require.NoError(t, db.Create(&user).Error)

	resp := performUpdateUserRawForAuthz(
		t,
		common.RoleRootUser,
		`{"id":309,"username":"clear-role-admin","display_name":"after","group":"default","authz_role":""}`,
	)

	require.True(t, resp.Success, resp.Message)
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Empty(t, stored.AuthzRole)
}

func TestUpdateUserRejectsNonRootAdminPermissions(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	user := model.User{Id: 303, Username: "common-user", Password: "password", DisplayName: "before", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "common-user"}
	require.NoError(t, db.Create(&user).Error)

	resp := performUpdateUserForAuthz(t, common.RoleAdminUser, model.User{
		Id:          user.Id,
		Username:    user.Username,
		DisplayName: "should-rollback",
		Group:       "vip",
		AdminPermissions: authz.PermissionsMap{
			authz.ResourceChannel: {
				authz.ActionSensitiveWrite: true,
			},
		},
	})

	require.False(t, resp.Success)
	assert.Contains(t, resp.Message, "only root")
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, "before", stored.DisplayName)
	assert.Equal(t, "default", stored.Group)
}

func TestCreateUserAdminPermissions(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	resp := performCreateUserForAuthz(t, common.RoleRootUser, model.User{
		Username:    "new-admin",
		Password:    "Password123",
		DisplayName: "new-admin",
		Role:        common.RoleAdminUser,
		AdminPermissions: authz.PermissionsMap{
			authz.ResourceChannel: {
				authz.ActionRead:           false,
				authz.ActionSensitiveWrite: true,
			},
		},
	})

	require.True(t, resp.Success, resp.Message)
	var stored model.User
	require.NoError(t, db.Where("username = ?", "new-admin").First(&stored).Error)
	assert.Equal(t, common.RoleAdminUser, stored.Role)
	assertPermission(t, authz.CapabilitiesForUser(stored.Id, common.RoleAdminUser), authz.ResourceChannel, authz.ActionRead, false)
	assertPermission(t, authz.CapabilitiesForUser(stored.Id, common.RoleAdminUser), authz.ResourceChannel, authz.ActionSensitiveWrite, true)
}

func TestAdminCreateCommonUserWithoutAuthzFields(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	resp := performCreateUserForAuthz(t, common.RoleAdminUser, model.User{
		Username:    "admin-created-user",
		Password:    "Password123",
		DisplayName: "admin-created-user",
		Role:        common.RoleCommonUser,
	})

	require.True(t, resp.Success, resp.Message)
	var stored model.User
	require.NoError(t, db.Where("username = ?", "admin-created-user").First(&stored).Error)
	assert.Equal(t, common.RoleCommonUser, stored.Role)
	assert.Empty(t, stored.AuthzRole)
	records, err := model.GetAuthzUserOverrides(stored.Id)
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestRootCreateCommonUserDoesNotWriteAuthzOverrides(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	resp := performCreateUserForAuthz(t, common.RoleRootUser, model.User{
		Username:    "root-created-common",
		Password:    "Password123",
		DisplayName: "root-created-common",
		Role:        common.RoleCommonUser,
	})

	require.True(t, resp.Success, resp.Message)
	var stored model.User
	require.NoError(t, db.Where("username = ?", "root-created-common").First(&stored).Error)
	assert.Equal(t, common.RoleCommonUser, stored.Role)
	assert.Empty(t, stored.AuthzRole)
	records, err := model.GetAuthzUserOverrides(stored.Id)
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestAdminCreateUserRejectsExplicitAuthzFields(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	resp := performCreateUserRawForAuthz(t, common.RoleAdminUser, `{
		"username": "forged-authz-user",
		"password": "Password123",
		"display_name": "forged-authz-user",
		"role": 1,
		"authz_role": "admin"
	}`)

	require.False(t, resp.Success)
	assert.Contains(t, resp.Message, "only root")
	var count int64
	require.NoError(t, db.Model(&model.User{}).Where("username = ?", "forged-authz-user").Count(&count).Error)
	assert.Zero(t, count)
}

func TestUpdateUserClearsAdminPermissionsWhenTargetIsCommon(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	user := model.User{Id: 304, Username: "demoted-earlier", Password: "password", DisplayName: "before", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "demoted-earlier"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, model.ReplaceAuthzUserResourceOverridesInTx(db, user.Id, authz.ResourceChannel, []model.AuthzUserOverride{
		{UserID: user.Id, Resource: authz.ResourceChannel, Action: authz.ActionSensitiveWrite, Effect: authz.EffectAllow},
	}))

	resp := performUpdateUserForAuthz(t, common.RoleRootUser, model.User{
		Id:          user.Id,
		Username:    user.Username,
		DisplayName: "after",
		Group:       "default",
	})

	require.True(t, resp.Success, resp.Message)
	records, err := model.GetAuthzUserOverrides(user.Id)
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestDemoteClearsAdminPermissions(t *testing.T) {
	db := setupUserAuthzControllerTestDB(t)

	user := model.User{Id: 305, Username: "admin-to-demote", Password: "password", DisplayName: "admin-to-demote", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "admin-to-demote"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, authz.SetUserPermissions(user.Id, authz.PermissionsMap{
		authz.ResourceChannel: {
			authz.ActionSensitiveWrite: true,
		},
	}))

	resp := performManageUserForAuthz(t, common.RoleRootUser, ManageRequest{Id: user.Id, Action: "demote"})

	require.True(t, resp.Success, resp.Message)
	var stored model.User
	require.NoError(t, db.First(&stored, user.Id).Error)
	assert.Equal(t, common.RoleCommonUser, stored.Role)
	assert.Empty(t, stored.AuthzRole)
	records, err := model.GetAuthzUserOverrides(user.Id)
	require.NoError(t, err)
	assert.Empty(t, records)
}

func TestUserManageActionNeedsSensitiveWrite(t *testing.T) {
	cases := []struct {
		name     string
		req      ManageRequest
		expected bool
	}{
		{name: "disable is operate", req: ManageRequest{Action: "disable"}, expected: false},
		{name: "enable is operate", req: ManageRequest{Action: "enable"}, expected: false},
		{name: "delete changes account lifecycle", req: ManageRequest{Action: "delete"}, expected: true},
		{name: "promote changes administrator boundary", req: ManageRequest{Action: "promote"}, expected: true},
		{name: "demote changes administrator boundary", req: ManageRequest{Action: "demote"}, expected: true},
		{name: "add quota changes wallet balance", req: ManageRequest{Action: "add_quota", Mode: "add"}, expected: true},
		{name: "unknown action stays non-sensitive and fails later validation", req: ManageRequest{Action: "unknown"}, expected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, userManageActionNeedsSensitiveWrite(tc.req))
		})
	}
}

func performGetSelfForAuthz(t *testing.T, userID int, role int) getSelfAuthzResponse {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/user/self", nil)
	c.Set("id", userID)
	c.Set("role", role)

	GetSelf(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response getSelfAuthzResponse
	require.NoError(t, common.DecodeJson(w.Body, &response))
	require.True(t, response.Success)
	require.NotNil(t, response.Data.Permissions.SidebarSettings)
	require.NotEmpty(t, response.Data.Permissions.AdminPermissions)
	return response
}

func performGetUserForAuthz(t *testing.T, targetUserID int, role int) userDetailAuthzResponse {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/user/%d", targetUserID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", targetUserID)}}
	c.Set("id", 1)
	c.Set("role", role)

	GetUser(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response userDetailAuthzResponse
	require.NoError(t, common.DecodeJson(w.Body, &response))
	require.True(t, response.Success, response.Message)
	require.NotEmpty(t, response.Data.AdminPermissions)
	return response
}

func performUpdateUserForAuthz(t *testing.T, role int, payload model.User) simpleAuthzResponse {
	t.Helper()

	body, err := common.Marshal(payload)
	require.NoError(t, err)
	return performUpdateUserRawForAuthz(t, role, string(body))
}

func performUpdateUserRawForAuthz(t *testing.T, role int, body string) simpleAuthzResponse {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/user/", strings.NewReader(body))
	c.Set("id", 1)
	c.Set("role", role)

	UpdateUser(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response simpleAuthzResponse
	require.NoError(t, common.DecodeJson(w.Body, &response))
	return response
}

func performCreateUserForAuthz(t *testing.T, role int, payload model.User) simpleAuthzResponse {
	t.Helper()

	body, err := common.Marshal(payload)
	require.NoError(t, err)
	return performCreateUserRawForAuthz(t, role, string(body))
}

func performCreateUserRawForAuthz(t *testing.T, role int, body string) simpleAuthzResponse {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/", strings.NewReader(body))
	c.Set("id", 1)
	c.Set("role", role)

	CreateUser(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response simpleAuthzResponse
	require.NoError(t, common.DecodeJson(w.Body, &response))
	return response
}

func performManageUserForAuthz(t *testing.T, role int, payload ManageRequest) simpleAuthzResponse {
	t.Helper()

	body, err := common.Marshal(payload)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/manage", bytes.NewReader(body))
	c.Set("id", 1)
	c.Set("role", role)
	c.Set("username", "root")

	ManageUser(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response simpleAuthzResponse
	require.NoError(t, common.DecodeJson(w.Body, &response))
	return response
}

func assertPermission(t *testing.T, grants authz.PermissionsMap, resource string, action string, expected bool) {
	t.Helper()

	resourceGrants, ok := grants[resource]
	require.True(t, ok, "resource %s should exist", resource)
	actual, ok := resourceGrants[action]
	require.True(t, ok, "action %s should exist on resource %s", action, resource)
	assert.Equal(t, expected, actual)
}
