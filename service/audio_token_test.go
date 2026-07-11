package service

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c1cada/NexusTok/common"
)

func TestEstimateAudioDurationTokens(t *testing.T) {
	tests := []struct {
		name     string
		duration float64
		want     int
	}{
		{
			name:     "rounds duration by existing minute formula",
			duration: 60.1,
			want:     1017,
		},
		{
			name:     "negative duration is not allowed to become negative tokens",
			duration: -3,
			want:     0,
		},
		{
			name:     "nan duration falls back to zero",
			duration: math.NaN(),
			want:     0,
		},
		{
			name:     "huge duration saturates before int conversion",
			duration: 1e100,
			want:     common.MaxQuota,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, EstimateAudioDurationTokens(tt.duration))
		})
	}
}

func TestEstimateRealtimeAudioTokens(t *testing.T) {
	t.Run("input uses existing realtime formula", func(t *testing.T) {
		require.Equal(t, 1666, estimateRealtimeAudioInputTokens(60))
	})

	t.Run("output uses existing realtime formula", func(t *testing.T) {
		require.Equal(t, 833, estimateRealtimeAudioOutputTokens(60))
	})

	t.Run("negative duration does not produce negative tokens", func(t *testing.T) {
		require.Zero(t, estimateRealtimeAudioInputTokens(-1))
		require.Zero(t, estimateRealtimeAudioOutputTokens(-1))
	})

	t.Run("nan duration falls back to zero", func(t *testing.T) {
		require.Zero(t, estimateRealtimeAudioInputTokens(math.NaN()))
		require.Zero(t, estimateRealtimeAudioOutputTokens(math.NaN()))
	})

	t.Run("huge duration saturates before int conversion", func(t *testing.T) {
		require.Equal(t, common.MaxQuota, estimateRealtimeAudioInputTokens(1e100))
		require.Equal(t, common.MaxQuota, estimateRealtimeAudioOutputTokens(1e100))
	})
}
