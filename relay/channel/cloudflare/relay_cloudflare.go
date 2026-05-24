// Package cloudflare 实现 Cloudflare Workers AI API 的请求转换和响应处理。
// 该文件包含对话请求转换、流式/非流式响应处理和语音转文字响应处理逻辑。
package cloudflare

// 标准库导入

// 第三方库导入

// 项目内部导入

// convertCf2CompletionsRequest 将 OpenAI 格式的通用请求转换为 Cloudflare Workers AI 格式。
// 从 OpenAI 请求中提取 prompt、max_tokens、stream 和 temperature 参数。
//
// 参数:
//   - textRequest: OpenAI 格式的通用请求
//
// 返回值:
//   - *CfRequest: 转换后的 Cloudflare 请求
func convertCf2CompletionsRequest(textRequest dto.GeneralOpenAIRequest) *CfRequest {
	p, _ := textRequest.Prompt.(string)
	return &CfRequest{
		Prompt:      p,
		MaxTokens:   textRequest.GetMaxTokens(),
		Stream:      lo.FromPtrOr(textRequest.Stream, false),
		Temperature: textRequest.Temperature,
	}
}

// cfStreamHandler 处理 Cloudflare Workers AI 的流式对话响应。
// 逐行读取 SSE 数据流，将 Cloudflare 格式的流式响应转换为 OpenAI 格式，
// 并通过 helper.ObjectData 实时推送给客户端。
// 处理完成后，根据响应文本计算 token 使用量，并在需要时发送最终的 usage 响应。
//
// 参数:
//   - c: gin 请求上下文
//   - info: 中继信息
//   - resp: Cloudflare 上游 API 的 HTTP 响应
//
// 返回值:
//   - *types.NexusTokError: 处理过程中的错误
//   - *dto.Usage: token 使用量统计
func cfStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NexusTokError, *dto.Usage) {
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)

	helper.SetEventStreamHeaders(c)
	id := helper.GetResponseID(c)
	var responseText string
	isFirst := true

	for scanner.Scan() {
		data := scanner.Text()
		if len(data) < len("data: ") {
			continue
		}
		data = strings.TrimPrefix(data, "data: ")
		data = strings.TrimSuffix(data, "\r")

		if data == "[DONE]" {
			break
		}

		var response dto.ChatCompletionsStreamResponse
		err := json.Unmarshal([]byte(data), &response)
		if err != nil {
			logger.LogError(c, "error_unmarshalling_stream_response: "+err.Error())
			continue
		}
		for _, choice := range response.Choices {
			choice.Delta.Role = "assistant"
			responseText += choice.Delta.GetContentString()
		}
		response.Id = id
		response.Model = info.UpstreamModelName
		err = helper.ObjectData(c, response)
		if isFirst {
			isFirst = false
			info.FirstResponseTime = time.Now()
		}
		if err != nil {
			logger.LogError(c, "error_rendering_stream_response: "+err.Error())
		}
	}

	if err := scanner.Err(); err != nil {
		logger.LogError(c, "error_scanning_stream_response: "+err.Error())
	}
	usage := service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens())
	if info.ShouldIncludeUsage {
		response := helper.GenerateFinalUsageResponse(id, info.StartTime.Unix(), info.UpstreamModelName, *usage)
		err := helper.ObjectData(c, response)
		if err != nil {
			logger.LogError(c, "error_rendering_final_usage_response: "+err.Error())
		}
	}
	helper.Done(c)

	service.CloseResponseBodyGracefully(resp)

	return nil, usage
}

// cfHandler 处理 Cloudflare Workers AI 的非流式对话响应。
// 读取完整响应体，将 Cloudflare 格式的响应转换为 OpenAI 格式，
// 根据响应文本计算 token 使用量后写入客户端。
//
// 参数:
//   - c: gin 请求上下文
//   - info: 中继信息
//   - resp: Cloudflare 上游 API 的 HTTP 响应
//
// 返回值:
//   - *types.NexusTokError: 处理过程中的错误
//   - *dto.Usage: token 使用量统计
func cfHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NexusTokError, *dto.Usage) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	service.CloseResponseBodyGracefully(resp)
	var response dto.TextResponse
	err = json.Unmarshal(responseBody, &response)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	response.Model = info.UpstreamModelName
	var responseText string
	for _, choice := range response.Choices {
		responseText += choice.Message.StringContent()
	}
	usage := service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens())
	response.Usage = *usage
	response.Id = helper.GetResponseID(c)
	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)
	return nil, usage
}

// cfSTTHandler 处理 Cloudflare Workers AI 的语音转文字（STT）响应。
// 读取 Cloudflare 的语音识别响应，提取识别文本，转换为通用的 AudioResponse 格式后写入客户端。
// 同时根据识别文本计算 token 使用量。
//
// 参数:
//   - c: gin 请求上下文
//   - info: 中继信息
//   - resp: Cloudflare 上游 API 的 HTTP 响应
//
// 返回值:
//   - *types.NexusTokError: 处理过程中的错误
//   - *dto.Usage: token 使用量统计
func cfSTTHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*types.NexusTokError, *dto.Usage) {
	var cfResp CfAudioResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	service.CloseResponseBodyGracefully(resp)
	err = json.Unmarshal(responseBody, &cfResp)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}

	audioResp := &dto.AudioResponse{
		Text: cfResp.Result.Text,
	}

	jsonResponse, err := json.Marshal(audioResp)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)

	usage := service.ResponseText2Usage(c, cfResp.Result.Text, info.UpstreamModelName, info.GetEstimatePromptTokens())
	return nil, usage
}
