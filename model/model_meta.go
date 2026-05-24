// Package model - model_meta.go
// 该文件定义了模型元数据（Model）数据模型及相关操作
//
// 主要结构体：
// - Model：模型元数据（名称规则、显示名称、绑定渠道等）
// - BoundChannel：绑定的渠道信息
//
// 常量定义：
// - NameRuleExact：精确匹配
// - NameRulePrefix：前缀匹配
// - NameRuleContains：包含匹配
// - NameRuleSuffix：后缀匹配
//
// 核心功能：
// - 模型元数据的增删改查
// - 模型名称规则匹配
// - 模型与渠道的绑定关系管理
package model

import (
	"strconv"

	"github.com/c1cada/NexusTok/common"

	"gorm.io/gorm"
)

// NameRule 模型名称匹配规则常量
const (
	NameRuleExact   = iota // 精确匹配
	NameRulePrefix         // 前缀匹配
	NameRuleContains       // 包含匹配
	NameRuleSuffix         // 后缀匹配
)

// BoundChannel 绑定的渠道信息
// 用于展示模型关联的渠道列表
type BoundChannel struct {
	Name string `json:"name"` // 渠道名称
	Type int    `json:"type"` // 渠道类型
}

// Model 模型元数据
// 存储模型的配置信息，包括名称规则、显示名称、绑定渠道等
type Model struct {
	Id           int            `json:"id"`                                                        // 模型 ID
	ModelName    string         `json:"model_name" gorm:"size:128;not null;uniqueIndex:uk_model_name_delete_at,priority:1"` // 模型名称（唯一索引）
	Description  string         `json:"description,omitempty" gorm:"type:text"`                    // 模型描述
	Icon         string         `json:"icon,omitempty" gorm:"type:varchar(128)"`                   // 模型图标
	Tags         string         `json:"tags,omitempty" gorm:"type:varchar(255)"`                   // 模型标签（逗号分隔）
	VendorID     int            `json:"vendor_id,omitempty" gorm:"index"`                          // 供应商 ID
	Endpoints    string         `json:"endpoints,omitempty" gorm:"type:text"`                      // 支持的端点类型（JSON 格式）
	Status       int            `json:"status" gorm:"default:1"`                                   // 模型状态（1=启用，2=禁用）
	SyncOfficial int            `json:"sync_official" gorm:"default:1"`                            // 是否同步官方数据
	CreatedTime  int64          `json:"created_time" gorm:"bigint"`                                // 创建时间
	UpdatedTime  int64          `json:"updated_time" gorm:"bigint"`                                // 更新时间
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index;uniqueIndex:uk_model_name_delete_at,priority:2"` // 软删除时间

	BoundChannels []BoundChannel `json:"bound_channels,omitempty" gorm:"-"` // 绑定的渠道列表（非持久化，运行时附加）
	EnableGroups  []string       `json:"enable_groups,omitempty" gorm:"-"`  // 启用的分组列表（非持久化，运行时附加）
	QuotaTypes    []int          `json:"quota_types,omitempty" gorm:"-"`    // 计费类型列表（非持久化，运行时附加）
	NameRule      int            `json:"name_rule" gorm:"default:0"`        // 名称匹配规则（0=精确，1=前缀，2=包含，3=后缀）

	MatchedModels []string `json:"matched_models,omitempty" gorm:"-"` // 匹配的模型列表（非持久化，运行时附加）
	MatchedCount  int      `json:"matched_count,omitempty" gorm:"-"`  // 匹配的模型数量（非持久化，运行时附加）
}

// Insert 创建新的模型元数据记录
// 自动设置创建时间和更新时间
//
// 返回值：
//   - error: 创建失败时返回错误
func (mi *Model) Insert() error {
	now := common.GetTimestamp()
	mi.CreatedTime = now
	mi.UpdatedTime = now

	// 保存原始值（因为 Create 后可能被 GORM 的 default 标签覆盖为 1）
	originalStatus := mi.Status
	originalSyncOfficial := mi.SyncOfficial

	// 先创建记录（GORM 会对零值字段应用默认值）
	if err := DB.Create(mi).Error; err != nil {
		return err
	}

	// 使用保存的原始值进行更新，确保零值能正确保存
	return DB.Model(&Model{}).Where("id = ?", mi.Id).Updates(map[string]interface{}{
		"status":        originalStatus,
		"sync_official": originalSyncOfficial,
	}).Error
}

