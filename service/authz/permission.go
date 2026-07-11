// Package authz 提供 NexusTok 管理权限 catalog、角色基线和用户级 override 授权判定。
//
// 权限语义由前端、用户自身份权限回传和路由权限表共用：未知权限必须失败关闭，
// Root 始终是 superuser，Admin 可以通过用户级 allow/deny override 做细粒度分权。
package authz

// Permission 标识某个资源上的一个动作。
type Permission struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

// PermissionsMap 是 resource -> action -> allowed 的权限矩阵。
type PermissionsMap map[string]map[string]bool

const (
	BuiltInRoleRoot  = "root"
	BuiltInRoleAdmin = "admin"
)

const (
	EffectAllow = "allow"
	EffectDeny  = "deny"
)

const (
	ResourceChannel             = "channel"
	ResourceAccountPool         = "account_pool"
	ResourceAccountPoolAuthFile = "account_pool_auth_file"
	ResourceUser                = "user"
	ResourceModel               = "model"
	ResourceSubscription        = "subscription"
	ResourceRedemption          = "redemption"
	ResourceUsageLog            = "usage_log"
	ResourceUsageData           = "usage_data"
	ResourceSystemSetting       = "system_setting"
)

const (
	ActionRead           = "read"
	ActionOperate        = "operate"
	ActionWrite          = "write"
	ActionSensitiveWrite = "sensitive_write"
	ActionSecretView     = "secret_view"
)

var (
	ChannelRead           = Permission{Resource: ResourceChannel, Action: ActionRead}
	ChannelOperate        = Permission{Resource: ResourceChannel, Action: ActionOperate}
	ChannelWrite          = Permission{Resource: ResourceChannel, Action: ActionWrite}
	ChannelSensitiveWrite = Permission{Resource: ResourceChannel, Action: ActionSensitiveWrite}
	ChannelSecretView     = Permission{Resource: ResourceChannel, Action: ActionSecretView}

	AccountPoolRead           = Permission{Resource: ResourceAccountPool, Action: ActionRead}
	AccountPoolOperate        = Permission{Resource: ResourceAccountPool, Action: ActionOperate}
	AccountPoolWrite          = Permission{Resource: ResourceAccountPool, Action: ActionWrite}
	AccountPoolSensitiveWrite = Permission{Resource: ResourceAccountPool, Action: ActionSensitiveWrite}
	AccountPoolSecretView     = Permission{Resource: ResourceAccountPool, Action: ActionSecretView}

	AccountPoolAuthFileRead = Permission{Resource: ResourceAccountPoolAuthFile, Action: ActionRead}
	// AccountPoolAuthFileSensitiveWrite 保护导入、替换和删除认证文件这类会触碰凭证材料的入口。
	AccountPoolAuthFileSensitiveWrite = Permission{Resource: ResourceAccountPoolAuthFile, Action: ActionSensitiveWrite}

	UserRead           = Permission{Resource: ResourceUser, Action: ActionRead}
	UserOperate        = Permission{Resource: ResourceUser, Action: ActionOperate}
	UserWrite          = Permission{Resource: ResourceUser, Action: ActionWrite}
	UserSensitiveWrite = Permission{Resource: ResourceUser, Action: ActionSensitiveWrite}

	ModelRead           = Permission{Resource: ResourceModel, Action: ActionRead}
	ModelOperate        = Permission{Resource: ResourceModel, Action: ActionOperate}
	ModelWrite          = Permission{Resource: ResourceModel, Action: ActionWrite}
	ModelSensitiveWrite = Permission{Resource: ResourceModel, Action: ActionSensitiveWrite}

	SubscriptionRead           = Permission{Resource: ResourceSubscription, Action: ActionRead}
	SubscriptionOperate        = Permission{Resource: ResourceSubscription, Action: ActionOperate}
	SubscriptionWrite          = Permission{Resource: ResourceSubscription, Action: ActionWrite}
	SubscriptionSensitiveWrite = Permission{Resource: ResourceSubscription, Action: ActionSensitiveWrite}

	RedemptionRead           = Permission{Resource: ResourceRedemption, Action: ActionRead}
	RedemptionOperate        = Permission{Resource: ResourceRedemption, Action: ActionOperate}
	RedemptionWrite          = Permission{Resource: ResourceRedemption, Action: ActionWrite}
	RedemptionSensitiveWrite = Permission{Resource: ResourceRedemption, Action: ActionSensitiveWrite}
	RedemptionSecretView     = Permission{Resource: ResourceRedemption, Action: ActionSecretView}

	UsageLogRead           = Permission{Resource: ResourceUsageLog, Action: ActionRead}
	UsageLogSensitiveWrite = Permission{Resource: ResourceUsageLog, Action: ActionSensitiveWrite}

	UsageDataRead = Permission{Resource: ResourceUsageData, Action: ActionRead}

	SystemSettingRead           = Permission{Resource: ResourceSystemSetting, Action: ActionRead}
	SystemSettingOperate        = Permission{Resource: ResourceSystemSetting, Action: ActionOperate}
	SystemSettingWrite          = Permission{Resource: ResourceSystemSetting, Action: ActionWrite}
	SystemSettingSensitiveWrite = Permission{Resource: ResourceSystemSetting, Action: ActionSensitiveWrite}
	SystemSettingSecretView     = Permission{Resource: ResourceSystemSetting, Action: ActionSecretView}
)

