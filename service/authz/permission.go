// Package authz 提供 NexusTok 管理权限 catalog 与基础授权判定。
//
// 当前阶段只维护资源、动作和内置角色基线，供前端、用户自身份权限回传和
// 渠道路由权限表共用；暂不在这里引入 Casbin 或用户级 override，避免一次迁移
// 影响账号池、订阅、系统设置等核心管理路径。
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
	ResourceChannel       = "channel"
	ResourceAccountPool   = "account_pool"
	ResourceUser          = "user"
	ResourceModel         = "model"
	ResourceSubscription  = "subscription"
	ResourceRedemption    = "redemption"
	ResourceUsageLog      = "usage_log"
	ResourceUsageData     = "usage_data"
	ResourceSystemSetting = "system_setting"
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
	key       string
	name      string
	builtIn   bool
	superuser bool
	sort      int
}

var builtInRoles = []roleSpec{
	{key: BuiltInRoleRoot, name: "Root", builtIn: true, superuser: true, sort: 0},
	{key: BuiltInRoleAdmin, name: "Admin", builtIn: true, superuser: false, sort: 10},
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

// Capabilities 按现有系统角色计算能力矩阵。
//
// 这个函数服务于前端按钮显隐、自身份权限回传和灰度路由权限表。当前仍只基于
// Root/Admin 系统角色基线；后续引入 Casbin 或用户级 override 时，应让它与 Can
// 继续共享同一套语义，避免页面显隐和服务端放行产生分叉。
func Capabilities(systemRole int) PermissionsMap {
	role := roleForSystemRole(systemRole)
	if role == nil {
		return emptyGrants()
	}
	return roleGrants(*role)
}

// Can 判断当前用户是否拥有指定资源动作权限。
//
// userID 预留给后续 Casbin/user override 使用；当前实现只读取系统角色基线。
// 未注册的资源或动作必须失败关闭，即使 Root 角色也不能绕过未知 permission，
// 这样新增管理资源时必须先进入 catalog 才能被路由权限表引用。
func Can(userID int, systemRole int, permission Permission) bool {
	_ = userID
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
	grants := make(PermissionsMap, len(registry))
	for _, resource := range registry {
		actions := make(map[string]bool, len(resource.Actions))
		for _, action := range resource.Actions {
			actions[action.Action] = role.superuser || actionHasRole(action, role.key)
		}
		grants[resource.Resource] = actions
	}
	return grants
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
