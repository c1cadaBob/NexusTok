package authz

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAuthzTestDB(t *testing.T, migrateModels ...interface{}) *gorm.DB {
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
	clearPersistentPolicySnapshot()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(migrateModels...))
	model.DB = db

	t.Cleanup(func() {
		clearPersistentPolicySnapshot()
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

	return db
}

func setupAuthzOverrideTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return setupAuthzTestDB(
		t,
		&model.AuthzUserOverride{},
		&model.AuthzRole{},
		&model.CasbinRule{},
	)
}

func setupAuthzPolicyMissingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return setupAuthzTestDB(t, &model.AuthzUserOverride{})
}

func TestSetUserPermissionsStoresOnlyOverrides(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)

	require.NoError(t, SetUserPermissions(42, PermissionsMap{
		ResourceChannel: {
			ActionRead:           true,
			ActionSensitiveWrite: true,
			ActionSecretView:     false,
		},
		ResourceSystemSetting: {
			ActionRead:  false,
			ActionWrite: true,
		},
		"unknown": {
			ActionRead: true,
		},
	}))

	var records []model.AuthzUserOverride
	require.NoError(t, db.Order("resource ASC, action ASC").Find(&records).Error)
	require.Len(t, records, 3)
	assertAuthzOverrideRecord(t, records[0], 42, ResourceChannel, ActionSensitiveWrite, EffectAllow)
	assertAuthzOverrideRecord(t, records[1], 42, ResourceSystemSetting, ActionRead, EffectDeny)
	assertAuthzOverrideRecord(t, records[2], 42, ResourceSystemSetting, ActionWrite, EffectAllow)

	assert.Equal(t, PermissionsMap{
		ResourceChannel: {
			ActionSensitiveWrite: true,
		},
		ResourceSystemSetting: {
			ActionRead:  false,
			ActionWrite: true,
		},
	}, ExplicitUserOverrides(42))
}

func TestCanUsesUserOverridesWithoutElevatingCommonUsers(t *testing.T) {
	setupAuthzOverrideTestDB(t)

	require.NoError(t, SetUserPermissions(42, PermissionsMap{ResourceChannel: {
		ActionRead:           false,
		ActionSensitiveWrite: true,
	}}))
	require.NoError(t, SetUserPermissions(7, PermissionsMap{ResourceChannel: {
		ActionRead:           false,
		ActionSensitiveWrite: false,
	}}))
	require.NoError(t, SetUserPermissions(99, PermissionsMap{ResourceChannel: {
		ActionSensitiveWrite: true,
	}}))

	assert.False(t, Can(42, common.RoleAdminUser, ChannelRead))
	assert.True(t, Can(42, common.RoleAdminUser, ChannelSensitiveWrite))
	assert.True(t, Can(43, common.RoleAdminUser, ChannelRead))
	assert.False(t, Can(43, common.RoleAdminUser, ChannelSensitiveWrite))
	assert.True(t, Can(7, common.RoleRootUser, ChannelRead))
	assert.True(t, Can(7, common.RoleRootUser, ChannelSensitiveWrite))
	assert.False(t, Can(99, common.RoleCommonUser, ChannelSensitiveWrite))
	assert.False(t, Can(42, common.RoleAdminUser, Permission{Resource: ResourceChannel, Action: "unknown"}))
}

func TestCapabilitiesForUserAppliesOverrides(t *testing.T) {
	setupAuthzOverrideTestDB(t)

	require.NoError(t, SetUserPermissions(42, PermissionsMap{ResourceChannel: {
		ActionRead:           false,
		ActionSensitiveWrite: true,
	}}))

	admin := CapabilitiesForUser(42, common.RoleAdminUser)
	assert.False(t, admin[ResourceChannel][ActionRead])
	assert.True(t, admin[ResourceChannel][ActionSensitiveWrite])
	assert.True(t, Capabilities(common.RoleAdminUser)[ResourceChannel][ActionRead])
	assert.False(t, Capabilities(common.RoleAdminUser)[ResourceChannel][ActionSensitiveWrite])

	root := CapabilitiesForUser(42, common.RoleRootUser)
	assert.True(t, root[ResourceChannel][ActionRead])
	assert.True(t, root[ResourceChannel][ActionSensitiveWrite])
}

func TestClearUserAuthorizationRemovesOverrides(t *testing.T) {
	setupAuthzOverrideTestDB(t)

	require.NoError(t, SetUserPermissions(42, PermissionsMap{ResourceChannel: {
		ActionSensitiveWrite: true,
	}}))
	require.NotEmpty(t, ExplicitUserOverrides(42))

	require.NoError(t, ClearUserAuthorization(42))

	assert.Empty(t, ExplicitUserOverrides(42))
	assert.False(t, Can(42, common.RoleAdminUser, ChannelSensitiveWrite))
}

func TestSetUserPermissionsInTxRollbackLeavesNoOverrides(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)

	errRollback := errors.New("rollback")
	err := db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, SetUserPermissionsInTx(tx, 42, PermissionsMap{ResourceChannel: {
			ActionSensitiveWrite: true,
		}}))
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)

	assert.Empty(t, ExplicitUserOverrides(42))
}

func assertAuthzOverrideRecord(t *testing.T, record model.AuthzUserOverride, userID int, resource string, action string, effect string) {
	t.Helper()

	assert.Equal(t, userID, record.UserID)
	assert.Equal(t, resource, record.Resource)
	assert.Equal(t, action, record.Action)
	assert.Equal(t, effect, record.Effect)
	assert.NotZero(t, record.CreatedAt)
	assert.NotZero(t, record.UpdatedAt)
}
