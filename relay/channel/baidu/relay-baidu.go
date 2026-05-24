// Package baidu 实现百度文心一言 API 的请求转换、响应处理和 Access Token 管理。
// 该文件包含 OpenAI 格式与百度格式之间的互相转换逻辑，
// 以及流式/非流式对话和向量化请求的响应处理器。
package baidu

// 标准库导入

// 第三方库导入

// 项目内部导入

// 参考文档: https://cloud.baidu.com/doc/WENXINWORKSHOP/s/flfmc9do2

// baiduTokenStore 使用 sync.Map 缓存百度 API 的 Access Token。
// Key 为 API Key（格式: client_id|client_secret），Value 为 BaiduAccessToken。
// 避免每次请求都重新获取 Token，减少 API 调用次数。
var baiduTokenStore sync.Map

// requestOpenAI2Baidu 将 OpenAI 格式的通用请求转换为百度文心一言格式。
// 处理逻辑：
//   - 映射 Temperature、TopP、FrequencyPenalty 等参数
//   - 将 system 消息提取为系统提示词，其他消息放入消息列表
//   - 处理 MaxTokens 参数（百度最小值为 2）
//
// 参数:
//   - request: OpenAI 格式的通用请求
//
// 返回值:
//   - *BaiduChatRequest: 转换后的百度对话请求
func requestOpenAI2Baidu(request dto.GeneralOpenAIRequest) *BaiduChatRequest {
	baiduRequest := BaiduChatRequest{
		Temperature:    request.Temperature,
		TopP:           lo.FromPtrOr(request.TopP, 0),
		PenaltyScore:   lo.FromPtrOr(request.FrequencyPenalty, 0),
		Stream:         lo.FromPtrOr(request.Stream, false),
		DisableSearch:  false,
		EnableCitation: false,
		UserId:         request.User,
	}
	if request.GetMaxTokens() != 0 {
		maxTokens := int(request.GetMaxTokens())
		if request.GetMaxTokens() == 1 {
			maxTokens = 2
		}
		baiduRequest.MaxOutputTokens = &maxTokens
	}
	for _, message := range request.Messages {
		if message.Role == "system" {
			baiduRequest.System = message.StringContent()
		} else {
			baiduRequest.Messages = append(baiduRequest.Messages, BaiduMessage{
				Role:    message.Role,
				Content: message.StringContent(),
			})
		}
	}
	return &baiduRequest
}

// responseBaidu2OpenAI 将百度文心一言的非流式响应转换为 OpenAI 格式。
// 将百度的 result 字段映射为 OpenAI 的 message.content，并设置 finish_reason 为 "stop"。
//
// 参数:
//   - response: 百度文心一言的非流式对话响应
//
// 返回值:
//   - *dto.OpenAITextResponse: OpenAI 格式的文本响应
func responseBaidu2OpenAI(response *BaiduChatResponse) *dto.OpenAITextResponse {
	choice := dto.OpenAITextResponseChoice{
		Index: 0,
		Message: dto.Message{
			Role:    "assistant",
			Content: response.Result,
		},
		FinishReason: "stop",
	}
	fullTextResponse := dto.OpenAITextResponse{
		Id:      response.Id,
		Object:  "chat.completion",
		Created: response.Created,
		Choices: []dto.OpenAITextResponseChoice{choice},
		Usage:   response.Usage,
	}
	return &fullTextResponse
}

