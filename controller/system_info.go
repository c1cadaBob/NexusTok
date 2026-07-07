// Package controller - system_info.go
// 该文件提供系统信息相关 API，当前先暴露多节点实例心跳列表。
// 后续 SystemTask 接入后，会继续在同一系统信息域下展示后台任务状态。
package controller

import (
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
)

// ListSystemInstances 返回所有已上报心跳的服务节点。
//
// 接口只允许 Root 访问，避免普通管理员看到主机名、版本、资源占用等运维信息。
func ListSystemInstances(c *gin.Context) {
	instances, err := model.ListSystemInstances()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	now := common.GetTimestamp()
	responses := make([]model.SystemInstanceResponse, 0, len(instances))
	for _, instance := range instances {
		responses = append(responses, instance.ToResponse(now))
	}

	common.ApiSuccess(c, responses)
}
