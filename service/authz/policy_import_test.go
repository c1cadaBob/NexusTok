package authz

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportPersistentPoliciesDefaultsToDryRun(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	req := requirePolicyImportRequest(t)
	req.Roles = append(req.Roles, PersistentPolicyExportRole{
		Key:         "auditor",
		Name:        "Auditor",
		Description: "Imported read-only role",
		BuiltIn:     false,
		Enabled:     true,
		Sort:        20,
	})
	req.Policies = append(req.Policies, PersistentPolicyExportRule{
		Ptype: "p",
		V0:    RoleSubject("auditor"),
		V1:    ResourceChannel,
		V2:    ActionRead,
		V3:    EffectAllow,
	})

	result, err := ImportPersistentPolicies(req)
	require.NoError(t, err)
	assert.True(t, result.DryRun)
	assert.False(t, result.Applied)
	assert.Equal(t, "merge", result.Mode)
	assert.Equal(t, 1, result.CreatedRoleCount)
	assert.Equal(t, 1, result.CreatedRolePolicyCount)

	var roleCount int64
	require.NoError(t, db.Model(&model.AuthzRole{}).
		Where("key = ?", "auditor").
		Count(&roleCount).Error)
	assert.Zero(t, roleCount)
}

func TestImportPersistentPoliciesMergeAppliesRolesAndUserOverrides(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	req := requirePolicyImportRequest(t)
	req.DryRun = boolPtr(false)
	req.Roles = append(req.Roles, PersistentPolicyExportRole{
		Key:         "auditor",
		Name:        "Auditor",
		Description: "Imported read-only role",
		BuiltIn:     false,
		Enabled:     true,
		Sort:        20,
	})
	req.Policies = append(req.Policies,
		PersistentPolicyExportRule{
			Ptype: "p",
			V0:    RoleSubject("auditor"),
			V1:    ResourceChannel,
			V2:    ActionRead,
			V3:    EffectAllow,
		},
		PersistentPolicyExportRule{
			Ptype: "p",
			V0:    UserSubject(42),
			V1:    ResourceChannel,
			V2:    ActionSensitiveWrite,
			V3:    EffectAllow,
		},
	)

	result, err := ImportPersistentPolicies(req)
	require.NoError(t, err)
	assert.False(t, result.DryRun)
	assert.True(t, result.Applied)
	assert.True(t, result.Reloaded)
	assert.Equal(t, 1, result.CreatedRoleCount)
	assert.Equal(t, 1, result.CreatedRolePolicyCount)
	assert.Equal(t, 1, result.ImportedUserOverrideCount)

	var role model.AuthzRole
	require.NoError(t, db.Where("key = ?", "auditor").First(&role).Error)
	assert.Equal(t, "Auditor", role.Name)

	var policyCount int64
	require.NoError(t, db.Model(&model.CasbinRule{}).
		Where("ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?", "p", RoleSubject("auditor"), ResourceChannel, ActionRead).
		Count(&policyCount).Error)
	assert.EqualValues(t, 1, policyCount)

	assert.True(t, ExplicitUserOverrides(42)[ResourceChannel][ActionSensitiveWrite])
	require.NoError(t, db.Model(&model.CasbinRule{}).
		Where("ptype = ? AND v0 = ? AND v1 = ? AND v2 = ? AND v3 = ?", "p", UserSubject(42), ResourceChannel, ActionSensitiveWrite, EffectAllow).
		Count(&policyCount).Error)
	assert.EqualValues(t, 1, policyCount)
}

func TestImportPersistentPoliciesReplaceRequiresConfirmation(t *testing.T) {
	setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	req := requirePolicyImportRequest(t)
	req.DryRun = boolPtr(false)
	req.Mode = "replace"

	_, err := ImportPersistentPolicies(req)
	require.ErrorIs(t, err, errAuthzPolicyImportConfirmRequired)
	assert.True(t, Can(7, common.RoleAdminUser, ChannelWrite))
}

