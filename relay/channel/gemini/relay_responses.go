// relay_responses.go 将 Gemini Chat 响应转换为 OpenAI Responses API 响应。
// 请求侧通过 Responses -> Chat -> Gemini 的链路复用现有适配器，响应侧再把 Gemini 的 Chat 结果还原为
// 客户端期望的 Responses JSON 或 SSE 事件。
package gemini

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/helper"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
)

// GeminiResponsesHandler 处理 Gemini 非流式响应，并输出 OpenAI Responses JSON。
func GeminiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	logger.LogDebug(c, "Gemini responses response body: %s", responseBody)

	var geminiResponse dto.GeminiChatResponse
	if err := common.Unmarshal(responseBody, &geminiResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if len(geminiResponse.Candidates) == 0 {
		usage := buildUsageFromGeminiMetadata(geminiResponse.UsageMetadata, info.GetEstimatePromptTokens())
		if geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != nil {
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, fmt.Sprintf("gemini_block_reason=%s", *geminiResponse.PromptFeedback.BlockReason))
			return &usage, types.NewOpenAIError(
				errors.New("request blocked by Gemini API: "+*geminiResponse.PromptFeedback.BlockReason),
				types.ErrorCodePromptBlocked,
				http.StatusBadRequest,
			)
		}
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "gemini_empty_candidates")
		return &usage, types.NewOpenAIError(
			errors.New("empty response from Gemini API"),
			types.ErrorCodeEmptyResponse,
			http.StatusInternalServerError,
		)
	}

	chatResp := responseGeminiChat2OpenAI(c, &geminiResponse)
	chatResp.Model = info.UpstreamModelName
	usage := buildUsageFromGeminiMetadata(geminiResponse.UsageMetadata, info.GetEstimatePromptTokens())
	chatResp.Usage = usage

	responsesResp, responsesUsage, err := service.ChatCompletionsResponseToResponsesResponse(chatResp, helper.GetResponseID(c))
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if responsesUsage == nil || responsesUsage.TotalTokens == 0 {
		responsesResp.Usage = service.UsageFromChatUsage(&usage)
	}

	responseBody, err = common.Marshal(responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return &usage, nil
}

// GeminiResponsesStreamHandler 处理 Gemini 流式响应，并输出 OpenAI Responses SSE。
func GeminiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	responseID := helper.GetResponseID(c)
	created := common.GetTimestamp()
	state := service.NewChatToResponsesStreamState(responseID, info.UpstreamModelName)
	state.Created = created
	finishReason := constant.FinishReasonStop
	toolCallIndexByChoice := make(map[int]map[string]int)
	nextToolCallIndexByChoice := make(map[int]int)
	var streamErr *types.NexusTokError

	sendEvent := func(event service.ChatToResponsesStreamEvent) bool {
		data, err := common.Marshal(event.Payload)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: event.Type}, string(data))
		return true
	}
	sendChunk := func(chunk *dto.ChatCompletionsStreamResponse) bool {
		events, err := service.ChatCompletionsStreamChunkToResponsesEvents(chunk, state)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		for _, event := range events {
			if !sendEvent(event) {
				return false
			}
		}
		return true
	}

	usage, err := geminiStreamHandler(c, info, resp, func(data string, geminiResponse *dto.GeminiChatResponse) bool {
		response, isStop := streamResponseGeminiChat2OpenAI(geminiResponse)
		response.Id = responseID
		response.Created = created
		response.Model = info.UpstreamModelName

		if response.IsToolCall() {
			finishReason = constant.FinishReasonToolCalls
		}
		for choiceIdx := range response.Choices {
			choiceKey := response.Choices[choiceIdx].Index
			for toolIdx := range response.Choices[choiceIdx].Delta.ToolCalls {
				tool := &response.Choices[choiceIdx].Delta.ToolCalls[toolIdx]
				if tool.ID == "" {
					continue
				}
				indexByID := toolCallIndexByChoice[choiceKey]
				if indexByID == nil {
					indexByID = make(map[string]int)
					toolCallIndexByChoice[choiceKey] = indexByID
				}
				if idx, ok := indexByID[tool.ID]; ok {
					tool.SetIndex(idx)
					continue
				}
				idx := nextToolCallIndexByChoice[choiceKey]
				nextToolCallIndexByChoice[choiceKey] = idx + 1
				indexByID[tool.ID] = idx
				tool.SetIndex(idx)
			}
		}

		if !sendChunk(response) {
			return false
		}
		if isStop {
			return sendChunk(helper.GenerateStopResponse(responseID, created, info.UpstreamModelName, finishReason))
		}
		return true
	})
	if err != nil {
		return usage, err
	}
	if streamErr != nil {
		return nil, streamErr
	}

	if usage != nil {
		state.Usage = service.UsageFromChatUsage(usage)
	}
	for _, event := range service.FinalizeChatCompletionsStreamToResponses(state) {
		if !sendEvent(event) {
			return nil, streamErr
		}
	}
	return usage, nil
}
