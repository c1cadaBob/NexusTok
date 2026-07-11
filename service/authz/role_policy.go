package authz

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/c1cada/NexusTok/model"
	"gorm.io/gorm"
)

var (
	errAuthzRoleKeyRequired      = errors.New("authz role key is required")
	errAuthzRoleNotFound         = errors.New("authz role not found")
	errAuthzRoleDisabled         = errors.New("authz role is disabled")
	errAuthzRootRoleReadOnly     = errors.New("root role policies are read-only")
	errAuthzRolePolicyEmptyAdmin = errors.New("admin role must keep at least one allow policy")
	errAuthzRoleBuiltInReadOnly  = errors.New("built-in authz roles cannot be changed")
	errAuthzRoleKeyInvalid       = errors.New("authz role key must start with a lowercase letter and contain only lowercase letters, numbers, underscores or dashes")
	errAuthzRoleKeyReserved      = errors.New("authz role key is reserved")
	errAuthzRoleAlreadyExists    = errors.New("authz role key already exists")
	errAuthzRoleNameRequired     = errors.New("authz role name is required")
)

var customRoleKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,62}$`)

// RolePolicyDescriptor 描述一个持久化角色及其当前权限矩阵。
//
// Root 是 superuser，不依赖 casbin_rule 中的 allow 策略；其它角色的 grants
// 来自 `casbin_rule` 中的 `role:<key>` 策略。自定义角色可分配给系统 Admin 用户作为
// 授权基线，RuntimeManaged 用于提示前端该角色是否会被授权热路径消费。
type RolePolicyDescriptor struct {
	Key            string         `json:"key"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	BuiltIn        bool           `json:"built_in"`
	Enabled        bool           `json:"enabled"`
	Sort           int            `json:"sort"`
	CreatedAt      int64          `json:"created_at"`
	UpdatedAt      int64          `json:"updated_at"`
	Superuser      bool           `json:"superuser"`
	RuntimeManaged bool           `json:"runtime_managed"`
	PolicyCount    int            `json:"policy_count"`
	Grants         PermissionsMap `json:"grants"`
}

// RolePolicyUpdateRequest 是角色策略编辑请求。
//
// DryRun 默认 true，调用方必须显式传入 false 才会写库。Grants 使用与 catalog
// 相同的 resource -> action -> allowed 矩阵；只有 true 会写入 allow 策略，false
// 或缺失都表示不授权。
type RolePolicyUpdateRequest struct {
	DryRun *bool          `json:"dry_run"`
	Grants PermissionsMap `json:"grants"`
}

// RolePolicyUpdateResult 返回角色策略更新的摘要。
type RolePolicyUpdateResult struct {
	RoleKey              string         `json:"role_key"`
	DryRun               bool           `json:"dry_run"`
	Applied              bool           `json:"applied"`
	Reloaded             bool           `json:"reloaded"`
	OldPolicyCount       int            `json:"old_policy_count"`
	NewPolicyCount       int            `json:"new_policy_count"`
	CreatedPolicyCount   int            `json:"created_policy_count"`
	DeletedPolicyCount   int            `json:"deleted_policy_count"`
	UnchangedPolicyCount int            `json:"unchanged_policy_count"`
	Grants               PermissionsMap `json:"grants"`
}

// RoleTemplateMutationRequest 是自定义角色模板的创建和元数据更新请求。
//
// Key 只在创建时使用；更新时角色 key 来自 URL，避免把重命名和策略迁移混在同
// 一次高风险操作里。Enabled 与 Sort 使用指针，确保调用方显式传入 false 或 0
// 时不会被误判为“未传字段”。
type RoleTemplateMutationRequest struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
	Sort        *int   `json:"sort"`
}

// RoleTemplateDeleteResult 返回删除角色模板后的清理摘要。
type RoleTemplateDeleteResult struct {
	RoleKey            string `json:"role_key"`
	DeletedPolicyCount int64  `json:"deleted_policy_count"`
	Reloaded           bool   `json:"reloaded"`
}

