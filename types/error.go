// Package types - error.go
// 该文件定义了通用错误类型和错误处理函数
//
// 主要类型：
// - OpenAIError：OpenAI 格式的错误响应
// - NexusTokError：NexusTok 内部错误
//
// 核心功能：
// - 错误创建和包装
// - 错误码管理
// - 错误格式转换（OpenAI、Claude 格式）
package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
)

// OpenAIError OpenAI 格式的错误响应
// 符合 OpenAI API 的错误响应规范
type OpenAIError struct {
	Message  string          `json:"message"`               // 错误消息
	Type     string          `json:"type"`                  // 错误类型（如 "invalid_request_error"）
	Param    string          `json:"param"`                 // 相关参数名
	Code     any             `json:"code"`                  // 错误码（可以是字符串或数字）
	Metadata json.RawMessage `json:"metadata,omitempty"`    // 元数据（如 OpenRouter 的额外信息）
}

// ClaudeError Claude 格式的错误响应
// 符合 Anthropic Claude API 的错误响应规范
type ClaudeError struct {
	Type    string `json:"type,omitempty"`    // 错误类型
	Message string `json:"message,omitempty"` // 错误消息
}

// ErrorType 错误类型常量
// 标识错误的来源和格式
type ErrorType string

const (
	ErrorTypeNexusTokError     ErrorType = "nexustok_error"      // NexusTok 内部错误
	ErrorTypeOpenAIError     ErrorType = "openai_error"        // OpenAI 格式错误
	ErrorTypeClaudeError     ErrorType = "claude_error"        // Claude 格式错误
	ErrorTypeMidjourneyError ErrorType = "midjourney_error"    // Midjourney 错误
	ErrorTypeGeminiError     ErrorType = "gemini_error"        // Gemini 错误
	ErrorTypeRerankError     ErrorType = "rerank_error"        // Rerank 错误
	ErrorTypeUpstreamError   ErrorType = "upstream_error"      // 上游服务错误
)

// ErrorCode 错误码常量
// 细分错误的具体原因，用于日志分析和问题定位
type ErrorCode string

const (
	ErrorCodeInvalidRequest         ErrorCode = "invalid_request"              // 无效请求
	ErrorCodeSensitiveWordsDetected ErrorCode = "sensitive_words_detected"     // 检测到敏感词
	ErrorCodeViolationFeeGrokCSAM   ErrorCode = "violation_fee.grok.csam"      // Grok CSAM 违规

	// NexusTok 内部错误码
	ErrorCodeCountTokenFailed   ErrorCode = "count_token_failed"              // Token 计数失败
	ErrorCodeModelPriceError    ErrorCode = "model_price_error"               // 模型价格配置错误
	ErrorCodeInvalidApiType     ErrorCode = "invalid_api_type"                // 无效的 API 类型
	ErrorCodeJsonMarshalFailed  ErrorCode = "json_marshal_failed"             // JSON 序列化失败
	ErrorCodeDoRequestFailed    ErrorCode = "do_request_failed"               // 发送请求失败
	ErrorCodeGetChannelFailed   ErrorCode = "get_channel_failed"              // 获取渠道失败
	ErrorCodeGenRelayInfoFailed ErrorCode = "gen_relay_info_failed"           // 生成中继信息失败

	// 渠道错误码（以 "channel:" 为前缀）
	ErrorCodeChannelNoAvailableKey        ErrorCode = "channel:no_available_key"         // 渠道无可用密钥
	ErrorCodeChannelParamOverrideInvalid  ErrorCode = "channel:param_override_invalid"   // 渠道参数覆盖无效
	ErrorCodeChannelHeaderOverrideInvalid ErrorCode = "channel:header_override_invalid"  // 渠道请求头覆盖无效
	ErrorCodeChannelModelMappedError      ErrorCode = "channel:model_mapped_error"       // 渠道模型映射错误
	ErrorCodeChannelAwsClientError        ErrorCode = "channel:aws_client_error"         // AWS 客户端错误
	ErrorCodeChannelInvalidKey            ErrorCode = "channel:invalid_key"              // 渠道密钥无效
	ErrorCodeChannelResponseTimeExceeded  ErrorCode = "channel:response_time_exceeded"   // 渠道响应超时

	// 客户端请求错误码
	ErrorCodeReadRequestBodyFailed ErrorCode = "read_request_body_failed"    // 读取请求体失败
	ErrorCodeConvertRequestFailed  ErrorCode = "convert_request_failed"      // 转换请求失败
	ErrorCodeAccessDenied          ErrorCode = "access_denied"               // 访问被拒绝

	// 请求错误码
	ErrorCodeBadRequestBody ErrorCode = "bad_request_body"                    // 请求体格式错误

	// 响应错误码
	ErrorCodeReadResponseBodyFailed ErrorCode = "read_response_body_failed"  // 读取响应体失败
	ErrorCodeBadResponseStatusCode  ErrorCode = "bad_response_status_code"   // 响应状态码异常
	ErrorCodeBadResponse            ErrorCode = "bad_response"               // 响应异常
	ErrorCodeBadResponseBody        ErrorCode = "bad_response_body"          // 响应体格式错误
	ErrorCodeEmptyResponse          ErrorCode = "empty_response"             // 空响应
	ErrorCodeAwsInvokeError         ErrorCode = "aws_invoke_error"           // AWS 调用错误
	ErrorCodeModelNotFound          ErrorCode = "model_not_found"            // 模型未找到
	ErrorCodePromptBlocked          ErrorCode = "prompt_blocked"             // 提示词被阻止

	// 数据库错误码
	ErrorCodeQueryDataError  ErrorCode = "query_data_error"                  // 查询数据失败
	ErrorCodeUpdateDataError ErrorCode = "update_data_error"                 // 更新数据失败

	// 配额错误码
	ErrorCodeInsufficientUserQuota      ErrorCode = "insufficient_user_quota"       // 用户配额不足
	ErrorCodePreConsumeTokenQuotaFailed ErrorCode = "pre_consume_token_quota_failed" // 预消耗 Token 配额失败
)

