package controller

import (
	"strconv"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
)

// GetAllRequestRules 分页获取请求规则列表
func GetAllRequestRules(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	// 状态过滤：-1=全部，0=禁用，1=启用
	status := -1
	if s, err := strconv.Atoi(c.Query("status")); err == nil {
		status = s
	}
	relayFormat := c.Query("relay_format")

	rules, total, err := model.GetAllRequestRules(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), status, relayFormat)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rules)
	common.ApiSuccess(c, pageInfo)
}

// SearchRequestRules 搜索请求规则
func SearchRequestRules(c *gin.Context) {
	keyword := c.Query("keyword")
	pageInfo := common.GetPageQuery(c)

	rules, total, err := model.SearchRequestRules(keyword, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rules)
	common.ApiSuccess(c, pageInfo)
}

// GetRequestRule 获取单条请求规则
func GetRequestRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "无效的规则 ID")
		return
	}
	rule, err := model.GetRequestRuleByID(id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rule)
}

// CreateRequestRule 创建请求规则
func CreateRequestRule(c *gin.Context) {
	var rule model.RequestRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		common.ApiError(c, err)
		return
	}
	if rule.Name == "" {
		common.ApiErrorMsg(c, "规则名称不能为空")
		return
	}
	if err := rule.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, &rule)
}

// UpdateRequestRule 更新请求规则
func UpdateRequestRule(c *gin.Context) {
	var rule model.RequestRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		common.ApiError(c, err)
		return
	}
	if rule.Id == 0 {
		common.ApiErrorMsg(c, "规则 ID 不能为空")
		return
	}
	if rule.Name == "" {
		common.ApiErrorMsg(c, "规则名称不能为空")
		return
	}
	if err := rule.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, &rule)
}

// DeleteRequestRule 删除请求规则
func DeleteRequestRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "无效的规则 ID")
		return
	}
	rule := &model.RequestRule{Id: id}
	if err := rule.Delete(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// UpdateRequestRuleStatus 快速切换规则启用/禁用状态
func UpdateRequestRuleStatus(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorMsg(c, "无效的规则 ID")
		return
	}
	var req struct {
		Status int `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	rule := &model.RequestRule{Id: id}
	if err := rule.UpdateStatus(req.Status); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
