package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedSubscriptionGroupUser(t *testing.T, id int, group string) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:       id,
		Username: "subscription_group_user",
		Status:   common.UserStatusEnabled,
		Group:    group,
	}).Error)
}

func seedSubscriptionGroupPlan(t *testing.T, id int, upgradeGroup string, downgradeGroup string) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:                  id,
		Title:               "分组订阅套餐",
		PriceAmount:         1,
		Currency:            "USD",
		DurationUnit:        SubscriptionDurationMonth,
		DurationValue:       1,
		Enabled:             true,
		TotalAmount:         1000,
		UpgradeGroup:        upgradeGroup,
		DowngradeGroup:      downgradeGroup,
		AllowWalletOverflow: common.GetPointer(true),
	}
	require.NoError(t, DB.Create(plan).Error)
	InvalidateSubscriptionPlanCache(id)
	return plan
}

func seedSubscriptionGroupSub(t *testing.T, sub *UserSubscription) {
	t.Helper()
	require.NoError(t, DB.Create(sub).Error)
}

func requireSubscriptionGroup(t *testing.T, userId int) string {
	t.Helper()
	group, err := getUserGroupByIdTx(DB, userId)
	require.NoError(t, err)
	return group
}

func TestCreateUserSubscriptionSnapshotsDowngradeGroup(t *testing.T) {
	truncateTables(t)

	seedSubscriptionGroupUser(t, 9601, "default")
	plan := seedSubscriptionGroupPlan(t, 9701, "vip", "trial_end")

	sub, err := CreateUserSubscriptionFromPlanTx(DB, 9601, plan, "admin")
	require.NoError(t, err)

	assert.Equal(t, "vip", sub.UpgradeGroup)
	assert.Equal(t, "default", sub.PrevUserGroup)
	assert.Equal(t, "trial_end", sub.DowngradeGroup)
	assert.Equal(t, "vip", requireSubscriptionGroup(t, 9601))
}

func TestExpireDueSubscriptionsUsesExplicitDowngradeGroup(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	seedSubscriptionGroupUser(t, 9602, "vip")
	seedSubscriptionGroupSub(t, &UserSubscription{
		Id:             9702,
		UserId:         9602,
		PlanId:         9802,
		AmountTotal:    1000,
		Status:         "active",
		StartTime:      now - 7200,
		EndTime:        now - 60,
		UpgradeGroup:   "vip",
		PrevUserGroup:  "default",
		DowngradeGroup: "trial_end",
	})

	expired, err := ExpireDueSubscriptions(100)
	require.NoError(t, err)

	assert.Equal(t, 1, expired)
	assert.Equal(t, "trial_end", requireSubscriptionGroup(t, 9602))
	var sub UserSubscription
	require.NoError(t, DB.Where("id = ?", 9702).First(&sub).Error)
	assert.Equal(t, "expired", sub.Status)
}

func TestExpireDueSubscriptionsKeepsLegacyPrevGroupFallback(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	seedSubscriptionGroupUser(t, 9603, "vip")
	seedSubscriptionGroupSub(t, &UserSubscription{
		Id:            9703,
		UserId:        9603,
		PlanId:        9803,
		AmountTotal:   1000,
		Status:        "active",
		StartTime:     now - 7200,
		EndTime:       now - 60,
		UpgradeGroup:  "vip",
		PrevUserGroup: "default",
	})

	expired, err := ExpireDueSubscriptions(100)
	require.NoError(t, err)

	assert.Equal(t, 1, expired)
	assert.Equal(t, "default", requireSubscriptionGroup(t, 9603))
}

func TestExpireDueSubscriptionsKeepsGroupWhenOtherUpgradeIsActive(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	seedSubscriptionGroupUser(t, 9604, "vip")
	seedSubscriptionGroupSub(t, &UserSubscription{
		Id:             9704,
		UserId:         9604,
		PlanId:         9804,
		AmountTotal:    1000,
		Status:         "active",
		StartTime:      now - 7200,
		EndTime:        now - 60,
		UpgradeGroup:   "vip",
		PrevUserGroup:  "default",
		DowngradeGroup: "trial_end",
	})
	seedSubscriptionGroupSub(t, &UserSubscription{
		Id:            9705,
		UserId:        9604,
		PlanId:        9805,
		AmountTotal:   1000,
		Status:        "active",
		StartTime:     now - 3600,
		EndTime:       now + 3600,
		UpgradeGroup:  "vip",
		PrevUserGroup: "default",
	})

	expired, err := ExpireDueSubscriptions(100)
	require.NoError(t, err)

	assert.Equal(t, 1, expired)
	assert.Equal(t, "vip", requireSubscriptionGroup(t, 9604))
}

func TestDowngradeUserGroupForSubscriptionTxSupportsExplicitDowngradeOnly(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	seedSubscriptionGroupUser(t, 9605, "trial")
	sub := &UserSubscription{
		Id:             9706,
		UserId:         9605,
		PlanId:         9806,
		AmountTotal:    1000,
		Status:         "active",
		StartTime:      now - 3600,
		EndTime:        now + 3600,
		DowngradeGroup: "default",
	}
	seedSubscriptionGroupSub(t, sub)

	target, err := downgradeUserGroupForSubscriptionTx(DB, sub, now)
	require.NoError(t, err)

	assert.Equal(t, "default", target)
	assert.Equal(t, "default", requireSubscriptionGroup(t, 9605))
}
