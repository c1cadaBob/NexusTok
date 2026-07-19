package service

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/stretchr/testify/assert"
)

func TestChannelAccountErrorUpdatesAffectCapabilities(t *testing.T) {
	assert.True(t, channelAccountErrorUpdatesAffectCapabilities(map[string]interface{}{
		"status": common.ChannelStatusAutoDisabled,
	}))
	assert.True(t, channelAccountErrorUpdatesAffectCapabilities(map[string]interface{}{
		"rate_limited_until": int64(100),
	}))
	assert.True(t, channelAccountErrorUpdatesAffectCapabilities(map[string]interface{}{
		"overload_until": int64(100),
	}))
	assert.True(t, channelAccountErrorUpdatesAffectCapabilities(map[string]interface{}{
		"temp_disabled_until": int64(100),
	}))
	assert.False(t, channelAccountErrorUpdatesAffectCapabilities(map[string]interface{}{
		"last_error": "upstream rejected request",
	}))
}