func TestImportPersistentPoliciesReplaceAppliesAndReloadsSnapshot(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	require.NoError(t, SetUserPermissions(42, PermissionsMap{
		ResourceChannel: {
			ActionSensitiveWrite: true,
		},
	}))

	req := requirePolicyImportRequest(t)
	req.DryRun = boolPtr(false)
	req.Mode = "replace"
	req.Confirm = persistentPolicyImportConfirm
	req.Policies = filterPolicies(req.Policies, func(policy PersistentPolicyExportRule) bool {
		if policy.V0 == UserSubject(42) {
			return false
		}
		return !(policy.V0 == RoleSubject(BuiltInRoleAdmin) &&
			policy.V1 == ResourceChannel &&
			policy.V2 == ActionWrite)
	})

	result, err := ImportPersistentPolicies(req)
	require.NoError(t, err)
	assert.True(t, result.Applied)
	assert.True(t, result.Reloaded)
	assert.Greater(t, result.DeletedRolePolicyCount, 0)
	assert.Equal(t, 1, result.ClearedUserOverrideCount)
	assert.False(t, Can(7, common.RoleAdminUser, ChannelWrite))
	assert.Empty(t, ExplicitUserOverrides(42))

	var policyCount int64
	require.NoError(t, db.Model(&model.CasbinRule{}).
		Where("ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?", "p", RoleSubject(BuiltInRoleAdmin), ResourceChannel, ActionWrite).
		Count(&policyCount).Error)
	assert.Zero(t, policyCount)
}

func TestImportPersistentPoliciesReplaceRejectsDeletingAssignedRole(t *testing.T) {
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

	req := requirePolicyImportRequest(t)
	req.Mode = "replace"
	req.Roles = filterRoles(req.Roles, func(role PersistentPolicyExportRole) bool {
		return role.Key != "auditor"
	})
	req.Policies = filterPolicies(req.Policies, func(policy PersistentPolicyExportRule) bool {
		return policy.V0 != RoleSubject("auditor")
	})

	_, err = ImportPersistentPolicies(req)
	require.ErrorIs(t, err, errAuthzRoleAssigned)
}

func TestImportPersistentPoliciesRejectsDisablingAssignedRole(t *testing.T) {
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

	req := requirePolicyImportRequest(t)
	for i := range req.Roles {
		if req.Roles[i].Key == "auditor" {
			req.Roles[i].Enabled = false
		}
	}

	_, err = ImportPersistentPolicies(req)
	require.ErrorIs(t, err, errAuthzRoleAssigned)
}

func TestImportPersistentPoliciesRejectsUnsafeBuiltInRoles(t *testing.T) {
	setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	req := requirePolicyImportRequest(t)
	for i := range req.Roles {
		if req.Roles[i].Key == BuiltInRoleRoot {
			req.Roles[i].Enabled = false
		}
	}

	_, err := ImportPersistentPolicies(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must stay built_in and enabled")
}

func TestImportPersistentPoliciesRejectsConflictingEffects(t *testing.T) {
	setupAuthzOverrideTestDB(t)
	require.NoError(t, SeedPersistentPolicies())
	req := requirePolicyImportRequest(t)
	req.Policies = append(req.Policies,
		PersistentPolicyExportRule{
			Ptype: "p",
			V0:    UserSubject(42),
			V1:    ResourceChannel,
			V2:    ActionRead,
			V3:    EffectAllow,
		},
		PersistentPolicyExportRule{
			Ptype: "p",
			V0:    UserSubject(42),
			V1:    ResourceChannel,
			V2:    ActionRead,
			V3:    EffectDeny,
		},
	)

	_, err := ImportPersistentPolicies(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting effects")
}

func requirePolicyImportRequest(t *testing.T) PersistentPolicyImportRequest {
	t.Helper()
	export, err := ExportPersistentPolicies()
	require.NoError(t, err)
	return PersistentPolicyImportRequest{
		Version:     export.Version,
		GeneratedAt: export.GeneratedAt,
		Roles:       append([]PersistentPolicyExportRole(nil), export.Roles...),
		Policies:    append([]PersistentPolicyExportRule(nil), export.Policies...),
	}
}

func filterPolicies(policies []PersistentPolicyExportRule, keep func(PersistentPolicyExportRule) bool) []PersistentPolicyExportRule {
	filtered := make([]PersistentPolicyExportRule, 0, len(policies))
	for _, policy := range policies {
		if keep(policy) {
			filtered = append(filtered, policy)
		}
	}
	return filtered
}

func filterRoles(roles []PersistentPolicyExportRole, keep func(PersistentPolicyExportRole) bool) []PersistentPolicyExportRole {
	filtered := make([]PersistentPolicyExportRole, 0, len(roles))
	for _, role := range roles {
		if keep(role) {
			filtered = append(filtered, role)
		}
	}
	return filtered
}

func boolPtr(value bool) *bool {
	return &value
}