// NexusTokError NexusTok 统一错误结构体
// 封装了所有类型的错误信息，支持多种错误格式的转换
type NexusTokError struct {
	Err            error           // 底层原始错误
	RelayError     any             // 中继层错误（OpenAIError、ClaudeError 等）
	RetryAfter     string          // 重试等待时间（用于限流场景）
	skipRetry      bool            // 是否跳过重试
	recordErrorLog *bool           // 是否记录错误日志（nil 表示默认记录）
	errorType      ErrorType       // 错误类型
	errorCode      ErrorCode       // 错误码
	StatusCode     int             // HTTP 状态码
	Metadata       json.RawMessage // 附加元数据
}

// Unwrap 返回底层的原始错误
// 实现 Go 标准库的 errors.Unwrap 接口，支持 errors.Is / errors.As 的链式错误匹配
//
// 返回值：
//   - error: 底层的原始错误，如果 NexusTokError 本身为 nil 则返回 nil
func (e *NexusTokError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// GetErrorCode 获取错误码
//
// 返回值：
//   - ErrorCode: 错误码常量，如果 NexusTokError 为 nil 则返回空字符串
func (e *NexusTokError) GetErrorCode() ErrorCode {
	if e == nil {
		return ""
	}
	return e.errorCode
}

// GetErrorType 获取错误类型
//
// 返回值：
//   - ErrorType: 错误类型常量（nexustok_error、openai_error、claude_error 等）
func (e *NexusTokError) GetErrorType() ErrorType {
	if e == nil {
		return ""
	}
	return e.errorType
}

// Error 返回错误的字符串描述
// 实现 Go 标准库的 error 接口
//
// 返回值：
//   - string: 错误消息文本，如果底层错误为 nil 则返回错误码字符串
func (e *NexusTokError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		// fallback message when underlying error is missing
		return string(e.errorCode)
	}
	return e.Err.Error()
}

// ErrorWithStatusCode 返回包含 HTTP 状态码的错误描述
// 格式为 "status_code=xxx, 错误消息"
//
// 返回值：
//   - string: 带状态码的错误消息
func (e *NexusTokError) ErrorWithStatusCode() string {
	if e == nil {
		return ""
	}
	msg := e.Error()
	if e.StatusCode == 0 {
		return msg
	}
	if msg == "" {
		return fmt.Sprintf("status_code=%d", e.StatusCode)
	}
	return fmt.Sprintf("status_code=%d, %s", e.StatusCode, msg)
}

// MaskSensitiveError 返回脱敏后的错误消息
// 对错误消息中的敏感信息（如 API 密钥、URL 等）进行掩码处理
// 特殊情况：ErrorCodeCountTokenFailed 类型的错误不做脱敏
//
// 返回值：
//   - string: 脱敏后的错误消息
func (e *NexusTokError) MaskSensitiveError() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return string(e.errorCode)
	}
	errStr := e.Err.Error()
	if e.errorCode == ErrorCodeCountTokenFailed {
		return errStr
	}
	return common.MaskSensitiveInfo(errStr)
}

