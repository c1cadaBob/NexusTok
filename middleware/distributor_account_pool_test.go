package middleware

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/gin-gonic/gin"
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

func TestApplyChannelContextStoresChannelAccountRatioConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	clearUpstreamRatioConversionContext(c)

	channel := &model.Channel{
		OtherSettings: upstreamRatioConversionSettings(t, 9),
	}
	account := &model.ChannelAccount{
		Id:            18,
		OtherSettings: upstreamRatioConversionSettings(t, 0.42),
	}

	applyChannelContext(c, channel, account)

	ratio, ok := common.GetContextKeyType[float64](c, constant.ContextKeyUpstreamRatioConversion)
	require.True(t, ok)
	require.InDelta(t, 0.42, ratio, 0.000001)
}

func TestApplyPoolAccountContextStoresPoolAccountRatioConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	clearUpstreamRatioConversionContext(c)

	channel := &model.Channel{
		OtherSettings: upstreamRatioConversionSettings(t, 9),
	}
	account := &model.PoolAccount{
		Id:            35,
		OtherSettings: upstreamRatioConversionSettings(t, 0.35),
	}

	applyPoolAccountContext(c, channel, nil, account)

	ratio, ok := common.GetContextKeyType[float64](c, constant.ContextKeyUpstreamRatioConversion)
	require.True(t, ok)
	require.InDelta(t, 0.35, ratio, 0.000001)
}

func TestApplyChannelContextUsesChannelRatioConversionWhenAccountMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	clearUpstreamRatioConversionContext(c)

	channel := &model.Channel{
		OtherSettings: upstreamRatioConversionSettings(t, 0.88),
	}

	applyChannelContext(c, channel, nil)

	ratio, ok := common.GetContextKeyType[float64](c, constant.ContextKeyUpstreamRatioConversion)
	require.True(t, ok)
	require.InDelta(t, 0.88, ratio, 0.000001)
}

func TestApplyChannelContextIgnoresInvalidAccountRatioWithoutChannelFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	clearUpstreamRatioConversionContext(c)

	channel := &model.Channel{
		OtherSettings: upstreamRatioConversionSettings(t, 0.8),
	}
	account := &model.ChannelAccount{
		Id:            19,
		OtherSettings: upstreamRatioConversionSettings(t, -1),
	}

	applyChannelContext(c, channel, account)

	ratio, ok := common.GetContextKeyType[float64](c, constant.ContextKeyUpstreamRatioConversion)
	require.True(t, ok)
	require.Zero(t, ratio)
}

func TestSetupContextForSelectedChannelClearsStaleRatioConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUpstreamRatioConversion, 0.7)

	channel := &model.Channel{
		Id:   101,
		Name: "regular-single-key",
		Type: constant.ChannelTypeOpenAI,
		Key:  "sk-regular",
	}

	require.Nil(t, SetupContextForSelectedChannel(c, channel, "gpt-test"))

	ratio, ok := common.GetContextKeyType[float64](c, constant.ContextKeyUpstreamRatioConversion)
	require.True(t, ok)
	require.Zero(t, ratio)
}

func upstreamRatioConversionSettings(t *testing.T, ratio float64) string {
	t.Helper()
	settingsBytes, err := common.Marshal(map[string]any{
		"upstream_account_sync": map[string]any{
			"platform":         "new-api",
			"base_url":         "https://upstream.example",
			"ratio_conversion": ratio,
		},
	})
	require.NoError(t, err)
	return string(settingsBytes)
}
