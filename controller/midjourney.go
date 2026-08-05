// Package controller - midjourney.go
// 该文件实现了 Midjourney 图像生成任务管理的 API 控制器
//
// Midjourney 是一个 AI 图像生成服务，通过中转 API 进行调用
// 本文件主要功能：
// - 后台任务轮询：定期检查未完成的 Midjourney 任务状态
// - 任务状态同步：将上游任务状态同步到本地数据库
// - 额度退还：任务失败时自动退还用户额度
// - 任务查询：管理员和用户查询任务列表
//
// 主要 API：
// - GetAllMidjourney：管理员获取所有任务列表
// - GetUserMidjourney：用户获取自己的任务列表
// - UpdateMidjourneyTaskBulk：后台任务状态轮询（非 API，启动时运行）
package controller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/setting"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/setting/system_setting"

	"github.com/gin-gonic/gin"
)

const midjourneyPollingInterval = 15 * time.Second

// MidjourneyPollSummary 是绘图任务轮询的一次执行摘要。
type MidjourneyPollSummary struct {
	Pending       int            `json:"pending"`
	NullTaskIDs   int            `json:"null_task_ids"`
	Channels      map[string]int `json:"channels,omitempty"`
	ChannelErrors int            `json:"channel_errors"`
	Updated       int            `json:"updated"`
	Refunded      int            `json:"refunded"`
}

type midjourneyPollHandler struct{}

func (midjourneyPollHandler) Type() string {
	return model.SystemTaskTypeMidjourneyPoll
}

func (midjourneyPollHandler) Enabled() bool {
	return constant.UpdateTask && operation_setting.GetSystemTaskSetting().MidjourneyPollEnabled
}

func (midjourneyPollHandler) Interval() time.Duration {
	return midjourneyPollingInterval
}

func (midjourneyPollHandler) NewPayload() any {
	return nil
}

func (midjourneyPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	if !operation_setting.GetSystemTaskSetting().MidjourneyPollEnabled {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, map[string]any{
			"skipped":     true,
			"skip_reason": "绘图任务轮询已关闭",
		}, nil)
		return
	}
	summary, err := RunMidjourneyPollingOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		if finishErr := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusFailed, summary, err.Error()); finishErr != nil {
			logMidjourneySystemTaskError(ctx, task, finishErr)
		}
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, summary, ""); err != nil {
		logMidjourneySystemTaskError(ctx, task, err)
	}
}

func init() {
	service.RegisterSystemTaskHandler(midjourneyPollHandler{})
}

func logMidjourneySystemTaskError(ctx context.Context, task *model.SystemTask, err error) {
	logger.LogWarn(ctx, fmt.Sprintf("midjourney poll system task %s update failed: %v", task.TaskID, err))
}

// UpdateMidjourneyTaskBulk 兼容旧启动入口，并创建一次绘图任务轮询系统任务。
//
// 后续周期调度由 midjourneyPollHandler 交给 SystemTask scheduler 完成，避免多节点
// 部署时每个进程都启动独立 goroutine 重复轮询同一批 Midjourney 任务。
func UpdateMidjourneyTaskBulk() {
	if !common.IsMasterNode || !(midjourneyPollHandler{}).Enabled() {
		return
	}
	if _, _, err := service.EnqueueSystemTask(model.SystemTaskTypeMidjourneyPoll, nil); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("midjourney poll system task enqueue failed: %v", err))
	}
}

