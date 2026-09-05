// channel_affinity.go 实现了渠道亲和性（Channel Affinity）功能。
// 渠道亲和性允许根据请求的特征（如模型、路径、用户代理、请求体中的字段等）
// 将特定请求路由到固定的渠道，以利用上游的缓存机制（如 prompt cache）降低延迟和成本。
//
// 核心机制：
// 1. 规则匹配：根据模型正则、路径正则、用户代理等条件匹配请求
// 2. 亲和值提取：从请求上下文或请求体中提取亲和值（如 prompt_cache_key）
// 3. 缓存查找：在 Redis/内存混合缓存中查找之前绑定的渠道 ID
// 4. 绑定记录：请求成功后将渠道 ID 缓存起来，后续相同亲和值的请求会路由到同一渠道
// 5. 模板覆盖：支持为匹配规则的请求附加参数覆盖模板
// 6. 使用统计：跟踪缓存命中率和 token 使用情况
package service

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/pkg/cachex"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
	"github.com/tidwall/gjson"
)

const (
	ginKeyChannelAffinityCacheKey    = "channel_affinity_cache_key"             // Gin 上下文键：缓存键
	ginKeyChannelAffinityCacheSuffix = "channel_affinity_cache_key_suffix"      // Gin 上下文键：不含命名空间的缓存键后缀
	ginKeyChannelAffinityTTLSeconds  = "channel_affinity_ttl_seconds"           // Gin 上下文键：TTL 秒数
	ginKeyChannelAffinityMeta        = "channel_affinity_meta"                  // Gin 上下文键：亲和性元数据
	ginKeyChannelAffinityLogInfo     = "channel_affinity_log_info"              // Gin 上下文键：日志信息
	ginKeyChannelAffinitySkipRetry   = "channel_affinity_skip_retry_on_failure" // Gin 上下文键：失败时跳过重试

	channelAffinityLegacyCacheNamespace     = "nexustok:channel_affinity:v2"                   // 旧版整型亲和缓存命名空间，仅用于运维清理
	channelAffinityCacheNamespace           = "nexustok:channel_affinity:v3"                   // 渠道亲和性结构体缓存的 Redis 命名空间
	channelAffinityUsageCacheStatsNamespace = "nexustok:channel_affinity_usage_cache_stats:v2" // 使用统计缓存的 Redis 命名空间
)

var (
	channelAffinityCacheOnce sync.Once                                   // 确保新版缓存只初始化一次
	channelAffinityCache     *cachex.HybridCache[channelAffinityBinding] // 渠道亲和性混合缓存（Redis + 内存）

	channelAffinityLegacyCacheOnce sync.Once                // 确保旧版缓存只初始化一次
	channelAffinityLegacyCache     *cachex.HybridCache[int] // 旧版整型亲和缓存，仅用于清理 v2 遗留键

	channelAffinityUsageCacheStatsOnce  sync.Once                                              // 确保使用统计缓存只初始化一次
	channelAffinityUsageCacheStatsCache *cachex.HybridCache[ChannelAffinityUsageCacheCounters] // 使用统计混合缓存

	channelAffinityRegexCache sync.Map // 正则表达式缓存，避免重复编译
)

// channelAffinityBinding 是新版亲和缓存值。
// 只记录非敏感 channel_id 与“上一次成功请求回写时间”，请求开始时用它判断亲和
// 绑定是否仍处于连续请求窗口；失败请求不会写入这里，因此不会续期窗口。
type channelAffinityBinding struct {
	ChannelID     int   `json:"channel_id"`
	LastSuccessAt int64 `json:"last_success_at"`
}

// channelAffinityMeta 存储渠道亲和性的匹配元数据。
// 在请求处理过程中存储在 Gin 上下文中，用于后续的缓存写入和日志记录。
type channelAffinityMeta struct {
	CacheKey                  string                 // 完整的缓存键
	CacheKeySuffix            string                 // 不含命名空间的缓存键后缀，便于新旧命名空间同时清理
	TTLSeconds                int                    // 缓存过期时间（秒）
	RuleName                  string                 // 匹配的规则名称
	SkipRetry                 bool                   // 失败时是否跳过重试
	Used                      bool                   // 本次请求是否实际被新鲜亲和绑定约束
	Bypassed                  bool                   // 本次请求是否因窗口过期等原因绕过亲和限制
	BypassReason              string                 // 绕过原因，如 cache_miss/stale_bypassed
	BindingChannelID          int                    // 缓存中记录的渠道 ID
	LastSuccessAt             int64                  // 缓存中记录的上一次成功请求时间
	RequestIntervalSeconds    int64                  // 当前请求距离上一次成功回写的秒数
	MaxRequestIntervalSeconds int                    // 当前使用的亲和新鲜度窗口秒数
	ParamTemplate             map[string]interface{} // 参数覆盖模板
	KeySourceType             string                 // 亲和值来源类型（gjson/context_int/context_string）
	KeySourceKey              string                 // 亲和值来源键（上下文键名）
	KeySourcePath             string                 // 亲和值来源路径（gjson 路径）
	KeyHint                   string                 // 亲和值的简短提示（用于日志）
	KeyFingerprint            string                 // 亲和值的指纹（SHA1 前 8 位）
	UsingGroup                string                 // 使用的分组标识
	ModelName                 string                 // 请求的模型名称
	RequestPath               string                 // 请求路径
}

// ChannelAffinityStatsContext 存储渠道亲和性的统计上下文。
// 用于在请求处理过程中传递统计所需的信息。
type ChannelAffinityStatsContext struct {
	RuleName       string // 规则名称
	UsingGroup     string // 使用的分组
	KeyFingerprint string // 亲和值指纹
	TTLSeconds     int64  // TTL 秒数
}

// 缓存 token 比率模式常量，用于区分不同上游的缓存命中率计算方式
const (
	cacheTokenRateModeCachedOverPrompt           = "cached_over_prompt"             // OpenAI 模式：cached / prompt
	cacheTokenRateModeCachedOverPromptPlusCached = "cached_over_prompt_plus_cached" // Claude 模式：cached / (prompt + cached)
	cacheTokenRateModeMixed                      = "mixed"                          // 混合模式：同时有 OpenAI 和 Claude 格式
)

// ChannelAffinityCacheStats 表示渠道亲和性缓存的统计信息。
// 用于管理界面展示缓存状态和按规则分类的统计。
type ChannelAffinityCacheStats struct {
	Enabled       bool           `json:"enabled"`        // 渠道亲和性功能是否启用
	Total         int            `json:"total"`          // 缓存中的总条目数
	Unknown       int            `json:"unknown"`        // 无法归类到已知规则的条目数
	ByRuleName    map[string]int `json:"by_rule_name"`   // 按规则名分类的条目数
	CacheCapacity int            `json:"cache_capacity"` // 缓存容量
	CacheAlgo     string         `json:"cache_algo"`     // 缓存淘汰算法（如 LRU）
}