// MaskSensitiveErrorWithStatusCode 返回包含 HTTP 状态码的脱敏错误描述
// 结合 MaskSensitiveError 和状态码信息
//
// 返回值：
//   - string: 带状态码的脱敏错误消息
func (e *NexusTokError) MaskSensitiveErrorWithStatusCode() string {
	if e == nil {
		return ""
	}
	msg := e.MaskSensitiveError()
	if e.StatusCode == 0 {
		return msg
	}
	if msg == "" {
		return fmt.Sprintf("status_code=%d", e.StatusCode)
	}
	return fmt.Sprintf("status_code=%d, %s", e.StatusCode, msg)
}

// SetMessage 设置错误消息
// 替换底层的错误对象为新的消息文本
//
// 参数：
//   - message: 新的错误消息文本
func (e *NexusTokError) SetMessage(message string) {
	e.Err = errors.New(message)
}

// ToOpenAIError 将 NexusTokError 转换为 OpenAI 格式的错误响应
// 根据错误类型进行不同的转换：
// - OpenAIError 类型：直接提取 OpenAIError 结构体
// - ClaudeError 类型：将 Claude 错误转换为 OpenAI 格式
// - 其他类型：使用错误类型和错误码构建 OpenAIError
//
// 返回值：
//   - OpenAIError: OpenAI 格式的错误结构体，消息中的敏感信息会被脱敏
func (e *NexusTokError) ToOpenAIError() OpenAIError {
	var result OpenAIError
	switch e.errorType {
	case ErrorTypeOpenAIError:
		if openAIError, ok := e.RelayError.(OpenAIError); ok {
			result = openAIError
		}
	case ErrorTypeClaudeError:
		if claudeError, ok := e.RelayError.(ClaudeError); ok {
			result = OpenAIError{
				Message: e.Error(),
				Type:    claudeError.Type,
				Param:   "",
				Code:    e.errorCode,
			}
		}
	default:
		result = OpenAIError{
			Message: e.Error(),
			Type:    string(e.errorType),
			Param:   "",
			Code:    e.errorCode,
		}
	}
	if e.errorCode != ErrorCodeCountTokenFailed {
		result.Message = common.MaskSensitiveInfo(result.Message)
	}
	if result.Message == "" {
		result.Message = string(e.errorType)
	}
	return result
}

// ToClaudeError 将 NexusTokError 转换为 Claude 格式的错误响应
// 根据错误类型进行不同的转换：
// - OpenAIError 类型：将 OpenAI 错误码作为 Claude 错误的 type
// - ClaudeError 类型：直接提取 ClaudeError 结构体
// - 其他类型：使用错误类型构建 ClaudeError
//
// 返回值：
//   - ClaudeError: Claude 格式的错误结构体，消息中的敏感信息会被脱敏
func (e *NexusTokError) ToClaudeError() ClaudeError {
	var result ClaudeError
	switch e.errorType {
	case ErrorTypeOpenAIError:
		if openAIError, ok := e.RelayError.(OpenAIError); ok {
			result = ClaudeError{
				Message: e.Error(),
				Type:    fmt.Sprintf("%v", openAIError.Code),
			}
		}
	case ErrorTypeClaudeError:
		if claudeError, ok := e.RelayError.(ClaudeError); ok {
			result = claudeError
		}
	default:
		result = ClaudeError{
			Message: e.Error(),
			Type:    string(e.errorType),
		}
	}
	if e.errorCode != ErrorCodeCountTokenFailed {
		result.Message = common.MaskSensitiveInfo(result.Message)
	}
	if result.Message == "" {
		result.Message = string(e.errorType)
	}
	return result
}

// NexusTokErrorOptions NexusTokError 的选项函数类型
// 用于通过函数选项模式配置 NexusTokError 的属性
type NexusTokErrorOptions func(*NexusTokError)

// NewError 创建新的 NexusTok 内部错误
// 如果传入的 error 已经是 NexusTokError，则保留其内部状态并应用选项
// 否则创建新的 NexusTokError，默认 HTTP 状态码为 500
//
// 参数：
//   - err: 原始错误
//   - errorCode: 错误码常量
//   - ops: 可选的配置选项函数
//
// 返回值：
//   - *NexusTokError: 创建或包装后的错误对象
func NewError(err error, errorCode ErrorCode, ops ...NexusTokErrorOptions) *NexusTokError {
	var newErr *NexusTokError
	// 保留深层传递的 new err
	if errors.As(err, &newErr) {
		for _, op := range ops {
			op(newErr)
		}
		return newErr
	}
	e := &NexusTokError{
		Err:        err,
		RelayError: nil,
		errorType:  ErrorTypeNexusTokError,
		StatusCode: http.StatusInternalServerError,
		errorCode:  errorCode,
	}
	for _, op := range ops {
		op(e)
	}
	return e
}

