// 本文件测试文本配额计算（calculateTextQuotaSummary）功能，包括：
// - Claude 语义模式下 Chat 和 Messages 两种 relay 格式的配额一致性
// - 分离式缓存创建比率（5 分钟/1 小时）的正确应用
// - 上游 Usage 中 anthropic 语义标记的识别
// - 缓存写入 token 总数的计算（兼容旧版和分离式）
// - 旧版 Claude 派生 OpenAI 格式的处理
// - OpenRouter 渠道的缓存读取/创建 token 从 prompt 中分离计费
// - 分层计费模式下的工具调用附加费保留
package service

import (
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/pkg/billingexpr"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// TestCalculateTextQuotaSummaryUnifiedForClaudeSemantic 测试 Claude 语义模式下
// Chat（OpenAI 格式）和 Messages（Claude 格式）两种 relay 格式的配额计算结果一致，
// 验证缓存 token（cache_read、cache_creation_5m、cache_creation_1h）的正确计费。
func TestCalculateTextQuotaSummaryUnifiedForClaudeSemantic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	usage := &dto.Usage{
		PromptTokens:     1000,
		CompletionTokens: 200,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         100,
			CachedCreationTokens: 50,
		},
		ClaudeCacheCreation5mTokens: 10,
		ClaudeCacheCreation1hTokens: 20,
	}

	priceData := types.PriceData{
		ModelRatio:           1,
		CompletionRatio:      2,
		CacheRatio:           0.1,
		CacheCreationRatio:   1.25,
		CacheCreation5mRatio: 1.25,
		CacheCreation1hRatio: 2,
		GroupRatioInfo: types.GroupRatioInfo{
			GroupRatio: 1,
		},
	}

	chatRelayInfo := &relaycommon.RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		FinalRequestRelayFormat: types.RelayFormatClaude,
		OriginModelName:         "claude-3-7-sonnet",
		PriceData:               priceData,
		StartTime:               time.Now(),
	}
	messageRelayInfo := &relaycommon.RelayInfo{
		RelayFormat:             types.RelayFormatClaude,
		FinalRequestRelayFormat: types.RelayFormatClaude,
		OriginModelName:         "claude-3-7-sonnet",
		PriceData:               priceData,
		StartTime:               time.Now(),
	}

	chatSummary := calculateTextQuotaSummary(ctx, chatRelayInfo, usage)
	messageSummary := calculateTextQuotaSummary(ctx, messageRelayInfo, usage)

	// 验证 Chat 和 Messages 两种格式的配额计算结果一致
	require.Equal(t, messageSummary.Quota, chatSummary.Quota)
	require.Equal(t, messageSummary.CacheCreationTokens5m, chatSummary.CacheCreationTokens5m)
	require.Equal(t, messageSummary.CacheCreationTokens1h, chatSummary.CacheCreationTokens1h)
	require.True(t, chatSummary.IsClaudeUsageSemantic) // 应识别为 Claude 语义
	require.Equal(t, 1488, chatSummary.Quota)          // 预期配额值
	require.Equal(t, 1488, chatSummary.StandardBillingQuota)
}

func TestCalculateTextQuotaSummaryRecordsStandardBillingQuotaWithoutGroupRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "gpt-test",
		PriceData: types.PriceData{
			ModelRatio:           1.5,
			CompletionRatio:      2,
			CacheRatio:           0.5,
			CacheCreationRatio:   1.25,
			CacheCreation5mRatio: 1.25,
			CacheCreation1hRatio: 2,
			ImageRatio:           3,
			GroupRatioInfo:       types.GroupRatioInfo{GroupRatio: 2},
		},
		StartTime: time.Now(),
	}
	usage := &dto.Usage{
		PromptTokens:     120,
		CompletionTokens: 40,
		TotalTokens:      160,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         20,
			CachedCreationTokens: 10,
			ImageTokens:          5,
		},
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	// 标准基准保留模型/补全/缓存/图片倍率：((85 + 20*0.5 + 10*1.25 + 5*3) + 40*2) * 1.5 = 303.75 => 304。
	require.Equal(t, 608, summary.Quota)
	require.Equal(t, 304, summary.StandardBillingQuota)
}

