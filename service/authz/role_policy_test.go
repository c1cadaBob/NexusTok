package authz

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistentRolesReturnsStoredRolesAndGrants(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	require.NoError(t, db.Create(&model.AuthzRole{
		Key:         "auditor",
		Name:        "Auditor",
		Description: "Read-only audit role",
		Enabled:     true,
		Sort:        20,
	}).Error)
	require.NoError(t, db.Create(&model.CasbinRule{
		Ptype: "p",
		V0:    RoleSubject("auditor"),
		V1:    ResourceUsageLog,
		V2:    ActionRead,
		V3:    EffectAllow,
	}).Error)

	roles, err := PersistentRoles()
	require.NoError(t, err)
	require.Len(t, roles, 3)

	root := requirePersistentRole(t, roles, BuiltInRoleRoot)
	assert.True(t, root.Superuser)
	assert.True(t, root.RuntimeManaged)
	assert.True(t, root.Grants[ResourceSystemSetting][ActionSecretView])
	assert.Zero(t, root.PolicyCount)

	admin := requirePersistentRole(t, roles, BuiltInRoleAdmin)
	assert.False(t, admin.Superuser)
	assert.True(t, admin.RuntimeManaged)
	assert.True(t, admin.Grants[ResourceChannel][ActionWrite])
	assert.False(t, admin.Grants[ResourceChannel][ActionSensitiveWrite])
	assert.Greater(t, admin.PolicyCount, 0)
	assert.NotZero(t, admin.CreatedAt)
	assert.NotZero(t, admin.UpdatedAt)

	auditor := requirePersistentRole(t, roles, "auditor")
	assert.False(t, auditor.BuiltIn)
	assert.True(t, auditor.RuntimeManaged)
	assert.Equal(t, "Read-only audit role", auditor.Description)
	assert.True(t, auditor.Grants[ResourceUsageLog][ActionRead])
	assert.False(t, auditor.Grants[ResourceChannel][ActionRead])
	assert.Equal(t, 1, auditor.PolicyCount)
	assert.NotZero(t, auditor.CreatedAt)
	assert.NotZero(t, auditor.UpdatedAt)
}

func TestCreateRoleTemplateCreatesCustomTemplate(t *testing.T) {
	setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())

	role, err := CreateRoleTemplate(RoleTemplateMutationRequest{
		Key:         "Security_Audit",
		Name:        " Security Audit ",
		Description: " Read-only audit template ",
	})
	require.NoError(t, err)
	assert.Equal(t, "security_audit", role.Key)
	assert.Equal(t, "Security Audit", role.Name)
	assert.Equal(t, "Read-only audit template", role.Description)
	assert.False(t, role.BuiltIn)
	assert.True(t, role.Enabled)
	assert.True(t, role.RuntimeManaged)
	assert.Zero(t, role.PolicyCount)
	assert.False(t, role.Grants[ResourceChannel][ActionRead])
	assert.GreaterOrEqual(t, role.Sort, 10)
	assert.NotZero(t, role.CreatedAt)
	assert.NotZero(t, role.UpdatedAt)

	roles, err := PersistentRoles()
	require.NoError(t, err)
	created := requirePersistentRole(t, roles, "security_audit")
	assert.Equal(t, "Security Audit", created.Name)
	assert.True(t, created.RuntimeManaged)
}

func TestCreateRoleTemplateRejectsInvalidReservedAndDuplicateKeys(t *testing.T) {
	setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())

	_, err := CreateRoleTemplate(RoleTemplateMutationRequest{
		Key:  "root",
		Name: "Root clone",
	})
	require.ErrorIs(t, err, errAuthzRoleKeyReserved)

	_, err = CreateRoleTemplate(RoleTemplateMutationRequest{
		Key:  "1auditor",
		Name: "Auditor",
	})
	require.ErrorIs(t, err, errAuthzRoleKeyInvalid)

	_, err = CreateRoleTemplate(RoleTemplateMutationRequest{
		Key:  "auditor",
		Name: "Auditor",
	})
	require.NoError(t, err)

	_, err = CreateRoleTemplate(RoleTemplateMutationRequest{
		Key:  "auditor",
		Name: "Auditor duplicate",
	})
	require.ErrorIs(t, err, errAuthzRoleAlreadyExists)
}

