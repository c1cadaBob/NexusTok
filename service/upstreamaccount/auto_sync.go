// auto_sync.go — 上游同步渠道的后台自动同步编排
// 职责：扫描符合条件的渠道，复用已保存凭据调用现有刷新流程，并汇总每个渠道
// 的成功、跳过和失败结果。
//
// 自动任务只负责调度，不改变 RefreshChannelFromCredential 的业务边界。刷新函数
// 仍然负责事务、余额快照、账号 upsert、缺失 key 禁用和渠道缓存重建。
package upstreamaccount

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
)

const upstreamAccountAutoSyncChannelTimeout = 5 * time.Minute
const upstreamAccountSyncTaskLogErrorMaxRunes = 512

var upstreamAccountSyncTaskLogSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(access[_-]?token|refresh[_-]?token|password|session|cookie|api[_-]?key|key)\s*[:=]\s*["']?[^"',\s}]+`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{12,}`),
}

// UpstreamAccountSyncSummary 是一次全局自动同步任务的可观察结果。
type UpstreamAccountSyncSummary struct {
	ScannedChannels   int                          `json:"scanned_channels"`
	EligibleChannels  int                          `json:"eligible_channels"`
	SkippedChannels   int                          `json:"skipped_channels"`
	SucceededChannels int                          `json:"succeeded_channels"`
	FailedChannels    int                          `json:"failed_channels"`
	CreatedAccounts   int                          `json:"created_accounts"`
	UpdatedAccounts   int                          `json:"updated_accounts"`
	DisabledAccounts  int                          `json:"disabled_accounts"`
	Skipped           bool                         `json:"skipped,omitempty"`
	SkipReason        string                       `json:"skip_reason,omitempty"`
	Failures          []UpstreamAccountSyncFailure `json:"failures,omitempty"`
}

// UpstreamAccountSyncFailure 记录单个渠道的失败信息。
//
// 这里只保存渠道标识和错误文本，绝不保存解密后的密码、session 或完整 API Key。
type UpstreamAccountSyncFailure struct {
	ChannelID   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Error       string `json:"error"`
}

// RunUpstreamAccountSyncOption 配置一次自动同步执行过程中的附加观测行为。
type RunUpstreamAccountSyncOption func(*runUpstreamAccountSyncOptions)

type runUpstreamAccountSyncOptions struct {
	TaskID     string
	SubmitTime int64
}

// WithSystemTaskLog 让自动同步为每个同步渠道写入任务日志。
//
// taskID 使用 SystemTask.TaskID，便于管理员在任务日志中按同一次后台任务筛选；
// submitTime 使用 SystemTask.CreatedAt，保证同次同步的逐渠道记录有一致的提交时间。
func WithSystemTaskLog(taskID string, submitTime int64) RunUpstreamAccountSyncOption {
	return func(options *runUpstreamAccountSyncOptions) {
		options.TaskID = strings.TrimSpace(taskID)
		options.SubmitTime = submitTime
	}
}

