// Package controller - system_task_handlers.go
// 该文件把控制器侧已有的运维动作接入 SystemTask runner。
//
// SystemTask 负责跨节点去重、租约续期、任务历史和进度观测；具体业务执行仍放在
// 对应控制器/服务的原生函数中，避免为了后台化而复制一套渠道测试、模型同步逻辑。
package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/service/upstreamaccount"
	"github.com/c1cada/NexusTok/setting/operation_setting"
)

func init() {
	service.RegisterSystemTaskHandler(channelTestHandler{})
	service.RegisterSystemTaskHandler(modelUpdateHandler{})
	service.RegisterSystemTaskHandler(accountPoolCheckHandler{})
	service.RegisterSystemTaskHandler(upstreamAccountSyncHandler{})
}

// channelTestHandler 执行批量渠道测试任务。
//
// 定时任务的启用和间隔仍读取原有 monitor_setting，执行层迁入 SystemTask 后可以获得
// 数据库租约、任务进度和历史结果，避免多节点同时自动测试同一批渠道。
type channelTestHandler struct{}

func (channelTestHandler) Type() string {
	return model.SystemTaskTypeChannelTest
}

func (channelTestHandler) Enabled() bool {
	return operation_setting.GetMonitorSetting().AutoTestChannelEnabled
}

func (channelTestHandler) Interval() time.Duration {
	minutes := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
	if minutes <= 0 {
		minutes = 10
	}
	return time.Duration(minutes * float64(time.Minute))
}

func (channelTestHandler) NewPayload() any {
	return nil
}

// channelTestTaskPayload 控制一次 channel_test 任务。
//
// 空 payload 表示定时任务，会使用 monitor_setting.channel_test_mode；手动“测试所有渠道”
// 固定使用 scheduled_all 并在完成后通知 Root，以保持旧接口的操作语义。
type channelTestTaskPayload struct {
	Mode   string `json:"mode,omitempty"`
	Notify bool   `json:"notify,omitempty"`
}

func (channelTestHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelTestTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}

	summary, err := runChannelTestTask(ctx, payload.Mode, payload.Notify, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// modelUpdateHandler 执行上游模型更新巡检任务。
//
// 定时任务继续读取原有环境变量控制开关和间隔；执行迁入 SystemTask 后可以复用
// 数据库租约、防重复触发、进度状态和任务历史。手动 detect-all 会创建 Manual=true
// 的任务，强制检测但不自动应用模型。
type modelUpdateHandler struct{}

func (modelUpdateHandler) Type() string {
	return model.SystemTaskTypeModelUpdate
}

func (modelUpdateHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", true)
}

func (modelUpdateHandler) Interval() time.Duration {
	intervalMinutes := common.GetEnvOrDefault(
		"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_INTERVAL_MINUTES",
		channelUpstreamModelUpdateTaskDefaultIntervalMinutes,
	)
	if intervalMinutes < 1 {
		intervalMinutes = channelUpstreamModelUpdateTaskDefaultIntervalMinutes
	}
	return time.Duration(intervalMinutes) * time.Minute
}

func (modelUpdateHandler) NewPayload() any {
	return nil
}

// modelUpdateTaskPayload 控制一次 model_update 任务。
//
// Manual=false 的定时巡检会尊重渠道最小检查间隔，并允许开启自动同步的渠道自动添加
// 新模型；Manual=true 的手动检测会强制重新检查所有启用该能力的渠道，但只暂存变更，
// 由管理员在渠道页显式应用。
type modelUpdateTaskPayload struct {
	Manual bool `json:"manual,omitempty"`
}

func (modelUpdateHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := modelUpdateTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary, err := runChannelUpstreamModelUpdateTaskOnce(ctx, payload.Manual, !payload.Manual, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	if summary.Cancelled {
		runErr := ctx.Err()
		if runErr == nil {
			runErr = context.Canceled
		}
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, runErr)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// accountPoolCheckHandler 执行账号池后台检测任务。
//
// 账号池检测保留原有 PoolAccountCheckTask 业务历史和页面接口；SystemTask 只负责跨节点
// 串行认领、租约续期、进度写入和全局任务面板观测，避免账号池模块继续维护独立内存队列。
type accountPoolCheckHandler struct{}

func (accountPoolCheckHandler) Type() string {
	return model.SystemTaskTypeAccountPoolCheck
}

func (accountPoolCheckHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := service.AccountPoolCheckSystemTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary, err := service.RunPoolAccountCheckSystemTask(ctx, payload.CheckTaskID, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	if summary.Status == model.PoolAccountCheckTaskStatusFailed {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, fmt.Errorf("%s", summary.Message))
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// upstreamAccountSyncHandler 执行全局上游同步渠道自动刷新。
//
// Enabled 和 Interval 只负责调度新任务；Run 开始时会再次检查配置，避免管理员关闭
// 自动同步后，已经排队的 pending 任务仍然修改渠道账号池。
type upstreamAccountSyncHandler struct{}

func (upstreamAccountSyncHandler) Type() string {
	return model.SystemTaskTypeUpstreamAccountSync
}

func (upstreamAccountSyncHandler) Enabled() bool {
	setting := operation_setting.GetUpstreamAccountSyncSetting()
	if !setting.Enabled {
		return false
	}
	_, err := setting.Duration()
	return err == nil
}

func (upstreamAccountSyncHandler) Interval() time.Duration {
	setting := operation_setting.GetUpstreamAccountSyncSetting()
	duration, err := setting.Duration()
	if err != nil || duration <= 0 {
		return time.Hour
	}
	return duration
}

func (upstreamAccountSyncHandler) NewPayload() any {
	return nil
}

func (upstreamAccountSyncHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	setting := operation_setting.GetUpstreamAccountSyncSetting()
	if !setting.Enabled {
		summary := &upstreamaccount.UpstreamAccountSyncSummary{
			Skipped:    true,
			SkipReason: "上游账号自动同步已关闭",
		}
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
		recordSystemUpstreamAccountSyncAudit(task, runnerID, summary, nil)
		return
	}
	if _, err := setting.Duration(); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		recordSystemUpstreamAccountSyncAudit(task, runnerID, nil, err)
		return
	}

	summary, err := upstreamaccount.RunUpstreamAccountSync(
		ctx,
		service.NewSystemTaskProgressReporter(task, runnerID),
		upstreamaccount.WithSystemTaskLog(task.TaskID, task.CreatedAt),
	)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		recordSystemUpstreamAccountSyncAudit(task, runnerID, summary, err)
		return
	}
	runErr := upstreamaccount.AutomaticSyncFailureError(summary)
	status := model.SystemTaskStatusSucceeded
	if runErr != nil {
		status = model.SystemTaskStatusFailed
	}
	finishSystemTaskHandler(task, runnerID, status, summary, runErr)
	recordSystemUpstreamAccountSyncAudit(task, runnerID, summary, runErr)
}

func finishSystemTaskHandler(task *model.SystemTask, runnerID string, status model.SystemTaskStatus, result any, runErr error) {
	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		common.SysError(fmt.Sprintf("system task %s failed to persist result: %v", task.TaskID, err))
	}
}