// PersistentRoles 返回持久化角色和角色策略矩阵。
//
// 该接口面向 Root 管理页展示当前数据库中的角色模板。数据库尚未完成种子时，
// 返回内置角色的静态描述，避免早期迁移环境无法打开权限页面。
func PersistentRoles() ([]RolePolicyDescriptor, error) {
	if model.DB == nil {
		return nil, errAuthzDatabaseNotInitialized
	}

	var roles []model.AuthzRole
	if err := model.DB.Order("sort ASC, key ASC").Find(&roles).Error; err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return fallbackBuiltInRolePolicies(), nil
	}

	result := make([]RolePolicyDescriptor, 0, len(roles))
	for _, role := range roles {
		descriptor, err := describePersistentRole(model.DB, role)
		if err != nil {
			return nil, err
		}
		result = append(result, descriptor)
	}
	return result, nil
}

// CreateRoleTemplate 创建一个可分配给系统 Admin 用户的自定义授权角色模板。
//
// 创建模板本身不会立即改变任何用户的授权结果；只有 Root 在用户编辑页显式分配该
// 角色后，它才会成为该管理员的权限基线。后续管理员可以通过角色策略矩阵维护 allow
// 策略，用户级 override 会继续作为相对该基线的差异保存。
func CreateRoleTemplate(req RoleTemplateMutationRequest) (*RolePolicyDescriptor, error) {
	if model.DB == nil {
		return nil, errAuthzDatabaseNotInitialized
	}

	roleKey, err := normalizeCustomRoleKey(req.Key)
	if err != nil {
		return nil, err
	}
	name, description, err := normalizeRoleTemplateText(req.Name, req.Description)
	if err != nil {
		return nil, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sortOrder := 0
	if req.Sort != nil {
		sortOrder = *req.Sort
	} else {
		sortOrder, err = nextCustomRoleSort()
		if err != nil {
			return nil, err
		}
	}

	var descriptor RolePolicyDescriptor
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var existing model.AuthzRole
		err := tx.Where("key = ?", roleKey).First(&existing).Error
		switch {
		case err == nil:
			return errAuthzRoleAlreadyExists
		case errors.Is(err, gorm.ErrRecordNotFound):
		default:
			return err
		}

		role := model.AuthzRole{
			Key:         roleKey,
			Name:        name,
			Description: description,
			BuiltIn:     false,
			Enabled:     enabled,
			Sort:        sortOrder,
		}
		if err := tx.Create(&role).Error; err != nil {
			return err
		}

		nextDescriptor, err := describePersistentRole(tx, role)
		if err != nil {
			return err
		}
		descriptor = nextDescriptor
		return nil
	}); err != nil {
		return nil, err
	}

	return &descriptor, nil
}

// UpdateRoleTemplate 更新非内置角色模板的元数据。
//
// 该操作只修改模板名称、说明、启用状态和排序，不修改 key，也不修改权限策略；
// 策略仍由 UpdateRolePolicies 通过 dry-run + 二次确认路径维护。
func UpdateRoleTemplate(roleKey string, req RoleTemplateMutationRequest) (*RolePolicyDescriptor, error) {
	if model.DB == nil {
		return nil, errAuthzDatabaseNotInitialized
	}

	normalizedKey := strings.ToLower(strings.TrimSpace(roleKey))
	if normalizedKey == "" {
		return nil, errAuthzRoleKeyRequired
	}
	name, description, err := normalizeRoleTemplateText(req.Name, req.Description)
	if err != nil {
		return nil, err
	}

	var descriptor RolePolicyDescriptor
	reloadAfterCommit := false
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var role model.AuthzRole
		err := tx.Where("key = ?", normalizedKey).First(&role).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errAuthzRoleNotFound
		}
		if err != nil {
			return err
		}
		if role.BuiltIn {
			return errAuthzRoleBuiltInReadOnly
		}

		if req.Enabled != nil && role.Enabled != *req.Enabled {
			if !*req.Enabled {
				if err := ensureRoleUnassignedInTx(tx, role.Key); err != nil {
					return err
				}
			}
			reloadAfterCommit = true
		}

		updates := map[string]interface{}{
			"name":        name,
			"description": description,
		}
		if req.Enabled != nil {
			updates["enabled"] = *req.Enabled
		}
		if req.Sort != nil {
			updates["sort"] = *req.Sort
		}
		if err := tx.Model(&role).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Where("key = ?", normalizedKey).First(&role).Error; err != nil {
			return err
		}

		nextDescriptor, err := describePersistentRole(tx, role)
		if err != nil {
			return err
		}
		descriptor = nextDescriptor
		return nil
	}); err != nil {
		return nil, err
	}
	if reloadAfterCommit {
		if err := ReloadPersistentPolicies(); err != nil {
			return nil, err
		}
	}

	return &descriptor, nil
}

