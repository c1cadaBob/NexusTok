// 本文件测试渠道亲和性使用缓存的统计功能，包括：
// - Claude 模式下的使用统计（缓存命中率基于 CachedTokens / (PromptTokens + CachedTokens)）
// - 混合模式下的使用统计（同时处理 OpenAI 和 Claude 格式的 Usage）
// - 不支持的 relay 格式（如 Gemini）下缓存统计仍正常记录但不计算命中率模式
package service

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// buildChannelAffinityStatsContextForTest 构建用于测试的 gin.Context，
// 并设置渠道亲和性元数据（包含缓存键、TTL、规则名、分组名、密钥指纹）。
func buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP string) *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	setChannelAffinityContext(ctx, channelAffinityMeta{
		CacheKey:       fmt.Sprintf("test:%s:%s:%s", ruleName, usingGroup, keyFP),
		TTLSeconds:     600,
		RuleName:       ruleName,
		UsingGroup:     usingGroup,
		KeyFingerprint: keyFP,
	})
	return ctx
}

// TestObserveChannelAffinityUsageCacheByRelayFormat_ClaudeMode 测试 Claude 格式下的使用缓存统计，
// 验证：总请求计数、命中计数、prompt/completion/total token 数、缓存 token 数均正确累加，
// 且缓存命中率模式为 "cachedOverPromptPlusCached"（Claude 语义：命中率 = cached / (prompt + cached)）。
func TestObserveChannelAffinityUsageCacheByRelayFormat_ClaudeMode(t *testing.T) {
	ruleName := fmt.Sprintf("rule_%d", time.Now().UnixNano())
	usingGroup := "default"
	keyFP := fmt.Sprintf("fp_%d", time.Now().UnixNano())
	ctx := buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP)

	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 40,
		TotalTokens:      140,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 30,
		},
	}

	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, usage, types.RelayFormatClaude)
	stats := GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFP)

	require.EqualValues(t, 1, stats.Total)
	require.EqualValues(t, 1, stats.Hit)
	require.EqualValues(t, 100, stats.PromptTokens)
	require.EqualValues(t, 40, stats.CompletionTokens)
	require.EqualValues(t, 140, stats.TotalTokens)
	require.EqualValues(t, 30, stats.CachedTokens)
	require.Equal(t, cacheTokenRateModeCachedOverPromptPlusCached, stats.CachedTokenRateMode)
}

// TestObserveChannelAffinityUsageCacheByRelayFormat_MixedMode 测试混合模式下的使用缓存统计，
// 验证当先后收到 OpenAI 和 Claude 两种格式的 Usage 时，统计数据正确累加，
// 且缓存命中率模式自动切换为 "mixed"（混合模式）。
func TestObserveChannelAffinityUsageCacheByRelayFormat_MixedMode(t *testing.T) {
	ruleName := fmt.Sprintf("rule_%d", time.Now().UnixNano())
	usingGroup := "default"
	keyFP := fmt.Sprintf("fp_%d", time.Now().UnixNano())
	ctx := buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP)

	openAIUsage := &dto.Usage{
		PromptTokens: 100,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 10,
		},
	}
	claudeUsage := &dto.Usage{
		PromptTokens: 80,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 20,
		},
	}

	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, openAIUsage, types.RelayFormatOpenAI)
	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, claudeUsage, types.RelayFormatClaude)
	stats := GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFP)

	require.EqualValues(t, 2, stats.Total)
	require.EqualValues(t, 2, stats.Hit)
	require.EqualValues(t, 180, stats.PromptTokens)
	require.EqualValues(t, 30, stats.CachedTokens)
	require.Equal(t, cacheTokenRateModeMixed, stats.CachedTokenRateMode)
}

// TestObserveChannelAffinityUsageCacheByRelayFormat_UnsupportedModeKeepsEmpty 测试不支持的 relay 格式（Gemini），
// 验证统计数据（总数、命中数、缓存 token 数）仍正常记录，
// 但 CachedTokenRateMode 为空字符串（不计算命中率模式）。
func TestObserveChannelAffinityUsageCacheByRelayFormat_UnsupportedModeKeepsEmpty(t *testing.T) {
	ruleName := fmt.Sprintf("rule_%d", time.Now().UnixNano())
	usingGroup := "default"
	keyFP := fmt.Sprintf("fp_%d", time.Now().UnixNano())
	ctx := buildChannelAffinityStatsContextForTest(ruleName, usingGroup, keyFP)

	usage := &dto.Usage{
		PromptTokens: 100,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 25,
		},
	}

	ObserveChannelAffinityUsageCacheByRelayFormat(ctx, usage, types.RelayFormatGemini)
	stats := GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFP)

	require.EqualValues(t, 1, stats.Total)
	require.EqualValues(t, 1, stats.Hit)
	require.EqualValues(t, 25, stats.CachedTokens)
	require.Equal(t, "", stats.CachedTokenRateMode)
}
