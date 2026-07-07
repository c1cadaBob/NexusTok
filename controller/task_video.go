// Package controller - task_video.go
// 该文件实现了视频生成任务的轮询更新逻辑
//
// 功能包括：
// - 批量更新视频任务状态
// - 单个视频任务状态查询和更新
// - 任务成功时的按 token 重新计费（补扣费/退还）
// - 任务失败时的自动退款
// - 响应体脱敏（移除 base64 数据）
//
// 主要函数：
// - UpdateVideoTaskAll：批量更新指定平台的所有视频任务
// - updateVideoTaskAll：更新单个渠道的所有视频任务
// - updateVideoSingleTask：更新单个视频任务
// - redactVideoResponseBody：脱敏视频响应体
package controller

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/relay"
	"github.com/c1cada/NexusTok/relay/channel"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
)

// UpdateVideoTaskAll 批量更新指定平台的所有视频任务
//
// 遍历所有渠道的任务 ID 列表，逐个渠道更新任务状态
// 单个渠道的更新失败不会影响其他渠道
//
// 参数：
//   - ctx: 请求上下文
//   - platform: 任务平台（如 kling、jimeng）
//   - taskChannelM: 渠道 ID -> 任务 ID 列表的映射
//   - taskM: 任务 ID -> 任务对象的映射
func UpdateVideoTaskAll(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	for channelId, taskIds := range taskChannelM {
		if err := updateVideoTaskAll(ctx, platform, channelId, taskIds, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Channel #%d failed to update video async tasks: %s", channelId, err.Error()))
		}
	}
	return nil
}

// updateVideoTaskAll 更新单个渠道的所有视频任务
//
// 流程：
// 1. 获取渠道信息
// 2. 初始化任务适配器
// 3. 遍历任务 ID 列表，逐个更新任务状态
//
// 参数：
//   - ctx: 请求上下文
//   - platform: 任务平台
//   - channelId: 渠道 ID
//   - taskIds: 任务 ID 列表
//   - taskM: 任务 ID -> 任务对象的映射
func updateVideoTaskAll(ctx context.Context, platform constant.TaskPlatform, channelId int, taskIds []string, taskM map[string]*model.Task) error {
	logger.LogInfo(ctx, fmt.Sprintf("Channel #%d pending video tasks: %d", channelId, len(taskIds)))
	if len(taskIds) == 0 {
		return nil
	}
	cacheGetChannel, err := model.CacheGetChannel(channelId)
	if err != nil {
		errUpdate := model.TaskBulkUpdate(taskIds, map[string]any{
			"fail_reason": fmt.Sprintf("Failed to get channel info, channel ID: %d", channelId),
			"status":      "FAILURE",
			"progress":    "100%",
		})
		if errUpdate != nil {
			common.SysLog(fmt.Sprintf("UpdateVideoTask error: %v", errUpdate))
		}
		return fmt.Errorf("CacheGetChannel failed: %w", err)
	}
	adaptor := relay.GetTaskAdaptor(platform)
	if adaptor == nil {
		return fmt.Errorf("video adaptor not found")
	}
	info := &relaycommon.RelayInfo{}
	info.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelBaseUrl: cacheGetChannel.GetBaseURL(),
	}
	info.ApiKey = cacheGetChannel.Key
	adaptor.Init(info)
	for _, taskId := range taskIds {
		if err := updateVideoSingleTask(ctx, adaptor, cacheGetChannel, taskId, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to update video task %s: %s", taskId, err.Error()))
		}
	}
	return nil
}

