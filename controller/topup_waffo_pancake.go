// Package controller - topup_waffo_pancake.go
// 该文件实现了 Waffo Pancake 支付平台的充值 API 控制器
//
// Waffo Pancake 是 Waffo 的一个子产品，提供简化的结账体验
// 功能包括：
// - 创建 Waffo Pancake 结账会话
// - 处理 Waffo Pancake Webhook 支付回调
// - 支持多种货币和分组倍率
//
// 主要 API：
// - RequestWaffoPancakeAmount：查询 Waffo Pancake 充值金额
// - RequestWaffoPancakePay：发起 Waffo Pancake 充值支付
// - WaffoPancakeWebhook：处理 Waffo Pancake 支付回调
package controller

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/setting"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
)

// WaffoPancakePayRequest Waffo Pancake 充值支付请求结构体
type WaffoPancakePayRequest struct {
	Amount int64 `json:"amount"` // 充值数量
}

// saveWaffoPancakeConfigRequest 是管理端 Waffo Pancake 配置原子保存请求。
//
// 密钥类字段在前端不会回显，传空表示保留服务端已保存的值；布尔和数字字段由
// 管理端表单始终显式传入，用于保留关闭网关、沙箱模式和最低充值等零值/默认值语义。
type saveWaffoPancakeConfigRequest struct {
	Enabled          bool    `json:"enabled"`
	Sandbox          bool    `json:"sandbox"`
	MerchantID       string  `json:"merchant_id"`
	PrivateKey       string  `json:"private_key"`
	WebhookPublicKey string  `json:"webhook_public_key"`
	WebhookTestKey   string  `json:"webhook_test_key"`
	StoreID          string  `json:"store_id"`
	ProductID        string  `json:"product_id"`
	ReturnURL        string  `json:"return_url"`
	Currency         string  `json:"currency"`
	UnitPrice        float64 `json:"unit_price"`
	MinTopUp         int     `json:"min_top_up"`
}

// RequestWaffoPancakeAmount 查询 Waffo Pancake 充值金额
//
// 根据充值数量和用户分组计算实际支付金额
func RequestWaffoPancakeAmount(c *gin.Context) {
	var req WaffoPancakePayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < int64(setting.WaffoPancakeMinTopUp) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", setting.WaffoPancakeMinTopUp)})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getWaffoPancakePayMoney(req.Amount, group)
	if payMoney <= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "data": fmt.Sprintf("%.2f", payMoney)})
}

// getWaffoPancakePayMoney 计算 Waffo Pancake 充值支付金额
//
// 使用 decimal 库进行精确计算，避免浮点数精度问题
// 计算公式：amount * unitPrice * topupGroupRatio * discount
//
// 参数：
//   - amount: 充值数量
//   - group: 用户分组
//
// 返回：
//   - float64: 实际支付金额
func getWaffoPancakePayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount = dAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok && ds > 0 {
		discount = ds
	}

	payMoney := dAmount.
		Mul(decimal.NewFromFloat(setting.WaffoPancakeUnitPrice)).
		Mul(decimal.NewFromFloat(topupGroupRatio)).
		Mul(decimal.NewFromFloat(discount))

	return payMoney.InexactFloat64()
}

// normalizeWaffoPancakeTopUpAmount 标准化 Waffo Pancake 充值数量
//
// 如果配额显示类型为 Token，将数量转换为配额单位
// 最小值为 1
func normalizeWaffoPancakeTopUpAmount(amount int64) int64 {
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
		return amount
	}

	normalized := decimal.NewFromInt(amount).
		Div(decimal.NewFromFloat(common.QuotaPerUnit)).
		IntPart()
	if normalized < 1 {
		return 1
	}
	return normalized
}

// formatWaffoPancakeAmount 格式化 Waffo Pancake 支付金额
//
// 使用 decimal 库确保两位小数精度
func formatWaffoPancakeAmount(payMoney float64) string {
	return decimal.NewFromFloat(payMoney).StringFixed(2)
}

