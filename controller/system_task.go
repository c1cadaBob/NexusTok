// Package controller - system_task.go
// 该文件提供系统任务观测 API。当前只暴露 Root 只读接口，用于在系统信息页展示
// 后台任务状态；实际任务创建和执行会在对应业务能力接入 runner 后再逐步开放。
package controller

import (
	"net/http"
	"strconv"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"

	"github.com/gin-gonic/gin"
)

// CreateLogCleanupSystemTask 创建日志清理系统任务。
//
// 日志清理可能删除大量历史记录，不能再绑定到一次 HTTP 请求内同步执行；
// 该接口只创建任务并立即返回，由 SystemTask runner 按租约异步执行。
func CreateLogCleanupSystemTask(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	if targetTimestamp == 0 {
		common.ApiErrorMsg(c, "target timestamp is required")
		return
	}

	task, err := service.StartLogCleanupTask(targetTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, task.ToResponse())
}

// CreateUpstreamAccountScheduleRefreshSystemTask 创建同步密钥调度建议统一刷新任务。
//
// 该任务不会在服务启动时自动修改数据，必须由 Root 管理员显式触发。执行时只扫描
// upstream_account_sync 同步密钥，将 priority 统一为 0，并按元数据倍率重算 weight。
func CreateUpstreamAccountScheduleRefreshSystemTask(c *gin.Context) {
	task, _, err := service.EnqueueSystemTask(model.SystemTaskTypeUpstreamAccountScheduleRefresh, nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, task.ToResponse())
}

// GetCurrentSystemTask 返回指定类型当前活跃的系统任务。
func GetCurrentSystemTask(c *gin.Context) {
	taskType := c.Query("type")
	if taskType == "" {
		common.ApiErrorMsg(c, "type is required")
		return
	}

	task, err := model.GetActiveSystemTask(taskType)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if task == nil {
		common.ApiSuccess(c, nil)
		return
	}

	common.ApiSuccess(c, task.ToResponse())
}

// ListSystemTasks 按创建时间倒序列出最近的系统任务。
func ListSystemTasks(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	tasks, err := model.ListSystemTasks(limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	responses := make([]model.SystemTaskResponse, 0, len(tasks))
	for _, task := range tasks {
		responses = append(responses, task.ToResponse())
	}

	common.ApiSuccess(c, responses)
}

// GetSystemTask 根据任务 ID 返回单条系统任务。
func GetSystemTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		common.ApiErrorMsg(c, "task id is required")
		return
	}

	task, err := model.GetSystemTaskByTaskID(taskID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "task not found",
		})
		return
	}

	common.ApiSuccess(c, task.ToResponse())
}
