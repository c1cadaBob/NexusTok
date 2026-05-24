// Package relay - audio_handler.go
// 该文件实现了音频请求（TTS/STT）的中继处理逻辑
//
// 处理流程：
// 1. 初始化渠道元数据
// 2. 验证并深拷贝请求
// 3. 应用模型映射
// 4. 获取并初始化对应的适配器
// 5. 转换请求格式并发送到上游
// 6. 处理响应并计算配额消耗
package relay

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/relay/helper"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// AudioHelper 音频请求中继处理函数
//
// 处理 TTS（文本转语音）和 STT（语音转文本）请求
//
// 处理流程：
// 1. 初始化渠道元数据
// 2. 验证请求类型并深拷贝
// 3. 应用模型映射
// 4. 获取并初始化对应的 API 适配器
// 5. 转换请求格式并发送到上游
// 6. 处理响应，根据是否有音频 token 分别计费
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息（包含渠道配置、请求信息等）
//
// 返回值：
//   - newAPIError: 错误信息，nil 表示成功
func AudioHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NexusTokError) {
	// 初始化渠道元数据
	info.InitChannelMeta(c)

	// 验证请求类型
	audioReq, ok := info.Request.(*dto.AudioRequest)
	if !ok {
		return types.NewError(errors.New("invalid request type"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	// 深拷贝请求，避免修改原始请求
	request, err := common.DeepCopy(audioReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to AudioRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	// 应用模型映射
	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	// 获取对应的 API 适配器
	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	// 转换请求格式
	ioReader, err := adaptor.ConvertAudioRequest(c, info, *request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	// 发送请求到上游
	resp, err := adaptor.DoRequest(c, info, ioReader)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	statusCodeMappingStr := c.GetString("status_code_mapping")

	// 检查上游响应状态码
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// 根据状态码映射重置错误状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	// 处理响应并获取用量信息
	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// 根据状态码映射重置错误状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	// 根据是否有音频 token 分别进行配额计费
	if usage.(*dto.Usage).CompletionTokenDetails.AudioTokens > 0 || usage.(*dto.Usage).PromptTokensDetails.AudioTokens > 0 {
		service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), "")
	} else {
		service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
	}

	return nil
}
