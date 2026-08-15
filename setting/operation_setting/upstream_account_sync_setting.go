// upstream_account_sync_setting.go — 上游账号自动同步配置
// 职责：保存管理员选择的自动同步开关、间隔数值和时间单位，并将配置安全地
// 转换为 SystemTask 调度器使用的 time.Duration。
//
// 月份按固定 30 天计算，不使用日历月份；这样可以与现有基于 time.Duration
// 的任务调度器保持一致，并避免月末、闰年和时区导致的周期不稳定。
package operation_setting

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/setting/config"
)

const (
	UpstreamAccountSyncUnitMonth  = "month"
	UpstreamAccountSyncUnitWeek   = "week"
	UpstreamAccountSyncUnitDay    = "day"
	UpstreamAccountSyncUnitHour   = "hour"
	UpstreamAccountSyncUnitMinute = "minute"
	UpstreamAccountSyncUnitSecond = "second"
)

// UpstreamAccountSyncSetting 是上游同步渠道的全局自动同步配置。
type UpstreamAccountSyncSetting struct {
	// Enabled=false 表示不同步；关闭后不会创建新的周期任务。
	Enabled bool `json:"enabled"`
	// Interval 是时间单位的正整数倍数。
	Interval int64 `json:"interval"`
	// Unit 是 month/week/day/hour/minute/second 之一。
	Unit string `json:"unit"`
	// SyncKeyModelsEnabled 控制账号同步后是否继续维护每个同步密钥的模型列表。
	//
	// 该开关只影响由上游账号同步流程维护的 ChannelAccount.models；普通账号池和渠道
	// 顶层模型列表仍按原有配置保存。默认开启，让最低倍率的模型候选能够跟随上游密钥
	// 能力更新；关闭时刷新会尽量保留已有本地模型白名单。
	SyncKeyModelsEnabled bool `json:"sync_key_models_enabled"`
	// KeyModelSyncOverwriteManualEnabled 控制模型同步是否覆盖管理员手动编辑过的白名单。
	//
	// 默认关闭。同步账号 settings 会记录模型白名单是否被人工编辑过；关闭时后台同步
	// 只更新未人工覆盖的账号，避免上游平台模型列表把本地治理白名单直接冲掉。
	KeyModelSyncOverwriteManualEnabled bool `json:"key_model_sync_overwrite_manual_enabled"`
}

var upstreamAccountSyncSetting = UpstreamAccountSyncSetting{
	Enabled:                            false,
	Interval:                           1,
	Unit:                               UpstreamAccountSyncUnitHour,
	SyncKeyModelsEnabled:               true,
	KeyModelSyncOverwriteManualEnabled: false,
}

// init 将配置注册到全局配置管理器，继续复用 Option 键值表持久化。
func init() {
	config.GlobalConfig.Register("upstream_account_sync", &upstreamAccountSyncSetting)
}

// GetUpstreamAccountSyncSetting 返回当前上游账号自动同步配置。
func GetUpstreamAccountSyncSetting() *UpstreamAccountSyncSetting {
	return &upstreamAccountSyncSetting
}

// Duration 将当前配置转换为任务调度周期。
//
// Enabled=false 时返回零周期且不报错；调用方应使用 Enabled 判断是否创建任务。
// 启用配置如果无效，则返回错误，让调度器安全停用而不是启动异常高频或溢出任务。
func (setting UpstreamAccountSyncSetting) Duration() (time.Duration, error) {
	if !setting.Enabled {
		return 0, nil
	}
	if setting.Interval <= 0 {
		return 0, fmt.Errorf("上游账号自动同步间隔必须大于 0")
	}

	unitDuration, ok := upstreamAccountSyncUnitDuration(strings.ToLower(strings.TrimSpace(setting.Unit)))
	if !ok {
		return 0, fmt.Errorf("上游账号自动同步时间单位无效：%s", setting.Unit)
	}

	unitNanos := int64(unitDuration)
	if setting.Interval > math.MaxInt64/unitNanos {
		return 0, fmt.Errorf("上游账号自动同步周期超出 time.Duration 可表示范围")
	}
	return time.Duration(setting.Interval * unitNanos), nil
}

func upstreamAccountSyncUnitDuration(unit string) (time.Duration, bool) {
	switch unit {
	case UpstreamAccountSyncUnitMonth:
		return 30 * 24 * time.Hour, true
	case UpstreamAccountSyncUnitWeek:
		return 7 * 24 * time.Hour, true
	case UpstreamAccountSyncUnitDay:
		return 24 * time.Hour, true
	case UpstreamAccountSyncUnitHour:
		return time.Hour, true
	case UpstreamAccountSyncUnitMinute:
		return time.Minute, true
	case UpstreamAccountSyncUnitSecond:
		return time.Second, true
	default:
		return 0, false
	}
}