// getWaffoPancakeBuyerEmail 获取 Waffo Pancake 买家邮箱
//
// 优先使用用户邮箱，如果没有则生成占位邮箱
func getWaffoPancakeBuyerEmail(user *model.User) string {
	if user != nil && strings.TrimSpace(user.Email) != "" {
		return user.Email
	}
	if user != nil {
		return fmt.Sprintf("%d@nexustok.local", user.Id)
	}
	return ""
}

// getWaffoPancakeReturnURL 获取 Waffo Pancake 支付返回 URL
//
// 优先使用配置的返回 URL，否则使用默认的充值历史页面
func getWaffoPancakeReturnURL() string {
	if strings.TrimSpace(setting.WaffoPancakeReturnURL) != "" {
		return setting.WaffoPancakeReturnURL
	}
	return paymentReturnPath("/console/topup?show_history=true")
}

// SaveWaffoPancakeConfig 原子保存管理端 Waffo Pancake 支付配置。
//
// 该接口仅负责保存 NexusTok 本地 option，不创建外部 Pancake Store/Product；
// 后续 catalog 和一键创建能力会在单独评审中接入，避免外部资源副作用影响当前支付链路。
func SaveWaffoPancakeConfig(c *gin.Context) {
	var req saveWaffoPancakeConfigRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	if err := service.SaveWaffoPancakeConfig(
		req.Enabled,
		req.Sandbox,
		req.MerchantID,
		req.PrivateKey,
		req.WebhookPublicKey,
		req.WebhookTestKey,
		req.StoreID,
		req.ProductID,
		req.ReturnURL,
		req.Currency,
		req.UnitPrice,
		req.MinTopUp,
	); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf(
			"Waffo Pancake 保存配置失败 enabled=%t sandbox=%t store_id=%q product_id=%q error=%q",
			req.Enabled, req.Sandbox, req.StoreID, req.ProductID, err.Error(),
		))
		common.ApiErrorMsg(c, "保存配置失败")
		return
	}

	common.ApiSuccess(c, gin.H{
		"enabled":    setting.WaffoPancakeEnabled,
		"sandbox":    setting.WaffoPancakeSandbox,
		"store_id":   setting.WaffoPancakeStoreID,
		"product_id": setting.WaffoPancakeProductID,
	})
}

// RequestWaffoPancakePay 发起 Waffo Pancake 充值支付
//
// 流程：
// 1. 验证 Waffo Pancake 支付是否启用
// 2. 验证配置完整性（商户 ID、私钥、公钥等）
// 3. 验证充值数量
// 4. 计算实际支付金额
// 5. 生成唯一订单号
// 6. 创建本地待处理订单
// 7. 调用 service 层创建结账会话
// 8. 返回结账 URL 和会话信息
func RequestWaffoPancakePay(c *gin.Context) {
	if !setting.WaffoPancakeEnabled {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 支付未启用"})
		return
	}
	currentWebhookKey := setting.WaffoPancakeWebhookPublicKey
	if setting.WaffoPancakeSandbox {
		currentWebhookKey = setting.WaffoPancakeWebhookTestKey
	}
	if strings.TrimSpace(setting.WaffoPancakeMerchantID) == "" ||
		strings.TrimSpace(setting.WaffoPancakePrivateKey) == "" ||
		strings.TrimSpace(currentWebhookKey) == "" ||
		strings.TrimSpace(setting.WaffoPancakeStoreID) == "" ||
		strings.TrimSpace(setting.WaffoPancakeProductID) == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo Pancake 配置不完整"})
		return
	}

	var req WaffoPancakePayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < int64(setting.WaffoPancakeMinTopUp) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", setting.WaffoPancakeMinTopUp)})
		return
	}

	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "用户不存在"})
		return
	}

	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getWaffoPancakePayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	tradeNo := fmt.Sprintf("WAFFO_PANCAKE-%d-%d-%s", id, time.Now().UnixMilli(), randstr.String(6))
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          normalizeWaffoPancakeTopUpAmount(req.Amount),
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodWaffoPancake,
		PaymentProvider: model.PaymentProviderWaffoPancake,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 创建充值订单失败 user_id=%d trade_no=%s amount=%d error=%q", id, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	expiresInSeconds := 45 * 60
	session, err := service.CreateWaffoPancakeCheckoutSession(c.Request.Context(), &service.WaffoPancakeCreateSessionParams{
		StoreID:     setting.WaffoPancakeStoreID,
		ProductID:   setting.WaffoPancakeProductID,
		ProductType: "onetime",
		Currency:    strings.ToUpper(strings.TrimSpace(setting.WaffoPancakeCurrency)),
		PriceSnapshot: &service.WaffoPancakePriceSnapshot{
			Amount:      formatWaffoPancakeAmount(payMoney),
			TaxIncluded: false,
			TaxCategory: "saas",
		},
		BuyerEmail:       getWaffoPancakeBuyerEmail(user),
		SuccessURL:       getWaffoPancakeReturnURL(),
		ExpiresInSeconds: &expiresInSeconds,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 创建结账会话失败 user_id=%d trade_no=%s error=%q", id, tradeNo, err.Error()))
		topUp.Status = common.TopUpStatusFailed
		_ = topUp.Update()
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo Pancake 充值订单创建成功 user_id=%d trade_no=%s session_id=%s amount=%d money=%.2f", id, tradeNo, session.SessionID, req.Amount, payMoney))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url": session.CheckoutURL,
			"session_id":   session.SessionID,
			"expires_at":   session.ExpiresAt,
			"order_id":     tradeNo,
		},
	})
}

