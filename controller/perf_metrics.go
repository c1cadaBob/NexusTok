// Package controller - perf_metrics.go
// 该文件实现了性能指标查询的 API 控制器
//
// 性能指标用于监控 API 调用的响应时间和成功率
//
// 主要 API：
// - GetPerfMetricsSummary：获取所有模型的性能指标汇总
// - GetPerfMetrics：获取指定模型的性能指标详情
package controller

import (
	"net/http"
	"strconv"

	perfmetrics "github.com/c1cada/NexusTok/pkg/perf_metrics"
	"github.com/c1cada/NexusTok/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// GetPerfMetricsSummary 获取所有模型的性能指标汇总
//
// 查询参数：
//   - hours: 查询时间范围（小时），默认 24
func GetPerfMetricsSummary(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	result, err := perfmetrics.QuerySummaryAll(hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
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

// GetPerfMetrics 获取指定模型的性能指标详情
//
// 查询参数：
//   - model: 模型名称（必填）
//   - group: 用户分组过滤
//   - hours: 查询时间范围（小时），默认 24
func GetPerfMetrics(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		Model: modelName,
		Group: c.Query("group"),
		Hours: hours,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	result.Groups = filterActiveGroups(result.Groups)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// filterActiveGroups 过滤出活跃的用户分组
//
// 只返回系统中已配置的分组和 auto 分组
func filterActiveGroups(groups []perfmetrics.GroupResult) []perfmetrics.GroupResult {
	activeGroups := ratio_setting.GetGroupRatioCopy()
	filtered := make([]perfmetrics.GroupResult, 0, len(groups))
	for _, g := range groups {
		if _, ok := activeGroups[g.Group]; ok || g.Group == "auto" {
			filtered = append(filtered, g)
		}
	}
	return filtered
}
