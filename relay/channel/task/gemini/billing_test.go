package gemini

import (
	"testing"

	relaycommon "github.com/c1cada/NexusTok/relay/common"

	"github.com/stretchr/testify/require"
)

// TestResolveVeoDurationCapsBillingSeconds 验证 Veo 计费时长在不同来源下都会被钳制。
func TestResolveVeoDurationCapsBillingSeconds(t *testing.T) {
	tests := []struct {
		name        string
		metadata    map[string]any
		duration    int
		seconds     string
		wantSeconds int
	}{
		{
			name:        "metadata durationSeconds is capped",
			metadata:    map[string]any{"durationSeconds": float64(relaycommon.MaxTaskDurationSeconds + 1)},
			wantSeconds: relaycommon.MaxTaskDurationSeconds,
		},
		{
			name:        "standard duration is capped",
			duration:    relaycommon.MaxTaskDurationSeconds + 1,
			wantSeconds: relaycommon.MaxTaskDurationSeconds,
		},
		{
			name:        "seconds string is capped",
			seconds:     "999999",
			wantSeconds: relaycommon.MaxTaskDurationSeconds,
		},
		{
			name:        "normal duration is kept",
			duration:    8,
			wantSeconds: 8,
		},
		{
			name:        "missing duration uses default",
			wantSeconds: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveVeoDuration(tt.metadata, tt.duration, tt.seconds)
			require.Equal(t, tt.wantSeconds, got)
		})
	}
}
