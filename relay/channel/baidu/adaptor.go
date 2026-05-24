// Package baidu 实现百度文心一言（ERNIE）AI 平台的渠道适配器。
// 该文件定义了 Adaptor 结构体及其方法，负责请求 URL 构建、请求头设置、
// 请求格式转换和响应处理等核心适配逻辑。
package baidu

// 标准库导入

// 第三方库导入

// 项目内部导入

// Adaptor 是百度文心一言渠道的适配器结构体。
// 实现了 channel.Adaptor 接口，提供从 OpenAI 格式到百度文心一言格式的请求转换能力。
type Adaptor struct {
}

// ConvertGeminiRequest 将 Gemini 格式的请求转换为百度格式。
// 当前未实现，直接返回错误。
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertClaudeRequest 将 Claude 格式的请求转换为百度格式。
// 当前未实现，直接 panic（应改为返回错误）。
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	//TODO implement me
	panic("implement me")
	return nil, nil
}

// ConvertAudioRequest 将音频请求转换为百度格式。
// 当前未实现，百度文心一言暂不支持音频请求。
func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// ConvertImageRequest 将图像请求转换为百度格式。
// 当前未实现，百度文心一言暂不支持图像生成请求。
func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// Init 初始化百度渠道适配器。
// 当前为空实现，百度渠道无需特殊的初始化逻辑。
func (a *Adaptor) Init(info *relaycommon.RelayInfo) {

}

// GetRequestURL 构建百度文心一言 API 的完整请求 URL。
// 根据模型名称确定 API 路径后缀（对话/向量化），并拼接 access_token 认证参数。
//
// 百度文心一言 API 的 URL 格式为:
//   {baseUrl}/rpc/2.0/ai_custom/v1/wenxinworkshop/{suffix}?access_token={token}
//
// 参数:
//   - info: 中继信息，包含渠道基础 URL、模型名称和 API Key
//
// 返回值:
//   - string: 完整的 API 请求 URL
//   - error: 获取 access_token 失败时返回错误
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// https://cloud.baidu.com/doc/WENXINWORKSHOP/s/clntwmv7t
	suffix := "chat/"
	if strings.HasPrefix(info.UpstreamModelName, "Embedding") {
		suffix = "embeddings/"
	}
	if strings.HasPrefix(info.UpstreamModelName, "bge-large") {
		suffix = "embeddings/"
	}
	if strings.HasPrefix(info.UpstreamModelName, "tao-8k") {
		suffix = "embeddings/"
	}
	switch info.UpstreamModelName {
	case "ERNIE-4.0":
		suffix += "completions_pro"
	case "ERNIE-Bot-4":
		suffix += "completions_pro"
	case "ERNIE-Bot":
		suffix += "completions"
	case "ERNIE-Bot-turbo":
		suffix += "eb-instant"
	case "ERNIE-Speed":
		suffix += "ernie_speed"
	case "ERNIE-4.0-8K":
		suffix += "completions_pro"
	case "ERNIE-3.5-8K":
		suffix += "completions"
	case "ERNIE-3.5-8K-0205":
		suffix += "ernie-3.5-8k-0205"
	case "ERNIE-3.5-8K-1222":
		suffix += "ernie-3.5-8k-1222"
	case "ERNIE-Bot-8K":
		suffix += "ernie_bot_8k"
	case "ERNIE-3.5-4K-0205":
		suffix += "ernie-3.5-4k-0205"
	case "ERNIE-Speed-8K":
		suffix += "ernie_speed"
	case "ERNIE-Speed-128K":
		suffix += "ernie-speed-128k"
	case "ERNIE-Lite-8K-0922":
		suffix += "eb-instant"
	case "ERNIE-Lite-8K-0308":
		suffix += "ernie-lite-8k"
	case "ERNIE-Tiny-8K":
		suffix += "ernie-tiny-8k"
	case "BLOOMZ-7B":
		suffix += "bloomz_7b1"
	case "Embedding-V1":
		suffix += "embedding-v1"
	case "bge-large-zh":
		suffix += "bge_large_zh"
	case "bge-large-en":
		suffix += "bge_large_en"
	case "tao-8k":
		suffix += "tao_8k"
	default:
		suffix += strings.ToLower(info.UpstreamModelName)
	}
	fullRequestURL := fmt.Sprintf("%s/rpc/2.0/ai_custom/v1/wenxinworkshop/%s", info.ChannelBaseUrl, suffix)
	var accessToken string
	var err error
	if accessToken, err = getBaiduAccessToken(info.ApiKey); err != nil {
		return "", err
	}
	fullRequestURL += "?access_token=" + accessToken
	return fullRequestURL, nil
}

