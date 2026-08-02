// general_setting.go — 通用运维配置
// 职责：管理站点级别的通用运维设置，包括文档链接、心跳间隔、
// 额度展示类型（美元/人民币/Token/自定义货币）以及自定义货币的符号和汇率。

package operation_setting

import "github.com/c1cada/NexusTok/setting/config"

// 额度展示类型常量
const (
	QuotaDisplayTypeUSD    = "USD"
	QuotaDisplayTypeCNY    = "CNY"
	QuotaDisplayTypeTokens = "TOKENS"
	QuotaDisplayTypeCustom = "CUSTOM"
)

// GeneralSetting 通用运维配置结构体
type GeneralSetting struct {
	// DocsLink 帮助文档链接地址
	DocsLink string `json:"docs_link"`
	// PingIntervalEnabled 是否启用心跳间隔检测
	PingIntervalEnabled bool `json:"ping_interval_enabled"`
	// PingIntervalSeconds 心跳间隔时间（秒）
	PingIntervalSeconds int `json:"ping_interval_seconds"`
	// 当前站点额度展示类型：USD / CNY / TOKENS
	QuotaDisplayType string `json:"quota_display_type"`
	// 自定义货币符号，用于 CUSTOM 展示类型
	CustomCurrencySymbol string `json:"custom_currency_symbol"`
	// 自定义货币与美元汇率（1 USD = X Custom）
	CustomCurrencyExchangeRate float64 `json:"custom_currency_exchange_rate"`
}

// 默认配置
var generalSetting = GeneralSetting{
	DocsLink:                   "https://github.com/c1cadaBob/NexusTok",
	PingIntervalEnabled:        false,
	PingIntervalSeconds:        60,
	QuotaDisplayType:           QuotaDisplayTypeUSD,
	CustomCurrencySymbol:       "¤",
	CustomCurrencyExchangeRate: 1.0,
}

// init 注册通用配置到全局配置管理器
func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("general_setting", &generalSetting)
}

// GetGeneralSetting 获取通用运维配置
func GetGeneralSetting() *GeneralSetting {
	return &generalSetting
}

// IsCurrencyDisplay 是否以货币形式展示（美元或人民币）
func IsCurrencyDisplay() bool {
	return generalSetting.QuotaDisplayType != QuotaDisplayTypeTokens
}

// IsCNYDisplay 是否以人民币展示
func IsCNYDisplay() bool {
	return generalSetting.QuotaDisplayType == QuotaDisplayTypeCNY
}

// GetQuotaDisplayType 返回额度展示类型
func GetQuotaDisplayType() string {
	return generalSetting.QuotaDisplayType
}

// GetCurrencySymbol 返回当前展示类型对应符号
func GetCurrencySymbol() string {
	switch generalSetting.QuotaDisplayType {
	case QuotaDisplayTypeUSD:
		return "$"
	case QuotaDisplayTypeCNY:
		return "¥"
	case QuotaDisplayTypeCustom:
		if generalSetting.CustomCurrencySymbol != "" {
			return generalSetting.CustomCurrencySymbol
		}
		return "¤"
	default:
		return ""
	}
}

// GetUsdToCurrencyRate 返回 1 USD = X <currency> 的 X（TOKENS 不适用）
func GetUsdToCurrencyRate(usdToCny float64) float64 {
	switch generalSetting.QuotaDisplayType {
	case QuotaDisplayTypeUSD:
		return 1
	case QuotaDisplayTypeCNY:
		return usdToCny
	case QuotaDisplayTypeCustom:
		if generalSetting.CustomCurrencyExchangeRate > 0 {
			return generalSetting.CustomCurrencyExchangeRate
		}
		return 1
	default:
		return 1
	}
}