// getChannelAffinityCache 获取新版渠道亲和性混合缓存实例（懒初始化）。
// 使用 Redis 作为持久化后端，内存 LRU 作为热缓存；缓存值包含渠道 ID 与
// 上一次成功请求时间，确保亲和窗口按成功请求续期，而不是按失败请求或读取续期。
func getChannelAffinityCache() *cachex.HybridCache[channelAffinityBinding] {
	channelAffinityCacheOnce.Do(func() {
		setting := operation_setting.GetChannelAffinitySetting()
		capacity := setting.MaxEntries
		if capacity <= 0 {
			capacity = 100_000
		}
		defaultTTLSeconds := setting.DefaultTTLSeconds
		if defaultTTLSeconds <= 0 {
			defaultTTLSeconds = 3600
		}

		channelAffinityCache = cachex.NewHybridCache[channelAffinityBinding](cachex.HybridCacheConfig[channelAffinityBinding]{
			Namespace: cachex.Namespace(channelAffinityCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[channelAffinityBinding]{},
			Memory: func() *hot.HotCache[string, channelAffinityBinding] {
				return hot.NewHotCache[string, channelAffinityBinding](hot.LRU, capacity).
					WithTTL(time.Duration(defaultTTLSeconds) * time.Second).
					WithJanitor().
					Build()
			},
		})
	})
	return channelAffinityCache
}

// getChannelAffinityLegacyCache 获取旧版整型亲和缓存实例。
// 新路由逻辑不会读取 v2 整型值，避免旧 Redis 数据被误判为新鲜绑定；保留该实例
// 仅用于清理当前请求、按规则清理和全量清理，降低升级后的运维困惑。
func getChannelAffinityLegacyCache() *cachex.HybridCache[int] {
	channelAffinityLegacyCacheOnce.Do(func() {
		setting := operation_setting.GetChannelAffinitySetting()
		capacity := 100_000
		defaultTTLSeconds := 3600
		if setting != nil {
			if setting.MaxEntries > 0 {
				capacity = setting.MaxEntries
			}
			if setting.DefaultTTLSeconds > 0 {
				defaultTTLSeconds = setting.DefaultTTLSeconds
			}
		}

		channelAffinityLegacyCache = cachex.NewHybridCache[int](cachex.HybridCacheConfig[int]{
			Namespace: cachex.Namespace(channelAffinityLegacyCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.IntCodec{},
			Memory: func() *hot.HotCache[string, int] {
				return hot.NewHotCache[string, int](hot.LRU, capacity).
					WithTTL(time.Duration(defaultTTLSeconds) * time.Second).
					WithJanitor().
					Build()
			},
		})
	})
	return channelAffinityLegacyCache
}

// GetChannelAffinityCacheStats 获取渠道亲和性缓存的统计信息。
// 遍历缓存中的所有键，按规则名分类统计。
func GetChannelAffinityCacheStats() ChannelAffinityCacheStats {
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil {
		return ChannelAffinityCacheStats{
			Enabled:    false,
			Total:      0,
			Unknown:    0,
			ByRuleName: map[string]int{},
		}
	}

	cache := getChannelAffinityCache()
	mainCap, _ := cache.Capacity()
	mainAlgo, _ := cache.Algorithm()

	rules := setting.Rules
	ruleByName := make(map[string]operation_setting.ChannelAffinityRule, len(rules))
	for _, r := range rules {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}
		if !r.IncludeRuleName {
			continue
		}
		ruleByName[name] = r
	}

	byRuleName := make(map[string]int, len(ruleByName))
	for name := range ruleByName {
		byRuleName[name] = 0
	}

	keys, err := cache.Keys()
	if err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache list keys failed: err=%v", err))
		keys = nil
	}
	total := len(keys)
	unknown := 0
	for _, k := range keys {
		prefix := channelAffinityCacheNamespace + ":"
		if !strings.HasPrefix(k, prefix) {
			unknown++
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		parts := strings.Split(rest, ":")
		if len(parts) < 2 {
			unknown++
			continue
		}
		ruleName := parts[0]
		rule, ok := ruleByName[ruleName]
		if !ok {
			unknown++
			continue
		}
		if rule.IncludeModelName {
			if len(parts) < 3 {
				unknown++
				continue
			}
		}
		if rule.IncludeUsingGroup {
			minParts := 3
			if rule.IncludeModelName {
				minParts = 4
			}
			if len(parts) < minParts {
				unknown++
				continue
			}
		}
		byRuleName[ruleName]++
	}

	return ChannelAffinityCacheStats{
		Enabled:       setting.Enabled,
		Total:         total,
		Unknown:       unknown,
		ByRuleName:    byRuleName,
		CacheCapacity: mainCap,
		CacheAlgo:     mainAlgo,
	}
}

// ClearChannelAffinityCacheAll 清空所有渠道亲和性缓存。
// 返回被删除的缓存条目数。
func ClearChannelAffinityCacheAll() int {
	deleted := 0
	cache := getChannelAffinityCache()
	keys, err := cache.Keys()
	if err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache list keys failed: err=%v", err))
		keys = nil
	}
	if len(keys) > 0 {
		if res, err := cache.DeleteMany(keys); err != nil {
			common.SysError(fmt.Sprintf("channel affinity cache delete many failed: err=%v", err))
		} else {
			deleted += countDeletedCacheKeys(res)
		}
	}

	legacyCache := getChannelAffinityLegacyCache()
	legacyKeys, legacyErr := legacyCache.Keys()
	if legacyErr != nil {
		common.SysError(fmt.Sprintf("legacy channel affinity cache list keys failed: err=%v", legacyErr))
		legacyKeys = nil
	}
	if len(legacyKeys) > 0 {
		if res, err := legacyCache.DeleteMany(legacyKeys); err != nil {
			common.SysError(fmt.Sprintf("legacy channel affinity cache delete many failed: err=%v", err))
		} else {
			deleted += countDeletedCacheKeys(res)
		}
	}
	return deleted
}

