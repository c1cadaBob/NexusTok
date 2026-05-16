package model

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/c1cada/NexusTok/common"
	"gorm.io/gorm"
)

// RequestRule 请求规则，用于按 RelayFormat 和模型名独立配置覆写和请求记录
type RequestRule struct {
	Id          int            `json:"id"`
	Name        string         `json:"name" gorm:"size:128;not null"`
	Description string         `json:"description,omitempty" gorm:"type:text"`
	Status      int            `json:"status" gorm:"default:1;index"`   // 1=启用, 0=禁用
	Priority    int            `json:"priority" gorm:"default:0;index"` // 越大越先执行

	// 匹配条件
	RelayFormat    string `json:"relay_format" gorm:"size:64;index;default:''"` // 空=匹配全部格式
	ModelPattern   string `json:"model_pattern" gorm:"size:255;default:'"`      // 模型匹配模式，空=匹配全部
	ModelMatchMode int    `json:"model_match_mode" gorm:"default:0"`            // 0=精确, 1=前缀, 2=包含, 3=后缀, 4=通配符

	// 覆写配置（格式与 Channel.ParamOverride 完全一致，复用 relay/common/override.go 引擎）
	ParamOverride  *string `json:"param_override" gorm:"type:text"`
	HeaderOverride *string `json:"header_override" gorm:"type:text"`

	// 请求记录配置
	LogRequest  bool `json:"log_request" gorm:"default:false"`  // 是否记录请求体
	LogResponse bool `json:"log_response" gorm:"default:false"` // 是否记录响应体
	LogMaxSize  int  `json:"log_max_size" gorm:"default:4096"`  // 单条记录最大字节数，0=不限制

	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

// 模型匹配模式常量
const (
	ModelMatchExact    = 0 // 精确匹配
	ModelMatchPrefix   = 1 // 前缀匹配
	ModelMatchContains = 2 // 包含匹配
	ModelMatchSuffix   = 3 // 后缀匹配
	ModelMatchWildcard = 4 // 通配符匹配（支持 * 和 ?）
)

// --- 内存缓存：缓存启用状态的规则列表，避免每次请求都查库 ---

var (
	enabledRulesCache []*RequestRule
	enabledRulesMu    sync.RWMutex
)

// InitRequestRuleCache 初始化请求规则缓存（服务启动时调用）
func InitRequestRuleCache() {
	RefreshRequestRuleCache()
}

// RefreshRequestRuleCache 刷新请求规则缓存
func RefreshRequestRuleCache() {
	var rules []*RequestRule
	err := DB.Where("status = ?", 1).Order("priority DESC, id ASC").Find(&rules).Error
	if err != nil {
		common.SysError(fmt.Sprintf("刷新请求规则缓存失败: %v", err))
		return
	}
	enabledRulesMu.Lock()
	enabledRulesCache = rules
	enabledRulesMu.Unlock()
}

// GetEnabledRequestRules 获取所有启用的规则（从缓存读取）
func GetEnabledRequestRules() []*RequestRule {
	enabledRulesMu.RLock()
	defer enabledRulesMu.RUnlock()
	return enabledRulesCache
}

// --- CRUD 方法 ---

func (r *RequestRule) Insert() error {
	now := common.GetTimestamp()
	r.CreatedTime = now
	r.UpdatedTime = now
	err := DB.Create(r).Error
	if err == nil {
		RefreshRequestRuleCache()
	}
	return err
}

func (r *RequestRule) Update() error {
	r.UpdatedTime = common.GetTimestamp()
	err := DB.Save(r).Error
	if err == nil {
		RefreshRequestRuleCache()
	}
	return err
}

func (r *RequestRule) Delete() error {
	err := DB.Delete(r).Error
	if err == nil {
		RefreshRequestRuleCache()
	}
	return err
}

// UpdateStatus 快速切换启用/禁用状态
func (r *RequestRule) UpdateStatus(status int) error {
	err := DB.Model(r).Update("status", status).Error
	if err == nil {
		RefreshRequestRuleCache()
	}
	return err
}

func GetRequestRuleByID(id int) (*RequestRule, error) {
	var rule RequestRule
	err := DB.First(&rule, id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// GetAllRequestRules 分页获取规则列表，支持按 status 和 relay_format 过滤
func GetAllRequestRules(offset int, limit int, status int, relayFormat string) ([]*RequestRule, int64, error) {
	db := DB.Model(&RequestRule{})
	if status >= 0 {
		db = db.Where("status = ?", status)
	}
	if relayFormat != "" {
		db = db.Where("relay_format = ?", relayFormat)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rules []*RequestRule
	if err := db.Offset(offset).Limit(limit).Order("priority DESC, id DESC").Find(&rules).Error; err != nil {
		return nil, 0, err
	}
	return rules, total, nil
}

// SearchRequestRules 按关键词搜索规则（匹配名称或模型模式）
func SearchRequestRules(keyword string, offset int, limit int) ([]*RequestRule, int64, error) {
	db := DB.Model(&RequestRule{})
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("name LIKE ? OR model_pattern LIKE ? OR description LIKE ?", like, like, like)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rules []*RequestRule
	if err := db.Offset(offset).Limit(limit).Order("priority DESC, id DESC").Find(&rules).Error; err != nil {
		return nil, 0, err
	}
	return rules, total, nil
}

// --- Override 解析方法（与 Channel 模式一致）---

// GetParamOverride 解析参数覆写配置为 map
func (r *RequestRule) GetParamOverride() map[string]interface{} {
	result := make(map[string]interface{})
	if r.ParamOverride != nil && *r.ParamOverride != "" {
		err := common.Unmarshal([]byte(*r.ParamOverride), &result)
		if err != nil {
			common.SysLog(fmt.Sprintf("解析请求规则参数覆写失败: rule_id=%d, error=%v", r.Id, err))
		}
	}
	return result
}

// GetHeaderOverride 解析请求头覆写配置为 map
func (r *RequestRule) GetHeaderOverride() map[string]interface{} {
	result := make(map[string]interface{})
	if r.HeaderOverride != nil && *r.HeaderOverride != "" {
		err := common.Unmarshal([]byte(*r.HeaderOverride), &result)
		if err != nil {
			common.SysLog(fmt.Sprintf("解析请求规则头覆写失败: rule_id=%d, error=%v", r.Id, err))
		}
	}
	return result
}

// MatchModel 检查给定的模型名是否匹配此规则的模型模式
func (r *RequestRule) MatchModel(modelName string) bool {
	// 空模式匹配全部模型
	if r.ModelPattern == "" {
		return true
	}
	switch r.ModelMatchMode {
	case ModelMatchExact:
		return modelName == r.ModelPattern
	case ModelMatchPrefix:
		return len(modelName) >= len(r.ModelPattern) && modelName[:len(r.ModelPattern)] == r.ModelPattern
	case ModelMatchContains:
		return contains(modelName, r.ModelPattern)
	case ModelMatchSuffix:
		return len(modelName) >= len(r.ModelPattern) && modelName[len(modelName)-len(r.ModelPattern):] == r.ModelPattern
	case ModelMatchWildcard:
		matched, _ := filepath.Match(r.ModelPattern, modelName)
		return matched
	default:
		return modelName == r.ModelPattern
	}
}

// contains 简单的子串包含检查
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
