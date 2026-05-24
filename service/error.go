// error.go - 错误处理与包装服务
// 本文件提供各类 API 错误的包装、转换和处理功能。
// 支持 Midjourney、Claude、Task 等多种错误格式的统一处理，
// 包括错误状态码重映射、上游错误解析等。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/types"
)

// MidjourneyErrorWrapper 将错误码和描述包装为 Midjourney 响应结构体。
// 用于 Midjourney API 的错误响应构建。
// 参数:
//   - code: Midjourney 错误码
//   - desc: 错误描述信息
// 返回值:
//   - *dto.MidjourneyResponse: 包装后的 Midjourney 响应对象
func MidjourneyErrorWrapper(code int, desc string) *dto.MidjourneyResponse {
	return &dto.MidjourneyResponse{
		Code:        code,
		Description: desc,
	}
}

// MidjourneyErrorWithStatusCodeWrapper 将错误码、描述和 HTTP 状态码包装为带状态码的 Midjourney 响应结构体。
// 用于需要同时返回 HTTP 状态码的 Midjourney 错误场景。
// 参数:
//   - code: Midjourney 错误码
//   - desc: 错误描述信息
//   - statusCode: HTTP 状态码
// 返回值:
//   - *dto.MidjourneyResponseWithStatusCode: 包装后的带状态码的 Midjourney 响应对象
func MidjourneyErrorWithStatusCodeWrapper(code int, desc string, statusCode int) *dto.MidjourneyResponseWithStatusCode {
	return &dto.MidjourneyResponseWithStatusCode{
		StatusCode: statusCode,
		Response:   *MidjourneyErrorWrapper(code, desc),
	}
}

//// OpenAIErrorWrapper wraps an error into an OpenAIErrorWithStatusCode
//func OpenAIErrorWrapper(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	text := err.Error()
//	lowerText := strings.ToLower(text)
//	if !strings.HasPrefix(lowerText, "get file base64 from url") && !strings.HasPrefix(lowerText, "mime type is not supported") {
//		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
//			common.SysLog(fmt.Sprintf("error: %s", text))
//			text = "请求上游地址失败"
//		}
//	}
//	openAIError := dto.OpenAIError{
//		Message: text,
//		Type:    "nexustok_error",
//		Code:    code,
//	}
//	return &dto.OpenAIErrorWithStatusCode{
//		Error:      openAIError,
//		StatusCode: statusCode,
//	}
//}
//
//func OpenAIErrorWrapperLocal(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	openaiErr := OpenAIErrorWrapper(err, code, statusCode)
//	openaiErr.LocalError = true
//	return openaiErr
//}

// ClaudeErrorWrapper 将错误包装为 Claude 格式的错误响应。
// 当错误消息包含网络相关关键词（post、dial、http）时，会遮盖为通用错误消息以避免泄露内部细节。
// 参数:
//   - err: 原始错误对象
//   - code: 错误码
//   - statusCode: HTTP 状态码
// 返回值:
//   - *dto.ClaudeErrorWithStatusCode: Claude 格式的带状态码错误响应
func ClaudeErrorWrapper(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if !strings.HasPrefix(lowerText, "get file base64 from url") {
		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
			common.SysLog(fmt.Sprintf("error: %s", text))
			text = "请求上游地址失败"
		}
	}
	claudeError := types.ClaudeError{
		Message: text,
		Type:    "nexustok_error",
	}
	return &dto.ClaudeErrorWithStatusCode{
		Error:      claudeError,
		StatusCode: statusCode,
	}
}

// ClaudeErrorWrapperLocal 将错误包装为 Claude 格式的本地错误响应。
// 与 ClaudeErrorWrapper 不同，此函数标记错误为本地错误（LocalError = true），
// 用于区分本地生成的错误和上游返回的错误。
// 参数:
//   - err: 原始错误对象
//   - code: 错误码
//   - statusCode: HTTP 状态码
// 返回值:
//   - *dto.ClaudeErrorWithStatusCode: 标记为本地错误的 Claude 格式错误响应
func ClaudeErrorWrapperLocal(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	claudeErr := ClaudeErrorWrapper(err, code, statusCode)
	claudeErr.LocalError = true
	return claudeErr
}

