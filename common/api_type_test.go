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
