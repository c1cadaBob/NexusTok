// 本文件测试渠道亲和性（Channel Affinity）的模板覆盖与重试跳过功能，包括：
// - 参数覆盖模板的合并逻辑（无模板时跳过、有模板时合并且基础参数优先）
// - operations 字段的合并（模板操作追加到基础操作列表之后）
// - 重试跳过判断逻辑（上下文标记或规则元数据中的 SkipRetry 标志）
// - Codex CLI 场景下的端到端模板传递（缓存命中、请求头透传、参数覆盖生效）
package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// buildChannelAffinityTemplateContextForTest 构建用于测试的 gin.Context，
// 并设置渠道亲和性元数据。
func buildChannelAffinityTemplateContextForTest(meta channelAffinityMeta) *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	setChannelAffinityContext(ctx, meta)
	return ctx
}

// TestApplyChannelAffinityOverrideTemplate_NoTemplate 测试无模板时的行为，
// 验证当规则没有配置 ParamTemplate 时，applied 为 false，基础参数原样返回。
func TestApplyChannelAffinityOverrideTemplate_NoTemplate(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-no-template",
	})
	base := map[string]interface{}{
		"temperature": 0.7,
	}

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, base)
	require.False(t, applied)
	require.Equal(t, base, merged)
}

// TestApplyChannelAffinityOverrideTemplate_MergeTemplate 测试模板参数合并逻辑，
// 验证：模板中的新参数（如 top_p）被添加；基础参数（如 temperature）保留原值不被覆盖；
// 合并后的日志信息正确记录到 gin.Context 中。
func TestApplyChannelAffinityOverrideTemplate_MergeTemplate(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-with-template",
		ParamTemplate: map[string]interface{}{
			"temperature": 0.2,
			"top_p":       0.95,
		},
		UsingGroup:     "default",
		ModelName:      "gpt-4.1",
		RequestPath:    "/v1/responses",
		KeySourceType:  "gjson",
		KeySourcePath:  "prompt_cache_key",
		KeyHint:        "abcd...wxyz",
		KeyFingerprint: "abcd1234",
	})
	base := map[string]interface{}{
		"temperature": 0.7,
		"max_tokens":  2000,
	}

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, base)
	require.True(t, applied)                     // 模板存在，应标记为已应用
	require.Equal(t, 0.7, merged["temperature"]) // 基础参数保持原值，不被模板覆盖
	require.Equal(t, 0.95, merged["top_p"])      // 模板中的新参数被正确添加
	require.Equal(t, 2000, merged["max_tokens"]) // 基础参数中的 max_tokens 保留
	require.Equal(t, 0.7, base["temperature"])   // 原始 base map 不被修改

	// 验证日志信息已正确写入 gin.Context
	anyInfo, ok := ctx.Get(ginKeyChannelAffinityLogInfo)
	require.True(t, ok)
	info, ok := anyInfo.(map[string]interface{})
	require.True(t, ok)
	overrideInfoAny, ok := info["override_template"]
	require.True(t, ok)
	overrideInfo, ok := overrideInfoAny.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, overrideInfo["applied"])
	require.Equal(t, "rule-with-template", overrideInfo["rule_name"])
	require.EqualValues(t, 2, overrideInfo["param_override_keys"])
}

// TestApplyChannelAffinityOverrideTemplate_MergeOperations 测试 operations 字段的合并逻辑，
// 验证模板中的 operations 会追加到基础 operations 列表之后（而非覆盖），
// 且合并顺序正确（模板操作在前，基础操作在后）。
func TestApplyChannelAffinityOverrideTemplate_MergeOperations(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-with-ops-template",
		ParamTemplate: map[string]interface{}{
			"operations": []map[string]interface{}{
				{
					"mode":  "pass_headers",
					"value": []string{"Originator"},
				},
			},
		},
	})
	base := map[string]interface{}{
		"temperature": 0.7,
		"operations": []map[string]interface{}{
			{
				"path":  "model",
				"mode":  "trim_prefix",
				"value": "openai/",
			},
		},
	}

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, base)
	require.True(t, applied)
	require.Equal(t, 0.7, merged["temperature"]) // 基础参数保持不变

	opsAny, ok := merged["operations"]
	require.True(t, ok)
	ops, ok := opsAny.([]interface{})
	require.True(t, ok)
	require.Len(t, ops, 2) // 合并后应有 2 个操作（模板 1 个 + 基础 1 个）

	// 验证模板操作（pass_headers）排在第一位
	firstOp, ok := ops[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "pass_headers", firstOp["mode"])

	// 验证基础操作（trim_prefix）排在第二位
	secondOp, ok := ops[1].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "trim_prefix", secondOp["mode"])
}

