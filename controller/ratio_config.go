// Package controller - ratio_config.go
// 该文件实现了倍率配置公开查询的 API 控制器
//
// 倍率配置接口允许用户查看当前的模型倍率设置
// 需要在系统配置中启用"暴露倍率"功能
//
// 主要 API：
// - GetRatioConfig：获取公开的倍率配置
package controller

import (
	"net/http"

	"github.com/c1cada/NexusTok/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// GetRatioConfig 获取公开的倍率配置
//
// 如果未启用"暴露倍率"功能，返回 403 错误
// 返回数据包括模型倍率、补全倍率、缓存倍率等
func GetRatioConfig(c *gin.Context) {
	if !ratio_setting.IsExposeRatioEnabled() {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "倍率配置接口未启用",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    ratio_setting.GetExposedData(),
	})
}
