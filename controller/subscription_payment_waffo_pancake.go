// Package controller - subscription_payment_waffo_pancake.go
// 该文件实现了 Waffo Pancake 的订阅购买 API。
//
// 当前只使用 Pancake OnetimeProduct 购买 NexusTok 内部订阅权益，不接入 Pancake
// 自动续费 SubscriptionProduct。原因是 NexusTok 订阅模型尚未表达外部续费、取消、
// 逾期和退款撤销权益等生命周期，贸然接入自动续费会导致上游扣款与本地权益不一致。
package controller

import (
	"fmt"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
)

// SubscriptionWaffoPancakePayRequest 表示用户使用 Waffo Pancake 购买订阅套餐的请求。
type SubscriptionWaffoPancakePayRequest struct {
	PlanId int `json:"plan_id"` // 订阅套餐 ID
}

// SubscriptionRequestWaffoPancakePay 发起 Waffo Pancake 订阅支付。
//
// 流程：
//  1. 检查支付合规、网关开关、凭证和 webhook 配置；
//  2. 校验套餐启用、金额、Waffo Pancake Product ID 和购买次数；
//  3. 创建 pending SubscriptionOrder，订单号使用 WAFFO_PANCAKE_SUB- 前缀；
//  4. 用 Authenticated checkout 传入本地订单号和稳定 buyer identity；
//  5. webhook 根据同一订单号前缀完成订阅权益发放。
func SubscriptionRequestWaffoPancakePay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionWaffoPancakePayRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	if !isWaffoPancakeRuntimeConfigured() {
		common.ApiErrorMsg(c, "Waffo Pancake 未配置或未启用")
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	if plan.PriceAmount < 0.01 {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}
	productID := strings.TrimSpace(plan.WaffoPancakeProductId)
	if productID == "" {
		common.ApiErrorMsg(c, "该套餐未配置 Waffo Pancake Product ID")
		return
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}

	if plan.MaxPurchasePerUser > 0 {
		count, err := model.CountUserSubscriptionsByPlan(userId, plan.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			common.ApiErrorMsg(c, "已达到该套餐购买上限")
			return
		}
	}

	tradeNo := fmt.Sprintf("WAFFO_PANCAKE_SUB-%d-%d-%s", userId, time.Now().UnixMilli(), randstr.String(6))
	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           plan.PriceAmount,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodWaffoPancake,
		PaymentProvider: model.PaymentProviderWaffoPancake,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅订单创建失败 user_id=%d plan_id=%d trade_no=%s error=%q", userId, plan.Id, tradeNo, err.Error()))
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}

	expiresInSeconds := 45 * 60
	session, err := service.CreateWaffoPancakeCheckoutSession(c.Request.Context(), &service.WaffoPancakeCreateSessionParams{
		ProductID: productID,
		Currency:  "USD",
		PriceSnapshot: &service.WaffoPancakePriceSnapshot{
			Amount:      decimal.NewFromFloat(plan.PriceAmount).StringFixed(2),
			TaxIncluded: false,
			TaxCategory: "saas",
		},
		BuyerEmail:              getWaffoPancakeBuyerEmail(user),
		BuyerIdentity:           service.WaffoPancakeBuyerIdentityFromUserID(user.Id),
		SuccessURL:              getWaffoPancakeReturnURL(),
		ExpiresInSeconds:        &expiresInSeconds,
		OrderMerchantExternalID: tradeNo,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅结账会话创建失败 user_id=%d plan_id=%d trade_no=%s product_id=%s error=%q", userId, plan.Id, tradeNo, productID, err.Error()))
		_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderWaffoPancake)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo Pancake 订阅订单创建成功 user_id=%d plan_id=%d trade_no=%s session_id=%s money=%.2f", userId, plan.Id, tradeNo, session.SessionID, plan.PriceAmount))
	common.ApiSuccess(c, gin.H{
		"checkout_url":     session.CheckoutURL,
		"session_id":       session.SessionID,
		"expires_at":       session.ExpiresAt,
		"order_id":         tradeNo,
		"token":            session.Token,
		"token_expires_at": session.TokenExpiresAt,
		"sandbox":          setting.WaffoPancakeSandbox,
	})
}
