// Package baidu_v2 实现百度文心一言 V2 版本（基于火山引擎）的渠道适配器。
// V2 版本采用 OpenAI 兼容的 API 格式，大部分响应处理直接委托给 OpenAI 适配器。
// 该文件主要处理 V2 特有的请求 URL 构建、请求头设置和搜索增强功能。
package baidu_v2

// 标准库导入

// 第三方库导入

// 项目内部导入

// Adaptor 是百度文心一言 V2 渠道的适配器结构体。
// 实现了 channel.Adaptor 接口，大部分功能委托给 OpenAI 适配器处理。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式的请求转换为百度 V2 格式。
// 当前未实现，直接返回错误。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式的请求转换为百度 V2 格式。
// 委托给 OpenAI 适配器处理，因为百度 V2 使用 OpenAI 兼容格式。
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := openai.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, req)
}

// ConvertAudioRequest 将音频请求转换为百度 V2 格式。
// 当前未实现。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 将图像请求转换为百度 V2 格式。
// 当前未实现。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化百度 V2 渠道适配器。
// 当前为空实现。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

// GetRequestURL 构建百度 V2 API 的完整请求 URL。
// 根据中继模式（RelayMode）选择不同的 API 端点：
//   - ChatCompletions: /v2/chat/completions
//   - Embeddings: /v2/embeddings
//   - ImagesGenerations: /v2/images/generations
//   - ImagesEdits: /v2/images/edits
//   - Rerank: /v2/rerank
//
// 参数:
//   - info: 中继信息，包含渠道基础 URL 和中继模式
//
// 返回值:
//   - string: 完整的 API 请求 URL
//   - error: 不支持的中继模式时返回错误
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	switch info.RelayMode {
	case constant.RelayModeChatCompletions:
		return fmt.Sprintf("%s/v2/chat/completions", info.ChannelBaseUrl), nil
	case constant.RelayModeEmbeddings:
		return fmt.Sprintf("%s/v2/embeddings", info.ChannelBaseUrl), nil
	case constant.RelayModeImagesGenerations:
		return fmt.Sprintf("%s/v2/images/generations", info.ChannelBaseUrl), nil
	case constant.RelayModeImagesEdits:
		return fmt.Sprintf("%s/v2/images/edits", info.ChannelBaseUrl), nil
	case constant.RelayModeRerank:
		return fmt.Sprintf("%s/v2/rerank", info.ChannelBaseUrl), nil
	default:
	}
	return "", fmt.Errorf("unsupported relay mode: %d", info.RelayMode)
}

// SetupRequestHeader 设置百度 V2 API 请求的 HTTP 头部。
// 从 API Key 中解析 Authorization Token 和可选的 appid。
// API Key 格式: "token" 或 "token|appid"。
//
// 参数:
//   - c: gin 请求上下文
//   - req: 待设置的 HTTP 请求头指针
//   - info: 中继信息，包含 API Key
//
// 返回值:
//   - error: API Key 无效时返回错误
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	keyParts := strings.Split(info.ApiKey, "|")
	if len(keyParts) == 0 || keyParts[0] == "" {
		return errors.New("invalid API key: authorization token is required")
	}
	if len(keyParts) > 1 {
		if keyParts[1] != "" {
			req.Set("appid", keyParts[1])
		}
	}
	req.Set("Authorization", "Bearer "+keyParts[0])
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的对话请求转换为百度 V2 格式。
// 特殊处理：如果模型名称以 "-search" 结尾，会自动启用百度的互联网搜索增强功能，
// 在请求中添加 web_search 配置（启用搜索、引用、追踪）。
//
// 参数:
//   - c: gin 请求上下文
//   - info: 中继信息，包含上游模型名称
//   - request: OpenAI 格式的通用请求
//
// 返回值:
//   - any: 转换后的请求（可能是原始请求或 Map 格式）
//   - error: 请求为 nil 时返回错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if strings.HasSuffix(info.UpstreamModelName, "-search") {
		info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-search")
		request.Model = info.UpstreamModelName
		if len(request.WebSearch) == 0 {
			toMap := request.ToMap()
			toMap["web_search"] = map[string]any{
				"enable":          true,
				"enable_citation": true,
				"enable_trace":    true,
				"enable_status":   false,
			}
			return toMap, nil
		}
		return request, nil
	}
	return request, nil
}

// ConvertRerankRequest 将重排序请求转换为百度 V2 格式。
// 当前未实现。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("not implemented")
}

// ConvertEmbeddingRequest 将向量化请求转换为百度 V2 格式。
// 当前未实现。
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertOpenAIResponsesRequest 将 OpenAI Responses 格式的请求转换为百度 V2 格式。
// 当前未实现。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 执行向百度 V2 API 发送 HTTP 请求。
// 委托给 channel.DoApiRequest 通用请求方法处理。
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理百度 V2 API 的响应。
// 委托给 OpenAI 适配器处理，因为百度 V2 使用 OpenAI 兼容的响应格式。
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	adaptor := openai.Adaptor{}
	usage, err = adaptor.DoResponse(c, resp, info)
	return
}

// GetModelList 返回百度 V2 渠道支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回百度 V2 渠道的名称标识。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
