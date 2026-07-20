package common

import (
	"testing"

	"github.com/c1cada/NexusTok/constant"
	"github.com/stretchr/testify/assert"
)

func TestChannelType2APITypeAdvancedCustom(t *testing.T) {
	apiType, ok := ChannelType2APIType(constant.ChannelTypeAdvancedCustom)

	assert.True(t, ok)
	assert.Equal(t, constant.APITypeAdvancedCustom, apiType)
}

func TestChannelType2APITypeUpstreamAccountPlatforms(t *testing.T) {
	for _, channelType := range []int{
		constant.ChannelTypeNewAPI,
		constant.ChannelTypeSub2API,
	} {
		apiType, ok := ChannelType2APIType(channelType)

		assert.True(t, ok)
		assert.Equal(t, constant.APITypeOpenAI, apiType)
	}
}
