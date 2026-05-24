// translator - helpers.go
// 该文件提供按格式名称（字符串标识符）调用翻译功能的便捷辅助函数。
// 这些函数是对底层翻译函数的简单包装，使调用方可以使用 Format 类型（字符串别名）进行操作。

package translator

import "context"

// TranslateRequestByFormatName 通过字符串标识符在不同格式之间转换请求载荷。
// 这是对 TranslateRequest 的便捷包装。
func TranslateRequestByFormatName(from, to Format, model string, rawJSON []byte, stream bool) []byte {
	return TranslateRequest(from, to, model, rawJSON, stream)
}

// HasResponseTransformerByFormatName 检查两个格式之间是否存在响应转换器。
func HasResponseTransformerByFormatName(from, to Format) bool {
	return HasResponseTransformer(from, to)
}

// TranslateStreamByFormatName 通过字符串标识符在不同格式之间转换流式响应。
func TranslateStreamByFormatName(ctx context.Context, from, to Format, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) [][]byte {
	return TranslateStream(ctx, from, to, model, originalRequestRawJSON, requestRawJSON, rawJSON, param)
}

// TranslateNonStreamByFormatName 通过字符串标识符在不同格式之间转换非流式响应。
func TranslateNonStreamByFormatName(ctx context.Context, from, to Format, model string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []byte {
	return TranslateNonStream(ctx, from, to, model, originalRequestRawJSON, requestRawJSON, rawJSON, param)
}

// TranslateTokenCountByFormatName 通过字符串标识符在不同格式之间转换 token 计数。
func TranslateTokenCountByFormatName(ctx context.Context, from, to Format, count int64, rawJSON []byte) []byte {
	return TranslateTokenCount(ctx, from, to, count, rawJSON)
}
