// quota_setting.go — 额度（配额）配置
// 职责：管理用户额度相关的配置，控制免费模型是否需要预消耗额度。

package operation_setting

import "github.com/c1cada/NexusTok/setting/config"

// QuotaSetting 额度配置结构体
type QuotaSetting struct {
	EnableFreeModelPreConsume bool `json:"enable_free_model_pre_consume"` // 是否对免费模型启用预消耗
}

// 默认配置
var quotaSetting = QuotaSetting{
	EnableFreeModelPreConsume: true,
}

// init 注册额度配置到全局配置管理器
func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("quota_setting", &quotaSetting)
}

// GetQuotaSetting 获取额度配置
func GetQuotaSetting() *QuotaSetting {
	return &quotaSetting
}
