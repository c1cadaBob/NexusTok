package authz

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedPersistentPoliciesStoresBuiltInRolesAndAdminPolicies(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)

	require.NoError(t, SeedPersistentPolicies())

	var roles []model.AuthzRole
	require.NoError(t, db.Order("sort ASC").Find(&roles).Error)
	require.Len(t, roles, 2)
	assert.Equal(t, BuiltInRoleRoot, roles[0].Key)
	assert.Equal(t, "Root", roles[0].Name)
	assert.True(t, roles[0].BuiltIn)
	assert.True(t, roles[0].Enabled)
	assert.Equal(t, BuiltInRoleAdmin, roles[1].Key)
	assert.Equal(t, "Admin", roles[1].Name)
	assert.True(t, roles[1].BuiltIn)
	assert.True(t, roles[1].Enabled)

	var adminPolicies []model.CasbinRule
	require.NoError(t, db.Where("ptype = ? AND v0 = ?", "p", RoleSubject(BuiltInRoleAdmin)).
		Order("v1 ASC, v2 ASC").
		Find(&adminPolicies).Error)
	require.NotEmpty(t, adminPolicies)
	assert.True(t, hasPersistentPolicy(adminPolicies, ResourceChannel, ActionRead))
	assert.True(t, hasPersistentPolicy(adminPolicies, ResourceChannel, ActionWrite))
	assert.False(t, hasPersistentPolicy(adminPolicies, ResourceChannel, ActionSensitiveWrite))
	assert.True(t, hasPersistentPolicy(adminPolicies, ResourceChannelAccount, ActionRead))
	assert.True(t, hasPersistentPolicy(adminPolicies, ResourceChannelAccount, ActionOperate))
	assert.True(t, hasPersistentPolicy(adminPolicies, ResourceChannelAccount, ActionWrite))
	assert.False(t, hasPersistentPolicy(adminPolicies, ResourceChannelAccount, ActionSensitiveWrite))
	assert.True(t, hasPersistentPolicy(adminPolicies, ResourceAccountPoolAuthFile, ActionRead))
	assert.False(t, hasPersistentPolicy(adminPolicies, ResourceAccountPoolAuthFile, ActionSensitiveWrite))

	var rootPolicyCount int64
	require.NoError(t, db.Model(&model.CasbinRule{}).
		Where("ptype = ? AND v0 = ?", "p", RoleSubject(BuiltInRoleRoot)).
		Count(&rootPolicyCount).Error)
	assert.Zero(t, rootPolicyCount)
}

func TestReloadPersistentPoliciesRefreshesAdminBaseline(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())

	require.NoError(t, db.Where(
		"ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?",
		"p",
		RoleSubject(BuiltInRoleAdmin),
		ResourceChannel,
		ActionWrite,
	).Delete(&model.CasbinRule{}).Error)

	// 角色基线使用内存快照，数据库变更需要显式 reload 后才会影响授权判定。
	admin := requireRole(t, Roles(), BuiltInRoleAdmin)
	assert.True(t, admin.Grants[ResourceChannel][ActionRead])
	assert.True(t, admin.Grants[ResourceChannel][ActionWrite])
	assert.True(t, Can(42, common.RoleAdminUser, ChannelWrite))

	require.NoError(t, ReloadPersistentPolicies())

	admin = requireRole(t, Roles(), BuiltInRoleAdmin)
	assert.True(t, admin.Grants[ResourceChannel][ActionRead])
	assert.False(t, admin.Grants[ResourceChannel][ActionWrite])
	assert.True(t, Can(42, common.RoleAdminUser, ChannelRead))
	assert.False(t, Can(42, common.RoleAdminUser, ChannelWrite))
}

func TestUserOverridesWinOverPersistentRolePolicies(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())

	require.NoError(t, db.Where(
		"ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?",
		"p",
		RoleSubject(BuiltInRoleAdmin),
		ResourceChannel,
		ActionWrite,
	).Delete(&model.CasbinRule{}).Error)
	require.NoError(t, ReloadPersistentPolicies())

	require.NoError(t, SetUserPermissions(42, PermissionsMap{
		ResourceChannel: {
			ActionRead:  false,
			ActionWrite: true,
		},
	}))

	assert.False(t, Can(42, common.RoleAdminUser, ChannelRead))
	assert.True(t, Can(42, common.RoleAdminUser, ChannelWrite))

	var records []model.AuthzUserOverride
	require.NoError(t, db.Order("action ASC").Find(&records).Error)
	require.Len(t, records, 2)
	assertAuthzOverrideRecord(t, records[0], 42, ResourceChannel, ActionRead, EffectDeny)
	assertAuthzOverrideRecord(t, records[1], 42, ResourceChannel, ActionWrite, EffectAllow)
}

func TestPersistentPolicyFallbackWhenPolicyTableMissing(t *testing.T) {
	setupAuthzPolicyMissingTestDB(t)

	admin := requireRole(t, Roles(), BuiltInRoleAdmin)
	assert.True(t, admin.Grants[ResourceChannel][ActionRead])
	assert.True(t, admin.Grants[ResourceChannel][ActionWrite])
	assert.False(t, admin.Grants[ResourceChannel][ActionSensitiveWrite])
	assert.True(t, Can(42, common.RoleAdminUser, ChannelWrite))
	assert.False(t, Can(42, common.RoleAdminUser, ChannelSensitiveWrite))
}

func TestPersistentPolicyFallbackWhenNoValidPoliciesExist(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)

	require.NoError(t, db.Create(&model.CasbinRule{
		Ptype: "p",
		V0:    RoleSubject(BuiltInRoleAdmin),
		V1:    "unknown",
		V2:    ActionRead,
		V3:    EffectAllow,
	}).Error)

	admin := requireRole(t, Roles(), BuiltInRoleAdmin)
	assert.True(t, admin.Grants[ResourceChannel][ActionWrite])
	assert.True(t, Can(42, common.RoleAdminUser, ChannelWrite))
}

func hasPersistentPolicy(records []model.CasbinRule, resource string, action string) bool {
	for _, record := range records {
		if record.V1 == resource && record.V2 == action && record.V3 == EffectAllow {
			return true
		}
	}
	return false
}
