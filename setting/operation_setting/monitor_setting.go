// monitor_setting.go — 渠道监控配置
// 职责：管理渠道自动测试功能的开关和测试频率。
// 当启用自动测试时，系统会按设定的时间间隔定期检测渠道可用性。

package operation_setting

import (
	"os"
	"strconv"

	"github.com/c1cada/NexusTok/setting/config"
)

// MonitorSetting 渠道监控配置结构体
type MonitorSetting struct {
	// AutoTestChannelEnabled 是否启用渠道自动测试
	AutoTestChannelEnabled bool `json:"auto_test_channel_enabled"`
	// AutoTestChannelMinutes 自动测试的时间间隔（分钟）
	AutoTestChannelMinutes float64 `json:"auto_test_channel_minutes"`
}

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled: false,
	AutoTestChannelMinutes: 10,
}

// init 注册渠道监控配置到全局配置管理器
func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("monitor_setting", &monitorSetting)
}

// GetMonitorSetting 获取渠道监控配置
// 优先从环境变量 CHANNEL_TEST_FREQUENCY 读取测试频率，
// 若环境变量有效则覆盖数据库中的配置值
func GetMonitorSetting() *MonitorSetting {
	if os.Getenv("CHANNEL_TEST_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_TEST_FREQUENCY"))
		if err == nil && frequency > 0 {
			monitorSetting.AutoTestChannelEnabled = true
			monitorSetting.AutoTestChannelMinutes = float64(frequency)
		}
	}
	return &monitorSetting
}