// ActionDefinition 描述资源上的一个可授权动作。
type ActionDefinition struct {
	Action         string   `json:"action"`
	LabelKey       string   `json:"label_key"`
	DescriptionKey string   `json:"description_key"`
	DefaultRoles   []string `json:"-"`
}

// ResourceDefinition 描述一个管理资源及其可授权动作。
type ResourceDefinition struct {
	Resource string             `json:"resource"`
	LabelKey string             `json:"label_key"`
	Actions  []ActionDefinition `json:"actions"`
}

// RoleDescriptor 描述内置角色和它的基线授权矩阵。
type RoleDescriptor struct {
	Key       string         `json:"key"`
	Name      string         `json:"name"`
	BuiltIn   bool           `json:"built_in"`
	Superuser bool           `json:"superuser"`
	Grants    PermissionsMap `json:"grants"`
}

type roleSpec struct {
	key         string
	name        string
	description string
	builtIn     bool
	superuser   bool
	sort        int
}

var builtInRoles = []roleSpec{
	{
		key:         BuiltInRoleRoot,
		name:        "Root",
		description: "Built-in root authorization role",
		builtIn:     true,
		superuser:   true,
		sort:        0,
	},
	{
		key:         BuiltInRoleAdmin,
		name:        "Admin",
		description: "Built-in admin authorization role",
		builtIn:     true,
		superuser:   false,
		sort:        10,
	},
}

// Catalog 返回权限资源注册表的副本。
func Catalog() []ResourceDefinition {
	result := make([]ResourceDefinition, 0, len(registry))
	for _, resource := range registry {
		actions := make([]ActionDefinition, len(resource.Actions))
		copy(actions, resource.Actions)
		result = append(result, ResourceDefinition{
			Resource: resource.Resource,
			LabelKey: resource.LabelKey,
			Actions:  actions,
		})
	}
	return result
}

// Roles 返回内置角色及其基线授权矩阵。
func Roles() []RoleDescriptor {
	result := make([]RoleDescriptor, 0, len(builtInRoles))
	for _, role := range builtInRoles {
		result = append(result, RoleDescriptor{
			Key:       role.key,
			Name:      role.name,
			BuiltIn:   role.builtIn,
			Superuser: role.superuser,
			Grants:    roleGrants(role),
		})
	}
	return result
}

