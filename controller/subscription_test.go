package controller

import (
	"testing"

	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withSubscriptionGroupRatioForTest(t *testing.T) {
	t.Helper()
	original := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"trial_end":1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(original))
	})
}

func TestSubscriptionPlanGroupValidationTrimsAndAcceptsKnownGroups(t *testing.T) {
	withSubscriptionGroupRatioForTest(t)

	plan := &model.SubscriptionPlan{
		UpgradeGroup:   " vip ",
		DowngradeGroup: " trial_end ",
	}

	msg := normalizeAndValidateSubscriptionPlanGroups(plan)

	assert.Empty(t, msg)
	assert.Equal(t, "vip", plan.UpgradeGroup)
	assert.Equal(t, "trial_end", plan.DowngradeGroup)
}

func TestSubscriptionPlanGroupValidationAllowsEmptyGroups(t *testing.T) {
	withSubscriptionGroupRatioForTest(t)

	plan := &model.SubscriptionPlan{}

	msg := normalizeAndValidateSubscriptionPlanGroups(plan)

	assert.Empty(t, msg)
	assert.Empty(t, plan.UpgradeGroup)
	assert.Empty(t, plan.DowngradeGroup)
}

func TestSubscriptionPlanGroupValidationRejectsMissingUpgradeGroup(t *testing.T) {
	withSubscriptionGroupRatioForTest(t)

	plan := &model.SubscriptionPlan{UpgradeGroup: "missing"}

	msg := normalizeAndValidateSubscriptionPlanGroups(plan)

	assert.Equal(t, "升级分组不存在", msg)
}

func TestSubscriptionPlanGroupValidationRejectsMissingDowngradeGroup(t *testing.T) {
	withSubscriptionGroupRatioForTest(t)

	plan := &model.SubscriptionPlan{DowngradeGroup: "missing"}

	msg := normalizeAndValidateSubscriptionPlanGroups(plan)

	assert.Equal(t, "降级分组不存在", msg)
}