// DeleteRoleTemplate 删除非内置角色模板，并同步清理该模板的策略记录。
//
// 删除模板不会影响 Root/Admin 等内置运行时角色；如果该模板已有策略，策略会在
// 同一个事务内删除，避免留下无法从管理页访问的孤儿 `role:<key>` 规则。
func DeleteRoleTemplate(roleKey string) (*RoleTemplateDeleteResult, error) {
	if model.DB == nil {
		return nil, errAuthzDatabaseNotInitialized
	}

	normalizedKey := strings.ToLower(strings.TrimSpace(roleKey))
	if normalizedKey == "" {
		return nil, errAuthzRoleKeyRequired
	}

	result := &RoleTemplateDeleteResult{RoleKey: normalizedKey}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		var role model.AuthzRole
		err := tx.Where("key = ?", normalizedKey).First(&role).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errAuthzRoleNotFound
		}
		if err != nil {
			return err
		}
		if role.BuiltIn {
			return errAuthzRoleBuiltInReadOnly
		}
		if err := ensureRoleUnassignedInTx(tx, role.Key); err != nil {
			return err
		}

		policyDelete := tx.Where("ptype = ? AND v0 = ?", "p", RoleSubject(role.Key)).
			Delete(&model.CasbinRule{})
		if policyDelete.Error != nil {
			return policyDelete.Error
		}
		result.DeletedPolicyCount = policyDelete.RowsAffected

		if err := tx.Delete(&role).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := ReloadPersistentPolicies(); err != nil {
		return nil, err
	}
	result.Reloaded = true
	return result, nil
}

// UpdateRolePolicies 校验并可选替换指定角色的 allow 策略。
//
// Root 角色永远只读；Admin 角色会影响当前系统管理员运行态能力，因此写入必须
// 通过 Root-only 路由进入。事务提交后立即 reload 持久策略快照，保证当前节点的
// `Can()` 与 `/api/user/self` 返回矩阵马上反映新策略。
func UpdateRolePolicies(roleKey string, req RolePolicyUpdateRequest) (*RolePolicyUpdateResult, error) {
	if model.DB == nil {
		return nil, errAuthzDatabaseNotInitialized
	}
	if roleKey == "" {
		return nil, errAuthzRoleKeyRequired
	}
	if roleKey == BuiltInRoleRoot {
		return nil, errAuthzRootRoleReadOnly
	}

	role, err := findPersistentRole(roleKey)
	if err != nil {
		return nil, err
	}
	if !role.Enabled {
		return nil, errAuthzRoleDisabled
	}

	nextGrants, err := normalizeRolePolicyGrants(req.Grants)
	if err != nil {
		return nil, err
	}
	nextPolicies := permissionsFromGrants(nextGrants)
	if role.Key == BuiltInRoleAdmin && len(nextPolicies) == 0 {
		return nil, errAuthzRolePolicyEmptyAdmin
	}

	currentPolicies, err := loadRolePolicySet(model.DB, role.Key)
	if err != nil {
		return nil, err
	}
	created, deleted, unchanged := diffPermissionSets(currentPolicies, nextPolicies)
	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}

	result := &RolePolicyUpdateResult{
		RoleKey:              role.Key,
		DryRun:               dryRun,
		Applied:              false,
		Reloaded:             false,
		OldPolicyCount:       len(currentPolicies),
		NewPolicyCount:       len(nextPolicies),
		CreatedPolicyCount:   created,
		DeletedPolicyCount:   deleted,
		UnchangedPolicyCount: unchanged,
		Grants:               nextGrants,
	}
	if dryRun {
		return result, nil
	}

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("ptype = ? AND v0 = ?", "p", RoleSubject(role.Key)).
			Delete(&model.CasbinRule{}).Error; err != nil {
			return err
		}
		if len(nextPolicies) == 0 {
			return nil
		}
		rules := make([]model.CasbinRule, 0, len(nextPolicies))
		for _, permission := range nextPolicies {
			rules = append(rules, model.CasbinRule{
				Ptype: "p",
				V0:    RoleSubject(role.Key),
				V1:    permission.Resource,
				V2:    permission.Action,
				V3:    EffectAllow,
			})
		}
		return tx.Create(&rules).Error
	}); err != nil {
		return nil, err
	}
	if err := ReloadPersistentPolicies(); err != nil {
		return nil, err
	}
	result.Applied = true
	result.Reloaded = true
	return result, nil
}