// RunMidjourneyPollingOnce 执行一次 Midjourney 任务状态轮询。
//
// 处理流程保持旧轮询逻辑：读取未完成任务、修复缺失上游 ID 的异常任务、按渠道批量
// 查询上游状态、同步本地字段，并在失败终态通过 CAS 赢得状态迁移后退还预扣额度。
func RunMidjourneyPollingOnce(ctx context.Context, report func(processed, total int)) (MidjourneyPollSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	summary := MidjourneyPollSummary{
		Channels: map[string]int{},
	}
	tasks := model.GetAllUnFinishTasks()
	summary.Pending = len(tasks)
	if len(tasks) == 0 {
		if report != nil {
			report(0, 0)
		}
		return summary, nil
	}

	logger.LogInfo(ctx, fmt.Sprintf("检测到未完成的任务数有: %v", len(tasks)))
	taskChannelM := make(map[int][]string)
	taskM := make(map[string]*model.Midjourney)
	nullTaskIds := make([]int, 0)
	for _, task := range tasks {
		if task.MjId == "" {
			// 缺少上游任务 ID 的记录无法再查询 provider，直接终止，避免无限占用轮询队列。
			nullTaskIds = append(nullTaskIds, task.Id)
			continue
		}
		taskM[task.MjId] = task
		taskChannelM[task.ChannelId] = append(taskChannelM[task.ChannelId], task.MjId)
	}
	if len(nullTaskIds) > 0 {
		err := model.MjBulkUpdateByTaskIds(nullTaskIds, map[string]any{
			"status":   "FAILURE",
			"progress": "100%",
		})
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("Fix null mj_id task error: %v", err))
			return summary, err
		}
		summary.NullTaskIDs = len(nullTaskIds)
		logger.LogInfo(ctx, fmt.Sprintf("Fix null mj_id task success: %v", nullTaskIds))
	}

	totalChannels := len(taskChannelM)
	processedChannels := 0
	if report != nil {
		report(0, totalChannels)
	}
	for channelId, taskIds := range taskChannelM {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		logger.LogInfo(ctx, fmt.Sprintf("渠道 #%d 未完成的任务有: %d", channelId, len(taskIds)))
		if len(taskIds) == 0 {
			continue
		}
		summary.Channels[fmt.Sprintf("%d", channelId)] = len(taskIds)
		channelSummary, err := updateMidjourneyChannelTasks(ctx, channelId, taskIds, taskM)
		summary.ChannelErrors += channelSummary.ChannelErrors
		summary.Updated += channelSummary.Updated
		summary.Refunded += channelSummary.Refunded
		if err != nil {
			return summary, err
		}
		processedChannels++
		if report != nil {
			report(processedChannels, totalChannels)
		}
	}
	if report != nil {
		report(totalChannels, totalChannels)
	}
	return summary, nil
}

func updateMidjourneyChannelTasks(ctx context.Context, channelId int, taskIds []string, taskM map[string]*model.Midjourney) (MidjourneyPollSummary, error) {
	summary := MidjourneyPollSummary{}
	if err := ctx.Err(); err != nil {
		return summary, err
	}
	midjourneyChannel, err := model.CacheGetChannel(channelId)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("CacheGetChannel: %v", err))
		summary.ChannelErrors++
		err := model.MjBulkUpdate(taskIds, map[string]any{
			"fail_reason": fmt.Sprintf("获取渠道信息失败，请联系管理员，渠道ID：%d", channelId),
			"status":      "FAILURE",
			"progress":    "100%",
		})
		if err != nil {
			logger.LogInfo(ctx, fmt.Sprintf("UpdateMidjourneyTask error: %v", err))
		}
		return summary, nil
	}

	requestUrl := fmt.Sprintf("%s/mj/task/list-by-condition", midjourneyChannel.GetBaseURL())
	body, err := common.Marshal(map[string]any{
		"ids": taskIds,
	})
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Get Task request body error: %v", err))
		summary.ChannelErrors++
		return summary, nil
	}

	requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, requestUrl, bytes.NewBuffer(body))
	if err != nil {
		cancel()
		logger.LogError(ctx, fmt.Sprintf("Get Task error: %v", err))
		summary.ChannelErrors++
		return summary, nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("mj-api-secret", midjourneyChannel.Key)

	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		cancel()
		logger.LogError(ctx, fmt.Sprintf("Get Task Do req error: %v", err))
		summary.ChannelErrors++
		if ctxErr := ctx.Err(); ctxErr != nil {
			return summary, ctxErr
		}
		return summary, nil
	}
	defer cancel()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.LogError(ctx, fmt.Sprintf("Get Task status code: %d", resp.StatusCode))
		summary.ChannelErrors++
		return summary, nil
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Get Mjp Task parse body error: %v", err))
		summary.ChannelErrors++
		if ctxErr := ctx.Err(); ctxErr != nil {
			return summary, ctxErr
		}
		return summary, nil
	}
	var responseItems []dto.MidjourneyDto
	if err := common.Unmarshal(responseBody, &responseItems); err != nil {
		logger.LogError(ctx, fmt.Sprintf("Get Mjp Task parse body error2: %v, body: %s", err, string(responseBody)))
		summary.ChannelErrors++
		return summary, nil
	}

	for _, responseItem := range responseItems {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		task := taskM[responseItem.MjId]
		if task == nil {
			continue
		}
		updated, refunded := syncMidjourneyTaskFromResponse(ctx, task, responseItem)
		if updated {
			summary.Updated++
		}
		if refunded {
			summary.Refunded++
		}
	}
	return summary, nil
}

