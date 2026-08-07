package upstreamaccount

import (
	"strings"
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
		BaseURL:  "https://newapi.example/",
		StoredCredential: &StoredCredential{
			Platform:  PlatformNewAPI,
			BaseURL:   "https://newapi.example",
			Username:  "alice",
			Password:  mustEncryptSensitiveString(t, "secret"),
			UpdatedAt: common.GetTimestamp(),
		},
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
			Name:    "synced-channel",
			Type:    constant.ChannelTypeOpenAI,
			BaseURL: stringPtr("https://should-use-snapshot.example/"),
		},
		Accounts: []AccountCreateConfig{
			{ExternalID: "b", Priority: int64Ptr(9), Weight: intPtr(7), BaseURL: stringPtr("https://account.example///")},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 2, result.Created)
	require.Greater(t, result.ChannelID, 0)

	var channel model.Channel
	require.NoError(t, db.First(&channel, result.ChannelID).Error)
	require.Equal(t, constant.ChannelTypeNewAPI, channel.Type)
	require.Equal(t, constant.ChannelCredentialModeAccountPool, channel.Key)
	require.Equal(t, constant.ChannelCredentialModeAccountPool, channel.ChannelInfo.CredentialMode)
	require.True(t, channel.ChannelInfo.AccountPoolEnabled)
	require.NotNil(t, channel.BaseURL)
	require.Equal(t, "https://newapi.example", *channel.BaseURL)
	require.ElementsMatch(t, []string{"gpt-4o", "gpt-4o-mini"}, strings.Split(channel.Models, ","))
	require.Equal(t, "default", channel.Group)
	require.Equal(t, float64(3.5), channel.Balance)
	require.Equal(t, int64(common.QuotaPerUnit*1.2), channel.UsedQuota)
	credential, ok, err := ReadChannelSyncCredential(channel.OtherSettings)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, PlatformNewAPI, credential.Platform)
	require.Equal(t, "https://newapi.example", credential.BaseURL)
	require.Equal(t, "alice", credential.Username)
	require.Equal(t, "secret", credential.Password)
	require.NotContains(t, SanitizeChannelSyncSettings(channel.OtherSettings), "credentials")

	var accounts []model.ChannelAccount
	require.NoError(t, db.Find(&accounts).Error)
	require.Len(t, accounts, 2)
	accountsByKey := map[string]model.ChannelAccount{}
	for _, account := range accounts {
		accountsByKey[account.Key] = account
	}
	require.Contains(t, accountsByKey, "sk-a")
	require.Contains(t, accountsByKey, "sk-b")
	accountA := accountsByKey["sk-a"]
	accountB := accountsByKey["sk-b"]
	require.Empty(t, stringPtrValue(accountA.BaseURL))
	require.Equal(t, "vip", accountA.Group)
	require.Equal(t, int64(2), accountA.Priority)
	require.Equal(t, 100, accountA.Weight)
	require.Equal(t, int64(common.QuotaPerUnit), accountA.UsedQuota)
	require.NotNil(t, accountB.BaseURL)
	require.Equal(t, "default", accountB.Group)
	require.Equal(t, "https://account.example", *accountB.BaseURL)
	require.Equal(t, int64(9), accountB.Priority)
	require.Equal(t, 7, accountB.Weight)

	var abilityCount int64
	require.NoError(t, db.Model(&model.Ability{}).Count(&abilityCount).Error)
	require.Equal(t, int64(2), abilityCount)

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

