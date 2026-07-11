package authz

import (
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
)

const persistentPolicyExportVersion = "authz-policy-export/v1"

// PersistentPolicyExport 描述可迁移的权限策略快照。
//
// 导出内容只包含跨环境稳定字段：角色使用 key 作为业务标识，策略使用
// Casbin 兼容的 ptype/v0..v5 字段；数据库自增 ID、创建时间和更新时间都不导出，
// 避免备份在不同环境间产生不可复用的引用或无意义 diff。
type PersistentPolicyExport struct {
	Version     string                       `json:"version"`
	GeneratedAt int64                        `json:"generated_at"`
	Roles       []PersistentPolicyExportRole `json:"roles"`
	Policies    []PersistentPolicyExportRule `json:"policies"`
}

// PersistentPolicyExportRole 是 authz_roles 的可迁移角色模板。
type PersistentPolicyExportRole struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	BuiltIn     bool   `json:"built_in"`
	Enabled     bool   `json:"enabled"`
	Sort        int    `json:"sort"`
}

// PersistentPolicyExportRule 是 casbin_rule 的可迁移策略记录。
type PersistentPolicyExportRule struct {
	Ptype string `json:"ptype"`
	V0    string `json:"v0"`
	V1    string `json:"v1"`
	V2    string `json:"v2"`
	V3    string `json:"v3"`
	V4    string `json:"v4"`
	V5    string `json:"v5"`
}

// ExportPersistentPolicies 导出当前持久化角色与策略快照。
//
// 该函数只读数据库，不触发 seed、reload 或授权判定；排序固定为角色 sort/key、
// 策略 ptype/v0..v5，方便 Root 管理员把导出结果用于审计、备份和后续导入前 diff。
func ExportPersistentPolicies() (*PersistentPolicyExport, error) {
	if model.DB == nil {
		return nil, errAuthzDatabaseNotInitialized
	}

	var roles []model.AuthzRole
	if err := model.DB.Order("sort ASC, key ASC").Find(&roles).Error; err != nil {
		return nil, err
	}

	var policies []model.CasbinRule
	if err := model.DB.
		Order("ptype ASC, v0 ASC, v1 ASC, v2 ASC, v3 ASC, v4 ASC, v5 ASC").
		Find(&policies).Error; err != nil {
		return nil, err
	}

	export := &PersistentPolicyExport{
		Version:     persistentPolicyExportVersion,
		GeneratedAt: common.GetTimestamp(),
		Roles:       make([]PersistentPolicyExportRole, 0, len(roles)),
		Policies:    make([]PersistentPolicyExportRule, 0, len(policies)),
	}

	for _, role := range roles {
		export.Roles = append(export.Roles, PersistentPolicyExportRole{
			Key:         role.Key,
			Name:        role.Name,
			Description: role.Description,
			BuiltIn:     role.BuiltIn,
			Enabled:     role.Enabled,
			Sort:        role.Sort,
		})
	}

	for _, policy := range policies {
		export.Policies = append(export.Policies, PersistentPolicyExportRule{
			Ptype: policy.Ptype,
			V0:    policy.V0,
			V1:    policy.V1,
			V2:    policy.V2,
			V3:    policy.V3,
			V4:    policy.V4,
			V5:    policy.V5,
		})
	}

	return export, nil
}