func TestUpdateRoleTemplateUpdatesOnlyCustomRoleMetadata(t *testing.T) {
	setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	_, err := CreateRoleTemplate(RoleTemplateMutationRequest{
		Key:  "auditor",
		Name: "Auditor",
		Sort: intPtr(40),
	})
	require.NoError(t, err)

	updated, err := UpdateRoleTemplate("auditor", RoleTemplateMutationRequest{
		Name:        "External Auditor",
		Description: "Reviews usage and billing records",
		Enabled:     boolPtr(false),
		Sort:        intPtr(70),
	})
	require.NoError(t, err)
	assert.Equal(t, "auditor", updated.Key)
	assert.Equal(t, "External Auditor", updated.Name)
	assert.Equal(t, "Reviews usage and billing records", updated.Description)
	assert.False(t, updated.Enabled)
	assert.Equal(t, 70, updated.Sort)

	_, err = UpdateRolePolicies("auditor", RolePolicyUpdateRequest{
		Grants: PermissionsMap{
			ResourceUsageLog: {
				ActionRead: true,
			},
		},
	})
	require.ErrorIs(t, err, errAuthzRoleDisabled)
}

func TestUpdateRoleTemplateRejectsBuiltInRole(t *testing.T) {
	setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())

	_, err := UpdateRoleTemplate(BuiltInRoleAdmin, RoleTemplateMutationRequest{
		Name: "Custom Admin",
	})
	require.ErrorIs(t, err, errAuthzRoleBuiltInReadOnly)
}

func TestDeleteRoleTemplateRemovesTemplateAndPolicies(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	_, err := CreateRoleTemplate(RoleTemplateMutationRequest{
		Key:  "auditor",
		Name: "Auditor",
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.CasbinRule{
		Ptype: "p",
		V0:    RoleSubject("auditor"),
		V1:    ResourceUsageLog,
		V2:    ActionRead,
		V3:    EffectAllow,
	}).Error)

	result, err := DeleteRoleTemplate("auditor")
	require.NoError(t, err)
	assert.Equal(t, "auditor", result.RoleKey)
	assert.Equal(t, int64(1), result.DeletedPolicyCount)
	assert.True(t, result.Reloaded)

	roles, err := PersistentRoles()
	require.NoError(t, err)
	for _, role := range roles {
		assert.NotEqual(t, "auditor", role.Key)
	}

	var policyCount int64
	require.NoError(t, db.Model(&model.CasbinRule{}).
		Where("ptype = ? AND v0 = ?", "p", RoleSubject("auditor")).
		Count(&policyCount).Error)
	assert.Zero(t, policyCount)
}

func TestDeleteRoleTemplateRejectsBuiltInRole(t *testing.T) {
	setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())

	_, err := DeleteRoleTemplate(BuiltInRoleAdmin)
	require.ErrorIs(t, err, errAuthzRoleBuiltInReadOnly)
}

func TestDeleteRoleTemplateRejectsAssignedRole(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	_, err := CreateRoleTemplate(RoleTemplateMutationRequest{
		Key:  "auditor",
		Name: "Auditor",
	})
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.User{}).
		Where("id = ?", 42).
		Update("authz_role", "auditor").Error)

	_, err = DeleteRoleTemplate("auditor")

	require.ErrorIs(t, err, errAuthzRoleAssigned)
	var role model.AuthzRole
	require.NoError(t, db.Where("key = ?", "auditor").First(&role).Error)
}

func TestUpdateRolePoliciesDefaultsToDryRun(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())

	req := RolePolicyUpdateRequest{
		Grants: PermissionsMap{
			ResourceChannel: {
				ActionRead: true,
			},
		},
	}
	result, err := UpdateRolePolicies(BuiltInRoleAdmin, req)
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.False(t, result.Applied)
	assert.False(t, result.Reloaded)
	assert.Equal(t, 1, result.NewPolicyCount)
	assert.True(t, result.DeletedPolicyCount > 0)

	var policyCount int64
	require.NoError(t, db.Model(&model.CasbinRule{}).
		Where("ptype = ? AND v0 = ?", "p", RoleSubject(BuiltInRoleAdmin)).
		Count(&policyCount).Error)
	assert.Greater(t, policyCount, int64(1))
	assert.True(t, Can(7, common.RoleAdminUser, ChannelWrite))
}

