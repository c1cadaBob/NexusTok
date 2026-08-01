// channel_routing_strategy.go 实现渠道智能切换和临时健康降级。
//
// 动态状态只存在 Redis/内存混合缓存中，不回写 Channel、Ability 或管理员配置。
// 这样可以让故障冷却快速生效，同时保证服务重启后只恢复静态配置，不携带旧的
// 临时故障痕迹。
package service

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/pkg/cachex"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
)

const (
	channelRoutingHealthNamespace = "nexustok:channel_routing_health:v1"
	channelRoutingHealthCacheTTL  = 10 * time.Minute
	channelRoutingInitialCooldown = 5 * time.Second
	channelRoutingMaxCooldown     = 5 * time.Minute
)

// ChannelRoutingHealthState 记录一个分组、模型和渠道组合的临时健康状态。
type ChannelRoutingHealthState struct {
	FailureCount  int   `json:"failure_count"`
	CooldownUntil int64 `json:"cooldown_until"`
	LastFailureAt int64 `json:"last_failure_at"`
	LastSuccessAt int64 `json:"last_success_at"`
}

var (
	channelRoutingHealthOnce  sync.Once
	channelRoutingHealthCache *cachex.HybridCache[ChannelRoutingHealthState]
	channelRoutingHealthMu    sync.Mutex
)

// getChannelRoutingHealthCache 获取渠道动态健康混合缓存。
func getChannelRoutingHealthCache() *cachex.HybridCache[ChannelRoutingHealthState] {
	channelRoutingHealthOnce.Do(func() {
		channelRoutingHealthCache = cachex.NewHybridCache[ChannelRoutingHealthState](cachex.HybridCacheConfig[ChannelRoutingHealthState]{
			Namespace:  cachex.Namespace(channelRoutingHealthNamespace),
			Redis:      common.RDB,
			RedisCodec: cachex.JSONCodec[ChannelRoutingHealthState]{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, ChannelRoutingHealthState] {
				return hot.NewHotCache[string, ChannelRoutingHealthState](hot.LRU, 100_000).
					WithTTL(channelRoutingHealthCacheTTL).
					WithJanitor().
					Build()
			},
		})
	})
	return channelRoutingHealthCache
}

// GetRoutingStrategyMode 返回经过白名单校验的当前策略。
func GetRoutingStrategyMode() string {
	switch common.RoutingStrategyMode {
	case "availability", "cost":
		return common.RoutingStrategyMode
	default:
		return "balanced"
	}
}

// channelRoutingHealthKey 构建不会受模型/分组特殊字符影响的缓存键。
func channelRoutingHealthKey(group, modelName string, channelID int) string {
	hash := fnv.New64a()
	_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%d", group, modelName, channelID)
	return fmt.Sprintf("%x", hash.Sum64())
}

// getChannelRoutingHealthState 读取动态健康状态；缓存故障不应阻断正常选路。
func getChannelRoutingHealthState(group, modelName string, channelID int) ChannelRoutingHealthState {
	if group == "" || modelName == "" || channelID <= 0 {
		return ChannelRoutingHealthState{}
	}
	state, found, err := getChannelRoutingHealthCache().Get(channelRoutingHealthKey(group, modelName, channelID))
	if err != nil || !found {
		return ChannelRoutingHealthState{}
	}
	return state
}

// IsChannelRoutingHealthy 判断渠道是否处于动态冷却期。
func IsChannelRoutingHealthy(group, modelName string, channelID int) bool {
	state := getChannelRoutingHealthState(group, modelName, channelID)
	return state.CooldownUntil <= time.Now().Unix()
}

