package middleware

import (
	"testing"

	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/require"
)

func TestResolvePoolChannelSettingUsesPoolAccountProxy(t *testing.T) {
	channelSetting := `{"proxy":"http://channel-proxy:8080","force_format":true}`
	channel := &model.Channel{
		Setting: &channelSetting,
	}
	group := &model.AccountPoolGroup{
		Settings: `{"proxy":"http://group-proxy:8080","thinking_to_content":true}`,
	}
	account := &model.PoolAccount{
		Id:    35,
		Proxy: "http://account-proxy:7897",
	}

	setting := resolvePoolChannelSetting(channel, group, account)

	require.Equal(t, "http://account-proxy:7897", setting.Proxy)
	require.True(t, setting.ForceFormat)
	require.True(t, setting.ThinkingToContent)
}

func TestResolvePoolChannelSettingKeepsAccountSettingProxyWhenDedicatedProxyEmpty(t *testing.T) {
	channelSetting := `{"proxy":"http://channel-proxy:8080"}`
	accountSetting := `{"proxy":"http://account-setting-proxy:7897"}`
	channel := &model.Channel{
		Setting: &channelSetting,
	}
	account := &model.PoolAccount{
		Id:      36,
		Setting: &accountSetting,
	}

	setting := resolvePoolChannelSetting(channel, nil, account)

	require.Equal(t, "http://account-setting-proxy:7897", setting.Proxy)
}
