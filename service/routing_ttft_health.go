package service

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/pkg/cachex"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/samber/hot"
)

const (
	routingTTFTHealthNamespace = "nexustok:routing_ttft_health:v1"
	routingTTFTHealthCacheTTL  = 30 * time.Minute

	routingTTFTThresholdDefaultMs     = 800
	routingTTFTCooldownDefaultSeconds = 90
	routingTTFTMinSamplesDefault      = 2
	routingTTFTEWMAAlpha              = 0.35
	routingTTFTThresholdEnv           = "ROUTING_TTFT_THRESHOLD_MS"
	routingTTFTCooldownEnv            = "ROUTING_TTFT_COOLDOWN_SECONDS"
	routingTTFTMinSamplesEnv          = "ROUTING_TTFT_MIN_SAMPLES"
)

// RoutingCandidateTTFTState 记录一个分组、模型和具体凭证的首包延迟状态。
// 普通渠道使用 account_id=0；账号池和全局账号池使用实际账号 ID，
// 因而单个慢账号不会把同渠道的其它账号一起降级。
type RoutingCandidateTTFTState struct {
	SampleCount     int     `json:"sample_count"`
	EWMATTFTMs      float64 `json:"ewma_ttft_ms"`
	LastTTFTMs      int64   `json:"last_ttft_ms"`
	ConsecutiveSlow int     `json:"consecutive_slow"`
	CooldownUntil   int64   `json:"cooldown_until"`
	LastSampleAt    int64   `json:"last_sample_at"`
	RecentTTFTMs    []int64 `json:"recent_ttft_ms,omitempty"`
}

var (
	routingTTFTHealthOnce  sync.Once
	routingTTFTHealthCache *cachex.HybridCache[RoutingCandidateTTFTState]
	routingTTFTHealthMu    sync.Mutex
)

// getRoutingTTFTHealthCache 获取 TTFT 动态健康混合缓存。
func getRoutingTTFTHealthCache() *cachex.HybridCache[RoutingCandidateTTFTState] {
	routingTTFTHealthOnce.Do(func() {
		routingTTFTHealthCache = cachex.NewHybridCache[RoutingCandidateTTFTState](cachex.HybridCacheConfig[RoutingCandidateTTFTState]{
			Namespace:  cachex.Namespace(routingTTFTHealthNamespace),
			Redis:      common.RDB,
			RedisCodec: cachex.JSONCodec[RoutingCandidateTTFTState]{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, RoutingCandidateTTFTState] {
				return hot.NewHotCache[string, RoutingCandidateTTFTState](hot.LRU, 100_000).
					WithTTL(routingTTFTHealthCacheTTL).
					WithJanitor().
					Build()
			},
		})
	})
	return routingTTFTHealthCache
}

// routingTTFTConfig 读取 TTFT 动态降级参数。
// 系统 option 是主来源；旧环境变量仅在配置仍为默认值时作为兼容 fallback，
// 这样历史部署可以继续通过环境变量覆盖默认行为，而管理员保存后的自定义值优先。
func routingTTFTConfig() (thresholdMs int64, cooldown time.Duration, minSamples int) {
	setting := operation_setting.GetRoutingTTFTSetting()
	thresholdValue := setting.ThresholdMs
	cooldownSeconds := setting.CooldownSeconds
	minSamplesValue := setting.MinSamples
	if thresholdValue == routingTTFTThresholdDefaultMs {
		thresholdValue = common.GetEnvOrDefault(routingTTFTThresholdEnv, thresholdValue)
	}
	if cooldownSeconds == routingTTFTCooldownDefaultSeconds {
		cooldownSeconds = common.GetEnvOrDefault(routingTTFTCooldownEnv, cooldownSeconds)
	}
	if minSamplesValue == routingTTFTMinSamplesDefault {
		minSamplesValue = common.GetEnvOrDefault(routingTTFTMinSamplesEnv, minSamplesValue)
	}
	thresholdMs = int64(thresholdValue)
	minSamples = minSamplesValue
	if thresholdMs <= 0 {
		thresholdMs = routingTTFTThresholdDefaultMs
	}
	if cooldownSeconds <= 0 {
		cooldownSeconds = routingTTFTCooldownDefaultSeconds
	}
	if minSamples <= 0 {
		minSamples = routingTTFTMinSamplesDefault
	}
	cooldown = time.Duration(cooldownSeconds) * time.Second
	return
}

// routingCandidateTTFTAccountID 返回候选实际使用的账号 ID。
func routingCandidateTTFTAccountID(candidate *model.RoutingCandidate) int {
	if candidate == nil {
		return 0
	}
	if candidate.ChannelAccountID > 0 {
		return candidate.ChannelAccountID
	}
	if candidate.PoolAccountID > 0 {
		return candidate.PoolAccountID
	}
	return 0
}

// routingCandidateTTFTKey 构建包含凭证来源的哈希键，避免不同账号池 ID 发生碰撞。
func routingCandidateTTFTKey(group, modelName string, candidate *model.RoutingCandidate) string {
	if candidate == nil || group == "" || modelName == "" || candidate.ChannelID <= 0 {
		return ""
	}
	accountID := routingCandidateTTFTAccountID(candidate)
	hash := fnv.New64a()
	_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%d\x00%s\x00%d\x00%d\x00%d",
		group, modelName, candidate.ChannelID, candidate.Kind, accountID, candidate.MultiKeyIndex, candidate.PoolGroupID)
	return fmt.Sprintf("%x", hash.Sum64())
}