// SetupRequestHeader 设置百度文心一言 API 请求的 HTTP 头部。
// 设置通用 API 请求头和 Authorization Bearer Token 认证头。
//
// 参数:
//   - c: gin 请求上下文
//   - req: 待设置的 HTTP 请求头指针
//   - info: 中继信息，包含 API Key
//
// 返回值:
//   - error: 始终返回 nil
func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// ConvertOpenAIRequest 将 OpenAI 格式的对话请求转换为百度文心一言格式。
// 调用 requestOpenAI2Baidu 进行格式转换。
//
// 参数:
//   - c: gin 请求上下文
//   - info: 中继信息
//   - request: OpenAI 格式的通用请求
//
// 返回值:
//   - any: 转换后的百度格式请求
//   - error: 请求为 nil 时返回错误
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	switch info.RelayMode {
	default:
		baiduRequest := requestOpenAI2Baidu(*request)
		return baiduRequest, nil
	}
}

// ConvertRerankRequest 将重排序请求转换为百度格式。
// 百度文心一言不支持重排序功能，始终返回 nil。
func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

// ConvertEmbeddingRequest 将 OpenAI 格式的向量化请求转换为百度格式。
// 调用 embeddingRequestOpenAI2Baidu 进行格式转换。
//
// 参数:
//   - c: gin 请求上下文
//   - info: 中继信息
//   - request: OpenAI 格式的向量化请求
//
// 返回值:
//   - any: 转换后的百度格式向量化请求
//   - error: 始终返回 nil
func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	baiduEmbeddingRequest := embeddingRequestOpenAI2Baidu(request)
	return baiduEmbeddingRequest, nil
}

// ConvertOpenAIResponsesRequest 将 OpenAI Responses 格式的请求转换为百度格式。
// 当前未实现，百度文心一言暂不支持 OpenAI Responses API。
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

// DoRequest 执行向百度文心一言 API 发送 HTTP 请求。
// 委托给 channel.DoApiRequest 通用请求方法处理。
//
// 参数:
//   - c: gin 请求上下文
//   - info: 中继信息
//   - requestBody: 请求体的 io.Reader
//
// 返回值:
//   - any: 原始 HTTP 响应
//   - error: 请求过程中的错误
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// DoResponse 处理百度文心一言 API 的响应。
// 根据是否为流式请求和中继模式，分别调用不同的处理器：
//   - 流式请求: baiduStreamHandler
//   - 向量化请求: baiduEmbeddingHandler
//   - 普通对话请求: baiduHandler
//
// 参数:
//   - c: gin 请求上下文
//   - resp: 百度上游 API 的 HTTP 响应
//   - info: 中继信息
//
// 返回值:
//   - usage: token 使用量统计
//   - err: 处理过程中的错误
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError) {
	if info.IsStream {
		err, usage = baiduStreamHandler(c, info, resp)
	} else {
		switch info.RelayMode {
		case constant.RelayModeEmbeddings:
			err, usage = baiduEmbeddingHandler(c, info, resp)
		default:
			err, usage = baiduHandler(c, info, resp)
		}
	}
	return
}

// GetModelList 返回百度文心一言渠道支持的模型列表。
func (a *Adaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回百度文心一言渠道的名称标识。
func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
