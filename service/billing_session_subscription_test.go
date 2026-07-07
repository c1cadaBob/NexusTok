package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/model"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedBillingSessionUser(t *testing.T, userID int, quota int) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.User{
		Id:       userID,
		Username: "billing_session_user",
		Quota:    quota,
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id:          userID,
		UserId:      userID,
		Key:         "sk-billing-session",
		Name:        "billing-session-token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: quota,
	}).Error)
}

func seedBillingSessionPlanAndSubscription(t *testing.T, userID int, allowWalletOverflow bool) {
	t.Helper()
	plan := &model.SubscriptionPlan{
		Id:                  userID,
		Title:               "严格订阅套餐",
		PriceAmount:         1,
		Currency:            "USD",
		DurationUnit:        model.SubscriptionDurationMonth,
		DurationValue:       1,
		Enabled:             true,
		TotalAmount:         100,
		AllowWalletOverflow: common.GetPointer(allowWalletOverflow),
	}
	require.NoError(t, model.DB.Create(plan).Error)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		Id:                  userID,
		UserId:              userID,
		PlanId:              plan.Id,
		AmountTotal:         100,
		AmountUsed:          100,
		Status:              "active",
		StartTime:           now - 10,
		EndTime:             now + 3600,
		AllowWalletOverflow: common.GetPointer(allowWalletOverflow),
	}).Error)
}

func newBillingSessionTestContext(tokenQuota int) *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	ctx.Set("token_quota", tokenQuota)
	return ctx
}

func newBillingSessionRelayInfo(userID int, pref string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		UserId:          userID,
		TokenId:         userID,
		TokenKey:        "sk-billing-session",
		OriginModelName: "gpt-test",
		RequestId:       "billing-session-request",
		UserSetting: dto.UserSetting{
			BillingPreference: pref,
		},
	}
}

func TestNewBillingSessionHonorsWalletOverflowBlock(t *testing.T) {
	truncate(t)
	const userID = 6101
	seedBillingSessionUser(t, userID, 1000)
	seedBillingSessionPlanAndSubscription(t, userID, false)

	session, apiErr := NewBillingSession(
		newBillingSessionTestContext(1000),
		newBillingSessionRelayInfo(userID, "subscription_first"),
		10,
	)

	require.Nil(t, session)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeInsufficientUserQuota, apiErr.GetErrorCode())
	assert.Equal(t, 1000, getUserQuota(t, userID))
	assert.Equal(t, 1000, getTokenRemainQuota(t, userID))
	assert.Equal(t, int64(100), getSubscriptionUsed(t, userID))
}

func TestNewBillingSessionFallsBackToWalletWhenOverflowAllowed(t *testing.T) {
	truncate(t)
	const userID = 6102
	seedBillingSessionUser(t, userID, 1000)
	seedBillingSessionPlanAndSubscription(t, userID, true)

	session, apiErr := NewBillingSession(
		newBillingSessionTestContext(1000),
		newBillingSessionRelayInfo(userID, "subscription_first"),
		10,
	)

	require.Nil(t, apiErr)
	require.NotNil(t, session)
	assert.Equal(t, BillingSourceWallet, session.funding.Source())
	assert.Equal(t, 990, getUserQuota(t, userID))
	assert.Equal(t, 990, getTokenRemainQuota(t, userID))
	assert.Equal(t, int64(100), getSubscriptionUsed(t, userID))
}
