// Package relay - claude_handler.go
// 该文件实现了 Claude API 的中继处理
// Claude 是 Anthropic 公司的 AI 模型，使用 Messages API 格式
//
// Claude API 特点：
// - 使用 Messages API（/v1/messages）
// - 支持流式响应（SSE）
// - 支持多模态（文本、图像）
// - 支持工具调用（Function Calling）
// - 支持 Thinking 扩展（扩展思考）
package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"                    // 公共工具包
	"github.com/c1cada/NexusTok/constant"                   // 常量定义
	"github.com/c1cada/NexusTok/dto"                        // 数据传输对象
	relaycommon "github.com/c1cada/NexusTok/relay/common"   // 中继公共包
	"github.com/c1cada/NexusTok/relay/helper"               // 中继辅助函数
	"github.com/c1cada/NexusTok/service"                    // 服务层
	"github.com/c1cada/NexusTok/setting/model_setting"      // 模型设置
	"github.com/c1cada/NexusTok/setting/reasoning"          // 推理设置
	"github.com/c1cada/NexusTok/types"                      // 类型定义

	"github.com/gin-gonic/gin" // Gin 框架
)

// ClaudeHelper Claude API 中继处理函数
// 处理 Claude Messages API 请求，包括：
// 1. 请求验证和类型转换
// 2. 模型映射
// 3. 适配器初始化
// 4. MaxTokens 默认值设置
// 5. Thinking 扩展处理
// 6. 请求转发和响应处理
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//
// 返回值：
//   - newAPIError: 错误信息，成功返回 nil
func ClaudeHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NexusTokError) {
	// 初始化渠道元数据
	info.InitChannelMeta(c)

	// 类型断言：获取 Claude 请求
	claudeReq, ok := info.Request.(*dto.ClaudeRequest)
	if !ok {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid request type, expected *dto.ClaudeRequest, got %T", info.Request),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	// 深拷贝请求，避免修改原始请求
	request, err := common.DeepCopy(claudeReq)
	if err != nil {
		return types.NewError(
			fmt.Errorf("failed to copy request to ClaudeRequest: %w", err),
			types.ErrorCodeInvalidRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	// 模型映射：将用户请求的模型映射到实际的上游模型
	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	// 获取 API 适配器
	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(
			fmt.Errorf("invalid api type: %d", info.ApiType),
			types.ErrorCodeInvalidApiType,
			types.ErrOptionWithSkipRetry(),
		)
	}

	// 初始化适配器
	adaptor.Init(info)

	// 设置 MaxTokens 默认值
	if request.MaxTokens == nil || *request.MaxTokens == 0 {
		defaultMaxTokens := uint(model_setting.GetClaudeSettings().GetDefaultMaxTokens(request.Model))
		request.MaxTokens = &defaultMaxTokens
	}

	// 处理 Thinking 扩展（扩展思考）
	// 检查是否是带有 effort 后缀的模型（如 claude-opus-4-6-high）
	if baseModel, effortLevel, ok := reasoning.TrimEffortSuffix(request.Model); ok && effortLevel != "" &&
		(strings.HasPrefix(request.Model, "claude-opus-4-6") || strings.HasPrefix(request.Model, "claude-opus-4-7")) {
		// 设置基础模型名称
		request.Model = baseModel

		// 启用 Thinking 扩展（自适应模式）
		request.Thinking = &dto.Thinking{
			Type: "adaptive",
		}

		// 设置输出配置（effort 级别）
		request.OutputConfig = json.RawMessage(fmt.Sprintf(`{"effort":"%s"}`, effortLevel))

		// Opus 4.7 特殊处理
		if strings.HasPrefix(request.Model, "claude-opus-4-7") {
			// Opus 4.7 不接受非默认的 temperature/top_p/top_k，会返回 400 错误
			// 默认显示为 "omitted"；恢复 4.6 的可见摘要
			request.Thinking.Display = "summarized"
			request.Temperature = nil
			request.TopP = nil
			request.TopK = nil
		} else {
			// Opus 4.6 设置 temperature 为 1.0
			request.Temperature = common.GetPointer[float64](1.0)
		}

		// 更新上游模型名称
		info.UpstreamModelName = request.Model
	} else if model_setting.GetClaudeSettings().ThinkingAdapterEnabled &&
		strings.HasSuffix(request.Model, "-thinking") {
		// 处理 -thinking 后缀的模型（Claude Thinking 适配器）
		if request.Thinking == nil {
			baseModel := strings.TrimSuffix(request.Model, "-thinking")

			if strings.HasPrefix(baseModel, "claude-opus-4-7") {
				// Opus 4.7 不接受 thinking.type="enabled"；使用自适应模式，高 effort
				request.Thinking = &dto.Thinking{Type: "adaptive", Display: "summarized"}
				request.OutputConfig = json.RawMessage(`{"effort":"high"}`)
				request.Temperature = nil
				request.TopP = nil
				request.TopK = nil
			} else {
				// 因为BudgetTokens 必须大于1024
				if request.MaxTokens == nil || *request.MaxTokens < 1280 {
					request.MaxTokens = common.GetPointer[uint](1280)
				}

				// BudgetTokens 为 max_tokens 的 80%
				request.Thinking = &dto.Thinking{
					Type:         "enabled",
					BudgetTokens: common.GetPointer[int](int(float64(*request.MaxTokens) * model_setting.GetClaudeSettings().ThinkingAdapterBudgetTokensPercentage)),
				}
				// TODO: 临时处理
				// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations-when-using-extended-thinking
				request.Temperature = common.GetPointer[float64](1.0)
			}
		}
		if !model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
			request.Model = strings.TrimSuffix(request.Model, "-thinking")
		}
		info.UpstreamModelName = request.Model
	}

	if info.ChannelSetting.SystemPrompt != "" {
		if request.System == nil {
			request.SetStringSystem(info.ChannelSetting.SystemPrompt)
		} else if info.ChannelSetting.SystemPromptOverride {
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
			if request.IsStringSystem() {
				existing := strings.TrimSpace(request.GetStringSystem())
				if existing == "" {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt)
				} else {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt + "\n" + existing)
				}
			} else {
				systemContents := request.ParseSystem()
				newSystem := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
				newSystem.SetText(info.ChannelSetting.SystemPrompt)
				if len(systemContents) == 0 {
					request.System = []dto.ClaudeMediaMessage{newSystem}
				} else {
					request.System = append([]dto.ClaudeMediaMessage{newSystem}, systemContents...)
				}
			}
		}
	}

	if !model_setting.GetGlobalSettings().PassThroughRequestEnabled &&
		!info.ChannelSetting.PassThroughBodyEnabled &&
		service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		openAIRequest, convErr := service.ClaudeToOpenAIRequest(*request, info)
		if convErr != nil {
			return types.NewError(convErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		usage, newApiErr := chatCompletionsViaResponses(c, info, adaptor, openAIRequest)
		if newApiErr != nil {
			return newApiErr
		}

		service.PostTextConsumeQuota(c, info, usage, nil)
		return nil
	}

	var requestBody io.Reader
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		requestBody = common.ReaderOnly(storage)
	} else {
		convertedRequest, err := adaptor.ConvertClaudeRequest(c, info, request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for Claude API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
			if err != nil {
				return newAPIErrorFromParamOverride(err)
			}
		}

		// 应用请求规则覆写
		if common.DebugEnabled {
			println("requestBody: ", string(jsonData))
		}
		requestBody = bytes.NewBuffer(jsonData)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	//log.Printf("usage: %v", usage)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
	return nil
}