// TestShouldSkipRetryAfterChannelAffinityFailure 测试重试跳过判断函数的各种场景：
// - nil 上下文时返回 false
// - 上下文中显式设置了跳过重试标记时返回 true
// - 规则元数据中 SkipRetry 为 true 时返回 true
// - 规则元数据中 SkipRetry 为 false 时返回 false
func TestShouldSkipRetryAfterChannelAffinityFailure(t *testing.T) {
	tests := []struct {
		name string
		ctx  func() *gin.Context
		want bool
	}{
		{
			name: "nil context",
			ctx: func() *gin.Context {
				return nil
			},
			want: false,
		},
		{
			name: "explicit skip retry flag in context",
			ctx: func() *gin.Context {
				ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
					RuleName:   "rule-explicit-flag",
					SkipRetry:  false,
					UsingGroup: "default",
					ModelName:  "gpt-5",
				})
				ctx.Set(ginKeyChannelAffinitySkipRetry, true)
				return ctx
			},
			want: true,
		},
		{
			name: "fallback to matched rule meta",
			ctx: func() *gin.Context {
				return buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
					RuleName:   "rule-skip-retry",
					SkipRetry:  true,
					UsingGroup: "default",
					ModelName:  "gpt-5",
				})
			},
			want: true,
		},
		{
			name: "no flag and no skip retry meta",
			ctx: func() *gin.Context {
				return buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
					RuleName:   "rule-no-skip-retry",
					SkipRetry:  false,
					UsingGroup: "default",
					ModelName:  "gpt-5",
				})
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ShouldSkipRetryAfterChannelAffinityFailure(tt.ctx()))
		})
	}
}

// TestExtractChannelAffinityValue_RequestHeader 测试 request_header 键来源。
// 管理员可通过该来源把租户 ID、会话 ID 等上游客户端头作为亲和键。
func TestExtractChannelAffinityValue_RequestHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Affinity-Key", " tenant-123 ")

	value := extractChannelAffinityValue(ctx, operation_setting.ChannelAffinityKeySource{
		Type: "request_header",
		Key:  "X-Affinity-Key",
	})

	require.Equal(t, "tenant-123", value)
}

// TestGetPreferredChannelByAffinity_RequestHeaderKeySource 测试 request_header 规则可命中缓存。
// 这让非 JSON 请求或不能稳定暴露 body 字段的客户端，也能通过显式 Header 建立渠道亲和。
func TestGetPreferredChannelByAffinity_RequestHeaderKeySource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rule := operation_setting.ChannelAffinityRule{
		Name:       "header-affinity",
		ModelRegex: []string{"^gpt-5$"},
		PathRegex:  []string{"/v1/chat/completions"},
		KeySources: []operation_setting.ChannelAffinityKeySource{
			{Type: "request_header", Key: "X-Affinity-Key"},
		},
		TTLSeconds:        60,
		IncludeUsingGroup: true,
		IncludeRuleName:   true,
	}
	affinityValue := fmt.Sprintf("header-hit-%d", time.Now().UnixNano())
	cacheKeySuffix := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", affinityValue)
	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, 9528, time.Minute))
	t.Cleanup(func() {
		_, _ = cache.DeleteMany([]string{cacheKeySuffix})
	})

	setting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, setting)
	originalRules := append([]operation_setting.ChannelAffinityRule(nil), setting.Rules...)
	setting.Rules = append([]operation_setting.ChannelAffinityRule{rule}, originalRules...)
	t.Cleanup(func() {
		setting.Rules = originalRules
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Affinity-Key", affinityValue)

	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)
	require.Equal(t, 9528, channelID)

	meta, ok := getChannelAffinityMeta(ctx)
	require.True(t, ok)
	require.Equal(t, "request_header", meta.KeySourceType)
	require.Equal(t, "X-Affinity-Key", meta.KeySourceKey)
	require.Equal(t, buildChannelAffinityKeyHint(affinityValue), meta.KeyHint)
}

