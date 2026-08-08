package controller

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service/upstreamaccount"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelAccountUpdateMapOnlyUpdatesSubmittedFields(t *testing.T) {
	priority := int64(0)
	weight := 0
	maxConcurrency := 0
	req := channelAccountUpsertRequest{
		Name:           " renamed ",
		Models:         "",
		Group:          "",
		Priority:       &priority,
		Weight:         &weight,
		MaxConcurrency: &maxConcurrency,
	}

	updates := channelAccountUpdateMap(nil, req, map[string]any{"name": req.Name})
	assert.Equal(t, map[string]interface{}{"name": "renamed"}, updates)

	updates = channelAccountUpdateMap(nil, req, map[string]any{
		"models":          req.Models,
		"group":           req.Group,
		"priority":        priority,
		"weight":          weight,
		"max_concurrency": maxConcurrency,
	})
	assert.Equal(t, map[string]interface{}{
		"models":          "",
		"group":           "",
		"priority":        int64(0),
		"weight":          0,
		"max_concurrency": 0,
	}, updates)
}

func TestChannelAccountUpdateMapKeepsEmptyKeyFromOverwritingCredential(t *testing.T) {
	req := channelAccountUpsertRequest{Key: "   "}

	updates := channelAccountUpdateMap(nil, req, map[string]any{"key": req.Key})

	assert.NotContains(t, updates, "key")
}

func TestChannelAccountUpdateMapUsesGormColumnForOpenAIOrganization(t *testing.T) {
	organization := "org-example"
	req := channelAccountUpsertRequest{OpenAIOrganization: &organization}

	updates := channelAccountUpdateMap(nil, req, map[string]any{
		"openai_organization": organization,
	})

	assert.Equal(t, map[string]interface{}{
		"open_ai_organization": organization,
	}, updates)
	assert.NotContains(t, updates, "openai_organization")
}

func TestChannelAccountUpdateMapPreservesUpstreamSyncMetadata(t *testing.T) {
	existing := &model.ChannelAccount{
		OtherSettings: upstreamaccount.PreserveAccountSyncMetadata(
			`{"upstream_account_sync":{"platform":"sub2api","base_url":"http://example.test","external_id":"9001","key_digest":"digest","synced_at":1}}`,
			`{"local_flag":false}`,
		),
	}
	req := channelAccountUpsertRequest{OtherSettings: `{"local_flag":true}`}

	updates := channelAccountUpdateMap(existing, req, map[string]any{
		"settings": req.OtherSettings,
	})

	settings, ok := updates["settings"].(string)
	assert.True(t, ok)
	assert.Contains(t, settings, `"local_flag":true`)
	assert.Contains(t, settings, `"upstream_account_sync"`)
	assert.Contains(t, settings, `"external_id":"9001"`)
}

func TestChannelAccountSensitiveChangeClassification(t *testing.T) {
	baseURL := "https://api.example.com"
	account := &model.ChannelAccount{
		Key:     "old-key",
		Status:  common.ChannelStatusEnabled,
		Models:  "gpt-5.6",
		Group:   "default",
		BaseURL: &baseURL,
	}

	t.Run("普通调度字段不需要敏感写权限", func(t *testing.T) {
		req := channelAccountUpsertRequest{
			Name:   "renamed",
			Models: "gpt-5.6,gpt-5.6-mini",
			Group:  "vip",
		}

		assert.False(t, channelAccountHasSensitiveChanges(account, req, map[string]any{
			"name":   req.Name,
			"models": req.Models,
			"group":  req.Group,
		}))
	})

	t.Run("密钥变化需要敏感写权限", func(t *testing.T) {
		req := channelAccountUpsertRequest{Key: "new-key"}

		assert.True(t, channelAccountHasSensitiveChanges(account, req, map[string]any{
			"key": req.Key,
		}))
	})

	t.Run("状态变化需要敏感写权限", func(t *testing.T) {
		disabled := common.ChannelStatusManuallyDisabled
		req := channelAccountUpsertRequest{Status: &disabled}

		assert.True(t, channelAccountHasSensitiveChanges(account, req, map[string]any{
			"status": disabled,
		}))
	})

	t.Run("清空上游地址需要敏感写权限", func(t *testing.T) {
		req := channelAccountUpsertRequest{}

		assert.True(t, channelAccountHasSensitiveChanges(account, req, map[string]any{
			"base_url": nil,
		}))
	})
}