func syncMidjourneyTaskFromResponse(ctx context.Context, task *model.Midjourney, responseItem dto.MidjourneyDto) (bool, bool) {
	useTime := (time.Now().UnixNano() / int64(time.Millisecond)) - task.SubmitTime
	// 如果时间超过一小时且进度不是 100%，则认为上游任务已经超时失败。
	if useTime > 3600000 && task.Progress != "100%" {
		responseItem.FailReason = "上游任务超时（超过1小时）"
		responseItem.Status = "FAILURE"
	}
	if !checkMjTaskNeedUpdate(task, responseItem) {
		return false, false
	}
	preStatus := task.Status
	task.Code = 1
	task.Progress = responseItem.Progress
	task.PromptEn = responseItem.PromptEn
	task.State = responseItem.State
	task.SubmitTime = responseItem.SubmitTime
	task.StartTime = responseItem.StartTime
	task.FinishTime = responseItem.FinishTime
	task.ImageUrl = responseItem.ImageUrl
	task.Status = responseItem.Status
	task.FailReason = responseItem.FailReason
	if responseItem.Properties != nil {
		propertiesStr, err := common.Marshal(responseItem.Properties)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("序列化 Properties 失败: %v", err))
		} else {
			task.Properties = string(propertiesStr)
		}
	}
	if responseItem.Buttons != nil {
		buttonStr, err := common.Marshal(responseItem.Buttons)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("序列化 Buttons 失败: %v", err))
		} else {
			task.Buttons = string(buttonStr)
		}
	}
	task.VideoUrl = responseItem.VideoUrl

	// VideoUrls 是上游返回的数组，本地历史字段使用字符串存储，需继续序列化保存。
	if responseItem.VideoUrls != nil && len(responseItem.VideoUrls) > 0 {
		videoUrlsStr, err := common.Marshal(responseItem.VideoUrls)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("序列化 VideoUrls 失败: %v", err))
			task.VideoUrls = "[]"
		} else {
			task.VideoUrls = string(videoUrlsStr)
		}
	} else {
		task.VideoUrls = ""
	}

	shouldReturnQuota := false
	if (task.Progress != "100%" && responseItem.FailReason != "") || (task.Progress == "100%" && task.Status == "FAILURE") {
		logger.LogInfo(ctx, task.MjId+" 构建失败，"+task.FailReason)
		task.Progress = "100%"
		if task.Quota != 0 {
			shouldReturnQuota = true
		}
	}
	won, err := task.UpdateWithStatus(preStatus)
	if err != nil {
		logger.LogError(ctx, "UpdateMidjourneyTask task error: "+err.Error())
		return false, false
	}
	if won && shouldReturnQuota {
		err = model.IncreaseUserQuota(task.UserId, task.Quota, false)
		if err != nil {
			logger.LogError(ctx, "fail to increase user quota: "+err.Error())
		}
		model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
			UserId:    task.UserId,
			LogType:   model.LogTypeRefund,
			Content:   "",
			ChannelId: task.ChannelId,
			ModelName: service.CovertMjpActionToModelName(task.Action),
			Quota:     task.Quota,
			Other: map[string]interface{}{
				"task_id": task.MjId,
				"reason":  "构图失败",
			},
		})
		return true, true
	}
	return won, false
}

