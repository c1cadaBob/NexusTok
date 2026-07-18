package upstreamaccount

import (
	"context"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRefreshChannelFromCredentialConsumesPreviewSnapshot(t *testing.T) {
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

	preview, err := SavePreviewSnapshot(&Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Balance:  &BalanceSnapshot{BalanceUSD: floatPtr(5), UsedUSD: floatPtr(1)},
		Keys: []SyncedKey{{
			ExternalID:        "new",
			Name:              "New Key",
			Key:               "sk-new",
			MaskedKey:         "sk-new",
			GroupName:         "default",
			Models:            []string{"gpt-4o"},
			SuggestedPriority: 1,
			SuggestedWeight:   100,
		}},
	})
	require.NoError(t, err)

	result, err := RefreshChannelFromCredential(context.Background(), RefreshRequest{
		ChannelID:         channel.Id,
		PreviewID:         preview.PreviewID,
		ApplySuggested:    true,
		DisableMissingKey: true,
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 0, result.Updated)

	var accounts []model.ChannelAccount
	require.NoError(t, db.Find(&accounts).Error)
	require.Len(t, accounts, 1)
	require.Equal(t, "sk-new", accounts[0].Key)

	_, err = GetPreviewRecord(preview.PreviewID)
	require.Error(t, err)
}

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
	require.Equal(t, int64(common.QuotaPerUnit*2), refreshed.UsedQuota)

	var accounts []model.ChannelAccount
	require.NoError(t, db.Order("id ASC").Find(&accounts).Error)
	require.Len(t, accounts, 3)
	require.Equal(t, "Old Key Renamed", accounts[0].Name)
	require.Equal(t, "sk-old-rotated", accounts[0].Key)
	require.Equal(t, int64(3), accounts[0].Priority)
	require.Equal(t, 90, accounts[0].Weight)
	require.Equal(t, int64(common.QuotaPerUnit), accounts[0].UsedQuota)
	require.Equal(t, common.ChannelStatusManuallyDisabled, accounts[1].Status)
	require.Equal(t, "New Key", accounts[2].Name)
	require.Equal(t, "sk-new", accounts[2].Key)

	var abilityCount int64
	require.NoError(t, db.Model(&model.Ability{}).Count(&abilityCount).Error)
	require.Equal(t, int64(2), abilityCount)
}