// ClearChannelAffinityCacheByRuleName 按规则名清空渠道亲和性缓存。
// 只有启用了 include_rule_name 的规则才能按规则名清空。
//
// 参数：
//   - ruleName: 规则名称
//
// 返回：
//   - int: 被删除的缓存条目数
//   - error: 规则未找到或不支持按规则名清空时返回错误
func ClearChannelAffinityCacheByRuleName(ruleName string) (int, error) {
	ruleName = strings.TrimSpace(ruleName)
	if ruleName == "" {
		return 0, fmt.Errorf("rule_name 不能为空")
	}

	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil {
		return 0, fmt.Errorf("channel_affinity_setting 未初始化")
	}

	var matchedRule *operation_setting.ChannelAffinityRule
	for i := range setting.Rules {
		r := &setting.Rules[i]
		if strings.TrimSpace(r.Name) != ruleName {
			continue
		}
		matchedRule = r
		break
	}
	if matchedRule == nil {
		return 0, fmt.Errorf("未知规则名称")
	}
	if !matchedRule.IncludeRuleName {
		return 0, fmt.Errorf("该规则未启用 include_rule_name，无法按规则清空缓存")
	}

	deleted := 0
	cache := getChannelAffinityCache()
	currentDeleted, err := cache.DeleteByPrefix(ruleName)
	if err != nil {
		return 0, err
	}
	deleted += currentDeleted

	legacyDeleted, legacyErr := getChannelAffinityLegacyCache().DeleteByPrefix(ruleName)
	if legacyErr != nil {
		return deleted, legacyErr
	}
	deleted += legacyDeleted
	return deleted, nil
}

func countDeletedCacheKeys(deleted map[string]bool) int {
	count := 0
	for _, ok := range deleted {
		if ok {
			count++
		}
	}
	return count
}

// matchAnyRegexCached 使用缓存的正则表达式匹配字符串。
// 编译后的正则表达式存储在 sync.Map 中，避免重复编译。
func matchAnyRegexCached(patterns []string, s string) bool {
	if len(patterns) == 0 || s == "" {
		return false
	}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		re, ok := channelAffinityRegexCache.Load(pattern)
		if !ok {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			re = compiled
			channelAffinityRegexCache.Store(pattern, re)
		}
		if re.(*regexp.Regexp).MatchString(s) {
			return true
		}
	}
	return false
}

// matchAnyIncludeFold 不区分大小写的包含匹配。
// 检查字符串 s 是否包含模式列表中的任一子串。
func matchAnyIncludeFold(patterns []string, s string) bool {
	if len(patterns) == 0 || s == "" {
		return false
	}
	sLower := strings.ToLower(s)
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(sLower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// extractChannelAffinityValue 从请求上下文中提取亲和值。
// 支持四种来源类型：
// - context_int: 从 Gin 上下文中获取整数值
// - context_string: 从 Gin 上下文中获取字符串值
// - request_header: 从 HTTP 请求头中获取字符串值
// - gjson: 从请求体 JSON 中使用 gjson 路径提取值
func extractChannelAffinityValue(c *gin.Context, src operation_setting.ChannelAffinityKeySource) string {
	switch src.Type {
	case "context_int":
		if src.Key == "" {
			return ""
		}
		v := c.GetInt(src.Key)
		if v <= 0 {
			return ""
		}
		return strconv.Itoa(v)
	case "context_string":
		if src.Key == "" {
			return ""
		}
		return strings.TrimSpace(c.GetString(src.Key))
	case "request_header":
		if c == nil || c.Request == nil || src.Key == "" {
			return ""
		}
		return strings.TrimSpace(c.GetHeader(src.Key))
	case "gjson":
		if src.Path == "" {
			return ""
		}
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return ""
		}
		body, err := storage.Bytes()
		if err != nil || len(body) == 0 {
			return ""
		}
		res := gjson.GetBytes(body, src.Path)
		if !res.Exists() {
			return ""
		}
		switch res.Type {
		case gjson.String, gjson.Number, gjson.True, gjson.False:
			return strings.TrimSpace(res.String())
		default:
			return strings.TrimSpace(res.Raw)
		}
	default:
		return ""
	}
}

// buildChannelAffinityCacheKeySuffix 构建缓存键后缀。
// 根据规则配置，可选包含规则名、模型名、分组名和亲和值。
func buildChannelAffinityCacheKeySuffix(rule operation_setting.ChannelAffinityRule, modelName string, usingGroup string, affinityValue string) string {
	parts := make([]string, 0, 4)
	if rule.IncludeRuleName && rule.Name != "" {
		parts = append(parts, rule.Name)
	}
	if rule.IncludeModelName && modelName != "" {
		parts = append(parts, modelName)
	}
	if rule.IncludeUsingGroup && usingGroup != "" {
		parts = append(parts, usingGroup)
	}
	parts = append(parts, affinityValue)
	return strings.Join(parts, ":")
}

// setChannelAffinityContext 将渠道亲和性元数据存储到 Gin 上下文中。
func setChannelAffinityContext(c *gin.Context, meta channelAffinityMeta) {
	if c == nil {
		return
	}
	c.Set(ginKeyChannelAffinityCacheKey, meta.CacheKey)
	c.Set(ginKeyChannelAffinityCacheSuffix, meta.CacheKeySuffix)
	c.Set(ginKeyChannelAffinityTTLSeconds, meta.TTLSeconds)
	c.Set(ginKeyChannelAffinityMeta, meta)
}

// getChannelAffinityContext 从 Gin 上下文中获取渠道亲和性的缓存键和 TTL。
func getChannelAffinityContext(c *gin.Context) (string, string, int, bool) {
	if c == nil {
		return "", "", 0, false
	}
	keyAny, ok := c.Get(ginKeyChannelAffinityCacheKey)
	if !ok {
		return "", "", 0, false
	}
	key, ok := keyAny.(string)
	if !ok || key == "" {
		return "", "", 0, false
	}
	suffix := ""
	if suffixAny, ok := c.Get(ginKeyChannelAffinityCacheSuffix); ok {
		suffix, _ = suffixAny.(string)
	}
	if suffix == "" {
		suffix = strings.TrimPrefix(key, channelAffinityCacheNamespace+":")
	}
	ttlAny, ok := c.Get(ginKeyChannelAffinityTTLSeconds)
	if !ok {
		return key, suffix, 0, true
	}
	ttlSeconds, _ := ttlAny.(int)
	return key, suffix, ttlSeconds, true
}

// getChannelAffinityMeta 从 Gin 上下文中获取渠道亲和性元数据。
func getChannelAffinityMeta(c *gin.Context) (channelAffinityMeta, bool) {
	anyMeta, ok := c.Get(ginKeyChannelAffinityMeta)
	if !ok {
		return channelAffinityMeta{}, false
	}
	meta, ok := anyMeta.(channelAffinityMeta)
	if !ok {
		return channelAffinityMeta{}, false
	}
	return meta, true
}

// GetChannelAffinityStatsContext 从 Gin 上下文中获取渠道亲和性统计上下文。
// 用于后续的使用缓存统计记录。
func GetChannelAffinityStatsContext(c *gin.Context) (ChannelAffinityStatsContext, bool) {
	if c == nil {
		return ChannelAffinityStatsContext{}, false
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok {
		return ChannelAffinityStatsContext{}, false
	}
	ruleName := strings.TrimSpace(meta.RuleName)
	keyFp := strings.TrimSpace(meta.KeyFingerprint)
	usingGroup := strings.TrimSpace(meta.UsingGroup)
	if ruleName == "" || keyFp == "" {
		return ChannelAffinityStatsContext{}, false
	}
	ttlSeconds := int64(meta.TTLSeconds)
	if ttlSeconds <= 0 {
		return ChannelAffinityStatsContext{}, false
	}
	return ChannelAffinityStatsContext{
		RuleName:       ruleName,
		UsingGroup:     usingGroup,
		KeyFingerprint: keyFp,
		TTLSeconds:     ttlSeconds,
	}, true
}

// affinityFingerprint 计算亲和值的指纹。
// 使用 SHA1 哈希并取前 8 个十六进制字符，用于日志和统计中的简短标识。
func affinityFingerprint(s string) string {
	if s == "" {
		return ""
	}
	hex := common.Sha1([]byte(s))
	if len(hex) >= 8 {
		return hex[:8]
	}
	return hex
}

// buildChannelAffinityKeyHint 构建亲和值的简短提示（用于日志）。
// 长度超过 12 字符时截断为 "前4...后4" 格式。
func buildChannelAffinityKeyHint(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) <= 12 {
		return s
	}
	return s[:4] + "..." + s[len(s)-4:]
}

// channelAffinityRequestStartTimestamp 返回本次请求开始的 Unix 秒时间。
//
// Channel Affinity 的连续请求窗口必须从请求进入分发链路时开始计算，而不是从
// 选择到候选渠道的中间时刻开始计算。前者由 Distribute 预先写入 observed 时间，
// 对于单元测试或复用 PrepareRelayChannelContext 的调用方，则依次回退到请求开始
// 时间和当前时间，保证未接入标准中间件链路时仍可正常工作。
func channelAffinityRequestStartTimestamp(c *gin.Context) int64 {
	if observedStart := common.GetContextKeyTime(c, constant.ContextKeyRequestObservedStartTime); !observedStart.IsZero() {
		return observedStart.Unix()
	}
	if requestStart := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime); !requestStart.IsZero() {
		return requestStart.Unix()
	}
	return common.GetTimestamp()
}