func fallbackBuiltInRolePolicies() []RolePolicyDescriptor {
	result := make([]RolePolicyDescriptor, 0, len(builtInRoles))
	for _, role := range builtInRoles {
		result = append(result, RolePolicyDescriptor{
			Key:            role.key,
			Name:           role.name,
			Description:    role.description,
			BuiltIn:        role.builtIn,
			Enabled:        true,
			Sort:           role.sort,
			CreatedAt:      0,
			UpdatedAt:      0,
			Superuser:      role.superuser,
			RuntimeManaged: true,
			PolicyCount:    len(permissionsFromGrants(roleGrants(role))),
			Grants:         roleGrants(role),
		})
	}
	return result
}

func describePersistentRole(db *gorm.DB, role model.AuthzRole) (RolePolicyDescriptor, error) {
	if spec, ok := builtInRoleByKey(role.Key); ok && spec.superuser {
		grants := roleGrants(*spec)
		return RolePolicyDescriptor{
			Key:            role.Key,
			Name:           role.Name,
			Description:    role.Description,
			BuiltIn:        role.BuiltIn,
			Enabled:        role.Enabled,
			Sort:           role.Sort,
			CreatedAt:      role.CreatedAt,
			UpdatedAt:      role.UpdatedAt,
			Superuser:      true,
			RuntimeManaged: true,
			PolicyCount:    0,
			Grants:         grants,
		}, nil
	}

	grants, count, err := loadRolePolicyGrants(db, role.Key)
	if err != nil {
		return RolePolicyDescriptor{}, err
	}
	if count == 0 {
		if spec, ok := builtInRoleByKey(role.Key); ok {
			grants = roleGrants(*spec)
			count = len(permissionsFromGrants(grants))
		}
	}
	runtimeManaged := role.Enabled
	return RolePolicyDescriptor{
		Key:            role.Key,
		Name:           role.Name,
		Description:    role.Description,
		BuiltIn:        role.BuiltIn,
		Enabled:        role.Enabled,
		Sort:           role.Sort,
		CreatedAt:      role.CreatedAt,
		UpdatedAt:      role.UpdatedAt,
		Superuser:      false,
		RuntimeManaged: runtimeManaged,
		PolicyCount:    count,
		Grants:         grants,
	}, nil
}

func findPersistentRole(roleKey string) (model.AuthzRole, error) {
	var role model.AuthzRole
	err := model.DB.Where("key = ?", roleKey).First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AuthzRole{}, errAuthzRoleNotFound
	}
	return role, err
}

