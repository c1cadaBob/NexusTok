// Package controller - channel_affinity_cache.go
// 该文件实现了渠道亲和性缓存管理的 API 控制器
//
// 渠道亲和性缓存用于将特定用户/密钥绑定到特定渠道，提高请求路由的一致性
// 功能包括：
// - 查询缓存统计信息
// - 清空指定规则或全部缓存
// - 查询特定规则的使用统计
//
// 主要 API：
// - GetChannelAffinityCacheStats：获取缓存统计信息
// - ClearChannelAffinityCache：清空缓存
// - GetChannelAffinityUsageCacheStats：获取使用统计
package controller

import (
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/service"
	"github.com/gin-gonic/gin"
)

// GetChannelAffinityCacheStats 获取渠道亲和性缓存统计信息
func GetChannelAffinityCacheStats(c *gin.Context) {
	stats := service.GetChannelAffinityCacheStats()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}

// ClearChannelAffinityCache 清空渠道亲和性缓存
//
// 支持两种模式：
// - all=true：清空所有缓存
// - rule_name：清空指定规则的缓存
func ClearChannelAffinityCache(c *gin.Context) {
	all := strings.TrimSpace(c.Query("all"))
	ruleName := strings.TrimSpace(c.Query("rule_name"))

	if all == "true" {
		deleted := service.ClearChannelAffinityCacheAll()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"deleted": deleted,
			},
		})
		return
	}

	if ruleName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "缺少参数：rule_name，或使用 all=true 清空全部",
		})
		return
	}

	deleted, err := service.ClearChannelAffinityCacheByRuleName(ruleName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"deleted": deleted,
		},
	})
}

// GetChannelAffinityUsageCacheStats 获取渠道亲和性使用缓存统计
//
// 查询参数：
//   - rule_name: 规则名称（必填）
//   - using_group: 使用分组
//   - key_fp: 密钥指纹（必填）
func GetChannelAffinityUsageCacheStats(c *gin.Context) {
	ruleName := strings.TrimSpace(c.Query("rule_name"))
	usingGroup := strings.TrimSpace(c.Query("using_group"))
	keyFp := strings.TrimSpace(c.Query("key_fp"))

	if ruleName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "missing param: rule_name",
		})
		return
	}
	if keyFp == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "missing param: key_fp",
		})
		return
	}

	stats := service.GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFp)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}
