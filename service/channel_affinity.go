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
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/pkg/cachex"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
	"github.com/tidwall/gjson"
)

const (
	ginKeyChannelAffinityCacheKey   = "channel_affinity_cache_key"           // Gin 上下文键：缓存键
	ginKeyChannelAffinityTTLSeconds = "channel_affinity_ttl_seconds"         // Gin 上下文键：TTL 秒数
	ginKeyChannelAffinityMeta       = "channel_affinity_meta"                // Gin 上下文键：亲和性元数据
	ginKeyChannelAffinityLogInfo    = "channel_affinity_log_info"            // Gin 上下文键：日志信息
	ginKeyChannelAffinitySkipRetry  = "channel_affinity_skip_retry_on_failure" // Gin 上下文键：失败时跳过重试

	channelAffinityCacheNamespace           = "nexustok:channel_affinity:v2"                        // 渠道亲和性缓存的 Redis 命名空间
	channelAffinityUsageCacheStatsNamespace = "nexustok:channel_affinity_usage_cache_stats:v2"      // 使用统计缓存的 Redis 命名空间
)

var (
	channelAffinityCacheOnce sync.Once                              // 确保缓存只初始化一次
	channelAffinityCache     *cachex.HybridCache[int]               // 渠道亲和性混合缓存（Redis + 内存）

	channelAffinityUsageCacheStatsOnce  sync.Once                                              // 确保使用统计缓存只初始化一次
	channelAffinityUsageCacheStatsCache *cachex.HybridCache[ChannelAffinityUsageCacheCounters] // 使用统计混合缓存

	channelAffinityRegexCache sync.Map // 正则表达式缓存，避免重复编译
)

