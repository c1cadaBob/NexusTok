// Package controller - channel_account_pool_test.go
// 该文件包含渠道账号池模式验证的单元测试
//
// 测试内容包括：
// - 全局账号池模式必须选择账号池组
// - 全局账号池模式允许空 Key 和 BaseURL
package controller

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAccountPoolChannelTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.AccountPoolGroup{}, &model.Channel{}))
}

func TestValidateChannelGlobalAccountPoolRequiresGroup(t *testing.T) {
	setupAccountPoolChannelTestDB(t)

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "",
		Models: "gpt-4o-mini",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode: constant.ChannelCredentialModeGlobalAccountPool,
		},
	}

	err := validateChannel(channel, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "账号池模式必须选择账号池组")
}

func TestValidateChannelGlobalAccountPoolAllowsEmptyKeyAndBaseURL(t *testing.T) {
	setupAccountPoolChannelTestDB(t)

	group := &model.AccountPoolGroup{
		Name:        "codex-main",
		Platform:    "codex",
		AuthType:    model.AccountPoolAuthTypeOfficialOAuth,
		Source:      model.AccountPoolGroupSourceCLIProxyAPI,
		ExternalKey: "codex-main",
		Status:      common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(group).Error)

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "",
		Models: "gpt-4o-mini",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeGlobalAccountPool,
			AccountPoolGroupId: group.Id,
		},
	}

	require.NoError(t, validateChannel(channel, true))
}

func TestAccountPoolGroupOptionResponseOnlyReturnsSchedulableCLIProxyGroup(t *testing.T) {
	nativeGroup := &model.AccountPoolGroup{
		Id:       1,
		Name:     "local-only",
		Platform: "codex",
		AuthType: model.AccountPoolAuthTypeAPIKey,
		Source:   model.AccountPoolGroupSourceNative,
		Status:   common.ChannelStatusEnabled,
		Stats:    map[string]int64{"total": 3, "enabled": 3},
	}
	item, ok := accountPoolGroupOptionResponse(nativeGroup)
	require.False(t, ok)
	require.Nil(t, item)

	emptyCLIProxyGroup := &model.AccountPoolGroup{
		Id:          2,
		Name:        "empty-remote",
		Platform:    "codex",
		AuthType:    model.AccountPoolAuthTypeOfficialOAuth,
		Source:      model.AccountPoolGroupSourceCLIProxyAPI,
		ExternalKey: "empty-remote",
		Status:      common.ChannelStatusEnabled,
		Stats:       map[string]int64{"total": 0, "enabled": 0},
	}
	item, ok = accountPoolGroupOptionResponse(emptyCLIProxyGroup)
	require.False(t, ok)
	require.Nil(t, item)

	activeCLIProxyGroup := &model.AccountPoolGroup{
		Id:          3,
		Name:        "remote-main",
		Platform:    "codex",
		AuthType:    model.AccountPoolAuthTypeOfficialOAuth,
		Source:      model.AccountPoolGroupSourceCLIProxyAPI,
		ExternalKey: "remote-main",
		Status:      common.ChannelStatusEnabled,
		Stats:       map[string]int64{"total": 2, "enabled": 1},
	}
	item, ok = accountPoolGroupOptionResponse(activeCLIProxyGroup)
	require.True(t, ok)
	require.Equal(t, activeCLIProxyGroup.Id, item["id"])
	require.Equal(t, model.AccountPoolGroupSourceCLIProxyAPI, item["source"])
}

func TestValidateChannelGlobalAccountPoolRejectsNativeGroup(t *testing.T) {
	setupAccountPoolChannelTestDB(t)

	group := &model.AccountPoolGroup{
		Name:     "local-only",
		Platform: "codex",
		AuthType: model.AccountPoolAuthTypeAPIKey,
		Source:   model.AccountPoolGroupSourceNative,
		Status:   common.ChannelStatusEnabled,
	}
	require.NoError(t, model.DB.Create(group).Error)

	channel := &model.Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "",
		Models: "gpt-4o-mini",
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			CredentialMode:     constant.ChannelCredentialModeGlobalAccountPool,
			AccountPoolGroupId: group.Id,
		},
	}

	err := validateChannel(channel, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "账号池模式只能选择 CPAMC 中已创建的账号组")
}