func TestRefreshChannelFromSnapshotPreservesLocalAccountOverrides(t *testing.T) {
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
		ExternalID: "local",
		Name:       "Local Key",
		Key:        "sk-old-local",
		MaskedKey:  "sk-old-local",
		GroupName:  "default",
		Models:     []string{"gpt-old"},
	}
	localBaseURL := "https://local-overridden.example/v1"
	localOrg := "org-local"
	localSetting := `{"timeout":30}`
	localModelMapping := `{"gpt-local":"gpt-upstream"}`
	localParamOverride := `{"temperature":0}`
	localHeaderOverride := `{"X-Local":"yes"}`
	localStatusCodeMapping := `{"401":"channel_invalid_key"}`
	existing := model.ChannelAccount{
		ChannelId:          channel.Id,
		Name:               "Local Key",
		Key:                "sk-old-local",
		Status:             common.ChannelStatusEnabled,
		Models:             "gpt-old",
		Group:              "default",
		Priority:           11,
		Weight:             22,
		UsedQuota:          3,
		BaseURL:            &localBaseURL,
		OpenAIOrganization: &localOrg,
		Other:              "local-other",
		Setting:            &localSetting,
		OtherSettings: mergeAccountSyncMetadata(`{"local_flag":true}`, &Snapshot{
			Platform: PlatformNewAPI,
			BaseURL:  "https://newapi.example",
		}, oldKey),
		ModelMapping:      &localModelMapping,
		ParamOverride:     &localParamOverride,
		HeaderOverride:    &localHeaderOverride,
		StatusCodeMapping: &localStatusCodeMapping,
		MaxConcurrency:    7,
	}
	require.NoError(t, db.Create(&existing).Error)

	snapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Balance: &BalanceSnapshot{
			BalanceUSD: floatPtr(5),
			UsedUSD:    floatPtr(2),
		},
		Keys: []SyncedKey{
			{
				ExternalID:        "local",
				Name:              "Local Key Renamed",
				Key:               "sk-new-local",
				MaskedKey:         "sk-new-local",
				GroupName:         "vip",
				Models:            []string{"gpt-4o"},
				SuggestedPriority: 8,
				SuggestedWeight:   66,
			},
		},
	}
	result, err := RefreshChannelFromSnapshot(channel.Id, snapshot, RefreshRequest{
		ChannelID:      channel.Id,
		ApplySuggested: true,
	})

	require.NoError(t, err)
	require.Equal(t, 0, result.Created)
	require.Equal(t, 1, result.Updated)
	require.Equal(t, 0, result.Disabled)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, "Local Key Renamed", refreshed.Name)
	require.Equal(t, "sk-new-local", refreshed.Key)
	require.Equal(t, "gpt-4o", refreshed.Models)
	require.Equal(t, "vip", refreshed.Group)
	require.Equal(t, int64(8), refreshed.Priority)
	require.Equal(t, 66, refreshed.Weight)
	require.Equal(t, localBaseURL, *refreshed.BaseURL)
	require.Equal(t, localOrg, *refreshed.OpenAIOrganization)
	require.Equal(t, "local-other", refreshed.Other)
	require.Equal(t, localSetting, *refreshed.Setting)
	require.Equal(t, localModelMapping, *refreshed.ModelMapping)
	require.Equal(t, localParamOverride, *refreshed.ParamOverride)
	require.Equal(t, localHeaderOverride, *refreshed.HeaderOverride)
	require.Equal(t, localStatusCodeMapping, *refreshed.StatusCodeMapping)
	require.Equal(t, 7, refreshed.MaxConcurrency)

	var settings map[string]any
	require.NoError(t, common.UnmarshalJsonStr(refreshed.OtherSettings, &settings))
	require.Equal(t, true, settings["local_flag"])
	metadata := readAccountSyncMetadata(refreshed.OtherSettings)
	require.Equal(t, PlatformNewAPI, metadata.Platform)
	require.Equal(t, "https://newapi.example", metadata.BaseURL)
	require.Equal(t, "local", metadata.ExternalID)
	require.Equal(t, keyDigest("sk-new-local"), metadata.KeyDigest)
}

func TestRefreshChannelFromSnapshotKeepsManualSchedulingWhenSuggestionsDisabled(t *testing.T) {
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
		Name:   "manual-scheduling-channel",
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
		ExternalID: "manual",
		Name:       "Manual Key",
		Key:        "sk-manual-old",
		MaskedKey:  "sk-manual-old",
		GroupName:  "default",
		Models:     []string{"gpt-old"},
	}
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Manual Key",
		Key:           "sk-manual-old",
		Status:        common.ChannelStatusManuallyDisabled,
		Models:        "gpt-old",
		Group:         "default",
		Priority:      33,
		Weight:        44,
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformSub2API, BaseURL: "https://sub2api.example"}, oldKey),
	}
	require.NoError(t, db.Create(&existing).Error)

	snapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub2api.example",
		Keys: []SyncedKey{
			{
				ExternalID:        "manual",
				Name:              "Manual Key Refreshed",
				Key:               "sk-manual-new",
				MaskedKey:         "sk-manual-new",
				GroupName:         "vip",
				Models:            []string{"gpt-4o"},
				SuggestedPriority: 8,
				SuggestedWeight:   66,
			},
		},
	}
	result, err := RefreshChannelFromSnapshot(channel.Id, snapshot, RefreshRequest{
		ChannelID:      channel.Id,
		ApplySuggested: false,
	})

	require.NoError(t, err)
	require.Equal(t, 0, result.Created)
	require.Equal(t, 1, result.Updated)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, "Manual Key Refreshed", refreshed.Name)
	require.Equal(t, "sk-manual-new", refreshed.Key)
	require.Equal(t, int64(33), refreshed.Priority)
	require.Equal(t, 44, refreshed.Weight)
	require.Equal(t, common.ChannelStatusManuallyDisabled, refreshed.Status)
}

