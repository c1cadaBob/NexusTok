// xai - apply.go
// 本文件实现了 xAI Grok Responses API 模型的 thinking 配置应用逻辑。
//
// xAI 模型使用与 OpenAI Responses API 兼容的 reasoning.effort 格式，支持离散级别。
// 通过嵌入 codex.Applier 复用 Codex 的实现逻辑。
package xai

import (
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/codex"
)

// Applier 实现了 xAI 模型的 thinking.ProviderApplier 接口。
// 通过嵌入 codex.Applier 复用 Codex 的实现逻辑。
type Applier struct {
	codex.Applier
}

var _ thinking.ProviderApplier = (*Applier)(nil)

// NewApplier 创建一个新的 xAI thinking 应用器。
func NewApplier() *Applier {
	return &Applier{}
}

func init() {
	thinking.RegisterProvider("xai", NewApplier())
}