// cloneStringAnyMap 深拷贝 map[string]interface{}。
// 避免修改原始 map 导致的副作用。
func cloneStringAnyMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return map[string]interface{}{}
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// mergeChannelOverride 合并渠道参数覆盖模板。
// 基础参数优先，模板中的新参数会被添加。
// operations 字段特殊处理：模板操作追加到基础操作列表之前。
func mergeChannelOverride(base map[string]interface{}, tpl map[string]interface{}) map[string]interface{} {
	if len(base) == 0 && len(tpl) == 0 {
		return map[string]interface{}{}
	}
	if len(tpl) == 0 {
		return base
	}
	out := cloneStringAnyMap(base)
	for k, v := range tpl {
		if strings.EqualFold(strings.TrimSpace(k), "operations") {
			baseOps, hasBaseOps := extractParamOperations(out[k])
			tplOps, hasTplOps := extractParamOperations(v)
			if hasTplOps {
				if hasBaseOps {
					out[k] = append(tplOps, baseOps...)
				} else {
					out[k] = tplOps
				}
				continue
			}
		}
		if _, exists := out[k]; exists {
			continue
		}
		out[k] = v
	}
	return out
}

// extractParamOperations 从值中提取 operations 列表。
// 支持 []interface{} 和 []map[string]interface{} 两种类型。
func extractParamOperations(value interface{}) ([]interface{}, bool) {
	switch ops := value.(type) {
	case []interface{}:
		if len(ops) == 0 {
			return []interface{}{}, true
		}
		cloned := make([]interface{}, 0, len(ops))
		cloned = append(cloned, ops...)
		return cloned, true
	case []map[string]interface{}:
		cloned := make([]interface{}, 0, len(ops))
		for _, op := range ops {
			cloned = append(cloned, op)
		}
		return cloned, true
	default:
		return nil, false
	}
}

// updateChannelAffinityMeta 更新 Gin 上下文中的亲和元数据。
// GetPreferredChannelByAffinity 会先写入规则匹配信息，再根据缓存是否命中、是否过期
// 补充运行态字段；后续模板覆盖、失败重试判断和日志都读取同一份元数据。
func updateChannelAffinityMeta(c *gin.Context, meta channelAffinityMeta) {
	if c == nil {
		return
	}
	setChannelAffinityContext(c, meta)
	c.Set(ginKeyChannelAffinityLogInfo, channelAffinityAdminInfo(meta, "", 0))
}

// channelAffinityAdminInfo 构造可写入日志的非敏感亲和信息。
// 这里不会记录原始亲和值，只写入短提示和 SHA1 指纹，便于排查同一亲和值的窗口命中
// 与绕过原因，同时避免日志中出现可恢复的用户请求内容。
func channelAffinityAdminInfo(meta channelAffinityMeta, selectedGroup string, channelID int) map[string]interface{} {
	info := map[string]interface{}{
		"reason":                       meta.RuleName,
		"rule_name":                    meta.RuleName,
		"using_group":                  meta.UsingGroup,
		"model":                        meta.ModelName,
		"request_path":                 meta.RequestPath,
		"key_source":                   meta.KeySourceType,
		"key_key":                      meta.KeySourceKey,
		"key_path":                     meta.KeySourcePath,
		"key_hint":                     meta.KeyHint,
		"key_fp":                       meta.KeyFingerprint,
		"used":                         meta.Used,
		"bypassed":                     meta.Bypassed,
		"max_request_interval_seconds": meta.MaxRequestIntervalSeconds,
	}
	if selectedGroup != "" {
		info["selected_group"] = selectedGroup
	}
	if channelID > 0 {
		info["channel_id"] = channelID
	}
	if meta.BindingChannelID > 0 {
		info["binding_channel_id"] = meta.BindingChannelID
	}
	if meta.BypassReason != "" {
		info["bypass_reason"] = meta.BypassReason
	}
	if meta.LastSuccessAt > 0 {
		info["last_success_at"] = meta.LastSuccessAt
	}
	if meta.RequestIntervalSeconds >= 0 {
		info["request_interval_seconds"] = meta.RequestIntervalSeconds
	}
	return info
}