func TestRefreshChannelFromSnapshotAppliesConfigBySyncIDWhenExternalIDMissing(t *testing.T) {
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
		Name:   "sync-id-refresh-channel",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-4o",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	key := SyncedKey{
		Name:              "No External ID",
		Key:               "sk-no-external-id",
		MaskedKey:         "sk-noe...l-id",
		GroupName:         "default",
		Models:            []string{"gpt-4o"},
		SuggestedPriority: 1,
		SuggestedWeight:   100,
	}
	snapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub2api.example",
		Keys:     []SyncedKey{key},
	}
	ApplySyncIDs(snapshot)

	result, err := RefreshChannelFromSnapshot(channel.Id, snapshot, RefreshRequest{
		ChannelID:      channel.Id,
		ApplySuggested: false,
		Accounts: []AccountCreateConfig{
			{SyncID: snapshot.Keys[0].SyncID, Priority: int64Ptr(12), Weight: intPtr(34)},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Created)

	var account model.ChannelAccount
	require.NoError(t, db.First(&account).Error)
	require.Equal(t, int64(12), account.Priority)
	require.Equal(t, 34, account.Weight)
}

func TestRefreshChannelFromSnapshotFallsBackWhenSyncedAccountModelsAndGroupAreEmpty(t *testing.T) {
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
		Name:   "explicit-empty-channel",
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

	key := SyncedKey{
		ExternalID: "empty",
		Name:       "Empty Key",
		Key:        "sk-empty",
		MaskedKey:  "sk-empty",
		GroupName:  "vip",
		Models:     []string{"gpt-4o"},
	}
	existing := model.ChannelAccount{
		ChannelId:     channel.Id,
		Name:          "Empty Key",
		Key:           "sk-empty",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-old",
		Group:         "default",
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{Platform: PlatformNewAPI, BaseURL: "https://newapi.example"}, key),
	}
	require.NoError(t, db.Create(&existing).Error)

	result, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{
			{
				ExternalID:        "empty",
				Name:              "Empty Key",
				Key:               "sk-empty-new",
				MaskedKey:         "sk-empty-new",
				GroupName:         "vip",
				Models:            []string{"gpt-4o"},
				SuggestedPriority: 5,
				SuggestedWeight:   70,
			},
		},
	}, RefreshRequest{
		ChannelID:      channel.Id,
		ApplySuggested: false,
		Accounts: []AccountCreateConfig{
			{
				ExternalID: "empty",
				Models:     "",
				Group:      "",
				Priority:   int64Ptr(9),
				Weight:     intPtr(8),
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 0, result.Created)
	require.Equal(t, 1, result.Updated)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, existing.Id).Error)
	require.Equal(t, "gpt-4o", refreshed.Models)
	require.Equal(t, "vip", refreshed.Group)
	require.Equal(t, int64(9), refreshed.Priority)
	require.Equal(t, 8, refreshed.Weight)
}

func TestRefreshChannelFromSnapshotDisablesMissingSub2APIKeyWithLoginMetadataURL(t *testing.T) {
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
		Name:   "sub2-login-metadata-channel",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-4o",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled: true,
			AccountPoolMode:    constant.ChannelAccountPoolModePolling,
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	missingKey := SyncedKey{
		ExternalID: "missing",
		Name:       "Missing",
		Key:        "sk-missing",
		MaskedKey:  "sk-missing",
		GroupName:  "default",
	}
	account := model.ChannelAccount{
		ChannelId: channel.Id,
		Name:      "Missing",
		Key:       "sk-missing",
		Status:    common.ChannelStatusEnabled,
		Models:    "gpt-4o",
		Group:     "default",
		OtherSettings: mergeAccountSyncMetadata("", &Snapshot{
			Platform: PlatformSub2API,
			BaseURL:  "https://sub2api.example/login",
		}, missingKey),
	}
	require.NoError(t, db.Create(&account).Error)

	result, err := RefreshChannelFromSnapshot(channel.Id, &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub2api.example",
		Keys:     []SyncedKey{},
	}, RefreshRequest{
		ChannelID:         channel.Id,
		DisableMissingKey: true,
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Disabled)

	var refreshed model.ChannelAccount
	require.NoError(t, db.First(&refreshed, account.Id).Error)
	require.Equal(t, common.ChannelStatusManuallyDisabled, refreshed.Status)
}
