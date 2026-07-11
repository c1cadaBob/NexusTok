package authz

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShadowEnforcerLoadsRolePolicies(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	resetShadowEnforcerForTest(t)

	require.NoError(t, SeedPersistentPolicies())
	require.NoError(t, InitShadowEnforcer(db))

	allowed, ok, err := ShadowRoleAllows(BuiltInRoleAdmin, ChannelRead)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.True(t, allowed)

	allowed, ok, err = ShadowRoleAllows(BuiltInRoleAdmin, ChannelSensitiveWrite)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.False(t, allowed)

	mismatches, err := CompareShadowRolePolicies()
	require.NoError(t, err)
	assert.Empty(t, mismatches)
}

func TestReloadShadowPolicyDoesNotChangeCanSnapshot(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	resetShadowEnforcerForTest(t)

	require.NoError(t, SeedPersistentPolicies())
	require.NoError(t, InitShadowEnforcer(db))

	assert.True(t, Can(42, common.RoleAdminUser, ChannelWrite))

	require.NoError(t, db.Where(
		"ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?",
		"p",
		RoleSubject(BuiltInRoleAdmin),
		ResourceChannel,
		ActionWrite,
	).Delete(&model.CasbinRule{}).Error)
	require.NoError(t, ReloadShadowPolicy())

	shadowAllowed, ok, err := ShadowRoleAllows(BuiltInRoleAdmin, ChannelWrite)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.False(t, shadowAllowed)

	// 影子 reload 只刷新 Casbin 观测视图，不会改变当前真实授权快照。
	assert.True(t, Can(42, common.RoleAdminUser, ChannelWrite))

	require.NoError(t, ReloadPersistentPolicies())
	assert.False(t, Can(42, common.RoleAdminUser, ChannelWrite))
}

func TestShadowEnforcerTreatsLegacyEmptyEffectAsAllow(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	resetShadowEnforcerForTest(t)

	require.NoError(t, SeedPersistentPolicies())
	require.NoError(t, db.Where(
		"ptype = ? AND v0 = ? AND v1 = ? AND v2 = ?",
		"p",
		RoleSubject(BuiltInRoleAdmin),
		ResourceChannel,
		ActionRead,
	).Delete(&model.CasbinRule{}).Error)
	require.NoError(t, db.Create(&model.CasbinRule{
		Ptype: "p",
		V0:    RoleSubject(BuiltInRoleAdmin),
		V1:    ResourceChannel,
		V2:    ActionRead,
	}).Error)

	require.NoError(t, InitShadowEnforcer(db))
	allowed, ok, err := ShadowRoleAllows(BuiltInRoleAdmin, ChannelRead)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.True(t, allowed)
}

func TestCompareShadowRolePoliciesReportsFallbackMismatch(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	resetShadowEnforcerForTest(t)

	require.NoError(t, InitShadowEnforcer(db))
	mismatches, err := CompareShadowRolePolicies()
	require.NoError(t, err)
	require.NotEmpty(t, mismatches)

	assert.Contains(t, mismatches, ShadowPolicyMismatch{
		RoleKey:  BuiltInRoleAdmin,
		Resource: ResourceChannel,
		Action:   ActionRead,
		Expected: true,
		Shadow:   false,
	})
}

func TestCompareShadowRolePolicyStatusReportsUnavailableWithoutEnforcer(t *testing.T) {
	setupAuthzOverrideTestDB(t)
	resetShadowEnforcerForTest(t)

	comparison, err := CompareShadowRolePolicyStatus()
	require.NoError(t, err)
	require.NotNil(t, comparison)
	assert.False(t, comparison.Available)
	assert.Zero(t, comparison.MismatchCount)
	assert.Empty(t, comparison.Mismatches)
}

func TestCompareShadowRolePolicyStatusReturnsMismatchCount(t *testing.T) {
	db := setupAuthzOverrideTestDB(t)
	resetShadowEnforcerForTest(t)

	require.NoError(t, InitShadowEnforcer(db))
	comparison, err := CompareShadowRolePolicyStatus()
	require.NoError(t, err)
	require.NotNil(t, comparison)
	assert.True(t, comparison.Available)
	assert.Equal(t, len(comparison.Mismatches), comparison.MismatchCount)
	assert.NotEmpty(t, comparison.Mismatches)
}

func resetShadowEnforcerForTest(t *testing.T) {
	t.Helper()

	shadowEnforcerMu.Lock()
	previous := shadowEnforcer
	shadowEnforcer = nil
	shadowEnforcerMu.Unlock()

	t.Cleanup(func() {
		shadowEnforcerMu.Lock()
		shadowEnforcer = previous
		shadowEnforcerMu.Unlock()
	})
}
