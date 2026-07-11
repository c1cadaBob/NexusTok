package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthzUserOverrideReplaceAndClear(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&AuthzUserOverride{}, &CasbinRule{}))
	require.NoError(t, DB.Exec("DELETE FROM authz_user_overrides").Error)
	require.NoError(t, DB.Exec("DELETE FROM casbin_rule").Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM authz_user_overrides").Error)
		require.NoError(t, DB.Exec("DELETE FROM casbin_rule").Error)
	})

	require.NoError(t, ReplaceAuthzUserResourceOverridesInTx(DB, 42, "channel", []AuthzUserOverride{
		{UserID: 42, Resource: "channel", Action: "read", Effect: "deny"},
		{UserID: 42, Resource: "channel", Action: "sensitive_write", Effect: "allow"},
	}))

	records, err := GetAuthzUserOverrides(42)
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "read", records[0].Action)
	assert.Equal(t, "sensitive_write", records[1].Action)
	assert.NotZero(t, records[0].CreatedAt)
	assert.NotZero(t, records[0].UpdatedAt)
	assertAuthzUserPolicyCount(t, 42, 2)

	require.NoError(t, ReplaceAuthzUserResourceOverridesInTx(DB, 42, "channel", []AuthzUserOverride{
		{UserID: 42, Resource: "channel", Action: "operate", Effect: "deny"},
	}))

	records, err = GetAuthzUserOverrides(42)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "operate", records[0].Action)
	assertAuthzUserPolicyCount(t, 42, 1)

	require.NoError(t, ClearAuthzUserOverridesInTx(DB, 42))
	records, err = GetAuthzUserOverrides(42)
	require.NoError(t, err)
	assert.Empty(t, records)
	assertAuthzUserPolicyCount(t, 42, 0)
}

func TestUserDeleteClearsAuthzUserOverrides(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&User{}, &AuthzUserOverride{}, &CasbinRule{}))
	require.NoError(t, DB.Exec("DELETE FROM authz_user_overrides").Error)
	require.NoError(t, DB.Exec("DELETE FROM casbin_rule").Error)
	require.NoError(t, DB.Unscoped().Exec("DELETE FROM users").Error)
	t.Cleanup(func() {
		require.NoError(t, DB.Exec("DELETE FROM authz_user_overrides").Error)
		require.NoError(t, DB.Exec("DELETE FROM casbin_rule").Error)
		require.NoError(t, DB.Unscoped().Exec("DELETE FROM users").Error)
	})

	softDeletedUser := User{Id: 4101, Username: "soft-delete-authz-user", Password: "password", AffCode: "soft-authz"}
	require.NoError(t, DB.Create(&softDeletedUser).Error)
	require.NoError(t, ReplaceAuthzUserResourceOverridesInTx(DB, softDeletedUser.Id, "channel", []AuthzUserOverride{
		{UserID: softDeletedUser.Id, Resource: "channel", Action: "sensitive_write", Effect: "allow"},
	}))
	require.NoError(t, softDeletedUser.Delete())
	records, err := GetAuthzUserOverrides(softDeletedUser.Id)
	require.NoError(t, err)
	assert.Empty(t, records)
	assertAuthzUserPolicyCount(t, softDeletedUser.Id, 0)

	hardDeletedUser := User{Id: 4102, Username: "hard-delete-authz-user", Password: "password", AffCode: "hard-authz"}
	require.NoError(t, DB.Create(&hardDeletedUser).Error)
	require.NoError(t, ReplaceAuthzUserResourceOverridesInTx(DB, hardDeletedUser.Id, "channel", []AuthzUserOverride{
		{UserID: hardDeletedUser.Id, Resource: "channel", Action: "sensitive_write", Effect: "allow"},
	}))
	require.NoError(t, HardDeleteUserById(hardDeletedUser.Id))
	records, err = GetAuthzUserOverrides(hardDeletedUser.Id)
	require.NoError(t, err)
	assert.Empty(t, records)
	assertAuthzUserPolicyCount(t, hardDeletedUser.Id, 0)
}

func assertAuthzUserPolicyCount(t *testing.T, userID int, expected int64) {
	t.Helper()

	var count int64
	require.NoError(t, DB.Model(&CasbinRule{}).
		Where("ptype = ? AND v0 = ?", "p", authzUserPolicySubject(userID)).
		Count(&count).Error)
	assert.Equal(t, expected, count)
}
