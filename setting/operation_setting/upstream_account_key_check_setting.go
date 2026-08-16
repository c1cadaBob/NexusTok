// upstream_account_key_check_setting.go — 同步密钥级自动连接测试配置
//
// 该配置只作用于带 upstream_account_sync 元数据的 ChannelAccount。普通渠道 key、多 key
// 渠道和全局账号池继续使用原有渠道测试/账号池检测机制，避免两个自动化任务交叉禁用。
package operation_setting

import (
	"time"

	"github.com/c1cada/NexusTok/setting/config"
)

// UpstreamAccountKeyCheckSetting 控制同步密钥自动连接测试。
type UpstreamAccountKeyCheckSetting struct {
	// Enabled=false 表示不创建周期任务。
	Enabled bool `json:"enabled"`
	// IntervalMinutes 是两次后台扫描的最小间隔，非法值按默认 30 分钟处理。
	IntervalMinutes int `json:"interval_minutes"`
	// RatioThreshold 为 0 时测试所有符合条件的同步密钥；大于 0 时只测试换算倍率小于该值的密钥。
	RatioThreshold float64 `json:"ratio_threshold"`
	// FailureThreshold 表示连续失败达到多少次后自动禁用，非法值按默认 3 次处理。
	FailureThreshold int `json:"failure_threshold"`
	// AutoRecoverEnabled 控制由本任务自动禁用的密钥在后续测试成功后是否自动恢复。
	AutoRecoverEnabled bool `json:"auto_recover_enabled"`
}

var upstreamAccountKeyCheckSetting = UpstreamAccountKeyCheckSetting{
	Enabled:            false,
	IntervalMinutes:    30,
	RatioThreshold:     0,
	FailureThreshold:   3,
	AutoRecoverEnabled: true,
}

func init() {
	config.GlobalConfig.Register("upstream_account_key_check", &upstreamAccountKeyCheckSetting)
}

// GetUpstreamAccountKeyCheckSetting 返回同步密钥自动连接测试配置。
func GetUpstreamAccountKeyCheckSetting() *UpstreamAccountKeyCheckSetting {
	return &upstreamAccountKeyCheckSetting
}

// Interval 返回调度器使用的周期。
func (setting UpstreamAccountKeyCheckSetting) Interval() time.Duration {
	minutes := setting.IntervalMinutes
	if minutes <= 0 {
		minutes = 30
	}
	return time.Duration(minutes) * time.Minute
}

// NormalizedFailureThreshold 返回最小为 1 的连续失败阈值。
func (setting UpstreamAccountKeyCheckSetting) NormalizedFailureThreshold() int {
	if setting.FailureThreshold <= 0 {
		return 3
	}
	return setting.FailureThreshold
}