// updateVideoSingleTask 更新单个视频任务
//
// 流程：
// 1. 调用适配器的 FetchTask 查询上游任务状态
// 2. 解析响应（支持 NexusTok 标准格式和原生格式）
// 3. 根据任务状态更新进度：
//   - submitted: 10%
//   - queued: 20%
//   - in_progress: 30%
//   - success: 100%，触发按 token 重新计费
//   - failure: 100%，触发自动退款
//
// 4. 更新任务记录到数据库
// 5. 处理退款逻辑（防止重复退款）
//
// 参数：
//   - ctx: 请求上下文
//   - adaptor: 任务适配器
//   - channel: 渠道信息
//   - taskId: 任务 ID
//   - taskM: 任务 ID -> 任务对象的映射
func updateVideoSingleTask(ctx context.Context, adaptor channel.TaskAdaptor, channel *model.Channel, taskId string, taskM map[string]*model.Task) error {
	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}
	proxy := channel.GetSetting().Proxy

	task := taskM[taskId]
	if task == nil {
		logger.LogError(ctx, fmt.Sprintf("Task %s not found in taskM", taskId))
		return fmt.Errorf("task %s not found", taskId)
	}
	key := channel.Key

	privateData := task.PrivateData
	if privateData.Key != "" {
		key = privateData.Key
	}
	resp, err := adaptor.FetchTask(baseURL, key, map[string]any{
		"task_id": taskId,
		"action":  task.Action,
	}, proxy)
	if err != nil {
		return fmt.Errorf("fetchTask failed for task %s: %w", taskId, err)
	}
	//if resp.StatusCode != http.StatusOK {
	//return fmt.Errorf("get Video Task status code: %d", resp.StatusCode)
	//}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("readAll failed for task %s: %w", taskId, err)
	}

	logger.LogDebug(ctx, fmt.Sprintf("UpdateVideoSingleTask response: %s", string(responseBody)))

	taskResult := &relaycommon.TaskInfo{}
	// try parse as NexusTok response format
	var responseItems dto.TaskResponse[model.Task]
	if err = common.Unmarshal(responseBody, &responseItems); err == nil && responseItems.IsSuccess() {
		logger.LogDebug(ctx, fmt.Sprintf("UpdateVideoSingleTask parsed as new api response format: %+v", responseItems))
		t := responseItems.Data
		taskResult.TaskID = t.TaskID
		taskResult.Status = string(t.Status)
		taskResult.Url = t.FailReason
		taskResult.Progress = t.Progress
		taskResult.Reason = t.FailReason
		task.Data = t.Data
	} else if taskResult, err = adaptor.ParseTaskResult(responseBody); err != nil {
		return fmt.Errorf("parseTaskResult failed for task %s: %w", taskId, err)
	} else {
		task.Data = redactVideoResponseBody(responseBody)
	}

	logger.LogDebug(ctx, fmt.Sprintf("UpdateVideoSingleTask taskResult: %+v", taskResult))

	now := time.Now().Unix()
	if taskResult.Status == "" {
		//return fmt.Errorf("task %s status is empty", taskId)
		taskResult = relaycommon.FailTaskInfo("upstream returned empty status")
	}

	// 记录原本的状态，防止重复退款
	shouldRefund := false
	quota := task.Quota
	preStatus := task.Status

	task.Status = model.TaskStatus(taskResult.Status)
	switch taskResult.Status {
	case model.TaskStatusSubmitted:
		task.Progress = "10%"
	case model.TaskStatusQueued:
		task.Progress = "20%"
	case model.TaskStatusInProgress:
		task.Progress = "30%"
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusSuccess:
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		if !(len(taskResult.Url) > 5 && taskResult.Url[:5] == "data:") {
			task.FailReason = taskResult.Url
		}

		// 如果返回了 total_tokens 并且配置了模型倍率(非固定价格),则重新计费
		if taskResult.TotalTokens > 0 {
			// 获取模型名称
			var taskData map[string]interface{}
			if err := common.Unmarshal(task.Data, &taskData); err == nil {
				if modelName, ok := taskData["model"].(string); ok && modelName != "" {
					// 获取模型价格和倍率
					modelRatio, hasRatioSetting, _ := ratio_setting.GetModelRatio(modelName)
					// 只有配置了倍率(非固定价格)时才按 token 重新计费
					if hasRatioSetting && modelRatio > 0 {
						// 获取用户和组的倍率信息
						group := task.Group
						if group == "" {
							user, err := model.GetUserById(task.UserId, false)
							if err == nil {
								group = user.Group
							}
						}
						if group != "" {
							groupRatio := ratio_setting.GetGroupRatio(group)
							userGroupRatio, hasUserGroupRatio := ratio_setting.GetGroupGroupRatio(group, group)

							var finalGroupRatio float64
							if hasUserGroupRatio {
								finalGroupRatio = userGroupRatio
							} else {
								finalGroupRatio = groupRatio
							}

							// 计算实际应扣费额度: totalTokens * modelRatio * groupRatio。
							// 上游返回 tokens、管理员模型倍率和分组倍率共同决定该值，统一使用
							// QuotaFromFloatChecked 防止极端组合在 int 转换时溢出成负数。
							actualQuota, clamp := common.QuotaFromFloatChecked(float64(taskResult.TotalTokens) * modelRatio * finalGroupRatio)
							if clamp != nil {
								logger.LogWarn(ctx, fmt.Sprintf("视频任务额度换算触发饱和保护 task=%s op=%s kind=%s original=%g clamped=%d user=%d",
									task.TaskID, clamp.Op, clamp.Kind, clamp.Original, clamp.Clamped, task.UserId))
							}

							// 计算差额
							preConsumedQuota := task.Quota
							quotaDelta := actualQuota - preConsumedQuota

							if quotaDelta > 0 {
								// 需要补扣费
								logger.LogInfo(ctx, fmt.Sprintf("视频任务 %s 预扣费后补扣费：%s（实际消耗：%s，预扣费：%s，tokens：%d）",
									task.TaskID,
									logger.LogQuota(quotaDelta),
									logger.LogQuota(actualQuota),
									logger.LogQuota(preConsumedQuota),
									taskResult.TotalTokens,
								))
								if err := model.DecreaseUserQuota(task.UserId, quotaDelta, false); err != nil {
									logger.LogError(ctx, fmt.Sprintf("补扣费失败: %s", err.Error()))
								} else {
									model.UpdateUserUsedQuotaAndRequestCount(task.UserId, quotaDelta)
									model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
									task.Quota = actualQuota // 更新任务记录的实际扣费额度

									// 记录消费日志
									logContent := fmt.Sprintf("视频任务成功补扣费，模型倍率 %.2f，分组倍率 %.2f，tokens %d，预扣费 %s，实际扣费 %s，补扣费 %s",
										modelRatio, finalGroupRatio, taskResult.TotalTokens,
										logger.LogQuota(preConsumedQuota), logger.LogQuota(actualQuota), logger.LogQuota(quotaDelta))
									model.RecordLog(task.UserId, model.LogTypeSystem, logContent)
								}
							} else if quotaDelta < 0 {
								// 需要退还多扣的费用
								refundQuota := -quotaDelta
								logger.LogInfo(ctx, fmt.Sprintf("视频任务 %s 预扣费后返还：%s（实际消耗：%s，预扣费：%s，tokens：%d）",
									task.TaskID,
									logger.LogQuota(refundQuota),
									logger.LogQuota(actualQuota),
									logger.LogQuota(preConsumedQuota),
									taskResult.TotalTokens,
								))
								if err := model.IncreaseUserQuota(task.UserId, refundQuota, false); err != nil {
									logger.LogError(ctx, fmt.Sprintf("退还预扣费失败: %s", err.Error()))
								} else {
									task.Quota = actualQuota // 更新任务记录的实际扣费额度

									// 记录退款日志
									logContent := fmt.Sprintf("视频任务成功退还多扣费用，模型倍率 %.2f，分组倍率 %.2f，tokens %d，预扣费 %s，实际扣费 %s，退还 %s",
										modelRatio, finalGroupRatio, taskResult.TotalTokens,
										logger.LogQuota(preConsumedQuota), logger.LogQuota(actualQuota), logger.LogQuota(refundQuota))
									model.RecordLog(task.UserId, model.LogTypeSystem, logContent)
								}
							} else {
								// quotaDelta == 0, 预扣费刚好准确
								logger.LogInfo(ctx, fmt.Sprintf("视频任务 %s 预扣费准确（%s，tokens：%d）",
									task.TaskID, logger.LogQuota(actualQuota), taskResult.TotalTokens))
							}
						}
					}
				}
			}
		}
	case model.TaskStatusFailure:
		logger.LogJson(ctx, fmt.Sprintf("Task %s failed", taskId), task)
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskResult.Reason
		logger.LogInfo(ctx, fmt.Sprintf("Task %s failed: %s", task.TaskID, task.FailReason))
		taskResult.Progress = "100%"
		if quota != 0 {
			if preStatus != model.TaskStatusFailure {
				shouldRefund = true
			} else {
				logger.LogWarn(ctx, fmt.Sprintf("Task %s already in failure status, skip refund", task.TaskID))
			}
		}
	default:
		return fmt.Errorf("unknown task status %s for task %s", taskResult.Status, taskId)
	}
	if taskResult.Progress != "" {
		task.Progress = taskResult.Progress
	}
	if err := task.Update(); err != nil {
		common.SysLog("UpdateVideoTask task error: " + err.Error())
		shouldRefund = false
	}

	if shouldRefund {
		// 任务失败且之前状态不是失败才退还额度，防止重复退还
		if err := model.IncreaseUserQuota(task.UserId, quota, false); err != nil {
			logger.LogWarn(ctx, "Failed to increase user quota: "+err.Error())
		}
		logContent := fmt.Sprintf("Video async task failed %s, refund %s", task.TaskID, logger.LogQuota(quota))
		model.RecordLog(task.UserId, model.LogTypeSystem, logContent)
	}

	return nil
}

