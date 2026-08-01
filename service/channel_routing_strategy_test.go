package service

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func routingStrategyTestChannel(id int, priority int64, weight uint) *model.Channel {
	return &model.Channel{
		Id:       id,
		Status:   common.ChannelStatusEnabled,
		Priority: &priority,
		Weight:   &weight,
	}
}

func clearRoutingHealthCacheForTest(t *testing.T) {
	t.Helper()
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })
	if cache := getChannelRoutingHealthCache(); cache != nil {
		require.NoError(t, cache.Purge())
		t.Cleanup(func() { _ = cache.Purge() })
	}
}

func TestSelectChannelByRoutingStrategyAvoidsCoolingChannel(t *testing.T) {
	clearRoutingHealthCacheForTest(t)
	oldMode := common.RoutingStrategyMode
	common.RoutingStrategyMode = "balanced"
	t.Cleanup(func() { common.RoutingStrategyMode = oldMode })

	first := routingStrategyTestChannel(1, 10, 100)
	second := routingStrategyTestChannel(2, 10, 100)
	RecordChannelRoutingFailure("default", "gpt-health", first.Id)

	selected := SelectChannelByRoutingStrategy("default", "gpt-health", []*model.Channel{first, second})
	require.Same(t, second, selected)
	require.Equal(t, int64(10), *first.Priority)
	require.Equal(t, uint(100), *first.Weight)
}

func TestSelectChannelByRoutingStrategyFallsBackToHealthyLowerPriority(t *testing.T) {
	clearRoutingHealthCacheForTest(t)
	oldMode := common.RoutingStrategyMode
	common.RoutingStrategyMode = "balanced"
	t.Cleanup(func() { common.RoutingStrategyMode = oldMode })

	high := routingStrategyTestChannel(1, 100, 100)
	low := routingStrategyTestChannel(2, 1, 100)
	RecordChannelRoutingFailure("default", "gpt-fallback", high.Id)

	selected := SelectChannelByRoutingStrategy("default", "gpt-fallback", []*model.Channel{high, low})
	require.Same(t, low, selected)
}

func TestSelectChannelByRoutingStrategyKeepsBestPriorityAmongHealthyFallbacks(t *testing.T) {
	clearRoutingHealthCacheForTest(t)
	oldMode := common.RoutingStrategyMode
	common.RoutingStrategyMode = "balanced"
	t.Cleanup(func() { common.RoutingStrategyMode = oldMode })

	coolingHigh := routingStrategyTestChannel(1, 100, 100)
	healthyMiddle := routingStrategyTestChannel(2, 50, 1)
	healthyLow := routingStrategyTestChannel(3, 1, 1000)
	RecordChannelRoutingFailure("default", "gpt-priority", coolingHigh.Id)

	for i := 0; i < 20; i++ {
		selected := SelectChannelByRoutingStrategy("default", "gpt-priority", []*model.Channel{coolingHigh, healthyMiddle, healthyLow})
		require.Same(t, healthyMiddle, selected)
	}
}

func TestChannelRoutingSuccessClearsSingleFailure(t *testing.T) {
	clearRoutingHealthCacheForTest(t)
	RecordChannelRoutingFailure("default", "gpt-success", 7)
	require.False(t, IsChannelRoutingHealthy("default", "gpt-success", 7))

	RecordChannelRoutingSuccess("default", "gpt-success", 7)
	require.True(t, IsChannelRoutingHealthy("default", "gpt-success", 7))
}

func TestChannelRoutingCostAndAvailabilityModes(t *testing.T) {
	clearRoutingHealthCacheForTest(t)
	one := routingStrategyTestChannel(1, 10, 5)
	two := routingStrategyTestChannel(2, 10, 20)
	three := routingStrategyTestChannel(3, 10, 10)
	candidates := []*model.Channel{one, two, three}

	oldMode := common.RoutingStrategyMode
	t.Cleanup(func() { common.RoutingStrategyMode = oldMode })
	common.RoutingStrategyMode = "cost"
	for i := 0; i < 20; i++ {
		require.Same(t, two, SelectChannelByRoutingStrategy("default", "gpt-cost", candidates))
	}

	common.RoutingStrategyMode = "availability"
	RecordChannelRoutingFailure("default", "gpt-availability", one.Id)
	RecordChannelRoutingFailure("default", "gpt-availability", three.Id)
	require.Same(t, two, SelectChannelByRoutingStrategy("default", "gpt-availability", candidates))
}

func TestChannelRoutingHealthCacheExpires(t *testing.T) {
	clearRoutingHealthCacheForTest(t)
	key := channelRoutingHealthKey("default", "gpt-ttl", 11)
	require.NoError(t, getChannelRoutingHealthCache().SetWithTTL(key, ChannelRoutingHealthState{
		FailureCount:  1,
		CooldownUntil: time.Now().Add(time.Hour).Unix(),
	}, time.Millisecond))
	time.Sleep(20 * time.Millisecond)
	require.True(t, IsChannelRoutingHealthy("default", "gpt-ttl", 11))
}

func TestShouldRecordChannelRoutingFailureRespectsSpecificChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set("specific_channel_id", "1")
	err := types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	require.False(t, ShouldRecordChannelRoutingFailure(c, err))
}

func TestShouldRecordChannelRoutingFailureKeepsAccountPoolFallbackLocal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyChannelAccountPool, true)
	err := types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	require.False(t, ShouldRecordChannelRoutingFailure(c, err))
}
