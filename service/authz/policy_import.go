package authz

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/model"
	"gorm.io/gorm"
)

const persistentPolicyImportConfirm = "IMPORT_AUTHZ_POLICIES"

var (
	errAuthzPolicyImportVersionRequired = errors.New("authz policy import version is required")
	errAuthzPolicyImportInvalidVersion  = errors.New("authz policy import version is invalid")
	errAuthzPolicyImportConfirmRequired = errors.New("authz policy import replace mode requires confirmation")
)

// PersistentPolicyImportRequest 描述策略导入请求。
//
// 请求体兼容导出接口返回的 JSON，并额外支持 dry_run、mode 与 confirm。dry_run
// 默认为 true，调用方必须显式传入 false 才会写库；replace 模式会重建角色策略并
// 同步用户 override，必须额外传入确认字符串，避免误操作清空权限策略。
type PersistentPolicyImportRequest struct {
	Version     string                       `json:"version"`
	GeneratedAt int64                        `json:"generated_at"`
	Roles       []PersistentPolicyExportRole `json:"roles"`
	Policies    []PersistentPolicyExportRule `json:"policies"`
	DryRun      *bool                        `json:"dry_run,omitempty"`
	Mode        string                       `json:"mode,omitempty"`
	Confirm     string                       `json:"confirm,omitempty"`
}

// PersistentPolicyImportResult 返回导入校验和应用摘要。
type PersistentPolicyImportResult struct {
	DryRun                    bool   `json:"dry_run"`
	Mode                      string `json:"mode"`
	Applied                   bool   `json:"applied"`
	Reloaded                  bool   `json:"reloaded"`
	RoleCount                 int    `json:"role_count"`
	PolicyCount               int    `json:"policy_count"`
	RolePolicyCount           int    `json:"role_policy_count"`
	UserPolicyCount           int    `json:"user_policy_count"`
	DuplicatePolicyCount      int    `json:"duplicate_policy_count"`
	CreatedRoleCount          int    `json:"created_role_count"`
	UpdatedRoleCount          int    `json:"updated_role_count"`
	DeletedRoleCount          int    `json:"deleted_role_count"`
	CreatedRolePolicyCount    int    `json:"created_role_policy_count"`
	DeletedRolePolicyCount    int    `json:"deleted_role_policy_count"`
	ImportedUserOverrideCount int    `json:"imported_user_override_count"`
	ClearedUserOverrideCount  int    `json:"cleared_user_override_count"`
}

type normalizedPolicyImport struct {
	roles                []PersistentPolicyExportRole
	rolePolicies         []PersistentPolicyExportRule
	userOverridePolicies map[int]map[string][]model.AuthzUserOverride
	duplicatePolicies    int
}