// NewOpenAIError 创建 OpenAI 格式的错误
// 如果传入的 error 已经是 NexusTokError，则在其上附加 OpenAIError 信息
// 否则创建新的包含 OpenAIError 的 NexusTokError
//
// 参数：
//   - err: 原始错误
//   - errorCode: 错误码常量
//   - statusCode: HTTP 状态码
//   - ops: 可选的配置选项函数
//
// 返回值：
//   - *NexusTokError: 包含 OpenAI 错误格式的错误对象
func NewOpenAIError(err error, errorCode ErrorCode, statusCode int, ops ...NexusTokErrorOptions) *NexusTokError {
	var newErr *NexusTokError
	// 保留深层传递的 new err
	if errors.As(err, &newErr) {
		if newErr.RelayError == nil {
			openaiError := OpenAIError{
				Message: newErr.Error(),
				Type:    string(errorCode),
				Code:    errorCode,
			}
			newErr.RelayError = openaiError
		}
		for _, op := range ops {
			op(newErr)
		}
		return newErr
	}
	openaiError := OpenAIError{
		Message: err.Error(),
		Type:    string(errorCode),
		Code:    errorCode,
	}
	return WithOpenAIError(openaiError, statusCode, ops...)
}

// InitOpenAIError 初始化 OpenAI 格式的错误（不包含具体的错误消息）
// 适用于只需要错误码和类型，不需要具体错误消息的场景
//
// 参数：
//   - errorCode: 错误码常量
//   - statusCode: HTTP 状态码
//   - ops: 可选的配置选项函数
//
// 返回值：
//   - *NexusTokError: 初始化后的错误对象
func InitOpenAIError(errorCode ErrorCode, statusCode int, ops ...NexusTokErrorOptions) *NexusTokError {
	openaiError := OpenAIError{
		Type: string(errorCode),
		Code: errorCode,
	}
	return WithOpenAIError(openaiError, statusCode, ops...)
}

// NewErrorWithStatusCode 创建带 HTTP 状态码的 NexusTok 内部错误
// 与 NewError 类似，但允许指定自定义的 HTTP 状态码
//
// 参数：
//   - err: 原始错误
//   - errorCode: 错误码常量
//   - statusCode: HTTP 状态码
//   - ops: 可选的配置选项函数
//
// 返回值：
//   - *NexusTokError: 创建的错误对象
func NewErrorWithStatusCode(err error, errorCode ErrorCode, statusCode int, ops ...NexusTokErrorOptions) *NexusTokError {
	e := &NexusTokError{
		Err: err,
		RelayError: OpenAIError{
			Message: err.Error(),
			Type:    string(errorCode),
		},
		errorType:  ErrorTypeNexusTokError,
		StatusCode: statusCode,
		errorCode:  errorCode,
	}
	for _, op := range ops {
		op(e)
	}

	return e
}

// WithOpenAIError 使用 OpenAIError 结构体创建 NexusTokError
// 将 OpenAI 格式的错误信息包装为 NexusTokError
// 特殊处理：如果 OpenAIError 包含 Metadata（如 OpenRouter），会附加到错误消息中
//
// 参数：
//   - openAIError: OpenAI 格式的错误结构体
//   - statusCode: HTTP 状态码
//   - ops: 可选的配置选项函数
//
// 返回值：
//   - *NexusTokError: 包含 OpenAI 错误格式的错误对象
func WithOpenAIError(openAIError OpenAIError, statusCode int, ops ...NexusTokErrorOptions) *NexusTokError {
	code, ok := openAIError.Code.(string)
	if !ok {
		if openAIError.Code != nil {
			code = fmt.Sprintf("%v", openAIError.Code)
		} else {
			code = "unknown_error"
		}
	}
	if openAIError.Type == "" {
		openAIError.Type = "upstream_error"
	}
	e := &NexusTokError{
		RelayError: openAIError,
		errorType:  ErrorTypeOpenAIError,
		StatusCode: statusCode,
		Err:        errors.New(openAIError.Message),
		errorCode:  ErrorCode(code),
	}
	// OpenRouter
	if len(openAIError.Metadata) > 0 {
		openAIError.Message = fmt.Sprintf("%s (%s)", openAIError.Message, openAIError.Metadata)
		e.Metadata = openAIError.Metadata
		e.RelayError = openAIError
		e.Err = errors.New(openAIError.Message)
	}
	for _, op := range ops {
		op(e)
	}
	return e
}

