package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/model"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/setting/model_setting"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

const endpointAutoConvertDisableHeader = "NexusTok-Disable-Endpoint-Auto-Convert"

// prepareEndpointAutoConversion 在渠道选择前判断是否需要启用端点安全纠错。
//
// 这里刻意只读取已由 getModelRequest 解析出的 model 与当前 path，不做完整请求体验证：
// 完整验证仍由 Relay handler 按客户端原始路径负责。这样 Chat 请求仍按 Chat 结构校验并
// 返回 Chat 响应，Responses 请求仍按 Responses 结构校验并返回 Responses 响应；本函数只
// 影响“应该用哪个上游端点选路和转发”。
func prepareEndpointAutoConversion(c *gin.Context, modelRequest *ModelRequest) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil || modelRequest == nil {
		return true
	}
	if !model_setting.GetGlobalSettings().EndpointAutoConversionEnabled {
		return true
	}
	if endpointAutoConvertDisabled(c) {
		return true
	}

	requestPath := c.Request.URL.Path
	currentEndpoint, ok := endpointTypeFromRequestPath(requestPath)
	if !ok {
		return true
	}

	modelName := strings.TrimSpace(modelRequest.Model)
	if modelName == "" {
		return true
	}

	supported := model.GetModelSupportEndpointTypes(modelName)
	if len(supported) == 0 && strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		supported = model.GetModelSupportEndpointTypes(strings.TrimSuffix(modelName, ratio_setting.CompactModelSuffix))
	}
	if len(supported) == 0 || endpointSupportedForRequestPath(currentEndpoint, requestPath, supported) {
		return true
	}

	switch {
	case currentEndpoint == constant.EndpointTypeOpenAI && supportsEndpoint(supported, constant.EndpointTypeOpenAIResponse):
		setEndpointAutoConversion(c, modelName, currentEndpoint, constant.EndpointTypeOpenAIResponse, requestPath, "/v1/responses")
		return true
	case currentEndpoint == constant.EndpointTypeOpenAIResponse && supportsEndpoint(supported, constant.EndpointTypeOpenAI):
		setEndpointAutoConversion(c, modelName, currentEndpoint, constant.EndpointTypeOpenAI, requestPath, "/v1/chat/completions")
		return true
	default:
		abortWithOpenAiMessage(
			c,
			http.StatusBadRequest,
			endpointMismatchMessage(modelName, requestPath, currentEndpoint, supported),
			types.ErrorCodeInvalidRequest,
		)
		return false
	}
}

func endpointAutoConvertDisabled(c *gin.Context) bool {
	headerValue := strings.TrimSpace(c.GetHeader(endpointAutoConvertDisableHeader))
	if strings.EqualFold(headerValue, "true") || headerValue == "1" {
		return true
	}
	queryValue := strings.TrimSpace(c.Query("endpoint_auto_convert"))
	return strings.EqualFold(queryValue, "false") || queryValue == "0"
}

func setEndpointAutoConversion(c *gin.Context, modelName string, from constant.EndpointType, to constant.EndpointType, fromPath string, toPath string) {
	common.SetContextKey(c, constant.ContextKeyEndpointAutoConversion, &relaycommon.EndpointAutoConversion{
		Model:        modelName,
		FromEndpoint: from,
		ToEndpoint:   to,
		FromPath:     fromPath,
		ToPath:       toPath,
	})
}

func endpointTypeFromRequestPath(path string) (constant.EndpointType, bool) {
	switch {
	case strings.HasPrefix(path, "/v1/chat/completions"):
		return constant.EndpointTypeOpenAI, true
	case strings.HasPrefix(path, "/v1/responses/compact"):
		return constant.EndpointTypeOpenAIResponse, true
	case strings.HasPrefix(path, "/v1/responses"):
		return constant.EndpointTypeOpenAIResponse, true
	case strings.HasPrefix(path, "/v1/messages"):
		return constant.EndpointTypeAnthropic, true
	case strings.HasPrefix(path, "/v1beta/models/") || strings.HasPrefix(path, "/v1/models/"):
		return constant.EndpointTypeGemini, true
	case strings.HasPrefix(path, "/v1/embeddings") || strings.HasSuffix(path, "embeddings"):
		return constant.EndpointTypeEmbeddings, true
	case strings.HasPrefix(path, "/v1/images/generations") || strings.HasPrefix(path, "/v1/images/edits"):
		return constant.EndpointTypeImageGeneration, true
	case strings.HasPrefix(path, "/v1/rerank"):
		return constant.EndpointTypeJinaRerank, true
	default:
		return "", false
	}
}

func endpointSupportedForRequestPath(current constant.EndpointType, path string, supported []constant.EndpointType) bool {
	if supportsEndpoint(supported, current) {
		return true
	}
	// /v1/responses/compact 在模型端点能力上允许显式配置 compact 类型；请求路径在
	// 自动纠错策略中仍归类为 Responses，避免把普通 Responses-only 模型误判为不支持。
	return strings.HasPrefix(path, "/v1/responses/compact") &&
		supportsEndpoint(supported, constant.EndpointTypeOpenAIResponseCompact)
}

func supportsEndpoint(supported []constant.EndpointType, endpoint constant.EndpointType) bool {
	for _, item := range supported {
		if item == endpoint {
			return true
		}
	}
	return false
}

func endpointMismatchMessage(modelName string, requestPath string, current constant.EndpointType, supported []constant.EndpointType) string {
	supportedNames := make([]string, 0, len(supported))
	suggestion := ""
	for _, endpoint := range supported {
		if endpoint == "" {
			continue
		}
		supportedNames = append(supportedNames, string(endpoint))
		if suggestion == "" {
			if info, ok := common.GetDefaultEndpointInfo(endpoint); ok && strings.TrimSpace(info.Path) != "" {
				suggestion = info.Path
			}
		}
	}
	if len(supportedNames) == 0 {
		supportedNames = append(supportedNames, "unknown")
	}
	if suggestion == "" {
		suggestion = "该模型支持端点对应的路径"
	}
	return fmt.Sprintf(
		"模型 %s 不支持当前请求路径 %s（端点 %s），支持端点：%s。请改用 %s，或通过 %s: true 关闭自动路径纠错后自行处理。",
		modelName,
		requestPath,
		current,
		strings.Join(supportedNames, ", "),
		suggestion,
		endpointAutoConvertDisableHeader,
	)
}
