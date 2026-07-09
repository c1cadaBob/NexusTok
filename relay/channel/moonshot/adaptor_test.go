package moonshot

import (
	"testing"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	relaycommon "github.com/c1cada/NexusTok/relay/common"

	"github.com/stretchr/testify/require"
)

// convertOpenAIRequestForTest 执行 Moonshot OpenAI 请求转换并返回强类型请求对象。
func convertOpenAIRequestForTest(t *testing.T, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) *dto.GeneralOpenAIRequest {
	t.Helper()

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)
	require.NoError(t, err)
	convertedRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	return convertedRequest
}

// moonshotRelayInfoForTest 构造包含上游模型名的 RelayInfo。
func moonshotRelayInfoForTest(upstreamModel string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: upstreamModel,
		},
	}
}

// TestConvertOpenAIRequestKimiK26UsesOnlyAllowedTemperature 验证 kimi-k2.6 的显式 temperature 会被修正为上游允许值。
func TestConvertOpenAIRequestKimiK26UsesOnlyAllowedTemperature(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:       "kimi-k2.6",
		Temperature: common.GetPointer[float64](0.7),
	}

	convertedRequest := convertOpenAIRequestForTest(t, moonshotRelayInfoForTest("kimi-k2.6"), request)

	require.NotNil(t, convertedRequest.Temperature)
	require.Equal(t, 1.0, *convertedRequest.Temperature)
}

// TestConvertOpenAIRequestKimiK26KeepsOmittedTemperatureOmitted 验证未传 temperature 时保持字段省略。
func TestConvertOpenAIRequestKimiK26KeepsOmittedTemperatureOmitted(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model: "kimi-k2.6",
	}

	convertedRequest := convertOpenAIRequestForTest(t, moonshotRelayInfoForTest("kimi-k2.6"), request)

	require.Nil(t, convertedRequest.Temperature)
}

// TestConvertOpenAIRequestKimiK26KeepsAllowedTemperature 验证显式 1.0 不会被重复改写。
func TestConvertOpenAIRequestKimiK26KeepsAllowedTemperature(t *testing.T) {
	temperature := 1.0
	request := &dto.GeneralOpenAIRequest{
		Model:       "kimi-k2.6",
		Temperature: &temperature,
	}

	convertedRequest := convertOpenAIRequestForTest(t, moonshotRelayInfoForTest("kimi-k2.6"), request)

	require.Same(t, &temperature, convertedRequest.Temperature)
	require.Equal(t, 1.0, *convertedRequest.Temperature)
}

// TestConvertOpenAIRequestKimiK26ClampsExplicitZeroTemperature 验证显式零值也会按上游限制修正，而不是被 omitempty 语义误判为缺省。
func TestConvertOpenAIRequestKimiK26ClampsExplicitZeroTemperature(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:       "kimi-k2.6",
		Temperature: common.GetPointer[float64](0),
	}

	convertedRequest := convertOpenAIRequestForTest(t, moonshotRelayInfoForTest("kimi-k2.6"), request)

	require.NotNil(t, convertedRequest.Temperature)
	require.Equal(t, 1.0, *convertedRequest.Temperature)
}

// TestConvertOpenAIRequestOtherMoonshotModelKeepsTemperature 验证其它 Moonshot 模型不会被误改 temperature。
func TestConvertOpenAIRequestOtherMoonshotModelKeepsTemperature(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:       "kimi-k2.5",
		Temperature: common.GetPointer[float64](0.7),
	}

	convertedRequest := convertOpenAIRequestForTest(t, moonshotRelayInfoForTest("kimi-k2.5"), request)

	require.NotNil(t, convertedRequest.Temperature)
	require.Equal(t, 0.7, *convertedRequest.Temperature)
}

// TestConvertOpenAIRequestUsesUpstreamModelName 验证模型映射后以上游模型名为准。
func TestConvertOpenAIRequestUsesUpstreamModelName(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:       "local-kimi-alias",
		Temperature: common.GetPointer[float64](0.2),
	}

	convertedRequest := convertOpenAIRequestForTest(t, moonshotRelayInfoForTest("kimi-k2.6"), request)

	require.NotNil(t, convertedRequest.Temperature)
	require.Equal(t, 1.0, *convertedRequest.Temperature)
}

// TestConvertOpenAIRequestFallsBackToRequestModel 验证未初始化 ChannelMeta 的路径仍能按请求 model 判断。
func TestConvertOpenAIRequestFallsBackToRequestModel(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{
		Model:       "kimi-k2.6",
		Temperature: common.GetPointer[float64](0.3),
	}

	convertedRequest := convertOpenAIRequestForTest(t, nil, request)

	require.NotNil(t, convertedRequest.Temperature)
	require.Equal(t, 1.0, *convertedRequest.Temperature)
}
