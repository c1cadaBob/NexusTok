package authz

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogIncludesNexusTokCoreResources(t *testing.T) {
	catalog := Catalog()
	require.Len(t, catalog, 7)

	resources := make(map[string]ResourceDefinition, len(catalog))
	for _, resource := range catalog {
		resources[resource.Resource] = resource
		assert.NotEmpty(t, resource.LabelKey)
		assert.NotEmpty(t, resource.Actions)
	}

	for _, name := range []string{"channel", "account_pool", "user", "model", "subscription", "redemption", "system_setting"} {
		_, ok := resources[name]
		assert.True(t, ok, "resource %s should be registered", name)
	}

	assert.Equal(t, "Channel Management", resources["channel"].LabelKey)
	assertActionExists(t, resources["channel"], ActionSensitiveWrite)
	assertActionExists(t, resources["account_pool"], ActionSecretView)
	assertActionExists(t, resources["redemption"], ActionSensitiveWrite)
	assertActionExists(t, resources["system_setting"], ActionSecretView)
}

func TestRolesExposeRootAndAdminBaselines(t *testing.T) {
	roles := Roles()
	require.Len(t, roles, 2)

	root := requireRole(t, roles, BuiltInRoleRoot)
	admin := requireRole(t, roles, BuiltInRoleAdmin)

	assert.True(t, root.Superuser)
	assert.True(t, root.Grants["channel"][ActionSensitiveWrite])
	assert.True(t, root.Grants["system_setting"][ActionSecretView])

	assert.False(t, admin.Superuser)
	assert.True(t, admin.Grants["channel"][ActionRead])
	assert.True(t, admin.Grants["channel"][ActionOperate])
	assert.True(t, admin.Grants["channel"][ActionWrite])
	assert.False(t, admin.Grants["channel"][ActionSensitiveWrite])
	assert.False(t, admin.Grants["channel"][ActionSecretView])
	assert.True(t, admin.Grants["account_pool"][ActionRead])
	assert.True(t, admin.Grants["account_pool"][ActionOperate])
	assert.True(t, admin.Grants["account_pool"][ActionWrite])
	assert.False(t, admin.Grants["account_pool"][ActionSensitiveWrite])
	assert.False(t, admin.Grants["account_pool"][ActionSecretView])
	assert.True(t, admin.Grants["user"][ActionRead])
	assert.True(t, admin.Grants["user"][ActionOperate])
	assert.True(t, admin.Grants["user"][ActionWrite])
	assert.False(t, admin.Grants["user"][ActionSensitiveWrite])
	assert.True(t, admin.Grants["model"][ActionRead])
	assert.True(t, admin.Grants["model"][ActionOperate])
	assert.True(t, admin.Grants["model"][ActionWrite])
	assert.False(t, admin.Grants["model"][ActionSensitiveWrite])
	assert.True(t, admin.Grants["subscription"][ActionRead])
	assert.True(t, admin.Grants["subscription"][ActionOperate])
	assert.True(t, admin.Grants["subscription"][ActionWrite])
	assert.False(t, admin.Grants["subscription"][ActionSensitiveWrite])
	assert.True(t, admin.Grants["redemption"][ActionRead])
	assert.True(t, admin.Grants["redemption"][ActionOperate])
	assert.True(t, admin.Grants["redemption"][ActionWrite])
	assert.False(t, admin.Grants["redemption"][ActionSensitiveWrite])
	assert.False(t, admin.Grants["system_setting"][ActionWrite])
}

func TestCapabilitiesFollowExistingSystemRoles(t *testing.T) {
	root := Capabilities(common.RoleRootUser)
	admin := Capabilities(common.RoleAdminUser)
	user := Capabilities(common.RoleCommonUser)

	assert.True(t, root["subscription"][ActionSensitiveWrite])
	assert.True(t, admin["subscription"][ActionWrite])
	assert.False(t, admin["subscription"][ActionSensitiveWrite])
	assert.True(t, admin["redemption"][ActionWrite])
	assert.False(t, admin["redemption"][ActionSensitiveWrite])
	assert.False(t, user["channel"][ActionRead])
	assert.False(t, user["user"][ActionRead])
	assert.False(t, user["model"][ActionRead])
	assert.False(t, user["redemption"][ActionRead])
	assert.False(t, user["system_setting"][ActionRead])
}