func TestCreateFromPreviewAppliesConfigBySyncIDWhenExternalIDMissing(t *testing.T) {
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

	previewID := "create-preview-sync-id"
	snapshot := &Snapshot{
		Platform: PlatformSub2API,
		BaseURL:  "https://sub2api.example",
		Keys: []SyncedKey{
			{
				Name:              "No External ID",
				Key:               "sk-no-external-id",
				MaskedKey:         "sk-noe...l-id",
				GroupName:         "default",
				Models:            []string{"gpt-4o"},
				SuggestedPriority: 1,
				SuggestedWeight:   100,
			},
		},
	}
	ApplySyncIDs(snapshot)
	require.NoError(t, previewCache.SetWithTTL(previewID, PreviewRecord{
		ID:        previewID,
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
		Snapshot:  snapshot,
	}, time.Minute))

	result, err := CreateFromPreview(CreateRequest{
		PreviewID:      previewID,
		ApplySuggested: false,
		Channel: ChannelCreateConfig{
			Name: "sync-id-channel",
			Type: constant.ChannelTypeOpenAI,
		},
		Accounts: []AccountCreateConfig{
			{SyncID: snapshot.Keys[0].SyncID, Enabled: boolPtr(false), Priority: int64Ptr(9), Weight: intPtr(7)},
		},
	})

	require.ErrorContains(t, err, "没有可创建的同步密钥")
	require.Nil(t, result)
}

func TestCreateFromPreviewSummarizesOnlyEnabledAccounts(t *testing.T) {
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

	previewID := "create-preview-enabled-only"
	snapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{
			{
				ExternalID:        "enabled",
				Name:              "Enabled Key",
				Key:               "sk-enabled",
				MaskedKey:         "sk-enabled",
				GroupName:         "enabled-group",
				Models:            []string{"gpt-enabled"},
				SuggestedPriority: 1,
				SuggestedWeight:   100,
			},
			{
				ExternalID:        "disabled",
				Name:              "Disabled Key",
				Key:               "sk-disabled",
				MaskedKey:         "sk-disabled",
				GroupName:         "disabled-group",
				Models:            []string{"gpt-disabled"},
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
			Name: "enabled-only-channel",
			Type: constant.ChannelTypeOpenAI,
		},
		Accounts: []AccountCreateConfig{
			{ExternalID: "disabled", Enabled: boolPtr(false)},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Created)

	var channel model.Channel
	require.NoError(t, db.First(&channel, result.ChannelID).Error)
	require.Equal(t, "gpt-enabled", channel.Models)
	require.Equal(t, "default", channel.Group)

	var abilities []model.Ability
	require.NoError(t, db.Find(&abilities).Error)
	require.Len(t, abilities, 1)
	require.Equal(t, "gpt-enabled", abilities[0].Model)
	require.Equal(t, "default", abilities[0].Group)
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
	require.Equal(t, constant.ChannelTypeSub2API, channel.Type)
	require.Equal(t, "", channel.Models)
	require.Equal(t, "default", channel.Group)

	var abilityCount int64
	require.NoError(t, db.Model(&model.Ability{}).Count(&abilityCount).Error)
	require.Equal(t, int64(0), abilityCount)
}

func TestCreateFromPreviewKeepsExplicitEmptySyncedAccountModels(t *testing.T) {
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

	previewID := "create-preview-explicit-empty"
	snapshot := &Snapshot{
		Platform: PlatformNewAPI,
		BaseURL:  "https://newapi.example",
		Keys: []SyncedKey{
			{
				ExternalID:        "empty",
				Name:              "Empty Key",
				Key:               "sk-empty",
				MaskedKey:         "sk-empty",
				GroupName:         "vip",
				Models:            []string{"gpt-4o"},
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
			Name:   "explicit-empty-channel",
			Type:   constant.ChannelTypeOpenAI,
			Group:  "",
			Models: "",
		},
		Accounts: []AccountCreateConfig{
			{
				ExternalID: "empty",
				Models:     strPtr(""),
				Group:      "",
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, result.Created)

	var account model.ChannelAccount
	require.NoError(t, db.First(&account).Error)
	require.Equal(t, "", account.Models)
	require.Equal(t, "vip", account.Group)
}

func strPtr(value string) *string {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func mustEncryptSensitiveString(t *testing.T, value string) string {
	t.Helper()
	encrypted, err := common.EncryptSensitiveString(value)
	require.NoError(t, err)
	return encrypted
}
