package service

import (
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/stretchr/testify/require"
)

func clearRoutingTTFTHealthCacheForTest(t *testing.T) {
	t.Helper()
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	require.NoError(t, getRoutingTTFTHealthCache().Purge())
	t.Cleanup(func() { _ = getRoutingTTFTHealthCache().Purge() })
}

func newTTFTRoutingCandidate(channelID, accountID int) *model.RoutingCandidate {
	kind := model.RoutingCredentialKindSingleKey
	if accountID > 0 {
		kind = model.RoutingCredentialKindChannelAccount
	}
	return &model.RoutingCandidate{
		ChannelID:        channelID,
		ChannelAccountID: accountID,
		Kind:             kind,
	}
}

func recordSlowTTFTSamples(t *testing.T, group, modelName string, candidate *model.RoutingCandidate) {
	t.Helper()
	for i := 0; i < 3; i++ {
		RecordRoutingCandidateTTFTSample(group, modelName, candidate, 9000)
	}
}

func TestRoutingCandidateTTFTHasNoEffectWithoutSamples(t *testing.T) {
	clearRoutingTTFTHealthCacheForTest(t)

	candidate := newTTFTRoutingCandidate(1, 0)
	require.True(t, IsRoutingCandidateTTFTHealthy("default", "gpt-ttft", candidate))
}

func TestRoutingCandidateTTFTCoolsSlowCandidateAndRecovers(t *testing.T) {
	clearRoutingTTFTHealthCacheForTest(t)
	t.Setenv(routingTTFTThresholdEnv, "8000")
	t.Setenv(routingTTFTCooldownEnv, "600")
	t.Setenv(routingTTFTMinSamplesEnv, "3")

	candidate := newTTFTRoutingCandidate(1, 0)
	recordSlowTTFTSamples(t, "default", "gpt-ttft", candidate)
	require.False(t, IsRoutingCandidateTTFTHealthy("default", "gpt-ttft", candidate))

	// 仅有少量快样本时，最近窗口的 P90 仍然较高，应继续保持冷却。
	RecordRoutingCandidateTTFTSample("default", "gpt-ttft", candidate, 1000)
	require.False(t, IsRoutingCandidateTTFTHealthy("default", "gpt-ttft", candidate))

	for i := 0; i < 20; i++ {
		RecordRoutingCandidateTTFTSample("default", "gpt-ttft", candidate, 1000)
	}
	require.True(t, IsRoutingCandidateTTFTHealthy("default", "gpt-ttft", candidate))
}

func TestRoutingCandidateTTFTIsolatedByAccount(t *testing.T) {
	clearRoutingTTFTHealthCacheForTest(t)
	t.Setenv(routingTTFTMinSamplesEnv, "3")

	slowAccount := newTTFTRoutingCandidate(27, 164)
	fastAccount := newTTFTRoutingCandidate(27, 165)
	recordSlowTTFTSamples(t, "default", "gpt-5.5", slowAccount)

	require.False(t, IsRoutingCandidateTTFTHealthy("default", "gpt-5.5", slowAccount))
	require.True(t, IsRoutingCandidateTTFTHealthy("default", "gpt-5.5", fastAccount))
}

func TestRoutingCandidateTTFTMovesSelectionToHealthyLowerLayer(t *testing.T) {
	clearRoutingTTFTHealthCacheForTest(t)
	clearRoutingHealthCacheForTest(t)
	t.Setenv(routingTTFTMinSamplesEnv, "3")

	high := &model.RoutingCandidate{
		ChannelID:    1,
		Kind:         model.RoutingCredentialKindSingleKey,
		Schedule:     model.NewRoutingSchedule(10, 100, 0, 0),
		PolicyDomain: "channel:1:single_key",
	}
	low := &model.RoutingCandidate{
		ChannelID:    2,
		Kind:         model.RoutingCredentialKindSingleKey,
		Schedule:     model.NewRoutingSchedule(0, 1, 0, 0),
		PolicyDomain: "channel:2:single_key",
	}
	recordSlowTTFTSamples(t, "default", "gpt-5.5", high)

	selected := selectRoutingCandidateByStrategy("default", "gpt-5.5", []*model.RoutingCandidate{high, low})
	require.Same(t, low, selected)
}

func TestRoutingCandidateTTFTKeepsFallbackWhenAllCandidatesAreSlow(t *testing.T) {
	clearRoutingTTFTHealthCacheForTest(t)
	clearRoutingHealthCacheForTest(t)
	t.Setenv(routingTTFTMinSamplesEnv, "3")

	high := &model.RoutingCandidate{
		ChannelID:    1,
		Kind:         model.RoutingCredentialKindSingleKey,
		Schedule:     model.NewRoutingSchedule(10, 100, 0, 0),
		PolicyDomain: "channel:1:single_key",
	}
	low := &model.RoutingCandidate{
		ChannelID:    2,
		Kind:         model.RoutingCredentialKindSingleKey,
		Schedule:     model.NewRoutingSchedule(0, 1, 0, 0),
		PolicyDomain: "channel:2:single_key",
	}
	recordSlowTTFTSamples(t, "default", "gpt-5.5", high)
	recordSlowTTFTSamples(t, "default", "gpt-5.5", low)

	selected := selectRoutingCandidateByStrategy("default", "gpt-5.5", []*model.RoutingCandidate{high, low})
	require.Same(t, high, selected)
}

func TestRoutingCandidateTTFTUsesDistinctMultiKeyIndexes(t *testing.T) {
	clearRoutingTTFTHealthCacheForTest(t)
	t.Setenv(routingTTFTMinSamplesEnv, "3")

	slowKey := &model.RoutingCandidate{
		ChannelID:     5,
		Kind:          model.RoutingCredentialKindMultiKey,
		MultiKeyIndex: 0,
	}
	otherKey := &model.RoutingCandidate{
		ChannelID:     5,
		Kind:          model.RoutingCredentialKindMultiKey,
		MultiKeyIndex: 1,
	}
	recordSlowTTFTSamples(t, "default", "gpt-5.5", slowKey)

	require.False(t, IsRoutingCandidateTTFTHealthy("default", "gpt-5.5", slowKey))
	require.True(t, IsRoutingCandidateTTFTHealthy("default", "gpt-5.5", otherKey))
}

func TestRoutingTTFTConfigRejectsInvalidValues(t *testing.T) {
	clearRoutingTTFTHealthCacheForTest(t)
	t.Setenv(routingTTFTThresholdEnv, "-1")
	t.Setenv(routingTTFTCooldownEnv, "0")
	t.Setenv(routingTTFTMinSamplesEnv, "-1")

	threshold, cooldown, minSamples := routingTTFTConfig()
	require.EqualValues(t, routingTTFTThresholdDefaultMs, threshold)
	require.Equal(t, time.Duration(routingTTFTCooldownDefaultSeconds)*time.Second, cooldown)
	require.Equal(t, routingTTFTMinSamplesDefault, minSamples)
}
