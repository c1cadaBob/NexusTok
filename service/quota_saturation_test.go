package service

import (
	"fmt"
	"math"
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAttachQuotaSaturationNestsUnderAdminInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{
		UserId:          7,
		OriginModelName: "gpt-image-1",
		QuotaClamp: &common.QuotaClamp{
			Op:       "QuotaFromDecimal",
			Kind:     common.QuotaClampOverflow,
			Original: 1.8e19,
			Clamped:  common.MaxQuota,
		},
	}

	other := map[string]interface{}{"model_price": 0.004}
	AttachQuotaSaturation(ctx, relayInfo, other)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	saturation, ok := adminInfo["quota_saturation"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "QuotaFromDecimal", saturation["op"])
	require.Equal(t, common.QuotaClampOverflow, saturation["kind"])
	require.Equal(t, common.MaxQuota, saturation["clamped"])
}

func TestAttachQuotaSaturationPreservesExistingAdminInfo(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		QuotaClamp: &common.QuotaClamp{
			Op:      "QuotaFromFloat",
			Kind:    common.QuotaClampUnderflow,
			Clamped: common.MinQuota,
		},
	}
	other := map[string]interface{}{
		"admin_info": map[string]interface{}{"admin_username": "root"},
	}

	AttachQuotaSaturation(nil, relayInfo, other)

	adminInfo := other["admin_info"].(map[string]interface{})
	require.Equal(t, "root", adminInfo["admin_username"])
	require.NotNil(t, adminInfo["quota_saturation"])
}

func TestAttachQuotaSaturationNoClampNoMarker(t *testing.T) {
	other := map[string]interface{}{"model_price": 0.004}
	AttachQuotaSaturation(nil, &relaycommon.RelayInfo{}, other)

	_, exists := other["admin_info"]
	require.False(t, exists)
}

func TestGenerateTextOtherInfoIncludesUpstreamRatioConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(ctx, constant.ContextKeyUpstreamRatioConversion, 0.35)

	relayInfo := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 0, 0, 1)

	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	require.InDelta(t, 0.35, adminInfo["ratio_conversion"], 0.000001)
}

func TestGenerateTextOtherInfoIncludesClientGoneWarningSeverity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)

	relayInfo := &relaycommon.RelayInfo{
		IsStream:     true,
		StreamStatus: relaycommon.NewStreamStatus(),
		ChannelMeta:  &relaycommon.ChannelMeta{},
	}
	relayInfo.StreamStatus.SetEndReason(
		relaycommon.StreamEndReasonClientGone,
		fmt.Errorf("context canceled"),
	)

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 0, 0, 1)

	streamInfo, ok := other["stream_status"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "error", streamInfo["status"])
	require.Equal(t, "warning", streamInfo["severity"])
	require.Equal(t, "client_gone", streamInfo["end_reason"])
	require.Equal(t, "context canceled", streamInfo["end_error"])
}

func TestAttachStandardBillingQuotaNestsUnderAdminInfo(t *testing.T) {
	other := map[string]interface{}{
		"admin_info": map[string]interface{}{
			"ratio_conversion": 0.35,
		},
	}

	AttachStandardBillingQuotaToOther(other, 1234)

	adminInfo := other["admin_info"].(map[string]interface{})
	require.InDelta(t, 0.35, adminInfo["ratio_conversion"], 0.000001)
	require.Equal(t, 1234, adminInfo["standard_billing_quota"])
}

func TestStandardBillingQuotaFromPriceDataIgnoresGroupRatio(t *testing.T) {
	quota, ok := StandardBillingQuotaFromPriceData(types.PriceData{
		UsePrice:   true,
		ModelPrice: 0.004,
		OtherRatios: map[string]float64{
			"n": 2,
		},
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 3},
		Quota:          12000,
	}, 12000)

	require.True(t, ok)
	require.Equal(t, 4000, quota)
}

func TestStandardBillingQuotaFromPriceDataReportsUnavailableFallback(t *testing.T) {
	quota, ok := StandardBillingQuotaFromPriceData(types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0},
		Quota:          0,
	}, 0)

	require.False(t, ok)
	require.Equal(t, 0, quota)
}

func TestComputeToolCallQuotaUsesQuotaRound(t *testing.T) {
	result := ComputeToolCallQuota(ToolCallUsage{
		ModelName:              "gpt-4o",
		WebSearchCalls:         2,
		WebSearchToolName:      "web_search_preview",
		FileSearchCalls:        1,
		ImageGenerationCall:    true,
		ImageGenerationQuality: "low",
		ImageGenerationSize:    "1024x1024",
	}, 1)

	require.Equal(t, 3, len(result.Items))
	require.Equal(t, 25000, result.Items[0].Quota)
	require.Equal(t, 1250, result.Items[1].Quota)
	require.Equal(t, 5500, result.Items[2].Quota)
	require.Equal(t, 31750, result.TotalQuota)
}

func TestComputeToolCallQuotaSaturatesLargeUsage(t *testing.T) {
	result := ComputeToolCallQuota(ToolCallUsage{
		ModelName:         "gpt-4o",
		WebSearchCalls:    math.MaxInt32,
		WebSearchToolName: "web_search_preview",
		FileSearchCalls:   math.MaxInt32,
	}, 1)

	require.Equal(t, 2, len(result.Items))
	require.Equal(t, common.MaxQuota, result.Items[0].Quota)
	require.Equal(t, common.MaxQuota, result.Items[1].Quota)
	require.Equal(t, common.MaxQuota, result.TotalQuota)
}

func TestCalcViolationFeeQuotaUsesDecimalAndSaturates(t *testing.T) {
	quota, clamp := calcViolationFeeQuota(0.05, 1)
	require.Equal(t, 25000, quota)
	require.Nil(t, clamp)

	quota, clamp = calcViolationFeeQuota(0.000001, 1)
	require.Equal(t, 1, quota)
	require.Nil(t, clamp)

	quota, clamp = calcViolationFeeQuota(1e20, 1)
	require.Equal(t, common.MaxQuota, quota)
	require.NotNil(t, clamp)
	require.Equal(t, "QuotaFromDecimal", clamp.Op)
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
}

func TestCalcViolationFeeQuotaHandlesInvalidFloat(t *testing.T) {
	quota, clamp := calcViolationFeeQuota(math.NaN(), 1)
	require.Equal(t, 0, quota)
	require.NotNil(t, clamp)
	require.Equal(t, common.QuotaClampNaN, clamp.Kind)
	require.Equal(t, 0, clamp.Clamped)

	quota, clamp = calcViolationFeeQuota(math.Inf(1), 1)
	require.Equal(t, common.MaxQuota, quota)
	require.NotNil(t, clamp)
	require.Equal(t, common.QuotaClampOverflow, clamp.Kind)
}