// appendChannelAffinityTemplateAdminInfo 将模板覆盖的管理信息追加到日志中。
// 记录模板是否已应用、规则名、覆盖的参数数量等信息。
func appendChannelAffinityTemplateAdminInfo(c *gin.Context, meta channelAffinityMeta) {
	if c == nil {
		return
	}
	if len(meta.ParamTemplate) == 0 {
		return
	}

	templateInfo := map[string]interface{}{
		"applied":             true,
		"rule_name":           meta.RuleName,
		"param_override_keys": len(meta.ParamTemplate),
	}
	if anyInfo, ok := c.Get(ginKeyChannelAffinityLogInfo); ok {
		if info, ok := anyInfo.(map[string]interface{}); ok {
			info["override_template"] = templateInfo
			c.Set(ginKeyChannelAffinityLogInfo, info)
			return
		}
	}
	info := channelAffinityAdminInfo(meta, "", 0)
	info["override_template"] = templateInfo
	c.Set(ginKeyChannelAffinityLogInfo, info)
}

// ApplyChannelAffinityOverrideTemplate 将渠道亲和性规则的参数覆盖模板合并到渠道参数中。
// 基础参数优先，模板中的新参数会被添加。合并后记录管理信息到日志。
//
// 参数：
//   - c: Gin 请求上下文
//   - paramOverride: 基础渠道参数覆盖
//
// 返回：
//   - map[string]interface{}: 合并后的参数覆盖
//   - bool: 是否成功应用了模板
func ApplyChannelAffinityOverrideTemplate(c *gin.Context, paramOverride map[string]interface{}) (map[string]interface{}, bool) {
	if c == nil {
		return paramOverride, false
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok {
		return paramOverride, false
	}
	if len(meta.ParamTemplate) == 0 {
		return paramOverride, false
	}

	mergedParam := mergeChannelOverride(paramOverride, meta.ParamTemplate)
	appendChannelAffinityTemplateAdminInfo(c, meta)
	return mergedParam, true
}

// GetPreferredChannelByAffinity 根据渠道亲和性规则查找优选的渠道 ID。
// 这是渠道亲和性的核心入口函数。
// 遍历所有配置的规则，按顺序匹配：
// 1. 模型正则匹配
// 2. 路径正则匹配
// 3. 用户代理包含匹配
// 4. 亲和值提取和验证
// 5. 缓存查找
// 匹配成功后将元数据存储到 Gin 上下文中。
//
// 参数：
//   - c: Gin 请求上下文
//   - modelName: 请求的模型名称
//   - usingGroup: 使用的分组标识
//
// 返回：
//   - int: 优选的渠道 ID（0 表示未找到）
//   - bool: 是否找到优选渠道
func GetPreferredChannelByAffinity(c *gin.Context, modelName string, usingGroup string) (int, bool) {
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil || !setting.Enabled {
		return 0, false
	}
	path := ""
	if c != nil && c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	userAgent := ""
	if c != nil && c.Request != nil {
		userAgent = c.Request.UserAgent()
	}

	for _, rule := range setting.Rules {
		if !matchAnyRegexCached(rule.ModelRegex, modelName) {
			continue
		}
		if len(rule.PathRegex) > 0 && !matchAnyRegexCached(rule.PathRegex, path) {
			continue
		}
		if len(rule.UserAgentInclude) > 0 && !matchAnyIncludeFold(rule.UserAgentInclude, userAgent) {
			continue
		}
		var affinityValue string
		var usedSource operation_setting.ChannelAffinityKeySource
		for _, src := range rule.KeySources {
			affinityValue = extractChannelAffinityValue(c, src)
			if affinityValue != "" {
				usedSource = src
				break
			}
		}
		if affinityValue == "" {
			continue
		}
		if rule.ValueRegex != "" && !matchAnyRegexCached([]string{rule.ValueRegex}, affinityValue) {
			continue
		}

		ttlSeconds := rule.TTLSeconds
		if ttlSeconds <= 0 {
			ttlSeconds = setting.DefaultTTLSeconds
		}
		maxRequestIntervalSeconds := setting.NormalizedMaxRequestIntervalSeconds()
		cacheKeySuffix := buildChannelAffinityCacheKeySuffix(rule, modelName, usingGroup, affinityValue)
		cacheKeyFull := channelAffinityCacheNamespace + ":" + cacheKeySuffix
		meta := channelAffinityMeta{
			CacheKey:                  cacheKeyFull,
			CacheKeySuffix:            cacheKeySuffix,
			TTLSeconds:                ttlSeconds,
			RuleName:                  rule.Name,
			SkipRetry:                 rule.SkipRetryOnFailure,
			RequestIntervalSeconds:    -1,
			MaxRequestIntervalSeconds: maxRequestIntervalSeconds,
			ParamTemplate:             cloneStringAnyMap(rule.ParamOverrideTemplate),
			KeySourceType:             strings.TrimSpace(usedSource.Type),
			KeySourceKey:              strings.TrimSpace(usedSource.Key),
			KeySourcePath:             strings.TrimSpace(usedSource.Path),
			KeyHint:                   buildChannelAffinityKeyHint(affinityValue),
			KeyFingerprint:            affinityFingerprint(affinityValue),
			UsingGroup:                usingGroup,
			ModelName:                 modelName,
			RequestPath:               path,
		}
		setChannelAffinityContext(c, meta)

		cache := getChannelAffinityCache()
		binding, found, err := cache.Get(cacheKeySuffix)
		if err != nil {
			common.SysError(fmt.Sprintf("channel affinity cache get failed: key=%s, err=%v", cacheKeyFull, err))
			meta.Bypassed = true
			meta.BypassReason = "cache_error"
			updateChannelAffinityMeta(c, meta)
			return 0, false
		}
		if !found {
			meta.Bypassed = true
			meta.BypassReason = "cache_miss"
			updateChannelAffinityMeta(c, meta)
			return 0, false
		}

		meta.BindingChannelID = binding.ChannelID
		meta.LastSuccessAt = binding.LastSuccessAt
		now := channelAffinityRequestStartTimestamp(c)
		if binding.LastSuccessAt > 0 {
			meta.RequestIntervalSeconds = now - binding.LastSuccessAt
			if meta.RequestIntervalSeconds < 0 {
				meta.RequestIntervalSeconds = 0
			}
		}
		if binding.ChannelID <= 0 || binding.LastSuccessAt <= 0 {
			meta.Bypassed = true
			meta.BypassReason = "invalid_binding"
			updateChannelAffinityMeta(c, meta)
			return 0, false
		}
		if meta.RequestIntervalSeconds >= int64(maxRequestIntervalSeconds) {
			meta.Bypassed = true
			meta.BypassReason = "stale_bypassed"
			updateChannelAffinityMeta(c, meta)
			return 0, false
		}

		// 此处只说明存在新鲜绑定；真正完成亲和渠道选择后才由
		// MarkChannelAffinityUsed 标记 used=true，避免渠道不可用时误触发
		// skip_retry_on_failure 或记录成“已使用亲和”。
		meta.Used = false
		meta.Bypassed = false
		meta.BypassReason = ""
		updateChannelAffinityMeta(c, meta)
		return binding.ChannelID, true
	}
	return 0, false
}

// ShouldSkipRetryAfterChannelAffinityFailure 判断渠道亲和性匹配失败后是否跳过重试。
// skip_retry_on_failure 只在本次请求被“新鲜亲和绑定”实际约束到某个渠道时生效；
// 缓存缺失、窗口过期或仅命中参数模板的请求都应继续走普通全渠道重试。
func ShouldSkipRetryAfterChannelAffinityFailure(c *gin.Context) bool {
	if c == nil {
		return false
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok || !meta.SkipRetry || !meta.Used || meta.Bypassed {
		return false
	}
	v, ok := c.Get(ginKeyChannelAffinitySkipRetry)
	if ok {
		b, ok := v.(bool)
		if ok {
			return b
		}
	}
	return true
}

// AllowChannelAffinityDegradationRetry 允许本次亲和命中在真实上游失败后继续降级重试。
//
// skip_retry_on_failure 的原始用途是避免缓存亲和请求在失败时盲目重放；但当一次
// 请求已经命中了具体 channel/key，并且后续错误处理会把失败候选加入请求级排除集时，
// 继续阻断重试会导致坏密钥直接中断用户对话。这里仅清除本次请求上下文中的跳过
// 重试标记，不修改管理员规则本身，也不删除亲和缓存；如果后续重试成功且开启了
// SwitchOnSuccess，成功渠道会按既有逻辑回写为新的亲和目标。
func AllowChannelAffinityDegradationRetry(c *gin.Context) {
	if c == nil {
		return
	}
	if _, ok := getChannelAffinityMeta(c); !ok {
		return
	}
	c.Set(ginKeyChannelAffinitySkipRetry, false)
	if anyInfo, ok := c.Get(ginKeyChannelAffinityLogInfo); ok {
		if info, ok := anyInfo.(map[string]interface{}); ok {
			info["degradation_retry"] = true
			c.Set(ginKeyChannelAffinityLogInfo, info)
		}
	}
}

// ClearCurrentChannelAffinityCache 清理当前请求命中的渠道亲和缓存。
//
// 当缓存命中的渠道已禁用、已删除或不再支持当前分组/模型时，继续保留该缓存会让后续
// 请求反复命中过期渠道。该函数只清理本次请求上下文中的缓存键，并清除跳过重试标记，
// 让本次之后的请求可以重新选择健康渠道。
func ClearCurrentChannelAffinityCache(c *gin.Context) bool {
	if c == nil {
		return false
	}
	cacheKey, cacheKeySuffix, _, ok := getChannelAffinityContext(c)
	if !ok || (cacheKey == "" && cacheKeySuffix == "") {
		return false
	}

	cache := getChannelAffinityCache()
	deleted, err := cache.DeleteMany(nonEmptyStrings(cacheKeySuffix, cacheKey))
	if err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache delete current failed: err=%v", err))
		return false
	}
	legacyDeleted, legacyErr := getChannelAffinityLegacyCache().DeleteMany(nonEmptyStrings(cacheKeySuffix, cacheKey))
	if legacyErr != nil {
		common.SysError(fmt.Sprintf("legacy channel affinity cache delete current failed: err=%v", legacyErr))
	}
	if meta, ok := getChannelAffinityMeta(c); ok {
		meta.Used = false
		meta.Bypassed = true
		meta.BypassReason = "affinity_channel_unusable"
		updateChannelAffinityMeta(c, meta)
	}
	c.Set(ginKeyChannelAffinitySkipRetry, false)
	for _, ok := range deleted {
		if ok {
			return true
		}
	}
	for _, ok := range legacyDeleted {
		if ok {
			return true
		}
	}
	return false
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// ShouldKeepChannelAffinityOnChannelDisabled 判断亲和渠道不可用时是否保留旧缓存。
//
// 默认 false，与 new-api-main 对齐：失效缓存会被清理，后续请求重新选择渠道。
// 管理员显式开启 keep_on_channel_disabled 后才保留旧缓存，适合等待短暂维护恢复的场景。
func ShouldKeepChannelAffinityOnChannelDisabled() bool {
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil {
		return false
	}
	return setting.KeepOnChannelDisabled
}

// MarkChannelAffinityUsed 标记渠道亲和性已被使用。
// 将选中的渠道信息记录到 Gin 上下文中，用于日志和管理界面展示。
func MarkChannelAffinityUsed(c *gin.Context, selectedGroup string, channelID int) {
	if c == nil || channelID <= 0 {
		return
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok {
		return
	}
	meta.Used = true
	meta.Bypassed = false
	meta.BypassReason = ""
	updateChannelAffinityMeta(c, meta)
	c.Set(ginKeyChannelAffinitySkipRetry, meta.SkipRetry)
	c.Set(ginKeyChannelAffinityLogInfo, channelAffinityAdminInfo(meta, selectedGroup, channelID))
}

// AppendChannelAffinityAdminInfo 将渠道亲和性的管理信息追加到 adminInfo 中。
// 用于管理界面展示渠道亲和性的匹配详情。
func AppendChannelAffinityAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil || adminInfo == nil {
		return
	}
	anyInfo, ok := c.Get(ginKeyChannelAffinityLogInfo)
	if !ok || anyInfo == nil {
		return
	}
	adminInfo["channel_affinity"] = anyInfo
}

// RecordChannelAffinity 记录渠道亲和性绑定。
// 在请求成功后调用，将当前请求绑定的渠道 ID 写入缓存，
// 使得后续相同亲和值的请求能路由到同一渠道。
// 如果启用了 SwitchOnSuccess，使用实际成功的渠道 ID 而非初始选择的。
func RecordChannelAffinity(c *gin.Context, channelID int) {
	if channelID <= 0 {
		return
	}
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil || !setting.Enabled {
		return
	}
	if setting.SwitchOnSuccess && c != nil {
		if successChannelID := c.GetInt("channel_id"); successChannelID > 0 {
			channelID = successChannelID
		}
	}
	cacheKey, cacheKeySuffix, ttlSeconds, ok := getChannelAffinityContext(c)
	if !ok {
		return
	}
	if ttlSeconds <= 0 {
		ttlSeconds = setting.DefaultTTLSeconds
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}
	cache := getChannelAffinityCache()
	key := cacheKeySuffix
	if key == "" {
		key = cacheKey
	}
	binding := channelAffinityBinding{
		ChannelID:     channelID,
		LastSuccessAt: common.GetTimestamp(),
	}
	if err := cache.SetWithTTL(key, binding, time.Duration(ttlSeconds)*time.Second); err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache set failed: key=%s, err=%v", cacheKey, err))
	}
}

// ChannelAffinityUsageCacheStats 表示渠道亲和性使用缓存的统计信息。
// 跟踪缓存命中率、token 使用量等指标。
type ChannelAffinityUsageCacheStats struct {
	RuleName            string `json:"rule_name"`              // 规则名称
	UsingGroup          string `json:"using_group"`            // 使用的分组
	KeyFingerprint      string `json:"key_fp"`                 // 亲和值指纹
	CachedTokenRateMode string `json:"cached_token_rate_mode"` // 缓存 token 比率模式

	Hit           int64 `json:"hit"`            // 缓存命中次数
	Total         int64 `json:"total"`          // 总请求次数
	WindowSeconds int64 `json:"window_seconds"` // 统计窗口时间（秒）

	PromptTokens         int64 `json:"prompt_tokens"`           // prompt token 总数
	CompletionTokens     int64 `json:"completion_tokens"`       // completion token 总数
	TotalTokens          int64 `json:"total_tokens"`            // token 总数
	CachedTokens         int64 `json:"cached_tokens"`           // 缓存 token 总数
	PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"` // prompt 缓存命中 token 总数
	LastSeenAt           int64 `json:"last_seen_at"`            // 最后一次见到的时间戳
}

// ChannelAffinityUsageCacheCounters 表示使用缓存的内部计数器。
// 用于在缓存中累加统计数据。
type ChannelAffinityUsageCacheCounters struct {
	CachedTokenRateMode string `json:"cached_token_rate_mode"` // 缓存 token 比率模式

	Hit           int64 `json:"hit"`            // 缓存命中次数
	Total         int64 `json:"total"`          // 总请求次数
	WindowSeconds int64 `json:"window_seconds"` // 统计窗口时间（秒）

	PromptTokens         int64 `json:"prompt_tokens"`           // prompt token 总数
	CompletionTokens     int64 `json:"completion_tokens"`       // completion token 总数
	TotalTokens          int64 `json:"total_tokens"`            // token 总数
	CachedTokens         int64 `json:"cached_tokens"`           // 缓存 token 总数
	PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"` // prompt 缓存命中 token 总数
	LastSeenAt           int64 `json:"last_seen_at"`            // 最后一次见到的时间戳
}

// channelAffinityUsageCacheStatsLocks 是分段锁数组，用于减少使用统计更新时的锁竞争。
var channelAffinityUsageCacheStatsLocks [64]sync.Mutex

// ObserveChannelAffinityUsageCacheByRelayFormat 根据 relay 格式记录使用缓存统计。
// 根据 relay 格式自动确定缓存 token 比率模式。
func ObserveChannelAffinityUsageCacheByRelayFormat(c *gin.Context, usage *dto.Usage, relayFormat types.RelayFormat) {
	ObserveChannelAffinityUsageCacheFromContext(c, usage, cachedTokenRateModeByRelayFormat(relayFormat))
}

// ObserveChannelAffinityUsageCacheFromContext 从 Gin 上下文中提取统计上下文并记录使用缓存统计。
func ObserveChannelAffinityUsageCacheFromContext(c *gin.Context, usage *dto.Usage, cachedTokenRateMode string) {
	statsCtx, ok := GetChannelAffinityStatsContext(c)
	if !ok {
		return
	}
	observeChannelAffinityUsageCache(statsCtx, usage, cachedTokenRateMode)
}

// GetChannelAffinityUsageCacheStats 获取指定规则/分组/指纹的使用缓存统计信息。
func GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFp string) ChannelAffinityUsageCacheStats {
	ruleName = strings.TrimSpace(ruleName)
	usingGroup = strings.TrimSpace(usingGroup)
	keyFp = strings.TrimSpace(keyFp)

	entryKey := channelAffinityUsageCacheEntryKey(ruleName, usingGroup, keyFp)
	if entryKey == "" {
		return ChannelAffinityUsageCacheStats{
			RuleName:       ruleName,
			UsingGroup:     usingGroup,
			KeyFingerprint: keyFp,
		}
	}

	cache := getChannelAffinityUsageCacheStatsCache()
	v, found, err := cache.Get(entryKey)
	if err != nil || !found {
		return ChannelAffinityUsageCacheStats{
			RuleName:       ruleName,
			UsingGroup:     usingGroup,
			KeyFingerprint: keyFp,
		}
	}
	return ChannelAffinityUsageCacheStats{
		CachedTokenRateMode:  v.CachedTokenRateMode,
		RuleName:             ruleName,
		UsingGroup:           usingGroup,
		KeyFingerprint:       keyFp,
		Hit:                  v.Hit,
		Total:                v.Total,
		WindowSeconds:        v.WindowSeconds,
		PromptTokens:         v.PromptTokens,
		CompletionTokens:     v.CompletionTokens,
		TotalTokens:          v.TotalTokens,
		CachedTokens:         v.CachedTokens,
		PromptCacheHitTokens: v.PromptCacheHitTokens,
		LastSeenAt:           v.LastSeenAt,
	}
}