// checkMjTaskNeedUpdate 检查 Midjourney 任务是否需要更新
//
// 比较旧任务和新任务的各个字段，判断是否有变化
//
// 参数：
//   - oldTask: 本地存储的旧任务
//   - newTask: 从上游获取的新任务状态
//
// 返回值：
//   - bool: 是否需要更新
func checkMjTaskNeedUpdate(oldTask *model.Midjourney, newTask dto.MidjourneyDto) bool {
	if oldTask.Code != 1 {
		return true
	}
	if oldTask.Progress != newTask.Progress {
		return true
	}
	if oldTask.PromptEn != newTask.PromptEn {
		return true
	}
	if oldTask.State != newTask.State {
		return true
	}
	if oldTask.SubmitTime != newTask.SubmitTime {
		return true
	}
	if oldTask.StartTime != newTask.StartTime {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if oldTask.ImageUrl != newTask.ImageUrl {
		return true
	}
	if oldTask.Status != newTask.Status {
		return true
	}
	if oldTask.FailReason != newTask.FailReason {
		return true
	}
	if oldTask.FinishTime != newTask.FinishTime {
		return true
	}
	if oldTask.Progress != "100%" && newTask.FailReason != "" {
		return true
	}
	// 检查 VideoUrl 是否需要更新
	if oldTask.VideoUrl != newTask.VideoUrl {
		return true
	}
	// 检查 VideoUrls 是否需要更新
	if newTask.VideoUrls != nil && len(newTask.VideoUrls) > 0 {
		newVideoUrlsStr, _ := common.Marshal(newTask.VideoUrls)
		if oldTask.VideoUrls != string(newVideoUrlsStr) {
			return true
		}
	} else if oldTask.VideoUrls != "" {
		// 如果新数据没有 VideoUrls 但旧数据有，需要更新（清空）
		return true
	}

	return false
}

// GetAllMidjourney 管理员获取所有 Midjourney 任务列表
//
// 支持分页和多种过滤条件
//
// 查询参数：
//   - channel_id: 渠道 ID 过滤
//   - mj_id: Midjourney 任务 ID 过滤
//   - start_timestamp: 开始时间戳过滤
//   - end_timestamp: 结束时间戳过滤
func GetAllMidjourney(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	// 解析其他查询参数
	queryParams := model.TaskQueryParams{
		ChannelID:      c.Query("channel_id"),
		MjID:           c.Query("mj_id"),
		StartTimestamp: c.Query("start_timestamp"),
		EndTimestamp:   c.Query("end_timestamp"),
	}

	items := model.GetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.CountAllTasks(queryParams)

	if setting.MjForwardUrlEnabled {
		for i, midjourney := range items {
			midjourney.ImageUrl = system_setting.ServerAddress + "/mj/image/" + midjourney.MjId
			items[i] = midjourney
		}
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// GetUserMidjourney 用户获取自己的 Midjourney 任务列表
//
// 支持分页和时间范围过滤
//
// 查询参数：
//   - mj_id: Midjourney 任务 ID 过滤
//   - start_timestamp: 开始时间戳过滤
//   - end_timestamp: 结束时间戳过滤
func GetUserMidjourney(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	queryParams := model.TaskQueryParams{
		MjID:           c.Query("mj_id"),
		StartTimestamp: c.Query("start_timestamp"),
		EndTimestamp:   c.Query("end_timestamp"),
	}

	items := model.GetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.CountAllUserTask(userId, queryParams)

	if setting.MjForwardUrlEnabled {
		for i, midjourney := range items {
			midjourney.ImageUrl = system_setting.ServerAddress + "/mj/image/" + midjourney.MjId
			items[i] = midjourney
		}
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}