// streamResponseBaidu2OpenAI 将百度文心一言的流式响应转换为 OpenAI 格式。
// 当百度响应标记 IsEnd 为 true 时，设置 finish_reason 为 "stop"。
//
// 参数:
//   - baiduResponse: 百度文心一言的流式对话响应
//
// 返回值:
//   - *dto.ChatCompletionsStreamResponse: OpenAI 格式的流式响应
func streamResponseBaidu2OpenAI(baiduResponse *BaiduChatStreamResponse) *dto.ChatCompletionsStreamResponse {
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta.SetContentString(baiduResponse.Result)
	if baiduResponse.IsEnd {
		choice.FinishReason = &constant.FinishReasonStop
	}
	response := dto.ChatCompletionsStreamResponse{
		Id:      baiduResponse.Id,
		Object:  "chat.completion.chunk",
		Created: baiduResponse.Created,
		Model:   "ernie-bot",
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response
}

// embeddingRequestOpenAI2Baidu 将 OpenAI 格式的向量化请求转换为百度格式。
// 从通用请求中提取输入文本列表。
//
// 参数:
//   - request: OpenAI 格式的向量化请求
//
// 返回值:
//   - *BaiduEmbeddingRequest: 百度格式的向量化请求
func embeddingRequestOpenAI2Baidu(request dto.EmbeddingRequest) *BaiduEmbeddingRequest {
	return &BaiduEmbeddingRequest{
		Input: request.ParseInput(),
	}
}

// embeddingResponseBaidu2OpenAI 将百度文心一言的向量化响应转换为 OpenAI 格式。
// 遍历百度返回的向量数据列表，逐条映射为 OpenAI 格式的响应项。
//
// 参数:
//   - response: 百度文心一言的向量化响应
//
// 返回值:
//   - *dto.OpenAIEmbeddingResponse: OpenAI 格式的向量化响应
func embeddingResponseBaidu2OpenAI(response *BaiduEmbeddingResponse) *dto.OpenAIEmbeddingResponse {
	openAIEmbeddingResponse := dto.OpenAIEmbeddingResponse{
		Object: "list",
		Data:   make([]dto.OpenAIEmbeddingResponseItem, 0, len(response.Data)),
		Model:  "baidu-embedding",
		Usage:  response.Usage,
	}
	for _, item := range response.Data {
		openAIEmbeddingResponse.Data = append(openAIEmbeddingResponse.Data, dto.OpenAIEmbeddingResponseItem{
			Object:    item.Object,
			Index:     item.Index,
			Embedding: item.Embedding,
		})
	}
	return &openAIEmbeddingResponse
}

// baiduStreamHandler 处理百度文心一言的流式对话响应。
// 使用 StreamScannerHandler 逐行读取 SSE 数据流，将百度格式的流式响应转换为 OpenAI 格式，
// 并通过 helper.ObjectData 实时推送给客户端。同时累计 token 使用量。
//
// 参数:
//   - c: gin 请求上下文
//   - info: 中继信息
//   - resp: 百度上游 API 的 HTTP 响应
//
// 返回值:
//   - *types.NexusTokError: 处理过程中的错误
//   - *dto.Usage: token 使用量统计
func baiduStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NexusTokError, *dto.Usage) {
	usage := &dto.Usage{}
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var baiduResponse BaiduChatStreamResponse
		if err := common.Unmarshal([]byte(data), &baiduResponse); err != nil {
			common.SysLog("error unmarshalling stream response: " + err.Error())
			sr.Error(err)
			return
		}
		if baiduResponse.Usage.TotalTokens != 0 {
			usage.TotalTokens = baiduResponse.Usage.TotalTokens
			usage.PromptTokens = baiduResponse.Usage.PromptTokens
			usage.CompletionTokens = baiduResponse.Usage.TotalTokens - baiduResponse.Usage.PromptTokens
		}
		response := streamResponseBaidu2OpenAI(&baiduResponse)
		if err := helper.ObjectData(c, response); err != nil {
			common.SysLog("error sending stream response: " + err.Error())
			sr.Error(err)
		}
	})
	service.CloseResponseBodyGracefully(resp)
	return nil, usage
}