// TestClearCurrentChannelAffinityCache 测试当前请求命中的亲和缓存清理能力。
// 当亲和渠道已经禁用或不再可用时，分发层会调用该函数清理本次命中的缓存键，
// 同时清除跳过重试标记，让后续请求可以重新选择健康渠道。
func TestClearCurrentChannelAffinityCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cacheKeySuffix := fmt.Sprintf("codex cli trace:default:clear-current-%d", time.Now().UnixNano())
	cacheKeyFull := channelAffinityCacheNamespace + ":" + cacheKeySuffix
	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, 9527, time.Minute))
	t.Cleanup(func() {
		_, _ = cache.DeleteMany([]string{cacheKeySuffix})
	})

	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		CacheKey:   cacheKeyFull,
		TTLSeconds: 60,
		RuleName:   "codex cli trace",
		SkipRetry:  true,
	})
	require.True(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx))

	deleted := ClearCurrentChannelAffinityCache(ctx)
	require.True(t, deleted)
	_, found, err := cache.Get(cacheKeySuffix)
	require.NoError(t, err)
	require.False(t, found)
	require.False(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx))
}

// TestShouldKeepChannelAffinityOnChannelDisabled 测试亲和渠道不可用时的保留开关。
// 默认 false 会清理失效缓存；管理员显式开启后才保留旧缓存，等待渠道恢复。
func TestShouldKeepChannelAffinityOnChannelDisabled(t *testing.T) {
	setting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, setting)
	previous := setting.KeepOnChannelDisabled
	t.Cleanup(func() {
		setting.KeepOnChannelDisabled = previous
	})

	setting.KeepOnChannelDisabled = false
	require.False(t, ShouldKeepChannelAffinityOnChannelDisabled())

	setting.KeepOnChannelDisabled = true
	require.True(t, ShouldKeepChannelAffinityOnChannelDisabled())
}

// TestChannelAffinityHitCodexTemplatePassHeadersEffective 测试 Codex CLI 场景下的端到端流程：
// - 缓存命中时能正确获取优选渠道 ID
// - 参数覆盖模板正确应用
// - 请求头透传（pass_headers）操作生效，将原始请求中的 Originator、Session_id 等头传递到上游
// - 未在白名单中的头（如 x-codex-beta-features）不会被透传
func TestChannelAffinityHitCodexTemplatePassHeadersEffective(t *testing.T) {
	gin.SetMode(gin.TestMode)

	setting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, setting)

	var codexRule *operation_setting.ChannelAffinityRule
	for i := range setting.Rules {
		rule := &setting.Rules[i]
		if strings.EqualFold(strings.TrimSpace(rule.Name), "codex cli trace") {
			codexRule = rule
			break
		}
	}
	require.NotNil(t, codexRule)

	affinityValue := fmt.Sprintf("pc-hit-%d", time.Now().UnixNano())
	cacheKeySuffix := buildChannelAffinityCacheKeySuffix(*codexRule, "gpt-5", "default", affinityValue)

	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, 9527, time.Minute))
	t.Cleanup(func() {
		_, _ = cache.DeleteMany([]string{cacheKeySuffix})
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"prompt_cache_key":"%s"}`, affinityValue)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)            // 缓存命中，应找到优选渠道
	require.Equal(t, 9527, channelID) // 返回缓存中存储的渠道 ID

	baseOverride := map[string]interface{}{
		"temperature": 0.2,
	}
	mergedOverride, applied := ApplyChannelAffinityOverrideTemplate(ctx, baseOverride)
	require.True(t, applied)
	require.Equal(t, 0.2, mergedOverride["temperature"])

	info := &relaycommon.RelayInfo{
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
			"User-Agent": "codex-cli-test",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: mergedOverride,
			HeadersOverride: map[string]interface{}{
				"X-Static": "legacy-static",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-5"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride) // 应启用运行时头覆盖

	// 验证原始静态头保留，以及 pass_headers 操作透传的请求头生效
	require.Equal(t, "legacy-static", info.RuntimeHeadersOverride["x-static"])
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])      // 从原始请求头透传
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])       // 从原始请求头透传
	require.Equal(t, "codex-cli-test", info.RuntimeHeadersOverride["user-agent"]) // 从原始请求头透传

	// 验证未在白名单中的 Codex 特定头不会被透传
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	_, exists = info.RuntimeHeadersOverride["x-codex-turn-metadata"]
	require.False(t, exists)
}
