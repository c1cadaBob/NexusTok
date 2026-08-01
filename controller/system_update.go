// Package controller - system_update.go
// 该文件提供系统维护更新 API。控制器只负责权限后的请求编排，具体 GitHub Release
// 查询、checksum 校验、二进制替换和回滚逻辑集中在 service 层，便于测试和复用。
package controller

import (
	"strconv"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/service"

	"github.com/gin-gonic/gin"
)

// GetLatestSystemUpdate 检查 GitHub Release 中是否存在可用的新版本。
func GetLatestSystemUpdate(c *gin.Context) {
	force, _ := strconv.ParseBool(c.DefaultQuery("force", "false"))
	info, err := service.CheckLatestSystemUpdate(c.Request.Context(), force)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, info)
}

// ApplySystemUpdate 创建系统更新后台任务。
func ApplySystemUpdate(c *gin.Context) {
	task, err := service.StartSystemUpdateTask(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, task.ToResponse())
}

// RollbackSystemUpdate 创建系统回滚后台任务。
func RollbackSystemUpdate(c *gin.Context) {
	task, err := service.StartSystemRollbackTask()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, task.ToResponse())
}

// RestartSystemUpdate 调度服务重启。
func RestartSystemUpdate(c *gin.Context) {
	common.ApiSuccess(c, service.RestartSystemService())
}