// RecordChannelRoutingFailure 记录可用于自动切换的渠道级失败。
func RecordChannelRoutingFailure(group, modelName string, channelID int) {
	if group == "" || modelName == "" || channelID <= 0 {
		return
	}

	channelRoutingHealthMu.Lock()
	defer channelRoutingHealthMu.Unlock()

	key := channelRoutingHealthKey(group, modelName, channelID)
	state, _, _ := getChannelRoutingHealthCache().Get(key)
	state.FailureCount++
	state.LastFailureAt = time.Now().Unix()
	cooldown := channelRoutingInitialCooldown
	for i := 1; i < state.FailureCount; i++ {
		cooldown *= 3
		if cooldown >= channelRoutingMaxCooldown {
			cooldown = channelRoutingMaxCooldown
			break
		}
	}
	state.CooldownUntil = time.Now().Add(cooldown).Unix()
	_ = getChannelRoutingHealthCache().SetWithTTL(key, state, channelRoutingHealthCacheTTL)
}

// RecordChannelRoutingSuccess 衰减成功渠道的动态故障状态。
func RecordChannelRoutingSuccess(group, modelName string, channelID int) {
	if group == "" || modelName == "" || channelID <= 0 {
		return
	}

	channelRoutingHealthMu.Lock()
	defer channelRoutingHealthMu.Unlock()

	key := channelRoutingHealthKey(group, modelName, channelID)
	state := getChannelRoutingHealthState(group, modelName, channelID)
	state.LastSuccessAt = time.Now().Unix()
	if state.FailureCount <= 1 {
		_, _ = getChannelRoutingHealthCache().DeleteMany([]string{key})
		return
	}
	state.FailureCount /= 2
	state.CooldownUntil = 0
	_ = getChannelRoutingHealthCache().SetWithTTL(key, state, channelRoutingHealthCacheTTL)
}

// ShouldRecordChannelRoutingFailure 判断错误是否应触发动态降级。
//
// 参数错误、计费错误和本地转换错误不应污染渠道健康度；账号池单账号失败
// 仍由同渠道账号轮换处理，只有普通渠道级失败进入此缓存。
func ShouldRecordChannelRoutingFailure(c *gin.Context, err *types.NexusTokError) bool {
	if err == nil || types.IsSkipRetryError(err) {
		return false
	}
	if c != nil {
		if _, ok := c.Get("specific_channel_id"); ok {
			return false
		}
		if common.GetContextKeyBool(c, constant.ContextKeyChannelAccountPool) {
			return false
		}
	}
	if types.IsChannelError(err) {
		return true
	}
	if err.StatusCode == 0 || err.StatusCode < 100 || err.StatusCode > 599 {
		return true
	}
	return err.StatusCode == 408 || err.StatusCode == 429 || err.StatusCode >= 500
}

// RecordChannelRoutingSuccessFromContext 记录一次真实成功请求最终使用的渠道。
func RecordChannelRoutingSuccessFromContext(c *gin.Context, channel *model.Channel) {
	if c == nil || channel == nil {
		return
	}
	group := RoutingGroupFromContext(c)
	modelName := c.GetString("original_model")
	if modelName == "" {
		modelName = c.GetString("model")
	}
	RecordChannelRoutingSuccess(group, modelName, channel.Id)
}

// RecordChannelRoutingFailureIfEligible 记录符合条件的渠道级失败。
func RecordChannelRoutingFailureIfEligible(c *gin.Context, channel *model.Channel, modelName string, err *types.NexusTokError) {
	if c == nil || channel == nil || !ShouldRecordChannelRoutingFailure(c, err) {
		return
	}
	RecordChannelRoutingFailure(RoutingGroupFromContext(c), modelName, channel.Id)
}

// RecordChannelRoutingSetupFailure 记录 setup 阶段已经确认的渠道级不可用。
//
// 账号池在真正转发前可能因为没有任何可用账号返回 ChannelNoAvailableKey；
// 这时还没有进入 Relay 的上游错误处理路径，但渠道已经应当在本轮请求中降级。
// 该函数只记录会触发“排除当前渠道”的 setup 错误，不处理单账号业务失败。
func RecordChannelRoutingSetupFailure(c *gin.Context, channel *model.Channel, modelName string, err *types.NexusTokError) {
	if c == nil || channel == nil || err == nil {
		return
	}
	if err.GetErrorCode() != types.ErrorCodeChannelNoAvailableKey {
		return
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return
	}
	RecordChannelRoutingFailure(RoutingGroupFromContext(c), modelName, channel.Id)
}