func TestCanFollowsRoleBaselinesAndFailsClosed(t *testing.T) {
	assert.True(t, Can(1, common.RoleRootUser, ChannelSensitiveWrite))
	assert.True(t, Can(1, common.RoleRootUser, ChannelSecretView))
	assert.True(t, Can(2, common.RoleAdminUser, ChannelRead))
	assert.True(t, Can(2, common.RoleAdminUser, ChannelOperate))
	assert.True(t, Can(2, common.RoleAdminUser, ChannelWrite))
	assert.False(t, Can(2, common.RoleAdminUser, ChannelSensitiveWrite))
	assert.False(t, Can(2, common.RoleAdminUser, ChannelSecretView))
	assert.True(t, Can(2, common.RoleAdminUser, AccountPoolRead))
	assert.True(t, Can(2, common.RoleAdminUser, AccountPoolOperate))
	assert.True(t, Can(2, common.RoleAdminUser, AccountPoolWrite))
	assert.False(t, Can(2, common.RoleAdminUser, AccountPoolSensitiveWrite))
	assert.False(t, Can(2, common.RoleAdminUser, AccountPoolSecretView))
	assert.True(t, Can(2, common.RoleAdminUser, UserRead))
	assert.True(t, Can(2, common.RoleAdminUser, UserOperate))
	assert.True(t, Can(2, common.RoleAdminUser, UserWrite))
	assert.False(t, Can(2, common.RoleAdminUser, UserSensitiveWrite))
	assert.True(t, Can(2, common.RoleAdminUser, ModelRead))
	assert.True(t, Can(2, common.RoleAdminUser, ModelOperate))
	assert.True(t, Can(2, common.RoleAdminUser, ModelWrite))
	assert.False(t, Can(2, common.RoleAdminUser, ModelSensitiveWrite))
	assert.True(t, Can(2, common.RoleAdminUser, SubscriptionRead))
	assert.True(t, Can(2, common.RoleAdminUser, SubscriptionOperate))
	assert.True(t, Can(2, common.RoleAdminUser, SubscriptionWrite))
	assert.False(t, Can(2, common.RoleAdminUser, SubscriptionSensitiveWrite))
	assert.True(t, Can(2, common.RoleAdminUser, RedemptionRead))
	assert.True(t, Can(2, common.RoleAdminUser, RedemptionOperate))
	assert.True(t, Can(2, common.RoleAdminUser, RedemptionWrite))
	assert.False(t, Can(2, common.RoleAdminUser, RedemptionSensitiveWrite))
	assert.False(t, Can(3, common.RoleCommonUser, ChannelRead))
	assert.False(t, Can(3, common.RoleCommonUser, AccountPoolRead))
	assert.False(t, Can(3, common.RoleCommonUser, UserRead))
	assert.False(t, Can(3, common.RoleCommonUser, ModelRead))
	assert.False(t, Can(3, common.RoleCommonUser, SubscriptionRead))
	assert.False(t, Can(3, common.RoleCommonUser, RedemptionRead))

	assert.False(t, Can(1, common.RoleRootUser, Permission{Resource: ResourceChannel, Action: "unknown"}))
	assert.False(t, Can(1, common.RoleRootUser, Permission{Resource: "unknown", Action: ActionRead}))
}

func assertActionExists(t *testing.T, resource ResourceDefinition, action string) {
	t.Helper()
	for _, item := range resource.Actions {
		if item.Action == action {
			return
		}
	}
	t.Fatalf("action %s should exist on resource %s", action, resource.Resource)
}

func requireRole(t *testing.T, roles []RoleDescriptor, key string) RoleDescriptor {
	t.Helper()
	for _, role := range roles {
		if role.Key == key {
			return role
		}
	}
	t.Fatalf("role %s should exist", key)
	return RoleDescriptor{}
}