// IsModelNameDuplicated 检查模型名称是否重复（排除自身 ID）
//
// 参数：
//   - id: 当前模型 ID（排除自身）
//   - name: 要检查的模型名称
//
// 返回值：
//   - bool: 名称重复返回 true
//   - error: 查询失败时返回错误
func IsModelNameDuplicated(id int, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	var cnt int64
	err := DB.Model(&Model{}).Where("model_name = ? AND id <> ?", name, id).Count(&cnt).Error
	return cnt > 0, err
}

// Update 更新模型元数据记录
// 使用 Select 强制更新所有字段，包括零值
//
// 返回值：
//   - error: 更新失败时返回错误
func (mi *Model) Update() error {
	mi.UpdatedTime = common.GetTimestamp()
	// 使用 Select 强制更新所有字段，包括零值
	return DB.Model(&Model{}).Where("id = ?", mi.Id).
		Select("model_name", "description", "icon", "tags", "vendor_id", "endpoints", "status", "sync_official", "name_rule", "updated_time").
		Updates(mi).Error
}

// Delete 软删除模型元数据记录
//
// 返回值：
//   - error: 删除失败时返回错误
func (mi *Model) Delete() error {
	return DB.Delete(mi).Error
}

// GetVendorModelCounts 获取每个供应商的模型数量统计
//
// 返回值：
//   - map[int64]int64: 供应商 ID -> 模型数量的映射
//   - error: 查询失败时返回错误
func GetVendorModelCounts() (map[int64]int64, error) {
	var stats []struct {
		VendorID int64
		Count    int64
	}
	if err := DB.Model(&Model{}).
		Select("vendor_id as vendor_id, count(*) as count").
		Group("vendor_id").
		Scan(&stats).Error; err != nil {
		return nil, err
	}
	m := make(map[int64]int64, len(stats))
	for _, s := range stats {
		m[s.VendorID] = s.Count
	}
	return m, nil
}

// GetAllModels 分页获取所有模型元数据
//
// 参数：
//   - offset: 偏移量
//   - limit: 每页数量
//
// 返回值：
//   - []*Model: 模型列表
//   - error: 查询失败时返回错误
func GetAllModels(offset int, limit int) ([]*Model, error) {
	var models []*Model
	err := DB.Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error
	return models, err
}

// GetBoundChannelsByModelsMap 批量获取模型绑定的渠道信息
// 联合查询 abilities 和 channels 表，返回模型名称到绑定渠道列表的映射
//
// 参数：
//   - modelNames: 模型名称列表
//
// 返回值：
//   - map[string][]BoundChannel: 模型名称 -> 绑定渠道列表的映射
//   - error: 查询失败时返回错误
func GetBoundChannelsByModelsMap(modelNames []string) (map[string][]BoundChannel, error) {
	result := make(map[string][]BoundChannel)
	if len(modelNames) == 0 {
		return result, nil
	}
	type row struct {
		Model string
		Name  string
		Type  int
	}
	var rows []row
	err := DB.Table("channels").
		Select("abilities.model as model, channels.name as name, channels.type as type").
		Joins("JOIN abilities ON abilities.channel_id = channels.id").
		Where("abilities.model IN ? AND abilities.enabled = ?", modelNames, true).
		Distinct().
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.Model] = append(result[r.Model], BoundChannel{Name: r.Name, Type: r.Type})
	}
	return result, nil
}

// SearchModels 搜索模型元数据
// 支持按关键词（模型名称、描述、标签）和供应商筛选
//
// 参数：
//   - keyword: 搜索关键词
//   - vendor: 供应商名称或 ID
//   - offset: 偏移量
//   - limit: 每页数量
//
// 返回值：
//   - []*Model: 模型列表
//   - int64: 总数
//   - error: 查询失败时返回错误
func SearchModels(keyword string, vendor string, offset int, limit int) ([]*Model, int64, error) {
	var models []*Model
	db := DB.Model(&Model{})
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("model_name LIKE ? OR description LIKE ? OR tags LIKE ?", like, like, like)
	}
	if vendor != "" {
		if vid, err := strconv.Atoi(vendor); err == nil {
			db = db.Where("models.vendor_id = ?", vid)
		} else {
			db = db.Joins("JOIN vendors ON vendors.id = models.vendor_id").Where("vendors.name LIKE ?", "%"+vendor+"%")
		}
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := db.Order("models.id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	return models, total, nil
}