// getRoutingCandidateTTFTState 读取候选 TTFT 状态；缓存异常只视为没有历史样本。
func getRoutingCandidateTTFTState(group, modelName string, candidate *model.RoutingCandidate) RoutingCandidateTTFTState {
	key := routingCandidateTTFTKey(group, modelName, candidate)
	if key == "" {
		return RoutingCandidateTTFTState{}
	}
	state, found, err := getRoutingTTFTHealthCache().Get(key)
	if err != nil || !found {
		return RoutingCandidateTTFTState{}
	}
	return state
}

// IsRoutingCandidateTTFTHealthy 判断候选是否处于慢首包冷却期。
func IsRoutingCandidateTTFTHealthy(group, modelName string, candidate *model.RoutingCandidate) bool {
	if !operation_setting.GetRoutingTTFTSetting().Enabled {
		return true
	}
	state := getRoutingCandidateTTFTState(group, modelName, candidate)
	return state.CooldownUntil <= time.Now().Unix()
}

// RecordRoutingCandidateTTFT 记录一次成功请求的首包延迟。
// 没有有效首包、非流式请求或无法识别候选时不写入状态，避免把本地错误和
// 没有首包的失败请求误判成慢渠道。
func RecordRoutingCandidateTTFT(info *relaycommon.RelayInfo) {
	if info == nil || !info.IsStream || !info.HasSendResponse() || info.ChannelMeta == nil {
		return
	}
	ttftMs := info.FirstResponseTime.Sub(info.StartTime).Milliseconds()
	if ttftMs < 0 {
		return
	}
	group := info.UsingGroup
	if group == "" {
		group = info.TokenGroup
	}
	candidate := &model.RoutingCandidate{
		ChannelID:        info.ChannelId,
		MultiKeyIndex:    info.ChannelMultiKeyIndex,
		ChannelAccountID: info.ChannelAccountId,
		PoolGroupID:      info.PoolGroupId,
		PoolAccountID:    info.PoolAccountId,
		Kind:             routingCandidateKindFromRelayInfo(info),
	}
	RecordRoutingCandidateTTFTSample(group, info.OriginModelName, candidate, ttftMs)
}

// routingCandidateKindFromRelayInfo 将 RelayInfo 中的凭证上下文映射到缓存维度。
func routingCandidateKindFromRelayInfo(info *relaycommon.RelayInfo) model.RoutingCredentialKind {
	if info == nil || info.ChannelMeta == nil {
		return model.RoutingCredentialKindSingleKey
	}
	if info.PoolAccountId > 0 {
		return model.RoutingCredentialKindPoolAccount
	}
	if info.ChannelAccountId > 0 {
		return model.RoutingCredentialKindChannelAccount
	}
	if info.ChannelIsMultiKey {
		return model.RoutingCredentialKindMultiKey
	}
	return model.RoutingCredentialKindSingleKey
}

// RecordRoutingCandidateTTFTSample 写入指定候选的单次 TTFT 样本。
// 该函数独立于 RelayInfo，便于路由测试和后续从消费日志补录历史数据。
func RecordRoutingCandidateTTFTSample(group, modelName string, candidate *model.RoutingCandidate, ttftMs int64) {
	key := routingCandidateTTFTKey(group, modelName, candidate)
	if key == "" || ttftMs < 0 {
		return
	}

	thresholdMs, cooldown, minSamples := routingTTFTConfig()
	now := time.Now()
	routingTTFTHealthMu.Lock()
	defer routingTTFTHealthMu.Unlock()

	state, _, _ := getRoutingTTFTHealthCache().Get(key)
	state.SampleCount++
	state.LastTTFTMs = ttftMs
	state.LastSampleAt = now.Unix()
	if state.SampleCount == 1 || state.EWMATTFTMs <= 0 {
		state.EWMATTFTMs = float64(ttftMs)
	} else {
		state.EWMATTFTMs = routingTTFTEWMAAlpha*float64(ttftMs) +
			(1-routingTTFTEWMAAlpha)*state.EWMATTFTMs
	}
	if ttftMs >= thresholdMs {
		state.ConsecutiveSlow++
	} else {
		// 快首包表示当前连续状态恢复，后续仍会结合最近窗口的 P90 判断。
		state.ConsecutiveSlow = 0
	}
	state.RecentTTFTMs = append(state.RecentTTFTMs, ttftMs)
	if len(state.RecentTTFTMs) > 20 {
		state.RecentTTFTMs = state.RecentTTFTMs[len(state.RecentTTFTMs)-20:]
	}
	p90 := routingTTFTP90(state.RecentTTFTMs)
	if state.SampleCount >= minSamples && p90 >= thresholdMs {
		state.CooldownUntil = now.Add(cooldown).Unix()
	} else if p90 < thresholdMs {
		state.CooldownUntil = 0
	}
	_ = getRoutingTTFTHealthCache().SetWithTTL(key, state, routingTTFTHealthCacheTTL)
}

// routingTTFTP90 计算最近 TTFT 窗口的近似 P90。
// 样本量较小时向上取位，确保少量连续慢首包不会被平均值掩盖。
func routingTTFTP90(samples []int64) int64 {
	if len(samples) == 0 {
		return 0
	}
	sorted := append([]int64(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (len(sorted)*90+99)/100 - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
