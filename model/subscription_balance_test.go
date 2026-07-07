package model

import (
	"strings"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertBalancePurchasePlan(t *testing.T, id int, price float64, allowBalancePay *bool, allowWalletOverflow *bool) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:                  id,
		Title:               "余额订阅套餐",
		PriceAmount:         price,
		Currency:            "USD",
		DurationUnit:        SubscriptionDurationMonth,
		DurationValue:       1,
		Enabled:             true,
		TotalAmount:         1000,
		AllowBalancePay:     allowBalancePay,
		AllowWalletOverflow: allowWalletOverflow,
	}
	require.NoError(t, DB.Create(plan).Error)
	InvalidateSubscriptionPlanCache(id)
	return plan
}

func TestPurchaseSubscriptionWithBalanceCreatesSnapshotAndOrder(t *testing.T) {
	truncateTables(t)

	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 100
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
	})

	allowOverflow := false
	insertUserForPaymentGuardTest(t, 9101, 500)
	plan := insertBalancePurchasePlan(t, 9201, 1.25, nil, &allowOverflow)

	err := PurchaseSubscriptionWithBalance(9101, plan.Id)
	require.NoError(t, err)

	assert.Equal(t, 375, getUserQuotaForPaymentGuardTest(t, 9101))

	var sub UserSubscription
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", 9101, plan.Id).First(&sub).Error)
	assert.Equal(t, PaymentMethodBalance, sub.Source)
	require.NotNil(t, sub.AllowWalletOverflow)
	assert.False(t, *sub.AllowWalletOverflow)

	var order SubscriptionOrder
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", 9101, plan.Id).First(&order).Error)
	assert.Equal(t, PaymentMethodBalance, order.PaymentMethod)
	assert.Equal(t, PaymentProviderBalance, order.PaymentProvider)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Contains(t, order.ProviderPayload, "charged_quota=125")
	assert.NotZero(t, order.CompleteTime)

	var topupCount int64
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", order.TradeNo).Count(&topupCount).Error)
	assert.Zero(t, topupCount)

	var log Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", 9101, LogTypeTopup).Order("id desc").First(&log).Error)
	assert.True(t, strings.Contains(log.Content, "使用余额购买订阅成功"))
}

func TestPurchaseSubscriptionWithBalanceRejectsDisabledBalancePay(t *testing.T) {
	truncateTables(t)

	allowBalancePay := false
	insertUserForPaymentGuardTest(t, 9102, 500)
	plan := insertBalancePurchasePlan(t, 9202, 1.00, &allowBalancePay, nil)

	err := PurchaseSubscriptionWithBalance(9102, plan.Id)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不允许使用余额兑换")
	assert.Equal(t, 500, getUserQuotaForPaymentGuardTest(t, 9102))
	assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, 9102))
}

func TestUserActiveSubscriptionsAllowWalletOverflowUsesStrictestActiveSnapshot(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 9103, 0)
	now := time.Now().Unix()
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:              9103,
		PlanId:              9301,
		AmountTotal:         100,
		AmountUsed:          100,
		Status:              "active",
		StartTime:           now - 10,
		EndTime:             now + 3600,
		AllowWalletOverflow: common.GetPointer(false),
	}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:              9103,
		PlanId:              9302,
		AmountTotal:         100,
		AmountUsed:          0,
		Status:              "active",
		StartTime:           now - 10,
		EndTime:             now + 3600,
		AllowWalletOverflow: common.GetPointer(true),
	}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:              9103,
		PlanId:              9303,
		AmountTotal:         100,
		AmountUsed:          100,
		Status:              "expired",
		StartTime:           now - 7200,
		EndTime:             now - 3600,
		AllowWalletOverflow: common.GetPointer(false),
	}).Error)

	allow, err := UserActiveSubscriptionsAllowWalletOverflow(9103)
	require.NoError(t, err)
	assert.False(t, allow)

	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", 9103).
		Update("allow_wallet_overflow", true).Error)
	allow, err = UserActiveSubscriptionsAllowWalletOverflow(9103)
	require.NoError(t, err)
	assert.True(t, allow)
}
