// Package controller - rankings.go
// 该文件实现了排行榜功能的 API 控制器
//
// 排行榜功能展示用户的 API 使用量排名
// 可通过系统配置启用或禁用
//
// 主要 API：
// - GetRankings：获取排行榜数据
package controller

import (
	"net/http"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/service"
	"github.com/gin-gonic/gin"
)

// isRankingsEnabled 检查排行榜功能是否启用
//
// 从系统配置中读取 HeaderNavModules 配置
// 如果配置中包含 rankings.enabled 字段，则使用该值
// 否则默认启用
//
// 返回值：
//   - bool: 排行榜功能是否启用
func isRankingsEnabled() bool {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap["HeaderNavModules"]
	common.OptionMapRWMutex.RUnlock()

	if raw == "" {
		return true
	}

	var parsed map[string]interface{}
	if err := common.Unmarshal([]byte(raw), &parsed); err != nil {
		return true
	}
	rankings, ok := parsed["rankings"]
	if !ok {
		return true
	}
	switch v := rankings.(type) {
	case bool:
		return v
	case map[string]interface{}:
		if enabled, ok := v["enabled"]; ok {
			if b, ok := enabled.(bool); ok {
				return b
			}
		}
		return true
	}
	return true
}

// GetRankings 获取排行榜数据
//
// 如果排行榜功能未启用，返回 403 错误
//
// 参数：
//   - c: Gin 上下文
func GetRankings(c *gin.Context) {
	// 检查排行榜是否启用
	if !isRankingsEnabled() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "rankings is disabled",
		})
		return
	}

	result, err := service.GetRankingsSnapshot(c.DefaultQuery("period", "week"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