// observeChannelAffinityUsageCache 底层使用缓存统计记录实现。
// 使用分段锁减少并发竞争，原子累加各项统计指标。
func observeChannelAffinityUsageCache(statsCtx ChannelAffinityStatsContext, usage *dto.Usage, cachedTokenRateMode string) {
	entryKey := channelAffinityUsageCacheEntryKey(statsCtx.RuleName, statsCtx.UsingGroup, statsCtx.KeyFingerprint)
	if entryKey == "" {
		return
	}

	windowSeconds := statsCtx.TTLSeconds
	if windowSeconds <= 0 {
		return
	}

	cache := getChannelAffinityUsageCacheStatsCache()
	ttl := time.Duration(windowSeconds) * time.Second

	lock := channelAffinityUsageCacheStatsLock(entryKey)
	lock.Lock()
	defer lock.Unlock()

	prev, found, err := cache.Get(entryKey)
	if err != nil {
		return
	}
	next := prev
	if !found {
		next = ChannelAffinityUsageCacheCounters{}
	}
	currentMode := normalizeCachedTokenRateMode(cachedTokenRateMode)
	if currentMode != "" {
		if next.CachedTokenRateMode == "" {
			next.CachedTokenRateMode = currentMode
		} else if next.CachedTokenRateMode != currentMode && next.CachedTokenRateMode != cacheTokenRateModeMixed {
			next.CachedTokenRateMode = cacheTokenRateModeMixed
		}
	}
	next.Total++
	hit, cachedTokens, promptCacheHitTokens := usageCacheSignals(usage)
	if hit {
		next.Hit++
	}
	next.WindowSeconds = windowSeconds
	next.LastSeenAt = time.Now().Unix()
	next.CachedTokens += cachedTokens
	next.PromptCacheHitTokens += promptCacheHitTokens
	next.PromptTokens += int64(usagePromptTokens(usage))
	next.CompletionTokens += int64(usageCompletionTokens(usage))
	next.TotalTokens += int64(usageTotalTokens(usage))
	_ = cache.SetWithTTL(entryKey, next, ttl)
}

