// synthesizer - interface.go
// 本文件定义了认证合成策略的接口。
// 实现了策略模式以支持多种认证来源：
//   - ConfigSynthesizer：从配置 API 密钥生成 Auth 条目
//   - FileSynthesizer：从 OAuth JSON 文件生成 Auth 条目
package synthesizer

import (
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// AuthSynthesizer 定义了从各种来源生成 Auth 条目的接口。
type AuthSynthesizer interface {
	// Synthesize 从给定上下文生成 Auth 条目。
	// 返回 Auth 指针切片和遇到的任何错误。
	Synthesize(ctx *SynthesisContext) ([]*coreauth.Auth, error)
}