func TestUpdateRolePoliciesAppliesAndReloadsAdminSnapshot(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())

	admin := requirePersistentRoleFromService(t, BuiltInRoleAdmin)
	admin.Grants[ResourceChannel][ActionWrite] = false
	req := RolePolicyUpdateRequest{
		DryRun: boolPtr(false),
		Grants: admin.Grants,
	}

	result, err := UpdateRolePolicies(BuiltInRoleAdmin, req)
	require.NoError(t, err)
	assert.False(t, result.DryRun)
	assert.True(t, result.Applied)
	assert.True(t, result.Reloaded)
	assert.Equal(t, 1, result.DeletedPolicyCount)
	assert.False(t, result.Grants[ResourceChannel][ActionWrite])
	assert.False(t, Can(7, common.RoleAdminUser, ChannelWrite))
	assert.True(t, Can(7, common.RoleAdminUser, ChannelRead))

	var policyCount int64
	require.NoError(t, db.Model(&model.CasbinRule{}).
		Where("ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?", "p", RoleSubject(BuiltInRoleAdmin), ResourceChannel, ActionWrite).
		Count(&policyCount).Error)
	assert.Zero(t, policyCount)
}

func TestUpdateRolePoliciesRejectsRootRole(t *testing.T) {
	setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())

	_, err := UpdateRolePolicies(BuiltInRoleRoot, RolePolicyUpdateRequest{
		DryRun: boolPtr(false),
		Grants: PermissionsMap{
			ResourceChannel: {
				ActionRead: true,
			},
		},
	})
	require.ErrorIs(t, err, errAuthzRootRoleReadOnly)
	assert.True(t, Can(1, common.RoleRootUser, ChannelSensitiveWrite))
}

func TestUpdateRolePoliciesRejectsUnknownPermission(t *testing.T) {
	setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())

	_, err := UpdateRolePolicies(BuiltInRoleAdmin, RolePolicyUpdateRequest{
		Grants: PermissionsMap{
			ResourceChannel: {
				"unknown": true,
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown")
}

func TestUpdateRolePoliciesRejectsDisabledRole(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	require.NoError(t, db.Create(&model.AuthzRole{
		Key:     "disabled",
		Name:    "Disabled",
		Enabled: false,
		Sort:    30,
	}).Error)

	_, err := UpdateRolePolicies("disabled", RolePolicyUpdateRequest{
		Grants: PermissionsMap{
			ResourceChannel: {
				ActionRead: true,
			},
		},
	})
	require.ErrorIs(t, err, errAuthzRoleDisabled)
}

func TestUpdateRoleTemplateRejectsDisablingAssignedRole(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	_, err := CreateRoleTemplate(RoleTemplateMutationRequest{
		Key:  "auditor",
		Name: "Auditor",
	})
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.User{}).
		Where("id = ?", 42).
		Update("authz_role", "auditor").Error)

	_, err = UpdateRoleTemplate("auditor", RoleTemplateMutationRequest{
		Name:    "Auditor",
		Enabled: boolPtr(false),
	})

	require.ErrorIs(t, err, errAuthzRoleAssigned)
	var role model.AuthzRole
	require.NoError(t, db.Where("key = ?", "auditor").First(&role).Error)
	assert.True(t, role.Enabled)
}

func requirePersistentRoleFromService(t *testing.T, key string) RolePolicyDescriptor {
	t.Helper()
	roles, err := PersistentRoles()
	require.NoError(t, err)
	return requirePersistentRole(t, roles, key)
}

func requirePersistentRole(t *testing.T, roles []RolePolicyDescriptor, key string) RolePolicyDescriptor {
	t.Helper()
	for _, role := range roles {
		if role.Key == key {
			return role
		}
	}
	t.Fatalf("persistent role %s should exist", key)
	return RolePolicyDescriptor{}
}

func intPtr(value int) *int {
	return &value
}