func TestCalculateTextQuotaSummaryStandardBillingQuotaIgnoresUserGroupSpecialRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "gpt-special",
		PriceData: types.PriceData{
			ModelRatio:      1,
			CompletionRatio: 1,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio:        3,
				GroupSpecialRatio: 3,
				HasSpecialRatio:   true,
			},
		},
		StartTime: time.Now(),
	}
	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 20,
		TotalTokens:      120,
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	require.Equal(t, 360, summary.Quota)
	require.Equal(t, 120, summary.StandardBillingQuota)
}

func TestCalculateTextQuotaSummaryStandardBillingQuotaForPriceMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "fixed-price",
		PriceData: types.PriceData{
			UsePrice:   true,
			ModelPrice: 0.004,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 2.5,
			},
		},
		StartTime: time.Now(),
	}
	usage := &dto.Usage{
		PromptTokens:     1,
		CompletionTokens: 1,
		TotalTokens:      2,
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	require.Equal(t, 5000, summary.Quota)
	require.Equal(t, 2000, summary.StandardBillingQuota)
}

func TestCalculateTextQuotaSummaryStandardBillingQuotaKeepsOtherRatiosAndFreeGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "free-downstream-group",
		PriceData: types.PriceData{
			ModelRatio:      2,
			CompletionRatio: 1,
			OtherRatios: map[string]float64{
				"request_multiplier": 1.5,
			},
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0},
		},
		StartTime: time.Now(),
	}
	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 0,
		TotalTokens:      100,
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	require.Equal(t, 0, summary.Quota)
	require.Equal(t, 300, summary.StandardBillingQuota)
}

func TestCalculateTextQuotaSummaryStandardBillingQuotaKeepsMinimumWhenGroupIsFree(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "tiny-free-group",
		PriceData: types.PriceData{
			ModelRatio:      0.001,
			CompletionRatio: 1,
			GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 0},
		},
		StartTime: time.Now(),
	}
	usage := &dto.Usage{
		PromptTokens:     1,
		CompletionTokens: 0,
		TotalTokens:      1,
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	require.Equal(t, 0, summary.Quota)
	require.Equal(t, 1, summary.StandardBillingQuota)
}

// TestCalculateTextQuotaSummaryUsesSplitClaudeCacheCreationRatios 测试分离式缓存创建比率的应用，
// 验证 cache_creation_5m 和 cache_creation_1h 使用各自独立的比率进行计费。
func TestCalculateTextQuotaSummaryUsesSplitClaudeCacheCreationRatios(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		FinalRequestRelayFormat: types.RelayFormatClaude,
		OriginModelName:         "claude-3-7-sonnet",
		PriceData: types.PriceData{
			ModelRatio:           1,
			CompletionRatio:      1,
			CacheRatio:           0,
			CacheCreationRatio:   1,
			CacheCreation5mRatio: 2,
			CacheCreation1hRatio: 3,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
		StartTime: time.Now(),
	}

	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 0,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedCreationTokens: 10,
		},
		ClaudeCacheCreation5mTokens: 2,
		ClaudeCacheCreation1hTokens: 3,
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	// 100 + remaining(5)*1 + 2*2 + 3*3 = 118
	require.Equal(t, 118, summary.Quota) // 验证分离式缓存比率的配额计算结果
}

// TestCalculateTextQuotaSummaryUsesAnthropicUsageSemanticFromUpstreamUsage 测试上游 Usage 中
// 包含 "anthropic" 语义标记时，系统能正确识别为 Claude 语义模式并进行相应计费。
func TestCalculateTextQuotaSummaryUsesAnthropicUsageSemanticFromUpstreamUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAI,
		OriginModelName: "claude-3-7-sonnet",
		PriceData: types.PriceData{
			ModelRatio:           1,
			CompletionRatio:      2,
			CacheRatio:           0.1,
			CacheCreationRatio:   1.25,
			CacheCreation5mRatio: 1.25,
			CacheCreation1hRatio: 2,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
		StartTime: time.Now(),
	}

	usage := &dto.Usage{
		PromptTokens:     1000,
		CompletionTokens: 200,
		UsageSemantic:    "anthropic",
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         100,
			CachedCreationTokens: 50,
		},
		ClaudeCacheCreation5mTokens: 10,
		ClaudeCacheCreation1hTokens: 20,
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	require.True(t, summary.IsClaudeUsageSemantic)       // 应识别为 Claude 语义
	require.Equal(t, "anthropic", summary.UsageSemantic) // 语义标记保持为 "anthropic"
	require.Equal(t, 1488, summary.Quota)                // 配额与 Chat 格式一致
}