// RunUpstreamAccountSync 扫描并刷新所有符合条件的上游同步渠道。
//
// 单个渠道失败后继续处理后续渠道；只有数据库扫描失败或任务上下文取消等整体
// 错误才通过 error 返回。渠道级失败会保留在 summary.Failures 中，由 SystemTask
// handler 根据失败数量将任务标记为 failed。
func RunUpstreamAccountSync(ctx context.Context, report func(processed, total int), options ...RunUpstreamAccountSyncOption) (summary *UpstreamAccountSyncSummary, runErr error) {
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		return nil, err
	}
	runOptions := runUpstreamAccountSyncOptions{}
	for _, applyOption := range options {
		if applyOption != nil {
			applyOption(&runOptions)
		}
	}
	taskLogger := newUpstreamAccountSyncTaskLogger(runOptions)
	if err := taskLogger.Init(channels); err != nil {
		return nil, err
	}
	defer func() {
		if runErr == nil {
			return
		}
		reason := "上游账号自动同步任务未完成"
		if text := strings.TrimSpace(runErr.Error()); text != "" {
			reason += "：" + text
		}
		_ = taskLogger.FailUnfinished(reason)
	}()

	summary = &UpstreamAccountSyncSummary{
		ScannedChannels: len(channels),
		Failures:        make([]UpstreamAccountSyncFailure, 0),
	}
	if report != nil {
		report(0, len(channels))
	}

	for index, channel := range channels {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if channel == nil {
			summary.SkippedChannels++
			if report != nil {
				report(index+1, len(channels))
			}
			continue
		}
		if !channel.HasUpstreamAccountSyncMetadata() {
			summary.SkippedChannels++
			if report != nil {
				report(index+1, len(channels))
			}
			continue
		}
		if channel.Status != common.ChannelStatusEnabled {
			summary.SkippedChannels++
			_ = taskLogger.Skip(channel, "渠道已禁用，跳过上游账号自动同步")
			if report != nil {
				report(index+1, len(channels))
			}
			continue
		}
		_ = taskLogger.Start(channel)

		credential, ok, err := ReadChannelSyncCredential(channel.OtherSettings)
		if err != nil {
			appendUpstreamAccountSyncFailure(summary, channel, err)
			_ = taskLogger.Fail(channel, err)
			if report != nil {
				report(index+1, len(channels))
			}
			continue
		}
		if !ok {
			summary.SkippedChannels++
			_ = taskLogger.Skip(channel, "当前同步渠道没有保存上游账号凭据，请先在编辑渠道中重新同步上游账号")
			if report != nil {
				report(index+1, len(channels))
			}
			continue
		}

		summary.EligibleChannels++
		accounts, err := automaticAccountConfigs(channel.Id)
		if err != nil {
			appendUpstreamAccountSyncFailure(summary, channel, err)
			_ = taskLogger.Fail(channel, err)
			if report != nil {
				report(index+1, len(channels))
			}
			continue
		}

		channelCtx, cancel := context.WithTimeout(ctx, upstreamAccountAutoSyncChannelTimeout)
		result, refreshErr := RefreshChannelFromCredential(channelCtx, RefreshRequest{
			ChannelID:         channel.Id,
			Credential:        credential,
			Accounts:          accounts,
			ApplySuggested:    true,
			DisableMissingKey: true,
		})
		cancel()
		if refreshErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return summary, ctxErr
			}
			appendUpstreamAccountSyncFailure(summary, channel, refreshErr)
			_ = taskLogger.Fail(channel, refreshErr)
		} else {
			summary.SucceededChannels++
			if result != nil {
				summary.CreatedAccounts += result.Created
				summary.UpdatedAccounts += result.Updated
				summary.DisabledAccounts += result.Disabled
			}
			_ = taskLogger.Success(channel, result)
		}
		if report != nil {
			report(index+1, len(channels))
		}
	}

	return summary, nil
}

func appendUpstreamAccountSyncFailure(summary *UpstreamAccountSyncSummary, channel *model.Channel, err error) {
	summary.FailedChannels++
	summary.Failures = append(summary.Failures, UpstreamAccountSyncFailure{
		ChannelID:   channel.Id,
		ChannelName: channel.Name,
		Error:       strings.TrimSpace(err.Error()),
	})
}

type upstreamAccountSyncTaskLogger struct {
	taskID     string
	submitTime int64
	enabled    bool
}

type upstreamAccountSyncTaskLogData struct {
	Type             string `json:"type"`
	SystemTaskID     string `json:"system_task_id"`
	ChannelID        int    `json:"channel_id"`
	ChannelName      string `json:"channel_name"`
	Platform         string `json:"platform,omitempty"`
	Status           string `json:"status"`
	CreatedAccounts  int    `json:"created_accounts,omitempty"`
	UpdatedAccounts  int    `json:"updated_accounts,omitempty"`
	DisabledAccounts int    `json:"disabled_accounts,omitempty"`
	SkipReason       string `json:"skip_reason,omitempty"`
	Error            string `json:"error,omitempty"`
}

func newUpstreamAccountSyncTaskLogger(options runUpstreamAccountSyncOptions) upstreamAccountSyncTaskLogger {
	taskID := strings.TrimSpace(options.TaskID)
	submitTime := options.SubmitTime
	if submitTime <= 0 {
		submitTime = common.GetTimestamp()
	}
	return upstreamAccountSyncTaskLogger{
		taskID:     taskID,
		submitTime: submitTime,
		enabled:    taskID != "",
	}
}

