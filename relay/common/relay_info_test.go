// 本文件是 relay/common 包中 RelayInfo.GetFinalRequestRelayFormat 方法的单元测试集。
// 测试了该方法的三种回退策略：显式指定的 FinalRequestRelayFormat、转换链最后一个格式、以及原始 RelayFormat。
package common

import (
	"testing"
	"time"

	"github.com/c1cada/NexusTok/types"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoTimingMetricsSplitsRequestStages(t *testing.T) {
	observed := time.Unix(100, 0)
	selected := observed.Add(120 * time.Millisecond)
	upstreamStart := selected.Add(30 * time.Millisecond)
	header := upstreamStart.Add(450 * time.Millisecond)
	firstSSE := header.Add(80 * time.Millisecond)
	now := firstSSE.Add(2 * time.Second)
	info := &RelayInfo{
		ObservedStartTime:          observed,
		StartTime:                  selected,
		UpstreamRequestStartTime:   upstreamStart,
		UpstreamResponseHeaderTime: header,
		FirstResponseTime:          firstSSE,
	}

	require.Equal(t, int64(120), info.TimingMetrics(now)["selected_ms"])
	require.Equal(t, int64(450), info.TimingMetrics(now)["upstream_header_ms"])
	require.Equal(t, int64(80), info.TimingMetrics(now)["first_sse_ms"])
	require.Equal(t, int64(2680), info.TimingMetrics(now)["total_ms"])
}

func TestRelayInfoTimingMetricsLeavesMissingStagesAtZero(t *testing.T) {
	info := &RelayInfo{
		ObservedStartTime: time.Unix(100, 0),
		StartTime:         time.Unix(101, 0),
	}

	metrics := info.TimingMetrics(time.Unix(102, 0))
	require.Equal(t, int64(1000), metrics["selected_ms"])
	require.Zero(t, metrics["upstream_header_ms"])
	require.Zero(t, metrics["first_sse_ms"])
	require.Equal(t, int64(2000), metrics["total_ms"])
}

// TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal 测试优先使用显式指定的 FinalRequestRelayFormat。
func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

// TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain 测试回退到转换链最后一个格式。
func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

// TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat 测试回退到原始 RelayFormat。
func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

// TestRelayInfoGetFinalRequestRelayFormatNilReceiver 测试 nil 接收者调用时返回空字符串。
func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}
