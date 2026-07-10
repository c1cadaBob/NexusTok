// Package model - casbin_rule.go
// 该文件保存管理权限策略规则。
//
// NexusTok 当前不引入 Casbin 运行时依赖，但保留与 new-api/casbin-adapter 兼容的
// casbin_rule 表结构：ptype + v0..v5。service/authz 会把角色策略写成
// p, role:<key>, <resource>, <action>, allow，为后续完整策略编辑和导入导出留出空间。
package model

// CasbinRule 表示一条通用授权策略规则。
//
// 唯一索引覆盖 ptype 和 v0..v5，避免重复写入相同策略。空字符串是有效的索引组成
// 部分，和 Casbin GORM adapter 的常见表结构保持一致。
type CasbinRule struct {
	ID    int64  `json:"id" gorm:"primaryKey"`
	Ptype string `json:"ptype" gorm:"type:varchar(100);index:idx_casbin_rule,priority:1;uniqueIndex:idx_casbin_rule_unique,priority:1"`
	V0    string `json:"v0" gorm:"type:varchar(100);index:idx_casbin_rule,priority:2;uniqueIndex:idx_casbin_rule_unique,priority:2"`
	V1    string `json:"v1" gorm:"type:varchar(100);index:idx_casbin_rule,priority:3;uniqueIndex:idx_casbin_rule_unique,priority:3"`
	V2    string `json:"v2" gorm:"type:varchar(100);index:idx_casbin_rule,priority:4;uniqueIndex:idx_casbin_rule_unique,priority:4"`
	V3    string `json:"v3" gorm:"type:varchar(100);index:idx_casbin_rule,priority:5;uniqueIndex:idx_casbin_rule_unique,priority:5"`
	V4    string `json:"v4" gorm:"type:varchar(100);index:idx_casbin_rule,priority:6;uniqueIndex:idx_casbin_rule_unique,priority:6"`
	V5    string `json:"v5" gorm:"type:varchar(100);index:idx_casbin_rule,priority:7;uniqueIndex:idx_casbin_rule_unique,priority:7"`
}

// TableName 固定表名，保持与 new-api 的 casbin_rule 表兼容。
func (CasbinRule) TableName() string {
	return "casbin_rule"
}