// Init 在系统任务开始时为每个上游账号同步渠道预建任务日志。
//
// 普通渠道不会写入任务日志；带同步元数据的渠道即使当前禁用或缺少凭据，也会先写入
// NOT_START 记录，随后在本轮处理到该渠道时转成 SKIPPED/FAILURE/SUCCESS。这样系统任务
// 中断时，管理员仍能从任务日志看出哪些同步渠道没有被处理完成。
func (logger upstreamAccountSyncTaskLogger) Init(channels []*model.Channel) error {
	if !logger.enabled {
		return nil
	}
	for _, channel := range channels {
		if channel == nil || !channel.HasUpstreamAccountSyncMetadata() {
			continue
		}
		if err := logger.upsert(channel, model.TaskStatusNotStart, "0%", "", upstreamAccountSyncTaskLogData{}); err != nil {
			return err
		}
	}
	return nil
}

func (logger upstreamAccountSyncTaskLogger) Start(channel *model.Channel) error {
	return logger.upsert(channel, model.TaskStatusInProgress, "50%", "", upstreamAccountSyncTaskLogData{})
}

func (logger upstreamAccountSyncTaskLogger) Success(channel *model.Channel, result *RefreshResult) error {
	data := upstreamAccountSyncTaskLogData{}
	if result != nil {
		data.CreatedAccounts = result.Created
		data.UpdatedAccounts = result.Updated
		data.DisabledAccounts = result.Disabled
	}
	return logger.upsert(channel, model.TaskStatusSuccess, "100%", "", data)
}

func (logger upstreamAccountSyncTaskLogger) Skip(channel *model.Channel, reason string) error {
	reason = sanitizeUpstreamAccountSyncTaskLogText(reason, upstreamAccountSyncTaskLogErrorMaxRunes)
	return logger.upsert(channel, model.TaskStatusSkipped, "100%", reason, upstreamAccountSyncTaskLogData{
		SkipReason: reason,
	})
}

func (logger upstreamAccountSyncTaskLogger) Fail(channel *model.Channel, err error) error {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	reason = sanitizeUpstreamAccountSyncTaskLogText(reason, upstreamAccountSyncTaskLogErrorMaxRunes)
	return logger.upsert(channel, model.TaskStatusFailure, "100%", reason, upstreamAccountSyncTaskLogData{
		Error: reason,
	})
}