// normalizeCachedTokenRateMode 规范化缓存 token 比率模式。
// 只接受已知的三种模式，其他值返回空字符串。
func normalizeCachedTokenRateMode(mode string) string {
	switch mode {
	case cacheTokenRateModeCachedOverPrompt:
		return cacheTokenRateModeCachedOverPrompt
	case cacheTokenRateModeCachedOverPromptPlusCached:
		return cacheTokenRateModeCachedOverPromptPlusCached
	case cacheTokenRateModeMixed:
		return cacheTokenRateModeMixed
	default:
		return ""
	}
}

// cachedTokenRateModeByRelayFormat 根据 relay 格式确定缓存 token 比率模式。
// OpenAI 格式使用 cachedOverPrompt，Claude 格式使用 cachedOverPromptPlusCached。
func cachedTokenRateModeByRelayFormat(relayFormat types.RelayFormat) string {
	switch relayFormat {
	case types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIResponsesCompaction:
		return cacheTokenRateModeCachedOverPrompt
	case types.RelayFormatClaude:
		return cacheTokenRateModeCachedOverPromptPlusCached
	default:
		return ""
	}
}

// channelAffinityUsageCacheEntryKey 构建使用缓存的条目键。
// 格式：{ruleName}\n{usingGroup}\n{keyFp}
func channelAffinityUsageCacheEntryKey(ruleName, usingGroup, keyFp string) string {
	ruleName = strings.TrimSpace(ruleName)
	usingGroup = strings.TrimSpace(usingGroup)
	keyFp = strings.TrimSpace(keyFp)
	if ruleName == "" || keyFp == "" {
		return ""
	}
	return ruleName + "\n" + usingGroup + "\n" + keyFp
}

