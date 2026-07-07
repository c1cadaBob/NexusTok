// Package controller - system_task.go
// 该文件提供系统任务观测 API。当前只暴露 Root 只读接口，用于在系统信息页展示
// 后台任务状态；实际任务创建和执行会在对应业务能力接入 runner 后再逐步开放。
package controller

import (
	"net/http"
	"strconv"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
)

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
