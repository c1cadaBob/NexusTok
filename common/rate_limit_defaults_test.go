package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultGlobalRateLimitIsTenRequestsPerMinute(t *testing.T) {
	assert.Equal(t, 10, DefaultGlobalRateLimitNum)
	assert.Equal(t, 60, DefaultGlobalRateLimitDuration)
}
