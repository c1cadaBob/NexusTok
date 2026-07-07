package service

import (
	"context"
	"sync"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSubscriptionMaintenanceTest(t *testing.T) {
	t.Helper()
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, model.DB.AutoMigrate(
		&model.SubscriptionPlan{},
		&model.UserSubscription{},
		&model.SubscriptionPreConsumeRecord{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
	))
	t.Cleanup(func() {
		model.DB = oldDB
		subscriptionCleanupLast.Store(0)
		subscriptionResetRunning.Store(false)
	})
}

func TestRunSubscriptionMaintenanceOnceExpiresResetsAndCleans(t *testing.T) {
	setupSubscriptionMaintenanceTest(t)
	now := model.GetDBTimestamp()

	plan := &model.SubscriptionPlan{
		Title:                   "Daily quota",
		PriceAmount:             1,
		Currency:                "USD",
		DurationUnit:            model.SubscriptionDurationMonth,
		DurationValue:           1,
		TotalAmount:             100,
		QuotaResetPeriod:        model.SubscriptionResetCustom,
		QuotaResetCustomSeconds: 3600,
	}
	require.NoError(t, model.DB.Create(plan).Error)

	expiredSub := &model.UserSubscription{
		UserId:      501,
		PlanId:      plan.Id,
		AmountTotal: 100,
		AmountUsed:  40,
		StartTime:   now - 7200,
		EndTime:     now - 60,
		Status:      "active",
	}
	resetSub := &model.UserSubscription{
		UserId:        502,
		PlanId:        plan.Id,
		AmountTotal:   100,
		AmountUsed:    80,
		StartTime:     now - 7200,
		EndTime:       now + 7200,
		Status:        "active",
		LastResetTime: now - 7200,
		NextResetTime: now - 3600,
	}
	require.NoError(t, model.DB.Create(expiredSub).Error)
	require.NoError(t, model.DB.Create(resetSub).Error)
	require.NoError(t, model.DB.Create(&model.SubscriptionPreConsumeRecord{
		RequestId:          "old-pre-consume",
		UserId:             502,
		UserSubscriptionId: resetSub.Id,
		PreConsumed:        5,
		Status:             "consumed",
	}).Error)
	require.NoError(t, model.DB.Model(&model.SubscriptionPreConsumeRecord{}).
		Where("request_id = ?", "old-pre-consume").
		Updates(map[string]any{
			"created_at": now - 9*24*3600,
			"updated_at": now - 9*24*3600,
		}).Error)

	var states []SubscriptionMaintenanceState
	result, err := RunSubscriptionMaintenanceOnce(context.Background(), func(state SubscriptionMaintenanceState) error {
		states = append(states, state)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.Expired)
	require.Equal(t, 1, result.Reset)
	require.True(t, result.CleanupRan)
	require.Equal(t, int64(1), result.CleanupDeleted)
	require.NotEmpty(t, states)
	require.Equal(t, "finished", states[len(states)-1].Phase)
	require.Equal(t, 100, states[len(states)-1].Progress)

	var reloadedExpired model.UserSubscription
	require.NoError(t, model.DB.Where("id = ?", expiredSub.Id).First(&reloadedExpired).Error)
	require.Equal(t, "expired", reloadedExpired.Status)

	var reloadedReset model.UserSubscription
	require.NoError(t, model.DB.Where("id = ?", resetSub.Id).First(&reloadedReset).Error)
	require.Equal(t, int64(0), reloadedReset.AmountUsed)
	require.Greater(t, reloadedReset.LastResetTime, resetSub.LastResetTime)
	require.Greater(t, reloadedReset.NextResetTime, now)

	var oldRecords int64
	require.NoError(t, model.DB.Model(&model.SubscriptionPreConsumeRecord{}).Where("request_id = ?", "old-pre-consume").Count(&oldRecords).Error)
	require.Equal(t, int64(0), oldRecords)
}

func TestSubscriptionMaintenanceHandlerFinishesSystemTask(t *testing.T) {
	setupSubscriptionMaintenanceTest(t)
	now := model.GetDBTimestamp()
	plan := &model.SubscriptionPlan{
		Title:                   "Hourly quota",
		PriceAmount:             1,
		Currency:                "USD",
		DurationUnit:            model.SubscriptionDurationMonth,
		DurationValue:           1,
		TotalAmount:             100,
		QuotaResetPeriod:        model.SubscriptionResetCustom,
		QuotaResetCustomSeconds: 3600,
	}
	require.NoError(t, model.DB.Create(plan).Error)
	require.NoError(t, model.DB.Create(&model.UserSubscription{
		UserId:        503,
		PlanId:        plan.Id,
		AmountTotal:   100,
		AmountUsed:    75,
		StartTime:     now - 7200,
		EndTime:       now + 7200,
		Status:        "active",
		LastResetTime: now - 7200,
		NextResetTime: now - 3600,
	}).Error)

	task, err := model.CreateSystemTask(model.SystemTaskTypeSubscriptionMaintenance, nil, SubscriptionMaintenanceState{})
	require.NoError(t, err)
	claimedTask, claimed, err := model.ClaimSystemTask(task.ID, model.SystemTaskTypeSubscriptionMaintenance, "runner-subscription-maintenance", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)

	subscriptionMaintenanceHandler{}.Run(context.Background(), claimedTask, "runner-subscription-maintenance")

	finished, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.NotNil(t, finished)
	require.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	require.Nil(t, finished.ActiveKey)

	var state SubscriptionMaintenanceState
	require.NoError(t, finished.DecodeState(&state))
	require.Equal(t, "finished", state.Phase)
	require.Equal(t, 100, state.Progress)
	require.Equal(t, 1, state.Reset)

	var result SubscriptionMaintenanceResult
	require.NoError(t, finished.DecodeResult(&result))
	require.Equal(t, 1, result.Reset)
}

func TestStartSubscriptionQuotaResetTaskQueuesSystemTask(t *testing.T) {
	setupSubscriptionMaintenanceTest(t)
	oldMaster := common.IsMasterNode
	common.IsMasterNode = true
	subscriptionResetOnce = sync.Once{}
	t.Cleanup(func() {
		common.IsMasterNode = oldMaster
		subscriptionResetOnce = sync.Once{}
	})

	StartSubscriptionQuotaResetTask()

	task, err := model.GetActiveSystemTask(model.SystemTaskTypeSubscriptionMaintenance)
	require.NoError(t, err)
	require.NotNil(t, task)
	require.Equal(t, model.SystemTaskStatusPending, task.Status)
}

func TestEnqueueSubscriptionMaintenanceSystemTaskSkipsRecentRun(t *testing.T) {
	setupSubscriptionMaintenanceTest(t)
	finishedAt := common.GetTimestamp()
	require.NoError(t, model.DB.Create(&model.SystemTask{
		TaskID:    "systask_recent_subscription_maintenance",
		Type:      model.SystemTaskTypeSubscriptionMaintenance,
		Status:    model.SystemTaskStatusSucceeded,
		UpdatedAt: finishedAt,
		CreatedAt: finishedAt,
	}).Error)

	task, created, err := enqueueSubscriptionMaintenanceSystemTask()
	require.NoError(t, err)
	require.False(t, created)
	require.NotNil(t, task)
	require.Equal(t, "systask_recent_subscription_maintenance", task.TaskID)

	var count int64
	require.NoError(t, model.DB.Model(&model.SystemTask{}).
		Where("type = ?", model.SystemTaskTypeSubscriptionMaintenance).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}