// Capabilities 按系统角色计算能力矩阵。
//
// 保留旧签名供没有用户上下文的调用点使用；存在用户上下文时应优先使用
// CapabilitiesForUser，让前端显隐和服务端 Can 共享用户级 override 语义。
func Capabilities(systemRole int) PermissionsMap {
	return CapabilitiesForUser(0, systemRole)
}

// CapabilitiesForUser 按系统角色和用户级 override 计算完整能力矩阵。
func CapabilitiesForUser(userID int, systemRole int) PermissionsMap {
	role := roleForSystemRole(systemRole)
	if role == nil {
		return emptyGrants()
	}
	if role.superuser {
		return roleGrants(*role)
	}
	overrides := safeUserOverrideEffects(userID)
	return roleGrantsWithOverrides(*role, overrides)
}

// Can 判断当前用户是否拥有指定资源动作权限。
//
// 未注册的资源或动作必须失败关闭，即使 Root 角色也不能绕过未知 permission。
// Admin 用户的显式 allow/deny override 优先于角色基线；普通用户没有管理角色，
// 即使数据库中误写 override 也不能被提升为管理员权限。
func Can(userID int, systemRole int, permission Permission) bool {
	action, ok := findAction(permission)
	if !ok {
		return false
	}
	role := roleForSystemRole(systemRole)
	if role == nil {
		return false
	}
	if role.superuser {
		return true
	}
	if allowed, ok := explicitUserOverride(userID, permission); ok {
		return allowed
	}
	if allowed, ok := persistentRoleAllows(*role, permission); ok {
		return allowed
	}
	return actionHasRole(action, role.key)
}

func roleForSystemRole(systemRole int) *roleSpec {
	for i := range builtInRoles {
		role := &builtInRoles[i]
		switch {
		case systemRole >= 100 && role.key == BuiltInRoleRoot:
			return role
		case systemRole >= 10 && systemRole < 100 && role.key == BuiltInRoleAdmin:
			return role
		}
	}
	return nil
}

func roleGrants(role roleSpec) PermissionsMap {
	return roleGrantsWithOverrides(role, nil)
}

func roleGrantsWithOverrides(role roleSpec, overrides overrideEffects) PermissionsMap {
	grants := make(PermissionsMap, len(registry))
	persistentGrants, persistentOK := persistentRoleGrants(role)
	for _, resource := range registry {
		actions := make(map[string]bool, len(resource.Actions))
		for _, action := range resource.Actions {
			permission := Permission{Resource: resource.Resource, Action: action.Action}
			if allowed, ok := overrideAllows(overrides, permission); ok {
				actions[action.Action] = allowed
				continue
			}
			if role.superuser {
				actions[action.Action] = true
				continue
			}
			if persistentOK {
				actions[action.Action] = persistentGrants[resource.Resource][action.Action]
				continue
			}
			actions[action.Action] = actionHasRole(action, role.key)
		}
		grants[resource.Resource] = actions
	}
	return grants
}

func builtInRoleByKey(roleKey string) (*roleSpec, bool) {
	for i := range builtInRoles {
		if builtInRoles[i].key == roleKey {
			return &builtInRoles[i], true
		}
	}
	return nil, false
}

func findAction(permission Permission) (ActionDefinition, bool) {
	for _, resource := range registry {
		if resource.Resource != permission.Resource {
			continue
		}
		for _, action := range resource.Actions {
			if action.Action == permission.Action {
				return action, true
			}
		}
		return ActionDefinition{}, false
	}
	return ActionDefinition{}, false
}

func emptyGrants() PermissionsMap {
	grants := make(PermissionsMap, len(registry))
	for _, resource := range registry {
		actions := make(map[string]bool, len(resource.Actions))
		for _, action := range resource.Actions {
			actions[action.Action] = false
		}
		grants[resource.Resource] = actions
	}
	return grants
}

func actionHasRole(action ActionDefinition, roleKey string) bool {
	for _, r := range action.DefaultRoles {
		if r == roleKey {
			return true
		}
	}
	return false
}
