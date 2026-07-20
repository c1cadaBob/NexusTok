package controller

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/require"
)

func TestClearChannelInfoSanitizesUpstreamAccountCredential(t *testing.T) {
	encrypted, err := common.EncryptSensitiveString("secret")
	require.NoError(t, err)

	channel := &model.Channel{
		OtherSettings: `{"upstream_account_sync":{"platform":"new-api","base_url":"https://newapi.example","credentials":{"platform":"new-api","base_url":"https://newapi.example","username":"alice","password":"` + encrypted + `"}},"allow_service_tier":false}`,
	}

	clearChannelInfo(channel)

	require.Contains(t, channel.OtherSettings, `"upstream_account_sync"`)
	require.Contains(t, channel.OtherSettings, `"allow_service_tier":false`)
	require.NotContains(t, channel.OtherSettings, "credentials")
	require.NotContains(t, channel.OtherSettings, encrypted)
}
