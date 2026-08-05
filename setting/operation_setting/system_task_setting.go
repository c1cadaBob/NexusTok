// system_task_setting.go — 后台维护任务配置
// 职责：集中管理异步任务、绘图任务、订阅维护和 models.dev 目录同步的开关。
//
// 配置首次初始化时兼容既有环境变量；管理员在系统设置中保存后，数据库配置
// 会覆盖默认值并在运行时立即生效，不需要为每个任务重新维护一套环境变量。
package operation_setting

import (
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting/config"
)

const defaultModelsDevSyncTime = "02:00"

// SystemTaskSetting 是后台维护任务的统一开关配置。
type SystemTaskSetting struct {
	AsyncTaskPollEnabled           bool   `json:"async_task_poll_enabled"`
	MidjourneyPollEnabled          bool   `json:"midjourney_poll_enabled"`
	SubscriptionMaintenanceEnabled bool   `json:"subscription_maintenance_enabled"`
	ModelsDevSyncEnabled           bool   `json:"models_dev_sync_enabled"`
	ModelsDevSyncTime              string `json:"models_dev_sync_time"`
}

var systemTaskSetting = SystemTaskSetting{
	// UPDATE_TASK 仍由 handler 作为总开关；新设置默认开启，避免因为启动时
	// 环境变量尚未完成初始化而把旧版本默认任务错误关掉。
	AsyncTaskPollEnabled:           true,
	MidjourneyPollEnabled:          true,
	SubscriptionMaintenanceEnabled: true,
	ModelsDevSyncEnabled: common.GetEnvOrDefaultBool(
		"MODELS_DEV_AUTO_SYNC_ENABLED",
		true,
	),
	ModelsDevSyncTime: common.GetEnvOrDefaultString(
		"MODELS_DEV_AUTO_SYNC_TIME",
		defaultModelsDevSyncTime,
	),
}

func init() {
	config.GlobalConfig.Register("system_task_setting", &systemTaskSetting)
}

// GetSystemTaskSetting 返回后台维护任务当前配置。
func GetSystemTaskSetting() *SystemTaskSetting {
	return &systemTaskSetting
}
