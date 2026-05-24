// Package controller - usedata.go
// 该文件实现了用量数据查询的 API 控制器
//
// 功能包括：
// - 管理员查询所有用户的配额使用日期分布
// - 管理员按用户分组查询配额使用数据
// - 用户查询自己的配额使用数据（时间跨度限制 1 个月）
//
// 主要 API：
// - GetAllQuotaDates：管理员查询所有配额使用日期
// - GetQuotaDatesByUser：管理员按用户分组查询配额使用数据
// - GetUserQuotaDates：用户查询自己的配额使用数据
package controller

import (
	"net/http"
	"strconv"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"

	"github.com/gin-gonic/gin"
)

// GetAllQuotaDates 管理员查询所有配额使用日期
//
// 支持的查询参数：
//   - start_timestamp: 开始时间戳
//   - end_timestamp: 结束时间戳
//   - username: 用户名筛选
func GetAllQuotaDates(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	dates, err := model.GetAllQuotaDates(startTimestamp, endTimestamp, username)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

// GetQuotaDatesByUser 管理员按用户分组查询配额使用数据
//
// 返回每个用户的配额使用汇总，支持时间范围筛选
func GetQuotaDatesByUser(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	dates, err := model.GetQuotaDataGroupByUser(startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
}

// GetUserQuotaDates 用户查询自己的配额使用数据
//
// 限制：
// - 时间跨度不能超过 1 个月（2592000 秒）
// - 只能查询自己的数据
func GetUserQuotaDates(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 判断时间跨度是否超过 1 个月
	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	dates, err := model.GetQuotaDataByUserId(userId, startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}
