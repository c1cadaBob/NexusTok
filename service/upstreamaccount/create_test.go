package upstreamaccount

import (
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCreateFromPreviewCreatesChannelAndAccounts(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	previewID := "create-preview"
	snapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Balance: &BalanceSnapshot{
			BalanceUSD: floatPtr(3.5),
			UsedUSD:    floatPtr(1.2),
		},
		Keys: []SyncedKey{
			{
				ExternalID:        "a",
				Name:              "A",
				Key:               "sk-a",
				MaskedKey:         "sk-a",
				GroupName:         "vip",
				Models:            []string{"gpt-4o"},
				QuotaUsedUSD:      floatPtr(1),
				SuggestedPriority: 2,
				SuggestedWeight:   100,
			},
			{
				ExternalID:        "b",
				Name:              "B",
				Key:               "sk-b",
				MaskedKey:         "sk-b",
				GroupName:         "default",
				Models:            []string{"gpt-4o-mini"},
				SuggestedPriority: 1,
				SuggestedWeight:   50,
			},
		},
	}
	require.NoError(t, previewCache.SetWithTTL(previewID, PreviewRecord{
		ID:        previewID,
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
		Snapshot:  snapshot,
	}, time.Minute))

	result, err := CreateFromPreview(CreateRequest{
		PreviewID:      previewID,
		ApplySuggested: true,
		Channel: ChannelCreateConfig{
			Name: "synced-channel",
			Type: constant.ChannelTypeOpenAI,
		},
		Accounts: []AccountCreateConfig{
			{ExternalID: "b", Priority: int64Ptr(9), Weight: intPtr(7)},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 2, result.Created)
	require.Greater(t, result.ChannelID, 0)

	var channel model.Channel
	require.NoError(t, db.First(&channel, result.ChannelID).Error)
	require.Equal(t, constant.ChannelCredentialModeAccountPool, channel.Key)
	require.Equal(t, constant.ChannelCredentialModeAccountPool, channel.ChannelInfo.CredentialMode)
	require.True(t, channel.ChannelInfo.AccountPoolEnabled)
	require.Equal(t, "gpt-4o,gpt-4o-mini", channel.Models)
	require.Equal(t, "vip,default", channel.Group)
	require.Equal(t, float64(3.5), channel.Balance)
	require.Equal(t, int64(1), channel.UsedQuota)

	var accounts []model.ChannelAccount
	require.NoError(t, db.Order("key ASC").Find(&accounts).Error)
	require.Len(t, accounts, 2)
	require.Equal(t, "sk-a", accounts[0].Key)
	require.Equal(t, int64(2), accounts[0].Priority)
	require.Equal(t, 100, accounts[0].Weight)
	require.Equal(t, "sk-b", accounts[1].Key)
	require.Equal(t, int64(9), accounts[1].Priority)
	require.Equal(t, 7, accounts[1].Weight)

	var abilityCount int64
	require.NoError(t, db.Model(&model.Ability{}).Count(&abilityCount).Error)
	require.Equal(t, int64(4), abilityCount)

	_, err = GetPreviewRecord(previewID)
	require.ErrorContains(t, err, "预览快照不存在或已过期")
	_, err = CreateFromPreview(CreateRequest{
		PreviewID: previewID,
		Channel: ChannelCreateConfig{
			Name: "duplicated-channel",
			Type: constant.ChannelTypeOpenAI,
		},
	})
	require.ErrorContains(t, err, "预览快照不存在或已过期")
}

func TestCreateFromPreviewAllowsDeferredTypeAndModels(t *testing.T) {
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	previewID := "create-preview-deferred"
	snapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub2api.example",
		Keys: []SyncedKey{
			{
				ExternalID:        "deferred",
				Name:              "Deferred",
				Key:               "sk-deferred",
				MaskedKey:         "sk-deferred",
				GroupName:         "default",
				SuggestedPriority: 1,
				SuggestedWeight:   100,
			},
		},
	}
	require.NoError(t, previewCache.SetWithTTL(previewID, PreviewRecord{
		ID:        previewID,
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
		Snapshot:  snapshot,
	}, time.Minute))

	result, err := CreateFromPreview(CreateRequest{
		PreviewID:      previewID,
		ApplySuggested: true,
		Channel: ChannelCreateConfig{
			Name: "deferred-channel",
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Created)

	var channel model.Channel
	require.NoError(t, db.First(&channel, result.ChannelID).Error)
	require.Equal(t, constant.ChannelTypeOpenAI, channel.Type)
	require.Equal(t, "", channel.Models)
	require.Equal(t, "default", channel.Group)

	var abilityCount int64
	require.NoError(t, db.Model(&model.Ability{}).Count(&abilityCount).Error)
	require.Equal(t, int64(0), abilityCount)
}

func int64Ptr(value int64) *int64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}
