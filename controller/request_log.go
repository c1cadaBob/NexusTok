package controller

import (
	"strconv"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
)

// GetAllRequestLogs 分页获取请求记录列表
func GetAllRequestLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	ruleId, _ := strconv.Atoi(c.Query("rule_id"))
	userId, _ := strconv.Atoi(c.Query("user_id"))
	modelName := c.Query("model_name")
	relayFormat := c.Query("relay_format")
	startTime, _ := strconv.ParseInt(c.Query("start_time"), 10, 64)
	endTime, _ := strconv.ParseInt(c.Query("end_time"), 10, 64)

	logs, total, err := model.GetAllRequestLogs(
		pageInfo.GetStartIdx(), pageInfo.GetPageSize(),
		ruleId, userId, modelName, relayFormat, startTime, endTime,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
}

// GetRequestLogDetail 获取单条请求记录详情（包含完整 body）
func GetRequestLogDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorMsg(c, "无效的记录 ID")
		return
	}
	log, err := model.GetRequestLogDetail(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, log)
}

// DeleteRequestLogs 批量清理请求记录
func DeleteRequestLogs(c *gin.Context) {
	// 按时间范围删除，如果 before_time=0 则删除全部
	beforeTime, _ := strconv.ParseInt(c.Query("before_time"), 10, 64)

	var affected int64
	var err error
	if beforeTime > 0 {
		affected, err = model.DeleteRequestLogsBefore(beforeTime)
	} else {
		affected, err = model.DeleteAllRequestLogs()
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"deleted": affected})
}
