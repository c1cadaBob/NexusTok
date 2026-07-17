package upstreamaccount

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRefreshChannelFromSnapshotUpsertsAccountsAndDisablesMissing(t *testing.T) {
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

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   "synced-channel",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-old",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	oldKey := SyncedKey{
		ExternalID: "old",
		Name:       "Old Key",
		Key:        "sk-old",
		MaskedKey:  "sk-old",
		GroupName:  "default",
		Models:     []string{"gpt-old"},
	}
	existingSettings := mergeAccountSyncMetadata("", &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
	}, oldKey)
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Old Key",
		Key:           "sk-old",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-old",
		Group:         "default",
		OtherSettings: existingSettings,
	}
	missingKey := SyncedKey{
		ExternalID: "missing",
		Name:       "Missing Key",
		Key:        "sk-missing",
		MaskedKey:  "sk-missing",
		GroupName:  "default",
	}
	missing := model.ChannelAccount{
		ChannelId: channel.Id,
		Name:      "Missing Key",
		Key:       "sk-missing",
		Status:    common.ChannelStatusEnabled,
		Models:    "gpt-old",
		Group:     "default",
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{
			Platform: PlatformNewAPI,
			BaseURL:  "https://newapi.example",
		}, missingKey),
	}
	require.NoError(t, db.Create(&existing).Error)
	require.NoError(t, db.Create(&missing).Error)

	snapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Balance: &BalanceSnapshot{
			BalanceUSD: floatPtr(8),
			UsedUSD:    floatPtr(2),
		},
		Keys: []SyncedKey{
			{
				ExternalID:        "old",
				Name:              "Old Key Renamed",
				Key:               "sk-old-rotated",
				MaskedKey:         "sk-old-rotated",
				GroupName:         "vip",
				Models:            []string{"gpt-4o"},
				QuotaUsedUSD:      floatPtr(1),
				SuggestedPriority: 3,
				SuggestedWeight:   90,
			},
			{
				ExternalID:        "new",
				Name:              "New Key",
				Key:               "sk-new",
				MaskedKey:         "sk-new",
				GroupName:         "vip",
				Models:            []string{"gpt-4o-mini"},
				SuggestedPriority: 2,
				SuggestedWeight:   80,
			},
		},
	}
	result, err := RefreshChannelFromSnapshot(channel.Id, snapshot, RefreshRequest{
		ChannelID:         channel.Id,
		ApplySuggested:    true,
		DisableMissingKey: true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 1, result.Updated)
	require.Equal(t, 1, result.Disabled)

	var refreshed model.Channel
	require.NoError(t, db.First(&refreshed, channel.Id).Error)
	require.Equal(t, "gpt-4o,gpt-4o-mini", refreshed.Models)
	require.Equal(t, "vip", refreshed.Group)
	require.Equal(t, float64(8), refreshed.Balance)
	require.Equal(t, int64(2), refreshed.UsedQuota)

	var accounts []model.ChannelAccount
	require.NoError(t, db.Order("id ASC").Find(&accounts).Error)
	require.Len(t, accounts, 3)
	require.Equal(t, "Old Key Renamed", accounts[0].Name)
	require.Equal(t, "sk-old-rotated", accounts[0].Key)
	require.Equal(t, int64(3), accounts[0].Priority)
	require.Equal(t, 90, accounts[0].Weight)
	require.Equal(t, common.ChannelStatusManuallyDisabled, accounts[1].Status)
	require.Equal(t, "New Key", accounts[2].Name)
	require.Equal(t, "sk-new", accounts[2].Key)

	var abilityCount int64
	require.NoError(t, db.Model(&model.Ability{}).Count(&abilityCount).Error)
	require.Equal(t, int64(2), abilityCount)
}
