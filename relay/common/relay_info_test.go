// 本文件是 relay/common 包中 RelayInfo.GetFinalRequestRelayFormat 方法的单元测试集。
// 测试了该方法的三种回退策略：显式指定的 FinalRequestRelayFormat、转换链最后一个格式、以及原始 RelayFormat。
package common

import (
	"testing"

	"github.com/c1cada/NexusTok/types"
	"github.com/stretchr/testify/require"
)

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
