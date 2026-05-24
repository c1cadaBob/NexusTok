// Package controller - payment_compliance.go
// 该文件实现了支付合规确认的 API 控制器
//
// 支付功能需要管理员先确认合规声明才能启用
// 确认信息包括：确认时间、确认用户、确认 IP、条款版本
//
// 主要 API：
// - ConfirmPaymentCompliance：确认支付合规声明
package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/i18n"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// PaymentComplianceRequest 支付合规确认请求结构体
type PaymentComplianceRequest struct {
	Confirmed bool `json:"confirmed"` // 是否确认合规声明
}

// requirePaymentCompliance 检查支付合规是否已确认
//
// 如果未确认，返回错误响应
//
// 返回值：
//   - bool: 是否已确认
func requirePaymentCompliance(c *gin.Context) bool {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorI18n(c, i18n.MsgPaymentComplianceRequired)
		return false
	}
	return true
}

// ConfirmPaymentCompliance 确认支付合规声明
//
// 记录确认信息到数据库，包括：
// - 确认状态
// - 条款版本
// - 确认时间
// - 确认用户 ID
// - 确认 IP 地址
//
// 注意：API access token 不允许执行此操作，需要 dashboard 会话认证
func ConfirmPaymentCompliance(c *gin.Context) {
	if c.GetBool("use_access_token") {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "This operation requires dashboard session authentication. API access token is not allowed.",
		})
		return
	}

	var req PaymentComplianceRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if !req.Confirmed {
		common.ApiErrorMsg(c, "请确认合规声明")
		return
	}

	now := time.Now().Unix()
	userId := c.GetInt("id")
	clientIP := c.ClientIP()

	updates := map[string]string{
		"payment_setting.compliance_confirmed":     "true",
		"payment_setting.compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
		"payment_setting.compliance_confirmed_at":  strconv.FormatInt(now, 10),
		"payment_setting.compliance_confirmed_by":  strconv.Itoa(userId),
		"payment_setting.compliance_confirmed_ip":  clientIP,
	}

	for key, value := range updates {
		if err := model.UpdateOption(key, value); err != nil {
			common.ApiError(c, err)
			return
		}
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf(
		"payment compliance confirmed user_id=%d ip=%s terms_version=%s confirmed_at=%d",
		userId,
		clientIP,
		operation_setting.CurrentComplianceTermsVersion,
		now,
	))

	common.ApiSuccess(c, gin.H{
		"confirmed":     true,
		"terms_version": operation_setting.CurrentComplianceTermsVersion,
		"confirmed_at":  now,
		"confirmed_by":  userId,
	})
}