func normalizeCustomRoleKey(input string) (string, error) {
	roleKey := strings.ToLower(strings.TrimSpace(input))
	if roleKey == "" {
		return "", errAuthzRoleKeyRequired
	}
	if roleKey == BuiltInRoleRoot || roleKey == BuiltInRoleAdmin ||
		strings.HasPrefix(roleKey, "role:") || strings.HasPrefix(roleKey, "user:") {
		return "", errAuthzRoleKeyReserved
	}
	if !customRoleKeyPattern.MatchString(roleKey) {
		return "", errAuthzRoleKeyInvalid
	}
	return roleKey, nil
}

func normalizeRoleTemplateText(name string, description string) (string, string, error) {
	normalizedName := strings.TrimSpace(name)
	if normalizedName == "" {
		return "", "", errAuthzRoleNameRequired
	}
	if len([]rune(normalizedName)) > 100 {
		return "", "", errors.New("authz role name must be at most 100 characters")
	}
	normalizedDescription := strings.TrimSpace(description)
	if len([]rune(normalizedDescription)) > 2000 {
		return "", "", errors.New("authz role description must be at most 2000 characters")
	}
	return normalizedName, normalizedDescription, nil
}

func nextCustomRoleSort() (int, error) {
	var lastRole model.AuthzRole
	err := model.DB.Order("sort DESC, key DESC").First(&lastRole).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 100, nil
	}
	if err != nil {
		return 0, err
	}
	return lastRole.Sort + 10, nil
}

func normalizeRolePolicyGrants(input PermissionsMap) (PermissionsMap, error) {
	normalized := emptyGrants()
	for resource, actions := range input {
		for action, allowed := range actions {
			permission := Permission{Resource: resource, Action: action}
			if !isKnownPermission(permission) {
				return nil, fmt.Errorf("authz role policy %s/%s is unknown", resource, action)
			}
			normalized[resource][action] = allowed
		}
	}
	return normalized, nil
}

func loadRolePolicyGrants(db *gorm.DB, roleKey string) (PermissionsMap, int, error) {
	policies, err := loadRolePolicySet(db, roleKey)
	if err != nil {
		return nil, 0, err
	}
	grants := emptyGrants()
	for _, permission := range policies {
		grants[permission.Resource][permission.Action] = true
	}
	return grants, len(policies), nil
}

func loadRolePolicySet(db *gorm.DB, roleKey string) ([]Permission, error) {
	var records []model.CasbinRule
	if err := db.Where("ptype = ? AND v0 = ?", "p", RoleSubject(roleKey)).
		Order("v1 ASC, v2 ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	policies := make([]Permission, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if record.V3 != "" && record.V3 != EffectAllow {
			continue
		}
		permission := Permission{Resource: record.V1, Action: record.V2}
		if !isKnownPermission(permission) {
			continue
		}
		key := permissionSortKey(permission)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		policies = append(policies, permission)
	}
	sortPermissions(policies)
	return policies, nil
}

func permissionsFromGrants(grants PermissionsMap) []Permission {
	policies := make([]Permission, 0)
	for resource, actions := range grants {
		for action, allowed := range actions {
			if !allowed {
				continue
			}
			policies = append(policies, Permission{Resource: resource, Action: action})
		}
	}
	sortPermissions(policies)
	return policies
}

func diffPermissionSets(oldPolicies []Permission, newPolicies []Permission) (int, int, int) {
	oldSet := permissionSet(oldPolicies)
	newSet := permissionSet(newPolicies)
	created := 0
	deleted := 0
	unchanged := 0
	for key := range newSet {
		if _, ok := oldSet[key]; ok {
			unchanged++
		} else {
			created++
		}
	}
	for key := range oldSet {
		if _, ok := newSet[key]; !ok {
			deleted++
		}
	}
	return created, deleted, unchanged
}

func permissionSet(policies []Permission) map[string]struct{} {
	result := make(map[string]struct{}, len(policies))
	for _, permission := range policies {
		result[permissionSortKey(permission)] = struct{}{}
	}
	return result
}

func sortPermissions(policies []Permission) {
	sort.Slice(policies, func(i, j int) bool {
		return permissionSortKey(policies[i]) < permissionSortKey(policies[j])
	})
}

func permissionSortKey(permission Permission) string {
	return permission.Resource + "\x00" + permission.Action
}
