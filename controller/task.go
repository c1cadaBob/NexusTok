// Package controller - task.go
// 该文件实现了异步任务管理的 API 控制器
//
// 异步任务用于处理耗时操作（如视频生成、图像处理等）
// 功能包括：
// - 管理员查询所有任务（支持分页、筛选）
// - 用户查询自己的任务
// - 任务轮询更新（委托给 service 层）
//
// 主要 API：
// - GetAllTask：管理员查询所有任务
// - GetUserTask：用户查询自己的任务
// - UpdateTaskBulk：启动任务轮询循环
package controller

import (
	"strconv"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/relay"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// UpdateTaskBulk 启动批量任务轮询
//
// 这是一个薄入口，实际轮询逻辑在 service 层的 TaskPollingLoop 中实现
func UpdateTaskBulk() {
	service.TaskPollingLoop()
}

// GetAllTask 管理员查询所有任务
//
// 支持的查询参数：
//   - platform: 任务平台（如 kling、jimeng）
//   - task_id: 任务 ID 精确匹配
//   - status: 任务状态筛选
//   - action: 任务动作类型
//   - start_timestamp: 开始时间戳
//   - end_timestamp: 结束时间戳
//   - channel_id: 渠道 ID
//
// 返回分页任务列表，包含用户信息
func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 解析其他查询参数
	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ChannelID:      c.Query("channel_id"),
	}

	items := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllTasks(queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, true))
	common.ApiSuccess(c, pageInfo)
}

// GetUserTask 用户查询自己的任务
//
// 支持与 GetAllTask 相同的查询参数（除 channel_id 外）
// 返回分页任务列表，不包含用户信息
func GetUserTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}

	items := model.TaskGetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllUserTask(userId, queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, false))
	common.ApiSuccess(c, pageInfo)
}

// tasksToDto 将任务模型列表转换为 DTO 列表
//
// 如果 fillUser 为 true，会批量查询用户信息并填充到任务的 Username 字段
// 使用缓存减少数据库查询
//
// 参数：
//   - tasks: 任务模型列表
//   - fillUser: 是否填充用户信息
//
// 返回：
//   - []*dto.TaskDto: 任务 DTO 列表
func tasksToDto(tasks []*model.Task, fillUser bool) []*dto.TaskDto {
	var userIdMap map[int]*model.UserBase
	if fillUser {
		userIdMap = make(map[int]*model.UserBase)
		userIds := types.NewSet[int]()
		for _, task := range tasks {
			if task.UserId == 0 {
				continue
			}
			userIds.Add(task.UserId)
		}
		for _, userId := range userIds.Items() {
			cacheUser, err := model.GetUserCache(userId)
			if err == nil {
				userIdMap[userId] = cacheUser
			}
		}
	}
	result := make([]*dto.TaskDto, len(tasks))
	for i, task := range tasks {
		if fillUser {
			if task.UserId == 0 {
				task.Username = "System"
			} else if user, ok := userIdMap[task.UserId]; ok {
				task.Username = user.Username
			}
		}
		result[i] = relay.TaskModel2Dto(task)
	}
	return result
}