// WaffoPancakeWebhook 处理 Waffo Pancake 支付回调
//
// 流程：
// 1. 检查 Webhook 是否启用
// 2. 读取请求体
// 3. 验证签名（X-Waffo-Signature）
// 4. 解析 Webhook 事件
// 5. 处理 order.completed 事件
// 6. 解析订单号映射
// 7. 加锁处理订单，完成充值
func WaffoPancakeWebhook(c *gin.Context) {
	if !isWaffoPancakeWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo Pancake webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.String(http.StatusForbidden, "webhook disabled")
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake webhook 读取请求体失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.String(http.StatusBadRequest, "bad request")
		return
	}

	signature := c.GetHeader("X-Waffo-Signature")
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo Pancake webhook 收到请求 path=%q client_ip=%s signature=%q body=%q", c.Request.RequestURI, c.ClientIP(), signature, string(bodyBytes)))

	event, err := service.VerifyConfiguredWaffoPancakeWebhook(string(bodyBytes), signature)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo Pancake webhook 验签失败 path=%q client_ip=%s signature=%q body=%q error=%q", c.Request.RequestURI, c.ClientIP(), signature, string(bodyBytes), err.Error()))
		c.String(http.StatusUnauthorized, "invalid signature")
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo Pancake webhook 验签成功 event_type=%s event_id=%s order_id=%s client_ip=%s", event.NormalizedEventType(), event.ID, event.Data.OrderID, c.ClientIP()))
	if event.NormalizedEventType() != "order.completed" {
		c.String(http.StatusOK, "OK")
		return
	}

	tradeNo, err := service.ResolveWaffoPancakeTradeNo(event)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo Pancake webhook 订单号映射失败 event_id=%s order_id=%s error=%q", event.ID, event.Data.OrderID, err.Error()))
		c.String(http.StatusOK, "OK")
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	if err := model.RechargeWaffoPancake(tradeNo); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 充值处理失败 trade_no=%s event_id=%s order_id=%s client_ip=%s error=%q", tradeNo, event.ID, event.Data.OrderID, c.ClientIP(), err.Error()))
		c.String(http.StatusInternalServerError, "retry")
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo Pancake 充值成功 trade_no=%s event_id=%s order_id=%s client_ip=%s", tradeNo, event.ID, event.Data.OrderID, c.ClientIP()))
	c.String(http.StatusOK, "OK")
}