// ImportPersistentPolicies 校验并可选应用持久化权限策略快照。
//
// mode=merge 只新增/更新角色并补充缺失角色策略；导入的用户策略会按用户+资源替换
// 对应 override。mode=replace 会替换所有 role:* 策略、清理导入中缺失的非内置角色，
// 并把用户 override 重建为导入快照。两种写入都会在事务成功后 ReloadPersistentPolicies。
func ImportPersistentPolicies(req PersistentPolicyImportRequest) (*PersistentPolicyImportResult, error) {
	if model.DB == nil {
		return nil, errAuthzDatabaseNotInitialized
	}

	dryRun := true
	if req.DryRun != nil {
		dryRun = *req.DryRun
	}
	mode := normalizePolicyImportMode(req.Mode)
	if mode != "merge" && mode != "replace" {
		return nil, fmt.Errorf("unsupported authz policy import mode %q", req.Mode)
	}
	if mode == "replace" && !dryRun && req.Confirm != persistentPolicyImportConfirm {
		return nil, errAuthzPolicyImportConfirmRequired
	}

	normalized, err := normalizePersistentPolicyImport(req)
	if err != nil {
		return nil, err
	}
	if err := validatePersistentPolicyImportAssignments(normalized, mode); err != nil {
		return nil, err
	}

	result, err := buildPersistentPolicyImportResult(normalized, dryRun, mode)
	if err != nil {
		return nil, err
	}
	if dryRun {
		return result, nil
	}

	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := applyPersistentPolicyRoleImport(tx, normalized.roles, mode); err != nil {
			return err
		}
		if err := applyPersistentRolePolicyImport(tx, normalized.rolePolicies, mode); err != nil {
			return err
		}
		return applyPersistentUserPolicyImport(tx, normalized.userOverridePolicies, mode)
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

func normalizePolicyImportMode(mode string) string {
	trimmed := strings.TrimSpace(strings.ToLower(mode))
	if trimmed == "" {
		return "merge"
	}
	return trimmed
}

func normalizePersistentPolicyImport(req PersistentPolicyImportRequest) (*normalizedPolicyImport, error) {
	if strings.TrimSpace(req.Version) == "" {
		return nil, errAuthzPolicyImportVersionRequired
	}
	if strings.TrimSpace(req.Version) != persistentPolicyExportVersion {
		return nil, errAuthzPolicyImportInvalidVersion
	}

	roles, roleKeys, err := normalizeImportRoles(req.Roles)
	if err != nil {
		return nil, err
	}

	policies, duplicatePolicyCount, err := dedupeImportPolicies(req.Policies)
	if err != nil {
		return nil, err
	}
	if err := rejectConflictingImportPolicies(policies); err != nil {
		return nil, err
	}

	imported := &normalizedPolicyImport{
		roles:                roles,
		rolePolicies:         make([]PersistentPolicyExportRule, 0, len(policies)),
		userOverridePolicies: make(map[int]map[string][]model.AuthzUserOverride),
		duplicatePolicies:    duplicatePolicyCount,
	}
	for _, policy := range policies {
		switch {
		case strings.HasPrefix(policy.V0, "role:"):
			if err := validateRolePolicy(policy, roleKeys); err != nil {
				return nil, err
			}
			imported.rolePolicies = append(imported.rolePolicies, policy)
		case strings.HasPrefix(policy.V0, "user:"):
			override, err := normalizeUserPolicyOverride(policy)
			if err != nil {
				return nil, err
			}
			if imported.userOverridePolicies[override.UserID] == nil {
				imported.userOverridePolicies[override.UserID] = make(map[string][]model.AuthzUserOverride)
			}
			imported.userOverridePolicies[override.UserID][override.Resource] = append(
				imported.userOverridePolicies[override.UserID][override.Resource],
				override,
			)
		default:
			return nil, fmt.Errorf("unsupported authz policy subject %q", policy.V0)
		}
	}
	sortUserOverrides(imported.userOverridePolicies)
	return imported, nil
}

func normalizeImportRoles(roles []PersistentPolicyExportRole) ([]PersistentPolicyExportRole, map[string]bool, error) {
	result := make([]PersistentPolicyExportRole, 0, len(roles))
	roleKeys := make(map[string]bool, len(roles))
	requiredBuiltIns := map[string]bool{
		BuiltInRoleRoot:  false,
		BuiltInRoleAdmin: false,
	}

	for _, role := range roles {
		nextRole := PersistentPolicyExportRole{
			Key:         strings.TrimSpace(role.Key),
			Name:        strings.TrimSpace(role.Name),
			Description: role.Description,
			BuiltIn:     role.BuiltIn,
			Enabled:     role.Enabled,
			Sort:        role.Sort,
		}
		if nextRole.Key == "" {
			return nil, nil, errors.New("authz policy import role key is required")
		}
		if nextRole.Name == "" {
			return nil, nil, fmt.Errorf("authz policy import role %q name is required", nextRole.Key)
		}
		if len(nextRole.Key) > 64 {
			return nil, nil, fmt.Errorf("authz policy import role %q key is too long", nextRole.Key)
		}
		if len(nextRole.Name) > 100 {
			return nil, nil, fmt.Errorf("authz policy import role %q name is too long", nextRole.Key)
		}
		if roleKeys[nextRole.Key] {
			return nil, nil, fmt.Errorf("authz policy import role %q is duplicated", nextRole.Key)
		}
		if _, ok := requiredBuiltIns[nextRole.Key]; ok {
			if !nextRole.BuiltIn || !nextRole.Enabled {
				return nil, nil, fmt.Errorf("authz built-in role %q must stay built_in and enabled", nextRole.Key)
			}
			requiredBuiltIns[nextRole.Key] = true
		}
		roleKeys[nextRole.Key] = true
		result = append(result, nextRole)
	}

	for roleKey, exists := range requiredBuiltIns {
		if !exists {
			return nil, nil, fmt.Errorf("authz built-in role %q is required", roleKey)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Sort != result[j].Sort {
			return result[i].Sort < result[j].Sort
		}
		return result[i].Key < result[j].Key
	})
	return result, roleKeys, nil
}

func dedupeImportPolicies(policies []PersistentPolicyExportRule) ([]PersistentPolicyExportRule, int, error) {
	seen := make(map[string]bool, len(policies))
	result := make([]PersistentPolicyExportRule, 0, len(policies))
	duplicateCount := 0

	for _, policy := range policies {
		nextPolicy := PersistentPolicyExportRule{
			Ptype: strings.TrimSpace(policy.Ptype),
			V0:    strings.TrimSpace(policy.V0),
			V1:    strings.TrimSpace(policy.V1),
			V2:    strings.TrimSpace(policy.V2),
			V3:    strings.TrimSpace(policy.V3),
			V4:    strings.TrimSpace(policy.V4),
			V5:    strings.TrimSpace(policy.V5),
		}
		if err := validateImportPolicyFieldLengths(nextPolicy); err != nil {
			return nil, 0, err
		}
		if nextPolicy.Ptype != "p" {
			return nil, 0, fmt.Errorf("unsupported authz policy ptype %q", nextPolicy.Ptype)
		}
		key := importPolicyKey(nextPolicy)
		if seen[key] {
			duplicateCount++
			continue
		}
		seen[key] = true
		result = append(result, nextPolicy)
	}
	sort.Slice(result, func(i, j int) bool {
		return importPolicyKey(result[i]) < importPolicyKey(result[j])
	})
	return result, duplicateCount, nil
}

func rejectConflictingImportPolicies(policies []PersistentPolicyExportRule) error {
	semanticKeys := make(map[string]string, len(policies))
	for _, policy := range policies {
		// 同一 subject/resource/action 同时出现 allow 与 deny 会让策略语义依赖
		// 运行时解析顺序；用户 override 表也无法保存这种冲突，因此导入阶段直接拒绝。
		semanticKey := strings.Join([]string{policy.Ptype, policy.V0, policy.V1, policy.V2}, "\x00")
		fullKey := importPolicyKey(policy)
		if previous, ok := semanticKeys[semanticKey]; ok && previous != fullKey {
			return fmt.Errorf("authz policy %s/%s/%s has conflicting effects", policy.V0, policy.V1, policy.V2)
		}
		semanticKeys[semanticKey] = fullKey
	}
	return nil
}

func validateImportPolicyFieldLengths(policy PersistentPolicyExportRule) error {
	fields := []struct {
		name  string
		value string
	}{
		{name: "ptype", value: policy.Ptype},
		{name: "v0", value: policy.V0},
		{name: "v1", value: policy.V1},
		{name: "v2", value: policy.V2},
		{name: "v3", value: policy.V3},
		{name: "v4", value: policy.V4},
		{name: "v5", value: policy.V5},
	}
	for _, field := range fields {
		if field.value == "" && field.name != "v3" && field.name != "v4" && field.name != "v5" {
			return fmt.Errorf("authz policy %s is required", field.name)
		}
		if len(field.value) > 100 {
			return fmt.Errorf("authz policy %s is too long", field.name)
		}
	}
	return nil
}

func validateRolePolicy(policy PersistentPolicyExportRule, roleKeys map[string]bool) error {
	roleKey := strings.TrimPrefix(policy.V0, "role:")
	if roleKey == "" || !roleKeys[roleKey] {
		return fmt.Errorf("authz role policy subject %q has no imported role", policy.V0)
	}
	if !isKnownPermission(Permission{Resource: policy.V1, Action: policy.V2}) {
		return fmt.Errorf("authz role policy %s/%s is unknown", policy.V1, policy.V2)
	}
	if policy.V3 != "" && policy.V3 != EffectAllow && policy.V3 != EffectDeny {
		return fmt.Errorf("authz role policy effect %q is invalid", policy.V3)
	}
	return nil
}

func normalizeUserPolicyOverride(policy PersistentPolicyExportRule) (model.AuthzUserOverride, error) {
	userID, err := strconv.Atoi(strings.TrimPrefix(policy.V0, "user:"))
	if err != nil || userID <= 0 {
		return model.AuthzUserOverride{}, fmt.Errorf("authz user policy subject %q is invalid", policy.V0)
	}
	if !isKnownPermission(Permission{Resource: policy.V1, Action: policy.V2}) {
		return model.AuthzUserOverride{}, fmt.Errorf("authz user policy %s/%s is unknown", policy.V1, policy.V2)
	}
	if policy.V3 != EffectAllow && policy.V3 != EffectDeny {
		return model.AuthzUserOverride{}, fmt.Errorf("authz user policy effect %q is invalid", policy.V3)
	}
	return model.AuthzUserOverride{
		UserID:   userID,
		Resource: policy.V1,
		Action:   policy.V2,
		Effect:   policy.V3,
	}, nil
}

func sortUserOverrides(overrides map[int]map[string][]model.AuthzUserOverride) {
	for userID := range overrides {
		for resource := range overrides[userID] {
			sort.Slice(overrides[userID][resource], func(i, j int) bool {
				return overrides[userID][resource][i].Action < overrides[userID][resource][j].Action
			})
		}
	}
}

func buildPersistentPolicyImportResult(imported *normalizedPolicyImport, dryRun bool, mode string) (*PersistentPolicyImportResult, error) {
	existingRoles, err := loadExistingAuthzRoles()
	if err != nil {
		return nil, err
	}
	existingRolePolicies, err := loadExistingRolePolicyKeys()
	if err != nil {
		return nil, err
	}

	result := &PersistentPolicyImportResult{
		DryRun:               dryRun,
		Mode:                 mode,
		RoleCount:            len(imported.roles),
		RolePolicyCount:      len(imported.rolePolicies),
		UserPolicyCount:      countImportedUserOverrides(imported.userOverridePolicies),
		DuplicatePolicyCount: imported.duplicatePolicies,
	}
	result.PolicyCount = result.RolePolicyCount + result.UserPolicyCount

	importRoleKeys := make(map[string]bool, len(imported.roles))
	for _, role := range imported.roles {
		importRoleKeys[role.Key] = true
		existingRole, ok := existingRoles[role.Key]
		if !ok {
			result.CreatedRoleCount++
			continue
		}
		if authzRoleChanged(existingRole, role) {
			result.UpdatedRoleCount++
		}
	}
	if mode == "replace" {
		for _, existingRole := range existingRoles {
			if existingRole.BuiltIn {
				continue
			}
			if !importRoleKeys[existingRole.Key] {
				result.DeletedRoleCount++
			}
		}
		result.DeletedRolePolicyCount = len(existingRolePolicies)
		result.CreatedRolePolicyCount = len(imported.rolePolicies)
		currentOverrides, err := countExistingUserOverrides()
		if err != nil {
			return nil, err
		}
		result.ClearedUserOverrideCount = currentOverrides
	} else {
		for _, policy := range imported.rolePolicies {
			if !existingRolePolicies[importPolicyKey(policy)] {
				result.CreatedRolePolicyCount++
			}
		}
	}
	result.ImportedUserOverrideCount = result.UserPolicyCount
	return result, nil
}

func validatePersistentPolicyImportAssignments(imported *normalizedPolicyImport, mode string) error {
	existingRoles, err := loadExistingAuthzRoles()
	if err != nil {
		return err
	}
	importRoleKeys := make(map[string]PersistentPolicyExportRole, len(imported.roles))
	for _, role := range imported.roles {
		importRoleKeys[role.Key] = role
		if existing, ok := existingRoles[role.Key]; ok && existing.Enabled && !role.Enabled {
			if err := ensureRoleUnassignedInTx(model.DB, role.Key); err != nil {
				return err
			}
		}
	}
	if mode != "replace" {
		return nil
	}
	for _, existingRole := range existingRoles {
		if existingRole.BuiltIn {
			continue
		}
		if _, ok := importRoleKeys[existingRole.Key]; !ok {
			if err := ensureRoleUnassignedInTx(model.DB, existingRole.Key); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadExistingAuthzRoles() (map[string]model.AuthzRole, error) {
	var roles []model.AuthzRole
	if err := model.DB.Find(&roles).Error; err != nil {
		return nil, err
	}
	result := make(map[string]model.AuthzRole, len(roles))
	for _, role := range roles {
		result[role.Key] = role
	}
	return result, nil
}

func loadExistingRolePolicyKeys() (map[string]bool, error) {
	var policies []model.CasbinRule
	if err := model.DB.Where("ptype = ? AND v0 LIKE ?", "p", "role:%").Find(&policies).Error; err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(policies))
	for _, policy := range policies {
		result[importPolicyKey(PersistentPolicyExportRule{
			Ptype: policy.Ptype,
			V0:    policy.V0,
			V1:    policy.V1,
			V2:    policy.V2,
			V3:    policy.V3,
			V4:    policy.V4,
			V5:    policy.V5,
		})] = true
	}
	return result, nil
}

func countExistingUserOverrides() (int, error) {
	var count int64
	if err := model.DB.Model(&model.AuthzUserOverride{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func authzRoleChanged(existing model.AuthzRole, imported PersistentPolicyExportRole) bool {
	return existing.Name != imported.Name ||
		existing.Description != imported.Description ||
		existing.BuiltIn != imported.BuiltIn ||
		existing.Enabled != imported.Enabled ||
		existing.Sort != imported.Sort
}

func countImportedUserOverrides(overrides map[int]map[string][]model.AuthzUserOverride) int {
	count := 0
	for _, resources := range overrides {
		for _, records := range resources {
			count += len(records)
		}
	}
	return count
}

func applyPersistentPolicyRoleImport(tx *gorm.DB, roles []PersistentPolicyExportRole, mode string) error {
	importRoleKeys := make([]string, 0, len(roles))
	for _, role := range roles {
		importRoleKeys = append(importRoleKeys, role.Key)
		var existing model.AuthzRole
		err := tx.Where("key = ?", role.Key).First(&existing).Error
		switch {
		case err == nil:
			if existing.Enabled && !role.Enabled {
				if err := ensureRoleUnassignedInTx(tx, existing.Key); err != nil {
					return err
				}
			}
			if err := tx.Model(&existing).Updates(map[string]interface{}{
				"name":        role.Name,
				"description": role.Description,
				"built_in":    role.BuiltIn,
				"enabled":     role.Enabled,
				"sort":        role.Sort,
			}).Error; err != nil {
				return err
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Create(&model.AuthzRole{
				Key:         role.Key,
				Name:        role.Name,
				Description: role.Description,
				BuiltIn:     role.BuiltIn,
				Enabled:     role.Enabled,
				Sort:        role.Sort,
			}).Error; err != nil {
				return err
			}
		default:
			return err
		}
	}
	if mode != "replace" {
		return nil
	}
	var staleRoles []model.AuthzRole
	if err := tx.Where("built_in = ? AND key NOT IN ?", false, importRoleKeys).
		Find(&staleRoles).Error; err != nil {
		return err
	}
	for _, role := range staleRoles {
		if err := ensureRoleUnassignedInTx(tx, role.Key); err != nil {
			return err
		}
	}
	return tx.Where("built_in = ? AND key NOT IN ?", false, importRoleKeys).
		Delete(&model.AuthzRole{}).Error
}

func applyPersistentRolePolicyImport(tx *gorm.DB, policies []PersistentPolicyExportRule, mode string) error {
	if mode == "replace" {
		if err := tx.Where("ptype = ? AND v0 LIKE ?", "p", "role:%").
			Delete(&model.CasbinRule{}).Error; err != nil {
			return err
		}
		return createCasbinRulesInTx(tx, policies)
	}

	existingKeys, err := loadExistingRolePolicyKeysInTx(tx)
	if err != nil {
		return err
	}
	missingPolicies := make([]PersistentPolicyExportRule, 0, len(policies))
	for _, policy := range policies {
		if existingKeys[importPolicyKey(policy)] {
			continue
		}
		missingPolicies = append(missingPolicies, policy)
	}
	return createCasbinRulesInTx(tx, missingPolicies)
}

func loadExistingRolePolicyKeysInTx(tx *gorm.DB) (map[string]bool, error) {
	var policies []model.CasbinRule
	if err := tx.Where("ptype = ? AND v0 LIKE ?", "p", "role:%").Find(&policies).Error; err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(policies))
	for _, policy := range policies {
		result[importPolicyKey(PersistentPolicyExportRule{
			Ptype: policy.Ptype,
			V0:    policy.V0,
			V1:    policy.V1,
			V2:    policy.V2,
			V3:    policy.V3,
			V4:    policy.V4,
			V5:    policy.V5,
		})] = true
	}
	return result, nil
}

func createCasbinRulesInTx(tx *gorm.DB, policies []PersistentPolicyExportRule) error {
	if len(policies) == 0 {
		return nil
	}
	rules := make([]model.CasbinRule, 0, len(policies))
	for _, policy := range policies {
		rules = append(rules, model.CasbinRule{
			Ptype: policy.Ptype,
			V0:    policy.V0,
			V1:    policy.V1,
			V2:    policy.V2,
			V3:    policy.V3,
			V4:    policy.V4,
			V5:    policy.V5,
		})
	}
	return tx.Create(&rules).Error
}

func applyPersistentUserPolicyImport(tx *gorm.DB, overrides map[int]map[string][]model.AuthzUserOverride, mode string) error {
	if mode == "replace" {
		userIDs, err := existingAuthzUserOverrideIDsInTx(tx)
		if err != nil {
			return err
		}
		for _, userID := range userIDs {
			if err := model.ClearAuthzUserOverridesInTx(tx, userID); err != nil {
				return err
			}
		}
	}

	userIDs := make([]int, 0, len(overrides))
	for userID := range overrides {
		userIDs = append(userIDs, userID)
	}
	sort.Ints(userIDs)
	for _, userID := range userIDs {
		resources := make([]string, 0, len(overrides[userID]))
		for resource := range overrides[userID] {
			resources = append(resources, resource)
		}
		sort.Strings(resources)
		for _, resource := range resources {
			if err := model.ReplaceAuthzUserResourceOverridesInTx(tx, userID, resource, overrides[userID][resource]); err != nil {
				return err
			}
		}
	}
	return nil
}

func existingAuthzUserOverrideIDsInTx(tx *gorm.DB) ([]int, error) {
	var records []model.AuthzUserOverride
	if err := tx.Select("user_id").Group("user_id").Find(&records).Error; err != nil {
		return nil, err
	}
	userIDs := make([]int, 0, len(records))
	for _, record := range records {
		userIDs = append(userIDs, record.UserID)
	}
	sort.Ints(userIDs)
	return userIDs, nil
}

func importPolicyKey(policy PersistentPolicyExportRule) string {
	return strings.Join([]string{
		policy.Ptype,
		policy.V0,
		policy.V1,
		policy.V2,
		policy.V3,
		policy.V4,
		policy.V5,
	}, "\x00")
}