// channelAffinityMeta 存储渠道亲和性的匹配元数据。
// 在请求处理过程中存储在 Gin 上下文中，用于后续的缓存写入和日志记录。
type channelAffinityMeta struct {
	CacheKey       string                 // 完整的缓存键
	TTLSeconds     int                    // 缓存过期时间（秒）
	RuleName       string                 // 匹配的规则名称
	SkipRetry      bool                   // 失败时是否跳过重试
	ParamTemplate  map[string]interface{} // 参数覆盖模板
	KeySourceType  string                 // 亲和值来源类型（gjson/context_int/context_string）
	KeySourceKey   string                 // 亲和值来源键（上下文键名）
	KeySourcePath  string                 // 亲和值来源路径（gjson 路径）
	KeyHint        string                 // 亲和值的简短提示（用于日志）
	KeyFingerprint string                 // 亲和值的指纹（SHA1 前 8 位）
	UsingGroup     string                 // 使用的分组标识
	ModelName      string                 // 请求的模型名称
	RequestPath    string                 // 请求路径
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
	cacheTokenRateModeCachedOverPrompt           = "cached_over_prompt"            // OpenAI 模式：cached / prompt
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

// getChannelAffinityCache 获取渠道亲和性混合缓存实例（懒初始化）。
// 使用 Redis 作为持久化后端，内存 LRU 作为热缓存。
func getChannelAffinityCache() *cachex.HybridCache[int] {
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

		channelAffinityCache = cachex.NewHybridCache[int](cachex.HybridCacheConfig[int]{
			Namespace: cachex.Namespace(channelAffinityCacheNamespace),
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
	return channelAffinityCache
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
	cache := getChannelAffinityCache()
	keys, err := cache.Keys()
	if err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache list keys failed: err=%v", err))
		keys = nil
	}
	if len(keys) > 0 {
		if _, err := cache.DeleteMany(keys); err != nil {
			common.SysError(fmt.Sprintf("channel affinity cache delete many failed: err=%v", err))
		}
	}
	return len(keys)
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

	cache := getChannelAffinityCache()
	deleted, err := cache.DeleteByPrefix(ruleName)
	if err != nil {
		return 0, err
	}
	return deleted, nil
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
// 支持三种来源类型：
// - context_int: 从 Gin 上下文中获取整数值
// - context_string: 从 Gin 上下文中获取字符串值
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
	c.Set(ginKeyChannelAffinityCacheKey, meta.CacheKey)
	c.Set(ginKeyChannelAffinityTTLSeconds, meta.TTLSeconds)
	c.Set(ginKeyChannelAffinityMeta, meta)
}

// getChannelAffinityContext 从 Gin 上下文中获取渠道亲和性的缓存键和 TTL。
func getChannelAffinityContext(c *gin.Context) (string, int, bool) {
	keyAny, ok := c.Get(ginKeyChannelAffinityCacheKey)
	if !ok {
		return "", 0, false
	}
	key, ok := keyAny.(string)
	if !ok || key == "" {
		return "", 0, false
	}
	ttlAny, ok := c.Get(ginKeyChannelAffinityTTLSeconds)
	if !ok {
		return key, 0, true
	}
	ttlSeconds, _ := ttlAny.(int)
	return key, ttlSeconds, true
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
	c.Set(ginKeyChannelAffinityLogInfo, map[string]interface{}{
		"reason":            meta.RuleName,
		"rule_name":         meta.RuleName,
		"using_group":       meta.UsingGroup,
		"model":             meta.ModelName,
		"request_path":      meta.RequestPath,
		"key_source":        meta.KeySourceType,
		"key_key":           meta.KeySourceKey,
		"key_path":          meta.KeySourcePath,
		"key_hint":          meta.KeyHint,
		"key_fp":            meta.KeyFingerprint,
		"override_template": templateInfo,
	})
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
		cacheKeySuffix := buildChannelAffinityCacheKeySuffix(rule, modelName, usingGroup, affinityValue)
		cacheKeyFull := channelAffinityCacheNamespace + ":" + cacheKeySuffix
		setChannelAffinityContext(c, channelAffinityMeta{
			CacheKey:       cacheKeyFull,
			TTLSeconds:     ttlSeconds,
			RuleName:       rule.Name,
			SkipRetry:      rule.SkipRetryOnFailure,
			ParamTemplate:  cloneStringAnyMap(rule.ParamOverrideTemplate),
			KeySourceType:  strings.TrimSpace(usedSource.Type),
			KeySourceKey:   strings.TrimSpace(usedSource.Key),
			KeySourcePath:  strings.TrimSpace(usedSource.Path),
			KeyHint:        buildChannelAffinityKeyHint(affinityValue),
			KeyFingerprint: affinityFingerprint(affinityValue),
			UsingGroup:     usingGroup,
			ModelName:      modelName,
			RequestPath:    path,
		})

		cache := getChannelAffinityCache()
		channelID, found, err := cache.Get(cacheKeySuffix)
		if err != nil {
			common.SysError(fmt.Sprintf("channel affinity cache get failed: key=%s, err=%v", cacheKeyFull, err))
			return 0, false
		}
		if found {
			return channelID, true
		}
		return 0, false
	}
	return 0, false
}

// ShouldSkipRetryAfterChannelAffinityFailure 判断渠道亲和性匹配失败后是否跳过重试。
// 优先使用上下文中的显式标记，其次使用规则元数据中的 SkipRetry 标志。
func ShouldSkipRetryAfterChannelAffinityFailure(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(ginKeyChannelAffinitySkipRetry)
	if ok {
		b, ok := v.(bool)
		if ok {
			return b
		}
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok {
		return false
	}
	return meta.SkipRetry
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
	c.Set(ginKeyChannelAffinitySkipRetry, meta.SkipRetry)
	info := map[string]interface{}{
		"reason":         meta.RuleName,
		"rule_name":      meta.RuleName,
		"using_group":    meta.UsingGroup,
		"selected_group": selectedGroup,
		"model":          meta.ModelName,
		"request_path":   meta.RequestPath,
		"channel_id":     channelID,
		"key_source":     meta.KeySourceType,
		"key_key":        meta.KeySourceKey,
		"key_path":       meta.KeySourcePath,
		"key_hint":       meta.KeyHint,
		"key_fp":         meta.KeyFingerprint,
	}
	c.Set(ginKeyChannelAffinityLogInfo, info)
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
	cacheKey, ttlSeconds, ok := getChannelAffinityContext(c)
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
	if err := cache.SetWithTTL(cacheKey, channelID, time.Duration(ttlSeconds)*time.Second); err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache set failed: key=%s, err=%v", cacheKey, err))
	}
}

// ChannelAffinityUsageCacheStats 表示渠道亲和性使用缓存的统计信息。
// 跟踪缓存命中率、token 使用量等指标。
type ChannelAffinityUsageCacheStats struct {
	RuleName            string `json:"rule_name"`             // 规则名称
	UsingGroup          string `json:"using_group"`           // 使用的分组
	KeyFingerprint      string `json:"key_fp"`                // 亲和值指纹
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
