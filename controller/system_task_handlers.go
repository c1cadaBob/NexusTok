package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/constant"
	"github.com/c1cadaBob/NexusTok/model"
	"github.com/c1cadaBob/NexusTok/service"
	"github.com/c1cadaBob/NexusTok/setting/operation_setting"
)

// RegisterScheduledSystemTasks wires the periodic channel test, upstream model
// update, and async task polling (Midjourney / Suno / video) jobs into the
// system task framework so a DB lease dedups execution across multiple master
// instances and each run is recorded as one task row. Call this before
// service.StartSystemTaskRunner.
func RegisterScheduledSystemTasks() {
	service.RegisterSystemTaskHandler(channelTestHandler{})
	service.RegisterSystemTaskHandler(modelUpdateHandler{})
	service.RegisterSystemTaskHandler(upstreamSiteSyncHandler{})
	service.RegisterSystemTaskHandler(routingKeyRecoveryHandler{})
	service.RegisterSystemTaskHandler(platformSiteUsageCostHandler{})
	service.RegisterSystemTaskHandler(midjourneyPollHandler{})
	service.RegisterSystemTaskHandler(asyncTaskPollHandler{})
}

type upstreamSiteSyncHandler struct{}

func (upstreamSiteSyncHandler) Type() string { return model.SystemTaskTypeUpstreamSync }

func (upstreamSiteSyncHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("UPSTREAM_SITE_SYNC_TASK_ENABLED", true)
}

func (upstreamSiteSyncHandler) Interval() time.Duration {
	minutes := common.GetEnvOrDefault("UPSTREAM_SITE_SYNC_TASK_INTERVAL_MINUTES", 15)
	if minutes < 1 {
		minutes = 15
	}
	return time.Duration(minutes) * time.Minute
}

type upstreamSiteSyncPayload struct {
	ChannelID int `json:"channel_id,omitempty"`
}

func (upstreamSiteSyncHandler) NewPayload() any { return upstreamSiteSyncPayload{} }

func (upstreamSiteSyncHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := upstreamSiteSyncPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	successCount, failureCount, err := service.SyncUpstreamSites(
		ctx,
		payload.ChannelID,
		service.NewSystemTaskProgressReporter(task, runnerID),
	)
	result := map[string]any{
		"success_count": successCount,
		"failure_count": failureCount,
	}
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, result, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, result, nil)
}

type platformSiteUsageCostHandler struct{}

func (platformSiteUsageCostHandler) Type() string {
	return model.SystemTaskTypePlatformSiteUsageCost
}

func (platformSiteUsageCostHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("PLATFORM_SITE_USAGE_COST_SYNC_ENABLED", true)
}

func (platformSiteUsageCostHandler) Interval() time.Duration {
	return time.Minute
}

func (platformSiteUsageCostHandler) NewPayload() any { return nil }

func (platformSiteUsageCostHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	result, err := service.SyncPlatformSiteUsageCosts(ctx, time.Now().UTC())
	if err != nil {
		finishSystemTaskHandler(
			task,
			runnerID,
			model.SystemTaskStatusFailed,
			result,
			fmt.Errorf("平台站点使用日志成本同步失败"),
		)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, result, nil)
}

// channelTestHandler runs the scheduled "test all channels" job. Enablement and
// cadence still come from the monitor settings; only the execution path moved
// into the system task runner.
type channelTestHandler struct{}

func (channelTestHandler) Type() string { return model.SystemTaskTypeChannelTest }

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

func (channelTestHandler) NewPayload() any { return nil }

// channelTestTaskPayload controls one channel_test run. A nil/empty payload is a
// scheduled run, which uses the configured monitor ChannelTestMode and does not
// notify. A manual "test all channels" trigger sets Mode=scheduled_all and
// Notify=true to reproduce the legacy manual behavior (test every channel and
// notify root on completion).
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

// modelUpdateHandler runs the scheduled upstream model update detection job.
type modelUpdateHandler struct{}

func (modelUpdateHandler) Type() string { return model.SystemTaskTypeModelUpdate }

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

func (modelUpdateHandler) NewPayload() any { return nil }