// WithClaudeError 使用 ClaudeError 结构体创建 NexusTokError
// 将 Claude 格式的错误信息包装为 NexusTokError
//
// 参数：
//   - claudeError: Claude 格式的错误结构体
//   - statusCode: HTTP 状态码
//   - ops: 可选的配置选项函数
//
// 返回值：
//   - *NexusTokError: 包含 Claude 错误格式的错误对象
func WithClaudeError(claudeError ClaudeError, statusCode int, ops ...NexusTokErrorOptions) *NexusTokError {
	if claudeError.Type == "" {
		claudeError.Type = "upstream_error"
	}
	e := &NexusTokError{
		RelayError: claudeError,
		errorType:  ErrorTypeClaudeError,
		StatusCode: statusCode,
		Err:        errors.New(claudeError.Message),
		errorCode:  ErrorCode(claudeError.Type),
	}
	for _, op := range ops {
		op(e)
	}
	return e
}

// IsChannelError 判断是否为渠道错误
// 渠道错误的错误码以 "channel:" 前缀标识
//
// 参数：
//   - err: NexusTokError 对象
//
// 返回值：
//   - bool: 是渠道错误返回 true，否则返回 false
func IsChannelError(err *NexusTokError) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(string(err.errorCode), "channel:")
}

// IsSkipRetryError 判断是否为跳过重试的错误
// 某些错误（如参数错误）不需要重试，通过此函数判断
//
// 参数：
//   - err: NexusTokError 对象
//
// 返回值：
//   - bool: 应跳过重试返回 true，否则返回 false
func IsSkipRetryError(err *NexusTokError) bool {
	if err == nil {
		return false
	}

	return err.skipRetry
}

// ErrOptionWithSkipRetry 返回设置跳过重试标志的选项函数
// 标记该错误不应被重试
//
// 返回值：
//   - NexusTokErrorOptions: 选项函数
func ErrOptionWithSkipRetry() NexusTokErrorOptions {
	return func(e *NexusTokError) {
		e.skipRetry = true
	}
}

// ErrOptionWithNoRecordErrorLog 返回设置不记录错误日志的选项函数
// 标记该错误不需要记录到错误日志中
//
// 返回值：
//   - NexusTokErrorOptions: 选项函数
func ErrOptionWithNoRecordErrorLog() NexusTokErrorOptions {
	return func(e *NexusTokError) {
		e.recordErrorLog = common.GetPointer(false)
	}
}

// ErrOptionWithStatusCode 返回设置 HTTP 状态码的选项函数
//
// 参数：
//   - statusCode: HTTP 状态码
//
// 返回值：
//   - NexusTokErrorOptions: 选项函数
func ErrOptionWithStatusCode(statusCode int) NexusTokErrorOptions {
	return func(e *NexusTokError) {
		e.StatusCode = statusCode
	}
}

// ErrOptionWithHideErrMsg 返回隐藏错误消息的选项函数
// 将错误消息替换为指定的通用消息，防止敏感信息泄露
// 调试模式下会打印原始错误信息
//
// 参数：
//   - replaceStr: 替换后的错误消息
//
// 返回值：
//   - NexusTokErrorOptions: 选项函数
func ErrOptionWithHideErrMsg(replaceStr string) NexusTokErrorOptions {
	return func(e *NexusTokError) {
		if common.DebugEnabled {
			fmt.Printf("ErrOptionWithHideErrMsg: %s, origin error: %s", replaceStr, e.Err)
		}
		e.Err = errors.New(replaceStr)
	}
}

// IsRecordErrorLog 判断是否应记录错误日志
// 默认返回 true，除非通过 ErrOptionWithNoRecordErrorLog 显式设置为 false
//
// 参数：
//   - e: NexusTokError 对象
//
// 返回值：
//   - bool: 应记录错误日志返回 true，否则返回 false
func IsRecordErrorLog(e *NexusTokError) bool {
	if e == nil {
		return false
	}
	if e.recordErrorLog == nil {
		// default to true if not set
		return true
	}
	return *e.recordErrorLog
}