func (logger upstreamAccountSyncTaskLogger) FailUnfinished(reason string) error {
	if !logger.enabled {
		return nil
	}
	reason = sanitizeUpstreamAccountSyncTaskLogText(reason, upstreamAccountSyncTaskLogErrorMaxRunes)
	var tasks []*model.Task
	if err := model.DB.
		Where("task_id = ? AND action = ? AND status IN ?", logger.taskID, constant.TaskActionUpstreamAccountSync, []model.TaskStatus{
			model.TaskStatusNotStart,
			model.TaskStatusSubmitted,
			model.TaskStatusQueued,
			model.TaskStatusInProgress,
		}).
		Find(&tasks).Error; err != nil {
		return err
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		channel := &model.Channel{
			Id:   task.ChannelId,
			Name: task.ChannelName,
		}
		if channel.Name == "" {
			channel.Name = task.Properties.Input
		}
		if err := logger.upsert(channel, model.TaskStatusFailure, "100%", reason, upstreamAccountSyncTaskLogData{
			Error: reason,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (logger upstreamAccountSyncTaskLogger) upsert(channel *model.Channel, status model.TaskStatus, progress string, failReason string, data upstreamAccountSyncTaskLogData) error {
	if !logger.enabled || channel == nil {
		return nil
	}
	now := common.GetTimestamp()
	finishTime := int64(0)
	if status == model.TaskStatusSuccess || status == model.TaskStatusFailure || status == model.TaskStatusSkipped {
		finishTime = now
	}
	data.Type = "upstream_account_sync"
	data.SystemTaskID = logger.taskID
	data.ChannelID = channel.Id
	data.ChannelName = channel.Name
	data.Platform = channel.UpstreamAccountSyncPlatform()
	data.Status = string(status)

	task := &model.Task{
		CreatedAt:  logger.submitTime,
		UpdatedAt:  now,
		TaskID:     logger.taskID,
		Platform:   constant.TaskPlatformSystem,
		UserId:     0,
		ChannelId:  channel.Id,
		Action:     constant.TaskActionUpstreamAccountSync,
		Status:     status,
		FailReason: failReason,
		SubmitTime: logger.submitTime,
		Progress:   progress,
		Properties: model.Properties{
			Input:             channel.Name,
			UpstreamModelName: data.Platform,
			OriginModelName:   "upstream_account_sync",
		},
	}
	if status == model.TaskStatusInProgress {
		task.StartTime = now
	}
	if finishTime > 0 {
		task.FinishTime = finishTime
	}
	task.SetData(data)

	var existing model.Task
	query := model.DB.
		Where("task_id = ? AND channel_id = ? AND action = ?", logger.taskID, channel.Id, constant.TaskActionUpstreamAccountSync).
		Limit(1).
		Find(&existing)
	if query.Error != nil {
		return query.Error
	}
	if query.RowsAffected == 0 {
		return model.DB.Create(task).Error
	}

	startTime := existing.StartTime
	if task.StartTime > 0 && startTime == 0 {
		startTime = task.StartTime
	}
	createdAt := existing.CreatedAt
	if createdAt <= 0 {
		createdAt = logger.submitTime
	}
	return model.DB.Model(&model.Task{}).
		Where("id = ?", existing.ID).
		Updates(map[string]interface{}{
			"created_at":  createdAt,
			"updated_at":  now,
			"platform":    constant.TaskPlatformSystem,
			"user_id":     0,
			"channel_id":  channel.Id,
			"action":      constant.TaskActionUpstreamAccountSync,
			"status":      status,
			"fail_reason": failReason,
			"submit_time": logger.submitTime,
			"start_time":  startTime,
			"finish_time": task.FinishTime,
			"progress":    progress,
			"properties":  task.Properties,
			"data":        task.Data,
		}).Error
}

// sanitizeUpstreamAccountSyncTaskLogText 清理写入任务日志的同步错误文本。
//
// 上游站点可能把 password、access_token、session cookie 或完整 API Key 回显进错误；
// 任务日志会长期展示给管理员排障，因此只能保存脱敏后的短文本。
func sanitizeUpstreamAccountSyncTaskLogText(value string, maxRunes int) string {
	text := strings.TrimSpace(value)
	for _, pattern := range upstreamAccountSyncTaskLogSecretPatterns {
		text = pattern.ReplaceAllStringFunc(text, func(match string) string {
			if strings.HasPrefix(strings.ToLower(match), "sk-") {
				return "[redacted-key]"
			}
			key, _, ok := strings.Cut(match, "=")
			if !ok {
				key, _, ok = strings.Cut(match, ":")
			}
			if !ok {
				return "[redacted-secret]"
			}
			return strings.TrimSpace(key) + "=[redacted]"
		})
	}
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

// automaticAccountConfigs 读取当前账号池的本地配置，供刷新时覆盖上游建议值。
//
// 不传 Enabled 指针是有意为之：RefreshChannelFromSnapshot 在没有显式状态时会
// 保留账号当前的完整状态，包括手动禁用、冷却和其他运行态；priority、weight、
// model、group 则通过非 nil 配置指针明确保留本地管理员设置。
func automaticAccountConfigs(channelID int) ([]AccountCreateConfig, error) {
	var accounts []model.ChannelAccount
	if err := model.DB.Where("channel_id = ?", channelID).Find(&accounts).Error; err != nil {
		return nil, err
	}

	configs := make([]AccountCreateConfig, 0, len(accounts))
	for index := range accounts {
		account := &accounts[index]
		metadata := readAccountSyncMetadata(account.OtherSettings)
		syncID := strings.TrimSpace(metadata.ExternalID)
		if syncID == "" {
			// 历史账号可能没有 external_id；ApplySyncIDs 会使用同样的脱敏 key
			// 作为稳定标识，因此这里使用脱敏 key 继续匹配而不暴露完整 key。
			syncID = strings.TrimSpace(maskKey(account.Key))
		}
		priority := account.Priority
		weight := account.Weight
		models := account.Models
		configs = append(configs, AccountCreateConfig{
			SyncID:     syncID,
			ExternalID: strings.TrimSpace(metadata.ExternalID),
			Models:     &models,
			Group:      account.Group,
			Priority:   &priority,
			Weight:     &weight,
		})
	}
	return configs, nil
}

// AutomaticSyncFailureError 将部分失败转换为任务终态错误，同时保留完整 summary。
func AutomaticSyncFailureError(summary *UpstreamAccountSyncSummary) error {
	if summary == nil || summary.FailedChannels == 0 {
		return nil
	}
	return fmt.Errorf("上游账号自动同步有 %d 个渠道失败", summary.FailedChannels)
}