// TestCacheWriteTokensTotal 测试缓存写入 token 总数的计算函数，
// 验证三种场景：分离式缓存创建（含聚合余数）、旧版缓存创建、分离式无聚合余数。
func TestCacheWriteTokensTotal(t *testing.T) {
	t.Run("split cache creation", func(t *testing.T) { // 分离式：返回 CacheCreationTokens（聚合值）
		summary := textQuotaSummary{
			CacheCreationTokens:   50,
			CacheCreationTokens5m: 10,
			CacheCreationTokens1h: 20,
		}
		require.Equal(t, 50, cacheWriteTokensTotal(summary))
	})

	t.Run("legacy cache creation", func(t *testing.T) { // 旧版：仅 CacheCreationTokens，直接返回
		summary := textQuotaSummary{CacheCreationTokens: 50}
		require.Equal(t, 50, cacheWriteTokensTotal(summary))
	})

	t.Run("split cache creation without aggregate remainder", func(t *testing.T) { // 分离式无聚合余数：返回 5m + 1h 之和
		summary := textQuotaSummary{
			CacheCreationTokens5m: 10,
			CacheCreationTokens1h: 20,
		}
		require.Equal(t, 30, cacheWriteTokensTotal(summary))
	})
}

// TestCalculateTextQuotaSummaryHandlesLegacyClaudeDerivedOpenAIUsage 测试旧版 Claude 派生 OpenAI 格式的处理，
// 验证当 relay 格式为 OpenAI 但模型为 Claude 时，缓存 token 正确从 prompt 中分离并使用相应比率计费。
func TestCalculateTextQuotaSummaryHandlesLegacyClaudeDerivedOpenAIUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAI,
		OriginModelName: "claude-3-7-sonnet",
		PriceData: types.PriceData{
			ModelRatio:           1,
			CompletionRatio:      5,
			CacheRatio:           0.1,
			CacheCreationRatio:   1.25,
			CacheCreation5mRatio: 1.25,
			CacheCreation1hRatio: 2,
			GroupRatioInfo:       types.GroupRatioInfo{GroupRatio: 1},
		},
		StartTime: time.Now(),
	}

	usage := &dto.Usage{
		PromptTokens:     62,
		CompletionTokens: 95,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 3544,
		},
		ClaudeCacheCreation5mTokens: 586,
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	// 62 + 3544*0.1 + 586*1.25 + 95*5 = 1624.9 => 1624
	require.Equal(t, 1624, summary.Quota)
}

// TestCalculateTextQuotaSummarySeparatesOpenRouterCacheReadFromPromptBilling 测试 OpenRouter 渠道的缓存读取 token 分离计费，
// 验证 prompt_tokens 保持原始总数用于日志显示，但计费时缓存读取部分使用独立比率。
func TestCalculateTextQuotaSummarySeparatesOpenRouterCacheReadFromPromptBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "openai/gpt-4.1",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenRouter,
		},
		PriceData: types.PriceData{
			ModelRatio:         1,
			CompletionRatio:    1,
			CacheRatio:         0.1,
			CacheCreationRatio: 1.25,
			GroupRatioInfo:     types.GroupRatioInfo{GroupRatio: 1},
		},
		StartTime: time.Now(),
	}

	usage := &dto.Usage{
		PromptTokens:     2604,
		CompletionTokens: 383,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 2432,
		},
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	// OpenRouter OpenAI-format display keeps prompt_tokens as total input,
	// but billing still separates normal input from cache read tokens.
	// quota = (2604 - 2432) + 2432*0.1 + 383 = 798.2 => 798
	require.Equal(t, 2604, summary.PromptTokens) // prompt_tokens 保持原始总数（用于日志显示）
	require.Equal(t, 798, summary.Quota)         // 配额按分离后的 token 计算
}

