// Package mokaai 实现 Moka AI（百度文心/ERNIE 相关）通道的适配器。
// Moka AI 基于百度智能云平台，提供文本嵌入（Embedding）等能力。
// 该适配器将 OpenAI 格式请求转换为 Moka AI 格式，并处理嵌入响应。
// 文本对话功能部分实现，其他功能（图片、音频等）暂未实现。
package mokaai

// 标准库导入
import (
	"errors"    // 错误创建
	"fmt"       // 格式化字符串
	"io"        // IO 读写接口
	"net/http"  // HTTP 客户端和响应处理
	"strings"   // 字符串操作

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"                     // 数据传输对象定义
	"github.com/c1cada/NexusTok/relay/channel"            // 通道通用工具函数
	relaycommon "github.com/c1cada/NexusTok/relay/common"   // Relay 通用模块
	"github.com/c1cada/NexusTok/relay/constant"           // Relay 常量定义
	"github.com/c1cada/NexusTok/types"                    // 公共类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin" // Gin Web 框架
)

// Adaptor Moka AI 通道适配器。
// 支持 Embedding（向量嵌入）请求模式，对话模式部分实现。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式请求转换为 Moka AI 格式（未实现）。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式请求转换为 Moka AI 格式（未实现）。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
	return nil, nil
}

// ConvertAudioRequest 音频请求转换（Moka AI 不支持）。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 图片请求转换（Moka AI 不支持）。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertEmbeddingRequest 将 Embedding 请求透传给 Moka AI。
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息
//   - request: OpenAI 格式的 Embedding 请求
// 返回:
//   - any: 原始请求（直接透传）
//   - error: 错误信息（当前始终返回 nil）
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return request, nil
}

// Init 初始化 Moka AI 适配器（无特殊初始化操作）。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {

}

// GetRequestURL 根据上游模型名称构建 Moka AI 的请求 URL。
// - m3e 系列模型: 使用 embeddings 路径
// - 其他模型: 使用 chat/ 路径（对话模式）
// 参考文档: https://cloud.baidu.com/doc/WENXINWORKSHOP/s/clntwmv7t
// 参数:
//   - info: Relay 信息，包含渠道基础 URL 和上游模型名称
// 返回:
//   - string: 完整的 API 请求 URL
//   - error: 错误信息（当前始终返回 nil）
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// https://cloud.baidu.com/doc/WENXINWORKSHOP/s/clntwmv7t
	suffix := "chat/"
	if strings.HasPrefix(info.UpstreamModelName, "m3e") {
		suffix = "embeddings"
	}
	fullRequestURL := fmt.Sprintf("%s/%s", info.ChannelBaseUrl, suffix)
	return fullRequestURL, nil
}

// SetupRequestHeader 设置 Moka AI API 请求头。
// 使用标准 API 请求头设置，并添加 Bearer Token 认证。
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", fmt.Sprintf("Bearer %s", info.ApiKey))
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式请求转换为 Moka AI 格式。
// 根据 RelayMode 分发处理：
// - Embedding 模式: 调用 embeddingRequestOpenAI2Moka 转换格式
// - 其他模式: 返回未实现错误
// 参数:
//   - c: Gin 上下文
//   - info: Relay 信息，包含请求模式
//   - request: OpenAI 格式的通用请求对象
// 返回:
//   - any: Moka AI 格式的请求体
//   - error: 错误信息
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	switch info.RelayMode {
	case constant.RelayModeEmbeddings:
		baiduEmbeddingRequest := embeddingRequestOpenAI2Moka(*request)
		return baiduEmbeddingRequest, nil
	default:
		return nil, errors.New("not implemented")
	}
}

// ConvertRerankRequest Rerank 请求透传。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertOpenAIResponsesRequest OpenAI Responses API 请求转换（未实现）。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 执行向 Moka AI API 的 HTTP 请求。
// 委托给通道通用的 DoApiRequest 函数处理。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理 Moka AI API 的响应。
// 根据 RelayMode 分发到不同的处理器：
// - Embedding 模式: 使用 mokaEmbeddingHandler 处理嵌入响应
// - 其他模式: 暂未实现
// 参数:
//   - c: Gin 上下文
//   - resp: Moka AI API 返回的 HTTP 响应
//   - info: Relay 信息
// 返回:
//   - usage: token 使用量信息
//   - err: 错误信息
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {

	switch info.RelayMode {
	case constant.RelayModeEmbeddings:
		return mokaEmbeddingHandler(c, info, resp)
	default:
		// err, usage = mokaHandler(c, resp)

	}
	return
}

// GetModelList 返回 Moka AI 支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回通道名称。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