func TestChannelAccountUnknownFieldsFailClosed(t *testing.T) {
	assert.False(t, channelAccountHasUnknownFields(map[string]any{
		"name":   "renamed",
		"models": "gpt-5.6",
	}))
	assert.True(t, channelAccountHasUnknownFields(map[string]any{
		"future_secret_field": "value",
	}))
}

func TestSyncChannelAccountCapabilitiesIfNeededRebuildsAccountPoolAbilities(t *testing.T) {
	oldDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelAccount{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.InitChannelCache()
	})

	channel := model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    constant.ChannelCredentialModeAccountPool,
		Name:   "synced",
		Status: common.ChannelStatusEnabled,
		Models: "gpt-old,gpt-b",
		Group:  "default,vip",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:      constant.ChannelCredentialModeAccountPool,
			AccountPoolEnabled:  true,
			AccountPoolMode:     constant.ChannelAccountPoolModePolling,
			AccountPoolFallback: false,
		},
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&[]model.ChannelAccount{
		{
			ChannelId: channel.Id,
			Name:      "enabled",
			Key:       "sk-enabled",
			Status:    common.ChannelStatusEnabled,
			Models:    "gpt-new",
			Group:     "vip",
		},
		{
			ChannelId: channel.Id,
			Name:      "disabled",
			Key:       "sk-disabled",
			Status:    common.ChannelStatusManuallyDisabled,
			Models:    "gpt-disabled",
			Group:     "default",
		},
	}).Error)
	require.NoError(t, (&channel).UpdateAbilities(nil))

	require.NoError(t, syncChannelAccountCapabilitiesIfNeeded(channel.Id))

	var refreshed model.Channel
	require.NoError(t, db.First(&refreshed, channel.Id).Error)
	assert.Equal(t, "gpt-new", refreshed.Models)
	assert.Equal(t, "vip", refreshed.Group)

	var abilities []model.Ability
	require.NoError(t, db.Order("model ASC").Find(&abilities).Error)
	require.Len(t, abilities, 1)
	assert.Equal(t, "gpt-new", abilities[0].Model)
	assert.Equal(t, "vip", abilities[0].Group)
}

func TestChannelAccountUpdatesAffectCapabilities(t *testing.T) {
	assert.True(t, channelAccountUpdatesAffectCapabilities(map[string]interface{}{
		"models": "gpt-new",
	}))
	assert.True(t, channelAccountUpdatesAffectCapabilities(map[string]interface{}{
		"group": "vip",
	}))
	assert.True(t, channelAccountUpdatesAffectCapabilities(map[string]interface{}{
		"status": common.ChannelStatusManuallyDisabled,
	}))
	assert.False(t, channelAccountUpdatesAffectCapabilities(map[string]interface{}{
		"name": "renamed",
	}))
}

func TestValidateSyncedChannelAccountCapabilityForUpdate(t *testing.T) {
	syncSettings := upstreamaccount.PreserveAccountSyncMetadata(
		`{"upstream_account_sync":{"platform":"new-api","base_url":"https://newapi.example","external_id":"key-1","key_digest":"digest","synced_at":1}}`,
		`{"local_flag":true}`,
	)
	account := &model.ChannelAccount{
		Name:          "Synced Key",
		Status:        common.ChannelStatusEnabled,
		Models:        "gpt-4o",
		AccessGroups:  "default",
		OtherSettings: syncSettings,
	}

	assert.ErrorContains(
		t,
		validateSyncedChannelAccountCapabilityForUpdate(account, map[string]interface{}{"models": ""}),
		"必须配置至少一个模型",
	)
	assert.ErrorContains(
		t,
		validateSyncedChannelAccountCapabilityForUpdate(account, map[string]interface{}{"access_groups": ""}),
		"必须配置至少一个 NexusTok 可访问用户组",
	)
	assert.NoError(
		t,
		validateSyncedChannelAccountCapabilityForUpdate(account, map[string]interface{}{
			"status":        common.ChannelStatusManuallyDisabled,
			"models":        "",
			"access_groups": "",
		}),
	)
	assert.NoError(
		t,
		validateSyncedChannelAccountCapabilityForUpdate(&model.ChannelAccount{
			Name:         "普通账号",
			Status:       common.ChannelStatusEnabled,
			Models:       "",
			AccessGroups: "",
		}, map[string]interface{}{"models": "", "access_groups": ""}),
	)
}