// RelayErrorHandler 处理上游 Relay 请求的错误响应。
// 解析上游返回的 HTTP 响应体，将其转换为统一的 NexusTokError 格式。
// 支持多种错误格式（OpenAI、Anthropic、Gemini 等），并可选择是否在错误中显示响应体内容。
// 参数:
//   - ctx: 请求上下文
//   - resp: 上游返回的 HTTP 响应（非 200 状态码）
//   - showBodyWhenFail: 是否在错误消息中包含响应体内容（调试用）
// 返回值:
//   - newApiErr: 统一格式的 NexusTokError 对象
func RelayErrorHandler(ctx context.Context, resp *http.Response, showBodyWhenFail bool) (newApiErr *types.NexusTokError) {
	newApiErr = types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	newApiErr.RetryAfter = resp.Header.Get("Retry-After")

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	CloseResponseBodyGracefully(resp)
	var errResponse dto.GeneralErrorResponse
	buildErrWithBody := func(message string) error {
		if message == "" {
			return fmt.Errorf("bad response status code %d, body: %s", resp.StatusCode, string(responseBody))
		}
		return fmt.Errorf("bad response status code %d, message: %s, body: %s", resp.StatusCode, message, string(responseBody))
	}

	err = common.Unmarshal(responseBody, &errResponse)
	if err != nil {
		if showBodyWhenFail {
			newApiErr.Err = buildErrWithBody("")
		} else {
			logger.LogError(ctx, fmt.Sprintf("bad response status code %d, body: %s", resp.StatusCode, string(responseBody)))
			newApiErr.Err = fmt.Errorf("bad response status code %d", resp.StatusCode)
		}
		return
	}

	if common.GetJsonType(errResponse.Error) == "object" {
		// General format error (OpenAI, Anthropic, Gemini, etc.)
		oaiError := errResponse.TryToOpenAIError()
		if oaiError != nil {
			newApiErr = types.WithOpenAIError(*oaiError, resp.StatusCode)
			newApiErr.RetryAfter = resp.Header.Get("Retry-After")
			if showBodyWhenFail {
				newApiErr.Err = buildErrWithBody(newApiErr.Error())
			}
			return
		}
	}
	newApiErr = types.NewOpenAIError(errors.New(errResponse.ToMessage()), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	newApiErr.RetryAfter = resp.Header.Get("Retry-After")
	if showBodyWhenFail {
		newApiErr.Err = buildErrWithBody(newApiErr.Error())
	}
	return
}

// ResetStatusCode 根据状态码映射配置重置错误的 HTTP 状态码。
// 用于将上游返回的错误状态码映射为自定义状态码（如将 429 映射为 503）。
// 状态码 200 不参与重映射。映射值支持字符串和整数两种格式。
// 参数:
//   - newApiErr: 待重映射的错误对象
//   - statusCodeMappingStr: 状态码映射配置的 JSON 字符串（如 '{"429":"503"}'）
func ResetStatusCode(newApiErr *types.NexusTokError, statusCodeMappingStr string) {
	if newApiErr == nil {
		return
	}
	if statusCodeMappingStr == "" || statusCodeMappingStr == "{}" {
		return
	}
	statusCodeMapping := make(map[string]any)
	err := common.Unmarshal([]byte(statusCodeMappingStr), &statusCodeMapping)
	if err != nil {
		return
	}
	if newApiErr.StatusCode == http.StatusOK {
		return
	}
	codeStr := strconv.Itoa(newApiErr.StatusCode)
	if value, ok := statusCodeMapping[codeStr]; ok {
		intCode, ok := parseStatusCodeMappingValue(value)
		if !ok {
			return
		}
		newApiErr.StatusCode = intCode
	}
}

// parseStatusCodeMappingValue 解析状态码映射值，将其转换为整数类型。
// 支持 string、float64、int、json.Number 四种类型的输入。
// 参数:
//   - value: 待解析的映射值
// 返回值:
//   - int: 解析后的状态码整数值
//   - bool: 解析是否成功
func parseStatusCodeMappingValue(value any) (int, bool) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return 0, false
		}
		statusCode, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return statusCode, true
	case float64:
		if v != math.Trunc(v) {
			return 0, false
		}
		return int(v), true
	case int:
		return v, true
	case json.Number:
		statusCode, err := strconv.Atoi(v.String())
		if err != nil {
			return 0, false
		}
		return statusCode, true
	default:
		return 0, false
	}
}

// TaskErrorWrapperLocal 将错误包装为本地任务错误。
// 标记 LocalError 为 true，用于区分本地错误和上游错误。
// 参数:
//   - err: 原始错误对象
//   - code: 错误码
//   - statusCode: HTTP 状态码
// 返回值:
//   - *dto.TaskError: 标记为本地错误的任务错误对象
func TaskErrorWrapperLocal(err error, code string, statusCode int) *dto.TaskError {
	openaiErr := TaskErrorWrapper(err, code, statusCode)
	openaiErr.LocalError = true
	return openaiErr
}

// TaskErrorWrapper 将错误包装为任务错误对象。
// 当错误消息包含网络相关关键词时，会对敏感信息进行遮盖处理。
// 参数:
//   - err: 原始错误对象
//   - code: 错误码
//   - statusCode: HTTP 状态码
// 返回值:
//   - *dto.TaskError: 包装后的任务错误对象
func TaskErrorWrapper(err error, code string, statusCode int) *dto.TaskError {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
		common.SysLog(fmt.Sprintf("error: %s", text))
		//text = "请求上游地址失败"
		text = common.MaskSensitiveInfo(text)
	}
	//避免暴露内部错误
	taskError := &dto.TaskError{
		Code:       code,
		Message:    text,
		StatusCode: statusCode,
		Error:      err,
	}

	return taskError
}

// TaskErrorFromAPIError 将 PreConsumeBilling 返回的 NexusTokError 转换为 TaskError。
func TaskErrorFromAPIError(apiErr *types.NexusTokError) *dto.TaskError {
	if apiErr == nil {
		return nil
	}
	return &dto.TaskError{
		Code:       string(apiErr.GetErrorCode()),
		Message:    apiErr.Err.Error(),
		StatusCode: apiErr.StatusCode,
		Error:      apiErr.Err,
	}
}
