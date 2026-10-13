// Package controller - system_update.go
// 该文件提供系统维护更新 API。控制器只负责权限后的请求编排，具体 GitHub Release
// 查询、checksum 校验、二进制替换和回滚逻辑集中在 service 层，便于测试和复用。
package controller

import (
	"net/http"
	"strconv"

	"github.com/c1cadaBob/NexusTok/common"
	"github.com/c1cadaBob/NexusTok/service"

	"github.com/gin-gonic/gin"
)

// GetLatestSystemUpdate 检查 GitHub Release 中是否存在可用的新版本。
func GetLatestSystemUpdate(c *gin.Context) {
	force, _ := strconv.ParseBool(c.DefaultQuery("force", "false"))
	info, err := service.CheckLatestSystemUpdate(c.Request.Context(), force)
	if err != nil {
		common.ApiErrorMsg(c, service.SystemUpdateErrorMessage(err))
		return
	}
	common.ApiSuccess(c, info)
}

// ApplySystemUpdate 创建系统更新后台任务。
func ApplySystemUpdate(c *gin.Context) {
	task, err := service.StartSystemUpdateTask(c.Request.Context())
	if err != nil {
		common.ApiErrorMsg(c, service.SystemUpdateErrorMessage(err))
		return
	}
	payload := service.SystemUpdateTaskPayload{}
	_ = task.DecodePayload(&payload)
	recordManageAudit(c, "system_update.apply", map[string]any{
		"target_version": payload.TargetVersion,
		"result_status":  "accepted",
	})
	common.ApiSuccess(c, task.ToResponse())
}

// RollbackSystemUpdate 创建系统回滚后台任务。
func RollbackSystemUpdate(c *gin.Context) {
	var request struct {
		TargetVersion string `json:"target_version"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "target_version is required",
		})
		return
	}
	task, err := service.StartSystemRollbackTask(c.Request.Context(), request.TargetVersion)
	if err != nil {
		common.ApiErrorMsg(c, service.SystemUpdateErrorMessage(err))
		return
	}
	recordManageAudit(c, "system_update.rollback", map[string]any{
		"target_version": request.TargetVersion,
		"result_status":  "accepted",
	})
	common.ApiSuccess(c, task.ToResponse())
}

// RestartSystemUpdate 调度服务重启。
func RestartSystemUpdate(c *gin.Context) {
	result := service.RestartSystemService()
	resultStatus := "manual_required"
	if result.RestartScheduled {
		resultStatus = "scheduled"
	} else if result.RestartSupported {
		resultStatus = "accepted"
	}
	recordManageAudit(c, "system_update.restart", map[string]any{"result_status": resultStatus})
	common.ApiSuccess(c, result)
}