// redactVideoResponseBody 脱敏视频响应体
//
// 移除响应中的 base64 编码数据，避免存储大量二进制数据
// 处理逻辑：
// - 删除 response.bytesBase64Encoded 字段
// - 截断 response.video 中的 base64 数据
// - 删除 response.videos 中每个元素的 bytesBase64Encoded 字段
//
// 参数：
//   - body: 原始响应体
//
// 返回：
//   - []byte: 脱敏后的响应体
func redactVideoResponseBody(body []byte) []byte {
	var m map[string]any
	if err := common.Unmarshal(body, &m); err != nil {
		return body
	}
	resp, _ := m["response"].(map[string]any)
	if resp != nil {
		delete(resp, "bytesBase64Encoded")
		if v, ok := resp["video"].(string); ok {
			resp["video"] = truncateBase64(v)
		}
		if vs, ok := resp["videos"].([]any); ok {
			for i := range vs {
				if vm, ok := vs[i].(map[string]any); ok {
					delete(vm, "bytesBase64Encoded")
				}
			}
		}
	}
	b, err := common.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

// truncateBase64 截断 base64 字符串
//
// 保留前 maxKeep 个字符，超过部分用 "..." 替代
//
// 参数：
//   - s: base64 字符串
//
// 返回：
//   - string: 截断后的字符串
func truncateBase64(s string) string {
	const maxKeep = 256
	if len(s) <= maxKeep {
		return s
	}
	return s[:maxKeep] + "..."
}
