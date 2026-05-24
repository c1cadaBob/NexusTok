// Package dto - error.go
// 该文件定义了错误响应相关的数据传输对象
//
// 主要结构体：
// - OpenAIErrorWithStatusCode：带 HTTP 状态码的 OpenAI 格式错误
// - GeneralErrorResponse：通用错误响应（兼容多种上游 provider 的错误格式）
//
// GeneralErrorResponse 兼容说明：
// 支持从多种上游 provider 的错误响应中提取错误信息，包括：
// - OpenAI 格式（error 对象）
// - 字符串格式（error/message/msg/err/error_msg 字段）
// - 嵌套格式（header.message、response.error.message）
// - JSON 字符串格式的 error 字段
package dto

import (
	"encoding/json"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/types"
)

//type OpenAIError struct {
//	Message string `json:"message"`
//	Type    string `json:"type"`
//	Param   string `json:"param"`
//	Code    any    `json:"code"`
//}

// OpenAIErrorWithStatusCode 带 HTTP 状态码的 OpenAI 格式错误
// Error：OpenAI 标准错误对象
// StatusCode：HTTP 状态码
// LocalError：标记是否为本地产生的错误（非上游返回）
type OpenAIErrorWithStatusCode struct {
	Error      types.OpenAIError `json:"error"`
	StatusCode int               `json:"status_code"`
	LocalError bool
}

// GeneralErrorResponse 通用错误响应结构体
// 兼容多种上游 AI provider 的错误格式
// Error：错误对象或 JSON 字符串（兼容 OpenAI 格式）
// Message/Msg/Err/ErrorMsg/Detail：各 provider 的错误消息字段
// Metadata：扩展元数据
// Header.Message：嵌套头部错误消息（如某些国产模型）
// Response.Error.Message：嵌套响应错误消息（如某些 REST API）
type GeneralErrorResponse struct {
	Error    json.RawMessage `json:"error"`
	Message  string          `json:"message"`
	Msg      string          `json:"msg"`
	Err      string          `json:"err"`
	ErrorMsg string          `json:"error_msg"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
	Detail   string          `json:"detail,omitempty"`
	Header   struct {
		Message string `json:"message"`
	} `json:"header"`
	Response struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
}

// TryToOpenAIError 尝试将通用错误转换为 OpenAI 标准错误格式
// 仅当 Error 字段是有效的 OpenAI 格式 JSON 对象时返回非 nil
func (e GeneralErrorResponse) TryToOpenAIError() *types.OpenAIError {
	var openAIError types.OpenAIError
	if len(e.Error) > 0 {
		err := common.Unmarshal(e.Error, &openAIError)
		if err == nil && openAIError.Message != "" {
			return &openAIError
		}
	}
	return nil
}

// ToMessage 从通用错误响应中提取错误消息字符串
// 按优先级尝试以下来源：
// 1. Error 字段（JSON 对象或字符串）
// 2. Message/Msg/Err/ErrorMsg/Detail 字段
// 3. Header.Message 或 Response.Error.Message 嵌套字段
// 返回第一个非空的错误消息，全部为空则返回空字符串
func (e GeneralErrorResponse) ToMessage() string {
	if len(e.Error) > 0 {
		switch common.GetJsonType(e.Error) {
		case "object":
			var openAIError types.OpenAIError
			err := common.Unmarshal(e.Error, &openAIError)
			if err == nil && openAIError.Message != "" {
				return openAIError.Message
			}
		case "string":
			var msg string
			err := common.Unmarshal(e.Error, &msg)
			if err == nil && msg != "" {
				return msg
			}
		default:
			return string(e.Error)
		}
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Msg != "" {
		return e.Msg
	}
	if e.Err != "" {
		return e.Err
	}
	if e.ErrorMsg != "" {
		return e.ErrorMsg
	}
	if e.Detail != "" {
		return e.Detail
	}
	if e.Header.Message != "" {
		return e.Header.Message
	}
	if e.Response.Error.Message != "" {
		return e.Response.Error.Message
	}
	return ""
}
