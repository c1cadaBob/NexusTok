// Package relay 实现了 AI API 中继/代理的核心逻辑。
// 本文件负责将参数覆盖（Param Override）过程中产生的错误转换为统一的 NexusTok 错误格式，
// 以便在 API 响应中返回结构化的错误信息。
package relay

import (
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/types"
)

// newAPIErrorFromParamOverride 将参数覆盖过程中的错误转换为统一的 NexusTokError。
// 如果错误是 ParamOverrideReturnError 类型（即由 return_error 操作显式触发的），
// 则使用其自定义的错误信息；否则包装为通用的渠道参数覆盖无效错误。
//
// 参数：
//   - err: 参数覆盖过程中产生的原始错误
//
// 返回值：
//   - *types.NexusTokError: 格式化后的 API 错误对象
func newAPIErrorFromParamOverride(err error) *types.NexusTokError {
	// 尝试将错误断言为 ParamOverrideReturnError 类型
	if fixedErr, ok := relaycommon.AsParamOverrideReturnError(err); ok {
		// 使用自定义错误信息（包含 status_code、code、message 等）
		return relaycommon.NexusTokErrorFromParamOverride(fixedErr)
	}
	// 通用参数覆盖无效错误，跳过重试
	return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
}