// TestCalculateTextQuotaSummarySeparatesOpenRouterCacheCreationFromPromptBilling 测试 OpenRouter 渠道的缓存创建 token 分离计费，
// 验证缓存创建 token 从 prompt 中分离后使用独立的创建比率计费。
func TestCalculateTextQuotaSummarySeparatesOpenRouterCacheCreationFromPromptBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "openai/gpt-4.1",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenRouter,
		},
		PriceData: types.PriceData{
			ModelRatio:         1,
			CompletionRatio:    1,
			CacheCreationRatio: 1.25,
			GroupRatioInfo:     types.GroupRatioInfo{GroupRatio: 1},
		},
		StartTime: time.Now(),
	}

	usage := &dto.Usage{
		PromptTokens:     2604,
		CompletionTokens: 383,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedCreationTokens: 100,
		},
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	// prompt_tokens is still logged as total input, but cache creation is billed separately.
	// quota = (2604 - 100) + 100*1.25 + 383 = 3012
	require.Equal(t, 2604, summary.PromptTokens)
	require.Equal(t, 3012, summary.Quota)
}

// TestCalculateTextQuotaSummaryKeepsPrePRClaudeOpenRouterBilling 测试 OpenRouter 上 Claude 模型的旧版计费行为保持不变，
// 验证在 PostClaudeConsumeQuota 之前的逻辑：prompt tokens 从总数中减去缓存 tokens 后再计费。
func TestCalculateTextQuotaSummaryKeepsPrePRClaudeOpenRouterBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		FinalRequestRelayFormat: types.RelayFormatClaude,
		OriginModelName:         "anthropic/claude-3.7-sonnet",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenRouter,
		},
		PriceData: types.PriceData{
			ModelRatio:         1,
			CompletionRatio:    1,
			CacheRatio:         0.1,
			CacheCreationRatio: 1.25,
			GroupRatioInfo:     types.GroupRatioInfo{GroupRatio: 1},
		},
		StartTime: time.Now(),
	}

	usage := &dto.Usage{
		PromptTokens:     2604,
		CompletionTokens: 383,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 2432,
		},
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	// Pre-PR PostClaudeConsumeQuota behavior for OpenRouter:
	// prompt = 2604 - 2432 = 172
	// quota = 172 + 2432*0.1 + 383 = 798.2 => 798
	require.True(t, summary.IsClaudeUsageSemantic) // 应识别为 Claude 语义
	require.Equal(t, 172, summary.PromptTokens)    // prompt = 2604 - 2432 = 172（旧版逻辑）
	require.Equal(t, 798, summary.Quota)           // 配额与新版 OpenRouter 逻辑一致
}

// TestComposeTieredTextQuotaKeepsToolCallSurcharges 测试分层计费模式下工具调用附加费的保留，
// 验证 web_search、file_search 等内置工具的附加费正确累加到总配额中。
func TestComposeTieredTextQuotaKeepsToolCallSurcharges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Set("image_generation_call", true)
	ctx.Set("image_generation_call_quality", "low")
	ctx.Set("image_generation_call_size", "1024x1024")

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "o1",
		PriceData: types.PriceData{
			ModelRatio:      1,
			CompletionRatio: 1,
			GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 1},
		},
		ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
			BuiltInTools: map[string]*relaycommon.BuildInToolInfo{
				dto.BuildInToolWebSearchPreview: &relaycommon.BuildInToolInfo{
					CallCount: 1,
				},
				dto.BuildInToolFileSearch: &relaycommon.BuildInToolInfo{
					CallCount: 2,
				},
			},
		},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:               "tiered_expr",
			GroupRatio:                1,
			EstimatedQuotaBeforeGroup: 1000,
		},
		StartTime: time.Now(),
	}

	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)
	quota := composeTieredTextQuota(relayInfo, summary, 1000, &billingexpr.TieredResult{
		ActualQuotaBeforeGroup: 1000,
		ActualQuotaAfterGroup:  1000,
	})
	standardQuota := composeTieredStandardBillingQuota(summary, 1000, &billingexpr.TieredResult{
		ActualQuotaBeforeGroup: 1000,
		ActualQuotaAfterGroup:  1000,
	})

	require.Equal(t, int64(13000), summary.ToolCallSurchargeQuota.Round(0).IntPart()) // 工具调用附加费
	require.Equal(t, int64(13000), summary.StandardToolSurcharge.Round(0).IntPart())  // group_ratio=1 的标准附加费
	require.Equal(t, 14000, quota)                                                    // 总配额 = 分层配额 + 附加费
	require.Equal(t, 14000, standardQuota)
}

