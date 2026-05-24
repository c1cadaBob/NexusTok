// thinking - errors.go
// 该文件定义了思考配置处理过程中的错误类型和错误码。
// 包括后缀格式错误、未知级别、模型不支持思考、级别不支持、预算超范围和提供商不匹配等。

// Package thinking provides unified thinking configuration processing logic.
package thinking

import "net/http"

// ErrorCode 表示思考配置错误的类型。
type ErrorCode string

// 思考配置处理的错误码常量。
const (
	// ErrInvalidSuffix 表示后缀格式无法解析。
	// 示例："model(abc"（缺少右括号）
	ErrInvalidSuffix ErrorCode = "INVALID_SUFFIX"

	// ErrUnknownLevel 表示级别值不在有效列表中。
	// 示例："model(ultra)" 其中 "ultra" 不是有效级别
	ErrUnknownLevel ErrorCode = "UNKNOWN_LEVEL"

	// ErrThinkingNotSupported 表示模型不支持思考功能。
	// 示例：claude-haiku-4-5 没有思考能力
	ErrThinkingNotSupported ErrorCode = "THINKING_NOT_SUPPORTED"

	// ErrLevelNotSupported 表示模型不支持级别模式。
	// 示例：对仅支持预算的模型使用级别模式
	ErrLevelNotSupported ErrorCode = "LEVEL_NOT_SUPPORTED"

	// ErrBudgetOutOfRange 表示预算值超出模型允许范围。
	// 示例：预算 64000 超过最大值 20000
	ErrBudgetOutOfRange ErrorCode = "BUDGET_OUT_OF_RANGE"

	// ErrProviderMismatch 表示提供商与模型不匹配。
	// 示例：将 Claude 格式应用于 Gemini 模型
	ErrProviderMismatch ErrorCode = "PROVIDER_MISMATCH"
)

// ThinkingError 表示思考配置处理过程中发生的错误。
// 提供结构化的错误信息，包括机器可读的错误码和人类可读的描述。
type ThinkingError struct {
	// Code 机器可读的错误码
	Code ErrorCode
	// Message 人类可读的错误描述。应为小写，无尾随句号。
	Message string
	// Model 与此错误相关的模型名称（可选）
	Model string
	// Details 包含额外的上下文信息（可选）
	Details map[string]interface{}
}

// Error 实现 error 接口。直接返回消息，不包含错误码前缀。
// 使用 Code 字段进行程序化错误处理。
func (e *ThinkingError) Error() string {
	return e.Message
}

// NewThinkingError 创建带有给定错误码和消息的 ThinkingError。
func NewThinkingError(code ErrorCode, message string) *ThinkingError {
	return &ThinkingError{
		Code:    code,
		Message: message,
	}
}

// NewThinkingErrorWithModel 创建带有模型上下文的 ThinkingError。
func NewThinkingErrorWithModel(code ErrorCode, message, model string) *ThinkingError {
	return &ThinkingError{
		Code:    code,
		Message: message,
		Model:   model,
	}
}

// StatusCode 实现可移植的状态码接口，供 HTTP 处理器使用。
func (e *ThinkingError) StatusCode() int {
	return http.StatusBadRequest
}
