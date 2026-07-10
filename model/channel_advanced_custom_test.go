package model

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelValidateSettingsRequiresAdvancedCustomConfig(t *testing.T) {
	channel := &Channel{
		Type:          constant.ChannelTypeAdvancedCustom,
		OtherSettings: "{}",
	}

	err := channel.ValidateSettings()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "advanced_custom is required")
}

func TestChannelValidateSettingsValidatesAdvancedCustomConfig(t *testing.T) {
	settings := dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{
				{
					IncomingPath: "/v1/chat/completions",
					UpstreamPath: "https://upstream.example/v1/chat/completions",
				},
			},
		},
	}
	settingsBytes, err := common.Marshal(settings)
	require.NoError(t, err)
	channel := &Channel{
		Type:          constant.ChannelTypeAdvancedCustom,
		OtherSettings: string(settingsBytes),
	}

	require.NoError(t, channel.ValidateSettings())
}

func TestChannelValidateSettingsRejectsInvalidAdvancedCustomOnAnyChannel(t *testing.T) {
	settings := dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{
				{
					IncomingPath: "/v1/messages",
					UpstreamPath: "https://upstream.example/v1/responses",
					Converter:    dto.AdvancedCustomConverterOpenAIResponsesToOpenAIChatCompletions,
				},
			},
		},
	}
	settingsBytes, err := common.Marshal(settings)
	require.NoError(t, err)
	channel := &Channel{
		Type:          constant.ChannelTypeOpenAI,
		OtherSettings: string(settingsBytes),
	}

	err = channel.ValidateSettings()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "converter does not match incoming_path")
}

func TestChannelValidateSettingsRequiresBaseURLForRelativeAdvancedCustomRoute(t *testing.T) {
	settings := dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{
				{
					IncomingPath: "/v1/chat/completions",
					UpstreamPath: "/proxy/v1/chat/completions",
				},
			},
		},
	}
	settingsBytes, err := common.Marshal(settings)
	require.NoError(t, err)
	channel := &Channel{
		Type:          constant.ChannelTypeAdvancedCustom,
		OtherSettings: string(settingsBytes),
	}

	err = channel.ValidateSettings()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires channel base_url")
}