// usageCacheSignals 从 Usage 中提取缓存相关的信号。
// 返回是否命中缓存、缓存 token 数和 prompt 缓存命中 token 数。
func usageCacheSignals(usage *dto.Usage) (hit bool, cachedTokens int64, promptCacheHitTokens int64) {
	if usage == nil {
		return false, 0, 0
	}

	cached := int64(0)
	if usage.PromptTokensDetails.CachedTokens > 0 {
		cached = int64(usage.PromptTokensDetails.CachedTokens)
	} else if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
		cached = int64(usage.InputTokensDetails.CachedTokens)
	}
	pcht := int64(0)
	if usage.PromptCacheHitTokens > 0 {
		pcht = int64(usage.PromptCacheHitTokens)
	}
	return cached > 0 || pcht > 0, cached, pcht
}

// usagePromptTokens 从 Usage 中获取 prompt token 数。
// 兼容 OpenAI 格式（PromptTokens）和 Claude 格式（InputTokens）。
func usagePromptTokens(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	if usage.PromptTokens > 0 {
		return usage.PromptTokens
	}
	return usage.InputTokens
}

// usageCompletionTokens 从 Usage 中获取 completion token 数。
// 兼容 OpenAI 格式（CompletionTokens）和 Claude 格式（OutputTokens）。
func usageCompletionTokens(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	if usage.CompletionTokens > 0 {
		return usage.CompletionTokens
	}
	return usage.OutputTokens
}

// usageTotalTokens 从 Usage 中获取 token 总数。
// 如果 TotalTokens 为 0，则通过 prompt + completion 计算。
func usageTotalTokens(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	pt := usagePromptTokens(usage)
	ct := usageCompletionTokens(usage)
	if pt > 0 || ct > 0 {
		return pt + ct
	}
	return 0
}

// getChannelAffinityUsageCacheStatsCache 获取使用统计混合缓存实例（懒初始化）。
func getChannelAffinityUsageCacheStatsCache() *cachex.HybridCache[ChannelAffinityUsageCacheCounters] {
	channelAffinityUsageCacheStatsOnce.Do(func() {
		setting := operation_setting.GetChannelAffinitySetting()
		capacity := 100_000
		defaultTTLSeconds := 3600
		if setting != nil {
			if setting.MaxEntries > 0 {
				capacity = setting.MaxEntries
			}
			if setting.DefaultTTLSeconds > 0 {
				defaultTTLSeconds = setting.DefaultTTLSeconds
			}
		}

		channelAffinityUsageCacheStatsCache = cachex.NewHybridCache[ChannelAffinityUsageCacheCounters](cachex.HybridCacheConfig[ChannelAffinityUsageCacheCounters]{
			Namespace: cachex.Namespace(channelAffinityUsageCacheStatsNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[ChannelAffinityUsageCacheCounters]{},
			Memory: func() *hot.HotCache[string, ChannelAffinityUsageCacheCounters] {
				return hot.NewHotCache[string, ChannelAffinityUsageCacheCounters](hot.LRU, capacity).
					WithTTL(time.Duration(defaultTTLSeconds) * time.Second).
					WithJanitor().
					Build()
			},
		})
	})
	return channelAffinityUsageCacheStatsCache
}

// channelAffinityUsageCacheStatsLock 根据键获取分段锁。
// 使用 FNV-1a 哈希将键映射到 64 个锁中的一个，减少锁竞争。
func channelAffinityUsageCacheStatsLock(key string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	idx := h.Sum32() % uint32(len(channelAffinityUsageCacheStatsLocks))
	return &channelAffinityUsageCacheStatsLocks[idx]
}