// RoutingGroupFromContext 返回本次请求真正用于能力匹配的分组。
// auto 分组会在 ContextKeyAutoGroup 中保存最终选中的实际分组。
func RoutingGroupFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if group := common.GetContextKeyString(c, constant.ContextKeyAutoGroup); group != "" {
		return group
	}
	return common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
}

// SelectChannelByRoutingStrategy 按策略从原有候选列表中选择渠道。
//
// 候选列表已经由 model 层按 retry 保留了原有优先级语义。这里首先过滤
// 动态冷却渠道；若全部候选都在冷却期，则选择“最不坏”的最小失败次数/最早
// 冷却结束候选，避免临时状态把路由打空。
func SelectChannelByRoutingStrategy(group, modelName string, candidates []*model.Channel) *model.Channel {
	if len(candidates) == 0 {
		return nil
	}
	now := time.Now().Unix()
	healthy := make([]*model.Channel, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		state := getChannelRoutingHealthState(group, modelName, candidate.Id)
		if state.CooldownUntil <= now {
			healthy = append(healthy, candidate)
		}
	}
	if len(healthy) == 0 {
		healthy = leastDegradedCandidates(group, modelName, candidates)
	}
	if len(healthy) == 0 {
		return nil
	}

	switch GetRoutingStrategyMode() {
	case "availability":
		healthy = bestAvailabilityCandidates(group, modelName, healthy)
	case "cost":
		healthy = bestCostCandidates(healthy)
	default:
		healthy = bestPriorityCandidates(healthy)
	}
	return model.SelectChannelByWeight(healthy)
}

func leastDegradedCandidates(group, modelName string, candidates []*model.Channel) []*model.Channel {
	bestFailureCount := int(^uint(0) >> 1)
	bestCooldown := int64(^uint64(0) >> 1)
	var result []*model.Channel
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		state := getChannelRoutingHealthState(group, modelName, candidate.Id)
		if state.FailureCount < bestFailureCount ||
			(state.FailureCount == bestFailureCount && state.CooldownUntil < bestCooldown) {
			bestFailureCount = state.FailureCount
			bestCooldown = state.CooldownUntil
			result = []*model.Channel{candidate}
		} else if state.FailureCount == bestFailureCount && state.CooldownUntil == bestCooldown {
			result = append(result, candidate)
		}
	}
	return result
}

func bestAvailabilityCandidates(group, modelName string, candidates []*model.Channel) []*model.Channel {
	minFailureCount := int(^uint(0) >> 1)
	for _, candidate := range candidates {
		state := getChannelRoutingHealthState(group, modelName, candidate.Id)
		if state.FailureCount < minFailureCount {
			minFailureCount = state.FailureCount
		}
	}
	result := make([]*model.Channel, 0, len(candidates))
	for _, candidate := range candidates {
		state := getChannelRoutingHealthState(group, modelName, candidate.Id)
		if state.FailureCount == minFailureCount {
			result = append(result, candidate)
		}
	}
	return bestPriorityCandidates(result)
}

func bestPriorityCandidates(candidates []*model.Channel) []*model.Channel {
	if len(candidates) == 0 {
		return nil
	}
	bestPriority := int64(-1 << 63)
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if candidate.GetPriority() > bestPriority {
			bestPriority = candidate.GetPriority()
		}
	}
	result := make([]*model.Channel, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate != nil && candidate.GetPriority() == bestPriority {
			result = append(result, candidate)
		}
	}
	return result
}

func bestCostCandidates(candidates []*model.Channel) []*model.Channel {
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].GetPriority() != candidates[j].GetPriority() {
			return candidates[i].GetPriority() > candidates[j].GetPriority()
		}
		return candidates[i].GetWeight() > candidates[j].GetWeight()
	})
	bestPriority := candidates[0].GetPriority()
	bestWeight := candidates[0].GetWeight()
	result := make([]*model.Channel, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.GetPriority() == bestPriority && candidate.GetWeight() == bestWeight {
			result = append(result, candidate)
		}
	}
	return result
}
