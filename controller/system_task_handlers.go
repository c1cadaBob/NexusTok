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
	"github.com/c1cada/NexusTok/setting/operation_setting"
)

func init() {
	service.RegisterSystemTaskHandler(channelTestHandler{})
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

func finishSystemTaskHandler(task *model.SystemTask, runnerID string, status model.SystemTaskStatus, result any, runErr error) {
	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		common.SysError(fmt.Sprintf("system task %s failed to persist result: %v", task.TaskID, err))
	}
}