// baiduHandler 处理百度文心一言的非流式对话响应。
// 读取完整响应体，检查是否包含错误信息，将百度格式的响应转换为 OpenAI 格式后写入客户端。
//
// 参数:
//   - c: gin 请求上下文
//   - info: 中继信息
//   - resp: 百度上游 API 的 HTTP 响应
//
// 返回值:
//   - *types.NexusTokError: 处理过程中的错误
//   - *dto.Usage: token 使用量统计
func baiduHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NexusTokError, *dto.Usage) {
	var baiduResponse BaiduChatResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	service.CloseResponseBodyGracefully(resp)
	err = json.Unmarshal(responseBody, &baiduResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	if baiduResponse.ErrorMsg != "" {
		return types.NewError(fmt.Errorf("%s", baiduResponse.ErrorMsg), types.ErrorCodeBadResponseBody), nil
	}
	fullTextResponse := responseBaidu2OpenAI(&baiduResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}

// baiduEmbeddingHandler 处理百度文心一言的向量化响应。
// 读取完整响应体，检查是否包含错误信息，将百度格式的向量化响应转换为 OpenAI 格式后写入客户端。
//
// 参数:
//   - c: gin 请求上下文
//   - info: 中继信息
//   - resp: 百度上游 API 的 HTTP 响应
//
// 返回值:
//   - *types.NexusTokError: 处理过程中的错误
//   - *dto.Usage: token 使用量统计
func baiduEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NexusTokError, *dto.Usage) {
	var baiduResponse BaiduEmbeddingResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	service.CloseResponseBodyGracefully(resp)
	err = json.Unmarshal(responseBody, &baiduResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	if baiduResponse.ErrorMsg != "" {
		return types.NewError(fmt.Errorf("%s", baiduResponse.ErrorMsg), types.ErrorCodeBadResponseBody), nil
	}
	fullTextResponse := embeddingResponseBaidu2OpenAI(&baiduResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return nil, &fullTextResponse.Usage
}

// getBaiduAccessToken 获取百度 API 的 Access Token。
// 优先从本地缓存（baiduTokenStore）中读取，如果 Token 即将过期（1 小时内），
// 则在后台异步刷新。缓存未命中时同步获取新 Token。
//
// 参数:
//   - apiKey: 百度 API Key，格式为 "client_id|client_secret"
//
// 返回值:
//   - string: Access Token 字符串
//   - error: 获取失败时返回错误
func getBaiduAccessToken(apiKey string) (string, error) {
	if val, ok := baiduTokenStore.Load(apiKey); ok {
		var accessToken BaiduAccessToken
		if accessToken, ok = val.(BaiduAccessToken); ok {
			// soon this will expire
			if time.Now().Add(time.Hour).After(accessToken.ExpiresAt) {
				go func() {
					_, _ = getBaiduAccessTokenHelper(apiKey)
				}()
			}
			return accessToken.AccessToken, nil
		}
	}
	accessToken, err := getBaiduAccessTokenHelper(apiKey)
	if err != nil {
		return "", err
	}
	if accessToken == nil {
		return "", errors.New("getBaiduAccessToken return a nil token")
	}
	return (*accessToken).AccessToken, nil
}

// getBaiduAccessTokenHelper 实际执行百度 Access Token 的获取逻辑。
// 从 API Key 中解析出 client_id 和 client_secret，调用百度 OAuth 2.0 接口获取 Token。
// 获取成功后将 Token 缓存到 baiduTokenStore 中。
//
// 百度 Token 接口: https://aip.baidubce.com/oauth/2.0/token
//
// 参数:
//   - apiKey: 百度 API Key，格式为 "client_id|client_secret"
//
// 返回值:
//   - *BaiduAccessToken: 获取到的 Token 信息
//   - error: 获取失败时返回错误
func getBaiduAccessTokenHelper(apiKey string) (*BaiduAccessToken, error) {
	parts := strings.Split(apiKey, "|")
	if len(parts) != 2 {
		return nil, errors.New("invalid baidu apikey")
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("https://aip.baidubce.com/oauth/2.0/token?grant_type=client_credentials&client_id=%s&client_secret=%s",
		parts[0], parts[1]), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	res, err := service.GetHttpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var accessToken BaiduAccessToken
	err = json.NewDecoder(res.Body).Decode(&accessToken)
	if err != nil {
		return nil, err
	}
	if accessToken.Error != "" {
		return nil, errors.New(accessToken.Error + ": " + accessToken.ErrorDescription)
	}
	if accessToken.AccessToken == "" {
		return nil, errors.New("getBaiduAccessTokenHelper get empty access token")
	}
	accessToken.ExpiresAt = time.Now().Add(time.Duration(accessToken.ExpiresIn) * time.Second)
	baiduTokenStore.Store(apiKey, accessToken)
	return &accessToken, nil
}
