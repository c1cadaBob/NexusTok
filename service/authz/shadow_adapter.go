package authz

import (
	"errors"
	"strings"

	"github.com/c1cada/NexusTok/model"
	casbinmodel "github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"gorm.io/gorm"
)

var errAuthzShadowAdapterReadOnly = errors.New("authz shadow adapter is read-only")

// shadowGormAdapter 是 Casbin 影子运行时使用的只读 GORM adapter。
//
// NexusTok 当前的真实授权仍由 Can()/持久策略快照负责。该 adapter 只把
// casbin_rule 加载进 SyncedEnforcer，用于验证未来切换到 Casbin runtime 前的
// 策略一致性；所有写入方法都显式拒绝，避免 shadow enforcer 误改数据库策略。
type shadowGormAdapter struct {
	db *gorm.DB
}

func newShadowGormAdapter(db *gorm.DB) *shadowGormAdapter {
	return &shadowGormAdapter{db: db}
}

// LoadPolicy 按数据库主键顺序加载 casbin_rule，保持与 new-api-main adapter 的
// 兼容语义。历史策略中 p 规则的 V3 为空时，按 allow 处理。
func (a *shadowGormAdapter) LoadPolicy(m casbinmodel.Model) error {
	if a == nil || a.db == nil {
		return errAuthzDatabaseNotInitialized
	}

	var rules []model.CasbinRule
	if err := a.db.Order("id ASC").Find(&rules).Error; err != nil {
		return err
	}
	for _, rule := range rules {
		if err := persist.LoadPolicyLine(shadowRuleToLine(rule), m); err != nil {
			return err
		}
	}
	return nil
}

// SavePolicy 在影子模式下禁止写入；真实策略写入仍由角色策略/导入接口维护。
func (a *shadowGormAdapter) SavePolicy(casbinmodel.Model) error {
	return errAuthzShadowAdapterReadOnly
}

// AddPolicy 在影子模式下禁止写入，避免 AutoSave 或误用改变 casbin_rule。
func (a *shadowGormAdapter) AddPolicy(string, string, []string) error {
	return errAuthzShadowAdapterReadOnly
}

// RemovePolicy 在影子模式下禁止写入，避免影子检查影响真实授权策略。
func (a *shadowGormAdapter) RemovePolicy(string, string, []string) error {
	return errAuthzShadowAdapterReadOnly
}

// RemoveFilteredPolicy 在影子模式下禁止写入，避免影子检查影响真实授权策略。
func (a *shadowGormAdapter) RemoveFilteredPolicy(string, string, int, ...string) error {
	return errAuthzShadowAdapterReadOnly
}

func shadowRuleToLine(rule model.CasbinRule) string {
	parts := []string{rule.Ptype}
	values := []string{rule.V0, rule.V1, rule.V2, rule.V3, rule.V4, rule.V5}
	if rule.Ptype == "p" && rule.V0 != "" && rule.V1 != "" && rule.V2 != "" && rule.V3 == "" {
		values[3] = EffectAllow
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		parts = append(parts, value)
	}
	return strings.Join(parts, ", ")
}
