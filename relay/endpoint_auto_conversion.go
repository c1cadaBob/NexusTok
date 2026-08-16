package relay

import (
	"fmt"
	"net/http"

	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"
)

// endpointAutoConversionChannelAllowed 判断当前渠道是否适合执行 OpenAI Chat ↔ Responses 包装转换。
//
// 自动纠错要求上游响应最终仍是 OpenAI Chat 或 OpenAI Responses 形态，否则响应处理器会把
// Gemini、Claude 等原生 JSON/SSE 错当成 OpenAI 响应解析。这里采用保守白名单：只允许
// OpenAI 兼容渠道和明确按 path 配置上游的 Advanced Custom；其它原生适配器保持明确报错。
func endpointAutoConversionChannelAllowed(info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelMeta == nil {
		return false
	}
	return relaycommon.EndpointAutoConversionChannelTypeAllowed(info.ChannelType)
}

func endpointAutoConversionUnsupportedError(info *relaycommon.RelayInfo) *types.NexusTokError {
	modelName := ""
	fromPath := ""
	toPath := ""
	if info != nil && info.EndpointAutoConversion != nil {
		modelName = info.EndpointAutoConversion.Model
		fromPath = info.EndpointAutoConversion.FromPath
		toPath = info.EndpointAutoConversion.ToPath
	}
	return types.NewErrorWithStatusCode(
		fmt.Errorf("模型 %s 的请求路径 %s 需要自动转换到 %s，但当前渠道类型不支持安全的 OpenAI Chat ↔ Responses 包装转换，请改用支持目标端点的 OpenAI 兼容渠道或关闭自动路径纠错", modelName, fromPath, toPath),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)
}

func endpointAutoConversionPassthroughError(info *relaycommon.RelayInfo) *types.NexusTokError {
	modelName := ""
	fromPath := ""
	toPath := ""
	if info != nil && info.EndpointAutoConversion != nil {
		modelName = info.EndpointAutoConversion.Model
		fromPath = info.EndpointAutoConversion.FromPath
		toPath = info.EndpointAutoConversion.ToPath
	}
	return types.NewErrorWithStatusCode(
		fmt.Errorf("模型 %s 的请求路径 %s 需要自动转换到 %s，但 passthrough 模式无法安全改写请求体，请关闭 passthrough 或使用正确端点", modelName, fromPath, toPath),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)
}
