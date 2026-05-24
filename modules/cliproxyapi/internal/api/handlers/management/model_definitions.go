// management - model_definitions.go
// 静态模型定义查询端点。
// 该模块提供按渠道（channel）查询静态模型元数据的接口，
// 用于管理面板展示各渠道支持的模型列表及其属性。
package management

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
)

// GetStaticModelDefinitions 返回指定渠道的静态模型元数据。
// 渠道名称可通过路径参数 (:channel) 或查询参数 (?channel=...) 提供。
// 如果渠道名称为空或未知，返回相应的错误响应。
// 返回的 JSON 包含渠道名称和模型定义列表。
func (h *Handler) GetStaticModelDefinitions(c *gin.Context) {
	channel := strings.TrimSpace(c.Param("channel"))
	if channel == "" {
		channel = strings.TrimSpace(c.Query("channel"))
	}
	if channel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "channel is required"})
		return
	}

	models := registry.GetStaticModelDefinitionsByChannel(channel)
	if models == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown channel", "channel": channel})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"channel": strings.ToLower(strings.TrimSpace(channel)),
		"models":  models,
	})
}
