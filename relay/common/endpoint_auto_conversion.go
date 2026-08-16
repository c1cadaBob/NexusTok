package common

import (
	"strings"

	"github.com/c1cada/NexusTok/constant"

	"github.com/gin-gonic/gin"
)

// EndpointAutoConversion 记录一次请求的端点自动纠错诊断信息。
//
// 该结构只包含模型、端点类型、路径和原始上游错误摘要等非敏感字段。Relay 会先按
// 用户原始 path 请求上游；只有上游明确返回端点/路径不匹配时，中间件才在失败后
// 生成它。后续 Relay 层按该信息选择现有 Chat ↔ Responses 兼容转换链，消费日志
// 则用它帮助管理员定位客户端使用了哪个错误路径以及回退原因。
type EndpointAutoConversion struct {
	Model                 string                `json:"model"`
	FromEndpoint          constant.EndpointType `json:"from_endpoint"`
	ToEndpoint            constant.EndpointType `json:"to_endpoint"`
	FromPath              string                `json:"from_path"`
	ToPath                string                `json:"to_path"`
	TriggeredAfterFailure bool                  `json:"triggered_after_failure,omitempty"`
	OriginalErrorCode     string                `json:"original_error_code,omitempty"`
	OriginalErrorSummary  string                `json:"original_error_summary,omitempty"`
}

// IsChatToResponses 表示客户端用 Chat Completions 路径请求了 Responses-only 模型。
func (info EndpointAutoConversion) IsChatToResponses() bool {
	return info.FromEndpoint == constant.EndpointTypeOpenAI && info.ToEndpoint == constant.EndpointTypeOpenAIResponse
}

// IsResponsesToChat 表示客户端用 Responses 路径请求了 Chat-only 模型。
func (info EndpointAutoConversion) IsResponsesToChat() bool {
	return info.FromEndpoint == constant.EndpointTypeOpenAIResponse && info.ToEndpoint == constant.EndpointTypeOpenAI
}

// AuditMap 返回可直接写入日志 other 的非敏感诊断字段。
func (info EndpointAutoConversion) AuditMap() map[string]interface{} {
	return map[string]interface{}{
		"endpoint_auto_converted": true,
		"from_endpoint":           string(info.FromEndpoint),
		"to_endpoint":             string(info.ToEndpoint),
		"from_path":               info.FromPath,
		"to_path":                 info.ToPath,
		"model":                   info.Model,
		"triggered_after_failure": info.TriggeredAfterFailure,
		"original_path":           info.FromPath,
		"fallback_path":           info.ToPath,
		"original_error_code":     info.OriginalErrorCode,
		"original_error_summary":  info.OriginalErrorSummary,
	}
}

// EndpointAutoConversionChannelTypeAllowed 判断渠道类型是否适合执行 OpenAI Chat ↔ Responses 包装转换。
//
// 端点回退要求上游响应仍保持 OpenAI Chat 或 OpenAI Responses 形态，否则响应处理器会把
// Claude、Gemini 等原生协议误当成 OpenAI 响应解析。这里采用保守白名单，并被选路层和
// relay handler 共同复用，确保不兼容渠道在回退阶段会被跳过而不是在 handler 内硬失败。
func EndpointAutoConversionChannelTypeAllowed(channelType int) bool {
	switch channelType {
	case constant.ChannelTypeOpenAI,
		constant.ChannelTypeOpenAIMax,
		constant.ChannelTypeAzure,
		constant.ChannelTypeXai,
		constant.ChannelTypeCodex,
		constant.ChannelTypeNewAPI,
		constant.ChannelTypeSub2API,
		constant.ChannelTypeAdvancedCustom:
		return true
	default:
		return false
	}
}

// GetEndpointAutoConversion 从 Gin Context 读取端点纠错信息。
func GetEndpointAutoConversion(c *gin.Context) (*EndpointAutoConversion, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(string(constant.ContextKeyEndpointAutoConversion))
	if !ok || value == nil {
		return nil, false
	}
	switch converted := value.(type) {
	case EndpointAutoConversion:
		return &converted, true
	case *EndpointAutoConversion:
		if converted == nil {
			return nil, false
		}
		return converted, true
	default:
		return nil, false
	}
}

// EffectiveRequestPath 返回用于选路和 Advanced Custom 路由匹配的请求路径。
//
// 客户端原始路径不能被直接改写，因为请求解析和返回格式仍要保持客户端形态；
// 但渠道能力筛选必须使用纠错后的目标路径，否则 Responses-only 或 Chat-only 的
// Advanced Custom route 会在分发阶段被错误排除。
func EffectiveRequestPath(c *gin.Context) string {
	if conversion, ok := GetEndpointAutoConversion(c); ok && strings.TrimSpace(conversion.ToPath) != "" {
		return conversion.ToPath
	}
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	return c.Request.URL.Path
}
