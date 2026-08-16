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

// prepareEndpointAutoConversion 保留在分发前调用，但不再提前启用端点安全纠错。
//
// 端点纠错必须先让用户原始 path 真实请求一次；只有上游明确返回端点/路径不匹配
// 时，Relay 重试层才会调用 TryPrepareEndpointAutoConversionAfterFailure 设置兼容
// 目标 path。这里返回 true 是为了保留调用点和旧测试入口，避免再次引入预选路改写。
func prepareEndpointAutoConversion(c *gin.Context, modelRequest *ModelRequest) bool {
	_ = c
	_ = modelRequest
	return true
}

// TryPrepareEndpointAutoConversionAfterFailure 在原始请求 path 失败后尝试启用安全兼容回退。
//
// 返回 true 表示本次请求已经挂上 EndpointAutoConversion，调用方应当把本次错误当作
// “可回退的端点不匹配”处理：不累计密钥失败、不禁用渠道，并立即使用兼容 path 重试。
// 它只覆盖 OpenAI Chat Completions ↔ OpenAI Responses，且必须同时满足：
//   - 全局和请求级开关均允许自动纠错；
//   - 当前请求尚未做过端点回退；
//   - 上游错误明确像端点/路径不匹配；
//   - 模型能力中存在对应的安全目标端点。
func TryPrepareEndpointAutoConversionAfterFailure(c *gin.Context, modelName string, err *types.NexusTokError) bool {
	if c == nil || c.Request == nil || c.Request.URL == nil || err == nil {
		return false
	}
	if !model_setting.GetGlobalSettings().EndpointAutoConversionEnabled || endpointAutoConvertDisabled(c) {
		return false
	}
	if _, exists := relaycommon.GetEndpointAutoConversion(c); exists {
		return false
	}
	if !isEndpointMismatchError(err) {
		return false
	}
	requestPath := c.Request.URL.Path
	currentEndpoint, ok := endpointTypeFromRequestPath(requestPath)
	if !ok {
		return false
	}

	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}

	supported := model.GetModelSupportEndpointTypes(modelName)
	if len(supported) == 0 && strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		supported = model.GetModelSupportEndpointTypes(strings.TrimSuffix(modelName, ratio_setting.CompactModelSuffix))
	}
	if len(supported) == 0 {
		return false
	}

	switch {
	case currentEndpoint == constant.EndpointTypeOpenAI && supportsEndpoint(supported, constant.EndpointTypeOpenAIResponse):
		setEndpointAutoConversionAfterFailure(c, modelName, currentEndpoint, constant.EndpointTypeOpenAIResponse, requestPath, "/v1/responses", err)
		return true
	case currentEndpoint == constant.EndpointTypeOpenAIResponse && supportsEndpoint(supported, constant.EndpointTypeOpenAI):
		setEndpointAutoConversionAfterFailure(c, modelName, currentEndpoint, constant.EndpointTypeOpenAI, requestPath, "/v1/chat/completions", err)
		return true
	default:
		return false
	}
}

func isEndpointMismatchError(err *types.NexusTokError) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error() + " " + string(err.GetErrorCode())))
	if text == "" {
		return false
	}
	strongKeywords := []string{
		"unsupported endpoint",
		"unsupported_endpoint",
		"endpoint_not_allowed",
		"endpoint not allowed",
		"invalid endpoint",
		"wrong endpoint",
		"endpoint mismatch",
		"unsupported path",
		"unsupported route",
		"route not found",
		"no route",
		"path not found",
		"model not support",
		"model does not support",
		"model is not supported on this endpoint",
		"model is not supported by this endpoint",
		"not support this endpoint",
		"does not support this endpoint",
		"/v1/chat/completions",
		"/v1/responses",
		"端点",
		"端點",
		"路径",
		"路徑",
		"路由",
		"不支持",
	}
	if containsEndpointAutoConversionKeyword(text, strongKeywords) {
		return true
	}

	// 部分 OpenAI 兼容上游只返回 404/405 或 model_not_found，不包含明确 path
	// 文案。只有这些更窄的状态/错误码才允许兜底判断，避免余额、鉴权、限流等
	// 与端点无关的问题触发二次请求。
	if err.StatusCode == http.StatusNotFound || err.StatusCode == http.StatusMethodNotAllowed {
		return !containsEndpointAutoConversionKeyword(text, endpointAutoConversionNonMismatchKeywords())
	}
	if err.StatusCode == http.StatusBadRequest && err.GetErrorCode() == types.ErrorCodeModelNotFound {
		return !containsEndpointAutoConversionKeyword(text, endpointAutoConversionNonMismatchKeywords())
	}
	return false
}

func containsEndpointAutoConversionKeyword(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func endpointAutoConversionNonMismatchKeywords() []string {
	return []string{
		"unauthorized",
		"forbidden",
		"invalid key",
		"invalid_key",
		"incorrect api key",
		"authentication",
		"permission",
		"quota",
		"insufficient",
		"balance",
		"billing",
		"rate limit",
		"rate_limit",
		"too many requests",
		"overload",
		"timeout",
		"timed out",
		"connection refused",
		"connection reset",
		"network",
		"余额",
		"配额",
		"额度",
		"鉴权",
		"认证",
		"权限",
		"限流",
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

func setEndpointAutoConversionAfterFailure(c *gin.Context, modelName string, from constant.EndpointType, to constant.EndpointType, fromPath string, toPath string, err *types.NexusTokError) {
	summary := ""
	code := ""
	if err != nil {
		code = string(err.GetErrorCode())
		summary = common.MaskSensitiveInfo(err.Error())
		if len([]rune(summary)) > 240 {
			summary = string([]rune(summary)[:240]) + "..."
		}
	}
	common.SetContextKey(c, constant.ContextKeyEndpointAutoConversion, &relaycommon.EndpointAutoConversion{
		Model:                 modelName,
		FromEndpoint:          from,
		ToEndpoint:            to,
		FromPath:              fromPath,
		ToPath:                toPath,
		TriggeredAfterFailure: true,
		OriginalErrorCode:     code,
		OriginalErrorSummary:  summary,
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