// modelUpdateTaskPayload controls one model_update run. A scheduled run
// (Manual=false) respects the per-channel minimum check interval and may
// auto-apply detected models when a channel has auto-sync enabled. A manual
// "detect all" trigger sets Manual=true to reproduce the legacy detect-all
// semantics: force a re-check regardless of the interval and never auto-apply,
// so the admin reviews and applies changes explicitly.
type modelUpdateTaskPayload struct {
	Manual bool `json:"manual,omitempty"`
}

func (modelUpdateHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := modelUpdateTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary := runChannelUpstreamModelUpdateTaskOnce(ctx, payload.Manual, !payload.Manual, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

type routingKeyRecoveryHandler struct{}

func (routingKeyRecoveryHandler) Type() string {
	return model.SystemTaskTypeRoutingKeyRecovery
}

func (routingKeyRecoveryHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("ROUTING_KEY_RECOVERY_TASK_ENABLED", true)
}

func (routingKeyRecoveryHandler) Interval() time.Duration {
	return time.Minute
}

func (routingKeyRecoveryHandler) NewPayload() any { return nil }

type routingKeyRecoveryResult struct {
	Tested    int `json:"tested"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	NextDue   int `json:"next_due"`
}

func (routingKeyRecoveryHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	candidates, err := model.ListDueRoutingKeyRecoveryCandidates(time.Now())
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	result := routingKeyRecoveryResult{}
	report := service.NewSystemTaskProgressReporter(task, runnerID)
	if len(candidates) == 0 {
		report(0, 0)
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, result, nil)
		return
	}

	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, result, err)
		return
	}
	for index := range candidates {
		if ctx.Err() != nil {
			finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, result, ctx.Err())
			return
		}
		candidate := candidates[index]
		testResult := testChannelWithOptions(
			ctx,
			candidate.Channel,
			testUserID,
			"",
			"",
			shouldUseStreamForAutomaticChannelTest(candidate.Channel),
			candidate.RoutingKeyID,
			0,
			channelTestOptions{recoveryTest: true},
		)
		if ctx.Err() != nil {
			finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, result, ctx.Err())
			return
		}
		success := testResult.localErr == nil && testResult.newAPIError == nil
		result.Tested++
		if success {
			result.Succeeded++
		} else {
			result.Failed++
		}
		if err := model.RecordRoutingKeyRecoverySample(
			candidate.RoutingKeyID,
			candidate.ChannelID,
			candidate.Source,
			success,
			testResult.firstLatencyMs,
			time.Now(),
		); err != nil {
			common.SysLog(fmt.Sprintf(
				"记录密钥恢复测试样本失败: key_id=%d channel_id=%d error=%v",
				candidate.RoutingKeyID,
				candidate.ChannelID,
				err,
			))
			result.Failed++
		}
		if state := model.GetRoutingKeyRecoveryState(candidate.RoutingKeyID); state.NextTestAt > time.Now().Unix() {
			result.NextDue++
		}
		report(index+1, len(candidates))
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, result, nil)
}

// midjourneyPollHandler runs one Midjourney polling pass per scheduled run.
// Enabled() folds the "are there unfinished tasks?" check into enablement so the
// scheduler creates no row when the system is idle; only when at least one
// Midjourney task is in progress does a row get scheduled.
type midjourneyPollHandler struct{}

func (midjourneyPollHandler) Type() string { return model.SystemTaskTypeMidjourneyPoll }

func (midjourneyPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedMidjourneyTasks()
}

func (midjourneyPollHandler) Interval() time.Duration { return 15 * time.Second }

func (midjourneyPollHandler) NewPayload() any { return nil }

func (midjourneyPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := runMidjourneyTaskUpdateOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// asyncTaskPollHandler runs one async-task (Suno/video) polling pass per
// scheduled run. Like midjourneyPollHandler, Enabled() folds in the unfinished
// task existence check so an idle system schedules no rows.
type asyncTaskPollHandler struct{}

func (asyncTaskPollHandler) Type() string { return model.SystemTaskTypeAsyncTaskPoll }

func (asyncTaskPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedSyncTasks()
}

func (asyncTaskPollHandler) Interval() time.Duration { return 15 * time.Second }

func (asyncTaskPollHandler) NewPayload() any { return nil }

func (asyncTaskPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := service.RunTaskPollingOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

func finishSystemTaskHandler(task *model.SystemTask, runnerID string, status model.SystemTaskStatus, result any, runErr error) {
	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		common.SysLog(fmt.Sprintf("system task %s failed to persist result: %v", task.TaskID, err))
	}
}