func TestComposeTieredTextQuotaStandardBillingUsesBeforeGroup(t *testing.T) {
	summary := textQuotaSummary{
		GroupRatio:            2,
		StandardBillingQuota:  999,
		StandardToolSurcharge: decimal.NewFromInt(250),
	}

	standardQuota := composeTieredStandardBillingQuota(summary, 2000, &billingexpr.TieredResult{
		ActualQuotaBeforeGroup: 1000,
		ActualQuotaAfterGroup:  2000,
	})

	require.Equal(t, 1250, standardQuota)
}

func TestCalculateTextQuotaSummaryRecordsQuotaClamp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "quota-clamp-model",
		PriceData: types.PriceData{
			UsePrice:   true,
			ModelPrice: 1.8446744073686647e19,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
		StartTime: time.Now(),
	}
	usage := &dto.Usage{
		PromptTokens:     1,
		CompletionTokens: 1,
		TotalTokens:      2,
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	require.Equal(t, math.MaxInt32, summary.Quota)
	require.NotNil(t, relayInfo.QuotaClamp)
	require.Equal(t, common.QuotaClampOverflow, relayInfo.QuotaClamp.Kind)
	require.Equal(t, common.MaxQuota, relayInfo.QuotaClamp.Clamped)
}

// TestComposeTieredTextQuotaFallbackKeepsToolCallSurcharges 测试分层计费回退路径下的工具调用附加费保留，
// 验证当 tieredResult 为 nil 时，使用回退配额计算附加费。
func TestComposeTieredTextQuotaFallbackKeepsToolCallSurcharges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Set("claude_web_search_requests", 2)

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-7-sonnet",
		PriceData: types.PriceData{
			ModelRatio:      1,
			CompletionRatio: 1,
			GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 1.25},
		},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:               "tiered_expr",
			GroupRatio:                1.25,
			EstimatedQuotaBeforeGroup: 1000,
		},
		StartTime: time.Now(),
	}

	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)
	quota := composeTieredTextQuota(relayInfo, summary, 1250, nil)

	require.Equal(t, int64(12500), summary.ToolCallSurchargeQuota.Round(0).IntPart())
	require.Equal(t, 13750, quota)
}

// TestComposeTieredTextQuotaErrorFallbackUsesPreConsumedQuota 测试结算错误时使用预扣配额作为回退，
// 验证当 tieredResult 为 nil 且 preConsumedFallback 与估算值不同时，
// 附加费仍基于 groupRatio 计算，总配额使用预扣配额 + 附加费。
func TestComposeTieredTextQuotaErrorFallbackUsesPreConsumedQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Set("claude_web_search_requests", 2)

	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-7-sonnet",
		PriceData: types.PriceData{
			ModelRatio:      1,
			CompletionRatio: 1,
			GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 1.25},
		},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:               "tiered_expr",
			GroupRatio:                1.25,
			EstimatedQuotaBeforeGroup: 1000,
		},
		StartTime: time.Now(),
	}

	usage := &dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	summary := calculateTextQuotaSummary(ctx, relayInfo, usage)

	// tieredResult=nil 模拟结算错误，TryTieredSettle 回退到 FinalPreConsumedQuota (2000)，
	// 与 EstimatedQuotaBeforeGroup * GroupRatio (1250) 不同。
	preConsumedFallback := 2000
	quota := composeTieredTextQuota(relayInfo, summary, preConsumedFallback, nil)

	require.Equal(t, int64(12500), summary.ToolCallSurchargeQuota.Round(0).IntPart()) // 附加费基于 groupRatio 计算
	require.Equal(t, 14500, quota)                                                    // 总配额 = 预扣回退值 + 附加费
}
